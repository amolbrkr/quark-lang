# Quark

Quark is a high-level, dynamically-typed language that compiles to optimized C++17.

It is designed to feel readable and expressive like a scripting language, while still producing native binaries that can run fast on data-heavy workloads.

This repository contains the active Go compiler implementation, C++ runtime headers, and smoke/benchmark programs.

## 1) Introduction: What Quark Is and Language Goals

### Language goals

Quark is built around five practical goals:

1. Minimal readable syntax with low ceremony.
2. Out of the box performance for data heavy operations.
3. Good compile time guarantees and interop with C++.

### Core philosophy

1. Language ergonomics and performance are equal, primary goals.
2. Developer productivity and quality-of-life are first-class concerns.
3. Performance should not come at the cost of user-facing complexity.
4. Fail early and fail loudly with clear diagnostics.
5. Less is more for language surface area and syntax.
6. Explicit is better than implicit, especially at boundaries.
7. Prefer boring, reliable defaults over cleverness.

### Current language shape

Quark currently supports:

- Indentation-based blocks.
- Functions, lambdas, and closures.
- Conditionals, loops, ternary expressions, and pattern matching.
- Explicit result values using ok/err pattern and related helpers.
- Pipelined call style via the pipe operator.
- Method dispatch on strings, lists, dicts, and vectors.
- Multi-file imports and stdlib path imports.

```quark
nums = list [1, 2, 3]
nums.push(4)
len(nums)
'hello'.upper()
```

Pipes chain free-function calls:

```quark
'hello'.upper() | println()
```

Dot syntax serves three purposes: dict key access, method calls, and module-qualified calls.

```quark
d = dict { name: 'quark' }
println(d.name)

'hello world'.split(' ') | println()

use 'std/demo_math' as dm
println(dm.add10(5))
```

### Where Quark sits today

Quark is already usable for many small to medium programs and language experiments. It has strict analyzer/runtime checks and a full compile pipeline, but some features are still planned (for example structs/impl blocks and tensor support).

## 2) Install and Run the Compiler

### Prerequisites

- Go 1.21+
- clang++ or g++ in PATH
- CMake in PATH (for Boehm GC bootstrap)
- Windows, Linux, or macOS

### Build the compiler

From the repository root:

```bash
cd src/core/quark
go build -o quark .
```

On Windows, if you want an explicit exe file name:

```powershell
cd src/core/quark
go build -o quark.exe .
```

### CLI commands

```bash
quark lex <file>
quark parse <file>
quark check <file>
quark emit <file>
quark build <file> [-o out]
quark run <file> [--debug|-d]
```

Shorthand:

```bash
quark program.qrk
```

This is equivalent to running `quark run program.qrk`.

### First run example

```bash
cd src/core/quark
./quark run ../../../src/testfiles/smoke_syntax.qrk
```

### Boehm GC behavior

Quark vendors Boehm GC under deps/bdwgc. During build/run, the compiler will try to find a built GC library and, if missing, bootstrap it via CMake.

### Stdlib import resolution

Quoted stdlib imports use the `std/...` prefix:

```quark
use 'std/demo_math' as dm
```

Stdlib root is resolved in this order:

1. QUARK_STDLIB_ROOT environment variable.
2. Upward search for a directory named stdlib from the source file location.
3. Executable-relative fallbacks (stdlib near the compiler binary).

If you package Quark for production, setting QUARK_STDLIB_ROOT explicitly is the most robust approach.

## 3) Common Language Patterns (with Examples)

### Functions and expression bodies

```quark
fn add(x, y) -> x + y
println(add(2, 3))
```

### Multi-line function bodies

```quark
fn classify(n) ->
    if n < 0:
        'negative'
    elseif n == 0:
        'zero'
    else:
        'positive'

println(classify(10))
```

### Pattern matching with when

