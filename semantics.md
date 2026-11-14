# Quark Language Semantics

How Quark programs behave: values, types, scoping, functions, errors. Syntax and operator precedence are in **grammar.md**, builtins and stdlib modules in **stdlib.md**, compiler and runtime internals in **architecture.md**. Current bugs are listed in **known_issues.md**, not here.

## Contents

1. [Value Model](#1-value-model)
2. [Type Coercion and Promotion](#2-type-coercion-and-promotion)
3. [Truthiness](#3-truthiness)
4. [Operator Semantics](#4-operator-semantics)
5. [Variables and Scope](#5-variables-and-scope)
6. [Functions and Closures](#6-functions-and-closures)
7. [Pipe Operator](#7-pipe-operator)
8. [Pattern Matching (when)](#8-pattern-matching-when)
9. [Collection Semantics](#9-collection-semantics)
10. [Modules and Imports](#10-modules-and-imports) ([extern functions](#108-extern-functions-native-c-interop))
11. [Error Model](#11-error-model)
12. [ok/err Result Type](#12-okerr-result-type)
13. [Type Annotations](#13-type-annotations)
14. [for Loops](#14-for-loops)
15. [Dot Syntax](#15-dot-syntax)
16. [Key Design Decisions](#16-key-design-decisions)

---

## 1) Value Model

Every runtime value has one of twelve types. `type(v)` returns the name in the first column (`vec[i64]`, `vec[f64]`, `vec[bool]` or `vec[str]` for a vector, the struct's name for a struct).

| Type | Payload | Notes |
|------|---------|-------|
| `int` | 64-bit signed integer | Overflow is a runtime error (§4.1) |
| `float` | 64-bit IEEE double | |
| `str` | Immutable byte string | `len`, indexing and `for` work on bytes, not characters |
| `bool` | Boolean | |
| `null` | (none) | A distinct type, not a zero value of another type (§1.3) |
| `list` | Ordered, mutable array | Its element type is fixed when it is created (§9.1) |
| `vec` | Immutable typed column | dtype `i64`, `f64`, `bool` or `str` (§9.4) |
| `dict` | String-keyed map | Unordered; its value type is fixed when it is created (§9.2) |
| `fn` | Function value | Equality is identity (§6.1) |
| `result` | `ok` or `err` with a payload | Has no truthiness (§12) |
| `resource` | Opaque native handle | Created only by native code, such as `io::open` (§1.4) |
| `struct` | Fixed set of named fields | Declared with `struct`; immutable (grammar.md §9.1) |

Tables (`tbl`) are not a Quark type yet: the runtime has the table engine and CSV reader, and the compiler side is planned in `specs/tables_v0_1_spec.md` (the type is written `tbl[Employee]`, D10).

### 1.1 Aliasing

Assignment and argument passing copy the value. For `int`, `float`, `bool` and `null` that is the whole value. For the other types the copy refers to the same data, so two names can alias one list or dict, and a change through one name is visible through the other. Strings and vectors can't be changed. Structs and results can't be changed either, but a list or dict inside one is shared: after `b = a`, `b.xs.push(2)` changes `a.xs`.

### 1.2 Strings

A string is a sequence of bytes. String operations return new strings; nothing changes a string in place. Source files are read as bytes, so UTF-8 text works, but `len('é')` is `2` and `for c in 'é'` yields two one-byte strings. A string can't contain a NUL byte (grammar.md §3.3).

### 1.3 Null

`null` is the value of:

- `xs.get(i)` past the end of a list, `d.get(k)` for a missing key and `v.get(i)` for a missing vector entry, so all three have type `any`;
- an `if` without `else`, or a `when` with no matching arm, used as a value (its inferred type includes `void`);
- builtins that return nothing, such as `print`.

Only `any`, struct and `resource` annotations accept `null` (§13.2), so the result of a `.get` stored into a typed slot is checked: `n: int = xs.get(9)` is a runtime error (`variable 'n' is declared int but got null`) when `xs` has no item 9. The typed reads `xs[i]`, `s[i]`, `v[i]` and `d.key` never give `null` for something missing; they are runtime errors (§9).

### 1.4 Resources

A resource is a handle to native state, such as an open file. The runtime keeps handles in a generational registry (slot, generation, kind), so a closed or stale handle is detected: using one is a runtime error (`io.read() expects an open file (resource is closed)`). Every handle has the type `resource`; `print` shows its kind, as in `<resource file>`. User code can't create resources. Native functions create and consume them (`io::open`, `io::read`, `io::close` in `std/io`).

---

## 2) Type Coercion and Promotion

Quark converts types implicitly in three cases only:

1. **int → float in mixed arithmetic.** When one operand is `int` and the other `float`, the int is promoted and the result is `float`.
2. **An int and a float compare exactly.** `<`, `<=`, `>`, `>=`, `==` and `!=` compare the two mathematical values. An int that a float can hold exactly compares as that float (`1 == 1.0`, `2 < 2.5`). A larger int is not rounded first, so `9007199254740993 == 9007199254740992.0` is false and `9007199254740993 > 9007199254740992.0` is true. NaN compares as in IEEE 754. Arithmetic still promotes (rule 1): `9007199254740993 + 0.0` is the float `9007199254740992.0`.
3. **int → float where a `float` is stored.** An `int` passed to a `float` parameter (of a Quark function or an `extern fn`), stored in a `float` variable, struct field, list element or dict value, or returned by a function annotated `float` becomes a `float`: `x: float = 3` holds `3.0`. A `float` is never converted to `int`; passing one to an `int` parameter is an error (§13.2).

Nothing is converted below the top level: a `list[int]` is not a `list[float]` (§9.1). There is no string coercion: `'n = ' + 5` is an error; write `'n = ' + to_str(5)`.

All other mismatches are errors: at compile time when the types are known, at runtime otherwise.

---

## 3) Truthiness

Truthiness applies to every condition (`if`, `elseif`, `while`, the ternary `if`), to the `and`/`or`/`!` operators, and to `to_bool()`.

| Type | Truthy when |
|------|------------|
| `bool` | `true` |
| `int` | nonzero |
| `float` | nonzero |
| `str` | non-empty |
| `null` | never |
| `list` | non-empty |
| `dict` | non-empty |
| `fn` | always |
| `resource` | the handle is open (not closed or stale) |
| `struct` | always |
| `vec` | **error**: use `all(v)`, `any(v)` or `len(v) > 0` |
| `tbl` | **error**: test a column, such as `len(t.col) > 0` |
| `result` | **error**: use `is_ok(r)`, `is_err(r)` or `when` |

**Results, vectors and tables are rejected as conditions.** Treating an `err` as false would silently discard the error. A vector comparison such as `v == w` yields a `vec[bool]`, so its emptiness says nothing about whether the elements matched, and a table holds columns of such values. Using one as a condition, with `!`, or as an `and`/`or` operand is a compile-time error when the type is known and a runtime error otherwise.

In a query's verb expressions (§9.5) there is no truthiness: `and`, `or`, `!` and a ternary's condition take bools.

---

## 4) Operator Semantics

Precedence and associativity are in grammar.md §10.3.

### 4.1 Arithmetic

| Operator | Accepted types | Result type | Notes |
|----------|---------------|-------------|-------|
| `+` | int×int | int | Overflow is a runtime error |
| `+` | int×float, float×float | float | Promotion |
| `+` | str×str | str | Concatenation |
| `-` `*` | numeric×numeric | int if both int, else float | Int overflow is a runtime error |
| `/` | numeric×numeric | **always float** | Division by zero is a runtime error |
| `+` `-` `*` `/` | vec×vec, vec×scalar | vec | Element-wise; different lengths are a runtime error |
| `%` | **int×int only** | int | Modulo by zero is a runtime error; no vector form |
| `**` | numeric×numeric | int if both int, else float | Exact for ints; int overflow is a runtime error; a negative int exponent truncates (`2 ** -1` is `0`); no vector form |
| unary `-` | numeric | same type | Negating the smallest int is a runtime error |

Any other combination is an error (`operator '%' requires integer operands, got float and int`).

**Integer overflow is always an error.** Ints never wrap, on every path: boxed values, native locals the compiler lowers, and `vec[i64]` arithmetic. `LLONG_MIN % -1` is `0`.

### 4.2 Comparison

| Operator | Accepted types | Result |
|----------|---------------|--------|
| `<` `<=` `>` `>=` | numeric×numeric | bool; an int and a float compare exactly (§2) |
| `<` `<=` `>` `>=` | str×str | bool, by byte order (`'Zebra' < 'apple'`), the order `list.sort()` uses |
| `<` `<=` `>` `>=` | vec×vec, vec×scalar | `vec[bool]`, element-wise |
| `==` `!=` | any×any | bool (`vec[bool]` for vectors) |

Ordering any other pair, such as `'a' < 1` or `true < false`, is a runtime error (`operator '<' expects two numbers or two strings`).

**Equality.** Values of the same type compare by the rules below; an int and a float compare exactly (§2); any other pair of different types is `false`, not an error (`1 == 'a'` is `false`).

| Type | `==` means |
|------|---------------|
| `int`, `bool` | same value |
| `float` | IEEE 754 equality (`NaN != NaN`) |
| `str` | same bytes |
| `null` | always equal to `null` |
| `resource` | same slot, generation and kind |
| `list` | same length and pairwise-equal elements |
| `dict` | same keys and equal values |
| `result` | same tag (`ok`/`err`) and equal payloads |
| `fn` | the same function value (`f == f`; two lambdas with the same body are different) |
| `vec` | element-wise, giving `vec[bool]`. A vector nested in a list, dict or result compares as a whole: same dtype, length, null positions and values |
| `struct` | compile-time error, and so is `==` on a list or dict that holds structs; compare fields explicitly. Structs reached through untyped values (`any`, a `list` of any) are a runtime error when two of them are compared (`'==' is not defined for struct values`) |
| `tbl` | error (compile time, or runtime through `any`); compare columns, `t.col == u.col` |

Comparing a `result` with `null` is a compile-time error, since a result is never null. `==` that nests deeper than 512 levels (a container that contains itself) is a runtime error.

### 4.3 Logical operators

`and`, `or` and `!` accept any type except results and vectors, and use truthiness (§3).

`and` and `or` short-circuit and return one of their operands, so the result is a `bool` only when both operands are (`id(0) or id(5)` is `5`):
- `x and y` evaluates `x`; if it is falsy, the result is `x` and `y` is not evaluated. Otherwise the result is `y`.
- `x or y` evaluates `x`; if it is truthy, the result is `x` and `y` is not evaluated. Otherwise the result is `y`.
- `!x` is `true` if `x` is falsy and `false` otherwise; it always returns a `bool`.

So `if n != 0 and total / n > 2:` never divides by zero, and `false and f().upper()` never calls `f`. The ternary is the same: `x if c else y` evaluates only the branch it takes.

### 4.4 Conditions

`if`, `elseif`, `while` and the ternary `x if c else y` accept any type except results and vectors, and branch on truthiness:

```quark
if my_list:          // non-empty
if count:            // nonzero
if name:             // non-empty string
```

### 4.5 Evaluation order

Every part of an expression runs once, when the expression needs it, left to right as written:

- A call evaluates the function value (for a call through a variable), then a method's receiver, then the arguments in order: `f().contains(g())` calls `f` before `g`, and `t('a') + t('b')` calls `t('a')` first.
- An assignment evaluates the target's object and index before the value: `obj().k = v()` and `xs[i()] = v()` call `obj` and `i` before `v`. The value is computed once, also when the assignment is a function's last statement.
- A struct literal evaluates its field values in the order written, whatever the declaration order.
- `and`, `or` and the ternary evaluate their right side or untaken branch only when needed (§4.3).
- `elseif` conditions are tested in order, each only when the ones before it were false; a `while` condition is evaluated again before every iteration, method calls included.
- A `when` evaluates its value once, then tests the arms in order.
- An assignment is a statement, so it can't run in the middle of another expression (grammar.md §10.1). A variable assigned by a `while` condition is visible after the loop, holding the value of the last test.

---

## 5) Variables and Scope

**Creation and scope.** A variable is created by its first assignment and belongs to the block where that happens: a function body, an `if`/`elseif`/`else` branch, or a loop body. It is not visible after the block ends:

```quark
if ready:
    msg = 'go'
print(msg)           // error: undefined identifier 'msg'
```

To use a value after the block, assign the variable before it (`msg = ''`). A `for` loop variable belongs to the loop body.

**Assigning an existing name assigns that variable**, whether it was created in this block or an enclosing one, including from inside a function body: with `x = 0` at the top level, `fn f() -> x = 10` changes the top-level `x`. Closures share variables by reference (§6.2). A parameter is always a new local.

**No forward references to variables.** A function body sees only variables assigned above it. `fn` declarations are hoisted (§6.1), so a call can run a function before a variable it uses is assigned; that is a compile-time error (`'report' uses 'total', which is assigned on line 9, after this call`), or, when the call goes through a function value, a runtime error (`'total' is used before it is assigned`).

**A variable's type never changes.** It is the variable's annotation, or else the type of its first value. Assigning a value of another type later is a compile-time error: `x = 1` then `x = 2.5`, `x = list [1]` then `x = 5`, and `x = null` then `x = 1` are all rejected. An `int` assigned to a `float` variable is widened: `x = 1.5`, then `x = 2`, `print(x)` prints `2.0`.

- A variable whose first value has no known type (`x = id('a')`, with `id` unannotated) stays untyped, whatever is assigned to it later; `x = 5` doesn't make it an `int`. Convert it once to get a typed variable: `n: int = x` (checked when stored).
- A variable holding a function keeps that function's signature, including its return type: `g = fn() -> 5`, then `g = fn() -> 'a'`, is a compile-time error. Declare `g: fn = ...` to hold any function; a call through it returns an untyped value.
- `v: any = 1` holds a value of any type and can be reassigned to anything (§13.4).

**Names that can't be assigned:** builtin function names (`sum = 0` is an error, also inside a function body), names declared with `fn` (§6.1), and every name of an imported file (§10.4). A parameter may reuse a builtin's name (`fn f(sum) -> sum`). Names containing `__` are reserved for the compiler, so `my__x = 1` is a load error.

---

## 6) Functions and Closures

### 6.1 Declarations and function values

A function returns the value of the last statement of its body: an expression's value, the assigned value for an assignment, the taken branch's value for an `if`/`else` or `when`, and `null` for a loop or an `if` without `else` that doesn't run. There is no `return` statement.

`fn name(params) -> body` declares a **fixed, hoisted** function:

- Every `fn` of a scope exists before the scope's first statement runs, so calls work in any order and mutual recursion needs no forward declaration.
- A name is declared once per scope. It can't be reassigned (`f = ...` after `fn f` is an error) or reused by a nested `fn` while it is visible.
- A declaration that captures no variables (directly or through another declaration it calls) compiles to a plain C++ function, called directly. One that captures variables is a closure in a binding that never changes.

A **function value** is any function used as a value: a declared function passed or stored (`f = sq`, `apply(sq, 2)`), a lambda (`fn(x) -> x * 2`), a closure returned from a call, an `extern fn` or a module member (`f = io::exists`). All function values have the type `fn` and the same runtime representation (a closure with a static descriptor of its name, arity and defaults), and they compare by identity. A lambda's body is a single expression.

A variable holding a function (`g = fn(x) -> x + 1`) is an ordinary variable: it can be reassigned, but keeps its signature (§5).

### 6.2 Capture by reference

A closure shares the variables it captures with the scope that defines them and with every other closure that captures them. An assignment through any of them is visible to all.

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

A lambda can capture variables from any enclosing scope; a lambda nested in a lambda captures through the intermediate one.

### 6.4 Default parameters

Default values must be literals: an int, float, string, bool, `null`, a negated number, an empty `list []` or an empty `dict {}` (a new list or dict for every call). Required parameters must come before defaulted ones. A default must fit the parameter's annotation, so `xs: list = null` is an error (write `xs: list = list []`).

When the compiler knows which function is called, omitted trailing arguments are filled in at the call site. A call through a function value fills them in at runtime (§6.5). Either way a default is built fresh for every call, so an empty-list default is never shared between calls. A pipe counts its input as the first argument: `5 | add_n()` fills the default of `add_n`'s second parameter.

Return annotations are described in §13.3.

### 6.5 Calls through function values

A call through a parameter or variable holding a function is checked in two layers.

**At compile time (call shapes).** While analyzing a function body, the compiler records every call made through one of the function's parameters (`k(1)`, `x | k()`): the number of arguments, their types, and which of them are the function's own parameters passed on unchanged (`fn apply(k, v) -> k(v)`). Passing a parameter on to another function's parameter that is called inherits that function's calls, through any number of levels. At each call of the function, an argument whose signature is known (a declared function, a lambda, an `extern fn`, a module member) is checked against those calls: too few or too many arguments, or an argument type that conflicts with an annotated parameter, is a compile-time error naming the call inside the body (`argument 1 of 'apply' is called with 1 argument (line 2), but 'h' takes 2 arguments`). A known non-function (`apply(5)`) is rejected the same way; `null` is allowed, as an optional callback the body may test before calling. A parameter that the body reassigns isn't tracked. Calling a list element whose type is one of several function signatures is rejected when none of them accepts the argument count.

**At runtime (the backstop).** A call whose callee the compiler couldn't resolve, because the value came from `any`, a list, a struct field or native code, checks the argument count against the function's descriptor (`function 'h' expects 2 arguments but got 1`, at the call's location) and fills in omitted defaults. Calling a value that isn't a function is a runtime error (`attempted to call a non-function value`). A dynamic call takes at most 12 arguments. Argument types are then checked by the callee on entry, as for any call (§13.2).

---

## 7) Pipe Operator

`x | f(a, b)` is the call `f(x, a, b)`: the piped value becomes the first argument. Pipes are left-associative, so `x | f() | g()` is `g(f(x))`. A pipe into a method call passes the value as the method's first argument and keeps the receiver: `x | s.replace('b')` is `s.replace(x, 'b')`. A qualified name works as a target: `p | io::exists()`.

A pipe is exactly the call it stands for and is checked like one, so `x | d.f()` is the same error as `d.f(x)` (a dict member can't be called with method syntax). Arity errors add "(including piped input)". The target must be a call; `x | f` is a parse error (grammar.md §10.4).

A line that starts with `|` continues the previous line's expression, at the same indentation:

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

`when` evaluates its scrutinee once and tries the arms in order:

- **Result patterns**: `ok name` or `err name` match an `ok`/`err` result and bind its payload (`ok _` binds nothing). Using them on a value that isn't a result is a compile-time error.
- **Value patterns**: literals (numbers, including negative ones, strings, booleans and `null`) separated by `or`; the arm matches if the scrutinee equals one of them. Variables, calls and other expressions can't be patterns (compile-time error); compare with `if` instead.
- **Wildcard**: `_` matches anything.

A value pattern compares with `==`, and `==` on a vec compares elementwise, so a value pattern on a vec is a compile-time error, or a runtime error when the compiler couldn't know the value is a vec (`when can't match a vec`); test `all(v == x)` or `any(v == x)` with `if` instead.

Each arm's body is a single expression, and the `when` evaluates to the matched arm's value, so it can end a line, as in an assignment:

```
size = when n:
    0 -> 'none'
    1 or 2 -> 'few'
    _ -> 'many'
```

A `when` is **exhaustive** when it has a `_` arm, or both an `ok` and an `err` arm. A non-exhaustive `when` evaluates to `null` when no arm matches, so its type includes `void`, and it can't initialize a variable annotated with a concrete type such as `str`.

---

## 9) Collection Semantics

### 9.1 Lists

- Created with `list [a, b, c]` (the keyword is required). Lists are mutable.
- A list's **element type is fixed when it is created**: `list [1, 2]` is a `list[int]` for good, also through every other name for it (`b = a`), so `xs.push('s')`, `xs[0] = 's'` and `xs.set(0, 's')` on it are compile-time errors. A list of mixed literals (`list [1, 'a']`) holds those types. A list that holds values of any type is created under a `list` annotation: `xs: list = list [1, 2]` (also a literal passed to a `list` parameter or stored in a `list` struct field). An `int` written into a list of floats becomes a `float`.
- An empty `list []` takes its element type from where it is first stored (`xs = list [1]`, then `xs = list []`, stays a `list[int]`) or else from its first write (`out = list []`, then `out.push(i)` makes it a `list[int]`); passed to an untyped parameter before that, it holds values of any type.
- Code whose types the compiler can't see (an untyped function's parameter, a value from `any`, native code) is checked when it writes: `push() can't add a str to a list of int`. Lists made by builtins have element types too: `range` gives a `list[int]`, `split` and `keys` a `list[str]`, `values` the dict's value type, `slice` and `concat` the receiver's. `concat` checks the other list's elements against the receiver's element type.
- Containers nested in a list are checked by their own element type: a `list[list[int]]` takes a `list[int]` but not a `list[str]`, a `list[float]` or a list annotated as holding anything. A function stored in a list keeps its parameters but not its return type (a call through it returns an untyped value).
- Indexes may be negative where an index is accepted (`-1` is the last item).
- `xs[i]` is the typed read: an index out of range is a runtime error (`list index 5 out of range (list length 2)`), so `xs[i]` on a `list[int]` is always an `int`. `xs.get(i)` is the probing read: `null` past the end, so its type is `any`; convert a result you know is there with a typed declaration (`n: int = xs.get(i)`, checked once). `xs.set(i, v)`, `xs[i] = v` and `xs.remove(i)` out of range and `xs.pop()` on an empty list are runtime errors. `insert` clamps its index to `[0, len]`, and `slice` clamps its bounds.
- A list keeps its length during a `for` loop over it (§14).
- `sort`, `sort_by` and `reverse` change the list in place and return it. Sorting is stable and ascending; the keys must be all numbers, all strings or all bools.

The methods are listed in stdlib.md.

### 9.2 Dicts

- Created with `dict { key: value }`; keys in a literal are identifiers.
- A dict's **value type is fixed when it is created**, like a list's element type: `d = dict { a: 1 }`, then `d.b = 'x'` or `d.set('b', 'x')` is a compile-time error, and a write from untyped code is a runtime error (`can't store a str under key 'k' of a dict of int`). `d: dict = dict { a: 1 }` holds values of any type; an empty `dict {}` takes the type of its first value.
- Keys are strings. `.get` and `.set` take a `str` key: another type is a compile-time error when the compiler knows it (`argument 1 of 'set' expects str, got int`), otherwise a runtime error (`set(): dict keys are str, got int; convert it with to_str()`). A key is never converted, so `1` and `'1'` can't silently become the same key; convert explicitly (`d.set(to_str(id), v)`).
- `d.key` names a key in the source, so reading a missing key with it is a runtime error: `dict has no key 'nmae'; use .get('nmae') if the key may be missing`. `d.get(key)` returns `null` for a missing key, so its type is `any`, whatever the dict's value type.
- `d['key']` is a compile-time error (use `d.key` or `d.get('key')`). A dict isn't iterable; loop over `d.keys()`, `d.values()` or `d.items()`.

### 9.3 Ranges

`range(end)`, `range(start, end)` and `range(start, end, step)` return a `list[int]`, from `start` (default 0) up to but not including `end`; a negative `step` counts down. `step = 0` is a runtime error. The arguments are ints: a float is a compile-time error when the compiler knows the type (`range(n / 2)`: `argument 1 of 'range' expects int, got float`; convert with `floor`, `ceil`, `round` or `to_int`), otherwise a runtime error. Ranges near the int64 limits end without wrapping around. In `for i in range(...)`, assigning `i` in the body doesn't change which values the loop visits.

### 9.4 Vectors

- Created with `vec [a, b, c]`, `xs.to_vec()`, or by vector operations. A literal's elements must all be ints, all floats, all bools or all strings (`vec [1, 2.5]` is an error), with no `null`.
- **Vectors are immutable.** Every vector operation, including `fillna` and `astype`, returns a new vector.
- `v[i]` and `v.get(i)` out of range are runtime errors. `v[mask]`, with a `vec[bool]` mask of the same length, selects elements.
- Comparisons between `vec[i64]` and floats, or `vec[i64]` and `vec[f64]`, use the same exact int/float rule (§2). `astype('i64')` of a NaN or out-of-range float is a runtime error.
- A vector can hold missing entries (for example from `to_vec()` of a list with `null` in it). Operations on the whole vector keep them (arithmetic, comparisons) or skip them (reductions); `fillna` replaces them and `drop_na` removes them. Taking one out as a single value (`v[i]`, `for x in v`, `to_list()`, `enumerate()`) is a runtime error naming `fillna` and `drop_na`, so a missing entry never reads as a number. `v.get(i)` returns `null` for one and has type `any`.
- Storage follows the Apache Arrow columnar layout, so buffers can be shared with Arrow-compatible libraries without copying (architecture.md, `QVector`).

### 9.5 Tables

A table (`tbl`) is an immutable, in-memory, columnar set of rows: one vec per column, all of one length, in the Arrow layout. Its design and decisions are in `specs/tables_v0_1_spec.md` and `specs/tables_v0_2_spec.md`. The column-wise table literal is deferred (`specs/tables_v0_2_spec.md` §6.1).

**Types.** `tbl[R]` is a table whose rows are the struct `R`: its columns are `R`'s fields, in field order, with their types, which must be `int`, `float`, `bool` or `str` (`struct S can't be a table row: field 'xs' is list[any]`). A bare `tbl` is a table whose row type the compiler doesn't know: it can be printed, written and passed on, but its columns can't be read and it can't be queried until it reaches a `tbl[R]` slot. A query step's output has an *anonymous* row, written by its shape: `tbl[{dept: str, n: int}]`. A table is never `null`.

**Columns.** `t.salary` on a `tbl[Employee]` is the column as a `vec[float]` (no copy); an unknown column is a compile-time error that lists the columns. There is no cell or column assignment, no `t[i]`, no iteration over rows (`for r in t`), and no `==` (§4.2); `len(t)` isn't defined (use `len(t.col)`).

**Matching rows by shape.** Where a table is stored (a variable, parameter, struct field or return value of type `tbl[R]`, whether `R` is a struct or an anonymous row, as for a variable whose first value was a query's result), a table of row `R` is stored as it is. One with an anonymous row matches when its columns are exactly `R`'s fields: the same names (in any order) and types, where an `int` column also fills a `float` field. It is then relabeled `R`, its columns put in field order and int columns converted. Two named structs never match, even with the same fields. The compiler checks this when it knows both rows and names the difference (`the table has a column 'avg', but DeptStats has no field 'avg'`; `column 'n' is int, but S declares it str`); a table of unknown row (a bare `tbl`, or a value of type `any`) is checked when it is stored: `variable 'v' is declared tbl[Emp] but got tbl[{id: int, name: str}]`. Containers share their elements, so a list of `tbl[R]` holds tables of row `R` only, with no relabeling.

**Creating tables.** `csv::read_file` and `csv::parse_text` (`stdlib.md`, `std/csv`) read a table, with the column types of the row struct where the expected type names one (`staff: tbl[Employee] = unwrap(csv::read_file('staff.csv'))`, a `tbl[R]` parameter, field or return annotation), and with inferred column types otherwise. Queries make new tables from tables.

**Queries.** `t | e -> verb(...) | verb(...)` runs verbs on `t` in order (`grammar.md` §10.5). `e` names the row of each step's input: inside a verb's arguments it is read only as `e.col`, it shadows an outer variable of that name, and a function written inside a verb can't read it. The compiler follows the row type through every step, so every column a query reads is checked at compile time; the source must be a table whose row type it knows.

| Step | Result |
|---|---|
| `filter(cond)` | The rows where `cond` (a bool) is true; rows where it is false or missing are dropped |
| `mutate(name = expr, ...)` | The input's columns with each new column added at the end, or a column of the same name replaced in place (its type may change). Columns are computed in the order written, and each expression sees the columns written before it in the same `mutate` |
| `select(e.a, e.b)` | An anonymous row of those columns, in that order. Arguments are columns. `select(-e.a)` is an error; dropping columns is `deselect` |
| `deselect(e.a, e.b)` | An anonymous row of every other column, in the input's order. Every row is kept. Dropping every column is an error |
| `rename(b = e.a)` | An anonymous row with those names changed and every column kept, in the input's order. The renames apply together, so `rename(a = e.b, b = e.a)` swaps the names |
| `distinct(e.a, e.b)` | The first row of each key, in the order the keys first appear, with the input's row type. `distinct()` uses every column. A missing key is one group, and so is NaN; `distinct` does not sort |
| `sort(e.a, desc(e.b))` | The rows ordered by the key columns, stable, missing values last (NaN after numbers, before missing values) in either direction |
| `group_by(e.k, ...)` then `summarize(...)` | One row per distinct key, in key order (missing keys last): the key columns, then the summarize columns. `group_by` must be followed directly by `summarize` |
| `summarize(name = agg(e.col), ...)` | Without `group_by`, one row for the whole table. Each column is one aggregate of one column: `count()` (rows), `count(e.col)` (present values), `sum`, `mean`, `min`, `max`, `n_distinct` (keys, including one for missing and one for NaN; 0 on an empty table) |
| `head(n)`, `tail(n)` | The first or last `n` rows (6 by default); `n` is computed once and may not read the row |

Sort and group keys, `select`, `deselect`, `distinct` and aggregate arguments are columns, not expressions: to sort, group, select or aggregate by a computed value, `mutate` it first. `rename` takes `name = e.col`. A verb expression may use arithmetic, comparisons, `and`/`or`/`!`, `x if c else y`, literals, outer variables and calls of builtins, methods and functions with known return types; the column a `mutate` computes must have a column type. Each expression runs once per row, in row order.

Aggregates: `sum` of an int or bool column is an int (overflow is a runtime error), of a float column a float; `mean` is a float (of a bool column, the share of true values); `min` and `max` keep the column's type, and compare strings by bytes.

**Missing values.** A column may hold missing values (empty or `NA` CSV cells). Aggregates skip them, and NaN in float columns: `sum` of no values is 0, `count()` counts every row, and `mean`, `min` and `max` of no values are missing. In a verb expression, a part that reads a missing value is missing, without running it: `e.salary * 0.1` is missing where `salary` is, and a function call whose arguments read a missing value isn't made for that row. `and`, `or`, `!` and `x if c else y` follow SQL's three-valued logic instead: `true or <missing>` is true, `false and <missing>` is false, other combinations with a missing operand are missing, and a ternary with a missing condition is missing. A missing `filter` condition drops the row. Taking a missing value out of a column as one value (`v[i]`, `for x in t.col`) is a runtime error (§9.4).

**Inspection.** `print(t)` shows the table's type and row count, the column names and types, and its first 10 rows (`fmt::full(t)` shows all), a missing value as `null`; inside a list it prints as `<tbl[Emp], 5 rows>`. `t.head(n)` and `t.tail(n)` (6 by default) are tables of the same type, and `t.describe(n)` prints one line per column with its name, type and first `n` values (5 by default).

---

## 10) Modules and Imports

The full rules are in `specs/implemented/modules_v0_2_spec.md`; this section summarizes them.

### 10.1 Import forms

| Syntax | Binds | Resolution |
|--------|-------|------------|
| `incl './path'` | the file's stem (`path`) | Relative to the importing file; `.qrk` is added when the path has no extension |
| `incl './path' as name` | `name` | Same |
| `incl 'std/module'` (or `as name`) | `module` (or `name`) | The stdlib (§10.2) |

`incl` is only allowed at the top level of a file. A module in the same file needs no `incl`: it is written `module::name` directly (`incl m` is an error). An imported file's name, and each directory written in a relative or `std/` import path, must match `[a-z][a-z0-9_]*`; the main script is exempt. Any other quoted path (`incl 'foo'`) is an error.

### 10.2 Path resolution

A path starting with `./`, `../`, `/` or a drive letter (`C:/`) is a file path, resolved against the importing file's directory unless it is absolute.

A `std/` path is resolved in the stdlib root: the `QUARK_STDLIB_ROOT` environment variable when it is set (a value that isn't a directory is an error), else `stdlib` next to the compiler executable, or one or two directories above it. A directory named `stdlib` near the program is never used.

### 10.3 Scopes and qualified names

A file is the import unit and the scope boundary. Inside a file, a bare name resolves, in order, to a local (parameter, loop variable, `when` binding, local variable), a member of the enclosing `module` block, a top-level name of the file (a definition, a module, or an import binding), then the prelude. Nothing from another file is visible unqualified.

`::` names everything else:

| Form | Meaning |
|------|---------|
| `module::name` | a member of a module of this file (`mathx::square`) |
| `file::name` | a top-level name of an imported file, or a member of its primary module (the module named like the file): `fmt::show_vec`, `geo::Point` |
| `file::module::name` | a member of another module of an imported file (`core::helpers::double`) |

A qualified name works wherever a name does: as a value (`f = io::exists`), a call, a pipe target (`p | io::exists()`), a type annotation (`p: geo::Point`) and a struct literal (`geo::Point { x: 1 }`). `.` is only for fields, keys and methods of values: `io.exists` on an import binding, or `m.f` on a module, is a load error suggesting `::`. Imports are not re-exported: `geo::io::open` is an error even when `geo.qrk` imports `std/io`.

Names never collide silently. Within a file, a module member with the same name as a top-level name, an import binding equal to a top-level name or another binding, two modules with one name, and any name defined twice are `QK-LOAD-001` errors naming both definitions. Between files there is nothing to collide: each file's names are its own, and a library's own calls always reach its own definitions. Modules can't be nested and must be at the top level of a file.

### 10.4 Imported files

An imported file may only contain definitions, at the top level and in module bodies: `fn`, `struct`, `module`, `incl` (top level), the `extern` forms, and constants (`name = ` an int, float, str or bool literal, unary minus on a number, or `+ - * / %` over those). Anything else is a `QK-LOAD-001` error naming the statement, so importing a file never runs code. A constant's references are replaced by its value. An importer can read and call every name of an imported file (there are no access modifiers) but can't assign to any of them (`'geo::k' belongs to lib/geo.qrk and can't be assigned here`); a library's constants can't be assigned in the library either.

### 10.5 Structs across files

A struct may be defined at a file's top level or in a module body (not in a function body; a known issue in known_issues.md). Its identity is its definition: `a::Point` and `b::Point` are different types at compile time, and at runtime the parameter and field checks compare struct definitions, not names. `type()`, `print` and diagnostics show an imported struct as `stem::Point` (the defining file's stem, whatever alias imported it) and a main-script struct bare (`Point`).

### 10.6 Cycles and deduplication

An import cycle is a load error that shows the full chain. Each file is loaded once per compilation, identified by its real path (symbolic links resolved), so a file imported along several paths has one set of functions and one identity for each struct.

### 10.7 Internal names

The loader gives every definition a unique internal name (`q2_fmt__fmt_vec` for `fmt_vec` of the second imported file, `mathx__square` for a main-script module member) and rewrites every reference to it, so the analyzer and the back end see one flat namespace. Diagnostics and runtime messages always show the name as written (`fmt::fmt_vec`). User code can't reach an internal name, since user names can't contain `__` (§5).

### 10.8 Extern functions (native C++ interop)

`specs/ffi_v0_1_spec.md` is the full specification of native interop. A Quark file binds native C++ with `extern` declarations:

```quark
extern './math_ext.hpp'
extern fn fast_sqrt(x: float) float as 'my_sqrt'
```

`extern 'file.hpp'` `#include`s a C++ header, resolved relative to the declaring file (`std/...` paths resolve in the stdlib). `extern fn` maps a Quark name to a C++ symbol; `as 'symbol'` is required. The compiler converts values at the boundary according to the annotations below.

An extern function can also be declared as a method of a builtin type (`int`, `float`, `str`, `bool`, `list`, `dict` or `vec`):

```quark
extern fn list.shuffle(seed: int) list as 'q_list_shuffle'
```

which makes `my_list.shuffle(42)` call it. An extern method is visible only in the file that declares it, since a method call can't be qualified: another file calls it through a function of that file (`lib::shuffle(xs, 42)`, §10.3). Its name can't be the name of a prelude method of any type (`extern fn list.contains(x)` is an error because of `str.contains`), and declaring it twice for one receiver in a file is an error.

Types map to C++ as follows: `int`, `float`, `bool` and `str` become `int64_t`, `double`, `bool` and `const char*`; `list`, `dict` and `vec` become `QList*`, `QDict*` and `QVector*`; `fn` becomes `QClosure*`; every other annotation (`any`, `result`, `resource`, struct names), and a missing one, is passed as a boxed `QValue`. Arguments follow the parameter rules (§13.2): an `int` widens to a `float` parameter, and a `float` for an `int` parameter is an error. A native function can't return a null pointer for `str`, `list`, `dict`, `vec` or `fn` (`a native function declared to return list returned a null pointer`).

**Callbacks.** An `fn` parameter arrives as `QClosure*`. Native code calls it with `qext::call(f, args...)` (up to three `QValue` arguments; the result is a `QValue`) and can return it, or store it in a Quark value, with `qext::box(f)`. Any Quark function value can be passed. `qext::call` checks the argument count and fills in defaults like a call in Quark (§6.5).

Native code that adds to a list or dict should use `qext::list_push` and `qext::dict_set`, which check the element type. The `qext` API is in `runtime/include/quark/ext/api.hpp`.

#### Native libraries

A file links a native library and adds a C++ include search path with one-line directives (`specs/ffi_v0_1_spec.md` P1):

```quark
extern include_path 'include'   // -I, relative to this file
extern link 'openblas'          // -lopenblas
extern 'blas_glue.hpp'          // the glue header, #included
```

- `link` and `include_path` are contextual words after `extern`, not reserved keywords.
- `extern link` and `extern include_path` must be at the top level of a file (`QK-LOAD-001` inside a module or block). They apply to the whole program, including directives in imported files and the stdlib. Duplicates are removed.
- Paths are relative to the declaring file and should point at files shipped with the module. Where a library is installed on a given machine is passed on the command line (`--lib-path`, `--include-path`) or through `QUARK_LIB_PATH` / `QUARK_INCLUDE_PATH`.
- Link order: a file's libraries come before those of the files it imports; a library requested more than once keeps its last position.
- Nothing may follow the directive's string; words there are reserved for future per-library options.
- A library that can't be found is reported before linking (`QK-FFI-002`), naming the requesting file and the directories searched. A missing `include_path` directory is `QK-FFI-001`, and a C++ error inside a glue header is `QK-FFI-003`.

---

## 11) Error Model

There are three kinds of diagnostics: compile-time errors, compile-time warnings and runtime errors. Every one names a code, a stage and a source location: `error[QK-CHECK-001] (check): ... at file.qrk:3:9`. The codes are listed in error_codes.md.

### 11.1 Compile-time errors

Reported by the parser, the loader, the analyzer or the invariant checker. The compiler reports as many errors as it can find in one run, not only the first; the parser stops after 10.

| Category | Examples |
|----------|---------|
| Syntax | Missing tokens, malformed expressions, non-literal default values, a pipe into something that isn't a call |
| Names | Undefined names, a name defined twice, `fn` names or builtins assigned, `break`/`continue` outside a loop |
| Modules | Missing files, circular imports, unknown members, `.` where `::` is needed, statements in an imported file |
| Types | Wrong argument, field or element types, arithmetic on non-numbers, a variable assigned a value of another type |
| Arity | Too few or too many arguments, including to callbacks (§6.5) |
| Annotations | A body whose type doesn't match its return annotation or that can end without a value, a default that doesn't fit its annotation |
| Results | A result used as a condition, compared with `null`, or stored where a non-result is declared |

### 11.2 Compile-time warnings

A warning (`warning[QK-CHECK-002]`) is printed but doesn't stop compilation. There is one: a ternary whose branches have incompatible types (`1 if c else 'a'`).

### 11.3 Runtime errors

A runtime error prints its message and source location to stderr and exits with status 1. There are no exceptions and no recovery; expected failures are `result` values (§12). A stack overflow is a runtime error too (`stack overflow`, at the current source location), and output printed before it is flushed.

| Condition | Example message |
|-----------|-----------------|
| Wrong operand types where the compiler couldn't know them | `operator '<' expects two numbers or two strings` |
| Division or modulo by zero | `division by zero` |
| Integer overflow in `+`, `-`, `*`, `**`, unary `-`, `abs` | `integer overflow in '+'` |
| The call stack overflows | `stack overflow` |
| A float or string that doesn't fit an int, or NaN, converted with `to_int`, `floor`, `ceil` or `round` | `floor() can't convert 1e+30 to an int: it is outside the int range` |
| A value of the wrong type reaching an annotated parameter, variable, field or return | `argument 'x' of 'foo' expects int, got str` |
| A wrong-typed write into a list or dict from untyped code | `push() can't add a str to a list of int` |
| A dict key that isn't a `str`, from untyped code | `set(): dict keys are str, got int; convert it with to_str()` |
| `==` on two structs reached through untyped values | `'==' is not defined for struct values; compare their fields (a.x == b.x)` |
| `d.key` on a missing key | `dict has no key 'b'; use .get('b') if the key may be missing` |
| Dot access on a value that isn't a dict or struct, or on `null` | `cannot access member 'name' on null` |
| A method the value's type doesn't have (dynamic dispatch, §15) | `type 'int' has no method 'upper'` |
| An index out of range: `xs[i]`, `s[i]`, `v[i]`, `.set`, `.remove`; `.pop()` on an empty list | `list index 5 out of range (list length 2); use .get(i) when the index may be past the end` |
| A value pattern of `when` on a vec the compiler couldn't see (§8) | `when can't match a vec (== on a vec compares elementwise); ...` |
| `unwrap` of an `err`, or of a value that isn't a result | `unwrap on err: boom` |
| A result or vector used as a condition | |
| Calling a non-function, wrong argument count through a function value, more than 12 arguments | `function 'h' expects 2 arguments but got 1` |
| `range` with `step = 0`, `sqrt` of a negative number, `min`/`max` of an empty vector, vectors of different lengths in arithmetic | `sqrt() domain error: argument is negative (-1.0)` |
| A closed or stale resource, or a resource of the wrong kind | `io.read() expects an open file (resource is closed)` |
| `for` over a value that isn't a list, string or vector | |
| A loop body changing the length of the list the `for` iterates (§14) | `list changed size during the for loop over it (length 3 at the start, 2 now)` |
| `==` nested deeper than 512 levels | |

---

## 12) ok/err Result Type

`ok value` and `err value` create results: a tag and an arbitrary payload. A result is never `null`.

There are three ways to use one:

1. **`unwrap(r)`** returns the `ok` payload. On an `err` it is a runtime error that prints the payload.
2. **`when`** destructures it safely:
   ```
   when r:
       ok x  -> use(x)
       err e -> handle(e)
   ```
3. **`is_ok(r)`** and **`is_err(r)`** return a bool.

A result has no truthiness (§3). Storing a result where a non-result type is declared is a compile-time error that suggests `unwrap()` or `when`. Fallible builtins and stdlib functions return results: `input()`, `io::open`, `io::read`.

---

## 13) Type Annotations

### 13.1 Available types

Variable, parameter, struct field and return annotations: `int`, `float`, `str`, `bool`, `list`, `dict`, `vec`, `result`, `resource`, `fn`, `any`, struct names (qualified for another file: `geo::Point`), and tables: `tbl[R]` for a table whose rows are the struct `R`, and a bare `tbl` for a table of any row (§9.5). `fn` accepts any function value; it has no signature, and how the body calls the parameter is checked against the function passed instead (§6.5).

`null` and `void` are not annotations; `void` appears only in inferred types, such as a non-exhaustive `when`. Element types can't be written: `list[int]` is a compile-time error (`'list' takes no type argument`), and a `list` annotation holds any element type. The compiler infers element types (§9.1) and shows them in messages (`list[int]`, `dict[str, int]`). Written element types are planned (`list[T]`, `specs/tables_v0_1_spec.md` D16).

### 13.2 Parameter annotations

```
fn foo(x: int, y: float = 0.0) -> x + y
```

Each annotated parameter is enforced, whether or not the function's other parameters or its return type are annotated, with the same rules for Quark functions and `extern fn`:

- When the argument's type is known at compile time, it is checked then (`argument 1 of 'foo' expects int, got str`), at no runtime cost.
- When it isn't (`any`, for example the result of `unwrap` or of an unannotated function), it is checked on entry to the function, and a mismatch is a runtime error naming the parameter: `argument 'x' of 'foo' expects int, got str`. Calls through function values, pipes and default arguments are covered too.
- An `int` passed to a `float` parameter is converted to `float`.
- `null` is accepted only by `any`, struct and `resource` annotations. `list`, `dict`, `vec`, `tbl` and `fn` mean a list, dict, vec, table or function (an optional list parameter is written `xs: list = list []`; use `any` for a value that may be null). A `result` is always `ok(...)` or `err(...)`, so `null` can't be passed, stored, returned or used as a default where a `result` is expected.

A parameter without an annotation but with a literal default takes the default's type and is enforced the same way: `fn foo(y = 0)` has an `int` parameter, so `foo('a')` is a compile-time error and `foo(v)` with a `v` of unknown type is checked on entry. A `null` default gives no type. A parameter with both an annotation and a default needs a default that fits the annotation.

Other unannotated parameters accept any value. Inside the body, annotated `int`, `float` and `bool` parameters are native C++ values, so arithmetic on them is unboxed. Fully annotated functions compile to native C++ functions, and calls with arguments already known to have the right types pay no check.

Struct fields follow the same rules. A field value whose type is known at compile time is checked then (`cannot assign value of type 'str' to field 'x' of type 'int'`). Otherwise it is checked when the struct literal is built, so a field never holds a value of the wrong type: `field 'x' of P expects int, got str`. An `int` going into a `float` field, including a default, is converted to `float`.

### 13.3 Return type annotations

```
fn foo(x: int) int -> x + 1
```

A return annotation holds on every function. The compiler infers the body's type and checks it against the annotation; if the body has branches of different types, every one must fit. A body that can end without a value (an `if` without `else`, a `when` with no matching arm) is a compile-time error unless the annotation is `any`: `function 'f' declares return type 'int' but can end without a value`. A body whose type the compiler can't know is checked when the function returns, with the parameter rules (an `int` widens to `float`): `fn f(k) int -> k()`, then `f(fn() -> 'a')`, is the runtime error `function 'f' declares return type int but returned str`.

### 13.4 Variable type annotations

```
x: int = 42
r: result = ok 1
v: any = 1
```

The annotation is the variable's type for its whole life (§5): the initial value and every later assignment must fit it, and a mismatch is a compile-time error. An initial value whose type the compiler can't know is checked when it is stored, with the parameter rules: `x: int = id('a')` is the runtime error `variable 'x' is declared int but got str`. So a typed variable never holds a value of another type.

`any` makes a variable hold values of any type:

```
v: any = 1
v = 'changed'    // allowed
```

An unannotated variable is untyped in the same way only when its first value has no known type (§5). The compiler stores a variable of type `int`, `float` or `bool` as a native C++ scalar; an `any` variable is always boxed.

---

## 14) for Loops

```
for item in iterable:
    body
```

| Iterable | Yields |
|----------|--------|
| `list` | its elements, in order |
| `str` | one-byte strings |
| `vec` | scalars (`f64` → `float`, `i64` → `int`, `bool` → `bool`, `str` → `str`) |

Anything else is a compile-time error when the type is known (`for loop expects list, vec, or str iterable, got dict[str, int]`) and a runtime error otherwise. The loop variable belongs to the loop body (§5). `break` exits the innermost loop and `continue` skips to its next iteration; both are compile-time errors outside a loop.

**A list keeps its length during a `for` loop over it.** If the body pushes, pops, inserts or removes, the loop stops with a runtime error at the `for` before its next step, also when that happens in the last iteration:

```
xs = list [1, 2, 3]
for y in xs:
    xs.pop()      // runtime error: list changed size during the for loop over it (length 3 at the start, 2 now)
```

Writes that keep the length (`xs[i] = v`) are allowed. Each element is read when its iteration starts, so the loop sees a write to an element it hasn't reached yet. A change followed by `break` is allowed too, since the loop takes no further step. To process a list that grows while you work through it (a worklist), loop over an index: `while i < len(xs):`. Strings and vectors can't change length.

---

## 15) Dot Syntax

`.` reads keys, fields and methods of values. Names from modules and imported files use `::` (§10.3).

| Form | Meaning | Example |
|------|---------|---------|
| `d.key` | dict key read (a missing key is a runtime error, §9.2) | `user.name` |
| `d.key = val` | dict key write | `user.age = 31` |
| `s.field` | struct field read (fields can't be assigned) | `customer.name` |
| `value.method(args)` | method call | `'hello'.upper()`, `xs.push(4)` |

The receiver's type selects the method; stdlib.md lists the methods of each type. Dot access on a value that is neither a dict nor a struct is a compile-time error when the type is known (`dot access is only supported on dict ...`), and calling a method the type doesn't have is a compile-time error.

When the receiver's type isn't known at compile time (the result of `unwrap`, or an unannotated parameter), the method is chosen at runtime from the value's type: `unwrap(io::read(f, n)).split('\n')` works without an annotation. It is still a compile-time error if no type has the method or no type's version accepts that many arguments, and a runtime error (`type 'int' has no method 'upper'`) if the value's type lacks it.

---

## 16) Key Design Decisions

| Decision | Rationale |
|----------|-----------|
| Division always returns float | Prevents silent truncation (`5 / 2` is `2.5`) |
| Modulo is int-only | Avoids floating-point modulo surprises |
| Integer overflow is an error | Signed overflow is undefined in C++; an error is deterministic and keeps data pipelines from silently producing wrong numbers |
| Truthiness in conditions and logical operators | Familiar behaviour for scalars and collections in `if`/`while`/`and`/`or` |
| Results and vectors have no truthiness | `if r:` would silently discard errors; `if v == w:` would test emptiness, not equality |
| Short-circuit `and`/`or` return an operand | Guard patterns (`if n != 0 and total / n > 2:`) |
| A variable's type never changes or narrows | Every static type is a guarantee the generated code can rely on, so reads stay raw and fast (architecture.md §12) |
| A list's or dict's element type is fixed when it is created | Same reason; writes the compiler can't see are checked at runtime, which costs about 1.5% in a loop that only pushes |
| `list`, `dict`, `vec`, `fn` and `result` annotations reject `null` | `xs: list` means a list; use `any` or an empty default for optional values |
| `fn` declarations are fixed and hoisted | Calls work in any order and mutual recursion needs no forward declaration; a non-capturing declaration compiles to a direct call |
| `xs[i]` and `s[i]` out of range are errors; `xs.get(i)` and `d.get(k)` return `null` and have type `any` | A typed read is never a hidden `null`; probing stays available, and its result is checked where it meets a type |
| `d.key` on a missing key is an error | `d.key` names a key in the source, so a missing one is usually a typo; `.get` is for keys that may be absent |
| Dict keys are strings | Simple hashing and printing; `.get`/`.set` convert other keys to strings |
| Element types are inferred, not written | Keeps annotations short; `list[T]` is planned (`specs/tables_v0_1_spec.md` D16) |
| Runtime errors are fatal (no exceptions) | One predictable failure mode with no hidden control flow; expected failures are results |
| Lists, dicts and results compare structurally | Value-equal containers are equal, as users expect; functions compare by identity |
| `any` forces boxed storage | Lets a variable change type; native scalar storage is an optimization `any` opts out of |
| Resources use generational handles | Use after close is detected without relying on GC finalization |
