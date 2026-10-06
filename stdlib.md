# Quark Standard Library

This document describes the built-in prelude functions and methods available in Quark.

Prelude builtins are globally available without imports. Additional stdlib modules are imported via `use 'std/...'`, and extension modules may declare native bindings via QEI (`extern` / `extern fn`).

## QEI and Lowering Notes

- QEI extern declarations are part of the language surface:
    - `extern 'path.hpp'`
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
xs.push(4) | println()

// Method on dict
d = dict { a: 1 }
d.get('a') | println()
```

**Dot-call vs dot-access:** Dot syntax serves two purposes:
- `d.key` — dict key read/write (static key access)
- `d.method(args)` — method dispatch (when method name is known for the receiver type)
- `module.fn(args)` — module-qualified call via `use` alias

---

## Core Free Functions

### I/O

| Function | Signature | Description |
|----------|-----------|-------------|
| `print` | `value[, end: str[, width: int[, align: str[, pad: str]]]] -> void` | Print value with configurable line ending and optional width/alignment |
| `println` | `value -> void` | Print value with newline |
| `input` | `[prompt: str] -> str` | Read line from stdin; optional prompt must be a string |

```quark
println('Hello, World!')
name = input('Name: ')
println(name)

print('row', '|', 8, 'left', '.')
print('42', '\n', 8, 'right', '0')
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
| `range` | `number, number -> list` | Generate `[start, ... end-1]` |
| `range` | `number, number, number -> list` | Generate `[start, start+step, ...]` |

```quark
range(5) | println()          // [0, 1, 2, 3, 4]
range(2, 5) | println()       // [2, 3, 4]
range(10, 0, -2) | println()  // [10, 8, 6, 4, 2]
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
sqrt(16) | println()         // 4.0
floor(3.7) | println()       // 3
```

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
'a,b,c'.split(',') | println()        // [a, b, c]

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
| `.push(item)` | `any -> list` | Add item to end; returns updated list |
| `.pop()` | `-> any` | Remove and return last item. **Runtime error on empty list.** |
| `.get(idx)` | `int -> any` | Get item at index (negative supported; out-of-bounds → `null`) |
| `.set(idx, val)` | `int, any -> any` | Set item at index; returns the value set |
| `.insert(idx, val)` | `int, any -> list` | Insert item at index; returns updated list |
| `.remove(idx)` | `int -> any` | Remove and return item at index |
| `.slice(start, end)` | `int, int -> list` | Sublist `[start:end)` |
| `.reverse()` | `-> list` | Reverse list in place; returns the list |
| `.concat(other)` | `list -> list` | Concatenate two lists |
| `.join(sep)` | `str -> str` | Join elements with separator (non-strings are coerced) |
| `.enumerate()` | `-> list` | Build list of `{ index, value }` records |
| `.to_vector()` | `-> vector` | Convert to typed vector (requires homogeneous elements) |

```quark
xs = list [10, 20, 30]
xs = xs.push(40)
xs.get(0) | println()          // 10
xs.get(-1) | println()         // 40 (last element)
xs.get(999) | println()        // null (out of bounds)

xs.pop() | println()           // 40
xs.set(1, 99) | println()      // 99 (the value set)
xs = xs.insert(1, 55)

xs.slice(0, 2) | println()     // [10, 55]
xs.reverse() | println()

xs2 = list [1, 2]
xs.concat(xs2) | println()

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
- `.push` and `.insert` return the modified list; reassign to update the variable: `xs = xs.push(val)`
- `.get` out-of-bounds returns `null`; `.set`, `.insert`, `.remove` with invalid args are runtime errors
- `.join` coerces non-string elements via `to_str`
- `.to_vector` requires all elements to be the same type (`int`, `float`, `bool`, or `str`)

---

## Dict Methods (`dict`)

Dicts are key-value maps backed by `std::unordered_map<std::string, QValue>`.

### Static key access (dot syntax)

```quark
info = dict { name: 'Alex', age: 30 }
println(info.name)      // Alex
info.city = 'NYC'
len(info) | println()   // 3
```

### Dynamic key methods

Use methods when the key comes from a variable or expression:

| Method | Signature | Description |
|--------|-----------|-------------|
| `.get(key)` | `any -> any` | Get value by key; missing key returns `null` |
| `.set(key, val)` | `any, any -> dict` | Set value by key; returns the updated dict |
| `.keys()` | `-> list` | Return list of keys |
| `.values()` | `-> list` | Return list of values |
| `.items()` | `-> list` | Return list of `{ key, value }` records |

```quark
mydict = dict { a: 1, b: 2 }

// Dynamic key read
for item in list ['a', 'b', 'missing']:
    mydict.get(item) | println()    // 1, 2, null

// Dynamic key write
mydict = mydict.set('x', 99)
println(mydict.x)   // 99

// Enumerate
for item in mydict.items():
    println(item.key)
    println(item.value)

// Keys/values
mydict.keys() | println()
mydict.values() | println()
```

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
```

### Vector methods

| Method | Signature | Description |
|--------|-----------|-------------|
| `.get(idx)` | `int -> any` | Get scalar value at index |
| `.fillna(val)` | `any -> vector` | Replace null entries |
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

Low-level file I/O via `_file_*` builtins. The first-party module `std/io` wraps these.
These functions are intentionally primitive and fail loudly on invalid argument types.

| Function | Signature | Description |
|----------|-----------|-------------|
| `_file_open` | `path: str, mode: str[, binary: bool] -> result[file_handle]` | Open file path with mode (`r`, `w`, `a`, etc.) |
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
| Type mismatch | Runtime error | `to_int('abc')`, `sqrt('hello')` |
| Domain error | Runtime error | `.pop()` on empty list, `sqrt(-1)` |
| Arity mismatch | Compile-time error | `.push()` (missing arg), `len(a, b)` (extra arg) |
| Unknown method | Compile-time error | `42.upper()` (int has no method `upper`) |
| Truthiness | Accepted (not an error) | `if 1:`, `while 'yes':` use truthiness |
| Result misuse | Compile-time error | `x: int = some_result_fn()` |

**Intentional `null` returns (not errors):**
- `.get(idx)` on list when index is out of bounds
- `.get(key)` on dict when key is missing

---

For implementation details on how builtins are wired (catalog, runtime headers, codegen), see **architecture.md** §6.
