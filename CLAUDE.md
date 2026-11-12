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
- CMake (only on platforms without a prebuilt Boehm GC; linux-amd64, linux-arm64, darwin-arm64 and windows-amd64 have one)

## Compiler Pipeline

```
.qrk source → Lexer → Parser → Loader → Analyzer → Invariants → Codegen → C++ → clang++ → binary
```

All stages live under `src/core/quark/`:

| Stage | Package | Purpose |
|-------|---------|---------|
| Lexer | `lexer/` | Indentation-aware tokenization (emits INDENT/DEDENT tokens) |
| Parser | `parser/` | Recursive descent → AST (`ast/ast.go` defines `TreeNode`) |
| Loader | `loader/` | Resolves `use` imports (relative paths and `std/` stdlib), splices ASTs, then renames module members to `module__member` (`namespace.go`) so modules and user code can't collide |
| Analyzer | `types/` | Semantic analysis, scope/symbol tables, closure capture computation, generates `CallPlan` IR |
| Invariants | `invariants/` | Pre-codegen validation (call plans, return types) |
| Codegen | `codegen/` | AST → C++17 source generation |
| Driver | `driver/` | Runs the stages above in order (`CompileFile(path, stage)`); used by every CLI command except `lex`. `internal/testutil` runs its own lex/parse/analyze on in-memory source and reuses `driver.ResolveImports`/`Validate`/`Generate` |
| Toolchain | `toolchain/` | clang++/runtime/GC discovery, compile and link steps, precompiled runtime header cache |
| Entry | `main.go` | CLI dispatch and flag parsing |

## Runtime System

Header-only C++17 library at `src/core/quark/runtime/include/quark/`. Key files:

- `core/value.hpp` — `QValue` tagged union (the universal value type)
- `types/closure.hpp` — `QClosure` struct (all function values are closures)
- `core/gc.hpp` — GC allocation wrappers (`q_malloc`, `q_malloc_atomic`, `q_strdup`)
- `ops/` — Arithmetic, comparison, logical, member access operators
- `builtins/` — prelude builtins (console I/O, string, math, list, dict, conversion); each header must be listed in `quark.hpp`
- `builtins/repr.hpp` — how values print (`println`/`to_str`: contents, 100-item elision); `core/format.hpp` — float text (shortest round-trip)
- `types/resource.hpp` — generational registry for native handles; every handle has the single Quark type `resource` (the kind, e.g. `file`, is runtime-only)

There are no runtime unit tests; the runtime is tested through the Go integration tests (some compile small C++ programs against the headers, e.g. `TestVectorArrowLayout`).

## Naming Conventions (Critical)

| Prefix | Domain | Examples |
|--------|--------|---------|
| `q_` | Runtime builtins/helpers (C++ runtime code) | `q_add()`, `q_print()`, `q_malloc()` |
| `quark_` | User-defined names in generated C++ (compiler codegen output); `extern fn` symbols keep their C++ name and get `_thunk_<symbol>` wrappers | `quark_main()`, `quark_x` |
| `qv_` | Value constructors | `qv_int()`, `qv_string()`, `qv_null()` |
| `QValue`, `QList`, `QClosure` | Type names (PascalCase) | — |

**Never mix `q_` and `quark_` prefixes** — `q_` is runtime-only, `quark_` is codegen-only.

## Key Architecture Details

