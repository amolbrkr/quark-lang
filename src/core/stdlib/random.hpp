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
//
// In a verb (specs/tables_v0_3_spec.md §5), a draw doesn't take numbers from
// this generator: each draw function takes the row's stream (q_row_stream,
// core/rowrand.hpp) and turns its bits into a value with the q_row_*
// algorithms below, which are the runtime's own (not std:: distributions,
// whose algorithms differ between C++ libraries), so a seeded program gets
// the same numbers for every thread count, and the same uniform floats, ints
// and choices on every platform.
inline std::mt19937_64& q_random_engine() {
    static std::mt19937_64 engine{std::random_device{}()};
    return engine;
}

// seed(n) seeds the sequence and the draws in verbs, whose loop count it
// resets.
inline QValue q_random_seed(int64_t seed) {
    q_random_engine().seed(static_cast<uint64_t>(seed));
    q_rand_set_seed(static_cast<uint64_t>(seed));
    return qv_null();
}

// A uniform float in [lo, hi), lo < hi, for a draw in a verb: lo + u * (hi -
// lo), drawn again when it rounds up to hi; when hi - lo overflows, halved
// bounds doubled, as q_random_uniform_range does.
inline double q_row_uniform(QRowStream& s, double lo, double hi) {
    if (std::isfinite(hi - lo)) {
        for (;;) {
            const double x = lo + q_row_unit(s) * (hi - lo);
            if (x < hi) return x;
        }
    }
    const double a = lo / 2, b = hi / 2;
    for (;;) {
        const double x = 2.0 * (a + q_row_unit(s) * (b - a));
        if (x < hi) return x;
    }
}

// A normal float for a draw in a verb, by Box-Muller from two uniforms (the
// platform's log and cos, so the last bit can differ between platforms).
inline double q_row_normal(QRowStream& s, double mean, double sd) {
    const double u1 = 1.0 - q_row_unit(s); // (0, 1]
    const double u2 = q_row_unit(s);
    const double z = std::sqrt(-2.0 * std::log(u1)) * std::cos(6.283185307179586 * u2);
    return mean + sd * z;
}

// A uniform int in [lo, hi], lo <= hi, for a draw in a verb.
inline int64_t q_row_randint(QRowStream& s, int64_t lo, int64_t hi) {
    const uint64_t range = static_cast<uint64_t>(hi) - static_cast<uint64_t>(lo) + 1; // 0: all of int64
    return static_cast<int64_t>(static_cast<uint64_t>(lo) + q_row_below(s, range));
}

[[noreturn]] inline void q_random_fail(const char* msg) {
    q_runtime_reportf("runtime error: %s\n", msg);
    q_exit(1);
}

// A draw of dist, which is below hi. The distribution computes
// lo + u * (hi - lo), which rounds up to hi when u is close to 1 or the range
// is a few floats wide (and some standard libraries' u can be 1), so a draw
// that reaches hi is drawn again. At least half the draws are below hi, and
// a seeded program gets the same values as before wherever no draw reached hi.
inline double q_random_below(std::uniform_real_distribution<double>& dist, double scale, double hi) {
    for (;;) {
        const double x = scale * dist(q_random_engine());
        if (x < hi) return x;
    }
}

// Uniform float in [lo, hi). lo == hi returns lo. lo > hi, or a non-finite
// bound, is an error. uniform() is this with lo = 0 and hi = 1.
inline double q_random_uniform_range(double lo, double hi) {
    QRowStream rs;
    const bool verb = q_row_stream(&rs);
    if (!std::isfinite(lo) || !std::isfinite(hi) || lo > hi) {
        char a[32], b[32];
        q_format_float(lo, a, sizeof(a));
        q_format_float(hi, b, sizeof(b));
        q_runtime_reportf("runtime error: random::uniform() needs a finite lo <= hi, got %s and %s\n", a, b);
        q_exit(1);
    }
    if (lo == hi) return lo;
    if (verb) return q_row_uniform(rs, lo, hi);
    if (std::isfinite(hi - lo)) {
        std::uniform_real_distribution<double> dist(lo, hi);
        return q_random_below(dist, 1.0, hi);
    }
    // hi - lo overflows (uniform(-1e308, 1e308)): draw in [lo/2, hi/2) and
    // double it. Both bounds are then far from the subnormals, so halving and
    // doubling are exact.
    std::uniform_real_distribution<double> dist(lo / 2, hi / 2);
    return q_random_below(dist, 2.0, hi);
}

