package parser

import (
	"strings"
	"testing"
)

func TestParser_BasicStructure(t *testing.T) {
	input := `
	fn test() {
		let x u32 = 10
	}
	`
	p := NewParser(strings.NewReader(input), "test.lk")
	ast, err := p.ParseProgram()
	if err != nil {
		t.Fatalf("Parser failed: %s", err)
	}

	if len(ast) != 1 {
		t.Errorf("Expected 1 statement, got %d", len(ast))
	}

	fn, ok := ast[0].(*FunctionStatement)
	if !ok {
		t.Fatalf("Expected function statement")
	}

	if fn.Name != "test" {
		t.Errorf("Expected name 'test', got '%s'", fn.Name)
	}
}
