# Quark

**Quark** is a high-level, dynamically-typed language that compiles to C++17. It aims for Python-like syntax with native performance through a small Go frontend and a lightweight C++ runtime.

## What Works Today

- Indentation-based blocks and a Pratt parser for expressions
- Functions, lambdas, and closures with shared mutable captures
- If/elseif/else, `when` pattern matching (with `ok`/`err` result patterns), `for`/`while` loops, ternary
- Pipe operator for data-flow style calls
- Unified invocation model: `callable(entity, ...)` for all builtins and user functions
- Result values (`ok`/`err`) with pattern matching and helpers (`is_ok`, `is_err`, `unwrap`)
- Lists backed by `std::vector<QValue>` with indexing
- Typed 1D vectors (`f64`, `i64`, `bool`, `str`) with arithmetic, reductions, and boolean mask filtering
- Dicts backed by `std::unordered_map<std::string, QValue>` with dot access (data only, no method dispatch)
- 40+ builtins for I/O, math, strings, lists, dicts, and vectors
- Multi-file module imports (`use './path'`) with circular import detection
- Strict error enforcement: bool-only conditions, type-checked arguments, runtime errors crash with clear messages (no silent null returns)
- Boehm GC for automatic memory management

## Quick Example

```quark
fn factorial(n) ->
    when n:
        0 or 1 -> 1
        _ -> n * factorial(n - 1)

factorial(10) | println()

fn fizzbuzz(n) ->
    when n % 15:
        0 -> 'FizzBuzz'
        _ -> when n % 3:
            0 -> 'Fizz'
            _ -> when n % 5:
                0 -> 'Buzz'
                _ -> n

for i in range(1, 21):
    fizzbuzz(i) | println()
```

## Installation

```bash
git clone https://github.com/user/quark-lang.git
cd quark-lang

cd src/core/quark
go build -o quark .

./quark run ../../../src/testfiles/smoke_syntax.qrk
```

### Requirements

- **Go 1.21+**
- **clang++ or g++** in PATH (for C++17 codegen)
- **CMake** in PATH (used to auto-build vendored Boehm GC on first run/build)
- **Windows, Linux, or macOS**

### Boehm GC (auto-bootstrapped)

Quark vendors Boehm GC source under `deps/bdwgc`. When you run `quark run` or `quark build`, the compiler automatically configures/builds `deps/bdwgc/build` if no GC library is present, then links it.

Manual build is still possible if you prefer:

```bash
cd deps/bdwgc
cmake -S . -B build
cmake --build build
```

## Usage

```bash
./quark lex program.qrk
./quark parse program.qrk
./quark check program.qrk
./quark emit program.qrk      # Emits C++ to stdout
./quark build program.qrk -o myapp
./quark run program.qrk --debug   # Keeps a .cpp next to the source
```

## Language Notes

### Invocation Model

Quark uses a single canonical call form for all operations — no method-style dispatch:

```quark
// Canonical: callable(entity, ...)
push(mylist, 42)
upper('hello')
len(mylist)

// Pipe equivalent: entity | callable(...)
'hello' | upper() | println()
mylist | len() | println()

// Dot is data-only (dict key access)
d = dict { name: 'Quark' }
println(d.name)        // OK: dict key read
d.version = 1          // OK: dict key write
// mylist.push(42)     // ERROR: dot-call not supported
```

### Variables and Functions

```quark
x = 42
name = 'Quark'
pi = 3.14159

fn greet(name) ->
    println(concat('Hello, ', name))

// Closures capture enclosing variables
fn counter(start) ->
    n = start
    fn() ->
        n = n + 1
        n
```

### Control Flow

```quark
if x > 10:
    println('big')
elseif x > 5:
    println('medium')
else:
    println('small')

when value:
    0 or 1 -> 'zero or one'
    2 -> 'two'
    _ -> 'other'

result = 'yes' if condition else 'no'
```

### Result Values

```quark
fn safe_div(a, b) ->
    if b == 0:
        err 'division by zero'
    else:
        ok a / b

when safe_div(10, 3):
    ok value -> println(value)
    err msg -> println(msg)

// Helpers
is_ok(safe_div(10, 2)) | println()   // true
unwrap(safe_div(10, 2)) | println()  // 5
```

### Loops and Pipes

