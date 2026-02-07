# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

**Quark** is a high-level, dynamically-typed language that compiles to C++, designed for fast data-heavy applications. It combines Python-like syntax with native performance through aggressive compiler optimizations.

## Repository Structure

```
quark-lang/
├── src/
│   ├── core/quark/        # PRIMARY: Go compiler implementation
│   │   ├── main.go        # CLI entry point
│   │   ├── go.mod         # Go module definition
│   │   ├── token/         # Token types (44+ types)
│   │   ├── lexer/         # Lexer with indentation handling
│   │   ├── ast/           # AST node types, precedence levels
│   │   ├── parser/        # Pratt parser + recursive descent
│   │   ├── types/         # Type system, semantic analyzer
│   │   ├── codegen/       # C++ code generator with embedded runtime
│   │   └── runtime/       # C++ runtime library (header-only)
│   │       ├── include/quark/  # Modular C++ headers
│   │       │   ├── core/       # QValue, constructors, truthy
│   │       │   ├── ops/        # arithmetic, comparison, logical
│   │       │   ├── types/      # list, string, function
│   │       │   └── builtins/   # io, conversion, math
│   │       └── tests/          # Catch2 unit tests
│   ├── legacy/            # Python implementation (reference only)
│   └── testfiles/         # Test .qrk files for validation
├── CLAUDE.md              # This file - AI assistant guidance
├── stdlib.md              # Standard library documentation
```

> **IMPORTANT**: The Go implementation in `src/core/quark/` is the PRIMARY and ACTIVE implementation.
> All development, bug fixes, and new features should be made to the Go codebase.
> The Python implementation in `src/legacy/` is for reference only.

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
│  │  CODEGEN    │  → C++ source code                             │
│  │ codegen/    │     (Boxed values, std::vector lists)          │
│  └─────────────┘                                                │
│       ↓                                                         │
│  ┌─────────────┐                                                │
│  │  CLANG++    │  → Native binary                               │
│  │ -std=c++17  │     (SIMD auto-vectorization)                  │
│  │ -O3 -lm     │                                                │
│  └─────────────┘                                                │
└─────────────────────────────────────────────────────────────────┘
```

### Why This Architecture

| Component | Choice | Rationale |
|-----------|--------|-----------|
| Frontend | Go | Fast compilation, easy to modify, excellent tooling |
| Backend | C++ | Modern features (std::vector, RAII), leverages clang's optimizer |
| Target | C++17 | Modern standard with good STL support |
| Optimizer | clang++ -O3 | Aggressive optimizations, loop vectorization |
| CPU Target | -march=native | Uses AVX/SSE SIMD instructions for current CPU |
| Memory | Boxed values | Dynamic typing with tagged union (QValue) |
| Lists | std::vector | Type-safe, automatic memory management |

### C++ Compilation Flags

```bash
clang++ -std=c++17 -O3 -march=native -o output input.cpp -lm
```

| Flag | Purpose |
|------|---------|
| `-std=c++17` | C++17 standard for modern features |
| `-O3` | Maximum optimization (includes auto-vectorization) |
| `-march=native` | Optimize for current CPU architecture |
| `-lm` | Link math library (sqrt, floor, ceil, etc.) |

## CLI Commands

```bash
cd src/core/quark
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

# Testing
./quark run ../../../src/testfiles/test_clean.qrk
```

## Core Language Design

**High-Level + Native Performance:** Quark provides Python-like ergonomics with C++ performance:
```quark
// Clean, readable syntax
print msg
data | transform | filter | save

// Compiles to optimized C++ with -O3
// Uses std::vector for lists, SIMD auto-vectorization
```

**Key Features:**
- Dynamic typing with type inference
- First-class functions and closures
- Pattern matching (`when` expressions)
- Pipe operator for data flow
- Lists backed by `std::vector<QValue>`

**Syntax Conventions:**
- Function calls: `print x` (parentheses optional)
- Nested calls require parens: `outer (inner x)`
- Arithmetic binds tighter: `fact n - 1` parses as `fact(n-1)`

## Type System

### Basic Types

| Type | Quark | C++ Representation |
|------|-------|-------------------|
| Integer | `42` | `long long` |
| Float | `3.14` | `double` |
| String | `'hello'` | `char*` (strdup'd) |
| Boolean | `true`, `false` | `bool` |
| Null | `null` | - |
| List | `[1, 2, 3]` | `std::vector<QValue>*` |
| Function | `fn x: x * 2` | function pointer |

### Runtime Value (QValue)

All Quark values are boxed in a tagged union:

```cpp
using QList = std::vector<QValue>;

struct QValue {
    enum ValueType {
        VAL_INT, VAL_FLOAT, VAL_STRING, VAL_BOOL, VAL_NULL, VAL_LIST, VAL_FUNC
    } type;

    union {
        long long int_val;
        double float_val;
        char* string_val;
        bool bool_val;
        QList* list_val;    // std::vector<QValue>*
        void* func_val;
    } data;
};
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

## Module System

Quark supports a simple module system for organizing code.

### Defining Modules

```quark
module math:
    fn square n:
        n * n

    fn cube n:
        n * n * n
```

### Using Modules

```quark
use math

// All symbols from math are now available
square 5 | println    // 25
cube 3 | println      // 27
```

### Implementation Details

- **Parser**: `parseModule()` and `parseUse()` in `parser/parser.go`
- **AST**: `ModuleNode` and `UseNode` in `ast/ast.go`
- **Analyzer**: Module scopes and symbol import in `types/analyzer.go`
- **Codegen**: Functions are generated globally (no namespacing in C output)

The `use` statement imports all symbols from a module into the current scope. Modules are currently compile-time only - they provide code organization but don't create separate compilation units.

