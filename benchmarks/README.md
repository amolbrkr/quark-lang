# Benchmarks

## List vs Vector

Compares Quark `list` (boxed `QValue` elements, scalar `get`/`set` loop) against Quark `vector` (contiguous numeric buffer, `nums = nums + 1`) for the same workload: add 1 to every element, repeated many times.

Python list and NumPy baselines are included for context.

### Running

From the repo root:

```powershell
./benchmarks/run_benchmark.ps1 -N 5000 -Iterations 5000 -Runs 5
```

Options:

| Flag | Default | Description |
|------|---------|-------------|
| `-N` | 5000 | Number of elements |
| `-Iterations` | 5000 | Update passes over the array |
| `-Runs` | 5 | Timed runs (plus 1 warmup) |
| `-Rebuild` | off | Force-rebuild `quark.exe` |
| `-SkipPython` | off | Skip Python baselines |

The script generates `.qrk` source files (with N-element literals) into `benchmarks/generated/`, compiles them once, then times only binary execution.

### Benchmark code

**Quark vector** (vector-scalar add assignment):

```quark
nums = vector [1, 2, ..., N]

iter = 0
while iter < ITERATIONS:
    nums = nums + 1
    iter = iter + 1

println(sum(nums))
```

**Quark list** (scalar `get`/`set` loop):

```quark
nums = list [1, 2, ..., N]

iter = 0
while iter < ITERATIONS:
    idx1 = 0
    while idx1 < len(nums):
        set(nums, idx1, get(nums, idx1) + 1)
        idx1 = idx1 + 1
    iter = iter + 1

checksum = 0
idx2 = 0
while idx2 < len(nums):
    checksum = checksum + get(nums, idx2)
    idx2 = idx2 + 1

println(checksum)
```

**Python list** (scalar loop, equivalent to Quark list):

```python
nums = list(range(1, N + 1))
for _ in range(ITERATIONS):
    for i in range(len(nums)):
        nums[i] += 1
print(sum(nums))
```

**NumPy** (vectorized, equivalent to Quark vector):

```python
import numpy as np
nums = np.arange(1, N + 1, dtype=np.float64)
for _ in range(ITERATIONS):
    nums += 1
print(nums.sum())
```

### Results

N=5000 elements, 5000 iterations (25M element updates). Quark compiled with `g++ -O3 -march=native`. Best-of-5 runs after warmup.

| Implementation | Time (ms) | vs Python list |
|---|---|---|
| NumPy (vectorized) | 6.8 | 158x |
| **Quark vector** (`nums = nums + 1`) | **11.5** | **93x** |
| **Quark list** (scalar `get`/`set`) | **47.7** | **23x** |
| Python list (scalar loop) | 1074 | 1x |

Quark vector vs Quark list speedup: **4.1x**

### Compilation time breakdown

The Quark Go frontend (lex/parse/analyze/codegen) is fast. The bottleneck is `g++ -O3` compiling the generated C++, particularly large literal initializer lists.

| Stage | Vector | List |
|---|---|---|
| Quark frontend (Go) | ~30ms | ~30ms |
| g++ -O3 (C++ backend) | ~5s | ~40s |

The list benchmark compiles slower because g++ must optimize nested while loops with multiple `q_get`/`q_set`/`q_add` calls.

### Files

| File | Description |
|------|-------------|
| `run_benchmark.ps1` | Generates `.qrk` files, compiles, runs, reports |
| `bench_python.py` | Python list + NumPy baselines |
| `generated/` | Generated `.qrk` sources and binaries (gitignored) |
