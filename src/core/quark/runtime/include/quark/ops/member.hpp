// quark/ops/member.hpp - Dict member access
#ifndef QUARK_OPS_MEMBER_HPP
#define QUARK_OPS_MEMBER_HPP

#include "../core/value.hpp"
#include "../core/constructors.hpp"
#include <cstring>
#include <cstdio>

// Dict member read: d.key → q_member_get(d, "key")
inline QValue q_member_get(QValue obj, const char* member) {
    if (obj.type == QValue::VAL_NULL) {
        fprintf(stderr, "runtime error: cannot access member '%s' on null\n", member);
        return qv_null();
    }

    if (obj.type == QValue::VAL_DICT) {
        return q_dict_get(obj, qv_string(member));
    }

    const char* type_names[] = {"int", "float", "string", "bool", "null", "list", "vector", "dict", "func", "result"};
    const char* type_name = (obj.type >= 0 && obj.type <= 9) ? type_names[obj.type] : "unknown";
    fprintf(stderr, "runtime error: dot access is only supported on dict; got type '%s'\n", type_name);
    return qv_null();
}

// Dict member write: d.key = value → q_member_set(d, "key", value)
inline QValue q_member_set(QValue obj, const char* member, QValue value) {
    if (obj.type == QValue::VAL_DICT) {
        return q_dict_set(obj, qv_string(member), value);
    }
    fprintf(stderr, "runtime error: cannot set member '%s' on non-dict type\n", member);
    return qv_null();
}

#endif // QUARK_OPS_MEMBER_HPP
