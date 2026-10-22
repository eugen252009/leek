package constraints

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math/big"
	"strings"
	"testing"

	"leek/bin/parser"
)

func graphFor(t testing.TB, source string) *parser.ProgramGraph {
	t.Helper()
	ast, err := parser.NewParser(strings.NewReader(source), "constraint.lk").ParseProgram()
	if err != nil {
		t.Fatal(err)
	}
	g, err := parser.NewResolver().Resolve(ast)
	if err != nil {
		t.Fatal(err)
	}
	return g
}
func reportFor(t testing.TB, source string) *Report {
	t.Helper()
	r, err := Analyze(graphFor(t, source))
	if err != nil {
		t.Fatal(err)
	}
	return r
}
func function(t testing.TB, r *Report, name string) *FunctionReport {
	t.Helper()
	for _, f := range r.Functions {
		if f.Name == name {
			return f
		}
	}
	t.Fatalf("function %s missing", name)
	return nil
}

const compositionSource = `fn minus1(x u32) u32 { return x - 1 }
fn divide_12_by(x u32) u32 { return 12 / x }
fn calculate(x u32) u32 { return x -> minus1 -> divide_12_by }
fn longer(x u32) u32 { return x -> minus1 -> minus1 -> divide_12_by }
fn direct(x u32) u32 { return divide_12_by(minus1(x)) }
fn safe() u32 { return 2 -> calculate }
fn zero() u32 { return 0 -> calculate }
fn one() u32 { return 1 -> calculate }`

// CA-TYPE: type facts are independently true, not arithmetic obligations.
func TestTypeFacts(t *testing.T) {
	r := reportFor(t, "fn id(x u32) u32 { return x }")
	f := function(t, r, "id")
	fact := f.InputFacts[0]
	if fact.Range.Min != "0" || fact.Range.Max != "4294967295" || fact.Scope != "function input" {
		t.Fatalf("u32 domain: %+v", fact)
	}
	if f.Proof.Status != Proven || len(f.Obligations) != 0 || f.Precondition != yes {
		t.Fatalf("identity: %+v", f)
	}
	for _, typ := range []string{"u8", "u16", "u32", "u64", "i8", "i16", "i32", "i64"} {
		t.Run(typ, func(t *testing.T) {
			var bits uint
			fmt.Sscanf(typ[1:], "%d", &bits)
			hi := new(big.Int).Lsh(big.NewInt(1), bits)
			lo := new(big.Int)
			if typ[0] == 'i' {
				hi.Rsh(hi, 1)
				lo.Neg(new(big.Int).Set(hi))
			}
			hi.Sub(hi, big.NewInt(1))
			f := function(t, reportFor(t, fmt.Sprintf("fn id(x %s) %s { return x }", typ, typ)), "id")
			if f.InputFacts[0].Range.Min != lo.String() || f.InputFacts[0].Range.Max != hi.String() {
				t.Fatalf("bad %s bounds: %+v", typ, f.InputFacts)
			}
		})
	}
}

