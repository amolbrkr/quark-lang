# Quark Standard Library

This document describes the built-in prelude functions and methods available in Quark.

Prelude builtins are globally available without imports. Additional stdlib modules are imported via `incl 'std/...'`, and extension modules may declare native bindings via QEI (`extern` / `extern fn`).

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
print('hello')

// Method on str
'hello world'.upper() | print()

// Method on list
xs = list [1, 2, 3]
xs.push(4).join(', ') | print()   // 1, 2, 3, 4

// Method on dict
d = dict { a: 1 }
d.get('a') | print()   // 1
```

**Dot-call vs dot-access:** Dot syntax has four uses:
- `d.key` — dict key read/write (static key access)
- `p.field` — struct field read/write
- `d.method(args)` — method dispatch (when method name is known for the receiver type)
- `module.fn(args)` — module-qualified call via `incl` alias

**How values print:** `print` and `to_str` show values by their contents: lists and vectors as `[1, 'a', null]` (strings inside a collection are quoted), dicts as `{age: 36, 'first name': 'Ada'}` (keys sorted, quoted unless they read like names), structs as `Point {x: 1, y: 2}` and results as `ok 5` / `err 'boom'`. A collection with more than 100 items shows its first and last 5 and a count: `[0, 1, 2, 3, 4, ..., 995, 996, 997, 998, 999] (1000 items)`; `fmt.full(x)` gives every item. A container that contains itself prints `[...]` at the repeat. Floats print as the shortest text that reads back as the same number, with `.0` on whole values, as in Python: `4.0`, `0.1`, `1.4142135623730951`, `1e+16`, `inf`, `nan`. So `0.1 + 0.2` prints `0.30000000000000004`.

---

## Core Free Functions

### I/O

| Function | Signature | Description |
|----------|-----------|-------------|
| `print` | `value[, end: str[, width: int[, align: str[, pad: str]]]] -> void` | Print value followed by `end` (default `'\n'`, so each call prints its own line); optional width/alignment |
| `input` | `[prompt: str] -> str` | Read a line from stdin, without its `\n` or `\r\n` ending; any length. Returns `null` at end of input (an empty line gives `''`). Optional prompt must be a string |

```quark
print('Hello, World!')
name = input('Name: ')
print(name)

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
print(count)
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
| `len` | `str\|list\|dict\|vec -> int` | Get length |
| `is_ok` | `result -> bool` | Returns `true` if the result is `ok` |
| `is_err` | `result -> bool` | Returns `true` if the result is `err` |
| `unwrap` | `result -> any` | Extract the `ok` value. **Runtime error if result is `err`.** |

```quark
to_str(42) | print()        // '42'
to_int('123') | print()     // 123
to_float('3.14') | print()  // 3.14
to_bool(0) | print()        // false
type(42) | print()          // int
len('hello') | print()      // 5
```

### Range

| Function | Signature | Description |
|----------|-----------|-------------|
| `range` | `number -> list` | Generate `[0, 1, ... end-1]` |
| `range` | `number, number -> list` | Generate `[start, ... end-1]`; counts down (`[start, ... end+1]`) when `start > end` |
| `range` | `number, number, number -> list` | Generate `[start, start+step, ...]`; `step` of `0` is a runtime error |

```quark
range(5).join(', ') | print()          // 0, 1, 2, 3, 4
range(2, 5).join(', ') | print()       // 2, 3, 4
range(5, 2).join(', ') | print()       // 5, 4, 3
range(10, 0, -2).join(', ') | print()  // 10, 8, 6, 4, 2
```

### Math

| Function | Signature | Description |
|----------|-----------|-------------|
| `abs` | `int\|float -> int\|float` | Absolute value (preserves type) |
| `min` | `number, number -> number` | Minimum of two values |
| `max` | `number, number -> number` | Maximum of two values |
| `sum` | `vec -> float` | Sum of all vector elements |
| `sqrt` | `float -> float` | Square root |
| `floor` | `float -> int` | Round down |
| `ceil` | `float -> int` | Round up |
| `round` | `float -> int` | Round to nearest |

```quark
abs(0 - 5) | print()       // 5
min(10, 5) | print()       // 5
max(10, 5) | print()       // 10
sqrt(16) | print()         // 4.0
floor(3.7) | print()       // 3
```

