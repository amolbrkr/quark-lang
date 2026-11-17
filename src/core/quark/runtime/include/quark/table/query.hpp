// quark/table/query.hpp - what generated code calls for table queries
// (specs/tables_v0_1_spec.md §4, lower/query.go).
//
// The compiler lowers each verb expression into a typed loop over the input
// table's column buffers: per row, it tests the validity of the columns the
// expression reads, computes the value, and pushes it (or a missing value)
// into a ColOut, or the row's index into a RowSet. The verbs themselves are
// the engine's kernels (engine.hpp). Included by generated code only when the
// program has a query.
#ifndef QUARK_TABLE_QUERY_HPP
#define QUARK_TABLE_QUERY_HPP

#include "engine.hpp"
#include "../builtins/repr.hpp"
#include "../builtins/contains.hpp"

#include <cstring>
#include <initializer_list>
#include <string>
#include <string_view>
#include <vector>

namespace quark {
namespace tbl {

// RowSet collects the rows a filter keeps, in order.
struct RowSet {
    int32_t* idx = nullptr;
    int32_t n = 0;
};

// ColOut builds one column, a row at a time in row order.
struct ColOut {
    QVector::Type type = QVector::Type::F64;
    QVector* v = nullptr;          // F64, I64, BOOL: allocated for every row
    std::string bytes;             // STR
    std::vector<int32_t> offsets;  // STR
    std::vector<int32_t> nulls;
    int32_t n = 0;                 // rows pushed
};

} // namespace tbl
} // namespace quark

// The table a query step reads: a tbl[R] value (proven by the compiler).
inline QTable* q_tbl_of(QValue v) {
    if (v.type != QValue::VAL_TABLE || !v.data.table_val) {
        q_runtime_reportf("runtime error: internal: a query's source is a %s, not a table\n", q_type_name(v.type));
        q_exit(1);
    }
    return v.data.table_val;
}

inline long long q_tbl_rows(const QTable* t) { return t->nrows; }

inline const QVector& q_tbl_col(const QTable* t, int c) { return *quark::tbl::column(t, c); }

// Whether row i of a column holds a value (validity only: NaN is a value
// here, as in arithmetic; aggregates skip it).
inline bool q_tbl_present(const QVector& v, long long i) { return !quark::tbl::is_null(v, static_cast<int32_t>(i)); }

// A str cell as a view of the column buffer. No Quark string is allocated.
inline std::string_view q_tbl_str_view(const QVector& v, long long i) {
    return q_vec_str_at(v, static_cast<size_t>(i));
}

// q_str_view_box allocates a Quark string for a cell that is stored or passed
// to a function. Str cells hold no NUL byte, which the reader rejects.
inline QValue q_str_view_box(std::string_view s) {
    char* out = static_cast<char*>(q_malloc_atomic(s.size() + 1));
    if (!s.empty()) std::memcpy(out, s.data(), s.size());
    out[s.size()] = '\0';
    return qv_string_own(out);
}

inline QValue q_tbl_str(const QVector& v, long long i) { return q_str_view_box(q_tbl_str_view(v, i)); }

// q_sv_holds reports whether the order of two views matches op ("==", "<", ...).
inline bool q_sv_holds(int c, const char* op) {
    switch (op[0]) {
        case '=': return c == 0;
        case '!': return c != 0;
        case '<': return op[1] == '=' ? c <= 0 : c < 0;
        case '>': return op[1] == '=' ? c >= 0 : c > 0;
        default: return false;
    }
}

inline int q_sv_ord(std::string_view a, std::string_view b) {
    if (a < b) return -1;
    if (b < a) return 1;
    return 0;
}

inline bool q_sv_cmp(std::string_view a, std::string_view b, const char* op) {
    return q_sv_holds(q_sv_ord(a, b), op);
}

// q_sv_cmp_val compares a cell view with a Quark value. == and != are false
// and true when the value is not a string; an ordered comparison of a
// non-string is the same error as q_lt.
inline bool q_sv_cmp_val(std::string_view a, QValue b, const char* op) {
    const bool ordered = op[0] == '<' || op[0] == '>';
    if (b.type != QValue::VAL_STRING || b.data.string_val == nullptr) {
        if (!ordered) return op[0] == '!';
        q_runtime_reportf("runtime error: operator '%s' expects two numbers or two strings\n", op);
        q_exit(1);
    }
    return q_sv_holds(q_sv_ord(a, std::string_view(b.data.string_val)), op);
}

