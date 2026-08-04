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
