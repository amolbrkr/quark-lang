// quark/builtins/repr.hpp - text form of values for print and
// to_str: scalars as themselves, containers by their contents.
//
//   [1, 'a', null]             list (strings inside containers are quoted)
//   [1.5, 2.0]                 vector (same brackets; type() tells them apart)
//   {age: 36, 'first name': 'Ada'}   dict, keys sorted, bare when name-like
//   Point {x: 1, y: 2}         struct
//   ok 5, err 'boom'           result
//
// A collection with more than Q_PRINT_THRESHOLD items shows the first and last
// Q_PRINT_EDGE and a count: [0, 1, 2, 3, 4, ..., 995, 996, 997, 998, 999] (1000 items).
// A container that contains itself prints [...] / {...} at the repeat.
// std/fmt's fmt.full(x) renders without truncation.
#ifndef QUARK_BUILTINS_REPR_HPP
#define QUARK_BUILTINS_REPR_HPP

#include "../core/value.hpp"
#include "../core/format.hpp"
#include "../types/vector.hpp"
#include "../types/dict.hpp"
#include "../types/struct.hpp"
#include "../types/table.hpp"
#include "../types/resource.hpp"
#include <algorithm>
#include <cstdio>
#include <cstring>
#include <string>
#include <utility>
#include <vector>

constexpr size_t Q_PRINT_THRESHOLD = 100;
constexpr size_t Q_PRINT_EDGE = 5;

struct QReprCtx {
    bool truncate = true;
    // Containers being rendered, innermost last, to stop at cycles. They are
    // all reachable from the value being printed, so the GC keeps them alive.
    std::vector<const void*> open;
};

inline void q_repr_into(std::string& out, QValue v, bool quoteStrings, QReprCtx& ctx);

inline void q_table_rows_text(std::string& out, long long n) {
    out += std::to_string(n);
    out += n == 1 ? " row" : " rows";
}

inline void q_repr_quoted(std::string& out, const char* s) {
    out.push_back('\'');
    for (const char* p = s ? s : ""; *p; p++) {
        unsigned char c = static_cast<unsigned char>(*p);
        switch (c) {
            case '\'': out += "\\'"; break;
            case '\\': out += "\\\\"; break;
            case '\n': out += "\\n"; break;
            case '\t': out += "\\t"; break;
            case '\r': out += "\\r"; break;
            default:
                if (c < 0x20 || c == 0x7f) {
                    char buf[8];
                    std::snprintf(buf, sizeof(buf), "\\x%02x", c);
                    out += buf;
                } else {
                    out.push_back(static_cast<char>(c));
                }
        }
    }
    out.push_back('\'');
}

// A dict key prints bare when it reads like a name (as in a dict literal).
inline bool q_repr_is_name(const char* s, size_t n) {
    if (n == 0) return false;
    for (size_t i = 0; i < n; i++) {
        char c = s[i];
        bool letter = (c >= 'a' && c <= 'z') || (c >= 'A' && c <= 'Z') || c == '_';
        bool digit = c >= '0' && c <= '9';
        if (!(letter || (digit && i > 0))) return false;
    }
    return true;
}

// q_repr_items renders `n` items between `open` and `close`, eliding the middle
// of long collections. item(i) appends item i.
template <typename Item>
inline void q_repr_items(std::string& out, size_t n, char open, char close, QReprCtx& ctx, Item item) {
    out.push_back(open);
    const bool cut = ctx.truncate && n > Q_PRINT_THRESHOLD;
    for (size_t i = 0; i < n; i++) {
        if (cut && i == Q_PRINT_EDGE) {
            out += ", ...";
            i = n - Q_PRINT_EDGE - 1;
            continue;
        }
        if (i > 0) out += ", ";
        item(i);
    }
    out.push_back(close);
    if (cut) {
        out += " (" + std::to_string(n) + " items)";
    }
}

inline bool q_repr_enter(QReprCtx& ctx, const void* p) {
    if (std::find(ctx.open.begin(), ctx.open.end(), p) != ctx.open.end()) return false;
    ctx.open.push_back(p);
    return true;
}

