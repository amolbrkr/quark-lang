# Quark Architecture & Implementation

This document covers the compiler pipeline, runtime internals, code generation, and memory management. It complements **semantics.md** (language behaviour), **grammar.md** (syntax), and **stdlib.md** (builtins).

---

## 1) Compilation Pipeline

The pipeline has seven stages. Each stage's output feeds the next.

```
.qrk source → Lexer → Parser → Loader → Analyzer → Invariants → Codegen → C++ → clang++ → binary
```

The front end (lexer through codegen) runs in `driver/`: `driver.CompileFile(path, stage)` runs every stage up to a cutoff (`StageParse`, `StageCheck`, `StageEmit`) and returns a `Unit` with the AST, the analyzer's `types.Analysis`, the C++ and all diagnostics. The `parse`, `check`, `emit`, `build` and `run` commands go through it; `quark lex` calls the lexer directly. `internal/testutil` runs its own lex/parse/analyze steps on in-memory source and reuses the driver's `ResolveImports`, `Validate` and `Generate`. The native back end (clang++, runtime headers, GC) lives in `toolchain/`.

### 1.1 Lexer → Token Stream

Three-pass tokenization:
1. **Raw tokenization**: Produces tokens from source characters. Supports single-quoted and double-quoted strings with escapes (`\n`, `\t`, `\r`, `\\`, `\'`, `\"`; `\0` becomes an `ILLEGAL` token because strings are NUL-terminated). Line comments with `//`.
2. **Line-start tracking**: Marks which tokens begin a new line.
3. **Indentation injection**: Converts leading whitespace after `:` or `->` + newline into `INDENT`/`DEDENT` tokens. Tracks an indent stack; mismatched dedent levels produce `ILLEGAL` tokens.

Inside brackets (`()`, `[]`, `{}`), indentation processing is suppressed — newlines are ignored and no INDENT/DEDENT is emitted.

### 1.2 Parser → AST

Recursive-descent parser producing a tree of `TreeNode`s. Uses Pratt parsing for expressions with 12 precedence levels. Named function definitions are immediately desugared to assignments (see semantics.md §6.4).

Key parser decisions:
- `fn name(...)` at statement level → named function (desugared to assignment)
- `fn(...)` in expression → lambda
- `list [...]` → list literal (keyword required, avoids ambiguity with indexing)
- Dict keys in literals are bare identifiers, interpreted as string keys

### 1.3 Loader → Merged AST

Resolves all `use` statements by loading, parsing, and splicing external files into the main AST. See semantics.md §10 for resolution rules.

After splicing, `namespaceModules` (`loader/namespace.go`) gives module members unique names, since all files share one scope: each definition at the top level of module `m` and every reference to it becomes `m__name` (`loader.MemberName`). That covers sibling references inside the module, the member side of `alias.member` (kept as a dot so the analyzer resolves it through the module's symbols), and bare names imported by an unaliased `use`. Parameters, loop variables and ok/err bindings shadow as usual. The analyzer shows the original name in messages (`displayMember`). The loader also tags nodes from imported files with `SourceFile`, which codegen's `q_set_source_loc` calls use so runtime errors name the module's file (code from `std/` sets no location, so its errors point at the user's call). Every generated function starts with a `QSourceLocGuard` that restores the caller's location when it returns.

### 1.4 Analyzer → Annotated AST + Metadata

Two-pass semantic analysis:
1. **Predeclaration pass**: Scans top-level statements to register all named functions and function-binding assignments. This enables forward references — a function can call another function defined later in the file.
2. **Full analysis pass**: Walks the entire AST, building scopes, type-checking expressions, computing closure captures, and generating CallPlans.

