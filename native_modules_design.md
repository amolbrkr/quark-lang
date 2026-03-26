# Quark Extensions Interface (QEI) Design

Status: Implemented (Phase 1 + Phase 2a–2e complete)
Date: 2026-03-26

---

## Changelog

### 2026-03-26 — Phase 2e + scalar lowering extensions implemented

Annotated user-defined functions now emit native C++ signatures (Phase 2e). Scalar lowering extended to cover `==`/`!=` comparisons and `if`/`while` bool conditions.

#### Phase 2e: Annotated Function Parameters → Native C++ Signatures

**`src/core/quark/ir/call.go`**
- Added `DispatchNative` dispatch mode constant (distinct from `DispatchExtern` — for user-defined functions, not extern declarations)

**`src/core/quark/types/analyzer.go`**
- Added `nativeFns map[string]*ir.CallPlan` field, initialized in `NewAnalyzer()`

**`src/core/quark/types/analyzer_modules.go`**
- Added `GetNativeFns() map[string]*ir.CallPlan` accessor
- Added `isFullyAnnotated()` predicate — true when all params have non-Any scalar annotations, return type is annotated as scalar, and no default params
- Reuses `quarkTypeToNativeCType()` from Phase 2b

**`src/core/quark/types/analyzer_functions.go`**
- Added native fn registration in `analyzeLambda()` after `validateReturnType`: when `funcName != "lambda"` and `isFullyAnnotated()`, registers a `CallPlan` with `Dispatch: DispatchNative`, `RuntimeSymbol: "quark_" + name`, and `NativeParamTypes`/`NativeReturnType` populated

**`src/core/quark/types/analyzer_calls.go`**
- Modified `analyzeFunctionCall()`: checks `nativeFns` map before the `sym.Mutable` guard — necessary because lambda-assigned functions have `Mutable: true` in scope, which previously blocked the direct-dispatch path
- Modified `analyzePipeCall()`: same check added for pipe call sites

**`src/core/quark/invariants/callplan.go`**
- Added `DispatchExtern` and `DispatchNative` to the allowed dispatch mode set (both were previously missing, causing invariant failures)
- Updated `INV-RUNTIME-SYMBOL` check to require a runtime symbol for both `DispatchExtern` and `DispatchNative`

**`src/core/quark/codegen/codegen.go`**
- Extended `funcDecl` struct with `nativeParamTypes []string` and `nativeReturnType string`
- Added `nativeFns map[string]*ir.CallPlan` field to `Generator`; added `SetNativeFns()` method
- Added `nativeCppTypeToTier()` — maps C++ type strings to scalar tier strings for `scalarExpr` compatibility
- Updated `collectFunctions()`: added `OperatorNode[=]` case to detect native lambda assignments and give them their canonical user name (e.g., `quark_add` instead of `quark__lambda1`); LambdaNode case skips already-named lambdas
- Split `generateLambdaFunc()`: delegates to `generateNativeFunction()` for native fns, `generateQValueFunction()` for all others
- Added `generateNativeFunction()` — emits native-typed C++ signature with `int64_t`/`double`/`bool` params; uses `scalarExpr` on the last body expression to avoid box-unbox roundtrip in return value
- Added `generateNativeThunk()` — emits `QValue quark_name__thunk(QClosure*, QValue, ...)` wrapper for first-class use
- Updated `generateLambdaExpr()`: returns `qv_func((void*)quark_name__thunk)` for native fns
- Updated `generateIdentifier()`: returns thunk reference for native function names used as values
- Added `DispatchNative` case in `generateFunctionCall()` and `generatePipe()`: uses `scalarExpr` on arg nodes to avoid box-then-unbox at call sites; falls back to `adaptArgForExtern()` for QValue args

**`src/core/quark/main.go`**
- Added `gen.SetNativeFns(analyzer.GetNativeFns())` at all three codegen call sites (emit, build, run paths)

**`src/testfiles/smoke_native_fns.qrk`** (new file)
- Tests: direct native calls, scalar-tiered variable args, multi-statement body (`clamp`), first-class use via thunk (`apply(double_it, 6)`)

**`src/core/quark/integration_smoke_test.go`**
- Added `native_fns` test case

#### Scalar Lowering Extensions (Section 5.3.1 + 5.3.2)

**`src/core/quark/codegen/codegen.go`**
- Extended `scalarExpr()` OperatorNode case to handle `==` and `!=`: both operands must be scalar-tiered; result tier is `"bool"`
- Updated `generateIf()`: checks `scalarExpr()` on condition — if tier is `"bool"`, emits raw C++ bool directly instead of `q_truthy(...)`. Same applied to `elseif` inline condition.
- Updated `generateWhile()`: same `scalarExpr` check on condition

**`src/testfiles/smoke_numeric_lowering.qrk`**
- Added test cases for `==`/`!=` scalar lowering, scalar `while` condition, scalar `if`/`elseif` condition

**`src/core/quark/integration_smoke_test.go`**
- Updated `numeric_lowering` expected output

#### What is NOT yet implemented

- **Thunk generation for `extern fn`** — `DispatchNative` (user-defined) functions have thunks; `DispatchExtern` (extern declarations) do not. Using an `extern fn` as a first-class value (assigning to a variable, passing as a callback) will not work correctly.
- **`for i in range(n)` → raw C++ loop** (Section 5.3.3) — `range(n)` still allocates a `QList` of boxed integers
- **Direct vector literal construction** (Section 5.3.5) — `vector [1, 2, 3]` still builds an intermediate list then calls `q_to_vector()`
- **Phase 3** (stdlib migration: `std/io`, `std/fmt`, removal of `_`-prefixed catalog entries) — deferred

---

### 2026-03-25 — Phase 1 + Phase 2a–2d implemented

All language prerequisites and core QEI machinery are implemented and building cleanly. Phase 3 (stdlib migration) is deferred.

#### Phase 1: Language Prerequisites

**`src/core/quark/token/token.go`**
- Added `EXTERN` token type constant, name string, and keyword mapping (`"extern" → EXTERN`)

**`src/core/quark/ast/ast.go`**
- Added `ExternSourceNode` and `ExternFnNode` node types with name strings
- Added `ExternSymbol string` and `ExternReceiver string` fields to `TreeNode` (the `as 'symbol'` name and receiver type prefix for methods)

**`src/core/quark/parser/parser.go`**
- Added `case token.EXTERN: return p.parseExtern()` dispatch in `parseStatement()`
- Added `parseExtern()` — dispatches on string literal vs `fn` keyword
- Added `parseExternFn()` — parses `extern fn [type.]name(params) ReturnType as 'symbol'`; detects the `type.name` method pattern by checking for a DOT after the first identifier
- Added `isBuiltinTypeKeyword()` helper — recognizes `list`, `dict`, `vector`, `int`, `float`, `str`, `bool` tokens for method receiver detection

#### Phase 2a: Loader

**`src/core/quark/loader/loader.go`**
- Added `normalizeExternPath()` helper — resolves relative paths without appending `.qrk`
- Added `ExternSourceNode` handling in `resolveImportsInNode()`: rewrites the path token literal to the resolved absolute path (supports relative, stdlib `std/`, and absolute forms). Codegen reads this resolved path directly.

#### Phase 2b: Analyzer

**`src/core/quark/types/analyzer.go`**
- Added `externFns map[string]*ir.CallPlan` and `externSources []string` fields to `Analyzer`
- Added `case ast.ExternSourceNode` (no-op) and `case ast.ExternFnNode` dispatch in `analyze()`
- Initialized both new fields in `NewAnalyzer()`

**`src/core/quark/types/analyzer_modules.go`**
- Added `GetExternFns() map[string]*ir.CallPlan` accessor
- Added `analyzeExternFn()`: validates name and `as` symbol; collects `ParameterNode` children and maps annotations to native C++ types; for free functions — registers in `a.builtins`, `a.currentScope`, `a.functions`, and `a.externFns[name]`; for methods — resolves receiver key, registers in `a.methods[receiverKey]` and `a.externFns["type.method"]`; guards against redefining prelude entries
- Added `quarkTypeToNativeCType()` — maps `Type` → C++ type string (`"int64_t"`, `"double"`, `"QVector*"`, etc.)
- Added `receiverStringToTypeKey()` — maps `"list"`, `"vector"`, etc. to `builtins.TypeKey`
- Added `receiverStringToNativeCType()` — maps receiver string to C++ type string
- Fixed: `returnType := TypeAny` → `var returnType Type = TypeAny` (Go type inference was picking `*BasicType` instead of the `Type` interface, causing a build error)

