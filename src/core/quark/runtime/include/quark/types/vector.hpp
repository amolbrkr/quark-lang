// quark/types/vector.hpp - 1D float64 vector with optional xsimd kernels
#ifndef QUARK_TYPES_VECTOR_HPP
#define QUARK_TYPES_VECTOR_HPP

#include "../core/value.hpp"
#include "../core/constructors.hpp"

#include <algorithm>
#include <vector>

#if defined(__has_include)
  #if __has_include(<xsimd/xsimd.hpp>)
    #include <xsimd/xsimd.hpp>
    #define QUARK_HAS_XSIMD 1
  #endif
#endif

#ifndef QUARK_HAS_XSIMD
#define QUARK_HAS_XSIMD 0
#endif

struct QVector {
    std::vector<double> data;
};

inline bool q_is_numeric_scalar(QValue v) {
    return v.type == QValue::VAL_INT || v.type == QValue::VAL_FLOAT;
}

inline double q_to_double_scalar(QValue v) {
    return v.type == QValue::VAL_FLOAT ? v.data.float_val : static_cast<double>(v.data.int_val);
}

inline QValue qv_vector(int initial_cap = 0) {
    QValue q;
    q.type = QValue::VAL_VECTOR;
    q.data.vector_val = new QVector();
    if (initial_cap > 0) {
        q.data.vector_val->data.reserve(static_cast<size_t>(initial_cap));
    }
    return q;
}

inline QValue q_vec_push(QValue vec, QValue value) {
    if (vec.type != QValue::VAL_VECTOR || !vec.data.vector_val) {
        return qv_null();
    }
    if (!q_is_numeric_scalar(value)) {
        return qv_null();
    }
    vec.data.vector_val->data.push_back(q_to_double_scalar(value));
    return vec;
}

inline int q_vec_size(QValue vec) {
    if (vec.type != QValue::VAL_VECTOR || !vec.data.vector_val) {
        return 0;
    }
    return static_cast<int>(vec.data.vector_val->data.size());
}

template <typename BinaryOp>
inline QValue q_vec_binary_impl(QValue a, QValue b, BinaryOp op) {
    const bool aVec = a.type == QValue::VAL_VECTOR && a.data.vector_val;
    const bool bVec = b.type == QValue::VAL_VECTOR && b.data.vector_val;

    if (!aVec && !bVec) {
        return qv_null();
    }

    if (aVec && bVec) {
        const std::vector<double>& av = a.data.vector_val->data;
        const std::vector<double>& bv = b.data.vector_val->data;
        if (av.size() != bv.size()) {
            return qv_null();
        }
        QValue out = qv_vector(static_cast<int>(av.size()));
        out.data.vector_val->data.resize(av.size());

        size_t i = 0;
#if QUARK_HAS_XSIMD
        using batch = xsimd::batch<double>;
        constexpr size_t lanes = batch::size;
        for (; i + lanes <= av.size(); i += lanes) {
            batch va = batch::load_unaligned(&av[i]);
            batch vb = batch::load_unaligned(&bv[i]);
            batch vr = op(va, vb);
            vr.store_unaligned(&out.data.vector_val->data[i]);
        }
#endif
        for (; i < av.size(); i++) {
            out.data.vector_val->data[i] = op(av[i], bv[i]);
        }
        return out;
    }

    if (aVec && q_is_numeric_scalar(b)) {
        const std::vector<double>& av = a.data.vector_val->data;
        double bs = q_to_double_scalar(b);
        QValue out = qv_vector(static_cast<int>(av.size()));
        out.data.vector_val->data.resize(av.size());
        size_t i = 0;
#if QUARK_HAS_XSIMD
        using batch = xsimd::batch<double>;
        constexpr size_t lanes = batch::size;
        batch vb(bs);
        for (; i + lanes <= av.size(); i += lanes) {
            batch va = batch::load_unaligned(&av[i]);
            batch vr = op(va, vb);
            vr.store_unaligned(&out.data.vector_val->data[i]);
        }
#endif
        for (; i < av.size(); i++) {
            out.data.vector_val->data[i] = op(av[i], bs);
        }
        return out;
    }

    if (bVec && q_is_numeric_scalar(a)) {
        const std::vector<double>& bv = b.data.vector_val->data;
        double as = q_to_double_scalar(a);
        QValue out = qv_vector(static_cast<int>(bv.size()));
        out.data.vector_val->data.resize(bv.size());
        size_t i = 0;
#if QUARK_HAS_XSIMD
        using batch = xsimd::batch<double>;
        constexpr size_t lanes = batch::size;
        batch va(as);
        for (; i + lanes <= bv.size(); i += lanes) {
            batch vb = batch::load_unaligned(&bv[i]);
            batch vr = op(va, vb);
            vr.store_unaligned(&out.data.vector_val->data[i]);
        }
#endif
        for (; i < bv.size(); i++) {
            out.data.vector_val->data[i] = op(as, bv[i]);
        }
        return out;
    }

    return qv_null();
}

