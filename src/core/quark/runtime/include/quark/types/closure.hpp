// quark/types/closure.hpp - Closure support for captured variables
#ifndef QUARK_TYPES_CLOSURE_HPP
#define QUARK_TYPES_CLOSURE_HPP

#include "../core/value.hpp"
#include "../core/cell.hpp"
#include "../core/gc.hpp"

// QFnInfo: static metadata the compiler emits for every function that can
// become a value (named functions, lambdas, native and extern thunks). Calls
// whose signature the compiler doesn't know (a fn value from a parameter, a
// list, a struct field, or native code) check the argument count against it
// and fill in omitted default arguments, so a wrong call is a runtime error
// instead of the callee reading a missing argument.
struct QFnInfo {
    const char* name;  // display name for errors ("add", "lambda")
    int min_arity;     // parameters without a default
    int max_arity;     // all parameters
    // Value of default parameter `index` (min_arity <= index < max_arity),
    // built fresh on every call; null when the function has no defaults.
    QValue (*default_arg)(int index);
};

// QClosure: holds a function pointer + captured values
// All function values (named, lambda, closure) are represented as QClosure*
// stored in QValue.data.func_val
struct QClosure {
    void* func;           // The actual function pointer (takes QClosure* as first arg)
    const QFnInfo* info;  // Arity and defaults; never null
    int capture_count;    // Number of captured values (0 for non-capturing)
    QCell* captures[];    // Flexible array of shared captured variable cells
};

// Allocate a closure with N captures via GC
inline QClosure* q_alloc_closure(void* func, const QFnInfo* info, int capture_count) {
    // captures[] is QCell*[], so allocate pointer-sized slots (not QValue-sized)
    static_assert(sizeof(QCell*) <= sizeof(QValue), "QCell* must not exceed QValue size");
    size_t size = sizeof(QClosure) + capture_count * sizeof(QCell*);
    QClosure* cl = static_cast<QClosure*>(q_malloc(size));
    cl->func = func;
    cl->info = info;
    cl->capture_count = capture_count;
    return cl;
}

#endif // QUARK_TYPES_CLOSURE_HPP
