# Quark

**Quark** is a high-level, dynamically-typed language that compiles to C++17. It aims for Python-like syntax with native performance through a small Go frontend and a lightweight C++ runtime.

## What Works Today

- Indentation-based blocks and a Pratt parser for expressions
- Functions and lambdas (no captures yet)
- If/elseif/else, `when` pattern matching, `for`/`while` loops, and ternary expressions
- Pipe operator for data-flow style calls
- Lists backed by `std::vector<QValue>` with indexing
- Builtins for I/O, math, strings, and list operations
- Modules and `use` as compile-time organization (single file)

## Quick Example

```quark
fn factorial n ->
    when n:
        0 or 1 -> 1
        _ -> n * factorial (n - 1)

factorial 10 | println

fn fizzbuzz n ->
    when n % 15:
        0 -> 'FizzBuzz'
        _ -> when n % 3:
            0 -> 'Fizz'
            _ -> when n % 5:
                0 -> 'Buzz'
                _ -> n

for i in range 1, 21:
    fizzbuzz i | println
```

## Installation

```bash
git clone https://github.com/user/quark-lang.git
cd quark-lang

cd src/core/quark
go build -o quark .

./quark run ../../../src/testfiles/test_clean.qrk
```

### Requirements

- **Go 1.19+**
- **clang++ or g++** in PATH (for C++17 codegen)
- **Windows, Linux, or macOS**

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

fn greet name ->
    println 'Hello, ' + name
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
for i in range 0, 10:
    println i

while x > 0:
    x = x - 1

'hello world' | upper | println
```

### Lists

```quark
nums = [1, 2, 3]
first = nums[0]
nums | push 4
```

## Standard Library (Builtins)

| Category | Functions |
| --- | --- |
| **I/O** | `print`, `println`, `input` |
| **Types** | `str`, `int`, `float`, `bool`, `len` |
| **Math** | `abs`, `min`, `max`, `sqrt`, `floor`, `ceil`, `round` |
| **String** | `upper`, `lower`, `trim`, `contains`, `startswith`, `endswith`, `replace`, `concat` |
| **List** | `push`, `pop`, `get`, `set` |

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
- Codegen for functions, control flow, pipes, lists, and builtins
- Modules (`module`/`use`) as a single-file organization tool

Not yet implemented or incomplete:

- Dict runtime/codegen (dict literals parse but do not compile)
- Slicing (`[start:end[:step]]`)
- String interpolation
- Structs and impl blocks
- Result/try/unwrap error handling
- Tensor types
- Multi-file modules
- Memory management / GC

## Contributing

Contributions welcome. Priorities:

- Tests in [src/testfiles/](src/testfiles/)
- Compiler diagnostics and error recovery
- Runtime correctness and memory management

## License

MIT License
