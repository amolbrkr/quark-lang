# Benchmark results

Measured on 2026-11-14 at commit `5bae826` (tables v0.1) plus the change that added the `quark_tbl` implementations and the `query`, `sortkeys` and `readcsv` benchmarks, with the `default` preset (`README.md` describes every benchmark and the rules). To measure again: `python3 bench.py run` (one thread per library) and `python3 bench.py run --threads 0` (library defaults); replace this file's tables with the new reports when the numbers change materially.

## Contents

- [Headlines](#headlines)
- [Setup](#setup)
- [One thread per library](#one-thread-per-library)
- [Library default threads](#library-default-threads)
- [Tables against data frames](#tables-against-data-frames)
- [What std/blas adds](#what-stdblas-adds)
- [Hand-written C++ for reference](#hand-written-c-for-reference)
- [What limits Quark today](#what-limits-quark-today)
- [Where a function call's time goes](#where-a-function-calls-time-goes)

## Headlines

- **A table query is as fast as the best data frame libraries, single-threaded.** The five-step `query` (filter, mutate, group by two keys, four aggregates, sort) on 1M rows takes 134 ms with Quark's `tbl`, against 142 ms for data.table, 167 ms for Polars and 198 ms for pandas; `groupby` takes 52 ms (Polars 55 ms, pandas and data.table 59 ms). A `mutate` compiles its whole expression into one loop, so `expr` (eight operations on 10M rows) takes 59 ms, the fastest of every tool (Polars 93 ms, NumPy 111 ms).
- **Some table operations are still slow**, each for a cause in the engine: sorting by a string key and a float (`sortkeys`, 4.4 times data.table), sorting one float column (6.5 times NumPy, 2.2 times Polars), `filter` (4.1 times Polars), whole-table `min`/`max` (`reduce`, 9.5 times Quark's own `vec` code) and reading CSV (2.3 times pandas). The details are [below](#tables).
- **Plain compiled code is where Quark wins most.** Recursive `fib` runs 25 times faster than CPython 3.11 and 150 times faster than R. The scalar Mandelbrot loop (14 ms) beats NumPy's vectorized version (102 ms) by 7 times and pure Python by 72 times. Logic that can't be vectorized (recursion, early exits, state carried from row to row) is a strong case.
- **`std/blas` puts Quark on par with NumPy and R for BLAS-shaped work**: `gemv` is as fast as NumPy, `dot`, `gemm` and `ols` within 10-20% of it, and `gemm` within 10% of R's `%*%` on the same OpenBLAS build. Over plain `vec` code it is 4 times faster for `dot`, 19 times for `gemv`, 83 times for `gemm`, 3.2 times for `ols`.
- **`vec` arithmetic and reductions are close to NumPy** (reductions 0.86 times NumPy's time, `a*b + c` 1.5 times, a longer expression 1.8 times) and 1.7-4.4 times faster than R vectors.
- **Building lists, hashing in dicts and strings are no faster than CPython.** Quark's `list` code that builds a list of 10M floats is only 2.2 times faster than the same Python; a dict-based group-by is 1.2 times faster than CPython (the table does it 6 times faster); string cleaning is 1.2-1.3 times *slower* than CPython, in a list or a table. The causes are in the runtime's data structures (below), not in the compiler.
- With every library at its default thread count (16 here), Polars and data.table pull 2-12 times ahead of Quark on table work (`query`: Polars 52 ms, data.table 67 ms, Quark 133 ms), while Quark, its tables included, stays single-threaded. Linking OpenBLAS with its default thread pool also makes the GC print warnings and slows `vec` allocation (below).

Quark's runtime is a safety-first design: overflow-checked integers, checked element types, a null check on every vector element, a zero check on every divisor, UTF-8 validation of every string a CSV file brings in. These benchmarks measure that cost along with everything else; where Quark is behind, the notes say whether the cause is a check or a data structure.

## Setup

| | |
|---|---|
| Commit | 5bae826 (uncommitted changes: the table benchmarks) |
| Preset | default |
| Threads | 1 |
| Runs | 1 warmup + up to 5 timed, 30.0 s budget per implementation |
| CPU | AMD EPYC 7571 (16 logical CPUs, amazon, docker) |
| Memory | 124.8 GB |
| OS | Linux 6.12.113 |
| Load average | [1.47, 1.06, 0.88] at start, [1.62, 1.68, 1.36] at end |
| Quark toolchain | clang version 22.1.0 (https://github.com/llvm/llvm-project 4434dabb69916856b824f68a64b029c67175e532); go version go1.26.0 linux/amd64 |
| System OpenBLAS (Quark std/blas) | OpenBLAS 0.3.20 NO_LAPACKE DYNAMIC_ARCH NO_AFFINITY Zen MAX_THREADS=64 |
| Python | 3.11.18: numpy 2.4.6, pandas 3.1.1, polars 2.1.0, pyarrow 26.0.1 |
| NumPy BLAS | scipy-openblas 0.3.31.188.0 |
| R | R version 4.4.3 (2025-02-28), data.table 1.18.4 |
| R BLAS | /usr/lib/x86_64-linux-gnu/openblas-pthread/libblas.so.3 |

Sizes: `vec_n=10,000,000`, `sort_n=1,000,000`, `grp_n=1,000,000`, `grp_k=1,000`, `join_n=1,000,000`, `join_k=100,000`, `words_n=1,000,000`, `vocab=20,000`, `str_n=1,000,000`, `fib_n=30`, `sieve_n=10,000,000`, `mandel_n=400`, `mandel_iters=100`, `gemv_n=2,000`, `gemm_n=300`, `ols_n=500,000`, `ols_p=8`.

The machine had other sessions open but was mostly idle (load average 1-1.7 on 16 vCPUs). Variance was low (spread under 10%) except for Quark's list tier, where garbage collection makes runs vary by up to 3 times, and the operations that write a new 10M-element column, where one of the five runs is often about 50 ms slower in Quark, Polars and R alike; the medians are used throughout.

## One thread per library

The fastest implementation in each language, and how many times slower Quark's best is (below 1 means Quark is faster).

| Benchmark | Best Quark | Best Python | Best R | Quark / Python | Quark / R |
|---|---|---|---|---|---|
| [fib](README.md#fib) | 10.7 ms (Quark list) | 264 ms (Python) | 1.59 s (R loop) | 0.04 | 0.01 |
| [mandelbrot](README.md#mandelbrot) | 13.8 ms (Quark list) | 102 ms (NumPy) | 274 ms (R vector) | 0.14 | 0.05 |
| [sieve](README.md#sieve) | 624 ms (Quark list) | 54.2 ms (NumPy) | 455 ms (R vector) | 11.52 | 1.37 |
| [ewma](README.md#ewma) | 463 ms (Quark vec) | 133 ms (pandas) | 632 ms (R vector) | 3.49 | 0.73 |
| [reduce](README.md#reduce) | 18.4 ms (Quark vec) | 20.2 ms (Polars) | 81.4 ms (R vector) | 0.91 | 0.23 |
| [arith](README.md#arith) | 29.7 ms (Quark vec+blas) | 23.6 ms (Polars) | 85.6 ms (R data.table) | 1.26 | 0.35 |
| [expr](README.md#expr) | 58.8 ms (Quark tbl) | 92.8 ms (Polars) | 326 ms (R vector) | 0.63 | 0.18 |
| [dot](README.md#dot) | 7.63 ms (Quark vec+blas) | 6.84 ms (NumPy) | 99.5 ms (R vector) | 1.12 | 0.08 |
| [zscore](README.md#zscore) | 59.1 ms (Quark vec+blas) | 44.3 ms (Polars) | 206 ms (R data.frame) | 1.34 | 0.29 |
| [filter](README.md#filter) | 99.4 ms (Quark tbl) | 24.2 ms (Polars) | 261 ms (R data.table) | 4.11 | 0.38 |
| [sort](README.md#sort) | 173 ms (Quark tbl) | 26.5 ms (NumPy) | 148 ms (R vector) | 6.52 | 1.17 |
| [groupby](README.md#groupby) | 52.3 ms (Quark tbl) | 54.6 ms (Polars) | 59.5 ms (R data.table) | 0.96 | 0.88 |
| [join](README.md#join) | 304 ms (Quark list) | 133 ms (Polars) | 88.7 ms (R vector) | 2.28 | 3.43 |
| [wordcount](README.md#wordcount) | 271 ms (Quark list) | 119 ms (Polars) | 200 ms (R data.table) | 2.27 | 1.35 |
| [strings](README.md#strings) | 491 ms (Quark tbl) | 192 ms (pandas) | 3.17 s (R vector) | 2.56 | 0.15 |
| [query](README.md#query) | 134 ms (Quark tbl) | 167 ms (Polars) | 142 ms (R data.table) | 0.80 | 0.94 |
| [sortkeys](README.md#sortkeys) | 1.13 s (Quark tbl) | 364 ms (pandas) | 255 ms (R data.table) | 3.11 | 4.43 |
| [readcsv](README.md#readcsv) | 513 ms (Quark tbl) | 226 ms (pandas) | 301 ms (R data.table) | 2.27 | 1.70 |
| [gemv](README.md#gemv) | 1.55 ms (Quark vec+blas) | 1.55 ms (NumPy) | 4.31 ms (R vector) | 1.00 | 0.36 |
| [gemm](README.md#gemm) | 4.13 ms (Quark vec+blas) | 3.53 ms (NumPy) | 3.77 ms (R vector) | 1.17 | 1.09 |
| [ols](README.md#ols) | 17.4 ms (Quark vec+blas) | 14.4 ms (NumPy) | 20.3 ms (R vector) | 1.21 | 0.86 |

Median time per implementation. **Bold** is the fastest in the row; `n/a` means the tool has no natural way to do it, `planned` an implementation still to be written.

| Benchmark | Quark list | Quark vec | Quark vec+blas | Quark tbl | Python | NumPy | pandas | Polars | R loop | R vector | R data.frame | R data.table |
|---|---|---|---|---|---|---|---|---|---|---|---|---|
| fib | **10.7 ms** | n/a | n/a | n/a | 264 ms | n/a | n/a | n/a | 1.59 s | n/a | n/a | n/a |
| mandelbrot | **13.8 ms** | 230 ms | n/a | n/a | 989 ms | 102 ms | n/a | n/a | 730 ms | 274 ms | n/a | n/a |
| sieve | 624 ms | n/a | n/a | n/a | 1.49 s | **54.2 ms** | n/a | n/a | 2.67 s | 455 ms | n/a | n/a |
| ewma | 504 ms | 463 ms | n/a | n/a | 1.39 s | 6.39 s | **133 ms** | 185 ms | 1.27 s | 632 ms | n/a | n/a |
| reduce | 46.3 ms | **18.4 ms** | n/a | 175 ms | 714 ms | 21.3 ms | 65.1 ms | 20.2 ms | n/a | 81.4 ms | n/a | 83.3 ms |
| arith | 605 ms | 44.8 ms | 29.7 ms | 64.8 ms | 1.32 s | 29.3 ms | 41.9 ms | **23.6 ms** | n/a | 87.8 ms | 87.2 ms | 85.6 ms |
| expr | 646 ms | 197 ms | n/a | **58.8 ms** | 2.66 s | 111 ms | 168 ms | 92.8 ms | n/a | 326 ms | n/a | 330 ms |
| dot | 116 ms | 30.5 ms | 7.63 ms | n/a | 958 ms | **6.84 ms** | 7.98 ms | 20.7 ms | n/a | 99.5 ms | n/a | n/a |
| zscore | 497 ms | 75.6 ms | 59.1 ms | 75.4 ms | 2.45 s | 72.7 ms | 137 ms | **44.3 ms** | n/a | 206 ms | 206 ms | 206 ms |
| filter | 671 ms | 185 ms | n/a | 99.4 ms | 1.05 s | 178 ms | 113 ms | **24.2 ms** | n/a | 336 ms | 842 ms | 261 ms |
| sort | 278 ms | 282 ms | n/a | 173 ms | 396 ms | **26.5 ms** | 140 ms | 80.4 ms | n/a | 148 ms | 198 ms | 178 ms |
| groupby | 326 ms | n/a | n/a | **52.3 ms** | 393 ms | 370 ms | 59.5 ms | 54.6 ms | n/a | 181 ms | 612 ms | 59.5 ms |
| join | 304 ms | n/a | n/a | planned | 367 ms | 449 ms | 302 ms | 133 ms | n/a | **88.7 ms** | 14.8 s | 372 ms |
| wordcount | 271 ms | n/a | n/a | planned | 252 ms | 544 ms | 307 ms | **119 ms** | n/a | 286 ms | n/a | 200 ms |
| strings | 509 ms | n/a | n/a | 491 ms | 399 ms | 1.12 s | **192 ms** | 251 ms | n/a | 3.17 s | n/a | n/a |
| query | 470 ms | n/a | n/a | **134 ms** | 737 ms | n/a | 198 ms | 167 ms | n/a | n/a | n/a | 142 ms |
| sortkeys | 3.80 s | n/a | n/a | 1.13 s | 4.61 s | 696 ms | 364 ms | 879 ms | n/a | n/a | 7.54 s | **255 ms** |
| readcsv | n/a | n/a | n/a | 513 ms | 3.40 s | n/a | **226 ms** | 357 ms | n/a | n/a | 1.77 s | 301 ms |
| gemv | 47.2 ms | 28.7 ms | **1.55 ms** | n/a | 365 ms | 1.55 ms | n/a | n/a | n/a | 4.31 ms | n/a | n/a |
| gemm | 374 ms | 341 ms | 4.13 ms | n/a | 2.54 s | **3.53 ms** | n/a | n/a | n/a | 3.77 ms | n/a | n/a |
| ols | 408 ms | 55.9 ms | 17.4 ms | n/a | 3.39 s | **14.4 ms** | n/a | n/a | n/a | 20.3 ms | n/a | n/a |

Peak memory of the processes that build large lists or read large tables (the inputs are included):

| Benchmark | Quark list | Python | Quark vec | NumPy | Quark tbl | Polars |
|---|---|---|---|---|---|---|
| sieve | 645 MB | 86 MB | n/a | 32 MB | n/a | n/a |
| ewma | 1,276 MB | 1,165 MB | 871 MB | 251 MB | n/a | 284 MB |
| arith | 1,366 MB | 1,548 MB | 386 MB | 327 MB | 677 MB | 359 MB |
| filter | 1,293 MB | 935 MB | 312 MB | 337 MB | 677 MB | 359 MB |
| groupby | 66 MB | 128 MB | n/a | 112 MB | 98 MB | 141 MB |
| query | 176 MB | 219 MB | n/a | n/a | 143 MB | 296 MB |

A Quark `list` of 10M bools peaks at 645 MB; Python's list of 10M references to `True` needs 86 MB. The `quark_tbl` peaks of `arith` and `filter` include the CSV reader holding the whole 385 MB text of `cols.csv` while it parses (it is freed after, but the process's peak stays).

## Library default threads

`--threads 0`: NumPy, R and Quark's `std/blas` use OpenBLAS's threads, Polars and data.table their own thread pools, and the Boehm GC marks in parallel. Quark code itself (tables included), NumPy outside BLAS, pandas and base R stay single-threaded.

The fastest implementation in each language, and how many times slower Quark's best is (below 1 means Quark is faster).

| Benchmark | Best Quark | Best Python | Best R | Quark / Python | Quark / R |
|---|---|---|---|---|---|
| [fib](README.md#fib) | 10.6 ms (Quark list) | 267 ms (Python) | 1.59 s (R loop) | 0.04 | 0.01 |
| [mandelbrot](README.md#mandelbrot) | 13.8 ms (Quark list) | 105 ms (NumPy) | 275 ms (R vector) | 0.13 | 0.05 |
| [sieve](README.md#sieve) | 635 ms (Quark list) | 54.2 ms (NumPy) | 449 ms (R vector) | 11.70 | 1.41 |
| [ewma](README.md#ewma) | 431 ms (Quark vec) | 134 ms (pandas) | 629 ms (R vector) | 3.23 | 0.69 |
| [reduce](README.md#reduce) | 18.1 ms (Quark vec) | 9.25 ms (Polars) | 80.7 ms (R vector) | 1.95 | 0.22 |
| [arith](README.md#arith) | 45.4 ms (Quark vec) | 24.0 ms (Polars) | 86.7 ms (R data.table) | 1.89 | 0.52 |
| [expr](README.md#expr) | 58.6 ms (Quark tbl) | 95.3 ms (Polars) | 325 ms (R vector) | 0.62 | 0.18 |
| [dot](README.md#dot) | 5.97 ms (Quark vec+blas) | 8.36 ms (pandas) | 98.9 ms (R vector) | 0.71 | 0.06 |
| [zscore](README.md#zscore) | 75.1 ms (Quark vec) | 30.6 ms (Polars) | 203 ms (R data.table) | 2.46 | 0.37 |
| [filter](README.md#filter) | 98.9 ms (Quark tbl) | 17.0 ms (Polars) | 206 ms (R data.table) | 5.80 | 0.48 |
| [sort](README.md#sort) | 176 ms (Quark tbl) | 14.4 ms (Polars) | 56.0 ms (R data.table) | 12.23 | 3.15 |
| [groupby](README.md#groupby) | 53.7 ms (Quark tbl) | 19.4 ms (Polars) | 23.5 ms (R data.table) | 2.77 | 2.29 |
| [join](README.md#join) | 324 ms (Quark list) | 46.9 ms (Polars) | 94.1 ms (R vector) | 6.91 | 3.45 |
| [wordcount](README.md#wordcount) | 269 ms (Quark list) | 120 ms (Polars) | 160 ms (R data.table) | 2.25 | 1.68 |
| [strings](README.md#strings) | 511 ms (Quark list) | 192 ms (pandas) | 3.27 s (R vector) | 2.66 | 0.16 |
| [query](README.md#query) | 133 ms (Quark tbl) | 51.7 ms (Polars) | 67.2 ms (R data.table) | 2.58 | 1.99 |
| [sortkeys](README.md#sortkeys) | 1.14 s (Quark tbl) | 195 ms (Polars) | 103 ms (R data.table) | 5.84 | 11.08 |
| [readcsv](README.md#readcsv) | 510 ms (Quark tbl) | 105 ms (Polars) | 113 ms (R data.table) | 4.88 | 4.51 |
| [gemv](README.md#gemv) | 1.25 ms (Quark vec+blas) | 1.09 ms (NumPy) | 4.91 ms (R vector) | 1.14 | 0.26 |
| [gemm](README.md#gemm) | 1.28 ms (Quark vec+blas) | 553 µs (NumPy) | 1.16 ms (R vector) | 2.32 | 1.11 |
| [ols](README.md#ols) | 19.7 ms (Quark vec+blas) | 14.7 ms (NumPy) | 19.9 ms (R vector) | 1.34 | 0.99 |

Median time per implementation. **Bold** is the fastest in the row; `n/a` means the tool has no natural way to do it, `planned` an implementation still to be written.

| Benchmark | Quark list | Quark vec | Quark vec+blas | Quark tbl | Python | NumPy | pandas | Polars | R loop | R vector | R data.frame | R data.table |
|---|---|---|---|---|---|---|---|---|---|---|---|---|
| fib | **10.6 ms** | n/a | n/a | n/a | 267 ms | n/a | n/a | n/a | 1.59 s | n/a | n/a | n/a |
| mandelbrot | **13.8 ms** | 225 ms | n/a | n/a | 987 ms | 105 ms | n/a | n/a | 758 ms | 275 ms | n/a | n/a |
| sieve | 635 ms | n/a | n/a | n/a | 1.47 s | **54.2 ms** | n/a | n/a | 2.70 s | 449 ms | n/a | n/a |
| ewma | 501 ms | 431 ms | n/a | n/a | 1.41 s | 6.25 s | **134 ms** | 185 ms | 1.58 s | 629 ms | n/a | n/a |
| reduce | 45.3 ms | 18.1 ms | n/a | 181 ms | 732 ms | 20.3 ms | 65.4 ms | **9.25 ms** | n/a | 80.7 ms | n/a | 81.8 ms |
| arith | 601 ms | 45.4 ms | 143 ms | 60.1 ms | 1.33 s | 29.7 ms | 45.8 ms | **24.0 ms** | n/a | 91.3 ms | 87.4 ms | 86.7 ms |
| expr | 650 ms | 199 ms | n/a | **58.6 ms** | 2.69 s | 111 ms | 169 ms | 95.3 ms | n/a | 325 ms | n/a | 326 ms |
| dot | 123 ms | 30.6 ms | **5.97 ms** | n/a | 932 ms | 10.0 ms | 8.36 ms | 19.3 ms | n/a | 98.9 ms | n/a | n/a |
| zscore | 503 ms | 75.1 ms | 294 ms | 77.4 ms | 2.45 s | 71.7 ms | 136 ms | **30.6 ms** | n/a | 208 ms | 206 ms | 203 ms |
| filter | 653 ms | 185 ms | n/a | 98.9 ms | 1.06 s | 179 ms | 109 ms | **17.0 ms** | n/a | 343 ms | 840 ms | 206 ms |
| sort | 277 ms | 288 ms | n/a | 176 ms | 397 ms | 26.2 ms | 141 ms | **14.4 ms** | n/a | 155 ms | 200 ms | 56.0 ms |
| groupby | 325 ms | n/a | n/a | 53.7 ms | 398 ms | 399 ms | 59.9 ms | **19.4 ms** | n/a | 184 ms | 626 ms | 23.5 ms |
| join | 324 ms | n/a | n/a | planned | 348 ms | 454 ms | 325 ms | **46.9 ms** | n/a | 94.1 ms | 15.0 s | 415 ms |
| wordcount | 269 ms | n/a | n/a | planned | 253 ms | 538 ms | 308 ms | **120 ms** | n/a | 284 ms | n/a | 160 ms |
| strings | 511 ms | n/a | n/a | 521 ms | 396 ms | 1.15 s | **192 ms** | 256 ms | n/a | 3.27 s | n/a | n/a |
| query | 502 ms | n/a | n/a | 133 ms | 749 ms | n/a | 196 ms | **51.7 ms** | n/a | n/a | n/a | 67.2 ms |
| sortkeys | 3.70 s | n/a | n/a | 1.14 s | 4.59 s | 698 ms | 369 ms | 195 ms | n/a | n/a | 7.40 s | **103 ms** |
| readcsv | n/a | n/a | n/a | 510 ms | 3.35 s | n/a | 222 ms | **105 ms** | n/a | n/a | 1.75 s | 113 ms |
| gemv | 47.5 ms | 28.5 ms | 1.25 ms | n/a | 365 ms | **1.09 ms** | n/a | n/a | n/a | 4.91 ms | n/a | n/a |
| gemm | 374 ms | 331 ms | 1.28 ms | n/a | 2.54 s | **553 µs** | n/a | n/a | n/a | 1.16 ms | n/a | n/a |
| ols | 406 ms | 56.1 ms | 19.7 ms | n/a | 3.40 s | **14.7 ms** | n/a | n/a | n/a | 19.9 ms | n/a | n/a |

Polars and data.table gain the most on table work: `query` 167 → 52 ms and 142 → 67 ms, `groupby` 55 → 19 ms and 60 → 24 ms, `readcsv` 357 → 105 ms and 301 → 113 ms, `sortkeys` 879 → 195 ms and 255 → 103 ms, and Polars' `sort` 80 → 14 ms. Quark's tables stay single-threaded, so these are the multi-core targets. Quark's `vec+blas` gets slower and erratic in `arith` and `zscore` (30 → 143 ms and 59 → 294 ms): see [OpenBLAS threads and the GC](#openblas-threads-and-the-gc).

Another session's test build loaded the machine while `mandelbrot`, `arith`, `expr` and `filter` first ran (CPython's `expr` took 10.7 s instead of 2.7 s), so those four were rerun once the machine was quiet again, from an export of the same commit with the same benchmark files (load average 1.2-1.3). The other rows are from the first run (load average 1.5 at the start, 2.1 at the end).

## Tables against data frames

`quark_tbl` against the data frame libraries, one thread, with the fastest of the five in bold; the last column is Quark's fastest implementation without a table:

| Benchmark | Quark tbl | pandas | Polars | R data.table | R data.frame | Quark without tbl |
|---|---|---|---|---|---|---|
| [query](README.md#query) | **134 ms** | 198 ms | 167 ms | 142 ms | n/a | 470 ms (list) |
| [groupby](README.md#groupby) | **52.3 ms** | 59.5 ms | 54.6 ms | 59.5 ms | 612 ms | 326 ms (list) |
| [expr](README.md#expr) | **58.8 ms** | 168 ms | 92.8 ms | 330 ms | n/a | 197 ms (vec) |
| [arith](README.md#arith) | 64.8 ms | 41.9 ms | **23.6 ms** | 85.6 ms | 87.2 ms | 29.7 ms (vec+blas) |
| [zscore](README.md#zscore) | 75.4 ms | 137 ms | **44.3 ms** | 206 ms | 206 ms | 59.1 ms (vec+blas) |
| [reduce](README.md#reduce) | 175 ms | 65.1 ms | **20.2 ms** | 83.3 ms | n/a | 18.4 ms (vec) |
| [filter](README.md#filter) | 99.4 ms | 113 ms | **24.2 ms** | 261 ms | 842 ms | 185 ms (vec) |
| [sort](README.md#sort) | 173 ms | 140 ms | **80.4 ms** | 178 ms | 198 ms | 278 ms (list) |
| [sortkeys](README.md#sortkeys) | 1.13 s | 364 ms | 879 ms | **255 ms** | 7.54 s | 3.80 s (list) |
| [strings](README.md#strings) | 491 ms | **192 ms** | 251 ms | n/a | n/a | 509 ms (list) |
| [readcsv](README.md#readcsv) | 513 ms | **226 ms** | 357 ms | 301 ms | 1.77 s | n/a |

Where the table is fast, it is because each verb is a typed loop over the column buffers: `mutate` evaluates the whole expression per row with no temporary columns (`expr` is one pass over `a` and `b`, where NumPy, pandas and Quark's own `vec` write seven 80 MB temporaries), and `group_by` + `summarize` hash the key column once with the engine's open-addressing table and aggregate typed buffers. Where it is slow, the causes are in [What limits Quark today](#tables).

## What std/blas adds

| Benchmark | Quark vec | Quark vec+blas | Speedup | NumPy | R vector |
|---|---|---|---|---|---|
| arith (`axpy`) | 44.8 ms | 29.7 ms | 1.5× | 29.3 ms | 87.8 ms |
| dot | 30.5 ms | 7.63 ms | 4.0× | 6.84 ms | 99.5 ms |
| zscore (`dot`, `scal`) | 75.6 ms | 59.1 ms | 1.3× | 72.7 ms | 206 ms |
| gemv | 28.7 ms | 1.55 ms | 19× | 1.55 ms | 4.31 ms |
| gemm | 341 ms | 4.13 ms | 83× | 3.53 ms | 3.77 ms |
| ols (`syrk`, `gemv` with `trans`) | 55.9 ms | 17.4 ms | 3.2× | 14.4 ms | 20.3 ms |

`gemm` measured on a quiet machine on 2026-11-13, 15 runs each, one thread: Quark 4.5 ms, R 4.1 ms (both the system OpenBLAS 0.3.20), NumPy 3.5 ms (its bundled OpenBLAS 0.3.31). The level-1 routines (`axpy`, `scal`) return a new vector, so they pay for a copy of the input before BLAS works in place; that is why `axpy` is no faster than NumPy's two passes. The `vec` version of `gemv` varies (9 to 34 ms): its rows are 2,000 short vectors, allocated and freed per run.

## Hand-written C++ for reference

A few kernels written directly in C++, built with Quark's flags (`clang++ -O3 -march=x86-64-v3`) and run on the same data and machine (median of 7, one thread; the C++ column was measured on 2026-11-13):

| Kernel | C++ | Quark | NumPy |
|---|---|---|---|
| `fib(30)` | 4.6 ms | 10.7 ms (list) | (Python 264 ms) |
| dot product, plain loop | 20.0 ms | 116 ms (list), 30.5 ms (vec), 7.63 ms (blas) | 6.84 ms |
| sum, min and max in one loop | 12.6 ms | 46.3 ms (list), 18.4 ms (vec, three passes), 175 ms (tbl `summarize`) | 21.3 ms (four passes) |
| `2.5*a + b` into a reused buffer | 14.8 ms | | |
| `2.5*a + b` into a new `malloc` buffer | 73.6 ms | 44.8 ms (vec), 29.7 ms (blas), 64.8 ms (tbl `mutate`) | 29.3 ms |
| sort 1M doubles (`std::sort` / `std::stable_sort`, on a copy) | 124 / 113 ms | 278 ms (list), 173 ms (tbl) | 26.5 ms |
| append 10M values (`push_back`; reserved first) | 189 ms (75 ms) | 0.4-0.7 s (`push` into a list) | |

- **Plain code**: Quark's `fib` takes 2.3 times as long as C++: every call saves and restores the current source location, and int arithmetic is overflow-checked ([breakdown](#where-a-function-calls-time-goes)). Quark's list loops read elements through a checked conversion each.
- **New large buffers cost more than the arithmetic.** Writing 10M results into memory that is already mapped takes 15 ms; into a fresh `malloc` buffer 74 ms, because every 4 KB page faults the first time it is written. NumPy asks the kernel for huge pages for large arrays (`madvise(MADV_HUGEPAGE)`): the same NumPy expression takes 30 ms with them and 82 ms without (this machine gives huge pages only to programs that ask). Quark's GC reuses its heap, so it lands in between (45 ms). Most of the gap between Quark's `vec` arithmetic and NumPy is this, not the loop: asking for huge pages for large vec buffers is a cheap thing to try.
- **Growing an array is slow on this machine even in C++**: 19 ns per `push_back`, 7.5 ns with the buffer reserved. Quark's `push` (35-65 ns) costs 2-3 times what C++ pays, not 10 times; the rest is the 16-byte `QValue`, the GC scanning the list, and the element-type check.

## What limits Quark today

Diagnosed from the runtime code and isolated runs; each item names where the time goes and a direction, for the people deciding what to change.

### Tables

- **`min` and `max` in `summarize` have no fast path.** They compare every row through the engine's generic comparator (`compare_rows`: null and NaN tests and a type switch per row), while `sum` and `mean` use the vec reductions for a whole table and typed loops per group. `summarize(lo = min(e.a))` on 10M floats takes 76 ms, against 6.1 ms for `min(t.a)`; `sum` and `mean` together take 12 ms. That is most of `reduce`'s 175 ms (`known_issues.md`, Slow paths).
- **Sorting takes the generic comparator unless there is one dense numeric key.** `sort(e.k, desc(e.v))` on 1M rows, a str key with 1,000 distinct values, takes 1.13 s: a `std::stable_sort` of row positions that compares string views through the column offsets on every comparison. data.table (255 ms) and pandas (364 ms) sort the key's codes instead. Direction: replace a str key by the rank of its value among the distinct values (one hash pass and a sort of the distinct values), then sort ints. One dense float key takes the fast path, a stable sort of `(value, row)` pairs: 173 ms for 1M, against 80 ms for Polars and 26 ms for NumPy's SIMD sort; a radix sort of the doubles would close most of it.
- **`filter` and `mutate` test every row for missing values and don't vectorize.** The generated loop checks each column's validity per row (even when the column has no missing values), and `filter` appends the selected row with a branch, which mispredicts on random data: `filter(e.a > 0.5)` on 10M rows takes 99 ms (Polars 24 ms), and `mutate(z = 2.5 * e.a + e.b)` 65 ms (Quark's `vec` 45 ms, NumPy 29 ms). Direction: a second loop for columns with `null_count == 0`, with no validity tests, a branch-free append in `filter`, and the column buffers held in local pointers, so clang can vectorize it.
- **Reading CSV runs at about 100 MB/s.** `readcsv` (52 MB) takes 513 ms against 226 ms for pandas, 301 ms for data.table and 357 ms for Polars, single-threaded. Reading only the `k` column (the reader still splits every field) takes 280 ms, so splitting lines into fields is about half of it and converting the numbers the rest. Direction: find field boundaries with a vectorized scan for separators, quotes and line ends, as fread and Polars do. The reader also checks every str cell for valid UTF-8, which pandas doesn't.
- **A str cell read allocates a Quark string per row**, so `mutate(s = e.s.trim().lower().replace('_', '-'))` costs the same as the list loop (491 against 509 ms) and the string methods' costs below apply.

### Building a list

`list.push` costs 35-65 ns per element (C++'s `push_back` costs 19 ns here, see above), and the list tier spends most of its time there: `arith` (10M pushes) takes 605 ms against 29 ms for NumPy and 1.32 s for CPython, although the loop body itself is a few ns. Two causes:
- `QList` is a `std::vector<QValue>` (16 bytes per element) allocated on Boehm's *scanned* heap. Every collection conservatively scans every element of every live list, including a `list[float]` that can't hold a pointer, and the two 10M-element input lists are scanned in each collection while the output grows. That is also why these runs vary by up to 3 times. Memory peaks are high because the GC heap grows to hold the buffer, its growth copies and the scanned inputs, and isn't given back.
- There is no way to create a list of a given size (`[x] * n` in Python, `rep()` in R): `sieve` builds its list with 10M `push(true)` calls (0.4-0.7 s measured alone on 2026-11-13, when the whole benchmark took 778 ms), then does about 25M index writes, each checked against the element type and the bounds.

Directions: allocate the buffer of a list whose element type is `int`, `float` or `bool` with `GC_MALLOC_ATOMIC` (never scanned), or store such lists unboxed (the planned `list[T]`); add a fill constructor and `reserve`.

### Dicts with string keys

The dict-based `groupby` (326 ms) is barely faster than CPython (393 ms) and 6 times slower than the table's `group_by` (52 ms) or Polars; the dict-based `join` (304 ms) is 3.4 times slower than R's `match`, and the dict-based `query` (470 ms) 3.5 times slower than the table. A `QDict` is a node-based `std::unordered_map`; every `get` and `set` builds a new key string from the C string (a `strlen` and a copy) and hashes it again, so the usual "get, then set" costs two full lookups. CPython caches each string's hash. The table engine groups with its own open-addressing table, which is where its 6 times come from. Directions: open addressing with stored hashes, and an update-in-place operation.

### Strings

String cleaning (`trim().lower().replace()`, 509 ms in a list, 491 ms in a table) is slower than CPython (399 ms) and 2.6 times slower than pandas on pyarrow strings. Quark strings are NUL-terminated C strings with no stored length: every method calls `strlen` again, `lower` calls the locale-aware `tolower` per byte, and every step allocates a new GC string. Directions: keep the length with the string and add ASCII fast paths.

### Vectors

- **Missing operations** made several implementations take detours: no `sort` (`v.to_list().sort().to_vec()`, 282 ms against 27 ms for NumPy; a one-column table sorts in 173 ms), no recurrence or cumulative operations (`ewma` loops over `v[i]`), no element-wise `sqrt`, `exp`, `log`, `**` or `abs`, no slicing or gather by index (the matrix benchmarks keep lists of row vectors), no `zeros`/fill constructor (Mandelbrot builds its zero vector from a list), and no operations on `vec[str]`.
- **Per-element checks with no dense fast path**: division checks every divisor for zero and every slot for null; the mask filter tests validity and the mask bit per element and switches on the dtype inside the loop (`filter` 185 ms; NumPy 178 ms, Polars 24 ms, the table's `filter` 99 ms). When `null_count == 0`, these loops could run without the null tests, so the compiler can vectorize them. The reductions already have this path (`reduce`: 18 ms, NumPy 21 ms).
- **Fresh memory for every result**: each operation writes a new buffer, and its first write to each 4 KB page faults; NumPy avoids most of that with huge pages ([above](#hand-written-c-for-reference)).
- **No expression fusion**: `expr` materializes seven 80 MB temporaries (197 ms; NumPy 111 ms, Polars 93 ms). The same expression in a table's `mutate` is one loop (59 ms).
- **Fixed cost per operation**: the `vec` version of `gemm` does 180,000 operations on 300-element vectors in 341 ms, about 1.9 µs each (allocation and validation), slightly faster than the plain `list` loops (374 ms). Row-by-row code on short vectors gains little from `vec`.

### Sorting

`list.sort()` of 1M floats takes 278 ms (CPython 396 ms, R's radix sort 148 ms, NumPy's SIMD introsort 27 ms). It sorts `QValue`s with a type-checking comparator; a sort of a known `float` list or an f64 `vec` could sort raw doubles, as the table engine's fast path does (173 ms). `sort_by` with a closure is slower again: the list version of `sortkeys`, two `sort_by` calls on 1M row positions, takes 3.8 s.

### OpenBLAS threads and the GC

A program that imports `std/blas` starts OpenBLAS's thread pool at load (one thread per CPU unless `OPENBLAS_NUM_THREADS` is set). With it, allocating large vectors makes the GC print `GC Warning: Repeated allocation of very large block (appr. size 78128 KiB): May lead to memory leak and poor performance` to stderr, and `vec` allocation becomes erratic: `arith` with `blas::axpy` went from 30 ms to 143 ms (median) with default threads, and `zscore` from 59 to 294 ms. Reproduction: a program with `incl 'std/blas'` that computes `2.5 * a + b` on two 10M-element vecs in a loop prints the warning; the same program without the `incl`, or with `OPENBLAS_NUM_THREADS=1`, doesn't. The single-threaded runs above set `OPENBLAS_NUM_THREADS=1`.

## Where a function call's time goes

`fib(30)` takes 10.7 ms. The generated function adds `fib(n - 1) + fib(n - 2)` with `q_checked_add` on two `int64`s and records each statement's source location as one pointer into a static table, and saves and restores the current location on every call (`QSourceLocGuard`). Rebuilding the generated function in C++ with the runtime headers on 2026-11-13, and removing one layer at a time:

| `fib(30)` | Time |
|---|---|
| boxed results added with the generic `q_add`, three stores per statement for the location (an earlier back end) | 31.5 ms |
| `q_checked_add` on `int64`, three stores per statement | 22.4 ms |
| the same with the location as one pointer into a static table, saved and restored per call (what the compiler generates today) | 10.6 ms |
| the same without the per-call save and restore | 8.7 ms |
| the same without source-location tracking | 7.4 ms |
| overflow-checked `int64` arithmetic only | 7.8 ms |
| plain C++ | 4.5 ms |

The per-call save and restore (which also adds an exception cleanup path) is the next 2 ms; the overflow checks cost the rest (7.8 against 4.5 ms). These items are tracked in `architecture.md` §15.
