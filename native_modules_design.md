# Quark Extensions Interface (QEI) Design

Status: Design Document (Draft)
Date: 2026-03-24

## 1. Vision

Quark is a high-level data language that compiles to C++17 native binaries. Its value proposition is R/dplyr ergonomics with compiled-binary performance.

Two insights drive this design:

1. **The boundary between Quark and C++ should be invisible.** Users write high-level pipelines. Performance-critical code lives in C++. The transition is seamless — same types, same binary, zero overhead.

2. **The extension mechanism powers the library ecosystem.** Like Python's C extensions enabled NumPy/pandas, Quark extensions enable the data library ecosystem without the language itself becoming bloated.

### Core Design Principle

**QValue is the interface. Typed data is the implementation. Extensions bridge them.**

```
User writes high-level Quark (QValue world)
    -> data stored as typed vectors (contiguous arrays)
        -> operations dispatch to extensions for bulk work
            -> extensions access raw typed arrays (C++ speed)
                -> results flow back as QValues to user code
```

### Why Quark Has a Structural Advantage

Quark compiles to C++. C++ is not foreign to Quark — it IS Quark's execution environment. Every Quark function already compiles to a C++ function taking `QValue` and returning `QValue`. An extension is just more C++ functions with the same signature.

This means:
- No marshalling between Quark and C++ (unlike Python's C API or R's `.Call()`)
- No dynamic linking needed — everything compiles into one static binary
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

### 2.2 How It Integrates with the Catalog

The key architectural insight: `extern fn` and `extern method` declarations create entries in the **same catalog** that hardcoded builtins use today. The analyzer, CallPlan generation, and codegen work unchanged.

```
catalog.go (hardcoded core intrinsics: print, len, type, push, pop, get, set, ...)
    +
extern declarations from .qrk files (stdlib + user extensions)
    ↓
Analyzer populates builtinSigs + methodSigs (unchanged)
    ↓
CallPlan generation (unchanged)
    ↓
Codegen emits runtime symbol calls (unchanged)
```

An `extern` declaration maps directly to an existing catalog `Spec`:

```quark
extern method str.upper() str as 'q_upper'
```

Creates the same entry as the current hardcoded line in `catalog.go`:

```go
{Name: "upper", Runtime: "q_upper", MinArgs: 0, MaxArgs: 0,
 ParamTypes: []TypeKey{}, ReturnType: TypeString, ReceiverType: TypeString}
```

Same `Name`, `Runtime`, arity, types, `ReceiverType`. The analyzer finds `(TypeString, "upper")` in the registry, builds a `CallPlan` with `RuntimeSymbol: "q_upper"`, codegen emits `q_upper(receiver)`. No new dispatch modes. No special cases.

### 2.3 The `as` Clause

The `as 'symbol'` specifies the C++ function name (maps to the `Runtime` field in the catalog Spec). It tells codegen exactly what C++ function to call. Required for all extern declarations — no guessing.

```quark
extern fn sqrt(x: float) float as 'q_sqrt'
// → Spec{Name: "sqrt", Runtime: "q_sqrt", MinArgs: 1, MaxArgs: 1, ...}

extern method str.upper() str as 'q_upper'
// → Spec{Name: "upper", Runtime: "q_upper", ReceiverType: TypeString, ...}
```

### 2.4 Two-File Pattern

Each extension consists of:
- A `.qrk` file declaring the module interface (extern declarations + optional Quark wrapper functions)
- A `.hpp` file containing the C++ implementations

```quark
// stdlib/io.qrk — Quark interface
extern source 'io_impl.hpp'

module io:
    // Extern function: signature in Quark, body in io_impl.hpp
    extern fn read_all(path: str) result as 'quark_io_read_all'

    // Pure Quark function: composes extern primitives
    fn read_lines(path: str) result ->
        when read_all(path):
            ok text -> ok text.split('\n')
            err e -> err e

    fn read_csv(path: str, sep: str = ',') result ->
        when read_lines(path):
            err e -> err e
            ok lines ->
                headers = lines.get(0).split(sep)
                data = dict {}
                for h in headers:
                    data = data.set(h.trim(), list [])
                for i in range(1, len(lines)):
                    line = lines.get(i)
                    if line.trim() != '':
                        values = line.split(sep)
                        for j in range(len(headers)):
                            col = headers.get(j).trim()
                            data.get(col).push(values.get(j))
                ok data
```

