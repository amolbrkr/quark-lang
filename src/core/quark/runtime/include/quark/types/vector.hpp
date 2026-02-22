// quark/types/vector.hpp - Typed 1D vector runtime kernels
#ifndef QUARK_TYPES_VECTOR_HPP
#define QUARK_TYPES_VECTOR_HPP

#include "../core/value.hpp"
#include "../core/constructors.hpp"

#include <algorithm>
#include <cstdio>
#include <cstdint>
#include <cstring>
#include <unordered_map>
#include <string>
#include <variant>
#include <vector>

struct QStringStorage {
    std::vector<uint32_t> offsets;
    std::vector<char> bytes;
};

struct QNullMask {
    std::vector<uint8_t> is_null; // 0 = valid, 1 = null
};

struct QVector {
    enum class Type { F64, I64, BOOL, STR };

    Type type;
    size_t count;
    bool has_nulls;
    std::variant<
        std::vector<double>,
        std::vector<int64_t>,
        std::vector<uint8_t>,
        QStringStorage
    > storage;
    QNullMask nulls;

    QVector()
        : type(Type::F64),
          count(0),
          has_nulls(false),
          storage(std::vector<double>{}),
          nulls() {}
};

inline bool q_vec_has_valid_handle(QValue vec) {
    return vec.type == QValue::VAL_VECTOR && vec.data.vector_val;
}

inline bool q_vec_storage_matches_type(const QVector& vec) {
    switch (vec.type) {
        case QVector::Type::F64: return std::holds_alternative<std::vector<double>>(vec.storage);
        case QVector::Type::I64: return std::holds_alternative<std::vector<int64_t>>(vec.storage);
        case QVector::Type::BOOL: return std::holds_alternative<std::vector<uint8_t>>(vec.storage);
        case QVector::Type::STR: return std::holds_alternative<QStringStorage>(vec.storage);
        default: return false;
    }
}

inline bool q_vec_validate(const QVector& vec) {
    if (!q_vec_storage_matches_type(vec)) {
        return false;
    }

    switch (vec.type) {
        case QVector::Type::F64:
            if (std::get<std::vector<double>>(vec.storage).size() != vec.count) {
                return false;
            }
            break;
        case QVector::Type::I64:
            if (std::get<std::vector<int64_t>>(vec.storage).size() != vec.count) {
                return false;
            }
            break;
        case QVector::Type::BOOL:
            if (std::get<std::vector<uint8_t>>(vec.storage).size() != vec.count) {
                return false;
            }
            break;
        case QVector::Type::STR: {
            const QStringStorage& s = std::get<QStringStorage>(vec.storage);
            if (s.offsets.size() != vec.count + 1 || s.offsets.empty() || s.offsets.front() != 0) {
                return false;
            }
            if (static_cast<size_t>(s.offsets.back()) != s.bytes.size()) {
                return false;
            }
            for (size_t i = 1; i < s.offsets.size(); i++) {
                if (s.offsets[i] < s.offsets[i - 1]) {
                    return false;
                }
            }
            break;
        }
    }

    if (!vec.has_nulls) {
        return vec.nulls.is_null.empty();
    }

    return vec.nulls.is_null.size() == vec.count;
}

inline const char* q_vec_dtype_name(const QVector& vec) {
    switch (vec.type) {
        case QVector::Type::F64: return "f64";
        case QVector::Type::I64: return "i64";
        case QVector::Type::BOOL: return "bool";
        case QVector::Type::STR: return "str";
        default: return "unknown";
    }
}

inline bool q_vec_is_type(QValue vec, QVector::Type type) {
    return q_vec_has_valid_handle(vec) && vec.data.vector_val->type == type && q_vec_validate(*vec.data.vector_val);
}

inline std::vector<double>* q_vec_f64_mut(QValue vec) {
    if (!q_vec_is_type(vec, QVector::Type::F64)) {
        return nullptr;
    }
    return &std::get<std::vector<double>>(vec.data.vector_val->storage);
}

inline const std::vector<double>* q_vec_f64_const(QValue vec) {
    if (!q_vec_is_type(vec, QVector::Type::F64)) {
        return nullptr;
    }
    return &std::get<std::vector<double>>(vec.data.vector_val->storage);
}

inline std::vector<int64_t>* q_vec_i64_mut(QValue vec) {
    if (!q_vec_is_type(vec, QVector::Type::I64)) {
        return nullptr;
    }
    return &std::get<std::vector<int64_t>>(vec.data.vector_val->storage);
}

inline const std::vector<int64_t>* q_vec_i64_const(QValue vec) {
    if (!q_vec_is_type(vec, QVector::Type::I64)) {
        return nullptr;
    }
    return &std::get<std::vector<int64_t>>(vec.data.vector_val->storage);
}

inline std::vector<uint8_t>* q_vec_bool_mut(QValue vec) {
    if (!q_vec_is_type(vec, QVector::Type::BOOL)) {
        return nullptr;
    }
    return &std::get<std::vector<uint8_t>>(vec.data.vector_val->storage);
}

inline const std::vector<uint8_t>* q_vec_bool_const(QValue vec) {
    if (!q_vec_is_type(vec, QVector::Type::BOOL)) {
        return nullptr;
    }
    return &std::get<std::vector<uint8_t>>(vec.data.vector_val->storage);
}

inline void q_vec_ensure_null_mask(QVector& vec) {
    if (!vec.has_nulls) {
        vec.has_nulls = true;
        vec.nulls.is_null.assign(vec.count, static_cast<uint8_t>(0));
    }
}

inline bool q_vec_is_null_at(const QVector& vec, size_t index) {
    if (!vec.has_nulls || index >= vec.count) {
        return false;
    }
    return vec.nulls.is_null[index] != 0;
}

inline bool q_vec_set_null_at(QValue vec, size_t index, bool is_null) {
    if (!q_vec_has_valid_handle(vec) || index >= vec.data.vector_val->count) {
        return false;
    }
    QVector& qvec = *vec.data.vector_val;
    q_vec_ensure_null_mask(qvec);
    qvec.nulls.is_null[index] = static_cast<uint8_t>(is_null ? 1 : 0);
    return true;
}

inline bool q_is_numeric_scalar(QValue v) {
    return v.type == QValue::VAL_INT || v.type == QValue::VAL_FLOAT;
}

inline bool q_is_integral_scalar(QValue v) {
    return v.type == QValue::VAL_INT || v.type == QValue::VAL_BOOL;
}

inline bool q_is_boolish_scalar(QValue v) {
    return v.type == QValue::VAL_BOOL || v.type == QValue::VAL_INT;
}

inline double q_to_double_scalar(QValue v) {
    return v.type == QValue::VAL_FLOAT ? v.data.float_val : static_cast<double>(v.data.int_val);
}

inline int64_t q_to_i64_scalar(QValue v) {
    if (v.type == QValue::VAL_BOOL) {
        return v.data.bool_val ? 1 : 0;
    }
    if (v.type == QValue::VAL_FLOAT) {
        return static_cast<int64_t>(v.data.float_val);
    }
    return static_cast<int64_t>(v.data.int_val);
}

inline QValue qv_vector(int initial_cap = 0) {
    QValue q;
    q.type = QValue::VAL_VECTOR;
    q.data.vector_val = new QVector();
    if (initial_cap > 0) {
        std::get<std::vector<double>>(q.data.vector_val->storage).reserve(static_cast<size_t>(initial_cap));
    }
    return q;
}

inline QValue qv_vector_i64(int initial_cap = 0) {
    QValue q;
    q.type = QValue::VAL_VECTOR;
    q.data.vector_val = new QVector();
    q.data.vector_val->type = QVector::Type::I64;
    q.data.vector_val->storage = std::vector<int64_t>{};
    if (initial_cap > 0) {
        std::get<std::vector<int64_t>>(q.data.vector_val->storage).reserve(static_cast<size_t>(initial_cap));
    }
    return q;
}

