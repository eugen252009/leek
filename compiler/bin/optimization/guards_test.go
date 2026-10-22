package optimization

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"leek/bin/constraints"
	"leek/bin/parser"
)

func graphFor(t testing.TB, source string) *parser.ProgramGraph {
	t.Helper()
	ast, err := parser.NewParser(strings.NewReader(source), "optimization.lk").ParseProgram()
	if err != nil {
		t.Fatal(err)
	}
	g, err := parser.NewResolver().Resolve(ast)
	if err != nil {
		t.Fatal(err)
	}
	return g
}

const proofCases = `fn sub(x u32) u32 { if x > 0 { return x-1 } return 0 }
fn mul(x u32) u32 { if x > 0 { return x*x } return 0 }
fn div(x u32) u32 { if x != 0 { return 100/x } return 0 }
fn short(x u32) u32 { if x != 0 && 100/x > 2 { return 1 } return 0 }
fn add(x u32) u32 { if x <= 100 { return x+5 } return 0 }
fn rem(x u32) u32 { if x != 0 { return 100%x } return 0 }
fn bounded(x u32) u32 { if x <= 100 { return x*x } return 0 }
fn minus1(x u32) u32 { return x-1 }
fn inverse(x u32) u32 { return 12/x }
fn pipeline(x u32) u32 { if x > 1 { return x -> minus1 -> inverse } return 0 }
fn direct(x u32) u32 { if x > 1 { return inverse(minus1(x)) } return 0 }
fn leak(x u32) u32 { if x > 1 { let y u32 = x-1 } return x-1 }
fn continuation(x u32) u32 { if x == 0 { return 0 } return x-1 }
fn disjunction(x u32) u32 { if x > 10 || x == 0 { return x-1 } return 0 }
fn failed(x u32) u32 { if x > 1 { return 1/0 } return x }
fn unreachable(x u32) u32 { if x > 10 && x < 5 { return 1/0 } return x }
fn join(x u32) u32 { if x > 1 { let a u32 = 1 } else { let b u32 = 1 } if x == 0 { return 1/0 } return x }
fn negative(x i8) i8 { if x != -128 { return -x } return x }
fn signed_div(x i8) i8 { if x != 0 { return x / -1 } return 0 }
impure fn effect(x u32) u32 { print(x) if x > 0 { return x-1 } return 0 }
`

