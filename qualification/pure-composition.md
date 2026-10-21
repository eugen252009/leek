# Scalar pure-composition qualification

Historical milestone evidence, supplemented (not promoted to formal proof) by
[path-sensitive](path-sensitive-constraints.md) and
[proof-gated optimization](proof-gated-optimization.md) requalification.

## Scope and assessment

**PASS for the implemented scalar reference path**, with empirical evidence only.
This is not a proof of all-input correctness, a self-hosting milestone completion,
or a release approval. No publishing, deployment, or release was performed.

Artifact: working-tree implementation based on `e726089`, including the preceding
uncommitted fixes for local copies, empty functions, parameter validation, and
unsupported captures. Those changes were preserved/extended rather than reset.
Environment: Linux amd64, Go 1.26.4, GCC 14.2.0. The C backend depends on GCC's
checked-overflow builtins; no new external package or solver was installed.

Authority: the approved pure-composition decision, revised `axioms.md`, and
[pure-composition.md](../pure-composition.md). The removal of the pure-output
exception is intentional and explicitly documented, not claimed as preservation
of the older rule.

## Contracts and executed checks

| Contract | Conditions and required behavior | Evidence | Status |
|---|---|---|---|
| PC-TYPE | Valid scalar operands/arguments/results must agree; malformed or unsupported programs receive located diagnostics before native artifacts | `TestParser_CompositionStructureAndPrecedence`, `TestParser_RejectsMalformedComposition`, `TestComposition_SemanticRejections`, existing parameter/parser tests, CLI example tests | TESTED |
| PC-COMPOSE | Every stage is a resolved pure scalar function; source result is passed first, without textual substitution | Simple/chained/nested/extra-argument runtime cases; direct-call equivalence; explicit and transitive impure-stage rejection | TESTED |
| PC-EFFECT | Pure functions have no transitive user I/O; impure declarations retain their boundary; expression and stage effects are distinct | IR separation test, transitive rejection tests, effectful-source example, unknown external/indirect call rejection | TESTED |
| PC-ORDER | Source and arguments run once in defined order; short-circuit branches are conditional; argument failure precedes stage application | Observable `1,2,3,123` trace, ordinary call order, short-circuit effect/failure tests, `TestComposition_ArgumentsFailBeforeStageApplication` | TESTED |
| PC-FAIL | Integer operations either produce the defined in-range result or fail deterministically, without subsequent effects | All eight integer widths' min/max, out-of-range literals, overflow/underflow/multiply checks; signed minimum division/remainder, zero divisors, unary negation; unused result failure; earlier/later effect checks | TESTED |
| PC-EQUIV | Reference lowering preserves values, output order, and defined failure behavior under native optimization | Runtime tests at GCC `-O0` and `-O3`; direct versus composed calculation checked against `i*i+3` for all `i` in `[0,256)` | TESTED |
| Scope/lifetime preservation | Scalar local stores are private, returned values are copies, no capture/escaping-reference mechanism is introduced | Existing local-copy tests now use scalar returns and independent runtime assertions; nested noncapturing functions; rejected captures; UBSan-backed runtime tests | TESTED within scalar subset |

The resolver's cycle check still validates the value/effect graph. Recursive call
graphs are rejected separately with an unsupported diagnostic; their rejection
is an implementation restriction, not a theorem that recursion violates a DAG.

The minimum successful arithmetic witnesses, malformed programs, boundary
failures, and effect traces are executable fixtures in:

* `compiler/bin/parser/composition_test.go`
* `compiler/bin/parser/codegen_test.go`
* `compiler/bin/parser/arithmetic_test.go`
* `compiler/composition_test.go`

Native test binaries use `-Wall -Wextra -Werror`, with
`-fsanitize=undefined -fno-sanitize-recover=all`. Checked failures must be Leek
runtime diagnostics, not C undefined-behavior reports.

## Commands actually executed

