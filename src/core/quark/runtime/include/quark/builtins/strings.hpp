// quark/builtins/strings.hpp - Additional string builtins
#ifndef QUARK_BUILTINS_STRINGS_HPP
#define QUARK_BUILTINS_STRINGS_HPP

#include "../core/value.hpp"
#include "../core/constructors.hpp"
#include "../core/gc.hpp"
#include "../builtins/conversion.hpp"

#include <string>
#include <cstring>
#include <cstdio>
#include <cstdlib>

// sslice(str, start, end) -> str
// Returns the substring str[start:end).
// Supports negative indices (counted from the end).
// Clamps out-of-range indices rather than erroring.
inline QValue q_str_slice(QValue str, QValue start, QValue end) {
    if (str.type != QValue::VAL_STRING || !str.data.string_val) {
        q_runtime_reportf("runtime error: sslice() expects str as first argument\n");
        std::exit(1);
    }
    if (start.type != QValue::VAL_INT || end.type != QValue::VAL_INT) {
        q_runtime_reportf("runtime error: sslice() start and end must be int\n");
        std::exit(1);
    }
    int len = static_cast<int>(strlen(str.data.string_val));
    int s = static_cast<int>(start.data.int_val);
    int e = static_cast<int>(end.data.int_val);

    if (s < 0) s = len + s;
    if (e < 0) e = len + e;
    if (s < 0) s = 0;
    if (e > len) e = len;
    if (s >= e) return qv_string("");

    size_t rlen = static_cast<size_t>(e - s);
    char* result = static_cast<char*>(q_malloc_atomic(rlen + 1));
    if (!result) {
        q_runtime_reportf("runtime error: sslice() failed to allocate result\n");
        std::exit(1);
    }
    memcpy(result, str.data.string_val + s, rlen);
    result[rlen] = '\0';
    return qv_string_own(result);
}

// sjoin(list[str], sep) -> str
// Concatenates all elements of the list with sep between each pair.
// Non-string elements are coerced via q_str().
inline QValue q_str_join(QValue lst, QValue sep) {
    if (lst.type != QValue::VAL_LIST) {
        q_runtime_reportf("runtime error: sjoin() expects list as first argument\n");
        std::exit(1);
    }
    if (sep.type != QValue::VAL_STRING || !sep.data.string_val) {
        q_runtime_reportf("runtime error: sjoin() expects str as second argument\n");
        std::exit(1);
    }
    if (!lst.data.list_val || lst.data.list_val->empty()) {
        return qv_string("");
    }

    const char* dsep = sep.data.string_val;
    size_t seplen = strlen(dsep);
    size_t n = lst.data.list_val->size();

    // Compute total length, coercing each element to string
    size_t total = 0;
    for (size_t i = 0; i < n; i++) {
        QValue elem = (*lst.data.list_val)[i];
        if (elem.type != QValue::VAL_STRING) {
            elem = q_str(elem);
        }
        total += elem.data.string_val ? strlen(elem.data.string_val) : 0;
        if (i + 1 < n) total += seplen;
    }

    char* result = static_cast<char*>(q_malloc_atomic(total + 1));
    if (!result) {
        q_runtime_reportf("runtime error: sjoin() failed to allocate result\n");
        std::exit(1);
    }

    char* dest = result;
    for (size_t i = 0; i < n; i++) {
        QValue elem = (*lst.data.list_val)[i];
        if (elem.type != QValue::VAL_STRING) {
            elem = q_str(elem);
        }
        const char* s = elem.data.string_val ? elem.data.string_val : "";
        size_t slen = strlen(s);
        memcpy(dest, s, slen);
        dest += slen;
        if (i + 1 < n) {
            memcpy(dest, dsep, seplen);
            dest += seplen;
        }
    }
    *dest = '\0';
    return qv_string_own(result);
}

// split(string, sep) -> list[str]
// - If sep is "", returns a single-element list containing the original string.
// - Preserves empty fields (leading/trailing separators produce "").
inline QValue q_split(QValue str, QValue sep) {
    if (str.type != QValue::VAL_STRING || sep.type != QValue::VAL_STRING) {
        q_runtime_reportf("runtime error: split() expects (str, str)\n");
        std::exit(1);
    }
    if (!str.data.string_val || !sep.data.string_val) {
        q_runtime_reportf("runtime error: split() expects non-null string arguments\n");
        std::exit(1);
    }

    const char* s = str.data.string_val;
    const char* d = sep.data.string_val;
    if (d[0] == '\0') {
        QValue out = qv_list(1);
        out.data.list_val->push_back(str);
        return out;
    }

    std::string hay(s);
    std::string delim(d);

    QValue out = qv_list();
    size_t start = 0;
    while (true) {
        size_t pos = hay.find(delim, start);
        if (pos == std::string::npos) {
            std::string part = hay.substr(start);
            out.data.list_val->push_back(qv_string(part.c_str()));
            break;
        }
        std::string part = hay.substr(start, pos - start);
        out.data.list_val->push_back(qv_string(part.c_str()));
        start = pos + delim.size();
    }

    return out;
}

#endif // QUARK_BUILTINS_STRINGS_HPP
