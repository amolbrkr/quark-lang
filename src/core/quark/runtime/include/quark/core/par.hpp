// quark/core/par.hpp - the thread pool (specs/tables_v0_3_spec.md §8)
//
// One pool runs a program's parallel loops (D52). A loop over n rows is
// split into blocks of q_par_block rows (D55). It runs on the calling thread,
// as one block, when n is under q_par_min_rows, when the program uses one
// thread, and when it starts inside a block (a query in a function a row
// calls): only the outermost loop is split. Otherwise the calling thread and
// the workers take blocks in increasing order until none is left.
//
// The same output for every thread count (D53, §8.4): a loop's body writes
// each block's part on its own (a filter's kept rows, a column's values at
// their rows), and the caller joins the parts in block order. A loop's
// Plan, made before the loop, says how many blocks it has, so the parts can
// be set up first.
//
// Errors (D56, §8.5): a thread running a block records an error instead of
// reporting it (q_error_capture, core/diagnostics.hpp), which ends the
// block. Blocks after the lowest failing one are not started; those before
// it finish, since one of them could fail at an earlier row. The caller
// then reports the lowest failing block's error, which is the error of the
// first failing row, and exits.
//
// A stack overflow in a block (each worker has its own guard, core/stack.hpp)
// is reported at once by the thread it happens on, which then ends the
// program: the thread may have overflowed inside the garbage collector,
// holding its lock, so it can't run on, and the other threads can't be
// waited for.
//
// Each block starts with the caller's source location and random-draw
// context (q_runtime_loc, q_row_ctx), as the rows would on one thread.
// Workers are registered with the garbage collector, so a row can allocate.
#ifndef QUARK_CORE_PAR_HPP
#define QUARK_CORE_PAR_HPP

#include "gc.hpp"
#include "diagnostics.hpp"
#include "rowrand.hpp"
#include "stack.hpp"
#include "threads.hpp"

#include <atomic>
#include <chrono>
#include <climits>
#include <condition_variable>
#include <cstdint>
#include <cstring>
#include <exception>
#include <mutex>
#include <thread>

#if !defined(_WIN32)
#include <pthread.h>
#include <sys/resource.h>
#else
#include <process.h>
extern "C" __declspec(dllimport) void __stdcall GetCurrentThreadStackLimits(unsigned long long*, unsigned long long*);
#endif

inline constexpr long long q_par_block = 65536;
inline constexpr long long q_par_min_rows = 131072;

