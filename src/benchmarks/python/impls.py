"""Python implementations of the benchmarks: pure Python (py_list), NumPy,
pandas and Polars.

Each implementation is a function (p, measure) -> checksum. It loads its data
(untimed), passes the work as a function of no arguments to measure, and
computes the checksum from the result outside the timed region. What each
benchmark computes and the rules every implementation follows are in
src/benchmarks/README.md; keep this file in step with the Quark and R
implementations.

Libraries are imported inside the implementations that use them, so a
process only loads what it measures.
"""

import array
import csv
import math
from collections import Counter, defaultdict

REGISTRY = {}


def impl(bench, name):
    def register(fn):
        REGISTRY[(bench, name)] = fn
        return fn

    return register


# ---------------------------------------------------------------- loading


def path(p, name):
    return f"{p['data']}/{name}"


def f64_list(p, name, n=-1):
    """The first n doubles of a data file as a Python list (all when n < 0)."""
    a = array.array("d")
    with open(path(p, name), "rb") as f:
        if n < 0:
            a.frombytes(f.read())
        else:
            a.fromfile(f, n)
    return a.tolist()


def f64_np(p, name, n=-1):
    import numpy as np

    return np.fromfile(path(p, name), dtype="<f8", count=n)


def lines(p, name):
    with open(path(p, name), encoding="utf-8") as f:
        return f.read().split("\n")[:-1]


def text(p, name):
    with open(path(p, name), encoding="utf-8") as f:
        return f.read()


def rows(flat, ncols):
    return [flat[i : i + ncols] for i in range(0, len(flat), ncols)]


# ------------------------------------------------------------------- fib


def fib(n):
    return n if n < 2 else fib(n - 1) + fib(n - 2)


@impl("fib", "py_list")
def fib_py(p, measure):
    n = int(p["n"])
    return measure(lambda: fib(n))


# ------------------------------------------------------------ mandelbrot


def mandel_py(n, iters):
    total = 0
    for j in range(n):
        ci = -1.5 + 3.0 * j / n
        for i in range(n):
            cr = -2.0 + 3.0 * i / n
            zr = zi = 0.0
            k = 0
            while k < iters:
                zr2 = zr * zr
                zi2 = zi * zi
                if zr2 + zi2 > 4.0:
                    break
                zi = 2.0 * zr * zi + ci
                zr = zr2 - zi2 + cr
                k += 1
            total += k
    return total


@impl("mandelbrot", "py_list")
def mandel_py_impl(p, measure):
    n, iters = int(p["n"]), int(p["iters"])
    return measure(lambda: mandel_py(n, iters))


def mandel_np(n, iters):
    import numpy as np

    xs = -2.0 + 3.0 * np.arange(n) / n
    ys = -1.5 + 3.0 * np.arange(n) / n
    cr = np.tile(xs, n)
    ci = np.repeat(ys, n)
    zr = np.zeros(n * n)
    zi = np.zeros(n * n)
    total = 0
    # Escaped points overflow to inf and then nan; both compare false.
    with np.errstate(over="ignore", invalid="ignore"):
        for _ in range(iters):
            zr2 = zr * zr
            zi2 = zi * zi
            total += int(np.count_nonzero(zr2 + zi2 <= 4.0))
            zi = 2.0 * zr * zi + ci
            zr = zr2 - zi2 + cr
    return total


@impl("mandelbrot", "numpy")
def mandel_np_impl(p, measure):
    n, iters = int(p["n"]), int(p["iters"])
    return measure(lambda: mandel_np(n, iters))


# ----------------------------------------------------------------- sieve


def sieve_py(n):
    s = [True] * n
    s[0] = s[1] = False
    i = 2
    while i * i < n:
        if s[i]:
            for j in range(i * i, n, i):
                s[j] = False
        i += 1
    return sum(s)


@impl("sieve", "py_list")
def sieve_py_impl(p, measure):
    n = int(p["n"])
    return measure(lambda: sieve_py(n))


