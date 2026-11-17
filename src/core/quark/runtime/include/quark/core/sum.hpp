// quark/core/sum.hpp - float summation shared by the vec reductions (sum, mean,
// sd) and the table aggregates (specs/tables_v0_1_spec.md D13), so that sum(v),
// mean(v) and summarize over the same values give the same bits.
#ifndef QUARK_CORE_SUM_HPP
#define QUARK_CORE_SUM_HPP

#include <cmath>
#include <cstddef>
#include <cstdint>
#include <vector>

// q_pairwise_sum is NumPy's pairwise summation in double: blocks of up to
// 128 values are summed in eight independent lanes, and blocks are combined
// pairwise. The rounding error grows with log n rather than n, the inner loop
// vectorizes, and the result is the same on every platform: plain double
// operations in a fixed order (no long double, whose width differs between
// x86-64, macOS arm64 and Linux arm64, and no reassociation).
inline double q_pairwise_sum(const double* x, size_t n) {
    if (n < 8) {
        double s = 0.0;
        for (size_t i = 0; i < n; i++) s += x[i];
        return s;
    }
    if (n <= 128) {
        double r[8];
        for (size_t j = 0; j < 8; j++) r[j] = x[j];
        size_t i = 8;
        for (; i + 8 <= n; i += 8) {
            for (size_t j = 0; j < 8; j++) r[j] += x[i + j];
        }
        double s = ((r[0] + r[1]) + (r[2] + r[3])) + ((r[4] + r[5]) + (r[6] + r[7]));
        for (; i < n; i++) s += x[i];
        return s;
    }
    size_t half = n / 2;
    half -= half % 8;  // keep the blocks whole multiples of the lane count
    return q_pairwise_sum(x, half) + q_pairwise_sum(x + half, n - half);
}

// q_i128_to_double converts an exact 128-bit int sum to double, with plain
// 64-bit conversions only (no compiler runtime call, which not every target
// links): exact for sums that fit in 64 bits, and within a rounding or two
// beyond that.
inline double q_i128_to_double(__int128 v) {
    if (v >= INT64_MIN && v <= INT64_MAX) return static_cast<double>(static_cast<int64_t>(v));
    const int64_t hi = static_cast<int64_t>(v >> 64);
    const uint64_t lo = static_cast<uint64_t>(v);
    return static_cast<double>(hi) * 18446744073709551616.0 + static_cast<double>(lo);
}

// q_sample_sd is the sample standard deviation of n values (n >= 2): the
// mean, then the pairwise sum of squared deviations from it, divided by
// n - 1, then the square root (specs/tables_v0_3_spec.md D40). The caller
// passes the present values in order, so a column and a vec of those values
// share this kernel.
inline double q_sample_sd(const double* x, size_t n) {
    const double mean = q_pairwise_sum(x, n) / static_cast<double>(n);
    std::vector<double> sq(n);
    for (size_t i = 0; i < n; i++) {
        const double d = x[i] - mean;
        sq[i] = d * d;
    }
    return std::sqrt(q_pairwise_sum(sq.data(), n) / static_cast<double>(n - 1));
}

#endif // QUARK_CORE_SUM_HPP
