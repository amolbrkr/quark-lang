/* qffi_test.h - C API of the FFI test library (libqffi_test). */
#ifndef QFFI_TEST_H
#define QFFI_TEST_H

#include <stdbool.h>
#include <stdint.h>

int64_t qffi_add(int64_t a, int64_t b);
double qffi_scale(double x, double factor);
int64_t qffi_strlen(const char* s);
const char* qffi_version(void);
bool qffi_is_even(int64_t n);

#endif
