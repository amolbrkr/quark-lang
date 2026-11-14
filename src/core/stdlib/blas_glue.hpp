// blas_glue.hpp - glue for std/blas: CBLAS (OpenBLAS) over Quark f64 vectors.
//
// Quark f64 vectors are contiguous double buffers (Arrow layout), so BLAS
// reads them in place through qext::as_f64; nothing is copied on the way in.
//
// Rules (specs/ffi_v0_1_spec.md §5.3):
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
        fail(fn, "%s is not a valid vec", arg);
    }
    if (qext::vec_dtype(v) != QVector::Type::F64) {
        fail(fn, "%s must be an f64 vec, got %s; convert it with %s.astype('f64')",
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
        fail(fn, "vec size mismatch: x has %d elements, y has %d", nx, ny);
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

// Matrix shapes. Matrices are flat row-major f64 vectors with explicit
// dimensions (Quark has no matrix type), so every routine checks that the
// dimensions are non-negative, fit a CBLAS int, and multiply to the vector's
// length.
namespace qblas {

inline void check_shape(const char* fn, const char* arg, int len, int64_t rows, int64_t cols,
                        const char* rows_name, const char* cols_name) {
    if (rows < 0 || cols < 0) {
        fail(fn, "%s and %s must not be negative, got %s=%lld %s=%lld", rows_name, cols_name,
             rows_name, static_cast<long long>(rows), cols_name, static_cast<long long>(cols));
    }
    if (rows > INT_MAX || cols > INT_MAX) {
        fail(fn, "%s=%lld %s=%lld: BLAS supports dimensions up to %d", rows_name,
             static_cast<long long>(rows), cols_name, static_cast<long long>(cols), INT_MAX);
    }
    if ((cols != 0 && rows > static_cast<int64_t>(len) / cols) || rows * cols != len) {
        fail(fn, "%s has %d elements, but %s * %s = %lld * %lld", arg, len, rows_name, cols_name,
             static_cast<long long>(rows), static_cast<long long>(cols));
    }
}

// check_result stops when a result would have more elements than a BLAS
// vector may (INT_MAX). Inputs are already limited to that, but a product
// with an empty inner dimension (k = 0, or rows = 0 in syrk) has a zero
// result whose size isn't bounded by any input.
inline void check_result(const char* fn, int64_t rows, int64_t cols) {
    if (rows != 0 && cols > static_cast<int64_t>(INT_MAX) / rows) {
        fail(fn, "the result would have %lld x %lld elements; BLAS vectors are limited to %d",
             static_cast<long long>(rows), static_cast<long long>(cols), INT_MAX);
    }
}

// zeros returns a new f64 vector of n zeros.
inline QVector* zeros(int64_t n) {
    QVector* out = qext::new_f64(static_cast<size_t>(n));
    double* d = qext::as_f64_mut(out).data;
    for (int64_t i = 0; i < n; i++) d[i] = 0.0;
    return out;
}

}  // namespace qblas

// a (rows x cols, row-major, flattened) times x, as a new vector of length
// rows; with trans, the transpose of a times x (x has rows elements, the
// result cols).
inline QVector* qblas_gemv(QVector* a, int64_t rows, int64_t cols, QVector* x, bool trans) {
    const char* fn = "blas.gemv";
    int na = qblas::check_f64(fn, "a", a);
    int nx = qblas::check_f64(fn, "x", x);
    qblas::check_shape(fn, "a", na, rows, cols, "rows", "cols");
    const int64_t want = trans ? rows : cols;
    if (nx != want) {
        if (trans) {
            qblas::fail(fn, "x has %d elements, but the matrix has %lld rows (trans multiplies by the transpose)",
                        nx, static_cast<long long>(rows));
        }
        qblas::fail(fn, "x has %d elements, but the matrix has %lld columns", nx, static_cast<long long>(cols));
    }
    const int64_t out_len = trans ? cols : rows;
    if (rows == 0 || cols == 0) {
        return qblas::zeros(out_len);
    }
    QVector* out = qext::new_f64(static_cast<size_t>(out_len));
    cblas_dgemv(CblasRowMajor, trans ? CblasTrans : CblasNoTrans,
                static_cast<int>(rows), static_cast<int>(cols),
                1.0, qext::as_f64(a).data, static_cast<int>(cols),
                qext::as_f64(x).data, 1, 0.0, qext::as_f64_mut(out).data, 1);
    return out;
}

// a (m x k) times b (k x n), both row-major and flattened, as a new m x n
// row-major vector.
inline QVector* qblas_gemm(QVector* a, int64_t m, int64_t k, QVector* b, int64_t n) {
    const char* fn = "blas.gemm";
    int na = qblas::check_f64(fn, "a", a);
    int nb = qblas::check_f64(fn, "b", b);
    qblas::check_shape(fn, "a", na, m, k, "m", "k");
    qblas::check_shape(fn, "b", nb, k, n, "k", "n");
    qblas::check_result(fn, m, n);
    if (m == 0 || n == 0 || k == 0) {
        return qblas::zeros(m * n);
    }
    QVector* out = qext::new_f64(static_cast<size_t>(m * n));
    cblas_dgemm(CblasRowMajor, CblasNoTrans, CblasNoTrans,
                static_cast<int>(m), static_cast<int>(n), static_cast<int>(k),
                1.0, qext::as_f64(a).data, static_cast<int>(k),
                qext::as_f64(b).data, static_cast<int>(n),
                0.0, qext::as_f64_mut(out).data, static_cast<int>(n));
    return out;
}

// The transpose of a times a, for a (rows x cols, row-major, flattened): a
// new cols x cols row-major vector with both triangles filled.
inline QVector* qblas_syrk(QVector* a, int64_t rows, int64_t cols) {
    const char* fn = "blas.syrk";
    int na = qblas::check_f64(fn, "a", a);
    qblas::check_shape(fn, "a", na, rows, cols, "rows", "cols");
    qblas::check_result(fn, cols, cols);
    if (rows == 0 || cols == 0) {
        return qblas::zeros(cols * cols);
    }
    QVector* out = qext::new_f64(static_cast<size_t>(cols * cols));
    double* c = qext::as_f64_mut(out).data;
    const int nc = static_cast<int>(cols);
    cblas_dsyrk(CblasRowMajor, CblasUpper, CblasTrans, nc, static_cast<int>(rows),
                1.0, qext::as_f64(a).data, nc, 0.0, c, nc);
    // dsyrk writes only the upper triangle; mirror it into the lower one.
    for (int64_t i = 0; i < cols; i++) {
        for (int64_t j = 0; j < i; j++) c[i * cols + j] = c[j * cols + i];
    }
    return out;
}

#endif  // QUARK_STDLIB_BLAS_GLUE_HPP
