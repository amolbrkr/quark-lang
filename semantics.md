# Quark Language Semantics

This document covers the runtime semantics, compilation model, error behaviour, and architectural decisions of the Quark language. It complements **grammar.md** (syntax/grammar) and **stdlib.md** (built-in function reference).

---

## 1) Value Model

Every runtime value is a `QValue` — a tagged union carrying one of ten types:

| Tag | Payload | Notes |
|-----|---------|-------|
| `int` | 64-bit signed integer (`long long`) | |
| `float` | 64-bit IEEE double | |
| `str` | Null-terminated `char*` | GC-owned copy; never aliased |
| `bool` | C++ `bool` | |
| `null` | (none) | Singleton-like; no payload |
| `list` | Pointer to `QList` | `std::vector<QValue>` with GC allocator |
| `vector` | Pointer to `QVector` | Typed columnar storage (f64/i64/bool/str) |
| `dict` | Pointer to `QDict` | String-keyed `unordered_map` with GC allocator |
| `fn` | Pointer to `QClosure` | All functions, including non-capturing ones |
| `result` | Pointer to `QResult` | Tagged `{is_ok, payload}` |

### 1.1 Pass-by-value vs reference semantics

`QValue` structs are passed by value (copied). For scalar types (int, float, bool, null) this is a full copy. For heap types (list, dict, vector, fn, result, str) the struct is copied but the **pointer** is shared — so two `QValue`s can alias the same underlying list/dict. Mutations through one alias are visible through the other.

### 1.2 String ownership

Every string-producing operation (`qv_string`, `q_upper`, `q_trim`, `q_replace`, `+` on strings, etc.) allocates a **fresh GC copy**. Strings are never shared or mutated in place.

### 1.3 Null

`null` is a distinct type, not a zero-value of another type. It is used as the return value for missing keys, out-of-bounds reads, and void-returning builtins.

---

## 2) Type Coercion and Promotion

Quark has **no implicit coercion** except in two narrow cases:

1. **int → float promotion in mixed arithmetic**: When one operand is `int` and the other is `float`, the int is promoted to double before the operation. The result is `float`.

2. **int → float promotion in mixed comparison**: `<`, `<=`, `>`, `>=`, `==`, `!=` promote the int operand to double when comparing int to float.

All other type mismatches are errors (compile-time when type info is available, runtime otherwise).

---

## 3) Truthiness

Truthiness governs `to_bool()` and the internal `q_truthy()` predicate. It does **not** govern `if`/`while`/ternary conditions or `and`/`or` (those require strict `bool` — see §4.4).

| Type | Truthy when |
|------|------------|
| `bool` | `true` |
| `int` | nonzero |
| `float` | nonzero |
| `str` | non-null **and** non-empty |
| `null` | never |
| `list` | non-null **and** non-empty |
| `vector` | size > 0 |
| `dict` | non-null **and** non-empty |
| `fn` | non-null (always true for valid closures) |
| `result` | payload is `ok` (not `err`) |

---

## 4) Operator Semantics

### 4.1 Arithmetic

| Operator | Accepted types | Result type | Notes |
|----------|---------------|-------------|-------|
| `+` | int×int | int | |
| `+` | int×float / float×float | float | Promotion |
| `+` | str×str | str | Concatenation; null string → error |
| `+` | vector×vector / vector×scalar | vector | Element-wise via `q_vec_add` |
| `-` | numeric×numeric | int or float | Same promotion rules as `+` |
| `*` | numeric×numeric | int or float | |
| `/` | numeric×numeric | **always float** | Division by zero → fatal |
| `%` | **int×int only** | int | Modulo by zero → fatal |
| `**` | numeric×numeric | int if both int and result fits; float otherwise | Uses `std::pow`; overflow → float fallback |
| unary `-` | numeric | same type | |

Any other type combination → runtime error.

### 4.2 Comparison

| Operator | Accepted types | Result |
|----------|---------------|--------|
| `<` `<=` `>` `>=` | numeric×numeric | bool (with int→float promotion) |
| `==` `!=` | any×any | bool |

**Equality rules**: Same-type comparisons use natural equality. Cross-type int/float uses double comparison. All other cross-type comparisons return `false` (not an error). Lists, dicts, vectors, and functions are compared by pointer identity, not structural equality.

### 4.3 Logical operators

`and`, `or`, and `!` are **strict-bool** — operands must be `bool`. Using a truthy non-bool value (like an int or string) is a runtime error. This is a deliberate design choice to prevent implicit truthiness bugs.

**`and`/`or` are NOT short-circuit.** Both operands are fully evaluated before the operator runs (they lower to function calls `q_and(left, right)` where both arguments are evaluated). Use nested `if` when short-circuit evaluation is needed.

