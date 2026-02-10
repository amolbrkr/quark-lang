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
- Result values with `ok`/`err`
- Lists backed by `std::vector<QValue>`
- Dicts backed by `std::unordered_map<std::string, QValue>`

**Syntax Conventions:**
- Function calls: `print x` (parentheses optional)
- Nested calls require parens: `outer (inner x)`
- Arithmetic binds tighter: `fact n - 1` parses as `fact(n-1)`
- **Unary operators have no whitespace**: `-5` is negative five; `a - b` is subtraction; `a -b` is invalid
  - `f -5` → function call with negative argument
  - `a - b` → binary subtraction
  - Unary `-` and `!` must not have whitespace between operator and operand

### Symbol Meanings

Quark uses punctuation intentionally to convey semantic meaning:

| Symbol | Meaning | Used For |
|--------|---------|----------|
| `->` | "produces" / "maps to" | Function bodies, pattern results |
| `:` | "has type" / "contains" | Type annotations, block containers, dict entries |
| `\|` | "then" / "pipe to" | Data flow pipelines |

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
| Dict | `dict { a: 1 }` | `std::unordered_map<std::string, QValue>*` |
| Tensor | `tensor [1, 2, 3]` | contiguous buffer (future) |
| Function | `fn x -> x * 2` | function pointer |
| Result | `ok value` or `err msg` | tagged union (VAL_RESULT) |

### Runtime Value (QValue)

All Quark values are boxed in a tagged union:

```cpp
using QList = std::vector<QValue>;

struct QValue {
    enum ValueType {
        VAL_INT, VAL_FLOAT, VAL_STRING, VAL_BOOL, VAL_NULL, VAL_LIST, VAL_DICT,
        VAL_FUNC, VAL_RESULT
    } type;

    union {
        long long int_val;
        double float_val;
        char* string_val;
        bool bool_val;
        QList* list_val;    // std::vector<QValue>*
        QDict* dict_val;    // std::unordered_map<std::string, QValue>*
        void* func_val;
        QResult* result_val;
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
| 14 | `.` `[]` | Left | Access |
| 13 | (space) | Left | Application |
| 12 | `**` | Right | Exponent |
| 11 | `!` `-` (unary) | Right | Unary |
| 10 | `*` `/` `%` | Left | Multiplicative |
| 9 | `+` `-` | Left | Additive |
| 8 | `<` `<=` `>` `>=` | Left | Comparison |
| 7 | `==` `!=` | Left | Equality |
| 6 | `and` | Left | LogicalAnd |
| 5 | `or` | Left | LogicalOr |
| 4 | `if-else` | Right | Ternary |
| 3 | `\|` | Left | Pipe |
| 2 | `->` | Right | Arrow |
| 1 | `=` | Right | Assignment |

**Note:** Bitwise operations (`&`, `|`, `^`, `~`, `<<`, `>>`) are NOT supported. Quark is a high-level language focused on readability over low-level bit manipulation.

**Note:** Ranges use the `range()` builtin function, not a `..` operator.

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
    email: string
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

### Unwrap Keyword

The `unwrap` keyword extracts values from Results with a required default for the error case:

```quark
// unwrap result, default_value
value = unwrap divide 10, 2, 0          // Returns 5
value = unwrap divide 10, 0, -1         // Returns -1 (division failed)

user = unwrap find_user id, default_user
port = unwrap parse_int port_str, 8080

