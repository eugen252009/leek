## 📐 The Mechanical Axiomatic System for Leek (.lk)

### A1 — Scope-Containment (Lexical Isolation)
Every entity (variable, memory region, value, or binding) exists strictly within a uniquely defined, hierarchically nested scope.

- No entity is directly visible or accessible outside of its declaring scope.
- Access is restricted exclusively to the current scope and its nested descendants.
- Scope boundaries form the fundamental structural unit of execution, memory ownership, and dependency derivation.

---

### A2 — Single Escape Path (The Return Axiom)
The only permitted mechanism for transferring data from a child scope into its parent scope is an explicit return path.

- Implicit upward escapes do not exist.
- References, pointers, or handles may never outlive their originating scope.
- Hidden mutation channels, global state leakage, or undeclared side-paths are structurally forbidden.

This guarantees that all upward data movement remains explicit, finite, and compiler-verifiable.

---

### A3 — Deterministic Purity & Passive Export
Operations are mechanically divided into two execution classes:

#### Pure (Deterministic)
Pure operations are fully deterministic transformations:

Input → Output

- Identical input always produces identical output.
- No external mutable state may be read or modified.
- No hidden mutation or uncontrolled aliasing may occur.
- Internal execution remains fully analyzable and causally closed.

#### Passive Outbound Exception
Pure scopes may emit already-determined data into passive outbound sinks (for example: debug output, logging, or printing), provided that:

- the outbound operation cannot mutate or influence program computation,
- no external state is observed during emission,
- and the operation introduces no inbound causal dependency back into the execution graph.

Outbound observation is therefore treated as non-participatory in computational causality.

#### Impure (Stateful)
Impure execution includes any operation involving:

- external mutable state,
- nondeterministic behavior,
- hardware interaction,
- FFI boundaries,
- timing,
- interrupts,
- synchronization primitives,
- or externally observable mutation.

Impurity propagates transitively across dependent execution paths.

---

### A4 — Controlled Effect Boundary (Context Isolation)
Interactions with the external environment are separated by directional causality.

#### Inbound Effects (Mutating / Observational)
Operations that:
- observe mutable external state,
- alter external systems,
- disrupt deterministic execution flow,
- or inject nondeterministic information

are restricted exclusively to impure scopes.

Pure execution may never depend on inbound impure state.

#### Outbound Effects (Passive)
Pure scopes may export finalized data outward through passive sinks.

Because outbound data flow cannot influence internal computation, the dependency graph remains causally clean and mechanically optimizable.

---

### A5 — Arena-Based Lifetime Model
Memory ownership is structurally bound to lexical scope topology.

- Every allocation belongs to exactly one arena.
- Every arena belongs to exactly one owning scope or tightly defined scope cluster.
- Upon scope termination, the entire arena is deallocated atomically.
- Partial runtime deallocation does not exist.

This removes fragmented ownership patterns and eliminates lifetime ambiguity.

---

### A6 — Referential Closure (Controlled Aliasing)
Every value possesses a fully bounded and compiler-verifiable reference set R(v).

- R(v) defines the complete set of all valid references to value v.
- No hidden or untracked aliasing paths may exist.
- Upon termination of the owning scope:

R(v) = ∅

This guarantees that reference topology remains mechanically finite, structurally analyzable, and safe for aggressive compiler optimization.

---

### A7 — Dependency Derivation (The Graph Axiom)
The program dependency graph is not reconstructed heuristically after parsing; it emerges directly from the axiomatic structure itself.

Graph edges arise exclusively through:

- read-before-write dependencies,
- write causalities,
- scope nesting,
- explicit return flow,
- and controlled effect boundaries.

Because the graph is mechanically derived from structural rules rather than inferred probabilistically, the compiler can perform highly reliable optimization decisions regarding:

- parallelization,
- vectorization,
- inlining,
- memory scheduling,
- and execution isolation.

---

# 📦 Ecosystem & Ingestion Axioms

## A8 — Dual Ingestion Path (add vs. add-unpure)

### add (Default / Deterministic Path)
The standard ingestion path accepts only deterministic, structurally pure `.lk` logic.

- Hidden side-effect injection is mechanically rejected.
- Imported code remains causally analyzable.
- Pure modules may treat external state only as explicit immutable input.

### add-unpure (Explicit Exception Boundary)
Impure dependencies must be declared explicitly.

This includes:
- FFI bridges,
- native binaries,
- hardware interaction,
- system interfaces,
- or nondeterministic execution sources.

The impurity boundary is therefore visible, explicit, and traceable within the dependency graph.

### Standard Library
To reduce dependency fragmentation and fragile micro-import ecosystems, critical low-level impure primitives are provided as part of the official runtime environment.

---

## A9 — Subrepo Isolation (Immutable Vendor Boundary)

External dependencies are isolated as independent subrepositories.

- The standard deterministic ingestion path accepts only `.lk` source files.
- Purity guarantees are compiler-verified structurally, not socially assumed.
- If a previously deterministic dependency introduces impure behavior during synchronization, ingestion aborts immediately with a hard structural error.

This prevents silent contamination of deterministic dependency chains.

---

## A10 — Transitive Verification Mesh

Because deterministic modules possess stable and reproducible execution behavior, verification becomes highly compositional.

- Local tests remain precise and inexpensive.
- Validation propagates transitively through dependency relationships.
- Downstream consumers continuously and implicitly revalidate imported deterministic logic through their own execution and testing processes.

As dependency chains expand, this forms a distributed verification mesh across the ecosystem.

When regressions, incompatibilities, or behavioral deviations emerge, the resulting breakage propagates visibly through dependent validation paths, rapidly surfacing structural inconsistencies across affected consumers.

---

## A11 — Native Git Synchronization & Local Patch Ownership

Dependencies are not opaque locked artifacts.

Developers may:
- patch,
- optimize,
- fork,
- or locally adapt imported packages at any time using standard Git workflows.

Upstream synchronization occurs through native Git operations and conflict resolution mechanisms.

The toolchain tracks local modifications and deterministically reapplies maintained patches during future updates whenever structurally possible.

This preserves long-term developer sovereignty over dependency behavior and integration.