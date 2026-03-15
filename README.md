# Quark

Quark is a high-level, dynamically-typed language that compiles to optimized C++17.

It is designed to feel readable and expressive like a scripting language, while still producing native binaries that can run fast on data-heavy workloads.

This repository contains the active Go compiler implementation, C++ runtime headers, and smoke/benchmark programs.

## 1) Introduction: What Quark Is and Language Goals

### Language goals

Quark is built around five practical goals:

1. Readable syntax with low ceremony.
2. Native performance via ahead-of-time C++ codegen.
3. Predictable semantics and explicit errors.
4. Practical data operations with list, dict, and typed vector support.

### Current language shape

Quark currently supports:

- Indentation-based blocks.
- Functions, lambdas, and closures.
- Conditionals, loops, ternary expressions, and pattern matching.
- Explicit result values using ok/err and related helpers.
- Pipelined call style via the pipe operator.
- Multi-file imports and stdlib path imports.

Quark deliberately does not support value method dispatch. The canonical model is function-call style:

```quark
push(nums, 4)
len(nums)
upper('hello')
```

Pipes are equivalent call sugar:

```quark
'hello' | upper() | println()
```

Dot access is for dict data access, with one exception: module-qualified calls via aliases.

```quark
d = dict { name: 'quark' }
println(d.name)

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
'  quark  ' | trim() | upper() | println()
```

### Lists for general-purpose dynamic collections

```quark
nums = list [1, 2, 3]
push(nums, 4)
println(get(nums, 0))
println(len(nums))
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
println(dget(user, k))
user = dset(user, 'city', 'dublin')
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

### I/O

| Function | Arity | Returns | Notes |
|---|---:|---|---|
| print | 1 | void | Prints without newline |
| println | 1 | void | Prints with newline |
| input | 0..1 | str | Optional prompt must be string |

### Conversions, Introspection, Result Helpers

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

### Range

| Function | Arity | Returns | Notes |
|---|---:|---|---|
| range | 1..3 | list[int] | range(end), range(start,end), range(start,end,step) |

### Math

| Function | Arity | Returns | Notes |
|---|---:|---|---|
| abs | 1 | any | Numeric/vector behaviors enforced by analyzer/runtime |
| min | 1..2 | any | Scalar or vector overload behavior |
| max | 1..2 | any | Scalar or vector overload behavior |
| sum | 1 | any | Scalar/list/vector dependent behavior |
| sqrt | 1 | float | Domain error on negative |
| floor | 1 | int | Float to int |
| ceil | 1 | int | Float to int |
| round | 1 | int | Float to nearest int |

### String

| Function | Arity | Returns | Notes |
|---|---:|---|---|
| upper | 1 | str | Uppercase copy |
| lower | 1 | str | Lowercase copy |
| trim | 1 | str | Strip leading/trailing whitespace |
| contains | 2 | bool | substring test |
| startswith | 2 | bool | prefix test |
| endswith | 2 | bool | suffix test |
| replace | 3 | str | Replace all occurrences |
| concat | 2 | any | Supports str+str and list+list |
| split | 2 | list[str] | Separator-based split |

### List

| Function | Arity | Returns | Notes |
|---|---:|---|---|
| push | 2 | list | Append item |
| pop | 1 | any | Runtime error on empty list |
| get | 2 | any | Out-of-bounds returns null |
| set | 3 | any | Index assignment semantics |
| insert | 3 | list | Insert at index |
| remove | 2 | any | Remove at index |
| slice | 3 | list | [start, end) |
| reverse | 1 | list | In-place reverse |

### Dict

| Function | Arity | Returns | Notes |
|---|---:|---|---|
| dget | 2 | any | Missing key returns null |
| dset | 3 | dict | Set key/value and return dict |

### Vector

| Function | Arity | Returns | Notes |
|---|---:|---|---|
| fillna | 2 | vector | Replace null-like entries |
| astype | 2 | vector | Cast vector dtype |
| to_vector | 1 | any | Convert list/vector to vector form |
| to_list | 1 | any | Convert vector/list to list form |

### Quick stdlib snippets

```quark
println(to_int('42'))
println(range(1, 5))
println(upper('quark'))

vals = list [1, 2, 3]
push(vals, 4)
println(sum(to_vector(vals)))
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

- get(list, idx) may return null for out-of-bounds.
- dget(dict, key) returns null for missing keys.

### Status summary

Implemented:

- Full lexer/parser/analyzer/codegen pipeline.
- Closures and function values.
- Pipes, control-flow, pattern matching.
- Lists, dicts, vectors, results.
- Multi-file imports and stdlib imports.

Planned:

- Structs and impl blocks.
- Tensor type.
- Additional optimizer passes beyond current architecture.

## License

MIT License
