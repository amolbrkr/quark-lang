# Quark Architecture & Implementation

This document covers the compiler pipeline, runtime internals, code generation, and memory management. It complements **semantics.md** (language behaviour), **grammar.md** (syntax), and **stdlib.md** (builtins).

---

## 1) Compilation Pipeline

The pipeline has seven stages. Each stage's output feeds the next.

```
.qrk source → Lexer → Parser → Loader → Analyzer → Invariants → Codegen → C++ → clang++ → binary
```

The front end (lexer through codegen) runs in `driver/`: `driver.CompileFile(path, stage)` runs every stage up to a cutoff (`StageParse`, `StageCheck`, `StageEmit`) and returns a `Unit` with the AST, the analyzer's `types.Analysis`, the C++ and all diagnostics. The `parse`, `check`, `emit`, `build` and `run` commands go through it; `quark lex` calls the lexer directly. `internal/testutil` runs in-memory test sources through `driver.CompileSource` too (only its `Lex` and `Parse` helpers stop before the loader), so tests see the same stage order, early stops and diagnostics as the CLI. The native back end (clang++, runtime headers, GC) lives in `toolchain/`.

### 1.1 Lexer → Token Stream

Three-pass tokenization:
1. **Raw tokenization**: Produces tokens from source characters. Supports single-quoted and double-quoted strings with escapes (`\n`, `\t`, `\r`, `\\`, `\'`, `\"`; `\0` becomes an `ILLEGAL` token because strings are NUL-terminated). Line comments with `//`.
2. **Line-start tracking**: Marks which tokens begin a new line.
3. **Indentation injection**: Converts leading whitespace after `:` or `->` + newline into `INDENT`/`DEDENT` tokens. Tracks an indent stack; mismatched dedent levels produce `ILLEGAL` tokens.

Inside brackets (`()`, `[]`, `{}`), indentation processing is suppressed — newlines are ignored and no INDENT/DEDENT is emitted.

### 1.2 Parser → AST

Recursive-descent parser producing a tree of `TreeNode`s. Uses Pratt parsing for expressions with 12 precedence levels. Each construct has its own node kind: `AssignNode` (`target = value`, the target being an identifier, `MemberNode` or `IndexNode`), `MemberNode` (`object.name`), `UnaryNode`, `BinaryNode`, `PairNode` (a dict literal entry) and `FnDeclNode` (`fn name(...)`: `[name, LambdaNode]`).

Key parser decisions:
- `fn name(...)` at statement level → `FnDeclNode`, a fixed, hoisted declaration (`ast.IsFnDecl`)
- `fn(...)` in expression → lambda
- `x | f(a)` → the call `f(x, a)` marked `Piped` (`x | obj.m(a)` → `obj.m(x, a)`); no later stage has a pipe path, and `Piped` only adds "(including piped input)" to arity messages. A pipe target that isn't a call is a parse error.
- `list [...]` → list literal (keyword required, avoids ambiguity with indexing)
- Dict keys in literals are bare identifiers, interpreted as string keys

### 1.3 Loader → Merged AST

Resolves all `incl` statements by loading, parsing, and splicing external files into the main AST. See semantics.md §10 for resolution rules.