// CA-COMPOSE: every intermediate definedness requirement survives substitution.
func TestCompositionPreconditions(t *testing.T) {
	r := reportFor(t, compositionSource)
	for _, tt := range []struct {
		name, text string
		status     Status
	}{
		{"minus1", "(x >= 1)", Unknown}, {"divide_12_by", "(x != 0)", Unknown},
		{"calculate", "(x > 1)", Unknown}, {"direct", "(x > 1)", Unknown},
		{"longer", "(x > 2)", Unknown}, {"safe", "true", Proven}, {"zero", "false", Violated}, {"one", "false", Violated},
	} {
		f := function(t, r, tt.name)
		if f.PreconditionText != tt.text || f.Proof.Status != tt.status || !f.Complete {
			t.Errorf("%s: %s %+v complete=%v", tt.name, f.PreconditionText, f.Proof, f.Complete)
		}
	}
	f := function(t, r, "calculate")
	if len(f.Obligations) != 2 {
		t.Fatalf("lost intermediate check: %+v", f.Obligations)
	}
	for _, o := range f.Obligations {
		if o.Proof.Status != Unknown || len(o.Origin.Via) != 1 || o.Origin.Location.File != "constraint.lk" {
			t.Fatalf("unjustified proof or lost provenance: %+v", o)
		}
	}
	safe := function(t, r, "safe")
	if safe.Output.Range.Min != "12" || safe.Output.Range.Max != "12" {
		t.Fatalf("constant output: %+v", safe.Output)
	}
	if !function(t, r, "one").Output.Range.Empty {
		t.Fatal("invented output for failing call")
	}
}
func TestAdditionalArgumentsAndBindings(t *testing.T) {
	r := reportFor(t, `fn subtract(a u32,b u32) u32 { return a-b }
fn divide(a u32,b u32) u32 { return a/b }
fn f(x u32) u32 { let y u32 = x -> subtract(1) return divide(12,y) }
fn g(x u32) u32 { let unused u32 = 1 / x return 5 }`)
	if f := function(t, r, "f"); f.PreconditionText != "(x > 1)" {
		t.Fatal(f.PreconditionText)
	}
	if g := function(t, r, "g"); g.PreconditionText != "(x != 0)" {
		t.Fatalf("discarded unused failure: %s", g.PreconditionText)
	}
}

// CA-ARITH: constraints follow checked language widths, not Go/C wraparound.
func TestArithmeticObligations(t *testing.T) {
	for _, op := range []string{"+", "-", "*", "/", "%"} {
		t.Run(op, func(t *testing.T) {
			r := reportFor(t, fmt.Sprintf("fn f(a u32,b u32) u32 { return a %s b }", op))
			f := function(t, r, "f")
			if len(f.Obligations) != 1 || f.Proof.Status != Unknown {
				t.Fatalf("missing/assumed obligation: %+v", f)
			}
			a := &arena{nodes: r.Terms}
			condition := a.text(f.Obligations[0].Required)
			expected := map[string]string{"+": "((f:a + f:b) <= 4294967295)", "-": "(f:a >= f:b)", "*": "((f:a * f:b) <= 4294967295)", "/": "(f:b != 0)", "%": "(f:b != 0)"}[op]
			if condition != expected {
				t.Fatalf("got %s want %s", condition, expected)
			}
		})
	}
	r := reportFor(t, `fn f(a u32,b u32) u32 { return (a+b)*2 }`)
	if f := function(t, r, "f"); len(f.Obligations) != 2 || f.Proof.Status != Unknown {
		t.Fatalf("lost earlier overflow: %+v", f)
	}
}
func TestConstantSafetyAndBoundaries(t *testing.T) {
	for _, tt := range []struct {
		typ, expr string
		status    Status
	}{
		{"u32", "0 + 0", Proven}, {"u32", "4294967295 + 0", Proven}, {"u32", "4294967295 + 1", Violated},
		{"u32", "0 - 1", Violated}, {"u32", "1 - 1", Proven}, {"u32", "1 / 0", Violated}, {"u32", "1 % 0", Violated},
		{"u64", "18446744073709551615 * 18446744073709551615", Violated},
		{"i64", "-9223372036854775808 / -1", Violated}, {"i64", "-9223372036854775808 % -1", Violated},
		{"i8", "127 + 1", Violated}, {"i8", "-128 - 1", Violated}, {"i8", "-7 / 3", Proven}, {"i8", "-7 % 3", Proven},
	} {
		t.Run(tt.typ+tt.expr, func(t *testing.T) {
			f := function(t, reportFor(t, fmt.Sprintf("fn f() %s { return %s }", tt.typ, tt.expr)), "f")
			if f.Proof.Status != tt.status {
				t.Fatalf("%s: %s %+v", tt.expr, f.PreconditionText, f.Proof)
			}
		})
	}
	r := reportFor(t, `fn f(a i64,b i64) i64 { return a/b } fn neg(a i8) i8 { return -a }`)
	if len(function(t, r, "f").Obligations) != 2 {
		t.Fatal("lost signed min/-1 exception")
	}
	if n := function(t, r, "neg"); n.Proof.Status != Unknown {
		t.Fatalf("signed negation wrongly total: %+v", n)
	}
}

