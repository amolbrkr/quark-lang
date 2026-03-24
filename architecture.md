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
| `long long` | native int | not captured + static type `int` |
| `double` | native float | not captured + static type `float` |
| `bool` | native bool | not captured + static type `bool` |
| `QValue` | tagged union | not captured + unknown/any/composite type |

Closure captures received from an enclosing scope (extracted from `_cl->captures[i]`) are always `QCell*` — the cell is shared with the outer scope regardless of type.

`generateIdentifier` boxes scalar vars back to `QValue` at read sites (e.g., `qv_int(quark_x)`) so the rest of codegen always operates on `QValue` expressions. Scalar storage is transparent to all other codegen paths.

Codegen tracks storage choices in `cellVars` (names stored as `QCell*`) and `varTiers` (names stored as a named C++ scalar type), both scoped and stacked with the scope stack.

**Function calls**: Lowered according to the CallPlan's dispatch mode:
- `DispatchBuiltin` → `q_print(arg)` (direct C++ call)
- `DispatchDirect` → `quark_foo(nullptr, arg)` (known function, no closure)
- `DispatchClosure` → `q_call1(val, arg)` (dynamic dispatch through QClosure)

If codegen encounters an unexpected dispatch/runtime-symbol invariant break, it terminates with an internal compiler error (`INV-*`) rather than emitting fallback runtime behavior.

**Method calls**: When `IsMethod=true`, the receiver is injected as the first runtime argument (e.g., `"hello".upper()` → `q_upper(receiver_val)`).

**Operator lowering**: For arithmetic (`+`, `-`, `*`, `/`, `%`) and ordering comparisons (`<`, `<=`, `>`, `>=`), codegen first checks whether both operands are scalar atoms (literals or scalar-tiered local variables). If so, it emits raw C++ operators and boxes the result — e.g., `qv_int(((long long)x) + ((long long)y))` — instead of calling `q_add(...)`. Division between two `int` operands promotes to `double`. Falls back to the boxed `q_add` etc. path whenever either operand is `QValue`/unknown.

**Other lowerings**:
- **Pipe**: `x | f(a)` → emits `f` call with `x` prepended to args
- **Default injection**: Missing trailing args filled from DefaultNodes in the CallPlan
- **Lambdas**: Emitted as top-level C++ functions with closure allocation at the capture site
- **for loops**: Lowered to index-based iteration (`q_iter_get` with incrementing counter); loop variable uses scalar tier when the iterable's element type is statically known
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
