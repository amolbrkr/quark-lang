// Table engine kernels: take, sort, group, aggregates (tables v0.1).
#include "check.hpp"

#include <cmath>

using namespace quark::tbl;
using std::nullopt;

static void test_take() {
    QTable* t = table({"i", "f", "s", "b"},
                      {i64_col({1, nullopt, 3, 4}), f64_col({1.5, 2.5, nullopt, 4.5}),
                       str_col({"a", "bb", nullopt, ""}), bool_col({true, false, nullopt, true})});
    int32_t idx[] = {3, 1, 1, 2};
    QTable* r = take_table(t, idx, 4);
    EXPECT("take well formed", well_formed(r));
    EXPECT("take i64", ints(col(r, 0)) == std::vector<Opt>{4, nullopt, nullopt, 3});
    EXPECT("take f64", floats(col(r, 1)) == std::vector<OptF>{4.5, 2.5, 2.5, nullopt});
    EXPECT("take str", strs(col(r, 2)) == std::vector<OptS>{"", "bb", "bb", nullopt});
    EXPECT("take bool", bools(col(r, 3)) == std::vector<OptB>{true, false, false, nullopt});
    EXPECT("take input unchanged", ints(col(t, 0)) == std::vector<Opt>{1, nullopt, 3, 4});

    int32_t all[] = {0, 1, 2, 3};
    EXPECT("take all rows in order shares the table", take_table(t, all, 4) == t);
    QTable* none = take_table(t, idx, 0);
    EXPECT("take no rows", none->nrows == 0 && well_formed(none));

    // A gather without nulls in the result has no validity bitmap.
    int32_t dense[] = {0, 3};
    QTable* d = take_table(t, dense, 2);
    EXPECT("dense take has no validity", col(d, 0)->validity == nullptr && col(d, 0)->null_count == 0);
}

static void test_sort() {
    // Stable: equal keys keep their order (the second column records it).
    QTable* t = table({"k", "pos"}, {i64_col({3, 1, 3, 2, 1}), i64_col({0, 1, 2, 3, 4})});
    SortKey asc[] = {{0, false}};
    QTable* a = sort_table(t, asc, 1);
    EXPECT("int asc", ints(col(a, 0)) == std::vector<Opt>{1, 1, 2, 3, 3});
    EXPECT("int asc stable", ints(col(a, 1)) == std::vector<Opt>{1, 4, 3, 0, 2});
    SortKey desc[] = {{0, true}};
    QTable* d = sort_table(t, desc, 1);
    EXPECT("int desc", ints(col(d, 0)) == std::vector<Opt>{3, 3, 2, 1, 1});
    EXPECT("int desc stable", ints(col(d, 1)) == std::vector<Opt>{0, 2, 3, 1, 4});

    // Nulls last and NaN after numbers, in both directions.
    const double nan = std::nan("");
    QTable* f = table({"x"}, {f64_col({2.0, nullopt, nan, -1.0, 5.0})});
    SortKey fa[] = {{0, false}}, fd[] = {{0, true}};
    std::vector<OptF> fasc = floats(col(sort_table(f, fa, 1), 0));
    std::vector<OptF> fdesc = floats(col(sort_table(f, fd, 1), 0));
    EXPECT("float asc values", fasc[0] == -1.0 && fasc[1] == 2.0 && fasc[2] == 5.0);
    EXPECT("float asc NaN then null", std::isnan(*fasc[3]) && !fasc[4]);
    EXPECT("float desc values", fdesc[0] == 5.0 && fdesc[1] == 2.0 && fdesc[2] == -1.0);
    EXPECT("float desc NaN then null", std::isnan(*fdesc[3]) && !fdesc[4]);

    // Several keys: dept ascending, salary descending; nulls last.
    QTable* e = table({"dept", "salary"},
                      {str_col({"b", "a", nullopt, "a", "b"}), i64_col({10, 30, 50, nullopt, 20})});
    SortKey keys[] = {{0, false}, {1, true}};
    QTable* s = sort_table(e, keys, 2);
    EXPECT("multi-key first", strs(col(s, 0)) == std::vector<OptS>{"a", "a", "b", "b", nullopt});
    EXPECT("multi-key second", ints(col(s, 1)) == std::vector<Opt>{30, nullopt, 20, 10, 50});

    QTable* bt = table({"b"}, {bool_col({true, false, nullopt, false})});
    SortKey bk[] = {{0, false}};
    EXPECT("bool asc", bools(col(sort_table(bt, bk, 1), 0)) == std::vector<OptB>{false, false, true, nullopt});

    QTable* empty = table({"x"}, {i64_col({})});
    EXPECT("sort empty", sort_table(empty, asc, 1)->nrows == 0);
}

