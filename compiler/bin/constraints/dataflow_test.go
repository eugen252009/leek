package constraints

import (
	"leek/bin/parser"
	"strings"
	"testing"
)

const dfSource = `fn square(x u32) u32 { return x*x }
 fn even(x u32) bool { return x%2==0 }
 fn add(a u32,b u32) u32 { return a+b }
 fn f(n u32) u32 { return range(0,n) -> filter(even) -> map(square) -> reduce(7,add) }
 fn caller(n u32) u32 { return f(n) }`

func TestDataflowConstraints(t *testing.T) {
	g := graphFor(t, dfSource)
	before, _ := parser.NewCodeGenerator().Generate(g)
	dot := g.ToDOT()
	r, err := Analyze(g)
	if err != nil {
		t.Fatal(err)
	}
	f := function(t, r, "f")
	if f.Complete || f.Proof.Status != Unknown || len(f.Dataflow) != 4 {
		t.Fatalf("%+v", f)
	}
	count := 0
	for _, d := range f.Dataflow {
		wantStage := Unknown
		if d.Kind == parser.Range || d.Kind == parser.Filter {
			wantStage = Proven
		}
		if d.Proof.Status != wantStage || d.Order != parser.ElementMajor || d.ElementRange == nil {
			t.Fatalf("%+v", d)
		}
		if d.Kind == parser.Range && (d.Start == nil || d.End == nil || len(d.DomainFacts) != 2) {
			t.Fatal("range facts lost")
		}
		if d.Kind == parser.Filter && d.Predicate == nil {
			t.Fatal("predicate lost")
		}
		if d.Kind == parser.Reduce && (d.Initial == nil || r.Terms[*d.Initial].Value != "7" || d.Accumulator == nil || *d.Initial == *d.Accumulator || d.AccumulatorRange == nil) {
			t.Fatal("initial incorrectly assumed to be every accumulator")
		}
		for _, o := range d.Requirements {
			count++
			want := Unknown
			if d.Kind == parser.Filter {
				want = Proven
			} // constant nonzero divisor, now universally discharged
			if o.Proof.Status != want || o.Environment < 0 || f.Environments[o.Environment].Domain != d.Quantified.Domain.Node || len(o.Origin.Via) == 0 || o.Origin.SemanticNode == nil {
				t.Fatalf("invalid quantified requirement: %+v", o)
			}
		}
	}
	if count != 3 {
		t.Fatalf("lost checked callback obligations: %d", count)
	}
	if function(t, r, "caller").Complete || function(t, r, "caller").Proof.Status != Unknown {
		t.Fatal("sequence wrapper fabricated proof")
	}
	after, _ := parser.NewCodeGenerator().Generate(g)
	if before != after || dot != g.ToDOT() {
		t.Fatal("analysis changed canonical execution")
	}
}
func TestDataflowSetupObligations(t *testing.T) {
	r := reportFor(t, strings.Replace(dfSource, "range(0,n)", "range(1/0,n)", 1))
	f := function(t, r, "f")
	found := false
	for _, o := range f.Obligations {
		if o.Origin.Operation == "/" {
			found = true
			if o.Proof.Status != Violated {
				t.Fatalf("lost setup failure: %+v", o)
			}
		}
	}
	if !found || f.Proof.Status != Violated {
		t.Fatal("setup failure lost")
	}
}
