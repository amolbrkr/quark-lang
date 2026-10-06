// stdlib/fmt.hpp — Native-typed C++ implementations for std/fmt module.
// These functions are called via extern fn declarations in fmt.qrk.
// They receive native C++ types (QList*, QDict*, int64_t, bool) and return const char*.
#ifndef QUARK_STDLIB_FMT_HPP
#define QUARK_STDLIB_FMT_HPP

#include <algorithm>
#include <cstdio>
#include <cstring>
#include <string>
#include <vector>

// ============================================================
// Internal helpers
// ============================================================

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

static inline std::string q_fmt_pad(const std::string& s, size_t width, bool right_align = false) {
    if (s.size() >= width) return s;
    size_t pad = width - s.size();
    if (right_align) return std::string(pad, ' ') + s;
    return s + std::string(pad, ' ');
}

static inline bool q_fmt_is_numeric(QValue v) {
    return v.type == QValue::VAL_INT || v.type == QValue::VAL_FLOAT;
}

// ============================================================
// q_fmt_list(lst, n, show_index) -> str
// ============================================================
inline const char* q_fmt_list(QList* lst, int64_t max_rows, bool show_index) {
    if (!lst) {
        q_runtime_reportf("runtime error: fmt.show_list() expects list\n");
        std::exit(1);
    }

    const QList& items = *lst;
    const size_t total = items.size();

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
    for (size_t i = 0; i < total; i++) {
        val_w = std::max(val_w, q_fmt_cell(items[i]).size());
    }

    std::string out;
    if (show_index) {
        out += q_fmt_pad("", idx_w + 2);
    }
    out += q_fmt_pad("value", val_w) + "\n";

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

    out += "[list: " + std::to_string(total) + " element" + (total != 1 ? "s" : "") + "]";
    return q_strdup(out.c_str());
}

// ============================================================
// q_fmt_vector(vec, n, show_index) -> str
// ============================================================
inline const char* q_fmt_vector(QVector* vec, int64_t max_rows, bool show_index) {
    if (!vec) {
        q_runtime_reportf("runtime error: fmt.show_vec() expects vector\n");
        std::exit(1);
    }

    const QVector& qvec = *vec;
    const size_t total = qvec.count;
    const char* dtype = q_vec_dtype_name(qvec);

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
    return q_strdup(out.c_str());
}

// ============================================================
// q_fmt_dict(dct, n, show_index) -> str
// ============================================================
inline const char* q_fmt_dict(QDict* dct, int64_t max_rows, bool show_index) {
    if (!dct) {
        q_runtime_reportf("runtime error: fmt.show_dict() expects dict\n");
        std::exit(1);
    }

    std::vector<std::pair<std::string, QValue>> raw_rows;
    for (const auto& kv : dct->entries) {
        raw_rows.push_back({std::string(kv.first.c_str()), kv.second});
    }
    std::sort(raw_rows.begin(), raw_rows.end(),
              [](const auto& a, const auto& b) { return a.first < b.first; });
    std::vector<std::pair<std::string, std::string>> rows;
    bool any_numeric_val = false;
    for (const auto& kv : raw_rows) {
        rows.push_back({kv.first, q_fmt_cell(kv.second)});
        if (q_fmt_is_numeric(kv.second)) any_numeric_val = true;
    }

    const size_t total = rows.size();

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
    return q_strdup(out.c_str());
}

// ============================================================
// q_fmt_table(df, n, show_index) -> str
//   df is either a table value, or a dict whose values are equal-length
//   list/vector columns (the dataframe form; columns sorted by name).
// ============================================================

// Formats the first nrows cells of a vector column into out.
inline void q_fmt_vector_cells(const QVector& qv, size_t nrows,
                               std::vector<std::string>& out, bool& numeric) {
    numeric = (qv.type == QVector::Type::F64 || qv.type == QVector::Type::I64);
    for (size_t r = 0; r < nrows; r++) {
        if (q_vec_is_null_at(qv, r)) { out[r] = "NA"; continue; }
        char buf[64];
        switch (qv.type) {
            case QVector::Type::F64:
                std::snprintf(buf, sizeof(buf), "%g", std::get<QVecF64>(qv.storage)[r]);
                out[r] = buf; break;
            case QVector::Type::I64:
                std::snprintf(buf, sizeof(buf), "%lld", (long long)std::get<QVecI64>(qv.storage)[r]);
                out[r] = buf; break;
            case QVector::Type::BOOL:
                out[r] = std::get<QVecU8>(qv.storage)[r] ? "true" : "false"; break;
            case QVector::Type::STR: {
                const auto& ss = std::get<QStringStorage>(qv.storage);
                uint32_t s = ss.offsets[r], e = ss.offsets[r+1];
                out[r] = std::string(ss.bytes.data() + s, ss.bytes.data() + e);
                break;
            }
        }
    }
}

