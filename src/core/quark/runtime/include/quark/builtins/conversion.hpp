// quark/builtins/conversion.hpp - Type conversion operations
#ifndef QUARK_BUILTINS_CONVERSION_HPP
#define QUARK_BUILTINS_CONVERSION_HPP

#include "../core/value.hpp"
#include "../core/constructors.hpp"
#include "../core/truthy.hpp"
#include "../core/format.hpp"
#include "repr.hpp"
#include "../types/dict.hpp"
#include "../types/resource.hpp"
#include "../types/table.hpp"
#include <cstdio>
#include <cstdlib>
#include <cstring>

// Get length of string or list
inline QValue q_len(QValue v) {
    switch (v.type) {
        case QValue::VAL_STRING:
            return qv_int(v.data.string_val ? static_cast<long long>(strlen(v.data.string_val)) : 0);
        case QValue::VAL_LIST:
            return qv_int(v.data.list_val ? static_cast<long long>(v.data.list_val->size()) : 0);
        case QValue::VAL_VECTOR:
            return qv_int(static_cast<long long>(q_vec_size(v)));
        case QValue::VAL_DICT:
            return qv_int(v.data.dict_val ? static_cast<long long>(v.data.dict_val->entries.size()) : 0);
        default:
            q_runtime_reportf("runtime error: len() expects str, list, dict, or vec\n");
            std::exit(1);
    }
}

// Generic iterable index access used by for-loop lowering
inline QValue q_iter_get(QValue iterable, QValue index) {
    if (iterable.type == QValue::VAL_LIST || iterable.type == QValue::VAL_STRING) {
        return q_get(iterable, index);
    }
    if (iterable.type != QValue::VAL_VECTOR) {
        q_runtime_reportf("runtime error: for-loop iterable must be list, string, or vec\n");
        std::exit(1);
    }
    if (!q_vec_has_valid_handle(iterable) || !q_vec_validate(*iterable.data.vector_val)) {
        q_runtime_reportf("runtime error: for-loop iterable vec is invalid\n");
        std::exit(1);
    }
    if (index.type != QValue::VAL_INT) {
        q_runtime_reportf("runtime error: iterable index must be int\n");
        std::exit(1);
    }

    long long idx = index.data.int_val;
    long long len = static_cast<long long>(iterable.data.vector_val->count);
    if (idx < 0) idx = len + idx;
    if (idx < 0 || idx >= len) {
        return qv_null();
    }

    // A missing entry is an error (D12), as for v[i].
    QValue out = q_vec_box_at(*iterable.data.vector_val, static_cast<size_t>(idx));
    if (out.type == QValue::VAL_NULL) q_vec_missing(idx);
    return out;
}

// A list keeps its length for the whole for loop over it: a body that pushes,
// pops, inserts or removes is a runtime error at the `for` (loc; nullptr
// keeps the current location, for loops in std/ code). Writes that keep the
// length (`xs[i] = v`) are allowed. Strings and vecs can't change length.
[[noreturn]] inline void q_list_resized(long long was, long long now, const QSourceLoc* loc) {
    if (loc != nullptr) q_runtime_loc = loc;
    q_runtime_reportf("runtime error: list changed size during the for loop over it (length %lld at the start, %lld now)\n", was, now);
    std::exit(1);
}

// The length of a `for` loop's iterable of unknown type: anything but a list,
// string or vec is an error before the first iteration, also when it is empty.
inline long long q_for_len(QValue it) {
    switch (it.type) {
        case QValue::VAL_LIST:
            if (it.data.list_val != nullptr) return static_cast<long long>(it.data.list_val->size());
            break;
        case QValue::VAL_STRING:
            if (it.data.string_val != nullptr) return static_cast<long long>(strlen(it.data.string_val));
            break;
        case QValue::VAL_VECTOR:
            return static_cast<long long>(q_vec_size(it));
        default:
            break;
    }
    q_runtime_reportf("runtime error: for-loop iterable must be list, string, or vec, got %s\n", q_type_name(it.type));
    std::exit(1);
}

// The list a `for` over a value the analyzer typed as a list iterates.
inline QList* q_for_list(QValue v) {
    if (v.type != QValue::VAL_LIST || v.data.list_val == nullptr) {
        q_runtime_reportf("runtime error: for-loop iterable must be list, string, or vec\n");
        std::exit(1);
    }
    return v.data.list_val;
}

inline long long q_list_len(const QList* xs) { return static_cast<long long>(xs->size()); }

// Element i of a list the loop test has kept in range.
inline QValue q_list_at(const QList* xs, long long i) { return (*xs)[static_cast<size_t>(i)]; }

// The test of an index loop over a list with n elements at the start.
inline bool q_for_list_more(const QList* xs, long long n, long long i, const QSourceLoc* loc) {
    const long long now = static_cast<long long>(xs->size());
    if (__builtin_expect(now != n, 0)) q_list_resized(n, now, loc);
    return i < n;
}

// The same test for an iterable of unknown type (q_iter_get loops).
inline bool q_for_more(QValue it, long long n, long long i, const QSourceLoc* loc) {
    if (it.type == QValue::VAL_LIST && it.data.list_val != nullptr) {
        return q_for_list_more(it.data.list_val, n, i, loc);
    }
    return i < n;
}

