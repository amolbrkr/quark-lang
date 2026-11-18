// quark/core/parcheck.hpp - checked mode's second run of parallel work
// (specs/tables_v0_3_spec.md §8.7, D57).
//
// A program built with --checked computes each query step and each vec
// operation through q_par_check(what, f). f runs as usual; when it split a
// loop across threads (par::split), it runs again with every loop on this
// thread (par::serial), from the same random-draw state, and the two
// outcomes are compared: the same value (missing entries, order and float
// bits included), or the same error. A difference is INV-PAR-001: a bug in
// the engine, or native code whose `rowsafe` promise doesn't hold. Without
// --checked, generated code doesn't call q_par_check.
//
// Both runs record their errors (q_error_capture) instead of ending the
// program; after the comparison the error is reported as it would have
// been. What f runs must be safe to run twice: lowering wraps only steps and
// vec operations, whose arguments are computed before f.
#ifndef QUARK_CORE_PARCHECK_HPP
#define QUARK_CORE_PARCHECK_HPP

#include "diagnostics.hpp"
#include "par.hpp"
#include "rowrand.hpp"
#include "value.hpp"
#include "../types/table.hpp"
#include "../types/vector.hpp"

#include <cstdio>
#include <cstring>
#include <string_view>
#include <type_traits>

// q_par_checked is set at startup by the main of a --checked program.
inline bool q_par_checked = false;
inline void q_par_set_checked() { q_par_checked = true; }

namespace quark {
namespace par {

// Outcome is one run of a checked step: its value, or its error.
template <class R>
struct Outcome {
    R value{};
    bool failed = false;
    int code = 1;
    QErrorCapture err;
};

template <class R, class F>
inline void attempt(F& f, Outcome<R>& out) {
    out.err.text[0] = '\0';
    QErrorCapture* saved = q_error_capture;
    q_error_capture = &out.err;
    try {
        out.value = f();
    } catch (const QBlockExit& e) {
        out.failed = true;
        out.code = e.code;
    } catch (...) {
        q_error_capture = saved;
        throw;
    }
    q_error_capture = saved;
}

// Differences are described in why (at most n bytes).
inline bool same_bits(double a, double b) { return std::memcmp(&a, &b, sizeof a) == 0; }

inline bool same_column(const QVector& a, const QVector& b, const char* name, char* why, size_t n) {
    auto differ = [&](const char* what, size_t row) {
        if (name != nullptr) std::snprintf(why, n, "column '%s', %s at row %zu", name, what, row + 1);
        else std::snprintf(why, n, "%s at element %zu", what, row + 1);
        return false;
    };
    if (a.type != b.type) return differ("element type", 0);
    if (a.count != b.count) {
        if (name != nullptr) std::snprintf(why, n, "column '%s', %zu rows against %zu", name, a.count, b.count);
        else std::snprintf(why, n, "%zu elements against %zu", a.count, b.count);
        return false;
    }
    for (size_t i = 0; i < a.count; i++) {
        const bool an = q_vec_is_null_at(a, i), bn = q_vec_is_null_at(b, i);
        if (an != bn) return differ("a missing value", i);
        if (an) continue;
        bool same = true;
        switch (a.type) {
            case QVector::Type::F64: same = same_bits(q_vec_f64_data(a)[i], q_vec_f64_data(b)[i]); break;
            case QVector::Type::I64: same = q_vec_i64_data(a)[i] == q_vec_i64_data(b)[i]; break;
            case QVector::Type::DATE: same = q_vec_date_data(a)[i] == q_vec_date_data(b)[i]; break;
            case QVector::Type::BOOL: same = q_vec_bool_at(a, i) == q_vec_bool_at(b, i); break;
            case QVector::Type::STR: same = q_vec_str_at(a, i) == q_vec_str_at(b, i); break;
        }
        if (!same) return differ("a value", i);
    }
    return true;
}

inline bool same_table(const QTable* a, const QTable* b, char* why, size_t n) {
    if (a == b) return true;
    if (a == nullptr || b == nullptr || a->def != b->def || a->ncols != b->ncols) {
        std::snprintf(why, n, "a different table");
        return false;
    }
    if (a->nrows != b->nrows) {
        std::snprintf(why, n, "%d rows against %d", a->nrows, b->nrows);
        return false;
    }
    for (int c = 0; c < a->ncols; c++) {
        const QValue& x = a->cols[c];
        const QValue& y = b->cols[c];
        const char* name = a->def->field_names[c];
        if (x.type != QValue::VAL_VECTOR || y.type != QValue::VAL_VECTOR || x.data.vector_val == nullptr || y.data.vector_val == nullptr) {
            std::snprintf(why, n, "column '%s' is not a column", name);
            return false;
        }
        if (!same_column(*x.data.vector_val, *y.data.vector_val, name, why, n)) return false;
    }
    return true;
}

// same_value compares the values a vec operation can give: scalars, strings,
// vecs, tables and lists of them. Other kinds compare as the same object.
inline bool same_value(const QValue& a, const QValue& b, char* why, size_t n) {
    if (a.type != b.type) {
        std::snprintf(why, n, "values of different types");
        return false;
    }
    switch (a.type) {
        case QValue::VAL_INT:
        case QValue::VAL_DATE:
            if (a.data.int_val == b.data.int_val) return true;
            std::snprintf(why, n, "%lld against %lld", a.data.int_val, b.data.int_val);
            return false;
        case QValue::VAL_FLOAT:
            if (same_bits(a.data.float_val, b.data.float_val)) return true;
            std::snprintf(why, n, "%.17g against %.17g", a.data.float_val, b.data.float_val);
            return false;
        case QValue::VAL_BOOL:
            if (a.data.bool_val == b.data.bool_val) return true;
            std::snprintf(why, n, "%s against %s", a.data.bool_val ? "true" : "false", b.data.bool_val ? "true" : "false");
            return false;
        case QValue::VAL_NULL: return true;
        case QValue::VAL_STRING:
            if (std::strcmp(a.data.string_val, b.data.string_val) == 0) return true;
            std::snprintf(why, n, "different strings");
            return false;
        case QValue::VAL_VECTOR: return same_column(*a.data.vector_val, *b.data.vector_val, nullptr, why, n);
        case QValue::VAL_TABLE: return same_table(a.data.table_val, b.data.table_val, why, n);
        case QValue::VAL_LIST: {
            const QList& x = *a.data.list_val;
            const QList& y = *b.data.list_val;
            if (x.size() != y.size()) {
                std::snprintf(why, n, "%zu elements against %zu", x.size(), y.size());
                return false;
            }
            for (size_t i = 0; i < x.size(); i++) {
                if (!same_value(x[i], y[i], why, n)) return false;
            }
            return true;
        }
        default: return std::memcmp(&a.data, &b.data, sizeof a.data) == 0;
    }
}

inline bool same_result(const QValue& a, const QValue& b, char* why, size_t n) { return same_value(a, b, why, n); }
inline bool same_result(QTable* a, QTable* b, char* why, size_t n) { return same_table(a, b, why, n); }
inline bool same_result(double a, double b, char* why, size_t n) {
    if (same_bits(a, b)) return true;
    std::snprintf(why, n, "%.17g against %.17g", a, b);
    return false;
}
inline bool same_result(long long a, long long b, char* why, size_t n) {
    if (a == b) return true;
    std::snprintf(why, n, "%lld against %lld", a, b);
    return false;
}
inline bool same_result(bool a, bool b, char* why, size_t n) {
    if (a == b) return true;
    std::snprintf(why, n, "%s against %s", a ? "true" : "false", b ? "true" : "false");
    return false;
}

// mismatch ends the program with INV-PAR-001, whatever the step's error
// capture: it is not an error of the program.
[[noreturn]] inline void mismatch(const char* what, const char* why) {
    q_error_capture = nullptr;
    char msg[1024];
    std::snprintf(msg, sizeof msg,
                  "%s gave another result on %d threads than on one (%s): a bug in Quark, or native code that breaks its rowsafe promise",
                  what, q_par_threads(), why);
    std::fflush(stdout);
    q_report_message("INV-PAR-001", msg);
    q_exit(1);
}

} // namespace par
} // namespace quark

