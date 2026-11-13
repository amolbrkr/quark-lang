#!/usr/bin/env python3
"""Quark benchmark suite driver (stdlib only).

    python3 bench.py setup                 create .venv with requirements.txt
    python3 bench.py list                  list benchmarks and implementations
    python3 bench.py run [options]         generate data, build, run, report
    python3 bench.py report <results-dir>  rebuild report.md from results.json

See README.md for what each benchmark measures and how to read the report.
"""

import argparse
import csv
import datetime
import json
import os
import platform
import shutil
import statistics
import subprocess
import sys
import tempfile
import time
from dataclasses import dataclass, field

HERE = os.path.dirname(os.path.abspath(__file__))
REPO = os.path.abspath(os.path.join(HERE, "..", ".."))
COMPILER_DIR = os.path.join(REPO, "src", "core", "quark")
STDLIB = os.path.join(REPO, "src", "core", "stdlib")
VENV = os.path.join(HERE, ".venv")
IS_WIN = os.name == "nt"
EXE = ".exe" if IS_WIN else ""
VENV_PY = os.path.join(VENV, "Scripts", "python.exe") if IS_WIN else os.path.join(VENV, "bin", "python")

# ----------------------------------------------------------------- presets
# Input sizes. "default" is the reference configuration; "small" checks that
# every implementation runs and agrees in under a minute; "large" stresses
# memory (several GB for the list implementations).

PRESETS = {
    "small": dict(
        vec_n=100_000, sort_n=20_000, grp_n=20_000, grp_k=100, join_n=20_000, join_k=2_000,
        words_n=20_000, vocab=2_000, str_n=20_000, fib_n=20, sieve_n=200_000,
        mandel_n=60, mandel_iters=50, gemv_n=200, gemm_n=40, ols_n=5_000, ols_p=4,
    ),
    "default": dict(
        vec_n=10_000_000, sort_n=1_000_000, grp_n=1_000_000, grp_k=1_000, join_n=1_000_000,
        join_k=100_000, words_n=1_000_000, vocab=20_000, str_n=1_000_000, fib_n=30,
        sieve_n=10_000_000, mandel_n=400, mandel_iters=100, gemv_n=2_000, gemm_n=300,
        ols_n=500_000, ols_p=8,
    ),
    "large": dict(
        vec_n=30_000_000, sort_n=10_000_000, grp_n=10_000_000, grp_k=10_000, join_n=10_000_000,
        join_k=1_000_000, words_n=10_000_000, vocab=50_000, str_n=5_000_000, fib_n=32,
        sieve_n=50_000_000, mandel_n=1_000, mandel_iters=100, gemv_n=5_000, gemm_n=1_000,
        ols_n=5_000_000, ols_p=16,
    ),
}

# ----------------------------------------------------------- implementations


@dataclass
class Impl:
    id: str
    lang: str  # quark, python, r
    tier: str  # list, vector, blas, table
    label: str


IMPLS = [
    Impl("quark_list", "quark", "list", "Quark list"),
    Impl("quark_vec", "quark", "vector", "Quark vec"),
    Impl("quark_blas", "quark", "blas", "Quark vec+blas"),
    Impl("quark_tbl", "quark", "table", "Quark tbl"),
    Impl("py_list", "python", "list", "Python"),
    Impl("numpy", "python", "vector", "NumPy"),
    Impl("pandas", "python", "table", "pandas"),
    Impl("polars", "python", "table", "Polars"),
    Impl("r_loop", "r", "list", "R loop"),
    Impl("r_vec", "r", "vector", "R vector"),
    Impl("r_df", "r", "table", "R data.frame"),
    Impl("r_dt", "r", "table", "R data.table"),
]
IMPL = {i.id: i for i in IMPLS}
LANGS = ["quark", "python", "r"]


@dataclass
class Bench:
    name: str
    group: str
    title: str
    params: object  # sizes -> dict of params
    impls: list
    planned: list = field(default_factory=list)  # implementations that don't exist yet
    rtol: float = 1e-9


ALL_VEC = ["quark_list", "quark_vec", "py_list", "numpy", "pandas", "polars", "r_vec"]

