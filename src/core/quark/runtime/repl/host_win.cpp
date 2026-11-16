// Windows REPL host. Windows has no fork, so an input runs in this process.
// The session values are copied first. A runtime error (q_exit) or an access
// violation is caught and the copy is put back, so the input leaves nothing
// behind. Unix keeps using host.cpp and fork.
#include "quark/quark.hpp"
#include "quark/repl/state.hpp"

#include <cstdio>
#include <cstring>
#include <string>
#include <unordered_map>
#define WIN32_LEAN_AND_MEAN
#define NOMINMAX
#include <windows.h>

static const DWORD kAbort = 0xE0C0FFEE;
static const DWORD kInterrupt = 0xE0C0FFEF;
static const char kMark[] = "@@QUARK@@ ";

static void write_status(const char* s) {
    std::fprintf(stderr, "%s%s\n", kMark, s);
    std::fflush(stderr);
}

static int read_line(std::string& out) {
    out.clear();
    for (;;) {
        int c = std::getchar();
        if (c == EOF) return 0;
        if (c == '\n') return 1;
        if (c != '\r') out.push_back(static_cast<char>(c));
    }
}

// A copy of every session value, taken before an input runs. Containers that
// an input can change (lists, dicts, structs, closures and the cells they
// capture) are copied. Strings, vectors and tables are immutable, so the copy
// shares them.
struct Checkpoint {
    int ni = 0, nf = 0, nb = 0, nc = 0;
    long long* i64 = nullptr;
    unsigned char* i64_live = nullptr;
    double* f64 = nullptr;
    unsigned char* f64_live = nullptr;
    bool* b = nullptr;
    unsigned char* b_live = nullptr;
    QValue* cells = nullptr;
};

static std::unordered_map<const void*, QValue> memo;

static QValue copy_value(QValue v);

static QCell* copy_cell(QCell* cell) {
    if (cell == nullptr) return nullptr;
    auto it = memo.find(cell);
    if (it != memo.end()) return static_cast<QCell*>(it->second.data.func_val);
    QCell* n = q_new_cell(qv_null());
    QValue hold;
    hold.type = QValue::VAL_FUNC;
    hold.data.func_val = n;
    memo[cell] = hold;
    n->value = copy_value(cell->value);
    return n;
}

static QValue copy_value(QValue v) {
    switch (v.type) {
    case QValue::VAL_LIST: {
        QList* old = v.data.list_val;
        if (old == nullptr) return v;
        auto it = memo.find(old);
        if (it != memo.end()) return it->second;
        QList* n = q_new<QList>();
        n->elem = old->elem;
        QValue out;
        out.type = QValue::VAL_LIST;
        out.data.list_val = n;
        memo[old] = out;
        n->reserve(old->size());
        for (const QValue& e : *old) n->push_back(copy_value(e));
        return out;
    }
    case QValue::VAL_DICT: {
        QDict* old = v.data.dict_val;
        if (old == nullptr) return v;
        auto it = memo.find(old);
        if (it != memo.end()) return it->second;
        QDict* n = q_new<QDict>();
        n->elem = old->elem;
        QValue out;
        out.type = QValue::VAL_DICT;
        out.data.dict_val = n;
        memo[old] = out;
        for (const auto& e : old->entries) n->entries.emplace(e.first, copy_value(e.second));
        return out;
    }
    case QValue::VAL_STRUCT: {
        QStruct* old = v.data.struct_val;
        if (old == nullptr || old->def == nullptr) return v;
        auto it = memo.find(old);
        if (it != memo.end()) return it->second;
        QStruct* n = q_struct_create(old->def);
        QValue out;
        out.type = QValue::VAL_STRUCT;
        out.data.struct_val = n;
        memo[old] = out;
        for (int i = 0; i < old->def->field_count; i++) n->fields[i] = copy_value(old->fields[i]);
        return out;
    }
    case QValue::VAL_FUNC: {
        auto* old = static_cast<QClosure*>(v.data.func_val);
        if (old == nullptr) return v;
        auto it = memo.find(old);
        if (it != memo.end()) return it->second;
        QClosure* n = q_alloc_closure(old->func, old->info, old->capture_count);
        QValue out;
        out.type = QValue::VAL_FUNC;
        out.data.func_val = n;
        memo[old] = out;
        for (int i = 0; i < old->capture_count; i++) n->captures[i] = copy_cell(old->captures[i]);
        return out;
    }
    case QValue::VAL_RESULT: {
        QResult* old = v.data.result_val;
        if (old == nullptr) return v;
        auto it = memo.find(old);
        if (it != memo.end()) return it->second;
        QResult* n = q_new<QResult>();
        QValue out;
        out.type = QValue::VAL_RESULT;
        out.data.result_val = n;
        memo[old] = out;
        n->is_ok = old->is_ok;
        n->payload = copy_value(old->payload);
        return out;
    }
    default:
        return v;
    }
}

