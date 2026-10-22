package optimization

import (
	"strconv"
	"testing"

	"leek/bin/parser"
)

func FuzzQuantifiedNative(f *testing.F) {
	source := quantifiedCallbacks + `fn f(a u32,b u32,initial u32) u32 {
 if a<=100 && b<=100 { return range(a,b) -> filter(positive) -> map(square) -> reduce(initial,add) }
 return range(65535,65537) -> map(square) -> reduce(initial,add)
 }`
	g := graphFor(f, source)
	canonical, _ := parser.NewCodeGenerator().Generate(g)
	result, err := GenerateSpecialized(g)
	if err != nil {
		f.Fatal(err)
	}
	removed := 0
	for _, d := range result.Quantified {
		removed += d.GuardsEliminated
	}
	if removed == 0 {
		f.Fatal("fixture did not enable quantified optimization")
	}
	harness := dataflowHarness(g)
	a := buildC(f, traceFunctions(g, canonical, nil)+harness, "-O0")
	b := buildC(f, traceFunctions(g, result.Code, result.Variants)+harness, "-O0")
	for _, n := range []byte{0, 1, 2, 99, 100, 101, 102} {
		f.Add(n, byte(100), uint32(0))
		f.Add(byte(0), n, uint32(4294967295))
	}
	f.Fuzz(func(t *testing.T, start, end byte, initial uint32) {
		args := []string{strconv.Itoa(int(start) % 103), strconv.Itoa(int(end) % 103), strconv.FormatUint(uint64(initial), 10)}
		want := referenceOutcome(t, g, args)
		ca, cb := execute(t, a, args...), execute(t, b, args...)
		if ca != want || cb != want {
			t.Fatalf("%v: reference %+v canonical %+v optimized %+v", args, want, ca, cb)
		}
	})
}
