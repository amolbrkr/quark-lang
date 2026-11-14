# Quark Tables v0.2 Spec

Status: Draft, 2026-11-14. Nothing here is implemented. Tables v0.1 is the language as it runs today (`specs/tables_v0_1_spec.md`); this spec adds to it. A program that compiles under v0.1 compiles under v0.2 and gives the same results, apart from the new syntax.
Last Updated: 2026-11-14
Related: `specs/tables_v0_1_spec.md` (the implemented language and decisions D1–D25), `architecture.md` §15 (the measurements), `src/benchmarks/RESULTS.md`, `known_issues.md` (Slow paths).

## Contents

1. [Purpose](#1-purpose)
2. [Decisions](#2-decisions)
3. [Verbs](#3-verbs)
4. [Engine requirements](#4-engine-requirements)
5. [Tests](#5-tests)
6. [Deferred](#6-deferred)

## 1. Purpose

v0.1 reads a CSV file into a `tbl[R]`, then filters, mutates, groups, summarizes and sorts it. Two things are still missing for that loop, and the benchmark suite shows where the loop that exists spends its time.

This spec adds the verbs that reshape a table (`select`, `deselect`, `rename`, `distinct`) and the aggregate `n_distinct`, and faster implementations of the v0.1 operations the suite measured. The column-wise table literal (D19, §6.1) and joins come later.

The results of every v0.1 operation stay as v0.1 defines them: the same rows, the same order, the same missing values, the same errors. The fast paths are requirements on the implementation, not new behaviour a program can observe, except by how long it takes.

## 2. Decisions

v0.1's decisions D1–D25 still hold. These are the new ones.

| # | Decision |
|---|---|
| D26 | **`select` keeps columns.** `select(e.id, e.name)` is a query step. Its arguments are columns of the input row, written `e.col`, in the order to keep. The output is an anonymous row of those columns. An expression is a compile-time error. Dropping columns is `deselect` (D30). |
| D27 | **`rename` renames columns in place, all at once.** `rename(emp = e.id)` changes names and keeps each column's position and type. The renames are applied together, so `rename(a = e.b, b = e.a)` swaps the names. A new name that collides with a column this step does not rename is a compile-time error. The right-hand side is a column, not an expression. |
| D28 | **`distinct` keeps the first row of each key, in input order, with all of its columns.** `distinct()` uses every column as the key; `distinct(e.dept, e.year)` uses those columns. Keys are columns, as in `group_by`. Two missing values are the same key, and so are two NaNs, which is the grouping equality (`rows_equal`), so a missing key is its own group as in D6 and D9. The row kept is the first in the table's current order, not the smallest key: `distinct` does not sort. The output row type is the input's row type. |
| D29 | **`n_distinct(e.col)` counts those keys.** It is a `summarize` aggregate of one column, so D24 still holds, and it is not a function outside `summarize`. The count is the number of rows `distinct(e.col)` would keep: present values, plus one if any value is missing, plus one if any float is NaN. An empty table counts 0. The result is an `int`, never missing. It differs from `sum` and `mean` on purpose: those skip missing values, and this one is counting keys. |
| D30 | **`deselect` drops columns.** `deselect(e.id, e.dept)` is a query step. Its arguments are columns, written `e.col`. The output is an anonymous row of every other column, in the input's order, and every row is kept. `select` does not take `-e.col`: `-` is arithmetic, and a `select` argument is a column, not an expression. Naming every column is a compile-time error. |
| D31 | **The engine fast paths in §4 are part of v0.2.** Each one produces the same values as the v0.1 path it replaces. A path that can change a result, a row order or an error is wrong even if it is faster. |
| D32 | **Sort stays stable.** Equal keys keep their order, as v0.1 specifies. The numeric sort is held to a stable sort, not to NumPy's unstable one (the numbers are in §4.3). |
| D33 | **A string cell is compared and hashed as a view of the column buffer.** A pointer and a length, with no allocation. A Quark string is allocated only when the cell is stored into a new column or passed to a function or method. The representation of Quark strings themselves does not change in v0.2. |
| D34 | **v0.2 stays single-threaded.** No verb and no reader gains a thread pool. Parallel execution is a later spec. |

## 3. Verbs

The query syntax is unchanged (`grammar.md` §10.5): one row name, then steps. `select`, `deselect`, `rename` and `distinct` are verbs, resolved as syntax the way `filter` is, so a user function of the same name has no effect on a step. `n_distinct` is an aggregate name inside `summarize`, the way `sum` is.

```quark
struct Employee:
    id: int
    name: str
    dept: str
    salary: float

staff: tbl[Employee] = unwrap(csv::read_file('staff.csv'))

eng = staff | e ->
    filter(e.dept == 'eng')
    | select(e.name, e.salary)
    | rename(pay = e.salary)
    | distinct(e.name)

rest = staff | e -> deselect(e.id, e.dept)
```

`e` in each step is that step's input row. `group_by` is still followed only by `summarize`; a `select`, `deselect`, `rename` or `distinct` after `group_by` is the same compile-time error as a `filter` there.

### 3.1 select

`select(e.name, e.salary)` keeps those columns in that order and drops every other column.

- The output row is anonymous: `tbl[{name: str, salary: float}]`. Column types are unchanged. Storing it into a `tbl[R]` follows v0.1 shape matching, so the selection can be given a struct name at the variable that holds it.
- Each argument is one column, written `e.col`. `select(e.salary * 1.1)`, `select(pay = e.salary)` and `select(-e.id)` are compile-time errors (`select keeps columns (e.col); drop columns with deselect, compute a column with mutate, or rename it with rename`).
- Zero arguments, an unknown column and the same column twice are compile-time errors. The unknown-column error lists the columns, as a bad `e.col` elsewhere does.
- `select` on a table whose row type is not known is the D11 error.

### 3.2 deselect

`deselect(e.id, e.dept)` drops those columns and keeps every other column, in the order they have in the input. Every row is kept.

```quark
// columns: id, name, dept, salary
staff | e -> deselect(e.id, e.dept)
// name, salary
```

- The output row is anonymous: the input's columns except the ones named, types unchanged, input order. Storing it into a `tbl[R]` follows v0.1 shape matching.
- Each argument is one column, written `e.col`. `deselect(-e.id)` and `deselect(e.salary * 1.1)` are compile-time errors (`deselect drops columns (e.col)`).
- Zero arguments, an unknown column and the same column twice are compile-time errors. The unknown-column error lists the columns.
- Naming every column of the input is a compile-time error (`deselect would remove every column`).
- `deselect` on a table whose row type is not known is the D11 error.

### 3.3 rename

`rename(pay = e.salary, department = e.dept)` replaces those names. Columns that are not mentioned keep their names. Positions and types do not change, and no column is added or dropped.

- The renames happen together. Each right-hand side is the input column, and each left-hand side is the name it will have in the output. `rename(a = e.b, b = e.a)` exchanges the two names.
- The output row is anonymous, with the new names in the input's column order.
- A right-hand side that is not a bare `e.col` is a compile-time error (`rename takes a column (pay = e.salary)`).
- Two renames to one new name, or a new name equal to a column this step does not rename, is a compile-time error (`rename would give two columns named 'pay'`). Renaming a column to the name it already has is allowed.
- Zero arguments is a compile-time error. An unknown column is a compile-time error that lists the columns.

### 3.4 distinct

`distinct(e.dept, e.year)` keeps, for each different key, the first row of the input in its current order, including the columns that are not keys. `distinct()` does the same with every column as the key.

```quark
// rows: (eng, Ann), (ops, Bo), (eng, Cy)
staff | e -> distinct(e.dept)
// (eng, Ann), (ops, Bo)     Cy is dropped; eng stays first, not sorted
```

- The output has the input's row type, the way `filter` and `sort` do. A `tbl[Employee]` stays a `tbl[Employee]`.
- A key is a column. `distinct(e.salary * 1.1)` is a compile-time error that says to `mutate` the value first, as `group_by` does.
- An unknown column, or the same column twice, is a compile-time error.
- Equality is the grouping equality: two missing values match, two NaNs match, and `0.0` matches `-0.0`. A column of only missing values contributes one key when the table has at least one row.
- The kept rows are in the order their keys first appear. `distinct` does not order keys the way `summarize` does. Implementing it by sorting, or by taking `group_by`'s key-ordered groups, would be wrong.
- An empty table gives an empty table of the same row type.

### 3.5 n_distinct

Inside `summarize`, `n_distinct(e.dept)` is the number of distinct keys in that column, for the whole table or for one group.

```quark
staff | e -> summarize(depts = n_distinct(e.dept), n = count())
staff | e -> group_by(e.dept) | summarize(people = n_distinct(e.name))
```

- One column argument. `n_distinct()`, `n_distinct(e.a, e.b)` and `n_distinct(e.a + e.b)` are compile-time errors. A count of several columns is `distinct(e.a, e.b)` and then `count()`.
- The result column is `int`. A group with no rows cannot occur; a group whose rows are all missing counts 1, because the missing key is a key (D28). A `summarize` of an empty table counts 0.
- It is only an aggregate. `n_distinct(staff.dept)` outside a query is an unknown name, and a `mutate` column cannot call it.
- `min`, `max`, `sum`, `mean` and `count` are unchanged, including that `min` and `max` of no present values are missing.

### 3.6 Row types

| Step | Output row type |
|---|---|
| `select(e.name, e.salary)` | Anonymous, those columns, in that order |
| `deselect(e.id, e.dept)` | Anonymous, the other columns, in the input's order |
| `rename(pay = e.salary)` | Anonymous, the input's types and order, with the new names |
| `distinct(...)` | The input's row type, named or anonymous |
| `summarize(... n_distinct ...)` | As in v0.1, with that column an `int` |

`filter`, `sort`, `head` and `tail` still preserve the row type. Shape matching (§3.2 of the v0.1 spec) applies to the anonymous rows `select`, `deselect` and `rename` produce.

## 4. Engine requirements

These replace the slow paths measured on 2026-11-14 (one thread; the Linux numbers are in `src/benchmarks/RESULTS.md`, and the 500K-row sort is in `architecture.md` §15). They are not pass/fail time limits: those machines disagree, and a later machine will too. The implementation remeasures the benchmarks named below and records the new times in `RESULTS.md`. A result that differs from v0.1 fails even when the time improved.

Which loop runs is decided when the step runs. A column's `null_count` is a property of the value, and a CSV column's nulls are not known at compile time. Generated code may keep both loops and pick from `null_count` at the start of the step.

### 4.1 min and max

`agg_extremum` compares every row through `compare_rows`, which tests nulls, switches on the column type and, for a float, tests NaN, then compares. `summarize(lo = min(e.a))` on 10M floats takes 76 ms. `min(t.a)` takes 6.1 ms, and `summarize(s = sum(e.a), m = mean(e.a))` takes 12 ms. Most of the `reduce` benchmark's table time (175 ms, against 18 ms for the vec) is this aggregate.

v0.2 compares raw values in one loop per column type, the way `agg_sum` and `q_vec_extremum` already do:

- The type switch happens once, outside the loop. A null test is the validity bit for that type. A float loop skips NaN the way `q_vec_extremum` does, so a NaN never wins and the remaining comparisons are raw `double` comparisons.
- A `summarize` with no `group_by` uses that dense loop for the whole column. It must not call `q_vec_extremum` itself: a vec `min` of no present values is a runtime error, and the aggregate of no present values is missing (D13).
- Per group, the same typed loop updates that group's best value. Ties keep the first value, as `q_vec_extremum` and today's aggregate do.
- Strings compare as byte views (D33), with no Quark string allocated.

`compare_rows` remains the general comparator. `min` and `max` stop using it.

### 4.2 filter and mutate

A `filter` or `mutate` tests each column's validity on every row, including a column with no missing values, and `filter` appends a selected row with a branch. On 10M rows, `filter(e.a > 0.5)` takes 99 ms (Polars 24 ms) and `mutate(z = 2.5 * e.a + e.b)` takes 65 ms (the vec expression 45 ms, NumPy 29 ms).

When every column the expression reads has `null_count == 0`:

- The loop does not test validity. Column pointers are locals, so the loop can vectorize.
- `and`, `or` and `!` are ordinary bool operations. A missing operand cannot occur, so this agrees with three-valued logic on that input.
- `filter` gathers the selected row indexes without branching on the predicate per row.
- A `mutate` whose inputs are all dense writes a column with `null_count == 0`.

When any column the expression reads has a missing value, the v0.1 loop runs, three-valued logic included. An expression that calls a function still calls it once per row (D21); on a dense input it is called for every row, because nothing is missing. Arithmetic and comparisons around the call still take the dense loop.

NaN is a value here, as in v0.1 arithmetic. A dense float comparison is the raw comparison, so a NaN predicate is false and the row is dropped.

### 4.3 sort

Sorting stays stable (D32). Missing keys stay last, and NaN stays after every number and before missing values, in either direction.

Today `sort_perm` has a fast path for one dense numeric key and sends everything else through `compare_rows`: a string comparison and a type switch per key per comparison. The measured costs:

| Sort | Time | Comparison |
|---|---|---|
| One dense float key, 1M rows (`sort`) | 173 ms | NumPy 26 ms, Polars 80 ms, `std::stable_sort` of the same doubles 113 ms |
| One float key with 1% missing, 500K rows | 239 ms | the dense path on that key, 97 ms |
| A string key then a float, 1M rows, 1,000 distinct strings (`sortkeys`) | 1.13 s | data.table 255 ms, pandas 364 ms |

A single NaN or a single missing value also leaves the fast path, because the fast path bails out.

v0.2 has three paths, and `compare_rows` is no longer on any of them:

1. **One numeric key with no missing values.** The existing sort of `(value, row)` pairs. NaN values are partitioned out first and placed after the numbers, so one NaN does not leave this path. The pair sort is what stability requires; a radix sort is not part of v0.2. NumPy's 26 ms is an unstable SIMD sort, and matching it is not a goal while D32 holds. The number to put beside the new measurement is `std::stable_sort` (113 ms) and Polars (80 ms).
2. **One numeric key with missing values.** Partition the missing rows off, sort the present rows on path 1, then append the missing rows. They stay last in their original order.
3. **A string key, or more than one key.** Encode each string key as the rank of its bytes among the distinct values in that column: one hash pass, a sort of the distinct values, then an integer per row. Missing cells share one rank that sorts last, past every value and past NaN. Equal strings share a rank, so the stable row sort keeps their order. Numeric keys on this path are compared as raw values of a known type, not through `compare_rows` and not by ranking every distinct number. The row sort is then a stable sort of integers and raw numbers.

Path 3 is what `sortkeys` measures. Ranking the strings is the work; the row sort then compares integers.

### 4.4 String cells

`q_tbl_str` allocates a Quark string for every cell it reads. A `filter` that compares `e.dept` pays for that, and so does hashing a string group key. `filter(e.dept == 'eng' or ...)` takes 38 ms, and `mutate(up = e.name.upper())` takes 54 ms.

Under D33, equality, ordering and hashing of a string cell inside `filter`, `mutate`'s comparisons, `group_by`, `distinct`, `sort` and `n_distinct` use the cell's pointer and length in the column buffer. No Quark string is allocated. A comparison with a Quark string compares against those bytes.

A Quark string is still allocated when the cell is stored into a new column (`mutate(s = e.name)`), or passed to a function or method (`e.name.upper()`, `grade(e.name)`). `trim().lower().replace()` on a string column therefore costs what it costs today. That cost is the string type: no stored length, and `lower` calls `tolower` per byte (`architecture.md` §15). Changing the string type is out of scope (§6).

### 4.5 CSV reader

`readcsv` reads a 52 MB, five-column file in 513 ms (pandas 226 ms, data.table 301 ms, Polars 357 ms), about 100 MB/s. Reading only the string column still takes 280 ms, so splitting fields is about half of it.

The reader finds separators, quotes and line endings with a vectorized scan, and parses quoted fields with a scalar scan. A quoted field may contain a newline, so a scan for newlines alone is not a parser.

Everything a program can observe stays as `specs/tables_v0_1_spec.md` §8: the same types, the same missing values, the same UTF-8 rejection, and the same errors naming the file, line, column and text. The UTF-8 check stays, including on the fast path. Pandas does not make that check, so the remaining gap after the scan is not a defect by itself.

### 4.6 Results that must not move

Checked against the v0.1 tests and the engine tests, on the fast path and the slow path alike:

- `filter` drops a row whose predicate is false or missing. Three-valued `and`, `or`, `!` and ternaries are unchanged on an input that has a missing value.
- `min` and `max` skip missing values and NaN, keep the column's type, keep the first value on a tie, and are missing when a group has no present value. Strings compare by bytes.
- `sum`, `mean` and `count` are untouched, including pairwise float sums.
- Sort is stable. Missing keys sort last, NaN after numbers and before missing keys, in either direction.
- `group_by` still emits groups in key order. `distinct` emits first-seen order (§3.4). The two orders are different on purpose.
- The CSV reader's errors and accepted input are unchanged.

## 5. Tests

Language tests extend `TestTables_*` in `integration_tables_test.go`. Each new verb gets the misuse cases as compile-time errors: an unknown column, a duplicated column, an expression where a column is required, a use after `group_by`, a table whose row type is not known, and `n_distinct` outside `summarize`. `deselect` of every column is a compile-time error. The table literal is not part of v0.2 (§6.1).

Result tests that pin the fast paths down:

- `distinct` keeps the first row and does not sort. A missing key and a NaN key are each one group. `n_distinct` counts both, and counts 0 on an empty table.
- `rename(a = e.b, b = e.a)` swaps. A colliding new name is rejected.
- `select` of a subset stored into a struct of that shape is accepted; a subset stored into the original row struct is rejected with the columns named.
- `deselect(e.id, e.dept)` keeps the other columns in the input's order and keeps every row. Storing that into a struct of the remaining shape is accepted; storing it into the original row struct is rejected with the columns named.
- `min` and `max` of a float column that contains NaN and missing values agree with the vec reductions on the present numbers, and a group of only NaN is missing. The existing agreement test is `src/testfiles/table/reduce_test.cpp`.
- A sort with a NaN, a sort with a missing key, and a sort with a string key and a float key keep v0.1's order, including stability for equal keys.

The benchmarks to remeasure, against the compiler from before the engine change, are `reduce`, `filter`, `expr`, `sort`, `sortkeys`, `groupby`, `strings` and `readcsv`. `strings` is expected to stay slow (§4.4). The new times replace the tables in `src/benchmarks/RESULTS.md` when they have changed materially, and the matching bullets in `architecture.md` §15 are rewritten to the new numbers. The slow-path entries in `known_issues.md` that this work fixes are deleted in that change, with the fix described in `changelog.md`.

## 6. Deferred

Not part of v0.2.

### 6.1 Table literal

The column-wise literal decided as D19. `tbl` stays a type name. A program builds a `tbl[R]` from one vec per field. It is an expression, so it can be assigned, passed and returned.

```quark
staff = tbl[Employee] {
    id: vec [1, 2],
    name: vec ['Ann', 'Bo'],
    dept: vec ['eng', 'ops'],
    salary: vec [120000.0, 90000.5],
}
```

- `R` is a struct whose fields are all column types (`int`, `float`, `bool`, `str`), the same structs a `tbl[R]` annotation accepts. `tbl[geo::Point] { ... }` is allowed when `Point` is such a struct. A bare `tbl { ... }`, a `tbl` of an anonymous row, or a struct with a field a table cannot hold is a compile-time error.
- The brace body has the form of a struct literal: `field: expr`, separated by commas, a trailing comma allowed, and newlines inside the braces ignored. Field order in the literal does not matter. The table's column order is the struct's field order.
- Every field appears once. A missing field, an unknown field and a repeated field are compile-time errors, naming the field.
- Each expression is a `vec`. When the compiler knows the vec's element type, it checks it against the field: the types match, or the vec is `int` and the field is `float`. A mismatch is a compile-time error (`field 'salary' of Employee expects float, got vec[str]`). A vec whose element type the compiler does not know is checked when the literal runs.
- An `int` vec for a `float` field is widened by converting the column. Any other matching vec is shared: the column is that vec's buffer, and no copy is made. Vecs are immutable, so the sharing is not observable.
- The vecs must have one length. A length the compiler can see (a vec literal, or the same expression reused) that disagrees with another is a compile-time error. Otherwise a different length is a runtime error naming the field and both lengths (`table literal for Employee: field 'name' has 3 values, but 'id' has 2`). Zero-length vecs make a table of zero rows. An empty `vec []` takes its field's type.
- Missing elements stay missing. They come from the vec (D12); the literal adds none of its own.
- The value's type is `tbl[R]`, the named row, not an anonymous row.

`tbl[` and a type and `{` is how the parser recognizes a literal. `tbl[i]` on a variable named `tbl` is still an index, because no `{` follows. This syntax does not change the meaning of any program v0.1 accepts.

When this is implemented, the tests are: a missing field, an unknown field, a repeated field, a wrong element type, an `int` vec widened to `float`, a length the compiler can see that disagrees, a length it cannot (a runtime error), a zero-row table, a shared `str` or `float` buffer, and missing elements that stay missing.

### 6.2 Later

Specified elsewhere:

- **Joins**, of every kind. They need their own spec: the syntax for a second row, which joins exist, whether a missing key matches another missing key, and what happens when a column name exists on both sides.
- **`summary(t)`** and the quantile kernel (`specs/tables_v0_1_spec.md` §6).
- **`slice`**, date and categorical columns, `map`, `list[T]`, DuckDB, and compile-time schema inference (v0.1 spec §9 and §10).
- **Row access** (`t[i]`) and masks (`specs/tables_v0_1_spec.md` §12).

Out of scope because they are not table operations. The suite measured them, and this spec does not schedule them:

- Quark's string representation: storing a length, and an ASCII path for `lower` and `trim`. This is what the `strings` benchmark is waiting on.
- `list` and `dict` layout: scanned `QValue` buffers and the node-based dict. A table `group_by` is already the fast path for that work.
- A thread pool for verbs or for the reader.
- The OpenBLAS thread pool disturbing large allocations in the garbage collector (`known_issues.md`). It shows up in programs that import `std/blas`, not in table code.
- Huge pages for vec buffers, and fusing vec expressions. A table `mutate` is already one loop.
