# Specialization reuse and static cost — qualification

Assessment: **PASS for the supported opt-in unsigned subset**. The previous
milestone was pushed first as `eead78a307bf54fa4b0bee96cc396e102dedd244` and its
presence on `origin/main` was verified before new code was written. This report
qualifies the extension based on that commit. Environment: Linux amd64,
Go 1.26.2, GCC 14.2.0. [Implementation policy](../docs/specialization-policy.md).

## Structural and independent gates

| Requirement | Executed evidence | Outcome |
|---|---|---|
| Logical implication is independent of cost | Dedicated direction/normalization tests and 451,584 independent u8 valuations in `constraints/implication_test.go` | PASS |
| Equivalent/stronger caller domains reuse an implementation | `TestPolicyEquivalentAndStrongerFactsShareVariant`: four syntactically different valid domains produce one minus variant and three actual reuses; weak unsigned domain falls back | PASS |
| Multiple applicable variants are ranked | `TestPolicySubsumptionRankingAndFallback`: a one-guard and two-guard implementation both apply to the strong caller; actual generated calls select the latter | PASS |
| Weak/unrelated/inconsistent facts do not authorize stronger contracts | Same structural test rejects weak, unrelated and contradictory callers; equality 5 rejects the two-guard contract while reusing the weaker one | PASS |
| Whole contract, body, semantics and binding are checked | `TestPolicyRejectsForeignBodySemanticsAndBinding`: foreign body, wrapping semantics and missing exact argument binding cannot pass applicability | PASS |
| Avoid low-benefit and zero-benefit bodies | `TestPolicyCostRejectsLowBenefitAndIdentity`: a large body with one removable guard is rejected; identity has no demonstrated guard benefit | PASS |
| Reuse survives budget exhaustion | `TestPolicyBudgetReusesWeakerApplicableVariant`: four incremental contracts fill the per-function budget; a fifth stronger caller uses the best existing legal implementation, unknown caller stays canonical | PASS |
| Additional code growth is bounded | `TestPolicyAdditionalCodeBudget`: 50 otherwise profitable functions hit the 4096-unit limit before all can be cloned; remaining callers use canonical targets | PASS |
| Existing count/depth bounds, partial proofs and fallback survive | Previous per-function/global/depth/incomplete-analysis and actual C structure tests remain active | PASS |
| Canonical implementations and determinism remain | Byte-identical canonical generation before/after; actual checked canonical helpers retained; deep equality of repeated decisions/registry plus identical emitted C | PASS |
| Machine-readable diagnostics and controls | `compiler/policy_test.go`: real JSON decisions, final targets and reuse; no generated C payload; flag requires optimization; inspection writes no policy report and needs no GCC | PASS |

Structure tests inspect actual C call expressions and arithmetic assignments,
not just comments or counts. The "never specialize" and "emit variants but call
canonical" mutations must therefore fail despite possible runtime equivalence.

## Execution and regressions

`TestPolicyNativeDifferential` performs **224 paired native comparisons**:
eight entry functions × 14 boundary/threshold values × GCC O0/O3. Outputs,
stderr and exit status must match exactly. Independent witnesses check successful
strong-caller return values, retained underflow from weak callers, and observable
effect traces. Tested values include 0, 1, 2, 5, thresholds around 10 and 20,
UINT32_MAX-1 and UINT32_MAX.

Budget-reuse tests add **6 paired executions** including zero-divisor failure
after a successful reused specialization. Existing qualification remains active:
native addition/multiplication overflow, subtraction underflow, zero division,
nested compositions, ordinary calls, short-circuit behavior, conditional paths,
effects, first-failure ordering and all unsigned widths. All native C fixtures
use warnings as errors and UBSan with nonrecovering diagnostics, at both O0/O3
where specified. No sanitizer violation was observed.

Final full regression: **591 passed test/subtest outcomes**, including 101
top-level tests/fuzz seed parents, **0 failed and 0 skipped** across all four Go
packages. `go vet ./...`, `go test -race -count=1 ./...`, build, formatting and
whitespace checks passed. This distinguishes the new results from the previous
milestone's 519 outcomes.

`FuzzPolicyNativeEquivalence`, requested 10 seconds with two workers, completed
**3,058 cases** without mismatch. It compares native canonical/optimized
stdout/stderr/status on sampled u32 values for the eight policy entry points.
The logical fuzzer's **143,715 cases** are described in the
[implication report](constraint-subsumption.md). These are bounded runs, not
formal verification or exhaustive wider-integer coverage.

