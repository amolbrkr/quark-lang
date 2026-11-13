// bench_io.hpp - native loaders for the benchmark data files.
//
// The data files are written once by datagen.py and read by every language,
// so all implementations work on the same values. Loading is not timed.
//
//   *.f64  raw little-endian IEEE doubles, no header
//   *.txt  UTF-8 text; read_lines splits it at '\n'
//
// A file that can't be read, or has the wrong size, stops the program with a
// message (a benchmark must never run on partial data).
#ifndef QBENCH_IO_HPP
#define QBENCH_IO_HPP

#include <cerrno>
#include <cstdint>
#include <cstdio>
#include <cstring>

#include "quark/ext/api.hpp"

namespace qbench {

// slurp reads the whole file into GC memory with a terminating NUL.
inline char* slurp(const char* path, size_t* out_len) {
    std::FILE* f = std::fopen(path, "rb");
    if (!f) qext::panicf("bench_io: cannot open %s: %s\n", path, std::strerror(errno));
    if (std::fseek(f, 0, SEEK_END) != 0) qext::panicf("bench_io: cannot seek %s\n", path);
    long size = std::ftell(f);
    if (size < 0) qext::panicf("bench_io: cannot size %s\n", path);
    std::rewind(f);
    char* buf = static_cast<char*>(qext::malloc_atomic(static_cast<size_t>(size) + 1));
    size_t got = std::fread(buf, 1, static_cast<size_t>(size), f);
    std::fclose(f);
    if (got != static_cast<size_t>(size)) qext::panicf("bench_io: short read of %s\n", path);
    buf[size] = '\0';
    *out_len = static_cast<size_t>(size);
    return buf;
}

}  // namespace qbench

// The first n doubles of a raw f64 file as a vec[f64]; all of them when n < 0.
inline QVector* qbench_read_f64(const char* path, int64_t n) {
    std::FILE* f = std::fopen(path, "rb");
    if (!f) qext::panicf("bench_io: cannot open %s: %s\n", path, std::strerror(errno));
    std::fseek(f, 0, SEEK_END);
    long size = std::ftell(f);
    std::rewind(f);
    if (size < 0 || size % 8 != 0) qext::panicf("bench_io: %s is not a whole number of doubles\n", path);
    int64_t avail = size / 8;
    if (n < 0) n = avail;
    if (n > avail) qext::panicf("bench_io: %s has %lld values, %lld requested\n", path,
                                static_cast<long long>(avail), static_cast<long long>(n));
    QVector* v = qext::new_f64(static_cast<size_t>(n));
    size_t got = std::fread(qext::as_f64_mut(v).data, 8, static_cast<size_t>(n), f);
    std::fclose(f);
    if (got != static_cast<size_t>(n)) qext::panicf("bench_io: short read of %s\n", path);
    return v;
}

// The lines of a text file, without their '\n'. A final '\n' doesn't start
// another line.
inline QList* qbench_read_lines(const char* path) {
    size_t len = 0;
    char* buf = qbench::slurp(path, &len);
    QList* out = qext::new_list();
    size_t start = 0;
    for (size_t i = 0; i <= len; i++) {
        if (i == len || buf[i] == '\n') {
            if (i == len && start == len) break;
            buf[i] = '\0';
            qext::list_push(out, qext::box(static_cast<const char*>(buf + start)));
            start = i + 1;
        }
    }
    return out;
}

// The whole text file as one string.
inline const char* qbench_read_text(const char* path) {
    size_t len = 0;
    char* buf = qbench::slurp(path, &len);
    if (std::strlen(buf) != len) qext::panicf("bench_io: %s contains a NUL byte\n", path);
    return buf;
}

#endif  // QBENCH_IO_HPP
