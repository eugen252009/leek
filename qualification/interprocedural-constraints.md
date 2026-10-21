# Interprocedural contracts — qualification

Assessment: **PASS for the supported direct, nonrecursive scalar subset**.
Starting commit: `117bd09`; Linux amd64, Go 1.26.2, GCC 14.2.0. These results
describe the working implementation, not a pushed commit or published release.

## Gates A–C

Architecture and integration points are documented in
[interprocedural constraints](../docs/interprocedural-constraints.md). Initial
unmodified tests passed in all four Go packages. Existing summaries, substitution,
path environments, proof classifications and output relations were reused rather
than introducing a second logical system for specialization.

`TestReusableSummaryAndInstantiatedCalls` independently checks the reusable
two-parameter subtraction summary, its full u32 type domain, symbolic output
relation, unknown generic success condition, and individual arithmetic obligation.
It checks proven direct/composition calls, an unknown call, reversed argument
substitution yielding a violated condition, and a definitely failing constant
call. Call-site proof/environment/completeness metadata is inspected separately
from the unchanged generic contract.

`TestInterproceduralProofIndependent8BitOracle` evaluates **55,296**
input/program valuations: six comparisons × six thresholds × six direct,
composition, nested and conditional call bodies × all 256 u8 inputs.
The existing test interpreter follows source AST evaluation and checked integer
semantics independently of analyzer predicates. It checks classified obligations
at their actual execution paths, including call-site provenance. This supplies
an independent soundness gate before using imported proofs for specialization.

`TestIncompleteCallCannotReportProvenSuccess` verifies that partial float
summaries and opaque effectful calls cannot claim proven aggregate success.
The existing conditional-callee and path-oracle suites continue to run, including
alternative joins, termination, short circuits, type boundaries, and inconsistent
assumptions. No zero-test run or skipped required gate is credited as validation.

## Outcome and limits

Summaries distinguish full typed inputs from successful-execution preconditions.
Caller facts discharge imported requirements; requirements are never assumed
merely to prove themselves. Output relations are success-conditional. Complete
pure summaries support chains without source inlining; incomplete relations keep
canonical execution.

Full-suite, race, vet, native, fuzz and mutation evidence for the combined
implementation is recorded in [specialization qualification](proof-guided-specialization.md).
There are no known unresolved counterexamples within the tested scope. Finite
8-bit valuations and sampled wider arithmetic are not a proof for all programs.
Recursion, indirect calls, general relational solving and signed specialization
remain outside this milestone.