inline QValue qv_vector_bool(int initial_cap = 0) {
    QValue q;
    q.type = QValue::VAL_VECTOR;
    q.data.vector_val = new QVector();
    q.data.vector_val->type = QVector::Type::BOOL;
    q.data.vector_val->storage = std::vector<uint8_t>{};
    if (initial_cap > 0) {
        std::get<std::vector<uint8_t>>(q.data.vector_val->storage).reserve(static_cast<size_t>(initial_cap));
    }
    return q;
}

inline QValue qv_vector_str(int initial_string_cap = 0, int initial_byte_cap = 0) {
    QValue q;
    q.type = QValue::VAL_VECTOR;
    q.data.vector_val = new QVector();
    q.data.vector_val->type = QVector::Type::STR;
    QStringStorage storage;
    storage.offsets.push_back(0);
    if (initial_string_cap > 0) {
        storage.offsets.reserve(static_cast<size_t>(initial_string_cap) + 1);
    }
    if (initial_byte_cap > 0) {
        storage.bytes.reserve(static_cast<size_t>(initial_byte_cap));
    }
    q.data.vector_val->storage = std::move(storage);
    return q;
}

inline QValue q_vec_push(QValue vec, QValue value) {
    if (!q_vec_has_valid_handle(vec)) {
        return qv_null();
    }
    if (vec.data.vector_val->type != QVector::Type::F64) {
        return qv_null();
    }
    if (!q_is_numeric_scalar(value)) {
        return qv_null();
    }
    std::vector<double>& values = std::get<std::vector<double>>(vec.data.vector_val->storage);
    values.push_back(q_to_double_scalar(value));
    vec.data.vector_val->count = values.size();
    if (vec.data.vector_val->has_nulls) {
        vec.data.vector_val->nulls.is_null.push_back(0);
    }
    return vec;
}

inline QValue q_vec_push_i64(QValue vec, QValue value) {
    if (!q_vec_has_valid_handle(vec) || vec.data.vector_val->type != QVector::Type::I64) {
        return qv_null();
    }
    if (!(value.type == QValue::VAL_INT || value.type == QValue::VAL_FLOAT || value.type == QValue::VAL_BOOL)) {
        return qv_null();
    }
    std::vector<int64_t>& values = std::get<std::vector<int64_t>>(vec.data.vector_val->storage);
    values.push_back(q_to_i64_scalar(value));
    vec.data.vector_val->count = values.size();
    if (vec.data.vector_val->has_nulls) {
        vec.data.vector_val->nulls.is_null.push_back(0);
    }
    return vec;
}

inline QValue q_vec_push_bool(QValue vec, QValue value) {
    if (!q_vec_has_valid_handle(vec) || vec.data.vector_val->type != QVector::Type::BOOL) {
        return qv_null();
    }
    if (!q_is_boolish_scalar(value)) {
        return qv_null();
    }
    std::vector<uint8_t>& values = std::get<std::vector<uint8_t>>(vec.data.vector_val->storage);
    const bool b = (value.type == QValue::VAL_BOOL) ? value.data.bool_val : (value.data.int_val != 0);
    values.push_back(static_cast<uint8_t>(b ? 1 : 0));
    vec.data.vector_val->count = values.size();
    if (vec.data.vector_val->has_nulls) {
        vec.data.vector_val->nulls.is_null.push_back(0);
    }
    return vec;
}

inline QStringStorage q_vec_encode_strings(const std::vector<std::string>& values) {
    QStringStorage out;
    out.offsets.reserve(values.size() + 1);
    out.offsets.push_back(0);
    size_t total = 0;
    for (const auto& s : values) {
        total += s.size();
        out.bytes.insert(out.bytes.end(), s.begin(), s.end());
        out.offsets.push_back(static_cast<uint32_t>(total));
    }
    return out;
}

inline std::vector<std::string> q_vec_decode_strings(const QStringStorage& storage, size_t count) {
    std::vector<std::string> values;
    values.reserve(count);
    for (size_t i = 0; i < count; i++) {
        uint32_t start = storage.offsets[i];
        uint32_t end = storage.offsets[i + 1];
        values.emplace_back(storage.bytes.data() + start, storage.bytes.data() + end);
    }
    return values;
}

inline QValue q_vec_clone(QValue vec) {
    if (!q_vec_has_valid_handle(vec) || !q_vec_validate(*vec.data.vector_val)) {
        return qv_null();
    }
    QValue out;
    out.type = QValue::VAL_VECTOR;
    out.data.vector_val = new QVector(*vec.data.vector_val);
    return out;
}

inline int q_vec_size(QValue vec) {
    if (!q_vec_has_valid_handle(vec) || !q_vec_validate(*vec.data.vector_val)) {
        return 0;
    }
    return static_cast<int>(vec.data.vector_val->count);
}

inline QValue q_vec_dtype(QValue vec) {
    if (!q_vec_has_valid_handle(vec) || !q_vec_validate(*vec.data.vector_val)) {
        return qv_null();
    }
    return qv_string(q_vec_dtype_name(*vec.data.vector_val));
}

template <typename BinaryOp>
inline QValue q_vec_binary_impl(QValue a, QValue b, BinaryOp op) {
    const bool aVec = q_vec_has_valid_handle(a);
    const bool bVec = q_vec_has_valid_handle(b);

    if (!aVec && !bVec) {
        return qv_null();
    }

    if (aVec && bVec) {
        const std::vector<double>* avp = q_vec_f64_const(a);
        const std::vector<double>* bvp = q_vec_f64_const(b);
        if (!avp || !bvp) {
            return qv_null();
        }
        const std::vector<double>& av = *avp;
        const std::vector<double>& bv = *bvp;
        if (av.size() != bv.size()) {
            return qv_null();
        }
        QValue out = qv_vector(static_cast<int>(av.size()));
        std::vector<double>& outv = std::get<std::vector<double>>(out.data.vector_val->storage);
        outv.resize(av.size());
        out.data.vector_val->count = av.size();

        for (size_t i = 0; i < av.size(); i++) {
            outv[i] = op(av[i], bv[i]);
        }
        return out;
    }

    if (aVec && q_is_numeric_scalar(b)) {
        const std::vector<double>* avp = q_vec_f64_const(a);
        if (!avp) {
            return qv_null();
        }
        const std::vector<double>& av = *avp;
        double bs = q_to_double_scalar(b);
        QValue out = qv_vector(static_cast<int>(av.size()));
        std::vector<double>& outv = std::get<std::vector<double>>(out.data.vector_val->storage);
        outv.resize(av.size());
        out.data.vector_val->count = av.size();
        for (size_t i = 0; i < av.size(); i++) {
            outv[i] = op(av[i], bs);
        }
        return out;
    }

    if (bVec && q_is_numeric_scalar(a)) {
        const std::vector<double>* bvp = q_vec_f64_const(b);
        if (!bvp) {
            return qv_null();
        }
        const std::vector<double>& bv = *bvp;
        double as = q_to_double_scalar(a);
        QValue out = qv_vector(static_cast<int>(bv.size()));
        std::vector<double>& outv = std::get<std::vector<double>>(out.data.vector_val->storage);
        outv.resize(bv.size());
        out.data.vector_val->count = bv.size();
        for (size_t i = 0; i < bv.size(); i++) {
            outv[i] = op(as, bv[i]);
        }
        return out;
    }

    return qv_null();
}

