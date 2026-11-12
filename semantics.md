# Quark Language Semantics

This document covers the runtime semantics, error behaviour, and design decisions of the Quark language. It complements **grammar.md** (syntax/grammar), **stdlib.md** (built-in function reference), and **architecture.md** (implementation internals).

---

## 1) Value Model

Every runtime value carries one of thirteen types:

| Type | Payload | Notes |
|------|---------|-------|
| `int` | 64-bit signed integer | |
| `float` | 64-bit IEEE double | |
| `str` | Immutable string | Every string operation produces a fresh copy |
| `bool` | Boolean | |
| `null` | (none) | Distinct type, not a zero-value of another type |
| `list` | Ordered dynamic array | Heterogeneous elements |
| `vec` | Typed columnar array | Homogeneous dtype: f64, i64, bool, or str |
| `dict` | String-keyed map | Unordered |
| `fn` | Function value | All functions are closures (even non-capturing ones) |
| `result` | Tagged ok/err wrapper | Holds an arbitrary payload |
| `resource` | Opaque external handle | Generational slot-based; currently used for file I/O |
| `struct` | Fixed set of named fields | Declared with `struct`; immutable; `type()` returns the struct's name (grammar.md §9.1) |
| `tbl` | Columnar table | Runtime representation only; no syntax creates one yet (`tables_v0_1_spec.md`) |

### 1.1 Pass-by-value vs reference semantics

Values are passed by copy. For scalar types (int, float, bool, null) this is a full copy. For heap types (list, dict, vec, fn, result, str, struct) the value is copied but the **underlying data is shared** — so two variables can alias the same list/dict. Mutations through one alias are visible through the other.

### 1.2 String ownership

Every string-producing operation allocates a **fresh copy**. Strings are never shared or mutated in place.

### 1.3 Null

`null` is a distinct type, not a zero-value of another type. It is used as the return value for missing keys read with `.get`, out-of-bounds reads, and void-returning builtins.

### 1.4 Resources

Resources are opaque handles to external state managed outside the Quark value system. The runtime uses a generational slot-based registry: each resource handle carries a slot index, generation counter, and kind tag. Operations on a closed or stale handle produce a runtime error. Every handle has the single type `resource`: `type()` returns `"resource"`, and `to_str`/`print` show the kind, as in `<resource file>`.

Resources are currently used exclusively by the `std/io` stdlib module for file handles. User code cannot construct resources directly — they are created by native functions (e.g., `io.open()`) and consumed by others (e.g., `io.read()`, `io.close()`).

---

## 2) Type Coercion and Promotion

Quark has **no implicit coercion** except in three narrow cases:

1. **int → float promotion in mixed arithmetic**: When one operand is `int` and the other is `float`, the int is promoted before the operation. The result is `float`.

2. **int → float promotion in mixed comparison**: `<`, `<=`, `>`, `>=`, `==`, `!=` promote the int operand when comparing int to float.

3. **int → float widening at `float` parameters and struct fields**: an `int` argument passed to a parameter annotated `float`, of a Quark function or an `extern fn`, is converted to `float`, and so is an `int` stored in a `float` struct field (literal value or default). The reverse is not done: a `float` passed to an `int` parameter or field is an error (§13.2).

All other type mismatches are errors (compile-time when type info is available, runtime otherwise).

---

## 3) Truthiness

Truthiness governs all condition positions (`if`, `while`, ternary), the `and`/`or`/`!` operators, and the `to_bool()` builtin. Most values can be used where a boolean is expected and are converted using these rules. Results and vectors are the exception: they have no truthiness (see below).

| Type | Truthy when |
|------|------------|
| `bool` | `true` |
| `int` | nonzero |
| `float` | nonzero |
| `str` | non-empty |
| `null` | never |
| `list` | non-empty |
| `vec` | **error** — use `all(v)`, `any(v)` or `len(v) > 0` |
| `dict` | non-empty |
| `fn` | always true (for valid closures) |
| `result` | **error** — use `is_ok(r)`, `is_err(r)` or `when` |
| `resource` | handle is alive (not closed or stale) |
| `struct` | always true |

**Results and vectors are rejected as conditions.** Treating an `err` result as false would silently discard the error. A vector comparison such as `v == w` yields a `vec[bool]`, so its emptiness says nothing about whether the elements matched. Using either in a condition, with `!`, or as an `and`/`or` operand is a compile-time error when the type is known and a runtime error otherwise.

