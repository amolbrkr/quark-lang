// quark/core/rowrand.hpp - random draws in a verb (specs/tables_v0_3_spec.md
// §5, D45).
//
// Outside verbs, std/random draws from one sequence. In a verb, a draw is
// computed instead: from the seed, the row loop, the row and the draw's
// position in the row, with the counter-based generator Philox4x32-10
// (Salmon et al., "Parallel random numbers: as easy as 1, 2, 3", SC 2011).
// The numbers then don't depend on which thread runs a row, or in which
// order rows run.
//
// Generated code numbers each row loop whose expressions draw
// (q_rand_loop_key, in program order on the thread that runs the query),
// and sets the row's context (q_row_ctx_set) before each row. A draw
// function takes a stream for its draw (q_row_stream) when a context is
// active, and the sequential generator otherwise.
#ifndef QUARK_CORE_ROWRAND_HPP
#define QUARK_CORE_ROWRAND_HPP

#include <cstdint>
#include <random>

// ------------------------------------------------------------
// Philox4x32-10

struct QPhilox4x32 {
    uint32_t v[4];
};

inline QPhilox4x32 q_philox4x32_10(uint32_t c0, uint32_t c1, uint32_t c2, uint32_t c3, uint32_t k0, uint32_t k1) {
    const uint32_t M0 = 0xD2511F53u, M1 = 0xCD9E8D57u;
    const uint32_t W0 = 0x9E3779B9u, W1 = 0xBB67AE85u;
    for (int r = 0; r < 10; r++) {
        if (r > 0) {
            k0 += W0;
            k1 += W1;
        }
        const uint64_t p0 = static_cast<uint64_t>(M0) * c0;
        const uint64_t p1 = static_cast<uint64_t>(M1) * c2;
        const uint32_t n0 = static_cast<uint32_t>(p1 >> 32) ^ c1 ^ k0;
        const uint32_t n1 = static_cast<uint32_t>(p1);
        const uint32_t n2 = static_cast<uint32_t>(p0 >> 32) ^ c3 ^ k1;
        const uint32_t n3 = static_cast<uint32_t>(p0);
        c0 = n0;
        c1 = n1;
        c2 = n2;
        c3 = n3;
    }
    return QPhilox4x32{{c0, c1, c2, c3}};
}

// ------------------------------------------------------------
// The row context

// QRowCtx is the draw context of the row this thread is computing: the
// loop's key, the row (a group's index in a summarize), and how many draws
// the row has made. active is false outside verbs.
struct QRowCtx {
    uint64_t key = 0;
    uint64_t row = 0;
    uint32_t draw = 0;
    bool active = false;
};

inline thread_local QRowCtx q_row_ctx;

// q_rand_state is the program's verb seed and its count of drawing row
// loops. random::seed(n) sets the seed and resets the count; a program that
// never calls it takes a seed from the system at its first drawing loop.
struct QRandState {
    uint64_t seed = 0;
    uint64_t loops = 0;
    bool seeded = false;
};

inline QRandState q_rand_state;

inline void q_rand_set_seed(uint64_t seed) {
    q_rand_state.seed = seed;
    q_rand_state.loops = 0;
    q_rand_state.seeded = true;
}

// q_philox_key splits a 64-bit key for Philox.
inline QPhilox4x32 q_row_block(uint64_t key, uint64_t row, uint32_t draw, uint32_t block) {
    return q_philox4x32_10(static_cast<uint32_t>(row), static_cast<uint32_t>(row >> 32), draw, block,
                           static_cast<uint32_t>(key), static_cast<uint32_t>(key >> 32));
}

// q_rand_loop_key is the key of the next row loop that draws: from the seed
// and the loop's number in program order. A loop that starts while a row is
// being computed (a query in a function a verb calls) takes its key from
// that row, as a draw of it, so the numbering doesn't depend on threads.
inline uint64_t q_rand_loop_key() {
    QRowCtx& c = q_row_ctx;
    if (c.active) {
        const QPhilox4x32 b = q_row_block(c.key, c.row, c.draw++, 0);
        return static_cast<uint64_t>(b.v[0]) | (static_cast<uint64_t>(b.v[1]) << 32);
    }
    if (!q_rand_state.seeded) {
        std::random_device rd;
        const uint64_t hi = rd(), lo = rd();
        q_rand_set_seed((hi << 32) ^ lo);
    }
    // Distinct loops of one seed get distinct keys (the multiplier is odd).
    return q_rand_state.seed + (q_rand_state.loops++) * 0x9E3779B97F4A7C15ull;
}

inline QRowCtx q_row_ctx_save() { return q_row_ctx; }
inline void q_row_ctx_restore(QRowCtx c) { q_row_ctx = c; }
inline void q_row_ctx_set(uint64_t key, long long row) {
    QRowCtx& c = q_row_ctx;
    c.key = key;
    c.row = static_cast<uint64_t>(row);
    c.draw = 0;
    c.active = true;
}

// q_row_stream_exhausted reports a draw in a verb that needs more than
// 2^33 random words (a vec of billions of draws in one row), rather than
// repeat its numbers.
[[noreturn]] inline void q_row_stream_exhausted();

// QRowStream is the bits of one draw in a verb: Philox blocks 0, 1, 2, ...
// of the counter (row, draw, block), two 64-bit words each.
struct QRowStream {
    uint64_t key = 0;
    uint64_t row = 0;
    uint32_t draw = 0;
    uint32_t block = 0;
    uint64_t buf[2] = {0, 0};
    int left = 0;

    uint64_t next() {
        if (left == 0) {
            if (block == 0xffffffffu) q_row_stream_exhausted();
            const QPhilox4x32 b = q_row_block(key, row, draw, block++);
            buf[0] = static_cast<uint64_t>(b.v[0]) | (static_cast<uint64_t>(b.v[1]) << 32);
            buf[1] = static_cast<uint64_t>(b.v[2]) | (static_cast<uint64_t>(b.v[3]) << 32);
            left = 2;
        }
        return buf[2 - left--];
    }
};

// q_row_stream starts the stream of the row's next draw and returns true in
// a verb; outside verbs it returns false.
inline bool q_row_stream(QRowStream* s) {
    QRowCtx& c = q_row_ctx;
    if (!c.active) return false;
    s->key = c.key;
    s->row = c.row;
    s->draw = c.draw++;
    s->block = 0;
    s->left = 0;
    return true;
}

// A uniform float in [0, 1), from the top 53 bits of a word.
inline double q_row_unit(QRowStream& s) { return static_cast<double>(s.next() >> 11) * 0x1.0p-53; }

// A uniform int in [0, range), by rejection, so no value is more likely; a
// range of 0 stands for 2^64.
inline uint64_t q_row_below(QRowStream& s, uint64_t range) {
    if (range == 0) return s.next();
    const uint64_t threshold = (0 - range) % range;
    for (;;) {
        const uint64_t x = s.next();
        if (x >= threshold) return x % range;
    }
}

#endif // QUARK_CORE_ROWRAND_HPP
