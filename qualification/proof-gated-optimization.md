# Proof-gated guard elimination — qualification

## Assessment

**PASS for the implemented experimental local-unsigned subset.** Canonical
execution remains checked by default; enabling `-optimize` is explicit. Signed
arithmetic, unary negation, impure function bodies, incomplete analysis, and
caller-specific callee specialization are **not optimized**. This is an explicit
supported-operation boundary, not a claim that all proven checks are removed.

Artifact/environment: the working tree based on `e726089`, following independently
qualified Stage A; Linux amd64, Go 1.26.4, GCC 14.2.0. See
[the mechanism and legality conditions](../docs/proof-gated-optimization.md).
Final standalone inspection artifacts: `/tmp/leek-path-qualification.9FH41A`
(canonical/optimized C, semantic DOT, proof/elision JSON, binaries and captured
O0/O3 output). Both variants printed `12`, `12`, `105`, with identical bytes and
no sanitizer diagnostics. Canonical C was byte-identical before/after analysis
and optimized generation. Relevant implementation SHA256 fingerprints:

```text
a3904b8485301cdf9ee9d8a196df83220cd55eda7868b9ecb97a8285ec69c6a7  compiler/bin/parser/codegen.go
4ab70206385aed4dc8f9119972932b9343345c6bb6bc89a2074a469ec074d944  compiler/bin/optimization/guards.go
```

No solver/dependency, speculation, extra guard, general optimizer, inlining,
collection, parallelism, or JIT was added.

## Construction and preservation

The requested guarantee is that every removed guard is redundant under the
conditions at its own execution point, and no observable canonical behavior is
changed. The consumer runs analysis and lowering on the same validated graph in
one call. It accepts no external proof/plan, uses semantic node pointers rather
than diagnostic strings for authorization, requires a complete pure function,
consistent exact `PROVEN` status, empty call-propagation path, a supported
unsigned binary operation, and its entire single-obligation local contract.
Resource exhaustion disables elimination. The canonical generator ignores these
decisions and is independently callable before and after optimized generation.

Immutable source bindings and no mutable aliases mean established path facts
cannot be revoked by source assignment. Call-site facts do not change generic
callee bodies. Raw C arithmetic replaces only the already-safe helper call;
operand temporaries, calls, branches, effects, source locations of remaining
failures, and statement order are unchanged. Normal platform operation and no
concurrent mutation of the validated graph are assumptions, not new guarantees.

This is a design/construction argument plus test evidence, **not formal
verification** of the compiler, GCC, or all programs.

## Executed checks

All tests below are in `compiler/bin/optimization/guards_test.go` unless noted.

| ID | Required property | Actual evidence | Qualification |
|---|---|---|---|
| PG-GATE | Remove only exact-site PROVEN whole-operation contracts; keep UNKNOWN/VIOLATED/unsupported checks | `TestProofGatedStructure` cross-checks each elision's proof, environment, node and provenance; requires guards for multiplication, live joins, disjunction, failing/unreachable arithmetic, signed operations and impure bodies | TESTED |
| PG-STRUCTURE | Elimination actually changes lowered execution, not just a status/count | Canonical helper-call assignment must exist; corresponding optimized statement must use raw operator and no checked helper; expected function-local elimination counts; never-optimizing mutation detected | TESTED |
| PG-EQUIV | Values, failures, failure category/location, branch outcomes, effects and output bytes match canonical | `TestCanonicalOptimizedDifferential`: **720 paired native comparisons** (18 functions × 20 boundary/threshold inputs × O0/O3). Exact stdout/stderr/status equality, plus independent success/failure witnesses. Includes 0/1/2/MAX-1/MAX and overflow threshold 65535/65536 | TESTED |
| PC-ORDER / PC-FAIL | Source/arguments once in order, skipped paths stay skipped, first failure/effect prefix preserved | `TestDifferentialEffectsFailuresAndSkippedPaths`: eight effect/short-circuit/failure-order fixtures at O0/O3. Signed minimum division and remainder retain canonical failure. Independent expected traces/status/category supplement differential equality | TESTED |
| PG-NATIVE-BOUNDS | Proven arithmetic compiles safely with C integer promotion and preserves values | `TestNativeBoundedUnsignedOperations`: every x in 0..255, all four unsigned widths, O0/O3; all successful. Includes promoted u8/u16 addition/multiplication, division/remainder and continuing subtraction | TESTED within those native domains |
| PG-PRESERVE | Optimization does not destroy canonical IR or leak decisions between generations | Byte-identical canonical C/DOT before/after; deterministic optimized output/elisions; valid graph required; canonical reusable generator | TESTED |
| PG-LIMIT | Unsupported/partial/resource-limited analysis fails closed | Explicit float-incomplete and 4,098-obligation fixtures retain checks, even at independently provable sites | TESTED |
| PG-CONTROL | Experimental pass disabled by default and explicitly disableable; inspection always read-only | `compiler/proof_optimization_test.go`: default/enabled/disabled CLI compile/run, expected **3** actual example elisions, equal semantic DOT, no temporary C left; inspection with both flags and no GCC emits only JSON/no artifacts | TESTED |

