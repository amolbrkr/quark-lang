// stdlib/time.hpp — native implementations for the std/time module (time.qrk).
#ifndef QUARK_STDLIB_TIME_HPP
#define QUARK_STDLIB_TIME_HPP

#include <chrono>
#include <cmath>
#include <cstdint>
#include <ctime>
#include <iomanip>
#include <locale>
#include <sstream>
#include <string>
#include <thread>

// Seconds since the Unix epoch (wall clock), with sub-second precision.
inline double q_time_now() {
    using namespace std::chrono;
    return duration<double>(system_clock::now().time_since_epoch()).count();
}

// Seconds from an arbitrary fixed point (monotonic); use differences to time code.
inline double q_time_clock() {
    using namespace std::chrono;
    return duration<double>(steady_clock::now().time_since_epoch()).count();
}

// Pause for the given number of seconds (fractions allowed; negative is 0).
inline QValue q_time_sleep(double seconds) {
    if (seconds > 0) {
        std::this_thread::sleep_for(std::chrono::duration<double>(seconds));
    }
    return qv_null();
}

// A broken-down UTC or local time. weekday is 0 = Monday .. 6 = Sunday.
// yearday is 1 = 1 January. second is in [0, 60) and may be fractional.
struct QTimeParts {
    int year = 0;
    int month = 0;
    int day = 0;
    int hour = 0;
    int minute = 0;
    int weekday = 0;
    int yearday = 0;
    double second = 0;
};

// Years the conversions promise. A double holds whole seconds exactly across
// this span, and the civil-date arithmetic below stays inside 32-bit years.
constexpr int q_time_year_min = -999999;
constexpr int q_time_year_max = 999999;

[[noreturn]] inline void q_time_fail(const char* fn, const char* msg) {
    q_runtime_reportf("runtime error: time::%s() %s\n", fn, msg);
    std::exit(1);
}

