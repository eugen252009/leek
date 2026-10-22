# Leek pure composition — scalar MVP

Status: implemented reference semantics for the scalar subset described here.
This document records the approved pure-composition decision and the initial
implementation limits. Collections and source-level constraint annotations below
remain **future design**. The separate [automatic constraint analysis
MVP](docs/constraint-analysis.md) now derives advisory implicit constraints without
changing these execution semantics. The subsequent [path-sensitive extension](docs/path-sensitive-constraints.md)
adds conditional blocks; [opt-in guard elimination](docs/proof-gated-optimization.md)
uses exact-site proofs without redefining arithmetic.

## 1. Repository assessment and compatibility

The preceding prototype had `fn`/`impure fn`, typed parameters, string-valued
`let` initializers, arena-backed C locals, and a scope/data graph. It did not
have calls, returns, expression typing, or transitive effect verification.
`PureScope`, `ImpureScope`, and `EffectEdge` already existed and are reused.

The implementation extends those structures; there is no second compiler IR.
Existing typed declarations, parameter copies, empty functions, and noncapturing
nested function declarations remain supported. The legacy programmatic
`LetStatement.Value` input is normalized to an expression by the resolver.
Native output and DOT paths retain their CLI behavior.

Intentional changes:

* A3/A4 in `axioms.md` now prohibit **all user-visible I/O** in pure functions.
  The old observational-printing exception is explicitly superseded.
* Primitive types, calls, literal ranges, and results are checked before C.
  Implicit C conversions and narrowing are not language behavior.
* Parameter/argument lists require commas; omission was previously accepted
  accidentally. Return types follow `)` directly, never `->`.
* Decimal literals are normalized; leading zeroes never imply C octal notation.
* Backend names are generated independently of source names, avoiding collisions
  with C keywords, runtime helpers, parameters, and temporary variables.
* Scalar arenas use typed union slots, with native alignment and C effective
  types. The compiler sizes each arena from its local declarations instead of
  imposing the previous arbitrary 4096-byte limit. No references escape.

## 2. Syntax and typing (PC-TYPE)

```leek
fn square(x u32) u32 {
    return x * x
}

fn add(x u32, extra u32) u32 {
    return x + extra
}

impure fn main() {
    let result u32 = 3 -> square -> add(2)
    print(result)
}
```

A function with no result omits the result type. `return` alone returns from
such a function. A value-returning function must have a value return. The current
scalar subset rejects executable statements after an unconditional return or
an `if` whose two branches return. Value-returning functions must return on every
structurally possible path. Nested function
declarations are not executable statements. `main`, if present, must be a
top-level parameterless function with no result; it can be pure or impure.

A return expression begins on the same line as `return`; it may continue across
lines. A newline after `return`, or a closing brace, means a bare return.
Other whitespace, including newlines in pipelines, does not terminate an
expression. Braces delimit function bodies. `//` and `/* ... */` comments work.

Supported scalar types: `u8/u16/u32/u64`, `i8/i16/i32/i64`, `bool`, `f32/f64`.
There are no implicit conversions between typed values. Decimal integer literals
are checked against their contextual type: declaration, result, parameter, or
an already typed operand. Otherwise nonnegative integer expressions default to
`u32`; an unconstrained unary signed expression defaults to `i32`. `true` and
`false` have type `bool`. Literal-only comparisons default their operands to
`u32`; use explicitly typed bindings for signed or wider comparisons.

Arithmetic is currently integer-only. Floating-point scalars may be passed,
returned, copied, compared, and printed, and may be initialized by in-range
decimal integer literals rounded to the declared precision by the GCC backend
(f32 constants are not first rounded to f64). Fractional literal syntax, floating-point arithmetic,
and explicit conversions are deferred. The compiler does not silently substitute
C arithmetic for unsupported operations.

## 3. Pure composition (PC-COMPOSE)

`source -> f(extra1, extra2)` applies `f` to the source value followed by the
additional arguments. `source -> f` is the zero-extra-argument form.
Every stage must be a statically resolved **pure** function returning a scalar.
No overload resolution, generics, function values, indirect calls, `_` placeholder,
or implicit external declarations exist in this subset.

Chains associate left. From weakest to strongest, precedence is:

1. `->`
2. `||`
3. `&&`
4. `== !=`
5. `< <= > >=`
6. `+ -`
7. `* / %`
8. unary `! + -`
9. direct calls and parenthesized expressions

