// quark/types/closure.hpp - Closure support for captured variables
#ifndef QUARK_TYPES_CLOSURE_HPP
#define QUARK_TYPES_CLOSURE_HPP

#include "../core/value.hpp"
#include "../core/gc.hpp"

// QClosure: holds a function pointer + captured values
// All function values (named, lambda, closure) are represented as QClosure*
// stored in QValue.data.func_val
struct QClosure {
    void* func;           // The actual function pointer (takes QClosure* as first arg)
    int capture_count;    // Number of captured values (0 for non-capturing)
    QValue captures[];    // Flexible array of captured values
};

// Allocate a closure with N captures via GC
inline QClosure* q_alloc_closure(void* func, int capture_count) {
    size_t size = sizeof(QClosure) + capture_count * sizeof(QValue);
    QClosure* cl = static_cast<QClosure*>(q_malloc(size));
    cl->func = func;
    cl->capture_count = capture_count;
    return cl;
}

#endif // QUARK_TYPES_CLOSURE_HPP