static Checkpoint checkpoint;

static void save_checkpoint() {
    GC_disable();
    memo.clear();
    checkpoint.ni = q_repl_ni;
    checkpoint.nf = q_repl_nf;
    checkpoint.nb = q_repl_nb;
    checkpoint.nc = q_repl_nc;
    auto dup = [](const void* p, size_t n) -> void* {
        if (n == 0 || p == nullptr) return nullptr;
        void* d = std::malloc(n);
        if (d != nullptr) std::memcpy(d, p, n);
        return d;
    };
    checkpoint.i64 = static_cast<long long*>(dup(q_repl_i64, sizeof(long long) * q_repl_ni));
    checkpoint.i64_live = static_cast<unsigned char*>(dup(q_repl_i64_live, q_repl_ni));
    checkpoint.f64 = static_cast<double*>(dup(q_repl_f64, sizeof(double) * q_repl_nf));
    checkpoint.f64_live = static_cast<unsigned char*>(dup(q_repl_f64_live, q_repl_nf));
    checkpoint.b = static_cast<bool*>(dup(q_repl_bool, sizeof(bool) * q_repl_nb));
    checkpoint.b_live = static_cast<unsigned char*>(dup(q_repl_bool_live, q_repl_nb));
    std::free(checkpoint.cells);
    checkpoint.cells = q_repl_nc > 0 ? static_cast<QValue*>(std::malloc(sizeof(QValue) * q_repl_nc)) : nullptr;
    for (int i = 0; i < q_repl_nc; i++) {
        QValue cur = q_repl_cell[i] != nullptr ? q_repl_cell[i]->value : qv_null();
        checkpoint.cells[i] = copy_value(cur);
    }
    if (checkpoint.cells != nullptr) {
        GC_add_roots(checkpoint.cells, checkpoint.cells + checkpoint.nc);
    }
    memo.clear();
    GC_enable();
}

static void drop_checkpoint() {
    if (checkpoint.cells != nullptr) {
        GC_remove_roots(checkpoint.cells, checkpoint.cells + checkpoint.nc);
    }
    std::free(checkpoint.i64);
    std::free(checkpoint.i64_live);
    std::free(checkpoint.f64);
    std::free(checkpoint.f64_live);
    std::free(checkpoint.b);
    std::free(checkpoint.b_live);
    std::free(checkpoint.cells);
    checkpoint = {};
}

static void restore_checkpoint() {
    auto put = [](void* dst, const void* src, size_t n) {
        if (n > 0 && dst != nullptr && src != nullptr) std::memcpy(dst, src, n);
    };
    put(q_repl_i64, checkpoint.i64, sizeof(long long) * checkpoint.ni);
    put(q_repl_i64_live, checkpoint.i64_live, checkpoint.ni);
    put(q_repl_f64, checkpoint.f64, sizeof(double) * checkpoint.nf);
    put(q_repl_f64_live, checkpoint.f64_live, checkpoint.nf);
    put(q_repl_bool, checkpoint.b, sizeof(bool) * checkpoint.nb);
    put(q_repl_bool_live, checkpoint.b_live, checkpoint.nb);
    for (int i = 0; i < checkpoint.nc; i++) {
        if (q_repl_cell[i] != nullptr) q_repl_cell[i]->value = checkpoint.cells[i];
    }
    q_repl_ni = checkpoint.ni;
    q_repl_nf = checkpoint.nf;
    q_repl_nb = checkpoint.nb;
    q_repl_nc = checkpoint.nc;
    drop_checkpoint();
}

