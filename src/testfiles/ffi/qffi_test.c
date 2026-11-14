/* libqffi_test: a small C library the FFI integration tests build and link
 * (specs/ffi_v0_1_spec.md §5.2). Plain functions over int64_t, double, bool and
 * const char*. */
#include <string.h>

#include "qffi_test.h"

int64_t qffi_add(int64_t a, int64_t b) { return a + b; }
double qffi_scale(double x, double factor) { return x * factor; }
int64_t qffi_strlen(const char* s) { return (int64_t)strlen(s); }
const char* qffi_version(void) { return "qffi 1.0"; }
bool qffi_is_even(int64_t n) { return n % 2 == 0; }