Names are resolved per file (modules v0.2, `semantics.md` §10). `loader/files.go` loads each imported file once (keyed by absolute path, numbered in load order), checks file names and that imported files only contain definitions, and builds each file's scope: top-level definitions, modules with their members, and import bindings, reporting clashes. `loader/resolve.go` then walks every file: each definition of an imported file gets the internal name `q<N>_<stem>__name` (module members `q<N>_<stem>__<module>__name`; main-script module members `<module>__name`), and every reference, bare (local, then enclosing module, then file, then prelude) or `::`-qualified, is rewritten to that internal name. Constants of imported files are inlined. Parameters, loop variables and ok/err bindings shadow as usual. The files are then spliced into one tree, imported files first, without their `incl` statements. The resulting `names.Table` maps internal names back to the names users write; the analyzer's diagnostics and codegen's runtime names go through it. The loader also tags nodes from imported files with `SourceFile`, which codegen's `q_set_source_loc` calls use so runtime errors name the module's file (code from `std/` sets no location, so its errors point at the user's call). Every generated function starts with a `QSourceLocGuard` that restores the caller's location when it returns.

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
- **Default injection**: Missing trailing args filled from DefaultNodes in the CallPlan
- **Lambdas**: Emitted as top-level C++ functions with closure allocation at the capture site
- **for loops**: Lowered to index-based iteration (`q_iter_get` with incrementing counter); loop variable uses scalar tier when the iterable's element type is statically known. `range()` calls are lowered to raw C++ loops unless the loop variable is captured. Vector iteration has a fast path (see §9).
- **if/while**: Conditions are wrapped in `q_truthy()` (truthiness rules; `result` and `vec` conditions are rejected), or emitted as a raw C++ `bool` when `scalarExpr` lowers the condition
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
        QTable* table_val;     // runtime only; `tbl` syntax is not implemented yet
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
    CalleeName        string
    MinArity          int
    MaxArity          int
    Dispatch          DispatchMode   // see below
    RuntimeSymbol     string         // C++ symbol (e.g., "q_upper", "quark_myfunc", "qei_sqrt")
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
- `DispatchDynamicMethod` — Builtin method call on a receiver of unknown type (`any`). The analyzer (`resolveDynamicMethodCall`) collects every builtin receiver type (str, list, dict, vec) that has the method and accepts the argument count; codegen switches on the receiver's type tag, and any other type is a runtime error (`q_no_method`). Argument types are checked at compile time only when a single receiver type matches. If no type has the method, or none accepts the argument count, it is a compile-time error. The `INV-DYNAMIC-METHOD` invariant checks each candidate against the catalog.

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

The extern fn system allows Quark programs to call native C++ functions. See semantics.md §10.8 for the language-level syntax.

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

When iterating a vector (`for x in v`), codegen emits a runtime type check. If the vector's dtype matches the loop variable's scalar tier and it has no nulls, iteration goes directly over the typed backing buffer:

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

When the analyzer knows the precise element type of a vector literal, codegen emits typed constructors directly (e.g., `qv_vector_i64` + `q_vec_push_i64`). Otherwise, it falls back to building a list then calling `q_to_vec` at runtime.

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

---

## 12) Code Health and Simplification Plan

A health review on 2026-11-12 (at `b52660f` plus the uncommitted modules v0.2 work) measured the Go compiler and the C++ runtime and read each stage for duplicated or conflicting logic. Line numbers are as of that review and will drift. The bugs it found are in `changelog.md` under Known issues. This section is about structure: where the code is too complex, where one decision is made in several places, and what is dead. Most of the known issues trace back to a decision that lives in more than one place.

Tools: `gocyclo` (cyclomatic complexity, non-test Go), a function-length count with `go/ast`, and `golang.org/x/tools/cmd/deadcode`. Build, `go vet` and the full test suite were green; the integration package takes about 4 minutes uncached.

### 12.1 Measurements

A cyclomatic complexity above about 15-20 means a function has more paths than its tests can cover. `analyzeOperator` is more than twice the next one.

| Function | Location | Lines | Complexity |
|---|---|---|---|
| `Analyzer.analyzeOperator` | `types/analyzer_expressions.go:45-399` | 355 | 159 |
| `ModuleLoader.walk` | `loader/resolve.go:75-238` | 164 | 64 |
| `Generator.generateOperator` | `codegen/codegen.go:1683-1883` | 201 | 60 |
| `Lexer.nextRawToken` | `lexer/lexer.go:255-416` | 162 | 48 |
| `ValidateCallPlans` | `invariants/callplan.go:43-154` | 112 | 48 |
| `Analyzer.inferBuiltinReturnType` | `types/analyzer_typecheck.go:8-94` | 87 | 40 |
| `CanAssign` | `types/types.go:335` | | 37 |
| `Analyzer.analyzeFunctionCall` | `types/analyzer_calls.go:252-381` | 130 | 37 |
| `Analyzer.checkDefiniteAssignment` | `types/analyzer_fixed.go:298-402` | 105 | 34 |
| `ModuleLoader.collect` | `loader/files.go:536-626` | 91 | 34 |
| `Generator.scalarExpr` | `codegen/codegen.go:1496-1575` | 80 | 34 |
| `Generator.checkedFieldValue` | `codegen/codegen.go:2883` | | 32 |
| `Generator.generateFunctionCall` | `codegen/codegen.go:1944-2056` | 113 | 31 |
| `Analyzer.analyzePipe` | `types/analyzer_calls.go:433-554` | 122 | 30 |
| `Generator.generatePipe` | `codegen/codegen.go:2071-2180` | 110 | 29 |
| `Generator.Generate` | `codegen/codegen.go:537-660` | 124 | 29 |
| `Generator.generateFor` | `codegen/codegen.go:2433-2556` | 124 | |
| `Generator.generateNativeFunction` | `codegen/codegen.go:1096-1203` | 108 | 22 |
| `Analyzer.analyzeExternFn` | `types/analyzer_modules.go:143-250` | 108 | |
| `Lexer.indentationFilter` | `lexer/lexer.go:150-253` | 104 | |
| `Generator.generateWhen` | `codegen/codegen.go:2249-2349` | 101 | 22 |
| `Analyzer.resolveMethodCall` | `types/analyzer_calls.go:54-154` | 101 | |