```cpp
// stdlib/io_impl.hpp — C++ implementation
#include <fstream>
#include <sstream>

// Calling convention: QClosure* _cl first (ignore it), then QValue params, return QValue
QValue quark_io_read_all(QClosure* _cl, QValue path) {
    if (path.type != VAL_STRING) {
        q_runtime_reportf("io.read_all: expected str, got %s", q_type_name(path));
        exit(1);
    }
    std::ifstream file(path.s);
    if (!file.is_open()) {
        return qv_err(qv_string(q_strdup("could not open file")));
    }
    std::ostringstream ss;
    ss << file.rdbuf();
    return qv_ok(qv_string(q_strdup(ss.str().c_str())));
}
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

```cpp
// === Preamble ===
#include "quark/quark.hpp"          // runtime
#include "quark/ext/api.hpp"        // extension author API
#include "std/io_impl.hpp"          // extern source from stdlib
#include "ext/parquet_impl.hpp"     // extern source from extension

// === Forward declarations ===
QValue quark_main(QClosure*);

// === User's compiled Quark functions ===
QValue quark_main(QClosure* _cl) {
    // ... user code, calling quark_io_read_all etc. ...
}

// === Entry point ===
int main() {
    q_gc_init();
    quark_main(nullptr);
    return 0;
}
```

### 2.8 Calling Convention

All functions — Quark-compiled and extern — share the same calling convention:

```cpp
QValue function_name(QClosure* _cl, QValue param1, QValue param2, ...);
```

- `QClosure* _cl`: Hidden first parameter. Extern functions should accept it but can ignore it (it's `nullptr` for direct calls).
- All parameters are `QValue`.
- Return value is `QValue`.

This uniformity means the analyzer and codegen don't need special dispatch logic. A call to an extern function is identical to a call to a builtin or Quark function.

### 2.9 Callbacks: C++ Calling Quark Closures

Extensions can call Quark closures using the existing `q_call*` functions:

```cpp
// Extension filter — takes a Quark predicate, applies it in C++
QValue quark_data_fast_filter(QClosure* _cl, QValue list_val, QValue predicate) {
    QList* src = list_val.list;
    QList* result = q_new_list();
    for (int i = 0; i < src->length; i++) {
        QValue item = src->items[i];
        QValue keep = q_call1(predicate, item);  // calls Quark closure
        if (q_truthy(keep)) {
            q_list_push(result, item);
        }
    }
    return qv_list(result);
}
```

This enables the pattern that makes NumPy's `apply()` and pandas' `map()` possible — C++ doing iteration, calling back into user-defined Quark logic.

---

## 3. Catalog Evolution

### 3.1 What Stays in catalog.go

Only intrinsics the compiler needs awareness of for type inference, operator lowering, or codegen specialization:

- **I/O**: `print`, `println`, `input`
- **Type system**: `len`, `type`, `to_str`, `to_int`, `to_float`, `to_bool`
- **Control**: `range`
- **Results**: `ok`, `err`, `is_ok`, `is_err`, `unwrap`
- **Data structure methods**: `.push()`, `.pop()`, `.get()`, `.set()`, `.keys()`, `.values()`, `.items()`, `.slice()`, `.reverse()`, `.concat()`
- **Collection methods**: `.enumerate()`, `.join()`, `.to_vector()`, `.to_list()`
- **Vector methods**: `.fillna()`, `.astype()`

~30 entries. These remain hardcoded because the compiler uses them for type inference and codegen decisions.

### 3.2 What Migrates to extern Declarations

Everything else moves to `.qrk` files with `extern fn` / `extern method`:

- **String methods**: `.upper()`, `.lower()`, `.trim()`, `.split()`, `.contains()`, etc. → `std/string.qrk`
- **Math**: `sqrt`, `floor`, `ceil`, `round`, `abs` → `std/math.qrk`
- **File I/O**: all `_file_*` builtins → `std/io.qrk`
- **Formatting**: all `_fmt_*` builtins → `std/fmt.qrk`

Example migration:

```quark
// std/string.qrk
extern source 'runtime_string_methods.hpp'