### 4.4 Conditions (if, while, ternary)

All condition positions (`if`, `elseif`, `while`, ternary `if`) require a **strict bool** value. Non-bool conditions produce a runtime error with the message:

> `<context> condition must be bool, got <type>`

Use an explicit comparison (`x != 0`, `len(s) > 0`) or `to_bool()` to convert.

---

## 5) Operator Precedence

From lowest to highest binding:

| Level | Operators | Associativity |
|-------|-----------|---------------|
| 1 | `=` (assignment) | right |
| 2 | `\|` (pipe) | left |
| 3 | `if`/`else` (ternary) | right |
| 4 | `or` | left |
| 5 | `and` | left |
| 6 | `==` `!=` | left |
| 7 | `<` `<=` `>` `>=` | left |
| 8 | `+` `-` | left |
| 9 | `*` `/` `%` | left |
| 10 | `!` unary`-` | prefix |
| 11 | `**` | right |
| 12 | `.` `[]` `()` (access/call) | left |

---

## 6) Functions and Closures

### 6.1 Unified closure representation

ALL function values — named functions, lambdas, closures — are represented as `QClosure*` at runtime. A QClosure holds a function pointer, a capture count, and a flexible array of capture cells.

Non-capturing named functions simply have `capture_count == 0`. There is no separate "plain function pointer" representation.

### 6.2 Hidden closure parameter

Every generated function signature includes a hidden first parameter `QClosure* _cl`. For direct calls to named functions, the compiler passes `nullptr`. For dynamic/closure calls, the actual closure pointer is passed.

### 6.3 All variables live in QCell

Every variable (locals, parameters, loop variables) is stored in a `QCell*` — a heap-allocated mutable reference cell. Reading a variable dereferences `cell->value`; assignment writes `cell->value`.

This uniform-cell design means **any variable can be captured by a closure** without special handling at the capture site. Multiple closures over the same variable share the same `QCell*`, enabling shared mutable state.

### 6.4 Capture analysis

The analyzer walks each lambda's AST to find **free variables** — identifiers that are:
- Not the lambda's own parameters
- Not defined locally within the lambda body
- Not builtins
- Resolvable in an enclosing scope

These free variables become the lambda's capture list. Nested lambdas transitively capture from the outermost scope that defines the variable.

### 6.5 Named function desugaring

The parser immediately desugars `fn foo(x) -> body` into the assignment `foo = fn(x) -> body`. At the AST level there is no separate "named function" node — only lambdas and assignments.

### 6.6 Default parameters

Default values must be **literals only** (int, float, string, bool, null, or empty list). Required parameters must come before defaulted parameters. Defaults are filled at the **call site** by codegen (not by the callee). The analyzer computes which defaults to inject and stores them in the CallPlan.

### 6.7 Return type annotations

Return types are compile-time only — no runtime cost. The analyzer infers the body's return type and checks it against the annotation. Mismatch produces a compile-time error. At runtime, the function can return any `QValue`.

---

## 7) Pipe Operator

`x | f(a, b)` desugars to `f(x, a, b)` — the piped value becomes the **first argument** to the call on the right.

Pipes are left-associative: `x | f() | g()` means `g(f(x))`.

The right side of `|` must be a function call expression. Piping to a bare identifier (e.g., `x | f`) is a compile-time error — use `x | f()`.

Newlines before `|` are treated as line continuations, allowing multi-line pipe chains.

---

## 8) Pattern Matching (when)

```
when expr:
    ok x  -> handle_ok(x)
    err e -> handle_err(e)
    _     -> fallback
```

`when` inspects a value and branches on its shape:

- **Result patterns**: `ok identifier` or `err identifier` — matches `ok`/`err` results and binds the payload to the identifier.
- **Value patterns**: literal values separated by `or` — matches if the scrutinee equals any listed value.
- **Wildcard**: `_` matches anything.

Each arm's `->` body is a single expression. The entire `when` expression evaluates to the matched arm's result.

The scrutinee is evaluated once. For result patterns, the analyzer checks that the scrutinee is actually a result type (compile-time error otherwise).

---

## 9) Collection Semantics

### 9.1 Lists

- Created with `list [a, b, c]` (keyword required).
- Mutable: `push`, `pop`, `set`, `insert`, `remove`, `reverse` modify in place.
- **Safe reads**: `get(list, idx)` returns `null` on out-of-bounds.
- **Unsafe writes**: `set(list, idx, val)` and `remove(list, idx)` are fatal on out-of-bounds.
- Negative indexing: `-1` is last element, `-2` is second-to-last, etc.
- `slice(list, start, end)` uses half-open `[start, end)` semantics; negative indices and out-of-range values are clamped.
- `range()` produces lists of integers (1, 2, or 3 argument forms).

