// quark/types/struct.hpp - QStruct: immutable fixed-shape record type
#ifndef QUARK_TYPES_STRUCT_HPP
#define QUARK_TYPES_STRUCT_HPP

#include "../core/value.hpp"
#include "../core/gc.hpp"
#include <cstdint>
#include <cstring>
#include <cstdio>

// QStructDef: static metadata for a struct type. One per struct definition,
// emitted as a global constant by codegen.
struct QStructDef {
    const char* name;              // struct type name (e.g. "Customer")
    const char* const* field_names; // array of field name strings
    int field_count;
};

// QStruct: a runtime struct instance. Allocated via GC with trailing QValue fields.
struct QStruct {
    const QStructDef* def;
    QValue fields[];  // flexible array of field_count QValues
};

// q_struct_create allocates a new struct with all fields set to qv_null().
// Caller must fill in fields afterwards.
inline QStruct* q_struct_create(const QStructDef* def) {
    size_t size = sizeof(QStruct) + static_cast<size_t>(def->field_count) * sizeof(QValue);
    QStruct* s = static_cast<QStruct*>(q_malloc(size));
    s->def = def;
    for (int i = 0; i < def->field_count; i++) {
        s->fields[i].type = QValue::VAL_NULL;
        s->fields[i].data.int_val = 0;
    }
    return s;
}

// q_struct_get_field returns the field value at the given index.
// Index must be valid (enforced at compile time by the analyzer).
inline QValue q_struct_get_field(QValue val, int index) {
    if (val.type != QValue::VAL_STRUCT || !val.data.struct_val) {
        fprintf(stderr, "QK-RUNTIME-001: expected struct, got different type\n");
        exit(1);
    }
    QStruct* s = val.data.struct_val;
    if (index < 0 || index >= s->def->field_count) {
        fprintf(stderr, "QK-RUNTIME-001: struct field index %d out of range\n", index);
        exit(1);
    }
    return s->fields[index];
}

// qv_struct wraps a QStruct pointer into a QValue.
inline QValue qv_struct(QStruct* s) {
    QValue q;
    q.type = QValue::VAL_STRUCT;
    q.data.struct_val = s;
    return q;
}

#endif // QUARK_TYPES_STRUCT_HPP
