package main

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"leek/bin/constraints"
	"leek/bin/optimization"
)

func TestQuantifiedCLI(t *testing.T) {
	dir := t.TempDir()
	var baseline []byte
	for _, mode := range []string{"canonical", "optimized", "disabled"} {
		binary := filepath.Join(dir, mode)
		args := []string{"-o", binary, "-graph", binary + ".dot"}
		if mode == "optimized" {
			args = append(args, "-optimize", "-optimization-report", binary+".json")
		}
		if mode == "disabled" {
			args = append(args, "-optimize=false")
		}
		args = append(args, "examples/quantified.lk")
		var out, errout bytes.Buffer
		if status := run(args, &out, &errout); status != 0 {
			t.Fatalf("%d %s", status, errout.String())
		}
		result, err := exec.Command(binary).CombinedOutput()
		if err != nil || string(result) != "328350\n1\n" {
			t.Fatalf("%s: %v %s", mode, err, result)
		}
		dot, err := os.ReadFile(binary + ".dot")
		if err != nil {
			t.Fatal(err)
		}
		if mode == "canonical" {
			baseline = dot
		} else if !bytes.Equal(baseline, dot) {
			t.Fatal("strategy mutated semantic graph")
		}
		if mode == "optimized" {
			data, err := os.ReadFile(binary + ".json")
			if err != nil {
				t.Fatal(err)
			}
			var report optimization.SpecializationResult
			if err := json.Unmarshal(data, &report); err != nil {
				t.Fatal(err)
			}
			found := 0
			for _, d := range report.Quantified {
				if d.Function == "calculate" {
					found++
					if d.GuardsEliminated != 1 || d.Bounds.Domain.Count.Max != "100" || d.Bounds.Proof.Status != constraints.Proven {
						t.Fatalf("%+v", d)
					}
					if d.Callback == "square" && d.Bounds.Elements.Max != "9801" {
						t.Fatal("wrong map bound")
					}
					if d.Callback == "add" && d.Bounds.Prefix.AllPrefixes.Max != "980100" {
						t.Fatal("wrong prefix invariant")
					}
				}
			}
			if found != 2 || report.Summary.Reused < 1 {
				t.Fatal("quantified proof diagnostics/reuse missing")
			}
		}
	}
}
