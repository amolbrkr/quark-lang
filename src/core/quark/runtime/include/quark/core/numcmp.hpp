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

// q_int_exact_in_float reports whether |i| <= 2^53, the range in which every
// int is exactly a float. Inside it, converting the int and comparing the two
// floats (IEEE 754, NaN included) gives the exact answer; this is the fast
// path of every comparison below.
inline bool q_int_exact_in_float(long long i) {
    return static_cast<unsigned long long>(i) + 9007199254740992ULL <= 18014398509481984ULL;
}

// q_cmp_int_float_slow is q_cmp_int_float for |i| > 2^53.
inline int q_cmp_int_float_slow(long long i, double f) {
    if (f != f) return 2;
    // 2^63 is the smallest double past every positive int; -2^63 is the
    // smallest int, and every double below it is below every int.
    constexpr double k2_63 = 9223372036854775808.0;
    if (f >= k2_63) return -1;
    if (f < -k2_63) return 1;
    // A float inside ±2^53 is on the int's other side.
    constexpr double kMantissa = 9007199254740992.0;
    if (f <= kMantissa && f >= -kMantissa) return i > 0 ? 1 : -1;
    // Past 2^53 every finite float is an integer, and f is inside the int
    // range, so the cast truncates nothing.
    const long long fi = static_cast<long long>(f);
    if (i < fi) return -1;
    if (i > fi) return 1;
    return 0;
}

// q_cmp_int_float returns -1 if i < f, 0 if they are equal, 1 if i > f,
// and 2 if f is NaN (unordered: == is false, ordered comparisons are false,
// != is true).
inline int q_cmp_int_float(long long i, double f) {
    if (__builtin_expect(q_int_exact_in_float(i), 1)) {
        const double di = static_cast<double>(i);
        if (di < f) return -1;
        if (di > f) return 1;
        if (di == f) return 0;
        return 2;
    }
    return q_cmp_int_float_slow(i, f);
}

// q_int_fits_float reports whether i converts to a float and back unchanged.
inline bool q_int_fits_float(long long i) {
    if (q_int_exact_in_float(i)) return true;
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

// q_int_float_<op>(i, f) is `i <op> f` on the mathematical values. Lowering
// emits these for a comparison of a raw int with a raw float.
inline bool q_int_float_lt(long long i, double f) {
    if (__builtin_expect(q_int_exact_in_float(i), 1)) return static_cast<double>(i) < f;
    return q_cmp_int_float_slow(i, f) == -1;
}
inline bool q_int_float_le(long long i, double f) {
    if (__builtin_expect(q_int_exact_in_float(i), 1)) return static_cast<double>(i) <= f;
    const int c = q_cmp_int_float_slow(i, f);
    return c == -1 || c == 0;
}
inline bool q_int_float_gt(long long i, double f) {
    if (__builtin_expect(q_int_exact_in_float(i), 1)) return static_cast<double>(i) > f;
    return q_cmp_int_float_slow(i, f) == 1;
}
inline bool q_int_float_ge(long long i, double f) {
    if (__builtin_expect(q_int_exact_in_float(i), 1)) return static_cast<double>(i) >= f;
    const int c = q_cmp_int_float_slow(i, f);
    return c == 1 || c == 0;
}
inline bool q_int_float_eq(long long i, double f) {
    if (__builtin_expect(q_int_exact_in_float(i), 1)) return static_cast<double>(i) == f;
    return q_cmp_int_float_slow(i, f) == 0;
}
inline bool q_int_float_ne(long long i, double f) {
    if (__builtin_expect(q_int_exact_in_float(i), 1)) return static_cast<double>(i) != f;
    return q_cmp_int_float_slow(i, f) != 0;
}

#endif // QUARK_CORE_NUMCMP_HPP
