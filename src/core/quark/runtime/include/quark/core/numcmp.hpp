// quark/core/numcmp.hpp - Exact comparison of an int with a float.
//
// A float cannot hold every int (past 2^53, consecutive ints collapse onto
// one double). Comparing by converting the int to a float would treat
// 9007199254740993 and 9007199254740992.0 as equal. These helpers compare
// the mathematical values: an int that fits in a float exactly compares as
// that float, and a larger int compares with the float's integer value.
// Arithmetic still promotes; only comparisons, ordering and min/max use this.
#ifndef QUARK_CORE_NUMCMP_HPP
#define QUARK_CORE_NUMCMP_HPP

#include <cmath>
#include <limits>

// q_cmp_int_float returns -1 if i < f, 0 if they are equal, 1 if i > f,
// and 2 if f is NaN (unordered: == is false, ordered comparisons are false,
// != is true).
inline int q_cmp_int_float(long long i, double f) {
    if (f != f) return 2;
    if (f == std::numeric_limits<double>::infinity()) return -1;
    if (f == -std::numeric_limits<double>::infinity()) return 1;
    // 2^63 is the smallest double past every positive int. 2^53 is the
    // largest absolute value at which every int is exactly a float.
    constexpr double k2_63 = 9223372036854775808.0;
    constexpr long long k2_53 = 9007199254740992LL;
    if (f >= k2_63) return -1;
    if (f < -k2_63) return 1;
    if (i >= -k2_53 && i <= k2_53) {
        const double di = static_cast<double>(i);
        if (di < f) return -1;
        if (di > f) return 1;
        return 0;
    }
    // |i| > 2^53. A float inside ±2^53 is on the int's other side.
    constexpr double kMantissa = 9007199254740992.0;
    if (f <= kMantissa && f >= -kMantissa) return i > 0 ? 1 : -1;
    // Past 2^53 every finite float is an integer, and f is inside the int
    // range, so the cast truncates nothing.
    const long long fi = static_cast<long long>(f);
    if (i < fi) return -1;
    if (i > fi) return 1;
    return 0;
}

// q_int_fits_float reports whether i converts to a float and back unchanged.
inline bool q_int_fits_float(long long i) {
    constexpr long long k2_53 = 9007199254740992LL;
    if (i >= -k2_53 && i <= k2_53) return true;
    const double d = static_cast<double>(i);
    constexpr double k2_63 = 9223372036854775808.0;
    if (d >= k2_63 || d < -k2_63) return false;
    return static_cast<long long>(d) == i;
}

// q_cmp_holds is the operator named by op ("<", "<=", ">", ">=", "==", "!=")
// applied to a q_cmp_int_float result. NaN (2) makes != true and the rest false.
inline bool q_cmp_holds(int c, const char* op) {
    switch (op[0]) {
        case '<': return op[1] == '=' ? (c == -1 || c == 0) : c == -1;
        case '>': return op[1] == '=' ? (c == 1 || c == 0) : c == 1;
        case '=': return c == 0;
        case '!': return c != 0;
        default: return false;
    }
}

inline bool q_int_float_lt(long long i, double f) { return q_cmp_holds(q_cmp_int_float(i, f), "<"); }
inline bool q_int_float_le(long long i, double f) { return q_cmp_holds(q_cmp_int_float(i, f), "<="); }
inline bool q_int_float_gt(long long i, double f) { return q_cmp_holds(q_cmp_int_float(i, f), ">"); }
inline bool q_int_float_ge(long long i, double f) { return q_cmp_holds(q_cmp_int_float(i, f), ">="); }
inline bool q_int_float_eq(long long i, double f) { return q_cmp_holds(q_cmp_int_float(i, f), "=="); }
inline bool q_int_float_ne(long long i, double f) { return q_cmp_holds(q_cmp_int_float(i, f), "!="); }

#endif // QUARK_CORE_NUMCMP_HPP
