// stdlib/csv.hpp — native implementations for the std/csv module (csv.qrk).
// Parses RFC 4180 CSV: fields separated by one character, optionally quoted
// with "...", where "" inside quotes is a literal quote and quoted fields may
// contain separators and newlines. Lines end with \n or \r\n; fully blank
// lines are skipped. Every field is returned as a str.
#ifndef QUARK_STDLIB_CSV_HPP
#define QUARK_STDLIB_CSV_HPP

#include <cerrno>
#include <cstdio>
#include <cstring>
#include <string>

// Parses text into a list of rows (each a list of str). On malformed input
// returns nullptr and sets *error.
inline QList* q_csv_parse_text(const char* text, size_t len, char sep, std::string* error) {
    QValue rows = qv_list();
    QValue row = qv_list();
    std::string field;
    bool inQuotes = false;
    bool quoted = false;   // current field started with a quote
    bool rowHasData = false;
    int line = 1;
    int quoteLine = 0;

    auto endField = [&]() {
        row.data.list_val->push_back(qv_string(field.c_str()));
        field.clear();
        quoted = false;
    };
    auto endRow = [&]() {
        endField();
        // Skip fully blank lines (one empty, unquoted field).
        if (rowHasData) {
            rows.data.list_val->push_back(row);
        }
        row = qv_list();
        rowHasData = false;
    };

    for (size_t i = 0; i < len; i++) {
        char c = text[i];
        if (inQuotes) {
            if (c == '"') {
                if (i + 1 < len && text[i + 1] == '"') {
                    field.push_back('"');
                    i++;
                } else {
                    inQuotes = false;
                }
            } else {
                if (c == '\n') line++;
                field.push_back(c);
            }
            continue;
        }
        if (c == '"' && field.empty() && !quoted) {
            inQuotes = true;
            quoted = true;
            rowHasData = true;
            quoteLine = line;
        } else if (c == sep) {
            endField();
            rowHasData = true;
        } else if (c == '\r' && i + 1 < len && text[i + 1] == '\n') {
            // handled by the following \n
        } else if (c == '\n') {
            endRow();
            line++;
        } else {
            field.push_back(c);
            rowHasData = true;
        }
    }
    if (inQuotes) {
        *error = "unterminated quoted field starting on line " + std::to_string(quoteLine);
        return nullptr;
    }
    if (rowHasData || !field.empty()) {
        endRow();
    }
    return rows.data.list_val;
}

inline char q_csv_separator(const char* sep, const char* fn) {
    if (!sep || std::strlen(sep) != 1) {
        q_runtime_reportf("runtime error: %s: separator must be a single character, got '%s'\n", fn, sep ? sep : "");
        std::exit(1);
    }
    return sep[0];
}

// csv.parse(text, sep): rows of a CSV string. Malformed input is a runtime error.
inline QList* q_csv_parse(const char* text, const char* sep) {
    char s = q_csv_separator(sep, "csv.parse");
    std::string error;
    QList* rows = q_csv_parse_text(text ? text : "", text ? std::strlen(text) : 0, s, &error);
    if (!rows) {
        q_runtime_reportf("runtime error: csv.parse: %s\n", error.c_str());
        std::exit(1);
    }
    return rows;
}

// csv.read(path, sep): ok(rows) for a CSV file, or err(message).
inline QValue q_csv_read(const char* path, const char* sep) {
    char s = q_csv_separator(sep, "csv.read");
    FILE* f = std::fopen(path, "rb");
    if (!f) {
        std::string msg = std::string("csv.read: ") + path + ": " + std::strerror(errno);
        return qv_err(qv_string(msg.c_str()));
    }
    std::string data;
    char buf[1 << 16];
    size_t n;
    while ((n = std::fread(buf, 1, sizeof(buf), f)) > 0) {
        data.append(buf, n);
    }
    std::fclose(f);
    std::string error;
    QList* rows = q_csv_parse_text(data.data(), data.size(), s, &error);
    if (!rows) {
        std::string msg = std::string("csv.read: ") + path + ": " + error;
        return qv_err(qv_string(msg.c_str()));
    }
    return qv_ok(qv_list_ptr(rows));
}

#endif // QUARK_STDLIB_CSV_HPP
