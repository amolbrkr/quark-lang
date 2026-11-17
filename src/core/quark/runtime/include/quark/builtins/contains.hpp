// quark/builtins/contains.hpp - xs.contains(x) on lists and vecs
// (specs/tables_v0_3_spec.md D48, §7.1).
//
// Whether some element equals x under ==: an int and a float are equal when
// they are the same number (core/numcmp.hpp), values of other different
// types never are, and a missing vec entry and a NaN are never equal to
// anything. str.contains(sub), the substring test, is in types/string.hpp.
#ifndef QUARK_BUILTINS_CONTAINS_HPP
#define QUARK_BUILTINS_CONTAINS_HPP

#include "../core/value.hpp"
#include "../core/constructors.hpp"
#include "../types/vector.hpp"
#include "../ops/comparison.hpp"
#include <cstring>
#include <string_view>

// q_float_as_int reports whether f equals an int64 exactly (it is integral
// and in range; NaN and infinities are not), and gives that int. An int
// equals a float exactly when the float converts this way to the same int.
inline bool q_float_as_int(double f, long long* out) {
    if (!(f >= -9223372036854775808.0 && f < 9223372036854775808.0)) return false;
    const long long i = static_cast<long long>(f);
    if (static_cast<double>(i) != f) return false;
    *out = i;
    return true;
}

// q_int_as_float reports whether the int i equals a float exactly, and
// gives that float.
inline bool q_int_as_float(long long i, double* out) {
    const double d = static_cast<double>(i);
    long long back = 0;
    if (!q_float_as_int(d, &back) || back != i) return false;
    *out = d;
    return true;
}

inline QValue q_list_contains(QValue xs, QValue x) {
    if (xs.type != QValue::VAL_LIST || !xs.data.list_val) {
        q_runtime_reportf("runtime error: list.contains() expects a list, got %s\n", q_type_name(xs.type));
        q_exit(1);
    }
    for (const QValue& e : *xs.data.list_val) {
        if (quark::detail::values_equal(e, x, 0)) return qv_bool(true);
    }
    return qv_bool(false);
}

inline QValue q_vec_contains(QValue vv, QValue x) {
    if (vv.type != QValue::VAL_VECTOR || !vv.data.vector_val) {
        q_runtime_reportf("runtime error: vec.contains() expects a vec, got %s\n", q_type_name(vv.type));
        q_exit(1);
    }
    const QVector& v = *vv.data.vector_val;
    const size_t n = v.count;
    auto present = [&](size_t i) { return !q_vec_is_null_at(v, i); };
    switch (v.type) {
        case QVector::Type::I64: {
            long long want = 0;
            if (x.type == QValue::VAL_INT) want = x.data.int_val;
            else if (x.type != QValue::VAL_FLOAT || !q_float_as_int(x.data.float_val, &want)) return qv_bool(false);
            const int64_t* d = q_vec_i64_data(v);
            for (size_t i = 0; i < n; i++) {
                if (d[i] == want && present(i)) return qv_bool(true);
            }
            return qv_bool(false);
        }
        case QVector::Type::F64: {
            double want = 0;
            if (x.type == QValue::VAL_FLOAT) want = x.data.float_val;
            else if (x.type != QValue::VAL_INT || !q_int_as_float(x.data.int_val, &want)) return qv_bool(false);
            if (want != want) return qv_bool(false);
            const double* d = q_vec_f64_data(v);
            for (size_t i = 0; i < n; i++) {
                if (d[i] == want && present(i)) return qv_bool(true);
            }
            return qv_bool(false);
        }
        case QVector::Type::BOOL: {
            if (x.type != QValue::VAL_BOOL) return qv_bool(false);
            for (size_t i = 0; i < n; i++) {
                if (present(i) && q_vec_bool_at(v, i) == x.data.bool_val) return qv_bool(true);
            }
            return qv_bool(false);
        }
        case QVector::Type::STR: {
            if (x.type != QValue::VAL_STRING || !x.data.string_val) return qv_bool(false);
            const std::string_view want(x.data.string_val);
            for (size_t i = 0; i < n; i++) {
                if (present(i) && q_vec_str_at(v, i) == want) return qv_bool(true);
            }
            return qv_bool(false);
        }
        case QVector::Type::DATE: {
            if (x.type != QValue::VAL_DATE) return qv_bool(false);
            const int32_t* d = q_vec_date_data(v);
            for (size_t i = 0; i < n; i++) {
                if (d[i] == x.data.int_val && present(i)) return qv_bool(true);
            }
            return qv_bool(false);
        }
    }
    return qv_bool(false);
}

#endif // QUARK_BUILTINS_CONTAINS_HPP
