# Quark Error Codes

The registry of every error code the compiler and runtime emit. `QK-*` codes are for problems in the user's program; `INV-*` codes are compiler bugs.

## Contents

- [Message format](#message-format)
- [QK codes](#qk-codes)
- [Runtime errors](#runtime-errors)
- [Internal invariant codes (INV-*)](#internal-invariant-codes-inv-)
  - [Invariant validation](#invariant-validation)
  - [Lowering](#lowering)
  - [IR verification](#ir-verification)
- [Planned codes](#planned-codes)
- [Rules for new codes](#rules-for-new-codes)

## Message format

Diagnostics print as

```
error[CODE] (stage): message at file:line:col
warning[CODE] (stage): message at file:line:col
  note: ...
```

The location is `file:line` when there is no column, and is left out when unknown. Notes follow on their own lines. Every name in a message is shown as written in the source (`geo::Point`), never as a compiler-internal name.

Stages: `parse`, `load`, `check`, `build` (clang++ compile and link), `runtime`. Errors stop `check`, `emit`, `build` and `run`; warnings are printed and don't.

## QK codes

| Code | Stage | Meaning | Emitted from |
| --- | --- | --- | --- |
| `QK-PARSE-001` | `parse` | Syntax error. Also covers lexer errors, reported with the lexer's message: `unexpected indent`, `expected indented block`, `inconsistent indentation`, `unexpected character '#'`, `'\0' is not supported in string literals`. A bad character inside an expression is reported by the parser instead (`expected RPAR but got ILLEGAL`). Also a pipe target that isn't a call, and anything after an `extern` directive's string. | `src/core/quark/parser/parser.go` |
| `QK-LOAD-001` | `load` | Import and name resolution (modules v0.2): a file that can't be found, an unsupported import path, an import cycle; an imported file or directory name that isn't `[a-z][a-z0-9_]*`; a statement (anything but a definition or constant) in an imported file; a name defined twice at the top level of a file, a module member named like a top-level name, an import binding named like a definition or another binding, two names that would get the same internal name; a name containing `__`; an unknown or malformed qualified name (`'fmt' (std/fmt) has no 'show_vector'; did you mean 'show_vec'?`, a re-export like `geo::io::open`); `.` on an import binding or module (`'io' is an imported file; use io::exists`); assigning an imported name or constant; a bare name in an imported file that the file doesn't define; `incl` of a same-file module or `incl` outside the top level; an `extern link` / `extern include_path` inside a block or module; an extern method declared twice for one receiver in a file, or named like a prelude method of any type; a `QUARK_STDLIB_ROOT` that isn't a directory, or no stdlib next to the compiler. | `src/core/quark/loader/files.go`, `loader.go`, `resolve.go` |
| `QK-CHECK-001` | `check` | Semantic or type error found by the analyzer: wrong argument types or counts, unknown methods, wrong-typed writes into lists and dicts, `null` where an annotation rejects it, call-shape mismatches of callbacks, return annotations, redefined builtins, duplicate or reassigned `fn` declarations in a nested scope, and so on. | `src/core/quark/types/analyzer.go` |
| `QK-CHECK-002` | `check` | Warning (does not stop compilation). Currently emitted for one case: the two branches of `x if c else y` have incompatible types (`ternary branches have incompatible types: 'int' and 'str'`). | `src/core/quark/types/analyzer.go` (`warnAt`), called from `analyzer_controlflow.go` |
| `QK-RUNTIME-001` | `runtime` | Any runtime failure; see [Runtime errors](#runtime-errors). | `src/core/quark/runtime/include/quark/core/diagnostics.hpp` |
| `QK-FFI-001` | `load` | An `extern include_path` directory that doesn't exist, or a `std/` include path when no stdlib directory can be found. | `src/core/quark/loader/loader.go` |
| `QK-FFI-002` | `build` | An `extern link` library that can't be found. Notes list the file names tried and the directories searched. | `src/core/quark/toolchain/native.go` |
| `QK-FFI-003` | `build` | A C++ compile error inside an extern glue header or a file it includes (for example a missing `#include`). Names the glue file and line; clang's full output is printed above it. | `src/core/quark/toolchain/native.go` |

A C++ compile or link error outside glue code has no code: `quark` prints clang's output and `Compilation failed: ...`. It means a wrong `extern fn` symbol or header, or a compiler bug.

## Runtime errors

All runtime errors use `QK-RUNTIME-001` and exit with status 1. They go through `q_runtime_reportf` (`core/diagnostics.hpp`):

```
error[QK-RUNTIME-001] (runtime): modulo by zero at file.qrk:3:10
```

- The location is the statement that failed. After a call returns, the caller's statement is current again (every generated function restores it), so an error later in the same statement points there, not into the callee.
- Code from an imported user file reports that file (`lib/helpers.qrk:3:11`). Code in `std/` modules sets no location, so an error raised inside the stdlib (usually a bad argument) points at the user's call.
- An annotated parameter that receives a value of the wrong type is reported at the call: `argument 'x' of 'f' expects int, got str`. A fully annotated function compiled to native C++ (most stdlib module functions) reports it as `expected int, got float`, without the parameter name.
- Errors raised by native extension code through `qext` start with `extension error:` (`extension error: blas.dot: x must be an f64 vec, got i64; ...`).

The code is deliberately coarse for now; the message carries the detail.

## Internal invariant codes (INV-*)

These mean the compiler violated one of its own invariants. They are compiler bugs, never errors in the user's program.

### Invariant validation

Run between the analyzer and lowering (`src/core/quark/invariants/callplan.go`). Printed as `error[INV-...] (invariants): message at line N, col M`.

| Code | Checks |
| --- | --- |
| `INV-CALLPLAN-MAP` | The analyzer produced a call-plan map |
| `INV-CALLPLAN-MISSING` | Every call node has a call plan |
| `INV-ARITY-ENVELOPE` | A plan's min/max arity is valid |
| `INV-DISPATCH-MODE` | A plan's dispatch mode is resolved |
| `INV-RUNTIME-SYMBOL` | Plans that need a runtime symbol have one |
| `INV-DEFAULT-COUNT` | Planned default arguments fit the missing argument slots |
| `INV-BUILTIN-NAME` | A builtin plan names its callee |
| `INV-METHOD-RECEIVER` | A method plan has a receiver type |
| `INV-BUILTIN-CATALOG` | The builtin or method is in the catalog |
| `INV-BUILTIN-RUNTIME` | The plan's runtime symbol matches the catalog's |
| `INV-DYNAMIC-METHOD` | A dynamic method plan has a receiver and valid candidates |
| `INV-RETURN-MAP` | The analyzer produced a return-validation map |
| `INV-RETURN-VALIDATE` | Every annotated return type was validated |

### Lowering

Raised while lowering the analyzed tree to the IR (`src/core/quark/lower/`). Printed as `internal compiler error [INV-...]: message at line N`; no C++ is emitted.

| Code | Invariant | Raised in |
|---|---|---|
| `INV-NODE-TYPE` | An AST node type has a lowering | `lower/expr.go` |
| `INV-OPERATOR` | A binary operator has a lowering | `lower/expr.go` |
| `INV-ASSIGN-TARGET` | An assignment target kind has a lowering | `lower/expr.go` |
| `INV-FIXED-FN` | A `fn` declaration was analyzed | `lower/expr.go` |
| `INV-LAMBDA` | Every lambda was collected as a function | `lower/expr.go` |
| `INV-CALLPLAN-MISSING` | A call has a call plan | `lower/calls.go` |
| `INV-CALLPLAN-RUNTIME` | A builtin, direct, native or extern call plan has a runtime symbol, and a dynamic method call a receiver | `lower/calls.go` |
| `INV-CALLPLAN-DISPATCH` | A call plan's dispatch kind is known | `lower/calls.go` |
| `INV-DOT-CALL` | No dot-call node reaches lowering as a function value call | `lower/calls.go` |
| `INV-STRUCT-LITERAL` | A struct literal names a known struct | `lower/expr.go` |
| `INV-STRUCT-DEFAULT` | A struct field default is well formed | `lower/expr.go` |
| `INV-SYMBOL` | An identifier has an analyzer symbol | `lower/expr.go` |
| `INV-STORAGE` | A scalar conversion is only requested for scalar storage | `lower/store.go` |
| `INV-QUERY` | A query was analyzed, and each of its steps is a known verb | `lower/query.go` |
| `INV-QUERY-REP` | A verb expression's value has a scalar or `QValue` representation that converts to its column's | `lower/query.go` |
| `INV-QUERY-COLUMN` | A column read (`e.col`) is lowered inside the row loop that binds the column | `lower/query.go` |

### IR verification

Run on every compile after lowering (`src/core/quark/lower/verify.go`, `architecture.md` §1.6). Printed as `internal compiler error [INV-IR-...]: in <function>: message`; no C++ is emitted.

| Code | Invariant |
|---|---|
| `INV-IR-SCOPE` | Every variable, temporary and parameter is used inside the scope that declares it, after its declaration, and declared once per scope; a closure captures cells in scope |
| `INV-IR-REP` | Every value has the representation its use needs: a raw bool for a condition, a `QValue` for `q_truthy`, `Unbox`, `Adapt` and `Check`, the storage's type for a variable, the function's return type for `return`, a list or `QValue` for the list a loop checks; captured variables are cells |
| `INV-IR-UNBOX` | Every `Unbox` (a raw read of a `QValue`'s union field) says why the tag is known, and the reason fits: an analyzer type that is exactly the scalar read, or a loop's element read (`architecture.md` §12) |
| `INV-IR-LOOP` | `break` and `continue` are inside a loop; a loop's length, range bounds and the list whose length it checks are temporaries or constants, since they are re-evaluated each iteration |
| `INV-IR-RETURN` | `return` is inside a function |
| `INV-IR-CALL` | A call of a generated function passes one argument per parameter, of the parameter's type; `q_callN` gets N arguments; a function value names a generated function |
| `INV-IR-NODE` | Every node is known; a closure is bound by a `Let`; a tag test reads a stable value; a source location is an entry of the location table |

## Planned codes

- `QK-FFI-004` (runtime): wrong handle kind or use of a closed handle (`specs/ffi_v0_1_spec.md` P8). Today these are `QK-RUNTIME-001`.

## Rules for new codes

- User-visible errors get `QK-<STAGE>-NNN`. A new code needs a row in the table above, with its stage and source file.
- Compiler-bug assertions get `INV-<NAME>` and a row in the matching INV table.
- Never reuse a code for a different meaning.
- When a code stops being emitted, remove its row.
