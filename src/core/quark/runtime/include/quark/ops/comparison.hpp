// quark/ops/comparison.hpp - Comparison operations
#ifndef QUARK_OPS_COMPARISON_HPP
#define QUARK_OPS_COMPARISON_HPP

#include "../core/value.hpp"
#include "../core/constructors.hpp"
#include "../types/resource.hpp"
#include <cstring>
#include <cstdio>
#include <cstdlib>

// Helper functions to_double() and either_float() are defined in arithmetic.hpp.

// Less than
inline QValue q_lt(QValue a, QValue b) {
    if (a.type == QValue::VAL_VECTOR || b.type == QValue::VAL_VECTOR) {
        return q_vec_lt(a, b);
    }
    // Type guard: only INT and FLOAT are valid
    if ((a.type != QValue::VAL_INT && a.type != QValue::VAL_FLOAT) ||
        (b.type != QValue::VAL_INT && b.type != QValue::VAL_FLOAT)) {
        q_runtime_reportf("runtime error: operator '<' expects numeric operands\n");
        std::exit(1);
    }
    if (quark::detail::either_float(a, b)) {
        return qv_bool(quark::detail::to_double(a) < quark::detail::to_double(b));
    }
    return qv_bool(a.data.int_val < b.data.int_val);
}

// Less than or equal
inline QValue q_lte(QValue a, QValue b) {
    if (a.type == QValue::VAL_VECTOR || b.type == QValue::VAL_VECTOR) {
        return q_vec_lte(a, b);
    }
    // Type guard: only INT and FLOAT are valid
    if ((a.type != QValue::VAL_INT && a.type != QValue::VAL_FLOAT) ||
        (b.type != QValue::VAL_INT && b.type != QValue::VAL_FLOAT)) {
        q_runtime_reportf("runtime error: operator '<=' expects numeric operands\n");
        std::exit(1);
    }
    if (quark::detail::either_float(a, b)) {
        return qv_bool(quark::detail::to_double(a) <= quark::detail::to_double(b));
    }
    return qv_bool(a.data.int_val <= b.data.int_val);
}

// Greater than
inline QValue q_gt(QValue a, QValue b) {
    if (a.type == QValue::VAL_VECTOR || b.type == QValue::VAL_VECTOR) {
        return q_vec_gt(a, b);
    }
    // Type guard: only INT and FLOAT are valid
    if ((a.type != QValue::VAL_INT && a.type != QValue::VAL_FLOAT) ||
        (b.type != QValue::VAL_INT && b.type != QValue::VAL_FLOAT)) {
        q_runtime_reportf("runtime error: operator '>' expects numeric operands\n");
        std::exit(1);
    }
    if (quark::detail::either_float(a, b)) {
        return qv_bool(quark::detail::to_double(a) > quark::detail::to_double(b));
    }
    return qv_bool(a.data.int_val > b.data.int_val);
}

// Greater than or equal
inline QValue q_gte(QValue a, QValue b) {
    if (a.type == QValue::VAL_VECTOR || b.type == QValue::VAL_VECTOR) {
        return q_vec_gte(a, b);
    }
    // Type guard: only INT and FLOAT are valid
    if ((a.type != QValue::VAL_INT && a.type != QValue::VAL_FLOAT) ||
        (b.type != QValue::VAL_INT && b.type != QValue::VAL_FLOAT)) {
        q_runtime_reportf("runtime error: operator '>=' expects numeric operands\n");
        std::exit(1);
    }
    if (quark::detail::either_float(a, b)) {
        return qv_bool(quark::detail::to_double(a) >= quark::detail::to_double(b));
    }
    return qv_bool(a.data.int_val >= b.data.int_val);
}

