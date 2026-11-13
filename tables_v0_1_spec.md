# Quark Tables v0.1 Spec

Status: Draft, revision 3 (2026-11-13). The design is the set of decisions taken on 2026-11-13 (§2, D1-D17). The runtime engine (§7), the CSV/TSV reader and writer (§8) the missing-value rule for vectors (D12) and the vector reductions (D13) are implemented and tested (`TestTableEngine`, `TestSemanticsSafety_Rejected`). The compiler side is not: `tbl` is not a keyword or a type yet (`x: tbl = ...` is `unknown type 'tbl'`), and no Quark program can create a table. Revision 3 replaces the 2026-04-02 first draft (literal-first, `orders: Order = tbl { ... }`, `as` aliases, pluggable backends), which is in git history.
Last Updated: 2026-11-13
Related: `structs_v0_1_spec.md` (row types), `architecture.md` §1.6 (the lowering pass and IR the compiler work targets), `ffi_v0_1_spec.md` P5/P6 (Arrow export, DuckDB).

## Contents

1. [Purpose](#1-purpose)
2. [Decisions](#2-decisions)
3. [Type model](#3-type-model) (schema through verbs and functions: §3.1-3.4)
4. [Query surface (planned)](#4-query-surface-planned)
5. [Missing values](#5-missing-values)
6. [Inspection (planned)](#6-inspection-planned)
7. [Runtime engine (implemented)](#7-runtime-engine-implemented)
8. [CSV / TSV (implemented in the runtime)](#8-csv--tsv-implemented-in-the-runtime)
9. [Plan](#9-plan)
10. [Extension: schema inference at compile time](#10-extension-schema-inference-at-compile-time-planned-after-the-first-implementation)
11. [Arrow conformance](#11-arrow-conformance)
12. [Open questions](#12-open-questions)

## 1. Purpose

Tables cover the everyday data-wrangling loop: read tabular files, filter, derive columns, group, summarize, sort, write. A `tbl` is a native, eager, in-memory columnar table for that work. Its row type is a struct, so the compiler checks every column reference and expression in a query, and each query step compiles to a typed loop over column buffers.

Stats and ML come after this. DuckDB is a later, optional backend, not the foundation.

## 2. Decisions

| # | Decision |
|---|---|
| D1 | **Eager, in-memory, columnar.** Every verb runs to completion and returns a new immutable table; there is no lazy plan in v0.1. Columns use the existing `QVector` buffers (Arrow layout: f64/i64 values, bit-packed bool, int32 offsets + UTF-8 for str, validity bitmaps), so `t.salary` is a `vec` without copying. Only the flat primitive layouts are used; no Arrow library. |
| D2 | **Typed by a struct.** The struct gives column names and types, and the compiler checks every column reference and expression against it. The schema flows through each verb, so the compiler knows every intermediate table's columns (rules in §3.1-3.4). There is no separate schema type (decided 2026-11-12: schema is folded into struct). |
| D3 | **Query syntax: one row name per pipe** (`t \| e -> verb(...) \| verb(...)`), re-bound at each verb to that verb's input row. `e` may only appear as `e.col` inside verb arguments. |
| D4 | **Verbs v0.1:** `filter`, `mutate` (add or replace columns), `sort`, `group_by`, `summarize`. No `reduce`: `summarize` without `group_by` aggregates the whole table into one row. `map` over vecs/tables (like `purrr::map`) comes later. |
| D5 | **`summarize` returns an ungrouped table**, so there is no `ungroup()`. `group_by` returns a grouped table that only `summarize` accepts (a compile-time error otherwise). |
| D6 | **Missing values follow `na.rm = TRUE`**: aggregates skip nulls (and NaN in float columns). `sum` of no values is 0; `mean`, `min`, `max` of no values are null; `count()` counts rows. `filter` drops rows whose predicate is null. Sort and group keys put nulls last (NaN after numbers, before nulls). All of this is documented behaviour, not configuration. |
| D7 | **Row indices are int32**: a table holds at most 2,147,483,647 rows. |
| D8 | **CSV/TSV reader written in-house**, parsing straight into typed column buffers; no third-party CSV library. |
| D9 | **Group order is key order** (ascending, nulls last), as in dplyr, so `summarize` output is deterministic. |
| D10 | **The table type is written `tbl[Employee]`**: `staff: tbl[Employee] = ...`, `fn top(t: tbl[Employee]) tbl[Employee] -> ...`. A bare struct annotation (`t: Employee`) always means one struct value, never a table. It is the first bracketed type; `list[T]` follows the same form (D16). |
| D11 | **Queries need a known row type** (first implementation). A query on a table whose row struct the compiler doesn't know (`t = csv::read_file(path)` with no annotation) is a compile-time error that says to give the table a row struct. Such tables can still be printed, rendered with `fmt::table` and written. Runtime-checked queries on them come with the inference extension (§10). |
| D12 | **Missing values stay in columns; a value taken out must be present** (the "middle path"). Vectors and table columns keep their validity mask, so CSV gaps and `vec [1, null, 3]` are allowed (a `null` in a vec literal is a compile error today; `list [1, null, 3].to_vec()` works), and vectorized code handles them (arithmetic and comparisons propagate nulls, aggregates skip them, `filter` drops null predicates). Reading one element as a scalar (`for x in v`, `v[i]`, `to_list()`, `enumerate()`, later a row as a struct) checks it, and a null is a runtime error (`vec element 3 is missing; use fillna or drop_na first`); the check is in the runtime, so it holds for vecs of unknown type too. `v.get(i)` returns `null` for a missing entry and is typed `any`. *Implemented 2026-11-13* (`q_vec_get_scalar`, `q_iter_get`, `q_to_list`, `q_drop_na` in the runtime). There are no nullable types (`T?`). `fillna` stays and `drop_na` is added, as the bridges from "may be missing" to "present". The analyzer may later skip the check where it can prove a vector has no nulls (a literal without `null`, the result of `fillna`, arithmetic on such vectors). |
| D13 | **Vector reductions follow the table aggregate rules** (D6), since `sum(t.salary)` and `summarize(s = sum(e.salary))` must agree: an int or bool vec sums exactly to an int (overflow is a runtime error), a float vec, a float column and each group of one sum their present values pairwise in `double` (decided 2026-11-13: the same bits on every platform, unlike `long double`, at SIMD speed), `min`/`max` keep the element type (strings in byte order), NaN is skipped, and a sum of no values is 0. `min`/`max`/`mean` of a vec with no present values are a runtime error (decided 2026-11-13): a value taken out of a vec must be present (D12), and their results are typed exactly; `summarize` gives a missing value instead, since a column can hold one. The mean is `stats::mean` in `std/stats`, not a prelude builtin, so `mean` stays free as a user name. *Implemented 2026-11-13*; `src/testfiles/table/reduce_test.cpp` checks that the vec reductions and the engine's aggregates agree. |
| D14 | **One null rule for container parameters**, which `tbl` follows. *Done 2026-11-13:* `list`, `dict`, `vec` and `fn` annotations reject `null` everywhere (`architecture.md` §12), so a `tbl[T]` parameter will too. |
| D15 | **The old `std/csv` is removed** (`csv::parse` and `csv::read`, which return lists of str lists). `std/csv` is rebuilt on the new reader (`csv::read_file`, `csv::write_file`, and a text form such as `csv::parse_text`, all with tables). Breaking. Empty and `NA` cells are missing in every column, as in readr. |
| D16 | **`list[T]` is planned.** A typed list of scalars (`list[float]`) would be stored like `vec` today (columnar, with a mask), plain `list` stays `list[any]`, and `vec` can then be folded into `list[T]`. Not part of the first table implementation. |
| D17 | **Columns are matched to struct fields by name; a mapping renames them.** By default a field `id` reads the header column `id`, exactly; a missing column is an error (`employee.csv: no column named 'id' in the header (columns: EmpID, EmpDept, EmpSalary); map it with dict { id: 'EmpID' }`). A literal `dict` argument maps fields to header text: `csv::read_file('employee.csv', dict { id: 'EmpID', dept: 'EmpDept', sal: 'Salary ($)' })`. Its keys are checked against the struct at compile time (an unknown field is a compile error), its values against the header at read time; unmapped fields match by their own name. The mapping must be a literal; a dict built at runtime is a compile error. The table's column names are always the struct's field names. Nothing is matched by position unless the file has no header. |

## 3. Type model

- `tbl[R]`, where `R` is a struct, is a table whose columns are `R`'s fields, in field order, with the field types (D2, D10). Column types are `int`, `float`, `bool` and `str` in v0.1 (`date` and others later, §9).
- A table whose row struct the compiler doesn't know (D11) can be printed, rendered and written, but not queried.
- `t.col` on a `tbl[R]` is the column as a `vec` (D1); an unknown column is a compile-time error.
- A table value is immutable; there is no cell assignment.

### 3.1 Schema through verbs (decided 2026-11-13)

The compiler knows every table's row type: column names, column types and their order. Each verb produces its output row type at compile time:

| Verb | Output row type |
|---|---|
| `filter`, `sort`, `head`, `tail` | Unchanged: `tbl[Emp]` stays `tbl[Emp]` |
| `mutate(bonus = e.salary * 0.1)` | The input's columns, then each new column at the end, in the order written (`bonus: float`) |
| `mutate(salary = e.salary * 1.1)` | A replaced column keeps its position and takes the expression's type, which may differ from the old one: `mutate(id = to_str(e.id))` makes `id` a `str` (common in cleaning, as in dplyr) |
| `group_by(e.dept)` | The input row, marked as grouped by `dept`; only `summarize` accepts it |
| `summarize(avg = mean(e.salary), n = count())` | The group keys first (in `group_by` order, with their types), then the aggregates in the order written: `{dept: str, avg: float, n: int}` |

- **Column types** follow the expression rules (§4): int arithmetic stays int, `/` gives float, `mean` is float, `count` is int, `min`/`max` keep the column's type, `sum` of int or bool is int.
- **Compile-time errors:** two columns with one name in one `mutate` or `summarize`; a summarize column named like a group key; an unknown column; any verb but `summarize` after `group_by`.
- **Grouped tables exist only inside one query chain.** `group_by` must be followed directly by `summarize` in the same chain; `g = t | e -> group_by(e.dept)` on its own is a compile-time error. A grouped table is a compile-time state, not a type a user can annotate, store or pass (may be relaxed later).

### 3.2 Anonymous and named rows

A row type produced by `mutate` or `summarize` is anonymous: generated by the compiler and shown in messages as `{dept: str, avg: float, n: int}`. Anonymous rows match named structs by shape; named structs match only themselves.

- `stats: tbl[DeptStats] = staff | e -> ... | summarize(...)` is accepted when the derived row has exactly `DeptStats`'s field names and types. Strict: no extra and no missing columns (dropping columns is an explicit `select`, so a typo can't lose one silently). Column order doesn't matter: the columns are reordered to the struct's order, which only moves column pointers. From then on the value is a `tbl[DeptStats]`.
- Two named structs never match each other, even with identical fields (`geo::Point` and `other::Point`, as for struct values).
- A mismatch is a compile-time error naming the difference: `the query produces {dept: str, avg: float, n: int}, but DeptStats has no column 'n'`, or `column 'avg' is float, but DeptStats declares it int`.

### 3.3 Functions

- A parameter `t: tbl[Emp]` accepts a `tbl[Emp]`, or a table with an anonymous row of exactly `Emp`'s shape. Queries in the body are checked against `Emp`.
- An unannotated parameter (`fn top(t)`) is `any`; a query on it is the D11 compile-time error (`give the table a row struct: t: tbl[Emp]`).
- A return annotation (`fn stats(t: tbl[Emp]) tbl[DeptStats] ->`) checks the body's derived row against `DeptStats`, as an assignment does.
- Without a return annotation, the inferred return type is the anonymous row, so callers get the full schema. A call above the function's declaration (hoisting) sees only its signature; a query on such a result is an error that suggests adding the return annotation.
- No row polymorphism in v0.1: a parameter can't mean "any table with at least a `salary: float` column". A possible later feature.

### 3.4 At runtime

Tables that pass through `any`, lists, dicts or native code are checked the same way when they reach a `tbl[Emp]` parameter or annotated variable. Every table carries its row definition (`QStructDef`), and every column its physical type. The check passes when the definition is `Emp`'s (pointer identity, as for struct values), or when the row is anonymous and its names and column types match `Emp` exactly, in which case the table is relabeled `Emp` and its columns reordered. Anything else is a runtime error: `argument 't' of 'top' expects tbl[Emp], got tbl[{dept: str, avg: float}]`. The back end emits one static `QStructDef` per anonymous row type, marked as anonymous.

## 4. Query surface (planned)

```quark
struct Employee:
    id: int
    name: str
    dept: str
    salary: float

staff: tbl[Employee] = unwrap(csv::read_file('staff.csv'))

stats = staff | e ->
    filter(e.salary >= 100_000)
    | mutate(bonus = e.salary * 0.1)
    | group_by(e.dept)
    | summarize(avg = mean(e.salary), top = max(e.bonus), n = count())
    | sort(desc(e.avg))            // e is now the summarize row: {dept, avg, top, n}
```

- After `|`, an `ID ->` pair starts a query; the chain parses into one query node. Each step must be a verb: anything else, `e` used as a value (`f(e)`), an unknown column, a type mismatch, an aggregate outside `summarize`, or a non-table source is a compile-time error.
- `name = expr` inside `mutate` and `summarize` defines a column (Quark has no keyword arguments elsewhere).
- Verb expressions are arithmetic, comparisons, `and`/`or`/`!`, literals and outer scalar variables in v0.1. Lowering compiles each one into a typed loop over the column buffers (no expression interpreter, no temporary columns).
- Aggregates: `count()`, `count(e.col)` (non-missing values), `sum`, `mean`, `min`, `max`. `sum` of int is int (overflow is a runtime error), of bool counts true values, of float is float; `mean` is float (a bool mean is the share of true values); `min`/`max` keep the column's type and also work on str (byte order).
- `sort(e.a, desc(e.b))`: stable, several keys.
- `100_000` needs digit separators in the lexer (planned with the query work).

## 5. Missing values

Collected from D6, D12 and D15, since every part of the design depends on them:

- A column may hold missing values; the mask is the Arrow validity bitmap. CSV cells that are empty or `NA` are missing in every column type.
- Vectorized operations propagate them: arithmetic and comparisons give a missing value where an input is missing.
- Aggregates skip them (and NaN in float columns): `sum` of nothing is 0, `mean`/`min`/`max` of nothing are null, `count()` counts rows, `count(e.col)` counts present values.
- `filter` drops a row whose predicate is missing.
- Sort and group keys put missing values last (NaN after numbers, before missing).
- Taking one value out as a scalar checks it: a missing value is a runtime error that names `fillna` and `drop_na`. There are no nullable types.

This is fixed behaviour, documented for users, not an option.

## 6. Inspection (planned)

- `print(t)` shows the row count, the column names with types, and the first 10 rows (like a tibble). Today the runtime prints a table as `[table<T> rows=N]`.
- `head(t, n = 6)` / `tail(t, n = 6)` return a `tbl`, and also work as query steps.
- `describe(t, n = 5)` prints one line per column: name, type, and the first `n` values (R `str`, `dplyr::glimpse`).
- `summary(t)` returns a table with one row per column, which prints like any table. Number columns get count, missing, mean, sd, min, q1, median, q3 and max; str and bool columns get count, missing, distinct count and the most common value. It is computed with the engine's aggregates, so it follows §5. Median and quartiles need a quantile kernel (on a sorted copy of the present values, R's default type 7 interpolation), which the engine doesn't have yet.

## 7. Runtime engine (implemented)

`runtime/include/quark/table/engine.hpp` (namespace `quark::tbl`), header-only and not part of `quark.hpp` (generated programs include it only when they use tables). A table is a `QTable` (`types/table.hpp`): a `QStructDef` naming the columns, the row count and one `QVector*` per column; its value tag is `VAL_TABLE`.

| Function | Does |
|---|---|
| `make_table(def, cols)` | Builds a table from equal-length columns |
| `take` / `take_table(t, idx, n)` | Gathers rows by index (filter, sort); taking every row in order returns the input |
| `sort_perm` / `sort_table(t, keys, n)` | Stable multi-key sort; a fast path for one dense numeric key |
| `group_rows(t, key_cols, n)` | Hash grouping (open addressing), groups renumbered in key order; `whole_table(n)` for summarize without `group_by` |
| `group_keys(t, g, col)` | The key column of the summarize output |
| `agg_count`, `agg_count_values`, `agg_sum`, `agg_mean`, `agg_min`, `agg_max` | Per-group aggregates (§5) |

Filter predicates and mutate expressions are not engine functions: generated code runs their loops and feeds the selected row indices to `take_table`.

Measured on 500K rows (`-O2`, one core, Linux x86_64, 2026-11-13): `group_by` + `mean` 23 ms, sort by one float key 97 ms, sort by two keys 271 ms.

## 8. CSV / TSV (implemented in the runtime)

`runtime/include/quark/table/csv.hpp` (namespace `quark::csv`). Not reachable from Quark until `std/csv` is rebuilt on it (D15).

- `read_table(path, options, def, schema)` / `parse_table(text, ...)`: typed when a schema (column names and types) is given, otherwise every column's type is inferred from all of its values (int, else float, else bool, else str).
- `write_table(t, path, options)` / `format_table(t, sep)`.
- Accepted input: `,` or `\t` (from a `.tsv`/`.tab` path, or set explicitly); quoted fields with separators, line breaks and `""`; `\n`, `\r\n` and `\r` line endings; a UTF-8 BOM; blank lines (skipped); spaces around values and quotes (trimmed). `""` and `NA`, quoted or not, are missing in every column (as in readr); the list is an option. Header columns are matched to the schema by name, so a file may have more columns than the schema needs; without a header, by position.
- Errors name the file, line, column and text, and count the rest: `staff.csv:412: column 'salary': '12k' is not a float (and 3 more problems)`. A row with the wrong number of fields, an unterminated quote, text after a closing quote, a missing or duplicated column, an empty file and an int that overflows are all errors; nothing becomes null silently.
- Writing: a header line, missing values as empty fields, strings quoted only when needed, floats in shortest round-trip form.
- Measured: a 27.6 MB file of 500K rows reads in 219 ms typed and 480 ms inferred.
- Not yet: the D17 field-to-header mapping, and UTF-8 validation (§9).

## 9. Plan

**Prerequisites, before `tbl` compiler work starts:**
1. CSV reader: validate UTF-8 in str columns (Arrow's `utf8` requires it); report `line 12: column 'name' is not valid UTF-8 (the file may be Latin-1)`, or transcode with an encoding option.

**The first implementation**, in the lowering pass and IR (`lower/`, `emit/`, `architecture.md` §1.6):
- The `tbl[T]` type (D10), the query node and verb checks (D3-D5, D11), digit separators.
- `std/csv` rebuilt on the new reader, with the D17 mapping (D15).
- Printing and the inspection functions (§6).

**After v0.1:**
- Verbs: joins, `select`/`rename`, `distinct`, `n_distinct`, `slice`/`head`.
- Column types: `date` (int32 days, Arrow `date32`), then timestamps and categoricals.
- `map` over vecs and tables (D4), `list[T]` (D16).
- DuckDB as an optional backend (`ffi_v0_1_spec.md` §6). User-facing semantics must stay the same across backends; only performance and where the work runs may differ.

## 10. Extension: schema inference at compile time (planned, after the first implementation)

When the path is a string literal (`staff = csv::read_file('staff.csv')`), the compiler reads the file while compiling, infers the row struct the same way the runtime does (D8, all rows scanned), and checks every query against it, like F# type providers. At runtime the reader checks the file still matches the inferred types, and a mismatch is a runtime error naming the column.

- A compiler flag skips inference to keep compiles fast; reads are then typed at runtime only.
- If the file can't be read at compile time (missing, no permission), the compiler emits a warning and the read is typed at runtime.
- A path the compiler can't know (`read_file(args[0])`) gets no inference and is handled entirely at runtime. With this extension, queries on such tables become possible with runtime column checks (lifting D11).
- A column whose values aren't all one type (`'123', 'abc', 9.5`) is inferred as `str`, with a note saying why: in a CSV every value is text.

## 11. Arrow conformance

Columns follow the Arrow columnar format for the types used: float64 (`g`), int64 (`l`), boolean (`b`, bit-packed LSB first), utf8 (`u`, int32 offsets); a table is a struct array (`+s`) or record batch with no struct-level nulls; `date` will be date32 (`tdD`). Validity bitmaps are LSB first and absent when there are no nulls; buffers are 64-byte aligned; offsets are 0; arrays are immutable. D12 changes no layout.

Known differences:
- Buffer lengths are exact, not padded to 64 bytes (Arrow recommends it; the IPC format requires 8-byte padding). In-memory sharing through the C Data Interface doesn't need it; writing IPC/Parquet would pad while serializing.
- str columns aren't yet checked for valid UTF-8 (§9, prerequisite 1).
- int32 offsets cap a str column at 2 GiB of text (Arrow `large_utf8` isn't used).
- Conformance is checked byte by byte (`TestVectorArrowLayout`), not yet against an Arrow implementation. FFI P5 (the C Data Interface export) should come with a round-trip test through pyarrow or the Arrow C library, and must keep exported buffers alive for the GC until the consumer's release callback runs.

## 12. Open questions

1. **Table literals.** The first draft proposed `tbl { ... }` literals in three forms: rows mapped positionally to the struct fields, a typed header line followed by rows, and a header followed by one column source (a vec or list) per column. Under D10 that would read `orders: tbl[Order] = tbl { 1, 'c1', 10.0, 2 ... }`. Are literals needed in v0.1, given that tables come from CSV files, and which forms? `tbl` would become a keyword (it isn't one today).
2. **Row access.** Should `t[i]` return the row as a struct value (D12 already says a missing value in it is a runtime error)? And is an out-of-range `t[i]` fatal or `null`? Today the two sequence types disagree: `list [1, 2][5]` is `null`, `vec [1, 2][5]` is a runtime error. Tables should follow whichever rule the language settles on.
3. **Slicing and masks outside queries.** The first draft proposed `t[0:10]` (Quark has no slice syntax; `head`/`tail`/`slice` are planned as verbs) and `t[t.qty > 1]` (a `vec[bool]` mask, as for vectors). Keep the mask form alongside `filter`, or only `filter`?
4. **An annotation for a table of unknown row type.** D11 allows such tables to be printed and written. Can a parameter accept one (a bare `tbl`), or must every annotation name a row struct?
5. **Diagnostics.** Tables reuse `QK-PARSE-001`, `QK-CHECK-001` and `QK-RUNTIME-001`; does the CSV reader's error deserve its own code (it is an environment failure returned as an `err`, not a crash)?
6. **Typed lists, `vec` and `list[any]`.** Quark has three sequence kinds: typed lists (`list[int]`, element type fixed at creation, boxed `QValue` elements), `vec` (columnar, with a missing-value mask) and lists of any value (`xs: list = ...`). Is it worth keeping all three? D16 would fold `vec` into `list[T]`; the other direction is a slow but Python-like `list[any]` as the general list, with `vec`/`list[T]` as the fast typed form. To decide after the first table implementation, with the D12 behaviour (a missing entry taken out is an error, `to_list()` of a vec with missing entries included) as a constraint.
