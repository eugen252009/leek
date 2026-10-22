# Proof-safe scalar fusion — qualification

## Assessment and authority

**PASS for the implemented, bounded structural fusion patterns**, with empirical
implementation evidence plus the trace-preservation construction argument in
[loop-fusion.md](../docs/loop-fusion.md). Not a formal compiler proof or claim of
all-program correctness. Canonical execution remains the default.

Same baseline/environment as [declarative dataflow qualification](declarative-dataflow.md):
`9f2251a`, Linux amd64, Go 1.26.4, GCC 14.2.0. Stage-order semantics were specified
and canonical execution qualified before the fusion pass. No existing scalar
failure, effect, implication, or specialization guarantee was weakened.

## Legality and independent evidence

The new legality obligation is equality of the setup/element/callback/state trace,
not universal arithmetic safety. The gate verifies exact semantic identities,
Range source, Map/Filter chain, Reduce terminal, compatible types, pure callbacks,
element-major version, no materialization requirement, <=8 total stages, and no
analysis resource exhaustion. UNKNOWN keeps canonical lowering. Fold, bare
Range→Reduce, and longer chains are deliberately not fused.

An eligible direct loop preserves the same calls/checks, order, filter skips,
initial value, and accumulator dependency as the pull interpreter. An infallible
private cursor increment is the only advance placement difference. No arithmetic
obligation is assumed and no failing call is speculated. The existing scalar
proof/specialization pass can independently remove proven scalar checks; fusion
does not authorize that removal. Source/graph immutability during fresh planning
and lowering and normal target operation remain assumptions.

| Contract | Actual executed checks | Qualification |
|---|---|---|
| DF-FUSION legality | `TestFusionLegalityFailsClosed`: incompatible order, effectful callback, materialization contract, missing graph, resource-limit fallback | TESTED |
| Actual transformation | `TestFusionStructure`: canonical next/helper/state chain becomes a direct loop; no iterator helpers, arrays or malloc in eligible body; existing unknown square/add checks remain | TESTED |
| Real producer/consumer loop fusion | `TestFilterLoopFusionStructure`: canonical filter producer while-loop plus terminal consumer while-loop become exactly one for-loop, no while/iterator chain | TESTED |
| Honest fallback/diagnostics | `TestFusionCanonicalFallback`: Fold, long chain, bare reduction remain canonical; JSON decisions correspond to lowering; CLI example has 2 fused pipelines/1 canonical Fold | TESTED |
| Canonical preservation/determinism | Original C/DOT unchanged after analysis/optimization, repeated optimized C/JSON equal, no persistent plan accepted from external diagnostics | TESTED |
| Values/failures/effects/order | Independent exact-AST/native matrix and callback-argument traces at O0/O3; 3,116 comparisons and 9,318 final bounded fuzz executions, detailed in companion report | TESTED within those domains |
| Existing optimization integration | Mixed callback with checked nested scalar call must produce/reuse scalar variants while the sequence fuses; previous policy/proof/native suites and 22 mutations rerun | TESTED / mutation-sensitive |

This is not a stage-major-array optimization. Canonical pull execution already
avoids arrays. The demonstrated saving is loop/iterator-state/next-boundary
elimination; no performance numbers or nonexistent baseline allocations are
claimed. User-visible evaluation remains declarative under either strategy.

## Actual inspected artifacts

Reused the existing API artifact harness, extended to accept actual fusion as
well as specialization:

```sh
cd compiler
go run ../qualification/specialization-artifacts.go /tmp/<evidence> examples/dataflow.lk
```

Executed artifacts for this session: `/tmp/leek-dataflow-artifacts.mCTzF9`, with
canonical/optimized C, semantic DOT, analysis/plan/policy JSON, SHA256 sums,
O0/O3 UBSan binaries and captured outputs. Both variants produced exactly:

```text
30
20
11
```

Two pipelines fused, Fold stayed canonical, and canonical C/DOT were byte-identical
before/after optimized generation. The regular CLI independently emitted its JSON
report and ran successfully. Its existing scalar pass removed one independently
proven remainder-zero guard; unknown map/reduction arithmetic stayed checked.
Implementation fingerprints:

