// stdlib/csv.hpp — native implementations for the std/csv module (csv.qrk):
// CSV / TSV files and text read into tables, and tables written back
// (specs/tables_v0_1_spec.md §8, D15-D18). The reader and writer are the runtime's
// (quark/table/csv.hpp).
//
// A read takes the row struct of the table it produces when the compiler
// knows it (D18): csv::read_file and csv::parse_text calls whose expected
// type is a tbl[R] pass R's definition as `row`, and the file is read with
// R's field names and column types. Without one (row == nullptr) every
// column's type is inferred from its values and the table's row is
// anonymous. `columns` maps fields to header names (D17): dict { id: 'EmpID' }.
#ifndef QUARK_STDLIB_CSV_HPP
#define QUARK_STDLIB_CSV_HPP

#include "quark/table/csv.hpp"

#include <cstring>
#include <string>
#include <utility>
#include <vector>

namespace qcsv {

// mapping reads the D17 mapping (field -> header text). The compiler checks
// a literal mapping; a dict that reaches here through a function value is
// checked now.
inline bool mapping(QDict* columns, const QStructDef* row, std::vector<std::pair<std::string, std::string>>* out,
                    std::string* error) {
    if (columns == nullptr) return true;
    for (const auto& kv : columns->entries) {
        const std::string field(kv.first.data(), kv.first.size());
        if (kv.second.type != QValue::VAL_STRING || kv.second.data.string_val == nullptr) {
            *error = "csv: the column mapping maps '" + field + "' to a value of type " + std::string(q_type_name(kv.second.type)) +
                     "; map each field to a header name (a str)";
            return false;
        }
        if (row) {
            bool known = false;
            for (int f = 0; f < row->field_count && !known; f++) known = field == row->field_names[f];
            if (!known) {
                *error = "csv: the column mapping names '" + field + "', which isn't a field of " + row->name;
                return false;
            }
        }
        out->emplace_back(field, kv.second.data.string_val);
    }
    return true;
}

inline QValue read(const char* text, size_t len, const std::string& source, char sep, QDict* columns,
                   const QStructDef* row) {
    quark::csv::Options opt;
    std::vector<std::pair<std::string, std::string>> pairs;
    std::string error;
    if (!mapping(columns, row, &pairs, &error)) return qv_err(qv_string(error.c_str()));
    std::vector<quark::csv::Column> schema;
    if (row) {
        if (row->field_dtypes == nullptr) {
            q_runtime_reportf("runtime error: internal: csv read with row %s, which has no column types\n", row->name);
            std::exit(1);
        }
        for (int f = 0; f < row->field_count; f++) {
            quark::csv::Column c;
            c.name = row->field_names[f];
            c.type = static_cast<QVector::Type>(row->field_dtypes[f]);
            for (const auto& p : pairs) {
                if (p.first == c.name) c.header = p.second;
            }
            schema.push_back(c);
        }
    } else {
        opt.rename = pairs;
    }
    QTable* t = quark::csv::parse_table(text, len, source, sep, opt, row, row ? &schema : nullptr, &error);
    if (!t) return qv_err(qv_string(error.c_str()));
    return qv_ok(qv_table(t));
}

} // namespace qcsv

// csv::read_file(path, columns): ok(table) or err(message). A path ending in
// .tsv or .tab is tab-separated.
inline QValue q_csv_read_file(const char* path, QDict* columns, const QStructDef* row = nullptr) {
    std::string text, error;
    if (!quark::csv::read_file(path, &text, &error)) return qv_err(qv_string(error.c_str()));
    return qcsv::read(text.data(), text.size(), path, quark::csv::separator_for(path, quark::csv::Options()), columns, row);
}

// csv::parse_text(text, columns): the same for CSV text (comma-separated).
inline QValue q_csv_parse_text(const char* text, QDict* columns, const QStructDef* row = nullptr) {
    return qcsv::read(text, std::strlen(text), "text", ',', columns, row);
}

// csv::write_file(t, path): ok(null) or err(message).
inline QValue q_csv_write_file(QTable* t, const char* path) {
    std::string error;
    if (!quark::csv::write_table(t, path, quark::csv::Options(), &error)) return qv_err(qv_string(error.c_str()));
    return qv_ok(qv_null());
}

// csv::format_text(t): the table as CSV text, as write_file writes it.
inline const char* q_csv_format_text(QTable* t) {
    return q_strdup(quark::csv::format_table(t, ',').c_str());
}

#endif // QUARK_STDLIB_CSV_HPP
