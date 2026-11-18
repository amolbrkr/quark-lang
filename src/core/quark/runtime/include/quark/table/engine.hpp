// quark/table/engine.hpp - eager columnar table engine (tables v0.1)
//
// The kernels behind the table verbs. They work on raw column buffers, never
// on boxed QValues per cell, and every result is a new table or column: the
// inputs are immutable and may share columns with the outputs.
//
// Columns are QVectors (Arrow layout, see types/vector.hpp); a table is a
// QTable whose QStructDef names its columns. Row indices are int32_t, so a
// table holds at most INT32_MAX rows.
//
//   take / take_table      gather rows by index (filter, sort)
//   make_table             build a table from columns (mutate, summarize)
//   sort_perm / sort_table stable multi-key sort, nulls last
//   group_rows             hash grouping, groups ordered by key, nulls last
//   agg_*                  per-group aggregates for summarize
//
// Missing values (nulls, and NaN in float columns) follow na.rm = TRUE:
// aggregates skip them. sum of no values is 0; mean, sd, min and max of no
// values are null (sd also when fewer than two are present); count counts
// rows. Group keys and sort keys put nulls last.
//
// Generated code evaluates filter predicates and mutate expressions with its own
// typed loops over the column buffers; this header supplies the column
// builders and the row gathering they feed.
#ifndef QUARK_TABLE_ENGINE_HPP
#define QUARK_TABLE_ENGINE_HPP

#include "../types/table.hpp"

#include <algorithm>
#include <cmath>
#include <cstdint>
#include <cstring>
#include <limits>
#include <memory>
#include <string_view>
#include <utility>
#include <vector>

