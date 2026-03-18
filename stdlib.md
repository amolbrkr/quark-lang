# Quark Standard Library

This document describes the built-in functions available in Quark. All standard library functions are implemented in C++ for performance and are automatically available without any imports.

Current builtin surface uses explicit prefixes for collections and strings:

- `s*` for string builtins (`supper`, `ssplit`, ...)
- `l*` for list builtins (`lpush`, `lget`, ...)
- `d*` for dict builtins (`dget`, `ditems`, ...)
- `v*` for vector conversion/utilities (`vfrom_list`, `vastype`, ...)

**Invocation model**: All builtins use function-call syntax: `callable(entity, ...)`. Dot-call syntax on values (`entity.method()`) is not supported. Dot is used for dict key access and module-qualified calls via aliases (for example `use 'std/math' as math`, `math.floor(...)`). Use pipes for chaining: `entity | callable() | next()`.

### Design principles

- Prefer explicit runtime failures over silent fallbacks for invalid type/domain usage.
- Keep hot numeric/vector kernels fast; compose higher-level behavior in stdlib modules.
- Keep APIs predictable: pure functions by default; in-place behavior must be clearly named/documented.

## Core Functions

### I/O Functions

| Function | Signature | Description |
|----------|-----------|-------------|
| `print` | `value[, end: str[, width: int[, align: str[, pad: str]]]] -> void` | Print value with configurable line ending and optional width/alignment |
| `println` | `value -> void` | Print value with newline |
| `input` | `[prompt: str] -> str` | Read line from stdin; optional prompt must be a string |

```quark
println('Hello, World!')
name = input()
println(name)

name = input('Name: ')
println(name)

print('row', '|', 8, 'left', '.')
print('42', '\n', 8, 'right', '0')
```

`print` alignment values:

- `left` (default)
- `right`
- `center`

If `width` is `0` or smaller than the rendered text width, no padding is applied.

Default behavior is `print(value)` with newline (`end='\n'`). Use `print(value, '')` for no trailing newline.

### File I/O Intrinsics and std/io

Low-level file I/O is exposed through `_file_*` builtins. The first-party module [src/core/stdlib/io.qrk](src/core/stdlib/io.qrk) provides thin wrappers (`io.open`, `io.read`, ...).

| Function | Signature | Description |
|----------|-----------|-------------|
| `_file_open` | `path: str, mode: str[, binary: bool] -> result` | Open file and return `ok(file_handle)` or `err(str)`; current modes are `r` and `w` |
| `_file_read` | `file: file_handle, n: int -> result` | Read up to `n` bytes/chars and return `ok(str)` or `err(str)` |
| `_file_write` | `file: file_handle, data: str -> result` | Write string data and return `ok(int)` byte count or `err(str)` |
| `_file_close` | `file: file_handle -> result` | Close file and return `ok(null)` or `err(str)` |
| `_file_seek` | `file: file_handle, offset: int, whence: int -> result` | Seek and return `ok(int)` new cursor position or `err(str)` |
| `_file_exists` | `path: str -> bool` | Check path existence |

```quark
use 'std/io' as io

when io.open('tmp.txt', 'w'):
  ok f ->
    _ = io.write(f, 'hello')
    _ = io.close(f)
  err msg -> println(msg)
```

### Type Conversion

| Function | Signature | Description |
|----------|-----------|-------------|
| `to_str` | `any -> str` | Convert to string |
| `to_int` | `int\|float\|str\|bool -> int` | Convert to integer. **Runtime error on invalid input.** |
| `to_float` | `int\|float\|str\|bool -> float` | Convert to float. **Runtime error on invalid input.** |
| `to_bool` | `any -> bool` | Convert to boolean (truthiness) |
| `type` | `any -> str` | Return runtime type name |
| `len` | `str\|list\|dict\|vector -> int` | Get length of string, list, dict, or vector |

```quark
to_str(42) | println()        // '42'
to_int('123') | println()     // 123
to_float('3.14') | println()  // 3.14
to_bool(0) | println()        // false
to_bool(1) | println()        // true
type(42) | println()          // int
len('hello') | println()      // 5
type(fn(x) -> x) | println()  // fn

dict { a: 1, b: 2 } | len() | println()  // 2
```

**Error behavior:**
- `to_int('abc')` — runtime error: cannot parse as integer
- `to_int('')` — runtime error: cannot convert empty string
- `to_float('xyz')` — runtime error: cannot parse as float
- `to_int(some_list)` — runtime error: cannot convert list to int
- `to_int(3.7)` returns `3` (truncation), `to_float(42)` returns `42.0` (widening) — these are valid conversions