namespace quark { namespace detail {

// Nesting limit for structural equality; guards against cyclic containers
// (a list pushed into itself, for example) recursing forever.
constexpr int kMaxEqualityDepth = 512;

// Whole-vector equality: same dtype, length, null positions and values.
// Used when vectors are compared as elements of a list, dict or result;
// top-level `v == w` stays element-wise.
inline bool vectors_identical(const QVector& a, const QVector& b) {
    if (a.type != b.type || a.count != b.count) return false;
    for (size_t i = 0; i < a.count; i++) {
        const bool an = q_vec_is_null_at(a, i);
        const bool bn = q_vec_is_null_at(b, i);
        if (an != bn) return false;
        if (an) continue;
        switch (a.type) {
            case QVector::Type::F64:
                if (q_vec_f64_data(a)[i] != q_vec_f64_data(b)[i]) return false;
                break;
            case QVector::Type::I64:
                if (q_vec_i64_data(a)[i] != q_vec_i64_data(b)[i]) return false;
                break;
            case QVector::Type::BOOL:
                if (q_vec_bool_at(a, i) != q_vec_bool_at(b, i)) return false;
                break;
            case QVector::Type::STR:
                if (q_vec_str_at(a, i) != q_vec_str_at(b, i)) return false;
                break;
        }
    }
    return true;
}

// Structural equality. Lists compare element-wise in order, dicts compare
// key sets and values, results compare tag and payload. Functions compare
// by identity. Struct equality is rejected at compile time.
inline bool values_equal(const QValue& a, const QValue& b, int depth) {
    if (depth > kMaxEqualityDepth) {
        q_runtime_reportf("runtime error: '==' nesting too deep (cyclic list or dict?)\n");
        std::exit(1);
    }
    if (a.type != b.type) {
        if ((a.type == QValue::VAL_INT || a.type == QValue::VAL_FLOAT) &&
            (b.type == QValue::VAL_INT || b.type == QValue::VAL_FLOAT)) {
            return to_double(a) == to_double(b);
        }
        return false;
    }
    switch (a.type) {
        case QValue::VAL_INT:
            return a.data.int_val == b.data.int_val;
        case QValue::VAL_FLOAT:
            return a.data.float_val == b.data.float_val;
        case QValue::VAL_BOOL:
            return a.data.bool_val == b.data.bool_val;
        case QValue::VAL_STRING:
            if (!a.data.string_val || !b.data.string_val) {
                return a.data.string_val == b.data.string_val;
            }
            return strcmp(a.data.string_val, b.data.string_val) == 0;
        case QValue::VAL_NULL:
            return true;
        case QValue::VAL_RESOURCE:
            if (!a.data.resource_val || !b.data.resource_val) {
                return a.data.resource_val == b.data.resource_val;
            }
            return a.data.resource_val->slot == b.data.resource_val->slot &&
                   a.data.resource_val->generation == b.data.resource_val->generation &&
                   a.data.resource_val->kind == b.data.resource_val->kind;
        case QValue::VAL_LIST: {
            const QList* la = a.data.list_val;
            const QList* lb = b.data.list_val;
            if (la == lb) return true;
            if (!la || !lb || la->size() != lb->size()) return false;
            for (size_t i = 0; i < la->size(); i++) {
                if (!values_equal((*la)[i], (*lb)[i], depth + 1)) return false;
            }
            return true;
        }
        case QValue::VAL_DICT: {
            const QDict* da = a.data.dict_val;
            const QDict* db = b.data.dict_val;
            if (da == db) return true;
            if (!da || !db || da->entries.size() != db->entries.size()) return false;
            for (const auto& kv : da->entries) {
                auto it = db->entries.find(kv.first);
                if (it == db->entries.end()) return false;
                if (!values_equal(kv.second, it->second, depth + 1)) return false;
            }
            return true;
        }
        case QValue::VAL_RESULT: {
            const QResult* ra = a.data.result_val;
            const QResult* rb = b.data.result_val;
            if (ra == rb) return true;
            if (!ra || !rb || ra->is_ok != rb->is_ok) return false;
            return values_equal(ra->payload, rb->payload, depth + 1);
        }
        case QValue::VAL_VECTOR: {
            const QVector* va = a.data.vector_val;
            const QVector* vb = b.data.vector_val;
            if (va == vb) return true;
            if (!va || !vb) return false;
            return vectors_identical(*va, *vb);
        }
        case QValue::VAL_FUNC:
            return a.data.func_val == b.data.func_val;
        default:
            return false;
    }
}

}} // namespace quark::detail

// Equality (type-sensitive, structural for containers)
inline QValue q_eq(QValue a, QValue b) {
    if (a.type == QValue::VAL_VECTOR || b.type == QValue::VAL_VECTOR) {
        return q_vec_eq(a, b);
    }
    return qv_bool(quark::detail::values_equal(a, b, 0));
}

// Not equal
inline QValue q_neq(QValue a, QValue b) {
    if (a.type == QValue::VAL_VECTOR || b.type == QValue::VAL_VECTOR) {
        return q_vec_neq(a, b);
    }
    return qv_bool(!q_eq(a, b).data.bool_val);
}

#endif // QUARK_OPS_COMPARISON_HPP
