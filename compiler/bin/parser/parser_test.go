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

func TestParser_ParameterPositions(t *testing.T) {
	p := NewParser(strings.NewReader("fn helper(\n  x u32,\n  y u64\n) {}"), "params.lk")
	ast, err := p.ParseProgram()
	if err != nil {
		t.Fatal(err)
	}
	params := ast[0].(*FunctionStatement).Parameters
	if len(params) != 2 {
		t.Fatalf("expected two parameters, got %d", len(params))
	}
	for i, param := range params {
		if param.Pos.Filename != "params.lk" || param.Pos.Line != i+2 || param.Pos.Column != 3 {
			t.Errorf("unexpected parameter position: %+v", param.Pos)
		}
	}
}

func TestParser_DiagnosticIncludesLocation(t *testing.T) {
	p := NewParser(strings.NewReader("fn main() {\n  let x u32 =\n}"), "broken.lk")
	_, err := p.ParseProgram()
	if err == nil || !strings.Contains(err.Error(), "broken.lk:3:") {
		t.Fatalf("expected located syntax error, got %v", err)
	}
}