BENCHMARKS = [
    # Plain code: loops, calls, scalar arithmetic.
    Bench("fib", "core", "recursive Fibonacci (function calls)",
          lambda z: {"n": z["fib_n"]}, ["quark_list", "py_list", "r_loop"], rtol=0),
    Bench("mandelbrot", "core", "escape-time Mandelbrot over a grid (float loops)",
          lambda z: {"n": z["mandel_n"], "iters": z["mandel_iters"]},
          ["quark_list", "quark_vec", "py_list", "numpy", "r_loop", "r_vec"], rtol=1e-6),
    Bench("sieve", "core", "primes below n (list index reads and writes)",
          lambda z: {"n": z["sieve_n"]}, ["quark_list", "py_list", "numpy", "r_loop", "r_vec"], rtol=0),
    Bench("ewma", "core", "exponentially weighted moving average (sequential recurrence)",
          lambda z: {"n": z["vec_n"], "alpha": 0.05},
          ["quark_list", "quark_vec", "py_list", "numpy", "pandas", "polars", "r_loop", "r_vec"]),
    # Whole-column numeric work.
    Bench("reduce", "vector", "sum, min, max, mean of a column",
          lambda z: {"n": z["vec_n"]}, ALL_VEC + ["r_dt"], planned=["quark_tbl"]),
    Bench("arith", "vector", "z = 2.5*a + b",
          lambda z: {"n": z["vec_n"]}, ALL_VEC + ["quark_blas", "r_df", "r_dt"], planned=["quark_tbl"]),
    Bench("expr", "vector", "z = (a*a - b)*0.5 + (a - b)/(b*b + 1)",
          lambda z: {"n": z["vec_n"]}, ALL_VEC + ["r_dt"], planned=["quark_tbl"]),
    Bench("dot", "vector", "dot product of two columns",
          lambda z: {"n": z["vec_n"]}, ALL_VEC + ["quark_blas"]),
    Bench("zscore", "vector", "standardize a column: (a - mean) / sd",
          lambda z: {"n": z["vec_n"]}, ALL_VEC + ["quark_blas", "r_df", "r_dt"], planned=["quark_tbl"]),
    Bench("filter", "vector", "rows of (a, b) where a > 0.5",
          lambda z: {"n": z["vec_n"]}, ALL_VEC + ["r_df", "r_dt"], planned=["quark_tbl"]),
    Bench("sort", "vector", "sort a column of normal floats",
          lambda z: {"n": z["sort_n"]}, ALL_VEC + ["r_df", "r_dt"], planned=["quark_tbl"]),
    # Hashing: grouping, joins, strings.
    Bench("groupby", "table", "sum and count per string key",
          lambda z: {"n": z["grp_n"], "k": z["grp_k"]},
          ["quark_list", "py_list", "numpy", "pandas", "polars", "r_vec", "r_df", "r_dt"],
          planned=["quark_tbl"]),
    Bench("join", "table", "left join on a string key, then sum(v * w)",
          lambda z: {"n": z["join_n"], "k": z["join_k"]},
          ["quark_list", "py_list", "numpy", "pandas", "polars", "r_vec", "r_df", "r_dt"],
          planned=["quark_tbl"]),
    Bench("wordcount", "table", "split a text into words and count them",
          lambda z: {"n": z["words_n"], "vocab": z["vocab"]},
          ["quark_list", "py_list", "numpy", "pandas", "polars", "r_vec", "r_dt"], planned=["quark_tbl"]),
    Bench("strings", "table", "trim, lowercase and replace in string labels",
          lambda z: {"n": z["str_n"]}, ["quark_list", "py_list", "numpy", "pandas", "polars", "r_vec"],
          planned=["quark_tbl"]),
    # Linear algebra.
    Bench("gemv", "matrix", "matrix-vector product",
          lambda z: {"n": z["gemv_n"]}, ["quark_list", "quark_vec", "quark_blas", "py_list", "numpy", "r_vec"]),
    Bench("gemm", "matrix", "matrix-matrix product",
          lambda z: {"n": z["gemm_n"]}, ["quark_list", "quark_vec", "quark_blas", "py_list", "numpy", "r_vec"]),
    Bench("ols", "matrix", "least squares via the normal equations",
          lambda z: {"n": z["ols_n"], "p": z["ols_p"]},
          ["quark_list", "quark_vec", "quark_blas", "py_list", "numpy", "r_vec"]),
]
BENCH = {b.name: b for b in BENCHMARKS}

# Implementations whose checksum others are compared with, in order of
# preference.
REFERENCE_ORDER = ["numpy", "py_list", "r_vec", "r_loop", "quark_list"]

# --------------------------------------------------------------- utilities


def r_env(env=None):
    """env without R_HOME: R ignores a set R_HOME that doesn't match its
    installation, but its front end prints a warning to stdout about it."""
    env = dict(os.environ if env is None else env)
    env.pop("R_HOME", None)
    return env


def find_rscript():
    """Rscript from $BENCH_RSCRIPT, the PATH, or (Windows) the newest R under
    Program Files, which the R installer doesn't put on the PATH."""
    if os.environ.get("BENCH_RSCRIPT"):
        return os.environ["BENCH_RSCRIPT"]
    found = shutil.which("Rscript")
    if found or not IS_WIN:
        return found
    import glob
    import re
    hits = []
    for root in {os.environ.get("ProgramFiles", r"C:\Program Files"), os.environ.get("ProgramW6432", "")}:
        if root:
            hits += glob.glob(os.path.join(root, "R", "R-*", "bin", "Rscript.exe"))

    def version(path):
        return [int(x) for x in re.findall(r"\d+", os.path.basename(os.path.dirname(os.path.dirname(path))))]

    return max(hits, key=version) if hits else None


RSCRIPT = find_rscript()


def sh(cmd, **kw):
    """Output of a command, or '' when it fails or doesn't exist."""
    if cmd and cmd[0] and cmd[0] == RSCRIPT:
        kw.setdefault("env", r_env())
    try:
        return subprocess.run(cmd, capture_output=True, text=True, timeout=60, **kw).stdout.strip()
    except (OSError, subprocess.SubprocessError):
        return ""


def first_line(s):
    return s.splitlines()[0] if s else ""


