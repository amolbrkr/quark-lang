// quark/core/format.hpp - text form of floats, shared by every place that
// prints one (println, to_str, std/fmt, error messages).
#ifndef QUARK_CORE_FORMAT_HPP
#define QUARK_CORE_FORMAT_HPP

#include <cmath>
#include <cstdio>
#include <cstdlib>
#include <cstring>

// q_format_float writes the shortest text that reads back as exactly `v`, laid
// out like Python's repr: plain notation for exponents -4..15 with ".0" on
// integral values (4.0, 0.1, 3.14159265, 1000000000000000.0), scientific
// otherwise with at least two exponent digits (1e+16, 1.5e-05), plus inf,
// -inf, nan and -0.0. `out` needs at least 32 bytes. Returns `out`.
inline char* q_format_float(double v, char* out, size_t size) {
    if (std::isnan(v)) {
        std::snprintf(out, size, "nan");
        return out;
    }
    if (std::isinf(v)) {
        std::snprintf(out, size, v < 0 ? "-inf" : "inf");
        return out;
    }

    // Shortest round-trip digits: a double needs at most 17 significant digits,
    // and if a normal double reads back from k <= 15 digits, the 15-digit form
    // is those k digits followed by zeros, so trying 15, 16, 17 and stripping
    // trailing zeros finds the shortest.
    // Subnormals carry fewer significant bits, so search them from one digit.
    char sci[40];
    const bool subnormal = v != 0 && std::fabs(v) < 2.2250738585072014e-308;
    for (int prec = subnormal ? 0 : 14; prec <= 16; prec++) {
        std::snprintf(sci, sizeof(sci), "%.*e", prec, v);
        if (std::strtod(sci, nullptr) == v) {
            break;
        }
    }
    // sci is [-]d.ddddde[+-]xx
    const bool neg = sci[0] == '-';
    const char* p = neg ? sci + 1 : sci;
    char digits[24];
    int nd = 0;
    for (; *p && *p != 'e'; p++) {
        if (*p >= '0' && *p <= '9' && nd < 23) {
            digits[nd++] = *p;
        }
    }
    while (nd > 1 && digits[nd - 1] == '0') {
        nd--;
    }
    digits[nd] = '\0';
    const int exp = (*p == 'e') ? std::atoi(p + 1) : 0;

    char* o = out;
    char* end = out + size - 1;
    auto put = [&](char c) {
        if (o < end) *o++ = c;
    };
    if (neg) put('-');
    if (exp >= -4 && exp < 16) {
        if (exp >= 0) {
            // integer part: digits[0..exp], padded with zeros
            for (int i = 0; i <= exp; i++) put(i < nd ? digits[i] : '0');
            put('.');
            if (nd > exp + 1) {
                for (int i = exp + 1; i < nd; i++) put(digits[i]);
            } else {
                put('0');
            }
        } else {
            put('0');
            put('.');
            for (int i = 0; i < -exp - 1; i++) put('0');
            for (int i = 0; i < nd; i++) put(digits[i]);
        }
    } else {
        put(digits[0]);
        if (nd > 1) {
            put('.');
            for (int i = 1; i < nd; i++) put(digits[i]);
        }
        char ebuf[8];
        std::snprintf(ebuf, sizeof(ebuf), "e%c%02d", exp < 0 ? '-' : '+', exp < 0 ? -exp : exp);
        for (char* e = ebuf; *e; e++) put(*e);
    }
    *o = '\0';
    return out;
}

#endif // QUARK_CORE_FORMAT_HPP
