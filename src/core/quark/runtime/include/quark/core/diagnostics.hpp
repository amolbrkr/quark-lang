// quark/core/diagnostics.hpp - runtime diagnostics context and emitters
#ifndef QUARK_CORE_DIAGNOSTICS_HPP
#define QUARK_CORE_DIAGNOSTICS_HPP

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

inline void q_report_message(const char* code, const char* message) {
    const QSourceLoc& loc = *q_runtime_loc;
    if (loc.line > 0 && loc.col > 0) {
        std::fprintf(stderr, "error[%s] (runtime): %s at %s:%d:%d\n", code, message, loc.file, loc.line, loc.col);
    } else if (loc.line > 0) {
        std::fprintf(stderr, "error[%s] (runtime): %s at %s:%d\n", code, message, loc.file, loc.line);
    } else {
        std::fprintf(stderr, "error[%s] (runtime): %s\n", code, message);
    }
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
    if (q_exit_hook != nullptr) {
        q_exit_hook(code);
    }
    ::exit(code);
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