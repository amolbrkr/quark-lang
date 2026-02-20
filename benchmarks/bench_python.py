"""
Quark benchmark baseline: Python list vs NumPy.

Usage:
    python bench_python.py [--n 5000] [--iter 5000]

Runs two benchmarks with N elements and ITER update passes:
  1. Python list  - scalar loop: list[i] += 1
  2. NumPy array  - vectorized:  arr += 1
"""

import argparse
import time


def bench_list(n: int, iterations: int) -> float:
    """Scalar list[i] += 1 loop (equivalent to Quark list get/set)."""
    nums = list(range(1, n + 1))
    t0 = time.perf_counter()
    for _ in range(iterations):
        for i in range(len(nums)):
            nums[i] += 1
    elapsed = time.perf_counter() - t0
    checksum = sum(nums)
    return elapsed, checksum


def bench_numpy(n: int, iterations: int) -> float:
    """Vectorized arr += 1 (equivalent to Quark nums = nums + 1)."""
    import numpy as np

    nums = np.arange(1, n + 1, dtype=np.float64)
    t0 = time.perf_counter()
    for _ in range(iterations):
        nums += 1
    elapsed = time.perf_counter() - t0
    checksum = nums.sum()
    return elapsed, checksum


def main():
    parser = argparse.ArgumentParser(description="Quark benchmark baselines")
    parser.add_argument("--n", type=int, default=5000, help="number of elements")
    parser.add_argument("--iter", type=int, default=5000, help="update iterations")
    args = parser.parse_args()

    n, iterations = args.n, args.iter
    total_ops = n * iterations

    print(f"Python baselines: N={n}, iterations={iterations} ({total_ops:,} element updates)")
    print()

    # Python list
    list_time, list_sum = bench_list(n, iterations)
    print(f"Python list : {list_time*1000:8.1f} ms   checksum={int(list_sum)}")

    # NumPy
    try:
        np_time, np_sum = bench_numpy(n, iterations)
        print(f"NumPy array : {np_time*1000:8.1f} ms   checksum={int(np_sum)}")
    except ImportError:
        np_time = None
        print("NumPy array : (numpy not installed, skipped)")

    print()
    if np_time:
        print(f"NumPy speedup vs Python list: {list_time/np_time:.1f}x")


if __name__ == "__main__":
    main()