def sieve_np(n):
    import numpy as np

    s = np.ones(n, dtype=bool)
    s[:2] = False
    i = 2
    while i * i < n:
        if s[i]:
            s[i * i :: i] = False
        i += 1
    return int(np.count_nonzero(s))


@impl("sieve", "numpy")
def sieve_np_impl(p, measure):
    n = int(p["n"])
    return measure(lambda: sieve_np(n))


# ------------------------------------------------------------------ ewma


def ewma_py(xs, alpha):
    beta = 1.0 - alpha
    it = iter(xs)
    prev = next(it)
    out = [prev]
    for x in it:
        prev = alpha * x + beta * prev
        out.append(prev)
    return out


@impl("ewma", "py_list")
def ewma_py_impl(p, measure):
    xs = f64_list(p, "a.f64", int(p["n"]))
    out = measure(lambda: ewma_py(xs, float(p["alpha"])))
    return sum(out)


def ewma_np(x, alpha):
    # NumPy has no recurrence primitive, so this is a loop over the array.
    import numpy as np

    beta = 1.0 - alpha
    out = np.empty(len(x))
    prev = x[0]
    out[0] = prev
    for i in range(1, len(x)):
        prev = alpha * x[i] + beta * prev
        out[i] = prev
    return out


@impl("ewma", "numpy")
def ewma_np_impl(p, measure):
    x = f64_np(p, "a.f64", int(p["n"]))
    return measure(lambda: ewma_np(x, float(p["alpha"]))).sum()


@impl("ewma", "pandas")
def ewma_pd(p, measure):
    import pandas as pd

    df = pd.DataFrame({"a": f64_np(p, "a.f64", int(p["n"]))})
    alpha = float(p["alpha"])
    return measure(lambda: df["a"].ewm(alpha=alpha, adjust=False).mean()).sum()


@impl("ewma", "polars")
def ewma_pl(p, measure):
    import polars as pl

    df = pl.DataFrame({"a": f64_np(p, "a.f64", int(p["n"]))})
    alpha = float(p["alpha"])
    out = measure(lambda: df.select(pl.col("a").ewm_mean(alpha=alpha, adjust=False)))
    return out["a"].sum()


# ---------------------------------------------------------------- reduce


@impl("reduce", "py_list")
def reduce_py(p, measure):
    xs = f64_list(p, "a.f64", int(p["n"]))

    def run():
        s = sum(xs)
        return s, min(xs), max(xs), s / len(xs)

    return sum(measure(run))


@impl("reduce", "numpy")
def reduce_np(p, measure):
    a = f64_np(p, "a.f64", int(p["n"]))
    return sum(measure(lambda: (a.sum(), a.min(), a.max(), a.mean())))


@impl("reduce", "pandas")
def reduce_pd(p, measure):
    import pandas as pd

    df = pd.DataFrame({"a": f64_np(p, "a.f64", int(p["n"]))})

    def run():
        c = df["a"]
        return c.sum(), c.min(), c.max(), c.mean()

    return sum(measure(run))


@impl("reduce", "polars")
def reduce_pl(p, measure):
    import polars as pl

    df = pl.DataFrame({"a": f64_np(p, "a.f64", int(p["n"]))})
    a = pl.col("a")
    out = measure(
        lambda: df.select(a.sum().alias("s"), a.min().alias("mn"), a.max().alias("mx"), a.mean().alias("m"))
    )
    return sum(out.row(0))


# ----------------------------------------------------------------- arith


def two_cols_list(p):
    n = int(p["n"])
    return f64_list(p, "a.f64", n), f64_list(p, "b.f64", n)


def two_cols_np(p):
    n = int(p["n"])
    return f64_np(p, "a.f64", n), f64_np(p, "b.f64", n)


def two_cols_pd(p):
    import pandas as pd

    a, b = two_cols_np(p)
    return pd.DataFrame({"a": a, "b": b})


def two_cols_pl(p):
    import polars as pl

    a, b = two_cols_np(p)
    return pl.DataFrame({"a": a, "b": b})


