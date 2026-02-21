# Quark

**Quark** is a high-level, dynamically-typed language that compiles to C++17. It aims for Python-like syntax with native performance through a small Go frontend and a lightweight C++ runtime.

## What Works Today

- Indentation-based blocks and a Pratt parser for expressions
- Functions, lambdas, and closures (capture-by-value)
- If/elseif/else, `when` pattern matching, `for`/`while` loops, and ternary expressions
- Pipe operator for data-flow style calls
- Unified invocation model: `callable(entity, ...)` for all builtins and user functions
- Lists backed by `std::vector<QValue>` with indexing
- Typed 1D vectors (`f64`, `i64`, `bool`, `str`, `cat`) with invariant checks and null-mask scaffolding
- Vector arithmetic and reductions (`+`, `-`, `*`, `/`, `sum`, `min`, `max`) for numeric paths
- Vector helpers `astype`, `fillna`, `to_vector`, `cat_from_str`, and `cat_to_str`
- Dicts backed by `std::unordered_map<std::string, QValue>` with dot access (data only, no method dispatch)
- Builtins for I/O, math, strings, lists, and dict helpers
- Modules and `use` as compile-time organization (single file)

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
- **Boehm GC** headers + library (built from `deps/bdwgc`, see below)
- **Windows, Linux, or macOS**

### Boehm GC (required for `quark run/build`)

Quark defaults to compiling generated C++ with `-DQUARK_USE_GC` and links against `libgc`.

From the repo root:

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

### Vectors (MVP)

```quark
v = vector [1, 2, 3, 4]
w = v + 1
u = v * w

// Literal inference
vi = vector [1, 2, 3]          // vector[i64]
vs = vector ['a', 'b', 'c']    // vector[str]

iv = astype(v, 'i64')
iv = iv + 2
filled = fillna(iv, 0)

v2 = to_vector(list [10, 20, 30])
println(type(v2))

labels = list ['red', 'blue', 'red']
cats = cat_from_str(labels)
decoded = cat_to_str(cats)

println(sum(v))
println(min(v))
println(max(v))
```

Vector literal and `to_vector(...)` rules are homogeneous:

- `vector [1, 2, 3]` -> `vector[i64]`
- `vector [1.0, 2.0]` -> `vector[f64]`
- `vector ['a', 'b']` -> `vector[str]`
- Mixed literals (for example `vector [1, '2', 3]`) are a type error
- `to_vector(list [...])` follows the same homogeneous rule and rejects mixed element types
- Vector arithmetic `+ - * /` is numeric-only (`vector[str]` arithmetic is rejected)

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

## Standard Library (Builtins)

| Category | Functions |
| --- | --- |
| **I/O** | `print`, `println`, `input` |
| **Types** | `str`, `int`, `float`, `bool`, `type`, `len` |
| **Math** | `abs`, `min`, `max`, `sum`, `sqrt`, `floor`, `ceil`, `round` |
| **String** | `upper`, `lower`, `trim`, `contains`, `startswith`, `endswith`, `replace`, `concat`, `split` |
| **List** | `push`, `pop`, `get`, `set`, `insert`, `remove`, `slice`, `reverse`, `range` |
| **Dict** | `dget`, `dset` |
| **Vector** | `fillna`, `astype`, `to_vector`, `cat_from_str`, `cat_to_str` |

See [stdlib.md](stdlib.md) for details.

## Architecture

```
Source (.qrk) -> Lexer -> Parser -> Analyzer -> C++ Codegen -> clang++/g++ -> Binary
```

- **Frontend**: Go
- **Backend**: C++17 codegen
- **Runtime**: Header-only C++ runtime included via `#include "quark/quark.hpp"` in generated output

## Status and Gaps

Implemented:

- Lexer with indentation and a Pratt parser
- Analyzer with basic type inference
- Codegen for functions, control flow, pipes, lists, dicts, and builtins
- Unified invocation model: `callable(entity, ...)` — no dot-call/method dispatch
- Closures with capture-by-value semantics
- Modules (`module`/`use`) as a single-file organization tool
- Boehm GC integration (via `deps/bdwgc`)
- Typed vector runtime foundation (`f64`, `i64`, `bool`, `str`, `cat`) with validation helpers
- Numeric vector arithmetic/reductions with i64 paths and scalar broadcasting
- Vector builtins `fillna`, `astype`, `cat_from_str`, and `cat_to_str`
- Homogeneous vector literal inference (`i64`, `f64`, `str`) and aligned `to_vector(...)` type checks
- Portable amd64 compile baseline `-march=x86-64-v3` (replacing `-march=native`)
- Clang loop-vectorization diagnostics enabled during Quark compilation
- `xsimd` dependency removed from runtime and build flow

Not yet implemented or incomplete:

- Slicing (`[start:end[:step]]`)
- String interpolation
- Structs and impl blocks
- Result/try/unwrap-style helpers (beyond `ok`/`err` + `when` patterns)
- Multi-file modules
- Vector utilities from spec: `where`, `unique`, `value_counts`
- Full null-propagation semantics across all vector kernels
- `mean` vector reduction

## Tests

Quark has two test layers:

- **Go tests** for the compiler frontend (lexer/parser/analyzer/codegen) plus an **end-to-end smoke** test that compiles and runs Quark programs.
- **C++ Catch2 tests** for the runtime library.

### Prerequisite: build Boehm GC

The end-to-end tests compile generated C++ with `-DQUARK_USE_GC` and link `libgc`, so build the dependency once:

```bash
cd deps/bdwgc
cmake -S . -B build
cmake --build build
```

### End-to-end smoke (Go)

Runs the full pipeline (Quark → generated C++ → native exe) against the 4 smoke programs in `src/testfiles/`:

```bash
cd src/core/quark
go test -run TestSmokePrograms_Run -v
```

### Go unit tests (compiler)

Runs lexer/parser/analyzer/codegen unit tests (and also the end-to-end smoke test):

```bash
cd src/core/quark
go test ./...
```

If you want to run only the unit tests (no end-to-end compile/run), run packages directly:

```bash
cd src/core/quark
go test ./lexer ./parser ./types ./codegen
```

### Runtime unit tests (Catch2)

```bash
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