// Chained unwraps
name = unwrap (unwrap user.profile, default_profile).name, 'Anonymous'
```

The default argument is **always required** - this forces explicit handling of error cases. If you want to crash on error, use pattern matching and call a panic function explicitly.

## Standard Library

See [STDLIB.md](STDLIB.md) for complete documentation.

Built-in functions are implemented in the C runtime (in `codegen/codegen.go`) for performance:

### Core Functions
- `print`, `println` - Output
- `len` - Length of strings/lists
- `input` - Read line from stdin
- `str`, `int`, `float`, `bool` - Type conversion
- `range` - Generate integer ranges: `range 10`, `range 1, 100`, `range 0, 100, 5`

### Math Functions
- `abs`, `min`, `max` - Basic math
- `sqrt`, `floor`, `ceil`, `round` - Advanced math

### String Functions
- `upper`, `lower`, `trim` - Case and whitespace
- `contains`, `startswith`, `endswith` - Searching
- `replace` - Manipulation
- `concat` - Concatenation (works for both strings and lists)

### List Functions
- `push`, `pop` - Add/remove from end
- `get`, `set` - Access/modify by index
- `insert` - Insert at index: `insert list, 2, value`
- `remove` - Remove at index: `remove list, 0`
- `slice` - Slice [start, end): `slice list, 1, 3`
- `reverse` - Reverse in place

## Tensor Type

**Status:** Planned (per grammar.md) - Future feature for SIMD/GPU acceleration

Quark will support native N-dimensional `tensor` types optimized for numerical computing with SIMD instructions.

### Tensor Literals

```quark
// 1D tensor (vector)
vec = tensor [1.0, 2.0, 3.0, 4.0]

// 2D tensor (matrix) - semicolon separates rows
matrix = tensor [1, 2, 3; 4, 5, 6; 7, 8, 9]
eye = tensor [1, 0, 0; 0, 1, 0; 0, 0, 1]

// Higher dimensions via constructor functions
cube = zeros 10, 10, 10
```

### Tensor vs List

| Feature | List | Tensor |
|---------|------|--------|
| Element types | Mixed (any QValue) | Homogeneous (float64) |
| Memory layout | Pointer array | Contiguous buffer |
| Operations | General purpose | SIMD-accelerated |
| Use case | General data structures | Numerical computation |

```quark
// List - general purpose, mixed types
items = [1, 'hello', true, [1, 2, 3]]

// Tensor - numeric, SIMD operations
data = tensor [1.0, 2.0, 3.0, 4.0]
result = data * 2.0 + 1.0   // Vectorized operations
```

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
| `ok value` | `qv_ok(value)` |
| `err msg` | `qv_err(msg)` |
| `unwrap result, default` | `q_unwrap(result, default)` (planned) |
| `list [1, 2, 3]` | `qv_list_init({qv_int(1), qv_int(2), qv_int(3)})` |

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
QValue q_concat(QValue a, QValue b);  // Works for both strings and lists

// Range
QValue q_range(QValue end);                          // range(10)
QValue q_range(QValue start, QValue end);            // range(1, 100)
QValue q_range(QValue start, QValue end, QValue step); // range(0, 100, 5)

// Result types
QValue qv_ok(QValue value);
QValue qv_err(const char* message);

// Error handling (unwrap planned)
QValue q_unwrap(QValue result, QValue default_val);
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
for i in range 10:
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

// Modules
module mymodule:
    fn helper x -> x * 2

use mymodule
helper 5 | println

// Lists (require `list` keyword)
nums = list [1, 2, 3]
push nums, 4

// ok / err result values
fn safe_div a, b ->
    if b == 0:
        err 'division by zero'
    else:
        ok a / b

when safe_div 10, 2:
    ok value -> println value
    err msg -> println msg

// Typed parameters
fn add x: int, y: int -> x + y

// Typed variables
name: str = 'hello'
nums: list = list [1, 2, 3]

// Lambdas assigned to variables
double = fn x -> x * 2
```

### Future (Per grammar.md)

- **Structs and Impl Blocks** - Define data structures with methods
- **Tensor Type** - SIMD-accelerated N-dimensional arrays
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

for i in range 10:
    fib i | println
```

### FizzBuzz

```quark
for i in range 1, 101:
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

// Using unwrap with default
scaled = unwrap rect.scale 2.0, rect   // Returns scaled or original on error
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