def thread_env(threads):
    """Environment that pins every library's thread pool to `threads`
    (0 leaves each library at its default)."""
    env = dict(os.environ)
    env["BENCH_THREADS"] = str(threads)
    if threads > 0:
        t = str(threads)
        for var in ("OMP_NUM_THREADS", "OPENBLAS_NUM_THREADS", "MKL_NUM_THREADS", "POLARS_MAX_THREADS",
                    "R_DATATABLE_NUM_THREADS", "GC_MARKERS"):
            env[var] = t
    return env


def fmt_time(s):
    if s is None:
        return ""
    if s >= 10:
        return f"{s:.1f} s"
    if s >= 1:
        return f"{s:.2f} s"
    ms = s * 1000
    if ms >= 100:
        return f"{ms:.0f} ms"
    if ms >= 10:
        return f"{ms:.1f} ms"
    if ms >= 1:
        return f"{ms:.2f} ms"
    return f"{ms * 1000:.0f} µs"


# -------------------------------------------------------------- environment


def tool_status(r_needed):
    """Which languages and optional pieces can run here."""
    st = {}
    st["clang"] = shutil.which("clang++") is not None
    st["go"] = shutil.which("go") is not None
    st["python"] = os.path.exists(VENV_PY)
    st["python_libs"] = {}
    if st["python"]:
        out = sh([VENV_PY, "-c", "import importlib.util as u\n"
                  "for m in ('numpy','pandas','polars','pyarrow'): print(m, u.find_spec(m) is not None)"])
        for line in out.splitlines():
            m, ok = line.split()
            st["python_libs"][m] = ok == "True"
    st["r"] = RSCRIPT is not None
    st["data.table"] = False
    if st["r"] and r_needed:
        st["data.table"] = sh([RSCRIPT, "-e", "cat(requireNamespace('data.table', quietly=TRUE))"]) == "TRUE"
    import ctypes.util
    st["openblas"] = ctypes.util.find_library("openblas") is not None or bool(system_openblas())
    return st


def impl_available(impl_id, st):
    lang = IMPL[impl_id].lang
    if lang == "quark":
        return st["clang"] and st["go"]
    if lang == "python":
        need = {"numpy": "numpy", "pandas": "pandas", "polars": "polars"}.get(impl_id)
        return st["python"] and (need is None or st["python_libs"].get(need, False))
    if lang == "r":
        return st["r"] and (impl_id != "r_dt" or st["data.table"])
    return False


def system_openblas():
    code = ("import ctypes, ctypes.util\n"
            "for name in ('libopenblas.so.0', 'libopenblas.so', ctypes.util.find_library('openblas')):\n"
            "    try:\n"
            "        lib = ctypes.CDLL(name)\n"
            "    except (OSError, TypeError):\n"
            "        continue\n"
            "    lib.openblas_get_config.restype = ctypes.c_char_p\n"
            "    print(lib.openblas_get_config().decode()); break\n")
    return sh([sys.executable, "-c", code])


def environment(quark_exe, threads):
    env = {}
    env["date"] = datetime.datetime.now().isoformat(timespec="seconds")
    commit = sh(["git", "-C", REPO, "rev-parse", "--short", "HEAD"])
    dirty = sh(["git", "-C", REPO, "status", "--porcelain"])
    env["commit"] = commit + (" (uncommitted changes)" if dirty else "")
    env["quark"] = quark_exe
    cpu = ""
    try:
        with open("/proc/cpuinfo") as f:
            for line in f:
                if line.startswith("model name"):
                    cpu = line.split(":", 1)[1].strip()
                    break
    except OSError:
        cpu = sh(["sysctl", "-n", "machdep.cpu.brand_string"]) or platform.processor()
    env["cpu"] = cpu
    env["cpus"] = os.cpu_count()
    try:
        with open("/proc/meminfo") as f:
            kb = int(f.readline().split()[1])
            env["memory_gb"] = round(kb / 1024 / 1024, 1)
    except (OSError, ValueError, IndexError):
        mem = sh(["sysctl", "-n", "hw.memsize"])
        if mem.isdigit():
            env["memory_gb"] = round(int(mem) / 1024 ** 3, 1)
    env["os"] = f"{platform.system()} {platform.release()}"
    if shutil.which("systemd-detect-virt"):
        vm = sh(["systemd-detect-virt", "--vm"])
        container = sh(["systemd-detect-virt", "--container"])
        env["virtualized"] = ", ".join(x for x in (vm, container) if x and x != "none") or "bare metal"
    else:
        env["virtualized"] = "unknown"
    env["threads"] = threads
    env["clang"] = first_line(sh(["clang++", "--version"]))
    env["go"] = sh(["go", "version"])
    env["system_openblas"] = system_openblas()
    if os.path.exists(VENV_PY):
        env["python"] = sh([VENV_PY, "-c", "import platform; print(platform.python_version())"])
        env["python_libs"] = sh([VENV_PY, "-c",
                                 "import importlib.metadata as m\n"
                                 "print(', '.join(f'{p} {m.version(p)}' for p in "
                                 "('numpy','pandas','polars','pyarrow')))"])
        env["numpy_blas"] = sh([VENV_PY, "-c",
                                "import numpy as np\n"
                                "c = np.show_config(mode='dicts')['Build Dependencies']['blas']\n"
                                "print(c.get('name'), c.get('version'))"])
    if RSCRIPT:
        env["r"] = sh([RSCRIPT, "-e", "cat(R.version.string)"])
        env["r_blas"] = sh([RSCRIPT, "-e", "cat(extSoftVersion()[['BLAS']])"])
        env["data.table"] = sh([RSCRIPT, "-e", "cat(as.character(packageVersion('data.table')))"])
    env["load_average_start"] = load_average()
    return env


