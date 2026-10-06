// quark/types/vector.hpp - Typed 1D vector runtime kernels
//
// Memory layout (Apache Arrow compatible)
// ---------------------------------------
// A QVector holds one column in the Arrow columnar format, so its buffers can
// be handed to Arrow-speaking libraries (DuckDB, Parquet readers, ...) through
// the Arrow C data interface without copying:
//
//   dtype  Arrow format  buffers
//   F64    "g"           values  = double[count]
//   I64    "l"           values  = int64_t[count]
//   BOOL   "b"           values  = bit-packed bitmap, LSB first
//   STR    "u"           offsets = int32_t[count + 1], values = UTF-8 bytes
//
// `validity` is an Arrow validity bitmap: bit i set means element i is valid,
// LSB first. It is nullptr exactly when null_count == 0, which Arrow allows.
// Values in null slots are unspecified; kernels must not trap on them.
// Every buffer is 64-byte aligned, as Arrow recommends. The Arrow array
// `offset` is always 0 for vectors created by the runtime.
//
// Immutability
// ------------
// Vectors are immutable once Quark code can see them: every operation returns
// a new vector, so buffers may be shared freely. The builder helpers below
// (q_vec_alloc, q_vec_push*, q_vec_str_append*, q_vec_mark_null, and the
// *_mut accessors) may only be used on a vector that was just created and has
// not escaped yet.
#ifndef QUARK_TYPES_VECTOR_HPP
#define QUARK_TYPES_VECTOR_HPP

#include "../core/checked.hpp"
#include "../core/value.hpp"
#include "../core/constructors.hpp"

#include <algorithm>
#include <climits>
#include <cstdio>
#include <cstdint>
#include <cstring>
#include <string>
#include <string_view>
#include <vector>

struct QVector {
    enum class Type { F64, I64, BOOL, STR };

    Type type = Type::F64;
    size_t count = 0;
    size_t null_count = 0;

    uint8_t* validity = nullptr;  // Arrow validity bitmap; nullptr when null_count == 0
    uint8_t* values = nullptr;    // fixed-width values, BOOL bitmap, or STR bytes
    int32_t* offsets = nullptr;   // STR only: count + 1 offsets into values

    // Builder capacities (validity and values in bytes, offsets in elements).
    size_t validity_cap = 0;
    size_t values_cap = 0;
    size_t offsets_cap = 0;

    // Allocation base pointers. The aligned buffer pointers above point inside
    // these allocations; keeping the bases here keeps the buffers reachable for
    // the collector regardless of its interior-pointer settings.
    void* validity_base = nullptr;
    void* values_base = nullptr;
    void* offsets_base = nullptr;

    bool has_nulls() const { return null_count > 0; }
};

// ============================================================
// Buffer and bitmap primitives
// ============================================================
namespace quark {
namespace vec {

constexpr size_t kAlign = 64;

// Allocates a pointer-free, 64-byte aligned buffer of at least `bytes` bytes.
inline uint8_t* alloc(size_t bytes, void** base_out) {
    if (bytes == 0) bytes = 1;
    void* base = q_malloc_atomic(bytes + kAlign);
    if (!base) {
        q_runtime_reportf("runtime error: out of memory allocating vector buffer\n");
        std::exit(1);
    }
    const uintptr_t p = reinterpret_cast<uintptr_t>(base);
    const uintptr_t aligned = (p + kAlign - 1) & ~static_cast<uintptr_t>(kAlign - 1);
    *base_out = base;
    return reinterpret_cast<uint8_t*>(aligned);
}

inline uint8_t* alloc_zeroed(size_t bytes, void** base_out) {
    uint8_t* p = alloc(bytes, base_out);
    std::memset(p, 0, bytes == 0 ? 1 : bytes);
    return p;
}

// Grows a buffer to hold at least `need` bytes, preserving the first `used`
// bytes and zeroing the rest.
inline uint8_t* grow(uint8_t* old, size_t used, size_t& cap, size_t need, void** base_out) {
    if (old != nullptr && need <= cap) return old;
    size_t ncap = cap > 0 ? cap : 64;
    while (ncap < need) ncap *= 2;
    uint8_t* fresh = alloc_zeroed(ncap, base_out);
    if (old != nullptr && used > 0) std::memcpy(fresh, old, used);
    cap = ncap;
    return fresh;
}

inline size_t bitmap_bytes(size_t n) { return (n + 7) / 8; }

inline bool get_bit(const uint8_t* bits, size_t i) {
    return ((bits[i >> 3] >> (i & 7)) & 1u) != 0;
}

inline void put_bit(uint8_t* bits, size_t i, bool v) {
    const uint8_t mask = static_cast<uint8_t>(1u << (i & 7));
    if (v) bits[i >> 3] |= mask;
    else   bits[i >> 3] &= static_cast<uint8_t>(~mask);
}

// Number of set bits among the first n bits.
inline size_t count_set_bits(const uint8_t* bits, size_t n) {
    size_t total = 0;
    const size_t full = n / 8;
    for (size_t k = 0; k < full; k++) total += static_cast<size_t>(__builtin_popcount(bits[k]));
    const size_t rem = n % 8;
    if (rem) total += static_cast<size_t>(__builtin_popcount(bits[full] & ((1u << rem) - 1u)));
    return total;
}

// Packs op(i) for i in [0, n) into a bitmap, eight results per byte.
template <typename F>
inline void fill_bits(uint8_t* out, size_t n, F f) {
    const size_t full = n / 8;
    for (size_t byte = 0; byte < full; byte++) {
        uint8_t bits = 0;
        const size_t base = byte * 8;
        for (unsigned k = 0; k < 8; k++) {
            bits |= static_cast<uint8_t>((f(base + k) ? 1u : 0u) << k);
        }
        out[byte] = bits;
    }
    const size_t rem = n % 8;
    if (rem) {
        uint8_t bits = 0;
        const size_t base = full * 8;
        for (unsigned k = 0; k < rem; k++) {
            bits |= static_cast<uint8_t>((f(base + k) ? 1u : 0u) << k);
        }
        out[full] = bits;
    }
}

inline size_t elem_width(QVector::Type t) {
    return (t == QVector::Type::F64 || t == QVector::Type::I64) ? 8 : 0;
}

} // namespace vec
} // namespace quark

// ============================================================
// Accessors
// ============================================================

inline bool q_vec_has_valid_handle(QValue vec) {
    return vec.type == QValue::VAL_VECTOR && vec.data.vector_val;
}

