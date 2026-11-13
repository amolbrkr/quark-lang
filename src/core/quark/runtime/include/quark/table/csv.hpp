// quark/table/csv.hpp - CSV / TSV reading into tables, and writing (tables v0.1)
//
// Reading parses straight into typed column buffers, so a file is read once
// and every value is converted once.
//
// Format accepted (RFC 4180 plus what real exports contain):
//   - one-character separator: ',' by default, '\t' for .tsv / .tab paths;
//   - fields optionally quoted with "...": a quoted field may contain the
//     separator, newlines, and "" for a literal quote; spaces around the
//     quotes are ignored;
//   - lines end with \n, \r\n or \r; a UTF-8 byte-order mark is skipped;
//   - empty and whitespace-only lines are skipped;
//   - unquoted fields are trimmed of spaces (and tabs, unless tab separates);
//   - "" and "NA" (quoted or not) are missing in every column, as in readr.
//
// Typed reads take a schema (column names and types). Columns are matched to
// the header by name, so a file may have more columns than the schema; a
// schema column missing from the header is an error. Int columns accept an
// optional sign and digits; float columns also accept decimals, exponents,
// Inf and NaN; bool columns accept true/false, TRUE/FALSE, True/False, T/F.
//
// Nothing is guessed silently: a row with the wrong number of fields, a value
// that doesn't parse as its column's type, an unterminated quote or text after
// a closing quote is an error naming the line, the column and the text. The
// message reports the first problem and how many more there are.
//
// Untyped reads infer each column's type from all of its values: int if every
// value is an int, else float if every value is a number, else bool, else str
// (a column with no values is str).
#ifndef QUARK_TABLE_CSV_HPP
#define QUARK_TABLE_CSV_HPP

#include "../core/format.hpp"
#include "engine.hpp"

#include <cerrno>
#include <charconv>
#include <cstdio>
#include <cstdlib>
#include <cstring>
#include <string>
#include <string_view>
#include <vector>

namespace quark {
namespace csv {

struct Options {
    char sep = 0;      // 0: '\t' for .tsv / .tab paths, else ','
    bool header = true;
    bool trim = true;
    std::vector<std::string> na = {"", "NA"};
};

struct Column {
    std::string name;
    QVector::Type type;
};

// ------------------------------------------------------------
// Tokenizer
// ------------------------------------------------------------

struct Field {
    std::string_view text;
    bool quoted = false;
};

class Tokenizer {
public:
    Tokenizer(const char* data, size_t len, char sep) : p_(data), end_(data + len), sep_(sep) {
        if (len >= 3 && static_cast<unsigned char>(data[0]) == 0xEF && static_cast<unsigned char>(data[1]) == 0xBB &&
            static_cast<unsigned char>(data[2]) == 0xBF) {
            p_ += 3;
        }
    }

    // next reads the next non-blank row into fields and its first line into
    // *row_line. Returns false at the end of input, or on malformed quoting,
    // after which failed() is true and error_line() / error() describe it.
    bool next(std::vector<Field>& fields, int* row_line) {
        while (true) {
            if (failed_ || p_ >= end_) return false;
            fields.clear();
            arena_.clear();
            spans_.clear();
            *row_line = line_;
            if (!read_row()) {
                failed_ = true;
                return false;
            }
            // Resolve fields that were unescaped into the arena.
            for (const Span& s : spans_) {
                Field f;
                f.quoted = s.quoted;
                f.text = s.in_arena ? std::string_view(arena_.data() + s.off, s.len) : std::string_view(s.ptr, s.len);
                fields.push_back(f);
            }
            if (fields.size() == 1 && !fields[0].quoted && is_blank(fields[0].text)) continue;
            return true;
        }
    }

    struct Span {
        const char* ptr = nullptr;
        size_t off = 0;
        size_t len = 0;
        bool quoted = false;
        bool in_arena = false;
    };

    bool is_space(char c) const { return c == ' ' || (c == '\t' && sep_ != '\t'); }

    bool is_blank(std::string_view s) const {
        for (char c : s) {
            if (!is_space(c)) return false;
        }
        return true;
    }

