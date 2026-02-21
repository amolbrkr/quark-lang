# QVector v1 Specification (DataFrame-Oriented)

Status: Draft v1  
Audience: Quark runtime/codegen contributors  
Scope: Runtime vector type and core builtin semantics for performant, reliable columnar data

## 1) Design Goals

1. Build a strong columnar foundation for a future `DataFrame` type.
2. Keep implementation simple and maintainable in a small runtime.
3. Be fast on common hardware without requiring explicit SIMD libraries.
4. Offer ergonomic math/ML-style vector operations through operators + builtins.
5. Prioritize reliability: deterministic semantics, strong invariants, and testable kernels.

## 2) Non-Goals (v1)

- No JIT compilation or runtime-generated compiler hints.
- No GPU acceleration.
- No sparse vectors.
- No full Arrow compatibility surface (only Arrow-inspired storage for strings/categories).

## 3) High-Level Type Model

Use one logical vector type with explicit physical variants:

```cpp
struct StringStorage {
    std::vector<uint32_t> offsets; // size = count + 1
    std::vector<char> bytes;       // UTF-8 payload
};

struct CategoricalStorage {
    std::vector<int32_t> codes;    // -1 => null
    std::vector<std::string> dictionary;
};

struct NullMask {
    // v1 simple byte mask: 0 = valid, 1 = null
    // (v2 may use bit-packed representation)
    std::vector<uint8_t> is_null;
};

struct QVector {
    enum class Type { F64, I64, BOOL, STR, CAT };

    Type type;
    size_t count;
    bool has_nulls;

    std::variant<
        std::vector<double>,
        std::vector<int64_t>,
        std::vector<uint8_t>,
        StringStorage,
        CategoricalStorage
    > storage;

    NullMask nulls; // empty when has_nulls == false
};
```

## 4) Invariants (Must Always Hold)

1. `count` equals logical element count.
2. Active `storage` variant matches `type`.
3. For `F64/I64/BOOL`: physical array length equals `count`.
4. For `STR`: `offsets.size() == count + 1`, `offsets[0] == 0`, and offsets are monotonic non-decreasing with `offsets.back() == bytes.size()`.
5. For `CAT`: `codes.size() == count`, each non-null code is in `[0, dictionary.size())`.
6. If `has_nulls == false`, `nulls.is_null` is empty.
7. If `has_nulls == true`, `nulls.is_null.size() == count`.
8. Null semantics are consistent across all ops (see section 8).

Implementation rule: all constructors and mutating operations must validate and preserve these invariants.

## 5) Construction and Core API

## 5.1 Constructors

- `qv_f64(capacity?)`
- `qv_i64(capacity?)`
- `qv_bool(capacity?)`
- `qv_str(capacity_strings?, capacity_bytes?)`
- `qv_cat(capacity?, dictionary?)`

## 5.2 Core Methods

- `size(vec) -> int`
- `dtype(vec) -> string` (`"f64"|"i64"|"bool"|"str"|"cat"`)
- `is_nullable(vec) -> bool`
- `null_count(vec) -> int`
- `reserve(vec, n) -> vec`
- `clone(vec) -> vec`
- `astype(vec, target_dtype) -> vec`

## 5.3 Element Access

- `get(vec, i)` supports negative index.
- `set(vec, i, value)` returns updated vec.
- Out-of-bounds behavior: return `null` for `get`, no-op+`null` for invalid `set` (consistent with existing runtime conventions).

## 6) Data-Type Semantics

### F64

- Primary numeric type for math/ML.
- NaN/Inf are preserved.

### I64

- Integer counts, indices, and exact arithmetic where feasible.
- Overflow policy v1: C++ native wrap behavior for speed, documented as implementation-defined.

### BOOL

- Byte-per-bool (`uint8_t`) in v1 for straightforward kernels.
- Valid values are 0/1.

### STR

- UTF-8 bytes + offsets.
- String comparisons are bytewise UTF-8 lexical comparisons in v1.

### CAT

- Dictionary-encoded strings via integer codes.
- Null category value represented through null mask (preferred) and/or `-1` code for robustness.
- Operations requiring lexical/string output may decode on demand.

## 7) Operator and Builtin Surface (v1)

## 7.1 Arithmetic

Supported on `F64` and `I64`:

- `+`, `-`, `*`, `/` vector-vector
- `+`, `-`, `*`, `/` vector-scalar
- Scalar updates through vector-scalar arithmetic (`v = v + s`, `v = v * s`) with optional in-place kernels as future optimization

Type promotion:

- `I64 op I64 -> I64` except division -> `F64`.
- `F64` with anything numeric -> `F64`.

## 7.2 Comparisons

- `== != < <= > >=` for numeric vectors and scalars.
- `== !=` for `BOOL`, `STR`, `CAT`.
- Output type: `BOOL` vector.

## 7.3 Logical

- `and`, `or`, `not` on `BOOL` vectors.

## 7.4 Reductions

