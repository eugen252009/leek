package optimization

import (
	"fmt"
	"reflect"
	"strconv"
	"strings"
	"testing"

	"leek/bin/constraints"
	"leek/bin/parser"
)

const policyCases = `fn two(x u32) u32 { let earlier u32 = x-1 return x-10 }
fn a_first(x u32) u32 { if x>=1 { return two(x) } return 0 }
fn b_second(x u32) u32 { if x>=10 { return two(x) } return 0 }
fn c_strong(x u32) u32 { if x>=20 { return two(x) } return 0 }
fn d_equal(x u32) u32 { if x==5 { return two(x) } return 0 }
fn e_weak(x u32) u32 { if x>=0 { return two(x) } return 0 }
fn f_unrelated(x u32,y u32) u32 { if y>10 { return two(x) } return 0 }
fn g_contradictory(x u32) u32 { if x>10 && x<5 { return two(x) } return 0 }
impure fn mark(x u32) u32 { print(x) return x }
impure fn h_effect(x u32) u32 { if x>=20 { let observed u32=mark(x) return x -> two } return 0 }
fn i_short(x u32) u32 { if x>=10 && (x -> two)>0 { return 1 } return 0 }
`

func selectedFor(t testing.TB, r *SpecializationResult, caller string) Decision {
	t.Helper()
	for _, d := range r.Decisions {
		if d.Caller == caller && len(d.Path) == 1 {
			return d
		}
	}
	t.Fatalf("no decision for %s", caller)
	return Decision{}
}

func TestPolicySubsumptionRankingAndFallback(t *testing.T) {
	g := graphFor(t, policyCases)
	before, _ := parser.NewCodeGenerator().Generate(g)
	r, err := GenerateSpecialized(g)
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Variants) != 2 || len(r.Variants[0].Assumptions) != 1 || len(r.Variants[1].Assumptions) != 2 {
		t.Fatalf("unexpected registry: %+v", r.Variants)
	}
	a, b := r.Variants[0], r.Variants[1]
	for caller, want := range map[string]string{"a_first": a.Name, "b_second": b.Name, "c_strong": b.Name, "d_equal": a.Name, "h_effect": b.Name, "i_short": b.Name} {
		d := selectedFor(t, r, caller)
		if d.Selected != want || d.FinalTarget != want || !strings.Contains(bodyFor(t, r.Code, g.Functions[caller].CName), "= "+want+"(") {
			t.Fatalf("%s selected wrong variant: %+v", caller, d)
		}
	}
	for _, caller := range []string{"e_weak", "f_unrelated", "g_contradictory"} {
		d := selectedFor(t, r, caller)
		if d.Action != "FALLBACK" || d.FinalTarget != g.Functions["two"].CName {
			t.Fatalf("weaker/unrelated/inconsistent caller reused variant: %+v", d)
		}
	}
	if d := selectedFor(t, r, "d_equal"); len(d.Considered) != 2 || d.Considered[1].Proof.Status != constraints.Violated {
		t.Fatal("stronger contract was not independently rejected", d)
	}
	if d := selectedFor(t, r, "c_strong"); len(d.Considered) != 2 || d.Considered[0].Proof.Status != constraints.Proven || d.Considered[1].Proof.Status != constraints.Proven || d.Action != "REUSE" {
		t.Fatal("did not rank multiple legal variants", d)
	}
	if a.Cost.RemovedGuards != 1 || b.Cost.RemovedGuards != 2 || a.Cost.RemainingGuards != 1 || b.Cost.RemainingGuards != 0 || r.Summary.Reused < 4 {
		t.Fatal("inaccurate registry metrics", r.Summary, a.Cost, b.Cost)
	}
	if !strings.Contains(bodyFor(t, r.Code, g.Functions["two"].CName), "lk_sub_u32(") {
		t.Fatal("canonical body lost")
	}
	after, _ := parser.NewCodeGenerator().Generate(g)
	again, err := GenerateSpecialized(g)
	if err != nil || before != after || !reflect.DeepEqual(r.Decisions, again.Decisions) || !reflect.DeepEqual(r.Variants, again.Variants) || r.Code != again.Code {
		t.Fatal("policy mutated IR or is nondeterministic")
	}
}

func TestPolicyEquivalentAndStrongerFactsShareVariant(t *testing.T) {
	g := graphFor(t, `fn minus(x u32) u32 { return x-1 }
fn a(x u32) u32 { if x>10 { return minus(x) } return 0 }
fn b(x u32) u32 { if x==5 { return minus(x) } return 0 }
fn c(x u32) u32 { if x>=1 { return minus(x) } return 0 }
fn d(x u32) u32 { if x>0 { return minus(x) } return 0 }
fn e(x u32) u32 { if x>=0 { return minus(x) } return 0 }`)
	r, err := GenerateSpecialized(g)
	if err != nil || len(r.Variants) != 1 || r.Summary.Reused != 3 || r.Variants[0].ReuseCount != 3 {
		t.Fatalf("redundant variants: %v %+v", err, r.Summary)
	}
	for _, name := range []string{"a", "b", "c", "d"} {
		if d := selectedFor(t, r, name); d.FinalTarget != r.Variants[0].Name {
			t.Fatal(d)
		}
	}
	if d := selectedFor(t, r, "e"); d.Action != "FALLBACK" {
		t.Fatal(d)
	}
}