inline bool q_vec_dense(const QVector& v) { return v.null_count == 0; }

// ------------------------------------------------------------
// filter

inline quark::tbl::RowSet q_rows_new(long long n) {
    quark::tbl::RowSet s;
    s.idx = quark::tbl::alloc_i32(static_cast<size_t>(n));
    return s;
}

inline void q_rows_push(quark::tbl::RowSet& s, long long i) { s.idx[s.n++] = static_cast<int32_t>(i); }

// q_rows_keep records row i when keep is true, without a branch on keep.
inline void q_rows_keep(quark::tbl::RowSet& s, long long i, bool keep) {
    s.idx[s.n] = static_cast<int32_t>(i);
    s.n += static_cast<int32_t>(keep);
}

inline QTable* q_tbl_take(QTable* t, const quark::tbl::RowSet& s) { return quark::tbl::take_table(t, s.idx, s.n); }

// ------------------------------------------------------------
// mutate

inline quark::tbl::ColOut q_col_new(int dtype, long long n) {
    quark::tbl::ColOut b;
    b.type = static_cast<QVector::Type>(dtype);
    if (b.type == QVector::Type::STR) {
        b.offsets.reserve(static_cast<size_t>(n) + 1);
        b.offsets.push_back(0);
    } else {
        b.v = q_vec_alloc(b.type, static_cast<size_t>(n));
    }
    return b;
}

inline void q_col_push_float(quark::tbl::ColOut& b, double x) { q_vec_f64_data_mut(*b.v)[b.n++] = x; }
inline void q_col_push_int(quark::tbl::ColOut& b, long long x) { q_vec_i64_data_mut(*b.v)[b.n++] = static_cast<int64_t>(x); }
inline void q_col_push_date(quark::tbl::ColOut& b, int32_t x) { q_vec_date_data_mut(*b.v)[b.n++] = x; }
inline void q_col_push_bool(quark::tbl::ColOut& b, bool x) {
    if (x) quark::vec::put_bit(b.v->values, static_cast<size_t>(b.n), true);
    b.n++;
}
inline void q_col_push_str(quark::tbl::ColOut& b, QValue s) {
    if (s.type != QValue::VAL_STRING || !s.data.string_val) {
        q_runtime_reportf("runtime error: internal: a str column got a %s\n", q_type_name(s.type));
        q_exit(1);
    }
    b.bytes += s.data.string_val;
    if (b.bytes.size() > static_cast<size_t>(INT32_MAX)) quark::tbl::fail("a str column holds more than 2 GiB of text");
    b.offsets.push_back(static_cast<int32_t>(b.bytes.size()));
    b.n++;
}
inline void q_col_push_null(quark::tbl::ColOut& b) {
    b.nulls.push_back(b.n);
    if (b.type == QVector::Type::STR) b.offsets.push_back(static_cast<int32_t>(b.bytes.size()));
    b.n++;
}

inline QVector* q_col_finish(quark::tbl::ColOut& b) {
    QVector* v = b.v;
    if (b.type == QVector::Type::STR) {
        v = q_vec_alloc(QVector::Type::STR, static_cast<size_t>(b.n), b.bytes.size());
        if (!b.bytes.empty()) std::memcpy(v->values, b.bytes.data(), b.bytes.size());
        std::memcpy(v->offsets, b.offsets.data(), b.offsets.size() * sizeof(int32_t));
    }
    for (int32_t i : b.nulls) quark::tbl::set_null(*v, i);
    return v;
}

// q_tbl_put is t with column index set to col, as a table of row def: the
// column replaced when def has as many columns as t, else appended (index
// is then t's column count).
inline QTable* q_tbl_put(QTable* t, const QStructDef* def, int index, QVector* col) {
    QTable* out = q_table_create(def, t->nrows);
    for (int c = 0; c < t->ncols; c++) out->cols[c] = t->cols[c];
    out->cols[index] = qv_vector_from(col);
    return out;
}