### Range

| Function | Signature | Description |
|----------|-----------|-------------|
| `range` | `number -> list` | Generate `[0, 1, ... end-1]` |
| `range` | `number, number -> list` | Generate `[start, ... end-1]` |
| `range` | `number, number, number -> list` | Generate `[start, start+step, ...]` |

```quark
range(5) | println()          // [0, 1, 2, 3, 4]
range(2, 5) | println()       // [2, 3, 4]
range(10, 0, -2) | println()  // [10, 8, 6, 4, 2]
```

## List Functions

List operations backed by `std::vector<QValue>` for efficient data processing.

| Function | Signature | Description |
|----------|-----------|-------------|
| `lpush` | `list, any -> list` | Add item to end of list |
| `lpop` | `list -> any` | Remove and return last item. **Runtime error on empty list.** |
| `lget` | `list, int -> any` | Get item at index (supports negative; out-of-bounds returns `null`) |
| `lset` | `list, int, any -> any` | Set item at index |
| `linsert` | `list, int, any -> list` | Insert item at index |
| `lremove` | `list, int -> any` | Remove and return item at index |
| `lslice` | `list, int, int -> list` | Get sublist [start:end) |
| `lreverse` | `list -> list` | Reverse list in place |
| `lconcat` | `list, list -> list` | Concatenate two lists |
| `enumerate` | `list\|str\|vector -> list` | Build list of `{ index, value }` records |
| `len` | `list -> int` | Get number of items |

### Examples

```quark
// List literals use the `list` keyword
list = list [1, 2, 3]

// Basic operations
list = lpush(list, 10)
list = lpush(list, 20)
list = lpush(list, 30)

lget(list, 0) | println()        // 10
lget(list, -1) | println()       // 30 (negative index)

// Modify
lset(list, 1, 99)
lget(list, 1) | println()        // 99

// Remove
lpop(list) | println()           // 30
len(list) | println()           // 2

// Slice (returns new list)
sublist = lslice(list, 0, 2)

pairs = enumerate(list ['a', 'b'])
println(pairs)
```

### Notes

- Lists use `std::vector` internally for O(1) push/pop and O(1) random access
- Negative indices count from the end: `-1` is last item, `-2` is second-to-last
- `lslice` returns a new list; original is not modified
- `lreverse` modifies the list in place
- `lget` out-of-bounds access returns `null`; `lset`, `linsert`, and `lremove` with invalid arguments cause a runtime error
- `lpop` on an empty list causes a runtime error
- `enumerate` returns list records shaped like `dict { index: int, value: any }`

## Dict Functions

Dicts are key-value maps backed by `std::unordered_map<std::string, QValue>`.

### Static keys (dot access)

Dot syntax is exclusively for dict key read/write (not method calls):

```quark
info = dict { name: 'Alex', age: 30 }
println(info.name)      // 'Alex'
info.city = 'NYC'
len(info) | println()   // 3
```

### Dynamic keys (variable/expression)

If the key comes from a variable/expression, use these helpers:

| Function | Signature | Description |
|----------|-----------|-------------|
| `dget` | `dict, any -> any` | Get value by key (key is converted to str); missing key returns `null` |
| `dset` | `dict, any, any -> dict` | Set value by key (key is converted to str); returns the dict |
| `dkeys` | `dict -> list` | Return key list |
| `dvalues` | `dict -> list` | Return value list |
| `ditems` | `dict -> list` | Return list of records shaped like `dict { key, value }` |

```quark
mydict = dict { a: 1, b: 2 }

for item in list ['a', 'b', 'c']:
  println(dget(mydict, item))

mydict = dset(mydict, 'x', 99)
println(mydict.x)   // 99

for item in ditems(mydict):
  println(item.key)
  println(item.value)
```

## Math Functions

Mathematical operations implemented using C++'s math library.

| Function | Signature | Description |
|----------|-----------|-------------|
| `abs` | `int\|float -> int\|float` | Absolute value (preserves type) |
| `min` | `number, number -> number` | Minimum of two values |
| `max` | `number, number -> number` | Maximum of two values |
| `sqrt` | `number -> float` | Square root |
| `floor` | `float -> int` | Round down to integer |
| `ceil` | `float -> int` | Round up to integer |
| `round` | `float -> int` | Round to nearest integer |