// CA-FACTS: output refinements are conditional; they are not circular proofs.
func TestConditionalRanges(t *testing.T) {
	r := reportFor(t, `fn add_five(x u32) u32 { return x+5 }
fn f(x u32) u32 { return (x+1)-1 }
fn bad() u32 { let x u32 = 0-1 return 12 }`)
	f := function(t, r, "add_five")
	if f.PreconditionText != "(x <= 4294967290)" || f.Proof.Status != Unknown {
		t.Fatal(f.PreconditionText, f.Proof)
	}
	if f.Output.Range.Min != "5" || f.Output.Range.Max != "4294967295" || !strings.Contains(f.Output.Scope, "successfully") {
		t.Fatalf("conditional facts: %+v", f.Output)
	}
	if f := function(t, r, "f"); f.Proof.Status != Unknown || f.PreconditionText != "(x <= 4294967294)" {
		t.Fatalf("algebra erased overflow: %s %+v", f.PreconditionText, f.Proof)
	}
	if f := function(t, r, "bad"); f.Proof.Status != Violated || !f.Output.Range.Empty {
		t.Fatalf("output exists only after failure: %+v", f)
	}
}
func TestContradictorySuccessDomainAndFalseResult(t *testing.T) {
	r := reportFor(t, `fn impossible(x u32) u32 { let first u32 = x-1 return 0-x }
fn predicate() bool { return false }`)
	if f := function(t, r, "impossible"); f.Proof.Status != Violated || f.Precondition != no || !f.Output.Range.Empty {
		t.Fatalf("contradictory requirements: %+v", f)
	}
	if f := function(t, r, "predicate"); f.Proof.Status != Proven || f.Precondition != yes || f.Output.Range.Min != "0" {
		t.Fatalf("false result confused with failure: %+v", f)
	}
}

func TestShortCircuitGuards(t *testing.T) {
	r := reportFor(t, `fn fail() bool { let x u32 = 1/0 return true }
fn skipped() bool { return false && fail() }
fn also_skipped() bool { return true || fail() }
fn executed() bool { return true && fail() }
fn conditional(b bool) bool { return b && fail() }
fn guarded(x u32) bool { return x != 0 && 12/x > 1 }`)
	for _, name := range []string{"skipped", "also_skipped", "guarded"} {
		if f := function(t, r, name); f.Proof.Status != Proven {
			t.Fatalf("%s: %s %+v", name, f.PreconditionText, f.Proof)
		}
	}
	if f := function(t, r, "executed"); f.Proof.Status != Violated {
		t.Fatal(f.Proof)
	}
	if f := function(t, r, "conditional"); f.Proof.Status != Unknown {
		t.Fatalf("unknown branch was assumed: %+v", f)
	}
}
func TestUnknownAndEffectBoundaries(t *testing.T) {
	r := reportFor(t, `impure fn input() u32 { return 7 }
fn subtract(a u32,b u32) u32 { return a-b }
impure fn f() u32 { return input() -> subtract(input()) }
fn square(x u32) u32 { return x*x }
fn floats(x f64,y f64) bool { return x==y }`)
	f := function(t, r, "f")
	if f.Complete || f.Proof.Status != Unknown || len(f.AuxiliaryFacts) != 2 {
		t.Fatalf("effects inlined or proven: %+v", f)
	}
	if f.AuxiliaryFacts[0].Value == f.AuxiliaryFacts[1].Value {
		t.Fatal("distinct effectful calls conflated")
	}
	last := f.Obligations[len(f.Obligations)-1]
	if last.Proof.Status != Unknown {
		t.Fatal("independent sources treated as identical")
	}
	if f := function(t, r, "square"); f.Proof.Status != Unknown {
		t.Fatal("nonlinear range was assumed safe")
	}
	if f := function(t, r, "floats"); f.Complete || f.Proof.Status != Unknown {
		t.Fatal("floating point equality treated as exact integers")
	}
}

