# Quark

**Quark** is a human-friendly, functional, type-inferred language inspired by Python. It emphasizes **minimal punctuation** - using as few parentheses, brackets, and braces as possible to create English-like readable code.

## Features

- **Minimal Punctuation** - Function calls without parentheses, pipe chains for data flow
- **Type Inference** - Types are inferred from usage, no annotations required
- **Pattern Matching** - Expressive `when` expressions with wildcard support
- **First-Class Functions** - Functions are values, support pipes and composition
- **Python-Style Indentation** - Blocks defined by indentation, no braces needed
- **Native Compilation** - Compiles to C, then to optimized native binaries via clang

## Quick Example

```quark
// Function definition - no parentheses needed
fn factorial n:
    when n:
        0 or 1: 1
        _: n * factorial n - 1

// Pipe chains for readable data flow
factorial 10 | println

// Pattern matching with multiple conditions
fn fizzbuzz n:
    when n % 15:
        0: 'FizzBuzz'
        _: when n % 3:
            0: 'Fizz'
            _: when n % 5:
                0: 'Buzz'
                _: n

// Loops with ranges
for i in 1..21:
    fizzbuzz i | println
```

## Installation

```bash
# Clone the repository
git clone https://github.com/user/quark-lang.git
cd quark-lang

# Build the compiler (requires Go 1.19+)
cd src/core/quark
go build -o quark .

# Run a program
./quark run ../../../src/testfiles/test_clean.qrk
```

### Requirements

- **Go 1.19+** - For building the compiler
- **clang or gcc** - For compiling generated C code
- **Linux/macOS/WSL** - Primary development platforms

## Usage

```bash
# Run a Quark program
./quark run program.qrk

# Compile to executable
./quark build program.qrk -o myapp

# View generated C code
./quark emit program.qrk

# Debug mode (keeps .c file)
./quark run program.qrk --debug
```

## Language Syntax

### Variables and Functions

```quark
// Variables - type inferred
x = 42
name = 'Quark'
pi = 3.14159

// Functions
fn greet name:
    println 'Hello, ' + name

fn add a, b:
    a + b
```

### Control Flow

```quark
// If-elseif-else
if x > 10:
    println 'big'
elseif x > 5:
    println 'medium'
else:
    println 'small'

// Pattern matching
when value:
    0 or 1: 'zero or one'
    2: 'two'
    _: 'other'

// Ternary
result = 'yes' if condition else 'no'
```

### Loops

```quark
// For loop with range
for i in 0..10:
    println i

// While loop
while x > 0:
    x = x - 1
```

### Pipes

```quark
// Chain operations naturally
'hello world' | upper | println

// Works with any function
data | transform | filter | save
```

## Standard Library

Quark includes built-in functions for common operations:

| Category | Functions |
|----------|-----------|
| **I/O** | `print`, `println`, `input` |
| **Types** | `str`, `int`, `float`, `bool`, `len` |
| **Math** | `abs`, `min`, `max`, `sqrt`, `floor`, `ceil`, `round` |
| **String** | `upper`, `lower`, `trim`, `contains`, `startswith`, `endswith`, `replace`, `concat` |

See [STDLIB.md](STDLIB.md) for complete documentation.

## Project Structure

```
quark-lang/
├── src/
│   ├── core/quark/     # Go compiler (primary implementation)
│   ├── legacy/         # Python reference implementation
│   └── testfiles/      # Test programs
├── CLAUDE.md           # Development guide
├── STDLIB.md           # Standard library docs
└── README.md           # This file
```

## Architecture

Quark uses a multi-stage compilation pipeline:

```
Source (.qrk) → Lexer → Parser → Analyzer → C Codegen → clang → Binary
```

- **Frontend**: Written in Go for fast compilation and easy modification
- **Backend**: Generates C code, leveraging clang's mature optimizer
- **Runtime**: Boxed values with tagged union for dynamic typing

## Status

Quark is in active development. Current status:

- [x] Lexer with Python-style indentation
- [x] Pratt parser for expressions
- [x] Type inference and semantic analysis
- [x] C code generation
- [x] Functions, loops, conditionals
- [x] Pattern matching (`when`)
- [x] Pipe operator
- [x] Module system (`module`/`use`)
- [x] Standard library (math, string)
- [ ] Lists and arrays
- [ ] Classes/structs
- [ ] Multi-file modules
- [ ] Garbage collection

## Contributing

Contributions welcome! Areas that need help:

- **Testing** - Write test programs in `src/testfiles/`
- **Documentation** - Improve docs and examples
- **Standard Library** - Add useful built-in functions
- **Error Messages** - Better compiler diagnostics

## License

MIT License - See LICENSE file for details.