### Examples

```quark
// Absolute value
x = 0 - 5
abs(x) | println()            // 5
abs(3.14) | println()         // 3.14

// Min and max
min(10, 5) | println()        // 5
max(10, 5) | println()        // 10
min(3.5, 2.1) | println()     // 2.1

// Square root
sqrt(16) | println()          // 4
sqrt(2) | println()           // 1.41421

// Rounding
floor(3.7) | println()        // 3
ceil(3.2) | println()         // 4
round(3.5) | println()        // 4
round(3.4) | println()        // 3

// Chained operations
x = 0 - 16
x | abs() | sqrt() | println()   // 4
sqrt(10) | floor() | println()  // 3
```

### Notes

- `print()` accepts 1 to 5 args: `value[, end[, width[, align[, pad]]]]`
- `println()` accepts exactly one argument
- `abs` accepts only `int` or `float`; passing any other type is a compile-time error
- `abs` preserves the input type (int returns int, float returns float)
- `min` and `max` return float if either argument is float; single-argument vector overloads are documented in the Vector section
- `sqrt` always returns float; negative argument causes a runtime error
- `floor`, `ceil`, `round` return int
- `range` accepts int or float inputs; float values are converted to integers internally

## Vector Functions

Typed vector operations for data-oriented workloads.

### Construction and Conversion

| Function | Signature | Description |
|----------|-----------|-------------|
| `vfrom_list` | `list\|vector -> vector` | Convert list to typed vector (or clone vector) |
| `vto_list` | `vector\|list -> list` | Convert vector back to list (identity for lists) |
| `vastype` | `vector, str -> vector` | Cast vector dtype (`f64`, `i64`, `bool`) |
| `vget` | `vector, int -> any` | Get scalar value at index |

### Reductions and Utilities

| Function | Signature | Description |
|----------|-----------|-------------|
| `sum` | `vector -> float` | Sum of all vector elements; `vector[bool]` is supported (true=1, false=0) |
| `min` | `vector -> float` | Minimum element in vector |
| `max` | `vector -> float` | Maximum element in vector |
| `vfillna` | `vector, any -> vector` | Replace null entries in a vector |

### Examples

```quark
// Literal inference
vi = vector [1, 2, 3, 4]          // vector[i64]
vf = vector [1.0, 2.0, 3.0]       // vector[f64]
vs = vector ['a', 'b', 'c']       // vector[str]

println(type(vi))
println(type(vf))
println(type(vs))

// Homogeneous conversion from list
v2 = vfrom_list(list [10, 20, 30])
println(type(v2))

// Mixed list conversion is invalid
// vfrom_list(list [1, '2', 3])     // error

// Numeric vector arithmetic
a = vector [10, 20, 30, 40]
b = vector [1, 2, 3, 4]
z = a - b
println(sum(z))

// String vectors support equality/inequality comparisons,
// but arithmetic (+, -, *, /) is not supported.

// Null fill and casts
filled = vfillna(a, 0)
iv = vastype(vf, 'i64')

// Convert vector back to list
back = vto_list(v2)
println(type(back))              // list
```

### Notes

- Vector literals must be homogeneous (`int`, `float`, or `str`)
- `vfrom_list` enforces the same homogeneity rule as vector literals
- Numeric vector arithmetic (`+`, `-`, `*`, `/`) supports numeric vectors only
- `sum`, `min`, and `max` return float
- `sum` supports `vector[bool]` (counts `true` as 1, `false` as 0) — useful for boolean mask operations like `sum(v > 25)`
- `vastype` currently supports casts among numeric/bool vector dtypes

## String Functions

String manipulation functions implemented in the C++ runtime.

| Function | Signature | Description |
|----------|-----------|-------------|
| `supper` | `str -> str` | Convert to uppercase |
| `slower` | `str -> str` | Convert to lowercase |
| `strim` | `str -> str` | Remove leading/trailing whitespace |
| `scontains` | `str, str -> bool` | Check if contains substring |
| `sstartswith` | `str, str -> bool` | Check if starts with prefix |
| `sendswith` | `str, str -> bool` | Check if ends with suffix |
| `sreplace` | `str, str, str -> str` | Replace all occurrences |
| `sconcat` | `str, str -> str` | Concatenate two strings |
| `ssplit` | `str, str -> list` | Split string by separator |

### Examples