// Using unwrap with defaults
data = unwrap load_csv 'data.csv', []
results = data | map transform | filter valid
```

## Conventions

- **Arrow `->` for mapping** - Functions and pattern results use `->` (reads as "produces")
  - `fn double x -> x * 2` (function produces result)
  - `ok value -> process value` (pattern produces result)
- **Colon `:` for typing/containing** - Type annotations and block containers use `:`
  - `x: int` (x has type int)
  - `if condition:` (if contains block)
  - `struct Point:` (struct contains fields)
- **Unary operators have no whitespace** - `-5` (negative), `!flag` (not), `a - b` (subtraction)
- **Use `elseif` not `elif`** - More English-like
- **Single-quoted strings** - `'hello'` not `"hello"`
- **Space for function calls** - `print x` not `print(x)`
- **Pipe for chaining** - `x | f | g` not nested calls
- **Underscore for wildcard** - `_` in pattern matching
- **Ranges use `range()` function** - `range 10`, `range 1, 100`, not `0..10`
- **Unwrap requires default** - `unwrap result, default` forces explicit error handling
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

4. **Error Handling with ok/err/unwrap**
   - New keywords: `ok`, `err`, `try`, `unwrap`
   - Result types for explicit error handling
   - Pattern matching on results
   - `unwrap` requires a default value (no implicit panicking)

5. **Range Function (Not Operator)**
   - Removed `..` operator
   - Use `range()` builtin: `range 10`, `range 1, 100`, `range 0, 100, 5`

6. **Tensor Type (Future)**
   - Native N-dimensional arrays for SIMD/GPU acceleration
   - Literal syntax: `tensor [1, 2, 3]` (1D), `tensor [1, 2; 3, 4]` (2D)
   - Homogeneous float64 data, contiguous memory layout

7. **Typed Parameters in One-Line Functions**
   - Single-line functions support type annotations: `fn add x: int, y: int -> x + y`

8. **Unary Operator Whitespace Rule**
   - Unary `-` and `!` must NOT have whitespace before operand
   - `-5` is negative five, `a - b` is subtraction, `a -b` is invalid
   - This resolves ambiguity between function calls and arithmetic

9. **No Null Safety Operators**
   - Removed `?`, `??`, `?.`, `?[]` operators
   - Error handling via explicit Result types instead

10. **No Bitwise Operations**
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

- **Unary operator whitespace**: Unary operators must have no whitespace. `f -5` (function call with negative argument) is valid, but `a -b` (space before, no space after) is a parse error. Use `a - b` for subtraction.
- **Garbage collection**: Boehm GC is enabled by default. GC_init() is called at program start. Requires Boehm GC installed on the system.
- **Dict access**: Dicts only support dot access (`d.key`), not bracket indexing (`d['key']`). Dict keys are always identifiers (no string literal keys).
- **Dict iteration**: No `for key in dict` support yet.

## Feature Matrix (Grammar vs Implementation)

Key: Yes = implemented, Partial = present but incomplete, No = missing.

| Feature | Lexer | Parser | Analyzer | Codegen | Runtime | Notes |
| --- | --- | --- | --- | --- | --- | --- |
| Indentation blocks | Yes | Yes | Yes | Yes | N/A | INDENT/DEDENT injection works for `:` and `->`. |
| Comments `//` | Yes | N/A | N/A | N/A | N/A | Stripped in lexer. |
| Literals (int/float/bool/null) | Yes | Yes | Yes | Yes | Yes | Basic literals are fully supported. |
| Strings (no interpolation) | Yes | Yes | Yes | Yes | Yes | Interpolation is not implemented. |
| Function definitions | Yes | Yes | Yes | Yes | Yes | Named functions generate `quark_<name>`. |
| Lambda expressions | Yes | Yes | Yes | Yes | Yes | No closures/captures. |
| Function calls / application | Yes | Yes | Yes | Yes | Yes | Analyzer validates arg counts for builtins and user functions. |
| If / elseif / else | Yes | Yes | Yes | Yes | Yes | Emits temp result in codegen. |
| Ternary `a if cond else b` | Yes | Yes | Yes | Yes | Yes | Expression form only. |
| When / pattern matching | Yes | Yes | Yes | Yes | Yes | Only equality and `_` wildcard. |
| For `for x in expr` | Yes | Yes | Yes | Yes | Yes | Loop variable typed from iterable; codegen uses list iteration. |
| While loops | Yes | Yes | Yes | Yes | Yes | Truthy check at runtime. |
| Pipe operator `|` | Yes | Yes | Yes | Yes | Yes | Builtin expansion in codegen. |
| Assignment `=` | Yes | Yes | Yes | Yes | N/A | Type tracking and assignment validation. |
| Arithmetic ops `+ - * / % **` | Yes | Yes | Yes | Yes | Yes | Enforces numeric/string operand checks. |
| Comparison ops `< <= > >= == !=` | Yes | Yes | Yes | Yes | Yes | Enforces comparable operand checks. |
| Logical ops `and` / `or` | Yes | Yes | Yes | Yes | Yes | Keyword `not` not implemented. |
| Unary ops `!` / `-` / `~` | Yes | Yes | Yes | Yes | Yes | `~` maps to logical not. |
| Member access `.` | Yes | Yes | Yes | Yes | Yes | Properties, no-arg methods, and method calls with args on lists/strings. |
| List literals `list [a, b]` | Yes | Yes | Yes | Yes | Yes | Uses `std::vector<QValue>`. Requires `list` keyword prefix. |
| Typed parameters `x: int` | Yes | Yes | Yes | Yes | N/A | Basic annotations on params and variable declarations. No generic types. |
| Indexing `list[idx]` | Yes | Yes | Yes | Yes | Yes | `q_get` supports negative indices. |
| Dict literals `dict {k: v}` | Yes | Yes | Yes | Yes | Yes | Requires `dict` keyword. Dot access only (`d.key`). |
| Dict dot access/assignment | Yes | Yes | Yes | Yes | Yes | `d.key` reads, `d.key = val` writes. No bracket indexing. |
| Modules `module` / `use` | Yes | Yes | Yes | Partial | N/A | Compile-time only, no namespacing. |
| Structs / impl blocks | No | No | No | No | No | Future. |
| Result / ok / err | Yes | Yes | Yes | Yes | Yes | `ok`/`err` values and `when` pattern matching on results. `try`/`unwrap` not yet implemented. |
| Tensor types | No | No | No | No | No | Grammar-only. |
| Builtins (io/math/string/list) | Yes | Yes | Yes | Yes | Yes | Implemented in runtime and codegen. |