@impl("arith", "py_list")
def arith_py(p, measure):
    a, b = two_cols_list(p)
    return sum(measure(lambda: [2.5 * x + y for x, y in zip(a, b)]))


@impl("arith", "numpy")
def arith_np(p, measure):
    a, b = two_cols_np(p)
    return measure(lambda: 2.5 * a + b).sum()


@impl("arith", "pandas")
def arith_pd(p, measure):
    df = two_cols_pd(p)
    return measure(lambda: df.assign(z=2.5 * df["a"] + df["b"]))["z"].sum()


@impl("arith", "polars")
def arith_pl(p, measure):
    import polars as pl

    df = two_cols_pl(p)
    return measure(lambda: df.with_columns(z=2.5 * pl.col("a") + pl.col("b")))["z"].sum()


# ------------------------------------------------------------------ expr
# z = (a*a - b) * 0.5 + (a - b) / (b*b + 1)


@impl("expr", "py_list")
def expr_py(p, measure):
    a, b = two_cols_list(p)
    return sum(measure(lambda: [(x * x - y) * 0.5 + (x - y) / (y * y + 1.0) for x, y in zip(a, b)]))


@impl("expr", "numpy")
def expr_np(p, measure):
    a, b = two_cols_np(p)
    return measure(lambda: (a * a - b) * 0.5 + (a - b) / (b * b + 1.0)).sum()


@impl("expr", "pandas")
def expr_pd(p, measure):
    df = two_cols_pd(p)

    def run():
        a, b = df["a"], df["b"]
        return df.assign(z=(a * a - b) * 0.5 + (a - b) / (b * b + 1.0))

    return measure(run)["z"].sum()


@impl("expr", "polars")
def expr_pl(p, measure):
    import polars as pl

    df = two_cols_pl(p)
    a, b = pl.col("a"), pl.col("b")
    return measure(lambda: df.with_columns(z=(a * a - b) * 0.5 + (a - b) / (b * b + 1.0)))["z"].sum()


# ------------------------------------------------------------------- dot


@impl("dot", "py_list")
def dot_py(p, measure):
    a, b = two_cols_list(p)
    return measure(lambda: sum(x * y for x, y in zip(a, b)))


@impl("dot", "numpy")
def dot_np(p, measure):
    import numpy as np

    a, b = two_cols_np(p)
    return measure(lambda: np.dot(a, b))


@impl("dot", "pandas")
def dot_pd(p, measure):
    df = two_cols_pd(p)
    return measure(lambda: df["a"].dot(df["b"]))


@impl("dot", "polars")
def dot_pl(p, measure):
    import polars as pl

    df = two_cols_pl(p)
    return measure(lambda: df.select(pl.col("a").dot(pl.col("b"))).item())


# ---------------------------------------------------------------- zscore
# z = (a - mean(a)) / sd(a), sd with n - 1; checksum sum(z * b).


@impl("zscore", "py_list")
def zscore_py(p, measure):
    a, b = two_cols_list(p)

    def run():
        n = len(a)
        m = sum(a) / n
        s = math.sqrt(sum((x - m) * (x - m) for x in a) / (n - 1))
        return [(x - m) / s for x in a]

    z = measure(run)
    return sum(x * y for x, y in zip(z, b))


@impl("zscore", "numpy")
def zscore_np(p, measure):
    a, b = two_cols_np(p)
    z = measure(lambda: (a - a.mean()) / a.std(ddof=1))
    return (z * b).sum()


@impl("zscore", "pandas")
def zscore_pd(p, measure):
    df = two_cols_pd(p)

    def run():
        a = df["a"]
        return df.assign(z=(a - a.mean()) / a.std())

    out = measure(run)
    return (out["z"] * out["b"]).sum()


@impl("zscore", "polars")
def zscore_pl(p, measure):
    import polars as pl

    df = two_cols_pl(p)
    a = pl.col("a")
    out = measure(lambda: df.with_columns(z=(a - a.mean()) / a.std()))
    return (out["z"] * out["b"]).sum()


