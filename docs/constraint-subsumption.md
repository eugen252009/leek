# Constraint implication and subsumption

Status: implemented as a bounded, deterministic extension of the existing
symbolic analyzer. No SMT solver or new source-language contract syntax is used.

## Starting architecture and publication gate

The previous interprocedural milestone was inspected and committed **before**
this extension as `eead78a307bf54fa4b0bee96cc396e102dedd244`, then pushed to
`origin/main`. A read-back of `refs/heads/main` confirmed that exact hash. Its
existing evidence contained 519 passed test/subtest outcomes, passing race output
and 11 killed mutations. Those were verified historical results, not new runs.

The existing `FunctionReport`, interned term DAG, exact-site environments and
bottom-up parameter substitution remain the authorities. The old registry
required identical eliminated-operation masks and child routes. The extension
adds `constraints.ImplicationEngine`, explicit variant contracts, and a
[policy](specialization-policy.md) that can reuse a weaker contract even when a
caller proves a different or larger safe-operation set. Lowering still receives
only freshly authorized exact-body operations and call destinations.

## Query semantics

`NewImplicationEngine(report)` owns a bounded copy of one report's term arena.
It checks term identities, acyclic dependencies, supported operations/arity and
integer representation. `Implies(function, environment, required...)` asks:

```text
all established facts in this exact premise snapshot
    imply
the conjunction of these required predicates
```

Required predicates are never inserted as assumptions. The engine reconstructs
the known predicate/range context and uses the analyzer's existing exact-integer,
logical and interval reasoner. It returns `PROVEN`, `UNKNOWN` or `VIOLATED` plus
a normalized premise view. Here `VIOLATED` means the conclusion is false for
every value in the consistent known domain. An overlapping domain with a
counterexample can remain `UNKNOWN`; it is never credited as proof.

`Compare(function, environment, left, right)` performs both hypothetical directed
queries. Two consistent proven directions establish equivalence. A demonstrated
empty intersection establishes contradiction. Otherwise the result can be a
one-way implication or remain unknown. These cases are distinct; a contradictory
premise does not prove everything. Inconsistent environments yield `UNKNOWN`
with `assumptions_consistent=false`.

The supported core includes type bounds, constants, equality/inequality, strict
and non-strict comparisons, conjunctions, finite interval relationships and the
existing bounded arithmetic relations. For integer `x`, examples include:

| Premise | Requirement | Result |
|---|---|---|
| `x > 10` | `x >= 1` | PROVEN |
| `x == 5` | `x > 0` | PROVEN |
| `x >= 5` | `x >= 1` | PROVEN |
| `x > 0`, unsigned type domain | `x >= 1` | PROVEN; equivalent |
| `x >= 0` | `x >= 1` | UNKNOWN |
| `x == 0` | `x >= 1` | VIOLATED |
| facts about an unrelated parameter | `x >= 1` | UNKNOWN |

## Canonical representation

`NormalizedEnvironment` contains sorted `IntegerDomain` records keyed by term
identity and integer type, each with inclusive bounds and sorted finite
exclusions. Simple variable/constant predicates are represented by these domains.
For example, `x > 0` and `x >= 1` have the same integer lower bound, while
`x >= 1 AND x >= 5` has lower bound 5. Equality 7 subsumes the redundant lower
bound in `x == 7 AND x > 0`.

Exclusions at an endpoint tighten the interval without inventing values:
`x >= 1 AND x != 1` normalizes to a lower bound of 2. Interior exclusions are
retained explicitly. Predicate and domain ordering does not depend on Go map
iteration. Variable IDs and signed/unsigned widths remain distinct.

Arithmetic, opaque, relational or otherwise unsupported predicates remain
structured residual predicates; other known ranges remain in `OtherRanges`.
Normalization never drops a required definedness obligation or treats a
successful-output relation as proof that its earlier arithmetic succeeds.
The normalized representation is for inspection and reasoning, not a text key
that can authorize another compilation.

## Checked arithmetic and boundaries

The DAG models exact mathematical relations, while separate obligations model
checked execution. Algebra is valid for those value relationships only when
execution reaches the corresponding successful values. Required overflow,
underflow or zero-divisor conditions are not erased. No finite-width wraparound
is introduced by normalization; out-of-domain bounds can instead establish an
inconsistent premise.

The existing term and integer-reasoning limits remain. Exhausted or unsupported
reasoning returns unknown and preserves checked execution. No general relational
solver, recursive fixed point, disjunctive enumeration or SMT theory is added.
The registry is scoped to one fresh analysis/lowering invocation, never a
cross-compilation proof cache. See [qualification](../qualification/constraint-subsumption.md).
