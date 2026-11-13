// Shared helpers for the table engine tests (integration_table_test.go
// compiles each *_test.cpp in this directory against the runtime headers).
#ifndef QUARK_TABLE_TEST_CHECK_HPP
#define QUARK_TABLE_TEST_CHECK_HPP

#include "quark/quark.hpp"
#include "quark/table/csv.hpp"
#include "quark/table/engine.hpp"

#include <chrono>
#include <cstdio>
#include <optional>
#include <string>
#include <vector>

static int pass_count = 0, fail_count = 0;

// The condition is variadic so it may contain braced lists with commas.
#define EXPECT(label, ...)                                                   \
    do {                                                                     \
        if (__VA_ARGS__) {                                                   \
            pass_count++;                                                    \
        } else {                                                             \
            fail_count++;                                                    \
            std::fprintf(stderr, "FAIL %s:%d: %s\n", __FILE__, __LINE__, label); \
        }                                                                    \
    } while (0)

inline int finish(const char* name) {
    std::printf("%s: %d passed, %d failed\n", name, pass_count, fail_count);
    return fail_count > 0 ? 1 : 0;
}

using Opt = std::optional<long long>;
using OptF = std::optional<double>;
using OptS = std::optional<std::string>;
using OptB = std::optional<bool>;

inline QVector* i64_col(std::vector<Opt> xs) {
    QVector* v = q_vec_alloc(QVector::Type::I64, xs.size());
    for (size_t i = 0; i < xs.size(); i++) {
        if (xs[i]) q_vec_i64_data_mut(*v)[i] = *xs[i];
        else q_vec_mark_null(*v, i);
    }
    return v;
}

inline QVector* f64_col(std::vector<OptF> xs) {
    QVector* v = q_vec_alloc(QVector::Type::F64, xs.size());
    for (size_t i = 0; i < xs.size(); i++) {
        if (xs[i]) q_vec_f64_data_mut(*v)[i] = *xs[i];
        else q_vec_mark_null(*v, i);
    }
    return v;
}

inline QVector* bool_col(std::vector<OptB> xs) {
    QVector* v = q_vec_alloc(QVector::Type::BOOL, xs.size());
    for (size_t i = 0; i < xs.size(); i++) {
        if (xs[i]) q_vec_bool_put(*v, i, *xs[i]);
        else q_vec_mark_null(*v, i);
    }
    return v;
}

inline QVector* str_col(std::vector<OptS> xs) {
    QValue v = qv_vector_str();
    for (const OptS& x : xs) {
        if (x) {
            q_vec_str_append(*v.data.vector_val, x->data(), x->size());
        } else {
            q_vec_str_append(*v.data.vector_val, "", 0);
            q_vec_mark_null(*v.data.vector_val, v.data.vector_val->count - 1);
        }
    }
    return v.data.vector_val;
}

inline const QStructDef* schema(std::vector<const char*> names) {
    const char** fields = static_cast<const char**>(q_malloc(sizeof(char*) * names.size()));
    for (size_t i = 0; i < names.size(); i++) fields[i] = names[i];
    QStructDef* d = static_cast<QStructDef*>(q_malloc(sizeof(QStructDef)));
    d->name = "T";
    d->field_names = fields;
    d->field_count = static_cast<int>(names.size());
    return d;
}

inline QTable* table(std::vector<const char*> names, std::vector<QVector*> cols) {
    return quark::tbl::make_table(schema(names), cols.data());
}

// Readers that return nullopt for null, so expectations can compare whole columns.
inline std::vector<Opt> ints(const QVector* v) {
    std::vector<Opt> out;
    for (size_t i = 0; i < v->count; i++) {
        if (q_vec_is_null_at(*v, i)) out.push_back(std::nullopt);
        else out.push_back(q_vec_i64_data(*v)[i]);
    }
    return out;
}

inline std::vector<OptF> floats(const QVector* v) {
    std::vector<OptF> out;
    for (size_t i = 0; i < v->count; i++) {
        if (q_vec_is_null_at(*v, i)) out.push_back(std::nullopt);
        else out.push_back(q_vec_f64_data(*v)[i]);
    }
    return out;
}

inline std::vector<OptS> strs(const QVector* v) {
    std::vector<OptS> out;
    for (size_t i = 0; i < v->count; i++) {
        if (q_vec_is_null_at(*v, i)) out.push_back(std::nullopt);
        else out.push_back(std::string(q_vec_str_at(*v, i)));
    }
    return out;
}

inline std::vector<OptB> bools(const QVector* v) {
    std::vector<OptB> out;
    for (size_t i = 0; i < v->count; i++) {
        if (q_vec_is_null_at(*v, i)) out.push_back(std::nullopt);
        else out.push_back(q_vec_bool_at(*v, i));
    }
    return out;
}

inline QVector* col(QTable* t, int c) { return quark::tbl::column(t, c); }

// Every column of t passes the vector layout invariants and has t->nrows rows.
inline bool well_formed(QTable* t) {
    for (int c = 0; c < t->ncols; c++) {
        const QVector* v = col(t, c);
        if (!q_vec_validate(*v) || v->count != static_cast<size_t>(t->nrows)) return false;
        size_t nulls = 0;
        for (size_t i = 0; i < v->count; i++) nulls += q_vec_is_null_at(*v, i) ? 1 : 0;
        if (nulls != v->null_count) return false;
    }
    return true;
}

struct Timer {
    std::chrono::steady_clock::time_point start = std::chrono::steady_clock::now();
    double ms() const {
        return std::chrono::duration<double, std::milli>(std::chrono::steady_clock::now() - start).count();
    }
};

#endif
