// quark/types/dict.hpp - Dict operations using std::unordered_map
#ifndef QUARK_TYPES_DICT_HPP
#define QUARK_TYPES_DICT_HPP

#include "../core/value.hpp"
#include "../core/constructors.hpp"
#include <unordered_map>
#include <string>
#include <cstdio>
#include <cstdlib>

// GC-aware string key type so key payload bytes are allocated via q_allocator.
using QDictKey = std::basic_string<char, std::char_traits<char>, q_allocator<char>>;

// QDictMap: unordered_map whose internal nodes and key payload bytes are
// GC-allocated so the collector can track all dictionary-owned memory.
using QDictMap = std::unordered_map<
    QDictKey, QValue,
    std::hash<QDictKey>,
    std::equal_to<QDictKey>,
    q_allocator<std::pair<const QDictKey, QValue>>
>;

struct QDict {
    QDictMap entries;
};

inline QValue qv_dict() {
    QValue q;
    q.type = QValue::VAL_DICT;
    q.data.dict_val = q_new<QDict>();
    return q;
}

inline bool q_require_dict(const QValue& v, const char* action) {
    if (v.type == QValue::VAL_DICT) {
        return true;
    }
    std::fprintf(stderr, "runtime error: %s expects dict\n", action);
    std::exit(1);
}

inline bool q_require_string_key(const QValue& key) {
    if (key.type == QValue::VAL_STRING) {
        return true;
    }
    std::fprintf(stderr, "runtime error: dict key must be string\n");
    std::exit(1);
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
    const char* raw_key = key.data.string_val ? key.data.string_val : "";
    auto it = dict.data.dict_val->entries.find(QDictKey(raw_key));
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
        dict.data.dict_val = q_new<QDict>();
    }
    const char* raw_key = key.data.string_val ? key.data.string_val : "";
    dict.data.dict_val->entries[QDictKey(raw_key)] = value;
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
    const char* raw_key = key.data.string_val ? key.data.string_val : "";
    auto it = dict.data.dict_val->entries.find(QDictKey(raw_key));
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