## Standard Library

See [STDLIB.md](STDLIB.md) for complete documentation.

Built-in functions are implemented in the C runtime (in `codegen/codegen.go`) for performance:

### Core Functions
- `print`, `println` - Output
- `len` - Length of strings/lists
- `input` - Read line from stdin
- `str`, `int`, `float`, `bool` - Type conversion

### Math Functions
- `abs`, `min`, `max` - Basic math
- `sqrt`, `floor`, `ceil`, `round` - Advanced math

### String Functions
- `upper`, `lower`, `trim` - Case and whitespace
- `contains`, `startswith`, `endswith` - Searching
- `replace`, `concat` - Manipulation

## Semantic Analyzer

### Symbol Tables

Nested scopes track variables and functions:
```go
Scope {
    Parent  *Scope           // Lexical parent
    Symbols map[string]*Symbol
}

Module {
    Name    string
    Scope   *Scope
    Symbols map[string]*Symbol
}
```

### Analysis Phases

1. **Build symbol tables** - Track variables/functions per scope
2. **Infer types** - Determine types from literals and operations
3. **Check semantics** - Validate function calls, undefined variables
4. **Resolve modules** - Process module definitions and imports

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
| `sqrt 16` | `q_sqrt(qv_int(16))` |
| `upper 'hi'` | `q_upper(qv_string("hi"))` |

### Runtime Functions

```cpp
// Constructors
QValue qv_int(long long v);
QValue qv_float(double v);
QValue qv_string(const char* v);
QValue qv_bool(bool v);
QValue qv_null();
QValue qv_list(int initial_cap = 0);
QValue qv_list_init(std::initializer_list<QValue> items);

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

// List operations (backed by std::vector)
QValue q_push(QValue list, QValue item);
QValue q_pop(QValue list);
QValue q_get(QValue list, QValue index);
QValue q_set(QValue list, QValue index, QValue value);
QValue q_insert(QValue list, QValue index, QValue item);
QValue q_remove(QValue list, QValue index);
QValue q_slice(QValue list, QValue start, QValue end);
QValue q_reverse(QValue list);
QValue q_list_concat(QValue a, QValue b);

// I/O
QValue q_print(QValue v);
QValue q_println(QValue v);
QValue q_input();

// Math (requires -lm)
QValue q_abs(QValue v);
QValue q_min(QValue a, QValue b);
QValue q_max(QValue a, QValue b);
QValue q_sqrt(QValue v);
QValue q_floor(QValue v);
QValue q_ceil(QValue v);
QValue q_round(QValue v);

// String
QValue q_upper(QValue v);
QValue q_lower(QValue v);
QValue q_trim(QValue v);
QValue q_contains(QValue str, QValue sub);
QValue q_startswith(QValue str, QValue prefix);
QValue q_endswith(QValue str, QValue suffix);
QValue q_replace(QValue str, QValue old, QValue new_str);
QValue q_concat(QValue a, QValue b);
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

// Modules
module mymodule:
    fn helper x:
        x * 2

use mymodule
helper 5 | println
```

### Not Yet Implemented

- `class` definitions
- Array slicing `arr[0:5]`
- Lambda shorthand `map x: x * 2`
- Type annotations `int.x = 5`
- Boehm GC integration
- Multi-file module imports

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

### Using Standard Library

```quark
// Math operations
sqrt 16 | println           // 4
abs (0 - 5) | println       // 5
max 10, 20 | println        // 20

// String operations
upper 'hello' | println     // HELLO
'  hi  ' | trim | println   // hi
replace 'foo', 'o', 'a' | println  // faa
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
cd src/core/quark
go build -o quark .
```

### Testing

```bash
# Run from src/core/quark directory
./quark run ../../../src/testfiles/test_clean.qrk
./quark run ../../../src/testfiles/test_math.qrk
./quark run ../../../src/testfiles/test_string.qrk
./quark run ../../../src/testfiles/test_module.qrk

# Debug mode (saves .c file)
./quark run test.qrk --debug

# View generated C
./quark emit test.qrk
```

### Adding Features

1. **New token**: Add to `token/token.go`
2. **New operator**: Add precedence in `ast/ast.go`, handler in `parser/expr.go`
3. **New statement**: Add parser in `parser/parser.go`
4. **New built-in function**:
   - Add C++ implementation in `runtime/include/quark/` (appropriate header)
   - Regenerate `codegen/runtime.hpp` (run `build_runtime.ps1`)
   - Add case in `generateFunctionCall()` and `generatePipe()`
   - Register type signature in `types/analyzer.go` builtins map
5. **Code generation**: Add case in `codegen/codegen.go`

### Runtime Development

The C++ runtime is a header-only library in `runtime/include/quark/`:

```
runtime/
├── include/quark/
│   ├── quark.hpp           # Master include
│   ├── core/               # QValue, constructors, truthy
│   ├── ops/                # arithmetic, comparison, logical
│   ├── types/              # list (std::vector), string, function
│   └── builtins/           # io, conversion, math
├── tests/                  # Catch2 unit tests
└── CMakeLists.txt          # Build tests with: cmake -B build && cmake --build build
```

After modifying headers, regenerate the embedded runtime:
```bash
cd runtime && pwsh build_runtime.ps1
```

### Known Limitations

- **Negative number arguments**: `func -5` is parsed as `func - 5` (binary subtraction). Use `x = 0 - 5; func x` as workaround.
- **Garbage collection**: Currently no GC - strings are strdup'd and list memory must be manually freed with `q_list_free()`.
- **List syntax**: List literals `[1, 2, 3]` not yet parsed; use `qv_list()` and `q_push()` in generated code.
