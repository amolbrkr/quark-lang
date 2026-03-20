# Quark Language Semantics

This document covers the runtime semantics, error behaviour, and design decisions of the Quark language. It complements **grammar.md** (syntax/grammar), **stdlib.md** (built-in function reference), and **architecture.md** (implementation internals).

---

## 1) Value Model

Every runtime value carries one of ten types:

| Type | Payload | Notes |
|------|---------|-------|
| `int` | 64-bit signed integer | |
| `float` | 64-bit IEEE double | |
| `str` | Immutable string | Every string operation produces a fresh copy |
| `bool` | Boolean | |
| `null` | (none) | Distinct type, not a zero-value of another type |
| `list` | Ordered dynamic array | Heterogeneous elements |
| `vector` | Typed columnar array | Homogeneous dtype: f64, i64, bool, or str |
| `dict` | String-keyed map | Unordered |
| `fn` | Function value | All functions are closures (even non-capturing ones) |
| `result` | Tagged ok/err wrapper | Holds an arbitrary payload |

### 1.1 Pass-by-value vs reference semantics

Values are passed by copy. For scalar types (int, float, bool, null) this is a full copy. For heap types (list, dict, vector, fn, result, str) the value is copied but the **underlying data is shared** — so two variables can alias the same list/dict. Mutations through one alias are visible through the other.

### 1.2 String ownership

Every string-producing operation allocates a **fresh copy**. Strings are never shared or mutated in place.

### 1.3 Null

`null` is a distinct type, not a zero-value of another type. It is used as the return value for missing keys, out-of-bounds reads, and void-returning builtins.

---

## 2) Type Coercion and Promotion

Quark has **no implicit coercion** except in two narrow cases:

1. **int → float promotion in mixed arithmetic**: When one operand is `int` and the other is `float`, the int is promoted before the operation. The result is `float`.

2. **int → float promotion in mixed comparison**: `<`, `<=`, `>`, `>=`, `==`, `!=` promote the int operand when comparing int to float.

All other type mismatches are errors (compile-time when type info is available, runtime otherwise).

---

## 3) Truthiness

Truthiness governs `to_bool()`. It does **not** govern `if`/`while`/ternary conditions or `and`/`or` (those require strict `bool` — see §4.4).

| Type | Truthy when |
|------|------------|
| `bool` | `true` |
| `int` | nonzero |
| `float` | nonzero |
| `str` | non-empty |
| `null` | never |
| `list` | non-empty |
| `vector` | size > 0 |
| `dict` | non-empty |
| `fn` | always true (for valid closures) |
| `result` | payload is `ok` (not `err`) |

---

## 4) Operator Semantics

### 4.1 Arithmetic

| Operator | Accepted types | Result type | Notes |
|----------|---------------|-------------|-------|
| `+` | int×int | int | |
| `+` | int×float / float×float | float | Promotion |
| `+` | str×str | str | Concatenation |
| `+` | vector×vector / vector×scalar | vector | Element-wise |
| `-` | numeric×numeric | int or float | Same promotion rules as `+` |
| `*` | numeric×numeric | int or float | |
| `/` | numeric×numeric | **always float** | Division by zero → fatal |
| `%` | **int×int only** | int | Modulo by zero → fatal |
| `**` | numeric×numeric | int if both int and result fits; float otherwise | Overflow → float fallback |
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

**`and`/`or` are NOT short-circuit.** Both operands are fully evaluated before the operator runs. Use nested `if` when short-circuit evaluation is needed.

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

### 6.1 All functions are closures

ALL function values — named functions, lambdas, closures — share the same representation at runtime. A non-capturing named function is simply a closure with zero captures. There is no separate "plain function" representation.

### 6.2 Mutable capture by reference

Captured variables are shared by reference between the enclosing scope and all closures that capture them. Assigning to a captured variable in one closure is visible in all others, and in the original scope.

```quark
fn make_counter() ->
    count = 0
    fn next() ->
        count = count + 1
        count
    next

counter = make_counter()
println(counter())  // 1
println(counter())  // 2
```

### 6.3 Nested closures

Nested lambdas transitively capture from the outermost scope that defines the variable. A lambda inside a lambda can capture variables from any enclosing scope.

### 6.4 Named function desugaring

The parser immediately desugars `fn foo(x) -> body` into the assignment `foo = fn(x) -> body`. At the AST level there is no separate "named function" node — only lambdas and assignments.

### 6.5 Default parameters

Default values must be **literals only** (int, float, string, bool, null, or empty list). Required parameters must come before defaulted parameters. Defaults are filled at the **call site** — omitted trailing arguments are replaced with their default values before the function is called.

### 6.6 Return type annotations

Return types are compile-time only — no runtime cost. The analyzer infers the body's return type and checks it against the annotation. Mismatch produces a compile-time error. At runtime, the function can return any value.

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
- Mutable: `.push()`, `.pop()`, `.set()`, `.insert()`, `.remove()`, `.reverse()` modify in place.
- **Safe reads**: `.get(idx)` returns `null` on out-of-bounds.
- **Unsafe writes**: `.set(idx, val)` and `.remove(idx)` are fatal on out-of-bounds.
- Negative indexing: `-1` is last element, `-2` is second-to-last, etc.
- `.slice(start, end)` uses half-open `[start, end)` semantics; negative indices and out-of-range values are clamped.
- `range()` produces lists of integers (1, 2, or 3 argument forms).

### 9.2 Dicts

- Created with `dict { key: value }`. Keys in literals are identifiers converted to strings.
- **String keys only** — enforced at runtime.
- Dot syntax on values serves two purposes:
  - **Key access**: `d.key` reads/writes dict entries
  - **Method dispatch**: `d.get('key')`, `d.set('key', val)`, `d.keys()`, `d.values()`, `d.items()`
