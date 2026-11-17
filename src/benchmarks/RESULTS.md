# Benchmark results

Measured on 2026-11-17 at commit `277beb3` (tables v0.3, the thread pool) plus that day's uncommitted changes (the pool's garbage-collector and wake-up fixes, the mask filter's loops), with the `default` preset (`README.md` describes every benchmark and the rules). Two runs: every library pinned to one thread, and every library at its default thread count. To measure again: `python3 bench.py run` (one thread per library) and `python3 bench.py run --threads 0` (library defaults); replace this file's tables with the new reports when the numbers change materially.

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

- **On one thread, a table query is as fast as the best data frame libraries.** The five-step `query` (filter, mutate, group by two keys, four aggregates, sort) on 1M rows takes 132 ms with Quark's `tbl`, against 140 ms for data.table, 141 ms for Polars and 191 ms for pandas; `groupby` takes 54 ms (Polars 54 ms, pandas 60 ms, data.table 61 ms). A `mutate` compiles its whole expression into one loop, so `expr` (eight operations on 10M rows) takes 35 ms, the fastest of every tool (Polars 93 ms, NumPy 112 ms), and `arith` 15 ms (Polars 24 ms).
- **With threads, Quark's row loops and column work scale; its grouping and CSV reading don't yet.** At library defaults (6 threads here, the CPUs the container's quota allows), `expr` takes 12 ms (Polars 94 ms), `reduce` 8.0 ms (Polars 10 ms), `filter` 30 ms (Polars 17 ms) and a one-key `sort` 55 ms (Polars 15 ms, data.table 52 ms). A `query` takes 95 ms and `groupby` 40 ms, about twice what Polars (48 and 19 ms) and data.table (67 and 18 ms) take, because Quark's grouped aggregates and the building of its groups table still run on one thread; `readcsv` takes 465 ms against 110-117 ms, because the CSV reader does.
- **Some table operations are still slow on one thread**: sorting by a string key and a float (`sortkeys` 779 ms, 2.9 times data.table), sorting one float column (174 ms, 6.7 times NumPy, 2.2 times Polars), reading CSV (451 ms, 2.0 times pandas) and string cleaning in a table (519 ms, 2.7 times pandas). The details are [below](#tables).
- **Plain compiled code is where Quark wins most.** Recursive `fib` runs 25 times faster than CPython 3.11 and 150 times faster than R. The scalar Mandelbrot loop (14 ms) beats NumPy's vectorized version (102 ms) by 7 times and pure Python by 72 times. Logic that can't be vectorized (recursion, early exits, state carried from row to row) is a strong case.
- **`std/blas` puts Quark on par with NumPy and R for BLAS-shaped work** on one thread: `gemv` is as fast as NumPy, `dot`, `gemm` and `ols` within about 20% of it. With OpenBLAS's default threads, its pool makes Quark's large `vec` allocations slow and erratic ([below](#openblas-threads-and-the-gc)).
- **`vec` arithmetic and reductions are as fast as NumPy on one thread** (reductions 0.85 times NumPy's time, `2.5*a + b` 0.93 times, the eight-operation expression 1.1 times) and faster with threads (`reduce` 8.0 ms, `arith` 22 ms, the mask filter 31 ms against NumPy's 177 ms).
- **Building lists, hashing in dicts and strings are no faster than CPython**, and the list code varies a lot from run to run: a dict-based group-by is 1.2 times faster than CPython (the table does it 6 times faster), string cleaning 1.3 times *slower*. The causes are in the runtime's data structures (below), not in the compiler.

Quark's runtime is a safety-first design: overflow-checked integers, checked element types, a null check on every vector element, a zero check on every divisor, UTF-8 validation of every string a CSV file brings in. These benchmarks measure that cost along with everything else; where Quark is behind, the notes say whether the cause is a check or a data structure.

## Setup

| | |
|---|---|
| Commit | 277beb3 (uncommitted changes) |
| Preset | default |
| Threads | 1 |
| Runs | 1 warmup + up to 5 timed, 30.0 s budget per implementation |
| CPU | AMD EPYC 7571 (16 logical CPUs, amazon, docker) |
| CPUs the process may use | 16 in its affinity mask, a CPU quota of 6 |
| Memory | 124.8 GB |
| OS | Linux 6.12.113 |
| Load average | [1.13, 1.35, 1.65] at start, [1.64, 1.57, 1.56] at end |
| Quark toolchain | clang version 22.1.0 (https://github.com/llvm/llvm-project 4434dabb69916856b824f68a64b029c67175e532); go version go1.26.0 linux/amd64 |
| System OpenBLAS (Quark std/blas) | OpenBLAS 0.3.20 NO_LAPACKE DYNAMIC_ARCH NO_AFFINITY Zen MAX_THREADS=64 |
| Python | 3.11.18: numpy 2.4.6, pandas 3.1.1, polars 2.1.0, pyarrow 26.0.1 |
| NumPy BLAS | scipy-openblas 0.3.31.188.0 |
| R | R version 4.4.3 (2025-02-28), data.table 1.18.4 |
| R BLAS | /usr/lib/x86_64-linux-gnu/openblas-pthread/libblas.so.3 |

Sizes: `vec_n=10,000,000`, `sort_n=1,000,000`, `grp_n=1,000,000`, `grp_k=1,000`, `join_n=1,000,000`, `join_k=100,000`, `words_n=1,000,000`, `vocab=20,000`, `str_n=1,000,000`, `fib_n=30`, `sieve_n=10,000,000`, `mandel_n=400`, `mandel_iters=100`, `gemv_n=2,000`, `gemm_n=300`, `ols_n=500,000`, `ols_p=8`.

The library-defaults run used the same machine and versions (load average 2.1 at the start, 1.5 at the end). The container's CPU quota lets a process use 6 of the 16 logical CPUs: Quark's default is 6 threads, and libraries that count CPUs another way start more threads than the quota lets run at once. Other sessions were open on the machine, mostly idle (load average 1.1-2.1 on 16 vCPUs). Variance was low (spread under 10%) except for Quark's list tier, where garbage collection makes runs vary by up to 20 times (`filter` with lists: 0.5 to 11.9 s within one run; its fastest runs are the same as on 2026-11-14), the operations that write a new 10M-element column, where one of the five runs is often slower in Quark, Polars and R alike, and Quark's many small `vec` operations with threads (below); the medians are used throughout.

## One thread per library

The fastest implementation in each language, and how many times slower Quark's best is (below 1 means Quark is faster).

| Benchmark | Best Quark | Best Python | Best R | Quark / Python | Quark / R |
|---|---|---|---|---|---|
| [fib](README.md#fib) | 10.5 ms (Quark list) | 264 ms (Python) | 1.57 s (R loop) | 0.04 | 0.01 |
| [mandelbrot](README.md#mandelbrot) | 13.8 ms (Quark list) | 102 ms (NumPy) | 271 ms (R vector) | 0.14 | 0.05 |
| [sieve](README.md#sieve) | 639 ms (Quark list) | 53.3 ms (NumPy) | 456 ms (R vector) | 11.99 | 1.40 |
| [ewma](README.md#ewma) | 424 ms (Quark vec) | 134 ms (pandas) | 652 ms (R vector) | 3.17 | 0.65 |
| [reduce](README.md#reduce) | 17.6 ms (Quark vec) | 20.8 ms (Polars) | 81.2 ms (R vector) | 0.85 | 0.22 |
| [arith](README.md#arith) | 15.3 ms (Quark tbl) | 23.8 ms (Polars) | 86.6 ms (R data.table) | 0.64 | 0.18 |
| [expr](README.md#expr) | 34.7 ms (Quark tbl) | 93.4 ms (Polars) | 327 ms (R vector) | 0.37 | 0.11 |
| [dot](README.md#dot) | 7.68 ms (Quark vec+blas) | 6.88 ms (NumPy) | 100 ms (R vector) | 1.12 | 0.08 |
| [zscore](README.md#zscore) | 50.9 ms (Quark vec+blas) | 43.9 ms (Polars) | 207 ms (R vector) | 1.16 | 0.25 |
| [filter](README.md#filter) | 48.3 ms (Quark tbl) | 25.9 ms (Polars) | 259 ms (R data.table) | 1.86 | 0.19 |
| [sort](README.md#sort) | 174 ms (Quark tbl) | 26.0 ms (NumPy) | 151 ms (R vector) | 6.69 | 1.15 |
| [groupby](README.md#groupby) | 54.1 ms (Quark tbl) | 53.7 ms (Polars) | 61.1 ms (R data.table) | 1.01 | 0.88 |
| [join](README.md#join) | 283 ms (Quark list) | 133 ms (Polars) | 84.8 ms (R vector) | 2.12 | 3.34 |
| [wordcount](README.md#wordcount) | 265 ms (Quark list) | 113 ms (Polars) | 206 ms (R data.table) | 2.35 | 1.29 |
| [strings](README.md#strings) | 512 ms (Quark list) | 189 ms (pandas) | 3.16 s (R vector) | 2.71 | 0.16 |
| [query](README.md#query) | 132 ms (Quark tbl) | 141 ms (Polars) | 140 ms (R data.table) | 0.93 | 0.94 |
| [sortkeys](README.md#sortkeys) | 779 ms (Quark tbl) | 364 ms (pandas) | 266 ms (R data.table) | 2.14 | 2.93 |
| [readcsv](README.md#readcsv) | 451 ms (Quark tbl) | 229 ms (pandas) | 301 ms (R data.table) | 1.97 | 1.50 |
| [gemv](README.md#gemv) | 1.57 ms (Quark vec+blas) | 1.55 ms (NumPy) | 4.30 ms (R vector) | 1.02 | 0.36 |
| [gemm](README.md#gemm) | 4.24 ms (Quark vec+blas) | 3.49 ms (NumPy) | 3.65 ms (R vector) | 1.21 | 1.16 |
| [ols](README.md#ols) | 16.9 ms (Quark vec+blas) | 14.5 ms (NumPy) | 20.5 ms (R vector) | 1.17 | 0.83 |

Median time per implementation. **Bold** is the fastest in the row; `n/a` means the tool has no natural way to do it, `planned` an implementation still to be written.

| Benchmark | Quark list | Quark vec | Quark vec+blas | Quark tbl | Python | NumPy | pandas | Polars | R loop | R vector | R data.frame | R data.table |
|---|---|---|---|---|---|---|---|---|---|---|---|---|
| fib | **10.5 ms** | n/a | n/a | n/a | 264 ms | n/a | n/a | n/a | 1.57 s | n/a | n/a | n/a |
| mandelbrot | **13.8 ms** | 216 ms | n/a | n/a | 990 ms | 102 ms | n/a | n/a | 731 ms | 271 ms | n/a | n/a |
| sieve | 639 ms | n/a | n/a | n/a | 1.50 s | **53.3 ms** | n/a | n/a | 2.68 s | 456 ms | n/a | n/a |
| ewma | 1.31 s | 424 ms | n/a | n/a | 1.40 s | 6.28 s | **134 ms** | 185 ms | 1.30 s | 652 ms | n/a | n/a |
| reduce | 46.0 ms | **17.6 ms** | n/a | 25.2 ms | 758 ms | 20.8 ms | 65.0 ms | 20.8 ms | n/a | 81.2 ms | n/a | 83.5 ms |
| arith | 851 ms | 27.2 ms | 30.2 ms | **15.3 ms** | 1.36 s | 29.4 ms | 41.5 ms | 23.8 ms | n/a | 88.0 ms | 87.2 ms | 86.6 ms |
| expr | 878 ms | 124 ms | n/a | **34.7 ms** | 2.70 s | 112 ms | 168 ms | 93.4 ms | n/a | 327 ms | n/a | 331 ms |
| dot | 116 ms | 21.8 ms | 7.68 ms | n/a | 897 ms | **6.88 ms** | 7.94 ms | 20.9 ms | n/a | 100 ms | n/a | n/a |
| zscore | 503 ms | 69.3 ms | 50.9 ms | 61.3 ms | 2.42 s | 71.9 ms | 138 ms | **43.9 ms** | n/a | 207 ms | 208 ms | 207 ms |
| filter | 3.41 s | 151 ms | n/a | 48.3 ms | 1.06 s | 178 ms | 108 ms | **25.9 ms** | n/a | 341 ms | 834 ms | 259 ms |
| sort | 211 ms | 207 ms | n/a | 174 ms | 390 ms | **26.0 ms** | 130 ms | 79.8 ms | n/a | 151 ms | 195 ms | 180 ms |
| groupby | 326 ms | n/a | n/a | 54.1 ms | 398 ms | 370 ms | 59.9 ms | **53.7 ms** | n/a | 189 ms | 613 ms | 61.1 ms |
| join | 283 ms | n/a | n/a | planned | 341 ms | 445 ms | 303 ms | 133 ms | n/a | **84.8 ms** | 14.6 s | 375 ms |
| wordcount | 265 ms | n/a | n/a | planned | 253 ms | 541 ms | 309 ms | **113 ms** | n/a | 282 ms | n/a | 206 ms |
| strings | 512 ms | n/a | n/a | 519 ms | 392 ms | 1.12 s | **189 ms** | 260 ms | n/a | 3.16 s | n/a | n/a |
| query | 502 ms | n/a | n/a | **132 ms** | 796 ms | n/a | 191 ms | 141 ms | n/a | n/a | n/a | 140 ms |
| sortkeys | 2.45 s | n/a | n/a | 779 ms | 4.58 s | 688 ms | 364 ms | 869 ms | n/a | n/a | 7.57 s | **266 ms** |
| readcsv | n/a | n/a | n/a | 451 ms | 3.39 s | n/a | **229 ms** | 362 ms | n/a | n/a | 1.77 s | 301 ms |
| gemv | 47.9 ms | 27.1 ms | 1.57 ms | n/a | 366 ms | **1.55 ms** | n/a | n/a | n/a | 4.30 ms | n/a | n/a |
| gemm | 359 ms | 309 ms | 4.24 ms | n/a | 2.56 s | **3.49 ms** | n/a | n/a | n/a | 3.65 ms | n/a | n/a |
| ols | 403 ms | 44.0 ms | 16.9 ms | n/a | 3.51 s | **14.5 ms** | n/a | n/a | n/a | 20.5 ms | n/a | n/a |

Peak memory of the processes that build large lists or read large tables (the inputs are included):

| Benchmark | Quark list | Python | Quark vec | NumPy | Quark tbl | Polars |
|---|---|---|---|---|---|---|
| sieve | 781 MB | 86 MB | n/a | 32 MB | n/a | n/a |
| ewma | 1,388 MB | 1,165 MB | 810 MB | 251 MB | n/a | 283 MB |
| arith | 1,608 MB | 1,548 MB | 386 MB | 327 MB | 677 MB | 359 MB |
| filter | 1,293 MB | 935 MB | 351 MB | 337 MB | 677 MB | 360 MB |
| groupby | 60 MB | 128 MB | n/a | 112 MB | 98 MB | 143 MB |
| query | 176 MB | 218 MB | n/a | n/a | 143 MB | 285 MB |

A Quark `list` of 10M bools peaks at 781 MB; Python's list of 10M references to `True` needs 86 MB. The `quark_tbl` peaks of `arith` and `filter` include the CSV reader holding the whole 385 MB text of `cols.csv` while it parses (it is freed after, but the process's peak stays).

## Library default threads

`--threads 0`: Quark runs its query loops, table engine and `vec` operations on its thread pool (6 threads here), Polars and data.table use their own thread pools, and NumPy, R and Quark's `std/blas` use OpenBLAS's threads. Quark's CSV reader, its grouped aggregates, NumPy outside BLAS, pandas and base R stay single-threaded.

The fastest implementation in each language, and how many times slower Quark's best is (below 1 means Quark is faster).

| Benchmark | Best Quark | Best Python | Best R | Quark / Python | Quark / R |
|---|---|---|---|---|---|
| [fib](README.md#fib) | 10.6 ms (Quark list) | 263 ms (Python) | 1.60 s (R loop) | 0.04 | 0.01 |
| [mandelbrot](README.md#mandelbrot) | 13.8 ms (Quark list) | 101 ms (NumPy) | 280 ms (R vector) | 0.14 | 0.05 |
| [sieve](README.md#sieve) | 632 ms (Quark list) | 51.5 ms (NumPy) | 451 ms (R vector) | 12.27 | 1.40 |
| [ewma](README.md#ewma) | 461 ms (Quark vec) | 132 ms (pandas) | 649 ms (R vector) | 3.49 | 0.71 |
| [reduce](README.md#reduce) | 8.02 ms (Quark vec) | 10.3 ms (Polars) | 80.9 ms (R vector) | 0.78 | 0.10 |
| [arith](README.md#arith) | 12.5 ms (Quark tbl) | 24.3 ms (Polars) | 85.6 ms (R data.table) | 0.51 | 0.15 |
| [expr](README.md#expr) | 12.4 ms (Quark tbl) | 94.1 ms (Polars) | 328 ms (R vector) | 0.13 | 0.04 |
| [dot](README.md#dot) | 5.80 ms (Quark vec+blas) | 5.71 ms (NumPy) | 104 ms (R vector) | 1.02 | 0.06 |
| [zscore](README.md#zscore) | 34.3 ms (Quark vec) | 30.7 ms (Polars) | 205 ms (R data.table) | 1.12 | 0.17 |
| [filter](README.md#filter) | 29.6 ms (Quark tbl) | 16.6 ms (Polars) | 206 ms (R data.table) | 1.79 | 0.14 |
| [sort](README.md#sort) | 54.7 ms (Quark tbl) | 15.3 ms (Polars) | 51.5 ms (R data.table) | 3.58 | 1.06 |
| [groupby](README.md#groupby) | 40.1 ms (Quark tbl) | 18.7 ms (Polars) | 18.2 ms (R data.table) | 2.14 | 2.20 |
| [join](README.md#join) | 302 ms (Quark list) | 43.0 ms (Polars) | 87.7 ms (R vector) | 7.02 | 3.44 |
| [wordcount](README.md#wordcount) | 273 ms (Quark list) | 117 ms (Polars) | 158 ms (R data.table) | 2.34 | 1.73 |
| [strings](README.md#strings) | 243 ms (Quark tbl) | 194 ms (pandas) | 3.19 s (R vector) | 1.25 | 0.08 |
| [query](README.md#query) | 94.7 ms (Quark tbl) | 47.6 ms (Polars) | 66.6 ms (R data.table) | 1.99 | 1.42 |
| [sortkeys](README.md#sortkeys) | 321 ms (Quark tbl) | 192 ms (Polars) | 95.3 ms (R data.table) | 1.67 | 3.37 |
| [readcsv](README.md#readcsv) | 465 ms (Quark tbl) | 110 ms (Polars) | 117 ms (R data.table) | 4.22 | 3.96 |
| [gemv](README.md#gemv) | 1.19 ms (Quark vec+blas) | 9.93 ms (NumPy) | 4.85 ms (R vector) | 0.12 | 0.24 |
| [gemm](README.md#gemm) | 1.23 ms (Quark vec+blas) | 1.01 ms (NumPy) | 1.02 ms (R vector) | 1.23 | 1.21 |
| [ols](README.md#ols) | 29.8 ms (Quark vec) | 17.9 ms (NumPy) | 20.0 ms (R vector) | 1.66 | 1.49 |

Median time per implementation. **Bold** is the fastest in the row; `n/a` means the tool has no natural way to do it, `planned` an implementation still to be written.

| Benchmark | Quark list | Quark vec | Quark vec+blas | Quark tbl | Python | NumPy | pandas | Polars | R loop | R vector | R data.frame | R data.table |
|---|---|---|---|---|---|---|---|---|---|---|---|---|
| fib | **10.6 ms** | n/a | n/a | n/a | 263 ms | n/a | n/a | n/a | 1.60 s | n/a | n/a | n/a |
| mandelbrot | **13.8 ms** | 269 ms | n/a | n/a | 984 ms | 101 ms | n/a | n/a | 732 ms | 280 ms | n/a | n/a |
| sieve | 632 ms | n/a | n/a | n/a | 1.49 s | **51.5 ms** | n/a | n/a | 2.66 s | 451 ms | n/a | n/a |
| ewma | 1.09 s | 461 ms | n/a | n/a | 1.42 s | 6.05 s | **132 ms** | 184 ms | 1.28 s | 649 ms | n/a | n/a |
| reduce | 45.3 ms | **8.02 ms** | n/a | 10.8 ms | 733 ms | 21.4 ms | 64.6 ms | 10.3 ms | n/a | 80.9 ms | n/a | 82.0 ms |
| arith | 804 ms | 22.3 ms | 134 ms | **12.5 ms** | 1.37 s | 29.2 ms | 40.9 ms | 24.3 ms | n/a | 91.3 ms | 87.5 ms | 85.6 ms |
| expr | 669 ms | 88.0 ms | n/a | **12.4 ms** | 2.70 s | 112 ms | 163 ms | 94.1 ms | n/a | 328 ms | n/a | 330 ms |
| dot | 116 ms | 15.1 ms | 5.80 ms | n/a | 898 ms | **5.71 ms** | 6.80 ms | 19.6 ms | n/a | 104 ms | n/a | n/a |
| zscore | 498 ms | 34.3 ms | 185 ms | 36.2 ms | 2.42 s | 72.0 ms | 136 ms | **30.7 ms** | n/a | 207 ms | 206 ms | 205 ms |
| filter | 2.73 s | 30.5 ms | n/a | 29.6 ms | 1.03 s | 177 ms | 108 ms | **16.6 ms** | n/a | 340 ms | 838 ms | 206 ms |
| sort | 212 ms | 215 ms | n/a | 54.7 ms | 387 ms | 26.4 ms | 131 ms | **15.3 ms** | n/a | 144 ms | 198 ms | 51.5 ms |
| groupby | 321 ms | n/a | n/a | 40.1 ms | 403 ms | 369 ms | 60.0 ms | 18.7 ms | n/a | 181 ms | 609 ms | **18.2 ms** |
| join | 302 ms | n/a | n/a | planned | 350 ms | 449 ms | 309 ms | **43.0 ms** | n/a | 87.7 ms | 14.5 s | 391 ms |
| wordcount | 273 ms | n/a | n/a | planned | 249 ms | 542 ms | 314 ms | **117 ms** | n/a | 283 ms | n/a | 158 ms |
| strings | 532 ms | n/a | n/a | 243 ms | 399 ms | 1.15 s | **194 ms** | 258 ms | n/a | 3.19 s | n/a | n/a |
| query | 475 ms | n/a | n/a | 94.7 ms | 774 ms | n/a | 198 ms | **47.6 ms** | n/a | n/a | n/a | 66.6 ms |
| sortkeys | 2.44 s | n/a | n/a | 321 ms | 4.55 s | 688 ms | 369 ms | 192 ms | n/a | n/a | 7.35 s | **95.3 ms** |
| readcsv | n/a | n/a | n/a | 465 ms | 3.32 s | n/a | 223 ms | **110 ms** | n/a | n/a | 1.74 s | 117 ms |
| gemv | 47.9 ms | 26.7 ms | **1.19 ms** | n/a | 364 ms | 9.93 ms | n/a | n/a | n/a | 4.85 ms | n/a | n/a |
| gemm | 357 ms | 339 ms | 1.23 ms | n/a | 2.53 s | **1.01 ms** | n/a | n/a | n/a | 1.02 ms | n/a | n/a |
| ols | 416 ms | 29.8 ms | 78.9 ms | n/a | 3.45 s | **17.9 ms** | n/a | n/a | n/a | 20.0 ms | n/a | n/a |

Quark's gains from 1 to 6 threads: `expr` 35 → 12 ms in a table, `sort` 174 → 55 ms, `sortkeys` 779 → 321 ms, the `vec` mask filter 151 → 31 ms, `strings` in a table 519 → 243 ms, `reduce` 18 → 8.0 ms, `zscore` 69 → 34 ms with vecs, `filter` 48 → 30 ms in a table, `query` 132 → 95 ms and `groupby` 54 → 40 ms. Polars and data.table gain more on grouping (`groupby` 54 → 19 ms and 61 → 18 ms, `query` 141 → 48 ms and 140 → 67 ms) and CSV reading (362 → 110 ms and 301 → 117 ms), the parts of Quark that stay on one thread. Element-wise work on 10M rows is bound by memory bandwidth on this machine past 2-3 threads (`arith` in a table: 15 → 12.5 ms). Two Quark rows get slower: `mandelbrot` with vecs (216 → 269 ms, spread 240-346 ms), a loop of 1,000 operations on vecs of 160,000 elements, just over the size the pool splits ([below](#threads)); and `std/blas` with OpenBLAS's threads ([below](#openblas-threads-and-the-gc)).

## Tables against data frames

`quark_tbl` against the data frame libraries, one thread, with the fastest of the five in bold; the last column is Quark's fastest implementation without a table:

| Benchmark | Quark tbl | pandas | Polars | R data.table | R data.frame | Quark without tbl |
|---|---|---|---|---|---|---|
| [query](README.md#query) | **132 ms** | 191 ms | 141 ms | 140 ms | n/a | 502 ms (list) |
| [groupby](README.md#groupby) | 54.1 ms | 59.9 ms | **53.7 ms** | 61.1 ms | 613 ms | 326 ms (list) |
| [expr](README.md#expr) | **34.7 ms** | 168 ms | 93.4 ms | 331 ms | n/a | 124 ms (vec) |
| [arith](README.md#arith) | **15.3 ms** | 41.5 ms | 23.8 ms | 86.6 ms | 87.2 ms | 27.2 ms (vec) |
| [zscore](README.md#zscore) | 61.3 ms | 138 ms | **43.9 ms** | 207 ms | 208 ms | 50.9 ms (vec+blas) |
| [reduce](README.md#reduce) | 25.2 ms | 65.0 ms | **20.8 ms** | 83.5 ms | n/a | 17.6 ms (vec) |
| [filter](README.md#filter) | 48.3 ms | 108 ms | **25.9 ms** | 259 ms | 834 ms | 151 ms (vec) |
| [sort](README.md#sort) | 174 ms | 130 ms | **79.8 ms** | 180 ms | 195 ms | 207 ms (vec) |
| [sortkeys](README.md#sortkeys) | 779 ms | 364 ms | 869 ms | **266 ms** | 7.57 s | 2.45 s (list) |
| [strings](README.md#strings) | 519 ms | **189 ms** | 260 ms | n/a | n/a | 512 ms (list) |
| [readcsv](README.md#readcsv) | 451 ms | **229 ms** | 362 ms | 301 ms | 1.77 s | n/a |

Where the table is fast, it is because each verb is a typed loop over the column buffers: `mutate` evaluates the whole expression per row with no temporary columns (`expr` is one pass over `a` and `b`, where NumPy, pandas and Quark's own `vec` write seven 80 MB temporaries), and `group_by` + `summarize` hash the key column once with the engine's open-addressing table and aggregate typed buffers. Where it is slow, the causes are in [What limits Quark today](#tables).

## What std/blas adds

One thread:

| Benchmark | Quark vec | Quark vec+blas | Speedup | NumPy | R vector |
|---|---|---|---|---|---|
| arith (`axpy`) | 27.2 ms | 30.2 ms | 0.9× | 29.4 ms | 88.0 ms |
| dot | 21.8 ms | 7.68 ms | 2.8× | 6.88 ms | 100 ms |
| zscore (`dot`, `scal`) | 69.3 ms | 50.9 ms | 1.4× | 71.9 ms | 207 ms |
| gemv | 27.1 ms | 1.57 ms | 17× | 1.55 ms | 4.30 ms |
| gemm | 309 ms | 4.24 ms | 73× | 3.49 ms | 3.65 ms |
| ols (`syrk`, `gemv` with `trans`) | 44.0 ms | 16.9 ms | 2.6× | 14.5 ms | 20.5 ms |

The level-1 routines (`axpy`, `scal`) return a new vector, so they pay for a copy of the input before BLAS works in place; that is why `axpy` is now slower than Quark's own `vec` arithmetic, which writes its result in one pass. `gemm` and R's `%*%` use the same system OpenBLAS 0.3.20; NumPy bundles OpenBLAS 0.3.31. The `vec` version of `gemv` varies: its rows are 2,000 short vectors, allocated and freed per run.

## Hand-written C++ for reference

A few kernels written directly in C++, built with Quark's flags (`clang++ -O3 -march=x86-64-v3`) and run on the same data and machine (median of 7, one thread; the C++ column was measured on 2026-11-13, the others in the one-thread run above):

| Kernel | C++ | Quark | NumPy |
|---|---|---|---|
| `fib(30)` | 4.6 ms | 10.5 ms (list) | (Python 264 ms) |
| dot product, plain loop | 20.0 ms | 116 ms (list), 21.8 ms (vec), 7.68 ms (blas) | 6.88 ms |
| sum, min and max in one loop | 12.6 ms | 46.0 ms (list), 17.6 ms (vec, three passes), 25.2 ms (tbl `summarize`) | 20.8 ms (four passes) |
| `2.5*a + b` into a reused buffer | 14.8 ms | | |
| `2.5*a + b` into a new `malloc` buffer | 73.6 ms | 27.2 ms (vec), 30.2 ms (blas), 15.3 ms (tbl `mutate`) | 29.4 ms |
| sort 1M doubles (`std::sort` / `std::stable_sort`, on a copy) | 124 / 113 ms | 211 ms (list), 174 ms (tbl) | 26.0 ms |
| append 10M values (`push_back`; reserved first) | 189 ms (75 ms) | 0.4-0.7 s (`push` into a list) | |

- **Plain code**: Quark's `fib` takes 2.3 times as long as C++: every call saves and restores the current source location, and int arithmetic is overflow-checked ([breakdown](#where-a-function-calls-time-goes)). Quark's list loops read elements through a checked conversion each.
- **New large buffers cost more than the arithmetic.** Writing 10M results into memory that is already mapped takes 15 ms; into a fresh `malloc` buffer 74 ms, because every 4 KB page faults the first time it is written. NumPy asks the kernel for huge pages for large arrays (`madvise(MADV_HUGEPAGE)`): the same NumPy expression takes 30 ms with them and 82 ms without (this machine gives huge pages only to programs that ask). Quark's GC reuses its heap, and a new column or vec buffer is no longer zeroed before every element is written, so Quark's `vec` arithmetic takes 27 ms and a table's `mutate` 15 ms (one loop over both columns).
- **Growing an array is slow on this machine even in C++**: 19 ns per `push_back`, 7.5 ns with the buffer reserved. Quark's `push` (35-65 ns) costs 2-3 times what C++ pays, not 10 times; the rest is the 16-byte `QValue`, the GC scanning the list, and the element-type check.

## What limits Quark today

Diagnosed from the runtime code and isolated runs; each item names where the time goes and a direction, for the people deciding what to change.

### Tables

- **Grouping runs on one thread.** `group_by` hashes the keys on the thread pool, but builds the table of groups in one pass in row order, and the aggregates of many small groups (`count`, int and bool `sum`, `mean`, `n_distinct`, per-group `min` and `max`) are one loop over the rows each. `groupby` (1,000 groups) goes from 54 ms on one thread to 40 ms on 6, where Polars and data.table reach 19 and 18 ms; `query` from 132 to 95 ms (Polars 48 ms, data.table 67 ms). Direction: per-block partial aggregates merged in block order (counts and int sums are exact, so the order doesn't matter; float sums need the per-group pairwise rule), as `specs/tables_v0_3_spec.md` §8.1 plans.
- **The CSV reader runs on one thread**, at about 115 MB/s: `readcsv` (52 MB) takes 451 ms against 229 ms for pandas and 301 ms for data.table on one thread, and 465 ms against 110 ms for Polars and 117 ms for data.table with their threads. Direction: split the text at line ends outside quotes and parse the parts in parallel, with rows in file order and the first problem in file order (spec §8.1, §8.4). The reader also checks every str cell for valid UTF-8, which pandas doesn't.
- **Sorting.** One dense float key sorts `(value, row)` pairs with a stable merge sort: 174 ms for 1M on one thread (Polars 80 ms, NumPy's SIMD sort 26 ms) and 55 ms on 6 (Polars 15 ms); a radix sort of the doubles would close most of it. A string key and a float (`sortkeys`) ranks the strings first and then sorts integers: 779 ms on one thread (data.table 266 ms, pandas 364 ms) and 321 ms on 6 (data.table 95 ms).
- **`filter`** takes 48 ms on 10M rows on one thread (Polars 26 ms) and 30 ms on 6 (Polars 17 ms): the predicate loop is branch-free, and most of the rest is gathering the kept rows of both columns into new buffers.
- **A str cell passed to a method allocates a Quark string per row**, and the string methods' costs below apply: `mutate(s = e.s.trim().lower().replace('_', '-'))` costs the same as the list loop on one thread (519 against 512 ms; pandas 189 ms). With threads it takes 243 ms.

### Threads

- **Many small `vec` operations gain little and vary.** `mandelbrot` with vecs runs 1,000 operations on vecs of 160,000 elements, just over the 131,072 the pool splits: 216 ms on one thread, 269 ms (240 to 346 ms) at 6. Each operation allocates a new buffer, so collections are frequent, and a split of three blocks saves less than the scheduling costs when other processes share the CPU quota. Directions: a higher threshold for operations that allocate, or fusing `vec` expressions into one loop (`specs/tables_v0_3_spec.md` §13).
- **Element-wise work on 10M rows is bound by memory bandwidth** past 2-3 threads on this machine: `arith` in a table goes 15 → 12.5 ms, an addition of two 2M-element vecs into a preallocated buffer 3.0 → 2.6 ms.
- **The collector marks on one thread, and idle workers are unregistered from it.** Both were measured as the slowdowns they replace: with 6 marker threads, a table `mutate` that builds strings took 0.83 s on 6 threads (0.20 s with one marker), and five idle registered workers made every collection take 282 µs instead of 64 µs (`architecture.md` §2.6).

### Building a list

`list.push` costs 35-65 ns per element (C++'s `push_back` costs 19 ns here, see above), and the list tier spends most of its time there: `arith` (10M pushes) takes 589-851 ms in the runs of this day against 29 ms for NumPy and 1.36 s for CPython, although the loop body itself is a few ns. Two causes:
- `QList` is a `std::vector<QValue>` (16 bytes per element) allocated on Boehm's *scanned* heap. Every collection conservatively scans every element of every live list, including a `list[float]` that can't hold a pointer, and the two 10M-element input lists are scanned in each collection while the output grows. That is also why these runs vary by up to 20 times. Memory peaks are high because the GC heap grows to hold the buffer, its growth copies and the scanned inputs, and isn't given back.
- There is no way to create a list of a given size (`[x] * n` in Python, `rep()` in R): `sieve` builds its list with 10M `push(true)` calls, then does about 25M index writes, each checked against the element type and the bounds.

Directions: allocate the buffer of a list whose element type is `int`, `float` or `bool` with `GC_MALLOC_ATOMIC` (never scanned), or store such lists unboxed (the planned `list[T]`); add a fill constructor and `reserve`.

### Dicts with string keys

The dict-based `groupby` (326 ms) is barely faster than CPython (398 ms) and 6 times slower than the table's `group_by` (54 ms) or Polars; the dict-based `join` (283 ms) is 3.3 times slower than R's `match`, and the dict-based `query` (502 ms) 3.8 times slower than the table. A `QDict` is a node-based `std::unordered_map`; every `get` and `set` builds a new key string from the C string (a `strlen` and a copy) and hashes it again, so the usual "get, then set" costs two full lookups. CPython caches each string's hash. The table engine groups with its own open-addressing table, which is where its 6 times come from. Directions: open addressing with stored hashes, and an update-in-place operation.

### Strings

String cleaning (`trim().lower().replace()`, 512 ms in a list, 519 ms in a table) is slower than CPython (392 ms) and 2.7 times slower than pandas on pyarrow strings. Quark strings are NUL-terminated C strings with no stored length: every method calls `strlen` again, `lower` calls the locale-aware `tolower` per byte, and every step allocates a new GC string. Directions: keep the length with the string and add ASCII fast paths.

### Vectors

- **Missing operations** made several implementations take detours: no `sort` (`v.to_list().sort().to_vec()`, 207 ms against 26 ms for NumPy; a one-column table sorts in 174 ms), no recurrence or cumulative operations (`ewma` loops over `v[i]`), no element-wise `sqrt`, `exp`, `log`, `**` or `abs`, no slicing or gather by index (the matrix benchmarks keep lists of row vectors), no `zeros`/fill constructor (Mandelbrot builds its zero vector from a list), and no operations on `vec[str]`.
- **Per-element checks**: division checks every divisor for zero and every slot for null. The mask filter is a branch per element (`filter` with vecs: 151 ms on one thread, NumPy 178 ms, Polars 26 ms).
- **Fresh memory for every result**: each operation writes a new buffer, and its first write to each 4 KB page faults; NumPy avoids most of that with huge pages ([above](#hand-written-c-for-reference)).
- **No expression fusion**: `expr` materializes seven 80 MB temporaries (124 ms on one thread, 88 ms on 6; NumPy 112 ms, Polars 93 ms). The same expression in a table's `mutate` is one loop (35 ms, 12 ms on 6).
- **Fixed cost per operation**: the `vec` version of `gemm` does 180,000 operations on 300-element vectors in 309 ms, about 1.7 µs each (allocation and validation), slightly faster than the plain `list` loops (359 ms). Row-by-row code on short vectors gains little from `vec`.

### Sorting

`list.sort()` of 1M floats takes 211 ms (CPython 390 ms, R's radix sort 151 ms, NumPy's SIMD introsort 26 ms). It sorts `QValue`s with a type-checking comparator; a sort of a known `float` list or an f64 `vec` could sort raw doubles, as the table engine does (174 ms). `sort_by` with a closure is slower again: the list version of `sortkeys`, two `sort_by` calls on 1M row positions, takes 2.45 s.

### OpenBLAS threads and the GC

A program that imports `std/blas` starts OpenBLAS's thread pool at load (one thread per CPU unless `OPENBLAS_NUM_THREADS` is set). With it, allocating large vectors makes the GC print `GC Warning: Repeated allocation of very large block (appr. size 78128 KiB): May lead to memory leak and poor performance` to stderr, and `vec` allocation becomes erratic: with default threads, `arith` with `blas::axpy` went from 30 ms to 134 ms (median), `zscore` from 51 to 185 ms and `ols` from 17 to 79 ms. Reproduction: a program with `incl 'std/blas'` that computes `2.5 * a + b` on two 10M-element vecs in a loop prints the warning; the same program without the `incl`, or with `OPENBLAS_NUM_THREADS=1`, doesn't. The single-threaded runs above set `OPENBLAS_NUM_THREADS=1`.

## Where a function call's time goes

`fib(30)` takes 10.5 ms. The generated function adds `fib(n - 1) + fib(n - 2)` with `q_checked_add` on two `int64`s and records each statement's source location as one pointer into a static table, and saves and restores the current location on every call (`QSourceLocGuard`). Rebuilding the generated function in C++ with the runtime headers on 2026-11-13, and removing one layer at a time:

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