---

## 4) Operator Semantics

### 4.1 Arithmetic

| Operator | Accepted types | Result type | Notes |
|----------|---------------|-------------|-------|
| `+` | int×int | int | Overflow → fatal |
| `+` | int×float / float×float | float | Promotion |
| `+` | str×str | str | Concatenation |
| `+` | vec×vec / vec×scalar | vec | Element-wise |
| `-` | numeric×numeric | int or float | Same promotion rules as `+`; int overflow → fatal |
| `*` | numeric×numeric | int or float | int overflow → fatal |
| `/` | numeric×numeric | **always float** | Division by zero → fatal |
| `-` `*` `/` | vec×vec / vec×scalar | vec | Element-wise, like `+` |
| `%` | **int×int only** | int | Modulo by zero → fatal |
| `**` | numeric×numeric | int if both int; float otherwise | Exact for ints; int overflow → fatal; negative int exponent truncates (`2 ** -1` = `0`). No vector form (nor for `%`) |
| unary `-` | numeric | same type | Negating the minimum int → fatal |

Any other type combination → runtime error.

**Integer overflow is always fatal.** Ints are 64-bit and never wrap silently. This holds on every execution path: boxed values, compiler-lowered native locals, and `vec[i64]` arithmetic. `LLONG_MIN % -1` is `0`.

### 4.2 Comparison

| Operator | Accepted types | Result |
|----------|---------------|--------|
| `<` `<=` `>` `>=` | numeric×numeric | bool (with int→float promotion) |
| `<` `<=` `>` `>=` | str×str | bool, by byte order (`'Zebra' < 'apple'`), as `list.sort()` orders strings |
| `<` `<=` `>` `>=` | vec×vec / vec×scalar | `vec[bool]`, element-wise |
| `==` `!=` | any×any | bool (`vec[bool]` for vectors, see below) |

Any other pair, such as `'a' < 1` or `true < false`, is a runtime error ("operator '<' expects two numbers or two strings"). The checker lets mixed operands through, like other cross-type comparisons, and the runtime rejects them.

**Equality rules**: Same-type comparisons use natural equality. Cross-type int/float uses double comparison. All other cross-type comparisons return `false` (not an error).

Type-specific equality:

| Type | `==` semantics |
|------|---------------|
| `int` | Value equality |
| `float` | IEEE 754 equality (`NaN != NaN`) |
| `bool` | Value equality |
| `str` | Character-by-character content equality |
| `null` | Always equal to `null` |
| `resource` | Same slot, generation, and kind |
| `list` | Structural: same length and pairwise-equal elements |
| `dict` | Structural: same key set and equal values |
| `result` | Same tag (`ok`/`err`) and equal payloads |
| `fn` | Identity: the same function value |
| `vec` | Element-wise comparison producing `vec[bool]`. A vector nested inside a list, dict or result compares as a whole: same dtype, length, null positions and values |
| `struct` | Compile-time error; compare fields explicitly |

### 4.3 Logical operators

`and`, `or`, and `!` accept any type except results and vectors, and use truthiness (§3) to evaluate operands.

**`and`/`or` are short-circuit** (Python semantics):
- `x and y` — evaluates `x`; if falsy, returns `x` without evaluating `y`. Otherwise evaluates and returns `y`.
- `x or y` — evaluates `x`; if truthy, returns `x` without evaluating `y`. Otherwise evaluates and returns `y`.
- `!x` — returns `true` if `x` is falsy, `false` if truthy. Always returns `bool`.

This enables guard patterns: `if len(lst) > 0 and lst.get(0) == 5:` is safe because the second operand is never evaluated when the list is empty.

### 4.4 Conditions (if, while, ternary)

All condition positions (`if`, `elseif`, `while`, ternary `if`) accept any type except results and vectors, and use truthiness (§3) to determine the branch. No explicit conversion is needed:

```quark
if my_list:          // truthy if non-empty
if count:            // truthy if nonzero
if name:             // truthy if non-empty string
if to_bool(x):       // also works (explicit)
```

---

## 5) Operator Precedence

From lowest to highest binding:

| Level | Operators | Associativity |
|-------|-----------|---------------|
| 1 | `=` (assignment) | none — `a = b = 1` is a compile-time error |
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

`**` binds tighter than unary minus, as in math and Python: `-2 ** 2` is `-(2 ** 2)`, which is `-4`; write `(-2) ** 2` for `4`. The right operand of `**` may be negated (`2 ** -1`).

