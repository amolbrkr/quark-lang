// quark/types/list.hpp - List operations using std::vector
#ifndef QUARK_TYPES_LIST_HPP
#define QUARK_TYPES_LIST_HPP

#include "../core/value.hpp"
#include "../core/constructors.hpp"

// Push item to end of list
inline QValue q_push(QValue list, QValue item) {
    if (list.type != QValue::VAL_LIST || !list.data.list_val) {
        return qv_null();
    }
    list.data.list_val->push_back(item);
    return list;
}

// Pop item from end of list
inline QValue q_pop(QValue list) {
    if (list.type != QValue::VAL_LIST || !list.data.list_val || list.data.list_val->empty()) {
        return qv_null();
    }
    QValue item = list.data.list_val->back();
    list.data.list_val->pop_back();
    return item;
}

// Get item at index (supports negative indexing)
inline QValue q_get(QValue list, QValue index) {
    if (list.type != QValue::VAL_LIST || !list.data.list_val) {
        return qv_null();
    }
    int idx = static_cast<int>(index.data.int_val);
    int len = static_cast<int>(list.data.list_val->size());
    if (idx < 0) idx = len + idx;
    if (idx < 0 || idx >= len) {
        return qv_null();
    }
    return (*list.data.list_val)[idx];
}

// Set item at index (supports negative indexing)
inline QValue q_set(QValue list, QValue index, QValue value) {
    if (list.type != QValue::VAL_LIST || !list.data.list_val) {
        return qv_null();
    }
    int idx = static_cast<int>(index.data.int_val);
    int len = static_cast<int>(list.data.list_val->size());
    if (idx < 0) idx = len + idx;
    if (idx < 0 || idx >= len) {
        return qv_null();
    }
    (*list.data.list_val)[idx] = value;
    return value;
}

// Get list size
inline int q_list_size(QValue list) {
    if (list.type != QValue::VAL_LIST || !list.data.list_val) {
        return 0;
    }
    return static_cast<int>(list.data.list_val->size());
}

// Check if list is empty
inline bool q_list_empty(QValue list) {
    if (list.type != QValue::VAL_LIST || !list.data.list_val) {
        return true;
    }
    return list.data.list_val->empty();
}

// Clear all items from list
inline QValue q_list_clear(QValue list) {
    if (list.type != QValue::VAL_LIST || !list.data.list_val) {
        return qv_null();
    }
    list.data.list_val->clear();
    return list;
}

// Insert item at index
inline QValue q_insert(QValue list, QValue index, QValue item) {
    if (list.type != QValue::VAL_LIST || !list.data.list_val) {
        return qv_null();
    }
    int idx = static_cast<int>(index.data.int_val);
    int len = static_cast<int>(list.data.list_val->size());
    if (idx < 0) idx = len + idx;
    if (idx < 0) idx = 0;
    if (idx > len) idx = len;
    list.data.list_val->insert(list.data.list_val->begin() + idx, item);
    return list;
}

// Remove item at index
inline QValue q_remove(QValue list, QValue index) {
    if (list.type != QValue::VAL_LIST || !list.data.list_val) {
        return qv_null();
    }
    int idx = static_cast<int>(index.data.int_val);
    int len = static_cast<int>(list.data.list_val->size());
    if (idx < 0) idx = len + idx;
    if (idx < 0 || idx >= len) {
        return qv_null();
    }
    QValue item = (*list.data.list_val)[idx];
    list.data.list_val->erase(list.data.list_val->begin() + idx);
    return item;
}

// Concatenate two lists, returns new list
inline QValue q_list_concat(QValue a, QValue b) {
    if (a.type != QValue::VAL_LIST || b.type != QValue::VAL_LIST) {
        return qv_null();
    }
    QValue result = qv_list();
    if (a.data.list_val) {
        for (const auto& item : *a.data.list_val) {
            result.data.list_val->push_back(item);
        }
    }
    if (b.data.list_val) {
        for (const auto& item : *b.data.list_val) {
            result.data.list_val->push_back(item);
        }
    }
    return result;
}

// Slice list [start:end), returns new list
inline QValue q_slice(QValue list, QValue start, QValue end) {
    if (list.type != QValue::VAL_LIST || !list.data.list_val) {
        return qv_null();
    }
    int len = static_cast<int>(list.data.list_val->size());
    int s = static_cast<int>(start.data.int_val);
    int e = static_cast<int>(end.data.int_val);

    // Handle negative indices
    if (s < 0) s = len + s;
    if (e < 0) e = len + e;

    // Clamp to bounds
    if (s < 0) s = 0;
    if (e > len) e = len;
    if (s >= e) return qv_list();

    QValue result = qv_list(e - s);
    for (int i = s; i < e; i++) {
        result.data.list_val->push_back((*list.data.list_val)[i]);
    }
    return result;
}

// Reverse list in place
inline QValue q_reverse(QValue list) {
    if (list.type != QValue::VAL_LIST || !list.data.list_val) {
        return qv_null();
    }
    std::reverse(list.data.list_val->begin(), list.data.list_val->end());
    return list;
}

// Free list memory (for manual cleanup if needed)
inline void q_list_free(QValue list) {
    if (list.type == QValue::VAL_LIST && list.data.list_val) {
        delete list.data.list_val;
    }
}

#endif // QUARK_TYPES_LIST_HPP