```quark
// Case conversion
supper('hello world') | println()     // HELLO WORLD
slower('HELLO WORLD') | println()     // hello world

// Whitespace
strim('  hello  ') | println()        // hello

// Searching
scontains('hello world', 'world') | println()    // true
scontains('hello world', 'xyz') | println()      // false

sstartswith('hello world', 'hello') | println()  // true
sstartswith('hello world', 'world') | println()  // false

sendswith('hello world', 'world') | println()    // true
sendswith('hello world', 'hello') | println()    // false

// Manipulation
sreplace('hello world', 'world', 'quark') | println()  // hello quark
sconcat('hello ', 'world') | println()                  // hello world

ssplit('a,b,c', ',') | println()                       // ["a", "b", "c"]

// With pipe
'a,b,c' | ssplit(',') | println()

// Chaining
'  hello world  ' | strim() | supper() | println()   // HELLO WORLD
```

### Notes

- All string functions return new strings (original is not modified)
- `sreplace` replaces all occurrences, not just the first
- `sconcat` is string-only; list concatenation uses `lconcat`
- `ssplit` preserves empty fields (`,a,` becomes `['', 'a', '']`)
- Empty string handling:
  - `supper('')` returns `''`
  - `strim('')` returns `''`
  - `scontains('', 'x')` returns `false`
  - `sreplace('hello', '', 'x')` returns `'hello'` (no-op for empty pattern)

## Result Functions

Result values (`ok`/`err`) are Quark's explicit error handling mechanism. These helpers inspect and extract values from results.

At the Quark language level, `result` is a first-class type keyword. In v0.1 it is intentionally non-generic: use `result` in type annotations, and use `ok` / `err` to construct values.

| Function | Signature | Description |
|----------|-----------|-------------|
| `is_ok` | `result -> bool` | Returns `true` if the result is `ok` |
| `is_err` | `result -> bool` | Returns `true` if the result is `err` |
| `unwrap` | `result -> any` | Extract the `ok` value. **Runtime error if the result is `err`.** |

### Examples

```quark
fn safe_div(a, b) ->
    if b == 0:
        err 'division by zero'
    else:
        ok a / b

r = safe_div(10, 2)
is_ok(r) | println()       // true
is_err(r) | println()      // false
unwrap(r) | println()      // 5

r2: result = ok 42
println(type(r2))          // result

// Pattern matching (preferred for handling both cases)
when safe_div(10, 0):
    ok value -> println(value)
    err msg -> println(msg)      // 'division by zero'

// Pipe-friendly
safe_div(10, 2) | unwrap() | println()
safe_div(10, 2) | is_ok() | println()
```

### Notes

- `unwrap` on an `err` result crashes with the error message — use `when` pattern matching for safe handling
- `unwrap` on a non-result value crashes — result values are explicit, not magical
- `is_ok`/`is_err` on a non-result value is a runtime error
- Assigning a result directly to a typed variable (e.g. `x: int = safe_div(10, 2)`) is a compile-time error — use `unwrap()` or `when` to extract the value first

## String Literals

Quark supports both single-quoted and double-quoted string literals in v0.1.

```quark
s1 = 'hello'
s2 = "world"
println(sconcat(s1, ' '))
println(s2)
```

Supported escape sequences:

- `\\`
- `\'` inside single-quoted strings
- `\"` inside double-quoted strings
- `\n`, `\t`, `\r`, `\0`

String interpolation is not implemented in v0.1.

## Pipes

All functions work seamlessly with Quark's pipe operator:

```quark
// Single argument functions pipe naturally
'hello' | supper() | println()

// Multi-argument functions receive piped value as first argument
'hello world' | sreplace('world', 'quark') | println()
// Equivalent to: sreplace('hello world', 'world', 'quark')

// Complex chains
'  HELLO world  ' | strim() | slower() | sreplace('world', 'quark') | println()
// Output: hello quark
```

## Implementation Details

All standard library functions are implemented in the header-only C++ runtime under `src/core/quark/runtime/include/quark/`. Generated programs include `quark/quark.hpp` through compiler include paths.

### Runtime Data Model

Quark values are boxed at runtime using a tagged value model:

- Primitive: `int`, `float`, `str`, `bool`, `null`
- Collections: `list`, `dict`, `vector`
- Callable/result: `fn`, `result`

Container implementation choices:

- `list` uses `std::vector<QValue>`
- `dict` uses `std::unordered_map<std::string, QValue>`
- `vector` uses typed storage (`f64`, `i64`, `bool`, `str`) with runtime invariants