`ok` and `err` take everything that binds tighter than `=` as their operand, so `ok 5 == 1` is `ok (5 == 1)`. Parenthesize a result to compare it: `(ok 5) == (ok 5)`.

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
print(counter())  // 1
print(counter())  // 2
```

### 6.3 Nested closures

Nested lambdas transitively capture from the outermost scope that defines the variable. A lambda inside a lambda can capture variables from any enclosing scope.

### 6.4 Named function desugaring

The parser immediately desugars `fn foo(x) -> body` into the assignment `foo = fn(x) -> body`. At the AST level there is no separate "named function" node — only lambdas and assignments.

### 6.5 Default parameters

Default values must be **literals only** (int, float, string, bool, null, or empty list). Required parameters must come before defaulted parameters. Defaults are filled at the **call site** when the compiler knows the function being called — omitted trailing arguments are replaced with their default values before the function is called. A call through a function value it can't see through fills them in at runtime instead (§6.7). Either way a default is built fresh for every call, so an empty-list default is never shared between calls.

### 6.6 Return type annotations

Return types are checked at compile time — no runtime cost. The analyzer infers the body's return type and checks it against the annotation. Mismatch produces a compile-time error. A body whose type can't be inferred (it returns an `any` value) is checked at runtime only in a fully annotated function, which is compiled to native C++ and converts its result with the checked `q_as_*` helpers (an `int` result widens to a `float` return type); other functions don't check it yet.

### 6.7 Calls through function values

A call through a parameter or variable holding a function is checked in two layers.

**At compile time (call shapes).** While analyzing a function body, the compiler records every call made through one of the function's parameters (`k(1)`, `x | k()`): the number of arguments, their types, and which of them are the function's own parameters passed on unchanged (`fn apply(k, v) -> k(v)`). Passing a parameter on to another function's parameter that is called inherits that function's calls, through any number of levels. At each call of the function, an argument whose signature is known (a named function, a lambda, an `extern fn`, a module member) is checked against those calls: too few or too many arguments, or an argument type that conflicts with an annotated parameter, is a compile-time error naming the call inside the body (`argument 1 of 'apply' is called with 1 argument (line 2), but 'h' takes 2 arguments`). A known non-function (`apply(5)`) is rejected the same way; `null` is allowed, as an optional callback the body may test before calling. A parameter that the body reassigns isn't tracked. Calling a list element whose type is one of several function signatures is rejected when none of them accepts the argument count.

**At runtime (the backstop).** Every function value carries a static descriptor (`QFnInfo` in `types/closure.hpp`: its name, minimum and maximum argument count, and how to build its defaults). A call whose callee the compiler couldn't resolve, because the value came from `any`, a list, a struct field or native code, checks the argument count (`function 'h' expects 2 arguments but got 1`, at the call's location) and fills in omitted defaults. A call with an argument for every parameter takes a direct path, with one comparison. Argument types are then checked by the callee on entry, as for any call (§13.2).

---

## 7) Pipe Operator

`x | f(a, b)` desugars to `f(x, a, b)` — the piped value becomes the **first argument** to the call on the right.

Pipes are left-associative: `x | f() | g()` means `g(f(x))`.

The right side of `|` must be a function call expression. Piping to a bare identifier (e.g., `x | f`) is a compile-time error — use `x | f()`.

A line that starts with `|` continues the previous line's expression, allowing multi-line pipe chains. The `|` line must be at the same indentation as the line it continues; indenting it is a syntax error.

```quark
fn double(x) -> x * 2
fn inc(x) -> x + 1

y = 5
| double()
| inc()
print(y)  // 11
```

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
- **Value patterns**: literal values (numbers, including negative ones, strings, booleans and `null`) separated by `or` — matches if the scrutinee equals any listed value. Variables, calls and other expressions can't be patterns (compile-time error); compare with `if` instead.
- **Wildcard**: `_` matches anything.

Each arm's `->` body is a single expression. The entire `when` expression evaluates to the matched arm's result, so it can be used anywhere an expression ends a line, including the right side of an assignment:

```
size = when n:
    0 -> 'none'
    1 or 2 -> 'few'
    _ -> 'many'