File sizes: `codegen/codegen.go` is 2996 lines in one file; the next largest non-test files are `parser/parser.go` (1100), `loader/files.go` (756), `types/analyzer_calls.go` (749). In the runtime, `types/vector.hpp` is 1252 lines, `ext/api.hpp` 458.

`analyzeOperator` handles member access, every kind of assignment (to a name, a struct field, a dict key, a list index, a fixed `fn`, a builtin) and all unary, binary, vector and comparison operators in one function. It should be split by node shape: `analyzeMemberAccess`, `analyzeAssignment` (dispatching to one function per target kind), `analyzeUnary`, `analyzeVectorArith`, `analyzeBinary`.

### 12.2 One decision, several places (Go)

Each of these is a decision made in more than one place, with copies that already disagree. Fixing each one in one place also fixes the known issues listed next to it.

- **Variable storage tier** (`QCell*` / `long long` / `double` / `bool` / `QValue`) is chosen in at least six places in codegen: the assignment branch of `generateOperator` (~1857-1876), `generateVarDecl` (~2716-2736), parameter bindings (~1048-1057), native parameters (~1143-1155), the `for` loop variable (three times inside `generateFor`), and `hoistScopes`. `generateVarDecl` picks the tier from the value instead of the annotation, and reassignment unboxes with the variable's tier whatever the value's type. Known issues: `x: float = len(l)` stays an int, an int assigned to a float variable reads garbage. Fix: one `declareVar(name, declaredType, rhs)` that is the only tier decision.
- **Raw union reads.** `unboxForTier` (`codegen.go:264`) and the loop element read (`codegen.go:2486`) emit `.data.int_val` / `.data.float_val` with no tag check whenever the analyzer reports a scalar type, including element types of lists and dicts and the type of a narrowed `any` variable, which the analyzer does not keep sound (see 12.6). `unboxForTier` also ignores its type argument.
- *Done in step 1:* calls are built once (pipes are parsed into calls), the arity messages come from one `checkArity`, `generateLiteral` uses `escapeCppString`, `nativeArgToQValue`/`wrapExternReturn` are one `boxNative` and `unboxToNative` is gone, `topDef`/`memberDef` share `defOf`, and the target of `x.f = v` and a receiver whose method isn't found are analyzed once.
- **Calls are built twice in the analyzer and twice in codegen.** `analyzeFunctionCall` and `analyzePipe` build the builtin, extern and user-function plans and arity messages separately; so do `generateFunctionCall` and `generatePipe`. They have drifted: `3 | d.f()` falls through to calling the dict member while `d.f(3)` is "type has no method", and codegen differs in native-argument indexing and zero-argument direct calls. Fix: one `buildCallPlan(callee, prefixArgs, args)` in the analyzer and one `emitCall(plan, callee, prefixArgs, args)` in codegen, both used by call and pipe. The arity error text is also copied in about ten places in the analyzer.
- **Loop bodies skip `generateBlock`.** The range loop (~2416-2424), `emitLoopBody` (~2444-2454) and `while` (~2576-2584) emit their statements directly, so loop bodies get no `fn` hoisting, no per-statement source location, and `while` gets no block scope. Known issues: `fn` inside a loop fails in clang, a variable first assigned in a `while` body fails in clang, runtime errors in a `for` body report the `for` line. Fix: every body goes through one block helper.
- **Expression preludes.** Method-receiver temporaries (~1971-1981, ~2100-2110) and `if`/list temporaries are written to the output at the current position, i.e. before the statement, not inside the expression that needs them. Known issues: `and`/`or` and `x if c else y` run both sides, a `while` condition is evaluated once, `elseif` with a method call fails in clang, `f().m(g())` runs `g` first. Fix: expression generation returns `(prelude, expr)` and each conditional or repeated context (short-circuit, ternary, `while` condition, `elseif`) places the prelude itself, or wraps it in a lambda.
- **Function values** are built in three places: `directFnValue`, `generateIdentifier` (~1458-1463) and `generateLambdaExpr` (~2773-2776).
- **Native ↔ `QValue` conversion tables**: `nativeArgToQValue` (`codegen.go:217`) and `wrapExternReturn` (`codegen.go:511`) are the same table; `unboxToNative` is an alias of `adaptArgForExtern`.
- **String escaping**: `generateLiteral` (~1416-1425) re-implements `escapeCppString` and escapes more characters than it, so the two can produce different C++ for the same string.
- **Internal errors**: `panicICEf` calls `os.Exit(2)` from a library package, `panicMissingCallPlan` calls `panic`. Pick one (return an error, or panic and recover in the driver).
- **Constant rules**: three functions decide what a constant is: `isConstantDefault` (`parser.go:325`, allows `null` and `-'s'`), `isLiteralDefault` (`parser.go:509`) and `isConstantValue` (`loader/files.go:450`).
- **Loader definitions**: `topDef` and `memberDef` (`loader/files.go:641-722`) are near copies; `collect` now reports a nested `ModuleNode` in the main script (it used to be dropped silently, because `memberDef` ignores it). `collect` contains a hand-written insertion sort.
- **Parser statement loop**: `parseBlock` (`parser.go:164-217`) repeats the statement loop of `Parse`.
- **Duplicate analysis**: the target of `x.f = v` is analyzed twice (`analyzer_expressions.go:125` and `:168`), and the method-not-found path (`analyzer_calls.go:273`) and the pipe fallthrough (`:469`) re-analyze the receiver, so diagnostics, fixed-fn calls and call shapes can be recorded twice.
- **Name mangling**: module members are joined as `module + "__" + name`, which is not unique when a name starts or ends with `_` (`m` + `_x` and `m_` + `x`), and main-script module names are not prefixed, so `module q1_a` can take the internal names of the first imported file. Since 2026-11-12 the loader rejects any two definitions with one internal name (`reportInternalClash`) instead of merging them, and renames a library's locals that share a name with the main script (`renameLocal`). Codegen's `newLambda` (`codegen.go:388`) still makes `quark__lambda1`, which a user variable `_lambda1` also becomes. Every generated name should come from one place that guarantees it can't be written by a user (rule 6 of the robustness bar).

