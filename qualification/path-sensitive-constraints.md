# Path-sensitive constraint analysis — qualification

## Assessment and artifact

**PASS for the exercised scalar subset.** Evidence is empirical, with exhaustive
finite input domains; it is not a formal proof for arbitrary programs or a
release authorization. Stage A was qualified before adding the optimization
consumer; subsequent full regression runs cover both together.

Artifact: changes on the working tree based on `e726089`, preserving the prior
uncommitted pure-composition and advisory-analysis milestones. Linux amd64,
Go 1.26.4, GCC 14.2.0. No new dependency or solver. Authority: `axioms.md`,
`pure-composition.md`, and the requested path-sensitive milestone, specified in
[the analysis model](../docs/path-sensitive-constraints.md).

The existing compiler had expression short-circuiting but no conditional
statements. Consequently Stage A extends parser, resolver, scope graph, and
canonical backend with `if`/`else`, block scopes/arenas, and all-path return
validation. This supersedes earlier evidence statements that the backend and
syntax were unchanged. Straight-line semantic behavior remains covered by the
original suites, which were rerun rather than assumed applicable unchanged.

## Contracts and checks

| ID | Required property | Executed evidence | Qualification |
|---|---|---|---|
| PS-CONTROL | Selected branch only; correct block reads, shadowing, scope expiry, returns, and effect/type validation | `parser/conditionals_test.go`: AST/semantic metadata, malformed syntax, non-bool conditions, missing return, unreachable statements, sibling escape, duplicate locals, nested capture/unsupported declaration and impure-stage errors; native nested branches at O0/O3 with UBSan | TESTED |
| PS-PROOF | Requirements do not prove themselves; use condition facts only after successful evaluation and at the exact execution point | `constraints/paths_test.go`: all comparisons and integer types, Boolean negation/short circuit, bounded addition, unknown multiplication, signed exceptions, condition's own failing arithmetic, no retroactive proofs | TESTED |
| PS-JOIN | No fact leakage; joins retain only facts valid on reachable continuing paths | Live/live joins, returned predecessors, nested survivors, interior exclusions, contradictions, impossible predecessor and a join that must not invent unreachability | TESTED |
| PS-COMPOSE | Same substitution path for pure calls/pipelines; preserve generic summaries and all callee obligations | Direct/composed subtraction plus inverse proven from a branch; original callee obligations remain UNKNOWN; call-path and operation provenance assertions | TESTED |
| PS-OUTPUT | Conditional return relations/ranges remain success-conditional and correct at callers | Piecewise output ranges and bounded independent interpreter comparisons, including branch-selected callees and repeated pure calls | TESTED within stated domains |
| PS-INSPECT | Stable structural operation IDs and branch-premise snapshots; no execution mutation | Report determinism, exact node references, environment/path IDs, proof consistency; C/DOT byte equality before/after analysis; CLI inspection without GCC/artifacts | TESTED |

Test paths above are under `compiler/bin/`; CLI checks are in
`compiler/proof_optimization_test.go`. Existing arithmetic/resource-limit,
parser/resolver, purity, composition, advisory-inspection and native tests all
pass. Unknown/violated constraints still do not reject programs.

## Independent bounded soundness oracle

`constraints/path_oracle_test.go` interprets the **typed source AST**, following
actual branch decisions, left-to-right calls, short-circuiting, and early returns.
It computes arithmetic using exact integers and independent explicit u8/i8
limits. It does not execute analyzer guards/contexts as its failure oracle.
At every reached operation it checks that PROVEN cannot fail and VIOLATED cannot
succeed. It also compares whole-function success, derived preconditions, and
successful output relations against the independent execution.

Executed finite domains:

* `TestPathProofsExhaustive8Bit`: **829,440 input/program valuations**. All 256
  values of each selected signed/unsigned 8-bit domain, six comparisons, six
  Boolean/control-flow forms, six operation/composition forms, seven unsigned
  and eight signed thresholds. Includes returning branches and live joins.
* `TestPathRelationalPairs`: **65,536 pairs** of u8 inputs under relational
  branch guards, exercising both operand orderings of subtraction.
* `TestConditionalCalleeExhaustive8Bit`: **552,960 input/program valuations**.
  Branch-selected pure outputs, six arithmetic/identity outputs, six comparisons,
  boundary thresholds, and four calling contexts including pipelines and repeated
  calls. Checks actual output relations as well as definedness.
* Total new bounded valuations: **1,447,936**. These count input/program pairs,
  not distinct arithmetic proofs or arbitrary-program coverage.
* The prior **655,360** u8/i8 binary arithmetic operand checks and affine/
  composition finite-domain tests remain part of the passing full regression.

Final 10-second bounded `FuzzPathProofSoundness` run: **14,761 executions**, no
reported failure. Each generated program is checked across all 256 inputs of its
selected 8-bit domain. The earlier Stage-A gate run completed **20,930 executions**
before adding output-relation checks. These runs overlap and are not claimed as
distinct exhaustive domains. Parser/resolver fuzz regression: **38,024 executions**
in the final 5-second bounded run, no reported failure.

## Isolated analysis mutations

Each mutation ran in a disposable copy of the full current compiler. The mutated
file was restored and byte-compared, then the copy deleted. Logs remain in
`/tmp/leek-path-mutations.0fpmu0hv` for this session.

| Mutation | Detector / meaningful failure | Outcome |
|---|---|---|
| Incorrect comparison complement | Bounded interpreter finds PROVEN addition failing at u8 input 251; negation fixture also fails | KILLED |
| Leak true-branch context into continuation | `no_leaking` fixture marks unguarded subtraction PROVEN | KILLED |
| Intersect lower bounds instead of taking a hull | Live-join no-leak fixture incorrectly proves following subtraction | KILLED |
| Drop pure-callee obligations | Explicit required obligation-count assertion in composition test | KILLED |
| Refine AND's RHS under the wrong truth value | Guarded division fixture changes from PROVEN to VIOLATED/UNKNOWN | KILLED |

**5 killed, 0 survived, 0 inconclusive** in these selected checks. This demonstrates
sensitivity to these injected defects, not a general mutation score or proof.

## Commands and limitations

From `compiler/`, with `/usr/local/go/bin` on PATH:

```sh
go test -count=1 ./...
go vet ./...
go test -race ./...
go test ./bin/constraints -run 'TestPathProofsExhaustive8Bit|TestConditionalCalleeExhaustive8Bit|TestPathRelationalPairs' -v
go test ./bin/constraints -run '^$' -fuzz '^FuzzPathProofSoundness$' -fuzztime=10s -parallel=2
go test ./bin/parser -run '^$' -fuzz '^FuzzParserAndResolver$' -fuzztime=5s -parallel=2
go run . -constraints examples/path-sensitive.lk
```

All passed. There is no unresolved counterexample in the exercised subset.
Unbounded program soundness, all wider integer valuations, general relational or
nonlinear reasoning, floating-point facts, effects' external success, recursion,
and arbitrary adversarial resource consumption remain unverified/unsupported.
Unreachable obligations remain UNKNOWN, not vacuously eligible for optimization.
Scope arenas still depend on normal native stack/resource operation.

The independent proof-gated consumer is qualified
[separately](proof-gated-optimization.md). The work was not deployed or released.
