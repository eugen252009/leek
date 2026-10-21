# Automatic constraint analysis MVP — qualification

Historical evidence for the original advisory milestone. Current control-flow
and backend changes are qualified separately in
[path-sensitive constraints](path-sensitive-constraints.md) and
[proof-gated optimization](proof-gated-optimization.md). The backend fingerprint
below identifies that earlier artifact, not the current backend.

## Scope and assessment

**PASS for the exercised advisory integer-analysis subset and runtime-preservation
checks.** Evidence is empirical, including exhaustive finite-domain tests; this
is not a formal proof for all programs, a general solver qualification, or a
release approval.

Artifact: new `compiler/bin/constraints/`, the optional CLI inspection branch,
new tests/example/docs, on the existing working tree based on `e726089`. The
preceding uncommitted pure-composition work was preserved. No parser, resolver,
checked-arithmetic backend, or native compilation flag changes were made by this
milestone.

Environment: Linux amd64, Go 1.26.4, GCC 14.2.0. No new dependency or SMT solver
was installed. The backend source fingerprint during qualification was:

```text
aa117c22a815d8ec2d589ca188d8155da0baf091d411bfc3b8e5857c96f1fe40  compiler/bin/parser/codegen.go
```

Authority: the approved analysis-only milestone and
[the implicit-constraint specification](../docs/constraint-analysis.md), preserving
`axioms.md` and the execution contracts in `pure-composition.md`.

## Contracts and evidence

| Contract | Required property | Actual checks | Qualification |
|---|---|---|---|
| CA-TYPE | Integer input bounds are exact known facts, not assumptions introduced by the operation being checked | Bounds for all eight integer types, explicit u32 `[0,4294967295]`, u64 extrema | TESTED |
| CA-ARITH | Conditions characterize checked `+ - * / %` and negation, including unsigned underflow and signed minimum divided/reduced modulo `-1` | All 65,536 operand pairs for each of five operations for both u8 and i8: **655,360 cases**; signed/u64 constant boundaries and large mathematical products | TESTED, exhaustive for those finite domains |
| CA-COMPOSE | All intermediate obligations propagate through direct pure calls and `->`; unused failing work is retained | Expected `x > 1`, longer `x > 2`, additional parameters, bindings, unused declarations, checked cancellation; bounded composition checks across all u8 inputs and `(a+b)*2` across all u8 input pairs | TESTED |
| CA-FACTS | Conditional output facts never prove their own prerequisites; failing executions do not invent outputs | `add_five` range `[5,MAX]` with UNKNOWN precondition, empty ranges for impossible completion, constant caller success/failure, false Boolean result versus execution failure | TESTED |
| CA-PROOF | PROVEN/VIOLATED are supported conclusions; failed reasoning stays UNKNOWN; inconsistent incoming assumptions do not permit vacuity | 77,616 small-domain affine rewrite valuations; interval-proof checks over small signed domains; explicit contradictions, nonlinear unknowns, effectful/float unknowns, term/obligation limit tests | TESTED |
| CA-INSPECT | Reports are structured, deterministic, and honest about completeness and effects | Repeat JSON comparison, source/call provenance, guarded obligations, distinct effectful call identities, CLI JSON decoding with no GCC in PATH and no generated artifacts | TESTED |
| CA-PRESERVE | Advisory analysis never changes source acceptance, generated C/DOT, runtime checks, results, effects, or failure order | Byte-for-byte C/DOT comparison before/after API analysis; native executions before/after CLI inspection; complete prior suite | TESTED |

### Specific counterexamples exercised

* `calculate(0)` must retain subtraction underflow; excluding only `1` is unsound.
* `calculate(1)` must retain division-by-zero failure; `calculate(2)` succeeds.
* `(x+1)-(x+1)` has result relation zero but still fails for maximal unsigned x.
* Signed minimum `/ -1` **and** `% -1` fail under Leek's rules.
* `x == 5 AND x > 10` is an impossible requirement; treating it as incoming
  assumptions must not produce a vacuous successful proof.
* An effectful call returning 7 today is not inlined as a pure constant source;
  two effectful calls have distinct identities.
* `false && fail()` and `true || fail()` do not require the skipped arithmetic
  to succeed. The failure conditions of evaluating a guard remain independent.
* A `VIOLATED` report is not a compilation error. All existing runtime checks
  and acceptance behavior remain intact.

Test locations:

* `compiler/bin/constraints/analyze_test.go`
* `compiler/bin/constraints/proof_test.go`
* `compiler/constraint_analysis_test.go`
* Existing parser/semantic/purity/native tests under `compiler/bin/parser/`
  and `compiler/*_test.go`