    // at_eol consumes a line ending at p_, if there is one.
    bool at_eol() {
        if (p_ >= end_) return true;
        if (*p_ == '\n') {
            p_++;
            line_++;
            return true;
        }
        if (*p_ == '\r') {
            p_++;
            if (p_ < end_ && *p_ == '\n') p_++;
            line_++;
            return true;
        }
        return false;
    }

    bool failed() const { return failed_; }
    int error_line() const { return error_line_; }
    const std::string& error() const { return error_; }

private:
    bool read_row() {
        while (true) {
            // Skip spaces before a field, to find an opening quote.
            const char* start = p_;
            while (p_ < end_ && is_space(*p_)) p_++;
            if (p_ < end_ && *p_ == '"') {
                if (!read_quoted()) return false;
            } else {
                p_ = start;
                const char* f = p_;
                while (p_ < end_ && *p_ != sep_ && *p_ != '\n' && *p_ != '\r') p_++;
                Span s;
                s.ptr = f;
                s.len = static_cast<size_t>(p_ - f);
                spans_.push_back(s);
            }
            if (p_ < end_ && *p_ == sep_) {
                p_++;
                continue;
            }
            at_eol();
            return true;
        }
    }

    bool read_quoted() {
        const int quote_line = line_;
        p_++; // opening quote
        const char* f = p_;
        bool escaped = false;
        size_t arena_start = arena_.size();
        while (true) {
            if (p_ >= end_) {
                error_line_ = quote_line;
                error_ = "a quoted field is never closed";
                return false;
            }
            const char c = *p_;
            if (c == '"') {
                if (p_ + 1 < end_ && p_[1] == '"') {
                    if (!escaped) {
                        arena_.append(f, static_cast<size_t>(p_ - f));
                        escaped = true;
                    }
                    arena_.push_back('"');
                    p_ += 2;
                    continue;
                }
                break;
            }
            if (c == '\n' || (c == '\r' && !(p_ + 1 < end_ && p_[1] == '\n'))) line_++;
            if (escaped) arena_.push_back(c);
            p_++;
        }
        Span s;
        s.quoted = true;
        if (escaped) {
            s.in_arena = true;
            s.off = arena_start;
            s.len = arena_.size() - arena_start;
        } else {
            s.ptr = f;
            s.len = static_cast<size_t>(p_ - f);
        }
        spans_.push_back(s);
        p_++; // closing quote
        while (p_ < end_ && is_space(*p_)) p_++;
        if (p_ < end_ && *p_ != sep_ && *p_ != '\n' && *p_ != '\r') {
            error_line_ = line_;
            error_ = "unexpected text after a closing quote";
            return false;
        }
        return true;
    }

