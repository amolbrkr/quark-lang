// quark/ops/member.hpp - Member access dispatch
#ifndef QUARK_OPS_MEMBER_HPP
#define QUARK_OPS_MEMBER_HPP

// Note: This header must be included AFTER all type and builtin headers
// in quark.hpp, since it calls q_len, q_upper, q_lower, q_trim, q_reverse,
// q_pop, q_list_clear, q_list_empty.

#include "../core/value.hpp"
#include "../core/constructors.hpp"
#include <cstring>
#include <cstdio>

inline QValue q_member_get(QValue obj, const char* member) {
    // Null guard
    if (obj.type == QValue::VAL_NULL) {
        fprintf(stderr, "runtime error: cannot access member '%s' on null\n", member);
        return qv_null();
    }

    // List members
    if (obj.type == QValue::VAL_LIST) {
        if (strcmp(member, "length") == 0 || strcmp(member, "size") == 0) {
            return q_len(obj);
        }
        if (strcmp(member, "empty") == 0) {
            return qv_bool(q_list_empty(obj));
        }
        if (strcmp(member, "reverse") == 0) {
            return q_reverse(obj);
        }
        if (strcmp(member, "pop") == 0) {
            return q_pop(obj);
        }
        if (strcmp(member, "clear") == 0) {
            return q_list_clear(obj);
        }
        fprintf(stderr, "runtime error: list has no member '%s'\n", member);
        return qv_null();
    }

    // String members
    if (obj.type == QValue::VAL_STRING) {
        if (strcmp(member, "length") == 0 || strcmp(member, "size") == 0) {
            return q_len(obj);
        }
        if (strcmp(member, "upper") == 0) {
            return q_upper(obj);
        }
        if (strcmp(member, "lower") == 0) {
            return q_lower(obj);
        }
        if (strcmp(member, "trim") == 0) {
            return q_trim(obj);
        }
        fprintf(stderr, "runtime error: string has no member '%s'\n", member);
        return qv_null();
    }

    // Dict members
    if (obj.type == QValue::VAL_DICT) {
        if (strcmp(member, "length") == 0 || strcmp(member, "size") == 0) {
            return q_len(obj);
        }
        // Fall through to key lookup
        return q_dict_get(obj, qv_string(member));
    }

    // Unsupported type
    const char* type_names[] = {"int", "float", "string", "bool", "null", "list", "vector", "dict", "func", "result"};
    const char* type_name = (obj.type >= 0 && obj.type <= 9) ? type_names[obj.type] : "unknown";
    fprintf(stderr, "runtime error: type '%s' has no member '%s'\n", type_name, member);
    return qv_null();
}

// Member method calls with arguments
// obj.method(arg1) → q_member_call1(obj, "method", arg1)
inline QValue q_member_call1(QValue obj, const char* method, QValue arg1) {
    if (obj.type == QValue::VAL_NULL) {
        fprintf(stderr, "runtime error: cannot call method '%s' on null\n", method);
        return qv_null();
    }

    // List methods with 1 arg
    if (obj.type == QValue::VAL_LIST) {
        if (strcmp(method, "push") == 0) return q_push(obj, arg1);
        if (strcmp(method, "get") == 0) return q_get(obj, arg1);
        if (strcmp(method, "remove") == 0) return q_remove(obj, arg1);
        if (strcmp(method, "concat") == 0) return q_concat(obj, arg1);
        fprintf(stderr, "runtime error: list has no method '%s' taking 1 argument\n", method);
        return qv_null();
    }

    // String methods with 1 arg
    if (obj.type == QValue::VAL_STRING) {
        if (strcmp(method, "contains") == 0) return q_contains(obj, arg1);
        if (strcmp(method, "startswith") == 0) return q_startswith(obj, arg1);
        if (strcmp(method, "endswith") == 0) return q_endswith(obj, arg1);
        if (strcmp(method, "concat") == 0) return q_concat(obj, arg1);
        fprintf(stderr, "runtime error: string has no method '%s' taking 1 argument\n", method);
        return qv_null();
    }

    const char* type_names[] = {"int", "float", "string", "bool", "null", "list", "vector", "dict", "func", "result"};
    const char* type_name = (obj.type >= 0 && obj.type <= 9) ? type_names[obj.type] : "unknown";
    fprintf(stderr, "runtime error: type '%s' has no method '%s'\n", type_name, method);
    return qv_null();
}

// obj.method(arg1, arg2) → q_member_call2(obj, "method", arg1, arg2)
inline QValue q_member_call2(QValue obj, const char* method, QValue arg1, QValue arg2) {
    if (obj.type == QValue::VAL_NULL) {
        fprintf(stderr, "runtime error: cannot call method '%s' on null\n", method);
        return qv_null();
    }

    // List methods with 2 args
    if (obj.type == QValue::VAL_LIST) {
        if (strcmp(method, "set") == 0) return q_set(obj, arg1, arg2);
        if (strcmp(method, "insert") == 0) return q_insert(obj, arg1, arg2);
        if (strcmp(method, "slice") == 0) return q_slice(obj, arg1, arg2);
        fprintf(stderr, "runtime error: list has no method '%s' taking 2 arguments\n", method);
        return qv_null();
    }

    // String methods with 2 args
    if (obj.type == QValue::VAL_STRING) {
        if (strcmp(method, "replace") == 0) return q_replace(obj, arg1, arg2);
        fprintf(stderr, "runtime error: string has no method '%s' taking 2 arguments\n", method);
        return qv_null();
    }

    const char* type_names[] = {"int", "float", "string", "bool", "null", "list", "vector", "dict", "func", "result"};
    const char* type_name = (obj.type >= 0 && obj.type <= 9) ? type_names[obj.type] : "unknown";
    fprintf(stderr, "runtime error: type '%s' has no method '%s'\n", type_name, method);
    return qv_null();
}

// Member set: obj.member = value (for dict key assignment)
inline QValue q_member_set(QValue obj, const char* member, QValue value) {
    if (obj.type == QValue::VAL_DICT) {
        return q_dict_set(obj, qv_string(member), value);
    }
    fprintf(stderr, "runtime error: cannot set member '%s' on non-dict type\n", member);
    return qv_null();
}

#endif // QUARK_OPS_MEMBER_HPP