template <typename BinaryOp>
inline QValue q_vec_binary_i64_impl(QValue a, QValue b, BinaryOp op) {
    const bool aVec = q_vec_has_valid_handle(a);
    const bool bVec = q_vec_has_valid_handle(b);

    if (aVec && bVec) {
        const std::vector<int64_t>* avp = q_vec_i64_const(a);
        const std::vector<int64_t>* bvp = q_vec_i64_const(b);
        if (!avp || !bvp || avp->size() != bvp->size()) {
            return qv_null();
        }
        QValue out = qv_vector_i64(static_cast<int>(avp->size()));
        std::vector<int64_t>& outv = std::get<std::vector<int64_t>>(out.data.vector_val->storage);
        outv.resize(avp->size());
        out.data.vector_val->count = avp->size();
        for (size_t i = 0; i < avp->size(); i++) {
            outv[i] = op((*avp)[i], (*bvp)[i]);
        }
        return out;
    }

    if (aVec && q_is_integral_scalar(b)) {
        const std::vector<int64_t>* avp = q_vec_i64_const(a);
        if (!avp) {
            return qv_null();
        }
        const int64_t bs = q_to_i64_scalar(b);
        QValue out = qv_vector_i64(static_cast<int>(avp->size()));
        std::vector<int64_t>& outv = std::get<std::vector<int64_t>>(out.data.vector_val->storage);
        outv.resize(avp->size());
        out.data.vector_val->count = avp->size();
        for (size_t i = 0; i < avp->size(); i++) {
            outv[i] = op((*avp)[i], bs);
        }
        return out;
    }

    if (bVec && q_is_integral_scalar(a)) {
        const std::vector<int64_t>* bvp = q_vec_i64_const(b);
        if (!bvp) {
            return qv_null();
        }
        const int64_t as = q_to_i64_scalar(a);
        QValue out = qv_vector_i64(static_cast<int>(bvp->size()));
        std::vector<int64_t>& outv = std::get<std::vector<int64_t>>(out.data.vector_val->storage);
        outv.resize(bvp->size());
        out.data.vector_val->count = bvp->size();
        for (size_t i = 0; i < bvp->size(); i++) {
            outv[i] = op(as, (*bvp)[i]);
        }
        return out;
    }

    return qv_null();
}

inline QValue q_vec_div_i64(QValue a, QValue b) {
    const bool aVec = q_vec_has_valid_handle(a);
    const bool bVec = q_vec_has_valid_handle(b);

    if (aVec && bVec) {
        const std::vector<int64_t>* avp = q_vec_i64_const(a);
        const std::vector<int64_t>* bvp = q_vec_i64_const(b);
        if (!avp || !bvp || avp->size() != bvp->size()) {
            return qv_null();
        }
        QValue out = qv_vector(static_cast<int>(avp->size()));
        std::vector<double>& outv = std::get<std::vector<double>>(out.data.vector_val->storage);
        outv.resize(avp->size());
        out.data.vector_val->count = avp->size();
        for (size_t i = 0; i < avp->size(); i++) {
            outv[i] = static_cast<double>((*avp)[i]) / static_cast<double>((*bvp)[i]);
        }
        return out;
    }

    if (aVec && q_is_integral_scalar(b)) {
        const std::vector<int64_t>* avp = q_vec_i64_const(a);
        if (!avp) {
            return qv_null();
        }
        const double bs = static_cast<double>(q_to_i64_scalar(b));
        QValue out = qv_vector(static_cast<int>(avp->size()));
        std::vector<double>& outv = std::get<std::vector<double>>(out.data.vector_val->storage);
        outv.resize(avp->size());
        out.data.vector_val->count = avp->size();
        for (size_t i = 0; i < avp->size(); i++) {
            outv[i] = static_cast<double>((*avp)[i]) / bs;
        }
        return out;
    }

    if (bVec && q_is_integral_scalar(a)) {
        const std::vector<int64_t>* bvp = q_vec_i64_const(b);
        if (!bvp) {
            return qv_null();
        }
        const double as = static_cast<double>(q_to_i64_scalar(a));
        QValue out = qv_vector(static_cast<int>(bvp->size()));
        std::vector<double>& outv = std::get<std::vector<double>>(out.data.vector_val->storage);
        outv.resize(bvp->size());
        out.data.vector_val->count = bvp->size();
        for (size_t i = 0; i < bvp->size(); i++) {
            outv[i] = as / static_cast<double>((*bvp)[i]);
        }
        return out;
    }

    return qv_null();
}

inline QValue q_vec_add(QValue a, QValue b) {
    if (q_vec_is_type(a, QVector::Type::I64) || q_vec_is_type(b, QVector::Type::I64)) {
        QValue out = q_vec_binary_i64_impl(a, b, [](int64_t x, int64_t y) { return x + y; });
        if (out.type != QValue::VAL_NULL) {
            return out;
        }
    }
    return q_vec_binary_impl(a, b, [](double x, double y) { return x + y; });
}

inline QValue q_vec_sub(QValue a, QValue b) {
    if (q_vec_is_type(a, QVector::Type::I64) || q_vec_is_type(b, QVector::Type::I64)) {
        QValue out = q_vec_binary_i64_impl(a, b, [](int64_t x, int64_t y) { return x - y; });
        if (out.type != QValue::VAL_NULL) {
            return out;
        }
    }
    return q_vec_binary_impl(a, b, [](double x, double y) { return x - y; });
}

inline QValue q_vec_mul(QValue a, QValue b) {
    if (q_vec_is_type(a, QVector::Type::I64) || q_vec_is_type(b, QVector::Type::I64)) {
        QValue out = q_vec_binary_i64_impl(a, b, [](int64_t x, int64_t y) { return x * y; });
        if (out.type != QValue::VAL_NULL) {
            return out;
        }
    }
    return q_vec_binary_impl(a, b, [](double x, double y) { return x * y; });
}

inline QValue q_vec_div(QValue a, QValue b) {
    if (q_vec_is_type(a, QVector::Type::I64) || q_vec_is_type(b, QVector::Type::I64)) {
        QValue out = q_vec_div_i64(a, b);
        if (out.type != QValue::VAL_NULL) {
            return out;
        }
    }
    return q_vec_binary_impl(a, b, [](double x, double y) { return x / y; });
}

inline QValue q_vec_sum(QValue vec) {
    const std::vector<int64_t>* vi = q_vec_i64_const(vec);
    if (vi) {
        double acc = 0.0;
        for (size_t i = 0; i < vi->size(); i++) {
            acc += static_cast<double>((*vi)[i]);
        }
        return qv_float(acc);
    }

    const std::vector<uint8_t>* vb = q_vec_bool_const(vec);
    if (vb) {
        double acc = 0.0;
        for (size_t i = 0; i < vb->size(); i++) {
            acc += ((*vb)[i] != 0) ? 1.0 : 0.0;
        }
        return qv_float(acc);
    }

    const std::vector<double>* vp = q_vec_f64_const(vec);
    if (!vp) {
        return qv_null();
    }
    const std::vector<double>& v = *vp;
    double acc = 0.0;
    for (size_t i = 0; i < v.size(); i++) {
        acc += v[i];
    }
    return qv_float(acc);
}

inline QValue q_vec_min(QValue vec) {
    const std::vector<int64_t>* vi = q_vec_i64_const(vec);
    if (vi && !vi->empty()) {
        int64_t cur = (*vi)[0];
        for (size_t i = 1; i < vi->size(); i++) {
            cur = std::min(cur, (*vi)[i]);
        }
        return qv_float(static_cast<double>(cur));
    }

    const std::vector<double>* vp = q_vec_f64_const(vec);
    if (!vp || vp->empty()) {
        return qv_null();
    }
    const std::vector<double>& v = *vp;
    double cur = v[0];
    for (size_t i = 1; i < v.size(); i++) {
        cur = std::min(cur, v[i]);
    }
    return qv_float(cur);
}

inline QValue q_vec_max(QValue vec) {
    const std::vector<int64_t>* vi = q_vec_i64_const(vec);
    if (vi && !vi->empty()) {
        int64_t cur = (*vi)[0];
        for (size_t i = 1; i < vi->size(); i++) {
            cur = std::max(cur, (*vi)[i]);
        }
        return qv_float(static_cast<double>(cur));
    }

    const std::vector<double>* vp = q_vec_f64_const(vec);
    if (!vp || vp->empty()) {
        return qv_null();
    }
    const std::vector<double>& v = *vp;
    double cur = v[0];
    for (size_t i = 1; i < v.size(); i++) {
        cur = std::max(cur, v[i]);
    }
    return qv_float(cur);
}

