// quark/ops/comparison.hpp - Comparison operations
#ifndef QUARK_OPS_COMPARISON_HPP
#define QUARK_OPS_COMPARISON_HPP

#include "../core/value.hpp"
#include "../core/constructors.hpp"
#include <cstring>

// Helper functions to_double() and either_float() are defined in arithmetic.hpp
// (removed from here to avoid duplication in the concatenated runtime.hpp)

// Less than
inline QValue q_lt(QValue a, QValue b) {
    if (quark::detail::either_float(a, b)) {
        return qv_bool(quark::detail::to_double(a) < quark::detail::to_double(b));
    }
    return qv_bool(a.data.int_val < b.data.int_val);
}

// Less than or equal
inline QValue q_lte(QValue a, QValue b) {
    if (quark::detail::either_float(a, b)) {
        return qv_bool(quark::detail::to_double(a) <= quark::detail::to_double(b));
    }
    return qv_bool(a.data.int_val <= b.data.int_val);
}

// Greater than
inline QValue q_gt(QValue a, QValue b) {
    if (quark::detail::either_float(a, b)) {
        return qv_bool(quark::detail::to_double(a) > quark::detail::to_double(b));
    }
    return qv_bool(a.data.int_val > b.data.int_val);
}

// Greater than or equal
inline QValue q_gte(QValue a, QValue b) {
    if (quark::detail::either_float(a, b)) {
        return qv_bool(quark::detail::to_double(a) >= quark::detail::to_double(b));
    }
    return qv_bool(a.data.int_val >= b.data.int_val);
}

// Equality (type-sensitive)
inline QValue q_eq(QValue a, QValue b) {
    if (a.type != b.type) {
        // Allow int/float comparison
        if ((a.type == QValue::VAL_INT || a.type == QValue::VAL_FLOAT) &&
            (b.type == QValue::VAL_INT || b.type == QValue::VAL_FLOAT)) {
            return qv_bool(quark::detail::to_double(a) == quark::detail::to_double(b));
        }
        return qv_bool(false);
    }

    switch (a.type) {
        case QValue::VAL_INT:
            return qv_bool(a.data.int_val == b.data.int_val);
        case QValue::VAL_FLOAT:
            return qv_bool(a.data.float_val == b.data.float_val);
        case QValue::VAL_BOOL:
            return qv_bool(a.data.bool_val == b.data.bool_val);
        case QValue::VAL_STRING:
            return qv_bool(strcmp(a.data.string_val, b.data.string_val) == 0);
        case QValue::VAL_NULL:
            return qv_bool(true);
        default:
            return qv_bool(false);
    }
}

// Not equal
inline QValue q_neq(QValue a, QValue b) {
    return qv_bool(!q_eq(a, b).data.bool_val);
}

#endif // QUARK_OPS_COMPARISON_HPP
