# Declarative dataflow — qualification

## Scope and assessment

**PASS for the implemented inline scalar sequence subset**, empirical evidence
only. This is not formal verification, full collection support, or qualification
of SIMD/parallel execution. No such deferred strategy was introduced.

Baseline: clean `9f2251ad907bb9178051fd16660d0ee15938ed08`. The previous milestone's
591 results and 22 mutations were treated as historical evidence, not evidence
for this change. A fresh complete run now records **674 passing test/subtest
results**, zero failures, with successful vet and race checks. Existing parser,
type, constraints, native arithmetic, implication, and bounded cost-aware
specialization suites are included in that rerun.

Environment: Linux amd64, Go 1.26.4, GCC 14.2.0. No dependency, solver, public
sequence type, indirect calls, source loops, array mutation, or new runtime
allocation mechanism was added. Required semantics are in
[collection-semantics.md](../docs/collection-semantics.md), preserving the axioms
and existing composition/effect contracts. See
[the repository assessment/IR boundary](../docs/declarative-dataflow.md).

## Qualification gates

1. Inspected actual scalar AST, semantic graph, callbacks/call graph, checked
   helpers, analyzer, proof-subsumption and specialization registry before edits.
   The scalar baseline suite passed.
2. Specified element-major evaluation, setup/initial ordering, filtering,
   reduction/fold dependency, half-open ranges and failures before fusion.
3. Implemented and independently ran canonical Range→Map→Reduce, then Filter and
   Fold; canonical type/native/constraint suites passed before fusion was added.
4. Added the separate structural trace-preservation gate/direct-loop strategy;
   qualified it [independently](loop-fusion.md).

## Contract matrix

| ID | Required behavior | Actual tests | Status |
|---|---|---|---|
| DF-IR | Range/Map/Filter/Reduce/Fold remain distinct semantic operations with dependency/type/order/effect/failure information | `parser/TestDataflowStructure`, independent structural inspection and typed callback references/data edges | TESTED |
| DF-TYPE | Compatible static pure callbacks only; no escaping sequence/higher-order value | `TestDataflowSemanticRejections`: escaping/unconsumed values, print, scalar source, arity, integer bounds, map input mismatch, predicate output, reduction/fold state compatibility, literal initial mismatch, non-name references, effectful and transitive callbacks, shadowing and reserved functions | TESTED |
| DF-RANGE | Half-open ascending, empty/reversed domains, no count/increment overflow or domain-sized allocation | Canonical range/type-edge native tests for all eight widths, min/max-adjacent singleton domains, empty/reversed cases, no malloc/array lowering; native reference comparisons for all widths | TESTED at stated boundaries |
| DF-ORDER | Bounds then initial once, element-major callbacks, stable filters, skipped downstream work, ordered state | `TestDataflowCanonical`, `TestDataflowCanonicalFailureOrder`, reference/native traces including actual parameters/accumulator values, setup marks, short-circuit/nested pipelines | TESTED |
| DF-FAIL | Preserve checked arithmetic, first failure category/location, earlier effects and skipped later effects | Map overflow, reduction overflow, underflow, zero division, failed predicates, competing map failures, signed min / -1 and % -1, empty initial failure, UBSan/native oracle | TESTED |
| DF-FOLD | Empty returns initial; state may differ; no identity/associativity assumption | Canonical ordered doubling fold (result 11), u64 state/u32 elements, arbitrary nonidentity initial values, float scalar transport, distinct Fold IR and canonical fallback | TESTED |
| DF-CONSTRAINTS | Scalar setup requirements and quantified callback conditions retained without unsound proofs | `constraints/TestDataflowConstraints`, `TestDataflowSetupObligations`: type/domain facts, predicate, independent initial/accumulator, exact callback origins, 3 retained arithmetic requirements, UNKNOWN/incomplete propagated to callers, independent setup violation | TESTED |
| Existing guarantees | Scope isolation, effects, checked helpers, branch proofs, specialization eligibility/reuse/budgets preserved | Full baseline suites rerun; mixed sequence/scalar specialization native fixture requires actual variants and reuse; previous 22 mutations rerun and killed | TESTED / mutation-sensitive |

Tests are under `compiler/bin/parser`, `compiler/bin/constraints`, and
`compiler/bin/optimization`. CLI tests are in `compiler/dataflow_test.go`:
canonical/enabled/explicitly-disabled compilation, expected output, matching DOT,
truthful JSON, and inspection with no GCC/no generated artifacts.

