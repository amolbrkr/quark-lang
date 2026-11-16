// quark/builtins/math.hpp - Math operations
#ifndef QUARK_BUILTINS_MATH_HPP
#define QUARK_BUILTINS_MATH_HPP

#include "../core/value.hpp"
#include "../core/constructors.hpp"
#include "../core/format.hpp"
#include "../core/checked.hpp"
#include "../core/numcmp.hpp"
#include <cstdlib>
#include <cstdio>
#include <cmath>

// Helper: type name lookup for error messages
inline const char* q_type_name_math(QValue::ValueType t) { return q_type_name(t); }

// Helper to_double() and either_float() are defined in arithmetic.hpp

// Absolute value
inline QValue q_abs(QValue v) {
    if (v.type != QValue::VAL_INT && v.type != QValue::VAL_FLOAT) {
        q_runtime_reportf("runtime error: abs() expects numeric argument, got %s\n", q_type_name_math(v.type));
        q_exit(1);
    }
    if (v.type == QValue::VAL_FLOAT) {
        return qv_float(fabs(v.data.float_val));
    }
    return qv_int(q_checked_abs(v.data.int_val));
}

// q_minmax returns the smaller (want_min) or larger of two numbers. An int
// and a float are ordered exactly. The result is a float when either
// argument is a float and the winning int fits in a float (min(1, 2.5) is
// 1.0); an int that wins and does not fit is returned as that int, so the
// result is not a nearby float. NaN wins, in either order.
inline QValue q_minmax_wide(long long i, double f, bool want_min);

inline QValue q_minmax(QValue a, QValue b, bool want_min) {
    if (a.type == QValue::VAL_INT && b.type == QValue::VAL_INT) {
        const long long x = a.data.int_val, y = b.data.int_val;
        return qv_int((want_min ? x < y : x > y) ? x : y);
    }
    const bool a_int = a.type == QValue::VAL_INT;
    const bool b_int = b.type == QValue::VAL_INT;
    // An int past 2^53 isn't exactly a float; the rest compare as floats.
    if ((a_int && !q_int_exact_in_float(a.data.int_val)) || (b_int && !q_int_exact_in_float(b.data.int_val))) {
        return a_int ? q_minmax_wide(a.data.int_val, b.data.float_val, want_min)
                     : q_minmax_wide(b.data.int_val, a.data.float_val, want_min);
    }
    const double x = a_int ? static_cast<double>(a.data.int_val) : a.data.float_val;
    const double y = b_int ? static_cast<double>(b.data.int_val) : b.data.float_val;
    if (x != x) return qv_float(x);
    if (y != y) return qv_float(y);
    // A tie keeps the float operand (min(0, -0.0) is -0.0); two floats keep y.
    const bool a_wins = want_min ? x < y : x > y;
    return qv_float((a_wins || (b_int && x == y)) ? x : y);
}

// q_minmax_wide is q_minmax of an int past 2^53 and a float.
inline QValue q_minmax_wide(long long i, double f, bool want_min) {
    if (f != f) return qv_float(f);
    const int c = q_cmp_int_float(i, f);
    const bool int_wins = want_min ? c < 0 : c > 0;
    if (!int_wins) return qv_float(f);
    if (q_int_fits_float(i)) return qv_float(static_cast<double>(i));
    return qv_int(i);
}

// Minimum of two values
inline QValue q_min(QValue a, QValue b) {
    if ((a.type != QValue::VAL_INT && a.type != QValue::VAL_FLOAT) ||
        (b.type != QValue::VAL_INT && b.type != QValue::VAL_FLOAT)) {
        q_runtime_reportf("runtime error: min() expects numeric arguments, got %s and %s\n", q_type_name_math(a.type), q_type_name_math(b.type));
        q_exit(1);
    }
    return q_minmax(a, b, true);
}

// Minimum of vector values
inline QValue q_min(QValue v) {
    if (v.type == QValue::VAL_VECTOR) {
        return q_vec_min(v);
    }
    q_runtime_reportf("runtime error: single-argument min() expects numeric vec, got %s\n", q_type_name_math(v.type));
    q_exit(1);
}

