# Mechanical C (mC)

## A Deterministic Scope-Based Programming Model for Visible Dataflow Execution

### Draft Whitepaper — Revision 0.1

---

# Abstract

Modern software systems increasingly suffer from hidden execution paths, implicit memory behavior, opaque concurrency, and unpredictable runtime costs. Mechanical C (mC) proposes a fundamentally different approach:

* execution is scope-driven,
* dataflow is explicit,
* memory lifetime is deterministic,
* parallelism emerges from structural independence,
* abstractions never change mechanical behavior.

mC treats programs not primarily as text, but as visible execution graphs composed of deterministic transformation scopes. The language is designed around a minimal set of mechanical rules that remain globally consistent across the entire system.

The primary goal of mC is not abstraction maximization, but structural clarity, predictable execution, analyzable parallelism, and compiler-assisted scheduling.

---

# 1. Philosophy

## 1.1 Core Principle

mC is based on a simple premise:

> If the execution path is visible, the system becomes understandable.

Most modern programming languages optimize for:

* syntactic flexibility,
* abstraction density,
* runtime dynamism.

mC instead optimizes for:

* visible data movement,
* explicit lifetime boundaries,
* deterministic transformations,
* structural analyzability.

---

# 1.2 Mechanical Thinking

mC models programs similarly to:

* production lines,
* pipeline systems,
* scheduling graphs,
* industrial process flows.

A program is composed of:

* stations,
* transitions,
* transformations,
* resource boundaries.

Not invisible runtime magic.

---

# 1.3 Simplicity Through Constraints

mC intentionally restricts semantics to a small number of globally consistent rules.

The language attempts to solve entire classes of problems structurally:

* use-after-free,
* hidden allocations,
* invisible ownership transfer,
* accidental shared mutation,
* implicit synchronization complexity.

---

# 2. Core Rules

---

# Rule 1 — Scope Is Law

Every `{}` block defines:

* a lifetime boundary,
* an allocation region,
* an ownership boundary.

Objects created inside a scope are destroyed automatically when the scope ends.

```mc
{
    let buffer = alloc(1024)
} // buffer destroyed here
```

No garbage collector exists.

Memory release is deterministic and compiler-generated.

---

# Rule 2 — Data Cannot Escape Implicitly

Data may only leave a scope through:

* explicit `copy`,
* explicit reference passing,
* deterministic `return`.

Copies must always be visible in syntax.

```mc
copy output <- local_buffer
```

Assignment is not ownership transfer.

The goal is to make memory movement visible to the developer.

---

# Rule 3 — Pure By Default

Functions are deterministic unless explicitly marked otherwise.

```mc
fn process(data) -> result
```

A pure function:

* cannot perform IO,
* cannot access hidden global state,
* cannot use time/network/randomness,
* cannot mutate external state.

This allows:

* deterministic analysis,
* safe scheduling,
* automatic parallelization,
* aggressive compiler optimization.

---

# Rule 4 — Impure Isolated Boundaries

External uncertainty must be isolated explicitly.

```mc
impure fn read_file(path) -> buffer
```

Impure scopes include:

* file IO,
* networking,
* timers,
* raw pointers,
* OS interaction.

Impure boundaries act as:

* quarantine zones,
* scheduling barriers,
* external dependency markers.

Errors originating inside impure scopes must be resolved or propagated explicitly.

---

# Rule 5 — Structure Defines Parallelism

Parallelism is not created through thread commands.

Instead:

> independent scopes are naturally parallelizable.

The compiler analyzes:

* dependencies,
* mutations,
* ownership,
* scope isolation.

A `parallel` annotation acts only as:

* a structural contract,
* a linter verification zone,
* a parallelizability guarantee request.

It does not directly spawn threads.

---

# 3. The Scope Graph Model

---

# 3.1 Programs As Graphs

mC internally represents programs as:

> Directed Acyclic Scope Graphs (DASG)

Nodes:

* scopes,
* transformations,
* reductions,
* impure boundaries.

Edges:

* data dependencies,
* ownership transitions,
* state outputs.

---

# 3.2 State Transition Model

Every scope represents:

```text
Input State
 → Transformation
 → Output State
```

