// quark/core/stack.hpp - Turn a stack overflow into a runtime error.
//
// Generated main runs the program through q_protect_and_run. The guard is
// the operating system's stack probe, so a call costs nothing until the
// stack actually overflows. The handler flushes stdout (a crash used to
// discard output still sitting in the buffer) and reports the current
// source location. It then exits; it does not resume the program.
#ifndef QUARK_CORE_STACK_HPP
#define QUARK_CORE_STACK_HPP

#include "diagnostics.hpp"
#include <cstdio>
#include <cstdlib>

inline void q_stack_overflow_exit() {
    std::fflush(stdout);
    q_runtime_reportf("runtime error: stack overflow\n");
    q_exit(1);
}

#if defined(_WIN32)

// Structured exception handling, so the handler runs after the stack
// pointer is back in q_protect_and_run. No windows.h: the exception code
// and the filter results are fixed, and _resetstkoflw is the CRT.
extern "C" int _resetstkoflw(void);

inline void q_protect_and_run(void (*fn)()) {
    __try {
        fn();
    } __except (_exception_code() == 0xC00000FDL ? 1 : 0) {
        _resetstkoflw();
        q_stack_overflow_exit();
    }
}

#else

#include <csignal>
#include <csetjmp>
#include <cstdint>
#include <pthread.h>

// QStackGuard is one thread's guard: where its stack is, and where to jump
// when it overflows. Each thread that runs Quark code has its own: the main
// thread (q_protect_and_run) and every worker of the thread pool, which arms
// it around each block it runs (core/par.hpp). A signal on a thread without
// an armed guard (a library's worker) is not this overflow and is re-raised.
struct QStackGuard {
    sigjmp_buf jb;
    uintptr_t lo = 0;
    uintptr_t hi = 0;
    volatile sig_atomic_t ready = 0;
};

inline thread_local QStackGuard q_stack_guard;

// q_stack_read_bounds records the calling thread's stack in its guard.
inline void q_stack_read_bounds() {
    QStackGuard& g = q_stack_guard;
#if defined(__APPLE__)
    const uintptr_t top = reinterpret_cast<uintptr_t>(pthread_get_stackaddr_np(pthread_self()));
    const uintptr_t size = pthread_get_stacksize_np(pthread_self());
    g.hi = top;
    g.lo = top - size;
#else
    // The whole stack the thread may use, not just the part it has used so
    // far (the main thread's [stack] mapping grows on demand, up to the
    // stack limit). For the main thread glibc derives it from the limit.
    pthread_attr_t attr;
    if (pthread_getattr_np(pthread_self(), &attr) != 0) return;
    void* addr = nullptr;
    size_t size = 0;
    if (pthread_attr_getstack(&attr, &addr, &size) == 0 && addr != nullptr && size > 0) {
        g.lo = reinterpret_cast<uintptr_t>(addr);
        g.hi = g.lo + size;
    }
    pthread_attr_destroy(&attr);
#endif
}

inline void q_stack_reraise(int sig) {
    struct sigaction dfl;
    dfl.sa_handler = SIG_DFL;
    sigemptyset(&dfl.sa_mask);
    dfl.sa_flags = 0;
    sigaction(sig, &dfl, nullptr);
    raise(sig);
}

inline void q_stack_on_signal(int sig, siginfo_t* info, void*) {
    QStackGuard& g = q_stack_guard;
    if (!g.ready || info == nullptr) {
        q_stack_reraise(sig);
        return;
    }
    const uintptr_t addr = reinterpret_cast<uintptr_t>(info->si_addr);
    // The guard page is the page below the stack mapping, not inside it.
    const bool on_stack = g.lo != 0 && addr + 65536 >= g.lo && addr < g.hi;
    if (!on_stack) {
        q_stack_reraise(sig);
        return;
    }
    g.ready = 0;
    siglongjmp(g.jb, 1);
}

// q_stack_alt_stack gives the calling thread an alternate signal stack, so
// the handler can run after the thread's own stack is exhausted. It is
// ordinary memory, never freed: a thread that runs Quark code lives as long
// as the program.
inline void q_stack_alt_stack(void* mem, size_t size) {
    stack_t ss;
    ss.ss_sp = mem;
    ss.ss_size = size;
    ss.ss_flags = 0;
    sigaltstack(&ss, nullptr);
}

inline void q_stack_install() {
    static bool installed = false;
    if (installed) return;
    installed = true;
    q_stack_read_bounds();
    // Static, not on the main stack: the handler has to run after that
    // stack is exhausted.
    alignas(16) static unsigned char alt[65536];
    q_stack_alt_stack(alt, sizeof(alt));
    struct sigaction sa;
    sa.sa_sigaction = q_stack_on_signal;
    sigemptyset(&sa.sa_mask);
    sa.sa_flags = SA_SIGINFO | SA_ONSTACK;
    sigaction(SIGSEGV, &sa, nullptr);
#if defined(SIGBUS)
    sigaction(SIGBUS, &sa, nullptr);
#endif
}

// q_stack_thread_init sets up a worker thread of the pool: its stack bounds
// and its alternate signal stack. The handler is the process's, installed
// by the main thread.
inline void q_stack_thread_init() {
    q_stack_read_bounds();
    const size_t size = 65536;
    void* alt = std::malloc(size);
    if (alt != nullptr) q_stack_alt_stack(alt, size);
}

inline void q_protect_and_run(void (*fn)()) {
    q_stack_install();
    QStackGuard& g = q_stack_guard;
    if (sigsetjmp(g.jb, 1) != 0) {
        q_stack_overflow_exit();
    }
    g.ready = 1;
    fn();
    g.ready = 0;
}

#endif // _WIN32

#endif // QUARK_CORE_STACK_HPP