// O(1) structural check of the layout invariants.
inline bool q_vec_validate(const QVector& vec) {
    switch (vec.type) {
        case QVector::Type::F64:
        case QVector::Type::I64:
        case QVector::Type::BOOL:
            if (vec.values == nullptr) return false;
            break;
        case QVector::Type::STR:
            if (vec.values == nullptr || vec.offsets == nullptr || vec.offsets[0] != 0) return false;
            break;
        default:
            return false;
    }
    if (vec.null_count > vec.count) return false;
    return (vec.validity != nullptr) == (vec.null_count > 0);
}

inline const char* q_vec_dtype_name(const QVector& vec) {
    switch (vec.type) {
        case QVector::Type::F64: return "f64";
        case QVector::Type::I64: return "i64";
        case QVector::Type::BOOL: return "bool";
        case QVector::Type::STR: return "str";
        default: return "unknown";
    }
}

inline bool q_vec_is_type(QValue vec, QVector::Type type) {
    return q_vec_has_valid_handle(vec) && vec.data.vector_val->type == type && q_vec_validate(*vec.data.vector_val);
}

// True for a valid vector of the given dtype with no nulls. Used by codegen
// fast paths that read raw buffers.
inline bool q_vec_is_dense_of(QValue vec, QVector::Type type) {
    return q_vec_is_type(vec, type) && vec.data.vector_val->null_count == 0;
}

inline const double* q_vec_f64_data(const QVector& v) { return reinterpret_cast<const double*>(v.values); }
inline const int64_t* q_vec_i64_data(const QVector& v) { return reinterpret_cast<const int64_t*>(v.values); }
inline double* q_vec_f64_data_mut(QVector& v) { return reinterpret_cast<double*>(v.values); }
inline int64_t* q_vec_i64_data_mut(QVector& v) { return reinterpret_cast<int64_t*>(v.values); }

inline bool q_vec_bool_at(const QVector& v, size_t i) { return quark::vec::get_bit(v.values, i); }
inline void q_vec_bool_put(QVector& v, size_t i, bool b) { quark::vec::put_bit(v.values, i, b); }

inline std::string_view q_vec_str_at(const QVector& v, size_t i) {
    const int32_t start = v.offsets[i];
    const int32_t end = v.offsets[i + 1];
    return std::string_view(reinterpret_cast<const char*>(v.values) + start, static_cast<size_t>(end - start));
}

inline bool q_vec_is_null_at(const QVector& vec, size_t index) {
    return vec.validity != nullptr && index < vec.count && !quark::vec::get_bit(vec.validity, index);
}

// ============================================================
// Builders (only for vectors that have not escaped yet)
// ============================================================

// Allocates a vector of n zeroed elements. STR vectors get n empty strings
// and room for `str_bytes` bytes of string data.
inline QVector* q_vec_alloc(QVector::Type type, size_t n, size_t str_bytes = 0) {
    QVector* v = q_new<QVector>();
    v->type = type;
    v->count = n;
    switch (type) {
        case QVector::Type::F64:
        case QVector::Type::I64:
            v->values_cap = n * 8;
            v->values = quark::vec::alloc_zeroed(v->values_cap, &v->values_base);
            break;
        case QVector::Type::BOOL:
            v->values_cap = quark::vec::bitmap_bytes(n);
            v->values = quark::vec::alloc_zeroed(v->values_cap, &v->values_base);
            break;
        case QVector::Type::STR:
            v->offsets_cap = n + 1;
            v->offsets = reinterpret_cast<int32_t*>(
                quark::vec::alloc_zeroed(v->offsets_cap * sizeof(int32_t), &v->offsets_base));
            v->values_cap = str_bytes;
            v->values = quark::vec::alloc_zeroed(v->values_cap, &v->values_base);
            break;
    }
    return v;
}

inline QValue qv_vector_from(QVector* v) {
    QValue q;
    q.type = QValue::VAL_VECTOR;
    q.data.vector_val = v;
    return q;
}

// Empty builders with a capacity hint, used for vector literals.
inline QValue qv_vector(int initial_cap = 0) {
    QVector* v = q_vec_alloc(QVector::Type::F64, 0);
    v->values = quark::vec::grow(v->values, 0, v->values_cap, static_cast<size_t>(std::max(initial_cap, 0)) * 8, &v->values_base);
    return qv_vector_from(v);
}

inline QValue qv_vector_i64(int initial_cap = 0) {
    QVector* v = q_vec_alloc(QVector::Type::I64, 0);
    v->values = quark::vec::grow(v->values, 0, v->values_cap, static_cast<size_t>(std::max(initial_cap, 0)) * 8, &v->values_base);
    return qv_vector_from(v);
}

inline QValue qv_vector_bool(int initial_cap = 0) {
    QVector* v = q_vec_alloc(QVector::Type::BOOL, 0);
    v->values = quark::vec::grow(v->values, 0, v->values_cap,
                                 quark::vec::bitmap_bytes(static_cast<size_t>(std::max(initial_cap, 0))), &v->values_base);
    return qv_vector_from(v);
}

inline QValue qv_vector_str(int initial_string_cap = 0, int initial_byte_cap = 0) {
    QVector* v = q_vec_alloc(QVector::Type::STR, 0, static_cast<size_t>(std::max(initial_byte_cap, 0)));
    const size_t want = static_cast<size_t>(std::max(initial_string_cap, 0)) + 1;
    if (want > v->offsets_cap) {
        size_t cap_bytes = v->offsets_cap * sizeof(int32_t);
        v->offsets = reinterpret_cast<int32_t*>(quark::vec::grow(
            reinterpret_cast<uint8_t*>(v->offsets), sizeof(int32_t), cap_bytes, want * sizeof(int32_t), &v->offsets_base));
        v->offsets_cap = cap_bytes / sizeof(int32_t);
    }
    return qv_vector_from(v);
}

// Marks element i null. Allocates the validity bitmap on first use.
inline void q_vec_mark_null(QVector& v, size_t i) {
    if (v.validity == nullptr) {
        const size_t cap_elems = std::max(v.count, i + 1);
        v.validity_cap = quark::vec::bitmap_bytes(cap_elems);
        v.validity = quark::vec::alloc(v.validity_cap, &v.validity_base);
        std::memset(v.validity, 0xFF, v.validity_cap);
    }
    if (quark::vec::get_bit(v.validity, i)) {
        quark::vec::put_bit(v.validity, i, false);
        v.null_count++;
    }
}