The right-hand side of `->` is restricted to a function name or application.
Thus `x + 1 -> f` means `(x + 1) -> f`, and `(x -> f) + 1` requires the
parentheses. `x -> f + 1` is rejected, not silently assigned another precedence.
Nested compositions can be arguments: `x -> add(y -> square)`.

### Evaluation (PC-ORDER)

1. Evaluate the source exactly once.
2. Evaluate the current stage's additional arguments left-to-right, once each.
3. Apply the stage to those values.
4. Continue with the next stage only after this stage succeeds.

Ordinary call arguments and binary operands likewise evaluate left-to-right.
`&&` and `||` short-circuit: the right operand is evaluated only when needed.
Statements execute in source order. This defines observable effects and failure
order, not mandatory intermediate storage or machine instructions. Implementations
may change physical execution only when the defined behavior is preserved.

An ordinary nested call and the corresponding pipeline have the same result,
evaluation order, and failures, **provided the pipeline stages are pure**.

## 4. Purity and effects (PC-EFFECT)

`fn` is a checked purity contract, not a trusted annotation. `impure fn` is an
explicit effect boundary and remains impure even when its body currently only
calls pure functions. Pure functions cannot call effectful functions, directly
or transitively. They cannot observe external mutable state or perform I/O.

The resolver collects function signatures, resolves direct calls, rejects call
cycles, and computes transitive effect/failure summaries. It validates every
function, including unused functions. A pure wrapper around an impure function
is invalid; it cannot hide an effect from pipeline validation.

The intrinsic `print(value)` is a known I/O operation with one scalar argument
and no result. It emits the scalar and a newline to stdout and flushes it;
Booleans print as `0`/`1`, integers as decimal, and floating-point values use
9/17 significant digits for f32/f64. Reported output errors cause failure.
`print` is a reserved function name. Unknown library/external calls are rejected,
including calls inside impure functions: permission to perform effects is not
permission to assume an unknown ABI or unknown effects.

### Stage effects are not source effects

```leek
impure fn source() u32 {
    print(1)
    return 3
}

impure fn main() {
    let result u32 = source() -> square -> add(2)
    print(result)
}
```

This is valid. The source runs once; both stages are pure; the **whole expression
is effectful**. Effectful additional arguments are also supported in impure
scopes and run in the specified order. They do not turn the stage function into
an impure function, nor may their effects be absorbed into the pure region.
The same expressions inside `fn` are rejected. `value -> print` is always invalid.

### Local implementation mutation

Purity is not physical immutability. Initializing or modifying unaliased,
function-local compiler storage is permitted when it cannot affect any external
observer and no mutable reference escapes. The C backend uses local arena slots
and evaluation temporaries, including a temporary updated for short-circuit
Boolean evaluation. Source-level reassignment, mutable references, and ownership
transfer are not implemented; there is no new aliasing mechanism to trust.

## 5. Partial operations and failure (PC-FAIL)

Integer `+`, `-`, `*`, and unary negation are checked at their declared width.
Overflow and underflow terminate execution with status 1 and a source-located
runtime diagnostic. `/` and `%` fail on zero divisors. Signed minimum divided
by `-1` fails for **both** quotient and remainder operations. Signed division
otherwise truncates toward zero; remainder has the dividend's sign.

Out-of-range literals are compile-time errors. Arithmetic expressions, including
constant expressions such as `1 / 0`, currently use the runtime failure path.
The implementation does not depend on signed C overflow or wraparound.

A pure computation is a deterministic mapping from inputs to a value **or a
defined failure**; purity does not promise totality. The runtime's failure
reporting is a language mechanism, not permission for pure source functions to
print arbitrary observations. No later expression or effect executes after a
failure. Earlier effects remain observable. Unused results do not license
removing a computation that would fail.

These guarantees assume normal platform/resource operation. Stack exhaustion,
process termination by the OS, hardware failures, and external I/O success are
not claimed to be deterministic language arithmetic outcomes.

## 6. Semantic representation