## Commands executed

From `compiler/`, with `/usr/local/go/bin` on PATH for `gofmt`:

```sh
go test -count=1 ./...
go vet ./...
go test -race ./...
go test ./bin/constraints -cover
go test ./bin/constraints -run '^$' -fuzz '^FuzzAffineRewriteSoundness$' -fuzztime=5s -parallel=2
go test ./bin/constraints -run '^$' -fuzz '^FuzzAnalysisReadOnly$' -fuzztime=5s -parallel=2
go test ./bin/parser -run '^$' -fuzz '^FuzzParserAndResolver$' -fuzztime=5s -parallel=2
go run . -constraints examples/constraints.lk
go run . -o <temporary>/sample -graph <temporary>/sample.dot
go run . -o <temporary>/example -graph <temporary>/example.dot examples/constraints.lk
go build -o <temporary>/leek .
```

Results:

* All suites, vet, race checks, and build passed. Modified Go files were formatted;
  `git diff --check` passed.
* Final bounded affine fuzz run: **264,620 executions**, no reported failures.
* Final bounded read-only analyzer fuzz run: **94,001 executions**, no reported
  failures. The property includes unchanged generated C and valid symbolic DAG
  references for successfully parsed/resolved inputs.
* Parser/resolver fuzz regression: **167,464 executions**, no reported failures.
  These runs are bounded smoke tests, not exhaustive malformed-input coverage.
* The original sample compiled/executed. The constraint example compiled and
  printed `12` despite also declaring an unused function with a `VIOLATED`
  precondition. Inspection reported `calculate: (x > 1), UNKNOWN`,
  `safe: true, PROVEN`, and `failing: false, VIOLATED`.
* Runtime comparisons covered successful composition, unsigned underflow,
  division by zero, an effectful source preceding failure, and a skipped failing
  branch. Stdout, stderr (including failure locations), and exit status matched
  before/after inspection.
* Native artifacts and JSON inspection output were written to temporary paths;
  the tracked sample DOT file was not rewritten.

Coverage was measured as a development aid, not used as a soundness argument.
The stronger evidence is the independent arithmetic oracle, finite-domain
comparisons, preservation assertions, and targeted mutation failures below.

## Isolated mutation checks

Each mutation was applied to its own disposable compiler copy, not to the user's
working source. Target tests ran against a passing baseline. Copies were restored
from the baseline source, byte-compared, and removed after inspection of failures.

| Injected defect | Targeted test | Observed assertion failure | Outcome |
|---|---|---|---|
| Drop unsigned subtraction requirement | `TestCompositionPreconditions` | Derived `x != 1` instead of `x > 1`; `calculate(0)` incorrectly PROVEN | KILLED |
| Drop imported pure-callee obligations | `TestCompositionPreconditions` | Compositions reported `true/PROVEN`, with missing intermediate obligations | KILLED |
| Assume each required condition when proving itself | `TestCompositionPreconditions` | Required `x >= 1` incorrectly marked PROVEN for arbitrary u32 input | KILLED |
| Drop signed min/-1 division/remainder exception | `TestArithmeticPreconditionsExhaustive8Bit/i8` | Derived precondition accepted `(-128,-1)` for `/` and `%`, contrary to the independent execution oracle | KILLED |

All four failures were meaningful contract assertion failures, not build failures.
They demonstrate sensitivity to these defects, not a general mutation score or
completeness proof.

## Limits and qualification boundary

* Full-program soundness has not been formally proven. Exhaustive evidence is
  scoped to the finite domains enumerated above; wider integer domains and all
  possible compositions are not exhaustively enumerated.
* Supported symbolic conditions may be exact while their proof status remains
  UNKNOWN. Nonlinear relations are retained rather than solved generally.
* Floating-point reasoning and effectful-call success remain opaque. Auxiliary
  type facts for returned values are explicitly conditional and do not establish
  that those calls return.
* Short-circuit conditions are retained; general path-sensitive/control-flow
  analysis and arbitrary incoming assertions are not implemented.
* Resource limits produce partial/unknown results. They are not hard time or
  memory isolation against arbitrary adversarial source files.
* No backend check removal, new runtime guards, speculation, scheduling,
  optimization, collections, annotations, SMT dependency, recursion, or general
  interprocedural fixed-point analysis was added.
* Previous execution qualification is supplemented by rerunning its tests, not
  promoted to a stronger proof. Runtime/OS resource assumptions remain unchanged.

No unresolved counterexample was observed in the qualified subset. Nothing was
pushed, published, deployed, or released.
