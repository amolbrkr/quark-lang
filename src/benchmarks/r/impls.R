# R implementations of the benchmarks: loops (r_loop), base vectors (r_vec),
# data.frame (r_df) and data.table (r_dt).
#
# Each implementation is a function (p, measure) -> checksum. It loads its
# data (untimed), passes the work as a function of no arguments to measure,
# and computes the checksum from the result outside the timed region. What
# each benchmark computes and the rules every implementation follows are in
# src/benchmarks/README.md; keep this file in step with the Quark and Python
# implementations. R vectors are 1-based: where a benchmark is defined with
# 0-based positions (sieve, checksums), the code says so.

IMPLS <- list()
impl <- function(bench, name, fn) IMPLS[[paste(bench, name)]] <<- fn

# ------------------------------------------------------------------ loading

f64 <- function(p, name, n = -1) {
  path <- file.path(p$data, name)
  if (n < 0) n <- file.size(path) / 8
  readBin(path, what = "double", n = n, size = 8, endian = "little")
}
lines <- function(p, name) readLines(file.path(p$data, name), encoding = "UTF-8")
text <- function(p, name) {
  path <- file.path(p$data, name)
  readChar(path, file.size(path), useBytes = TRUE)
}
int <- function(p, name) as.integer(p[[name]])

use_data_table <- function() {
  suppressPackageStartupMessages(library(data.table))
  threads <- as.integer(Sys.getenv("BENCH_THREADS", "0"))
  if (threads > 0) setDTthreads(threads)
}

# Matrices are stored row-major in the data files; R matrices are
# column-major, so byrow = TRUE.
matrix_rows <- function(values, nrow, ncol) matrix(values, nrow = nrow, ncol = ncol, byrow = TRUE)

# ---------------------------------------------------------------------- fib

fib <- function(n) if (n < 2L) n else fib(n - 1L) + fib(n - 2L)

impl("fib", "r_loop", function(p, measure) {
  n <- int(p, "n")
  measure(function() fib(n))
})

# --------------------------------------------------------------- mandelbrot

mandel_loop <- function(n, iters) {
  total <- 0
  for (j in 0:(n - 1)) {
    ci <- -1.5 + 3 * j / n
    for (i in 0:(n - 1)) {
      cr <- -2 + 3 * i / n
      zr <- 0
      zi <- 0
      k <- 0L
      while (k < iters) {
        zr2 <- zr * zr
        zi2 <- zi * zi
        if (zr2 + zi2 > 4) break
        zi <- 2 * zr * zi + ci
        zr <- zr2 - zi2 + cr
        k <- k + 1L
      }
      total <- total + k
    }
  }
  total
}

impl("mandelbrot", "r_loop", function(p, measure) {
  n <- int(p, "n")
  iters <- int(p, "iters")
  measure(function() mandel_loop(n, iters))
})

mandel_vec <- function(n, iters) {
  xs <- -2 + 3 * (0:(n - 1)) / n
  ys <- -1.5 + 3 * (0:(n - 1)) / n
  cr <- rep(xs, times = n)
  ci <- rep(ys, each = n)
  zr <- numeric(n * n)
  zi <- numeric(n * n)
  total <- 0
  for (k in seq_len(iters)) {
    zr2 <- zr * zr
    zi2 <- zi * zi
    # Escaped points overflow to Inf and then NaN, which compares as NA.
    total <- total + sum(zr2 + zi2 <= 4, na.rm = TRUE)
    zi <- 2 * zr * zi + ci
    zr <- zr2 - zi2 + cr
  }
  total
}

impl("mandelbrot", "r_vec", function(p, measure) {
  n <- int(p, "n")
  iters <- int(p, "iters")
  measure(function() mandel_vec(n, iters))
})

# -------------------------------------------------------------------- sieve
# s[k + 1] says whether k is prime (positions are 0-based in the definition).

sieve_loop <- function(n) {
  s <- rep(TRUE, n)
  s[1:2] <- FALSE
  i <- 2
  while (i * i < n) {
    if (s[i + 1]) {
      j <- i * i
      while (j < n) {
        s[j + 1] <- FALSE
        j <- j + i
      }
    }
    i <- i + 1
  }
  sum(s)
}

impl("sieve", "r_loop", function(p, measure) {
  n <- int(p, "n")
  measure(function() sieve_loop(n))
})

sieve_vec <- function(n) {
  s <- rep(TRUE, n)
  s[1:2] <- FALSE
  i <- 2
  while (i * i < n) {
    if (s[i + 1]) s[seq.int(i * i, n - 1, by = i) + 1] <- FALSE
    i <- i + 1
  }
  sum(s)
}

