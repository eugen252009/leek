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
TESTS = "TestSpecialization|TestSpecialized|TestReusableSummary|TestInterproceduralProof|TestConstraintImplication|TestImplication|TestPolicy"
SPECIALIZER = "bin/optimization/specialization.go"
ANALYZER = "bin/constraints/analyze.go"
BACKEND = "bin/parser/codegen.go"
POLICY = "bin/optimization/policy.go"
IMPLICATION = "bin/constraints/implication.go"
MUTATIONS = [
    ("unknown-as-proven", POLICY, "if proof.Status != constraints.Proven || !proof.Consistent", "if proof.Status == constraints.Violated || !proof.Consistent"),
    ("caller-facts-global", SPECIALIZER, "current.checks[n] = true", "current.checks[n] = true\nlocal[n] = true"),
    ("omit-callee-precondition", ANALYZER, "a.substitute(o.Required, mapping, memo)", "yes"),
    ("reverse-parameter-substitution", ANALYZER, "mapping[p] = args[i]", "mapping[p] = args[len(args)-1-i]"),
    ("reverse-required-comparison", ANALYZER, 'a.op(">=", x, y)', 'a.op("<=", x, y)'),
    ("leak-exclusive-branch", "bin/constraints/paths.go", "s.context = joinContexts(leftContext, s.context)", "s.context = leftContext"),
    ("drop-unproven-division-guard", BACKEND, "cg.proven[info.Node] &&", '(cg.proven[info.Node] || e.Name == "/") &&'),
    ("reuse-wrong-variant", POLICY, "if proof.Status == constraints.Proven && proof.Consistent && better(b.records[v.Name], best)", "if better(b.records[v.Name], best)"),
    ("route-to-canonical", BACKEND, "target = name", "target = info.Callee.CName"),
    ("reverse-argument-evaluation", BACKEND, "for _, arg := range e.Args {\n\t\t\tvalues = append(values, cg.expr(arg))\n\t\t}", "values = append(values, make([]string,len(e.Args))...)\nfor i:=len(e.Args)-1;i>=0;i-- { values[len(values)-len(e.Args)+i]=cg.expr(e.Args[i]) }"),
    ("never-specialize", SPECIALIZER, "if !report.Limited {", "if false {"),
    ("reverse-implication-direction", IMPLICATION, "fwd, rev := e.a.prove(right, l), e.a.prove(left, r)", "fwd, rev := e.a.prove(left, r), e.a.prove(right, l)"),
    ("incorrect-integer-normalization", IMPLICATION, "r.lo.Add(r.lo, big.NewInt(1))", "r.lo.Add(r.lo, big.NewInt(2))"),
    ("ignore-parameter-binding", POLICY, "if !found {\n\t\t\treturn unknown\n\t\t}", "if !found { index=0 }"),
    ("ignore-arithmetic-semantics", POLICY, "record.Semantics != checkedUnsignedSemantics", "false"),
    ("reuse-foreign-body", POLICY, "if v.Function != c.f || record == nil", "if false || record == nil"),
    ("ignore-all-budgets", POLICY, "if b.counts[c.f] >= maxVariantsPerFunction || len(b.variants) >= maxVariants || b.units+d.Cost.BodyUnits > maxAdditionalUnits", "if false"),
    ("duplicate-equivalent-variants", POLICY, "benefit := d.Cost.RemovedGuards", "best=nil\nbenefit := d.Cost.RemovedGuards"),
    ("select-inferior-applicable-variant", POLICY, "a.Cost.RemovedGuards > b.Cost.RemovedGuards", "a.Cost.RemovedGuards < b.Cost.RemovedGuards"),
    ("ignore-benefit-cost", POLICY, "if d.Cost.BodyUnits > maxBodyUnits || d.Cost.BodyUnits > benefit*unitsPerAdditionalGuard", "if false"),
    ("ignore-additional-code-budget", POLICY, " || b.units+d.Cost.BodyUnits > maxAdditionalUnits", ""),
    ("assume-required-predicate", IMPLICATION, 'proof := e.a.prove(e.a.op("and", required...), c)', 'c=e.a.refine(c,e.a.op("and",required...))\nproof := e.a.prove(e.a.op("and", required...), c)'),
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