```quark
fn fib(n) ->
    when n:
        0 -> 0
        1 -> 1
        _ -> fib(n - 1) + fib(n - 2)

println(fib(8))
```

### Error-aware flows with ok/err

```quark
fn safe_div(a, b) ->
    if b == 0:
        err 'division by zero'
    else:
        ok a / b

when safe_div(10, 2):
    ok value -> println(value)
    err msg -> println(msg)
```

### Pipes for readable transformations

```quark
'  quark  '.trim().upper() | println()
```

### Lists for general-purpose dynamic collections

```quark
nums = list [1, 2, 3]
nums.push(4)
nums.get(0) | println()
len(nums) | println()
```

### Vectors for typed, data-oriented operations

```quark
v = vector [1, 2, 3, 4]
w = v + 10
println(sum(w))
println(sum(v > 2))
```

### Dict access patterns

```quark
user = dict { name: 'alex', age: 30 }
println(user.name)

k = 'name'
user.get(k) | println()
user = user.set('city', 'dublin')
println(user.city)
```

### Module usage patterns

Same-file module:

```quark
module math:
    fn square(x) -> x * x

use math as m
println(m.square(9))
```

File import:

```quark
use './lib/helpers' as h
println(h.format_name('Ada'))
```

Stdlib path import:

```quark
use 'std/demo_math' as dm
println(dm.add10(32))
```

## 4) Stdlib: Complete Builtins Reference

All builtins are globally available; no import is required.

### Free Functions

#### I/O

| Function | Arity | Returns | Notes |
|---|---:|---|---|
| print | 1..5 | void | Configurable end, width, alignment, pad |
| println | 1 | void | Prints with newline |
| input | 0..1 | str | Optional prompt must be string |

#### Conversions, Introspection, Result Helpers

| Function | Arity | Returns | Notes |
|---|---:|---|---|
| len | 1 | int | Works on str/list/dict/vector |
| to_str | 1 | str | General conversion |
| to_int | 1 | int | Runtime error on invalid parse |
| to_float | 1 | float | Runtime error on invalid parse |
| to_bool | 1 | bool | Truthiness conversion |
| type | 1 | str | Runtime type name |
| is_ok | 1 | bool | Expects result value |
| is_err | 1 | bool | Expects result value |
| unwrap | 1 | any | Panics on err |

#### Range

| Function | Arity | Returns | Notes |
|---|---:|---|---|
| range | 1..3 | list | range(end), range(start,end), range(start,end,step) |

#### Math

| Function | Arity | Returns | Notes |
|---|---:|---|---|
| abs | 1 | any | Preserves type |
| min | 1..2 | any | Scalar or vector |
| max | 1..2 | any | Scalar or vector |
| sum | 1 | any | Vector/list reduction |
| sqrt | 1 | float | Domain error on negative |
| floor | 1 | int | Float to int |
| ceil | 1 | int | Float to int |
| round | 1 | int | Float to nearest int |

#### Other

| Function | Arity | Returns | Notes |
|---|---:|---|---|
| enumerate | 1 | list | Build list of `{ index, value }` records |

### Methods (by receiver type)

#### String methods (`str`)

| Method | Description |
|--------|-------------|
| `.upper()` | Uppercase copy |
| `.lower()` | Lowercase copy |
| `.trim()` | Strip leading/trailing whitespace |
| `.contains(sub)` | Substring test |
| `.startswith(prefix)` | Prefix test |
| `.endswith(suffix)` | Suffix test |
| `.replace(old, new)` | Replace all occurrences |
| `.concat(other)` | Concatenate strings |
| `.split(sep)` | Split by separator |
| `.slice(start, end)` | Substring `[start:end)` |

#### List methods (`list`)

