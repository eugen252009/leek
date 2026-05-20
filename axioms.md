Leek (.lk)

A Mechanical Axiomatic Programming Model

Draft Whitepaper — Foundational Specification (Pre-Semantic Phase)


---

Abstract

Leek (.lk) is an experimental systems programming language and semantic execution model built around a fixed axiomatic core. Unlike conventional languages that evolve feature-first and retroactively attempt to enforce safety, Leek begins from a small set of immutable mechanical invariants from which semantics, optimization behavior, memory lifetime, execution flow, and effect isolation are derived.

The central thesis of Leek is:

> Simplicity, determinism, memory safety, and parallelizability can emerge naturally from structural constraints rather than from increasingly complex runtime systems, borrow analysis, or garbage collection.



Leek intentionally rejects:

garbage collectors,

implicit shared mutable state,

hidden aliasing,

unrestricted lifetime escape,

and uncontrolled side effects.


Instead, the language defines a scope-driven execution universe in which:

lifetimes are explicit,

memory is arena-bound,

effects are isolated,

and dependency graphs emerge mechanically from structure itself.


This document describes the foundational axioms of the system and the philosophical constraints under which the language may evolve.


---

1. Design Goals

Leek attempts to combine:

the mechanical simplicity of C Programming Language,

the memory safety goals of Rust,

and the deterministic reasoning strengths commonly associated with Haskell,


without inheriting the full complexity footprint of:

garbage-collected runtimes,

ownership-heavy borrow systems,

or unrestricted heap semantics.


The language prioritizes:

deterministic execution,

structural memory safety,

compiler visibility,

explicit lifetime topology,

and naturally derivable optimization graphs.



---

2. Philosophical Position

Leek is not designed around “maximum feature inclusion”.

Instead, the language treats the axioms as hard physical laws.

If a language feature cannot exist without violating the axioms, then the feature is rejected rather than the axioms being weakened.

The project therefore follows this hierarchy:

Axioms
  ↓
Semantics
  ↓
Intermediate Representation
  ↓
Frontend Syntax
  ↓
Tooling / Ecosystem

The frontend is not the source of truth.

The axioms are.


---

3. Core Principle

The system assumes:

> Hidden complexity is the root cause of unsafe and unoptimizable software systems.



Leek therefore attempts to make:

lifetimes visible,

effects visible,

dependencies visible,

ownership visible,

and execution ordering visible.


The compiler should not “guess” program structure through expensive heuristics if the structure can instead be mechanically derived from the language itself.


---

4. The Mechanical Axioms

A1 — Scope Containment (Lexical Isolation)

Every entity:

variable,

value,

allocation,

reference,

or memory region


exists strictly within a uniquely defined lexical scope.

Entities are only visible:

inside their defining scope,

or inside nested child scopes.


No implicit upward visibility exists.


---

A2 — Single Escape Path (Return Exclusivity)

The only legal mechanism for transferring data from a child scope into a parent scope is an explicit return operation.

Implicit lifetime escape is forbidden.

References may not outlive:

their defining scope,

nor the arena that owns them.


Global mutation channels and hidden side effects are excluded from the pure execution model.


---

A3 — Purity and Effect Classification

Every operation belongs to exactly one category:

Pure

Deterministic transformations:

identical input always produces identical output,

no externally visible mutation occurs,

no impure state may be observed.


Impure

Operations interacting with:

hardware,

external state,

system clocks,

interrupts,

non-deterministic sources,

foreign interfaces,

or mutable external memory.


Impure behavior is explicitly isolated.


---

A4 — Controlled Effect Boundaries

External interaction is directional.

Inbound Effects

Any operation that:

reads mutable external state,

mutates global state,

or introduces non-determinism


must occur inside impure scopes.

Outbound Effects

Pure scopes may emit observational effects:

logging,

printing,

tracing,


provided such effects:

do not mutate computational state,

and do not alter determinism.