## Stdlib Feature Matrix (Runtime + Compiler + Quark Modules)

Key: Yes = implemented, Partial = present but incomplete, No = missing.

| Area / API | C++ Runtime | Compiler Wired | Quark Module | Notes |
| --- | --- | --- | --- | --- |
| I/O: `print`, `println`, `input` | Yes | Yes | No | Builtins only. |
| Conversions: `len`, `str`, `int`, `float`, `bool` | Yes | Yes | No | Builtins only. |
| Math: `abs`, `min`, `max`, `sqrt`, `floor`, `ceil`, `round` | Yes | Yes | No | Builtins only. |
| String: `upper`, `lower`, `trim`, `contains`, `startswith`, `endswith`, `replace`, `concat` | Yes | Yes | No | Builtins only. |
| List: `push`, `pop`, `get`, `set`, `insert`, `remove`, `slice`, `reverse` | Yes | Yes | No | Builtins only. |
| List extras: `size`, `empty`, `clear` | Yes | No | No | Implemented in runtime but not exposed as builtins. |
| `concat` (overloaded) | Yes | Yes | No | Works for both strings and lists. |
| Dict: `dict {}`, dot access, `len`, `.size` | Yes | Yes | No | Dot access only. No bracket indexing. |
| Time / clock | No | No | No | Not yet implemented. |
| Random | No | No | No | Not yet implemented. |
| OS / filesystem | No | No | No | Not yet implemented. |
| Process / exec | No | No | No | Not yet implemented. |
| Env vars | No | No | No | Not yet implemented. |
| JSON / serialization | No | No | No | Not yet implemented. |
| Quark stdlib modules | No | No | No | Module loader needed for external stdlib files. |

