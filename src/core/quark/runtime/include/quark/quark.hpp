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
#include "core/constructors.hpp"
#include "core/truthy.hpp"

// Operations
#include "ops/arithmetic.hpp"
#include "ops/comparison.hpp"
#include "ops/logical.hpp"

// Type-specific operations
#include "types/string.hpp"
#include "types/list.hpp"
#include "types/function.hpp"

// Built-in functions
#include "builtins/io.hpp"
#include "builtins/conversion.hpp"
#include "builtins/math.hpp"

// Member access (must come after types and builtins)
#include "ops/member.hpp"

#endif // QUARK_RUNTIME_HPP