```quark
for i in range(0, 10):
    println(i)

while x > 0:
    x = x - 1

'hello world' | upper() | println()
```

### Lists

```quark
nums = list [1, 2, 3]
first = nums[0]
push(nums, 4)
len(nums) | println()
```

### Vectors

```quark
v = vector [1, 2, 3, 4]
w = v + 1
u = v * w

// Boolean mask filtering
big = v[v > 2]

// Reductions
println(sum(v))
println(min(v))
println(max(v))
```

Vector literals must be homogeneous:

- `vector [1, 2, 3]` -> `vector[i64]`
- `vector [1.0, 2.0]` -> `vector[f64]`
- `vector ['a', 'b']` -> `vector[str]`
- Mixed literals (for example `vector [1, '2', 3]`) are a type error
- Vector arithmetic `+ - * /` is numeric-only

### Dicts

```quark
d = dict { a: 1, b: 2 }
println(d.a)

// Dynamic key (variable/expression)
k = 'a'
println(dget(d, k))
d = dset(d, 'c', 3)
println(d.c)
```

### Modules

```quark
// Same-file modules
module math:
    fn square(n) -> n * n

use math
square(5) | println()

// Multi-file imports
use './utils'
```

## Standard Library (Builtins)

| Category | Functions |
| --- | --- |
| **I/O** | `print`, `println`, `input` |
| **Types** | `to_str`, `to_int`, `to_float`, `to_bool`, `type`, `len` |
| **Math** | `abs`, `min`, `max`, `sum`, `sqrt`, `floor`, `ceil`, `round` |
| **String** | `upper`, `lower`, `trim`, `contains`, `startswith`, `endswith`, `replace`, `concat`, `split` |
| **List** | `push`, `pop`, `get`, `set`, `insert`, `remove`, `slice`, `reverse`, `range` |
| **Dict** | `dget`, `dset` |
| **Vector** | `fillna`, `astype`, `to_vector`, `to_list` |
| **Result** | `is_ok`, `is_err`, `unwrap` |

See [stdlib.md](stdlib.md) for details.

## Architecture

```
Source (.qrk) -> Lexer -> Parser -> Analyzer -> C++ Codegen -> clang++/g++ -> Binary
```

- **Frontend**: Go (lexer, Pratt parser, semantic analyzer, C++ codegen)
- **Backend**: C++17 with `-O3 -march=native`
- **Runtime**: Header-only C++ library (`runtime/include/quark/`)
- **Memory**: Boehm GC (auto-bootstrapped from `deps/bdwgc`)

## Error Handling

Quark enforces strict error contracts at both compile time and runtime:

- **Compile time**: The analyzer checks argument counts, type compatibility (when inferable), bool-only conditions, result-to-scalar assignment, and reserved feature usage
- **Runtime**: All type/domain violations crash with a clear error message and non-zero exit — no silent null returns
- **Documented null**: Only `get()` (out-of-bounds) and `dget()` (missing key) return null by design

## Status

Implemented and stable:

- Full compiler pipeline (lexer, parser, analyzer, codegen)
- Functions, closures, lambdas as first-class values
- All control flow (if/elseif/else, when, for, while, ternary, pipes)
- Result types with `ok`/`err`/`when` pattern matching and `is_ok`/`is_err`/`unwrap`
- Lists, dicts, typed vectors with 40+ builtins
- Multi-file module imports with circular detection
- Boehm GC integration
- Strict error enforcement (compile-time and runtime)

Not yet implemented:

- Structs and impl blocks
- Tensor type (N-dimensional arrays)
- String interpolation
- `break`/`continue` in loops
- Stdlib module imports (non-relative `use 'name'`)

## Tests

Quark has two test layers:

- **Go tests** for the compiler frontend plus an **end-to-end smoke** test that compiles and runs Quark programs.
- **C++ Catch2 tests** for the runtime library.

```bash
# End-to-end smoke tests
cd src/core/quark
go test -run TestSmokePrograms_Run -v

# All Go tests
go test ./...

# Runtime unit tests (Catch2)
cd src/core/quark/runtime
cmake -S . -B build-tests
cmake --build build-tests
ctest --test-dir build-tests --output-on-failure
```

## Contributing

Contributions welcome. Priorities:

- Smoke programs in [src/testfiles/](src/testfiles/)
- Compiler diagnostics and error recovery
- Runtime correctness and memory management

## License

MIT License
