// quark/ops/logical.hpp - Logical operations
#ifndef QUARK_OPS_LOGICAL_HPP
#define QUARK_OPS_LOGICAL_HPP

#include "../core/value.hpp"
#include "../core/constructors.hpp"
#include "../core/truthy.hpp"

// Logical AND
inline QValue q_and(QValue a, QValue b) {
    return qv_bool(q_truthy(a) && q_truthy(b));
}

// Logical OR
inline QValue q_or(QValue a, QValue b) {
    return qv_bool(q_truthy(a) || q_truthy(b));
}

// Logical NOT
inline QValue q_not(QValue a) {
    return qv_bool(!q_truthy(a));
}

#endif // QUARK_OPS_LOGICAL_HPP
