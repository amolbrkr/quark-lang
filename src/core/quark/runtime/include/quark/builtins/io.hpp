// quark/builtins/io.hpp - I/O operations
#ifndef QUARK_BUILTINS_IO_HPP
#define QUARK_BUILTINS_IO_HPP

#include "../core/value.hpp"
#include "../core/constructors.hpp"
#include "../types/dict.hpp"
#include <cstdio>
#include <cstring>

// Print a QValue (without newline)
inline void print_qvalue(QValue v) {
    switch (v.type) {
        case QValue::VAL_INT:
            printf("%lld", v.data.int_val);
            break;
        case QValue::VAL_FLOAT:
            printf("%g", v.data.float_val);
            break;
        case QValue::VAL_STRING:
            printf("%s", v.data.string_val);
            break;
        case QValue::VAL_BOOL:
            printf(v.data.bool_val ? "true" : "false");
            break;
        case QValue::VAL_NULL:
            printf("null");
            break;
        case QValue::VAL_LIST:
            printf("[list len=%zu]", v.data.list_val ? v.data.list_val->size() : 0);
            break;
        case QValue::VAL_DICT:
            printf("[dict len=%zu]", v.data.dict_val ? v.data.dict_val->entries.size() : 0);
            break;
        case QValue::VAL_FUNC:
            printf("<function>");
            break;
        default:
            printf("<value>");
            break;
    }
}

// Print without newline
inline QValue q_print(QValue v) {
    print_qvalue(v);
    return qv_null();
}

// Print with newline
inline QValue q_println(QValue v) {
    print_qvalue(v);
    printf("\n");
    return qv_null();
}

// Read line from stdin (with optional prompt)
inline QValue q_input(QValue prompt) {
    // Print prompt if it's a string
    if (prompt.type == QValue::VAL_STRING) {
        printf("%s", prompt.data.string_val);
        fflush(stdout);
    }
    char buffer[4096];
    if (fgets(buffer, sizeof(buffer), stdin) != nullptr) {
        // Remove trailing newline
        size_t len = strlen(buffer);
        if (len > 0 && buffer[len - 1] == '\n') {
            buffer[len - 1] = '\0';
        }
        return qv_string(buffer);
    }
    return qv_string("");
}

// Read line from stdin (no prompt)
inline QValue q_input() {
    return q_input(qv_null());
}

#endif // QUARK_BUILTINS_IO_HPP
