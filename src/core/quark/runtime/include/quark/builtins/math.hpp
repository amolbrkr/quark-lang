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
inline const char* q_type_name_math(QValue::ValueType t) {
    static const char* names[] = {"int", "float", "str", "bool", "null", "list", "vec", "dict", "fn", "result"};
    return (t >= 0 && t <= 9) ? names[t] : "unknown";
}

// Helper to_double() and either_float() are defined in arithmetic.hpp

// Absolute value
inline QValue q_abs(QValue v) {
    if (v.type != QValue::VAL_INT && v.type != QValue::VAL_FLOAT) {
        q_runtime_reportf("runtime error: abs() expects numeric argument, got %s\n", q_type_name_math(v.type));
        std::exit(1);
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
inline QValue q_minmax(QValue a, QValue b, bool want_min) {
    const bool a_nan = a.type == QValue::VAL_FLOAT && a.data.float_val != a.data.float_val;
    const bool b_nan = b.type == QValue::VAL_FLOAT && b.data.float_val != b.data.float_val;
    if (a_nan) return a;
    if (b_nan) return b;
    if (a.type == QValue::VAL_INT && b.type == QValue::VAL_INT) {
        const long long x = a.data.int_val, y = b.data.int_val;
        const bool a_wins = want_min ? x < y : x > y;
        return qv_int(a_wins ? x : y);
    }
    if (a.type == QValue::VAL_FLOAT && b.type == QValue::VAL_FLOAT) {
        const double x = a.data.float_val, y = b.data.float_val;
        const bool a_wins = want_min ? x < y : x > y;
        return qv_float(a_wins ? x : y);
    }
    const bool a_int = a.type == QValue::VAL_INT;
    const long long i = a_int ? a.data.int_val : b.data.int_val;
    const double f = a_int ? b.data.float_val : a.data.float_val;
    const int c = q_cmp_int_float(i, f);
    const bool int_wins = want_min ? c < 0 : c > 0;
    if (!int_wins) return a_int ? b : a;
    if (q_int_fits_float(i)) return qv_float(static_cast<double>(i));
    return a_int ? a : b;
}

// Minimum of two values
inline QValue q_min(QValue a, QValue b) {
    if ((a.type != QValue::VAL_INT && a.type != QValue::VAL_FLOAT) ||
        (b.type != QValue::VAL_INT && b.type != QValue::VAL_FLOAT)) {
        q_runtime_reportf("runtime error: min() expects numeric arguments, got %s and %s\n", q_type_name_math(a.type), q_type_name_math(b.type));
        std::exit(1);
    }
    return q_minmax(a, b, true);
}

// Minimum of vector values
inline QValue q_min(QValue v) {
    if (v.type == QValue::VAL_VECTOR) {
        return q_vec_min(v);
    }
    q_runtime_reportf("runtime error: single-argument min() expects numeric vec, got %s\n", q_type_name_math(v.type));
    std::exit(1);
}

// Maximum of two values
inline QValue q_max(QValue a, QValue b) {
    if ((a.type != QValue::VAL_INT && a.type != QValue::VAL_FLOAT) ||
        (b.type != QValue::VAL_INT && b.type != QValue::VAL_FLOAT)) {
        q_runtime_reportf("runtime error: max() expects numeric arguments, got %s and %s\n", q_type_name_math(a.type), q_type_name_math(b.type));
        std::exit(1);
    }
    return q_minmax(a, b, false);
}

// Maximum of vector values
inline QValue q_max(QValue v) {
    if (v.type == QValue::VAL_VECTOR) {
        return q_vec_max(v);
    }
    q_runtime_reportf("runtime error: single-argument max() expects numeric vec, got %s\n", q_type_name_math(v.type));
    std::exit(1);
}

// Sum of vector values
inline QValue q_sum(QValue v) {
    if (v.type == QValue::VAL_VECTOR) {
        return q_vec_sum(v);
    }
    q_runtime_reportf("runtime error: sum() expects numeric or bool vec, got %s\n", q_type_name_math(v.type));
    std::exit(1);
}

// all(v) / any(v) for vec[bool]. Null entries are skipped, matching the
// other vector reductions. all() of an empty vector is true; any() is false.
inline QValue q_vec_bool_reduce(QValue v, bool want_all, const char* name) {
    if (v.type != QValue::VAL_VECTOR || !v.data.vector_val ||
        v.data.vector_val->type != QVector::Type::BOOL) {
        q_runtime_reportf("runtime error: %s() expects vec[bool], got %s\n",
                          name, q_type_name_math(v.type));
        std::exit(1);
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
        std::exit(1);
    }
    double val = quark::detail::to_double(v);
    if (val < 0.0) {
        char fbuf[32];
        q_runtime_reportf("runtime error: sqrt() domain error: argument is negative (%s)\n", q_format_float(val, fbuf, sizeof(fbuf)));
        std::exit(1);
    }
    return qv_float(sqrt(val));
}

// Floor (returns int)
inline QValue q_floor(QValue v) {
    if (v.type != QValue::VAL_INT && v.type != QValue::VAL_FLOAT) {
        q_runtime_reportf("runtime error: floor() expects numeric argument, got %s\n", q_type_name_math(v.type));
        std::exit(1);
    }
    if (v.type == QValue::VAL_INT) return v;
    return qv_int(q_checked_f2i(floor(v.data.float_val), "floor()"));
}

// Ceiling (returns int)
inline QValue q_ceil(QValue v) {
    if (v.type != QValue::VAL_INT && v.type != QValue::VAL_FLOAT) {
        q_runtime_reportf("runtime error: ceil() expects numeric argument, got %s\n", q_type_name_math(v.type));
        std::exit(1);
    }
    if (v.type == QValue::VAL_INT) return v;
    return qv_int(q_checked_f2i(ceil(v.data.float_val), "ceil()"));
}

// Round to nearest integer (returns int)
inline QValue q_round(QValue v) {
    if (v.type != QValue::VAL_INT && v.type != QValue::VAL_FLOAT) {
        q_runtime_reportf("runtime error: round() expects numeric argument, got %s\n", q_type_name_math(v.type));
        std::exit(1);
    }
    if (v.type == QValue::VAL_INT) return v;
    return qv_int(q_checked_f2i(round(v.data.float_val), "round()"));
}

#endif // QUARK_BUILTINS_MATH_HPP
