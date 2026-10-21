package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"leek/bin/constraints"
	"leek/bin/optimization"
	"leek/bin/parser"
)

type options struct {
	input       string
	output      string
	graph       string
	constraints bool
	optimize    bool
}

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}

func run(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("leek", flag.ContinueOnError)
	fs.SetOutput(stderr)
	output := fs.String("o", "app", "output executable path")
	graph := fs.String("graph", "graph.dot", "output DOT graph path")
	analysis := fs.Bool("constraints", false, "report advisory constraints as JSON without compiling or writing artifacts")
	optimize := fs.Bool("optimize", false, "experimental: eliminate proven redundant unsigned arithmetic guards (default: checked everywhere)")
	if err := fs.Parse(args); err != nil {
		return 2
	}

	input := "main.lk"
	if fs.NArg() > 1 {
		fmt.Fprintln(stderr, "leek: expected at most one input .lk path")
		return 2
	}
	if fs.NArg() == 1 {
		input = fs.Arg(0)
	}
	if filepath.Ext(input) != ".lk" {
		fmt.Fprintf(stderr, "leek: input %q is not a .lk file\n", input)
		return 2
	}

	if err := compile(options{input: input, output: *output, graph: *graph, constraints: *analysis, optimize: *optimize}, stdout, stderr); err != nil {
		fmt.Fprintf(stderr, "leek: %v\n", err)
		return 1
	}
	return 0
}

func compile(opts options, stdout, stderr io.Writer) error {
	source, err := os.ReadFile(opts.input)
	if err != nil {
		return fmt.Errorf("cannot read %s: %w", opts.input, err)
	}

	program, err := parser.NewParser(strings.NewReader(string(source)), opts.input).ParseProgram()
	if err != nil {
		return err
	}
	res := parser.NewResolver()
	graph, err := res.Resolve(program)
	if err != nil {
		return err
	}

	if opts.constraints {
		report, err := constraints.Analyze(graph)
		if err != nil {
			return err
		}
		encoder := json.NewEncoder(stdout)
		encoder.SetIndent("", "  ")
		return encoder.Encode(report)
	}

	dot := graph.ToDOT()
	if err := writeArtifact(opts.graph, []byte(dot)); err != nil {
		return fmt.Errorf("write graph: %w", err)
	}

	var cCode string
	if opts.optimize {
		var result *optimization.SpecializationResult
		result, err = optimization.GenerateSpecialized(graph)
		if err == nil {
			cCode = result.Code
			fmt.Fprintf(stdout, "proof-gated: removed %d checked guards\n", len(result.LocalElisions))
			for _, variant := range result.Variants {
				fmt.Fprintf(stdout, "specialization: %s -> %s; removed %d guards; specialized %d calls; canonical preserved\n", variant.Function, variant.Name, len(variant.Eliminated), len(variant.Calls))
			}
		}
	} else {
		cCode, err = parser.NewCodeGenerator().Generate(graph)
	}
	if err != nil {
		return fmt.Errorf("code generation: %w", err)
	}
	if err := compileToBinary(cCode, opts.output, stderr); err != nil {
		return err
	}
	fmt.Fprintf(stdout, "compiled %s -> %s\n", opts.input, opts.output)
	return nil
}

func writeArtifact(path string, data []byte) error {
	if dir := filepath.Dir(path); dir != "." {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return err
		}
	}
	return os.WriteFile(path, data, 0644)
}

func compileToBinary(cCode, outputBinary string, stderr io.Writer) error {
	if err := writeArtifact(outputBinary+"_tmp.c", []byte(cCode)); err != nil {
		return fmt.Errorf("write temporary C file: %w", err)
	}
	tmpCFile := outputBinary + "_tmp.c"
	defer os.Remove(tmpCFile)

	outputPath, err := filepath.Abs(outputBinary)
	if err != nil {
		return fmt.Errorf("resolve output path: %w", err)
	}
	cmd := exec.Command("gcc", "-std=c11", "-Wall", "-Wextra", "-Werror", "-Wno-unused-parameter", "-O3", "-march=native", tmpCFile, "-o", outputPath)
	cmd.Stderr = stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("GCC compilation failed: %w", err)
	}
	return nil
}
