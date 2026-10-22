package optimization

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"leek/bin/constraints"
	"leek/bin/parser"
)

const dataflowCallbacks = `fn id(x u32) u32 { return x }
fn square(x u32) u32 { return x*x }
fn sub(x u32) u32 { return x-1 }
fn div(x u32) u32 { return 100/x }
fn late(x u32) u32 { if x==2 { return 0-1 } return x }
fn add(a u32,b u32) u32 { return a+b }
fn step(state u32,x u32) u32 { return state*2+x }
fn even(x u32) bool { return x%2==0 }
fn keep(x u32) bool { return true }
fn drop(x u32) bool { return false }
fn fail_pred(x u32) bool { return 1/(x-1)>0 }
`

func dataflowSource(stages, terminal string) string {
	return dataflowCallbacks + `fn f(a u32,b u32,initial u32) u32 { return range(a,b) -> ` + stages + ` -> ` + terminal + ` }`
}

// DF-FUSION: a never-fusing compiler fails even if all runtime comparisons pass.
func TestFusionStructure(t *testing.T) {
	g := graphFor(t, dataflowSource("map(square)", "reduce(initial,add)"))
	canonical, _ := parser.NewCodeGenerator().Generate(g)
	dot := g.ToDOT()
	result, err := GenerateSpecialized(g)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Dataflow) != 1 {
		t.Fatal(result.Dataflow)
	}
	d := result.Dataflow[0]
	if d.Function != "f" || d.Decision != "FUSED" || d.Legality.Status != constraints.Proven || d.IteratorStates || d.IntermediateMaterialization || fmt.Sprint(d.Dataflow) != "[Range Map Reduce]" {
		t.Fatalf("%+v", d)
	}
	if !strings.Contains(canonical, "canonical pull") || !strings.Contains(canonical, "_next(") || strings.Contains(canonical, "for (") {
		t.Fatal("missing independent canonical iterator chain")
	}
	definition := regexp.MustCompile(`(?m)^uint32_t ` + g.Functions["f"].CName + `\([^;{}]*\) \{`).FindStringIndex(result.Code)
	if definition == nil {
		t.Fatal("missing native body")
	}
	body, _, _ := strings.Cut(result.Code[definition[1]:], "\n}")
	if !strings.Contains(result.Code, "fused element-major loop") || strings.Count(result.Code, "for (") != 1 || strings.Contains(result.Code, "lk_iter_") || strings.Contains(body, "[") || strings.Contains(result.Code, "malloc(") {
		t.Fatal("fusion did not remove pull machinery or introduced materialization")
	}
	// Callback arithmetic stays checked unless the separate scalar proof pass
	// independently proves it. Square/add are both UNKNOWN for arbitrary inputs.
	for _, helper := range []string{"= lk_mul_u32(", "= lk_add_u32("} {
		if !strings.Contains(result.Code, helper) {
			t.Fatalf("callback arithmetic guard lost: %s", helper)
		}
	}
	after, _ := parser.NewCodeGenerator().Generate(g)
	second, err := GenerateSpecialized(g)
	if err != nil {
		t.Fatal(err)
	}
	a, _ := json.Marshal(result)
	b, _ := json.Marshal(second)
	if canonical != after || dot != g.ToDOT() || result.Code != second.Code || string(a) != string(b) {
		t.Fatal("fusion mutated canonical graph or nondeterministic diagnostics")
	}
	if result.Analysis.Functions == nil {
		t.Fatal("analysis not retained")
	}
}
func TestFilterLoopFusionStructure(t *testing.T) {
	g := graphFor(t, dataflowSource("filter(even) -> map(square)", "reduce(initial,add)"))
	canonical, _ := parser.NewCodeGenerator().Generate(g)
	result, err := GenerateSpecialized(g)
	if err != nil {
		t.Fatal(err)
	}
	// Canonical filter producer and terminal consumer have separate pull loops.
	// The fused strategy combines both into one direct source-domain loop.
	if strings.Count(canonical, "while (") != 2 || strings.Count(result.Code, "for (") != 1 || strings.Contains(result.Code, "while (") || strings.Contains(result.Code, "lk_iter_") || result.Dataflow[0].Decision != "FUSED" {
		t.Fatal("producer/consumer loops not actually fused")
	}
}
func TestFusionCanonicalFallback(t *testing.T) {
	for _, source := range []string{
		dataflowSource("map(id)", "fold(initial,step)"),
		dataflowSource(strings.Repeat("map(id) -> ", 8)+"map(id)", "reduce(initial,add)"),
		dataflowCallbacks + `fn f(a u32,b u32,initial u32) u32 { return range(a,b) -> reduce(initial,add) }`,
	} {
		g := graphFor(t, source)
		result, err := GenerateSpecialized(g)
		if err != nil {
			t.Fatal(err)
		}
		if len(result.Dataflow) != 1 || result.Dataflow[0].Decision != "CANONICAL" || result.Dataflow[0].Legality.Status != constraints.Unknown || !result.Dataflow[0].IteratorStates || !strings.Contains(result.Code, "canonical pull") || strings.Contains(result.Code, "fused element-major loop") {
			t.Fatalf("unsupported pattern not canonical: %+v", result.Dataflow)
		}
	}
}
func TestFusionLegalityFailsClosed(t *testing.T) {
	g := graphFor(t, dataflowSource("map(square)", "reduce(initial,add)"))
	e := parser.DataflowTerminals(g)[0]
	chain := parser.Pipeline(g, e)
	d := g.Expressions[chain[1]].Dataflow
	d.Order = "stage-major"
	if FusionLegality(g, e).Status != constraints.Unknown {
		t.Fatal("accepted incompatible failure order")
	}
	d.Order = parser.ElementMajor
	d.Callback.Effects = parser.EffectIO
	if FusionLegality(g, e).Status != constraints.Unknown {
		t.Fatal("accepted effectful callback")
	}
	d.Callback.Effects = parser.EffectNone
	d.Materialization = "required"
	if FusionLegality(g, e).Status != constraints.Unknown {
		t.Fatal("accepted incompatible materialization contract")
	}
	d.Materialization = parser.NoMaterialization
	limited := &constraints.Report{Limited: true}
	plan, decisions := fusionPlan(g, limited)
	if len(plan) != 0 || decisions[0].Decision != "CANONICAL" {
		t.Fatal("resource limit ignored")
	}
	if FusionLegality(nil, e).Status != constraints.Unknown {
		t.Fatal("accepted missing graph")
	}
}

