# Known issues

Bugs in the compiler, runtime, standard library and tools that are not fixed yet, each with a reproduction. Last checked against the compiler at `c92a964` (2026-11-13, after the `lowering-ir` merge). The structural causes behind many of them, and the plan for them, are in `architecture.md` §12-14.

This file lists bugs only. Deliberate gaps (features that don't exist yet) are in README.md, Known limitations; what changed and what was fixed is in `changelog.md`.

## Contents

- [Adding an issue](#adding-an-issue)
- [Wrong results with no error](#wrong-results-with-no-error)
- [Fails loudly, but with a C++ error, a wrong location or a misleading message](#fails-loudly-but-with-a-c-error-a-wrong-location-or-a-misleading-message)
- [Slow paths](#slow-paths)

## Adding an issue

A bug found while working on something else is recorded here and not fixed in the same run (CLAUDE.md, Robustness Bar); known issues are fixed in runs of their own. An entry has:
- **What is wrong**, in one sentence, under the heading that matches its effect: wrong results with no error (soundness bugs) first.
- **A reproduction**: the smallest program, what it does, and what it should do.
- **Where it was found**: the date and the work it turned up in, and the cause if known (file and function).
- **Reach**, when a change made it easier to hit: which change, and how.

When a change fixes an issue, delete its entry here and describe the fix under Fixed in `changelog.md`.

## Wrong results with no error

**The lexer accepts an unclosed string.** It ends at the end of the line with no error: `msg = "hello` then `print(msg)` prints `hello`.

**Structs and extern methods.**
- `==` on structs reached through an untyped path is always `false`, with no error: `struct P:` / `x: int`, `fn id(v) -> v`, `a = P { x: 1 }`, then `print(id(a) == id(a))` and `print(list [a] == list [a])` both print `false`. A direct `a == a` is a compile error (structs have no `==`).
- Extern methods with one receiver and name collide silently and leak across files. If `e1.qrk` declares `extern fn str.shout() str as 'h1_shout'` and `e2.qrk` declares the same method as `'h2_shout'`, a script that includes both can call `'a'.shout()` unqualified and gets e2's function, while `e1::one('a')` (which calls `s.shout()` inside e1) gets h1's. Declaring the same method twice in one file is accepted too. Methods are registered by `(receiver, name)` in one table (`a.methods`, `types/analyzer_modules.go`), and the loader doesn't resolve them per file.

**Stdlib lookup and `print`.**
- Any directory named `stdlib` in the script's directory or a parent is used as the standard library, before the real one and with no warning (`findStdlibRoot`, `loader/loader.go`). With `/tmp/x/stdlib/io.qrk` present, `incl 'std/io'` in `/tmp/x/a/main.qrk` silently loads that file; without one it fails with `cannot find module 'std/io': file '/tmp/x/stdlib/io.qrk' does not exist`.
- A misspelled `print` alignment is ignored: `print('x', '|\n', 5, 'rigth', '.')` prints `x....|` (left-aligned) with no error.

**Runtime numbers and edge cases.**
- List indexes in methods are cut to 32 bits: `list [10, 20, 30].get(4294967296)` returns `10`, and `xs.set(4294967297, 99)` writes element 1 (the same for `insert` and `remove`). `xs[i]` and `s[i]` take the whole index.
- `abs(-9223372036854775807 - 1)` returns `0`.
- `to_int('99999999999999999999')` returns `9223372036854775807`. `to_int(1e19)`, `floor(1e30)`, `round(-1e300)` and `fillna(1e30)` on an int vec give arbitrary values; `fillna(2.7)` on an int vec stores `2`.
- `range(0.5, 3.0)` is `[0, 1, 2]`. `range(9223372036854775800, 9223372036854775807, 5)` never ends, and a lowered `for` loop over it wraps to negative numbers.
- `d.set(1, 'a')` and `d.set('1', 'b')` write the same key: `d` prints `{'1': 'b'}`.
- `sys::exit(256)` exits with status 0.
- `io::read` stops at a NUL byte: on a file containing `ab\0cd`, the first read returns `ab` and the next returns `''`.
- Two-argument `min`/`max` with NaN depend on the order: `min(n, 1.0)` with `n = to_float('nan')` is `1.0`, but `min(1.0, n)` is `nan` (vec reductions skip NaN). Sorting a list that contains NaN is undefined behavior in the C++ sort.

**Unbounded recursion crashes with no message.** `print('before')`, `fn f(n) -> f(n + 1)`, `f(0)` overflows the C++ stack: the binary dies with a segmentation fault (status 139), `quark run` exits with 255 and prints nothing, not even `before`, since buffered output is lost. It should be a runtime error with a location, and output printed before it should appear.

## Fails loudly, but with a C++ error, a wrong location or a misleading message

- A struct named like a builtin type is accepted, and the annotation then means the builtin: `struct int:` / `    x: int`, then `p: int = int { x: 1 }` is `cannot assign value of type 'int' to 'int'`, and `struct vec:` is a parse error (`expected struct name after 'struct'`). It should be a clear error at the `struct` (`'int' is a builtin type; choose another name`). Found 2026-11-13 while adding the `tbl` type (tables v0.1). Reach: `tbl` is now a builtin type name, so a program with `struct tbl:`, which worked before, now gets these messages where it uses `tbl` as an annotation.
- A prelude builtin used as a value passes the checker and fails in clang: `f = len`, then `print(f('ab'))`, or `print(len)`, gives `use of undeclared identifier 'quark_len'`. Extern fns used as values work (they have thunks).
- `struct S` inside a `fn` body fails with `internal: struct 'S' not pre-declared`.
- A call written above a `fn` declaration is checked less than one below it. `print(apply(fn(a, b) -> a + b))` above `fn apply(k) -> k(1)` is a runtime error, below it a compile error.
- Calling a function value with more than 12 arguments compiles and always fails at runtime (`function call supports up to 12 arguments, got 13`); it could be a compile error.
- A struct field named with `__` (`a__b: int`) is accepted in the `struct` and rejected only where a literal uses it.
- Every lexer error after `=` is reported twice: `x = @` gives `unexpected character '@'` and then `unexpected '@' after '='; put each statement on its own line, or join the two with an operator`, and `x = 1__0` repeats the digit separator message inside a second one. The parser reports the `ILLEGAL` token as the expression error (`parseExpression`) and again when the statement doesn't end there (`endLine`, `parser/parser.go`). Found 2026-11-13 while adding digit separators (tables v0.1), whose error goes through the same path. Reach: digit separators add one more lexer error that hits it.
- A syntax error in an imported file is reported inside a load error located at the importer's `incl` line. Load errors in the main file show its absolute path, while check errors show the path as written. Several messages name token kinds instead of source text (`operator 'PLUS'`, `expected identifier after OK`, `no prefix parse function for NEWLINE`), and a multi-line `g = fn(x) ->` body gives about ten cascading `DEDENT` errors.
- Native code that appends to a list with `push_back`, or writes `d->entries` directly, skips the element-type check; `qext::list_push` and `qext::dict_set` check it.
- `fmt::table(t)` shows a missing value as `NA`, while `print(t)` and vec printing show `null`: with a `tbl[Emp]` read from `id,salary` / `1,` / `2,5`, `print(t)` has `null` in the salary column and `fmt::table(t)` has `NA` (`q_fmt_vector_cells`, `stdlib/fmt.hpp`). Found 2026-11-14 adding table printing (tables v0.1). Reach: tables are now reachable from Quark, so `fmt::table` on one is too.
- The editor extension's hover and completion show builtin method signatures without their first parameter: `xs.slice(start, end)` is shown as `slice(int) list`, `s.replace(old, new)` as one parameter too, and the new `t.head(n)` as `head()`. `catalogSignature` (`internal/editordata/editordata_test.go`) drops `ParamTypes[0]` and lowers `MinArgs` for methods, as if the catalog listed the receiver among the parameters, which it doesn't. Found 2026-11-14 regenerating the editor data for the table methods (tables v0.1).
- Tooling: the integration tests share one binary, `src/core/quark/quark_test`, which `TestMain` deletes at the start and end of every run. A second `go test` in the same checkout (another session, an editor) deletes it mid-run, and every integration test then fails with `fork/exec .../quark_test: no such file or directory`. Each run should build into its own temporary directory.
- A field read on a struct of unknown type that lacks the field says `dot access is only supported on dict; got type 'struct'` (`fn id(v) -> v`, `p = id(P { x: 1 })`, `p.y`).
- A wrong argument type for a fully annotated function or an `extern fn`, from an untyped value, reports `expected int, got str` without the function or parameter name (ordinary functions say `argument 'x' of 'f' expects int, got str`).
- Stdlib runtime errors use the old `.` spelling: `io::read` on a closed file says `io.read() expects an open file (resource is closed)`; `random.randint` and `blas.dot` likewise.
- `qext::box(int)` in native code doesn't compile (ambiguous between the `int64_t`, `double` and `bool` overloads); pass an `int64_t`.
- A program that imports `std/blas` starts OpenBLAS's thread pool (one thread per CPU), and then allocating large vecs makes the GC print `GC Warning: Repeated allocation of very large block (appr. size 78128 KiB): May lead to memory leak and poor performance` to stderr, and vec allocation gets erratic (`2.5 * a + b` on 10M-element vecs with `blas::axpy` measured 30 ms with `OPENBLAS_NUM_THREADS=1`, 135 ms without). The same program without `incl 'std/blas'`, or with `OPENBLAS_NUM_THREADS=1`, prints nothing (`src/benchmarks/RESULTS.md`).

## Slow paths

- Sorting a table by a column with missing values takes the generic comparator: 500K rows by one float key with 1% of it missing sort in 239 ms, against 97 ms for a key with no missing values (`sort_perm`, `table/engine.hpp`, takes its fast path only when `null_count == 0`). The fast path could sort the present values and put the missing rows last. Found 2026-11-14 measuring the tables v0.1 queries. Reach: `sort(...)` in a query reaches it for every nullable key, which CSV columns with gaps are.
