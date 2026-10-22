// Run from compiler/: go run ../qualification/quantified-benchmark.go <directory> <tag>
// Deliberately uses APIs shared with 7605b8f so the same harness can measure
// baseline fusion-only and quantified optimization in disposable module copies.
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"leek/bin/optimization"
	"leek/bin/parser"
)

const source = `fn square(x u32) u32 { return x*x }
fn add(a u32,b u32) u32 { return a+b }
fn calculate(n u32) u32 { if n<=100 { return range(0,n) -> map(square) -> reduce(0,add) } return 0 }`

type measurement struct {
	Mode, GCC                      string
	CodeBytes, BinaryBytes         int
	GenerationNS, GCCCompilationNS int64
	ExecutionNS                    []int64
	Checksum                       string
}

func checked(err error) {
	if err != nil {
		panic(err)
	}
}
func main() {
	if len(os.Args) != 3 {
		panic("usage: quantified-benchmark <directory> <tag>")
	}
	dir, err := filepath.Abs(os.Args[1])
	checked(err)
	checked(os.MkdirAll(dir, 0755))
	tag := os.Args[2]
	ast, err := parser.NewParser(strings.NewReader(source), "benchmark.lk").ParseProgram()
	checked(err)
	g, err := parser.NewResolver().Resolve(ast)
	checked(err)
	started := time.Now()
	canonical, err := parser.NewCodeGenerator().Generate(g)
	checked(err)
	canonicalNS := time.Since(started).Nanoseconds()
	started = time.Now()
	result, err := optimization.GenerateSpecialized(g)
	checked(err)
	optimizedNS := time.Since(started).Nanoseconds()
	checksum := uint64(0)
	for r := 0; r < 200000; r++ {
		n := (100 + r) % 101
		sum := uint64(0)
		for i := 0; i < n; i++ {
			sum += uint64(i * i)
		}
		checksum += sum
	}
	expected := strconv.FormatUint(checksum, 10) + "\n"
	cname := g.Functions["calculate"].CName
	harness := fmt.Sprintf("\nint main(void) { uint64_t checksum=0; volatile uint32_t input=100; for(uint32_t r=0;r<200000;++r) checksum += %s((input+r)%%101); printf(\"%%\" PRIu64 \"\\n\",checksum);return 0;}\n", cname)
	rows := []measurement{}
	for _, variant := range []struct {
		name, code string
		duration   int64
	}{{"canonical", canonical, canonicalNS}, {"optimized", result.Code, optimizedNS}} {
		// No-inline on the public entry keeps this a repeated-kernel benchmark;
		// callback inlining and GCC's own proof optimizations remain enabled.
		code := strings.Replace(variant.code, "\nuint32_t "+cname+"(", "\n__attribute__((noinline)) uint32_t "+cname+"(", 1) + harness
		src := filepath.Join(dir, tag+"-"+variant.name+".c")
		checked(os.WriteFile(src, []byte(code), 0600))
		for _, opt := range []string{"-O0", "-O3"} {
			binary := filepath.Join(dir, tag+"-"+variant.name+opt)
			start := time.Now()
			output, err := exec.Command("gcc", "-std=c11", "-Wall", "-Wextra", "-Werror", "-Wno-unused-parameter", opt, src, "-o", binary).CombinedOutput()
			if err != nil {
				panic(fmt.Sprintf("GCC: %v %s", err, output))
			}
			compileNS := time.Since(start).Nanoseconds()
			stat, err := os.Stat(binary)
			checked(err)
			row := measurement{Mode: tag + "-" + variant.name, GCC: opt, CodeBytes: len(code), BinaryBytes: int(stat.Size()), GenerationNS: variant.duration, GCCCompilationNS: compileNS, Checksum: expected}
			for trial := 0; trial < 3; trial++ {
				start := time.Now()
				out, err := exec.Command(binary).CombinedOutput()
				elapsed := time.Since(start).Nanoseconds()
				checked(err)
				if string(out) != expected {
					panic("benchmark checksum mismatch")
				}
				row.ExecutionNS = append(row.ExecutionNS, elapsed)
			}
			rows = append(rows, row)
		}
	}
	data, err := json.MarshalIndent(rows, "", "  ")
	checked(err)
	checked(os.WriteFile(filepath.Join(dir, tag+".json"), append(data, '\n'), 0600))
	fmt.Println(string(data))
}