# ---------------------------------------------------------------- filter
# Rows of (a, b) where a > 0.5; checksum count + sum(a) + sum(b) of them.


@impl("filter", "py_list")
def filter_py(p, measure):
    a, b = two_cols_list(p)

    def run():
        fa = [x for x in a if x > 0.5]
        fb = [y for x, y in zip(a, b) if x > 0.5]
        return fa, fb

    fa, fb = measure(run)
    return len(fa) + sum(fa) + sum(fb)


@impl("filter", "numpy")
def filter_np(p, measure):
    a, b = two_cols_np(p)

    def run():
        m = a > 0.5
        return a[m], b[m]

    fa, fb = measure(run)
    return len(fa) + fa.sum() + fb.sum()


@impl("filter", "pandas")
def filter_pd(p, measure):
    df = two_cols_pd(p)
    out = measure(lambda: df[df["a"] > 0.5])
    return len(out) + out["a"].sum() + out["b"].sum()


@impl("filter", "polars")
def filter_pl(p, measure):
    import polars as pl

    df = two_cols_pl(p)
    out = measure(lambda: df.filter(pl.col("a") > 0.5))
    return len(out) + out["a"].sum() + out["b"].sum()


# ------------------------------------------------------------------ sort
# Sort one f64 column ascending; checksum sum(sorted[i] * i) / n.


def sort_check_list(s):
    return sum(x * i for i, x in enumerate(s)) / len(s)


def sort_check_np(s):
    import numpy as np

    return float((s * np.arange(len(s))).sum() / len(s))


@impl("sort", "py_list")
def sort_py(p, measure):
    xs = f64_list(p, "sort.f64", int(p["n"]))
    return sort_check_list(measure(lambda: sorted(xs)))


@impl("sort", "numpy")
def sort_np(p, measure):
    import numpy as np

    x = f64_np(p, "sort.f64", int(p["n"]))
    return sort_check_np(measure(lambda: np.sort(x)))


@impl("sort", "pandas")
def sort_pd(p, measure):
    import pandas as pd

    df = pd.DataFrame({"x": f64_np(p, "sort.f64", int(p["n"]))})
    return sort_check_np(measure(lambda: df.sort_values("x"))["x"].to_numpy())


@impl("sort", "polars")
def sort_pl(p, measure):
    import polars as pl

    df = pl.DataFrame({"x": f64_np(p, "sort.f64", int(p["n"]))})
    return sort_check_np(measure(lambda: df.sort("x"))["x"].to_numpy())


# --------------------------------------------------------------- groupby
# Sum and count of v per string key; checksum sum(sum_k * count_k) / n + groups.


@impl("groupby", "py_list")
def groupby_py(p, measure):
    keys, vals = lines(p, "grp_k.txt"), f64_list(p, "grp_v.f64")

    def run():
        sums, counts = defaultdict(float), defaultdict(int)
        for k, v in zip(keys, vals):
            sums[k] += v
            counts[k] += 1
        return sums, counts

    sums, counts = measure(run)
    return sum(sums[k] * counts[k] for k in sums) / len(keys) + len(sums)


@impl("groupby", "numpy")
def groupby_np(p, measure):
    import numpy as np

    keys, vals = np.array(lines(p, "grp_k.txt")), f64_np(p, "grp_v.f64")

    def run():
        uniq, inv = np.unique(keys, return_inverse=True)
        return uniq, np.bincount(inv, weights=vals), np.bincount(inv)

    uniq, s, c = measure(run)
    return (s * c).sum() / len(keys) + len(uniq)


@impl("groupby", "pandas")
def groupby_pd(p, measure):
    import pandas as pd

    df = pd.DataFrame({"k": lines(p, "grp_k.txt"), "v": f64_np(p, "grp_v.f64")})
    out = measure(lambda: df.groupby("k")["v"].agg(["sum", "count"]))
    return (out["sum"] * out["count"]).sum() / len(df) + len(out)


