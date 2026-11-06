/* libqffi_b: built static-only. libqffi_a calls into it, so -lqffi_a must
 * come before -lqffi_b on the link line. */
#include <stdint.h>

int64_t qffi_b_triple(int64_t x) { return 3 * x; }
