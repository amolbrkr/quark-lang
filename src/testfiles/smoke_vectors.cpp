#include "quark/quark.hpp"

// Forward declarations


int main() {
    q_gc_init();
    QValue _t1 = qv_vector(5);
    _t1 = q_vec_push(_t1, qv_int(10));
    _t1 = q_vec_push(_t1, qv_int(20));
    _t1 = q_vec_push(_t1, qv_int(30));
    _t1 = q_vec_push(_t1, qv_int(40));
    _t1 = q_vec_push(_t1, qv_int(50));
    QValue quark_v = _t1;
    quark_v;
    QValue quark_mask = q_gt(quark_v, qv_int(25));
    quark_mask;
    q_println(quark_mask);
    q_println(q_sum(quark_mask));
    QValue quark_mask2 = q_lt(quark_v, qv_int(30));
    quark_mask2;
    q_println(q_sum(quark_mask2));
    QValue quark_mask3 = q_gte(quark_v, qv_int(30));
    quark_mask3;
    q_println(q_sum(quark_mask3));
    QValue quark_mask4 = q_lte(quark_v, qv_int(30));
    quark_mask4;
    q_println(q_sum(quark_mask4));
    QValue quark_eq_mask = q_eq(quark_v, qv_int(30));
    quark_eq_mask;
    q_println(q_sum(quark_eq_mask));
    QValue quark_neq_mask = q_neq(quark_v, qv_int(30));
    quark_neq_mask;
    q_println(q_sum(quark_neq_mask));
    QValue _t2 = qv_vector(5);
    _t2 = q_vec_push(_t2, qv_int(10));
    _t2 = q_vec_push(_t2, qv_int(25));
    _t2 = q_vec_push(_t2, qv_int(30));
    _t2 = q_vec_push(_t2, qv_int(35));
    _t2 = q_vec_push(_t2, qv_int(50));
    QValue quark_w = _t2;
    quark_w;
    QValue quark_cmp = q_gt(quark_v, quark_w);
    quark_cmp;
    q_println(q_sum(quark_cmp));
    QValue quark_big = q_get(quark_v, q_gt(quark_v, qv_int(25)));
    quark_big;
    q_println(quark_big);
    q_println(q_len(quark_big));
    q_println(q_get(quark_v, qv_int(0)));
    q_println(q_get(quark_v, qv_int(4)));
    q_println(q_get(quark_v, q_neg(qv_int(1))));
    QValue quark_total = q_sum(q_get(quark_v, q_gt(quark_v, qv_int(25))));
    quark_total;
    q_println(quark_total);
    QValue quark_none = q_get(quark_v, q_gt(quark_v, qv_int(100)));
    quark_none;
    q_println(q_len(quark_none));
    QValue _t3 = qv_vector(5);
    _t3 = q_vec_push(_t3, qv_int(1));
    _t3 = q_vec_push(_t3, qv_int(2));
    _t3 = q_vec_push(_t3, qv_int(3));
    _t3 = q_vec_push(_t3, qv_int(4));
    _t3 = q_vec_push(_t3, qv_int(5));
    QValue quark_vi = _t3;
    quark_vi;
    q_println(q_get(quark_vi, q_gt(quark_vi, qv_int(3))));
    q_println(q_sum(q_eq(quark_vi, qv_int(3))));
    q_println(q_sum(q_gte(quark_vi, qv_int(2))));
    return 0;
}