The analyzer returns one `types.Analysis` struct (`types/analyzer_modules.go`) with eight outputs, consumed by the invariant checks and by codegen (via `SetAnalysis`):
- **CallPlans**: Per-call-site metadata (dispatch mode, arity, defaults to inject)
- **Captures**: Per-lambda map of captured variable names
- **ReturnValidation**: Per-function declared-vs-inferred return type checks
- **CapturedByFunction**: Inverted capture map — per-function/lambda, the set of variable names that any directly-nested lambda captures from it. Used by codegen to decide `QCell*` vs scalar storage per variable.
- **NodeTypes**: Per-AST-node inferred type (`map[*ast.TreeNode]Type`). Every expression node has its analyzer-computed type recorded here. Codegen reads this to drive scalar storage selection and native operator lowering without re-deriving type information.
- **NativeFns**: Prototype CallPlans for fully annotated user functions (`DispatchNative`)
- **ExternFns**: Prototype CallPlans for `extern fn` declarations (`DispatchExtern`), keyed by name or `"type.method"`
- **StructTypes**: Declared struct types and their fields

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
- Exceptions: `extern fn` symbols are called by their own C++ name (e.g. `qei_sqrt`), and their first-class thunks are named `_thunk_<symbol>`. Native (fully annotated) functions get a `quark_<name>__thunk`.

**Variables**: Storage is chosen per variable at first declaration using two criteria: (1) whether a nested lambda captures it, and (2) the static type from `NodeTypes`. Five storage tiers:

| Tier | C++ type | Condition |
|------|----------|-----------|
| `QCell*` | heap reference cell | captured by any nested lambda; `->value` for reads/writes |
| `long long` | native int | not captured + static type `int` + no `any` annotation |
| `double` | native float | not captured + static type `float` + no `any` annotation |
| `bool` | native bool | not captured + static type `bool` + no `any` annotation |
| `QValue` | tagged union | not captured + unknown/any/composite type, or `any`-annotated |

Closure captures received from an enclosing scope (extracted from `_cl->captures[i]`) are always `QCell*` — the cell is shared with the outer scope regardless of type.

`generateIdentifier` boxes scalar vars back to `QValue` at read sites (e.g., `qv_int(quark_x)`) so the rest of codegen always operates on `QValue` expressions. The exception is operator lowering (below): an expression made only of scalars is emitted unboxed and assigned directly to a scalar-tier variable (`long long quark_c = q_checked_add(quark_a, quark_b);`).

Parameters follow the same capture rule: a parameter of an ordinary Quark function is `QCell*` if captured, otherwise `QValue`. A native (fully annotated) function receives scalar parameters as `int64_t`/`double`/`bool` and keeps them at that tier unless captured. Loop variables use a scalar tier when the element type is statically known; `range()` loop variables are always `long long`.

Codegen tracks storage choices in `cellVars` (names stored as `QCell*`) and `varTiers` (names stored as a named C++ scalar type), both scoped and stacked with the scope stack.

**Function calls**: Lowered according to the CallPlan's dispatch mode:
- `DispatchBuiltin` → `q_print(arg)` (direct C++ call)
- `DispatchDirect` → `quark_foo(nullptr, arg)` (immutable function symbol, no closure). Named functions are desugared to mutable bindings, so in practice ordinary calls to them use `DispatchClosure`.
- `DispatchClosure` → `q_call1(quark_foo, arg)` (dynamic dispatch through QClosure) — the usual path for named functions and lambdas
- `DispatchExtern` → `qei_sqrt(q_as_float(arg))` (unbox args, call native, box return)
- `DispatchNative` → `quark_foo(nullptr, raw_arg)` (fully annotated function with native C++ types, with a QValue thunk for first-class use)
- `DispatchDynamicMethod` → `([&]() -> QValue { switch (recv.type) { case QValue::VAL_STRING: return q_split(recv, arg); ... default: q_no_method(recv, "split"); } }())` (builtin method on a receiver whose type is unknown at compile time, e.g. the result of `unwrap`; an immediately-invoked lambda so the call stays a lazily evaluated expression)

If codegen encounters an unexpected dispatch/runtime-symbol invariant break, it terminates with an internal compiler error (`INV-*`) rather than emitting fallback runtime behavior.

**Method calls**: When `IsMethod=true`, the receiver is injected as the first runtime argument (e.g., `"hello".upper()` → `q_upper(receiver_val)`).

