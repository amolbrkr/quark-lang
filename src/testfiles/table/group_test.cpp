// Grouping, the aggregates and distinct on the thread pool (specs/
// tables_v0_3_spec.md §8.4) give the result of one loop over the rows: each
// is checked here against a reference built another way, from a stable sort
// of the rows by key (compare_rows and rows_equal), with each group's rows
// in row order. integration_table_test.go runs this program at several
// thread counts (QUARK_THREADS); the tables are large enough to be split
// into blocks, and there are steps with few groups (each range of rows folds
// its own states) and many (the rows go through group_parts).
#include "check.hpp"

#include <algorithm>
#include <cstring>
#include <random>

using namespace quark::tbl;

namespace {

struct Ref {
    std::vector<int32_t> group_of_row;
    std::vector<int32_t> first_row;
    std::vector<std::vector<int32_t>> rows; // each group's rows, in row order
};

Ref ref_groups(int32_t n, const std::vector<const QVector*>& keys) {
    std::vector<int32_t> order(static_cast<size_t>(n));
    for (int32_t r = 0; r < n; r++) order[static_cast<size_t>(r)] = r;
    std::stable_sort(order.begin(), order.end(), [&](int32_t a, int32_t b) {
        for (const QVector* k : keys) {
            const int c = compare_rows(*k, a, b, false);
            if (c != 0) return c < 0;
        }
        return false;
    });
    Ref ref;
    ref.group_of_row.assign(static_cast<size_t>(n), -1);
    for (size_t i = 0; i < order.size(); i++) {
        const int32_t r = order[i];
        bool same = i > 0;
        for (const QVector* k : keys) same = same && rows_equal(*k, r, order[i - 1]);
        if (!same) {
            ref.first_row.push_back(r);
            ref.rows.emplace_back();
        }
        ref.group_of_row[static_cast<size_t>(r)] = static_cast<int32_t>(ref.first_row.size()) - 1;
        ref.rows.back().push_back(r);
    }
    for (auto& rs : ref.rows) std::sort(rs.begin(), rs.end());
    return ref;
}

Ref ref_whole(int32_t n) {
    Ref ref;
    ref.group_of_row.assign(static_cast<size_t>(n), 0);
    ref.first_row.push_back(0);
    ref.rows.emplace_back();
    for (int32_t r = 0; r < n; r++) ref.rows[0].push_back(r);
    return ref;
}

bool present(const QVector& c, int32_t r) { return c.type == QVector::Type::F64 ? !is_missing_f64(c, r) : !is_null(c, r); }

double as_double(const QVector& c, int32_t r) {
    switch (c.type) {
        case QVector::Type::F64: return q_vec_f64_data(c)[r];
        case QVector::Type::I64: return static_cast<double>(q_vec_i64_data(c)[r]);
        case QVector::Type::BOOL: return q_vec_bool_at(c, static_cast<size_t>(r)) ? 1.0 : 0.0;
        default: return 0;
    }
}

// Same values, same missing entries, same float bits.
bool same_vec(const QVector& a, const QVector& b) {
    if (a.type != b.type || a.count != b.count) return false;
    for (size_t i = 0; i < a.count; i++) {
        const bool an = q_vec_is_null_at(a, i), bn = q_vec_is_null_at(b, i);
        if (an != bn) return false;
        if (an) continue;
        switch (a.type) {
            case QVector::Type::F64:
                if (std::memcmp(&q_vec_f64_data(a)[i], &q_vec_f64_data(b)[i], sizeof(double)) != 0) return false;
                break;
            case QVector::Type::I64:
                if (q_vec_i64_data(a)[i] != q_vec_i64_data(b)[i]) return false;
                break;
            case QVector::Type::DATE:
                if (q_vec_date_data(a)[i] != q_vec_date_data(b)[i]) return false;
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

QVector* ref_count(const Ref& ref, const QVector* c) {
    QVector* out = q_vec_alloc(QVector::Type::I64, ref.rows.size());
    for (size_t g = 0; g < ref.rows.size(); g++) {
        int64_t n = 0;
        for (int32_t r : ref.rows[g]) n += c == nullptr || present(*c, r) ? 1 : 0;
        q_vec_i64_data_mut(*out)[g] = n;
    }
    return out;
}

std::vector<double> present_values(const Ref& ref, size_t g, const QVector& c) {
    std::vector<double> xs;
    for (int32_t r : ref.rows[g]) {
        if (present(c, r)) xs.push_back(as_double(c, r));
    }
    return xs;
}

QVector* ref_sum(const Ref& ref, const QVector& c) {
    const size_t ng = ref.rows.size();
    if (c.type == QVector::Type::F64) {
        QVector* out = q_vec_alloc(QVector::Type::F64, ng);
        for (size_t g = 0; g < ng; g++) {
            const std::vector<double> xs = present_values(ref, g, c);
            q_vec_f64_data_mut(*out)[g] = q_pairwise_sum(xs.data(), xs.size());
        }
        return out;
    }
    QVector* out = q_vec_alloc(QVector::Type::I64, ng);
    for (size_t g = 0; g < ng; g++) {
        __int128 s = 0;
        for (int32_t r : ref.rows[g]) {
            if (present(c, r)) s += c.type == QVector::Type::I64 ? q_vec_i64_data(c)[r] : (q_vec_bool_at(c, static_cast<size_t>(r)) ? 1 : 0);
        }
        q_vec_i64_data_mut(*out)[g] = static_cast<int64_t>(s);
    }
    return out;
}

QVector* ref_mean(const Ref& ref, const QVector& c) {
    const size_t ng = ref.rows.size();
    QVector* out = q_vec_alloc(QVector::Type::F64, ng);
    for (size_t g = 0; g < ng; g++) {
        if (c.type == QVector::Type::F64) {
            const std::vector<double> xs = present_values(ref, g, c);
            if (xs.empty()) q_vec_mark_null(*out, g);
            else q_vec_f64_data_mut(*out)[g] = q_pairwise_sum(xs.data(), xs.size()) / static_cast<double>(xs.size());
            continue;
        }
        __int128 s = 0;
        int64_t n = 0;
        for (int32_t r : ref.rows[g]) {
            if (!present(c, r)) continue;
            s += c.type == QVector::Type::I64 ? q_vec_i64_data(c)[r] : (q_vec_bool_at(c, static_cast<size_t>(r)) ? 1 : 0);
            n++;
        }
        if (n == 0) q_vec_mark_null(*out, g);
        else q_vec_f64_data_mut(*out)[g] = q_i128_to_double(s) / static_cast<double>(n);
    }
    return out;
}

QVector* ref_sd(const Ref& ref, const QVector& c) {
    const size_t ng = ref.rows.size();
    QVector* out = q_vec_alloc(QVector::Type::F64, ng);
    for (size_t g = 0; g < ng; g++) {
        const std::vector<double> xs = present_values(ref, g, c);
        if (xs.size() < 2) q_vec_mark_null(*out, g);
        else q_vec_f64_data_mut(*out)[g] = q_sample_sd(xs.data(), xs.size());
    }
    return out;
}

// The first row holding the group's best present value.
QVector* ref_extremum(const Ref& ref, const QVector& c, bool want_max) {
    const size_t ng = ref.rows.size();
    std::vector<int32_t> best(ng, -1);
    for (size_t g = 0; g < ng; g++) {
        for (int32_t r : ref.rows[g]) {
            if (!present(c, r)) continue;
            const int32_t b = best[g];
            if (b < 0 || (want_max ? compare_rows(c, r, b, false) > 0 : compare_rows(c, r, b, false) < 0)) best[g] = r;
        }
    }
    QVector* out;
    if (c.count == 0) {
        out = q_vec_alloc(c.type, ng);
    } else {
        std::vector<int32_t> rows(ng);
        for (size_t g = 0; g < ng; g++) rows[g] = best[g] < 0 ? 0 : best[g];
        out = take(c, rows.data(), static_cast<int32_t>(ng));
    }
    for (size_t g = 0; g < ng; g++) {
        if (best[g] < 0 && !q_vec_is_null_at(*out, g)) q_vec_mark_null(*out, g);
    }
    return out;
}

// Distinct values per group: runs of a sort of the group's rows.
QVector* ref_n_distinct(const Ref& ref, const QVector& c) {
    const size_t ng = ref.rows.size();
    QVector* out = q_vec_alloc(QVector::Type::I64, ng);
    for (size_t g = 0; g < ng; g++) {
        std::vector<int32_t> rs = ref.rows[g];
        std::stable_sort(rs.begin(), rs.end(), [&](int32_t a, int32_t b) { return compare_rows(c, a, b, false) < 0; });
        int64_t n = 0;
        for (size_t i = 0; i < rs.size(); i++) n += i == 0 || !rows_equal(c, rs[i], rs[i - 1]) ? 1 : 0;
        q_vec_i64_data_mut(*out)[g] = n;
    }
    return out;
}

struct Data {
    int32_t n;
    QVector *ki, *kf, *ks, *kb, *fv, *iv, *bv, *dv, *sv;
};

// make_data builds n rows whose keys take about groups values (0: every
// row its own key), with missing values, NaN, -0.0 and 0.0, and ties.
Data make_data(int32_t n, int64_t groups) {
    std::mt19937_64 rng(static_cast<uint64_t>(n) * 1000003 + static_cast<uint64_t>(groups));
    Data d;
    d.n = n;
    std::vector<Opt> ki, iv;
    std::vector<OptF> kf, fv;
    std::vector<OptS> ks, sv;
    std::vector<OptB> kb, bv;
    for (int32_t r = 0; r < n; r++) {
        const int64_t k = groups == 0 ? (static_cast<int64_t>(r) * 7919) % n : static_cast<int64_t>(rng() % static_cast<uint64_t>(groups));
        ki.push_back(rng() % 53 == 0 ? Opt() : Opt(k));
        double f = static_cast<double>(k % 997) / 4.0;
        if (k % 11 == 0) f = 0.0;
        if (k % 13 == 0) f = -0.0;
        if (k % 17 == 0) f = std::nan("");
        kf.push_back(rng() % 61 == 0 ? OptF() : OptF(f));
        ks.push_back(rng() % 47 == 0 ? OptS() : OptS("k" + std::to_string(k)));
        kb.push_back(rng() % 71 == 0 ? OptB() : OptB(k % 2 == 0));
        const uint64_t u = rng() % 100;
        double x = static_cast<double>(rng() % 100000) / 7.0;
        if (u == 0) x = std::nan("");
        if (u == 1) x = -0.0;
        if (u == 2) x = 0.0;
        if (u == 3) x = 1e300;
        if (u == 4) x = -1e300;
        fv.push_back(rng() % 37 == 0 ? OptF() : OptF(x));
        iv.push_back(rng() % 41 == 0 ? Opt() : Opt(static_cast<long long>(rng() % 2001) - 1000 + (u == 5 ? 1000000000000LL : 0)));
        bv.push_back(rng() % 43 == 0 ? OptB() : OptB(rng() % 3 == 0));
        sv.push_back(rng() % 31 == 0 ? OptS() : OptS("s" + std::to_string(rng() % 300)));
    }
    d.ki = i64_col(ki);
    d.kf = f64_col(kf);
    d.ks = str_col(ks);
    d.kb = bool_col(kb);
    d.fv = f64_col(fv);
    d.iv = i64_col(iv);
    d.bv = bool_col(bv);
    d.sv = str_col(sv);
    d.dv = q_vec_alloc(QVector::Type::DATE, static_cast<size_t>(n));
    for (int32_t r = 0; r < n; r++) {
        q_vec_date_data_mut(*d.dv)[r] = static_cast<int32_t>(rng() % 5000) - 2500;
        if (rng() % 29 == 0) q_vec_mark_null(*d.dv, static_cast<size_t>(r));
    }
    return d;
}

void check_aggregates(const char* what, const Data& d, const Grouping& g, const Ref& ref) {
    char label[256];
    auto expect = [&](const char* agg, const QVector* got, const QVector* want) {
        std::snprintf(label, sizeof label, "%s: %s", what, agg);
        EXPECT(label, same_vec(*got, *want));
    };
    expect("count", agg_count(g), ref_count(ref, nullptr));
    const QVector* vals[] = {d.fv, d.iv, d.bv, d.dv, d.sv};
    const char* names[] = {"float", "int", "bool", "date", "str"};
    for (int i = 0; i < 5; i++) {
        const QVector& c = *vals[i];
        char agg[64];
        auto name = [&](const char* fn) {
            std::snprintf(agg, sizeof agg, "%s %s", fn, names[i]);
            return agg;
        };
        expect(name("count"), agg_count_values(c, g), ref_count(ref, &c));
        if (i < 3) {
            expect(name("sum"), agg_sum(c, g), ref_sum(ref, c));
            expect(name("mean"), agg_mean(c, g), ref_mean(ref, c));
            expect(name("sd"), agg_sd(c, g), ref_sd(ref, c));
        }
        expect(name("min"), agg_min(c, g), ref_extremum(ref, c, false));
        expect(name("max"), agg_max(c, g), ref_extremum(ref, c, true));
        expect(name("n_distinct"), agg_n_distinct(c, g), ref_n_distinct(ref, c));
    }
}

void check_grouping(const Data& d, int64_t groups, const char* keyset, std::vector<const QVector*> keys) {
    char what[128];
    std::snprintf(what, sizeof what, "%d rows, %lld keys, %s", d.n, static_cast<long long>(groups), keyset);
    const Grouping g = group_by_vecs(d.n, keys.data(), static_cast<int>(keys.size()));
    const Ref ref = ref_groups(d.n, keys);
    bool same = g.ngroups == static_cast<int32_t>(ref.first_row.size());
    for (int32_t i = 0; same && i < g.ngroups; i++) same = g.first_row[i] == ref.first_row[static_cast<size_t>(i)];
    for (int32_t r = 0; same && r < d.n; r++) same = g.group_of_row[r] == ref.group_of_row[static_cast<size_t>(r)];
    char label[160];
    std::snprintf(label, sizeof label, "%s: groups in key order, each with its first row", what);
    EXPECT(label, same);
    if (!same) return;
    check_aggregates(what, d, g, ref);

    // distinct keeps each key's first row, in row order.
    std::vector<QVector*> cols;
    std::vector<const char*> names;
    std::vector<int> key_cols;
    for (size_t k = 0; k < keys.size(); k++) {
        cols.push_back(const_cast<QVector*>(keys[k]));
        names.push_back(k == 0 ? "a" : (k == 1 ? "b" : "c"));
        key_cols.push_back(static_cast<int>(k));
    }
    cols.push_back(d.sv);
    names.push_back("v");
    QTable* t = table(names, cols);
    QTable* dt = distinct_rows(t, key_cols.data(), static_cast<int>(key_cols.size()));
    std::vector<int32_t> firsts = ref.first_row;
    std::sort(firsts.begin(), firsts.end());
    QTable* want = take_table(t, firsts.data(), static_cast<int32_t>(firsts.size()));
    bool same_rows = dt->nrows == want->nrows;
    for (int c = 0; same_rows && c < t->ncols; c++) same_rows = same_vec(*col(dt, c), *col(want, c));
    std::snprintf(label, sizeof label, "%s: distinct keeps first rows in order", what);
    EXPECT(label, same_rows);
}

} // namespace

int main() {
    q_gc_init();
    for (int32_t n : {1000, 140000}) {
        for (int64_t groups : {int64_t(1), int64_t(3), int64_t(1000), int64_t(50000), int64_t(0)}) {
            const Data d = make_data(n, groups);
            check_grouping(d, groups, "int key", {d.ki});
            check_grouping(d, groups, "float key", {d.kf});
            check_grouping(d, groups, "str key", {d.ks});
            check_grouping(d, groups, "str, bool and float keys", {d.ks, d.kb, d.kf});
        }
        const Data d = make_data(n, 1000);
        char what[64];
        std::snprintf(what, sizeof what, "%d rows, whole table", n);
        check_aggregates(what, d, whole_table(n), ref_whole(n));
    }
    // A key column with no missing values takes the typed path.
    {
        std::vector<Opt> k;
        std::vector<OptS> s;
        for (int32_t r = 0; r < 140000; r++) {
            k.push_back((static_cast<long long>(r) * 7919) % 20000 - 10000);
            s.push_back("key" + std::to_string((r * 31) % 777));
        }
        Data d = make_data(140000, 1000);
        d.ki = i64_col(k);
        d.ks = str_col(s);
        QVector* dates = q_vec_alloc(QVector::Type::DATE, 140000);
        for (int32_t r = 0; r < 140000; r++) q_vec_date_data_mut(*dates)[r] = (r * 13) % 5000 - 2500;
        check_grouping(d, 20000, "dense int key", {d.ki});
        check_grouping(d, 777, "dense str key", {d.ks});
        check_grouping(d, 5000, "dense date key", {dates});
    }
    std::printf("threads: %d\n", q_par_threads());
    return finish("group");
}
