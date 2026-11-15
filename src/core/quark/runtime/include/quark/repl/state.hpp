// quark/repl/state.hpp - session variables for the REPL host.
//
// An input's library does not own the session. The host does: one array per
// storage, grown by q_repl_ensure. int, float and bool are raw slots, so
// arithmetic on them lowers like a file. Every other value is a QCell, which
// the collector scans. The arrays themselves are roots because the host's
// globals point at them.
#ifndef QUARK_REPL_STATE_HPP
#define QUARK_REPL_STATE_HPP

#include <cstring>
#include <cstdint>
#include "../core/gc.hpp"
#include "../core/value.hpp"
#include "../core/cell.hpp"
#include "../core/diagnostics.hpp"
#include "../types/elemtype.hpp"
#include "../types/list.hpp"
#include "../types/dict.hpp"

#if defined(QUARK_REPL) && defined(_WIN32)
#if defined(QUARK_REPL_BUILD)
#define QREPL_DATA __declspec(dllexport)
#else
#define QREPL_DATA __declspec(dllimport)
#endif
#else
#define QREPL_DATA
#endif

extern "C" {

extern QREPL_DATA long long* q_repl_i64;
extern QREPL_DATA unsigned char* q_repl_i64_live;
extern QREPL_DATA double* q_repl_f64;
extern QREPL_DATA unsigned char* q_repl_f64_live;
extern QREPL_DATA bool* q_repl_bool;
extern QREPL_DATA unsigned char* q_repl_bool_live;
extern QREPL_DATA QCell** q_repl_cell;

extern QREPL_DATA int q_repl_ni, q_repl_nf, q_repl_nb, q_repl_nc;

// Defined in the runtime DLL. Initializes the one GC that DLL owns.
QREPL_DATA void q_repl_host_init(void);

}

inline void q_repl_ensure(int ni, int nf, int nb, int nc) {
    if (ni > q_repl_ni) {
        auto* n = static_cast<long long*>(q_malloc_atomic(sizeof(long long) * ni));
        auto* live = static_cast<unsigned char*>(q_malloc_atomic(ni));
        if (q_repl_ni > 0) {
            std::memcpy(n, q_repl_i64, sizeof(long long) * q_repl_ni);
            std::memcpy(live, q_repl_i64_live, q_repl_ni);
        }
        q_repl_i64 = n;
        q_repl_i64_live = live;
        q_repl_ni = ni;
    }
    if (nf > q_repl_nf) {
        auto* n = static_cast<double*>(q_malloc_atomic(sizeof(double) * nf));
        auto* live = static_cast<unsigned char*>(q_malloc_atomic(nf));
        if (q_repl_nf > 0) {
            std::memcpy(n, q_repl_f64, sizeof(double) * q_repl_nf);
            std::memcpy(live, q_repl_f64_live, q_repl_nf);
        }
        q_repl_f64 = n;
        q_repl_f64_live = live;
        q_repl_nf = nf;
    }
    if (nb > q_repl_nb) {
        auto* n = static_cast<bool*>(q_malloc_atomic(sizeof(bool) * nb));
        auto* live = static_cast<unsigned char*>(q_malloc_atomic(nb));
        if (q_repl_nb > 0) {
            std::memcpy(n, q_repl_bool, sizeof(bool) * q_repl_nb);
            std::memcpy(live, q_repl_bool_live, q_repl_nb);
        }
        q_repl_bool = n;
        q_repl_bool_live = live;
        q_repl_nb = nb;
    }
    if (nc > q_repl_nc) {
        auto* n = static_cast<QCell**>(q_malloc(sizeof(QCell*) * nc));
        if (q_repl_nc > 0) {
            std::memcpy(n, q_repl_cell, sizeof(QCell*) * q_repl_nc);
        }
        for (int i = q_repl_nc; i < nc; i++) {
            n[i] = q_new_unassigned_cell();
        }
        q_repl_cell = n;
        q_repl_nc = nc;
    }
}

inline long long q_repl_i64_get(int slot, const char* name) {
    if (!q_repl_i64_live[slot]) {
        q_runtime_reportf("runtime error: '%s' is used before it is assigned\n", name);
        q_exit(1);
    }
    return q_repl_i64[slot];
}

inline double q_repl_f64_get(int slot, const char* name) {
    if (!q_repl_f64_live[slot]) {
        q_runtime_reportf("runtime error: '%s' is used before it is assigned\n", name);
        q_exit(1);
    }
    return q_repl_f64[slot];
}

inline bool q_repl_bool_get(int slot, const char* name) {
    if (!q_repl_bool_live[slot]) {
        q_runtime_reportf("runtime error: '%s' is used before it is assigned\n", name);
        q_exit(1);
    }
    return q_repl_bool[slot];
}

inline void q_repl_i64_clear(int slot) { q_repl_i64_live[slot] = 0; }
inline void q_repl_f64_clear(int slot) { q_repl_f64_live[slot] = 0; }
inline void q_repl_bool_clear(int slot) { q_repl_bool_live[slot] = 0; }

inline void q_repl_cell_clear(int slot) {
    if (q_repl_cell[slot] != nullptr) {
        q_repl_cell[slot]->value = qv_null();
    }
}

// q_settle_elems gives containers that still have no element type the type
// this input fixed, and checks elements already stored. A mismatch is reported
// at the caller's source location, before any statement of the input runs.
inline void q_settle_elems(QValue v, const QElemType* t, const char* name) {
    if (t == nullptr) {
        return;
    }
    if (v.type == QValue::VAL_LIST && v.data.list_val != nullptr && t->tag == QValue::VAL_LIST) {
        QList* xs = v.data.list_val;
        if (xs->elem == nullptr) {
            if (t->elem != nullptr) {
                for (size_t i = 0; i < xs->size(); i++) {
                    if (!q_elem_is(t->elem, (*xs)[i])) {
                        char buf[160];
                        q_runtime_reportf("runtime error: '%s' holds a %s, but this input makes it a %s\n",
                            name, q_value_type_name((*xs)[i], buf, sizeof buf), t->name);
                        q_exit(1);
                    }
                }
            }
            xs->elem = t->elem;
        }
        if (t->elem != nullptr) {
            for (size_t i = 0; i < xs->size(); i++) {
                q_settle_elems((*xs)[i], t->elem, name);
            }
        }
        return;
    }
    if (v.type == QValue::VAL_DICT && v.data.dict_val != nullptr && t->tag == QValue::VAL_DICT) {
        QDict* d = v.data.dict_val;
        if (d->elem == nullptr) {
            if (t->elem != nullptr) {
                for (auto& e : d->entries) {
                    if (!q_elem_is(t->elem, e.second)) {
                        char buf[160];
                        q_runtime_reportf("runtime error: '%s' holds a %s, but this input makes it a %s\n",
                            name, q_value_type_name(e.second, buf, sizeof buf), t->name);
                        q_exit(1);
                    }
                }
            }
            d->elem = t->elem;
        }
        if (t->elem != nullptr) {
            for (auto& e : d->entries) {
                q_settle_elems(e.second, t->elem, name);
            }
        }
    }
}

#endif
