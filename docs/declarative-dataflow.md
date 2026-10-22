# Declarative dataflow — implemented scalar foundation

Status: parsing, semantic validation, canonical native execution, constraint
inspection, and bounded proof-safe fusion are implemented and qualified for the
subset below. See [collection semantics](collection-semantics.md),
[fusion legality](loop-fusion.md), and [qualification](../qualification/declarative-dataflow.md).

```leek
fn square(x u32) u32 { return x * x }
fn add(a u32, b u32) u32 { return a + b }
fn is_even(x u32) bool { return x % 2 == 0 }

fn process(n u32) u32 {
    return range(0, n)
        -> filter(is_even)
        -> map(square)
        -> reduce(0, add)
}
```

The source describes a range, independent maps, stable selection, and ordered
combination. There is no public iterator, array, function-pointer, generic, or
loop syntax. `->` still accepts only pure stages. Transitive callback effects and
call cycles are checked through the existing function graph; effectful bounds
and terminal initial expressions are allowed in impure scopes and retain order.

## Repository assessment and smallest extension

Baseline inspected: `9f2251ad907bb9178051fd16660d0ee15938ed08`, clean tree.
The existing compiler had all eight integer types, bool/f32/f64 scalar transport,
conditional statements, immutable source bindings, typed expression side tables,
direct-call/composition identities, explicit argument/effect order, and checked C
arithmetic helpers. It had no sequence values or function-reference values.

The constraint pass reads the validated semantic graph without rewriting it.
Its summaries, guarded obligation/environment IDs, implication/subsumption
queries, and fresh graph-bound specialization registry are retained. The registry
selects proven scalar variants under deterministic cost and code-size bounds;
those policies are not replaced by a new global planner.

Minimal additions:

* `ExprInfo.Dataflow` stores a distinct `Range/Map/Filter/Reduce/Fold` operation.
  The source AST still records composition, not rewritten opaque ordinary calls.
* Internal `seq:<element>` typing exists only while resolving a pipeline. Source
  signatures/declarations remain scalar; unconsumed/escaping sequences fail
  validation. A terminal yields an ordinary scalar usable by existing features.
* A callback argument is a statically resolved name with `ExprInfo.Reference`,
  not an indirect function call. It retains lexical visibility/capture rules.
* `DataflowOp` retains input-expression identity, bounds/initial expressions,
  callback, element/state types, element-major order version, and no-materialization
  contract. Existing expression nodes carry effects, possible failures, source
  positions and data edges. Callback-reference nodes supply dependency edges;
  callback call-graph entries support transitive purity/failure/cycle validation.
* Separate pull-iterator and direct-loop lowering consume this same semantic IR.
  Neither destroys it nor forces every dataflow node into an independent traversal.

Scope-local pull state is compiler-private stack/arena-region storage, not a new
user allocation or aliasing feature. Only scalar terminal values leave the scope.
This preserves axioms A1/A2/A5/A6; pure internal state mutation is permitted by A3.
DOT still visualizes dependencies, not a complete executable schedule.

## Constraints and diagnostics (DF-CONSTRAINTS)

```sh
cd compiler
go run . -constraints examples/dataflow.lk
```

Function reports have an additive `dataflow` array. Each stage records its node,
kind, input identity, types, order, materialization contract, and logical element.
Range stages retain bound relations and conditional `start <= element < end`
facts. Bounds/initial expressions retain ordinary scalar obligations and effects.

Callback summaries use the existing scalar analyzer and actual-parameter
substitution. Their guarded arithmetic requirements retain operation/call-path
provenance in `element_requirements`, explicitly scoped to **every reached
callback invocation**. Their environment is `-1`: these quantified records are
not scalar environment snapshots and cannot authorize scalar optimization.
Per-element output relations/type ranges apply only if that invocation succeeds.
Filter predicates are recorded separately. Fold/reduce initial values are
separate from a fresh logical accumulator: initial is not every later state.

This milestone deliberately does not solve universal element safety or evolving
accumulator invariants. Quantified statuses remain `UNKNOWN`; sequence-containing
summaries are incomplete and keep an unknown scalar success obligation. Ordinary
callers inherit that incompleteness rather than a fabricated proof. Even empty
or obviously safe domains may remain unknown in advisory analysis. Actual setup
failures can still be `VIOLATED` independently.

Existing local/scalar specialization remains available for independently proven
scalar callback bodies and setup calls. Per-element requirements are not fed into
its scalar variant registry. Fusion uses a different, explicit trace-equivalence
obligation; it never assumes those arithmetic requirements hold.

## Current scope

* All eight integer range types; all existing scalar callback/result types.
* Pure static map/filter callbacks, ordered reduce and fold, distinct IR identities.
* Fold state may differ from element type; Reduce requires identical types.
* Inline nesting, ordinary calls, conditionals/short-circuiting, and scalar
  computations surrounding terminal results retain their existing semantics.
* Source names `range`, `map`, `filter`, `reduce`, `fold` are reserved intrinsic
  function names. No other new source syntax was introduced.

First-class stored/returned sequences, general generics/higher-order functions,
arrays/mutation, user iterator protocols, gather/scatter, SIMD, parallelism,
recursion, SMT, JIT, speculative scheduling, profiling and autotuning remain
unsupported. Full quantified proofs and a combined global strategy/cost planner
are deferred, not silently approximated as proven.
