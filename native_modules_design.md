# Quark Extensions Interface (QEI) Design

Status: Design Document (Draft)
Date: 2026-03-24

## 1. Vision

Quark is a high-level data language that compiles to C++17 native binaries. Its value proposition is high level language ergonomics (similar to Python or R/dplyr) with compiled-binary performance.

To achieve this dual mandate of ergonomics <> performance, Quark's architecture must allow users to write high-level code while enabling performance-critical operations to be implemented in C++. The Quark Extensions Interface (QEI) is the design for how this works.

Two insights drive this design:

1. **The boundary between Quark and C++ should be invisible.** Users write high-level pipelines. Performance-critical code lives in C++. The transition is seamless — same types, same binary, zero overhead.

2. **The extension mechanism powers the library ecosystem.** Like Python's C extensions enabled NumPy/pandas, Quark extensions enable the data library ecosystem without the language itself becoming bloated.

### Core Design Principle

**QValue is the interface. Typed data is the implementation. Extensions bridge them.**

```
User writes high-level Quark (QValue world)
    -> QValues are translated and stored as typed vectors (contiguous arrays) or typed atomic types
        -> operations dispatch to extensions for bulk work
            -> extensions access raw typed arrays (C++ speed)
                -> results flow back as QValues to user code
```

### Why Quark Has a Structural Advantage

Quark compiles to C++. C++ is not foreign to Quark — it IS Quark's execution environment. Every Quark function already compiles to a C++ function taking `QValue` and returning `QValue`. An extension is just more C++ functions with the same signature.

What this means for an FFI design:
- No marshalling between Quark and C++ (unlike Python's C API or R's `.Call()`)
- No dynamic linking needed — everything compiles into one static binary. Though we need dynamic loading for user extensions, the stdlib can be compiled in.
- Extensions share the same heap, GC, and type system
- Zero boundary overhead

---

## 2. The Extension Mechanism

An extension is a Quark module where some or all functions are implemented in C++ rather than Quark. Extension declarations integrate into the **existing builtin catalog** — the same registry the analyzer, invariants checker, and codegen already use. Nothing downstream changes.

### 2.1 Syntax

Three new constructs:

```quark
extern source '<path>'                                    // include a C++ implementation file
extern fn <name>(<params>) <return_type> as '<symbol>'    // declare a C++ free function
extern method <type>.<name>(<params>) <type> as '<symbol>' // declare a C++ method on a type
```

### 2.3 The `as` Clause

The `as 'symbol'` specifies the C++ function name (maps to the `Runtime` field in the catalog Spec). It tells codegen exactly what C++ function to call. Required for all extern declarations — no guessing.

```quark
extern fn sqrt(x: float) float as 'q_sqrt'
// → Spec{Name: "sqrt", Runtime: "q_sqrt", MinArgs: 1, MaxArgs: 1, ...}

extern method str.upper() str as 'q_upper'
// → Spec{Name: "upper", Runtime: "q_upper", ReceiverType: TypeString, ...}
```

### 2.5 Path Resolution

`extern source` paths resolve the same way as `use` imports:

| Path form | Resolution |
|---|---|
| `'io_impl.hpp'` | Relative to the `.qrk` file containing the directive |
| `'../shared/utils.hpp'` | Relative path traversal from `.qrk` file |
| `'std/io_impl.hpp'` | Stdlib root (same discovery as `use 'std/...'`) |

System headers (e.g., `<arrow/api.h>`, `<sqlite3.h>`) go INSIDE the `.hpp` file as normal C++ `#include`s. Quark only needs to find the extension author's code.

### 2.6 What the Compiler Does

1. **Parser**: Recognizes `extern source`, `extern fn`, and `extern method`. Creates AST nodes with signature metadata but no body.

2. **Loader**: Resolves `extern source` paths (same logic as `use` path resolution). Does NOT parse the `.hpp` file — it's opaque C++ to the compiler.

3. **Analyzer**: Processes `extern fn` / `extern method` nodes and adds entries to the **same function/method registries** populated from `catalog.go`. Validates call sites (arity, types) normally. Generates `CallPlan` with `RuntimeSymbol` from the `as` clause.

4. **Codegen**: Emits `#include "resolved/path/to/impl.hpp"` in the preamble. For extern function calls, emits calls using the `RuntimeSymbol` — identical to how builtins are emitted today. Does NOT generate a function body for extern declarations.

5. **clang++**: Compiles everything together — generated code + included extension headers — into one binary.

### 2.7 Generated C++ Structure



### 2.8 Calling Convention


### 2.9 Callbacks: C++ Calling Quark Closures


## 3. Catalog Evolution

### 3.3 The `_` Prefix Convention Dies

No more `_file_open` as a "private" builtin. File I/O lives in `std/io` as extern functions. The module boundary IS the encapsulation. No naming conventions needed.


## 4. Extension Author API

A stable C++ API that ships with Quark for writing extensions. Lives at `quark/ext/api.hpp`.

### 4.1 Vector Access (the Performance Bridge)



### 4.2 Dict Access


### 4.3 Closure Calling

### 4.4 Error Reporting

### 4.5 Value Construction

The existing `qv_*` constructors are the API:

### 4.6 Memory Management

All allocations must use GC-aware functions:

## 5. Performance Architecture


### 5.3 Performance Spectrum

```
Pure Quark code                              → baseline
Quark with type annotations + scalar tiering → 2-5x (compiler de-boxes where it can)
Vector operations in Quark                   → 10-50x (bulk ops on typed arrays)
Extensions on vector internals               → 100x+ (raw C++, SIMD, parallelizable)
External C++ libraries (Arrow, BLAS)         → industry-best performance
```

### 5.4 QValue as IR, Not Final Representation


### 5.5 Vectors Should Be Central


## 6. Stdlib Architecture


## 7. Extension Ecosystem

### 7.1 Writing an Extension

### 7.2 Extension Distribution (Future)

For v0.1: extensions are directories users copy into their project or a global extensions path. The compiler discovers them via `use 'ext/...'` path resolution.

Future: a `quark.toml` manifest and package manager.

### 7.3 Extern Link (Future)

Deferred from v0.1. Start with `extern source` only. When implemented:

```quark
extern source 'sqlite_impl.hpp'
extern link 'sqlite3'              // compiler resolves to -lsqlite3 or sqlite3.lib
```

For v0.1: users pass raw clang++ flags via `quark run program.qrk -- -lsqlite3 -I/path`.

---

## 8. Language Changes Required

### 8.1 `return` Keyword (Priority: High)



Error propagation pattern: `if is_err(r): return r` followed by `unwrap(r)`. Explicit, readable, no special operators.

### 8.2 String Interpolation (Priority: High)


### 8.3 `extern source`, `extern fn`, `extern method` (Priority: High)

The core extension mechanism. Described in detail in Sections 2-3.

### 8.4 More Vector Operations (Priority: Medium)

### 8.6 `range()` Returns Vector (Priority: Low)

`range(n)` currently returns a list. For data work, it should return a vector (typed, contiguous). Breaking change but aligns with vectors being central.

---

## 9. Comparison with FFI Spec
