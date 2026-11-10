# Quark Standard Library

This document describes the built-in prelude functions and methods available in Quark.

Prelude builtins are globally available without imports. Additional stdlib modules are imported via `use 'std/...'`, and extension modules may declare native bindings via QEI (`extern` / `extern fn`).

## QEI and Lowering Notes

- QEI extern declarations are part of the language surface:
    - `extern 'path.hpp'`
    - `extern link 'name'` and `extern include_path 'dir'` (native libraries; see semantics.md §10.6)
    - `extern fn name(...) Type as 'symbol'`
    - `extern fn type.name(...) Type as 'symbol'`
- Extern call sites are analyzer/codegen lowered through `DispatchExtern` and native type adaptation.
- Free extern functions can be used as first-class values via generated thunks.
- Scalar lowering is active for scalar-tiered arithmetic/comparisons (including `==`/`!=`) and scalar-bool `if`/`while` conditions.
- `for i in range(...)` has a raw-loop lowering fast path in codegen.

## Invocation Model

Quark builtins use two invocation styles:

1. **Free functions** — called as `fn(args)`. Used for I/O, conversions, math, range, and type-independent utilities.
2. **Methods** — called as `value.method(args)`. The value type determines which method is called. The value becomes the first argument to the underlying runtime function.

```quark
// Free function
println('hello')

// Method on str
'hello world'.upper() | println()

// Method on list
xs = list [1, 2, 3]
xs.push(4).join(', ') | println()   // 1, 2, 3, 4

// Method on dict
d = dict { a: 1 }
d.get('a') | println()   // 1
```

**Dot-call vs dot-access:** Dot syntax has four uses:
- `d.key` — dict key read/write (static key access)
- `p.field` — struct field read/write
- `d.method(args)` — method dispatch (when method name is known for the receiver type)
- `module.fn(args)` — module-qualified call via `use` alias

**How values print:** `println`, `print` and `to_str` render scalars directly, but containers only as a summary: a list prints as `[list len=3]`, a vector as `[vector len=3]`, a dict as `[dict len=2]`. To see elements, use `.join(sep)` or the `std/fmt` module. Floats are formatted with C's `%g`, so `4.0` prints as `4` and `sqrt(2)` as `1.41421`.

---

## Core Free Functions

### I/O

| Function | Signature | Description |
|----------|-----------|-------------|
| `print` | `value[, end: str[, width: int[, align: str[, pad: str]]]] -> void` | Print value with configurable line ending and optional width/alignment |
| `println` | `value -> void` | Print value with newline |
| `input` | `[prompt: str] -> str` | Read a line from stdin, without its `\n` or `\r\n` ending; any length. Returns `null` at end of input (an empty line gives `''`). Optional prompt must be a string |

```quark
println('Hello, World!')
name = input('Name: ')
println(name)

print('row', '|', 8, 'left', '.')
print('42', '\n', 8, 'right', '0')
```

To process stdin line by line, loop until `input()` returns `null`:

```quark
count = 0
while true:
    line = input()
    if line == null:
        break
    count = count + 1
println(count)
```

`print` alignment values: `left` (default), `right`, `center`. If `width` is 0 or smaller than the rendered text, no padding is applied.

### Type Conversion

| Function | Signature | Description |
|----------|-----------|-------------|
| `to_str` | `any -> str` | Convert to string |
| `to_int` | `int\|float\|str\|bool -> int` | Convert to integer. **Runtime error on invalid input.** |
| `to_float` | `int\|float\|str\|bool -> float` | Convert to float. **Runtime error on invalid input.** |
| `to_bool` | `any -> bool` | Convert to boolean (truthiness) |
| `type` | `any -> str` | Return runtime type name |
| `len` | `str\|list\|dict\|vector -> int` | Get length |
| `is_ok` | `result -> bool` | Returns `true` if the result is `ok` |
| `is_err` | `result -> bool` | Returns `true` if the result is `err` |
| `unwrap` | `result -> any` | Extract the `ok` value. **Runtime error if result is `err`.** |

