// quark/types/date.hpp — calendar dates (Arrow date32) and calendar periods.
//
// A date is a signed count of days since 1970-01-01, stored in 32 bits
// (specs/dates_v0_1_spec.md). A period is years, months and days. Adding a
// period applies years and months, rolls an impossible day back to the last
// day of that month (lubridate's %m+%), then adds the day count exactly.
#ifndef QUARK_TYPES_DATE_HPP
#define QUARK_TYPES_DATE_HPP

#include "../core/constructors.hpp"
#include <cstdint>
#include <cstdio>
#include <cstring>
#include <limits>
#include <string>

struct QPeriod {
    int32_t years = 0;
    int32_t months = 0;
    int32_t days = 0;
};

inline QValue qv_date(int32_t days) {
    QValue q;
    q.type = QValue::VAL_DATE;
    q.data.int_val = days;
    return q;
}

inline QPeriod* q_period_new(int32_t years, int32_t months, int32_t days) {
    QPeriod* p = static_cast<QPeriod*>(q_malloc_atomic(sizeof(QPeriod)));
    if (!p) {
        q_runtime_reportf("runtime error: out of memory allocating a period\n");
        q_exit(1);
    }
    p->years = years;
    p->months = months;
    p->days = days;
    return p;
}

inline QValue qv_period(int32_t years, int32_t months, int32_t days) {
    QValue q;
    q.type = QValue::VAL_PERIOD;
    q.data.period_val = q_period_new(years, months, days);
    return q;
}

inline const QPeriod* q_period_get(QValue v) {
    if (v.type != QValue::VAL_PERIOD || v.data.period_val == nullptr) {
        q_runtime_reportf("runtime error: expected period, got %s\n", q_type_name(v.type));
        q_exit(1);
    }
    return v.data.period_val;
}

inline int32_t q_as_date(QValue v) {
    if (v.type != QValue::VAL_DATE) {
        q_runtime_reportf("runtime error: expected date, got %s\n", q_type_name(v.type));
        q_exit(1);
    }
    return static_cast<int32_t>(v.data.int_val);
}

inline int32_t q_prove_date(QValue v) { return q_as_date(v); }

[[noreturn]] inline void q_date_fail(const char* msg) {
    q_runtime_reportf("runtime error: %s\n", msg);
    q_exit(1);
}

// Days since 1970-01-01 (Howard Hinnant, public domain). month is 1-12.
inline int64_t q_days_from_civil(int year, int month, int day) {
    year -= month <= 2;
    const int era = (year >= 0 ? year : year - 399) / 400;
    const unsigned yoe = static_cast<unsigned>(year - era * 400);
    const unsigned doy = (153u * static_cast<unsigned>(month + (month > 2 ? -3 : 9)) + 2) / 5 +
                         static_cast<unsigned>(day) - 1;
    const unsigned doe = yoe * 365 + yoe / 4 - yoe / 100 + doy;
    return static_cast<int64_t>(era) * 146097 + static_cast<int64_t>(doe) - 719468;
}

inline void q_civil_from_days(int64_t z, int& year, int& month, int& day) {
    z += 719468;
    const int64_t era = (z >= 0 ? z : z - 146096) / 146097;
    const unsigned doe = static_cast<unsigned>(z - era * 146097);
    const unsigned yoe = (doe - doe / 1460 + doe / 36524 - doe / 146096) / 365;
    const int y = static_cast<int>(yoe) + static_cast<int>(era) * 400;
    const unsigned doy = doe - (365 * yoe + yoe / 4 - yoe / 100);
    const unsigned mp = (5 * doy + 2) / 153;
    const unsigned d = doy - (153 * mp + 2) / 5 + 1;
    const unsigned m = mp < 10 ? mp + 3 : mp - 9;
    year = y + (m <= 2 ? 1 : 0);
    month = static_cast<int>(m);
    day = static_cast<int>(d);
}

inline bool q_date_leap(int year) {
    if (year % 4 != 0) return false;
    if (year % 100 != 0) return true;
    return year % 400 == 0;
}

inline int q_days_in_month(int year, int month) {
    static const int md[12] = {31, 28, 31, 30, 31, 30, 31, 31, 30, 31, 30, 31};
    if (month < 1 || month > 12) return 0;
    if (month == 2 && q_date_leap(year)) return 29;
    return md[month - 1];
}

inline int32_t q_date_fit(int64_t days) {
    if (days < static_cast<int64_t>(std::numeric_limits<int32_t>::min()) ||
        days > static_cast<int64_t>(std::numeric_limits<int32_t>::max())) {
        q_date_fail("date is outside the range of dates");
    }
    return static_cast<int32_t>(days);
}

inline int32_t q_date_add_days(int32_t days, long long n) {
    return q_date_fit(static_cast<int64_t>(days) + static_cast<int64_t>(n));
}