inline void q_repr_into(std::string& out, QValue v, bool quoteStrings, QReprCtx& ctx) {
    char buf[64];
    switch (v.type) {
        case QValue::VAL_INT:
            std::snprintf(buf, sizeof(buf), "%lld", v.data.int_val);
            out += buf;
            return;
        case QValue::VAL_FLOAT:
            out += q_format_float(v.data.float_val, buf, sizeof(buf));
            return;
        case QValue::VAL_BOOL:
            out += v.data.bool_val ? "true" : "false";
            return;
        case QValue::VAL_NULL:
            out += "null";
            return;
        case QValue::VAL_STRING:
            if (quoteStrings) {
                q_repr_quoted(out, v.data.string_val);
            } else {
                out += v.data.string_val ? v.data.string_val : "";
            }
            return;
        case QValue::VAL_LIST: {
            QList* list = v.data.list_val;
            if (!list) { out += "[]"; return; }
            if (!q_repr_enter(ctx, list)) { out += "[...]"; return; }
            q_repr_items(out, list->size(), '[', ']', ctx, [&](size_t i) {
                q_repr_into(out, (*list)[i], true, ctx);
            });
            ctx.open.pop_back();
            return;
        }
        case QValue::VAL_VECTOR: {
            QVector* vec = v.data.vector_val;
            if (!vec) { out += "[]"; return; }
            q_repr_items(out, static_cast<size_t>(vec->count), '[', ']', ctx, [&](size_t i) {
                q_repr_into(out, q_vec_box_at(*vec, i), true, ctx);
            });
            return;
        }
        case QValue::VAL_DICT: {
            QDict* dict = v.data.dict_val;
            if (!dict) { out += "{}"; return; }
            if (!q_repr_enter(ctx, dict)) { out += "{...}"; return; }
            // Sorted keys: dicts are unordered, and iteration order differs
            // between standard libraries, so sorting keeps output stable.
            std::vector<std::pair<std::string, QValue>> entries;
            entries.reserve(dict->entries.size());
            for (const auto& kv : dict->entries) {
                entries.emplace_back(std::string(kv.first.data(), kv.first.size()), kv.second);
            }
            std::sort(entries.begin(), entries.end(),
                      [](const auto& a, const auto& b) { return a.first < b.first; });
            q_repr_items(out, entries.size(), '{', '}', ctx, [&](size_t i) {
                const std::string& k = entries[i].first;
                if (q_repr_is_name(k.data(), k.size())) {
                    out += k;
                } else {
                    q_repr_quoted(out, k.c_str());
                }
                out += ": ";
                q_repr_into(out, entries[i].second, true, ctx);
            });
            ctx.open.pop_back();
            return;
        }
        case QValue::VAL_STRUCT: {
            QStruct* st = v.data.struct_val;
            if (!st || !st->def) { out += "<struct>"; return; }
            if (!q_repr_enter(ctx, st)) { out += st->def->name; out += " {...}"; return; }
            out += st->def->name;
            out += " {";
            for (int i = 0; i < st->def->field_count; i++) {
                if (i > 0) out += ", ";
                out += st->def->field_names[i];
                out += ": ";
                q_repr_into(out, st->fields[i], true, ctx);
            }
            out += "}";
            ctx.open.pop_back();
            return;
        }
        case QValue::VAL_RESULT: {
            QResult* r = v.data.result_val;
            if (!r) { out += "<result>"; return; }
            out += r->is_ok ? "ok " : "err ";
            q_repr_into(out, r->payload, true, ctx);
            return;
        }
        case QValue::VAL_FUNC:
            out += "<function>";
            return;
        case QValue::VAL_RESOURCE:
            std::snprintf(buf, sizeof(buf), "<resource %s>",
                          q_resource_kind_name(v.data.resource_val ? v.data.resource_val->kind : QRES_KIND_NONE));
            out += buf;
            return;
        case QValue::VAL_TABLE:
            // Inside a container, a table is its type and size; on its own
            // (q_repr) it prints its rows (q_table_render).
            out += "<";
            out += q_table_type_name(v.data.table_val);
            out += ", ";
            q_table_rows_text(out, v.data.table_val ? v.data.table_val->nrows : 0);
            out += ">";
            return;
        default:
            out += "<value>";
            return;
    }
}

// Table printing (tables_v0_1_spec.md §6): a line with the table's type and
// row count, the column names and their types, then the rows, numbers
// right-aligned, strings left-aligned and unquoted, a missing value as null.
// Q_TABLE_ROWS rows are shown and the rest counted, unless truncation is off.
constexpr int32_t Q_TABLE_ROWS = 10;
constexpr size_t Q_TABLE_CELL = 32; // code points; longer cells end in "..."

