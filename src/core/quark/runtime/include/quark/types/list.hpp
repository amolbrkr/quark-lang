// quark/types/list.hpp - List operations
#ifndef QUARK_TYPES_LIST_HPP
#define QUARK_TYPES_LIST_HPP

#include "../core/value.hpp"
#include "../core/constructors.hpp"
#include <cstdlib>

// Grow list capacity (doubles it)
inline void q_list_grow(QValue* list) {
    if (list->type != QValue::VAL_LIST) return;
    int new_cap = list->data.list_val.cap * 2;
    list->data.list_val.items = static_cast<void**>(
        realloc(list->data.list_val.items, sizeof(QValue) * new_cap)
    );
    list->data.list_val.cap = new_cap;
}

// Push item to end of list
inline QValue q_push(QValue list, QValue item) {
    if (list.type != QValue::VAL_LIST) return qv_null();
    if (list.data.list_val.len >= list.data.list_val.cap) {
        q_list_grow(&list);
    }
    QValue* items = reinterpret_cast<QValue*>(list.data.list_val.items);
    items[list.data.list_val.len] = item;
    list.data.list_val.len++;
    return list;
}

// Pop item from end of list
inline QValue q_pop(QValue list) {
    if (list.type != QValue::VAL_LIST || list.data.list_val.len == 0) {
        return qv_null();
    }
    QValue* items = reinterpret_cast<QValue*>(list.data.list_val.items);
    list.data.list_val.len--;
    return items[list.data.list_val.len];
}

// Get item at index (supports negative indexing)
inline QValue q_get(QValue list, QValue index) {
    if (list.type != QValue::VAL_LIST) return qv_null();
    int idx = static_cast<int>(index.data.int_val);
    if (idx < 0) idx = list.data.list_val.len + idx;
    if (idx < 0 || idx >= list.data.list_val.len) return qv_null();
    QValue* items = reinterpret_cast<QValue*>(list.data.list_val.items);
    return items[idx];
}

// Set item at index (supports negative indexing)
inline QValue q_set(QValue list, QValue index, QValue value) {
    if (list.type != QValue::VAL_LIST) return qv_null();
    int idx = static_cast<int>(index.data.int_val);
    if (idx < 0) idx = list.data.list_val.len + idx;
    if (idx < 0 || idx >= list.data.list_val.len) return qv_null();
    QValue* items = reinterpret_cast<QValue*>(list.data.list_val.items);
    items[idx] = value;
    return value;
}

#endif // QUARK_TYPES_LIST_HPP