static int filter_code(unsigned long code) {
    if (code == kAbort || code == kInterrupt || code == EXCEPTION_ACCESS_VIOLATION || code == EXCEPTION_INT_DIVIDE_BY_ZERO) {
        return EXCEPTION_EXECUTE_HANDLER;
    }
    return EXCEPTION_CONTINUE_SEARCH;
}

extern "C" int q_call_guarded(void (*fn)()) {
    __try {
        fn();
        return 0;
    } __except (filter_code(GetExceptionCode())) {
        if (GetExceptionCode() == kInterrupt) return 3;
        return GetExceptionCode() == kAbort ? 1 : 2;
    }
}

static void abort_input(int) {
    RaiseException(kAbort, 0, 0, nullptr);
}

static void raise_interrupt() {
    RaiseException(kInterrupt, 0, 0, nullptr);
}

static volatile long input_running = 0;
static DWORD main_tid = 0;

static void raise_interrupt();

// Ctrl-C is delivered on another thread. Point the input's thread at
// abort_input so the guarded call returns and the checkpoint is restored.
static BOOL WINAPI on_ctrl(DWORD kind) {
    if (kind != CTRL_C_EVENT && kind != CTRL_BREAK_EVENT) return FALSE;
    if (!input_running) return TRUE;
    HANDLE th = OpenThread(THREAD_SUSPEND_RESUME | THREAD_GET_CONTEXT | THREAD_SET_CONTEXT, FALSE, main_tid);
    if (th == nullptr) return TRUE;
    SuspendThread(th);
    CONTEXT ctx;
    std::memset(&ctx, 0, sizeof ctx);
    ctx.ContextFlags = CONTEXT_CONTROL;
    if (GetThreadContext(th, &ctx)) {
        ctx.Rip = reinterpret_cast<DWORD64>(raise_interrupt);
        SetThreadContext(th, &ctx);
    }
    ResumeThread(th);
    CloseHandle(th);
    return TRUE;
}

static int run_library(const std::string& path, const std::string& symbol) {
    HMODULE lib = LoadLibraryA(path.c_str());
    if (lib == nullptr) {
        std::fprintf(stderr, "error[QK-REPL-001] (repl): could not load the input (%lu)\n", GetLastError());
        return 1;
    }
    auto fn = reinterpret_cast<void (*)()>(GetProcAddress(lib, symbol.c_str()));
    if (fn == nullptr) {
        std::fprintf(stderr, "error[QK-REPL-001] (repl): the input has no entry %s\n", symbol.c_str());
        return 1;
    }
    save_checkpoint();
    q_exit_hook = abort_input;
    input_running = 1;
    int rc = q_call_guarded(fn);
    input_running = 0;
    q_exit_hook = nullptr;
    q_set_source_loc(nullptr);
    if (rc == 0) {
        drop_checkpoint();
        return 0;
    }
    restore_checkpoint();
    if (rc == 2) {
        std::fprintf(stderr, "error[QK-REPL-001] (repl): the input stopped with an access violation; the session is as it was before the input\n");
    }
    return rc;
}

int main() {
    main_tid = GetCurrentThreadId();
    SetConsoleCtrlHandler(on_ctrl, TRUE);
    q_repl_host_init();
    std::string line;
    while (read_line(line) == 1) {
        if (line == "quit") return 0;
        if (line.compare(0, 4, "run\t") != 0) {
            write_status("fail");
            continue;
        }
        std::string rest = line.substr(4);
        auto tab = rest.rfind('\t');
        if (tab == std::string::npos) {
            write_status("fail");
            continue;
        }
        int rc = run_library(rest.substr(0, tab), rest.substr(tab + 1));
        // The input's output is shown before the status that ends it. The
        // host and every input share one C runtime (toolchain/repl.go), so
        // this is the stdout that print wrote to.
        std::fflush(stdout);
        if (rc == 0) write_status("ok");
        else if (rc == 3) write_status("interrupted");
        else if (rc == 2) write_status("signal");
        else write_status("fail");
    }
    return 0;
}
