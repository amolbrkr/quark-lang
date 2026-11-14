"""Generates the benchmark input files (run by bench.py inside the venv).

Every implementation reads the same files, so they all work on identical
values and their checksums can be compared. The files are deterministic for
a given seed and set of sizes:

    *.f64   raw little-endian IEEE doubles, no header
    *.txt   UTF-8 text, one item per line (words.txt is one line of words
            separated by single spaces)
    *.csv   the same values as a CSV file with a header, for the table
            implementations (Quark tables are read from CSV) and the csv
            benchmark; floats in their shortest round-trip form, so a
            correct parser reads back the exact doubles

Usage: python datagen.py <out_dir> <sizes_json>
"""

import json
import os
import sys

import numpy as np

# Bump when the content of a file changes for the same sizes, so bench.py
# regenerates data written by an older version.
VERSION = 2
SEED = 20261113


def write_f64(path, values):
    np.ascontiguousarray(values, dtype="<f8").tofile(path)


def write_lines(path, lines):
    with open(path, "w", encoding="utf-8", newline="\n") as f:
        f.write("\n".join(lines))
        f.write("\n")


def text_column(values, kind):
    if kind == "float":
        return map(repr, values.tolist())  # shortest round-trip form
    if kind == "int":
        return map(str, values.tolist())
    if kind == "quoted":
        return ('"' + v + '"' for v in values)  # keeps the padding spaces
    return iter(values)


def write_csv(path, columns):
    """columns: (name, values, kind) with kind float, int, str or quoted.
    Written in chunks so the large preset doesn't build every line at once."""
    n = len(columns[0][1])
    step = 1_000_000
    with open(path, "w", encoding="utf-8", newline="\n") as f:
        f.write(",".join(name for name, _, _ in columns) + "\n")
        for lo in range(0, n, step):
            parts = [text_column(values[lo : lo + step], kind) for _, values, kind in columns]
            f.write("\n".join(map(",".join, zip(*parts))))
            f.write("\n")


def vocabulary(rng, n):
    """n distinct lowercase pseudo-words of 2-9 letters."""
    letters = np.array(list("abcdefghijklmnopqrstuvwxyz"))
    words = set()
    while len(words) < n:
        length = int(rng.integers(2, 10))
        words.add("".join(rng.choice(letters, length)))
    return sorted(words)


def zipf_weights(n, s=1.1):
    w = 1.0 / np.arange(1, n + 1) ** s
    return w / w.sum()


def generate(out, z):
    rng = np.random.default_rng(SEED)
    os.makedirs(out, exist_ok=True)

    # Columns that only the CSV files have come from their own generator, so
    # the other files are the same as before they were added.
    extra = np.random.default_rng(SEED + 1)

    # Two uniform [0, 1) columns for the vector benchmarks.
    a, b = rng.random(z["vec_n"]), rng.random(z["vec_n"])
    write_f64(f"{out}/a.f64", a)
    write_f64(f"{out}/b.f64", b)
    write_csv(f"{out}/cols.csv", [("a", a, "float"), ("b", b, "float")])

    # Standard normal values to sort.
    x = rng.standard_normal(z["sort_n"])
    write_f64(f"{out}/sort.f64", x)
    write_csv(f"{out}/sort.csv", [("x", x, "float")])

    # Group-by: string keys drawn uniformly from grp_k distinct values.
    width = len(str(z["grp_k"] - 1))
    keys = np.array([f"id{i:0{width}d}" for i in range(z["grp_k"])])
    k = keys[rng.integers(0, z["grp_k"], z["grp_n"])]
    v = rng.random(z["grp_n"]) * 100.0
    write_lines(f"{out}/grp_k.txt", k)
    write_f64(f"{out}/grp_v.f64", v)

    # A table of grp_n rows for groupby (k, v), query, sortkeys and csv: an
    # id, the group-by key and value, a small int g in 0..9 and a uniform
    # [0, 1) weight w.
    n = z["grp_n"]
    write_csv(f"{out}/rows.csv", [
        ("id", np.arange(n), "int"), ("k", k, "str"), ("g", extra.integers(0, 10, n), "int"),
        ("v", v, "float"), ("w", extra.random(n), "float"),
    ])

    # Join: a dimension table of join_k distinct keys (shuffled) with a
    # weight, and a fact table whose keys all occur in it.
    width = len(str(z["join_k"] - 1))
    dim = np.array([f"k{i:0{width}d}" for i in range(z["join_k"])])
    rng.shuffle(dim)
    write_lines(f"{out}/join_dim_k.txt", dim)
    write_f64(f"{out}/join_dim_w.f64", rng.random(z["join_k"]))
    write_lines(f"{out}/join_k.txt", dim[rng.integers(0, z["join_k"], z["join_n"])])
    write_f64(f"{out}/join_v.f64", rng.random(z["join_n"]))

    # Word count: words_n words from a Zipf-distributed vocabulary, one line.
    vocab = np.array(vocabulary(rng, z["vocab"]))
    picks = rng.choice(len(vocab), z["words_n"], p=zipf_weights(len(vocab)))
    with open(f"{out}/words.txt", "w", encoding="utf-8", newline="\n") as f:
        f.write(" ".join(vocab[picks]))

    # String cleaning: padded, mixed-case labels with underscores, such as
    # "  ITEM_4821_Lorem ". About 3 in 5 start with item/ITEM/Item.
    prefixes = np.array(["Item", "ITEM", "item", "Product", "PRODUCT"])
    pads = np.array(["", " ", "  "])
    n = z["str_n"]
    pre = prefixes[rng.integers(0, len(prefixes), n)]
    num = rng.integers(0, 100_000, n)
    word = vocab[rng.integers(0, len(vocab), n)]
    upper = rng.random(n) < 0.5
    left = pads[rng.integers(0, len(pads), n)]
    right = pads[rng.integers(0, len(pads), n)]
    lines = [
        f"{l}{p}_{k}_{w.capitalize() if u else w}{r}"
        for l, p, k, w, u, r in zip(left, pre, num, word, upper, right)
    ]
    write_lines(f"{out}/strings.txt", lines)
    write_csv(f"{out}/strings.csv", [("s", lines, "quoted")])

    # Matrices, row-major.
    write_f64(f"{out}/gemv_a.f64", rng.random(z["gemv_n"] * z["gemv_n"]))
    write_f64(f"{out}/gemv_x.f64", rng.random(z["gemv_n"]))
    write_f64(f"{out}/gemm_a.f64", rng.random(z["gemm_n"] * z["gemm_n"]))
    write_f64(f"{out}/gemm_b.f64", rng.random(z["gemm_n"] * z["gemm_n"]))

    # Least squares: y = X beta + noise, X standard normal (rows x p).
    p = z["ols_p"]
    x = rng.standard_normal((z["ols_n"], p))
    beta = np.array([(j + 1) * (-1) ** j / 2 for j in range(p)])
    y = x @ beta + 0.1 * rng.standard_normal(z["ols_n"])
    write_f64(f"{out}/ols_x.f64", x)
    write_f64(f"{out}/ols_y.f64", y)


def main():
    out, sizes = sys.argv[1], json.loads(sys.argv[2])
    generate(out, sizes)
    with open(f"{out}/manifest.json", "w") as f:
        json.dump({"version": VERSION, "seed": SEED, "sizes": sizes}, f, indent=2, sort_keys=True)


if __name__ == "__main__":
    main()