inline const char* q_fmt_table(QValue df, int64_t max_rows, bool show_index) {
    std::vector<std::string> col_names;
    std::vector<std::vector<std::string>> cells;
    std::vector<bool> col_numeric;
    size_t nrows = 0;

    if (df.type == QValue::VAL_TABLE && df.data.table_val) {
        QTable* tbl = df.data.table_val;
        nrows = static_cast<size_t>(tbl->nrows);
        size_t ncols = static_cast<size_t>(tbl->ncols);
        col_names.resize(ncols);
        cells.assign(ncols, std::vector<std::string>(nrows));
        col_numeric.assign(ncols, false);
        for (size_t c = 0; c < ncols; c++) {
            col_names[c] = tbl->def->field_names[c];
            QValue colval = tbl->cols[c];
            if (colval.type == QValue::VAL_VECTOR && colval.data.vector_val) {
                bool numeric = false;
                q_fmt_vector_cells(*colval.data.vector_val, nrows, cells[c], numeric);
                col_numeric[c] = numeric;
            } else {
                for (size_t r = 0; r < nrows; r++) cells[c][r] = "?";
            }
        }
    } else if (df.type == QValue::VAL_DICT && df.data.dict_val) {
        QDict* dct = df.data.dict_val;
        for (const auto& kv : dct->entries) {
            col_names.push_back(std::string(kv.first.c_str()));
        }
        if (col_names.empty()) {
            return q_strdup("[empty dataframe]");
        }
        std::sort(col_names.begin(), col_names.end());

        std::vector<QValue> cols;
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
            if (cols.empty()) {
                nrows = col_len;
            } else if (col_len != nrows) {
                q_runtime_reportf("runtime error: fmt.table() column '%s' has length %zu, expected %zu\n",
                                  cname.c_str(), col_len, nrows);
                std::exit(1);
            }
            cols.push_back(col);
        }

        size_t ncols = col_names.size();
        cells.assign(ncols, std::vector<std::string>(nrows));
        col_numeric.assign(ncols, false);
        for (size_t c = 0; c < ncols; c++) {
            bool numeric = false;
            if (cols[c].type == QValue::VAL_VECTOR) {
                q_fmt_vector_cells(*cols[c].data.vector_val, nrows, cells[c], numeric);
            } else {
                const QList& lst = *cols[c].data.list_val;
                for (size_t r = 0; r < nrows; r++) {
                    cells[c][r] = q_fmt_cell(lst[r]);
                    if (q_fmt_is_numeric(lst[r])) numeric = true;
                }
            }
            col_numeric[c] = numeric;
        }
    } else {
        q_runtime_reportf("runtime error: fmt.table() expects table or dict of columns\n");
        std::exit(1);
    }

    size_t ncols = col_names.size();
    std::vector<size_t> col_w(ncols);
    for (size_t c = 0; c < ncols; c++) {
        col_w[c] = col_names[c].size();
        for (size_t r = 0; r < nrows; r++) {
            col_w[c] = std::max(col_w[c], cells[c][r].size());
        }
    }

    size_t idx_w = show_index ? std::to_string(nrows > 0 ? nrows - 1 : 0).size() : 0;

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

    std::string out;

    auto hline = [&]() {
        if (show_index) out += "+" + std::string(idx_w + 2, '-');
        for (size_t c = 0; c < ncols; c++) out += "+" + std::string(col_w[c] + 2, '-');
        out += "+\n";
    };

    auto print_data_row = [&](size_t r) {
        if (show_index) out += "| " + q_fmt_pad(std::to_string(r), idx_w, true) + " ";
        for (size_t c = 0; c < ncols; c++) {
            out += "| " + q_fmt_pad(cells[c][r], col_w[c], col_numeric[c]) + " ";
        }
        out += "|\n";
    };

    hline();
    if (show_index) out += "| " + q_fmt_pad("#", idx_w) + " ";
    for (size_t c = 0; c < ncols; c++) out += "| " + q_fmt_pad(col_names[c], col_w[c]) + " ";
    out += "|\n";
    hline();

    for (size_t r = 0; r < head_n; r++) print_data_row(r);
    if (truncated) {
        if (show_index) out += "| " + q_fmt_pad("...", idx_w) + " ";
        for (size_t c = 0; c < ncols; c++) out += "| " + q_fmt_pad("...", col_w[c]) + " ";
        out += "|\n";
        for (size_t r = nrows - tail_n; r < nrows; r++) print_data_row(r);
    }
    hline();

    out += "[" + std::to_string(nrows) + " row" + (nrows != 1 ? "s" : "") +
           " x " + std::to_string(ncols) + " col" + (ncols != 1 ? "s" : "") + "]";
    return q_strdup(out.c_str());
}

#endif // QUARK_STDLIB_FMT_HPP
