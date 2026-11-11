// quark/builtins/params.hpp - enforcement of parameter annotations at function
// entry. Generated functions call these for each annotated parameter of a
// QValue-convention function, so an annotation holds however the function was
// called (directly, through a function value, a pipe or a default argument).
// Arguments whose type was already proven at compile time pay one tag check.
#ifndef QUARK_BUILTINS_PARAMS_HPP
#define QUARK_BUILTINS_PARAMS_HPP

#include "../core/value.hpp"
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

// q_param_check checks a non-scalar annotation. Reference types (list, dict,
// vector, result, resource, fn) also accept null, as the checker does.
inline QValue q_param_check(QValue v, QValue::ValueType tag, bool allowNull, const char* expected, const char* fn, const char* param) {
    if (v.type == tag || (allowNull && v.type == QValue::VAL_NULL)) return v;
    q_param_type_error(v, expected, fn, param);
}

// q_param_struct checks a struct-typed parameter (null allowed, as for other
// reference types).
inline QValue q_param_struct(QValue v, const char* structName, const char* fn, const char* param) {
    if (v.type == QValue::VAL_NULL) return v;
    if (v.type == QValue::VAL_STRUCT && v.data.struct_val && v.data.struct_val->def &&
        std::strcmp(v.data.struct_val->def->name, structName) == 0) {
        return v;
    }
    q_param_type_error(v, structName, fn, param);
}

#endif // QUARK_BUILTINS_PARAMS_HPP
