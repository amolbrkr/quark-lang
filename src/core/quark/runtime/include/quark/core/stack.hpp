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
    std::exit(1);
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

// One jump buffer for the thread that armed the guard. A signal on another
// thread (a library's worker) is not this overflow and is re-raised.
inline sigjmp_buf& q_stack_jmp() {
    static sigjmp_buf buf;
    return buf;
}
inline volatile sig_atomic_t& q_stack_ready() {
    static volatile sig_atomic_t ready = 0;
    return ready;
}
inline pthread_t& q_stack_thread() {
    static pthread_t thread;
    return thread;
}
inline uintptr_t& q_stack_lo() {
    static uintptr_t lo = 0;
    return lo;
}
inline uintptr_t& q_stack_hi() {
    static uintptr_t hi = 0;
    return hi;
}

inline void q_stack_read_bounds() {
    uintptr_t marker = 0;
#if defined(__APPLE__)
    const uintptr_t top = reinterpret_cast<uintptr_t>(pthread_get_stackaddr_np(pthread_self()));
    const uintptr_t size = pthread_get_stacksize_np(pthread_self());
    q_stack_hi() = top;
    q_stack_lo() = top - size;
    (void)marker;
#else
    // The mapping that contains a local is this thread's stack. The guard
    // page sits just below it, so the fault check allows a page of slack.
    FILE* maps = std::fopen("/proc/self/maps", "r");
    if (!maps) return;
    char line[512];
    const uintptr_t here = reinterpret_cast<uintptr_t>(&marker);
    while (std::fgets(line, sizeof(line), maps)) {
        unsigned long start = 0, end = 0;
        if (std::sscanf(line, "%lx-%lx", &start, &end) != 2) continue;
        if (here >= start && here < end) {
            q_stack_lo() = start;
            q_stack_hi() = end;
            break;
        }
    }
    std::fclose(maps);
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
    if (!q_stack_ready() || !pthread_equal(pthread_self(), q_stack_thread()) || info == nullptr) {
        q_stack_reraise(sig);
        return;
    }
    const uintptr_t addr = reinterpret_cast<uintptr_t>(info->si_addr);
    const uintptr_t lo = q_stack_lo();
    const uintptr_t hi = q_stack_hi();
    // The guard page is the page below the stack mapping, not inside it.
    const bool on_stack = lo != 0 && addr + 65536 >= lo && addr < hi;
    if (!on_stack) {
        q_stack_reraise(sig);
        return;
    }
    q_stack_ready() = 0;
    siglongjmp(q_stack_jmp(), 1);
}

inline void q_stack_install() {
    static bool installed = false;
    if (installed) return;
    installed = true;
    q_stack_read_bounds();
    // Static, not on the main stack: the handler has to run after that
    // stack is exhausted.
    alignas(16) static unsigned char alt[65536];
    stack_t ss;
    ss.ss_sp = alt;
    ss.ss_size = sizeof(alt);
    ss.ss_flags = 0;
    sigaltstack(&ss, nullptr);
    struct sigaction sa;
    sa.sa_sigaction = q_stack_on_signal;
    sigemptyset(&sa.sa_mask);
    sa.sa_flags = SA_SIGINFO | SA_ONSTACK;
    sigaction(SIGSEGV, &sa, nullptr);
#if defined(SIGBUS)
    sigaction(SIGBUS, &sa, nullptr);
#endif
}

inline void q_protect_and_run(void (*fn)()) {
    q_stack_install();
    q_stack_thread() = pthread_self();
    if (sigsetjmp(q_stack_jmp(), 1) != 0) {
        q_stack_overflow_exit();
    }
    q_stack_ready() = 1;
    fn();
    q_stack_ready() = 0;
}

#endif // _WIN32

#endif // QUARK_CORE_STACK_HPP
