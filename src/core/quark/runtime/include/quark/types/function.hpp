// quark/types/function.hpp - Function value operations
#ifndef QUARK_TYPES_FUNCTION_HPP
#define QUARK_TYPES_FUNCTION_HPP

#include "../core/value.hpp"
#include "../core/constructors.hpp"
#include "closure.hpp"
#include <cstdio>

inline bool q_require_callable(const QValue& f) {
    if (f.type == QValue::VAL_FUNC) {
        return true;
    }
    std::fprintf(stderr, "runtime error: attempted to call a non-function value\n");
    return false;
}

// Call function value with 0 arguments
inline QValue q_call0(QValue f) {
    if (!q_require_callable(f)) return qv_null();
    QClosure* cl = static_cast<QClosure*>(f.data.func_val);
    return reinterpret_cast<QClFunc0>(cl->func)(cl);
}

// Call function value with 1 argument
inline QValue q_call1(QValue f, QValue a) {
    if (!q_require_callable(f)) return qv_null();
    QClosure* cl = static_cast<QClosure*>(f.data.func_val);
    return reinterpret_cast<QClFunc1>(cl->func)(cl, a);
}

// Call function value with 2 arguments
inline QValue q_call2(QValue f, QValue a, QValue b) {
    if (!q_require_callable(f)) return qv_null();
    QClosure* cl = static_cast<QClosure*>(f.data.func_val);
    return reinterpret_cast<QClFunc2>(cl->func)(cl, a, b);
}

// Call function value with 3 arguments
inline QValue q_call3(QValue f, QValue a, QValue b, QValue c) {
    if (!q_require_callable(f)) return qv_null();
    QClosure* cl = static_cast<QClosure*>(f.data.func_val);
    return reinterpret_cast<QClFunc3>(cl->func)(cl, a, b, c);
}

// Call function value with 4 arguments
inline QValue q_call4(QValue f, QValue a, QValue b, QValue c, QValue d) {
    if (!q_require_callable(f)) return qv_null();
    QClosure* cl = static_cast<QClosure*>(f.data.func_val);
    return reinterpret_cast<QClFunc4>(cl->func)(cl, a, b, c, d);
}

#endif // QUARK_TYPES_FUNCTION_HPP
