# Quark Error Codes

This file is the canonical registry of error codes currently emitted by the compiler/runtime.

## Format

- User-facing diagnostics use:
	- `error[CODE] (stage): message ...`
	- `warning[CODE] (stage): message ...`
- Frontend diagnostics use stable `QK-*` codes.
- Runtime diagnostics use stable `QK-*` codes. They go through `q_runtime_reportf` in `diagnostics.hpp`, which prints `error[QK-RUNTIME-001] (runtime): message at file:line:col`.
- The runtime location is the statement that failed. After a call returns, the caller's statement is current again (every generated function restores it on return), so an error later in the same statement points there, not into the callee. Code from an imported user module reports that module's file (`lib/helpers.qrk:3:11`); code from `std/` modules sets no location, so an error raised inside the stdlib (usually a bad argument) points at the user's call. An annotated parameter that receives a value of the wrong type is reported at the call: `argument 'x' of 'f' expects int, got str`.
- `QK-PARSE-001` also covers lexer errors, reported with the lexer's message: `unexpected indent`, `expected indented block`, `inconsistent indentation`, `unexpected character '#'`, and `'\0' is not supported in string literals`.
- Internal invariant/ICE failures use `INV-*` codes (compiler-bug class, not user-program errors).

## Stable QK Codes

| Code | Stage | Meaning | Emitted from |
| --- | --- | --- | --- |
| `QK-PARSE-001` | `parse` | Parse error in source program. | `src/core/quark/parser/parser.go` |
| `QK-LOAD-001` | `load` | Import/module resolution failure: file not found, imported file defines no `module`, unsupported import path, import cycle. Also `extern link` / `extern include_path` placed inside a block or module instead of at the top level of a file. | `src/core/quark/loader/loader.go` |
| `QK-CHECK-001` | `check` | Semantic/type analysis error. | `src/core/quark/types/analyzer.go` |
| `QK-CHECK-002` | `check` | Semantic analysis warning (non-fatal). | `src/core/quark/types/analyzer.go` |
| `QK-RUNTIME-001` | `runtime` | Runtime failure emitted by runtime diagnostics reporter. | `src/core/quark/runtime/include/quark/core/diagnostics.hpp` |
| `QK-FFI-001` | `load` | `extern include_path` directory not found. | `src/core/quark/loader/loader.go` |
| `QK-FFI-002` | `build` | `extern link` library not found; notes list the file names and directories searched. | `src/core/quark/toolchain/native.go` |
| `QK-FFI-003` | `build` | C++ compile error inside an extern glue header (or a file it includes); names the glue file. | `src/core/quark/toolchain/native.go` |

Notes:
- `QK-RUNTIME-001` currently covers multiple runtime failure causes and is intentionally coarse for now.
- Analyzer warnings (`QK-CHECK-002`) are displayed but do not fail compilation/execution commands.

## Internal Invariant Codes (INV-*)

These indicate compiler invariant violations. They are intentionally separate from user-facing `QK-*` program diagnostics.

### Codegen invariant codes

- `INV-NODE-TYPE`
- `INV-CALLPLAN-MISSING`
- `INV-DOT-CALL`
- `INV-CALLPLAN-RUNTIME`
- `INV-CALLPLAN-DISPATCH`
- `INV-STRUCT-LITERAL`
- `INV-STRUCT-DEFAULT`

Primary source: `src/core/quark/codegen/codegen.go`

### Invariant validation codes

- `INV-CALLPLAN-MAP`
- `INV-CALLPLAN-MISSING`
- `INV-ARITY-ENVELOPE`
- `INV-ARGCHECK`
- `INV-DISPATCH-MODE`
- `INV-RUNTIME-SYMBOL`
- `INV-BUILTIN-NAME`
- `INV-METHOD-RECEIVER`
- `INV-DYNAMIC-METHOD`
- `INV-BUILTIN-CATALOG`
- `INV-BUILTIN-RUNTIME`
- `INV-DEFAULT-COUNT`
- `INV-RETURN-MAP`
- `INV-RETURN-VALIDATE`

Primary source: `src/core/quark/invariants/callplan.go`

## Consistency Rules

- Add new user-visible compiler errors under `QK-<STAGE>-NNN`.
- Add new stage-internal ICE errors under `QK-<STAGE>-ICE-NNN`.
- Keep `INV-*` for invariant checks and compiler-bug assertions.
- Do not reuse a code for a different semantic meaning.
