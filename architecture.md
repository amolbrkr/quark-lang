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

The analyzer produces three metadata outputs consumed by later stages:
- **CallPlans**: Per-call-site metadata (dispatch mode, arity, defaults to inject)
- **Captures**: Per-lambda map of captured variable names
- **Return validation**: Per-function declared-vs-inferred return type checks

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

### 1.6 Codegen → C++17 Source

Walks the annotated AST and emits C++17 code using the runtime headers.

**Naming**:
- User identifiers prefixed with `quark_` (e.g., `quark_x`, `quark_main`)
- Runtime builtins use `q_` prefix
- Value constructors use `qv_` prefix
- Lambdas: `quark__lambda1`, `quark__lambda2`, etc.

**Variables**: All variables are `QCell*`. Reads emit `quark_x->value`, writes emit `quark_x->value = expr`.

**Function calls**: Lowered according to the CallPlan's dispatch mode:
- `DispatchBuiltin` → `q_print(arg)` (direct C++ call)
- `DispatchDirect` → `quark_foo(nullptr, arg)` (known function, no closure)
- `DispatchClosure` → `q_call1(val, arg)` (dynamic dispatch through QClosure)

**Method calls**: When `IsMethod=true`, the receiver is injected as the first runtime argument (e.g., `"hello".upper()` → `q_upper(receiver_val)`).

**Other lowerings**:
- **Pipe**: `x | f(a)` → emits `f` call with `x` prepended to args
- **Default injection**: Missing trailing args filled from DefaultNodes in the CallPlan
- **Lambdas**: Emitted as top-level C++ functions with closure allocation at the capture site
- **for loops**: Lowered to index-based iteration (`q_iter_get` with incrementing counter)
- **if/while**: Conditions wrapped in `q_condition_bool()` for strict-bool enforcement
- **when**: Lowered to a chain of if/else with result payload extraction

**Scope management**:
- `pushScope()` — Isolate function scope (clean slate)
- `pushBlockScope()` — Copy parent scope for nested blocks (allows shadowing)
- `popScope()` — Restore previous scope
- `declaredVars` — Tracks declarations to prevent redeclaration

**Generation order**:
1. Collect all function declarations (forward declarations)
2. Emit function definitions
3. Emit `main()` with GC initialization
4. Predeclare function bindings in main for mutual recursion
5. Initialize module bindings
6. Execute top-level statements

### 1.7 C++ Compiler → Binary

The generated C++ is compiled with clang++ (preferred) or g++ using:
- `-std=c++17 -O3 -march=x86-64-v3` (on amd64)
- `-DQUARK_USE_GC` + Boehm GC include/link flags
- Optional `-flto` for link-time optimization

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

Every variable (locals, parameters, loop variables) is stored in a `QCell*` — a heap-allocated mutable reference cell. Reading dereferences `cell->value`; assignment writes `cell->value`.

This uniform-cell design means **any variable can be captured by a closure** without special handling at the capture site. Multiple closures over the same variable share the same `QCell*`, enabling shared mutable state.

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
    Kind            CallKind       // CallBuiltin | CallFunctionValue
    CalleeName      string
    MinArity        int
    MaxArity        int
    Dispatch        DispatchMode   // DispatchBuiltin | DispatchDirect | DispatchClosure
    RuntimeSymbol   string         // C++ symbol (e.g., "q_upper", "quark_myfunc")
    DefaultNodes    []*TreeNode    // Trailing default args to inject
    IsMethod        bool           // True for x.method(args)
    ReceiverNode    *ast.TreeNode  // The receiver expression
    ReceiverTypeKey string         // builtins.TypeKey of receiver
}
```

**Dispatch modes:**
- `DispatchBuiltin` — Direct call to `q_*` runtime function
- `DispatchDirect` — Direct call to `quark_*` user function (nullptr closure)
- `DispatchClosure` — Dynamic call through `q_call*` (dereferences QClosure)

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
