// stdlib/random.hpp — native implementations for the std/random module (random.qrk).
#ifndef QUARK_STDLIB_RANDOM_HPP
#define QUARK_STDLIB_RANDOM_HPP

#include <algorithm>
#include <cmath>
#include <cstdint>
#include <numeric>
#include <random>
#include <vector>

// One generator per program, seeded from std::random_device unless seed() is
// called, so results are reproducible after seed(n).
inline std::mt19937_64& q_random_engine() {
    static std::mt19937_64 engine{std::random_device{}()};
    return engine;
}

inline QValue q_random_seed(int64_t seed) {
    q_random_engine().seed(static_cast<uint64_t>(seed));
    return qv_null();
}

[[noreturn]] inline void q_random_fail(const char* msg) {
    q_runtime_reportf("runtime error: %s\n", msg);
    std::exit(1);
}

// Uniform float in [lo, hi). lo == hi returns lo. lo > hi, or a non-finite
// bound, is an error. uniform() is this with lo = 0 and hi = 1.
inline double q_random_uniform_range(double lo, double hi) {
    if (!std::isfinite(lo) || !std::isfinite(hi) || lo > hi) {
        char a[32], b[32];
        q_format_float(lo, a, sizeof(a));
        q_format_float(hi, b, sizeof(b));
        q_runtime_reportf("runtime error: random::uniform() needs a finite lo <= hi, got %s and %s\n", a, b);
        std::exit(1);
    }
    if (lo == hi) return lo;
    return std::uniform_real_distribution<double>(lo, hi)(q_random_engine());
}

// Normal float. sd == 0 returns mean. A negative or non-finite sd is an error.
inline double q_random_normal(double mean, double sd) {
    if (!std::isfinite(mean) || !std::isfinite(sd) || sd < 0) {
        char a[32], b[32];
        q_format_float(mean, a, sizeof(a));
        q_format_float(sd, b, sizeof(b));
        q_runtime_reportf("runtime error: random::normal() needs a finite mean and sd >= 0, got %s and %s\n", a, b);
        std::exit(1);
    }
    if (sd == 0) return mean;
    return std::normal_distribution<double>(mean, sd)(q_random_engine());
}

// Uniform int in [lo, hi], both ends included.
inline int64_t q_random_randint(int64_t lo, int64_t hi) {
    if (lo > hi) {
        q_runtime_reportf("runtime error: random.randint(lo, hi) needs lo <= hi, got %lld and %lld\n",
                          static_cast<long long>(lo), static_cast<long long>(hi));
        std::exit(1);
    }
    return std::uniform_int_distribution<int64_t>(lo, hi)(q_random_engine());
}

inline void q_random_check_count(int64_t n, const char* fn) {
    if (n < 0) {
        q_runtime_reportf("runtime error: %s() count must be >= 0, got %lld\n", fn, static_cast<long long>(n));
        std::exit(1);
    }
    if (static_cast<unsigned long long>(n) > static_cast<unsigned long long>(SIZE_MAX / 8)) {
        q_runtime_reportf("runtime error: %s() count is too large\n", fn);
        std::exit(1);
    }
}

// A uniformly chosen element of a non-empty list.
inline QValue q_random_choice(QList* list) {
    if (!list || list->empty()) {
        q_runtime_reportf("runtime error: random.choice() on an empty list\n");
        std::exit(1);
    }
    std::uniform_int_distribution<size_t> pick(0, list->size() - 1);
    return (*list)[pick(q_random_engine())];
}

// Shuffles a list in place and returns it.
inline QList* q_random_shuffle(QList* list) {
    if (list) {
        std::shuffle(list->begin(), list->end(), q_random_engine());
    }
    return list;
}

// k elements of xs, without replacement, as a new list. xs is not modified.
// k == 0 returns an empty list of the same element type.
inline QList* q_random_sample(QList* xs, int64_t k) {
    if (xs == nullptr) {
        q_random_fail("random::sample() expects a list");
    }
    const int64_t n = static_cast<int64_t>(xs->size());
    if (k < 0 || k > n) {
        q_runtime_reportf("runtime error: random::sample() needs 0 <= k <= len(xs), got k=%lld and len=%lld\n",
                          static_cast<long long>(k), static_cast<long long>(n));
        std::exit(1);
    }
    const int cap = k > 0 && k <= 0x7fffffffLL ? static_cast<int>(k) : 0;
    QValue outv = qv_list(cap, xs->elem);
    QList* out = outv.data.list_val;
    if (k == 0) return out;
    std::vector<int64_t> idx(static_cast<size_t>(n));
    std::iota(idx.begin(), idx.end(), 0);
    for (int64_t i = 0; i < k; i++) {
        std::uniform_int_distribution<int64_t> pick(i, n - 1);
        const int64_t j = pick(q_random_engine());
        std::swap(idx[static_cast<size_t>(i)], idx[static_cast<size_t>(j)]);
        out->push_back((*xs)[static_cast<size_t>(idx[static_cast<size_t>(i)])]);
    }
    return out;
}