namespace quark {
namespace vec {

// Makes room for one more element at index v.count, and keeps the validity
// bitmap (if any) covering it as valid.
inline void reserve_one(QVector& v) {
    const size_t n = v.count;
    switch (v.type) {
        case QVector::Type::F64:
        case QVector::Type::I64:
            v.values = grow(v.values, n * 8, v.values_cap, (n + 1) * 8, &v.values_base);
            break;
        case QVector::Type::BOOL:
            v.values = grow(v.values, bitmap_bytes(n), v.values_cap, bitmap_bytes(n + 1), &v.values_base);
            break;
        case QVector::Type::STR: {
            size_t cap_bytes = v.offsets_cap * sizeof(int32_t);
            v.offsets = reinterpret_cast<int32_t*>(grow(reinterpret_cast<uint8_t*>(v.offsets),
                (n + 1) * sizeof(int32_t), cap_bytes, (n + 2) * sizeof(int32_t), &v.offsets_base));
            v.offsets_cap = cap_bytes / sizeof(int32_t);
            break;
        }
    }
    if (v.validity != nullptr) {
        const size_t old_cap = v.validity_cap;
        v.validity = grow(v.validity, bitmap_bytes(n), v.validity_cap, bitmap_bytes(n + 1), &v.validity_base);
        if (v.validity_cap > old_cap) {
            std::memset(v.validity + old_cap, 0xFF, v.validity_cap - old_cap);
        }
        put_bit(v.validity, n, true);
    }
}

} // namespace vec
} // namespace quark

inline bool q_is_numeric_scalar(QValue v) {
    return v.type == QValue::VAL_INT || v.type == QValue::VAL_FLOAT;
}

inline bool q_is_integral_scalar(QValue v) {
    return v.type == QValue::VAL_INT || v.type == QValue::VAL_BOOL;
}

inline bool q_is_boolish_scalar(QValue v) {
    return v.type == QValue::VAL_BOOL || v.type == QValue::VAL_INT;
}

inline double q_to_double_scalar(QValue v) {
    return v.type == QValue::VAL_FLOAT ? v.data.float_val : static_cast<double>(v.data.int_val);
}

inline int64_t q_to_i64_scalar(QValue v) {
    if (v.type == QValue::VAL_BOOL) {
        return v.data.bool_val ? 1 : 0;
    }
    if (v.type == QValue::VAL_FLOAT) {
        return static_cast<int64_t>(v.data.float_val);
    }
    return static_cast<int64_t>(v.data.int_val);
}

inline bool q_boolish_value(QValue v) {
    return v.type == QValue::VAL_BOOL ? v.data.bool_val : (v.data.int_val != 0);
}

inline QValue q_vec_push(QValue vec, QValue value) {
    if (!q_vec_has_valid_handle(vec) || vec.data.vector_val->type != QVector::Type::F64) {
        q_runtime_reportf("runtime error: vector push expects vector[f64]\n");
        std::exit(1);
    }
    if (!q_is_numeric_scalar(value)) {
        q_runtime_reportf("runtime error: vector push expects numeric scalar value\n");
        std::exit(1);
    }
    QVector& v = *vec.data.vector_val;
    quark::vec::reserve_one(v);
    q_vec_f64_data_mut(v)[v.count] = q_to_double_scalar(value);
    v.count++;
    return vec;
}

inline QValue q_vec_push_i64(QValue vec, QValue value) {
    if (!q_vec_has_valid_handle(vec) || vec.data.vector_val->type != QVector::Type::I64) {
        q_runtime_reportf("runtime error: vector push expects vector[i64]\n");
        std::exit(1);
    }
    if (!(value.type == QValue::VAL_INT || value.type == QValue::VAL_FLOAT || value.type == QValue::VAL_BOOL)) {
        q_runtime_reportf("runtime error: vector[i64] push expects int, float, or bool scalar value\n");
        std::exit(1);
    }
    QVector& v = *vec.data.vector_val;
    quark::vec::reserve_one(v);
    q_vec_i64_data_mut(v)[v.count] = q_to_i64_scalar(value);
    v.count++;
    return vec;
}

inline QValue q_vec_push_bool(QValue vec, QValue value) {
    if (!q_vec_has_valid_handle(vec) || vec.data.vector_val->type != QVector::Type::BOOL) {
        q_runtime_reportf("runtime error: vector push expects vector[bool]\n");
        std::exit(1);
    }
    if (!q_is_boolish_scalar(value)) {
        q_runtime_reportf("runtime error: vector[bool] push expects bool or int scalar value\n");
        std::exit(1);
    }
    QVector& v = *vec.data.vector_val;
    quark::vec::reserve_one(v);
    q_vec_bool_put(v, v.count, q_boolish_value(value));
    v.count++;
    return vec;
}

// Appends one string to a STR builder.
inline void q_vec_str_append(QVector& v, const char* s, size_t len) {
    quark::vec::reserve_one(v);
    const size_t used = static_cast<size_t>(v.offsets[v.count]);
    if (used + len > static_cast<size_t>(INT32_MAX)) {
        q_runtime_reportf("runtime error: vector[str] data exceeds 2 GiB\n");
        std::exit(1);
    }
    v.values = quark::vec::grow(v.values, used, v.values_cap, used + len, &v.values_base);
    if (len > 0) std::memcpy(v.values + used, s, len);
    v.offsets[v.count + 1] = static_cast<int32_t>(used + len);
    v.count++;
}

inline void q_vec_str_append_null(QVector& v) {
    q_vec_str_append(v, "", 0);
    q_vec_mark_null(v, v.count - 1);
}

// Copies the validity of `src` onto `dst` (same length).
inline void q_vec_copy_validity(QVector& dst, const QVector& src) {
    if (src.validity == nullptr || src.null_count == 0) {
        dst.validity = nullptr;
        dst.null_count = 0;
        return;
    }
    const size_t nb = quark::vec::bitmap_bytes(dst.count);
    dst.validity_cap = nb;
    dst.validity = quark::vec::alloc(nb, &dst.validity_base);
    std::memcpy(dst.validity, src.validity, nb);
    dst.null_count = src.null_count;
}

// Sets dst validity to the AND of the validity of a and b (either may be
// nullptr for a scalar operand). dst.count must already be set.
inline void q_vec_and_validity(QVector& dst, const QVector* a, const QVector* b) {
    const uint8_t* va = (a && a->null_count > 0) ? a->validity : nullptr;
    const uint8_t* vb = (b && b->null_count > 0) ? b->validity : nullptr;
    if (va == nullptr && vb == nullptr) {
        dst.validity = nullptr;
        dst.null_count = 0;
        return;
    }
    if (va == nullptr || vb == nullptr) {
        q_vec_copy_validity(dst, va ? *a : *b);
        return;
    }
    const size_t nb = quark::vec::bitmap_bytes(dst.count);
    dst.validity_cap = nb;
    dst.validity = quark::vec::alloc(nb, &dst.validity_base);
    for (size_t k = 0; k < nb; k++) dst.validity[k] = static_cast<uint8_t>(va[k] & vb[k]);
    dst.null_count = dst.count - quark::vec::count_set_bits(dst.validity, dst.count);
    if (dst.null_count == 0) dst.validity = nullptr;
}