namespace quark {
namespace par {

// Plan is a loop's split: n rows in nblocks blocks (1: on the calling thread).
struct Plan {
    long long n = 0;
    long long nblocks = 1;
};

struct Task;

// The thread's state while it runs a block: depth > 0 inside a block (so a
// loop started there runs on this thread), and the loop's task.
inline thread_local int depth = 0;
inline thread_local Task* current = nullptr;

// Task is one parallel loop.
struct Task {
    void (*run)(void* fn, long long lo, long long hi, long long blk) = nullptr;
    void* fn = nullptr;
    long long n = 0;
    long long nblocks = 0;
    std::atomic<long long> next{0};
    std::atomic<long long> fail{LLONG_MAX}; // the lowest failing block
    const QSourceLoc* loc = nullptr;
    QRowCtx row_ctx;
    // The lowest failing block's error (under mu).
    std::mutex mu;
    long long err_block = LLONG_MAX;
    QErrorCapture err;
    int err_code = 1;
    std::exception_ptr exc;
};

struct Pool {
    std::mutex mu;
    std::condition_variable wake; // workers wait for a task
    std::condition_variable idle; // the caller waits for workers to leave it
    Task* task = nullptr;
    uint64_t gen = 0;
    int joined = 0; // workers working on task
};

// The pool and the once-call lock are never destroyed: workers wait on them
// until the program exits.
inline Pool*& pool_ptr() {
    static Pool* p = nullptr;
    return p;
}

inline std::mutex& once_mu() {
    static std::mutex* m = new std::mutex();
    return *m;
}

inline std::condition_variable& once_cv() {
    static std::condition_variable* cv = new std::condition_variable();
    return *cv;
}

inline void record(Task& t, long long blk, const QErrorCapture& cap, int code, std::exception_ptr exc) {
    {
        std::lock_guard<std::mutex> lk(t.mu);
        if (blk < t.err_block) {
            t.err_block = blk;
            t.err = cap;
            t.err_code = code;
            t.exc = exc;
        }
    }
    long long cur = t.fail.load(std::memory_order_relaxed);
    while (blk < cur && !t.fail.compare_exchange_weak(cur, blk, std::memory_order_acq_rel)) {
    }
}

// Block outcomes.
enum : int { BLOCK_OK = 0, BLOCK_ERROR = 1, BLOCK_OVERFLOW = 2, BLOCK_EXCEPTION = 3 };

#if defined(__clang__) || defined(__GNUC__)
#define QPAR_NOINLINE __attribute__((noinline))
#else
#define QPAR_NOINLINE
#endif

QPAR_NOINLINE inline int call_block(Task& t, long long lo, long long hi, long long blk, int& code, std::exception_ptr& exc) {
    try {
        t.run(t.fn, lo, hi, blk);
        return BLOCK_OK;
    } catch (const QBlockExit& e) {
        code = e.code;
        return BLOCK_ERROR;
    } catch (...) {
        exc = std::current_exception();
        return BLOCK_EXCEPTION;
    }
}

#if defined(_WIN32)

extern "C" int _resetstkoflw(void);

// The stack guard of a block (SEH): __try can't be in a function with C++
// objects to unwind, so the block's C++ code is call_block's.
QPAR_NOINLINE inline int guarded_block(Task& t, long long lo, long long hi, long long blk, int& code, std::exception_ptr& exc) {
    __try {
        return call_block(t, lo, hi, blk, code, exc);
    } __except (_exception_code() == 0xC00000FDL ? 1 : 0) {
        _resetstkoflw();
        return BLOCK_OVERFLOW;
    }
}

#else

// The stack guard of a block: the thread's guard jumps back here, and the
// guard the thread had (the main thread's, armed by q_protect_and_run) is
// restored after the block.
QPAR_NOINLINE inline int guarded_block(Task& t, long long lo, long long hi, long long blk, int& code, std::exception_ptr& exc) {
    QStackGuard& g = q_stack_guard;
    sigjmp_buf saved;
    std::memcpy(&saved, &g.jb, sizeof saved);
    const sig_atomic_t was = g.ready;
    g.ready = 0;
    int out = BLOCK_OK;
    if (sigsetjmp(g.jb, 1) != 0) {
        out = BLOCK_OVERFLOW;
    } else {
        g.ready = 1;
        out = call_block(t, lo, hi, blk, code, exc);
        g.ready = 0;
    }
    std::memcpy(&g.jb, &saved, sizeof saved);
    g.ready = was;
    return out;
}

#endif

// overflow_exit reports a stack overflow in a block, as q_stack_overflow_exit
// does on one thread (at the location the thread had reached), and ends the
// program. Only the first thread to overflow reports; another one waits for
// the exit.
[[noreturn]] inline void overflow_exit() {
    static std::atomic<bool> taken{false};
    if (taken.exchange(true)) {
        for (;;) std::this_thread::sleep_for(std::chrono::seconds(1));
    }
    q_error_capture = nullptr;
    q_stack_overflow_exit();
    std::abort(); // not reached: q_stack_overflow_exit exits
}

inline void run_block(Task& t, long long blk) {
    const long long lo = blk * q_par_block;
    const long long hi = lo + q_par_block < t.n ? lo + q_par_block : t.n;
    const QSourceLoc* saved_loc = q_runtime_loc;
    const QRowCtx saved_ctx = q_row_ctx;
    QErrorCapture* saved_cap = q_error_capture;
    Task* saved_task = current;
    QErrorCapture cap;
    cap.text[0] = '\0';
    q_runtime_loc = t.loc;
    q_row_ctx = t.row_ctx;
    q_error_capture = &cap;
    current = &t;
    depth++;
    int code = 1;
    std::exception_ptr exc;
    const int out = guarded_block(t, lo, hi, blk, code, exc);
    if (out == BLOCK_OVERFLOW) overflow_exit();
    depth--;
    current = saved_task;
    q_error_capture = saved_cap;
    q_row_ctx = saved_ctx;
    q_runtime_loc = saved_loc;
    if (out != BLOCK_OK) record(t, blk, cap, code, exc);
}

// work takes blocks of t, in increasing order, until none is left or a
// lower block has failed.
inline void work(Task& t) {
    while (true) {
        const long long blk = t.next.fetch_add(1, std::memory_order_relaxed);
        if (blk >= t.nblocks) return;
        if (blk > t.fail.load(std::memory_order_acquire)) return;
        run_block(t, blk);
    }
}

inline void worker_loop() {
    Pool& p = *pool_ptr();
    uint64_t seen = 0;
    std::unique_lock<std::mutex> lk(p.mu);
    while (true) {
        p.wake.wait(lk, [&] { return p.gen != seen && p.task != nullptr; });
        seen = p.gen;
        Task* t = p.task;
        p.joined++;
        lk.unlock();
        work(*t);
        lk.lock();
        if (--p.joined == 0) p.idle.notify_all();
    }
}

inline void worker_start() {
#ifdef QUARK_USE_GC
    struct GC_stack_base sb;
    if (GC_get_stack_base(&sb) == GC_SUCCESS) GC_register_my_thread(&sb);
#endif
#if !defined(_WIN32)
    q_stack_thread_init();
#endif
    worker_loop();
}

// worker_stack is a worker's stack size: the main thread's, so a recursion
// that fits on one thread fits on a worker.
inline size_t worker_stack() {
    size_t size = 0;
#if defined(_WIN32)
    unsigned long long lo = 0, hi = 0;
    GetCurrentThreadStackLimits(&lo, &hi);
    size = static_cast<size_t>(hi - lo);
#else
    const QStackGuard& g = q_stack_guard;
    if (g.hi > g.lo) size = static_cast<size_t>(g.hi - g.lo);
    if (size == 0) {
        struct rlimit rl;
        if (getrlimit(RLIMIT_STACK, &rl) == 0 && rl.rlim_cur != RLIM_INFINITY) size = static_cast<size_t>(rl.rlim_cur);
    }
#endif
    const size_t mb = static_cast<size_t>(1) << 20;
    if (size == 0) size = 8 * mb;
    if (size < mb) size = mb;
    if (size > 1024 * mb) size = 1024 * mb;
    return (size + 65535) & ~static_cast<size_t>(65535);
}

#if defined(_WIN32)
inline unsigned __stdcall worker_entry(void*) {
    worker_start();
    return 0;
}
#else
inline void* worker_entry(void*) {
    worker_start();
    return nullptr;
}
#endif

// start_worker starts one worker; false if the system refuses.
inline bool start_worker(size_t stack) {
#if defined(_WIN32)
    const unsigned stack_reservation = 0x00010000; // STACK_SIZE_PARAM_IS_A_RESERVATION
    const uintptr_t h = _beginthreadex(nullptr, static_cast<unsigned>(stack), &worker_entry, nullptr, stack_reservation, nullptr);
    return h != 0;
#else
    pthread_attr_t attr;
    if (pthread_attr_init(&attr) != 0) return false;
    pthread_attr_setstacksize(&attr, stack);
    pthread_attr_setdetachstate(&attr, PTHREAD_CREATE_DETACHED);
    pthread_t th;
    const bool ok = pthread_create(&th, &attr, &worker_entry, nullptr) == 0;
    pthread_attr_destroy(&attr);
    return ok;
#endif
}

// pool starts the workers the first time a loop is split: the thread count
// less the calling thread. A worker the system refuses to start is left
// out; the output doesn't depend on how many run.
inline Pool& pool() {
    Pool*& p = pool_ptr();
    if (p != nullptr) return *p;
    p = new Pool();
#ifdef QUARK_USE_GC
    GC_allow_register_threads();
#endif
    const size_t stack = worker_stack();
    for (int i = 1; i < q_par_threads(); i++) {
        if (!start_worker(stack)) break;
    }
    return *p;
}

template <class F>
inline void trampoline(void* fn, long long lo, long long hi, long long blk) {
    (*static_cast<F*>(fn))(lo, hi, blk);
}

// report ends the program with t's lowest failing block's error, as the one
// thread would have reported it.
[[noreturn]] inline void report(Task& t) {
    if (t.exc) std::rethrow_exception(t.exc);
    if (QErrorCapture* c = q_error_capture) {
        // Not reached: a loop on a block's thread runs on that thread.
        q_capture_text(*c, t.err.text);
        throw QBlockExit{t.err_code};
    }
    if (t.err.flush_stdout) std::fflush(stdout);
    std::fputs(t.err.text, stderr);
    q_exit(t.err_code);
}

inline Plan plan(long long n) {
    Plan p;
    p.n = n < 0 ? 0 : n;
    p.nblocks = 1;
    if (p.n >= q_par_min_rows && depth == 0 && q_par_threads() > 1) p.nblocks = (p.n + q_par_block - 1) / q_par_block;
    return p;
}

// run runs f(lo, hi, blk) for every block of p.
template <class F>
inline void run(const Plan& p, F& f) {
    if (p.nblocks <= 1) {
        f(0LL, p.n, 0LL);
        return;
    }
    Pool& pl = pool();
    Task t;
    t.run = &trampoline<F>;
    t.fn = static_cast<void*>(&f);
    t.n = p.n;
    t.nblocks = p.nblocks;
    t.loc = q_runtime_loc;
    t.row_ctx = q_row_ctx;
    {
        std::lock_guard<std::mutex> lk(pl.mu);
        pl.task = &t;
        pl.gen++;
    }
    pl.wake.notify_all();
    work(t);
    {
        std::unique_lock<std::mutex> lk(pl.mu);
        pl.task = nullptr;
        pl.idle.wait(lk, [&] { return pl.joined == 0; });
    }
    if (t.err_block != LLONG_MAX) report(t);
}

// ------------------------------------------------------------
// Once-calls (D44): a call a loop's rows share is made at most once, by the
// first row that needs it; a row on another thread that needs it meanwhile
// waits for it. When it fails, every row that needs it fails with its error,
// so the loop reports it at the first of them (the error of one thread).

struct Once {
    std::atomic<int> state{0}; // 0 not made, 1 being made, 2 made, 3 failed
    QErrorCapture* err = nullptr;
    int code = 1;
};

enum : int { ONCE_NEW = 0, ONCE_RUNNING = 1, ONCE_READY = 2, ONCE_FAILED = 3 };

// fail_as ends the current block with err, as the row that made the call
// failed.
[[noreturn]] inline void fail_as(const QErrorCapture& err, int code) {
    if (QErrorCapture* c = q_error_capture) {
        q_capture_text(*c, err.text);
        c->flush_stdout = c->flush_stdout || err.flush_stdout;
        throw QBlockExit{code};
    }
    if (err.flush_stdout) std::fflush(stdout);
    std::fputs(err.text, stderr);
    q_exit(code);
}

struct OnceGuard {
    Once* o;
    bool mine = false;
    bool finished = false;

