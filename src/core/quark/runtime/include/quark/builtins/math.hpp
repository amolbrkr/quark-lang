// quark/builtins/math.hpp - Math operations
#ifndef QUARK_BUILTINS_MATH_HPP
#define QUARK_BUILTINS_MATH_HPP

#include "../core/value.hpp"
#include "../core/constructors.hpp"
#include <cmath>
#include <cstdlib>

// Helper functions to_double() and either_float() are defined in arithmetic.hpp
// (removed from here to avoid duplication in the concatenated runtime.hpp)

// Absolute value
inline QValue q_abs(QValue v) {
    // Type guard: only INT and FLOAT are valid
    if (v.type != QValue::VAL_INT && v.type != QValue::VAL_FLOAT) {
        return qv_null();
    }
    if (v.type == QValue::VAL_FLOAT) {
        return qv_float(fabs(v.data.float_val));
    }
    return qv_int(llabs(v.data.int_val));
}

// Minimum of two values
inline QValue q_min(QValue a, QValue b) {
    // Type guard: only INT and FLOAT are valid
    if ((a.type != QValue::VAL_INT && a.type != QValue::VAL_FLOAT) ||
        (b.type != QValue::VAL_INT && b.type != QValue::VAL_FLOAT)) {
        return qv_null();
    }
    if (quark::detail::either_float(a, b)) {
        double av = quark::detail::to_double(a);
        double bv = quark::detail::to_double(b);
        return qv_float(av < bv ? av : bv);
    }
    return qv_int(a.data.int_val < b.data.int_val ? a.data.int_val : b.data.int_val);
}

// Maximum of two values
inline QValue q_max(QValue a, QValue b) {
    // Type guard: only INT and FLOAT are valid
    if ((a.type != QValue::VAL_INT && a.type != QValue::VAL_FLOAT) ||
        (b.type != QValue::VAL_INT && b.type != QValue::VAL_FLOAT)) {
        return qv_null();
    }
    if (quark::detail::either_float(a, b)) {
        double av = quark::detail::to_double(a);
        double bv = quark::detail::to_double(b);
        return qv_float(av > bv ? av : bv);
    }
    return qv_int(a.data.int_val > b.data.int_val ? a.data.int_val : b.data.int_val);
}

// Square root (always returns float)
inline QValue q_sqrt(QValue v) {
    // Type guard: only INT and FLOAT are valid
    if (v.type != QValue::VAL_INT && v.type != QValue::VAL_FLOAT) {
        return qv_null();
    }
    double val = quark::detail::to_double(v);
    // Check for negative values
    if (val < 0.0) {
        return qv_null();
    }
    return qv_float(sqrt(val));
}

// Floor (returns int)
inline QValue q_floor(QValue v) {
    // Type guard: only INT and FLOAT are valid
    if (v.type != QValue::VAL_INT && v.type != QValue::VAL_FLOAT) {
        return qv_null();
    }
    if (v.type == QValue::VAL_INT) return v;
    return qv_int(static_cast<long long>(floor(v.data.float_val)));
}

// Ceiling (returns int)
inline QValue q_ceil(QValue v) {
    // Type guard: only INT and FLOAT are valid
    if (v.type != QValue::VAL_INT && v.type != QValue::VAL_FLOAT) {
        return qv_null();
    }
    if (v.type == QValue::VAL_INT) return v;
    return qv_int(static_cast<long long>(ceil(v.data.float_val)));
}

// Round to nearest integer (returns int)
inline QValue q_round(QValue v) {
    // Type guard: only INT and FLOAT are valid
    if (v.type != QValue::VAL_INT && v.type != QValue::VAL_FLOAT) {
        return qv_null();
    }
    if (v.type == QValue::VAL_INT) return v;
    return qv_int(static_cast<long long>(round(v.data.float_val)));
}

#endif // QUARK_BUILTINS_MATH_HPP
