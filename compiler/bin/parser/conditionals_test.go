package parser

import (
	"strings"
	"testing"
)

// PS-CONTROL: only the selected branch executes; returns terminate the function.
func TestConditionalSyntaxAndSemantics(t *testing.T) {
	source := `fn f(x u32) u32 {
 if x > 1 { let y u32 = x - 1 if y > 10 { return y } else { return 10 } }
 return 0
 }`
	ast, err := NewParser(strings.NewReader(source), "if.lk").ParseProgram()
	if err != nil {
		t.Fatal(err)
	}
	f := ast[0].(*FunctionStatement)
	branch, ok := f.Body.Statements[0].(*IfStatement)
	if !ok || branch.Else != nil || branch.Then == nil || branch.Condition.Name != ">" {
		t.Fatalf("branch: %+v", branch)
	}
	g, err := NewResolver().Resolve(ast)
	if err != nil || !g.Validated {
		t.Fatalf("resolve: %v", err)
	}
	if len(g.Blocks) != 4 || len(g.Returns) != 3 {
		t.Fatalf("block/return metadata: %d/%d", len(g.Blocks), len(g.Returns))
	}
}
func TestConditionalRejections(t *testing.T) {
	for _, tt := range []struct{ source, diagnostic string }{
		{`fn f() { if true return }`, "expected {"},
		{`fn f() { if true {} else return }`, "expected {"},
		{`fn f() { else {} }`, "reserved word"},
		{`fn if() {}`, "reserved word"},
		{`fn f(x u32) { if x {} }`, "type mismatch"},
		{`fn f(x u32) u32 { if x > 0 { return x } }`, "must return"},
		{`fn f() u32 { if true { return 1 } else { return 0 } return 2 }`, "unreachable"},
		{`fn f() u32 { if true { let y u32 = 1 } return y }`, "undefined symbol"},
		{`fn f() { if true { let y u32 = 1 let y u32 = 2 } }`, "duplicate declaration"},
		{`fn f() { if true { fn nested() {} } }`, "inside conditional blocks"},
		{`fn f(x u32) { fn nested() { if true { let y u32 = x } } }`, "capture of outer variable"},
		{`fn f() { if false { print(1) } }`, "pure function"},
		{`impure fn bad(x u32) u32 { return x } fn f(x u32) u32 { if x > 0 { return x -> bad } return 0 }`, "only accepts pure stages"},
	} {
		t.Run(tt.diagnostic+tt.source, func(t *testing.T) {
			ast, err := NewParser(strings.NewReader(tt.source), "if.lk").ParseProgram()
			if err == nil {
				_, err = NewResolver().Resolve(ast)
			}
			if err == nil || !strings.Contains(err.Error(), tt.diagnostic) {
				t.Fatalf("want %q, got %v", tt.diagnostic, err)
			}
		})
	}
}
func TestConditionalCanonicalExecution(t *testing.T) {
	source := `impure fn mark(x u32) u32 { print(x) return x }
 impure fn f(x u32) u32 {
 let outer u32 = x
 if mark(x) > 1 {
  let x u32 = outer - 1
  if x > 10 { return x }
  print(x)
 } else { let y u32 = outer print(y) }
 return outer
 }
 impure fn main() {
 print(f(0)) print(f(2)) print(f(20))
 if false && mark(99) > 0 { print(98) } else { print(7) }
 if true || mark(97) > 0 { print(8) }
 }`
	code, _ := generatedSource(t, source)
	for _, opt := range []string{"-O0", "-O3"} {
		got := executeC(t, code, opt)
		if got.status != 0 || got.stderr != "" || got.stdout != "0\n0\n0\n2\n1\n2\n20\n19\n7\n8\n" {
			t.Fatalf("%s: %+v", opt, got)
		}
	}
}
