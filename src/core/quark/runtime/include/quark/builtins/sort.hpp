// quark/builtins/sort.hpp - list sorting (list.sort, list.sort_by)
#ifndef QUARK_BUILTINS_SORT_HPP
#define QUARK_BUILTINS_SORT_HPP

#include "../core/value.hpp"
#include "../builtins/conversion.hpp"
#include "../types/function.hpp"
#include <algorithm>
#include <cstring>
#include <vector>

// q_sort_less orders two sort keys: numbers (int and float together) by value,
// with NaN after every number (as table sorts put it) so that the order is a
// strict weak ordering, strings by byte order, bools false before true. Any
// other pair, such as a str and an int or anything with null, is a runtime
// error.
inline bool q_sort_less(const QValue& a, const QValue& b, const char* method) {
    const bool aNum = a.type == QValue::VAL_INT || a.type == QValue::VAL_FLOAT;
    const bool bNum = b.type == QValue::VAL_INT || b.type == QValue::VAL_FLOAT;
    if (aNum && bNum) {
        if (a.type == QValue::VAL_INT && b.type == QValue::VAL_INT) {
            return a.data.int_val < b.data.int_val;
        }
        const double x = a.type == QValue::VAL_INT ? static_cast<double>(a.data.int_val) : a.data.float_val;
        const double y = b.type == QValue::VAL_INT ? static_cast<double>(b.data.int_val) : b.data.float_val;
        const bool xNaN = x != x, yNaN = y != y;
        if (xNaN || yNaN) {
            return !xNaN && yNaN;  // a number before NaN; two NaNs are equal
        }
        return x < y;
    }
    if (a.type == QValue::VAL_STRING && b.type == QValue::VAL_STRING) {
        return std::strcmp(a.data.string_val ? a.data.string_val : "", b.data.string_val ? b.data.string_val : "") < 0;
    }
    if (a.type == QValue::VAL_BOOL && b.type == QValue::VAL_BOOL) {
        return !a.data.bool_val && b.data.bool_val;
    }
    QValue an = q_type(a);
    QValue bn = q_type(b);
    q_runtime_reportf("runtime error: %s() can't compare %s and %s; sort keys must all be numbers, all strings or all bools\n",
                      method, an.data.string_val, bn.data.string_val);
    std::exit(1);
}

// list.sort(): sorts the list in place (stable, ascending) and returns it.
inline QValue q_sort(QValue list) {
    if (list.type != QValue::VAL_LIST || !list.data.list_val) {
        q_runtime_reportf("runtime error: sort() expects list argument\n");
        std::exit(1);
    }
    QList& items = *list.data.list_val;
    std::stable_sort(items.begin(), items.end(), [](const QValue& a, const QValue& b) {
        return q_sort_less(a, b, "sort");
    });
    return list;
}

// list.sort_by(key): sorts the list in place (stable, ascending) by key(item),
// calling key once per item, and returns it.
inline QValue q_sort_by(QValue list, QValue key) {
    if (list.type != QValue::VAL_LIST || !list.data.list_val) {
        q_runtime_reportf("runtime error: sort_by() expects list argument\n");
        std::exit(1);
    }
    if (key.type != QValue::VAL_FUNC) {
        q_runtime_reportf("runtime error: sort_by() expects a function as its argument\n");
        std::exit(1);
    }
    QList& items = *list.data.list_val;
    const size_t n = items.size();
    // Keys and items live in GC-scanned memory so the GC sees them while
    // sorting (a std::vector's buffer is not scanned).
    QValue* keys = static_cast<QValue*>(q_malloc(sizeof(QValue) * (n > 0 ? n : 1)));
    QValue* copy = static_cast<QValue*>(q_malloc(sizeof(QValue) * (n > 0 ? n : 1)));
    std::vector<size_t> order(n);
    for (size_t i = 0; i < n; i++) {
        keys[i] = q_call1(key, items[i]);
        copy[i] = items[i];
        order[i] = i;
    }
    std::stable_sort(order.begin(), order.end(), [keys](size_t a, size_t b) {
        return q_sort_less(keys[a], keys[b], "sort_by");
    });
    for (size_t i = 0; i < n; i++) {
        items[i] = copy[order[i]];
    }
    return list;
}

#endif // QUARK_BUILTINS_SORT_HPP
