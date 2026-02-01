// quark/builtins/math.hpp - Math operations
#ifndef QUARK_BUILTINS_MATH_HPP
#define QUARK_BUILTINS_MATH_HPP

#include "../core/value.hpp"
#include "../core/constructors.hpp"
#include <cmath>
#include <cstdlib>

namespace quark {
namespace detail {

// Helper to extract numeric value as double
#ifndef QUARK_DETAIL_MATH_TO_DOUBLE_DEFINED
#define QUARK_DETAIL_MATH_TO_DOUBLE_DEFINED
inline double math_to_double(const QValue& v) {
    return v.type == QValue::VAL_FLOAT ? v.data.float_val
                                       : static_cast<double>(v.data.int_val);
}

inline bool math_either_float(const QValue& a, const QValue& b) {
    return a.type == QValue::VAL_FLOAT || b.type == QValue::VAL_FLOAT;
}
#endif

} // namespace detail
} // namespace quark

// Absolute value
inline QValue q_abs(QValue v) {
    if (v.type == QValue::VAL_FLOAT) {
        return qv_float(fabs(v.data.float_val));
    }
    return qv_int(llabs(v.data.int_val));
}

// Minimum of two values
inline QValue q_min(QValue a, QValue b) {
    if (quark::detail::math_either_float(a, b)) {
        double av = quark::detail::math_to_double(a);
        double bv = quark::detail::math_to_double(b);
        return qv_float(av < bv ? av : bv);
    }
    return qv_int(a.data.int_val < b.data.int_val ? a.data.int_val : b.data.int_val);
}

// Maximum of two values
inline QValue q_max(QValue a, QValue b) {
    if (quark::detail::math_either_float(a, b)) {
        double av = quark::detail::math_to_double(a);
        double bv = quark::detail::math_to_double(b);
        return qv_float(av > bv ? av : bv);
    }
    return qv_int(a.data.int_val > b.data.int_val ? a.data.int_val : b.data.int_val);
}

// Square root (always returns float)
inline QValue q_sqrt(QValue v) {
    double val = quark::detail::math_to_double(v);
    return qv_float(sqrt(val));
}

// Floor (returns int)
inline QValue q_floor(QValue v) {
    if (v.type == QValue::VAL_INT) return v;
    return qv_int(static_cast<long long>(floor(v.data.float_val)));
}

// Ceiling (returns int)
inline QValue q_ceil(QValue v) {
    if (v.type == QValue::VAL_INT) return v;
    return qv_int(static_cast<long long>(ceil(v.data.float_val)));
}

// Round to nearest integer (returns int)
inline QValue q_round(QValue v) {
    if (v.type == QValue::VAL_INT) return v;
    return qv_int(static_cast<long long>(round(v.data.float_val)));
}

#endif // QUARK_BUILTINS_MATH_HPP