// Convert value to string
inline QValue q_str(QValue v) {
    char buffer[256];
    switch (v.type) {
        case QValue::VAL_INT:
            snprintf(buffer, sizeof(buffer), "%lld", v.data.int_val);
            return qv_string(buffer);
        case QValue::VAL_FLOAT:
            q_format_float(v.data.float_val, buffer, sizeof(buffer));
            return qv_string(buffer);
        case QValue::VAL_BOOL:
            return qv_string(v.data.bool_val ? "true" : "false");
        case QValue::VAL_STRING:
            return v.data.string_val ? v : qv_string("");
        case QValue::VAL_NULL:
            return qv_string("null");
        default:
            // Containers, structs, results, functions, resources, tables.
            return qv_string(q_repr(v).c_str());
    }
}

// Convert value to integer
inline QValue q_int(QValue v) {
    switch (v.type) {
        case QValue::VAL_INT:
            return v;
        case QValue::VAL_FLOAT:
            return qv_int(static_cast<long long>(v.data.float_val));
        case QValue::VAL_BOOL:
            return qv_int(v.data.bool_val ? 1 : 0);
        case QValue::VAL_STRING: {
            if (!v.data.string_val || v.data.string_val[0] == '\0') {
                q_runtime_reportf("runtime error: to_int() cannot convert empty string\n");
                std::exit(1);
            }
            char* end = nullptr;
            long long result = strtoll(v.data.string_val, &end, 10);
            if (end == v.data.string_val || *end != '\0') {
                q_runtime_reportf("runtime error: to_int() cannot parse '%s' as integer\n", v.data.string_val);
                std::exit(1);
            }
            return qv_int(result);
        }
        default: {
            const char* tname = q_type_name(v.type);
            q_runtime_reportf("runtime error: to_int() cannot convert %s to int\n", tname);
            std::exit(1);
        }
    }
}

// Convert value to float
inline QValue q_float(QValue v) {
    switch (v.type) {
        case QValue::VAL_INT:
            return qv_float(static_cast<double>(v.data.int_val));
        case QValue::VAL_FLOAT:
            return v;
        case QValue::VAL_BOOL:
            return qv_float(v.data.bool_val ? 1.0 : 0.0);
        case QValue::VAL_STRING: {
            if (!v.data.string_val || v.data.string_val[0] == '\0') {
                q_runtime_reportf("runtime error: to_float() cannot convert empty string\n");
                std::exit(1);
            }
            char* end = nullptr;
            double result = strtod(v.data.string_val, &end);
            if (end == v.data.string_val || *end != '\0') {
                q_runtime_reportf("runtime error: to_float() cannot parse '%s' as float\n", v.data.string_val);
                std::exit(1);
            }
            return qv_float(result);
        }
        default: {
            const char* tname = q_type_name(v.type);
            q_runtime_reportf("runtime error: to_float() cannot convert %s to float\n", tname);
            std::exit(1);
        }
    }
}

// Convert value to boolean
inline QValue q_bool(QValue v) {
    return qv_bool(q_truthy(v));
}

// Return runtime type name as string
inline QValue q_type(QValue v) {
    switch (v.type) {
        case QValue::VAL_INT:
            return qv_string("int");
        case QValue::VAL_FLOAT:
            return qv_string("float");
        case QValue::VAL_STRING:
            return qv_string("str");
        case QValue::VAL_BOOL:
            return qv_string("bool");
        case QValue::VAL_NULL:
            return qv_string("null");
        case QValue::VAL_LIST:
            return qv_string("list");
        case QValue::VAL_DICT:
            return qv_string("dict");
        case QValue::VAL_FUNC:
            return qv_string("fn");
        case QValue::VAL_RESULT:
            return qv_string("result");
        case QValue::VAL_RESOURCE:
            return qv_string("resource");
        case QValue::VAL_STRUCT:
            if (v.data.struct_val && v.data.struct_val->def) {
                return qv_string(v.data.struct_val->def->name);
            }
            return qv_string("struct");
        case QValue::VAL_TABLE:
            return qv_string(q_table_type_name(v.data.table_val));
        case QValue::VAL_VECTOR: {
            if (!q_vec_has_valid_handle(v) || !q_vec_validate(*v.data.vector_val)) {
                return qv_string("vec[invalid]");
            }
            char buffer[64];
            std::snprintf(buffer, sizeof(buffer), "vec[%s]", q_vec_dtype_name(*v.data.vector_val));
            return qv_string(buffer);
        }
        default:
            return qv_string("unknown");
    }
}

// Reports a method call on a receiver whose runtime type has no such method.
// Used by dynamic method dispatch (receiver type unknown at compile time).
[[noreturn]] inline void q_no_method(QValue receiver, const char* method) {
    QValue name = q_type(receiver);
    q_runtime_reportf("runtime error: type '%s' has no method '%s'\n",
                      name.data.string_val ? name.data.string_val : "unknown", method);
    std::exit(1);
}

inline QValue q_is_ok_builtin(QValue v) {
    return qv_bool(q_is_ok(v));
}

inline QValue q_is_err_builtin(QValue v) {
    return qv_bool(v.type == QValue::VAL_RESULT && v.data.result_val && !v.data.result_val->is_ok);
}

inline QValue q_unwrap(QValue v) {
    if (v.type != QValue::VAL_RESULT || !v.data.result_val) {
        q_runtime_reportf("runtime error: unwrap expects result\n");
        std::exit(1);
    }
    if (v.data.result_val->is_ok) {
        return v.data.result_val->payload;
    }

    QValue err = v.data.result_val->payload;
    QValue errText = q_str(err);
    const char* msg = (errText.type == QValue::VAL_STRING && errText.data.string_val)
        ? errText.data.string_val
        : "<error>";
    q_runtime_reportf("runtime error: unwrap on err: %s\n", msg);
    std::exit(1);
}

#endif // QUARK_BUILTINS_CONVERSION_HPP
