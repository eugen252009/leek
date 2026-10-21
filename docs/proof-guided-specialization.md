# Proof-guided function specialization

Status: experimental, **off by default**. The fully checked canonical program
defines behavior. This extends the earlier [local guard pass](proof-gated-optimization.md).

## Use and observable evidence

From `compiler/`:

```sh
go run . -o /tmp/leek-checked -graph /tmp/leek-checked.dot examples/interprocedural.lk
go run . -optimize -o /tmp/leek-specialized -graph /tmp/leek-specialized.dot examples/interprocedural.lk
go run . -constraints examples/interprocedural.lk
```

Both executables print `12`, `1`, `6`, `0`, `0` on separate lines. The first
optimization diagnostic counts the original local guard elisions. Additional
`specialization:` lines identify real emitted variants, their removed guards,
and redirected child calls; these counts do not include discarded candidates.

`optimization.GenerateSpecialized` returns generated C, local elisions, emitted
variant identities, and eligible exact-site proof witnesses plus their fresh
analysis report. Witnesses refer to caller, call path, operation, obligation and
environment; a witness indicates eligibility, not guaranteed emission after
budget fallback. The report contains structured required predicates, argument
substitutions, guards and premises. These are inspection outputs only.

## Proof gate and representation

The consumer analyzes the **same validated graph** immediately before lowering.
It accepts no deserialized proof, user assertion, cached plan or arbitrary proof
string. Every eligible imported operation must satisfy:

1. Global analysis budgets were not exhausted.
2. Its exact imported obligation is `PROVEN` under consistent assumptions.
3. Its call path resolves, step by step, to the actual semantic callees.
4. Every cloned body is verified pure and completely analyzed.
5. The final origin belongs to that body's exact arithmetic node in this graph.
6. The operation is unsigned binary `+ - * / %` on u8/u16/u32/u64, and its
   entire current local contract has exactly one obligation.

Candidate trees are rooted in one analyzed caller, keyed by semantic call-node
identity, and carry only eligible operation masks. Intermediate pure wrapper
clones redirect their child calls; leaves remove the selected checks. All
unproven operations in every clone remain checked. Canonical bodies retain their
own independently justified local optimization decisions, never incoming caller
facts. Default generation remains fully checked everywhere.

Effectful or otherwise incomplete **callers** can supply valid exact integer
facts; their external success is not assumed. Their **callees** must still have
complete pure summaries. For example, a branch on `x` followed by an observable
mark and `minus1(x)` can specialize without moving the mark. `minus1(mark(x))`
cannot infer that an opaque effectful result equals `x`; its guard is retained
unless other established facts suffice.

## Identity, reuse and deterministic limits

Variants are private static C functions, not normal Leek declarations. A variant
is identified within this generation by its exact function-body identity, exact
eliminated operation set, and exact redirected child-call map. Arithmetic types
and semantics are fixed by those expression/body identities in the current
validated graph. There is no cross-compilation cache.

Its contract is the conjunction of the removed requirements under their
execution guards, including corresponding child-route requirements. Different
argument substitutions or stronger caller facts may share the same variant
**only after independent proof discharge at each call path**. Equal readable
predicates alone never authorize reuse. Caller-specific facts are never installed
as global function invariants.

Processing is deterministic (sorted functions and semantic call IDs), with
at most **4 variants per function**, **64 variants per compilation**, and
**16 imported call edges** per specialization candidate. Equivalent plans are
reused before applying budgets. New variants require a concrete guard elimination
or a child route leading to one. Budget exhaustion keeps the canonical call;
orphan children are pruned. The budget conservatively counts allocated orphan
candidates, so an abandoned parent cannot trigger repeated unbounded allocations.
These small limits favor inspectability, not maximum optimization coverage.

## Preservation argument

Lowering reuses the original body and ordered temporary generation. It changes
only the callee C name at an approved call and the helper expression at an
approved arithmetic node. Both operands and all call arguments still evaluate
once, in their original order, under the same condition and short-circuit blocks.
No operation is moved, speculated, folded across an effect boundary or inlined.

Every omitted guard was proven unable to fail whenever that execution point is
reached. All earlier failures still prevent entry; every remaining failure keeps
its original category, location and ordering. This does not assert that the
entire surrounding expression or effectful argument succeeds. Unsigned result
fit also protects u8/u16 arithmetic from unsafe C integer promotion.

The backend's `GenerateSpecializedChecks` is a trusted internal compiler boundary,
like `GenerateWithProvenChecks`; it is not a public proof-validation service.
Use the optimization consumer. Concurrent mutation of the graph during analysis
and lowering remains unsupported.

## Structural verification and scope

Tests isolate actual generated function bodies and helper/raw-operation
assignments. They require the original checked callee, a private clone with the
specific guard absent, an actual specialized **call expression**, and an unproven
call expression using the original function. Comments and aggregate counts alone
cannot satisfy these checks. Wrapper routes and partial guard retention are
checked separately. Canonical C and semantic DOT remain unchanged after analysis
and optimized generation; repeated optimized results must be identical.

No signed/unary arithmetic specialization, recursive or indirect calls, effects
inside variants, source inlining, SMT, general optimization, runtime guards for
contracts, JIT, deoptimization, collections, SIMD or parallel execution is added.
Conditional or unsupported relations that cannot be proven keep canonical checks.
This is a construction argument supported by bounded evidence, not a complete
correctness proof. See [qualification](../qualification/proof-guided-specialization.md).