namespace quark {
namespace tbl {

using Type = QVector::Type;

[[noreturn]] inline void fail(const char* what) {
    q_runtime_reportf("runtime error: %s\n", what);
    q_exit(1);
}

inline int32_t* alloc_i32(size_t n) {
    return static_cast<int32_t*>(q_malloc_atomic((n == 0 ? 1 : n) * sizeof(int32_t)));
}

inline int32_t checked_rows(size_t n) {
    if (n > static_cast<size_t>(std::numeric_limits<int32_t>::max())) {
        fail("a table can hold at most 2147483647 rows");
    }
    return static_cast<int32_t>(n);
}

// ------------------------------------------------------------
// Column access
// ------------------------------------------------------------

inline QVector* column(const QTable* t, int c) {
    if (c < 0 || c >= t->ncols || t->cols[c].type != QValue::VAL_VECTOR || !t->cols[c].data.vector_val) {
        fail("internal: table column index out of range");
    }
    return t->cols[c].data.vector_val;
}

inline bool is_null(const QVector& v, int32_t i) {
    return v.validity != nullptr && !quark::vec::get_bit(v.validity, static_cast<size_t>(i));
}

// A float value that aggregates and grouping treat as missing.
inline bool is_missing_f64(const QVector& v, int32_t i) {
    return is_null(v, i) || std::isnan(q_vec_f64_data(v)[i]);
}

// Marks row i of a fresh column null (allocating the validity bitmap).
inline void set_null(QVector& v, int32_t i) { q_vec_mark_null(v, static_cast<size_t>(i)); }

// ------------------------------------------------------------
// Tables
// ------------------------------------------------------------

// make_table builds a table with schema def from def->field_count columns,
// which must all have the same length.
inline QTable* make_table(const QStructDef* def, QVector* const* cols) {
    const int n = def->field_count;
    size_t rows = n > 0 ? cols[0]->count : 0;
    for (int c = 0; c < n; c++) {
        if (cols[c] == nullptr || cols[c]->count != rows) {
            fail("internal: table columns have different lengths");
        }
    }
    QTable* t = q_table_create(def, checked_rows(rows));
    for (int c = 0; c < n; c++) {
        t->cols[c] = qv_vector_from(cols[c]);
    }
    return t;
}

// take gathers rows idx[0..n) of src into a new column, a block of output
// rows at a time on the thread pool (core/par.hpp): each block writes its
// own rows, and its own bytes of a bool or validity bitmap (blocks are
// multiples of 8 rows).
inline QVector* take(const QVector& src, const int32_t* idx, int32_t n) {
    QVector* dst = nullptr;
    const quark::par::Plan plan = q_par_plan(n);
    switch (src.type) {
        case Type::F64:
        case Type::I64: {
            dst = q_vec_alloc_raw(src.type, static_cast<size_t>(n));
            const uint64_t* s = reinterpret_cast<const uint64_t*>(src.values);
            uint64_t* d = reinterpret_cast<uint64_t*>(dst->values);
            q_par_run(plan, [&](long long lo, long long hi, long long) {
                for (long long i = lo; i < hi; i++) d[i] = s[idx[i]];
            });
            break;
        }
        case Type::DATE: {
            dst = q_vec_alloc_raw(Type::DATE, static_cast<size_t>(n));
            const int32_t* s = q_vec_date_data(src);
            int32_t* d = q_vec_date_data_mut(*dst);
            q_par_run(plan, [&](long long lo, long long hi, long long) {
                for (long long i = lo; i < hi; i++) d[i] = s[idx[i]];
            });
            break;
        }
        case Type::BOOL: {
            dst = q_vec_alloc(Type::BOOL, static_cast<size_t>(n));
            q_par_run(plan, [&](long long lo, long long hi, long long) {
                for (long long i = lo; i < hi; i++) {
                    if (quark::vec::get_bit(src.values, static_cast<size_t>(idx[i]))) {
                        quark::vec::put_bit(dst->values, static_cast<size_t>(i), true);
                    }
                }
            });
            break;
        }
        case Type::STR: {
            const size_t nb = static_cast<size_t>(plan.nblocks);
            std::vector<size_t> start(nb + 1, 0);
            q_par_run(plan, [&](long long lo, long long hi, long long b) {
                size_t bytes = 0;
                for (long long i = lo; i < hi; i++) bytes += static_cast<size_t>(src.offsets[idx[i] + 1] - src.offsets[idx[i]]);
                start[static_cast<size_t>(b) + 1] = bytes;
            });
            for (size_t b = 0; b < nb; b++) start[b + 1] += start[b];
            if (start[nb] > static_cast<size_t>(std::numeric_limits<int32_t>::max())) {
                fail("str column data exceeds 2 GiB");
            }
            dst = q_vec_alloc(Type::STR, static_cast<size_t>(n), start[nb]);
            q_par_run(plan, [&](long long lo, long long hi, long long b) {
                int32_t pos = static_cast<int32_t>(start[static_cast<size_t>(b)]);
                for (long long i = lo; i < hi; i++) {
                    const int32_t a = src.offsets[idx[i]];
                    const int32_t len = src.offsets[idx[i] + 1] - a;
                    if (len > 0) std::memcpy(dst->values + pos, src.values + a, static_cast<size_t>(len));
                    pos += len;
                    dst->offsets[i + 1] = pos;
                }
            });
            break;
        }
    }
    if (src.null_count > 0) {
        // The validity bitmap is made first, all valid, and each block
        // clears its missing rows and counts them.
        const size_t nb = static_cast<size_t>(plan.nblocks);
        dst->validity_cap = quark::vec::bitmap_bytes(static_cast<size_t>(n));
        dst->validity = quark::vec::alloc(dst->validity_cap, &dst->validity_base);
        std::memset(dst->validity, 0xFF, dst->validity_cap);
        std::vector<size_t> missing(nb, 0);
        q_par_run(plan, [&](long long lo, long long hi, long long b) {
            size_t c = 0;
            for (long long i = lo; i < hi; i++) {
                if (is_null(src, idx[i])) {
                    quark::vec::put_bit(dst->validity, static_cast<size_t>(i), false);
                    c++;
                }
            }
            missing[static_cast<size_t>(b)] = c;
        });
        size_t total = 0;
        for (size_t c : missing) total += c;
        dst->null_count = total;
        if (total == 0) {
            dst->validity = nullptr;
            dst->validity_cap = 0;
        }
    }
    return dst;
}

// take_table gathers rows idx[0..n) of every column. Taking every row in
// order returns the input itself (tables are immutable).
inline QTable* take_table(QTable* t, const int32_t* idx, int32_t n) {
    if (n == t->nrows) {
        bool identity = true;
        for (int32_t i = 0; i < n && identity; i++) identity = idx[i] == i;
        if (identity) return t;
    }
    QTable* out = q_table_create(t->def, n);
    for (int c = 0; c < t->ncols; c++) {
        out->cols[c] = qv_vector_from(take(*column(t, c), idx, n));
    }
    return out;
}

// ------------------------------------------------------------
// Comparison (sort and group keys)
// ------------------------------------------------------------

// compare_rows orders rows a and b of one column: <0, 0 or >0. Nulls sort
// after every value, and NaN after every number but before nulls, whatever
// the direction.
inline int compare_rows(const QVector& v, int32_t a, int32_t b, bool desc) {
    const bool an = is_null(v, a), bn = is_null(v, b);
    if (an || bn) return an == bn ? 0 : (an ? 1 : -1);
    int c = 0;
    switch (v.type) {
        case Type::I64: {
            const int64_t x = q_vec_i64_data(v)[a], y = q_vec_i64_data(v)[b];
            c = x < y ? -1 : (x > y ? 1 : 0);
            break;
        }
        case Type::DATE: {
            const int32_t x = q_vec_date_data(v)[a], y = q_vec_date_data(v)[b];
            c = x < y ? -1 : (x > y ? 1 : 0);
            break;
        }
        case Type::F64: {
            const double x = q_vec_f64_data(v)[a], y = q_vec_f64_data(v)[b];
            const bool xn = std::isnan(x), yn = std::isnan(y);
            if (xn || yn) return xn == yn ? 0 : (xn ? 1 : -1);
            c = x < y ? -1 : (x > y ? 1 : 0);
            break;
        }
        case Type::BOOL: {
            const int x = q_vec_bool_at(v, static_cast<size_t>(a)), y = q_vec_bool_at(v, static_cast<size_t>(b));
            c = x - y;
            break;
        }
        case Type::STR: {
            const int r = q_vec_str_at(v, static_cast<size_t>(a)).compare(q_vec_str_at(v, static_cast<size_t>(b)));
            c = r < 0 ? -1 : (r > 0 ? 1 : 0);
            break;
        }
    }
    return desc ? -c : c;
}

// load_u64 and load_u32 read 8 and 4 bytes of a string, unaligned.
inline uint64_t load_u64(const char* p) {
    uint64_t w;
    std::memcpy(&w, p, 8);
    return w;
}

inline uint64_t load_u32(const char* p) {
    uint32_t w;
    std::memcpy(&w, p, 4);
    return w;
}

// bytes_equal compares two strings of n bytes, with fixed-size reads for up
// to 16 bytes (no call to memcmp for a short key).
inline bool bytes_equal(const char* x, const char* y, size_t n) {
    if (n >= 8) {
        if (n > 16) return std::memcmp(x, y, n) == 0;
        return load_u64(x) == load_u64(y) && load_u64(x + n - 8) == load_u64(y + n - 8);
    }
    if (n >= 4) return load_u32(x) == load_u32(y) && load_u32(x + n - 4) == load_u32(y + n - 4);
    for (size_t i = 0; i < n; i++) {
        if (x[i] != y[i]) return false;
    }
    return true;
}

inline bool same_str(std::string_view a, std::string_view b) { return a.size() == b.size() && bytes_equal(a.data(), b.data(), a.size()); }

// rows_equal reports whether rows a and b hold the same key value (two nulls
// are equal, as are two NaNs, and 0.0 and -0.0).
inline bool rows_equal(const QVector& v, int32_t a, int32_t b) {
    const bool an = is_null(v, a), bn = is_null(v, b);
    if (an || bn) return an == bn;
    switch (v.type) {
        case Type::I64: return q_vec_i64_data(v)[a] == q_vec_i64_data(v)[b];
        case Type::DATE: return q_vec_date_data(v)[a] == q_vec_date_data(v)[b];
        case Type::F64: {
            const double x = q_vec_f64_data(v)[a], y = q_vec_f64_data(v)[b];
            return x == y || (std::isnan(x) && std::isnan(y));
        }
        case Type::BOOL: return q_vec_bool_at(v, static_cast<size_t>(a)) == q_vec_bool_at(v, static_cast<size_t>(b));
        case Type::STR: return same_str(q_vec_str_at(v, static_cast<size_t>(a)), q_vec_str_at(v, static_cast<size_t>(b)));
    }
    return false;
}

// ------------------------------------------------------------
// Sort
// ------------------------------------------------------------

struct SortKey {
    int col;
    bool desc;
};

// par_stable_sort is std::stable_sort of a[0, n) on the thread pool: each
// block is stable-sorted, then runs are merged in pairs, wider each pass
// (std::merge takes the left run's element on a tie, so it is stable too).
// A stable sort has one result, so it is the same for every thread count.
template <class T, class Less>
inline void par_stable_sort(T* a, size_t n, Less less) {
    const quark::par::Plan plan = q_par_plan(static_cast<long long>(n));
    if (plan.nblocks <= 1) {
        std::stable_sort(a, a + n, less);
        return;
    }
    q_par_run(plan, [&](long long lo, long long hi, long long) { std::stable_sort(a + lo, a + hi, less); });
    std::unique_ptr<T[]> tmp(new T[n]);
    T* src = a;
    T* dst = tmp.get();
    for (size_t width = static_cast<size_t>(q_par_block); width < n; width *= 2) {
        const size_t pairs = (n + 2 * width - 1) / (2 * width);
        quark::par::Plan mp;
        mp.nblocks = static_cast<long long>(pairs);
        mp.n = mp.nblocks * q_par_block;
        q_par_run(mp, [&](long long, long long, long long b) {
            const size_t lo = static_cast<size_t>(b) * 2 * width;
            const size_t mid = std::min(lo + width, n);
            const size_t hi = std::min(lo + 2 * width, n);
            std::merge(src + lo, src + mid, src + mid, src + hi, dst + lo, less);
        });
        std::swap(src, dst);
    }
    if (src != a) {
        q_par_run(plan, [&](long long lo, long long hi, long long) { std::copy(src + lo, src + hi, a + lo); });
    }
}

// sort_one_numeric orders one int, float or bool key. Missing rows stay last,
// in their original order, and NaN stays after every number and before the
// missing rows, in either direction. The numbers are a stable sort of
// (value, row) pairs.
inline int32_t* sort_one_numeric(QTable* t, SortKey key) {
    const QVector& col = *column(t, key.col);
    const int32_t n = t->nrows;
    int32_t* perm = alloc_i32(static_cast<size_t>(n));
    std::vector<int32_t> nums, nans, missing;
    nums.reserve(static_cast<size_t>(n));
    const double* fd = col.type == Type::F64 ? q_vec_f64_data(col) : nullptr;
    for (int32_t i = 0; i < n; i++) {
        if (is_null(col, i)) {
            missing.push_back(i);
            continue;
        }
        if (fd != nullptr && std::isnan(fd[i])) {
            nans.push_back(i);
            continue;
        }
        nums.push_back(i);
    }
    auto write = [&](auto pairs) {
        if (key.desc) {
            par_stable_sort(pairs.data(), pairs.size(), [](const auto& x, const auto& y) { return x.first > y.first; });
        } else {
            par_stable_sort(pairs.data(), pairs.size(), [](const auto& x, const auto& y) { return x.first < y.first; });
        }
        int32_t w = 0;
        for (const auto& p : pairs) perm[w++] = p.second;
        for (int32_t i : nans) perm[w++] = i;
        for (int32_t i : missing) perm[w++] = i;
    };
    switch (col.type) {
        case Type::I64: {
            const int64_t* x = q_vec_i64_data(col);
            std::vector<std::pair<int64_t, int32_t>> pairs;
            pairs.reserve(nums.size());
            for (int32_t i : nums) pairs.push_back({x[i], i});
            write(std::move(pairs));
            break;
        }
        case Type::F64: {
            std::vector<std::pair<double, int32_t>> pairs;
            pairs.reserve(nums.size());
            for (int32_t i : nums) pairs.push_back({fd[i], i});
            write(std::move(pairs));
            break;
        }
        case Type::BOOL: {
            std::vector<std::pair<int, int32_t>> pairs;
            pairs.reserve(nums.size());
            for (int32_t i : nums) pairs.push_back({q_vec_bool_at(col, static_cast<size_t>(i)) ? 1 : 0, i});
            write(std::move(pairs));
            break;
        }
        case Type::DATE: {
            const int32_t* x = q_vec_date_data(col);
            std::vector<std::pair<int32_t, int32_t>> pairs;
            pairs.reserve(nums.size());
            for (int32_t i : nums) pairs.push_back({x[i], i});
            write(std::move(pairs));
            break;
        }
        case Type::STR:
            fail("internal: numeric sort of a str key");
    }
    return perm;
}

inline uint64_t hash_value(const QVector& v, int32_t i);

// str_ranks is, for each row, the rank of its bytes among the distinct
// present values (0 is the first in byte order). A missing cell ranks after
// every present value. Equal strings share a rank.
inline std::vector<int32_t> str_ranks(const QVector& v) {
    const int32_t n = static_cast<int32_t>(v.count);
    std::vector<int32_t> rank(static_cast<size_t>(n), 0);
    if (n == 0) return rank;
    size_t cap = 16;
    while (cap < static_cast<size_t>(n) * 2) cap <<= 1;
    std::vector<int32_t> slots(cap, 0);
    std::vector<int32_t> first;
    std::vector<uint64_t> hashes;
    const size_t mask = cap - 1;
    for (int32_t r = 0; r < n; r++) {
        if (is_null(v, r)) continue;
        const uint64_t h = hash_value(v, r);
        size_t s = static_cast<size_t>(h) & mask;
        while (true) {
            const int32_t slot = slots[s];
            if (slot == 0) {
                slots[s] = static_cast<int32_t>(first.size()) + 1;
                first.push_back(r);
                hashes.push_back(h);
                break;
            }
            const int32_t g = slot - 1;
            if (hashes[static_cast<size_t>(g)] == h && rows_equal(v, r, first[static_cast<size_t>(g)])) break;
            s = (s + 1) & mask;
        }
    }
    std::vector<int32_t> order(first.size());
    for (size_t i = 0; i < first.size(); i++) order[i] = static_cast<int32_t>(i);
    // The values are distinct, so the order has no ties.
    par_stable_sort(order.data(), order.size(), [&](int32_t a, int32_t b) {
        return q_vec_str_at(v, static_cast<size_t>(first[static_cast<size_t>(a)])) <
               q_vec_str_at(v, static_cast<size_t>(first[static_cast<size_t>(b)]));
    });
    std::vector<int32_t> id_rank(first.size());
    for (size_t i = 0; i < order.size(); i++) id_rank[static_cast<size_t>(order[i])] = static_cast<int32_t>(i);
    const int32_t missing_rank = static_cast<int32_t>(first.size());
    for (int32_t r = 0; r < n; r++) {
        if (is_null(v, r)) {
            rank[static_cast<size_t>(r)] = missing_rank;
            continue;
        }
        const uint64_t h = hash_value(v, r);
        size_t s = static_cast<size_t>(h) & mask;
        while (true) {
            const int32_t g = slots[s] - 1;
            if (hashes[static_cast<size_t>(g)] == h && rows_equal(v, r, first[static_cast<size_t>(g)])) {
                rank[static_cast<size_t>(r)] = id_rank[static_cast<size_t>(g)];
                break;
            }
            s = (s + 1) & mask;
        }
    }
    return rank;
}

// cmp_key orders two rows of one key of a known type. A string key compares
// the ranks from str_ranks (null when the key is not a string). Missing
// values and NaN stay in place in either direction: only present numbers,
// bools and string ranks follow desc.
inline int cmp_key(const QVector& v, int32_t a, int32_t b, bool desc, const int32_t* ranks) {
    const bool an = is_null(v, a), bn = is_null(v, b);
    if (an || bn) return an == bn ? 0 : (an ? 1 : -1);
    int c = 0;
    switch (v.type) {
        case Type::I64: {
            const int64_t x = q_vec_i64_data(v)[a], y = q_vec_i64_data(v)[b];
            c = x < y ? -1 : (x > y ? 1 : 0);
            break;
        }
        case Type::DATE: {
            const int32_t x = q_vec_date_data(v)[a], y = q_vec_date_data(v)[b];
            c = x < y ? -1 : (x > y ? 1 : 0);
            break;
        }
        case Type::F64: {
            const double x = q_vec_f64_data(v)[a], y = q_vec_f64_data(v)[b];
            const bool xn = std::isnan(x), yn = std::isnan(y);
            if (xn || yn) return xn == yn ? 0 : (xn ? 1 : -1);
            c = x < y ? -1 : (x > y ? 1 : 0);
            break;
        }
        case Type::BOOL: {
            const int x = q_vec_bool_at(v, static_cast<size_t>(a)) ? 1 : 0;
            const int y = q_vec_bool_at(v, static_cast<size_t>(b)) ? 1 : 0;
            c = x - y;
            break;
        }
        case Type::STR: {
            const int32_t x = ranks[a], y = ranks[b];
            c = x < y ? -1 : (x > y ? 1 : 0);
            break;
        }
    }
    return desc ? -c : c;
}

// sort_perm returns the row order of t sorted by keys (stable: rows with
// equal keys keep their order). One numeric key is sort_one_numeric. A
// string key, or more than one key, ranks each string and then stable-sorts
// integers and raw numbers.
inline int32_t* sort_perm(QTable* t, const SortKey* keys, int nkeys) {
    const int32_t n = t->nrows;
    if (n < 2 || nkeys == 0) {
        int32_t* perm = alloc_i32(static_cast<size_t>(n));
        for (int32_t i = 0; i < n; i++) perm[i] = i;
        return perm;
    }
    const QVector& k0 = *column(t, keys[0].col);
    if (nkeys == 1 && (k0.type == Type::I64 || k0.type == Type::F64 || k0.type == Type::BOOL || k0.type == Type::DATE)) {
        return sort_one_numeric(t, keys[0]);
    }

    std::vector<const QVector*> cols(static_cast<size_t>(nkeys));
    std::vector<std::vector<int32_t>> rank_store(static_cast<size_t>(nkeys));
    std::vector<const int32_t*> ranks(static_cast<size_t>(nkeys), nullptr);
    for (int k = 0; k < nkeys; k++) {
        cols[static_cast<size_t>(k)] = column(t, keys[k].col);
        if (cols[static_cast<size_t>(k)]->type == Type::STR) {
            rank_store[static_cast<size_t>(k)] = str_ranks(*cols[static_cast<size_t>(k)]);
            ranks[static_cast<size_t>(k)] = rank_store[static_cast<size_t>(k)].data();
        }
    }
    int32_t* perm = alloc_i32(static_cast<size_t>(n));
    for (int32_t i = 0; i < n; i++) perm[i] = i;
    par_stable_sort(perm, static_cast<size_t>(n), [&](int32_t a, int32_t b) {
        for (int k = 0; k < nkeys; k++) {
            const int c = cmp_key(*cols[static_cast<size_t>(k)], a, b, keys[k].desc, ranks[static_cast<size_t>(k)]);
            if (c != 0) return c < 0;
        }
        return false;
    });
    return perm;
}

inline QTable* sort_table(QTable* t, const SortKey* keys, int nkeys) {
    return take_table(t, sort_perm(t, keys, nkeys), t->nrows);
}

// ------------------------------------------------------------
// Grouping
// ------------------------------------------------------------

// Grouping, the aggregates and distinct run on the thread pool (specs/
// tables_v0_3_spec.md §8.1) and give the result of one loop over the rows
// at every thread count (§8.4): groups in key order, each with its first
// row; float sums over a group's values in row order (pairwise); exact
// counts and int sums; the first row on a tie for min and max.

struct GroupParts;

// Grouping assigns every row of a table to a group. Groups are numbered in
// key order (ascending, nulls last). An ungrouped table is one group of all
// rows, with group_of_row == nullptr.
struct Grouping {
    int32_t nrows = 0;
    int32_t ngroups = 0;
    int32_t* group_of_row = nullptr; // [nrows]: the group of each row
    int32_t* first_row = nullptr;    // [ngroups]: the first row of each group
    // The rows split by ranges of groups (group_parts), made by the first
    // aggregate of a step with many groups and shared by the others.
    mutable GroupParts* parts = nullptr;
};

// par_pieces runs f(lo, hi) over the items [0, k) in pieces of at most per
// items: as the blocks of one loop on the pool when a loop over work rows
// would be split (q_par_plan), else as one piece on this thread. A piece
// writes only what belongs to its items.
template <class F>
inline void par_pieces(long long k, long long per, long long work, F f) {
    if (k <= 0) return;
    if (per < 1) per = 1;
    const long long nb = (k + per - 1) / per;
    if (nb <= 1 || q_par_plan(work).nblocks <= 1) {
        f(0LL, k);
        return;
    }
    quark::par::Plan p;
    p.nblocks = nb;
    p.n = nb * q_par_block;
    q_par_run(p, [&](long long, long long, long long b) {
        const long long lo = b * per;
        f(lo, lo + per < k ? lo + per : k);
    });
}

// RowChunks split the rows [0, n) into contiguous ranges, for a pass that
// keeps a state per group in each range: fewer, longer ranges than the
// pool's blocks (a few per thread), so the states stay few. One range when
// a loop over n rows wouldn't be split.
struct RowChunks {
    long long n = 0;
    long long nchunks = 1;
    long long lo(long long c) const { return n * c / nchunks; }
    long long hi(long long c) const { return n * (c + 1) / nchunks; }
};

inline RowChunks row_chunks(long long n) {
    RowChunks rc;
    rc.n = n;
    const long long nb = q_par_plan(n).nblocks;
    if (nb > 1) rc.nchunks = std::min<long long>(nb, 4LL * q_par_threads());
    return rc;
}

// for_chunks runs f(c, lo, hi) for every range c of rc, on the pool.
template <class F>
inline void for_chunks(const RowChunks& rc, F f) {
    par_pieces(rc.nchunks, 1, rc.n, [&](long long a, long long b) {
        for (long long c = a; c < b; c++) f(c, rc.lo(c), rc.hi(c));
    });
}

inline Grouping whole_table(int32_t nrows) {
    Grouping g;
    g.nrows = nrows;
    g.ngroups = 1;
    g.first_row = alloc_i32(1);
    g.first_row[0] = 0;
    return g;
}

inline uint64_t mix64(uint64_t x) {
    x ^= x >> 30;
    x *= 0xbf58476d1ce4e5b9ULL;
    x ^= x >> 27;
    x *= 0x94d049bb133111ebULL;
    x ^= x >> 31;
    return x;
}

// hash_bytes hashes a string with fixed-size reads (two that overlap for
// up to 16 bytes, as wyhash does), so a short key costs no call to memcpy.
// Hashes only place keys in a table: no result depends on them.
inline uint64_t hash_bytes(const char* p, size_t n) {
    uint64_t a = 0, b = 0;
    uint64_t h = 0x9e3779b97f4a7c15ULL ^ (static_cast<uint64_t>(n) * 0xff51afd7ed558ccdULL);
    if (n <= 16) {
        if (n >= 8) {
            a = load_u64(p);
            b = load_u64(p + n - 8);
        } else if (n >= 4) {
            a = load_u32(p);
            b = load_u32(p + n - 4);
        } else if (n > 0) {
            const unsigned char* s = reinterpret_cast<const unsigned char*>(p);
            a = (static_cast<uint64_t>(s[0]) << 16) | (static_cast<uint64_t>(s[n >> 1]) << 8) | s[n - 1];
        }
    } else {
        size_t i = 0;
        for (; i + 16 < n; i += 16) {
            h = mix64(h ^ load_u64(p + i)) ^ load_u64(p + i + 8);
        }
        a = load_u64(p + n - 16);
        b = load_u64(p + n - 8);
    }
    return mix64(h ^ mix64(a ^ (b * 0xbf58476d1ce4e5b9ULL)));
}

inline uint64_t hash_value(const QVector& v, int32_t i) {
    if (is_null(v, i)) return 0x9e3779b97f4a7c15ULL;
    switch (v.type) {
        case Type::I64: return mix64(static_cast<uint64_t>(q_vec_i64_data(v)[i]));
        case Type::DATE: return mix64(static_cast<uint64_t>(static_cast<uint32_t>(q_vec_date_data(v)[i])));
        case Type::F64: {
            double x = q_vec_f64_data(v)[i];
            if (x == 0.0) x = 0.0; // -0.0 groups with 0.0
            if (std::isnan(x)) return 0x7ff8000000000000ULL;
            uint64_t bits;
            std::memcpy(&bits, &x, sizeof bits);
            return mix64(bits);
        }
        case Type::BOOL: return mix64(q_vec_bool_at(v, static_cast<size_t>(i)) ? 2 : 1);
        case Type::STR: {
            const std::string_view s = q_vec_str_at(v, static_cast<size_t>(i));
            return hash_bytes(s.data(), s.size());
        }
    }
    return 0;
}

// KeyTable holds distinct keys, numbered 0, 1, ... in the order they are
// added, each with its hash and the row it was added from. It is open
// addressing that grows (at most half full), so its size follows the keys,
// not the rows: a few keys stay in cache however many rows there are.
struct KeyTable {
    // A slot is the key's hash's top 32 bits and its id + 1 (0 is empty),
    // so a probe reads the hash it compares from the slot itself.
    std::vector<uint64_t> slots;
    std::vector<uint64_t> hash;
    std::vector<int32_t> first;
    size_t mask = 0;

