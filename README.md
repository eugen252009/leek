# The mC Whitepaper

**Mechanical C — Deterministic Systems Programming Through Mechanical Rules**

> “Do not hide complexity. Organize it.”

---

### 1. Introduction

Mechanical C (mC) is a systems programming language concept designed around one central idea:

> The programmer should be able to understand memory, lifetime, data flow, and performance directly from the structure of the code itself.

Modern programming languages often solve memory safety through hidden runtimes, garbage collectors, complex ownership systems, implicit allocations, or heavy compiler analysis. mC takes a different approach.

Instead of hiding complexity behind abstractions, mC attempts to make the underlying mechanics visible, deterministic, and mechanically understandable. The goal is not to eliminate complexity. The goal is to align software structure with how memory and processors actually work.

---

### 2. Core Philosophy

mC is built around a small number of invariant rules. These rules are absolute.

Everything else—interfaces, threading, abstractions, pipelines, libraries—must operate within these rules. **No language feature may ever override or bypass these core mechanics.** The language is intentionally designed to:

* Favor explicitness over magic.
* Favor deterministic lifetimes over hidden runtimes.
* Favor reusable memory over repeated allocation.
* Favor visible costs over implicit behavior.
* Map cleanly to the stack, making aggressive, predictable inlining the default behavior.

---

### 3. The Five Core Rules

#### Rule 1 — Scopes Own Memory

Every `{}` block defines a scope. A scope is not merely syntactic structure; it is a physical memory region. All data created inside a scope belongs to that scope.

When the scope ends:

* The data is destroyed.
* References become invalid.
* Temporary memory is released deterministically.

```c
{
    Buffer local_buffer = create_buffer(1024);
} // local_buffer is deterministically released here.

```

The scope owns the data — not the variable. Variables are only handles into the scope’s memory region.

#### Rule 2 — Upward Escapes are Explicit, Downward Flow is Free

Data flow is strictly directional, tied to the mechanical reality of the stack.

**Downward Flow (Borrowing):** Passing a reference to a child scope or a nested function is implicitly safe and zero-cost. The parent scope guarantees the memory will outlive the child.

```c
{
    Buffer data = load_data();
    process(data); // Safe. 'process' is a child scope and dies first.
}

```

**Upward Escape (Moving/Copying):** Data may *only* leave a scope upwards through explicit action. Hidden lifetime extension is forbidden.
To escape a scope, the programmer must use explicit mechanisms like `return`, a built-in `move()`, or `copy()`.

```c
fn get_data() -> Buffer {
    Buffer temp = create_buffer(1024);
    return move(temp); // Explicit transfer of ownership
}

```

The programmer must always know when memory is duplicated and when ownership changes.

#### Rule 3 — Reuse Memory Instead of Recreating It

mC encourages reusable memory regions and reusable buffers.

The preferred design is:

```c
Buffer reusable = create_buffer(1024);
for (item in data) {
    reusable.reset();
    process(item, reusable);
}

```

Repeated allocation inside loops is treated as an architectural smell. The language and linter encourage stable lifetimes, cache locality, and allocator awareness (e.g., using Arenas).

#### Rule 4 — `impure` Marks Contact With the External World

By default, functions are deterministic. Any operation involving files, networking, clocks, devices, external APIs, or raw C-pointers must be explicitly marked as `impure`.

```c
impure fn open_file(path: String) -> File { ... }

```

`impure` defines a strict boundary between deterministic computation and unpredictable external systems. Data crossing this boundary must be validated. After leaving an impure boundary, deterministic processing resumes. An error cannot escape an impure block unhandled.

#### Rule 5 — The Language Must Remain Mechanically Explainable

Every language feature must be explainable through the core rules. No feature may violate scope ownership, deterministic lifetime behavior, explicit data movement, or visible external interaction.

If a feature requires bending the five rules to work, it does not belong in mC. The language must remain mechanically understandable, architecturally predictable, and mentally traceable.

---

### 4. Memory Model and Allocators

mC uses region-oriented memory semantics. Memory belongs to scopes. Because scopes behave like stack frames, the compiler can heavily optimize and inline code without guessing.

For dynamic needs, mC relies on explicit allocator injection (like Arenas) rather than a global magic heap. When an Arena's scope ends, all memory within it vanishes instantly. This eliminates use-after-free, hidden heap ownership, and uncontrolled fragmentation.

---

### 5. Explicit Copying

Copying memory is considered a meaningful architectural decision. Therefore, copies are explicit, visible, and lintable.

```c
Buffer backup = copy(original_data);

```

Copies are not forbidden, but they are intentionally visible to expose performance costs directly in the syntax.

---

### 6. The Role of the Linter

The compiler guarantees structural correctness (The Rules). The linter acts as a mechanical mentor.

The linter is designed to teach lifetime awareness, cache-friendly design, and deterministic structure. It will warn about:

* Repeated allocations in loops.
* Unnecessary copies where a reference (downward flow) would suffice.
* Unhandled validation paths at `impure` boundaries.

Warnings are advisory, but they guide the developer toward the "mechanical ideal."

---

### 7. Simplicity vs. Simplicity Illusion

mC does not attempt to eliminate complexity. Modern systems are inherently complex. Instead, mC attempts to organize complexity, expose costs, and align software structure with hardware reality.

The language favors visible architecture, explicit lifetimes, deterministic structure, and mechanical reasoning over hidden automation.

---

### 8. Intended Outcome

mC aims to make memory behavior, ownership, dataflow, performance implications, and lifetime boundaries as visible as control flow already is today.

The ideal mC developer should be able to read code and immediately infer where memory lives, when it dies, when copying occurs, where nondeterminism enters the system, and where performance costs exist.

> “Everything in mC must remain explainable through a small set of mechanical rules.”


---

### 9. Near-Zero Cost Testing (Mechanical Validation)

In mC, testing is not an afterthought or a separate framework—it is a direct consequence of **Rule 4 (impure)**. Because the language enforces a strict boundary between deterministic logic and the external world, validation becomes a mechanical certainty.

#### The "Pure" Advantage

Since `pure` functions are mathematically deterministic (same input always equals same output), they can be validated with zero setup, zero mocking, and zero state management.

```c
// Built-in validation syntax
assert(calculate_dimensions, 10, 20).toBe(200);

```

* **Static Execution:** The compiler can choose to execute these assertions during transpilation. If a test fails, the "machine" doesn't even get built.
* **Loop Validation:** Testing an entire range of tolerances becomes a simple loop.
* **Production Erasure:** Since mC transpiles to C, all test logic is stripped out in the release build. You pay for the validation during development, but the final "product" carries zero overhead.

#### Why it’s "Near-Free"

It is not 100% free (it costs developer thought and a few milliseconds of compile time), but compared to traditional suites, it eliminates:

1. **Context-Switching:** You test where you code.
2. **Runtime-Dependency:** No test-runner or VM required.
3. **Mocking-Hell:** Since dependencies are passed downward (Rule 2), you just pass the data you want to test.

> “A machine is not finished when it is assembled, but when it is measured. mC makes the measurement as simple as the assembly.”
