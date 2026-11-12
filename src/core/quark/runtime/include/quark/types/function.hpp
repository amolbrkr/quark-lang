// quark/types/function.hpp - Function value operations
#ifndef QUARK_TYPES_FUNCTION_HPP
#define QUARK_TYPES_FUNCTION_HPP

#include "../core/value.hpp"
#include "../core/constructors.hpp"
#include "closure.hpp"
#include <cstdio>
#include <cstdlib>
#include <cstring>
#include <vector>

// Largest number of arguments a function value can be called with.
constexpr int Q_MAX_CALL_ARGS = 12;

inline QClosure* q_require_closure(const QValue& f) {
    if (f.type != QValue::VAL_FUNC) {
        q_runtime_reportf("runtime error: attempted to call a non-function value\n");
        std::exit(1);
    }
    return static_cast<QClosure*>(f.data.func_val);
}

[[noreturn]] inline void q_arity_error(const QFnInfo* info, int argc) {
    char expects[64];
    if (info->min_arity == info->max_arity) {
        std::snprintf(expects, sizeof(expects), "%d argument%s", info->max_arity, info->max_arity == 1 ? "" : "s");
    } else {
        std::snprintf(expects, sizeof(expects), "%d-%d arguments", info->min_arity, info->max_arity);
    }
    if (std::strcmp(info->name, "lambda") == 0) {
        q_runtime_reportf("runtime error: function expects %s but got %d\n", expects, argc);
    } else {
        q_runtime_reportf("runtime error: function '%s' expects %s but got %d\n", info->name, expects, argc);
    }
    std::exit(1);
}

// q_call_adapt is the slow path of a call whose argument count differs from
// the callee's parameter count: it rejects a count outside the callee's range
// and fills omitted trailing parameters with their defaults. argv has room for
// Q_MAX_CALL_ARGS values.
inline QValue q_call_adapt(QClosure* cl, int argc, QValue* argv) {
    const QFnInfo* info = cl->info;
    if (argc < info->min_arity || argc > info->max_arity) q_arity_error(info, argc);
    for (int i = argc; i < info->max_arity; i++) argv[i] = info->default_arg(i);
    void* fn = cl->func;
    QValue* a = argv;
    switch (info->max_arity) {
        case 0:  return reinterpret_cast<QClFunc0>(fn)(cl);
        case 1:  return reinterpret_cast<QClFunc1>(fn)(cl, a[0]);
        case 2:  return reinterpret_cast<QClFunc2>(fn)(cl, a[0], a[1]);
        case 3:  return reinterpret_cast<QClFunc3>(fn)(cl, a[0], a[1], a[2]);
        case 4:  return reinterpret_cast<QClFunc4>(fn)(cl, a[0], a[1], a[2], a[3]);
        case 5:  return reinterpret_cast<QClFunc5>(fn)(cl, a[0], a[1], a[2], a[3], a[4]);
        case 6:  return reinterpret_cast<QClFunc6>(fn)(cl, a[0], a[1], a[2], a[3], a[4], a[5]);
        case 7:  return reinterpret_cast<QClFunc7>(fn)(cl, a[0], a[1], a[2], a[3], a[4], a[5], a[6]);
        case 8:  return reinterpret_cast<QClFunc8>(fn)(cl, a[0], a[1], a[2], a[3], a[4], a[5], a[6], a[7]);
        case 9:  return reinterpret_cast<QClFunc9>(fn)(cl, a[0], a[1], a[2], a[3], a[4], a[5], a[6], a[7], a[8]);
        case 10: return reinterpret_cast<QClFunc10>(fn)(cl, a[0], a[1], a[2], a[3], a[4], a[5], a[6], a[7], a[8], a[9]);
        case 11: return reinterpret_cast<QClFunc11>(fn)(cl, a[0], a[1], a[2], a[3], a[4], a[5], a[6], a[7], a[8], a[9], a[10]);
        case 12: return reinterpret_cast<QClFunc12>(fn)(cl, a[0], a[1], a[2], a[3], a[4], a[5], a[6], a[7], a[8], a[9], a[10], a[11]);
        default:
            q_runtime_reportf("runtime error: a function value can take at most %d parameters to be called, '%s' has %d\n",
                              Q_MAX_CALL_ARGS, info->name, info->max_arity);
            std::exit(1);
    }
}

