// quark/core/diagnostics.hpp - runtime diagnostics context and emitters
#ifndef QUARK_CORE_DIAGNOSTICS_HPP
#define QUARK_CORE_DIAGNOSTICS_HPP

#include <cstdarg>
#include <cstring>
#include <cstdio>

struct QSourceLoc {
    const char* file;
    int line;
    int col;
};

inline thread_local QSourceLoc q_runtime_loc = {"<unknown>", 0, 0};

inline void q_set_source_loc(const char* file, int line, int col) {
    q_runtime_loc.file = file != nullptr ? file : "<unknown>";
    q_runtime_loc.line = line;
    q_runtime_loc.col = col;
}

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

    if (q_runtime_loc.line > 0 && q_runtime_loc.col > 0) {
        std::fprintf(stderr, "error[QK-RUNTIME-001] (runtime): %s at %s:%d:%d\n", message, q_runtime_loc.file, q_runtime_loc.line, q_runtime_loc.col);
    } else if (q_runtime_loc.line > 0) {
        std::fprintf(stderr, "error[QK-RUNTIME-001] (runtime): %s at %s:%d\n", message, q_runtime_loc.file, q_runtime_loc.line);
    } else {
        std::fprintf(stderr, "error[QK-RUNTIME-001] (runtime): %s\n", message);
    }
}

#endif // QUARK_CORE_DIAGNOSTICS_HPP