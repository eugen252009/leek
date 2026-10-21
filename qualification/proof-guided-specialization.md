# Proof-guided specialization — qualification

Assessment: **PASS for the implemented bounded unsigned subset**. Default
execution is fully checked; specialization requires `-optimize`. Starting tree:
`117bd09`; Linux amd64, Go 1.26.2, GCC 14.2.0. No push, release, publication or
deployment was performed.

The [architecture and proof gate](../docs/proof-guided-specialization.md)
describe the construction and limits. Tests below are in
`compiler/bin/optimization/specialization_test.go`, unless noted otherwise.

## Gates D–E: structure and behavior

| Check | Evidence | Outcome |
|---|---|---|
| Exact proof ownership | `TestSpecializationStructureAndProvenance` checks every eligible witness against its fresh obligation, environment, consistent `PROVEN` status, operation identity and complete call path | PASS |
| Actual specialization | Isolated C bodies must contain the original checked helper, a private variant's raw arithmetic assignment, an actual specialized call expression, and an actual original call for an unproven caller | PASS; comments/counts alone cannot satisfy assertions |
| Calls, composition, nested wrappers | Guarded direct/composition inputs, additional parameters, swapped parameters, nested two-stage wrappers, sibling scopes and unproven callers | PASS |
| Partial and conditional contracts | A mixed subtraction/division clone removes only subtraction; unknown division remains. A conditional square clone retains the other branch's unknown addition check | PASS |
| Reuse | Independently proven sites reuse one minus/inverse implementation; unrelated callers do not acquire those proofs | PASS |
| Per-function/global/depth limits | `TestSpecializationBudgetAndFallback`, `TestSpecializationGlobalBudget`, `TestSpecializationIncompleteAndDepthFallback`: 4/function, 64 total, 16 call edges, complete pure callees required | PASS; canonical fallback retained |
| Concrete benefit | `TestSpecializationDoesNotCloneAlreadyOptimizedWrapper` rejects a redundant wrapper variant when canonical dispatch already provides the same guard elimination | PASS |
| Semantic preservation | Canonical C and DOT before/after are byte-identical; repeated generated C, variant plans and witnesses are deterministic | PASS |
| CLI controls | `compiler/specialization_test.go`: default, explicit disabled, enabled; equal output/DOT, real variant diagnostics only when enabled. Existing inspection test retains read-only precedence | PASS |

`TestSpecializationNativeDifferential` performs **504 paired native comparisons**
(18 entry functions × 14 boundary/threshold inputs × GCC O0/O3). It compares
exact stdout, stderr and exit status. Independent witnesses also check pipeline
return values, underflow from unproven callers, preserved unknown division
failure after proven subtraction, and observable effect prefixes.

`TestSpecializedArgumentsAndFirstFailure` adds **14 paired executions** (seven
fixtures × O0/O3), with independent expected traces and failure categories:
left-to-right effectful arguments, earlier argument failures, skipped boolean
RHS calls, and successive successful/failing calls. An opaque mark result is
not treated as equal to its input.

`TestSpecializationUnsignedWidths` runs every input **0..255** for all four
unsigned widths at O0/O3, with at least five genuinely eliminated callee guards
per width. That is **2,048 paired function-input evaluations** within eight
paired process runs. u8/u16 promotion, addition, subtraction, multiplication,
division and remainder are exercised. Per-function budget tests add ten native
comparisons, including zero-divisor failure on a fallback route.

All these C fixtures use warnings as errors and
`-fsanitize=undefined -fno-sanitize-recover=all`. No sanitizer diagnostic was
observed. Local unsigned optimization, signed canonical arithmetic, earlier
path-sensitive analysis and existing language behavior remain covered by the
full regression suite.

## Reproducible standalone artifacts

From `compiler/`, with the selected Go toolchain on PATH:

```sh
go run ../qualification/specialization-artifacts.go /tmp/leek-specialization-evidence
```

The harness retains canonical/optimized C, the unchanged semantic DOT, structured
analysis and variant/witness JSON, binaries, output bytes and C SHA-256 hashes.
It verifies canonical generation before/after optimization and O0/O3 UBSan
execution. In this session the output directory is
`/workspace/.leek-artifacts/specialization-qualification/example`.

The example emits **5 private variants**: minus1, divide_12_by, nested, other,
and subtract. Three leaf guards are eliminated. `calculate`, `guarded_nested`
and `ten` already have safe generic dispatch, so their redundant wrapper clones
are discarded. The generic `other` still calls checked minus1; only the proven
constant call from main can enter its specialized wrapper. All four example
binaries print `12`, `1`, `6`, `0`, `0` on separate lines and exit successfully.