- `sum`, `min`, `max`, `mean` (mean may return `F64`).
- For `BOOL`: `sum` returns count of `true`.

## 7.5 Utility Builtins

- `fillna(vec, value)`
- `where(mask, a, b)`
- `unique(vec)`
- `value_counts(vec)` (returns dict in v1; later table-like structure)
- `cast(vec, dtype)` / `astype`

## 8) Null Semantics (Required)

General rule: nulls propagate unless operation explicitly handles them.

- Binary arithmetic/comparison:
  - If either side is null at index `i`, result index `i` is null.
- Reductions:
  - Ignore nulls by default.
  - If all values are null, return `null`.
- Logical ops:
  - v1 two-valued with propagation (`null op x => null`), no SQL 3-valued logic initially.
- `fillna` replaces nulls with provided scalar.

## 9) Category-Specific Semantics

- `cat_from_str(str_vec) -> cat_vec`: build dictionary in first-seen order.
- `cat_as_str(cat_vec) -> str_vec`: decode codes using dictionary.
- Equality comparisons operate on codes if dictionaries are aligned.
- For mismatched dictionaries:
  - v1 behavior: decode to strings and compare (correctness-first).
  - Future optimization: dictionary unification cache.

## 10) Performance Model

## 10.1 Fast-Path Strategy

Use a 3-tier kernel approach:

1. Scalar reference kernel (correctness baseline).
2. Auto-vectorizable loop kernel (default fast path).
3. Optional explicit SIMD kernel (future, guarded behind compile-time flag).

## 10.2 Auto-Vectorization-Friendly Kernel Rules

- Operate on raw contiguous pointers in hot loops.
- No aliasing in numeric binary kernels (`__restrict__` pointers internally).
- Straight counted loops: `for (size_t i = 0; i < n; ++i)`.
- No function calls in the inner loop.
- Separate null-mask path from no-null path.
- Pre-size outputs before loop entry.

## 10.3 Compiler Profiles

Portable default profile:

- `-O3 -DNDEBUG -march=x86-64-v3` on x86_64/amd64
- `-O3 -DNDEBUG` on other architectures

Native optional profile (developer opt-in):

- `-O3 -DNDEBUG -march=native`

Rationale: portable binaries work across machines; native profile is for local maximum speed.

## 10.4 Vectorization Diagnostics (Developer Mode)

Optional debug build toggles:

- `-Rpass=loop-vectorize`
- `-Rpass-missed=loop-vectorize`

Use these for profiling/inspection only, not runtime behavior.

## 11) Reliability and Error Policy

- Type mismatch in vector ops returns `null` (consistent with current runtime style).
- Length mismatch in vector-vector ops returns `null`.
- Invalid casts return `null` for the element (or whole-op `null` in v1 if simpler).
- Runtime never crashes from user-level type misuse; prefer graceful null/error return.

## 12) Testing Contract

## 12.1 Unit Tests

- Constructor invariants for every type.
- Null propagation across arithmetic/comparison/logical/reduction.
- String offsets correctness (including empty strings).
- Category encode/decode and dictionary edge cases.
- Casting matrix tests.

## 12.2 Differential Tests

For each optimized numeric kernel, compare against scalar reference output on random inputs:

- Include sizes: `0,1,2,3,4,7,8,15,16,31,32,1k,1M`.
- Include null-density ranges: `0%, 1%, 10%, 50%, 100%`.
- Include NaN/Inf cases for F64.

## 12.3 Property Tests

- `cat_as_str(cat_from_str(x)) == x` (modulo null encoding).
- `sum(fillna(x,0))` equals null-ignoring sum of `x`.
- `where(mask,a,b)` output length and dtype invariants.

## 13) DataFrame Integration Points

The future `DataFrame` should treat each column as `QVector` plus metadata:

- Column schema: `{name, dtype, nullable}`
- Alignment guarantee: all columns same row count.
- Row filter operation becomes boolean-mask apply across columns.
- Groupby keys strongly benefit from `CAT` columns.

## 14) Migration from Current Runtime

Current vector runtime appears `double`-only; migrate in phases:

1. Introduce `QVector` v1 type and constructors (`F64` first).
2. Preserve existing `vector [..]` literal to create `F64` vectors.
3. Add `I64` and `BOOL` kernels.
4. Add `STR` storage API and builtins.
5. Add `CAT` encode/decode builtins.
6. Extend parser/codegen syntax later for typed constructors if desired.

## 15) Minimal v1 Deliverable Checklist

- `QVector` type + invariants
- `F64/I64/BOOL/STR/CAT` constructors
- Null mask support
- Numeric arithmetic + reductions (`sum/min/max/mean`)
- Comparison ops to bool masks
- `fillna`, `where`, `astype`
- `cat_from_str`, `cat_as_str`, `value_counts`
- Portable default compile profile + opt-in native profile
- Differential tests for all optimized kernels

---

This spec is intentionally correctness-first with selective high-impact performance paths. It keeps the runtime understandable while enabling a practical path to DataFrame and ML-friendly vector operations.