func TestTermBudgetCannotFabricateSuccessOrFailure(t *testing.T) {
	g := graphFor(t, "fn f(x u32) u32 { return (x+1)/(x-1) }")
	for cap := 3; cap < 40; cap++ {
		a := newArena()
		a.limit = cap
		b := &analyzer{g: g, a: a, summaries: map[*parser.FunctionInfo]*FunctionReport{}, params: map[*parser.FunctionInfo][]ID{}, visiting: map[*parser.FunctionInfo]bool{}}
		f, err := b.function(g.Functions["f"])
		if err != nil {
			t.Fatal(err)
		}
		if f.Proof.Status != Unknown || (a.limited && f.Complete) {
			t.Fatalf("limit %d: %+v complete=%v", cap, f.Proof, f.Complete)
		}
		for _, o := range f.Obligations {
			if o.Proof.Status != Proven && o.Proof.Status != Unknown && o.Proof.Status != Violated {
				t.Fatalf("limit %d: unfinished obligation %+v", cap, o)
			}
		}
	}
}

func TestObligationBudgetIsExplicitUnknown(t *testing.T) {
	var source strings.Builder
	source.WriteString("fn f() u32 {\n")
	for i := 0; i < maxObligations+2; i++ {
		fmt.Fprintf(&source, "let x%d u32 = 1 / 1\n", i)
	}
	source.WriteString("return 1 }")
	r := reportFor(t, source.String())
	f := function(t, r, "f")
	if !r.Limited || f.Complete || f.Proof.Status != Unknown {
		t.Fatalf("budget exhaustion became a proof: complete=%v limited=%v proof=%+v", f.Complete, r.Limited, f.Proof)
	}
}

// CA-INSPECT and CA-PRESERVE: deterministic JSON; no mutation of the executable IR.
func FuzzAnalysisReadOnly(f *testing.F) {
	for _, source := range []string{compositionSource, dfSource,
		"fn f(x u32) u32 { return (x+1)-(x+1) }",
		"fn f(x i64) i64 { return x / -1 }",
		"fn f(x u32) bool { return x != 0 && 12/x > 1 }",
		"impure fn main() { print(1/0) }",
	} {
		f.Add(source)
	}
	f.Fuzz(func(t *testing.T, source string) {
		if len(source) > 4096 {
			t.Skip()
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
		r, err := Analyze(g)
		if err != nil {
			t.Fatal(err)
		}
		for i, term := range r.Terms {
			if term.ID != ID(i) {
				t.Fatal("unstable term ID")
			}
			for _, arg := range term.Args {
				if arg < 0 || int(arg) >= i {
					t.Fatal("symbolic DAG has dangling or cyclic references")
				}
			}
		}
		after, err := parser.NewCodeGenerator().Generate(g)
		if err != nil {
			t.Fatal(err)
		}
		if before != after {
			t.Fatal("analysis changed generated C")
		}
	})
}

func TestAnalysisIsReadOnlyAndDeterministic(t *testing.T) {
	g := graphFor(t, compositionSource)
	cg := parser.NewCodeGenerator()
	before, err := cg.Generate(g)
	if err != nil {
		t.Fatal(err)
	}
	dot := g.ToDOT()
	first, err := Analyze(g)
	if err != nil {
		t.Fatal(err)
	}
	second, err := Analyze(g)
	if err != nil {
		t.Fatal(err)
	}
	x, _ := json.Marshal(first)
	y, _ := json.Marshal(second)
	if !bytes.Equal(x, y) {
		t.Fatal("analysis report is nondeterministic")
	}
	after, err := cg.Generate(g)
	if err != nil {
		t.Fatal(err)
	}
	if before != after || dot != g.ToDOT() {
		t.Fatal("analysis mutated execution IR")
	}
	if !first.AnalysisOnly || first.RuntimeChecks != "preserved" {
		t.Fatal("missing inspection safety boundary")
	}
	if _, err := Analyze(parser.NewProgramGraph()); err == nil {
		t.Fatal("accepted unvalidated graph")
	}
}
