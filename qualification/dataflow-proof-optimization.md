# Proof-gated dataflow arithmetic — qualification

## Assessment

**PASS for the implemented unsigned callback specialization subset**, empirical
qualification, not formal verification or release authorization. The mechanism
and assumptions are in [dataflow-proof-optimization.md](../docs/dataflow-proof-optimization.md);
[domain/prefix qualification](quantified-range-analysis.md) covers the analyzer.
Baseline `7605b8f`, Linux amd64, Go 1.26.4, GCC 14.2.0, no new dependency/solver.

Canonical C remains checked and is reconstructible from the unchanged graph.
Quantified candidates use the existing registry, implication gate, cost model,
reuse, budgets and orphan/alias handling. The current stage's exact domain and
obligation ledger must bind each private variant. Unknown, empty, unsupported,
incomplete and cost-ineligible cases retain canonical callback targets/checks.
Fusion's legality/iteration strategy is unchanged.

## Structural and differential evidence

| Property | Executed test/evidence | Status |
|---|---|---|
| Real elimination, not comments/counters | `TestQuantifiedGuardEliminationStructure`: canonical AND optimized generic square/add bodies still call checked helpers; selected private bodies contain raw typed * / + instead; owning loop actually calls those private targets | TESTED |
| Partial optimization | Map proven at n<=65536 while add remains checked; near-MAX initial prevents additive safety; unknown upstream map with safe bounded downstream transfer remains supported | TESTED |
| Exact proof scope and reuse | Bounded/stronger domains reuse variants; unbounded and continuing paths cannot reuse them; scalar caller can share a valid formal variant; empty domains emit none | TESTED |
| Existing costs/budgets | Large proven callback deliberately retains its generic target because body cost is excessive; mixed scalar/quantified registry uses unchanged budget policy | TESTED |
| Pull and fused paths | Long canonical pull pipeline dispatches to proven variants without becoming fused; supported pipelines still fuse independently | TESTED |
| Canonical/deterministic preservation | Original C/DOT unchanged before/after analysis/optimized generation; repeat optimized C/JSON equal; CLI canonical/enabled/disabled outputs agree | TESTED |
| Runtime behavior/first failure/effects | Independent typed-AST reference versus canonical/optimized native execution at O0/O3, with exact function-argument/state traces, stderr/status/stdout | TESTED within stated domains |

Test code: `compiler/bin/optimization/quantified_test.go` and existing independent
`dataflow_oracle_test.go`; CLI report checks in `compiler/quantified_test.go`.

**199 new three-way native input comparisons** include:

* n = 0,1,2,99,100,101,2344,2345,65536,65537,MAX across bounded, partial,
  nonzero-initial, filtered, multi-pipeline and nonzero-start computations;
* bounded high-valued ranges that fail multiplication or reduction without
  enumerating enormous domains;
* a pure conditional cap summary supplying endpoint bounds;
* distinct opaque sequence-returning endpoint calls;
* setup/surrounding effects, earlier reducer failures, later division failures,
  and an impure surrounding scope with actual proven callback guard removal;
* an unfused long pipeline using the same valid callback variants.

Comparison includes successful values, stdout, exact arithmetic failure
category/location, exit status, and test-only source-function traces including
arguments/accumulator values. The independent AST interpreter does not consume
quantified proofs. Native tests use warnings as errors and UBSan; production
purity is not weakened by test instrumentation. Original 3,116 dataflow native
cases and scalar/native/implication/policy suites were rerun as regressions, not
assumed to cover the new feature.

Final 10-second `FuzzQuantifiedNative`: **8,690 executions**, no reported failure.
It samples bounded/unbounded branches, endpoints 0..102, arbitrary u32 initial
values, and a fixed two-element high-valued failing domain. Both safe private
variants and checked generic fallbacks are exercised. This is sampled testing.

## Counterexamples and mutation effectiveness

`qualification/quantified-mutations.py` reuses the isolated dataflow harness.
All injected changes are in disposable copies, with original Go-file fingerprints
verified unchanged. Build failures/timeouts are INCONCLUSIVE, not kills.

Initial targeted run: 13 killed, **2 survived**, 0 inconclusive. Investigation:
negative endpoint subtraction had no negative-end fixture, and the fact-leak test
checked only after a join that already discarded the leaked facts. Added signed
negative-end bounds and an unrelated scalar statement immediately after the
pipeline inside the same branch. These were real test-coverage gaps, not ignored
or relabeled kills. The subsequent baseline passed and both mutations were killed.

Review additionally found/repaired shared opaque endpoint identities, as detailed
in the analyzer report. Its regression was added as another mutation.

Final fresh run: **16 killed, 0 survived, 0 inconclusive**:

