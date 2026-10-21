package optimization

import (
	"fmt"
	"strconv"
	"strings"
	"testing"

	"leek/bin/constraints"
	"leek/bin/parser"
)

const specializationCases = `fn minus(x u32) u32 { return x-1 }
fn inverse(x u32) u32 { return 12/x }
fn subtract(x u32,amount u32) u32 { return x-amount }
fn safe(x u32) u32 { if x > 1 { return x -> minus -> inverse } return 0 }
fn direct(x u32) u32 { if x > 1 { return inverse(minus(x)) } return 0 }
fn other(x u32) u32 { return minus(x) }
fn nested(x u32) u32 { return inverse(minus(x)) }
fn wrapper(x u32) u32 { if x > 1 { return nested(x) } return 0 }
fn ten(x u32) u32 { if x >= 10 { return subtract(x,10) } return 0 }
fn ten_pipe(x u32) u32 { if x >= 10 { return x -> subtract(10) } return 0 }
fn reversed(x u32) u32 { if x > 10 { return subtract(10,x) } return 0 }
fn mixed(x u32,y u32) u32 { return (x-1)/y }
fn partial(x u32) u32 { if x > 0 { return mixed(x,0) } return 0 }
fn choose(x u32) u32 { if x > 10 { return x*x } return x+4294967295 }
fn branch(x u32) u32 { if x > 10 && x <= 100 { return choose(x) } return 0 }
fn branch_unknown(x u32) u32 { return choose(x) }
fn short(x u32) u32 { if x > 1 && inverse(minus(x)) > 0 { return 1 } return 0 }
fn disjunction(x u32) u32 { if x > 1 || x == 0 { return minus(x) } return 0 }
fn unreachable(x u32) u32 { if x > 10 && x < 5 { return minus(0) } return x }
fn siblings(x u32) u32 { if x > 1 { let a u32 = minus(x) } return minus(x) }
fn signed(x i8) i8 { return x-1 }
fn signed_caller(x i8) i8 { if x > 1 { return signed(x) } return 0 }
impure fn marked(x u32) u32 { print(x) return x }
impure fn effect(x u32) u32 { if x > 1 { print(7) let observed u32 = marked(x) return minus(x) } return 0 }
`

func bodyFor(t testing.TB, code, name string) string {
	t.Helper()
	for _, line := range strings.Split(code, "\n") {
		if !strings.HasPrefix(line, " ") && strings.Contains(line, " "+name+"(") && strings.HasSuffix(line, " {") {
			_, rest, _ := strings.Cut(code, line+"\n")
			body, _, ok := strings.Cut(rest, "\n}")
			if ok {
				return body
			}
		}
	}
	t.Fatalf("missing function body %s", name)
	return ""
}

