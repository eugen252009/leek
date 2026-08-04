# Leek MVP Step-by-Step Plan

Each numbered item is intended to be one focused, commit-sized task. Do not
combine unrelated items. Mark an item with `[x]` only after its acceptance
criteria pass, then record the actual commit message after `✅ Commit:`.

## MVP target

The MVP is a command-line compiler that accepts a `.lk` source file, validates
its syntax and core semantic invariants, emits a directed graph and C code,
and produces a runnable native executable. It supports typed values, basic
expressions, functions, returns, lexical scopes, pure/impure boundaries, and
scope-bound arenas.

## 1. Baseline

- [x] **1.1 — Establish the language design documents.** Keep `README.md`,
  `axioms.md`, and `base-model.md` as the semantic reference. — ✅ Commit:
  `1c19a65 axioms.md erstellen`, `4a9ae6b Revise README for clarity and structure`
- [x] **1.2 — Establish the prototype compiler.** Keep the parser, resolver,
  graph model, and C backend building together. — ✅ Commit:
  `2a4e187 feat(compiler): implement core graph architecture, parser frontend, and C-codegen backend`
- [x] **1.3 — Add baseline parser and resolver tests.** — ✅ Commit:
  `d02e88f feat(compiler): add parser and resolver modules`
- [x] **1.4 — Add repository development guidance.** — ✅ Commit:
  `1d35e23 docs: add project guidance and MVP roadmap`
- [x] **1.5 — Document the currently supported prototype syntax.** Include a
  minimal valid `.lk` example and its limitations. — ✅ Commit message:
  `docs: document prototype syntax and limitations` *(implemented locally; not yet committed)*

## 2. Command-line compiler

- [x] **2.1 — Add compiler options.** Accept an input path, output binary path,
  and optional DOT path while retaining `main.lk` as a sample. — ✅ Commit message:
  `feat(cli): add source output and graph options` *(implemented locally; not yet committed)*
- [x] **2.2 — Separate compiler stages.** Expose parse, resolve, graph export,
  code generation, and native compilation as testable functions. — ✅ Commit message:
  `refactor(cli): separate compiler pipeline stages` *(implemented locally; not yet committed)*
- [x] **2.3 — Fix process exit status.** Return non-zero status for every
  compiler or GCC failure and zero only on success. — ✅ Commit message:
  `fix(cli): return failure status for compilation errors` *(implemented locally; not yet committed)*
- [x] **2.4 — Add source locations to diagnostics.** Include filename, line,
  and column in parser and semantic errors. — ✅ Commit message:
  `feat(diagnostics): add source locations to compiler errors` *(implemented locally; not yet committed)*
- [x] **2.5 — Add CLI integration coverage.** Compile a temporary source file,
  inspect generated artifacts, and execute the resulting binary. — ✅ Commit message:
  `test(cli): add source-to-binary integration test` *(implemented locally; not yet committed)*

## 3. Minimal frontend

- [ ] **3.1 — Specify the MVP grammar.** Document functions, parameters,
  blocks, `let`, `return`, literals, identifiers, and calls. — Commit:
  `docs(parser): specify MVP grammar`
- [ ] **3.2 — Replace string values with expression AST nodes.** Preserve
  source positions on AST nodes. — Commit:
  `feat(parser): add typed expression AST`
- [ ] **3.3 — Parse primitive literals and types.** Support integer, boolean,
  floating-point literals and the existing numeric types. — Commit:
  `feat(parser): parse MVP literals and types`
- [ ] **3.4 — Parse operators and parentheses.** Add precedence-aware unary and
  binary expression parsing. — Commit:
  `feat(parser): parse MVP expressions and operators`
- [ ] **3.5 — Parse returns and calls.** Support function return statements,
  function-call expressions, and argument lists. — Commit:
  `feat(parser): parse returns and function calls`
- [ ] **3.6 — Parse nested blocks.** Ensure every block creates an explicit AST
  scope and malformed blocks produce useful errors. — Commit:
  `feat(parser): support nested lexical blocks`
- [ ] **3.7 — Expand parser tests.** Cover valid syntax, precedence, malformed
  input, and filename/line/column reporting. — Commit:
  `test(parser): cover MVP grammar and diagnostics`

## 4. Semantic resolver and graph

- [ ] **4.1 — Resolve nested lexical environments.** Implement child scopes,
  shadowing rules, and scope-local bindings without implicit upward escape. —
  Commit: `feat(resolver): enforce lexical scope containment`
