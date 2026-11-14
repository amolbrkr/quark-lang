// stdlib/io.hpp — native implementations for the std/io module (io.qrk).
// Files are resources of kind QRES_KIND_FILE (runtime types/resource.hpp).
#ifndef QUARK_STDLIB_IO_HPP
#define QUARK_STDLIB_IO_HPP

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
    } else if (std::strcmp(mode, "a") == 0) {
        flags = O_WRONLY | O_CREAT | O_APPEND;
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
        q_runtime_reportf("runtime error: %s() expects an open file (%s)\n", op, err ? err : "invalid handle");
        std::exit(1);
    }
    if (!slot || !slot->payload) {
        q_runtime_reportf("runtime error: %s() file has no payload\n", op);
        std::exit(1);
    }
    return static_cast<QFilePayload*>(slot->payload);
}

// Open `path` with mode 'r', 'w' or 'a'. Returns ok(resource) or err(message).
inline QValue q_file_open(const char* path, const char* mode, bool binary) {
    int fd = q_file_open_native(path, mode, binary);
    if (fd == -2) {
        return q_file_err_with_detail("open", "mode must be 'r', 'w' or 'a'");
    }
    if (fd < 0) {
        return q_file_sys_err("open", path);
    }

    QFilePayload* payload = static_cast<QFilePayload*>(q_malloc(sizeof(QFilePayload)));
    if (!payload) {
#ifdef _WIN32
        _close(fd);
#else
        ::close(fd);
#endif
        q_runtime_reportf("runtime error: io.open() failed to allocate payload\n");
        std::exit(1);
    }

    payload->fd = fd;
    payload->binary = binary;

    QResourceHandle* handle = q_resource_create(QRES_KIND_FILE, payload, QRES_FLAG_OWNED | QRES_FLAG_FINALIZABLE);
    if (!handle) {
#ifdef _WIN32
        _close(fd);
#else
        ::close(fd);
#endif
        // payload is GC-managed, no manual free needed
        q_runtime_reportf("runtime error: io.open() failed to allocate resource handle\n");
        std::exit(1);
    }

    return qv_ok(qv_resource(handle));
}

// Read up to `n` bytes. Returns ok(str), which is '' at end of file, or err(message).
inline QValue q_file_read(QValue file, int64_t n) {
    if (n < 0) {
        q_runtime_reportf("runtime error: io.read() argument 2 must be >= 0\n");
        std::exit(1);
    }

    QFilePayload* payload = q_expect_file_payload(file, "io.read");

    const size_t want = static_cast<size_t>(n);
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
    // A str ends at its first NUL byte, so returning the bytes would cut the
    // data short with no sign of it; std/io reads text.
    if (std::memchr(buffer.data(), '\0', buffer.size()) != nullptr) {
        return q_file_err_with_detail("read", "the data contains a NUL byte, which a str can't hold (std/io reads text files)");
    }
    return qv_ok(qv_string(buffer.c_str()));
}

// Write `data`. Returns ok(bytes written) or err(message).
inline QValue q_file_write(QValue file, const char* data) {
    QFilePayload* payload = q_expect_file_payload(file, "io.write");

    const size_t len = std::strlen(data);

#ifdef _WIN32
    int wrote = _write(payload->fd, data, static_cast<unsigned int>(len));
#else
    ssize_t wrote = ::write(payload->fd, data, len);
#endif

    if (wrote < 0) {
        return q_file_sys_err("write", nullptr);
    }

    return qv_ok(qv_int(static_cast<long long>(wrote)));
}

// Close the file. Returns ok(null), or err(message) when it is already closed.
inline QValue q_file_close(QValue file) {
    QResourceSlot* slot = nullptr;
    QResourceHandle* handle = nullptr;
    const char* err = nullptr;
    if (!q_resource_resolve(file, QRES_KIND_FILE, slot, handle, err)) {
        return q_file_err_with_detail("close", err ? err : "not a file");
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

    // payload is GC-managed, no manual free needed
    q_resource_invalidate(handle);
    return qv_ok(qv_null());
}

// Move the file position. whence: 0 = start, 1 = current, 2 = end.
// Returns ok(new position) or err(message).
inline QValue q_file_seek(QValue file, int64_t offset, int64_t whence) {
    int cWhence = 0;
    if (whence == 0) {
        cWhence = SEEK_SET;
    } else if (whence == 1) {
        cWhence = SEEK_CUR;
    } else if (whence == 2) {
        cWhence = SEEK_END;
    } else {
        q_runtime_reportf("runtime error: io.seek() whence must be 0, 1, or 2\n");
        std::exit(1);
    }

    QFilePayload* payload = q_expect_file_payload(file, "io.seek");
    long long newPos = q_file_seek_native(payload->fd, offset, cWhence);
    if (newPos < 0) {
        return q_file_sys_err("seek", nullptr);
    }

    return qv_ok(qv_int(newPos));
}

inline bool q_file_exists(const char* path) {
    return q_file_exists_native(path);
}

#endif // QUARK_STDLIB_IO_HPP