```quark
to_str(42) | println()        // '42'
to_int('123') | println()     // 123
to_float('3.14') | println()  // 3.14
to_bool(0) | println()        // false
type(42) | println()          // int
len('hello') | println()      // 5
```

### Range

| Function | Signature | Description |
|----------|-----------|-------------|
| `range` | `number -> list` | Generate `[0, 1, ... end-1]` |
| `range` | `number, number -> list` | Generate `[start, ... end-1]`; counts down (`[start, ... end+1]`) when `start > end` |
| `range` | `number, number, number -> list` | Generate `[start, start+step, ...]`; `step` of `0` is a runtime error |

```quark
range(5).join(', ') | println()          // 0, 1, 2, 3, 4
range(2, 5).join(', ') | println()       // 2, 3, 4
range(5, 2).join(', ') | println()       // 5, 4, 3
range(10, 0, -2).join(', ') | println()  // 10, 8, 6, 4, 2
```

### Math

| Function | Signature | Description |
|----------|-----------|-------------|
| `abs` | `int\|float -> int\|float` | Absolute value (preserves type) |
| `min` | `number, number -> number` | Minimum of two values |
| `max` | `number, number -> number` | Maximum of two values |
| `sum` | `vector -> float` | Sum of all vector elements |
| `sqrt` | `float -> float` | Square root |
| `floor` | `float -> int` | Round down |
| `ceil` | `float -> int` | Round up |
| `round` | `float -> int` | Round to nearest |

```quark
abs(0 - 5) | println()       // 5
min(10, 5) | println()       // 5
max(10, 5) | println()       // 10
sqrt(16) | println()         // 4   (a float; %g drops the .0)
floor(3.7) | println()       // 3
```

