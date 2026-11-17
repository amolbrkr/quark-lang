# Quark Untyped Tables v0.1 Spec

Status: Proposal, incomplete: more is to be added before it is reviewed. Nothing is implemented. §6 (typing a read from its file at compile time) is wanted, but whether it is built is not decided.
Last Updated: 2026-11-17
Related: `specs/tables_v0_1_spec.md` (D11, D18, D19), `specs/tables_v0_2_spec.md`, `specs/tables_v0_3_spec.md` (aggregates, missing values in verbs, row-safe calls, threads), `semantics.md` §9.5, `stdlib.md` (`std/csv`), `known_issues.md` (the REPL drops warnings).

## Contents

1. [Purpose](#1-purpose)
2. [Decisions](#2-decisions)
3. [Columns of an untyped table](#3-columns-of-an-untyped-table)
4. [Queries on an untyped table](#4-queries-on-an-untyped-table)
5. [The slow-query warning](#5-the-slow-query-warning)
6. [Typing a read from its file at compile time (wanted, not decided)](#6-typing-a-read-from-its-file-at-compile-time-wanted-not-decided)
7. [Performance](#7-performance)
8. [Changes to earlier specs](#8-changes-to-earlier-specs)
9. [Tests](#9-tests)
10. [Plan](#10-plan)
11. [Open questions](#11-open-questions)

## 1. Purpose

A table read without a row struct (`t = unwrap(csv::read_file(path))`) has the type `tbl`: its row type is unknown to the compiler. Today such a table can be printed, passed on, written, and inspected with `head`, `tail` and `describe`, but reading a column or running a query on it is a compile-time error:

```
error[QK-CHECK-001] (check): the row type of this table isn't known, so its columns aren't either; give it a row struct where it is created or passed (t: tbl[Employee])
```

So exploring a file means writing a struct for it first, when the column names and types may not be known yet. Elsewhere Quark has a middle path for values the compiler can't type: they are checked at runtime (`any`, unannotated parameters). Tables already get it by accident: the same table passed through an unannotated function has its columns checked at runtime (`fn avg(t) -> mean(t.mpg)` works, and `t.mpgg` is a runtime error that lists the columns). A bare `tbl` is stricter than a value of unknown type.

This spec gives untyped tables that middle path. Column reads and queries compile and are checked at runtime, and every misuse fails loudly with a location. A query on an untyped table is slower, and the compiler says so with a warning. Typed tables don't change.

```quark
incl 'std/csv'
incl 'std/sys'

path = sys::args()[0]
t = unwrap(csv::read_file(path))      // row type unknown at compile time
print(mean(t.mpg))                    // the column is found and checked at runtime
fast = t | e -> filter(e.mpg > 25) | sort(desc(e.mpg))
// warning[QK-CHECK-003] (check): this query is on a table whose row type isn't known, ...
cars: tbl[Car] = fast                 // the row type is checked here, as today
```

## 2. Decisions

| # | Decision |
|---|---|
| U1 | **A column of an untyped table can be read**, and is checked at runtime. `t.mpg` on a bare `tbl` is that column as a vec whose element type the compiler doesn't know, used like a value from an untyped function. A missing column is a runtime error that lists the columns, as it is today for a table reached through an untyped value. |
| U2 | **A query on an untyped table compiles.** A step whose arguments are only columns finds them by name when it starts and runs in the table engine as it does on a typed table (§4.1). A step with expressions of the row runs a checked loop: each cell is a boxed value, and every operation checks its operands' types (§4.2). |
| U3 | **An untyped query gives the same output as the same query on the table read with its row struct**, except for the column types of U4. That covers missing values (D6, D22, D46), three-valued `and`/`or`, calls with a missing argument, int overflow, int/float comparison, row order, the order of NaN and missing values, group keys, and the bits of float aggregates. Differential tests enforce it (§9). |
| U4 | **A computed column's type comes from its values.** Int and float values together make a float column. Any other mix is a runtime error that names the column and the row, and so is a computed column with no present values (decided 2026-11-17). Where the typed rules give float and every value is an int, the two differ: `mutate(x = 1 if e.flag else 2.5)` where every `flag` is true is an int column on an untyped table and a float column on a typed one. |
| U5 | **Every column a step names is checked when the step starts**, before any row runs, so a misspelled column is reported even on an empty table. So is a check that needs only a column's type (`mean(e.name)` on a str column). A type error inside an expression is reported at the first row that hits it, with the row number; on an empty table it isn't reported, as with code on `any` values that never runs. |
| U6 | **What doesn't depend on the row type stays a compile-time error**: verbs and their arguments, where aggregates may appear (v0.3 §3), where `group_by` may appear (D36), the row-safety of calls (D41–D44), and the names and arities of called functions. |
| U7 | **The result of a query on an untyped table is an untyped table.** Its row type becomes known where it is stored in a `tbl[R]` slot, which checks it as today (semantics.md §9.5, "Matching rows by shape"). |
| U8 | **Each query on an untyped table is a compile-time warning, once per query** (§5). `--no-warn` silences every warning (decided 2026-11-17). The REPL shows this warning once per session (decided 2026-11-17). |
| U9 | **A checked loop runs on one thread.** Column-only steps run in the engine and use the thread pool as on a typed table (v0.3 §8). |
| U10 | **Typing a read from its file at compile time is wanted but not decided** (§6). If it is built, a read with a literal path gets a known row type and none of this spec applies to it. |

## 3. Columns of an untyped table

`t.col` on a bare `tbl` is lowered like a column read on a value of unknown type is today (`q_member`, `ops/member.hpp`): the column is looked up by name in the table's row definition, which every table carries at runtime, and returned as a vec. A missing column is:

```
error[QK-RUNTIME-001] (runtime): tbl[{model: str, mpg: float, cyl: int}] has no column 'mpgg' (columns: model, mpg, cyl) at a.qrk:5:9
```

The vec's element type is unknown to the compiler, so what is done with it is checked where it is used: `mean(t.mpg)` checks for a numeric or bool vec at runtime, and `m: vec = t.mpg` stores it as a vec. Table methods (`head`, `tail`, `describe`, and v0.3's `nrow`, `ncol` and `names`) work on a bare `tbl` as they do now.

## 4. Queries on an untyped table

### 4.1 Steps that take only columns

These steps name columns and nothing else, so they need no per-row evaluation. On an untyped table each one looks its columns up by name when it starts (U5), then runs in the table engine, which already works on the columns' runtime types:

- `select`, `deselect`, `rename`, `distinct`, `sort` (column keys, `desc(e.col)`), the `head` and `tail` steps;
- `group_by` with column keys, followed by a `summarize` whose columns are `count()` or an aggregate of a single column (`mean(e.mpg)`);
- v0.3's `count(e.k)` step and `drop_na(e.a, e.b)`.

An aggregate of one column picks the kernel for the column's runtime type, so it gives the same bits as on a typed table. An aggregate that doesn't apply to the column's type (`sum(e.name)`) is a runtime error when the step starts.

### 4.2 Steps with expressions: the checked loop

`filter`, `mutate`, computed group keys, aggregates of expressions and `summarize` expressions read the row inside an expression, so they need per-row code. The compiler lowers each expression once, with every `e.col` read as a boxed value:

- `e.col` gives the row's cell, boxed. A missing cell is the verb's missing value, not `null`.
- Operators and calls follow the verb rules, not the rules for `any` values. Under the `any` rules `null > 4` is an error; in a verb it is missing. So the loop uses missing-aware boxed helpers: a missing operand gives a missing result, `and`/`or`/`!` are three-valued (D22), a call with a missing argument isn't made and gives missing (D21), and `x == null` tests for a missing value (D46).
- A row-safe user function is called through its `QValue` convention. A call that doesn't read the row still runs once per step (D44).
- `filter`'s condition must be a bool. Any other value is a runtime error at that row. A missing condition drops the row, as on a typed table.
- A computed key or an aggregate of an expression is computed into a column first, and then grouped or aggregated by the same kernels as a column.
- A str cell is copied to a NUL-terminated string when it is boxed (§7).

### 4.3 Types of computed columns

A `mutate` column, a computed key or a `summarize` column collects its values, and its type is decided from them (U4):

| Values | Column |
|---|---|
| All one type | That type |
| Int and float | Float, ints converted |
| Any other mix | Runtime error: `column 'x' has int and str values (row 17 is str)` |
| No present values | Runtime error: `column 'x' has no values to take its type from` |

A `mutate` that replaces a column may change its type, as on a typed table.

### 4.4 Errors

Every error is a `QK-RUNTIME-001` with the query's location, and with the row for a per-row error. The row number counts from 1 in the step's input. The texts below show what each must say; the exact wording is settled when it is implemented.

```
error[QK-RUNTIME-001] (runtime): tbl[{model: str, mpg: float}] has no column 'mpgg' (columns: model, mpg) at a.qrk:6:24
error[QK-RUNTIME-001] (runtime): row 3: operator '>' is not defined for str and int ('e.model > 4') at a.qrk:7:22
error[QK-RUNTIME-001] (runtime): row 9: filter's condition is int, not bool at a.qrk:8:18
```

### 4.5 What stays at compile time

U6: an unknown verb, a wrong argument count, an aggregate where none may appear, a `group_by` not followed by a step that takes it, a call that isn't row-safe, and an unknown function are compile-time errors on an untyped table too. Only column names and types move to runtime.

## 5. The slow-query warning

Each query on an untyped table is a warning at compile time, at the query's location. It is a new code in the `check` family; `QK-CHECK-003` is free at the time of writing.

```
warning[QK-CHECK-003] (check): this query is on a table whose row type isn't known, so it checks every value as it runs and is much slower; give the table a row struct (t: tbl[Car] = ...) for a fast query, or pass --no-warn to hide warnings at a.qrk:5:8
```

- It is printed by `quark run`, `build`, `check` and `emit`, once per query. It is never printed by the built program, so a program's stderr stays its own.
- `--no-warn`, on every command that compiles, silences every warning, `QK-CHECK-002` included.
- The REPL shows it once per session: at the first input that has a query on an untyped table. This needs the REPL to print the warnings of an input that succeeds, which it doesn't today (`known_issues.md`, "`quark repl` doesn't print the type-change warning"), and to print each warning once.
- A table whose read is typed by §6 gets no warning.

## 6. Typing a read from its file at compile time (wanted, not decided)

This section was §10 of `specs/tables_v0_1_spec.md`. It is wanted; whether it is built is a separate decision.

When the path is a string literal (`t = unwrap(csv::read_file('cars.csv'))`), the compiler reads the file while compiling, infers the row type the way the runtime does (D8, every row scanned), and checks every column read and query against it, like F# type providers. The table is then typed, so its queries are fast and get no warning:

```quark
t = unwrap(csv::read_file('cars.csv'))   // tbl[{model: str, mpg: float, cyl: int}]
print(t.mpg)                             // checked at compile time
t | e -> filter(e.mpgg > 20)             // compile-time error: no column 'mpgg' (columns: model, mpg, cyl)
```

- **At runtime** the read uses the inferred types, as a read typed by a struct does (D18), so a file that changed since compiling is a runtime error naming the line and the column.
- **A path the compiler can't know** (`csv::read_file(sys::args()[0])`) isn't inferred; the table is untyped (§3, §4).
- **A file the compiler can't read** (missing, no permission) is a warning, and the table is untyped.
- **A flag skips inference**, to keep compiles fast; reads are then untyped.
- **A column whose values aren't all one type** (`'123', 'abc', 9.5`) is inferred as `str`, with a note saying why: in a CSV every value is text.
- **`csv::parse_text` of a string literal** is inferred the same way, without a file.

What building it involves:

- **The same inference in two languages.** The compiler is Go and the reader is C++ (`table/csv.hpp`). Either the inference is ported to Go and both run one corpus of files in the tests, or the compiler runs the C++ reader through a helper it builds once. If they disagree, the runtime check catches it, but as a confusing error.
- **Compile time.** Inferring scans the whole file. A schema cache keyed by the file's real path, size and modification time keeps later compiles fast. The REPL needs the cache most, because it analyses every earlier input again for each new one.
- **The data must be there when compiling.** On another machine, or in CI without the file, the read falls back to untyped (with the warning above).
- **Relative paths** are resolved against the compiler's working directory, which is the program's under `quark run`. A built program run from elsewhere reads another file, which the runtime check covers.
- **An inferred row can't be written** in an annotation: `tbl[{model: str, mpg: float}]` is a parse error today. So a function that takes such a table can't name its type (§11, question 3).

## 7. Performance

Measured on 2026-11-17 on 2M rows (columns `a: float`, `b: int`, `s: str` with 51 distinct values), one thread, clang 22 at `-O3`. The checked loop doesn't exist yet. Its stand-in is a Quark `for` loop over the same columns as boxed values (each read through an untyped `fn id(v) -> v`, so every operation is the generic `QValue` one). It builds no output table and handles no missing values, so the real loop may be somewhat slower.

| Query | Typed query | Boxed stand-in | Ratio |
|---|---|---|---|
| `filter(e.a > 0.5 and e.b < 50) \| mutate(c = e.a * 2.0 + e.b)` | 35 ms | 77 ms | 2.2× |
| `filter(e.s == 'k7')` | 98 ms | 260 ms (110 ms of it boxing the strings) | 2.6× |

Once v0.3 runs typed loops on every core, the gap grows by about the thread count, because a checked loop stays on one thread (U9). Column-only steps (§4.1) run at typed speed.

## 8. Changes to earlier specs

When this spec is implemented:

- `specs/tables_v0_1_spec.md` D11 (a query needs a known row struct) and D19 (a bare `tbl` can only be printed, written and passed on) are rewritten to point here.
- `semantics.md` §9.5: the sentence on a bare `tbl` says its columns are checked at runtime and its queries run checked loops.
- `error_codes.md`: the new warning code. `README.md`: `--no-warn`.
- `specs/tables_v0_3_spec.md` §8.1: checked loops run on one thread.

## 9. Tests

- **Differential**: every query test in the suite also runs on an untyped read of the same data, and the output must be identical. The one listed exception is U4's int/float case, which gets its own test.
- **Rejected at runtime** (`TestSemanticsSafety_Rejected` or alongside it): an unknown column, including on an empty table; a wrong operand type at a row; a non-bool `filter` condition; a computed column with mixed types; a computed column with no present values; an aggregate of a column of the wrong type.
- **Still rejected at compile time**: each item of §4.5, on an untyped table.
- **Where the table comes from**: an untyped table from an imported file, from an `extern fn` that returns `tbl`, passed through an untyped function, as a `tbl` parameter, and from `when csv::read_file(p): ok t`.
- **Missing values**: `e.a == null`, `fillna`, three-valued `and`/`or`, and a call with a missing argument, on an untyped table.
- **The warning**: printed once per query by `run`, `build`, `check` and `emit`; not printed for a typed table; not printed with `--no-warn`; printed once per REPL session.

## 10. Plan

After tables v0.3 is implemented: both change the query analysis and lowering (`types/analyzer_query.go`, `lower/query.go`).

1. The REPL prints warnings of inputs that succeed, once each (the known issue). `--no-warn`.
2. Column reads on a bare `tbl` (§3).
3. Column-only steps (§4.1), with the differential test harness.
4. The checked loop (§4.2), computed column types (§4.3), and the warning (§5).
5. §6, if it is decided.

## 11. Open questions

1. **An empty input.** With U4, a computed column on a table with no rows has no values, so `t | e -> filter(e.mpg > 1000) | mutate(x = e.mpg * 2)` fails on an untyped table when no row passes, and works on a typed one. Options: keep the error; take the type of the single column when the expression is just one column; or give an empty computed column a fixed type.
2. **A query on a value of unknown type** (`any`, an unannotated parameter) is a compile-time error today (`a query needs a table whose row type is known`). It could run the checked path after a runtime check that the value is a table, like column reads on such values already do.
3. **Writable anonymous rows** (`t: tbl[{mpg: float, cyl: int}]`), so a function can name an inferred row (§6) or a query's result without a struct.
4. **§6, unreadable file**: a warning and an untyped table (§6), or a compile-time error unless a flag says otherwise.
5. **Help with writing the struct**: `describe()` could end with a struct to paste, and `quark schema <file>` could print one.
6. **Joins** (`specs/data_work_v0_1_spec.md` §5) on an untyped table. A join's condition names only columns, so it could run as a column-only step (§4.1), with the keys found and their types compared when the step starts; the joined columns' names, and any clash between them, would then be checked at runtime too.
