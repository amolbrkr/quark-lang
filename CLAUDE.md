# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

**Quark** is a human-friendly, functional, type-inferred language inspired by Python. The language emphasizes a **minimal punctuation philosophy** - using as few parentheses, brackets, and braces as possible to create English-like readable code.

## Active Implementation

> **IMPORTANT**: The Go implementation in `src/core_new/` is the PRIMARY and ACTIVE implementation.
> All development, bug fixes, and new features should be made to the Go codebase.
> The Python implementation in `src/core/` is legacy/reference only.

## Compiler Architecture

### Pipeline Overview

```
┌─────────────────────────────────────────────────────────────────┐
│                        QUARK COMPILER                           │
├─────────────────────────────────────────────────────────────────┤
│  Source (.qrk)                                                  │
│       ↓                                                         │
│  ┌─────────────┐                                                │
│  │   LEXER     │  → Token stream with INDENT/DEDENT            │
│  │ lexer/      │     (Python-style indentation)                 │
│  └─────────────┘                                                │
│       ↓                                                         │
│  ┌─────────────┐                                                │
│  │   PARSER    │  → Abstract Syntax Tree                        │
│  │ parser/     │     (Pratt parser + recursive descent)         │
│  └─────────────┘                                                │
│       ↓                                                         │
│  ┌─────────────┐                                                │
│  │  ANALYZER   │  → Type-checked AST                            │
│  │ types/      │     (Type inference, symbol tables)            │
│  └─────────────┘                                                │
│       ↓                                                         │
│  ┌─────────────┐                                                │
│  │  CODEGEN    │  → C source code                               │
│  │ codegen/    │     (Boxed values, runtime functions)          │
│  └─────────────┘                                                │
│       ↓                                                         │
│  ┌─────────────┐                                                │
│  │   CLANG     │  → Native binary                               │
│  │ -std=gnu11  │     (SIMD auto-vectorization)                  │
│  │ -O3         │                                                │
│  └─────────────┘                                                │
└─────────────────────────────────────────────────────────────────┘
```

### Why This Architecture

| Component | Choice | Rationale |
|-----------|--------|-----------|
| Frontend | Go | Fast compilation, easy to modify, excellent tooling |
| Backend | C | Leverages clang's mature optimizer, portable |
| Target | GNU C11 | C11 standard + POSIX extensions (strdup, etc.) |
| Optimizer | clang -O3 | Aggressive optimizations, loop vectorization |
| CPU Target | -march=native | Uses AVX/SSE SIMD instructions for current CPU |
| Memory | Boxed values | Dynamic typing with tagged union (QValue) |

### C Compilation Flags

```bash
clang -std=gnu11 -O3 -march=native -o output input.c
```

| Flag | Purpose |
|------|---------|
| `-std=gnu11` | C11 standard with GNU/POSIX extensions |
| `-O3` | Maximum optimization (includes auto-vectorization) |
| `-march=native` | Optimize for current CPU architecture |

### Project Structure

```
src/core_new/
├── main.go              # CLI entry point
├── go.mod               # Go module definition
├── token/
│   └── token.go         # Token types (44+ types)
├── lexer/
│   └── lexer.go         # Lexer with indentation handling
├── ast/
│   └── ast.go           # AST node types, precedence levels
├── parser/
│   ├── parser.go        # Statement parser
│   └── expr.go          # Pratt expression parser
├── types/
│   ├── types.go         # Type system (int, float, string, bool, list, fn)
│   └── analyzer.go      # Semantic analyzer, symbol tables
└── codegen/
    └── codegen.go       # C code generator with runtime
```

## CLI Commands

```bash
cd src/core_new
go build -o quark .

# Commands
./quark lex <file>                 # Tokenize and print tokens
./quark parse <file>               # Parse and print AST
./quark check <file>               # Type check only
./quark emit <file>                # Emit C code to stdout
./quark build <file> [-o out]      # Compile to executable
./quark run <file> [--debug|-d]    # Compile and run

# Shortcuts
./quark file.qrk                   # Same as: quark run file.qrk
./quark run file.qrk --debug       # Saves file.c for inspection
```

## Core Language Philosophy

**Minimal Punctuation:** Quark reduces punctuation wherever possible:
```quark
// Function calls without parentheses
print msg
add 2, 5

// Pipe chains
data | transform | filter | print

// Arithmetic binds arguments
fact n - 1    // parses as fact(n-1)
```

**Parentheses ONLY required for:**
1. Overriding precedence: `2 * (3 + 4)`
2. Nested function calls: `outer (inner x, y)`
3. Complex expressions as arguments: `func (x + y), z`

## Type System

### Basic Types

