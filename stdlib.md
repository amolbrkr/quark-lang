# Quark Standard Library

This document describes the built-in functions available in Quark. All standard library functions are implemented in C++ for performance and are automatically available without any imports.

**Invocation model**: All builtins use function-call syntax: `callable(entity, ...)`. Dot syntax (`entity.method()`) is not supported — dot is reserved for dict key access only. Use pipes for chaining: `entity | callable() | next()`.

### Design principles

- Prefer explicit failure for I/O and parsing: return `result` (`ok`/`err`) instead of silent fallbacks.
- Keep hot numeric/vector kernels fast; compose higher-level behavior in stdlib modules.
- Keep APIs predictable: pure functions by default; in-place behavior must be clearly named/documented.

## Core Functions

### I/O Functions

| Function | Signature | Description |
|----------|-----------|-------------|
| `print` | `[value] -> void` | Print optional value without newline |
| `println` | `[value] -> void` | Print optional value with newline |
| `input` | `[prompt] -> str` | Read line from stdin; optional prompt string |

```quark
println('Hello, World!')
name = input()
println(name)

name = input('Name: ')
println(name)
```

### Type Conversion

| Function | Signature | Description |
|----------|-----------|-------------|
| `to_str` | `any -> str` | Convert to string |
| `to_int` | `any -> int` | Convert to integer |
| `to_float` | `any -> float` | Convert to float |
| `to_bool` | `any -> bool` | Convert to boolean (truthiness) |
| `type` | `any -> str` | Return runtime type name |
| `len` | `str\|list\|dict -> int` | Get length of string, list, or dict |

```quark
str(42) | println()           // '42'
int('123') | println()        // 123
float('3.14') | println()     // 3.14
bool(0) | println()           // false
bool(1) | println()           // true
type(42) | println()          // int
len('hello') | println()      // 5

dict { a: 1, b: 2 } | len() | println()  // 2
```

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
| `push` | `list, any -> list` | Add item to end of list |
| `pop` | `list -> any` | Remove and return last item |
| `get` | `list, int -> any` | Get item at index (supports negative) |
| `set` | `list, int, any -> any` | Set item at index |
| `insert` | `list, int, any -> list` | Insert item at index |
| `remove` | `list, int -> any` | Remove and return item at index |
| `slice` | `list, int, int -> list` | Get sublist [start:end) |
| `reverse` | `list -> list` | Reverse list in place |
| `concat` | `list, list -> list` | Concatenate two lists |
| `len` | `list -> int` | Get number of items |

### Examples

```quark
// List literals use the `list` keyword
list = list [1, 2, 3]

// Basic operations
list = push(list, 10)
list = push(list, 20)
list = push(list, 30)

get(list, 0) | println()        // 10
get(list, -1) | println()       // 30 (negative index)

// Modify
set(list, 1, 99)
get(list, 1) | println()        // 99

// Remove
pop(list) | println()           // 30
len(list) | println()           // 2

// Slice (returns new list)
sublist = slice(list, 0, 2)
```

### Notes

- Lists use `std::vector` internally for O(1) push/pop and O(1) random access
- Negative indices count from the end: `-1` is last item, `-2` is second-to-last
- `slice` returns a new list; original is not modified
- `reverse` modifies the list in place
- Out-of-bounds access returns `null`

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

```quark
mydict = dict { a: 1, b: 2 }

for item in list ['a', 'b', 'c']:
  println(dget(mydict, item))

mydict = dset(mydict, 'x', 99)
println(mydict.x)   // 99
```

## Math Functions

Mathematical operations implemented using C++'s math library.

| Function | Signature | Description |
|----------|-----------|-------------|
| `abs` | `number -> number` | Absolute value |
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

- `print()` and `println()` can be called with zero arguments
- `abs` preserves the input type (int returns int, float returns float)
- `min` and `max` return float if either argument is float
- `sqrt` always returns float
- `floor`, `ceil`, `round` return int
- `range` accepts int or float inputs; float values are converted to integers internally

## Vector Functions

Typed vector operations for data-oriented workloads.

### Construction and Conversion

| Function | Signature | Description |
|----------|-----------|-------------|
| `to_vector` | `list\|vector -> vector` | Convert list to typed vector (or clone vector) |
| `to_list` | `vector\|list -> list` | Convert vector back to list (identity for lists) |
| `astype` | `vector, str -> vector` | Cast vector dtype (`f64`, `i64`, `bool`) |

### Reductions and Utilities

| Function | Signature | Description |
|----------|-----------|-------------|
| `sum` | `vector -> float` | Sum of all vector elements |
| `min` | `vector -> float` | Minimum element in vector |
| `max` | `vector -> float` | Maximum element in vector |
| `fillna` | `vector, any -> vector` | Replace null entries in a vector |

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
v2 = to_vector(list [10, 20, 30])
println(type(v2))

// Mixed list conversion is invalid
// to_vector(list [1, '2', 3])     // error

// Numeric vector arithmetic
a = vector [10, 20, 30, 40]
b = vector [1, 2, 3, 4]
z = a - b
println(sum(z))

// String vectors support equality/inequality comparisons,
// but arithmetic (+, -, *, /) is not supported.

// Null fill and casts
filled = fillna(a, 0)
iv = astype(vf, 'i64')

