package main

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestCompositionExamples(t *testing.T) {
	for _, tt := range []struct{ file, stdout, diagnostic string }{
		{"composition.lk", "200\n", ""},
		{"effectful-source.lk", "10\n2\n102\n", ""},
		{"invalid-impure-stage.lk", "", "only accepts pure stages"},
		{"invalid-transitive-effect.lk", "", "only accepts pure stages"},
	} {
		t.Run(tt.file, func(t *testing.T) {
			dir := t.TempDir()
			binary := filepath.Join(dir, "app")
			dot := filepath.Join(dir, "graph.dot")
			var out, errout bytes.Buffer
			status := run([]string{"-o", binary, "-graph", dot, filepath.Join("examples", tt.file)}, &out, &errout)
			if tt.diagnostic != "" {
				if status != 1 || !strings.Contains(errout.String(), tt.diagnostic) || !strings.Contains(errout.String(), tt.file+":") {
					t.Fatalf("status %d: %s", status, errout.String())
				}
				for _, path := range []string{binary, dot} {
					if _, err := os.Stat(path); !os.IsNotExist(err) {
						t.Errorf("unexpected artifact %s: %v", path, err)
					}
				}
				return
			}
			if status != 0 {
				t.Fatalf("status %d: %s", status, errout.String())
			}
			got, err := exec.Command(binary).CombinedOutput()
			if err != nil || string(got) != tt.stdout {
				t.Fatalf("execution: %v, %q; want %q", err, got, tt.stdout)
			}
			if _, err := os.Stat(binary + "_tmp.c"); !os.IsNotExist(err) {
				t.Fatalf("temporary C file not cleaned up: %v", err)
			}
		})
	}
}
