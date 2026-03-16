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

#ifdef QUARK_USE_GC
    #include <gc.h>
    #include <gc/gc_allocator.h>

    // Initialize GC (call once at program start)
    inline void q_gc_init() {
        GC_INIT();
    }

    // GC allocation macros (expand to Boehm GC calls)
    #define q_malloc(n)         GC_MALLOC(n)
    #define q_malloc_atomic(n)  GC_MALLOC_ATOMIC(n)
    #define q_realloc(p, n)     GC_REALLOC(p, n)
    #define q_free(p)           GC_FREE(p)
    #define q_strdup(s)         GC_STRDUP(s)

    // For future tensor/array data (no pointer scanning needed)
    #define q_malloc_tensor(n)  GC_MALLOC_ATOMIC(n)

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
    #define q_realloc(p, n)     realloc(p, n)
    #define q_free(p)           free(p)

    inline char* q_strdup(const char* s) {
        return strdup(s);
    }

    #define q_malloc_tensor(n)  malloc(n)

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
        std::fprintf(stderr, "runtime error: allocation failed\n");
        std::exit(1);
    }
    return ::new(mem) T(std::forward<Args>(args)...);
}

#endif // QUARK_CORE_GC_HPP