`min`, `max` and `sum` also take a single vector; see [Vector free functions](#vector-free-functions-reductions).

### `enumerate`

| Function | Signature | Description |
|----------|-----------|-------------|
| `enumerate` | `list\|str\|vector -> list` | Build list of `{ index, value }` records |

```quark
pairs = enumerate(list ['x', 'y', 'z'])
for pair in pairs:
    println(pair.index)
    println(pair.value)
```

---

## String Methods (`str`)

All string methods leave the original string unchanged and return a new string (or value).

| Method | Signature | Description |
|--------|-----------|-------------|
| `.upper()` | `str -> str` | Convert to uppercase |
| `.lower()` | `str -> str` | Convert to lowercase |
| `.trim()` | `str -> str` | Remove leading/trailing whitespace |
| `.contains(sub)` | `str -> bool` | Check if substring exists |
| `.startswith(prefix)` | `str -> bool` | Check if starts with prefix |
| `.endswith(suffix)` | `str -> bool` | Check if ends with suffix |
| `.replace(old, new)` | `str, str -> str` | Replace all occurrences |
| `.concat(other)` | `str -> str` | Concatenate two strings |
| `.split(sep)` | `str -> list` | Split by separator |
| `.slice(start, end)` | `int, int -> str` | Substring `[start:end)` (negative indices supported) |

```quark
// Case
'hello world'.upper() | println()     // HELLO WORLD
'HELLO WORLD'.lower() | println()     // hello world

// Whitespace
'  hello  '.trim() | println()        // hello

// Searching
'hello world'.contains('world') | println()    // true
'hello world'.startswith('hello') | println()  // true
'hello world'.endswith('world') | println()    // true

// Manipulation
'hello world'.replace('world', 'quark') | println()  // hello quark
'hello '.concat('world') | println()                  // hello world

// Split
parts = 'a,b,c'.split(',')
len(parts) | println()                // 3
parts.get(1) | println()              // b

// Slice
'hello world'.slice(6, 11) | println()  // world
'hello'.slice(-3, 5) | println()        // llo   (negative start)
'hello'.slice(0, -2) | println()        // hel   (negative end)

// Chaining
'hello world'.slice(6, 11).upper() | println()  // WORLD

// Chain with pipe
'  HELLO world  '.trim().lower().replace('world', 'quark') | println()
// Output: hello quark
```

**Notes:**
- `.split('')` (empty separator) returns a single-element list containing the original string
- `.split` preserves empty fields: `',a,'` splits to `['', 'a', '']`
- `.slice` clamps out-of-range indices; returns `''` when `start >= end`
- `.replace` replaces all occurrences, not just the first

---

## List Methods (`list`)

| Method | Signature | Description |
|--------|-----------|-------------|
| `.push(item)` | `any -> list` | Add item to end, in place; returns the same list |
| `.pop()` | `-> any` | Remove and return last item. **Runtime error on empty list.** |
| `.get(idx)` | `int -> any` | Get item at index (negative supported; out-of-bounds → `null`) |
| `.set(idx, val)` | `int, any -> any` | Set item at index; returns the value set |
| `.insert(idx, val)` | `int, any -> list` | Insert item before index, in place; returns the same list |
| `.remove(idx)` | `int -> any` | Remove and return item at index |
| `.slice(start, end)` | `int, int -> list` | Sublist `[start:end)` |
| `.reverse()` | `-> list` | Reverse list in place; returns the list |
| `.sort()` | `-> list` | Sort in place, ascending, and return the list. Stable. Elements must all be numbers (ints and floats mix), all strings (byte order) or all bools; anything else, including `null`, is a runtime error. For descending order: `.sort().reverse()` |
| `.sort_by(key)` | `key: fn -> list` | Sort in place by `key(item)`, calling `key` once per item, and return the list. Stable, so items with equal keys keep their order. Keys follow the same rules as `.sort()` |
| `.concat(other)` | `list -> list` | Concatenate two lists |
| `.join(sep)` | `str -> str` | Join elements with separator (non-strings are coerced) |
| `.enumerate()` | `-> list` | Build list of `{ index, value }` records |
| `.to_vector()` | `-> vector` | Convert to typed vector (requires homogeneous elements; nulls allowed) |

```quark
xs = list [10, 20, 30]
xs.push(40)                    // changes xs in place
xs.get(0) | println()          // 10
xs.get(-1) | println()         // 40 (last element)
xs.get(999) | println()        // null (out of bounds)

xs.pop() | println()           // 40
xs.set(1, 99) | println()      // 99 (the value set)
xs.insert(1, 55)

xs.slice(0, 2).join(', ') | println()   // 10, 55
xs.reverse().join(', ') | println()     // 30, 99, 55, 10
xs.sort().join(', ') | println()        // 10, 30, 55, 99

rows = list [dict {name: 'b', n: 2}, dict {name: 'a', n: 1}]
rows.sort_by(fn(r) -> r.n)
rows.get(0).name | println()            // a

xs2 = list [1, 2]
xs.concat(xs2).join(', ') | println()   // 10, 30, 55, 99, 1, 2

// Join
list ['a', 'b', 'c'].join(',') | println()  // a,b,c
list [1, 2, 3].join('+') | println()        // 1+2+3

// Split then rejoin
'a,b,c'.split(',').join('-') | println()    // a-b-c

// Convert to vector
v = list [1, 2, 3].to_vector()
println(type(v))  // vector[i64]
```

**Notes:**
- `.push`, `.insert`, `.set`, `.remove`, `.pop`, `.reverse`, `.sort` and `.sort_by` change the list in place; `.push` and `.insert` also return it, so calls can be chained, but no reassignment is needed
- `.get` out-of-bounds returns `null`; `.set` and `.remove` out of bounds are runtime errors
- `.insert` clamps the index: a negative index counts from the end, and anything past the end appends (`list [1, 2, 3].insert(99, 7)` gives `1, 2, 3, 7`); a non-int index is a runtime error
- `.join` coerces non-string elements via `to_str`
- `.to_vector` requires all non-null elements to be the same type (`int`, `float`, `bool`, or `str`); nulls become null entries, and an empty list gives a `vector[i64]`

---

## Dict Methods (`dict`)

Dicts are key-value maps backed by `std::unordered_map<std::string, QValue>`.

### Static key access (dot syntax)

```quark
info = dict { name: 'Alex', role: 'dev' }
println(info.name)      // Alex
info.city = 'NYC'
len(info) | println()   // 3
```

The analyzer infers a value type from the literal. Assigning a value of another type (here, `info.age = 30` on a dict of strings) is allowed but produces a `QK-CHECK-002` warning.

### Dynamic key methods

Use methods when the key comes from a variable or expression:

| Method | Signature | Description |
|--------|-----------|-------------|
| `.get(key)` | `any -> any` | Get value by key; missing key returns `null` |
| `.set(key, val)` | `any, any -> dict` | Set value by key, in place; returns the same dict |
| `.keys()` | `-> list` | Return list of keys |
| `.values()` | `-> list` | Return list of values |
| `.items()` | `-> list` | Return list of `{ key, value }` records |

```quark
mydict = dict { a: 1, b: 2 }

// Dynamic key read
for item in list ['a', 'b', 'missing']:
    mydict.get(item) | println()    // 1, 2, null

// Dynamic key write
mydict.set('x', 99)
println(mydict.x)   // 99

// Enumerate
for item in mydict.items():
    println(item.key)
    println(item.value)

// Keys/values (order is not guaranteed)
len(mydict.keys()) | println()     // 3
len(mydict.values()) | println()   // 3
```

Keys are always strings. `.get` and `.set` convert a non-string key with `to_str`, so `mydict.set(1, 2)` stores the key `'1'`, and `mydict.get(1)` reads it back.

---

## Vector Methods (`vector`)

Typed vector operations for data-oriented workloads.

### Construction

```quark
vi = vector [1, 2, 3, 4]       // vector[i64]  (from literal)
vf = vector [1.0, 2.0, 3.0]    // vector[f64]  (from literal)
vs = vector ['a', 'b', 'c']    // vector[str]  (from literal)

// From a homogeneous list
v2 = list [10, 20, 30].to_vector()
println(type(v2))              // vector[i64]

// Free-function form of .to_vector()
v3 = vfrom_list(list [1.5, 2.5])
println(type(v3))              // vector[f64]
```

### Vector methods

| Method | Signature | Description |
|--------|-----------|-------------|
| `.get(idx)` | `int -> any` | Get scalar value at index. **Runtime error out of bounds** (unlike list `.get`, which returns `null`) |
| `.fillna(val)` | `any -> vector` | New vector with null entries replaced |
| `.astype(dtype)` | `str -> vector` | Cast vector dtype (`f64`, `i64`, `bool`) |
| `.to_list()` | `-> list` | Convert vector back to list |

### Vector free functions (reductions)

| Function | Signature | Description |
|----------|-----------|-------------|
| `sum` | `vector -> float` | Sum all elements |
| `min` | `vector -> float` | Minimum element |
| `max` | `vector -> float` | Maximum element |
| `all` | `vector[bool] -> bool` | True if every non-null element is true (true for empty) |
| `any` | `vector[bool] -> bool` | True if any non-null element is true (false for empty) |

```quark
vi = vector [1, 2, 3, 4]
println(sum(vi))              // 10
println(min(vi))              // 1
println(max(vi))              // 4

// Arithmetic (numeric vectors only)
a = vector [10, 20, 30, 40]
b = vector [1, 2, 3, 4]
z = a - b
println(sum(z))

// Cast
vf = vi.astype('f64')
println(type(vf))             // vector[f64]

// Fill nulls
filled = vf.fillna(0.0)

// Convert back to list
back = vi.to_list()
println(type(back))           // list
println(back.get(0))          // 1
```

**Notes:**
- Vector literals must be homogeneous (`int`, `float`, `bool`, or `str`)
- `.to_vector()` on a list applies the same homogeneity rule
- Numeric vector arithmetic (`+`, `-`, `*`, `/`) requires numeric vectors
- `sum` supports `vector[bool]` (true=1, false=0)
- `sum`, `min`, and `max` skip null entries in vectors; all-null reductions return `null`
- On an empty vector, `sum` returns `0.0`, while `min` and `max` are runtime errors

---

## Result Handling

Result values (`ok`/`err`) are Quark's explicit error mechanism.

| Function | Signature | Description |
|----------|-----------|-------------|
| `is_ok` | `result -> bool` | Returns `true` if the result is `ok` |
| `is_err` | `result -> bool` | Returns `true` if the result is `err` |
| `unwrap` | `result -> any` | Extract the `ok` value. **Runtime error if `err`.** |

```quark
fn safe_div(a, b) ->
    if b == 0:
        err 'division by zero'
    else:
        ok a / b

r = safe_div(10, 2)
is_ok(r) | println()       // true
unwrap(r) | println()      // 5

// Pattern matching (preferred)
when safe_div(10, 0):
    ok value -> println(value)
    err msg -> println(msg)      // division by zero
```

---

## File I/O Intrinsics

Low-level file I/O via `_file_*` builtins. The first-party module [`std/io`](#stdio) wraps these; prefer it in programs.
These functions are intentionally primitive. Invalid argument types, and reading, writing or seeking a closed handle, are runtime errors. `_file_close` on an already closed handle instead returns `err('close: resource is closed')`.

| Function | Signature | Description |
|----------|-----------|-------------|
| `_file_open` | `path: str, mode: str[, binary: bool] -> result[file_handle]` | Open file path with mode `r` (read), `w` (write: create or truncate) or `a` (append: create if missing, write at the end); any other mode returns `err("open: mode must be 'r', 'w' or 'a'")` |
| `_file_read` | `file: file_handle, n: int -> result[str]` | Read up to `n` bytes/chars |
| `_file_write` | `file: file_handle, data: str -> result[int]` | Write data and return bytes/chars written |
| `_file_close` | `file: file_handle -> result[null]` | Close file |
| `_file_seek` | `file: file_handle, offset: int, whence: int -> result[int]` | Seek (`0`=set, `1`=cur, `2`=end) |
| `_file_exists` | `path: str -> bool` | Check path existence |

```quark
f = unwrap(_file_open('notes.txt', 'r'))
chunk = unwrap(_file_read(f, 4096))
println(chunk)
unwrap(_file_close(f))
```

---

## `std/io`

File I/O over the `_file_*` intrinsics, with defaults. Import it with `use 'std/io' as io`.

| Function | Signature | Description |
|----------|-----------|-------------|
| `open` | `path: str, mode: str = 'r', binary: bool = false -> result` | Open a file (`r`, `w` or `a`, as `_file_open`) |
| `read` | `file: file_handle, n: int -> result` | Read up to `n` bytes/chars |
| `write` | `file: file_handle, data: str -> result` | Write data; `ok` holds the count written |
| `close` | `file: file_handle -> result` | Close the file |
| `seek` | `file: file_handle, offset: int, whence: int = 0 -> result` | Seek; `ok` holds the new position |
| `exists` | `path: str -> bool` | Check path existence |
| `seek_set`, `seek_cur`, `seek_end` | `-> int` | `whence` values `0`, `1`, `2` |

```quark
use 'std/io' as io

f = unwrap(io.open('notes.txt', 'w'))
println(unwrap(io.write(f, 'hello quark')))   // 11
unwrap(io.close(f))

g = unwrap(io.open('notes.txt'))
unwrap(io.seek(g, 6, io.seek_set()))
println(unwrap(io.read(g, 100)))              // quark
unwrap(io.close(g))

a = unwrap(io.open('notes.txt', 'a'))         // append
unwrap(io.write(a, '!'))
unwrap(io.close(a))
r = unwrap(io.open('notes.txt'))
println(unwrap(io.read(r, 100)))              // hello quark!
unwrap(io.close(r))

when io.open('notes.txt', 'x'):
    ok h -> println('opened')
    err msg -> println(msg)                   // open: mode must be 'r', 'w' or 'a'
```

---

## `std/fmt`

Text rendering of containers, which `println` only summarizes. Each function returns a `str`. Import it with `use 'std/fmt' as fmt`.

| Function | Signature | Description |
|----------|-----------|-------------|
| `show_list` | `lst: list, n: int = 10, show_index: bool = true -> str` | Render a list as a column, up to `n` items |
| `show_vec` | `vec: vector, n: int = 10, show_index: bool = true -> str` | Render a vector as a column, up to `n` items |
| `show_dict` | `dct: dict, n: int = 10, show_index: bool = true -> str` | Render a dict as key/value columns |
| `table` | `df: any, n: int = 20, show_index: bool = true -> str` | Render a table, or a dict of equal-length lists/vectors, as a boxed table |
| `head` | `lst: list, n: int = 5 -> str` | Render the first `n` items of a list |
| `tail` | `lst: list, n: int = 5 -> str` | Render the last `n` items of a list |

```quark
use 'std/fmt' as fmt

println(fmt.show_list(list [10, 20, 30]))
// Output:
//    value
// --------
// 0     10
// 1     20
// 2     30
// [list: 3 elements]
```

---

## `std/sys`

Program arguments, environment variables and exit status. Import it with `use 'std/sys' as sys`.

| Function | Signature | Description |
|----------|-----------|-------------|
| `args` | `-> list` | Command-line arguments after the program name, as strings. With `quark run`, arguments after `--` are passed: `quark run tool.qrk -- in.csv -v` |
| `env` | `name: str -> str` | Value of the environment variable `name`, or `null` when it isn't set |
| `exit` | `code: int = 0` | Exit immediately with status `code`, after flushing output. `0` means success |

```quark
use 'std/sys' as sys

args = sys.args()
if len(args) == 0:
    println('usage: tool <file>')
    sys.exit(2)
println('reading ' + args[0])
user = sys.env('USER')
if user != null:
    println('hello ' + user)
```

Run it with `quark run tool.qrk -- data.csv`, or build it once with `quark build tool.qrk -o tool` and run `./tool data.csv`.

---

## `std/csv`

Reading CSV data. Every field is returned as a `str`; convert with `to_int` / `to_float`. Import it with `use 'std/csv' as csv`.

| Function | Signature | Description |
|----------|-----------|-------------|
| `parse` | `text: str, sep: str = ',' -> list` | Rows of a CSV string, each a list of `str`. Malformed input (an unterminated quote) is a runtime error |
| `read` | `path: str, sep: str = ',' -> result` | `ok(rows)` for a CSV file, or `err(message)` if it can't be read or is malformed |

Parsing follows RFC 4180: a field may be quoted with `"..."`, and a quoted field can contain the separator, newlines and doubled quotes (`""` is one `"`). Lines end with `\n` or `\r\n`; fully blank lines are skipped. `sep` must be a single character (`';'`, `'\t'`, ...). The first row is returned like any other, so skip it when it's a header.

```quark
use 'std/csv' as csv

rows = csv.parse('name,city\n"Smith, J",NYC\n')
println(len(rows))                 // 2
println(rows.get(1).get(0))        // Smith, J

when csv.read('sales.csv'):
    ok rows -> println(len(rows))
    err e -> println(e)            // csv.read: sales.csv: No such file or directory
```

---

## `std/time`

Wall-clock and monotonic time. Import it with `use 'std/time' as time`.

| Function | Signature | Description |
|----------|-----------|-------------|
| `now` | `-> float` | Seconds since the Unix epoch (wall clock), with sub-second precision |
| `clock` | `-> float` | Monotonic seconds from an arbitrary starting point; subtract two readings to time code |
| `sleep` | `seconds: float` | Pause for `seconds` (fractions allowed) |

```quark
use 'std/time' as time

start = time.clock()
time.sleep(0.1)
println(time.clock() - start >= 0.1)   // true
```

---

## `std/random`

Pseudo-random numbers (64-bit Mersenne Twister). The generator is seeded randomly at startup; call `seed(n)` for a reproducible sequence. Not suitable for cryptography. Import it with `use 'std/random' as random`.

| Function | Signature | Description |
|----------|-----------|-------------|
| `seed` | `n: int` | Seed the generator; the same seed gives the same sequence |
| `uniform` | `-> float` | A float in `[0, 1)` |
| `randint` | `lo: int, hi: int -> int` | An int in `[lo, hi]`, both ends included; `lo > hi` is a runtime error |
| `choice` | `xs: list -> any` | A random element; an empty list is a runtime error |
| `shuffle` | `xs: list -> list` | Shuffle in place and return the list |

```quark
use 'std/random' as random

random.seed(7)
roll = random.randint(1, 6)
println(roll >= 1 and roll <= 6)            // true
deck = list ['a', 'b', 'c']
println(random.shuffle(deck).sort().join(''))   // abc
```

---

## `std/blas` (OpenBLAS)

BLAS routines over f64 vectors, through the system OpenBLAS. The module links it with `extern link 'openblas'`, so it needs the library and `cblas.h`:

- Debian/Ubuntu: `sudo apt install libopenblas-dev`
- macOS: `brew install openblas`, then pass `--lib-path "$(brew --prefix openblas)/lib"` and `--include-path "$(brew --prefix openblas)/include"` (or set `QUARK_LIB_PATH` / `QUARK_INCLUDE_PATH`).

Without OpenBLAS, `use 'std/blas'` fails at build time with `QK-FFI-002`.

| Function | Signature | Description |
|----------|-----------|-------------|
| `dot` | `x: vector, y: vector -> float` | Dot product |
| `axpy` | `a: float, x: vector, y: vector -> vector` | `a*x + y`, as a new vector |
| `scal` | `a: float, x: vector -> vector` | `a*x`, as a new vector |
| `nrm2` | `x: vector -> float` | Euclidean norm |
| `asum` | `x: vector -> float` | Sum of absolute values |
| `gemv` | `a: vector, rows: int, cols: int, x: vector -> vector` | Matrix-vector product; `a` is `rows x cols`, row-major, flattened |

```quark
use 'std/blas' as blas

x = vector [1.0, 2.0, 3.0]
y = vector [4.0, 5.0, 6.0]
println(blas.dot(x, y))         // 32
z = blas.axpy(2, x, y)          // [6, 9, 12]; y is unchanged
```

- BLAS reads the vectors' buffers in place; inputs are never modified.
- Inputs must be f64 vectors without nulls (convert with `.astype('f64')`, fill with `.fillna(...)`). Length mismatches, other dtypes, nulls and wrong `gemv` dimensions stop the program with a runtime error.
- Vectors are limited to `INT_MAX` elements, the CBLAS integer size.

---

## Pipes

All builtins work with Quark's pipe operator. The piped value becomes the first argument:

```quark
// Pipe into free function
'hello' | println()

// Pipe into method call: value | receiver.method(extra_args)
// The piped value becomes an extra argument after the receiver.

// Chain method calls then pipe to println
'  HELLO world  '.trim().lower().replace('world', 'quark') | println()

// Pipe into list.join
list ['a', 'b', 'c'].join(',') | println()   // a,b,c
```

---

## Error Contract Summary

| Category | Behavior | Examples |
|----------|----------|---------|
| Type mismatch (type known at compile time) | Compile-time error | `sqrt('hello')` (`sqrt` expects `float`) |
| Invalid value | Runtime error | `to_int('abc')` |
| Domain error | Runtime error | `.pop()` on empty list, `sqrt(-1)`, `min()` of an empty vector |
| Arity mismatch | Compile-time error | `.push()` (missing arg), `len(a, b)` (extra arg) |
| Unknown method | Compile-time error | `x = 42` then `x.upper()` (type 'int' has no method 'upper') |
| Truthiness | Accepted (not an error) | `if 1:`, `while 'yes':` use truthiness |
| Result misuse | Compile-time error | `x: int = some_result_fn()` |

**Intentional `null` returns (not errors):**
- `.get(idx)` on list when index is out of bounds (vector `.get` is a runtime error instead)
- `.get(key)` on dict when key is missing

---

For implementation details on how builtins are wired (catalog, runtime headers, codegen), see **architecture.md** §6.