// PG-GATE / PG-STRUCTURE: actual lowered statements, not just counters or runtime
// equivalence. A never-optimizing compiler MUST fail these assertions.
func TestProofGatedStructure(t *testing.T) {
	g := graphFor(t, proofCases)
	beforeDot := g.ToDOT()
	cg := parser.NewCodeGenerator()
	canonical, err := cg.Generate(g)
	if err != nil {
		t.Fatal(err)
	}
	optimized, elisions, err := Generate(g)
	if err != nil {
		t.Fatal(err)
	}
	report, err := constraints.Analyze(g)
	if err != nil {
		t.Fatal(err)
	}
	wantCounts := map[string]int{"sub": 1, "div": 1, "short": 1, "add": 1, "rem": 1, "bounded": 1, "leak": 1, "continuation": 1}
	counts := map[string]int{}
	for _, e := range elisions {
		counts[e.Function]++
		var f *constraints.FunctionReport
		for _, candidate := range report.Functions {
			if candidate.Name == e.Function {
				f = candidate
			}
		}
		if f == nil {
			t.Fatal("unknown function")
		}
		ob := f.Obligations[e.Obligation]
		if ob.Proof.Status != constraints.Proven || !ob.Proof.Consistent || ob.Environment != e.Environment || ob.Origin.Node != e.Node || len(ob.Origin.Via) != 0 {
			t.Fatalf("invalid elision authorization: %+v / %+v", e, ob)
		}
		checked := statementAfter(t, canonical, "// arithmetic "+e.Node+": checked")
		raw := statementAfter(t, optimized, "// arithmetic "+e.Node+": proven guard elided")
		if !strings.Contains(checked, "= lk_") || strings.Contains(raw, "= lk_") || !strings.Contains(raw, " "+ob.Origin.Operation+" ") {
			t.Fatalf("guard not actually removed: %s -> %s", checked, raw)
		}
	}
	if fmt.Sprint(counts) != fmt.Sprint(wantCounts) {
		t.Fatalf("elision counts: %v, want %v", counts, wantCounts)
	}
	for _, f := range report.Functions {
		for _, o := range f.Obligations {
			if len(o.Origin.Via) != 0 || o.Origin.SemanticNode == nil {
				continue
			}
			expr := o.Origin.SemanticNode.SourceNode.(*parser.Expr)
			if expr.Kind != "binary" {
				continue
			}
			shouldKeep := o.Proof.Status != constraints.Proven || !f.Pure || !f.Complete || !supported(g.Expressions[expr].Type, expr.Name)
			if shouldKeep {
				stmt := statementAfter(t, optimized, "// arithmetic "+o.Origin.Node+": checked")
				if !strings.Contains(stmt, "= lk_") {
					t.Fatalf("unproven/unsupported guard removed: %+v", o)
				}
			}
		}
	}
	after, err := cg.Generate(g)
	if err != nil {
		t.Fatal(err)
	}
	again, againElisions, err := Generate(g)
	if err != nil {
		t.Fatal(err)
	}
	if canonical != after || beforeDot != g.ToDOT() || optimized != again || fmt.Sprint(elisions) != fmt.Sprint(againElisions) {
		t.Fatal("canonical IR mutated or optimization nondeterministic")
	}
	if !strings.Contains(optimized, "lk_sub_u32(") {
		t.Fatal("generic callee guards must remain despite proven caller obligations")
	}
}
func statementAfter(t testing.TB, code, marker string) string {
	t.Helper()
	_, after, ok := strings.Cut(code, marker+"\n")
	if !ok {
		t.Fatalf("missing lowered operation %q", marker)
	}
	line, _, _ := strings.Cut(strings.TrimSpace(after), "\n")
	return line
}

type outcome struct {
	out, err string
	status   int
}

