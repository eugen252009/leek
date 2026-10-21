# Automatic constraint analysis — advisory MVP

Historical milestone: the initial MVP below was **analysis-only**, with no new
syntax. It is now extended by [path-sensitive analysis](path-sensitive-constraints.md)
(report version 2) and a separate [opt-in proof consumer](proof-gated-optimization.md).
Statements below about absent conditionals/optimization describe the initial
milestone, not the current compiler. Inspection itself remains analysis-only.

Initial MVP status: implemented, **analysis-only**. No language syntax was added. Runtime
checks, evaluation order, effects, compilation acceptance, and the C backend are
unchanged. This implements implicit constraints, not source-level contracts.

## Repository assessment

The scalar compiler already provides the required authority boundary:

* `parser.Expr` retains operations, argument order, and composition stages.
* `ExprInfo` supplies validated types, constants, bindings, callees, and effects.
* `FunctionInfo` supplies direct calls and verified transitive effects.
* The resolver rejects recursion and unknown external calls before analysis.
* The C backend defines checked integer operations, including signed minimum
  divided **or reduced modulo** `-1`, and preserves short-circuit evaluation.

`compiler/bin/constraints` reads this existing representation. It does not modify
ASTs, graph edges, effect summaries, or backend annotations. Mathematical terms
are a small analysis DAG, not another executable IR: checked source expressions
and their mathematical relations have different definedness semantics. All
relations and obligations retain links to existing graph nodes/source positions.

The baseline parser, semantic, and native tests passed before this change. The
preceding uncommitted composition implementation was preserved.

## Inspection

From `compiler/`:

```sh
go run . -constraints examples/constraints.lk > /tmp/constraints.json
```

This mode parses and validates normally, emits only JSON, and does not invoke
GCC or write DOT/C/executable artifacts. It works for a library without `main`.
`UNKNOWN` and `VIOLATED` do **not** cause a nonzero exit status. Parse/type/effect
errors and report-writing errors still do. Without this flag the existing
compilation path is unchanged and does not run the advisory pass.

For the example's `calculate(x u32)` the actual report contains:

```text
Input fact: x in [0, 4294967295]
Derived precondition: (x > 1)
Proof status: UNKNOWN
Complete: true
Runtime checks: preserved
```

`complete: true` means the supported arithmetic success conditions were retained,
not that they hold for every caller. `safe()` calling `calculate(2)` reports
`PROVEN`; calls at 0 and 1 report `VIOLATED`. All still compile normally.

The Go API is `constraints.Analyze(validatedGraph) (*Report, error)`.

## Information categories (CA-FACTS)

1. **Input facts** are established by scalar parameter types. Integer literals
   have exact values, recorded as literal relations and singleton ranges.
2. **Required conditions** are obligations attached to checked operations. They
   are never added to the context used to prove themselves.
3. **Derived preconditions** are conjunctions of the obligations, substituted
   onto original function inputs. Derivation is not proof or enforcement.
4. **Success-conditional facts** describe intermediate/result values **if they
   are evaluated and complete successfully**. Their ranges may intersect the
   mathematical result with the declared type range. They do not assert that
   the operation is reached or succeeds.

In particular, a mathematical `u32` sum may exceed `4294967295` during reasoning.
Clamping it to `u32` before proving its overflow obligation would be circular and
unsound. Only the separately labelled post-success range is intersected.
An empty conditional range cannot prove arbitrary predicates.

## Type and arithmetic rules (CA-TYPE, CA-ARITH)

All existing integer types are supported: `u8/u16/u32/u64` and
`i8/i16/i32/i64`. For unsigned width `w`, bounds are `[0, 2^w - 1]`; for signed
width `w`, `[-2^(w-1), 2^(w-1)-1]`. Boolean inputs have their existing two-value
domain. No new language type is introduced.

Reasoning uses `math/big.Int`. JSON numbers representing mathematical integers
are decimal **strings**, so u64 bounds and larger products are not rounded by
JSON consumers.