Only scalar terminal values escape. Pull descriptors and per-element values
remain private scope-bound implementation storage; map/filter callbacks cannot
capture external mutable state. Lifetime/alias extensions were not introduced.

## Independent reference and finite domains

`optimization/dataflow_oracle_test.go` interprets **source syntax**, scalar typed
AST operations, exact big integers and explicit finite-width failure rules. It
has its own straightforward element loop and callback recipe; it does not use
`parser.Pipeline`, iterator/fused lowering, optimization decisions, or analyzer
classifications as its oracle. It follows conditional/short-circuit scalar code
and returns, handles prints, and records callback arguments and state transitions.

**3,116 three-way native input comparisons** check reference/canonical/optimized
stdout, exit status, first arithmetic failure reason/location and source-function
traces. Breakdown:

* 2,904 primary cases: 12 pipeline patterns × 112 inputs × O0/O3, plus two
  ordered-fold patterns × 108 bounded inputs. The primary log records **2,342
  successes and 562 failures**.
* The small domain exhausts start/end in 0..5 and initial in {0,1,MAX}; additional
  cases include [0,100), multiplication thresholds 65535/65536, and MAX-1/MAX.
* 162 signed i8 comparisons at nine min/zero/max-adjacent endpoints, O0/O3.
* 28 other integer-width edge/signed-exception cases, 11 setup-effect/different
  state cases, three Boolean-element cases, eight nested/short-circuit/scalar
  specialization cases.

Ordinary uninstrumented native tests additionally exercise all range widths,
Boolean/f32/f64 transport, expected outputs, and the actual CLI example. Reference
tracing is test-only C instrumentation after lowering (including specialized
variants), not permission for pure source callbacks to perform I/O. Normal test
I/O/platform behavior is assumed. All native compilations use warnings as errors
and UBSan; no sanitizer deviation was observed.

These are finite program/input domains, not an exhaustive claim for all programs
or all u32/u64 inputs. The native oracle bounds its own iterations to 4,096;
production has **no** such hidden iteration cap. Large production ranges require
constant storage but may take impractically long.

## Fuzzing and commands

Bounded final runs, no reported failure:

| Target | Duration requested | Executions |
|---|---:|---:|
| New `FuzzDataflowNative` (reference + canonical + fused, callback arguments traced) | 10s | 9,318 |
| Parser/resolver/lowering, with sequence seed | 5s | 23,935 |
| Read-only analysis, with sequence seed | 5s | 9,150 |
| Optimizer IR preservation, with sequence seed | 5s | 88,195 |
| Previous policy native equivalence | 5s | 4,907 |
| Previous specialization native equivalence | 5s | 4,849 |
| Previous path-proof soundness | 5s | 7,660 |

Native dataflow fuzzing samples domains of at most eight elements across the u32
range, including clipped MAX endpoints and arbitrary initial accumulators. It
checks one supported filter/map/reduce schema; the unit matrix covers other
schemas. Arbitrary-source fuzzing often rejects invalid inputs; its execution
count is not a count of valid native programs. Requested 5-second runs can take
about six seconds including worker shutdown. Sampling is not formal proof.

Executed from `compiler/` with `/usr/local/go/bin` on PATH:

```sh
go test -count=1 ./... -json
go vet ./...
go test -race ./...
go test ./bin/optimization -run '^$' -fuzz '^FuzzDataflowNative$' -fuzztime=10s -parallel=2
# The other named targets above used -run '^$', -fuzztime=5s, -parallel=2.
go run . -constraints examples/dataflow.lk
go run . -optimize -optimization-report <temporary>/decisions.json -o <temporary>/app -graph <temporary>/graph.dot examples/dataflow.lk
go run . -o <temporary>/sample -graph <temporary>/sample.dot
go build -o <temporary>/leek .
```

Build/sample/example, gofmt and diff whitespace checks passed. Generated outputs
were kept outside the repository; tracked `compiler/graph.dot` was not rewritten.

## Limits and conclusion

Universal sequence arithmetic proofs remain UNKNOWN; no accumulator invariant
solver or sequence-specific unchecked arithmetic was added. Canonical Reduce
and Fold remain ordered. Source sequences cannot be stored/returned, callbacks
are static names, and float arithmetic remains unsupported as before. All wider
integer domains and arbitrary nested pipelines are not exhaustively verified.
OS failures, arbitrary stack/resource exhaustion and compiler backend defects
are outside the empirical qualification boundary.

No unresolved applicable counterexample remains in the exercised subset. This
supports the implemented milestone, not release/deployment approval. See the
separate [fusion mutation and preservation report](loop-fusion.md).
