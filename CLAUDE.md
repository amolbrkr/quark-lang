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
- Structs with impl blocks (not classes)
- Error handling with `ok`/`err`/`try`
- Lists backed by `std::vector<QValue>`

**Syntax Conventions:**
- Function calls: `print x` (parentheses optional)
- Nested calls require parens: `outer (inner x)`
- Arithmetic binds tighter: `fact n - 1` parses as `fact(n-1)`

### Symbol Meanings

Quark uses punctuation intentionally to convey semantic meaning:

| Symbol | Meaning | Used For |
|--------|---------|----------|
| `->` | "produces" / "maps to" | Function bodies, pattern results |
| `:` | "has type" / "contains" | Type annotations, block containers, dict entries |
| `\|` | "then" / "pipe to" | Data flow pipelines |
| `?` | "might be null" | Optional types (future) |
| `??` | "or else" | Null coalescing (future) |

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
| Function | `fn x -> x * 2` | function pointer |
| Result | `ok value` or `err msg` | tagged union (planned) |
| Struct | `Point { x: 1, y: 2 }` | struct instance (planned) |

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

| Precedence | Operators | Associativity | Rule |
|------------|-----------|---------------|------|
| 16 | `.` `?.` `[]` `?[]` | Left | Access |
| 15 | (space) | Left | Application |
| 14 | `**` | Right | Exponent |
| 13 | `!` `-` (unary) | Right | Unary |
| 12 | `*` `/` `%` | Left | Multiplicative |
| 11 | `+` `-` | Left | Additive |
| 10 | `..` | None | Range |
| 9 | `<` `<=` `>` `>=` | Left | Comparison |
| 8 | `==` `!=` | Left | Equality |
| 7 | `and` | Left | LogicalAnd |
| 6 | `or` | Left | LogicalOr |
| 5 | `if-else` | Right | Ternary |
| 4 | `??` | Left | NullCoalesce (future) |
| 3 | `\|` | Left | Pipe |
| 2 | `->` | Right | Arrow |
| 1 | `=` | Right | Assignment |

**Note:** Bitwise operations (`&`, `|`, `^`, `~`, `<<`, `>>`) are NOT supported. Quark is a high-level language focused on readability over low-level bit manipulation.

## Module System

Quark supports a simple module system for organizing code.

### Defining Modules

```quark
module math:
    fn square n -> n * n

    fn cube n -> n * n * n
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

## Structs and Impl Blocks

**Status:** Planned (per grammar.md)

Quark uses structs for data structures and impl blocks for methods, avoiding the complexity of class-based OOP.

### Struct Definitions

Structs define typed fields with optional default values:

```quark
struct Point:
    x: float
    y: float

struct Config:
    name: string
    port: int = 8080
    debug: bool = false

struct Person:
    name: string
    age: int
    email: string?    // Optional field (future)
```

### Struct Literals

Create instances using brace syntax:

```quark
p = Point { x: 1.0, y: 2.0 }

config = Config {
    name: 'myapp',
    port: 3000,
    debug: true
}
```

### Impl Blocks

Attach methods to structs using impl blocks. Methods receive `self` as first parameter:

```quark
impl Point:
    fn distance self, other -> float
        dx = self.x - other.x
        dy = self.y - other.y
        sqrt (dx*dx + dy*dy)

    fn scale self, factor -> Point
        Point { x: self.x * factor, y: self.y * factor }

    fn origin -> Point      // Static method (no self)
        Point { x: 0.0, y: 0.0 }
```

### Method Calls

Use dot notation to call methods:

```quark
p1 = Point { x: 3.0, y: 4.0 }
p2 = Point { x: 0.0, y: 0.0 }

dist = p1.distance p2     // Instance method
scaled = p1.scale 2.0

origin = Point.origin     // Static method
```

## Error Handling

**Status:** Planned (per grammar.md)

Quark uses explicit Result types for error handling, similar to Rust but with simpler syntax.

### Result Values

Functions that can fail return either `ok value` or `err message`:

```quark
fn divide a, b -> Result
    if b == 0:
        err 'Division by zero'
    else:
        ok a / b

fn parse_int text -> Result[int]
    // ... parsing logic
    if valid:
        ok number
    else:
        err 'Invalid number: {text}'
```

### Pattern Matching on Results

Use `when` with `ok` and `err` patterns:

```quark
result = divide 10, 2

when result:
    ok value -> println 'Result: {value}'
    err msg -> println 'Error: {msg}'
```

### Try Statement

Block-based error handling:

```quark
try:
    config = load_config 'app.json'
    data = fetch_data config.url
    process data
err e:
    println 'Operation failed: {e}'
    use_defaults
```

### Error Propagation

Functions can propagate errors upward:

```quark
fn full_pipeline path -> Result
    when load_file path:
        err e -> err e              // Propagate error
        ok content ->
            when parse content:
                err e -> err e      // Propagate error
                ok data -> ok (transform data)