def load_average():
    """The 1, 5 and 15 minute load averages, or None where the OS has none
    (Windows)."""
    try:
        return [round(x, 2) for x in os.getloadavg()]
    except (AttributeError, OSError):
        return None


# ----------------------------------------------------------------- datagen


def ensure_data(preset, sizes, force=False):
    out = os.path.join(HERE, "data", preset)
    manifest = os.path.join(out, "manifest.json")
    version = None
    with open(os.path.join(HERE, "datagen.py")) as f:
        for line in f:
            if line.startswith("VERSION ="):
                version = int(line.split("=")[1])
    if not force and os.path.exists(manifest):
        with open(manifest) as f:
            m = json.load(f)
        if m.get("sizes") == sizes and m.get("version") == version:
            return out
    print(f"generating {preset} data in {os.path.relpath(out)} ...", flush=True)
    t0 = time.time()
    subprocess.run([VENV_PY, os.path.join(HERE, "datagen.py"), out, json.dumps(sizes)], check=True)
    print(f"  done in {time.time() - t0:.1f} s", flush=True)
    return out


# ------------------------------------------------------------------- build


def build_compiler():
    exe = os.path.join(COMPILER_DIR, "quark" + EXE)
    print("building the Quark compiler ...", flush=True)
    subprocess.run(["go", "build", "-o", exe, "."], cwd=COMPILER_DIR, check=True)
    return exe


def program(bench, impl_id):
    """The Quark program that holds an implementation. The std/blas ones are
    separate programs, so a machine without OpenBLAS skips only them."""
    return f"{bench}_blas" if impl_id == "quark_blas" else bench


OPENBLAS_HINT = ("OpenBLAS not found; quark_blas needs it (Debian/Ubuntu: sudo apt install libopenblas-dev; "
                 "Fedora: sudo dnf install openblas-devel; macOS: brew install openblas, then set QUARK_LIB_PATH "
                 "and QUARK_INCLUDE_PATH as in stdlib.md)")


def build_quark(quark_exe, benches, build_dir):
    """Builds the Quark programs the benchmarks need; returns
    {program: seconds} and {program: (kind, message)}, kind being "error" or
    "no-openblas"."""
    os.makedirs(build_dir, exist_ok=True)
    env = dict(os.environ, QUARK_STDLIB_ROOT=STDLIB)
    times, errors = {}, {}
    programs = []
    for b in benches:
        for impl_id in b.impls:
            if IMPL[impl_id].lang == "quark" and program(b.name, impl_id) not in programs:
                programs.append(program(b.name, impl_id))
    for name in programs:
        src = os.path.join(HERE, "quark", f"{name}.qrk")
        exe = os.path.join(build_dir, name)
        t0 = time.time()
        r = subprocess.run([quark_exe, "build", src, "-o", exe], env=env, capture_output=True, text=True)
        times[name] = round(time.time() - t0, 2)
        text = (r.stdout + r.stderr).strip()
        if r.returncode == 0:
            print(f"  quark build {name}: {times[name]:.1f} s", flush=True)
        elif name.endswith("_blas") and ("QK-FFI-002" in text or ("QK-FFI-003" in text and "cblas.h" in text)):
            errors[name] = ("no-openblas", OPENBLAS_HINT + "\n" + text)
            print(f"  quark build {name}: skipped, OpenBLAS not found", flush=True)
        else:
            errors[name] = ("error", text)
            print(f"  quark build {name}: FAILED", flush=True)
    return times, errors


# --------------------------------------------------------------------- run


def command(impl_id, bench, data_dir, args, build_dir):
    lang = IMPL[impl_id].lang
    kv = [f"bench={bench.name}", f"impl={impl_id}", f"data={data_dir}"]
    kv += [f"{k}={v}" for k, v in args.items()]
    if lang == "quark":
        return [os.path.join(build_dir, program(bench.name, impl_id) + EXE)] + kv
    if lang == "python":
        return [VENV_PY, os.path.join(HERE, "python", "run.py")] + kv
    return [RSCRIPT, "--no-init-file", os.path.join(HERE, "r", "run.R")] + kv


def peak_rss_mb(usage):
    # ru_maxrss is in KiB on Linux and in bytes on macOS.
    return usage.ru_maxrss / (1024 * 1024 if sys.platform == "darwin" else 1024)