func TestSpecializationStructureAndProvenance(t *testing.T) {
	g := graphFor(t, specializationCases)
	canonical, _ := parser.NewCodeGenerator().Generate(g)
	dot := g.ToDOT()
	r, err := GenerateSpecialized(g)
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Variants) == 0 || len(r.Witnesses) == 0 {
		t.Fatal("never specialized")
	}
	variants := map[string]Variant{}
	for _, v := range r.Variants {
		variants[v.Name] = v
	}
	for _, w := range r.Witnesses {
		var f *constraints.FunctionReport
		for _, candidate := range r.Analysis.Functions {
			if candidate.Name == w.Caller {
				f = candidate
			}
		}
		if f == nil {
			t.Fatal(w)
		}
		o := f.Obligations[w.Obligation]
		if o.Proof.Status != constraints.Proven || !o.Proof.Consistent || o.Environment != w.Environment || o.Origin.Node != w.Operation || fmt.Sprint(o.Origin.Via) != fmt.Sprint(w.Path) {
			t.Fatalf("wrong proof witness: %+v", w)
		}
	}
	for _, name := range []string{"minus", "inverse", "subtract", "mixed", "nested"} {
		body := bodyFor(t, r.Code, g.Functions[name].CName)
		if strings.Contains(body, "= lk_spec_") || strings.Contains(body, "proven guard elided") {
			t.Fatalf("caller facts leaked into %s: %s", name, body)
		}
	}
	for _, name := range []string{"safe", "direct", "wrapper", "ten", "ten_pipe", "partial", "branch", "short", "effect"} {
		if !strings.Contains(bodyFor(t, r.Code, g.Functions[name].CName), "= lk_spec_") {
			t.Fatalf("%s did not specialize", name)
		}
	}
	for _, name := range []string{"other", "reversed", "disjunction", "unreachable", "branch_unknown", "signed_caller"} {
		if strings.Contains(bodyFor(t, r.Code, g.Functions[name].CName), "= lk_spec_") {
			t.Fatalf("unproven/unsupported %s specialized", name)
		}
	}
	if body := bodyFor(t, r.Code, g.Functions["siblings"].CName); !strings.Contains(body, "= lk_spec_") || !strings.Contains(body, g.Functions["minus"].CName+"(") {
		t.Fatal("sibling facts leaked", body)
	}
	counts := map[string]int{}
	for _, v := range r.Variants {
		counts[v.Function]++
		body := bodyFor(t, r.Code, v.Name)
		for _, node := range v.Eliminated {
			raw := statementAfter(t, body, "// arithmetic "+node+": proven guard elided")
			checked := statementAfter(t, canonical, "// arithmetic "+node+": checked")
			if strings.Contains(raw, "= lk_") || !strings.Contains(checked, "= lk_") {
				t.Fatal(raw, checked)
			}
		}
		for _, route := range v.Calls {
			if _, ok := variants[route.Target]; !ok || !strings.Contains(body, route.Target+"(") {
				t.Fatal("dangling specialization route", route)
			}
		}
		if v.Function == "mixed" && (len(v.Eliminated) != 1 || !strings.Contains(body, "lk_div_u32(") || strings.Contains(body, "lk_sub_u32(")) {
			t.Fatal("partial proof lost unchecked obligation", body)
		}
		if v.Function == "choose" && !strings.Contains(body, "lk_add_u32(") {
			t.Fatal("alternative branch guard lost")
		}
	}
	if counts["minus"] != 1 || counts["inverse"] != 1 || counts["nested"] != 1 || counts["mixed"] != 1 {
		t.Fatalf("missing nested variant or equivalent variants not reused: %v", counts)
	}
	after, _ := parser.NewCodeGenerator().Generate(g)
	again, err := GenerateSpecialized(g)
	if err != nil || canonical != after || dot != g.ToDOT() || r.Code != again.Code || fmt.Sprint(r.Variants) != fmt.Sprint(again.Variants) || fmt.Sprint(r.Witnesses) != fmt.Sprint(again.Witnesses) {
		t.Fatal("IR changed or nondeterministic specialization")
	}
}

func specializationHarness(g *parser.ProgramGraph) (string, []string) {
	names := []string{"safe", "direct", "other", "nested", "wrapper", "ten", "ten_pipe", "reversed", "partial", "branch", "branch_unknown", "short", "disjunction", "unreachable", "siblings", "effect", "minus", "inverse"}
	h := "\nint main(int argc,char** argv) { if(argc!=3)return 2; uint32_t x=(uint32_t)strtoull(argv[2],NULL,10); uint32_t result=0; switch(atoi(argv[1])) {\n"
	for i, name := range names {
		h += fmt.Sprintf("case %d: result=%s(x); break;\n", i, g.Functions[name].CName)
	}
	h += "default:return 2;} printf(\"%\" PRIu32 \"\\n\",result);return 0;}\n"
	return h, names
}

func TestSpecializationNativeDifferential(t *testing.T) {
	g := graphFor(t, specializationCases)
	a, _ := parser.NewCodeGenerator().Generate(g)
	r, err := GenerateSpecialized(g)
	if err != nil || len(r.Variants) == 0 {
		t.Fatal(err)
	}
	h, names := specializationHarness(g)
	inputs := []uint64{0, 1, 2, 3, 4, 5, 9, 10, 11, 12, 13, 100, 4294967294, 4294967295}
	for _, opt := range []string{"-O0", "-O3"} {
		ca := buildC(t, a+h, opt)
		cb := buildC(t, r.Code+h, opt)
		for i, name := range names {
			for _, x := range inputs {
				args := []string{strconv.Itoa(i), strconv.FormatUint(x, 10)}
				want, got := execute(t, ca, args...), execute(t, cb, args...)
				if want != got {
					t.Fatalf("%s %s(%d): %+v != %+v", opt, name, x, want, got)
				}
				if (name == "safe" || name == "direct" || name == "wrapper") && x > 1 && (want.status != 0 || want.out != fmt.Sprint(12/(x-1))+"\n") {
					t.Fatal("independent pipeline oracle", want)
				}
				if (name == "other" || name == "siblings" || name == "disjunction") && x == 0 && (want.status != 1 || !strings.Contains(want.err, "underflow")) {
					t.Fatal("unknown caller lost underflow", want)
				}
				if name == "partial" && x > 0 && (want.status != 1 || !strings.Contains(want.err, "division by zero")) {
					t.Fatal("partial proof erased failure", want)
				}
				if name == "effect" && x > 1 && (want.status != 0 || want.out != "7\n"+fmt.Sprint(x)+"\n"+fmt.Sprint(x-1)+"\n") {
					t.Fatal("effect trace", want)
				}
			}
		}
	}
}