impl("sieve", "r_vec", function(p, measure) {
  n <- int(p, "n")
  measure(function() sieve_vec(n))
})

# --------------------------------------------------------------------- ewma

ewma_loop <- function(x, alpha) {
  beta <- 1 - alpha
  n <- length(x)
  out <- numeric(n)
  prev <- x[1]
  out[1] <- prev
  for (i in 2:n) {
    prev <- alpha * x[i] + beta * prev
    out[i] <- prev
  }
  out
}

impl("ewma", "r_loop", function(p, measure) {
  x <- f64(p, "a.f64", int(p, "n"))
  alpha <- as.numeric(p$alpha)
  sum(measure(function() ewma_loop(x, alpha)))
})

impl("ewma", "r_vec", function(p, measure) {
  x <- f64(p, "a.f64", int(p, "n"))
  alpha <- as.numeric(p$alpha)
  # y[i] = alpha * x[i] + (1 - alpha) * y[i - 1], started so that y[1] = x[1].
  run <- function() as.numeric(stats::filter(alpha * x, 1 - alpha, method = "recursive", init = x[1]))
  sum(measure(run))
})

# ------------------------------------------------------------------- reduce

impl("reduce", "r_vec", function(p, measure) {
  a <- f64(p, "a.f64", int(p, "n"))
  sum(measure(function() c(sum(a), min(a), max(a), mean(a))))
})

impl("reduce", "r_dt", function(p, measure) {
  use_data_table()
  dt <- data.table(a = f64(p, "a.f64", int(p, "n")))
  out <- measure(function() dt[, .(s = sum(a), mn = min(a), mx = max(a), m = mean(a))])
  sum(unlist(out))
})

# -------------------------------------------------------------------- arith

two_cols <- function(p) {
  n <- int(p, "n")
  list(a = f64(p, "a.f64", n), b = f64(p, "b.f64", n))
}

impl("arith", "r_vec", function(p, measure) {
  d <- two_cols(p)
  a <- d$a
  b <- d$b
  sum(measure(function() 2.5 * a + b))
})

impl("arith", "r_df", function(p, measure) {
  df <- as.data.frame(two_cols(p))
  run <- function() {
    df$z <- 2.5 * df$a + df$b
    df
  }
  sum(measure(run)$z)
})

impl("arith", "r_dt", function(p, measure) {
  use_data_table()
  dt <- as.data.table(two_cols(p))
  sum(measure(function() dt[, z := 2.5 * a + b])$z)
})

# --------------------------------------------------------------------- expr
# z = (a*a - b) * 0.5 + (a - b) / (b*b + 1)

impl("expr", "r_vec", function(p, measure) {
  d <- two_cols(p)
  a <- d$a
  b <- d$b
  sum(measure(function() (a * a - b) * 0.5 + (a - b) / (b * b + 1)))
})

impl("expr", "r_dt", function(p, measure) {
  use_data_table()
  dt <- as.data.table(two_cols(p))
  sum(measure(function() dt[, z := (a * a - b) * 0.5 + (a - b) / (b * b + 1)])$z)
})

# ---------------------------------------------------------------------- dot

impl("dot", "r_vec", function(p, measure) {
  d <- two_cols(p)
  a <- d$a
  b <- d$b
  measure(function() sum(a * b))
})

# ------------------------------------------------------------------- zscore
# z = (a - mean(a)) / sd(a); checksum sum(z * b).

impl("zscore", "r_vec", function(p, measure) {
  d <- two_cols(p)
  a <- d$a
  sum(measure(function() (a - mean(a)) / sd(a)) * d$b)
})

impl("zscore", "r_df", function(p, measure) {
  df <- as.data.frame(two_cols(p))
  run <- function() {
    df$z <- (df$a - mean(df$a)) / sd(df$a)
    df
  }
  out <- measure(run)
  sum(out$z * out$b)
})

impl("zscore", "r_dt", function(p, measure) {
  use_data_table()
  dt <- as.data.table(two_cols(p))
  out <- measure(function() dt[, z := (a - mean(a)) / sd(a)])
  sum(out$z * out$b)
})

# ------------------------------------------------------------------- filter
# Rows of (a, b) where a > 0.5; checksum count + sum(a) + sum(b) of them.

impl("filter", "r_vec", function(p, measure) {
  d <- two_cols(p)
  a <- d$a
  b <- d$b
  run <- function() {
    m <- a > 0.5
    list(a = a[m], b = b[m])
  }
  out <- measure(run)
  length(out$a) + sum(out$a) + sum(out$b)
})

