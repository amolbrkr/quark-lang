# Quark FFI v0.1 Spec

Status: Phase 1 implemented 2026-11-04 (native library directives, CLI search paths, int→float widening at the boundary, build diagnostics `QK-FFI-001`–`003`, `libqffi_test`, `std/blas`). Since then: function values cross the boundary as `QClosure*` (2026-11-12), and native code is held to the element-type and `null` rules (2026-11-13). Not implemented: separately compiled glue (P2; `extern source` is a parse error), extension handles (P3, `QK-FFI-004`), `qext` result helpers (P4), the Arrow bridge (P5), tables from native code (P6), and the SQLite and DuckDB providers. The table decisions of 2026-11-13 (`tables_v0_1_spec.md`) put the native eager table engine first and make DuckDB a later, optional provider (D1); P5 and P6 follow that spec (§4).
Last Updated: 2026-11-13
Canonical for native interop: `extern` declarations, linking, the `qext` API, handles. Related: `semantics.md` §10.8 (language summary), `architecture.md` §7–8 (pipeline and resource registry), `tables_v0_1_spec.md`.

## Contents

- [1. Summary](#1-summary)
- [2. The Current Interface](#2-the-current-interface)
  - [2.1 Declarations](#21-declarations)
  - [2.2 Type mapping and checks](#22-type-mapping-and-checks)
  - [2.3 Callbacks](#23-callbacks)
  - [2.4 The qext API](#24-the-qext-api)
  - [2.5 Rules for glue code](#25-rules-for-glue-code)
  - [2.6 Native libraries](#26-native-libraries)
  - [2.7 Build diagnostics](#27-build-diagnostics)
  - [2.8 Resource handles](#28-resource-handles)
- [3. Key Decision: One Mechanism](#3-key-decision-one-mechanism)
- [4. Planned Proposals](#4-planned-proposals)
  - [P2. Separately compiled glue sources](#p2-separately-compiled-glue-sources)
  - [P3. Extension handles](#p3-extension-handles)
  - [P4. Results from native code](#p4-results-from-native-code)
  - [P5. Arrow C Data Interface bridge](#p5-arrow-c-data-interface-bridge)
  - [P6. Tables from native code](#p6-tables-from-native-code)
- [5. Providers](#5-providers)
  - [5.1 Order](#51-order)
  - [5.2 Test library: libqffi_test](#52-test-library-libqffi_test)
  - [5.3 CBLAS: std/blas](#53-cblas-stdblas)
  - [5.4 SQLite: std/sqlite (planned)](#54-sqlite-stdsqlite-planned)
- [6. DuckDB (planned)](#6-duckdb-planned)
- [7. Rollout](#7-rollout)
- [8. Decisions](#8-decisions)
  - [8.1 Decided](#81-decided)
  - [8.2 Open](#82-open)
- [9. Security](#9-security)

## 1. Summary

Quark calls native C and C++ code through one mechanism, `extern` (the Quark Extensions Interface, QEI): a C++ glue header is `#include`d into the generated program, and `extern fn` binds a Quark name to a C++ function in it. One-line directives link native libraries and add include paths. The stdlib is built on the same path (`std/io`, `std/fmt`, `std/sys`, `std/time`, `std/random`, `std/csv`, `std/blas`).

Still to come: separately compiled glue, typed handles with finalizers for library objects, result helpers, an Arrow C Data Interface bridge, and tables built by native code. They are added in the order the providers need them: SQLite, then DuckDB (§5).

## 2. The Current Interface

### 2.1 Declarations

```quark
extern 'math_glue.hpp'                               // #include the glue header
extern fn fast_sqrt(x: float) float as 'my_sqrt'     // Quark name -> C++ symbol
extern fn list.shuffle2(seed: int) list as 'q_shuf'  // a method on list
```

- `extern '<path>'` includes a header, resolved relative to the declaring `.qrk` file, or in the stdlib for `std/...`. Text after the string is a parse error.
- `extern fn name(params) [ReturnType] as 'symbol'` binds `name` to the C++ function `symbol`. `as 'symbol'` is required. An unannotated parameter or return is `any` (`QValue`).
- `extern fn type.name(...)` adds a method to `str`, `list`, `dict` or `vec`; the receiver is passed as the first C++ argument. A prelude method can't be redefined.
- An `extern fn` can be declared at the top level of a file, in a module body or in a function body. It is a definition of its file like any other (`modules_v0_2_spec.md`): reached as `file::name` from an importer, file-scoped, and an error to define twice in one file. Free extern fns are first-class values (`g = io::exists`, `x | kid()`): the back end emits a `QValue` thunk for each.

### 2.2 Type mapping and checks

| Annotation | C++ type | Argument from a value of unknown type |
|---|---|---|
| `int` | `int64_t` | checked (`q_as_int`) |
| `float` | `double` | checked; an `int` is widened |
| `bool` | `bool` | checked |
| `str` | `const char*` | checked |
| `list`, `dict`, `vec` | `QList*`, `QDict*`, `QVector*` | checked; `null` is an error |
| `fn` | `QClosure*` | checked; `null` is an error |
| `any`, `result`, `resource`, struct names, unannotated | `QValue` | passed boxed |

- Arguments of known type are checked at compile time like any call (`argument 1 of 'glen' expects list[any], got null`), including int→float widening. Arguments of unknown type go through the `q_as_*` helpers, which fail with `expected int, got str`. A `float` for an `int` parameter is an error, never truncated.
- Returns use the same mapping and are boxed back. A native function declared to return `str`, `list`, `dict`, `vec` or `fn` must not return a null pointer: that is a runtime error, `a native function declared to return list returned a null pointer`. Return `qext::null_val()` from a `QValue` function for `null`.
- A native function's value is trusted to have its declared type, so glue must return values of that type (and lists whose elements match their element type, §2.5).

### 2.3 Callbacks

An `fn` parameter arrives as `QClosure*`. Glue calls it with `qext::call(f, args...)` (zero to three `QValue` arguments, result `QValue`) and returns or stores it with `qext::box(f)`. Any Quark function value can be passed: a lambda, a capturing closure, a declared function, an `extern fn` or a module member. Each closure carries a `QFnInfo` (`types/closure.hpp`), so `qext::call` checks the argument count and fills defaults like a call in Quark: a callback called with the wrong number of arguments is a runtime error (`function 'h' expects 2 arguments but got 1`), never a missing argument. Calling a null closure panics.

C function-pointer callbacks (a `void*` userdata trampoline for APIs such as `qsort`) are not supported.

### 2.4 The qext API

`runtime/include/quark/ext/api.hpp`, namespace `qext`:

| Category | Functions |
|---|---|
| Memory | `malloc`, `malloc_atomic`, `strdup` (GC-allocated) |
| Boxing | `box(int64_t)`, `box(double)`, `box(bool)`, `box(const char*)`, `box_own(char*)`, `box(QVector*)`, `box(QList*)`, `box(QDict*)`, `box(QClosure*)`, `null_val()`. Boxing a null pointer is an error |
| Unboxing | `as_int`, `as_float`, `as_bool`, `as_str`, `as_vector`, `as_list`, `as_dict`, `as_closure` (checked; panic on a wrong type) |
| Vectors | `as_f64`, `as_i64` (zero-copy views), `as_f64_mut`, `as_i64_mut` (only on vectors the glue just created), `bool_at`, `set_bool`, `str_at`, `is_null_at`, `set_null`, `validity_bitmap`, `vec_size`, `vec_has_nulls`, `vec_null_count`, `vec_dtype`, `vec_dtype_name`, `new_f64`, `new_i64`, `new_bool_vec` |
| Lists | `new_list()` (holds any value), `list_push` (checks the list's element type) |
| Dicts | `dict_get`, `dict_set` (checks the dict's value type), `dict_size`, `dict_has` |
| Calls | `call(fn, ...)`, 0–3 arguments |
| Errors | `panic(msg)`, `panicf(fmt, ...)`: a runtime error at the Quark call site |

Results are built with the runtime constructors `qv_ok(v)` / `qv_err(v)` for now (P4 adds `qext` wrappers).

### 2.5 Rules for glue code

- **Element types.** Every list and dict carries its element type (`architecture.md` §12). Add to lists with `qext::list_push` and to dicts with `qext::dict_set`, which check it. `push_back` or writing `d->entries` directly skips the check, and a wrong element would be read with the wrong type (changelog, Known issues).
- **Vectors are immutable** once returned to Quark. Write only into vectors the glue has just created.
- **GC.** Memory reachable from Quark values must be GC memory (`qext::malloc`, or `q_allocator` for C++ containers stored in runtime data). The collector doesn't scan `malloc` memory and never runs destructors.
- **Boxing ints:** `qext::box(1)` is ambiguous in C++ (between the `int64_t`, `double` and `bool` overloads); pass an `int64_t`.
- **C++ symbols are global.** Two glue headers defining the same C++ function clash in C++, whichever Quark files include them.
- **Failures.** A programming error (wrong vector type, mismatched lengths) uses `qext::panic`. Failures caused by input or the environment should return `result` (P4).

### 2.6 Native libraries

Decided 2026-11-04: one directive per line, no grouped block (§8.1, D1).

```quark
extern 'blas_glue.hpp'          // glue header, #included
extern link 'openblas'          // link libopenblas
extern include_path 'include'   // add an include search path, relative to this file
```

- `link` and `include_path` are contextual words after `extern`, not reserved keywords. The directive is `include_path`, not `include`, because `extern '<path>'` already includes a header (D2).
- Directives must be at the top level of a file (inside a block or module: `QK-LOAD-001`). They may appear in any file, including stdlib modules; the loader collects them across all imported files into `loader.NativeDeps{IncludeDirs, Libs, Headers}` (one `NativeLib{Name, File, Line, Column}` record per `extern link`), removes duplicates, and the driver stores them in `Unit.Native`.
- Paths in directives are relative to the declaring file and point at files shipped with the module. Where a library is installed on a given machine belongs on the command line or in the environment, never in `.qrk` source, so there is no directive for library search paths.
- Link order: a module's libraries come before those of the modules it imports. A library requested more than once keeps its last position, so it stays after everything that needs it (this matters for static libraries).
- Words after the string (`extern link 'duckdb' static`) are reserved for per-library options. None exist; anything there is `QK-PARSE-001`.

CLI flags for `run` and `build`, all repeatable:

| Flag | Effect |
|---|---|
| `--lib-path <dir>` | `-L<dir>`, plus `-Wl,-rpath,<dir>` for shared libraries, so the binary runs without `LD_LIBRARY_PATH` |
| `--include-path <dir>` | `-I<dir>` |
| `--link-arg <arg>` | Passed to the linker as is |

Environment: `QUARK_LIB_PATH` and `QUARK_INCLUDE_PATH`, OS path-list separated.

**Fail fast.** Before linking, the toolchain (`toolchain/native.go`) looks for each library in the `--lib-path` and `QUARK_LIB_PATH` directories, then in the linker's defaults (`LIBRARY_PATH`, then `clang++ -print-search-dirs`), as `lib<name>.so` / `.a` (`.dylib`, `.tbd` on macOS; `<name>.lib` on Windows). A missing library is `QK-FFI-002` at the `.qrk` line that requested it, with notes listing the file names tried and the directories searched. `--debug` prints the file found for each library.

### 2.7 Build diagnostics

| Code | Stage | Meaning |
|---|---|---|
| `QK-FFI-001` | load | `extern include_path` directory not found |
| `QK-FFI-002` | build | `extern link` library not found |
| `QK-FFI-003` | build | C++ error inside a glue header, reported at the header's line |

For `QK-FFI-002` and `QK-FFI-003`, `quark run` prints only the diagnostic, not the generated C++. Other C++ compile or link errors print clang's output and a short note (check `extern fn` symbols and headers, otherwise it's a compiler bug; rerun with `--debug` to keep the C++).

### 2.8 Resource handles

`runtime/include/quark/types/resource.hpp`: a generational slot registry. Every handle has the single Quark type `resource`; its kind is runtime-only. The only kind is `file` (`std/io`). The registry's slot vectors use `q_allocator`, so payloads may be GC memory and are kept alive by the registry. Handles themselves are allocated atomic (they can't reference other objects), and nothing registers finalizers yet. P3 extends this for library objects.

## 3. Key Decision: One Mechanism

An earlier draft (2026-03-15) proposed `ffi lib 'name' as m:` blocks that bind C symbols directly, next to `extern`. Only `extern` was kept:

1. **Quark compiles to C++.** A glue header compiled into the same program is a stable boundary: the C++ compiler checks it, it can use any C or C++ API, and it costs nothing at runtime.
2. **Real C APIs need glue anyway.** Out-parameters (`duckdb_open(path, &db)`), status enums, opaque structs and caller-freed results can't be expressed with Quark parameter types.
3. **Two mechanisms would duplicate** the parser, registries, marshalling and diagnostics.
4. **Plain C functions still work** through a one-line glue header when their signature uses only `int64_t`, `double`, `bool` and `const char*`.

## 4. Planned Proposals

### P2. Separately compiled glue sources

```quark
extern source 'duckdb_glue.cpp'
```

- Compiled as its own translation unit with the program's flags (including the PCH) and linked into the binary. The glue header then holds only declarations, so programs don't reparse the implementation on every build.
- Objects cached under `~/.cache/quark/obj/`, keyed by the source, every header it includes, the compile flags and the runtime header state. The header dependency tracking is why P2 was left out of Phase 1.
- Today `extern source` is `QK-PARSE-001` (`extern source is not supported yet`). P2 adds `ExternCppSourceNode`, a `Sources` field in `NativeDeps` and `toolchain.Options`, and `QK-FFI-001` for a missing source.
- Optional for small extensions; header-only glue keeps working.

### P3. Extension handles

Extensions register their own resource kinds:

```cpp
static const uint16_t KIND_DB  = qext::define_handle_kind("duckdb.database",   close_db);
static const uint16_t KIND_CON = qext::define_handle_kind("duckdb.connection", close_con);

QValue h = qext::new_handle(KIND_CON, con_ptr, /*parent=*/db_handle);
duckdb_connection* con = qext::handle_get<duckdb_connection>(value, KIND_CON);  // QK-FFI-004 on wrong kind or closed handle
qext::handle_close(value);   // runs close_con once; later use is a runtime error
```

On the Quark side, handles keep the `resource` annotation: `extern fn duck_query(con: resource, sql: str) result as 'qduck_query'`.

Runtime changes in `resource.hpp`:

1. **Dynamic kinds.** A kind table maps id → `{name, close_fn}`, replacing the fixed `QResourceKind` enum. Built-in kinds keep ids below 16. Messages use the kind name: `expected duckdb.connection, got file`.
2. **Parent references.** `QResourceHandle` gains a `QValue parent` and is allocated with `q_malloc` (scanned), so a connection keeps its database reachable.
3. **Finalizers.** For handles with `QRES_FLAG_FINALIZABLE` (defined, unused today), `new_handle` registers a Boehm finalizer that calls `close_fn` if the handle becomes unreachable while open. Boehm finalizes in reachability order, so a connection is closed before its database. Explicit `close` stays the documented API.
4. **Payloads of library objects** are native pointers owned by the library (`malloc` or the library's allocator). A payload that points to GC memory must be kept alive through the registry, as file payloads are.
5. New code `QK-FFI-004` (runtime): wrong handle kind, or use of a closed handle.

### P4. Results from native code

```cpp
return qext::ok(qext::box(rows));
return qext::err("duckdb: " + std::string(duckdb_result_error(&res)));   // also qext::errf(fmt, ...)
```

An `extern fn` declared to return `result` already works: it returns a `QValue` built with `qv_ok`/`qv_err`. P4 adds the `qext` helpers and the convention for providers: anything that can fail because of input or the environment (bad SQL, a missing file, a failed connection) returns `result`; `qext::panic` is for bugs, such as a wrong handle kind or a broken invariant.

### P5. Arrow C Data Interface bridge

New header `runtime/include/quark/ext/arrow.hpp`. Vectors already use the Arrow layout for the types Quark has (float64 `g`, int64 `l`, boolean `b`, utf8 `u` with int32 offsets; validity bitmaps LSB first, 64-byte aligned, offset 0, immutable; `tables_v0_1_spec.md` §11).

- **ABI structs.** Vendor `struct ArrowSchema` and `struct ArrowArray` from the Arrow spec, with `ARROW_C_DATA_INTERFACE` guards so they coexist with Arrow's or DuckDB's copy.
- **Export, zero-copy.** `qext::export_vector(const QVector*, ArrowArray*, ArrowSchema*)` points at the vector's buffers. `private_data` holds an uncollectable GC root to the vector, and `release` frees it, so the buffers stay alive until the consumer releases them. Boehm never moves objects.
- **Import, copying.** `qext::import_vector(const ArrowArray*, const ArrowSchema*) -> QVector*` for `g`, `l`, `b`, `u`, plus `i`/`s`/`c` widened to i64 and `f` widened to f64, honoring `offset` and validity bitmaps. Copying is the v0.1 choice because Quark vectors are single contiguous buffers while producers like DuckDB return many small arrays. Zero-copy import (vectors wrapping foreign buffers with a release callback) waits until profiling shows the copy matters.
- **Tables** map to an Arrow struct array (`+s`) whose children are the columns, with no struct-level nulls (`export_table`, `import_table`; import needs P6).

Conformance (tables §11):
- **Padding.** Buffers are aligned but sized exactly. The C Data Interface doesn't require padding; writing Arrow IPC or Parquet must pad while serializing.
- **UTF-8.** Arrow `utf8` requires valid UTF-8. The CSV reader is to validate str columns (tables §9, prerequisite 4), every other path that builds str vectors from native bytes must validate too, and import must check.
- **Large strings.** int32 offsets cap a str vector at 2 GiB. Importing `U` (`large_utf8`) is a clear error, or a checked conversion when the data fits.
- **Proof.** P5 lands with a round-trip test through a real Arrow implementation (pyarrow or the Arrow C library, skipped when absent, like `TestStdBlas`). Today conformance is checked byte by byte (`TestVectorArrowLayout`).

### P6. Tables from native code

A provider such as DuckDB returns tables whose columns are known only when the query runs.

- **Runtime.** A table is a `QTable` whose `QStructDef` names its columns (`table/engine.hpp`). The CSV reader already builds a GC-allocated `QStructDef` at runtime for a file read without a schema (`table/csv.hpp`). P6 exposes the same to glue: `qext::new_table(names, columns, n)`.
- **Typing** follows the tables spec. A native function returning a table is annotated `tbl`. Such a table has no row struct the compiler knows, so it can be printed, rendered with `fmt::table` and written, but a query on it is a compile error until runtime-checked queries arrive with the inference extension (D11, tables §10). Whether a native table can be given a row struct with `t: tbl[Employee] = ...`, checked against the struct at runtime the way `csv::read_file` checks a file, is open (§8.2).
- P6 starts after the `tbl` type exists in the compiler (tables §9).

## 5. Providers

### 5.1 Order

Providers are added in order of difficulty, each proving one more set of proposals (D3):

| Order | Provider | Proves | Phase | State |
|---|---|---|---|---|
| 1 | `libqffi_test`, built by the test suite (§5.2) | Linking, search paths, rpath, link order, `QK-FFI-001`/`002`, int→float widening | 1 | Done 2026-11-04 |
| 2 | CBLAS via OpenBLAS, `std/blas` (§5.3) | A real system library; native code reading vector buffers without copying | 1 | Done 2026-11-04 |
| 3 | SQLite, `std/sqlite` (§5.4) | P3 handles with parents and finalizers; P4 results; building columns from native results | 2 | Planned |
| 4 | DuckDB, `std/duckdb` (§6) | Chunked columnar results and the type matrix; then P5 and P6 | 3+ | Planned, after the native tables |

### 5.2 Test library: libqffi_test

`src/testfiles/ffi/`: small C sources (`qffi_test.c`, `qffi_a.c`, `qffi_b.c`), glue headers, and the programs `ffi_basic.qrk` and `ffi_modules.qrk` with modules `mod_a`–`mod_c`. `TestFFI_Phase1` (`integration_ffi_test.go`) builds the libraries with clang into a temporary directory and compiles the programs against them with `--lib-path`. It needs only clang++, so it runs in CI. Cases:

- plain functions over `int64_t`, `double` and `const char*` through a one-line glue header;
- shared linking with rpath (the binary runs without `LD_LIBRARY_PATH`);
- static linking and link order: `libqffi_a` calls into the static-only `libqffi_b`, declared in different modules;
- the same `extern link` in two modules is linked once;
- `extern include_path` resolves relative to the declaring file;
- errors: a missing library (`QK-FFI-002`, listing searched directories), a missing include path (`QK-FFI-001`), words after a directive's string (`QK-PARSE-001`);
- an `int` argument to an extern `float` parameter.

Callbacks are covered by `smoke_extern.qrk` and `TestNativeCallback_Arity`; element-type checks in `qext::list_push`/`dict_set` and native null returns by `TestElementTypes_ModulesAndNative`.

### 5.3 CBLAS: std/blas

BLAS level-1 routines take a pointer and a length, and f64 vectors are contiguous buffers that `qext::as_f64` exposes without copying, so native code runs over Quark data at native speed. There are no handles, results or callbacks.

```
src/core/stdlib/blas.qrk         extern link 'openblas', extern fn bindings, module blas
src/core/stdlib/blas_glue.hpp    header-only glue; includes cblas.h and quark/ext/api.hpp
```

- Links the system OpenBLAS (not vendored). Debian/Ubuntu: `sudo apt install libopenblas-dev`. macOS: `brew install openblas`; Homebrew's OpenBLAS is keg-only, so pass `--lib-path "$(brew --prefix openblas)/lib"` and `--include-path "$(brew --prefix openblas)/include"`, or set the environment variables.
- OpenBLAS only. Other BLAS builds (reference `libblas`, MKL, Apple Accelerate) use other library names or need `-framework`, which needs per-library options.

| Function | CBLAS routine | Returns |
|---|---|---|
| `dot(x: vec, y: vec)` | `cblas_ddot` | `float` |
| `axpy(a: float, x: vec, y: vec)` | `cblas_daxpy` | new vector `a*x + y` |
| `scal(a: float, x: vec)` | `cblas_dscal` | new vector `a*x` |
| `nrm2(x: vec)` | `cblas_dnrm2` | `float` |
| `asum(x: vec)` | `cblas_dasum` | `float` |
| `gemv(a: vec, rows: int, cols: int, x: vec)` | `cblas_dgemv`, row-major | new vector of length `rows` |

`gemv` takes the matrix as a flat row-major f64 vector with explicit dimensions, because Quark has no matrix type.

```quark
incl 'std/blas'

x = vec [1.0, 2.0, 3.0]
y = vec [4.0, 5.0, 6.0]
print(blas::dot(x, y))          // 32.0
z = blas::axpy(2, x, y)         // the int 2 widens to 2.0
print(z[0])                     // 6.0
print(y[0])                     // 4.0: inputs are never modified
```

Rules:
- Inputs must be f64 vectors without nulls. An i64 vector, a vector with nulls, mismatched lengths, or `rows * cols != len(a)` is a runtime error through `qext::panic` naming the function and suggesting `astype('f64')` or `fillna(...)`.
- Routines that write in place (`daxpy`, `dscal`) run on a fresh copy, which is returned.
- Lengths are passed as CBLAS `int` (32-bit in common builds); a longer vector is a runtime error.
- OpenBLAS worker threads touch only the buffers passed in and finish before the call returns, so they don't need GC registration.

Tests: `smoke_blas.qrk`, and `TestStdBlas`, which also runs one program per error case. Both skip when OpenBLAS isn't found.

### 5.4 SQLite: std/sqlite (planned)

A rehearsal for DuckDB with the same API shape (`open`, `exec`, `query`, `close`), handles with parents and finalizers, and `err` results. It uses the system library (`libsqlite3-dev`); vendoring the amalgamation through `extern source` becomes possible with P2. The detailed design is written before Phase 2 starts. The main question: SQLite types each value, not each column, so `query` needs a rule for building a typed column (declared column type, first non-null value, or `err` on mixed types).

## 6. DuckDB (planned)

DuckDB is an optional provider, not the core table engine: tables are native and eager (`tables_v0_1_spec.md` D1), and DuckDB comes after them, for SQL over data larger than the native engine handles comfortably.

**Why DuckDB.** A stable C API (`duckdb.h`) with prebuilt `libduckdb` for Linux, macOS and Windows; columnar, chunked results that map onto Quark vectors with little more than `memcpy` for numeric columns; and it exercises P1, P3, P4, P5 and P6.

**Packaging.**

```
src/core/stdlib/duckdb.qrk        module duckdb (Quark API)
src/core/stdlib/duckdb_glue.hpp   declarations for the externs
src/core/stdlib/duckdb_glue.cpp   implementation (P2), includes duckdb.h and quark/ext/api.hpp
```

- The library is not vendored (tens of megabytes). Users point Quark at an install with `--lib-path`/`--include-path` or the environment variables. A `scripts/fetch_duckdb.sh` downloads a pinned release into `deps/duckdb/`, which the toolchain also checks. Without the library, `incl 'std/duckdb'` fails with `QK-FFI-002` and a hint to run the script.
- Shared linking by default, with rpath. Static linking (`libduckdb_static.a`, about 50 MB per binary) is an open question (§8.2).
- Pin one 1.x release and check `duckdb_library_version()` at `open`; a major-version mismatch is a clear error.

**API (first version).**

```quark
incl 'std/duckdb' as duck

db = unwrap(duck::open(':memory:'))          // or a file path
con = unwrap(duck::connect(db))

unwrap(duck::exec(con, "create table t as select range as i, range * 0.5 as f from range(1000000)"))

cols = unwrap(duck::query(con, "select i, f from t where i % 2 = 0"))
print(len(cols.i))        // 500000

when duck::query(con, "select * from missing"):
    ok(c) -> print(c)
    err(msg) -> print(msg)   // "duckdb: Catalog Error: Table with name missing does not exist!..."

duck::close(con)
duck::close(db)
```

| Function | Returns | Notes |
|---|---|---|
| `open(path: str)` | `result` (database handle) | `':memory:'` for in-memory |
| `connect(db: resource)` | `result` (connection handle) | The connection keeps the database alive (P3 parent) |
| `exec(con: resource, sql: str)` | `result` (int rows changed) | DDL and DML |
| `query(con: resource, sql: str)` | `result` (dict of column name → vector, or `tbl`, §8.2) | |
| `close(h: resource)` | `null` | Idempotent; either handle kind |
| `version()` | `str` | `duckdb_library_version()` |

Later: `query_params(con, sql, params: list)` with prepared statements, `insert(con, table_name, cols)` with the appender, and `register(con, name, t)` to query a Quark table in place through Arrow (P5).

**Result conversion.** `query` runs `duckdb_query`, then converts one chunk at a time (`duckdb_fetch_chunk`, `duckdb_data_chunk_get_vector`, `duckdb_vector_get_data`, `duckdb_vector_get_validity`); each column becomes one vector.

| DuckDB type | Quark dtype | Conversion |
|---|---|---|
| `BIGINT` | i64 | `memcpy` per chunk |
| `INTEGER`, `SMALLINT`, `TINYINT`, `UINTEGER`, `USMALLINT`, `UTINYINT` | i64 | widening loop |
| `UBIGINT` | i64 | checked; values above `INT64_MAX` return `err` |
| `DOUBLE` | f64 | `memcpy` |
| `FLOAT` | f64 | widening loop |
| `BOOLEAN` | bool | one byte per value, packed into bits |
| `VARCHAR` | str | `duckdb_string_t` (inlined up to 12 bytes, otherwise a pointer), appended to the offsets and bytes buffers |
| `DECIMAL`, `HUGEINT`, `DATE`, `TIMESTAMP`, nested types | not supported | `err` naming the column and suggesting a cast (`CAST(x AS DOUBLE)`) |

Nulls: DuckDB's validity mask is `uint64_t` words, one bit per row, set = valid, and a null mask pointer means all valid. On little-endian targets that is the Arrow bitmap layout, so a chunk starting at a multiple of 8 rows copies its mask with `memcpy`; other chunks use a bit-shifting copy. Column buffers grow geometrically, since the row count isn't known up front. All copying happens on the calling thread, so DuckDB's worker threads never see GC memory.

**Handles.**

| Kind | Payload | Close | Parent |
|---|---|---|---|
| `duckdb.database` | `duckdb_database*` (malloc) | `duckdb_close` | none |
| `duckdb.connection` | `duckdb_connection*` (malloc) | `duckdb_disconnect` | database handle |

Results and data chunks never become handles: they are converted and destroyed inside the native call.

**Tests.** Skipped when libduckdb isn't found. `smoke_duckdb.qrk`: open, exec, query, close; one column per supported type; nulls in every dtype; results over 2048 rows (several chunks); a filtered query whose chunk boundaries aren't multiples of 8; error results for bad SQL and unsupported types; use after close (`QK-FFI-004`). A finalizer test (open a connection, drop it, force a GC, check a debug counter in the glue). Benchmark, not in CI: importing a 10M-row `BIGINT` column should run near `memcpy` speed.

## 7. Rollout

| Phase | Contents | Done when |
|---|---|---|
| 1. Linking | Directives, CLI, widening, `QK-FFI-001`–`003` | **Done 2026-11-04** (§5.2, §5.3) |
| 2. Handles and results | P2, P3, P4, `QK-FFI-004` | `std/sqlite` `open`/`exec`/`query`/`close` pass a smoke test, including a finalizer test |
| 3. DuckDB | `std/duckdb` | `open`/`connect`/`exec`/`query`/`close` pass `smoke_duckdb.qrk` |
| 4. Tables and parameters | P6, `query_params`, `insert` | `query` returns a `tbl`; parameterized queries; bulk insert |
| 5. Arrow | P5 | `register` queries a Quark table from DuckDB without copying; `import_vector` serves Arrow-producing providers |

Phase 4 depends on the `tbl` type in the compiler (`tables_v0_1_spec.md` §9).

## 8. Decisions

### 8.1 Decided

**D1. Directive syntax: one directive per line** (2026-11-04). `extern link 'x'`, `extern include_path 'dir'` and (with P2) `extern source 'x.cpp'` each stand alone, next to `extern '<path>'`. A block per library (`extern lib 'duckdb':` with entries) was rejected: it doesn't match the one-line header form, it is harder to parse, it adds a second way to include a header, and it invites `extern fn` declarations inside it, which would bring back the `ffi lib` design (§3). Flat directives work at scale in Nim (`{.passL.}`, `{.compile.}`) and cgo (`#cgo LDFLAGS:`). Per-library records internally and the reserved words after the string keep a block form or per-library options possible later. Revisit when static linking, platform-specific libraries (`-framework`, Windows `.lib` names) or `pkg-config` lookup are needed.

**D2. `extern include_path`, not `extern include`** (2026-11-04): `extern '<path>'` already includes a header, so `extern include 'duckdb.h'` would be misread.

**D3. Provider order** (2026-11-04): `libqffi_test` → CBLAS → SQLite → DuckDB (§5.1). DuckDB needs nearly every proposal at once, so it is a poor first proof.

**D4. DuckDB is optional, after native tables** (2026-11-13, tables D1): the table engine is native and eager; DuckDB is a provider for later, not the backend of `tbl`.

### 8.2 Open

1. **Static DuckDB.** A flag (`--static-lib duckdb`), a directive option (the reserved words after the string), or later?
2. **Fetching libduckdb.** A fetch script into `deps/duckdb/`, or a user-installed library only?
3. **Unsupported column types.** For `DECIMAL` and temporal types: an error with a cast hint (proposed), or automatic conversion (`DECIMAL` → f64, `TIMESTAMP` → i64 microseconds) with documented precision loss? The same applies to SQLite. Once tables have a `date` type (tables §9), `DATE` maps to it.
4. **Named handle kinds.** Plain `resource` (proposed), or `resource[duckdb.connection]` annotations?
5. **What `query` returns.** A dict of column name → vector in phases 2 and 3, or a `tbl` from the start, since the native `tbl` type is planned before DuckDB?
6. **Typing native tables.** Can a table from native code be given a row struct (`t: tbl[Employee] = unwrap(duck::query(...))`), checked against the struct at runtime like `csv::read_file`, so it can be queried before tables §10?

## 9. Security

Native code runs with full process privileges; Quark's runtime checks don't sandbox it. Glue code must follow the `qext` rules (§2.5), never mutate a vector after returning it, and give every native allocation exactly one owner. Third-party providers are trusted code; the documentation of `extern link` must say so.