def run_process(cmd, env, timeout):
    """Runs cmd; returns (status, stdout, stderr, peak_rss_mb, wall). Peak
    memory comes from wait4, so it is None on Windows."""
    with tempfile.TemporaryFile("w+") as out, tempfile.TemporaryFile("w+") as err:
        t0 = time.time()
        proc = subprocess.Popen(cmd, stdout=out, stderr=err, env=env, cwd=HERE)
        status, rss = "ok", None
        if not hasattr(os, "wait4"):
            try:
                status = "ok" if proc.wait(timeout=timeout) == 0 else "error"
            except subprocess.TimeoutExpired:
                proc.kill()
                proc.wait()
                status = "timeout"
        while hasattr(os, "wait4"):
            pid, code, usage = os.wait4(proc.pid, os.WNOHANG)
            if pid != 0:
                proc.returncode = os.waitstatus_to_exitcode(code)
                rss = peak_rss_mb(usage)
                if proc.returncode != 0:
                    status = "error"
                break
            if time.time() - t0 > timeout:
                proc.kill()
                _, code, usage = os.wait4(proc.pid, 0)
                proc.returncode = -9
                status, rss = "timeout", peak_rss_mb(usage)
                break
            time.sleep(0.02)
        wall = time.time() - t0
        out.seek(0)
        err.seek(0)
        return status, out.read(), err.read(), rss, wall


def parse_output(stdout):
    times, check = [], None
    for line in stdout.splitlines():
        parts = line.split("\t")
        if parts[0] == "TIME" and len(parts) == 5:
            times.append(float(parts[4]))
        elif parts[0] == "CHECK" and len(parts) == 4:
            check = float(parts[3])
    return times, check


def verify(runs, bench):
    """Marks each run's check against the reference implementation."""
    ok = {r["impl"]: r for r in runs if r["status"] == "ok" and r["checksum"] is not None}
    ref_id = next((i for i in REFERENCE_ORDER if i in ok), next(iter(ok), None))
    if ref_id is None:
        return
    ref = ok[ref_id]["checksum"]
    for r in runs:
        if r["status"] != "ok":
            continue
        if r["checksum"] is None:
            r["status"], r["check"] = "error", "no checksum printed"
            continue
        if r["impl"] == ref_id:
            r["check"] = "reference"
            continue
        err = abs(r["checksum"] - ref)
        tol = bench.rtol * max(abs(ref), 1e-300)
        if err <= tol:
            r["check"] = "ok"
        else:
            r["check"] = f"mismatch: {r['checksum']!r} vs {ref_id} {ref!r}"
            r["status"] = "wrong"


def run_suite(args):
    sizes = dict(PRESETS[args.preset])
    benches = [BENCH[b] for b in args.bench] if args.bench else BENCHMARKS
    langs = args.lang or LANGS
    wanted = set(args.impl) if args.impl else None

    st = tool_status(r_needed="r" in langs)
    if not st["python"]:
        sys.exit("no .venv: run `python3 bench.py setup` first (data generation needs numpy)")
    print_status(st, langs)
    print()
    # Without go and clang++ the Quark rows are reported as skipped.
    build_q = "quark" in langs and st["clang"] and st["go"]

    quark_exe = args.quark
    if build_q and quark_exe is None:
        quark_exe = build_compiler()
    if quark_exe:
        quark_exe = os.path.abspath(quark_exe)

    stamp = datetime.datetime.now().strftime("%Y-%m-%d_%H%M%S")
    out_dir = args.out or os.path.join(HERE, "results", f"{stamp}_{args.preset}_t{args.threads}")
    os.makedirs(out_dir, exist_ok=True)
    build_dir = os.path.join(HERE, "build", "quark")

    data_dir = ensure_data(args.preset, sizes, force=args.regen)
    env_info = environment(quark_exe, args.threads)

    builds, build_errors = {}, {}
    if build_q:
        quark_benches = [b for b in benches if any(IMPL[i].lang == "quark" for i in b.impls)]
        builds, build_errors = build_quark(quark_exe, quark_benches, build_dir)

    proc_env = thread_env(args.threads)
    proc_env["QUARK_STDLIB_ROOT"] = STDLIB
    runs = []
    for b in benches:
        params = b.params(sizes)
        bench_runs = []
        for impl_id in b.impls + b.planned:
            imp = IMPL[impl_id]
            if imp.lang not in langs or (wanted and impl_id not in wanted):
                continue
            row = {"bench": b.name, "impl": impl_id, "status": "ok", "times": [], "checksum": None,
                   "check": "", "peak_rss_mb": None, "wall": None, "error": ""}
            if impl_id in b.planned:
                row["status"] = "planned"
            elif not impl_available(impl_id, st):
                row["status"] = "skipped"
                row["error"] = {"quark": "go and clang++ are needed", "python": "the library is not in .venv",
                                "r": "Rscript (or data.table) not found"}[imp.lang]
            elif imp.lang == "quark" and program(b.name, impl_id) in build_errors:
                kind, text = build_errors[program(b.name, impl_id)]
                row["status"] = "skipped" if kind == "no-openblas" else "error"
                row["error"] = ("" if kind == "no-openblas" else "build failed: ") + text[-2000:]
            else:
                cmd = command(impl_id, b, data_dir, dict(params, warmup=args.warmup, reps=args.reps,
                                                         budget=args.budget), build_dir)
                env = r_env(proc_env) if imp.lang == "r" else proc_env
                status, out, err, rss, wall = run_process(cmd, env, args.timeout)
                times, check = parse_output(out)
                row.update(status=status, times=times, checksum=check,
                           peak_rss_mb=round(rss, 1) if rss else None, wall=round(wall, 2))
                if status == "ok" and not times:
                    row["status"] = "error"
                if row["status"] != "ok":
                    row["error"] = (err.strip() or out.strip())[-2000:]
            bench_runs.append(row)
            show = fmt_time(statistics.median(row["times"])) if row["times"] else row["status"]
            print(f"  {b.name:<11} {impl_id:<11} {show}", flush=True)
        verify(bench_runs, b)
        for r in bench_runs:
            if r["status"] == "wrong":
                print(f"  {b.name:<11} {r['impl']:<11} CHECK FAILED: {r['check']}", flush=True)
        runs += bench_runs

    env_info["load_average_end"] = load_average()
    results = {
        "env": env_info,
        "config": {"preset": args.preset, "threads": args.threads, "warmup": args.warmup, "reps": args.reps,
                   "budget": args.budget, "timeout": args.timeout, "sizes": sizes},
        "benchmarks": [{"name": b.name, "group": b.group, "title": b.title, "params": b.params(sizes),
                        "impls": b.impls, "planned": b.planned, "rtol": b.rtol} for b in benches],
        "builds": builds,
        "runs": runs,
    }
    with open(os.path.join(out_dir, "results.json"), "w") as f:
        json.dump(results, f, indent=1)
    write_csv(results, os.path.join(out_dir, "times.csv"))
    write_report(results, os.path.join(out_dir, "report.md"))
    print(f"\nresults: {os.path.relpath(out_dir)}/report.md")
    bad = [r for r in runs if r["status"] in ("error", "wrong", "timeout")]
    return 1 if bad and args.strict else 0


