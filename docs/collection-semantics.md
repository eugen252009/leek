# Declarative scalar sequence semantics

Status: implemented for inline pipelines; [qualification](../qualification/declarative-dataflow.md)
and [the IR/implementation boundary](declarative-dataflow.md) are tracked separately.
Authority: axioms A1–A7 and the existing scalar composition/evaluation contracts.
No source loop, sequence annotation, indirect call, or generic type syntax is added.
The source forms are the range/map/filter/reduce/fold applications requested for
this milestone. These names become reserved intrinsic function names.

## Sequence model (DF-ORDER)

An inline pipeline defines a finite logical sequence, not an allocated array.
It must be consumed by `reduce(initial, combine)` or `fold(initial, step)` within
the same expression. Sequences cannot be stored, returned, passed to ordinary
functions, printed, compared, or discarded as unconsumed expression statements.
Static function names in callback positions are not first-class function values.

Evaluation is **element-major, pull-driven**:

1. Evaluate range start then end, once each, left-to-right.
2. Construct logical pure map/filter stages (no callback is invoked yet).
3. Evaluate the terminal initial expression once, even for an empty range.
4. For each source element in ascending order, execute its map/filter callbacks
   in pipeline order. A false filter skips every downstream callback and the
   accumulator update for that element. A retained element invokes the terminal
   callback once with (current accumulator, element).
5. Complete that update before requesting the next source element.
6. Return the final accumulator. Empty/all-filtered input returns the evaluated
   initial value. Stop immediately on the first defined failure.

Consequently this is NOT eager stage-major map evaluation. For two maps, a
second-map failure on an early element precedes a first-map failure on a later
element. No implementation may change that order merely because callbacks are
pure. Unused scalar terminal results still execute the entire computation.
Short-circuit/conditional surrounding expressions retain their existing rules.
Effects in bounds or initial expressions remain allowed in impure scopes and
occur in the order above; callbacks must be transitively pure.

## Domain and types (DF-TYPE, DF-RANGE)

`range(start, end)` has matching existing integer bounds, inferred from typed
operands, otherwise u32. All eight integer types are supported. Its domain is
`[start,end)`; start >= end is empty (not descending). Increment is exact: an
executed element is strictly less than end <= MAX, so advancing it cannot overflow.
No count calculation in a bounded integer type is required. Range construction
has constant storage, including for u64 domains. There is no hidden runtime
iteration cap: finite domains can take impractically long; OS/resource failure is
not promised to be a deterministic arithmetic result.

`map(f)`: f must accept one value of the input element type and return a scalar;
its result is the new element type. Relative logical order is unchanged.
`filter(p)`: p must accept the input element type and return bool; order of
survivors is unchanged. Both are distinct semantic operations.

`reduce(initial, combine)`: combine takes (element type, element type) and returns
that same type. Initial is checked against that type. Evaluation is ordered left
accumulation: A0 = initial and A(k+1) = combine(Ak, retained[k]), using partial
checked scalar application; the first failed application terminates execution. Neither identity nor associativity is assumed.

`fold(initial, step)`: step takes (state type, element type) and returns state
type. State may differ from element type. This is explicit ordered dependency,
not an independently reorderable map. Reduce and Fold retain distinct IR kinds.
All current scalar types may flow out of callbacks; range bounds remain integers.

## Failures, materialization, and transformation legality (DF-FAIL)

Every callback executes existing checked scalar semantics, including transitive
calls and signed minimum/-1 exceptions. Moving operations across elements or
filters, speculative calls, reassociation, or skipping unused failing work is
illegal without further proof. Initial is not a proven reduction identity.

No intermediate array or heap materialization is semantically required. The
canonical strategy is a chain of pull iterators, each holding private scope-bound
state. Any alternative must preserve exactly the same element/callback trace,
values, first arithmetic failure, and effects from setup/surrounding expressions.
A single direct loop can meet this rule without proving callbacks infallible,
provided a structural legality check establishes the same ordered trace.

The [quantified range extension](quantified-range-analysis.md) can prove supported
universal element and unsigned-additive-prefix requirements. Execution semantics
above do not change. Quantified arithmetic safety over a sequence is not assumed. Type/range facts,
callback requirements and their provenance must remain inspectable; unsupported
quantified proofs are UNKNOWN. Neither fusion nor a sequence wrapper authorizes
removing a callback's arithmetic checks.