```

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
| `fn foo n ->` | `QValue q_foo(QValue n) {` |
| `x \| f` | `q_f(x)` |
| `sqrt 16` | `q_sqrt(qv_int(16))` |
| `upper 'hi'` | `q_upper(qv_string("hi"))` |
| `ok value` | `qv_ok(value)` (planned) |
| `err msg` | `qv_err(msg)` (planned) |

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

// Functions (using -> arrow operator)
fn double n -> n * 2

fn fact n ->
    when n:
        0 or 1 -> 1
        _ -> n * fact (n - 1)

// Control flow
if x > 10:
    println 'big'
elseif x > 5:
    println 'medium'
else:
    println 'small'

// Pattern matching (using -> for results)
when value:
    0 or 1 -> 'zero or one'
    _ -> 'other'

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
    fn helper x -> x * 2

use mymodule
helper 5 | println
```

### Planned (Per grammar.md)

- **Structs and Impl Blocks** - Define data structures with methods
- **Error Handling** - `ok`, `err`, `try` statements
- **String Interpolation** - `'Hello, {name}!'`
- **Array Slicing** - `arr[0:5]`, `arr[::2]`, `arr[::-1]`
- **Type Annotations** - `fn add x: int, y: int -> int`

### Future Features

- **Null Safety** - `?`, `??`, `?.`, `?[]` operators
- **Boehm GC Integration**
- **Multi-file Module Imports**

## Example Programs

### Factorial

```quark
fn fact n ->
    when n:
        0 or 1 -> 1
        _ -> n * fact (n - 1)

fact 10 | println
```

### Fibonacci

```quark
fn fib n -> n if n <= 1 else fib (n - 1) + fib (n - 2)

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

### Structs with Methods (Planned)

```quark
struct Rectangle:
    width: float
    height: float

impl Rectangle:
    fn area self -> self.width * self.height

    fn perimeter self -> 2 * (self.width + self.height)

    fn scale self, factor -> Result[Rectangle]
        if factor <= 0:
            err 'Scale factor must be positive'
        else:
            ok Rectangle {
                width: self.width * factor,
                height: self.height * factor
            }

rect = Rectangle { width: 10.0, height: 5.0 }
println (rect.area)

when rect.scale 2.0:
    ok scaled -> println 'New area: {scaled.area}'
    err msg -> println 'Error: {msg}'
```

### Error Handling (Planned)

```quark
fn load_csv path -> Result[list]
    if !file_exists path:
        err 'File not found: {path}'
    else:
        content = read_file path
        ok parse_csv content

fn process_data path ->
    when load_csv path:
        err e ->
            println 'Error: {e}'
            []
        ok data ->
            data
                | filter (fn row -> row['value'] > 0)
                | map (fn row -> row['value'] * 2)

// With try statement
try:
    data = load_csv 'data.csv'
    results = data | map transform | filter valid
    save_csv results, 'output.csv'
err e:
    println 'Pipeline failed: {e}'
```

## Conventions

- **Arrow `->` for mapping** - Functions and pattern results use `->` (reads as "produces")
  - `fn double x -> x * 2` (function produces result)
  - `ok value -> process value` (pattern produces result)
- **Colon `:` for typing/containing** - Type annotations and block containers use `:`
  - `x: int` (x has type int)
  - `if condition:` (if contains block)
  - `struct Point:` (struct contains fields)
- **Use `elseif` not `elif`** - More English-like
- **Single-quoted strings** - `'hello'` not `"hello"`
- **Space for function calls** - `print x` not `print(x)`
- **Pipe for chaining** - `x | f | g` not nested calls
- **Underscore for wildcard** - `_` in pattern matching
- **Python-style indentation** - Blocks defined by indentation
- **No bitwise operations** - High-level language focused on readability

## Grammar Updates Summary

**IMPORTANT:** Always reference [grammar.md](grammar.md) before implementing new features to ensure no drift between specification and implementation.

### Major Grammar Changes

1. **Arrow Operator `->` Introduced**
   - Replaces colon `:` in function definitions: `fn name params -> body`
   - Used in pattern matching results: `pattern -> result`
   - Semantically means "produces" or "maps to"

2. **Colon `:` Repurposed**
   - Now only for type annotations: `x: int`
   - Block containers: `if condition:`, `struct Name:`
   - Dict/struct entries: `key: value`
   - Semantically means "has type" or "contains"

3. **Structs + Impl (Not Classes)**
   - Structs define data structures with typed fields
   - Impl blocks attach methods to structs
   - Simpler than class-based OOP, easier to implement

4. **Error Handling**
   - New keywords: `ok`, `err`, `try`
   - Result types for explicit error handling
   - Pattern matching on results

5. **Null Safety (Future)**
   - `?` for optional types and safe navigation `?.`
   - `??` for null coalescing
   - `?[]` for safe indexing

6. **No Bitwise Operations**
   - Removed: `&`, `|`, `^`, `~`, `<<`, `>>`
   - Quark is high-level, not focused on bit manipulation

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

**Before implementing**, always check [grammar.md](grammar.md) for:
- Grammar rules and precedence
- Syntax examples
- Semantic rules
- Whether the feature fits Quark's design philosophy

**Implementation steps:**

1. **New token**: Add to `token/token.go`
   - Example: `ARROW` for `->`, `OK`/`ERR`/`TRY` keywords, `STRUCT`/`IMPL` keywords
2. **New operator**: Add precedence in `ast/ast.go`, handler in `parser/expr.go`
3. **New statement**: Add parser in `parser/parser.go`
   - Example: `parseStruct()`, `parseImpl()`, `parseTry()`
4. **New AST nodes**: Add to `ast/ast.go`
   - Example: `StructNode`, `ImplNode`, `TryNode`, `OkNode`, `ErrNode`
5. **New built-in function**:
   - Add C++ implementation in `runtime/include/quark/` (appropriate header)
   - Regenerate `codegen/runtime.hpp` (run `build_runtime.ps1`)
   - Add case in `generateFunctionCall()` and `generatePipe()`
   - Register type signature in `types/analyzer.go` builtins map
6. **Code generation**: Add case in `codegen/codegen.go`
7. **Type checking**: Update `types/analyzer.go` for new types/constructs

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
