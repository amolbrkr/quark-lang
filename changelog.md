# Changelog

Notable changes to the Quark language, compiler, runtime, stdlib and tooling.

Quark has no tagged releases yet, so entries are grouped by date and by piece of work, newest first. Each entry lists the commits it covers. Entries were written from the diffs, tests and smoke programs, not from the commit messages, which are often vague (`huge chunk of changes`, `minor QOL improvments`) or misleading (`fix returns inside if` fixes scalar assignment codegen; Quark has no `return`).

Sections follow [Keep a Changelog](https://keepachangelog.com/): **Breaking**, **Added**, **Changed**, **Fixed**, **Removed**, **Internal**. "Breaking" covers anything that can make a program that worked before fail to compile, fail at runtime, or print something different.

---

## Known issues (as of `ac274c0`)

These were checked against the current compiler. They are silent wrong results or silent name resolution, so they break the robustness rules in `CLAUDE.md`.

- **An int stored in a `float` struct field reads back as a garbage float.** The analyzer accepts int→float in a struct literal, but codegen stores the int unconverted, and the field read uses `.data.float_val`.
  ```
  struct P:
      x: float
  p = P { x: 1 }
  v = p.x
  println(v)          // 5e-324
  ```
  The same happens through `s: float = r.score` (`s * 1.5` prints `1.5e-323`).
- **An `any` value goes into a typed struct field without a check.** With `struct P: x: int` and `fn mk(a) -> P { x: a }`, `p = mk('hello')` stores a string. `p.x + 1` then prints a pointer-sized integer.
- **A user definition that shares a name with an imported top-level name is silently ignored.** For example, `fn fmt_full(x) -> 'mine'` after `use 'std/fmt' as fmt` still calls the extern `fmt_full`. Names inside `module` blocks are safe: they are namespaced, and a clash is a `QK-LOAD-001` error.
- `input()` is declared to return `str`, but it returns `null` at end of input. The mismatch fails loudly at runtime (for example `len()` or a `str` parameter check) rather than at compile time.
- Using a field on `o: O = null` fails with `expected struct, got different type`. The program does stop, but the message doesn't mention `null`.

---

## 2026-11-12: Function values, `fn` type, call-shape checking

Commits: `c0ac8cd`, `ac274c0`

### Breaking
- New compile errors. Programs that pass a callback with the wrong arity or argument types, or pass a non-function where the body calls the parameter, are now rejected (see *Call shapes* below). They used to compile and then misbehave at runtime.
- Native/extension C++ API:
  - `QClosure` has a new `const QFnInfo* info` field.
  - `q_alloc_closure(fn, info, n)` and `qv_func(fn, info)` take a new `QFnInfo*` argument.
  - `q_require_callable` was replaced by `q_require_closure`.
  - Hand-written glue that builds closures must be updated.
- A fully annotated (native) function now checks its result at runtime. For example, `fn nat(f: fn) int -> f(1)` fails with `expected int, got str` when the callback returns a string; it used to read garbage.

### Added
- **`fn` annotation type.** Means "any function value", with no signature. It works on parameters, return types, variables and struct fields, and accepts `null`.
  ```
  fn apply(g: fn, x: int) -> g(x)
  fn pick(up: bool) fn -> ...
  cb: fn = mx.inc
  struct Handler:
      run: fn
  ```
- **Module members as values**, for example `f = io.exists`, `list [mx.inc, io.exists]`, `apply(mx.inc, 2)`. An unknown member is a compile error: `module 'io' has no symbol 'nope'`.
- **`extern fn` callbacks.** An `fn` parameter or return maps to `QClosure*`. Glue code calls it with `qext::call` and can return it with the new `qext::box(QClosure*)`. Example: `extern fn apply_n(f: fn, n: int, x: any) any`.
- **Call shapes: compile-time checks on callbacks.** The analyzer records how a function body calls its parameters, and checks every function passed in against those calls:
  - **Arity:** `fn apply(k) -> k(1)`, then `apply(h)` where `h` takes 2 → `argument 1 of 'apply' is called with 1 argument (line 2), but 'h' takes 2 arguments`.
  - **Argument types:** `apply(fn(x: int) -> x, 'str')` → `... is called with str as argument 1 ..., but the lambda passed expects int`.
  - **Pass-through:** a parameter handed on to another function inherits that function's calls, at any depth (`fn wrap(k) -> apply(k)` is checked at `wrap(h)`).
  - **Non-functions:** `apply(5)` → `... is called as a function (line 1), but gets int`. `null` is still allowed.
  - **Lists of functions:** `fs[0](1, 2, 3)` → `none of the functions this value can be accepts 3 arguments`.
  - This covers direct, module-member, pipe and lambda arguments. It doesn't track a parameter the body reassigns.
- **Runtime arity checks and default filling** for calls the compiler can't resolve (values from `any`, lists, struct fields, native code):
  - Errors look like `function 'h' expects 2 arguments but got 1` or `function 'g' expects 1-2 arguments but got 0`.
  - Omitted default arguments are built fresh on every call.
  - `qext::call` from native code gets the same checks.
- A list of functions with different signatures can be indexed and called (`fs[0](9)`). This used to fail with "expression is not callable".

### Fixed
- A function value called with too few or too many arguments, from Quark or through `qext::call`, used to read missing arguments silently.
- `unboxToNative` read the union fields of a native function body's result without checking the tag, which gave garbage values. The read is now checked.
- Fully annotated (native) functions can be called from closures and nested functions.
- `QClosure*` native arguments and returns are boxed with `qv_closure_ptr`. They used to be wrapped in a second closure.

### Internal
- `QFnInfo {name, min_arity, max_arity, default_arg}` (`types/closure.hpp`). Codegen `emitFnInfos` emits `_qfi_<name>`, and `emitDefaultArgFns` emits `quark_<name>__default_arg`.
- `types/function.hpp` rewritten:
  - `q_call0`…`q_call12` go through `q_call_values`, which calls directly when the argument count equals the parameter count and uses `q_call_adapt` otherwise.
  - New limit `Q_MAX_CALL_ARGS = 12`.
- `types/analyzer_callshapes.go`: `FunctionType.ParamCalls`, `CallShape`, `recordParamCall`, `checkCallbackArgs`, `markParamReassigned`.
- `types.TypeFn`, `IsCallableType`, `Analysis.ModuleMemberRefs`.
- `CLAUDE.md` gains the *Language Goals*, *Catch It at Compile Time* and *Robustness Bar* sections.
- New `smoke_function_values.qrk`, about 24 new `TestSemanticsSafety_Rejected` cases, and `TestNativeCallback_Arity`.

---

## 2026-11-11: Value printing, float formatting, `**` precedence, parameter enforcement

Commit: `c33d362`

### Breaking
- **`**` binds tighter than unary minus and `!`.** `-2 ** 2` is now `-4` (it was `4`), and `!x ** 2` is `!(x ** 2)`. `2 ** -1` now parses. Existing code changes result with no warning.
- **Float output** uses the shortest round-trip form, like Python, instead of `%g`.
  - Whole floats keep `.0`: `sqrt(16)` → `4.0`, `10 / 2` → `5.0`, `sum(vector [1,2,3,4])` → `10.0`.
  - Other examples: `0.30000000000000004`, `1e+16`, `1.5e-05`, `inf`, `nan`, `-0.0`.
  - This applies to `println`, `to_str`, `std/fmt`, and error messages.
- **Container output.** `println`/`to_str` print contents instead of summaries such as `[list len=N]`, `[dict len=N]` or `Name{...}`. Tables print `[table<T> rows=N]`.
- **Parameter annotations are enforced at runtime** when the argument type isn't known at compile time, for example `fn f(x: int)` called with an `unwrap(...)` that holds a string. Such calls used to be accepted.

### Added
- **Printing by contents** (`builtins/repr.hpp`, `q_repr`):
  - Lists and vectors: `[1, 'a', null]`. Strings inside collections are quoted and escaped.
  - Dicts: `{age: 36, 'first name': 'A', name: 'Ada'}`, keys sorted and bare when they look like names.
  - Structs: `Point {x: 1, y: 2}`. Results: `ok 5` / `err 'boom'`.
  - A collection with more than 100 items shows the first and last 5 items plus a count: `[0, 1, 2, 3, 4, ..., 995, 996, 997, 998, 999] (1000 items)`.
  - Self-references print as `[...]` / `{...}`.
- `fmt.full(x)` in `std/fmt`: the `println` rendering with nothing elided.
- **Runtime parameter checks** (`builtins/params.hpp`):
  - They apply to every annotated parameter, including in partly annotated functions and in calls through function values, pipes and defaults.
  - An int passed to a `float` parameter widens: `fn scale(x: float, k) -> x * k`, then `scale(1, 2)` → `2.0`.
  - Reference annotations (`list`, `dict`, `vector`, `result`, `resource`, struct names) accept `null`.
  - The error reads `argument 'x' of 'f' expects int, got str`.

### Fixed
- A fully annotated function that captures an outer variable is no longer compiled as `DispatchNative`. That path called it without its closure.

### Docs
- New draft `stdlib_expansion_v0_1_spec.md`, not implemented: multi-argument `println`, `fmt.format`, `std/fs`, and sys/time/io/csv additions.
- Float exponent literals documented. README gains a *Known limitations* section.

### Internal
- `core/format.hpp` (`q_format_float`). Codegen `emitParamBindings` uses `q_param_int/float/bool/check/struct` and replaces two duplicated binding loops.

---

## 2026-11-11: Core cleanup; file I/O moved to `std/io`

Commit: `a5afa58`

### Breaking: removed builtins
| Removed | Use instead |
|---|---|
| `_file_open`, `_file_read`, `_file_write`, `_file_close`, `_file_seek`, `_file_exists` | `use 'std/io' as io`, then `io.open/read/write/close/seek/exists` |
| `enumerate(xs)` | `xs.enumerate()` |
| `vfrom_list(xs)` | `xs.to_vector()` |
| `str.concat(s)` | `'a' + 'b'` (`list.concat` is kept) |
| `file_handle` annotation type | `resource` |

### Breaking: other
- For an open file, `type(f)` returns `'resource'` (was `'file_handle'`), and `println(f)` shows `<resource file>`.
- `std/io` error messages changed, for example `io.read() expects an open file`.

### Added
- `vector.enumerate()`.
- GC regression tests (`integration_gc_test.go`, `src/testfiles/gc/`). They compare the GC heap and peak RSS across repeated rounds.

### Changed
- `std/io` is implemented with `extern fn` bindings to `src/core/stdlib/io.hpp` (moved from `runtime/.../builtins/fileio.hpp`). Its parameters are typed `resource`.

### Fixed
- **GC: an open file could be collected while still in use.** The resource registry was stored in `malloc`'d memory, which the GC doesn't scan. It now uses `q_allocator`.
- **GC: dict keys longer than 15 bytes leaked memory** (about 6 MiB over 8 rounds in the test). `QDictKey` now uses `q_allocator`.

### Removed
- `native_modules_design.md`. `ffi_v0_1_spec.md` is now the canonical spec for native interop.

---

## 2026-11-10: Module namespacing, dynamic methods, sorting, new stdlib modules

Commits: `5bc40d9`, `1a96b6d` (`1a96b6d` holds files `5bc40d9` needs to build, so treat them as one change)

### Breaking
- `input()` returns `null` at end of input instead of `''`. It also strips `\r\n` and reads lines of any length; before, long lines came back in roughly 4 KiB pieces.
- **Module namespacing.** Module members are renamed to `module__member`, so aliased modules can share names (`io.read` and `csv.read`), and user code can no longer shadow a module member. With an un-aliased `use`, clashing names are now `QK-LOAD-001` errors:
  - `symbol 'x' from module 'b' conflicts with symbol 'x' from module 'a'; import one of them with an alias`
  - `symbol 'x' from module 'a' (imported on line N) conflicts with this definition; ...`

  Before, whichever module was spliced last silently won.
- New compile errors for programs that used to be accepted:
  - Calling a function above its definition in the same block: `'later' is called before its definition on line N`.
  - `when` patterns other than literals, `_` or `ok`/`err` bindings: `when patterns must be literal values ...`.
  - Combining `ok`/`err` patterns with `or`.
  - `\0` in string literals. It used to cut the string short silently.

### Added
- **New stdlib modules**, each bound with `extern fn`:
  - `std/sys`: `args()`, `env(name)` (returns `null` if unset), `exit(code = 0)`.
  - `std/csv`: `parse(text, sep = ',')` and `read(path, sep = ',') result`. Follows RFC 4180 (quotes, `""` escapes, embedded newlines, CRLF). Every field is a `str`.
  - `std/time`: `now()`, `clock()` (monotonic), `sleep(seconds)`.
  - `std/random`: `seed`, `uniform`, `randint(lo, hi)` (both ends included), `choice`, `shuffle`. Uses mt19937_64, so results are reproducible after `seed`.
- `list.sort()` and `list.sort_by(key)`: in place, stable, ascending. Keys must be all numbers, all strings or all bools; mixed keys or `null` are a runtime error.
- String ordering: `<`, `<=`, `>`, `>=` on two strings compare by bytes.
- **Dynamic method dispatch** on values whose type is unknown at compile time (for example `unwrap(io.read(f, n)).split('\n')`):
  - The call switches on the runtime type tag. A receiver of the wrong type is an error: `type 'int' has no method 'upper'`.
  - At compile time, `no type has a method 'frobnicate'` is an error, and arity and argument types are still checked where possible.
- Float exponent literals: `1e3`, `2.5E-4`, `6e+2`.
- `quark run file.qrk -- arg1 arg2` passes arguments to the program.
- `io.open` mode `'a'` (append).

### Changed
- **Runtime error locations.**
  - After a call returns, an error is reported at the caller's statement.
  - An error inside an imported user module names that module's file.
  - An error inside `std/` points at the user's call.
  - Struct and table runtime errors now use the standard `error[QK-RUNTIME-001] ... at file:line:col` format.
- If generated C++ fails to build, `quark build` and `quark run` print a hint instead of dumping the C++. Rerun with `--debug` to keep it.

### Fixed
- Lowered `for i in range(...)` loops read their arguments without a type check (a float or string gave garbage), and a step of `0` looped forever or was skipped. They now behave like `range()`.
- Module functions can be called from inside user functions and closures, directly or through pipes.

### Removed
- `ops/logical.hpp` (unused) and `runtime/CMakeLists.txt` (a Catch2 setup for runtime tests that don't exist).

### Internal
- `loader/namespace.go`, `ir.DispatchDynamicMethod` with `MethodCandidates`, invariant `INV-DYNAMIC-METHOD`, `QSourceLocGuard` in every generated function, `builtins/sort.hpp`, `core/process.hpp`.

---

## 2026-11-06: Vendored Boehm GC, VS Code extension 0.1.0

Commits: `3d817ad`, `798d76d`, `6a7c994` (the prebuilt GC from `3d817ad` only works with the toolchain change in `798d76d`)

### Changed
- **Prerequisites.** Prebuilt static Boehm GC libraries are committed for linux-amd64, linux-arm64, darwin-arm64 and windows-amd64, so CMake is only needed on other platforms. This was also the first time anything under `deps/` was tracked; before, a fresh clone could not build.
- `toolchain.EnsureGC` looks for a GC library in this order: `deps/bdwgc/lib/<os>-<arch>/`, then a local `build-static/`, then a CMake build (only when the GC source is present).

### Added
- **VS Code / Positron extension 0.1.0.** See `src/devtools/vscode-extension/quark/CHANGELOG.md`. Highlights: a grammar for the current language, snippets checked against the parser, completion and hover from generated compiler data, and Python/R habits highlighted as errors.
- `go run ./cmd/quark-vsix` builds the `.vsix` without Node or npm.
- `internal/editordata` generates `data/quark.json` from the catalog, lexer keywords and stdlib, and fails when the extension falls out of sync.
- The FFI test C sources (`src/testfiles/ffi/*.c`). They were missing from the previous commit because `.gitignore` excluded `*.c`.

---

## 2026-11-04: FFI phase 1 and `std/blas`

Commits: `195a8d2`, `306b764`

### Breaking
- Text after `extern '<header>.hpp'` on the same line is a parse error.
- `quark parse` output names the header node `ExternHeader` (was `ExternSource`).

### Added
- **Native library directives**, allowed only at the top level of a file:
  ```
  extern include_path 'include'   // -I, relative to this file
  extern link 'openblas'          // -lopenblas
  ```
  - They apply to the whole program, including imported modules, with duplicates removed.
  - Link order follows the import graph.
- CLI flags `--lib-path`, `--include-path` and `--link-arg` (all repeatable), and the environment variables `QUARK_LIB_PATH` and `QUARK_INCLUDE_PATH`.
- Libraries are resolved before linking. `--debug` prints where each one was found.
- **`std/blas`** (OpenBLAS/CBLAS over f64 vectors): `dot`, `axpy`, `scal`, `nrm2`, `asum`, `gemv`. Inputs are never modified. Requires OpenBLAS.
- New error codes:
  - `QK-FFI-001`: bad `include_path`.
  - `QK-FFI-002`: native library not found. Reported at the `.qrk` line that requested it, with notes on the paths searched.
  - `QK-FFI-003`: C++ error in a glue header, reported at the header.
- Diagnostics can carry `note:` lines.

### Fixed
- An `int` argument to a `float` parameter (Quark or `extern fn`) passed `quark check` but aborted at runtime. It now widens.
- A scalar-typed (`int`/`float`/`bool`) assignment used as the value of an `if` branch generated C++ that failed to compile (`306b764`).

---

## 2026-10-07: Compilation pipeline rewrite

Commit: `c349da8`

### Breaking
- Unknown CLI flags and extra positional arguments are errors. They used to be ignored.

### Added
- **Precompiled runtime header**, cached under `$XDG_CACHE_HOME/quark/pch/` and on by default. It roughly halves compile time for small programs. Disable it with `--no-pch`.
- `quark build --debug` keeps `<output>.cpp` and prints the clang commands.
- `--lto` uses `lld` when `ld.lld` is on PATH.

### Changed
- Flags can come before or after the file, and `-flag` and `--flag` both work.
- Each build uses its own temporary directory, so concurrent runs no longer collide on `$TMP/quark_temp.cpp`.
- Compilation and linking are separate clang steps.

### Internal
- New `driver/` package (`CompileFile`/`CompileSource` by stage, returning a `Unit`) and `toolchain/` package (clang/GC discovery, compile, link, PCH). `main.go` shrank from about 860 lines to about 380.
- `types.Analysis` with a single `Generator.SetAnalysis` replaces seven `codegen.Set*` setters.
- Deleted the dead `runtime/.../builtins/fmt.hpp`.
- `ffi_v0_1_spec.md` rewritten, design only: `extern` directives instead of `ffi lib`.

---

## 2026-10-06: Arrow-compatible vectors

Commit: `f43d0f5`

### Breaking
- **`v.fillna(x)` returns a new vector and leaves `v` unchanged.** Vectors are now immutable. Code that called `v.fillna(0)` and discarded the result now silently keeps its nulls.
- Native/FFI authors:
  - Removed from `qext`: `as_bool_vec`, `as_bool_vec_mut`, `null_mask`. The old null mask used 0 = valid.
  - Added: `bool_at`, `set_bool`, `str_at`, `set_null`, `validity_bitmap` (Arrow polarity, set bit = valid), `vec_null_count`.
  - Glue that touched `QVector::storage` or `nulls` no longer compiles.

### Changed
- `QVector` uses the Apache Arrow columnar layout:
  - Validity is a bitmap.
  - Bools are bit-packed.
  - Strings use `int32` offsets plus UTF-8 data.
  - Buffers are 64-byte aligned.
- Comparing `vector[i64]` with floats or with a `vector[f64]` promotes to float. It used to be "operator not supported".
- Comparing vectors of different lengths fails with `vector size mismatch in comparison: N vs M`.
- A `vector[str]` holds at most 2 GiB of string data.

### Fixed
- `astype('i64')` on NaN or an out-of-range float is a runtime error. It used to be undefined behavior.
- `vector[i64]` arithmetic skips null slots, so a garbage value in a null slot can't raise a false overflow.

---

## 2026-10-06: Structural equality, overflow checks, stricter conditions, `all`/`any`

Commits: `26d21f9`, `f76348a` (merged through PR #1, `d899f2c`)

### Breaking
- **Equality.** `==`/`!=` on lists, dicts and results compare by value. Before, they were always `false`, even for `a == a`. Function values compare by identity.
- **Integer overflow is fatal.** Int `+ - *`, unary `-` and `**` stop with `integer overflow in '<op>'` instead of wrapping (which was undefined behavior in C++). This covers boxed values, scalar-lowered code and `vector[i64]`.
  - `2 ** 63` used to fall back to a float; it is now an error.
- **Results and vectors can't be used as conditions** in `if`, `while`, the ternary, `!`, `and` or `or`. The compile error suggests `is_ok(r)`/`when`, or `all(v)`/`any(v)`/`len(v) > 0`. Dynamic values fail at runtime.
- Assigning a non-exhaustive `when` (no `_` arm) to an annotated variable is a compile error.

### Added
- `all(v)` and `any(v)` for `vector[bool]`, skipping nulls.
- `when` as an expression on the right of an assignment:
  ```
  size = when n:
      0 -> 'none'
      _ -> 'many'
  ```

### Changed
- The Boehm GC is always built and linked statically, so the binaries Quark produces don't need libgc at runtime.

### Fixed
- `fmt.table` accepts a dict of columns again. This broke on 2026-04-03.
- Scalar-lowered `%` by zero and float `/` by zero are fatal, matching the boxed operators.
- Closure capture analysis no longer captures a `for` variable or an `ok v`/`err e` binding that shadows an outer variable.

### Docs
- README rewritten: quickstart, language tour, stdlib and CLI reference, roadmap.

---

## 2026-04-03: Table runtime scaffolding; struct fixes

Commits: `fd14038`, `df813dc`, `7b150a1`

### Added
- `tables_v0_1_spec.md`, a draft design for a struct-shaped columnar `table` type.
- Runtime support for tables (`types/table.hpp`, `VAL_TABLE`). **`table { ... }` literals don't parse**, so user code can't create a table yet.
- An empty `dict {}` is allowed as a default parameter value.

### Changed
- `fmt.show_dict` lists keys in sorted order.
- A parameter or return annotated with a struct name gets the struct type. It used to fall back to `any`.
- `null` can be assigned to struct- and vector-typed variables.

### Fixed
- Struct field types can refer to structs declared later, to each other, and to themselves.
- The type check on a struct field's default value could check the wrong field.

---

## 2026-04-02: Structs v0.1

Commits: `7fd7670`, `0474262`, `22c8a71`, `97a43ee`

### Breaking
- `struct` is a reserved keyword.

### Added
- Immutable, nominal struct types:
  ```
  struct Customer:
      id: int
      name: str
      age: int = 0            // constant defaults only
  c = Customer { id: 1, name: 'Alice' }
  fn get_name(c: Customer) str -> c.name
  ```
  - Field access is checked at compile time.
  - `type(c)` returns `'Customer'`.
  - Structs are always truthy.
- Compile errors:
  - unknown or duplicate fields;
  - missing required fields;
  - field type mismatches;
  - assignment to a field (structs are immutable);
  - `==`/`!=` on structs (compare fields explicitly).
- `structs_v0_1_spec.md`.

---

## 2026-03-28: Vector soundness, `std/fmt` on `extern`, build output

Commits: `09ac4c0`, `882108e`, `7b3f8e9`, `ab9a14d`, `eb13d87`, `a7120e1`, `1381b04`

### Breaking
- Removed the prelude builtins `_fmt_list`, `_fmt_vector`, `_fmt_dict` and `_fmt_table`. `std/fmt` now binds them as `extern fn`.
- `fmt.show_vec` requires a `vector`.

### Added
- Bool vectors: `vector [true, false]`, and `list [true, null].to_vector()`.

### Changed
- `sum`, `min` and `max` on vectors skip nulls. Each returns `null` when every value is null.
- `quark build` and `quark run` no longer print clang's loop-vectorizer remarks.
- Generated C++ no longer triggers `-Wunused-value` warnings.

### Fixed
- **Mixed-dtype vector arithmetic silently returned `null`** (for example i64 vector + f64 vector). Mixed numeric operands now promote to f64, non-numeric operands are a runtime error, and nulls propagate element by element.
- **`extern fn` arguments from untyped values were reinterpreted without a check.** For example, `extern fn add1(x: int)` given a string read garbage. `q_as_*` now fails with `expected int, got str`.
- A `vector` annotation was treated as `vector[float]`, so `for x in v` over an int vector read garbage values.
- `v.astype('i64')` now gets its result type from the dtype string, not from the receiver.
- Piping into an `extern fn` (`4 | add1()`) uses extern dispatch and checks argument types.
- A variable annotated `any` and initialized with an int can be reassigned to another type.