// Maximum of two values
inline QValue q_max(QValue a, QValue b) {
    if ((a.type != QValue::VAL_INT && a.type != QValue::VAL_FLOAT) ||
        (b.type != QValue::VAL_INT && b.type != QValue::VAL_FLOAT)) {
        q_runtime_reportf("runtime error: max() expects numeric arguments, got %s and %s\n", q_type_name_math(a.type), q_type_name_math(b.type));
        q_exit(1);
    }
    return q_minmax(a, b, false);
}

// Maximum of vector values
inline QValue q_max(QValue v) {
    if (v.type == QValue::VAL_VECTOR) {
        return q_vec_max(v);
    }
    q_runtime_reportf("runtime error: single-argument max() expects numeric vec, got %s\n", q_type_name_math(v.type));
    q_exit(1);
}

// Sum of vector values
inline QValue q_sum(QValue v) {
    if (v.type == QValue::VAL_VECTOR) {
        return q_vec_sum(v);
    }
    q_runtime_reportf("runtime error: sum() expects numeric or bool vec, got %s\n", q_type_name_math(v.type));
    q_exit(1);
}

// all(v) / any(v) for vec[bool]. Null entries are skipped, matching the
// other vector reductions. all() of an empty vector is true; any() is false.
inline QValue q_vec_bool_reduce(QValue v, bool want_all, const char* name) {
    if (v.type != QValue::VAL_VECTOR || !v.data.vector_val ||
        v.data.vector_val->type != QVector::Type::BOOL) {
        q_runtime_reportf("runtime error: %s() expects vec[bool], got %s\n",
                          name, q_type_name_math(v.type));
        q_exit(1);
    }
    const QVector& vec = *v.data.vector_val;
    for (size_t i = 0; i < vec.count; i++) {
        if (q_vec_is_null_at(vec, i)) continue;
        const bool bit = q_vec_bool_at(vec, i);
        if (want_all && !bit) return qv_bool(false);
        if (!want_all && bit) return qv_bool(true);
    }
    return qv_bool(want_all);
}

inline QValue q_all(QValue v) { return q_vec_bool_reduce(v, true, "all"); }
inline QValue q_any(QValue v) { return q_vec_bool_reduce(v, false, "any"); }

// Square root (always returns float)
inline QValue q_sqrt(QValue v) {
    if (v.type != QValue::VAL_INT && v.type != QValue::VAL_FLOAT) {
        q_runtime_reportf("runtime error: sqrt() expects numeric argument, got %s\n", q_type_name_math(v.type));
        q_exit(1);
    }
    double val = quark::detail::to_double(v);
    if (val < 0.0) {
        char fbuf[32];
        q_runtime_reportf("runtime error: sqrt() domain error: argument is negative (%s)\n", q_format_float(val, fbuf, sizeof(fbuf)));
        q_exit(1);
    }
    return qv_float(sqrt(val));
}

// Floor (returns int)
inline QValue q_floor(QValue v) {
    if (v.type != QValue::VAL_INT && v.type != QValue::VAL_FLOAT) {
        q_runtime_reportf("runtime error: floor() expects numeric argument, got %s\n", q_type_name_math(v.type));
        q_exit(1);
    }
    if (v.type == QValue::VAL_INT) return v;
    return qv_int(q_checked_f2i(floor(v.data.float_val), "floor()"));
}

// Ceiling (returns int)
inline QValue q_ceil(QValue v) {
    if (v.type != QValue::VAL_INT && v.type != QValue::VAL_FLOAT) {
        q_runtime_reportf("runtime error: ceil() expects numeric argument, got %s\n", q_type_name_math(v.type));
        q_exit(1);
    }
    if (v.type == QValue::VAL_INT) return v;
    return qv_int(q_checked_f2i(ceil(v.data.float_val), "ceil()"));
}

// Round to nearest integer (returns int)
inline QValue q_round(QValue v) {
    if (v.type != QValue::VAL_INT && v.type != QValue::VAL_FLOAT) {
        q_runtime_reportf("runtime error: round() expects numeric argument, got %s\n", q_type_name_math(v.type));
        q_exit(1);
    }
    if (v.type == QValue::VAL_INT) return v;
    return qv_int(q_checked_f2i(round(v.data.float_val), "round()"));
}

#endif // QUARK_BUILTINS_MATH_HPP
