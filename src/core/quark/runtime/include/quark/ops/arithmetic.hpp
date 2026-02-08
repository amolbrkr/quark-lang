// quark/ops/arithmetic.hpp - Arithmetic operations
#ifndef QUARK_OPS_ARITHMETIC_HPP
#define QUARK_OPS_ARITHMETIC_HPP

#include "../core/value.hpp"
#include "../core/constructors.hpp"
#include <cmath>

namespace quark {
namespace detail {

// Helper to extract numeric value as double
#ifndef QUARK_DETAIL_TO_DOUBLE_DEFINED
#define QUARK_DETAIL_TO_DOUBLE_DEFINED
inline double to_double(const QValue& v) {
    return v.type == QValue::VAL_FLOAT ? v.data.float_val
                                       : static_cast<double>(v.data.int_val);
}

inline bool either_float(const QValue& a, const QValue& b) {
    return a.type == QValue::VAL_FLOAT || b.type == QValue::VAL_FLOAT;
}
#endif

} // namespace detail
} // namespace quark

// Addition: int + int = int, otherwise float
inline QValue q_add(QValue a, QValue b) {
    if (quark::detail::either_float(a, b)) {
        return qv_float(quark::detail::to_double(a) + quark::detail::to_double(b));
    }
    return qv_int(a.data.int_val + b.data.int_val);
}

// Subtraction: int - int = int, otherwise float
inline QValue q_sub(QValue a, QValue b) {
    if (quark::detail::either_float(a, b)) {
        return qv_float(quark::detail::to_double(a) - quark::detail::to_double(b));
    }
    return qv_int(a.data.int_val - b.data.int_val);
}

// Multiplication: int * int = int, otherwise float
inline QValue q_mul(QValue a, QValue b) {
    if (quark::detail::either_float(a, b)) {
        return qv_float(quark::detail::to_double(a) * quark::detail::to_double(b));
    }
    return qv_int(a.data.int_val * b.data.int_val);
}

// Division: always returns float for precision
inline QValue q_div(QValue a, QValue b) {
    return qv_float(quark::detail::to_double(a) / quark::detail::to_double(b));
}

// Modulo: integer only
inline QValue q_mod(QValue a, QValue b) {
    return qv_int(a.data.int_val % b.data.int_val);
}

// Power: preserves int type when possible
inline QValue q_pow(QValue a, QValue b) {
    double av = quark::detail::to_double(a);
    double bv = quark::detail::to_double(b);
    double result = std::pow(av, bv);

    if (quark::detail::either_float(a, b)) {
        return qv_float(result);
    }
    return qv_int(static_cast<long long>(result));
}

// Unary negation
inline QValue q_neg(QValue a) {
    if (a.type == QValue::VAL_FLOAT) {
        return qv_float(-a.data.float_val);
    }
    return qv_int(-a.data.int_val);
}

#endif // QUARK_OPS_ARITHMETIC_HPP
