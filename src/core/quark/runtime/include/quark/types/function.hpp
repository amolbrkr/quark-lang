// quark/types/function.hpp - Function value operations
#ifndef QUARK_TYPES_FUNCTION_HPP
#define QUARK_TYPES_FUNCTION_HPP

#include "../core/value.hpp"
#include "../core/constructors.hpp"

// Call function value with 0 arguments
inline QValue q_call0(QValue f) {
    if (f.type != QValue::VAL_FUNC) return qv_null();
    return reinterpret_cast<QFunc0>(f.data.func_val)();
}

// Call function value with 1 argument
inline QValue q_call1(QValue f, QValue a) {
    if (f.type != QValue::VAL_FUNC) return qv_null();
    return reinterpret_cast<QFunc1>(f.data.func_val)(a);
}

// Call function value with 2 arguments
inline QValue q_call2(QValue f, QValue a, QValue b) {
    if (f.type != QValue::VAL_FUNC) return qv_null();
    return reinterpret_cast<QFunc2>(f.data.func_val)(a, b);
}

// Call function value with 3 arguments
inline QValue q_call3(QValue f, QValue a, QValue b, QValue c) {
    if (f.type != QValue::VAL_FUNC) return qv_null();
    return reinterpret_cast<QFunc3>(f.data.func_val)(a, b, c);
}

// Call function value with 4 arguments
inline QValue q_call4(QValue f, QValue a, QValue b, QValue c, QValue d) {
    if (f.type != QValue::VAL_FUNC) return qv_null();
    return reinterpret_cast<QFunc4>(f.data.func_val)(a, b, c, d);
}

#endif // QUARK_TYPES_FUNCTION_HPP
