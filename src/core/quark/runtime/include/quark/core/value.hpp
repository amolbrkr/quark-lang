// quark/core/value.hpp - QValue tagged union type
#ifndef QUARK_CORE_VALUE_HPP
#define QUARK_CORE_VALUE_HPP

#include <cstdlib>
#include <vector>

// Forward declarations
struct QValue;
struct QResult;
struct QDict;
struct QClosure;

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
        VAL_DICT,
        VAL_FUNC,
        VAL_RESULT
    } type;

    union {
        long long int_val;
        double float_val;
        char* string_val;
        bool bool_val;
        QList* list_val;    // std::vector<QValue>* - automatic memory management
        QDict* dict_val;    // std::unordered_map<std::string, QValue>*
        void* func_val;
        QResult* result_val;
    } data;
};

// Function pointer types: all take QClosure* as hidden first parameter
using QClFunc0 = QValue (*)(QClosure*);
using QClFunc1 = QValue (*)(QClosure*, QValue);
using QClFunc2 = QValue (*)(QClosure*, QValue, QValue);
using QClFunc3 = QValue (*)(QClosure*, QValue, QValue, QValue);
using QClFunc4 = QValue (*)(QClosure*, QValue, QValue, QValue, QValue);

struct QResult {
    bool is_ok;
    QValue payload;
};

#endif // QUARK_CORE_VALUE_HPP
