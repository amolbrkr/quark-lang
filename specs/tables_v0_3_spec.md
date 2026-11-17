# Quark Tables v0.3 Spec

Status: In progress. Implemented on one thread: aggregates in `filter` and `mutate` (including grouped steps and aggregates of expressions), `summarize` expressions, computed group keys, `sd`, and prelude `mean`/`sd` with `std/stats` removed; row-safe calls in a verb (`extern rowsafe fn`, the user-function check) and a call that doesn't read the row computed once per step. Not yet: missing values in verbs (`== null`, `fillna`, `drop_na`), `contains`, `nrow`/`ncol`/`names`, the `count` step, two-argument `min`/`max` typing, random draws in verbs (a draw is row-safe and is not computed once, but it still comes from the sequential generator), and parallel execution. A whole-table float aggregate of an expression is computed by materializing the expression as a column and then using the column kernel (same bits; the spec's blockwise path with no temporary column is not done). Tables v0.1 and v0.2 are the language this adds to (`specs/tables_v0_1_spec.md`, `specs/tables_v0_2_spec.md`); §9 lists the earlier decisions this one changes, and §10 the programs that stop working or print something different.
Last Updated: 2026-11-17
Related: `specs/tables_v0_1_spec.md` (D1–D25), `specs/tables_v0_2_spec.md` (D26–D34), `specs/ffi_v0_1_spec.md` (native functions), `architecture.md` §15 and `src/benchmarks/RESULTS.md` (the measurements).

## Contents