### 9.2 Dicts

- Created with `dict { key: value }`. Keys in literals are identifiers converted to strings.
- **String keys only** — enforced at runtime.
- Dot syntax on values serves two purposes:
  - **Key access**: `d.key` reads/writes dict entries (`d.key = val` writes, `d.key` reads)
  - **Method dispatch**: `value.method(args)` calls a built-in method for the receiver's type (e.g. `'hello'.upper()`, `xs.push(4)`, `d.get('key')`)
- Missing keys return `null` (not an error).
- Dicts are unordered.

### 9.3 Vectors

Typed columnar arrays with four dtype variants: `f64` (default), `i64`, `bool`, `str`.

- Element-wise arithmetic: `vec + vec`, `vec * scalar`, etc. Operands must have matching lengths (or one is a scalar).
- Division of i64 vectors always produces f64 (same rationale as scalar division).
- Comparison operators produce bool vectors.
- Null support via a per-element null mask; `vec.fillna(value)` replaces nulls.
- `list.to_vector()` converts a homogeneous list; `vec.to_list()` converts back.
- String vectors use offset-based columnar storage internally (not pointers per element).

---

## 10) Module and Import System

### 10.1 Three import forms

| Syntax | Resolution |
|--------|------------|
| `use './path' as alias` | Relative to current file; auto-appends `.qrk` |
| `use 'std/module' as alias` | Stdlib path resolution (see below) |
| `use modulename` | Same-file module reference |

### 10.2 Path resolution

Relative imports (starting with `./`, `../`, `/`, or drive letter) resolve against the importing file's directory.

Stdlib imports (starting with `std/`) resolve by searching: `QUARK_STDLIB_ROOT` env var → walking up from current file looking for a `stdlib/` directory → relative to the compiler executable.

### 10.3 Module structure

Imported files must define at least one top-level `module` block. The loader parses the file, splices its AST into the importing file, and creates an alias binding. Module-qualified calls like `alias.function()` are resolved at analysis time and lowered to direct calls in codegen.

### 10.4 Cycle detection

The loader maintains a set of files currently being resolved. If a file appears twice in the resolution stack, a circular import error is reported with the full dependency chain.

### 10.5 Deduplication

Files are loaded at most once per compilation. Subsequent imports of the same absolute path reuse the previously loaded AST.

---

## 11) Compilation Pipeline

The pipeline has seven stages. Each stage's output feeds the next.

### 11.1 Lexer → Token Stream

Three-pass tokenization:
1. **Raw tokenization**: Produces tokens from source characters. Supports single-quoted and double-quoted strings with escapes (`\n`, `\t`, `\r`, `\\`, `\0`, `\'`, `\"`). Line comments with `//`.
2. **Line-start tracking**: Marks which tokens begin a new line.
3. **Indentation injection**: Converts leading whitespace after `:` or `->` + newline into `INDENT`/`DEDENT` tokens. Tracks an indent stack; mismatched dedent levels produce `ILLEGAL` tokens.

Inside brackets (`()`, `[]`, `{}`), indentation processing is suppressed — newlines are ignored and no INDENT/DEDENT is emitted.

### 11.2 Parser → AST

Recursive-descent parser producing a tree of `TreeNode`s. Uses Pratt parsing for expressions with the precedence table from §5. Named function definitions are immediately desugared to assignments (§6.5).

Key parser decisions:
- `fn name(...)` at statement level → named function (desugared to assignment)
- `fn(...)` in expression → lambda
- `list [...]` → list literal (keyword required, avoids ambiguity with indexing)
- Dict keys in literals are bare identifiers, interpreted as string keys

### 11.3 Loader → Merged AST

Resolves all `use` statements by loading, parsing, and splicing external files into the main AST. See §10 for details.

### 11.4 Analyzer → Annotated AST + Metadata

Two-pass semantic analysis:
1. **Predeclaration pass**: Scans top-level statements to register all named functions and function-binding assignments. This enables forward references — a function can call another function defined later in the file.
2. **Full analysis pass**: Walks the entire AST, building scopes, type-checking expressions, computing closure captures, and generating CallPlans.

The analyzer produces three metadata outputs consumed by later stages:
- **CallPlans**: Per-call-site metadata (dispatch mode, arity, defaults to inject)
- **Captures**: Per-lambda map of captured variable names
- **Return validation**: Per-function declared-vs-inferred return type checks

#### Type checking policy (Knowability Rule)