inline QValue q_vec_add(QValue a, QValue b) {
    return q_vec_binary_impl(a, b, [](auto x, auto y) { return x + y; });
}

inline QValue q_vadd_inplace(QValue vec, QValue scalar) {
    if (vec.type != QValue::VAL_VECTOR || !vec.data.vector_val) {
        return qv_null();
    }
    if (!q_is_numeric_scalar(scalar)) {
        return qv_null();
    }

    std::vector<double>& v = vec.data.vector_val->data;
    const double s = q_to_double_scalar(scalar);

    size_t i = 0;
#if QUARK_HAS_XSIMD
    using batch = xsimd::batch<double>;
    constexpr size_t lanes = batch::size;
    batch vs(s);
    for (; i + lanes <= v.size(); i += lanes) {
        batch vb = batch::load_unaligned(&v[i]);
        (vb + vs).store_unaligned(&v[i]);
    }
#endif
    for (; i < v.size(); i++) {
        v[i] += s;
    }

    return vec;
}

inline QValue q_vec_sub(QValue a, QValue b) {
    return q_vec_binary_impl(a, b, [](auto x, auto y) { return x - y; });
}

inline QValue q_vec_mul(QValue a, QValue b) {
    return q_vec_binary_impl(a, b, [](auto x, auto y) { return x * y; });
}

inline QValue q_vec_div(QValue a, QValue b) {
    // Keep behavior simple for MVP: division by zero propagates IEEE inf/nan in vector kernels.
    return q_vec_binary_impl(a, b, [](auto x, auto y) { return x / y; });
}

inline QValue q_vec_sum(QValue vec) {
    if (vec.type != QValue::VAL_VECTOR || !vec.data.vector_val) {
        return qv_null();
    }
    const std::vector<double>& v = vec.data.vector_val->data;
    double acc = 0.0;
    size_t i = 0;
#if QUARK_HAS_XSIMD
    using batch = xsimd::batch<double>;
    constexpr size_t lanes = batch::size;
    batch vacc(0.0);
    for (; i + lanes <= v.size(); i += lanes) {
        batch vb = batch::load_unaligned(&v[i]);
        vacc += vb;
    }
    acc += xsimd::reduce_add(vacc);
#endif
    for (; i < v.size(); i++) {
        acc += v[i];
    }
    return qv_float(acc);
}

inline QValue q_vec_min(QValue vec) {
    if (vec.type != QValue::VAL_VECTOR || !vec.data.vector_val || vec.data.vector_val->data.empty()) {
        return qv_null();
    }
    const std::vector<double>& v = vec.data.vector_val->data;
    double cur = v[0];
    for (size_t i = 1; i < v.size(); i++) {
        cur = std::min(cur, v[i]);
    }
    return qv_float(cur);
}

inline QValue q_vec_max(QValue vec) {
    if (vec.type != QValue::VAL_VECTOR || !vec.data.vector_val || vec.data.vector_val->data.empty()) {
        return qv_null();
    }
    const std::vector<double>& v = vec.data.vector_val->data;
    double cur = v[0];
    for (size_t i = 1; i < v.size(); i++) {
        cur = std::max(cur, v[i]);
    }
    return qv_float(cur);
}

#endif // QUARK_TYPES_VECTOR_HPP