## Recent Changes (2026-02-07)

### Refactored: Separate Runtime Headers Instead of Embedding

**Problem**: The entire 1033-line runtime.hpp was embedded into every generated C++ file, resulting in:
- Generated files over 1000 lines for even simple programs
- Bloated, unreadable output
- Slower compilation (runtime re-parsed every time)
- Difficult debugging

**Solution**: Refactored code generator to use external header includes instead of embedding.

**Changes Made**:

1. **codegen.go**: Added `embedRuntime` flag (default: false)
   - When false: generates `#include "quark/quark.hpp"`
   - When true: embeds full runtime (fallback for portability)

2. **main.go**: Updated compiler commands to pass runtime include path
   - Added `getRuntimeIncludePath()` function
   - Passes `-I{runtime_path}` to clang++/g++

3. **quark.hpp**: Fixed missing includes
   - Added `<algorithm>` and `<vector>` for std::reverse and std::vector

**Results**:

Before refactoring (embedded):
```cpp
// runtime.hpp - Quark Runtime Library
// [1033 lines of runtime code...]

QValue q_double(QValue x) { return q_mul(x, qv_int(2)); }
int main() { q_println(q_double(qv_int(5))); return 0; }
```
**Total**: 1050+ lines

After refactoring (separate headers):
```cpp
#include "quark/quark.hpp"

QValue q_double(QValue x) { return q_mul(x, qv_int(2)); }
int main() { q_println(q_double(qv_int(5))); return 0; }
```
**Total**: ~13 lines for simple programs, ~48 lines for complex programs

**Benefits**:
- ✅ 95%+ reduction in generated code size
- ✅ Clean, readable generated code
- ✅ Faster compilation (headers can be precompiled)
- ✅ Easier debugging
- ✅ Still supports embedded mode for single-file compilation

### Fixed: User Function Name Collisions with Runtime Builtins

**Problem**: User-defined functions used the same `q_` prefix as runtime builtins, causing name collisions. For example, a user function named `add` would generate `q_add()`, conflicting with the runtime's `q_add()` (the `+` operator).

**Why `q_` prefix exists**:
1. Avoid C++ keyword collisions (`class`, `new`, `delete`, etc.)
2. Namespace all Quark runtime functions consistently
3. Prevent conflicts with C++ standard library

**Solution**: Changed user-defined functions to use `quark_` prefix instead of `q_`, creating clear separation between user code and runtime builtins.

**Changes Made**:

Updated `codegen.go` to use `quark_` prefix for all user-defined functions:
- Function declarations: `QValue quark_add();`
- Function definitions: `QValue quark_add(QValue a, QValue b) { ... }`
- Function calls: `quark_add(x, y)`
- Lambda functions: `quark__lambda1()`

**Results**:

Before (name collision):
```cpp
// User function 'add'
QValue q_add(QValue a, QValue b) { return q_add(a, b); }  // ERROR: redefinition!

// Runtime builtin for '+'
inline QValue q_add(QValue a, QValue b) { ... }  // CONFLICT!
```

After (clear separation):
```cpp
// User function 'add' → quark_add
QValue quark_add(QValue a, QValue b) {
    return q_add(a, b);  // Calls runtime builtin for '+'
}

// Runtime builtin for '+' → q_add
inline QValue q_add(QValue a, QValue b) { ... }  // No conflict!
```

**Example**:
```quark
add = fn a, b -> a + b
result = add 3, 4      // Calls quark_add()
x = 10 + 20            // Calls q_add() operator
```

Generated C++:
```cpp
QValue quark_add(QValue a, QValue b) { return q_add(a, b); }
// quark_add = user function, q_add = runtime operator
```

**Benefits**:
- ✅ Users can now use any function name, including `add`, `mul`, `sub`, etc.
- ✅ Clear distinction between user code (`quark_*`) and runtime (`q_*`)
- ✅ No more reserved function names
- ✅ Generated code is more readable and self-documenting

