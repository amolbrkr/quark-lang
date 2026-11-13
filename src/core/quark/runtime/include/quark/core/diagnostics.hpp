// quark/core/diagnostics.hpp - runtime diagnostics context and emitters
#ifndef QUARK_CORE_DIAGNOSTICS_HPP
#define QUARK_CORE_DIAGNOSTICS_HPP

#include <cstdarg>
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

inline void q_runtime_reportf(const char* fmt, ...) {
    char buffer[2048];
    va_list args;
    va_start(args, fmt);
    std::vsnprintf(buffer, sizeof(buffer), fmt, args);
    va_end(args);

    size_t len = std::strlen(buffer);
    for (; len > 0; --len) {
        if (buffer[len-1] != '\n' && buffer[len-1] != '\r') {
            break;
        }
        buffer[len-1] = '\0';
    }

    const char* message = buffer;
    const char* prefix = "runtime error: ";
    if (std::strncmp(message, prefix, std::strlen(prefix)) == 0) {
        message += std::strlen(prefix);
    }

    const QSourceLoc& loc = *q_runtime_loc;
    if (loc.line > 0 && loc.col > 0) {
        std::fprintf(stderr, "error[QK-RUNTIME-001] (runtime): %s at %s:%d:%d\n", message, loc.file, loc.line, loc.col);
    } else if (loc.line > 0) {
        std::fprintf(stderr, "error[QK-RUNTIME-001] (runtime): %s at %s:%d\n", message, loc.file, loc.line);
    } else {
        std::fprintf(stderr, "error[QK-RUNTIME-001] (runtime): %s\n", message);
    }
}

#endif // QUARK_CORE_DIAGNOSTICS_HPP