@impl("groupby", "polars")
def groupby_pl(p, measure):
    import polars as pl

    df = pl.DataFrame({"k": lines(p, "grp_k.txt"), "v": f64_np(p, "grp_v.f64")})
    out = measure(lambda: df.group_by("k").agg(pl.col("v").sum().alias("s"), pl.len().alias("n")))
    return (out["s"] * out["n"]).sum() / len(df) + len(out)


# ------------------------------------------------------------------ join
# Left join of (k, v) to (k, w) on the string key k; checksum sum(v * w).


@impl("join", "py_list")
def join_py(p, measure):
    keys, vals = lines(p, "join_k.txt"), f64_list(p, "join_v.f64")
    dim_k, dim_w = lines(p, "join_dim_k.txt"), f64_list(p, "join_dim_w.f64")

    def run():
        lookup = dict(zip(dim_k, dim_w))
        return [lookup[k] for k in keys]

    w = measure(run)
    return sum(v * x for v, x in zip(vals, w))


@impl("join", "numpy")
def join_np(p, measure):
    import numpy as np

    keys, vals = np.array(lines(p, "join_k.txt")), f64_np(p, "join_v.f64")
    dim_k, dim_w = np.array(lines(p, "join_dim_k.txt")), f64_np(p, "join_dim_w.f64")

    def run():
        # NumPy has no hash join: sort the dimension keys and binary-search.
        order = np.argsort(dim_k)
        sk, sw = dim_k[order], dim_w[order]
        idx = np.searchsorted(sk, keys).clip(max=len(sk) - 1)
        w = sw[idx]
        w[sk[idx] != keys] = np.nan
        return w

    return (vals * measure(run)).sum()


@impl("join", "pandas")
def join_pd(p, measure):
    import pandas as pd

    fact = pd.DataFrame({"k": lines(p, "join_k.txt"), "v": f64_np(p, "join_v.f64")})
    dim = pd.DataFrame({"k": lines(p, "join_dim_k.txt"), "w": f64_np(p, "join_dim_w.f64")})
    out = measure(lambda: fact.merge(dim, on="k", how="left"))
    return (out["v"] * out["w"]).sum()


@impl("join", "polars")
def join_pl(p, measure):
    import polars as pl

    fact = pl.DataFrame({"k": lines(p, "join_k.txt"), "v": f64_np(p, "join_v.f64")})
    dim = pl.DataFrame({"k": lines(p, "join_dim_k.txt"), "w": f64_np(p, "join_dim_w.f64")})
    out = measure(lambda: fact.join(dim, on="k", how="left"))
    return (out["v"] * out["w"]).sum()


# ------------------------------------------------------------- wordcount
# Split a text at single spaces and count each word;
# checksum distinct words + sum(count^2) / words.


@impl("wordcount", "py_list")
def wordcount_py(p, measure):
    t = text(p, "words.txt")
    counts = measure(lambda: Counter(t.split(" ")))
    total = sum(counts.values())
    return len(counts) + sum(c * c for c in counts.values()) / total


@impl("wordcount", "numpy")
def wordcount_np(p, measure):
    import numpy as np

    t = text(p, "words.txt")
    _, c = measure(lambda: np.unique(np.array(t.split(" ")), return_counts=True))
    return len(c) + (c.astype(float) ** 2).sum() / c.sum()


@impl("wordcount", "pandas")
def wordcount_pd(p, measure):
    import pandas as pd

    t = text(p, "words.txt")
    c = measure(lambda: pd.Series(t.split(" ")).value_counts()).to_numpy().astype(float)
    return len(c) + (c**2).sum() / c.sum()


@impl("wordcount", "polars")
def wordcount_pl(p, measure):
    import polars as pl

    s = pl.Series("t", [text(p, "words.txt")])
    out = measure(lambda: s.str.split(" ").explode().value_counts())
    c = out["count"].cast(pl.Float64)
    return len(c) + (c**2).sum() / c.sum()


# --------------------------------------------------------------- strings
# s.trim().lower() with '_' replaced by '-'; checksum
# count(starts with 'item-') + total length of the results.


