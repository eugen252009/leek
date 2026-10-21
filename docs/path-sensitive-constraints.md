# Path-sensitive scalar constraints

Status: implemented for the current acyclic scalar language. This extends the
[advisory constraint MVP](constraint-analysis.md); it does not itself rewrite
execution or reject unknown/violated obligations. The independent
[opt-in optimization consumer](proof-gated-optimization.md) has a narrower scope.

## Conditional execution and scopes (PS-CONTROL)

```leek
fn calculate(x u32) u32 {
    if x > 1 {
        let denominator u32 = x - 1
        return 12 / denominator
    }
    return 0
}
```

`if condition { ... }` optionally followed by `else { ... }` is a statement.
The condition must have type `bool`, is evaluated once, and only the selected
branch executes. There is no implicit truthiness, conditional expression,
`else if` shorthand, loop, or new return-type syntax. Nest another `if` inside
`else` when needed. `if` and `else` are reserved identifiers.

Each branch is a lexical child scope with its own sized scalar arena. Child
blocks can read parameters/enclosing locals and shadow their names; declarations
cannot escape to a sibling/parent scope. Returned scalar values are copies, not
escaping references. `return` exits the function, including from nested branches.
Value-returning functions must structurally return on every path. Executable
statements after an unconditional return or a conditional whose two branches
return are rejected. Constant conditions do not relax this validation.

Noncapturing nested functions in function bodies retain their existing behavior;
function declarations inside conditional blocks are explicitly unsupported.
Transitive effect verification includes conditions and both branches, even when
a branch is statically unreachable. Existing outer-function captures remain
unsupported; ordinary child-block reads are not function captures.

The resolver records block scope nodes/arenas, condition-to-block control edges,
and binding/return identities in the semantic graph. The AST retains true/false
branch selection, execution order, and short-circuit structure. DOT edges alone
are not an executable schedule and never force both branches to run.

## Environments and refinement (PS-PROOF)

Each traversal environment contains immutable-binding identities, exact integer
intervals, established predicates, a path condition, and consistency/reachability
information. Requirements are **never** premises for proving themselves.

Conditions are analyzed **before** using their truth value. The true branch
receives the condition; the false branch receives its complement. Supported
integer comparisons are `== != < <= > >=`, with signed/unsigned type domains and
mathematical `big.Int` arithmetic. Existing affine simplification permits facts
about such relations as `x - 10 != 0` to be related back to `x != 10`.

Boolean rules:

* `a && b`: evaluate/analyze b under a; the true branch establishes both.
* `a || b`: evaluate/analyze b under NOT a; the true branch retains alternatives,
  **not** the conjunction of a and b.
* `!`: complements comparisons, eliminates double negation, and applies De Morgan
  to supported conjunctions/disjunctions.
* False conjunctions and true disjunctions are conservatively approximated when
  their alternatives cannot be refined by this small domain.

Refinement records exact established predicates as well as interval bounds.
An interior exclusion such as signed `x != 0` need not be representable by one
interval to prove that exact predicate. General nonlinear/relational reasoning
is not attempted. `x > 0` does not prove that `x*x` fits in u32.

## Joins and returns (PS-JOIN)

Branches are analyzed independently. At a join, interval hulls and only common
predicates survive from reachable continuing predecessors. A returned or
established-unreachable predecessor contributes no continuation facts. Thus:

```leek
if x == 0 { return 0 }
return x - 1 // unsigned x is now nonzero
```

Conversely, a fact from a live branch cannot leak past a join with another live
branch. Earlier operations are classified at their execution point, never from
the function's final environment. Conditions on later statements do not justify
removing earlier checks.

Contradictory environments report `UNKNOWN` and
`assumptions_consistent: false`, not vacuous `PROVEN`. Such an operation has no
success requirement on reachable executions, but its guard is **not** eligible
for optimization. Unrecognized contradictions stay conservative/unknown.

## Pure calls and conditional output relations (PS-COMPOSE, PS-OUTPUT)

Ordinary calls and `->` use the same pure-summary substitution machinery.
All callee obligations, guards, arguments, and operation/call-path origins are
retained. Caller facts prove substituted requirements without changing the
callee's generic summary.

```leek
fn subtract(x u32, amount u32) u32 { return x - amount }
fn inverse(x u32) u32 { return 100 / x }
fn calculate(x u32) u32 {
    if x > 10 {
        return x -> subtract(10) -> inverse
    }
    return 0
}
```

Here `x >= 10` and `x - 10 != 0` are both `PROVEN` at the call sites.
The generic `subtract` and `inverse` remain `UNKNOWN` for arbitrary inputs.

Multiple scalar returns form a mathematical `select(condition, true, false)`
relation in the term DAG, with branch-refined conservative output ranges.
This is not speculative executable evaluation. Actual C still branches and
short-circuits. Return/output facts remain conditional on successful evaluation;
all computations required to reach a return retain their separate obligations.

## Structured inspection and proof identity

```sh
cd compiler
go run . -constraints examples/path-sensitive.lk
```

Report version **2** adds function-local obligation IDs, environment IDs,
environment snapshots, and guarded return relations. Every obligation retains
its original operation node, source location, and ordered call-site propagation
path. Environment snapshots record the path condition, available predicate/range
premises, and whether the path was demonstrated unreachable. They are available
premises, **not** a minimal proof core. IDs are deterministic for the same
validated graph, not persistent identifiers across source edits.

An internal operation-node reference connects proofs to the exact semantic
operation. JSON does not serialize that pointer and cannot authorize a compiler
transformation. Statuses refer to the requirement **when reached** under the
recorded premises. A whole-function derived precondition drops requirements
independently discharged from known facts; individual guarded requirements
remain visible. The example's `calculate` therefore has precondition `true` and
status `PROVEN`, while its raw pipeline requirements are still reported.

Inspection remains read-only: no generated artifacts/GCC, no check removal, and
no errors merely for `UNKNOWN` or `VIOLATED`.

## Limits

This is interval/predicate refinement over acyclic scalar control flow, not
SMT, general symbolic execution, a fixed-point solver, or arbitrary assertion
checking. Pure calls are summarized, not recursively executed by the analyzer.
Effects and floating-point relations remain opaque. Source bindings cannot be
reassigned and no mutable alias can invalidate a branch premise.

Existing term/obligation/expression/bit budgets remain in force; stored predicate
facts are bounded (approximately 256 per context). Resource exhaustion means
incomplete/unknown analysis. Limits are not hard adversarial time/memory quotas.
Branch nesting and shared mathematical relations can still make reasoning costly.

See [qualification](../qualification/path-sensitive-constraints.md).
