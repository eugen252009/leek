Mechanical C (mC)

> Visible execution. Deterministic memory. Structural parallelism.



Mechanical C (mC) is an experimental systems programming language focused on:

explicit dataflow,

deterministic scope-based memory,

visible execution paths,

compiler-driven scheduling,

structural simplicity.


mC treats programs as deterministic transformation graphs instead of opaque runtime processes.

The goal is not to hide complexity behind abstractions, but to make execution understandable, analyzable, and optimizable.


---

Philosophy

Modern languages often hide:

ownership,

memory movement,

synchronization,

execution order,

dependency chains.


mC attempts to make those mechanics visible again.

Programs are modeled similarly to:

assembly lines,

processing pipelines,

scheduling graphs,

deterministic state transitions.


The language is intentionally built around a very small set of globally consistent rules.


---

Core Principles

1. Scope Is Law

Every {} block defines:

a lifetime boundary,

an ownership region,

an allocation scope.


Objects created inside a scope are destroyed automatically when the scope ends.

{
    let buffer = alloc(1024)
} // buffer destroyed here

There is:

no garbage collector,

no hidden lifetime extension,

no invisible ownership transfer.



---

2. Data Cannot Escape Implicitly

Data may only leave a scope through:

explicit copy,

explicit references,

deterministic return.


Copies must always be visible.

copy output <- local_buffer

The goal is to make memory movement mechanically understandable.


---

3. Pure By Default

Functions are deterministic unless explicitly marked otherwise.

fn parse(data) -> result

Pure functions:

cannot perform IO,

cannot access hidden global state,

cannot use timers/network/randomness,

cannot mutate external state.


This enables:

deterministic execution,

aggressive compiler optimization,

automatic dependency analysis,

structural parallelization.



---

4. Impure Is Explicit

External uncertainty must be isolated explicitly.

impure fn read_file(path) -> Buffer

Impure functions include:

file IO,

networking,

OS interaction,

timers,

unsafe/raw memory access.


Impure boundaries are visible execution boundaries.


---

5. Parallelism Emerges From Structure

mC does not treat parallelism as manual thread orchestration.

Instead:

pure scopes,

independent dependencies,

isolated state,


become naturally parallelizable.

The compiler analyzes the execution graph and schedules work automatically.

Optional parallel annotations are treated as:

validation contracts,

linter verification zones,

structural hints.


They do not directly spawn threads.


---

The Scope Graph Model

Internally, mC programs are represented as:

> Directed Acyclic Scope Graphs (DASG)



Each scope represents:

Input State
 → Transformation
 → Output State

The compiler tracks:

ownership,

dependencies,

purity,

memory regions,

scheduling opportunities.



---

Compiler Design Goals

The compiler is responsible for the complexity — not the language.

mC aims to keep syntax and mental models simple while allowing the compiler to:

fuse scopes,

inline transformations,

reuse buffers,

schedule independent work,

optimize cache locality,

compute critical paths,

parallelize pure subgraphs.



---

Memory Model

mC uses deterministic scope-based lifetime management.

Typical allocation model:

Scope Start  → Allocate Region
Scope End    → Release Region

This enables:

predictable memory behavior,

low fragmentation,

cheap cleanup,

strong cache locality.


The language encourages:

buffer reuse,

in-place transformation,

region-oriented thinking.



---

Scheduling Model

The compiler transforms programs into execution graphs.

Independent pure scopes may execute:

concurrently,

out-of-order,

fused into larger execution units.


This is conceptually similar to:

CPU out-of-order execution,

job scheduling,

pipeline processing,

ECS systems,

industrial flow systems.


The runtime acts as:

a DAG executor,

a worker scheduler,

a dependency resolver.



---

Visibility Over Magic

One of the primary goals of mC is:

> The execution path should never become invisible.



Possible future tooling:

live dependency visualization,

execution graph export,

critical path analysis,

inline flow tracing,

purity visualization,

buffer lifetime overlays.


Example:

impure read_file
        ↓
pure parse_json
        ↓
pure extract_ids
        ↓
pure aggregate


---

Structural Tooling Vision

The compiler/linter may eventually provide:

dependency overlays,

flow tracing,

parallelization hints,

allocation warnings,

scope visualizations.


Example:

depends on:
  parse_json()
    depends on:
      read_file()

The goal is to make:

dataflow,

ownership,

dependencies,

purity,

execution structure


visible directly inside development tools.


---

OOP Philosophy

mC does not reject OOP.

However:

classes remain scope-bound,

ownership rules still apply,

abstraction cannot bypass mechanical constraints.


OOP becomes:

organizational syntax,

not hidden runtime behavior.



---

Design Goals

Goal	Priority

Visible execution	High
Deterministic memory	High
Structural simplicity	High
Predictable performance	High
Automatic scheduling	High
Hidden runtime behavior	Forbidden
Implicit ownership transfer	Forbidden
Invisible synchronization	Forbidden



---

Long-Term Vision

mC explores the idea that programs can be:

mechanically understandable,

structurally analyzable,

naturally parallelizable,

visually explainable.


The language is intentionally experimental and currently focused on:

core semantics,

compiler architecture,

execution modeling,

dependency visibility,

deterministic scheduling.



---

Final Principle

> Syntax may evolve. Mechanics must remain visible.