inline QValue q_fillna(QValue vec, QValue value) {
    if (!q_vec_has_valid_handle(vec) || !q_vec_validate(*vec.data.vector_val)) {
        return qv_null();
    }

    QVector& out = *vec.data.vector_val;
    if (!out.has_nulls || out.nulls.is_null.empty()) {
        return vec;
    }

    switch (out.type) {
        case QVector::Type::F64: {
            if (!q_is_numeric_scalar(value)) {
                return qv_null();
            }
            auto& values = std::get<std::vector<double>>(out.storage);
            const double fill = q_to_double_scalar(value);
            for (size_t i = 0; i < out.count; i++) {
                if (out.nulls.is_null[i] != 0) {
                    values[i] = fill;
                }
            }
            out.has_nulls = false;
            out.nulls.is_null.clear();
            return vec;
        }
        case QVector::Type::I64: {
            if (!(value.type == QValue::VAL_INT || value.type == QValue::VAL_FLOAT || value.type == QValue::VAL_BOOL)) {
                return qv_null();
            }
            auto& values = std::get<std::vector<int64_t>>(out.storage);
            const int64_t fill = q_to_i64_scalar(value);
            for (size_t i = 0; i < out.count; i++) {
                if (out.nulls.is_null[i] != 0) {
                    values[i] = fill;
                }
            }
            out.has_nulls = false;
            out.nulls.is_null.clear();
            return vec;
        }
        case QVector::Type::BOOL: {
            if (!q_is_boolish_scalar(value)) {
                return qv_null();
            }
            auto& values = std::get<std::vector<uint8_t>>(out.storage);
            const uint8_t fill = static_cast<uint8_t>((value.type == QValue::VAL_BOOL ? value.data.bool_val : (value.data.int_val != 0)) ? 1 : 0);
            for (size_t i = 0; i < out.count; i++) {
                if (out.nulls.is_null[i] != 0) {
                    values[i] = fill;
                }
            }
            out.has_nulls = false;
            out.nulls.is_null.clear();
            return vec;
        }
        case QVector::Type::STR: {
            if (value.type != QValue::VAL_STRING || value.data.string_val == nullptr) {
                return qv_null();
            }
            const auto& storage = std::get<QStringStorage>(out.storage);
            std::vector<std::string> values = q_vec_decode_strings(storage, out.count);
            const std::string fill(value.data.string_val);
            for (size_t i = 0; i < out.count; i++) {
                if (out.nulls.is_null[i] != 0) {
                    values[i] = fill;
                }
            }
            out.storage = q_vec_encode_strings(values);
            out.has_nulls = false;
            out.nulls.is_null.clear();
            return vec;
        }
        default:
            return qv_null();
    }
}

inline QValue q_astype(QValue vec, QValue dtype) {
    if (!q_vec_has_valid_handle(vec) || !q_vec_validate(*vec.data.vector_val)) {
        return qv_null();
    }
    if (dtype.type != QValue::VAL_STRING || dtype.data.string_val == nullptr) {
        return qv_null();
    }

    const QVector& src = *vec.data.vector_val;
    const char* target = dtype.data.string_val;

    if (std::strcmp(target, "f64") == 0) {
        if (src.type == QVector::Type::F64) {
            return q_vec_clone(vec);
        }
        QValue out = qv_vector(static_cast<int>(src.count));
        auto& outv = std::get<std::vector<double>>(out.data.vector_val->storage);
        outv.resize(src.count);
        out.data.vector_val->count = src.count;
        out.data.vector_val->has_nulls = src.has_nulls;
        out.data.vector_val->nulls = src.nulls;

        if (src.type == QVector::Type::I64) {
            const auto& in = std::get<std::vector<int64_t>>(src.storage);
            for (size_t i = 0; i < src.count; i++) outv[i] = static_cast<double>(in[i]);
            return out;
        }
        if (src.type == QVector::Type::BOOL) {
            const auto& in = std::get<std::vector<uint8_t>>(src.storage);
            for (size_t i = 0; i < src.count; i++) outv[i] = (in[i] != 0) ? 1.0 : 0.0;
            return out;
        }
        return qv_null();
    }

    if (std::strcmp(target, "i64") == 0) {
        if (src.type == QVector::Type::I64) {
            return q_vec_clone(vec);
        }
        QValue out = qv_vector_i64(static_cast<int>(src.count));
        auto& outv = std::get<std::vector<int64_t>>(out.data.vector_val->storage);
        outv.resize(src.count);
        out.data.vector_val->count = src.count;
        out.data.vector_val->has_nulls = src.has_nulls;
        out.data.vector_val->nulls = src.nulls;

        if (src.type == QVector::Type::F64) {
            const auto& in = std::get<std::vector<double>>(src.storage);
            for (size_t i = 0; i < src.count; i++) outv[i] = static_cast<int64_t>(in[i]);
            return out;
        }
        if (src.type == QVector::Type::BOOL) {
            const auto& in = std::get<std::vector<uint8_t>>(src.storage);
            for (size_t i = 0; i < src.count; i++) outv[i] = (in[i] != 0) ? 1 : 0;
            return out;
        }
        return qv_null();
    }

    if (std::strcmp(target, "bool") == 0) {
        if (src.type == QVector::Type::BOOL) {
            return q_vec_clone(vec);
        }
        QValue out = qv_vector_bool(static_cast<int>(src.count));
        auto& outv = std::get<std::vector<uint8_t>>(out.data.vector_val->storage);
        outv.resize(src.count);
        out.data.vector_val->count = src.count;
        out.data.vector_val->has_nulls = src.has_nulls;
        out.data.vector_val->nulls = src.nulls;

        if (src.type == QVector::Type::F64) {
            const auto& in = std::get<std::vector<double>>(src.storage);
            for (size_t i = 0; i < src.count; i++) outv[i] = static_cast<uint8_t>(in[i] != 0.0 ? 1 : 0);
            return out;
        }
        if (src.type == QVector::Type::I64) {
            const auto& in = std::get<std::vector<int64_t>>(src.storage);
            for (size_t i = 0; i < src.count; i++) outv[i] = static_cast<uint8_t>(in[i] != 0 ? 1 : 0);
            return out;
        }
        return qv_null();
    }

    return qv_null();
}

