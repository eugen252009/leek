package parser

import (
	"strings"
	"testing"
)

func TestResolver_ScopeInvariant(t *testing.T) {
	ast := []Statement{
		&FunctionStatement{
			Name: "test",
			Body: &BlockStatement{
				Statements: []Statement{
					&LetStatement{Name: "v", Type: "u32", Value: "42"},
				},
			},
		},
	}

	res := NewResolver()
	graph, err := res.Resolve(ast)
	if err != nil {
		t.Fatalf("Resolver failed: %s", err)
	}

	if _, exists := graph.Nodes["test"]; !exists {
		t.Errorf("Resolver did not create a graph node for function 'test'")
	}
}

func TestResolver_ParameterAndCaptureValidation(t *testing.T) {
	tests := []struct {
		name   string
		source string
		want   string
	}{
		{"duplicate parameters", "fn helper(x u32,\n x u32) {}", "program.lk:2:2: semantic error: duplicate parameter \"x\""},
		{"unknown parameter type", "fn helper(\n x mystery) {}", "program.lk:2:2: semantic error: unknown type \"mystery\" for parameter \"x\""},
		{"parameter redeclared as local", "fn helper(x u32) { let x u32 = 1 }", "duplicate declaration \"x\""},
		{"outer local capture", "fn outer() { let x u32 = 1\n fn inner() { let y u32 = x } }", "program.lk:2:15: semantic error: capture of outer variable \"x\" is not yet supported"},
		{"outer parameter capture", "fn outer(x u32) { fn inner() { let y u32 = x } }", "capture of outer variable \"x\" is not yet supported"},
		{"undefined reference", "fn helper() { let y u32 = x }", "undefined symbol: x"},
		{"local shadows outer variable", "fn outer() { let x u32 = 1 fn inner() { let x u32 = 2 let y u32 = x } }", ""},
		{"parameter shadows outer variable", "fn outer() { let x u32 = 1 fn inner(x u32) { let y u32 = x } }", ""},
		{"independent parameter names", "fn first(x u32) {} fn second(x u32) {}", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ast, err := NewParser(strings.NewReader(tt.source), "program.lk").ParseProgram()
			if err != nil {
				t.Fatal(err)
			}
			_, err = NewResolver().Resolve(ast)
			if tt.want == "" {
				if err != nil {
					t.Fatal(err)
				}
			} else if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("expected %q, got %v", tt.want, err)
			}
		})
	}
}

func TestResolver_AllPrimitiveParameterTypes(t *testing.T) {
	for _, typ := range []string{"u8", "u16", "u32", "u64", "i8", "i16", "i32", "i64", "f32", "f64", "bool"} {
		t.Run(typ, func(t *testing.T) {
			ast, err := NewParser(strings.NewReader("fn helper(x "+typ+") {}"), "program.lk").ParseProgram()
			if err != nil {
				t.Fatal(err)
			}
			graph, err := NewResolver().Resolve(ast)
			if err != nil {
				t.Fatal(err)
			}
			if graph.Nodes["helper:x"] == nil {
				t.Fatal("missing parameter graph node")
			}
		})
	}
}

func TestResolver_RejectsDuplicateDeclarationWithLocation(t *testing.T) {
	ast := []Statement{&FunctionStatement{
		Name: "main",
		Pos:  SourcePos{Filename: "program.lk", Line: 1, Column: 1},
		Body: &BlockStatement{Statements: []Statement{
			&LetStatement{Name: "x", Type: "u32", Value: "1", Pos: SourcePos{Filename: "program.lk", Line: 2, Column: 1}},
			&LetStatement{Name: "x", Type: "u32", Value: "2", Pos: SourcePos{Filename: "program.lk", Line: 3, Column: 1}},
		}},
	}}
	_, err := NewResolver().Resolve(ast)
	if err == nil || !strings.Contains(err.Error(), "program.lk:3:1") {
		t.Fatalf("expected located duplicate declaration error, got %v", err)
	}
}
