// quark/builtins/math.hpp - Math operations
#ifndef QUARK_BUILTINS_MATH_HPP
#define QUARK_BUILTINS_MATH_HPP

#include "../core/value.hpp"
#include "../core/constructors.hpp"
#include <cstdlib>
#include <cstdio>
#include <cmath>

// Helper: type name lookup for error messages
inline const char* q_type_name_math(QValue::ValueType t) {
    static const char* names[] = {"int", "float", "str", "bool", "null", "list", "vector", "dict", "fn", "result"};
    return (t >= 0 && t <= 9) ? names[t] : "unknown";
}

// Helper to_double() and either_float() are defined in arithmetic.hpp

// Absolute value
inline QValue q_abs(QValue v) {
    if (v.type != QValue::VAL_INT && v.type != QValue::VAL_FLOAT) {
        std::fprintf(stderr, "runtime error: abs() expects numeric argument, got %s\n", q_type_name_math(v.type));
        std::exit(1);
    }
    if (v.type == QValue::VAL_FLOAT) {
        return qv_float(fabs(v.data.float_val));
    }
    return qv_int(llabs(v.data.int_val));
}

// Minimum of two values
inline QValue q_min(QValue a, QValue b) {
    if ((a.type != QValue::VAL_INT && a.type != QValue::VAL_FLOAT) ||
        (b.type != QValue::VAL_INT && b.type != QValue::VAL_FLOAT)) {
        std::fprintf(stderr, "runtime error: min() expects numeric arguments, got %s and %s\n", q_type_name_math(a.type), q_type_name_math(b.type));
        std::exit(1);
    }
    if (quark::detail::either_float(a, b)) {
        double av = quark::detail::to_double(a);
        double bv = quark::detail::to_double(b);
        return qv_float(av < bv ? av : bv);
    }
    return qv_int(a.data.int_val < b.data.int_val ? a.data.int_val : b.data.int_val);
}

// Minimum of vector values
inline QValue q_min(QValue v) {
    if (v.type == QValue::VAL_VECTOR) {
        return q_vec_min(v);
    }
    std::fprintf(stderr, "runtime error: single-argument min() expects numeric vector, got %s\n", q_type_name_math(v.type));
    std::exit(1);
    return qv_null();
}

// Maximum of two values
inline QValue q_max(QValue a, QValue b) {
    if ((a.type != QValue::VAL_INT && a.type != QValue::VAL_FLOAT) ||
        (b.type != QValue::VAL_INT && b.type != QValue::VAL_FLOAT)) {
        std::fprintf(stderr, "runtime error: max() expects numeric arguments, got %s and %s\n", q_type_name_math(a.type), q_type_name_math(b.type));
        std::exit(1);
    }
    if (quark::detail::either_float(a, b)) {
        double av = quark::detail::to_double(a);
        double bv = quark::detail::to_double(b);
        return qv_float(av > bv ? av : bv);
    }
    return qv_int(a.data.int_val > b.data.int_val ? a.data.int_val : b.data.int_val);
}

// Maximum of vector values
inline QValue q_max(QValue v) {
    if (v.type == QValue::VAL_VECTOR) {
        return q_vec_max(v);
    }
    std::fprintf(stderr, "runtime error: single-argument max() expects numeric vector, got %s\n", q_type_name_math(v.type));
    std::exit(1);
    return qv_null();
}

// Sum of vector values
inline QValue q_sum(QValue v) {
    if (v.type == QValue::VAL_VECTOR) {
        return q_vec_sum(v);
    }
    std::fprintf(stderr, "runtime error: sum() expects numeric vector or list, got %s\n", q_type_name_math(v.type));
    std::exit(1);
    return qv_null();
}

// Square root (always returns float)
inline QValue q_sqrt(QValue v) {
    if (v.type != QValue::VAL_INT && v.type != QValue::VAL_FLOAT) {
        std::fprintf(stderr, "runtime error: sqrt() expects numeric argument, got %s\n", q_type_name_math(v.type));
        std::exit(1);
    }
    double val = quark::detail::to_double(v);
    if (val < 0.0) {
        std::fprintf(stderr, "runtime error: sqrt() domain error: argument is negative (%g)\n", val);
        std::exit(1);
    }
    return qv_float(sqrt(val));
}

// Floor (returns int)
inline QValue q_floor(QValue v) {
    if (v.type != QValue::VAL_INT && v.type != QValue::VAL_FLOAT) {
        std::fprintf(stderr, "runtime error: floor() expects numeric argument, got %s\n", q_type_name_math(v.type));
        std::exit(1);
    }
    if (v.type == QValue::VAL_INT) return v;
    return qv_int(static_cast<long long>(floor(v.data.float_val)));
}

// Ceiling (returns int)
inline QValue q_ceil(QValue v) {
    if (v.type != QValue::VAL_INT && v.type != QValue::VAL_FLOAT) {
        std::fprintf(stderr, "runtime error: ceil() expects numeric argument, got %s\n", q_type_name_math(v.type));
        std::exit(1);
    }
    if (v.type == QValue::VAL_INT) return v;
    return qv_int(static_cast<long long>(ceil(v.data.float_val)));
}

// Round to nearest integer (returns int)
inline QValue q_round(QValue v) {
    if (v.type != QValue::VAL_INT && v.type != QValue::VAL_FLOAT) {
        std::fprintf(stderr, "runtime error: round() expects numeric argument, got %s\n", q_type_name_math(v.type));
        std::exit(1);
    }
    if (v.type == QValue::VAL_INT) return v;
    return qv_int(static_cast<long long>(round(v.data.float_val)));
}

#endif // QUARK_BUILTINS_MATH_HPP
