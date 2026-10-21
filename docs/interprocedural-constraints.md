# Interprocedural constraint contracts

Status: implemented for validated, direct, nonrecursive scalar pure functions.
The [specialization consumer](proof-guided-specialization.md) remains opt-in.

## Architecture assessment and integration points

The starting tree is commit `117bd09`. Its existing analyzer already contains
most of the logical machinery required for this milestone. A second constraint
system or source inliner would duplicate it:

| Layer | Existing authority | Integration |
|---|---|---|
| Typed AST and semantic graph | `parser/resolver.go`: expression types, bindings, exact node identities, resolved callees, verified transitive effects and acyclic calls | Remain unchanged; both ordinary calls and composition resolve to the same callee metadata |
| Symbolic terms | `constraints/terms.go`: interned mathematical DAG, parameter substitution, bounded simplification | Reuse for summary instantiation and output substitution |
| Contracts | `constraints/analyze.go`: bottom-up `FunctionReport` summaries | Reuse the summary representation; extend `CallInfo` with instantiated precondition, proof, guard, premise snapshot and completeness |
| Path analysis | `constraints/paths.go`, `proof.go`: immutable bindings, branch refinement, alternative joins, continuation/short-circuit guards | Reuse exact-site proofs; no conjunction of mutually exclusive branches |
| Optimization | `optimization/guards.go`: fresh exact local proofs | Retain local pass; add `specialization.go` to consume imported proofs by exact call path |
| C lowering | `parser/codegen.go`: ordered temporaries, checked helpers, conditional evaluation | Emit private body variants and change only proven call targets; preserve operand and statement order |
| Driver | `main.go`: canonical default, explicit `-optimize`, read-only `-constraints` | Preserve controls; report emitted variants when optimization is enabled |

No AST expansion, semantic graph mutation, source-language function declaration,
new dependency, solver, or language semantic revision is required.

## Summary representation

`constraints.FunctionReport` is the reusable function contract. All term IDs
are local to one `Report.Terms` arena; report IDs and readable strings are not
proof capabilities that can be reloaded into the optimizer.

| Field | Meaning |
|---|---|
| `InputFacts` | Parameter identities, types, and independently valid type intervals |
| `Obligations` | Individual arithmetic requirements, execution guards, source locations, operation identities, path provenance, proof status and environment |
| `Precondition` | Derived condition for successful completion, accounting for guarded obligations |
| `Output`, `Returns` | Symbolic successful output relationship, guarded returns and conditional ranges |
| `Pure`, `Complete`, `Limitations` | Verified effects and supported-analysis boundary |
| `Calls` | Actual argument substitutions, direct/composition kind, instantiated success condition and its call-site proof |
| `Environments` | Premise snapshots: predicates, ranges, path condition and consistency |

For `minus1(x u32) { return x-1 }`, the type domain is **0..4294967295**,
the success precondition is **x >= 1**, and the successful output relation is
**x-1**. The original input domain is never narrowed to the success domain.
The subtraction obligation records the possible underflow at the original
source location. Output ranges are explicitly conditional on success.

## Call-site proof propagation

The analyzer builds pure summaries bottom-up over the validated acyclic call
graph. At each call, it evaluates argument relations in source order, maps formal
parameter IDs to actual argument terms, substitutes every guarded obligation and
the output relation, and proves each requirement against the caller's established
environment. `Origin.Via` records the complete call-site sequence.

`CallInfo.Precondition` is the substituted aggregate contract; `CallInfo.Proof`
reports whether that aggregate is established at this call. An incomplete callee
cannot report proven aggregate success. Individual imported obligations remain
separate: a partially proven contract can still justify eliminating one guard
without eliminating another. The optimizer uses these individual proofs.

`subtract(a,10)` and `a -> subtract(10)` use the same mapping and proof machinery.
`a >= 10` proves their imported subtraction requirement. `subtract(10,a)` does
not inherit that proof: reversing argument positions changes the requirement.

For `x > 1` followed by `x -> minus1 -> divide_12_by`, output substitution
exposes both `x >= 1` and `x-1 != 0`; both are proven. The equivalent nested
direct call has the same obligations. Wrappers can carry these relationships
through additional pure calls, without source inlining.

Conditional summaries retain guarded requirements and `select` output terms.
One safe branch cannot establish the safety of an alternative branch. Returns
exclude terminated paths from the continuation, and short-circuit RHS obligations
exist only under their execution guards. Contradictory environments report
`UNKNOWN` with inconsistent assumptions; they never authorize check removal.

## Limits and inspection

`go run . -constraints examples/interprocedural.lk` emits advisory JSON and
writes no artifacts, even if `-optimize` is also specified. Existing report
version 2 gains additive call fields. Checked default behavior is unchanged.

Effects are opaque, floats have incomplete relations, and no required arithmetic
condition is assumed merely because an operation must satisfy it. A proven
callee operation does not prove earlier argument evaluation succeeds. Existing
term, expression and obligation budgets still apply; global analysis exhaustion
disables all optimization.

See [independent qualification](../qualification/interprocedural-constraints.md).