module string:
    extern method str.upper() str as 'q_upper'
    extern method str.lower() str as 'q_lower'
    extern method str.trim() str as 'q_trim'
    extern method str.contains(sub: str) bool as 'q_contains'
    extern method str.startswith(prefix: str) bool as 'q_startswith'
    extern method str.endswith(suffix: str) bool as 'q_endswith'
    extern method str.replace(old: str, new: str) str as 'q_replace'
    extern method str.split(sep: str) list as 'q_split'
    extern method str.slice(start: int, end: int) str as 'q_str_slice'
```

The C++ implementations already exist in the runtime — no new C++ code needed. The `as` clause points to the existing `q_*` functions. The migration is purely moving metadata from `catalog.go` to `.qrk` declarations.

### 3.3 The `_` Prefix Convention Dies

No more `_file_open` as a "private" builtin. File I/O lives in `std/io` as extern functions. The module boundary IS the encapsulation. No naming conventions needed.

### 3.4 Auto-Loading Question

If string methods move to `std/string.qrk`, does the user need `use 'std/string'` for `"hello".upper()`?

**For now: keep core type methods (str, list, dict, vector) in the hardcoded catalog.** They're fundamental to the language — you can't write Quark without `.split()` or `.push()`. Only migrate FREE FUNCTIONS (sqrt, file ops, formatting) to extern declarations in stdlib modules.

In the future, an auto-prelude mechanism can auto-load certain stdlib modules without explicit `use`, at which point methods can migrate too.

---

## 4. Extension Author API

A stable C++ API that ships with Quark for writing extensions. Lives at `quark/ext/api.hpp`.

### 4.1 Vector Access (the Performance Bridge)

Vectors are internally contiguous typed arrays. This is where extensions achieve real speedups — raw memory access, no QValue per element, SIMD-friendly.

```cpp
// Type-safe read access
const double*   q_ext_vec_f64(QValue vec, size_t* out_len);
const int64_t*  q_ext_vec_i64(QValue vec, size_t* out_len);
const uint8_t*  q_ext_vec_bool(QValue vec, size_t* out_len);
size_t          q_ext_vec_len(QValue vec);
int             q_ext_vec_dtype(QValue vec);  // F64, I64, BOOL, STR

// Vector construction
QValue q_ext_vec_from_f64(const double* data, size_t len);
QValue q_ext_vec_from_i64(const int64_t* data, size_t len);
QValue q_ext_vec_from_bool(const uint8_t* data, size_t len);
```

Usage in an extension:
```cpp
QValue quark_data_fast_sum(QClosure* _cl, QValue vec) {
    size_t len;
    const double* data = q_ext_vec_f64(vec, &len);
    if (!data) {
        q_runtime_reportf("fast_sum: expected vector[f64]");
        exit(1);
    }
    double total = 0.0;
    for (size_t i = 0; i < len; i++) {
        total += data[i];  // raw double, SIMD-friendly
    }
    return qv_float(total);
}
```

### 4.2 Dict Access

```cpp
QValue q_ext_dict_get(QValue dict, const char* key);
void   q_ext_dict_set(QValue dict, const char* key, QValue val);
size_t q_ext_dict_len(QValue dict);
```

### 4.3 Closure Calling

```cpp
QValue q_ext_call(QValue closure, QValue arg);
QValue q_ext_call2(QValue closure, QValue arg1, QValue arg2);
QValue q_ext_call3(QValue closure, QValue arg1, QValue arg2, QValue arg3);
```

### 4.4 Error Reporting

Extensions use the existing `q_runtime_reportf()` from `diagnostics.hpp`. Source location is automatically set by generated code before calling the extension function, so error messages point to the correct `.qrk` line.

```cpp
// Fatal error — prints formatted error and exits
q_runtime_reportf("io.read_all: could not open file '%s'", path.s);
exit(1);

// Non-fatal — return err to caller
return qv_err(qv_string(q_strdup("could not open file")));
```

Error output format (from existing infrastructure):
```
error[QK-RUNTIME-001] (runtime): io.read_all: could not open file 'missing.csv'
  at script.qrk:15:4