func TestSpecializedArgumentsAndFirstFailure(t *testing.T) {
	prelude := `fn subtract(x u32,a u32) u32 { return x-a }
fn pair(x u32,y u32) u32 { return x+y }
impure fn mark(x u32) u32 { print(x) return x }
fn bad(x u32) u32 { return 1/x }
fn minus(x u32) u32 { return x-1 }
`
	for _, tt := range []struct {
		body, out, reason string
		status            int
	}{
		{`print(pair(subtract(10,mark(1)),mark(2)))`, "1\n2\n11\n", "", 0},
		{`print(subtract(mark(10),1))`, "10\n9\n", "", 0},
		{`print(pair(mark(2),bad(0))) print(99)`, "2\n", "division by zero", 1},
		{`print(pair(bad(0),mark(2))) print(99)`, "", "division by zero", 1},
		{`print(subtract(minus(0),1)) print(99)`, "", "underflow", 1},
		{`if false && minus(0)>0 { print(99) } if true || bad(0)>0 { print(7) }`, "7\n", "", 0},
		{`print(subtract(10,mark(1))) print(subtract(mark(0),1)) print(99)`, "1\n9\n0\n", "underflow", 1},
	} {
		g := graphFor(t, prelude+"impure fn main() {"+tt.body+"}")
		a, _ := parser.NewCodeGenerator().Generate(g)
		r, err := GenerateSpecialized(g)
		if err != nil {
			t.Fatal(err)
		}
		for _, opt := range []string{"-O0", "-O3"} {
			want := execute(t, buildC(t, a, opt))
			got := execute(t, buildC(t, r.Code, opt))
			if want != got || want.out != tt.out || want.status != tt.status || !strings.Contains(want.err, tt.reason) {
				t.Fatalf("%s: %s: %+v != %+v", opt, tt.body, want, got)
			}
		}
	}
}

func TestSpecializationBudgetAndFallback(t *testing.T) {
	source := `fn target(a u32,b u32,c u32,d u32,e u32) u32 {
let aa u32 = 10/a let bb u32 = 10/b let cc u32 = 10/c let dd u32 = 10/d let ee u32 = 10/e return 0 }
`
	for i := 0; i < 5; i++ {
		args := []string{"x", "x", "x", "x", "x"}
		args[i] = "1"
		source += fmt.Sprintf("fn caller%d(x u32) u32 { return target(%s) }\n", i, strings.Join(args, ","))
	}
	g := graphFor(t, source)
	r, err := GenerateSpecialized(g)
	if err != nil || len(r.Variants) != maxVariantsPerFunction {
		t.Fatalf("budget: %v %+v", err, r)
	}
	for i := 0; i < 5; i++ {
		body := bodyFor(t, r.Code, g.Functions[fmt.Sprint("caller", i)].CName)
		if strings.Contains(body, "= lk_spec_") != (i < 4) {
			t.Fatal("nondeterministic budget fallback", body)
		}
	}
	h := "\nint main(int argc,char**argv) { if(argc!=3)return 2; uint32_t x=(uint32_t)strtoull(argv[2],NULL,10); switch(atoi(argv[1])) {"
	for i := 0; i < 5; i++ {
		h += fmt.Sprintf("case %d: return (int)%s(x);", i, g.Functions[fmt.Sprint("caller", i)].CName)
	}
	h += "default:return 2;} }\n"
	canonical, _ := parser.NewCodeGenerator().Generate(g)
	a, b := buildC(t, canonical+h, "-O0"), buildC(t, r.Code+h, "-O0")
	for i := 0; i < 5; i++ {
		for _, x := range []string{"0", "1"} {
			args := []string{strconv.Itoa(i), x}
			if want, got := execute(t, a, args...), execute(t, b, args...); want != got {
				t.Fatalf("budget fallback changed behavior: %+v != %+v", want, got)
			}
		}
	}
}