inline const char* q_time_copy(const std::string& s) {
    return q_strdup(s.c_str());
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

// A real calendar day. An impossible day (31 February) does not round-trip
// through the civil conversion, so it is rejected rather than normalized.
inline bool q_time_valid_date(int year, int month, int day) {
    if (year < q_time_year_min || year > q_time_year_max) return false;
    if (month < 1 || month > 12 || day < 1 || day > 31) return false;
    int y = 0, m = 0, d = 0;
    q_civil_from_days(q_days_from_civil(year, month, day), y, m, d);
    return y == year && m == month && d == day;
}

inline bool q_time_valid_clock(int hour, int minute, double second) {
    return hour >= 0 && hour <= 23 && minute >= 0 && minute <= 59 &&
           std::isfinite(second) && second >= 0.0 && second < 60.0;
}

// Split a finite timestamp into whole seconds and a fraction in [0, 1).
// A negative fraction is borrowed from the next second so the fraction is
// never negative (-1.5 → whole -2, fraction 0.5).
inline bool q_time_split(double ts, int64_t& whole, double& frac) {
    if (!std::isfinite(ts) || std::fabs(ts) > 3.2e13) return false;
    double ip = 0;
    frac = std::modf(ts, &ip);
    if (frac < 0) {
        frac += 1.0;
        ip -= 1.0;
    }
    whole = static_cast<int64_t>(ip);
    return true;
}

inline bool q_time_break_utc(double ts, QTimeParts& out) {
    int64_t sec = 0;
    double frac = 0;
    if (!q_time_split(ts, sec, frac)) return false;
    int64_t days = sec / 86400;
    int64_t sod = sec % 86400;
    if (sod < 0) {
        sod += 86400;
        days -= 1;
    }
    int year = 0, month = 0, day = 0;
    q_civil_from_days(days, year, month, day);
    if (year < q_time_year_min || year > q_time_year_max) return false;
    out.year = year;
    out.month = month;
    out.day = day;
    out.hour = static_cast<int>(sod / 3600);
    out.minute = static_cast<int>((sod % 3600) / 60);
    out.second = static_cast<double>(sod % 60) + frac;
    int64_t wd = (days + 3) % 7; // 1970-01-01 was a Thursday; Monday = 0
    if (wd < 0) wd += 7;
    out.weekday = static_cast<int>(wd);
    out.yearday = static_cast<int>(days - q_days_from_civil(year, 1, 1) + 1);
    return true;
}

inline bool q_time_local_tm(double ts, std::tm& tm, double& frac) {
    int64_t sec = 0;
    if (!q_time_split(ts, sec, frac)) return false;
    time_t t = static_cast<time_t>(sec);
    if (static_cast<int64_t>(t) != sec) return false;
#ifdef _WIN32
    if (localtime_s(&tm, &t) != 0) return false;
#else
    if (localtime_r(&t, &tm) == nullptr) return false;
#endif
    return true;
}

inline bool q_time_break_local(double ts, QTimeParts& out) {
    std::tm tm{};
    double frac = 0;
    if (!q_time_local_tm(ts, tm, frac)) return false;
    out.year = tm.tm_year + 1900;
    out.month = tm.tm_mon + 1;
    out.day = tm.tm_mday;
    out.hour = tm.tm_hour;
    out.minute = tm.tm_min;
    out.second = static_cast<double>(tm.tm_sec) + frac;
    out.weekday = (tm.tm_wday + 6) % 7; // tm_wday 0 = Sunday
    out.yearday = tm.tm_yday + 1;
    return out.year >= q_time_year_min && out.year <= q_time_year_max;
}

inline bool q_time_break(double ts, bool local, QTimeParts& out) {
    return local ? q_time_break_local(ts, out) : q_time_break_utc(ts, out);
}

// UTC timestamp for a valid civil time. local uses mktime and rejects a time
// the zone does not have (a spring-forward gap) instead of shifting it.
inline bool q_time_compose(const QTimeParts& p, bool local, double& out) {
    if (!q_time_valid_date(p.year, p.month, p.day) || !q_time_valid_clock(p.hour, p.minute, p.second)) {
        return false;
    }
    double sec_whole = 0;
    double frac = std::modf(p.second, &sec_whole);
    const int sec = static_cast<int>(sec_whole);
    if (!local) {
        const int64_t days = q_days_from_civil(p.year, p.month, p.day);
        const int64_t sod = static_cast<int64_t>(p.hour) * 3600 + static_cast<int64_t>(p.minute) * 60 + sec;
        out = static_cast<double>(days) * 86400.0 + static_cast<double>(sod) + frac;
        return true;
    }
    const long long tm_year = static_cast<long long>(p.year) - 1900;
    if (tm_year < -2147483647LL - 1 || tm_year > 2147483647LL) return false;
    std::tm tm{};
    tm.tm_year = static_cast<int>(tm_year);
    tm.tm_mon = p.month - 1;
    tm.tm_mday = p.day;
    tm.tm_hour = p.hour;
    tm.tm_min = p.minute;
    tm.tm_sec = sec;
    tm.tm_isdst = -1;
    const time_t t = std::mktime(&tm);
    // mktime returns -1 both for a real time one second before the epoch and
    // for a time it cannot represent, and on failure it may leave tm as we
    // set it. Read the result back; only a time whose local fields match is
    // accepted, so a failure is not reported as the timestamp -1.
    std::tm back{};
#ifdef _WIN32
    if (localtime_s(&back, &t) != 0) return false;
#else
    if (localtime_r(&t, &back) == nullptr) return false;
#endif
    if (back.tm_year != static_cast<int>(tm_year) || back.tm_mon != p.month - 1 || back.tm_mday != p.day ||
        back.tm_hour != p.hour || back.tm_min != p.minute || back.tm_sec != sec) {
        return false;
    }
    out = static_cast<double>(t) + frac;
    return true;
}

inline std::string q_time_number(double v) {
    const double rounded = std::round(v);
    if (std::fabs(v - rounded) < 1e-9 && std::fabs(rounded) < 9223372036854775000.0) {
        return std::to_string(static_cast<long long>(rounded));
    }
    char buf[32];
    q_format_float(v, buf, sizeof(buf));
    return buf;
}

// Human duration: 0.25 → "250ms", 75.5 → "1m 15.5s", 93784 → "1d 2h 3m 4s".
// Zero components are left out. A negative duration keeps the sign.
inline const char* q_time_duration(double seconds) {
    if (!std::isfinite(seconds)) {
        q_time_fail("duration", "seconds must be a finite number");
    }
    const bool neg = seconds < 0;
    double a = std::fabs(seconds);
    if (a < 1e-12) a = 0;
    std::string out = (neg && a != 0) ? "-" : "";
    if (a < 1.0 - 1e-9) {
        out += q_time_number(a * 1000.0);
        out += "ms";
        return q_time_copy(out);
    }
    if (a > 9223372036854775000.0) {
        q_time_fail("duration", "seconds is too large");
    }
    double whole = 0;
    double frac = std::modf(a, &whole);
    if (frac >= 1.0 - 1e-9) {
        whole += 1.0;
        frac = 0;
    } else if (frac < 1e-9) {
        frac = 0;
    }
    const int64_t total = static_cast<int64_t>(whole);
    const int64_t days = total / 86400;
    const int64_t hours = (total % 86400) / 3600;
    const int64_t mins = (total % 3600) / 60;
    const double secs = static_cast<double>(total % 60) + frac;
    bool wrote = false;
    auto add_int = [&](int64_t n, const char* unit) {
        if (n == 0) return;
        if (wrote) out += ' ';
        out += std::to_string(n);
        out += unit;
        wrote = true;
    };
    add_int(days, "d");
    add_int(hours, "h");
    add_int(mins, "m");
    if (secs != 0.0 || !wrote) {
        if (wrote) out += ' ';
        out += q_time_number(secs);
        out += 's';
    }
    return q_time_copy(out);
}

inline std::string q_time_strftime(const std::tm& tm, const char* pattern) {
    if (pattern == nullptr || pattern[0] == '\0') return "";
    size_t n = 128;
    while (n <= 1024u * 1024u) {
        std::string buf(n, '\0');
        const size_t wrote = std::strftime(&buf[0], n, pattern, &tm);
        if (wrote > 0) return buf.substr(0, wrote);
        n *= 2;
    }
    q_time_fail("format", "pattern produced no output, or more than 1MB of it");
}

inline void q_time_fill_tm(const QTimeParts& p, std::tm& tm) {
    tm = std::tm{};
    tm.tm_year = p.year - 1900;
    tm.tm_mon = p.month - 1;
    tm.tm_mday = p.day;
    tm.tm_hour = p.hour;
    tm.tm_min = p.minute;
    tm.tm_sec = static_cast<int>(p.second);
    tm.tm_wday = (p.weekday + 1) % 7; // struct tm: 0 = Sunday
    tm.tm_yday = p.yearday - 1;
    tm.tm_isdst = 0;
}

inline const char* q_time_format(double ts, const char* pattern, bool local) {
    std::tm tm{};
    if (local) {
        double frac = 0;
        if (!q_time_local_tm(ts, tm, frac)) {
            q_time_fail("format", "timestamp is not a finite time it can show");
        }
    } else {
        QTimeParts p;
        if (!q_time_break_utc(ts, p)) {
            q_time_fail("format", "timestamp is not a finite time it can show");
        }
        q_time_fill_tm(p, tm);
    }
    return q_time_copy(q_time_strftime(tm, pattern));
}

// Fractional seconds as ".5" or ".25", up to microseconds, or "" for a whole
// second. A fraction that rounds up to the next second sets carry.
inline std::string q_time_fraction(double second, bool& carry) {
    carry = false;
    double whole = 0;
    const double frac = std::modf(second, &whole);
    long long micros = std::llround(frac * 1000000.0);
    if (micros >= 1000000) {
        carry = true;
        return "";
    }
    if (micros <= 0) return "";
    char buf[16];
    std::snprintf(buf, sizeof(buf), "%06lld", micros);
    int end = 5;
    while (end > 0 && buf[end] == '0') end--;
    buf[end + 1] = '\0';
    return std::string(".") + buf;
}

inline const char* q_time_iso(double ts) {
    QTimeParts p;
    if (!q_time_break_utc(ts, p)) {
        q_time_fail("iso", "timestamp is not a finite time it can show");
    }
    bool carry = false;
    std::string frac = q_time_fraction(p.second, carry);
    if (carry) {
        double whole = 0;
        const double f = std::modf(p.second, &whole);
        if (!q_time_break_utc(ts + (1.0 - f), p)) {
            q_time_fail("iso", "timestamp is not a finite time it can show");
        }
        frac.clear();
    }
    char buf[64];
    std::snprintf(buf, sizeof(buf), "%04d-%02d-%02dT%02d:%02d:%02d", p.year, p.month, p.day, p.hour, p.minute,
                  static_cast<int>(p.second));
    std::string text = buf;
    // %04d of a negative year already includes the sign ("-001").
    text += frac;
    text += 'Z';
    return q_time_copy(text);
}

inline std::string q_time_shown(const char* text) {
    std::string shown = text ? text : "";
    if (shown.size() > 120) shown = shown.substr(0, 120) + "...";
    return shown;
}

inline QValue q_time_parse_err(const char* text, const std::string& how) {
    std::string msg = "cannot parse '";
    msg += q_time_shown(text);
    msg += "' ";
    msg += how;
    return qv_err(qv_string(msg.c_str()));
}

inline QValue q_time_parse(const char* text, const char* pattern, bool local) {
    if (pattern == nullptr || pattern[0] == '\0') {
        q_time_fail("parse", "pattern must not be empty");
    }
    if (text == nullptr) text = "";
    std::tm tm{};
    std::istringstream in(text);
    in.imbue(std::locale::classic());
    in >> std::get_time(&tm, pattern);
    if (in.fail()) return q_time_parse_err(text, std::string("with pattern '") + pattern + "'");
    char extra = 0;
    if (in >> extra) return q_time_parse_err(text, std::string("with pattern '") + pattern + "'");
    QTimeParts p;
    p.year = tm.tm_year + 1900;
    p.month = tm.tm_mon + 1;
    p.day = tm.tm_mday;
    p.hour = tm.tm_hour;
    p.minute = tm.tm_min;
    p.second = static_cast<double>(tm.tm_sec);
    double ts = 0;
    if (!q_time_compose(p, local, ts)) {
        return q_time_parse_err(text, std::string("with pattern '") + pattern + "'");
    }
    return qv_ok(qv_float(ts));
}

// ISO 8601: YYYY-MM-DD, optional T (or space) and HH:MM:SS, optional fraction,
// optional Z or ±hh:mm. A missing zone is UTC. The whole string must match.
inline QValue q_time_parse_iso(const char* text) {
    const char* s = text ? text : "";
    const char* const start = s;
    auto digits = [&](int n, int& out) -> bool {
        int v = 0;
        for (int i = 0; i < n; i++) {
            if (*s < '0' || *s > '9') return false;
            v = v * 10 + (*s - '0');
            s++;
        }
        out = v;
        return true;
    };
    int year = 0, month = 0, day = 0;
    if (!digits(4, year) || *s++ != '-' || !digits(2, month) || *s++ != '-' || !digits(2, day)) {
        return q_time_parse_err(start, "as ISO 8601");
    }
    int hour = 0, minute = 0, second = 0;
    double frac = 0;
    if (*s == 'T' || *s == 't' || *s == ' ') {
        s++;
        if (!digits(2, hour) || *s++ != ':' || !digits(2, minute) || *s++ != ':' || !digits(2, second)) {
            return q_time_parse_err(start, "as ISO 8601");
        }
        if (*s == '.') {
            s++;
            if (*s < '0' || *s > '9') return q_time_parse_err(start, "as ISO 8601");
            double scale = 0.1;
            int n = 0;
            while (*s >= '0' && *s <= '9') {
                if (n < 12) frac += (*s - '0') * scale;
                scale *= 0.1;
                n++;
                s++;
            }
        }
    }
    int off = 0;
    if (*s == 'Z' || *s == 'z') {
        s++;
    } else if (*s == '+' || *s == '-') {
        const int sign = *s == '+' ? 1 : -1;
        s++;
        int oh = 0, om = 0;
        if (!digits(2, oh) || *s++ != ':' || !digits(2, om) || oh > 23 || om > 59) {
            return q_time_parse_err(start, "as ISO 8601");
        }
        off = sign * (oh * 3600 + om * 60);
    }
    if (*s != '\0') return q_time_parse_err(start, "as ISO 8601");
    QTimeParts p;
    p.year = year;
    p.month = month;
    p.day = day;
    p.hour = hour;
    p.minute = minute;
    p.second = static_cast<double>(second) + frac;
    double ts = 0;
    if (!q_time_compose(p, false, ts)) return q_time_parse_err(start, "as ISO 8601");
    return qv_ok(qv_float(ts - static_cast<double>(off)));
}

// Eight values, in order: year, month, day, hour, minute, second, weekday,
// yearday. time::parts builds a time::Parts from them. The list is untyped
// because second is a float and the rest are ints.
inline QList* q_time_parts(double ts, bool local) {
    QTimeParts p;
    if (!q_time_break(ts, local, p)) {
        q_time_fail("parts", "timestamp is not a finite time it can show");
    }
    QValue list = qv_list(8, nullptr);
    QList* xs = list.data.list_val;
    xs->push_back(qv_int(p.year));
    xs->push_back(qv_int(p.month));
    xs->push_back(qv_int(p.day));
    xs->push_back(qv_int(p.hour));
    xs->push_back(qv_int(p.minute));
    xs->push_back(qv_float(p.second));
    xs->push_back(qv_int(p.weekday));
    xs->push_back(qv_int(p.yearday));
    return xs;
}

inline double q_time_make(int64_t year, int64_t month, int64_t day, int64_t hour, int64_t minute, double second, bool local) {
    if (year < q_time_year_min || year > q_time_year_max) {
        q_time_fail("make", "year is out of range");
    }
    QTimeParts p;
    p.year = static_cast<int>(year);
    p.month = static_cast<int>(month);
    p.day = static_cast<int>(day);
    p.hour = static_cast<int>(hour);
    p.minute = static_cast<int>(minute);
    p.second = second;
    // The casts above truncate a value that does not fit in int. Reject those
    // before compose, so a huge month is an error and not a truncated one.
    if (!std::isfinite(second)) {
        q_time_fail("make", "second must be a finite number");
    }
    if (month != p.month || day != p.day || hour != p.hour || minute != p.minute) {
        q_time_fail("make", "date is not a real calendar time");
    }
    double ts = 0;
    if (!q_time_compose(p, local, ts)) {
        char secbuf[32];
        q_format_float(p.second, secbuf, sizeof(secbuf));
        char buf[160];
        std::snprintf(buf, sizeof(buf), "date is not a real calendar time (%d-%02d-%02d %02d:%02d:%s)", p.year, p.month,
                      p.day, p.hour, p.minute, secbuf);
        q_time_fail("make", buf);
    }
    return ts;
}

inline double q_time_today(bool local) {
    QTimeParts p;
    if (!q_time_break(q_time_now(), local, p)) {
        q_time_fail("today", "could not read the clock");
    }
    p.hour = 0;
    p.minute = 0;
    p.second = 0;
    double ts = 0;
    if (!q_time_compose(p, local, ts)) {
        q_time_fail("today", "could not find midnight");
    }
    return ts;
}

#endif // QUARK_STDLIB_TIME_HPP
