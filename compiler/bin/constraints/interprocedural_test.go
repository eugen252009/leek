package constraints

import (
	"fmt"
	"testing"
)

func TestReusableSummaryAndInstantiatedCalls(t *testing.T) {
	r := reportFor(t, `fn subtract(x u32, amount u32) u32 { return x-amount }
fn direct(a u32) u32 { if a >= 10 { return subtract(a,10) } return 0 }
fn pipe(a u32) u32 { if a >= 10 { return a -> subtract(10) } return 0 }
fn unknown(a u32) u32 { return subtract(a,10) }
fn reversed(a u32) u32 { if a > 10 { return subtract(10,a) } return 0 }
fn broken() u32 { return subtract(0,1) }`)
	f := function(t, r, "subtract")
	if !f.Pure || !f.Complete || len(f.InputFacts) != 2 || f.InputFacts[0].Range.Min != "0" || f.InputFacts[0].Range.Max != "4294967295" || f.PreconditionText != "(x >= amount)" || f.Proof.Status != Unknown || f.Output == nil || len(f.Obligations) != 1 {
		t.Fatalf("invalid reusable contract: %+v", f)
	}
	a := &arena{nodes: r.Terms}
	if got := a.text(f.Output.Relation); got != "(subtract:x - subtract:amount)" {
		t.Fatal(got)
	}
	for name, want := range map[string]Status{"direct": Proven, "pipe": Proven, "unknown": Unknown, "reversed": Violated, "broken": Violated} {
		caller := function(t, r, name)
		if len(caller.Calls) != 1 {
			t.Fatal(caller.Calls)
		}
		c := caller.Calls[0]
		if c.Proof.Status != want || !c.Proof.Consistent || !c.Complete || !c.Pure || len(c.Arguments) != 2 || c.Environment >= len(caller.Environments) {
			t.Fatalf("%s: %+v", name, c)
		}
		if len(caller.Obligations) != 1 || caller.Obligations[0].Proof.Status != want || len(caller.Obligations[0].Origin.Via) != 1 {
			t.Fatal(caller.Obligations)
		}
	}
}

func TestInterproceduralProofIndependent8BitOracle(t *testing.T) {
	inputs := [][]int64{}
	for x := 0; x < 256; x++ {
		inputs = append(inputs, []int64{int64(x)})
	}
	count := 0
	for _, cmp := range []string{">", ">=", "<", "<=", "==", "!="} {
		for _, bound := range []int{0, 1, 2, 10, 254, 255} {
			for _, body := range []string{
				"return subtract(x,10)", "return x -> subtract(10)",
				"return subtract(10,x)", "return x -> minus -> inverse",
				"return nested(x)", "return choose(x)",
			} {
				source := fmt.Sprintf(`fn subtract(x u8,a u8) u8 { return x-a }
fn minus(x u8) u8 { return x-1 }
fn inverse(x u8) u8 { return 12/x }
fn nested(x u8) u8 { return inverse(minus(x)) }
fn choose(x u8) u8 { if x > 10 { return x-10 } return x+255 }
fn f(x u8) u8 { if x %s %d { %s } return 0 }`, cmp, bound, body)
				checkPathDomain(t, source, inputs)
				count += len(inputs)
			}
		}
	}
	t.Logf("%d interprocedural input/program valuations", count)
}

func TestIncompleteCallCannotReportProvenSuccess(t *testing.T) {
	r := reportFor(t, `fn f(x f64) f64 { return x } fn g(x f64) f64 { return f(x) }
impure fn mark(x u32) u32 { print(x) return x }
impure fn effect(x u32) u32 { return mark(x) }
impure fn skipped(x u32) u32 { if false && mark(x)>0 { return 1 } return 0 }`)
	for _, name := range []string{"g", "effect", "skipped"} {
		c := function(t, r, name).Calls[0]
		if c.Complete || c.Proof.Status != Unknown {
			t.Fatal(c)
		}
		if c.Proof.Consistent != function(t, r, name).Environments[c.Environment].Reachable {
			t.Fatal("call proof does not match its exact premise snapshot", c)
		}
	}
}
