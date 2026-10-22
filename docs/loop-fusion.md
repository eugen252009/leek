# Proof-safe scalar loop fusion

Status: implemented, experimental, opt-in through the existing `-optimize` flag.
Canonical pull execution is independent and remains the default.

```sh
cd compiler
go run . -o /tmp/leek-canonical -graph /tmp/leek-canonical.dot examples/dataflow.lk
go run . -optimize -optimization-report /tmp/leek-decisions.json \
    -o /tmp/leek-fused -graph /tmp/leek-fused.dot examples/dataflow.lk
go run . -optimize=false -o /tmp/leek-canonical examples/dataflow.lk
```

The existing JSON specialization report adds `dataflow` decisions with function,
node, semantic stage kinds, `loop_fusion`, `FUSED`/`CANONICAL`, reason, legality
status, intermediate materialization, and iterator-state information. These
correspond to the plan actually passed to lowering. Inspection with `-constraints`
always remains read-only, even when optimization/report flags are also supplied.

## Two independent strategies

Canonical lowering generates scope-local pull descriptors and typed `next`
functions for Range, Map and Filter. A terminal while-loop requests one retained
value, performs one accumulator transition, then requests the next. Filter's
producer while-loop pulls/rejects values before yielding a survivor. This has
constant storage with respect to domain length, not stage-major arrays.

Fused lowering directly generates one ascending source-domain for-loop. Map calls
produce scalar temporaries in stage order; false filters `continue` without
running downstream callbacks. The reducer runs last and updates the accumulator
before the next element. All setup expressions are evaluated before iteration,
exactly once in the same order. No iterator descriptors, next-call chain, or
intermediate arrays remain for that fused pipeline.

Range→Map→Reduce fusion removes producer/consumer iterator state and call
boundaries. Range→Filter→Map→Reduce also combines distinct producer and consumer
pull loops into one direct loop, which structural tests explicitly require.
The canonical path already avoids arrays; this pass does **not** claim savings
from arrays that were never allocated. It demonstrates real loop/state fusion,
not a diagnostic-only change, and makes no benchmark claim.

## Legality gate (DF-FUSION)

The optimizer constructs a fresh graph-bound plan alongside, not in place of,
the existing bounded scalar-specialization registry. `FusionLegality` requires:

* A validated graph and exact semantic node/input identities.
* A Range source, a Reduce terminal, and one or more Map/Filter stages only.
* At most eight total semantic stages (including Range/Reduce).
* Matching element flow, verified transitively pure callbacks, the same
  element-major ordering version, and no required intermediate materialization.
* No analysis resource limit for the generation in which the plan is consumed.

Only `PROVEN` with consistent premises enters the lowering plan. Unknown legality
keeps canonical pull lowering. Fold, a bare Range→Reduce, and longer chains are
explicit canonical fallbacks in this milestone, though they execute normally.
Setup expressions may be effectful: the gate does not confuse their effects with
callback purity or move them across any iteration.

The proof is a **structural trace-equivalence argument**, not a universal proof
that callback arithmetic is safe:

For source elements `i` in `[start,end)`, define `P(i,acc)` as the ordered sequence
of callbacks for that element: maps, predicates, then the accumulator transition
if every predicate retained the element. A failed callback terminates the trace;
a rejected predicate truncates that element's downstream work. Both strategies
produce the same setup trace followed by the same ordered `P(i,acc)` sequence.
Therefore induction over source elements/stages preserves values, evolving
accumulator, retained order, and the first failing callback/source location.
The pull range advances its private cursor before yielding; the fused loop
advances afterward. That advance is unobservable and total because current <
end <= MAX, so this difference cannot introduce or reorder a defined failure.

This establishes the transformation's legality under validated scalar semantics,
without using an unknown arithmetic obligation as a premise. Callback operations
remain checked unless the **separate existing scalar proof pass** independently
discharges their exact-site guards. There is no map/filter reordering,
speculation, tree reduction, identity/associativity assumption, SIMD or parallelism.
A reduction is not reassociated and Fold is not misclassified as independent work.

`GenerateDataflowPlan` is a trusted internal lowering boundary, just like existing
scalar specialization boundaries. Public/serialized diagnostics are not reusable
proof capabilities. Source/semantic IR must not be concurrently mutated between
fresh analysis, planning and immediate lowering.

## Evidence and limits

Tests require checked canonical iterator code, actual direct-loop fused code,
absence of fused-pipeline descriptors/arrays, required fallback decisions,
deterministic JSON/C and unchanged canonical C/DOT before/after optimization.
An independent typed-AST oracle uses source syntax, exact arithmetic, and a
straightforward element loop; it does not reuse semantic-chain flattening,
iterator lowering, the fusion plan, or callback safety classifications.

Native comparisons at GCC O0/O3 use UBSan and compare results, stdout, exact
failure categories/locations, status, and test-only source-function traces
including arguments/accumulator values. Production purity is not weakened by
that test instrumentation. Uninstrumented CLI/example executions are also tested.
See [qualification](../qualification/loop-fusion.md) for actual finite domains,
fuzz runs and isolated mutations. This is empirical implementation evidence,
not formal verification of arbitrary programs, the Go compiler, GCC, or the OS.