```

### 4.5 Value Construction

The existing `qv_*` constructors are the API:

```cpp
qv_int(42)                    // QValue with type VAL_INT
qv_float(3.14)                // QValue with type VAL_FLOAT
qv_string(q_strdup("hello"))  // QValue with type VAL_STRING (GC-managed copy)
qv_bool(true)                 // QValue with type VAL_BOOL
qv_null()                     // QValue with type VAL_NULL
qv_ok(inner_val)              // QValue with type VAL_OK
qv_err(inner_val)             // QValue with type VAL_ERR
```

### 4.6 Memory Management

All allocations must use GC-aware functions:

```cpp
q_malloc(size)           // GC-tracked allocation (for data containing pointers)
q_malloc_atomic(size)    // GC-tracked, no-scan (for strings, numeric buffers)
q_strdup(str)            // GC-tracked string copy
```

---

## 5. Performance Architecture

### 5.1 The QValue Bottleneck

QValue is a tagged union. Looping over QValue arrays requires per-element type checking and unboxing. This overhead means C++ loops over QValue lists are only marginally faster (~2-3x) than equivalent Quark loops.

### 5.2 Vectors as the Performance Bridge

Vectors store data as contiguous typed arrays:

```
vector[f64]  →  QVecF64 = std::vector<double>     → contiguous double*
vector[i64]  →  QVecI64 = std::vector<int64_t>    → contiguous int64_t*
vector[bool] →  QVecU8  = std::vector<uint8_t>     → contiguous uint8_t*
vector[str]  →  QStringStorage = offset-encoded bytes
```

Extensions accessing raw vector data achieve 100x+ speedups over QValue loops: no type checking per element, contiguous memory access, auto-vectorizable (SIMD).

This is Quark's equivalent of NumPy's ndarray. The vector type bridges high-level Quark and high-performance C++.

### 5.3 Performance Spectrum

```
Pure Quark code                              → baseline
Quark with type annotations + scalar tiering → 2-5x (compiler de-boxes where it can)
Vector operations in Quark                   → 10-50x (bulk ops on typed arrays)
Extensions on vector internals               → 100x+ (raw C++, SIMD, parallelizable)
External C++ libraries (Arrow, BLAS)         → industry-best performance
```

### 5.4 QValue as IR, Not Final Representation

Long-term direction: QValue should sit at the IR / analyzer level, not as the universal runtime representation.

```
Quark source ——— Analyzer/IR ——— Generated C++
                    QValue           unboxed stack types
                   (abstract)        (concrete, fast)
```

The scalar tiering work (storing `int` as `long long`, `float` as `double` instead of QValue for locals with known types) is the first step. Future steps:

- Function parameters with type annotations passed as raw types
- Function return values with type annotations returned as raw types
- Extension function signatures using raw types when fully annotated
- Automatic unboxing at function boundaries when both caller and callee types are known

For v0.1, QValue everywhere is correct and simple. Type annotations provide the information; the compiler can exploit it incrementally.

### 5.5 Vectors Should Be Central

Vectors should be the default data container for data work, not lists:

- `range()` should return a vector (not a list)
- Data operations (filter, sort, group_by) should operate on vectors / dict-of-vectors
- More vector operations needed: comparison (`vec > 5` -> bool vector), map, etc.
- Lists remain for heterogeneous collections (configs, argument lists, mixed data)

---

## 6. Stdlib Architecture

### 6.1 Three-Layer Model

```
+---------------------------------------------------------+
|  Layer 3: User Code                                     |
|  Writes data pipelines using stdlib + extensions        |
+---------------------------------------------------------+
|  Layer 2: Quark Stdlib (.qrk + optional extern .hpp)    |
|  read_csv, filter, map, group_by, sort, fmt.table       |
|  Written in Quark for dogfooding.                       |
|  Performance-critical parts implemented in C++ via      |
|  extern declarations.                                   |
+---------------------------------------------------------+
|  Layer 1: Builtins (catalog, ~30 intrinsics)            |
|  Compiler-aware primitives: print, len, type,           |
|  to_*, range, push, pop, get, set, ok, err, unwrap     |
|  These need compiler awareness for type inference,      |
|  operator lowering, and codegen specialization.         |
+---------------------------------------------------------+
```

### 6.2 Stdlib Module Layout

Each module has up to two files:

```
stdlib/
  io.qrk           <- Quark source + extern declarations
  io_impl.hpp      <- C++ implementations for extern functions