```text
24e3e7fd65cff2666c7a2c47027bb2d7104c49f4cd11a163f22e59d9d7ccdc39  compiler/bin/parser/dataflow_codegen.go
e2031e0de12f823273851cfe38bb29e0d5771f1b00f396a087824c72b6f4fb61  compiler/bin/parser/dataflow.go
68aefe15de9fc4949a427afb9dc750c326fec75084e08989d5fbb9c57109fec5  compiler/bin/constraints/dataflow.go
5950ebc56a2e0bd84e1e33f728a48e77ed46eb06e765aff70be37e3cd977480b  compiler/bin/optimization/fusion.go
```

## Targeted mutation qualification

`qualification/dataflow-mutations.py` follows the existing isolated harness:
fresh disposable compiler copy, passing baseline, one defect per copy, cleanup,
original Go-file fingerprints unchanged. Compile failures/timeouts are classified
INCONCLUSIVE, not kills. Final logs and machine-readable results:
`/tmp/leek-dataflow-mutations-qualified`.

| Injected defect | Meaningful detector | Outcome |
|---|---|---|
| Inclusive range end | Canonical expected sums/element coverage differ | KILLED |
| Replace initial by zero, including empty inputs | Nonidentity and empty-range expected values differ | KILLED |
| Invert filter polarity | Stable-selection expected result differs | KILLED |
| Skip map callback | Expected map result differs | KILLED |
| Use element twice instead of current accumulator | Ordered reduce/fold expected values differ | KILLED |
| Reverse fused stage order | Reference/canonical/fused callback-argument traces and first-failure prefix differ | KILLED |
| Reverse fused element order | Explicit value/accumulator traces differ even for commutative final sums | KILLED |
| Enumerate last element repeatedly | Canonical element/result expectations differ | KILLED |
| Remove callback multiplication check without proof | Reference reports overflow at 65536; both corrupted native variants instead return successfully | KILLED |
| Fuse without legality validation | Unsupported Fold/long/bare pipeline structural fallback assertion fails | KILLED |
| Report fusion but never select fused lowering | Required direct-loop/no-iterator assertions fail | KILLED |
| Retain an intermediate array in fused body | Structural no-array assertion fails although behavior can agree | KILLED |
| Accept mismatched map input type | Invalid u32→callback(u64) program is incorrectly accepted | KILLED |
| Remove callback purity enforcement | Impure wrapper incorrectly accepts effectful map callback | KILLED |
| Drop quantified callback obligations | Required retained obligation count becomes zero | KILLED |

**15 killed, 0 survived, 0 inconclusive** for these selected faults. These are
contract assertion failures, not build failures. Tests sensitive only to a wrong
diagnostic were not used as the sole purity/type evidence: the key malformed
programs actually became accepted in those mutants.

The prior `qualification/specialization-mutations.py` was also rerun against the
changed compiler: **22 killed, 0 survived, 0 inconclusive**, with passing baseline.
Logs: `/tmp/leek-dataflow-specialization-qualified`. Thus **37/37 selected mutations**
were detected in fresh runs; this is test-effectiveness evidence, not a general
mutation score or formal soundness proof.

Executed from repository root with the intended Go on PATH:

```sh
python qualification/dataflow-mutations.py <temporary>/dataflow-mutations
python qualification/specialization-mutations.py <temporary>/scalar-mutations
```

Full tests (674 passing test/subtest results), vet, race, build, native UBSan runs,
gofmt and whitespace checks passed. No tracked graph, executable, temporary C,
mutation copy, corpus cache, or index database belongs in the commit.

## Remaining boundary

No quantified safety discharge, reassociation, check hoisting, speculative calls,
filter/map reordering, signed specialization, generic fusion planner, SIMD or
parallelism. Wider domains and arbitrary programs are not exhaustively checked;
OS/resource failures, concurrent compiler-IR mutation and GCC defects are not
qualified. No applicable unresolved counterexample remains. Nothing was deployed
or released.
