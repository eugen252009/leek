package constraints

import (
	"fmt"
	"testing"
)

// PS-COMPOSE / PS-OUTPUT: branch-selected output relations must be valid at a
// caller's exact operation. Contextual proofs never rewrite a generic summary.
func TestConditionalCalleeExhaustive8Bit(t *testing.T) {
	cases := 0
	for _, typ := range []string{"u8", "i8"} {
		low := 0
		thresholds := []int{0, 1, 2, 10, 127, 254, 255}
		if typ == "i8" {
			low = -128
			thresholds = []int{-128, -127, -1, 0, 1, 10, 126, 127}
		}
		inputs := [][]int64{}
		for x := low; x < low+256; x++ {
			inputs = append(inputs, []int64{int64(x)})
		}
		for _, threshold := range thresholds {
			for _, cmp := range []string{"==", "!=", "<", "<=", ">", ">="} {
				for _, value := range []string{"x", "x-1", "x+1", "x*x", "100/x", "100%x"} {
					callee := fmt.Sprintf("fn choose(x %s) %s { if x %s %d { return %s } return 1 }", typ, typ, cmp, threshold, value)
					for _, body := range []string{
						"return 100/choose(x)",
						"return x -> choose -> inverse",
						"if choose(x) != 0 { return 100/choose(x) } return 0",
						"if x == 0 { return choose(x) } return choose(x)-1",
					} {
						source := fmt.Sprintf("%s fn inverse(x %s) %s { return 100/x } fn f(x %s) %s { %s }", callee, typ, typ, typ, typ, body)
						checkPathDomain(t, source, inputs)
						cases += len(inputs)
					}
				}
			}
		}
	}
	t.Logf("%d conditional-callee input/program valuations", cases)
}
