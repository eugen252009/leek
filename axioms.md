## 📐 The Mechanical Axiomatic System for Leek (.lk)
### A1 — Scope-Containment (Lexical Isolation)
 * Every entity (variable, memory, value) exists strictly within a uniquely defined, hierarchically nested scope.
 * No entity is directly visible or accessible outside of its declaring scope.
 * Access is rigidly restricted to the current scope or more deeply nested sub-scopes.
### A2 — Single Escape Path (The Return Axiom)
 * The only permitted path to pass a data value from a child scope to a parent scope is an explicit return.
 * There are zero implicit escapes.
 * No pointers or references may be passed upwards if they outlive the lifespan of their originating scope.
 * Global variables or hidden side-channels outside of this mechanism do not exist.
### A3 — Purity / Passive Data Export
Every operation is strictly split into a binary classification:
 * **Pure (Deterministic):** Pure data processing. The same input always guarantees the exact same output (Input \rightarrow Output). No mutation of state occurs outside the local scope.
   * *The Functional Exception:* Purely passive outbound effects (read-only) that capture data fleetingly (e.g., print) are permitted, provided they leave the internal computation state and the determinism of the data 100% untouched.
 * **Impure (Stateful):** Non-deterministic operations or actions that actively manipulate the outside world (e.g., writing to hardware registers, mutating memory outside the local isolation space, waiting for interrupts, FFI calls).
### A4 — Controlled Effect Boundary (Context Isolation)
Interactions with the external environment are strictly separated based on their direction of causality:
 * **Inbound Effects (Mutating):** Operations that read external data, alter global states, or disrupt the control flow are exclusively bound to impure scopes. Pure code is fundamentally barred from reading impure state.
 * **Outbound Effects (Observing):** A pure block is allowed to trigger passive sinks (like print). Since this data only flows *out* of the system, the causal graph of the internal computation remains completely clean and maximally optimizable for the CPU.
### A5 — Arena-Based Lifetime Model
Memory management is never ad-hoc; it is mechanically welded to the structural layout of the scopes:
 * Every memory allocation occurs inside an arena that belongs strictly to a single scope (or a tightly defined scope cluster).
 * Deallocation of the entire memory space happens atomically and completely the exact moment the owning scope is exited.
 * There is no partial, manual deallocation (free()) during the runtime of the scope.
### A6 — Referential Closure (No Hidden Aliasing)
Every value possesses a closed reference set R(v), completely tracked and controlled by the compiler.
 * R(v) represents the set of all valid references to a given value v.
 * It stands as an ironclad law that: 
 * Upon exiting the defining scope, the set instantly collapses: R(v) = \emptyset.
 * No unmodeled reference paths or side-effects through aliasing can exist, allowing the compiler to perfectly parallelize memory access directly within the register layout.
### A7 — Dependency Derivation (The Graph Axiom)
 * The complete data- and control-flow graph (dependency graph) of the program is not an artificial layer built on top, but a direct, logical mathematical derivation from axioms **A1** through **A6**.
 * Edges within the graph are generated exclusively by *Read-before-Write* relationships, write causalities, scope nestings, and return flows.
 * Because the graph emerges cleanly out of the structural scopes, the compiler can directly decide—without complex or slow heuristics—where code can be parallelized, inlined, or vectorized via SIMD.

## 📦 Ecosystem & Ingestion Axioms (The Package Manager)
### A8 — Dual Ingestion Path (add vs. add-unpure)
 * **add (Alias for add-pure / The Default):** Ingests exclusively deterministic logic into the system. Implicit importing of side-effects is physically impossible. Pure code treats external state purely as immutable input.
 * **add-unpure (The Explicit Exception):** Explicitly encapsulates and permits the injection of non-deterministic side-effects, external binaries, or FFI bridges. It is the declared "black sheep" of the dependency graph.
 * **The Standard Library (Batteries Included):** To eliminate the ecosystem's need for brittle, external micro-imports, Leek provides critical impure primitives (I/O, networking, file-watchers) built-in out of the box.
### A9 — Subrepo Isolation (The Immutable Vendor)
 * External dependencies are cloned and isolated as independent subrepos.
 * The compiler restively accepts **only .lk source files** within the standard add path.
 * **The Breakage Guarantee:** If a previously trusted pure library introduces unpure behavior during an upstream sync, the compiler immediately aborts the ingestion with a hard error, alerting the developer before toxic code can infiltrate the system.
### A10 — Transitive Wave Verification (The Self-Validating Mesh)
 * Since pure logic is inherently deterministic via **A3** and **A6**, unit testing is ultra-simple and execution cost is near-zero.
 * These highly precise local tests weave a transitive verification mesh across the entire dependency graph.
 * **The Wave Effect:** If a regression, logic error, or incompatibility occurs anywhere within the system, the disruption behaves like a shockwave tearing through the mesh. The failure is immediately isolated and surfaced at the exact point of breakage, preventing silently corrupted states.
### A11 — Native Git Synchronization & Local Patches
 * An ingested package is not an untouchable monolith. Updates and external dependencies can be modified, optimized, or custom-patched by the developer locally at any time using standard Git workflows.
 * **The Consequence:** Upstream updates are integrated via Git, leveraging native conflict resolution (git rerere). The toolchain tracks your local changes and reapplies your custom patches deterministically onto the new code. You are never held hostage by an upstream maintainer.
