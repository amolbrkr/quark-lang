# Quark Architecture & Implementation

This document covers the compiler pipeline, runtime internals, code generation, and memory management. It complements **semantics.md** (language behaviour), **grammar.md** (syntax), and **stdlib.md** (builtins).

---

## 1) Compilation Pipeline

The pipeline has seven stages. Each stage's output feeds the next.

```
.qrk source → Lexer → Parser → Loader → Analyzer → Invariants → Codegen → C++ → clang++ → binary
```

### 1.1 Lexer → Token Stream

Three-pass tokenization:
1. **Raw tokenization**: Produces tokens from source characters. Supports single-quoted and double-quoted strings with escapes (`\n`, `\t`, `\r`, `\\`, `\0`, `\'`, `\"`). Line comments with `//`.
2. **Line-start tracking**: Marks which tokens begin a new line.
3. **Indentation injection**: Converts leading whitespace after `:` or `->` + newline into `INDENT`/`DEDENT` tokens. Tracks an indent stack; mismatched dedent levels produce `ILLEGAL` tokens.

Inside brackets (`()`, `[]`, `{}`), indentation processing is suppressed — newlines are ignored and no INDENT/DEDENT is emitted.

### 1.2 Parser → AST

Recursive-descent parser producing a tree of `TreeNode`s. Uses Pratt parsing for expressions with 12 precedence levels. Named function definitions are immediately desugared to assignments (see semantics.md §6.5).

Key parser decisions:
- `fn name(...)` at statement level → named function (desugared to assignment)
- `fn(...)` in expression → lambda
- `list [...]` → list literal (keyword required, avoids ambiguity with indexing)
- Dict keys in literals are bare identifiers, interpreted as string keys

### 1.3 Loader → Merged AST

Resolves all `use` statements by loading, parsing, and splicing external files into the main AST. See semantics.md §10 for resolution rules.

### 1.4 Analyzer → Annotated AST + Metadata

Two-pass semantic analysis:
1. **Predeclaration pass**: Scans top-level statements to register all named functions and function-binding assignments. This enables forward references — a function can call another function defined later in the file.
2. **Full analysis pass**: Walks the entire AST, building scopes, type-checking expressions, computing closure captures, and generating CallPlans.

The analyzer produces five metadata outputs consumed by later stages:
- **CallPlans**: Per-call-site metadata (dispatch mode, arity, defaults to inject)
- **Captures**: Per-lambda map of captured variable names
- **CapturedByFunction**: Inverted capture map — per-function/lambda, the set of variable names that any directly-nested lambda captures from it. Used by codegen to decide `QCell*` vs scalar storage per variable.
- **NodeTypes**: Per-AST-node inferred type (`map[*ast.TreeNode]Type`). Every expression node has its analyzer-computed type recorded here. Codegen reads this to drive scalar storage selection and native operator lowering without re-deriving type information.
- **Return validation**: Per-function declared-vs-inferred return type checks

Diagnostics emitted by the analyzer are severity-tagged:
- **Errors** (`QK-CHECK-001`) are fatal and stop `check`/`emit`/`build`/`run`.
- **Warnings** (`QK-CHECK-002`) are reported to the user but are non-fatal.

#### Type checking policy (Knowability Rule)

When either the parameter type or argument type is unknown or `any`, the check is **deferred to runtime**. Only when both types are statically known does the analyzer enforce assignability.

#### Assignability rules

- `any` is assignable to/from everything
- `null` is assignable to reference types (list, dict, fn, result)
- `int` is assignable to `float` (promotion)
- Collections are covariant in element type
- All other combinations require exact match

### 1.5 Invariants → Validated

Pre-codegen checks that verify CallPlans are well-formed and return type annotations are consistent. Acts as a safety net between analysis and code generation.

Invariant breaches are compiler-bug class failures (`INV-*`) and are treated as fail-loud conditions.

### 1.6 Codegen → C++17 Source

Walks the annotated AST and emits C++17 code using the runtime headers.

