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
