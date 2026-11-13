# R benchmark runner.
#
#   Rscript run.R bench=<name> impl=<id> data=<dir> warmup=<n> reps=<n> budget=<s> [param=value ...]
#
# Runs one implementation of one benchmark and prints the TIME and CHECK
# lines that bench.py parses (protocol in src/benchmarks/README.md).

local({
  file_arg <- grep("^--file=", commandArgs(trailingOnly = FALSE), value = TRUE)
  here <- dirname(normalizePath(sub("^--file=", "", file_arg[1])))
  source(file.path(here, "impls.R"))
})

params <- function(args) {
  p <- list()
  for (a in args) {
    eq <- regexpr("=", a, fixed = TRUE)
    if (eq < 1) stop("bad argument (expected key=value): ", a)
    p[[substr(a, 1, eq - 1)]] <- substr(a, eq + 1, nchar(a))
  }
  p
}

now <- function() as.numeric(Sys.time())

# Runs f `warmup` times untimed, then up to `reps` times timed (one TIME line
# each), stopping once `budget` seconds are spent (warmup included, at least
# one timed run). Returns the last result.
measure <- function(p, f) {
  warmup <- as.integer(p$warmup)
  reps <- as.integer(p$reps)
  budget <- as.numeric(p$budget)
  spent <- 0
  out <- NULL
  for (w in seq_len(warmup)) {
    t0 <- now()
    out <- f()
    spent <- spent + (now() - t0)
  }
  for (r in seq_len(reps)) {
    t0 <- now()
    out <- f()
    dt <- now() - t0
    cat(sprintf("TIME\t%s\t%s\t%d\t%.17g\n", p$bench, p$impl, r - 1L, dt))
    spent <- spent + dt
    if (spent > budget) break
  }
  out
}

main <- function() {
  p <- params(commandArgs(trailingOnly = TRUE))
  fn <- IMPLS[[paste(p$bench, p$impl)]]
  if (is.null(fn)) stop("no R implementation ", p$impl, " of ", p$bench)
  value <- fn(p, function(f) measure(p, f))
  cat(sprintf("CHECK\t%s\t%s\t%.17g\n", p$bench, p$impl, as.numeric(value)))
}

main()
