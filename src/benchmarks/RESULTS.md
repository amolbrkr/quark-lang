# Benchmark results

Measured on 2026-11-13 at commit `f782060` plus the change that added this suite, with the `default` preset (`README.md` describes every benchmark and the rules). To measure again: `python3 bench.py run` (one thread per library) and `python3 bench.py run --threads 0` (library defaults); replace this file's tables with the new reports when the numbers change materially.

## Contents

- [Headlines](#headlines)
- [Setup](#setup)
- [One thread per library](#one-thread-per-library)
- [Library default threads](#library-default-threads)
- [What std/blas adds](#what-stdblas-adds)
- [Hand-written C++ for reference](#hand-written-c-for-reference)
- [What limits Quark today](#what-limits-quark-today)
- [The lowering-ir back end](#the-lowering-ir-back-end)
- [What this means for the table work](#what-this-means-for-the-table-work)

## Headlines

- **Plain compiled code is where Quark wins clearly.** Recursive `fib` runs 8.7 times faster than CPython 3.11 and 52 times faster than R. The scalar Mandelbrot loop (14 ms) beats NumPy's vectorized version (124 ms) by 9 times and pure Python by 73 times. Logic that can't be vectorized (recursion, early exits, state carried from row to row) is Quark's strongest case today.
- **`std/blas` puts Quark on par with NumPy and R for BLAS-shaped work**: `gemv` is as fast as NumPy, `dot` and `ols` within 10-30% of it, and `gemm` within 10% of R's `%*%` on the same OpenBLAS build when the machine is quiet. Over plain `vec` code it is 5 times faster for `dot`, 13 times for `gemv`, 180 times for `gemm`, 4.5 times for `ols`.
- **`vec` arithmetic and reductions are close to NumPy** (reductions 1.15×, `a*b + c` 1.5×, a longer expression 1.7× NumPy's time) and 1.7-3.4 times faster than R vectors. Polars is still 2 times faster than Quark `vec` on expressions (it doesn't materialize intermediates) and 7 times faster on filtering.
- **Building lists, hashing and strings are no faster than CPython.** Quark's `list` code that builds a list of 10M floats is only 2.3 times faster than the same Python; a dict-based group-by is 1.2 times faster than CPython and 6 times slower than Polars; string cleaning is 1.3 times *slower* than CPython. The causes are in the runtime's data structures (below), not in the compiler.
- **The gaps in `vec` force slow detours**: no sort (10 times slower than NumPy through a list), no recurrence (`ewma` 3.4 times slower than pandas), no element-wise math beyond `+ - * /`, no slicing or gather.
- With every library at its default thread count (16 here), Polars and data.table pull 2-19 times ahead of Quark on table work, while Quark stays single-threaded. Linking OpenBLAS with its default thread pool also makes the GC print warnings and slows `vec` allocation (below).

Quark's runtime is a safety-first design: overflow-checked integers, checked element types, a null check on every vector element, a zero check on every divisor. These benchmarks measure that cost along with everything else; where Quark is behind, the notes say whether the cause is a check or a data structure.

## Setup

| | |
|---|---|
| Commit | f782060 (uncommitted changes) |
| Preset | default |
| Threads | 1 |
| Runs | 1 warmup + up to 5 timed, 30.0 s budget per implementation |
| CPU | AMD EPYC 7571 (16 logical CPUs, amazon, docker) |
| Memory | 124.8 GB |
| OS | Linux 6.12.113 |
| Load average | [2.38, 2.85, 2.08] at start, [5.05, 3.68, 2.82] at end |
| Quark toolchain | clang version 22.1.0 (https://github.com/llvm/llvm-project 4434dabb69916856b824f68a64b029c67175e532); go version go1.26.0 linux/amd64 |
| System OpenBLAS (Quark std/blas) | OpenBLAS 0.3.20 NO_LAPACKE DYNAMIC_ARCH NO_AFFINITY Zen MAX_THREADS=64 |
| Python | 3.11.18: numpy 2.4.6, pandas 3.1.1, polars 2.1.0, pyarrow 26.0.1 |
| NumPy BLAS | scipy-openblas 0.3.31.188.0 |
| R | R version 4.4.3 (2025-02-28), data.table 1.18.4 |
| R BLAS | /usr/lib/x86_64-linux-gnu/openblas-pthread/libblas.so.3 |

Sizes: `vec_n=10,000,000`, `sort_n=1,000,000`, `grp_n=1,000,000`, `grp_k=1,000`, `join_n=1,000,000`, `join_k=100,000`, `words_n=1,000,000`, `vocab=20,000`, `str_n=1,000,000`, `fib_n=30`, `sieve_n=10,000,000`, `mandel_n=400`, `mandel_iters=100`, `gemv_n=2,000`, `gemm_n=300`, `ols_n=500,000`, `ols_p=8`.

The machine was shared with another session doing compiler work during the runs (load average 2-5 on 16 vCPUs), so expect about ±10% on any single number. Variance was low (spread under 10%) except for Quark's list tier, where garbage collection makes runs vary by up to 3 times; the medians are used throughout.

## One thread per library

The fastest implementation per language. A ratio below 1 means Quark's best is faster.

The fastest implementation in each language, and how many times slower Quark's best is (below 1 means Quark is faster).

| Benchmark | Best Quark | Best Python | Best R | Quark / Python | Quark / R |
|---|---|---|---|---|---|
| [fib](README.md#fib) | 31.7 ms (Quark list) | 275 ms (Python) | 1.65 s (R loop) | 0.12 | 0.02 |
| [mandelbrot](README.md#mandelbrot) | 13.9 ms (Quark list) | 124 ms (NumPy) | 278 ms (R vector) | 0.11 | 0.05 |
| [sieve](README.md#sieve) | 778 ms (Quark list) | 65.7 ms (NumPy) | 454 ms (R vector) | 11.85 | 1.72 |
| [ewma](README.md#ewma) | 485 ms (Quark vec) | 141 ms (pandas) | 623 ms (R vector) | 3.43 | 0.78 |
| [reduce](README.md#reduce) | 23.9 ms (Quark vec) | 20.4 ms (Polars) | 80.9 ms (R vector) | 1.17 | 0.30 |
| [arith](README.md#arith) | 30.0 ms (Quark vec+blas) | 23.9 ms (Polars) | 84.5 ms (R data.table) | 1.26 | 0.35 |
| [expr](README.md#expr) | 195 ms (Quark vec) | 93.6 ms (Polars) | 328 ms (R data.table) | 2.09 | 0.60 |
| [dot](README.md#dot) | 7.59 ms (Quark vec+blas) | 6.91 ms (NumPy) | 101 ms (R vector) | 1.10 | 0.07 |
| [zscore](README.md#zscore) | 67.4 ms (Quark vec+blas) | 44.8 ms (Polars) | 205 ms (R data.frame) | 1.50 | 0.33 |
| [filter](README.md#filter) | 195 ms (Quark vec) | 27.3 ms (Polars) | 267 ms (R data.table) | 7.15 | 0.73 |
| [sort](README.md#sort) | 280 ms (Quark list) | 27.3 ms (NumPy) | 156 ms (R vector) | 10.25 | 1.80 |
| [groupby](README.md#groupby) | 332 ms (Quark list) | 55.4 ms (Polars) | 70.4 ms (R data.table) | 6.00 | 4.72 |
| [join](README.md#join) | 387 ms (Quark list) | 155 ms (Polars) | 95.3 ms (R vector) | 2.50 | 4.06 |
| [wordcount](README.md#wordcount) | 292 ms (Quark list) | 132 ms (Polars) | 218 ms (R data.table) | 2.21 | 1.34 |
| [strings](README.md#strings) | 530 ms (Quark list) | 190 ms (pandas) | 3.28 s (R vector) | 2.79 | 0.16 |
| [gemv](README.md#gemv) | 1.56 ms (Quark vec+blas) | 1.68 ms (NumPy) | 4.36 ms (R vector) | 0.93 | 0.36 |
| [gemm](README.md#gemm) | 5.64 ms (Quark vec+blas) | 3.63 ms (NumPy) | 3.78 ms (R vector) | 1.55 | 1.49 |
| [ols](README.md#ols) | 18.5 ms (Quark vec+blas) | 14.4 ms (NumPy) | 20.4 ms (R vector) | 1.29 | 0.91 |

Every implementation:

Median time per implementation. **Bold** is the fastest in the row; `n/a` means the tool has no natural way to do it, `planned` an implementation still to be written.

| Benchmark | Quark list | Quark vec | Quark vec+blas | Quark tbl | Python | NumPy | pandas | Polars | R loop | R vector | R data.frame | R data.table |
|---|---|---|---|---|---|---|---|---|---|---|---|---|
| fib | **31.7 ms** | n/a | n/a | n/a | 275 ms | n/a | n/a | n/a | 1.65 s | n/a | n/a | n/a |
| mandelbrot | **13.9 ms** | 351 ms | n/a | n/a | 1.02 s | 124 ms | n/a | n/a | 743 ms | 278 ms | n/a | n/a |
| sieve | 778 ms | n/a | n/a | n/a | 1.51 s | **65.7 ms** | n/a | n/a | 2.71 s | 454 ms | n/a | n/a |
| ewma | 530 ms | 485 ms | n/a | n/a | 1.44 s | 6.47 s | **141 ms** | 187 ms | 1.27 s | 623 ms | n/a | n/a |
| reduce | 106 ms | 23.9 ms | n/a | planned | 739 ms | 20.8 ms | 65.4 ms | **20.4 ms** | n/a | 80.9 ms | n/a | 83.0 ms |
| arith | 584 ms | 46.7 ms | 30.0 ms | planned | 1.33 s | 31.3 ms | 41.1 ms | **23.9 ms** | n/a | 87.7 ms | 86.4 ms | 84.5 ms |
| expr | 690 ms | 195 ms | n/a | planned | 2.70 s | 117 ms | 172 ms | **93.6 ms** | n/a | 329 ms | n/a | 328 ms |
| dot | 120 ms | 37.3 ms | 7.59 ms | n/a | 1.02 s | **6.91 ms** | 7.92 ms | 21.1 ms | n/a | 101 ms | n/a | n/a |
| zscore | 775 ms | 90.1 ms | 67.4 ms | planned | 2.44 s | 74.8 ms | 136 ms | **44.8 ms** | n/a | 207 ms | 205 ms | 207 ms |
| filter | 704 ms | 195 ms | n/a | planned | 1.05 s | 180 ms | 109 ms | **27.3 ms** | n/a | 349 ms | 854 ms | 267 ms |
| sort | 280 ms | 283 ms | n/a | planned | 415 ms | **27.3 ms** | 145 ms | 82.3 ms | n/a | 156 ms | 218 ms | 180 ms |
| groupby | 332 ms | n/a | n/a | planned | 397 ms | 402 ms | 60.8 ms | **55.4 ms** | n/a | 193 ms | 633 ms | 70.4 ms |
| join | 387 ms | n/a | n/a | planned | 452 ms | 512 ms | 441 ms | 155 ms | n/a | **95.3 ms** | 15.4 s | 401 ms |
| wordcount | 292 ms | n/a | n/a | planned | 257 ms | 551 ms | 319 ms | **132 ms** | n/a | 298 ms | n/a | 218 ms |
| strings | 530 ms | n/a | n/a | planned | 402 ms | 1.12 s | **190 ms** | 265 ms | n/a | 3.28 s | n/a | n/a |
| gemv | 49.9 ms | 19.7 ms | **1.56 ms** | n/a | 374 ms | 1.68 ms | n/a | n/a | n/a | 4.36 ms | n/a | n/a |
| gemm | 362 ms | 1.02 s | 5.64 ms | n/a | 2.55 s | **3.63 ms** | n/a | n/a | n/a | 3.78 ms | n/a | n/a |
| ols | 410 ms | 82.9 ms | 18.5 ms | n/a | 3.65 s | **14.4 ms** | n/a | n/a | n/a | 20.4 ms | n/a | n/a |

Peak memory of the processes that build large lists (10M elements; the inputs are included):

| Benchmark | Quark list | Python | Quark vec | NumPy |
|---|---|---|---|---|
| sieve | 789 MB | 85 MB | n/a | 31 MB |
| ewma | 1,276 MB | 1,165 MB | 871 MB | 246 MB |
| arith | 1,507 MB | 1,545 MB | 388 MB | 327 MB |
| filter | 1,370 MB | 934 MB | 312 MB | 336 MB |
| groupby | 60 MB | 127 MB | n/a | 112 MB |

A Quark `list` of 10M bools peaks at 789 MB; Python's list of 10M references to `True` needs 85 MB.

## Library default threads

`--threads 0`: NumPy, R and Quark's `std/blas` use OpenBLAS's threads, Polars and data.table their own thread pools, and the Boehm GC marks in parallel. Quark code itself, NumPy outside BLAS, pandas and base R stay single-threaded.

The fastest implementation in each language, and how many times slower Quark's best is (below 1 means Quark is faster).

| Benchmark | Best Quark | Best Python | Best R | Quark / Python | Quark / R |
|---|---|---|---|---|---|
| [fib](README.md#fib) | 31.5 ms (Quark list) | 270 ms (Python) | 1.59 s (R loop) | 0.12 | 0.02 |
| [mandelbrot](README.md#mandelbrot) | 14.0 ms (Quark list) | 118 ms (NumPy) | 283 ms (R vector) | 0.12 | 0.05 |
| [sieve](README.md#sieve) | 781 ms (Quark list) | 65.4 ms (NumPy) | 476 ms (R vector) | 11.94 | 1.64 |
| [ewma](README.md#ewma) | 464 ms (Quark vec) | 143 ms (pandas) | 643 ms (R vector) | 3.24 | 0.72 |
| [reduce](README.md#reduce) | 24.3 ms (Quark vec) | 9.45 ms (Polars) | 81.0 ms (R vector) | 2.57 | 0.30 |
| [arith](README.md#arith) | 106 ms (Quark vec) | 25.2 ms (Polars) | 90.6 ms (R data.frame) | 4.20 | 1.17 |
| [expr](README.md#expr) | 197 ms (Quark vec) | 102 ms (Polars) | 327 ms (R vector) | 1.93 | 0.60 |
| [dot](README.md#dot) | 6.93 ms (Quark vec+blas) | 6.95 ms (pandas) | 103 ms (R vector) | 1.00 | 0.07 |
| [zscore](README.md#zscore) | 100 ms (Quark vec) | 32.7 ms (Polars) | 207 ms (R data.table) | 3.07 | 0.48 |
| [filter](README.md#filter) | 192 ms (Quark vec) | 16.7 ms (Polars) | 204 ms (R data.table) | 11.50 | 0.94 |
| [sort](README.md#sort) | 273 ms (Quark vec) | 14.6 ms (Polars) | 52.9 ms (R data.table) | 18.62 | 5.15 |
| [groupby](README.md#groupby) | 326 ms (Quark list) | 18.6 ms (Polars) | 19.8 ms (R data.table) | 17.51 | 16.50 |
| [join](README.md#join) | 356 ms (Quark list) | 39.5 ms (Polars) | 87.2 ms (R vector) | 9.00 | 4.09 |
| [wordcount](README.md#wordcount) | 281 ms (Quark list) | 115 ms (Polars) | 163 ms (R data.table) | 2.44 | 1.72 |
| [strings](README.md#strings) | 523 ms (Quark list) | 192 ms (pandas) | 3.21 s (R vector) | 2.72 | 0.16 |
| [gemv](README.md#gemv) | 1.46 ms (Quark vec+blas) | 1.17 ms (NumPy) | 5.01 ms (R vector) | 1.24 | 0.29 |
| [gemm](README.md#gemm) | 1.36 ms (Quark vec+blas) | 558 µs (NumPy) | 1.80 ms (R vector) | 2.44 | 0.75 |
| [ols](README.md#ols) | 17.3 ms (Quark vec+blas) | 19.6 ms (NumPy) | 20.0 ms (R vector) | 0.88 | 0.87 |

Median time per implementation. **Bold** is the fastest in the row; `n/a` means the tool has no natural way to do it, `planned` an implementation still to be written.

| Benchmark | Quark list | Quark vec | Quark vec+blas | Quark tbl | Python | NumPy | pandas | Polars | R loop | R vector | R data.frame | R data.table |
|---|---|---|---|---|---|---|---|---|---|---|---|---|
| fib | **31.5 ms** | n/a | n/a | n/a | 270 ms | n/a | n/a | n/a | 1.59 s | n/a | n/a | n/a |
| mandelbrot | **14.0 ms** | 260 ms | n/a | n/a | 1000 ms | 118 ms | n/a | n/a | 743 ms | 283 ms | n/a | n/a |
| sieve | 781 ms | n/a | n/a | n/a | 1.47 s | **65.4 ms** | n/a | n/a | 2.69 s | 476 ms | n/a | n/a |
| ewma | 527 ms | 464 ms | n/a | n/a | 1.41 s | 6.45 s | **143 ms** | 191 ms | 1.33 s | 643 ms | n/a | n/a |
| reduce | 106 ms | 24.3 ms | n/a | planned | 731 ms | 20.9 ms | 66.7 ms | **9.45 ms** | n/a | 81.0 ms | n/a | 82.8 ms |
| arith | 603 ms | 106 ms | 135 ms | planned | 1.33 s | 32.4 ms | 44.4 ms | **25.2 ms** | n/a | 91.8 ms | 90.6 ms | 95.0 ms |
| expr | 688 ms | 197 ms | n/a | planned | 2.71 s | 116 ms | 176 ms | **102 ms** | n/a | 327 ms | n/a | 332 ms |
| dot | 121 ms | 38.6 ms | **6.93 ms** | n/a | 925 ms | 7.46 ms | 6.95 ms | 20.1 ms | n/a | 103 ms | n/a | n/a |
| zscore | 866 ms | 100 ms | 183 ms | planned | 2.47 s | 76.9 ms | 139 ms | **32.7 ms** | n/a | 213 ms | 235 ms | 207 ms |
| filter | 698 ms | 192 ms | n/a | planned | 1.05 s | 180 ms | 107 ms | **16.7 ms** | n/a | 339 ms | 847 ms | 204 ms |
| sort | 273 ms | 273 ms | n/a | planned | 401 ms | 26.1 ms | 132 ms | **14.6 ms** | n/a | 147 ms | 188 ms | 52.9 ms |
| groupby | 326 ms | n/a | n/a | planned | 395 ms | 383 ms | 61.4 ms | **18.6 ms** | n/a | 187 ms | 618 ms | 19.8 ms |
| join | 356 ms | n/a | n/a | planned | 370 ms | 450 ms | 318 ms | **39.5 ms** | n/a | 87.2 ms | 15.0 s | 404 ms |
| wordcount | 281 ms | n/a | n/a | planned | 264 ms | 542 ms | 314 ms | **115 ms** | n/a | 293 ms | n/a | 163 ms |
| strings | 523 ms | n/a | n/a | planned | 395 ms | 1.16 s | **192 ms** | 254 ms | n/a | 3.21 s | n/a | n/a |
| gemv | 47.2 ms | 15.6 ms | 1.46 ms | n/a | 367 ms | **1.17 ms** | n/a | n/a | n/a | 5.01 ms | n/a | n/a |
| gemm | 356 ms | 527 ms | 1.36 ms | n/a | 2.55 s | **558 µs** | n/a | n/a | n/a | 1.80 ms | n/a | n/a |
| ols | 397 ms | 72.3 ms | **17.3 ms** | n/a | 3.58 s | 19.6 ms | n/a | n/a | n/a | 20.0 ms | n/a | n/a |

Polars and data.table gain the most (`groupby` 55 → 19 ms and 70 → 20 ms, `sort` 82 → 15 ms, `join` 155 → 40 ms). Quark's `vec` and `vec+blas` get slower and erratic in `arith` and `zscore`: see [OpenBLAS threads and the GC](#openblas-threads-and-the-gc).

## What std/blas adds

| Benchmark | Quark vec | Quark vec+blas | Speedup | NumPy | R vector |
|---|---|---|---|---|---|
| arith (`axpy`) | 46.7 ms | 30.0 ms | 1.6× | 31.3 ms | 87.7 ms |
| dot | 37.3 ms | 7.59 ms | 4.9× | 6.91 ms | 101 ms |
| zscore (`dot`, `scal`) | 90.1 ms | 67.4 ms | 1.3× | 74.8 ms | 207 ms |
| gemv | 19.7 ms | 1.56 ms | 13× | 1.68 ms | 4.36 ms |
| gemm | 1.02 s | 5.64 ms | 180× | 3.63 ms | 3.78 ms |
| ols (`syrk`, `gemv` with `trans`) | 82.9 ms | 18.5 ms | 4.5× | 14.4 ms | 20.4 ms |

`gemm` measured again on a quiet machine, 15 runs each, one thread: Quark 4.5 ms, R 4.1 ms (both the system OpenBLAS 0.3.20), NumPy 3.5 ms (its bundled OpenBLAS 0.3.31). The level-1 routines (`axpy`, `scal`) return a new vector, so they pay for a copy of the input before BLAS works in place; that is why `axpy` is no faster than NumPy's two passes.

## Hand-written C++ for reference

A few kernels written directly in C++, built with Quark's flags (`clang++ -O3 -march=x86-64-v3`) and run on the same data and machine (median of 7, one thread):

| Kernel | C++ | Quark | NumPy |
|---|---|---|---|
| `fib(30)` | 4.6 ms | 31.7 ms (list) | (Python 275 ms) |
| dot product, plain loop | 20.0 ms | 120 ms (list), 37.3 ms (vec), 7.59 ms (blas) | 6.91 ms |
| sum, min and max in one loop | 12.6 ms | 106 ms (list), 23.9 ms (vec, three passes) | 20.8 ms (four passes) |
| `2.5*a + b` into a reused buffer | 14.8 ms | | |
| `2.5*a + b` into a new `malloc` buffer | 73.6 ms | 46.7 ms (vec), 30.0 ms (blas) | 31.3 ms |
| sort 1M doubles (`std::sort` / `std::stable_sort`, on a copy) | 124 / 113 ms | 280 ms (list) | 27.3 ms |
| append 10M values (`push_back`; reserved first) | 189 ms (75 ms) | 0.4-0.7 s (`push` into a list) | |

- **Plain code**: Quark's `fib` is 7 times slower than C++: the generated code boxes each call's result into a `QValue` and adds them with the dynamic `q_add`, and tracks the source location in every call ([breakdown](#the-lowering-ir-back-end)). Quark's list loops read elements through a checked conversion each.
- **New large buffers cost more than the arithmetic.** Writing 10M results into memory that is already mapped takes 15 ms; into a fresh `malloc` buffer 74 ms, because every 4 KB page faults the first time it is written. NumPy asks the kernel for huge pages for large arrays (`madvise(MADV_HUGEPAGE)`): the same NumPy expression takes 30 ms with them and 82 ms without (this machine gives huge pages only to programs that ask). Quark's GC reuses its heap, so it lands in between (47 ms). Most of the gap between Quark's `vec` arithmetic and NumPy is this, not the loop: asking for huge pages for large vec buffers is a cheap thing to try.
- **Growing an array is slow on this machine even in C++**: 19 ns per `push_back`, 7.5 ns with the buffer reserved. Quark's `push` (35-65 ns) costs 2-3 times what C++ pays, not 10 times; the rest is the 16-byte `QValue`, the GC scanning the list, and the element-type check.

## What limits Quark today

Diagnosed from the runtime code and isolated runs; each item names where the time goes and a direction, for the people deciding what to change.

### Building a list

`list.push` costs 35-65 ns per element (C++'s `push_back` costs 19 ns here, see above), and the list tier spends most of its time there: `arith` (10M pushes) takes 584 ms against 31 ms for NumPy and 1.33 s for CPython, although the loop body itself is a few ns. Two causes:
- `QList` is a `std::vector<QValue>` (16 bytes per element) allocated on Boehm's *scanned* heap. Every collection conservatively scans every element of every live list, including a `list[float]` that can't hold a pointer, and the two 10M-element input lists are scanned in each collection while the output grows. That is also why these runs vary by up to 3 times. Memory peaks are high because the GC heap grows to hold the buffer, its growth copies and the scanned inputs, and isn't given back.
- There is no way to create a list of a given size (`[x] * n` in Python, `rep()` in R): about half of `sieve`'s time is the 10M `push(true)` calls (0.4-0.7 s measured alone). The other half is about 25M index writes at roughly 12 ns each, each checked against the element type and the bounds.

Directions: allocate the buffer of a list whose element type is `int`, `float` or `bool` with `GC_MALLOC_ATOMIC` (never scanned), or store such lists unboxed (the planned `list[T]`); add a fill constructor and `reserve`.

### Dicts with string keys

The dict-based `groupby` (332 ms) is barely faster than CPython (397 ms) and 6 times slower than Polars or data.table; the dict-based `join` (387 ms) is 4 times slower than R's `match`. A `QDict` is a node-based `std::unordered_map`; every `get` and `set` builds a new key string from the C string (a `strlen` and a copy) and hashes it again, so the usual "get, then set" costs two full lookups. CPython caches each string's hash. Directions: open addressing with stored hashes, and an update-in-place operation. The table engine (`table/engine.hpp`) already groups with its own open-addressing table, so `quark_tbl` should be measured before changing `dict`.

### Strings

String cleaning (`trim().lower().replace()`, 530 ms) is slower than CPython (402 ms) and 2.8 times slower than pandas on pyarrow strings. Quark strings are NUL-terminated C strings with no stored length: every method calls `strlen` again, `lower` calls the locale-aware `tolower` per byte, and every step allocates a new GC string. Directions: keep the length with the string and add ASCII fast paths.

### Vectors

- **Missing operations** made several implementations take detours: no `sort` (`v.to_list().sort().to_vec()`, 283 ms against 27 ms for NumPy), no recurrence or cumulative operations (`ewma` loops over `v[i]`), no element-wise `sqrt`, `exp`, `log`, `**` or `abs`, no slicing or gather by index (the matrix benchmarks keep lists of row vectors), no `zeros`/fill constructor (Mandelbrot builds its zero vector from a list), and no operations on `vec[str]`. `sum`, `min` and `max` had type `any`, which, with a `+` typing bug fixed since (`5ff0225`), rejected `sum(v) + sum(v)` used as a number.
- **Per-element checks with no dense fast path**: division checks every divisor for zero and every slot for null; `sum` checks every slot for null even when the vector has none; the mask filter tests validity and the mask bit per element and switches on the dtype inside the loop (`filter` 195 ms; NumPy 180 ms, Polars 27 ms). When `null_count == 0`, these loops could run without the null tests, so the compiler can vectorize them.
- **Fresh memory for every result**: each operation writes a new buffer, and its first write to each 4 KB page faults; NumPy avoids most of that with huge pages ([above](#hand-written-c-for-reference)).
- **No expression fusion**: `expr` materializes seven 80 MB temporaries (195 ms; NumPy 117 ms, Polars 94 ms).
- **Fixed cost per operation**: the `vec` version of `gemm` does 180,000 operations on 300-element vectors in 1.02 s, about 5.6 µs each (allocation and validation), against 0.36 s for the plain `list` loops. Row-by-row code on short vectors is better written with lists today.

### Sorting

`list.sort()` of 1M floats takes 280 ms (CPython 415 ms, R's radix sort 156 ms, NumPy's SIMD introsort 27 ms). It sorts `QValue`s with a type-checking comparator; a sort of a known `float` list or an f64 `vec` could sort raw doubles.

### OpenBLAS threads and the GC

A program that imports `std/blas` starts OpenBLAS's thread pool at load (one thread per CPU unless `OPENBLAS_NUM_THREADS` is set). With it, allocating large vectors makes the GC print `GC Warning: Repeated allocation of very large block (appr. size 78128 KiB): May lead to memory leak and poor performance` to stderr, and `vec` allocation becomes erratic: `arith` with `blas::axpy` went from 30 ms to 135 ms (median) with default threads. Reproduction: a program with `incl 'std/blas'` that computes `2.5 * a + b` on two 10M-element vecs in a loop prints the warning; the same program without the `incl`, or with `OPENBLAS_NUM_THREADS=1`, doesn't. The single-threaded runs above set `OPENBLAS_NUM_THREADS=1`.

## The lowering-ir back end

The same Quark programs built with the compiler of the `lowering-ir` worktree as it was during these runs (uncommitted work in progress: the lowering pass and typed IR, plus the vec changes of tables D12 and part of D13), one thread:

| Benchmark | Implementation | master | lowering-ir | lowering-ir / master |
|---|---|---|---|---|
| fib | Quark list | 31.7 ms | 22.3 ms | 0.70 |
| mandelbrot | Quark list | 13.9 ms | 13.8 ms | 0.99 |
| mandelbrot | Quark vec | 351 ms | 224 ms | 0.64 |
| sieve | Quark list | 778 ms | 770 ms | 0.99 |
| ewma | Quark list | 530 ms | 529 ms | 1.00 |
| ewma | Quark vec | 485 ms | 452 ms | 0.93 |
| reduce | Quark list | 106 ms | 111 ms | 1.05 |
| reduce | Quark vec | 23.9 ms | 101 ms | 4.24 |
| arith | Quark list | 584 ms | 635 ms | 1.09 |
| arith | Quark vec | 46.7 ms | 45.8 ms | 0.98 |
| arith | Quark vec+blas | 30.0 ms | 29.5 ms | 0.98 |
| expr | Quark list | 690 ms | 682 ms | 0.99 |
| expr | Quark vec | 195 ms | 201 ms | 1.03 |
| dot | Quark list | 120 ms | 119 ms | 1.00 |
| dot | Quark vec | 37.3 ms | 56.9 ms | 1.52 |
| dot | Quark vec+blas | 7.59 ms | 7.70 ms | 1.01 |
| zscore | Quark list | 775 ms | 793 ms | 1.02 |
| zscore | Quark vec | 90.1 ms | 128 ms | 1.42 |
| zscore | Quark vec+blas | 67.4 ms | 85.3 ms | 1.27 |
| filter | Quark list | 704 ms | 678 ms | 0.96 |
| filter | Quark vec | 195 ms | 186 ms | 0.95 |
| sort | Quark list | 280 ms | 275 ms | 0.98 |
| sort | Quark vec | 283 ms | 282 ms | 1.00 |
| groupby | Quark list | 332 ms | 321 ms | 0.97 |
| join | Quark list | 387 ms | 315 ms | 0.81 |
| wordcount | Quark list | 292 ms | 279 ms | 0.95 |
| strings | Quark list | 530 ms | 506 ms | 0.95 |
| gemv | Quark list | 49.9 ms | 47.4 ms | 0.95 |
| gemv | Quark vec | 19.7 ms | 25.3 ms | 1.28 |
| gemv | Quark vec+blas | 1.56 ms | 1.56 ms | 1.00 |
| gemm | Quark list | 362 ms | 352 ms | 0.97 |
| gemm | Quark vec | 1022 ms | 1028 ms | 1.01 |
| gemm | Quark vec+blas | 5.64 ms | 4.14 ms | 0.73 |
| ols | Quark list | 410 ms | 404 ms | 0.98 |
| ols | Quark vec | 82.9 ms | 117 ms | 1.41 |
| ols | Quark vec+blas | 18.5 ms | 17.2 ms | 0.93 |

Every checksum agreed with the reference. The one clear gain is `fib` (31.7 → 22.3 ms). The other ratios below 1 are within the master run's own spread: its `join` runs ranged from 310 to 465 ms, its `vec` Mandelbrot runs from 247 to 508 ms, and `gemm` with BLAS measured 4.5 ms on master once the machine was quiet. Rerun on a quiet machine with both compilers alternating (3 rounds of 7 runs), `join` took 406 ms on master and 413 ms on `lowering-ir`: no difference.

The reductions are slower: `reduce` 4.2 times (24.9 against 103.9 ms in the quiet rerun), and every `vec` benchmark that calls `sum` by 1.3-1.5 times. Timed alone on 10M doubles, `sum` went from 12.2 to 31.8 ms and `min` and `max` from 6.1 to 35 ms. The branch's new `sum` of an f64 vec accumulates in `long double` (x87 instructions, no SIMD) and checks every element for null and NaN, and `min`/`max` compare through a per-element dtype switch. The version merged into master (`5ff0225`) fixed this with pairwise `double` summation and fast paths for vectors with no missing entries: `reduce` with `vec` measured 18.9 ms there (24.9 ms before the merge, 20.8 ms for NumPy), `sum` of 10M doubles 6.4 ms and `min`/`max` 6.1 ms.

Where `fib`'s time goes. Master compiles `fib(n - 1) + fib(n - 2)` by boxing both results into `QValue`s and calling the runtime's generic `q_add`, which checks for vectors and strings before it adds two ints, and which clang doesn't inline. The lowering pass knows that `fib` returns `int` and emits `q_checked_add` on the two `int64`s. Rebuilding the generated function in C++ with the runtime headers, and removing one layer at a time, reproduces both numbers and shows what is left:

| `fib(30)` | Time |
|---|---|
| master: boxed results, `q_add`, source locations | 31.5 ms |
| lowering-ir: `q_checked_add` on `int64`, source locations | 22.4 ms |
| the same with locations as one pointer into a static table (newer `lowering-ir` work), saved and restored per call | 10.6 ms |
| the same without the per-call save and restore | 8.7 ms |
| the same without source-location tracking | 7.4 ms |
| overflow-checked `int64` arithmetic only | 7.8 ms |
| plain C++ | 4.5 ms |

Two thirds of the remaining time is source-location tracking. Every generated function saves the thread-local current location on entry and restores it on exit (`QSourceLocGuard`, which also adds an exception cleanup path), and every statement stores three fields; that costs about 5.5 ns per call. The `lowering-ir` worktree has since moved to one pointer store per statement into a static table of locations; measured the same way, that recovers most of it (10.6 ms), and dropping the per-call save and restore would give 8.7 ms. The overflow checks cost the rest (7.8 against 4.5 ms). These items are tracked in `architecture.md` §15.

## What this means for the table work

- The single-thread targets for `quark_tbl`: Polars `groupby` 55 ms, `filter` 27 ms, `sort` 82 ms, `join` 155 ms, `reduce` 20 ms; data.table `groupby` 70 ms. With default threads, Polars and data.table are 3-4 times faster again.
- Building tables on `QList` or `QDict` would inherit the costs above; the columnar engine, its typed buffers and its open-addressing grouping avoid them.
- The open question of keeping `vec`, typed lists and `list[any]` (`tables_v0_1_spec.md` §12): the list numbers argue for unboxed storage of scalar-typed lists whichever way it is decided.