**Operator lowering**: `scalarExpr()` tries to emit arithmetic (`+`, `-`, `*`, `/`, `%`), unary `-`, and comparisons (`<`, `<=`, `>`, `>=`, `==`, `!=`) as native C++ when every operand is a scalar: a literal, a scalar-tiered local, or (recursively) another lowered expression. Int arithmetic uses overflow-checked helpers (`q_checked_add/sub/mul/mod`, `q_checked_neg`), and `/` uses `q_checked_fdiv`, which always yields `double`; float `+ - *` and comparisons use raw C++ operators. `**` is never lowered (it calls `q_pow`). Falls back to the boxed `q_add` etc. path whenever an operand is `QValue`/unknown. See §9.

**Other lowerings**:
- **Pipe**: `x | f(a)` → emits `f` call with `x` prepended to args
- **Default injection**: Missing trailing args filled from DefaultNodes in the CallPlan
- **Lambdas**: Emitted as top-level C++ functions with closure allocation at the capture site
- **for loops**: Lowered to index-based iteration (`q_iter_get` with incrementing counter); loop variable uses scalar tier when the iterable's element type is statically known. `range()` calls are lowered to raw C++ loops unless the loop variable is captured. Vector iteration has a fast path (see §9).
- **if/while**: Conditions are wrapped in `q_truthy()` (truthiness rules; `result` and `vector` conditions are rejected), or emitted as a raw C++ `bool` when `scalarExpr` lowers the condition
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

`toolchain.Find()` locates clang++, the runtime headers and the GC once. The build then runs in two steps, so additional native objects and libraries can join the link:

1. **Compile** (`Toolchain.Compile`): C++ → object file with clang++ (g++ is not supported due to `gc_allocator` / `std::hash` incompatibilities):
    - `-std=c++17 -O3 -march=x86-64-v3` (on amd64)
    - `-DQUARK_USE_GC` + Boehm GC include flags
    - `-Wno-deprecated-declarations` on Windows
    - `-include-pch <cached quark.hpp PCH>` unless `--no-pch` (see below)
2. **Link** (`Toolchain.Link`): objects + native libraries from `extern link` + static `libgc` (`-pthread`, `-ldl` on Linux) + `-lm` on non-Windows. With `--lto`, links with `lld` when `ld.lld` is on PATH, since the system linker needs LLVM's gold plugin for LTO.

**Native dependencies** (`toolchain/native.go`). The loader collects `extern link` and `extern include_path` directives from every loaded file into `driver.Unit.Native` (`loader.NativeDeps`: include directories, libraries in link order, and the extern glue headers). `main.go` combines them with `--lib-path` / `--include-path` / `--link-arg` and `QUARK_LIB_PATH` / `QUARK_INCLUDE_PATH` into `toolchain.Options`. Before linking, `Link` looks for every library in the user directories, then `LIBRARY_PATH`, then clang's own search path (`clang++ -print-search-dirs`), and reports each missing one as `QK-FFI-002`. The link line gets `-L` for each user directory, `-Wl,-rpath,<dir>` for user directories that provide a shared library, then `-l<name>` in link order, then `--link-arg` values. Extra include directories are added to the compile but not to the precompiled header, which stays shared between programs. When a compile fails and clang's output has an error located in an extern glue header (or in a file one includes), `Compile` returns a `QK-FFI-003` diagnostic and `quark run` skips the generated-C++ dump.

Intermediate files go in a fresh `os.MkdirTemp` directory per build, so concurrent builds never collide.

**Precompiled runtime header.** The runtime is header-only, so every compile parses ~70k lines of runtime headers before reaching user code. `toolchain/pch.go` precompiles `quark/quark.hpp` into `$XDG_CACHE_HOME/quark/pch/<key>.pch` (`os.UserCacheDir()`), where the key hashes the compiler binary, the compile flags, and the path, size and mtime of every runtime and GC header. A PCH clang rejects (for example after a system header update) is deleted and the compile retried without it. This roughly halves compile time for small programs.