```

### 6.3 Planned Stdlib Modules

**std/io** — File I/O
```
extern:    read_all(path)              C++ file I/O syscalls
extern:    write_all(path, data)       C++ file I/O syscalls
extern:    exists(path)                C++ stat()
quark:     read_lines(path)            split(read_all(path), '\n')
quark:     read_csv(path, sep)         parse lines, build dict-of-vectors
quark:     write_csv(data, path)       serialize, write_all
```

**std/data** — Data operations (mostly Quark — dogfooding layer)
```
quark:     filter(data, predicate)     for loop + if + push
quark:     map(data, transform)        for loop + push
quark:     reduce(data, fn, init)      for loop + accumulate
quark:     group_by(data, key)         for loop + dict building
quark:     select(data, columns)       dict subsetting
quark:     head(data, n)               slice
quark:     tail(data, n)               slice
extern:    sort(data, key, desc)       C++ std::sort for performance
```

**std/fmt** — Formatting and display
```
extern:    table(data, n)              C++ for fast string building
quark:     show(value)                 to_str wrapper
```

**std/math** — Extended math operations
```
extern:    sqrt, floor, ceil, round    libc calls
quark:     mean, median, std           compose from sum/len/sort
```

**std/string** (future, after auto-prelude is implemented)
```
extern:    str.upper, str.lower, ...   wraps existing q_* functions
```

---

## 7. Extension Ecosystem

### 7.1 Writing an Extension

Example: wrapping SQLite for Quark.

```quark
// ext/sqlite.qrk
extern source 'sqlite_impl.hpp'

module sqlite:
    extern fn open(path: str) result as 'quark_sqlite_open'
    extern fn query(db, sql: str) result as 'quark_sqlite_query'
    extern fn close(db) result as 'quark_sqlite_close'

    // Pure Quark convenience function
    fn run(path: str, sql: str) result ->
        r = open(path)
        if is_err(r):
            return r
        db = unwrap(r)
        result = query(db, sql)
        close(db)
        result
```

```cpp
// ext/sqlite_impl.hpp
#include <sqlite3.h>
#include "quark/ext/api.hpp"

QValue quark_sqlite_open(QClosure* _cl, QValue path) {
    if (path.type != VAL_STRING) {
        q_runtime_reportf("sqlite.open: expected str path");
        exit(1);
    }
    sqlite3* db;
    int rc = sqlite3_open(path.s, &db);
    if (rc != SQLITE_OK) {
        return qv_err(qv_string(q_strdup(sqlite3_errmsg(db))));
    }
    return qv_ok(qv_int((long long)(uintptr_t)db));
}

QValue quark_sqlite_query(QClosure* _cl, QValue handle, QValue sql) {
    sqlite3* db = (sqlite3*)(uintptr_t)handle.i;
    // ... execute query, build QValue list of dicts ...
    return qv_ok(results);
}

QValue quark_sqlite_close(QClosure* _cl, QValue handle) {
    sqlite3* db = (sqlite3*)(uintptr_t)handle.i;
    sqlite3_close(db);
    return qv_ok(qv_null());
}
```

Usage from user code:
```quark
use 'ext/sqlite' as db

result = db.run("analytics.db", "SELECT region, SUM(revenue) FROM sales GROUP BY region")
println(unwrap(result))
```

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

Unblocks error propagation in stdlib functions. Without it, every fallible call adds a `when` nesting level.

```quark
fn read_csv(path: str) result ->
    r = io.read_all(path)
    if is_err(r):
        return r
    content = unwrap(r)
    // ... parse CSV (flat, readable) ...