static void test_group_and_aggregates() {
    QTable* t = table({"dept", "salary", "bonus", "remote"},
                      {str_col({"eng", "ops", "eng", nullopt, "ops", "eng"}),
                       i64_col({100, 80, 120, 70, nullopt, 110}),
                       f64_col({1.5, nullopt, 2.5, 4.0, 1.0, std::nan("")}),
                       bool_col({true, false, true, nullopt, true, false})});
    int keys[] = {0};
    Grouping g = group_rows(t, keys, 1);
    EXPECT("group count", g.ngroups == 3);
    // Groups in key order: eng, ops, then the null key.
    EXPECT("group keys", strs(group_keys(t, g, 0)) == std::vector<OptS>{"eng", "ops", nullopt});
    EXPECT("group of rows", g.group_of_row[0] == 0 && g.group_of_row[1] == 1 && g.group_of_row[3] == 2 &&
                                g.group_of_row[5] == 0);

    EXPECT("n", ints(agg_count(g)) == std::vector<Opt>{3, 2, 1});
    EXPECT("sum int skips null", ints(agg_sum(*col(t, 1), g)) == std::vector<Opt>{330, 80, 70});
    EXPECT("count values", ints(agg_count_values(*col(t, 1), g)) == std::vector<Opt>{3, 1, 1});
    EXPECT("count values skips NaN", ints(agg_count_values(*col(t, 2), g)) == std::vector<Opt>{2, 1, 1});
    EXPECT("mean int", floats(agg_mean(*col(t, 1), g)) == std::vector<OptF>{110.0, 80.0, 70.0});
    EXPECT("sum float skips null and NaN", floats(agg_sum(*col(t, 2), g)) == std::vector<OptF>{4.0, 1.0, 4.0});
    EXPECT("mean float", floats(agg_mean(*col(t, 2), g)) == std::vector<OptF>{2.0, 1.0, 4.0});
    EXPECT("min int", ints(agg_min(*col(t, 1), g)) == std::vector<Opt>{100, 80, 70});
    EXPECT("max int", ints(agg_max(*col(t, 1), g)) == std::vector<Opt>{120, 80, 70});
    EXPECT("max str", strs(agg_max(*col(t, 0), g)) == std::vector<OptS>{"eng", "ops", nullopt});
    EXPECT("sum bool counts true", ints(agg_sum(*col(t, 3), g)) == std::vector<Opt>{2, 1, 0});
    EXPECT("mean bool is a share", floats(agg_mean(*col(t, 3), g)) == std::vector<OptF>{2.0 / 3.0, 0.5, nullopt});

    // A group whose values are all missing: sum 0, mean/min/max null.
    QTable* m = table({"k", "v"}, {str_col({"a", "b", "b"}), i64_col({1, nullopt, nullopt})});
    int mk[] = {0};
    Grouping mg = group_rows(m, mk, 1);
    EXPECT("all-null sum is 0", ints(agg_sum(*col(m, 1), mg)) == std::vector<Opt>{1, 0});
    EXPECT("all-null mean is null", floats(agg_mean(*col(m, 1), mg)) == std::vector<OptF>{1.0, nullopt});
    EXPECT("all-null min is null", ints(agg_min(*col(m, 1), mg)) == std::vector<Opt>{1, nullopt});

    // Several keys, including float keys where 0.0 and -0.0 are one group.
    QTable* mt = table({"a", "b", "v"}, {i64_col({1, 1, 2, 1, 2}), f64_col({0.0, -0.0, 1.0, 1.0, 1.0}),
                                         i64_col({1, 2, 3, 4, 5})});
    int mkeys[] = {0, 1};
    Grouping mm = group_rows(mt, mkeys, 2);
    EXPECT("multi-key groups", mm.ngroups == 3);
    EXPECT("multi-key sums", ints(agg_sum(*col(mt, 2), mm)) == std::vector<Opt>{3, 4, 8});
    EXPECT("multi-key key a", ints(group_keys(mt, mm, 0)) == std::vector<Opt>{1, 1, 2});

    // Whole-table aggregation (summarize without group_by), also on no rows.
    Grouping all = whole_table(t->nrows);
    EXPECT("whole n", ints(agg_count(all)) == std::vector<Opt>{6});
    EXPECT("whole sum", ints(agg_sum(*col(t, 1), all)) == std::vector<Opt>{480});
    EXPECT("whole max float skips NaN", floats(agg_max(*col(t, 2), all)) == std::vector<OptF>{4.0});
    QTable* empty = table({"x"}, {i64_col({})});
    Grouping none = whole_table(0);
    EXPECT("empty n", ints(agg_count(none)) == std::vector<Opt>{0});
    EXPECT("empty sum", ints(agg_sum(*col(empty, 0), none)) == std::vector<Opt>{0});
    EXPECT("empty mean", floats(agg_mean(*col(empty, 0), none)) == std::vector<OptF>{nullopt});
    EXPECT("empty max", ints(agg_max(*col(empty, 0), none)) == std::vector<Opt>{nullopt});
    int ek[] = {0};
    EXPECT("group empty table", group_rows(empty, ek, 1).ngroups == 0);

    // summarize: keys, then aggregates, as one table.
    QVector* out[] = {group_keys(t, g, 0), agg_mean(*col(t, 1), g), agg_count(g)};
    QTable* s = make_table(schema({"dept", "mean_salary", "n"}), out);
    EXPECT("summarize table", s->nrows == 3 && s->ncols == 3 && well_formed(s));
}