inline QValue q_vec_clone(QValue vec) {
    if (!q_vec_has_valid_handle(vec) || !q_vec_validate(*vec.data.vector_val)) {
        q_runtime_reportf("runtime error: cannot clone invalid vector\n");
        std::exit(1);
    }
    const QVector& src = *vec.data.vector_val;
    const size_t str_bytes = src.type == QVector::Type::STR ? static_cast<size_t>(src.offsets[src.count]) : 0;
    QVector* out = q_vec_alloc(src.type, src.count, str_bytes);
    switch (src.type) {
        case QVector::Type::F64:
        case QVector::Type::I64:
            std::memcpy(out->values, src.values, src.count * 8);
            break;
        case QVector::Type::BOOL:
            std::memcpy(out->values, src.values, quark::vec::bitmap_bytes(src.count));
            break;
        case QVector::Type::STR:
            std::memcpy(out->offsets, src.offsets, (src.count + 1) * sizeof(int32_t));
            if (str_bytes > 0) std::memcpy(out->values, src.values, str_bytes);
            break;
    }
    q_vec_copy_validity(*out, src);
    return qv_vector_from(out);
}

inline int q_vec_size(QValue vec) {
    if (!q_vec_has_valid_handle(vec) || !q_vec_validate(*vec.data.vector_val)) {
        return 0;
    }
    return static_cast<int>(vec.data.vector_val->count);
}

inline QValue q_vec_dtype(QValue vec) {
    if (!q_vec_has_valid_handle(vec) || !q_vec_validate(*vec.data.vector_val)) {
        q_runtime_reportf("runtime error: dtype query expects valid vector\n");
        std::exit(1);
    }
    return qv_string(q_vec_dtype_name(*vec.data.vector_val));
}

inline bool q_vec_is_numeric_dtype(const QVector& vec) {
    return vec.type == QVector::Type::F64 || vec.type == QVector::Type::I64;
}

inline double q_vec_numeric_as_f64(const QVector& vec, size_t index) {
    if (vec.type == QVector::Type::F64) {
        return q_vec_f64_data(vec)[index];
    }
    return static_cast<double>(q_vec_i64_data(vec)[index]);
}

inline int64_t q_vec_numeric_as_i64(const QVector& vec, size_t index) {
    return q_vec_i64_data(vec)[index];
}

// Boxes element i (null-aware).
inline QValue q_vec_box_at(const QVector& v, size_t i) {
    if (q_vec_is_null_at(v, i)) return qv_null();
    switch (v.type) {
        case QVector::Type::F64: return qv_float(q_vec_f64_data(v)[i]);
        case QVector::Type::I64: return qv_int(static_cast<long long>(q_vec_i64_data(v)[i]));
        case QVector::Type::BOOL: return qv_bool(q_vec_bool_at(v, i));
        case QVector::Type::STR: {
            const std::string_view s = q_vec_str_at(v, i);
            char* buf = static_cast<char*>(q_malloc_atomic(s.size() + 1));
            if (!s.empty()) std::memcpy(buf, s.data(), s.size());
            buf[s.size()] = '\0';
            return qv_string_own(buf);
        }
    }
    return qv_null();
}

// ============================================================
// Operand readers
// ============================================================
// A binary vector kernel takes two operands, each either a vector or a
// scalar. These helpers hand the kernel a typed reader for each operand so
// the inner loop is specialized per operand kind (no per-element dispatch).
namespace quark {
namespace vec {

inline const QVector* as_vec(QValue x) {
    return q_vec_has_valid_handle(x) ? x.data.vector_val : nullptr;
}

template <typename F>
inline void with_f64_reader(QValue x, F&& f) {
    if (const QVector* v = as_vec(x)) {
        if (v->type == QVector::Type::F64) {
            const double* d = q_vec_f64_data(*v);
            f([d](size_t i) { return d[i]; });
        } else {
            const int64_t* d = q_vec_i64_data(*v);
            f([d](size_t i) { return static_cast<double>(d[i]); });
        }
        return;
    }
    const double s = q_to_double_scalar(x);
    f([s](size_t) { return s; });
}

template <typename F>
inline void with_i64_reader(QValue x, F&& f) {
    if (const QVector* v = as_vec(x)) {
        const int64_t* d = q_vec_i64_data(*v);
        f([d](size_t i) { return d[i]; });
        return;
    }
    const int64_t s = q_to_i64_scalar(x);
    f([s](size_t) { return s; });
}

template <typename F>
inline void with_bool_reader(QValue x, F&& f) {
    if (const QVector* v = as_vec(x)) {
        f([v](size_t i) { return q_vec_bool_at(*v, i); });
        return;
    }
    const bool s = q_boolish_value(x);
    f([s](size_t) { return s; });
}

template <typename F>
inline void with_str_reader(QValue x, F&& f) {
    if (const QVector* v = as_vec(x)) {
        f([v](size_t i) { return q_vec_str_at(*v, i); });
        return;
    }
    const std::string_view s(x.data.string_val);
    f([s](size_t) { return s; });
}

inline bool is_vec_of(QValue x, QVector::Type t) {
    const QVector* v = as_vec(x);
    return v && v->type == t && q_vec_validate(*v);
}

inline bool is_numeric_operand(QValue x) {
    return is_vec_of(x, QVector::Type::F64) || is_vec_of(x, QVector::Type::I64) || q_is_numeric_scalar(x);
}

inline bool is_int_operand(QValue x) {
    return is_vec_of(x, QVector::Type::I64) || q_is_integral_scalar(x);
}

inline bool is_bool_operand(QValue x) {
    return is_vec_of(x, QVector::Type::BOOL) || q_is_boolish_scalar(x);
}

inline bool is_str_operand(QValue x) {
    return is_vec_of(x, QVector::Type::STR) || (x.type == QValue::VAL_STRING && x.data.string_val);
}

// Length of a binary operation over a and b (at least one is a vector).
inline size_t binary_len(QValue a, QValue b, const char* what) {
    const QVector* av = as_vec(a);
    const QVector* bv = as_vec(b);
    if (av && bv && av->count != bv->count) {
        q_runtime_reportf("runtime error: vector size mismatch in %s: %zu vs %zu\n", what, av->count, bv->count);
        std::exit(1);
    }
    return av ? av->count : bv->count;
}

} // namespace vec
} // namespace quark

