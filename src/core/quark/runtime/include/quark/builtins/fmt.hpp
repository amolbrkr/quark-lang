// quark/builtins/fmt.hpp - Pretty-printing for lists, vectors, dicts, and dataframe tables
#ifndef QUARK_BUILTINS_FMT_HPP
#define QUARK_BUILTINS_FMT_HPP

#include "../core/value.hpp"
#include "../core/constructors.hpp"
#include "../builtins/conversion.hpp"
#include "../types/dict.hpp"
#include "../types/vector.hpp"

#include <algorithm>
#include <cstdio>
#include <cstring>
#include <string>
#include <vector>

// ============================================================
// Internal helpers
// ============================================================

// Stringify a single QValue to a short display string.
static inline std::string q_fmt_cell(QValue v) {
    char buf[128];
    switch (v.type) {
        case QValue::VAL_INT:
            std::snprintf(buf, sizeof(buf), "%lld", v.data.int_val);
            return buf;
        case QValue::VAL_FLOAT:
            std::snprintf(buf, sizeof(buf), "%g", v.data.float_val);
            return buf;
        case QValue::VAL_BOOL:
            return v.data.bool_val ? "true" : "false";
        case QValue::VAL_STRING:
            return v.data.string_val ? v.data.string_val : "";
        case QValue::VAL_NULL:
            return "null";
        case QValue::VAL_LIST:
            std::snprintf(buf, sizeof(buf), "[list len=%zu]",
                         v.data.list_val ? v.data.list_val->size() : 0);
            return buf;
        case QValue::VAL_VECTOR:
            std::snprintf(buf, sizeof(buf), "[vector len=%d]", q_vec_size(v));
            return buf;
        case QValue::VAL_DICT:
            std::snprintf(buf, sizeof(buf), "[dict len=%zu]",
                         v.data.dict_val ? v.data.dict_val->entries.size() : 0);
            return buf;
        case QValue::VAL_FUNC:
            return "<fn>";
        default:
            return "<value>";
    }
}

// Pad a string to width, left- or right-aligned.
static inline std::string q_fmt_pad(const std::string& s, size_t width, bool right_align = false) {
    if (s.size() >= width) return s;
    size_t pad = width - s.size();
    if (right_align) return std::string(pad, ' ') + s;
    return s + std::string(pad, ' ');
}

// Detect whether a value is "numeric-looking" for right-alignment.
static inline bool q_fmt_is_numeric(QValue v) {
    return v.type == QValue::VAL_INT || v.type == QValue::VAL_FLOAT;
}