inline QValue q_to_vector(QValue input) {
    if (q_vec_has_valid_handle(input) && q_vec_validate(*input.data.vector_val)) {
        return q_vec_clone(input);
    }

    if (input.type != QValue::VAL_LIST || !input.data.list_val) {
        std::fprintf(stderr, "runtime error: to_vector expects list or vector input\n");
        return qv_null();
    }

    const QList& items = *input.data.list_val;
    const size_t n = items.size();

    enum class Mode { UNKNOWN, I64, F64, STR, INVALID };
    Mode mode = Mode::UNKNOWN;

    auto type_name = [](QValue::ValueType t) -> const char* {
        switch (t) {
            case QValue::VAL_INT: return "int";
            case QValue::VAL_FLOAT: return "float";
            case QValue::VAL_STRING: return "str";
            case QValue::VAL_BOOL: return "bool";
            case QValue::VAL_NULL: return "null";
            case QValue::VAL_LIST: return "list";
            case QValue::VAL_VECTOR: return "vector";
            case QValue::VAL_DICT: return "dict";
            case QValue::VAL_FUNC: return "func";
            case QValue::VAL_RESULT: return "result";
            default: return "unknown";
        }
    };

    for (size_t i = 0; i < n; i++) {
        const QValue& item = items[i];
        if (item.type == QValue::VAL_NULL) {
            continue;
        }

        switch (item.type) {
            case QValue::VAL_INT:
                if (mode == Mode::UNKNOWN) mode = Mode::I64;
                else if (mode != Mode::I64) mode = Mode::INVALID;
                break;
            case QValue::VAL_FLOAT:
                if (mode == Mode::UNKNOWN) mode = Mode::F64;
                else if (mode != Mode::F64) mode = Mode::INVALID;
                break;
                case QValue::VAL_STRING:
                    if (mode == Mode::UNKNOWN) mode = Mode::STR;
                    else if (mode != Mode::STR) mode = Mode::INVALID;
                    break;
            default:
                    std::fprintf(stderr, "runtime error: to_vector only supports int/float/str lists (null allowed), got %s at index %zu\n", type_name(item.type), i);
                mode = Mode::INVALID;
                break;
        }

        if (mode == Mode::INVALID) {
                if (item.type == QValue::VAL_INT || item.type == QValue::VAL_FLOAT || item.type == QValue::VAL_STRING) {
                    std::fprintf(stderr, "runtime error: to_vector requires homogeneous element types (all int, all float, or all str)\n");
            }
            return qv_null();
        }
    }

    if (mode == Mode::UNKNOWN) {
        mode = Mode::I64;
    }

    if (mode == Mode::F64) {
        QValue out = qv_vector(static_cast<int>(n));
        std::vector<double>& values = std::get<std::vector<double>>(out.data.vector_val->storage);
        values.resize(n, 0.0);
        out.data.vector_val->count = n;

        bool hasNulls = false;
        for (size_t i = 0; i < n; i++) {
            const QValue& item = items[i];
            if (item.type == QValue::VAL_NULL) {
                hasNulls = true;
                continue;
            }
            values[i] = item.data.float_val;
        }

        if (hasNulls) {
            q_vec_ensure_null_mask(*out.data.vector_val);
            for (size_t i = 0; i < n; i++) {
                if (items[i].type == QValue::VAL_NULL) {
                    out.data.vector_val->nulls.is_null[i] = 1;
                }
            }
        }
        return out;
    }

    if (mode == Mode::I64) {
        QValue out = qv_vector_i64(static_cast<int>(n));
        std::vector<int64_t>& values = std::get<std::vector<int64_t>>(out.data.vector_val->storage);
        values.resize(n, 0);
        out.data.vector_val->count = n;

        bool hasNulls = false;
        for (size_t i = 0; i < n; i++) {
            const QValue& item = items[i];
            if (item.type == QValue::VAL_NULL) {
                hasNulls = true;
                continue;
            }
            values[i] = static_cast<int64_t>(item.data.int_val);
        }

        if (hasNulls) {
            q_vec_ensure_null_mask(*out.data.vector_val);
            for (size_t i = 0; i < n; i++) {
                if (items[i].type == QValue::VAL_NULL) {
                    out.data.vector_val->nulls.is_null[i] = 1;
                }
            }
        }
        return out;
    }

    if (mode == Mode::STR) {
        std::vector<std::string> values(n);
        bool hasNulls = false;
        for (size_t i = 0; i < n; i++) {
            const QValue& item = items[i];
            if (item.type == QValue::VAL_NULL) {
                hasNulls = true;
                continue;
            }
            if (item.type != QValue::VAL_STRING || item.data.string_val == nullptr) {
                std::fprintf(stderr, "runtime error: to_vector requires homogeneous element types (all int, all float, or all str)\n");
                return qv_null();
            }
            values[i] = item.data.string_val;
        }

        QValue out = qv_vector_str(static_cast<int>(n), 0);
        out.data.vector_val->storage = q_vec_encode_strings(values);
        out.data.vector_val->count = n;

        if (hasNulls) {
            q_vec_ensure_null_mask(*out.data.vector_val);
            for (size_t i = 0; i < n; i++) {
                if (items[i].type == QValue::VAL_NULL) {
                    out.data.vector_val->nulls.is_null[i] = 1;
                }
            }
        }
        return out;
    }

    std::fprintf(stderr, "runtime error: to_vector could not determine output vector type\n");
    return qv_null();
}

// ============================================================
// Vector-to-List Conversion
// ============================================================

inline QValue q_to_list(QValue input) {
    // Identity: already a list
    if (input.type == QValue::VAL_LIST) {
        return input;
    }

    if (!q_vec_has_valid_handle(input)) {
        std::fprintf(stderr, "runtime error: to_list expects a vector or list input\n");
        return qv_null();
    }

    const QVector& v = *input.data.vector_val;
    const size_t n = v.count;
    QValue out = qv_list(static_cast<int>(n));
    QList& items = *out.data.list_val;
    items.reserve(n);

    for (size_t i = 0; i < n; i++) {
        // Check null mask
        if (v.has_nulls && i < v.nulls.is_null.size() && v.nulls.is_null[i]) {
            items.push_back(qv_null());
            continue;
        }

        switch (v.type) {
            case QVector::Type::I64: {
                const auto& vals = std::get<std::vector<int64_t>>(v.storage);
                items.push_back(qv_int(vals[i]));
                break;
            }
            case QVector::Type::F64: {
                const auto& vals = std::get<std::vector<double>>(v.storage);
                items.push_back(qv_float(vals[i]));
                break;
            }
            case QVector::Type::BOOL: {
                const auto& vals = std::get<std::vector<uint8_t>>(v.storage);
                items.push_back(qv_bool(vals[i] != 0));
                break;
            }
            case QVector::Type::STR: {
                auto strs = q_vec_decode_strings(std::get<QStringStorage>(v.storage), n);
                // Bulk add remaining strings from this point
                for (size_t j = i; j < n; j++) {
                    if (v.has_nulls && j < v.nulls.is_null.size() && v.nulls.is_null[j]) {
                        items.push_back(qv_null());
                    } else {
                        items.push_back(qv_string(q_strdup(strs[j].c_str())));
                    }
                }
                return out;
            }
        }
    }
    return out;
}

// ============================================================
// Vector Comparison Operations (output BOOL vectors)
// ============================================================

// Helper: build a BOOL output vector for a comparison result of size n.
// If either input has nulls, allocates the null mask on the output.
inline QValue q_vec_cmp_alloc_bool(size_t n, bool hasNulls) {
    QValue out = qv_vector_bool(static_cast<int>(n));
    auto& outv = std::get<std::vector<uint8_t>>(out.data.vector_val->storage);
    outv.resize(n, 0);
    out.data.vector_val->count = n;
    if (hasNulls) {
        q_vec_ensure_null_mask(*out.data.vector_val);
    }
    return out;
}

// F64 comparison template: vec-vec, vec-scalar, scalar-vec → BOOL vector
template <typename CmpOp>
inline QValue q_vec_cmp_f64_impl(QValue a, QValue b, CmpOp op) {
    const bool aVec = q_vec_has_valid_handle(a);
    const bool bVec = q_vec_has_valid_handle(b);
    if (!aVec && !bVec) return qv_null();

    // vec-vec
    if (aVec && bVec) {
        const auto* avp = q_vec_f64_const(a);
        const auto* bvp = q_vec_f64_const(b);
        if (!avp || !bvp) return qv_null();
        const auto& av = *avp;
        const auto& bv = *bvp;
        if (av.size() != bv.size()) return qv_null();
        const size_t n = av.size();
        const bool aNull = a.data.vector_val->has_nulls;
        const bool bNull = b.data.vector_val->has_nulls;
        QValue out = q_vec_cmp_alloc_bool(n, aNull || bNull);
        auto& outv = std::get<std::vector<uint8_t>>(out.data.vector_val->storage);
        if (!aNull && !bNull) {
            for (size_t i = 0; i < n; i++) {
                outv[i] = static_cast<uint8_t>(op(av[i], bv[i]) ? 1 : 0);
            }
        } else {
            auto& outNulls = out.data.vector_val->nulls.is_null;
            for (size_t i = 0; i < n; i++) {
                if (q_vec_is_null_at(*a.data.vector_val, i) || q_vec_is_null_at(*b.data.vector_val, i)) {
                    outNulls[i] = 1;
                } else {
                    outv[i] = static_cast<uint8_t>(op(av[i], bv[i]) ? 1 : 0);
                }
            }
        }
        return out;
    }

    // vec-scalar
    if (aVec && q_is_numeric_scalar(b)) {
        const auto* avp = q_vec_f64_const(a);
        if (!avp) return qv_null();
        const auto& av = *avp;
        const double bs = q_to_double_scalar(b);
        const size_t n = av.size();
        const bool aNull = a.data.vector_val->has_nulls;
        QValue out = q_vec_cmp_alloc_bool(n, aNull);
        auto& outv = std::get<std::vector<uint8_t>>(out.data.vector_val->storage);
        if (!aNull) {
            for (size_t i = 0; i < n; i++) {
                outv[i] = static_cast<uint8_t>(op(av[i], bs) ? 1 : 0);
            }
        } else {
            auto& outNulls = out.data.vector_val->nulls.is_null;
            for (size_t i = 0; i < n; i++) {
                if (q_vec_is_null_at(*a.data.vector_val, i)) {
                    outNulls[i] = 1;
                } else {
                    outv[i] = static_cast<uint8_t>(op(av[i], bs) ? 1 : 0);
                }
            }
        }
        return out;
    }

    // scalar-vec
    if (bVec && q_is_numeric_scalar(a)) {
        const auto* bvp = q_vec_f64_const(b);
        if (!bvp) return qv_null();
        const auto& bv = *bvp;
        const double as = q_to_double_scalar(a);
        const size_t n = bv.size();
        const bool bNull = b.data.vector_val->has_nulls;
        QValue out = q_vec_cmp_alloc_bool(n, bNull);
        auto& outv = std::get<std::vector<uint8_t>>(out.data.vector_val->storage);
        if (!bNull) {
            for (size_t i = 0; i < n; i++) {
                outv[i] = static_cast<uint8_t>(op(as, bv[i]) ? 1 : 0);
            }
        } else {
            auto& outNulls = out.data.vector_val->nulls.is_null;
            for (size_t i = 0; i < n; i++) {
                if (q_vec_is_null_at(*b.data.vector_val, i)) {
                    outNulls[i] = 1;
                } else {
                    outv[i] = static_cast<uint8_t>(op(as, bv[i]) ? 1 : 0);
                }
            }
        }
        return out;
    }

    return qv_null();
}

