# Proof-gated dataflow arithmetic specialization

Status: implemented, opt-in under the existing `-optimize` flag. No new source
syntax or unchecked operator. Canonical generation remains fully checked.

```sh
cd compiler
go run . -constraints examples/quantified.lk
go run . -optimize -optimization-report /tmp/leek-quantified.json \
    -o /tmp/leek-quantified -graph /tmp/leek-quantified.dot examples/quantified.lk
```

The example's calculate derives actual elements [0,99], map results [0,9801],
count [0,100], previous states [0,970299], and all prefixes [0,980100]. Its
multiplication and reduction addition are PROVEN. Partial allows n<=65536:
map safety is PROVEN, reduction safety is UNKNOWN. Unbounded retains both checks.

## Architecture and authority (Q-OPT)

[Quantified analysis](quantified-range-analysis.md) uses the existing scalar term
arena, summaries and reasoner. A quantified ledger remains separate from scalar
function success obligations. Its snapshots are tagged with exact domain and
stage IDs and describe **every reached invocation**, not one sampled element.
Empty domains create neither invocation environments nor specialization witnesses.

The fresh optimizer validates the semantic call path from owning function through
the dataflow callback and any nested direct calls. It validates the original
arithmetic node pointer, supported unsigned binary operation and complete
single-operation safety contract. The stage/domain/environment must match.
Only exact PROVEN, consistent requirements that also pass the existing implication
engine can select guards for removal. Unsupported/incomplete callback summaries,
unreachable environments and resource limits fail closed.

These candidates enter the **same** specialization registry as scalar candidates.
The only interface extension is an explicit proof-ledger kind on a bound candidate.
Formal variant contracts remain scalar requirements on parameters and descendant
call paths. Applicability uses the current bound invocation context, so a variant
may be reused by another quantified domain or a scalar caller only if that
caller's own facts imply every formal requirement. UNKNOWN never counts.
Existing body/cost thresholds, per-function/global limits, implication caching,
canonical aliases and orphan pruning remain in force. A PROVEN requirement can
still retain its check if emitting a variant is not cost-eligible.

Callback routing works in **both** fused loops and canonical pull helpers.
Generic callback bodies stay checked. Scope-specific facts are not installed as
global function invariants or global backend proven-node entries. No per-element
variants are generated. The execution/fusion strategy is not changed to simplify
a proof; long unfused pipelines may still use proven callback variants.

JSON optimization reports add `quantified_range` records with the stage/domain,
computed bounds, selected callback target and actual additional guards removed by
that selected variant (including descendant variants). Eligibility alone is not
reported as an actual elimination. Existing scalar/fusion diagnostics remain.
Analysis reports add domain/cardinality, element, callback and prefix proof data;
full sequence-result scalar summaries remain incomplete as documented separately.

## Preservation argument

A private arithmetic guard is removed only when its formal requirement follows
for every actual invocation represented by the selected stage's domain. The
operation remains at the same scalar execution point, after the same arguments
and earlier checks. Earlier failing maps/filters/reducer work still prevent later
operations from executing. Downstream success-conditional bounds do not prove the
upstream operation that supplied them.

Unsigned additive prefix bounds are justified by ordered nonnegative mathematical
sums, not by final-only cancellation, identity or associativity assumptions.
Initial expressions still execute once, even for empty ranges. No reduction
reassociation or check motion is performed. Effects in setup/surrounding scopes
retain their original trace; callbacks remain pure. Internal state/IR must not be
concurrently mutated during fresh analysis, planning and lowering.

The construction argument is supported by structural C inspection, independent
bounded element/prefix enumeration, native reference comparisons, fuzzing and
mutations, not formal verification of all source programs or the native compiler.
See [qualification](../qualification/dataflow-proof-optimization.md).

## Limits

Only existing unsigned binary arithmetic guard specialization is enabled. Signed
bounds can be analyzed but signed/unary guard removal is not added. General folds,
non-additive/signed reduction recurrences, quantified exported return contracts,
SMT, SIMD, parallelism, arbitrary arrays, speculation, JIT and deoptimization are
deferred. Nonlinear/conditional transfer uses only existing conservative scalar
rules; unsupported relations stay UNKNOWN. A type-valid returned intermediate
is a reachability fact, never evidence that its generating operation cannot fail.
