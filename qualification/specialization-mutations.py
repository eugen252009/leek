#!/usr/bin/env python3
"""Bounded isolated mutation qualification; run with the intended Go on PATH.

Each mutant is applied to a disposable copy, never to the working compiler.
Logs and machine-readable results are retained in the supplied output directory.
Compilation failures and timeouts are INCONCLUSIVE, never counted as kills.
"""
import hashlib
import json
from pathlib import Path
import shutil
import subprocess
import sys
import tempfile

ROOT = Path(__file__).resolve().parents[1]
OUTPUT = Path(sys.argv[1]).resolve() if len(sys.argv) > 1 else Path(tempfile.mkdtemp(prefix="leek-specialization-mutations-"))
OUTPUT.mkdir(parents=True, exist_ok=True)
TESTS = "TestSpecialization|TestSpecialized|TestReusableSummary|TestInterproceduralProof"
SPECIALIZER = "bin/optimization/specialization.go"
ANALYZER = "bin/constraints/analyze.go"
BACKEND = "bin/parser/codegen.go"
MUTATIONS = [
    ("unknown-as-proven", SPECIALIZER, "o.Proof.Status != constraints.Proven", "o.Proof.Status == constraints.Violated"),
    ("caller-facts-global", SPECIALIZER, "current.checks[n] = true", "current.checks[n] = true\nlocal[n] = true"),
    ("omit-callee-precondition", ANALYZER, "a.substitute(o.Required, mapping, memo)", "yes"),
    ("reverse-parameter-substitution", ANALYZER, "mapping[p] = args[i]", "mapping[p] = args[len(args)-1-i]"),
    ("reverse-required-comparison", ANALYZER, 'a.op(">=", x, y)', 'a.op("<=", x, y)'),
    ("leak-exclusive-branch", "bin/constraints/paths.go", "s.context = joinContexts(leftContext, s.context)", "s.context = leftContext"),
    ("drop-unproven-division-guard", BACKEND, "cg.proven[info.Node] &&", '(cg.proven[info.Node] || e.Name == "/") &&'),
    ("reuse-wrong-variant", SPECIALIZER, "v.Function == c.f && sameMap(v.Proven, proven) && sameMap(v.Routes, routes)", "v.Function == c.f"),
    ("route-to-canonical", BACKEND, "target = name", "target = info.Callee.CName"),
    ("reverse-argument-evaluation", BACKEND, "for _, arg := range e.Args {\n\t\t\tvalues = append(values, cg.expr(arg))\n\t\t}", "values = append(values, make([]string,len(e.Args))...)\nfor i:=len(e.Args)-1;i>=0;i-- { values[len(values)-len(e.Args)+i]=cg.expr(e.Args[i]) }"),
    ("never-specialize", SPECIALIZER, "if !report.Limited {", "if false {"),
]


def fingerprints():
    return {str(p.relative_to(ROOT / "compiler")): hashlib.sha256(p.read_bytes()).hexdigest()
            for p in (ROOT / "compiler").rglob("*.go")}


def run(name, mutation=None):
    with tempfile.TemporaryDirectory(prefix="leek-mutant-") as directory:
        checkout = Path(directory) / "compiler"
        shutil.copytree(ROOT / "compiler", checkout, ignore=shutil.ignore_patterns("app", "*_tmp.c"))
        if mutation:
            relative, before, after = mutation
            path = checkout / relative
            text = path.read_text()
            if text.count(before) != 1:
                raise RuntimeError(f"{name}: expected one mutation target, found {text.count(before)}")
            path.write_text(text.replace(before, after))
        try:
            result = subprocess.run(["go", "test", "-count=1", "./bin/constraints", "./bin/optimization", "-run", TESTS],
                                    cwd=checkout, stdout=subprocess.PIPE, stderr=subprocess.STDOUT, text=True, timeout=60)
            log = result.stdout
            if result.returncode == 0:
                status = "PASS" if mutation is None else "SURVIVED"
            elif "--- FAIL: Test" in log and "[build failed]" not in log:
                status = "KILLED" if mutation else "BASELINE_FAILED"
            else:
                status = "INCONCLUSIVE"
        except subprocess.TimeoutExpired as error:
            log = str(error)
            status = "INCONCLUSIVE"
        (OUTPUT / f"{name}.log").write_text(log)
        print(f"{name}: {status}", flush=True)
        return {"name": name, "status": status}


original = fingerprints()
results = [run("baseline")]
if results[0]["status"] != "PASS":
    raise SystemExit("Baseline must pass before testing mutations")
for name, relative, before, after in MUTATIONS:
    results.append(run(name, (relative, before, after)))
assert original == fingerprints(), "Working compiler files changed during qualification"
(OUTPUT / "results.json").write_text(json.dumps(results, indent=2) + "\n")
print(f"Evidence: {OUTPUT}")
raise SystemExit(0 if all(r["status"] in ("PASS", "KILLED") for r in results) else 1)