When either the parameter type or argument type is unknown or `any`, the check is **deferred to runtime**. Only when both types are statically known does the analyzer enforce assignability. This means Quark programs may contain latent type errors that only surface at runtime for dynamically-typed code paths.

#### Assignability rules

- `any` is assignable to/from everything
- `null` is assignable to reference types (list, dict, fn, result)
- `int` is assignable to `float` (promotion)
- Collections are covariant in element type
- All other combinations require exact match

### 11.5 Invariants → Validated

Pre-codegen checks that verify CallPlans are well-formed and return type annotations are consistent. Acts as a safety net between analysis and code generation.

### 11.6 Codegen → C++17 Source

Walks the annotated AST and emits C++17 code using the runtime headers. Key codegen decisions:

- **Naming**: User identifiers are prefixed with `quark_` (e.g., `quark_x`, `quark_main`). Runtime builtins use `q_` prefix.
- **Variables**: All variables are `QCell*` (see §6.3). Reads emit `quark_x->value`, writes emit `quark_x->value = expr`.
- **Function calls**: Lowered according to the CallPlan's dispatch mode:
  - `DispatchBuiltin` → `q_print(arg)` (direct C++ call)
  - `DispatchDirect` → `quark_foo(nullptr, arg)` (known function, no closure)
  - `DispatchClosure` → `q_call1(val, arg)` (dynamic dispatch through QClosure)
- **Pipe**: `x | f(a)` → emits `f` call with `x` prepended to args
- **Default injection**: Missing trailing args filled from DefaultNodes in the CallPlan
- **Lambdas**: Emitted as top-level C++ functions (`_lambda1`, `_lambda2`, ...) with closure allocation at the capture site
- **for loops**: Lowered to index-based iteration (`q_iter_get` with incrementing counter)
- **if/while**: Conditions wrapped in `q_condition_bool()` for strict-bool enforcement
- **when**: Lowered to a chain of if/else with result payload extraction

### 11.7 C++ Compiler → Binary

The generated C++ is compiled with clang++ (preferred) or g++ using:
- `-std=c++17 -O3 -march=x86-64-v3` (on amd64)
- `-DQUARK_USE_GC` + Boehm GC include/link flags
- Optional `-flto` for link-time optimization

---

## 12) Memory Management

All heap allocation goes through Boehm GC when `QUARK_USE_GC` is defined (the default). The three allocation paths:

| Path | Usage | GC behaviour |
|------|-------|-------------|
| `q_malloc(n)` / `q_new<T>(args...)` | Objects containing pointers (closures, cells, results, containers) | Scanned for pointers to other GC objects |
| `q_malloc_atomic(n)` / `q_strdup(s)` | Pointer-free data (strings, numeric buffers) | Not scanned (no embedded pointers) |
| `q_allocator<T>` (STL allocator) | Internal buffers of `std::vector`, `std::unordered_map` | Scanned/atomic depending on element type |

There is **no manual free**. The GC reclaims unreachable objects automatically. Destructors are not called by the GC (this is a known Boehm GC behaviour), but since all sub-allocations also use the GC, no manual cleanup is needed for correctness.

`q_gc_init()` is emitted as the first statement in the generated `main()`.

---

## 13) Error Model

Quark has two error categories: compile-time diagnostics and runtime panics.

### 13.1 Compile-time errors

Reported by the parser, analyzer, or invariant checker. Multiple errors can be accumulated in a single compilation. The compiler does **not** stop at the first error — it continues to find as many issues as possible. Categories:

| Category | Examples |
|----------|---------|
| **Syntax** | Missing tokens, malformed expressions, invalid default values |
| **Scope** | Undefined identifiers, duplicate definitions, break/continue outside loops |
| **Type mismatch** | Wrong argument types, arithmetic on non-numeric, non-bool conditions |
| **Arity** | Too few or too many arguments to functions or builtins |
| **Module** | Undefined modules, missing symbols, circular imports, missing files |
| **Annotation** | Return type vs inferred type mismatch, default value vs parameter type mismatch |
| **Result safety** | Assigning result to non-result variable without unwrap/when |

The parser stops after 10 errors to avoid cascading noise.

### 13.2 Runtime panics

All runtime errors are **fatal** — they print to stderr and call `exit(1)` (or `abort()` for unwrap failures). There are no exceptions and no recovery mechanism.

