// gcstats.hpp — memory introspection for the GC regression tests
// (integration_gc_test.go). Not part of the stdlib.
#ifndef QUARK_TESTFILES_GCSTATS_HPP
#define QUARK_TESTFILES_GCSTATS_HPP

#include <gc.h>
#ifndef _WIN32
#include <sys/resource.h>
#endif

// Bytes in use on the GC heap. Call gc_collect() first for a meaningful number.
inline int64_t q_test_gc_live() {
    return static_cast<int64_t>(GC_get_heap_size() - GC_get_free_bytes());
}

inline QValue q_test_gc_collect() {
    GC_gcollect();
    return qv_null();
}

// Peak resident set size in bytes, or -1 where it isn't available. Memory the
// GC doesn't manage (malloc'd by the C++ library) shows up here, not in
// q_test_gc_live.
inline int64_t q_test_peak_rss() {
#ifdef _WIN32
    return -1;
#else
    struct rusage usage;
    if (getrusage(RUSAGE_SELF, &usage) != 0) {
        return -1;
    }
#ifdef __APPLE__
    return static_cast<int64_t>(usage.ru_maxrss);          // bytes
#else
    return static_cast<int64_t>(usage.ru_maxrss) * 1024;   // KiB on Linux
#endif
#endif
}

#endif // QUARK_TESTFILES_GCSTATS_HPP
