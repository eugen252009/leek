package optimization

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"testing"

	"leek/bin/constraints"
	"leek/bin/parser"
)

const quantifiedCallbacks = `fn square(x u32) u32 { return x*x }
fn add(a u32,b u32) u32 { return a+b }
fn positive(x u32) bool { return x>0 }
fn inverse(x u32) u32 { return 100/x }
`

func nativeBody(t testing.TB, code, name string) string {
	t.Helper()
	match := regexp.MustCompile(`(?m)^(?:static )?\w+ ` + name + `\([^;{}]*\) \{`).FindStringIndex(code)
	if match == nil {
		t.Fatalf("missing definition %s", name)
	}
	body, _, _ := strings.Cut(code[match[1]:], "\n}")
	return body
}
func TestQuantifiedGuardEliminationStructure(t *testing.T) {
	source := quantifiedCallbacks + `fn f(n u32) u32 { if n<=100 { return range(0,n) -> map(square) -> reduce(0,add) } return 0 }`
	g := graphFor(t, source)
	canonical, _ := parser.NewCodeGenerator().Generate(g)
	dot := g.ToDOT()
	result, err := GenerateSpecialized(g)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Quantified) != 2 || len(result.Variants) != 2 {
		t.Fatalf("no real quantified specialization: %+v %+v", result.Quantified, result.Variants)
	}
	total := 0
	for _, decision := range result.Quantified {
		total += decision.GuardsEliminated
		if decision.Function != "f" || decision.Domain == "" || decision.GuardsEliminated != 1 || decision.Bounds.Proof.Status != constraints.Proven {
			t.Fatalf("invalid decision %+v", decision)
		}
		op, helper := "*", "lk_mul_u32("
		if decision.Callback == "add" {
			op, helper = "+", "lk_add_u32("
		}
		original := nativeBody(t, canonical, g.Functions[decision.Callback].CName)
		generic := nativeBody(t, result.Code, g.Functions[decision.Callback].CName)
		specialized := nativeBody(t, result.Code, decision.Target)
		if !strings.Contains(original, helper) || !strings.Contains(generic, helper) || strings.Contains(specialized, helper) || !strings.Contains(specialized, " "+op+" ") {
			t.Fatalf("guard not genuinely removed only from variant: %s", specialized)
		}
		if !strings.Contains(nativeBody(t, result.Code, g.Functions["f"].CName), decision.Target+"(") {
			t.Fatal("dataflow does not dispatch to the proven variant")
		}
	}
	if total != 2 {
		t.Fatal("wrong eliminated guard count")
	}
	after, _ := parser.NewCodeGenerator().Generate(g)
	again, err := GenerateSpecialized(g)
	if err != nil {
		t.Fatal(err)
	}
	a, _ := json.Marshal(result)
	b, _ := json.Marshal(again)
	if after != canonical || dot != g.ToDOT() || result.Code != again.Code || string(a) != string(b) {
		t.Fatal("canonical mutation or nondeterministic proof/routing")
	}
}
func TestQuantifiedPartialReuseAndScope(t *testing.T) {
	source := quantifiedCallbacks + `fn a_bounded(n u32) u32 { if n<=100 { return range(0,n) -> map(square) -> reduce(0,add) } return 0 }
 fn b_stronger(n u32) u32 { if n<=20 { return range(0,n) -> map(square) -> reduce(0,add) } return 0 }
 fn c_map_only(n u32) u32 { if n<=65536 { return range(0,n) -> map(square) -> reduce(0,add) } return 0 }
 fn d_unbounded(n u32) u32 { return range(0,n) -> map(square) -> reduce(0,add) }
 fn e_scoped(n u32) u32 { if n<=100 { let first u32 = range(0,n) -> map(square) -> reduce(0,add) } return range(0,n) -> map(square) -> reduce(0,add) }
 fn scalar(x u32) u32 { if x<=99 { return square(x) } return 0 }`
	g := graphFor(t, source)
	result, err := GenerateSpecialized(g)
	if err != nil {
		t.Fatal(err)
	}
	bounded := map[string]string{}
	scoped := []int{}
	for _, d := range result.Quantified {
		switch d.Function {
		case "a_bounded":
			if d.GuardsEliminated != 1 {
				t.Fatal(d)
			}
			bounded[d.Callback] = d.Target
		case "b_stronger":
			if d.GuardsEliminated != 1 || d.Target != bounded[d.Callback] {
				t.Fatalf("registry reuse missing: %+v", d)
			}
		case "c_map_only":
			want := 0
			if d.Callback == "square" {
				want = 1
			}
			if d.GuardsEliminated != want {
				t.Fatalf("partial proof not respected %+v", d)
			}
		case "d_unbounded":
			if d.GuardsEliminated != 0 || d.Target != g.Functions[d.Callback].CName {
				t.Fatal("bounded proof leaked to unrelated range")
			}
		case "e_scoped":
			scoped = append(scoped, d.GuardsEliminated)
		}
	}
	if fmt.Sprint(scoped) != "[1 1 0 0]" || result.Summary.Reused < 2 || result.Summary.Generated != 2 {
		t.Fatalf("scope/registry regression: %v %+v", scoped, result.Summary)
	}
	// Empty domains must not manufacture a callable variant from vacuous proofs.
	g = graphFor(t, quantifiedCallbacks+`fn f() u32 { return range(0,0) -> map(square) -> reduce(7,add) }`)
	result, err = GenerateSpecialized(g)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Variants) != 0 {
		t.Fatal("empty domain created a specialization")
	}
}
func TestQuantifiedCostPolicyFallback(t *testing.T) {
	var body strings.Builder
	for i := 0; i < 80; i++ {
		fmt.Fprintf(&body, "let v%d u32 = x\n", i)
	}
	source := quantifiedCallbacks + "fn expensive(x u32) u32 {" + body.String() + " return x*x } fn f(n u32) u32 { if n<=100 { return range(0,n) -> map(expensive) -> reduce(0,add) } return 0 }"
	g := graphFor(t, source)
	result, err := GenerateSpecialized(g)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, d := range result.Quantified {
		if d.Callback == "expensive" {
			found = true
			if d.Bounds.Proof.Status != constraints.Proven || d.GuardsEliminated != 0 || d.Target != g.Functions["expensive"].CName {
				t.Fatal("quantified candidates bypassed cost eligibility")
			}
		}
	}
	if !found {
		t.Fatal("missing cost-fallback diagnostic")
	}
}
func TestQuantifiedCanonicalPullRoutes(t *testing.T) {
	// Nine stages exceed fusion's bound, but per-invocation proofs are still valid
	// for canonical pull execution. This must not change the iteration strategy.
	source := quantifiedCallbacks + `fn id(x u32) u32 { return x }
 fn f(n u32) u32 { if n<=10 { return range(0,n) -> map(square) -> map(id) -> map(id) -> map(id) -> map(id) -> map(id) -> map(id) -> reduce(0,add) } return 0 }`
	g := graphFor(t, source)
	result, err := GenerateSpecialized(g)
	if err != nil {
		t.Fatal(err)
	}
	if result.Dataflow[0].Decision != "CANONICAL" || !strings.Contains(result.Code, "_next(") {
		t.Fatal("quantified guards changed the fusion strategy")
	}
	found := 0
	for _, d := range result.Quantified {
		if d.GuardsEliminated > 0 {
			found++
			if !strings.Contains(result.Code, d.Target+"(") {
				t.Fatal("pull callback routing missing")
			}
		}
	}
	if found != 2 {
		t.Fatalf("pull path did not specialize map/reduce: %d", found)
	}
	compareDataflow(t, source, [][]string{{"0"}, {"1"}, {"2"}, {"10"}, {"11"}}, "-O0")
}
func TestQuantifiedNativeReferenceDifferential(t *testing.T) {
	inputs := [][]string{{"0"}, {"1"}, {"2"}, {"99"}, {"100"}, {"101"}, {"2344"}, {"2345"}, {"65536"}, {"65537"}, {"4294967295"}}
	for _, body := range []string{
		`if n<=100 { return range(0,n) -> map(square) -> reduce(0,add) } return 0`,
		`if n<=65536 { return range(0,n) -> map(square) -> reduce(0,add) } return 0`,
		`if n<=100 { return range(0,n) -> map(square) -> reduce(4294967295,add) } return 0`,
		`if n<=100 { return range(0,n) -> filter(positive) -> map(inverse) -> reduce(7,add) } return 0`,
		`if n<=100 { let ignored u32 = range(0,n) -> map(square) -> reduce(0,add) } if n<=100 { return range(0,n) -> map(inverse) -> reduce(0,add) } return 0`,
		`if n<=100 { return range(10,n) -> map(square) -> reduce(1,add) } return 0`,
	} {
		source := quantifiedCallbacks + "fn f(n u32) u32 {" + body + "}"
		for _, opt := range []string{"-O0", "-O3"} {
			compareDataflow(t, source, inputs, opt)
		}
	}
	opaqueEndpoints := quantifiedCallbacks + `fn size(n u32) u32 { return range(0,n) -> reduce(0,add) } fn f() u32 { return range(size(1),size(3)) -> map(square) -> reduce(0,add) }`
	compareDataflow(t, opaqueEndpoints, [][]string{{}}, "-O0")
	clamped := quantifiedCallbacks + `fn cap(n u32) u32 { if n>100 { return 100 } return n } fn f(n u32) u32 { return range(0,cap(n)) -> map(square) -> reduce(0,add) }`
	compareDataflow(t, clamped, inputs, "-O0")
	effects := quantifiedCallbacks + `impure fn f(n u32) u32 { print(77) if n<=100 { let value u32 = range(0,n) -> map(square) -> reduce(0,add) print(88) return value+(1/0) } return 0 }`
	effectsPlan, err := GenerateSpecialized(graphFor(t, effects))
	if err != nil {
		t.Fatal(err)
	}
	removed := 0
	for _, d := range effectsPlan.Quantified {
		removed += d.GuardsEliminated
	}
	if removed != 2 {
		t.Fatal("effectful surrounding scope accidentally disabled valid pure callback proofs")
	}
	for _, opt := range []string{"-O0", "-O3"} {
		compareDataflow(t, effects, inputs, opt)
	}
	// Computationally bounded high-valued domains actually fail, rather than
	// attempting to enumerate an enormous range just to exercise a threshold.
	source := quantifiedCallbacks + `fn f(a u32,b u32) u32 { return range(a,b) -> map(square) -> reduce(0,add) }`
	for _, opt := range []string{"-O0", "-O3"} {
		compareDataflow(t, source, [][]string{{"65535", "65537"}, {"4294967294", "4294967295"}, {"0", "0"}}, opt)
	}
	source = quantifiedCallbacks + `fn bad(a u32,b u32) u32 { let ignored u32 = b-1 return a+b }
 impure fn mark(x u32) u32 { print(x) return x }
 impure fn f(n u32) u32 { print(77) if n<=100 { return range(mark(0),mark(n)) -> map(square) -> reduce(mark(0),bad) } return 0 }`
	for _, opt := range []string{"-O0", "-O3"} {
		compareDataflow(t, source, inputs, opt)
	}
}