// ============================================================
// q_fmt_list(list, n, show_index) -> str
//   n          = max rows to show (0 = all)
//   show_index = whether to print index column
// ============================================================
inline QValue q_fmt_list(QValue lst, QValue n_val, QValue show_index_val) {
    if (lst.type != QValue::VAL_LIST || !lst.data.list_val) {
        q_runtime_reportf("runtime error: fmt.list() expects list\n");
        std::exit(1);
    }
    if (n_val.type != QValue::VAL_INT) {
        q_runtime_reportf("runtime error: fmt.list() n must be int\n");
        std::exit(1);
    }
    if (show_index_val.type != QValue::VAL_BOOL) {
        q_runtime_reportf("runtime error: fmt.list() show_index must be bool\n");
        std::exit(1);
    }

    const QList& items = *lst.data.list_val;
    const size_t total = items.size();
    long long max_rows = n_val.data.int_val;
    bool show_index = show_index_val.data.bool_val;

    // Determine how many rows to show from head and tail (like R)
    size_t head_n, tail_n;
    bool truncated = false;
    if (max_rows <= 0 || static_cast<size_t>(max_rows) >= total) {
        head_n = total;
        tail_n = 0;
    } else {
        head_n = static_cast<size_t>(max_rows) / 2 + static_cast<size_t>(max_rows) % 2;
        tail_n = static_cast<size_t>(max_rows) / 2;
        truncated = (head_n + tail_n < total);
    }

    // Compute column widths
    size_t idx_w = show_index ? std::to_string(total - 1).size() : 0;
    size_t val_w = 5; // "value"
    for (size_t i = 0; i < total; i++) {
        val_w = std::max(val_w, q_fmt_cell(items[i]).size());
    }

    std::string out;
    // header
    if (show_index) {
        out += q_fmt_pad("", idx_w + 2);
    }
    out += q_fmt_pad("value", val_w) + "\n";

    // separator
    if (show_index) out += std::string(idx_w + 2, '-');
    out += std::string(val_w, '-') + "\n";

    auto print_row = [&](size_t i) {
        std::string cell = q_fmt_cell(items[i]);
        bool num = q_fmt_is_numeric(items[i]);
        if (show_index) {
            out += q_fmt_pad(std::to_string(i), idx_w, true) + "  ";
        }
        out += q_fmt_pad(cell, val_w, num) + "\n";
    };

    for (size_t i = 0; i < head_n; i++) print_row(i);
    if (truncated) {
        size_t omitted = total - head_n - tail_n;
        out += "... (" + std::to_string(omitted) + " rows omitted) ...\n";
        for (size_t i = total - tail_n; i < total; i++) print_row(i);
    }

    // footer
    out += "[list: " + std::to_string(total) + " element" + (total != 1 ? "s" : "") + "]";
    return qv_string(out.c_str());
}

// ============================================================
// q_fmt_vector(vec, n, show_index) -> str
// ============================================================
inline QValue q_fmt_vector(QValue vec, QValue n_val, QValue show_index_val) {
    if (!q_vec_has_valid_handle(vec)) {
        q_runtime_reportf("runtime error: fmt.vector() expects vector\n");
        std::exit(1);
    }
    if (n_val.type != QValue::VAL_INT) {
        q_runtime_reportf("runtime error: fmt.vector() n must be int\n");
        std::exit(1);
    }
    if (show_index_val.type != QValue::VAL_BOOL) {
        q_runtime_reportf("runtime error: fmt.vector() show_index must be bool\n");
        std::exit(1);
    }

    const QVector& qvec = *vec.data.vector_val;
    const size_t total = qvec.count;
    long long max_rows = n_val.data.int_val;
    bool show_index = show_index_val.data.bool_val;
    const char* dtype = q_vec_dtype_name(qvec);

    // Materialize all string representations
    std::vector<std::string> cells(total);
    bool all_numeric = (qvec.type == QVector::Type::F64 || qvec.type == QVector::Type::I64);

    for (size_t i = 0; i < total; i++) {
        if (q_vec_is_null_at(qvec, i)) {
            cells[i] = "NA";
            continue;
        }
        char buf[64];
        switch (qvec.type) {
            case QVector::Type::F64:
                std::snprintf(buf, sizeof(buf), "%g", std::get<QVecF64>(qvec.storage)[i]);
                cells[i] = buf;
                break;
            case QVector::Type::I64:
                std::snprintf(buf, sizeof(buf), "%lld", (long long)std::get<QVecI64>(qvec.storage)[i]);
                cells[i] = buf;
                break;
            case QVector::Type::BOOL:
                cells[i] = std::get<QVecU8>(qvec.storage)[i] ? "true" : "false";
                break;
            case QVector::Type::STR: {
                const auto& ss = std::get<QStringStorage>(qvec.storage);
                uint32_t s = ss.offsets[i], e = ss.offsets[i+1];
                cells[i] = std::string(ss.bytes.data() + s, ss.bytes.data() + e);
                break;
            }
        }
    }

    size_t head_n, tail_n;
    bool truncated = false;
    if (max_rows <= 0 || static_cast<size_t>(max_rows) >= total) {
        head_n = total;
        tail_n = 0;
    } else {
        head_n = static_cast<size_t>(max_rows) / 2 + static_cast<size_t>(max_rows) % 2;
        tail_n = static_cast<size_t>(max_rows) / 2;
        truncated = (head_n + tail_n < total);
    }

    size_t idx_w = show_index ? std::to_string(total - 1).size() : 0;
    size_t val_w = 5; // "value"
    for (const auto& c : cells) val_w = std::max(val_w, c.size());

    std::string out;
    if (show_index) out += q_fmt_pad("", idx_w + 2);
    out += q_fmt_pad("value", val_w) + "\n";
    if (show_index) out += std::string(idx_w + 2, '-');
    out += std::string(val_w, '-') + "\n";

    auto print_row = [&](size_t i) {
        if (show_index) out += q_fmt_pad(std::to_string(i), idx_w, true) + "  ";
        out += q_fmt_pad(cells[i], val_w, all_numeric) + "\n";
    };

    for (size_t i = 0; i < head_n; i++) print_row(i);
    if (truncated) {
        size_t omitted = total - head_n - tail_n;
        out += "... (" + std::to_string(omitted) + " rows omitted) ...\n";
        for (size_t i = total - tail_n; i < total; i++) print_row(i);
    }

    out += "[vector[" + std::string(dtype) + "]: " + std::to_string(total) +
           " element" + (total != 1 ? "s" : "") + "]";
    return qv_string(out.c_str());
}