The CLI build and the repository's default sample were also compiled and run
with artifacts outside the checkout. No tracked graph fixture was overwritten.

## Final regression and bounded fuzzing

The final `go test -count=1 -json ./...` completed all four packages with
**519 passed test/subtest outcomes**, including 86 top-level tests/fuzz seed
parents, **0 failures and 0 skips**. `go vet ./...`, `go test -race -count=1 ./...`,
`go build`, gofmt checks and `git diff --check` passed. Raw test events and race
output are retained under `/workspace/.leek-artifacts/specialization-qualification`.

The final native fuzz run (`FuzzSpecializationNativeEquivalence`, requested
10 seconds, two workers) executed **3,791 cases** without a mismatch. It builds
canonical/optimized UBSan binaries and compares exact stdout, stderr and status
for sampled u32 values over 18 fixed entry functions. This includes proven and
unproven callers, nested calls/composition, partial proofs, alternative branches,
short-circuiting, contradictory paths and observable effects.

The final arbitrary-source IR-preservation smoke fuzz
(`FuzzOptimizationPreservesIR`, requested 5 seconds, about 6 seconds including
shutdown) executed **60,112 cases**, without a reported failure. Only accepted
validated inputs reach lowering; rejected inputs are not native behavior checks.
This run checks unchanged canonical C/DOT and local proof legality, rather than
proving arbitrary-source native equivalence. These are bounded samples, not
unbounded correctness guarantees.

Commands from `compiler/`:

```sh
go test -count=1 -json ./...
go vet ./...
go test -race -count=1 ./...
go test ./bin/optimization -run '^$' -fuzz '^FuzzSpecializationNativeEquivalence$' -fuzztime=10s -parallel=2
go test ./bin/optimization -run '^$' -fuzz '^FuzzOptimizationPreservesIR$' -fuzztime=5s -parallel=2
go build -o /tmp/leek .
```

## Gate F: targeted mutation qualification

Run from the repository root, with Go available:

```sh
python3 qualification/specialization-mutations.py /tmp/leek-specialization-mutations
```

The script requires a passing baseline, copies the compiler into a fresh
temporary directory for each mutation, retains logs, checks working-tree source
fingerprints, and records results as JSON. Build errors and timeouts are
inconclusive, never credited as kills.

| Mutation | Meaningful detector | Result |
|---|---|---|
| Accept UNKNOWN as PROVEN | Exact witness status and forbidden unproven routing | KILLED |
| Install caller-specific facts globally | Canonical-body structure and unproven-caller behavior | KILLED |
| Omit imported callee requirement | Independent summary/call proof checks and 8-bit oracle | KILLED |
| Reverse parameter substitution | Direct/composition/reversed-argument contract checks | KILLED |
| Reverse required subtraction comparison | Expected success contract and independent source oracle | KILLED |
| Leak one exclusive branch into continuation | Sibling-call structure and source-path oracle | KILLED |
| Drop unproven division guard | Required checked helper, failure oracle and UBSan | KILLED |
| Reuse an incorrect mask for the same function | Variant-mask/budget assertions and native failure comparisons | KILLED |
| Emit variants but route calls to canonical code | Actual specialized call-expression assertion | KILLED |
| Reverse argument evaluation | Independent expected effect trace / first failure | KILLED |
| Never specialize | Required variants and actual guard elimination | KILLED |

**11 killed, 0 survived, 0 inconclusive.** This measures these chosen faults,
not a complete mutation score or compiler correctness proof. Evidence is retained
under `/workspace/.leek-artifacts/specialization-qualification/mutations`.

## Remaining limits

Only complete pure scalar callees and proven unsigned binary operations are
specialized. A caller may contain effects, but no effectful body is cloned.
Unknown, violated, unreachable/inconsistent or unsupported proofs do not remove
checks. Individual successful-operation proofs do not guarantee surrounding
expression success. Budgets may conservatively decline an otherwise legal
optimization. No signed specialization, recursion, indirect calls, SMT, JIT,
source inlining, runtime deoptimization or broader optimizer is included.

The argument assumes immutable validated bindings, no concurrent mutation of
compiler IR and the existing scalar checked semantics. Tests do not prove the
absence of all analyzer/backend/GCC defects, qualify arbitrary I/O/OS failures,
or exhaust wider integer domains.