A scope may only influence its explicit output.

No hidden side effects are allowed in pure scopes.

---

# 3.3 Structural Visibility

mC attempts to make:

* memory movement,
* synchronization,
* parallelism,
* execution order,
* resource ownership

visible in the graph itself.

---

# 4. Compiler Architecture

---

# 4.1 Frontend

The compiler extracts:

* scopes,
* ownership boundaries,
* purity information,
* data dependencies.

Result:

* Scope Graph IR.

---

# 4.2 Dependency Analysis

The compiler computes:

* critical paths,
* independent subgraphs,
* synchronization points,
* lifetime regions,
* allocation reuse opportunities.

---

# 4.3 Scheduling

The compiler transforms the graph into executable scheduling units.

Pure independent scopes may execute concurrently.

Small scopes may be:

* fused,
* inlined,
* batched.

---

# 4.4 Runtime Model

The runtime acts as:

* DAG executor,
* worker scheduler,
* region allocator.

Workers process:

* isolated transformation scopes,
* ready dependency nodes,
* deterministic execution batches.

---

# 5. Memory Model

---

# 5.1 Region-Based Allocation

Scopes allocate memory regions.

When the scope ends:

* the entire region is released in bulk.

Advantages:

* low fragmentation,
* predictable deallocation,
* cache-friendly locality,
* reduced allocator overhead.

---

# 5.2 Buffer Reuse

mC encourages:

* passing buffers downward,
* transforming in-place,
* avoiding repeated allocations.

The compiler and linter may warn about:

* repeated allocation inside loops,
* unnecessary copies,
* avoidable heap churn.

---

# 6. Parallelism Model

---

# 6.1 Parallelism As Emergent Property

Parallelism is discovered, not manually orchestrated.

If two scopes:

* share no mutable state,
* have no dependency edge,
* are pure,

they may execute concurrently.

---

# 6.2 Two Forms Of Parallelism

## Pipeline Parallelism

```text
A → B → C
```

Different workers process different stages simultaneously.

---

## Data Parallelism

```text
process(chunk1)
process(chunk2)
process(chunk3)
```

Identical transformations operate on independent datasets.

---

# 6.3 Deterministic Scheduling

The meaning of a program never depends on:

* thread count,
* scheduling order,
* runtime timing.

Parallelism changes execution strategy, not semantics.

---

# 7. Algorithms In mC

Algorithms are not special language constructs.

An algorithm is simply:

* a deterministic transformation scope.

Examples:

* sorting,
* graph traversal,
* parsing,
* simulation.

All follow the same rule:

```text
Input State
 → Pure Transformation
 → Output State
```

---

# 8. OOP And Abstractions

mC does not forbid OOP.

However:

* objects remain scope-bound,
* classes cannot bypass ownership rules,
* abstraction cannot alter mechanical semantics.

OOP becomes:

* organizational syntax,
* not runtime reality.

Abstractions may improve readability, but never change mechanical behavior.

---

# 9. Exportable Execution Graphs

mC may expose:

* execution graphs,
* dependency trees,
* critical paths,
* scheduling plans.

Example:

```bash
mc build --export-path graph.pdf
```

This allows:

* debugging,
* optimization,
* educational visualization,
* AI-assisted graph refinement.

---

# 10. AI-Assisted Development

mC is structurally compatible with AI-assisted decomposition.

Because the system is graph-oriented:

* problems can be refined incrementally,
* scopes can be generated hierarchically,
* transformations can be synthesized locally.

AI acts as:

* graph refinement engine,
* structural decomposer,
* deterministic transformation generator.

Not as an unconstrained code author.

---

# 11. Design Goals

mC prioritizes:

| Goal                        | Priority  |
| --------------------------- | --------- |
| Visible execution           | High      |
| Deterministic memory        | High      |
| Structural parallelism      | High      |
| Predictable performance     | High      |
| Minimal semantic rules      | High      |
| Hidden runtime behavior     | Forbidden |
| Implicit ownership transfer | Forbidden |

---

# 12. Final Principle

Mechanical C is built around a single foundational idea:

> Programs should behave like visible mechanical systems, not hidden magical processes.

Or more simply:

> Syntax may vary. Mechanics may not.