* `Expr` preserves literal/name/unary/binary/call/**composition** operations,
  operand order, source positions, source input, and additional arguments.
* `ProgramGraph.Expressions` associates each resolved expression with its type,
  whole-expression `Effects`, composition `StageEffects`, `MayFail`, callee,
  binding, and graph node. Checked literal normalization retains the source AST.
* `FunctionInfo` retains the declaration, lexical parent, direct call sites,
  transitive effects, and transitive failure summary.
* `DataEdge` represents value dependencies. `EffectEdge` records the relative
  order of effectful call sites in each function. Scope containment is recorded
  by children; it is not confused with “parent must terminate before child.”
* Function calls refer to declarations rather than adding call-graph cycles to
  the value graph. Recursion is rejected **as an MVP restriction**, not because
  recursive calls and cyclic value dependencies are equivalent.

The AST and semantic annotations together are the current IR. DOT alone is a
visualization, **not an executable schedule**. In particular, short-circuit AST
structure records whether a right-hand operation executes. Effect edges order
executed call sites; they do not force skipped branches to execute. Any future
scheduler must retain these conditions and failure order.

The separate advisory constraint pass reads these operation/binding relationships
and reports derived requirements, conditional ranges, and proof statuses. It does
not modify this IR. A separate opt-in consumer may authorize local guard
elimination from independently proven exact-site obligations. There is no general solver
or source-level contract enforcement. Value flow, effect flow, and logical
analysis are not conflated. The backend does not destroy the semantic AST when
lowering it to C.

## 7. Optimization legality (PC-EQUIV)

The current backend is a reference lowering: explicit, typed temporaries make
C's unspecified argument evaluation order irrelevant. It emits checked helpers
and ordinary calls. GCC may inline or optimize the generated C under the normal
C rules; Leek does not currently implement fusion, inlining, or a scheduler.
Tests compare GCC `-O0` and `-O3` outputs, failure diagnostics, and status, and
compare direct calls against composition over bounded inputs.

Pure, deterministic, independent, associative, commutative, reorderable, and
fusable are separate properties. None is automatically inferred from type
compatibility. An optimization must preserve values, effects, arithmetic,
short-circuiting, preconditions, and failures. Speculating a potentially failing
operation from an unexecuted path is not legal merely because it is pure.

## 8. Implicit analysis and future contracts/collections

**Subsequent implemented extension:** inline `range`, `map`, `filter`, ordered
`reduce` and `fold` now have distinct semantic dataflow identities. See
[declarative dataflow](docs/declarative-dataflow.md) and
[collection semantics](docs/collection-semantics.md) for typing, per-element
failure order, setup effects and scope-bound representation. Optional
[fusion](docs/loop-fusion.md) preserves that order; purity alone never permits
reordering failing callbacks. The earlier broad collection sketch below remains
future design beyond this limited scalar-sequence subset.


For deterministic pure functions with complete definedness contracts:

```text
Pre(f composed with g)(x) = Pre_g(x) AND Pre_f(g(x))
```

This is backward propagation of a requirement, **not** proof that a caller
satisfies it. Forward facts describe established outputs. Original function
contracts remain unchanged by composition-specific requirements.

Under checked u32 subtraction, `div(12, minus1(x))` needs both `x >= 1` for
subtraction and `x - 1 != 0` for division, hence **x > 1**. Deriving only `x != 1`
would be unsound. The advisory analyzer now derives this checked-arithmetic
precondition; it does not assume or enforce it. Wrapping multiplication also invalidates mathematical-integer
simplifications such as interpreting `square(x) < 100` as `x < 10` in general.

Constraint records must distinguish declared, derived, proven, unknown,
runtime-enforced, and assumption-dependent conditions. The implicit-analysis MVP
implements derived obligations and conservative proofs; explicit declarations
and runtime enforcement remain future work. A solver timeout or
unsupported expression is unknown, never proof. No constraint syntax, guards
for user contracts, solver dependency, or `_` argument placement is added here.

`gather` and `scatter` remain deferred operations requiring explicit indexed
read/write, collision, bounds, and arithmetic semantics. General collection
`range`/`map`/`filter`/`reduce`/`fold` APIs remain future design; the implemented
inline scalar subset above has explicit semantic nodes rather than opaque calls.
Conditional `if`/`else` blocks are now implemented as lexical child scopes;
see [the control-flow specification](docs/path-sensitive-constraints.md).
Direct pure calls and composition now share reusable
[interprocedural contracts](docs/interprocedural-constraints.md). Opt-in
[proof-guided specialization](docs/proof-guided-specialization.md) selects
private implementations only after caller-specific proof discharge; the checked
canonical definition, effect order and failure semantics remain authoritative.
Arrays, loops, recursion, generic/indirect/external
calls, input/file I/O, SIMD, parallelism, and ownership/aliasing extensions are
also deferred.
