// quark/types/table.hpp - QTable: columnar, struct-typed table type (v0.1)
//
// Storage model:
//   - Each column is a QValue holding a QVector* (typed column vector).
//   - Column metadata (name, field index) is taken from the attached QStructDef.
//   - Row count is stored explicitly; all columns must have equal length.
//
// Row materialization:
//   - q_table_get_row(tbl, i) assembles a QStruct from the column vectors on demand.
//   - The returned struct is a value, not a live view.
//
// Mutability:
//   - v0.1: immutable after construction. No in-place cell assignment.
//
#ifndef QUARK_TYPES_TABLE_HPP
#define QUARK_TYPES_TABLE_HPP

#include "../core/value.hpp"
#include "../core/gc.hpp"
#include "../core/constructors.hpp"
#include "struct.hpp"
#include "vector.hpp"
#include <cstdint>
#include <cstdio>
#include <cstdlib>

// QTable: runtime table instance.
struct QTable {
    const QStructDef* def;   // row schema (field names, count)
    int               ncols; // == def->field_count
    int               nrows; // row count (all columns have this length)
    QValue*           cols;  // array of ncols QValue, each holding a QVector*
};

// qv_table wraps a QTable pointer into a QValue.
inline QValue qv_table(QTable* t) {
    QValue q;
    q.type = QValue::VAL_TABLE;
    q.data.table_val = t;
    return q;
}

// q_table_create allocates a QTable with ncols uninitialized column slots.
// Caller must fill cols[0..ncols-1] with column QVector* values.
inline QTable* q_table_create(const QStructDef* def, int nrows) {
    QTable* t = static_cast<QTable*>(q_malloc(sizeof(QTable)));
    t->def   = def;
    t->ncols = def->field_count;
    t->nrows = nrows;
    t->cols  = static_cast<QValue*>(q_malloc(static_cast<size_t>(def->field_count) * sizeof(QValue)));
    for (int i = 0; i < def->field_count; i++) {
        t->cols[i].type = QValue::VAL_NULL;
        t->cols[i].data.int_val = 0;
    }
    return t;
}

// q_table_get_col returns the QValue (VAL_VECTOR) for column index i.
inline QValue q_table_get_col(QValue tbl, int col_idx) {
    if (tbl.type != QValue::VAL_TABLE || !tbl.data.table_val) {
        fprintf(stderr, "QK-RUNTIME-001: q_table_get_col: expected table\n");
        exit(1);
    }
    QTable* t = tbl.data.table_val;
    if (col_idx < 0 || col_idx >= t->ncols) {
        fprintf(stderr, "QK-RUNTIME-001: q_table_get_col: column index %d out of range (ncols=%d)\n", col_idx, t->ncols);
        exit(1);
    }
    return t->cols[col_idx];
}

// q_table_get_col_by_name returns the column vector by name. Fatal if not found.
inline QValue q_table_get_col_by_name(QValue tbl, const char* name) {
    if (tbl.type != QValue::VAL_TABLE || !tbl.data.table_val) {
        fprintf(stderr, "QK-RUNTIME-001: q_table_get_col_by_name: expected table\n");
        exit(1);
    }
    QTable* t = tbl.data.table_val;
    for (int i = 0; i < t->ncols; i++) {
        if (strcmp(t->def->field_names[i], name) == 0) {
            return t->cols[i];
        }
    }
    fprintf(stderr, "QK-RUNTIME-001: table has no column '%s'\n", name);
    exit(1);
}

// q_table_get_row materialises row i as a QStruct value.
// Fatal on out-of-bounds — use q_table_get (planned post-v0.1) for safe access.
inline QValue q_table_get_row(QValue tbl, int idx) {
    if (tbl.type != QValue::VAL_TABLE || !tbl.data.table_val) {
        fprintf(stderr, "QK-RUNTIME-001: q_table_get_row: expected table\n");
        exit(1);
    }
    QTable* t = tbl.data.table_val;
    if (idx < 0) idx = t->nrows + idx;
    if (idx < 0 || idx >= t->nrows) {
        fprintf(stderr, "QK-RUNTIME-001: row index %d out of bounds for table with %d rows\n", idx, t->nrows);
        exit(1);
    }
    QStruct* s = q_struct_create(t->def);
    for (int c = 0; c < t->ncols; c++) {
        s->fields[c] = q_vec_get_scalar(t->cols[c], qv_int(idx));
    }
    return qv_struct(s);
}

// q_table_mask_filter applies a bool vector mask to a table, returning a new table
// containing only the rows where mask[i] is true.
inline QValue q_table_mask_filter(QValue tbl, QValue mask) {
    if (tbl.type != QValue::VAL_TABLE || !tbl.data.table_val) {
        fprintf(stderr, "QK-RUNTIME-001: q_table_mask_filter: expected table as first argument\n");
        exit(1);
    }
    if (mask.type != QValue::VAL_VECTOR || !mask.data.vector_val) {
        fprintf(stderr, "QK-RUNTIME-001: q_table_mask_filter: expected bool vector as mask\n");
        exit(1);
    }
    QTable* src = tbl.data.table_val;
    QVector* mv = mask.data.vector_val;
    if (mv->type != QVector::Type::BOOL) {
        fprintf(stderr, "QK-RUNTIME-001: table mask index requires bool vector\n");
        exit(1);
    }
    if (mv->count != static_cast<size_t>(src->nrows)) {
        fprintf(stderr, "QK-RUNTIME-001: table mask length mismatch: rows=%d, mask=%zu\n", src->nrows, mv->count);
        exit(1);
    }
    // Build filtered column vectors
    QTable* dst = q_table_create(src->def, 0);
    // Count selected rows first
    const auto& bools = std::get<QVecU8>(mv->storage);
    int selected = 0;
    for (int i = 0; i < src->nrows; i++) {
        if (i < static_cast<int>(bools.size()) && bools[i]) selected++;
    }
    dst->nrows = selected;
    // For each column, build a filtered vector
    for (int c = 0; c < src->ncols; c++) {
        dst->cols[c] = q_vec_mask_filter(src->cols[c], mask);
    }
    return qv_table(dst);
}

// q_table_index is the unified runtime indexing dispatch for tables.
// Accepts int (row index → struct) or vector[bool] (mask → table).
inline QValue q_table_index(QValue tbl, QValue index) {
    if (tbl.type != QValue::VAL_TABLE || !tbl.data.table_val) {
        fprintf(stderr, "QK-RUNTIME-001: q_table_index: expected table\n");
        exit(1);
    }
    if (index.type == QValue::VAL_INT) {
        return q_table_get_row(tbl, static_cast<int>(index.data.int_val));
    }
    if (index.type == QValue::VAL_VECTOR) {
        return q_table_mask_filter(tbl, index);
    }
    fprintf(stderr, "QK-RUNTIME-001: table index must be int or bool vector\n");
    exit(1);
}

#endif // QUARK_TYPES_TABLE_HPP
