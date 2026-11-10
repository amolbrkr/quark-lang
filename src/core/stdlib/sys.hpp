// stdlib/sys.hpp — native implementations for the std/sys module (sys.qrk).
#ifndef QUARK_STDLIB_SYS_HPP
#define QUARK_STDLIB_SYS_HPP

#include <cstdio>
#include <cstdlib>

// Program arguments after the executable name, as a list of strings.
inline QList* q_sys_args() {
    QValue list = qv_list();
    for (int i = 1; i < q_process_argc; i++) {
        list.data.list_val->push_back(qv_string(q_process_argv[i]));
    }
    return list.data.list_val;
}

// Value of an environment variable, or null when it isn't set.
inline QValue q_sys_env(const char* name) {
    const char* value = name ? std::getenv(name) : nullptr;
    return value ? qv_string(value) : qv_null();
}

// Exit the program with the given status after flushing output.
[[noreturn]] inline QValue q_sys_exit(int64_t code) {
    std::fflush(stdout);
    std::fflush(stderr);
    std::exit(static_cast<int>(code));
}

#endif // QUARK_STDLIB_SYS_HPP