---

## 2) Runtime Value System

### 2.1 QValue

Every runtime value is a `QValue` — a C++ tagged union:

```cpp
struct QValue {
    enum ValueType {
        VAL_INT, VAL_FLOAT, VAL_STRING, VAL_BOOL, VAL_NULL,
        VAL_LIST, VAL_VECTOR, VAL_DICT, VAL_FUNC, VAL_RESULT, VAL_RESOURCE,
        VAL_STRUCT, VAL_TABLE
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
        QStruct* struct_val;
        QTable* table_val;     // runtime only; `table` syntax is not implemented yet
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

Every generated function signature includes a hidden first parameter `QClosure* _cl`. Calls through a closure value (`q_callN`, the usual path for named functions and lambdas) pass the actual closure pointer. Direct calls that bypass the closure — `DispatchNative`, `DispatchDirect`, and the call inside a native function's thunk — pass `nullptr`.

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

**Storage selection** (decided at compile time per variable; full rules in §1.6):
- A variable captured by at least one nested lambda is allocated as `QCell*`. Multiple closures over the same variable share the same cell pointer, enabling shared mutable state.
- A variable that is never captured lives on the stack — as a native scalar (`long long`, `double`, `bool`) when its static type allows, otherwise as a `QValue`. No heap allocation, no GC pressure.

`Analysis.CapturedByFunction` drives the cell decision. Codegen records which variables are cells in `cellVars` and uses that to emit correct read/write patterns.

### 2.4 Container Types

- **QList**: `std::vector<QValue, q_allocator<QValue>>` — GC-managed dynamic array
- **QDict**: `std::unordered_map<std::string, QValue, ..., q_allocator<...>>` — string-keyed hash map
- **QVector**: Typed columnar storage with four dtype variants (f64, i64, bool, str). Buffers follow the Arrow layout: string vectors use offsets plus UTF-8 bytes (not pointers per element), and nulls are tracked in an Arrow validity bitmap that is `nullptr` when the vector has no nulls (`types/vector.hpp`).
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
| `quark_` | User-defined names in generated C++ (except `extern fn` symbols and their `_thunk_` wrappers, see §1.6) | `quark_main()`, `quark_x`, `quark__lambda1` |
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
    ArgTypesChecked   bool           // Analyzer already checked argument types
    DefaultNodes      []*TreeNode    // Trailing default args to inject
    IsMethod          bool           // True for x.method(args)
    ReceiverNode      *ast.TreeNode  // The receiver expression
    ReceiverTypeKey   string         // builtins.TypeKey of receiver
    MethodCandidates  []MethodCandidate // receiver types for DispatchDynamicMethod (value tag, type key, runtime symbol)
    NativeParamTypes  []string       // C++ types per param (extern/native dispatch)
    NativeReturnType  string         // C++ return type (extern/native dispatch)
    NativeReceiverType string        // C++ receiver type for extern methods
}
```

**Dispatch modes:**
- `DispatchBuiltin` — Direct call to `q_*` runtime function
- `DispatchDirect` — Direct call to `quark_*` user function (nullptr closure); only chosen for immutable function symbols, so ordinary named functions use `DispatchClosure`
- `DispatchClosure` — Dynamic call through `q_call*` (dereferences QClosure)
- `DispatchExtern` — Call to a native C++ function declared via `extern fn`. Args are unboxed from `QValue` to native C++ types (`int64_t`, `double`, `bool`, `const char*`, `QList*`, `QDict*`, `QVector*`, `QClosure*`) at the call site; the return value is boxed back. A QValue-convention thunk is also generated so extern functions can be stored as first-class values.
- `DispatchNative` — Fully-typed user function (all params + return annotated with scalar types). Codegen emits both a native C++ signature (taking `int64_t`/`double`/`bool`/`const char*`) and a QValue-convention thunk for first-class use.
- `DispatchDynamicMethod` — Builtin method call on a receiver of unknown type (`any`). The analyzer (`resolveDynamicMethodCall`) collects every builtin receiver type (str, list, dict, vector) that has the method and accepts the argument count; codegen switches on the receiver's type tag, and any other type is a runtime error (`q_no_method`). Argument types are checked at compile time only when a single receiver type matches. If no type has the method, or none accepts the argument count, it is a compile-time error. The `INV-DYNAMIC-METHOD` invariant checks each candidate against the catalog.