- [x] **4.2 — Validate declarations.** Reject duplicate bindings and invalid
  declarations with actionable diagnostics. — ✅ Commit message:
  `feat(resolver): validate declarations` *(implemented locally; not yet committed)*
- [ ] **4.3 — Resolve expressions and calls.** Reject undefined names, unknown
  functions, incorrect argument counts, and invalid call targets. — Commit:
  `feat(resolver): resolve expressions and function calls`
- [ ] **4.4 — Add MVP type checking.** Validate assignments, operators,
  arguments, returns, and function result types. — Commit:
  `feat(semantics): add MVP type checker`
- [ ] **4.5 — Represent explicit data and return edges.** Derive edges from
  reads, values, calls, and explicit outward returns. — Commit:
  `feat(graph): add data and return flow edges`
- [ ] **4.6 — Represent effect edges.** Mark impure operations and connect them
  only through explicit impure boundaries. — Commit:
  `feat(graph): add explicit effect edges`
- [ ] **4.7 — Enforce purity.** Reject IO, mutation, and other effects inside
  pure scopes. — Commit: `feat(semantics): enforce pure and impure boundaries`
- [ ] **4.8 — Enforce lifetime rules.** Reject references or values escaping the
  scope/arena that owns them. — Commit:
  `feat(semantics): validate scope-bound lifetimes`
- [ ] **4.9 — Validate graph constraints.** Check DAG ordering, illegal cycles,
  edge direction, and invalid parallel dependencies. — Commit:
  `feat(graph): validate DAG and dependency constraints`
- [ ] **4.10 — Test semantic invariants.** Add resolver/graph tests for scopes,
  bindings, types, purity, lifetimes, edges, and cycles. — Commit:
  `test(resolver): cover MVP semantic invariants`

## 5. C backend and runtime

- [ ] **5.1 — Generate typed function signatures.** Emit correct C parameter
  and return types from the resolved program. — Commit:
  `feat(codegen): generate typed function signatures`
- [ ] **5.2 — Generate typed declarations and literals.** Stop treating source
  values as unchecked C fragments. — Commit:
  `feat(codegen): generate typed declarations and literals`
- [ ] **5.3 — Generate expressions and calls.** Lower operators, parentheses,
  function calls, and returns from the AST/graph. — Commit:
  `feat(codegen): lower MVP expressions and calls`
- [ ] **5.4 — Generate control flow.** Lower the MVP block and return structure
  while preserving graph order. — Commit:
  `feat(codegen): lower MVP control flow`
- [ ] **5.5 — Make arena ownership explicit.** Allocate each scope's values in
  its arena and release the arena deterministically at scope exit. — Commit:
  `feat(runtime): implement scope-bound arena ownership`
- [ ] **5.6 — Add explicit impure runtime hooks.** Provide only approved IO or
  output operations, with no hidden effects in pure code. — Commit:
  `feat(runtime): add explicit impure boundary hooks`
- [x] **5.7 — Enable strict generated-C checks.** Compile generated C with
  warnings enabled and fail tests on warnings or compiler errors. — ✅ Commit message:
  `build(codegen): enable strict generated-C checks` *(implemented locally; not yet committed)*
- [ ] **5.8 — Add backend tests.** Add generated-C golden tests and runtime
  tests for pure, impure, return, and arena behavior. — Commit:
  `test(codegen): add generated-C and runtime coverage`

## 6. MVP release gate

- [ ] **6.1 — Add a complete example program.** Exercise typed data flow,
  returns, an impure boundary, and deterministic cleanup. — Commit:
  `docs: add complete MVP example`
- [ ] **6.2 — Add an end-to-end acceptance test.** Verify parse, resolution,
  DOT output, C output, GCC compilation, and executable behavior. — Commit:
  `test: verify complete MVP pipeline`
- [ ] **6.3 — Update user documentation.** Document installation, CLI usage,
  supported syntax, examples, limitations, and MVP status. — Commit:
  `docs: publish MVP usage and limitations`
- [ ] **6.4 — Run the release checks.** From `compiler/`, run `gofmt` on changed
  Go files, `go test ./...`, and the full compiler example. — Commit:
  `chore: validate MVP release checks`
- [ ] **6.5 — Tag the MVP.** Tag only after every preceding item is complete
  and the working tree is clean. — Commit:
  `release: Leek MVP`

## Completion rule

An item is implemented only when the code, tests, and documentation required
by that item are complete. Then commit it, change `[ ]` to `[x]`, and replace
the planned message with the actual message, for example:

```text
- [x] **2.1 — Add compiler options.** ... — ✅ Commit:
  `abc1234 feat(cli): add source output and graph options`
```
