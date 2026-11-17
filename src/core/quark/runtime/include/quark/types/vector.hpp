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
#include "../core/numcmp.hpp"
#include "../core/value.hpp"
#include "../core/constructors.hpp"
#include "../core/format.hpp"
#include "../core/sum.hpp"

#include <algorithm>
#include <climits>
#include <cmath>
#include <cstdio>
#include <cstdint>
#include <cstring>
#include <memory>
#include <vector>
#include <string>
#include <string_view>
#include <type_traits>
#include <vector>

struct QVector {
    enum class Type { F64, I64, BOOL, STR, DATE };

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
// QUARK_ALWAYS_INLINE forces a small predicate inline into the loop that
// calls it per element.
#if defined(__clang__) || defined(__GNUC__)
#define QUARK_ALWAYS_INLINE __attribute__((always_inline))
#else
#define QUARK_ALWAYS_INLINE
#endif

namespace quark {
namespace vec {

constexpr size_t kAlign = 64;

// Allocates a pointer-free, 64-byte aligned buffer of at least `bytes` bytes.
inline uint8_t* alloc(size_t bytes, void** base_out) {
    if (bytes == 0) bytes = 1;
    void* base = q_malloc_atomic(bytes + kAlign);
    if (!base) {
        q_runtime_reportf("runtime error: out of memory allocating vec buffer\n");
        q_exit(1);
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
inline void fill_bits_seq(uint8_t* out, size_t n, F f) {
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

// par_range runs f(lo, hi) over the blocks of [0, n) on the thread pool
// (core/par.hpp): a large range is split in blocks of q_par_block entries,
// which start on a byte of a bitmap. f must write only its own entries.
template <class F>
inline void par_range(size_t n, F&& f) {
    const quark::par::Plan p = q_par_plan(static_cast<long long>(n));
    q_par_run(p, [&](long long lo, long long hi, long long) { f(static_cast<size_t>(lo), static_cast<size_t>(hi)); });
}

// fill_bits is fill_bits_seq, a block of the range at a time.
template <typename F>
inline void fill_bits(uint8_t* out, size_t n, F f) {
    par_range(n, [&](size_t lo, size_t hi) {
        fill_bits_seq(out + lo / 8, hi - lo, [&](size_t j) { return f(lo + j); });
    });
}

inline size_t elem_width(QVector::Type t) {
    if (t == QVector::Type::F64 || t == QVector::Type::I64) return 8;
    if (t == QVector::Type::DATE) return 4;
    return 0;
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
        case QVector::Type::DATE:
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
        case QVector::Type::DATE: return "date";
        default: return "unknown";
    }
}

inline bool q_vec_is_type(QValue vec, QVector::Type type) {
    return q_vec_has_valid_handle(vec) && vec.data.vector_val->type == type && q_vec_validate(*vec.data.vector_val);
}

// True for a valid vector of the given dtype with no nulls. Used by generated code
// fast paths that read raw buffers.
inline bool q_vec_is_dense_of(QValue vec, QVector::Type type) {
    return q_vec_is_type(vec, type) && vec.data.vector_val->null_count == 0;
}

inline const double* q_vec_f64_data(const QVector& v) { return reinterpret_cast<const double*>(v.values); }
inline const int64_t* q_vec_i64_data(const QVector& v) { return reinterpret_cast<const int64_t*>(v.values); }
inline const int32_t* q_vec_date_data(const QVector& v) { return reinterpret_cast<const int32_t*>(v.values); }
inline int32_t* q_vec_date_data_mut(QVector& v) { return reinterpret_cast<int32_t*>(v.values); }
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
        case QVector::Type::DATE:
            v->values_cap = n * 4;
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

// q_vec_alloc_raw is q_vec_alloc for an F64, I64 or DATE vector whose every
// element the caller writes: the values aren't zeroed first (a sequential
// write of the whole buffer that a parallel kernel would wait for). Other
// types are zeroed as usual.
inline QVector* q_vec_alloc_raw(QVector::Type type, size_t n) {
    if (type != QVector::Type::F64 && type != QVector::Type::I64 && type != QVector::Type::DATE) return q_vec_alloc(type, n);
    QVector* v = q_new<QVector>();
    v->type = type;
    v->count = n;
    v->values_cap = n * quark::vec::elem_width(type);
    v->values = quark::vec::alloc(v->values_cap, &v->values_base);
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
        case QVector::Type::DATE:
            v.values = grow(v.values, n * 4, v.values_cap, (n + 1) * 4, &v.values_base);
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
        q_runtime_reportf("runtime error: vec push expects vec[f64]\n");
        q_exit(1);
    }
    if (!q_is_numeric_scalar(value)) {
        q_runtime_reportf("runtime error: vec push expects numeric scalar value\n");
        q_exit(1);
    }
    QVector& v = *vec.data.vector_val;
    quark::vec::reserve_one(v);
    q_vec_f64_data_mut(v)[v.count] = q_to_double_scalar(value);
    v.count++;
    return vec;
}

inline QValue q_vec_push_i64(QValue vec, QValue value) {
    if (!q_vec_has_valid_handle(vec) || vec.data.vector_val->type != QVector::Type::I64) {
        q_runtime_reportf("runtime error: vec push expects vec[i64]\n");
        q_exit(1);
    }
    if (!(value.type == QValue::VAL_INT || value.type == QValue::VAL_FLOAT || value.type == QValue::VAL_BOOL)) {
        q_runtime_reportf("runtime error: vec[i64] push expects int, float, or bool scalar value\n");
        q_exit(1);
    }
    QVector& v = *vec.data.vector_val;
    quark::vec::reserve_one(v);
    q_vec_i64_data_mut(v)[v.count] = q_to_i64_scalar(value);
    v.count++;
    return vec;
}

inline QValue q_vec_push_bool(QValue vec, QValue value) {
    if (!q_vec_has_valid_handle(vec) || vec.data.vector_val->type != QVector::Type::BOOL) {
        q_runtime_reportf("runtime error: vec push expects vec[bool]\n");
        q_exit(1);
    }
    if (!q_is_boolish_scalar(value)) {
        q_runtime_reportf("runtime error: vec[bool] push expects bool or int scalar value\n");
        q_exit(1);
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
        q_runtime_reportf("runtime error: vec[str] data exceeds 2 GiB\n");
        q_exit(1);
    }
    v.values = quark::vec::grow(v.values, used, v.values_cap, used + len, &v.values_base);
    if (len > 0) std::memcpy(v.values + used, s, len);
    v.offsets[v.count + 1] = static_cast<int32_t>(used + len);
    v.count++;
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
        q_runtime_reportf("runtime error: cannot clone invalid vec\n");
        q_exit(1);
    }
    const QVector& src = *vec.data.vector_val;
    const size_t str_bytes = src.type == QVector::Type::STR ? static_cast<size_t>(src.offsets[src.count]) : 0;
    QVector* out = q_vec_alloc(src.type, src.count, str_bytes);
    switch (src.type) {
        case QVector::Type::F64:
        case QVector::Type::I64:
            std::memcpy(out->values, src.values, src.count * 8);
            break;
        case QVector::Type::DATE:
            std::memcpy(out->values, src.values, src.count * 4);
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

inline bool q_vec_is_numeric_dtype(const QVector& vec) {
    return vec.type == QVector::Type::F64 || vec.type == QVector::Type::I64;
}


// Boxes element i (null-aware).
inline QValue q_vec_box_at(const QVector& v, size_t i) {
    if (q_vec_is_null_at(v, i)) return qv_null();
    switch (v.type) {
        case QVector::Type::F64: return qv_float(q_vec_f64_data(v)[i]);
        case QVector::Type::I64: return qv_int(static_cast<long long>(q_vec_i64_data(v)[i]));
        case QVector::Type::DATE: return qv_date(q_vec_date_data(v)[i]);
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
        q_runtime_reportf("runtime error: vec size mismatch in %s: %zu vs %zu\n", what, av->count, bv->count);
        q_exit(1);
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
            q_runtime_reportf("runtime error: vec arithmetic expects valid vectors\n");
            q_exit(1);
        }
        if (xv ? !q_vec_is_numeric_dtype(*xv) : !q_is_numeric_scalar(x)) {
            q_runtime_reportf("runtime error: vec arithmetic requires numeric vectors and scalars\n");
            q_exit(1);
        }
    }
    if (av && bv && av->count != bv->count) {
        q_runtime_reportf("runtime error: vec size mismatch in arithmetic: %zu vs %zu\n", av->count, bv->count);
        q_exit(1);
    }

    const size_t n = av ? av->count : bv->count;
    const bool wantF64 = op == QVecArithOp::Div ||
                         (av && av->type == QVector::Type::F64) || (bv && bv->type == QVector::Type::F64) ||
                         (!av && a.type == QValue::VAL_FLOAT) || (!bv && b.type == QValue::VAL_FLOAT);

    // Every element is written (a missing one too), so the output isn't
    // zeroed first. The loops run a block at a time on the thread pool; an
    // error is the first failing element's (core/par.hpp).
    QVector* out = q_vec_alloc_raw(wantF64 ? QVector::Type::F64 : QVector::Type::I64, n);
    q_vec_and_validity(*out, av, bv);
    const QVector& res = *out;

    if (wantF64) {
        double* od = q_vec_f64_data_mut(*out);
        with_f64_reader(a, [&](auto ra) {
            with_f64_reader(b, [&](auto rb) {
                par_range(n, [&](size_t lo, size_t hi) {
                    switch (op) {
                        case QVecArithOp::Add: for (size_t i = lo; i < hi; i++) od[i] = ra(i) + rb(i); break;
                        case QVecArithOp::Sub: for (size_t i = lo; i < hi; i++) od[i] = ra(i) - rb(i); break;
                        case QVecArithOp::Mul: for (size_t i = lo; i < hi; i++) od[i] = ra(i) * rb(i); break;
                        case QVecArithOp::Div:
                            for (size_t i = lo; i < hi; i++) {
                                if (q_vec_is_null_at(res, i)) { od[i] = 0.0; continue; }
                                const double y = rb(i);
                                if (y == 0.0) {
                                    q_runtime_reportf("runtime error: vec division by zero (f64)\n");
                                    q_exit(1);
                                }
                                od[i] = ra(i) / y;
                            }
                            break;
                    }
                });
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
            par_range(n, [&](size_t lo, size_t hi) {
                for (size_t i = lo; i < hi; i++) {
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

// The reductions follow the table aggregates (specs/tables_v0_1_spec.md D6, D13;
// table/engine.hpp agg_*), so sum(t.x) and summarize(s = sum(e.x)) agree:
// missing entries are skipped, and NaN too in float vectors.

// q_vec_skips reports whether entry i is left out of a reduction.
inline bool q_vec_skips(const QVector& v, size_t i) {
    return q_vec_is_null_at(v, i) || (v.type == QVector::Type::F64 && std::isnan(q_vec_f64_data(v)[i]));
}

// The reductions run on the thread pool (core/par.hpp) with results that
// don't depend on the thread count (specs/tables_v0_3_spec.md §8.4): float
// sums follow the pairwise halving (q_pairwise_sum_par), int sums are exact
// (128-bit), and the present values of a vector with missing entries are
// gathered a block at a time, in order.
namespace quark {
namespace vec {

// DoubleBuf is a buffer of doubles that isn't zeroed first.
struct DoubleBuf {
    std::unique_ptr<double[]> p;
    size_t n = 0;
};

// gather_doubles collects val(i) for the entries of [0, n) where keep(i), in
// order.
template <class Keep, class Val>
inline DoubleBuf gather_doubles(size_t n, Keep keep, Val val) {
    const quark::par::Plan plan = q_par_plan(static_cast<long long>(n));
    const size_t nb = static_cast<size_t>(plan.nblocks);
    std::vector<size_t> start(nb + 1, 0);
    q_par_run(plan, [&](long long lo, long long hi, long long b) {
        size_t c = 0;
        for (long long i = lo; i < hi; i++) c += keep(static_cast<size_t>(i)) ? 1 : 0;
        start[static_cast<size_t>(b) + 1] = c;
    });
    for (size_t b = 0; b < nb; b++) start[b + 1] += start[b];
    DoubleBuf out;
    out.n = start[nb];
    out.p.reset(new double[out.n > 0 ? out.n : 1]);
    double* dst = out.p.get();
    q_par_run(plan, [&](long long lo, long long hi, long long b) {
        size_t at = start[static_cast<size_t>(b)];
        for (long long i = lo; i < hi; i++) {
            if (keep(static_cast<size_t>(i))) dst[at++] = val(static_cast<size_t>(i));
        }
    });
    return out;
}

// sum_i128 is the exact sum of val(i), and the count, over the entries of
// [0, n) where keep(i).
template <class Keep, class Val>
inline __int128 sum_i128(size_t n, Keep keep, Val val, size_t* count) {
    const quark::par::Plan plan = q_par_plan(static_cast<long long>(n));
    const size_t nb = static_cast<size_t>(plan.nblocks);
    std::vector<__int128> part(nb, 0);
    std::vector<size_t> cnt(nb, 0);
    q_par_run(plan, [&](long long lo, long long hi, long long b) {
        __int128 acc = 0;
        size_t c = 0;
        for (long long i = lo; i < hi; i++) {
            if (!keep(static_cast<size_t>(i))) continue;
            acc += val(static_cast<size_t>(i));
            c++;
        }
        part[static_cast<size_t>(b)] = acc;
        cnt[static_cast<size_t>(b)] = c;
    });
    __int128 total = 0;
    size_t c = 0;
    for (size_t b = 0; b < nb; b++) {
        total += part[b];
        c += cnt[b];
    }
    if (count != nullptr) *count = c;
    return total;
}

} // namespace vec
} // namespace quark

// q_vec_f64_present_sum is the pairwise sum (core/sum.hpp) of a float
// vector's present values in order: missing entries and NaN are left out.
// *count, when given, receives how many values were summed. A dense vector
// is summed in place; otherwise its present values are gathered first, as
// the table aggregates gather a group's values (table/engine.hpp), so both
// sum the same array.
inline double q_vec_f64_present_sum(const QVector& v, size_t* count = nullptr) {
    const double* d = q_vec_f64_data(v);
    if (v.null_count == 0) {
        const double s = q_pairwise_sum_par(d, v.count);
        // A NaN entry makes the sum NaN, so a sum that isn't NaN had none.
        if (!std::isnan(s)) {
            if (count != nullptr) *count = v.count;
            return s;
        }
    }
    const quark::vec::DoubleBuf present = quark::vec::gather_doubles(
        v.count, [&](size_t i) { return !q_vec_skips(v, i); }, [&](size_t i) { return d[i]; });
    if (count != nullptr) *count = present.n;
    return q_pairwise_sum_par(present.p.get(), present.n);
}

// q_vec_i64_present_sum is the exact sum of an int vector's present entries,
// and how many there are.
inline __int128 q_vec_i64_present_sum(const QVector& v, size_t* count) {
    const int64_t* d = q_vec_i64_data(v);
    if (v.null_count == 0) {
        return quark::vec::sum_i128(v.count, [](size_t) { return true; }, [&](size_t i) { return d[i]; }, count);
    }
    return quark::vec::sum_i128(v.count, [&](size_t i) { return !q_vec_is_null_at(v, i); }, [&](size_t i) { return d[i]; }, count);
}

// q_vec_bool_present_trues is the number of true entries of a bool vector,
// and *count the number of present ones.
inline size_t q_vec_bool_present_trues(const QVector& v, size_t* count) {
    const __int128 trues = quark::vec::sum_i128(
        v.count, [&](size_t i) { return !q_vec_is_null_at(v, i); }, [&](size_t i) { return q_vec_bool_at(v, i) ? 1 : 0; }, count);
    return static_cast<size_t>(trues);
}

// q_sum_fits is an exact int sum as an int, or the overflow error.
inline int64_t q_sum_fits(__int128 total) {
    if (total < INT64_MIN || total > INT64_MAX) {
        q_runtime_reportf("runtime error: integer overflow in 'sum'\n");
        q_exit(1);
    }
    return static_cast<int64_t>(total);
}

// q_vec_mean_present is the mean of a numeric or bool vector's present
// values, with the table aggregate's rules (agg_mean): float values summed
// pairwise, int values exactly (128-bit), bools as the share of true values.
// *count receives the number of present values; the mean of none is 0 here,
// and each caller decides what that means (an error, or a missing entry).
inline double q_vec_mean_present(const QVector& v, size_t* count) {
    switch (v.type) {
        case QVector::Type::F64: {
            const double s = q_vec_f64_present_sum(v, count);
            return *count == 0 ? 0.0 : s / static_cast<double>(*count);
        }
        case QVector::Type::I64: {
            const __int128 acc = q_vec_i64_present_sum(v, count);
            return *count == 0 ? 0.0 : q_i128_to_double(acc) / static_cast<double>(*count);
        }
        case QVector::Type::BOOL: {
            const size_t trues = q_vec_bool_present_trues(v, count);
            return *count == 0 ? 0.0 : static_cast<double>(trues) / static_cast<double>(*count);
        }
        case QVector::Type::STR:
            break;
        case QVector::Type::DATE:
            q_runtime_reportf("runtime error: mean of a vec[date]: dates can't be averaged\n");
            q_exit(1);
    }
    *count = 0;
    return 0.0;
}

// q_vec_present_doubles is v's present values as doubles, in order. Float
// NaN is skipped. A bool's true is 1. str and date are not numeric (false;
// the caller reports them); date is not an error here.
inline bool q_vec_present_doubles(const QVector& v, quark::vec::DoubleBuf& out) {
    switch (v.type) {
        case QVector::Type::F64: {
            const double* d = q_vec_f64_data(v);
            out = quark::vec::gather_doubles(v.count, [&](size_t i) { return !q_vec_skips(v, i); }, [&](size_t i) { return d[i]; });
            return true;
        }
        case QVector::Type::I64: {
            const int64_t* d = q_vec_i64_data(v);
            out = quark::vec::gather_doubles(
                v.count, [&](size_t i) { return !q_vec_is_null_at(v, i); }, [&](size_t i) { return static_cast<double>(d[i]); });
            return true;
        }
        case QVector::Type::BOOL: {
            out = quark::vec::gather_doubles(
                v.count, [&](size_t i) { return !q_vec_is_null_at(v, i); }, [&](size_t i) { return q_vec_bool_at(v, i) ? 1.0 : 0.0; });
            return true;
        }
        case QVector::Type::STR:
        case QVector::Type::DATE:
            return false;
    }
    return false;
}

// q_sd_present is the sample standard deviation of v, or *ok false when
// fewer than two values are present (or v isn't numeric). The table
// aggregate uses the same function for a whole column (agg_sd).
inline double q_sd_present(const QVector& v, bool* ok) {
    quark::vec::DoubleBuf xs;
    if (!q_vec_present_doubles(v, xs) || xs.n < 2) {
        *ok = false;
        return 0.0;
    }
    *ok = true;
    return q_sample_sd(xs.p.get(), xs.n);
}

// sum: an int for int and bool vectors (bools count as 0 and 1), a float
// for float vectors; no values sum to 0. An int sum is exact, so it is an
// error only when the total doesn't fit in an int (specs/tables_v0_3_spec.md
// §8.4), whatever the order of the values.
inline QValue q_vec_sum(QValue vec) {
    if (!q_vec_has_valid_handle(vec) || !q_vec_validate(*vec.data.vector_val)) {
        q_runtime_reportf("runtime error: sum() requires numeric or bool vec\n");
        q_exit(1);
    }
    const QVector& v = *vec.data.vector_val;
    switch (v.type) {
        case QVector::Type::I64: {
            size_t n = 0;
            return qv_int(q_sum_fits(q_vec_i64_present_sum(v, &n)));
        }
        case QVector::Type::BOOL: {
            size_t n = 0;
            return qv_int(static_cast<int64_t>(q_vec_bool_present_trues(v, &n)));
        }
        case QVector::Type::F64:
            return qv_float(q_vec_f64_present_sum(v));
        case QVector::Type::STR:
        case QVector::Type::DATE:
            break;
    }
    q_runtime_reportf("runtime error: sum() requires numeric or bool vec, got vec[%s]\n", q_vec_dtype_name(v));
    q_exit(1);
}

// q_vec_compare orders entries i and j of v: numbers by value, bools false
// before true, strings in byte order.
inline int q_vec_compare(const QVector& v, size_t i, size_t j) {
    switch (v.type) {
        case QVector::Type::I64: {
            const int64_t a = q_vec_i64_data(v)[i], b = q_vec_i64_data(v)[j];
            return a < b ? -1 : (a > b ? 1 : 0);
        }
        case QVector::Type::F64: {
            const double a = q_vec_f64_data(v)[i], b = q_vec_f64_data(v)[j];
            return a < b ? -1 : (a > b ? 1 : 0);
        }
        case QVector::Type::BOOL:
            return static_cast<int>(q_vec_bool_at(v, i)) - static_cast<int>(q_vec_bool_at(v, j));
        case QVector::Type::STR: {
            const int c = q_vec_str_at(v, i).compare(q_vec_str_at(v, j));
            return c < 0 ? -1 : (c > 0 ? 1 : 0);
        }
        case QVector::Type::DATE: {
            const int32_t a = q_vec_date_data(v)[i], b = q_vec_date_data(v)[j];
            return a < b ? -1 : (a > b ? 1 : 0);
        }
    }
    return 0;
}

// min / max: the smallest or largest present entry, with the vector's
// element type (strings in byte order). A vector with no present entries is
// an error: a value taken out of a vec must be present (D12).
inline QValue q_vec_extremum(QValue vec, bool want_min, const char* name) {
    if (!q_vec_has_valid_handle(vec) || !q_vec_validate(*vec.data.vector_val)) {
        q_runtime_reportf("runtime error: %s() requires a vec\n", name);
        q_exit(1);
    }
    const QVector& v = *vec.data.vector_val;
    // Each block of the thread pool finds its best entry, and the blocks are
    // compared in order, so a tie keeps the first entry, as one loop does.
    const quark::par::Plan plan = q_par_plan(static_cast<long long>(v.count));
    const size_t nb = static_cast<size_t>(plan.nblocks);
    // Dense numeric vectors compare values directly (the generic loop below
    // switches on the dtype for every entry).
    if (v.null_count == 0 && v.count > 0 && v.type == QVector::Type::I64) {
        const int64_t* d = q_vec_i64_data(v);
        std::vector<int64_t> part(nb);
        q_par_run(plan, [&](long long lo, long long hi, long long b) {
            int64_t best = d[lo];
            for (long long i = lo + 1; i < hi; i++) best = want_min ? (d[i] < best ? d[i] : best) : (d[i] > best ? d[i] : best);
            part[static_cast<size_t>(b)] = best;
        });
        int64_t best = part[0];
        for (size_t b = 1; b < nb; b++) best = want_min ? (part[b] < best ? part[b] : best) : (part[b] > best ? part[b] : best);
        return qv_int(best);
    }
    if (v.null_count == 0 && v.type == QVector::Type::F64) {
        const double* d = q_vec_f64_data(v);
        std::vector<double> part(nb);
        std::vector<char> found(nb, 0);
        q_par_run(plan, [&](long long lo, long long hi, long long b) {
            long long first = lo;
            while (first < hi && std::isnan(d[first])) first++;
            if (first == hi) return;
            // A NaN never compares less or greater, so it never wins.
            double best = d[first];
            for (long long i = first + 1; i < hi; i++) best = want_min ? (d[i] < best ? d[i] : best) : (d[i] > best ? d[i] : best);
            part[static_cast<size_t>(b)] = best;
            found[static_cast<size_t>(b)] = 1;
        });
        bool any = false;
        double best = 0.0;
        for (size_t b = 0; b < nb; b++) {
            if (!found[b]) continue;
            if (!any) {
                best = part[b];
                any = true;
            } else {
                best = want_min ? (part[b] < best ? part[b] : best) : (part[b] > best ? part[b] : best);
            }
        }
        if (any) return qv_float(best);
    }
    std::vector<long long> part(nb, -1);
    q_par_run(plan, [&](long long lo, long long hi, long long b) {
        long long best = -1;
        for (long long i = lo; i < hi; i++) {
            if (q_vec_skips(v, static_cast<size_t>(i))) continue;
            if (best < 0) {
                best = i;
                continue;
            }
            const int c = q_vec_compare(v, static_cast<size_t>(i), static_cast<size_t>(best));
            if (want_min ? c < 0 : c > 0) best = i;
        }
        part[static_cast<size_t>(b)] = best;
    });
    long long best = -1;
    for (size_t b = 0; b < nb; b++) {
        if (part[b] < 0) continue;
        if (best < 0) {
            best = part[b];
            continue;
        }
        const int c = q_vec_compare(v, static_cast<size_t>(part[b]), static_cast<size_t>(best));
        if (want_min ? c < 0 : c > 0) best = part[b];
    }
    if (best < 0) {
        q_runtime_reportf("runtime error: %s() of a vec with no present values\n", name);
        q_exit(1);
    }
    return q_vec_box_at(v, static_cast<size_t>(best));
}

inline QValue q_vec_min(QValue vec) { return q_vec_extremum(vec, true, "min"); }
inline QValue q_vec_max(QValue vec) { return q_vec_extremum(vec, false, "max"); }

// ============================================================
// fillna / astype (both return new vectors)
// ============================================================

inline QValue q_fillna(QValue vec, QValue value) {
    if (!q_vec_has_valid_handle(vec) || !q_vec_validate(*vec.data.vector_val)) {
        q_runtime_reportf("runtime error: fillna() expects valid vec as first argument\n");
        q_exit(1);
    }
    const QVector& src = *vec.data.vector_val;
    // The fill value must have the vec's element type, as a write into a
    // typed list must (an int widens to a float): fillna(2.7) on a vec[i64]
    // would otherwise store 2. It is checked whether or not anything is
    // missing, so a wrong fill fails on every input.
    bool fits = false;
    const char* dtype = "?";
    switch (src.type) {
        case QVector::Type::F64: fits = q_is_numeric_scalar(value); dtype = "f64"; break;
        case QVector::Type::I64: fits = value.type == QValue::VAL_INT; dtype = "i64"; break;
        case QVector::Type::BOOL: fits = value.type == QValue::VAL_BOOL; dtype = "bool"; break;
        case QVector::Type::STR: fits = value.type == QValue::VAL_STRING && value.data.string_val != nullptr; dtype = "str"; break;
        case QVector::Type::DATE: fits = value.type == QValue::VAL_DATE; dtype = "date"; break;
    }
    if (!fits) {
        q_runtime_reportf("runtime error: fillna() can't fill vec[%s] with %s: the fill value must have the element type (an int may fill a vec of float)\n", dtype, q_type_name(value.type));
        q_exit(1);
    }
    if (src.null_count == 0) {
        return vec;  // immutable: sharing the same vector is safe
    }

    switch (src.type) {
        case QVector::Type::F64: {
            QValue out = q_vec_clone(vec);
            double* d = q_vec_f64_data_mut(*out.data.vector_val);
            const double fill = q_to_double_scalar(value);
            quark::vec::par_range(src.count, [&](size_t lo, size_t hi) {
                for (size_t i = lo; i < hi; i++) if (q_vec_is_null_at(src, i)) d[i] = fill;
            });
            out.data.vector_val->validity = nullptr;
            out.data.vector_val->null_count = 0;
            return out;
        }
        case QVector::Type::I64: {
            QValue out = q_vec_clone(vec);
            int64_t* d = q_vec_i64_data_mut(*out.data.vector_val);
            const int64_t fill = value.data.int_val;
            quark::vec::par_range(src.count, [&](size_t lo, size_t hi) {
                for (size_t i = lo; i < hi; i++) if (q_vec_is_null_at(src, i)) d[i] = fill;
            });
            out.data.vector_val->validity = nullptr;
            out.data.vector_val->null_count = 0;
            return out;
        }
        case QVector::Type::BOOL: {
            QValue out = q_vec_clone(vec);
            const bool fill = value.data.bool_val;
            QVector& ov = *out.data.vector_val;
            quark::vec::par_range(src.count, [&](size_t lo, size_t hi) {
                for (size_t i = lo; i < hi; i++) {
                    if (q_vec_is_null_at(src, i)) q_vec_bool_put(ov, i, fill);
                }
            });
            out.data.vector_val->validity = nullptr;
            out.data.vector_val->null_count = 0;
            return out;
        }
        case QVector::Type::DATE: {
            QValue out = q_vec_clone(vec);
            int32_t* d = q_vec_date_data_mut(*out.data.vector_val);
            const int32_t fill = static_cast<int32_t>(value.data.int_val);
            quark::vec::par_range(src.count, [&](size_t lo, size_t hi) {
                for (size_t i = lo; i < hi; i++) if (q_vec_is_null_at(src, i)) d[i] = fill;
            });
            out.data.vector_val->validity = nullptr;
            out.data.vector_val->null_count = 0;
            return out;
        }
        case QVector::Type::STR: {
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
    q_runtime_reportf("runtime error: fillna() unsupported vec dtype\n");
    q_exit(1);
}

inline QValue q_astype(QValue vec, QValue dtype) {
    if (!q_vec_has_valid_handle(vec) || !q_vec_validate(*vec.data.vector_val)) {
        q_runtime_reportf("runtime error: astype() expects valid vec as first argument\n");
        q_exit(1);
    }
    if (dtype.type != QValue::VAL_STRING || dtype.data.string_val == nullptr) {
        q_runtime_reportf("runtime error: astype() expects dtype string as second argument\n");
        q_exit(1);
    }

    const QVector& src = *vec.data.vector_val;
    const char* target = dtype.data.string_val;
    const size_t n = src.count;

    if (std::strcmp(target, "f64") == 0) {
        if (src.type == QVector::Type::F64) return q_vec_clone(vec);
        if (src.type != QVector::Type::I64 && src.type != QVector::Type::BOOL) {
            q_runtime_reportf("runtime error: astype() cannot cast source vec to f64\n");
            q_exit(1);
        }
        QVector* out = q_vec_alloc_raw(QVector::Type::F64, n);
        double* d = q_vec_f64_data_mut(*out);
        if (src.type == QVector::Type::I64) {
            const int64_t* in = q_vec_i64_data(src);
            quark::vec::par_range(n, [&](size_t lo, size_t hi) {
                for (size_t i = lo; i < hi; i++) d[i] = static_cast<double>(in[i]);
            });
        } else {
            quark::vec::par_range(n, [&](size_t lo, size_t hi) {
                for (size_t i = lo; i < hi; i++) d[i] = q_vec_bool_at(src, i) ? 1.0 : 0.0;
            });
        }
        q_vec_copy_validity(*out, src);
        return qv_vector_from(out);
    }

    if (std::strcmp(target, "i64") == 0) {
        if (src.type == QVector::Type::I64) return q_vec_clone(vec);
        if (src.type != QVector::Type::F64 && src.type != QVector::Type::BOOL) {
            q_runtime_reportf("runtime error: astype() cannot cast source vec to i64\n");
            q_exit(1);
        }
        QVector* out = q_vec_alloc_raw(QVector::Type::I64, n);
        int64_t* d = q_vec_i64_data_mut(*out);
        if (src.type == QVector::Type::F64) {
            const double* in = q_vec_f64_data(src);
            quark::vec::par_range(n, [&](size_t lo, size_t hi) {
                for (size_t i = lo; i < hi; i++) {
                    if (q_vec_is_null_at(src, i)) { d[i] = 0; continue; }
                    const double x = in[i];
                    // Casting NaN or an out-of-range double to int64 is undefined.
                    if (!(x >= -9223372036854775808.0 && x < 9223372036854775808.0)) {
                        char fbuf[32];
                        q_runtime_reportf("runtime error: astype('i64'): value %s at index %zu is out of i64 range\n", q_format_float(x, fbuf, sizeof(fbuf)), i);
                        q_exit(1);
                    }
                    d[i] = static_cast<int64_t>(x);
                }
            });
        } else {
            quark::vec::par_range(n, [&](size_t lo, size_t hi) {
                for (size_t i = lo; i < hi; i++) d[i] = q_vec_bool_at(src, i) ? 1 : 0;
            });
        }
        q_vec_copy_validity(*out, src);
        return qv_vector_from(out);
    }

    if (std::strcmp(target, "bool") == 0) {
        if (src.type == QVector::Type::BOOL) return q_vec_clone(vec);
        if (src.type != QVector::Type::F64 && src.type != QVector::Type::I64) {
            q_runtime_reportf("runtime error: astype() cannot cast source vec to bool\n");
            q_exit(1);
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
    q_exit(1);
}

// ============================================================
// List <-> vector conversion
// ============================================================

inline QValue q_to_vec(QValue input) {
    if (q_vec_has_valid_handle(input) && q_vec_validate(*input.data.vector_val)) {
        return q_vec_clone(input);
    }

    if (input.type != QValue::VAL_LIST || !input.data.list_val) {
        q_runtime_reportf("runtime error: to_vec expects list or vec input\n");
        q_exit(1);
    }

    const QList& items = *input.data.list_val;
    const size_t n = items.size();

    enum class Mode { UNKNOWN, I64, F64, BOOL, STR, INVALID };
    Mode mode = Mode::UNKNOWN;

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
                q_runtime_reportf("runtime error: to_vec only supports int/float/bool/str lists (null allowed), got %s at index %zu\n", q_type_name(item.type), i);
                q_exit(1);
        }
        if (mode == Mode::UNKNOWN) {
            mode = want;
        } else if (mode != want) {
            q_runtime_reportf("runtime error: to_vec requires homogeneous element types (all int, all float, all bool, or all str)\n");
            q_exit(1);
        }
    }

    if (mode == Mode::UNKNOWN) {
        mode = Mode::I64;
    }

    if (mode == Mode::STR) {
        if (str_bytes > static_cast<size_t>(INT32_MAX)) {
            q_runtime_reportf("runtime error: vec[str] data exceeds 2 GiB\n");
            q_exit(1);
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
            case QVector::Type::STR:
            case QVector::Type::DATE: break;  // not built here
        }
    }
    return qv_vector_from(out);
}

inline QValue q_to_list(QValue input) {
    if (input.type == QValue::VAL_LIST) {
        return input;
    }
    if (!q_vec_has_valid_handle(input)) {
        q_runtime_reportf("runtime error: to_list expects a vec or list input\n");
        q_exit(1);
    }
    const QVector& v = *input.data.vector_val;
    // The list holds the vec's dtype (to_list() is typed list[int] for a
    // vec[int]), so a missing entry can't go into it (D12).
    if (v.null_count > 0) {
        q_runtime_reportf("runtime error: to_list() of a vec with %zu missing %s; use fillna or drop_na first\n",
                          v.null_count, v.null_count == 1 ? "entry" : "entries");
        q_exit(1);
    }
    const QElemType* elem = nullptr;
    switch (v.type) {
        case QVector::Type::F64: elem = &q_elem_float; break;
        case QVector::Type::I64: elem = &q_elem_int; break;
        case QVector::Type::BOOL: elem = &q_elem_bool; break;
        case QVector::Type::STR: elem = &q_elem_str; break;
        case QVector::Type::DATE: elem = &q_elem_date; break;
    }
    QValue out = qv_list(static_cast<int>(v.count), elem);
    QList& items = *out.data.list_val;
    items.reserve(v.count);
    for (size_t i = 0; i < v.count; i++) {
        items.push_back(q_vec_box_at(v, i));
    }
    return out;
}

// ============================================================
// Comparisons (produce vec[bool]; nulls propagate)
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

// The six comparison operators, for kernels that pick their loop once
// instead of testing the operator per element.
enum class CmpKind { LT, LE, GT, GE, EQ, NE };

inline CmpKind cmp_kind(const char* name) {
    switch (name[0]) {
        case '<': return name[1] == '=' ? CmpKind::LE : CmpKind::LT;
        case '>': return name[1] == '=' ? CmpKind::GE : CmpKind::GT;
        case '=': return CmpKind::EQ;
        default: return CmpKind::NE;
    }
}

// cmp_flip is the operator with its operands swapped: a < b is b > a.
inline CmpKind cmp_flip(CmpKind k) {
    switch (k) {
        case CmpKind::LT: return CmpKind::GT;
        case CmpKind::LE: return CmpKind::GE;
        case CmpKind::GT: return CmpKind::LT;
        case CmpKind::GE: return CmpKind::LE;
        default: return k;
    }
}

template <CmpKind K, typename X, typename Y>
inline bool cmp_apply(X x, Y y) {
    if constexpr (K == CmpKind::LT) return x < y;
    else if constexpr (K == CmpKind::LE) return x <= y;
    else if constexpr (K == CmpKind::GT) return x > y;
    else if constexpr (K == CmpKind::GE) return x >= y;
    else if constexpr (K == CmpKind::EQ) return x == y;
    else return x != y;
}

// with_cmp_kind calls f with k as a compile-time constant.
template <typename F>
inline void with_cmp_kind(CmpKind k, F&& f) {
    switch (k) {
        case CmpKind::LT: f(std::integral_constant<CmpKind, CmpKind::LT>{}); break;
        case CmpKind::LE: f(std::integral_constant<CmpKind, CmpKind::LE>{}); break;
        case CmpKind::GT: f(std::integral_constant<CmpKind, CmpKind::GT>{}); break;
        case CmpKind::GE: f(std::integral_constant<CmpKind, CmpKind::GE>{}); break;
        case CmpKind::EQ: f(std::integral_constant<CmpKind, CmpKind::EQ>{}); break;
        case CmpKind::NE: f(std::integral_constant<CmpKind, CmpKind::NE>{}); break;
    }
}

// cmp_int_float_exact is `i K f` on the mathematical values (core/numcmp.hpp).
template <CmpKind K>
inline bool cmp_int_float_exact(long long i, double f) {
    if (__builtin_expect(q_int_exact_in_float(i), 1)) return cmp_apply<K>(static_cast<double>(i), f);
    const int c = q_cmp_int_float_slow(i, f);
    if constexpr (K == CmpKind::LT) return c == -1;
    else if constexpr (K == CmpKind::LE) return c == -1 || c == 0;
    else if constexpr (K == CmpKind::GT) return c == 1;
    else if constexpr (K == CmpKind::GE) return c == 1 || c == 0;
    else if constexpr (K == CmpKind::EQ) return c == 0;
    else return c != 0;
}

// cmp_int_vec_float_scalar is `iv[j] K f` for an int vec and one float. An
// int compares with a float exactly as it compares with the float rounded
// to an int in the direction that keeps the answer (i < f is i < ceil(f),
// i <= f is i <= floor(f)), so the loop compares ints. A float outside the
// int range, or NaN, gives the same answer for every element.
template <CmpKind K>
inline void cmp_int_vec_float_scalar(uint8_t* bits, const int64_t* iv, double f, size_t n) {
    constexpr double k2_63 = 9223372036854775808.0;
    bool constant = false, value = false;
    if (f != f) {
        constant = true;
        value = K == CmpKind::NE;
    } else if (f >= k2_63) {
        constant = true;
        value = K == CmpKind::LT || K == CmpKind::LE || K == CmpKind::NE;
    } else if (f < -k2_63) {
        constant = true;
        value = K == CmpKind::GT || K == CmpKind::GE || K == CmpKind::NE;
    }
    if (constant) {
        fill_bits(bits, n, [value](size_t) { return value; });
        return;
    }
    // f is in [-2^63, 2^63), so floor(f) and ceil(f) are ints.
    const double fl = std::floor(f), ce = std::ceil(f);
    const int64_t lo = static_cast<int64_t>(fl), hi = static_cast<int64_t>(ce);
    if constexpr (K == CmpKind::LT) {
        fill_bits(bits, n, [iv, hi](size_t j) { return iv[j] < hi; });
    } else if constexpr (K == CmpKind::LE) {
        fill_bits(bits, n, [iv, lo](size_t j) { return iv[j] <= lo; });
    } else if constexpr (K == CmpKind::GT) {
        fill_bits(bits, n, [iv, lo](size_t j) { return iv[j] > lo; });
    } else if constexpr (K == CmpKind::GE) {
        fill_bits(bits, n, [iv, hi](size_t j) { return iv[j] >= hi; });
    } else {
        const bool whole = fl == f;
        constexpr bool ne = K == CmpKind::NE;
        if (!whole) {
            fill_bits(bits, n, [](size_t) { return ne; });
        } else if (ne) {
            fill_bits(bits, n, [iv, lo](size_t j) { return iv[j] != lo; });
        } else {
            fill_bits(bits, n, [iv, lo](size_t j) { return iv[j] == lo; });
        }
    }
}

// all_exact_in_float reports whether every entry of iv[0, n) is within
// ±2^53, where converting to float loses nothing (entries behind a missing
// slot are included: they are ignored, but only make the check stricter).
inline bool all_exact_in_float(const int64_t* iv, size_t n) {
    bool out_of_range = false;
    for (size_t j = 0; j < n; j++) {
        out_of_range |= !q_int_exact_in_float(static_cast<long long>(iv[j]));
    }
    return !out_of_range;
}

// cmp_int_vec_float_vec is `iv[j] K fv[j]`, in blocks: a block whose ints
// are all within ±2^53 compares as floats, the rest exactly.
template <CmpKind K>
inline void cmp_int_vec_float_vec(uint8_t* bits, const int64_t* iv, const double* fv, size_t n) {
    constexpr size_t kBlock = 1024;  // a multiple of 8, so each block starts on a byte
    // A pool block is a multiple of kBlock, so the blocks are the same.
    par_range(n, [&](size_t lo, size_t hi) {
        for (size_t base = lo; base < hi; base += kBlock) {
            const size_t m = hi - base < kBlock ? hi - base : kBlock;
            const int64_t* ib = iv + base;
            const double* fb = fv + base;
            if (all_exact_in_float(ib, m)) {
                fill_bits_seq(bits + base / 8, m, [ib, fb](size_t j) { return cmp_apply<K>(static_cast<double>(ib[j]), fb[j]); });
            } else {
                fill_bits_seq(bits + base / 8, m, [ib, fb](size_t j) { return cmp_int_float_exact<K>(static_cast<long long>(ib[j]), fb[j]); });
            }
        }
    });
}

// cmp_exact_int_float is an element-wise `int_side K float_side`, where
// one side is an int (vec or scalar) and the other a float (vec or scalar).
// Casting every int to double would collapse ints past 2^53; each case
// below takes a float loop only where the conversion is exact.
inline QValue cmp_exact_int_float(QValue int_side, QValue float_side, size_t n, CmpKind kind) {
    QVector* out = q_vec_alloc(QVector::Type::BOOL, n);
    q_vec_and_validity(*out, as_vec(int_side), as_vec(float_side));
    const QVector* iv = as_vec(int_side);
    const QVector* fv = as_vec(float_side);
    const int64_t* idata = iv ? q_vec_i64_data(*iv) : nullptr;
    const double* fdata = fv ? q_vec_f64_data(*fv) : nullptr;
    with_cmp_kind(kind, [&](auto kc) {
        constexpr CmpKind K = decltype(kc)::value;
        if (!fdata) {
            cmp_int_vec_float_scalar<K>(out->values, idata, float_side.data.float_val, n);
        } else if (!idata) {
            const long long i = q_to_i64_scalar(int_side);
            if (q_int_exact_in_float(i)) {
                const double d = static_cast<double>(i);
                fill_bits(out->values, n, [d, fdata](size_t j) { return cmp_apply<K>(d, fdata[j]); });
            } else {
                fill_bits(out->values, n, [i, fdata](size_t j) { return cmp_int_float_exact<K>(i, fdata[j]); });
            }
        } else {
            cmp_int_vec_float_vec<K>(out->values, idata, fdata, n);
        }
    });
    return qv_vector_from(out);
}

template <typename Op>
inline QValue cmp_numeric(QValue a, QValue b, Op op, const char* name) {
    const size_t n = binary_len(a, b, "comparison");
    const bool a_int = is_int_operand(a);
    const bool b_int = is_int_operand(b);
    if (a_int && b_int) {
        QValue result = qv_null();
        with_i64_reader(a, [&](auto ra) {
            with_i64_reader(b, [&](auto rb) { result = cmp_finish(a, b, n, ra, rb, op); });
        });
        return result;
    }
    // Mixed: compare as `int K float`, flipping the operator when the float
    // is on the left (a > b is b < a).
    if (a_int) return cmp_exact_int_float(a, b, n, cmp_kind(name));
    if (b_int) return cmp_exact_int_float(b, a, n, cmp_flip(cmp_kind(name)));
    QValue result = qv_null();
    with_f64_reader(a, [&](auto ra) {
        with_f64_reader(b, [&](auto rb) { result = cmp_finish(a, b, n, ra, rb, op); });
    });
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

inline bool is_date_operand(QValue x) {
    return is_vec_of(x, QVector::Type::DATE) || x.type == QValue::VAL_DATE;
}

// A date vec compares by day number (D9), with a date or another date vec.
template <typename Op>
inline QValue cmp_date(QValue a, QValue b, Op op) {
    const size_t n = binary_len(a, b, "comparison");
    auto reader = [](QValue x) {
        const QVector* v = as_vec(x);
        const int32_t* d = v ? q_vec_date_data(*v) : nullptr;
        const int32_t s = v ? 0 : static_cast<int32_t>(x.data.int_val);
        return [d, s](size_t i) { return d ? d[i] : s; };
    };
    return cmp_finish(a, b, n, reader(a), reader(b), op);
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
    q_runtime_reportf("runtime error: operator '%s' not supported for these vec types\n", op);
    q_exit(1);
}

template <typename Op>
inline QValue cmp_ordered(QValue a, QValue b, Op op, const char* name) {
    if (is_numeric_operand(a) && is_numeric_operand(b)) return cmp_numeric(a, b, op, name);
    if (is_date_operand(a) && is_date_operand(b)) return cmp_date(a, b, op);
    cmp_unsupported(name);
}

template <typename Op>
inline QValue cmp_equality(QValue a, QValue b, Op op, const char* name) {
    if (is_numeric_operand(a) && is_numeric_operand(b)) return cmp_numeric(a, b, op, name);
    if (is_bool_operand(a) && is_bool_operand(b)) return cmp_bool(a, b, op);
    if (is_str_operand(a) && is_str_operand(b)) return cmp_str(a, b, op);
    if (is_date_operand(a) && is_date_operand(b)) return cmp_date(a, b, op);
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

// q_vec_get is v.get(i): the element at index i (negative counts from the
// end), or null for a missing entry; out of range is an error.
inline QValue q_vec_get(QValue vec, QValue index) {
    if (!q_vec_has_valid_handle(vec)) {
        q_runtime_reportf("runtime error: vec index expects valid vec\n");
        q_exit(1);
    }
    if (index.type != QValue::VAL_INT) {
        q_runtime_reportf("runtime error: vec index must be int\n");
        q_exit(1);
    }
    const QVector& v = *vec.data.vector_val;
    long long idx = index.data.int_val;
    const long long len = static_cast<long long>(v.count);
    if (idx < 0) idx = len + idx;
    if (idx < 0 || idx >= len) {
        q_runtime_reportf("runtime error: vec index %lld out of range (len=%lld)\n", idx, len);
        q_exit(1);
    }
    return q_vec_box_at(v, static_cast<size_t>(idx));
}

// q_vec_missing reports a missing entry taken out of a vec as one value
// (specs/tables_v0_1_spec.md D12): it is an error, never a null read as a number.
[[noreturn]] inline void q_vec_missing(long long index) {
    q_runtime_reportf("runtime error: vec element %lld is missing; use fillna or drop_na first\n", index);
    q_exit(1);
}

// q_vec_get_scalar is v[i]: like v.get(i), but a missing entry is an error.
// Every other single-element read of a vec (for loops, enumerate, table
// rows) goes through it or checks the same way.
inline QValue q_vec_get_scalar(QValue vec, QValue index) {
    QValue out = q_vec_get(vec, index);
    if (out.type == QValue::VAL_NULL) q_vec_missing(index.data.int_val);
    return out;
}

// True when mask element i selects a row: valid and true.
inline bool q_vec_mask_selects(const QVector& mask, size_t i) {
    return !q_vec_is_null_at(mask, i) && q_vec_bool_at(mask, i);
}

inline size_t q_vec_mask_count(const QVector& mask) {
    size_t selected = 0;
    quark::vec::sum_i128(mask.count, [&](size_t i) { return q_vec_mask_selects(mask, i); }, [](size_t) { return 0; }, &selected);
    return selected;
}

namespace quark {
namespace vec {

// The loops of q_vec_keep, one block at a time. They are functions of their
// own (noinline), so each is optimized with keep inlined into it however
// deep the caller is: inlined into a caller, the same loop compiled 40%
// slower.
template <class Keep>
QPAR_NOINLINE inline size_t keep_count(size_t lo, size_t hi, Keep& keep) {
    size_t c = 0;
    for (size_t i = lo; i < hi; i++) c += keep(i) ? 1 : 0;
    return c;
}

template <class T, class Keep>
QPAR_NOINLINE inline void keep_copy(const T* src, T* dst, size_t lo, size_t hi, size_t j, Keep& keep, const QVector& dv,
                                    std::vector<size_t>& nulls) {
    if (dv.null_count == 0) {
        for (size_t i = lo; i < hi; i++) {
            if (keep(i)) dst[j++] = src[i];
        }
        return;
    }
    for (size_t i = lo; i < hi; i++) {
        if (!keep(i)) continue;
        dst[j] = src[i];
        if (q_vec_is_null_at(dv, i)) nulls.push_back(j);
        j++;
    }
}

} // namespace vec
} // namespace quark

// q_vec_keep is the elements of dv for which keep(i) holds, in order, with
// their missing marks (mask filtering, drop_na). It runs in two passes over
// the blocks of the thread pool: each block counts what it keeps (and the
// bytes of a str vector), then copies it from where the blocks before it end.
template <class Keep>
inline QValue q_vec_keep(const QVector& dv, Keep keep) {
    const size_t n = dv.count;
    const bool is_str = dv.type == QVector::Type::STR;
    const quark::par::Plan plan = q_par_plan(static_cast<long long>(n));
    const size_t nb = static_cast<size_t>(plan.nblocks);
    std::vector<size_t> start(nb + 1, 0), bytes(nb + 1, 0);
    q_par_run(plan, [&](long long lo, long long hi, long long b) {
        if (!is_str) {
            start[static_cast<size_t>(b) + 1] = quark::vec::keep_count(static_cast<size_t>(lo), static_cast<size_t>(hi), keep);
            return;
        }
        size_t c = 0, sb = 0;
        for (long long i = lo; i < hi; i++) {
            if (!keep(static_cast<size_t>(i))) continue;
            c++;
            sb += q_vec_str_at(dv, static_cast<size_t>(i)).size();
        }
        start[static_cast<size_t>(b) + 1] = c;
        bytes[static_cast<size_t>(b) + 1] = sb;
    });
    for (size_t b = 0; b < nb; b++) {
        start[b + 1] += start[b];
        bytes[b + 1] += bytes[b];
    }
    const size_t selected = start[nb];
    if (is_str && bytes[nb] > static_cast<size_t>(INT32_MAX)) {
        q_runtime_reportf("runtime error: a str vec holds more than 2 GiB of text\n");
        q_exit(1);
    }

    // A fixed-width vector's output is written in full, so it isn't zeroed.
    QVector* out = is_str || dv.type == QVector::Type::BOOL ? q_vec_alloc(dv.type, selected, is_str ? bytes[nb] : 0)
                                                            : q_vec_alloc_raw(dv.type, selected);
    std::vector<std::vector<size_t>> nulls(nb);
    // One loop per element type, over raw pointers.
    auto copy_fixed = [&](auto* dst, const auto* src) {
        q_par_run(plan, [&, dst, src](long long lo, long long hi, long long b) {
            quark::vec::keep_copy(src, dst, static_cast<size_t>(lo), static_cast<size_t>(hi), start[static_cast<size_t>(b)], keep, dv,
                                  nulls[static_cast<size_t>(b)]);
        });
    };
    switch (dv.type) {
        case QVector::Type::F64: copy_fixed(q_vec_f64_data_mut(*out), q_vec_f64_data(dv)); break;
        case QVector::Type::I64: copy_fixed(q_vec_i64_data_mut(*out), q_vec_i64_data(dv)); break;
        case QVector::Type::DATE: copy_fixed(q_vec_date_data_mut(*out), q_vec_date_data(dv)); break;
        case QVector::Type::BOOL: {
            // The blocks would share bytes of the output bitmap: one pass on
            // the calling thread.
            size_t j = 0;
            for (size_t i = 0; i < n; i++) {
                if (!keep(i)) continue;
                q_vec_bool_put(*out, j, q_vec_bool_at(dv, i));
                if (q_vec_is_null_at(dv, i)) nulls[0].push_back(j);
                j++;
            }
            break;
        }
        case QVector::Type::STR:
            q_par_run(plan, [&](long long lo, long long hi, long long b) {
                size_t j = start[static_cast<size_t>(b)];
                int32_t pos = static_cast<int32_t>(bytes[static_cast<size_t>(b)]);
                for (long long i = lo; i < hi; i++) {
                    if (!keep(static_cast<size_t>(i))) continue;
                    const std::string_view sv = q_vec_str_at(dv, static_cast<size_t>(i));
                    if (!sv.empty()) std::memcpy(out->values + pos, sv.data(), sv.size());
                    pos += static_cast<int32_t>(sv.size());
                    out->offsets[j + 1] = pos;
                    if (q_vec_is_null_at(dv, static_cast<size_t>(i))) nulls[static_cast<size_t>(b)].push_back(j);
                    j++;
                }
            });
            break;
    }
    for (const std::vector<size_t>& block : nulls) {
        for (size_t j : block) q_vec_mark_null(*out, j);
    }
    return qv_vector_from(out);
}

inline QValue q_vec_mask_filter(QValue data, QValue mask) {
    if (!q_vec_has_valid_handle(data) || !q_vec_has_valid_handle(mask)) {
        q_runtime_reportf("runtime error: mask filter expects vec operands\n");
        q_exit(1);
    }
    const QVector& dv = *data.data.vector_val;
    const QVector& mv = *mask.data.vector_val;
    if (mv.type != QVector::Type::BOOL) {
        q_runtime_reportf("runtime error: mask index must be a bool vec, got vec[%s]\n", q_vec_dtype_name(mv));
        q_exit(1);
    }
    if (dv.count != mv.count) {
        q_runtime_reportf("runtime error: mask length (%zu) does not match vec length (%zu)\n", mv.count, dv.count);
        q_exit(1);
    }

    // The predicates read the bitmaps through pointers they hold, and are
    // forced inline: q_vec_keep calls them per element from the blocks of
    // the thread pool, where clang otherwise may leave them as calls.
    const uint8_t* bits = mv.values;
    const uint8_t* valid = mv.null_count > 0 ? mv.validity : nullptr;
    if (valid == nullptr) {
        return q_vec_keep(dv, [bits](size_t i) QUARK_ALWAYS_INLINE { return quark::vec::get_bit(bits, i); });
    }
    return q_vec_keep(dv, [bits, valid](size_t i) QUARK_ALWAYS_INLINE {
        return quark::vec::get_bit(valid, i) && quark::vec::get_bit(bits, i);
    });
}

// v.drop_na(): the present entries of v, in order.
inline QValue q_drop_na(QValue vec) {
    if (!q_vec_has_valid_handle(vec)) {
        q_runtime_reportf("runtime error: drop_na() expects a vec\n");
        q_exit(1);
    }
    const QVector& v = *vec.data.vector_val;
    if (v.null_count == 0) return vec;  // vectors are immutable, so it can be shared
    const uint8_t* valid = v.validity;
    return q_vec_keep(v, [valid](size_t i) QUARK_ALWAYS_INLINE { return quark::vec::get_bit(valid, i); });
}

#endif // QUARK_TYPES_VECTOR_HPP
