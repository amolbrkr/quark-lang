// quark/core/sum.hpp - float summation shared by the vec reductions (sum, mean,
// sd) and the table aggregates (specs/tables_v0_1_spec.md D13), so that sum(v),
// mean(v) and summarize over the same values give the same bits.
#ifndef QUARK_CORE_SUM_HPP
#define QUARK_CORE_SUM_HPP

#include "par.hpp"

#include <cmath>
#include <cstddef>
#include <cstdint>
#include <memory>
#include <utility>
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

// The parallel pairwise sum (specs/tables_v0_3_spec.md §8.4): the halving
// above depends only on n, so the subtrees of at most q_par_block values are
// summed as the blocks of a parallel loop, and their sums are added up in
// the tree's order. The result is q_pairwise_sum's, bit for bit, for every
// thread count.
namespace quark {
namespace par {

inline size_t pairwise_half(size_t n) {
    size_t half = n / 2;
    return half - half % 8;
}

// pairwise_leaves lists, in order, the subtrees of the halving over n values
// (from off) that have at most q_par_block values.
inline void pairwise_leaves(size_t off, size_t n, std::vector<std::pair<size_t, size_t>>& out) {
    if (n <= static_cast<size_t>(q_par_block)) {
        out.push_back({off, n});
        return;
    }
    const size_t half = pairwise_half(n);
    pairwise_leaves(off, half, out);
    pairwise_leaves(off + half, n - half, out);
}

// pairwise_combine adds the leaf sums part[k...] as the tree over n values
// adds them.
inline double pairwise_combine(size_t n, const double* part, size_t& k) {
    if (n <= static_cast<size_t>(q_par_block)) return part[k++];
    const size_t half = pairwise_half(n);
    const double a = pairwise_combine(half, part, k);
    const double b = pairwise_combine(n - half, part, k);
    return a + b;
}

} // namespace par
} // namespace quark

inline double q_pairwise_sum_par(const double* x, size_t n) {
    if (q_par_plan(static_cast<long long>(n)).nblocks <= 1) return q_pairwise_sum(x, n);
    std::vector<std::pair<size_t, size_t>> leaves;
    quark::par::pairwise_leaves(0, n, leaves);
    std::vector<double> part(leaves.size());
    quark::par::Plan lp;
    lp.nblocks = static_cast<long long>(leaves.size());
    lp.n = lp.nblocks * q_par_block;
    q_par_run(lp, [&](long long, long long, long long b) {
        const auto& leaf = leaves[static_cast<size_t>(b)];
        part[static_cast<size_t>(b)] = q_pairwise_sum(x + leaf.first, leaf.second);
    });
    size_t k = 0;
    return quark::par::pairwise_combine(n, part.data(), k);
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
    const double mean = q_pairwise_sum_par(x, n) / static_cast<double>(n);
    std::unique_ptr<double[]> sq(new double[n]);
    double* out = sq.get();
    const quark::par::Plan p = q_par_plan(static_cast<long long>(n));
    q_par_run(p, [&](long long lo, long long hi, long long) {
        for (long long i = lo; i < hi; i++) {
            const double d = x[i] - mean;
            out[i] = d * d;
        }
    });
    return std::sqrt(q_pairwise_sum_par(out, n) / static_cast<double>(n - 1));
}

#endif // QUARK_CORE_SUM_HPP
