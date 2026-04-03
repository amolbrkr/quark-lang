// quark/quark.hpp - Quark Runtime Library
// Master include file - includes all runtime components
#ifndef QUARK_RUNTIME_HPP
#define QUARK_RUNTIME_HPP

// Suppress MSVC CRT deprecation warnings for standard C functions
// (strerror, _open, etc.) when compiling with clang on Windows.
#ifdef _WIN32
#ifndef _CRT_SECURE_NO_WARNINGS
#define _CRT_SECURE_NO_WARNINGS
#endif
#endif

// Standard library includes
#include <cstdio>
#include <cstdlib>
#include <cstring>
#include <cmath>
#include <cctype>
#include <cstdarg>
#include <algorithm>
#include <vector>

// Core types and constructors (gc.hpp must precede value.hpp — QList needs q_allocator)
#include "core/gc.hpp"
#include "core/value.hpp"
#include "core/diagnostics.hpp"
#include "core/cell.hpp"
#include "types/closure.hpp"
#include "core/constructors.hpp"

// Type-specific operations
#include "types/string.hpp"
#include "types/vector.hpp"
#include "types/list.hpp"
#include "types/dict.hpp"
#include "types/function.hpp"
#include "types/resource.hpp"
#include "types/struct.hpp"

// Core helpers depending on type definitions
#include "core/truthy.hpp"

// Operations
#include "ops/arithmetic.hpp"
#include "ops/comparison.hpp"
#include "ops/logical.hpp"

// Built-in functions
#include "builtins/io.hpp"
#include "builtins/fileio.hpp"
#include "builtins/conversion.hpp"
#include "builtins/math.hpp"
#include "builtins/dict.hpp"
#include "builtins/strings.hpp"
// Member access (must come after types and builtins)
#include "ops/member.hpp"

// Quark Extensions Interface — unboxing helpers and qext:: namespace
#include "ext/api.hpp"

#endif // QUARK_RUNTIME_HPP
