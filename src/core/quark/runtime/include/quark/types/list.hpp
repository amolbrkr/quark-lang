// quark/types/list.hpp - List operations using std::vector
#ifndef QUARK_TYPES_LIST_HPP
#define QUARK_TYPES_LIST_HPP

#include "../core/value.hpp"
#include "../core/constructors.hpp"
#include "string.hpp"
#include "dict.hpp"

// Push item to end of list
inline QValue q_push(QValue list, QValue item) {
    if (list.type != QValue::VAL_LIST || !list.data.list_val) {
        q_runtime_reportf("runtime error: push() expects list as first argument\n");
        std::exit(1);
    }
    list.data.list_val->push_back(item);
    return list;
}

// Pop item from end of list
inline QValue q_pop(QValue list) {
    if (list.type != QValue::VAL_LIST || !list.data.list_val) {
        q_runtime_reportf("runtime error: pop() expects list argument\n");
        std::exit(1);
    }
    if (list.data.list_val->empty()) {
        q_runtime_reportf("runtime error: pop() on empty list\n");
        std::exit(1);
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
    // Vector indexing: scalar int or boolean mask
    if (list.type == QValue::VAL_VECTOR) {
        if (index.type == QValue::VAL_INT) {
            return q_vec_get_scalar(list, index);
        }
        if (index.type == QValue::VAL_VECTOR) {
            return q_vec_mask_filter(list, index);
        }
        q_runtime_reportf("runtime error: vec index must be int or bool vec\n");
        std::exit(1);
    }
    if (list.type != QValue::VAL_LIST || !list.data.list_val) {
        q_runtime_reportf("runtime error: get() expects list/string/vec as first argument\n");
        std::exit(1);
    }
    if (index.type != QValue::VAL_INT) {
        q_runtime_reportf("runtime error: get() index must be int\n");
        std::exit(1);
    }
    int idx = static_cast<int>(index.data.int_val);
    int len = static_cast<int>(list.data.list_val->size());
    if (idx < 0) idx = len + idx;
    // Policy: get() returns null on OOB (documented safe-read API).
    // Write operations (set/insert/remove) crash on invalid indices.
    if (idx < 0 || idx >= len) {
        return qv_null();
    }
    return (*list.data.list_val)[idx];
}

// Set item at index (supports negative indexing)
inline QValue q_set(QValue list, QValue index, QValue value) {
    if (list.type != QValue::VAL_LIST || !list.data.list_val) {
        q_runtime_reportf("runtime error: set() expects list as first argument\n");
        std::exit(1);
    }
    if (index.type != QValue::VAL_INT) {
        q_runtime_reportf("runtime error: set() index must be int\n");
        std::exit(1);
    }
    int idx = static_cast<int>(index.data.int_val);
    int len = static_cast<int>(list.data.list_val->size());
    if (idx < 0) idx = len + idx;
    if (idx < 0 || idx >= len) {
        q_runtime_reportf("runtime error: set() index %d out of range (list length %d)\n", idx, len);
        std::exit(1);
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
        q_runtime_reportf("runtime error: insert() expects list as first argument\n");
        std::exit(1);
    }
    if (index.type != QValue::VAL_INT) {
        q_runtime_reportf("runtime error: insert() index must be int\n");
        std::exit(1);
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
        q_runtime_reportf("runtime error: remove() expects list as first argument\n");
        std::exit(1);
    }
    if (index.type != QValue::VAL_INT) {
        q_runtime_reportf("runtime error: remove() index must be int\n");
        std::exit(1);
    }
    int idx = static_cast<int>(index.data.int_val);
    int len = static_cast<int>(list.data.list_val->size());
    if (idx < 0) idx = len + idx;
    if (idx < 0 || idx >= len) {
        q_runtime_reportf("runtime error: remove() index %d out of range (list length %d)\n", idx, len);
        std::exit(1);
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
        q_runtime_reportf("runtime error: slice() expects list as first argument\n");
        std::exit(1);
    }
    if (start.type != QValue::VAL_INT || end.type != QValue::VAL_INT) {
        q_runtime_reportf("runtime error: slice() start and end indices must be int\n");
        std::exit(1);
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
        q_runtime_reportf("runtime error: reverse() expects list argument\n");
        std::exit(1);
    }
    std::reverse(list.data.list_val->begin(), list.data.list_val->end());
    return list;
}

// xs.enumerate() / v.enumerate() -> list[dict{index: int, value: any}]
// Supports list, string, and vector sequences.
inline QValue q_enumerate(QValue seq) {
    int len = 0;
    if (seq.type == QValue::VAL_LIST && seq.data.list_val) {
        len = static_cast<int>(seq.data.list_val->size());
    } else if (seq.type == QValue::VAL_STRING && seq.data.string_val) {
        len = static_cast<int>(std::strlen(seq.data.string_val));
    } else if (seq.type == QValue::VAL_VECTOR) {
        len = q_vec_size(seq);
    } else {
        q_runtime_reportf("runtime error: enumerate() expects list, string, or vec\n");
        std::exit(1);
    }

    QValue out = qv_list(len);
    for (int i = 0; i < len; i++) {
        QValue item = qv_dict();
        item = q_dict_set(item, qv_string("index"), qv_int(i));
        item = q_dict_set(item, qv_string("value"), q_get(seq, qv_int(i)));
        out.data.list_val->push_back(item);
    }
    return out;
}

// Free list memory for non-GC builds.
// In GC mode, object lifetime is collector-owned; manual free is a no-op.
inline void q_list_free(QValue list) {
#ifndef QUARK_USE_GC
    if (list.type == QValue::VAL_LIST && list.data.list_val) {
        list.data.list_val->~QList();
        q_free(list.data.list_val);
    }
#else
    (void)list;
#endif
}

// q_range_arg converts one argument of a lowered `for i in range(...)` loop to an
// integer, with the same rules and messages as the q_range builtins: ints as is,
// floats truncated, anything else a runtime error. `sig` is the call form, e.g.
// "range(start, end)", and `what` the argument name, e.g. "end".
inline long long q_range_arg(QValue v, const char* sig, const char* what) {
    if (v.type == QValue::VAL_INT) {
        return v.data.int_val;
    }
    if (v.type == QValue::VAL_FLOAT) {
        return static_cast<long long>(v.data.float_val);
    }
    q_runtime_reportf("runtime error: %s expects numeric %s value\n", sig, what);
    std::exit(1);
}

// q_range_step validates the step of a lowered `for i in range(start, end, step)`.
inline long long q_range_step(long long step) {
    if (step == 0) {
        q_runtime_reportf("runtime error: range() step must not be zero\n");
        std::exit(1);
    }
    return step;
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
        q_runtime_reportf("runtime error: range(end) expects numeric end value\n");
        std::exit(1);
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
        q_runtime_reportf("runtime error: range(start, end) expects numeric start value\n");
        std::exit(1);
    }

    if (end.type == QValue::VAL_INT) {
        e = end.data.int_val;
    } else if (end.type == QValue::VAL_FLOAT) {
        e = static_cast<long long>(end.data.float_val);
    } else {
        q_runtime_reportf("runtime error: range(start, end) expects numeric end value\n");
        std::exit(1);
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
        q_runtime_reportf("runtime error: range(start, end, step) expects numeric start value\n");
        std::exit(1);
    }

    if (end.type == QValue::VAL_INT) {
        e = end.data.int_val;
    } else if (end.type == QValue::VAL_FLOAT) {
        e = static_cast<long long>(end.data.float_val);
    } else {
        q_runtime_reportf("runtime error: range(start, end, step) expects numeric end value\n");
        std::exit(1);
    }

    if (step.type == QValue::VAL_INT) {
        st = step.data.int_val;
    } else if (step.type == QValue::VAL_FLOAT) {
        st = static_cast<long long>(step.data.float_val);
    } else {
        q_runtime_reportf("runtime error: range(start, end, step) expects numeric step value\n");
        std::exit(1);
    }

    if (st == 0) {
        q_runtime_reportf("runtime error: range() step must not be zero\n");
        std::exit(1);
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