// I64 comparison template: vec-vec, vec-scalar, scalar-vec → BOOL vector
template <typename CmpOp>
inline QValue q_vec_cmp_i64_impl(QValue a, QValue b, CmpOp op) {
    const bool aVec = q_vec_has_valid_handle(a);
    const bool bVec = q_vec_has_valid_handle(b);

    // vec-vec
    if (aVec && bVec) {
        const auto* avp = q_vec_i64_const(a);
        const auto* bvp = q_vec_i64_const(b);
        if (!avp || !bvp || avp->size() != bvp->size()) return qv_null();
        const size_t n = avp->size();
        const bool aNull = a.data.vector_val->has_nulls;
        const bool bNull = b.data.vector_val->has_nulls;
        QValue out = q_vec_cmp_alloc_bool(n, aNull || bNull);
        auto& outv = std::get<std::vector<uint8_t>>(out.data.vector_val->storage);
        if (!aNull && !bNull) {
            for (size_t i = 0; i < n; i++) {
                outv[i] = static_cast<uint8_t>(op((*avp)[i], (*bvp)[i]) ? 1 : 0);
            }
        } else {
            auto& outNulls = out.data.vector_val->nulls.is_null;
            for (size_t i = 0; i < n; i++) {
                if (q_vec_is_null_at(*a.data.vector_val, i) || q_vec_is_null_at(*b.data.vector_val, i)) {
                    outNulls[i] = 1;
                } else {
                    outv[i] = static_cast<uint8_t>(op((*avp)[i], (*bvp)[i]) ? 1 : 0);
                }
            }
        }
        return out;
    }

    // vec-scalar
    if (aVec && q_is_integral_scalar(b)) {
        const auto* avp = q_vec_i64_const(a);
        if (!avp) return qv_null();
        const int64_t bs = q_to_i64_scalar(b);
        const size_t n = avp->size();
        const bool aNull = a.data.vector_val->has_nulls;
        QValue out = q_vec_cmp_alloc_bool(n, aNull);
        auto& outv = std::get<std::vector<uint8_t>>(out.data.vector_val->storage);
        if (!aNull) {
            for (size_t i = 0; i < n; i++) {
                outv[i] = static_cast<uint8_t>(op((*avp)[i], bs) ? 1 : 0);
            }
        } else {
            auto& outNulls = out.data.vector_val->nulls.is_null;
            for (size_t i = 0; i < n; i++) {
                if (q_vec_is_null_at(*a.data.vector_val, i)) {
                    outNulls[i] = 1;
                } else {
                    outv[i] = static_cast<uint8_t>(op((*avp)[i], bs) ? 1 : 0);
                }
            }
        }
        return out;
    }

    // scalar-vec
    if (bVec && q_is_integral_scalar(a)) {
        const auto* bvp = q_vec_i64_const(b);
        if (!bvp) return qv_null();
        const int64_t as = q_to_i64_scalar(a);
        const size_t n = bvp->size();
        const bool bNull = b.data.vector_val->has_nulls;
        QValue out = q_vec_cmp_alloc_bool(n, bNull);
        auto& outv = std::get<std::vector<uint8_t>>(out.data.vector_val->storage);
        if (!bNull) {
            for (size_t i = 0; i < n; i++) {
                outv[i] = static_cast<uint8_t>(op(as, (*bvp)[i]) ? 1 : 0);
            }
        } else {
            auto& outNulls = out.data.vector_val->nulls.is_null;
            for (size_t i = 0; i < n; i++) {
                if (q_vec_is_null_at(*b.data.vector_val, i)) {
                    outNulls[i] = 1;
                } else {
                    outv[i] = static_cast<uint8_t>(op(as, (*bvp)[i]) ? 1 : 0);
                }
            }
        }
        return out;
    }

    return qv_null();
}

// BOOL comparison template: vec-vec or vec-scalar → BOOL vector (eq/neq only)
template <typename CmpOp>
inline QValue q_vec_cmp_bool_impl(QValue a, QValue b, CmpOp op) {
    const bool aVec = q_vec_is_type(a, QVector::Type::BOOL);
    const bool bVec = q_vec_is_type(b, QVector::Type::BOOL);

    if (aVec && bVec) {
        const auto& av = *q_vec_bool_const(a);
        const auto& bv = *q_vec_bool_const(b);
        if (av.size() != bv.size()) return qv_null();
        const size_t n = av.size();
        const bool aNull = a.data.vector_val->has_nulls;
        const bool bNull = b.data.vector_val->has_nulls;
        QValue out = q_vec_cmp_alloc_bool(n, aNull || bNull);
        auto& outv = std::get<std::vector<uint8_t>>(out.data.vector_val->storage);
        if (!aNull && !bNull) {
            for (size_t i = 0; i < n; i++) {
                outv[i] = static_cast<uint8_t>(op(av[i] != 0, bv[i] != 0) ? 1 : 0);
            }
        } else {
            auto& outNulls = out.data.vector_val->nulls.is_null;
            for (size_t i = 0; i < n; i++) {
                if (q_vec_is_null_at(*a.data.vector_val, i) || q_vec_is_null_at(*b.data.vector_val, i)) {
                    outNulls[i] = 1;
                } else {
                    outv[i] = static_cast<uint8_t>(op(av[i] != 0, bv[i] != 0) ? 1 : 0);
                }
            }
        }
        return out;
    }

    // BOOL vec vs bool scalar
    if (aVec && (b.type == QValue::VAL_BOOL || b.type == QValue::VAL_INT)) {
        const auto& av = *q_vec_bool_const(a);
        const bool bs = (b.type == QValue::VAL_BOOL) ? b.data.bool_val : (b.data.int_val != 0);
        const size_t n = av.size();
        const bool aNull = a.data.vector_val->has_nulls;
        QValue out = q_vec_cmp_alloc_bool(n, aNull);
        auto& outv = std::get<std::vector<uint8_t>>(out.data.vector_val->storage);
        if (!aNull) {
            for (size_t i = 0; i < n; i++) {
                outv[i] = static_cast<uint8_t>(op(av[i] != 0, bs) ? 1 : 0);
            }
        } else {
            auto& outNulls = out.data.vector_val->nulls.is_null;
            for (size_t i = 0; i < n; i++) {
                if (q_vec_is_null_at(*a.data.vector_val, i)) {
                    outNulls[i] = 1;
                } else {
                    outv[i] = static_cast<uint8_t>(op(av[i] != 0, bs) ? 1 : 0);
                }
            }
        }
        return out;
    }

    // bool scalar vs BOOL vec
    if (bVec && (a.type == QValue::VAL_BOOL || a.type == QValue::VAL_INT)) {
        const auto& bv = *q_vec_bool_const(b);
        const bool as = (a.type == QValue::VAL_BOOL) ? a.data.bool_val : (a.data.int_val != 0);
        const size_t n = bv.size();
        const bool bNull = b.data.vector_val->has_nulls;
        QValue out = q_vec_cmp_alloc_bool(n, bNull);
        auto& outv = std::get<std::vector<uint8_t>>(out.data.vector_val->storage);
        if (!bNull) {
            for (size_t i = 0; i < n; i++) {
                outv[i] = static_cast<uint8_t>(op(as, bv[i] != 0) ? 1 : 0);
            }
        } else {
            auto& outNulls = out.data.vector_val->nulls.is_null;
            for (size_t i = 0; i < n; i++) {
                if (q_vec_is_null_at(*b.data.vector_val, i)) {
                    outNulls[i] = 1;
                } else {
                    outv[i] = static_cast<uint8_t>(op(as, bv[i] != 0) ? 1 : 0);
                }
            }
        }
        return out;
    }

    return qv_null();
}

