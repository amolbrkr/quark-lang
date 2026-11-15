// Session slots and the exit hook. Linked into the Unix host, and into the
// Windows runtime DLL (the one copy the input DLLs import).
#include "quark/quark.hpp"
#include "quark/repl/state.hpp"

extern "C" {

long long* q_repl_i64 = nullptr;
unsigned char* q_repl_i64_live = nullptr;
double* q_repl_f64 = nullptr;
unsigned char* q_repl_f64_live = nullptr;
bool* q_repl_bool = nullptr;
unsigned char* q_repl_bool_live = nullptr;
QCell** q_repl_cell = nullptr;
int q_repl_ni = 0;
int q_repl_nf = 0;
int q_repl_nb = 0;
int q_repl_nc = 0;
#ifdef _WIN32
void (*q_exit_hook)(int) = nullptr;

void q_repl_host_init(void) {
    GC_INIT();
}
#endif

}
