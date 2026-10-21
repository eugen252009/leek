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

func TestOptimizationCLI(t *testing.T) {
	dir := t.TempDir()
	var canonicalDOT []byte
	for _, mode := range []string{"default", "enabled", "disabled"} {
		binary := filepath.Join(dir, mode)
		args := []string{"-o", binary, "-graph", binary + ".dot"}
		if mode == "enabled" {
			args = append(args, "-optimize")
		}
		if mode == "disabled" {
			args = append(args, "-optimize=false")
		}
		args = append(args, "examples/path-sensitive.lk")
		var out, errout bytes.Buffer
		if code := run(args, &out, &errout); code != 0 {
			t.Fatalf("%s: %d %s", mode, code, errout.String())
		}
		if mode == "enabled" && !strings.Contains(out.String(), "proof-gated: removed 3 checked guards") {
			t.Fatalf("optimization not enabled: %s", out.String())
		}
		if mode != "enabled" && strings.Contains(out.String(), "proof-gated:") {
			t.Fatal("experimental optimization enabled by default")
		}
		result, err := exec.Command(binary).CombinedOutput()
		if err != nil || string(result) != "12\n12\n105\n" {
			t.Fatalf("%s: %v: %s", mode, err, result)
		}
		dot, err := os.ReadFile(binary + ".dot")
		if err != nil {
			t.Fatal(err)
		}
		if mode == "default" {
			canonicalDOT = dot
		} else if !bytes.Equal(canonicalDOT, dot) {
			t.Fatal("optimization changed semantic DOT")
		}
		if _, err := os.Stat(binary + "_tmp.c"); !os.IsNotExist(err) {
			t.Fatal("temporary C not removed")
		}
	}
}
func TestOptimizationInspectionIsAlwaysReadOnly(t *testing.T) {
	// Inspection takes priority even when both flags are supplied.
	t.Setenv("PATH", t.TempDir())
	dir := t.TempDir()
	var out, errout bytes.Buffer
	args := []string{"-optimize", "-constraints", "-o", filepath.Join(dir, "app"), "-graph", filepath.Join(dir, "graph.dot"), "examples/path-sensitive.lk"}
	if status := run(args, &out, &errout); status != 0 {
		t.Fatalf("%d: %s", status, errout.String())
	}
	var report constraints.Report
	if err := json.Unmarshal(out.Bytes(), &report); err != nil {
		t.Fatal(err)
	}
	if report.Version != 2 || !report.AnalysisOnly || report.RuntimeChecks != "preserved" {
		t.Fatal("inspection changed runtime")
	}
	for _, f := range report.Functions {
		if f.Name == "calculate" || f.Name == "local_calculate" {
			if f.Proof.Status != constraints.Proven || len(f.Obligations) != 2 {
				t.Fatalf("%s: %+v", f.Name, f)
			}
			for _, o := range f.Obligations {
				if o.Proof.Status != constraints.Proven {
					t.Fatal(o)
				}
			}
		}
	}
	entries, err := os.ReadDir(dir)
	if err != nil || len(entries) != 0 {
		t.Fatalf("inspection wrote artifacts: %v %v", entries, err)
	}
}