## Standalone evidence

From `compiler/`:

```sh
go run ../qualification/specialization-artifacts.go /tmp/leek-policy-evidence examples/specialization-policy.lk
go run . -optimize -optimization-report /tmp/leek-policy.json -o /tmp/leek-policy -graph /tmp/leek-policy.dot examples/specialization-policy.lk
```

The harness retains C, semantic DOT, structured analysis/registry/decisions,
binaries, captured output and hashes. O0/O3 checked/optimized binaries all print
`2`, `0`, `10`, `0`, `2` and exit successfully. The example emits **4 variants**:
two contracts for `two`, and useful `a_first`/`e_weak` wrappers. It reports 6
static reuses, 2 redundant wrapper proposals avoided, 6 allocations and 74
conservatively charged AST units. These are compiler plan metrics, not runtime
invocation counts or measured performance claims.

Session evidence is retained under `/workspace/.leek-artifacts/policy-qualification`:
`tests.jsonl`, `race.log`, native/implication fuzz logs, CLI JSON, generated
example artifacts and mutation logs/results. Generated files and binaries stay
outside the checkout; no tracked DOT fixture or dependency manifest changed.

## Targeted mutations

`qualification/specialization-mutations.py` now runs the current implication,
policy and earlier specialization gates. It requires a passing baseline, uses
fresh disposable compiler copies, verifies working-source fingerprints and
retains logs/JSON. Build failures and timeouts are INCONCLUSIVE, not kills.

| Mutation | Meaningful detector | Result |
|---|---|---|
| UNKNOWN accepted as PROVEN | Weak/unrelated caller structural gate | KILLED |
| Caller facts installed globally | Canonical/body and applicability assertions | KILLED |
| Omit callee safety requirement | Independent reusable-contract checks | KILLED |
| Reverse parameter substitution | Parameterized call contract assertions | KILLED |
| Reverse required arithmetic comparison | Wide-integer proof/domain test | KILLED |
| Merge exclusive branch facts | Sibling/unproven call structure | KILLED |
| Drop unproven division guard | Native budget/failure equivalence with UBSan | KILLED |
| Select existing variant without applicability | Strong/weak registry selection test | KILLED |
| Emit variants but call canonical | Actual specialized-call assertion | KILLED |
| Reverse argument evaluation order | Independent effect trace / first failure | KILLED |
| Disable specialization entirely | Required registry and actual transformed calls | KILLED |
| Reverse implication direction | Direction/equivalence table | KILLED |
| Incorrectly tighten integer normalization | Exact exclusion endpoint expectation | KILLED |
| Ignore required parameter binding | Explicit missing-binding rejection | KILLED |
| Ignore arithmetic semantics | Explicit wrapping/checked incompatibility rejection | KILLED |
| Reuse another function's body | Explicit foreign-body rejection | KILLED |
| Ignore all specialization budgets | Bounded registry/reuse assertions | KILLED |
| Duplicate equivalent variants | Exact registry/reuse structure | KILLED |
| Select inferior applicable implementation | Multiple-eligible ranking and emitted-call assertion | KILLED |
| Ignore benefit/cost ratio | Large-body low-benefit rejection | KILLED |
| Ignore additional-code budget | 50-function code-growth test | KILLED |
| Assume requested predicate to prove itself | Weak-caller applicability rejection | KILLED |

**22 killed, 0 survived, 0 inconclusive.** This is effectiveness evidence for
these selected faults, not a complete mutation score or correctness proof.

```sh
# Repository root; intended Go on PATH:
python3 qualification/specialization-mutations.py /tmp/leek-policy-mutations
# compiler/:
go test -count=1 -json ./...
go vet ./...
go test -race -count=1 ./...
go test ./bin/optimization -run '^$' -fuzz '^FuzzPolicyNativeEquivalence$' -fuzztime=10s -parallel=2
go test ./bin/constraints -run '^$' -fuzz '^FuzzIntegerImplication$' -fuzztime=10s -parallel=2
```

## Supported boundary

No required correctness gate is unresolved. Full checked semantics remain the
default and canonical functions remain available. Cost may reject a valid
optimization; it never supplies proof. Exact implication remains conservative
for unsupported relations. No new signed specialization, recursion, indirect
calls, collections, SIMD, parallelism, SMT, profiling, JIT or deoptimization is
included. The construction assumes immutable validated IR during compilation;
it does not qualify arbitrary OS failures, all compiler/GCC defects or every
possible program. Static guard counts/AST units are not measured speed claims.
