package parser

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

type execution struct {
	stdout, stderr string
	status         int
}

func resolveSource(t *testing.T, source string) *ProgramGraph {
	t.Helper()
	ast, err := NewParser(strings.NewReader(source), "program.lk").ParseProgram()
	if err != nil {
		t.Fatal(err)
	}
	g, err := NewResolver().Resolve(ast)
	if err != nil {
		t.Fatal(err)
	}
	return g
}
func generatedSource(t *testing.T, source string) (string, *ProgramGraph) {
	t.Helper()
	g := resolveSource(t, source)
	c, err := NewCodeGenerator().Generate(g)
	if err != nil {
		t.Fatal(err)
	}
	return c, g
}
func executeC(t *testing.T, code, opt string) execution {
	t.Helper()
	dir := t.TempDir()
	src := filepath.Join(dir, "program.c")
	bin := filepath.Join(dir, "program")
	if err := os.WriteFile(src, []byte(code), 0600); err != nil {
		t.Fatal(err)
	}
	args := []string{"-std=c11", "-Wall", "-Wextra", "-Werror", "-Wno-unused-parameter", opt, "-fsanitize=undefined", "-fno-sanitize-recover=all", src, "-o", bin}
	if out, err := exec.Command("gcc", args...).CombinedOutput(); err != nil {
		t.Fatalf("GCC: %v\n%s", err, out)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, bin)
	var out, errout bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errout
	err := cmd.Run()
	status := 0
	if err != nil {
		if e, ok := err.(*exec.ExitError); ok {
			status = e.ExitCode()
		} else {
			t.Fatal(err)
		}
	}
	if ctx.Err() != nil {
		t.Fatal(ctx.Err())
	}
	if strings.Contains(errout.String(), "runtime error:") {
		t.Fatalf("C undefined behavior: %s", errout.String())
	}
	return execution{out.String(), errout.String(), status}
}
func TestCodeGenerator_InitializersPreserveValues(t *testing.T) {
	// The old test inspected pointer spellings and injected a read before teardown.
	// Scalar returns now let us assert the same value-copy contract directly.
	for name, body := range map[string]string{
		"literal":              "let result u32 = 12",
		"local copy chain":     "let x u32 = 12 let y u32 = x let result u32 = y",
		"parameter then local": "let x u32 = input let result u32 = x",
		"direct parameter":     "let result u32 = input",
	} {
		t.Run(name, func(t *testing.T) {
			code, g := generatedSource(t, "fn check(input u32) u32 { "+body+" return result }")
			code += fmt.Sprintf("\nint main(void) { return %s(12) != 12; }\n", g.Functions["check"].CName)
			if got := executeC(t, code, "-O3"); got.status != 0 {
				t.Fatalf("copy check: %+v", got)
			}
		})
	}
}
func TestComposition_Runtime(t *testing.T) {
	tests := []struct{ name, source, want string }{
		{"chain and ordinary call equivalence", `fn square(x u32) u32 { return x * x }
fn double(x u32) u32 { return x * 2 }
impure fn main() { print(3 -> square -> double) print(double(square(3))) }`, "18\n18\n"},
		{"extra and nested arguments", `fn add(a u32,b u32) u32 { return a + b }
fn square(x u32) u32 { return x * x }
impure fn main() { print((2 -> add(3 -> square)) -> square) }`, "121\n"},
		{"forward call", `impure fn main() { print(4 -> square) } fn square(x u32) u32 { return x * x }`, "16\n"},
		{"effectful source and argument order", `impure fn mark(x u32) u32 { print(x) return x }
fn combine(a u32,b u32,c u32) u32 { return a * 100 + b * 10 + c }
fn identity(x u32) u32 { return x }
impure fn main() { print(mark(1) -> combine(mark(2),mark(3)) -> identity) }`, "1\n2\n3\n123\n"},
		{"ordinary call argument order", `impure fn mark(x u32) u32 { print(x) return x }
fn add(a u32,b u32) u32 { return a + b }
impure fn main() { print(add(mark(1),mark(2))) }`, "1\n2\n3\n"},
		{"arithmetic and Boolean operators", `impure fn main() {
print(2 + 3 * 4) print((2 + 3) * 4) print(17 / 3) print(17 % 3)
print(1 < 2 && 2 <= 2) print(3 > 2 && 3 >= 3) print(2 == 2 && 2 != 3) print(!true || true)
}`, "14\n20\n5\n2\n1\n1\n1\n1\n"},
		{"short circuit failure and effects", `fn fail() bool { let x u32 = 1 / 0 return x == 0 }
impure fn mark() bool { print(99) return true }
impure fn main() { print(false && fail()) print(true || fail()) print(false && mark()) print(true || mark()) }`, "0\n1\n0\n1\n"},
		{"signed bounds and division", `impure fn main() {
let min i64 = -9223372036854775808 let max u64 = 18446744073709551615
let a i8 = -7 let b i8 = 3
print(min) print(max) print(a / b) print(a % b) print(+b)
}`, "-9223372036854775808\n18446744073709551615\n-2\n-1\n3\n"},
		{"void return", `fn empty() { return } impure fn main() { empty() print(1) }`, "1\n"},
		{"local storage and C identifier isolation", `fn switch(parent_arena u32) u32 { let local_buffer u32 = parent_arena let uint32_t u32 = local_buffer return uint32_t }
impure fn main() { print(12 -> switch) }`, "12\n"},
		{"nested functions without captures", `fn outer() u32 { fn inner(x u32) u32 { return x + 1 } return 2 -> inner }
impure fn main() { print(outer()) }`, "3\n"},
		{"float scalar transport", `fn identity(x f64) f64 { return x } impure fn main() { let x f64 = 12 print(x -> identity) }`, "12\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			code, _ := generatedSource(t, tt.source)
			for _, opt := range []string{"-O0", "-O3"} {
				t.Run(opt, func(t *testing.T) {
					got := executeC(t, code, opt)
					if got.status != 0 || got.stderr != "" || got.stdout != tt.want {
						t.Fatalf("got %+v, want stdout %q", got, tt.want)
					}
				})
			}
		})
	}
}
func TestCheckedArithmetic_RuntimeFailures(t *testing.T) {
	tests := []struct{ name, source, reason, stdout string }{
		{"division zero", "let x u32 = 1 / 0", "division by zero", ""},
		{"remainder zero", "let x u32 = 1 % 0", "division by zero", ""},
		{"unsigned underflow", "let x u32 = 0 - 1", "overflow or underflow", ""},
		{"unsigned overflow", "let x u64 = 18446744073709551615 + 1", "overflow or underflow", ""},
		{"multiply narrow", "let x u8 = 16 * 16", "overflow or underflow", ""},
		{"signed overflow", "let x i64 = 9223372036854775807 + 1", "overflow or underflow", ""},
		{"signed underflow", "let x i64 = -9223372036854775808 - 1", "overflow or underflow", ""},
		{"signed division overflow", "let a i64 = -9223372036854775808 let b i64 = -1 let x i64 = a / b", "overflow or underflow", ""},
		{"signed remainder overflow", "let a i64 = -9223372036854775808 let b i64 = -1 let x i64 = a % b", "overflow or underflow", ""},
		{"negate signed min", "let a i8 = -128 let x i8 = -a", "overflow or underflow", ""},
		{"negate unsigned", "let a u8 = 1 let x u8 = -a", "overflow or underflow", ""},
		{"effects before failure survive", "print(7) let x u32 = 1 / 0 print(8)", "division by zero", "7\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			code, _ := generatedSource(t, "impure fn main() { "+tt.source+" }")
			for _, opt := range []string{"-O0", "-O3"} {
				got := executeC(t, code, opt)
				if got.status != 1 || got.stdout != tt.stdout || !strings.Contains(got.stderr, "program.lk:1:") || !strings.Contains(got.stderr, tt.reason) {
					t.Fatalf("%s: %+v", opt, got)
				}
			}
		})
	}
}
func TestComposition_PreservesFailureOrder(t *testing.T) {
	source := `fn minus1(x u32) u32 { return x - 1 }
fn divide(b u32) u32 { return 12 / b }
impure fn later() u32 { print(99) return 1 }
fn add(a u32,b u32) u32 { return a + b }
impure fn main() { let result u32 = INPUT -> minus1 -> divide -> add(later()) print(result) }`
	for _, tt := range []struct{ input, reason string }{{"0", "overflow or underflow"}, {"1", "division by zero"}} {
		code, _ := generatedSource(t, strings.Replace(source, "INPUT", tt.input, 1))
		for _, opt := range []string{"-O0", "-O3"} {
			got := executeC(t, code, opt)
			if got.status != 1 || got.stdout != "" || !strings.Contains(got.stderr, tt.reason) {
				t.Fatalf("input %s, %s: %+v", tt.input, opt, got)
			}
		}
	}
	code, _ := generatedSource(t, strings.Replace(source, "INPUT", "2", 1))
	if got := executeC(t, code, "-O3"); got.status != 0 || got.stdout != "99\n13\n" {
		t.Fatalf("valid domain: %+v", got)
	}
}
func TestComposition_ArgumentsFailBeforeStageApplication(t *testing.T) {
	code, _ := generatedSource(t, `fn stage(x u32, unused u32) u32 { return x - 1 }
fn argument() u32 { return 1 / 0 }
impure fn main() { let result u32 = 0 -> stage(argument()) print(result) }`)
	for _, opt := range []string{"-O0", "-O3"} {
		got := executeC(t, code, opt)
		if got.status != 1 || got.stdout != "" || !strings.Contains(got.stderr, "division by zero") {
			t.Fatalf("%s: argument failure must precede stage underflow: %+v", opt, got)
		}
	}
}

