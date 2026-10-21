package main

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestSpecializationCLI(t *testing.T) {
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
		args = append(args, "examples/interprocedural.lk")
		var stdout, stderr bytes.Buffer
		if status := run(args, &stdout, &stderr); status != 0 {
			t.Fatalf("%s: %d: %s", mode, status, stderr.String())
		}
		if strings.Contains(stdout.String(), "specialization:") != (mode == "enabled") {
			t.Fatal(stdout.String())
		}
		out, err := exec.Command(binary).CombinedOutput()
		if err != nil || string(out) != "12\n1\n6\n0\n0\n" {
			t.Fatalf("%s: %v %s", mode, err, out)
		}
		dot, err := os.ReadFile(binary + ".dot")
		if err != nil {
			t.Fatal(err)
		}
		if mode == "default" {
			canonicalDOT = dot
		} else if !bytes.Equal(dot, canonicalDOT) {
			t.Fatal("specialization modified semantic graph")
		}
	}
}
