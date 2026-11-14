// stdlib/sys.hpp — native implementations for the std/sys module (sys.qrk).
#ifndef QUARK_STDLIB_SYS_HPP
#define QUARK_STDLIB_SYS_HPP

#include <cerrno>
#include <cstdio>
#include <cstdlib>
#include <cstring>
#include <string>
#include <thread>

#ifdef _WIN32
#include <direct.h>
#include <process.h>
#else
#include <unistd.h>
#endif

// Program arguments after the executable name, as a list of strings.
inline QList* q_sys_args() {
    QValue list = qv_list(q_process_argc > 0 ? q_process_argc - 1 : 0, &q_elem_str);
    for (int i = 1; i < q_process_argc; i++) {
        list.data.list_val->push_back(qv_string(q_process_argv[i]));
    }
    return list.data.list_val;
}

// The path the program was started as (argv[0]). Empty when the host
// provided no arguments.
inline const char* q_sys_program() {
    if (q_process_argc < 1 || q_process_argv == nullptr || q_process_argv[0] == nullptr) {
        return q_strdup("");
    }
    return q_strdup(q_process_argv[0]);
}

// Value of an environment variable, or null when it isn't set.
inline QValue q_sys_env(const char* name) {
    const char* value = (name != nullptr && name[0] != '\0') ? std::getenv(name) : nullptr;
    return value ? qv_string(value) : qv_null();
}

// The value, or fallback when the variable is unset. Never null.
inline const char* q_sys_env_or(const char* name, const char* fallback) {
    const char* value = (name != nullptr && name[0] != '\0') ? std::getenv(name) : nullptr;
    if (value == nullptr) value = fallback != nullptr ? fallback : "";
    return q_strdup(value);
}

[[noreturn]] inline void q_sys_fail(const char* fn, const char* msg) {
    q_runtime_reportf("runtime error: sys::%s() %s\n", fn, msg);
    std::exit(1);
}

inline void q_sys_check_name(const char* name, const char* fn) {
    if (name == nullptr || name[0] == '\0') {
        q_sys_fail(fn, "name must not be empty");
    }
    if (std::strchr(name, '=') != nullptr) {
        q_sys_fail(fn, "name must not contain '='");
    }
}

// Set a variable for this process. A failure (the OS refused the name) is a
// runtime error: leaving the old value in place would be a silent miss.
inline QValue q_sys_set_env(const char* name, const char* value) {
    q_sys_check_name(name, "set_env");
    if (value == nullptr) value = "";
#ifdef _WIN32
    if (_putenv_s(name, value) != 0) {
        q_sys_fail("set_env", "the operating system rejected the variable");
    }
#else
    if (setenv(name, value, 1) != 0) {
        q_runtime_reportf("runtime error: sys::set_env() failed: %s\n", std::strerror(errno));
        std::exit(1);
    }
#endif
    return qv_null();
}

// Remove a variable. Removing one that is not set succeeds.
inline QValue q_sys_unset_env(const char* name) {
    q_sys_check_name(name, "unset_env");
#ifdef _WIN32
    // An empty value removes the variable (MSVC _putenv_s).
    if (_putenv_s(name, "") != 0) {
        q_sys_fail("unset_env", "the operating system rejected the name");
    }
#else
    if (unsetenv(name) != 0) {
        q_runtime_reportf("runtime error: sys::unset_env() failed: %s\n", std::strerror(errno));
        std::exit(1);
    }
#endif
    return qv_null();
}

// Every variable, as a dict whose values are strings. A later set of a
// non-string fails at runtime. Windows drive-directory entries (names that
// start with '=') are not variables and are skipped.
inline QDict* q_sys_environ() {
    QValue dict = qv_dict(&q_elem_str);
    char** env = nullptr;
#ifdef _WIN32
    env = _environ;
#else
    extern char** environ;
    env = environ;
#endif
    if (env == nullptr) return dict.data.dict_val;
    for (char** p = env; *p != nullptr; ++p) {
        const char* eq = std::strchr(*p, '=');
        if (eq == nullptr || eq == *p) continue;
        std::string name(*p, static_cast<size_t>(eq - *p));
        q_dict_set(dict, qv_string(name.c_str()), qv_string(eq + 1));
    }
    return dict.data.dict_val;
}

// Exit the program with the given status after flushing output. Only the
// low 8 bits of a status reach the parent process, so a code outside 0-255
// (256 would read as success) is an error rather than silently wrapped.
[[noreturn]] inline QValue q_sys_exit(int64_t code) {
    if (code < 0 || code > 255) {
        q_runtime_reportf("runtime error: sys::exit() status must be between 0 and 255, got %lld\n", static_cast<long long>(code));
        std::exit(1);
    }
    std::fflush(stdout);
    std::fflush(stderr);
    std::exit(static_cast<int>(code));
}

inline const char* q_sys_cwd() {
    int cap = 256;
    for (;;) {
        std::string buf(static_cast<size_t>(cap), '\0');
        char* got = nullptr;
#ifdef _WIN32
        got = _getcwd(&buf[0], cap);
#else
        got = getcwd(&buf[0], static_cast<size_t>(cap));
#endif
        if (got != nullptr) return q_strdup(got);
        if (errno != ERANGE || cap > 1024 * 1024) {
            q_runtime_reportf("runtime error: sys::cwd() failed: %s\n", std::strerror(errno));
            std::exit(1);
        }
        cap *= 2;
    }
}

// ok(null), or err when the directory cannot be entered.
inline QValue q_sys_chdir(const char* path) {
    if (path == nullptr || path[0] == '\0') {
        return qv_err(qv_string("chdir: path is empty"));
    }
    int rc = 0;
#ifdef _WIN32
    rc = _chdir(path);
#else
    rc = ::chdir(path);
#endif
    if (rc != 0) {
        std::string msg = "chdir: ";
        msg += path;
        msg += ": ";
        msg += std::strerror(errno);
        return qv_err(qv_string(msg.c_str()));
    }
    return qv_ok(qv_null());
}

inline const char* q_sys_platform() {
#if defined(_WIN32)
    return "windows";
#elif defined(__APPLE__)
    return "darwin";
#elif defined(__linux__)
    return "linux";
#else
    return "unknown";
#endif
}

inline const char* q_sys_arch() {
#if defined(_M_X64) || defined(__x86_64__)
    return "x86_64";
#elif defined(_M_ARM64) || defined(__aarch64__)
    return "arm64";
#elif defined(_M_IX86) || defined(__i386__)
    return "x86";
#else
    return "unknown";
#endif
}

inline int64_t q_sys_pid() {
#ifdef _WIN32
    return static_cast<int64_t>(_getpid());
#else
    return static_cast<int64_t>(::getpid());
#endif
}

// Logical CPUs. 0 means the operating system did not report a count.
inline int64_t q_sys_cpu_count() {
    return static_cast<int64_t>(std::thread::hardware_concurrency());
}

// x rendered the way print renders it, written to stderr, then end.
inline QValue q_sys_eprint(QValue v, const char* end) {
    if (end == nullptr) {
        q_sys_fail("eprint", "end must be str");
    }
    QValue text = q_str(v);
    const char* s = (text.type == QValue::VAL_STRING && text.data.string_val != nullptr) ? text.data.string_val : "";
    std::fputs(s, stderr);
    std::fputs(end, stderr);
    std::fflush(stderr);
    return qv_null();
}

#endif // QUARK_STDLIB_SYS_HPP