# ------------------------------------------------------------------ report


def write_csv(results, path):
    with open(path, "w", newline="") as f:
        w = csv.writer(f)
        w.writerow(["bench", "impl", "lang", "tier", "rep", "seconds"])
        for r in results["runs"]:
            imp = IMPL[r["impl"]]
            for i, t in enumerate(r["times"]):
                w.writerow([r["bench"], r["impl"], imp.lang, imp.tier, i, repr(t)])


def summarize(run):
    t = run["times"]
    if run["status"] != "ok" or not t:
        return None
    return {"median": statistics.median(t), "min": min(t), "max": max(t), "n": len(t)}


def write_report(results, path):
    env, cfg = results["env"], results["config"]
    runs = {(r["bench"], r["impl"]): r for r in results["runs"]}
    benches = results["benchmarks"]
    impl_order = [i.id for i in IMPLS if any((b["name"], i.id) in runs for b in benches)]
    L = []
    L.append("# Quark benchmark results")
    L.append("")
    L.append(f"Generated by `bench.py` on {env['date']}. How to read this report: "
             "[README.md](../../README.md#reading-the-report). Times are the median of the timed runs; "
             "lower is better.")
    L.append("")
    L.append("## Contents")
    L.append("")
    L.append("- [Setup](#setup)")
    L.append("- [Summary](#summary)")
    L.append("- [All times](#all-times)")
    L.append("- [Details](#details)")
    L.append("- [Failures](#failures)")
    L.append("")
    L.append("## Setup")
    L.append("")
    L.append("| | |")
    L.append("|---|---|")
    rows = [
        ("Commit", env.get("commit")), ("Preset", cfg["preset"]),
        ("Threads", "library defaults" if cfg["threads"] == 0 else cfg["threads"]),
        ("Runs", f"{cfg['warmup']} warmup + up to {cfg['reps']} timed, {cfg['budget']} s budget per implementation"),
        ("CPU", f"{env.get('cpu')} ({env.get('cpus')} logical CPUs, {env.get('virtualized')})"),
        ("Memory", f"{env.get('memory_gb')} GB"), ("OS", env.get("os")),
        ("Load average", f"{env.get('load_average_start')} at start, {env.get('load_average_end')} at end"),
        ("Quark toolchain", f"{env.get('clang')}; {env.get('go')}"),
        ("System OpenBLAS (Quark std/blas)", env.get("system_openblas")),
        ("Python", f"{env.get('python')}: {env.get('python_libs')}"),
        ("NumPy BLAS", env.get("numpy_blas")),
        ("R", f"{env.get('r')}, data.table {env.get('data.table')}"),
        ("R BLAS", env.get("r_blas")),
    ]
    for k, v in rows:
        L.append(f"| {k} | {v} |")
    L.append("")
    L.append("Sizes: " + ", ".join(f"`{k}={v:,}`" for k, v in cfg["sizes"].items()) + ".")
    L.append("")

    # Summary: best per language.
    L.append("## Summary")
    L.append("")
    L.append("The fastest implementation in each language, and how many times slower Quark's best is "
             "(below 1 means Quark is faster).")
    L.append("")
    L.append("| Benchmark | Best Quark | Best Python | Best R | Quark / Python | Quark / R |")
    L.append("|---|---|---|---|---|---|")
    for b in benches:
        best = {}
        for lang in LANGS:
            cands = [(summarize(runs[(b["name"], i)]), i) for i in b["impls"]
                     if IMPL[i].lang == lang and (b["name"], i) in runs]
            cands = [(s["median"], i) for s, i in cands if s]
            if cands:
                best[lang] = min(cands)
        cells = []
        for lang in LANGS:
            if lang in best:
                t, i = best[lang]
                cells.append(f"{fmt_time(t)} ({IMPL[i].label})")
            else:
                cells.append("—")
        ratios = []
        for lang in ("python", "r"):
            if "quark" in best and lang in best:
                ratios.append(f"{best['quark'][0] / best[lang][0]:.2f}")
            else:
                ratios.append("—")
        L.append(f"| [{b['name']}](#{b['name']}) | " + " | ".join(cells + ratios) + " |")
    L.append("")

    # Matrix of all medians.
    L.append("## All times")
    L.append("")
    L.append("Median time per implementation. **Bold** is the fastest in the row; `n/a` means the tool has "
             "no natural way to do it, `planned` an implementation still to be written.")
    L.append("")
    L.append("| Benchmark | " + " | ".join(IMPL[i].label for i in impl_order) + " |")
    L.append("|---|" + "---|" * len(impl_order))
    for b in benches:
        meds = {i: summarize(runs[(b["name"], i)]) for i in impl_order if (b["name"], i) in runs}
        fastest = min((s["median"] for s in meds.values() if s), default=None)
        cells = []
        for i in impl_order:
            r = runs.get((b["name"], i))
            s = meds.get(i)
            if r is None:
                cells.append("n/a" if i not in b["impls"] + b["planned"] else "")
            elif s:
                txt = fmt_time(s["median"])
                cells.append(f"**{txt}**" if s["median"] == fastest else txt)
            else:
                cells.append(r["status"])
        L.append(f"| {b['name']} | " + " | ".join(cells) + " |")
    L.append("")

    # Details per benchmark.
    L.append("## Details")
    L.append("")
    L.append("`× fastest` is the median divided by the fastest median. Peak memory is the process's maximum "
             "resident set size, including the interpreter and the loaded input data.")
    L.append("")
    for b in benches:
        L.append(f"### {b['name']}")
        L.append("")
        params = ", ".join(f"{k}={v:,}" if isinstance(v, int) else f"{k}={v}" for k, v in b["params"].items())
        L.append(f"{b['title'][0].upper() + b['title'][1:]} ({params}). "
                 f"[What it computes](../../README.md#{b['name']}).")
        L.append("")
        L.append("| Implementation | Median | Min | Max | Runs | × fastest | Peak memory | Check |")
        L.append("|---|---|---|---|---|---|---|---|")
        bench_runs = [runs[(b["name"], i)] for i in impl_order if (b["name"], i) in runs]
        stats = [(r, summarize(r)) for r in bench_runs]
        fastest = min((s["median"] for _, s in stats if s), default=None)
        for r, s in sorted(stats, key=lambda x: (x[1] is None, x[1]["median"] if x[1] else 0)):
            label = IMPL[r["impl"]].label
            mem = f"{r['peak_rss_mb']:,.0f} MB" if r["peak_rss_mb"] else ""
            if s:
                L.append(f"| {label} | {fmt_time(s['median'])} | {fmt_time(s['min'])} | {fmt_time(s['max'])} | "
                         f"{s['n']} | {s['median'] / fastest:.1f} | {mem} | {r['check']} |")
            else:
                L.append(f"| {label} | {r['status']} | | | | | {mem} | {r['check']} |")
        L.append("")
    if results.get("builds"):
        L.append("Quark build times (`quark build`, seconds): "
                 + ", ".join(f"{k} {v}" for k, v in results["builds"].items()) + ".")
        L.append("")

    L.append("## Failures")
    L.append("")
    failed = [r for r in results["runs"] if r["status"] in ("error", "timeout", "wrong", "skipped")]
    if not failed:
        L.append("None: every implementation ran and agreed with the reference checksum.")
    for r in failed:
        L.append(f"- **{r['bench']} / {r['impl']}**: {r['status']}"
                 + (f" ({r['check']})" if r["check"] else ""))
        if r["error"]:
            L.append("")
            L.append("  ```")
            for line in r["error"].splitlines()[-15:]:
                L.append("  " + line)
            L.append("  ```")
    L.append("")
    with open(path, "w") as f:
        f.write("\n".join(L))


