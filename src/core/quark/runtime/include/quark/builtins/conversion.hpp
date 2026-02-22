// quark/builtins/conversion.hpp - Type conversion operations
#ifndef QUARK_BUILTINS_CONVERSION_HPP
#define QUARK_BUILTINS_CONVERSION_HPP

#include "../core/value.hpp"
#include "../core/constructors.hpp"
#include "../core/truthy.hpp"
#include "../types/dict.hpp"
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
            return qv_int(0);
    }
}

// Generic iterable index access used by for-loop lowering
inline QValue q_iter_get(QValue iterable, QValue index) {
    if (iterable.type == QValue::VAL_LIST || iterable.type == QValue::VAL_STRING) {
        return q_get(iterable, index);
    }
    if (iterable.type != QValue::VAL_VECTOR) {
        return qv_null();
    }
    if (!q_vec_has_valid_handle(iterable) || !q_vec_validate(*iterable.data.vector_val)) {
        return qv_null();
    }
    if (index.type != QValue::VAL_INT) {
        return qv_null();
    }

    long long idx = index.data.int_val;
    long long len = static_cast<long long>(iterable.data.vector_val->count);
    if (idx < 0) idx = len + idx;
    if (idx < 0 || idx >= len) {
        return qv_null();
    }

    size_t pos = static_cast<size_t>(idx);
    const QVector& vec = *iterable.data.vector_val;
    if (q_vec_is_null_at(vec, pos)) {
        return qv_null();
    }

    switch (vec.type) {
        case QVector::Type::F64: {
            const auto& values = std::get<std::vector<double>>(vec.storage);
            return qv_float(values[pos]);
        }
        case QVector::Type::I64: {
            const auto& values = std::get<std::vector<int64_t>>(vec.storage);
            return qv_int(static_cast<long long>(values[pos]));
        }
        case QVector::Type::BOOL: {
            const auto& values = std::get<std::vector<uint8_t>>(vec.storage);
            return qv_bool(values[pos] != 0);
        }
        case QVector::Type::STR: {
            const auto& values = std::get<QStringStorage>(vec.storage);
            uint32_t start = values.offsets[pos];
            uint32_t end = values.offsets[pos + 1];
            std::string s(values.bytes.data() + start, values.bytes.data() + end);
            return qv_string(s.c_str());
        }
        default:
            return qv_null();
    }
}

// Convert value to string
inline QValue q_str(QValue v) {
    char buffer[256];
    switch (v.type) {
        case QValue::VAL_INT:
            snprintf(buffer, sizeof(buffer), "%lld", v.data.int_val);
            return qv_string(buffer);
        case QValue::VAL_FLOAT:
            snprintf(buffer, sizeof(buffer), "%g", v.data.float_val);
            return qv_string(buffer);
        case QValue::VAL_BOOL:
            return qv_string(v.data.bool_val ? "true" : "false");
        case QValue::VAL_STRING:
            return v.data.string_val ? v : qv_string("");
        case QValue::VAL_NULL:
            return qv_string("null");
        case QValue::VAL_LIST:
            snprintf(buffer, sizeof(buffer), "[list len=%zu]",
                     v.data.list_val ? v.data.list_val->size() : 0);
            return qv_string(buffer);
        case QValue::VAL_VECTOR:
            snprintf(buffer, sizeof(buffer), "[vector len=%d]", q_vec_size(v));
            return qv_string(buffer);
        case QValue::VAL_DICT:
            snprintf(buffer, sizeof(buffer), "[dict len=%zu]",
                     v.data.dict_val ? v.data.dict_val->entries.size() : 0);
            return qv_string(buffer);
        case QValue::VAL_FUNC:
            return qv_string("<function>");
        default:
            return qv_string("<value>");
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
        case QValue::VAL_STRING:
            return qv_int(v.data.string_val ? atoll(v.data.string_val) : 0);
        default:
            return qv_int(0);
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
        case QValue::VAL_STRING:
            return qv_float(v.data.string_val ? atof(v.data.string_val) : 0.0);
        default:
            return qv_float(0.0);
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
            return qv_string("func");
        case QValue::VAL_RESULT:
            return qv_string("result");
        case QValue::VAL_VECTOR: {
            if (!q_vec_has_valid_handle(v) || !q_vec_validate(*v.data.vector_val)) {
                return qv_string("vector[invalid]");
            }
            char buffer[64];
            std::snprintf(buffer, sizeof(buffer), "vector[%s]", q_vec_dtype_name(*v.data.vector_val));
            return qv_string(buffer);
        }
        default:
            return qv_string("unknown");
    }
}

#endif // QUARK_BUILTINS_CONVERSION_HPP