func TestComposition_BoundedEquivalence(t *testing.T) {
	code, g := generatedSource(t, `fn square(x u32) u32 { return x*x }
fn add(x u32,y u32) u32 { return x+y }
fn pipeline(x u32) u32 { return x -> square -> add(3) }
fn direct(x u32) u32 { return add(square(x),3) }`)
	code += fmt.Sprintf(`
int main(void) {
 for (uint32_t i=0;i<256;i++) {
  if (%s(i) != i*i+3 || %s(i) != i*i+3) return 2;
 }
 return 0;
}
`, g.Functions["pipeline"].CName, g.Functions["direct"].CName)
	for _, opt := range []string{"-O0", "-O3"} {
		if got := executeC(t, code, opt); got.status != 0 {
			t.Fatalf("%s: %+v", opt, got)
		}
	}
}
func TestCodeGenerator_RequiresValidatedGraphAndIsReusable(t *testing.T) {
	cg := NewCodeGenerator()
	if _, err := cg.Generate(NewProgramGraph()); err == nil {
		t.Fatal("accepted unvalidated graph")
	}
	g := resolveSource(t, "fn main() {}")
	first, err := cg.Generate(g)
	if err != nil {
		t.Fatal(err)
	}
	second, err := cg.Generate(g)
	if err != nil {
		t.Fatal(err)
	}
	if first != second {
		t.Fatal("generator leaked previous output")
	}
}
