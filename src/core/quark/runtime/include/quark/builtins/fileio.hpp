// quark/builtins/fileio.hpp - Low-level file I/O primitives
#ifndef QUARK_BUILTINS_FILEIO_HPP
#define QUARK_BUILTINS_FILEIO_HPP

#include "../core/value.hpp"
#include "../core/constructors.hpp"
#include "../types/resource.hpp"

#include <cerrno>
#include <cstdio>
#include <cstdlib>
#include <cstring>
#include <string>

#ifdef _WIN32
#include <fcntl.h>
#include <io.h>
#include <sys/stat.h>
#else
#include <fcntl.h>
#include <sys/stat.h>
#include <unistd.h>
#endif

struct QFilePayload {
    int fd;
    bool binary;
};

inline QValue q_file_err_with_detail(const char* op, const std::string& detail) {
    std::string msg(op);
    msg += ": ";
    msg += detail;
    return qv_err(qv_string(msg.c_str()));
}

inline QValue q_file_sys_err(const char* op, const char* path) {
    std::string detail;
    if (path && path[0] != '\0') {
        detail += path;
        detail += ": ";
    }
    detail += std::strerror(errno);
    return q_file_err_with_detail(op, detail);
}

inline int q_file_open_native(const char* path, const char* mode, bool binary) {
    int flags = 0;

    if (std::strcmp(mode, "r") == 0) {
        flags = O_RDONLY;
    } else if (std::strcmp(mode, "w") == 0) {
        flags = O_WRONLY | O_CREAT | O_TRUNC;
    } else {
        return -2;
    }

#ifdef _WIN32
    flags |= binary ? _O_BINARY : _O_TEXT;
    return _open(path, flags, _S_IREAD | _S_IWRITE);
#else
    (void)binary;
    return ::open(path, flags, 0666);
#endif
}

inline long long q_file_seek_native(int fd, long long offset, int whence) {
#ifdef _WIN32
    return _lseeki64(fd, offset, whence);
#else
    return static_cast<long long>(::lseek(fd, static_cast<off_t>(offset), whence));
#endif
}

inline bool q_file_exists_native(const char* path) {
#ifdef _WIN32
    return _access(path, 0) == 0;
#else
    return access(path, F_OK) == 0;
#endif
}

inline QFilePayload* q_expect_file_payload(QValue file, const char* op) {
    QResourceSlot* slot = nullptr;
    QResourceHandle* handle = nullptr;
    const char* err = nullptr;
    if (!q_resource_resolve(file, QRES_KIND_FILE, slot, handle, err)) {
        std::fprintf(stderr, "runtime error: %s() expects live file_handle (%s)\n", op, err ? err : "invalid handle");
        std::exit(1);
    }
    if (!slot || !slot->payload) {
        std::fprintf(stderr, "runtime error: %s() file_handle has no payload\n", op);
        std::exit(1);
    }
    return static_cast<QFilePayload*>(slot->payload);
}

inline QValue q_file_open(QValue path, QValue mode, QValue binary) {
    if (path.type != QValue::VAL_STRING || !path.data.string_val) {
        std::fprintf(stderr, "runtime error: _file_open() argument 1 must be str\n");
        std::exit(1);
    }
    if (mode.type != QValue::VAL_STRING || !mode.data.string_val) {
        std::fprintf(stderr, "runtime error: _file_open() argument 2 must be str\n");
        std::exit(1);
    }
    if (binary.type != QValue::VAL_BOOL) {
        std::fprintf(stderr, "runtime error: _file_open() argument 3 must be bool\n");
        std::exit(1);
    }

    int fd = q_file_open_native(path.data.string_val, mode.data.string_val, binary.data.bool_val);
    if (fd == -2) {
        return q_file_err_with_detail("open", "mode must be 'r' or 'w'");
    }
    if (fd < 0) {
        return q_file_sys_err("open", path.data.string_val);
    }

    QFilePayload* payload = static_cast<QFilePayload*>(std::malloc(sizeof(QFilePayload)));
    if (!payload) {
#ifdef _WIN32
        _close(fd);
#else
        ::close(fd);
#endif
        std::fprintf(stderr, "runtime error: _file_open() failed to allocate payload\n");
        std::exit(1);
    }

    payload->fd = fd;
    payload->binary = binary.data.bool_val;

    QResourceHandle* handle = q_resource_create(QRES_KIND_FILE, payload, QRES_FLAG_OWNED | QRES_FLAG_FINALIZABLE);
    if (!handle) {
#ifdef _WIN32
        _close(fd);
#else
        ::close(fd);
#endif
        std::free(payload);
        std::fprintf(stderr, "runtime error: _file_open() failed to allocate resource handle\n");
        std::exit(1);
    }

    return qv_ok(qv_resource(handle));
}

