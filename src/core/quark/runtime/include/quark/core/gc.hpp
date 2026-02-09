// quark/core/gc.hpp - Boehm GC integration wrapper
#ifndef QUARK_CORE_GC_HPP
#define QUARK_CORE_GC_HPP

// Boehm GC configuration
// Define QUARK_USE_GC to enable garbage collection
// Otherwise falls back to manual memory management (leaking for now)

#ifdef QUARK_USE_GC
    #include <gc.h>

    // Initialize GC (call once at program start)
    inline void q_gc_init() {
        GC_INIT();
    }

    // GC allocation macros (expand to Boehm GC calls)
    #define q_malloc(n)         GC_MALLOC(n)
    #define q_malloc_atomic(n)  GC_MALLOC_ATOMIC(n)
    #define q_realloc(p, n)     GC_REALLOC(p, n)
    #define q_free(p)           /* GC handles it */
    #define q_strdup(s)         GC_STRDUP(s)

    // For future tensor/array data (no pointer scanning needed)
    #define q_malloc_tensor(n)  GC_MALLOC_ATOMIC(n)

#else
    // No GC - use standard malloc (will leak for now)
    #include <cstdlib>
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
#endif

#endif // QUARK_CORE_GC_HPP
