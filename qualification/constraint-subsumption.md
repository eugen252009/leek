# Constraint subsumption — qualification

Assessment: **PASS for the bounded integer implication subset**. Base milestone
`eead78a307bf54fa4b0bee96cc396e102dedd244` was committed, pushed to `origin/main`
and verified remotely before this implementation began. Its historical evidence
was inspected: 519 passing test/subtest outcomes, passing race output and 11
killed mutations. No new qualification result was inferred from those old logs.

Environment for this extension: Linux amd64, Go 1.26.2, GCC 14.2.0.
[Query semantics and architecture](../docs/constraint-subsumption.md).

## Independent logical gate

`TestConstraintImplicationDirectionAndNormalization` tests both implication
directions and independent expected classifications. It covers strict/non-strict
integer equivalence, stronger/weaker bounds, equality, inequality, conjunctions,
unrelated parameters, contradiction and residual arithmetic information.
Normalized forms for `x > 0` / `x >= 1`, redundant lower bounds and redundant
equality bounds must match as structured data, preserving variable identity.

`TestImplicationBoundariesAndNoVacuousProof` checks impossible u8 domains, no
proof from inconsistent assumptions, exclusion-driven endpoint normalization,
and rejection of foreign term/environment identities.

`TestImplicationWideIntegerBounds` checks exact known domains and strict positive
bounds for all eight unsigned/signed widths through real parsed/resolved calls.
This qualifies implication reasoning, not signed specialization.

`TestImplicationIndependentU8Oracle` executes **451,584 independent valuations**:
six premise comparisons × seven thresholds × six required comparisons × seven
thresholds × all 256 u8 inputs. Concrete integer comparison is independent of
the interval engine. Every reported PROVEN forward implication must have a
nonempty premise domain and no counterexample; VIOLATED must have a nonempty
domain and no successful conclusion. Full pair enumeration also exercises the
reverse direction. Empty domains cannot produce vacuous proofs.

## Bounded fuzzing

`FuzzIntegerImplication`, requested 10 seconds with two workers, completed
**143,715 cases** without a reported failure. Each generated comparison pair is
checked against every u8 value. This is sampled comparison/program generation,
not an unbounded proof. The dedicated finite threshold matrix above is separate.

```sh
# From compiler/, with the intended Go on PATH:
go test -count=1 -v ./bin/constraints -run 'TestConstraintImplication|TestImplication'
go test ./bin/constraints -run '^$' -fuzz '^FuzzIntegerImplication$' -fuzztime=10s -parallel=2
```

New direction, normalization, missing-binding, incompatible-semantics and
self-assumed-requirement mutations were killed. Full mutation, native regression,
race and vet results are recorded in [policy qualification](specialization-policy.md).

## Limits

The engine reuses exact integer/interval reasoning and preserves residual
definedness predicates. General relations and overlapping domains can remain
UNKNOWN; no theorem prover completeness is claimed. Resource exhaustion remains
UNKNOWN. Checked mathematical relationships do not imply that earlier argument
evaluation or arithmetic completes. No unsupported relation is strengthened to
make a specialization eligible. No required qualification gate remains failed.
