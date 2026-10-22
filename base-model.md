> **Implementation boundary:** This is the aspirational graph model. The current
> scalar implementation is specified in [pure-composition.md](pure-composition.md).
> Conditional block scopes and condition-to-block control edges are now included;
> see [path-sensitive constraints](docs/path-sensitive-constraints.md).
> It retains AST evaluation/short-circuit structure alongside value/effect edges;
> DOT alone is not a scheduler. Function-call recursion is distinct from cyclic
> value dependencies and is deferred, not permanently forbidden by this model.
> Inline sequence operations now retain distinct typed semantic dataflow nodes;
> see [declarative dataflow](docs/declarative-dataflow.md). Canonical pull and fused
> scalar strategies consume the same representation without changing failure order.
> `axioms.md` remains authoritative where older terminology differs.

# 1. Core Model

The language is built upon a directed, acyclic data- and effect-graph:

## 1.1 Program Entity

A program is defined as a tuple:

$$P = (N, E, S, A)$$

* $N$: The set of Nodes (Scopes / Functions / Expressions)
* $E \subseteq N \times N$: Directed Edges (representing Data- or Control-Flow)
* $S \subseteq N$: Start Nodes (Entry Scopes)
* $A$: Arena Allocator Layout (linear memory regions assigned per Scope)

---

# 2. Node Types

Every node $n \in N$ possesses exactly one type:

$$n \in \{ PURE\_SCOPE, IMPURE\_SCOPE, PARALLEL\_SCOPE, MEMORY\_SCOPE, IO\_NODE \}$$

---

## 2.1 PURE_SCOPE

Characteristics:

* Deterministic
* No externally observable mutation or user I/O; unaliased local implementation mutation is permitted.
* Deterministic mapping from inputs to values or defined failures; failure behavior must be preserved.
* Referentially transparent

Formal Invariant:


$$\forall n \in PURE\_SCOPE: \nexists e \in outgoing(n) \text{ such that } SideEffect(e)$$

---

## 2.2 IMPURE_SCOPE

* Permitted to generate side effects
* Permitted to read from and write to the external world
* Generates "Boundary Events"

Formal Definition:


$$IMPURE\_SCOPE = (inputs, outputs, effects)$$

---

## 2.3 PARALLEL_SCOPE

* A discrete set of $PURE\_SCOPE$ nodes
* Zero internal dependencies within the set

Validity Constraint:


$$\forall a,b \in PARALLEL\_SCOPE: \neg(a \rightarrow b \vee b \rightarrow a)$$

---

## 2.4 MEMORY_SCOPE (Arena)

* A linear memory region
* Strictly scoped lifetime bound

Formal Definition:


$$\text{Arena } A = [base, base + size]$$

Invariant Rule:


$$\forall allocation \in A: lifetime(allocation) \subseteq scope(n)$$

---

## 2.5 IO_NODE

* The concrete bridge between the operating system and the program system
* Enforces an absolute $IMPURE$ classification

---

# 3. Edges

## 3.1 Edge Classifications

$$E = \{ DATA\_EDGE, CONTROL\_EDGE, EFFECT\_EDGE \}$$

---

## 3.2 Semantics

### DATA_EDGE

$$a \xrightarrow{DATA} b \quad \text{specifies that } b \text{ consumes the evaluation result of } a$$

### CONTROL_EDGE

$$a \xrightarrow{CONTROL} b \quad \text{specifies that } b \text{ is strictly barred from executing until } a \text{ terminates}$$

### EFFECT_EDGE

$$a \xrightarrow{EFFECT} b \quad \text{specifies that } b \text{ is causally dependent on the side effects executed by } a$$

---

# 4. Core Axioms (Immutable)

These invariants serve as the **physical constants** of your system:

* ### A1 — Scope Isolation


Every node lives within exactly one defined scope. There is zero implicit lifetime extension outside of that scope boundary.
* ### A2 — No Upward Escape


Data values are structurally barred from being implicitly referenced outside of their originating scope. Outward movement is restricted to explicit `return` operations, explicit arena transfers, or explicitly declared impure boundaries.
* ### A3 — Determinism of PURE


Any $PURE\_SCOPE$ is functionally deterministic. The mapping of $Input \rightarrow Output$ is completely unambiguous, repeatable, and pure.
* ### A4 — Explicit Side Effects


All external or mutable side effects are ruthlessly contained. They are restricted to existing exclusively within an $IMPURE\_SCOPE$.
* ### A5 — Parallelism Equals Independence


A scope qualifies as a $PARALLEL\_SCOPE$ if and only if there are zero data or effect edges between its constituent nodes.
* ### A6 — Linear Memory Binding


Every allocation belongs to exactly one specific scope. The entire memory layer allocated within that scope is atomized and invalidated instantly upon scope exit.
* ### A7 — Flow-Direction Constraint


The complete data-flow topology is a strictly Directed Acyclic Graph (DAG). Backedges within the data flow are mathematically illegal.

---

# 5. Validation Rules (Compiler Verification Passes)

## V1 — Cycle Check

The subgraph formed by $E_{DATA}$ must be verified as acyclic.


$$\text{If a cycle is detected} \rightarrow \text{emit hard compiler error (Invalid Program)}$$

## V2 — Purity Check

Any node classified as a $PURE\_SCOPE$ is strictly barred from originating or containing an $EFFECT\_EDGE$.

## V3 — Parallel Safety

A $PARALLEL\_SCOPE$ is verified as safe if and only if:

* There are zero concurrent writes to shared memory locations.
* There are zero $EFFECT\_EDGE$ occurrences within the scope set.
* There are zero internal $DATA$ dependencies between the parallel nodes.

## V4 — Lifetime Safety

The compiler enforces that no pointer or reference remains valid or addressable outside the precise boundary of its originating scope's lifetime.

## V5 — Impurity Containment

All operations generating side effects must be statically proven to route exclusively through an $IMPURE\_SCOPE$.

---

# 6. Semantics of "parallel for"

Formal Transformation:


$$\text{parallel for } R = [0..n]$$

is lowered directly by the compiler into:

$$PARALLEL\_SCOPE \{ PURE\_SCOPE(i) \}$$

subjected to the strict invariant:


$$\forall i,j: i \neq j \implies independent(i,j)$$

The corresponding reduction operation:


$$\text{reduce}(res, \oplus)$$

requires the operator $\oplus$ to be mathematically proven as both associative and commutative:


$$(a \oplus b) \oplus c = a \oplus (b \oplus c) \quad \text{and} \quad a \oplus b = b \oplus a$$

---

# 7. Memory Model (Arena + Flow)

```text
Scope Entry:
    allocate(Arena)

Scope Exit:
    invalidate(Arena region)

```

*Zero Garbage Collection, zero runtime Borrow Checker overhead $\rightarrow$ absolute static lifetime binding determined entirely by graph topology.*

---

# 8. Core Architecture in a Single Sentence

> The language is a **deterministic data-flow graph with explicit effects and linear scope-bound memory allocation**, optimized fundamentally via structural lexical isolation rather than active runtime management.
