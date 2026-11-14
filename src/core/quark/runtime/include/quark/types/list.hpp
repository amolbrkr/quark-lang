// quark/types/list.hpp - List operations using std::vector
#ifndef QUARK_TYPES_LIST_HPP
#define QUARK_TYPES_LIST_HPP

#include "../core/value.hpp"
#include "../core/constructors.hpp"
#include "string.hpp"
#include "dict.hpp"
#include "elemtype.hpp"
#include "../core/checked.hpp"

// Push item to end of list
inline QValue q_push(QValue list, QValue item) {
    if (list.type != QValue::VAL_LIST || !list.data.list_val) {
        q_runtime_reportf("runtime error: push() expects list as first argument\n");
        std::exit(1);
    }
    list.data.list_val->push_back(q_elem_store(list.data.list_val->elem, item, "push()"));
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
    const long long len = static_cast<long long>(list.data.list_val->size());
    const long long idx = q_resolve_index(index.data.int_val, len);
    // Policy: get() returns null on OOB (documented safe-read API).
    // Write operations (set/insert/remove) crash on invalid indices.
    if (idx < 0 || idx >= len) {
        return qv_null();
    }
    return (*list.data.list_val)[static_cast<size_t>(idx)];
}

// q_index is `target[index]`, the typed read (semantics.md §9): an index out
// of range is a runtime error for lists and strings, as for vecs, and so is a
// non-int index. `xs.get(i)` (q_get) is the probing read that returns null.
inline QValue q_index(QValue target, QValue index) {
    if (target.type == QValue::VAL_VECTOR) {
        return q_get(target, index);  // an int index or a bool mask; checked there
    }
    const bool is_list = target.type == QValue::VAL_LIST && target.data.list_val != nullptr;
    const bool is_str = target.type == QValue::VAL_STRING && target.data.string_val != nullptr;
    if (!is_list && !is_str) {
        q_runtime_reportf("runtime error: cannot index a %s; only list, str and vec can be indexed\n", q_type_name(target.type));
        std::exit(1);
    }
    const char* kind = is_list ? "list" : "str";
    if (index.type != QValue::VAL_INT) {
        q_runtime_reportf("runtime error: %s index must be int, got %s\n", kind, q_type_name(index.type));
        std::exit(1);
    }
    const long long len = is_list ? static_cast<long long>(target.data.list_val->size())
                                  : static_cast<long long>(strlen(target.data.string_val));
    const long long given = index.data.int_val;
    const long long idx = given < 0 ? len + given : given;
    if (idx < 0 || idx >= len) {
        q_runtime_reportf("runtime error: %s index %lld out of range (%s length %lld)%s\n", kind, given, kind, len,
                          is_list ? "; use .get(i) when the index may be past the end" : "");
        std::exit(1);
    }
    if (is_list) {
        return (*target.data.list_val)[static_cast<size_t>(idx)];
    }
    char buf[2] = {target.data.string_val[idx], '\0'};
    return qv_string(buf);
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
    const long long len = static_cast<long long>(list.data.list_val->size());
    const long long idx = q_resolve_index(index.data.int_val, len);
    if (idx < 0 || idx >= len) {
        q_runtime_reportf("runtime error: set() index %lld out of range (list length %lld)\n", index.data.int_val, len);
        std::exit(1);
    }
    value = q_elem_store(list.data.list_val->elem, value, "set()");
    (*list.data.list_val)[static_cast<size_t>(idx)] = value;
    return value;
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
    const long long len = static_cast<long long>(list.data.list_val->size());
    long long idx = q_resolve_index(index.data.int_val, len);
    if (idx < 0) idx = 0;
    if (idx > len) idx = len;
    item = q_elem_store(list.data.list_val->elem, item, "insert()");
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
    const long long len = static_cast<long long>(list.data.list_val->size());
    const long long idx = q_resolve_index(index.data.int_val, len);
    if (idx < 0 || idx >= len) {
        q_runtime_reportf("runtime error: remove() index %lld out of range (list length %lld)\n", index.data.int_val, len);
        std::exit(1);
    }
    QValue item = (*list.data.list_val)[static_cast<size_t>(idx)];
    list.data.list_val->erase(list.data.list_val->begin() + idx);
    return item;
}