inline QValue q_file_open(QValue path, QValue mode) {
    return q_file_open(path, mode, qv_bool(false));
}

inline QValue q_file_read(QValue file, QValue n) {
    if (n.type != QValue::VAL_INT) {
        std::fprintf(stderr, "runtime error: _file_read() argument 2 must be int\n");
        std::exit(1);
    }
    if (n.data.int_val < 0) {
        std::fprintf(stderr, "runtime error: _file_read() argument 2 must be >= 0\n");
        std::exit(1);
    }

    QFilePayload* payload = q_expect_file_payload(file, "_file_read");

    const size_t want = static_cast<size_t>(n.data.int_val);
    if (want == 0) {
        return qv_ok(qv_string(""));
    }

    std::string buffer;
    buffer.resize(want);

#ifdef _WIN32
    int got = _read(payload->fd, &buffer[0], static_cast<unsigned int>(want));
#else
    ssize_t got = ::read(payload->fd, &buffer[0], want);
#endif

    if (got < 0) {
        return q_file_sys_err("read", nullptr);
    }
    if (got == 0) {
        return qv_ok(qv_string(""));
    }

    buffer.resize(static_cast<size_t>(got));
    return qv_ok(qv_string(buffer.c_str()));
}

inline QValue q_file_write(QValue file, QValue data) {
    if (data.type != QValue::VAL_STRING || !data.data.string_val) {
        std::fprintf(stderr, "runtime error: _file_write() argument 2 must be str\n");
        std::exit(1);
    }

    QFilePayload* payload = q_expect_file_payload(file, "_file_write");

    const char* raw = data.data.string_val;
    const size_t len = std::strlen(raw);

#ifdef _WIN32
    int wrote = _write(payload->fd, raw, static_cast<unsigned int>(len));
#else
    ssize_t wrote = ::write(payload->fd, raw, len);
#endif

    if (wrote < 0) {
        return q_file_sys_err("write", nullptr);
    }

    return qv_ok(qv_int(static_cast<long long>(wrote)));
}

inline QValue q_file_close(QValue file) {
    QResourceSlot* slot = nullptr;
    QResourceHandle* handle = nullptr;
    const char* err = nullptr;
    if (!q_resource_resolve(file, QRES_KIND_FILE, slot, handle, err)) {
        return q_file_err_with_detail("close", err ? err : "invalid file_handle");
    }

    QFilePayload* payload = static_cast<QFilePayload*>(slot->payload);
    if (!payload) {
        return q_file_err_with_detail("close", "file payload missing");
    }

#ifdef _WIN32
    int rc = _close(payload->fd);
#else
    int rc = ::close(payload->fd);
#endif
    if (rc != 0) {
        return q_file_sys_err("close", nullptr);
    }

    std::free(payload);
    q_resource_invalidate(handle);
    return qv_ok(qv_null());
}

inline QValue q_file_seek(QValue file, QValue offset, QValue whence) {
    if (offset.type != QValue::VAL_INT) {
        std::fprintf(stderr, "runtime error: _file_seek() argument 2 must be int\n");
        std::exit(1);
    }
    if (whence.type != QValue::VAL_INT) {
        std::fprintf(stderr, "runtime error: _file_seek() argument 3 must be int\n");
        std::exit(1);
    }

    int cWhence = 0;
    if (whence.data.int_val == 0) {
        cWhence = SEEK_SET;
    } else if (whence.data.int_val == 1) {
        cWhence = SEEK_CUR;
    } else if (whence.data.int_val == 2) {
        cWhence = SEEK_END;
    } else {
        std::fprintf(stderr, "runtime error: _file_seek() whence must be 0, 1, or 2\n");
        std::exit(1);
    }

    QFilePayload* payload = q_expect_file_payload(file, "_file_seek");
    long long newPos = q_file_seek_native(payload->fd, offset.data.int_val, cWhence);
    if (newPos < 0) {
        return q_file_sys_err("seek", nullptr);
    }

    return qv_ok(qv_int(newPos));
}

inline QValue q_file_exists(QValue path) {
    if (path.type != QValue::VAL_STRING || !path.data.string_val) {
        std::fprintf(stderr, "runtime error: _file_exists() argument 1 must be str\n");
        std::exit(1);
    }

    return qv_bool(q_file_exists_native(path.data.string_val));
}

#endif // QUARK_BUILTINS_FILEIO_HPP
