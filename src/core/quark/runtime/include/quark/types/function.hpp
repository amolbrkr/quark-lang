// quark/types/function.hpp - Function value operations
#ifndef QUARK_TYPES_FUNCTION_HPP
#define QUARK_TYPES_FUNCTION_HPP

#include "../core/value.hpp"
#include "../core/constructors.hpp"
#include "closure.hpp"
#include <cstdio>
#include <cstdlib>
#include <vector>

inline bool q_require_callable(const QValue& f) {
    if (f.type == QValue::VAL_FUNC) {
        return true;
    }
    std::fprintf(stderr, "runtime error: attempted to call a non-function value\n");
    std::exit(1);
}

// Call function value with 0 arguments
inline QValue q_call0(QValue f) {
    q_require_callable(f);
    QClosure* cl = static_cast<QClosure*>(f.data.func_val);
    return reinterpret_cast<QClFunc0>(cl->func)(cl);
}

// Call function value with 1 argument
inline QValue q_call1(QValue f, QValue a) {
    q_require_callable(f);
    QClosure* cl = static_cast<QClosure*>(f.data.func_val);
    return reinterpret_cast<QClFunc1>(cl->func)(cl, a);
}

// Call function value with 2 arguments
inline QValue q_call2(QValue f, QValue a, QValue b) {
    q_require_callable(f);
    QClosure* cl = static_cast<QClosure*>(f.data.func_val);
    return reinterpret_cast<QClFunc2>(cl->func)(cl, a, b);
}

// Call function value with 3 arguments
inline QValue q_call3(QValue f, QValue a, QValue b, QValue c) {
    q_require_callable(f);
    QClosure* cl = static_cast<QClosure*>(f.data.func_val);
    return reinterpret_cast<QClFunc3>(cl->func)(cl, a, b, c);
}

// Call function value with 4 arguments
inline QValue q_call4(QValue f, QValue a, QValue b, QValue c, QValue d) {
    q_require_callable(f);
    QClosure* cl = static_cast<QClosure*>(f.data.func_val);
    return reinterpret_cast<QClFunc4>(cl->func)(cl, a, b, c, d);
}

inline QValue q_call5(QValue f, QValue a, QValue b, QValue c, QValue d, QValue e) {
    q_require_callable(f);
    QClosure* cl = static_cast<QClosure*>(f.data.func_val);
    return reinterpret_cast<QClFunc5>(cl->func)(cl, a, b, c, d, e);
}

inline QValue q_call6(QValue f, QValue a, QValue b, QValue c, QValue d, QValue e, QValue g) {
    q_require_callable(f);
    QClosure* cl = static_cast<QClosure*>(f.data.func_val);
    return reinterpret_cast<QClFunc6>(cl->func)(cl, a, b, c, d, e, g);
}

inline QValue q_call7(QValue f, QValue a, QValue b, QValue c, QValue d, QValue e, QValue g, QValue h) {
    q_require_callable(f);
    QClosure* cl = static_cast<QClosure*>(f.data.func_val);
    return reinterpret_cast<QClFunc7>(cl->func)(cl, a, b, c, d, e, g, h);
}

inline QValue q_call8(QValue f, QValue a, QValue b, QValue c, QValue d, QValue e, QValue g, QValue h, QValue i) {
    q_require_callable(f);
    QClosure* cl = static_cast<QClosure*>(f.data.func_val);
    return reinterpret_cast<QClFunc8>(cl->func)(cl, a, b, c, d, e, g, h, i);
}

inline QValue q_call9(QValue f, QValue a, QValue b, QValue c, QValue d, QValue e, QValue g, QValue h, QValue i, QValue j) {
    q_require_callable(f);
    QClosure* cl = static_cast<QClosure*>(f.data.func_val);
    return reinterpret_cast<QClFunc9>(cl->func)(cl, a, b, c, d, e, g, h, i, j);
}

inline QValue q_call10(QValue f, QValue a, QValue b, QValue c, QValue d, QValue e, QValue g, QValue h, QValue i, QValue j, QValue k) {
    q_require_callable(f);
    QClosure* cl = static_cast<QClosure*>(f.data.func_val);
    return reinterpret_cast<QClFunc10>(cl->func)(cl, a, b, c, d, e, g, h, i, j, k);
}

inline QValue q_call11(QValue f, QValue a, QValue b, QValue c, QValue d, QValue e, QValue g, QValue h, QValue i, QValue j, QValue k, QValue l) {
    q_require_callable(f);
    QClosure* cl = static_cast<QClosure*>(f.data.func_val);
    return reinterpret_cast<QClFunc11>(cl->func)(cl, a, b, c, d, e, g, h, i, j, k, l);
}

inline QValue q_call12(QValue f, QValue a, QValue b, QValue c, QValue d, QValue e, QValue g, QValue h, QValue i, QValue j, QValue k, QValue l, QValue m) {
    q_require_callable(f);
    QClosure* cl = static_cast<QClosure*>(f.data.func_val);
    return reinterpret_cast<QClFunc12>(cl->func)(cl, a, b, c, d, e, g, h, i, j, k, l, m);
}

inline QValue q_calln(QValue f, const std::vector<QValue>& args) {
    switch (args.size()) {
        case 0:  return q_call0(f);
        case 1:  return q_call1(f, args[0]);
        case 2:  return q_call2(f, args[0], args[1]);
        case 3:  return q_call3(f, args[0], args[1], args[2]);
        case 4:  return q_call4(f, args[0], args[1], args[2], args[3]);
        case 5:  return q_call5(f, args[0], args[1], args[2], args[3], args[4]);
        case 6:  return q_call6(f, args[0], args[1], args[2], args[3], args[4], args[5]);
        case 7:  return q_call7(f, args[0], args[1], args[2], args[3], args[4], args[5], args[6]);
        case 8:  return q_call8(f, args[0], args[1], args[2], args[3], args[4], args[5], args[6], args[7]);
        case 9:  return q_call9(f, args[0], args[1], args[2], args[3], args[4], args[5], args[6], args[7], args[8]);
        case 10: return q_call10(f, args[0], args[1], args[2], args[3], args[4], args[5], args[6], args[7], args[8], args[9]);
        case 11: return q_call11(f, args[0], args[1], args[2], args[3], args[4], args[5], args[6], args[7], args[8], args[9], args[10]);
        case 12: return q_call12(f, args[0], args[1], args[2], args[3], args[4], args[5], args[6], args[7], args[8], args[9], args[10], args[11]);
        default:
            std::fprintf(stderr, "runtime error: function call supports up to 12 arguments, got %zu\n", args.size());
            std::exit(1);
    }
}

#endif // QUARK_TYPES_FUNCTION_HPP
