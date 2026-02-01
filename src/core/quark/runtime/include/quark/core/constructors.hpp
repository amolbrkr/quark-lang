// quark/core/constructors.hpp - QValue constructors
#ifndef QUARK_CORE_CONSTRUCTORS_HPP
#define QUARK_CORE_CONSTRUCTORS_HPP

#include "value.hpp"
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

// String value constructor (makes a copy)
inline QValue qv_string(const char* v) {
    QValue q;
    q.type = QValue::VAL_STRING;
    q.data.string_val = strdup(v);
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

// Function value constructor
inline QValue qv_func(void* f) {
    QValue q;
    q.type = QValue::VAL_FUNC;
    q.data.func_val = f;
    return q;
}

// List value constructor with initial capacity
inline QValue qv_list(int initial_cap) {
    QValue q;
    q.type = QValue::VAL_LIST;
    q.data.list_val.cap = initial_cap > 0 ? initial_cap : 8;
    q.data.list_val.len = 0;
    q.data.list_val.items = static_cast<void**>(malloc(sizeof(QValue) * q.data.list_val.cap));
    return q;
}

// List value constructor from variadic arguments
inline QValue qv_list_from(int count, ...) {
    QValue q = qv_list(count > 0 ? count : 8);
    va_list args;
    va_start(args, count);
    for (int i = 0; i < count; i++) {
        QValue* items = reinterpret_cast<QValue*>(q.data.list_val.items);
        items[i] = va_arg(args, QValue);
    }
    q.data.list_val.len = count;
    va_end(args);
    return q;
}

#endif // QUARK_CORE_CONSTRUCTORS_HPP
