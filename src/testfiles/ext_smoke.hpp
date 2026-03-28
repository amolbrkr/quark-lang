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

#endif // EXT_SMOKE_HPP
