// quark/ops/logical.hpp - Logical operations
#ifndef QUARK_OPS_LOGICAL_HPP
#define QUARK_OPS_LOGICAL_HPP

#include "../core/value.hpp"
#include "../core/constructors.hpp"
#include <cstdio>
#include <cstdlib>

// Runtime bool enforcement helper (policy §4.3 — EXTRA-4)
inline void q_require_bool(QValue v, const char* op) {
    if (v.type != QValue::VAL_BOOL) {
        static const char* names[] = {"int", "float", "str", "bool", "null", "list", "vector", "dict", "fn", "result"};
        const char* tname = (v.type >= 0 && v.type <= 9) ? names[v.type] : "unknown";
        std::fprintf(stderr, "runtime error: '%s' expects bool operand, got %s\n", op, tname);
        std::exit(1);
    }
}

inline bool q_condition_bool(QValue v, const char* context) {
    if (v.type != QValue::VAL_BOOL) {
        static const char* names[] = {"int", "float", "str", "bool", "null", "list", "vector", "dict", "fn", "result"};
        const char* tname = (v.type >= 0 && v.type <= 9) ? names[v.type] : "unknown";
        std::fprintf(stderr, "runtime error: %s condition must be bool, got %s\n", context, tname);
        std::exit(1);
    }
    return v.data.bool_val;
}

// Logical AND
inline QValue q_and(QValue a, QValue b) {
    q_require_bool(a, "and");
    q_require_bool(b, "and");
    return qv_bool(a.data.bool_val && b.data.bool_val);
}

// Logical OR
inline QValue q_or(QValue a, QValue b) {
    q_require_bool(a, "or");
    q_require_bool(b, "or");
    return qv_bool(a.data.bool_val || b.data.bool_val);
}

// Logical NOT
inline QValue q_not(QValue a) {
    q_require_bool(a, "!");
    return qv_bool(!a.data.bool_val);
}

#endif // QUARK_OPS_LOGICAL_HPP