// ------------------------------------------------------------
// sort, head, tail

// keys holds (column, descending) pairs.
inline QTable* q_tbl_sort(QTable* t, std::initializer_list<int> keys) {
    std::vector<quark::tbl::SortKey> ks;
    for (auto it = keys.begin(); it != keys.end(); it += 2) ks.push_back({*it, *(it + 1) != 0});
    return quark::tbl::sort_table(t, ks.data(), static_cast<int>(ks.size()));
}

inline QTable* q_tbl_slice(QTable* t, long long n, bool from_end, const char* verb) {
    if (n < 0) {
        q_runtime_reportf("runtime error: %s(n) needs n >= 0, got %lld\n", verb, n);
        q_exit(1);
    }
    const int32_t rows = t->nrows;
    const int32_t take = n < rows ? static_cast<int32_t>(n) : rows;
    int32_t* idx = quark::tbl::alloc_i32(static_cast<size_t>(take));
    const int32_t start = from_end ? rows - take : 0;
    for (int32_t i = 0; i < take; i++) idx[i] = start + i;
    return quark::tbl::take_table(t, idx, take);
}

inline QTable* q_tbl_head(QTable* t, long long n) { return q_tbl_slice(t, n, false, "head"); }
inline QTable* q_tbl_tail(QTable* t, long long n) { return q_tbl_slice(t, n, true, "tail"); }

// ------------------------------------------------------------
// group_by + summarize

inline quark::tbl::Grouping q_tbl_group(QTable* t, std::initializer_list<int> keys) {
    std::vector<int> ks(keys);
    return quark::tbl::group_rows(t, ks.data(), static_cast<int>(ks.size()));
}

// q_tbl_group_vecs groups by key columns that may be computed (a QVector*
// per key, length nrows). q_tbl_col_ptr is a table column; a computed key
// is a column the query built.
inline const QVector* q_tbl_col_ptr(const QTable* t, int c) { return quark::tbl::column(t, c); }

inline quark::tbl::Grouping q_tbl_group_vecs(long long nrows, std::initializer_list<const QVector*> keys) {
    return quark::tbl::group_by_vecs(static_cast<int32_t>(nrows), keys.begin(), static_cast<int>(keys.size()));
}

inline quark::tbl::Grouping q_tbl_whole(QTable* t) { return quark::tbl::whole_table(t->nrows); }

inline long long q_tbl_ngroups(const quark::tbl::Grouping& g) { return g.ngroups; }

inline long long q_tbl_group_of(const quark::tbl::Grouping& g, long long row) {
    return quark::tbl::group_of(g, static_cast<int32_t>(row));
}

inline QVector* q_tbl_group_key(QTable* t, const quark::tbl::Grouping& g, int c) { return quark::tbl::group_keys(t, g, c); }

// q_tbl_key_col is group_keys for a column that isn't one of the table's
// (a computed group key).
inline QVector* q_tbl_key_col(const QVector* col, const quark::tbl::Grouping& g) {
    return quark::tbl::take(*col, g.first_row, g.ngroups);
}

