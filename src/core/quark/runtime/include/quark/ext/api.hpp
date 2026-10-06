// quark/ext/api.hpp - Quark Extensions Interface (QEI) public API
//
// Include this header in your native extension implementation file to get
// access to all QEI types and helpers. Your extension is compiled as part
// of the generated Quark program, so all runtime types are available.
//
// Usage:
//   #include <quark/ext/api.hpp>
//
//   extern "C" QValue my_fn(int64_t x) {
//       return qext::box(x * 2);
//   }
//
// All memory must go through qext::malloc / qext::malloc_atomic / qext::strdup
// (or directly through q_malloc / q_malloc_atomic / q_strdup) so the Boehm GC
// can track it. Never use bare ::new or ::malloc for heap objects that outlive
// the call.

#ifndef QUARK_EXT_API_HPP
#define QUARK_EXT_API_HPP

// When included standalone (e.g. directly from an extension file), pull in
// the full runtime. When included from quark.hpp, the guards prevent re-inclusion.
#include "../core/value.hpp"
#include "../core/gc.hpp"
#include "../core/constructors.hpp"
#include "../types/vector.hpp"
#include "../types/dict.hpp"
#include "../types/closure.hpp"

#include <cstdint>
#include <cstdlib>
#include <cstdio>
#include <cstring>
#include <string_view>

// ============================================================
// qext namespace — public API for extension authors
// ============================================================
namespace qext {

// ------------------------------------------------------------
// Memory helpers (re-exports of runtime GC allocators)
// ------------------------------------------------------------

inline void* malloc(size_t n)        { return q_malloc(n); }
inline void* malloc_atomic(size_t n) { return q_malloc_atomic(n); }
inline char* strdup(const char* s)   { return q_strdup(s); }

// ------------------------------------------------------------
// Error reporting
// ------------------------------------------------------------

// Terminate the program with a plain error message.
[[noreturn]] inline void panic(const char* msg) {
    q_runtime_reportf("extension error: %s\n", msg);
    std::exit(1);
}

// Terminate the program with a printf-style formatted message.
template <typename... Args>
[[noreturn]] inline void panicf(const char* fmt, Args... args) {
    q_runtime_reportf(fmt, args...);
    std::exit(1);
}

// ------------------------------------------------------------
// Boxing helpers: native C++ → QValue
// ------------------------------------------------------------

inline QValue box(int64_t v)     { return qv_int(v); }
inline QValue box(double v)      { return qv_float(v); }
inline QValue box(bool v)        { return qv_bool(v); }
// Copies the string via GC — safe to pass a stack pointer.
inline QValue box(const char* v) { return qv_string(v); }
// Takes ownership of an already GC-allocated buffer (no copy).
inline QValue box_own(char* v)   { return qv_string_own(v); }
inline QValue box(QVector* v) {
    QValue q;
    q.type = QValue::VAL_VECTOR;
    q.data.vector_val = v;
    return q;
}
inline QValue box(QList* v) {
    QValue q;
    q.type = QValue::VAL_LIST;
    q.data.list_val = v;
    return q;
}
inline QValue box(QDict* v) {
    QValue q;
    q.type = QValue::VAL_DICT;
    q.data.dict_val = v;
    return q;
}
inline QValue null_val() { return qv_null(); }

// ------------------------------------------------------------
// Unboxing helpers: QValue → native C++ (with runtime checks)
// ------------------------------------------------------------

inline int64_t as_int(QValue v) {
    if (v.type != QValue::VAL_INT) {
        panic("as_int: value is not int");
    }
    return v.data.int_val;
}

inline double as_float(QValue v) {
    if (v.type != QValue::VAL_FLOAT) {
        panic("as_float: value is not float");
    }
    return v.data.float_val;
}

inline bool as_bool(QValue v) {
    if (v.type != QValue::VAL_BOOL) {
        panic("as_bool: value is not bool");
    }
    return v.data.bool_val;
}

inline const char* as_str(QValue v) {
    if (v.type != QValue::VAL_STRING || !v.data.string_val) {
        panic("as_str: value is not str");
    }
    return v.data.string_val;
}

inline QVector* as_vector(QValue v) {
    if (v.type != QValue::VAL_VECTOR || !v.data.vector_val) {
        panic("as_vector: value is not vector");
    }
    return v.data.vector_val;
}

inline QList* as_list(QValue v) {
    if (v.type != QValue::VAL_LIST || !v.data.list_val) {
        panic("as_list: value is not list");
    }
    return v.data.list_val;
}

inline QDict* as_dict(QValue v) {
    if (v.type != QValue::VAL_DICT || !v.data.dict_val) {
        panic("as_dict: value is not dict");
    }
    return v.data.dict_val;
}

inline QClosure* as_closure(QValue v) {
    if (v.type != QValue::VAL_FUNC || !v.data.func_val) {
        panic("as_closure: value is not fn");
    }
    return static_cast<QClosure*>(v.data.func_val);
}

// ------------------------------------------------------------
// Slice descriptor — C++17-compatible pointer+size pair
// ------------------------------------------------------------

template <typename T>
struct QSlice {
    T*     data;
    size_t size;

