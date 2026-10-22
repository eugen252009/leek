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
	"leek/bin/optimization"
)

func TestDataflowCLI(t *testing.T) {
	dir := t.TempDir()
	var canonicalDOT []byte
	for _, mode := range []string{"canonical", "optimized", "disabled"} {
		binary := filepath.Join(dir, mode)
		args := []string{"-o", binary, "-graph", binary + ".dot"}
		if mode == "optimized" {
			args = append(args, "-optimize", "-optimization-report", binary+".json")
		}
		if mode == "disabled" {
			args = append(args, "-optimize=false")
		}
		args = append(args, "examples/dataflow.lk")
		var out, errout bytes.Buffer
		if status := run(args, &out, &errout); status != 0 {
			t.Fatalf("%s %d: %s", mode, status, errout.String())
		}
		result, err := exec.Command(binary).CombinedOutput()
		if err != nil || string(result) != "30\n20\n11\n" {
			t.Fatalf("%s: %v %s", mode, err, result)
		}
		if mode == "optimized" {
			if !strings.Contains(out.String(), "dataflow fusion: fused 2; canonical 1") {
				t.Fatalf("missing decisions: %s", out.String())
			}
			data, err := os.ReadFile(binary + ".json")
			if err != nil {
				t.Fatal(err)
			}
			var report optimization.SpecializationResult
			if err := json.Unmarshal(data, &report); err != nil {
				t.Fatal(err)
			}
			fused, fallback := 0, 0
			for _, d := range report.Dataflow {
				if d.Decision == "FUSED" {
					fused++
					if d.Legality.Status != constraints.Proven || d.IteratorStates || d.IntermediateMaterialization {
						t.Fatal(d)
					}
				} else {
					fallback++
				}
			}
			if fused != 2 || fallback != 1 {
				t.Fatalf("invalid report: %+v", report.Dataflow)
			}
		} else if strings.Contains(out.String(), "dataflow fusion:") {
			t.Fatal("fusion enabled without opt-in")
		}
		dot, err := os.ReadFile(binary + ".dot")
		if err != nil {
			t.Fatal(err)
		}
		if mode == "canonical" {
			canonicalDOT = dot
		} else if !bytes.Equal(canonicalDOT, dot) {
			t.Fatal("semantic graph changed with execution strategy")
		}
		if _, err := os.Stat(binary + "_tmp.c"); !os.IsNotExist(err) {
			t.Fatal("temporary C remains")
		}
	}
}
func TestDataflowInspectionWithoutArtifacts(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("PATH", t.TempDir())
	var out, errout bytes.Buffer
	if status := run([]string{"-constraints", "-optimize", "-o", filepath.Join(dir, "app"), "-graph", filepath.Join(dir, "graph.dot"), "-optimization-report", filepath.Join(dir, "opt.json"), "examples/dataflow.lk"}, &out, &errout); status != 0 {
		t.Fatalf("%d: %s", status, errout.String())
	}
	var report constraints.Report
	if err := json.Unmarshal(out.Bytes(), &report); err != nil {
		t.Fatal(err)
	}
	found := false
	for _, f := range report.Functions {
		if f.Name == "sum_squares" {
			found = true
			if f.Proof.Status != constraints.Unknown || f.Complete || len(f.Dataflow) != 3 {
				t.Fatalf("%+v", f)
			}
		}
	}
	entries, err := os.ReadDir(dir)
	if err != nil || len(entries) != 0 || !found {
		t.Fatalf("inspection artifacts/missing report: %v %v", entries, err)
	}
}