**`src/core/quark/types/analyzer_calls.go`**
- Modified `analyzeFunctionCall()`: after builtin lookup, checks `a.externFns[name]`; if found, copies prototype `CallPlan`, sets `DispatchExtern`, and stores at the call site
- Modified `resolveMethodCall()`: after method sig lookup, checks `a.externFns[receiverStr+"."+methodName]`; if found, copies prototype plan with receiver info and sets `DispatchExtern`
- Added `typeKeyToReceiverString()` helper — maps `TypeKey` back to receiver string prefix

#### Phase 2c: IR + Codegen

**`src/core/quark/ir/call.go`**
- Added `DispatchExtern` dispatch mode constant
- Added `NativeParamTypes []string`, `NativeReturnType string`, `NativeReceiverType string` fields to `CallPlan`

**`src/core/quark/codegen/codegen.go`**
- Added `collectExternSources()` — walks AST collecting unique `ExternSourceNode` path literals (preserves order, deduplicates)
- Added `adaptArgForExtern()` — emits unboxing helper per native type (`q_as_int()`, `q_as_float()`, `q_as_str()`, `q_as_vector()`, etc.) or passes through for `QValue`
- Added `wrapExternReturn()` — boxes native return values back to `QValue` (`qv_int()`, `qv_float()`, `qv_vector_ptr()`, etc.)
- Modified `Generate()` to collect and emit `#include` directives for extern sources before forward declarations
- Added `DispatchExtern` case in `generateFunctionCall()` and `generatePipe()`: adapts receiver (for methods) and explicit args per `NativeParamTypes`, wraps result with `wrapExternReturn()`
- Added `ast.ExternSourceNode, ast.ExternFnNode` no-op cases in `generateNode()` and `generateExpr()`
- Modified top-level statement loop to skip `ExternSourceNode` and `ExternFnNode`
- Fixed: `qv_vector(callExpr)` → `qv_vector_ptr(callExpr)` for `QVector*` return wrapping (`qv_vector()` takes a capacity `int`, not a pointer)

#### Phase 2d: Extension Author API

**`src/core/quark/runtime/include/quark/ext/api.hpp`** (new file)
- `qext::` namespace: `box()` overloads for all types, `null_val()`, unboxing with runtime checks (`as_int()`, `as_float()`, `as_bool()`, `as_str()`, `as_vector()`, `as_list()`, `as_dict()`, `as_closure()`), memory helpers (`malloc`, `malloc_atomic`, `strdup`), error reporting (`panic()`, `panicf()`)
- `QSlice<T>` — C++17-compatible pointer+size view (replaces `std::span` which is C++20)
- Vector accessors: `as_f64()`, `as_f64_mut()`, `as_i64()`, `as_i64_mut()`, `as_bool_vec()`, `as_bool_vec_mut()` returning `QSlice<T>` views into internal storage
- Vector constructors: `new_f64(n)`, `new_i64(n)`, `new_bool_vec(n)` — GC-allocated, zeroed
- Vector metadata: `vec_size()`, `vec_has_nulls()`, `vec_dtype()`, `vec_dtype_name()`, `is_null_at()`, `null_mask()`
- Dict helpers: `dict_get()`, `dict_set()`, `dict_size()`, `dict_has()`
- Closure call helpers: `call(fn)` through `call(fn, a0, a1, a2)` overloads
- Codegen-facing helpers outside namespace: `q_as_int()`, `q_as_float()`, `q_as_bool()`, `q_as_str()`, `q_as_vector()`, `q_as_list()`, `q_as_dict()`, `q_as_closure()`, `qv_vector_ptr()`, `qv_list_ptr()`, `qv_dict_ptr()`, `qv_closure_ptr()`

