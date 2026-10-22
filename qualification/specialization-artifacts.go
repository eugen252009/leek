// Run from compiler/: go run ../qualification/specialization-artifacts.go <output-directory>
// This harness uses compiler APIs to retain lowered C and proof evidence without
// adding a source-language C-emission option or weakening the proof boundary.
package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"leek/bin/optimization"
	"leek/bin/parser"
)

func check(err error) {
	if err != nil {
		panic(err)
	}
}
func main() {
	if len(os.Args) < 2 || len(os.Args) > 3 {
		panic("usage: specialization-artifacts <output-directory> [source.lk]; run from compiler/")
	}
	dir, err := filepath.Abs(os.Args[1])
	check(err)
	check(os.MkdirAll(dir, 0755))
	filename := "examples/interprocedural.lk"
	if len(os.Args) == 3 {
		filename = os.Args[2]
	}
	source, err := os.ReadFile(filename)
	check(err)
	ast, err := parser.NewParser(strings.NewReader(string(source)), filename).ParseProgram()
	check(err)
	g, err := parser.NewResolver().Resolve(ast)
	check(err)
	canonical, err := parser.NewCodeGenerator().Generate(g)
	check(err)
	dot := g.ToDOT()
	r, err := optimization.GenerateSpecialized(g)
	check(err)
	after, err := parser.NewCodeGenerator().Generate(g)
	check(err)
	fused := 0
	for _, d := range r.Dataflow {
		if d.Decision == "FUSED" {
			fused++
		}
	}
	if canonical != after || dot != g.ToDOT() || (len(r.Variants) == 0 && fused == 0) {
		panic("IR preservation or actual specialization/fusion failed")
	}
	check(os.WriteFile(filepath.Join(dir, "canonical.c"), []byte(canonical), 0644))
	check(os.WriteFile(filepath.Join(dir, "optimized.c"), []byte(r.Code), 0644))
	check(os.WriteFile(filepath.Join(dir, "semantic.dot"), []byte(dot), 0644))
	for name, value := range map[string]any{"analysis": r.Analysis, "policy": r, "plan": map[string]any{"local_elisions": r.LocalElisions, "variants": r.Variants, "eligible_witnesses": r.Witnesses, "dataflow": r.Dataflow}} {
		data, err := json.MarshalIndent(value, "", "  ")
		check(err)
		check(os.WriteFile(filepath.Join(dir, name+".json"), append(data, '\n'), 0644))
	}
	for _, opt := range []string{"-O0", "-O3"} {
		var expected []byte
		for _, mode := range []string{"canonical", "optimized"} {
			binary := filepath.Join(dir, mode+opt)
			args := []string{"-std=c11", "-Wall", "-Wextra", "-Werror", "-Wno-unused-parameter", opt, "-fsanitize=undefined", "-fno-sanitize-recover=all", filepath.Join(dir, mode+".c"), "-o", binary}
			if out, err := exec.Command("gcc", args...).CombinedOutput(); err != nil {
				panic(fmt.Sprintf("GCC: %v: %s", err, out))
			}
			out, err := exec.Command(binary).CombinedOutput()
			check(err)
			if len(os.Args) == 2 && string(out) != "12\n1\n6\n0\n0\n" {
				panic(fmt.Sprintf("bad output: %q", out))
			}
			if mode == "canonical" {
				expected = out
			} else if !bytes.Equal(expected, out) {
				panic("differential mismatch")
			}
			check(os.WriteFile(binary+".output", out, 0644))
		}
	}
	fingerprints := fmt.Sprintf("%x  canonical.c\n%x  optimized.c\n", sha256.Sum256([]byte(canonical)), sha256.Sum256([]byte(r.Code)))
	check(os.WriteFile(filepath.Join(dir, "SHA256SUMS"), []byte(fingerprints), 0644))
	fmt.Printf("%d emitted variants; %d fused pipelines; C/DOT preservation and O0/O3 UBSan outputs verified. Evidence: %s\n", len(r.Variants), fused, dir)
}