static void test_scale() {
    // 500k rows, 1000 departments: group + aggregate and sort stay well
    // under a second each (timings are printed, not asserted).
    const int32_t n = 500000;
    QVector* dept = q_vec_alloc(QVector::Type::I64, n);
    QVector* sal = q_vec_alloc(QVector::Type::F64, n);
    uint64_t x = 42;
    for (int32_t i = 0; i < n; i++) {
        x = x * 6364136223846793005ULL + 1442695040888963407ULL;
        q_vec_i64_data_mut(*dept)[i] = static_cast<int64_t>((x >> 33) % 1000);
        q_vec_f64_data_mut(*sal)[i] = static_cast<double>((x >> 20) % 200000) / 2.0;
    }
    QVector* cols[] = {dept, sal};
    QTable* t = make_table(schema({"dept", "salary"}), cols);

    Timer tg;
    int key[] = {0};
    Grouping g = group_rows(t, key, 1);
    QVector* means = agg_mean(*col(t, 1), g);
    const double group_ms = tg.ms();
    EXPECT("scale groups", g.ngroups == 1000 && means->count == 1000);

    Timer ts;
    SortKey by[] = {{1, true}};
    QTable* sorted = sort_table(t, by, 1);
    const double sort_ms = ts.ms();
    bool ordered = true;
    for (int32_t i = 1; i < n && ordered; i++) ordered = q_vec_f64_data(*col(sorted, 1))[i - 1] >= q_vec_f64_data(*col(sorted, 1))[i];
    EXPECT("scale sorted", ordered);

    Timer tm;
    SortKey two[] = {{0, false}, {1, true}};
    sort_table(t, two, 2);
    const double sort2_ms = tm.ms();

    std::printf("scale (500k rows): group_by+mean %.1f ms, sort by float %.1f ms, sort by 2 keys %.1f ms\n",
                group_ms, sort_ms, sort2_ms);
}

int main() {
    q_gc_init();
    test_take();
    test_sort();
    test_group_and_aggregates();
    test_scale();
    return finish("table engine");
}