func TestPolicyCostRejectsLowBenefitAndIdentity(t *testing.T) {
	source := "fn expensive(x u32) u32 {"
	for i := 0; i < 30; i++ {
		source += fmt.Sprintf("let unused%d u32=1\n", i)
	}
	source += `return x-1 }
fn identity(x u32) u32 { return x }
fn a(x u32) u32 { if x>0 { return expensive(x) } return 0 }
fn b(x u32) u32 { return identity(x) }`
	r, err := GenerateSpecialized(graphFor(t, source))
	if err != nil || len(r.Variants) != 0 {
		t.Fatal("low benefit specialization created", err, r.Variants)
	}
	if d := selectedFor(t, r, "a"); d.Reason != "new_body_cost_exceeds_benefit" || d.Cost.RemovedGuards != 1 {
		t.Fatal(d)
	}
	if d := selectedFor(t, r, "b"); d.Reason != "no_proven_guard_benefit" {
		t.Fatal(d)
	}
}

func TestPolicyBudgetReusesWeakerApplicableVariant(t *testing.T) {
	source := `fn target(a u32,b u32,c u32,d u32,e u32) u32 {
let aa u32=10/a let bb u32=10/b let cc u32=10/c let dd u32=10/d let ee u32=10/e return 0 }
`
	for i := 0; i < 5; i++ {
		args := []string{"x", "x", "x", "x", "x"}
		for j := 0; j <= i; j++ {
			args[j] = "1"
		}
		source += fmt.Sprintf("fn caller%d(x u32) u32 {return target(%s)}\n", i, strings.Join(args, ","))
	}
	source += "fn unknown(x u32) u32 {return target(x,x,x,x,x)}"
	g := graphFor(t, source)
	r, err := GenerateSpecialized(g)
	if err != nil || len(r.Variants) != 4 || !r.Summary.BudgetExhausted {
		t.Fatal("budget not enforced", err, r.Summary)
	}
	if d := selectedFor(t, r, "caller4"); d.Action != "REUSE" || d.Reason != "budget_exhausted_reuse" || d.FinalTarget != r.Variants[3].Name {
		t.Fatal("budget failed to reuse best weaker legal contract", d)
	}
	if d := selectedFor(t, r, "unknown"); d.Action != "FALLBACK" || d.Selected != "" {
		t.Fatal("budget allowed unproven reuse", d)
	}
	canonical, _ := parser.NewCodeGenerator().Generate(g)
	h := fmt.Sprintf("\nint main(int argc,char**argv){if(argc!=2)return 2; uint32_t x=(uint32_t)strtoull(argv[1],NULL,10); %s(x); return (int)%s(x); }", g.Functions["caller4"].CName, g.Functions["unknown"].CName)
	for _, opt := range []string{"-O0", "-O3"} {
		a, b := buildC(t, canonical+h, opt), buildC(t, r.Code+h, opt)
		for _, x := range []string{"0", "1", "4294967295"} {
			if want, got := execute(t, a, x), execute(t, b, x); want != got {
				t.Fatal(want, got)
			}
		}
	}
}

func TestPolicyAdditionalCodeBudget(t *testing.T) {
	var source strings.Builder
	for i := 0; i < 50; i++ {
		fmt.Fprintf(&source, "fn target%02d(x u32) u32 {", i)
		for j := 0; j < 40; j++ {
			fmt.Fprintf(&source, "let p%d u32=1\n", j)
		}
		source.WriteString("let a u32=1/x let b u32=2/x let c u32=3/x let d u32=4/x return 0 }\n")
		fmt.Fprintf(&source, "fn caller%02d(x u32) u32 {if x>0 {return target%02d(x)} return 0}\n", i, i)
	}
	g := graphFor(t, source.String())
	r, err := GenerateSpecialized(g)
	if err != nil || !r.Summary.BudgetExhausted || r.Summary.AdditionalUnits > maxAdditionalUnits || len(r.Variants) >= 50 || len(r.Variants) == 0 {
		t.Fatal("additional code budget not enforced", err, r.Summary)
	}
	if d := selectedFor(t, r, "caller49"); d.Action != "FALLBACK" || d.Reason != "budget_exhausted" {
		t.Fatal(d)
	}
}