// q_call_values calls a function value. The common case, an argument for
// every parameter, is a direct call; anything else goes through q_call_adapt.
template <typename... Args>
inline QValue q_call_values(QValue f, Args... args) {
    constexpr int n = static_cast<int>(sizeof...(Args));
    static_assert(n <= Q_MAX_CALL_ARGS, "too many arguments for a function value call");
    QClosure* cl = q_require_closure(f);
    if (cl->info->max_arity == n) {
        using Fn = QValue (*)(QClosure*, Args...);
        return reinterpret_cast<Fn>(cl->func)(cl, args...);
    }
    QValue argv[Q_MAX_CALL_ARGS] = {args...};
    return q_call_adapt(cl, n, argv);
}

// q_call0 .. q_call12: the entry points codegen emits for calls through a
// function value (q_callN takes N arguments).
inline QValue q_call0(QValue f) { return q_call_values(f); }
inline QValue q_call1(QValue f, QValue a) { return q_call_values(f, a); }
inline QValue q_call2(QValue f, QValue a, QValue b) { return q_call_values(f, a, b); }
inline QValue q_call3(QValue f, QValue a, QValue b, QValue c) { return q_call_values(f, a, b, c); }
inline QValue q_call4(QValue f, QValue a, QValue b, QValue c, QValue d) { return q_call_values(f, a, b, c, d); }
inline QValue q_call5(QValue f, QValue a, QValue b, QValue c, QValue d, QValue e) { return q_call_values(f, a, b, c, d, e); }
inline QValue q_call6(QValue f, QValue a, QValue b, QValue c, QValue d, QValue e, QValue g) { return q_call_values(f, a, b, c, d, e, g); }
inline QValue q_call7(QValue f, QValue a, QValue b, QValue c, QValue d, QValue e, QValue g, QValue h) { return q_call_values(f, a, b, c, d, e, g, h); }
inline QValue q_call8(QValue f, QValue a, QValue b, QValue c, QValue d, QValue e, QValue g, QValue h, QValue i) { return q_call_values(f, a, b, c, d, e, g, h, i); }
inline QValue q_call9(QValue f, QValue a, QValue b, QValue c, QValue d, QValue e, QValue g, QValue h, QValue i, QValue j) { return q_call_values(f, a, b, c, d, e, g, h, i, j); }
inline QValue q_call10(QValue f, QValue a, QValue b, QValue c, QValue d, QValue e, QValue g, QValue h, QValue i, QValue j, QValue k) { return q_call_values(f, a, b, c, d, e, g, h, i, j, k); }
inline QValue q_call11(QValue f, QValue a, QValue b, QValue c, QValue d, QValue e, QValue g, QValue h, QValue i, QValue j, QValue k, QValue l) { return q_call_values(f, a, b, c, d, e, g, h, i, j, k, l); }
inline QValue q_call12(QValue f, QValue a, QValue b, QValue c, QValue d, QValue e, QValue g, QValue h, QValue i, QValue j, QValue k, QValue l, QValue m) { return q_call_values(f, a, b, c, d, e, g, h, i, j, k, l, m); }

inline QValue q_calln(QValue f, const std::vector<QValue>& args) {
    if (args.size() > static_cast<size_t>(Q_MAX_CALL_ARGS)) {
        q_runtime_reportf("runtime error: function call supports up to %d arguments, got %zu\n", Q_MAX_CALL_ARGS, args.size());
        std::exit(1);
    }
    QValue argv[Q_MAX_CALL_ARGS] = {};
    for (size_t i = 0; i < args.size(); i++) argv[i] = args[i];
    QClosure* cl = q_require_closure(f);
    return q_call_adapt(cl, static_cast<int>(args.size()), argv);
}

#endif // QUARK_TYPES_FUNCTION_HPP