## Recent Bug Fixes (2026-02-07)

### Fixed: OOM Errors Due to Infinite Parser Loops

**Problem**: After grammar overhaul to use `->` arrow syntax, several test files caused infinite loops that consumed all system memory, leading to OOM crashes.

**Root Cause**: Multiple parsing loops in `parser.go` failed to advance the token position when parsing returned `nil`, causing the parser to loop forever on the same token.

**Fixes Applied**:

1. **parser.go:70-81 (Parse function)**: Added token advancement when `parseStatement()` returns `nil`
2. **parser.go:122-134 (parseBlock indented)**: Added token advancement when `parseStatement()` returns `nil`
3. **parser.go:146-153 (parseBlock inline)**: Added token advancement when `parseStatement()` returns `nil`
4. **parser.go:332-344 (parseWhenStatement)**: Added token advancement when `parsePattern()` returns `nil`

```go
// Example fix pattern:
stmt := p.parseStatement()
if stmt != nil {
    node.AddChild(stmt)
} else {
    // Parsing failed - advance token to avoid infinite loop
    p.nextToken()
    continue
}
```

**Result**: Parser now gracefully exits with error messages instead of hanging and consuming all memory.

### Fixed: Lexer Not Recognizing Arrow for Indentation

**Problem**: After grammar change from `:` to `->` for function bodies, lexer produced "unexpected indent" errors for indented blocks after arrow.

**Root Cause**: Lexer only tracked `COLON` tokens as triggering indentation, not `ARROW` tokens.

**Fix**: Updated `lexer.go:111` to include both:
```go
case token.COLON, token.ARROW:
    indent = MAY_INDENT
```

**Result**: Indented blocks after `->` are now properly recognized.

### Fixed: Lambda Parameters Parsed as Comma Operators

**Problem**: Lambda definitions like `fn a, b -> a + b` generated C++ code with missing parameters: `QValue q_add(QValue ,)`.

**Root Cause**: The `parseArguments()` function called `parseExpression(ast.PrecLowest)`, which has lower precedence than comma. This caused `a, b` to be parsed as a single comma operator expression instead of two separate parameters.

**AST Before Fix**:
```
Arguments
  Operator[,]        # Wrong: comma treated as operator
    Identifier[a]
    Identifier[b]
```

**AST After Fix**:
```
Arguments
  Identifier[a]      # Correct: separate parameters
  Identifier[b]
```

**Fix**: Changed `parser.go:250` to parse at higher precedence:
```go
// Parse at PrecTernary to stop before comma (which has lower precedence)
// This ensures we get individual parameters, not comma expressions
expr := p.parseExpression(ast.PrecTernary)
```

**Result**: Lambda parameters are now correctly extracted and generate proper C++ function signatures.

### Testing

All core test files now work correctly:
- ✅ test_for.qrk
- ✅ test_arrow.qrk
- ✅ test_when_arrow.qrk
- ✅ test_clean.qrk
- ✅ test_lambda_working.qrk (lambdas with non-conflicting names)

**Known Issue**: test_functions.qrk still has name collisions with builtin functions (`add`, `double`). Use different names to avoid conflicts.

## Recent Changes (2026-02-09)

### Compiler Robustness: Union Types and MergeTypes

**Problem**: Type information was collapsing to `any` for branches, lists, and other multi-path constructs, preventing the analyzer from detecting real type errors.

**Solution**: Added `UnionType` and `MergeTypes()` to `types/types.go` (lines 107-306) so branches, lists, and other constructs retain precise type info instead of collapsing to `any`.

**New type utilities**:
- `UnionType` struct — represents a value that can be one of several concrete types
- `MergeTypes(...Type)` — combines multiple type possibilities into the most precise representation
- `IsComparable()`, `CanAssign()` — union-aware type checking helpers
- `isIntLike()`, `isStringLike()`, `isBoolLike()` — union-aware type predicates