    const char* p_;
    const char* end_;
    char sep_;
    int line_ = 1;
    bool failed_ = false;
    int error_line_ = 0;
    std::string error_;
    std::string arena_;
    std::vector<Span> spans_;
};

// ------------------------------------------------------------
// Value parsing
// ------------------------------------------------------------

inline std::string_view trim(std::string_view s, char sep) {
    auto space = [&](char c) { return c == ' ' || (c == '\t' && sep != '\t'); };
    while (!s.empty() && space(s.front())) s.remove_prefix(1);
    while (!s.empty() && space(s.back())) s.remove_suffix(1);
    return s;
}

inline bool parse_i64(std::string_view s, int64_t* out) {
    if (s.empty()) return false;
    size_t i = 0;
    bool neg = false;
    if (s[0] == '+' || s[0] == '-') {
        neg = s[0] == '-';
        i = 1;
    }
    if (i == s.size()) return false;
    uint64_t v = 0;
    const uint64_t limit = neg ? static_cast<uint64_t>(INT64_MAX) + 1 : static_cast<uint64_t>(INT64_MAX);
    for (; i < s.size(); i++) {
        const char c = s[i];
        if (c < '0' || c > '9') return false;
        const uint64_t d = static_cast<uint64_t>(c - '0');
        if (v > (limit - d) / 10) return false;
        v = v * 10 + d;
    }
    *out = neg ? static_cast<int64_t>(0 - v) : static_cast<int64_t>(v);
    return true;
}

inline bool parse_f64(std::string_view s, double* out) {
    if (s.empty() || s.size() > 512) return false;
#if defined(__cpp_lib_to_chars) && __cpp_lib_to_chars >= 201611L
    {
        // from_chars is locale-independent and much faster than strtod; it
        // takes no leading '+' and no hex. Out-of-range values fall through
        // to strtod, which tells underflow (accepted) from overflow.
        const char* b = s.data();
        const char* e = b + s.size();
        if (*b == '+') {
            b++;
            if (b == e || *b == '-' || *b == '+') return false;
        }
        double v = 0;
        const std::from_chars_result r = std::from_chars(b, e, v, std::chars_format::general);
        if (r.ec == std::errc() && r.ptr == e) {
            *out = v;
            return true;
        }
        if (r.ec != std::errc::result_out_of_range) return false;
    }
#endif
    // strtod accepts hex floats; CSV numbers are decimal.
    for (char c : s) {
        if (c == 'x' || c == 'X') return false;
    }
    char buf[513];
    std::memcpy(buf, s.data(), s.size());
    buf[s.size()] = '\0';
    char* endp = nullptr;
    errno = 0;
    const double v = std::strtod(buf, &endp);
    if (endp != buf + s.size()) return false;
    if (errno == ERANGE && std::isfinite(v) == false) {
        // Overflow to ±Inf is a value too large for a float.
        return false;
    }
    *out = v;
    return true;
}

inline bool parse_bool(std::string_view s, bool* out) {
    if (s == "true" || s == "TRUE" || s == "True" || s == "T") {
        *out = true;
        return true;
    }
    if (s == "false" || s == "FALSE" || s == "False" || s == "F") {
        *out = false;
        return true;
    }
    return false;
}

inline const char* type_word(QVector::Type t) {
    switch (t) {
        case QVector::Type::I64: return "an int";
        case QVector::Type::F64: return "a float";
        case QVector::Type::BOOL: return "a bool";
        case QVector::Type::STR: return "a str";
    }
    return "a value";
}

// ------------------------------------------------------------
// Column builders
// ------------------------------------------------------------

struct Builder {
    QVector::Type type = QVector::Type::STR;
    std::vector<int64_t> i64;
    std::vector<double> f64;
    std::vector<uint8_t> b;
    std::string bytes;
    std::vector<int32_t> offsets{0};
    std::vector<int32_t> nulls;
    int32_t count = 0;

    void push_null() {
        nulls.push_back(count);
        switch (type) {
            case QVector::Type::I64: i64.push_back(0); break;
            case QVector::Type::F64: f64.push_back(0); break;
            case QVector::Type::BOOL: b.push_back(0); break;
            case QVector::Type::STR: offsets.push_back(static_cast<int32_t>(bytes.size())); break;
        }
        count++;
    }