For successfully evaluated, correctly typed operands:

| Operation | Additional success requirement |
|---|---|
| unsigned `a + b` | mathematical `a + b <= MAX` |
| unsigned `a - b` | `a >= b` |
| unsigned `a * b` | mathematical `a * b <= MAX` |
| signed `+`, `-`, `*` | `MIN <= mathematical result <= MAX` |
| integer `/`, `%` | `b != 0` |
| signed `/`, `%` | additionally `NOT(a == MIN AND b == -1)` |
| unary `-a` | checked `0 - a`, using the corresponding type rules |
| unary `+a` | no additional requirement |

Requirements of operand computations are retained independently. Unsigned
addition/multiplication need no separate lower check because successful typed
operands are nonnegative. Subtraction's upper bound follows from those same
operand invariants; none of this assumes earlier computations succeed.

Division/remainder relations use truncation toward zero, matching Leek, not
floor division. Exact constant reasoning may discover a definite failure; it
remains an advisory finding, not a new compilation rejection.

## Composition and backward propagation (CA-COMPOSE)

Each pure function has an acyclic summary containing an output relation and
**all evaluated-body obligations**, including unused declarations and calls.
An ordinary pure call and a pure composition stage substitute the already
analyzed argument relations into that summary. The pipeline source occupies the
first parameter slot. Callee summaries are not changed by caller-specific facts.

Conceptually:

```text
Pre(f(g(x))) = Pre_g(x) AND Pre_f(g(x))
```

Both source/argument evaluation requirements and stage requirements are retained.
For the example:

```text
minus1(x):       requires x >= 1; result relation x - 1
divide_12_by(y): requires y != 0
substitution:   x >= 1 AND (x - 1 != 0)
simplification: x >= 1 AND x != 1
combined:       x > 1
```

This is exact within the supported scalar, acyclic, checked-arithmetic subset.
It does not invert functions or assume that `x > 1` is established. An expression
such as `(x + 1) - (x + 1)` may have constant result relation zero, but retains
both additions' overflow conditions. Algebra on the result never erases their
requirements.

Short-circuit right-hand requirements are guarded:

```text
left && right: Pre_left AND (left => Pre_right)
left || right: Pre_left AND (NOT left => Pre_right)
```

The DAG represents implication as `NOT guard OR required`. A skipped failing
branch is not an unconditional failure. Requirements for evaluating the guard
itself remain present. There are no new conditional statements or loops.

## Forward ranges

Supported forward ranges include type/literal bounds, interval `+`, `-`, `*`,
truncating division when the divisor interval excludes zero, and conservative
remainder bounds. Multiplication considers all endpoint combinations for signed
values. No bounded host arithmetic is used to multiply endpoints.

Pure calls propagate their symbolic result relations. For example, `x + 5`
with a u32 input has mathematical interval `[5, MAX+5]`, obligation
`x <= MAX-5`, and conditional output interval `[5, MAX]`.

Function output refinement may use the derived precondition **only in a separate
success-conditional context**. That context never discharges the precondition.
If the precondition is inconsistent, the output range is marked empty rather
than inventing an output from a failed execution.

## Simplification and proof (CA-PROOF)

The small simplifier implements:

* Exact constant arithmetic/comparisons, retaining partial division if undefined.
* Safe logical constants, conjunction flattening/deduplication, and guarded
  tautologies where terms have known mathematical definedness.
* One-variable affine comparison isolation, including negative coefficients
  and exact integer floor/ceiling for inequalities.
* Intersections of same-variable bounds, equalities, and excluded endpoint
  values. Thus `x >= 1 AND x != 1` becomes `x > 1`.
* Contradictions such as `x == 5 AND x > 10`.
* Removal of predicates independently proven from incoming type/constant facts.

