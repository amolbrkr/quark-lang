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
// aggregates skip them. sum of no values is 0; mean, min and max of no values
// are null; count counts rows. Group keys and sort keys put nulls last.
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
#include <string_view>
#include <utility>
#include <vector>

namespace quark {
namespace tbl {

using Type = QVector::Type;

[[noreturn]] inline void fail(const char* what) {
    q_runtime_reportf("runtime error: %s\n", what);
    std::exit(1);
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

// take gathers rows idx[0..n) of src into a new column.
inline QVector* take(const QVector& src, const int32_t* idx, int32_t n) {
    QVector* dst = nullptr;
    switch (src.type) {
        case Type::F64:
        case Type::I64: {
            dst = q_vec_alloc(src.type, static_cast<size_t>(n));
            const uint64_t* s = reinterpret_cast<const uint64_t*>(src.values);
            uint64_t* d = reinterpret_cast<uint64_t*>(dst->values);
            for (int32_t i = 0; i < n; i++) d[i] = s[idx[i]];
            break;
        }
        case Type::BOOL: {
            dst = q_vec_alloc(Type::BOOL, static_cast<size_t>(n));
            for (int32_t i = 0; i < n; i++) {
                if (quark::vec::get_bit(src.values, static_cast<size_t>(idx[i]))) {
                    quark::vec::put_bit(dst->values, static_cast<size_t>(i), true);
                }
            }
            break;
        }
        case Type::STR: {
            size_t bytes = 0;
            for (int32_t i = 0; i < n; i++) {
                bytes += static_cast<size_t>(src.offsets[idx[i] + 1] - src.offsets[idx[i]]);
            }
            if (bytes > static_cast<size_t>(std::numeric_limits<int32_t>::max())) {
                fail("str column data exceeds 2 GiB");
            }
            dst = q_vec_alloc(Type::STR, static_cast<size_t>(n), bytes);
            int32_t pos = 0;
            for (int32_t i = 0; i < n; i++) {
                const int32_t a = src.offsets[idx[i]];
                const int32_t len = src.offsets[idx[i] + 1] - a;
                if (len > 0) std::memcpy(dst->values + pos, src.values + a, static_cast<size_t>(len));
                pos += len;
                dst->offsets[i + 1] = pos;
            }
            break;
        }
    }
    if (src.null_count > 0) {
        for (int32_t i = 0; i < n; i++) {
            if (is_null(src, idx[i])) set_null(*dst, i);
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

// rows_equal reports whether rows a and b hold the same key value (two nulls
// are equal, as are two NaNs, and 0.0 and -0.0).
inline bool rows_equal(const QVector& v, int32_t a, int32_t b) {
    const bool an = is_null(v, a), bn = is_null(v, b);
    if (an || bn) return an == bn;
    switch (v.type) {
        case Type::I64: return q_vec_i64_data(v)[a] == q_vec_i64_data(v)[b];
        case Type::F64: {
            const double x = q_vec_f64_data(v)[a], y = q_vec_f64_data(v)[b];
            return x == y || (std::isnan(x) && std::isnan(y));
        }
        case Type::BOOL: return q_vec_bool_at(v, static_cast<size_t>(a)) == q_vec_bool_at(v, static_cast<size_t>(b));
        case Type::STR: return q_vec_str_at(v, static_cast<size_t>(a)) == q_vec_str_at(v, static_cast<size_t>(b));
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
            std::stable_sort(pairs.begin(), pairs.end(), [](const auto& x, const auto& y) { return x.first > y.first; });
        } else {
            std::stable_sort(pairs.begin(), pairs.end(), [](const auto& x, const auto& y) { return x.first < y.first; });
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
    std::sort(order.begin(), order.end(), [&](int32_t a, int32_t b) {
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
    if (nkeys == 1 && (k0.type == Type::I64 || k0.type == Type::F64 || k0.type == Type::BOOL)) {
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
    std::stable_sort(perm, perm + n, [&](int32_t a, int32_t b) {
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

// Grouping assigns every row of a table to a group. Groups are numbered in
// key order (ascending, nulls last). An ungrouped table is one group of all
// rows, with group_of_row == nullptr.
struct Grouping {
    int32_t nrows = 0;
    int32_t ngroups = 0;
    int32_t* group_of_row = nullptr; // [nrows]: the group of each row
    int32_t* first_row = nullptr;    // [ngroups]: a row of each group (its first)
};

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

inline uint64_t hash_value(const QVector& v, int32_t i) {
    if (is_null(v, i)) return 0x9e3779b97f4a7c15ULL;
    switch (v.type) {
        case Type::I64: return mix64(static_cast<uint64_t>(q_vec_i64_data(v)[i]));
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
            uint64_t h = 0xcbf29ce484222325ULL; // FNV-1a
            for (unsigned char ch : s) {
                h ^= ch;
                h *= 0x100000001b3ULL;
            }
            return mix64(h);
        }
    }
    return 0;
}

// group_rows groups the rows of t by the key columns.
inline Grouping group_rows(QTable* t, const int* key_cols, int nkeys) {
    const int32_t n = t->nrows;
    if (nkeys == 0) return whole_table(n);
    std::vector<const QVector*> keys(static_cast<size_t>(nkeys));
    for (int k = 0; k < nkeys; k++) keys[static_cast<size_t>(k)] = column(t, key_cols[k]);

    std::vector<uint64_t> row_hash(static_cast<size_t>(n));
    for (int32_t r = 0; r < n; r++) {
        uint64_t h = 0;
        for (const QVector* k : keys) h = mix64(h * 31 + hash_value(*k, r));
        row_hash[static_cast<size_t>(r)] = h;
    }

    // Open addressing; slots hold group id + 1 (0 = empty).
    size_t cap = 16;
    while (cap < static_cast<size_t>(n) * 2) cap <<= 1;
    std::vector<int32_t> slots(cap, 0);
    std::vector<uint64_t> group_hash;
    std::vector<int32_t> first;
    int32_t* group_of_row = alloc_i32(static_cast<size_t>(n));
    const size_t mask = cap - 1;
    for (int32_t r = 0; r < n; r++) {
        const uint64_t h = row_hash[static_cast<size_t>(r)];
        size_t s = static_cast<size_t>(h) & mask;
        while (true) {
            const int32_t slot = slots[s];
            if (slot == 0) {
                const int32_t g = static_cast<int32_t>(first.size());
                slots[s] = g + 1;
                first.push_back(r);
                group_hash.push_back(h);
                group_of_row[r] = g;
                break;
            }
            const int32_t g = slot - 1;
            if (group_hash[static_cast<size_t>(g)] == h) {
                bool same = true;
                for (const QVector* k : keys) {
                    if (!rows_equal(*k, r, first[static_cast<size_t>(g)])) {
                        same = false;
                        break;
                    }
                }
                if (same) {
                    group_of_row[r] = g;
                    break;
                }
            }
            s = (s + 1) & mask;
        }
    }

    // Renumber the groups in key order.
    const int32_t ng = static_cast<int32_t>(first.size());
    std::vector<int32_t> order(static_cast<size_t>(ng));
    for (int32_t g = 0; g < ng; g++) order[static_cast<size_t>(g)] = g;
    std::stable_sort(order.begin(), order.end(), [&](int32_t x, int32_t y) {
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
    for (int32_t i = 0; i < ng; i++) {
        rank[static_cast<size_t>(order[static_cast<size_t>(i)])] = i;
        out.first_row[i] = first[static_cast<size_t>(order[static_cast<size_t>(i)])];
    }
    for (int32_t r = 0; r < n; r++) group_of_row[r] = rank[static_cast<size_t>(group_of_row[r])];
    out.group_of_row = group_of_row;
    return out;
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

// agg_count is the number of rows in each group (n()).
inline QVector* agg_count(const Grouping& g) {
    QVector* out = q_vec_alloc(Type::I64, static_cast<size_t>(g.ngroups));
    int64_t* d = q_vec_i64_data_mut(*out);
    if (!g.group_of_row) {
        if (g.ngroups == 1) d[0] = g.nrows;
        return out;
    }
    for (int32_t r = 0; r < g.nrows; r++) d[g.group_of_row[r]]++;
    return out;
}

// agg_count_values is the number of non-missing values of col in each group.
inline QVector* agg_count_values(const QVector& col, const Grouping& g) {
    QVector* out = q_vec_alloc(Type::I64, static_cast<size_t>(g.ngroups));
    int64_t* d = q_vec_i64_data_mut(*out);
    for (int32_t r = 0; r < g.nrows; r++) {
        const bool missing = col.type == Type::F64 ? is_missing_f64(col, r) : is_null(col, r);
        if (!missing) d[group_of(g, r)]++;
    }
    return out;
}

// is_whole_table reports whether g is the single group of whole_table, whose
// aggregates are the vec reductions of the column itself.
inline bool is_whole_table(const Grouping& g) { return g.ngroups == 1 && g.group_of_row == nullptr; }

// GroupValues are the present values of a float column, group by group in
// row order: group i's are vals[start[i] .. start[i + 1]).
struct GroupValues {
    std::vector<int64_t> start;
    std::vector<double> vals;
    size_t count(size_t i) const { return static_cast<size_t>(start[i + 1] - start[i]); }
    const double* of(size_t i) const { return vals.data() + start[i]; }
};

// gather_f64 collects each group's present values (a counting sort of the
// rows by group), so a group is summed exactly as sum() sums a vec of the
// same values (q_vec_f64_present_sum).
inline GroupValues gather_f64(const QVector& col, const Grouping& g) {
    const size_t ng = static_cast<size_t>(g.ngroups);
    GroupValues gv;
    gv.start.assign(ng + 1, 0);
    for (int32_t r = 0; r < g.nrows; r++) {
        if (!is_missing_f64(col, r)) gv.start[static_cast<size_t>(group_of(g, r)) + 1]++;
    }
    for (size_t i = 0; i < ng; i++) gv.start[i + 1] += gv.start[i];
    gv.vals.resize(static_cast<size_t>(gv.start[ng]));
    std::vector<int64_t> next(gv.start.begin(), gv.start.end() - 1);
    const double* x = q_vec_f64_data(col);
    for (int32_t r = 0; r < g.nrows; r++) {
        if (!is_missing_f64(col, r)) gv.vals[static_cast<size_t>(next[static_cast<size_t>(group_of(g, r))]++)] = x[r];
    }
    return gv;
}

// agg_sum: int and bool columns give an int (bools count as 0/1), float
// columns a float, summed pairwise (core/sum.hpp). A group with no values
// sums to 0. Int overflow is fatal.
inline QVector* agg_sum(const QVector& col, const Grouping& g) {
    const size_t ng = static_cast<size_t>(g.ngroups);
    switch (col.type) {
        case Type::I64: {
            QVector* out = q_vec_alloc(Type::I64, ng);
            int64_t* d = q_vec_i64_data_mut(*out);
            const int64_t* x = q_vec_i64_data(col);
            for (int32_t r = 0; r < g.nrows; r++) {
                if (is_null(col, r)) continue;
                int64_t& acc = d[group_of(g, r)];
                if (__builtin_add_overflow(acc, x[r], &acc)) {
                    q_runtime_reportf("runtime error: integer overflow in 'sum'\n");
                    std::exit(1);
                }
            }
            return out;
        }
        case Type::BOOL: {
            QVector* out = q_vec_alloc(Type::I64, ng);
            int64_t* d = q_vec_i64_data_mut(*out);
            for (int32_t r = 0; r < g.nrows; r++) {
                if (!is_null(col, r) && q_vec_bool_at(col, static_cast<size_t>(r))) d[group_of(g, r)]++;
            }
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
            for (size_t i = 0; i < ng; i++) d[i] = q_pairwise_sum(gv.of(i), gv.count(i));
            return out;
        }
        case Type::STR: break;
    }
    fail("sum needs a number or bool column, got str");
}

// agg_mean: a float per group; null for a group with no values. Float
// values are summed pairwise, int values exactly (128-bit), and bools give the
// share of true values, as q_vec_mean_present does for a whole column.
inline QVector* agg_mean(const QVector& col, const Grouping& g) {
    if (col.type == Type::STR) fail("mean needs a number or bool column, got str");
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
        for (size_t i = 0; i < ng; i++) {
            const size_t n = gv.count(i);
            if (n == 0) set_null(*out, static_cast<int32_t>(i));
            else d[i] = q_pairwise_sum(gv.of(i), n) / static_cast<double>(n);
        }
        return out;
    }
    std::vector<__int128> acc(ng, 0);  // int values, or the number of trues
    std::vector<int64_t> cnt(ng, 0);
    for (int32_t r = 0; r < g.nrows; r++) {
        if (is_null(col, r)) continue;
        const size_t gi = static_cast<size_t>(group_of(g, r));
        acc[gi] += col.type == Type::I64 ? q_vec_i64_data(col)[r] : (q_vec_bool_at(col, static_cast<size_t>(r)) ? 1 : 0);
        cnt[gi]++;
    }
    for (size_t i = 0; i < ng; i++) {
        if (cnt[i] == 0) set_null(*out, static_cast<int32_t>(i));
        else d[i] = q_i128_to_double(acc[i]) / static_cast<double>(cnt[i]);
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

// agg_extremum (min / max): one typed loop per column type. A float loop
// skips NaN, so the comparisons that remain are raw doubles. Null for a
// group with no present value. Ties keep the first value.
inline QVector* agg_extremum(const QVector& col, const Grouping& g, bool want_max) {
    const size_t ng = static_cast<size_t>(g.ngroups);
    std::vector<int32_t> best(ng, -1);
    switch (col.type) {
        case Type::I64: {
            const int64_t* x = q_vec_i64_data(col);
            for (int32_t r = 0; r < g.nrows; r++) {
                if (is_null(col, r)) continue;
                int32_t& b = best[static_cast<size_t>(group_of(g, r))];
                if (b < 0 || (want_max ? x[r] > x[b] : x[r] < x[b])) b = r;
            }
            break;
        }
        case Type::F64: {
            const double* x = q_vec_f64_data(col);
            for (int32_t r = 0; r < g.nrows; r++) {
                if (is_null(col, r) || std::isnan(x[r])) continue;
                int32_t& b = best[static_cast<size_t>(group_of(g, r))];
                if (b < 0 || (want_max ? x[r] > x[b] : x[r] < x[b])) b = r;
            }
            break;
        }
        case Type::BOOL: {
            for (int32_t r = 0; r < g.nrows; r++) {
                if (is_null(col, r)) continue;
                int32_t& b = best[static_cast<size_t>(group_of(g, r))];
                const int x = q_vec_bool_at(col, static_cast<size_t>(r)) ? 1 : 0;
                const int y = b < 0 ? 0 : (q_vec_bool_at(col, static_cast<size_t>(b)) ? 1 : 0);
                if (b < 0 || (want_max ? x > y : x < y)) b = r;
            }
            break;
        }
        case Type::STR: {
            for (int32_t r = 0; r < g.nrows; r++) {
                if (is_null(col, r)) continue;
                int32_t& b = best[static_cast<size_t>(group_of(g, r))];
                if (b < 0) {
                    b = r;
                    continue;
                }
                const std::string_view xs = q_vec_str_at(col, static_cast<size_t>(r));
                const std::string_view ys = q_vec_str_at(col, static_cast<size_t>(b));
                if (want_max ? ys < xs : xs < ys) b = r;
            }
            break;
        }
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
    size_t cap = 16;
    while (cap < static_cast<size_t>(n) * 2) cap <<= 1;
    std::vector<int32_t> slots(cap, 0);
    std::vector<uint64_t> group_hash;
    std::vector<int32_t> first;
    const size_t mask = cap - 1;
    for (int32_t r = 0; r < n; r++) {
        uint64_t h = 0;
        for (const QVector* k : keys) h = mix64(h * 31 + hash_value(*k, r));
        size_t s = static_cast<size_t>(h) & mask;
        while (true) {
            const int32_t slot = slots[s];
            if (slot == 0) {
                slots[s] = static_cast<int32_t>(first.size()) + 1;
                first.push_back(r);
                group_hash.push_back(h);
                break;
            }
            const int32_t g = slot - 1;
            if (group_hash[static_cast<size_t>(g)] == h) {
                bool same = true;
                for (const QVector* k : keys) {
                    if (!rows_equal(*k, r, first[static_cast<size_t>(g)])) {
                        same = false;
                        break;
                    }
                }
                if (same) break;
            }
            s = (s + 1) & mask;
        }
    }
    return take_table(t, first.data(), static_cast<int32_t>(first.size()));
}

// agg_n_distinct counts keys per group: present values, plus one if any
// value is missing, plus one if any float is NaN. An empty group counts 0.
// The result is never missing. Equality is rows_equal, so 0.0 and -0.0
// are one key and two NaNs are one key.
inline QVector* agg_n_distinct(const QVector& col, const Grouping& g) {
    const size_t ng = static_cast<size_t>(g.ngroups);
    QVector* out = q_vec_alloc(Type::I64, ng);
    if (g.nrows == 0) return out;
    int64_t* d = q_vec_i64_data_mut(*out);
    size_t cap = 16;
    while (cap < static_cast<size_t>(g.nrows) * 2) cap <<= 1;
    std::vector<int32_t> slots(cap, 0);
    std::vector<uint64_t> hashes;
    std::vector<int32_t> rep;
    const size_t mask = cap - 1;
    for (int32_t r = 0; r < g.nrows; r++) {
        const int32_t gi = group_of(g, r);
        const uint64_t h = mix64(hash_value(col, r) ^ (static_cast<uint64_t>(gi) * 0x9e3779b97f4a7c15ULL));
        size_t s = static_cast<size_t>(h) & mask;
        while (true) {
            const int32_t slot = slots[s];
            if (slot == 0) {
                slots[s] = static_cast<int32_t>(rep.size()) + 1;
                rep.push_back(r);
                hashes.push_back(h);
                d[gi]++;
                break;
            }
            const int32_t k = slot - 1;
            if (hashes[static_cast<size_t>(k)] == h && group_of(g, rep[static_cast<size_t>(k)]) == gi &&
                rows_equal(col, r, rep[static_cast<size_t>(k)])) {
                break;
            }
            s = (s + 1) & mask;
        }
    }
    return out;
}

inline QVector* agg_min(const QVector& col, const Grouping& g) { return agg_extremum(col, g, false); }
inline QVector* agg_max(const QVector& col, const Grouping& g) { return agg_extremum(col, g, true); }

} // namespace tbl
} // namespace quark

#endif // QUARK_TABLE_ENGINE_HPP