// ============================================================
// Arithmetic
// ============================================================

enum class QVecArithOp { Add, Sub, Mul, Div };

inline QValue q_vec_binary_numeric(QValue a, QValue b, QVecArithOp op) {
    using namespace quark::vec;
    const QVector* av = as_vec(a);
    const QVector* bv = as_vec(b);
    if (!av && !bv) {
        return qv_null();
    }
    for (QValue x : {a, b}) {
        const QVector* xv = as_vec(x);
        if (xv && !q_vec_validate(*xv)) {
            q_runtime_reportf("runtime error: vector arithmetic expects valid vectors\n");
            std::exit(1);
        }
        if (xv ? !q_vec_is_numeric_dtype(*xv) : !q_is_numeric_scalar(x)) {
            q_runtime_reportf("runtime error: vector arithmetic requires numeric vectors and scalars\n");
            std::exit(1);
        }
    }
    if (av && bv && av->count != bv->count) {
        q_runtime_reportf("runtime error: vector size mismatch in arithmetic: %zu vs %zu\n", av->count, bv->count);
        std::exit(1);
    }

    const size_t n = av ? av->count : bv->count;
    const bool wantF64 = op == QVecArithOp::Div ||
                         (av && av->type == QVector::Type::F64) || (bv && bv->type == QVector::Type::F64) ||
                         (!av && a.type == QValue::VAL_FLOAT) || (!bv && b.type == QValue::VAL_FLOAT);

    QVector* out = q_vec_alloc(wantF64 ? QVector::Type::F64 : QVector::Type::I64, n);
    q_vec_and_validity(*out, av, bv);
    const QVector& res = *out;

    if (wantF64) {
        double* od = q_vec_f64_data_mut(*out);
        with_f64_reader(a, [&](auto ra) {
            with_f64_reader(b, [&](auto rb) {
                switch (op) {
                    case QVecArithOp::Add: for (size_t i = 0; i < n; i++) od[i] = ra(i) + rb(i); break;
                    case QVecArithOp::Sub: for (size_t i = 0; i < n; i++) od[i] = ra(i) - rb(i); break;
                    case QVecArithOp::Mul: for (size_t i = 0; i < n; i++) od[i] = ra(i) * rb(i); break;
                    case QVecArithOp::Div:
                        for (size_t i = 0; i < n; i++) {
                            if (q_vec_is_null_at(res, i)) { od[i] = 0.0; continue; }
                            const double y = rb(i);
                            if (y == 0.0) {
                                q_runtime_reportf("runtime error: vector division by zero (f64)\n");
                                std::exit(1);
                            }
                            od[i] = ra(i) / y;
                        }
                        break;
                }
            });
        });
        return qv_vector_from(out);
    }

    // Integer arithmetic is overflow-checked. Null slots hold unspecified
    // values, so they are skipped rather than risk a spurious overflow.
    int64_t* od = q_vec_i64_data_mut(*out);
    const bool anyNull = res.null_count > 0;
    with_i64_reader(a, [&](auto ra) {
        with_i64_reader(b, [&](auto rb) {
            for (size_t i = 0; i < n; i++) {
                if (anyNull && q_vec_is_null_at(res, i)) { od[i] = 0; continue; }
                switch (op) {
                    case QVecArithOp::Add: od[i] = q_checked_add(ra(i), rb(i)); break;
                    case QVecArithOp::Sub: od[i] = q_checked_sub(ra(i), rb(i)); break;
                    case QVecArithOp::Mul: od[i] = q_checked_mul(ra(i), rb(i)); break;
                    case QVecArithOp::Div: od[i] = 0; break;  // Div always takes the f64 path.
                }
            }
        });
    });
    return qv_vector_from(out);
}

inline QValue q_vec_add(QValue a, QValue b) { return q_vec_binary_numeric(a, b, QVecArithOp::Add); }
inline QValue q_vec_sub(QValue a, QValue b) { return q_vec_binary_numeric(a, b, QVecArithOp::Sub); }
inline QValue q_vec_mul(QValue a, QValue b) { return q_vec_binary_numeric(a, b, QVecArithOp::Mul); }
inline QValue q_vec_div(QValue a, QValue b) { return q_vec_binary_numeric(a, b, QVecArithOp::Div); }

// ============================================================
// Reductions (nulls are skipped)
// ============================================================

inline QValue q_vec_sum(QValue vec) {
    if (!q_vec_has_valid_handle(vec) || !q_vec_validate(*vec.data.vector_val)) {
        q_runtime_reportf("runtime error: sum() requires numeric or bool vector\n");
        std::exit(1);
    }
    const QVector& v = *vec.data.vector_val;
    double acc = 0.0;
    switch (v.type) {
        case QVector::Type::I64: {
            const int64_t* d = q_vec_i64_data(v);
            for (size_t i = 0; i < v.count; i++) {
                if (!q_vec_is_null_at(v, i)) acc += static_cast<double>(d[i]);
            }
            break;
        }
        case QVector::Type::F64: {
            const double* d = q_vec_f64_data(v);
            for (size_t i = 0; i < v.count; i++) {
                if (!q_vec_is_null_at(v, i)) acc += d[i];
            }
            break;
        }
        case QVector::Type::BOOL:
            for (size_t i = 0; i < v.count; i++) {
                if (!q_vec_is_null_at(v, i) && q_vec_bool_at(v, i)) acc += 1.0;
            }
            break;
        default:
            q_runtime_reportf("runtime error: sum() requires numeric or bool vector\n");
            std::exit(1);
    }
    if (v.count > 0 && v.null_count == v.count) {
        return qv_null();
    }
    return qv_float(acc);
}

inline QValue q_vec_extremum(QValue vec, bool want_min, const char* name) {
    if (!q_vec_has_valid_handle(vec) || !q_vec_validate(*vec.data.vector_val) ||
        !q_vec_is_numeric_dtype(*vec.data.vector_val)) {
        q_runtime_reportf("runtime error: %s() requires numeric vector\n", name);
        std::exit(1);
    }
    const QVector& v = *vec.data.vector_val;
    if (v.count == 0) {
        q_runtime_reportf("runtime error: %s() on empty vector\n", name);
        std::exit(1);
    }
    bool hasValue = false;
    double cur = 0.0;
    for (size_t i = 0; i < v.count; i++) {
        if (q_vec_is_null_at(v, i)) continue;
        const double x = q_vec_numeric_as_f64(v, i);
        if (!hasValue || (want_min ? x < cur : x > cur)) {
            cur = x;
            hasValue = true;
        }
    }
    return hasValue ? qv_float(cur) : qv_null();
}

