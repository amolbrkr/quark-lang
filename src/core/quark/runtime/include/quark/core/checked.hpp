// quark/core/checked.hpp - Checked integer arithmetic shared by operators,
// scalar-lowered codegen output, and i64 vector kernels.
#ifndef QUARK_CORE_CHECKED_HPP
#define QUARK_CORE_CHECKED_HPP

#include "diagnostics.hpp"
#include <climits>
#include <cstdlib>

// ------------------------------------------------------------
// Checked integer arithmetic.
//
// Signed overflow is undefined behavior in C++, so int arithmetic must never
// overflow silently. Every int path (boxed operators, scalar-lowered locals
// emitted by codegen, and i64 vector kernels) goes through these helpers so
// overflow is a deterministic fatal error everywhere.
// ------------------------------------------------------------
[[noreturn]] inline void q_int_overflow(const char* op) {
    q_runtime_reportf("runtime error: integer overflow in '%s'\n", op);
    std::exit(1);
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
