package optimization

import (
	"fmt"
	"strconv"
	"testing"

	"leek/bin/parser"
)

// PG-EQUIV: bounded subprocess fuzzing compares native observables, not just
// analysis statuses. UBSan turns accidental C UB into a test failure.
func FuzzNativeEquivalence(f *testing.F) {
	source := `fn sub(x u32) u32 { if x > 0 { return x-1 } return 0 }
 fn mul(x u32) u32 { if x > 0 { return x*x } return 0 }
 fn div(x u32) u32 { if x != 0 { return 100/x } return 0 }
 fn short(x u32) u32 { if x != 0 && 100/x > 2 { return 1 } return 0 }
 fn disjunction(x u32) u32 { if x > 10 || x == 0 { return x-1 } return 0 }`
	g := graphFor(f, source)
	canonical, err := parser.NewCodeGenerator().Generate(g)
	if err != nil {
		f.Fatal(err)
	}
	optimized, elisions, err := Generate(g)
	if err != nil {
		f.Fatal(err)
	}
	if len(elisions) != 3 {
		f.Fatalf("unexpected optimization decisions: %v", elisions)
	}
	harness := "\nint main(int argc,char** argv) { if(argc!=3)return 2; uint32_t x=(uint32_t)strtoull(argv[2],NULL,10); uint32_t result=0;switch(atoi(argv[1])) {\n"
	for i, name := range []string{"sub", "mul", "div", "short", "disjunction"} {
		harness += fmt.Sprintf("case %d:result=%s(x);break;\n", i, g.Functions[name].CName)
	}
	harness += "default:return 2;} printf(\"%\" PRIu32 \"\\n\",result);return 0;}\n"
	a := buildC(f, canonical+harness, "-O0")
	b := buildC(f, optimized+harness, "-O0")
	for i := byte(0); i < 5; i++ {
		for _, x := range []uint32{0, 1, 2, 10, 11, 100, 101, 65535, 65536, 4294967294, 4294967295} {
			f.Add(i, x)
		}
	}
	f.Fuzz(func(t *testing.T, which byte, x uint32) {
		args := []string{strconv.Itoa(int(which % 5)), strconv.FormatUint(uint64(x), 10)}
		want := execute(t, a, args...)
		got := execute(t, b, args...)
		if want != got {
			t.Fatalf("case %v: canonical %+v optimized %+v", args, want, got)
		}
	})
}
