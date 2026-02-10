// quark/types/dict.hpp - Dict operations using std::unordered_map
#ifndef QUARK_TYPES_DICT_HPP
#define QUARK_TYPES_DICT_HPP

#include "../core/value.hpp"
#include "../core/constructors.hpp"
#include <unordered_map>
#include <string>
#include <cstdio>

struct QDict {
    std::unordered_map<std::string, QValue> entries;
};

inline QValue qv_dict() {
    QValue q;
    q.type = QValue::VAL_DICT;
    q.data.dict_val = new QDict();
    return q;
}

inline bool q_require_dict(const QValue& v, const char* action) {
    if (v.type == QValue::VAL_DICT) {
        return true;
    }
    std::fprintf(stderr, "runtime error: %s expects dict\n", action);
    return false;
}

inline bool q_require_string_key(const QValue& key) {
    if (key.type == QValue::VAL_STRING) {
        return true;
    }
    std::fprintf(stderr, "runtime error: dict key must be string\n");
    return false;
}

inline QValue q_dict_get(QValue dict, QValue key) {
    if (!q_require_dict(dict, "dict get")) {
        return qv_null();
    }
    if (!q_require_string_key(key)) {
        return qv_null();
    }
    if (!dict.data.dict_val) {
        return qv_null();
    }
    auto it = dict.data.dict_val->entries.find(key.data.string_val ? key.data.string_val : "");
    if (it == dict.data.dict_val->entries.end()) {
        return qv_null();
    }
    return it->second;
}

inline QValue q_dict_set(QValue dict, QValue key, QValue value) {
    if (!q_require_dict(dict, "dict set")) {
        return qv_null();
    }
    if (!q_require_string_key(key)) {
        return qv_null();
    }
    if (!dict.data.dict_val) {
        dict.data.dict_val = new QDict();
    }
    dict.data.dict_val->entries[std::string(key.data.string_val ? key.data.string_val : "")] = value;
    return dict;
}

inline QValue q_dict_has(QValue dict, QValue key) {
    if (!q_require_dict(dict, "dict has")) {
        return qv_bool(false);
    }
    if (!q_require_string_key(key)) {
        return qv_bool(false);
    }
    if (!dict.data.dict_val) {
        return qv_bool(false);
    }
    auto it = dict.data.dict_val->entries.find(key.data.string_val ? key.data.string_val : "");
    return qv_bool(it != dict.data.dict_val->entries.end());
}

inline int q_dict_size(QValue dict) {
    if (!q_require_dict(dict, "dict size")) {
        return 0;
    }
    if (!dict.data.dict_val) {
        return 0;
    }
    return static_cast<int>(dict.data.dict_val->entries.size());
}

#endif // QUARK_TYPES_DICT_HPP