inline QVector* q_tbl_agg_count(const quark::tbl::Grouping& g) { return quark::tbl::agg_count(g); }
inline QVector* q_tbl_agg_count_values(QTable* t, int c, const quark::tbl::Grouping& g) {
    return quark::tbl::agg_count_values(*quark::tbl::column(t, c), g);
}
inline QVector* q_tbl_agg_count_values_vec(const QVector* col, const quark::tbl::Grouping& g) {
    return quark::tbl::agg_count_values(*col, g);
}
inline QVector* q_tbl_agg_sum(QTable* t, int c, const quark::tbl::Grouping& g) { return quark::tbl::agg_sum(*quark::tbl::column(t, c), g); }
inline QVector* q_tbl_agg_sum_vec(const QVector* col, const quark::tbl::Grouping& g) { return quark::tbl::agg_sum(*col, g); }
inline QVector* q_tbl_agg_mean(QTable* t, int c, const quark::tbl::Grouping& g) { return quark::tbl::agg_mean(*quark::tbl::column(t, c), g); }
inline QVector* q_tbl_agg_mean_vec(const QVector* col, const quark::tbl::Grouping& g) { return quark::tbl::agg_mean(*col, g); }
inline QVector* q_tbl_agg_sd(QTable* t, int c, const quark::tbl::Grouping& g) { return quark::tbl::agg_sd(*quark::tbl::column(t, c), g); }
inline QVector* q_tbl_agg_sd_vec(const QVector* col, const quark::tbl::Grouping& g) { return quark::tbl::agg_sd(*col, g); }
inline QVector* q_tbl_agg_min(QTable* t, int c, const quark::tbl::Grouping& g) { return quark::tbl::agg_min(*quark::tbl::column(t, c), g); }
inline QVector* q_tbl_agg_min_vec(const QVector* col, const quark::tbl::Grouping& g) { return quark::tbl::agg_min(*col, g); }
inline QVector* q_tbl_agg_max(QTable* t, int c, const quark::tbl::Grouping& g) { return quark::tbl::agg_max(*quark::tbl::column(t, c), g); }
inline QVector* q_tbl_agg_max_vec(const QVector* col, const quark::tbl::Grouping& g) { return quark::tbl::agg_max(*col, g); }
inline QVector* q_tbl_agg_n_distinct(QTable* t, int c, const quark::tbl::Grouping& g) {
    return quark::tbl::agg_n_distinct(*quark::tbl::column(t, c), g);
}
inline QVector* q_tbl_agg_n_distinct_vec(const QVector* col, const quark::tbl::Grouping& g) {
    return quark::tbl::agg_n_distinct(*col, g);
}

// A group's aggregate value, read by the row loop. The vector is one value
// per group; i is the group index (0 for the whole table).
inline const QVector& q_vec_deref(const QVector* v) { return *v; }
inline bool q_grp_dense(const QVector* v) { return v->null_count == 0; }
inline bool q_grp_present(const QVector* v, long long i) { return q_tbl_present(*v, i); }
inline double q_grp_f64(const QVector* v, long long i) { return q_vec_f64_data(*v)[i]; }
inline long long q_grp_i64(const QVector* v, long long i) { return q_vec_i64_data(*v)[i]; }
inline bool q_grp_bool(const QVector* v, long long i) { return q_vec_bool_at(*v, static_cast<size_t>(i)); }
inline int32_t q_grp_date(const QVector* v, long long i) { return q_vec_date_data(*v)[i]; }
inline std::string_view q_grp_str(const QVector* v, long long i) { return q_tbl_str_view(*v, i); }

// q_tbl_project builds a table of def from columns of t (shared, not copied).
inline QTable* q_tbl_project(QTable* t, const QStructDef* def, std::initializer_list<int> cols) {
    std::vector<int> c(cols);
    return quark::tbl::project_table(t, def, c.data(), static_cast<int>(c.size()));
}

// q_tbl_distinct keeps the first row of each key, in first-seen order.
inline QTable* q_tbl_distinct(QTable* t, std::initializer_list<int> keys) {
    std::vector<int> k(keys);
    return quark::tbl::distinct_rows(t, k.data(), static_cast<int>(k.size()));
}

// q_tbl_drop_na keeps the rows where every listed column is present
// (tables v0.3 D47). A NaN is present. A table with nothing to drop is
// returned as it is (tables are immutable).
inline QTable* q_tbl_drop_na(QTable* t, std::initializer_list<int> cols) {
    std::vector<const QVector*> gaps;
    for (int c : cols) {
        const QVector* v = quark::tbl::column(t, c);
        if (v->null_count != 0) gaps.push_back(v);
    }
    if (gaps.empty()) return t;
    quark::tbl::RowSet s = q_rows_new(t->nrows);
    for (int32_t r = 0; r < t->nrows; r++) {
        bool keep = true;
        for (const QVector* v : gaps) {
            if (quark::tbl::is_null(*v, r)) {
                keep = false;
                break;
            }
        }
        if (keep) q_rows_push(s, r);
    }
    return q_tbl_take(t, s);
}

inline QTable* q_tbl_make(const QStructDef* def, std::initializer_list<QVector*> cols) {
    std::vector<QVector*> cs(cols);
    return quark::tbl::make_table(def, cs.data());
}