All native fixtures compile with warnings as errors and
`-fsanitize=undefined -fno-sanitize-recover=all`. Both GCC `-O0` and `-O3` are used;
no sanitizer violation was observed. Raw operation/helper inspection is at the
lowered C execution point, not just runtime helper definitions or timing.

The canonical sample and examples were also compiled/run using temporary output
and DOT paths. No tracked graph fixture or binary was committed. Full tests,
`go vet`, `go test -race`, build, gofmt, and whitespace checks passed. The earlier
composition/advisory checks were rerun against the changed backend.

### Required examples

* A: `x > 0` proves subtraction; its local guard is removed.
* B: `x > 0` does not prove square's overflow requirement; guard remains and
  input 65536 still fails for u32.
* C: unsigned `x != 0` proves division; local zero guard is removed.
* D: `x != 0 && 100/x > 2` proves the RHS division, while input zero still skips it.
* E: guarded pure composition/direct calls prove imported callee obligations.
  **Generic callee checks remain**: no caller specialization is implemented.
  The equivalent local two-operation body removes both guards, demonstrating
  the exact-site proof mechanism without incorrectly weakening generic callees.
* F: observable marks and argument/source ordering remain unchanged around pure
  optimized helpers and later checked failures.

## Bounded fuzzing

`native_fuzz_test.go` builds canonical/optimized UBSan binaries and compares
stdout, stderr, and status for generated u32 inputs and five branch/arithmetic
functions. **9,430 executions** in a 10-second final run, no reported failure.
This exercises native success, overflow, unsigned underflow, zero boundaries,
short-circuiting, and branch disjunctions; it is sampled input testing.

`FuzzOptimizationPreservesIR`: **53 executions** in a 5-second requested bounded
run (approximately six seconds including shutdown), no reported failure. This
small arbitrary-source smoke test checks valid generated graphs, fresh proof
statuses and unchanged canonical C/DOT; it is not a native-equivalence or broad
malformed-input claim. Structured path-proof fuzzing and independent 8-bit
soundness domains are reported [separately](path-sensitive-constraints.md).

Commands, from `compiler/`:

```sh
go test -count=1 ./...
go vet ./...
go test -race ./...
go test ./bin/optimization -run '^$' -fuzz '^FuzzNativeEquivalence$' -fuzztime=10s -parallel=2
go test ./bin/optimization -run '^$' -fuzz '^FuzzOptimizationPreservesIR$' -fuzztime=5s -parallel=2
go run . -optimize -o <temporary>/app -graph <temporary>/graph.dot examples/path-sensitive.lk
go build -o <temporary>/leek .
```

## Isolated optimization/backend mutations

Fresh disposable compiler copies, passing baseline, one mutation each. Each
source was restored and byte-compared before deleting its copy. Session logs:
`/tmp/leek-opt-mutations.aoptjf9t`.

| Mutation | Observed meaningful detector | Outcome |
|---|---|---|
| Admit UNKNOWN as though PROVEN | Structural proof-authorization assertion; native mul(65536) wraps to 0 rather than failing | KILLED |
| Drop unsigned guards without proof in the backend | Canonical checked-statement assertion; independent multiplication failure oracle also detects that both paths were corrupted | KILLED |
| Never elide any guard | Required optimized raw-statement assertion, despite possible runtime equivalence | KILLED |
| Invert short-circuit execution condition | Independent expected skipped-path outcome detects division-by-zero instead of successful output, even though both variants share the bad backend | KILLED |

**4 killed, 0 survived, 0 inconclusive** for these selected defects, in addition to
five analysis mutations. This is evidence of test effectiveness for those faults,
not a soundness proof or general mutation score.

## Remaining limits

No unbounded all-program correctness proof; wider native domains are sampled.
Tests do not qualify arbitrary OS/I/O failures, stack exhaustion, GCC defects,
or concurrent mutation of compiler IR. Caller specialization and signed/unary
elimination require future independent qualification; they were not forced into
this milestone. No applicable unresolved counterexample was observed. Nothing
was deployed or released.
