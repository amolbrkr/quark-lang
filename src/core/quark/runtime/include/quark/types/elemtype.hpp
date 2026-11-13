// quark/types/elemtype.hpp - element types of lists and dicts
//
// A list's element type (a dict's value type) is fixed when it is created
// (architecture.md §12.9, rules B and C): the compiler gives every list it
// creates a QElemType describing the static element type, and builtins that
// create lists give theirs (range: int, split: str, ...). nullptr means any
// value. Every write goes through q_elem_store, which rejects a value of
// another type and widens an int written into a float container, so code
// reading an element can trust the type the compiler inferred for it. This
// catches the writes the compiler can't see the types of: an untyped
// function pushing into a typed list, a value of unknown type, native code
// going through qext::list_push or qext::dict_set.
//
// Nested containers carry their own element types, and a container value is
// written into a container of containers only when its element type is a
// subtype of the expected one (no conversion: a list[int] is not a
// list[float], since its ints would be read as floats).
#ifndef QUARK_TYPES_ELEMTYPE_HPP
#define QUARK_TYPES_ELEMTYPE_HPP

#include "../core/value.hpp"
#include "../core/constructors.hpp"
#include "struct.hpp"
#include "vector.hpp"
#include <cstdio>
#include <cstdlib>

// Defined in dict.hpp, after QDict.
inline const QElemType* q_dict_elem(const QDict* d);
// Defined in table.hpp, after QTable.
inline const QStructDef* q_table_def(const QTable* t);
inline const char* q_table_type_name(const QTable* t);

// q_elem_sub reports whether every value a container with element type have
// can hold is a value of type want, without conversion.
inline bool q_elem_sub(const QElemType* have, const QElemType* want) {
    if (want == nullptr || have == want) return true;
    if (have == nullptr || have->tag != want->tag) return false;
    switch (want->tag) {
        case QValue::VAL_LIST:
        case QValue::VAL_DICT:
            return q_elem_sub(have->elem, want->elem);
        case QValue::VAL_RESULT:
            return q_elem_sub(have->elem, want->elem) && q_elem_sub(have->err, want->err);
        case QValue::VAL_STRUCT:
            return have->sdef == want->sdef;
        case QValue::VAL_TABLE:
            // A bare `tbl` (sdef nullptr) holds tables of any row.
            return want->sdef == nullptr || have->sdef == want->sdef;
        case QValue::VAL_VECTOR:
            return want->vdtype < 0 || have->vdtype == want->vdtype;
        default:
            return true;
    }
}

// q_elem_is reports whether v is a value of type t, without conversion.
// null is a value of the struct and resource types only.
inline bool q_elem_is(const QElemType* t, const QValue& v) {
    if (t == nullptr) return true;
    if (v.type != t->tag) {
        return v.type == QValue::VAL_NULL && (t->tag == QValue::VAL_STRUCT || t->tag == QValue::VAL_RESOURCE);
    }
    switch (t->tag) {
        case QValue::VAL_LIST:
            return v.data.list_val && q_elem_sub(v.data.list_val->elem, t->elem);
        case QValue::VAL_DICT:
            return v.data.dict_val && q_elem_sub(q_dict_elem(v.data.dict_val), t->elem);
        case QValue::VAL_RESULT:
            return v.data.result_val &&
                   q_elem_is(v.data.result_val->is_ok ? t->elem : t->err, v.data.result_val->payload);
        case QValue::VAL_STRUCT:
            return v.data.struct_val && v.data.struct_val->def == t->sdef;
        case QValue::VAL_TABLE:
            // Containers share their elements, so an anonymous row isn't
            // relabeled here: a tbl[R] element is a table of row R.
            return v.data.table_val && (t->sdef == nullptr || q_table_def(v.data.table_val) == t->sdef);
        case QValue::VAL_VECTOR:
            return t->vdtype < 0 || (v.data.vector_val && static_cast<int>(v.data.vector_val->type) == t->vdtype);
        default:
            return true;
    }
}

// q_value_type_name writes the type of v as the compiler would show it
// (`list[int]` for a list whose element type is known, a struct's name).
inline const char* q_value_type_name(const QValue& v, char* buf, size_t n) {
    switch (v.type) {
        case QValue::VAL_LIST:
            if (v.data.list_val && v.data.list_val->elem) {
                std::snprintf(buf, n, "list[%s]", v.data.list_val->elem->name);
                return buf;
            }
            return "list[any]";
        case QValue::VAL_DICT:
            if (v.data.dict_val && q_dict_elem(v.data.dict_val)) {
                std::snprintf(buf, n, "dict[str, %s]", q_dict_elem(v.data.dict_val)->name);
                return buf;
            }
            return "dict[str, any]";
        case QValue::VAL_STRUCT:
            if (v.data.struct_val && v.data.struct_val->def) return v.data.struct_val->def->name;
            return "struct";
        case QValue::VAL_TABLE:
            return q_table_type_name(v.data.table_val);
        default:
            return q_type_name(v.type);
    }
}

// q_elem_error reports a write of v into a container with element type t.
// what is the operation ("push()", "set()", "an index assignment"); key is
// the dict key for dict writes, nullptr for lists.
[[noreturn]] inline void q_elem_error(const QElemType* t, const QValue& v, const char* what, bool isDict, const char* key) {
    char buf[160];
    const char* got = q_value_type_name(v, buf, sizeof buf);
    if (isDict) {
        if (key) {
            q_runtime_reportf("runtime error: can't store a %s under key '%s' of a dict of %s: a dict keeps the value type it was created with (create it as 'd: dict = dict {...}' to hold mixed values)\n", got, key, t->name);
        } else {
            q_runtime_reportf("runtime error: %s can't store a %s in a dict of %s: a dict keeps the value type it was created with (create it as 'd: dict = dict {...}' to hold mixed values)\n", what, got, t->name);
        }
    } else {
        q_runtime_reportf("runtime error: %s can't add a %s to a list of %s: a list keeps the element type it was created with (create it as 'xs: list = list [...]' to hold mixed values)\n", what, got, t->name);
    }
    std::exit(1);
}

// q_elem_store returns v as stored into a container with element type t: an
// int becomes a float in a float container, a value of any other type is a
// runtime error.
inline QValue q_elem_store(const QElemType* t, QValue v, const char* what, bool isDict = false, const char* key = nullptr) {
    if (t == nullptr || q_elem_is(t, v)) return v;
    if (t->tag == QValue::VAL_FLOAT && v.type == QValue::VAL_INT) {
        return qv_float(static_cast<double>(v.data.int_val));
    }
    q_elem_error(t, v, what, isDict, key);
}

#endif // QUARK_TYPES_ELEMTYPE_HPP