    QVector* finish(std::string* error) {
        QVector* v = nullptr;
        const size_t n = static_cast<size_t>(count);
        switch (type) {
            case QVector::Type::I64:
                v = q_vec_alloc(type, n);
                if (n) std::memcpy(v->values, i64.data(), n * 8);
                break;
            case QVector::Type::F64:
                v = q_vec_alloc(type, n);
                if (n) std::memcpy(v->values, f64.data(), n * 8);
                break;
            case QVector::Type::BOOL:
                v = q_vec_alloc(type, n);
                for (size_t i = 0; i < n; i++) {
                    if (b[i]) quark::vec::put_bit(v->values, i, true);
                }
                break;
            case QVector::Type::STR:
                if (bytes.size() > static_cast<size_t>(INT32_MAX)) {
                    *error = "a str column holds more than 2 GiB of text";
                    return nullptr;
                }
                v = q_vec_alloc(type, n, bytes.size());
                if (!bytes.empty()) std::memcpy(v->values, bytes.data(), bytes.size());
                std::memcpy(v->offsets, offsets.data(), (n + 1) * sizeof(int32_t));
                break;
        }
        for (int32_t i : nulls) quark::tbl::set_null(*v, i);
        return v;
    }
};

// ------------------------------------------------------------
// Reading
// ------------------------------------------------------------

inline char separator_for(const char* path, const Options& opt) {
    if (opt.sep != 0) return opt.sep;
    if (path) {
        const std::string_view p(path);
        auto ends = [&](std::string_view suf) {
            return p.size() >= suf.size() && p.compare(p.size() - suf.size(), suf.size(), suf) == 0;
        };
        if (ends(".tsv") || ends(".TSV") || ends(".tab") || ends(".TAB")) return '\t';
    }
    return ',';
}

class Problems {
public:
    explicit Problems(std::string source) : source_(std::move(source)) {}
    void add(int line, const std::string& msg) {
        if (count_ == 0) first_ = source_ + ":" + std::to_string(line) + ": " + msg;
        count_++;
    }
    bool any() const { return count_ > 0; }
    std::string message() const {
        if (count_ <= 1) return first_;
        return first_ + " (and " + std::to_string(count_ - 1) + " more problem" + (count_ == 2 ? "" : "s") + ")";
    }

private:
    std::string source_;
    std::string first_;
    int count_ = 0;
};

inline std::string quoted_list(const std::vector<std::string>& names) {
    std::string out;
    for (size_t i = 0; i < names.size(); i++) {
        if (i) out += ", ";
        out += names[i];
    }
    return out;
}

// parse_table parses text into a table. With schema == nullptr, column types
// are inferred and def is built from the header; otherwise def names the
// schema's columns in order and schema gives their types. source names the
// input in messages. Returns nullptr and sets *error on failure.
inline QTable* parse_table(const char* text, size_t len, const std::string& source, char sep, const Options& opt,
                           const QStructDef* def, const std::vector<Column>* schema, std::string* error) {
    Tokenizer tok(text, len, sep);
    std::vector<Field> fields;
    int line = 0;
    auto tokenizer_error = [&](const Tokenizer& t) {
        return source + ":" + std::to_string(t.error_line()) + ": " + t.error();
    };

    // The header, or (without one) the first row, sets the number of fields.
    std::vector<std::string> header;
    bool have_first = false;
    if (tok.next(fields, &line)) {
        if (opt.header) {
            for (const Field& f : fields) header.emplace_back(trim(f.text, sep));
        } else {
            have_first = true;
            for (size_t i = 0; i < fields.size(); i++) header.push_back("x" + std::to_string(i + 1));
        }
    } else if (tok.failed()) {
        *error = tokenizer_error(tok);
        return nullptr;
    } else if (opt.header) {
        *error = source + ": the file is empty (expected a header line)";
        return nullptr;
    }
    const size_t width = header.size();

    // Map the output columns to file columns.
    std::vector<int> file_col;
    std::vector<QVector::Type> types;
    std::vector<std::string> names;
    if (schema) {
        if (!opt.header && !have_first) {
            // An empty file without a header is an empty table.
            for (size_t c = 0; c < schema->size(); c++) file_col.push_back(static_cast<int>(c));
        } else if (!opt.header) {
            // Without a header, the schema's columns are the file's, in order.
            if (schema->size() != width) {
                *error = source + ": the schema has " + std::to_string(schema->size()) + " columns but the file has " +
                         std::to_string(width);
                return nullptr;
            }
            for (size_t c = 0; c < width; c++) file_col.push_back(static_cast<int>(c));
        } else {
            for (const Column& c : *schema) {
                int found = -1, seen = 0;
                for (size_t i = 0; i < header.size(); i++) {
                    if (header[i] == c.name) {
                        if (found < 0) found = static_cast<int>(i);
                        seen++;
                    }
                }
                if (found < 0) {
                    *error = source + ": no column named '" + c.name + "' in the header (columns: " + quoted_list(header) + ")";
                    return nullptr;
                }
                if (seen > 1) {
                    *error = source + ": the header has " + std::to_string(seen) + " columns named '" + c.name + "'";
                    return nullptr;
                }
                file_col.push_back(found);
            }
        }
        for (const Column& c : *schema) {
            types.push_back(c.type);
            names.push_back(c.name);
        }
    } else {
        for (size_t i = 0; i < header.size(); i++) {
            if (header[i].empty()) {
                *error = source + ": column " + std::to_string(i + 1) + " has no name in the header";
                return nullptr;
            }
            for (size_t j = 0; j < i; j++) {
                if (header[j] == header[i]) {
                    *error = source + ": the header has two columns named '" + header[i] + "'";
                    return nullptr;
                }
            }
            file_col.push_back(static_cast<int>(i));
            names.push_back(header[i]);
        }
    }
    const size_t ncols = file_col.size();

    auto is_na = [&](std::string_view v) {
        for (const std::string& na : opt.na) {
            if (v == na) return true;
        }
        return false;
    };
    auto value_of = [&](const Field& f) { return opt.trim && !f.quoted ? trim(f.text, sep) : f.text; };

    Problems problems(source);
    // Rows are read once into the builders. Untyped reads first scan the rows
    // to infer the types (a second tokenizer over the same text).
    if (!schema) {
        std::vector<bool> can_int(ncols, true), can_float(ncols, true), can_bool(ncols, true), any(ncols, false);
        Tokenizer scan(text, len, sep);
        std::vector<Field> row;
        int rl = 0;
        if (opt.header) scan.next(row, &rl);
        while (scan.next(row, &rl)) {
            if (row.size() != width) continue; // reported in the second pass
            for (size_t c = 0; c < ncols; c++) {
                const std::string_view v = value_of(row[static_cast<size_t>(file_col[c])]);
                if (is_na(v)) continue;
                any[c] = true;
                int64_t i;
                double d;
                bool b;
                if (can_int[c] && !parse_i64(v, &i)) can_int[c] = false;
                if (can_float[c] && !parse_f64(v, &d)) can_float[c] = false;
                if (can_bool[c] && !parse_bool(v, &b)) can_bool[c] = false;
            }
        }
        for (size_t c = 0; c < ncols; c++) {
            if (!any[c]) types.push_back(QVector::Type::STR);
            else if (can_int[c]) types.push_back(QVector::Type::I64);
            else if (can_float[c]) types.push_back(QVector::Type::F64);
            else if (can_bool[c]) types.push_back(QVector::Type::BOOL);
            else types.push_back(QVector::Type::STR);
        }
    }

    std::vector<Builder> cols(ncols);
    for (size_t c = 0; c < ncols; c++) cols[c].type = types[c];

    auto add_row = [&](const std::vector<Field>& row, int rl) {
        if (row.size() != width) {
            problems.add(rl, "expected " + std::to_string(width) + " fields, found " + std::to_string(row.size()));
            return;
        }
        for (size_t c = 0; c < ncols; c++) {
            Builder& bld = cols[c];
            const std::string_view v = value_of(row[static_cast<size_t>(file_col[c])]);
            if (is_na(v)) {
                bld.push_null();
                continue;
            }
            bool ok = true;
            switch (bld.type) {
                case QVector::Type::I64: {
                    int64_t x = 0;
                    ok = parse_i64(v, &x);
                    bld.i64.push_back(x);
                    break;
                }
                case QVector::Type::F64: {
                    double x = 0;
                    ok = parse_f64(v, &x);
                    bld.f64.push_back(x);
                    break;
                }
                case QVector::Type::BOOL: {
                    bool x = false;
                    ok = parse_bool(v, &x);
                    bld.b.push_back(x ? 1 : 0);
                    break;
                }
                case QVector::Type::STR:
                    bld.bytes.append(v.data(), v.size());
                    if (bld.bytes.size() > static_cast<size_t>(INT32_MAX)) {
                        problems.add(rl, "column '" + names[c] + "' holds more than 2 GiB of text");
                        return;
                    }
                    bld.offsets.push_back(static_cast<int32_t>(bld.bytes.size()));
                    break;
            }
            bld.count++;
            if (!ok) {
                problems.add(rl, "column '" + names[c] + "': '" + std::string(v) + "' is not " + type_word(bld.type));
            }
        }
    };

    int64_t rows = 0;
    if (have_first) {
        add_row(fields, line);
        rows++;
    }
    while (tok.next(fields, &line)) {
        if (++rows > INT32_MAX) {
            *error = source + ": more than 2147483647 rows";
            return nullptr;
        }
        add_row(fields, line);
    }
    if (tok.failed()) problems.add(tok.error_line(), tok.error());
    if (problems.any()) {
        *error = problems.message();
        return nullptr;
    }

    // The table's schema: the given one, or one built from the header.
    if (!def) {
        const char** fnames = static_cast<const char**>(q_malloc(sizeof(char*) * (ncols == 0 ? 1 : ncols)));
        for (size_t c = 0; c < ncols; c++) fnames[c] = q_strdup(names[c].c_str());
        QStructDef* d = static_cast<QStructDef*>(q_malloc(sizeof(QStructDef)));
        d->name = "tbl";
        d->field_names = fnames;
        d->field_count = static_cast<int>(ncols);
        def = d;
    }
    QVector** vecs = static_cast<QVector**>(q_malloc(sizeof(QVector*) * (ncols == 0 ? 1 : ncols)));
    for (size_t c = 0; c < ncols; c++) {
        vecs[c] = cols[c].finish(error);
        if (!vecs[c]) return nullptr;
    }
    if (ncols == 0) return q_table_create(def, 0);
    return quark::tbl::make_table(def, vecs);
}

inline bool read_file(const char* path, std::string* out, std::string* error) {
    FILE* f = std::fopen(path, "rb");
    if (!f) {
        *error = std::string("cannot open '") + path + "': " + std::strerror(errno);
        return false;
    }
    char buf[1 << 16];
    size_t n;
    while ((n = std::fread(buf, 1, sizeof buf, f)) > 0) out->append(buf, n);
    const bool bad = std::ferror(f) != 0;
    std::fclose(f);
    if (bad) {
        *error = std::string("cannot read '") + path + "'";
        return false;
    }
    return true;
}

// read_table reads a CSV / TSV file. See parse_table for def and schema.
inline QTable* read_table(const char* path, const Options& opt, const QStructDef* def,
                          const std::vector<Column>* schema, std::string* error) {
    std::string text;
    if (!read_file(path, &text, error)) return nullptr;
    return parse_table(text.data(), text.size(), path, separator_for(path, opt), opt, def, schema, error);
}

// ------------------------------------------------------------
// Writing
// ------------------------------------------------------------

inline void append_field(std::string& out, std::string_view s, char sep) {
    bool quote = s.empty() ? false : (s.front() == ' ' || s.back() == ' ' || s.front() == '\t' || s.back() == '\t');
    for (char c : s) {
        if (c == sep || c == '"' || c == '\n' || c == '\r') {
            quote = true;
            break;
        }
    }
    if (!quote) {
        out.append(s.data(), s.size());
        return;
    }
    out.push_back('"');
    for (char c : s) {
        if (c == '"') out.push_back('"');
        out.push_back(c);
    }
    out.push_back('"');
}

// format_table renders a table as CSV text: a header line, then one line per
// row. Missing values are empty fields; strings are quoted when they contain
// the separator, a quote, a line break or surrounding whitespace.
inline std::string format_table(QTable* t, char sep) {
    std::string out;
    for (int c = 0; c < t->ncols; c++) {
        if (c) out.push_back(sep);
        append_field(out, t->def->field_names[c], sep);
    }
    out.push_back('\n');
    char num[64];
    for (int32_t r = 0; r < t->nrows; r++) {
        for (int c = 0; c < t->ncols; c++) {
            if (c) out.push_back(sep);
            const QVector& v = *quark::tbl::column(t, c);
            if (quark::tbl::is_null(v, r)) continue;
            switch (v.type) {
                case QVector::Type::I64:
                    std::snprintf(num, sizeof num, "%lld", static_cast<long long>(q_vec_i64_data(v)[r]));
                    out += num;
                    break;
                case QVector::Type::F64:
                    out += q_format_float(q_vec_f64_data(v)[r], num, sizeof num);
                    break;
                case QVector::Type::BOOL:
                    out += q_vec_bool_at(v, static_cast<size_t>(r)) ? "true" : "false";
                    break;
                case QVector::Type::STR:
                    append_field(out, q_vec_str_at(v, static_cast<size_t>(r)), sep);
                    break;
            }
        }
        out.push_back('\n');
    }
    return out;
}

inline bool write_table(QTable* t, const char* path, const Options& opt, std::string* error) {
    const std::string text = format_table(t, separator_for(path, opt));
    FILE* f = std::fopen(path, "wb");
    if (!f) {
        *error = std::string("cannot open '") + path + "' for writing: " + std::strerror(errno);
        return false;
    }
    const bool ok = std::fwrite(text.data(), 1, text.size(), f) == text.size();
    const bool closed = std::fclose(f) == 0;
    if (!ok || !closed) {
        *error = std::string("cannot write '") + path + "'";
        return false;
    }
    return true;
}

} // namespace csv
} // namespace quark

#endif // QUARK_TABLE_CSV_HPP
