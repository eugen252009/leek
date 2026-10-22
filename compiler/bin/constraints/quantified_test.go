package constraints

import (
	"fmt"
	"math/big"
	"strings"
	"testing"

	"leek/bin/parser"
)

const quantPrelude = `fn square(x u32) u32 { return x*x }
fn add(a u32,b u32) u32 { return a+b }
fn small(x u32) bool { return x<10 }
`

func stages(t testing.TB, f *FunctionReport) []DataflowInfo {
	t.Helper()
	if len(f.Dataflow) == 0 {
		t.Fatal("missing dataflow")
	}
	return f.Dataflow
}
func TestQuantifiedMotivatingBounds(t *testing.T) {
	r := reportFor(t, quantPrelude+`fn f(n u32) u32 { if n<=100 { return range(0,n) -> map(square) -> reduce(0,add) } return 0 }`)
	f := function(t, r, "f")
	d := stages(t, f)
	domain := d[0].Quantified.Domain
	if domain.State != MaybeEmptyDomain || domain.Count.Min != "0" || domain.Count.Max != "100" || domain.Elements.Min != "0" || domain.Elements.Max != "99" || !domain.EndExclusive {
		t.Fatalf("domain %+v", domain)
	}
	mapped := d[1].Quantified
	if mapped.Elements.Min != "0" || mapped.Elements.Max != "9801" || mapped.Proof.Status != Proven {
		t.Fatalf("map %+v", mapped)
	}
	prefix := d[2].Quantified.Prefix
	if prefix == nil || prefix.Initial.Min != "0" || prefix.AllPrefixes.Max != "980100" || prefix.PreviousState.Max != "970299" || prefix.Proof.Status != Proven {
		t.Fatalf("prefix %+v", prefix)
	}
	if len(f.QuantifiedObligations) != 2 || d[2].Quantified.PipelineProof.Status != Proven {
		t.Fatal("missing universal arithmetic proofs")
	}
	for _, o := range f.QuantifiedObligations {
		env := f.Environments[o.Environment]
		if o.Proof.Status != Proven || env.Domain != domain.Node || env.Stage != o.Origin.Via[0] || !env.Reachable {
			t.Fatalf("bad provenance %+v %+v", o, env)
		}
	}
	// Quantified environments are not scalar inputs/summaries or premises for an
	// unrelated call. Scalar sequence-result relations remain conservatively opaque.
	if f.Complete || f.Proof.Status != Unknown || function(t, r, "square").Proof.Status != Unknown {
		t.Fatal("range facts promoted to global scalar guarantees")
	}
}
func TestQuantifiedDomainEdges(t *testing.T) {
	for _, tt := range []struct{ start, end, count, state, lo, hi string }{
		{"0", "0", "0", "EMPTY", "", ""}, {"5", "2", "0", "EMPTY", "", ""},
		{"0", "1", "1", "NONEMPTY", "0", "0"}, {"10", "20", "10", "NONEMPTY", "10", "19"},
		{"4294967294", "4294967295", "1", "NONEMPTY", "4294967294", "4294967294"},
	} {
		f := function(t, reportFor(t, quantPrelude+fmt.Sprintf("fn f() u32 { return range(%s,%s) -> map(square) -> reduce(7,add) }", tt.start, tt.end)), "f")
		q := f.Dataflow[0].Quantified
		if q.Count.Min != tt.count || q.Count.Max != tt.count || string(q.State) != tt.state {
			t.Fatalf("%s..%s: %+v", tt.start, tt.end, q)
		}
		if q.State == EmptyDomain {
			for _, d := range f.Dataflow {
				if d.Quantified.Elements != nil || d.Quantified.Environment != -1 {
					t.Fatal("empty domain invented an element")
				}
			}
			if len(f.QuantifiedObligations) != 0 {
				t.Fatal("vacuous domain created optimization witnesses")
			}
		} else if q.Elements.Min != tt.lo || q.Elements.Max != tt.hi {
			t.Fatalf("elements %+v", q.Elements)
		}
	}
	for _, typ := range []string{"u8", "u16", "u32", "u64", "i8", "i16", "i32", "i64"} {
		bounds, _ := integerBounds(typ)
		source := fmt.Sprintf("fn id(x %s) %s { return x } fn last(a %s,b %s) %s { return b } fn f() %s { let lo %s = %s let hi %s = %s return range(lo,hi) -> map(id) -> reduce(0,last) }", typ, typ, typ, typ, typ, typ, typ, bounds.lo, typ, bounds.hi)
		f := function(t, reportFor(t, source), "f")
		q := f.Dataflow[0].Quantified
		want := new(big.Int).Sub(bounds.hi, bounds.lo).String()
		if q.Count.Max != want || q.Elements.Max != new(big.Int).Sub(bounds.hi, big.NewInt(1)).String() {
			t.Fatalf("%s: %+v", typ, q)
		}
	}
	general := function(t, reportFor(t, `fn id(x u8) u8 { return x } fn last(a u8,b u8) u8 { return b } fn f(a u8,b u8) u8 { if a<=10 && b>=20 && b<=25 { return range(a,b) -> map(id) -> reduce(0,last) } return 0 }`), "f")
	qg := general.Dataflow[0].Quantified
	if qg.State != NonemptyDomain || qg.Count.Min != "10" || qg.Count.Max != "25" || qg.Elements.Min != "0" || qg.Elements.Max != "24" {
		t.Fatalf("general endpoint bounds %+v", qg)
	}
	for a := 0; a <= 10; a++ {
		for b := 20; b <= 25; b++ {
			for x := a; x < b; x++ {
				contained(t, qg.Elements, x, "general endpoint enclosure")
			}
		}
	}
	negative := function(t, reportFor(t, `fn id(x i8) i8 { return x } fn last(a i8,b i8) i8 { return b } fn f() i8 { let lo i8 = -128 let hi i8 = -127 return range(lo,hi) -> map(id) -> reduce(0,last) }`), "f")
	if q := negative.Dataflow[0].Quantified; q.Elements.Min != "-128" || q.Elements.Max != "-128" || q.Count.Max != "1" {
		t.Fatalf("negative exclusive endpoint lost sign: %+v", q)
	}
	f := function(t, reportFor(t, quantPrelude+`fn f(n u32) u32 { return range(n,n) -> map(square) -> reduce(0,add) }`), "f")
	if f.Dataflow[0].Quantified.State != EmptyDomain {
		t.Fatal("identical symbolic endpoints not empty")
	}
}
func TestQuantifiedPartialFiltersAndScope(t *testing.T) {
	for _, tt := range []struct {
		body                    string
		mapStatus, reduceStatus Status
		upper                   string
	}{
		{`return range(0,n) -> map(square) -> reduce(0,add)`, Unknown, Unknown, ""},
		{`if n<=65536 { return range(0,n) -> map(square) -> reduce(0,add) } return 0`, Proven, Unknown, ""},
		{`if n<=100 { return range(0,n) -> map(square) -> reduce(4294967295,add) } return 0`, Proven, Unknown, ""},
		{`if n<=100 { return range(0,n) -> filter(small) -> map(square) -> reduce(100,add) } return 0`, Proven, Proven, "8200"},
	} {
		f := function(t, reportFor(t, quantPrelude+"fn f(n u32) u32 {"+tt.body+"}"), "f")
		d := f.Dataflow
		var mapped, reduced *QuantifiedInfo
		for _, stage := range d {
			if stage.Kind == parser.Map {
				mapped = stage.Quantified
			}
			if stage.Kind == parser.Reduce {
				reduced = stage.Quantified
			}
		}
		if mapped.Proof.Status != tt.mapStatus || reduced.Proof.Status != tt.reduceStatus {
			t.Fatalf("%s: map %+v reduce %+v", tt.body, mapped, reduced)
		}
		if tt.upper != "" && reduced.Prefix.AllPrefixes.Max != tt.upper {
			t.Fatalf("filter/prefix %+v", reduced)
		}
	}
	source := quantPrelude + `fn f(n u32) u32 { if n<=100 { let first u32 = range(0,n) -> map(square) -> reduce(0,add) } return range(0,n) -> map(square) -> reduce(0,add) }`
	f := function(t, reportFor(t, source), "f")
	if f.Dataflow[1].Quantified.Proof.Status != Proven || f.Dataflow[4].Quantified.Proof.Status != Unknown || f.InputFacts[0].Range.Max != "4294967295" {
		t.Fatal("path/domain facts leaked")
	}
}
func TestQuantifiedFoldAndCircularity(t *testing.T) {
	for _, callback := range []string{`fn step(a u32,b u32) u32 { return a*2+b }`, `fn step(a u32,b u32) u32 { return a-b }`} {
		f := function(t, reportFor(t, callback+`fn f() u32 { return range(0,5) -> fold(0,step) }`), "f")
		q := f.Dataflow[1].Quantified
		if q.Prefix != nil || q.Proof.Status != Unknown {
			t.Fatalf("unsupported fold recurrence %+v", q)
		}
	}
	// A mathematically overflowing map cannot prove its own safety by clamping
	// to the output type. A downstream modulo may still supply a safe small range.
	source := `fn huge(x u32) u32 { return x+4294967295 } fn small(x u32) u32 { return x%4 } fn add(a u32,b u32) u32 { return a+b } fn f() u32 { return range(0,2) -> map(huge) -> map(small) -> reduce(0,add) }`
	f := function(t, reportFor(t, source), "f")
	if f.Dataflow[1].Quantified.Proof.Status != Unknown || f.Dataflow[2].Quantified.Elements.Max != "3" || f.Dataflow[3].Quantified.Proof.Status != Proven {
		t.Fatal("circular output assumption or lost partial proof")
	}
	// A combine with unused potentially failing work is not assumed total merely
	// because its output relation is addition.
	source = quantPrelude + `fn bad(a u32,b u32) u32 { let ignored u32 = b-1 return a+b } fn f() u32 { return range(0,5) -> map(square) -> reduce(0,bad) }`
	f = function(t, reportFor(t, source), "f")
	if f.Dataflow[2].Quantified.Proof.Status != Unknown {
		t.Fatal("ignored reduction obligations")
	}
}
func TestQuantifiedIncompleteEndpointIdentity(t *testing.T) {
	source := quantPrelude + `fn size(n u32) u32 { return range(0,n) -> reduce(0,add) } fn f() u32 { return range(size(1),size(3)) -> map(square) -> reduce(0,add) }`
	f := function(t, reportFor(t, source), "f")
	domain := f.Dataflow[0].Quantified.Domain
	if domain.Start == domain.End || domain.State == EmptyDomain || integer(domain.Count.Max).Cmp(big.NewInt(3)) < 0 {
		t.Fatal("unrelated opaque function results were equated as range endpoints")
	}
}
func TestQuantifiedSummaryTransfer(t *testing.T) {
	for _, body := range []string{`return x+1`, `if x>0 { return x-1 } return 0`, `return 100/(x+1)`, `return x%7`, `return x -> double`, `if x<10 { return x } return 10`} {
		source := fmt.Sprintf(`fn double(x u32) u32 { return x*2 } fn transform(x u32) u32 { %s } fn add(a u32,b u32) u32 { return a+b } fn f(n u32) u32 { if n<=20 { return range(0,n) -> map(transform) -> reduce(0,add) } return 0 }`, body)
		f := function(t, reportFor(t, source), "f")
		if f.Dataflow[1].Quantified.Proof.Status != Proven || f.Dataflow[1].Quantified.Elements == nil || f.Dataflow[2].Quantified.Proof.Status != Proven {
			t.Fatalf("%s: %+v", body, f.Dataflow)
		}
	}
}
func TestQuantifiedDeterministicReadOnly(t *testing.T) {
	g := graphFor(t, quantPrelude+`fn f(n u32) u32 { if n<=100 { return range(0,n) -> map(square) -> reduce(0,add) } return 0 }`)
	before, _ := parser.NewCodeGenerator().Generate(g)
	dot := g.ToDOT()
	r, err := Analyze(g)
	if err != nil {
		t.Fatal(err)
	}
	after, _ := parser.NewCodeGenerator().Generate(g)
	if before != after || dot != g.ToDOT() {
		t.Fatal("analysis mutated IR")
	}
	engine, err := NewImplicationEngine(r)
	if err != nil {
		t.Fatal(err)
	}
	f := function(t, r, "f")
	for _, o := range f.QuantifiedObligations {
		p, err := engine.Implies("f", o.Environment, o.Required)
		if err != nil || p.Proof.Status != Proven {
			t.Fatalf("quantified/scalar implication bridge: %+v %v", p, err)
		}
	}
	scope := function(t, reportFor(t, quantPrelude+`fn f(n u32) u32 { if n<=100 { let first u32 = range(0,n) -> map(square) -> reduce(0,add) let after u32 = n-1 } return n-1 }`), "f")
	for _, o := range scope.Obligations {
		if o.Origin.Operation == "-" {
			for _, premise := range scope.Environments[o.Environment].Ranges {
				for _, stage := range scope.Dataflow {
					if premise.Value == stage.Quantified.Value {
						t.Fatal("quantified element facts escaped into a scalar environment")
					}
				}
			}
		}
	}
	if !strings.Contains(f.Limitations[0], "quantified") {
		t.Fatal("scalar summary limitation disappeared")
	}
}
