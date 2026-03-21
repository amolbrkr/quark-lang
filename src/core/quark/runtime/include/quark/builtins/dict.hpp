// quark/builtins/dict.hpp - Dict-related builtins
#ifndef QUARK_BUILTINS_DICT_HPP
#define QUARK_BUILTINS_DICT_HPP

#include "../core/value.hpp"
#include "../builtins/conversion.hpp"
#include "../types/dict.hpp"

// dget(dict, key) -> any
// Key is converted to string via q_str when needed.
inline QValue q_dget(QValue dict, QValue key) {
    if (key.type != QValue::VAL_STRING) {
        key = q_str(key);
    }
    return q_dict_get(dict, key);
}

// dset(dict, key, value) -> dict
// Key is converted to string via q_str when needed.
inline QValue q_dset(QValue dict, QValue key, QValue value) {
    if (key.type != QValue::VAL_STRING) {
        key = q_str(key);
    }
    return q_dict_set(dict, key, value);
}

// dkeys(dict) -> list[str]
inline QValue q_dkeys(QValue dict) {
    if (dict.type != QValue::VAL_DICT || !dict.data.dict_val) {
        q_runtime_reportf("runtime error: dkeys() expects dict\n");
        std::exit(1);
    }

    QValue out = qv_list(static_cast<int>(dict.data.dict_val->entries.size()));
    for (const auto& kv : dict.data.dict_val->entries) {
        out.data.list_val->push_back(qv_string(kv.first.c_str()));
    }
    return out;
}

// dvalues(dict) -> list
inline QValue q_dvalues(QValue dict) {
    if (dict.type != QValue::VAL_DICT || !dict.data.dict_val) {
        q_runtime_reportf("runtime error: dvalues() expects dict\n");
        std::exit(1);
    }

    QValue out = qv_list(static_cast<int>(dict.data.dict_val->entries.size()));
    for (const auto& kv : dict.data.dict_val->entries) {
        out.data.list_val->push_back(kv.second);
    }
    return out;
}

// ditems(dict) -> list[dict{key:..., value:...}]
inline QValue q_ditems(QValue dict) {
    if (dict.type != QValue::VAL_DICT || !dict.data.dict_val) {
        q_runtime_reportf("runtime error: ditems() expects dict\n");
        std::exit(1);
    }

    QValue out = qv_list(static_cast<int>(dict.data.dict_val->entries.size()));
    for (const auto& kv : dict.data.dict_val->entries) {
        QValue item = qv_dict();
        item = q_dict_set(item, qv_string("key"), qv_string(kv.first.c_str()));
        item = q_dict_set(item, qv_string("value"), kv.second);
        out.data.list_val->push_back(item);
    }
    return out;
}

#endif // QUARK_BUILTINS_DICT_HPP
