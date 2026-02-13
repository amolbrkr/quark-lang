package codegen

import (
	"fmt"
	"strings"
)

// BuiltinFunc describes a built-in function available in Quark
type BuiltinFunc struct {
	CFunc   string // C++ function name, e.g. "q_upper"
	MinArgs int    // Minimum args required
	MaxArgs int    // Maximum args accepted
}

// builtinRegistry is the single source of truth for all builtin function mappings.
// Adding a new builtin only requires adding one entry here.
var builtinRegistry = map[string]*BuiltinFunc{
	// I/O
	"print":   {CFunc: "q_print", MinArgs: 0, MaxArgs: 1},
	"println": {CFunc: "q_println", MinArgs: 0, MaxArgs: 1},
	"input":   {CFunc: "q_input", MinArgs: 0, MaxArgs: 1},

	// Conversions
	"len":   {CFunc: "q_len", MinArgs: 1, MaxArgs: 1},
	"str":   {CFunc: "q_str", MinArgs: 1, MaxArgs: 1},
	"int":   {CFunc: "q_int", MinArgs: 1, MaxArgs: 1},
	"float": {CFunc: "q_float", MinArgs: 1, MaxArgs: 1},
	"bool":  {CFunc: "q_bool", MinArgs: 1, MaxArgs: 1},

	// Range (variadic: 1-3 args)
	"range": {CFunc: "q_range", MinArgs: 1, MaxArgs: 3},

	// Math
	"abs":   {CFunc: "q_abs", MinArgs: 1, MaxArgs: 1},
	"min":   {CFunc: "q_min", MinArgs: 1, MaxArgs: 2},
	"max":   {CFunc: "q_max", MinArgs: 1, MaxArgs: 2},
	"sum":   {CFunc: "q_sum", MinArgs: 1, MaxArgs: 1},
	"sqrt":  {CFunc: "q_sqrt", MinArgs: 1, MaxArgs: 1},
	"floor": {CFunc: "q_floor", MinArgs: 1, MaxArgs: 1},
	"ceil":  {CFunc: "q_ceil", MinArgs: 1, MaxArgs: 1},
	"round": {CFunc: "q_round", MinArgs: 1, MaxArgs: 1},

	// String
	"upper":      {CFunc: "q_upper", MinArgs: 1, MaxArgs: 1},
	"lower":      {CFunc: "q_lower", MinArgs: 1, MaxArgs: 1},
	"trim":       {CFunc: "q_trim", MinArgs: 1, MaxArgs: 1},
	"contains":   {CFunc: "q_contains", MinArgs: 2, MaxArgs: 2},
	"startswith": {CFunc: "q_startswith", MinArgs: 2, MaxArgs: 2},
	"endswith":   {CFunc: "q_endswith", MinArgs: 2, MaxArgs: 2},
	"replace":    {CFunc: "q_replace", MinArgs: 3, MaxArgs: 3},
	"concat":     {CFunc: "q_concat", MinArgs: 2, MaxArgs: 2},
	"split":      {CFunc: "q_split", MinArgs: 2, MaxArgs: 2},

	// List
	"push":    {CFunc: "q_push", MinArgs: 2, MaxArgs: 2},
	"pop":     {CFunc: "q_pop", MinArgs: 1, MaxArgs: 1},
	"get":     {CFunc: "q_get", MinArgs: 2, MaxArgs: 2},
	"set":     {CFunc: "q_set", MinArgs: 3, MaxArgs: 3},
	"insert":  {CFunc: "q_insert", MinArgs: 3, MaxArgs: 3},
	"remove":  {CFunc: "q_remove", MinArgs: 2, MaxArgs: 2},
	"slice":   {CFunc: "q_slice", MinArgs: 3, MaxArgs: 3},
	"reverse": {CFunc: "q_reverse", MinArgs: 1, MaxArgs: 1},

	// Dict helpers
	"dget": {CFunc: "q_dget", MinArgs: 2, MaxArgs: 2},
	"dset": {CFunc: "q_dset", MinArgs: 3, MaxArgs: 3},

	// Vector helpers
	"vadd_inplace": {CFunc: "q_vadd_inplace", MinArgs: 2, MaxArgs: 2},
	"fillna":       {CFunc: "q_fillna", MinArgs: 2, MaxArgs: 2},
	"astype":       {CFunc: "q_astype", MinArgs: 2, MaxArgs: 2},
}

// LookupBuiltin returns the builtin definition if name is a builtin, nil otherwise.
func LookupBuiltin(name string) *BuiltinFunc {
	return builtinRegistry[name]
}

// GenerateBuiltinCall generates a C++ call for a builtin function.
// Returns the generated code and true if name is a builtin, or ("", false) otherwise.
func GenerateBuiltinCall(name string, args []string) (string, bool) {
	b := builtinRegistry[name]
	if b == nil {
		return "", false
	}

	nargs := len(args)

	// Too few arguments — return qv_null() fallback
	if nargs < b.MinArgs {
		return "qv_null()", true
	}

	// Clamp to MaxArgs (ignore extra args)
	if nargs > b.MaxArgs {
		args = args[:b.MaxArgs]
	}

	return fmt.Sprintf("%s(%s)", b.CFunc, strings.Join(args, ", ")), true
}
