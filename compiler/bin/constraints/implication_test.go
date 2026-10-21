package constraints

import (
	"encoding/json"
	"math/big"
	"testing"
)

func implicationFixture(t testing.TB, a *arena, contexts ...context) (*ImplicationEngine, *FunctionReport) {
	t.Helper()
	f := &FunctionReport{Name: "query"}
	for _, term := range a.nodes {
		if term.Op == "var" {
			f.InputFacts = append(f.InputFacts, Fact{Value: term.ID, Type: "u8"})
		}
	}
	s := &functionState{report: f}
	for _, c := range contexts {
		s.snapshot(c, yes)
	}
	e, err := NewImplicationEngine(&Report{Terms: a.nodes, Functions: []*FunctionReport{f}})
	if err != nil {
		t.Fatal(err)
	}
	return e, f
}

func TestConstraintImplicationDirectionAndNormalization(t *testing.T) {
	a := newArena()
	x := a.raw("var", "x")
	y := a.raw("var", "y")
	base := newContext()
	base.put(x, span{big.NewInt(0), big.NewInt(255)})
	base.put(y, span{big.NewInt(0), big.NewInt(255)})
	ge1, ge5, gt0, gt10, eq5, ge0 := a.op(">=", x, a.num(1)), a.op(">=", x, a.num(5)), a.op(">", x, a.num(0)), a.op(">", x, a.num(10)), a.op("==", x, a.num(5)), a.op(">=", x, a.num(0))
	conjunction := a.op("and", ge1, ge5)
	eq7 := a.op("==", x, a.num(7))
	redundant := a.op("and", eq7, gt0)
	ne0 := a.op("!=", x, a.num(0))
	unrelated := a.op(">=", y, a.num(1))
	arithmetic := a.op("<=", a.op("*", x, x), a.num(255))
	e, f := implicationFixture(t, a, base, a.refine(base, gt0), a.refine(base, ge1), a.refine(base, conjunction), a.refine(base, ge5), a.refine(base, eq7), a.refine(base, redundant), a.refine(base, arithmetic))
	for _, tt := range []struct {
		left, right               ID
		forward, reverse          Status
		equivalent, contradictory bool
	}{
		{gt10, ge1, Proven, Unknown, false, false}, {eq5, gt0, Proven, Unknown, false, false},
		{ge5, ge1, Proven, Unknown, false, false}, {gt0, ge1, Proven, Proven, true, false},
		{ge0, ge1, Unknown, Proven, false, false}, {conjunction, ge5, Proven, Proven, true, false},
		{redundant, eq7, Proven, Proven, true, false}, {ne0, ge1, Proven, Proven, true, false},
		{ge1, unrelated, Unknown, Unknown, false, false}, {eq7, a.op("==", x, a.num(0)), Violated, Violated, false, true},
	} {
		// New terms must be in the same query arena, so rebuild the fixture after
		// constructing predicates added by the table's final case.
		query, _ := implicationFixture(t, a, base)
		got, err := query.Compare(f.Name, 0, tt.left, tt.right)
		if err != nil || got.Forward.Status != tt.forward || got.Reverse.Status != tt.reverse || got.Equivalent != tt.equivalent || got.Contradictory != tt.contradictory {
			t.Fatalf("%s => %s: %+v %v", a.text(tt.left), a.text(tt.right), got, err)
		}
	}
	for _, pair := range [][2]int{{1, 2}, {3, 4}, {5, 6}} {
		left, err := e.Implies(f.Name, pair[0], yes)
		if err != nil {
			t.Fatal(err)
		}
		right, err := e.Implies(f.Name, pair[1], yes)
		if err != nil {
			t.Fatal(err)
		}
		l, _ := json.Marshal(left.Premises)
		r, _ := json.Marshal(right.Premises)
		if string(l) != string(r) {
			t.Fatalf("equivalent integer assumptions not canonical:\n%s\n%s", l, r)
		}
	}
	residual, err := e.Implies(f.Name, 7, arithmetic)
	if err != nil || residual.Proof.Status != Proven || len(residual.Premises.Residual) == 0 {
		t.Fatal("arithmetic/definedness information erased", residual, err)
	}
}

