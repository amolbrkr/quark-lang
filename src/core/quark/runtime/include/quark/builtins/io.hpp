// quark/builtins/io.hpp - I/O operations
#ifndef QUARK_BUILTINS_IO_HPP
#define QUARK_BUILTINS_IO_HPP

#include "../core/value.hpp"
#include "../core/constructors.hpp"
#include "../builtins/conversion.hpp"
#include "../types/dict.hpp"
#include "../types/resource.hpp"
#include <cstdio>
#include <cstring>
#include <string>

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
            printf("%s", v.data.string_val ? v.data.string_val : "null");
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
        case QValue::VAL_VECTOR:
            printf("[vector len=%d]", q_vec_size(v));
            break;
        case QValue::VAL_DICT:
            printf("[dict len=%zu]", v.data.dict_val ? v.data.dict_val->entries.size() : 0);
            break;
        case QValue::VAL_FUNC:
            printf("<function>");
            break;
        case QValue::VAL_RESOURCE:
            printf("<%s>", q_resource_kind_name(v.data.resource_val ? v.data.resource_val->kind : QRES_KIND_NONE));
            break;
        default:
            printf("<value>");
            break;
    }
}

// Print without newline
inline QValue q_print(QValue v) {
    print_qvalue(v);
    printf("\n");
    return qv_null();
}

inline QValue q_print_impl(QValue v, QValue end, QValue width, QValue align, QValue pad) {
    if (end.type != QValue::VAL_STRING || !end.data.string_val) {
        q_runtime_reportf("runtime error: print() end must be str\n");
        std::exit(1);
    }
    if (width.type != QValue::VAL_INT) {
        q_runtime_reportf("runtime error: print() width must be int\n");
        std::exit(1);
    }
    if (align.type != QValue::VAL_STRING || !align.data.string_val) {
        q_runtime_reportf("runtime error: print() align must be str\n");
        std::exit(1);
    }
    if (pad.type != QValue::VAL_STRING || !pad.data.string_val) {
        q_runtime_reportf("runtime error: print() pad must be str\n");
        std::exit(1);
    }

    QValue sv = q_str(v);
    if (sv.type != QValue::VAL_STRING || !sv.data.string_val) {
        q_runtime_reportf("runtime error: print() failed to stringify value\n");
        std::exit(1);
    }

    std::string text = sv.data.string_val;
    long long w = width.data.int_val;
    char padCh = pad.data.string_val[0] ? pad.data.string_val[0] : ' ';

    if (w > 0 && static_cast<size_t>(w) > text.size()) {
        size_t padCount = static_cast<size_t>(w) - text.size();
        std::string left;
        std::string right;

        if (std::strcmp(align.data.string_val, "right") == 0) {
            left.assign(padCount, padCh);
        } else if (std::strcmp(align.data.string_val, "center") == 0) {
            size_t leftCount = padCount / 2;
            size_t rightCount = padCount - leftCount;
            left.assign(leftCount, padCh);
            right.assign(rightCount, padCh);
        } else {
            right.assign(padCount, padCh);
        }

        text = left + text + right;
    }

    std::printf("%s%s", text.c_str(), end.data.string_val);
    return qv_null();
}

inline QValue q_print(QValue v, QValue end) {
    return q_print_impl(v, end, qv_int(0), qv_string("left"), qv_string(" "));
}

inline QValue q_print(QValue v, QValue end, QValue width) {
    return q_print_impl(v, end, width, qv_string("left"), qv_string(" "));
}

inline QValue q_print(QValue v, QValue end, QValue width, QValue align) {
    return q_print_impl(v, end, width, align, qv_string(" "));
}

inline QValue q_print(QValue v, QValue end, QValue width, QValue align, QValue pad) {
    return q_print_impl(v, end, width, align, pad);
}

// Print with newline
inline QValue q_println(QValue v) {
    return q_print(v, qv_string("\n"));
}

// Read line from stdin (with optional prompt)
inline QValue q_input(QValue prompt) {
    if (prompt.type != QValue::VAL_NULL && prompt.type != QValue::VAL_STRING) {
        q_runtime_reportf("runtime error: input() optional prompt must be str\n");
        std::exit(1);
    }
    // Print prompt if it's a string
    if (prompt.type == QValue::VAL_STRING && prompt.data.string_val) {
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
