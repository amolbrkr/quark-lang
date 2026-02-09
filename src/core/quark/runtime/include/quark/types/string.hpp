// quark/types/string.hpp - String operations
#ifndef QUARK_TYPES_STRING_HPP
#define QUARK_TYPES_STRING_HPP

#include "../core/value.hpp"
#include "../core/constructors.hpp"
#include "../core/gc.hpp"
#include <cstring>
#include <cctype>
#include <cstdlib>

// Convert string to uppercase
inline QValue q_upper(QValue v) {
    // Type guard: only STRING is valid
    if (v.type != QValue::VAL_STRING) return qv_null();
    char* result = q_strdup(v.data.string_val);
    for (int i = 0; result[i]; i++) {
        result[i] = static_cast<char>(toupper(static_cast<unsigned char>(result[i])));
    }
    QValue q = qv_string(result);
    // GC will handle cleanup - no explicit free needed
    return q;
}

// Convert string to lowercase
inline QValue q_lower(QValue v) {
    // Type guard: only STRING is valid
    if (v.type != QValue::VAL_STRING) return qv_null();
    char* result = q_strdup(v.data.string_val);
    for (int i = 0; result[i]; i++) {
        result[i] = static_cast<char>(tolower(static_cast<unsigned char>(result[i])));
    }
    QValue q = qv_string(result);
    // GC will handle cleanup - no explicit free needed
    return q;
}

// Trim whitespace from both ends
inline QValue q_trim(QValue v) {
    // Type guard: only STRING is valid
    if (v.type != QValue::VAL_STRING) return qv_null();
    const char* start = v.data.string_val;
    while (*start && isspace(static_cast<unsigned char>(*start))) start++;
    if (*start == '\0') return qv_string("");

    const char* end = v.data.string_val + strlen(v.data.string_val) - 1;
    while (end > start && isspace(static_cast<unsigned char>(*end))) end--;

    size_t len = static_cast<size_t>(end - start + 1);
    char* result = static_cast<char*>(q_malloc_atomic(len + 1));
    strncpy(result, start, len);
    result[len] = '\0';

    QValue q = qv_string(result);
    // GC will handle cleanup - no explicit free needed
    return q;
}

// Check if string contains substring
inline QValue q_contains(QValue str, QValue sub) {
    // Type guard: both must be STRING
    if (str.type != QValue::VAL_STRING || sub.type != QValue::VAL_STRING) {
        return qv_null();
    }
    return qv_bool(strstr(str.data.string_val, sub.data.string_val) != nullptr);
}

// Check if string starts with prefix
inline QValue q_startswith(QValue str, QValue prefix) {
    // Type guard: both must be STRING
    if (str.type != QValue::VAL_STRING || prefix.type != QValue::VAL_STRING) {
        return qv_null();
    }
    size_t plen = strlen(prefix.data.string_val);
    return qv_bool(strncmp(str.data.string_val, prefix.data.string_val, plen) == 0);
}

// Check if string ends with suffix
inline QValue q_endswith(QValue str, QValue suffix) {
    // Type guard: both must be STRING
    if (str.type != QValue::VAL_STRING || suffix.type != QValue::VAL_STRING) {
        return qv_null();
    }
    size_t slen = strlen(str.data.string_val);
    size_t suflen = strlen(suffix.data.string_val);
    if (suflen > slen) return qv_bool(false);
    return qv_bool(strcmp(str.data.string_val + slen - suflen, suffix.data.string_val) == 0);
}

// Replace all occurrences of old_str with new_str
inline QValue q_replace(QValue str, QValue old_str, QValue new_str) {
    // Type guard: all must be STRING
    if (str.type != QValue::VAL_STRING || old_str.type != QValue::VAL_STRING ||
        new_str.type != QValue::VAL_STRING) {
        return qv_null();
    }

    const char* s = str.data.string_val;
    const char* o = old_str.data.string_val;
    const char* n = new_str.data.string_val;
    size_t olen = strlen(o);
    size_t nlen = strlen(n);
    if (olen == 0) return str;

    // Count occurrences
    int count = 0;
    const char* tmp = s;
    while ((tmp = strstr(tmp, o)) != nullptr) {
        count++;
        tmp += olen;
    }

    // Allocate result (use atomic since it's just chars, no pointers)
    size_t slen = strlen(s);
    size_t rlen = slen + static_cast<size_t>(count) * (nlen - olen);
    char* result = static_cast<char*>(q_malloc_atomic(rlen + 1));
    char* dest = result;

    while (*s) {
        if (strncmp(s, o, olen) == 0) {
            strcpy(dest, n);
            dest += nlen;
            s += olen;
        } else {
            *dest++ = *s++;
        }
    }
    *dest = '\0';

    QValue q = qv_string(result);
    // GC will handle cleanup - no explicit free needed
    return q;
}

// Concatenate two strings
inline QValue q_concat(QValue a, QValue b) {
    // Type guard: both must be STRING
    if (a.type != QValue::VAL_STRING || b.type != QValue::VAL_STRING) {
        return qv_null();
    }
    size_t len = strlen(a.data.string_val) + strlen(b.data.string_val);
    char* result = static_cast<char*>(q_malloc_atomic(len + 1));
    strcpy(result, a.data.string_val);
    strcat(result, b.data.string_val);
    QValue q = qv_string(result);
    // GC will handle cleanup - no explicit free needed
    return q;
}

#endif // QUARK_TYPES_STRING_HPP