```

A `when` is **exhaustive** when it has a `_` arm, or both an `ok` and an `err` arm. A non-exhaustive `when` evaluates to `null` when no arm matches, so its type includes `void` and it cannot initialize a variable annotated with a concrete type such as `str`.

The scrutinee is evaluated once. For result patterns, the analyzer checks that the scrutinee is actually a result type (compile-time error otherwise).

---

## 9) Collection Semantics

### 9.1 Lists

Language semantics:

- Created with `list [a, b, c]` (keyword required).
- Lists are heterogeneous and mutable.
- Assignment is shallow for list values (aliases share the same underlying list data).
- Negative indexing is supported where index-based methods are accepted (`-1` last item, `-2` second last, ...).

Method/index behavior:

| Operation | Semantics |
|-----------|-----------|
| `xs.get(i)` | Safe read. Returns `null` when out of bounds. |
| `xs.set(i, v)` | In-place write. Fatal on out of bounds. |
| `xs.insert(i, v)` | In-place insert. Index is clamped to `[0, len]` after negative-index adjustment. |
| `xs.remove(i)` | Removes and returns element at index. Fatal on out of bounds. |
| `xs.pop()` | Removes and returns last element. Fatal on empty list. |
| `xs.slice(start, end)` | Returns new list with half-open `[start, end)`. Negative indices supported; bounds clamped. |
| `xs.reverse()` | In-place reversal; returns the mutated list value. |
| `xs.sort()`, `xs.sort_by(key)` | In-place stable ascending sort (by `key(item)` for `sort_by`); returns the mutated list value. Keys must all be numbers, all strings or all bools; other combinations are a runtime error. |
| `xs.concat(ys)` | Returns a new list containing shallow-copied element values from both operands. |
| `xs.enumerate()` | Returns `list[dict{index: int, value: any}]`. |

### 9.2 Dicts

- Created with `dict { key: value }`; keys in a literal are identifiers.
- Keys are strings. `.get` and `.set` convert a non-string key to its string form, so `d.set(1, 2)` stores the key `'1'` and `d.get(1)` reads it back.
- `d.key` names a key in the source, so reading a missing key with it is a runtime error: `dict has no key 'nmae'; use .get('nmae') if the key may be missing`. `d.get(key)` returns `null` for a missing key; use it when a key may be absent.

### 9.3 Ranges

`range()` and lists:

- `range(end)`, `range(start, end)`, `range(start, end, step)` return `list[int]`.
- Start/end/step accept numeric values; float arguments are truncated toward zero before iteration.
- `step = 0` is a runtime error.

### 9.4 Vectors

**Vectors are immutable.** Every vector operation, including `fillna` and `astype`, returns a new vector and never changes its inputs.

Comparisons between `vec[i64]` and floats, or between `vec[i64]` and `vec[f64]`, promote to float. `astype('i64')` on a float that is NaN or outside the i64 range is a runtime error.

Implementation details:

- Storage follows the Apache Arrow columnar format, so buffers can be shared with Arrow-compatible libraries without copying:
    - `f64`: contiguous `double` values
    - `i64`: contiguous `int64_t` values
    - `bool`: bit-packed, least significant bit first
    - `str`: `int32` offsets plus UTF-8 bytes (at most 2 GiB of string data per vector)
- Nulls are an Arrow validity bitmap: a set bit means the element is valid. Vectors without nulls have no bitmap.
- All buffers are 64-byte aligned and allocated on the garbage-collected heap.

---

## 10) Module and Import System

### 10.1 Three import forms

| Syntax | Resolution |
|--------|------------|
| `incl './path' as alias` | Relative to current file; auto-appends `.qrk` |
| `incl 'std/module' as alias` | Stdlib path resolution (see below) |
| `incl modulename` | Same-file module reference |

### 10.2 Path resolution

Relative imports (starting with `./`, `../`, `/`, or drive letter) resolve against the importing file's directory.

Stdlib imports (starting with `std/`) resolve by searching: `QUARK_STDLIB_ROOT` env var → walking up from current file looking for a `stdlib/` directory → relative to the compiler executable.

### 10.3 Module structure

Imported files must define at least one top-level `module` block. The loader parses the file, splices its AST into the importing file, and creates an alias binding. Module-qualified calls like `alias.function()` are resolved at analysis time and lowered to direct calls in codegen.

Each module's members have their own namespace. Two modules may define the same name (`io.read` and `csv.read`), and a user definition or variable with a member's name doesn't affect the member (`open = 5` leaves `io.open` working). Internally the loader renames member `x` of module `m` to `m__x`; messages show the original name. Without an alias, `incl m` makes `m`'s members usable as bare names; a bare name that two unaliased modules define, or that the program also defines at top level, is a `QK-LOAD-001` conflict.

Module members can be called (`io.exists(p)`, `p | io.exists()`), including from inside functions, but not yet used as values (`f = io.exists` is an error).

### 10.4 Cycle detection

The loader maintains a set of files currently being resolved. If a file appears twice in the resolution stack, a circular import error is reported with the full dependency chain.

### 10.5 Deduplication

Files are loaded at most once per compilation. Subsequent imports of the same absolute path reuse the previously loaded AST.

### 10.6 Extern functions (native C++ interop)

Quark programs can call native C++ functions via `extern fn` declarations:

```quark
extern './math_ext.hpp'
extern fn fast_sqrt(x: float) float as 'my_sqrt'
```

The `extern 'file.hpp'` directive `#include`s a C++ header, resolved relative to the declaring file (`std/...` paths resolve in the stdlib). The `extern fn` declaration maps a Quark-visible name to a C++ symbol. Parameters and return types must be explicitly annotated — the compiler generates unboxing/boxing code at call sites.