# --------------------------------------------------------------- commands


# pandas 3 and NumPy 2.4 (requirements.txt) need Python 3.11 or newer.
PY_MIN = (3, 11)


def install_data_table():
    """Installs data.table from CRAN into the R user library, creating it."""
    code = ('lib <- path.expand(Sys.getenv("R_LIBS_USER")); '
            'dir.create(lib, recursive = TRUE, showWarnings = FALSE); '
            'install.packages("data.table", lib = lib, repos = "https://cloud.r-project.org")')
    print("\ninstalling data.table from CRAN into the R user library ...", flush=True)
    return subprocess.run([RSCRIPT, "-e", code], env=r_env()).returncode == 0


def print_status(st, langs=LANGS):
    """Prints what can run here, for the languages in langs."""
    ok = lambda v: "ok" if v else "MISSING"
    if "quark" in langs:
        if not (st["clang"] and st["go"]):
            print("Quark:  MISSING: needs go and clang++ (Quark implementations are skipped)")
        elif st["openblas"]:
            print("Quark:  ok; OpenBLAS ok")
        else:
            print("Quark:  ok; OpenBLAS not found, so quark_blas is skipped. " + OPENBLAS_HINT.split("(", 1)[1].rstrip(")"))
    if "python" in langs:
        if st["python"]:
            version = sh([VENV_PY, "-c", "import platform; print(platform.python_version())"])
            libs = ", ".join(f"{k} {ok(v)}" for k, v in st["python_libs"].items())
            print(f"Python: .venv (Python {version}): {libs}")
        else:
            print("Python: MISSING .venv (run `bench.py setup`)")
    if "r" in langs:
        if not st["r"]:
            print("R:      MISSING: Rscript not found on the PATH; set BENCH_RSCRIPT to its path "
                  "(R implementations are skipped)")
        elif st["data.table"]:
            print(f"R:      {RSCRIPT}; data.table ok")
        else:
            print(f"R:      {RSCRIPT}; data.table MISSING, so r_dt is skipped (`bench.py setup` installs it)")