// Convert vector back to list
back = to_list(v2)
println(type(back))              // list
```

### Notes

- Vector literals must be homogeneous (`int`, `float`, or `str`)
- `to_vector` enforces the same homogeneity rule as vector literals
- Numeric vector arithmetic (`+`, `-`, `*`, `/`) supports numeric vectors only
- `sum`, `min`, and `max` return float
- `astype` currently supports casts among numeric/bool vector dtypes

## String Functions

String manipulation functions implemented in the C++ runtime.

| Function | Signature | Description |
|----------|-----------|-------------|
| `upper` | `str -> str` | Convert to uppercase |
| `lower` | `str -> str` | Convert to lowercase |
| `trim` | `str -> str` | Remove leading/trailing whitespace |
| `contains` | `str, str -> bool` | Check if contains substring |
| `startswith` | `str, str -> bool` | Check if starts with prefix |
| `endswith` | `str, str -> bool` | Check if ends with suffix |
| `replace` | `str, str, str -> str` | Replace all occurrences |
| `concat` | `str, str -> str` | Concatenate two strings |
| `split` | `str, str -> list` | Split string by separator |

### Examples

```quark
// Case conversion
upper('hello world') | println()     // HELLO WORLD
lower('HELLO WORLD') | println()     // hello world

// Whitespace
trim('  hello  ') | println()        // hello

// Searching
contains('hello world', 'world') | println()    // true
contains('hello world', 'xyz') | println()      // false

startswith('hello world', 'hello') | println()  // true
startswith('hello world', 'world') | println()  // false

endswith('hello world', 'world') | println()    // true
endswith('hello world', 'hello') | println()    // false

// Manipulation
replace('hello world', 'world', 'quark') | println()  // hello quark
concat('hello ', 'world') | println()                  // hello world

split('a,b,c', ',') | println()                       // ["a", "b", "c"]

// With pipe
'a,b,c' | split(',') | println()

// Chaining
'  hello world  ' | trim() | upper() | println()   // HELLO WORLD
```

### Notes

- All string functions return new strings (original is not modified)
- `replace` replaces all occurrences, not just the first
- `concat` also supports list + list and returns a list; mixed types return `null`
- `split` preserves empty fields (`,a,` becomes `['', 'a', '']`)
- Empty string handling:
  - `upper('')` returns `''`
  - `trim('')` returns `''`
  - `contains('', 'x')` returns `false`
  - `replace('hello', '', 'x')` returns `'hello'` (no-op for empty pattern)

## Pipes

All functions work seamlessly with Quark's pipe operator:

```quark
// Single argument functions pipe naturally
'hello' | upper() | println()

// Multi-argument functions receive piped value as first argument
'hello world' | replace('world', 'quark') | println()
// Equivalent to: replace('hello world', 'world', 'quark')

// Complex chains
'  HELLO world  ' | trim() | lower() | replace('world', 'quark') | println()
// Output: hello quark
```

## Implementation Details

All standard library functions are implemented in the header-only C++ runtime under `src/core/quark/runtime/include/quark/`. Generated programs include `quark/quark.hpp` through compiler include paths.

### Runtime Data Model

Quark values are boxed at runtime using a tagged value model:

- Primitive: `int`, `float`, `str`, `bool`, `null`
- Collections: `list`, `dict`, `vector`
- Callable/result: `func`, `result`

Container implementation choices:

- `list` uses `std::vector<QValue>`
- `dict` uses `std::unordered_map<std::string, QValue>`
- `vector` uses typed storage (`f64`, `i64`, `bool`, `str`, `cat`) with runtime invariants

### Typing and Validation Layers

Builtins are validated in two phases:

1. Compile-time analyzer checks argument counts and inferred types
2. Runtime guards validate concrete value kinds and return `null` or runtime errors when invalid

Vector-specific typing behavior:

- Vector literals infer homogeneous element type (`vector[i64]`, `vector[f64]`, `vector[str]`)
- Mixed element vector literals are analyzer errors
- `to_vector(list [...])` applies the same homogeneity rule
- Numeric vector arithmetic is allowed for numeric vectors; string vector arithmetic is rejected

### Builtin Wiring

Builtin functions are connected across three layers:

1. Runtime implementation in the C++ headers
2. Codegen builtin mapping (`src/core/quark/codegen/builtins.go`)
3. Analyzer builtin signatures (`src/core/quark/types/analyzer.go`)

All three layers must stay in sync for arity, naming, and return-type behavior.

### Adding New Builtins

To add a new builtin function:

1. **Add C++ implementation** in appropriate header under `runtime/include/quark/`
2. **Regenerate `runtime.hpp`** by running `build_runtime.ps1` (only needed for embedded-runtime fallback mode)
3. **Register the builtin mapping** in `src/core/quark/codegen/builtins.go`
4. **Register type signature** in `src/core/quark/types/analyzer.go`

For changes that impact syntax and semantics (for example new literal rules), update smoke files and both Go and runtime unit tests to preserve analyzer/runtime consistency.

## Stdlib Roadmap (v0.1)

This section defines the v1 standard library, the goal is a practical baseline for data-heavy programs with explicit behavior and error contracts.

### Proposed v1 modules and APIs

#### 1) `string`

Baseline text APIs beyond existing `upper/lower/trim/contains/startswith/endswith/replace/concat/split`:

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

- Domain errors for safe APIs should return `err`; low-level numeric primitives may keep existing null-return semantics if required for compatibility.

#### 7) `vector`

Current core (`to_vector`, `to_list`, `astype`, `fillna`, arithmetic/reductions) plus QoL:

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

Language-level `ok`/`err` remains central, plus helper utilities:

- `is_ok(res) -> bool`
- `is_err(res) -> bool`
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