### 12.3 Dead code (Go)

*Removed in step 1 (12.8), 2026-11-12, except `token.Keywords`, which the editor-data generator uses. The unreferenced runtime functions of 12.4 are removed too (`qv_list_from` stays: the Go tests use it).*

**Legacy `FunctionNode` path.** The parser never creates `ast.FunctionNode`; `fn f(...)` parses as `f = fn(...)` with `FnDecl` set. Everything that handles `FunctionNode` is dead:
- types: `analyzeFunction` (`analyzer_functions.go:54-124`), `predeclareFunctions` / `declareFunctionSignature` (`analyzer_declarations.go`), the `FunctionNode` case in `analyze` (`analyzer.go:282`), the branch in `GetCapturedByFunction` (`analyzer_modules.go:363`), and the two "Legacy FunctionNode" `DispatchDirect` blocks (`analyzer_calls.go` ~351-360 and ~515-524), whose `nativeParamTypes` / `nativeReturnType` locals are never set.
- `a.functions` (keyed by name, so it can't tell shadowed symbols apart) is read only by those legacy blocks. Because every `g = fn` lambda is added to it, a loop variable named like any function variable is called as `DispatchDirect quark_f` and fails in clang. Remove it with them.
- invariants: the `FunctionNode` branch of `ValidateReturnAnnotations` (`callplan.go:173`).
- codegen: the `FunctionNode` parts of `collectFunctions` and `generateNode`, `generateFunction`, `generateQValueFunction`, `generateModule` (a no-op), and `funcNames` with its branch in `generateIdentifier`.

**Unreachable functions** (`deadcode`): `ast.Precedence.String`, `codegen.boxToQValue`, `loader.def.qualified`, `parser.Parser.peekPrecedence`, `parser.Parser.parseCallArguments`, `token.Keywords` (used only by tests), `types.isFunctionBindingAssignment`, `types.functionTypeFromLambdaNode`, `Analyzer.declareFunctionAssignmentSignature`, and the getters `Analyzer.GetModules`, `GetReturnValidation`, `GetExternFns`, `GetNativeFns`, `GetStructTypes` (plus `Errors`, `ErrorDiagnostics`, `WarningDiagnostics`, `GetCaptures`, `GetCallPlans`, used only by tests). The getters predate `types.Analysis`, which replaced them.

**Unused or write-only state:**
- `Analyzer.currentModule` (written, never read); `a.modules` / `Module` only back `GetModules` and a duplicate-module check the loader already does.
- `ModuleLoader.loaded` (`loader.go:251`, written, never read).
- `ast.StatementNode`, `ExpressionNode`, `ConditionNode`: never created.
- `Generator.inFunction`, `Generator.currentFunc`: written, never read.
- `CallPlan.Kind`: never read by codegen or invariants.
- `CallPlan.ArgTypesChecked`: set to `true` at every site, so `INV-ARGCHECK` can't fail.
- `_ = tok` (`parser.go:283`), `_ = externTok` (`parser.go:1039`); the unused first parameter of `generateExternThunk`; `boxResultToQValue` (no-op).

**Nodes the loader removes but later stages still accept.** `InclNode` is handled in the analyzer (`analyzer.go:318`, returns `void`) and codegen (`codegen.go:642`, `1302`, `1381-1383`). After the loader drops every `incl`, these should be internal errors, not silent no-ops. The `incl ID` parse branch (`parser.go:1076`) exists only to produce the loader's "no incl needed" error, and the comment above it (`:1058-1063`) still documents the old form.

### 12.4 Runtime (C++)

The runtime has grown several helpers that do the same job and disagree with each other.

**Error convention.** The real convention is `q_runtime_reportf(...)` then `exit(1)` (about 190 sites), not "return `qv_null()` on mismatch". A few places still return `null` on a type mismatch (`list.concat` with a non-list at `list.hpp:160`, string index with a non-int at `string.hpp:399`), which is a silent wrong result. There is no `[[noreturn]] q_fatalf`, so `types/dict.hpp` has dead `if (!q_require_dict(...)) return qv_null();` branches. Add `q_fatalf`, use it everywhere, and remove the `null` returns.

**Seven type-name tables**, which disagree:

| Where | Difference |
|---|---|
| `ops/arithmetic.hpp:31` `q_type_name_arith` | full list |
| `builtins/math.hpp:13` `q_type_name_math` | stops at `result`, so `abs(struct)` says `unknown` |
| `ops/member.hpp:33` (inline) | says `string`, not `str` |
| `builtins/conversion.hpp:106` and `:137` (inline) | full list, twice |
| `types/vector.hpp:983` (`to_vec` lambda) | switch |
| `ext/api.hpp:377` `q_as_type_name` | stops at `resource` |

Replace all of them with one `q_type_name(ValueType)` next to `q_type`.

**Text formatting.** `print_qvalue` (`builtins/io.hpp:16`, also missing the `q_` prefix), `q_str`, `q_repr_into`, `q_fmt_cell` (formats lists differently from `print`), and `q_fmt_vec` / `q_fmt_vector_cells` (duplicates inside `stdlib/fmt.hpp`). `repr.hpp` should be the only renderer; the others call it with options.

**Boxing helpers.** `qv_vector_from`, `qv_vector_ptr` and `qext::box(QVector*)` are the same; so are `qv_list_ptr` and `qext::box(QList*)`. `qext::box(int)` doesn't compile because the `int64_t`, `double` and `bool` overloads make it ambiguous. A native function returning `nullptr` for a `list`/`vec` is boxed as an empty container and a `null` `const char*` becomes `qv_null()`; the boxing helper is the place to reject both.

**Index and size types.** Lists and strings index with 32-bit `int` (`list.hpp:59, 80, 126, 145, 186-188`, `string.hpp:402-403`, `strings.hpp:184-186`), vectors with `long long` (`q_vec_get_scalar`). `q_vec_size` and `q_list_size` return `int`. Use `int64_t` everywhere with one bounds-check helper.

**Numeric conversions.** float→int casts are unchecked in `floor`, `round`, `ceil` (`math.hpp:133, 143, 153`), `to_int` (`conversion.hpp:89`), `fillna` (`vector.hpp:313`, `q_to_i64_scalar`) and `range` (`list.hpp:263, 285`); `astype('i64')` has the right range check (`vector.hpp:917`). `to_int(str)` ignores `strtoll`'s `ERANGE` (`conversion.hpp:98`), `abs` uses `llabs` (`math.hpp:29`), and `range` steps with an unchecked `i += st` (`list.hpp:371`). One `q_checked_f2i` and the existing `core/checked.hpp` helpers should cover all of these.

**`vector.hpp`.** No macros; the complexity is nested reader lambdas (`with_*_reader` × `cmp_*`), which multiply template instantiations, and about eight hand-written per-dtype switches (`box_at`, `clone`, `mask_filter`, `vectors_identical`, `to_vec`, the fmt cells, `fillna`, `astype`). `q_vec_validate` is called by some entry points and not others (`get_scalar`, `mask_filter`, `to_list`, the reducers and repr skip it). Proposed split:
1. `vector/layout.hpp`: the struct, validity bitmap, buffers, accessors and builders (the Arrow contract `ext/api.hpp` depends on).
2. `vector/kernels.hpp`: arithmetic and comparison.
3. `vector/reduce.hpp`: reductions, including `all`/`any` from `math.hpp`.
4. `vector/convert.hpp`: `to_vec`, `to_list`, `astype`, `fillna`.
5. `vector/index.hpp`: indexing and masks.

A single `q_vec_visit_dtype(v, f)` would replace the per-dtype switches.

**Not referenced by the compiler or stdlib:** `q_list_size`, `q_list_empty`, `q_list_clear`, `q_list_free`, `q_dict_has`, `q_dict_size`, `q_vec_dtype`, `q_vec_numeric_as_i64`, `q_vec_str_append_null`, `qv_list_init`, and the `q_malloc_tensor` / `q_realloc` macros (`core/gc.hpp`). `qv_list_from` is used only by the Go tests that compile C++ against the headers.

**Tables.** `types/table.hpp` and the `VAL_TABLE` branches in `repr.hpp`, `conversion.hpp`, `truthy.hpp` and `value.hpp` are unreachable: no Quark program can create a table yet (`tables_v0_1_spec.md` is paused). Either keep them as the start of that work or remove them until it resumes.

**Smaller items.** `builtins/repr.hpp` is only included indirectly through `conversion.hpp`, though every builtins header is supposed to be listed in `quark.hpp`. `QFilePayload` holds no pointers and could use `q_malloc_atomic`. No live GC hazards were found: container buffers use `q_allocator` or are short-lived copies, vector buffers are atomic, the resource registry uses `gc_allocator`.

### 12.5 Tests

- **`internal/testutil` repeats the pipeline instead of calling the driver.** `Analyze` (`testutil.go:58-68`) runs the analyzer even after loader errors, on a tree the loader didn't finish; the driver stops there (`driver.go:100-103`). Loader errors lose their code and location and are mixed into `ParserErrors`; it uses `analyzer.Errors()` where the driver uses `Diagnostics()`/`HasErrors()`; it always compiles as `test_input.qrk`. Only 5 tests use the driver path, so a test can pass while the CLI behaves differently. Fix: `testutil` calls `driver.CompileSource` and exposes its result.
- **None of the known issues found in the review were caught by the suite.** The misuse paths that rule 2 of the robustness bar asks for are thin for: aliasing and mutation of containers, `any` values flowing into typed code, conditional and repeated expression contexts (short-circuit, ternary, `while` condition, `elseif`), definitions inside loop bodies, generated-name collisions, and numeric limits (int64 boundaries, float→int, NaN, indexes past 2^31).

### 12.6 Open design question: element types

The largest class of known issues is codegen reading a raw value based on an element type or narrowed type the analyzer got wrong (aliases, mutation through a function, index and key assignment, `any` values pushed into a list, narrowing inside a branch). There are three ways out, which trade execution speed against effort:

1. **Checked reads.** Read every list/dict element and every narrowed `any` variable with a checked helper (`q_as_int`, ...). Simple and sound; one tag check per read.
2. **Sound tracking.** Make the analyzer invalidate or widen element types on aliasing, calls and index/key assignment, and undo narrowing where branches merge. Reads stay unchecked, but it is the larger job and every new mutation path must keep it sound.
3. **Both.** Checked reads now, and remove the check where tracking can prove the type.

This needs a decision before the bugs in that group are fixed.

### 12.7 Target design: a lowering pass and a typed IR

Decided 2026-11-12. The front half of the pipeline (lexer, parser, loader, the analyzer's type checking, the builtin catalog) stays. The back half changes: today the analyzer hands codegen 13 side tables keyed by AST node (`types.Analysis`), and codegen walks the AST again and builds C++ strings, consulting those tables and keeping its own scope stack by name (`declaredVars`, `varTiers`, `cellVars`, `scopeStack`). Decisions that belong to analysis (how a variable is stored, in what order an expression is evaluated, how sugar expands) are made while printing, in several places. That is the source of most of 12.2 and of three groups of known issues (expression timing, storage, loop bodies).

Target pipeline:

```
.qrk → Lexer → Parser → Desugar → Loader → Analyzer → Lower → IR → Verify → Emit → C++ → clang++
```

- **Desugar** (after parsing): rewrites pipes `x | f(a)` into `f(x, a)` (the call node keeps a `Piped` flag for the pipe-specific messages), so no later stage has a pipe path. *Done in step 1, in the parser itself (`parsePipe`), so files the loader parses get it too.*
- **Analyzer**: types, as today, plus **symbols**. Every declaration (variable, parameter, loop variable, `when` binding, `fn`, extern, struct) becomes one `*ir.Symbol`; every identifier node maps to its symbol; the analyzer decides each symbol's storage once, from its declared or inferred type and whether it is captured.
- **Lower** (new package `lower/`): turns the analyzed AST into the IR. All lowering decisions live here and nowhere else: `for`/range/vector fast paths, `when` into tests and branches, method calls into calls with the receiver first, default arguments from the `CallPlan`, parameter and struct-field checks, fixed-`fn` hoisting, boxing and unboxing, and evaluation order.
- **Verify**: the invariants pass, run on the IR instead of the AST (every reference has a symbol in scope, every unbox reads a value of the proven type or goes through a check, call arity matches its plan).
- **Emit** (replaces `codegen/`): prints the IR as C++. It reads no AST, makes no type or storage decisions, and names every C++ symbol from the symbol's ID, so generated names can't collide with user names by construction.

IR shape (sketch; the final types are settled in step 3 below):

```go
type Symbol struct {
	ID      int
	Name    string     // as written, for messages
	Type    types.Type
	Storage Storage    // Int64, Float64, Bool, Value, Cell
	Kind    SymKind    // Local, Param, Global, Fn, Extern, Struct
	Checked bool       // a captured cell that may be read before it is assigned
}

type Func struct {
	Sym      *Symbol
	Params   []*Symbol
	Captures []*Symbol
	Native   bool       // fully annotated, no captures: plain C++ signature
	Body     *Block     // hoisted fns first
}

// Statements: Let{Sym, Value}, Assign{Sym, Value}, SetField, SetIndex, SetKey,
// If{Cond, Then, Else}, Loop{Body} (while and for: the condition is tested
// inside the body, then Break), Break, Continue, ExprStmt, Return.
// Expressions: Const, Ref{Sym}, Call{Plan, Callee, Args}, Unary, Binary{Op,
// Type}, Box, Unbox{Storage}, Check{Want, Value, What} (q_param_*/q_as_*),
// Closure{Func}, list/dict/vec/struct literals, MemberGet, Index.
```

**Evaluation order** is explicit: lowering an expression yields statements plus a result, and any expression that needs a temporary is flattened into `Let`s placed before it in the same block. Conditional and repeated contexts (`and`, `or`, `x if c else y`, a `while` condition, `elseif`) lower to blocks, so their temporaries land inside the branch or loop that evaluates them. This removes the whole "part of an expression runs at the wrong time" group of known issues rather than patching each one.

**Checks are IR nodes.** A raw read of a scalar is an `Unbox` only where the analyzer proved the type, otherwise a `Check`. Whichever answer 12.6 gets, it is implemented in one place (where lowering chooses `Unbox` or `Check`), and the verifier rejects an `Unbox` the analyzer didn't prove.

**AST cleanup that goes with it.** `OperatorNode` currently covers assignment, member access and every operator, which is why `analyzeOperator` and `generateOperator` are single giant switches. The parser produces distinct node kinds instead: `AssignNode` (by target: name, field, key, index), `MemberNode`, `UnaryNode`, `BinaryNode`, and a `FnDeclNode` for `fn f(...)` (as `modules_v0_2_spec.md` §5.4 intended).

What doesn't change: the runtime ABI (`QValue`, `QClosure`, `QCell`, `QFnInfo`, `q_call*`), the builtin catalog and `CallPlan`, the loader, the CLI and the test programs.

### 12.8 Migration plan

Each step ends with the full suite green and is committed on its own.

1. **Simplify without new machinery.**
   - Remove the dead code in 12.3 and the unreferenced runtime functions in 12.4.
   - Add the desugar pass for pipes; delete `analyzePipe` and `generatePipe`.
   - Split `OperatorNode` into the node kinds above and add `FnDeclNode`; split `analyzeOperator` by node kind; split the loader's `walk` by node kind.
   - Make `internal/testutil` call the driver (12.5).

   Exit: the call/pipe duplication is gone and no Go function scores above about 40 in `gocyclo`.

   *Done 2026-11-12.* Highest scores after it: `inferBuiltinReturnType` 40, `CanAssign` 37, `loader.collect` 37, `checkDefiniteAssignment` 34, `codegen.scalarExpr` 34 (`analyzeOperator` 159 → split into `analyzeMember`, `analyzeUnary`, `analyzeAssign` with `assignName`/`assignMember`/`assignIndex`, `analyzeBinary` with `analyzeVectorArith`/`analyzeArith`/`analyzeOrdering`/`analyzeEquality`; loader `walk` 66 → 15; `nextRawToken` 48 → 21 with token tables; `ValidateCallPlans` 48 → split per dispatch mode). Behavior changes: `x | d.f()` is the same error as `d.f(x)`; a pipe target that isn't a call is a parse error; arity messages read `function 'f' expects 1 argument but got 2`.
2. **Symbols.** The analyzer creates an `ir.Symbol` per declaration, maps identifier nodes to symbols, and decides storage once. Codegen reads storage and C++ names from symbols and drops `declaredVars`, `varTiers`, `cellVars` and its scope stack. Exit: the storage known issues are fixed (`x: float = len(l)`, an int assigned to a float variable, `_lambda1`, a variable first assigned in a `while` body).
3. **Lower, IR, verify, emit.** Build `lower/` and the emitter next to `codegen/`, selected by a driver switch while in progress. Run the smoke and integration suites through both back ends and compare outputs; switch the default once they match, then delete `codegen/`. Exit: the expression-timing and loop-body known issues are fixed, and the verifier runs on every compile.
4. **Element types** (12.6): implement the chosen answer in lowering as `Unbox`/`Check`, plus the analyzer changes it needs.

In parallel with any step:
- runtime consolidation (12.4): one type-name table, one renderer, `q_fatalf`, 64-bit indexes, checked float→int conversions, the `vector.hpp` split;
- the runtime numeric known issues;
- return-annotation checks for ordinary functions (an analyzer and runtime change, unaffected by the IR);
- the missing misuse tests (12.5).

Interim patches to the current codegen are limited to severe bugs that can't wait for step 3; today that is only the `while` condition evaluated once (an infinite loop). Other expression-timing and storage bugs are fixed by steps 2-3 rather than patched in code those steps replace.