```

Error propagation pattern: `if is_err(r): return r` followed by `unwrap(r)`. Explicit, readable, no special operators.

### 8.2 String Interpolation (Priority: High)

Syntax: `"Hello, !{name}"` (already sketched in grammar.md as future).

Implement BEFORE stdlib work. Unblocks readable error messages and formatting.

### 8.3 `extern source`, `extern fn`, `extern method` (Priority: High)

The core extension mechanism. Described in detail in Sections 2-3.

### 8.4 More Vector Operations (Priority: Medium)

Needed for data pipelines. Current gaps:

- Comparison operators returning bool vectors: `vec > 5` -> `vector[bool]`
- `map` over vectors: `vec.map(fn x -> x * 2)`
- `where(bool_vec, true_val, false_val)` — conditional selection
- `unique`, `cumsum`, `mean`, `std` — statistical operations

### 8.5 Dataframe / Table Type (Priority: Medium)

A first-class type for columnar tabular data. Performance is the primary driver. Internal representation may use vectors as building blocks. Replaces the current dict-of-vectors pattern. Exact design TBD in a separate document.

### 8.6 `range()` Returns Vector (Priority: Low)

`range(n)` currently returns a list. For data work, it should return a vector (typed, contiguous). Breaking change but aligns with vectors being central.

---

## 9. Comparison with FFI Spec

The existing `ffi_v0_1_spec.md` describes a traditional FFI with a stable C ABI boundary and marshalling between QValue and raw C types.

The Quark Extensions Interface takes a different approach:

| | FFI Spec (ffi_v0_1_spec.md) | Quark Extensions (this doc) |
|---|---|---|
| Boundary | Stable C ABI with marshalling | No boundary — same C++ code |
| Data types at boundary | Scalars + strings + opaque handles | Everything (QValue is universal) |
| Can pass lists/dicts/vectors? | No (explicit non-goal) | Yes |
| Can C++ call Quark closures? | No (explicit non-goal) | Yes (via q_call*) |
| Marshalling | Compiler-generated shims | None needed |
| Overhead per call | Extract -> call -> box | Zero (direct function call) |
| Compiler changes | New keywords, AST nodes, shim codegen, runtime helpers | `extern` declarations add to existing catalog |
| ABI stability | Cross-compiler safe (C ABI) | Tied to Quark's QValue layout (single compiler) |

**Recommendation:** Start with QEI (simpler, more powerful, zero overhead). The FFI spec's auto-marshalling approach can be built later as syntactic sugar on top — the compiler generates C++ wrapper code automatically for declared C function signatures.

---

## 10. Implementation Plan

### Phase 1: Language Features That Unblock Stdlib

1. **`return` keyword** — enables error propagation in stdlib
2. **String interpolation** (`"Hello, !{name}"`) — enables readable formatting
3. **More vector operations** — comparison, map, statistical ops

### Phase 2: Quark Extensions Interface

4. **`extern source 'path.hpp'`** — codegen emits `#include`
5. **`extern fn name(params) type as 'symbol'`** — adds entry to catalog, no body generated
6. **`extern method type.name(params) type as 'symbol'`** — adds method entry to catalog
7. **Extension author API header** (`quark/ext/api.hpp`) — vector access, dict access, closure calling, error helpers
8. **Path resolution** for `extern source` (same logic as `use` imports)

### Phase 3: Data Types + Performance

9. **Dataframe / table type** (using vectors internally)
10. **Push QValue toward IR** — more aggressive unboxing from type annotations
11. **`extern link`** for external library linking
12. **`range()` returns vector** instead of list
13. **Migrate builtins** out of catalog into extern declarations in stdlib

---

## 11. Future Optimization: AOT Compilation

Not part of the core extension architecture, but a natural optimization enabled by it.

Since Quark compiles to C++, stdlib `.qrk` modules could be pre-compiled to `.hpp` files (`quark compile-module io.qrk -o io_compiled.hpp`). At user compile time, the compiler would include the pre-compiled header instead of re-compiling the stdlib. This is the same mechanism as extern modules — the `.qrk` provides signatures, the `.hpp` provides implementation.

Deferred until compile times become a concern.

---

## 12. Testing Strategy

### 12.1 Extension Tests

The existing integration test framework works: write `.qrk` test programs that use extensions, check stdout output.

```quark
// test_extern_io.qrk
use 'std/io' as io

r = io.read_all("testdata/sample.txt")
println(is_ok(r))   // true
println(unwrap(r))   // file contents
```

### 12.2 Extension Author C++ Tests

The existing Catch2 test infrastructure in `runtime/tests/` can test extension functions directly as C++ unit tests.

---

## 13. Open Questions

1. **Opaque handle type**: Extensions wrapping C libraries (SQLite, Arrow) need to pass around opaque pointers. Currently stored as `qv_int((long long)ptr)` which is fragile. Should Quark have a dedicated `handle` type in QValue?

2. **Module visibility**: All functions in a module are currently accessible. No pub/private distinction. This is intentional for now (open by default).

3. **Dataframe design**: Needs its own design document. First-class type vs dict-of-vectors? Column access syntax? Schema representation?

4. **Auto-prelude**: Should certain stdlib modules (string methods, math) be auto-loaded without explicit `use`? This would enable migrating more builtins to extern declarations without breaking existing code.

5. **Package distribution**: How do community extensions get discovered, installed, and version-managed? v2+ concern but the design should not preclude it.