inline double q_random_weight_at(QList* weights, size_t i) {
    const QValue v = (*weights)[i];
    if (v.type == QValue::VAL_INT) return static_cast<double>(v.data.int_val);
    if (v.type == QValue::VAL_FLOAT) return v.data.float_val;
    q_runtime_reportf("runtime error: random::weighted() weight %zu must be a number, got %s\n", i,
                      q_type_name(v.type));
    std::exit(1);
}

// One element of xs, chosen with probability proportional to weights.
// A negative, non-finite or all-zero weight is an error. Lengths must match.
inline QValue q_random_weighted(QList* xs, QList* weights) {
    if (xs == nullptr || weights == nullptr) {
        q_random_fail("random::weighted() expects two lists");
    }
    if (xs->size() != weights->size()) {
        q_runtime_reportf("runtime error: random::weighted() needs one weight per element, got %zu elements and %zu weights\n",
                          xs->size(), weights->size());
        std::exit(1);
    }
    if (xs->empty()) {
        q_random_fail("random::weighted() on an empty list");
    }
    std::vector<double> w(xs->size());
    double sum = 0;
    for (size_t i = 0; i < xs->size(); i++) {
        const double x = q_random_weight_at(weights, i);
        if (!std::isfinite(x) || x < 0) {
            char buf[32];
            q_format_float(x, buf, sizeof(buf));
            q_runtime_reportf("runtime error: random::weighted() weight %zu must be a finite number >= 0, got %s\n", i, buf);
            std::exit(1);
        }
        w[i] = x;
        sum += x;
    }
    if (!(sum > 0.0) || !std::isfinite(sum)) {
        q_random_fail("random::weighted() weights must sum to a positive number");
    }
    double pick = std::uniform_real_distribution<double>(0.0, sum)(q_random_engine());
    double acc = 0;
    for (size_t i = 0; i < w.size(); i++) {
        acc += w[i];
        if (pick < acc) return (*xs)[i];
    }
    return xs->back();
}

inline QVector* q_random_uniform_vec(int64_t n) {
    q_random_check_count(n, "random::uniform_vec");
    QVector* v = q_vec_alloc(QVector::Type::F64, static_cast<size_t>(n));
    if (n == 0) return v;
    double* d = q_vec_f64_data_mut(*v);
    std::uniform_real_distribution<double> dist(0.0, 1.0);
    for (int64_t i = 0; i < n; i++) d[i] = dist(q_random_engine());
    return v;
}

inline QVector* q_random_normal_vec(int64_t n, double mean, double sd) {
    q_random_check_count(n, "random::normal_vec");
    if (!std::isfinite(mean) || !std::isfinite(sd) || sd < 0) {
        char a[32], b[32];
        q_format_float(mean, a, sizeof(a));
        q_format_float(sd, b, sizeof(b));
        q_runtime_reportf("runtime error: random::normal_vec() needs a finite mean and sd >= 0, got %s and %s\n", a, b);
        std::exit(1);
    }
    QVector* v = q_vec_alloc(QVector::Type::F64, static_cast<size_t>(n));
    if (n == 0) return v;
    double* d = q_vec_f64_data_mut(*v);
    if (sd == 0) {
        for (int64_t i = 0; i < n; i++) d[i] = mean;
        return v;
    }
    std::normal_distribution<double> dist(mean, sd);
    for (int64_t i = 0; i < n; i++) d[i] = dist(q_random_engine());
    return v;
}

inline QVector* q_random_randint_vec(int64_t n, int64_t lo, int64_t hi) {
    q_random_check_count(n, "random::randint_vec");
    if (lo > hi) {
        q_runtime_reportf("runtime error: random::randint_vec() needs lo <= hi, got %lld and %lld\n",
                          static_cast<long long>(lo), static_cast<long long>(hi));
        std::exit(1);
    }
    QVector* v = q_vec_alloc(QVector::Type::I64, static_cast<size_t>(n));
    if (n == 0) return v;
    int64_t* d = q_vec_i64_data_mut(*v);
    std::uniform_int_distribution<int64_t> dist(lo, hi);
    for (int64_t i = 0; i < n; i++) d[i] = dist(q_random_engine());
    return v;
}

#endif // QUARK_STDLIB_RANDOM_HPP
