# Quark Data Work Improvements v0.1 Spec

Status: Proposal. Nothing is implemented. Four parts: sorting a vec (§3), reading CSV within memory (§4), joins (§5), and a benchmark suite that shows where Quark stands against the tools it competes with, on a laptop-sized machine (§6-§12). The suite replaces the one described in `src/benchmarks/README.md`.
Last Updated: 2026-11-17
Related: `src/benchmarks/README.md` and `src/benchmarks/RESULTS.md` (the current suite), `specs/tables_v0_3_spec.md` (aggregates in verbs, `contains`, threads), `stdlib.md` (vecs, `std/csv`, `std/blas`), `architecture.md` §15 (performance findings), and the DuckDB Labs database-like operations benchmark (db-benchmark), whose method §6 follows.

## Contents

1. [Purpose](#1-purpose)
2. [Decisions](#2-decisions)
3. [Sorting a vec](#3-sorting-a-vec)
4. [Reading CSV within memory](#4-reading-csv-within-memory)
5. [Joins](#5-joins)
6. [Benchmark method](#6-benchmark-method)
7. [Parity](#7-parity)
8. [Data](#8-data)
9. [Questions](#9-questions)
10. [What is removed](#10-what-is-removed)
11. [What waits for a Quark feature](#11-what-waits-for-a-quark-feature)
12. [What the 100M preset will show](#12-what-the-100m-preset-will-show)
13. [Changes to other docs](#13-changes-to-other-docs)
14. [Tests](#14-tests)
15. [Plan](#15-plan)
16. [Later](#16-later)

## 1. Purpose

The current suite measures many small things (21 benchmarks, most on 1M rows or 10M values, several under 10 ms), and in places it measures a Quark workaround instead of a Quark operation: vecs have no sort, so `sort`'s Quark `vec` time is `v.to_list().sort().to_vec()`, and `join` compares a `dict` loop with real data frame joins. Small inputs mostly measure fixed costs (setting up hash tables, allocating, starting threads), not the throughput that matters on real data. And headline claims such as "`groupby` 0.96 times Polars" rest on 5 runs with no measure of their spread.

This spec adds what Quark is missing for that work, and replaces the suite:
- A sort and a sorting permutation for vecs (§3).
- A CSV reader whose peak memory is close to the size of the table it builds (§4); today it is about 3.7 times the file.
- Joins: semi, anti, inner and left, on equal keys (§5).
- Fewer benchmark tasks, with questions of increasing difficulty borrowed from db-benchmark, on 10M and 100M rows, run as on a laptop with 8 or 16 GB of memory, measuring only operations that Quark and its peers each do natively (§6-§12).

## 2. Decisions

| # | Decision |
|---|---|
| DW1 | **Vecs have `sort()` and `sort_desc()`**, which return a new sorted vec in the order a table sort uses: stable, missing values last in both directions, NaN after every number and before missing values. The table engine's single-key sort uses the same kernel (§3). |
| DW2 | **Vecs have `order()` and `order_desc()`**, the stable 0-based positions that sort the vec, as a `vec[int]` (NumPy's `argsort`, R's `order`; decided 2026-11-17). |
| DW3 | **Reading a CSV file peaks at no more than 1.25 times the finished table's columns, plus 64 MB and 16 MB per thread** (§4; decided 2026-11-17). |
| DW4 | **Joins are part of this spec**: `semi_join`, `anti_join`, `inner_join` and `left_join` (§5; decided 2026-11-17). Full joins come later. |
| DW5 | **Loading data is never timed**, except in the read questions, where reading is the work (§9.1). The current suite already follows this rule; it stays. |
| DW6 | **A question is in the suite only when Quark does it with its own operation**, and each peer runs it only with its own native operation. No conversion to another type, and no hand-written algorithm where the others call a built-in. A peer without one is `n/a` for that question; a question Quark can't do natively waits (§7, §11). |
| DW7 | **The reference machine is a laptop**: 8 logical CPUs, and 8 GB of memory for the `10m` preset or 16 GB for `100m`. A process may use 6 GB or 12 GB. A run above that is reported as out of memory and isn't ranked (§6.2). |
| DW8 | **Two presets: `10m` (the default) and `100m`.** Data larger than memory is out of scope. |
| DW9 | **Tasks have two or three questions of increasing difficulty**, after db-benchmark (§9). |
| DW10 | **Every data question runs on two variants of the table**: `G` with no missing values and `G-NA` with 5% missing in every column, as db-benchmark does (§8.2; decided 2026-11-17). |
| DW11 | **The first run of each question is reported on its own**, next to the median of the later runs, because a script runs each step once (§6.3). |
| DW12 | **Two thread settings.** 8 threads, the reference laptop, which is the headline once Quark runs queries on several threads (`specs/tables_v0_3_spec.md` §8); and 1 thread, which compares the code. |
| DW13 | **A ratio is a tie when the two time ranges overlap.** The report and `RESULTS.md` don't call one tool faster than another inside a tie (§6.4). |
| DW14 | **The peers are NumPy, pandas, Polars, base R and data.table.** DuckDB isn't included; Polars stands in for an in-memory columnar engine (decided 2026-11-17). The list tier (`quark_list`, `py_list`, `r_loop`) and base R's `data.frame` leave the suite (§10). |
| DW15 | **`fib` stays**, as the measure of plain looping and function-call performance (§9.10; decided 2026-11-17). |
| DW16 | **One BLAS benchmark stays, `gemm`**, to measure `std/blas` against NumPy and R on the same kind of library. It grows with a matrix type, when Quark has one (§9.11; decided 2026-11-17). |
| DW17 | **A join is a query step that names the other table's row**: `left \| a -> semi_join(right \| b, a.id == b.key_id)`. `b` exists only inside the step; a right side that is itself a query is put in parentheses (§5.1; decided 2026-11-17). |
| DW18 | **A join condition is one or more `a.col == b.col` joined by `and`.** Anything else is a compile-time error that says to `filter` after the join (decided 2026-11-17). |
| DW19 | **The two keys of each pair have exactly the same type** in joins v0.1 (decided 2026-11-17). |
| DW20 | **A missing key never matches**, as `==` on a missing value is never true in a verb (decided 2026-11-17). |
| DW21 | **Joined columns never clash silently.** An inner or left join keeps the left table's columns, then the right's other than its keys; a right column named like a left one is a compile-time error that names it and suggests `rename` (§5.3; decided 2026-11-17). |
| DW22 | **Keyword arguments are not part of this spec**; they come with other syntax changes to the language. `sort_desc` and `order_desc` stand until then (decided 2026-11-17). |
| DW23 | **Joins v0.1 run on one thread, and nothing else does because of them.** Only the join step is single-threaded: a query's other steps, and every query without a join, run on the thread count in force (the default, `--threads N` or `QUARK_THREADS`; `specs/tables_v0_3_spec.md` D54). The join is built in blocks so that a parallel version changes no output (§5.5; decided 2026-11-17). |

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

`csv::read_file` reads the whole file into one `std::string`, which grows by doubling (`read_file`, `table/csv.hpp`). An untyped read then scans that text twice: once to infer the column types, once to build the columns. The columns are built in `std::vector` builders that also grow by doubling, and are copied into the table's vecs at the end, while the text is still held. A 157 MB file of 10M rows (a float, an int and a short `str` column) peaks at 589 MB of resident memory, against about 235 MB for the finished columns: 3.7 times the file, 2.5 times the columns. For the 4.1 GB file of §8 that is about 15 GB, more than a 16 GB laptop gives one process (`known_issues.md`).

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

## 6. Benchmark method

### 6.1 Runs

- One process per tool, task, preset, table variant and thread setting. It loads the task's data once, untimed, then runs the task's questions in order. Each question's result is materialized (no lazy result) and checksummed outside the timed region; the checksum is compared across tools, as now.
- `10m`: per question, a first run, then 5 timed runs. `100m`: a first run, then 2 timed runs, as db-benchmark does at its sizes.
- A process is stopped after a timeout and the question is reported as `timeout`.

### 6.2 Memory and CPUs

- The driver measures each process's peak resident memory, and polls it while the process runs (every 100 ms). Above the preset's budget (6 GB for `10m`, 12 GB for `100m`) it stops the process. The question is then reported as `out of memory` and isn't ranked, as db-benchmark reports a solution that runs out of memory. This also keeps a run from exhausting the machine the suite runs on.
- The 8-thread setting pins each process to 8 logical CPUs (CPU affinity where the OS has it) and sets every thread pool to 8. The 1-thread setting pins one CPU and sets every pool to 1 (`OMP_NUM_THREADS`, `OPENBLAS_NUM_THREADS`, `POLARS_MAX_THREADS`, `R_DATATABLE_NUM_THREADS`, `GC_MARKERS`, and Quark's `--threads` once it exists).

### 6.3 What is reported

Per question and tool: the first run, the median of the later runs, their minimum and maximum, peak memory, and the checksum result. `out of memory`, `timeout`, `wrong` (checksum mismatch) and `n/a` (no native operation, §7) are results, shown in the table. A summary per task gives each tool's total over the questions it ran.

### 6.4 Ties

When the minimum-to-maximum ranges of two tools overlap, the report prints `tie` instead of a ratio. A headline in `RESULTS.md` that one tool is faster needs ranges that don't overlap, at both presets.

## 7. Parity

A question measures what each tool does natively. For each tool and question:

- **Native**: the tool's own operation for this task, as an experienced user writes it (`df.group_by(...).agg(...)`, `dt[, .(...), by = ...]`, `t | e -> group_by(...) | summarize(...)`), with the options that give the same result as the others (`dropna=False`, `na.rm = TRUE`, `nulls_last=True`). That is what is timed.
- **`n/a`**: the tool has no such operation. It is not replaced by a composition of other operations or a loop. NumPy has no group by, so NumPy is `n/a` in every group-by question, instead of `np.unique` with `np.bincount` as now. NumPy's integer arrays can't hold a missing value, so NumPy is `n/a` on `G-NA`.
- **Quark**: if Quark has no native operation for a question, the question isn't in the suite until it does (§11). A Quark workaround, such as going through a list, is never measured.

## 8. Data

### 8.1 The table `G`

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

Every tool reads the same CSV file before timing (untimed except in §9.1).

### 8.2 The variant `G-NA`

`G` with 5% missing values in every column, by db-benchmark's rule: in a key column, 5% of its distinct values are replaced by a missing value in every row that has them, so the missing key is one group of about 5% of the rows; in a value column, 5% of the rows, drawn at random, are missing. A missing cell is an empty field in the CSV file. Quark aggregates skip missing values, and sort and group missing keys last (`specs/tables_v0_1_spec.md` §5); each peer uses the options that do the same (§7).

### 8.3 The labels `L`

N strings like `"  ITEM_4821_Lorem "` (padding, mixed case, underscores), as in the current `strings` benchmark, in a one-column CSV file. `L` has no missing-values variant.

### 8.4 The keys `D`

The right-hand table of the join questions: one row for each of 90% of the N/100 key values (the other 10%, drawn at random, are left out), with columns `k3: str`, `k4: int` and `w: float`. Row `i` has `k3 = 'id%010d' % i`, `k4 = i` and `w` uniform in [0, 1). About 90,000 rows at `10m` and 900,000 at `100m`. `D` has no missing values, so every tool gives the same result whatever it does with a missing key; `G-NA`'s missing keys are on the left. The questions use `D4`, its `(k4, w)` columns, and `D3`, its `(k3, w)` columns, selected before timing.

## 9. Questions

Each question lists Quark's operation and each peer's, or `n/a`. Quark's table code assumes `struct G` with the columns of §8.1; `t` is the table, and `v1` and `v2` are its columns as vecs. Questions on `G` also run on `G-NA` (DW10).

### 9.1 Read

| # | Question | Quark | NumPy | pandas | Polars | base R | data.table |
|---|---|---|---|---|---|---|---|
| RD1 | Read `G` with the column types given | `t: tbl[G] = unwrap(csv::read_file(p))` | n/a | `read_csv(dtype=...)` | `read_csv(schema=...)` | `read.csv(colClasses=...)` | `fread(colClasses=...)` |
| RD2 | Read `G` with the column types inferred | `t = unwrap(csv::read_file(p))` | n/a | `read_csv` | `read_csv` | `read.csv` | `fread` |

The file is in the OS page cache after the first run, so this measures parsing and building the columns.

### 9.2 Column expressions

| # | Question | Quark | NumPy | pandas | Polars | base R | data.table |
|---|---|---|---|---|---|---|---|
| EX1 | Sum and mean of `v2` | `sum(v2)`, `mean(v2)`; `summarize(s = sum(e.v2), m = mean(e.v2))` | `.sum()`, `.mean()` | Series `sum`, `mean` | `select(sum, mean)` | `sum`, `mean` | `dt[, .(sum(v2), mean(v2))]` |
| EX2 | New column `x = 2.5 * v2 + v1` | `2.5 * v2 + v1`; `mutate(x = 2.5 * e.v2 + e.v1)` | array expression | `assign` | `with_columns` | vector expression | `dt[, x := ...]` |
| EX3 | New column `z = (v2 * v2 - v1) * 0.5 + (v2 - v1) / (v1 * v1 + 1)` | the same, as vec and as `mutate` | array expression | `assign` | `with_columns` | vector expression | `dt[, z := ...]` |

Quark runs each as a vec and as a table (two rows in the report); NumPy and base R are vecs, the frame libraries tables. EX3 separates fused evaluation (Quark's `mutate`, Polars) from evaluation that materializes every intermediate.

### 9.3 Filter

| # | Question | Quark | NumPy | pandas | Polars | base R | data.table |
|---|---|---|---|---|---|---|---|
| FL1 | Rows where `v2 > 50` (about half) | `filter(e.v2 > 50.0)` | n/a | boolean indexing | `filter` | n/a | `dt[v2 > 50]` |
| FL2 | Rows where `k1 == 'id042'` and `v1 >= 3` (about 0.6%) | `filter(e.k1 == 'id042' and e.v1 >= 3)` | n/a | boolean indexing | `filter` | n/a | `dt[k1 == 'id042' & v1 >= 3]` |

Every column of the kept rows is materialized. A third question, rows whose `k4` is in a list of 1,000 values, waits for `contains` (§11).

### 9.4 Sort

| # | Question | Quark | NumPy | pandas | Polars | base R | data.table |
|---|---|---|---|---|---|---|---|
| SO1 | Sort `v2` | `v2.sort()` (§3) | `np.sort` | `Series.sort_values` | `Series.sort` | `sort(na.last = TRUE)` | n/a |
| SO2 | Sort `(k4, v2)` by `k4` | `sort(e.k4)` | n/a | `sort_values('k4')` | `sort('k4')` | n/a | `dt[order(k4)]` |
| SO3 | Sort `(k1, v2)` by `k1`, then `v2` descending | `sort(e.k1, desc(e.v2))` | n/a | `sort_values([...], ascending=[True, False])` | `sort([...], descending=[False, True])` | n/a | `dt[order(k1, -v2)]` |

SO2 and SO3 sort two-column tables (`select` before timing), so the gathered output stays inside the memory budget at `100m`. SO1 waits for §3.

### 9.5 Group by

| # | Question | Quark | pandas | Polars | data.table | db-benchmark |
|---|---|---|---|---|---|---|
| GB1 | `sum(v1)` by `k1` (100 groups, a str key) | `group_by(e.k1) \| summarize(s = sum(e.v1))` | `groupby('k1').agg` | `group_by('k1').agg` | `dt[, .(s = sum(v1)), by = k1]` | q1 |
| GB2 | `sum(v1)` and `mean(v2)` by `k3` (N/100 groups, a str key) | `group_by(e.k3) \| summarize(s = sum(e.v1), m = mean(e.v2))` | `groupby('k3').agg` | `group_by('k3').agg` | `dt[, .(...), by = k3]` | q3 |
| GB3 | `sd(v2)` and `max(v2) - min(v2)` by `k1, k2` (10,000 groups) | `group_by(e.k1, e.k2) \| summarize(s = sd(e.v2), r = max(e.v2) - min(e.v2))` | `groupby(['k1', 'k2']).agg` | `group_by('k1', 'k2').agg` | `dt[, .(...), by = .(k1, k2)]` | q6, q7 |

NumPy is `n/a`. base R's `tapply` does one aggregate of one key, so base R runs GB1 only.

### 9.6 Window

| # | Question | Quark | pandas | Polars | data.table |
|---|---|---|---|---|---|
| WN1 | New column `z = (v2 - mean(v2)) / sd(v2)` | `mutate(z = (e.v2 - mean(e.v2)) / sd(e.v2))` | `assign` with Series `mean`, `std` | `with_columns` | `dt[, z := ...]` |
| WN2 | New column `dev = v2 - mean(v2)` within each `k1` group | `group_by(e.k1) \| mutate(dev = e.v2 - mean(e.v2))` | `groupby('k1')['v2'].transform('mean')` | `pl.col('v2').mean().over('k1')` | `dt[, dev := v2 - mean(v2), by = k1]` |

Both use aggregates in `mutate` (`specs/tables_v0_3_spec.md` D35, implemented).

### 9.7 Pipeline

| # | Question | Quark |
|---|---|---|
| PL1 | Keep `v2 >= 10`, add `x = v2 * v1`, group by `k1, k2`, compute the row count, `sum(x)`, `mean(v2)` and `max(v2)`, and sort the groups by `sum(x)` descending | `t \| e -> filter(e.v2 >= 10.0) \| mutate(x = e.v2 * e.v1) \| group_by(e.k1, e.k2) \| summarize(n = count(), s = sum(e.x), m = mean(e.v2), hi = max(e.v2)) \| sort(desc(e.s))` |

pandas, Polars and data.table each write it as one chain; NumPy and base R are `n/a`.

### 9.8 Strings

| # | Question | Quark | NumPy | pandas | Polars | base R |
|---|---|---|---|---|---|---|
| ST1 | Trim, lowercase and replace `_` with `-` in every label of `L` | `mutate(s = e.s.trim().lower().replace('_', '-'))` | `np.strings` | `.str` methods | `.str` methods | `gsub(..., tolower(trimws(s)), fixed = TRUE)` |
| ST2 | Rows of `L` whose label contains `ITEM_48` | `filter(e.s.contains('ITEM_48'))` | n/a | `.str.contains(regex=False)` | `.str.contains(literal=True)` | `grepl(fixed = TRUE)` |

data.table uses base R's string functions, so it is the base R row.

### 9.9 Join

After db-benchmark's join task, on a left table of N rows and the right table `D` (§8.4). The left side is `(k4, v2)` or `(k3, v2)` from `G`, selected before timing, so the joined result stays inside the memory budget at `100m`.

| # | Question | Quark | pandas | Polars | data.table | db-benchmark |
|---|---|---|---|---|---|---|
| JN1 | Rows of `(k4, v2)` whose `k4` is in `D4` (about 90%; a semi join on an `int` key) | `g \| a -> semi_join(d4 \| b, a.k4 == b.k4)` | `g[g.k4.isin(d4.k4)]` | `join(how='semi')` | `g[k4 %in% d4$k4]` | |
| JN2 | `(k4, v2)` joined with `D4` on `k4`, keeping matched rows (each matches once) | `inner_join(d4 \| b, a.k4 == b.k4)` | `merge(how='inner')` | `join(how='inner')` | `d4[g, on = 'k4', nomatch = NULL]` | q2 |
| JN3 | `(k3, v2)` joined with `D3` on `k3`, keeping every row (`w` missing for about 10%; a `str` key) | `left_join(d3 \| b, a.k3 == b.k3)` | `merge(how='left')` | `join(how='left')` | `d3[g, on = 'k3']` | q3, q4 |

NumPy and base R are `n/a` (base R's join, `merge`, works on `data.frame`, which left the suite). The checksums (row count, `sum(w)`, `sum(v2 * w)` over present values) don't depend on row order, which Polars doesn't keep by default. db-benchmark's big join (a right table as large as the left) isn't included: both tables and the result don't fit the 16 GB laptop at `100m`.

### 9.10 Plain code

| # | Question | Quark | Python | R |
|---|---|---|---|---|
| FB1 | Recursive `fib(35)` | `fn fib(n: int) int` (a plain C++ function) | a function | a function |

The cost of a function call, an integer comparison and integer arithmetic in a loop the tool can't vectorize. `fib(35)` takes about 120 ms in Quark, 3 s in CPython and 18 s in R (scaled from `fib(30)` in `RESULTS.md`). It runs at both presets unchanged and isn't part of the data headline.

### 9.11 Matrix multiply

| # | Question | Quark | NumPy | R |
|---|---|---|---|---|
| MM1 | `C = A · B`, 1,000 × 1,000 | `blas::gemm(a, n, n, b, n)` (`std/blas`) | `a @ b` | `a %*% b` |

All three call a BLAS `dgemm`; this measures what `std/blas` adds around it (a flat vec with explicit dimensions, the call and the result). Quark's plain `vec` loops are not run (§7). The question grows when Quark has a matrix type.

## 10. What is removed

| Benchmark or tier | Why |
|---|---|
| `mandelbrot`, `sieve` | Plain code, which `fib` covers |
| `ewma` | Quark has no recurrence operation (its `vec` time is a loop), and neither does NumPy |
| `reduce`, `arith`, `expr` | Folded into EX1-EX3, on `G` |
| `dot`, `gemv`, `ols` | BLAS calls that `gemm` covers; `gemv` and `dot` finish in a few milliseconds |
| `filter`, `sort`, `sortkeys`, `groupby`, `zscore`, `query`, `readcsv`, `strings` | Replaced by §9.3, §9.4, §9.5, §9.6, §9.7, §9.1 and §9.8 on `G`, `G-NA` and `L` |
| `join`, `wordcount` | Quark has no join and can't build a table from a list, so its times were `dict` loops against real joins and counts. Joins come back with §5 |
| The list tier, base R `data.frame` | Element-at-a-time loops aren't how any peer does data work; `data.table` is R's tool at these sizes. `quark_blas` stays for `gemm` only |

## 11. What waits for a Quark feature

| Question | Waits for |
|---|---|
| SO1 (sort a vec) | §3 |
| The join questions (§9.9) | §5 |
| FL3: rows whose `k4` is in a list of 1,000 values | `contains` in verbs (`specs/tables_v0_3_spec.md` D48) |
| `median(v2)` by key, the largest two `v2` per group (db-benchmark q6, q8) | A quantile kernel; top-n per group |
| Word count | Building a table from a list, and v0.3's `count` step |
| The 8-thread headline (DW12) | The rest of `specs/tables_v0_3_spec.md` §8: the CSV reader, `distinct` and the grouped aggregates of many small groups still run on one thread |

## 12. What the 100M preset will show

These are expected from today's code and are to be confirmed by the first `100m` run.

- **Until §4 is done, Quark can't load `G` at `100m` on a 16 GB laptop**: about 15 GB at the reader's peak, so RD1, RD2 and every task that loads `G` would be out of memory.
- **A `str` column holds at most 2 GiB of text** (32-bit offsets; past that, `vec[str] data exceeds 2 GiB` is a runtime error). `k3` at `100m` is 1.2 GB and `L` about 1.8 GB, so both fit, `L` with little room.
- **What still runs on one thread.** Query row loops, most of the table engine and the vec operations run on the thread pool (`specs/tables_v0_3_spec.md` §8); the CSV reader, `distinct`, the grouped aggregates of many small groups and joins v0.1 (DW23) don't yet, and the 8-thread setting shows what that costs.

## 13. Changes to other docs

When each part is implemented: `stdlib.md` lists `sort`, `sort_desc`, `order` and `order_desc` under the vec methods (§3); `architecture.md` describes the CSV reader's blocks and buffers (§4); joins go into `semantics.md` §9.5 (the verbs and the joined row), `grammar.md` §10.5 (the `right | b` argument), `architecture.md` (the engine's hash join) and the editor grammar's verb names, and `specs/tables_v0_1_spec.md` §9 stops listing them as planned (§5); `specs/tables_v0_3_spec.md` §8.1 gets the radix kernel as the engine's single-key sort once it is parallel (§3.2), the streamed reader's parallel parse (§4.2), and joins once they are parallel (§5.5); `src/benchmarks/README.md` is rewritten for the tasks, presets, variants, method and parity rule here; `src/benchmarks/RESULTS.md` is measured again at both presets; `architecture.md` §15 takes its performance findings from the new results.

## 14. Tests

- **Vec sort and order**: every element type; missing values, NaN, `-0.0` and `0.0`, ties (stability), empty and all-missing vecs, a vec of unknown element type, and both directions. The vec sort, the table sort and the list sort agree on the same values (differential); `t.x.sort()` equals `(t | e -> sort(e.x)).x`; `v.sort()` equals `v` read at `v.order()`. A wrong argument count is a compile-time error.
- **CSV memory**: a generated file of 10M rows, typed and untyped, read within DW3's bound (checked as `integration_gc_test.go` checks heap size and peak resident memory). The existing CSV tests pass unchanged, including rows that cross a block boundary inside a quoted field, a row longer than a block, and a file of exactly one block.
- **Joins**: each of the four joins on duplicate keys on both sides, missing and NaN keys on either side, `-0.0` against `0.0`, several keys, an empty table on either side, and keys with different names; the row order of §5.3; the result stored into a named struct by shape. Rejected at compile time (`TestSemanticsSafety_Rejected`): a condition other than `and`-joined equalities, a key of the wrong type, an unknown column on either side, the same row name for both sides, a missing `| b`, an unparenthesized query on the right, a clashing column, a right side of unknown row type or of type `any`, a right side that isn't a table, a right side that reads the query's row, and a join after `group_by`. A right side from an imported file, from an `extern fn` returning `tbl[R]`, and from a function's default-valued parameter. A result past the row limit is a runtime error.
- **Suite**: the `small` check of today becomes a `tiny` preset (100K rows) that runs every question of every tool, on `G` and `G-NA`, in a few minutes, and fails when a checksum disagrees. The memory limit is tested with a process that allocates past it.

## 15. Plan

1. The CSV reader (§4): the `100m` preset depends on it.
2. The vec sort and order (§3), with the table engine's single-key sort moved to its kernel.
3. The data generator for `G`, `G-NA` and `L` at `tiny`, `10m` and `100m`.
4. The driver: one process per task and variant, first run reported separately, memory polling and the out-of-memory result, CPU pinning, ties.
5. The questions of §9, per tool.
6. Joins (§5), and their questions (§9.9).
7. A run at both presets, both variants and both thread settings; `RESULTS.md` and `architecture.md` §15 from it.

The window questions need tables v0.3's aggregates in `mutate`, which are implemented. FL3 and the 8-thread headline wait for v0.3 (§11).

## 16. Later

- **Keyword arguments** come with other syntax changes to the language (DW22). Then `v.sort(desc = true)` can replace `sort_desc` and `order_desc`.
- **Joins**: `full_join`; conditions other than equality; keys of different numeric types; joins on tables whose row type the compiler doesn't know (`specs/untyped_tables_v0_1_spec.md`).
- The questions of §11, as their features arrive.
