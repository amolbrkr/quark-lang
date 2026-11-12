# Quark FFI v0.1 Spec

Status: Draft (revision 3)
Last Updated: 2026-11-04
Replaces: the 2026-03-15 draft of this file
Revision 3: records the directive syntax decision, renames `extern include` to `extern include_path`, and sets the provider order (§5, §9.1).
Implementation status (2026-11-04): Phase 1 is implemented (P1, P7, the P8 build codes `QK-FFI-001`–`003`, `libqffi_test`, `std/blas`) except P2, which moved to Phase 2. `extern source` currently reports `QK-PARSE-001` saying it isn't supported yet. §2 describes the state before Phase 1.
Canonical for native interop (`extern` declarations, linking, handles); it replaces `native_modules_design.md`, which was removed on 2026-11-10.
Related: `architecture.md` §1.7 and §7, `semantics.md` §10.6, `tables_v0_1_spec.md`

## 1. Summary

Quark can already call native C++ through `extern` (the Quark Extensions Interface, QEI). What it cannot do is use a native *library*: there is no way to link one, no way for native code to hand Quark a long-lived object safely, and no fast way to move bulk data across the boundary.

This revision drops the separate `ffi lib` mechanism proposed in the March draft and instead extends `extern` with the missing pieces:

1. **Native dependencies**: `extern link`, `extern include_path` and `extern source` directives, plus library search flags, so a Quark program can link a library such as `libopenblas` or `libduckdb`.
2. **Extension handles**: extensions define their own resource kinds (`duckdb.connection`, ...) on top of the existing resource registry, with GC finalizers as a safety net.
3. **Results from native code**: `ok`/`err` helpers so native calls report failures as Quark results instead of aborting.
4. **Bulk data**: an Arrow C Data Interface bridge for Quark vectors (which already use the Arrow layout), and tables whose schema is built at runtime.

DuckDB is the target provider: it exercises every one of these pieces, and its columnar results map directly onto Quark vectors. Because it needs nearly everything at once, it is not the first library linked. Providers are added in order of difficulty: a test library built by the test suite, then CBLAS, then SQLite, then DuckDB (§5).

## 2. Current State

### 2.1 What exists