func buildC(t testing.TB, code, opt string) string {
	t.Helper()
	dir := t.TempDir()
	src := filepath.Join(dir, "program.c")
	binary := filepath.Join(dir, "program")
	if err := os.WriteFile(src, []byte(code), 0600); err != nil {
		t.Fatal(err)
	}
	args := []string{"-std=c11", "-Wall", "-Wextra", "-Werror", "-Wno-unused-parameter", opt, "-fsanitize=undefined", "-fno-sanitize-recover=all", src, "-o", binary}
	if out, err := exec.Command("gcc", args...).CombinedOutput(); err != nil {
		t.Fatalf("GCC: %v\n%s", err, out)
	}
	return binary
}
func execute(t testing.TB, binary string, args ...string) outcome {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, binary, args...)
	var out, errout bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errout
	err := cmd.Run()
	status := 0
	if err != nil {
		if e, ok := err.(*exec.ExitError); ok {
			status = e.ExitCode()
		} else {
			t.Fatal(err)
		}
	}
	if ctx.Err() != nil {
		t.Fatal(ctx.Err())
	}
	if strings.Contains(errout.String(), "runtime error:") {
		t.Fatalf("C undefined behavior: %s", errout.String())
	}
	return outcome{out.String(), errout.String(), status}
}
func TestCanonicalOptimizedDifferential(t *testing.T) {
	g := graphFor(t, proofCases)
	canonical, _ := parser.NewCodeGenerator().Generate(g)
	optimized, elisions, err := Generate(g)
	if err != nil {
		t.Fatal(err)
	}
	if len(elisions) == 0 {
		t.Fatal("optimization did not occur")
	}
	names := []string{"sub", "mul", "div", "short", "add", "rem", "bounded", "minus1", "inverse", "pipeline", "direct", "leak", "continuation", "disjunction", "failed", "unreachable", "join", "effect"}
	harness := "\nint main(int argc,char** argv) { if(argc!=3) return 2; uint32_t x=(uint32_t)strtoull(argv[2],NULL,10); uint32_t result=0; switch(atoi(argv[1])) {\n"
	for i, name := range names {
		harness += fmt.Sprintf("case %d: result=%s(x); break;\n", i, g.Functions[name].CName)
	}
	harness += "default:return 2; } printf(\"%\" PRIu32 \"\\n\",result);return 0;}\n"
	inputs := []uint64{0, 1, 2, 3, 9, 10, 11, 19, 20, 21, 49, 50, 51, 99, 100, 101, 65535, 65536, 4294967294, 4294967295}
	for _, opt := range []string{"-O0", "-O3"} {
		a := buildC(t, canonical+harness, opt)
		b := buildC(t, optimized+harness, opt)
		for i, name := range names {
			for _, x := range inputs {
				args := []string{strconv.Itoa(i), strconv.FormatUint(x, 10)}
				want := execute(t, a, args...)
				got := execute(t, b, args...)
				if got != want {
					t.Fatalf("%s %s(%d): canonical %+v optimized %+v", opt, name, x, want, got)
				}
				// Independent witnesses ensure that differential equality is not vacuous.
				if name == "sub" && x > 0 && (want.status != 0 || want.out != fmt.Sprint(x-1)+"\n") {
					t.Fatalf("sub oracle: %+v", want)
				}
				if name == "mul" && x == 65536 && (want.status != 1 || !strings.Contains(want.err, "overflow or underflow")) {
					t.Fatalf("multiplication failure erased: %+v", want)
				}
				if name == "disjunction" && x == 0 && (want.status != 1 || !strings.Contains(want.err, "overflow or underflow")) {
					t.Fatalf("disjunction assumptions leaked: %+v", want)
				}
			}
		}
	}
}
func TestDifferentialEffectsFailuresAndSkippedPaths(t *testing.T) {
	prelude := `fn safe(x u32) u32 { if x > 0 { return x-1 } return 0 }
 fn fail(x u32) u32 { return x-1 }
 impure fn mark(x u32) u32 { print(x) return x }
 fn add(a u32,b u32) u32 { return a+b }
 `
	for _, tt := range []struct {
		body, out, reason string
		status            int
	}{
		{`print(mark(3) -> safe) print(add(mark(1),mark(2)))`, "3\n2\n1\n2\n3\n", "", 0},
		{`print(mark(2) -> safe) print(mark(0) -> fail) print(99)`, "2\n1\n0\n", "overflow or underflow", 1},
		{`if false && (1/0 > 0) { print(99) } if true || mark(98)>0 { print(1) }`, "1\n", "", 0},
		{`if mark(0)!=0 && 100/mark(0)>2 { print(99) } print(7)`, "0\n7\n", "", 0},
		{`print(mark(1)) let x u32 = 1/0 let y u32 = 0-1 print(99)`, "1\n1\n", "division by zero", 1},
		{`print(mark(1)) let x u32 = 0-1 let y u32 = 1/0 print(99)`, "1\n1\n", "overflow or underflow", 1},
		{`let x i8 = -128 let y i8 = -1 print(x/y) print(99)`, "", "overflow or underflow", 1},
		{`let x i8 = -128 let y i8 = -1 print(x%y) print(99)`, "", "overflow or underflow", 1},
	} {
		g := graphFor(t, prelude+"impure fn main() {"+tt.body+"}")
		canonical, _ := parser.NewCodeGenerator().Generate(g)
		optimized, elisions, err := Generate(g)
		if err != nil {
			t.Fatal(err)
		}
		if len(elisions) != 1 {
			t.Fatalf("safe helper should be optimized: %v", elisions)
		}
		for _, opt := range []string{"-O0", "-O3"} {
			a := execute(t, buildC(t, canonical, opt))
			b := execute(t, buildC(t, optimized, opt))
			if a != b || a.status != tt.status || a.out != tt.out || !strings.Contains(a.err, tt.reason) {
				t.Fatalf("%s: %s: canonical %+v optimized %+v", opt, tt.body, a, b)
			}
		}
	}
}