The prover uses constants, supported logical reasoning, and conservative exact
intervals. It is not a general satisfiability solver. Unsupported nonlinear
relations stay symbolic; an overlapping interval is not a proof of truth or
falsity. Reasoning never borrows a conditional output range as an unconditional
premise for the same operation's safety.

Statuses are relative to the recorded incoming domain:

* `PROVEN`: the condition holds for every valuation in that domain by supported
  reasoning. It does not mean that runtime checks were removed.
* `VIOLATED`: the condition is false for every valuation in that domain. Merely
  finding one failing input is insufficient.
* `UNKNOWN`: neither conclusion is established, or analysis is incomplete.

An individual obligation's status is not a prediction that its operation is
reached or is the first failure. Earlier evaluation requirements remain separate;
the combined precondition describes successful completion, not a reordered
execution or a replacement runtime diagnostic.

Known inconsistent incoming interval assumptions yield `UNKNOWN` with
`assumptions_consistent: false`, never a vacuous successful proof. In contrast,
a contradictory **required precondition** under consistent input facts is
`VIOLATED`. General contradictions outside the supported fragment may remain
unknown. There is no interface for treating arbitrary programmer assertions as
proven input facts.

## Effects and unsupported values

Effectful calls are opaque boundaries, not inlined pure summaries. Each call site
has its own output identity and unresolved success atom, even when the current
callee body happens to return a constant. Argument evaluation requirements are
still analyzed, in order. A result's declared type range applies **if that call
returns** and is explicitly reported as an auxiliary conditional fact.

Obligation proofs using such type ranges describe the mathematical condition for
possible typed returned values; they do not prove that the call occurs or returns.
Whole-function reports with unresolved effects are incomplete/unknown unless an
independent arithmetic contradiction already rules out successful completion.

Floating-point values and comparisons are opaque. In particular, rounded float
literal spellings are never mistaken for exact integer facts, and float equality
is not simplified using integer identities. This is conservative even for some
obviously harmless floating-point functions.

## Structured report and resource bounds (CA-INSPECT)

`Report` is versioned JSON with a term table. Each term's `args` are IDs into that
DAG, preserving shared relations instead of exponentially expanding text.
Function reports contain:

* input and auxiliary facts with explicit availability scopes;
* individual guarded obligations, proof status, source position, origin function,
  graph node, and call-site propagation path;
* the simplified derived precondition, bounded human-readable rendering, and
  its independent proof status;
* conditional output/intermediate ranges and symbolic relations;
* call-site argument mappings and ordinary-call/composition distinctions;
* completeness and explicit limitations.

The original typed AST is retained untouched. Value flow, effect flow, and this
logical analysis remain separate. Diagnostic strings are explanatory; tests also
inspect the structured terms and statuses.

The MVP bounds term creation to 50,000 nodes, obligation expansion to 4,096 per
function (plus one unknown-limit marker), and expression visits to 20,000 per
function. Exceeding these limits produces incomplete analysis and an unknown
obligation, not a proof. The `unknown` term is a lack of information, not a
shared symbolic value: two occurrences do not establish equality. Fact labels
retain source identities even when their relation cannot be represented.
Mathematical folding/range reasoning beyond a 4,096-bit
working bound is declined; the symbolic relation is retained. Rendered text is
also bounded and may use `term#N` references. These are reasoning limits, not new
runtime limits. They are not a hard wall-clock or memory quota for arbitrary
source programs.

## Deferred

No source annotations, generic SMT solver, general relational/path-sensitive
solver, collections, array bounds, gather/scatter, SIMD, parallel scheduling,
recursion, interprocedural fixed points, guard insertion, speculative execution,
or JIT specialization are implemented by this analyzer. Separate opt-in
[local optimization](proof-gated-optimization.md) and
[function specialization](proof-guided-specialization.md) consume its proofs.

See [qualification evidence](../qualification/constraint-analysis.md) for actual
executed tests, finite-domain coverage, mutations, and preservation limits.
