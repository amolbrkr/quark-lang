// quark/builtins/strings.hpp - Additional string builtins
#ifndef QUARK_BUILTINS_STRINGS_HPP
#define QUARK_BUILTINS_STRINGS_HPP

#include "../core/value.hpp"
#include "../core/constructors.hpp"
#include "../core/gc.hpp"

#include <string>

// split(string, sep) -> list[str]
// - If sep is "", returns a single-element list containing the original string.
// - Preserves empty fields (leading/trailing separators produce "").
inline QValue q_split(QValue str, QValue sep) {
    if (str.type != QValue::VAL_STRING || sep.type != QValue::VAL_STRING) {
        return qv_null();
    }
    if (!str.data.string_val || !sep.data.string_val) {
        return qv_null();
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
