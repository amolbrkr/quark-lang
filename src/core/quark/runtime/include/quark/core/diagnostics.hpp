// quark/core/diagnostics.hpp - runtime diagnostics context and emitters
#ifndef QUARK_CORE_DIAGNOSTICS_HPP
#define QUARK_CORE_DIAGNOSTICS_HPP

#include "rowrand.hpp"
#include <cstdarg>
#include <cstdlib>
#include <cstring>
#include <cstdio>

// A source location. Generated code keeps every location of the program in
// one static table and records the current one as a pointer into it, so
// marking a statement is a single store.
struct QSourceLoc {
    const char* file;
    int line;
    int col;
};

inline constexpr QSourceLoc q_no_source_loc = {"<unknown>", 0, 0};

inline thread_local const QSourceLoc* q_runtime_loc = &q_no_source_loc;

inline void q_set_source_loc(const QSourceLoc* loc) {
    q_runtime_loc = loc != nullptr ? loc : &q_no_source_loc;
}

// QSourceLocGuard restores the caller's source location when a generated
// function returns, so a runtime error later in the calling statement is
// reported there and not at the last statement that ran inside the callee.
struct QSourceLocGuard {
    const QSourceLoc* saved;
    QSourceLocGuard() : saved(q_runtime_loc) {}
    ~QSourceLocGuard() { q_runtime_loc = saved; }
    QSourceLocGuard(const QSourceLocGuard&) = delete;
    QSourceLocGuard& operator=(const QSourceLocGuard&) = delete;
};

// QErrorCapture holds the error text of a thread running a block of a
// parallel loop (core/par.hpp, specs/tables_v0_3_spec.md §8.5). While a
// thread's q_error_capture is set, an error is recorded there instead of
// printed, and q_exit ends the block (it throws QBlockExit) instead of the
// program: the thread that started the loop reports the error of the lowest
// failing block, as one thread would. flush_stdout is set by an error whose
// report flushes stdout first (a stack overflow).
struct QErrorCapture {
    char text[4096];
    size_t len = 0;
    int code = 1;
    bool flush_stdout = false;
};

inline thread_local QErrorCapture* q_error_capture = nullptr;

// QBlockExit ends a block whose row failed; code is the exit status the
// program ends with.
struct QBlockExit {
    int code;
};

inline void q_capture_text(QErrorCapture& c, const char* text) {
    const size_t room = sizeof(c.text) - 1 - c.len;
    size_t n = std::strlen(text);
    if (n > room) n = room;
    std::memcpy(c.text + c.len, text, n);
    c.len += n;
    c.text[c.len] = '\0';
}

inline void q_report_message(const char* code, const char* message) {
    const QSourceLoc& loc = *q_runtime_loc;
    char line[2560];
    if (loc.line > 0 && loc.col > 0) {
        std::snprintf(line, sizeof line, "error[%s] (runtime): %s at %s:%d:%d\n", code, message, loc.file, loc.line, loc.col);
    } else if (loc.line > 0) {
        std::snprintf(line, sizeof line, "error[%s] (runtime): %s at %s:%d\n", code, message, loc.file, loc.line);
    } else {
        std::snprintf(line, sizeof line, "error[%s] (runtime): %s\n", code, message);
    }
    if (QErrorCapture* c = q_error_capture) {
        q_capture_text(*c, line);
        return;
    }
    std::fputs(line, stderr);
}

inline void q_format_report(const char* code, const char* fmt, va_list args) {
    char buffer[2048];
    std::vsnprintf(buffer, sizeof(buffer), fmt, args);

    size_t len = std::strlen(buffer);
    for (; len > 0; --len) {
        if (buffer[len - 1] != '\n' && buffer[len - 1] != '\r') {
            break;
        }
        buffer[len - 1] = '\0';
    }

    const char* message = buffer;
    const char* prefix = "runtime error: ";
    if (std::strncmp(message, prefix, std::strlen(prefix)) == 0) {
        message += std::strlen(prefix);
    }
    q_report_message(code, message);
}

inline void q_runtime_reportf(const char* fmt, ...) {
    va_list args;
    va_start(args, fmt);
    q_format_report("QK-RUNTIME-001", fmt, args);
    va_end(args);
}

// q_exit ends a failed program. A REPL host on Windows sets q_exit_hook so a
// runtime error can return to the host instead of killing the process; the
// hook must not return. Everywhere else the hook is null and this is exit.
#if defined(QUARK_REPL) && defined(_WIN32)
#if defined(QUARK_REPL_BUILD)
#define QREPL_API __declspec(dllexport)
#else
#define QREPL_API __declspec(dllimport)
#endif
#else
#define QREPL_API
#endif

#if defined(QUARK_REPL)
extern "C" QREPL_API void (*q_exit_hook)(int);
#else
inline void (*q_exit_hook)(int) = nullptr;
#endif

[[noreturn]] inline void q_exit(int code) {
    if (QErrorCapture* c = q_error_capture) {
        c->code = code;
        throw QBlockExit{code};
    }
    if (q_exit_hook != nullptr) {
        // The REPL host (Windows) recovers from the failed input in this
        // process: a verb's random-draw context must not outlive it.
        q_row_ctx = QRowCtx{};
        q_exit_hook(code);
    }
    ::exit(code);
}

[[noreturn]] inline void q_row_stream_exhausted() {
    q_runtime_reportf("runtime error: a random draw in a query needs more numbers than one row can draw (2^33)\n");
    q_exit(1);
}

// QK-FFI-004: a handle of the wrong kind, or use of a handle that is closed.
[[noreturn]] inline void q_ffi_fail(const char* fmt, ...) {
    va_list args;
    va_start(args, fmt);
    q_format_report("QK-FFI-004", fmt, args);
    va_end(args);
    q_exit(1);
}

#endif // QUARK_CORE_DIAGNOSTICS_HPP