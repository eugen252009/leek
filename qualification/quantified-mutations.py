#!/usr/bin/env python3
"""Quantified proof/consumer faults, reusing the isolated dataflow harness."""
import json
from pathlib import Path
import runpy

library = runpy.run_path(str(Path(__file__).with_name("dataflow-mutations.py")), run_name="mutation_library")
run, fingerprints, OUTPUT = library["run"], library["fingerprints"], library["OUTPUT"]
ANALYSIS = "bin/constraints/quantified.go"
CONSUMER = "bin/optimization/quantified.go"
MUTATIONS = [
    ("shared-opaque-endpoint-identity", "./bin/constraints", "TestQuantifiedIncompleteEndpointIdentity", [
        ("bin/constraints/analyze.go", 'if !summary.Complete && info.Type != "" {', 'if false {')]),
    ("exclusive-end-included", "./bin/constraints", "TestQuantifiedMotivatingBounds|TestQuantifiedDomainEdges", [
        (ANALYSIS, 'new(big.Int).Sub(end.hi, big.NewInt(1))', 'new(big.Int).Set(end.hi)')]),
    ("ignore-empty-domain", "./bin/constraints", "TestQuantifiedDomainEdges", [
        (ANALYSIS, 'if maxCount.Sign() == 0 {', 'if false {')]),
    ("wrapping-end-subtraction", "./bin/constraints", "TestQuantifiedDomainEdges", [
        (ANALYSIS, 'new(big.Int).Sub(end.hi, big.NewInt(1))', 'new(big.Int).SetUint64(end.hi.Uint64()-1)')]),
    ("underestimate-cardinality", "./bin/constraints", "TestQuantifiedMotivatingBounds|TestQuantifiedBoundsExhaustiveSmallDomains", [
        (ANALYSIS, 'domain.Count = interval(span{minCount, maxCount})', 'maxCount=nonnegative(new(big.Int).Sub(maxCount,big.NewInt(1)))\n domain.Count = interval(span{minCount, maxCount})')]),
    ("underestimate-map-output", "./bin/constraints", "TestQuantifiedMotivatingBounds|TestQuantifiedBoundsExhaustiveSmallDomains", [
        (ANALYSIS, 'q.Elements = interval(*out)', 'out.hi.Quo(out.hi,big.NewInt(2)); q.Elements = interval(*out)')]),
    ("assume-arithmetic-safe", "./bin/constraints", "TestQuantifiedPartialFiltersAndScope|TestQuantifiedBoundsExhaustiveSmallDomains", [
        (ANALYSIS, 'o.Proof = a.prove(o.Required, premises)', 'o.Proof = Proof{Proven,"mutated arithmetic assumption",true}')]),
    ("ignore-reduction-initial", "./bin/constraints", "TestQuantifiedBoundsExhaustiveSmallDomains", [
        (ANALYSIS, 'new(big.Int).Add(state.hi, new(big.Int).Mul(count.hi, input.hi))', 'new(big.Int).Mul(count.hi, input.hi)')]),
    ("ignore-intermediate-prefixes", "./bin/constraints", "TestQuantifiedPartialFiltersAndScope|TestQuantifiedBoundsExhaustiveSmallDomains", [
        (ANALYSIS, 'previous := span{new(big.Int).Set(state.lo), new(big.Int).Set(beforeMax)}', 'previous := span{new(big.Int).Set(state.lo), new(big.Int).Set(state.hi)}; _=beforeMax')]),
    ("filter-always-halves-count", "./bin/constraints", "TestQuantifiedBoundsExhaustiveSmallDomains", [
        (ANALYSIS, 'q.Count = interval(span{new(big.Int), new(big.Int).Set(count.hi)})', 'q.Count = interval(span{new(big.Int),new(big.Int).Quo(count.hi,big.NewInt(2))})')]),
    ("drop-path-context", "./bin/constraints", "TestQuantifiedMotivatingBounds", [
        (ANALYSIS, 'c := a.refine(s.context, r.Guard)', 'c := newContext()')]),
    ("leak-domain-facts-to-scalar-context", "./bin/constraints", "TestQuantifiedDeterministicReadOnly", [
        (ANALYSIS, 'c.put(incoming.Value, copySpan(*input))', 'c.put(incoming.Value, copySpan(*input)); s.context.put(incoming.Value,copySpan(*input))')]),
    ("reuse-unrelated-domain-environment", "./bin/optimization", "TestQuantifiedPartialReuseAndScope", [
        ("bin/optimization/policy.go", 'o := ledger[index]', 'o := ledger[0]')]),
    ("range-proof-made-global", "./bin/optimization", "TestQuantifiedGuardEliminationStructure|TestQuantifiedPartialReuseAndScope", [
        (CONSUMER, 'current.checks[n] = true', 'current.checks[n] = true; local[n]=true')]),
    ("unknown-authorizes-check-removal", "./bin/optimization", "TestQuantifiedPartialReuseAndScope", [
        (CONSUMER, 'o.Proof.Status == constraints.Proven && o.Proof.Consistent && proof.Proof.Status == constraints.Proven && proof.Proof.Consistent', 'o.Proof.Status != constraints.Violated && o.Proof.Consistent && proof.Proof.Status != constraints.Violated && proof.Proof.Consistent'),
        ("bin/optimization/policy.go", 'if proof.Status != constraints.Proven || !proof.Consistent {', 'if proof.Status == constraints.Violated || !proof.Consistent {')]),
    ("never-dispatch-to-quantified-variant", "./bin/optimization", "TestQuantifiedGuardEliminationStructure|TestQuantifiedCanonicalPullRoutes", [
        ("bin/parser/dataflow_codegen.go", 'return route', 'return info.Dataflow.Callback.CName')]),
]
original = fingerprints()
results = [run("baseline", "./...", "TestQuantified", [])]
if results[0]["status"] != "PASS":
    raise SystemExit("Baseline must pass")
for name, package, tests, changes in MUTATIONS:
    results.append(run(name, package, tests, changes))
assert original == fingerprints(), "Working compiler changed"
(OUTPUT / "results.json").write_text(json.dumps(results, indent=2) + "\n")
print(f"Evidence: {OUTPUT}")
raise SystemExit(0 if all(r["status"] in ("PASS", "KILLED") for r in results) else 1)