---

## 6) Builtin Catalog

The shared catalog (`builtins/catalog.go`) is the single source of truth for builtin definitions. Its consumers are the analyzer (`types/`) and the invariant checks (`invariants/callplan.go`); codegen doesn't read it and emits `CallPlan.RuntimeSymbol` instead. The editor extension's completion data is also generated from it (`internal/editordata`).

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
2. Implement the C++ function in `runtime/include/quark/builtins/*.hpp` with `q_` prefix. A new header must also be added to the include list in `runtime/include/quark/quark.hpp`
3. Analyzer and codegen pick it up automatically via the catalog
4. Add a smoke test in `src/testfiles/smoke_*.qrk`
5. Regenerate the editor extension's data: `go test ./internal/editordata -update`, and add the name to the grammar's `builtin-functions` pattern (the test says so if it's missing)

---

## 7) Extern Function System

The extern fn system allows Quark programs to call native C++ functions. See semantics.md §10.6 for the language-level syntax.

### Pipeline flow

1. **Parser**: Produces `ExternHeaderNode` (header path), `ExternLinkNode`, `ExternIncludePathNode`, and `ExternFnNode` (function declaration with `ExternSymbol` and `ExternReceiver` fields).
2. **Loader**: Rewrites `ExternHeaderNode` paths to absolute paths so codegen can emit `#include "absolute/path.hpp"` directly, and collects link and include-path directives into `NativeDeps` (see §1.7).
3. **Analyzer** (`analyzeExternFn`): Maps Quark type annotations to C++ types via `quarkTypeToNativeCType`. Registers free functions in `builtins` + `externFns[name]`. Registers methods in `methods[receiverKey][methodName]` + `externFns["type.method"]`. Produces a prototype `CallPlan` with `DispatchExtern`.
4. **Codegen**: At call sites, unboxes each argument via `q_as_int`, `q_as_float`, etc. Boxes the return value back to `QValue`. Also emits a QValue-convention thunk so the function can be passed as a first-class value.

### Extension author API (`ext/api.hpp`)

The `qext` namespace provides safe helpers for C++ extension authors:

| Category | Functions |
|----------|----------|
| Memory | `qext::malloc`, `qext::malloc_atomic`, `qext::strdup` |
| Boxing | `qext::box(int64_t)`, `qext::box(double)`, `qext::box(bool)`, `qext::box(const char*)`, `qext::box(QVector*)`, `qext::box(QList*)`, `qext::box(QDict*)`, `qext::null_val()` |
| Unboxing | `qext::as_int`, `qext::as_float`, `qext::as_bool`, `qext::as_str`, `qext::as_vector`, `qext::as_list`, `qext::as_dict`, `qext::as_closure` |
| Vectors | `qext::as_f64`, `qext::as_i64` (and `_mut` variants for freshly created vectors), `qext::bool_at`, `qext::set_bool`, `qext::str_at`, `qext::is_null_at`, `qext::set_null`, `qext::validity_bitmap`, `qext::vec_size`, `qext::vec_null_count`, `qext::vec_dtype`, `qext::new_f64`, `qext::new_i64`, `qext::new_bool_vec` |
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
2. **Use**: `q_resource_resolve(value, expectedKind, &slot, &handle, &err)` validates the handle against the registry. Checks: slot in range, slot occupied, generation matches, kind matches. Returns `true` on success and passes the slot and handle out through the reference parameters (the payload is `slot->payload`); on failure it returns `false` and sets `err`.
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

`for i in range(...)` is detected at codegen time and emitted as a raw C++ `for` loop over start/end/step temporaries:
```cpp
long long _t1 = 0;      // start
long long _t2 = 10;     // end
long long _t3 = 1;      // step
for (long long quark_i = _t1; _t3 > 0 ? (quark_i < _t2) : (quark_i > _t2); quark_i += _t3) { ... }
```
Supports 1, 2, and 3-argument `range()` forms; the 2-argument form picks step `1` or `-1` from the direction of start and end. Avoids allocating a list of integers. Skipped when the loop variable is captured by a lambda (the generic path is used instead).

Arguments follow the same rules as the `range()` builtin: scalar ints are used directly, scalar floats are truncated, and any other value goes through `q_range_arg(v, "range(end)", "end")`, which truncates floats and reports a non-numeric value. A 3-argument step goes through `q_range_step`, so a step of `0` is a runtime error, as with `range()`.

### Vector loop fast path

When iterating a vector (`for x in vec`), codegen emits a runtime type check. If the vector's dtype matches the loop variable's scalar tier and it has no nulls, iteration goes directly over the typed backing buffer:

```cpp
if (q_vec_is_dense_of(_t8, QVector::Type::I64)) {
    const QVector& _t9 = *_t8.data.vector_val;
    for (long long _t10 = 0; _t10 < (long long)_t9.count; _t10++) {
        long long quark_x = static_cast<long long>(q_vec_i64_data(_t9)[_t10]);
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

`scalarExpr()` lowers arithmetic, unary `-` and comparisons when every operand is a scalar: a literal, a scalar-tiered local, or a sub-expression that itself lowers. Int arithmetic keeps the boxed operators' failure semantics (overflow, division and modulo by zero are fatal) through checked helpers:

```cpp
// c = a + b, d = a / b, f = (a + b) * 2 > 5   (a, b are long long tier)
long long quark_c = q_checked_add(quark_a, quark_b);
double quark_d = q_checked_fdiv((double)quark_a, (double)quark_b);
bool quark_f = (q_checked_mul(q_checked_add(quark_a, quark_b), 2) > 5);
```

Float `+ - *` and comparisons use plain C++ operators (`((double)x + (double)y)`). `/` always produces `double`. `**` is not lowered and goes through `q_pow`. The result stays unboxed when it's assigned to a scalar-tier variable or used as a condition, and is boxed (`qv_int(...)`) otherwise. Falls back to boxed `q_add` etc. when any operand is `QValue` or unknown.

---

## 10) GC Bootstrap

`toolchain.EnsureGC()` locates or builds Boehm GC. The repository commits only the GC's public headers and prebuilt static libraries, not its source (see `deps/bdwgc/VENDORED.md`):

1. **Find the GC directory**: Searches for `deps/bdwgc/` by walking candidate paths relative to the compiler executable and the current working directory. Validates by checking for `include/gc/gc.h`.
2. **Prebuilt library**: Uses `deps/bdwgc/lib/<GOOS>-<GOARCH>/libgc.a` (`gc.lib` on Windows) when the repository has one for the host. Prebuilt libraries exist for `linux-amd64`, `linux-arm64`, `darwin-arm64` and `windows-amd64`; `deps/bdwgc/build-prebuilt.sh` rebuilds them.
3. **Local build**: Otherwise looks for a static library in `deps/bdwgc/build-static/` (`libgc.a`, or `gc.lib` / `Release/gc.lib` on Windows). Shared libraries are never used, so generated executables run standalone without `libgc` on the loader path.
4. **Auto-build**: If neither exists, the GC source is present (`CMakeLists.txt`) and `cmake` is available, builds it into `build-static/` with `GC_BUILD_SHARED_LIBS=OFF`. On Windows, it also forces `clang` as the C compiler (not MinGW gcc) to match the MSVC ABI. Without the source, it fails with an error pointing to `VENDORED.md`.
5. **Return paths**: `(gcIncludePath, gcLibPath)` for `-I` and linker arguments. Static `libgc` needs the threads library, so the link line adds `-pthread` on Unix and `-ldl` on Linux.

Prerequisites: Go 1.21+, clang++ in PATH. CMake only on platforms without a prebuilt GC library.

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