Extern functions can also be declared as methods on existing types:

```quark
extern fn list.shuffle(seed: int) list as 'q_list_shuffle'
```

This registers a new method on the `list` type, callable via `my_list.shuffle(42)`.

Parameter types map to C++ types: `int`, `float`, `bool` and `str` become `int64_t`, `double`, `bool` and `const char*`; `list`, `dict` and `vec` become `QList*`, `QDict*` and `QVector*`; `fn` becomes `QClosure*`; every other annotation (`any`, `result`, `resource`, struct names) is passed as a boxed `QValue`. An `int` argument to a `float` parameter is widened; a `float` argument to an `int` parameter is a runtime error. Return types use the same mapping; the result is automatically boxed back to `QValue`.

**Callbacks.** An `fn` parameter arrives as `QClosure*`. Native code calls it with `qext::call(f, args...)` (up to three `QValue` arguments; the result is a `QValue`) and can return it, or store it in a Quark value, with `qext::box(f)`. Any Quark function value can be passed: a lambda, a capturing closure, a named function, an `extern fn` or a module member. `qext::call` checks the argument count and fills in defaults like a call in Quark (§6.7), so a callback called with too few arguments is a runtime error, not a missing argument.

See `runtime/include/quark/ext/api.hpp` for the `qext` namespace available to extension authors.

#### Native libraries

A file can link a native library and add a C++ include search path with one-line directives (see `ffi_v0_1_spec.md` P1):

```quark
extern include_path 'include'   // -I, relative to this file
extern link 'openblas'          // -lopenblas
extern 'blas_glue.hpp'          // the glue header, #included
```

- `link` and `include_path` are contextual words after `extern`, not reserved keywords.
- Directives must be at the top level of a file. They apply to the whole program, including directives in imported modules and the stdlib. Duplicates are removed.
- Paths are relative to the declaring file and should point at files shipped with the module. Where a library is installed on a given machine is passed on the command line (`--lib-path`, `--include-path`) or through `QUARK_LIB_PATH` / `QUARK_INCLUDE_PATH`.
- Link order: a module's libraries come before those of the modules it imports; a library requested more than once keeps its last position.
- Nothing may follow the directive's string; words there are reserved for future per-library options.
- A library that can't be found is reported before linking (`QK-FFI-002`), naming the requesting file and the directories searched. A missing `include_path` directory is `QK-FFI-001`, and a C++ error inside a glue header is `QK-FFI-003`.

---

## 11) Error Model

Quark has three diagnostic/failure categories: compile-time errors, compile-time warnings, and runtime panics.

### 11.1 Compile-time errors

Reported by the parser, analyzer, or invariant checker. Multiple errors can be accumulated in a single compilation. The compiler does **not** stop at the first error — it continues to find as many issues as possible. Categories:

| Category | Examples |
|----------|---------|
| **Syntax** | Missing tokens, malformed expressions, invalid default values |
| **Scope** | Undefined identifiers, duplicate definitions, break/continue outside loops |
| **Type mismatch** | Wrong argument types, arithmetic on non-numeric, invalid collection/index usage |
| **Arity** | Too few or too many arguments to functions or builtins |
| **Module** | Undefined modules, missing symbols, circular imports, missing files |
| **Annotation** | Return type vs inferred type mismatch, default value vs parameter type mismatch |
| **Result safety** | Assigning result to non-result variable without unwrap/when |

