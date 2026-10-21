# Safe specialization reuse and static cost policy

Status: experimental, opt-in with `-optimize`. Logical proof gates legality;
the following static heuristic chooses among legal implementations. It is not
a runtime performance prediction.

## Registry and requirements

The existing per-generation specialization registry now retains, for each exact
original body and private implementation:

* Formal parameter IDs, integer types and type domains from its summary.
* The guarded requirements corresponding to every removed arithmetic operation,
  including relative semantic call paths for redirected child implementations.
* Eliminated operation identities, remaining local checked-operation count,
  estimated body units, and transitive removed-guard benefit.
* The checked-integer semantic version and static reuse count.

Body pointers and operation/call-node identities belong to the current validated
graph. Diagnostic strings are never proof keys. Parameter substitution is the
existing analyzer's actual-argument mapping. For each candidate variant, every
contract operation/path must bind to the corresponding imported obligation in
this caller; `ImplicationEngine.Implies` then proves its instantiated requirement
against that obligation's exact guarded environment. A missing binding, foreign
body, incompatible semantics, inconsistent assumptions, UNKNOWN or VIOLATED
requirement rejects eligibility. Cost does not affect this test.

This supports more than equal operation masks. A caller that proves a larger
safe set may use an existing implementation that eliminates a subset. Different
caller constraints such as `x > 10`, `x == 5`, `x >= 1` and integer `x > 0` can
all prove the same `minus1` requirement. A weaker `x >= 0` cannot select it.
No caller-specific fact is installed as a global function invariant.

## Deterministic selection

Functions and semantic call sites are processed in sorted order. Existing
variants for the exact target are examined in stable allocation order. Among
proven eligible variants, the ranking is:

1. More removed guards, including redirected descendant guards.
2. Fewer remaining **local arithmetic operations** with checks.
3. Lower estimated body size.
4. Earlier registry allocation as the stable tie break.

This compares actual implementation properties, not the strength of a readable
constraint. Remaining operations are not a count of all low-level predicates
inside checked helpers. Guard counts ignore branch frequency and instruction
latency; they are a transparent MVP proxy, not a claim of measured speedup.

If an existing implementation is at least as beneficial as the proposed new
plan, it is reused. Otherwise creating a new plan must justify its incremental
benefit over that best legal existing implementation. If creation is declined,
the best proven existing variant is reused; if none exists, the canonical target
is used. A stronger existing contract cannot be chosen merely because it ranks
higher or because a budget is exhausted.

## Cost and growth bounds

One static unit is charged for each AST expression, statement, block, parameter
and function overhead. Counting is structural; it estimates generation/storage
cost without profiling or changing source evaluation. Defaults are fixed and
require no additional CLI configuration:

| Limit | Default |
|---|---:|
| Variants per original function | 4 |
| Allocated variants per compilation | 64 |
| Imported call-path depth | 16 |
| Additional body units per compilation | 4096 |
| Body units per new variant | 256 |
| Body units permitted per additional removed guard | 32 |

A candidate with zero demonstrated removed guards is rejected. A new body must
fit both its absolute size limit and `body_units <= 32 * additional_guards`.
When replacing an existing legal variant, `additional_guards` is the proposed
benefit minus that variant's benefit. Creating a duplicate equivalent plan is
therefore unprofitable. Reuse is considered before size/count budget rejection.
These are approximate AST-unit bounds, not an exact generated-byte or wall-clock
quota. They may decline a legal optimization.

Child plans are considered bottom-up. Allocated or subsequently discarded
children/wrappers conservatively consume allocation/unit budget; there is no
retry loop reclaiming and spending the same budget repeatedly. Equivalent
canonical dispatch eliminates redundant wrapper bodies, and unused variants
are pruned before C emission. A canonical checked target remains available at
every budget/cost fallback.

## Diagnostics

From `compiler/`:

```sh
go run . -optimize -optimization-report /tmp/leek-policy.json \
  -o /tmp/leek-policy -graph /tmp/leek-policy.dot examples/specialization-policy.lk
```

The JSON report contains `Analysis`, `Variants`, `Witnesses`, `Decisions` and
`Summary`; generated C is deliberately omitted. Each decision identifies caller,
full semantic path, target, considered variants and implication statuses,
proposed selection/action/reason, candidate cost, and `FinalTarget` after
canonical-wrapper simplification and ancestor selection. `Selected` describes
the considered plan; `FinalTarget` is authoritative for emitted dispatch. A
child proposal can be unused when an ancestor falls back or reuses another plan.
`CREATE` can therefore describe an allocated proposal that is later pruned;
the summary's `Generated` counts only emitted bodies.

Registry assumptions reference formal summary obligations and structured terms.
Actual bindings and exact known facts are available through the corresponding
call/obligation paths in `Analysis`. Static reuse counts count retained selection
queries, not runtime invocations. `Allocated` and `AdditionalUnits` conservatively
include pruned allocations; `RedundantAvoided` counts queries mapped back to
equivalent canonical dispatch. The compiler also prints a concise policy summary.

`-optimization-report` requires `-optimize`. `-constraints` retains priority:
inspection writes no executable, graph or policy report and needs no GCC.
Without optimization, default fully checked behavior remains unchanged.

## Semantics and limits

No argument, effect, condition or arithmetic operation is reordered. Applicability
uses facts valid at the exact execution point after required argument evaluation;
it never assumes that arguments or the entire surrounding expression succeed.
Earlier failures and remaining checked failures retain order, category and source
location. Existing direct-call/composition and conditional summary machinery is
shared; no new source inliner or second constraint system is used.

This is restricted to complete pure, nonrecursive target bodies and supported
unsigned binary arithmetic. Effectful callers are allowed, but their opaque
results do not acquire invented relationships. Signed specialization, collections,
SIMD, parallel execution, indirect/recursive calls, SMT, profiling, JIT,
deoptimization and autotuning remain deferred. See
[qualification](../qualification/specialization-policy.md).
