#!/usr/bin/env python3
"""Isolated dataflow qualification. Mirrors specialization-mutations.py.

One injected defect per temporary compiler; compilation failures/timeouts are
INCONCLUSIVE, never kills. Original source fingerprints must remain unchanged.
"""
import hashlib
import json
from pathlib import Path
import shutil
import subprocess
import sys
import tempfile

ROOT = Path(__file__).resolve().parents[1]
OUTPUT = Path(sys.argv[1]).resolve() if len(sys.argv) > 1 else Path(tempfile.mkdtemp(prefix="leek-dataflow-mutations-"))
OUTPUT.mkdir(parents=True, exist_ok=True)
BACKEND = "bin/parser/dataflow_codegen.go"
PARSER = "bin/parser/dataflow.go"
FUSION = "bin/optimization/fusion.go"
MUTATIONS = [
    ("inclusive-range-end", "./bin/parser", "TestDataflowCanonical", [
        (BACKEND, 'if (s->current >= s->end) return 0;', 'if (s->current > s->end) return 0;')]),
    ("wrong-initial-empty", "./bin/parser", "TestDataflowCanonical", [
        (BACKEND, 'acc := cg.temp(terminal.StateType, initial)\n\telement :=', 'acc := cg.temp(terminal.StateType, "0"); _ = initial\n\telement :=')]),
    ("filter-polarity", "./bin/parser", "TestDataflowCanonical", [
        (BACKEND, 'if (%s(element)) { *out = element; return 1; }', 'if (!%s(element)) { *out = element; return 1; }')]),
    ("skip-map", "./bin/parser", "TestDataflowCanonical", [
        (BACKEND, '*out = %s(element);', '*out = element; /* skipped %s */')]),
    ("wrong-accumulator", "./bin/parser", "TestDataflowCanonical", [
        (BACKEND, 'cg.indent += "    "\n\tcg.line("%s = %s(%s, %s);", acc, cg.callback(e), acc, element)', 'cg.indent += "    "\n\tcg.line("%s = %s(%s, %s);", acc, cg.callback(e), element, element)')]),
    ("reverse-fused-stage-order", "./bin/optimization", "TestDataflowReferenceDifferential", [
        (BACKEND, 'for _, stage := range chain[1 : len(chain)-1] {', 'for i:=len(chain)-2; i>0; i-- { stage:=chain[i]')]),
    ("reverse-fused-element-order", "./bin/optimization", "TestDataflowReferenceDifferential", [
        (BACKEND, 'cg.line("for (%s %s = %s; %s < %s; ++%s) {", cg.mapType(domain.ElementType), index, start, index, end, index)', 'cg.line("for (%s %s = %s; %s > %s; ) { --%s;", cg.mapType(domain.ElementType), index, end, index, start, index)')]),
    ("wrong-range-enumeration", "./bin/parser", "TestDataflowCanonical", [
        (BACKEND, '*out = s->current;', '*out = s->end - 1;')]),
    ("drop-callback-overflow-guard", "./bin/optimization", "TestDataflowReferenceDifferential", [
        ("bin/parser/codegen.go", 'cg.proven[info.Node] &&', '(cg.proven[info.Node] || e.Name == "*") &&')]),
    ("fuse-without-legality", "./bin/optimization", "TestFusionCanonicalFallback", [
        (FUSION, 'if proof.Status == constraints.Proven && proof.Consistent {', 'if true {')]),
    ("never-fuse-lowering", "./bin/optimization", "TestFusionStructure|TestFilterLoopFusionStructure", [
        (FUSION, 'plan[g.Expressions[e].Node] = true', 'plan[g.Expressions[e].Node] = false')]),
    ("retain-intermediate-array", "./bin/optimization", "TestFusionStructure", [
        (BACKEND, 'initial := cg.expr(terminal.Initial)\n\tacc := cg.temp(terminal.StateType, initial)\n\tindex :=', 'cg.line("uint32_t lk_materialized[1] = {0}; (void)lk_materialized;")\n\tinitial := cg.expr(terminal.Initial)\n\tacc := cg.temp(terminal.StateType, initial)\n\tindex :=')]),
    ("accept-wrong-map-input-type", "./bin/parser", "TestDataflowSemanticRejections", [
        (PARSER, 'len(params) != 1 || params[0].Type != d.InputType || !validType(callback.Decl.ReturnType)', 'len(params) != 1 || !validType(callback.Decl.ReturnType)')]),
    ("accept-effectful-callback", "./bin/parser", "TestDataflowSemanticRejections", [
        (PARSER, 'callback.Builtin || callback.Decl.IsImpure', 'callback.Builtin'),
        ("bin/parser/resolver.go", 'if callback.Effects != EffectNone {', 'if false {')]),
    ("drop-quantified-obligations", "./bin/constraints", "TestDataflowConstraints", [
        ("bin/constraints/dataflow.go", 'for _, o := range summary.Obligations {', 'for _, o := range summary.Obligations[:0] {')]),
]


def fingerprints():
    return {str(p.relative_to(ROOT / "compiler")): hashlib.sha256(p.read_bytes()).hexdigest()
            for p in (ROOT / "compiler").rglob("*.go")}


def run(name, package, tests, changes):
    with tempfile.TemporaryDirectory(prefix="leek-dataflow-mutant-") as directory:
        checkout = Path(directory) / "compiler"
        shutil.copytree(ROOT / "compiler", checkout, ignore=shutil.ignore_patterns("app", "*_tmp.c"))
        for relative, before, after in changes:
            path = checkout / relative
            text = path.read_text()
            if text.count(before) != 1:
                raise RuntimeError(f"{name}: expected one mutation target, found {text.count(before)}")
            path.write_text(text.replace(before, after))
        try:
            result = subprocess.run(["go", "test", "-count=1", package, "-run", tests], cwd=checkout,
                                    stdout=subprocess.PIPE, stderr=subprocess.STDOUT, text=True, timeout=90)
            log = result.stdout
            if result.returncode == 0:
                status = "PASS" if not changes else "SURVIVED"
            elif "--- FAIL: Test" in log and "[build failed]" not in log:
                status = "KILLED" if changes else "BASELINE_FAILED"
            else:
                status = "INCONCLUSIVE"
        except subprocess.TimeoutExpired as error:
            log, status = str(error), "INCONCLUSIVE"
        (OUTPUT / f"{name}.log").write_text(log)
        print(f"{name}: {status}", flush=True)
        return {"name": name, "status": status}


if __name__ == "__main__":
    original = fingerprints()
    results = [run("baseline", "./...", "TestDataflow|TestFusion|TestFilterLoopFusionStructure", [])]
    if results[0]["status"] != "PASS":
        raise SystemExit("Baseline must pass")
    for name, package, tests, changes in MUTATIONS:
        results.append(run(name, package, tests, changes))
    assert original == fingerprints(), "Working compiler changed"
    (OUTPUT / "results.json").write_text(json.dumps(results, indent=2) + "\n")
    print(f"Evidence: {OUTPUT}")
    raise SystemExit(0 if all(r["status"] in ("PASS", "KILLED") for r in results) else 1)