**Naming**:
- User identifiers prefixed with `quark_` (e.g., `quark_x`, `quark_main`)
- Runtime builtins use `q_` prefix
- Value constructors use `qv_` prefix
- Lambdas: `quark__lambda1`, `quark__lambda2`, etc.

**Variables**: Storage is chosen per variable at first declaration using two criteria: (1) whether a nested lambda captures it, and (2) the static type from `NodeTypes`. Five storage tiers:

| Tier | C++ type | Condition |
|------|----------|-----------|
| `QCell*` | heap reference cell | captured by any nested lambda; `->value` for reads/writes |
| `long long` | native int | not captured + static type `int` + no `any` annotation |
| `double` | native float | not captured + static type `float` + no `any` annotation |
| `bool` | native bool | not captured + static type `bool` + no `any` annotation |
| `QValue` | tagged union | not captured + unknown/any/composite type, or `any`-annotated |

Closure captures received from an enclosing scope (extracted from `_cl->captures[i]`) are always `QCell*` — the cell is shared with the outer scope regardless of type.

`generateIdentifier` boxes scalar vars back to `QValue` at read sites (e.g., `qv_int(quark_x)`) so the rest of codegen always operates on `QValue` expressions. Scalar storage is transparent to all other codegen paths.

Codegen tracks storage choices in `cellVars` (names stored as `QCell*`) and `varTiers` (names stored as a named C++ scalar type), both scoped and stacked with the scope stack.

**Function calls**: Lowered according to the CallPlan's dispatch mode:
- `DispatchBuiltin` → `q_print(arg)` (direct C++ call)
- `DispatchDirect` → `quark_foo(nullptr, arg)` (known function, no closure)
- `DispatchClosure` → `q_call1(val, arg)` (dynamic dispatch through QClosure)
- `DispatchExtern` → `qei_sqrt(q_as_float(arg))` (unbox args, call native, box return)
- `DispatchNative` → `quark_foo(raw_arg)` (native C++ types, with QValue thunk for first-class use)

If codegen encounters an unexpected dispatch/runtime-symbol invariant break, it terminates with an internal compiler error (`INV-*`) rather than emitting fallback runtime behavior.

**Method calls**: When `IsMethod=true`, the receiver is injected as the first runtime argument (e.g., `"hello".upper()` → `q_upper(receiver_val)`).

**Operator lowering**: For arithmetic (`+`, `-`, `*`, `/`, `%`) and ordering comparisons (`<`, `<=`, `>`, `>=`), codegen first checks whether both operands are scalar atoms (literals or scalar-tiered local variables). If so, it emits raw C++ operators and boxes the result — e.g., `qv_int(((long long)x) + ((long long)y))` — instead of calling `q_add(...)`. Division between two `int` operands promotes to `double`. Falls back to the boxed `q_add` etc. path whenever either operand is `QValue`/unknown.

**Other lowerings**:
- **Pipe**: `x | f(a)` → emits `f` call with `x` prepended to args
- **Default injection**: Missing trailing args filled from DefaultNodes in the CallPlan
- **Lambdas**: Emitted as top-level C++ functions with closure allocation at the capture site
- **for loops**: Lowered to index-based iteration (`q_iter_get` with incrementing counter); loop variable uses scalar tier when the iterable's element type is statically known. `range()` calls are lowered to raw C++ loops. Vector iteration has a fast path (see §9).
- **if/while**: Conditions wrapped in `q_condition_bool()` for strict-bool enforcement
- **when**: Lowered to a chain of if/else with result payload extraction; pattern bindings use `isCaptured` to choose `QCell*` vs `QValue`

**Scope management**:
- `pushScope()` — Isolate function scope (clean slate for `declaredVars`, `cellVars`, `varTiers`)
- `pushBlockScope()` — Copy parent scope for nested blocks (allows shadowing)
- `popScope()` — Restore previous scope
- `declaredVars` — Tracks which names have been declared to prevent redeclaration
- `cellVars` — Tracks which declared names are stored as `QCell*`
- `varTiers` — Tracks which declared names use a native C++ scalar type (`"long long"`, `"double"`, `"bool"`); names absent from this map use `QValue`

