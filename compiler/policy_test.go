package main

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"leek/bin/optimization"
)

func TestPolicyMachineReadableCLI(t *testing.T) {
	dir := t.TempDir()
	binary := filepath.Join(dir, "app")
	report := filepath.Join(dir, "policy.json")
	var stdout, stderr bytes.Buffer
	args := []string{"-optimize", "-optimization-report", report, "-o", binary, "-graph", binary + ".dot", "examples/specialization-policy.lk"}
	if status := run(args, &stdout, &stderr); status != 0 {
		t.Fatalf("%d %s", status, stderr.String())
	}
	data, err := os.ReadFile(report)
	if err != nil {
		t.Fatal(err)
	}
	var result optimization.SpecializationResult
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatal(err)
	}
	if result.Code != "" || result.Summary.Generated == 0 || result.Summary.Reused == 0 || !strings.Contains(stdout.String(), "specialization policy:") {
		t.Fatal("missing real decisions", stdout.String())
	}
	found := false
	for _, d := range result.Decisions {
		if d.Caller == "c_strong" && len(d.Path) == 1 {
			found = true
			if d.Action != "REUSE" || d.Selected != d.FinalTarget || len(d.Considered) != 2 {
				t.Fatal(d)
			}
		}
	}
	if !found {
		t.Fatal("missing selection trace")
	}
	out, err := exec.Command(binary).CombinedOutput()
	if err != nil || string(out) != "2\n0\n10\n0\n2\n" {
		t.Fatalf("%v %s", err, out)
	}
}

func TestPolicyReportRequiresOptimizationAndInspectionIsReadOnly(t *testing.T) {
	dir := t.TempDir()
	report := filepath.Join(dir, "policy.json")
	var stdout, stderr bytes.Buffer
	if status := run([]string{"-optimization-report", report, "examples/specialization-policy.lk"}, &stdout, &stderr); status != 2 {
		t.Fatalf("report silently enabled optimization: %d %s", status, stderr.String())
	}
	stdout.Reset()
	stderr.Reset()
	t.Setenv("PATH", t.TempDir())
	if status := run([]string{"-constraints", "-optimize", "-optimization-report", report, "examples/specialization-policy.lk"}, &stdout, &stderr); status != 0 {
		t.Fatalf("inspection required GCC: %d %s", status, stderr.String())
	}
	if _, err := os.Stat(report); !os.IsNotExist(err) {
		t.Fatal("inspection wrote an optimization report")
	}
}
