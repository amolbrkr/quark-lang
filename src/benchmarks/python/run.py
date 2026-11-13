"""Python benchmark runner.

    python run.py bench=<name> impl=<id> data=<dir> warmup=<n> reps=<n> budget=<s> [param=value ...]

Runs one implementation of one benchmark and prints the TIME and CHECK lines
that bench.py parses (protocol in src/benchmarks/README.md).
"""

import sys
import time

import impls


def params(argv):
    p = {}
    for arg in argv:
        key, sep, value = arg.partition("=")
        if not sep:
            sys.exit(f"bad argument (expected key=value): {arg}")
        p[key] = value
    return p


def measure(p, f):
    """Runs f `warmup` times untimed, then up to `reps` times timed (one TIME
    line each), stopping once `budget` seconds are spent (warmup included,
    at least one timed run). Returns the last result."""
    warmup, reps, budget = int(p["warmup"]), int(p["reps"]), float(p["budget"])
    spent = 0.0
    out = None
    for _ in range(warmup):
        t0 = time.perf_counter()
        out = f()
        spent += time.perf_counter() - t0
    for r in range(reps):
        t0 = time.perf_counter()
        out = f()
        dt = time.perf_counter() - t0
        print(f"TIME\t{p['bench']}\t{p['impl']}\t{r}\t{dt!r}", flush=True)
        spent += dt
        if spent > budget:
            break
    return out


def main():
    p = params(sys.argv[1:])
    fn = impls.REGISTRY.get((p["bench"], p["impl"]))
    if fn is None:
        sys.exit(f"no Python implementation {p['impl']} of {p['bench']}")
    value = fn(p, lambda f: measure(p, f))
    print(f"CHECK\t{p['bench']}\t{p['impl']}\t{float(value)!r}", flush=True)


if __name__ == "__main__":
    main()