**Generation order**:
1. Collect all function declarations (forward declarations)
2. Emit function definitions
3. Emit `main()` with GC initialization
4. Predeclare function bindings in main for mutual recursion
5. Initialize module bindings
6. Execute top-level statements

### 1.7 C++ Compiler → Binary

The generated C++ is compiled with clang++ (g++ is not supported due to `gc_allocator` / `std::hash` incompatibilities):
- `-std=c++17 -O3 -march=x86-64-v3` (on amd64)
- `-DQUARK_USE_GC` + Boehm GC include/link flags
- `-Wno-deprecated-declarations` on Windows
- `-lm` on non-Windows

---

## 2) Runtime Value System

### 2.1 QValue

Every runtime value is a `QValue` — a C++ tagged union:

```cpp
struct QValue {
    enum ValueType {
        VAL_INT, VAL_FLOAT, VAL_STRING, VAL_BOOL, VAL_NULL,
        VAL_LIST, VAL_VECTOR, VAL_DICT, VAL_FUNC, VAL_RESULT, VAL_RESOURCE
    } type;

    union {
        long long int_val;
        double float_val;
        char* string_val;
        bool bool_val;
        QList* list_val;
        QVector* vector_val;
        QDict* dict_val;
        void* func_val;        // Actually QClosure*
        QResult* result_val;   // {bool is_ok, QValue payload}
        QResourceHandle* resource_val;
    } data;
};
```

Value constructors use the `qv_` prefix: `qv_int(42)`, `qv_string("hello")`, `qv_null()`.

**Type guards are mandatory**: ALL runtime operations must check `QValue::type` before accessing union fields. Return `qv_null()` on type mismatch.

### 2.2 QClosure

All function values — named functions, lambdas, closures — are represented as `QClosure*`:

```cpp
struct QClosure {
    void* func;              // Function pointer (QClosure* as hidden first arg)
    int capture_count;
    QCell* captures[];       // Flexible array of captured variable cells
};
```

Every generated function signature includes a hidden first parameter `QClosure* _cl`. For direct calls to named functions, the compiler passes `nullptr`. For dynamic/closure calls, the actual closure pointer is passed.

Function pointer typedefs:
```cpp
using QClFunc0 = QValue (*)(QClosure*);
using QClFunc1 = QValue (*)(QClosure*, QValue);
// ... up to QClFunc12
```

### 2.3 QCell

`QCell` is a heap-allocated mutable reference cell used for variables that are captured by closures:

```cpp
struct QCell {
    QValue value;
};
```

**Storage selection** (decided at compile time per variable):
- A variable captured by at least one nested lambda is allocated as `QCell*`. Multiple closures over the same variable share the same cell pointer, enabling shared mutable state.
- A variable that is never captured is emitted as a stack `QValue` — no heap allocation, no GC pressure.
- Parameters and loop variables follow the same rule: `QCell*` only if captured, `QValue` otherwise.

The analyzer's `GetCapturedByFunction` output drives this decision. Codegen records which variables are cells in `cellVars` and uses that to emit correct read/write patterns.

### 2.4 Container Types

- **QList**: `std::vector<QValue, q_allocator<QValue>>` — GC-managed dynamic array
- **QDict**: `std::unordered_map<std::string, QValue, ..., q_allocator<...>>` — string-keyed hash map
- **QVector**: Typed columnar storage with four dtype variants (f64, i64, bool, str). String vectors use offset-based columnar storage internally (not pointers per element). Includes a per-element null mask.
- **QResult**: `{ bool is_ok; QValue payload; }` — tagged ok/err wrapper

### 2.5 Capture Analysis

The analyzer walks each lambda's AST to find **free variables** — identifiers that are:
- Not the lambda's own parameters
- Not defined locally within the lambda body
- Not builtins
- Resolvable in an enclosing scope