    T*       begin() const { return data; }
    T*       end()   const { return data + size; }
    T&       operator[](size_t i) { return data[i]; }
    const T& operator[](size_t i) const { return data[i]; }
};

// ------------------------------------------------------------
// Vector accessors
// ------------------------------------------------------------
// Vectors use the Apache Arrow columnar layout (see types/vector.hpp):
// f64/i64 values are contiguous arrays, bools are bit-packed, strings are
// int32 offsets plus UTF-8 bytes, and nulls live in an Arrow validity bitmap.
//
// Vectors are immutable once Quark code can see them. The *_mut accessors,
// set_bool and set_null are for filling a vector you just created with one
// of the new_* constructors, before returning it to Quark.

// Read-only view over f64 values. Panics if dtype != F64.
inline QSlice<const double> as_f64(const QVector* v) {
    if (!v || v->type != QVector::Type::F64) {
        panic("as_f64: vector dtype is not f64");
    }
    return QSlice<const double>{q_vec_f64_data(*v), v->count};
}

// Mutable view over f64 values of a vector you just created.
inline QSlice<double> as_f64_mut(QVector* v) {
    if (!v || v->type != QVector::Type::F64) {
        panic("as_f64_mut: vector dtype is not f64");
    }
    return QSlice<double>{q_vec_f64_data_mut(*v), v->count};
}

// Read-only view over i64 values. Panics if dtype != I64.
inline QSlice<const int64_t> as_i64(const QVector* v) {
    if (!v || v->type != QVector::Type::I64) {
        panic("as_i64: vector dtype is not i64");
    }
    return QSlice<const int64_t>{q_vec_i64_data(*v), v->count};
}

// Mutable view over i64 values of a vector you just created.
inline QSlice<int64_t> as_i64_mut(QVector* v) {
    if (!v || v->type != QVector::Type::I64) {
        panic("as_i64_mut: vector dtype is not i64");
    }
    return QSlice<int64_t>{q_vec_i64_data_mut(*v), v->count};
}

// Element i of a bool vector. Panics if dtype != BOOL.
inline bool bool_at(const QVector* v, size_t i) {
    if (!v || v->type != QVector::Type::BOOL) {
        panic("bool_at: vector dtype is not bool");
    }
    return q_vec_bool_at(*v, i);
}

// Sets element i of a bool vector you just created.
inline void set_bool(QVector* v, size_t i, bool b) {
    if (!v || v->type != QVector::Type::BOOL) {
        panic("set_bool: vector dtype is not bool");
    }
    q_vec_bool_put(*v, i, b);
}

// Element i of a str vector as a view into the vector's bytes.
inline std::string_view str_at(const QVector* v, size_t i) {
    if (!v || v->type != QVector::Type::STR) {
        panic("str_at: vector dtype is not str");
    }
    return q_vec_str_at(*v, i);
}

// ------------------------------------------------------------
// Vector null access
// ------------------------------------------------------------

// Returns true if element i is null.
inline bool is_null_at(const QVector* v, size_t i) {
    if (!v) return false;
    return q_vec_is_null_at(*v, i);
}

// Marks element i null in a vector you just created.
inline void set_null(QVector* v, size_t i) {
    if (!v || i >= v->count) panic("set_null: index out of range");
    q_vec_mark_null(*v, i);
}

// Read-only view over the Arrow validity bitmap (bit set = valid, LSB first).
// Returns an empty slice when the vector has no nulls.
inline QSlice<const uint8_t> validity_bitmap(const QVector* v) {
    if (!v || v->null_count == 0 || !v->validity) {
        return QSlice<const uint8_t>{nullptr, 0};
    }
    return QSlice<const uint8_t>{v->validity, quark::vec::bitmap_bytes(v->count)};
}

// ------------------------------------------------------------
// Vector metadata
// ------------------------------------------------------------

inline size_t vec_size(const QVector* v) {
    return v ? v->count : 0;
}

inline bool vec_has_nulls(const QVector* v) {
    return v && v->null_count > 0;
}

inline size_t vec_null_count(const QVector* v) {
    return v ? v->null_count : 0;
}

inline QVector::Type vec_dtype(const QVector* v) {
    if (!v) panic("vec_dtype: null vector pointer");
    return v->type;
}

inline const char* vec_dtype_name(const QVector* v) {
    if (!v) panic("vec_dtype_name: null vector pointer");
    return q_vec_dtype_name(*v);
}

// ------------------------------------------------------------
// Vector constructors — GC-allocated, zeroed, ready to fill
// ------------------------------------------------------------

inline QVector* new_f64(size_t n)      { return q_vec_alloc(QVector::Type::F64, n); }
inline QVector* new_i64(size_t n)      { return q_vec_alloc(QVector::Type::I64, n); }
inline QVector* new_bool_vec(size_t n) { return q_vec_alloc(QVector::Type::BOOL, n); }

// ------------------------------------------------------------
// Dict accessors
// ------------------------------------------------------------

// Get a value by string key. Returns null QValue if not found.
inline QValue dict_get(QDict* d, const char* key) {
    if (!d || !key) return qv_null();
    auto it = d->entries.find(QDictKey(key));
    if (it == d->entries.end()) return qv_null();
    return it->second;
}

// Set a key/value pair. Creates the key if it doesn't exist.
inline void dict_set(QDict* d, const char* key, QValue value) {
    if (!d || !key) return;
    d->entries[QDictKey(key)] = value;
}

// Returns the number of entries in the dict.
inline size_t dict_size(const QDict* d) {
    return d ? d->entries.size() : 0;
}

// Returns true if the dict contains the given key.
inline bool dict_has(const QDict* d, const char* key) {
    if (!d || !key) return false;
    return d->entries.find(QDictKey(key)) != d->entries.end();
}

// ------------------------------------------------------------
// Closure call helpers — call a Quark fn value from native code
// ------------------------------------------------------------

inline QValue call(QClosure* fn) {
    if (!fn || !fn->func) panic("call: null closure");
    return reinterpret_cast<QClFunc0>(fn->func)(fn);
}

inline QValue call(QClosure* fn, QValue a0) {
    if (!fn || !fn->func) panic("call: null closure");
    return reinterpret_cast<QClFunc1>(fn->func)(fn, a0);
}

inline QValue call(QClosure* fn, QValue a0, QValue a1) {
    if (!fn || !fn->func) panic("call: null closure");
    return reinterpret_cast<QClFunc2>(fn->func)(fn, a0, a1);
}

inline QValue call(QClosure* fn, QValue a0, QValue a1, QValue a2) {
    if (!fn || !fn->func) panic("call: null closure");
    return reinterpret_cast<QClFunc3>(fn->func)(fn, a0, a1, a2);
}

} // namespace qext