From `compiler/` (Go's bin directory was added to PATH for `gofmt`):

```sh
go test -count=1 ./...
go vet ./...
go test -race ./...
go test ./bin/parser -run '^$' -fuzz '^FuzzParserAndResolver$' -fuzztime=5s -parallel=2
go run . -o <temporary>/sample -graph <temporary>/sample.dot
go run . -o <temporary>/composition -graph <temporary>/composition.dot examples/composition.lk
go run . -o <temporary>/effects -graph <temporary>/effects.dot examples/effectful-source.lk
go build -o <temporary>/leek .
```

Results:

* All unit, semantic, CLI, and native runtime tests passed; vet and race checks
  passed. Modified Go files were formatted; `git diff --check` passed.
* The bounded fuzz smoke run reported 29,516 executions with no reported failure.
  Fixed regression cases cover trailing two-character operators at EOF; lexer
  token joining must not consume EOF repeatedly. Fuzzing is not a proof of parser
  termination or complete malformed-input coverage.
* Sample compiled and executed successfully. Composition printed `200`.
  Effectful-source example printed `10`, `2`, `102`, each on its own line.
* Invalid examples fail semantically before graph/binary output. Temporary C
  files are removed by the CLI. Generated DOT was inspected for distinct data
  and effect edges; generated C was compiled and executed by the tests.
* Pipeline artifacts and compiled binaries were kept outside the repository;
  the existing tracked sample `compiler/graph.dot` was not rewritten.

## Test effectiveness: isolated mutations

Three disposable copies of `compiler/` were mutated independently. Each started
from the passing implementation; only the mapped check was run. Mutated files
were restored from the working baseline, compared byte-for-byte, and the copies
were removed. The user's working source was never mutated for these checks.

| Mutation | Targeted test | Observed failure | Outcome |
|---|---|---|---|
| Evaluate pipeline source twice | `TestComposition_Runtime/effectful_source_and_argument_order` | Repeated `1,1,2,3,...` observations instead of `1,2,3,123`, at both `-O0` and `-O3` | KILLED |
| Ignore arithmetic overflow builtin result | `TestCheckedArithmetic_RuntimeFailures/unsigned_overflow` | Status 0 and no diagnostic instead of checked failure | KILLED |
| Disable both early and transitive stage-effect rejection | `TestComposition_SemanticRejections/impure_stage` | Resolver returned success for an impure stage | KILLED |

These demonstrate sensitivity to those three defects, not completeness of the
suite. They are not a general mutation score or formal verification.

## Logical findings and limits

* Purity and definedness are independent. A stage can have `StageEffects == none`
  while `MayFail == true`. An effectful source makes the expression effectful
  without changing stage purity.
* Checked unsigned subtraction plus division gives `x > 1` for
  `div(12, minus1(x))`. Tests exercise underflow at 0, division failure at 1,
  and successful evaluation at 2. No symbolic derivation is claimed.
* The source AST is retained for operand order, short-circuit conditions,
  bindings, calls, and future contract reasoning. DOT alone is not a schedule.
* Source-level mutable bindings are **not implemented**. Tested local mutation
  is compiler-owned arena initialization and Boolean temporary updates, with no
  external aliases. There is no general ownership/escape analysis to qualify.
* No Leek fusion/inlining/collection optimization was implemented. `-O0`/`-O3`
  comparison qualifies this C lowering on the tested toolchain, not a future
  optimizer or all architectures.
* Full collection semantics, conditional statements/loops, recursion, foreign
  and indirect calls, floating-point arithmetic, input/file APIs, constraints,
  solver proofs, SIMD, parallel scheduling, and general pointer/arena transfers
  remain unimplemented and unverified.
* I/O failures depend on the environment; output is flushed and reported errors
  are checked, but exhaustive OS/device fault injection was not performed.
* The tests do not establish universal lifetime, termination, resource-exhaustion,
  or all-input arithmetic guarantees. Stack/resource availability is an explicit
  platform assumption, not a constraint proven by this implementation.

No unresolved violation was observed within the exercised scalar subset. Future
features and platforms require their own semantics, checks, and qualification.