The parser stops after 10 errors to avoid cascading noise.

### 11.2 Compile-time warnings

Warnings are emitted by semantic analysis for suspicious but still legal constructs. They are displayed to the user but are **non-fatal**.

| Category | Examples |
|----------|----------|
| Suspicious assignments | Assigning a value whose type does not match an inferred dict value type |

Warnings use `warning[QK-CHECK-002] (check): ...` formatting.

### 11.3 Runtime panics

All runtime errors are **fatal** — they print to stderr and exit. There are no exceptions and no recovery mechanism.

| Condition | Behaviour |
|-----------|-----------|
| Type mismatch in operator | Fatal with type names in message |
| Division/modulo by zero | Fatal |
| Integer overflow in `+`, `-`, `*`, `**`, unary `-` | Fatal |
| Result or vector used as a condition (dynamic) | Fatal |
| `==` nesting deeper than 512 levels (cyclic containers) | Fatal |
| `.pop()` on empty list | Fatal |
| `.set()`/`.remove()` out of bounds | Fatal |
| `sqrt()` of negative number | Fatal |
| `unwrap()` on `err` value | Fatal (prints error payload) |
| `unwrap()` on non-result | Fatal |
| Calling a non-function value | Fatal |
| More than 12 arguments in dynamic call | Fatal |
| Vector size mismatch in arithmetic | Fatal |
| `min()`/`max()` on empty vector | Fatal |
| Dot-key access on non-dict when the type is unknown at compile time | Fatal |
| Member access on null | Fatal |
| Operation on closed/stale resource handle | Fatal |
| Resource kind mismatch (e.g., passing non-file to file op) | Fatal |
| `for` loop over non-iterable (not list, string, or vector) | Fatal |

### 11.4 Safe operations (return null instead of crashing)

| Operation | Behaviour on failure |
|-----------|---------------------|
| `.get(oob_index)` or `xs[oob_index]` on list | Returns `null` |
| `s[oob_index]` on string | Returns `null` |
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

Variable, parameter and struct field annotations: `int`, `float`, `str`, `bool`, `list`, `dict`, `vec`, `result`, `resource`, `fn`, `any`, and struct names. `fn` accepts any function value (named function, lambda, `extern fn`, module member). It has no signature; how the body calls the parameter is checked against the function passed instead (§6.7).

Return type annotations accept the same list.

`null` and `void` are not annotations. `void` appears only in inferred types, such as a non-exhaustive `when`.

There are **no generic types**. You cannot write `list[int]` or `dict[str, int]`.

### 13.2 Parameter annotations

```
fn foo(x: int, y: float = 0.0) -> x + y
```

Each annotated parameter is enforced, whether or not the function's other parameters or its return type are annotated, and with the same rules for Quark functions and `extern fn`:

- When the argument's type is known at compile time, it is checked then (`argument 1 of 'foo' expects int, got str`), at no runtime cost.
- When it isn't (`any`, for example the result of `unwrap` or of an unannotated function), it is checked on entry to the function, and a mismatch is a runtime error naming the parameter: `argument 'x' of 'foo' expects int, got str`. Calls through a function value, pipes and default arguments are covered too.
- An `int` passed to a `float` parameter is converted to `float`.
- Reference-type annotations (`list`, `dict`, `vec`, `result`, `resource`, `fn`, struct names) also accept `null`, as at compile time.

Unannotated parameters accept any value. Inside the body, annotated `int`, `float` and `bool` parameters are native C++ values, so arithmetic on them is unboxed. Fully annotated functions are compiled to native C++ functions, and calls with arguments already known to have the right types pay no check.

If a parameter has both a type annotation and a default value, the default's type must be assignable to the annotated type.

Struct fields follow the same rules. A field value whose type is known at compile time is checked then (`cannot assign value of type 'str' to field 'x' of type 'int'`). Otherwise it is checked when the struct literal is built, so a field never holds a value of the wrong type: `field 'x' of P expects int, got str`. An `int` going into a `float` field, including a default, is converted to `float`, and reference-typed fields accept `null`.

### 13.3 Return type annotations

```
fn foo(x: int) int -> x + 1
```