func TestImplicationBoundariesAndNoVacuousProof(t *testing.T) {
	a := newArena()
	x := a.raw("var", "x")
	base := newContext()
	base.put(x, span{big.NewInt(0), big.NewInt(255)})
	ge256 := a.op(">=", x, a.num(256))
	lt0 := a.op("<", x, a.num(0))
	ge1 := a.op(">=", x, a.num(1))
	ne1 := a.op("!=", x, a.num(1))
	ge2 := a.op(">=", x, a.num(2))
	c := a.refine(a.refine(base, ne1), ge1)
	e, f := implicationFixture(t, a, base, a.refine(base, ge256), a.refine(base, lt0), c)
	for _, i := range []int{1, 2} {
		got, err := e.Implies(f.Name, i, ge1)
		if err != nil || got.Proof.Status != Unknown || got.Proof.Consistent {
			t.Fatal("inconsistent premises produced a vacuous proof", got, err)
		}
	}
	got, err := e.Implies(f.Name, 3, ge2)
	if err != nil || got.Proof.Status != Proven || got.Premises.Domains[0].Min != "2" {
		t.Fatal(got, err)
	}
	if _, err := e.Implies(f.Name, 100, ge1); err == nil {
		t.Fatal("unknown environment accepted")
	}
	if _, err := e.Implies(f.Name, 0, ID(1000000)); err == nil {
		t.Fatal("foreign term accepted")
	}
}

func TestImplicationWideIntegerBounds(t *testing.T) {
	for _, typ := range []string{"u8", "u16", "u32", "u64", "i8", "i16", "i32", "i64"} {
		r := reportFor(t, "fn need(x "+typ+") "+typ+" {return x-1} fn caller(x "+typ+") "+typ+" {if x>0 {return need(x)} return 0}")
		engine, err := NewImplicationEngine(r)
		if err != nil {
			t.Fatal(err)
		}
		f := function(t, r, "caller")
		for _, o := range f.Obligations {
			got, err := engine.Implies(f.Name, o.Environment, o.Required)
			if err != nil || got.Proof.Status != Proven || !got.Proof.Consistent {
				t.Fatalf("%s: %+v %v", typ, got, err)
			}
			if len(got.Premises.Domains) != 1 || got.Premises.Domains[0].Type != typ || got.Premises.Domains[0].Min != "1" {
				t.Fatal("integer type or strict bound lost", typ, got)
			}
		}
	}
}

// Exhaust all u8 values independently of interval implication. PROVEN must have
// no satisfying-premise counterexample; VIOLATED must have no satisfying result.
func TestImplicationIndependentU8Oracle(t *testing.T) {
	comparisons := []string{"==", "!=", "<", "<=", ">", ">="}
	count := 0
	for _, op := range comparisons {
		for _, bound := range []int64{0, 1, 2, 10, 254, 255, 256} {
			a := newArena()
			x := a.raw("var", "x")
			base := newContext()
			base.put(x, span{big.NewInt(0), big.NewInt(255)})
			premise := a.op(op, x, a.num(bound))
			for _, requiredOp := range comparisons {
				for _, requiredBound := range []int64{0, 1, 2, 10, 254, 255, 256} {
					required := a.op(requiredOp, x, a.num(requiredBound))
					e, f := implicationFixture(t, a, base)
					got, err := e.Compare(f.Name, 0, premise, required)
					if err != nil {
						t.Fatal(err)
					}
					satisfying, success := 0, 0
					for n := int64(0); n < 256; n++ {
						if compare(op, big.NewInt(n).Cmp(big.NewInt(bound))) {
							satisfying++
							if compare(requiredOp, big.NewInt(n).Cmp(big.NewInt(requiredBound))) {
								success++
							}
						}
						count++
					}
					if (got.Forward.Status == Proven && (satisfying == 0 || success != satisfying)) || (got.Forward.Status == Violated && (satisfying == 0 || success != 0)) {
						t.Fatalf("unsound %s %d => %s %d: %+v (%d/%d)", op, bound, requiredOp, requiredBound, got, satisfying, success)
					}
				}
			}
		}
	}
	t.Logf("%d independent u8 implication valuations", count)
}

func FuzzIntegerImplication(f *testing.F) {
	f.Add(byte(4), byte(10), byte(5), byte(1))
	f.Fuzz(func(t *testing.T, op, bound, requiredOp, requiredBound byte) {
		ops := []string{"==", "!=", "<", "<=", ">", ">="}
		a := newArena()
		x := a.raw("var", "x")
		c := newContext()
		c.put(x, span{big.NewInt(0), big.NewInt(255)})
		p, q := a.op(ops[int(op)%6], x, a.num(int64(bound))), a.op(ops[int(requiredOp)%6], x, a.num(int64(requiredBound)))
		e, fn := implicationFixture(t, a, c)
		result, err := e.Compare(fn.Name, 0, p, q)
		if err != nil {
			t.Fatal(err)
		}
		seen, yesCount := 0, 0
		for n := 0; n < 256; n++ {
			if compare(ops[int(op)%6], n-int(bound)) {
				seen++
				if compare(ops[int(requiredOp)%6], n-int(requiredBound)) {
					yesCount++
				}
			}
		}
		if result.Forward.Status == Proven && (seen == 0 || seen != yesCount) || result.Forward.Status == Violated && (seen == 0 || yesCount != 0) {
			t.Fatal("implication direction or integer normalization is unsound", result)
		}
	})
}