// PG-NATIVE-BOUNDS: all input values, all supported widths; use native calls
// from a C harness to avoid recompiling once per input. u16 multiplication is
// deliberately included to exercise C integer promotion with UBSan enabled.
func TestNativeBoundedUnsignedOperations(t *testing.T) {
	for _, typ := range []string{"u8", "u16", "u32", "u64"} {
		source := fmt.Sprintf(`fn f(x %s) %s {
   if x <= 10 { let a %s = x+5 let b %s = a*x if x != 0 { return b/x + b%%x } return a }
   return x-1
  }`, typ, typ, typ, typ)
		g := graphFor(t, source)
		a, _ := parser.NewCodeGenerator().Generate(g)
		b, removed, err := Generate(g)
		if err != nil || len(removed) < 5 {
			t.Fatalf("%s: %v / %v", typ, removed, err)
		}
		harness := fmt.Sprintf("\nint main(void) { for(unsigned x=0;x<256;x++) printf(\"%%\" PRIu64 \"\\n\", (uint64_t)%s(x)); return 0; }", g.Functions["f"].CName)
		for _, opt := range []string{"-O0", "-O3"} {
			ca := execute(t, buildC(t, a+harness, opt))
			cb := execute(t, buildC(t, b+harness, opt))
			// Every execution is safe here, including promoted u8/u16 arithmetic.
			if ca != cb || ca.status != 0 {
				t.Fatalf("%s %s: %+v != %+v", typ, opt, ca, cb)
			}
		}
	}
}
func TestOptimizationRejectsUnvalidatedGraph(t *testing.T) {
	if _, _, err := Generate(parser.NewProgramGraph()); err == nil {
		t.Fatal("accepted unvalidated graph")
	}
	if _, _, err := Generate(nil); err == nil {
		t.Fatal("accepted nil graph")
	}
}
func TestIncompleteAnalysisKeepsChecks(t *testing.T) {
	g := graphFor(t, `fn f(x f64,y u32) u32 { let a bool = x == x if y > 0 { return y-1 } return 0 }`)
	code, elisions, err := Generate(g)
	if err != nil || len(elisions) != 0 || strings.Contains(code, ": proven guard elided") {
		t.Fatalf("partial analysis allowed optimization: %v %v", elisions, err)
	}
}

// PG-LIMIT: even independently proven sites are kept if analysis hits a budget.
func TestResourceLimitedAnalysisKeepsChecks(t *testing.T) {
	var source strings.Builder
	source.WriteString("fn f(x u32) u32 {\n")
	for i := 0; i < 4098; i++ {
		fmt.Fprintf(&source, "let v%d u32 = 1+1\n", i)
	}
	source.WriteString("return x }")
	g := graphFor(t, source.String())
	report, err := constraints.Analyze(g)
	if err != nil {
		t.Fatal(err)
	}
	if !report.Limited {
		t.Fatal("fixture failed to exhaust obligation budget")
	}
	code, elisions, err := Generate(g)
	if err != nil || len(elisions) != 0 || strings.Contains(code, ": proven guard elided") {
		t.Fatalf("resource-limited analysis allowed check removal: %v %v", elisions, err)
	}
}
func FuzzOptimizationPreservesIR(f *testing.F) {
	f.Add(dataflowSource("filter(even) -> map(square)", "reduce(initial,add)"))
	f.Add(proofCases)
	f.Add(`fn f(x u8) u8 { if x > 0 { return x-1 } return 0 }`)
	f.Fuzz(func(t *testing.T, source string) {
		if len(source) > 4096 {
			return
		}
		ast, err := parser.NewParser(strings.NewReader(source), "fuzz.lk").ParseProgram()
		if err != nil {
			return
		}
		g, err := parser.NewResolver().Resolve(ast)
		if err != nil {
			return
		}
		before, err := parser.NewCodeGenerator().Generate(g)
		if err != nil {
			t.Fatal(err)
		}
		dot := g.ToDOT()
		_, elisions, err := Generate(g)
		if err != nil {
			t.Fatal(err)
		}
		after, err := parser.NewCodeGenerator().Generate(g)
		if err != nil {
			t.Fatal(err)
		}
		if before != after || dot != g.ToDOT() {
			t.Fatal("optimizer changed canonical semantics")
		}
		report, err := constraints.Analyze(g)
		if err != nil {
			t.Fatal(err)
		}
		for _, e := range elisions {
			for _, fn := range report.Functions {
				if fn.Name == e.Function && fn.Obligations[e.Obligation].Proof.Status != constraints.Proven {
					t.Fatal("elided unproven operation")
				}
			}
		}
	})
}
