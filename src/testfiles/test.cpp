#include "quark/quark.hpp"

// Extension includes
#include "D:\\Source\\quark-lang\\src\\core\\stdlib\\random.hpp"

// Forward declarations
QValue quark_q1_random__random__seed(QClosure*, QValue);
double quark_q1_random__random__uniform(QClosure*);
QValue quark_q1_random__random__uniform__thunk(QClosure*);
int64_t quark_q1_random__random__randint(QClosure*, int64_t, int64_t);
QValue quark_q1_random__random__randint__thunk(QClosure*, QValue, QValue);
QValue quark_q1_random__random__choice(QClosure*, QValue);
QList* quark_q1_random__random__shuffle(QClosure*, QList*);
QValue quark_q1_random__random__shuffle__thunk(QClosure*, QValue);
QValue quark_pick1(QClosure*, QValue);
QValue _thunk_q_random_choice(QClosure*, QValue);
QValue _thunk_q_random_randint(QClosure*, QValue, QValue);
QValue _thunk_q_random_seed(QClosure*, QValue);
QValue _thunk_q_random_shuffle(QClosure*, QValue);
QValue _thunk_q_random_uniform(QClosure*);

static const QFnInfo _qfi_q1_random__random__seed = {"random::seed", 1, 1, nullptr};
static const QFnInfo _qfi_q1_random__random__uniform = {"random::uniform", 0, 0, nullptr};
static const QFnInfo _qfi_q1_random__random__randint = {"random::randint", 2, 2, nullptr};
static const QFnInfo _qfi_q1_random__random__choice = {"random::choice", 1, 1, nullptr};
static const QFnInfo _qfi_q1_random__random__shuffle = {"random::shuffle", 1, 1, nullptr};
static QValue quark_pick1__default_arg(int index);
static const QFnInfo _qfi_pick1 = {"pick1", 0, 1, quark_pick1__default_arg};
static const QFnInfo _qfi__thunk_q_random_choice = {"random::random_choice", 1, 1, nullptr};
static const QFnInfo _qfi__thunk_q_random_randint = {"random::random_randint", 2, 2, nullptr};
static const QFnInfo _qfi__thunk_q_random_seed = {"random::random_seed", 1, 1, nullptr};
static const QFnInfo _qfi__thunk_q_random_shuffle = {"random::random_shuffle", 1, 1, nullptr};
static const QFnInfo _qfi__thunk_q_random_uniform = {"random::random_uniform", 0, 0, nullptr};

// Source locations for runtime errors
static const QSourceLoc _qloc[] = {
    {"..\\..\\testfiles\\test.qrk", 3, 47},
    {"..\\..\\testfiles\\test.qrk", 5, 6},
    {"..\\..\\testfiles\\test.qrk", 7, 3},
    {"..\\..\\testfiles\\test.qrk", 8, 3},
    {"..\\..\\testfiles\\test.qrk", 9, 6},
};

static QValue quark_pick1__default_arg(int index) {
    switch (index) {
    case 0: {
        return qv_int(10);
    }
    }
    return qv_null();
}

QValue quark_q1_random__random__seed(QClosure* _cl, QValue _arg_quark_n) {
    QSourceLocGuard _loc_guard;
    long long quark_n = q_param_int(_arg_quark_n, "random::seed", "n");
    return q_random_seed(quark_n);
}

double quark_q1_random__random__uniform(QClosure* _cl) {
    QSourceLocGuard _loc_guard;
    return q_random_uniform();
}

QValue quark_q1_random__random__uniform__thunk(QClosure* _cl) {
    return qv_float(quark_q1_random__random__uniform(nullptr));
}

int64_t quark_q1_random__random__randint(QClosure* _cl, int64_t _arg_quark_lo, int64_t _arg_quark_hi) {
    QSourceLocGuard _loc_guard;
    long long quark_lo = _arg_quark_lo;
    long long quark_hi = _arg_quark_hi;
    return q_random_randint(quark_lo, quark_hi);
}

QValue quark_q1_random__random__randint__thunk(QClosure* _cl, QValue _a0, QValue _a1) {
    return qv_int(quark_q1_random__random__randint(nullptr, q_as_int(_a0), q_as_int(_a1)));
}

QValue quark_q1_random__random__choice(QClosure* _cl, QValue _arg_quark_xs) {
    QSourceLocGuard _loc_guard;
    QValue quark_xs = q_param_check(_arg_quark_xs, QValue::VAL_LIST, false, "list", "random::choice", "xs");
    return q_random_choice(q_as_list(quark_xs));
}

QList* quark_q1_random__random__shuffle(QClosure* _cl, QList* _arg_quark___v11_xs) {
    QSourceLocGuard _loc_guard;
    QValue quark___v11_xs = qv_list_ptr(_arg_quark___v11_xs);
    return q_as_list(qv_list_ptr(q_random_shuffle(q_as_list(quark___v11_xs))));
}

QValue quark_q1_random__random__shuffle__thunk(QClosure* _cl, QValue _a0) {
    return qv_list_ptr(quark_q1_random__random__shuffle(nullptr, q_as_list(_a0)));
}

QValue quark_pick1(QClosure* _cl, QValue _arg_quark_max) {
    QSourceLocGuard _loc_guard;
    long long quark_max = q_param_int(_arg_quark_max, "pick1", "max");
    q_set_source_loc(&_qloc[0]);
    return qv_int(quark_q1_random__random__randint(nullptr, 0, quark_max));
}

QValue _thunk_q_random_choice(QClosure* _cl, QValue _a0) {
    return q_random_choice(q_as_list(_a0));
}

QValue _thunk_q_random_randint(QClosure* _cl, QValue _a0, QValue _a1) {
    return qv_int(q_random_randint(q_as_int(_a0), q_as_int(_a1)));
}

QValue _thunk_q_random_seed(QClosure* _cl, QValue _a0) {
    return q_random_seed(q_as_int(_a0));
}

QValue _thunk_q_random_shuffle(QClosure* _cl, QValue _a0) {
    return qv_list_ptr(q_random_shuffle(q_as_list(_a0)));
}

QValue _thunk_q_random_uniform(QClosure* _cl) {
    return qv_float(q_random_uniform());
}


int main(int argc, char** argv) {
    q_gc_init();
    q_set_args(argc, argv);
    q_set_source_loc(&_qloc[1]);
    q_print(quark_pick1(nullptr, qv_int(10)));
    q_set_source_loc(&_qloc[2]);
    double quark_x = 1.5;
    q_set_source_loc(&_qloc[3]);
    QValue _t1 = qv_list(3, &q_elem_int);
    _t1 = q_push(_t1, qv_int(1));
    _t1 = q_push(_t1, qv_int(2));
    _t1 = q_push(_t1, qv_int(3));
    quark_x = (double)((q_len(_t1)).data.int_val);
    q_set_source_loc(&_qloc[4]);
    q_print(qv_float(quark_x));
    return 0;
}
