# Quantified range analysis — qualification

## Assessment and inspected artifact

**PASS for the implemented interval/domain and unsigned-additive-prefix subset.**
This is empirical implementation qualification with bounded exhaustive cases,
not a formal proof for arbitrary programs or all wider integer valuations.

Inspected clean baseline: `7605b8fadfdc15980d2e8e9fc3d46056bf738916`.
The previous 674 results, 3,116 comparisons and 37 mutations were historical;
all existing suites were rerun against the new implementation. Final full run:
**716 passing test/subtest results**, zero failures, successful vet and race
checks. Environment: Linux amd64, Go 1.26.4, GCC 14.2.0. No new dependency or
solver, source syntax, public sequence type, or execution semantics.

The repository already retained per-element callback requirements and scalar
summary substitutions, but assigned no universal intervals/prefix invariants.
The extension uses that same term arena, exact big integers, path contexts,
reasoner and summaries. It adds domain/stage-scoped environments and a separate
quantified obligation ledger. The scalar traversal context and function inputs
are not refined by those quantified facts. See
[the model](../docs/quantified-range-analysis.md).

## Contracts and executed evidence

| Contract | Required property | Actual evidence | Status |
|---|---|---|---|
| Q-DOMAIN | Half-open bounds/cardinality; EMPTY/NONEMPTY/POSSIBLY_EMPTY; no unsigned end-1 wrap or fictitious empty-domain element | `TestQuantifiedDomainEdges`: zero/reversed/singleton/nonzero starts, identical symbolic endpoints, all integer widths and 65-bit-scale mathematical differences, negative endpoint, independently varying bounded endpoints | TESTED |
| Q-TRANSFER | Scalar summary transfer and safety obligations remain independent of success-conditional output bounds | `TestQuantifiedSummaryTransfer`, `TestQuantifiedFoldAndCircularity`: + - * / %, branches, direct pure composition, overflowing map versus safe downstream modulo; extra unused reducer failure retained | TESTED |
| Q-FILTER | Filter cannot increase cardinality; arbitrary predicates do not imply invented numeric limits | Conservative [0,K], recognized <10/positive refinement, always/even selection in independent enumeration and native fixtures | TESTED |
| Q-PREFIX | Every reached unsigned additive prefix bounded, with actual initial value; no arbitrary fold recurrence assumption | Motivating [0,980100] and previous-state [0,970299], nonzero/near-MAX initial cases, map-only proofs, unsupported state*2+x/subtract folds | TESTED |
| Q-SCOPE | Exact domain/stage/operation origins, no global scalar invariant or cross-domain fact leakage | `TestQuantifiedDeterministicReadOnly`, `TestQuantifiedPartialFiltersAndScope`: scalar statements inside/after branches, distinct domains, tagged environments, unchanged C/DOT, implication queries | TESTED |
| Opaque identity preservation | Independent unknown function returns are not equated as range endpoints | `TestQuantifiedIncompleteEndpointIdentity` and independent native reference regression | TESTED |

Test paths are under `compiler/bin/constraints`; optimization/native checks are
qualified [separately](dataflow-proof-optimization.md). The initial independent
analysis gate passed before using its results as optimization evidence; final
regressions exercise both together. Sequence-containing scalar summaries remain
incomplete because general sequence return contracts are not exported. This is
distinct from the new PROVEN callback and pipeline arithmetic records.

## Independent bounded element/prefix oracle

`quantified_oracle_test.go` generates **5,670 programs** spanning:

* limit 0..20, start 0..4, initial {0,7,250}, u8 arithmetic;
* six map forms (+1, -1, square, 100/x, modulo 7, conditional cap);
* no filter, even filter, or <10 filter.

It independently enumerates **62,370 n/program valuations** and **312,930 source
element visits**, checking explicit u8 arithmetic rather than symbolic transfer.
It checks actual element/output enclosure, every claimed PROVEN map, filter
cardinality, previous-state enclosure, every visited accumulator prefix (including
the first mathematical overflowing sum), and every claimed PROVEN reduction.
It stops accumulator execution at the first failure but continues checking map
claims over represented elements, avoiding a test that overlooks unsafe later
maps merely because an earlier reducer happened to fail.

Additional unit cases cover wider type extrema, negative endpoints, uncertain
nonzero endpoints, path scopes, interprocedural bounds, unsupported folds and
partial safety. These are separate finite-domain checks, not a universal proof.

Final 10-second `FuzzQuantifiedBounds`: **32,983 executions**, no reported failure;
generated limits <=32, starts <=8, arbitrary u8 initial values, map/filter choices,
each independently enumerated across n and elements. Earlier development run:
25,375 executions; counts overlap and are not summed as unique coverage.
Read-only analysis fuzz regression: **30,382 executions** in a requested 5-second
run (about six seconds including shutdown), no reported failure.

## Counterexample found and repaired

Review exposed a real analysis defect: two calls to an incomplete pure function
returning a sequence reduction could share the callee summary's opaque output
identity. `range(size(1),size(3))`, with size being an ordered sum, could then be
misreported empty although its actual domain was [0,3).

Incomplete scalar-call results now have **call-site-local opaque identities**.
They retain type/success conditions but are not mistaken for a parameterized
mathematical function relation. The new regression verifies distinct endpoint
IDs/nonempty possibilities and reference/native behavior. This is a conservative
analysis correction, not a runtime semantic change. Empty-domain optimization
was already disabled, but incorrect analysis facts were not accepted as harmless.

## Scope of statuses and limits

Quantified requirements report PROVEN or UNKNOWN; they do not claim an entire
program must fail because one represented element might fail. Scalar setup
failures can still report VIOLATED independently. Empty domains have vacuous
callback safety but no invocation environment or optimization witness. Output
bounds remain conditional on successful returned values; they never prove the
producing operation. Unsupported/limited analysis remains unknown/fallback.

Only ordered unsigned addition receives the recurrence rule. Signed interval
analysis is supported, not signed specialization. General folds, recurrence
solving, quantified exported return contracts, correlations beyond the existing
small reasoner, SMT, SIMD and parallelism remain deferred. No hard adversarial
compiler time/memory quota or OS/resource-failure determinism is claimed.

Commands from `compiler/` (with `/usr/local/go/bin` on PATH):

```sh
go test -count=1 ./... -json
go vet ./...
go test -race ./...
go test ./bin/constraints -run 'TestQuantified' -count=1 -v
go test ./bin/constraints -run '^$' -fuzz '^FuzzQuantifiedBounds$' -fuzztime=10s -parallel=2
go test ./bin/constraints -run '^$' -fuzz '^FuzzAnalysisReadOnly$' -fuzztime=5s -parallel=2
go run . -constraints examples/quantified.lk
```

All passed. No unresolved applicable counterexample remains in the qualified
subset. Native/check-removal evidence, mutations and optional timing measurements
are in the [optimization report](dataflow-proof-optimization.md).
