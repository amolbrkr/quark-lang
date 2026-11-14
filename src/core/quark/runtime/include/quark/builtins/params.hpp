// quark/builtins/params.hpp - enforcement of parameter annotations at function
// entry. Generated functions call these for each annotated parameter of a
// QValue-convention function, so an annotation holds however the function was
// called (directly, through a function value, a pipe or a default argument).
// Arguments whose type was already proven at compile time pay one tag check.
#ifndef QUARK_BUILTINS_PARAMS_HPP
#define QUARK_BUILTINS_PARAMS_HPP

#include "../core/value.hpp"
#include "../types/table.hpp"
#include "conversion.hpp"
#include <cstdlib>
#include <cstring>

[[noreturn]] inline void q_param_type_error(QValue v, const char* expected, const char* fn, const char* param) {
    QValue got = q_type(v);
    q_runtime_reportf("runtime error: argument '%s' of '%s' expects %s, got %s\n",
                      param, fn, expected, got.data.string_val ? got.data.string_val : "unknown");
    std::exit(1);
}

inline long long q_param_int(QValue v, const char* fn, const char* param) {
    if (v.type != QValue::VAL_INT) q_param_type_error(v, "int", fn, param);
    return v.data.int_val;
}

// float parameters accept an int and widen it, as everywhere else.
inline double q_param_float(QValue v, const char* fn, const char* param) {
    if (v.type == QValue::VAL_FLOAT) return v.data.float_val;
    if (v.type == QValue::VAL_INT) return static_cast<double>(v.data.int_val);
    q_param_type_error(v, "float", fn, param);
}

inline bool q_param_bool(QValue v, const char* fn, const char* param) {
    if (v.type != QValue::VAL_BOOL) q_param_type_error(v, "bool", fn, param);
    return v.data.bool_val;
}

// q_param_check checks a non-scalar annotation. allowNull is set for
// resource only (structs use q_param_struct): `list`, `dict`, `vec` and `fn`
// mean a list, dict, vec or function, and a result is always ok(...) or
// err(...), as in the checker.
inline QValue q_param_check(QValue v, QValue::ValueType tag, bool allowNull, const char* expected, const char* fn, const char* param) {
    if (v.type == tag || (allowNull && v.type == QValue::VAL_NULL)) return v;
    q_param_type_error(v, expected, fn, param);
}

// q_param_struct checks a struct-typed parameter (null allowed, as for other
// reference types). Struct identity is the definition: there is one
// QStructDef per struct definition, so two structs named Point in different
// files are different types.
inline QValue q_param_struct(QValue v, const QStructDef* def, const char* fn, const char* param) {
    if (v.type == QValue::VAL_NULL) return v;
    if (v.type == QValue::VAL_STRUCT && v.data.struct_val && v.data.struct_val->def == def) {
        return v;
    }
    q_param_type_error(v, def->name, fn, param);
}

// q_param_table checks a tbl[R] parameter (def is R's definition, nullptr
// for a bare `tbl`): a table of row R, or one with an anonymous row of R's
// shape, which is relabeled R (q_table_conform). Tables don't accept null.
inline QValue q_param_table(QValue v, const QStructDef* def, const char* expected, const char* fn, const char* param) {
    QValue out;
    if (q_table_conform(v, def, &out)) return out;
    q_param_type_error(v, expected, fn, param);
}

// Struct field values. A struct literal checks each field value whose type the
// compiler couldn't prove, with the same rules as parameters (float widens an
// int, reference types accept null), so a field read can trust the stored tag.
[[noreturn]] inline void q_field_type_error(QValue v, const char* expected, const char* structName, const char* field) {
    QValue got = q_type(v);
    q_runtime_reportf("runtime error: field '%s' of %s expects %s, got %s\n",
                      field, structName, expected, got.data.string_val ? got.data.string_val : "unknown");
    std::exit(1);
}

inline QValue q_field_int(QValue v, const char* structName, const char* field) {
    if (v.type != QValue::VAL_INT) q_field_type_error(v, "int", structName, field);
    return v;
}

inline QValue q_field_float(QValue v, const char* structName, const char* field) {
    if (v.type == QValue::VAL_FLOAT) return v;
    if (v.type == QValue::VAL_INT) return qv_float(static_cast<double>(v.data.int_val));
    q_field_type_error(v, "float", structName, field);
}

inline QValue q_field_bool(QValue v, const char* structName, const char* field) {
    if (v.type != QValue::VAL_BOOL) q_field_type_error(v, "bool", structName, field);
    return v;
}

inline QValue q_field_check(QValue v, QValue::ValueType tag, bool allowNull, const char* expected, const char* structName, const char* field) {
    if (v.type == tag || (allowNull && v.type == QValue::VAL_NULL)) return v;
    q_field_type_error(v, expected, structName, field);
}