1. [Purpose](#1-purpose)
2. [Decisions](#2-decisions)
3. [Aggregates in every verb](#3-aggregates-in-every-verb)
4. [Calls in verbs](#4-calls-in-verbs)
5. [Random numbers in verbs](#5-random-numbers-in-verbs)
6. [Missing values in verbs](#6-missing-values-in-verbs)
7. [Other additions](#7-other-additions)
8. [Parallel execution](#8-parallel-execution)
9. [Changes to earlier specs](#9-changes-to-earlier-specs)
10. [Breaking changes](#10-breaking-changes)
11. [Tests](#11-tests)
12. [Plan](#12-plan)
13. [Later](#13-later)

## 1. Purpose

Quark's query model is row-first, as in SQL: inside a verb, `c.mpg` is one row's value, and an expression is compiled into one typed loop over the column buffers. That loop is why a query is fast (the `expr` benchmark: 42 ms as a `mutate`, 197 ms as the same expression on vecs, which writes a temporary column per operator). v0.3 keeps the model and fills the gaps it leaves:

- A value of the whole column, or of the row's group, can't be used in a row: `filter(c.mpg > mean(c.mpg))` and `group_by(c.cyl) | mutate(dev = c.mpg - mean(c.mpg))` are compile errors, and the workaround of calling `stats::mean(t.mpg)` inside a verb recomputes it on every row.
- A query can't test or replace a missing value, can't test membership, and can't count rows.
- Every verb runs on one thread, while Polars and data.table use every core by default.

The additions keep the loop: an aggregate is computed once before it, a call that doesn't read the row is computed once, and the loop itself is split across threads. Functions called from a verb must be safe to run that way (§4), which the compiler checks.

```quark
struct Car:
    name: str
    mpg: float
    cyl: int
    hp: int

cars: tbl[Car] = unwrap(csv::read_file('cars.csv'))

above = cars | c -> filter(c.mpg > mean(c.mpg))
z = cars | c -> mutate(z = (c.mpg - mean(c.mpg)) / sd(c.mpg))
dev = cars | c -> group_by(c.cyl) | mutate(dev = c.mpg - mean(c.mpg), share = c.hp / sum(c.hp))
stats = cars | c ->
    group_by(c.cyl, heavy = c.hp > 150)
    | summarize(n = count(), spread = max(c.mpg) - min(c.mpg), fast = mean(c.hp > 200))
known = cars | c -> filter(c.mpg != null) | count(c.cyl)
```

## 2. Decisions

The v0.1 and v0.2 decisions hold except where §9 says otherwise. These are the new ones.

| # | Decision |
|---|---|
| D35 | **Aggregates work in `filter` and `mutate`, not only in `summarize`.** `count()`, `count(x)`, `sum`, `mean`, `sd`, `min`, `max` and `n_distinct` are computed over the step's input table, or over the row's group when the step follows `group_by` (D36), once, before the row loop; every row of the table or group sees the same value. The missing-value rules are `summarize`'s (D6): `sum` and `count` of no values are 0, the others are missing, and a missing aggregate makes the expression part that contains it missing (D22). |
| D36 | **`group_by` applies to the next step only**, which is `filter`, `mutate` or `summarize`. A grouped `filter` or `mutate` keeps the input's rows in their order, and its output is not grouped, so there is no `ungroup()`. `group_by` followed by another step, or ending a query, is a compile-time error. `group_by(c.cyl) \| filter(...) \| summarize(...)` summarizes the whole table; to summarize per group, write `group_by` again (documented; decided 2026-11-17). |
| D37 | **An aggregate takes an expression of the row**, not only a column: `sum(c.price * c.qty)`, `mean(c.mpg > 25)` (the share of rows). A row whose expression is missing is skipped, as a missing cell is. An aggregate inside another aggregate is a compile-time error. A whole-table aggregate of an expression is computed without a temporary column. |
| D38 | **A `summarize` column is an expression of aggregates and group keys**: `spread = max(c.mpg) - min(c.mpg)`, `label = c.dept + ':'` (a key), `round(mean(c.x))`. A reference to the row outside an aggregate, other than a group key, is a compile-time error. |
| D39 | **A group key may be computed, and is then named**: `group_by(c.dept, year = time::year(c.d))`. A computed key is a column of the `summarize` or `count` output under its name; a grouped `filter` or `mutate` does not add it. A bare column key keeps its name, as now. |
| D40 | **`sd` is an aggregate, `mean` and `sd` are prelude builtins, and `std/stats` is removed.** `sd` is the sample standard deviation (divisor n − 1). Outside queries, `mean(v)` and `sd(v)` take a numeric or bool vec, like `sum`, with the same bits as the aggregate (D13). `mean` and `sd` become builtin names, which a program can't redefine; a column or struct field can still be named `mean`, as one can be named `min` today. |
| D41 | **A call in a verb must be row-safe**: it can run for any row, in any order, on several threads at once, and gives the same result for the same arguments. A call that isn't is a compile-time error that names it, and for a user function, the call inside it that makes it unsafe. |
| D42 | **The compiler works out whether a user `fn` is row-safe** from its body and the functions it calls (§4.3): only row-safe calls, no call through a function value, no assignment to a variable from outside its body, and no change to a list or dict. Reading outside variables is allowed. |
| D43 | **`extern rowsafe fn` marks a native function as row-safe.** The compiler can't check the promise. An `extern fn` without `rowsafe` can't be called in a verb. The standard library's bindings use the same qualifier. |
| D44 | **A row-safe call whose arguments don't read the row runs at most once per step**, the first time a row needs it, and its result is used for every row. If it fails, the error is reported at the first row, in row order, that needs it, so the step behaves as if it ran per row. |
| D45 | **A random draw in a verb is computed from the seed, the step, the row and the draw's position in the row**, with a counter-based generator (Philox4x32-10), so a seeded program gets the same numbers for every thread count. Uniform floats, ints and choices are also the same on every platform. Outside verbs, `std/random` works as now. |
| D46 | **`x == null` and `x != null` in a verb test whether `x` is missing.** The result is a bool and is never missing. Any other comparison with `null` in a verb is a compile-time error. |
| D47 | **`x.fillna(v)` works on any verb expression, and `drop_na` is a step.** `fillna` returns `v` where `x` is missing; `v` is evaluated only for those rows. `drop_na(c.a, c.b)` keeps the rows where every listed column is present; `drop_na()` uses every column. |
| D48 | **Lists and vecs have `contains`.** `xs.contains(x)` is whether some element equals `x`. In a verb, a list or vec that doesn't read the row is turned into a hash set once per step, so `filter(keep.contains(c.name))` and `filter(other.id.contains(c.id))` cost one lookup per row. |
| D49 | **A table has `nrow()`, `ncol()` and `names()`.** `len(t)` stays a compile-time error, and names `t.nrow()`. |
| D50 | **`count(keys)` is a step**: `count(c.cyl)` is `group_by(c.cyl) \| summarize(n = count())`. Keys follow D39; `count()` is one row. |
| D51 | **Two-argument `min` and `max` are typed by their arguments**: two values of one type give that type. An `int` and a `float` stay untyped, because a wide int that wins is returned as an int (decided 2026-11-16); in a verb, they are a compile-time error that says to convert one with `to_float`. |
| D52 | **One thread pool runs the query loops, the table engine, the CSV reader and the vec operations.** Everything else, the REPL included, stays on one thread. |
| D53 | **Every program prints the same output for every thread count**: the same rows in the same order, the same float bits, the same errors (§8.4). |
| D54 | **The default thread count is the number of logical CPUs the process may use**, after its CPU affinity and its container's CPU quota. `quark run --threads N` and the `QUARK_THREADS` variable set it; `--threads 1` runs every step on one thread, as today. `quark repl` always uses one thread. |
| D55 | **Work is split into blocks of 65,536 rows** (or vec elements), and an input of fewer than 131,072 rows runs on the calling thread. |
| D56 | **A runtime error in parallel code is the one a single thread reports**: in the first row loop that fails (a step runs its loops one after another), the first failing row, in row order. Worker threads never end the program; the main thread reports the error. |
| D57 | **`--checked` also runs every parallel step on one thread and compares the results.** A difference is an internal error (`INV-PAR-001`). |

## 3. Aggregates in every verb

### 3.1 Where an aggregate can appear

An aggregate is written as in `summarize` (v0.1 §4) and is syntax, not a function call. It can appear in a `filter` condition, a `mutate` column and a `summarize` column. The aggregate names are `count`, `sum`, `mean`, `sd`, `min`, `max` and `n_distinct`:

- `count()` is the number of rows of the table or group. `count(x)`, `sum(x)`, `mean(x)`, `sd(x)`, `min(x)`, `max(x)` and `n_distinct(x)` are aggregates when `x` reads the row (D37).
- `sum`, `mean`, `sd`, `min` and `max` with one argument that doesn't read the row are the prelude functions (`sum(t.hp)`, `max(limits)`), and D44 computes them once.
- Two-argument `min` and `max` are the prelude functions, run per row: `max(c.a, c.b)` (§7.4).

Types and empty results follow v0.1 §4 and D29. `sd` is a float; of fewer than two present values it is missing. Strings and dates have `min`, `max`, `count` and `n_distinct`, and no `sum`, `mean` or `sd`; a bool's `mean` is the share of true values and its `sd` treats true as 1.

In a `mutate`, a column may use an aggregate of a column written earlier in the same `mutate` (D23): `mutate(a = c.x * 2, share = c.a / sum(c.a))` behaves as `mutate(a = c.x * 2) | mutate(share = c.a / sum(c.a))`.

Aggregates do not appear in `sort`, `head`, `tail`, `select`, `rename`, `distinct` or `drop_na`.

### 3.2 Grouping applies to the next step

`group_by` modifies the step right after it (D36):

```quark
cars | c -> group_by(c.cyl) | mutate(dev = c.mpg - mean(c.mpg))       // a mean per cyl
cars | c -> group_by(c.cyl) | filter(c.hp == max(c.hp))                // the most powerful car of each cyl
cars | c -> group_by(c.cyl) | filter(count() >= 5) | summarize(n = count())   // one row: the whole table
```

A grouped `filter` or `mutate` outputs the rows in input order, with the input's columns (and a `mutate`'s new ones). A row whose key is missing is in the group of missing keys, as in `summarize` (D6). Nothing stays grouped after the step, so the third query's `summarize` is over the whole table; `group_by(c.cyl)` before it would make it per group. In a grouped step, each aggregate is the value of the row's group, and the key columns are ordinary columns of the row.

### 3.3 Aggregates of expressions

The argument of an aggregate is a verb expression, with the usual rules (§4, §6): `sum(c.price * c.qty)`, `mean(c.x > 0)`, `max(c.name.lower())`, `n_distinct(time::year(c.d))`. For each row the expression is computed when the columns it reads are present; a row where it is missing is skipped by the aggregate. `mean(sum(c.x))` and `sum(c.x - mean(c.x))` are compile-time errors (`an aggregate can't contain another aggregate`).

A whole-table float aggregate of an expression gives the same bits as the same aggregate over a column holding the expression's values: the expression is computed in blocks along the pairwise split (D13), with no temporary column.

### 3.4 Summarize columns

A `summarize` column is any verb expression in which every reference to the row is inside an aggregate or is a group key:

```quark
staff | e ->
    group_by(e.dept)
    | summarize(label = e.dept + ' team', cv = sd(e.salary) / mean(e.salary), top = round(max(e.salary)))
```

`summarize(x = e.salary)` without an aggregate is a compile-time error: `'e.salary' has a value per row; use it in an aggregate (mean(e.salary)) or group by it`. The column types follow the expression rules; a column that could be missing (any aggregate except `count`, `sum` and `n_distinct`, and expressions of them) is missing for a group where its aggregate is.

### 3.5 Computed group keys

A key of `group_by` or `count` is a column, `c.col`, or `name = expr`, where `expr` is a verb expression whose type is a column type (`int`, `float`, `bool`, `str`, `date`):

```quark
sales | s -> group_by(year = time::year(s.day), s.region) | summarize(total = sum(s.amount))
```

Groups are in key order (D9), comparing a computed key like a column of its type; a missing key value is its own group. The `summarize` or `count` output has the keys first, in `group_by` order, under their names. Two keys of the same name, or a key and an output column of the same name, are a compile-time error. A computed key needs a name: `group_by(time::year(s.day))` is a compile-time error that suggests `group_by(year = time::year(s.day))`.

### 3.6 `sd`, and `mean` and `sd` outside queries

`sd(x)` is the sample standard deviation of the present values (NaN skipped), with divisor n − 1. It is computed in two passes, each with the pairwise sum: the mean, then the sum of squared deviations from it, divided by n − 1, then the square root. The table aggregate and the prelude function share the kernel, so `sd(t.x)` and `summarize(s = sd(e.x))` give the same bits, as `sum` and `mean` do (D13).

`mean(v: vec) float` and `sd(v: vec) float` are prelude builtins for numeric and bool vecs, with `sum`'s rules (missing entries and NaN skipped). A vec with no present values is a runtime error for `mean`, and one with fewer than two is for `sd`, as for `min` and `max` (D13): a value taken out of a vec must be present. `mean(v)` gives the bits `stats::mean(v)` gives today. `std/stats`, whose only function was `mean`, is removed.

### 3.7 How aggregates run

Lowering computes a step's aggregates before its row loop:

1. The grouping: the whole table, or the `group_by` keys (`group_rows`, `table/engine.hpp`), once per step.
2. Each distinct aggregate once, with the kernels `summarize` uses, giving one value per group. The same aggregate written twice in a step is computed once. A `mutate` already runs one loop per column; an aggregate of a column written earlier in the same `mutate` is computed after that column's loop.
3. The row loop, which reads the value of the row's group.

The kernels meet v0.2 §4's requirements, including the whole-table `min` and `max` loop of v0.2 §4.1, which `agg_extremum` does not have (`known_issues.md`): `filter(c.x == max(c.x))` must not be slower than `max(t.x)` plus a `filter`.

## 4. Calls in verbs

### 4.1 Row-safe calls

v0.3 runs a verb's rows on several threads (§8), in blocks whose order isn't fixed, and computes a call that doesn't read the row once (D44). A call is **row-safe** when that can't change anything a program observes: it can run for any row, in any order, on several threads at once, and gives the same result for the same arguments. A verb expression may call only row-safe functions (D41).

Its other rules don't change: a column cell is a value of its column's type, so passing one where a vec is expected is a type error, which in a verb gets a message that points to the aggregate:

```
error[QK-CHECK-001] (check): 'c.mpg' is one row's float here; for the mean of the column, write mean(c.mpg)
```

and the value of a `mutate` column or a `filter` condition has a column type or bool, as now.

A call that isn't row-safe is a compile-time error naming the call, and for a user function the path to it:

```
error[QK-CHECK-001] (check): 'band' can't be called in a query: it calls 'print' (line 3)
error[QK-CHECK-001] (check): 'score' can't be called in a query: it calls 'band' (line 8), which calls 'print' (line 3)
error[QK-CHECK-001] (check): 'lib::norm' can't be called in a query: it calls the native function 'c_norm', which isn't declared rowsafe
```

### 4.2 Builtins and the standard library

Each builtin's catalog entry (`builtins/catalog.go`) says whether it is row-safe, and whether it changes its receiver. A new builtin is not row-safe until its entry says so.

- Not row-safe: `print`, `input`, the table method `describe` (it prints), and every method that changes its list or dict: `push`, `pop`, `set`, `insert`, `remove`, `reverse`, `sort` and `sort_by` on lists (they work in place) and `set` on dicts.
- Every other builtin is row-safe.

The standard library's `extern fn` bindings carry `rowsafe` (D43) where it applies, and its module functions are checked like user functions (§4.3):

| Module | Row-safe | Not row-safe |
|---|---|---|
| `std/time` | every calendar, formatting, parsing and duration function | `now`, `clock`, `today`, `sleep`, `since` |
| `std/random` | every draw: `uniform`, `normal`, `randint`, `choice`, `weighted`, `sample`, `uniform_vec`, `normal_vec`, `randint_vec` (§5) | `seed`, `shuffle` |
| `std/fmt` | every function (each returns a string) | |
| `std/sys`, `std/io`, `std/csv`, `std/blas` | | every function |

`time::format_time(..., local: true)` reads the local zone, which can't change while a step runs.

### 4.3 User functions

A `fn` declaration is row-safe when, in its body and in the bodies of the `fn` declarations it calls:

- every call is row-safe: a row-safe builtin, a `rowsafe` native function, a row-safe standard library function, or a row-safe `fn` declaration;
- no call is through a function value: a parameter, a variable holding a function, or a lambda;
- no assignment is to a variable declared outside the body (a top-level variable, or one a closure captures);
- no list or dict is changed: no element assignment (`xs[i] = v`, `d.key = v`) and no changing method (§4.2), even on a list the function made itself.

Reading outside variables is allowed: nothing can write them while a step runs.

```quark
threshold = 25.0
fn band(x: float) str -> 'high' if x > threshold else 'low'     // row-safe
fn noisy(x: float) float -> x + random::normal(0.0, 0.1)          // row-safe (§5)
log = list []
fn traced(x: float) float ->                                      // not row-safe: it changes a list
    log.push(x)
    x
```

The analyzer already resolves every call to its target (`CallPlan`) and every name to its variable (`Symbol`), so whether an assignment's target is outside the body is known. Row-safety is computed for all declarations at once, the way `finalizeFixedFns` (`types/analyzer_fixed.go`) works out which declarations need a closure: a declaration is unsafe when its body has an unsafe call or construct, or calls a declaration that is unsafe, repeated until nothing changes. Recursive and mutually recursive declarations with no unsafe call are row-safe. The rule is conservative on purpose (§13 lists relaxations).

### 4.4 Native functions: `extern rowsafe fn`

```quark
extern rowsafe fn c_norm(x: float, y: float) float as 'geo_norm'
```

`rowsafe` goes between `extern` and `fn`, on functions and on extern methods. It is the native author's promise that the function meets §4.1: it may be called from several threads at once (no shared mutable state, such as a `static` buffer or a global handle used without a lock) and gives the same result for the same arguments. The compiler can't see into native code, so it can't check the promise. If it is false, a parallel step can give wrong values or crash, like any other broken FFI contract; `--threads 1` runs the program on one thread, and `--checked` compares each parallel step with a one-thread run (D57). `specs/ffi_v0_1_spec.md` documents the qualifier with the other native-code contracts. An `extern fn` without `rowsafe` called in a verb is a compile-time error that names the qualifier.

### 4.5 Calls that don't read the row

A row-safe call whose arguments don't read the row gives every row the same value, so it is computed at most once per step (D44), the first time a row needs it. `filter(c.mpg > mean(t.mpg))` computes the mean once; in v0.2, `stats::mean(t.mpg)` in a verb ran on every row (`known_issues.md`). A call that no row needs is not made: one inside the false branch of every row's ternary, inside an `and` whose left side is false for every row, or in a step whose input has no rows. When the call fails, the error is reported at the first row, in row order, that needs it (§8.5). Random draws (§5) are not computed once: each row draws its own numbers.

## 5. Random numbers in verbs

`std/random` has one generator, a sequence that each draw advances. In a parallel step, rows on different threads would advance it at once (a data race), and with a lock, which row got which number would depend on the threads' timing. In a verb, a draw is computed instead of taken from the sequence (D45):

- Each row loop the program runs (an aggregate pass, a `filter`'s condition, a `mutate` column) gets the next number of a loop counter, in program order, whatever the thread count and whether or not the loop is large enough to use the pool. `random::seed(n)` sets the seed and resets the loop counter to 0; a program that never calls `seed` uses the seed it takes from the system at startup.
- A row numbers its draws 0, 1, 2, ... in evaluation order (`semantics.md` §4.5), including draws in the functions it calls.
- A draw's bits are Philox4x32-10 of the key (seed, loop number) and the counter (row index, draw number). The functions turn bits into values with algorithms the runtime defines (53 bits for a uniform float, Box–Muller for a normal, rejection for an int range), not with `std::` distributions, whose algorithms differ between C++ libraries. Uniform floats, ints, `choice`, `weighted` and `sample` are then the same on every platform; a normal uses the platform's `log` and `cos`, which can differ in the last bit between platforms, so normals are the same for every thread count on one platform.

The runtime sets the row's context (loop number, row index, draw number) in thread-local state before each row; a draw function reads it, and takes the sequential generator when there is none, so the same function works in and outside verbs. The numbers in a verb don't come from the sequence that draws outside verbs use, and a step doesn't advance that sequence: `x = random::uniform()`, a query that draws, then `y = random::uniform()` gives the same `x` and `y` as without the query.

`random::seed` and `random::shuffle` can't be called in a verb (§4.2).

## 6. Missing values in verbs

In a verb, a part that reads a missing value is missing without being computed (v0.1 §5), so a comparison with a missing operand is missing. That makes `filter(c.mpg == null)` keep no rows, with no error (`known_issues.md`). v0.3 adds the operations that look at the missing value itself:

- **`x == null`, `null == x`, `x != null`** (D46), for any verb expression `x`: true or false whether `x` is missing; the result is never missing. Outside verbs, `==` with `null` keeps its meaning. `x < null` and the other ordered comparisons with `null` are compile-time errors in a verb.
- **`x.fillna(v)`** (D47): `x` where it is present, else `v`. `v` is a verb expression of `x`'s type (an int widens to float), checked as the vec method's value is (`fillna() can't fill a float with str`). It is computed only for rows where `x` is missing, so `c.a.fillna(c.b)` takes `b` where `a` is missing, and the result is missing only where both are.
- **`drop_na(c.a, c.b)`** (D47): a step that keeps the rows where every listed column is present; `drop_na()` uses every column. A float NaN is present (as for the vec method `drop_na`). Its arguments are columns, as `distinct`'s are.

```quark
cars | c -> filter(c.mpg == null) | select(c.name)          // the cars with no mpg
cars | c -> mutate(mpg = c.mpg.fillna(mean(c.mpg)))        // fill with the column's mean
cars | c -> drop_na(c.mpg, c.hp) | summarize(r = mean(c.mpg / c.hp))
```

## 7. Other additions

### 7.1 `contains`

`xs.contains(x) bool` on a `list` and a `vec`: whether some element equals `x` under `==`. A missing vec entry and a NaN are never equal to anything. When the element type and `x`'s type are known, a comparison that can't be true (a `list[str]` and an `int`) is a compile-time error. `str.contains(sub)` stays the substring test.

In a verb, `xs.contains(x)` where `xs` doesn't read the row (an outside list or vec, or a column of another table) is computed by building a hash set of `xs` once per step (D44) and looking `x` up per row. The set holds int, float, bool, str or date elements; other element types are a compile-time error in a verb. A missing `x` makes the result missing, so `filter` drops the row, in `filter(!other.id.contains(c.id))` too.

```quark
keep = list ['eng', 'ops']
staff | e -> filter(keep.contains(e.dept))
orders | o -> filter(customers.id.contains(o.customer_id))     // the orders with a known customer
```

### 7.2 `nrow`, `ncol`, `names`

`t.nrow() int`, `t.ncol() int`, and `t.names() list` (the column names as a `list` of `str`, in column order). They are table methods (D25). `len(t)` stays a compile-time error, with a message naming `t.nrow()`.

### 7.3 The `count` step

`count(k1, k2, ...)` is `group_by(k1, k2, ...) | summarize(n = count())`: one row per key combination, in key order, with the keys and an `int` column `n`. Keys follow §3.5. `count()` has one row with the row count. A key named `n` is a compile-time error that suggests renaming it. As a step, `count` is a verb; inside a verb expression, `count()` is the aggregate.

### 7.4 Two-argument `min` and `max`

The prelude `min(a, b)` and `max(a, b)` are typed by their arguments everywhere (D51): two ints give an int, two floats a float, two strs a str, two dates a date. An int and a float give an untyped value, because a wide int that wins comes back as an int; in a verb, where a column needs a type, that is a compile-time error suggesting `to_float`. One-argument `min` and `max` on a vec are unchanged.

## 8. Parallel execution

### 8.1 What runs in parallel

One runtime thread pool (D52) runs:

- **Query row loops**: `filter`, `mutate`, `drop_na`, the aggregate passes (§3.7), and the per-row lookups of §7.1.
- **The table engine**: grouping (`group_rows` hashes the keys in parallel), the aggregate kernels, `sort` (a stable parallel merge sort), `distinct`, and building output columns.
- **The CSV reader**: the text is split at line ends outside quotes and the parts are parsed in parallel.
- **Vec operations**: arithmetic, comparisons, reductions (`sum`, `mean`, `sd`, `min`, `max`, `all`, `any`), masks, `fillna`, `drop_na` and `astype`.

The pool has two primitives: a loop over blocks of rows or elements, and a reduction that follows the pairwise split (§8.4). Lists, dicts, strings and all other code stay on one thread. `std/blas` keeps OpenBLAS's own threads and is not row-safe, so it never runs inside a parallel step. Vec expressions are still computed one operator at a time; compiling a vec expression into one loop (fusion) is separate work outside the tables specs (§13), and the pool's block loop is what it would run on.

### 8.2 Thread count

The default is the number of logical CPUs the process may use (D54):

- Linux: the CPUs in the process's affinity mask (`sched_getaffinity`), capped by the container's CPU quota (cgroup v2 `cpu.max`, or v1 `cpu.cfs_quota_us` / `cpu.cfs_period_us`) rounded up. A machine whose `nproc` is 16 under a 6-CPU quota gets 6: measured on 2026-11-17 in a test program, 16 threads under that quota were throttled and ran a vec-style expression 1.7 to 2.3 times slower than 6.
- macOS: the logical CPUs online.
- Windows: the logical processors in the process's affinity mask.
- At least 1. Hyperthreads count: on a machine with 8 cores and 2 threads per core the default is 16. Two threads per logical CPU gained nothing in the measurements (122 ms at 16 and at 32 threads on a compute-heavy loop).

`quark run --threads N` sets it for the program (N ≥ 1). A built program reads `QUARK_THREADS` at startup, which `quark run --threads` sets; a value that isn't a positive integer is an error at startup, not ignored. `--threads` wins over `QUARK_THREADS`, which wins over the default. `sys::threads() int` returns the count in use, next to `sys::cpu_count()`. `--threads 1` runs every step on the calling thread and starts no pool; its output is the output of every other count (D53). `quark repl` uses one thread (D52) and has no `--threads`.

### 8.3 Small inputs

The pool splits a loop into blocks of 65,536 rows or elements (D55). A step or vec operation with fewer than 131,072 rows or elements runs on the calling thread without waking the pool. Measured on 2026-11-16 with a test pool: waking 6 threads costs about 40 µs, a light expression over 16K rows takes 24 µs on one thread and 43 µs on six, and over 64K rows 97 µs and 47 µs. The CSV reader decides by the file's size, with a threshold measured when it is implemented. The block size and the thresholds are internal constants, retuned from measurements and recorded in `architecture.md` §15; a program can't set them, and they never change a result (§8.4).

### 8.4 The same output for every thread count

Floating-point addition depends on order (`(0.1 + 0.2) + 0.3` is `0.6000000000000001`, `0.1 + (0.2 + 0.3)` is `0.6`), so a sum split one chunk per thread gives a different answer for each thread count; measured on 2026-11-16, 8 thread counts gave 8 answers on a 10M-value column. Every parallel operation is specified so that its result doesn't depend on the thread count or the blocks (D53):

| Operation | Rule |
|---|---|
| `filter`, `drop_na`, vec masks | Each block keeps its rows in order; the blocks are joined in order |
| `mutate`, vec arithmetic and comparisons | Each output row is written at its input position |
| `sum`, `mean`, `sd` (float) | Split along the pairwise sum's halving (D13), which depends only on the number of values: threads take whole branches of it. Measured: bit-identical to one thread at every count |
| `sum` (int and bool) | Exact (128-bit), so order doesn't matter |
| `min`, `max` | On a tie the lowest row wins, as with one thread |
| `count`, `n_distinct` | Integer counts |
| `group_by`, `summarize`, `count` step | Groups are in key order (D9) |
| `distinct` | Each key keeps its lowest row (D28) |
| `sort` | Stable, so the output is unique |
| CSV reader | Rows in file order; the reported problem is the first in file order, with the total count of problems |
| Random draws in verbs | §5 |
| Runtime errors | §8.5 |

This is a guarantee to users (`semantics.md` states it once implemented): a result computed on a laptop reproduces on a server with another core count, and `--threads 1` is a faithful way to debug.

### 8.5 Errors

A runtime error in parallel code is the error the one-thread run reports (D56). A step runs its row loops one after another: an aggregate pass, a `filter`'s condition, and one loop per `mutate` column, in the order written (`queryMutate`, `lower/query.go`). A loop finishes before the next starts, so the error is the one in the first loop that fails, and within that loop the first failing row in row order (the first failing element for a vec operation) and, within the row, the first failure in evaluation order. In a loop, the blocks after the failing one are cancelled; the blocks before it finish, since one of them could fail at an earlier row. A worker never prints or exits: `q_runtime_reportf` on a worker records the message and the row and ends the worker's block, and the main thread reports the lowest row's error and exits as now.

### 8.6 Runtime requirements

- The pool is created the first time a loop is large enough, and its threads are registered with the garbage collector, so code on a worker can allocate (a `mutate` building strings) and is scanned by collections.
- Each worker has the stack-overflow guard (`core/stack.hpp`) for its own stack, and the step's source location (`q_runtime_loc` is `thread_local`).
- The generated code and the runtime keep no other per-program state that a worker could write; a new runtime global that a row-safe builtin writes is a bug.

### 8.7 Checked mode

With `--checked` (D57), each parallel step and vec operation also runs on one thread, and the two results are compared (values, missing entries, order, error). A difference is an internal error, `INV-PAR-001`, naming the step: it means a broken `rowsafe` promise or a bug in the engine. Checked mode is already slower; this doubles the work of the parallel parts.

## 9. Changes to earlier specs

When v0.3 is implemented, these parts of the earlier specs are rewritten to the new rules (the earlier specs describe the language until then):

- v0.1 D21 ("each call runs once per row, in row order"): calls must be row-safe (D41), and run in no fixed order, possibly on several threads, and once per step when they don't read the row (D44).
- v0.1 D24 ("a summarize column is one aggregate of one column"): D37 and D38.
- v0.1 §4: "group_by keys are columns" (D39), aggregates only in `summarize` (D35), and the rule that `group_by` is followed by `summarize` (D36).
- v0.1 D13: the mean is the prelude `mean` (D40), not `stats::mean`.
- v0.1 §5: `== null`, `fillna` and `drop_na` in verbs (D46, D47).

## 10. Breaking changes

A program that worked before fails to compile, fails at runtime or prints something different in these cases:

- `incl 'std/stats'` fails (the module is gone), and `stats::mean(v)` is written `mean(v)`.
- A variable or function named `mean` or `sd` is a compile-time error (`'mean' is a builtin function and can't be redefined or assigned`).
- A verb that calls a function that isn't row-safe is a compile-time error: `print` or `input` through a helper, `std/io`, `std/sys`, `std/csv`, `std/blas`, `time::now` and the other clocks, `random::seed` and `random::shuffle`, an `extern fn` without `rowsafe`, a function value, or a helper that assigns an outside variable or changes a list or dict.
- A random draw in a verb gives different numbers than today for a seeded program (§5).
- `filter(c.x == null)` keeps the rows where `x` is missing (it kept none), and `mutate(m = c.x == null)` is `true` there (it was missing).
- A variable whose first value is a two-argument `min` or `max` of two same-typed values now has that type, so a later value of another type is a compile-time error (it was untyped).

## 11. Tests

Language tests extend `TestTables_*` (`integration_tables_test.go`), and each feature gets the misuse cases of the Robustness Bar (rule 2) as negative tests: a wrong type, a wrong number of arguments, `null`, an untyped (`any`) value, default parameters, a value from another module, and a call into native code.

- **Aggregates**: each aggregate in `filter` and `mutate`, whole-table and grouped; an empty table; a group whose values are all missing; NaN; an int `sum` that overflows (a runtime error before any row error); a `mutate` whose aggregate reads a column written earlier in it; an aggregate inside an aggregate; an expression aggregate against the same aggregate over a column of its values (same bits); `summarize` expressions; a non-key row reference in `summarize`; computed keys (missing values, key order, name clashes, a missing name); `count` steps; `group_by` followed by a step other than `filter`, `mutate` or `summarize`; `sd` of 0, 1 and 2 present values; `mean(v)` and `sd(v)` against the aggregates; `filter(c.x == max(c.x))` timed against `max(t.x)`.
- **Row-safety**: each unsafe builtin and module function, called directly in a verb, through one helper, through two, through a module function and through a mutually recursive pair; a helper that reads an outside variable (allowed); one that assigns one, pushes to a list it made, or calls a lambda; an `extern fn` with and without `rowsafe`, from another module; `rowsafe` on an extern method; a helper with default parameters; the error text for each.
- **Calls that don't read the row**: computed once (a test glue library whose `rowsafe` function counts its calls); not made when no row needs it (in a never-taken branch, on an empty table); an error from it reported at the first row that needs it.
- **Random**: a seeded program prints the same output at 1, 2, 3, 7 and 16 threads; two draws in one row differ; a draw in an aggregate pass and one in the row loop differ; a query between two draws outside verbs leaves the second unchanged; `seed` and `shuffle` in a verb are compile-time errors; uniform floats and ints are the same on Linux, macOS and Windows (fixed expected output).
- **Missing values**: `== null`, `null ==`, `!= null` on columns, expressions and aggregates; `<` with `null`; `fillna` widening, a type mismatch, a fill value that would fail on present rows (not computed), and `c.a.fillna(c.b)` where both are missing; `drop_na` with and without columns, and NaN kept.
- **Other additions**: `contains` on lists and vecs of each element type, missing and NaN entries, an impossible comparison, in a verb against an outside list and another table's column, negated with a missing value; `nrow`, `ncol`, `names`, and the `len(t)` message; two-argument `min`/`max` typing in and outside verbs.
- **Parallel execution**: every operation of §8.1 at 1, 2, 3, 7 and 16 threads, on inputs of 131,071, 131,072 and 131,073 rows and on ones that end mid-block, gives identical output (printed values and float bits); two failing rows in different blocks report the first; CSV problems in several parts report the first and the right total; `QUARK_THREADS` of `0`, `-1` and `x` fail at startup; `--threads` wins over `QUARK_THREADS`; `sys::threads()`; a `mutate` that builds strings on many threads with collections running (the `integration_gc_test.go` pattern); a stack overflow in a row function on a worker; `--checked` catching a deliberately broken `rowsafe` glue function (`INV-PAR-001`); the CPU-quota default with a cgroup limit, where the test machine has one.
- **Benchmarks**: `bench.py` passes `--threads` to Quark through `QUARK_THREADS`; the suite is rerun at 1 thread and at the default, and `RESULTS.md` and `architecture.md` §15 are updated.

## 12. Plan

Each phase lands with its tests and docs (`semantics.md` §9.5, `stdlib.md`, `grammar.md` §10.5, `README.md`, `error_codes.md`, the changelog), and removes the `known_issues.md` entries it fixes: `filter(e.mpg == null)` keeping no rows, two-argument `min`/`max` in queries, the whole-table `min`/`max` slow path, and calls that don't read the row running on every row.

1. **Language, one thread**: aggregates in `filter` and `mutate`, grouped steps, aggregates of expressions, `summarize` expressions, computed keys, `sd`, the prelude `mean` and `sd` and removing `std/stats`, the whole-table `min`/`max` loop, missing values in verbs, `contains`, `nrow`/`ncol`/`names`, the `count` step, two-argument `min`/`max` typing.
2. **Row-safety**: the catalog flags, `extern rowsafe fn` and the standard library's bindings, the user-function check, calls computed once per step. Compile-time only, so it lands before threads, and every program that compiles under it is ready for them.
3. **Random draws in verbs**: the loop counter, the row context, Philox, the value algorithms.
4. **The thread pool**: thread count, `--threads`, `QUARK_THREADS`, `sys::threads()`, garbage-collector registration, the error path, the stack guard; then the query loops, the engine, the vec operations and the CSV reader, each against the §8.4 rules; then checked mode's comparison.
5. **Measurements**: the benchmark suite at 1 thread and at the default.

## 13. Later

Specified elsewhere or left for later:

- **The column-wise table literal** (D19, `specs/tables_v0_2_spec.md` §6.1): its syntax is being reconsidered, so it is not part of v0.3.
- **Joins**, of every kind, in their own spec (v0.2 §6.2 lists the questions). `contains` (§7.1) covers semi- and anti-joins on one key until then.
- **Window functions**: `lag`, `lead`, `cumsum`, `rank`, `row_number`. They depend on row order, and a float `cumsum` across threads needs its own rule for the same output at every thread count.
- **`median`, `quantile`** and `summary(t)` (v0.1 §6), which need a quantile kernel.
- **Per-group `head` and `tail`** (`group_by(c.cyl) | head(2)`), under D36's rule.
- **Vec expression fusion**: compiling `(a * a - b) * 0.5 + ...` on vecs into one loop, as `mutate` is; with §8 it would be split across threads. It is outside the tables specs.
- **Column types of untyped helpers**: `fn twice(x) -> x * 2` in a `mutate` needs a return annotation today; the call site's argument types could give it one.
- **Row types in the REPL**: a table read in an earlier input could keep the row type it has at runtime, so `t = unwrap(csv::read_file(p))` can be queried.
- **Relaxing row-safety**: changing lists a function made itself, and calling lambdas it defines.
- **Threads in the REPL**, and `std/blas` using Quark's thread count instead of OpenBLAS's.
- **`var`**, and other aggregates.