// STR equality: vec-vec or vec-scalar → BOOL vector
inline QValue q_vec_cmp_str_eq(QValue a, QValue b, bool negate) {
    const bool aStr = q_vec_is_type(a, QVector::Type::STR);
    const bool bStr = q_vec_is_type(b, QVector::Type::STR);

    // STR vec vs STR vec
    if (aStr && bStr) {
        const QVector& av = *a.data.vector_val;
        const QVector& bv = *b.data.vector_val;
        if (av.count != bv.count) return qv_null();
        const size_t n = av.count;
        auto aStrs = q_vec_decode_strings(std::get<QStringStorage>(av.storage), n);
        auto bStrs = q_vec_decode_strings(std::get<QStringStorage>(bv.storage), n);
        const bool aNull = av.has_nulls;
        const bool bNull = bv.has_nulls;
        QValue out = q_vec_cmp_alloc_bool(n, aNull || bNull);
        auto& outv = std::get<std::vector<uint8_t>>(out.data.vector_val->storage);
        if (!aNull && !bNull) {
            for (size_t i = 0; i < n; i++) {
                bool eq = (aStrs[i] == bStrs[i]);
                outv[i] = static_cast<uint8_t>((negate ? !eq : eq) ? 1 : 0);
            }
        } else {
            auto& outNulls = out.data.vector_val->nulls.is_null;
            for (size_t i = 0; i < n; i++) {
                if (q_vec_is_null_at(av, i) || q_vec_is_null_at(bv, i)) {
                    outNulls[i] = 1;
                } else {
                    bool eq = (aStrs[i] == bStrs[i]);
                    outv[i] = static_cast<uint8_t>((negate ? !eq : eq) ? 1 : 0);
                }
            }
        }
        return out;
    }

    // STR vec vs scalar string
    if (aStr && b.type == QValue::VAL_STRING && b.data.string_val) {
        const QVector& av = *a.data.vector_val;
        const size_t n = av.count;
        auto aStrs = q_vec_decode_strings(std::get<QStringStorage>(av.storage), n);
        const std::string scalar(b.data.string_val);
        const bool aNull = av.has_nulls;
        QValue out = q_vec_cmp_alloc_bool(n, aNull);
        auto& outv = std::get<std::vector<uint8_t>>(out.data.vector_val->storage);
        if (!aNull) {
            for (size_t i = 0; i < n; i++) {
                bool eq = (aStrs[i] == scalar);
                outv[i] = static_cast<uint8_t>((negate ? !eq : eq) ? 1 : 0);
            }
        } else {
            auto& outNulls = out.data.vector_val->nulls.is_null;
            for (size_t i = 0; i < n; i++) {
                if (q_vec_is_null_at(av, i)) {
                    outNulls[i] = 1;
                } else {
                    bool eq = (aStrs[i] == scalar);
                    outv[i] = static_cast<uint8_t>((negate ? !eq : eq) ? 1 : 0);
                }
            }
        }
        return out;
    }

    // scalar string vs STR vec
    if (bStr && a.type == QValue::VAL_STRING && a.data.string_val) {
        // Swap and reuse: eq is symmetric, neq is symmetric
        return q_vec_cmp_str_eq(b, a, negate);
    }

    return qv_null();
}

// ---- Dispatch functions (called from comparison.hpp) ----

inline QValue q_vec_lt(QValue a, QValue b) {
    if (q_vec_is_type(a, QVector::Type::I64) || q_vec_is_type(b, QVector::Type::I64)) {
        QValue out = q_vec_cmp_i64_impl(a, b, [](int64_t x, int64_t y) { return x < y; });
        if (out.type != QValue::VAL_NULL) return out;
    }
    QValue out = q_vec_cmp_f64_impl(a, b, [](double x, double y) { return x < y; });
    if (out.type != QValue::VAL_NULL) return out;
    std::fprintf(stderr, "runtime error: operator '<' not supported for these vector types\n");
    return qv_null();
}

inline QValue q_vec_lte(QValue a, QValue b) {
    if (q_vec_is_type(a, QVector::Type::I64) || q_vec_is_type(b, QVector::Type::I64)) {
        QValue out = q_vec_cmp_i64_impl(a, b, [](int64_t x, int64_t y) { return x <= y; });
        if (out.type != QValue::VAL_NULL) return out;
    }
    QValue out = q_vec_cmp_f64_impl(a, b, [](double x, double y) { return x <= y; });
    if (out.type != QValue::VAL_NULL) return out;
    std::fprintf(stderr, "runtime error: operator '<=' not supported for these vector types\n");
    return qv_null();
}

inline QValue q_vec_gt(QValue a, QValue b) {
    if (q_vec_is_type(a, QVector::Type::I64) || q_vec_is_type(b, QVector::Type::I64)) {
        QValue out = q_vec_cmp_i64_impl(a, b, [](int64_t x, int64_t y) { return x > y; });
        if (out.type != QValue::VAL_NULL) return out;
    }
    QValue out = q_vec_cmp_f64_impl(a, b, [](double x, double y) { return x > y; });
    if (out.type != QValue::VAL_NULL) return out;
    std::fprintf(stderr, "runtime error: operator '>' not supported for these vector types\n");
    return qv_null();
}

inline QValue q_vec_gte(QValue a, QValue b) {
    if (q_vec_is_type(a, QVector::Type::I64) || q_vec_is_type(b, QVector::Type::I64)) {
        QValue out = q_vec_cmp_i64_impl(a, b, [](int64_t x, int64_t y) { return x >= y; });
        if (out.type != QValue::VAL_NULL) return out;
    }
    QValue out = q_vec_cmp_f64_impl(a, b, [](double x, double y) { return x >= y; });
    if (out.type != QValue::VAL_NULL) return out;
    std::fprintf(stderr, "runtime error: operator '>=' not supported for these vector types\n");
    return qv_null();
}

inline QValue q_vec_eq(QValue a, QValue b) {
    // Try numeric paths first
    if (q_vec_is_type(a, QVector::Type::I64) || q_vec_is_type(b, QVector::Type::I64)) {
        QValue out = q_vec_cmp_i64_impl(a, b, [](int64_t x, int64_t y) { return x == y; });
        if (out.type != QValue::VAL_NULL) return out;
    }
    {
        QValue out = q_vec_cmp_f64_impl(a, b, [](double x, double y) { return x == y; });
        if (out.type != QValue::VAL_NULL) return out;
    }
    // BOOL equality
    {
        QValue out = q_vec_cmp_bool_impl(a, b, [](bool x, bool y) { return x == y; });
        if (out.type != QValue::VAL_NULL) return out;
    }
    // STR equality
    {
        QValue out = q_vec_cmp_str_eq(a, b, false);
        if (out.type != QValue::VAL_NULL) return out;
    }
    std::fprintf(stderr, "runtime error: operator '==' not supported for these vector types\n");
    return qv_null();
}