These free variables become the lambda's capture list. Nested lambdas transitively capture from the outermost scope that defines the variable.

---

## 3) Memory Management

All heap allocation goes through Boehm GC when `QUARK_USE_GC` is defined (the default). The three allocation paths:

| Path | Usage | GC behaviour |
|------|-------|-------------|
| `q_malloc(n)` / `q_new<T>(args...)` | Objects containing pointers (closures, cells, results, containers) | Scanned for pointers |
| `q_malloc_atomic(n)` / `q_strdup(s)` | Pointer-free data (strings, numeric buffers) | Not scanned |
| `q_allocator<T>` (STL allocator) | Internal buffers of `std::vector`, `std::unordered_map` | Scanned/atomic depending on element type |

There is **no manual free**. The GC reclaims unreachable objects automatically. Destructors are not called by the GC (Boehm GC behaviour), but since all sub-allocations also use the GC, no manual cleanup is needed.

`q_gc_init()` is emitted as the first statement in the generated `main()`.

---

## 4) Naming Conventions

| Prefix | Domain | Examples |
|--------|--------|---------|
| `q_` | Runtime builtins/helpers (C++ runtime code) | `q_add()`, `q_print()`, `q_malloc()` |
| `quark_` | ALL user-defined names in generated C++ | `quark_main()`, `quark_x`, `quark__lambda1` |
| `qv_` | Value constructors | `qv_int()`, `qv_string()`, `qv_null()` |
| `QValue`, `QList`, `QClosure` | Type names (PascalCase) | — |

**Never mix `q_` and `quark_` prefixes** — `q_` is runtime-only, `quark_` is codegen-only.

---

## 5) CallPlan IR

The `CallPlan` (`ir/call.go`) freezes call semantics after analysis so codegen doesn't re-derive them:

```go
type CallPlan struct {
    Kind              CallKind       // CallBuiltin | CallFunctionValue
    CalleeName        string
    MinArity          int
    MaxArity          int
    Dispatch          DispatchMode   // see below
    RuntimeSymbol     string         // C++ symbol (e.g., "q_upper", "quark_myfunc", "qei_sqrt")
    DefaultNodes      []*TreeNode    // Trailing default args to inject
    IsMethod          bool           // True for x.method(args)
    ReceiverNode      *ast.TreeNode  // The receiver expression
    ReceiverTypeKey   string         // builtins.TypeKey of receiver
    NativeParamTypes  []string       // C++ types per param (extern/native dispatch)
    NativeReturnType  string         // C++ return type (extern/native dispatch)
    NativeReceiverType string        // C++ receiver type for extern methods
}
```

**Dispatch modes:**
- `DispatchBuiltin` — Direct call to `q_*` runtime function
- `DispatchDirect` — Direct call to `quark_*` user function (nullptr closure)
- `DispatchClosure` — Dynamic call through `q_call*` (dereferences QClosure)
- `DispatchExtern` — Call to a native C++ function declared via `extern fn`. Args are unboxed from `QValue` to native C++ types (`int64_t`, `double`, `bool`, `const char*`, `QList*`, `QDict*`, `QVector*`, `QClosure*`) at the call site; the return value is boxed back. A QValue-convention thunk is also generated so extern functions can be stored as first-class values.
- `DispatchNative` — Fully-typed user function (all params + return annotated with scalar types). Codegen emits both a native C++ signature (taking `int64_t`/`double`/`bool`/`const char*`) and a QValue-convention thunk for first-class use.

---

## 6) Builtin Catalog

The shared catalog (`builtins/catalog.go`) is the single source of truth for builtin definitions:

```go
type Spec struct {
    Name         string      // e.g., "upper"
    Runtime      string      // e.g., "q_upper"
    MinArgs      int
    MaxArgs      int
    ParamTypes   []TypeKey
    ReturnType   TypeKey
    ReceiverType TypeKey     // "" = free function; non-empty = method
}
```

Methods are indexed by `(ReceiverType, methodName)` pair. Free functions are indexed by name alone.

