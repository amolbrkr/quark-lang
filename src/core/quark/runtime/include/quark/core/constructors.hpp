// quark/core/constructors.hpp - QValue constructors
#ifndef QUARK_CORE_CONSTRUCTORS_HPP
#define QUARK_CORE_CONSTRUCTORS_HPP

#include "value.hpp"
#include "gc.hpp"
#include <cstring>
#include <cstdarg>

// Integer value constructor
inline QValue qv_int(long long v) {
    QValue q;
    q.type = QValue::VAL_INT;
    q.data.int_val = v;
    return q;
}

// Float value constructor
inline QValue qv_float(double v) {
    QValue q;
    q.type = QValue::VAL_FLOAT;
    q.data.float_val = v;
    return q;
}

// String value constructor (makes a copy using GC)
inline QValue qv_string(const char* v) {
    QValue q;
    q.type = QValue::VAL_STRING;
    q.data.string_val = q_strdup(v);
    return q;
}

// Boolean value constructor
inline QValue qv_bool(bool v) {
    QValue q;
    q.type = QValue::VAL_BOOL;
    q.data.bool_val = v;
    return q;
}

// Null value constructor
inline QValue qv_null() {
    QValue q;
    q.type = QValue::VAL_NULL;
    return q;
}

// Function value constructor - wraps raw pointer in a QClosure with 0 captures
inline QValue qv_func(void* f) {
    QClosure* cl = q_alloc_closure(f, 0);
    QValue q;
    q.type = QValue::VAL_FUNC;
    q.data.func_val = cl;
    return q;
}

inline QValue qv_ok(QValue v) {
    QValue q;
    q.type = QValue::VAL_RESULT;
    QResult* result = static_cast<QResult*>(q_malloc(sizeof(QResult)));
    result->is_ok = true;
    result->payload = v;
    q.data.result_val = result;
    return q;
}

inline QValue qv_err(QValue v) {
    QValue q;
    q.type = QValue::VAL_RESULT;
    QResult* result = static_cast<QResult*>(q_malloc(sizeof(QResult)));
    result->is_ok = false;
    result->payload = v;
    q.data.result_val = result;
    return q;
}

inline bool q_is_ok(const QValue& v) {
    return v.type == QValue::VAL_RESULT && v.data.result_val->is_ok;
}

inline QValue q_result_value(const QValue& v) {
    if (v.type == QValue::VAL_RESULT && v.data.result_val->is_ok) {
        return v.data.result_val->payload;
    }
    return qv_null();
}

inline QValue q_result_error(const QValue& v) {
    if (v.type == QValue::VAL_RESULT && !v.data.result_val->is_ok) {
        return v.data.result_val->payload;
    }
    return qv_null();
}

// List value constructor with optional initial capacity
// Note: std::vector internally uses new/delete, which Boehm GC intercepts
inline QValue qv_list(int initial_cap = 0) {
    QValue q;
    q.type = QValue::VAL_LIST;
    q.data.list_val = new QList();  // Boehm GC intercepts operator new
    if (initial_cap > 0) {
        q.data.list_val->reserve(initial_cap);
    }
    return q;
}

// List value constructor from variadic arguments
inline QValue qv_list_from(int count, ...) {
    QValue q;
    q.type = QValue::VAL_LIST;
    q.data.list_val = new QList();  // Boehm GC intercepts operator new
    q.data.list_val->reserve(count);
    va_list args;
    va_start(args, count);
    for (int i = 0; i < count; i++) {
        q.data.list_val->push_back(va_arg(args, QValue));
    }
    va_end(args);
    return q;
}

// List value constructor from initializer list (C++ style)
inline QValue qv_list_init(std::initializer_list<QValue> items) {
    QValue q;
    q.type = QValue::VAL_LIST;
    q.data.list_val = new QList(items);  // Boehm GC intercepts operator new
    return q;
}

#endif // QUARK_CORE_CONSTRUCTORS_HPP
