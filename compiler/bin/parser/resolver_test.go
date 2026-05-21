package parser

import (
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