| Injected fault | Meaningful detector |
|---|---|
| Reuse opaque callee output identity between calls | Distinct endpoint/empty-domain regression |
| Include exclusive endpoint | Expected 99 versus 100 element bound |
| Ignore empty domain | Required EMPTY/no-element representation |
| Narrow/wrap endpoint subtraction | Negative end loses signed bound |
| Underestimate cardinality | Count/independent enumeration assertions |
| Underestimate mapped output | Actual value lies outside predicted interval |
| Assume arithmetic safe | Unknown/unsafe map and independent u8 oracle |
| Ignore initial value | Initial/prefix enclosure violated |
| Assume initial state at every step | Partial-reduction and actual prefix overflow checks |
| Assume filter halves count | Retained cardinality/prefix oracle |
| Drop path context | Motivating bound/proof assertions |
| Leak domain facts into scalar traversal | Tagged element facts found in unrelated scalar premise snapshot |
| Reuse unrelated domain's proof environment | Unbounded domain incorrectly selects a bounded variant |
| Install range-specific checks globally | Required generic checked body disappears |
| Admit UNKNOWN at proof authorization gates | Partial/unbounded fallback assertions |
| Never dispatch to private callback variants | Actual loop/helper target assertion, despite claimed proof/elision |

The previous **15 dataflow** and **22 scalar specialization/proof** mutations were
rerun against this implementation: all killed, no survived/inconclusive. Total
final selected mutations: **53/53 killed**. This demonstrates sensitivity to these
faults, not a general mutation score or universal soundness proof.

Final session logs:

* `/tmp/leek-quantified-final-mutations`
* `/tmp/leek-quantified-final-dataflow-mutations`
* `/tmp/leek-quantified-final-scalar-mutations`

## Inspected artifacts

`/tmp/leek-quantified-artifacts.3n3FWf` retains canonical/optimized C, semantic DOT,
analysis/optimization/plan JSON, O0/O3 UBSan binaries and outputs. The example
prints `328350` then `1` in both strategies. It emits two private variants, reuses
square for the partial case, leaves the unbounded callbacks checked, and retains
all generic definitions. Three dataflow pipelines fuse under the existing gate.
The API artifact tool and ordinary CLI were both executed; the default sample and
compiler build also succeeded with temporary output/graph paths.

## Optional measured performance experiment

Reproducible harness: `qualification/quantified-benchmark.go`. The same harness
runs from a disposable `git archive 7605b8f` compiler module for fusion-only output,
and from the new compiler for quantified output. No checkout/reset or unsafe
runtime path was used. Input varies over 0..100 for 200,000 kernel calls; checksum
is independently verified. Public entry is no-inline to avoid whole-program
constant folding, while GCC callback optimizations remain enabled.

Identical GCC flags per row: C11, warnings as errors, O0 or O3 (no march-native).
Three wall-clock trials including process startup; medians below. No concurrent
mutation/fuzz workload during measurement. Session artifacts/raw timings:
`/tmp/leek-quantified-benchmark.nUq6cd`.

| Strategy | C bytes | O0 binary bytes / median ms | O3 binary bytes / median ms |
|---|---:|---:|---:|
| Canonical pull (baseline) | 11,498 | 16,368 / 96.269 | 16,232 / 7.303 |
| Existing fusion only | 10,864 | 16,288 / 69.922 | 16,232 / 5.121 |
| Fusion + quantified variants | 11,839 | 16,360 / 39.002 | 16,192 / 2.365 |

Current canonical C is **byte-identical** to baseline canonical C in this harness;
its repeat medians were 93.796/7.282 ms. Structural hot-path guard calls per
retained element: 2 canonical, 2 fusion-only, 0 quantified. Generic definitions
still retain checks; these are not counts of helper definitions.

One-shot Go generation times (exclude parse/resolve/GCC): baseline canonical
0.107 ms, baseline fusion 0.221 ms, current canonical 0.209 ms, quantified optimized
0.252 ms. GCC compilation was roughly 30–41 ms (individual values in JSON).
These short, noisy local measurements are not a stable speedup guarantee. O0
helps distinguish Leek's own removal; O3 additionally includes GCC's optimizer.
No unsafe transformation was added for this benchmark. Code size grew at C/O0
because canonical bodies remain beside private variants.

## Final gates and limits

Full run: **716 passing test/subtest results**, vet and race checks passed; gofmt,
build, CLI/example, whitespace checks and canonical preservation passed. Bounded
analysis/native fuzzing reported no deviation. No applicable unresolved
counterexample remains in the implemented subset.

Signed/unary specialization, arbitrary recurrences, exported quantified function
contracts, SMT, SIMD, parallelism, speculative scheduling, arrays, JIT and runtime
deoptimization are not implemented. Platform/OS failures and arbitrary adversarial
compiler resource exhaustion are not qualified. Nothing was deployed or released.