// ------------------------------------------------------------
// xs.contains(x) in a verb (tables v0.3 D48): when xs is the same for every
// row, a hash set of its elements is built the first time a row needs it,
// and each row looks x up. Equality is ==, as in q_list_contains: a float
// that is an exact int is stored as that int, so 2 and 2.0 are one key; NaN
// and missing vec entries are never stored, so they never match. A list of
// unknown element type may hold other values (null, a list): they are kept
// aside and compared with == when x isn't a scalar, so the result is the
// linear scan's (the compiler rejects element types it knows aren't
// scalars).

namespace quark {
namespace tbl {

struct LookupSet {
    enum Kind : uint8_t { EMPTY = 0, INT, FLT, BOOL, STR, DATE };
    struct Slot {
        uint8_t kind = EMPTY;
        uint64_t bits = 0;
        std::string_view s;
    };
    // src keeps the list or vec, and so the strings slots point into,
    // reachable while the set is (the set is a local of the step).
    QValue src = qv_null();
    std::vector<Slot> slots;
    size_t mask = 0;
    std::vector<QValue> others; // list elements that aren't scalars, in order
};

inline uint64_t lookup_hash(uint8_t kind, uint64_t bits) { return mix64(bits ^ (static_cast<uint64_t>(kind) * 0x9e3779b97f4a7c15ULL)); }

inline uint64_t lookup_hash_str(std::string_view s) {
    uint64_t h = 0xcbf29ce484222325ULL; // FNV-1a
    for (unsigned char ch : s) {
        h ^= ch;
        h *= 0x100000001b3ULL;
    }
    return lookup_hash(LookupSet::STR, h);
}

// lookup_find is the slot holding the key, or the empty slot where it
// would go.
inline size_t lookup_find(const LookupSet& set, uint8_t kind, uint64_t bits, std::string_view s, uint64_t h) {
    size_t i = static_cast<size_t>(h) & set.mask;
    while (true) {
        const LookupSet::Slot& slot = set.slots[i];
        if (slot.kind == LookupSet::EMPTY) return i;
        if (slot.kind == kind && (kind == LookupSet::STR ? slot.s == s : slot.bits == bits)) return i;
        i = (i + 1) & set.mask;
    }
}

inline void lookup_add(LookupSet& set, uint8_t kind, uint64_t bits, std::string_view s = {}) {
    const uint64_t h = kind == LookupSet::STR ? lookup_hash_str(s) : lookup_hash(kind, bits);
    const size_t i = lookup_find(set, kind, bits, s, h);
    if (set.slots[i].kind != LookupSet::EMPTY) return;
    set.slots[i].kind = kind;
    set.slots[i].bits = bits;
    set.slots[i].s = s;
}

inline bool lookup_has(const LookupSet& set, uint8_t kind, uint64_t bits, std::string_view s = {}) {
    if (set.slots.empty()) return false;
    const uint64_t h = kind == LookupSet::STR ? lookup_hash_str(s) : lookup_hash(kind, bits);
    return set.slots[lookup_find(set, kind, bits, s, h)].kind != LookupSet::EMPTY;
}

// lookup_float_key is a float's key: an exact int is an INT key (so -0.0 is
// 0), anything else its bits. NaN has no key.
inline bool lookup_float_key(double f, uint8_t* kind, uint64_t* bits) {
    if (f != f) return false;
    long long i = 0;
    if (q_float_as_int(f, &i)) {
        *kind = LookupSet::INT;
        *bits = static_cast<uint64_t>(i);
        return true;
    }
    *kind = LookupSet::FLT;
    std::memcpy(bits, &f, sizeof f);
    return true;
}

inline void lookup_reserve(LookupSet& set, size_t n) {
    size_t cap = 16;
    while (cap < n * 2) cap <<= 1;
    set.slots.assign(cap, LookupSet::Slot{});
    set.mask = cap - 1;
}

} // namespace tbl
} // namespace quark

