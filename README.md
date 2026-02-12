# Quark

**Quark** is a high-level, dynamically-typed language that compiles to C++17. It aims for Python-like syntax with native performance through a small Go frontend and a lightweight C++ runtime.

## What Works Today

- Indentation-based blocks and a Pratt parser for expressions
- Functions and lambdas
- If/elseif/else, `when` pattern matching, `for`/`while` loops, and ternary expressions
- Pipe operator for data-flow style calls
- Lists backed by `std::vector<QValue>` with indexing
- 1D float vectors with elementwise arithmetic and reductions (`sum`, `min`, `max`)
- Dicts backed by `std::unordered_map<std::string, QValue>` with dot access
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
    println 'big'
elseif x > 5:
    println 'medium'
else:
    println 'small'

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
nums | push(4)
```

### Vectors (MVP)

```quark
v = vector [1, 2, 3, 4]
w = v + 1
u = v * w

println(sum(v))
println(min(v))
println(max(v))
```

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
| **Types** | `str`, `int`, `float`, `bool`, `len` |
| **Math** | `abs`, `min`, `max`, `sum`, `sqrt`, `floor`, `ceil`, `round` |
| **String** | `upper`, `lower`, `trim`, `contains`, `startswith`, `endswith`, `replace`, `concat` |
| **String** | `upper`, `lower`, `trim`, `contains`, `startswith`, `endswith`, `replace`, `concat`, `split` |
| **List** | `push`, `pop`, `get`, `set`, `insert`, `remove`, `slice`, `reverse`, `range` |
| **Dict** | `dget`, `dset` |

See [stdlib.md](stdlib.md) for details.

## Architecture

```
Source (.qrk) -> Lexer -> Parser -> Analyzer -> C++ Codegen -> clang++/g++ -> Binary
```

- **Frontend**: Go
- **Backend**: C++17 codegen
- **Runtime**: Header-only C++ runtime embedded into generated output

## Status and Gaps

Implemented:

- Lexer with indentation and a Pratt parser
- Analyzer with basic type inference
- Codegen for functions, control flow, pipes, lists, dicts, and builtins
- Modules (`module`/`use`) as a single-file organization tool
- Boehm GC integration (via `deps/bdwgc`)

Not yet implemented or incomplete:

- Slicing (`[start:end[:step]]`)
- String interpolation
- Structs and impl blocks
- Result/try/unwrap-style helpers (beyond `ok`/`err` + `when` patterns)
- Tensor types
- Multi-file modules

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
