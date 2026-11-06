/* libqffi_a: built static-only; depends on libqffi_b. */
#include <stdint.h>

int64_t qffi_b_triple(int64_t x);

int64_t qffi_a_triple_plus_one(int64_t x) { return qffi_b_triple(x) + 1; }