// ============================================================
// q_fmt_dict(dict, n, show_index) -> str
//   Prints key/value pairs, n = max rows
// ============================================================
inline QValue q_fmt_dict(QValue dct, QValue n_val, QValue show_index_val) {
    if (dct.type != QValue::VAL_DICT || !dct.data.dict_val) {
        q_runtime_reportf("runtime error: fmt.dict() expects dict\n");
        std::exit(1);
    }
    if (n_val.type != QValue::VAL_INT) {
        q_runtime_reportf("runtime error: fmt.dict() n must be int\n");
        std::exit(1);
    }
    if (show_index_val.type != QValue::VAL_BOOL) {
        q_runtime_reportf("runtime error: fmt.dict() show_index must be bool\n");
        std::exit(1);
    }

    // Collect entries in stable iteration order
    std::vector<std::pair<std::string, std::string>> rows;
    bool any_numeric_val = false;
    for (const auto& kv : dct.data.dict_val->entries) {
        rows.push_back({std::string(kv.first.c_str()), q_fmt_cell(kv.second)});
        if (q_fmt_is_numeric(kv.second)) any_numeric_val = true;
    }

    const size_t total = rows.size();
    long long max_rows = n_val.data.int_val;
    bool show_index = show_index_val.data.bool_val;

    size_t head_n, tail_n;
    bool truncated = false;
    if (max_rows <= 0 || static_cast<size_t>(max_rows) >= total) {
        head_n = total;
        tail_n = 0;
    } else {
        head_n = static_cast<size_t>(max_rows) / 2 + static_cast<size_t>(max_rows) % 2;
        tail_n = static_cast<size_t>(max_rows) / 2;
        truncated = (head_n + tail_n < total);
    }

    size_t idx_w = show_index ? std::to_string(total - 1).size() : 0;
    size_t key_w = 3; // "key"
    size_t val_w = 5; // "value"
    for (const auto& r : rows) {
        key_w = std::max(key_w, r.first.size());
        val_w = std::max(val_w, r.second.size());
    }

    std::string out;
    // header
    if (show_index) out += q_fmt_pad("", idx_w + 2);
    out += q_fmt_pad("key", key_w) + "  " + q_fmt_pad("value", val_w) + "\n";
    if (show_index) out += std::string(idx_w + 2, '-');
    out += std::string(key_w, '-') + "  " + std::string(val_w, '-') + "\n";

    auto print_row = [&](size_t i) {
        if (show_index) out += q_fmt_pad(std::to_string(i), idx_w, true) + "  ";
        out += q_fmt_pad(rows[i].first, key_w) + "  " +
               q_fmt_pad(rows[i].second, val_w, any_numeric_val) + "\n";
    };

    for (size_t i = 0; i < head_n; i++) print_row(i);
    if (truncated) {
        size_t omitted = total - head_n - tail_n;
        out += "... (" + std::to_string(omitted) + " rows omitted) ...\n";
        for (size_t i = total - tail_n; i < total; i++) print_row(i);
    }

    out += "[dict: " + std::to_string(total) + " key" + (total != 1 ? "s" : "") + "]";
    return qv_string(out.c_str());
}

