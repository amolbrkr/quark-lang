// quark/quark.hpp - Quark Runtime Library
// Master include file - includes all runtime components
#ifndef QUARK_RUNTIME_HPP
#define QUARK_RUNTIME_HPP

// Standard library includes
#include <cstdio>
#include <cstdlib>
#include <cstring>
#include <cmath>
#include <cctype>
#include <cstdarg>
#include <algorithm>
#include <vector>

// Core types and constructors
#include "core/value.hpp"
#include "core/gc.hpp"
#include "types/closure.hpp"
#include "core/constructors.hpp"

// Type-specific operations
#include "types/string.hpp"
#include "types/list.hpp"
#include "types/vector.hpp"
#include "types/dict.hpp"
#include "types/function.hpp"

// Core helpers depending on type definitions
#include "core/truthy.hpp"

// Operations
#include "ops/arithmetic.hpp"
#include "ops/comparison.hpp"
#include "ops/logical.hpp"

// Built-in functions
#include "builtins/io.hpp"
#include "builtins/conversion.hpp"
#include "builtins/math.hpp"
#include "builtins/dict.hpp"
#include "builtins/strings.hpp"

// Member access (must come after types and builtins)
#include "ops/member.hpp"

#endif // QUARK_RUNTIME_HPP