### Typing and Validation Layers

Builtins are validated in two phases:

1. **Compile-time**: The analyzer checks argument counts, type compatibility (when inferable), bool-only conditions, and result-to-scalar assignment
2. **Runtime**: All type/domain violations crash with a clear error message and non-zero exit — no silent null returns

**Error contract summary:**

| Category | Behavior | Examples |
|----------|----------|---------|
| Type mismatch | Runtime error (crash) | `to_int('abc')`, `sqrt('hello')`, `supper(42)` |
| Domain error | Runtime error (crash) | `lpop(list [])`, `sqrt(-1)` |
| Arity mismatch | Compile-time error | `lpush(list [1])` (missing arg), `len(a, b)` (extra arg) |
| Bool-only | Compile-time error | `if 1:`, `while 'yes':`, `x and 3` |
| Result misuse | Compile-time error | `x: int = some_result_fn()` |

**Intentional `null` data-return APIs** (documented exceptions to the crash policy):

- `lget(list, idx)` when index is out of bounds
- `dget(dict, key)` when key is missing

Vector-specific typing behavior:

- Vector literals infer homogeneous element type (`vector[i64]`, `vector[f64]`, `vector[str]`)
- Mixed element vector literals are analyzer errors
- `vfrom_list(list [...])` applies the same homogeneity rule
- Numeric vector arithmetic is allowed for numeric vectors; string vector arithmetic is rejected

### Builtin Wiring

Builtin definitions are wired through a shared catalog plus runtime implementation:

1. Runtime implementation in the C++ headers
2. Shared builtin catalog (`src/core/quark/builtins/catalog.go`) for names, arity, type keys, and runtime symbol mapping
3. Analyzer and codegen consume catalog data instead of maintaining separate hardcoded builtin lists

The catalog and runtime behavior must stay in sync for arity, naming, and return-type behavior.

### Adding New Builtins

To add a new builtin function:

1. **Add C++ implementation** in the appropriate runtime header under `runtime/include/quark/`
2. Runtime headers are modular under `src/core/quark/runtime/include/quark/` and consumed via `#include "quark/quark.hpp"`
3. **Register the builtin in the shared catalog** at `src/core/quark/builtins/catalog.go`
4. **Add analyzer special-casing only when needed** (for polymorphic/shape rules not expressible in catalog type keys)

For changes that impact syntax and semantics (for example new literal rules), update smoke files and both Go and runtime unit tests to preserve analyzer/runtime consistency.

## Future Stdlib Roadmap (Not Implemented Yet)

This section is a forward-looking roadmap and is not part of the currently implemented stdlib surface.

### Proposed v1 modules and APIs

#### 1) `string`

Baseline text APIs beyond existing `supper/slower/strim/scontains/sstartswith/sendswith/sreplace/sconcat/ssplit`:

- `join(parts, sep) -> str`
- `lstrip(s) -> str`
- `rstrip(s) -> str`
- `repeat(s, n) -> str`
- `index(s, sub) -> int` (`-1` if not found)
- `rindex(s, sub) -> int` (`-1` if not found)
- `count(s, sub) -> int`
- `substr(s, start, len) -> str`
- `slice_str(s, start, end) -> str`
- `is_alpha(s) -> bool`
- `is_digit(s) -> bool`
- `is_alnum(s) -> bool`
- `is_space(s) -> bool`
- `pad_left(s, width, fill=' ') -> str`
- `pad_right(s, width, fill=' ') -> str`
- `center(s, width, fill=' ') -> str`

Behavior notes:

- v1 is byte-oriented string indexing/slicing unless full Unicode semantics are explicitly introduced.
- `index`/`rindex` return `-1` when not found (no exception/panic behavior).

#### 2) `random`

Small, deterministic-friendly utilities:

- `seed(n) -> null`
- `rand_float() -> float` (range `[0, 1)`)
- `rand_int(min, max) -> int` (inclusive bounds)
- `choice(items) -> result[any]` (`err` on empty list)
- `shuffle(items) -> list` (pure copy)
- `shuffle_inplace(items) -> list` (mutates and returns list)

Behavior notes:

- Same seed should produce stable sequences for the same runtime/version.

#### 3) `fs` (base file I/O)

Minimal file and directory operations:

