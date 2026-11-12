// quark/core/cell.hpp - Mutable variable cell for shared closure captures
#ifndef QUARK_CORE_CELL_HPP
#define QUARK_CORE_CELL_HPP

#include <cstdlib>
#include "value.hpp"
#include "gc.hpp"
#include "diagnostics.hpp"

struct QCell {
    QValue value;
};

inline QCell* q_new_cell(QValue initial) {
    QCell* cell = static_cast<QCell*>(q_malloc(sizeof(QCell)));
    cell->value = initial;
    return cell;
}

// An unassigned cell holds a variable that a hoisted function captures before
// the variable's first assignment has run. Its value is VAL_NULL with
// data.func_val pointing at q_unassigned_tag; qv_null() zeroes data, so no
// real null looks like this. Any assignment overwrites the marker.
inline char q_unassigned_tag;

inline QCell* q_new_unassigned_cell() {
    QCell* cell = static_cast<QCell*>(q_malloc(sizeof(QCell)));
    cell->value.type = QValue::VAL_NULL;
    cell->value.data.func_val = &q_unassigned_tag;
    return cell;
}

// q_cell_get reads a captured variable that may still be unassigned (a call
// through a function value the compiler couldn't order before the
// assignment). Reading it is an error rather than null.
inline QValue q_cell_get(QCell* cell, const char* name) {
    if (cell->value.type == QValue::VAL_NULL && cell->value.data.func_val == &q_unassigned_tag) {
        q_runtime_reportf("runtime error: '%s' is used before it is assigned\n", name);
        std::exit(1);
    }
    return cell->value;
}

#endif // QUARK_CORE_CELL_HPP
