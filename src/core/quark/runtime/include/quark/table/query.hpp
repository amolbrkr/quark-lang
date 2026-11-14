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

#include <initializer_list>
#include <string>
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
        std::exit(1);
    }
    return v.data.table_val;
}

inline long long q_tbl_rows(const QTable* t) { return t->nrows; }

inline const QVector& q_tbl_col(const QTable* t, int c) { return *quark::tbl::column(t, c); }

// Whether row i of a column holds a value (validity only: NaN is a value
// here, as in arithmetic; aggregates skip it).
inline bool q_tbl_present(const QVector& v, long long i) { return !quark::tbl::is_null(v, static_cast<int32_t>(i)); }

// A str cell as a Quark string (one copy; str cells hold no NUL byte, which
// the reader rejects).
inline QValue q_tbl_str(const QVector& v, long long i) {
    const std::string_view s = q_vec_str_at(v, static_cast<size_t>(i));
    char* out = static_cast<char*>(q_malloc_atomic(s.size() + 1));
    if (!s.empty()) std::memcpy(out, s.data(), s.size());
    out[s.size()] = '\0';
    return qv_string_own(out);
}

// ------------------------------------------------------------
// filter

inline quark::tbl::RowSet q_rows_new(long long n) {
    quark::tbl::RowSet s;
    s.idx = quark::tbl::alloc_i32(static_cast<size_t>(n));
    return s;
}

inline void q_rows_push(quark::tbl::RowSet& s, long long i) { s.idx[s.n++] = static_cast<int32_t>(i); }

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
inline void q_col_push_bool(quark::tbl::ColOut& b, bool x) {
    if (x) quark::vec::put_bit(b.v->values, static_cast<size_t>(b.n), true);
    b.n++;
}
inline void q_col_push_str(quark::tbl::ColOut& b, QValue s) {
    if (s.type != QValue::VAL_STRING || !s.data.string_val) {
        q_runtime_reportf("runtime error: internal: a str column got a %s\n", q_type_name(s.type));
        std::exit(1);
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
        std::exit(1);
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

inline quark::tbl::Grouping q_tbl_whole(QTable* t) { return quark::tbl::whole_table(t->nrows); }

inline QVector* q_tbl_group_key(QTable* t, const quark::tbl::Grouping& g, int c) { return quark::tbl::group_keys(t, g, c); }
inline QVector* q_tbl_agg_count(const quark::tbl::Grouping& g) { return quark::tbl::agg_count(g); }
inline QVector* q_tbl_agg_count_values(QTable* t, int c, const quark::tbl::Grouping& g) {
    return quark::tbl::agg_count_values(*quark::tbl::column(t, c), g);
}
inline QVector* q_tbl_agg_sum(QTable* t, int c, const quark::tbl::Grouping& g) { return quark::tbl::agg_sum(*quark::tbl::column(t, c), g); }
inline QVector* q_tbl_agg_mean(QTable* t, int c, const quark::tbl::Grouping& g) { return quark::tbl::agg_mean(*quark::tbl::column(t, c), g); }
inline QVector* q_tbl_agg_min(QTable* t, int c, const quark::tbl::Grouping& g) { return quark::tbl::agg_min(*quark::tbl::column(t, c), g); }
inline QVector* q_tbl_agg_max(QTable* t, int c, const quark::tbl::Grouping& g) { return quark::tbl::agg_max(*quark::tbl::column(t, c), g); }

inline QTable* q_tbl_make(const QStructDef* def, std::initializer_list<QVector*> cols) {
    std::vector<QVector*> cs(cols);
    return quark::tbl::make_table(def, cs.data());
}

// ------------------------------------------------------------
// Table methods (D25): t.head(n), t.tail(n), t.describe(n). Builtin methods
// take QValues: the receiver is a table (dispatch chose this method by its
// type), the count is checked here, as it may come through `any`.

inline long long q_tbl_method_count(QValue n, const char* method) {
    if (n.type != QValue::VAL_INT) {
        q_runtime_reportf("runtime error: %s(n) expects an int, got %s\n", method, q_type_name(n.type));
        std::exit(1);
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
        std::exit(1);
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

#endif // QUARK_TABLE_QUERY_HPP