- `exists(path) -> bool`
- `is_file(path) -> bool`
- `is_dir(path) -> bool`
- `read_text(path) -> result[str]`
- `read_lines(path) -> result[list]`
- `write_text(path, text) -> result[null]`
- `append_text(path, text) -> result[null]`
- `write_lines(path, lines) -> result[null]`
- `list_dir(path) -> result[list]`
- `mkdir(path, parents=false) -> result[null]`
- `remove(path) -> result[null]`

Behavior notes:

- On failure, return `err` with path + operation context.
- Use UTF-8 text behavior as default.

#### 4) `csv`

Practical row-oriented CSV support:

- `read_csv(path, options=dict {}) -> result[list]`
- `write_csv(path, rows, options=dict {}) -> result[null]`
- `parse_csv(text, options=dict {}) -> result[list]`
- `to_csv(rows, options=dict {}) -> result[str]`

Minimum option support:

- `header` (default `true`)
- `delimiter` (default `','`)
- `quote` (default `'"'`)

Behavior notes:

- Supports quoted fields, escaped quotes, empty cells.
- Column/header shape errors return `err` (no silent truncation).

#### 5) `json`

Core JSON decode/encode APIs:

- `json_parse(text) -> result[any]`
- `json_stringify(value, pretty=false, indent=2) -> result[str]`
- `json_read(path) -> result[any]`
- `json_write(path, value, pretty=false, indent=2) -> result[null]`

Behavior notes:

- v1 is strict JSON (no comments/trailing commas).
- Decode returns plain Quark values (`dict`, `list`, primitives, `null`).

#### 6) `math`

Current functions remain (`abs`, `min`, `max`, `sqrt`, `floor`, `ceil`, `round`) plus:

- `clamp(x, lo, hi) -> number`
- `sign(x) -> int` (`-1`, `0`, `1`)
- `pow(x, y) -> number`
- `sum(values) -> number`
- `mean(values) -> float`
- `median(values) -> float`
- `variance(values) -> float`
- `stddev(values) -> float`
- Constants: `PI`, `E`

Behavior notes:

- Domain errors for safe APIs should return `err`; low-level numeric primitives now print a diagnostic to stderr and exit with code 1 (no silent null returns).

#### 7) `vector`

Current core (`vfrom_list`, `vto_list`, `vastype`, `vfillna`, arithmetic/reductions) plus QoL:

- `dtype(v) -> str`
- `head(v, n=5) -> vector`
- `tail(v, n=5) -> vector`
- `take(v, indices) -> vector`
- `unique(v) -> vector|list`
- `nunique(v) -> int`
- `count(v) -> int` (non-null count)
- `mean(v) -> float`
- `any(v) -> bool`
- `all(v) -> bool`
- `is_null(v) -> vector[bool]`
- `not_null(v) -> vector[bool]`
- `where(mask, a, b) -> vector`
- `clip(v, lo, hi) -> vector`

Behavior notes:

- Null propagation rules must be documented per function.
- Numeric kernels remain optimized for data-heavy workloads.

#### 8) `functional`

Small higher-order function set:

- `map(iterable, fn) -> list|vector`
- `filter(iterable, pred) -> list|vector`
- `reduce(iterable, init, fn) -> any`
- `find(iterable, pred) -> result[any]`
- `take(iterable, n) -> list|vector`
- `any(iterable, pred=null) -> bool`
- `all(iterable, pred=null) -> bool`

Behavior notes:

- Stable iteration order.
- Prefer pure returns; in-place variants must be explicitly named.

#### 9) `result` and QoL error helpers

Language-level `ok`/`err` with `is_ok`, `is_err`, and `unwrap` are already implemented. Future additions:

- `unwrap_or(res, default) -> any`
- `unwrap_or_else(res, fn) -> any`
- `map_ok(res, fn) -> result`
- `map_err(res, fn) -> result`
- `panic(message) -> never`
- `assert(cond, message='assertion failed') -> null`

Behavior notes:

- `panic` terminates execution with a non-zero exit code and readable message.
- `assert` is for invariants; it panics when false.

#### 10) Additional high-value small modules

- `time`: `now_ms()`, `sleep_ms(ms)`
- `path`: `join_path(...)`, `basename(path)`, `dirname(path)`, `extname(path)`
- `collections`: `keys(dict)`, `values(dict)`, `items(dict)`, `has_key(dict, key)`

### Suggested delivery phases

1. Phase 1: `result`, `string`, `math`, `functional`
2. Phase 2: `fs`, `path`, `random`
3. Phase 3: `csv`, `json`
4. Phase 4: extended `vector` QoL/statistics