func TestSpecializationGlobalBudget(t *testing.T) {
	var source strings.Builder
	for i := 0; i < maxVariants+1; i++ {
		fmt.Fprintf(&source, "fn leaf%d(x u32) u32 { return x-1 } fn caller%d(x u32) u32 { if x>1 { return leaf%d(x) } return 0 }\n", i, i, i)
	}
	g := graphFor(t, source.String())
	r, err := GenerateSpecialized(g)
	if err != nil || len(r.Variants) != maxVariants {
		t.Fatalf("global budget: %v %d", err, len(r.Variants))
	}
	count := 0
	for i := 0; i < maxVariants+1; i++ {
		if strings.Contains(bodyFor(t, r.Code, g.Functions[fmt.Sprint("caller", i)].CName), "= lk_spec_") {
			count++
		}
	}
	if count != maxVariants {
		t.Fatal("missing canonical fallback", count)
	}
	buildC(t, r.Code+"\nint main(void) { return 0; }\n", "-O0")
}

func TestSpecializationDoesNotCloneAlreadyOptimizedWrapper(t *testing.T) {
	g := graphFor(t, `fn minus(x u32) u32 { return x-1 }
fn safe(x u32) u32 { if x>0 { return minus(x) } return 0 }
fn caller(x u32) u32 { return safe(x) }`)
	r, err := GenerateSpecialized(g)
	if err != nil || len(r.Variants) != 1 || r.Variants[0].Function != "minus" {
		t.Fatalf("cloned a body without additional benefit: %v %+v", err, r.Variants)
	}
	if body := bodyFor(t, r.Code, g.Functions["caller"].CName); !strings.Contains(body, g.Functions["safe"].CName+"(") {
		t.Fatal("redundant wrapper call", body)
	}
}

func TestSpecializationUnsignedWidths(t *testing.T) {
	for _, typ := range []string{"u8", "u16", "u32", "u64"} {
		source := fmt.Sprintf(`fn op(x %s,a %s) %s { let p %s = x+a let q %s = p*x return q/a + q%%a }
fn caller(x %s) %s { if x<=10 { return op(x,1) } return x-1 }`, typ, typ, typ, typ, typ, typ, typ)
		g := graphFor(t, source)
		r, err := GenerateSpecialized(g)
		if err != nil || len(r.Variants) != 1 || len(r.Variants[0].Eliminated) < 5 {
			t.Fatalf("%s: %v %+v", typ, err, r.Variants)
		}
		a, _ := parser.NewCodeGenerator().Generate(g)
		h := fmt.Sprintf("\nint main(void) { for(unsigned x=0;x<256;x++) printf(\"%%\" PRIu64 \"\\n\",(uint64_t)%s(x)); return 0; }\n", g.Functions["caller"].CName)
		for _, opt := range []string{"-O0", "-O3"} {
			want, got := execute(t, buildC(t, a+h, opt)), execute(t, buildC(t, r.Code+h, opt))
			if want != got || want.status != 0 {
				t.Fatalf("%s %s: %+v != %+v", typ, opt, want, got)
			}
		}
	}
}

func TestSpecializationIncompleteAndDepthFallback(t *testing.T) {
	source := `fn incomplete(x u32,y f64) u32 { return x-1 }
fn caller(x u32,y f64) u32 { if x > 1 { return incomplete(x,y) } return 0 }
fn leaf(x u32) u32 { return x-1 }
`
	for i := 0; i < maxSpecializationDepth+1; i++ {
		next := "leaf"
		if i > 0 {
			next = fmt.Sprint("chain", i-1)
		}
		source += fmt.Sprintf("fn chain%d(x u32) u32 { return %s(x) }\n", i, next)
	}
	source += fmt.Sprintf("fn root(x u32) u32 { if x>1 { return chain%d(x) } return 0 }", maxSpecializationDepth)
	g := graphFor(t, source)
	r, err := GenerateSpecialized(g)
	if err != nil || len(r.Variants) != 0 {
		t.Fatalf("incomplete or too deep specialization: %v %+v", err, r.Variants)
	}
}

func FuzzSpecializationNativeEquivalence(f *testing.F) {
	g := graphFor(f, specializationCases)
	a, _ := parser.NewCodeGenerator().Generate(g)
	r, err := GenerateSpecialized(g)
	if err != nil || len(r.Variants) == 0 {
		f.Fatal(err)
	}
	h, names := specializationHarness(g)
	ca := buildC(f, a+h, "-O0")
	cb := buildC(f, r.Code+h, "-O0")
	for i := range names {
		for _, x := range []uint32{0, 1, 2, 10, 11, 4294967294, 4294967295} {
			f.Add(byte(i), x)
		}
	}
	f.Fuzz(func(t *testing.T, which byte, x uint32) {
		args := []string{strconv.Itoa(int(which) % len(names)), strconv.FormatUint(uint64(x), 10)}
		want, got := execute(t, ca, args...), execute(t, cb, args...)
		if want != got {
			t.Fatalf("%v: %+v != %+v", args, want, got)
		}
	})
}
