package parser

import (
	"strings"
	"testing"
)

func TestParser_CompositionStructureAndPrecedence(t *testing.T) {
	tests := []struct{ expr, want string }{
		{"x -> square -> double", "((x -> square()) -> double())"},
		{"x + 1 * 2 -> transform(3, 4)", "((x + (1 * 2)) -> transform(3, 4))"},
		{"x -> transform(y -> square)", "(x -> transform((y -> square())))"},
		{"(x -> square) + 1", "((x -> square()) + 1)"},
		{"x < y && true || false -> negate", "((((x < y) && true) || false) -> negate())"},
		{"x\n -> square\n -> double", "((x -> square()) -> double())"},
		{"f(x, y -> square)", "f(x, (y -> square()))"},
		{"-x * 2 + +y", "(((-x) * 2) + (+y))"},
	}
	for _, tt := range tests {
		t.Run(tt.expr, func(t *testing.T) {
			ast, err := NewParser(strings.NewReader("fn sample(x u32) u32 { return "+tt.expr+" }"), "parse.lk").ParseProgram()
			if err != nil {
				t.Fatal(err)
			}
			f := ast[0].(*FunctionStatement)
			if f.ReturnType != "u32" {
				t.Fatal("lost return type")
			}
			e := f.Body.Statements[0].(*ReturnStatement).Value
			if got := e.String(); got != tt.want {
				t.Fatalf("got %s, want %s", got, tt.want)
			}
			if e.Pos.Filename != "parse.lk" || e.Pos.Line == 0 {
				t.Fatal("missing position")
			}
		})
	}
}
func TestParser_RejectsMalformedComposition(t *testing.T) {
	for _, source := range []string{
		"->", "==", "!=", "<=", ">=", "&&", "||", "fn f() u32 { return 1 ->",
		"fn f() -> u32 { return 1 }",
		"fn f() u32 { return 1 -> }",
		"fn f() u32 { return 1 -> 2 }",
		"fn f() u32 { return 1 -> (other) }",
		"fn f() u32 { return 1 -> other + 2 }",
		"fn f() u32 { return other(1 2) }",
		"fn f(x u32 y u32) {}",
		"fn f() u32 { return 1 - > other }",
		"fn f() u32 { return 1 -> other( }",
		"fn f() u32 { return 0b102 }",
	} {
		t.Run(source, func(t *testing.T) {
			_, err := NewParser(strings.NewReader(source), "bad.lk").ParseProgram()
			if err == nil || !strings.Contains(err.Error(), "bad.lk:") {
				t.Fatalf("expected located error, got %v", err)
			}
		})
	}
}
func TestComposition_SemanticRejections(t *testing.T) {
	tests := []struct{ name, source, want string }{
		{"wrong input", `fn f(x u8) u8 { return x } fn g(x u32) u8 { return x -> f }`, "type mismatch"},
		{"wrong output", `fn f(x u8) u8 { return x } fn g() u32 { return 1 -> f }`, "type mismatch"},
		{"wrong extra argument", `fn f(x u32,b bool) u32 { return x } fn g() u32 { return 1 -> f(2) }`, "integer literal cannot have type bool"},
		{"missing extra argument", `fn f(x u32,b u32) u32 { return x } fn g() u32 { return 1 -> f }`, "expects 2 arguments, got 1"},
		{"too many arguments", `fn f(x u32) u32 { return x } fn g() u32 { return 1 -> f(2) }`, "expects 1 arguments, got 2"},
		{"impure stage", `impure fn effect(x u32) u32 { return x } impure fn main() { let x u32 = 1 -> effect }`, "only accepts pure stages"},
		{"print stage", `fn f(x u32) u32 { return x -> print }`, "only accepts pure stages"},
		{"direct print", `fn f() { print(1) }`, "pure function \"f\" cannot call effectful function \"print\""},
		{"transitive impurity", `fn a(x u32) u32 { return b(x) } fn b(x u32) u32 { print(x) return x } fn c() u32 { return 1 -> a }`, "only accepts pure stages"},
		{"transitive ordinary calls", `fn a(x u32) u32 { return b(x) } fn b(x u32) u32 { print(x) return x }`, "pure function \"a\" cannot call effectful function \"b\""},
		{"effectful source in pure scope", `impure fn source() u32 { return 1 } fn id(x u32) u32 { return x } fn f() u32 { return source() -> id }`, "cannot call effectful function \"source\""},
		{"effectful argument in pure scope", `impure fn source() u32 { return 1 } fn add(x u32,y u32) u32 { return x+y } fn f() u32 { return 1 -> add(source()) }`, "cannot call effectful function \"source\""},
		{"explicit impure boundary", `impure fn f(x u32) u32 { return x } fn g() u32 { return f(1) }`, "cannot call effectful function \"f\""},
		{"direct recursion", `fn f(x u32) u32 { return f(x) }`, "recursive calls are not yet supported"},
		{"mutual recursion", `fn a(x u32) u32 { return b(x) } fn b(x u32) u32 { return x -> a }`, "recursive calls are not yet supported"},
		{"unknown external", `impure fn main() { read_input() }`, "unknown function \"read_input\""},
		{"unknown stage", `fn f() u32 { return 1 -> missing }`, "unknown function \"missing\""},
		{"indirect call", `fn id(x u32) u32 { return x } fn f(id u32) u32 { return id(1) }`, "indirect calls are not supported"},
		{"void source", `fn empty() {} fn id(x u32) u32 { return x } fn f() u32 { return empty() -> id }`, "got no result"},
		{"void stage", `fn empty(x u32) {} fn f() { 1 -> empty }`, "must return a scalar"},
		{"missing return", `fn f() u32 { let x u32 = 1 }`, "must return u32"},
		{"bare value return", `fn f() u32 { return }`, "return requires a value"},
		{"void value return", `fn f() { return 1 }`, "has no result"},
		{"unreachable effect", `fn f() u32 { return 1 print(2) }`, "unreachable statement"},
		{"unknown result type", `fn f() mystery { return 1 }`, "unknown return type"},
		{"literal out of range", `fn f() u8 { return 256 }`, "out of range"},
		{"negative unsigned literal", `fn f() u32 { return -1 }`, "out of range"},
		{"numeric bool", `fn f() bool { return 1 }`, "integer literal cannot have type bool"},
		{"bool arithmetic", `fn f() bool { return true + false }`, "requires integer operands"},
		{"float arithmetic deferred", `fn f(x f32) f32 { return x + x }`, "requires integer operands"},
		{"bad entry", `fn main(x u32) {}`, "entry function main"},
		{"nested visibility", `fn outer() { fn inner() {} } fn other() { inner() }`, "outside its lexical scope"},
		{"underscore deferred", `fn f(x u32,b u32) u32 { return x+b } fn g() u32 { return 1 -> f(_) }`, "undefined symbol: _"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ast, err := NewParser(strings.NewReader(tt.source), "invalid.lk").ParseProgram()
			if err != nil {
				t.Fatalf("test must reach semantics: %v", err)
			}
			_, err = NewResolver().Resolve(ast)
			if err == nil || !strings.Contains(err.Error(), tt.want) || !strings.Contains(err.Error(), "invalid.lk:") {
				t.Fatalf("want %q with location, got %v", tt.want, err)
			}
		})
	}
}
func TestComposition_IRSeparatesStageAndExpressionEffects(t *testing.T) {
	g := resolveSource(t, `impure fn source() u32 { print(1) return 2 }
fn square(x u32) u32 { return x*x }
impure fn main() { let result u32 = source() -> square print(result) }`)
	f := g.Functions["main"]
	e := f.Decl.Body.Statements[0].(*LetStatement).Expr
	info := g.Expressions[e]
	if e.Kind != "composition" || e.Left.Kind != "call" || info.Callee != g.Functions["square"] {
		t.Fatal("composition relation lost")
	}
	if info.Type != "u32" || info.StageEffects != EffectNone || info.Effects == EffectNone {
		t.Fatalf("incorrect composition annotation: %+v", info)
	}
	if !info.MayFail || !g.Functions["square"].MayFail {
		t.Fatal("checked failure information lost")
	}
	if g.Functions["square"].Effects != EffectNone || f.Effects == EffectNone {
		t.Fatal("purity summaries conflated")
	}
	data, effect := false, false
	for _, edge := range g.Edges {
		if edge.Type == DataEdge && edge.From == g.Expressions[e.Left].Node && edge.To == info.Node {
			data = true
		}
		if edge.Type == EffectEdge {
			effect = true
		}
	}
	if !data || !effect {
		t.Fatal("missing value or effect edges")
	}
	if err := validateNoCycles(g); err != nil {
		t.Fatal(err)
	}
	if g.ToDOT() != g.ToDOT() {
		t.Fatal("DOT output is nondeterministic")
	}
}
func TestComposition_TransitiveFailureWithoutEffects(t *testing.T) {
	g := resolveSource(t, `fn outer(x u32) u32 { return x -> inner } fn inner(x u32) u32 { return 12/x }`)
	for _, name := range []string{"outer", "inner"} {
		if !g.Functions[name].MayFail || g.Functions[name].Effects != EffectNone {
			t.Fatalf("wrong summary for %s", name)
		}
	}
}