The annotated type is checked against the analyzer's inferred return type for the function body. If the body has branches returning different types, the inferred type is a union; the check succeeds if all non-void alternatives are assignable to the annotation.

### 13.4 Variable type annotations

```
x: int = 42
r: result = ok 1
v: any = 1
```

The annotated type constrains future assignments. Assigning a value of incompatible type is a compile-time error.

Builtin function names (`sum`, `len`, `max`, ...) are bindings that can't be reassigned, so `sum = 0` is a compile-time error, also inside a function body. A parameter may reuse a builtin's name (`fn f(sum) -> sum`).

The `any` annotation is significant: it forces the variable to use boxed `QValue` storage even when the initial value is a scalar. This allows the variable to hold values of different types over its lifetime:

```
v: any = 1       // stored as QValue, not as a raw long long
v = 'changed'    // legal — v can be reassigned to any type
```

Without an annotation, a variable's type is fixed by its first assignment, whatever that type is. Assigning a value of another type later is a compile-time error: `x = 1` then `x = 2.5`, `x = list [1]` then `x = 5`, and `x = null` then `x = 1` are all rejected. Only variables whose type is `any` (annotated, or inferred from a value of unknown type) can change type. Separately, the compiler may store a fixed-type `int`, `float` or `bool` variable as a native scalar (e.g. `long long`).

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
| `vec` | Iterates scalar values (f64→float, i64→int, bool→bool, str→str) |

`break` exits the innermost loop. `continue` skips to the next iteration. Both are compile-time errors if used outside a loop.

---

## 15) Dot Syntax

Dot syntax (`.`) serves five distinct purposes depending on context:

| Form | Purpose | Example |
|------|---------|---------|
| `d.key` | Dict key read | `user.name` |
| `d.key = val` | Dict key write | `user.age = 31` |
| `s.field` | Struct field read (structs are immutable) | `customer.name` |
| `value.method(args)` | Method dispatch | `'hello'.upper()`, `xs.push(4)` |
| `alias.fn(args)` | Module-qualified call | `math.square(9)` |

For method calls, the receiver's type determines which method is resolved. See stdlib.md for available methods per type.

When the receiver's type isn't known at compile time (for example the result of `unwrap`, or a parameter without an annotation), the method is resolved at runtime from the value's type: `unwrap(io.read(f, n)).split('\n')` works without an annotation. It is still a compile-time error if no type has the method or if no type's version accepts that many arguments, and a runtime error (`type 'int' has no method 'upper'`) if the value's type lacks it.

---

## 16) Key Design Decisions

| Decision | Rationale |
|----------|-----------|
| Division always returns float | Prevents silent truncation (`5/2` = `2.5`, not `2`) |
| Modulo is int-only | Avoids floating-point modulo surprises |
| Truthiness in conditions and logical ops | Familiar Python-like behaviour for scalars and collections in `if`/`while`/`and`/`or` |
| Results and vectors have no truthiness | `if r:` would silently discard errors; `if v == w:` would test emptiness, not equality |
| Short-circuit and/or (Python semantics) | Enables guard patterns (`if x and x.foo:`); returns operand values, not bool |
| Safe reads, fatal writes | `.get()` returning null is convenient; bad `.set()` is always a bug |
| Named functions desugar to assignments | One representation for all function values |
| Forward references via predeclaration | Two-pass analysis lets a function body call functions defined later in the file (mutual recursion works). Code that runs immediately must come after the definition: a statement that directly calls a function defined further down in the same block is a compile-time error ("'f' is called before its definition on line N"). A call made indirectly, through another function, before the definition runs fails at runtime |
| Dict keys are strings only | Simplifies hashing and serialisation; `.get`/`.set` convert other keys to strings |
| No generic type annotations | Keeps the type system simple; runtime is dynamically typed |
| Result assignment restrictions | Guides users toward explicit error handling |
| Fatal runtime errors (no exceptions) | Simple, predictable failure mode; no hidden control flow |
| Lists, dicts and results compare structurally | `a == a` and value-equal containers are equal, as users expect; functions compare by identity |
| Integer overflow is fatal | Signed overflow is undefined in C++; a fatal error is deterministic and keeps data pipelines from silently producing wrong numbers |
| `any` annotation forces boxed storage | Allows a variable to change type across assignments; scalar tiering is an optimization the annotation opts out of |
| Resources use generational handles | Detects use-after-close without relying on GC finalization; stale handles fail deterministically |