inline QValue q_field_struct(QValue v, const QStructDef* expected, const char* structName, const char* field) {
    if (v.type == QValue::VAL_NULL) return v;
    if (v.type == QValue::VAL_STRUCT && v.data.struct_val && v.data.struct_val->def == expected) {
        return v;
    }
    q_field_type_error(v, expected->name, structName, field);
}

inline QValue q_field_table(QValue v, const QStructDef* def, const char* expected, const char* structName, const char* field) {
    QValue out;
    if (q_table_conform(v, def, &out)) return out;
    q_field_type_error(v, expected, structName, field);
}

// Typed declarations (`x: int = value`). The value is checked when the
// compiler couldn't prove its type, with the same rules as parameters, so the
// variable can be stored and read as its declared type. Scalars return the
// raw value (float widens an int).
[[noreturn]] inline void q_var_type_error(QValue v, const char* expected, const char* var) {
    QValue got = q_type(v);
    q_runtime_reportf("runtime error: variable '%s' is declared %s but got %s\n",
                      var, expected, got.data.string_val ? got.data.string_val : "unknown");
    std::exit(1);
}

inline long long q_var_int(QValue v, const char* var) {
    if (v.type != QValue::VAL_INT) q_var_type_error(v, "int", var);
    return v.data.int_val;
}

inline double q_var_float(QValue v, const char* var) {
    if (v.type == QValue::VAL_FLOAT) return v.data.float_val;
    if (v.type == QValue::VAL_INT) return static_cast<double>(v.data.int_val);
    q_var_type_error(v, "float", var);
}

inline bool q_var_bool(QValue v, const char* var) {
    if (v.type != QValue::VAL_BOOL) q_var_type_error(v, "bool", var);
    return v.data.bool_val;
}

inline QValue q_var_check(QValue v, QValue::ValueType tag, bool allowNull, const char* expected, const char* var) {
    if (v.type == tag || (allowNull && v.type == QValue::VAL_NULL)) return v;
    q_var_type_error(v, expected, var);
}

inline QValue q_var_struct(QValue v, const QStructDef* def, const char* var) {
    if (v.type == QValue::VAL_NULL) return v;
    if (v.type == QValue::VAL_STRUCT && v.data.struct_val && v.data.struct_val->def == def) {
        return v;
    }
    q_var_type_error(v, def->name, var);
}

inline QValue q_var_table(QValue v, const QStructDef* def, const char* expected, const char* var) {
    QValue out;
    if (q_table_conform(v, def, &out)) return out;
    q_var_type_error(v, expected, var);
}

// Return values of functions with a return annotation (`fn f(c) int -> ...`)
// whose body's type the compiler couldn't prove: checked once on return,
// with the same rules as parameters (float widens an int; struct and
// resource annotations accept null, the others don't).
[[noreturn]] inline void q_ret_type_error(QValue v, const char* expected, const char* fn) {
    QValue got = q_type(v);
    q_runtime_reportf("runtime error: function '%s' declares return type %s but returned %s\n",
                      fn, expected, got.data.string_val ? got.data.string_val : "unknown");
    std::exit(1);
}

inline long long q_ret_int(QValue v, const char* fn) {
    if (v.type != QValue::VAL_INT) q_ret_type_error(v, "int", fn);
    return v.data.int_val;
}

inline double q_ret_float(QValue v, const char* fn) {
    if (v.type == QValue::VAL_FLOAT) return v.data.float_val;
    if (v.type == QValue::VAL_INT) return static_cast<double>(v.data.int_val);
    q_ret_type_error(v, "float", fn);
}

inline bool q_ret_bool(QValue v, const char* fn) {
    if (v.type != QValue::VAL_BOOL) q_ret_type_error(v, "bool", fn);
    return v.data.bool_val;
}

inline QValue q_ret_check(QValue v, QValue::ValueType tag, bool allowNull, const char* expected, const char* fn) {
    if (v.type == tag || (allowNull && v.type == QValue::VAL_NULL)) return v;
    q_ret_type_error(v, expected, fn);
}

inline QValue q_ret_struct(QValue v, const QStructDef* def, const char* fn) {
    if (v.type == QValue::VAL_NULL) return v;
    if (v.type == QValue::VAL_STRUCT && v.data.struct_val && v.data.struct_val->def == def) {
        return v;
    }
    q_ret_type_error(v, def->name, fn);
}

inline QValue q_ret_table(QValue v, const QStructDef* def, const char* expected, const char* fn) {
    QValue out;
    if (q_table_conform(v, def, &out)) return out;
    q_ret_type_error(v, expected, fn);
}

#endif // QUARK_BUILTINS_PARAMS_HPP
