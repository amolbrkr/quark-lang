# Quark benchmarks

A benchmark suite that compares Quark with Python (pure Python, NumPy, pandas, Polars) and R (loops, base vectors, data.frame, data.table) on common data processing and number crunching. The point is to see where Quark actually stands, so that design decisions are made on measurements. It is not meant to make Quark look good: each tool gets the code an experienced user of it would write, and the same work.

The latest measurements and what they show are in [RESULTS.md](RESULTS.md).

## Contents

- [Quick start](#quick-start)
  - [Prerequisites](#prerequisites)
  - [Reproducing the numbers](#reproducing-the-numbers)
- [What is compared](#what-is-compared)
- [Rules every implementation follows](#rules-every-implementation-follows)
- [Benchmarks](#benchmarks)
  - [Plain code: fib, mandelbrot, sieve, ewma](#plain-code)
  - [Columns: reduce, arith, expr, dot, zscore, filter, sort](#columns)
  - [Hashing and strings: groupby, join, wordcount, strings](#hashing-and-strings)
  - [Tables: query, sortkeys, readcsv](#tables)
  - [Linear algebra: gemv, gemm, ols](#linear-algebra)
- [How a run works](#how-a-run-works)
- [Reading the report](#reading-the-report)
- [Caveats](#caveats)
- [Adding a benchmark or an implementation](#adding-a-benchmark-or-an-implementation)
- [Files](#files)

## Quick start

From a fresh clone, on a machine with the prerequisites below:

```bash
cd src/benchmarks
python3 bench.py setup                    # .venv with the pinned Python libraries; installs data.table if missing (~1 min)
python3 bench.py run --preset small       # every implementation once on small inputs (~2-4 min); checks they agree
python3 bench.py run                      # default sizes, one thread per library (~20 min, 740 MB of data)
```

Checked on Linux on 2026-11-13: a fresh clone ran `setup` and the `small` preset with every implementation agreeing. `setup` and `run` print what is available; anything missing is skipped and listed in the report (no OpenBLAS: only `quark_blas`; no R: the R implementations; no data.table: `r_dt`), so a partial setup still runs.

More commands:

```bash
python3 bench.py list                     # benchmarks and their implementations
python3 bench.py run --threads 0          # each library at its default thread count
python3 bench.py run --bench groupby,join --lang quark,python
python3 bench.py run --quark /path/to/quark      # a different compiler build, e.g. another branch
```

### Prerequisites

| | Needed for | Linux (Debian/Ubuntu) | macOS | Windows |
|---|---|---|---|---|
| Go 1.21+, clang++ | Quark | `sudo apt install clang`; Go from [go.dev](https://go.dev/dl/) if the distribution's is older than 1.21 | `brew install llvm go` | Go and LLVM installers; `clang++` on the `PATH` |
| OpenBLAS | `quark_blas` only | `sudo apt install libopenblas-dev` | `brew install openblas`, then `export QUARK_LIB_PATH="$(brew --prefix openblas)/lib" QUARK_INCLUDE_PATH="$(brew --prefix openblas)/include"` | no standard package; `quark_blas` is skipped |
| Python 3.11+, or [uv](https://docs.astral.sh/uv/) | Python implementations, data generation | `sudo apt install python3 python3-venv`, or uv (it downloads Python 3.11 itself) | `brew install python` or uv | python.org installer or uv; run `python bench.py ...` |
| R 4.x | R implementations | `sudo apt install r-base r-base-dev` (data.table compiles from source) | CRAN installer | CRAN installer; `Rscript` is found under Program Files, or set `BENCH_RSCRIPT` |

`setup` creates `.venv/` and installs `requirements.txt` into it (with uv when it is on the `PATH`, else `venv` and pip); it never touches the system Python. If data.table is missing, it installs it from CRAN into the R user library (`R_LIBS_USER`, created if needed); `setup --no-r-packages` skips that.

**Windows is not tested yet.** The driver has the Windows paths (`.venv\Scripts\python.exe`, `.exe` programs, finding `Rscript`), but no run has been made there; peak memory is not measured on Windows (it comes from `wait4`).

### Reproducing the numbers

- **Fixed**: the input data (seeded, the same files on every machine), the code of every implementation, and the Python libraries (`requirements.txt`).
- **Not fixed, but recorded** in the report's Setup table: the Python patch version (uv picks the newest 3.11), R and data.table (current CRAN), the system OpenBLAS, clang and Go, the CPU and the load average.
- Absolute times depend on the machine. Compare implementations within one run; to compare two Quark compilers, use `--quark` on the same machine. In our checks, medians of reruns on a quiet machine agreed within about 5%, except Quark's list implementations, whose garbage collection makes runs vary more; on a shared or virtual machine expect 10% or more (check the load average in the report, and rerun a suspicious benchmark with `--bench`).
- `RESULTS.md` records the machine and versions behind its numbers.

### Output and options

`bench.py run` builds the compiler in `src/core/quark` (unless `--quark` is given), generates the input data once per preset into `data/`, builds the Quark programs into `build/` (one per benchmark, plus `<bench>_blas` for the `std/blas` implementations), runs every implementation, and writes `results/<date>_<preset>_t<threads>/` with `report.md`, `results.json` (environment, configuration, every run) and `times.csv` (one row per timed run). `bench.py report <dir>` rebuilds `report.md` from `results.json`. `data/`, `build/`, `results/` and `.venv/` are not committed.

Options of `run`: `--preset small|default|large`, `--bench`, `--impl`, `--lang` (comma-separated filters), `--threads N` (default 1; 0 = library defaults), `--warmup` (default 1), `--reps` (default 5), `--budget` (seconds per implementation, default 30), `--timeout` (seconds before a process is killed, default 600), `--quark PATH`, `--out DIR`, `--regen` (rewrite the data), `--strict` (exit 1 when an implementation fails or disagrees).

## What is compared

Implementations are grouped in tiers, from plain code to data frames:

| Tier | Quark | Python | R |
|---|---|---|---|
| list: element-at-a-time code with the core types | `quark_list`: `list`, `dict`, loops | `py_list`: `list`, `dict`, loops, builtins | `r_loop`: `for`/`while` over vectors |
| vector: whole-column operations | `quark_vec`: `vec` | `numpy` | `r_vec`: base vectors and matrices |
| vector + BLAS | `quark_blas`: `vec` + `std/blas` | (NumPy calls BLAS itself) | (R calls BLAS itself) |
| table | `quark_tbl`: `tbl` and its query verbs | `pandas`, `polars` | `r_df`: `data.frame`, `r_dt`: `data.table` |

The list tier is named after Quark's `list`, but it is the tier of plain code in general: `fib` and `mandelbrot` use no lists. R has no separate list-of-scalars type, so its plain-code tier is loops over vectors (`r_loop`), and it is included only where loops are the natural or the only way to write the benchmark.

A tool is left out of a benchmark (`n/a` in the report) when it has no natural way to do it, for example `quark_vec` in `sieve` (vectors are immutable) or `numpy` in `fib`. The table tiers appear where the frame API is different code from plain vectors (filter, sort, group, join, adding a column); `sum(df$a)` would only repeat `r_vec`.

`quark_tbl` uses tables v0.1 (`specs/tables_v0_1_spec.md`): a `tbl[Row]` read with `std/csv`, and the query verbs `filter`, `mutate`, `sort`, `group_by` and `summarize`. A table can only be made by reading CSV in v0.1, so its input is a CSV copy of the same data (`datagen.py` writes both; the floats read back to the same doubles), read before timing like every other input. It is `planned` in `join` (there is no join verb yet) and `wordcount` (a table can't be built from a list of words yet).

## Rules every implementation follows

1. **Same data.** `datagen.py` writes every input once per preset (seeded: same files on every machine), and every implementation reads those files. Loading is not timed, except in `readcsv`, where it is the work.
2. **Same result.** Every implementation computes the same result and prints a checksum of it (computed outside the timed region). `bench.py` compares each checksum with a reference implementation (NumPy when it exists, else pure Python, R, Quark) within a relative tolerance: 1e-9 by default (sums of floats in a different order), 1e-6 for `mandelbrot` (Quark's compiler may fuse `a*b + c` into one FMA instruction, which can flip a point at the escape boundary), exact for `fib` and `sieve`. A mismatch is reported as `wrong` and the time is not used.
3. **Same algorithm, idiomatic code.** Each implementation does the same work with the same algorithm (no implementation skips work another does), written the way an experienced user of that tool would: Python uses `sum()`, `Counter`, comprehensions; NumPy uses `np.sort`, boolean masks, `@`; R uses `tapply`, `match`, `%*%`. Where a tool has no way to do something, the implementation says so in a comment and does what a user of that tool would do instead (NumPy's `ewma` loops over the array; Quark's vec `sort` goes through a list).
4. **No accelerators.** No numba, Cython, numexpr, Rcpp, JIT decorators or hand-written native code. `std/blas` is part of Quark's standard library, so `quark_blas` is in.
5. **The result is materialized.** Polars runs eagerly (`DataFrame`, not `LazyFrame`), and every timed function returns its full result (a new column, the filtered rows, the grouped table).
6. **Fresh process per implementation**, so one implementation's heap, caches and thread pools don't affect another's.
7. **One thread by default.** `--threads 1` pins every thread pool: `OMP_NUM_THREADS`, `OPENBLAS_NUM_THREADS`, `MKL_NUM_THREADS`, `POLARS_MAX_THREADS`, `R_DATATABLE_NUM_THREADS` and data.table's `setDTthreads`, and `GC_MARKERS` (Quark's Boehm GC marks in parallel by default). This compares the code, not the core count. `--threads 0` shows what each tool does out of the box; Quark itself is single-threaded except inside OpenBLAS and the GC.
8. **Quark is built the way users build it**: `quark build` with its default flags (`-O3 -march=x86-64-v3` on x86-64, no LTO).

## Benchmarks

Sizes are those of the `default` preset; `small` is about 100 times smaller, `large` 3-10 times larger (`PRESETS` in `bench.py`). Checksums are listed so an implementation can be checked by hand.

### Plain code

#### fib

Recursive Fibonacci, `fib(30)`: the cost of a function call and an integer comparison. Quark's `fib` is fully annotated (`fn fib(n: int) int`), so it compiles to a plain C++ function. Checksum: the result.

#### mandelbrot

Escape-time iteration count over a 400 × 400 grid of `[-2, 1] × [-1.5, 1.5]`, at most 100 iterations per point; tight float loops with an early exit. Checksum: the total iteration count.

- Scalar (`quark_list`, `py_list`, `r_loop`): per point, `while k < iters`, break when `zr² + zi² > 4`.
- Vectorized (`quark_vec`, `numpy`, `r_vec`): all points iterate 100 times; each iteration counts the points with `zr² + zi² <= 4`. An escaped point grows to `inf` and then `nan`, which never counts again, so the total equals the scalar one. Real and imaginary parts are separate float vectors in every implementation (Quark has no complex type). NumPy and R pay for iterating points that have already escaped; that is the usual vectorized formulation.

#### sieve

The number of primes below 10,000,000 by the sieve of Eratosthenes: list index reads and writes in nested loops. Checksum: the count (664,579).

- `quark_list` builds the list with 10M `push(true)` calls (Quark has no `[x] * n`), crosses off with `for j in range(i * i, n, i): s[j] = false`, and counts with a loop.
- `py_list`: `[True] * n`, the same loops, `sum(s)`. `r_loop`: `rep(TRUE, n)` and `while` loops.
- `numpy`: slice assignment `s[i*i::i] = False`. `r_vec`: `s[seq.int(i*i, n - 1, by = i) + 1] <- FALSE`.
- No `quark_vec`: vectors can't be written.

#### ewma

Exponentially weighted moving average of 10M values, a sequential recurrence that can't be vectorized: `y[0] = x[0]`, `y[i] = α·x[i] + (1 − α)·y[i−1]`, α = 0.05. Checksum: `sum(y)`.

- `quark_list`, `py_list`, `r_loop`: a loop (Quark and Python append to a list; R fills a preallocated vector, the R idiom).
- `quark_vec`: the same loop reading `v[i]`, then `.to_vec()`: vectors have no recurrence operation.
- `numpy`: a loop over the array. NumPy has no recurrence primitive; this is what NumPy alone offers.
- `pandas`: `ewm(alpha=α, adjust=False).mean()`; `polars`: `ewm_mean(alpha=α, adjust=False)`; `r_vec`: `stats::filter(α·x, 1 − α, method = "recursive", init = x[1])`. All three are this recurrence in native code.

### Columns

Two columns `a` and `b` of 10M uniform `[0, 1)` doubles (`sort` uses 1M standard normals). `quark_tbl` reads them as a `tbl` of the two columns from `cols.csv` (`reduce` reads only `a`), and `sort`'s from `sort.csv`.

#### reduce

Sum, minimum, maximum and mean of `a`. Checksum: their sum.

- `quark_list`: one loop computing all four (Quark lists have no reductions). `py_list`: `sum`, `min`, `max`, and `sum / len`.
- `quark_vec`: `sum(a)`, `min(a)`, `max(a)`, `s / len(a)`. `numpy`: `.sum()`, `.min()`, `.max()`, `.mean()`.
- `quark_tbl`: one `summarize(s = sum(e.a), lo = min(e.a), hi = max(e.a), m = mean(e.a))`, a table of one row.
- `pandas`: the Series methods (which skip NaN, like Quark's table aggregates); `polars`: one `select` of the four expressions.
- `r_vec`: `sum`, `min`, `max`, `mean`. R's `sum` accumulates in long double and `mean` makes a second pass for accuracy, which costs time. `r_dt`: the same in `dt[, .(...)]`.

#### arith

`z = 2.5·a + b`, a new column. Checksum: `sum(z)`.

- `quark_list`: a loop pushing to a list. `py_list`: `[2.5 * x + y for x, y in zip(a, b)]`.
- `quark_vec`, `numpy`, `r_vec`: `2.5 * a + b` (two passes, one temporary). `quark_blas`: `blas::axpy(2.5, a, b)` (one pass).
- Tables add the column: `mutate(z = 2.5 * e.a + e.b)`, `df.assign(z=...)`, `df.with_columns(z=...)`, `df$z <- ...`, `dt[, z := ...]` (in place).

#### expr

`z = (a·a − b)·0.5 + (a − b) / (b·b + 1)`: eight operations, so vectorized code makes seven temporaries. Checksum: `sum(z)`.

This separates compiled loops (one fused pass) from vector libraries that materialize every intermediate (Quark `vec`, NumPy, pandas, R) and from Polars, which evaluates the expression without full temporaries. Quark's `vec` also checks every divisor for zero. `quark_tbl`'s `mutate(z = ...)` compiles the whole expression into one loop over the two column buffers, so it is a fused pass too.

#### dot

`sum(a·b)`. Checksum: the result.

- `quark_vec`, `r_vec`: `sum(a * b)` (a temporary, then a sum). `quark_blas`: `blas::dot`. `numpy`: `np.dot` (BLAS). `pandas`: `Series.dot`; `polars`: `pl.col('a').dot(pl.col('b'))`.
- `py_list`: `sum(x * y for x, y in zip(a, b))`.

#### zscore

Standardize a column: `z = (a − mean(a)) / sd(a)` with the `n − 1` standard deviation, a new column. Checksum: `sum(z·b)`.

- `quark_list`, `py_list`: three passes (mean, squared deviations, result).
- `quark_vec`: `d = a - m`, `sqrt(sum(d * d) / (n - 1))`, `d / sd`. `quark_blas`: the same with `blas::dot(d, d)` and `blas::scal(1 / sd, d)`.
- `numpy`: `(a - a.mean()) / a.std(ddof=1)`; `r_vec`: `(a - mean(a)) / sd(a)`; tables add `z` as a column.
- `quark_tbl`: the mean and sd from the column as a vec (`stats::mean(t.a)`, then `sum(d * d)`, as in `quark_vec`: there is no sd aggregate), then `mutate(z = (e.a - m) / sd)`. pandas also takes the mean and sd of the column (`a.mean()`, `a.std()`) before `assign`.

#### filter

The rows of `(a, b)` where `a > 0.5` (about half, in random order, so branches don't predict). Both columns are filtered. Checksum: row count + `sum(a)` + `sum(b)` of the kept rows.

- `quark_list`: one loop pushing to two lists. `py_list`: two comprehensions.
- `quark_vec`: `m = a > 0.5`, then `a[m]`, `b[m]`. `numpy`, `r_vec`: the same with boolean indexing.
- `quark_tbl`: `filter(e.a > 0.5)`; `pandas`: `df[df['a'] > 0.5]`; `polars`: `df.filter(...)`; `r_df`: `df[df$a > 0.5, ]`; `r_dt`: `dt[a > 0.5]`.

#### sort

Sort 1M standard normal doubles ascending, into a new column (each run sorts the unsorted input). Checksum: `sum(sorted[i] · i) / n` (0-based `i`, so a wrong order changes it).

- `quark_list`: `xs.slice(0, n).sort()` (a copy, then Quark's stable sort). `quark_vec`: `v.to_list().sort().to_vec()`, because vectors have no sort.
- `py_list`: `sorted(xs)` (Timsort). `numpy`: `np.sort` (introsort with SIMD kernels). `r_vec`: `sort(x)` (radix sort for doubles).
- Tables sort a one-column table: `sort(e.x)` (the engine's fast path for one numeric key without missing values), `sort_values`, `df.sort('x')`, `df[order(df$x), , drop = FALSE]`, `dt[order(x)]` (data.table's radix order).

### Hashing and strings

#### groupby

Sum and count of a value per string key: 1M rows, 1,000 distinct keys (`id000`-`id999`, uniform), values uniform `[0, 100)`. Checksum: `Σ sum_k · count_k / rows` + number of groups.

- `quark_list`: two `dict`s and `get`/`set` per row. `py_list`: two `defaultdict`s.
- `numpy`: `np.unique(keys, return_inverse=True)` (sorts the keys) and `np.bincount` with weights: NumPy's way to group.
- `quark_tbl`: `group_by(e.k) | summarize(s = sum(e.v), n = count())`, on the `k` and `v` columns of `rows.csv` (the same values as `grp_k.txt` and `grp_v.f64`).
- `pandas`: `groupby('k')['v'].agg(['sum', 'count'])`; `polars`: `group_by('k').agg(sum, len)`.
- `r_vec`: `tapply(v, k, sum)` and `tapply(v, k, length)`; `r_df`: `aggregate(v ~ k, ...)`; `r_dt`: `dt[, .(s = sum(v), n = .N), by = k]`.
- No `quark_vec`: vectors have no grouping.

#### join

Left join of 1M `(k, v)` rows to a 100,000-row `(k, w)` table on a string key, giving the joined `w` column (every key is found). Checksum: `sum(v · w)`.

The pandas, Polars, data.frame and data.table implementations are real data frame joins. Quark's tables have no join verb yet, so its only implementation is the one a Quark user can write today, a `dict` and lists, like the pure Python one. `quark_tbl` comes with the join verb, which is planned after tables v0.1 (`specs/tables_v0_1_spec.md` §9).

- `quark_list`: a `dict` from the second table, then a lookup per row. Its time is all runtime code (about 70% the hash lookups, 25% building the result list, 10% building the dict), so it doesn't change with the back end. A key missing from the dict is a runtime error (`w: float = lookup.get(k)` is checked), like Python's `KeyError`; the data has none. `py_list`: `dict(zip(...))` and a comprehension.
- `numpy`: no hash join: sort the dimension keys, `np.searchsorted`, and mark keys that don't match (NaN), so it is a real left join.
- `pandas`: `merge(how='left')`; `polars`: `join(how='left')`; `r_vec`: `w[match(k, dim_k)]`; `r_df`: `merge(all.x = TRUE)` (sorts by key); `r_dt`: `dim[fact, on = 'k']`.

#### wordcount

Split a 1M-word text (words separated by single spaces, a Zipf-distributed vocabulary of 20,000 words) and count each word. Checksum: distinct words + `Σ count² / words`.

- `quark_list`: `text.split(' ')` and a `dict`. `py_list`: `Counter(text.split(' '))`.
- `numpy`: `np.unique(np.array(text.split(' ')), return_counts=True)`; `pandas`: `pd.Series(text.split(' ')).value_counts()`; `polars`: the split, `explode` and `value_counts` all in Polars.
- `r_vec`: `table(strsplit(text, ' ', fixed = TRUE)[[1]])`; `r_dt`: the words in a data.table, `[, .N, by = w]`.
- `quark_tbl` is planned: a table can't be built from the list `split` returns until table literals come (`specs/tables_v0_1_spec.md` D19).

#### strings

Clean 1M labels such as `"  ITEM_4821_Lorem "`: trim, lowercase, replace `_` with `-`. Checksum: the number of results starting with `item-` + their total length.

- `quark_list`: `s.trim().lower().replace('_', '-')` per string. `py_list`: `s.strip().lower().replace('_', '-')`.
- `numpy`: `np.strings.strip`, `lower`, `replace` over a `StringDType` array (NumPy 2's string ufuncs). `pandas`: the `.str` methods (pyarrow strings, pandas 3's default). `polars`: `str.strip_chars`, `to_lowercase`, `replace_all(literal=True)`.
- `r_vec`: `gsub('_', '-', tolower(trimws(s)), fixed = TRUE)`.
- `quark_tbl`: `mutate(s = e.s.trim().lower().replace('_', '-'))`, the same methods called once per row, on the labels of `strings.csv` (quoted, so the reader keeps their padding).
- No `quark_vec`: `vec[str]` has no string operations.

### Tables

Work on a table of 1M rows, `rows.csv`: `id` (int, 0 to n − 1), `k` (str, the 1,000 keys of `groupby`), `g` (int, uniform 0-9), `v` (float, the values of `groupby`, uniform `[0, 100)`) and `w` (float, uniform `[0, 1)`). The Python and R table implementations load it with their CSV readers (`read_csv`, `read.csv`, `fread`), the list implementations into one list per column; only `readcsv` times the read.

#### query

One query of five steps: keep the rows with `v >= 10` (about 90%), add `x = v · w`, group by the two keys `(k, g)` (10,000 groups), compute the row count, `sum(x)`, `mean(w)` and `max(v)` per group, and sort the groups by `sum(x)`, descending. Checksum: groups + `Σ n` + `Σ s / 1000` + `Σ m` + `Σ hi / 100`, plus the `s`-weighted mean position `Σ s_i · i / Σ s` (0-based `i`), which a wrong order changes.

- `quark_tbl`: the query as written in one chain:
  ```quark
  t | e ->
      filter(e.v >= 10.0)
      | mutate(x = e.v * e.w)
      | group_by(e.k, e.g)
      | summarize(n = count(), s = sum(e.x), m = mean(e.w), hi = max(e.v))
      | sort(desc(e.s))
  ```
- `quark_list`: one loop over the rows with a `dict` from the key pair to the group's position (the two keys joined into one string, since dict keys are strings) and one list per aggregate, then `sort_by` on the positions. `py_list`: a `dict` keyed by the `(k, g)` tuple, then `sorted`.
- `pandas`: boolean indexing, `assign`, `groupby(['k', 'g']).agg(n=..., s=..., m=..., hi=...)`, `sort_values`; `polars`: `filter`, `with_columns`, `group_by('k', 'g').agg(...)`, `sort`; `r_dt`: `dt[v >= 10]`, `[, x := v * w]`, `[, .(...), by = .(k, g)][order(-s)]`.
- No base R `data.frame` implementation: `aggregate` with a different function per column is not how anyone writes this in base R.

#### sortkeys

Sort the `(k, v)` columns of the table by `k` ascending, then `v` descending: a two-key sort with a string key, where the second key decides within each of the 1,000 groups of equal keys. Checksum: `Σ v_i · i / n` over the sorted rows (0-based `i`).

- `quark_tbl`: `sort(e.k, desc(e.v))` (stable, the engine's general multi-key sort).
- `quark_list`: lists sort by one key, so `sort_by` the row positions by `-v`, then stably by `k`, and gather both columns. `py_list`: `sorted(range(n), key=lambda i: (k[i], -v[i]))` and the same gather.
- `numpy`: `np.lexsort((-v, k))` and fancy indexing. `pandas`: `sort_values(['k', 'v'], ascending=[True, False])`; `polars`: `sort(['k', 'v'], descending=[False, True])`.
- `r_df`: `df[order(df$k, -df$v), ]` (base R's `order` compares strings with the locale's collation, which orders these keys like byte order); `r_dt`: `dt[order(k, -v)]` (data.table's radix order, byte order).

#### readcsv

Read `rows.csv` (1M rows, 52 MB) into a table with the column types given: int, str, int, float, float. The file is in the OS page cache after the warmup run, so this measures parsing and building the columns, not the disk. Checksum: rows + `Σ id / rows` + `Σ g` + `Σ v` + `Σ w` + the total length of the `k` strings.

- `quark_tbl`: `t: tbl[Row] = unwrap(csv::read_file(path))`, typed by the row struct (`std/csv`'s in-house reader, which also checks every str cell is valid UTF-8).
- `py_list`: `csv.reader`, converting each field with `int` or `float` into one list per column.
- `pandas`: `read_csv` with `dtype` (its C parser); `polars`: `read_csv` with `schema`; `r_df`: `read.csv` with `colClasses`; `r_dt`: `fread` with `colClasses`.
- No `quark_list` (lists have no CSV reader since `std/csv` returns tables) and no `numpy` (no reader for mixed column types).

### Linear algebra

Matrices are stored row-major in the data files. Quark has no matrix type: `quark_list` uses a list of row lists, `quark_vec` a list of row (or column) vectors, `quark_blas` one flat vector with explicit dimensions (`std/blas`). NumPy uses `ndarray`, R a `matrix` (converted from row-major on load).

#### gemv

`y = A·x` for a 2,000 × 2,000 matrix. Checksum: `sum(y)`.

- `quark_list`: nested loops. `quark_vec`: `sum(row * x)` per row. `quark_blas`: `blas::gemv(a, n, n, x)`.
- `py_list`: a generator per row. `numpy`: `a @ x`. `r_vec`: `a %*% x`.

#### gemm

`C = A·B` for 300 × 300 matrices. Checksum: `Σ C[i][j] · (i + 1) / n` (detects a transposed result).

- `quark_list`, `py_list`: i-k-j loops (row `i` of `C` accumulates `A[i][k] · B[k]`). `quark_vec`: the same with vector rows (`ci = ci + bk * aik`). `quark_blas`: `blas::gemm(a, n, n, b, n)`.
- `numpy`: `a @ b`; `r_vec`: `a %*% b`. Both call the BLAS `dgemm`, like `quark_blas`.

The size keeps pure Python at a few seconds; BLAS's advantage grows with `n` (`--preset large` uses 1,000).

#### ols

Least squares by the normal equations, `β = solve(XᵀX, Xᵀy)`, for `X` with 500,000 rows and 8 columns. Checksum: `Σ β_j · (j + 1)`.

- `quark_list`, `py_list`: accumulate the upper triangle of `XᵀX` and `Xᵀy` row by row. `quark_vec`: `X` as 8 column vectors; each entry is `sum(ci * cj)`. `quark_blas`: `blas::syrk(x, n, p)` for `XᵀX` and `blas::gemv(x, n, p, y, true)` for `Xᵀy`.
- `numpy`: `np.linalg.solve(x.T @ x, x.T @ y)` (NumPy computes `x.T @ x` with `syrk`). `r_vec`: `solve(crossprod(x), crossprod(x, y))` (also `syrk`).
- The 8 × 8 solve is negligible; Quark and pure Python use the same Gaussian elimination with partial pivoting (`quark/lib/matrix.qrk`, `python/impls.py`), NumPy and R use LAPACK. R's idiomatic `lm()` is left out because it uses QR, a different algorithm.

## How a run works

For each benchmark and implementation, `bench.py` starts one process:

```
build/quark/<bench>[_blas]  bench=<b> impl=<id> data=<dir> warmup=1 reps=5 budget=30 [params]
.venv/bin/python python/run.py  bench=... (same arguments)
Rscript --no-init-file r/run.R  bench=... (same arguments)
```

The process loads its data, runs the work `warmup` times untimed and then up to `reps` times timed, and stops starting new runs once `budget` seconds have been spent (warmup included, always at least one timed run), so slow implementations finish in reasonable time. Each timed run is measured inside the process with a monotonic clock (`time::clock()` in Quark, `time.perf_counter()` in Python, `Sys.time()` in R, which has microsecond resolution), so process start, compilation and loading are excluded. The process prints, tab-separated:

```
TIME   <bench> <impl> <rep> <seconds>      one line per timed run
CHECK  <bench> <impl> <checksum>           once, after the last run
```

`bench.py` also records the process's peak resident memory (`wait4`), kills it after `--timeout` seconds, and keeps its stderr for the report when it fails. The R front end's warning about a mismatched `R_HOME` (which R ignores) is avoided by unsetting `R_HOME` for R processes.

### Checking the timing

The Quark harness (`quark/lib/harness.qrk`) was checked on 2026-11-13:
- **The clock** is `time::clock()`, `std::chrono::steady_clock` (monotonic, nanosecond resolution). Measuring an empty closure costs about 50 ns, against 1.5 ms for the fastest benchmark.
- **Nothing is optimized away or moved.** The timed work is an indirect call through a closure, which the C++ compiler can't see into, so it can't be moved past the clock reads or computed once for all runs; its result is kept and checksummed, so skipped work would fail the check. Hand-written C++ shows the danger: `fib(30)` timed in a plain loop measured 0 ms until its argument was read through a `volatile`.
- **The in-process times agree with the process's wall clock**: the extra wall time of a process doing 101 timed runs over one doing 1, divided by 100, matched the in-process median within about 1% for Quark (`arith` with `vec`, 46.3 vs 46.8 ms) and NumPy (30.2 vs 30.5 ms), and within 0.5% for Quark's `fib`, `dot` and `groupby` (20 runs).

**One file per language doesn't change the times.** All the Python implementations live in `python/impls.py` and all the R ones in `r/impls.R`, while Quark has one program per benchmark (each Quark program is a separately compiled binary). Parsing and loading happen before the first timed run in every language, like Quark's compile step: Python compiles `impls.py` to bytecode once on import, and a function's speed doesn't depend on how many other functions the module defines (names are looked up in a hash table); pandas, Polars and NumPy are imported inside the implementations that use them, so a NumPy run never loads Polars. R `source()`s `impls.R` before timing and compiles each function to bytecode on its first call, which the warmup run absorbs. Measured on 2026-11-13, each implementation run from a file of its own against the suite's combined file (3 rounds of 7 runs, medians): Python `fib(30)` 266.8 against 265.8 ms, Python `groupby` 388.9 against 392.4 ms, NumPy `arith` 30.2 against 29.6 ms, R `fib(30)` 1632 against 1599 ms. R `groupby` measured 206.5 ms through the suite's harness with the full 429-line `impls.R` and 208.7 ms with a 45-line one holding only that benchmark (its runs range from about 175 to 230 ms either way).

**Times are steady-state.** The median leaves out the first timed run's extra cost, but a script that runs a step once pays it. In Quark the first timed run (after one warmup) is typically 10-25% slower than the later ones, up to 2.6 times, because the GC heap is still growing; in Python and R it is 1-5%. A garbage collection is charged to the run that triggers it, which is why Quark's list tier varies by up to 3 times between runs.

## Reading the report

`report.md` has:
- **Setup**: commit, CPU, memory, threads, tool and library versions, the BLAS each tool uses, and the load average at the start and end (another busy process on the machine shows up there).
- **Summary**: the fastest Quark, Python and R implementation per benchmark, and Quark's best time divided by Python's and R's best (below 1: Quark is faster).
- **All times**: the median of every implementation; the fastest in each row is bold.
- **Details**: per benchmark, median, min and max of the timed runs, the number of runs, the ratio to the fastest, peak memory and the checksum check.
- **Failures**: implementations that errored, timed out, disagreed with the reference or were skipped, with the end of their stderr.

Compare tiers as well as languages: `quark_list` against `py_list` and `r_loop` shows what compiling plain code buys, `quark_vec` against NumPy and R vectors shows the vector runtime, `quark_blas` against `quark_vec` what `std/blas` adds, and the table tiers are the reference for `quark_tbl`.

## Caveats

- **Peak memory includes the runtime and the input.** Measured baselines: 15 MB for a Quark program or a Python process without NumPy, about 25 MB with NumPy, 70-80 MB with Polars, 110-130 MB with pandas, 70 MB for R. Compare memory within a benchmark, not across benchmarks.
- **BLAS builds differ.** Quark's `std/blas` and R link the system OpenBLAS; NumPy wheels bundle their own, usually newer, OpenBLAS (both versions are in the report). With one thread this matters only for `gemm`, `gemv` and `ols`.
- **Virtual machines and shared hosts are noisy.** Check the load average in the report; rerun a benchmark with `--bench` when a number looks off. The min column is the least noisy estimate; the median is what a typical run costs, including garbage collection.
- **Default sizes fit in memory with room to spare.** Data larger than RAM and reading from disk are not measured: `readcsv` reads a file the OS has cached, so it measures parsing.
- **Python's version matters for `py_list`**: CPython 3.11+ is much faster than 3.10 on plain loops. The report records the version.

## Adding a benchmark or an implementation

A benchmark:
1. If it needs new input, generate it in `datagen.py` and bump `VERSION` (cached data is regenerated when it changes).
2. Add a `Bench` entry to `BENCHMARKS` in `bench.py`: name, group, one-line title, parameters from the preset sizes, implementations, planned implementations, and a tolerance if the default 1e-9 doesn't fit.
3. Implement it in `quark/<name>.qrk` (one program for the Quark implementations, dispatching on `impl`; a `quark_blas` implementation goes in `quark/<name>_blas.qrk`, so that a machine without OpenBLAS skips only it), `python/impls.py` (`@impl(bench, id)`) and `r/impls.R` (`impl(bench, id, fn)`). Each implementation loads its data, calls `measure` with the work, and returns or prints the checksum computed from the result.
4. Document it in [Benchmarks](#benchmarks): what it computes, the checksum, and what each implementation does differently.
5. `python3 bench.py run --preset small --bench <name>` must show every implementation agreeing.

A `quark_tbl` implementation is an `elseif impl == 'quark_tbl':` branch in `quark/<name>.qrk` that reads its table with a loader of `quark/lib/tables.qrk` (a typed `csv::read_file` of a CSV file `datagen.py` writes; add the row struct and the file there if they are new). The two still `planned`, `join` and `wordcount`, wait for a join verb and table literals.

## Files

| Path | Contents |
|---|---|
| `RESULTS.md` | The latest measured results, with what limits Quark and where |
| `bench.py` | The driver: presets, the list of benchmarks and implementations, data generation, builds, runs, checks, report (Python standard library only) |
| `datagen.py` | Writes the input files, binary and CSV (needs NumPy; run in `.venv`) |
| `requirements.txt` | Pinned Python libraries |
| `quark/<bench>.qrk` | The Quark implementations of one benchmark, except `quark_blas` |
| `quark/<bench>_blas.qrk` | The `quark_blas` implementation (`std/blas` needs OpenBLAS at build time) |
| `quark/lib/harness.qrk` | Arguments, timing loop, TIME/CHECK output, data loading |
| `quark/lib/bench_io.hpp` | Native loaders for the data files (`extern fn`) |
| `quark/lib/matrix.qrk` | Matrix setup helpers and the small linear solver |
| `quark/lib/tables.qrk` | The row structs and typed CSV loaders of the `quark_tbl` implementations |
| `python/run.py`, `python/impls.py` | Python harness and implementations |
| `r/run.R`, `r/impls.R` | R harness and implementations |
