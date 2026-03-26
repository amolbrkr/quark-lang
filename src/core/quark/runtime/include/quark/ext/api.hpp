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
// Vector accessors — QSlice views over internal buffers
// ------------------------------------------------------------

// Returns a read-only view over the f64 data. Panics if dtype != F64.
inline QSlice<const double> as_f64(const QVector* v) {
    if (!v || v->type != QVector::Type::F64) {
        panic("as_f64: vector dtype is not f64");
    }
    const auto& buf = std::get<QVecF64>(v->storage);
    return QSlice<const double>{buf.data(), buf.size()};
}

// Returns a mutable view over the f64 data. Panics if dtype != F64.
inline QSlice<double> as_f64_mut(QVector* v) {
    if (!v || v->type != QVector::Type::F64) {
        panic("as_f64_mut: vector dtype is not f64");
    }
    auto& buf = std::get<QVecF64>(v->storage);
    return QSlice<double>{buf.data(), buf.size()};
}

// Returns a read-only view over the i64 data. Panics if dtype != I64.
inline QSlice<const int64_t> as_i64(const QVector* v) {
    if (!v || v->type != QVector::Type::I64) {
        panic("as_i64: vector dtype is not i64");
    }
    const auto& buf = std::get<QVecI64>(v->storage);
    return QSlice<const int64_t>{buf.data(), buf.size()};
}

// Returns a mutable view over the i64 data. Panics if dtype != I64.
inline QSlice<int64_t> as_i64_mut(QVector* v) {
    if (!v || v->type != QVector::Type::I64) {
        panic("as_i64_mut: vector dtype is not i64");
    }
    auto& buf = std::get<QVecI64>(v->storage);
    return QSlice<int64_t>{buf.data(), buf.size()};
}

// Returns a read-only view over the bool data (packed as uint8_t, 0/1).
// Panics if dtype != BOOL.
inline QSlice<const uint8_t> as_bool_vec(const QVector* v) {
    if (!v || v->type != QVector::Type::BOOL) {
        panic("as_bool_vec: vector dtype is not bool");
    }
    const auto& buf = std::get<QVecU8>(v->storage);
    return QSlice<const uint8_t>{buf.data(), buf.size()};
}

// Returns a mutable view over the bool data. Panics if dtype != BOOL.
inline QSlice<uint8_t> as_bool_vec_mut(QVector* v) {
    if (!v || v->type != QVector::Type::BOOL) {
        panic("as_bool_vec_mut: vector dtype is not bool");
    }
    auto& buf = std::get<QVecU8>(v->storage);
    return QSlice<uint8_t>{buf.data(), buf.size()};
}

// ------------------------------------------------------------
// Vector null mask access
// ------------------------------------------------------------

// Returns true if element i is null. Safe for vectors without null masks.
inline bool is_null_at(const QVector* v, size_t i) {
    if (!v) return false;
    return q_vec_is_null_at(*v, i);
}

// Returns a read-only view over the null mask bytes (0=valid, 1=null).
// Returns an empty slice (data=nullptr, size=0) if the vector has no null mask.
inline QSlice<const uint8_t> null_mask(const QVector* v) {
    if (!v || !v->has_nulls) {
        return QSlice<const uint8_t>{nullptr, 0};
    }
    const auto& mask = v->nulls.is_null;
    return QSlice<const uint8_t>{mask.data(), mask.size()};
}

// ------------------------------------------------------------
// Vector metadata
// ------------------------------------------------------------

inline size_t vec_size(const QVector* v) {
    return v ? v->count : 0;
}

inline bool vec_has_nulls(const QVector* v) {
    return v && v->has_nulls;
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
// Vector constructors — GC-allocated, ready-to-use
// ------------------------------------------------------------

// Create an f64 vector with n elements initialized to 0.0.
inline QVector* new_f64(size_t n) {
    QValue qv = qv_vector(static_cast<int>(n));
    QVector* v = qv.data.vector_val;
    auto& buf = std::get<QVecF64>(v->storage);
    buf.resize(n, 0.0);
    v->count = n;
    return v;
}

// Create an i64 vector with n elements initialized to 0.
inline QVector* new_i64(size_t n) {
    QValue qv = qv_vector_i64(static_cast<int>(n));
    QVector* v = qv.data.vector_val;
    auto& buf = std::get<QVecI64>(v->storage);
    buf.resize(n, static_cast<int64_t>(0));
    v->count = n;
    return v;
}

// Create a bool vector with n elements initialized to false.
inline QVector* new_bool_vec(size_t n) {
    QValue qv = qv_vector_bool(static_cast<int>(n));
    QVector* v = qv.data.vector_val;
    auto& buf = std::get<QVecU8>(v->storage);
    buf.resize(n, static_cast<uint8_t>(0));
    v->count = n;
    return v;
}

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

inline int64_t     q_as_int(QValue v)     { return v.data.int_val; }
inline double      q_as_float(QValue v)   { return v.data.float_val; }
inline bool        q_as_bool(QValue v)    { return v.data.bool_val; }
inline const char* q_as_str(QValue v)     { return v.data.string_val; }
inline QVector*    q_as_vector(QValue v)  { return v.data.vector_val; }
inline QList*      q_as_list(QValue v)    { return v.data.list_val; }
inline QDict*      q_as_dict(QValue v)    { return v.data.dict_val; }
inline QClosure*   q_as_closure(QValue v) { return static_cast<QClosure*>(v.data.func_val); }

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