[[noreturn]] inline void q_random_normal_overflow(const char* fn, double mean, double sd) {
    char a[32], b[32];
    q_format_float(mean, a, sizeof(a));
    q_format_float(sd, b, sizeof(b));
    q_runtime_reportf("runtime error: %s() drew a value too large for a float, with mean %s and sd %s\n", fn, a, b);
    q_exit(1);
}

// Normal float. sd == 0 returns mean. A negative or non-finite sd is an error,
// and so is a draw too large for a float (mean + sd * z overflows when mean
// and sd are near the float limit), which would otherwise be inf.
inline double q_random_normal(double mean, double sd) {
    QRowStream rs;
    const bool verb = q_row_stream(&rs);
    if (!std::isfinite(mean) || !std::isfinite(sd) || sd < 0) {
        char a[32], b[32];
        q_format_float(mean, a, sizeof(a));
        q_format_float(sd, b, sizeof(b));
        q_runtime_reportf("runtime error: random::normal() needs a finite mean and sd >= 0, got %s and %s\n", a, b);
        q_exit(1);
    }
    if (sd == 0) return mean;
    const double x = verb ? q_row_normal(rs, mean, sd) : std::normal_distribution<double>(mean, sd)(q_random_engine());
    if (!std::isfinite(x)) q_random_normal_overflow("random::normal", mean, sd);
    return x;
}

// Uniform int in [lo, hi], both ends included.
inline int64_t q_random_randint(int64_t lo, int64_t hi) {
    QRowStream rs;
    const bool verb = q_row_stream(&rs);
    if (lo > hi) {
        q_runtime_reportf("runtime error: random.randint(lo, hi) needs lo <= hi, got %lld and %lld\n",
                          static_cast<long long>(lo), static_cast<long long>(hi));
        q_exit(1);
    }
    if (verb) return q_row_randint(rs, lo, hi);
    return std::uniform_int_distribution<int64_t>(lo, hi)(q_random_engine());
}

inline void q_random_check_count(int64_t n, const char* fn) {
    if (n < 0) {
        q_runtime_reportf("runtime error: %s() count must be >= 0, got %lld\n", fn, static_cast<long long>(n));
        q_exit(1);
    }
    if (static_cast<unsigned long long>(n) > static_cast<unsigned long long>(SIZE_MAX / 8)) {
        q_runtime_reportf("runtime error: %s() count is too large\n", fn);
        q_exit(1);
    }
}

// A uniformly chosen element of a non-empty list.
inline QValue q_random_choice(QList* list) {
    QRowStream rs;
    const bool verb = q_row_stream(&rs);
    if (!list || list->empty()) {
        q_runtime_reportf("runtime error: random.choice() on an empty list\n");
        q_exit(1);
    }
    if (verb) return (*list)[static_cast<size_t>(q_row_below(rs, list->size()))];
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
    QRowStream rs;
    const bool verb = q_row_stream(&rs);
    if (xs == nullptr) {
        q_random_fail("random::sample() expects a list");
    }
    const int64_t n = static_cast<int64_t>(xs->size());
    if (k < 0 || k > n) {
        q_runtime_reportf("runtime error: random::sample() needs 0 <= k <= len(xs), got k=%lld and len=%lld\n",
                          static_cast<long long>(k), static_cast<long long>(n));
        q_exit(1);
    }
    const int cap = k > 0 && k <= 0x7fffffffLL ? static_cast<int>(k) : 0;
    QValue outv = qv_list(cap, xs->elem);
    QList* out = outv.data.list_val;
    if (k == 0) return out;
    std::vector<int64_t> idx(static_cast<size_t>(n));
    std::iota(idx.begin(), idx.end(), 0);
    for (int64_t i = 0; i < k; i++) {
        int64_t j = 0;
        if (verb) {
            j = i + static_cast<int64_t>(q_row_below(rs, static_cast<uint64_t>(n - i)));
        } else {
            std::uniform_int_distribution<int64_t> pick(i, n - 1);
            j = pick(q_random_engine());
        }
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
    q_exit(1);
}

// One element of xs, chosen with probability proportional to weights.
// A negative, non-finite or all-zero weight is an error. Lengths must match.
inline QValue q_random_weighted(QList* xs, QList* weights) {
    QRowStream rs;
    const bool verb = q_row_stream(&rs);
    if (xs == nullptr || weights == nullptr) {
        q_random_fail("random::weighted() expects two lists");
    }
    if (xs->size() != weights->size()) {
        q_runtime_reportf("runtime error: random::weighted() needs one weight per element, got %zu elements and %zu weights\n",
                          xs->size(), weights->size());
        q_exit(1);
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
            q_exit(1);
        }
        w[i] = x;
        sum += x;
    }
    if (std::isinf(sum)) {
        // Every weight is finite, so only the sum overflowed
        // (list [1e308, 1e308]). Dividing by the largest weight keeps the
        // proportions and makes the sum at most the number of weights.
        const double top = *std::max_element(w.begin(), w.end());
        sum = 0;
        for (double& x : w) {
            x /= top;
            sum += x;
        }
    }
    if (!(sum > 0.0)) {
        q_random_fail("random::weighted() weights must sum to a positive number");
    }
    // pick < sum, and acc reaches sum by the same additions, so some element
    // whose weight is positive is always chosen.
    double pick = 0;
    if (verb) {
        pick = q_row_uniform(rs, 0.0, sum);
    } else {
        std::uniform_real_distribution<double> dist(0.0, sum);
        pick = q_random_below(dist, 1.0, sum);
    }
    double acc = 0;
    size_t last = 0;
    for (size_t i = 0; i < w.size(); i++) {
        if (w[i] == 0) continue;
        acc += w[i];
        last = i;
        if (pick < acc) return (*xs)[i];
    }
    return (*xs)[last];
}