// q_lookup_build makes the set of xs's elements: a list's int, float, bool,
// str and date elements (any other element is kept aside), or a vec's
// present entries.
inline quark::tbl::LookupSet q_lookup_build(QValue xs) {
    using quark::tbl::LookupSet;
    LookupSet set;
    set.src = xs;
    if (xs.type == QValue::VAL_LIST && xs.data.list_val) {
        const QList& l = *xs.data.list_val;
        quark::tbl::lookup_reserve(set, l.size());
        for (const QValue& e : l) {
            switch (e.type) {
                case QValue::VAL_INT: quark::tbl::lookup_add(set, LookupSet::INT, static_cast<uint64_t>(e.data.int_val)); break;
                case QValue::VAL_FLOAT: {
                    uint8_t kind = 0;
                    uint64_t bits = 0;
                    if (quark::tbl::lookup_float_key(e.data.float_val, &kind, &bits)) quark::tbl::lookup_add(set, kind, bits);
                    break;
                }
                case QValue::VAL_BOOL: quark::tbl::lookup_add(set, LookupSet::BOOL, e.data.bool_val ? 1 : 0); break;
                case QValue::VAL_DATE: quark::tbl::lookup_add(set, LookupSet::DATE, static_cast<uint64_t>(static_cast<uint32_t>(e.data.int_val))); break;
                case QValue::VAL_STRING:
                    if (e.data.string_val) quark::tbl::lookup_add(set, LookupSet::STR, 0, std::string_view(e.data.string_val));
                    else set.others.push_back(e);
                    break;
                default: set.others.push_back(e); break;
            }
        }
        return set;
    }
    if (xs.type == QValue::VAL_VECTOR && xs.data.vector_val) {
        const QVector& v = *xs.data.vector_val;
        quark::tbl::lookup_reserve(set, v.count);
        for (size_t i = 0; i < v.count; i++) {
            if (q_vec_is_null_at(v, i)) continue;
            switch (v.type) {
                case QVector::Type::I64: quark::tbl::lookup_add(set, LookupSet::INT, static_cast<uint64_t>(q_vec_i64_data(v)[i])); break;
                case QVector::Type::F64: {
                    uint8_t kind = 0;
                    uint64_t bits = 0;
                    if (quark::tbl::lookup_float_key(q_vec_f64_data(v)[i], &kind, &bits)) quark::tbl::lookup_add(set, kind, bits);
                    break;
                }
                case QVector::Type::BOOL: quark::tbl::lookup_add(set, LookupSet::BOOL, q_vec_bool_at(v, i) ? 1 : 0); break;
                case QVector::Type::DATE: quark::tbl::lookup_add(set, LookupSet::DATE, static_cast<uint64_t>(static_cast<uint32_t>(q_vec_date_data(v)[i]))); break;
                case QVector::Type::STR: quark::tbl::lookup_add(set, LookupSet::STR, 0, q_vec_str_at(v, i)); break;
            }
        }
        return set;
    }
    q_runtime_reportf("runtime error: contains() expects a list or vec, got %s\n", q_type_name(xs.type));
    q_exit(1);
}

inline bool q_lookup_has_int(const quark::tbl::LookupSet& set, long long x) {
    return quark::tbl::lookup_has(set, quark::tbl::LookupSet::INT, static_cast<uint64_t>(x));
}
inline bool q_lookup_has_float(const quark::tbl::LookupSet& set, double x) {
    uint8_t kind = 0;
    uint64_t bits = 0;
    return quark::tbl::lookup_float_key(x, &kind, &bits) && quark::tbl::lookup_has(set, kind, bits);
}
inline bool q_lookup_has_bool(const quark::tbl::LookupSet& set, bool x) {
    return quark::tbl::lookup_has(set, quark::tbl::LookupSet::BOOL, x ? 1 : 0);
}
inline bool q_lookup_has_date(const quark::tbl::LookupSet& set, int32_t x) {
    return quark::tbl::lookup_has(set, quark::tbl::LookupSet::DATE, static_cast<uint64_t>(static_cast<uint32_t>(x)));
}
inline bool q_lookup_has_str(const quark::tbl::LookupSet& set, std::string_view x) {
    return quark::tbl::lookup_has(set, quark::tbl::LookupSet::STR, 0, x);
}
inline bool q_lookup_has_val(const quark::tbl::LookupSet& set, QValue x) {
    switch (x.type) {
        case QValue::VAL_INT: return q_lookup_has_int(set, x.data.int_val);
        case QValue::VAL_FLOAT: return q_lookup_has_float(set, x.data.float_val);
        case QValue::VAL_BOOL: return q_lookup_has_bool(set, x.data.bool_val);
        case QValue::VAL_DATE: return q_lookup_has_date(set, static_cast<int32_t>(x.data.int_val));
        case QValue::VAL_STRING: return x.data.string_val && q_lookup_has_str(set, std::string_view(x.data.string_val));
        default:
            for (const QValue& e : set.others) {
                if (quark::detail::values_equal(e, x, 0)) return true;
            }
            return false;
    }
}