func TestPolicyRejectsForeignBodySemanticsAndBinding(t *testing.T) {
	g := graphFor(t, `fn f(x u32) u32 { return x-1 } fn other(x u32) u32 {return x}
fn caller(x u32) u32 {if x>1 {return f(x)} return 0}`)
	report, err := constraints.Analyze(g)
	if err != nil {
		t.Fatal(err)
	}
	functions := map[string]*constraints.FunctionReport{}
	for _, f := range report.Functions {
		functions[f.Name] = f
	}
	engine, err := constraints.NewImplicationEngine(report)
	if err != nil {
		t.Fatal(err)
	}
	o := functions["caller"].Obligations[0]
	c := newCandidate(g.Functions["f"])
	c.caller = "caller"
	c.instances[o.Origin.SemanticNode] = o.ID
	v := parser.SpecializedFunction{Function: c.f, Name: "test"}
	record := &Variant{Semantics: checkedUnsignedSemantics, Assumptions: []Requirement{{Operation: o.Origin.Node}}}
	b := &specializer{functions: functions, implication: engine, records: map[string]*Variant{"test": record}, proofCache: map[proofQuery]constraints.Proof{}}
	if got := b.applicable(c, v); got.Status != constraints.Proven {
		t.Fatal(got)
	}
	foreign := v
	foreign.Function = g.Functions["other"]
	if got := b.applicable(c, foreign); got.Status == constraints.Proven {
		t.Fatal("foreign body reused")
	}
	record.Semantics = "wrapping-integer-v0"
	if got := b.applicable(c, v); got.Status == constraints.Proven {
		t.Fatal("incompatible arithmetic semantics reused")
	}
	record.Semantics = checkedUnsignedSemantics
	delete(c.instances, o.Origin.SemanticNode)
	if got := b.applicable(c, v); got.Status == constraints.Proven {
		t.Fatal("unbound parameter contract reused")
	}
}

func policyHarness(g *parser.ProgramGraph) (string, []string) {
	names := []string{"a_first", "b_second", "c_strong", "d_equal", "e_weak", "g_contradictory", "h_effect", "i_short"}
	h := "\nint main(int argc,char**argv){if(argc!=3)return 2;uint32_t x=(uint32_t)strtoull(argv[2],NULL,10);uint32_t result=0;switch(atoi(argv[1])){"
	for i, name := range names {
		h += fmt.Sprintf("case %d:result=%s(x);break;", i, g.Functions[name].CName)
	}
	h += "default:return 2;}printf(\"%\" PRIu32 \"\\n\",result);return 0;}\n"
	return h, names
}

func TestPolicyNativeDifferential(t *testing.T) {
	g := graphFor(t, policyCases)
	canonical, _ := parser.NewCodeGenerator().Generate(g)
	r, err := GenerateSpecialized(g)
	if err != nil || len(r.Variants) != 2 {
		t.Fatal(err)
	}
	h, names := policyHarness(g)
	inputs := []uint64{0, 1, 2, 4, 5, 6, 9, 10, 11, 19, 20, 21, 4294967294, 4294967295}
	for _, opt := range []string{"-O0", "-O3"} {
		a, b := buildC(t, canonical+h, opt), buildC(t, r.Code+h, opt)
		for i, name := range names {
			for _, x := range inputs {
				args := []string{strconv.Itoa(i), strconv.FormatUint(x, 10)}
				want, got := execute(t, a, args...), execute(t, b, args...)
				if want != got {
					t.Fatalf("%s %s(%d): %+v != %+v", opt, name, x, want, got)
				}
				if name == "e_weak" && x < 10 && (want.status != 1 || !strings.Contains(want.err, "underflow")) {
					t.Fatal("unknown caller lost checked failure", want)
				}
				if name == "c_strong" && x >= 20 && (want.status != 0 || want.out != fmt.Sprint(x-10)+"\n") {
					t.Fatal("strong caller oracle", want)
				}
				if name == "h_effect" && x >= 20 && want.out != fmt.Sprint(x)+"\n"+fmt.Sprint(x-10)+"\n" {
					t.Fatal("effect trace", want)
				}
			}
		}
	}
}

func FuzzPolicyNativeEquivalence(f *testing.F) {
	g := graphFor(f, policyCases)
	canonical, _ := parser.NewCodeGenerator().Generate(g)
	r, err := GenerateSpecialized(g)
	if err != nil {
		f.Fatal(err)
	}
	h, names := policyHarness(g)
	a, b := buildC(f, canonical+h, "-O0"), buildC(f, r.Code+h, "-O0")
	for i := range names {
		for _, x := range []uint32{0, 1, 5, 9, 10, 20, 4294967295} {
			f.Add(byte(i), x)
		}
	}
	f.Fuzz(func(t *testing.T, which byte, x uint32) {
		args := []string{strconv.Itoa(int(which) % len(names)), strconv.FormatUint(uint64(x), 10)}
		want, got := execute(t, a, args...), execute(t, b, args...)
		if want != got {
			t.Fatal(args, want, got)
		}
	})
}
