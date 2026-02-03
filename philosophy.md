# Language Philosophy

Quark is a high-level, dynamically-typed language that compiles to C++, designed for fast data-heavy applications. It aims to provide the ease of use of Python with the performance of native code.

## Design Goals

### 1. Performance First
- Compiles to optimized C++ with `-O3` and SIMD auto-vectorization
- Lists backed by `std::vector` for cache-friendly data processing
- Native binaries with no interpreter overhead

### 2. Clean Syntax
- Python-style indentation for blocks
- Minimal punctuation where possible (optional parentheses for function calls)
- Pipe operator `|` for readable data flow

### 3. Dynamic Yet Safe
- Type inference eliminates annotation burden
- Runtime type checking prevents undefined behavior
- Boxed values with tagged unions for safe dynamic typing

## Language Features

### Variables
- Type-inferred with optional annotations (planned)
- Currently mutable by default

### Functions
- The last expression in a function block is the return value
- First-class values - can be passed and returned
- Support for closures (planned)

### Data Structures
- Lists: Backed by `std::vector<QValue>` for efficient operations
- Strings: Immutable, operations return new strings
- More types planned: maps, sets, structs

## Target Use Cases

1. **Data Processing** - ETL pipelines, log analysis, data transformation
2. **Scripting** - Build scripts, automation, glue code
3. **Numerical Computing** - Math-heavy applications with SIMD optimization
4. **Prototyping** - Quick iteration with native performance

## Non-Goals

- **Systems Programming** - Use Rust/C++ for low-level control
- **Web Frontend** - No browser/WASM target planned
- **Enterprise Applications** - Not designed for large team codebases

## Implementation Philosophy

### Compiler Architecture
- **Go frontend**: Fast compilation, easy to extend
- **C++ backend**: Leverage mature optimizers (clang/gcc)
- **Header-only runtime**: Simple deployment, no library dependencies

### Code Generation
- Direct translation to C++ (no VM or bytecode)
- All runtime functions are `inline` for optimization
- Generated code is readable and debuggable
