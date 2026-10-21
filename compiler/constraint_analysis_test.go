package main

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"leek/bin/constraints"
)

func TestConstraintInspectionMode(t *testing.T) {
	dir := t.TempDir()
	source := filepath.Join(dir, "library.lk")
	input := `fn minus1(x u32) u32 { return x-1 }
fn divide(x u32) u32 { return 12/x }
fn calculate(x u32) u32 { return x -> minus1 -> divide }
fn failing() u32 { return 0 -> calculate }`
	if err := os.WriteFile(source, []byte(input), 0600); err != nil {
		t.Fatal(err)
	}
	binary, dot := filepath.Join(dir, "app"), filepath.Join(dir, "graph.dot")
	var out, errout bytes.Buffer
	// Inspection works for a library (no main) and requires no native toolchain.
	t.Setenv("PATH", dir)
	if status := run([]string{"-constraints", "-o", binary, "-graph", dot, source}, &out, &errout); status != 0 {
		t.Fatalf("status %d: %s", status, errout.String())
	}
	var report constraints.Report
	if err := json.Unmarshal(out.Bytes(), &report); err != nil {
		t.Fatalf("not standalone JSON: %v\n%s", err, out.String())
	}
	if !report.AnalysisOnly || report.RuntimeChecks != "preserved" {
		t.Fatal("missing advisory classification")
	}
	found := map[string]bool{}
	for _, f := range report.Functions {
		if f.Name == "calculate" {
			found[f.Name] = true
			if f.PreconditionText != "(x > 1)" || f.Proof.Status != constraints.Unknown {
				t.Fatalf("unexpected precondition: %+v", f)
			}
		}
		if f.Name == "failing" {
			found[f.Name] = true
			if f.Proof.Status != constraints.Violated {
				t.Fatal(f.Proof)
			}
		}
	}
	if !found["calculate"] || !found["failing"] {
		t.Fatal("missing summaries")
	}
	for _, path := range []string{binary, dot, binary + "_tmp.c"} {
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Fatalf("inspection wrote an execution artifact: %s", path)
		}
	}
}
func TestConstraintModeKeepsSemanticValidation(t *testing.T) {
	dir := t.TempDir()
	source := filepath.Join(dir, "bad.lk")
	if err := os.WriteFile(source, []byte("fn main() { print(1) }"), 0600); err != nil {
		t.Fatal(err)
	}
	var out, errout bytes.Buffer
	if status := run([]string{"-constraints", source}, &out, &errout); status != 1 || !strings.Contains(errout.String(), "pure function") {
		t.Fatalf("status %d: %s", status, errout.String())
	}
	if out.Len() != 0 {
		t.Fatal("emitted a report for an invalid program")
	}
}

// CA-PRESERVE: analysis never rejects failing programs or affects the native
// result, failure kind/location, effect trace, or exit status.
func TestConstraintInspectionPreservesRuntime(t *testing.T) {
	prelude := `fn minus1(x u32) u32 { return x-1 }
fn divide(x u32) u32 { return 12/x }
impure fn source(x u32) u32 { print(x) return x }
`
	for _, tt := range []struct {
		name, body, stdout, reason string
		status                     int
	}{
		{"success", "print(2 -> minus1 -> divide)", "12\n", "", 0},
		{"underflow", "print(7) print(0 -> minus1 -> divide) print(9)", "7\n", "overflow or underflow", 1},
		{"zero divisor", "print(1 -> minus1 -> divide)", "", "division by zero", 1},
		{"effectful source", "print(source(0) -> minus1 -> divide)", "0\n", "overflow or underflow", 1},
		{"skipped failure", "print(false && (12 / 0 == 1))", "0\n", "", 0},
	} {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			src := filepath.Join(dir, "program.lk")
			if err := os.WriteFile(src, []byte(prelude+"impure fn main() { "+tt.body+" }"), 0600); err != nil {
				t.Fatal(err)
			}
			type result struct {
				out, err string
				status   int
			}
			compileAndRun := func(suffix string) result {
				binary := filepath.Join(dir, suffix)
				var compilerOut, compilerErr bytes.Buffer
				if status := run([]string{"-o", binary, "-graph", binary + ".dot", src}, &compilerOut, &compilerErr); status != 0 {
					t.Fatalf("compile: %s", compilerErr.String())
				}
				cmd := exec.Command(binary)
				var out, errout bytes.Buffer
				cmd.Stdout = &out
				cmd.Stderr = &errout
				err := cmd.Run()
				status := 0
				if err != nil {
					e, ok := err.(*exec.ExitError)
					if !ok {
						t.Fatal(err)
					}
					status = e.ExitCode()
				}
				return result{out.String(), errout.String(), status}
			}
			before := compileAndRun("before")
			var analysisOut, analysisErr bytes.Buffer
			if status := run([]string{"-constraints", src}, &analysisOut, &analysisErr); status != 0 {
				t.Fatalf("analysis: %s", analysisErr.String())
			}
			after := compileAndRun("after")
			if before != after {
				t.Fatalf("behavior changed: before=%+v after=%+v", before, after)
			}
			if after.out != tt.stdout || after.status != tt.status || !strings.Contains(after.err, tt.reason) {
				t.Fatalf("wrong reference outcome: %+v", after)
			}
		})
	}
}