inline QValue q_vec_min(QValue vec) { return q_vec_extremum(vec, true, "min"); }
inline QValue q_vec_max(QValue vec) { return q_vec_extremum(vec, false, "max"); }

// ============================================================
// fillna / astype (both return new vectors)
// ============================================================

inline QValue q_fillna(QValue vec, QValue value) {
    if (!q_vec_has_valid_handle(vec) || !q_vec_validate(*vec.data.vector_val)) {
        q_runtime_reportf("runtime error: fillna() expects valid vector as first argument\n");
        std::exit(1);
    }
    const QVector& src = *vec.data.vector_val;
    if (src.null_count == 0) {
        return vec;  // immutable: sharing the same vector is safe
    }

    switch (src.type) {
        case QVector::Type::F64: {
            if (!q_is_numeric_scalar(value)) {
                q_runtime_reportf("runtime error: fillna() value is incompatible with vector[f64]\n");
                std::exit(1);
            }
            QValue out = q_vec_clone(vec);
            double* d = q_vec_f64_data_mut(*out.data.vector_val);
            const double fill = q_to_double_scalar(value);
            for (size_t i = 0; i < src.count; i++) if (q_vec_is_null_at(src, i)) d[i] = fill;
            out.data.vector_val->validity = nullptr;
            out.data.vector_val->null_count = 0;
            return out;
        }
        case QVector::Type::I64: {
            if (!(value.type == QValue::VAL_INT || value.type == QValue::VAL_FLOAT || value.type == QValue::VAL_BOOL)) {
                q_runtime_reportf("runtime error: fillna() value is incompatible with vector[i64]\n");
                std::exit(1);
            }
            QValue out = q_vec_clone(vec);
            int64_t* d = q_vec_i64_data_mut(*out.data.vector_val);
            const int64_t fill = q_to_i64_scalar(value);
            for (size_t i = 0; i < src.count; i++) if (q_vec_is_null_at(src, i)) d[i] = fill;
            out.data.vector_val->validity = nullptr;
            out.data.vector_val->null_count = 0;
            return out;
        }
        case QVector::Type::BOOL: {
            if (!q_is_boolish_scalar(value)) {
                q_runtime_reportf("runtime error: fillna() value is incompatible with vector[bool]\n");
                std::exit(1);
            }
            QValue out = q_vec_clone(vec);
            const bool fill = q_boolish_value(value);
            for (size_t i = 0; i < src.count; i++) {
                if (q_vec_is_null_at(src, i)) q_vec_bool_put(*out.data.vector_val, i, fill);
            }
            out.data.vector_val->validity = nullptr;
            out.data.vector_val->null_count = 0;
            return out;
        }
        case QVector::Type::STR: {
            if (value.type != QValue::VAL_STRING || value.data.string_val == nullptr) {
                q_runtime_reportf("runtime error: fillna() value is incompatible with vector[str]\n");
                std::exit(1);
            }
            const std::string_view fill(value.data.string_val);
            QValue out = qv_vector_str(static_cast<int>(src.count), 0);
            QVector& ov = *out.data.vector_val;
            for (size_t i = 0; i < src.count; i++) {
                const std::string_view s = q_vec_is_null_at(src, i) ? fill : q_vec_str_at(src, i);
                q_vec_str_append(ov, s.data(), s.size());
            }
            return out;
        }
    }
    q_runtime_reportf("runtime error: fillna() unsupported vector dtype\n");
    std::exit(1);
}

inline QValue q_astype(QValue vec, QValue dtype) {
    if (!q_vec_has_valid_handle(vec) || !q_vec_validate(*vec.data.vector_val)) {
        q_runtime_reportf("runtime error: astype() expects valid vector as first argument\n");
        std::exit(1);
    }
    if (dtype.type != QValue::VAL_STRING || dtype.data.string_val == nullptr) {
        q_runtime_reportf("runtime error: astype() expects dtype string as second argument\n");
        std::exit(1);
    }

    const QVector& src = *vec.data.vector_val;
    const char* target = dtype.data.string_val;
    const size_t n = src.count;

    if (std::strcmp(target, "f64") == 0) {
        if (src.type == QVector::Type::F64) return q_vec_clone(vec);
        if (src.type != QVector::Type::I64 && src.type != QVector::Type::BOOL) {
            q_runtime_reportf("runtime error: astype() cannot cast source vector to f64\n");
            std::exit(1);
        }
        QVector* out = q_vec_alloc(QVector::Type::F64, n);
        double* d = q_vec_f64_data_mut(*out);
        if (src.type == QVector::Type::I64) {
            const int64_t* in = q_vec_i64_data(src);
            for (size_t i = 0; i < n; i++) d[i] = static_cast<double>(in[i]);
        } else {
            for (size_t i = 0; i < n; i++) d[i] = q_vec_bool_at(src, i) ? 1.0 : 0.0;
        }
        q_vec_copy_validity(*out, src);
        return qv_vector_from(out);
    }

    if (std::strcmp(target, "i64") == 0) {
        if (src.type == QVector::Type::I64) return q_vec_clone(vec);
        if (src.type != QVector::Type::F64 && src.type != QVector::Type::BOOL) {
            q_runtime_reportf("runtime error: astype() cannot cast source vector to i64\n");
            std::exit(1);
        }
        QVector* out = q_vec_alloc(QVector::Type::I64, n);
        int64_t* d = q_vec_i64_data_mut(*out);
        if (src.type == QVector::Type::F64) {
            const double* in = q_vec_f64_data(src);
            for (size_t i = 0; i < n; i++) {
                if (q_vec_is_null_at(src, i)) { d[i] = 0; continue; }
                const double x = in[i];
                // Casting NaN or an out-of-range double to int64 is undefined.
                if (!(x >= -9223372036854775808.0 && x < 9223372036854775808.0)) {
                    q_runtime_reportf("runtime error: astype('i64'): value %g at index %zu is out of i64 range\n", x, i);
                    std::exit(1);
                }
                d[i] = static_cast<int64_t>(x);
            }
        } else {
            for (size_t i = 0; i < n; i++) d[i] = q_vec_bool_at(src, i) ? 1 : 0;
        }
        q_vec_copy_validity(*out, src);
        return qv_vector_from(out);
    }

    if (std::strcmp(target, "bool") == 0) {
        if (src.type == QVector::Type::BOOL) return q_vec_clone(vec);
        if (src.type != QVector::Type::F64 && src.type != QVector::Type::I64) {
            q_runtime_reportf("runtime error: astype() cannot cast source vector to bool\n");
            std::exit(1);
        }
        QVector* out = q_vec_alloc(QVector::Type::BOOL, n);
        if (src.type == QVector::Type::F64) {
            const double* in = q_vec_f64_data(src);
            quark::vec::fill_bits(out->values, n, [in](size_t i) { return in[i] != 0.0; });
        } else {
            const int64_t* in = q_vec_i64_data(src);
            quark::vec::fill_bits(out->values, n, [in](size_t i) { return in[i] != 0; });
        }
        q_vec_copy_validity(*out, src);
        return qv_vector_from(out);
    }

    q_runtime_reportf("runtime error: astype() unsupported target dtype '%s'\n", target);
    std::exit(1);
}

