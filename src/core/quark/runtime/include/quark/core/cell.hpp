// quark/core/cell.hpp - Mutable variable cell for shared closure captures
#ifndef QUARK_CORE_CELL_HPP
#define QUARK_CORE_CELL_HPP

#include "value.hpp"
#include "gc.hpp"

struct QCell {
    QValue value;
};

inline QCell* q_new_cell(QValue initial) {
    QCell* cell = static_cast<QCell*>(q_malloc(sizeof(QCell)));
    cell->value = initial;
    return cell;
}

#endif // QUARK_CORE_CELL_HPP