// Concatenate two lists into a new list with a's element type; b's elements
// are checked against it unless b's element type already is one.
inline QValue q_list_concat(QValue a, QValue b) {
    if (a.type != QValue::VAL_LIST || !a.data.list_val || b.type != QValue::VAL_LIST || !b.data.list_val) {
        q_runtime_reportf("runtime error: concat() expects two lists, got %s and %s\n", q_type_name(a.type), q_type_name(b.type));
        std::exit(1);
    }
    const QElemType* elem = a.data.list_val->elem;
    QValue result = qv_list(static_cast<int>(a.data.list_val->size() + b.data.list_val->size()), elem);
    for (const auto& item : *a.data.list_val) {
        result.data.list_val->push_back(item);
    }
    bool checked = !q_elem_sub(b.data.list_val->elem, elem);
    for (const auto& item : *b.data.list_val) {
        result.data.list_val->push_back(checked ? q_elem_store(elem, item, "concat()") : item);
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
    const long long len = static_cast<long long>(list.data.list_val->size());
    long long s = q_resolve_index(start.data.int_val, len);
    long long e = q_resolve_index(end.data.int_val, len);

    // Clamp to bounds
    if (s < 0) s = 0;
    if (e > len) e = len;
    if (s >= e) return qv_list(0, list.data.list_val->elem);

    QValue result = qv_list(static_cast<int>(e - s), list.data.list_val->elem);
    for (long long i = s; i < e; i++) {
        result.data.list_val->push_back((*list.data.list_val)[static_cast<size_t>(i)]);
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


// q_range_arg converts one argument of range() to an integer: `sig` is the
// call form, e.g. "range(start, end)", and `what` the argument name, e.g.
// "end". Arguments are ints; a float is an error rather than truncated, since
// range(0.5, 3.0) would otherwise silently start at 0.
inline long long q_range_arg(QValue v, const char* sig, const char* what) {
    if (v.type == QValue::VAL_INT) {
        return v.data.int_val;
    }
    if (v.type == QValue::VAL_FLOAT) {
        q_runtime_reportf("runtime error: %s takes ints, got a float for %s; convert it with floor(), ceil(), round() or to_int()\n", sig, what);
        std::exit(1);
    }
    q_runtime_reportf("runtime error: %s takes ints, got %s for %s\n", sig, q_type_name(v.type), what);
    std::exit(1);
}

// q_range_step validates the step of range(start, end, step).
inline long long q_range_step(long long step) {
    if (step == 0) {
        q_runtime_reportf("runtime error: range() step must not be zero\n");
        std::exit(1);
    }
    return step;
}

// q_range_count is how many values range(start, end, step) has (step != 0),
// computed in unsigned arithmetic so that ranges near the int64 limits
// neither overflow nor run forever.
inline unsigned long long q_range_count(long long start, long long end, long long step) {
    if (step > 0) {
        if (start >= end) return 0;
        const unsigned long long span = static_cast<unsigned long long>(end) - static_cast<unsigned long long>(start);
        return (span - 1) / static_cast<unsigned long long>(step) + 1;
    }
    if (start <= end) return 0;
    const unsigned long long span = static_cast<unsigned long long>(start) - static_cast<unsigned long long>(end);
    const unsigned long long magnitude = 0ULL - static_cast<unsigned long long>(step);
    return (span - 1) / magnitude + 1;
}

// q_range_at is value k of range(start, _, step), start + k * step. For
// k < q_range_count it lies between start and end, so it fits in an int64;
// the unsigned arithmetic only avoids a signed-overflow intermediate.
inline long long q_range_at(long long start, long long step, unsigned long long k) {
    return static_cast<long long>(static_cast<unsigned long long>(start) + k * static_cast<unsigned long long>(step));
}

// q_range_list builds the list of range(start, end, step).
inline QValue q_range_list(long long start, long long end, long long step) {
    const unsigned long long n = q_range_count(start, end, step);
    QValue result = qv_list(n < 1024 ? static_cast<int>(n) : 1024, &q_elem_int);
    for (unsigned long long k = 0; k < n; k++) {
        result.data.list_val->push_back(qv_int(q_range_at(start, step, k)));
    }
    return result;
}

// range(end) - [0, 1, ..., end-1]
inline QValue q_range(QValue end) {
    return q_range_list(0, q_range_arg(end, "range(end)", "end"), 1);
}

// range(start, end) - [start, start+1, ..., end-1], counting down to end+1
// when start > end.
inline QValue q_range(QValue start, QValue end) {
    const long long s = q_range_arg(start, "range(start, end)", "start");
    const long long e = q_range_arg(end, "range(start, end)", "end");
    return q_range_list(s, e, s <= e ? 1 : -1);
}

// range(start, end, step) - [start, start+step, ...] up to, not including, end.
inline QValue q_range(QValue start, QValue end, QValue step) {
    const char* sig = "range(start, end, step)";
    const long long s = q_range_arg(start, sig, "start");
    const long long e = q_range_arg(end, sig, "end");
    const long long st = q_range_step(q_range_arg(step, sig, "step"));
    return q_range_list(s, e, st);
}

#endif // QUARK_TYPES_LIST_HPP
