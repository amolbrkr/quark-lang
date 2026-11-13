// quark/core/value.hpp - QValue tagged union type
#ifndef QUARK_CORE_VALUE_HPP
#define QUARK_CORE_VALUE_HPP

#include <cstdlib>
#include <vector>
#include "gc.hpp"
#include "diagnostics.hpp"

// Forward declarations
struct QValue;
struct QResult;
struct QDict;
struct QClosure;
struct QVector;
struct QResourceHandle;
struct QStruct;
struct QStructDef;
struct QTable;

struct QElemType;

// List storage — uses q_allocator so the GC can scan the vector's internal
// buffer for pointers to collectible objects. `elem` is the element type the
// list was created with (types/elemtype.hpp), nullptr for any value; every
// write through the runtime checks the value against it, so code that reads
// the elements can trust their type. Native code that appends with
// push_back directly must keep to it (qext::list_push checks it).
struct QList : std::vector<QValue, q_allocator<QValue>> {
    using std::vector<QValue, q_allocator<QValue>>::vector;
    const QElemType* elem = nullptr;
};

// QValue: Tagged union for all Quark runtime values
struct QValue {
        enum ValueType {
        VAL_INT,
        VAL_FLOAT,
        VAL_STRING,
        VAL_BOOL,
        VAL_NULL,
        VAL_LIST,
        VAL_VECTOR,
        VAL_DICT,
        VAL_FUNC,
        VAL_RESULT,
        VAL_RESOURCE,
        VAL_STRUCT,
        VAL_TABLE
    } type;

    union {
        long long int_val;
        double float_val;
        char* string_val;
        bool bool_val;
        QList* list_val;    // std::vector<QValue>* - automatic memory management
        QVector* vector_val; // typed column vector (f64/i64/bool/str/cat)
        QDict* dict_val;    // QDictMap* (GC-allocator-backed map)
        void* func_val;
        QResult* result_val;
        QResourceHandle* resource_val;
        QStruct* struct_val;
        QTable* table_val;
    } data;
};

// Function pointer types: all take QClosure* as hidden first parameter
using QClFunc0 = QValue (*)(QClosure*);
using QClFunc1 = QValue (*)(QClosure*, QValue);
using QClFunc2 = QValue (*)(QClosure*, QValue, QValue);
using QClFunc3 = QValue (*)(QClosure*, QValue, QValue, QValue);
using QClFunc4 = QValue (*)(QClosure*, QValue, QValue, QValue, QValue);
using QClFunc5 = QValue (*)(QClosure*, QValue, QValue, QValue, QValue, QValue);
using QClFunc6 = QValue (*)(QClosure*, QValue, QValue, QValue, QValue, QValue, QValue);
using QClFunc7 = QValue (*)(QClosure*, QValue, QValue, QValue, QValue, QValue, QValue, QValue);
using QClFunc8 = QValue (*)(QClosure*, QValue, QValue, QValue, QValue, QValue, QValue, QValue, QValue);
using QClFunc9 = QValue (*)(QClosure*, QValue, QValue, QValue, QValue, QValue, QValue, QValue, QValue, QValue);
using QClFunc10 = QValue (*)(QClosure*, QValue, QValue, QValue, QValue, QValue, QValue, QValue, QValue, QValue, QValue);
using QClFunc11 = QValue (*)(QClosure*, QValue, QValue, QValue, QValue, QValue, QValue, QValue, QValue, QValue, QValue, QValue);
using QClFunc12 = QValue (*)(QClosure*, QValue, QValue, QValue, QValue, QValue, QValue, QValue, QValue, QValue, QValue, QValue, QValue);

struct QResult {
    bool is_ok;
    QValue payload;
};

// QElemType describes the element type of a list or the value type of a
// dict (checks in types/elemtype.hpp); nullptr stands for any value.
struct QElemType {
    QValue::ValueType tag;
    const QElemType* elem;  // list and dict: element type; result: ok type (nullptr: any)
    const QElemType* err;   // result: err type (nullptr: any)
    const QStructDef* sdef; // struct: its definition
    int vdtype;             // vec: QVector::Type as int, -1 for any dtype
    const char* name;       // the type as the compiler writes it: "int", "list[int]"
};

inline constexpr QElemType q_elem_int{QValue::VAL_INT, nullptr, nullptr, nullptr, -1, "int"};
inline constexpr QElemType q_elem_float{QValue::VAL_FLOAT, nullptr, nullptr, nullptr, -1, "float"};
inline constexpr QElemType q_elem_bool{QValue::VAL_BOOL, nullptr, nullptr, nullptr, -1, "bool"};
inline constexpr QElemType q_elem_str{QValue::VAL_STRING, nullptr, nullptr, nullptr, -1, "str"};

// q_type_name is the name Quark uses for a value type ("int", "str", ...),
// as type() returns it. Struct values are shown by their struct's name where
// the struct definition is available (q_value_type_name in elemtype.hpp).
inline const char* q_type_name(QValue::ValueType t) {
    switch (t) {
        case QValue::VAL_INT: return "int";
        case QValue::VAL_FLOAT: return "float";
        case QValue::VAL_STRING: return "str";
        case QValue::VAL_BOOL: return "bool";
        case QValue::VAL_NULL: return "null";
        case QValue::VAL_LIST: return "list";
        case QValue::VAL_VECTOR: return "vec";
        case QValue::VAL_DICT: return "dict";
        case QValue::VAL_FUNC: return "fn";
        case QValue::VAL_RESULT: return "result";
        case QValue::VAL_RESOURCE: return "resource";
        case QValue::VAL_STRUCT: return "struct";
        case QValue::VAL_TABLE: return "tbl";
    }
    return "unknown";
}

#endif // QUARK_CORE_VALUE_HPP
