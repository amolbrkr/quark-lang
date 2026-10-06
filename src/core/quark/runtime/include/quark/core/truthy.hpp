// quark/core/truthy.hpp - Truthiness checking
#ifndef QUARK_CORE_TRUTHY_HPP
#define QUARK_CORE_TRUTHY_HPP

#include "value.hpp"
#include "../types/vector.hpp"
#include "../types/dict.hpp"
#include "../types/resource.hpp"
#include "diagnostics.hpp"
#include <cstring>

// Check if a value is truthy (used for conditions, !, and/or, to_bool).
//
// Results and vectors have no truthiness. Treating an err result as false
// silently discards the error, and a vector comparison such as `v == w`
// yields a vector[bool] whose emptiness says nothing about its elements.
// Both are fatal here; the analyzer rejects them at compile time when the
// type is known.
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
            q_runtime_reportf("runtime error: vector used as a condition; "
                              "use all(v), any(v) or len(v) > 0\n");
            std::exit(1);
        case QValue::VAL_DICT:
            return v.data.dict_val && !v.data.dict_val->entries.empty();
        case QValue::VAL_FUNC:
            return v.data.func_val != nullptr;
        case QValue::VAL_RESULT:
            q_runtime_reportf("runtime error: result used as a condition; "
                              "use is_ok(r), is_err(r) or when\n");
            std::exit(1);
        case QValue::VAL_RESOURCE:
            return q_resource_is_alive(v);
        case QValue::VAL_STRUCT:
            return v.data.struct_val != nullptr;
        case QValue::VAL_TABLE:
            return v.data.table_val != nullptr;
        default:
            return false;
    }
}

#endif // QUARK_CORE_TRUTHY_HPP
