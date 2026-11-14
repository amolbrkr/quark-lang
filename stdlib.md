# Quark Standard Library

The prelude builtins (available everywhere without an import) and the stdlib modules imported with `incl 'std/...'`. Planned changes point to their spec.

## Contents

- [Overview](#overview)
  - [Functions and methods](#functions-and-methods)
  - [How values print](#how-values-print)
  - [Element types of returned lists](#element-types-of-returned-lists)
- [Prelude functions](#prelude-functions)
  - [Output and input](#output-and-input)
  - [Conversion and inspection](#conversion-and-inspection)
  - [Results](#results)
  - [Range](#range)
  - [Math](#math)
- [String methods (`str`)](#string-methods-str)
- [List methods (`list`)](#list-methods-list)
- [Dict methods (`dict`)](#dict-methods-dict)
- [Vectors (`vec`)](#vectors-vec)
  - [Vector methods](#vector-methods)
  - [Reductions](#reductions)
- [Tables (`tbl`)](#tables-tbl)
- [Stdlib modules](#stdlib-modules)
- [`std/io`](#stdio)
- [`std/fmt`](#stdfmt)
- [`std/sys`](#stdsys)
- [`std/csv`](#stdcsv)
- [`std/stats`](#stdstats)
- [`std/time`](#stdtime)
- [`std/random`](#stdrandom)
- [`std/blas` (OpenBLAS)](#stdblas-openblas)
- [Pipes](#pipes)
- [Error contract](#error-contract)

---

## Overview

### Functions and methods

Builtins are called in two ways:

1. **Free functions**: `f(args)`. I/O, conversions, math, `range`.
2. **Methods**: `value.method(args)`. The receiver's type selects the method, and the receiver becomes the first argument of the runtime function.

```quark
print('hello')                          // free function
'hello world'.upper() | print()         // method on str
xs = list [1, 2, 3]
xs.push(4).join(', ') | print()         // 1, 2, 3, 4
d = dict { a: 1 }
d.get('a') | print()                    // 1
```

Dot syntax has three uses, and `::` one:
- `d.key`: read or write a dict key written in the source. A missing key is a runtime error (`dict has no key 'nmae'; use .get('nmae') if the key may be missing`).
- `p.field`: read or write a struct field.
- `x.method(args)`: a method of the receiver's type. A method the type doesn't have is a compile-time error when the type is known (`type 'int' has no method 'upper'`).
- `io::exists(path)`: a member of an imported file. `incl 'std/io'` binds `io`; `incl 'std/io' as name` binds another name. `io.exists` is an error.

Builtin names can't be redefined or assigned (`len = 1`, `fn print(x) -> ...` are compile-time errors). A builtin can't be used as a value yet (`f = len` fails in clang; known_issues.md).

### How values print

`print` and `to_str` show values by their contents:
- Lists and vectors: `[1, 'a', null]`; strings inside a collection are quoted.
- Dicts: `{age: 36, 'first name': 'Ada'}`, keys sorted, quoted unless they read like names.
- Structs: `Point {x: 1, y: 2}`; results: `ok 5`, `err 'boom'`; files: `<resource file>`.
- A collection with more than 100 items shows its first and last 5 and a count: `[0, 1, 2, 3, 4, ..., 995, 996, 997, 998, 999] (1000 items)`. `fmt::full(x)` shows every item. A container that contains itself prints `[...]` at the repeat.
- Floats print as the shortest text that reads back as the same number, with `.0` on whole values: `4.0`, `0.1`, `1.4142135623730951`, `1e+16`, `inf`, `nan`. So `0.1 + 0.2` prints `0.30000000000000004`.
- Tables print their type and row count, the column names and types, and the first 10 rows ([Tables](#tables-tbl)); inside a collection, a table is `<tbl[Emp], 5 rows>`.

### Element types of returned lists

A list keeps the element type it was created with, and a dict its value type; writing a value of another type is a compile-time error, or a runtime error when the compiler can't see the value's type (`semantics.md`). Lists returned by builtins are typed the same way:

| Builtin | Returns |
|---|---|
| `range(...)` | `list[int]` |
| `s.split(sep)` | `list[str]` |
| `d.keys()` | `list[str]` |
| `d.values()` | a list of the dict's value type (`list[int]` for `dict { a: 1 }`) |
| `xs.slice(a, b)`, `xs.concat(ys)` | the receiver's element type; `concat` requires `ys` to have the same one |
| `v.to_list()` | `list[int]`, `list[float]`, `list[bool]` or `list[str]`, from the vector's dtype |

```quark
words = 'a b'.split(' ')
words.push(1)          // compile-time error: cannot add int to list[str]
```

To build a list that holds any value, annotate it: `xs: list = list []`.

---

## Prelude functions

### Output and input

| Function | Signature | Description |
|---|---|---|
| `print` | `value, end: str = '\n', width: int = 0, align: str = 'left', pad: str = ' '` | Print `value`, then `end`. Each call prints its own line unless `end` says otherwise. `width`/`align`/`pad` pad the text of `value` (not `end`) to `width` characters |
| `input` | `[prompt: str] -> result` | Print `prompt` (if given), then read one line from stdin without its `\n` or `\r\n` (any length). Returns `ok(line)` (`ok('')` for an empty line), or `err('end of input')` once stdin is exhausted |

```quark
print('Hello, World!')
print('a', '')
print('b')                              // ab
print('row', '|\n', 8, 'left', '.')     // row.....|
print('42', '\n', 8, 'right', '0')      // 00000042
name = unwrap(input('Name: '))
```

`align` is `left`, `right` or `center`. `pad` is one character (only its first character is used; `''` pads with spaces). If `width` is 0 or smaller than the text, nothing is padded. A misspelled `align` value is not rejected and aligns left.

To process stdin line by line, loop until `input()` returns an `err`:

```quark
count = 0
while true:
    r = input()
    if is_err(r):
        break
    line = unwrap(r)
    count = count + 1
print(count)
```

### Conversion and inspection

| Function | Signature | Description |
|---|---|---|
| `to_str` | `any -> str` | The text `print` would show |
| `to_int` | `int\|float\|str\|bool -> int` | A float is truncated toward zero (`to_int(-3.9)` is `-3`); a string must be an integer literal with nothing around it (`'12'`, not `' 12'` or `'12abc'`); `true` is `1`. Anything else, including `null`, is a runtime error |
| `to_float` | `int\|float\|str\|bool -> float` | A string must be a number literal (`'3.14'`, `'1e3'`). Anything else is a runtime error |
| `to_bool` | `any -> bool` | Truthiness: `0`, `0.0`, `''`, `null`, empty lists and empty dicts are `false`; `to_bool('false')` is `true`. A vector or a result is a runtime error (use `all`/`any`, or `is_ok`) |
| `type` | `any -> str` | Runtime type name: `int`, `float`, `str`, `bool`, `null`, `list`, `dict`, `fn`, `result`, `resource`, `vec[i64]`, `vec[f64]`, `vec[bool]`, `vec[str]`, or a struct's name (`Point`, `geo::Point`) |
| `len` | `str\|list\|dict\|vec -> int` | Length: bytes of a string, items of a list or vector, keys of a dict |

```quark
to_str(42) | print()        // 42
to_int('123') | print()     // 123
to_float('3.14') | print()  // 3.14
to_bool(0) | print()        // false
type(42) | print()          // int
len('hello') | print()      // 5
```

A value that doesn't fit an int is a runtime error, never wrapped: `to_int('99999999999999999999')`, `to_int(1e19)` and `to_int` of NaN or an infinity (`to_int() can't convert 1e+19 to an int: it is outside the int range`).

### Results

`ok(v)` and `err(v)` build results, Quark's explicit error values. A `result` is never `null`.

| Function | Signature | Description |
|---|---|---|
| `is_ok` | `result -> bool` | `true` for an `ok` |
| `is_err` | `result -> bool` | `true` for an `err` |
| `unwrap` | `result -> any` | The `ok` value. An `err` is a runtime error: `unwrap on err: boom` |

Passing a non-result to these is a compile-time error when the type is known. Storing a result where a value is expected (`x: int = f()` with `f` returning a result) is a compile-time error that suggests `unwrap()` or `when`.

```quark
fn safe_div(a, b) ->
    if b == 0:
        err 'division by zero'
    else:
        ok a / b

r = safe_div(10, 2)
is_ok(r) | print()       // true
unwrap(r) | print()      // 5.0

when safe_div(10, 0):
    ok value -> print(value)
    err msg -> print(msg)      // division by zero
```

### Range

| Call | Returns |
|---|---|
| `range(end)` | `[0, 1, ..., end-1]` |
| `range(start, end)` | `[start, ..., end-1]`; counts down (`[start, ..., end+1]`) when `start > end` |
| `range(start, end, step)` | `[start, start+step, ...]` up to, not including, `end`. A `step` of `0` is a runtime error |

Arguments are ints; the result is a `list[int]`. `for i in range(...)` compiles to a plain counting loop without building the list.

```quark
range(5).join(', ') | print()          // 0, 1, 2, 3, 4
range(2, 5).join(', ') | print()       // 2, 3, 4
range(5, 2).join(', ') | print()       // 5, 4, 3
range(10, 0, -2).join(', ') | print()  // 10, 8, 6, 4, 2
```

A float argument is an error, not truncated: a compile-time error when the type is known (`range(n / 2)`; use `floor(n / 2)`), otherwise a runtime error (`range(end) takes ints, got a float for end`). Ranges near the int64 limits end without wrapping around.

### Math

| Function | Signature | Description |
|---|---|---|
| `abs` | `int\|float -> int\|float` | Absolute value, same type as the argument |
| `min`, `max` | `number, number -> number` | The smaller/larger of two numbers: an `int` when both are ints, otherwise a `float` (`min(1, 2.5)` is `1.0`). With one argument they reduce a vector ([Reductions](#reductions)) |
| `sum`, `all`, `any` | `vec -> ...` | Vector reductions ([Reductions](#reductions)); the mean is `stats::mean` (`std/stats`) |
| `sqrt` | `float -> float` | Square root; an int argument is widened. A negative argument is a runtime error |
| `floor`, `ceil` | `float -> int` | Round down / up |
| `round` | `float -> int` | Round to nearest, halves away from zero (`round(2.5)` is `3`, `round(-2.5)` is `-3`) |

An int passed to `floor`, `ceil` or `round` is returned unchanged; a float result outside the int range, or NaN, is a runtime error (`floor() can't convert 1e+30 to an int: it is outside the int range`), and so is `abs` of the smallest int (`integer overflow in 'abs'`). A non-number is a compile-time error when the type is known (`sqrt('hello')`: `argument 1 of 'sqrt' expects float, got str`), otherwise a runtime error.

```quark
abs(0 - 5) | print()       // 5
min(10, 5) | print()       // 5
max(10, 5) | print()       // 10
sqrt(16) | print()         // 4.0
floor(3.7) | print()       // 3
```

Two-argument `min`/`max` with NaN give wrong values; see known_issues.md.

---

## String methods (`str`)

Strings are immutable: every method returns a new value. Join strings with `+` (`'a' + 'b'`); both sides must be strings, so convert other values with `to_str` first.

| Method | Signature | Description |
|---|---|---|
| `.upper()` | `-> str` | Uppercase (ASCII) |
| `.lower()` | `-> str` | Lowercase (ASCII) |
| `.trim()` | `-> str` | Remove leading and trailing whitespace |
| `.contains(sub)` | `str -> bool` | Whether `sub` occurs |
| `.startswith(prefix)` | `str -> bool` | Whether the string starts with `prefix` |
| `.endswith(suffix)` | `str -> bool` | Whether the string ends with `suffix` |
| `.replace(old, new)` | `str, str -> str` | Replace every occurrence |
| `.split(sep)` | `str -> list[str]` | Split at every `sep`, keeping empty fields: `',a,'.split(',')` is `['', 'a', '']`. `.split('')` returns `[s]` |
| `.slice(start, end)` | `int, int -> str` | Bytes `[start, end)`. Negative indices count from the end; out-of-range indices are clamped; `''` when `start >= end` |

```quark
'hello world'.upper() | print()                      // HELLO WORLD
'  hello  '.trim() | print()                         // hello
'hello world'.contains('world') | print()            // true
'hello world'.replace('world', 'quark') | print()    // hello quark

parts = 'a,b,c'.split(',')
len(parts) | print()                // 3
parts.get(1) | print()              // b

'hello world'.slice(6, 11) | print()  // world
'hello'.slice(-3, 5) | print()        // llo
'hello'.slice(0, -2) | print()        // hel

'  HELLO world  '.trim().lower().replace('world', 'quark') | print()   // hello quark
```

---

## List methods (`list`)

A list keeps its element type ([Element types](#element-types-of-returned-lists)): `push`, `insert`, `set` and `concat` reject a value of another type, at compile time when the type is known, otherwise at runtime. An `int` written into a list of floats is stored as a float.

| Method | Signature | Description |
|---|---|---|
| `.push(item)` | `any -> list` | Append, in place; returns the same list |
| `.pop()` | `-> any` | Remove and return the last item. An empty list is a runtime error |
| `.get(idx)` | `int -> any` | Item at `idx` (negative counts from the end); `null` when out of range, so the result has type `any` (`xs[idx]` is the typed read, and errors out of range) |
| `.set(idx, val)` | `int, any -> any` | Replace the item at `idx`; returns `val`. Out of range is a runtime error |
| `.insert(idx, val)` | `int, any -> list` | Insert before `idx`, in place; returns the same list. A negative index counts from the end; an index past either end is clamped (`list [1, 2, 3].insert(99, 7)` gives `[1, 2, 3, 7]`) |
| `.remove(idx)` | `int -> any` | Remove and return the item at `idx` (negative counts from the end). Out of range is a runtime error |
| `.slice(start, end)` | `int, int -> list` | New list of items `[start, end)`; negative indices count from the end, out-of-range indices are clamped |
| `.reverse()` | `-> list` | Reverse in place; returns the list |
| `.sort()` | `-> list` | Sort in place, ascending, stable; returns the list. Items must be all numbers (ints and floats mix), all strings (byte order) or all bools; anything else, including `null`, is a runtime error. Descending: `.sort().reverse()` |
| `.sort_by(key)` | `fn -> list` | Sort in place by `key(item)`, calling `key` once per item; stable; returns the list. Keys follow the `.sort()` rules |
| `.concat(other)` | `list -> list` | New list: the items of both. `other` must be a list with the same element type |
| `.join(sep)` | `str -> str` | Items joined by `sep`; non-strings are converted with `to_str` |
| `.enumerate()` | `-> list` | List of `{index, value}` dicts: `for e in xs.enumerate(): print(e.index)` |
| `.to_vec()` | `-> vec` | Typed vector of the items. All non-null items must have one type: `int`, `float`, `bool` or `str` (ints and floats don't mix). `null` items become missing entries; an empty list gives a `vec[i64]` |

```quark
xs = list [10, 20, 30]
xs.push(40)
xs.get(0) | print()           // 10
xs.get(-1) | print()          // 40
xs.pop() | print()            // 40
xs.set(1, 99) | print()       // 99
xs.insert(1, 55)
xs.slice(0, 2).join(', ') | print()   // 10, 55
xs.reverse().join(', ') | print()     // 30, 99, 55, 10
xs.sort().join(', ') | print()        // 10, 30, 55, 99

rows = list [dict {name: 'b', n: 2}, dict {name: 'a', n: 1}]
rows.sort_by(fn(r) -> r.n)
rows.get(0).name | print()            // a

xs.concat(list [1, 2]).join(', ') | print()   // 10, 30, 55, 99, 1, 2
list [1, 2, 3].join('+') | print()            // 1+2+3
print(type(list [1, 2, 3].to_vec()))          // vec[i64]
```

Known issues (known_issues.md): sorting a list that contains NaN is undefined.

---

## Dict methods (`dict`)

Keys are strings. A dict keeps the value type it was created with: `d = dict { a: 1 }`, then `d.b = 'x'` or `d.set('b', 'x')` is a compile-time error (`cannot store str under key 'b' of dict[str, int]`), and a runtime error from code the compiler can't check. `d: dict = dict { ... }` holds any value. An `int` stored in a dict of floats is stored as a float.

```quark
info = dict { name: 'Alex', role: 'dev' }
print(info.name)      // Alex
info.city = 'NYC'
len(info) | print()   // 3
```

Use the methods when the key is computed:

| Method | Signature | Description |
|---|---|---|
| `.get(key)` | `any -> any` | Value for `key`, or `null` when it's missing, so the result has type `any` (`d.key` is the typed read, and errors on a missing key) |
| `.set(key, val)` | `any, any -> dict` | Set `key`, in place; returns the same dict |
| `.keys()` | `-> list[str]` | The keys |
| `.values()` | `-> list` | The values, typed like the dict's values |
| `.items()` | `-> list` | List of `{key, value}` dicts |

`.get` and `.set` convert a non-string key with `to_str`: `d.set(1, 2)` stores the key `'1'`, and `d.get(1)` reads it. Iteration order of `.keys()`, `.values()` and `.items()` is not specified (printing sorts the keys).

```quark
mydict = dict { a: 1, b: 2 }
for k in list ['a', 'b', 'missing']:
    print(mydict.get(k))          // 1, 2, null
mydict.set('x', 99)
print(mydict.x)                   // 99
for item in mydict.items():
    print(item.key)
```

Known issues (known_issues.md): `d.set(1, ...)` and `d.set('1', ...)` write the same key.

---

## Vectors (`vec`)

Typed, columnar vectors for numeric work: `vec[i64]`, `vec[f64]`, `vec[bool]` or `vec[str]`, each with a validity mask for missing entries. Vectors are immutable: operations return new vectors.

```quark
vi = vec [1, 2, 3, 4]        // vec[i64]
vf = vec [1.0, 2.0, 3.0]     // vec[f64]
vs = vec ['a', 'b', 'c']     // vec[str]
v2 = list [10, null, 30].to_vec()   // vec[i64] with a missing entry
```

- A literal must be homogeneous and can't contain `null`; missing entries come from `list [...].to_vec()`. `vec [1, null, 3]` is planned (`tables_v0_1_spec.md` D12).
- Vectors can't be assigned into (`v[0] = 5` is a compile-time error).
- Arithmetic `+ - * /` works elementwise on numeric vectors of equal length, or a vector and a number; `/` gives `vec[f64]`. Comparisons (`<`, `==`, ...) give `vec[bool]`. Missing entries stay missing. A length mismatch is a runtime error.
- `v[i]` reads one element (negative counts from the end; out of range is a runtime error), and `v[mask]` with a `vec[bool]` of the same length keeps the selected elements: `v[v > 2]` (a mask of another length is a runtime error).
- **A missing entry taken out as one value is a runtime error** (`tables_v0_1_spec.md` D12): `v[i]`, `for x in v`, `.to_list()` and `.enumerate()` stop with `vec element 1 is missing; use fillna or drop_na first`, whatever the element type and whether or not the compiler knows the value is a vec. Operations on the whole vector (arithmetic, comparisons, masks, reductions, `fillna`, `drop_na`) keep or skip missing entries instead. `.get(i)` returns `null` for one, so its result has type `any`.

```quark
v = list [1, null, 3].to_vec()
print(v + 1)                 // [2, null, 4]
for x in v.drop_na():        // 1, 3
    print(x)
for x in v.fillna(0):        // 1, 0, 3
    print(x)
print(v.get(1))              // null
for x in v:                  // runtime error: vec element 1 is missing; use fillna or drop_na first
    print(x)
```
- A vector can't be used as a condition: `if v == 1:` is an error that suggests `all(v)`, `any(v)` or `len(v) > 0`.

### Vector methods

| Method | Signature | Description |
|---|---|---|
| `.get(idx)` | `int -> any` | Element at `idx` (negative counts from the end); `null` for a missing entry, so the result has type `any` (`v[idx]` is the typed read, and errors on a missing entry). Out of range is a runtime error (unlike list `.get`) |
| `.fillna(val)` | `any -> vec` | Missing entries replaced by `val`, which must have the element type, as a write into a typed list must: an int or float for `vec[float]`, an int for `vec[int]`, a bool for `vec[bool]`, a str for `vec[str]`. Anything else is an error even when nothing is missing: at compile time when both types are known (`fillna() can't fill vec[int] with float`), otherwise at runtime |
| `.drop_na()` | `-> vec` | The present entries, in order, with the same element type (the vector itself when nothing is missing) |
| `.astype(dtype)` | `str -> vec` | Cast between `'f64'`, `'i64'` and `'bool'`. f64 → i64 truncates; a value outside the int64 range is a runtime error. `vec[str]` can't be cast |
| `.to_list()` | `-> list` | List of the elements, typed by the dtype. A vector with missing entries is a runtime error (`to_list() of a vec with 2 missing entries; use fillna or drop_na first`) |
| `.enumerate()` | `-> list` | List of `{index, value}` dicts, as for lists; a missing entry is a runtime error |


### Reductions

Reductions follow the table aggregates (`tables_v0_1_spec.md` D6, D13), so `sum(t.x)` and `summarize(s = sum(e.x))` agree.

| Function | Signature | Description |
|---|---|---|
| `sum` | `vec[int] -> int`, `vec[bool] -> int`, `vec[float] -> float` | Sum of the present values: exact for ints (overflow is a runtime error), `true` counts 1. Floats are summed pairwise in `double` (NumPy's method): the rounding error grows with the log of the length, and the result is the same on every platform. No values sum to `0` (`0.0` for floats) |
| `min`, `max` | `vec[T] -> T` | Smallest / largest present value, with the vector's element type: numbers by value, `false` before `true`, strings in byte order. A vector with no present values is a runtime error |
| `all` | `vec[bool] -> bool` | Every present element is `true` (`true` for an empty vector) |
| `any` | `vec[bool] -> bool` | Some present element is `true` (`false` for an empty vector) |

- Missing entries are skipped, and NaN in float vectors too.
- `min`, `max` and `stats::mean` (`std/stats`) of a vector with no present values (empty, all missing, or all NaN) are runtime errors: `min() of a vec with no present values`. A value taken out of a vector must be present (D12); in a table, `summarize` gives a missing value instead, since a column can hold one.

```quark
vi = vec [1, 2, 3, 4]
print(sum(vi))                          // 10
print(min(vi))                          // 1
print(max(vi) - min(vi))                // 3, an int
print(sum(vec [10, 20] - vec [1, 2]))   // 27
print(sum(vi > 2))                      // 2: true values count 1
print(sum(vec [1.5, to_float('nan')]))  // 1.5: NaN is skipped
print(max(vec ['pear', 'apple']))       // pear
print(any(vi > 3))                      // true
```

---

## Tables (`tbl`)

A table is made by `std/csv` or a query (semantics.md §9.5, `tables_v0_1_spec.md`); `t.col` is a column as a vec. Its methods (D25):

| Method | Signature | Description |
|---|---|---|
| `head` | `t.head(n: int = 6) -> tbl` | The first `n` rows, as a table of the same type (all of them when there are fewer). A negative `n` is a runtime error |
| `tail` | `t.tail(n: int = 6) -> tbl` | The last `n` rows |
| `describe` | `t.describe(n: int = 5)` | Prints one line per column: its name, its type and its first `n` values |

`print(t)` shows at most 10 rows and counts the rest; `fmt::full(t)` shows all of them. Numbers are right-aligned, strings left-aligned and unquoted (a line break in one shows as `\n`, and a long one is cut at 32 characters with `...`), and a missing value is `null`. A table with an anonymous row shows `tbl` on its first line; the types are under the column names.

```quark
staff.describe()
// tbl[Emp]: 5 rows, 4 columns
// id      int    1, 2, 3, 4, 5
// name    str    'Ann', 'Bo', 'Cy', 'Di', 'Ed'
// dept    str    'eng', 'ops', 'eng', 'ops', null
// salary  float  120000.0, 90000.5, null, 150000.0, 80000.0

print(staff | e -> group_by(e.dept) | summarize(n = count()))
// tbl: 3 rows
// dept    n
// str   int
// eng     2
// ops     2
// null    1
```

---

## Stdlib modules

Each module is a `.qrk` file in `src/core/stdlib/` whose API is a `module` block named like the file, so its functions are `io::open`, `fmt::show_vec`, ... Nothing from an import is visible unqualified, so a program's own names never clash with the stdlib's.

The `extern fn` bindings a module is built on (`io_open`, `fmt_vec`, `blas_dot`, ...) sit at the file's top level and are reachable too (`fmt::fmt_vec`), since Quark has no access modifiers. They are implementation details: they take every argument explicitly (no defaults) and may change. Use the module functions. Native bindings are specified in `ffi_v0_1_spec.md`.

Module functions check their annotated parameters like any Quark function: `list`, `dict`, `vec` and `fn` parameters reject `null`; `resource` parameters accept it (and the function then reports an invalid handle).

`std/` paths are found in this order: the directory in `QUARK_STDLIB_ROOT`; else the first directory named `stdlib` in the importing file's directory or one of its parents; else a `stdlib` directory next to the `quark` executable or one or two levels above it. A project directory named `stdlib` above your script therefore replaces the standard library.

A runtime error raised inside a stdlib module is reported at the user's call.

---

## `std/io`

File I/O over `src/core/stdlib/io.hpp`. Import with `incl 'std/io' as io`. An open file is a `resource` (`type(f)` is `'resource'`; it prints as `<resource file>`). There are no file builtins in the prelude.

| Function | Signature | Description |
|---|---|---|
| `open` | `path: str, mode: str = 'r', binary: bool = false -> result` | Open with mode `r` (read), `w` (create or truncate, then write) or `a` (create if missing, write at the end); `ok` holds the file. Another mode returns `err("open: mode must be 'r', 'w' or 'a'")`; a system failure returns `err('open: <path>: <reason>')` |
| `read` | `file: resource, n: int -> result` | Read up to `n` bytes; `ok('')` at end of file. A negative `n` is a runtime error |
| `write` | `file: resource, data: str -> result` | Write `data`; `ok` holds the byte count. Writing to a file opened for reading returns `err('write: Bad file descriptor')` |
| `close` | `file: resource -> result` | Close; `ok(null)`. An already closed file returns `err('close: resource is closed')` |
| `seek` | `file: resource, offset: int, whence: int = 0 -> result` | Move the position (`0` = start, `1` = current, `2` = end); `ok` holds the new position. Another `whence` is a runtime error |
| `exists` | `path: str -> bool` | Whether `path` exists |
| `seek_set`, `seek_cur`, `seek_end` | `-> int` | The `whence` values `0`, `1`, `2` |

Reading, writing or seeking a closed file (or `null`) is a runtime error: `io.read() expects an open file (resource is closed)`.

Limits: writes are unbuffered; a file that is never closed stays open until the program exits; `binary` only changes newline handling on Windows; a read stops at a NUL byte (known_issues.md). `std/io` is for text. A rework is proposed in `stdlib_expansion_v0_1_spec.md` §6.

```quark
incl 'std/io' as io

f = unwrap(io::open('notes.txt', 'w'))
print(unwrap(io::write(f, 'hello quark')))   // 11
unwrap(io::close(f))

g = unwrap(io::open('notes.txt'))
unwrap(io::seek(g, 6, io::seek_set()))
print(unwrap(io::read(g, 100)))              // quark
unwrap(io::close(g))

a = unwrap(io::open('notes.txt', 'a'))
unwrap(io::write(a, '!'))
unwrap(io::close(a))

when io::open('notes.txt', 'x'):
    ok h -> print('opened')
    err msg -> print(msg)                    // open: mode must be 'r', 'w' or 'a'
```

---

## `std/fmt`

Text rendering beyond what `print` shows. Each function returns a `str`. Import with `incl 'std/fmt' as fmt`.

| Function | Signature | Description |
|---|---|---|
| `show_list` | `lst: list, n: int = 10, show_index: bool = true -> str` | A list as a column, up to `n` items |
| `show_vec` | `v: vec, n: int = 10, show_index: bool = true -> str` | A vector as a column, up to `n` items |
| `show_dict` | `dct: dict, n: int = 10, show_index: bool = true -> str` | A dict as key/value columns, sorted by key |
| `table` | `df: any, n: int = 20, show_index: bool = true -> str` | A dict of equal-length lists or vectors (columns sorted by name) as a boxed table, up to `n` rows. It also renders runtime tables, which Quark code can't create yet (`tables_v0_1_spec.md`). Columns of different lengths, or a column that isn't a list or vector, are runtime errors |
| `head` | `lst: list, n: int = 5 -> str` | The first `n` items of a list, rendered like `show_list` |
| `tail` | `lst: list, n: int = 5 -> str` | The last `n` items of a list |
| `full` | `x: any -> str` | `x` as `print` shows it, with every item of long collections |

```quark
incl 'std/fmt' as fmt

print(fmt::show_list(list [10, 20, 30]))
//    value
// --------
// 0     10
// 1     20
// 2     30
// [list: 3 elements]

print(fmt::table(dict {x: list [1, 2], y: vec ['a', 'b']}))
// +---+---+---+
// | # | x | y |
// +---+---+---+
// | 0 | 1 | a |
// | 1 | 2 | b |
// +---+---+---+
// [2 rows x 2 cols]
```

---

## `std/sys`

Program arguments, environment variables and exit status. Import with `incl 'std/sys' as sys`.

| Function | Signature | Description |
|---|---|---|
| `args` | `-> list` | Command-line arguments after the program name, as strings. With `quark run`, arguments after `--` are passed: `quark run tool.qrk -- in.csv -v` |
| `env` | `name: str -> str or null` | Value of the environment variable `name`, or `null` when it isn't set |
| `exit` | `code: int = 0` | Flush output and exit with status `code`, between 0 and 255; `0` means success. A code outside that range is a runtime error, since the OS keeps only the low 8 bits (256 would read as success) |

```quark
incl 'std/sys' as sys

args = sys::args()
if len(args) == 0:
    print('usage: tool <file>')
    sys::exit(2)
print('reading ' + args[0])
user = sys::env('USER')
if user != null:
    print('hello ' + user)
```

Run it with `quark run tool.qrk -- data.csv`, or build it once with `quark build tool.qrk -o tool` and run `./tool data.csv`.

---

## `std/csv`

CSV and TSV files as tables (`tables_v0_1_spec.md` §8, D15-D18). Import with `incl 'std/csv'`.

| Function | Signature | Description |
|---|---|---|
| `read_file` | `path: str, columns: dict = dict {} -> result` | `ok(table)` for the file at `path`, or `err(message)`. A path ending in `.tsv` or `.tab` is tab-separated, any other comma-separated |
| `parse_text` | `text: str, columns: dict = dict {} -> result` | The same for CSV text |
| `write_file` | `t: tbl, path: str -> result` | Writes a header line, then a line per row; `ok(null)` or `err(message)` |
| `format_text` | `t: tbl -> str` | The text `write_file` writes |

**Typed by where the table goes.** When the read's table is stored where a `tbl[R]` is expected, the file is read with `R`'s columns and types: an annotated variable (`staff: tbl[Employee] = unwrap(csv::read_file('staff.csv'))`), a `tbl[R]` parameter, a struct field or a return annotation. A cell that doesn't parse as its column's type is an error naming the line, the column and the text: `staff.csv:412: column 'salary': '12k' is not a float (and 3 more problems)`. Elsewhere (`t = unwrap(csv::read_file(p))`, `when csv::read_file(p): ...`) each column's type is inferred from all of its values: int if every value is an int, else float if every value is a number, else bool (`true`/`false`, `TRUE`/`FALSE`, `True`/`False`, `T`/`F`), else str. Such a table's row type is unknown to the compiler (`tbl`) until it reaches a `tbl[R]` slot, where it is checked (semantics.md §9.5).

**Columns are matched by name**, so a file may have more columns than the struct and in another order. `columns` maps fields to header names that differ: `csv::read_file('emp.csv', dict { id: 'EmpID', salary: 'Salary ($)' })`; unmapped fields use their own name. The mapping must be a `dict` literal of string literals, and its fields are checked against the row struct at compile time. A column missing from the header is an error that says how to map it: `emp.csv: no column named 'id' in the header (columns: EmpID, name); map field 'id' to one of them with dict { id: '...' }`. For an untyped read, the mapping names the table's columns.

**Format.** Fields may be quoted with `"..."`; a quoted field may contain the separator, line breaks and `""` (one `"`). Lines end with `\n`, `\r\n` or `\r`; a UTF-8 byte-order mark is skipped; blank lines are skipped; spaces around unquoted values are trimmed. Empty and `NA` cells, quoted or not, are missing values in every column. A row with the wrong number of fields, an unterminated quote, text after a closing quote, a duplicated header name, an empty file and an int that overflows are errors; the first problem is reported with the number of others. Str cells and header names must be valid UTF-8 without NUL bytes (`emp.csv:12: column 'name' is not valid UTF-8 (the file may be Latin-1 or another encoding)`).

**Writing.** Missing values are empty fields; strings are quoted when they contain the separator, a quote, a line break or surrounding spaces; floats are written in the shortest form that reads back the same.

```quark
incl 'std/csv'

struct Employee:
    id: int
    name: str
    salary: float

staff: tbl[Employee] = unwrap(csv::read_file('staff.csv'))
rich = staff | e -> filter(e.salary > 100_000)
when csv::write_file(rich, 'rich.csv'):
    ok _ -> print('written')
    err e -> print(e)
```

---

## `std/stats`

Statistics on vectors. Import with `incl 'std/stats'`. The functions follow the table aggregates (`tables_v0_1_spec.md` D6), so they agree with `summarize`.

| Function | Signature | Description |
|---|---|---|
| `mean` | `v: vec -> float` | Mean of the present values of a numeric or bool vector (a bool vector gives the share of `true` values): floats summed as `sum` sums them, ints summed exactly, then divided once. Missing entries and NaN are skipped; a vector with no present values, and a `vec[str]`, are runtime errors |

```quark
incl 'std/stats'

print(stats::mean(vec [1, 2, 3, 4]))                  // 2.5
print(stats::mean(list [1, null, 4].to_vec()))        // 2.5
print(stats::mean(vec [true, false, false, true]))    // 0.5
```

---

## `std/time`

Wall-clock and monotonic time. Import with `incl 'std/time' as time`.

| Function | Signature | Description |
|---|---|---|
| `now` | `-> float` | Seconds since the Unix epoch (wall clock), with sub-second precision |
| `clock` | `-> float` | Monotonic seconds from an arbitrary starting point; subtract two readings to time code |
| `sleep` | `seconds: float` | Pause for `seconds` (fractions allowed; zero or negative returns at once) |

```quark
incl 'std/time' as time

start = time::clock()
time::sleep(0.1)
print(time::clock() - start >= 0.1)   // true
```

---

## `std/random`

Pseudo-random numbers (64-bit Mersenne Twister), seeded randomly at startup; `seed(n)` makes the sequence reproducible. Not for cryptography. Import with `incl 'std/random' as random`.

| Function | Signature | Description |
|---|---|---|
| `seed` | `n: int` | Seed the generator; the same seed gives the same sequence |
| `uniform` | `-> float` | A float in `[0, 1)` |
| `randint` | `lo: int, hi: int -> int` | An int in `[lo, hi]`, both ends included; `lo > hi` is a runtime error |
| `choice` | `xs: list -> any` | A random element; an empty list is a runtime error |
| `shuffle` | `xs: list -> list` | Shuffle in place and return the list |

```quark
incl 'std/random' as random

random::seed(7)
roll = random::randint(1, 6)
print(roll >= 1 and roll <= 6)                  // true
deck = list ['a', 'b', 'c']
print(random::shuffle(deck).sort().join(''))    // abc
```

---

## `std/blas` (OpenBLAS)

BLAS routines over f64 vectors, through the system OpenBLAS. The module links it with `extern link 'openblas'`, so it needs the library and `cblas.h`:

- Debian/Ubuntu: `sudo apt install libopenblas-dev`
- macOS: `brew install openblas`, then pass `--lib-path "$(brew --prefix openblas)/lib"` and `--include-path "$(brew --prefix openblas)/include"` (or set `QUARK_LIB_PATH` / `QUARK_INCLUDE_PATH`).

Without the library, `incl 'std/blas'` fails at build time with `QK-FFI-002`; without `cblas.h`, with `QK-FFI-003`.

| Function | Signature | Description |
|---|---|---|
| `dot` | `x: vec, y: vec -> float` | Dot product |
| `axpy` | `a: float, x: vec, y: vec -> vec` | `a*x + y`, as a new vector |
| `scal` | `a: float, x: vec -> vec` | `a*x`, as a new vector |
| `nrm2` | `x: vec -> float` | Euclidean norm |
| `asum` | `x: vec -> float` | Sum of absolute values |
| `gemv` | `a: vec, rows: int, cols: int, x: vec, trans: bool = false -> vec` | Matrix-vector product `a·x`; `x` has `cols` elements and the result `rows`. With `trans`, `aᵀ·x`: `x` has `rows` elements and the result `cols` |
| `gemm` | `a: vec, m: int, k: int, b: vec, n: int -> vec` | Matrix product of `a` (`m x k`) and `b` (`k x n`); the result is `m x n` |
| `syrk` | `a: vec, rows: int, cols: int -> vec` | `aᵀ·a`, the `cols x cols` Gram matrix of `a` (`rows x cols`), with both triangles filled |

Matrices are flat f64 vectors in row-major order (row `i` is elements `i*cols` to `i*cols + cols - 1`), with their dimensions passed explicitly; Quark has no matrix type.

```quark
incl 'std/blas' as blas

x = vec [1.0, 2.0, 3.0]
y = vec [4.0, 5.0, 6.0]
print(blas::dot(x, y))           // 32.0
z = blas::axpy(2, x, y)          // [6.0, 9.0, 12.0]; y is unchanged

m = vec [1.0, 2.0, 3.0, 4.0, 5.0, 6.0]                 // [[1, 2, 3], [4, 5, 6]]
print(blas::gemv(m, 2, 3, x))                          // [14.0, 32.0]
print(blas::gemv(m, 2, 3, vec [1.0, 1.0], true))       // [5.0, 7.0, 9.0]: column sums
print(blas::gemm(m, 2, 3, vec [1.0, 0.0, 0.0, 1.0, 1.0, 1.0], 2))   // [4.0, 5.0, 10.0, 11.0]
print(blas::syrk(m, 2, 3))       // [17.0, 22.0, 27.0, 22.0, 29.0, 36.0, 27.0, 36.0, 45.0]
```

- BLAS reads the vectors' buffers in place; inputs are never modified.
- Inputs must be f64 vectors without missing entries (convert with `.astype('f64')`, fill with `.fillna(...)`). Length mismatches, other dtypes, missing entries, negative dimensions and dimensions that don't multiply to a matrix's length are runtime errors (`blas.gemm: b has 3 elements, but k * n = 2 * 1`).
- Vectors, dimensions and results are limited to `INT_MAX` elements, the CBLAS integer size. A dimension of 0 gives a result of zeros of the right shape (`blas::gemm(vec [], 2, 0, vec [], 3)` is six zeros).
- The routines use OpenBLAS's threads unless `OPENBLAS_NUM_THREADS` says otherwise.

---

## Pipes

`x | f(a)` is exactly `f(x, a)`, and `x | obj.m(a)` is exactly `obj.m(x, a)`: the piped value becomes the first argument. The target must be a call (`x | len` is a parse error).

```quark
'hello' | print()
list [1, 2] | len() | print()                                         // 2
'  HELLO world  '.trim().lower().replace('world', 'quark') | print()  // hello quark
```

---

## Error contract

| Misuse | Result | Example |
|---|---|---|
| Wrong argument type, type known at compile time | Compile-time error | `sqrt('hello')`, `len(5)`, `is_ok(5)` |
| Wrong argument count | Compile-time error | `xs.push()`, `len(a, b)` |
| Method the type doesn't have | Compile-time error | `x = 42`, then `x.upper()` |
| Writing a wrong-typed value into a list or dict | Compile-time error; runtime error from untyped code | `list [1].push('a')`, `dict {a: 1}.set('b', 'x')` |
| `null` for a `list`/`dict`/`vec`/`fn` parameter | Compile-time error; runtime error from untyped code | `fmt::show_list(null)` |
| Using a result as a value | Compile-time error | `x: int = f()` where `f` returns a result |
| Invalid value at runtime | Runtime error | `to_int('abc')`, `unwrap(err('x'))`, `range(1, 5, 0)` |
| Domain error | Runtime error | `list [].pop()`, `sqrt(-1)`, `min()` of an empty vector, `xs.set(99, 1)`, `xs[99]`, `'abc'[5]` |
| Missing dict key read with `d.key` | Runtime error | `dict {a: 1}.zz` |
| Failure caused by the environment | `err(...)` result | `io::open('missing.txt')`, `csv::read_file(path)` |

Truthiness is not an error: `if 1:` and `while 'yes':` follow `to_bool`.

These return `null` by design:
- `xs.get(i)` out of range, `d.get(k)` for a missing key, `v.get(i)` of a missing vector entry. All three have type `any`, so the result is checked where it meets a type (`n: int = xs.get(i)`).
- `sys::env(name)` for an unset variable.

---

How builtins are wired (catalog, runtime headers, call plans) is in `architecture.md` §6 (Builtin Catalog).