### Reworked Semantic Analyzer

**Changes to `types/analyzer.go`**:

1. **Builtin signatures with arity** (lines 9-96): New `builtinSignature` struct stores `MinArgs`/`MaxArgs` alongside the `FunctionType`. Builtin definitions are now a single table kept in sync with `codegen/builtins.go`.

2. **Location-aware diagnostics** (line 106-112): New `errorAt(node, format, args...)` method attaches line/column info to error messages.

3. **Predeclared functions** (lines 122-158): `predeclareFunctions()` scans blocks and modules for function definitions before analyzing bodies, enabling recursion and forward references.

4. **Argument count validation** (lines 281-328): `analyzeFunctionCall()` checks arg counts for both builtins (min/max arity) and user functions. Non-callable expressions now produce clear error messages.

5. **Expression analysis** (lines 467-605): Operators enforce operand compatibility:
   - Arithmetic: requires numeric (or string for `+`)
   - Modulo: requires integer operands
   - Comparison: requires comparable operands
   - Logical: requires boolean operands
   - Warns on undefined identifiers

6. **Branch merging** (lines 330-373, 620-631): If/elseif/else and ternary branches use `MergeTypes` for precise result types.

7. **Scoped blocks and loops** (lines 375-426): For/while loops push fresh scopes; loop variables are typed from the iterable's element type.

### Codegen Block-Level Scoping

**Changes to `codegen/codegen.go`**:

- `pushBlockScope()` (line 77-85): Creates a child scope that inherits parent declarations. Variables declared inside blocks don't leak to outer scopes, preventing C++ redeclaration errors.
- `generateFor()` uses `pushBlockScope`/`popScope` to isolate loop variables.
- `generateBlock()` (line 235-249) pushes its own block scope.

### Runtime Callable Guard

**Changes to `runtime/include/quark/types/function.hpp`**:

- New `q_require_callable(QValue)` helper (line 9-15): All `q_call*` functions now gate on this check, producing a clear `"runtime error: attempted to call a non-function value"` instead of silently returning null or crashing.

### Fixed: For Loop Body Last Statement Not Emitted

**Problem**: `generateBlock()` returns the last expression without emitting it (designed for function return values). When called from `generateFor()`, the returned value was discarded, causing the last (or only) statement in a for loop body to be silently dropped.

**Example**: `for i in range 10: print i` would generate an empty loop body.

**Fix**: Changed `generateFor()` to iterate block children directly (like `generateWhile()`) instead of delegating to `generateBlock()`, ensuring all statements are emitted.

### Known Issue: test_while.qrk (RESOLVED)

`test_while.qrk` previously used `x` without initializing it. The test file has been fixed to initialize `x = 5` before the loop.

## Recent Changes (2026-02-09, Batch 2)

### GC Enabled by Default

- Boehm GC initialization (`GC_init()`) is now always emitted in the generated C++ `main()` function
- Removed `--gc` / `--no-gc` CLI flags; GC is always on
- Updated usage text in `main.go`

### Implemented ok / err Result Values

Added full support for `ok` and `err` result values with pattern matching:

- **Tokens**: Added `OK` and `ERR` keywords in `token/token.go`
- **AST**: Added `OkNode` and `ErrNode` node types in `ast/ast.go`
- **Parser**: `ok expr` and `err expr` parsed as prefix expressions; `when` patterns now match `ok ident ->` and `err ident ->` arms in `parser/parser.go`
- **Analyzer**: Result scoping — `ok`/`err` arm identifiers are scoped correctly in `types/analyzer.go`
- **Codegen**: `ok expr` generates `qv_ok(expr)`, `err expr` generates `qv_err(expr_as_string)`; `when` on results generates `if (cond.type == QValue::VAL_OK)` / `else` branches in `codegen/codegen.go`
- **Runtime**: Added `VAL_OK` and `VAL_ERR` to `QValue::ValueType`, `qv_ok()` and `qv_err()` constructors in `value.hpp` and `constructors.hpp`

