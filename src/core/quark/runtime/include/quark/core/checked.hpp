// quark/core/checked.hpp - Checked integer arithmetic shared by operators,
// raw scalar arithmetic in generated code, and i64 vector kernels.
#ifndef QUARK_CORE_CHECKED_HPP
#define QUARK_CORE_CHECKED_HPP

#include "diagnostics.hpp"
#include "format.hpp"
#include <climits>
#include <cstdlib>

// ------------------------------------------------------------
// Checked integer arithmetic.
//
// Signed overflow is undefined behavior in C++, so int arithmetic must never
// overflow silently. Every int path (boxed operators, scalar-lowered locals
// emitted by the compiler, and i64 vector kernels) goes through these helpers so
// overflow is a deterministic fatal error everywhere.
// ------------------------------------------------------------
[[noreturn]] inline void q_int_overflow(const char* op) {
    q_runtime_reportf("runtime error: integer overflow in '%s'\n", op);
    std::exit(1);
}

// q_resolve_index is a list or string index as written (negative counts from
// the end) resolved against len, in 64 bits: the caller checks the bounds.
// len + given can't overflow, since len >= 0 > given.
inline long long q_resolve_index(long long given, long long len) {
    return given < 0 ? len + given : given;
}

inline long long q_checked_add(long long a, long long b) {
    long long r;
    if (__builtin_add_overflow(a, b, &r)) q_int_overflow("+");
    return r;
}

inline long long q_checked_sub(long long a, long long b) {
    long long r;
    if (__builtin_sub_overflow(a, b, &r)) q_int_overflow("-");
    return r;
}

inline long long q_checked_mul(long long a, long long b) {
    long long r;
    if (__builtin_mul_overflow(a, b, &r)) q_int_overflow("*");
    return r;
}

inline long long q_checked_neg(long long a) {
    if (a == LLONG_MIN) q_int_overflow("-");
    return -a;
}

inline long long q_checked_mod(long long a, long long b) {
    if (b == 0) {
        q_runtime_reportf("runtime error: modulo by zero\n");
        std::exit(1);
    }
    // LLONG_MIN % -1 is undefined in C++; the mathematical result is 0.
    if (b == -1) return 0;
    return a % b;
}

// q_checked_abs is |a|; the smallest int64 has no positive counterpart.
inline long long q_checked_abs(long long a) {
    if (a == LLONG_MIN) q_int_overflow("abs");
    return a < 0 ? -a : a;
}

// q_checked_f2i converts a float to an int for `what` ("to_int()", "floor()"):
// it truncates toward zero, and NaN or a value outside the int64 range is a
// runtime error (casting one is undefined behavior in C++).
inline long long q_checked_f2i(double x, const char* what) {
    if (!(x >= -9223372036854775808.0 && x < 9223372036854775808.0)) {
        char fbuf[32];
        q_runtime_reportf("runtime error: %s can't convert %s to an int: %s\n", what, q_format_float(x, fbuf, sizeof(fbuf)),
                          x != x ? "it is not a number" : "it is outside the int range");
        std::exit(1);
    }
    return static_cast<long long>(x);
}

inline double q_checked_fdiv(double a, double b) {
    if (b == 0.0) {
        q_runtime_reportf("runtime error: division by zero\n");
        std::exit(1);
    }
    return a / b;
}

// Exact integer power by repeated squaring. Negative exponents keep the
// existing truncating behavior (|base| > 1 gives 0).
inline long long q_checked_ipow(long long base, long long exp) {
    if (exp < 0) {
        if (base == 0) {
            q_runtime_reportf("runtime error: division by zero in '**'\n");
            std::exit(1);
        }
        if (base == 1) return 1;
        if (base == -1) return (exp % 2 == 0) ? 1 : -1;
        return 0;
    }
    long long result = 1;
    while (exp > 0) {
        if (exp & 1) {
            if (__builtin_mul_overflow(result, base, &result)) q_int_overflow("**");
        }
        exp >>= 1;
        if (exp > 0 && __builtin_mul_overflow(base, base, &base)) q_int_overflow("**");
    }
    return result;
}

#endif // QUARK_CORE_CHECKED_HPP
