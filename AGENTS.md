# AGENTS.md

## Project overview

Leek (`.lk`) is an experimental systems-programming language built around
scope-bound lifetimes, explicit data flow, deterministic/pure computation,
isolated effects, and compiler-derived parallelism. The project is in an
exploratory prototype phase; the written specifications are aspirational and
the compiler does not implement the complete language yet.

## Repository layout

- `README.md` — high-level language design and compiler/runtime goals.
- `axioms.md` — foundational invariants; treat these as the semantic source of
  truth when proposing language behavior.
- `base-model.md` — formal graph, node, edge, memory, and validation model.
- `compiler/` — Go prototype compiler.
  - `main.go` embeds `main.lk`, parses it, resolves semantic scopes into a
    graph, writes `graph.dot`, generates C, and invokes GCC to produce `app`.
  - `bin/parser/` contains the AST, parser, resolver, graph model, C backend,
    and Go tests.
  - `main.lk` is the current embedded compiler input/sample program.

## Development commands

Run commands from `compiler/` unless stated otherwise:

```sh
go test ./...                 # run all Go tests
gofmt -w <changed-go-files>   # format Go code
go run .                      # run the prototype compiler; requires gcc
go build .                    # build the compiler
```

The compiler currently emits `compiler/app` and rewrites `compiler/graph.dot`.
`app` is ignored by Git; treat `graph.dot` as generated output unless a graph
fixture is intentionally being updated. The C backend uses GCC flags including
`-O3` and `-march=native`.

## Implementation guidance

- Preserve the axioms: scope isolation, explicit outward data flow, purity and
  effect classification, arena/scope-bound lifetimes, and DAG constraints.
- Keep semantic behavior in the resolver/graph model rather than relying on
  frontend syntax or backend accidents. The frontend is not the semantic
  authority.
- Pure scopes must remain deterministic and free of hidden external effects.
  File, network, OS, time, randomness, raw-pointer, and similar operations must
  be represented as explicit impure boundaries.
- Do not add implicit ownership transfer, hidden aliasing, lifetime extension,
  garbage collection, or shared mutable state without first revising the
  specification and its invariants.
- Changes to parser syntax should be accompanied by parser tests. Changes to
  scope resolution, graph edges, purity, lifetime, or validation should include
  resolver/graph tests.
- Keep diagnostics actionable and avoid silently accepting malformed or
  semantically invalid programs.
- Use standard Go formatting and idiomatic error propagation. Avoid unrelated
  refactors in this small prototype.

## Testing expectations

Before submitting changes, run `go test ./...` and format modified Go files.
For compiler pipeline changes, also run `go run .` and check parsing,
resolution, generated DOT, generated C, and GCC compilation. Do not commit
compiled binaries or temporary C files.

## Documentation changes

When changing semantics, update the relevant specification documents along
with the implementation. Keep terminology consistent with the existing model:
scopes, data/control/effect edges, pure/impure scopes, arenas, and directed
acyclic scope graphs. Clearly label behavior that is proposed or incomplete.