inline QValue q_vec_neq(QValue a, QValue b) {
    // Try numeric paths first
    if (q_vec_is_type(a, QVector::Type::I64) || q_vec_is_type(b, QVector::Type::I64)) {
        QValue out = q_vec_cmp_i64_impl(a, b, [](int64_t x, int64_t y) { return x != y; });
        if (out.type != QValue::VAL_NULL) return out;
    }
    {
        QValue out = q_vec_cmp_f64_impl(a, b, [](double x, double y) { return x != y; });
        if (out.type != QValue::VAL_NULL) return out;
    }
    // BOOL inequality
    {
        QValue out = q_vec_cmp_bool_impl(a, b, [](bool x, bool y) { return x != y; });
        if (out.type != QValue::VAL_NULL) return out;
    }
    // STR inequality
    {
        QValue out = q_vec_cmp_str_eq(a, b, true);
        if (out.type != QValue::VAL_NULL) return out;
    }
    std::fprintf(stderr, "runtime error: operator '!=' not supported for these vector types\n");
    return qv_null();
}

// ============================================================
// Vector Scalar Indexing & Boolean Mask Filtering
// ============================================================

// Scalar integer index on a vector: vec[i] → boxed QValue
inline QValue q_vec_get_scalar(QValue vec, QValue index) {
    if (!q_vec_has_valid_handle(vec)) return qv_null();
    if (index.type != QValue::VAL_INT) return qv_null();

    const QVector& v = *vec.data.vector_val;
    int idx = static_cast<int>(index.data.int_val);
    int len = static_cast<int>(v.count);
    if (idx < 0) idx = len + idx;
    if (idx < 0 || idx >= len) return qv_null();
    const size_t i = static_cast<size_t>(idx);

    if (q_vec_is_null_at(v, i)) return qv_null();

    switch (v.type) {
        case QVector::Type::F64:
            return qv_float(std::get<std::vector<double>>(v.storage)[i]);
        case QVector::Type::I64:
            return qv_int(std::get<std::vector<int64_t>>(v.storage)[i]);
        case QVector::Type::BOOL:
            return qv_bool(std::get<std::vector<uint8_t>>(v.storage)[i] != 0);
        case QVector::Type::STR: {
            const auto& s = std::get<QStringStorage>(v.storage);
            uint32_t start = s.offsets[i];
            uint32_t end = s.offsets[i + 1];
            std::string elem(s.bytes.data() + start, s.bytes.data() + end);
            return qv_string(elem.c_str());
        }
        default:
            return qv_null();
    }
}

// Boolean mask filter: data[mask] → new vector with matching elements
inline QValue q_vec_mask_filter(QValue data, QValue mask) {
    if (!q_vec_has_valid_handle(data) || !q_vec_has_valid_handle(mask)) {
        return qv_null();
    }
    const QVector& dv = *data.data.vector_val;
    const QVector& mv = *mask.data.vector_val;

    if (mv.type != QVector::Type::BOOL) {
        std::fprintf(stderr, "runtime error: mask index must be a bool vector, got vector[%s]\n",
                     q_vec_dtype_name(mv));
        return qv_null();
    }
    if (dv.count != mv.count) {
        std::fprintf(stderr, "runtime error: mask length (%zu) does not match vector length (%zu)\n",
                     mv.count, dv.count);
        return qv_null();
    }

    const auto& maskBits = std::get<std::vector<uint8_t>>(mv.storage);
    const size_t n = dv.count;

    // Count selected elements (mask=1 and mask not null)
    size_t selected = 0;
    for (size_t i = 0; i < n; i++) {
        if (!q_vec_is_null_at(mv, i) && maskBits[i] != 0) selected++;
    }

    switch (dv.type) {
        case QVector::Type::F64: {
            QValue out = qv_vector(static_cast<int>(selected));
            auto& outv = std::get<std::vector<double>>(out.data.vector_val->storage);
            outv.reserve(selected);
            out.data.vector_val->count = 0;
            bool hasNulls = false;
            for (size_t i = 0; i < n; i++) {
                if (q_vec_is_null_at(mv, i) || maskBits[i] == 0) continue;
                outv.push_back(std::get<std::vector<double>>(dv.storage)[i]);
                if (q_vec_is_null_at(dv, i)) hasNulls = true;
            }
            out.data.vector_val->count = outv.size();
            if (hasNulls) {
                q_vec_ensure_null_mask(*out.data.vector_val);
                size_t j = 0;
                for (size_t i = 0; i < n; i++) {
                    if (q_vec_is_null_at(mv, i) || maskBits[i] == 0) continue;
                    if (q_vec_is_null_at(dv, i)) {
                        out.data.vector_val->nulls.is_null[j] = 1;
                    }
                    j++;
                }
            }
            return out;
        }
        case QVector::Type::I64: {
            QValue out = qv_vector_i64(static_cast<int>(selected));
            auto& outv = std::get<std::vector<int64_t>>(out.data.vector_val->storage);
            outv.reserve(selected);
            out.data.vector_val->count = 0;
            bool hasNulls = false;
            for (size_t i = 0; i < n; i++) {
                if (q_vec_is_null_at(mv, i) || maskBits[i] == 0) continue;
                outv.push_back(std::get<std::vector<int64_t>>(dv.storage)[i]);
                if (q_vec_is_null_at(dv, i)) hasNulls = true;
            }
            out.data.vector_val->count = outv.size();
            if (hasNulls) {
                q_vec_ensure_null_mask(*out.data.vector_val);
                size_t j = 0;
                for (size_t i = 0; i < n; i++) {
                    if (q_vec_is_null_at(mv, i) || maskBits[i] == 0) continue;
                    if (q_vec_is_null_at(dv, i)) {
                        out.data.vector_val->nulls.is_null[j] = 1;
                    }
                    j++;
                }
            }
            return out;
        }
        case QVector::Type::BOOL: {
            QValue out = qv_vector_bool(static_cast<int>(selected));
            auto& outv = std::get<std::vector<uint8_t>>(out.data.vector_val->storage);
            outv.reserve(selected);
            out.data.vector_val->count = 0;
            bool hasNulls = false;
            for (size_t i = 0; i < n; i++) {
                if (q_vec_is_null_at(mv, i) || maskBits[i] == 0) continue;
                outv.push_back(std::get<std::vector<uint8_t>>(dv.storage)[i]);
                if (q_vec_is_null_at(dv, i)) hasNulls = true;
            }
            out.data.vector_val->count = outv.size();
            if (hasNulls) {
                q_vec_ensure_null_mask(*out.data.vector_val);
                size_t j = 0;
                for (size_t i = 0; i < n; i++) {
                    if (q_vec_is_null_at(mv, i) || maskBits[i] == 0) continue;
                    if (q_vec_is_null_at(dv, i)) {
                        out.data.vector_val->nulls.is_null[j] = 1;
                    }
                    j++;
                }
            }
            return out;
        }
        case QVector::Type::STR: {
            // Decode, filter, re-encode
            auto strs = q_vec_decode_strings(std::get<QStringStorage>(dv.storage), n);
            std::vector<std::string> filtered;
            filtered.reserve(selected);
            std::vector<uint8_t> filteredNulls;
            bool hasNulls = false;
            for (size_t i = 0; i < n; i++) {
                if (q_vec_is_null_at(mv, i) || maskBits[i] == 0) continue;
                filtered.push_back(strs[i]);
                bool isNull = q_vec_is_null_at(dv, i);
                filteredNulls.push_back(isNull ? 1 : 0);
                if (isNull) hasNulls = true;
            }
            QValue out = qv_vector_str(static_cast<int>(filtered.size()), 0);
            out.data.vector_val->storage = q_vec_encode_strings(filtered);
            out.data.vector_val->count = filtered.size();
            if (hasNulls) {
                out.data.vector_val->has_nulls = true;
                out.data.vector_val->nulls.is_null = filteredNulls;
            }
            return out;
        }
        default:
            return qv_null();
    }
}

#endif // QUARK_TYPES_VECTOR_HPP