// ============================================================
// List <-> vector conversion
// ============================================================

inline QValue q_to_vector(QValue input) {
    if (q_vec_has_valid_handle(input) && q_vec_validate(*input.data.vector_val)) {
        return q_vec_clone(input);
    }

    if (input.type != QValue::VAL_LIST || !input.data.list_val) {
        q_runtime_reportf("runtime error: to_vector expects list or vector input\n");
        std::exit(1);
    }

    const QList& items = *input.data.list_val;
    const size_t n = items.size();

    enum class Mode { UNKNOWN, I64, F64, BOOL, STR, INVALID };
    Mode mode = Mode::UNKNOWN;

    auto type_name = [](QValue::ValueType t) -> const char* {
        switch (t) {
            case QValue::VAL_INT: return "int";
            case QValue::VAL_FLOAT: return "float";
            case QValue::VAL_STRING: return "str";
            case QValue::VAL_BOOL: return "bool";
            case QValue::VAL_NULL: return "null";
            case QValue::VAL_LIST: return "list";
            case QValue::VAL_VECTOR: return "vector";
            case QValue::VAL_DICT: return "dict";
            case QValue::VAL_FUNC: return "fn";
            case QValue::VAL_RESULT: return "result";
            case QValue::VAL_RESOURCE: return "resource";
            default: return "unknown";
        }
    };

    size_t str_bytes = 0;
    for (size_t i = 0; i < n; i++) {
        const QValue& item = items[i];
        if (item.type == QValue::VAL_NULL) {
            continue;
        }
        Mode want = Mode::INVALID;
        switch (item.type) {
            case QValue::VAL_INT: want = Mode::I64; break;
            case QValue::VAL_FLOAT: want = Mode::F64; break;
            case QValue::VAL_BOOL: want = Mode::BOOL; break;
            case QValue::VAL_STRING:
                want = Mode::STR;
                if (item.data.string_val) str_bytes += std::strlen(item.data.string_val);
                break;
            default:
                q_runtime_reportf("runtime error: to_vector only supports int/float/bool/str lists (null allowed), got %s at index %zu\n", type_name(item.type), i);
                std::exit(1);
        }
        if (mode == Mode::UNKNOWN) {
            mode = want;
        } else if (mode != want) {
            q_runtime_reportf("runtime error: to_vector requires homogeneous element types (all int, all float, all bool, or all str)\n");
            std::exit(1);
        }
    }

    if (mode == Mode::UNKNOWN) {
        mode = Mode::I64;
    }

    if (mode == Mode::STR) {
        if (str_bytes > static_cast<size_t>(INT32_MAX)) {
            q_runtime_reportf("runtime error: vector[str] data exceeds 2 GiB\n");
            std::exit(1);
        }
        QVector* out = q_vec_alloc(QVector::Type::STR, n, str_bytes);
        int32_t pos = 0;
        for (size_t i = 0; i < n; i++) {
            const QValue& item = items[i];
            if (item.type == QValue::VAL_STRING && item.data.string_val) {
                const size_t len = std::strlen(item.data.string_val);
                if (len > 0) std::memcpy(out->values + pos, item.data.string_val, len);
                pos += static_cast<int32_t>(len);
            } else {
                q_vec_mark_null(*out, i);
            }
            out->offsets[i + 1] = pos;
        }
        return qv_vector_from(out);
    }

    const QVector::Type type = mode == Mode::F64 ? QVector::Type::F64
                             : mode == Mode::BOOL ? QVector::Type::BOOL
                             : QVector::Type::I64;
    QVector* out = q_vec_alloc(type, n);
    for (size_t i = 0; i < n; i++) {
        const QValue& item = items[i];
        if (item.type == QValue::VAL_NULL) {
            q_vec_mark_null(*out, i);
            continue;
        }
        switch (type) {
            case QVector::Type::F64: q_vec_f64_data_mut(*out)[i] = item.data.float_val; break;
            case QVector::Type::I64: q_vec_i64_data_mut(*out)[i] = static_cast<int64_t>(item.data.int_val); break;
            case QVector::Type::BOOL: q_vec_bool_put(*out, i, item.data.bool_val); break;
            case QVector::Type::STR: break;
        }
    }
    return qv_vector_from(out);
}

inline QValue q_to_list(QValue input) {
    if (input.type == QValue::VAL_LIST) {
        return input;
    }
    if (!q_vec_has_valid_handle(input)) {
        q_runtime_reportf("runtime error: to_list expects a vector or list input\n");
        std::exit(1);
    }
    const QVector& v = *input.data.vector_val;
    QValue out = qv_list(static_cast<int>(v.count));
    QList& items = *out.data.list_val;
    items.reserve(v.count);
    for (size_t i = 0; i < v.count; i++) {
        items.push_back(q_vec_box_at(v, i));
    }
    return out;
}

// ============================================================
// Comparisons (produce vector[bool]; nulls propagate)
// ============================================================

