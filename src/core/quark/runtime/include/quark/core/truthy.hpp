// quark/core/truthy.hpp - Truthiness checking
#ifndef QUARK_CORE_TRUTHY_HPP
#define QUARK_CORE_TRUTHY_HPP

#include "value.hpp"
#include "../types/vector.hpp"
#include "../types/dict.hpp"
#include <cstring>

// Check if a value is truthy (used for conditions)
inline bool q_truthy(QValue v) {
    switch (v.type) {
        case QValue::VAL_BOOL:
            return v.data.bool_val;
        case QValue::VAL_INT:
            return v.data.int_val != 0;
        case QValue::VAL_FLOAT:
            return v.data.float_val != 0.0;
        case QValue::VAL_STRING:
            return v.data.string_val != nullptr && strlen(v.data.string_val) > 0;
        case QValue::VAL_NULL:
            return false;
        case QValue::VAL_LIST:
            return v.data.list_val && !v.data.list_val->empty();
        case QValue::VAL_VECTOR:
            return q_vec_size(v) > 0;
        case QValue::VAL_DICT:
            return v.data.dict_val && !v.data.dict_val->entries.empty();
        case QValue::VAL_FUNC:
            return v.data.func_val != nullptr;
        case QValue::VAL_RESULT:
            return v.data.result_val && v.data.result_val->is_ok;
        default:
            return false;
    }
}

#endif // QUARK_CORE_TRUTHY_HPP
