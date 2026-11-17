// quark/core/gc.hpp - Boehm GC integration wrapper
//
// ALL runtime heap objects must be allocated through the helpers below
// so that the garbage collector can track them:
//
//   q_malloc / q_malloc_atomic / q_strdup  — raw C-style allocations
//   q_new<T>(args...)                      — C++ objects (placement new on GC heap)
//   q_allocator<T>                         — STL allocator for containers
//
// Using bare `new` or std::allocator bypasses the GC and causes leaks
// or premature collection.  Do not use them for runtime objects.

#ifndef QUARK_CORE_GC_HPP
#define QUARK_CORE_GC_HPP

#include <cstdio>
#include <cstdlib>
#include <new>        // placement new
#include <memory>     // std::allocator (non-GC fallback)
#include <utility>    // std::forward
#include "diagnostics.hpp"
#include "threads.hpp"

#ifdef QUARK_USE_GC
    // The thread pool's workers register with the collector themselves
    // (core/par.hpp), so gc.h's redirection of thread creation is off.
    #ifndef GC_THREADS
        #define GC_THREADS
    #endif
    #ifndef GC_NO_THREAD_REDIRECTS
        #define GC_NO_THREAD_REDIRECTS
    #endif
    #include <gc.h>
    #include <gc/gc_allocator.h>

    // Initialize GC (call once at program start). The collector marks on
    // one thread (no marker threads), whatever the program's thread count:
    // with the thread pool running, parallel marking was the slowdown. A
    // mutate building strings on 1M rows took 0.83 s on 6 threads with 6
    // markers and 0.78 s with 2, against 0.20 s with one and 0.49 s on one
    // thread (2026-11-17, a 6-CPU quota). GC_MARKERS still overrides it.
    inline void q_gc_init() {
        (void)q_par_threads();
        GC_set_markers_count(1);
        GC_INIT();
    }

    // GC allocation macros (expand to Boehm GC calls)
    #define q_malloc(n)         GC_MALLOC(n)
    #define q_malloc_atomic(n)  GC_MALLOC_ATOMIC(n)
    #define q_free(p)           GC_FREE(p)
    #define q_strdup(s)         GC_STRDUP(s)

    // GC-aware STL allocator.  gc_allocator dispatches to GC_MALLOC for
    // pointer-containing types and GC_MALLOC_ATOMIC for pointer-free
    // primitives (double, int64_t, uint8_t, char, …).
    template<typename T>
    using q_allocator = gc_allocator<T>;

#else
    // No GC - use standard malloc (will leak for now)
    #include <cstring>

    inline void q_gc_init() {
        // No-op when GC disabled
    }

    #define q_malloc(n)         malloc(n)
    #define q_malloc_atomic(n)  malloc(n)
    #define q_free(p)           free(p)

    inline char* q_strdup(const char* s) {
        return strdup(s);
    }

    // No GC — fall back to the default STL allocator
    template<typename T>
    using q_allocator = std::allocator<T>;

#endif

// q_new<T>(args...): Construct a C++ object on GC-managed memory.
//
// The memory is allocated with q_malloc (pointer-scanning) so the GC
// will trace any embedded pointers (e.g. QValue members).
// ALL runtime container/struct heap allocations (QList, QDict, QVector, …)
// must go through q_new — never through bare `new`.
template<typename T, typename... Args>
inline T* q_new(Args&&... args) {
    void* mem = q_malloc(sizeof(T));
    if (!mem) {
        q_runtime_reportf("runtime error: allocation failed\n");
        q_exit(1);
    }
    return ::new(mem) T(std::forward<Args>(args)...);
}

#endif // QUARK_CORE_GC_HPP