// ------------------------------------------------------------
// Table methods (D25): t.head(n), t.tail(n), t.describe(n). Builtin methods
// take QValues: the receiver is a table (dispatch chose this method by its
// type), the count is checked here, as it may come through `any`.

inline long long q_tbl_method_count(QValue n, const char* method) {
    if (n.type != QValue::VAL_INT) {
        q_runtime_reportf("runtime error: %s(n) expects an int, got %s\n", method, q_type_name(n.type));
        q_exit(1);
    }
    return n.data.int_val;
}

inline QValue q_tbl_head_method(QValue t, QValue n) {
    return qv_table(q_tbl_head(q_tbl_of(t), q_tbl_method_count(n, "head")));
}
inline QValue q_tbl_head_method(QValue t) { return q_tbl_head_method(t, qv_int(6)); }

inline QValue q_tbl_tail_method(QValue t, QValue n) {
    return qv_table(q_tbl_tail(q_tbl_of(t), q_tbl_method_count(n, "tail")));
}
inline QValue q_tbl_tail_method(QValue t) { return q_tbl_tail_method(t, qv_int(6)); }

// t.describe(n) prints one line per column: its name, its type, and its
// first n values as a vec prints them (R's str, dplyr's glimpse).
inline QValue q_tbl_describe(QValue tv, QValue nv) {
    QTable* t = q_tbl_of(tv);
    const long long n = q_tbl_method_count(nv, "describe");
    if (n < 0) {
        q_runtime_reportf("runtime error: describe(n) needs n >= 0, got %lld\n", n);
        q_exit(1);
    }
    std::string out = t->def->anonymous ? "tbl" : q_table_type_name(t);
    out += ": ";
    q_table_rows_text(out, t->nrows);
    out += ", " + std::to_string(t->ncols) + (t->ncols == 1 ? " column" : " columns");
    size_t name_w = 0;
    for (int c = 0; c < t->ncols; c++) name_w = std::max(name_w, q_text_width(t->def->field_names[c]));
    const int32_t shown = n < t->nrows ? static_cast<int32_t>(n) : t->nrows;
    for (int c = 0; c < t->ncols; c++) {
        const QVector& col = *quark::tbl::column(t, c);
        const std::string name = t->def->field_names[c];
        const char* type = q_dtype_word(static_cast<int>(col.type));
        out += "\n" + name + std::string(name_w - q_text_width(name) + 2, ' ') + type + std::string(7 - std::strlen(type), ' ');
        QReprCtx ctx;
        for (int32_t r = 0; r < shown; r++) {
            if (r) out += ", ";
            q_repr_into(out, q_vec_box_at(col, static_cast<size_t>(r)), true, ctx);
        }
        if (shown < t->nrows) out += shown ? ", ..." : "...";
    }
    out += "\n";
    std::fputs(out.c_str(), stdout);
    return qv_null();
}
inline QValue q_tbl_describe(QValue t) { return q_tbl_describe(t, qv_int(5)); }

// t.nrow(), t.ncol() and t.names() (tables v0.3 D49): the row count, the
// column count, and the column names in column order as a list[str].
inline QValue q_tbl_nrow(QValue t) { return qv_int(q_tbl_of(t)->nrows); }
inline QValue q_tbl_ncol(QValue t) { return qv_int(q_tbl_of(t)->ncols); }
inline QValue q_tbl_names(QValue tv) {
    QTable* t = q_tbl_of(tv);
    QValue out = qv_list(t->ncols, &q_elem_str);
    for (int c = 0; c < t->ncols; c++) out.data.list_val->push_back(qv_string(t->def->field_names[c]));
    return out;
}

#endif // QUARK_TABLE_QUERY_HPP