// ============================================================
// q_fmt_table(df, n, show_index) -> str
//   df must be a dict where every value is a vector or list of equal length.
//   Columns are printed as an ASCII table like pandas/R output.
//   n = max rows to show (0 = all); show_index = show row number column.
// ============================================================
inline QValue q_fmt_table(QValue df, QValue n_val, QValue show_index_val) {
    if (df.type != QValue::VAL_DICT || !df.data.dict_val) {
        q_runtime_reportf("runtime error: fmt.table() expects dict (dataframe)\n");
        std::exit(1);
    }
    if (n_val.type != QValue::VAL_INT) {
        q_runtime_reportf("runtime error: fmt.table() n must be int\n");
        std::exit(1);
    }
    if (show_index_val.type != QValue::VAL_BOOL) {
        q_runtime_reportf("runtime error: fmt.table() show_index must be bool\n");
        std::exit(1);
    }

    long long max_rows = n_val.data.int_val;
    bool show_index = show_index_val.data.bool_val;

    // Collect column names in iteration order
    std::vector<std::string> col_names;
    for (const auto& kv : df.data.dict_val->entries) {
        col_names.push_back(std::string(kv.first.c_str()));
    }

    if (col_names.empty()) {
        return qv_string("[empty dataframe]");
    }

    // Sort columns for deterministic output
    std::sort(col_names.begin(), col_names.end());

    // Determine number of rows (all columns must have same length)
    size_t nrows = 0;
    for (const auto& cname : col_names) {
        QValue col = q_dict_get(df, qv_string(cname.c_str()));
        size_t col_len = 0;
        if (col.type == QValue::VAL_VECTOR && col.data.vector_val) {
            col_len = col.data.vector_val->count;
        } else if (col.type == QValue::VAL_LIST && col.data.list_val) {
            col_len = col.data.list_val->size();
        } else {
            q_runtime_reportf("runtime error: fmt.table() column '%s' must be vector or list\n",
                              cname.c_str());
            std::exit(1);
        }
        if (nrows == 0) {
            nrows = col_len;
        } else if (col_len != nrows) {
            q_runtime_reportf("runtime error: fmt.table() column '%s' has length %zu, expected %zu\n",
                              cname.c_str(), col_len, nrows);
            std::exit(1);
        }
    }

    // Materialize cell strings: cells[col][row]
    size_t ncols = col_names.size();
    std::vector<std::vector<std::string>> cells(ncols, std::vector<std::string>(nrows));
    std::vector<bool> col_numeric(ncols, false);

    for (size_t c = 0; c < ncols; c++) {
        QValue col = q_dict_get(df, qv_string(col_names[c].c_str()));
        bool numeric = false;

        if (col.type == QValue::VAL_VECTOR && col.data.vector_val) {
            const QVector& qv = *col.data.vector_val;
            numeric = (qv.type == QVector::Type::F64 || qv.type == QVector::Type::I64);
            for (size_t r = 0; r < nrows; r++) {
                if (q_vec_is_null_at(qv, r)) {
                    cells[c][r] = "NA";
                    continue;
                }
                char buf[64];
                switch (qv.type) {
                    case QVector::Type::F64:
                        std::snprintf(buf, sizeof(buf), "%g", std::get<QVecF64>(qv.storage)[r]);
                        cells[c][r] = buf;
                        break;
                    case QVector::Type::I64:
                        std::snprintf(buf, sizeof(buf), "%lld", (long long)std::get<QVecI64>(qv.storage)[r]);
                        cells[c][r] = buf;
                        break;
                    case QVector::Type::BOOL:
                        cells[c][r] = std::get<QVecU8>(qv.storage)[r] ? "true" : "false";
                        break;
                    case QVector::Type::STR: {
                        const auto& ss = std::get<QStringStorage>(qv.storage);
                        uint32_t s = ss.offsets[r], e = ss.offsets[r+1];
                        cells[c][r] = std::string(ss.bytes.data() + s, ss.bytes.data() + e);
                        break;
                    }
                }
            }
        } else {
            // list column
            const QList& lst = *col.data.list_val;
            for (size_t r = 0; r < nrows; r++) {
                cells[c][r] = q_fmt_cell(lst[r]);
                if (q_fmt_is_numeric(lst[r])) numeric = true;
            }
        }
        col_numeric[c] = numeric;
    }

    // Compute column widths (max of header and all cells)
    std::vector<size_t> col_w(ncols);
    for (size_t c = 0; c < ncols; c++) {
        col_w[c] = col_names[c].size();
        for (size_t r = 0; r < nrows; r++) {
            col_w[c] = std::max(col_w[c], cells[c][r].size());
        }
    }

    // Row number column width
    size_t idx_w = show_index ? std::to_string(nrows > 0 ? nrows - 1 : 0).size() : 0;

    // Determine which rows to show
    size_t head_n, tail_n;
    bool truncated = false;
    if (max_rows <= 0 || static_cast<size_t>(max_rows) >= nrows) {
        head_n = nrows;
        tail_n = 0;
    } else {
        head_n = static_cast<size_t>(max_rows) / 2 + static_cast<size_t>(max_rows) % 2;
        tail_n = static_cast<size_t>(max_rows) / 2;
        truncated = (head_n + tail_n < nrows);
    }

    // Build the table string
    std::string out;

    // Helper: print a horizontal rule
    auto hline = [&]() {
        if (show_index) {
            out += "+" + std::string(idx_w + 2, '-');
        }
        for (size_t c = 0; c < ncols; c++) {
            out += "+" + std::string(col_w[c] + 2, '-');
        }
        out += "+\n";
    };

    // Helper: print a data row
    auto print_data_row = [&](size_t r) {
        if (show_index) {
            std::string idx_str = std::to_string(r);
            out += "| " + q_fmt_pad(idx_str, idx_w, true) + " ";
        }
        for (size_t c = 0; c < ncols; c++) {
            out += "| " + q_fmt_pad(cells[c][r], col_w[c], col_numeric[c]) + " ";
        }
        out += "|\n";
    };

    // Header row
    hline();
    if (show_index) {
        out += "| " + q_fmt_pad("#", idx_w) + " ";
    }
    for (size_t c = 0; c < ncols; c++) {
        out += "| " + q_fmt_pad(col_names[c], col_w[c]) + " ";
    }
    out += "|\n";
    hline();

    for (size_t r = 0; r < head_n; r++) print_data_row(r);
    if (truncated) {
        size_t omitted = nrows - head_n - tail_n;
        // Print a "..." row
        if (show_index) {
            out += "| " + q_fmt_pad("...", idx_w) + " ";
        }
        for (size_t c = 0; c < ncols; c++) {
            out += "| " + q_fmt_pad("...", col_w[c]) + " ";
        }
        out += "|\n";
        for (size_t r = nrows - tail_n; r < nrows; r++) print_data_row(r);
        (void)omitted;
    }
    hline();

    // Footer summary
    out += "[" + std::to_string(nrows) + " row" + (nrows != 1 ? "s" : "") +
           " x " + std::to_string(ncols) + " col" + (ncols != 1 ? "s" : "") + "]";
    return qv_string(out.c_str());
}

#endif // QUARK_BUILTINS_FMT_HPP