| Method | Description |
|--------|-------------|
| `.push(item)` | Append item; returns updated list |
| `.pop()` | Remove and return last item |
| `.get(idx)` | Get at index (out-of-bounds → null) |
| `.set(idx, val)` | Set at index; returns value |
| `.insert(idx, val)` | Insert at index; returns list |
| `.remove(idx)` | Remove at index; returns removed item |
| `.slice(start, end)` | Sublist `[start:end)` |
| `.reverse()` | Reverse in place; returns list |
| `.concat(other)` | Concatenate two lists |
| `.join(sep)` | Join elements with separator |
| `.enumerate()` | Build `{ index, value }` records |
| `.to_vector()` | Convert to typed vector |

#### Dict methods (`dict`)

| Method | Description |
|--------|-------------|
| `.get(key)` | Get value by key (missing → null) |
| `.set(key, val)` | Set key/value; returns updated dict |
| `.keys()` | Return list of keys |
| `.values()` | Return list of values |
| `.items()` | Return list of `{ key, value }` records |

#### Vector methods (`vector`)

| Method | Description |
|--------|-------------|
| `.get(idx)` | Get scalar value at index |
| `.fillna(val)` | Replace null entries |
| `.astype(dtype)` | Cast vector dtype |
| `.to_list()` | Convert back to list |

### Quick stdlib snippets

```quark
println(to_int('42'))
println(range(1, 5))
'quark'.upper() | println()

vals = list [1, 2, 3]
vals.push(4)
println(sum(vals.to_vector()))
```

For a deeper narrative and behavior notes, see stdlib.md.

## 5) Architecture and Compiler Setup

### High-level pipeline

```text
                 Quark Source (.qrk)
                         |
                         v
+-------------------+  tokens  +-------------------+
| Lexer (Go)        | -------> | Parser (Go)       |
| - indentation     |          | - AST             |
| - token stream    |          | - module/use nodes|
+-------------------+          +-------------------+
                                        |
                                        v
                              +-------------------+
                              | Import Loader     |
                              | - file imports    |
                              | - std/ imports    |
                              | - cycle checks    |
                              +-------------------+
                                        |
                                        v
                              +-------------------+
                              | Analyzer (Go)     |
                              | - scopes/types    |
                              | - call plans      |
                              | - diagnostics     |
                              +-------------------+
                                        |
                                        v
                              +-------------------+
                              | Invariants        |
                              | - call plan checks|
                              | - return checks   |
                              +-------------------+
                                        |
                                        v
                              +-------------------+
                              | Codegen (Go)      |
                              | -> C++17 source   |
                              +-------------------+
                                        |
                                        v
                              +-------------------+
                              | clang++ / g++     |
                              | -O3 + arch flags  |
                              +-------------------+
                                        |
                                        v
                                 Native Executable
```

### Repository layout (important parts)

- src/core/quark: active Go compiler implementation.
- src/core/quark/runtime/include/quark: header-only runtime.
- deps/bdwgc: vendored Boehm GC source.
- src/testfiles: smoke programs.
- stdlib: repository stdlib modules (used by `use 'std/...'`).

### Build/link details

Compiler invocations generated by Quark use:

- C++17 mode.
- O3 optimization.
- Architecture flag on amd64 builds.
- Runtime include path for Quark headers.
- Boehm GC include/lib when GC is enabled.

### Error model

Quark favors explicit failure:

- Analyzer catches concrete type and arity errors when knowable.
- Runtime checks guard dynamic paths.
- Type/domain violations fail loudly rather than silently returning neutral values.

Documented exceptions:

- `.get(idx)` on list returns null for out-of-bounds.
- `.get(key)` on dict returns null for missing keys.

### Status summary

Implemented:

- Full lexer/parser/analyzer/codegen pipeline.
- Closures and function values.
- Pipes, control-flow, pattern matching.
- Lists, dicts, vectors, results.
- Method dispatch on str, list, dict, vector.
- Multi-file imports and stdlib imports.

Planned:

- Structs and impl blocks.
- Tensor type.
- Additional optimizer passes beyond current architecture.

For detailed implementation internals, see architecture.md.

## License

MIT License