**Adding a new builtin:**
1. Add a `Spec` entry in `builtins/catalog.go` (set `ReceiverType` for methods, leave empty for free functions)
2. Implement the C++ function in `runtime/include/quark/builtins/*.hpp` with `q_` prefix
3. Analyzer and codegen pick it up automatically via the catalog
4. Add a smoke test in `src/testfiles/smoke_*.qrk`

---

## 7) Extern Function System

The extern fn system allows Quark programs to call native C++ functions. See semantics.md §10.6 for the language-level syntax.

### Pipeline flow

1. **Parser**: Produces `ExternSourceNode` (header path) and `ExternFnNode` (function declaration with `ExternSymbol` and `ExternReceiver` fields).
2. **Loader**: Rewrites `ExternSourceNode` paths to absolute paths so codegen can emit `#include "absolute/path.hpp"` directly.
3. **Analyzer** (`analyzeExternFn`): Maps Quark type annotations to C++ types via `quarkTypeToNativeCType`. Registers free functions in `builtins` + `externFns[name]`. Registers methods in `methods[receiverKey][methodName]` + `externFns["type.method"]`. Produces a prototype `CallPlan` with `DispatchExtern`.
4. **Codegen**: At call sites, unboxes each argument via `q_as_int`, `q_as_float`, etc. Boxes the return value back to `QValue`. Also emits a QValue-convention thunk so the function can be passed as a first-class value.

### Extension author API (`ext/api.hpp`)

The `qext` namespace provides safe helpers for C++ extension authors:

| Category | Functions |
|----------|----------|
| Memory | `qext::malloc`, `qext::malloc_atomic`, `qext::strdup` |
| Boxing | `qext::box(int64_t)`, `qext::box(double)`, `qext::box(bool)`, `qext::box(const char*)`, `qext::box(QVector*)`, `qext::box(QList*)`, `qext::box(QDict*)`, `qext::null_val()` |
| Unboxing | `qext::as_int`, `qext::as_float`, `qext::as_bool`, `qext::as_str`, `qext::as_vector`, `qext::as_list`, `qext::as_dict`, `qext::as_closure` |
| Vectors | `qext::as_f64`, `qext::as_i64`, `qext::as_bool_vec` (and `_mut` variants), `qext::null_mask`, `qext::vec_size`, `qext::vec_dtype`, `qext::new_f64`, `qext::new_i64`, `qext::new_bool_vec` |
| Dict | `qext::dict_get`, `qext::dict_set`, `qext::dict_size`, `qext::dict_has` |
| Calls | `qext::call(fn, ...)` overloaded for 0-3 QValue args |
| Error | `qext::panic`, `qext::panicf` |

---

## 8) Resource System

Opaque handles for external state (currently file I/O only). See `types/resource.hpp`.

### Design

Generational slot-based registry:

- **`QResourceHandle`**: Opaque token `{slot, generation, kind}` stored in QValue's union. GC-allocated via `q_malloc_atomic`.
- **`QResourceSlot`**: Registry entry `{generation, kind, flags, payload, occupied}`. The `payload` is an opaque `void*` (e.g., `FILE*` for file handles).
- **`QResourceRegistry`**: Global singleton with a `slots` vector and a `free_list` for recycling closed slots.

### Lifecycle

1. **Create**: `q_resource_create(kind, payload, flags)` allocates a slot (reusing from free list if available), bumps the generation counter, and returns a `QResourceHandle*`.
2. **Use**: `q_resource_resolve(value, expectedKind, &slot, &handle, &err)` validates the handle against the registry. Checks: slot in range, slot occupied, generation matches, kind matches. Returns a pointer to the slot's payload on success.
3. **Close**: `q_resource_invalidate(handle)` marks the slot as unoccupied, nulls the payload, and pushes the slot index to the free list. The generation counter prevents stale handles from aliasing a recycled slot.

### Error detection