@impl("strings", "py_list")
def strings_py(p, measure):
    xs = lines(p, "strings.txt")
    out = measure(lambda: [s.strip().lower().replace("_", "-") for s in xs])
    return sum(1 for s in out if s.startswith("item-")) + sum(len(s) for s in out)


@impl("strings", "numpy")
def strings_np(p, measure):
    import numpy as np

    arr = np.array(lines(p, "strings.txt"), dtype=np.dtypes.StringDType())
    out = measure(lambda: np.strings.replace(np.strings.lower(np.strings.strip(arr)), "_", "-"))
    return int(np.strings.startswith(out, "item-").sum()) + int(np.strings.str_len(out).sum())


@impl("strings", "pandas")
def strings_pd(p, measure):
    import pandas as pd

    df = pd.DataFrame({"s": lines(p, "strings.txt")})
    out = measure(lambda: df["s"].str.strip().str.lower().str.replace("_", "-", regex=False))
    return int(out.str.startswith("item-").sum()) + int(out.str.len().sum())


@impl("strings", "polars")
def strings_pl(p, measure):
    import polars as pl

    df = pl.DataFrame({"s": lines(p, "strings.txt")})
    expr = pl.col("s").str.strip_chars().str.to_lowercase().str.replace_all("_", "-", literal=True)
    out = measure(lambda: df.select(expr))["s"]
    return int(out.str.starts_with("item-").sum()) + int(out.str.len_bytes().sum())


# ----------------------------------------------------------------- query
# Rows of rows.csv with v >= 10, x = v * w, grouped by (k, g): n, sum(x),
# mean(w), max(v), sorted by sum(x) descending; checksum query_check.

ROWS_DTYPES = {"id": "int64", "k": "str", "g": "int64", "v": "float64", "w": "float64"}


def read_rows_lists(file):
    """A CSV file of rows.csv's columns as five lists (id, k, g, v, w)."""
    ids, ks, gs, vs, ws = [], [], [], [], []
    with open(file, newline="", encoding="utf-8") as f:
        r = csv.reader(f)
        next(r)
        for i, k, g, v, w in r:
            ids.append(int(i))
            ks.append(k)
            gs.append(int(g))
            vs.append(float(v))
            ws.append(float(w))
    return ids, ks, gs, vs, ws


def read_rows_pd(p):
    import pandas as pd

    return pd.read_csv(path(p, "rows.csv"), dtype=ROWS_DTYPES)


def read_rows_pl(p):
    import polars as pl

    schema = {"id": pl.Int64, "k": pl.String, "g": pl.Int64, "v": pl.Float64, "w": pl.Float64}
    return pl.read_csv(path(p, "rows.csv"), schema=schema)


def query_check(n, s, m, hi):
    """groups + sum(n) + sum(s) / 1000 + sum(m) + sum(hi) / 100, plus the
    s-weighted mean position sum(s[i] * i) / sum(s), which a wrong order
    changes."""
    total_s = sum(s)
    weighted = sum(x * i for i, x in enumerate(s))
    return len(n) + sum(n) + total_s / 1000 + sum(m) + sum(hi) / 100 + weighted / total_s


@impl("query", "py_list")
def query_py(p, measure):
    _, ks, gs, vs, ws = read_rows_lists(path(p, "rows.csv"))

    def run():
        acc = {}
        for k, g, v, w in zip(ks, gs, vs, ws):
            if v >= 10.0:
                a = acc.get((k, g))
                if a is None:
                    acc[(k, g)] = [1, v * w, w, v]
                else:
                    a[0] += 1
                    a[1] += v * w
                    a[2] += w
                    if v > a[3]:
                        a[3] = v
        groups = sorted(acc.items(), key=lambda kv: -kv[1][1])
        return [(k, g, a[0], a[1], a[2] / a[0], a[3]) for (k, g), a in groups]

    out = measure(run)
    return query_check([r[2] for r in out], [r[3] for r in out], [r[4] for r in out], [r[5] for r in out])


