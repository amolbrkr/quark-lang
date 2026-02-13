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
            return qv_int(v.data.vector_val ? static_cast<long long>(v.data.vector_val->data.size()) : 0);
        case QValue::VAL_DICT:
            return qv_int(v.data.dict_val ? static_cast<long long>(v.data.dict_val->entries.size()) : 0);
        default:
            return qv_int(0);
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
            snprintf(buffer, sizeof(buffer), "[vector len=%zu]",
                     v.data.vector_val ? v.data.vector_val->data.size() : 0);
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

#endif // QUARK_BUILTINS_CONVERSION_HPP