| Condition | Error |
|-----------|-------|
| Null or non-resource value | "invalid resource handle" |
| Slot index out of range | "invalid resource handle" |
| Slot not occupied | "resource is closed" |
| Generation mismatch | "stale resource handle" |
| Kind mismatch | "resource kind mismatch" |

---

## 9) Codegen Lowering Details

### Range lowering

`for i in range(n)` is detected at codegen time and emitted as a raw C++ `for` loop:
```cpp
for (long long quark_i = 0; quark_i < n; quark_i++) { ... }
```
Supports 1, 2, and 3-argument `range()` forms. Avoids allocating a list of integers.

### Vector loop fast path

When iterating a vector (`for x in vec`), codegen emits a runtime type check. If the vector's dtype matches the loop variable's scalar tier and has no null mask, iteration goes directly over the typed backing buffer:

```cpp
if (vec.type == VAL_VECTOR && valid && !has_nulls && dtype == I64) {
    const QVecI64& data = std::get<QVecI64>(vec.data.vector_val->storage);
    for (long long i = 0; i < (long long)data.size(); i++) {
        long long quark_x = static_cast<long long>(data[i]);
        // ... body ...
    }
} else {
    // generic q_iter_get path
}
```

Both branches are emitted; only one executes at runtime. Falls back to the generic path for: captured loop variables, nullable vectors, dtype mismatches, or unknown tiers.

### Vector literal lowering

When the analyzer knows the precise element type of a vector literal, codegen emits typed constructors directly (e.g., `qv_vector_i64` + `q_vec_push_i64`). Otherwise, it falls back to building a list then calling `q_to_vector` at runtime.

### Scalar lowering

For arithmetic and comparison operators, codegen first checks whether both operands are "scalar atoms" (literals or scalar-tiered local variables). If so, it emits raw C++ operators:

```cpp
// a + b where both are long long tier
qv_int(((long long)quark_a) + ((long long)quark_b))
```

This bypasses the `q_add(QValue, QValue)` path and eliminates box/unbox overhead. Division between two `int` operands promotes to `double`. Falls back to boxed `q_add` etc. when either operand is `QValue` or unknown.

---

## 10) GC Bootstrap

`ensureGC()` in `main.go` locates or builds Boehm GC:

1. **Find source**: Searches for `deps/bdwgc/` by walking candidate paths relative to the compiler executable and the current working directory. Validates by checking for `CMakeLists.txt`.
2. **Find library**: Looks for `libgc.a` (or platform equivalents like `gc.lib`, `Release/gc.lib`) in `deps/bdwgc/build/`.
3. **Auto-build**: If the library is not found and `cmake` is available, builds automatically. On Windows, forces `clang` as the C compiler (not MinGW gcc) and static build (`BUILD_SHARED_LIBS=OFF`) to avoid `gc.dll`.
4. **Return paths**: `(gcIncludePath, gcLibPath)` for `-I` and linker arguments.

Prerequisites: Go 1.21+, clang++ in PATH, CMake (for first-time GC build only).

---

## 11) Testing Infrastructure

### Integration tests (`integration_smoke_test.go`)

`TestMain` compiles the full `quark` package into a test binary once. All tests call `runQuark(t, args...)` which executes the binary and captures stdout/stderr.

### Smoke tests

`TestSmokePrograms_Run` is table-driven. Each entry maps a `.qrk` file in `src/testfiles/smoke_*.qrk` to expected stdout. The test runs `quark run <file>`, normalizes line endings, and compares output exactly.

`TestSmokePrograms_CompileError` verifies that certain programs produce expected error substrings and exit non-zero.

### Inline source tests

Several tests construct temporary `.qrk` files from Go strings for targeted verification: runtime contracts, default parameter edge cases, file I/O builtins, GC heap coverage, `any` type annotations, and extern fn type guards.

### GC selfcheck

Compiles a standalone C++ program (not a Quark program) that allocates runtime values and verifies via `GC_base()` that all object and buffer allocations land on the Boehm GC heap. Covers: strings, lists, dicts, all four vector dtypes, closures, cells, and ok/err results.