// q_par_check runs f(), a step or vec operation (what names it in the
// message), as described at the top of this file, and returns its value.
template <class F>
inline auto q_par_check(const char* what, F f) -> decltype(f()) {
    using R = decltype(f());
    namespace par = quark::par;
    if (!q_par_checked || par::depth > 0 || par::serial > 0) return f();
    const bool outer = par::split;
    par::split = false;
    const uint64_t loops = q_rand_state.loops;
    const QRowCtx ctx = q_row_ctx;
    par::Outcome<R> many;
    par::attempt(f, many);
    const bool used = par::split;
    par::split = outer || used;
    if (used) {
        const uint64_t loops_after = q_rand_state.loops;
        const QRowCtx ctx_after = q_row_ctx;
        q_rand_state.loops = loops;
        q_row_ctx = ctx;
        par::Outcome<R> one;
        par::serial++;
        try {
            par::attempt(f, one);
        } catch (...) {
            par::serial--;
            throw;
        }
        par::serial--;
        char why[512] = "";
        if (many.failed != one.failed) {
            std::snprintf(why, sizeof why, "%s", many.failed ? "an error where one thread gives none" : "no error where one thread gives one");
            par::mismatch(what, why);
        }
        if (many.failed && std::strcmp(many.err.text, one.err.text) != 0) {
            std::snprintf(why, sizeof why, "a different error");
            par::mismatch(what, why);
        }
        if (!many.failed && !par::same_result(many.value, one.value, why, sizeof why)) par::mismatch(what, why);
        if (q_rand_state.loops != loops_after) par::mismatch(what, "a different number of drawing loops");
        q_row_ctx = ctx_after;
    }
    if (many.failed) par::fail_as(many.err, many.code);
    return many.value;
}

#endif // QUARK_CORE_PARCHECK_HPP
