// quark/builtins/dict.hpp - Dict-related builtins
#ifndef QUARK_BUILTINS_DICT_HPP
#define QUARK_BUILTINS_DICT_HPP

#include "../core/value.hpp"
#include "../builtins/conversion.hpp"
#include "../types/dict.hpp"

// dget(dict, key) -> any
// Key is converted to string via q_str when needed.
inline QValue q_dget(QValue dict, QValue key) {
    if (key.type != QValue::VAL_STRING) {
        key = q_str(key);
    }
    return q_dict_get(dict, key);
}

// dset(dict, key, value) -> dict
// Key is converted to string via q_str when needed.
inline QValue q_dset(QValue dict, QValue key, QValue value) {
    if (key.type != QValue::VAL_STRING) {
        key = q_str(key);
    }
    return q_dict_set(dict, key, value);
}

#endif // QUARK_BUILTINS_DICT_HPP