inline long long q_date_diff(int32_t a, int32_t b) {
    return static_cast<long long>(a) - static_cast<long long>(b);
}

// Years, then months, then roll the original day back when that day does not
// exist in the resulting month, then add the day count.
inline int32_t q_date_add_period_raw(int32_t days, const QPeriod& p) {
    int year = 0, month = 0, day = 0;
    q_civil_from_days(days, year, month, day);
    const int orig = day;
    const int64_t y2 = static_cast<int64_t>(year) + p.years;
    if (y2 < static_cast<int64_t>(std::numeric_limits<int>::min()) ||
        y2 > static_cast<int64_t>(std::numeric_limits<int>::max())) {
        q_date_fail("date is outside the range of dates");
    }
    year = static_cast<int>(y2);
    const int64_t total = static_cast<int64_t>(year) * 12 + static_cast<int64_t>(month - 1) + p.months;
    int64_t ny = total / 12;
    int64_t nm = total % 12;
    if (nm < 0) {
        nm += 12;
        ny -= 1;
    }
    if (ny < static_cast<int64_t>(std::numeric_limits<int>::min()) ||
        ny > static_cast<int64_t>(std::numeric_limits<int>::max())) {
        q_date_fail("date is outside the range of dates");
    }
    year = static_cast<int>(ny);
    month = static_cast<int>(nm) + 1;
    const int dim = q_days_in_month(year, month);
    day = orig > dim ? dim : orig;
    return q_date_add_days(q_date_fit(q_days_from_civil(year, month, day)), p.days);
}

inline int32_t q_date_add_period(int32_t days, QValue period) {
    return q_date_add_period_raw(days, *q_period_get(period));
}

inline int32_t q_date_sub_period(int32_t days, QValue period) {
    const QPeriod* p = q_period_get(period);
    if (p->years == std::numeric_limits<int32_t>::min() || p->months == std::numeric_limits<int32_t>::min() ||
        p->days == std::numeric_limits<int32_t>::min()) {
        q_date_fail("period is too large");
    }
    QPeriod neg{-p->years, -p->months, -p->days};
    return q_date_add_period_raw(days, neg);
}

inline int32_t q_period_fit(int64_t n) {
    if (n < static_cast<int64_t>(std::numeric_limits<int32_t>::min()) ||
        n > static_cast<int64_t>(std::numeric_limits<int32_t>::max())) {
        q_date_fail("period is too large");
    }
    return static_cast<int32_t>(n);
}

inline QValue q_period_add(QValue a, QValue b) {
    const QPeriod* x = q_period_get(a);
    const QPeriod* y = q_period_get(b);
    return qv_period(q_period_fit(static_cast<int64_t>(x->years) + y->years),
                     q_period_fit(static_cast<int64_t>(x->months) + y->months),
                     q_period_fit(static_cast<int64_t>(x->days) + y->days));
}

inline QValue q_period_sub(QValue a, QValue b) {
    const QPeriod* y = q_period_get(b);
    if (y->years == std::numeric_limits<int32_t>::min() || y->months == std::numeric_limits<int32_t>::min() ||
        y->days == std::numeric_limits<int32_t>::min()) {
        q_date_fail("period is too large");
    }
    return q_period_add(a, qv_period(-y->years, -y->months, -y->days));
}

inline QValue q_period_scale(QValue period, long long n) {
    const QPeriod* p = q_period_get(period);
    return qv_period(q_period_fit(static_cast<int64_t>(p->years) * n),
                     q_period_fit(static_cast<int64_t>(p->months) * n),
                     q_period_fit(static_cast<int64_t>(p->days) * n));
}

inline void q_format_date(int32_t days, char* buf, size_t n) {
    int year = 0, month = 0, day = 0;
    q_civil_from_days(days, year, month, day);
    if (year >= 0 && year <= 9999) {
        std::snprintf(buf, n, "%04d-%02d-%02d", year, month, day);
    } else {
        std::snprintf(buf, n, "%d-%02d-%02d", year, month, day);
    }
}

inline std::string q_format_period(const QPeriod& p) {
    std::string out;
    auto add = [&](int32_t n, const char* one, const char* many) {
        if (n == 0) return;
        if (!out.empty()) out += ' ';
        out += std::to_string(n);
        out += ' ';
        out += n == 1 ? one : many;
    };
    add(p.years, "year", "years");
    add(p.months, "month", "months");
    add(p.days, "day", "days");
    if (out.empty()) return "0 days";
    return out;
}