| Type | Quark | C Representation |
|------|-------|------------------|
| Integer | `42` | `long long` |
| Float | `3.14` | `double` |
| String | `'hello'` | `char*` (strdup'd) |
| Boolean | `true`, `false` | `bool` |
| Null | `null` | - |
| List | `[1, 2, 3]` | (not fully implemented) |
| Function | `fn x: x * 2` | function pointer |

### Runtime Value (QValue)

All Quark values are boxed in a tagged union:

```c
typedef struct {
    enum { VAL_INT, VAL_FLOAT, VAL_STRING, VAL_BOOL, VAL_NULL } type;
    union {
        long long int_val;
        double float_val;
        char* string_val;
        bool bool_val;
    } data;
} QValue;
```

### Type Inference

Types are inferred from usage:
```quark
x = 10          // x: int
y = 3.14        // y: float
z = x + y       // z: float (int promoted)
```

## Operator Precedence

| Level | Operators | Associativity |
|-------|-----------|---------------|
| 0 | `=` (assignment) | Right |
| 1 | `\|` (pipe) | Left |
| 2 | `,` (comma) | Left |
| 3 | `if-else` (ternary) | Right |
| 4 | `or` | Left |
| 5 | `and` | Left |
| 6 | `&` (bitwise AND) | Left |
| 7 | `==` `!=` | Left |
| 8 | `<` `<=` `>` `>=` | Left |
| 9 | `..` (range) | Left |
| 10 | `+` `-` | Left |
| 11 | `*` `/` `%` | Left |
| 12 | `**` (power) | **Right** |
| 13 | `!` `~` `-` (unary) | Right |
| 14 | function application | Left |
| 15 | `.` `[]` `()` (access) | Left |

## Semantic Analyzer

### Symbol Tables

Nested scopes track variables and functions:
```go
Scope {
    Parent  *Scope           // Lexical parent
    Symbols map[string]*Symbol
}
```

### Analysis Phases

1. **Build symbol tables** - Track variables/functions per scope
2. **Infer types** - Determine types from literals and operations
3. **Check semantics** - Validate function calls, undefined variables

## C Code Generation

### Translation Examples

| Quark | Generated C |
|-------|-------------|
| `42` | `qv_int(42)` |
| `3.14` | `qv_float(3.14)` |
| `'hi'` | `qv_string("hi")` |
| `x + y` | `q_add(x, y)` |
| `x > y` | `q_gt(x, y)` |
| `print x` | `q_print(x)` |
| `x = 5` | `QValue x = qv_int(5);` |
| `fn foo n:` | `QValue q_foo(QValue n) {` |
| `x \| f` | `q_f(x)` |

### Runtime Functions

```c
// Constructors
QValue qv_int(long long v);
QValue qv_float(double v);
QValue qv_string(const char* v);
QValue qv_bool(bool v);
QValue qv_null();

// Arithmetic
QValue q_add(QValue a, QValue b);
QValue q_sub(QValue a, QValue b);
QValue q_mul(QValue a, QValue b);
QValue q_div(QValue a, QValue b);
QValue q_mod(QValue a, QValue b);
QValue q_pow(QValue a, QValue b);

// Comparison
QValue q_lt(QValue a, QValue b);
QValue q_lte(QValue a, QValue b);
QValue q_gt(QValue a, QValue b);
QValue q_gte(QValue a, QValue b);
QValue q_eq(QValue a, QValue b);
QValue q_neq(QValue a, QValue b);

// Logical
QValue q_and(QValue a, QValue b);
QValue q_or(QValue a, QValue b);
QValue q_not(QValue a);

// I/O
QValue q_print(QValue v);
QValue q_println(QValue v);
```

## Language Features

### Implemented

```quark
// Variables
x = 10
y = x + 5

// Functions
fn double n:
    n * 2

fn fact n:
    when n:
        0 or 1: 1
        _: n * fact n - 1

// Control flow
if x > 10:
    println 'big'
elseif x > 5:
    println 'medium'
else:
    println 'small'

// Pattern matching
when value:
    0 or 1: 'zero or one'
    _: 'other'

// Loops
for i in 0..10:
    println i

while x > 0:
    x = x - 1

// Ternary
result = a if condition else b

// Pipes
10 | double | println

// Operators
a + b, a - b, a * b, a / b, a % b, a ** b
a == b, a != b, a < b, a > b, a <= b, a >= b
a and b, a or b, !a
a..b
```

### Not Yet Implemented

- `class` definitions
- `use`/`module` imports
- Array slicing `arr[0:5]`
- Lambda shorthand `map x: x * 2`
- Type annotations `int.x = 5`
- Boehm GC integration

## Example Programs

### Factorial

```quark
fn fact n:
    when n:
        0 or 1: 1
        _: n * fact n - 1

fact 10 | println
```

### Fibonacci

```quark
fn fib n:
    n if n <= 1 else fib (n - 1) + fib (n - 2)

for i in 0..10:
    fib i | println
```

### FizzBuzz

```quark
for i in 1..101:
    if i % 15 == 0:
        println 'FizzBuzz'
    elseif i % 3 == 0:
        println 'Fizz'
    elseif i % 5 == 0:
        println 'Buzz'
    else:
        println i
```

## Conventions

- **Use `elseif` not `elif`** - More English-like
- **Single-quoted strings** - `'hello'` not `"hello"`
- **Space for function calls** - `print x` not `print(x)`
- **Pipe for chaining** - `x | f | g` not nested calls
- **Underscore for wildcard** - `_` in pattern matching
- **Python-style indentation** - Blocks defined by indentation

## Development

### Building

```bash
cd src/core_new
go build -o quark .
```

### Testing

```bash
# Test specific file
./quark run test.qrk

# Debug mode (saves .c file)
./quark run test.qrk --debug

# View generated C
./quark emit test.qrk
```

### Adding Features

1. **New token**: Add to `token/token.go`
2. **New operator**: Add precedence in `ast/ast.go`, handler in `parser/expr.go`
3. **New statement**: Add parser in `parser/parser.go`
4. **Code generation**: Add case in `codegen/codegen.go`
