# Changelog

Notable changes to the Quark language, compiler, runtime, stdlib and tooling.

Quark has no tagged releases yet, so entries are grouped by date and by piece of work, newest first. Each entry lists the commits it covers. Entries were written from the diffs, tests and smoke programs, not from the commit messages, which are often vague (`huge chunk of changes`, `minor QOL improvments`) or misleading (`fix returns inside if` fixes scalar assignment codegen; Quark has no `return`).

Sections follow [Keep a Changelog](https://keepachangelog.com/): **Breaking**, **Added**, **Changed**, **Fixed**, **Removed**, **Internal**. "Breaking" covers anything that can make a program that worked before fail to compile, fail at runtime, or print something different.

Bugs that are not fixed yet are in [known_issues.md](known_issues.md), not here; an entry's Fixed section says when one is fixed.

## Contents

- [2026-11-14: Table queries, table printing and table methods (tables v0.1)](#2026-11-14-table-queries-table-printing-and-table-methods-tables-v01)
- [2026-11-13: The `tbl` type, `std/csv` on the table reader, digit separators (tables v0.1)](#2026-11-13-the-tbl-type-stdcsv-on-the-table-reader-digit-separators-tables-v01)
- [2026-11-13: A statement ends at the end of its line; indented pipe lines](#2026-11-13-a-statement-ends-at-the-end-of-its-line-indented-pipe-lines)
- [2026-11-13: Known issues move to `known_issues.md`](#2026-11-13-known-issues-move-to-known_issuesmd)
- [2026-11-13: Benchmark suite; `std/blas` gemm, syrk and transposed gemv](#2026-11-13-benchmark-suite-stdblas-gemm-syrk-and-transposed-gemv)
- [2026-11-13: Vec reductions follow the table aggregates; `std/stats` (tables D13)](#2026-11-13-vec-reductions-follow-the-table-aggregates-stdstats-tables-d13)
- [2026-11-13: A missing vec entry taken out as one value is an error (tables D12)](#2026-11-13-a-missing-vec-entry-taken-out-as-one-value-is-an-error-tables-d12)
- [2026-11-13: New back end: lowering pass, typed IR and verifier (architecture §13)](#2026-11-13-new-back-end-lowering-pass-typed-ir-and-verifier-architecture-13)
- [2026-11-13: Docs overhaul](#2026-11-13-docs-overhaul)
- [2026-11-13: Element types are fixed, variables don't narrow, return annotations are checked (architecture §12)](#2026-11-13-element-types-are-fixed-variables-dont-narrow-return-annotations-are-checked-architecture-12)
- [2026-11-13: Table engine and CSV reader (runtime only)](#2026-11-13-table-engine-and-csv-reader-runtime-only)
- [2026-11-13: Variable storage decided by the analyzer (architecture §12.8 step 2)](#2026-11-13-variable-storage-decided-by-the-analyzer-architecture-128-step-2)
- [2026-11-12: Function values, `fn` type, call-shape checking, keyword renames, struct field checks, name clashes, `input()` result, fixed `fn` declarations, modules v0.2, compiler cleanup](#2026-11-12-function-values-fn-type-call-shape-checking-keyword-renames-struct-field-checks-name-clashes-input-result-fixed-fn-declarations-modules-v02-compiler-cleanup)
- [2026-11-11: Value printing, float formatting, `**` precedence, parameter enforcement](#2026-11-11-value-printing-float-formatting--precedence-parameter-enforcement)
- [2026-11-11: Core cleanup; file I/O moved to `std/io`](#2026-11-11-core-cleanup-file-io-moved-to-stdio)
- [2026-11-10: Module namespacing, dynamic methods, sorting, new stdlib modules](#2026-11-10-module-namespacing-dynamic-methods-sorting-new-stdlib-modules)
- [2026-11-06: Vendored Boehm GC, VS Code extension 0.1.0](#2026-11-06-vendored-boehm-gc-vs-code-extension-010)
- [2026-11-04: FFI phase 1 and `std/blas`](#2026-11-04-ffi-phase-1-and-stdblas)
- [2026-10-07: Compilation pipeline rewrite](#2026-10-07-compilation-pipeline-rewrite)
- [2026-10-06: Arrow-compatible vectors](#2026-10-06-arrow-compatible-vectors)
- [2026-10-06: Structural equality, overflow checks, stricter conditions, `all`/`any`](#2026-10-06-structural-equality-overflow-checks-stricter-conditions-allany)
- [2026-04-03: Table runtime scaffolding; struct fixes](#2026-04-03-table-runtime-scaffolding-struct-fixes)
- [2026-04-02: Structs v0.1](#2026-04-02-structs-v01)
- [2026-03-28: Vector soundness, `std/fmt` on `extern`, build output](#2026-03-28-vector-soundness-stdfmt-on-extern-build-output)

---

## 2026-11-14: Table queries, table printing and table methods (tables v0.1)

Completes `tables_v0_1_spec.md` §9, the first implementation of tables, on branch `worktree-tbl-v01`. Four decisions were taken while implementing it (spec §2): `and`/`or` follow SQL's three-valued logic with missing values (D22), the columns of one `mutate` are computed in order (D23), a summarize column is one aggregate of one column (D24), and inspection is table methods rather than prelude functions (D25).

Commits: `e16a54a`

### Added
- **Queries** (semantics.md §9.5, `grammar.md` §10.5): `t | e -> verb(...) | verb(...)` with the verbs `filter`, `mutate`, `sort`, `group_by` + `summarize`, `head` and `tail`. The compiler follows the row type through every step, so every column a query reads, and its type, is checked at compile time; each step compiles to a loop over the column buffers.
  ```quark
  stats = staff | e ->
      filter(e.salary >= 100_000 or e.dept == 'eng')
      | mutate(bonus = e.salary * 0.1, total = e.salary + e.bonus)
      | group_by(e.dept)
      | summarize(avg = mean(e.salary), top = max(e.bonus), n = count())
      | sort(desc(e.avg))
  ```
  - `filter(cond)` keeps the rows where the bool `cond` is true. `mutate(name = expr, ...)` adds columns at the end or replaces one in place (its type may change), in the order written, each seeing the ones before it. `sort` takes columns, each optionally `desc(...)`, and is stable with missing values last. `group_by(cols)` must be followed by `summarize`, which gives one row per key in key order (missing keys last) with the keys first; without `group_by`, `summarize` gives one row. Aggregates are `count()`, `count(e.col)`, `sum`, `mean`, `min` and `max`, each of one column. `head(n)`/`tail(n)` keep the first or last rows (6 by default).
  - A verb expression may use arithmetic, comparisons, `and`/`or`/`!`, `x if c else y`, outer variables and calls of builtins, methods and functions with known return types. The row name shadows an outer variable of the same name, may only be used as `e.col`, and can't be read from a function written inside a verb.
  - A query's result has an anonymous row, shown by its shape (`tbl[{dept: str, avg: float, top: float, n: int}]`), which matches a struct of exactly that shape where it is stored: `s: tbl[DeptStats] = staff | e -> ...`.
  - Errors the compiler reports include an unknown verb or column (with the columns listed), a query on a table whose row type it doesn't know (`a query needs a table whose row type is known ...; annotate it (t: tbl[Employee])`), a non-bool condition, a `group_by` not followed by `summarize`, an expression where a sort key, group key or aggregate argument must be a column (`sum aggregates a column (e.col), not an expression; compute the value with mutate first`), `max(e.x) - min(e.x)` as a summarize column, duplicate column names, and a column of a type a table can't hold.
- **Missing values in queries** (§5): an expression is computed for a row only when the columns it reads are present, and is otherwise missing (a function it calls isn't called for that row); `and`, `or`, `!` and ternaries follow SQL's three-valued logic, so `filter(e.dept == 'Sales' or e.salary > 100)` keeps a Sales row whose salary is missing; a missing `filter` condition drops the row; aggregates skip missing values and NaN. Their operands and a ternary's condition must be bools (`in a query, 'and' takes bools, got str (write a comparison, such as e.name != '')`).
- **Printing a table** shows its type and row count, the column names and types, and the first 10 rows (`fmt::full(t)`: all), a missing value as `null`; inside a list it prints `<tbl[Emp], 5 rows>`:
  ```
  tbl[Emp]: 5 rows
   id  name  dept    salary
  int  str   str      float
    1  Ann   eng   120000.0
    2  Bo    ops    90000.5
    3  Cy    eng       null
  ```
- **Table methods** (D25): `t.head(n = 6)` and `t.tail(n = 6)` return a table of the same type, and `t.describe(n = 5)` prints each column's name, type and first values.
- A table can't be compared with `==` or `!=` (`operator '==' is not defined for tables; compare their columns ...`) or used as a condition (`a table cannot be used as a condition; test a column, such as len(t.col) > 0`), at compile time and, through `any`, at runtime.

### Internal
- `ast`: `QueryNode`, `QueryStepNode`, `ColumnDefNode`, `AggregateNode`, `SortKeyNode` (`TreeNode.Descending`). The parser makes verb, column and aggregate names and `desc` node tokens, so the loader never resolves them; the loader binds the row name in the steps like a parameter (`walkQuery`), renaming it in imported files.
- `types/analyzer_query.go`: `analyzeQuery` records a `types.Query` per query (steps with their input and output rows, mutate columns, keys, aggregates) and the column index of every `e.col` (`Analysis.Queries`, `ColumnRefs`); the row name is a `SymRow` symbol, which has no C++ variable. Derived rows are interned by shape (`anonRow`) and emitted as static `QStructDef`s.
- `lower/query.go`: queries lower to `QTable*` temporaries and engine calls; filter and mutate are `ForIndex` loops over bound columns, with strict parts guarded by `q_tbl_present` and three-valued parts lowered first (`rowLogic`, `rowTernary`, `lowerer.rowSubst`). Runtime: `table/query.hpp` (`RowSet`, `ColOut`, `q_tbl_*`, the table methods), included only when used: a catalog builtin may name its header (`builtins.Spec.Header` → `CallPlan.RuntimeHeader`). `INV-QUERY`, `INV-QUERY-REP` and `INV-QUERY-COLUMN` are new internal error codes.
- Measured on 500K rows (`architecture.md` §15): a two-column numeric `mutate` 10 ms, `group_by` + `summarize` 27 ms, a filter with a string comparison 38 ms (a string allocated per row), a sort by a nullable key 239 ms (the engine's slow path, `known_issues.md`).
- Tests: `TestTables_Query`, `TestTables_QueryRejected`, `TestTables_Inspect`, `TestTables_ValueRejected`, and `smoke_tables.qrk`. The editor grammar highlights the table methods, and its data has them.
- `known_issues.md` gains three issues found here: sorting by a nullable key is slow, `fmt::table` shows missing values as `NA` while `print` shows `null`, and the editor's method signatures drop their first parameter.

## 2026-11-13: The `tbl` type, `std/csv` on the table reader, digit separators (tables v0.1)

The first half of `tables_v0_1_spec.md` §9, on branch `worktree-tbl-v01`; queries followed on 2026-11-14.

Commits: `2785a46`, `54f38cf`

### Breaking
- **`std/csv` is rebuilt on the table reader** (D15): `csv::parse(text, sep)` and `csv::read(path, sep)`, which returned lists of str lists, are removed. Use `csv::parse_text(text)` and `csv::read_file(path)`, which return `ok(table)` (below); a `;`-separated text has no form yet (a path ending in `.tsv` or `.tab` is tab-separated).
- `tbl` is a type name: a program with its own `struct tbl:` now gets confusing messages where it writes `tbl` as an annotation (`known_issues.md`); rename the struct.

### Added
- **Digit separators** (`grammar.md` §3.3): `_` may stand between two digits of a number, in any part of it, and is ignored: `100_000`, `1_000.5`, `0.000_1`, `1e1_0`. Anywhere else in a number it is an error: `x = 1__0` is `'_' in a number must be between two digits: '1__0' (write 100_000)`, as are `1_`, `1_.5`, `1._5` and `1_e5`. `_1` is still a name.
- **The table type** (D10, D20): `tbl[Employee]` is a table whose rows are `Employee` structs (every field an `int`, `float`, `bool` or `str`), and a bare `tbl` is a table of any row. `t.salary` on a `tbl[Employee]` is the column as a `vec[float]`; an unknown column is a compile error that lists the columns (`tbl[Emp] has no column 'wage' (columns: id, name, dept, salary)`), and so is a column of a bare `tbl` (`the row type of this table isn't known`). `null` is not a table. `type(t)` is `tbl[Employee]`, or the shape for a table whose row has no name: `tbl[{id: int, name: str}]`.
  ```quark
  incl 'std/csv'

  struct Employee:
      id: int
      name: str
      salary: float

  staff: tbl[Employee] = unwrap(csv::read_file('staff.csv'))
  print(sum(staff.salary))
  ```
- **A table with an anonymous row matches a named one by shape** where it is stored (§3.2-3.4): its columns must be the struct's fields exactly (names and types, in any order; an int column fills a float field), and it is relabeled with the struct and its columns put in field order. The compiler checks what it can see and names the difference (`cannot assign value of type 'tbl[Emp]' to 'tbl[Emp2]': Emp and Emp2 are different structs`); a table whose row it doesn't know (a bare `tbl`, a value of type `any`) is checked when it reaches a `tbl[R]` parameter, variable, field or return value: `variable 'v' is declared tbl[Emp] but got tbl[{id: int, name: str}]`.
- **`csv::read_file(path)` and `csv::parse_text(text)` are typed by where their table goes** (D18): with `staff: tbl[Employee] = unwrap(csv::read_file('staff.csv'))`, a `tbl[Employee]` parameter, struct field or return annotation, the file is read with `Employee`'s columns and types, and a bad cell names its line and column (`staff.csv:3: column 'salary': '12k' is not a float`). Elsewhere every column's type is inferred from its values (int, else float, else bool, else str). Empty and `NA` cells are missing values in every column.
- **A column mapping** (D17): `csv::read_file('emp.csv', dict { id: 'EmpID', salary: 'Salary ($)' })` reads fields from header columns with other names. The mapping must be a dict literal of string literals; its fields are checked against the row struct at compile time. A field missing from the header says how to map it: `emp.csv: no column named 'id' in the header (columns: EmpID, name); map field 'id' to one of them with dict { id: '...' }`.
- **`csv::write_file(t, path)`** (a header line, missing values as empty fields; tab-separated for `.tsv` and `.tab`) returns `ok(null)` or `err(message)`, and **`csv::format_text(t)`** returns the same text.
- **CSV text must be UTF-8**: a str cell or header name that isn't valid UTF-8 (`emp.csv:2: column 'name' is not valid UTF-8 (the file may be Latin-1 or another encoding)`) or holds a NUL byte is an error, so a Latin-1 `José` can't silently fail to equal `'José'`.
- An empty `dict {}` may be a parameter default, like `list []`, and is a new dict on every call.
- An `extern fn` may take and return a bare `tbl`, as a `QTable*` (FFI P6); `tbl[R]` in an extern signature is a compile error.

### Fixed
- A struct or table passed to a native function that expects another type was reported as `expected int, got unknown`; it now names `struct` or `tbl`.
- Text after a closing quote in a CSV field is an error (`text:2: unexpected text after a closing quote`); the old `csv::parse('a,"b"x,c')` returned `[['a', 'bx', 'c']]`.
- A NUL byte in a CSV str cell is an error; the reader used to keep it in the column while a value taken out of it was cut short at the NUL.

### Internal
- `lexer.readNumber` reads digit runs with separators and drops them from the literal, so later stages see `100000`. The editor grammar highlights separators and exponents (`1e3` was highlighted as `1`).
- Types: `types.TableType{Row}` (nil row for `tbl`); `StructType.Anonymous` for derived rows, one per shape (`row__N`), shown as their shape; `RowMatch` is the shape rule, used by `CanAssign` (a conversion, like int to float) but not `IsSubtype`, so containers never hold a relabeled table. `annotationType` is the one annotation resolver behind `resolveTypeNode` and the error-free `resolveTypeNodeStatic`; a `TypeNode` may have one type argument (the parser reads `name[T]`, the checker accepts it on `tbl` only). Struct field types are resolved twice, so a `tbl[R]` field sees `R`'s fields whatever the definition order.
- Expected types: the analyzer records the type a node's value is stored as (`Analyzer.expected`: typed declarations, assignments to typed variables, arguments of functions with known signatures, struct literal fields, the last statement of a body with a return annotation; `unwrap` passes `result[T]` to its argument). The std/csv reads are recognized by their definition (`names.Origin` in `std/csv.qrk`) and recorded in `Analysis.TableReads`; lowering calls the native reader (`q_csv_read_file`, `q_csv_parse_text`) with the row's `QStructDef`, which now carries each field's column type (`field_dtypes`) and an `anonymous` flag.
- Runtime: `q_table_conform` is the shape rule at runtime, behind `q_param_table`, `q_var_table`, `q_field_table` and `q_ret_table` (IR `CheckTable`); element descriptors check tables by row definition. `q_member_get` reads a column of a table of unknown type by name. The reader takes a header name per schema column (`csv::Column::header`) and renames for untyped reads (`Options::rename`), and validates text (`csv::text_problem`). A function whose signature has a `tbl[R]` keeps the QValue convention: a `QTable*` can't carry the row check.
- Tests: `integration_tables_test.go` (`TestTables_TypeRejected`, `TestTables_CSV`, `TestTables_CSVRejected`, parallel subtests), new cases in `src/testfiles/table/csv_test.cpp` (UTF-8 edge cases, NUL, mapping), `smoke_csv.qrk` on the new API.

## 2026-11-13: A statement ends at the end of its line; indented pipe lines

Commits: `38f4d84`

### Breaking
- **Anything after a complete statement on its line is a parse error** (`grammar.md` §4). The parser ran it as another statement, so a missing operator went unnoticed:
  ```
  total = price qty     // was: total = price, then qty; now: unexpected 'qty' after 'price'; put each statement on its own line, or join the two with an operator
  y = 'a' 'b'           // was: 'a'
  fn f(x) -> x + 1 print(9)   // was: a two-statement body, f(1) printed 9 and returned null
  ```
  The same holds for a `when` arm (`1 -> 'one' 'two'`, which gave `expected ARROW but got NEWLINE`) and a struct field: `struct P:` / `x: int y: int` declared two fields, and is now `unexpected 'y' after 'int'; put each field on its own line`.

### Added
- **A pipe chain can continue on indented lines** (`grammar.md` §3.4, §10.4). A line starting with `|` that is indented deeper than the line it continues was `unexpected indent`; the same-indentation form keeps working, and comment and blank lines may come between:
  ```
  y = 5
      | double()
      | inc()
  ```

### Changed
- Clearer errors for the pipe lines that can't continue an expression: a line ending with `|` is `a line can't end with '|'; start the next line with it instead` (it was `no prefix parse function for NEWLINE`), and a `|` line indented less than the expression it would continue, or following an expression that ends with an indented block, is `'|' can't start a statement; ...` (it was `no prefix parse function for PIPE`).

### Fixed
- A `fn` declaration whose body is an inline `when` no longer swallows the next statement: after `fn pick(x) -> when x:` / `    1 -> 'one'` / `    _ -> 'other'`, a following `print(pick(1))` became part of `pick`'s body, so the program printed nothing. The same for `fn h(x) -> y = when x:` and an inline `when` in an `if` or `for` body (`if false: when 1:` and its arms, then `print('after')`, printed nothing). An inline block is one statement, as documented.

### Internal
- `parser.endStatement` / `endLine` run after every statement (top level, indented and inline blocks), `when` arm and struct field: the next token must end the line, the block or the file, unless the statement ended with a block of its own (the last token read was a `NEWLINE` or `DEDENT`). `parseBlock`'s inline form parses one statement. The lexer passes a deeper line that starts with `|` through without an `INDENT` (`indentationFilter`), and the parser's existing `NEWLINE` + `|` continuation joins it. The editor snippet test expands a placeholder with no default (`$0`) to nothing, as the editor does, instead of `null`, which only parsed because of the separator bug. Tests: `parser/statement_end_test.go`, `TestIndentation_DeeperPipeLineContinues`, `TestStatementEnd_Programs`, `TestSemanticsSafety_Rejected` (`no_separator*`, `pipe_*`).
- `known_issues.md` drops the two entries fixed here (statements on one line, the inline `when` body); the unclosed string stays. `tables_v0_1_spec.md` §9 has no prerequisite left before the `tbl` compiler work.

## 2026-11-13: Known issues move to `known_issues.md`

Commits: `f47aef1`

No compiler, runtime or stdlib changes.

### Docs
- **`known_issues.md`** (new) holds the bugs not fixed yet, each with a reproduction, moved unchanged from the top of this file, plus how to add one: what is wrong, a reproduction, where and when it was found, and its reach when a change made it easier to hit. This file is now only the history.
- **CLAUDE.md, Robustness Bar**: a bug found outside the task at hand is recorded in `known_issues.md` and reported at the top of the final summary, not fixed in the same run (new rule 7; known issues are fixed in runs of their own). A hole a change makes easier to hit is recorded with its reach and reported first, for the user to decide (rule 3; it used to require fixing it in the same change or stopping to ask). An issue that prevents a clean implementation of the task stops the work: the agent explains it with a reproduction, the options and their trade-offs (with code snippets) and a recommendation, instead of working around it (new rule 8). The Changelog section and the docs table point to the new file.
- Every reference to "Known issues in changelog.md" now points to `known_issues.md` (README, semantics, grammar, stdlib, architecture, and the FFI, structs and modules specs). Three sentences that pointed at issues fixed by the `lowering-ir` merge are corrected: the README said changing a list during a `for` loop over it isn't checked (it is a runtime error), `src/benchmarks/README.md` said a missing dict key in the join reads as `0.0` (it is a runtime error), and `src/benchmarks/RESULTS.md` called the `+` typing bug open.

## 2026-11-13: Benchmark suite; `std/blas` gemm, syrk and transposed gemv

Commits: `41b8279`, `cbb3ea7`, `c92a964`

### Breaking
- `blas::blas_gemv`, the `extern fn` binding under `blas::gemv`, takes a fifth argument, `trans`. Call `blas::gemv`, whose new `trans` parameter defaults to `false`.

### Added
- **`std/blas`**: `blas::gemm(a, m, k, b, n)`, the matrix product of `a` (`m x k`) and `b` (`k x n`) (`cblas_dgemm`); `blas::syrk(a, rows, cols)`, the Gram matrix `aᵀa` with both triangles filled (`cblas_dsyrk`); and a `trans` parameter on `blas::gemv` that multiplies by the transpose. Matrices are flat row-major vecs with explicit dimensions, as for `gemv`. A dimension that is negative, above `INT_MAX`, or doesn't multiply to the vector's length, or a result of more than `INT_MAX` elements, is a runtime error naming the routine (`blas.gemm: b has 3 elements, but k * n = 2 * 1`); a dimension of 0 gives zeros of the result's shape.

  ```quark
  incl 'std/blas'

  m = vec [1.0, 2.0, 3.0, 4.0, 5.0, 6.0]                 // 2 x 3, row-major
  print(blas::gemv(m, 2, 3, vec [1.0, 1.0], true))       // [5.0, 7.0, 9.0]
  print(blas::gemm(m, 2, 3, vec [1.0, 0.0, 0.0, 1.0, 1.0, 1.0], 2))   // [4.0, 5.0, 10.0, 11.0]
  print(blas::syrk(m, 2, 3))                             // 3 x 3: [17.0, 22.0, 27.0, ...]
  ```
- **Benchmark suite** in `src/benchmarks/` (`python3 bench.py run`, documented in `src/benchmarks/README.md`). 18 benchmarks (plain loops, column arithmetic and reductions, sorting, grouping, joins, strings, linear algebra) compare Quark's `list`, `vec` and `vec` + `std/blas` code with pure Python, NumPy, pandas, Polars, R loops, R vectors, `data.frame` and `data.table`. Every implementation reads the same generated data and its result is checked against a reference implementation; each runs in its own process, single-threaded by default. A `quark_tbl` entry is reserved in the benchmarks a table implementation will cover.

### Changed
- Benchmark suite setup, after a fresh-clone check: the `quark_blas` implementations are separate programs (`quark/<bench>_blas.qrk`), so a machine without OpenBLAS skips only them (before, the build failure took down every Quark implementation of `arith`, `dot`, `zscore`, `gemv`, `gemm` and `ols`). `bench.py setup` checks for Python 3.11+, installs data.table from CRAN into the R user library when it is missing (`--no-r-packages` skips that), and prints what is available; `run` prints the same and reports missing pieces as skipped. The driver has Windows paths (`.venv\Scripts\python.exe`, `.exe` programs, `Rscript` under Program Files or `BENCH_RSCRIPT`; no peak memory, since there is no `wait4`), untested so far, and reads peak memory in the right unit on macOS.
- A `blas::gemv` dimension above `INT_MAX` says `blas.gemv: rows=... cols=...: BLAS supports dimensions up to 2147483647` instead of reporting a size mismatch.

### Internal
- The BLAS glue checks every matrix shape in one place (`qblas::check_shape`).
- `architecture.md` §15 (new) lists the performance costs the benchmarks found, each with its cause, measurements and a direction: source-location tracking (two thirds of a call's cost in `fib`), boxed arithmetic on call results, page faults on fresh vec buffers (huge pages), missing dense fast paths and fusion in vec kernels, list push and GC scanning, dict key hashing, strings without a stored length, sorting, and the OpenBLAS thread pool.
- `TestStdBlas` checks the new routines' results and 16 more error cases: wrong dimensions, results over `INT_MAX` elements, i64 inputs, missing entries, `null` and an `int` reaching a parameter through `any`, and a wrong argument count through a function value.
- Known issues (now `known_issues.md`) gains the OpenBLAS thread-pool GC warning, found with the benchmarks. They also found that `+` with an operand of unknown type was typed `str`, which the new back end's work fixes (that entry, Fixed). The first measurements and what they show (list building, dict hashing and strings no faster than CPython; `vec` close to NumPy; `std/blas` on par with NumPy and R) are in `src/benchmarks/RESULTS.md`.

## 2026-11-13: Vec reductions follow the table aggregates; `std/stats` (tables D13)

Commits: `571cab3` (merged in `5ff0225`)

Implements `tables_v0_1_spec.md` D13: `sum`, `min` and `max` of a vector follow the table aggregate rules, so `sum(t.x)` and `summarize(s = sum(e.x))` will agree. Two decisions were taken while implementing it: `min`/`max`/mean of a vector with no present values are a runtime error (D13 had said `null`), and the mean is `stats::mean` in a new `std/stats` module rather than a prelude builtin.

### Breaking
- **`sum` of an int or bool vector is an `int`, and exact**; it was a float summed in floating point. `sum(vec [1, 2, 3, 4])` prints `10` (it printed `10.0`), `sum(vec [true, false, true])` is `2`, and `sum(vec [9007199254740993, 1])` is `9007199254740994` (it was `9007199254740992.0`). An overflowing int sum is a runtime error (`integer overflow in 'sum'`). A float vector still sums to a float.
- **`min` and `max` of a vector keep its element type**: `min(vec [1, 2])` is `1` (it was `1.0`), so `max(v) - min(v)` is int arithmetic. They also accept `vec[str]` (byte order: `max(vec ['pear', 'apple'])` is `pear`) and `vec[bool]`.
- **NaN is skipped** by `sum`, `min` and `max`, like a missing entry: `sum(vec [1.5, to_float('nan')])` is `1.5` (it was `nan`).
- **A float sum is pairwise** (NumPy's method) instead of one value at a time, so long sums can differ in the last digits: a million `0.1`s sum to within `1e-8` of `100000` (it was off by about `1e-6`). The result is the same on every platform. The `reduce` benchmark runs in 18 ms (25.5 ms before).
- **No present values:** `sum` is `0` (`0.0` for floats); it was `null` for a vector whose entries were all missing. `min` and `max` are a runtime error, `min() of a vec with no present values`, for an empty vector (as before) and now also for one whose entries are all missing or all NaN.

### Added
- **`std/stats`** with `stats::mean(v)`: the mean of the present values of a numeric or bool vector, as a float (a bool vector gives the share of `true` values); missing entries and NaN are skipped; no present values, or a `vec[str]`, are runtime errors. `mean` is not a prelude name, so programs can keep using it as a variable.
  ```
  incl 'std/stats'
  print(stats::mean(list [1, null, 4].to_vec()))   // 2.5
  ```

### Fixed
- `min(v)` and `max(v)` of a vector whose entries were all missing gave `null` while typed `float`, so `m = min(v)` stored `0.0`: `v = list [1, null].to_vec()[vec [false, true]]`, `m = min(v)`, `print(m + 1.0)` printed `1.0`. It is now the runtime error above.

### Internal
- `q_vec_sum` and `q_vec_extremum` (`types/vector.hpp`) use the engine's rules: `q_vec_skips` (missing, or NaN in a float vector), `__builtin_add_overflow` for int sums, `q_vec_compare` for each element type, with fast paths for vectors with no missing entries. Float sums go through `q_pairwise_sum` (new `core/sum.hpp`): `q_vec_f64_present_sum` sums a dense vector in place and gathers the present values of any other first; the engine's `agg_sum` and `agg_mean` gather each group's values the same way (`gather_f64`, a counting sort), so a group sums exactly as `sum()` sums a vec of its values. Int means sum exactly in 128 bits (`q_vec_mean_present`, shared by `stats::mean` and `agg_mean`). Nothing uses `long double`, whose width differs by platform. `q_vec_numeric_as_f64` is removed. The analyzer types `sum` as `int` or `float` and `min`/`max` as the element type (they were `any` and `float`).
- `src/core/stdlib/stats.qrk` and `stats.hpp` (`q_stats_mean`, with `agg_mean`'s rules).
- Tests: `src/testfiles/table/reduce_test.cpp` (run by `TestTableEngine`) checks that `sum`, `min`, `max` and `stats::mean` give what the engine's `agg_sum`, `agg_min`, `agg_max` and `agg_mean` give over a whole table, for int, float (with NaN), bool and str vectors with and without missing entries, plus: missing entries and NaN between the same values don't change a float sum's bits, each group of a grouped `agg_sum`/`agg_mean` equals `sum`/`stats::mean` of a vec of its values, and the pairwise error bound; 10 `TestSemanticsSafety_Rejected` cases cover no present values, all-NaN, untyped vectors, overflow, `vec[str]` sums, a typed result, and `stats::mean` misuse. The smoke expectations of `smoke_types`, `smoke_vectors`, `smoke_vector_soundness` and `smoke_vector_layout` change from floats to ints where an int or bool vector is reduced (22 values). The editor data lists `std/stats`.

## 2026-11-13: A missing vec entry taken out as one value is an error (tables D12)

Commits: `571cab3` (merged in `5ff0225`)

Implements `tables_v0_1_spec.md` D12, the first prerequisite of the table compiler work: vectors keep their missing entries, and a missing entry can't leave a vector as a single value. Before, it was read as `0` (`0.0`, `false`) into a typed variable.

### Breaking
- **`for x in v`, `v[i]`, `v.to_list()` and `v.enumerate()` on a vector with a missing entry are runtime errors**: `vec element 1 is missing; use fillna or drop_na first`, and `to_list() of a vec with 2 missing entries; use fillna or drop_na first`. This holds for every element type and also when the vector reaches the code through `any`. They printed or stored `0`, `0.0`, `false` or `null`:
  ```
  v = list [1, null, 3].to_vec()
  for x in v:            // printed 1, 0, 3; now 1, then the error
      print(x)
  n = v[1]
  print(n * 2)           // printed 0; now the error at `n = v[1]`
  ```
  Use `v.fillna(0)` or `v.drop_na()` first. Operations on the whole vector are unchanged: `v + 1` is `[2, null, 4]` and `sum(v)` skips the missing entry.
- **`v.get(i)` has type `any`.** It returns `null` for a missing entry (as documented), but was typed as the element type, so `n = v.get(1)` stored `0` in an int variable. Now `n` holds `null`; `n: int = v.get(1)` is `variable 'n' is declared int but got null`, and `v[i]` is the typed read.

### Added
- `v.drop_na()`: the present entries of a vector, in order, with the same element type (`list [1, null, 3].to_vec().drop_na()` is `[1, 3]`).

### Internal
- The check is in the runtime, where every single-element read goes: `q_vec_get_scalar` (`v[i]`, table rows) and `q_iter_get` (loops) report a missing entry (`q_vec_missing`); `q_vec_get` is the `.get` that returns `null`; `q_to_list` refuses a vector with missing entries. A `for` loop over a value the analyzer typed `vec` records its own location before each element read, so the error points at the loop. Loops over vectors without missing entries still read the buffer directly, with no check.
- `q_vec_keep` gathers the elements a predicate keeps; the mask filter (`v[mask]`) and `q_drop_na` share it.
- Tests: 14 `TestSemanticsSafety_Rejected` cases (loops over int, float, bool and str vectors, captured and untyped loop variables, a `vec` parameter, `v[i]` with positive and negative indexes and through `any`, `to_list`, `enumerate`, typed and untyped `.get`); `smoke_vector_soundness.qrk` covers `drop_na`, `fillna`, `.get` and `v[i]` on present entries. The editor data and grammar list `drop_na`.

## 2026-11-13: New back end: lowering pass, typed IR and verifier (architecture §13)

Commits: `571cab3` (merged in `5ff0225`)

The C++ back end is rewritten (`architecture.md` §1.6). The new one lowers the analyzed program to a small typed IR in which every temporary, conversion, runtime check and evaluation step is explicit, verifies the IR, and prints it. This fixes the "part of an expression runs at the wrong time" group of known issues as a whole, and it is the back end the table compiler work will target. It replaces `codegen/`, which is removed. The whole test suite passes on it, and loops run as fast as before.

### Breaking
- **Every part of an expression runs once, when it is needed, left to right as written** (`semantics.md` §4.5). Programs whose output depended on the old order print something different:
  - `false and f().upper()`, `true or f().upper()` and `'x' if true else f().upper()` no longer call `f` (the receiver of a method call ran before the whole statement, so the skipped side ran anyway).
  - `f().contains(g().upper())` calls `f` before `g`; it called `g` first.
  - A struct literal evaluates its field values in the order written. It evaluated them in the struct's declaration order: with `struct P:` / `a: int` / `b: int`, `P { b: f(), a: g() }` called `g` first.
- **An assignment can't be part of an expression.** `if a and (n = f()) > 0:`, `b = a or (z = 3) > 1`, `w = (q = 4) if c else 0`, `y = x + (x = 5)` and `elseif x = f():` are parse errors (`an assignment can't be part of an expression; assign it on its own line first`). Under the new evaluation order such an assignment could be skipped, leaving the variable unassigned; the old back end ran it eagerly, even when `a` was false (and an `elseif` assignment failed in clang). An assignment as the whole condition of an `if` or `while` still works.
- **`x and y` / `x or y` with an operand of unknown type returns that operand**, not a bool: `fn id(x) -> x`, `s = id(0) or id(5)`, `print(s)` prints `5`; it printed `true`, because the result was typed `bool` and read as one. `id(2) and id(0)` is `0` (it was `false`).
- **A bool never equals a number**, as documented: `b = true`, `print(b == 1)` prints `false` (it compared the raw C++ values and printed `true`); `b != 1` is `true`.
- **`xs[i]` and `s[i]` with an index out of range are runtime errors**, as `v[i]` already was: `list [1, 2][5]` is `list index 5 out of range (list length 2); use .get(i) when the index may be past the end`, and `'abc'[3]` is `str index 3 out of range (str length 3)`. They returned `null`, so `w = list [1, 2][5]`, `print(w)` printed `0`. Use `xs.get(i)` to probe.
- **`xs.get(i)` and `d.get(k)` have type `any`**, since they return `null` for a missing item, like `v.get(i)`. They had the element type, so a `null` from them could be read as a number. Code that used the result as a typed value needs a typed declaration, checked once:
  ```
  t = 0
  t = t + xs.get(0)      // now a compile error: cannot assign value of unknown type to 'int'
  n: int = xs.get(0)     // checked: a runtime error if xs is empty
  t = t + n
  ```
- **A `when` value pattern on a vec is an error**: a compile-time error when the vec is known (`when can't match a vec (== on a vec compares elementwise); test all(v == x) or any(v == x) with if instead`), a runtime error otherwise. It compared elementwise and read the first result as a bool, so `g(vec [1, 1])` with `when a:` / `1 -> 'one'` / `_ -> 'other'` returned `'other'`. A `_` arm alone still matches a vec.
- **A `for` loop whose body changes the length of the list it iterates stops with a runtime error** at the `for` (`semantics.md` §14). The length was counted once at the start, so items pushed during the loop were skipped silently and a list that shrank was read past its end:
  ```
  xs = list [1, 2, 3]
  for y in xs:
      xs.push(y)    // was: ran 3 times; now: list changed size during the for loop over it (length 3 at the start, 4 now)
  ```
  Writes that keep the length (`xs[i] = v`) and a change followed by `break` are still allowed. A list that grows while you work through it needs an index loop: `while i < len(xs):`.

### Fixed
- A `while` condition containing a method call is evaluated before every iteration: `while x.contains('a'):` / `x = x.replace('a', 'b')` ends. The condition was computed once, so the loop never ended.
- An `elseif` condition with a method call compiles: `elseif name().upper() == 'B':` failed in clang (`expected expression`). Such a condition also runs only when the conditions before it were false.
- A member or index assignment that is the last statement of a function computes its value once: `fn setit(d) ->` / `    d.x = bump()` called `bump()` twice, once to store and once for the function's result.
- A variable assigned by a `while` condition can be read after the loop: `while s = xs.get(i):` / `...`, then `print(s)`.
- `+` with an operand of unknown type is no longer typed `str`. The checker counted a value of unknown type as a string (and as a bool and an int), so a `str` annotation trusted the sum: `x: any = 1`, then `z: str = x + x` stored the int `2` with no error, and so did a `str` return annotation (`fn g(a: any) str -> a + a`) and a `str` struct field. Each is now a runtime error (`variable 'z' is declared str but got int`). Correct programs the wrong type rejected compile: `y = x + x`, then `y + 1` (`operator 'PLUS' requires numeric operands, got str and int`), `sqrt(x + x)`, and, with the D13 typing of `sum`, `w = sum(v) + sum(v)`, then `w + 1`.
- `and` and `or` of two bools are plain C++ `&&` and `||`; a loop testing `n % 3 == 0 and n % 5 != 0 or n % 7 == 0` runs about 45% faster.
- `'abc'[i]` with an `i` of unknown type that isn't an int is a runtime error (`str index must be int, got float`); it returned `null`. `xs[4294967296]` and `'abc'[4294967297]` are out of range; the index was cut to 32 bits (`'abc'[4294967297]` was `'b'`).
- A `for` loop over a value of unknown type that isn't a list, string or vec fails before the first iteration, also when it is empty: `for k in id(dict {}):` ran zero times with no error (a non-empty dict failed). The message names the type (`for-loop iterable must be list, string, or vec, got null`; a `null` said `len() expects str, list, dict, or vec`).
- Indexing a value of unknown type that isn't a list, string or vec says so: `cannot index a dict; only list, str and vec can be indexed` (was `get() expects list/string/vec as first argument`).
- A list that shrinks during a `for` loop over it is no longer read past its end: `for y in xs: xs.pop()` read `null`, or `0` for a list of ints (now the runtime error above).
- `for` loops over a list read its elements directly: a program that builds a 5M-element list and sums it 20 times runs 2.5x faster (1.18s to 0.48s).
- Loop bodies no longer pay for source locations: a loop that only adds up list elements runs 12% faster, back to its speed before `0cf468d`. Runtime errors report the same locations as before.

### Removed
- The old back end, `codegen/` (`codegen.go`, `storage.go`, `elemtype.go`). Nothing selects it any more: the driver has one back end, and there is no `QUARK_BACKEND` setting.

### Internal
- New packages: `lower/` (`ir.go` the IR, `lower.go`/`expr.go`/`calls.go`/`store.go` the lowering, `verify.go` the verifier) and `emit/` (prints the IR). The driver always runs lower → verify → emit (`driver.Generate`); `architecture.md` §13 lists the remaining back-end steps.
- The verifier runs on every compile and checks scopes, value representations, loop control, closure binding and call arity (`INV-IR-*`, `error_codes.md`). Lowering's internal errors are returned to the driver (`lower.ICE`) instead of exiting the process.
- Measured on the same programs with both back ends (best of 5, `-O3`): a 100M-iteration `range` loop, 10 passes over a 5M-element list, 10M closure calls and 30M native function calls are within 2%.
- A native pointer result passed straight to another native function is boxed and converted again, so a null `str`/`list`/`dict`/`vec`/`fn` from the first is reported (`a native function declared to return str returned a null pointer`) instead of reaching the second; bool/number comparisons and `and`/`or` of unknown types go through the boxed operators.
- Tests: `TestEvaluationOrder` (integration), `lower/lower_test.go` (every smoke program lowers and verifies; 15 kinds of malformed IR are rejected), `emit/emit_test.go` (the C++ shape of each lowering) and `emit/program_test.go` (codegen's C++ text tests, moved: the three that checked the old `and`/`or`, `!` and ternary forms are replaced by `emit_test.go` cases; the missing-call-plan test checks `lower.Lower`'s error; the closure-call helper test became a test of `q_call0`..`q_calln` by arity).
- The unused `diagnostics.StageCodegen` is removed.
- Every `Unbox` (a raw read of a `QValue`'s union field) carries the reason its tag is known (`lower.Proof`: `ProofType`, an exact analyzer type, or `ProofElement`, a loop's element read), and the verifier rejects one without a fitting reason (`INV-IR-UNBOX`, `architecture.md` §12). A runtime call's result is never unboxed; adding the reasons found the `when` hole above, where `q_eq`'s result was unboxed as a bool although it is a vec for a vec operand. `when` value patterns call the new `q_match_eq` (a C++ `bool`), and loops over an iterable of unknown type take their length from the new `q_for_len` (a `long long`, checked) instead of unboxing `q_len`. `xs[i]` lowers to the new `q_index`; `xs.get(i)` stays `q_get`.
- `for` over a value typed as a list: `q_for_list` (the `QList*`, checked once), `q_list_len`, `q_list_at`, and the loop test `q_for_list_more`, which calls `q_list_resized` when the length changed; a loop over an iterable of unknown type uses `q_for_more` with `q_iter_get` (`architecture.md` §9). `lower.ForIndex` has `Sized` and `Loc` for the test, `lower.LocRef` passes a location table entry to the runtime, and the verifier checks both (`INV-IR-REP`, `INV-IR-LOOP`, `INV-IR-NODE`). Tests: `TestForLoop_ListResized`, `TestForLoop_ListAllowed`.
- Source locations are a static table in the generated C++ (`static const QSourceLoc _qloc[]`, `lower.Program.Locs`); `SetLoc` is an index into it and prints as `q_set_source_loc(&_qloc[N])`, one pointer store where there were three. `q_runtime_loc` is a `const QSourceLoc*` and `QSourceLocGuard` saves one pointer. The verifier rejects an index outside the table (`INV-IR-NODE`).
- Known issues gains three bugs found while testing: a `v[i]` read of a missing vector entry into a variable is `0`, a `fn` declaration with an inline `when` body swallows the next top-level statement, and unbounded recursion crashes with no message and loses earlier output.

## 2026-11-13: Docs overhaul

Commits: `44a6460`, `f782060`

No compiler, runtime or stdlib changes. Every doc was rewritten against the current compiler, and each claim was checked by running programs or reading the code. Each doc now starts with a table of contents and describes only the current state; superseded designs and finished plans are removed (git history keeps them).

### Docs
- **README.md**: describes Quark as what it is: a data language whose compiler infers types and checks them at compile time, with runtime checks only as the backstop (it opened with "dynamically-typed"). Every sample was run against the compiler. Stale items are removed: the `QK-CHECK-002` dict warning, "return annotations are only checked on native functions", `::` listed as planned, the "inline lambda doesn't parse" caveat, and string interpolation as planned (not decided). Known limitations now list only deliberate gaps; bugs link to Known issues here.
- **tables_v0_1_spec.md** (revision 3): the 2026-11-13 decisions are now the spec itself, rather than a "Revision 2" section over the April draft that still contradicted them (`orders: Order = tbl {...}`, `as` aliases, backends, schemas). Sections are renumbered (§2 decisions, §7 engine, §8 CSV, §9 plan, §10 inference, §11 Arrow). Literals, row access, masks and an annotation for untyped tables are listed as open questions.
- **architecture.md**: named functions are fixed declarations, called directly when they capture nothing (it said they were mutable closure bindings, called through `DispatchClosure`). Runtime errors are report-and-exit (it said ops return `qv_null()` on a mismatch). Also brought up to date: the analyzer pass order, `types.Analysis` fields, `QClosure`/`QFnInfo`, the `qext` API, the codegen output order and tests. The old §12 health review is replaced by §12 Soundness contract, §13 Roadmap (the lowering/IR plan, remaining steps only) and §14 Structural debt (only items still true).
- **semantics.md, grammar.md**: fixed declarations, block scoping and assignment to outer names, element and value types, the `null` rules, unannotated extern parameters being `any`, structs across files. Duplication removed: precedence and syntax live in grammar.md, behaviour in semantics.md, builtins in stdlib.md.
- **stdlib.md, error_codes.md, stdlib_expansion_v0_1_spec.md**: element types of returned lists, dict value-type errors, vec reductions as they behave today (with the planned D13 change), `std/csv` marked for replacement; `INV-SYMBOL`, `INV-STORAGE`, `INV-FIXED-FN`, `INV-OPERATOR` and `INV-ASSIGN-TARGET` added, and `INV-ARGCHECK` removed; the expansion spec's CSV section points to the tables spec, and every name uses `::`.
- **structs, modules and FFI specs, editor extension README**: rewritten as current references, with the implementation deviations folded into the rules. Structs: imported and module structs, widening, `null` rules, `fn` fields, no separate schema type. FFI: the current interface, callbacks, the `qext` API; DuckDB after native tables.
- **CLAUDE.md**: new "Docs" section: which doc owns which topic, and the current-state-only rule. It also stops calling Quark dynamically typed, and the `qv_null()` rule is fixed.
- **Known issues** (now `known_issues.md`) gains bugs found while checking: two expressions with no separator are accepted (`total = price qty`), unclosed strings, struct `==` through untyped values, extern methods colliding across files, the `stdlib/` folder hijack, a misspelled `print` alignment being ignored, and four misleading messages.

## 2026-11-13: Element types are fixed, variables don't narrow, return annotations are checked (architecture §12)

Commits: `fd6822c`

Implements rules A-E of `architecture.md` §12 (§12.9 at the time) and the `null` decision. Together they close the largest group of known issues: values read with a type the compiler had inferred wrongly, which printed garbage numbers.

### Breaking
- **A list keeps the element type it was created with** (rule B), through every name for it. Writing a value of another type is a compile-time error when the compiler knows the value's type:
  ```
  a = list [1, 2]
  b = a
  b.push('s')        // error: cannot add str to list[int]: a list keeps the element type it was created with
  ```
  The same for `insert`, `set`, `l[i] = v` and `concat` (`list [1].concat(list ['a'])`). To mix types, create the list under a `list` annotation: `xs: list = list [1, 2]`, then `xs.push('s')` works. A literal passed to a `list` parameter or stored in a `list` struct field holds any value too. A list of mixed literals (`list [1, 'a']`) accepts both types.
- **A write the compiler can't check is checked by the runtime** (rule C): an untyped function's parameter, a value from `any`, native code.
  ```
  fn add(xs) -> xs.push('x')
  l = list [1, 2]
  add(l)             // runtime error: push() can't add a str to a list of int: a list keeps the element type ...
  ```
  Lists nested in lists are checked by their own element type: `fn add_row(g, r) -> g.push(r)`, `rows = list [list [1, 2]]`, `add_row(rows, list ['a'])` is `push() can't add a list[str] to a list of list[int]`. A `list[int]` can't go into a `list[list[float]]` (its ints would be read as floats), and a list made under a `list` annotation can't go into a `list[list[int]]`.
- **A dict keeps its value type** (rule E): `d = dict { a: 1 }`, then `d.b = 'hello'` or `d.set('b', 'x')` is a compile-time error (it was a warning, and the value was later read as an int). `d: dict = dict { a: 1 }` holds any value. Untyped code is checked at runtime: `can't store a str under key 'k' of a dict of int`.
- **A variable never narrows** (rule A). A variable whose first value has no known type stays untyped, whatever it is assigned later: `x = id('hello')`, then `if c: x = 5`, then `y = x` keeps `y` untyped (it used to make `y` an `int`, read from a string). Convert once with `n: int = x` to get a typed variable.
- **A variable holding a function keeps its signature**: `g = fn() -> 5`, then `g = fn() -> 'abc'` is `cannot assign fn() -> str to 'g', which holds fn() -> int`. Declare `g: fn = fn() -> 5` to hold any function (calls through it return a value of unknown type). Assigning a value typed only `fn` to such a variable is an error for the same reason.
- **Assigning a lambda to an existing name assigns that variable**, like every other assignment, also from a nested scope. `g = fn() -> 1` at the top level, then `g = fn() -> 2` inside a loop or function body, used to declare a second `g` there, so the outer `g` silently kept the old function.
- **Return annotations are checked on every function** (rule D), not only fully annotated ones. A body that can end without a value is a compile-time error: `fn f(c) int ->` / `if c:` / `1` is `function 'f' declares return type 'int' but can end without a value`. A body of unknown type is checked when the function returns: `fn f(k) int -> k()`, `f(fn() -> 'str')` is `function 'f' declares return type int but returned str` (it printed a number).
- **An `if` without `else` can have no value**: its type includes `void` (as a `when` with no matching arm already did), so the result of an untyped `fn f(c) ->` / `if c:` / `1` is no longer typed `int`. Code that does arithmetic on such a result directly, `f(true) + 1`, is now a compile-time error, since `f(false)` is `null`.
- **`list`, `dict`, `vec` and `fn` annotations don't accept `null`** (decided 2026-11-13), for parameters, typed declarations, struct fields, defaults and returns, and for every kind of function: `fn g(l: list) -> 1`, `g(null)` is `argument 1 of 'g' expects list[any], got null`; from an untyped value, `argument 'l' of 'g' expects list, got null`. Use `xs: list = list []` for an optional list parameter, or `any` for a value that may be `null`. Struct and `resource` annotations still accept `null`.
- **Native functions can't return a null pointer for `str`, `list`, `dict`, `vec` or `fn`.** An `extern fn` declared to return `list` that returns `nullptr` gave an empty list; it is now `a native function declared to return list returned a null pointer`; for `str` it gave `null`. `qext::box` of a null `QList*`, `QDict*`, `QVector*` or `QClosure*` is an error (use `qext::null_val()`).
- `list.concat` with a value that isn't a list is a runtime error (`concat() expects two lists, got list and int`); it returned `null`.

### Added
- **An empty `list []` or `dict {}` takes its element type from its first write**, so accumulating loops stay fast: `out = list []`, `for i in range(n): out.push(i * i)` makes `out` a `list[int]`, and a later `for v in out: total = total + v` reads raw ints. Storing it into a typed slot decides it too (`xs = list [1]`, then `xs = list []` stays a `list[int]`; `grid.push(list [])` on a `list[list[int]]`). The error for a later mismatch names the line of the first write. Passing it to an untyped parameter first makes it hold any value.
- An `int` written into a list of floats or a dict of floats is stored as a float: `l = list [1.5]`, `l.push(2)` gives `[1.5, 2.0]` (it read back as `1e-323`). An `int` returned by a function annotated `float` is widened: `fn f() float -> 1` returns `1.0`.
- `qext::list_push(QList*, QValue)` appends with the element-type check, `qext::new_list()` creates a list that holds any value, and `qext::dict_set` now checks the dict's value type.

### Fixed
- *All of these printed garbage numbers or wrong values with no error.*
- A list changed through another name, through a function (`fn add(xs) -> xs.push('x')`), through `l[0] = 'str'`, through `d.b = 'hello'` or `d.set` inside a function, or the results of `l.concat(list ['a'])` and `l.push('x')`, kept the old element type, so a loop over it read a string as an int.
- `zs = list [1, 2]`, `zs.push(id('hello'))`, then `zs[2] + 1` read the string as an int; now a runtime error at the push.
- An `any` variable assigned a typed value in a branch or a loop that may not run (`x = id('s')`, `for i in range(0): x = 5`) was trusted as that type afterwards; `P { n: x }` stored `2.5` in an `int` field.
- *Found 2026-11-13 while doing this work, and predating it:*
- **Arithmetic on an int-or-float value was typed `int`**: `xs = list [1, 2.5]`, `for v in xs: z = v + 1`, `print(z)` printed `4615063718147915776` for `2.5`. Its type is now int-or-float, and it prints `3.5`.
- **A `list[int]` could be added to a `list[list[float]]`** (int → float promotion was applied to element types): `b = list [list [1.5]]`, `b.push(list [1, 2])`, then reading `b[1]`'s elements printed `5e-324`. Now a compile-time error.
- **Reassigning a function variable changed the return type the compiler trusted**: `g = fn() -> 5`, `run = fn() -> g()`, `g = fn() -> 'abc'`, then `run() + 1` printed a number. Now a compile-time error (see Breaking).
- A `vec[any]` or `list[any]` value could be assigned to a variable typed `vec[int]` or `list[int]`, which a loop then read raw; now a compile-time error.
- A call written above `fn f(r: result)` didn't see that `r` is a result (`print(f(5))` above it was a runtime error); now it is a compile-time error like a call below it.

### Changed
- Messages for writes into lists and dicts name the container's element type and how to make a list that holds any value. The error for assigning a value of unknown type to a typed variable now suggests a typed declaration.

### Internal
- Types: `CanAssign` (stores, int → float) and the new `IsSubtype` (nested positions, no conversion; unions on both sides; `null` only for struct and resource; function values by arity and return type). Pending list and dict types (`NewPendingList`, `SettleContainers`, `FixElementType`, peers fixed together, `MergeTypes` settles them); `ElementOf` drops a function's return type in element positions.
- Analyzer: `assignName` split into `assignName`/`checkReassign`; `checkElemWrite`, `checkMutation` (replaces `widenReceiverOnMutation`), `settleArgs`, `escapePending`, `contextLiteral`. `validateReturnType` rejects a body that may end without a value; `analyzeIfStatement` adds `void` when there is no `else`. Predeclared `fn` parameter types use `resolveTypeNodeStaticWithStructs` (so `result` parameters are known before the body is analyzed).
- Runtime: `QList` is a struct deriving from the vector with an `elem` field; `QDict` has `elem`. `QElemType` and `q_elem_int`/`_float`/`_bool`/`_str` in `core/value.hpp`; `q_elem_sub`, `q_elem_is`, `q_elem_store` in `types/elemtype.hpp`. `q_push`, `q_set`, `q_insert`, `q_list_concat`, `q_dict_set` check; `range`, `split`, `keys`, `values`, `slice`, `concat` and `to_list` create typed lists. `q_ret_int`/`_float`/`_bool`/`_check`/`_struct` in `builtins/params.hpp`; `q_box_native_str`, null checks in `qv_*_ptr`. `q_type_name(ValueType)` in `core/value.hpp` is the canonical type-name table (the two copies in `conversion.hpp` use it).
- Codegen: `codegen/elemtype.go` (`elemDesc` emits `_qet_N` descriptors after the struct definitions; `checkedReturn`). List and dict literals pass their element type; `paramTagChecks` no longer allows `null` for `list`/`dict`/`vec`/`fn`.
- Tests: `smoke_element_types.qrk`; 55 `TestSemanticsSafety_Rejected` cases (writes from typed and untyped code and from `any`, nested lists, vecs, results and structs, first-write errors, no narrowing, function signatures, every return path, `null` for each annotation); `TestElementTypes_ModulesAndNative` (writes and returns through another module, `qext::list_push`/`dict_set`, native null returns). `smoke_printing`, `smoke_struct_fields`, `gc_steady_state` and two Go tests were updated for the breaking changes.

## 2026-11-13: Table engine and CSV reader (runtime only)

Commits: `e6a1db9`

Nothing here is reachable from Quark code yet: the `tbl` type, queries and the `std/csv` bindings are the next step (`tables_v0_1_spec.md`, Revision 2, which records the design decisions of this day).

### Internal
- **Table engine** (`runtime/include/quark/table/engine.hpp`, namespace `quark::tbl`): the kernels behind the planned verbs, working on raw column buffers. `make_table`, `take`/`take_table` (row gathering for filter and sort), `sort_perm`/`sort_table` (stable, several keys, nulls last, a fast path for one dense numeric key), `group_rows` (hash grouping, groups in key order, nulls last) and the aggregates `agg_count`, `agg_count_values`, `agg_sum`, `agg_mean`, `agg_min`, `agg_max`. Missing values follow `na.rm = TRUE`. Row indices are int32. On 500K rows: group + mean 23 ms, sort by one float key 97 ms.
- **CSV/TSV reader and writer** (`runtime/include/quark/table/csv.hpp`, namespace `quark::csv`): parses straight into typed columns, with a schema or with types inferred from every value. Handles quoted separators and line breaks, `""` escapes, CRLF/CR/LF, a BOM, blank lines, spaces around values, and `""`/`NA` as missing. Columns are matched to the header by name. Malformed input is an error naming the file, line, column and text (`staff.csv:412: column 'salary': '12k' is not a float (and 3 more problems)`), never a silent null. A 27.6 MB, 500K-row file reads in 219 ms typed. Float parsing uses `std::from_chars` where the standard library supports it.
- Neither header is included by `quark.hpp`, so programs that don't use tables don't compile them.
- Tests: `TestTableEngine` compiles and runs `src/testfiles/table/engine_test.cpp` (53 checks) and `csv_test.cpp` (52 checks, including messy input, every error path, inference and a TSV round trip) at `-O2` with `-Werror`; `TestTableEngine_FatalErrors` checks the engine's runtime errors (int overflow in `sum`, `sum`/`mean` of a str column, columns of different lengths).

## 2026-11-13: Variable storage decided by the analyzer (architecture §12.8 step 2)

Commits: `0cf468d`, `fd6822c` (the Docs entry)

### Breaking
- **A typed declaration checks its value.** `x: int = v`, where the compiler can't know `v`'s type (a value from an unannotated function, `unwrap`, a dict), is now a runtime error when `v` isn't an int: `variable 'x' is declared int but got str`. It used to store the value unchecked, so `x` held a string the compiler treated as an int (see Fixed). The rules are those of parameters: an int widens to `float`, `list`/`dict`/`vec`/`fn`/`resource` and structs accept `null`, `str` and `result` don't.
- **A parameter typed by a literal default is checked on entry.** `fn foo(y = 0)` makes `y` an `int` parameter, as calls with a known argument type already showed (`foo('a')` was a compile error). A call with a value of unknown type is now checked like an annotated parameter: `fn id(v) -> v`, then `foo(id('abc'))` stops with `argument 'y' of 'foo' expects int, got str`. A float default widens an int: `fn scale(r = 0.5) -> r * 2`, `scale(id(3))` returns `6.0`.
- **A `float` variable always holds a float.** An int assigned to it is widened, so it prints with a decimal point: `x = 1.5`, then `x = 2`, `print(x)` prints `2.0`. This also applies to a captured variable (`c = 1.5`, `f = fn() -> c`, `c = 2`, `f()` returns `2.0`, was `2`) and to a `x: float = 3` declaration (`type(x)` is `float`, was `int`).

### Fixed
- **An int assigned to a `float` variable read back as garbage.** `x = 1.5`, then `x = len(list [1, 2, 3])`, `print(x)` printed `1.5e-323`. The same happened to a `float` parameter (`fn f(a: float) -> a = 2`) and a loop variable over floats. It now prints `3.0`.
- **A `float` declaration with an int value was stored as an int.** `x: float = len(l)` printed `3`; `y: float = 3`, then `y = 2.5`, printed `2`. They now print `3.0` and `2.5`.
- *The next two were found on 2026-11-13 while doing this step, and predate it.*
- **A typed declaration of a value of unknown type wasn't checked**, so reads trusted a type the value didn't have: `fn id(v) -> v`, `x: int = id('hello')`, `y = x`, `print(y + 1)` printed a large number; `x: str = id(5)` gave a `str` variable of type `int`. Both are now runtime errors (see Breaking).
- **A parameter typed by a literal default wasn't checked at runtime**: `fn foo(y = 0) ->` / `z = y` / `z + 1`, called as `foo(id('abc'))`, printed a large number, and `fn foo(y = 0.5)` called as `foo(id(3))` printed `1.5e-323`.
- **A variable first assigned in a `while` body and assigned again after the loop failed in clang** (`use of undeclared identifier 'quark_y'`). The two are different variables, as the checker already said, and now compile as two.
- **A `fn` declared inside a `for` or `while` body failed in clang**: `for i in range(3):` / `fn f() -> i * 10` / `print(f())` now prints `0`, `10`, `20`.
- **A runtime error inside a `for` or `while` body was reported at the loop's line.** `for r in list [1, 2]:` / `x = r` / `print(r % 0)` now reports `modulo by zero at file.qrk:3:10`.
- **A variable named `_lambda1` collided with the compiler's first lambda**: `_lambda1 = 5`, `f = fn(x) -> x + 1`, `print(f(_lambda1))` crashed with exit code 255 and no message, and `fn _lambda1()` plus any lambda failed in clang. Every name the compiler generates now starts with `quark___`, which no user name can produce. This also removes a rarer clash between a nested `fn` declaration and a module member (`fn f_` nested in a function, and `module f` with a member `_2`, could get the same C++ name).

### Docs
- `architecture.md` §12.9: why silent wrong results keep appearing (the analyzer's types are used both as guesses for diagnostics and as guarantees for raw reads), the soundness contract (a type is trusted only when every way into the slot is guarded), and the decided rules: variables don't narrow (A), element types are fixed when a list or dict is created (B), writes from untyped code into a typed list or dict are checked (C), return annotations are checked (D), a wrong-typed dict value is an error (E). With the measurements behind them: a tag check costs about 1.5% in a loop that does nothing else, a direct list element read is about 4x faster than `q_iter_get`, and the per-statement source location costs about 9%. Implemented later the same day (`fd6822c`, entry above). §12.6 is marked decided and step 4 of §12.8 updated.

### Internal
- **Symbols** (`architecture.md` §12.8 step 2). Every variable, parameter, loop variable, `when` binding and `fn` declaration is one `types.Symbol` with an `ID`, `Kind`, `StoreType` (the type all its values are known to have) and `Decl` (where codegen declares it). `Analysis.Refs` maps every resolved identifier node to its symbol, and `Analysis.Captures` lists each lambda's captured symbols. `finalizeSymbols` decides each symbol's `ir.Storage` (`ir/storage.go`: cell, int, float, bool or QValue) and its C++ name once, after the captures are final. `Analysis.CapturedByFunction`, `Analysis.CheckedCaptures` and the name-keyed capture lists are removed; `HoistScope.Cells` holds symbols, `FixedFn.Sym` is exported.
- Codegen no longer keeps scope state: `declaredVars`, `cellVars`, `varTiers`, the scope stacks, `currentFuncNode`, `isCaptured` and `loopVarTier` are removed. `codegen/storage.go` holds what's left of storage handling: `varSym`, `readVar`, `assignSym` (declares at `Decl`, assigns otherwise; used by assignments and typed declarations), `storeValue`/`toScalar` (conversion and checks), and one type→check table (`checkFor`) shared by parameters (`q_param_*`), struct fields (`q_field_*`) and typed declarations (`q_var_*`). Loop bodies go through `generateBlock` (`emitBody`).
- Runtime: `q_var_int`, `q_var_float`, `q_var_bool`, `q_var_check` and `q_var_struct` (`builtins/params.hpp`).
- Generated names: lambdas `quark___lambdaN`, nested `fn` declarations `quark___fnN_name` (was `quark_name__N`), and a variable sharing its name with an earlier one `quark___v<id>_name`; native thunks name their parameters `_a0`, `_a1`, ...
- Tests: `smoke_storage.qrk`; 13 `TestSemanticsSafety_Rejected` cases (typed declarations of each kind from `any` and `null`, captured and in-function declarations, default-typed parameters of functions and lambdas); two loop-body cases in `TestRuntimeErrorLocations`.


---

## 2026-11-12: Function values, `fn` type, call-shape checking, keyword renames, struct field checks, name clashes, `input()` result, fixed `fn` declarations, modules v0.2, compiler cleanup

Commits: `c0ac8cd`, `ac274c0`, `6dc11a0` (keyword renames), `2e62209` (struct fields, dict keys), `4294dab` (name clashes, shadowing, `input()`), `b52660f` (fixed `fn` declarations, modules v0.2 step 1), `5c3d662` (the rest of modules v0.2: `::` names, one scope per file); `b46be3d` (compiler cleanup, `architecture.md` §12.8 step 1)

### Breaking
- **A pipe is exactly the call it stands for.** `x | f(a)` is parsed as `f(x, a)` and `x | obj.m(a)` as `obj.m(x, a)`, so a pipe can no longer behave differently from the call. The one difference there was: `3 | d.f()` called a function stored under the dict key `f`, while `d.f(3)` was `type 'dict[...]' has no method 'f'`. Both are now that error. To call a function held in a dict, read it first:
  ```
  f = d.f
  print(3 | f())           // was: print(3 | d.f())
  ```
- A pipe whose target isn't a call (`x | len`, `x | f() + 1`) is now a parse error (`pipe target must be a function call; use f(...) or obj.method(...)`, at the `|`). It was the same message as a check error.
- **`input()` returns a `result`.** It returns `ok(line)` (`ok('')` for an empty line), or `err('end of input')` once stdin is exhausted. It used to return a `str`, or `null` at end of input. Using the value as a string is now a compile error (`len(input())`, `input().upper()`, a `str` parameter), so code must unwrap it:
  ```
  name = unwrap(input('Name: '))       // was: name = input('Name: ')

  while true:                          // was: line = input(); if line == null: break
      r = input()
      if is_err(r):
          break
      line = unwrap(r)
      print(line)
  ```
- **A `result` is never `null`.** `null` can no longer be passed, stored, returned or used as a default where a `result` is expected. This used to be allowed, as for `list` and `dict`. It is a compile error when the compiler can see it (`fn f(r: result)` called with `f(null)`, `r: result = null`, `r = ok(1)` then `r = null`, a `result` struct field, `fn f() result -> null`, `r: result = null` as a default). Otherwise it is a runtime error (`argument 'r' of 'f' expects result, got null`). Comparing a result with `null` is a compile error: `a result is never null, so this comparison is always false; use is_ok(r), is_err(r) or when to check it`. This catches the old `line == null` loop, which would otherwise never stop.
- **An `extern fn` and another definition with the same name in one file is a compile error**: `extern fn foo(x: int) int as 'q_foo'`, then `fn foo(x) -> 1` gives `'foo' is already defined on line 2`. It used to resolve silently to one of them. Names in different files no longer clash at all: each file has its own names (see Added, modules v0.2), so an importer's `fn helper` no longer hijacks a library's calls to its own `helper`, and an importer's `fn fmt_full` next to `incl 'std/fmt'` is no longer ignored. (Earlier the same day, a name defined in two files was briefly a compile error; modules v0.2 replaced that rule.)
- **A `module` inside a `module` is a compile error** in the main script too: `module 'inner' is inside module 'outer'; modules can't be nested, so define it at the top level of the file`. It used to compile and silently drop the inner module: its statements never ran and its members couldn't be reached. In an imported file it was already an error.
- **Redefining or assigning a builtin is a compile error**: `fn len(x) -> 42`, `len = fn(x) -> 42`, `fn print(x) -> 1`, `sum = 0`, and `max = 1` inside a function. All report `'len' is a builtin function and can't be redefined or assigned; choose another name`. The `fn` forms used to be silently ignored, so `len('ab')` still called the builtin. The value forms failed with a confusing type error.
- A default of `null` for an annotated parameter it can't satisfy (`x: int = null`, `r: result = null`) is a compile error. It used to fail at runtime on every call that omitted the argument.

- **`fn` declarations are fixed** (modules v0.2 spec, rules 17-21). Each of these used to compile and is now an error:
  - Declaring a name twice in one scope. The second definition used to win silently: `fn f(x) -> x`, then `fn f(x) -> x + 1`, then `print(f(1))` printed `2`. Now: `'f' is already defined on line 1`.
  - Assigning to a declared name, as a function or any other value (`f = fn(x) -> 0`, `f = 3`, `f: int = 2`, also from inside another function): `'f' is a function declaration (line 1) and can't be reassigned; use a variable: g = fn ...`.
  - A nested `fn` reusing a name that is visible there: an outer function, variable or parameter (`'f' is already a function in an enclosing scope (line 1); choose another name`), or a builtin (`fn len` inside a function used to shadow `len`; it is now `'len' is a builtin function and can't be redefined or assigned`). Parameters, loop variables and `when` bindings may still shadow.

  To swap a function at runtime, hold it in a variable:
  ```
  g = fn(x) -> x + 1
  g = fn(x) -> 0        // fine: g is a variable
  ```
- **Modules v0.2: `::` names, one scope per file** (`modules_v0_2_spec.md`). Programs that use modules need these changes:

  | Before | Now |
  |---|---|
  | `io.exists(p)`, `p \| io.exists()`, `f = io.exists` | `io::exists(p)`, `p \| io::exists()`, `f = io::exists`. `io.exists` is now an error: `'io' is an imported file; use io::exists` |
  | `incl mathx as m`, then `m.square(9)` (a module in the same file) | `mathx::square(9)`; `incl mathx` is an error |
  | `incl './lib/core'` (no alias), then `double(5)` | `core::helpers::double(5)`: an import binds the file's name and makes nothing visible unqualified. `incl 'std/fmt'` binds `fmt` (`fmt::show_vec`) |
  | `incl './lib/geo.qrk'`, then `Point { x: 1 }`, `p: Point` | `geo::Point { x: 1 }`, `p: geo::Point`. An imported struct prints and has `type()` `geo::Point` (was `Point`) |
  | A library calling a name its importer defines (`counter`), or its importer's struct | An error in the library: `undefined identifier 'counter'`, `unknown type 'P'` |

  An imported file may now contain only definitions: `fn`, `struct`, `module`, `incl`, the `extern` forms, and constants (`X = 3.5`: int, float, str or bool literals and arithmetic on them). Any other statement is a load error, `'lib.qrk' is imported, so it can only contain definitions; line 3 is a statement (print(...))`; it used to run inside the importer. `CACHE = list []` in a library is rejected too. Neither the importer nor the library can assign an imported file's names or constants.

  An imported file's name, and each directory written in a relative or `std/` import path, must be lowercase letters, digits and underscores, starting with a letter: `incl './My Lib/geo-utils'` is an error. The main script is exempt.

  A name can't contain `__` (`'mathx__square': names can't contain '__', which is reserved for names the compiler generates`). Every generated name contains it, so user code can never reach or clash with one; before, `mathx__square` reached the module member and a function named `f__thunk` could clash with the C++ thunk of `f`.

  Within one file these are now errors: a module member named like a top-level name (`module member 'csv::read' has the same name as the top-level function 'read' on line 4`), an import binding named like a definition or another import (`incl './geo'` and `fn geo()`; importing two files both called `geo.qrk` without `as`), two modules with one name, and a struct and a function with one name. So are two names the compiler can't tell apart in generated code, which only happens when a module name ends or a member name starts with `_`: `module m:` / `_x = 1` next to `module m_:` / `x = 2` is `'m_::x' and 'm::_x' (line 2) can't both be defined: the compiler would give them the same internal name; rename one of them` (the same for a main-script `module q1_a` next to `incl './a'`). Runtime messages name module functions with `::` (`function 'io::exists' expects 1 argument but got 0`, was `io.exists`).

- **A function body can't name a `g = fn(...)` variable assigned below it.** `fn a() -> g(1)`, then `g = fn(x) -> x + 1`, used to work because such assignments were predeclared. It is now `undefined identifier 'g'`, as for any variable. Declare it with `fn g(x) -> x + 1` instead, which is hoisted.

- **Renamed keywords and builtins.** The old spellings no longer compile:

  | Old | New |
  |---|---|
  | `use 'std/io' as io`, `use mathx as m` | `incl 'std/io' as io`, `incl mathx as m` |
  | `vector [1, 2, 3]`, `x: vector`, `fn f() vector` | `vec [1, 2, 3]`, `x: vec`, `fn f() vec` |
  | `xs.to_vector()` | `xs.to_vec()` |
  | `println(x)` | `print(x)` |
  | `fmt_vector(v, n, true)` (the `std/fmt` extern binding) | `fmt_vec(v, n, true)`, or `fmt.show_vec(v)` |

  `incl` was chosen because importing a file doesn't "use" everything in it.
- `incl` and `vec` are reserved words now. A variable, parameter or function named `vec` or `incl` is a parse error. `use` and `vector` are ordinary names again.
- **One print function.** `println` is gone. `print(x)` prints `x` followed by `end`, which defaults to `'\n'`, so each call prints its own line (this is what `print(x)` already did). Pass `end` positionally to change it: `print(x, '')` prints no newline, `print(x, ' | ')` a separator. The optional `width`, `align` and `pad` arguments are unchanged.
- **Output says `vec`.** `type(vec [1.5])` returns `'vec[f64]'` (was `'vector[f64]'`), and code that compares `type()` strings must be updated. `fmt.show_vec` ends with `[vec[i64]: 2 elements]`. Compile-time and runtime messages name the type `vec`, for example `vec cannot be used as a condition` and `argument 'x' of 'f' expects vec, got list`.

- New compile errors. Programs that pass a callback with the wrong arity or argument types, or pass a non-function where the body calls the parameter, are now rejected (see *Call shapes* below). They used to compile and then misbehave at runtime.
- Native/extension C++ API:
  - `QClosure` has a new `const QFnInfo* info` field.
  - `q_alloc_closure(fn, info, n)` and `qv_func(fn, info)` take a new `QFnInfo*` argument.
  - `q_require_callable` was replaced by `q_require_closure`.
  - Hand-written glue that builds closures must be updated.
- **`d.key` on a missing dict key is a runtime error** instead of returning `null`: `dict has no key 'nmae'; use .get('nmae') if the key may be missing`. Code that relied on `d.key` reading `null` must use `d.get('key')`.
  ```
  rows = list [dict {name: 'b', amount: 2}]
  for r in rows:
      print(r.amonut)      // used to print null; now stops with an error
  ```
- **Struct field values of unknown type are checked when the literal is built.** `fn mk(a) -> P { x: a }` with `x: int` used to accept `mk('hello')`; it now stops with `field 'x' of P expects int, got str`.
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
- **Each file has its own names** (modules v0.2). Two imported files may both define `helper`, `Point` or `module utils`; a library's own calls to its `helper` reach it whatever the importer defines; and `geo::Point` and `other::Point` are different types. A library function's locals, parameters, loop variables and `when` bindings are unaffected by the importer's names too (`total = 5` in a library function works next to an importer's `fn total`). A file reached through a symbolic link and through its own path is one file, so its structs are one type. Before, top-level names of all files shared one scope, so these were errors (and before that, silent replacements), and `module utils` in two libraries was `module 'utils' already defined`.
  ```
  incl './lib/geo'
  incl './lib/other'
  fn helper() -> 'mine'                 // fine: geo and other have their own helper
  p = geo::Point { x: 1.0, y: 2.0 }
  print(geo::shapes::square(3))         // a member of module shapes in lib/geo.qrk
  ```
- **Any file of definitions is a library**; it no longer needs a `module` block (`does not define a module` is gone). Its top-level names are `file::name`. A module named like its file (the primary module) is also `file::name`, so the stdlib stays two-part (`fmt::show_vec`).
- **Structs in module bodies**, written bare inside the module, `module::Sq` elsewhere in the file and `file::module::Sq` from an importer. This used to fail with `internal: struct 'L' not pre-declared`.
- **Qualified struct literals, annotations and return types**: `geo::Point { x: 1 }`, `fn f(p: geo::Point) geo::Point -> p`. `shapes.Sq { side: 1 }` now says `a struct from a module or imported file is written shapes::Sq { ... }, with '::'` (was `no prefix parse function for LBRACE`).
- Unknown qualified names suggest the closest match: `'fmt' (std/fmt) has no 'show_vector'; did you mean 'show_vec'?`. `geo::io::open` explains that imports are not re-exported.
- Defining a name the stdlib also defines is fine now: `fn fmt_full(x)` or `fmt_vec = 3` next to `incl 'std/fmt'` used to be `'fmt_full' is already defined as an extern fn`. The stdlib's bindings are `fmt::fmt_full`.
- **`fn` declarations are hoisted** (rule 22). Every `fn` of a scope (the file, a module body or a block) exists before the scope's first statement runs, so calls work in any order and mutual recursion needs no particular order. A statement calling a function defined further down used to be a compile error (`'f' is called before its definition on line N`); through another function it failed at runtime.
  ```
  print(is_even(10))   // true
  fn is_even(n) -> ...is_odd(n - 1)...
  fn is_odd(n) -> ...is_even(n - 1)...
  ```
- **Use-before-assignment checks for hoisted functions** (rule 23). Variables are not hoisted: a body only sees variables assigned above it. A call that would run a function before a variable it uses is assigned is rejected when the order is visible, including through other declared functions and pipes:
  ```
  report()             // error: 'report' uses 'total', which is assigned on line 2, after this call
  total = 5
  fn report() -> print(total)
  ```
  Through a function value (`g = report`, then `g()`), it is a runtime error: `'total' is used before it is assigned`, never `null`.
- **Struct literals and struct return types work in an importing file.** With `struct Point` defined at the top level of `lib/geo.qrk`, the importer can write `Point { x: 1 }` and `fn mk(a: int) Point -> ...`. Both used to be parse errors (`no prefix parse function for LBRACE`). A misspelled struct name is now a check error: `unknown struct type 'Pnt'` or `unknown type 'Pnt'`.

### Changed
- **Compile-time arity messages read like the runtime ones**, with the function's name and a correct plural: `function 'f' expects 1 argument but got 2` (was `function expects 1 arguments but got 2`), `method 'split' expects 1 argument but got 0` (was `1 argument(s)`), `builtin 'len' expects 1 argument but got 2 (including piped input)`.
- **A declared function that captures no variables is a plain C++ function.** Calls to it are direct (`quark_f(nullptr, ...)`), with no binding, closure allocation or indirect call. Before, every named function lived in a closure and was called through it, unless it was fully annotated. Whether a function is native (fully annotated) is now decided after the whole program is analyzed, so a fully annotated function that calls another non-capturing function is native too. A declared function that captures a variable stays a closure, held in a binding that never changes.
- A fully annotated lambda assigned to a variable (`g = fn(x: int) int -> x`) is now a closure like any other variable. Only `fn` declarations compile to native C++ functions.
- Tables spec (`tables_v0_1_spec.md`, still not implemented): the literal is `tbl { ... }` and the type annotation is `tbl` (`fn f(t: tbl)`). The runtime type name for a table is `tbl`. `fmt.table(...)` keeps its name.

### Fixed
- **A loop variable named like a `g = fn` variable defined elsewhere failed in clang.** `f = fn(x) -> x + 1`, then inside a function `for f in list [fn(x) -> x * 10]: print(f(2))`, stopped with `type 'QValue' does not provide a call operator`. It now prints `20`: the loop variable was called as the C++ function of the outer `f`.
- `zz.x = 1` with an undefined `zz` reports `undefined identifier 'zz'` once (it was reported twice).
- **Struct parameter and field checks compared names**, so a struct from one file passed the runtime check of a same-named struct from another file once both could exist. They now compare definitions: a `geo2::Point` passed through `any` to a `geo::Point` parameter is `argument 'p' of 'f' expects geo::Point, got geo2::Point`.
- **A file included without an alias by two other files was rejected** (`symbol 'base__twice' from module 'base' conflicts with existing definition`). Each file is loaded once and has one identity, however it is imported.
- **Calling a fully annotated function declared below its caller failed in clang.** `fn a(n) -> b(n) + 1`, then `fn b(n: int) int -> n * 2`, stopped with `assigning to 'QCell *' from incompatible type 'int64_t (QClosure *, int64_t)'`. It now prints `11` for `a(5)`.
- **Reassigning a fully annotated function failed in clang** (`redefinition of 'quark_f'`). It is now the Quark error above.
- **Two functions that each declared a fully annotated helper of the same name failed in clang** (`redefinition of 'quark_helper'`). Each nested declaration now gets its own C++ name.
- **A module function that read a top-level variable failed in clang.** `counter = 7`, then `module cnt:` with `fn get() -> counter`, then `print(c.get())` after `incl cnt as c`, stopped with `use of undeclared identifier 'quark_counter'`. It now prints `7`, and sees later assignments to `counter`.
- **A parameter, loop variable, lambda parameter or `when` binding named like a builtin now shadows it.** (A nested `fn` named like a builtin is an error since the fixed-declaration change above.) Calls used to go to the builtin:
  - `fn f(len) -> len('abc')` called the builtin `len`.
  - `fn g(len) -> 'x' | len()` did the same through a pipe.
  - `fn adder(max) -> (fn(x) -> x + max)` failed in clang (`use of undeclared identifier 'quark_max'`).

  They now use the local binding. `fn clamp(x, min, max)` still works.
- **A captured parameter named like a fully annotated (native) function was called as that function.** With `fn f(x: int) int -> x + 1` defined, `fn g(f) -> (fn(z) -> f(z))` called the top-level `f`, so `g(fn(y) -> y * 100)(10)` printed `11` instead of `1000`.
- **An int stored in a `float` struct field read back as a garbage float.** `P { x: 1 }` with `x: float` stored the int unconverted, and the field read took the float bits: `print(p.x)` printed `5e-324`. Ints are now widened to float in struct literals and in field defaults (`weight: float = 2`), as for parameters.
- **A value of unknown type stored in a typed struct field wasn't checked**, so reading it reinterpreted the wrong type (`p.x + 1` on a string printed a large integer). It is now checked when the struct is built (see Breaking).
- Reading a field of a `null` struct value says so: `cannot read a field of null (expected a struct value)` (was `expected struct, got different type`).
- A function value called with too few or too many arguments, from Quark or through `qext::call`, used to read missing arguments silently.
- `unboxToNative` read the union fields of a native function body's result without checking the tag, which gave garbage values. The read is now checked.
- Fully annotated (native) functions can be called from closures and nested functions.
- `QClosure*` native arguments and returns are boxed with `qv_closure_ptr`. They used to be wrapped in a second closure.

### Internal
- **Compiler cleanup, step 1 of the plan in `architecture.md` §12.8** (no other behavior changes than those above; the full suite passes):
  - AST: `OperatorNode` is split into `AssignNode`, `MemberNode`, `UnaryNode`, `BinaryNode` and `PairNode` (dict literal entries). `fn name(...)` parses to a `FnDeclNode` `[name, lambda]` (`ast.IsFnDecl`) instead of an assignment with `TreeNode.FnDecl`, which is removed. `PipeNode` is removed: `parsePipe` builds the call and sets `TreeNode.Piped`. `quark parse` prints the new node names.
  - Analyzer: `analyzeOperator` (cyclomatic complexity 159) is split into `analyzeMember`, `analyzeUnary`, `analyzeAssign` (`assignName`, `assignMember`, `assignIndex`) and `analyzeBinary` (`analyzeVectorArith`, `analyzeArith`, `analyzeOrdering`, `analyzeEquality`). `analyzePipe` is removed; `analyzeFunctionCall` is the one call path, with arity messages from `checkArity`. Codegen: `generateOperator` is split the same way (`generateAssign`/`assignVar`, `generateMember`, `generateUnary`, `generateBinary`/`scalarBinary`); `generatePipe` is removed. Loader: `walk` is split by node kind, `topDef`/`memberDef` share `defOf`. Lexer: operator tables replace the big switch in `nextRawToken`. Invariants: `ValidateCallPlans` is split per dispatch mode.
  - Dead code removed: the legacy `FunctionNode` path (`analyzeFunction`, `predeclareFunctions`, `declareFunctionSignature`, `Analyzer.functions` and the "legacy" `DispatchDirect` call plans, codegen's `generateFunction`/`generateQValueFunction`/`generateModule`/`funcNames`), the `Analyzer.Get*`, `Errors`, `ErrorDiagnostics`, `WarningDiagnostics` getters, `Analyzer.modules`/`currentModule`, `ModuleLoader.loaded`, `ast.StatementNode`/`ExpressionNode`/`ConditionNode`, `CallPlan.Kind` and `CallPlan.ArgTypesChecked` (with the `INV-ARGCHECK` invariant, which couldn't fail), `Generator.inFunction`/`currentFunc`, `boxResultToQValue`, `boxToQValue`, `unboxToNative`, and several unreachable parser and loader helpers. An `incl` reaching the analyzer or codegen is now an internal error. `escapeCppString` is the only C++ string escaper; `boxNative` replaces `nativeArgToQValue` and `wrapExternReturn`.
  - Runtime: the unreferenced `q_list_size`, `q_list_empty`, `q_list_clear`, `q_list_free`, `q_dict_has`, `q_dict_size`, `q_vec_dtype`, `q_vec_numeric_as_i64`, `q_vec_str_append_null`, `qv_list_init` and the `q_realloc` / `q_malloc_tensor` macros are removed.
  - Tests: `internal/testutil` runs sources through `driver.CompileSource` (it used to repeat the pipeline and analyze trees the loader hadn't finished), and `Analyze` returns a `testutil.Result` with the `types.Analysis` and warnings. New parser tests for pipes and 11 `TestSemanticsSafety_Rejected` cases for pipe misuse (wrong arity, types, `null`, `any`, methods, a module's extern fn, dict members, callbacks), and a `smoke_shadowing.qrk` case for the loop-variable fix.
- Renames: `token.USE`→`token.INCL`, `token.VECTOR`→`token.VEC`, `ast.UseNode`→`ast.InclNode` (`quark parse` prints `Incl`), `parseUse`→`parseIncl`. The catalog key `vector_any` is now `vec_any`.
- Runtime: `q_println` removed; `q_to_vector`→`q_to_vec`, `q_fmt_vector`→`q_fmt_vec`. C++ type and tag names (`QVector`, `VAL_VECTOR`, `qv_vector*`, `qext::as_vector`) are unchanged.
- Editor extension: `incl`/`vec` highlighting and completion, snippets `incl`, `inclf` and `vec`; `data/quark.json` regenerated. The `incl` snippet lists every stdlib module (it offered only `io`, `fmt` and `blas`), and the extension has its own changelog entry for the renames.
- `TestNeg_RenamedSymbolsRejected` checks that the old spellings are rejected.
- Struct field checks: codegen `checkedFieldValue` wraps each field value whose type isn't proven in a new `q_field_int/float/bool/check/struct` helper (`builtins/params.hpp`); proven values are stored unchanged.
- Parser: the struct-name pre-pass (`collectStructNames`, `Parser.structNames`) is removed. `Name {` always parses as a struct literal and any name before `->` as a return type; the analyzer checks the name.
- `q_member_get` (`ops/member.hpp`) reports a missing dict key instead of returning `null`.
- New `smoke_struct_fields.qrk`, `TestStructsAcrossFiles`, and 12 `TestSemanticsSafety_Rejected` cases for struct fields and dict keys.
- Loader `checkTopLevelClashes` (`loader/loader.go`) runs on the fully spliced tree, before module namespacing. It groups top-level definitions by name and source file (`ast.TreeNode.SourceFile`) and reports clashes as `QK-LOAD-001`. The location is in the main file when one side is there, otherwise in the included file.
- Analyzer `refersToBuiltin` / `refersToNative` compare the symbol a name resolves to with the one the builtin, extern fn or native function was declared with (`Analyzer.builtinSyms`, `nativeSyms`). The direct-call, pipe and capture paths use the builtin or native dispatch only when the name isn't shadowed.
- `q_input` returns `qv_ok(str)` / `qv_err(str)`. The catalog return type is `result`, and `inferBuiltinReturnType` types it `result[str, str]`, so `unwrap(input())` is a `str`. `CanAssign(result, null)` is false, and codegen's `paramTagChecks` no longer allows null for `result`. `validateDefaultValues` checks a `null` default as `null`.
- New `smoke_shadowing.qrk`, `TestTopLevelNameClashes`, and 23 `TestSemanticsSafety_Rejected` cases (builtin and extern redefinition, `input()` misuse, null results). `TestInput_EOFAndLongLines` uses the new read loop. Editor `data/quark.json` regenerated (`input` signature).
- `QFnInfo {name, min_arity, max_arity, default_arg}` (`types/closure.hpp`). Codegen `emitFnInfos` emits `_qfi_<name>`, and `emitDefaultArgFns` emits `quark_<name>__default_arg`.
- `types/function.hpp` rewritten:
  - `q_call0`…`q_call12` go through `q_call_values`, which calls directly when the argument count equals the parameter count and uses `q_call_adapt` otherwise.
  - New limit `Q_MAX_CALL_ARGS = 12`.
- `types/analyzer_callshapes.go`: `FunctionType.ParamCalls`, `CallShape`, `recordParamCall`, `checkCallbackArgs`, `markParamReassigned`.
- `types.TypeFn`, `IsCallableType`, `Analysis.ModuleMemberRefs`.
- `CLAUDE.md` gains the *Language Goals*, *Catch It at Compile Time* and *Robustness Bar* sections.
- New `smoke_function_values.qrk`, about 24 new `TestSemanticsSafety_Rejected` cases, and `TestNativeCallback_Arity`.
- **Modules v0.2 (Phase A of the spec's §5.1):**
  - Lexer: `COLONCOLON` (`::`). Parser: `ast.QualifiedNameNode` (literal is the whole path) in expressions; qualified `TypeNode` and `StructLiteralNode` literals; `atReturnType` looks past `a::b` before `->`.
  - New package `names`: `Table` (internal name → `Origin{File, Module, Name, Kind}`), `Display`, `DisplayText`.
  - Loader rewritten: `loader/files.go` (loading by absolute path with file ids, rule 1, rule 15, per-file `def`/`modScope` tables and the clash rules) and `loader/resolve.go` (per-file renaming, `::` resolution with the primary-module rule, constant inlining, splice). `namespace.go`, `checkTopLevelClashes` and the synthetic `incl` nodes are removed. A file's identity is its path with symbolic links resolved (`realPath`). A local of an imported file named like any name of the main script is renamed `q<N>__<name>__l` (`renameLocal`, `names.Local`, displayed as written, also in runtime parameter and unassigned-variable messages). `record` rejects two definitions with one internal name (`reportInternalClash`). `driver.ResolveImports` returns the name table; `Unit.Names`, `Analyzer.SetNames`, `Analysis.Names`.
  - Analyzer: the module-alias machinery (`moduleAliases`, `lookupModuleCall`, `analyzeUse`, `moduleMemberRefs`, `displayMember`, `displayName`) is removed. Module bodies are analyzed in the file scope; structs in module bodies are predeclared; diagnostics carry the node's `SourceFile` and go through `DisplayText`. Rule 23 orders statements per statement list (`symDef.list`).
  - Codegen: `displayFuncName` uses the name table; `q_param_struct(v, &_sdef_X, ...)` and `q_field_struct(v, &_sdef_X, ...)`; `QStructDef.name` is the display name. The `alias.member` closure-call special cases are gone.
  - Runtime: `q_param_struct` / `q_field_struct` take a `const QStructDef*` and compare pointers.
  - Tests: `loader/modules_v02_test.go` (resolution, display names, primary modules, file identity, 31 rejected programs); `TestModuleNamespacing`, `TestNamesArePerFile` (replaces `TestTopLevelNameClashes`) and `TestStructsAcrossFiles` rewritten; smoke programs and inline test sources migrated to `::`. `modules_success` was always skipped (it checked for a `module core` that `lib/core.qrk` never had) and now runs.
  - Editor extension: `::` completion and hover, namespace highlighting for `::` paths, snippets.
- **Fixed `fn` declarations** (`types/analyzer_fixed.go`):
  - The parser still produces `name = fn(...)` for `fn name(...)`, with the new `ast.TreeNode.FnDecl` set.
  - `declareFixedFns` declares a scope's declarations as non-mutable symbols before its statements (replacing `predeclareFunctions` for them; plain `g = fn` assignments are no longer predeclared) and reports rules 18 and 20; rule 19 is checked in `analyzeOperator` and `analyzeVarDecl`. `checkCallsBeforeDefinition` is removed.
  - Call plans to declarations are recorded with `deferFixedDispatch` and resolved by `finalizeFixedFns` once the program is analyzed. It computes which declarations need a closure (least fixpoint over captured symbols, now recorded per lambda in `captureSyms`), assigns C++ names (`FixedFn.CName`: the name for a non-capturing top-level or module declaration, `name__N` otherwise), registers native functions (`nativeFns` is now keyed by `CName`), drops captures of declarations that need no closure, and sets each plan to `DispatchNative`, `DispatchDirect` or `DispatchClosure`. `refersToNative` and `nativeSyms` are removed.
  - `checkDefiniteAssignment` implements rule 23 per scope and records `HoistScope`s: the cells codegen creates unassigned and the closures it creates at the scope's start. The top level and module bodies are one hoisting unit, since they all run in `main`.
  - New `Analysis` fields: `FixedFns`, `FnRefs`, `HoistScopes`, `CheckedCaptures`.
  - Codegen: `hoistScopes` (replaces `predeclareMainFunctionBindings`) runs at the start of `main`, of every block and of the native-function fast path; `fixedFnValue`/`directFnValue` for declarations used as values; `generateIdentifier` reads `FnRefs` and uses `q_cell_get` for checked captures. The first assignment of a captured variable declares its cell before building the value, so `fib = fn(n) -> fib(n - 1)` captures it.
  - Runtime (`core/cell.hpp`): `q_new_unassigned_cell` and `q_cell_get`. `qv_null()` now zeroes its payload so no real null matches the unassigned marker.
  - Tests: new `smoke_fn_declarations.qrk`; 28 `TestSemanticsSafety_Rejected` cases replace the two `call_before_definition` ones; `smoke_shadowing.qrk` drops its nested `fn len`; four codegen tests now expect direct calls.

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
