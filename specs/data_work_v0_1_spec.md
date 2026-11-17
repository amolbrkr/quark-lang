# Quark Data Work Improvements v0.1 Spec

Status: Proposal. Nothing is implemented. The parts: sorting a vec (§3), reading CSV within memory (§4), joins (§5), calling an imported function by its bare name (§6), backtick names for columns and fields (§7), TSV and other separators (§8), JSON (§9), what Excel support must answer (§10, waiting for research), and a benchmark suite that shows where Quark stands against the tools it competes with, on a laptop-sized machine (§11-§17). The suite replaces the one described in `src/benchmarks/README.md`. §6-§9 have open questions (§22).
Last Updated: 2026-11-17
Related: `src/benchmarks/README.md` and `src/benchmarks/RESULTS.md` (the current suite), `specs/tables_v0_3_spec.md` (aggregates in verbs, `contains`, threads), `specs/implemented/modules_v0_2_spec.md` (imports and name lookup, for §6), `specs/untyped_tables_v0_1_spec.md` (queries on tables read without a row struct, for §7), `stdlib.md` (vecs, `std/csv`, `std/blas`), `architecture.md` §15 (performance findings), and the DuckDB Labs database-like operations benchmark (db-benchmark), whose method §11 follows.

## Contents

1. [Purpose](#1-purpose)
2. [Decisions](#2-decisions)
3. [Sorting a vec](#3-sorting-a-vec)
4. [Reading CSV within memory](#4-reading-csv-within-memory)
5. [Joins](#5-joins)
6. [Bare names from an imported file](#6-bare-names-from-an-imported-file)
7. [Backtick names for columns and fields](#7-backtick-names-for-columns-and-fields)
8. [TSV and other separators](#8-tsv-and-other-separators)
9. [JSON](#9-json)
10. [Excel (waiting for research)](#10-excel-waiting-for-research)
11. [Benchmark method](#11-benchmark-method)
12. [Parity](#12-parity)
13. [Data](#13-data)
14. [Questions](#14-questions)
15. [What is removed](#15-what-is-removed)
16. [What waits for a Quark feature](#16-what-waits-for-a-quark-feature)
17. [What the 100M preset will show](#17-what-the-100m-preset-will-show)
18. [Changes to other docs](#18-changes-to-other-docs)
19. [Tests](#19-tests)
20. [Plan](#20-plan)
21. [Later](#21-later)
22. [Open questions](#22-open-questions)

## 1. Purpose

The current suite measures many small things (21 benchmarks, most on 1M rows or 10M values, several under 10 ms), and in places it measures a Quark workaround instead of a Quark operation: vecs have no sort, so `sort`'s Quark `vec` time is `v.to_list().sort().to_vec()`, and `join` compares a `dict` loop with real data frame joins. Small inputs mostly measure fixed costs (setting up hash tables, allocating, starting threads), not the throughput that matters on real data. And headline claims such as "`groupby` 0.96 times Polars" rest on 5 runs with no measure of their spread.

Working with real files shows three more gaps. Every call of a library function carries its file's name (`csv::read_file`), which is noise in a short script. Real headers often aren't plain names (`Employee Name`, `dept-id`, `result`), so a table read without a row struct has columns that code can't name. And CSV is the only format: tab-separated text can't be parsed, and there is no JSON.

This spec adds what Quark is missing for that work, and replaces the suite:
- A sort and a sorting permutation for vecs (§3).
- A CSV reader whose peak memory is close to the size of the table it builds (§4); today it is about 3.7 times the file.
- Joins: semi, anti, inner and left, on equal keys (§5).
- Calling a function of an imported file by its bare name, when the file lists it (`incl csv::read_file`), with `::` still the canonical form (§6).
- Backtick names for columns and fields whose names aren't plain names: `` e.`Employee Name` ``, `` e.`result` `` (§7).
- Tab-separated text, and separators other than a comma or a tab, in `std/csv` (§8); `std/json` for records, JSON Lines and any JSON value (§9). Excel comes once its implementation is chosen (§10).
- Fewer benchmark tasks, with questions of increasing difficulty borrowed from db-benchmark, on 10M and 100M rows, run as on a laptop with 8 or 16 GB of memory, measuring only operations that Quark and its peers each do natively (§11-§17).

## 2. Decisions

| # | Decision |
|---|---|
| DW1 | **Vecs have `sort()` and `sort_desc()`**, which return a new sorted vec in the order a table sort uses: stable, missing values last in both directions, NaN after every number and before missing values. The table engine's single-key sort uses the same kernel (§3). |
| DW2 | **Vecs have `order()` and `order_desc()`**, the stable 0-based positions that sort the vec, as a `vec[int]` (NumPy's `argsort`, R's `order`; decided 2026-11-17). |
| DW3 | **Reading a CSV file peaks at no more than 1.25 times the finished table's columns, plus 64 MB and 16 MB per thread** (§4; decided 2026-11-17). |
| DW4 | **Joins are part of this spec**: `semi_join`, `anti_join`, `inner_join` and `left_join` (§5; decided 2026-11-17). Full joins come later. |
| DW5 | **Loading data is never timed**, except in the read questions, where reading is the work (§14.1). The current suite already follows this rule; it stays. |
| DW6 | **A question is in the suite only when Quark does it with its own operation**, and each peer runs it only with its own native operation. No conversion to another type, and no hand-written algorithm where the others call a built-in. A peer without one is `n/a` for that question; a question Quark can't do natively waits (§12, §16). |
| DW7 | **The reference machine is a laptop**: 8 logical CPUs, and 8 GB of memory for the `10m` preset or 16 GB for `100m`. A process may use 6 GB or 12 GB. A run above that is reported as out of memory and isn't ranked (§11.2). |
| DW8 | **Two presets: `10m` (the default) and `100m`.** Data larger than memory is out of scope. |
| DW9 | **Tasks have two or three questions of increasing difficulty**, after db-benchmark (§14). |
| DW10 | **Every data question runs on two variants of the table**: `G` with no missing values and `G-NA` with 5% missing in every column, as db-benchmark does (§13.2; decided 2026-11-17). |
| DW11 | **The first run of each question is reported on its own**, next to the median of the later runs, because a script runs each step once (§11.3). |
| DW12 | **Two thread settings.** 8 threads, the reference laptop, which is the headline once Quark runs queries on several threads (`specs/tables_v0_3_spec.md` §8); and 1 thread, which compares the code. |
| DW13 | **A ratio is a tie when the two time ranges overlap.** The report and `RESULTS.md` don't call one tool faster than another inside a tie (§11.4). |
| DW14 | **The peers are NumPy, pandas, Polars, base R and data.table.** DuckDB isn't included; Polars stands in for an in-memory columnar engine (decided 2026-11-17). The list tier (`quark_list`, `py_list`, `r_loop`) and base R's `data.frame` leave the suite (§15). |
| DW15 | **`fib` stays**, as the measure of plain looping and function-call performance (§14.10; decided 2026-11-17). |
| DW16 | **One BLAS benchmark stays, `gemm`**, to measure `std/blas` against NumPy and R on the same kind of library. It grows with a matrix type, when Quark has one (§14.11; decided 2026-11-17). |
| DW17 | **A join is a query step that names the other table's row**: `left \| a -> semi_join(right \| b, a.id == b.key_id)`. `b` exists only inside the step; a right side that is itself a query is put in parentheses (§5.1; decided 2026-11-17). |
| DW18 | **A join condition is one or more `a.col == b.col` joined by `and`.** Anything else is a compile-time error that says to `filter` after the join (decided 2026-11-17). |
| DW19 | **The two keys of each pair have exactly the same type** in joins v0.1 (decided 2026-11-17). |
| DW20 | **A missing key never matches**, as `==` on a missing value is never true in a verb (decided 2026-11-17). |
| DW21 | **Joined columns never clash silently.** An inner or left join keeps the left table's columns, then the right's other than its keys; a right column named like a left one is a compile-time error that names it and suggests `rename` (§5.3; decided 2026-11-17). |
| DW22 | **Keyword arguments are not part of this spec**; they come with other syntax changes to the language. `sort_desc` and `order_desc` stand until then (decided 2026-11-17). |
| DW23 | **Joins v0.1 run on one thread, and nothing else does because of them.** Only the join step is single-threaded: a query's other steps, and every query without a join, run on the thread count in force (the default, `--threads N` or `QUARK_THREADS`; `specs/tables_v0_3_spec.md` D54). The join is built in blocks so that a parallel version changes no output (§5.5; decided 2026-11-17). |
| DW24 | **`::` stays the canonical way to name what an imported file defines, and `.` stays for fields and methods**, so `csv.read_file` stays an error. A file may also call a member by its bare name (`read_file(p)`); how the name is bound, and what happens when it clashes, is §6 (decided 2026-11-17; §6's design is open). |
| DW25 | **TSV and JSON are in this spec (§8, §9). Excel waits** until the user has researched how best to read and write it in C++ (§10; decided 2026-11-17). |
| DW26 | **Bare names (§6) and backtick names (§7) are in this spec** as data work changes, although both change the syntax. Keyword arguments stay with the other syntax changes (DW22) (decided 2026-11-17). |

## 3. Sorting a vec

### 3.1 Surface

```quark
v = vec [3.5, null, -1.0, 2.0]
print(v.sort())         // [-1.0, 2.0, 3.5, null]
print(v.sort_desc())    // [3.5, 2.0, -1.0, null]
print(v.order())        // [2, 3, 0, 1]
print(v.order_desc())   // [0, 3, 2, 1]
```

- `sort` and `sort_desc` return a new vec of the same element type and length; `v` doesn't change (vecs are immutable). `order` and `order_desc` return a `vec[int]` of the same length with no missing values: position `i` of the result is the position in `v` of the `i`-th value in sorted order. `v.sort()` has the same values as `v` read at `v.order()`. Lists keep their in-place `.sort()`.
- Descending is a separate method because Quark has no keyword arguments (`grammar.md`), so `v.sort(desc = true)` can't be written, and `v.sort(true)` wouldn't say what `true` means. If keyword arguments come, `sort(desc = true)` can replace `sort_desc`.
- Element types: `int`, `float`, `bool` (`false` first), `str` (byte order, as a list or table sort) and `date`.
- The order is the table sort's (semantics.md §9.5): stable, missing values last in either direction, NaN after every number and before missing values. NaNs keep their input order, and so do `-0.0` and `0.0`, which are equal.
- An empty vec, or one with only missing values, sorts to itself. On a vec whose element type the compiler doesn't know, the kernel is picked at runtime. A wrong argument count is a compile-time error. All four methods are row-safe (`specs/tables_v0_3_spec.md` D41); in a verb, `t.x.sort()` reads no row, so it runs once per step (D44).

### 3.2 Kernel

- `int`, `float` and `date`: a stable LSD radix sort on 64-bit keys (`date`: 32-bit), with the sign bit flipped for ints and the IEEE-754 transform for floats. `-0.0` is keyed as `0.0`. A pass whose byte is the same in every key is skipped, so small ints take fewer passes. NaNs and missing values are split off first and appended in input order. `order` sorts (key, position) pairs the same way.
- `bool`: a count.
- `str`: a stable sort of (offset, length) views by byte order; the algorithm (MSD radix, or a merge sort with a memcmp compare) is chosen by measurement.
- The table engine's sort by one key, including the current "one numeric key without missing values" fast path, calls the same kernel, so `t | e -> sort(e.x)` and `t.x.sort()` agree by construction.
- Threads: `v.sort()` and `v.order()` run on one thread in v0.1. The radix sort parallelizes without changing its output: each block of keys counts its digits, prefix sums give every block its offsets, and the scatter keeps input order (`specs/tables_v0_3_spec.md` D53). The table engine's sort moves to this kernel only once the kernel runs on the thread pool, so a table sort that runs on several threads never goes back to one.

### 3.3 Performance target

On one thread, 10M values: `float` and `int` within 1.5 times NumPy's `np.sort` (and `order` within 1.5 times `np.argsort(kind='stable')`); `str` within 1.5 times Polars' `Series.sort`. Today a one-column table sort of 1M floats takes 6.5 times `np.sort` (`RESULTS.md`).

## 4. Reading CSV within memory

### 4.1 Today

`csv::read_file` reads the whole file into one `std::string`, which grows by doubling (`read_file`, `table/csv.hpp`). An untyped read then scans that text twice: once to infer the column types, once to build the columns. The columns are built in `std::vector` builders that also grow by doubling, and are copied into the table's vecs at the end, while the text is still held. A 157 MB file of 10M rows (a float, an int and a short `str` column) peaks at 589 MB of resident memory, against about 235 MB for the finished columns: 3.7 times the file, 2.5 times the columns. For the 4.1 GB file of §13 that is about 15 GB, more than a 16 GB laptop gives one process (`known_issues.md`).

### 4.2 The reader

- **Blocks.** The file is read in blocks of 16 MB. Only the current block is held, plus the end of a row that runs past it, which may be inside a quoted field. A row longer than a block grows the carried part until the row ends.
- **Passes.** An untyped read streams the file twice: the first pass infers the types (all rows, D8) and counts the rows; the second builds the columns. A typed read streams it once.
- **Final buffers.** Each column is built in the buffer the table keeps, with no builder to copy from. An untyped read knows the row count from its first pass and allocates each buffer once. A typed read estimates the row count from the file size and the first block's bytes per row, doubles a buffer when the estimate is short, and trims it at the end. A `str` column's text grows the same way.
- **Unchanged:** what is accepted and inferred, the error messages and their line numbers, the UTF-8 and NUL checks, `columns` mappings, and the 2 GiB limit on one `str` column's text.
- **`csv::parse_text`**, whose text is already in memory, uses the same final buffers.
- **Threads.** The reader parses on the thread pool, as `specs/tables_v0_3_spec.md` §8.1 plans for it (it runs on one thread today): it reads as many blocks as there are threads, splits them at line ends outside quotes (it knows whether each block starts inside a quoted field from the one before), and parses the parts on the thread pool, writing each part's rows at offsets known from a count of its rows. On one thread it is one block at a time. The output and the errors (the first problem in line order) are the same for every thread count.

### 4.3 Targets

- Peak resident memory: at most 1.25 times the finished columns, plus 64 MB and one 16 MB block per thread (DW3).
- Speed: a typed read is no slower than today; an untyped read's second pass over the file is measured, and is at most 20% slower than today's.

## 5. Joins

### 5.1 Syntax

A join is a query step. Its first argument is the other table with a name for its row, and its second is the condition that matches a row of each:

```quark
struct Order:
    id: int
    cust: int
    amount: float

struct Customer:
    cust_id: int
    region: str

orders: tbl[Order] = unwrap(csv::read_file('orders.csv'))
customers: tbl[Customer] = unwrap(csv::read_file('customers.csv'))

known = orders | o -> semi_join(customers | c, o.cust == c.cust_id)     // orders with a customer
orphans = orders | o -> anti_join(customers | c, o.cust == c.cust_id)   // orders without one
both = orders | o -> inner_join(customers | c, o.cust == c.cust_id)     // id, cust, amount, region
kept = orders | o -> left_join(customers | c, o.cust == c.cust_id)      // region missing where unmatched

by_region = orders | o ->
    inner_join(customers | c, o.cust == c.cust_id)
    | group_by(o.region)
    | summarize(total = sum(o.amount))
```

- The verb is `semi_join`, `anti_join`, `inner_join` or `left_join`.
- `right | b`: `right` is any expression whose value is a table of a known row type; `b` names its row. `b` must differ from the query's row name and, like it, shadows an outer variable of that name. `b` exists only inside the join step; after it, the query's row name (`o` above) names the joined row.
- A right side that is itself a query is put in parentheses, as a query's result is anywhere else: `semi_join((customers | c -> filter(c.active)) | c, o.cust == c.cust_id)`. Without them, the `| c` would be read as the next step, and the error says to add them.
- Forgetting the row name (`semi_join(customers, ...)`) is a compile-time error that shows the form with one.
- `t2 | b` is a parse error today (a pipe's right side must be a call), so the syntax changes no existing program.

### 5.2 The condition

- One or more `a.col == b.col` joined by `and`, where `a` is the query's row and `b` the right side's; either may be written first (`c.cust_id == o.cust`). Anything else, such as `<`, `or`, arithmetic on a key, or a column compared with a constant, is a compile-time error: `a join condition is one or more 'o.col == c.col' joined with 'and'; filter the result for other conditions`.
- Each column must exist on its side; an unknown one is a compile-time error that lists that side's columns.
- The two keys of a pair have exactly the same type, one of `int`, `float`, `bool`, `str` and `date` (DW19): `join keys o.cust (int) and c.cust_id (str) must have the same type`. An int key against a float key is this error too; convert one with `mutate` before the join.
- A missing key never matches (DW20), on either side. A float key compares as `==` does: NaN never matches, and `-0.0` matches `0.0`.

### 5.3 The result

| Join | Rows | Row type |
|---|---|---|
| `semi_join` | Each left row that has at least one match, once, in left order | The left table's |
| `anti_join` | Each left row that has no match, in left order; a row with a missing key has none | The left table's |
| `inner_join` | One row per matching pair, in left order, and for one left row in right order | Joined (below) |
| `left_join` | As `inner_join`, plus each left row with no match, in its place, with the right's columns missing | Joined (below) |

- **Joined rows** have the left table's columns, in order, then the right table's columns other than its keys, in order. A right key equals its left key in every joined row, so it isn't kept a second time, whatever it is called; the left key keeps its name. The row is anonymous (`tbl[{id: int, cust: int, amount: float, region: str}]`) and matches a named struct by shape where it is stored, like every derived row (semantics.md §9.5).
- **A clash is an error.** A right column other than a key that has the name of a left column is a compile-time error: `both tables have a column 'name'; rename one before the join, for example (customers | c -> rename(cust_name = c.name)) | c`.
- **Duplicate keys** on both sides give every matching pair, as in SQL. A result of more than 2,147,483,647 rows (row indices are 32-bit) is a runtime error.
- The output is the same for every thread count (`specs/tables_v0_3_spec.md` D53), since the order is defined above.

### 5.4 What the compiler checks, and how it runs

- **Both row types must be known.** A join on a table whose row type the compiler doesn't know is a compile-time error, as any query on one is today (`specs/untyped_tables_v0_1_spec.md` proposes queries on such tables).
- **The right side is evaluated once**, when the step starts and before any row is read; it can't refer to the query's row. It isn't a row expression, so the row-safety rule (`specs/tables_v0_3_spec.md` D41) doesn't apply to it.
- `group_by` followed by a join is a compile-time error, as `group_by` followed by any step other than `filter`, `mutate`, `summarize` or `count` is (D36).
- **Execution**: a hash join in the table engine (`table/engine.hpp`). The engine builds a hash table on the right side's keys, leaving out rows with a missing key, then probes it with the left rows in order (a hash set for `semi_join` and `anti_join`). The step reads only columns, so no per-row user code runs. `str` keys compare by bytes.

### 5.5 Threads

Joins v0.1 run on one thread (DW23). That is the join step only: the steps before and after it in the same query, and every query without a join, run on the thread count in force, as `specs/tables_v0_3_spec.md` §8 specifies.

The single-threaded join is written in the shape the parallel one will have, so turning the thread pool on changes no output and no error:

- **Build.** The right table's rows are split into partitions by the hash of their key, and each partition gets its own hash table. Each block of right rows counts its rows per partition, prefix sums give each block its offsets, and the scatter keeps right order within a partition. On one thread there is one partition.
- **Probe.** The left rows are probed in blocks of 65,536 (`specs/tables_v0_3_spec.md` D55). Each block first counts its output rows, then writes its (left row, right row) pairs, in left order and then right order, at the offset the prefix sum of the counts gives it. `semi_join` and `anti_join` write the kept left rows instead.
- **Size first.** The output's size is known from the counts before anything is written, so a result past the row limit is the same error at every thread count (D56), and the output columns are allocated once.
- **Gather.** The output columns are built from the pairs, which v0.3 already runs on the pool (§8.1, building output columns).

When the parallel version lands, `--checked` (D57) compares it with one thread, and `specs/tables_v0_3_spec.md` §8.1 lists joins.

## 6. Bare names from an imported file

### 6.1 Today

Every name of an imported file is written with the file's binding: `csv::read_file(p)` (`specs/implemented/modules_v0_2_spec.md`, rule 7). Nothing from another file is visible unqualified (rule 12, and CLAUDE.md's Robustness Bar rule 6), and `.` is only for fields and methods (rule 10), so `csv.read_file` is an error that points to `::`. The qualifier shows where every call goes, but a short script that calls a few library functions repeats it on every call.

`::` stays the canonical form (DW24): the stdlib, the documentation and every diagnostic write `csv::read_file`, and `.` doesn't change.

### 6.2 Proposal: names listed in an `incl`

```quark
incl 'std/csv'
incl 'std/json'
incl csv::read_file, csv::write_file
incl json::read_file as read_json

staff = unwrap(read_file('staff.csv'))       // csv::read_file
events = unwrap(read_json('events.jsonl'))   // json::read_file
write_file(staff, 'staff.tsv')               // csv::write_file
```

- `incl` followed by a qualified name, not a string, binds that name bare in this file, under its last part or its `as` name. Several names may be listed, separated by commas, each with its own `as`.
- The qualified name is resolved as it is everywhere (rules 7 and 8): `binding::name` or `binding::module::name`, where `binding` is an `incl 'path'` of the same file (its stem or its `as` name). That `incl 'path'` may be anywhere in the file, since a file's names are all known before any is resolved.
- Any top-level name of an imported file, or member of one of its modules, can be bound: a function, an `extern fn`, a struct or a constant. A module can't (`incl formats::csv` is an error; §21), and neither can a member of a module of the same file, which is written `m::name` as today.
- A bound name is the definition it names. Diagnostics, `type()`, the editor's hover and runtime messages show `csv::read_file`. Call shapes, default arguments and the typed read (§9.5) apply unchanged, because the loader resolves both spellings to the same internal name; the analyzer and the back end see no difference.
- The binding belongs to the file that writes it and, like an import binding, isn't re-exported (rule 11): an importer of this file can't write `this::read_file`.
- Both spellings stay valid in the file: `csv::read_file` is the same function.

### 6.3 Clashes

Each case is decided when the program is loaded, in the file that binds the name:

| Case | Result |
|---|---|
| The file defines `read_file` (a function, struct, `extern fn`, constant or top-level variable) and also binds `csv::read_file` | A load error naming both lines: `'read_file' is bound to csv::read_file on line 3 and defined on line 9; bind it under another name (incl csv::read_file as read_csv) or rename the function` |
| Two bindings of one name (`incl csv::read_file` and `incl json::read_file`) | A load error; `as` gives one of them another name |
| A bound name equal to a prelude builtin (`incl geo::len`) | An error, as a definition of that name is today (`'len' is a builtin function and can't be redefined or assigned`) |
| A bound name equal to an import binding or a module of the file (`incl geo::csv` beside `incl 'std/csv'`) | A load error (rule 14) |
| A bound name equal to an aggregate name (`incl lib::count`) | In a verb, `count(...)` is the error a definition of `count` gives there today (semantics.md §9.5); outside verbs it is the bound function |
| A local named `read_file` (a parameter, loop variable, `when` binding, or variable assigned in a function) | The local wins in its scope, as a local already wins over a top-level function (`fn g(helper) -> helper * 2` beside `fn helper`). No new rule |
| Another file defines `read_file` | No effect: names are per file, and that one stays reachable as `utils::read_file` |
| The library renames or removes `read_file` | A load error at the `incl` line, with a suggestion, as for any unknown member (`'csv' (std/csv) has no 'reed_file'; did you mean 'read_file'?`) |
| The library adds a name | No effect: only listed names are bare |

So a bare name has one meaning, visible in its own file, and nothing another file does can change it.

### 6.4 Trade-offs

These alternatives were weighed against the proposal:

- **Every name of the file bare** (`incl csv::*`, like R's `library()` or Python's `from csv import *`). It is shorter, but a name a library adds then changes its importers: if it clashes with a name an importer defines, a program that built stops building; if one side wins, a call silently changes target. Two such imports that share a name are ambiguous, and `std/csv` and `std/json` share four (§9.1). Rust makes a glob import lose to a definition and reports an ambiguity only where the name is used. That keeps builds working, but the meaning of a bare name then depends on what other files contain. Not proposed.
- **The file's own definition wins over a bound name** (R's masking, which prints a message when the package is attached). This is silent: adding `fn read_file` at the end of a script changes every earlier bare call. Not proposed, because names never collide silently.
- **The bound name wins over the file's own definition.** The definition is then dead code. Not proposed.
- **Lookup as a fallback**: an unresolved bare name is looked up in every imported file. It has the problems of `*`, and no line says where a name comes from. Not proposed.

What the proposal costs:
- A reader of `read_file(p)` has to look at the file's `incl` lines to know it is CSV's; `::` shows that at the call. Hover and every message show the canonical name.
- The stdlib's names were chosen to be read qualified. Bare, `read_file`, `parse`, `format`, `open` and `now` say little, and two formats can't both bind `read_file` without `as`.
- One function has two spellings in a program.
- CLAUDE.md's Robustness Bar rule 6 and modules rule 12 change from "nothing from another file is visible unqualified" to "nothing from another file is visible unqualified unless the file binds it by name".

### 6.5 Syntax

Proposed: `incl csv::read_file`. It needs no new keyword, because `incl` already parses a name after it (`incl m`, rejected today with `a module in the same file needs no incl`) and `as` already follows it. It also writes the canonical name where the bare one is made. The alternative takes one line per import, `incl 'std/csv' use read_file, write_file`, close to Python's `from csv import read_file`. It needs `use` as a keyword, which then can't name a variable or a function (`src/testfiles/smoke_shadowing.qrk` defines `fn use`), and the canonical name appears nowhere. Open (§22).

### 6.6 What changes in the compiler

- The parser: `incl` with a list of qualified names, each with an optional `as`.
- The loader: a binding that points at its definition, the clashes of §6.3, and bound names in step 3 of rule 12's lookup.
- The REPL: a binding is a definition of the session, so defining the name in a later input is an error.
- The editor: completion after `incl csv::`.

Nothing after the loader changes. A few hundred lines, with tests.

## 7. Backtick names for columns and fields

### 7.1 Today

- A read without a row struct keeps each header as written. A file whose header is `Employee Name,Annual Salary ($),dept-id` gives a table with those three columns, which prints and writes, but code can't name them: `e.Employee Name` isn't an expression. Queries on such tables (`specs/untyped_tables_v0_1_spec.md`) will make this common.
- A keyword can't be a field: a struct field `result: str` is the parse error `expected field name in struct definition`. `result`, `in`, `for`, `list`, `when` and `module` are all common column names.
- A typed read maps fields to such headers (`dict { salary: 'Annual Salary ($)' }`, `specs/tables_v0_1_spec.md` D17). That stays.

### 7.2 Surface

A name in backticks is a field name that may be any text:

```quark
struct Emp:
    `Employee Name`: str
    `Annual Salary ($)`: float
    `result`: str

staff: tbl[Emp] = unwrap(csv::read_file('staff.csv'))   // the headers match the fields, so no mapping
raised = staff | e -> mutate(`Annual Salary ($)` = e.`Annual Salary ($)` * 1.05)
print(staff.`Employee Name`)
```

A backtick name may appear wherever a field, column or dict key is named:
- after `.`: `` e.`Employee Name` ``, `` t.`dept-id` ``, `` r.`result` ``, `` d.`a key` ``;
- as a struct field, in its declaration and in a struct literal (`` Emp { `Employee Name`: 'Ann', ... } ``);
- as the name of a verb's named argument: in `mutate`, `summarize`, `rename`, `count` and a computed `group_by` key;
- as a dict literal key (`` dict { `Employee Name`: 1 } ``), including the field side of a read's column mapping.

Anywhere else it is a parse error. A variable, parameter, function, struct, module or import binding needs a plain name: `` `total sales` = 0 `` gives `` a name in backticks names a field or column; a variable needs a plain name ``. A backtick name used as a value on its own (`` mutate(x = `Employee Name`) ``) is the error `` `Employee Name` names a column; read it from the row: e.`Employee Name` ``, because a query reads columns through its row name.

The text inside:
- may be any UTF-8 text that isn't empty and has no backtick, no control character (U+0000-U+001F, U+007F) and no `__`. There are no escapes: a header with a backtick in it is renamed when it is read (D17). `__` stays reserved here, as in every user name (`checkReservedNames`), because diagnostics rewrite internal names, which contain `__`, and a field named like one would be shown wrong;
- is the field's name exactly, spaces included. `` `mpg` `` is the field `mpg`, and `` `result` `` is the field `result`.

### 7.3 How such a name is shown

Wherever Quark shows a field in code form, a name that isn't a plain name, or that is a keyword, is shown in backticks so it can be pasted back: in types (`` tbl[{`Employee Name`: str, mpg: float}] ``), in the column lists of errors (`` (columns: `Employee Name`, mpg) ``), and in `print` of a struct (`` Emp { `Employee Name`: 'Ann', ... } ``). A table's printed header and `t.names()` show the text as it is, since they are data, not code.

### 7.4 What changes in the compiler

- **Lexer**: a token for a backtick name, whose literal is the text inside.
- **Parser**: the places of §7.2 accept it wherever they accept a name; anywhere else it is the error above.
- **Display**: one function decides whether a name needs backticks. The type printer, the column lists of errors and the runtime's struct printing (`builtins/repr.hpp`) use it, and a test checks that the Go and C++ keyword lists agree.
- **Editor grammar**: the token.
- **Unchanged**: the loader, the analyzer, lowering and emit. From the parser on, a field name is already a string (the fields of a `StructType`, `QStructDef`), and it reaches the generated C++ only as an escaped string literal (`_sfields_*` in `emit/emit.go`), never as a C++ name.

About 200 lines plus tests: one to two days.

## 8. TSV and other separators

### 8.1 Today

`std/csv` reads and writes tab-separated files. `csv::read_file` and `csv::write_file` use a tab when the path ends in `.tsv` or `.tab` (in either case), with CSV's quoting: a quoted field may hold a tab, a line break or `""`, and a `"` inside an unquoted field is kept. A UTF-8 byte order mark is skipped. Three things can't be done:
- **Tab-separated text**: `csv::parse_text` and `csv::format_text` always use a comma, so `csv::parse_text('x\ty\n1\t2\n')` gives a one-column table whose column is named `x\ty`.
- **A tab-separated file with another extension**, such as `.txt` or `.dat`.
- **Another separator**: `;` (used where `,` is the decimal mark) or `|`.

### 8.2 Proposal

TSV stays in `std/csv`, since it is the same format with another separator. There are two ways to name the separator:

- **A: a `sep` parameter**, last and with a default, on all four functions. `''`, the default of the file functions, means by extension, as today; the text functions default to `','`.
  ```quark
  t = unwrap(csv::read_file('data.txt', dict {}, '\t'))
  u = unwrap(csv::parse_text(text, dict {}, ';'))
  csv::write_file(t, 'out.txt', '|')
  ```
  When keyword arguments come (DW22), `csv::read_file('data.txt', sep = '\t')` works with no change to the function. Until then, a separator needs the column mapping written out (`dict {}`).
- **B: tab functions**: `csv::read_tsv(path, columns)`, `csv::parse_tsv(text, columns)`, `csv::write_tsv(t, path)` and `csv::format_tsv(t)`, with a tab whatever the extension. They are easier to find and to write, but `;` and `|` still can't be read. Like `sort_desc` (DW22), they could give way to a keyword argument later.

Recommended: A, because it covers every separator, adds no names, and is already the keyword-argument form. Open (§22).

With A, a separator is one ASCII character other than `"`, a space, `\n` or `\r` (an unquoted field is trimmed of spaces, so a space can't separate). A literal separator is checked at compile time, since the analyzer already handles these calls (§9.5). A separator known only at runtime is checked when the call runs: it is an `err(...)` from the reads and writes, and a runtime error from `format_text`.

## 9. JSON

### 9.1 The module

`std/json` reads and writes JSON: tables from records, and any JSON value. Its table functions have `std/csv`'s names, arguments and typing (§9.5), so moving a program from CSV to JSON changes only the module name:

| | A file | Text |
|---|---|---|
| A table | `json::read_file(path, columns = dict {})`, `json::write_file(t, path)` | `json::parse_text(text, columns = dict {})`, `json::format_text(t)` |
| Any value | `json::read_value(path)`, `json::write_value(x, path, indent = 0)` | `json::parse_value(text)`, `json::format_value(x, indent = 0)` |

All eight return a `result`. Unlike `csv::format_text`, the format functions can fail, because JSON has no NaN or infinity (§9.3).

```quark
incl 'std/json'

struct Event:
    id: int
    kind: str
    at: date

events: tbl[Event] = unwrap(json::read_file('events.jsonl'))
config = unwrap(json::read_value('config.json'))   // a dict of any values
print(config.threshold)
clicks = events | e -> filter(e.kind == 'click')
json::write_file(clicks, 'clicks.json')
```

This replaces the sketch in `specs/stdlib_expansion_v0_1_spec.md` §9.2.

### 9.2 Reading a table

**Shape.** The first character after white space and any byte order mark decides it:
- `[`: an array of objects, one record each. `[]` is a table with no rows.
- `{`: JSON Lines (`.jsonl`, `.ndjson`): one object per line. Blank lines are skipped.
- Anything else is an error with the line and column, and so is an array element that isn't an object or a line that isn't one whole object: `events.json:3:5: expected an object (one record), found a number`. For an object whose values are arrays (columns, not records), the error suggests `json::read_value`.

**Columns of an untyped read** are the keys of all the records, in order of first appearance. Where a record lacks a key, or has `null` for it, the value is missing. A key that appears twice in one object is an error naming the record: JSON leaves the meaning undefined, and keeping one value would silently drop the other.

**Column types of an untyped read** follow CSV's inference (stdlib.md, `std/csv`) where JSON doesn't already give the type:

| Present values | Column |
|---|---|
| `true` and `false` | `bool` |
| Integers (no fraction or exponent) that fit an int | `int` |
| Numbers, some with a fraction or an exponent, or too large for an int | `float` |
| Strings, each one a calendar day as CSV reads one (`2024-01-02`, `2024-1-2`) | `date` (open, §22) |
| Strings | `str` |
| Values of different kinds (numbers and strings, bools and numbers) | `str`, each value as its text: a string's contents, or a number or `true`/`false` as written |
| None | `str`, as for an empty CSV column |

A string is never read as a number: `"01234"` stays text, unlike the same field in a CSV file.

**A typed read** (`events: tbl[Event] = ...`) reads each field from the key of its name, or from the key the column mapping gives it (D17), and ignores other keys. A field takes a JSON value of its type; a float field also takes an int (int widens to float), and any field takes a string that a typed CSV read accepts for its type (`"42"` for an int, `"2024-01-31"` for a date). A str field takes any scalar, as its text. Anything else is an error with the record and the key: `events.jsonl:17: key 'id': "a7" is not an int`. A field that no record has is an error, as a column missing from a CSV header is, unless there are no records at all.

**Nested values** are values of a record that are themselves objects or arrays. There are three options (open, §22):
- **a. Flatten objects, and make an array an error** (recommended). `{"addr": {"city": "Oslo"}}` gives a column `addr.city`, read as `` e.`addr.city` `` (§7). An array value is an error naming the record and the key, with a hint to use `json::read_value`. It is also an error when two paths flatten to one name (a key `"addr.city"` beside `"addr": {"city": ...}`), or when a key is an object in one record and a scalar in another. pandas' `json_normalize` and jsonlite's `flatten` do the same.
- **b. Any nested value is an error.** Flattening can be added later without breaking a program.
- **c. A nested value is kept as its JSON text**, in a str column.

**Text.** Escapes are decoded, and a `\u` surrogate pair gives one character. A lone surrogate, invalid UTF-8 and `\u0000` are errors, as invalid UTF-8 and NUL are in CSV. The grammar is RFC 8259's, so comments, trailing commas, single quotes, `NaN`, and a leading `+` or zero are errors. Nesting deeper than 512 levels is an error. A str column holds at most 2 GiB of text, as in CSV.

**Errors** name the file, the line and the column. The first problem in file order is the one reported, whatever the thread count.

### 9.3 Writing a table

`json::write_file(t, path)` writes JSON Lines when the path ends in `.jsonl` or `.ndjson`. Otherwise it writes an array, with one record per line:

```
[
{"name": "Ann", "salary": 100.5, "start": "2021-03-01"},
{"name": "Bob", "salary": null, "start": "2022-11-15"}
]
```

- Keys come in column order, each one the column's name as it is (`"addr.city"` stays one key).
- A missing value is `null`, and an int is written as an integer. A float is written in shortest round-trip form with a fraction or an exponent (`2.0`, never `2`), so its column reads back as float. A date is a `"YYYY-MM-DD"` string. A str is escaped (`"`, `\` and control characters), and other characters are written as UTF-8.
- A NaN or an infinity is an `err(...)` naming the column and the row, with a hint to use `fillna` or `mutate` first. Writing `null` instead would read back as a missing value, which is a different value.
- `json::format_text(t)` gives the array form as text.
- A table written by `json::write_file` reads back with the same columns, order, values and missing values. There are two differences, both as in CSV: a column with no present values comes back as `str`, and, with the date rule, a str column whose values are all calendar days comes back as `date`.

### 9.4 Any value

- `json::parse_value(text)` returns `ok(value)`. An object becomes a `dict` whose values are `any`, with its keys in input order. An array becomes a `list` of `any`, and a string a `str`. An integer that fits becomes an `int`, and any other number a `float`. `true` and `false` are `bool`, and `null` is `null`. The errors of §9.2 apply (a duplicate key, a lone surrogate, `\u0000`, nesting too deep), and so does an error for anything after the value. The value is `any` at compile time, so what is done with it is checked at runtime, as for any untyped value.
- `json::format_value(x, indent = 0)` writes a `dict` as an object, a `list` or `vec` as an array (with a vec's missing values as `null`), a struct as an object of its fields, a table as §9.3's array, and scalars as §9.3 does. An `indent` above 0 puts each element on its own line, indented by that many spaces. A function, a `resource`, a `result` and a `period` have no JSON form. That is a compile-time error when the compiler knows the type (`json::format_value(f)` with `f: fn`, or a `list` or struct field of them); otherwise it is an `err(...)` naming where the value is (`[3].callback`). NaN and infinities are also an `err(...)`.
- `json::read_value(path)` and `json::write_value(x, path, indent = 0)` do the same for a file.

### 9.5 Typing a read by where it goes

`std/csv`'s reads are typed by the expected type (D18), but the analyzer recognizes them by name: `tableReaders` in `types/analyzer_tables.go` lists `read_file` and `parse_text`, and `tableReaderOf` checks that the function is `std/csv`'s. `std/json`'s table reads need the same, and so would any later format. So the list of names becomes a property of the stdlib's declarations, a marker like the one `extern rowsafe fn` puts on row-safe native code, and the analyzer keeps no names. `std/csv` moves to the marker with no change in behaviour. The compile-time check of `json::format_value`'s argument is the same kind of property. The exact form is settled when it is implemented. User `extern fn`s don't get the marker in v0.1.

### 9.6 How it runs

- **An in-house reader**, as for CSV (D8). `table/json.hpp` parses straight into the column buffers of §4.2, in one pass over 16 MB blocks. A column can widen from int to float as it is read (its buffer is converted once), and a str column that might hold dates is checked at the end. No DOM is built, so the whole document is never held. v0.1 uses no third-party library: a DOM library (nlohmann/json, yyjson) holds the whole document, and simdjson is a large dependency. simdjson's on-demand parser is the candidate if the targets are missed.
- **Memory**: DW3's bound, 1.25 times the finished columns plus 64 MB and 16 MB per thread.
- **Threads**: JSON Lines can be split at line ends, because a JSON string can't hold a raw line break, so blocks parse on the thread pool. The array form is split the way CSV is split across quotes, by knowing whether a block starts inside a string. v0.1 may read on one thread, as joins may (DW23): that applies to the read only, and the output and errors are the same for every thread count.
- **Speed**: per byte, at least as fast as the CSV reader on the same table. Measured against Polars' `read_ndjson` and pandas' `read_json(lines=True)`.
- **Glue**: `std/json.qrk` binds the reader through an `extern source`, compiled once and cached, so importing `std/json` adds little to a build.

## 10. Excel (waiting for research)

Excel support waits until the user has looked at how best to read and write `.xlsx` in C++ (DW25). Whatever implementation is chosen has to answer these, to fit with §8 and §9:
- **The same functions**: `excel::read_file(path, columns = dict {})` for the first sheet, a read of a sheet by name, the list of sheets, and `excel::write_file(t, path)`; typed by the expected type (§9.5), with missing values as in CSV and JSON.
- **Numbers**: Excel stores every number as a double. Whether a column of whole numbers is an `int`, as CSV and JSON infer from integer text.
- **Dates**: a number with a date format is a `date`. Quark has no type for a date with a time of day (`std/time` uses float seconds), and Excel has two date systems (1900 and 1904).
- **Cells**: error cells (`#N/A`, `#DIV/0!`), a formula with no stored value, merged cells, and a table that doesn't start at A1.
- **Limits when writing**: 1,048,576 rows, 16,384 columns, 32,767 characters in a cell, and ints past 2^53, which a double can't hold exactly. Each is an error, never a truncation.
- **Nothing for the user to install**, and no whole sheet held as a DOM: a sheet is streamed, as CSV is.

## 11. Benchmark method

### 11.1 Runs

- One process per tool, task, preset, table variant and thread setting. It loads the task's data once, untimed, then runs the task's questions in order. Each question's result is materialized (no lazy result) and checksummed outside the timed region; the checksum is compared across tools, as now.
- `10m`: per question, a first run, then 5 timed runs. `100m`: a first run, then 2 timed runs, as db-benchmark does at its sizes.
- A process is stopped after a timeout and the question is reported as `timeout`.

### 11.2 Memory and CPUs

- The driver measures each process's peak resident memory, and polls it while the process runs (every 100 ms). Above the preset's budget (6 GB for `10m`, 12 GB for `100m`) it stops the process. The question is then reported as `out of memory` and isn't ranked, as db-benchmark reports a solution that runs out of memory. This also keeps a run from exhausting the machine the suite runs on.
- The 8-thread setting pins each process to 8 logical CPUs (CPU affinity where the OS has it) and sets every thread pool to 8. The 1-thread setting pins one CPU and sets every pool to 1 (`OMP_NUM_THREADS`, `OPENBLAS_NUM_THREADS`, `POLARS_MAX_THREADS`, `R_DATATABLE_NUM_THREADS`, `GC_MARKERS`, and Quark's `--threads` once it exists).

### 11.3 What is reported

Per question and tool: the first run, the median of the later runs, their minimum and maximum, peak memory, and the checksum result. `out of memory`, `timeout`, `wrong` (checksum mismatch) and `n/a` (no native operation, §12) are results, shown in the table. A summary per task gives each tool's total over the questions it ran.

### 11.4 Ties

When the minimum-to-maximum ranges of two tools overlap, the report prints `tie` instead of a ratio. A headline in `RESULTS.md` that one tool is faster needs ranges that don't overlap, at both presets.

## 12. Parity

A question measures what each tool does natively. For each tool and question:

- **Native**: the tool's own operation for this task, as an experienced user writes it (`df.group_by(...).agg(...)`, `dt[, .(...), by = ...]`, `t | e -> group_by(...) | summarize(...)`), with the options that give the same result as the others (`dropna=False`, `na.rm = TRUE`, `nulls_last=True`). That is what is timed.
- **`n/a`**: the tool has no such operation. It is not replaced by a composition of other operations or a loop. NumPy has no group by, so NumPy is `n/a` in every group-by question, instead of `np.unique` with `np.bincount` as now. NumPy's integer arrays can't hold a missing value, so NumPy is `n/a` on `G-NA`.
- **Quark**: if Quark has no native operation for a question, the question isn't in the suite until it does (§16). A Quark workaround, such as going through a list, is never measured.

## 13. Data

### 13.1 The table `G`

One seeded table, after db-benchmark's `groupby` data, with six columns:

| Column | Type | Values | db-benchmark |
|---|---|---|---|
| `k1` | `str` | 100 values, `id001` to `id100` | `id1` |
| `k2` | `int` | 100 values, 1 to 100 | `id4` |
| `k3` | `str` | N/100 values, `id0000000001` and up | `id3` |
| `k4` | `int` | N/100 values | `id6` |
| `v1` | `int` | 1 to 5 | `v1` |
| `v2` | `float` | uniform in [0, 100), 6 decimals | `v3` |

Keys are drawn uniformly, so `k1` and `k2` give 100 groups each, `(k1, k2)` 10,000, and `k3` and `k4` 100,000 at `10m` and 1,000,000 at `100m`.

| | `10m` | `100m` |
|---|---|---|
| CSV file | about 0.4 GB | about 4.1 GB |
| In memory, 64-bit ints and Arrow strings (Quark, Polars, pandas with pyarrow strings) | about 0.6 GB | about 5.7 GB |
| In memory, data.table (32-bit ints, shared strings) | about 0.4 GB | about 3.6 GB |

Every tool reads the same CSV file before timing (untimed except in §14.1).

### 13.2 The variant `G-NA`

`G` with 5% missing values in every column, by db-benchmark's rule: in a key column, 5% of its distinct values are replaced by a missing value in every row that has them, so the missing key is one group of about 5% of the rows; in a value column, 5% of the rows, drawn at random, are missing. A missing cell is an empty field in the CSV file. Quark aggregates skip missing values, and sort and group missing keys last (`specs/tables_v0_1_spec.md` §5); each peer uses the options that do the same (§12).

### 13.3 The labels `L`

N strings like `"  ITEM_4821_Lorem "` (padding, mixed case, underscores), as in the current `strings` benchmark, in a one-column CSV file. `L` has no missing-values variant.

### 13.4 The keys `D`

The right-hand table of the join questions: one row for each of 90% of the N/100 key values (the other 10%, drawn at random, are left out), with columns `k3: str`, `k4: int` and `w: float`. Row `i` has `k3 = 'id%010d' % i`, `k4 = i` and `w` uniform in [0, 1). About 90,000 rows at `10m` and 900,000 at `100m`. `D` has no missing values, so every tool gives the same result whatever it does with a missing key; `G-NA`'s missing keys are on the left. The questions use `D4`, its `(k4, w)` columns, and `D3`, its `(k3, w)` columns, selected before timing.

## 14. Questions

Each question lists Quark's operation and each peer's, or `n/a`. Quark's table code assumes `struct G` with the columns of §13.1; `t` is the table, and `v1` and `v2` are its columns as vecs. Questions on `G` also run on `G-NA` (DW10).

### 14.1 Read

| # | Question | Quark | NumPy | pandas | Polars | base R | data.table |
|---|---|---|---|---|---|---|---|
| RD1 | Read `G` with the column types given | `t: tbl[G] = unwrap(csv::read_file(p))` | n/a | `read_csv(dtype=...)` | `read_csv(schema=...)` | `read.csv(colClasses=...)` | `fread(colClasses=...)` |
| RD2 | Read `G` with the column types inferred | `t = unwrap(csv::read_file(p))` | n/a | `read_csv` | `read_csv` | `read.csv` | `fread` |

The file is in the OS page cache after the first run, so this measures parsing and building the columns.

### 14.2 Column expressions

| # | Question | Quark | NumPy | pandas | Polars | base R | data.table |
|---|---|---|---|---|---|---|---|
| EX1 | Sum and mean of `v2` | `sum(v2)`, `mean(v2)`; `summarize(s = sum(e.v2), m = mean(e.v2))` | `.sum()`, `.mean()` | Series `sum`, `mean` | `select(sum, mean)` | `sum`, `mean` | `dt[, .(sum(v2), mean(v2))]` |
| EX2 | New column `x = 2.5 * v2 + v1` | `2.5 * v2 + v1`; `mutate(x = 2.5 * e.v2 + e.v1)` | array expression | `assign` | `with_columns` | vector expression | `dt[, x := ...]` |
| EX3 | New column `z = (v2 * v2 - v1) * 0.5 + (v2 - v1) / (v1 * v1 + 1)` | the same, as vec and as `mutate` | array expression | `assign` | `with_columns` | vector expression | `dt[, z := ...]` |

Quark runs each as a vec and as a table (two rows in the report); NumPy and base R are vecs, the frame libraries tables. EX3 separates fused evaluation (Quark's `mutate`, Polars) from evaluation that materializes every intermediate.

### 14.3 Filter

| # | Question | Quark | NumPy | pandas | Polars | base R | data.table |
|---|---|---|---|---|---|---|---|
| FL1 | Rows where `v2 > 50` (about half) | `filter(e.v2 > 50.0)` | n/a | boolean indexing | `filter` | n/a | `dt[v2 > 50]` |
| FL2 | Rows where `k1 == 'id042'` and `v1 >= 3` (about 0.6%) | `filter(e.k1 == 'id042' and e.v1 >= 3)` | n/a | boolean indexing | `filter` | n/a | `dt[k1 == 'id042' & v1 >= 3]` |

Every column of the kept rows is materialized. A third question, rows whose `k4` is in a list of 1,000 values, waits for `contains` (§16).

### 14.4 Sort

| # | Question | Quark | NumPy | pandas | Polars | base R | data.table |
|---|---|---|---|---|---|---|---|
| SO1 | Sort `v2` | `v2.sort()` (§3) | `np.sort` | `Series.sort_values` | `Series.sort` | `sort(na.last = TRUE)` | n/a |
| SO2 | Sort `(k4, v2)` by `k4` | `sort(e.k4)` | n/a | `sort_values('k4')` | `sort('k4')` | n/a | `dt[order(k4)]` |
| SO3 | Sort `(k1, v2)` by `k1`, then `v2` descending | `sort(e.k1, desc(e.v2))` | n/a | `sort_values([...], ascending=[True, False])` | `sort([...], descending=[False, True])` | n/a | `dt[order(k1, -v2)]` |

SO2 and SO3 sort two-column tables (`select` before timing), so the gathered output stays inside the memory budget at `100m`. SO1 waits for §3.

### 14.5 Group by

| # | Question | Quark | pandas | Polars | data.table | db-benchmark |
|---|---|---|---|---|---|---|
| GB1 | `sum(v1)` by `k1` (100 groups, a str key) | `group_by(e.k1) \| summarize(s = sum(e.v1))` | `groupby('k1').agg` | `group_by('k1').agg` | `dt[, .(s = sum(v1)), by = k1]` | q1 |
| GB2 | `sum(v1)` and `mean(v2)` by `k3` (N/100 groups, a str key) | `group_by(e.k3) \| summarize(s = sum(e.v1), m = mean(e.v2))` | `groupby('k3').agg` | `group_by('k3').agg` | `dt[, .(...), by = k3]` | q3 |
| GB3 | `sd(v2)` and `max(v2) - min(v2)` by `k1, k2` (10,000 groups) | `group_by(e.k1, e.k2) \| summarize(s = sd(e.v2), r = max(e.v2) - min(e.v2))` | `groupby(['k1', 'k2']).agg` | `group_by('k1', 'k2').agg` | `dt[, .(...), by = .(k1, k2)]` | q6, q7 |

NumPy is `n/a`. base R's `tapply` does one aggregate of one key, so base R runs GB1 only.

### 14.6 Window

| # | Question | Quark | pandas | Polars | data.table |
|---|---|---|---|---|---|
| WN1 | New column `z = (v2 - mean(v2)) / sd(v2)` | `mutate(z = (e.v2 - mean(e.v2)) / sd(e.v2))` | `assign` with Series `mean`, `std` | `with_columns` | `dt[, z := ...]` |
| WN2 | New column `dev = v2 - mean(v2)` within each `k1` group | `group_by(e.k1) \| mutate(dev = e.v2 - mean(e.v2))` | `groupby('k1')['v2'].transform('mean')` | `pl.col('v2').mean().over('k1')` | `dt[, dev := v2 - mean(v2), by = k1]` |

Both use aggregates in `mutate` (`specs/tables_v0_3_spec.md` D35, implemented).

### 14.7 Pipeline

| # | Question | Quark |
|---|---|---|
| PL1 | Keep `v2 >= 10`, add `x = v2 * v1`, group by `k1, k2`, compute the row count, `sum(x)`, `mean(v2)` and `max(v2)`, and sort the groups by `sum(x)` descending | `t \| e -> filter(e.v2 >= 10.0) \| mutate(x = e.v2 * e.v1) \| group_by(e.k1, e.k2) \| summarize(n = count(), s = sum(e.x), m = mean(e.v2), hi = max(e.v2)) \| sort(desc(e.s))` |

pandas, Polars and data.table each write it as one chain; NumPy and base R are `n/a`.

### 14.8 Strings

| # | Question | Quark | NumPy | pandas | Polars | base R |
|---|---|---|---|---|---|---|
| ST1 | Trim, lowercase and replace `_` with `-` in every label of `L` | `mutate(s = e.s.trim().lower().replace('_', '-'))` | `np.strings` | `.str` methods | `.str` methods | `gsub(..., tolower(trimws(s)), fixed = TRUE)` |
| ST2 | Rows of `L` whose label contains `ITEM_48` | `filter(e.s.contains('ITEM_48'))` | n/a | `.str.contains(regex=False)` | `.str.contains(literal=True)` | `grepl(fixed = TRUE)` |

data.table uses base R's string functions, so it is the base R row.

### 14.9 Join

After db-benchmark's join task, on a left table of N rows and the right table `D` (§13.4). The left side is `(k4, v2)` or `(k3, v2)` from `G`, selected before timing, so the joined result stays inside the memory budget at `100m`.

| # | Question | Quark | pandas | Polars | data.table | db-benchmark |
|---|---|---|---|---|---|---|
| JN1 | Rows of `(k4, v2)` whose `k4` is in `D4` (about 90%; a semi join on an `int` key) | `g \| a -> semi_join(d4 \| b, a.k4 == b.k4)` | `g[g.k4.isin(d4.k4)]` | `join(how='semi')` | `g[k4 %in% d4$k4]` | |
| JN2 | `(k4, v2)` joined with `D4` on `k4`, keeping matched rows (each matches once) | `inner_join(d4 \| b, a.k4 == b.k4)` | `merge(how='inner')` | `join(how='inner')` | `d4[g, on = 'k4', nomatch = NULL]` | q2 |
| JN3 | `(k3, v2)` joined with `D3` on `k3`, keeping every row (`w` missing for about 10%; a `str` key) | `left_join(d3 \| b, a.k3 == b.k3)` | `merge(how='left')` | `join(how='left')` | `d3[g, on = 'k3']` | q3, q4 |

NumPy and base R are `n/a` (base R's join, `merge`, works on `data.frame`, which left the suite). The checksums (row count, `sum(w)`, `sum(v2 * w)` over present values) don't depend on row order, which Polars doesn't keep by default. db-benchmark's big join (a right table as large as the left) isn't included: both tables and the result don't fit the 16 GB laptop at `100m`.

### 14.10 Plain code

| # | Question | Quark | Python | R |
|---|---|---|---|---|
| FB1 | Recursive `fib(35)` | `fn fib(n: int) int` (a plain C++ function) | a function | a function |

The cost of a function call, an integer comparison and integer arithmetic in a loop the tool can't vectorize. `fib(35)` takes about 120 ms in Quark, 3 s in CPython and 18 s in R (scaled from `fib(30)` in `RESULTS.md`). It runs at both presets unchanged and isn't part of the data headline.

### 14.11 Matrix multiply

| # | Question | Quark | NumPy | R |
|---|---|---|---|---|
| MM1 | `C = A · B`, 1,000 × 1,000 | `blas::gemm(a, n, n, b, n)` (`std/blas`) | `a @ b` | `a %*% b` |

All three call a BLAS `dgemm`; this measures what `std/blas` adds around it (a flat vec with explicit dimensions, the call and the result). Quark's plain `vec` loops are not run (§12). The question grows when Quark has a matrix type.

## 15. What is removed

| Benchmark or tier | Why |
|---|---|
| `mandelbrot`, `sieve` | Plain code, which `fib` covers |
| `ewma` | Quark has no recurrence operation (its `vec` time is a loop), and neither does NumPy |
| `reduce`, `arith`, `expr` | Folded into EX1-EX3, on `G` |
| `dot`, `gemv`, `ols` | BLAS calls that `gemm` covers; `gemv` and `dot` finish in a few milliseconds |
| `filter`, `sort`, `sortkeys`, `groupby`, `zscore`, `query`, `readcsv`, `strings` | Replaced by §14.3, §14.4, §14.5, §14.6, §14.7, §14.1 and §14.8 on `G`, `G-NA` and `L` |
| `join`, `wordcount` | Quark has no join and can't build a table from a list, so its times were `dict` loops against real joins and counts. Joins come back with §5 |
| The list tier, base R `data.frame` | Element-at-a-time loops aren't how any peer does data work; `data.table` is R's tool at these sizes. `quark_blas` stays for `gemm` only |

## 16. What waits for a Quark feature

| Question | Waits for |
|---|---|
| SO1 (sort a vec) | §3 |
| The join questions (§14.9) | §5 |
| FL3: rows whose `k4` is in a list of 1,000 values | `contains` in verbs (`specs/tables_v0_3_spec.md` D48) |
| `median(v2)` by key, the largest two `v2` per group (db-benchmark q6, q8) | A quantile kernel; top-n per group |
| Word count | Building a table from a list, and v0.3's `count` step |
| The 8-thread headline (DW12) | The rest of `specs/tables_v0_3_spec.md` §8: the CSV reader, `distinct` and the grouped aggregates of many small groups still run on one thread |

## 17. What the 100M preset will show

These are expected from today's code and are to be confirmed by the first `100m` run.

- **Until §4 is done, Quark can't load `G` at `100m` on a 16 GB laptop**: about 15 GB at the reader's peak, so RD1, RD2 and every task that loads `G` would be out of memory.
- **A `str` column holds at most 2 GiB of text** (32-bit offsets; past that, `vec[str] data exceeds 2 GiB` is a runtime error). `k3` at `100m` is 1.2 GB and `L` about 1.8 GB, so both fit, `L` with little room.
- **What still runs on one thread.** Query row loops, most of the table engine and the vec operations run on the thread pool (`specs/tables_v0_3_spec.md` §8); the CSV reader, `distinct`, the grouped aggregates of many small groups and joins v0.1 (DW23) don't yet, and the 8-thread setting shows what that costs.

## 18. Changes to other docs

When each part is implemented: `stdlib.md` lists `sort`, `sort_desc`, `order` and `order_desc` under the vec methods (§3); `architecture.md` describes the CSV reader's blocks and buffers (§4); joins go into `semantics.md` §9.5 (the verbs and the joined row), `grammar.md` §10.5 (the `right | b` argument), `architecture.md` (the engine's hash join) and the editor grammar's verb names, and `specs/tables_v0_1_spec.md` §9 stops listing them as planned (§5); `specs/tables_v0_3_spec.md` §8.1 gets the radix kernel as the engine's single-key sort once it is parallel (§3.2), the streamed reader's parallel parse (§4.2), and joins once they are parallel (§5.5); `src/benchmarks/README.md` is rewritten for the tasks, presets, variants, method and parity rule here; `src/benchmarks/RESULTS.md` is measured again at both presets; `architecture.md` §15 takes its performance findings from the new results.

For §6-§9:
- **Bare names (§6)**: `specs/implemented/modules_v0_2_spec.md` (the `incl` form, rules 12 and 14, its diagnostics), `grammar.md` (`incl`), `semantics.md` §10, `README.md` (the language tour), `error_codes.md` if a code is added, and the Robustness Bar's rule 6 in CLAUDE.md.
- **Backtick names (§7)**: `grammar.md` (the token and where it may appear) and `semantics.md` (field names); `stdlib.md`'s `std/csv` section says that a backtick field matches a header as written.
- **The separator (§8)**: `stdlib.md` (`std/csv`).
- **JSON (§9)**: `stdlib.md` (`std/json`), `architecture.md` (the reader and the typed-read marker), and the editor's stdlib data (`go test ./internal/editordata -update`).
- `specs/stdlib_expansion_v0_1_spec.md` §9.2 already points to §9.

## 19. Tests

- **Vec sort and order**: every element type; missing values, NaN, `-0.0` and `0.0`, ties (stability), empty and all-missing vecs, a vec of unknown element type, and both directions. The vec sort, the table sort and the list sort agree on the same values (differential); `t.x.sort()` equals `(t | e -> sort(e.x)).x`; `v.sort()` equals `v` read at `v.order()`. A wrong argument count is a compile-time error.
- **CSV memory**: a generated file of 10M rows, typed and untyped, read within DW3's bound (checked as `integration_gc_test.go` checks heap size and peak resident memory). The existing CSV tests pass unchanged, including rows that cross a block boundary inside a quoted field, a row longer than a block, and a file of exactly one block.
- **Joins**: each of the four joins on duplicate keys on both sides, missing and NaN keys on either side, `-0.0` against `0.0`, several keys, an empty table on either side, and keys with different names; the row order of §5.3; the result stored into a named struct by shape. Rejected at compile time (`TestSemanticsSafety_Rejected`): a condition other than `and`-joined equalities, a key of the wrong type, an unknown column on either side, the same row name for both sides, a missing `| b`, an unparenthesized query on the right, a clashing column, a right side of unknown row type or of type `any`, a right side that isn't a table, a right side that reads the query's row, and a join after `group_by`. A right side from an imported file, from an `extern fn` returning `tbl[R]`, and from a function's default-valued parameter. A result past the row limit is a runtime error.
- **Bare names**: every case of §6.3. A bound function, `extern fn`, struct (in an annotation and in a literal) and constant. A bound name as a pipe target, as a function value, called with default arguments, and in a typed read (`t: tbl[R] = unwrap(read_file(p))`). Messages show the canonical name. A binding made in an imported file isn't reachable from its importer. The REPL rejects a later definition of a bound name. Binding a module, a member of a same-file module, or an unknown name is an error.
- **Backtick names**: each place of §7.2, with a space, punctuation, non-ASCII text and a keyword. Rejected (`TestSemanticsSafety_Rejected`): a backtick name as a variable, parameter, function, struct, module or alias; as a value on its own in a verb; empty; with a control character, a backtick or `__`. Shown in backticks in types, in the column lists of errors and in struct printing. A typed CSV read whose fields match headers with spaces. A struct with backtick fields defined in an imported file, read through an untyped (`any`) value, and passed to an `extern fn`.
- **Separators**: tab-separated text in both directions; a tab-separated file named `.txt`; `;` and `|`; a round trip. Rejected at compile time: a literal separator that is empty, longer than one character, `"`, a space or a line break. The same separators from a variable, rejected at runtime.
- **JSON**: both shapes; every row of §9.2's inference table; a typed read of each field type from a JSON value of the type, from a string, and from a wrong value; a missing key, `null`, a duplicate key, and a key that no record has; nested values, by the option chosen; strings with escapes, surrogate pairs, a lone surrogate, `\u0000` and invalid UTF-8; the strict grammar (comments, trailing commas, `NaN`); nesting 513 levels deep; the line and column of errors. Writing then reading gives the same table (differential, with missing values and every column type). NaN on write is an `err`. `format_value` of each kind of value, and a function value rejected at compile time when its type is known and at runtime inside a `list` of `any`. Values from an imported file and through `any`. Memory within DW3's bound on a generated JSON Lines file of 10M rows.
- **Suite**: the `small` check of today becomes a `tiny` preset (100K rows) that runs every question of every tool, on `G` and `G-NA`, in a few minutes, and fails when a checksum disagrees. The memory limit is tested with a process that allocates past it.

## 20. Plan

1. The CSV reader (§4): the `100m` preset depends on it.
2. The vec sort and order (§3), with the table engine's single-key sort moved to its kernel.
3. The data generator for `G`, `G-NA` and `L` at `tiny`, `10m` and `100m`.
4. The driver: one process per task and variant, first run reported separately, memory polling and the out-of-memory result, CPU pinning, ties.
5. The questions of §14, per tool.
6. Joins (§5), and their questions (§14.9).
7. A run at both presets, both variants and both thread settings; `RESULTS.md` and `architecture.md` §15 from it.

The window questions need tables v0.3's aggregates in `mutate`, which are implemented. FL3 and the 8-thread headline wait for v0.3 (§16).

§6-§9 don't depend on the suite. Their order, once their open questions (§22) are answered:
1. Backtick names (§7): queries on untyped tables need them, and so do JSON's flattened columns if §9.2's option a is chosen.
2. Bare names (§6).
3. The separator (§8).
4. The typed-read marker (§9.5), with `std/csv` moved to it.
5. `std/json` (§9), after the CSV reader (§4), whose block reading and column buffers it shares.

Excel waits for §10.

## 21. Later

- **Keyword arguments** come with other syntax changes to the language (DW22). Then `v.sort(desc = true)` can replace `sort_desc` and `order_desc`.
- **Joins**: `full_join`; conditions other than equality; keys of different numeric types; joins on tables whose row type the compiler doesn't know (`specs/untyped_tables_v0_1_spec.md`).
- The questions of §16, as their features arrive.
- **Bare names**: binding a module (`incl formats::csv`, then `csv::read`); binding every name of a file, if the trade-offs of §6.4 change.
- **Decimal commas**: reading `3,14` as a float, as `;`-separated files often write it.
- **JSON**: reading a JSON value into a typed value (a struct, or a list of structs) by its expected type, as table reads are typed; JSON Lines as text (`format_text` gives only the array form); a read question for JSON Lines in the suite (§14.1).
- **Excel** (§10).

## 22. Open questions

1. **Bare names (§6)**: the list in an `incl`, as proposed, and its syntax: `incl csv::read_file`, or `incl 'std/csv' use read_file`.
2. **Where backtick names may appear (§7.2)**: only where a field, column or key is named, as proposed, or also for variables and functions, which would need the generated C++ names to be mangled, for little gain in data work.
3. **Cleaning names**: whether to add `t.clean_names()`, and a read option, that turn headers into snake_case plain names (`Employee Name` becomes `employee_name`), with a clash after cleaning an error. On a typed table the compiler knows the names, so it can compute the new row type.
4. **The separator (§8.2)**: a `sep` parameter (A, recommended) or tab functions (B).
5. **JSON nested values (§9.2)**: flatten objects (a, recommended), an error (b), or JSON text (c).
6. **JSON dates (§9.2)**: whether an untyped read makes a str column of calendar days a `date`, as CSV does, so that a written table reads back the same; or keeps JSON strings as `str`, as Polars and jsonlite do.
