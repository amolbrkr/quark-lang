// ext_smoke.hpp — C++ extension used by smoke_extern.qrk
#ifndef EXT_SMOKE_HPP
#define EXT_SMOKE_HPP

#include <cstdint>
#include <cstdio>

inline int64_t qei_add1(int64_t x) {
    return x + 1;
}

inline double qei_halve(double x) {
    return x / 2.0;
}

inline bool qei_is_upper(const char* s) {
    if (!s || !*s) return false;
    return s[0] >= 'A' && s[0] <= 'Z';
}

inline const char* qei_greet(const char* name) {
    static thread_local char buf[256];
    std::snprintf(buf, sizeof(buf), "hello %s", name);
    // Return a GC-managed copy; do not return stack/thread-local storage.
    return q_strdup(buf);
}

// Callbacks: `fn` parameters arrive as QClosure*, called with qext::call.
// Applies f to x n times: f(f(...f(x))).
inline QValue qei_apply_n(QClosure* f, int64_t n, QValue x) {
    for (int64_t i = 0; i < n; i++) x = qext::call(f, x);
    return x;
}

// Returns the fn value it was given (QClosure* in, QClosure* out).
inline QClosure* qei_same_fn(QClosure* f) {
    return f;
}

#endif // EXT_SMOKE_HPP