// ============================================================
// Unboxing helpers used by DispatchExtern call sites in codegen
// ============================================================
// These are emitted directly by the compiler at extern call sites and must
// therefore be available as plain C-linkage inline functions (not in a
// namespace). Extension authors may also use them directly if preferred.
//
// Each helper validates QValue::type before reading the union field.
// On type mismatch the program panics with a clear diagnostic instead
// of silently reinterpreting the union bits as the wrong type.

inline const char* q_as_type_name(QValue::ValueType t) {
    static const char* names[] = {
        "int", "float", "str", "bool", "null",
        "list", "vector", "dict", "fn", "result", "resource"
    };
    int idx = static_cast<int>(t);
    if (idx >= 0 && idx <= 10) return names[idx];
    return "unknown";
}

[[noreturn]] inline void q_as_type_panic(const char* expected, QValue::ValueType got) {
    q_runtime_reportf("runtime error: expected %s, got %s\n", expected, q_as_type_name(got));
    std::exit(1);
}

inline int64_t q_as_int(QValue v) {
    if (v.type != QValue::VAL_INT) q_as_type_panic("int", v.type);
    return v.data.int_val;
}
inline double q_as_float(QValue v) {
    if (v.type != QValue::VAL_FLOAT) q_as_type_panic("float", v.type);
    return v.data.float_val;
}
inline bool q_as_bool(QValue v) {
    if (v.type != QValue::VAL_BOOL) q_as_type_panic("bool", v.type);
    return v.data.bool_val;
}
inline const char* q_as_str(QValue v) {
    if (v.type != QValue::VAL_STRING) q_as_type_panic("str", v.type);
    return v.data.string_val;
}
inline QVector* q_as_vector(QValue v) {
    if (v.type != QValue::VAL_VECTOR) q_as_type_panic("vector", v.type);
    return v.data.vector_val;
}
inline QList* q_as_list(QValue v) {
    if (v.type != QValue::VAL_LIST) q_as_type_panic("list", v.type);
    return v.data.list_val;
}
inline QDict* q_as_dict(QValue v) {
    if (v.type != QValue::VAL_DICT) q_as_type_panic("dict", v.type);
    return v.data.dict_val;
}
inline QClosure* q_as_closure(QValue v) {
    if (v.type != QValue::VAL_FUNC) q_as_type_panic("fn", v.type);
    return static_cast<QClosure*>(v.data.func_val);
}

// Boxing wrappers for native return values back to QValue.
// Used by wrapExternReturn() in codegen.
inline QValue qv_vector_ptr(QVector* v) {
    QValue q;
    q.type = QValue::VAL_VECTOR;
    q.data.vector_val = v;
    return q;
}

inline QValue qv_list_ptr(QList* v) {
    QValue q;
    q.type = QValue::VAL_LIST;
    q.data.list_val = v;
    return q;
}

inline QValue qv_dict_ptr(QDict* v) {
    QValue q;
    q.type = QValue::VAL_DICT;
    q.data.dict_val = v;
    return q;
}

inline QValue qv_closure_ptr(QClosure* v) {
    QValue q;
    q.type = QValue::VAL_FUNC;
    q.data.func_val = static_cast<void*>(v);
    return q;
}

#endif // QUARK_EXT_API_HPP
