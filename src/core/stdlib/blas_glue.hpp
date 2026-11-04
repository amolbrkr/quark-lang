// blas_glue.hpp - glue for std/blas: CBLAS (OpenBLAS) over Quark f64 vectors.
//
// Quark f64 vectors are contiguous double buffers (Arrow layout), so BLAS
// reads them in place through qext::as_f64; nothing is copied on the way in.
//
// Rules (ffi_v0_1_spec.md §5.3):
//   - Inputs must be f64 vectors without nulls. Anything else is a
//     programming error and stops the program with a message.
//   - Vectors are immutable: routines that write their output in place
//     (daxpy, dscal) run on a fresh copy, which is returned.
//   - Lengths are passed to CBLAS as int, so vectors are limited to INT_MAX
//     elements.
#ifndef QUARK_STDLIB_BLAS_GLUE_HPP
#define QUARK_STDLIB_BLAS_GLUE_HPP

#include <cblas.h>

#include <climits>
#include <cstdarg>
#include <cstdint>
#include <cstdio>

#include "quark/ext/api.hpp"

namespace qblas {

[[noreturn]] inline void fail(const char* fn, const char* fmt, ...) {
    char msg[512];
    int n = std::snprintf(msg, sizeof(msg), "%s: ", fn);
    va_list args;
    va_start(args, fmt);
    std::vsnprintf(msg + n, sizeof(msg) - static_cast<size_t>(n), fmt, args);
    va_end(args);
    qext::panic(msg);
}

// check_f64 validates a vector argument and returns its length as a CBLAS int.
inline int check_f64(const char* fn, const char* arg, const QVector* v) {
    if (v == nullptr) {
        fail(fn, "%s is not a valid vector", arg);
    }
    if (qext::vec_dtype(v) != QVector::Type::F64) {
        fail(fn, "%s must be an f64 vector, got %s; convert it with %s.astype('f64')",
             arg, qext::vec_dtype_name(v), arg);
    }
    if (qext::vec_has_nulls(v)) {
        fail(fn, "%s has %zu null values; replace them first with %s.fillna(...)",
             arg, qext::vec_null_count(v), arg);
    }
    if (qext::vec_size(v) > static_cast<size_t>(INT_MAX)) {
        fail(fn, "%s has %zu elements; BLAS supports at most %d", arg, qext::vec_size(v), INT_MAX);
    }
    return static_cast<int>(qext::vec_size(v));
}

inline void check_same_length(const char* fn, int nx, int ny) {
    if (nx != ny) {
        fail(fn, "vector size mismatch: x has %d elements, y has %d", nx, ny);
    }
}

// copy_of returns a new vector with the same values as v.
inline QVector* copy_of(QVector* v) {
    return qext::as_vector(q_vec_clone(qv_vector_ptr(v)));
}

}  // namespace qblas

inline double qblas_dot(QVector* x, QVector* y) {
    int n = qblas::check_f64("blas.dot", "x", x);
    qblas::check_same_length("blas.dot", n, qblas::check_f64("blas.dot", "y", y));
    return cblas_ddot(n, qext::as_f64(x).data, 1, qext::as_f64(y).data, 1);
}

// a*x + y, as a new vector.
inline QVector* qblas_axpy(double a, QVector* x, QVector* y) {
    int n = qblas::check_f64("blas.axpy", "x", x);
    qblas::check_same_length("blas.axpy", n, qblas::check_f64("blas.axpy", "y", y));
    QVector* out = qblas::copy_of(y);
    cblas_daxpy(n, a, qext::as_f64(x).data, 1, qext::as_f64_mut(out).data, 1);
    return out;
}

// a*x, as a new vector.
inline QVector* qblas_scal(double a, QVector* x) {
    int n = qblas::check_f64("blas.scal", "x", x);
    QVector* out = qblas::copy_of(x);
    cblas_dscal(n, a, qext::as_f64_mut(out).data, 1);
    return out;
}

inline double qblas_nrm2(QVector* x) {
    int n = qblas::check_f64("blas.nrm2", "x", x);
    return cblas_dnrm2(n, qext::as_f64(x).data, 1);
}

inline double qblas_asum(QVector* x) {
    int n = qblas::check_f64("blas.asum", "x", x);
    return cblas_dasum(n, qext::as_f64(x).data, 1);
}

// a (rows x cols, row-major, flattened) times x, as a new vector of length rows.
inline QVector* qblas_gemv(QVector* a, int64_t rows, int64_t cols, QVector* x) {
    const char* fn = "blas.gemv";
    int na = qblas::check_f64(fn, "a", a);
    int nx = qblas::check_f64(fn, "x", x);
    if (rows < 0 || cols < 0) {
        qblas::fail(fn, "rows and cols must not be negative, got rows=%lld cols=%lld",
                    static_cast<long long>(rows), static_cast<long long>(cols));
    }
    if (rows > INT_MAX || cols > INT_MAX ||
        (cols != 0 && rows > static_cast<int64_t>(na) / cols) || rows * cols != na) {
        qblas::fail(fn, "a has %d elements, but rows * cols = %lld * %lld",
                    na, static_cast<long long>(rows), static_cast<long long>(cols));
    }
    if (nx != cols) {
        qblas::fail(fn, "x has %d elements, but the matrix has %lld columns", nx, static_cast<long long>(cols));
    }
    QVector* out = qext::new_f64(static_cast<size_t>(rows));
    if (rows == 0) {
        return out;
    }
    double* y = qext::as_f64_mut(out).data;
    if (cols == 0) {
        for (int64_t i = 0; i < rows; i++) y[i] = 0.0;
        return out;
    }
    cblas_dgemv(CblasRowMajor, CblasNoTrans, static_cast<int>(rows), static_cast<int>(cols),
                1.0, qext::as_f64(a).data, static_cast<int>(cols),
                qext::as_f64(x).data, 1, 0.0, y, 1);
    return out;
}

#endif  // QUARK_STDLIB_BLAS_GLUE_HPP