| Piece | Where | Notes |
|---|---|---|
| `extern 'x.hpp'` | parser → `ExternSourceNode`; loader resolves the path; codegen emits `#include` | Header only. Path resolves relative to the `.qrk` file or `std/`. |
| `extern fn name(params) T as 'sym'` | `types/analyzer_modules.go` `analyzeExternFn` | Free functions and `type.method` methods. Registers a `CallPlan` with `DispatchExtern`. |
| Type mapping | `quarkTypeToNativeCType` | `int`→`int64_t`, `float`→`double`, `bool`→`bool`, `str`→`const char*`, `list`/`dict`/`vector`→`QList*`/`QDict*`/`QVector*`, `fn`→`QClosure*`, anything else→`QValue`. |
| Marshalling | `codegen.adaptArgForExtern` / `wrapExternReturn` | Guarded unboxing (`q_as_int`, ...) at call sites; returns boxed back. Thunks make free externs first-class. |
| Callbacks | `fn` annotation (`types.TypeFn`) | An `fn` parameter is a `QClosure*`; glue calls it with `qext::call` (argument count checked, defaults filled, via the closure's `QFnInfo`) and boxes it with `qext::box`. Tested in `smoke_extern.qrk` and `TestNativeCallback_Arity`. C function-pointer callbacks (a `void*` userdata trampoline for C APIs such as `qsort`) are not supported. |
| Extension author API | `runtime/include/quark/ext/api.hpp` (`qext::`) | Boxing, unboxing, vector buffer views, dict helpers, `qext::call`, `qext::panic`. |
| Resource handles | `runtime/include/quark/types/resource.hpp` | Generational slot registry. `resource` is the type annotation for every handle. Used only by file I/O (`std/io`). |
| Vector layout | `runtime/include/quark/types/vector.hpp` | Arrow-compatible since 2026-10-06: contiguous f64/i64, bit-packed bool, int32-offset str, Arrow validity bitmap, 64-byte aligned, immutable. |
| Build | `driver/` + `toolchain/` (2026-10-07) | Front end in one place; separate `Compile` and `Link` steps; cached precompiled runtime header. |

The stdlib already uses this path: `std/fmt.qrk` declares externs against `fmt.hpp` and wraps them in a `module fmt`.

### 2.2 Gaps

| # | Gap | Consequence |
|---|---|---|
| G1 | No way to declare a native library dependency. The link line is fixed: GC, `-pthread`, `-ldl`, `-lm`. | Only header-only C++ can be used. |
| G2 | No way to add include or library search paths. | Even a header-only wrapper around a system library can't find its headers outside the default paths. |
| G3 | Glue code must be header-only and is recompiled into every program. | Large glue layers slow down every build. |
| G4 | Resource kinds are a hardcoded enum (`QRES_KIND_FILE`). | Extensions cannot create typed handles; they would have to smuggle pointers through `int` or `any`. |
| G5 | `QRES_FLAG_FINALIZABLE` exists but nothing registers a finalizer. | A leaked handle leaks its native object. |
| G6 | Registry payloads live in a `std::vector` the GC does not scan, and `QResourceHandle` is allocated atomic. | A handle cannot keep a parent object alive (a connection cannot keep its database alive). |
| G7 | No `qext` helpers to build `ok`/`err` results. | Native failures are reported with `qext::panic`, which exits the program. |
| G8 | No Arrow C Data Interface export or import. | The Arrow-compatible layout can't be shared with Arrow-speaking libraries yet. |
| G9 | `QTable` requires a compile-time `QStructDef`. | Native code can't return a table whose columns are known only at runtime (any SQL query). |
| G10 | `q_as_float` rejects `int`. | The March draft's marshalling matrix (int widens to float) is not implemented; `halve(7)` fails at runtime. |

## 3. Key Decision: One Mechanism

The March draft proposed `ffi lib 'name' as m:` blocks that bind C symbols directly, next to QEI's header-based `extern`. This revision keeps only `extern`.

Reasons:

1. **Quark already compiles to C++.** A glue header compiled into the same program is a stable boundary: it is checked by the C++ compiler, it can use any C or C++ API, and it costs nothing at runtime.
2. **Real C APIs need glue anyway.** DuckDB's API uses out-parameters (`duckdb_open(path, &db)`), status enums, opaque structs, and caller-freed results. None of these can be expressed with Quark parameter types, so a direct-binding layer would still need hand-written wrappers.
3. **Two mechanisms would duplicate everything**: two parsers, two registries, two marshalling paths, two sets of diagnostics.
4. **Calling plain C functions still works.** When a C function's signature only uses `int64_t`, `double`, `bool` and `const char*`, a one-line glue header that includes the library header lets `extern fn` call it directly.

Kept from the March draft: the type surface and marshalling matrix (§6), the ownership contract (§7), the error contract (§8), the performance model, and the requirement to fail fast with actionable diagnostics when a library can't be found.

## 4. Proposals

Each proposal lists its syntax or API, then the compiler and runtime changes.

### P1. Native dependency directives (fixes G1, G2)

Decided 2026-11-04: one directive per line, no grouped block (§9.1, D1).

```quark
extern 'blas_glue.hpp'          // existing: glue header, #included
extern link 'openblas'          // new: link libopenblas
extern include_path 'include'   // new: add an include search path, relative to this file
```

- `link`, `include_path` and `source` (P2) are contextual words after `extern`, so there are no new reserved keywords. The existing `extern '<path>'` and `extern fn` forms keep their meaning.
- The directive is `include_path`, not `include`, because `extern '<path>'` already `#include`s a header. `extern include 'duckdb.h'` would read as if it did the same (§9.1, D2).
- `extern link 'name'` means `-lname`. The library is searched for in the `--lib-path` directories, then the `QUARK_LIB_PATH` entries, then the linker's defaults (`LIBRARY_PATH`, then clang's search path from `clang++ -print-search-dirs`; on macOS also the SDK's `usr/lib`).
- Paths in directives are relative to the declaring file and point at files shipped with the module. Where a library is installed on a given machine belongs in `--lib-path`/`--include-path` or the environment variables, never in `.qrk` source. For that reason there is no directive for library search paths.
- Directives may appear in any file, including stdlib modules, but only at the top level of the file: inside a `module` block or any other block they are reported as `QK-LOAD-001`. The loader collects them across all imported files and removes duplicates.
- Link order: a module's libraries come before those of the modules it imports. When a library is requested more than once, its last position is kept, so it stays after everything that needs it. This matters for static libraries.
- Reserved: words after the string (for example `extern link 'duckdb' static`) are reserved for per-library options. v0.1 defines none and reports `QK-PARSE-001` for anything after the string.
- Each `extern link` becomes one library record internally (`loader.NativeLib{Name, File, Line, Column}`), not a bare flag string. A grouped block form or per-library options (an `Options` field) can then be added later without changing the driver or toolchain.

CLI additions for `run` and `build`, all repeatable:

| Flag | Effect |
|---|---|
| `--lib-path <dir>` | `-L<dir>`, and `-Wl,-rpath,<dir>` for shared libraries so the binary runs without `LD_LIBRARY_PATH` |
| `--include-path <dir>` | `-I<dir>` |
| `--link-arg <arg>` | Passed to the linker as-is (escape hatch) |

Environment: `QUARK_LIB_PATH` and `QUARK_INCLUDE_PATH`, using the OS path-list separator.

**Fail fast.** Before linking, the toolchain looks for each `extern link` library in the search directories (`lib<name>.so`, `.dylib`, `.a`; `<name>.lib` on Windows). A missing library produces a diagnostic that names the library, the `.qrk` file and line that requested it, and the directories searched. With `--debug`, the resolved file for each library is printed.

Compiler changes:

| Package | Change |
|---|---|
| `ast` / `parser` | Rename the existing `ExternSourceNode` (the `extern '<path>'` header form) to `ExternHeaderNode`, so "source" means only P2's `.cpp` files. Parse `extern link` and `extern include_path` into `ExternLinkNode` and `ExternIncludePathNode`. `extern source` (`ExternCppSourceNode`) comes with P2; until then it is reported as `QK-PARSE-001`. |
| `loader` | Resolve `extern include_path` paths (and P2 `extern source` paths) to absolute paths, like `extern '<path>'` today. |
| `loader` / `driver` | The loader collects the directives into `loader.NativeDeps{IncludeDirs, Libs []NativeLib, Headers}`, keeping the source location of each; the driver stores it in `Unit.Native`. P2 will add a `Sources` field. |
| `toolchain` | Add `Options.IncludeDirs`, `LibDirs`, `Libs`, `LinkArgs`, and `GlueFiles` (the glue headers, for `QK-FFI-003`). Add library resolution with diagnostics. `Link` emits `-L`, `-l` and rpath arguments. `Options.Sources` comes with P2. |
| `main.go` | Register the new flags on the shared `buildFlags`. |

Codegen does not change: `#include` emission already works, and linking is the toolchain's job.

### P2. Separately compiled glue sources (fixes G3)

```quark
extern source 'duckdb_glue.cpp'
```

- The file is compiled as its own translation unit with the same flags as the program, including the PCH. It is linked into the binary.
- Objects are cached under `~/.cache/quark/obj/`, keyed by the source file's content, the compile flags, and the runtime header state (the same key inputs as the PCH).
- The glue header (`extern 'duckdb_glue.hpp'`) then holds only declarations, so programs don't reparse the glue implementation on every build.

This step is optional for small extensions; header-only glue keeps working.

### P3. Extension handles (fixes G4, G5, G6)

Extensions register their own resource kinds at startup:

```cpp
// in glue code
static const uint16_t KIND_DB  = qext::define_handle_kind("duckdb.database",   close_db);
static const uint16_t KIND_CON = qext::define_handle_kind("duckdb.connection", close_con);

QValue h = qext::new_handle(KIND_CON, con_ptr, /*parent=*/db_handle);
duckdb_connection* con = qext::handle_get<duckdb_connection>(value, KIND_CON);  // panics on wrong kind or closed handle
qext::handle_close(value);   // runs close_con once; later use is a runtime error
```

On the Quark side, handles are typed with the existing `resource` annotation:

```quark
extern fn duck_query(con: resource, sql: str) result as 'qduck_query'
```

Runtime changes in `resource.hpp`:

1. **Dynamic kinds.** A kind table maps id → `{name, close_fn}`. Built-in kinds (file) keep ids below 16; extension kinds get the following ids. Error messages use the kind name: `expected duckdb.connection, got file`.
2. **Parent references.** `QResourceHandle` gains a `QValue parent` field and is allocated with `q_malloc` (scanned) instead of `q_malloc_atomic`. A connection handle that references its database keeps the database reachable.
3. **Finalizers.** When a handle has `QRES_FLAG_FINALIZABLE`, `new_handle` registers a Boehm finalizer that calls `close_fn` if the handle is still open when it becomes unreachable. Boehm finalizes in reachability order (an object that points to another is finalized first), so a connection is closed before its database. Explicit `close` is still the documented API; finalizers are a safety net.
4. **Payload visibility.** Payloads are native pointers (`malloc`, or memory owned by the library) and must not point to GC memory. This is a documented rule, because the registry is not scanned by the GC.

Analyzer: no new types. `resource` already exists; a later version can add named kinds (`resource[duckdb.connection]`) if typos in kinds turn out to matter.

### P4. Results from native code (fixes G7)

```cpp
return qext::ok(qext::box(rows));
return qext::err("duckdb: " + std::string(duckdb_result_error(&res)));   // also qext::errf(fmt, ...)
```

- An extern fn whose return annotation is `result` returns `QValue`. This already works through `quarkTypeToNativeCType`'s fallback; P4 adds the helpers, a test, and documentation.
- Convention for providers: anything that can fail because of input or environment (bad SQL, missing file, failed connection) returns `result`. `qext::panic` is reserved for bugs, such as a wrong handle kind or a broken invariant.

### P5. Arrow C Data Interface bridge (fixes G8)

New header `runtime/include/quark/ext/arrow.hpp`:

- **ABI structs.** Vendor `struct ArrowSchema` and `struct ArrowArray` as defined by the Arrow spec. The spec intends these definitions to be copied into projects, with `ARROW_C_DATA_INTERFACE` guards so they coexist with Arrow's or DuckDB's copy.
- **Export, zero-copy.** `qext::export_vector(const QVector*, ArrowArray*, ArrowSchema*)`. Quark vectors are immutable and their buffers are already in Arrow layout (offset 0, 64-byte aligned), so export just points at them. The array's `private_data` holds an uncollectable GC root to the vector, and `release` frees that root. Boehm never moves objects, so other threads can safely read the buffers until `release` is called.
- **Import, copying.** `qext::import_vector(const ArrowArray*, const ArrowSchema*) -> QVector*` for formats `g`, `l`, `b`, `u`, plus `i`/`s`/`c` widened to i64 and `f` widened to f64. It honors the Arrow `offset` field and validity bitmaps. Copying is the v0.1 choice because Quark vectors are single contiguous buffers, while producers like DuckDB return many small arrays.
- **Tables.** `export_table` / `import_table` map a Quark table to and from an Arrow struct array whose children are the columns (needs P6 for import).

Zero-copy *import* needs vectors that can wrap foreign buffers, with an owner and a release callback. That's deferred until profiling shows the import copy matters.

### P6. Tables with a runtime schema (fixes G9)

- Runtime: `q_struct_def_create(name, field_names, n)` builds a GC-allocated `QStructDef` at runtime. `qext::new_table(names, columns, n)` builds a `QTable` from it.
- Analyzer: a native function returning a table is annotated with the plain `table` type that `tables_v0_1_spec.md` §4.3 already describes for untyped tables. Column access is checked at runtime. If the plain `table` annotation is not implemented yet when this phase starts, providers return a `dict` of column name → vector in the meantime (§6.3).

### P7. Numeric widening at the boundary (fixes G10)

`q_as_float` accepts `int` and converts it, as the marshalling matrix requires. Narrowing (`float` → `int`) stays a runtime error. This only relaxes existing behavior; no current program changes meaning.

### P8. Diagnostics and error codes

New codes in `error_codes.md`:

| Code | Stage | Meaning |
|---|---|---|
| `QK-FFI-001` | load | `extern include_path` or `extern source` path not found |
| `QK-FFI-002` | build | `extern link` library not found (lists searched directories) |
| `QK-FFI-003` | build | glue source failed to compile (names the glue file, not the generated C++) |
| `QK-FFI-004` | runtime | wrong handle kind, or use of a closed handle |

When a compile error is inside glue code (an `extern` header or `extern source` file), report `QK-FFI-003` naming that file and skip the generated-C++ dump. Before Phase 1, clang's message pointed at the right line in the glue header, but `quark run` then printed the entire generated C++ underneath it, which buried the real error. Phase 1 implements this for glue headers: for `QK-FFI-002` and `QK-FFI-003`, `quark run` prints only the diagnostic (`reportBuildError` in `main.go`). Other C++ compile or link errors print clang's output and a short note (check `extern fn` symbols and headers, otherwise it's a compiler bug; rerun with `--debug` to keep the C++), not the generated program.

## 5. Providers

### 5.1 Order

DuckDB is the target, but it is not the first library linked. It needs nearly every proposal at once, so a failure there wouldn't say which piece is wrong. Providers are added in order of difficulty, each proving one more set of proposals (§9.1, D3):

| Order | Provider | Proves | Phase |
|---|---|---|---|
| 1 | `libqffi_test`, built by the test suite (§5.2) | P1: linking, search paths, rpath, link order, `QK-FFI-001`/`002`; P7 | 1 |
| 2 | CBLAS via OpenBLAS, `std/blas` (§5.3) | P1 against a real system library; native code reading Quark vector buffers without copying | 1 |
| 3 | SQLite, `std/sqlite` (§5.4) | P3 handles with parents and finalizers; P4 results; building columns from native results | 2 |
| 4 | DuckDB, `std/duckdb` (§6) | Chunked columnar results and the type matrix; later P5 and P6 | 3+ |

### 5.2 Test library: `libqffi_test`

- Lives in `src/testfiles/ffi/`: a small C source plus a glue header. The integration test builds it with clang into a temporary directory and compiles Quark programs against it with `--lib-path`.
- It has no external dependency, so it runs wherever clang++ is available, including CI.
- Cases:
  - Plain functions over `int64_t`, `double` and `const char*`, called through a one-line glue header.
  - Shared linking with rpath: the binary runs without `LD_LIBRARY_PATH`.
  - Static linking and link order: `libqffi_a` calls into `libqffi_b`, which is built static-only, and the two are declared in different modules.
  - The same `extern link` in two modules is linked once.
  - `extern include_path` resolves relative to the declaring file.
  - Errors: a missing library gives `QK-FFI-002` listing the searched directories; a missing include path gives `QK-FFI-001`; words after a directive's string give `QK-PARSE-001`.
  - P7: an extern `float` parameter called with an `int` argument.

### 5.3 CBLAS: `std/blas`

**Why.** BLAS is the standard native numeric library. Level-1 routines take a pointer and a length, and Quark f64 vectors are contiguous Arrow-layout buffers that `qext::as_f64` exposes without copying. This is the first demonstration that native code can run over Quark data at native speed. It has no handles, no error results and no callbacks, so it tests linking and the vector boundary and nothing else.

**Packaging.**

```
src/core/stdlib/blas.qrk         public module
src/core/stdlib/blas_glue.hpp    header-only glue; includes cblas.h and quark/ext/api.hpp
```

- Not vendored. The module links the system OpenBLAS with `extern link 'openblas'`.
  - Debian/Ubuntu: `sudo apt install libopenblas-dev`. `cblas.h` lands in a default search path (for example `/usr/include/x86_64-linux-gnu/`).
  - macOS: `brew install openblas`. Homebrew's OpenBLAS is keg-only, so pass `--lib-path "$(brew --prefix openblas)/lib"` and `--include-path "$(brew --prefix openblas)/include"`, or set the environment variables.
- v0.1 supports OpenBLAS only. Other BLAS builds (reference `libblas`, MKL, Apple Accelerate) use different library names or need `-framework`, which needs per-library options (reserved in P1).
- The glue is small, so it stays header-only and does not need P2.

**API (v0.1).**

| Function | CBLAS routine | Returns |
|---|---|---|
| `dot(x: vector, y: vector)` | `cblas_ddot` | `float` |
| `axpy(a: float, x: vector, y: vector)` | `cblas_daxpy` | new vector `a*x + y` |
| `scal(a: float, x: vector)` | `cblas_dscal` | new vector `a*x` |
| `nrm2(x: vector)` | `cblas_dnrm2` | `float` |
| `asum(x: vector)` | `cblas_dasum` | `float` |
| `gemv(a: vector, rows: int, cols: int, x: vector)` | `cblas_dgemv`, row-major | new vector of length `rows` |

`gemv` takes the matrix as a flat row-major f64 vector with explicit dimensions, because Quark has no matrix type. It may change when the planned tensor type exists.

```quark
use 'std/blas' as blas

x = vector [1.0, 2.0, 3.0]
y = vector [4.0, 5.0, 6.0]
println(blas.dot(x, y))          // 32
z = blas.axpy(2, x, y)           // P7 lets 2 stand for 2.0
println(z[0])                    // 6
println(y[0])                    // 4: inputs are never modified
```

**Rules.**

- Inputs must be f64 vectors without nulls. An i64 vector, a vector with nulls, mismatched lengths, or `rows * cols != len(a)` is a runtime error through `qext::panic`. The message names the function and suggests `astype('f64')` or `fillna(...)`. This matches how built-in vector arithmetic treats size mismatches. These are programming errors, not environment failures, so under P4 they do not return `result`.
- Vectors are immutable. Routines that write their output in place (`daxpy`, `dscal`) run on a fresh copy, which is returned.
- Lengths are passed as CBLAS `int`, which is 32-bit in common OpenBLAS builds. A vector longer than `INT_MAX` elements is a runtime error.
- Threads: OpenBLAS may use worker threads during a call. They touch only the buffers passed in, never allocate GC memory, and finish before the call returns, so they don't need to be registered with the GC. Quark programs are single-threaded, so no collection runs during the call.

**Tests.**

- `smoke_blas.qrk`: each function on small vectors with known results; `axpy` and `scal` leave their inputs unchanged.
- `TestStdBlas` (`integration_ffi_test.go`) runs `smoke_blas.qrk` and one generated program per error case (length mismatch, nulls, i64 input, bad `gemv` dimensions), each of which must fail at runtime with the expected message. Skipped when OpenBLAS isn't found, like the tests that need clang++.
- Benchmark (not in CI): `dot` over 10M elements, against the same loop written in Quark.

### 5.4 SQLite: `std/sqlite`

A rehearsal for DuckDB: the same API shape as §6.3 (`open`, `exec`, `query` returning a dict of column name → vector, `close`), with handles, parent references, finalizers and `err` results. Once it works, DuckDB is mostly the chunked type conversion in §6.4.

- Uses the system library (`libsqlite3-dev` on Debian/Ubuntu). Vendoring the single-file amalgamation through `extern source` becomes possible once P2 lands.
- The detailed design will be written before Phase 2 starts. The main question to settle there: SQLite types each value, not each column, so `query` needs a rule for building a typed column (declared column type, first non-null value, or `err` on mixed types).

## 6. DuckDB

### 6.1 Why DuckDB

- It has a stable C API (`duckdb.h`) and prebuilt `libduckdb` binaries for Linux, macOS and Windows.
- Results are columnar and come in chunks, which maps onto Quark vectors with little more than `memcpy` for numeric columns.
- It covers every proposal: linking (P1), handles with a parent relationship (P3), failure as results (P4), bulk data (P5, P6).
- It pairs naturally with Quark's vectors and tables for analytics work.

### 6.2 Packaging and build

```
src/core/stdlib/duckdb.qrk        public module (Quark wrapper API)
src/core/stdlib/duckdb_glue.hpp   declarations for the externs
src/core/stdlib/duckdb_glue.cpp   implementation (P2), includes duckdb.h and quark/ext/api.hpp
```

- **The library is not vendored.** libduckdb is tens of megabytes. Users point Quark at an install with `--lib-path`/`--include-path` or `QUARK_LIB_PATH`/`QUARK_INCLUDE_PATH`. A `scripts/fetch_duckdb.sh` downloads a pinned release (`libduckdb-<os>-<arch>.zip`) into `deps/duckdb/`. As with the GC, the toolchain also checks `deps/duckdb/` automatically.
- **Linking.** Shared by default, with rpath set (P1). Static linking via `libduckdb_static.a` is possible but adds about 50 MB per binary; the opt-in mechanism is an open decision (§9.2).
- **Version.** Pin one DuckDB release (1.x) and check `duckdb_library_version()` at `open` time. Fail with a clear error on a major-version mismatch.
- `use 'std/duckdb'` without the library installed fails with `QK-FFI-002` before linking, with a hint to run the fetch script.

### 6.3 Quark API (v0.1)

```quark
use 'std/duckdb' as duck

db = unwrap(duck.open(':memory:'))          // or a file path
con = unwrap(duck.connect(db))

unwrap(duck.exec(con, "create table t as select range as i, range * 0.5 as f from range(1000000)"))

cols = unwrap(duck.query(con, "select i, f from t where i % 2 = 0"))
println(len(cols.i))        // 500000
println(sum(cols.f))

when duck.query(con, "select * from missing"):
    ok(c) -> println(c)
    err(msg) -> println(msg)   // "duckdb: Catalog Error: Table with name missing does not exist!..."

duck.close(con)
duck.close(db)
```

| Function | Returns | Notes |
|---|---|---|
| `open(path: str)` | `result` (database handle) | `':memory:'` for in-memory |
| `connect(db: resource)` | `result` (connection handle) | The connection keeps the database alive (P3 parent) |
| `exec(con: resource, sql: str)` | `result` (int rows changed) | For DDL and DML |
| `query(con: resource, sql: str)` | `result` (dict of column name → vector) | Becomes `table` once P6 lands |
| `close(h: resource)` | `null` | Idempotent; works on either handle kind |
| `version()` | `str` | `duckdb_library_version()` |

Later phases (§7): `query_params(con, sql, params: list)` using prepared statements and `duckdb_bind_*`; `insert(con, table_name, cols)` using the appender; `register(con, name, cols)` to query Quark vectors in place via Arrow (P5).

### 6.4 Result conversion

`query` runs `duckdb_query`, then converts the result one chunk at a time with `duckdb_fetch_chunk`, `duckdb_data_chunk_get_vector`, `duckdb_vector_get_data` and `duckdb_vector_get_validity`. Each column builds one Quark vector.

| DuckDB type | Quark dtype | Conversion |
|---|---|---|
| `BIGINT` | i64 | `memcpy` per chunk |
| `INTEGER`, `SMALLINT`, `TINYINT`, `UINTEGER`, `USMALLINT`, `UTINYINT` | i64 | widening loop |
| `UBIGINT` | i64 | checked; values above `INT64_MAX` return `err` |
| `DOUBLE` | f64 | `memcpy` |
| `FLOAT` | f64 | widening loop |
| `BOOLEAN` | bool | DuckDB uses one byte per value; packed into bits |
| `VARCHAR` | str | `duckdb_string_t` (inlined up to 12 bytes, otherwise a pointer); appended to the offsets and bytes buffers |
| `DECIMAL`, `HUGEINT`, `DATE`, `TIMESTAMP`, nested types | not supported | `err` naming the column and suggesting a cast, e.g. `CAST(x AS DOUBLE)` or `CAST(ts AS VARCHAR)` |

Nulls: DuckDB's validity mask is an array of `uint64_t` words, one bit per row, where a set bit means valid. A null mask pointer means every row is valid. On little-endian targets that is byte-for-byte the Arrow bitmap layout Quark uses, so a chunk whose starting row is a multiple of 8 copies its mask with `memcpy`. Other chunks use a bit-shifting copy. Chunk sizes vary (up to `duckdb_vector_size()`, usually 2048), so both paths are needed.

Allocation: the total row count isn't known up front, so column buffers grow geometrically, using the existing vector builders' growth policy. All copying happens on the calling thread. DuckDB's worker threads never see GC memory during a query, so they don't need to be registered with the GC.

### 6.5 Handles and lifetime

| Handle kind | Payload | Close | Parent |
|---|---|---|---|
| `duckdb.database` | `duckdb_database*` (malloc) | `duckdb_close` | none |
| `duckdb.connection` | `duckdb_connection*` (malloc) | `duckdb_disconnect` | database handle |

Results and data chunks never become handles. They are converted and destroyed inside the native call (`duckdb_destroy_result`, `duckdb_destroy_data_chunk`), so nothing native leaks into Quark values.

### 6.6 Tests

- Integration tests are skipped when libduckdb is not found, the same way they're skipped when clang++ is missing.
- `smoke_duckdb.qrk`: open, exec, query, close; a type-matrix query with one column per supported type; nulls in every dtype; results over 2048 rows so they span chunks; a filtered query, whose chunk boundaries aren't multiples of 8; error results for bad SQL and unsupported types; use after close (expects `QK-FFI-004`).
- A handle finalizer test: open a connection without closing it, force a GC, and check through a debug counter in the glue that `duckdb_disconnect` ran.
- Missing-library diagnostics are covered by the test library (§5.2), plus one case checking the fetch-script hint.
- Benchmark (not in CI): importing a 10M-row `BIGINT` column should run close to `memcpy` speed.

## 7. Rollout

| Phase | Contents | Done when |
|---|---|---|
| 1. Linking | P1, P7, P8 (build codes) | The `libqffi_test` cases pass (§5.2) and `smoke_blas.qrk` passes on Linux (§5.3). **Done 2026-11-04.** |
| 2. Handles and results | P2, P3, P4, P8 (`QK-FFI-004`) | `std/sqlite` `open`/`exec`/`query` (dict result)/`close` pass a smoke test, including a finalizer test (§5.4) |
| 3. DuckDB | `std/duckdb` | `open`/`connect`/`exec`/`query` (dict result)/`close` pass `smoke_duckdb.qrk` (§6.6) |
| 4. Tables and parameters | P6, `query_params`, `insert` | `query` returns `table`; parameterized queries; bulk insert via the appender |
| 5. Arrow | P5 | `register` queries a Quark table from DuckDB without copying; `import_vector` is used for Arrow-producing providers |

Phase 1 is independent of DuckDB and useful on its own: any library with a C API becomes usable through a small glue header. P2 moved to Phase 2: no Phase 1 provider needs it, and caching glue objects correctly needs header dependency tracking (an object depends on every header its `.cpp` includes, not only its own content), which is more than Phase 1 should carry.

## 8. Impact on the Project

| Area | Change | Compatibility |
|---|---|---|
| Lexer | None (contextual words) | — |
| Parser / AST | `ExternLinkNode`, `ExternIncludePathNode`, `ExternCppSourceNode`; existing `ExternSourceNode` renamed `ExternHeaderNode` | Additive; the rename is internal |
| Loader | Resolve the new path directives | Additive |
| Analyzer | None for P1/P2. P6 adds the plain `table` return annotation if it doesn't exist yet. | Additive |
| IR / codegen | None | — |
| Driver | `Unit.Native` (collected dependencies with source locations) | Additive |
| Toolchain | Search paths, library resolution, rpath, glue object cache | Additive; programs without directives build exactly as today |
| CLI | `--lib-path`, `--include-path`, `--link-arg` | Additive |
| Runtime: resources | Dynamic kinds; parent field; handle allocated scanned; finalizers | Internal. File handles keep their kind and behavior. |
| Runtime: `ext/api.hpp` | Handle, result and table helpers | Additive |
| Runtime: new `ext/arrow.hpp` | Arrow C Data Interface bridge | Additive |
| Runtime: `q_as_float` | Accepts `int` | Relaxation only |
| Stdlib | `std/blas` (needs OpenBLAS), then `std/sqlite` and `std/duckdb` (each needs its library) | Additive; programs that don't `use` them are unaffected |
| Docs | `semantics.md` §10.6, `architecture.md` §7–8, `error_codes.md`, `stdlib.md`. | — |

Existing extensions (`std/fmt`, `smoke_extern.qrk`) keep working unchanged.

## 9. Decisions

### 9.1 Decided

**D1. Directive syntax: one directive per line** (2026-11-04). `extern link 'x'`, `extern include_path 'dir'` and `extern source 'x.cpp'` each stand alone, next to the existing `extern '<path>'` (P1).

- Rejected: a block per library, such as `extern lib 'duckdb':` with `include`, `source` and `header` entries.
- Reasons:
  - It matches the existing one-line `extern '<path>'` form.
  - It is the cheapest to parse: one more branch in `parseExtern`, no indented block.
  - One fact per line keeps duplicate removal and error locations simple.
  - v0.1 needs no per-library options.
  - Flat directives work at scale in Nim (`{.passL.}`, `{.compile.}`) and Go's cgo (`#cgo LDFLAGS:`).
  - A block would add a second way to include a header, and it invites `extern fn` declarations inside it, which would bring back the `ffi lib` design §3 dropped.
- Costs accepted: nothing groups a library's pieces, and future per-library options will be words after the string.
- Mitigations: per-library records internally, and the words after the string are reserved (P1), so a block form can still be added later.
- Revisit when static linking, platform-specific libraries (`-framework`, Windows `.lib` names) or `pkg-config` lookup are needed.

**D2. `extern include_path`, not `extern include`** (2026-11-04). `extern '<path>'` already `#include`s a header, so `extern include` would read the same way and be misused as `extern include 'duckdb.h'`.

**D3. Provider order** (2026-11-04): `libqffi_test` → CBLAS (`std/blas`) → SQLite (`std/sqlite`) → DuckDB (`std/duckdb`), as in §5.1. DuckDB needs nearly every proposal at once and is not a good first proof of concept. CBLAS is the first real library because it is small, widely installed, and shows Quark's main case: native code over Quark vectors without copying.

### 9.2 Open

1. **Static DuckDB.** Should static linking be offered through a flag (`--static-lib duckdb`) or a directive option, or left for later? A directive option would use the reserved words after the string (P1).
2. **Fetching libduckdb.** Is a fetch script into `deps/duckdb/` acceptable, or should Quark require a user-installed library only?
3. **Unsupported column types.** For `DECIMAL` and temporal types: return an error with a cast hint (proposed), or convert automatically (`DECIMAL` → f64, `TIMESTAMP` → i64 microseconds) and document the precision loss? The same choice applies to SQLite.
4. **Named handle kinds.** Is plain `resource` enough for v0.1 (proposed), or should `resource[duckdb.connection]` annotations come first?
5. **`query` before tables land.** Ship `query` returning a dict of vectors in phases 2 and 3 (proposed), or wait for P6 so the API never changes?

## 10. Security

Native code runs with full process privileges; Quark's runtime checks don't sandbox it. Glue code must follow the `qext` rules: no GC pointers in handle payloads (P3), no mutating vectors after they have been returned (vector immutability), and every native allocation must have exactly one owner. Third-party providers are trusted code. Documentation for `extern link` must say so.
