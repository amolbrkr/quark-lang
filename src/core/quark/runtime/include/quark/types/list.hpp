// quark/types/list.hpp - List operations using std::vector
#ifndef QUARK_TYPES_LIST_HPP
#define QUARK_TYPES_LIST_HPP

#include "../core/value.hpp"
#include "../core/constructors.hpp"
#include "string.hpp"

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
    if (list.type == QValue::VAL_STRING) {
        return q_str_get(list, index);
    }
    if (list.type != QValue::VAL_LIST || !list.data.list_val) {
        return qv_null();
    }
    // Type guard: index must be INT
    if (index.type != QValue::VAL_INT) {
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
    // Type guard: index must be INT
    if (index.type != QValue::VAL_INT) {
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
    // Type guard: index must be INT
    if (index.type != QValue::VAL_INT) {
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
    // Type guard: index must be INT
    if (index.type != QValue::VAL_INT) {
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

// Unified concat - dispatches to string or list concat at runtime
inline QValue q_concat(QValue a, QValue b) {
    if (a.type == QValue::VAL_STRING && b.type == QValue::VAL_STRING) {
        return q_str_concat(a, b);
    }
    if (a.type == QValue::VAL_LIST && b.type == QValue::VAL_LIST) {
        return q_list_concat(a, b);
    }
    // Type mismatch - both arguments must be the same type
    const char* type_names[] = {"int", "float", "string", "bool", "null", "list", "vector", "dict", "func", "result"};
    const char* a_type = (a.type >= 0 && a.type <= 8) ? type_names[a.type] : "unknown";
    const char* b_type = (b.type >= 0 && b.type <= 8) ? type_names[b.type] : "unknown";
    fprintf(stderr, "runtime error: concat expects both arguments to be the same type (string+string or list+list), got %s and %s\n", a_type, b_type);
    return qv_null();
}

// Slice list [start:end), returns new list
inline QValue q_slice(QValue list, QValue start, QValue end) {
    if (list.type != QValue::VAL_LIST || !list.data.list_val) {
        return qv_null();
    }
    // Type guard: start and end must be INT
    if (start.type != QValue::VAL_INT || end.type != QValue::VAL_INT) {
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

// Range functions - generate lists of integers
// range(end) - generates [0, 1, 2, ..., end-1]
inline QValue q_range(QValue end) {
    long long e = 0;
    if (end.type == QValue::VAL_INT) {
        e = end.data.int_val;
    } else if (end.type == QValue::VAL_FLOAT) {
        e = static_cast<long long>(end.data.float_val);
    } else {
        return qv_list();
    }

    QValue result = qv_list();
    for (long long i = 0; i < e; i++) {
        result.data.list_val->push_back(qv_int(i));
    }
    return result;
}

// range(start, end) - generates [start, start+1, ..., end-1]
inline QValue q_range(QValue start, QValue end) {
    long long s = 0, e = 0;

    if (start.type == QValue::VAL_INT) {
        s = start.data.int_val;
    } else if (start.type == QValue::VAL_FLOAT) {
        s = static_cast<long long>(start.data.float_val);
    } else {
        return qv_list();
    }

    if (end.type == QValue::VAL_INT) {
        e = end.data.int_val;
    } else if (end.type == QValue::VAL_FLOAT) {
        e = static_cast<long long>(end.data.float_val);
    } else {
        return qv_list();
    }

    QValue result = qv_list();
    if (s < e) {
        for (long long i = s; i < e; i++) {
            result.data.list_val->push_back(qv_int(i));
        }
    } else {
        for (long long i = s; i > e; i--) {
            result.data.list_val->push_back(qv_int(i));
        }
    }
    return result;
}

// range(start, end, step) - generates [start, start+step, start+2*step, ...]
inline QValue q_range(QValue start, QValue end, QValue step) {
    long long s = 0, e = 0, st = 1;

    if (start.type == QValue::VAL_INT) {
        s = start.data.int_val;
    } else if (start.type == QValue::VAL_FLOAT) {
        s = static_cast<long long>(start.data.float_val);
    } else {
        return qv_list();
    }

    if (end.type == QValue::VAL_INT) {
        e = end.data.int_val;
    } else if (end.type == QValue::VAL_FLOAT) {
        e = static_cast<long long>(end.data.float_val);
    } else {
        return qv_list();
    }

    if (step.type == QValue::VAL_INT) {
        st = step.data.int_val;
    } else if (step.type == QValue::VAL_FLOAT) {
        st = static_cast<long long>(step.data.float_val);
    } else {
        return qv_list();
    }

    if (st == 0) {
        return qv_list(); // Avoid infinite loop
    }

    QValue result = qv_list();
    if (st > 0) {
        for (long long i = s; i < e; i += st) {
            result.data.list_val->push_back(qv_int(i));
        }
    } else {
        for (long long i = s; i > e; i += st) {
            result.data.list_val->push_back(qv_int(i));
        }
    }
    return result;
}

#endif // QUARK_TYPES_LIST_HPP
