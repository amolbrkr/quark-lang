# Quark Standard Library

This document describes the built-in functions available in Quark. All standard library functions are implemented in C++ for performance and are automatically available without any imports.

## Core Functions

### I/O Functions

| Function | Signature | Description |
|----------|-----------|-------------|
| `print` | `any -> void` | Print value without newline |
| `println` | `any -> void` | Print value with newline |
| `input` | `[prompt] -> string` | Read line from stdin; optional prompt string |

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
| `str` | `any -> string` | Convert to string |
| `int` | `any -> int` | Convert to integer |
| `float` | `any -> float` | Convert to float |
| `bool` | `any -> bool` | Convert to boolean (truthiness) |
| `len` | `string\|list\|dict -> int` | Get length of string, list, or dict |

```quark
str(42) | println()           // '42'
int('123') | println()        // 123
float('3.14') | println()     // 3.14
bool(0) | println()           // false
bool(1) | println()           // true
len('hello') | println()      // 5

dict { a: 1, b: 2 } | len() | println()  // 2
```

### Range

| Function | Signature | Description |
|----------|-----------|-------------|
| `range` | `int -> list[int]` | Generate `[0, 1, ... end-1]` |
| `range` | `int, int -> list[int]` | Generate `[start, ... end-1]` |
| `range` | `int, int, int -> list[int]` | Generate `[start, start+step, ...]` |

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

Use dot access for static, identifier keys:

```quark
info = dict { name: 'Alex', age: 30 }
println(info.name)      // 'Alex'
info.city = 'NYC'
```

### Dynamic keys (variable/expression)

If the key comes from a variable/expression, use these helpers:

| Function | Signature | Description |
|----------|-----------|-------------|
| `dget` | `dict, any -> any` | Get value by key (key is converted to string); missing key returns `null` |
| `dset` | `dict, any, any -> dict` | Set value by key (key is converted to string); returns the dict |

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
| `min` | `vector -> float` | Minimum element in vector |
| `max` | `number, number -> number` | Maximum of two values |
| `max` | `vector -> float` | Maximum element in vector |
| `sum` | `vector -> float` | Sum of all vector elements |
| `vadd_inplace` | `vector, number -> vector` | Add scalar to each vector element in place |
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

// Vector reductions
v = vector [1, 2, 3, 4]
sum(v) | println()              // 10
min(v) | println()              // 1
max(v) | println()              // 4

// Vector in-place scalar update
vadd_inplace(v, 1)
sum(v) | println()              // 14
```

### Notes

- `abs` preserves the input type (int returns int, float returns float)
- `min` and `max` return float if either argument is float
- `sqrt` always returns float
- `floor`, `ceil`, `round` return int

## String Functions

String manipulation functions implemented in C.

| Function | Signature | Description |
|----------|-----------|-------------|
| `upper` | `string -> string` | Convert to uppercase |
| `lower` | `string -> string` | Convert to lowercase |
| `trim` | `string -> string` | Remove leading/trailing whitespace |
| `contains` | `string, string -> bool` | Check if contains substring |
| `startswith` | `string, string -> bool` | Check if starts with prefix |
| `endswith` | `string, string -> bool` | Check if ends with suffix |
| `replace` | `string, string, string -> string` | Replace all occurrences |
| `concat` | `string, string -> string` | Concatenate two strings |
| `split` | `string, string -> list[string]` | Split string by separator |

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
  - `upper ''` returns `''`
  - `trim ''` returns `''`
  - `contains '', 'x'` returns `false`
  - `replace 'hello', '', 'x'` returns `'hello'` (no-op for empty pattern)

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

All standard library functions are implemented as C++ functions in the header-only runtime library at `runtime/include/quark/`. The concatenated header is embedded in `codegen/runtime.hpp` and compiled directly into the output binary, requiring no external dependencies except the C++ standard library and math library (`-lm`).

### Adding New Functions

To add a new built-in function:

1. **Add C++ implementation** in appropriate header under `runtime/include/quark/`
2. **Regenerate runtime.hpp** by running `build_runtime.ps1`
3. **Register the builtin mapping** in `src/core/quark/codegen/builtins.go`
4. **Register type signature** in `src/core/quark/types/analyzer.go`

Example C++ implementation pattern:
```cpp
inline QValue q_myfunc(QValue v) {
    // Type checking
    if (v.type != QValue::VAL_STRING) return qv_string("");

    // Implementation
    char* result = do_something(v.data.string_val);

    // Return boxed value
    QValue q = qv_string(result);
    free(result);  // Clean up temporary
    return q;
}
```

## Future Modules

Planned but not yet implemented:

- **Higher-order list functions**: `map`, `filter`, `reduce`, `sort`
- **File I/O**: `read_file`, `write_file`, `exists`
- **JSON**: `parse_json`, `to_json`
- **Time**: `now`, `sleep`, `format_time`
- **Random**: `random`, `random_int`, `shuffle`