---

A5 — Arena-Based Lifetime Model

Memory allocation is structurally bound to scopes.

Each scope owns:

a dedicated arena,

or a tightly defined arena cluster.


When the owning scope terminates:

the entire arena terminates atomically.


Partial runtime deallocation is not part of the core model.

The language intentionally avoids:

general-purpose heap ownership,

ad-hoc free(),

and unrestricted allocation lifetimes.



---

A6 — Referential Closure

Every value maintains a compiler-trackable reference set:

R(v)

representing all valid references to value v.

Upon scope termination:

R(v) = ∅

No hidden aliasing paths may exist outside compiler visibility.

This guarantees:

analyzable ownership,

deterministic memory topology,

and mechanically derivable dependency graphs.



---

A7 — Dependency Graph Derivation

Program dependency graphs are not manually constructed abstractions.

They emerge directly from:

scope nesting,

read/write relationships,

return flows,

and effect boundaries.


This enables:

deterministic scheduling,

automatic parallelization,

SIMD eligibility analysis,

and optimization visibility


without requiring opaque runtime systems.


---

5. Derived Extension Principle

The axioms are immutable foundational invariants.

However:

> Additional axioms, semantic rules, or compiler guarantees may be introduced only if they are strictly derivable from the existing axiomatic system.



This means:

extensions may refine the model,

but may never contradict foundational invariants.


The system therefore evolves through:

derivation,

formalization,

and structural consequence,


not through arbitrary exception insertion.


---

6. Scope-Oriented Execution Model

Leek treats scopes as:

memory regions,

effect regions,

dependency regions,

and optimization regions simultaneously.


A scope is not merely syntactic grouping.

It is:

a lifetime boundary,

a scheduling boundary,

and a semantic isolation boundary.



---

7. Determinism as Optimization Infrastructure

Determinism is not treated merely as a correctness feature.

It is treated as optimization infrastructure.

If the compiler can prove:

no hidden mutation,

no alias ambiguity,

and no uncontrolled side effects,


then:

aggressive inlining,

vectorization,

parallel execution,

speculative scheduling,

and memory layout optimization


become mechanically derivable.


---

8. Rejection of Hidden Runtime Complexity

Leek intentionally avoids:

tracing garbage collection,

hidden ownership mutation,

implicit lifetime extension,

and runtime borrow verification.


Instead, the language attempts to encode:

memory topology,

execution order,

and effect relationships


directly into the program structure itself.


---

9. External Interaction Model

The language does not reject:

interrupts,

hardware interaction,

networking,

DMA,

or asynchronous input.


Instead, these are isolated into explicit impure boundaries.

The intended execution flow is:

Impure Input
    ↓
Stabilized Snapshot
    ↓
Pure Deterministic Computation
    ↓
Impure Output

Pure computation therefore operates exclusively on stabilized state snapshots.


---

10. Frontend Independence

Leek syntax is not considered sacred.

The frontend exists only as:

a semantic expression layer,

not as the semantic authority itself.


Alternative frontends may exist provided they preserve:

the axioms,

the semantic guarantees,

and the derived invariants.



---

11. Current Project State

The project is currently in:

exploratory prototype phase,

semantic experimentation phase,

and implementation feasibility validation.


The current compiler:

is intentionally incomplete,

acts primarily as a proof-of-concept,

and currently uses C as a temporary transport layer.


The existing implementation should not be interpreted as the final architecture.


---

12. Final Position

Leek is not attempting to become:

a universal abstraction layer,

a feature-maximal language,

or a compatibility-first runtime ecosystem.


Its purpose is instead to explore whether:

deterministic structure,

explicit lifetime topology,

and mechanical semantic constraints


can produce a language that is simultaneously:

simple,

analyzable,

memory-safe,

and highly optimizable.


If those goals cannot be achieved under the axioms, then the project considers the experiment invalid rather than weakening the axiomatic foundation.