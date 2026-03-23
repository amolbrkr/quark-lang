# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## What is Quark

Quark is a dynamically-typed language that compiles to C++17 native binaries. The compiler is written in Go. The runtime is a header-only C++ library using Boehm GC for memory management.

## Build & Test Commands

```bash
# Build the compiler
cd src/core/quark && go build -o quark.exe .

# Run a Quark program (compile + execute)
quark run src/testfiles/test.qrk

# Build without running
quark build src/testfiles/test.qrk -o output.exe

# Other pipeline stages (debugging)
quark lex <file>      # Print tokens
quark parse <file>    # Print AST
quark check <file>    # Semantic analysis only
quark emit <file>     # Print generated C++ to stdout

# Run all integration tests
cd src/core/quark && go test -v

# Run a single test by name
cd src/core/quark && go test -v -run TestSmokePrograms_Run
```

## Prerequisites

- Go 1.21+
- clang++ (C++17) — **required**, g++ is not supported
  - Ubuntu/Debian: `sudo apt install clang`
  - macOS: `brew install llvm`
- CMake (for Boehm GC bootstrap on first run)

## Compiler Pipeline

```
.qrk source → Lexer → Parser → Loader → Analyzer → Invariants → Codegen → C++ → clang++ → binary
```

All stages live under `src/core/quark/`:

| Stage | Package | Purpose |
|-------|---------|---------|
| Lexer | `lexer/` | Indentation-aware tokenization (emits INDENT/DEDENT tokens) |
| Parser | `parser/` | Recursive descent → AST (`ast/ast.go` defines `TreeNode`) |
| Loader | `loader/` | Resolves `use` imports (relative paths and `std/` stdlib), splices ASTs |
| Analyzer | `types/` | Semantic analysis, scope/symbol tables, closure capture computation, generates `CallPlan` IR |
| Invariants | `invariants/` | Pre-codegen validation (call plans, return types) |
| Codegen | `codegen/` | AST → C++17 source generation |
| Entry | `main.go` | CLI dispatch, C++ compilation, GC library discovery |

## Runtime System

Header-only C++17 library at `src/core/quark/runtime/include/quark/`. Key files:

- `core/value.hpp` — `QValue` tagged union (the universal value type)
- `types/closure.hpp` — `QClosure` struct (all function values are closures)
- `core/gc.hpp` — GC allocation wrappers (`q_malloc`, `q_malloc_atomic`, `q_strdup`)
- `ops/` — Arithmetic, comparison, logical, member access operators
- `builtins/` — I/O, string, math, list, dict, conversion builtins

Runtime tests use Catch2: `src/core/quark/runtime/tests/`

## Naming Conventions (Critical)

| Prefix | Domain | Examples |
|--------|--------|---------|
| `q_` | Runtime builtins/helpers (C++ runtime code) | `q_add()`, `q_print()`, `q_malloc()` |
| `quark_` | ALL user-defined names in generated C++ (compiler codegen output) | `quark_main()`, `quark_x` |
| `qv_` | Value constructors | `qv_int()`, `qv_string()`, `qv_null()` |
| `QValue`, `QList`, `QClosure` | Type names (PascalCase) | — |

**Never mix `q_` and `quark_` prefixes** — `q_` is runtime-only, `quark_` is codegen-only.

## Key Architecture Details

- **All values are `QValue`** — a tagged union with type field. Runtime ops must check type before accessing union fields; return `qv_null()` on mismatch.
- **All function values are `QClosure*`** — even non-capturing functions. Generated functions take `QClosure* _cl` as hidden first parameter. Direct calls pass `nullptr`.
- **Variable storage is capture-driven**: variables captured by a nested lambda are emitted as `QCell*` (heap cell, `cell->value` for reads/writes); all other variables are emitted as stack `QValue`. The analyzer's `GetCapturedByFunction` output drives this. Codegen tracks which vars are cells in `cellVars`.
- **Memory** — Boehm GC vendored at `deps/bdwgc/`. Use `q_malloc_atomic()` for data without pointers (strings, numeric buffers).
- **CallPlan** (`ir/call.go`) — IR metadata attached to each call site by the analyzer. Tracks call kind (builtin vs user), arity, default arg filling. Codegen reads these instead of re-analyzing.
- **Builtin catalog** (`builtins/`) — shared metadata (name, arity, signatures) used by both analyzer and codegen. Methods are indexed by `(ReceiverType, methodName)` pair, separate from free functions.
- **Method dispatch** — When codegen sees `IsMethod=true` on a CallPlan, the receiver expression is injected as the first runtime argument (e.g., `"hello".upper()` becomes `q_upper(receiver_val)`).

## Adding a New Builtin

1. Add a `Spec` entry in `builtins/catalog.go` (set `ReceiverType` for methods, leave empty for free functions)
2. Implement the C++ function in `runtime/include/quark/builtins/*.hpp` with `q_` prefix
3. Analyzer and codegen pick it up automatically via the catalog
4. Add a smoke test in `src/testfiles/smoke_*.qrk`

## Test Files

Smoke test programs in `src/testfiles/smoke_*.qrk`. Integration tests in `src/core/quark/integration_smoke_test.go` compile and run these, comparing stdout against expected output.

## Language Notes

- Indentation-based blocks (Python-like)
- `list [1, 2, 3]` syntax (requires `list` keyword)
- `dict { key: val }` syntax
- Pipe operator: `x | foo() | bar()`
- Result types: `ok(val)` / `err(val)` with `unwrap()`
- Type annotations: `fn add(x: int, y: int) int -> x + y` (compile-time only)
- Default params: `fn foo(x: int, y: int = 0) -> x + y` (literals only)
- String type is `str` (not `string`)
- No generic types — only basic annotations: `int`, `float`, `str`, `bool`, `list`, `dict`