// DF-ORACLE: compare independent AST reference, canonical pull, and fused native
// execution, including function-entry traces and exact first failure locations.
func TestDataflowReferenceDifferential(t *testing.T) {
	inputs := [][]string{}
	for a := 0; a <= 5; a++ {
		for b := 0; b <= 5; b++ {
			for _, initial := range []string{"0", "1", "4294967295"} {
				inputs = append(inputs, []string{strconv.Itoa(a), strconv.Itoa(b), initial})
			}
		}
	}
	inputs = append(inputs, []string{"0", "100", "0"}, []string{"65535", "65537", "0"}, []string{"4294967294", "4294967295", "0"}, []string{"4294967295", "4294967295", "7"})
	patterns := []string{"map(id)", "map(square)", "map(sub)", "map(div)", "map(late) -> map(div)", "map(square) -> map(div)", "filter(even) -> map(square)", "map(sub) -> filter(even) -> map(div)", "filter(drop) -> map(div)", "filter(keep) -> map(id)", "filter(fail_pred) -> map(id)", "filter(even) -> map(late)"}
	success, failure := 0, 0
	for _, pattern := range patterns {
		for _, opt := range []string{"-O0", "-O3"} {
			t.Run(pattern+opt, func(t *testing.T) {
				a, b := compareDataflow(t, dataflowSource(pattern, "reduce(initial,add)"), inputs, opt)
				success += a
				failure += b
			})
		}
	}
	for _, pattern := range []string{"map(id)", "filter(even) -> map(id)"} {
		a, b := compareDataflow(t, dataflowSource(pattern, "fold(initial,step)"), inputs[:108], "-O0")
		success += a
		failure += b
	}
	t.Logf("%d successful and %d failing three-way native input comparisons", success, failure)
}
func TestDataflowEffectsAndDifferentFoldState(t *testing.T) {
	source := dataflowCallbacks + `impure fn mark(x u32) u32 { print(x) return x }
 impure fn f(a u32,b u32,initial u32) u32 { print(77) let result u32 = range(mark(a),mark(b)) -> filter(even) -> map(square) -> reduce(mark(initial),add) print(88) return result }`
	inputs := [][]string{{"0", "0", "7"}, {"0", "5", "7"}, {"2", "4", "4294967295"}, {"65536", "65537", "0"}}
	for _, opt := range []string{"-O0", "-O3"} {
		compareDataflow(t, source, inputs, opt)
	}
	source = `fn step(state u64,x u32) u64 { if x%2==0 { return state+1 } return state }
 fn id(x u32) u32 { return x }
 fn f(a u32,b u32) u64 { return range(a,b) -> map(id) -> fold(100,step) }`
	compareDataflow(t, source, [][]string{{"0", "0"}, {"0", "7"}, {"4294967294", "4294967295"}}, "-O0")
}
func TestDataflowSignedReference(t *testing.T) {
	source := `fn neg(x i8) i8 { return -x } fn add(a i8,b i8) i8 { return a+b } fn valid(x i8) bool { return x!=0 }
 fn f(a i8,b i8) i8 { return range(a,b) -> filter(valid) -> map(neg) -> reduce(0,add) }`
	inputs := [][]string{}
	for _, a := range []int{-128, -127, -3, -1, 0, 1, 3, 126, 127} {
		for _, b := range []int{-128, -127, -3, -1, 0, 1, 3, 126, 127} {
			inputs = append(inputs, []string{strconv.Itoa(a), strconv.Itoa(b)})
		}
	}
	for _, opt := range []string{"-O0", "-O3"} {
		compareDataflow(t, source, inputs, opt)
	}
}
func TestDataflowIntegerWidthEdges(t *testing.T) {
	for _, tt := range []struct{ typ, start, end string }{
		{"u8", "254", "255"}, {"i8", "-128", "-127"}, {"u16", "65534", "65535"}, {"i16", "32766", "32767"},
		{"u32", "4294967294", "4294967295"}, {"i32", "-2147483648", "-2147483647"},
		{"u64", "18446744073709551614", "18446744073709551615"}, {"i64", "-9223372036854775808", "-9223372036854775807"},
	} {
		source := fmt.Sprintf("fn id(x %s) %s { return x } fn last(a %s,b %s) %s { return b } fn f(a %s,b %s) %s { return range(a,b) -> map(id) -> reduce(0,last) }", tt.typ, tt.typ, tt.typ, tt.typ, tt.typ, tt.typ, tt.typ, tt.typ)
		compareDataflow(t, source, [][]string{{tt.start, tt.end}, {tt.end, tt.end}, {tt.end, tt.start}}, "-O0")
	}
	for _, op := range []string{"/", "%"} {
		source := fmt.Sprintf("fn transform(x i8) i8 { return x %s -1 } fn last(a i8,b i8) i8 { return b } fn f(a i8,b i8) i8 { return range(a,b) -> map(transform) -> reduce(0,last) }", op)
		compareDataflow(t, source, [][]string{{"-128", "-127"}, {"-127", "-126"}}, "-O0")
	}
}
func TestDataflowScalarElementTypes(t *testing.T) {
	boolean := `fn flag(x u32) bool { return x>1 } fn either(a bool,b bool) bool { return a||b } fn f(a u32,b u32) bool { return range(a,b) -> map(flag) -> reduce(false,either) }`
	compareDataflow(t, boolean, [][]string{{"0", "0"}, {"0", "2"}, {"0", "4"}}, "-O0")
	for _, typ := range []string{"f32", "f64"} {
		source := fmt.Sprintf(`fn transform(x u32) %s { if x==0 { return 12 } return 13 } fn last(a %s,b %s) %s { return b } impure fn main() { print(range(0,0) -> map(transform) -> reduce(7,last)) print(range(0,3) -> map(transform) -> reduce(0,last)) }`, typ, typ, typ, typ)
		g := graphFor(t, source)
		canonical, _ := parser.NewCodeGenerator().Generate(g)
		optimized, err := GenerateSpecialized(g)
		if err != nil {
			t.Fatal(err)
		}
		for _, code := range []string{canonical, optimized.Code} {
			got := execute(t, buildC(t, code, "-O0"))
			if got.status != 0 || got.err != "" || got.out != "7\n13\n" {
				t.Fatalf("%s: %+v", typ, got)
			}
		}
	}
}
func TestDataflowNestedAndScalarSpecialization(t *testing.T) {
	source := dataflowCallbacks + `fn checked(x u32) u32 { return x-1 }
 fn transform(x u32) u32 { if x>0 { return checked(x) } return 0 }
 fn wrapper(x u32) u32 { if x>2 { return checked(x) } return 0 }
 fn f(a u32,b u32,initial u32) u32 { return range(a,b) -> map(transform) -> reduce(initial,add) }`
	g := graphFor(t, source)
	result, err := GenerateSpecialized(g)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Variants) == 0 || result.Summary.Reused == 0 || result.Dataflow[0].Decision != "FUSED" {
		t.Fatalf("specialization/fusion coexistence: %+v", result.Summary)
	}
	compareDataflow(t, source, [][]string{{"0", "5", "0"}, {"1", "4", "4294967295"}}, "-O0")
	nested := dataflowCallbacks + `fn f(a u32,b u32) u32 { return range(0,(range(a,b) -> map(id) -> reduce(0,add))) -> map(square) -> reduce(0,add) }`
	compareDataflow(t, nested, [][]string{{"0", "0"}, {"0", "5"}, {"2", "4"}}, "-O0")
	skipped := dataflowCallbacks + `fn f(a u32,b u32) u32 { if a==0 || (range(0,b) -> map(div) -> reduce(0,add))>0 { return 1 } return 0 }`
	compareDataflow(t, skipped, [][]string{{"0", "4"}, {"1", "1"}, {"1", "0"}}, "-O0")
}
func FuzzDataflowNative(f *testing.F) {
	source := dataflowSource("filter(even) -> map(square)", "reduce(initial,add)")
	g := graphFor(f, source)
	canonical, _ := parser.NewCodeGenerator().Generate(g)
	result, err := GenerateSpecialized(g)
	if err != nil {
		f.Fatal(err)
	}
	if result.Dataflow[0].Decision != "FUSED" {
		f.Fatal("fixture not fused")
	}
	harness := dataflowHarness(g)
	a := buildC(f, traceFunctions(g, canonical, nil)+harness, "-O0")
	b := buildC(f, traceFunctions(g, result.Code, result.Variants)+harness, "-O0")
	for _, seed := range []uint32{0, 1, 2, 65535, 65536, 4294967294, 4294967295} {
		f.Add(seed, byte(4), seed)
	}
	f.Fuzz(func(t *testing.T, start uint32, length byte, initial uint32) {
		n := uint64(length % 9)
		end := uint64(start) + n
		if end > 4294967295 {
			end = 4294967295
		}
		args := []string{strconv.FormatUint(uint64(start), 10), strconv.FormatUint(end, 10), strconv.FormatUint(uint64(initial), 10)}
		want := referenceOutcome(t, g, args)
		ca, cb := execute(t, a, args...), execute(t, b, args...)
		if ca != want || cb != want {
			t.Fatalf("%v: reference %+v canonical %+v fused %+v", args, want, ca, cb)
		}
	})
}