| Condition | Behaviour |
|-----------|-----------|
| Type mismatch in operator | Fatal with type names in message |
| Division/modulo by zero | Fatal |
| `pop()` on empty list | Fatal |
| `set()`/`remove()` out of bounds | Fatal |
| `sqrt()` of negative number | Fatal |
| `unwrap()` on `err` value | Fatal (prints error payload, calls `abort()`) |
| `unwrap()` on non-result | Fatal (calls `abort()`) |
| Calling a non-function value | Fatal |
| More than 12 arguments in dynamic call | Fatal |
| Vector size mismatch in arithmetic | Fatal |
| `min()`/`max()` on empty vector | Fatal |
| Non-string dict key | Fatal |
| Dot-key access on non-dict (static key read/write) | Fatal |
| Dot method call with unknown method name for type | Compile-time error |
| Member access on null | Fatal |
| Non-bool condition (if/while/ternary) | Fatal |
| Non-bool operand to and/or/! | Fatal |

### 13.3 Safe operations (return null instead of crashing)

| Operation | Behaviour on failure |
|-----------|---------------------|
| `get(list, oob_index)` | Returns `null` |
| `get(string, oob_index)` | Returns `null` |
| `dict.missing_key` / `dict.get(missing)` | Returns `null` |
| `q_result_value()` on err | Returns `null` |
| `q_result_error()` on ok | Returns `null` |

---

## 14) ok/err Result Type

### 14.1 Construction

`ok value` and `err value` create result values. The result wraps an arbitrary `QValue` payload and a boolean tag indicating success or failure.

### 14.2 Extraction

Three ways to extract the payload:

1. **`unwrap(result)`** — Returns the ok payload. If the result is err, **panics** (prints the error payload via `to_str` and calls `abort()`). If the argument is not a result at all, also panics.

2. **`when` pattern matching** — Safely destructure:
   ```
   when result_val:
       ok x  -> use(x)
       err e -> handle(e)
   ```

3. **Direct predicates**: `is_ok(result)`, `is_err(result)` return bool.

### 14.3 Assignment restriction

The analyzer prevents assigning a `result`-typed value to a variable with a non-result type annotation. The error message guides the user to use `unwrap()` or `when`.

---

## 15) Type Annotation System

### 15.1 Available types

`int`, `float`, `str`, `bool`, `list`, `dict`, `vector`, `result`, `any`, `void`

There are **no generic types**. You cannot write `list[int]` or `dict[str, int]`.

### 15.2 Parameter annotations

```
fn foo(x: int, y: float = 0.0) -> x + y
```

Annotations are checked at compile time when both parameter and argument types are known. When either is `any` or unknown, the check is skipped (deferred to runtime).

If a parameter has both a type annotation and a default value, the default's type must be assignable to the annotated type.

### 15.3 Return type annotations

```
fn foo(x: int) int -> x + 1
```

The annotated type is checked against the analyzer's inferred return type for the function body. If the body has branches returning different types (e.g., an if with int in one branch and null in another), the inferred type is a union; the check succeeds if all non-void alternatives are assignable to the annotation.

### 15.4 Variable type annotations

```
x: int = 42
r: result = ok 1
```

The annotated type constrains future assignments. Assigning a value of incompatible type is a compile-time error.

---

## 16) for Loop Semantics

```
for item in iterable:
    body
```

The loop variable `item` is scoped to the loop body. Supported iterables:

| Iterable type | Iteration semantics |
|---------------|-------------------|
| `list` | Iterates elements in order |
| `str` | Iterates single-character strings |
| `vector` | Iterates scalar values (f64→float, i64→int, bool→bool, str→str) |

The loop is lowered to index-based iteration: a counter increments from 0 to `len(iterable)-1`, and each iteration calls `q_iter_get(iterable, index)` to extract the element.

`break` exits the innermost loop. `continue` skips to the next iteration. Both are compile-time errors if used outside a loop.

---

## 17) Key Design Decisions

| Decision | Rationale |
|----------|-----------|
| Division always returns float | Prevents silent truncation (`5/2` = `2.5`, not `2`) |
| Modulo is int-only | Avoids floating-point modulo surprises |
| Strict-bool conditions and logical ops | Prevents truthiness bugs; forces explicit intent |
| and/or are not short-circuit | Simplifies compilation (function calls); use `if` for short-circuit |
| Safe reads, fatal writes | `get()` returning null is convenient; bad `set()` is always a bug |
| All variables in QCell | Uniform closure capture without special-casing |
| Named functions desugar to assignments | One representation for all function values |
| Forward references via predeclaration | Two-pass analysis allows calling functions defined later |
| Dict keys are strings only | Simplifies hashing and serialisation |
| No generic type annotations | Keeps the type system simple; runtime is dynamically typed |
| Result assignment restrictions | Guides users toward explicit error handling |
| Fatal runtime errors (no exceptions) | Simple, predictable failure mode; no hidden control flow |