namespace quark {
namespace vec {

template <typename ReaderA, typename ReaderB, typename Op>
inline QValue cmp_finish(QValue a, QValue b, size_t n, ReaderA ra, ReaderB rb, Op op) {
    QVector* out = q_vec_alloc(QVector::Type::BOOL, n);
    q_vec_and_validity(*out, as_vec(a), as_vec(b));
    fill_bits(out->values, n, [&](size_t i) { return op(ra(i), rb(i)); });
    return qv_vector_from(out);
}

template <typename Op>
inline QValue cmp_numeric(QValue a, QValue b, Op op) {
    const size_t n = binary_len(a, b, "comparison");
    QValue result = qv_null();
    if (is_int_operand(a) && is_int_operand(b)) {
        with_i64_reader(a, [&](auto ra) {
            with_i64_reader(b, [&](auto rb) { result = cmp_finish(a, b, n, ra, rb, op); });
        });
    } else {
        with_f64_reader(a, [&](auto ra) {
            with_f64_reader(b, [&](auto rb) { result = cmp_finish(a, b, n, ra, rb, op); });
        });
    }
    return result;
}

template <typename Op>
inline QValue cmp_bool(QValue a, QValue b, Op op) {
    const size_t n = binary_len(a, b, "comparison");
    QValue result = qv_null();
    with_bool_reader(a, [&](auto ra) {
        with_bool_reader(b, [&](auto rb) { result = cmp_finish(a, b, n, ra, rb, op); });
    });
    return result;
}

template <typename Op>
inline QValue cmp_str(QValue a, QValue b, Op op) {
    const size_t n = binary_len(a, b, "comparison");
    QValue result = qv_null();
    with_str_reader(a, [&](auto ra) {
        with_str_reader(b, [&](auto rb) { result = cmp_finish(a, b, n, ra, rb, op); });
    });
    return result;
}

[[noreturn]] inline void cmp_unsupported(const char* op) {
    q_runtime_reportf("runtime error: operator '%s' not supported for these vector types\n", op);
    std::exit(1);
}

template <typename Op>
inline QValue cmp_ordered(QValue a, QValue b, Op op, const char* name) {
    if (is_numeric_operand(a) && is_numeric_operand(b)) return cmp_numeric(a, b, op);
    cmp_unsupported(name);
}

template <typename Op>
inline QValue cmp_equality(QValue a, QValue b, Op op, const char* name) {
    if (is_numeric_operand(a) && is_numeric_operand(b)) return cmp_numeric(a, b, op);
    if (is_bool_operand(a) && is_bool_operand(b)) return cmp_bool(a, b, op);
    if (is_str_operand(a) && is_str_operand(b)) return cmp_str(a, b, op);
    cmp_unsupported(name);
}

} // namespace vec
} // namespace quark

inline QValue q_vec_lt(QValue a, QValue b)  { return quark::vec::cmp_ordered(a, b, [](auto x, auto y) { return x < y; }, "<"); }
inline QValue q_vec_lte(QValue a, QValue b) { return quark::vec::cmp_ordered(a, b, [](auto x, auto y) { return x <= y; }, "<="); }
inline QValue q_vec_gt(QValue a, QValue b)  { return quark::vec::cmp_ordered(a, b, [](auto x, auto y) { return x > y; }, ">"); }
inline QValue q_vec_gte(QValue a, QValue b) { return quark::vec::cmp_ordered(a, b, [](auto x, auto y) { return x >= y; }, ">="); }
inline QValue q_vec_eq(QValue a, QValue b)  { return quark::vec::cmp_equality(a, b, [](auto x, auto y) { return x == y; }, "=="); }
inline QValue q_vec_neq(QValue a, QValue b) { return quark::vec::cmp_equality(a, b, [](auto x, auto y) { return x != y; }, "!="); }

// ============================================================
// Indexing and boolean mask filtering
// ============================================================

inline QValue q_vec_get_scalar(QValue vec, QValue index) {
    if (!q_vec_has_valid_handle(vec)) {
        q_runtime_reportf("runtime error: vector index expects valid vector\n");
        std::exit(1);
    }
    if (index.type != QValue::VAL_INT) {
        q_runtime_reportf("runtime error: vector index must be int\n");
        std::exit(1);
    }
    const QVector& v = *vec.data.vector_val;
    long long idx = index.data.int_val;
    const long long len = static_cast<long long>(v.count);
    if (idx < 0) idx = len + idx;
    if (idx < 0 || idx >= len) {
        q_runtime_reportf("runtime error: vector index %lld out of range (len=%lld)\n", idx, len);
        std::exit(1);
    }
    return q_vec_box_at(v, static_cast<size_t>(idx));
}

// True when mask element i selects a row: valid and true.
inline bool q_vec_mask_selects(const QVector& mask, size_t i) {
    return !q_vec_is_null_at(mask, i) && q_vec_bool_at(mask, i);
}

inline size_t q_vec_mask_count(const QVector& mask) {
    size_t selected = 0;
    for (size_t i = 0; i < mask.count; i++) {
        if (q_vec_mask_selects(mask, i)) selected++;
    }
    return selected;
}

inline QValue q_vec_mask_filter(QValue data, QValue mask) {
    if (!q_vec_has_valid_handle(data) || !q_vec_has_valid_handle(mask)) {
        q_runtime_reportf("runtime error: mask filter expects vector operands\n");
        std::exit(1);
    }
    const QVector& dv = *data.data.vector_val;
    const QVector& mv = *mask.data.vector_val;
    if (mv.type != QVector::Type::BOOL) {
        q_runtime_reportf("runtime error: mask index must be a bool vector, got vector[%s]\n", q_vec_dtype_name(mv));
        std::exit(1);
    }
    if (dv.count != mv.count) {
        q_runtime_reportf("runtime error: mask length (%zu) does not match vector length (%zu)\n", mv.count, dv.count);
        std::exit(1);
    }

    const size_t n = dv.count;
    const size_t selected = q_vec_mask_count(mv);
    size_t str_bytes = 0;
    if (dv.type == QVector::Type::STR) {
        for (size_t i = 0; i < n; i++) {
            if (q_vec_mask_selects(mv, i)) str_bytes += q_vec_str_at(dv, i).size();
        }
    }

    QVector* out = q_vec_alloc(dv.type, selected, str_bytes);
    size_t j = 0;
    int32_t pos = 0;
    for (size_t i = 0; i < n; i++) {
        if (!q_vec_mask_selects(mv, i)) continue;
        switch (dv.type) {
            case QVector::Type::F64: q_vec_f64_data_mut(*out)[j] = q_vec_f64_data(dv)[i]; break;
            case QVector::Type::I64: q_vec_i64_data_mut(*out)[j] = q_vec_i64_data(dv)[i]; break;
            case QVector::Type::BOOL: q_vec_bool_put(*out, j, q_vec_bool_at(dv, i)); break;
            case QVector::Type::STR: {
                const std::string_view s = q_vec_str_at(dv, i);
                if (!s.empty()) std::memcpy(out->values + pos, s.data(), s.size());
                pos += static_cast<int32_t>(s.size());
                out->offsets[j + 1] = pos;
                break;
            }
        }
        if (q_vec_is_null_at(dv, i)) q_vec_mark_null(*out, j);
        j++;
    }
    return qv_vector_from(out);
}

#endif // QUARK_TYPES_VECTOR_HPP