@impl("query", "pandas")
def query_pd(p, measure):
    df = read_rows_pd(p)

    def run():
        d = df[df["v"] >= 10.0]
        d = d.assign(x=d["v"] * d["w"])
        out = d.groupby(["k", "g"], as_index=False).agg(
            n=("v", "size"), s=("x", "sum"), m=("w", "mean"), hi=("v", "max")
        )
        return out.sort_values("s", ascending=False)

    out = measure(run)
    return query_check(*(out[c].tolist() for c in ("n", "s", "m", "hi")))


@impl("query", "polars")
def query_pl(p, measure):
    import polars as pl

    df = read_rows_pl(p)
    v, w = pl.col("v"), pl.col("w")

    def run():
        return (
            df.filter(v >= 10.0)
            .with_columns(x=v * w)
            .group_by("k", "g")
            .agg(n=pl.len(), s=pl.col("x").sum(), m=w.mean(), hi=v.max())
            .sort("s", descending=True)
        )

    out = measure(run)
    return query_check(*(out[c].to_list() for c in ("n", "s", "m", "hi")))


# -------------------------------------------------------------- sortkeys
# Sort the rows of rows.csv by k ascending, then v descending; checksum
# sum(v[i] * i) / n over the sorted rows.


@impl("sortkeys", "py_list")
def sortkeys_py(p, measure):
    _, ks, _, vs, _ = read_rows_lists(path(p, "rows.csv"))

    def run():
        order = sorted(range(len(ks)), key=lambda i: (ks[i], -vs[i]))
        return [ks[i] for i in order], [vs[i] for i in order]

    return sort_check_list(measure(run)[1])


@impl("sortkeys", "numpy")
def sortkeys_np(p, measure):
    import numpy as np

    _, ks, _, vs, _ = read_rows_lists(path(p, "rows.csv"))
    k, v = np.array(ks), np.array(vs)

    def run():
        o = np.lexsort((-v, k))  # the last key is the primary one
        return k[o], v[o]

    return sort_check_np(measure(run)[1])


@impl("sortkeys", "pandas")
def sortkeys_pd(p, measure):
    df = read_rows_pd(p)[["k", "v"]]
    out = measure(lambda: df.sort_values(["k", "v"], ascending=[True, False]))
    return sort_check_np(out["v"].to_numpy())


@impl("sortkeys", "polars")
def sortkeys_pl(p, measure):
    df = read_rows_pl(p).select("k", "v")
    out = measure(lambda: df.sort(["k", "v"], descending=[False, True]))
    return sort_check_np(out["v"].to_numpy())


# --------------------------------------------------------------- readcsv
# Read rows.csv (id int, k str, g int, v float, w float) with its column
# types; checksum rows + sum(id) / rows + sum(g) + sum(v) + sum(w) + the total
# length of k.


def readcsv_check(rows, ids, ks, gs, vs, ws):
    return rows + ids / rows + gs + vs + ws + ks


@impl("readcsv", "py_list")
def readcsv_py(p, measure):
    file = path(p, "rows.csv")
    ids, ks, gs, vs, ws = measure(lambda: read_rows_lists(file))
    return readcsv_check(len(ids), sum(ids), sum(len(k) for k in ks), sum(gs), sum(vs), sum(ws))


@impl("readcsv", "pandas")
def readcsv_pd(p, measure):
    df = measure(lambda: read_rows_pd(p))
    return readcsv_check(len(df), df["id"].sum(), df["k"].str.len().sum(), df["g"].sum(), df["v"].sum(),
                         df["w"].sum())


@impl("readcsv", "polars")
def readcsv_pl(p, measure):
    df = measure(lambda: read_rows_pl(p))
    return readcsv_check(len(df), df["id"].sum(), df["k"].str.len_bytes().sum(), df["g"].sum(), df["v"].sum(),
                         df["w"].sum())


# ------------------------------------------------------------------ gemv
# y = A x for an n x n matrix; checksum sum(y).


