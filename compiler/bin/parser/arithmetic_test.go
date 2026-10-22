package parser

import (
	"fmt"
	"math/big"
	"strings"
	"testing"
)

func TestIntegerWidths_Boundaries(t *testing.T) {
	for _, typ := range []string{"u8", "u16", "u32", "u64", "i8", "i16", "i32", "i64"} {
		t.Run(typ, func(t *testing.T) {
			var bits uint
			fmt.Sscanf(typ[1:], "%d", &bits)
			max := new(big.Int).Lsh(big.NewInt(1), bits)
			min := new(big.Int)
			if typ[0] == 'i' {
				max.Rsh(max, 1)
				min.Neg(new(big.Int).Set(max))
			}
			max.Sub(max, big.NewInt(1))
			src := fmt.Sprintf("impure fn main() { let lo %s = %s let hi %s = %s print(lo) print(hi) }", typ, min, typ, max)
			code, _ := generatedSource(t, src)
			got := executeC(t, code, "-O3")
			want := min.String() + "\n" + max.String() + "\n"
			if got.status != 0 || got.stdout != want || got.stderr != "" {
				t.Fatalf("boundary: %+v, want %q", got, want)
			}
			for _, n := range []*big.Int{new(big.Int).Sub(min, big.NewInt(1)), new(big.Int).Add(max, big.NewInt(1))} {
				ast, err := NewParser(strings.NewReader(fmt.Sprintf("fn f() %s { return %s }", typ, n)), "range.lk").ParseProgram()
				if err != nil {
					t.Fatal(err)
				}
				if _, err = NewResolver().Resolve(ast); err == nil || !strings.Contains(err.Error(), "out of range") {
					t.Fatalf("accepted out-of-range %s for %s: %v", n, typ, err)
				}
			}
			for _, expr := range []string{max.String() + " + 1", min.String() + " - 1", max.String() + " * 2"} {
				code, _ := generatedSource(t, fmt.Sprintf("fn main() { let x %s = %s }", typ, expr))
				got := executeC(t, code, "-O3")
				if got.status != 1 || !strings.Contains(got.stderr, "overflow or underflow") {
					t.Fatalf("%s: %+v", expr, got)
				}
			}
		})
	}
}
func TestDecimalLeadingZeros(t *testing.T) {
	code, _ := generatedSource(t, "impure fn main() { print(010) print(009) }")
	if got := executeC(t, code, "-O3"); got.status != 0 || got.stdout != "10\n9\n" {
		t.Fatalf("decimal literals: %+v", got)
	}
}

func TestFloatLiteralTransportRounding(t *testing.T) {
	code, _ := generatedSource(t, `impure fn main() {
let x f32 = 16777217
let y f64 = 9007199254740993
print(x) print(y)
}`)
	for _, opt := range []string{"-O0", "-O3"} {
		got := executeC(t, code, opt)
		if got.status != 0 || got.stdout != "16777216\n9007199254740992\n" {
			t.Fatalf("%s: %+v", opt, got)
		}
	}
}

func FuzzParserAndResolver(f *testing.F) {
	for _, seed := range []string{
		sequencePrelude + `fn f(n u32) u32 { return range(0,n) -> filter(even) -> map(square) -> reduce(0,add) }`,
		"", "fn main() {}", "fn f(x u32) u32 { return x -> f }",
		"fn f(x i64) i64 { return -9223372036854775808 }",
		"fn f(x bool) bool { return !x || false }",
		"impure fn main() { print(1) }", "fn f() u32 { return 1 -> missing(2) }",
	} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, source string) {
		if len(source) > 4096 {
			t.Skip()
		}
		ast, err := NewParser(strings.NewReader(source), "fuzz.lk").ParseProgram()
		if err != nil {
			return
		}
		graph, err := NewResolver().Resolve(ast)
		if err != nil {
			return
		}
		// Successful semantic validation must always have a usable lowering.
		if _, err := NewCodeGenerator().Generate(graph); err != nil {
			t.Fatal(err)
		}
	})
}