// ISO-like year-month-day. seps is the set of accepted separators (CSV uses
// "-", ymd() also accepts '/' and '.'). Unpadded month and day are accepted.
// The whole string must be the date, aside from surrounding spaces.
inline bool q_parse_ymd(const char* text, const char* seps, int32_t* out) {
    if (text == nullptr) return false;
    const char* s = text;
    while (*s == ' ' || *s == '\t') s++;
    int sign = 1;
    if (*s == '-') {
        sign = -1;
        s++;
    }
    if (*s < '0' || *s > '9') return false;
    int64_t year = 0;
    int nd = 0;
    while (*s >= '0' && *s <= '9') {
        year = year * 10 + (*s - '0');
        if (year > 1000000) return false;
        nd++;
        s++;
    }
    if (nd == 0) return false;
    year *= sign;
    if (*s == '\0' || std::strchr(seps, *s) == nullptr) return false;
    const char sep = *s++;
    auto piece = [&](int& n) -> bool {
        if (*s < '0' || *s > '9') return false;
        n = 0;
        int k = 0;
        while (*s >= '0' && *s <= '9') {
            n = n * 10 + (*s - '0');
            if (++k > 2 || n > 99) return false;
            s++;
        }
        return k > 0;
    };
    int month = 0, day = 0;
    if (!piece(month) || *s != sep) return false;
    s++;
    if (!piece(day)) return false;
    while (*s == ' ' || *s == '\t') s++;
    if (*s != '\0') return false;
    if (year < -999999 || year > 999999 || month < 1 || month > 12 || day < 1 || day > q_days_in_month(static_cast<int>(year), month)) {
        return false;
    }
    *out = q_date_fit(q_days_from_civil(static_cast<int>(year), month, day));
    return true;
}

// Boxed operator fallbacks, for a value whose type the compiler doesn't know.
inline QValue q_temporal_add(QValue a, QValue b) {
    if (a.type == QValue::VAL_DATE && b.type == QValue::VAL_PERIOD) return qv_date(q_date_add_period(q_as_date(a), b));
    if (a.type == QValue::VAL_PERIOD && b.type == QValue::VAL_DATE) return qv_date(q_date_add_period(q_as_date(b), a));
    if (a.type == QValue::VAL_DATE && b.type == QValue::VAL_INT) return qv_date(q_date_add_days(q_as_date(a), b.data.int_val));
    if (a.type == QValue::VAL_INT && b.type == QValue::VAL_DATE) return qv_date(q_date_add_days(q_as_date(b), a.data.int_val));
    if (a.type == QValue::VAL_PERIOD && b.type == QValue::VAL_PERIOD) return q_period_add(a, b);
    q_runtime_reportf("runtime error: operator '+' cannot add %s and %s\n", q_type_name(a.type), q_type_name(b.type));
    q_exit(1);
}

inline QValue q_temporal_sub(QValue a, QValue b) {
    if (a.type == QValue::VAL_DATE && b.type == QValue::VAL_PERIOD) return qv_date(q_date_sub_period(q_as_date(a), b));
    if (a.type == QValue::VAL_DATE && b.type == QValue::VAL_INT) {
        if (b.data.int_val == std::numeric_limits<long long>::min()) q_date_fail("date is outside the range of dates");
        return qv_date(q_date_add_days(q_as_date(a), -b.data.int_val));
    }
    if (a.type == QValue::VAL_DATE && b.type == QValue::VAL_DATE) return qv_int(q_date_diff(q_as_date(a), q_as_date(b)));
    if (a.type == QValue::VAL_PERIOD && b.type == QValue::VAL_PERIOD) return q_period_sub(a, b);
    q_runtime_reportf("runtime error: operator '-' cannot subtract %s from %s\n", q_type_name(b.type), q_type_name(a.type));
    q_exit(1);
}

inline QValue q_temporal_mul(QValue a, QValue b) {
    if (a.type == QValue::VAL_PERIOD && b.type == QValue::VAL_INT) return q_period_scale(a, b.data.int_val);
    if (a.type == QValue::VAL_INT && b.type == QValue::VAL_PERIOD) return q_period_scale(b, a.data.int_val);
    q_runtime_reportf("runtime error: operator '*' cannot multiply %s and %s\n", q_type_name(a.type), q_type_name(b.type));
    q_exit(1);
}

inline bool q_temporal_ordered(QValue a, QValue b, const char* op) {
    if (a.type != QValue::VAL_DATE || b.type != QValue::VAL_DATE) {
        q_runtime_reportf("runtime error: operator '%s' expects two dates, two numbers or two strings, got %s and %s\n",
                          op, q_type_name(a.type), q_type_name(b.type));
        q_exit(1);
    }
    const long long x = a.data.int_val, y = b.data.int_val;
    const int c = x < y ? -1 : (x > y ? 1 : 0);
    switch (op[0]) {
        case '<': return op[1] == '=' ? c <= 0 : c < 0;
        case '>': return op[1] == '=' ? c >= 0 : c > 0;
        default: return false;
    }
}

#endif // QUARK_TYPES_DATE_HPP