inline QVector* q_random_uniform_vec(int64_t n) {
    QRowStream rs;
    const bool verb = q_row_stream(&rs);
    q_random_check_count(n, "random::uniform_vec");
    QVector* v = q_vec_alloc(QVector::Type::F64, static_cast<size_t>(n));
    if (n == 0) return v;
    double* d = q_vec_f64_data_mut(*v);
    if (verb) {
        for (int64_t i = 0; i < n; i++) d[i] = q_row_unit(rs);
        return v;
    }
    std::uniform_real_distribution<double> dist(0.0, 1.0);
    for (int64_t i = 0; i < n; i++) d[i] = q_random_below(dist, 1.0, 1.0);
    return v;
}

inline QVector* q_random_normal_vec(int64_t n, double mean, double sd) {
    QRowStream rs;
    const bool verb = q_row_stream(&rs);
    q_random_check_count(n, "random::normal_vec");
    if (!std::isfinite(mean) || !std::isfinite(sd) || sd < 0) {
        char a[32], b[32];
        q_format_float(mean, a, sizeof(a));
        q_format_float(sd, b, sizeof(b));
        q_runtime_reportf("runtime error: random::normal_vec() needs a finite mean and sd >= 0, got %s and %s\n", a, b);
        q_exit(1);
    }
    QVector* v = q_vec_alloc(QVector::Type::F64, static_cast<size_t>(n));
    if (n == 0) return v;
    double* d = q_vec_f64_data_mut(*v);
    if (sd == 0) {
        for (int64_t i = 0; i < n; i++) d[i] = mean;
        return v;
    }
    std::normal_distribution<double> dist(mean, sd);
    for (int64_t i = 0; i < n; i++) {
        const double x = verb ? q_row_normal(rs, mean, sd) : dist(q_random_engine());
        if (!std::isfinite(x)) q_random_normal_overflow("random::normal_vec", mean, sd);
        d[i] = x;
    }
    return v;
}

inline QVector* q_random_randint_vec(int64_t n, int64_t lo, int64_t hi) {
    QRowStream rs;
    const bool verb = q_row_stream(&rs);
    q_random_check_count(n, "random::randint_vec");
    if (lo > hi) {
        q_runtime_reportf("runtime error: random::randint_vec() needs lo <= hi, got %lld and %lld\n",
                          static_cast<long long>(lo), static_cast<long long>(hi));
        q_exit(1);
    }
    QVector* v = q_vec_alloc(QVector::Type::I64, static_cast<size_t>(n));
    if (n == 0) return v;
    int64_t* d = q_vec_i64_data_mut(*v);
    if (verb) {
        for (int64_t i = 0; i < n; i++) d[i] = q_row_randint(rs, lo, hi);
        return v;
    }
    std::uniform_int_distribution<int64_t> dist(lo, hi);
    for (int64_t i = 0; i < n; i++) d[i] = dist(q_random_engine());
    return v;
}

#endif // QUARK_STDLIB_RANDOM_HPP