impl("filter", "r_df", function(p, measure) {
  df <- as.data.frame(two_cols(p))
  out <- measure(function() df[df$a > 0.5, ])
  nrow(out) + sum(out$a) + sum(out$b)
})

impl("filter", "r_dt", function(p, measure) {
  use_data_table()
  dt <- as.data.table(two_cols(p))
  out <- measure(function() dt[a > 0.5])
  nrow(out) + sum(out$a) + sum(out$b)
})

# --------------------------------------------------------------------- sort
# Sort one f64 column ascending; checksum sum(sorted[i] * i) / n, i 0-based.

sort_check <- function(s) sum(s * (seq_along(s) - 1)) / length(s)

impl("sort", "r_vec", function(p, measure) {
  x <- f64(p, "sort.f64", int(p, "n"))
  sort_check(measure(function() sort(x)))
})

impl("sort", "r_df", function(p, measure) {
  df <- data.frame(x = f64(p, "sort.f64", int(p, "n")))
  sort_check(measure(function() df[order(df$x), , drop = FALSE])$x)
})

impl("sort", "r_dt", function(p, measure) {
  use_data_table()
  dt <- data.table(x = f64(p, "sort.f64", int(p, "n")))
  sort_check(measure(function() dt[order(x)])$x)
})

# ------------------------------------------------------------------ groupby
# Sum and count of v per string key; checksum sum(sum_k * count_k) / n + groups.

impl("groupby", "r_vec", function(p, measure) {
  k <- lines(p, "grp_k.txt")
  v <- f64(p, "grp_v.f64")
  out <- measure(function() list(s = tapply(v, k, sum), n = tapply(v, k, length)))
  sum(out$s * out$n) / length(k) + length(out$s)
})

impl("groupby", "r_df", function(p, measure) {
  df <- data.frame(k = lines(p, "grp_k.txt"), v = f64(p, "grp_v.f64"))
  run <- function() aggregate(v ~ k, data = df, FUN = function(x) c(s = sum(x), n = length(x)))
  out <- measure(run)
  sum(out$v[, "s"] * out$v[, "n"]) / nrow(df) + nrow(out)
})

impl("groupby", "r_dt", function(p, measure) {
  use_data_table()
  dt <- data.table(k = lines(p, "grp_k.txt"), v = f64(p, "grp_v.f64"))
  out <- measure(function() dt[, .(s = sum(v), n = .N), by = k])
  sum(out$s * out$n) / nrow(dt) + nrow(out)
})

# --------------------------------------------------------------------- join
# Left join of (k, v) to (k, w) on the string key k; checksum sum(v * w).

impl("join", "r_vec", function(p, measure) {
  k <- lines(p, "join_k.txt")
  v <- f64(p, "join_v.f64")
  dk <- lines(p, "join_dim_k.txt")
  dw <- f64(p, "join_dim_w.f64")
  sum(v * measure(function() dw[match(k, dk)]))
})

impl("join", "r_df", function(p, measure) {
  fact <- data.frame(k = lines(p, "join_k.txt"), v = f64(p, "join_v.f64"))
  dim <- data.frame(k = lines(p, "join_dim_k.txt"), w = f64(p, "join_dim_w.f64"))
  out <- measure(function() merge(fact, dim, by = "k", all.x = TRUE))
  sum(out$v * out$w)
})

impl("join", "r_dt", function(p, measure) {
  use_data_table()
  fact <- data.table(k = lines(p, "join_k.txt"), v = f64(p, "join_v.f64"))
  dim <- data.table(k = lines(p, "join_dim_k.txt"), w = f64(p, "join_dim_w.f64"))
  out <- measure(function() dim[fact, on = "k"])
  sum(out$v * out$w)
})

# ---------------------------------------------------------------- wordcount
# Split a text at single spaces and count each word;
# checksum distinct words + sum(count^2) / words.

wordcount_check <- function(counts) {
  counts <- as.numeric(counts)
  length(counts) + sum(counts^2) / sum(counts)
}

impl("wordcount", "r_vec", function(p, measure) {
  t <- text(p, "words.txt")
  wordcount_check(measure(function() table(strsplit(t, " ", fixed = TRUE)[[1]])))
})

impl("wordcount", "r_dt", function(p, measure) {
  use_data_table()
  t <- text(p, "words.txt")
  out <- measure(function() data.table(w = strsplit(t, " ", fixed = TRUE)[[1]])[, .N, by = w])
  wordcount_check(out$N)
})

