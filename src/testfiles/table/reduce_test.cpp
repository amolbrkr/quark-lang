// Vector reductions agree with the table aggregates (specs/tables_v0_1_spec.md
// D13): sum(v), min(v), max(v) and stats::mean(v) of a vec must give what
// summarize gives for the same column over the whole table.

#include "check.hpp"
#include "../../core/stdlib/stats.hpp"

#include <cmath>

using namespace quark::tbl;

static QValue boxed(QVector* v) { return qv_vector_from(v); }

// The single value of a one-group aggregate column, as a QValue.
static QValue first(const QVector* out) {
    if (q_vec_is_null_at(*out, 0)) return qv_null();
    return q_vec_box_at(*out, 0);
}

static bool same(QValue a, QValue b) {
    if (a.type != b.type) return false;
    switch (a.type) {
        case QValue::VAL_INT: return a.data.int_val == b.data.int_val;
        case QValue::VAL_FLOAT: return a.data.float_val == b.data.float_val;
        case QValue::VAL_BOOL: return a.data.bool_val == b.data.bool_val;
        case QValue::VAL_STRING: return std::string(a.data.string_val) == std::string(b.data.string_val);
        case QValue::VAL_NULL: return true;
        default: return false;
    }
}

static void agree(const char* label, QVector* col) {
    Grouping all = whole_table(static_cast<int32_t>(col->count));
    std::string l(label);
    if (col->type != QVector::Type::STR) {
        EXPECT((l + ": sum").c_str(), same(q_vec_sum(boxed(col)), first(agg_sum(*col, all))));
        EXPECT((l + ": mean").c_str(), q_stats_mean(col) == q_vec_f64_data(*agg_mean(*col, all))[0]);
    }
    EXPECT((l + ": min").c_str(), same(q_vec_min(boxed(col)), first(agg_min(*col, all))));
    EXPECT((l + ": max").c_str(), same(q_vec_max(boxed(col)), first(agg_max(*col, all))));
}

int main() {
    q_gc_init();
    const double nan = std::nan("");

    agree("int", i64_col({3, 1, 4, 1, 5}));
    agree("int with nulls", i64_col({std::nullopt, 7, -2, std::nullopt, 9}));
    agree("int above 2^53", i64_col({9007199254740993LL, 1}));
    agree("float", f64_col({2.5, -1.25, 8.0}));
    agree("float with NaN and nulls", f64_col({nan, 2.5, std::nullopt, 1.5, nan}));
    agree("dense float with NaN first", f64_col({nan, 2.5, -1.0, 7.25}));
    agree("dense float ties of signed zero", f64_col({0.0, -0.0, 3.0}));
    std::vector<OptF> many;
    for (int i = 0; i < 1000; i++) many.push_back(0.1 * (i % 7) - 0.3 + 1e-3 * i);
    agree("float, many values (pairwise blocks)", f64_col(many));
    agree("bool", bool_col({true, false, true, true}));
    agree("bool with nulls", bool_col({std::nullopt, false, true}));
    agree("str", str_col({std::string("pear"), std::string("apple"), std::nullopt, std::string("fig")}));

    // No present values: sums are 0 on both sides.
    Grouping one = whole_table(1);
    QVector* none = i64_col({std::nullopt});
    EXPECT("all-missing int sum", same(q_vec_sum(boxed(none)), first(agg_sum(*none, one))));
    QVector* nans = f64_col({nan});
    EXPECT("all-NaN float sum", same(q_vec_sum(boxed(nans)), first(agg_sum(*nans, one))));
    EXPECT("exact int sum", same(q_vec_sum(boxed(i64_col({9007199254740993LL, 1}))), qv_int(9007199254740994LL)));

    // Float sums are pairwise in double (core/sum.hpp). The same present
    // values give the same bits whether the vec has missing entries or NaN
    // between them, and a group sums exactly as a vec of its values.
    std::vector<OptF> present, holey;
    for (int i = 0; i < 700; i++) {
        const double x = 1.0 / (i + 1);
        present.push_back(x);
        holey.push_back(x);
        if (i % 3 == 0) holey.push_back(std::nullopt);
        if (i % 5 == 0) holey.push_back(nan);
    }
    EXPECT("missing entries and NaN don't change the sum",
           same(q_vec_sum(boxed(f64_col(holey))), q_vec_sum(boxed(f64_col(present)))));
    std::vector<OptS> keys;
    std::vector<OptF> vals, group_a, group_b;
    for (int i = 0; i < 600; i++) {
        const double x = 1.0 / (i + 3);
        const bool a = (i % 4) != 1;
        keys.push_back(std::string(a ? "a" : "b"));
        vals.push_back(x);
        (a ? group_a : group_b).push_back(x);
        if (i % 9 == 0) {
            keys.push_back(std::string("a"));
            vals.push_back(std::nullopt);
        }
    }
    QTable* t = table({"k", "x"}, {str_col(keys), f64_col(vals)});
    int key[] = {0};
    Grouping g = group_rows(t, key, 1);
    QVector* sums = agg_sum(*col(t, 1), g);
    QVector* means = agg_mean(*col(t, 1), g);
    EXPECT("group a sums as a vec of its values", q_vec_f64_data(*sums)[0] == q_vec_sum(boxed(f64_col(group_a))).data.float_val);
    EXPECT("group b sums as a vec of its values", q_vec_f64_data(*sums)[1] == q_vec_sum(boxed(f64_col(group_b))).data.float_val);
    EXPECT("group a mean as a vec of its values", q_vec_f64_data(*means)[0] == q_stats_mean(f64_col(group_a)));
    EXPECT("group b mean as a vec of its values", q_vec_f64_data(*means)[1] == q_stats_mean(f64_col(group_b)));

    // Pairwise summation keeps the error near log n ulps: a million 0.1s sum
    // to within 1e-8 of 100000 (summing one by one is off by about 1e-6).
    std::vector<double> tenths(1000000, 0.1);
    EXPECT("pairwise accuracy", std::fabs(q_pairwise_sum(tenths.data(), tenths.size()) - 100000.0) < 1e-8);
    double naive = 0.0;
    for (double x : tenths) naive += x;
    EXPECT("naive summation is less accurate (the test can tell)", std::fabs(naive - 100000.0) > 1e-8);

    // Int means are exact sums divided once, also beyond 2^53.
    EXPECT("int mean above 2^53", q_stats_mean(i64_col({9007199254740993LL, 9007199254740995LL})) == 9007199254740994.0);

    return finish("vec reductions");
}
