# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Contents

- [What is Quark](#what-is-quark)
- [Language Goals: A language with focus on Data where developer productivity and execution speed are equal goals](#language-goals-a-language-with-focus-on-data-where-developer-productivity-and-execution-speed-are-equal-goals)
- [Design Principle: Catch It at Compile Time](#design-principle-catch-it-at-compile-time)
- [Robustness Bar (read before every change)](#robustness-bar-read-before-every-change)
- [Changelog (update every day there are changes)](#changelog-update-every-day-there-are-changes)
- [Docs (keep them current)](#docs-keep-them-current)
- [Build & Test Commands](#build--test-commands)
- [Prerequisites](#prerequisites)
- [Compiler Pipeline](#compiler-pipeline)
- [Runtime System](#runtime-system)
- [Naming Conventions (Critical)](#naming-conventions-critical)
- [Key Architecture Details](#key-architecture-details)
- [Adding a New Builtin](#adding-a-new-builtin)
- [Editor Extension](#editor-extension)
- [Test Files](#test-files)
- [Language Notes](#language-notes)

## What is Quark

Quark is a language for data work that compiles to C++17 native binaries. Types are inferred (annotations are optional) and checked at compile time; only values the compiler can't know (`any`, native code, data read at runtime) are checked at runtime. The compiler is written in Go. The runtime is a header-only C++ library using Boehm GC for memory management.

## Language Goals: A language with focus on Data where developer productivity and execution speed are equal goals
Quark needs to fast and convienent to write AND should also run fast in execution because the goal is to work with large amounts of Data in a safe and reproducible manner. These two goals are equal and can often be contrary to each, so ask the developer questions if you need input in desgin choices which can affect these goals. 

## Design Principle: Catch It at Compile Time

Quark is compiled so that it doesn't share the weakness of R and Python codebases, where most failures surface at runtime. The compiler's job is to work out as much as it can before the program runs: types (inference, not just annotations), execution paths, module symbols, and how values are *intended* to be used (for example, how a function parameter is called inside the body, so a bad argument is rejected at the call site) and more... Spending compile time on this is the right trade. The target is Rust-like trust: a program that compiles runs as intended about 99% of the time, and the rest fails the way it was designed to fail. When adding a feature, first ask what the compiler can prove about it, and treat runtime checks as the backstop for what it truly can't know (values from `any`, native code, data read at runtime), not as the main mechanism.

## Robustness Bar (read before every change)

The goal is a language any developer can trust: a wrong program fails loudly and clearly, never "works" with a wrong answer. Hold every change to these rules. They apply to the code the task writes or changes; bugs found elsewhere along the way are recorded, not fixed (rule 7), and an issue that stands in the way of the task stops it (rule 8).

1. **No silent wrong results.** Misuse is a compile-time error when the compiler can know, otherwise a runtime error with a source location. Never garbage values, undefined behavior, crashes, or a user's definition being silently ignored. Reading a `QValue` union field (`.data.int_val`, ...) without a tag check is only allowed where the analyzer has *proven* the type; values from untyped paths (`any`, callbacks, struct fields, native code) go through the checked `q_as_*`/`q_param_*` helpers.
2. **Probe the misuse paths of anything you add or make more reachable.** For a new feature, test what happens with: the wrong type, the wrong number of arguments, `null`, an untyped (`any`) value, default parameters, a value from another module, and a call from or into native code. Each gets a negative test (`TestSemanticsSafety_Rejected` or similar) before the work is called done.
3. **A hole your change makes easier to hit is the user's call.** If your feature makes an existing silent-wrong-result bug easier to hit, record it in `known_issues.md` with its reach (which change, and how), finish the task, and put it first in your summary: the user decides whether it blocks the change. Never present such a feature as done without saying so. "Known limitations" in the README is for deliberate gaps (missing features), not for unsoundness.
4. **Report what you found at the end, up front**: every issue you added to `known_issues.md` goes at the top of your final summary, with its reproduction, not as a footnote.
5. **Same rule everywhere.** New behavior must match the closest existing behavior (for example, if parameters widen int→float, so must struct fields and returns); if it can't, raise the inconsistency instead of adding another one.
6. **Names never collide silently.** Every top-level name of an `incl`uded file is reachable from the importer, qualified with `::`, by design (there are no access modifiers; stdlib `extern fn` bindings such as `fmt::fmt_vec` are reachable too). Names are resolved per file (modules v0.2), so two files can't clash; within one file, a clash (a name defined twice, a module member named like a top-level name, an import binding named like a definition) must be a compile-time error, never one definition silently winning. Nothing from another file may be visible unqualified. Compiler-internal names (`q<N>_<stem>__name`, `module__member`, `q_`/`quark_` C++ symbols) must never be reachable from user code: every generated name contains `__`, which the loader rejects in user names (`checkReservedNames`).
7. **Record, don't fix, what you find along the way.** A bug in code the task doesn't change (a soundness hole, a wrong doc, a misleading message, a slow path) goes into `known_issues.md` with a reproduction, and the task continues; don't fix it in the same run, however small it looks. Known issues are fixed in runs of their own, which the user starts. Two things are part of the task and get fixed: a defect in your own new code, and a doc sentence your change has to touch anyway; say so in the summary. An issue that stands in the way of the task is rule 8, not a fix-on-the-fly.
8. **Stop when the task can't be done well.** If, while implementing X, you find an issue that prevents a sensible implementation of X (a bug or a design gap underneath it), stop before going further and tell the user:
   - what the issue is, with a minimal reproduction and the code involved (file and function, a short excerpt);
   - why it blocks a clean X, and what X would look like on top of it;
   - the options and their trade-offs (fix the issue first, narrow or reshape X, wait), each with a short code snippet of what it would look like;
   - your recommendation.

   Don't route around it to make X work: no special cases, duplicated logic, checks in the wrong layer or other workarounds that add complexity or technical debt. A new feature lands with a clean implementation, or waits for the user's decision. Record the issue in `known_issues.md` as well.

## Changelog (update every day there are changes)

`changelog.md` at the repo root has one section per day, newest first. On any day you change behavior (language, CLI, stdlib, runtime, toolchain, editor extension), update it as part of the work, not as a later cleanup:

- Add today's section (`## YYYY-MM-DD: <short title>`) if it doesn't exist; otherwise add to it. Use the subsections in this order: Breaking, Added, Changed, Fixed, Removed, Internal. Put the commit hashes on a `Commits:` line once they exist.
- Describe what the change actually does, from the diff and the tests, not the commit message. Write it for a Quark user, with a short code example where it helps; compiler and runtime details go under Internal.
- Breaking means anything that makes a program that worked before fail to compile, fail at runtime, or print something different. Name each removed or renamed thing and what to use instead.
- Bugs that aren't fixed go in `known_issues.md`, not here. When a change fixes one, delete its entry there and describe the fix under Fixed here.
- If the user commits without you, check `git log` for commits since the last changelog entry and add them before starting new work.

## Docs (keep them current)

Each doc owns one topic; others link to it instead of repeating it:

| Doc | Owns |
|---|---|
| `README.md` | Onboarding: what Quark is, quickstart, language tour, CLI, troubleshooting |
| `grammar.md` | Syntax, keywords, precedence |
| `semantics.md` | Language behaviour (values, types, errors, modules) |
| `stdlib.md` | Prelude builtins and `std/` modules (`builtins/catalog.go` and `src/core/stdlib/` are the source of truth) |
| `architecture.md` | Compiler and runtime internals, the soundness contract (§12), the back-end roadmap (§13), performance findings from the benchmarks (§15) |
| `error_codes.md` | Every `QK-*` and `INV-*` code |
| `*_spec.md` | One feature's design and decisions; a status line at the top says what is implemented |
| `changelog.md` | History: what changed, and when |
| `known_issues.md` | Bugs not fixed yet, each with a reproduction, and how to add one (Robustness Bar, rules 3, 4, 7 and 8) |
| `src/benchmarks/README.md` | The benchmark suite against Python and R: what each benchmark measures, the fairness rules, how to run and extend it. `src/benchmarks/RESULTS.md` holds the latest measurements |

Rules: docs describe the current state only. No "used to", no superseded designs, no finished to-do lists; history is in `changelog.md` and git. Every doc starts with a table of contents. A change that alters behavior updates the doc that owns it in the same change. When a decision supersedes part of a spec, rewrite that part; don't add an override section on top of it.

Quark is general-purpose for data work and not tied to any domain. Examples the user gives in conversation (a field they work in, typical data sizes, their own tools or workflow) are context for you, not content: don't put them in user-facing docs, and don't present them as Quark's target use case.

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

# Benchmarks against Python and R (src/benchmarks/README.md)
cd src/benchmarks && python3 bench.py setup && python3 bench.py run --preset small
```

## Prerequisites

- Go 1.21+
- clang++ (C++17) — **required**, g++ is not supported
  - Ubuntu/Debian: `sudo apt install clang`
  - macOS: `brew install llvm`
- CMake (only on platforms without a prebuilt Boehm GC; linux-amd64, linux-arm64, darwin-arm64 and windows-amd64 have one)

## Compiler Pipeline

```
.qrk source → Lexer → Parser → Loader → Analyzer → Invariants → Lower → Verify → Emit → C++ → clang++ → binary
```

All stages live under `src/core/quark/`:

| Stage | Package | Purpose |
|-------|---------|---------|
| Lexer | `lexer/` | Indentation-aware tokenization (emits INDENT/DEDENT tokens) |
| Parser | `parser/` | Recursive descent → AST (`ast/ast.go` defines `TreeNode`) |
| Loader | `loader/` | Loads every `incl`uded file once (`files.go`: path and file-name checks, definitions-only rule for imported files, per-file symbol tables and clash rules), then resolves every bare and `::` name per file to a unique internal name and splices the files into one tree (`resolve.go`). Emits a `names.Table` (internal → as written) used for every user-facing name |
| Analyzer | `types/` | Semantic analysis, scope/symbol tables, closure capture computation, generates `CallPlan` IR |
| Invariants | `invariants/` | Pre-lowering validation (call plans, return types) |
| Lower | `lower/` | Analyzed AST → typed IR (`ir.go`): every evaluation step, temporary, conversion, check and fast path is decided here (`lower.go`, `expr.go`, `calls.go`, `store.go`); `verify.go` checks the IR on every compile (`INV-IR-*`) |
| Emit | `emit/` | Prints the IR as C++17; makes no decisions |
| Driver | `driver/` | Runs the stages above in order (`CompileFile(path, stage)`); used by every CLI command except `lex`. `internal/testutil` runs in-memory sources through `driver.CompileSource` (its `Lex`/`Parse` helpers stop before the loader) |
| Toolchain | `toolchain/` | clang++/runtime/GC discovery, compile and link steps, precompiled runtime header cache |
| Entry | `main.go` | CLI dispatch and flag parsing |

## Runtime System

Header-only C++17 library at `src/core/quark/runtime/include/quark/`. Key files:

- `core/value.hpp` — `QValue` tagged union (the universal value type)
- `types/closure.hpp` — `QClosure` struct (all function values are closures)
- `core/gc.hpp` — GC allocation wrappers (`q_malloc`, `q_malloc_atomic`, `q_strdup`)
- `ops/` — Arithmetic, comparison, logical, member access operators
- `builtins/` — prelude builtins (console I/O, string, math, list, dict, conversion); each header must be listed in `quark.hpp`
- `builtins/repr.hpp` — how values print (`print`/`to_str`: contents, 100-item elision); `core/format.hpp` — float text (shortest round-trip)
- `core/sum.hpp` — pairwise float summation, shared by the vec reductions, `std/stats` and the table aggregates so they give the same bits (no `long double`)
- `types/resource.hpp` — generational registry for native handles; every handle has the single Quark type `resource` (the kind, e.g. `file`, is runtime-only)
- `table/engine.hpp`, `table/csv.hpp`, `table/query.hpp` — the eager columnar table engine (gather, sort, group, aggregates), the CSV/TSV reader/writer (`tables_v0_1_spec.md` §7-8), and what generated query code and the table methods call. Not included by `quark.hpp`: a program includes `table/query.hpp` when it has a query or calls a builtin whose catalog `Spec.Header` names it (passed on as `CallPlan.RuntimeHeader`), and `std/csv`'s glue includes `table/csv.hpp`

The runtime is tested through the Go integration tests, some of which compile C++ programs against the headers: `TestVectorArrowLayout`, and `TestTableEngine`, which builds and runs the C++ test programs in `src/testfiles/table/` (`*_test.cpp`, sharing `check.hpp`). C++ test changes aren't seen by Go's test cache: run with `-count=1`.

## Naming Conventions (Critical)

| Prefix | Domain | Examples |
|--------|--------|---------|
| `q_` | Runtime builtins/helpers (C++ runtime code) | `q_add()`, `q_print()`, `q_malloc()` |
| `quark_` | User-defined names in generated C++ (back-end output); `extern fn` symbols keep their C++ name and get `_thunk_<symbol>` wrappers | `quark_main()`, `quark_x` |
| `qv_` | Value constructors | `qv_int()`, `qv_string()`, `qv_null()` |
| `QValue`, `QList`, `QClosure` | Type names (PascalCase) | — |

**Never mix `q_` and `quark_` prefixes** — `q_` is runtime-only, `quark_` is for generated code only.

**Internal names (loader).** Definitions of imported file N (stem `s`) become `q<N>_s__name`, members of its modules `q<N>_s__module__name`; main-script names are unchanged and its module members become `module__name`. A local of an imported file (parameter, loop variable, `when` binding, name assigned in a function) named like any main-script name becomes `q<N>__name__l`, because the spliced tree has one global scope where main's names are bare. Two definitions that would get the same internal name (`m` + `_x` and `m_` + `x`) are a load error, never merged. A file's identity is its real path (symlinks resolved). The back end prefixes them with `quark_` as usual. Show users `names.Table.Display(internal)` / `DisplayText(msg)` (`geo::Point`, `fmt::show_vec`), never an internal name: the analyzer applies `DisplayText` to every diagnostic, lowering to `QFnInfo` and struct display names.

## Key Architecture Details

- **All values are `QValue`** — a tagged union with type field. Runtime ops must check the tag before reading a union field, unless the analyzer proved the type (architecture.md §12). A value the op can't handle is a runtime error (`q_runtime_reportf`, then `exit(1)`), never a `qv_null()` stand-in; `null` is returned only where it is the documented result (`list.get` past the end, `dict.get` of a missing key).
- **All function values are `QClosure*`** — even non-capturing functions. Generated functions take `QClosure* _cl` as hidden first parameter. `fn f` parses as a `FnDeclNode` `[f, lambda]` (`ast.IsFnDecl`): a fixed, hoisted declaration (`types/analyzer_fixed.go`). Once analysis is done (`finalizeFixedFns`), a declaration that captures nothing (directly or through another declaration) is a plain C++ function `quark_<CName>`, called with `nullptr` (`DispatchDirect`, or `DispatchNative` when fully annotated) and never captured; one that captures stays a closure in a fixed binding, called through `q_callN`. Lowering creates every scope's closures at the scope's start (`hoistScopes`); variables they capture start as unassigned cells (`Symbol.Hoisted`), read with `q_cell_get` inside those closures. Plain `g = fn(...)` variables are always closures.
- **Variable storage is decided once, by the analyzer**: every variable, parameter, loop variable, `when` binding and `fn` declaration is one `types.Symbol`, and `Analysis.Refs` maps every resolved identifier node to its symbol. `finalizeSymbols` picks its `ir.Storage` — `QCell*` (captured), `long long` / `double` / `bool` (its `StoreType` is `int` / `float` / `bool`), `QValue` (everything else) — and its C++ name (`CName`). Lowering (`lower/store.go`) declares the variable at the symbol's `Decl` node (`DeclVar`), converts every stored value to the `StoreType` (int widens to float; unproven values are checked with `q_param_*` / `q_var_*`), and keeps no scope state of its own. Every IR expression has a representation (`lower.Rep`: `QValue`, raw scalar or native type) and conversions are explicit nodes (`Box`, `Unbox`, `Cast`, `Adapt`, `Check`). Generated C++ names start with `quark___` (`ir/storage.go`) so they can't collide with user names.
- **Evaluation order** (`semantics.md` §4.5): lowering an expression appends the statements it needs and returns a safe expression; operands run left to right (receiver before arguments), an operand becomes a temporary only when a later one needs statements or has effects (`operands`/`sequence`), and `and`/`or`/ternary/`elseif`/`while` conditions get their own blocks. Never emit a statement for an expression outside the block that evaluates it.
- **Operator lowering**: arithmetic, unary `-` and comparisons whose operands both have a raw scalar representation (literals, scalar-stored locals, native results, raw sub-expressions) are raw C++ (`Arith`, `Neg`, `Compare`); `and`/`or` of two raw bools are `&&`/`||`. Int `+ - * %` use overflow-checked `q_checked_add/sub/mul/mod`, `/` uses `q_checked_fdiv` (always `double`), floats use raw C++ operators; `**` is never lowered (`q_pow`). The result stays unboxed when assigned to a scalar-tier variable or used as a condition, otherwise it's boxed to `QValue`.
- **Conditions** (`if`/`while`/`and`/`or`/`!`) use `q_truthy()` truthiness rules (there is no `not` keyword); `and`/`or` are short-circuit lambdas that return an operand.
- **Memory** — Boehm GC vendored at `deps/bdwgc/`: only the public headers, prebuilt static libraries in `lib/<GOOS>-<GOARCH>/`, `build-prebuilt.sh` (rebuilds them), `VENDORED.md` and LICENSE are committed; the GC source is gitignored. `toolchain.EnsureGC` uses the prebuilt library, else a local `build-static/` CMake build (see `deps/bdwgc/VENDORED.md`). Use `q_malloc_atomic()` for data without pointers (strings, numeric buffers). The collector never runs destructors and doesn't scan `malloc`'d memory, so a `std::string`/`std::vector`/map stored in (or reachable only from) runtime data must use `q_allocator` (dict keys and the resource registry do); otherwise it leaks or its contents get collected while live. `integration_gc_test.go` (programs in `src/testfiles/gc/`) checks for both by comparing GC heap and peak RSS across repeated rounds.
- **Element types (architecture.md §12)** — a list's element type (a dict's value type) is fixed when it is created; an empty `list []`/`dict {}` is pending until its first write or the typed slot it is stored into (`types.SettleContainers`, `FixElementType`). The analyzer checks every write it can see (`checkElemWrite`); at runtime every `QList`/`QDict` carries a `const QElemType*` (`types/elemtype.hpp`, `nullptr` = any) and every write goes through `q_elem_store`, so element reads can stay raw. New list-creating builtins must pass the element type their catalog return type claims (`qv_list(n, &q_elem_int)`), and new write paths must call `q_elem_store`. Containers nest by subtype without conversion (`types.IsSubtype`: a `list[int]` is not a `list[float]`, a `list[any]` is not a `list[int]`). Variables never narrow, and return annotations are checked on every function (`checkedReturn`, `q_ret_*`).
- **Analyzer → back-end handoff** — the analyzer returns one `types.Analysis` struct (captures, call plans, node types, native/extern fns, struct types, ...), passed to `lower.Lower`. New analyzer outputs go in that struct, not new setters.
- **Function values** — every closure points to a static `QFnInfo` (`types/closure.hpp`; lowering's `fnInfos` emits `_qfi_<name>`, plus `quark_<name>__default_arg` for functions with defaults). `q_callN` (`types/function.hpp`) calls directly when the argument count equals the parameter count, otherwise `q_call_adapt` checks the range and fills defaults; `qext::call` goes through the same path. New closure-creation sites must pass an info (`q_alloc_closure(fn, info, n)`, `qv_func(fn, info)`).
- **Call shapes** (`types/analyzer_callshapes.go`) — the compile-time half: the analyzer records how a body calls its parameters (`FunctionType.ParamCalls`, propagated through parameters passed on to other functions) and checks function arguments with known signatures against them (arity, argument types, non-functions). Any new call path in the analyzer must call `recordParamCall` and `checkCallbackArgs` like the existing direct, module and pipe paths.
- **CallPlan** (`ir/call.go`) — IR metadata attached to each call site by the analyzer. Tracks call kind (builtin vs user), arity, default arg filling. Lowering reads these instead of re-analyzing.
- **Builtin catalog** (`builtins/`) — shared metadata (name, arity, signatures) used by the analyzer and `invariants/`; the back end never reads it and emits `CallPlan.RuntimeSymbol` instead. Methods are indexed by `(ReceiverType, methodName)` pair, separate from free functions.
- **Stdlib modules** (`src/core/stdlib/`) — each is a `.qrk` file with a primary `module X:` block named like the file (so its members are `X::name`), and most bind native code from a sibling `.hpp` via `extern 'x.hpp'` + `extern fn` (`io`, `sys`, `time`, `random`, `csv`, `fmt`, `blas`). Functionality that isn't needed in the prelude belongs here, not in the catalog; file I/O, for example, is `std/io` only, and there are no `_file_*` builtins.
- **Native libraries (FFI)** — `extern link 'name'` / `extern include_path 'dir'` are collected by the loader into `driver.Unit.Native` (link order follows the import graph); `toolchain/native.go` resolves libraries before linking (`QK-FFI-002`), adds `-L`/rpath/`-l` args, and maps C++ errors in glue headers to `QK-FFI-003`. `integration_ffi_test.go` builds the C test libraries in `src/testfiles/ffi/` at test time; `TestStdBlas` skips when OpenBLAS isn't installed. Spec: `ffi_v0_1_spec.md`, which is canonical for all native interop.
- **Modules (v0.2)** — `modules_v0_2_spec.md`. The analyzer never sees `::` or `incl`: the loader rewrites qualified names to identifiers and drops `incl`. Module bodies are analyzed in the file scope (their members have unique names) and their declarations are hoisted with the top level. Imported constants are inlined at each use. Struct identity is the definition: `q_param_struct` / `q_field_struct` compare `QStructDef` pointers, and `QStructDef.name` holds the display name.
- **Method dispatch** — When a CallPlan has `IsMethod=true`, the receiver is the first runtime argument and is evaluated before the other arguments (e.g., `"hello".upper()` becomes `q_upper(receiver_val)`). When the receiver's type is unknown at compile time (`any`, e.g. from `unwrap`), the plan is `DispatchDynamicMethod` and lowering tests the runtime type tag over `MethodCandidates` (an `if` chain ending in `q_no_method`).

## Adding a New Builtin

1. Add a `Spec` entry in `builtins/catalog.go` (set `ReceiverType` for methods, leave empty for free functions)
2. Implement the C++ function in `runtime/include/quark/builtins/*.hpp` with `q_` prefix (a new header must also be added to the include list in `runtime/include/quark/quark.hpp`)
3. Analyzer and back end pick it up automatically via the catalog (the back end through the CallPlan's `RuntimeSymbol`)
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
- Pipe operator: `x | foo() | bar()` (parsed as `bar(foo(x))`; `x | obj.m(a)` is `obj.m(x, a)`)
- Result types: `ok(val)` / `err(val)` with `unwrap()`
- Type annotations: `fn add(x: int, y: int) int -> x + y`. Each annotated parameter is enforced on its own (partial annotation counts): checked at compile time when the argument type is known, on function entry otherwise (`paramBindings` → `q_param_*` in `builtins/params.hpp`), with int→float widening; same rules as `extern fn`. Fully annotated, non-capturing functions compile to native C++ (`DispatchNative`)
- Default params: `fn foo(x: int, y: int = 0) -> x + y` (literals, or an empty `list []` / `dict {}`)
- String type is `str` (not `string`)
- Annotation types: `int`, `float`, `str`, `bool`, `any`, `list`, `dict`, `vec`, `result`, `resource`, `fn` (`types.TypeFn`: any function value, no signature; `QClosure*` in native/extern signatures), struct names, and tables: `tbl[R]` (`types.TableType{Row}`, rows of struct `R`) and a bare `tbl` (any row; `QTable*` in native signatures). A type argument on anything but `tbl` is an error (`list[int]`); `xs: list = ...` holds any element type. `null` is accepted only by struct and `resource` annotations (and `any`)
- Tables (`tables_v0_1_spec.md`, semantics.md §9.5): queries `t | e -> filter(...) | mutate(...) | group_by(...) | summarize(...)` are checked in `types/analyzer_query.go` (row type through every step; derived rows are anonymous `StructType`s, `row__N`) and lowered in `lower/query.go` (typed loops over column buffers). A table with an anonymous row matches a named row by shape where it is stored (`RowMatch`, `q_*_table` checks relabel it); std/csv's reads are typed by the expected type (`Analyzer.expected`, `TableReads`)
