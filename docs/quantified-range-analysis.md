# Quantified range analysis — model and implementation boundary

Status: implemented for the bounded interval-transfer and unsigned-additive
prefix subset below, with [qualification](../qualification/quantified-range-analysis.md).

This milestone extends the existing mathematical term arena, interval reasoner,
path contexts, and pure summaries. No language syntax or execution semantics change.
Canonical scalar/pull execution and element-major failure order remain authoritative.
Qualification is recorded separately; compiler PROVEN is not a formal proof of
all Go/backend code.

## Q-DOMAIN

A range identity denotes ascending `[start,end)`, over its existing integer type.
At its exact path, endpoint intervals are successful typed endpoint values.
For S=[s0,s1], E=[e0,e1], cardinality lies in
`[max(0,e0-s1), max(0,e1-s0)]`. All calculations use mathematical integers.
Known end<=start means empty. Otherwise each actual element lies in
`[s0,e1-1]`. The subtraction is mathematical and performed only after excluding
an empty upper domain; it never wraps at an unsigned zero endpoint.

EMPTY, NONEMPTY and POSSIBLY_EMPTY are distinct. Element facts are conditional on
an actual invocation; an empty range has no invented element interval. Its
universal conditions may be vacuous, but it authorizes no callback variant or
execution of skipped work. Cardinality/type facts do not escape into scalar paths.

## Q-TRANSFER

Callbacks reuse substituted pure scalar summaries and all their obligations,
including unused computations and intermediate direct calls/compositions.
An obligation is tested under incoming element/state intervals plus its own
execution guard, never under the condition it requires. Unsupported predicates
remain UNKNOWN. Quantified analysis makes no claim about the program's first
failure merely because some represented input might fail.

Output intervals are explicitly success-conditional and intersect the result type
only AFTER incoming obligations have been classified. This does not prove those
obligations. A later stage may use a prior callback's successful typed output,
because that stage cannot execute before the prior callback returns.

Filters have cardinality [0,K], not an assumed smaller K. Retained values may be
refined by the predicate's successful output relation using existing comparison
refinement. Unknown predicates preserve incoming bounds. A predicate that cannot
return true yields no retained elements; its own failures are still retained.

## Q-PREFIX

Only ordered unsigned Reduce whose complete scalar output relation is exactly
state+element receives the additive recurrence rule. Every other callback
obligation remains independently checked. For input [L,M] with L>=0, initial
[I0,I1], and at most K elements:

* every mathematical prefix is between I0 and I1+K*M;
* at a reached invocation with K>0, previous state is at most I1+(K-1)*M;
* a reached state is also constrained by its declared type, since all preceding
  checked steps must have returned successfully.

The last type intersection is a reachability fact, not a premise that the current
addition succeeds. The current addition is proved independently under the
previous-state bound. Nonzero initial values are not identities. No cancellation,
reassociation or final-only reasoning applies to signed/negative reductions.
General Fold recurrences remain UNKNOWN, while ordinary typed input facts and
independent callback requirements are retained.

## Q-SCOPE and optimization boundary

Each quantified proof names its domain, stage, original scalar operation/call
path, and a domain-scoped environment snapshot valid for every reached invocation.
Such contexts are not inserted into the scalar traversal environment or function
inputs. Empty/unreachable contexts cannot authorize specialization.

Quantified requirements remain separate from scalar function success obligations.
Sequence-containing scalar summaries remain incomplete: this milestone does not
export general sequence-result relations or prove every surrounding computation
safe. Per-stage/pipeline arithmetic proofs can nevertheless be PROVEN and may
support partial optimization. Solver/resource limits fail closed.

See [dataflow proof optimization](dataflow-proof-optimization.md) for the guarded
consumer and exact registry/budget boundary.