- Missing keys return `null` (not an error).
- Dicts are unordered.

### 9.3 Vectors

Typed columnar arrays with four dtype variants: `f64` (default), `i64`, `bool`, `str`.

- Element-wise arithmetic: `vec + vec`, `vec * scalar`, etc. Operands must have matching lengths (or one is a scalar).
- Division of i64 vectors always produces f64 (same rationale as scalar division).
- Comparison operators produce bool vectors.
- Null support via a per-element null mask; `.fillna(value)` replaces nulls.
- `list.to_vector()` converts a homogeneous list; `vec.to_list()` converts back.

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

## 11) Error Model

Quark has two error categories: compile-time diagnostics and runtime panics.

### 11.1 Compile-time errors

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

### 11.2 Runtime panics

All runtime errors are **fatal** — they print to stderr and exit. There are no exceptions and no recovery mechanism.

| Condition | Behaviour |
|-----------|-----------|
| Type mismatch in operator | Fatal with type names in message |
| Division/modulo by zero | Fatal |
| `.pop()` on empty list | Fatal |
| `.set()`/`.remove()` out of bounds | Fatal |
| `sqrt()` of negative number | Fatal |
| `unwrap()` on `err` value | Fatal (prints error payload) |
| `unwrap()` on non-result | Fatal |
| Calling a non-function value | Fatal |
| More than 12 arguments in dynamic call | Fatal |
| Vector size mismatch in arithmetic | Fatal |
| `min()`/`max()` on empty vector | Fatal |
| Non-string dict key | Fatal |
| Dot-key access on non-dict (static key read/write) | Fatal |
| Member access on null | Fatal |
| Non-bool condition (if/while/ternary) | Fatal |
| Non-bool operand to and/or/! | Fatal |

### 11.3 Safe operations (return null instead of crashing)

| Operation | Behaviour on failure |
|-----------|---------------------|
| `.get(oob_index)` on list | Returns `null` |
| `.get(oob_index)` on string | Returns `null` |
| `dict.missing_key` / `dict.get(missing)` | Returns `null` |

---

## 12) ok/err Result Type

### 12.1 Construction

`ok value` and `err value` create result values. The result wraps an arbitrary payload and a boolean tag indicating success or failure.

### 12.2 Extraction

Three ways to extract the payload:

1. **`unwrap(result)`** — Returns the ok payload. If the result is err, **panics**. If the argument is not a result, also panics.

2. **`when` pattern matching** — Safely destructure:
   ```
   when result_val:
       ok x  -> use(x)
       err e -> handle(e)
   ```

3. **Direct predicates**: `is_ok(result)`, `is_err(result)` return bool.

### 12.3 Assignment restriction

The analyzer prevents assigning a `result`-typed value to a variable with a non-result type annotation. The error message guides the user to use `unwrap()` or `when`.

---

## 13) Type Annotation System

### 13.1 Available types

`int`, `float`, `str`, `bool`, `list`, `dict`, `vector`, `result`, `any`, `void`

There are **no generic types**. You cannot write `list[int]` or `dict[str, int]`.

### 13.2 Parameter annotations

```
fn foo(x: int, y: float = 0.0) -> x + y
```

Annotations are checked at compile time when both parameter and argument types are known. When either is `any` or unknown, the check is skipped (deferred to runtime).

If a parameter has both a type annotation and a default value, the default's type must be assignable to the annotated type.

### 13.3 Return type annotations

```
fn foo(x: int) int -> x + 1
```

The annotated type is checked against the analyzer's inferred return type for the function body. If the body has branches returning different types, the inferred type is a union; the check succeeds if all non-void alternatives are assignable to the annotation.

### 13.4 Variable type annotations

```
x: int = 42
r: result = ok 1
```

The annotated type constrains future assignments. Assigning a value of incompatible type is a compile-time error.

---

## 14) for Loop Semantics

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

`break` exits the innermost loop. `continue` skips to the next iteration. Both are compile-time errors if used outside a loop.

---

## 15) Dot Syntax

Dot syntax (`.`) serves three distinct purposes depending on context:

| Form | Purpose | Example |
|------|---------|---------|
| `d.key` | Dict key read | `user.name` |
| `d.key = val` | Dict key write | `user.age = 31` |
| `value.method(args)` | Method dispatch | `'hello'.upper()`, `xs.push(4)` |
| `alias.fn(args)` | Module-qualified call | `math.square(9)` |

For method calls, the receiver's type determines which method is resolved. See stdlib.md for available methods per type.

---

## 16) Key Design Decisions

| Decision | Rationale |
|----------|-----------|
| Division always returns float | Prevents silent truncation (`5/2` = `2.5`, not `2`) |
| Modulo is int-only | Avoids floating-point modulo surprises |
| Strict-bool conditions and logical ops | Prevents truthiness bugs; forces explicit intent |
| and/or are not short-circuit | Simplifies compilation; use `if` for short-circuit |
| Safe reads, fatal writes | `.get()` returning null is convenient; bad `.set()` is always a bug |
| Named functions desugar to assignments | One representation for all function values |
| Forward references via predeclaration | Two-pass analysis allows calling functions defined later |
| Dict keys are strings only | Simplifies hashing and serialisation |
| No generic type annotations | Keeps the type system simple; runtime is dynamically typed |
| Result assignment restrictions | Guides users toward explicit error handling |
| Fatal runtime errors (no exceptions) | Simple, predictable failure mode; no hidden control flow |
