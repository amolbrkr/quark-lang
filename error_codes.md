# Quark Error Codes

This file is the canonical registry of error codes currently emitted by the compiler/runtime.

## Format

- User-facing diagnostics use: `error[CODE] (stage): message ...`
- Frontend diagnostics use stable `QK-*` codes.
- Runtime diagnostics use stable `QK-*` codes.
- Internal invariant/ICE failures use `INV-*` codes (compiler-bug class, not user-program errors).

## Stable QK Codes

| Code | Stage | Meaning | Emitted from |
| --- | --- | --- | --- |
| `QK-PARSE-001` | `parse` | Parse error in source program. | `src/core/quark/parser/parser.go` |
| `QK-LOAD-001` | `load` | Import/module resolution failure. | `src/core/quark/loader/loader.go` |
| `QK-CHECK-001` | `check` | Semantic/type analysis error. | `src/core/quark/types/analyzer.go` |
| `QK-CHECK-ICE-001` | `check` | Analyzer internal compiler error (scope underflow). | `src/core/quark/types/analyzer.go` |
| `QK-RUNTIME-001` | `runtime` | Runtime failure emitted by runtime diagnostics reporter. | `src/core/quark/runtime/include/quark/core/diagnostics.hpp` |

Notes:
- `QK-RUNTIME-001` currently covers multiple runtime failure causes and is intentionally coarse for now.
- `QK-CHECK-ICE-001` indicates a compiler bug/invariant break, not a user code mistake.

## Internal Invariant Codes (INV-*)

These indicate compiler invariant violations. They are intentionally separate from user-facing `QK-*` program diagnostics.

### Codegen invariant codes

- `INV-NODE-TYPE`
- `INV-CALLPLAN-MISSING`
- `INV-DOT-CALL`
- `INV-CALLPLAN-RUNTIME`
- `INV-CALLPLAN-DISPATCH`

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