- **All values are `QValue`** — a tagged union with type field. Runtime ops must check type before accessing union fields; return `qv_null()` on mismatch.
- **All function values are `QClosure*`** — even non-capturing functions. Generated functions take `QClosure* _cl` as hidden first parameter. Named functions desugar to mutable bindings and are normally called through `q_callN(closure, ...)`; only calls that bypass the closure (fully annotated `DispatchNative` functions, `DispatchDirect`, thunks) pass `nullptr`.
- **Variable storage is type-and-capture-driven**: five tiers selected at first declaration — `QCell*` (captured), `long long` (static `int`, not captured), `double` (static `float`, not captured), `bool` (static `bool`, not captured), `QValue` (everything else). `Analysis.CapturedByFunction` and `Analysis.NodeTypes` (passed via `SetAnalysis`) drive this. Codegen tracks cells in `cellVars` and scalar tiers in `varTiers`. `generateIdentifier` boxes scalars back to `QValue` at read sites so all downstream codegen works uniformly with `QValue` expressions.
- **Operator lowering**: `scalarExpr()` recursively lowers arithmetic, unary `-` and comparisons whose operands are all scalars (literals, scalar-tiered locals, or lowered sub-expressions). Int `+ - * %` use overflow-checked `q_checked_add/sub/mul/mod`, `/` uses `q_checked_fdiv` (always `double`), floats use raw C++ operators; `**` is never lowered (`q_pow`). The result stays unboxed when assigned to a scalar-tier variable or used as a condition, otherwise it's boxed to `QValue`.
- **Conditions** (`if`/`while`/`and`/`or`/`!`) use `q_truthy()` truthiness rules (there is no `not` keyword); `and`/`or` are short-circuit lambdas that return an operand.
- **Memory** — Boehm GC vendored at `deps/bdwgc/`: only the public headers, prebuilt static libraries in `lib/<GOOS>-<GOARCH>/`, `build-prebuilt.sh` (rebuilds them), `VENDORED.md` and LICENSE are committed; the GC source is gitignored. `toolchain.EnsureGC` uses the prebuilt library, else a local `build-static/` CMake build (see `deps/bdwgc/VENDORED.md`). Use `q_malloc_atomic()` for data without pointers (strings, numeric buffers). The collector never runs destructors and doesn't scan `malloc`'d memory, so a `std::string`/`std::vector`/map stored in (or reachable only from) runtime data must use `q_allocator` (dict keys and the resource registry do); otherwise it leaks or its contents get collected while live. `integration_gc_test.go` (programs in `src/testfiles/gc/`) checks for both by comparing GC heap and peak RSS across repeated rounds.
- **Analyzer → codegen handoff** — the analyzer returns one `types.Analysis` struct (captures, call plans, node types, native/extern fns, struct types, ...), passed to codegen with `SetAnalysis`. New analyzer outputs go in that struct, not new setters.
- **CallPlan** (`ir/call.go`) — IR metadata attached to each call site by the analyzer. Tracks call kind (builtin vs user), arity, default arg filling. Codegen reads these instead of re-analyzing.
- **Builtin catalog** (`builtins/`) — shared metadata (name, arity, signatures) used by the analyzer and `invariants/`; codegen never reads it and emits `CallPlan.RuntimeSymbol` instead. Methods are indexed by `(ReceiverType, methodName)` pair, separate from free functions.
- **Stdlib modules** (`src/core/stdlib/`) — each is a `.qrk` file with a `module X:` block, and most bind native code from a sibling `.hpp` via `extern 'x.hpp'` + `extern fn` (`io`, `sys`, `time`, `random`, `csv`, `fmt`, `blas`). Functionality that isn't needed in the prelude belongs here, not in the catalog; file I/O, for example, is `std/io` only, and there are no `_file_*` builtins.
- **Native libraries (FFI)** — `extern link 'name'` / `extern include_path 'dir'` are collected by the loader into `driver.Unit.Native` (link order follows the import graph); `toolchain/native.go` resolves libraries before linking (`QK-FFI-002`), adds `-L`/rpath/`-l` args, and maps C++ errors in glue headers to `QK-FFI-003`. `integration_ffi_test.go` builds the C test libraries in `src/testfiles/ffi/` at test time; `TestStdBlas` skips when OpenBLAS isn't installed. Spec: `ffi_v0_1_spec.md`, which is canonical for all native interop (`native_modules_design.md` was removed).
- **Method dispatch** — When codegen sees `IsMethod=true` on a CallPlan, the receiver expression is injected as the first runtime argument (e.g., `"hello".upper()` becomes `q_upper(receiver_val)`). When the receiver's type is unknown at compile time (`any`, e.g. from `unwrap`), the plan is `DispatchDynamicMethod` and codegen switches on the runtime type tag over `MethodCandidates`.

## Adding a New Builtin

1. Add a `Spec` entry in `builtins/catalog.go` (set `ReceiverType` for methods, leave empty for free functions)
2. Implement the C++ function in `runtime/include/quark/builtins/*.hpp` with `q_` prefix (a new header must also be added to the include list in `runtime/include/quark/quark.hpp`)
3. Analyzer and codegen pick it up automatically via the catalog (codegen through the CallPlan's `RuntimeSymbol`)
4. Add a smoke test in `src/testfiles/smoke_*.qrk`
5. Regenerate the editor extension's data: `go test ./internal/editordata -update` (and add the name to the grammar's `builtin-functions` pattern; the test says so if it's missing)

## Editor Extension

`src/devtools/vscode-extension/quark` (VS Code / Positron). Package with `go run ./cmd/quark-vsix` from `src/core/quark` (no Node/npm). `data/quark.json` is generated from the catalog, lexer keywords and stdlib by `internal/editordata`; that package's tests also check grammar coverage, that snippets parse, and run `test/extension.test.js` when `node` (or `QUARK_NODE`) is available.

## Test Files

Smoke test programs in `src/testfiles/smoke_*.qrk`. Integration tests in `src/core/quark/integration_smoke_test.go` compile and run these, comparing stdout against expected output.

## Language Notes

- Indentation-based blocks (Python-like)
- `list [1, 2, 3]` syntax (requires `list` keyword)
- `dict { key: val }` syntax
- Pipe operator: `x | foo() | bar()`
- Result types: `ok(val)` / `err(val)` with `unwrap()`
- Type annotations: `fn add(x: int, y: int) int -> x + y`. Each annotated parameter is enforced on its own (partial annotation counts): checked at compile time when the argument type is known, on function entry otherwise (`emitParamBindings` → `q_param_*` in `builtins/params.hpp`), with int→float widening; same rules as `extern fn`. Fully annotated, non-capturing functions compile to native C++ (`DispatchNative`)
- Default params: `fn foo(x: int, y: int = 0) -> x + y` (literals only)
- String type is `str` (not `string`)
- Annotation types: `int`, `float`, `str`, `bool`, `any`, `list`, `dict`, `vector`, `result`, `resource`, `fn` (`types.TypeFn`: any function value, no signature; `QClosure*` in native/extern signatures), and struct names. No generic/element types (`list[int]` doesn't parse)