Example:
```quark
fn load flag ->
    if flag:
        ok 'data loaded'
    else:
        err 'failed'

when load true:
    ok value -> println value
    err message -> println message
```

### Implemented Typed Parameters

- **Function parameters**: `fn add x: int, y: int -> x + y`
- **Variable declarations**: `name: str = 'hello'`, `nums: list = list [1, 2, 3]`
- Basic type annotations only (`int`, `float`, `str`, `bool`, `list`, `dict`). Generic type expressions (`list[int]`) have been removed.
- Files changed: `parser/parser.go`, `ast/ast.go`, `types/analyzer.go`, `codegen/codegen.go`

### Standardized String Type to `str`

- The type name is now `str` everywhere (removed `string` alias in the analyzer)
- Updated grammar and examples in `grammar.md`

### List Literal Disambiguation

- **Old syntax**: `[1, 2, 3]` (ambiguous with indexing)
- **New syntax**: `list [1, 2, 3]` (requires `list` keyword prefix)
- Added `LIST` token to `token/token.go`
- Updated parser in `expr.go` to require `list` keyword before bracket literals
- Updated all test files: `test_lists.qrk`, `test_list_extras.qrk`, `test_member.qrk`, `test_method_call.qrk`, `test_typed_params.qrk`
- Updated `grammar.md`

### Test Results (All Passing)

- test_hello.qrk
- test_math.qrk
- test_string.qrk
- test_module.qrk
- test_arrow.qrk
- test_when_arrow.qrk
- test_clean.qrk
- test_features.qrk
- test_for.qrk
- test_lambda_debug.qrk
- test_lambda_working.qrk
- test_functions.qrk
- test_add_collision.qrk
- test_full.qrk
- test_while.qrk
- test_result_when.qrk
- test_lists.qrk
- test_list_extras.qrk
- test_member.qrk
- test_method_call.qrk
- test_typed_params.qrk

## Recent Changes (2026-02-09, Grammar Trimming)

### Removed Generic Type Expressions

Generic type annotations like `list[int]`, `dict[str, int]` have been removed. Only basic type annotations are supported: `x: int`, `name: str`, `nums: list`, `data: dict`.

**Changes**:
- `parser/parser.go` `parseTypeExpr()`: Removed bracket parsing for generic params
- `types/analyzer.go` `resolveTypeNode()`: `list` always resolves to `ListType{ElementType: TypeAny}`, `dict` to `DictType{KeyType: TypeAny, ValueType: TypeAny}`
- `types/types.go` `CanAssign()`: Added list/dict covariance so `list[any]` accepts `list[str]` etc.
- Removed `checkListLiteralTypes()` from analyzer

### Removed Form 2 Lambda (id = fn special case)

The parser previously special-cased `id = fn params -> body` as a `FunctionNode` (Form 2). This was redundant with assigning a lambda expression to a variable. Now `double = fn x -> x * 2` parses as a normal assignment of a lambda expression.

**Changes**:
- `parser/parser.go`: Removed Form 2 lookahead and special-case branch in `parseFunction()`

### Unified `quark_` Prefix for All User Names

After removing Form 2, variables like `double` generated invalid C++ (`QValue double = ...`). Fixed by prefixing ALL user variable names with `quark_` (same prefix already used for user-defined functions). This eliminates the need for a C++ reserved keyword map — all user identifiers are uniformly namespaced in generated code (e.g., `x` → `quark_x`, `double` → `quark_double`).

### Fixed Lambda Forward Declarations

Lambda forward declarations were emitted with no parameters (`QValue quark__lambda1();`), causing C++ overload ambiguity. Changed `funcDecl` to track parameter counts and emit correct forward declarations.

### Grammar Cleanup

Removed from `grammar.md`:
- Python-like slicing syntax (never implemented)
- String interpolation (never implemented)
- `try`/`unwrap` statements (never implemented)
- Generic type expressions
- Form 2 lambda special case
- Struct/impl marked as [FUTURE]