# ------------------------------------------------------------------ strings
# s.trim().lower() with '_' replaced by '-'; checksum
# count(starts with 'item-') + total length of the results.

impl("strings", "r_vec", function(p, measure) {
  s <- lines(p, "strings.txt")
  out <- measure(function() gsub("_", "-", tolower(trimws(s)), fixed = TRUE))
  sum(startsWith(out, "item-")) + sum(nchar(out, type = "bytes"))
})

# -------------------------------------------------------------------- query
# Rows of rows.csv with v >= 10, x = v * w, grouped by (k, g): n, sum(x),
# mean(w), max(v), sorted by sum(x) descending; checksum query_check.

rows_classes <- c(id = "integer", k = "character", g = "integer", v = "numeric", w = "numeric")
read_rows_df <- function(p) read.csv(file.path(p$data, "rows.csv"), colClasses = rows_classes)
read_rows_dt <- function(p) fread(file.path(p$data, "rows.csv"), colClasses = rows_classes)

# groups + sum(n) + sum(s) / 1000 + sum(m) + sum(hi) / 100, plus the
# s-weighted mean position sum(s[i] * i) / sum(s) (0-based i).
query_check <- function(n, s, m, hi) {
  length(n) + sum(as.numeric(n)) + sum(s) / 1000 + sum(m) + sum(hi) / 100 +
    sum(s * (seq_along(s) - 1)) / sum(s)
}

impl("query", "r_dt", function(p, measure) {
  use_data_table()
  dt <- read_rows_dt(p)
  run <- function() {
    d <- dt[v >= 10]
    d[, x := v * w]
    d[, .(n = .N, s = sum(x), m = mean(w), hi = max(v)), by = .(k, g)][order(-s)]
  }
  out <- measure(run)
  query_check(out$n, out$s, out$m, out$hi)
})

# ----------------------------------------------------------------- sortkeys
# Sort the rows of rows.csv by k ascending, then v descending; checksum
# sum(v[i] * i) / n over the sorted rows, i 0-based.

impl("sortkeys", "r_df", function(p, measure) {
  df <- read_rows_df(p)[, c("k", "v")]
  sort_check(measure(function() df[order(df$k, -df$v), ])$v)
})

impl("sortkeys", "r_dt", function(p, measure) {
  use_data_table()
  dt <- read_rows_dt(p)[, .(k, v)]
  sort_check(measure(function() dt[order(k, -v)])$v)
})

# ------------------------------------------------------------------ readcsv
# Read rows.csv (id int, k str, g int, v float, w float) with its column
# types; checksum rows + sum(id) / rows + sum(g) + sum(v) + sum(w) + the total
# length of k.

readcsv_check <- function(d) {
  rows <- nrow(d)
  rows + sum(as.numeric(d$id)) / rows + sum(d$g) + sum(d$v) + sum(d$w) + sum(nchar(d$k, type = "bytes"))
}

impl("readcsv", "r_df", function(p, measure) {
  readcsv_check(measure(function() read_rows_df(p)))
})

impl("readcsv", "r_dt", function(p, measure) {
  use_data_table()
  readcsv_check(measure(function() read_rows_dt(p)))
})

# --------------------------------------------------------------------- gemv
# y = A x for an n x n matrix; checksum sum(y).

impl("gemv", "r_vec", function(p, measure) {
  n <- int(p, "n")
  a <- matrix_rows(f64(p, "gemv_a.f64", n * n), n, n)
  x <- f64(p, "gemv_x.f64", n)
  sum(measure(function() a %*% x))
})

# --------------------------------------------------------------------- gemm
# C = A B for n x n matrices; checksum sum(C[i, j] * i) / n, i 1-based.

impl("gemm", "r_vec", function(p, measure) {
  n <- int(p, "n")
  a <- matrix_rows(f64(p, "gemm_a.f64", n * n), n, n)
  b <- matrix_rows(f64(p, "gemm_b.f64", n * n), n, n)
  c <- measure(function() a %*% b)
  sum(rowSums(c) * seq_len(n)) / n
})

# ---------------------------------------------------------------------- ols
# beta = solve(X'X, X'y); checksum sum(beta[j] * j), j 1-based.

impl("ols", "r_vec", function(p, measure) {
  n <- int(p, "n")
  k <- int(p, "p")
  x <- matrix_rows(f64(p, "ols_x.f64", n * k), n, k)
  y <- f64(p, "ols_y.f64", n)
  beta <- measure(function() solve(crossprod(x), crossprod(x, y)))
  sum(beta * seq_len(k))
})
