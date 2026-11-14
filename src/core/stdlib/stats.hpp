// stdlib/stats.hpp — native implementations for the std/stats module (stats.qrk).
#ifndef QUARK_STDLIB_STATS_HPP
#define QUARK_STDLIB_STATS_HPP

#include <cmath>

// Mean of the present values of a vec, with the table aggregate's rules
// (specs/tables_v0_1_spec.md D6, table/engine.hpp agg_mean): missing entries and
// NaN are skipped, a bool vec gives the share of true values
// (q_vec_mean_present). A vec with no present values is an error, as for min
// and max (D13).
inline double q_stats_mean(QVector* v) {
    if (v == nullptr || !q_vec_validate(*v)) {
        q_runtime_reportf("runtime error: stats::mean() expects a vec\n");
        std::exit(1);
    }
    if (v->type == QVector::Type::STR) {
        q_runtime_reportf("runtime error: stats::mean() expects a numeric or bool vec, got vec[str]\n");
        std::exit(1);
    }
    size_t count = 0;
    const double mean = q_vec_mean_present(*v, &count);
    if (count == 0) {
        q_runtime_reportf("runtime error: stats::mean() of a vec with no present values\n");
        std::exit(1);
    }
    return mean;
}

#endif // QUARK_STDLIB_STATS_HPP