// q_text_width counts the code points of UTF-8 text.
inline size_t q_text_width(const std::string& s) {
    size_t n = 0;
    for (unsigned char c : s) n += (c & 0xC0) != 0x80;
    return n;
}

inline std::string q_table_cell(const QVector& v, int32_t r, bool truncate) {
    if (q_vec_is_null_at(v, static_cast<size_t>(r))) return "null";
    char buf[64];
    switch (v.type) {
        case QVector::Type::I64:
            std::snprintf(buf, sizeof buf, "%lld", static_cast<long long>(q_vec_i64_data(v)[r]));
            return buf;
        case QVector::Type::F64:
            return q_format_float(q_vec_f64_data(v)[r], buf, sizeof buf);
        case QVector::Type::BOOL:
            return q_vec_bool_at(v, static_cast<size_t>(r)) ? "true" : "false";
        case QVector::Type::STR: {
            std::string out;
            size_t width = 0;
            const std::string_view s = q_vec_str_at(v, static_cast<size_t>(r));
            for (size_t i = 0; i < s.size(); i++) {
                const unsigned char c = static_cast<unsigned char>(s[i]);
                if ((c & 0xC0) != 0x80) {
                    if (truncate && width == Q_TABLE_CELL - 3) {
                        out += "...";
                        return out;
                    }
                    width++;
                }
                switch (c) {
                    case '\n': out += "\\n"; break;
                    case '\t': out += "\\t"; break;
                    case '\r': out += "\\r"; break;
                    default: out += static_cast<char>(c);
                }
            }
            return out;
        }
    }
    return "";
}

inline void q_table_render(std::string& out, const QTable* t, bool truncate) {
    if (!t || !t->def) {
        out += "<tbl>";
        return;
    }
    out += t->def->anonymous ? "tbl" : q_table_type_name(t);
    out += ": ";
    q_table_rows_text(out, t->nrows);
    if (t->ncols == 0) return;
    const int32_t shown = truncate && t->nrows > Q_TABLE_ROWS ? Q_TABLE_ROWS : t->nrows;
    std::vector<std::vector<std::string>> cells(static_cast<size_t>(t->ncols));
    std::vector<size_t> width(static_cast<size_t>(t->ncols));
    std::vector<bool> right(static_cast<size_t>(t->ncols));
    for (int c = 0; c < t->ncols; c++) {
        const QVector& col = *t->cols[c].data.vector_val;
        auto& cs = cells[static_cast<size_t>(c)];
        cs.push_back(t->def->field_names[c]);
        cs.push_back(q_dtype_word(static_cast<int>(col.type)));
        for (int32_t r = 0; r < shown; r++) cs.push_back(q_table_cell(col, r, truncate));
        right[static_cast<size_t>(c)] = col.type != QVector::Type::STR;
        for (const std::string& s : cs) width[static_cast<size_t>(c)] = std::max(width[static_cast<size_t>(c)], q_text_width(s));
    }
    for (size_t line = 0; line < static_cast<size_t>(shown) + 2; line++) {
        out += "\n";
        std::string row;
        for (int c = 0; c < t->ncols; c++) {
            const std::string& s = cells[static_cast<size_t>(c)][line];
            const std::string pad(width[static_cast<size_t>(c)] - q_text_width(s), ' ');
            if (c) row += "  ";
            row += right[static_cast<size_t>(c)] ? pad + s : s + pad;
        }
        while (!row.empty() && row.back() == ' ') row.pop_back();
        out += row;
    }
    if (shown < t->nrows) {
        const int32_t more = t->nrows - shown;
        out += "\n... " + std::to_string(more) + (more == 1 ? " more row" : " more rows");
    }
}

// q_repr renders a value as print shows it (top-level strings unquoted, a
// table with its rows).
inline std::string q_repr(QValue v, bool truncate = true) {
    std::string out;
    if (v.type == QValue::VAL_TABLE) {
        q_table_render(out, v.data.table_val, truncate);
        return out;
    }
    QReprCtx ctx;
    ctx.truncate = truncate;
    q_repr_into(out, v, false, ctx);
    return out;
}

#endif // QUARK_BUILTINS_REPR_HPP
