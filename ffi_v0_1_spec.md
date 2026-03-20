# Quark FFI v0.1 Spec (Draft)

Status: Draft
Last Updated: 2026-03-15
Scope: Design/specification only (no implementation in this document)

## 1. Purpose

Define a minimal but scalable Foreign Function Interface (FFI) for Quark so developers can call native libraries (especially performance-focused C/C++ libraries) from Quark programs.

This spec prioritizes:

1. Practical developer experience at the Quark level.
2. Stable integration boundaries that survive real-world toolchain differences.
3. Data-heavy performance use cases.
4. Low implementation complexity for v0.1.

## 2. Design Constraints

### 2.1 Product constraints

The FFI must be:

1. Useful enough for real workloads and package ecosystems.
2. Simple enough to implement/maintain with current team bandwidth.
3. Consistent with Quark's existing compile-to-C++ architecture.

### 2.2 Engineering constraints

1. Avoid compiler research-heavy features for v0.1.
2. Avoid exposing raw C++ ABI details to Quark users.
3. Keep analyzer and codegen changes incremental and testable.

## 3. Core Concept: Stable Native Boundary

### 3.1 What this means

Even though Quark compiles to C++, v0.1 FFI should target a stable native function boundary rather than direct C++ class/template interop.

Reason:

1. Direct C++ ABI is fragile across compilers/platforms/stdlib implementations.
2. Stable native signatures reduce integration breakage.
3. Existing C++ libraries can still be consumed through thin wrappers.

### 3.2 Developer-facing intent

Quark developers should mostly write:

1. FFI declarations in Quark.
2. Normal Quark modules wrapping raw FFI calls.
3. Batch-style APIs for performance-critical operations.

## 4. v0.1 Goals and Non-Goals

### 4.1 Goals

1. Declare and call native functions from Quark.
2. Type-check calls at compile time using Quark signatures.
3. Support primitives and strings.
4. Support opaque native handles for complex objects.
5. Support practical linking/loading configuration.
6. Enable wrapper-module pattern for package-like ergonomics.

### 4.2 Non-goals (v0.1)

1. No callbacks from native into Quark.
2. No direct passing of Quark list/dict/vector object internals.
3. No direct C++ template/class binding generation.
4. No async/event-loop interop model in core v0.1.
5. No cross-platform binary packaging manager in compiler core.

## 5. Quark-Side Syntax (Proposed)

### 5.1 FFI library declaration

```quark
ffi lib 'mathlib' as m:
    fn add(x: int, y: int) int
    fn hypot(x: float, y: float) float
    fn version() str
```

### 5.2 Usage

```quark
println(m.add(2, 3))
println(m.hypot(3.0, 4.0))
println(m.version())
```

### 5.3 Optional explicit symbol mapping (future-compatible)

```quark
ffi lib 'mathlib' as m:
    fn add(x: int, y: int) int as 'mathlib_add_i64'
```

If omitted, symbol defaults to function name.

## 6. Type Surface (v0.1)

### 6.1 Supported parameter/return types

1. `int`
2. `float`
3. `bool`
4. `str`
5. `null` / void-like return
6. `any` only for opaque handle values (see Section 7)

### 6.2 Explicitly unsupported in v0.1

1. Direct `list`/`dict`/`vector` value marshalling.
2. Passing function/lambda values to native code.
3. Generic typed interop structs/unions.

### 6.3 Marshalling matrix (required behavior)

The compiler/runtime boundary must implement deterministic conversions for every supported type pair.

| Quark type | Native boundary type (conceptual) | Arg marshal rule | Return marshal rule | Failure mode |
|---|---|---|---|---|
| `int` | 64-bit signed integer | require runtime int value | box back as int | runtime error on non-int |
| `float` | 64-bit float | accept int/float numeric values | box back as float | runtime error on non-numeric |
| `bool` | boolean | require runtime bool value | box back as bool | runtime error on non-bool |
| `str` | UTF-8 C string | pass read-only UTF-8 pointer (or copied buffer) | see ownership rules in Section 8 | runtime error on non-string |
| `null` | void / no value | no argument value expected | map void return to `null` | analyzer/runtime mismatch error |
| `any` (handle-only) | opaque handle token | pass opaque handle payload unchanged | box native handle token as opaque `any` | runtime error on invalid handle token |

v0.1 restriction:

1. No implicit coercion from `str`/`bool` to numeric types.
2. Numeric widening (`int` -> `float`) is allowed for `float` parameters.
3. Numeric narrowing (`float` -> `int`) is rejected by default for FFI calls.

### 6.4 Declaration-time validation rules

The analyzer must reject FFI declarations that:

1. Use unsupported types in params/return.
2. Reuse duplicate function names within one `ffi lib` block.
3. Reuse conflicting aliases in the same scope.
4. Declare impossible signatures (for example `null` as non-void parameter type).

## 7. Opaque Handles for Complex Native Objects

### 7.1 Motivation

Large C++ libraries (GUI, ML, databases, geometry, etc.) often expose complex class hierarchies. v0.1 supports them via opaque handles instead of raw type interop.

### 7.2 Pattern

```quark
ffi lib 'qt_string' as qts:
    fn string_new(text: str) any
    fn string_len(h: any) int
    fn string_to_str(h: any) str
    fn string_free(h: any) null
```

### 7.3 Wrapper module pattern

```quark
module qt_string:
    fn len(text: str) int ->
        h = qts.string_new(text)
        n = qts.string_len(h)
        qts.string_free(h)
        n
```

This keeps raw handle lifecycle out of most application code.

## 8. Memory Ownership Contract

### 8.1 Required explicit ownership rules

Every FFI function that returns/accepts memory-backed values must have documented ownership semantics.

Minimum conventions for v0.1:

1. `str` parameters are read-only inputs from Quark.
2. `str` returns are copied into Quark-owned memory unless explicitly marked with paired free function.
3. Handle creation/destruction must be explicit (`new/free`, `open/close`, etc.).

### 8.2 Policy

If ownership is ambiguous, reject declaration or emit a compile-time warning requiring explicit annotation (final choice may be implementation-dependent).

### 8.3 Ownership contract table (v0.1)

| Return kind | Default ownership policy | Required wrapper convention |
|---|---|---|
| scalar (`int/float/bool`) | value copy | none |
| `null` | no ownership | none |
| `str` | copied into Quark-owned memory before returning to user code | if not copied, wrapper must expose paired free function and declaration metadata |
| opaque handle (`any`) | owned by caller unless documented otherwise | must provide explicit destructor API (`*_free`, `*_close`, etc.) |

### 8.4 Handle lifecycle guidance

FFI wrapper APIs should follow one of these explicit lifecycle patterns:

1. `*_new` / `*_free`
2. `*_open` / `*_close`
3. `*_create` / `*_destroy`

These names are conventions, not language keywords, but they should be treated as package-level standards to make ownership auditable and toolable.

## 9. Error Handling Contract

### 9.1 v0.1 baseline

1. Signature mismatch and invalid declarations are analyzer errors.
2. Marshalling/type conversion failures are runtime errors with clear messages.
3. Native call failures default to runtime errors unless wrapper API encodes result semantics.

### 9.2 Recommended package-level style

Expose safe Quark APIs returning `ok/err` by wrapping low-level FFI calls:

```quark
fn safe_open(path: str) result ->
    h = native.open(path)
    if h == null:
        err 'open failed'
    else:
        ok h
```

## 10. Performance Model

### 10.1 What scales

1. Batch-oriented native calls.
2. Coarse-grained operations (column ops, matrix ops, transforms).
3. Handle-based stateful engines.

### 10.2 What does not scale

1. Per-element FFI round trips in tight loops.
2. Repeated conversion/copy of small values across boundary.

### 10.3 Guidance for data-heavy libraries

1. Expose bulk APIs (`transform`, `aggregate`, `fit`, `predict`).
2. Minimize boundary crossings.
3. Evolve toward low-copy vector/buffer interop in later versions.

## 11. Compiler Architecture Changes (Planned)

### 11.1 Lexer/token

Add minimal tokens for `ffi` and `lib` (and optional symbol alias keyword reuse if needed).

### 11.2 Parser/AST

Add:

1. `FfiLibNode` (library declaration block).
2. `FfiFunctionDeclNode` (name, params, return, optional native symbol).

### 11.3 Analyzer

1. Validate FFI declaration structure and allowed types.
2. Register FFI symbols in scope/module alias map.
3. Enforce arity/type checks at call sites.

### 11.4 IR / call planning

Extend call planning with FFI dispatch kind.

### 11.5 Codegen

1. Emit native declarations/stubs.
2. Emit marshalling for supported types.
3. Emit calls through FFI dispatch path.
4. Keep diagnostics explicit for unsupported marshalling paths.

Implementation detail expectations:

1. Generate one call shim per FFI function declaration.
2. Perform argument conversion/checks before native invocation.
3. Convert native return into `QValue` immediately after call.
4. Emit source-location-aware runtime error paths for failed marshalling.

### 11.6 Runtime helper layer (required)

Add a minimal runtime helper surface dedicated to FFI lowering:

1. Scalar extract helpers: `qffi_expect_int`, `qffi_expect_float`, `qffi_expect_bool`.
2. String extract helper: `qffi_expect_cstr` (or equivalent).
3. Boxing helpers: `qffi_box_int`, `qffi_box_float`, `qffi_box_bool`, `qffi_box_null`, `qffi_box_string`.
4. Handle helpers: `qffi_box_handle`, `qffi_expect_handle`.
5. Error helper: `qffi_type_error(function, arg_index, expected, got)`.

These helpers should centralize FFI diagnostics so all generated shims emit consistent error messages.

### 11.7 CLI/build integration

Add link/load controls (exact flag names TBD), for example:

1. library search paths
2. linked native libraries
3. runtime loader hints for shared libraries

Minimum v0.1 CLI surface (proposed):

1. `--ffi-lib <name>` (repeatable)
2. `--ffi-lib-path <path>` (repeatable)
3. `--ffi-include <path>` (repeatable, if compile-time headers are needed)
4. `--ffi-link-arg <arg>` (escape hatch)
5. `--ffi-runtime-path <path>` (runtime loader search path hints)

### 11.8 Build-mode behavior

For `quark build` / `quark run`, behavior should be deterministic:

1. Resolve declared FFI libs against CLI flags + package metadata.
2. Fail fast before final link if required libs are unresolved.
3. Emit actionable diagnostics (missing lib name, searched paths, platform).
4. Record linked FFI artifacts in debug output when `--debug` is enabled.

Shared library mode (where supported):

1. Link against import library/stub at build time.
2. Ensure runtime loader path configuration is explicit.

Static library mode:

1. Link archive at build time.
2. Prefer this mode for simpler distribution when legal/licensing constraints allow.

## 12. Packaging Model (v0.1 Practical)

### 12.1 Package shape

A native-backed Quark package should include:

1. Quark module API (friendly wrapper layer).
2. Native library binary or build recipe.
3. FFI declaration files.
4. Basic metadata (name, version, platform notes, required libs).

### 12.1.1 Proposed metadata keys (minimum)

```text
name
version
abi_version
platforms
ffi_libs
ffi_lib_paths
ffi_runtime_paths
link_mode            # static | shared | auto
required_compiler    # optional constraints
notes
```

Optional per-symbol metadata:

1. native symbol override
2. ownership annotation (`caller_owns`, `callee_owns`, `borrowed`)
3. paired free function for returned string/buffer/handle types

### 12.2 Compatibility expectations

1. Package authors own native compatibility matrix.
2. Quark core provides stable FFI surface and diagnostics.
3. Versioning policy should include ABI/API version notes in package metadata.

### 12.3 ABI versioning policy (recommended)

1. Every native-backed package declares an `abi_version` integer.
2. Backward-incompatible native signature changes require ABI version bump.
3. Quark build should warn or fail on ABI mismatch depending on strictness mode.

## 13. Security and Safety

1. FFI executes native code with full process privileges.
2. Quark runtime safety guarantees do not sandbox native libraries.
3. Documentation must clearly state trust model for third-party native packages.

## 14. Test Strategy

### 14.1 Compiler tests

1. Parse/analyzer tests for valid/invalid FFI declarations.
2. Call-plan/invariant tests for FFI dispatch.
3. Negative tests for unsupported type usage.

### 14.2 End-to-end tests

1. Tiny native test library with int/float/bool/str functions.
2. Handle lifecycle test (create/use/free).
3. Error-path tests for missing symbol/library.
4. Marshalling-failure tests (wrong runtime types for each supported arg type).
5. Ownership smoke tests (returned string/handle cleanup path).

### 14.3 Performance sanity tests

1. Microbenchmarks comparing boundary crossing overhead.
2. Batch-call benchmark to validate practical throughput pattern.

## 15. Phased Rollout

### Phase A (v0.1 core)

1. FFI declarations in Quark.
2. Primitive + string + handle calls.
3. Static or explicit shared linking support.
4. Basic diagnostics and tests.

### Phase B (v0.2 candidate)

1. Better ergonomics for symbol mapping and ownership annotations.
2. Improved packaging metadata and linker UX.
3. Expanded diagnostics and portability checks.

### Phase C (future)

1. Low-copy vector/buffer interop.
2. Optional callback model.
3. Advanced package tooling.

## 16. Open Decisions

1. Exact concrete syntax keywords and grammar shape.
2. Final ownership annotation mechanism for returned strings/buffers.
3. Final CLI flag names for native linking/loading.
4. Whether `any` is sufficient for handles in v0.1 or if a dedicated `handle` type keyword is preferable.
5. Whether ownership annotation should be language syntax or package metadata only in v0.1.

## 17. Recommendation

Proceed with FFI v0.1 using a stable native boundary, opaque handle support, and wrapper-module ergonomics. This path is both realistically implementable and extensible to real-world, performance-focused native library ecosystems.
