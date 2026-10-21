package main

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunCompilesInputAndExecutesBinary(t *testing.T) {
	dir := t.TempDir()
	source := filepath.Join(dir, "program.lk")
	if err := writeArtifact(source, []byte("fn main() {\n  let x u32 = 12\n}\n")); err != nil {
		t.Fatal(err)
	}
	output := filepath.Join(dir, "bin", "program")
	graph := filepath.Join(dir, "graph.dot")
	var stdout, stderr bytes.Buffer

	if code := run([]string{"-o", output, "--graph", graph, source}, &stdout, &stderr); code != 0 {
		t.Fatalf("compile failed with status %d: %s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "compiled ") {
		t.Fatalf("unexpected compiler output: %q", stdout.String())
	}
	if _, err := exec.Command(output).CombinedOutput(); err != nil {
		t.Fatalf("generated binary failed: %v", err)
	}
	if data, err := os.ReadFile(graph); err != nil || !strings.Contains(string(data), "digraph LeekProgramGraph") {
		t.Fatalf("invalid graph artifact: %v", err)
	}
}

func TestRunCompilesEmptyFunctionsAndLocalCopies(t *testing.T) {
	for name, input := range map[string]string{
		"empty main":       "fn main() {}",
		"empty helper":     "fn helper(x u32) {} fn main() {}",
		"local copy chain": "fn main() { let x u32 = 12 let y u32 = x let z u32 = y }",
		"nested function":  "fn main() { fn inner() {} }",
	} {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			source := filepath.Join(dir, "program.lk")
			if err := writeArtifact(source, []byte(input)); err != nil {
				t.Fatal(err)
			}
			output := filepath.Join(dir, "program")
			var stderr bytes.Buffer
			if code := run([]string{"-o", output, "-graph", filepath.Join(dir, "graph.dot"), source}, &bytes.Buffer{}, &stderr); code != 0 {
				t.Fatalf("compile failed with status %d: %s", code, stderr.String())
			}
			if out, err := exec.Command(output).CombinedOutput(); err != nil {
				t.Fatalf("generated binary failed: %v\n%s", err, out)
			}
		})
	}
}

func TestRunRejectsInvalidParametersAndCapturesBeforeWritingArtifacts(t *testing.T) {
	for name, tt := range map[string]struct{ input, want string }{
		"unknown parameter type": {"fn helper(x mystery) {} fn main() {}", "unknown type \"mystery\""},
		"duplicate parameters":   {"fn helper(x u32, x u32) {} fn main() {}", "duplicate parameter \"x\""},
		"outer capture":          {"fn main() { let x u32 = 12 fn inner() { let y u32 = x } }", "capture of outer variable \"x\" is not yet supported"},
	} {
		t.Run(name, func(t *testing.T) {
			dir := t.TempDir()
			source := filepath.Join(dir, "program.lk")
			if err := writeArtifact(source, []byte(tt.input)); err != nil {
				t.Fatal(err)
			}
			output := filepath.Join(dir, "program")
			graph := filepath.Join(dir, "graph.dot")
			var stderr bytes.Buffer
			if code := run([]string{"-o", output, "-graph", graph, source}, &bytes.Buffer{}, &stderr); code != 1 {
				t.Fatalf("expected semantic failure, got status %d: %s", code, stderr.String())
			}
			if !strings.Contains(stderr.String(), source+":1:") || !strings.Contains(stderr.String(), tt.want) {
				t.Fatalf("unexpected diagnostic: %s", stderr.String())
			}
			for _, path := range []string{output, output + "_tmp.c", graph} {
				if _, err := os.Stat(path); !os.IsNotExist(err) {
					t.Errorf("unexpected artifact %s: %v", path, err)
				}
			}
		})
	}
}

func TestRunReturnsNonzeroForSyntaxError(t *testing.T) {
	dir := t.TempDir()
	source := filepath.Join(dir, "broken.lk")
	if err := writeArtifact(source, []byte("fn main( {")); err != nil {
		t.Fatal(err)
	}
	var stderr bytes.Buffer
	if code := run([]string{source}, &bytes.Buffer{}, &stderr); code == 0 {
		t.Fatal("syntax error returned success")
	}
	if !strings.Contains(stderr.String(), source+":1:") {
		t.Fatalf("diagnostic lacks source location: %q", stderr.String())
	}
}
