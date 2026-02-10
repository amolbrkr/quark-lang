// quark/core/value.hpp - QValue tagged union type
#ifndef QUARK_CORE_VALUE_HPP
#define QUARK_CORE_VALUE_HPP

#include <cstdlib>
#include <vector>

// Forward declaration
struct QValue;
struct QResult;

// Type alias for list storage
using QList = std::vector<QValue>;

// QValue: Tagged union for all Quark runtime values
struct QValue {
        enum ValueType {
        VAL_INT,
        VAL_FLOAT,
        VAL_STRING,
        VAL_BOOL,
        VAL_NULL,
        VAL_LIST,
        VAL_FUNC,
        VAL_RESULT
    } type;

    union {
        long long int_val;
        double float_val;
        char* string_val;
        bool bool_val;
        QList* list_val;    // std::vector<QValue>* - automatic memory management
        void* func_val;
        QResult* result_val;
    } data;
};

// Function pointer types for dynamic calls (different arities)
using QFunc0 = QValue (*)();
using QFunc1 = QValue (*)(QValue);
using QFunc2 = QValue (*)(QValue, QValue);
using QFunc3 = QValue (*)(QValue, QValue, QValue);
using QFunc4 = QValue (*)(QValue, QValue, QValue, QValue);

struct QResult {
    bool is_ok;
    QValue payload;
};

#endif // QUARK_CORE_VALUE_HPP
