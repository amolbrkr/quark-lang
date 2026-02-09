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

// Addition: int + int = int, float promotion, string + string = concat
inline QValue q_add(QValue a, QValue b) {
    // String concatenation: string + string
    if (a.type == QValue::VAL_STRING && b.type == QValue::VAL_STRING) {
        size_t len = strlen(a.data.string_val) + strlen(b.data.string_val);
        char* result = static_cast<char*>(q_malloc_atomic(len + 1));
        strcpy(result, a.data.string_val);
        strcat(result, b.data.string_val);
        QValue q;
        q.type = QValue::VAL_STRING;
        q.data.string_val = result;
        return q;
    }
    // Type guard: only INT and FLOAT are valid for numeric addition
    if ((a.type != QValue::VAL_INT && a.type != QValue::VAL_FLOAT) ||
        (b.type != QValue::VAL_INT && b.type != QValue::VAL_FLOAT)) {
        return qv_null();
    }
    if (quark::detail::either_float(a, b)) {
        return qv_float(quark::detail::to_double(a) + quark::detail::to_double(b));
    }
    return qv_int(a.data.int_val + b.data.int_val);
}

// Subtraction: int - int = int, otherwise float
inline QValue q_sub(QValue a, QValue b) {
    // Type guard: only INT and FLOAT are valid
    if ((a.type != QValue::VAL_INT && a.type != QValue::VAL_FLOAT) ||
        (b.type != QValue::VAL_INT && b.type != QValue::VAL_FLOAT)) {
        return qv_null();
    }
    if (quark::detail::either_float(a, b)) {
        return qv_float(quark::detail::to_double(a) - quark::detail::to_double(b));
    }
    return qv_int(a.data.int_val - b.data.int_val);
}

// Multiplication: int * int = int, otherwise float
inline QValue q_mul(QValue a, QValue b) {
    // Type guard: only INT and FLOAT are valid
    if ((a.type != QValue::VAL_INT && a.type != QValue::VAL_FLOAT) ||
        (b.type != QValue::VAL_INT && b.type != QValue::VAL_FLOAT)) {
        return qv_null();
    }
    if (quark::detail::either_float(a, b)) {
        return qv_float(quark::detail::to_double(a) * quark::detail::to_double(b));
    }
    return qv_int(a.data.int_val * b.data.int_val);
}

// Division: always returns float for precision
inline QValue q_div(QValue a, QValue b) {
    // Type guard: only INT and FLOAT are valid
    if ((a.type != QValue::VAL_INT && a.type != QValue::VAL_FLOAT) ||
        (b.type != QValue::VAL_INT && b.type != QValue::VAL_FLOAT)) {
        return qv_null();
    }
    double bv = quark::detail::to_double(b);
    // Check for division by zero
    if (bv == 0.0) {
        return qv_null();
    }
    return qv_float(quark::detail::to_double(a) / bv);
}

// Modulo: integer only
inline QValue q_mod(QValue a, QValue b) {
    // Type guard: only INT is valid for modulo
    if (a.type != QValue::VAL_INT || b.type != QValue::VAL_INT) {
        return qv_null();
    }
    // Check for modulo by zero
    if (b.data.int_val == 0) {
        return qv_null();
    }
    return qv_int(a.data.int_val % b.data.int_val);
}

// Power: preserves int type when possible
inline QValue q_pow(QValue a, QValue b) {
    // Type guard: only INT and FLOAT are valid
    if ((a.type != QValue::VAL_INT && a.type != QValue::VAL_FLOAT) ||
        (b.type != QValue::VAL_INT && b.type != QValue::VAL_FLOAT)) {
        return qv_null();
    }
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
    // Type guard: only INT and FLOAT are valid
    if (a.type != QValue::VAL_INT && a.type != QValue::VAL_FLOAT) {
        return qv_null();
    }
    if (a.type == QValue::VAL_FLOAT) {
        return qv_float(-a.data.float_val);
    }
    return qv_int(-a.data.int_val);
}

#endif // QUARK_OPS_ARITHMETIC_HPP