**`src/core/quark/runtime/include/quark/quark.hpp`**
- Added `#include "ext/api.hpp"` at the bottom so codegen-facing helpers are always available (the `q_as_*`/`qv_*_ptr` functions are emitted at `DispatchExtern` call sites regardless of whether the user's file explicitly includes `ext/api.hpp`)

#### What is NOT yet implemented

- **Phase 2e** (annotated function parameters with native C++ types) — deferred
- **Phase 3** (stdlib migration: `std/io`, `std/fmt`, removal of `_`-prefixed catalog entries) — deferred
- **Thunk generation** for extern functions used as first-class values — the `DispatchExtern` machinery is in place but thunk emission is not yet implemented; using an extern fn as a value (assigning to a variable, passing as a callback) will not work correctly in v0.1

---

## 1. Vision

Quark is a high-level data language that compiles to C++17 native binaries. Its value proposition is high level language ergonomics (similar to Python or R/dplyr) with compiled-binary performance.

To achieve this dual mandate of ergonomics <> performance, Quark's architecture must allow users to write high-level code while enabling performance-critical operations to be implemented in C++. The Quark Extensions Interface (QEI) is the design for how this works.

Two insights drive this design:

1. **The boundary between Quark and C++ should be invisible.** Users write high-level pipelines. Performance-critical code lives in C++. The transition is seamless — same types, same binary, zero overhead.

2. **The extension mechanism powers the library ecosystem.** Like Python's C extensions enabled NumPy/pandas, Quark extensions enable the data library ecosystem without the language itself becoming bloated.

## 1.2 Goals and Non-Goals

### Goals

1. **Stdlib in Quark, hot paths in C++.** The standard library should be authored as Quark modules where some functions delegate to C++ implementations. `std/io`, `std/math`, `std/csv` are Quark modules — they just happen to call C++ under the hood. Users `use 'std/io'` and never know the difference.

2. **Native-typed calling convention.** When type information is available (from annotations or inference), function calls between Quark and C++ should use native C++ types (`int64_t`, `double`, `bool`, `const char*`, `QVector*`) — not QValue. The compiler generates any needed adaptation. Extension authors write clean C++ with natural types.

3. **Zero-copy vector access.** Extension code operating on vectors must be able to access the underlying typed storage (`double*`, `int64_t*`) directly without copying or per-element unboxing. This is the performance bridge that makes Quark competitive with native C++ for numeric workloads.

4. **No Quark-specific plugin/loading mechanism.** Extensions are compiled in — no dlopen, no plugin registry, no runtime discovery. For pure-C++ extensions (custom algorithms, data parsers), this means a single static binary with zero deployment complexity. For extensions that wrap external libraries (DuckDB, SQLite, Arrow), the extension's `.hpp` wrapper is compiled in and the external library is linked by passing `-l` flags to clang++. A syntax for specifying these flags from within `.qrk` files (`extern link`) is planned but deferred to post-v0.1; for now, flags are passed manually.

5. **Extension authors learn one small API.** The `quark/ext/api.hpp` header provides vector access, value construction, GC-aware allocation, and closure callbacks. That's it. No framework, no macros, no base classes.

6. **Extend existing Quark types.** Extensions can add methods to built-in types (`list`, `str`, `vector`, `dict`, etc.) using the `extern fn type.name(...)` syntax. User-defined methods are indistinguishable from built-in methods — same dot-call syntax, same analyzer resolution path, same error messages. This is how the ecosystem grows: a CSV extension adds `str.parse_csv()`, a stats extension adds `vector.median()`, etc.

### Non-Goals

1. **Dynamic plugin loading.** Not in v0.1. Extensions are compiled in.
2. **Automatic C++ header parsing.** The compiler never parses `.hpp` files. The `extern fn` declaration is the contract — the compiler trusts it.
3. **C interop / extern "C".** QEI targets C++17 only. C libraries are wrapped in C++ by the extension author.
4. **ABI stability across compiler versions.** Extension headers are recompiled every build. No binary compatibility guarantees.
5. **Sandboxing or capability restrictions.** Extensions have full access to the process. Security is the extension author's responsibility.

---

## 2. The Extension Mechanism

An extension is a Quark module (`.qrk` file) where some or all functions are implemented in C++ rather than Quark. From the user's perspective, there is no difference between calling a pure-Quark function and calling an extern function — the syntax is identical.

### 2.1 Syntax

Two constructs, one keyword:

```quark
extern '<path>'                                                  # include a C++ implementation file
extern fn <name>(<params>) <return_type> as '<symbol>'          # free function
extern fn <type>.<name>(<params>) <return_type> as '<symbol>'   # method on a type
```

The parser distinguishes free functions from methods by the name pattern: `sqrt` is a free function, `list.shuffle` is a method on `list`. The type prefix must be a known type keyword (`int`, `float`, `str`, `bool`, `list`, `vector`, `dict`). No new keywords beyond `extern` — methods and functions share `extern fn`.

### 2.2 Complete Example: Free Functions

**Quark module** (`std/math.qrk`):
```quark
extern 'std/math_impl.hpp'

extern fn sqrt(x: float) float as 'q_sqrt'
extern fn abs(x: float) float as 'q_abs'
extern fn pow(base: float, exp: float) float as 'q_pow'
```

**C++ implementation** (`std/math_impl.hpp`):
```cpp
#include <cmath>
#include <quark/ext/api.hpp>

double q_sqrt(double x) { return std::sqrt(x); }
double q_abs(double x)  { return std::fabs(x); }
double q_pow(double base, double exp) { return std::pow(base, exp); }
```

**User code:**
```quark
use 'std/math'
x = sqrt(144.0)
println(x)
```

### 2.2.1 Complete Example: Type Extension (Methods)

**Quark module** (`ext/listutils.qrk`):
```quark
extern 'listutils_impl.hpp'

extern fn list.shuffle() list as 'q_list_shuffle'
extern fn list.chunk(size: int) list as 'q_list_chunk'
extern fn vector.normalize() vector as 'q_vec_normalize'
extern fn vector.dot(other: vector) float as 'q_vec_dot'
```

**C++ implementation** (`listutils_impl.hpp`):
```cpp
#include <quark/ext/api.hpp>
#include <algorithm>
#include <random>

QList* q_list_shuffle(QList* l) {
    // copy + shuffle (immutable semantics)
    QList* out = (QList*)q_malloc(sizeof(QList));
    new (out) QList(*l);
    std::shuffle(out->begin(), out->end(), std::mt19937{std::random_device{}()});
    return out;
}

double q_vec_dot(QVector* a, QVector* b) {
    auto va = qext::as_f64(a);
    auto vb = qext::as_f64(b);
    double sum = 0.0;
    for (size_t i = 0; i < va.size(); i++) sum += va[i] * vb[i];
    return sum;
}
```

**User code:**
```quark
use 'ext/listutils'

items = list [1, 2, 3, 4, 5]
shuffled = items.shuffle()        # dot syntax — same as builtin methods
chunks = items.chunk(2)

v1 = vector [1.0, 2.0, 3.0]
v2 = vector [4.0, 5.0, 6.0]
similarity = v1.dot(v2)           # dot syntax — indistinguishable from builtins
normed = v1.normalize()
```

The user sees no difference between `items.push(6)` (builtin) and `items.shuffle()` (extension). Both use dot syntax, both go through the same analyzer method resolution, both produce the same error messages on misuse.

**How it works in the analyzer**: When the parser sees `extern fn list.shuffle() ...`, it recognizes the `type.name` pattern. The analyzer registers this in the **same method catalog** used by builtins — `methods[TypeListAny]["shuffle"] = Spec{...}`. At call sites, `items.shuffle()` resolves through `resolveMethodCall()` exactly like `items.push(6)`. No new resolution logic, no fallback chains, no precedence rules.

### 2.3 The `as` Clause

The `as 'symbol'` specifies the exact C++ function name to call. Required for all extern declarations — no guessing, no implicit name mangling.

```quark
extern fn sqrt(x: float) float as 'q_sqrt'          # free function
extern fn str.upper() str as 'q_upper'               # method on str
extern fn vector.dot(other: vector) float as 'q_dot' # method on vector
```

The symbol maps to the `RuntimeSymbol` field in `CallPlan`. But unlike existing builtins which use `Dispatch: DispatchBuiltin` (where all arguments are QValue), extern functions use a new dispatch mode: **`DispatchExtern`**. This tells codegen that the target function has a native-typed signature and arguments must be adapted accordingly.

For methods, the receiver is implicit — it is not listed in the parameter list but is injected as the first C++ argument by codegen (same as existing builtin methods). So `extern fn str.upper() str as 'q_upper'` maps to `const char* q_upper(const char* self)` — the receiver becomes the `self` parameter.

### 2.4 The Type Annotation → ABI Contract

Type annotations on `extern fn` declarations serve double duty:

1. **Quark side**: The analyzer validates call sites against these types (arity, type compatibility) — same as any Quark function with annotations.
2. **C++ side**: The annotations define the native C++ types the extension function receives and returns.

**Type mapping table:**

| Quark annotation | C++ type | Notes |
|---|---|---|
| `int` | `int64_t` | Zero overhead — direct register pass |
| `float` | `double` | Zero overhead — direct register pass |
| `bool` | `bool` | Zero overhead — direct register pass |
| `str` | `const char*` | Pointer to GC-managed string. **Returning strings**: extension must use `q_strdup()` to allocate into GC-managed memory. Returning a stack buffer or `std::string::c_str()` is a dangling pointer bug — the GC doesn't know about non-GC memory |
| `vector` | `QVector*` | One pointer. Typed storage (`double*`, `int64_t*`) accessible via `qext::as_f64()` etc. — see Section 4.1 |
| `list` | `QList*` | One pointer. Elements are QValue (heterogeneous) |
| `dict` | `QDict*` | One pointer |
| `fn` | `QClosure*` | One pointer. For callback parameters — see Section 2.9 |
| *(no annotation)* | `QValue` | Tagged union. Extension author dispatches on `.type` manually. This is the escape hatch for genuinely polymorphic functions |

**The rule**: Annotate it → get the native type. Don't annotate → get QValue.

For extern declarations, every parameter should be annotated. Leaving a parameter unannotated is the explicit choice to accept any type and handle dispatch yourself in C++. The intended use case for unannotated parameters is genuinely polymorphic functions — for example, an extension that accepts either a float or int vector and dispatches on `vec_dtype()`:

```cpp
// Quark declaration — no annotation on v, accepts any vector dtype
extern fn vec_sum(v: vector) float as 'q_vec_sum'

// C++ implementation — checks dtype at runtime
double q_vec_sum(QVector* v) {
    switch (qext::vec_dtype(v)) {
        case QVector::F64: {
            auto data = qext::as_f64(v);
            return std::accumulate(data.begin(), data.end(), 0.0);
        }
        case QVector::I64: {
            auto data = qext::as_i64(v);
            return (double)std::accumulate(data.begin(), data.end(), 0LL);
        }
        default: qext::panicf("vec_sum: unsupported dtype %d", qext::vec_dtype(v));
    }
}
```

Note: `v` is still annotated as `vector` here — the receiver and container types should always be annotated. "Unannotated" refers to leaving off the scalar type annotation to get `QValue` for a parameter that could be `int` or `float`. Don't leave parameters unannotated just to avoid thinking about types — the QValue overhead is real.

**Annotation mismatch is a silent ABI bug**: The compiler never reads `.hpp` files and cannot verify that the C++ function signature matches the `extern fn` declaration. A mismatched annotation (e.g., declaring `x: int` but the C++ function takes `double`) will not be caught by Quark and may silently produce wrong results or corrupt data depending on how clang++ handles the type mismatch at the call site. Keep `extern fn` declarations and C++ signatures in sync manually.

### 2.4.1 What Changes in CallPlan

Today's `CallPlan` (in `ir/call.go`) carries `RuntimeSymbol` and `Dispatch` mode but no type information about parameters or return values. For extern functions, the analyzer must populate new fields so codegen knows how to adapt arguments:

```go
type CallPlan struct {
    // ... existing fields ...
    Dispatch        DispatchMode   // NEW value: DispatchExtern
    RuntimeSymbol   string         // C++ function name from `as` clause

    // NEW — populated for DispatchExtern calls:
    NativeParamTypes  []string     // C++ type per param: "int64_t", "double", "bool",
                                   // "const char*", "QVector*", "QList*", "QDict*",
                                   // "QClosure*", "QValue"
    NativeReturnType  string       // C++ return type (same set as above)
    NativeReceiverType string      // C++ type of the method receiver, e.g. "QVector*",
                                   // "const char*", "QList*". Empty for free functions.
}
```

This lets codegen handle each argument independently — some might be native, some might be QValue, depending on which parameters the extension author annotated. For method calls, codegen reads `NativeReceiverType` to adapt the receiver expression independently of `NativeParamTypes` (which covers only the explicit parameters).

### 2.4.2 Thunk Generation

When a typed extern function is used as a **function value** (assigned to a variable, passed as an argument, stored in a collection), it needs to be wrapped in a `QClosure`. Since closures always use the QValue calling convention (`QClosure* _cl, QValue, QValue, ...`), the compiler generates a thin thunk:

```cpp
// Extension author writes:
double q_sqrt(double x) { return std::sqrt(x); }

// Compiler generates (only when sqrt is used as a value):
QValue _thunk_q_sqrt(QClosure* _cl, QValue _arg_x) {
    return qv_float(q_sqrt(q_as_float(_arg_x)));
}
```

Note: `q_as_float()` is a safe unboxing helper that checks `_arg_x.type == VAL_FLOAT` and panics with a clear error on mismatch. Raw union access (`_arg_x.data.float_val`) is never used in thunks — the type is not statically guaranteed.

When `sqrt` is only ever called directly (the common case), no thunk is generated.

### 2.5 Path Resolution

`extern '<path>'` paths resolve the same way as `use` imports:

| Path form | Resolution |
|---|---|
| `'io_impl.hpp'` | Relative to the `.qrk` file containing the directive |
| `'../shared/utils.hpp'` | Relative path traversal from `.qrk` file |
| `'std/io_impl.hpp'` | Stdlib root (same discovery as `use 'std/...'`) |

System headers (e.g., `<arrow/api.h>`, `<sqlite3.h>`) go INSIDE the `.hpp` file as normal C++ `#include`s. Quark only needs to find the extension author's code.

### 2.6 What the Compiler Does

Each compiler stage has a specific, bounded responsibility for extern declarations:

**1. Parser**: Recognizes `extern '<path>'` and `extern fn`. For `extern fn`, detects the `type.name` pattern to distinguish methods from free functions. Creates AST nodes with signature metadata (param names, type annotations, `as` symbol, and receiver type if method) but no body. These are new node types — they do not reuse the existing function declaration node.

**2. Loader**: Resolves `extern '<path>'` paths using the same logic as `use` path resolution. Records the resolved absolute path for codegen to emit as `#include`. Does NOT parse or read the `.hpp` file — it's opaque C++ to the compiler.

**3. Analyzer**: Processes `extern fn` nodes and registers them into the same function/method tables that `catalog.go` populates for builtins. Free functions go into the function registry; `type.name` methods go into the method registry keyed by `(ReceiverType, methodName)` — identical to how builtin methods are registered. For each extern declaration:
   - Validates parameter annotations are valid types
   - Computes `NativeParamTypes` and `NativeReturnType` from the annotations using the type mapping table (Section 2.4)
   - For methods, computes `NativeReceiverType` from the type prefix (e.g. `vector` → `"QVector*"`, `str` → `"const char*"`)
   - Creates a `CallPlan` with `Dispatch: DispatchExtern`, the `RuntimeSymbol` from the `as` clause, and the native type fields
   - Call site validation (arity, type checking) works the same as for any other function
   - **Thunk detection**: when an extern fn identifier appears in a non-call-site position (assigned to a variable, passed as an argument, stored in a collection), the analyzer sets a `NeedsThunk bool` flag on the extern fn's registration entry. Codegen reads this flag to decide whether to emit the thunk. This is done in the analyzer — not codegen — because the analyzer already knows the call/non-call context when resolving identifiers, and codegen is single-pass.

**4. Codegen**: Two responsibilities:
   - **Preamble**: Emits `#include "resolved/path/to/impl.hpp"` for each `extern '<path>'`, before any generated code. This makes extension functions visible at all call sites.
   - **Call sites**: When codegen encounters a `DispatchExtern` call, it adapts each argument from its current representation to the native type specified in `NativeParamTypes`. Three cases per argument:
     - Argument is already the right native type (scalar-tiered local, literal) → pass directly
     - Argument is QValue but target is native → emit guarded unbox: `q_as_float(expr)`, `q_as_int(expr)`, etc.
     - Argument is QValue and target is QValue (unannotated param) → pass through unchanged
   - **Return**: Wraps the native return into whatever the caller needs — if caller expects QValue, emit `qv_float(result)`. If caller is also scalar-tiered, keep native.
   - **Thunks**: For extern functions where the analyzer set `NeedsThunk`, emits the QValue thunk (Section 2.4.2) in the preamble and wraps it in a `QClosure` at the use site.

**5. clang++**: Compiles everything together — generated code + included extension headers + runtime — into one binary. No special flags needed for pure-C++ extensions. For external library extensions that wrap external libraries (DuckDB, Arrow, etc.), additional `-l` flags will be needed — the mechanism for specifying these (`extern link`) is deferred to post-v0.1.

The key principle: **nothing downstream of the analyzer changes its architecture.** Extern functions enter the same registries, produce the same `CallPlan` IR (with additional fields), and flow through the same codegen paths (with a new dispatch branch). The extension mechanism is an addition, not a restructuring.

### 2.7 Generated C++ Structure

For a program that uses an extension module, the generated C++ has this structure:

```cpp
// ──── Preamble (always present) ────
#include "quark/runtime.hpp"            // QValue, operators, builtins

// ──── Extension includes (from extern 'path') ────
#include "resolved/path/to/math_impl.hpp"
#include "resolved/path/to/io_impl.hpp"

// ──── Thunks (only for extern fns used as function values) ────
QValue _thunk_q_sqrt(QClosure* _cl, QValue _arg_x) {
    return qv_float(q_sqrt(q_as_float(_arg_x)));
}

// ──── Forward declarations (Quark-defined functions) ────
QValue quark_process(QClosure* _cl, QValue _arg_data);

// ──── Quark function bodies ────
QValue quark_process(QClosure* _cl, QValue _arg_data) {
    // ...
    double _t1 = q_sqrt(3.14);           // direct native call (typed context)
    // ...
}

// ──── main ────
int main() {
    GC_init();
    // ...
    return 0;
}
```

Key points:
- Extension `.hpp` files are `#include`d in the preamble, before any generated code. This means extension functions are visible at all call sites without forward declarations.
- Thunks are generated **only** for extern functions that are used as values (assigned to variables, passed as arguments, stored in collections). If `sqrt` is only ever called directly, no thunk is emitted.
- The rest of the generated code is unchanged — Quark functions, lambdas, and `main()` are emitted the same way they are today.

### 2.8 Calling Convention

Extension functions do **not** follow the Quark closure calling convention. They are plain C++ functions — no hidden `QClosure* _cl` first parameter, no QValue wrapping (unless a parameter is deliberately unannotated).

**Direct call (common case — caller has type info):**
```quark
x = 2.0
y = sqrt(x)
```
```cpp
double quark_x = 2.0;
double quark_y = q_sqrt(quark_x);      // native double → native double
```

**Direct call (caller has QValue — no type info available):**
```quark
x = some_dynamic_value()
y = sqrt(x)
```
```cpp
QValue quark_x = quark_some_dynamic_value(nullptr);
// Compiler injects guarded unbox at call site:
double quark_y = q_sqrt(q_as_float(quark_x));
```

**Indirect call (extern fn used as a function value):**
```quark
transform = sqrt
y = transform(9.0)
```
```cpp
// sqrt is stored as a QClosure wrapping the thunk:
QValue quark_transform = qv_func(q_new_closure((void*)_thunk_q_sqrt, 0));
// Called through closure dispatch:
QValue quark_y = q_call1(quark_transform, qv_float(9.0));
```

This means extension functions pay the QValue cost **only** when used as first-class function values — which is rare for performance-critical math/data functions. The normal call path is zero-overhead.

**Method calls** follow the same rules but inject the receiver as the first C++ argument:

```quark
extern fn str.upper() str as 'q_upper'

name = "hello"
loud = name.upper()
```
```cpp
const char* quark_name = "hello";
const char* quark_loud = q_upper(quark_name);  // receiver passed as first arg
```

```quark
extern fn vector.dot(other: vector) float as 'q_vec_dot'

v1 = vector [1.0, 2.0, 3.0]
v2 = vector [4.0, 5.0, 6.0]
result = v1.dot(v2)
```
```cpp
QVector* quark_v1 = /* ... */;
QVector* quark_v2 = /* ... */;
double quark_result = q_vec_dot(quark_v1, quark_v2);  // receiver + explicit arg
```

### 2.9 Callbacks: C++ Calling Quark Closures

Extension functions sometimes need to call back into Quark code — for `map`, `filter`, `sort` with custom comparators, etc. The extension author API provides typed callback helpers:

```quark
# Quark module declaration
extern fn vmap(v: vector, f: fn) vector as 'q_vmap'
```

```cpp
// C++ implementation
QVector* q_vmap(QVector* v, QClosure* f) {
    auto data = qext::as_f64(v);
    auto result = qext::new_f64(data.size());
    auto out = qext::as_f64_mut(result);
    for (size_t i = 0; i < data.size(); i++) {
        // Call the Quark closure — must go through QValue at closure boundary
        QValue mapped = qext::call(f, qv_float(data[i]));
        out[i] = mapped.data.float_val;
    }
    return result;
}
```

**Why callbacks use QValue**: The closure `f` could be any Quark function — its body was compiled with whatever calling convention matched its definition. The extension can't know the internal types at compile time, so `qext::call()` uses the QValue closure dispatch (`q_call1`, `q_call2`, etc.) which is the same mechanism Quark uses for all dynamic function calls.

The `qext::call` API:

```cpp
namespace qext {
    QValue call(QClosure* fn);                              // 0 args
    QValue call(QClosure* fn, QValue a);                    // 1 arg
    QValue call(QClosure* fn, QValue a, QValue b);          // 2 args
    QValue call(QClosure* fn, QValue a, QValue b, QValue c); // 3 args
    // For higher arities: use q_calln(fn, {a, b, c, d, ...})
}
```

**Performance note**: The callback overhead (QValue box/unbox per element) is real. For hot loops where the callback body is trivial, this dominates. Mitigations:
- Extension authors can provide specialized non-callback versions for common operations (e.g., `q_vec_add_scalar` instead of `vmap(v, fn x -> x + k)`).
- Future optimization: if the compiler can prove the closure's type signature, it could pass a native function pointer alongside the QClosure, letting the extension call it directly. This is deferred — callbacks through QValue are correct and sufficient for v0.1.


## 3. Catalog Evolution

With QEI, the catalog (`builtins/catalog.go`) splits into two tiers: a small **prelude** of always-available primitives that stays in the catalog, and everything else which migrates to stdlib `.qrk` modules as extern declarations.

### 3.1 The Prelude (Stays in Catalog)

These functions and methods are available in every Quark program without any `use` import. They are language primitives — removing any of them would make Quark feel broken.

**Free functions:**

| Function | Why it's prelude |
|---|---|
| `print`, `println`, `input` | I/O primitives — every program needs these |
| `len` | Universal — works on str, list, vector, dict |
| `type` | Introspection primitive |
| `to_str`, `to_int`, `to_float`, `to_bool` | Type conversions — fundamental operations |
| `to_vector` | List→vector conversion — bridges the two collection types |
| `range` | Loop iteration primitive |
| `ok`, `err`, `is_ok`, `is_err`, `unwrap` | Result type primitives |
| `abs`, `min`, `max`, `sum` | Core math — polymorphic (work on scalars and vectors) |
| `sqrt`, `floor`, `ceil`, `round` | Core math — used too frequently to require an import |

**Type methods (always available on their types):**

| Type | Methods |
|---|---|
| `str` | `upper`, `lower`, `trim`, `contains`, `startswith`, `endswith`, `replace`, `concat`, `split`, `slice` |
| `list` | `push`, `pop`, `get`, `set`, `insert`, `remove`, `slice`, `reverse`, `enumerate`, `join`, `concat`, `to_vector` |
| `dict` | `get`, `set`, `keys`, `values`, `items` |
| `vector` | `get`, `fillna`, `astype`, `to_list` |

These stay as `DispatchBuiltin` with QValue args. No performance regression, no C++ rewrite needed. They're the foundation that extensions build on top of.

### 3.2 What Migrates to Stdlib Modules

Everything currently using the `_` prefix, plus any future builtins that belong to a specific domain:

| Current catalog entry | Migrates to | New name |
|---|---|---|
| `_file_open` | `std/io.qrk` | `file_open` |
| `_file_read` | `std/io.qrk` | `file_read` |
| `_file_write` | `std/io.qrk` | `file_write` |
| `_file_close` | `std/io.qrk` | `file_close` |
| `_file_seek` | `std/io.qrk` | `file_seek` |
| `_file_exists` | `std/io.qrk` | `file_exists` |
| `_fmt_list` | `std/fmt.qrk` | `fmt_list` |
| `_fmt_vector` | `std/fmt.qrk` | `fmt_vector` |
| `_fmt_dict` | `std/fmt.qrk` | `fmt_dict` |
| `_fmt_table` | `std/fmt.qrk` | `fmt_table` |
| `enumerate` (free fn alias) | removed | use `mylist.enumerate()` method |
| `vfrom_list` (free fn alias) | removed | use `mylist.to_vector()` method |

This is a **breaking change**. The `_` prefix is removed entirely — module boundaries provide encapsulation, not naming conventions. Code using `_file_open(...)` must change to:

```quark
use 'std/io'
file_open("data.csv", "r")
```

### 3.3 How Both Tiers Coexist

The analyzer maintains the same two registries it has today: `byName` (free functions) and `byMethod` (type methods keyed by `(ReceiverType, name)`).

At startup:
1. The catalog populates both registries with prelude entries (as today)
2. As the loader processes `use` imports and encounters `extern fn` declarations, the analyzer adds them to the **same registries**

At call sites, resolution is unchanged:
- Free function call `foo(x)` → look up `byName["foo"]`
- Method call `x.foo()` → look up `byMethod[type_of_x]["foo"]`

Both catalog entries and extern entries live in the same maps. The only difference is the `Dispatch` mode on the resulting `CallPlan`:
- Catalog entries → `DispatchBuiltin` (QValue args)
- Extern entries → `DispatchExtern` (native-typed args)

Codegen checks the dispatch mode and emits the appropriate call. No ambiguity, no precedence rules — a name is either in the registry or it isn't.

**Name collision rule**: If an extern declaration in a user module tries to register a name that already exists in the prelude (e.g., `extern fn len(...)`), the analyzer reports an error: "cannot redefine prelude function 'len'". Prelude names are reserved. Extension authors extend types by adding new method names, not by overriding existing ones.

### 3.4 Future: Catalog Shrinks Further

As the compiler matures, some prelude entries may migrate to extern declarations in an auto-imported prelude module (`std/prelude.qrk`). This would make even the core builtins self-hosted in `.qrk` files, with the catalog reduced to only things that need special compiler support (e.g., `ok`/`err` which produce result types). This is not planned for v0.1 — the current split is sufficient.

## 4. Extension Author API

A stable C++ API that ships with Quark for writing extensions. Lives at `quark/ext/api.hpp`. Extension authors include this one header and get everything they need.

```cpp
#include <quark/ext/api.hpp>
```

This header re-exports the runtime types (`QValue`, `QVector`, `QList`, `QDict`, `QClosure`) and provides the `qext::` namespace with typed accessors, constructors, and helpers.

### 4.1 Vector Access (the Performance Bridge)

Vectors store typed data in contiguous arrays (`std::vector<double>`, `std::vector<int64_t>`, etc.) with GC-aware allocators. The extension API provides zero-copy typed views into this storage:

```cpp
namespace qext {
    // Read-only typed views — zero-copy, returns span into existing storage
    std::span<const double>   as_f64(const QVector* v);
    std::span<const int64_t>  as_i64(const QVector* v);
    std::span<const uint8_t>  as_bool(const QVector* v);

    // Mutable typed views — for filling newly-created vectors
    std::span<double>   as_f64_mut(QVector* v);
    std::span<int64_t>  as_i64_mut(QVector* v);
    std::span<uint8_t>  as_bool_mut(QVector* v);

    // Vector construction — GC-allocated, zeroed
    QVector* new_f64(size_t n);
    QVector* new_i64(size_t n);
    QVector* new_bool(size_t n);

    // Convenience: create from existing data (copies into GC memory)
    QVector* new_f64(std::span<const double> data);
    QVector* new_i64(std::span<const int64_t> data);

    // Null mask access
    bool has_nulls(const QVector* v);
    std::span<const uint8_t> null_mask(const QVector* v);  // 0 = valid, 1 = null

    // Metadata
    size_t vec_size(const QVector* v);
    QVector::Type vec_dtype(const QVector* v);  // F64, I64, BOOL, STR
}
```

**Usage example** — a dot product extension:
```cpp
double q_vec_dot(QVector* a, QVector* b) {
    auto va = qext::as_f64(a);
    auto vb = qext::as_f64(b);
    double sum = 0.0;
    for (size_t i = 0; i < va.size(); i++) sum += va[i] * vb[i];
    return sum;
}
```

**Usage example** — creating a new vector:
```cpp
QVector* q_vec_scale(QVector* v, double factor) {
    auto data = qext::as_f64(v);
    auto result = qext::new_f64(data.size());
    auto out = qext::as_f64_mut(result);
    for (size_t i = 0; i < data.size(); i++) out[i] = data[i] * factor;
    return result;
}
```

**Implementation note**: These are thin wrappers. `as_f64()` is essentially `std::get<QVecF64>(v->storage).data()` with a size. No copying, no allocation, no type conversion. The typed storage already exists inside `QVector` — the API just gives you a typed pointer to it.

**Dtype mismatch**: If the extension calls `as_f64()` on an I64 vector, the function panics with a clear error: "expected F64 vector, got I64". Extension authors should check `vec_dtype()` if they need to handle multiple dtypes.

**GC lifetime warning**: A `std::span` is not a GC root — it is a raw pointer + size into the `QVector`'s internal storage. If a GC collection runs while you hold only a span (no live `QVector*`), the underlying vector can be collected and the span becomes dangling. This is most likely in callback-heavy code: you call `qext::as_f64(v)`, then call `qext::call(f, ...)` — the callback allocates, GC runs, and if `v` was the only reference it may be collected. The rule: **hold the `QVector*` alive for the entire lifetime of any span derived from it**. In practice: extract all data you need before calling back into Quark, or re-fetch the vector after the callback returns.

```cpp
// Unsafe — span held across a callback that can allocate:
auto data = qext::as_f64(v);
QValue result = qext::call(f, qv_float(data[0]));  // GC may run here
double x = data[1];  // data may be dangling

// Safe — extract needed values before calling back:
double first = qext::as_f64(v)[0];
QValue result = qext::call(f, qv_float(first));
```

### 4.2 Dict Access

Dicts are `QDictMap` — an `unordered_map<string, QValue>` with GC-allocated nodes. Extension authors access them directly:

```cpp
namespace qext {
    // Lookup — returns qv_null() if key not found
    QValue dict_get(const QDict* d, const char* key);

    // Insert/update — returns the dict (for chaining in C++)
    void dict_set(QDict* d, const char* key, QValue val);

    // Iteration
    size_t dict_size(const QDict* d);

    // Direct access to underlying map (for advanced use)
    const QDictMap& dict_entries(const QDict* d);
    QDictMap& dict_entries_mut(QDict* d);
}
```

Since dict values are `QValue` (dicts are heterogeneous), there's no typed accessor — the extension author works with QValue for dict values and uses `qv_int()`, `qv_string()` etc. to construct them.

### 4.3 Closure Calling

When an extension receives a Quark function as a callback (`QClosure*`), it calls it through `qext::call`:

```cpp
namespace qext {
    QValue call(QClosure* fn);                                // 0 args
    QValue call(QClosure* fn, QValue a);                      // 1 arg
    QValue call(QClosure* fn, QValue a, QValue b);            // 2 args
    QValue call(QClosure* fn, QValue a, QValue b, QValue c);  // 3 args
    // Higher arities: use q_calln(fn, std::vector<QValue>{...})
}
```

Callbacks always go through QValue — the extension can't know the closure's internal types at compile time. See Section 2.9 for details and performance notes.

### 4.4 Error Reporting

```cpp
namespace qext {
    // Fatal error — prints message and terminates
    [[noreturn]] void panic(const char* msg);

    // Formatted panic
    [[noreturn]] void panicf(const char* fmt, ...);
}
```

Extensions should panic on unrecoverable errors (dtype mismatch, out-of-bounds, invalid arguments). For recoverable errors, return `ok(val)` / `err(val)` result types using the existing constructors:

```cpp
// Return an ok result
QValue result = qv_ok(qv_int(42));

// Return an error result
QValue result = qv_err(qv_string_copy("file not found"));
```

### 4.5 Value Construction

The existing `qv_*` constructors are the API. Extension authors use these to create QValue when needed (callback arguments, dict values, return values from untyped functions):

```cpp
QValue qv_int(long long v);
QValue qv_float(double v);
QValue qv_string(const char* v);       // does NOT copy — pointer must be GC-managed
QValue qv_string_copy(const char* v);  // copies via q_strdup — always safe, use this by default
QValue qv_bool(bool v);
QValue qv_null();
QValue qv_list(int capacity);          // creates empty list with pre-allocated capacity
QValue qv_dict();                      // creates empty dict
QValue qv_ok(QValue inner);
QValue qv_err(QValue inner);
```

**String construction**: Prefer `qv_string_copy()` — it calls `q_strdup()` internally and is always safe. Use `qv_string()` only when you already have a GC-managed `const char*` and want to avoid a redundant copy (e.g., a string you just allocated with `q_strdup()`). Passing a stack buffer, a `std::string::c_str()`, or any non-GC pointer to `qv_string()` is a dangling pointer bug — the GC doesn't know about the allocation and may collect it.

### 4.6 Memory Management

All allocations must use GC-aware functions. The GC (Boehm) manages the entire heap — if you allocate with `malloc` or `new`, the GC won't track it and pointers stored inside may get collected.

```cpp
// General allocation — GC scans the block for pointers to other GC objects
void* q_malloc(size_t n);

// Atomic allocation — GC does NOT scan for pointers (use for strings, numeric buffers)
void* q_malloc_atomic(size_t n);

// String duplication — copies into GC-managed atomic memory
char* q_strdup(const char* s);
```

**When to use which:**
- `q_malloc()` — structs or buffers that contain pointers to other GC objects (e.g., a custom struct with a `QValue` field)
- `q_malloc_atomic()` — data without pointers: numeric arrays, string buffers, byte buffers. More efficient because the GC skips pointer scanning.
- `q_strdup()` — always use this for strings returned to Quark. Copies the string into GC-managed atomic memory.
- **Never** use `malloc`, `new`, `std::make_unique` etc. for objects that will be referenced by Quark values. The GC won't see them.

## 5. Performance Architecture

### 5.1 Core Principle: Scalars First, QValue as Fallback

Quark compiles to C++17. C++ is not a foreign environment — it IS the execution environment. The performance strategy follows from this:

**Use native C++ types (`long long`, `double`, `bool`, `const char*`) wherever the type is known. Use QValue only where the type is genuinely unknown at compile time.**

QValue is a runtime polymorphism mechanism — a tagged union that lets a single variable hold any type. It exists because Quark is dynamically typed and needs to represent "any value" in cases like heterogeneous lists, untyped function parameters, and dynamic dispatch. But it is not the default — it is the fallback.

The compiler should be **fast by default**: type information is inferred from literals, assignments, and operations — not just from annotations. `x = 2.3` gives the compiler everything it needs to store `x` as a `double`. Annotations exist for when the compiler can't figure it out, not as a prerequisite for performance.

### 5.2 Scalar Tiering (What Exists Today)

The compiler already implements a 5-tier variable storage system driven by type inference:

| Tier | C++ storage | When used |
|---|---|---|
| `QCell*` | Heap-allocated cell | Variable is captured by a nested closure |
| `long long` | Stack scalar | Type inferred as `int`, not captured |
| `double` | Stack scalar | Type inferred as `float`, not captured |
| `bool` | Stack scalar | Type inferred as `bool`, not captured |
| `QValue` | Tagged union | Type unknown, or container/function/string type |

The analyzer infers types from literals and annotations via `GetNodeTypes()`. Codegen checks each variable's inferred type at declaration and picks the appropriate tier. Within a function body, scalar-tiered variables use raw C++ operators:

```quark
x = 10
y = 20
z = x + y    # emits: long long quark_z = ((long long)quark_x + (long long)quark_y);
```

No `q_add()` call, no QValue construction, no type dispatch. This is real — it works today for arithmetic (`+`, `-`, `*`, `/`, `%`), comparisons (`<`, `<=`, `>`, `>=`), and unary negation on scalar locals and literals.

Loop variables are also scalar-tiered when the iterable has a known element type:

```quark
for x in list [1, 2, 3]     # x is long long, not QValue
    y = x * 2               # raw C++ multiplication
```

### 5.3 Where Scalar Tiering Doesn't Reach Yet

The following are concrete, bounded optimizations that extend the scalar-first principle to places where the compiler currently falls back to QValue or runtime dispatch unnecessarily.

#### 5.3.1 `==` and `!=` on Scalars (Priority: High, Effort: Low)

**Current behavior** (`codegen.go:918`): Equality and inequality are explicitly excluded from native comparison lowering. Two scalar-tiered ints go through `q_eq()` / `q_neq()` which check types at runtime.

```cpp
// Today: x == y where both are long long
q_eq(qv_int(quark_x), qv_int(quark_y))   // box both, call runtime, check types, unbox, compare

// Should be:
qv_bool(quark_x == quark_y)               // one C++ comparison, box the bool result
```

**Fix**: Remove the `op != token.DEQ && op != token.NE` guard in `generateOperator()`. Same scalar detection logic that works for `<`, `>` etc. applies here. The only reason to go through `q_eq` is when operands might be different types (string equality, list equality) — but if both are scalar-tiered, they're guaranteed to be the same type.

#### 5.3.2 `while` Condition on Booleans (Priority: Medium, Effort: Low)

**Current behavior** (`codegen.go:1437`): `while` always wraps the condition in `q_truthy()`, which takes a QValue, checks its type, and returns a C++ bool.

```cpp
// Today: while flag (where flag is bool-tiered)
while (q_truthy(qv_bool(quark_flag)))    // box to QValue, call q_truthy, extract bool

// Should be:
while (quark_flag)                        // direct C++ bool
```

**Fix**: Before emitting the `while`, check if the condition expression is a scalar bool (via `scalarExpr()`). If so, emit it directly. Same applies to `if` conditions.

#### 5.3.3 `for i in range(n)` Without List Allocation (Priority: High, Effort: Medium)

**Current behavior** (`codegen.go:1370-1427`): `for i in range(n)` calls `q_range()` which allocates a `QList` of boxed integers, then iterates it with `q_len()` / `q_iter_get()`. For `range(1_000_000)`, that's a million QValue allocations just to count.

The loop variable `i` is already scalar-tiered to `long long` via `loopVarTier()` — so the individual iterations are fast. But the list allocation and element access are pure waste.

```cpp
// Today:
QValue _t1 = q_range(qv_int(1000000));                     // allocate list of 1M QValues
long long _t2 = q_len(_t1).data.int_val;                   // get length
for (long long _t3 = 0; _t3 < _t2; _t3++) {
    long long quark_i = q_iter_get(_t1, qv_int(_t3)).data.int_val;  // unbox each element
    // ...
}

// Should be:
long long _range_end = 1000000;
for (long long quark_i = 0; quark_i < _range_end; quark_i++) {
    // ...
}
```

**Fix**: Codegen recognizes `for VAR in range(...)` as a special pattern. When `range` has 1-3 literal or scalar-tiered arguments, emit a raw C++ `for` loop directly. No list allocation, no `q_iter_get()`. This is the single highest-impact optimization for numeric loops.

#### 5.3.4 Annotated Function Parameters (Priority: High, Effort: Medium — Deferred to Phase 2)

> **Note**: This optimization requires the `NativeParamTypes`/`NativeReturnType` machinery added to `CallPlan` for QEI (Section 2.4.1) and the thunk generation mechanism (Section 2.4.2). It is in-scope but deferred until Phase 2 of the implementation plan, after extern functions are working end-to-end.

**Current behavior**: Even with type annotations, function parameters are always `QValue`:

```quark
fn add(x: int, y: int) int -> x + y
```
```cpp
// Today:
QValue quark_add(QClosure* _cl, QValue _arg_x, QValue _arg_y) {
    long long quark_x = _arg_x.data.int_val;    // unbox
    long long quark_y = _arg_y.data.int_val;    // unbox
    return qv_int(((long long)quark_x + (long long)quark_y));  // rebox
}
```

The annotations say `x: int, y: int` and return `int`. The compiler has all the information to generate a native signature — no monomorphization needed, no call-site analysis, just read the annotations:

```cpp
// Should be:
long long quark_add(QClosure* _cl, long long _arg_x, long long _arg_y) {
    return _arg_x + _arg_y;
}
```

**Fix**: When a function has type annotations on ALL parameters and a return type annotation, codegen generates the function with native C++ types. The analyzer computes `NativeParamTypes` and `NativeReturnType` for the function (same fields as `DispatchExtern` CallPlans). At call sites where the caller has typed values, the call is direct. At call sites where the caller has QValue, codegen emits unboxing at the call site.

This is not monomorphization — there is exactly one version of the function. It uses whatever types the annotations specify. If a function has no annotations, it stays QValue as today.

**Interaction with closures**: If an annotated function is used as a function value (assigned to a variable, passed as callback), codegen generates a QValue thunk — same mechanism as extern functions (Section 2.4.2). The native version is for direct calls; the thunk is for dynamic dispatch.

#### 5.3.5 Direct Vector Literal Construction (Priority: Medium, Effort: Medium)

**Current behavior** (`codegen.go:1474`): `vector [1, 2, 3]` builds an intermediate `QList`, then calls `q_to_vector()` which scans the list to detect the element type and copies into typed storage.

```cpp
// Today:
QValue _t1 = qv_list(3);
_t1 = q_push(_t1, qv_int(1));
_t1 = q_push(_t1, qv_int(2));
_t1 = q_push(_t1, qv_int(3));
QValue _t2 = q_to_vector(_t1);   // scan list, detect all-int, allocate QVecI64, copy

// Should be (when all elements are same-type literals):
QVector* _t1 = qext::new_i64(3);
auto _t1_data = qext::as_i64_mut(_t1);
_t1_data[0] = 1; _t1_data[1] = 2; _t1_data[2] = 3;
QValue _t2 = qv_vector(_t1);
```

**Fix**: When codegen sees a vector literal where all children are literals of the same type (all INT, all FLOAT, etc.), emit direct typed construction. Skip the intermediate list and the type-detection scan. The type is known at compile time — use it.

### 5.4 Extension Performance

The QEI calling convention (Section 2.4) extends the scalar-first principle across the Quark↔C++ boundary:

- **Typed extern functions** receive native C++ types directly. `extern fn sqrt(x: float) float as 'q_sqrt'` means the C++ function is `double q_sqrt(double)` — no QValue at the boundary.
- **Vector extensions** access typed storage via zero-copy spans (`qext::as_f64()` returns `std::span<double>` pointing directly into `QVector`'s internal `std::vector<double>`). The bulk data never touches QValue.
- **QValue at the boundary** only occurs for untyped parameters (the escape hatch for polymorphic functions) and closure callbacks (where the callee's types aren't known).

This means an extension that does `dot(v1, v2)` on two float vectors is: one function call with two native pointers → direct array access → native arithmetic → native double return. Zero QValue in the entire path.

### 5.5 What QValue Is Still Necessary For

QValue is not going away. It is the correct representation for:

- **Heterogeneous collections**: `list [1, "hello", true]` — elements have different types, QValue is the only common type.
- **Dict values**: Dicts map string keys to any value. Values must be QValue.
- **Untyped function parameters**: `fn identity(x) -> x` — the compiler can't know what `x` is.
- **Dynamic dispatch through closures**: When a function is called through a `QClosure*`, the caller doesn't know the callee's types.
- **`ok`/`err` result wrapping**: The inner value can be any type.
- **`print`, `len`, `type`**: Genuinely polymorphic — must accept any type.

The goal is not to eliminate QValue. The goal is to ensure it only appears where polymorphism is actually needed, and native types are used everywhere else.

## 6. Language Changes Required

One language change is needed before QEI implementation can begin: the `extern` keyword.

### 6.1 `extern` Keyword

The core QEI mechanism. One new keyword, two constructs:

```quark
extern 'math_impl.hpp'                                       # include C++ source
extern fn sqrt(x: float) float as 'q_sqrt'                   # free function
extern fn vector.normalize() vector as 'q_vec_normalize'     # method on a type
```

The parser distinguishes by what follows `extern`:
- String literal → C++ source include (previously `extern source`)
- `fn` → function/method declaration

Described in detail in Section 2.

### 6.2 `return` Keyword (Future — Not Required for v0.1 QEI)

Early return is a separate language feature, deferred to a future milestone. It is not required for QEI or stdlib migration.

## 7. Stdlib Migration

### 7.1 Current State

The stdlib lives at `src/core/stdlib/` with two pure-Quark modules:

```
src/core/stdlib/
├── io.qrk       # Quark wrappers around _file_* catalog builtins
└── fmt.qrk      # Quark wrappers around _fmt_* catalog builtins
```

These modules define user-friendly APIs (e.g., `io.open`) that delegate to `_` prefixed catalog builtins (e.g., `_file_open`). The C++ implementations live separately in `runtime/include/quark/builtins/`.

### 7.2 After QEI

Each stdlib module becomes a `.qrk` + `.hpp` pair. The `.qrk` file contains `extern` declarations that directly expose the C++ functions — no wrapper layer. The `.hpp` file contains the C++ implementations, moved from the runtime builtins directory.

```
src/core/stdlib/
├── io.qrk        # extern declarations
├── io.hpp        # C++ implementations (moved from runtime/builtins)
├── fmt.qrk       # extern declarations
└── fmt.hpp       # C++ implementations (moved from runtime/builtins)
```

**`io.qrk` after migration:**
```quark
extern 'io.hpp'

extern fn open(path: str, mode: str) result as 'q_file_open'
extern fn read(handle, size: int) result as 'q_file_read'
extern fn write(handle, data: str) result as 'q_file_write'
extern fn close(handle) result as 'q_file_close'
extern fn seek(handle, offset: int, whence: int) result as 'q_file_seek'
extern fn exists(path: str) bool as 'q_file_exists'

fn seek_set() int -> 0
fn seek_cur() int -> 1
fn seek_end() int -> 2
```

**`fmt.qrk` after migration:**
```quark
extern 'fmt.hpp'

extern fn show_list(lst: list, n: int, show_index: bool) str as 'q_fmt_list'
extern fn show_vec(vec: vector, n: int, show_index: bool) str as 'q_fmt_vector'
extern fn show_dict(dct: dict, n: int, show_index: bool) str as 'q_fmt_dict'
extern fn table(df: dict, n: int, show_index: bool) str as 'q_fmt_table'

fn head(lst: list, n: int = 5) str ->
    show_list(lst.slice(0, n), 0, true)

fn tail(lst: list, n: int = 5) str ->
    show_list(lst.slice(len(lst) - n, len(lst)), 0, true)
```

Note that `head` and `tail` stay as pure Quark — they're logic, not performance-critical. Only the functions that need C++ implementations use `extern fn`. Modules can freely mix extern and pure-Quark functions.

User code is unchanged:
```quark
use 'std/io'
handle = io.open("data.csv", "r")
```

### 7.3 The `_` Prefix Builtins Die

Once the stdlib modules use `extern fn` declarations, the `_` prefixed entries in `catalog.go` are removed:

- `_file_open`, `_file_read`, `_file_write`, `_file_close`, `_file_seek`, `_file_exists` → removed from catalog, live in `std/io.qrk` as `extern fn`
- `_fmt_list`, `_fmt_vector`, `_fmt_dict`, `_fmt_table` → removed from catalog, live in `std/fmt.qrk` as `extern fn`
- `enumerate` (free fn alias), `vfrom_list` (free fn alias) → removed, use method syntax instead

This is a breaking change for any code using `_` prefixed names directly. The fix is to `use 'std/io'` or `use 'std/fmt'` and call the clean names.

---

## 8. Implementation Plan

### Phase 1: Language Prerequisites

**1a. `extern` keyword — parser + AST**
- Lexer: add `EXTERN` token
- Parser: recognize `extern 'path'` (source include) and `extern fn name(...) type as 'symbol'` (function declaration). Detect `type.name` pattern for methods.
- AST: new node types `ExternSourceNode` and `ExternFnNode` with fields for params, type annotations, `as` symbol, and optional receiver type

### Phase 2: Extern Functions (Core QEI)

**2a. Loader**
- Resolve `extern 'path'` using existing `use` path resolution logic
- Record resolved absolute paths for codegen
- Do NOT parse `.hpp` files

**2b. Analyzer**
- Process `ExternFnNode`: validate param types, compute `NativeParamTypes` and `NativeReturnType`
- Register free functions in `byName`, methods in `byMethod` — same tables as catalog builtins
- Generate `CallPlan` with `Dispatch: DispatchExtern` and native type arrays
- Call site validation: arity and type checking work as normal

**2c. Codegen**
- Add `DispatchExtern` to `ir/call.go`
- Emit `#include` directives for resolved extern source paths in preamble
- Handle `DispatchExtern` in `generateFunctionCall()`: adapt each argument based on `NativeParamTypes` (pass native, guarded unbox from QValue, or pass QValue through)
- Wrap return values based on `NativeReturnType` and caller context
- Generate thunks for extern functions used as values

**2d. Extension Author API**
- Create `quark/ext/api.hpp` header with the `qext::` namespace
- Implement vector accessors (`as_f64`, `as_i64`, `new_f64`, etc.) — thin wrappers over `QVector` internals
- Implement dict accessors, closure call helpers, error reporting
- Tests: write a small test extension (e.g., `extern fn dot(a: vector, b: vector) float as 'q_dot'`) and verify end-to-end

**2e. Annotated Function Parameters (Section 5.3.4)**
- Reuse `NativeParamTypes`/`NativeReturnType`/`NativeReceiverType` fields added in 2b
- Analyzer: when a function has annotations on all params and return type, compute native type fields and set `Dispatch: DispatchNative` (or reuse `DispatchExtern` — TBD)
- Thunk detection: same analyzer-side `NeedsThunk` mechanism as extern functions
- Codegen: emit native-typed function signature; at call sites with QValue callers, inject unboxing; emit thunks for functions used as values
- Tests: annotated functions called directly, called with QValue args, and passed as callbacks

### Phase 3: Stdlib Migration

**3a. Migrate `std/io`**
- Create `src/core/stdlib/io.hpp` — move C++ implementations from `runtime/include/quark/builtins/`
- Rewrite `src/core/stdlib/io.qrk` with `extern` declarations (replace `_file_*` wrappers)
- Remove `_file_*` entries from `catalog.go`
- Update smoke tests

**3b. Migrate `std/fmt`**
- Same pattern: create `fmt.hpp`, rewrite `fmt.qrk`, remove `_fmt_*` from catalog
- Update smoke tests

**3c. Remove dead catalog entries**
- Remove `enumerate` and `vfrom_list` free function aliases
- Verify all `_` prefixed builtins are gone from catalog
- Run full test suite