    KeyTable() : slots(64, 0), mask(63) {}

    int32_t size() const { return static_cast<int32_t>(first.size()); }

    static uint64_t slot_of(uint64_t h, int32_t id) { return (h & 0xffffffff00000000ULL) | (static_cast<uint64_t>(id) + 1); }

    // find_or_add returns the id of row r's key, whose hash is h, adding it
    // (with r as its row) when it is new; same(a, b) compares the keys of
    // rows a and b. *added says which.
    template <class Same>
    int32_t find_or_add(uint64_t h, int32_t r, Same same, bool* added) {
        const uint64_t tag = h & 0xffffffff00000000ULL;
        size_t s = static_cast<size_t>(h) & mask;
        while (true) {
            const uint64_t slot = slots[s];
            if (slot == 0) break;
            const int32_t id = static_cast<int32_t>(static_cast<uint32_t>(slot) - 1);
            if ((slot & 0xffffffff00000000ULL) == tag && same(r, first[static_cast<size_t>(id)])) {
                *added = false;
                return id;
            }
            s = (s + 1) & mask;
        }
        const int32_t id = size();
        hash.push_back(h);
        first.push_back(r);
        if ((static_cast<size_t>(id) + 1) * 2 > slots.size()) {
            grow();
        } else {
            slots[s] = slot_of(h, id);
        }
        *added = true;
        return id;
    }