`min`, `max` and `sum` also take a single vector; see [Vector free functions](#vector-free-functions-reductions).

---

## String Methods (`str`)

All string methods leave the original string unchanged and return a new string (or value). Join strings with `+` (`'a' + 'b'`); both sides must be strings, so convert other values with `to_str` first.

| Method | Signature | Description |
|--------|-----------|-------------|
| `.upper()` | `str -> str` | Convert to uppercase |
| `.lower()` | `str -> str` | Convert to lowercase |
| `.trim()` | `str -> str` | Remove leading/trailing whitespace |
| `.contains(sub)` | `str -> bool` | Check if substring exists |
| `.startswith(prefix)` | `str -> bool` | Check if starts with prefix |
| `.endswith(suffix)` | `str -> bool` | Check if ends with suffix |
| `.replace(old, new)` | `str, str -> str` | Replace all occurrences |
| `.split(sep)` | `str -> list` | Split by separator |
| `.slice(start, end)` | `int, int -> str` | Substring `[start:end)` (negative indices supported) |

```quark
// Case
'hello world'.upper() | print()     // HELLO WORLD
'HELLO WORLD'.lower() | print()     // hello world

// Whitespace
'  hello  '.trim() | print()        // hello

// Searching
'hello world'.contains('world') | print()    // true
'hello world'.startswith('hello') | print()  // true
'hello world'.endswith('world') | print()    // true

// Manipulation
'hello world'.replace('world', 'quark') | print()  // hello quark
'hello ' + 'world' | print()                        // hello world

// Split
parts = 'a,b,c'.split(',')
len(parts) | print()                // 3
parts.get(1) | print()              // b

// Slice
'hello world'.slice(6, 11) | print()  // world
'hello'.slice(-3, 5) | print()        // llo   (negative start)
'hello'.slice(0, -2) | print()        // hel   (negative end)

// Chaining
'hello world'.slice(6, 11).upper() | print()  // WORLD

// Chain with pipe
'  HELLO world  '.trim().lower().replace('world', 'quark') | print()
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
| `.enumerate()` | `-> list` | Build list of `{ index, value }` records: `for e in xs.enumerate(): print(e.index)` |
| `.to_vec()` | `-> vec` | Convert to typed vector (requires homogeneous elements; nulls allowed) |

```quark
xs = list [10, 20, 30]
xs.push(40)                    // changes xs in place
xs.get(0) | print()          // 10
xs.get(-1) | print()         // 40 (last element)
xs.get(999) | print()        // null (out of bounds)

xs.pop() | print()           // 40
xs.set(1, 99) | print()      // 99 (the value set)
xs.insert(1, 55)

xs.slice(0, 2).join(', ') | print()   // 10, 55
xs.reverse().join(', ') | print()     // 30, 99, 55, 10
xs.sort().join(', ') | print()        // 10, 30, 55, 99

rows = list [dict {name: 'b', n: 2}, dict {name: 'a', n: 1}]
rows.sort_by(fn(r) -> r.n)
rows.get(0).name | print()            // a

xs2 = list [1, 2]
xs.concat(xs2).join(', ') | print()   // 10, 30, 55, 99, 1, 2

// Join
list ['a', 'b', 'c'].join(',') | print()  // a,b,c
list [1, 2, 3].join('+') | print()        // 1+2+3

// Split then rejoin
'a,b,c'.split(',').join('-') | print()    // a-b-c

// Convert to vector
v = list [1, 2, 3].to_vec()
print(type(v))  // vec[i64]
```

**Notes:**
- `.push`, `.insert`, `.set`, `.remove`, `.pop`, `.reverse`, `.sort` and `.sort_by` change the list in place; `.push` and `.insert` also return it, so calls can be chained, but no reassignment is needed
- `.get` out-of-bounds returns `null`; `.set` and `.remove` out of bounds are runtime errors
- `.insert` clamps the index: a negative index counts from the end, and anything past the end appends (`list [1, 2, 3].insert(99, 7)` gives `1, 2, 3, 7`); a non-int index is a runtime error
- `.join` coerces non-string elements via `to_str`
- `.to_vec` requires all non-null elements to be the same type (`int`, `float`, `bool`, or `str`); nulls become null entries, and an empty list gives a `vec[i64]`

---

## Dict Methods (`dict`)

Dicts are key-value maps backed by `std::unordered_map<std::string, QValue>`.

### Static key access (dot syntax)

```quark
info = dict { name: 'Alex', role: 'dev' }
print(info.name)      // Alex
info.city = 'NYC'
len(info) | print()   // 3
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
    mydict.get(item) | print()    // 1, 2, null

// Dynamic key write
mydict.set('x', 99)
print(mydict.x)   // 99

// Enumerate
for item in mydict.items():
    print(item.key)
    print(item.value)

// Keys/values (order is not guaranteed)
len(mydict.keys()) | print()     // 3
len(mydict.values()) | print()   // 3
```

Keys are always strings. `.get` and `.set` convert a non-string key with `to_str`, so `mydict.set(1, 2)` stores the key `'1'`, and `mydict.get(1)` reads it back.

---

## Vector Methods (`vec`)

Typed vector operations for data-oriented workloads.

### Construction

```quark
vi = vec [1, 2, 3, 4]       // vec[i64]  (from literal)
vf = vec [1.0, 2.0, 3.0]    // vec[f64]  (from literal)
vs = vec ['a', 'b', 'c']    // vec[str]  (from literal)

// From a homogeneous list
v2 = list [10, 20, 30].to_vec()
print(type(v2))              // vec[i64]
```

### Vector methods

| Method | Signature | Description |
|--------|-----------|-------------|
| `.get(idx)` | `int -> any` | Get scalar value at index. **Runtime error out of bounds** (unlike list `.get`, which returns `null`) |
| `.fillna(val)` | `any -> vec` | New vector with null entries replaced |
| `.astype(dtype)` | `str -> vec` | Cast vector dtype (`f64`, `i64`, `bool`) |
| `.to_list()` | `-> list` | Convert vector back to list |
| `.enumerate()` | `-> list` | Build list of `{ index, value }` records, as for lists |

### Vector free functions (reductions)

| Function | Signature | Description |
|----------|-----------|-------------|
| `sum` | `vec -> float` | Sum all elements |
| `min` | `vec -> float` | Minimum element |
| `max` | `vec -> float` | Maximum element |
| `all` | `vec[bool] -> bool` | True if every non-null element is true (true for empty) |
| `any` | `vec[bool] -> bool` | True if any non-null element is true (false for empty) |

```quark
vi = vec [1, 2, 3, 4]
print(sum(vi))              // 10.0 (reductions return a float)
print(min(vi))              // 1.0
print(max(vi))              // 4.0

// Arithmetic (numeric vectors only)
a = vec [10, 20, 30, 40]
b = vec [1, 2, 3, 4]
z = a - b
print(sum(z))

// Cast
vf = vi.astype('f64')
print(type(vf))             // vec[f64]

// Fill nulls
filled = vf.fillna(0.0)

// Convert back to list
back = vi.to_list()
print(type(back))           // list
print(back.get(0))          // 1
```

**Notes:**
- Vector literals must be homogeneous (`int`, `float`, `bool`, or `str`)
- `.to_vec()` on a list applies the same homogeneity rule
- Numeric vector arithmetic (`+`, `-`, `*`, `/`) requires numeric vectors
- `sum` supports `vec[bool]` (true=1, false=0)
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
is_ok(r) | print()       // true
unwrap(r) | print()      // 5.0

// Pattern matching (preferred)
when safe_div(10, 0):
    ok value -> print(value)
    err msg -> print(msg)      // division by zero
```

---

## Stdlib modules: what an import exposes

Each module below is a `.qrk` file whose public API is its `module` block (`io.open`, `fmt.show_vec`, ...). The `extern fn` bindings it is built on (`io_open`, `fmt_vec`, `blas_dot`, ...) are declared at the file's top level, and every top-level name of an imported file is visible to the program, alias or not. They are callable, but they are implementation details: they take every argument explicitly (no defaults) and may change. Use the module functions.

---

## `std/io`

File I/O. Import it with `incl 'std/io' as io`. The module is `extern fn` bindings over `src/core/stdlib/io.hpp`, like the other stdlib modules; there are no file builtins in the prelude. An open file is a `resource` (`type(f)` is `'resource'`; it prints as `<resource file>`).

Reading, writing or seeking a closed file, and invalid argument types, are runtime errors. `close` on an already closed file instead returns `err('close: resource is closed')`.

| Function | Signature | Description |
|----------|-----------|-------------|
| `open` | `path: str, mode: str = 'r', binary: bool = false -> result` | Open a file with mode `r` (read), `w` (write: create or truncate) or `a` (append: create if missing, write at the end); `ok` holds the file. Any other mode returns `err("open: mode must be 'r', 'w' or 'a'")` |
| `read` | `file: resource, n: int -> result` | Read up to `n` bytes; `ok('')` at end of file |
| `write` | `file: resource, data: str -> result` | Write data; `ok` holds the count written |
| `close` | `file: resource -> result` | Close the file; `ok(null)` |
| `seek` | `file: resource, offset: int, whence: int = 0 -> result` | Seek (`0` = start, `1` = current, `2` = end); `ok` holds the new position |
| `exists` | `path: str -> bool` | Check path existence |
| `seek_set`, `seek_cur`, `seek_end` | `-> int` | `whence` values `0`, `1`, `2` |

```quark
incl 'std/io' as io

f = unwrap(io.open('notes.txt', 'w'))
print(unwrap(io.write(f, 'hello quark')))   // 11
unwrap(io.close(f))

g = unwrap(io.open('notes.txt'))
unwrap(io.seek(g, 6, io.seek_set()))
print(unwrap(io.read(g, 100)))              // quark
unwrap(io.close(g))

a = unwrap(io.open('notes.txt', 'a'))         // append
unwrap(io.write(a, '!'))
unwrap(io.close(a))
r = unwrap(io.open('notes.txt'))
print(unwrap(io.read(r, 100)))              // hello quark!
unwrap(io.close(r))

when io.open('notes.txt', 'x'):
    ok h -> print('opened')
    err msg -> print(msg)                   // open: mode must be 'r', 'w' or 'a'
```

---

## `std/fmt`

Text rendering of containers beyond what `print` shows. Each function returns a `str`. Import it with `incl 'std/fmt' as fmt`.

| Function | Signature | Description |
|----------|-----------|-------------|
| `show_list` | `lst: list, n: int = 10, show_index: bool = true -> str` | Render a list as a column, up to `n` items |
| `show_vec` | `v: vec, n: int = 10, show_index: bool = true -> str` | Render a vector as a column, up to `n` items |
| `show_dict` | `dct: dict, n: int = 10, show_index: bool = true -> str` | Render a dict as key/value columns |
| `table` | `df: any, n: int = 20, show_index: bool = true -> str` | Render a table, or a dict of equal-length lists/vectors, as a boxed table |
| `head` | `lst: list, n: int = 5 -> str` | Render the first `n` items of a list |
| `tail` | `lst: list, n: int = 5 -> str` | Render the last `n` items of a list |
| `full` | `x: any -> str` | `x` as `print` shows it, but with every item of long collections |

```quark
incl 'std/fmt' as fmt

print(fmt.show_list(list [10, 20, 30]))
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

Program arguments, environment variables and exit status. Import it with `incl 'std/sys' as sys`.

| Function | Signature | Description |
|----------|-----------|-------------|
| `args` | `-> list` | Command-line arguments after the program name, as strings. With `quark run`, arguments after `--` are passed: `quark run tool.qrk -- in.csv -v` |
| `env` | `name: str -> str` | Value of the environment variable `name`, or `null` when it isn't set |
| `exit` | `code: int = 0` | Exit immediately with status `code`, after flushing output. `0` means success |

```quark
incl 'std/sys' as sys

args = sys.args()
if len(args) == 0:
    print('usage: tool <file>')
    sys.exit(2)
print('reading ' + args[0])
user = sys.env('USER')
if user != null:
    print('hello ' + user)
```

Run it with `quark run tool.qrk -- data.csv`, or build it once with `quark build tool.qrk -o tool` and run `./tool data.csv`.

---

## `std/csv`

Reading CSV data. Every field is returned as a `str`; convert with `to_int` / `to_float`. Import it with `incl 'std/csv' as csv`.

| Function | Signature | Description |
|----------|-----------|-------------|
| `parse` | `text: str, sep: str = ',' -> list` | Rows of a CSV string, each a list of `str`. Malformed input (an unterminated quote) is a runtime error |
| `read` | `path: str, sep: str = ',' -> result` | `ok(rows)` for a CSV file, or `err(message)` if it can't be read or is malformed |

Parsing follows RFC 4180: a field may be quoted with `"..."`, and a quoted field can contain the separator, newlines and doubled quotes (`""` is one `"`). Lines end with `\n` or `\r\n`; fully blank lines are skipped. `sep` must be a single character (`';'`, `'\t'`, ...). The first row is returned like any other, so skip it when it's a header.

```quark
incl 'std/csv' as csv

rows = csv.parse('name,city\n"Smith, J",NYC\n')
print(len(rows))                 // 2
print(rows.get(1).get(0))        // Smith, J

when csv.read('sales.csv'):
    ok rows -> print(len(rows))
    err e -> print(e)            // csv.read: sales.csv: No such file or directory
```

---

## `std/time`

Wall-clock and monotonic time. Import it with `incl 'std/time' as time`.

| Function | Signature | Description |
|----------|-----------|-------------|
| `now` | `-> float` | Seconds since the Unix epoch (wall clock), with sub-second precision |
| `clock` | `-> float` | Monotonic seconds from an arbitrary starting point; subtract two readings to time code |
| `sleep` | `seconds: float` | Pause for `seconds` (fractions allowed) |

```quark
incl 'std/time' as time

start = time.clock()
time.sleep(0.1)
print(time.clock() - start >= 0.1)   // true
```

---

## `std/random`

Pseudo-random numbers (64-bit Mersenne Twister). The generator is seeded randomly at startup; call `seed(n)` for a reproducible sequence. Not suitable for cryptography. Import it with `incl 'std/random' as random`.

| Function | Signature | Description |
|----------|-----------|-------------|
| `seed` | `n: int` | Seed the generator; the same seed gives the same sequence |
| `uniform` | `-> float` | A float in `[0, 1)` |
| `randint` | `lo: int, hi: int -> int` | An int in `[lo, hi]`, both ends included; `lo > hi` is a runtime error |
| `choice` | `xs: list -> any` | A random element; an empty list is a runtime error |
| `shuffle` | `xs: list -> list` | Shuffle in place and return the list |

```quark
incl 'std/random' as random

random.seed(7)
roll = random.randint(1, 6)
print(roll >= 1 and roll <= 6)            // true
deck = list ['a', 'b', 'c']
print(random.shuffle(deck).sort().join(''))   // abc
```

---

## `std/blas` (OpenBLAS)

BLAS routines over f64 vectors, through the system OpenBLAS. The module links it with `extern link 'openblas'`, so it needs the library and `cblas.h`:

- Debian/Ubuntu: `sudo apt install libopenblas-dev`
- macOS: `brew install openblas`, then pass `--lib-path "$(brew --prefix openblas)/lib"` and `--include-path "$(brew --prefix openblas)/include"` (or set `QUARK_LIB_PATH` / `QUARK_INCLUDE_PATH`).

Without OpenBLAS, `incl 'std/blas'` fails at build time with `QK-FFI-002`.

| Function | Signature | Description |
|----------|-----------|-------------|
| `dot` | `x: vec, y: vec -> float` | Dot product |
| `axpy` | `a: float, x: vec, y: vec -> vec` | `a*x + y`, as a new vector |
| `scal` | `a: float, x: vec -> vec` | `a*x`, as a new vector |
| `nrm2` | `x: vec -> float` | Euclidean norm |
| `asum` | `x: vec -> float` | Sum of absolute values |
| `gemv` | `a: vec, rows: int, cols: int, x: vec -> vec` | Matrix-vector product; `a` is `rows x cols`, row-major, flattened |

```quark
incl 'std/blas' as blas

x = vec [1.0, 2.0, 3.0]
y = vec [4.0, 5.0, 6.0]
print(blas.dot(x, y))         // 32.0
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
'hello' | print()

// Pipe into method call: value | receiver.method(extra_args)
// The piped value becomes an extra argument after the receiver.

// Chain method calls then pipe to print
'  HELLO world  '.trim().lower().replace('world', 'quark') | print()

// Pipe into list.join
list ['a', 'b', 'c'].join(',') | print()   // a,b,c
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