    explicit OnceGuard(Once& once) : o(&once) {
        int expect = ONCE_NEW;
        if (o->state.compare_exchange_strong(expect, ONCE_RUNNING, std::memory_order_acq_rel)) {
            mine = true;
            return;
        }
        std::unique_lock<std::mutex> lk(once_mu());
        once_cv().wait(lk, [&] { return o->state.load(std::memory_order_acquire) != ONCE_RUNNING; });
        if (o->state.load(std::memory_order_acquire) == ONCE_READY) return;
        const QErrorCapture err = o->err != nullptr ? *o->err : QErrorCapture{};
        const int code = o->code;
        lk.unlock();
        fail_as(err, code);
    }

    void done() {
        {
            std::lock_guard<std::mutex> lk(once_mu());
            o->state.store(ONCE_READY, std::memory_order_release);
        }
        once_cv().notify_all();
        finished = true;
    }

    // The call failed: its row's error, recorded in the block's capture, is
    // the error of every row that needs it.
    ~OnceGuard() {
        if (!mine || finished) return;
        {
            std::lock_guard<std::mutex> lk(once_mu());
            QErrorCapture* err = static_cast<QErrorCapture*>(std::malloc(sizeof(QErrorCapture)));
            if (err != nullptr) {
                *err = q_error_capture != nullptr ? *q_error_capture : QErrorCapture{};
                if (q_error_capture == nullptr) err->text[0] = '\0';
            }
            o->err = err;
            o->code = q_error_capture != nullptr ? q_error_capture->code : 1;
            o->state.store(ONCE_FAILED, std::memory_order_release);
        }
        once_cv().notify_all();
    }

    OnceGuard(const OnceGuard&) = delete;
    OnceGuard& operator=(const OnceGuard&) = delete;
};

} // namespace par
} // namespace quark

// What generated code calls (lower/query.go).

inline quark::par::Plan q_par_plan(long long n) { return quark::par::plan(n); }

template <class F>
inline void q_par_run(const quark::par::Plan& p, F&& f) {
    quark::par::run(p, f);
}

inline bool q_once_ready(const quark::par::Once& o) {
    return o.state.load(std::memory_order_acquire) == quark::par::ONCE_READY;
}
inline quark::par::OnceGuard q_once_begin(quark::par::Once& o) { return quark::par::OnceGuard(o); }
inline bool q_once_mine(const quark::par::OnceGuard& g) { return g.mine; }
inline void q_once_done(quark::par::OnceGuard& g) { g.done(); }

#endif // QUARK_CORE_PAR_HPP