def cmd_setup(args):
    uv = shutil.which("uv")
    if not os.path.exists(VENV_PY):
        if uv:
            # uv downloads Python 3.11 itself when it isn't installed.
            subprocess.run([uv, "venv", "--python", "3.11", VENV], check=True)
        elif sys.version_info < PY_MIN:
            sys.exit(f"Python {platform.python_version()} is too old: the pinned pandas and NumPy need 3.11 or "
                     "newer. Run this with a newer Python, or install uv (https://docs.astral.sh/uv/), which "
                     "downloads one.")
        else:
            subprocess.run([sys.executable, "-m", "venv", VENV], check=True)
    req = os.path.join(HERE, "requirements.txt")
    if uv:
        subprocess.run([uv, "pip", "install", "--python", VENV_PY, "-r", req], check=True)
    else:
        subprocess.run([VENV_PY, "-m", "pip", "install", "-r", req], check=True)
    st = tool_status(r_needed=True)
    if st["r"] and not st["data.table"] and not args.no_r_packages:
        if not install_data_table():
            print("data.table did not install; r_dt will be skipped (see the R output above)")
        st = tool_status(r_needed=True)
    print()
    print_status(st)
    print("\nNext: python3 bench.py run --preset small   (about 2-4 minutes; checks every implementation agrees)")


def cmd_list(_args):
    for b in BENCHMARKS:
        planned = f"  (planned: {', '.join(b.planned)})" if b.planned else ""
        print(f"{b.name:<11} [{b.group}] {b.title}")
        print(f"{'':<11} {', '.join(b.impls)}{planned}")


def cmd_report(args):
    with open(os.path.join(args.dir, "results.json")) as f:
        results = json.load(f)
    write_report(results, os.path.join(args.dir, "report.md"))
    print(os.path.join(args.dir, "report.md"))


def main():
    ap = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    sub = ap.add_subparsers(dest="cmd", required=True)
    sp = sub.add_parser("setup", help="create .venv with the Python libraries, install data.table if missing")
    sp.add_argument("--no-r-packages", action="store_true", help="don't install data.table")
    sub.add_parser("list", help="list benchmarks and implementations")
    rp = sub.add_parser("report", help="rebuild report.md from a results directory")
    rp.add_argument("dir")
    r = sub.add_parser("run", help="run the suite")
    r.add_argument("--preset", choices=PRESETS, default="default")
    r.add_argument("--bench", type=lambda s: s.split(","), help="comma-separated benchmarks (default: all)")
    r.add_argument("--impl", type=lambda s: s.split(","), help="comma-separated implementations")
    r.add_argument("--lang", type=lambda s: s.split(","), help="comma-separated languages: quark,python,r")
    r.add_argument("--threads", type=int, default=1,
                   help="threads per library (default 1; 0 = each library's default)")
    r.add_argument("--warmup", type=int, default=1)
    r.add_argument("--reps", type=int, default=5)
    r.add_argument("--budget", type=float, default=30.0,
                   help="seconds per implementation after which no more timed runs start (default 30)")
    r.add_argument("--timeout", type=float, default=600.0, help="seconds before a process is killed")
    r.add_argument("--quark", help="use this quark executable instead of building src/core/quark")
    r.add_argument("--out", help="results directory (default: results/<date>_<preset>_t<threads>)")
    r.add_argument("--regen", action="store_true", help="regenerate the input data")
    r.add_argument("--strict", action="store_true", help="exit 1 if any implementation fails")
    args = ap.parse_args()
    if args.cmd == "run":
        for name in args.bench or []:
            if name not in BENCH:
                sys.exit(f"unknown benchmark {name}; see `bench.py list`")
        for name in args.impl or []:
            if name not in IMPL:
                sys.exit(f"unknown implementation {name}; see `bench.py list`")
        sys.exit(run_suite(args))
    {"setup": cmd_setup, "list": cmd_list, "report": cmd_report}[args.cmd](args)


if __name__ == "__main__":
    main()