@impl("gemv", "py_list")
def gemv_py(p, measure):
    n = int(p["n"])
    a = rows(f64_list(p, "gemv_a.f64", n * n), n)
    x = f64_list(p, "gemv_x.f64", n)
    return sum(measure(lambda: [sum(r * v for r, v in zip(row, x)) for row in a]))


@impl("gemv", "numpy")
def gemv_np(p, measure):
    n = int(p["n"])
    a = f64_np(p, "gemv_a.f64", n * n).reshape(n, n)
    x = f64_np(p, "gemv_x.f64", n)
    return measure(lambda: a @ x).sum()


# ------------------------------------------------------------------ gemm
# C = A B for n x n matrices; checksum sum(C[i][j] * (i + 1)) / n.


def gemm_py(a, b, n):
    c = []
    for i in range(n):
        ai = a[i]
        ci = [0.0] * n
        for k in range(n):
            aik = ai[k]
            bk = b[k]
            for j in range(n):
                ci[j] += aik * bk[j]
        c.append(ci)
    return c


@impl("gemm", "py_list")
def gemm_py_impl(p, measure):
    n = int(p["n"])
    a = rows(f64_list(p, "gemm_a.f64", n * n), n)
    b = rows(f64_list(p, "gemm_b.f64", n * n), n)
    c = measure(lambda: gemm_py(a, b, n))
    return sum(sum(row) * (i + 1) for i, row in enumerate(c)) / n


@impl("gemm", "numpy")
def gemm_np(p, measure):
    import numpy as np

    n = int(p["n"])
    a = f64_np(p, "gemm_a.f64", n * n).reshape(n, n)
    b = f64_np(p, "gemm_b.f64", n * n).reshape(n, n)
    c = measure(lambda: a @ b)
    return (c.sum(axis=1) * np.arange(1, n + 1)).sum() / n


# ------------------------------------------------------------------- ols
# beta = solve(X'X, X'y) for X with n rows and p columns;
# checksum sum(beta[j] * (j + 1)).


def solve(a, b):
    """Gaussian elimination with partial pivoting on copies of a and b."""
    n = len(b)
    m = [row[:] + [b[i]] for i, row in enumerate(a)]
    for col in range(n):
        piv = max(range(col, n), key=lambda r: abs(m[r][col]))
        m[col], m[piv] = m[piv], m[col]
        for r in range(col + 1, n):
            f = m[r][col] / m[col][col]
            for c in range(col, n + 1):
                m[r][c] -= f * m[col][c]
    x = [0.0] * n
    for r in range(n - 1, -1, -1):
        s = m[r][n]
        for c in range(r + 1, n):
            s -= m[r][c] * x[c]
        x[r] = s / m[r][r]
    return x


def ols_py(x, y, p):
    xtx = [[0.0] * p for _ in range(p)]
    xty = [0.0] * p
    for row, yi in zip(x, y):
        for i in range(p):
            ri = row[i]
            xty[i] += ri * yi
            xi = xtx[i]
            for j in range(i, p):
                xi[j] += ri * row[j]
    for i in range(p):
        for j in range(i):
            xtx[i][j] = xtx[j][i]
    return solve(xtx, xty)


def ols_check(beta):
    return sum(b * (j + 1) for j, b in enumerate(beta))


@impl("ols", "py_list")
def ols_py_impl(p, measure):
    n, k = int(p["n"]), int(p["p"])
    x = rows(f64_list(p, "ols_x.f64", n * k), k)
    y = f64_list(p, "ols_y.f64", n)
    return ols_check(measure(lambda: ols_py(x, y, k)))


@impl("ols", "numpy")
def ols_np(p, measure):
    import numpy as np

    n, k = int(p["n"]), int(p["p"])
    x = f64_np(p, "ols_x.f64", n * k).reshape(n, k)
    y = f64_np(p, "ols_y.f64", n)
    return ols_check(measure(lambda: np.linalg.solve(x.T @ x, x.T @ y)).tolist())
