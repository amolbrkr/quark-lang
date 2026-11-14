// quark/builtins/dict.hpp - Dict-related builtins
#ifndef QUARK_BUILTINS_DICT_HPP
#define QUARK_BUILTINS_DICT_HPP

#include "../core/value.hpp"
#include "../builtins/conversion.hpp"
#include "../types/dict.hpp"

// q_dict_key checks the key of d.get(k) / d.set(k, v): dict keys are str,
// and a key of another type is an error rather than converted, since
// converting would make d.set(1, ...) and d.set('1', ...) the same key.
inline void q_dict_key(QValue key, const char* method) {
    if (key.type != QValue::VAL_STRING || key.data.string_val == nullptr) {
        q_runtime_reportf("runtime error: %s: dict keys are str, got %s; convert it with to_str()\n", method, q_type_name(key.type));
        std::exit(1);
    }
}

// dget(dict, key) -> any
inline QValue q_dget(QValue dict, QValue key) {
    q_dict_key(key, "get()");
    return q_dict_get(dict, key);
}

// dset(dict, key, value) -> dict
inline QValue q_dset(QValue dict, QValue key, QValue value) {
    q_dict_key(key, "set()");
    return q_dict_set(dict, key, value);
}

// dkeys(dict) -> list[str]
inline QValue q_dkeys(QValue dict) {
    if (dict.type != QValue::VAL_DICT || !dict.data.dict_val) {
        q_runtime_reportf("runtime error: dkeys() expects dict\n");
        std::exit(1);
    }

    QValue out = qv_list(static_cast<int>(dict.data.dict_val->entries.size()), &q_elem_str);
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

    QValue out = qv_list(static_cast<int>(dict.data.dict_val->entries.size()), dict.data.dict_val->elem);
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
