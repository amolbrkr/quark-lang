// stdlib/random.hpp — native implementations for the std/random module (random.qrk).
#ifndef QUARK_STDLIB_RANDOM_HPP
#define QUARK_STDLIB_RANDOM_HPP

#include <algorithm>
#include <cstdint>
#include <random>

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

// Uniform float in [0, 1).
inline double q_random_uniform() {
    return std::uniform_real_distribution<double>(0.0, 1.0)(q_random_engine());
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

#endif // QUARK_STDLIB_RANDOM_HPP