    void grow() {
        const size_t cap = slots.size() * 2;
        slots.assign(cap, 0);
        mask = cap - 1;
        for (int32_t id = 0; id < size(); id++) {
            const uint64_t h = hash[static_cast<size_t>(id)];
            size_t s = static_cast<size_t>(h) & mask;
            while (slots[s] != 0) s = (s + 1) & mask;
            slots[s] = slot_of(h, id);
        }
    }
};

// distinct_keys finds the distinct keys of the rows [0, n), where hash(r) is
// row r's key hash and same(a, b) compares two rows' keys, and returns the
// first row of each key, by key id. When row_id is given, it receives the
// id of every row's key. The ids are in no useful order; callers put the
// keys in key order or row order, which is unique, since two distinct keys
// are never equal.
//
// Each block of the pool's loop finds its own keys; then the blocks' keys
// are split by hash into parts, merged part by part. A key's entries all
// fall in one part, in block order, so the first one holds the key's first
// row. On one thread it is one table: blocks would only add the merge,
// whose key comparisons read rows at random (measured 2026-11-18: a group
// by of 10M rows in 1M groups took 3.0 s with blocks, 2.3 s without).
//
// The block loop is a function of its own (noinline), as fold's are.
template <class Hash, class Same>
QPAR_NOINLINE inline void block_keys(KeyTable& t, long long lo, long long hi, Hash hash, Same same, int32_t* row_id) {
    bool added = false;
    const int32_t a = static_cast<int32_t>(lo), z = static_cast<int32_t>(hi);
    if (row_id != nullptr) {
        for (int32_t r = a; r < z; r++) row_id[r] = t.find_or_add(hash(r), r, same, &added);
    } else {
        for (int32_t r = a; r < z; r++) t.find_or_add(hash(r), r, same, &added);
    }
}

template <class Hash, class Same>
inline std::vector<int32_t> distinct_keys(int32_t n, Hash hash, Same same, int32_t* row_id) {
    const quark::par::Plan plan = q_par_plan(n);
    const size_t nb = static_cast<size_t>(plan.nblocks);
    std::vector<KeyTable> local(nb);
    q_par_run(plan, [&](long long lo, long long hi, long long b) {
        KeyTable t;
        block_keys(t, lo, hi, hash, same, row_id);
        t.slots = std::vector<uint64_t>(); // not needed by the merge
        local[static_cast<size_t>(b)] = std::move(t);
    });
    if (nb == 1) return std::move(local[0].first);

    // Entry e = ebase[b] + id is key id of block b.
    std::vector<int64_t> ebase(nb + 1, 0);
    for (size_t b = 0; b < nb; b++) ebase[b + 1] = ebase[b] + local[b].size();
    const int64_t total = ebase[nb];
    int bits = 0;
    while (bits < 12 && (int64_t(32768) << bits) < total) bits++;
    const size_t np = size_t(1) << bits;
    auto part_of = [bits](uint64_t h) { return bits == 0 ? size_t(0) : static_cast<size_t>(h >> (64 - bits)); };

    // Order the entries by part, then block, then id: count, then place.
    std::vector<int64_t> at(nb * np, 0);
    par_pieces(static_cast<long long>(nb), 1, total, [&](long long lo, long long hi) {
        for (long long b = lo; b < hi; b++) {
            int64_t* c = &at[static_cast<size_t>(b) * np];
            for (uint64_t h : local[static_cast<size_t>(b)].hash) c[part_of(h)]++;
        }
    });
    std::vector<int64_t> pstart(np + 1, 0);
    int64_t pos = 0;
    for (size_t p = 0; p < np; p++) {
        pstart[p] = pos;
        for (size_t b = 0; b < nb; b++) {
            const int64_t c = at[b * np + p];
            at[b * np + p] = pos;
            pos += c;
        }
    }
    pstart[np] = pos;
    std::unique_ptr<uint64_t[]> eh(new uint64_t[static_cast<size_t>(total)]);
    std::unique_ptr<int32_t[]> ef(new int32_t[static_cast<size_t>(total)]);
    std::unique_ptr<int32_t[]> ee(new int32_t[static_cast<size_t>(total)]);
    par_pieces(static_cast<long long>(nb), 1, total, [&](long long lo, long long hi) {
        for (long long b = lo; b < hi; b++) {
            const KeyTable& t = local[static_cast<size_t>(b)];
            int64_t* c = &at[static_cast<size_t>(b) * np];
            for (int32_t id = 0; id < t.size(); id++) {
                const uint64_t h = t.hash[static_cast<size_t>(id)];
                const int64_t i = c[part_of(h)]++;
                eh[static_cast<size_t>(i)] = h;
                ef[static_cast<size_t>(i)] = t.first[static_cast<size_t>(id)];
                ee[static_cast<size_t>(i)] = static_cast<int32_t>(ebase[static_cast<size_t>(b)] + id);
            }
        }
    });
    local = std::vector<KeyTable>();

    // Merge each part: gmap[e] is the key of entry e, numbered in its part.
    std::unique_ptr<int32_t[]> gmap(new int32_t[static_cast<size_t>(total)]);
    std::vector<std::vector<int32_t>> pfirst(np);
    par_pieces(static_cast<long long>(np), 1, total, [&](long long lo, long long hi) {
        for (long long p = lo; p < hi; p++) {
            KeyTable t;
            bool added = false;
            for (int64_t i = pstart[static_cast<size_t>(p)]; i < pstart[static_cast<size_t>(p) + 1]; i++) {
                gmap[static_cast<size_t>(ee[static_cast<size_t>(i)])] = t.find_or_add(eh[static_cast<size_t>(i)], ef[static_cast<size_t>(i)], same, &added);
            }
            pfirst[static_cast<size_t>(p)] = std::move(t.first);
        }
    });
    std::vector<int64_t> gbase(np + 1, 0);
    for (size_t p = 0; p < np; p++) gbase[p + 1] = gbase[p] + static_cast<int64_t>(pfirst[p].size());
    std::vector<int32_t> first(static_cast<size_t>(gbase[np]));
    par_pieces(static_cast<long long>(np), 1, total, [&](long long lo, long long hi) {
        for (long long p = lo; p < hi; p++) {
            const int64_t base = gbase[static_cast<size_t>(p)];
            std::copy(pfirst[static_cast<size_t>(p)].begin(), pfirst[static_cast<size_t>(p)].end(), first.begin() + base);
            for (int64_t i = pstart[static_cast<size_t>(p)]; i < pstart[static_cast<size_t>(p) + 1]; i++) {
                gmap[static_cast<size_t>(ee[static_cast<size_t>(i)])] += static_cast<int32_t>(base);
            }
        }
    });
    if (row_id != nullptr) {
        q_par_run(plan, [&](long long lo, long long hi, long long b) {
            const int32_t* m = gmap.get() + ebase[static_cast<size_t>(b)];
            for (long long r = lo; r < hi; r++) row_id[r] = m[row_id[r]];
        });
    }
    return first;
}

// key_hashes is every row's hash over several key columns, computed a
// column at a time on the pool (the row loop of distinct_keys then reads it
// instead of dispatching on each key's type).
inline std::unique_ptr<uint64_t[]> key_hashes(int32_t n, const std::vector<const QVector*>& keys) {
    std::unique_ptr<uint64_t[]> h(new uint64_t[static_cast<size_t>(n) + 1]);
    uint64_t* hp = h.get();
    q_par_run(q_par_plan(n), [&](long long lo, long long hi, long long) {
        for (long long r = lo; r < hi; r++) hp[r] = 0;
        for (const QVector* k : keys) {
            for (long long r = lo; r < hi; r++) hp[r] = mix64(hp[r] * 31 + hash_value(*k, static_cast<int32_t>(r)));
        }
    });
    return h;
}

// same_key is the equality of two rows over key columns.
inline bool same_key(const std::vector<const QVector*>& keys, int32_t a, int32_t b) {
    for (const QVector* k : keys) {
        if (!rows_equal(*k, a, b)) return false;
    }
    return true;
}

// with_keys calls f(hash, same) with a row's hash and the key equality of
// two rows over keys, for rows [0, n). For one key column, the common case,
// they are chosen once by its type, so the row loop doesn't branch on it;
// several columns hash with key_hashes and compare with same_key.
template <class F>
inline auto with_keys(int32_t n, const std::vector<const QVector*>& keys, F f) {
    if (keys.size() == 1) {
        const QVector& v = *keys[0];
        if (v.validity == nullptr) {
            switch (v.type) {
                case Type::I64: {
                    const int64_t* x = q_vec_i64_data(v);
                    return f([x](int32_t r) { return mix64(static_cast<uint64_t>(x[r])); }, [x](int32_t a, int32_t b) { return x[a] == x[b]; });
                }
                case Type::DATE: {
                    const int32_t* x = q_vec_date_data(v);
                    return f([x](int32_t r) { return mix64(static_cast<uint64_t>(static_cast<uint32_t>(x[r]))); },
                             [x](int32_t a, int32_t b) { return x[a] == x[b]; });
                }
                case Type::STR:
                    return f(
                        [&v](int32_t r) {
                            const std::string_view s = q_vec_str_at(v, static_cast<size_t>(r));
                            return hash_bytes(s.data(), s.size());
                        },
                        [&v](int32_t a, int32_t b) { return same_str(q_vec_str_at(v, static_cast<size_t>(a)), q_vec_str_at(v, static_cast<size_t>(b))); });
                default: break;
            }
        }
        return f([&v](int32_t r) { return hash_value(v, r); }, [&v](int32_t a, int32_t b) { return rows_equal(v, a, b); });
    }
    const std::unique_ptr<uint64_t[]> h = key_hashes(n, keys);
    const uint64_t* hp = h.get();
    return f([hp](int32_t r) { return hp[r]; }, [&keys](int32_t a, int32_t b) { return same_key(keys, a, b); });
}

// group_by_vecs groups nrows rows by nkeys key columns (pointers into column
// buffers, or into a column built for a computed key). Groups are numbered
// in key order, nulls last. nkeys 0 is the whole table.
inline Grouping group_by_vecs(int32_t nrows, const QVector* const* key_ptrs, int nkeys) {
    const int32_t n = nrows;
    if (nkeys == 0 || key_ptrs == nullptr) return whole_table(n);
    const std::vector<const QVector*> keys(key_ptrs, key_ptrs + nkeys);
    int32_t* group_of_row = alloc_i32(static_cast<size_t>(n));
    const std::vector<int32_t> first =
        with_keys(n, keys, [&](auto hash, auto same) { return distinct_keys(n, hash, same, group_of_row); });

    // Number the groups in key order (unique: no two groups' keys are equal).
    const int32_t ng = static_cast<int32_t>(first.size());
    std::vector<int32_t> order(static_cast<size_t>(ng));
    for (int32_t g = 0; g < ng; g++) order[static_cast<size_t>(g)] = g;
    par_stable_sort(order.data(), order.size(), [&](int32_t x, int32_t y) {
        for (const QVector* k : keys) {
            const int c = compare_rows(*k, first[static_cast<size_t>(x)], first[static_cast<size_t>(y)], false);
            if (c != 0) return c < 0;
        }
        return false;
    });
    std::vector<int32_t> rank(static_cast<size_t>(ng));
    Grouping out;
    out.nrows = n;
    out.ngroups = ng;
    out.first_row = alloc_i32(static_cast<size_t>(ng));
    par_pieces(ng, q_par_block, ng, [&](long long lo, long long hi) {
        for (long long i = lo; i < hi; i++) {
            const int32_t g = order[static_cast<size_t>(i)];
            rank[static_cast<size_t>(g)] = static_cast<int32_t>(i);
            out.first_row[i] = first[static_cast<size_t>(g)];
        }
    });
    q_par_run(q_par_plan(n), [&](long long lo, long long hi, long long) {
        for (long long r = lo; r < hi; r++) group_of_row[r] = rank[static_cast<size_t>(group_of_row[r])];
    });
    out.group_of_row = group_of_row;
    return out;
}

// group_rows groups the rows of t by the key columns.
inline Grouping group_rows(QTable* t, const int* key_cols, int nkeys) {
    if (nkeys == 0) return whole_table(t->nrows);
    std::vector<const QVector*> keys(static_cast<size_t>(nkeys));
    for (int k = 0; k < nkeys; k++) keys[static_cast<size_t>(k)] = column(t, key_cols[k]);
    return group_by_vecs(t->nrows, keys.data(), nkeys);
}

inline int32_t group_of(const Grouping& g, int32_t r) {
    return g.group_of_row ? g.group_of_row[r] : 0;
}

// group_keys is the key column of the summarize output: the key value of
// each group, in group order.
inline QVector* group_keys(QTable* t, const Grouping& g, int key_col) {
    return take(*column(t, key_col), g.first_row, g.ngroups);
}

// ------------------------------------------------------------
// Aggregates (na.rm = TRUE)
// ------------------------------------------------------------

// GroupParts split the rows by ranges of groups, for a step with many
// groups: part p holds the groups [p << shift, (p + 1) << shift) and their
// rows, in row order, at rows[start[p] .. start[p + 1]), with each row's
// group next to it in gids (read in order, not at random). A part's groups
// are its own, so the parts run on their own, each group over its rows in
// row order, as one loop over the table does.
struct GroupParts {
    int32_t nparts;
    int shift;
    int64_t* start; // [nparts + 1]
    int32_t* rows;  // [nrows]
    int32_t* gids;  // [nrows]: the group of rows[i]
};

// StatePlan is how a pass that keeps a state per group splits the rows: one
// range (on this thread), several (each folds its own states, which are
// merged after), or, with too many groups for a state array per range,
// group_parts.
struct StatePlan {
    RowChunks rc;
    bool parts = false;
};

// state_plan picks it for states of state_bytes bytes. A range's states
// are read at random, so they must stay in a core's cache: up to 1 MB, the
// ranges of row_chunks (fewer if their states would take more than 64 MB,
// or 8 bytes a row, together); past that, group_parts. Measured 2026-11-18
// on 10M rows at 6 threads: count of 100K groups 8.6 ms by ranges, 32 ms by
// parts; count of 1M groups 112 ms by ranges, 30 ms by parts.
inline StatePlan state_plan(const Grouping& g, size_t state_bytes) {
    StatePlan sp;
    sp.rc = row_chunks(g.nrows);
    if (sp.rc.nchunks <= 1) return sp;
    const long long one = static_cast<long long>(g.ngroups) * static_cast<long long>(state_bytes);
    if (one > (1LL << 20)) {
        sp.parts = true;
        return sp;
    }
    const long long budget = std::max<long long>(64LL << 20, 8LL * g.nrows);
    sp.rc.nchunks = std::max<long long>(2, std::min(sp.rc.nchunks, budget / std::max<long long>(1, one)));
    return sp;
}

// group_parts makes g's parts once: a power of two of groups each, the
// smallest that makes at most max(4 per thread, groups / 8,192, rows /
// 65,536) parts, so a part's states and rows stay small (in cache) and there
// are parts enough to share out.
inline const GroupParts& group_parts(const Grouping& g) {
    if (g.parts != nullptr) return *g.parts;
    const long long ng = g.ngroups;
    const long long want = std::max({4LL * q_par_threads(), (ng + 8191) / 8192, (static_cast<long long>(g.nrows) + 65535) / 65536});
    int shift = 0;
    while (((ng + (1LL << shift) - 1) >> shift) > want) shift++;
    const size_t np = static_cast<size_t>((ng + (1LL << shift) - 1) >> shift);
    const RowChunks rc = row_chunks(g.nrows);
    const size_t nc = static_cast<size_t>(rc.nchunks);
    std::vector<int64_t> at(nc * np, 0);
    for_chunks(rc, [&](long long c, long long lo, long long hi) {
        int64_t* k = &at[static_cast<size_t>(c) * np];
        for (long long r = lo; r < hi; r++) k[g.group_of_row[r] >> shift]++;
    });
    GroupParts* gp = static_cast<GroupParts*>(q_malloc(sizeof(GroupParts)));
    gp->nparts = static_cast<int32_t>(np);
    gp->shift = shift;
    gp->start = static_cast<int64_t*>(q_malloc_atomic((np + 1) * sizeof(int64_t)));
    gp->rows = alloc_i32(static_cast<size_t>(g.nrows));
    gp->gids = alloc_i32(static_cast<size_t>(g.nrows));
    int64_t pos = 0;
    for (size_t p = 0; p < np; p++) {
        gp->start[p] = pos;
        for (size_t c = 0; c < nc; c++) {
            const int64_t k = at[c * np + p];
            at[c * np + p] = pos;
            pos += k;
        }
    }
    gp->start[np] = pos;
    for_chunks(rc, [&](long long c, long long lo, long long hi) {
        int64_t* k = &at[static_cast<size_t>(c) * np];
        for (long long r = lo; r < hi; r++) {
            const int32_t gid = g.group_of_row[r];
            const int64_t i = k[gid >> shift]++;
            gp->rows[i] = static_cast<int32_t>(r);
            gp->gids[i] = gid;
        }
    });
    g.parts = gp;
    return *gp;
}

// fold computes a state per group: init, then step(state, r) for the group's
// rows r in row order, as one loop over the table does. With few groups,
// each range of rows folds its own states and merge(earlier, later) adds a
// later range's state to an earlier one's, in range order; a merge must
// give what the single loop gives (exact counts and sums; a best row that
// keeps the earlier one on a tie). With many groups, each part of
// group_parts folds its own groups.
//
// The row loops are functions of their own (noinline), with the group
// lookup decided outside the loop: inlined into a caller, the same loop
// compiled about 50% slower (measured 2026-11-18, as in vector.hpp).
template <class S, class Step>
QPAR_NOINLINE inline void fold_rows(const int32_t* gor, long long lo, long long hi, S* s, Step step) {
    const int32_t a = static_cast<int32_t>(lo), z = static_cast<int32_t>(hi);
    if (gor != nullptr) {
        for (int32_t r = a; r < z; r++) step(s[gor[r]], r);
        return;
    }
    S acc = s[0];
    for (int32_t r = a; r < z; r++) step(acc, r);
    s[0] = acc;
}

template <class S, class Step>
QPAR_NOINLINE inline void fold_listed(const int32_t* gids, const int32_t* rows, int64_t lo, int64_t hi, S* s, Step step) {
    for (int64_t i = lo; i < hi; i++) step(s[gids[i]], rows[i]);
}

template <class S, class Step, class Merge>
inline std::vector<S> fold(const Grouping& g, S init, Step step, Merge merge) {
    const size_t ng = static_cast<size_t>(g.ngroups);
    std::vector<S> out(ng, init);
    const StatePlan sp = state_plan(g, sizeof(S));
    const RowChunks& rc = sp.rc;
    if (rc.nchunks <= 1) {
        fold_rows(g.group_of_row, 0, g.nrows, out.data(), step);
        return out;
    }
    if (!sp.parts) {
        const size_t nc = static_cast<size_t>(rc.nchunks);
        std::vector<S> part(nc * ng, init);
        for_chunks(rc, [&](long long c, long long lo, long long hi) {
            fold_rows(g.group_of_row, lo, hi, &part[static_cast<size_t>(c) * ng], step);
        });
        par_pieces(static_cast<long long>(ng), q_par_block / static_cast<long long>(nc), static_cast<long long>(nc * ng),
                   [&](long long lo, long long hi) {
                       for (long long i = lo; i < hi; i++) {
                           S acc = part[static_cast<size_t>(i)];
                           for (size_t c = 1; c < nc; c++) merge(acc, part[c * ng + static_cast<size_t>(i)]);
                           out[static_cast<size_t>(i)] = acc;
                       }
                   });
        return out;
    }
    const GroupParts& gp = group_parts(g);
    par_pieces(gp.nparts, 1, g.nrows, [&](long long lo, long long hi) {
        fold_listed(gp.gids, gp.rows, gp.start[lo], gp.start[hi], out.data(), step);
    });
    return out;
}

// per_group runs f(lo, hi) over the groups: pieces of groups on the pool
// when there are many, or one call for all of them when there are few
// (whose own work, a sum over a large group, can then use the pool). Both
// give the same bits: a group's result depends only on its values.
template <class F>
inline void per_group(const Grouping& g, F f) {
    const long long ng = g.ngroups;
    if (ng < 4LL * q_par_threads()) {
        f(0LL, ng);
        return;
    }
    const long long per = std::max<long long>(1, ng * q_par_block / std::max<long long>(1, g.nrows));
    par_pieces(ng, per, g.nrows, f);
}

// mark_missing sets the groups whose miss[i] is set to missing, after a
// per_group loop (validity bits of neighbouring groups share a byte, so the
// loop can't set them itself).
inline void mark_missing(QVector& out, const std::vector<char>& miss) {
    for (size_t i = 0; i < miss.size(); i++) {
        if (miss[i]) set_null(out, static_cast<int32_t>(i));
    }
}

// agg_count is the number of rows in each group (n()).
inline QVector* agg_count(const Grouping& g) {
    QVector* out = q_vec_alloc(Type::I64, static_cast<size_t>(g.ngroups));
    int64_t* d = q_vec_i64_data_mut(*out);
    if (!g.group_of_row) {
        if (g.ngroups == 1) d[0] = g.nrows;
        return out;
    }
    const std::vector<int64_t> n = fold<int64_t>(
        g, 0, [](int64_t& s, int32_t) { s++; }, [](int64_t& a, int64_t b) { a += b; });
    std::copy(n.begin(), n.end(), d);
    return out;
}

// agg_count_values is the number of non-missing values of col in each group.
inline QVector* agg_count_values(const QVector& col, const Grouping& g) {
    QVector* out = q_vec_alloc(Type::I64, static_cast<size_t>(g.ngroups));
    int64_t* d = q_vec_i64_data_mut(*out);
    const bool f64 = col.type == Type::F64;
    const std::vector<int64_t> n = fold<int64_t>(
        g, 0,
        [&](int64_t& s, int32_t r) {
            if (!(f64 ? is_missing_f64(col, r) : is_null(col, r))) s++;
        },
        [](int64_t& a, int64_t b) { a += b; });
    std::copy(n.begin(), n.end(), d);
    return out;
}

// is_whole_table reports whether g is the single group of whole_table, whose
// aggregates are the vec reductions of the column itself.
inline bool is_whole_table(const Grouping& g) { return g.ngroups == 1 && g.group_of_row == nullptr; }

// GroupValues are the present values of a column, as doubles, group by
// group in row order: group i's are vals[start[i] .. start[i + 1]).
struct GroupValues {
    std::vector<int64_t> start;
    std::unique_ptr<double[]> vals;
    size_t count(size_t i) const { return static_cast<size_t>(start[i + 1] - start[i]); }
    const double* of(size_t i) const { return vals.get() + start[i]; }
};

// The row loops of gather, functions of their own as fold's are:
// count_rows adds 1 to k[group] for each present row of [lo, hi), and
// place_rows puts its value at vals[next[group]++]; the _listed forms take
// the rows rows[lo .. hi).
template <class Value>
QPAR_NOINLINE inline void count_rows(const int32_t* gor, long long lo, long long hi, int64_t* k, Value value) {
    const int32_t a = static_cast<int32_t>(lo), z = static_cast<int32_t>(hi);
    double v = 0;
    if (gor != nullptr) {
        for (int32_t r = a; r < z; r++) {
            if (value(r, &v)) k[gor[r]]++;
        }
        return;
    }
    int64_t c = 0;
    for (int32_t r = a; r < z; r++) {
        if (value(r, &v)) c++;
    }
    k[0] += c;
}

template <class Value>
QPAR_NOINLINE inline void place_rows(const int32_t* gor, long long lo, long long hi, int64_t* next, double* vals, Value value) {
    const int32_t a = static_cast<int32_t>(lo), z = static_cast<int32_t>(hi);
    double v = 0;
    if (gor != nullptr) {
        for (int32_t r = a; r < z; r++) {
            if (value(r, &v)) vals[next[gor[r]]++] = v;
        }
        return;
    }
    int64_t j = next[0];
    for (int32_t r = a; r < z; r++) {
        if (value(r, &v)) vals[j++] = v;
    }
    next[0] = j;
}

template <class Value>
QPAR_NOINLINE inline int64_t count_listed(const int32_t* gids, const int32_t* rows, int64_t lo, int64_t hi, int64_t* k, Value value) {
    double v = 0;
    int64_t total = 0;
    for (int64_t i = lo; i < hi; i++) {
        if (value(rows[i], &v)) {
            k[gids[i]]++;
            total++;
        }
    }
    return total;
}

template <class Value>
QPAR_NOINLINE inline void place_listed(const int32_t* gids, const int32_t* rows, int64_t lo, int64_t hi, int64_t* next, double* vals, Value value) {
    double v = 0;
    for (int64_t i = lo; i < hi; i++) {
        if (value(rows[i], &v)) vals[next[gids[i]]++] = v;
    }
}

// gather collects each group's present values in row order (a stable
// counting sort of the rows by group), so a group is summed exactly as
// sum() sums a vec of the same values (q_vec_f64_present_sum). value(r, &x)
// says whether row r is present and gives its value. With few groups, each
// range of rows counts its values per group and then places them after the
// earlier ranges'; with many, each part of group_parts does its groups.
template <class Value>
inline GroupValues gather(const Grouping& g, Value value) {
    const size_t ng = static_cast<size_t>(g.ngroups);
    GroupValues gv;
    gv.start.assign(ng + 1, 0);
    const StatePlan sp = state_plan(g, sizeof(int64_t));
    const RowChunks& rc = sp.rc;
    if (rc.nchunks <= 1) {
        count_rows(g.group_of_row, 0, g.nrows, gv.start.data() + 1, value);
        for (size_t i = 0; i < ng; i++) gv.start[i + 1] += gv.start[i];
        gv.vals.reset(new double[static_cast<size_t>(gv.start[ng]) + 1]);
        std::vector<int64_t> next(gv.start.begin(), gv.start.end() - 1);
        place_rows(g.group_of_row, 0, g.nrows, next.data(), gv.vals.get(), value);
        return gv;
    }
    if (!sp.parts) {
        const size_t nc = static_cast<size_t>(rc.nchunks);
        std::vector<int64_t> at(nc * ng, 0);
        for_chunks(rc, [&](long long c, long long lo, long long hi) {
            count_rows(g.group_of_row, lo, hi, &at[static_cast<size_t>(c) * ng], value);
        });
        int64_t pos = 0;
        for (size_t i = 0; i < ng; i++) {
            gv.start[i] = pos;
            for (size_t c = 0; c < nc; c++) {
                const int64_t k = at[c * ng + i];
                at[c * ng + i] = pos;
                pos += k;
            }
        }
        gv.start[ng] = pos;
        gv.vals.reset(new double[static_cast<size_t>(pos) + 1]);
        for_chunks(rc, [&](long long c, long long lo, long long hi) {
            place_rows(g.group_of_row, lo, hi, &at[static_cast<size_t>(c) * ng], gv.vals.get(), value);
        });
        return gv;
    }
    // Many groups: count each part's values per group, add up the parts'
    // totals in order, then place each part's.
    const GroupParts& gp = group_parts(g);
    const size_t np = static_cast<size_t>(gp.nparts);
    std::vector<int64_t> cnt(ng, 0);
    std::vector<int64_t> ptotal(np + 1, 0);
    par_pieces(gp.nparts, 1, g.nrows, [&](long long lo, long long hi) {
        for (long long p = lo; p < hi; p++) {
            ptotal[static_cast<size_t>(p) + 1] = count_listed(gp.gids, gp.rows, gp.start[p], gp.start[p + 1], cnt.data(), value);
        }
    });
    for (size_t p = 0; p < np; p++) ptotal[p + 1] += ptotal[p];
    gv.vals.reset(new double[static_cast<size_t>(ptotal[np]) + 1]);
    std::vector<int64_t> next(ng, 0);
    par_pieces(gp.nparts, 1, g.nrows, [&](long long lo, long long hi) {
        for (long long p = lo; p < hi; p++) {
            const size_t g0 = static_cast<size_t>(p) << gp.shift;
            const size_t g1 = std::min(ng, (static_cast<size_t>(p) + 1) << gp.shift);
            int64_t pos = ptotal[static_cast<size_t>(p)];
            for (size_t i = g0; i < g1; i++) {
                gv.start[i] = pos;
                next[i] = pos;
                pos += cnt[i];
            }
            place_listed(gp.gids, gp.rows, gp.start[p], gp.start[p + 1], next.data(), gv.vals.get(), value);
        }
    });
    gv.start[ng] = ptotal[np];
    return gv;
}

// gather_f64 is gather of a float column's present values (not NaN).
inline GroupValues gather_f64(const QVector& col, const Grouping& g) {
    const double* x = q_vec_f64_data(col);
    return gather(g, [&](int32_t r, double* v) {
        if (is_missing_f64(col, r)) return false;
        *v = x[r];
        return true;
    });
}

// agg_sum: int and bool columns give an int (bools count as 0/1), float
// columns a float, summed pairwise (core/sum.hpp). A group with no values
// sums to 0. An int sum is exact (128-bit), so it is an error only when a
// group's total doesn't fit in an int, as for the vec sum.
inline QVector* agg_sum(const QVector& col, const Grouping& g) {
    const size_t ng = static_cast<size_t>(g.ngroups);
    switch (col.type) {
        case Type::I64: {
            QVector* out = q_vec_alloc(Type::I64, ng);
            int64_t* d = q_vec_i64_data_mut(*out);
            if (is_whole_table(g)) {
                size_t n = 0;
                d[0] = q_sum_fits(q_vec_i64_present_sum(col, &n));
                return out;
            }
            const int64_t* x = q_vec_i64_data(col);
            const std::vector<__int128> acc = fold<__int128>(
                g, 0,
                [&](__int128& s, int32_t r) {
                    if (!is_null(col, r)) s += x[r];
                },
                [](__int128& a, __int128 b) { a += b; });
            // In group order, so the error is the first group's that doesn't fit.
            for (size_t i = 0; i < ng; i++) d[i] = q_sum_fits(acc[i]);
            return out;
        }
        case Type::BOOL: {
            QVector* out = q_vec_alloc(Type::I64, ng);
            int64_t* d = q_vec_i64_data_mut(*out);
            if (is_whole_table(g)) {
                size_t n = 0;
                d[0] = static_cast<int64_t>(q_vec_bool_present_trues(col, &n));
                return out;
            }
            const std::vector<int64_t> acc = fold<int64_t>(
                g, 0,
                [&](int64_t& s, int32_t r) {
                    if (!is_null(col, r) && q_vec_bool_at(col, static_cast<size_t>(r))) s++;
                },
                [](int64_t& a, int64_t b) { a += b; });
            std::copy(acc.begin(), acc.end(), d);
            return out;
        }
        case Type::F64: {
            QVector* out = q_vec_alloc(Type::F64, ng);
            double* d = q_vec_f64_data_mut(*out);
            if (is_whole_table(g)) {
                d[0] = q_vec_f64_present_sum(col);
                return out;
            }
            const GroupValues gv = gather_f64(col, g);
            per_group(g, [&](long long lo, long long hi) {
                for (long long i = lo; i < hi; i++) d[i] = q_pairwise_sum_par(gv.of(static_cast<size_t>(i)), gv.count(static_cast<size_t>(i)));
            });
            return out;
        }
        case Type::STR: break;
        case Type::DATE: break;
    }
    fail(col.type == Type::DATE ? "sum needs a number or bool column, got date" : "sum needs a number or bool column, got str");
}

// agg_mean: a float per group; null for a group with no values. Float
// values are summed pairwise, int values exactly (128-bit), and bools give the
// share of true values, as q_vec_mean_present does for a whole column.
inline QVector* agg_mean(const QVector& col, const Grouping& g) {
    if (col.type == Type::STR) fail("mean needs a number or bool column, got str");
    if (col.type == Type::DATE) fail("mean needs a number or bool column, got date");
    const size_t ng = static_cast<size_t>(g.ngroups);
    QVector* out = q_vec_alloc(Type::F64, ng);
    double* d = q_vec_f64_data_mut(*out);
    if (is_whole_table(g)) {
        size_t n = 0;
        const double m = q_vec_mean_present(col, &n);
        if (n == 0) set_null(*out, 0);
        else d[0] = m;
        return out;
    }
    if (col.type == Type::F64) {
        const GroupValues gv = gather_f64(col, g);
        std::vector<char> miss(ng, 0);
        per_group(g, [&](long long lo, long long hi) {
            for (long long i = lo; i < hi; i++) {
                const size_t n = gv.count(static_cast<size_t>(i));
                if (n == 0) miss[static_cast<size_t>(i)] = 1;
                else d[i] = q_pairwise_sum_par(gv.of(static_cast<size_t>(i)), n) / static_cast<double>(n);
            }
        });
        mark_missing(*out, miss);
        return out;
    }
    struct SumCount {
        __int128 sum; // int values, or the number of trues
        int64_t n;
    };
    const bool ints = col.type == Type::I64;
    const std::vector<SumCount> acc = fold<SumCount>(
        g, SumCount{0, 0},
        [&](SumCount& s, int32_t r) {
            if (is_null(col, r)) return;
            s.sum += ints ? q_vec_i64_data(col)[r] : (q_vec_bool_at(col, static_cast<size_t>(r)) ? 1 : 0);
            s.n++;
        },
        [](SumCount& a, const SumCount& b) {
            a.sum += b.sum;
            a.n += b.n;
        });
    for (size_t i = 0; i < ng; i++) {
        if (acc[i].n == 0) set_null(*out, static_cast<int32_t>(i));
        else d[i] = q_i128_to_double(acc[i].sum) / static_cast<double>(acc[i].n);
    }
    return out;
}

// finish_extremum gathers the winning row of each group. A group with no
// present value is missing.
inline QVector* finish_extremum(const QVector& col, const Grouping& g, const std::vector<int32_t>& best) {
    const size_t ng = best.size();
    std::vector<int32_t> rows(ng);
    for (size_t i = 0; i < ng; i++) rows[i] = best[i] < 0 ? 0 : best[i];
    QVector* out;
    if (g.nrows == 0) {
        out = q_vec_alloc(col.type, ng);
    } else {
        out = take(col, rows.data(), static_cast<int32_t>(ng));
    }
    for (size_t i = 0; i < ng; i++) {
        if (best[i] < 0 && !is_null(*out, static_cast<int32_t>(i))) set_null(*out, static_cast<int32_t>(i));
    }
    return out;
}

// agg_extremum_whole is the dense loop for a summarize with no group_by
// (tables v0.2 §4.1): the best value lives in a register, and a tie keeps
// the first. A group_of lookup per row is the slow path this replaces.
// best_in_blocks reduces rows [0, nrows) a block of the thread pool at a
// time: block(lo, hi, &best) finds a block's best (false when it has none),
// and the blocks are combined in order, a later one winning only when
// beats(best, its) holds, so a tie keeps the first row, as one loop does.
template <class T, class Block, class Beats>
inline bool best_in_blocks(int32_t nrows, Block block, Beats beats, T* out) {
    const quark::par::Plan plan = q_par_plan(nrows);
    const size_t nb = static_cast<size_t>(plan.nblocks);
    std::vector<T> part(nb);
    std::vector<char> found(nb, 0);
    q_par_run(plan, [&](long long lo, long long hi, long long b) {
        T best{};
        if (block(static_cast<int32_t>(lo), static_cast<int32_t>(hi), &best)) {
            part[static_cast<size_t>(b)] = best;
            found[static_cast<size_t>(b)] = 1;
        }
    });
    bool any = false;
    T best{};
    for (size_t b = 0; b < nb; b++) {
        if (!found[b]) continue;
        if (!any || beats(best, part[b])) best = part[b];
        any = true;
    }
    *out = best;
    return any;
}

// extremum_of finds the best present value of a fixed-width column: with no
// missing values, the branch-free loop of min(v) / max(v) (q_vec_extremum),
// with the same result.
template <class T>
inline bool extremum_of(const QVector& col, const T* x, int32_t nrows, bool want_max, T* out) {
    auto beats = [want_max](T best, T y) { return want_max ? y > best : y < best; };
    if (col.null_count == 0) {
        return best_in_blocks<T>(nrows, [&](int32_t lo, int32_t hi, T* best) {
            if (hi <= lo) return false;
            T b = x[lo];
            for (int32_t r = lo + 1; r < hi; r++) b = want_max ? (x[r] > b ? x[r] : b) : (x[r] < b ? x[r] : b);
            *best = b;
            return true;
        }, beats, out);
    }
    return best_in_blocks<T>(nrows, [&](int32_t lo, int32_t hi, T* best) {
        bool any = false;
        T b{};
        for (int32_t r = lo; r < hi; r++) {
            if (is_null(col, r)) continue;
            if (!any || beats(b, x[r])) {
                b = x[r];
                any = true;
            }
        }
        *best = b;
        return any;
    }, beats, out);
}

inline QVector* agg_extremum_whole(const QVector& col, int32_t nrows, bool want_max) {
    auto missing = [&](QVector* out) {
        set_null(*out, 0);
        return out;
    };
    switch (col.type) {
        case Type::I64: {
            QVector* out = q_vec_alloc(Type::I64, 1);
            int64_t best = 0;
            if (!extremum_of<int64_t>(col, q_vec_i64_data(col), nrows, want_max, &best)) return missing(out);
            q_vec_i64_data_mut(*out)[0] = best;
            return out;
        }
        case Type::DATE: {
            QVector* out = q_vec_alloc(Type::DATE, 1);
            int32_t best = 0;
            if (!extremum_of<int32_t>(col, q_vec_date_data(col), nrows, want_max, &best)) return missing(out);
            q_vec_date_data_mut(*out)[0] = best;
            return out;
        }
        case Type::F64: {
            const double* x = q_vec_f64_data(col);
            auto beats = [want_max](double best, double y) { return want_max ? y > best : y < best; };
            double best = 0;
            bool any;
            if (col.null_count == 0) {
                // A NaN never compares less or greater, so after the first
                // non-NaN value it never wins (as in q_vec_extremum).
                any = best_in_blocks<double>(nrows, [&](int32_t lo, int32_t hi, double* out) {
                    int32_t first = lo;
                    while (first < hi && std::isnan(x[first])) first++;
                    if (first == hi) return false;
                    double b = x[first];
                    for (int32_t r = first + 1; r < hi; r++) b = want_max ? (x[r] > b ? x[r] : b) : (x[r] < b ? x[r] : b);
                    *out = b;
                    return true;
                }, beats, &best);
            } else {
                any = best_in_blocks<double>(nrows, [&](int32_t lo, int32_t hi, double* out) {
                    bool found = false;
                    double b = 0;
                    for (int32_t r = lo; r < hi; r++) {
                        if (is_null(col, r) || std::isnan(x[r])) continue;
                        if (!found || beats(b, x[r])) {
                            b = x[r];
                            found = true;
                        }
                    }
                    *out = b;
                    return found;
                }, beats, &best);
            }
            QVector* out = q_vec_alloc(Type::F64, 1);
            if (!any) return missing(out);
            q_vec_f64_data_mut(*out)[0] = best;
            return out;
        }
        case Type::BOOL: {
            auto beats = [want_max](bool best, bool x) { return want_max ? x && !best : !x && best; };
            bool best = false;
            const bool any = best_in_blocks<bool>(nrows, [&](int32_t lo, int32_t hi, bool* out) {
                bool found = false;
                bool b = false;
                for (int32_t r = lo; r < hi; r++) {
                    if (is_null(col, r)) continue;
                    const bool x = q_vec_bool_at(col, static_cast<size_t>(r));
                    if (!found || beats(b, x)) {
                        b = x;
                        found = true;
                    }
                }
                *out = b;
                return found;
            }, beats, &best);
            QVector* out = q_vec_alloc(Type::BOOL, 1);
            if (!any) return missing(out);
            if (best) quark::vec::put_bit(out->values, 0, true);
            return out;
        }
        case Type::STR: {
            auto beats = [&](int32_t best, int32_t r) {
                const std::string_view xs = q_vec_str_at(col, static_cast<size_t>(r));
                const std::string_view ys = q_vec_str_at(col, static_cast<size_t>(best));
                return want_max ? ys < xs : xs < ys;
            };
            int32_t best = -1;
            const bool any = best_in_blocks<int32_t>(nrows, [&](int32_t lo, int32_t hi, int32_t* out) {
                int32_t b = -1;
                for (int32_t r = lo; r < hi; r++) {
                    if (is_null(col, r)) continue;
                    if (b < 0 || beats(b, r)) b = r;
                }
                *out = b;
                return b >= 0;
            }, beats, &best);
            if (!any) {
                QVector* out = q_vec_alloc(Type::STR, 1);
                return missing(out);
            }
            return take(col, &best, 1);
        }
    }
    fail("min/max of an unknown column type");
}

// agg_extremum (min / max): one typed loop per column type. A float loop
// skips NaN, so the comparisons that remain are raw doubles. Null for a
// group with no present value. Ties keep the first value.
// best_rows is, for each group, the first row holding its best present
// value (-1 when it has none): present(r) says whether row r counts, and
// beats(r, b) whether row r's value is strictly better than row b's. A
// later range's best replaces an earlier one's only when it beats it, so a
// tie keeps the first row.
template <class Present, class Beats>
inline std::vector<int32_t> best_rows(const Grouping& g, Present present, Beats beats) {
    return fold<int32_t>(
        g, -1,
        [present, beats](int32_t& b, int32_t r) {
            if (present(r) && (b < 0 || beats(r, b))) b = r;
        },
        [beats](int32_t& a, int32_t b) {
            if (b >= 0 && (a < 0 || beats(b, a))) a = b;
        });
}

// extremum_rows is best_rows for min (want_max false) or max, where
// less(a, b) says row a's value is below row b's. The direction is chosen
// here, not in the row loop.
template <class Present, class Less>
inline std::vector<int32_t> extremum_rows(const Grouping& g, bool want_max, Present present, Less less) {
    if (want_max) return best_rows(g, present, [less](int32_t r, int32_t b) { return less(b, r); });
    return best_rows(g, present, [less](int32_t r, int32_t b) { return less(r, b); });
}

inline QVector* agg_extremum(const QVector& col, const Grouping& g, bool want_max) {
    if (is_whole_table(g)) return agg_extremum_whole(col, g.nrows, want_max);
    const QVector* c = &col;
    auto present = [c](int32_t r) { return !is_null(*c, r); };
    std::vector<int32_t> best;
    switch (col.type) {
        case Type::I64: {
            const int64_t* x = q_vec_i64_data(col);
            best = extremum_rows(g, want_max, present, [x](int32_t a, int32_t b) { return x[a] < x[b]; });
            break;
        }
        case Type::DATE: {
            const int32_t* x = q_vec_date_data(col);
            best = extremum_rows(g, want_max, present, [x](int32_t a, int32_t b) { return x[a] < x[b]; });
            break;
        }
        case Type::F64: {
            const double* x = q_vec_f64_data(col);
            best = extremum_rows(
                g, want_max, [c, x](int32_t r) { return !is_null(*c, r) && !std::isnan(x[r]); },
                [x](int32_t a, int32_t b) { return x[a] < x[b]; });
            break;
        }
        case Type::BOOL:
            best = extremum_rows(g, want_max, present, [c](int32_t a, int32_t b) {
                return !q_vec_bool_at(*c, static_cast<size_t>(a)) && q_vec_bool_at(*c, static_cast<size_t>(b));
            });
            break;
        case Type::STR:
            best = extremum_rows(g, want_max, present, [c](int32_t a, int32_t b) {
                return q_vec_str_at(*c, static_cast<size_t>(a)) < q_vec_str_at(*c, static_cast<size_t>(b));
            });
            break;
    }
    return finish_extremum(col, g, best);
}

// project_table is a table of def whose columns are columns of t, shared
// (tables are immutable). cols has def->field_count indexes.
inline QTable* project_table(QTable* t, const QStructDef* def, const int* cols, int n) {
    if (def == nullptr || n != def->field_count) fail("internal: project column count");
    QTable* out = q_table_create(def, t->nrows);
    for (int i = 0; i < n; i++) {
        if (cols[i] < 0 || cols[i] >= t->ncols) fail("internal: project index");
        out->cols[i] = t->cols[cols[i]];
    }
    return out;
}

// distinct_rows keeps the first row of each key, in the order the keys
// first appear. key_cols has nkeys columns; equality is rows_equal.
inline QTable* distinct_rows(QTable* t, const int* key_cols, int nkeys) {
    const int32_t n = t->nrows;
    if (n == 0) return take_table(t, nullptr, 0);
    std::vector<const QVector*> keys(static_cast<size_t>(nkeys));
    for (int k = 0; k < nkeys; k++) keys[static_cast<size_t>(k)] = column(t, key_cols[k]);
    std::vector<int32_t> first = with_keys(n, keys, [&](auto hash, auto same) { return distinct_keys(n, hash, same, nullptr); });
    par_stable_sort(first.data(), first.size(), [](int32_t a, int32_t b) { return a < b; });
    return take_table(t, first.data(), static_cast<int32_t>(first.size()));
}

// agg_n_distinct counts keys per group: present values, plus one if any
// value is missing, plus one if any float is NaN. An empty group counts 0.
// The result is never missing. Equality is rows_equal, so 0.0 and -0.0
// are one key and two NaNs are one key.
//
// With many groups (at least 4,096), each part of group_parts counts the
// distinct values of its own groups, in a table of its own (any thread
// count: one table over every row would hold nearly a pair per row).
// Otherwise the distinct (group, value) pairs come from distinct_keys, and a
// group counts the rows that are a pair's first.
inline QVector* agg_n_distinct(const QVector& col, const Grouping& g) {
    const size_t ng = static_cast<size_t>(g.ngroups);
    QVector* out = q_vec_alloc(Type::I64, ng);
    if (g.nrows == 0) return out;
    int64_t* d = q_vec_i64_data_mut(*out);
    auto pair_hash = [&](int32_t r) { return mix64(hash_value(col, r) ^ (static_cast<uint64_t>(group_of(g, r)) * 0x9e3779b97f4a7c15ULL)); };
    auto same_pair = [&](int32_t a, int32_t b) { return group_of(g, a) == group_of(g, b) && rows_equal(col, a, b); };
    if (g.group_of_row != nullptr && ng >= 16) {
        const GroupParts& gp = group_parts(g);
        par_pieces(gp.nparts, 1, g.nrows, [&](long long lo, long long hi) {
            for (long long p = lo; p < hi; p++) {
                KeyTable t;
                bool added = false;
                for (int64_t i = gp.start[p]; i < gp.start[p + 1]; i++) {
                    const int32_t r = gp.rows[i];
                    t.find_or_add(pair_hash(r), r, same_pair, &added);
                    if (added) d[gp.gids[i]]++;
                }
            }
        });
        return out;
    }
    const std::vector<int32_t> pairs = distinct_keys(g.nrows, pair_hash, same_pair, nullptr);
    std::unique_ptr<char[]> is_first(new char[static_cast<size_t>(g.nrows)]());
    par_pieces(static_cast<long long>(pairs.size()), q_par_block, static_cast<long long>(pairs.size()), [&](long long lo, long long hi) {
        for (long long i = lo; i < hi; i++) is_first[static_cast<size_t>(pairs[static_cast<size_t>(i)])] = 1;
    });
    const std::vector<int64_t> n = fold<int64_t>(
        g, 0,
        [&](int64_t& s, int32_t r) {
            if (is_first[static_cast<size_t>(r)]) s++;
        },
        [](int64_t& a, int64_t b) { a += b; });
    std::copy(n.begin(), n.end(), d);
    return out;
}

inline QVector* agg_min(const QVector& col, const Grouping& g) { return agg_extremum(col, g, false); }
inline QVector* agg_max(const QVector& col, const Grouping& g) { return agg_extremum(col, g, true); }

// agg_sd is the sample standard deviation (divisor n - 1). Fewer than two
// present values is missing. Float groups use the same present-value order
// as mean (gather_f64); int and bool values are converted to double first.
// The square deviations go through q_sample_sd, which mean's vec builtin
// shares, so sd(v) and summarize(s = sd(e.x)) give the same bits.
inline QVector* agg_sd(const QVector& col, const Grouping& g) {
    if (col.type == Type::STR) fail("sd needs a number or bool column, got str");
    if (col.type == Type::DATE) fail("sd needs a number or bool column, got date");
    const size_t ng = static_cast<size_t>(g.ngroups);
    QVector* out = q_vec_alloc(Type::F64, ng);
    double* d = q_vec_f64_data_mut(*out);
    if (col.type == Type::F64 && is_whole_table(g)) {
        bool ok = false;
        const double s = q_sd_present(col, &ok);
        if (!ok) set_null(*out, 0);
        else d[0] = s;
        return out;
    }
    const bool ints = col.type == Type::I64;
    const GroupValues gv = col.type == Type::F64 ? gather_f64(col, g) : gather(g, [&](int32_t r, double* v) {
        if (is_null(col, r)) return false;
        *v = ints ? static_cast<double>(q_vec_i64_data(col)[r]) : (q_vec_bool_at(col, static_cast<size_t>(r)) ? 1.0 : 0.0);
        return true;
    });
    std::vector<char> miss(ng, 0);
    per_group(g, [&](long long lo, long long hi) {
        std::vector<double> sq;
        for (long long i = lo; i < hi; i++) {
            const size_t n = gv.count(static_cast<size_t>(i));
            if (n < 2) {
                miss[static_cast<size_t>(i)] = 1;
                continue;
            }
            if (sq.size() < n) sq.resize(n);
            d[i] = q_sample_sd_with(gv.of(static_cast<size_t>(i)), n, sq.data());
        }
    });
    mark_missing(*out, miss);
    return out;
}

} // namespace tbl
} // namespace quark

#endif // QUARK_TABLE_ENGINE_HPP
