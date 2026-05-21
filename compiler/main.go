package main

import (
	_ "embed"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"leek/bin/parser"
)

//go:embed main.lk
var input string

func compileToBinary(cCode string, outputBinary string) error {
	tmpCFile := outputBinary + "_tmp.c"
	err := os.WriteFile(tmpCFile, []byte(cCode), 0644)
	if err != nil {
		return fmt.Errorf("error writing temporary C file: %w", err)
	}
	defer os.Remove(tmpCFile)

	outputPath, err := filepath.Abs(outputBinary)
	if err != nil {
		return err
	}

	cmd := exec.Command("gcc", "-O3", "-march=native", tmpCFile, "-o", outputPath)

	cmd.Stderr = os.Stderr
	cmd.Stdout = os.Stdout

	err = cmd.Run()
	if err != nil {
		return fmt.Errorf("GCC compilation failed: %w", err)
	}

	return nil
}

func main() {
	srcReader := strings.NewReader(input)
	p := parser.NewParser(srcReader, "main.lk")

	programAST, err := p.ParseProgram()
	if err != nil {
		fmt.Printf("❌ Syntax Error: %s\n", err)
		return
	}

	fmt.Println("--- 1. Generated AST ---")
	for _, stmt := range programAST {
		fmt.Println(stmt.String())
	}

	fmt.Println("\n--- 2. Semantic Analysis & Graph Construction ---")
	res := parser.NewResolver()

	graph, err := res.Resolve(programAST)
	if err != nil {
		fmt.Printf("❌ Invariant Error: %s\n", err)
		return
	}
	fmt.Println("✅ Axioms and scoping invariants satisfied!")

	fmt.Println("\n--- 2b. Generated Data Flow Graph (DOT) ---")
	dotOutput := graph.ToDOT()
	fmt.Println(dotOutput)

	_ = os.WriteFile("graph.dot", []byte(dotOutput), 0644)
	fmt.Println("💾 Graph specification exported to 'graph.dot'!")

	fmt.Println("\n--- 3. Generated C Code ---")
	codegen := parser.NewCodeGenerator()

	cCode, err := codegen.Generate(graph)
	if err != nil {
		fmt.Printf("❌ Code generation error: %s\n", err)
		return
	}
	fmt.Println(cCode)

	outputName := "app"
	fmt.Printf("--- 4. Backend Compilation ---\n")
	fmt.Printf("[COMPILER] Generating native binary '%s'...\n", outputName)

	err = compileToBinary(cCode, outputName)
	if err != nil {
		fmt.Printf("❌ Binary generation error: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("✅ Compilation successful! Executable created: './%s'\n", outputName)
}
