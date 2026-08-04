# Leek MVP Roadmap

This checklist takes the current prototype to a small, usable MVP. A task is
only complete when its tests and acceptance criteria pass. After committing a
completed task, replace `[ ]` with `[x]` and record the actual commit message
on the same line as `✅ Commit:`.

## MVP definition

The MVP should be able to compile a small `.lk` program from a file into a
native executable while enforcing the core rules that are currently testable:

- lexical scopes and explicit bindings;
- typed values and basic expressions;
- explicit pure/impure function boundaries;
- undefined-symbol and type diagnostics;
- a validated acyclic data/control/effect graph;
- deterministic scope-bound arena cleanup;
- generated C and a runnable native binary;
- parser, resolver, code-generation, and end-to-end tests.

## Phase 0 — Baseline and project hygiene

- [x] Establish the written design and axioms in `README.md`, `axioms.md`,
  and `base-model.md`. — ✅ Commit: `1c19a65 axioms.md erstellen`, `4a9ae6b Revise README for clarity and structure`
- [x] Create the initial parser, resolver, graph model, and C backend. — ✅ Commit: `2a4e187 feat(compiler): implement core graph architecture, parser frontend, and C-codegen backend`
- [x] Add initial parser and resolver tests. — ✅ Commit: `d02e88f feat(compiler): add parser and resolver modules`
- [x] Add repository development guidance. — ✅ Commit: record the commit that adds `AGENTS.md`
- [ ] Run the baseline test suite and document the current supported `.lk`
  syntax in the README. — Commit when complete: `docs: document MVP baseline and supported syntax`

## Phase 1 — Make the compiler a real CLI

- [ ] Accept an input `.lk` path instead of always embedding `main.lk`.
  Preserve `main.lk` as a sample program. — Commit when complete:
  `feat(cli): compile source files from the command line`
- [ ] Add explicit output options for the native binary and graph/DOT file.
  Do not write generated files unexpectedly into the repository root. — Commit
  when complete: `feat(cli): add output and graph options`
- [ ] Return non-zero exit codes for parse, semantic, code-generation, and GCC
  failures; print source locations in diagnostics. — Commit when complete:
  `fix(cli): report compiler failures with actionable diagnostics`
- [ ] Add an end-to-end test that invokes the compiler on a temporary source
  file and verifies the produced binary. — Commit when complete:
  `test(compiler): add source-to-binary integration coverage`

## Phase 2 — Complete the minimal frontend

- [ ] Define and document the MVP grammar for functions, parameters, blocks,
  `let`, `return`, literals, identifiers, and calls. — Commit when complete:
  `docs(parser): specify the MVP grammar`
- [ ] Replace stringly-typed values with expression AST nodes. — Commit when
  complete: `feat(parser): add typed expression AST`
- [ ] Parse integer, boolean, and floating-point literals with operators and
  parentheses. — Commit when complete: `feat(parser): parse MVP expressions`
- [ ] Parse return statements, function calls, and nested blocks. — Commit when
  complete: `feat(parser): support returns calls and nested blocks`
- [ ] Add malformed-input tests for every grammar rule and ensure errors include
  filename and line/column information. — Commit when complete:
  `test(parser): cover MVP syntax errors`

## Phase 3 — Enforce semantic invariants

- [ ] Implement lexical scope creation and lookup for nested blocks without
  implicit upward escape. — Commit when complete:
  `feat(resolver): enforce lexical scope containment`
- [ ] Detect duplicate bindings, undefined names, invalid returns, and invalid
  function calls with actionable errors. — Commit when complete:
  `feat(resolver): validate bindings returns and calls`
- [ ] Add a type checker for the MVP types and operators. — Commit when
  complete: `feat(semantics): add MVP type checking`
- [ ] Track pure and impure operations in the graph and reject effects from
  pure scopes. — Commit when complete: `feat(semantics): enforce purity boundaries`
- [ ] Represent explicit return/data/effect edges and validate their direction.
  Reject graph cycles with a diagnostic pointing to the involved nodes. — Commit
  when complete: `feat(graph): validate data control and effect flow`
- [ ] Add resolver and graph tests for scope isolation, purity, lifetime escape,
  edge creation, and cycle rejection. — Commit when complete:
  `test(resolver): cover core semantic invariants`

## Phase 4 — Make generated C correct and safe for the MVP

- [ ] Generate correct C function signatures and return values from the typed
  AST/graph. — Commit when complete: `feat(codegen): generate typed functions and returns`
- [ ] Generate expression evaluation, calls, and control flow without using
  raw source strings as C expressions. — Commit when complete:
  `feat(codegen): lower MVP expressions and calls`
- [ ] Make arena ownership explicit per scope and reject references that would
  outlive their arena. — Commit when complete:
  `feat(codegen): enforce scope-bound arena lifetimes`
- [ ] Add explicit impure runtime hooks for approved IO/output operations;
  keep the pure backend free of hidden external effects. — Commit when complete:
  `feat(runtime): add explicit impure boundary hooks`
- [ ] Compile generated C with strict warnings and treat warnings as failures
  in tests. — Commit when complete: `build(codegen): enable strict generated-C checks`
- [ ] Add golden tests for generated C and runtime tests for representative
  pure and impure programs. — Commit when complete:
  `test(codegen): add generated-C and runtime coverage`

## Phase 5 — MVP release gate

- [ ] Add a documented example program that exercises parsing, data flow,
  return, an impure boundary, and deterministic cleanup. — Commit when complete:
  `docs: add complete MVP example`
- [ ] Run `gofmt` and `go test ./...` from `compiler/`; all tests pass. — Commit
  when complete: `chore: validate MVP test suite`
- [ ] Run `go run .` (or the new CLI), verify DOT output, generated C, GCC
  compilation, and execution of the example. — Commit when complete:
  `test: verify end-to-end MVP pipeline`
- [ ] Update `README.md` with installation, CLI usage, supported syntax,
  limitations, and the exact MVP status. — Commit when complete:
  `docs: publish Leek MVP usage and limitations`
- [ ] Tag the first MVP release only after the full checklist is complete. —
  Commit when complete: `release: Leek MVP`

## Working rule for future steps

Keep each implementation step small enough to produce one focused commit.
Never mark a task complete because code exists; mark it complete only after its
acceptance test passes and the implementation commit has been recorded above.
