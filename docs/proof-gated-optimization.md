# Proof-gated guard elimination

Status: experimental, **off by default**, deliberately narrower than constraint
analysis. Canonical checked arithmetic remains the semantic authority.

## Interface

From `compiler/`:

```sh
# Canonical: checked arithmetic at every integer operation.
go run . -o /tmp/leek-checked -graph /tmp/leek-checked.dot examples/path-sensitive.lk

# Experimental: remove only independently proven redundant local guards.
go run . -optimize -o /tmp/leek-opt -graph /tmp/leek-opt.dot examples/path-sensitive.lk

# Explicitly restore canonical lowering.
go run . -optimize=false -o /tmp/leek-checked examples/path-sensitive.lk

# Inspection never rewrites execution, even with -optimize also present.
go run . -constraints examples/path-sensitive.lk
```

Optimized compilation prints `proof-gated: removed N checked guards` in addition
to the usual compile message. The example removes **3** guards: two in
`local_calculate` and one in `bounded_add`. The pipeline has proven caller
obligations but its generic callees are deliberately not specialized.

## Construction and authority boundary (PG-GATE)

`optimization.Generate(validatedGraph)` runs fresh read-only constraint analysis,
selects eligible exact operation identities, and immediately calls the separate
backend lowering entry point. It accepts no cached plan, user assertion, JSON
proof, or string-keyed authorization. The canonical `CodeGenerator.Generate`
does not run analysis and does not consult optimization decisions.

Legality requires **all** of the following:

1. No analysis resource limit was reached.
2. The owning function is verified pure and completely analyzed.
3. The obligation belongs to the exact local operation in that function, not an
   imported caller-specific occurrence (`via` must be empty).
4. Its status is exactly `PROVEN`, with consistent assumptions.
5. The operation is supported unsigned binary `+ - * / %` on u8/u16/u32/u64.
6. Its entire current local contract consists of exactly that one obligation.

Otherwise the original checked helper remains. In particular, `UNKNOWN`,
`VIOLATED`, inconsistent paths, partial analysis, signed arithmetic, unary
negation, and operations in impure functions do not authorize elimination.

The backend's `GenerateWithProvenChecks` is a trusted compiler-internal boundary,
not a validation API for untrusted decisions. Use `optimization.Generate` to own
the proof gate. As with canonical generation, the validated graph must not be
concurrently mutated. There is no long-lived proof artifact to become stale;
the graph, facts, proof, and execution point are the same throughout one call.
Immutable source bindings and no shared mutable aliases are essential premises.

Caller-specific checks remain in generic functions because their other callers
may not establish the same conditions. Specialization/inlining is deferred;
propagating a call-site proof into a generic callee would be unsound.

## Lowering and arithmetic semantics (PG-EQUIV)

Canonical statements call the existing checked arithmetic helpers. At an eligible
site the optimized backend instead emits typed raw unsigned arithmetic after
lowering both operands to temporaries in the **same** original order. No operand,
call, condition, statement, or effect is moved, cloned, or removed.

The proof establishes the mathematical result's fit or divisor's nonzero value
whenever execution reaches that operation. Raw arithmetic therefore agrees with
the checked result. In particular, promoted u8/u16 arithmetic remains within its
declared unsigned width, avoiding accidental signed-C promotion overflow.
Signed guards are retained, including minimum divided/reduced modulo -1.

A removed guard cannot be the first failure because it was independently proven
unable to fail at that point. Earlier failures still stop execution; later
failures retain their original order, category, and source location. Required
conditions are never assumed to justify their own removal.

This is a construction argument under the validated scalar semantics, not a
formal proof of the Go analyzer or C compiler. It is supported by bounded
soundness oracles, structural checks, native comparisons, and targeted mutation
checks described in the [qualification report](../qualification/proof-gated-optimization.md).

## Structural evidence (PG-STRUCTURE)

Generated C labels each binary arithmetic execution point:

```text
// arithmetic <semantic node>: checked
... = lk_sub_u32(...);

// arithmetic <same node>: proven guard elided
... = (uint32_t)(... - ...);
```

Tests inspect the actual lowered statement as well as its associated obligation
and environment, not just this comment or the elision count. They also require
unproven helper calls to remain and canonical generation to remain unchanged
after optimization. Runtime helper **definitions** remain in both outputs;
counting definitions would not establish removal at any execution point.

Tests at GCC `-O0` demonstrate Leek-side elimination independently of GCC's own
optimizations. GCC `-O3` may additionally remove redundant checks in either
variant; that is not credited as this pass's work.

## Deferred

No signed/unary guard elimination, optimization of impure function bodies,
caller specialization, inlining, check merging, reordering, speculation,
guard insertion, runtime deoptimization, generic optimizer framework, collection
bounds, alias analysis, SIMD, parallelization, or JIT is implemented. Stronger
proofs can be added later without treating unknown reasoning as proof today.
