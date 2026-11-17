package builtins

// TypeKey is a frontend-agnostic type hint used by the analyzer when
// building builtin signatures from the shared catalog.
type TypeKey string

const (
	TypeAny        TypeKey = "any"
	TypeInt        TypeKey = "int"
	TypeFloat      TypeKey = "float"
	TypeString     TypeKey = "str"
	TypeBool       TypeKey = "bool"
	TypeVoid       TypeKey = "void"
	TypeListAny    TypeKey = "list_any"
	TypeListInt    TypeKey = "list_int"
	TypeListString TypeKey = "list_str"
	TypeDictAny    TypeKey = "dict_any"
	TypeVectorAny  TypeKey = "vec_any"
	TypeResultAny  TypeKey = "result_any"
	TypeTableAny   TypeKey = "tbl_any"
)

// Spec is the single source of truth for builtin definitions.
// ReceiverType is empty for free functions.
// When non-empty, the builtin is a method: x.Method(args) where x has type ReceiverType.
// The receiver is passed as the first argument (lowering, lower/calls.go).
type Spec struct {
	Name         string
	Runtime      string
	MinArgs      int
	MaxArgs      int
	ParamTypes   []TypeKey
	ReturnType   TypeKey
	ReceiverType TypeKey // empty = free function; non-empty = method on this type
	// Header is the runtime header that defines Runtime when it isn't part
	// of quark.hpp (the table runtime); generated code includes it when it
	// calls the builtin.
	Header string
	// RowSafe is true when the builtin can run for any row, in any order,
	// on several threads, and gives the same result for the same arguments
	// (specs/tables_v0_3_spec.md D41). A builtin is not row-safe until this
	// is set.
	RowSafe bool
	// Mutates is true when the method changes its receiver in place.
	Mutates bool
}

// TableHeader is the table runtime (queries, table methods), which
// quark.hpp doesn't include.
const TableHeader = "quark/table/query.hpp"

var catalog = []Spec{
	// I/O (free functions). print and input are not row-safe.
	{Name: "print", Runtime: "q_print", MinArgs: 1, MaxArgs: 5, ParamTypes: []TypeKey{TypeAny, TypeString, TypeInt, TypeString, TypeString}, ReturnType: TypeVoid},
	{Name: "input", Runtime: "q_input", MinArgs: 0, MaxArgs: 1, ParamTypes: []TypeKey{TypeString}, ReturnType: TypeResultAny},

	// Conversions (free functions)
	{Name: "len", Runtime: "q_len", MinArgs: 1, MaxArgs: 1, ParamTypes: []TypeKey{TypeAny}, ReturnType: TypeInt, RowSafe: true},
	{Name: "to_str", Runtime: "q_str", MinArgs: 1, MaxArgs: 1, ParamTypes: []TypeKey{TypeAny}, ReturnType: TypeString, RowSafe: true},
	{Name: "to_int", Runtime: "q_int", MinArgs: 1, MaxArgs: 1, ParamTypes: []TypeKey{TypeAny}, ReturnType: TypeInt, RowSafe: true},
	{Name: "to_float", Runtime: "q_float", MinArgs: 1, MaxArgs: 1, ParamTypes: []TypeKey{TypeAny}, ReturnType: TypeFloat, RowSafe: true},
	{Name: "to_bool", Runtime: "q_bool", MinArgs: 1, MaxArgs: 1, ParamTypes: []TypeKey{TypeAny}, ReturnType: TypeBool, RowSafe: true},
	{Name: "type", Runtime: "q_type", MinArgs: 1, MaxArgs: 1, ParamTypes: []TypeKey{TypeAny}, ReturnType: TypeString, RowSafe: true},
	{Name: "is_ok", Runtime: "q_is_ok_builtin", MinArgs: 1, MaxArgs: 1, ParamTypes: []TypeKey{TypeAny}, ReturnType: TypeBool, RowSafe: true},
	{Name: "is_err", Runtime: "q_is_err_builtin", MinArgs: 1, MaxArgs: 1, ParamTypes: []TypeKey{TypeAny}, ReturnType: TypeBool, RowSafe: true},
	{Name: "unwrap", Runtime: "q_unwrap", MinArgs: 1, MaxArgs: 1, ParamTypes: []TypeKey{TypeAny}, ReturnType: TypeAny, RowSafe: true},

	// Range (free function)
	{Name: "range", Runtime: "q_range", MinArgs: 1, MaxArgs: 3, ParamTypes: []TypeKey{TypeInt, TypeInt, TypeInt}, ReturnType: TypeListInt, RowSafe: true},

	// Math (free functions)
	{Name: "abs", Runtime: "q_abs", MinArgs: 1, MaxArgs: 1, ParamTypes: []TypeKey{TypeAny}, ReturnType: TypeAny, RowSafe: true},
	{Name: "min", Runtime: "q_min", MinArgs: 1, MaxArgs: 2, ParamTypes: []TypeKey{TypeAny, TypeAny}, ReturnType: TypeAny, RowSafe: true},
	{Name: "max", Runtime: "q_max", MinArgs: 1, MaxArgs: 2, ParamTypes: []TypeKey{TypeAny, TypeAny}, ReturnType: TypeAny, RowSafe: true},
	{Name: "sum", Runtime: "q_sum", MinArgs: 1, MaxArgs: 1, ParamTypes: []TypeKey{TypeAny}, ReturnType: TypeAny, RowSafe: true},
	{Name: "mean", Runtime: "q_mean", MinArgs: 1, MaxArgs: 1, ParamTypes: []TypeKey{TypeAny}, ReturnType: TypeFloat, RowSafe: true},
	{Name: "sd", Runtime: "q_sd", MinArgs: 1, MaxArgs: 1, ParamTypes: []TypeKey{TypeAny}, ReturnType: TypeFloat, RowSafe: true},
	{Name: "all", Runtime: "q_all", MinArgs: 1, MaxArgs: 1, ParamTypes: []TypeKey{TypeAny}, ReturnType: TypeBool, RowSafe: true},
	{Name: "any", Runtime: "q_any", MinArgs: 1, MaxArgs: 1, ParamTypes: []TypeKey{TypeAny}, ReturnType: TypeBool, RowSafe: true},
	{Name: "sqrt", Runtime: "q_sqrt", MinArgs: 1, MaxArgs: 1, ParamTypes: []TypeKey{TypeFloat}, ReturnType: TypeFloat, RowSafe: true},
	{Name: "floor", Runtime: "q_floor", MinArgs: 1, MaxArgs: 1, ParamTypes: []TypeKey{TypeFloat}, ReturnType: TypeInt, RowSafe: true},
	{Name: "ceil", Runtime: "q_ceil", MinArgs: 1, MaxArgs: 1, ParamTypes: []TypeKey{TypeFloat}, ReturnType: TypeInt, RowSafe: true},
	{Name: "round", Runtime: "q_round", MinArgs: 1, MaxArgs: 1, ParamTypes: []TypeKey{TypeFloat}, ReturnType: TypeInt, RowSafe: true},

	// String methods (receiver = str)
	{Name: "upper", Runtime: "q_upper", MinArgs: 0, MaxArgs: 0, ParamTypes: []TypeKey{}, ReturnType: TypeString, ReceiverType: TypeString, RowSafe: true},
	{Name: "lower", Runtime: "q_lower", MinArgs: 0, MaxArgs: 0, ParamTypes: []TypeKey{}, ReturnType: TypeString, ReceiverType: TypeString, RowSafe: true},
	{Name: "trim", Runtime: "q_trim", MinArgs: 0, MaxArgs: 0, ParamTypes: []TypeKey{}, ReturnType: TypeString, ReceiverType: TypeString, RowSafe: true},
	{Name: "contains", Runtime: "q_contains", MinArgs: 1, MaxArgs: 1, ParamTypes: []TypeKey{TypeString}, ReturnType: TypeBool, ReceiverType: TypeString, RowSafe: true},
	{Name: "startswith", Runtime: "q_startswith", MinArgs: 1, MaxArgs: 1, ParamTypes: []TypeKey{TypeString}, ReturnType: TypeBool, ReceiverType: TypeString, RowSafe: true},
	{Name: "endswith", Runtime: "q_endswith", MinArgs: 1, MaxArgs: 1, ParamTypes: []TypeKey{TypeString}, ReturnType: TypeBool, ReceiverType: TypeString, RowSafe: true},
	{Name: "replace", Runtime: "q_replace", MinArgs: 2, MaxArgs: 2, ParamTypes: []TypeKey{TypeString, TypeString}, ReturnType: TypeString, ReceiverType: TypeString, RowSafe: true},
	{Name: "split", Runtime: "q_split", MinArgs: 1, MaxArgs: 1, ParamTypes: []TypeKey{TypeString}, ReturnType: TypeListString, ReceiverType: TypeString, RowSafe: true},
	{Name: "slice", Runtime: "q_str_slice", MinArgs: 2, MaxArgs: 2, ParamTypes: []TypeKey{TypeInt, TypeInt}, ReturnType: TypeString, ReceiverType: TypeString, RowSafe: true},
	// Note: join is on list, not str — runtime signature is q_str_join(list, sep).

	// List methods (receiver = list). push, pop, set, insert, remove, reverse,
	// sort and sort_by change the list, so they are not row-safe.
	{Name: "concat", Runtime: "q_list_concat", MinArgs: 1, MaxArgs: 1, ParamTypes: []TypeKey{TypeListAny}, ReturnType: TypeListAny, ReceiverType: TypeListAny, RowSafe: true},
	{Name: "push", Runtime: "q_push", MinArgs: 1, MaxArgs: 1, ParamTypes: []TypeKey{TypeAny}, ReturnType: TypeListAny, ReceiverType: TypeListAny, Mutates: true},
	{Name: "pop", Runtime: "q_pop", MinArgs: 0, MaxArgs: 0, ParamTypes: []TypeKey{}, ReturnType: TypeAny, ReceiverType: TypeListAny, Mutates: true},
	{Name: "get", Runtime: "q_get", MinArgs: 1, MaxArgs: 1, ParamTypes: []TypeKey{TypeInt}, ReturnType: TypeAny, ReceiverType: TypeListAny, RowSafe: true},
	{Name: "set", Runtime: "q_set", MinArgs: 2, MaxArgs: 2, ParamTypes: []TypeKey{TypeInt, TypeAny}, ReturnType: TypeAny, ReceiverType: TypeListAny, Mutates: true},
	{Name: "insert", Runtime: "q_insert", MinArgs: 2, MaxArgs: 2, ParamTypes: []TypeKey{TypeInt, TypeAny}, ReturnType: TypeListAny, ReceiverType: TypeListAny, Mutates: true},
	{Name: "remove", Runtime: "q_remove", MinArgs: 1, MaxArgs: 1, ParamTypes: []TypeKey{TypeInt}, ReturnType: TypeAny, ReceiverType: TypeListAny, Mutates: true},
	{Name: "slice", Runtime: "q_slice", MinArgs: 2, MaxArgs: 2, ParamTypes: []TypeKey{TypeInt, TypeInt}, ReturnType: TypeListAny, ReceiverType: TypeListAny, RowSafe: true},
	{Name: "reverse", Runtime: "q_reverse", MinArgs: 0, MaxArgs: 0, ParamTypes: []TypeKey{}, ReturnType: TypeListAny, ReceiverType: TypeListAny, Mutates: true},
	{Name: "sort", Runtime: "q_sort", MinArgs: 0, MaxArgs: 0, ParamTypes: []TypeKey{}, ReturnType: TypeListAny, ReceiverType: TypeListAny, Mutates: true},
	{Name: "sort_by", Runtime: "q_sort_by", MinArgs: 1, MaxArgs: 1, ParamTypes: []TypeKey{TypeAny}, ReturnType: TypeListAny, ReceiverType: TypeListAny, Mutates: true},
	{Name: "enumerate", Runtime: "q_enumerate", MinArgs: 0, MaxArgs: 0, ParamTypes: []TypeKey{}, ReturnType: TypeListAny, ReceiverType: TypeListAny, RowSafe: true},
	{Name: "join", Runtime: "q_str_join", MinArgs: 1, MaxArgs: 1, ParamTypes: []TypeKey{TypeString}, ReturnType: TypeString, ReceiverType: TypeListAny, RowSafe: true},
	{Name: "to_vec", Runtime: "q_to_vec", MinArgs: 0, MaxArgs: 0, ParamTypes: []TypeKey{}, ReturnType: TypeAny, ReceiverType: TypeListAny, RowSafe: true},

	// Dict methods (receiver = dict). set changes the dict.
	{Name: "get", Runtime: "q_dget", MinArgs: 1, MaxArgs: 1, ParamTypes: []TypeKey{TypeString}, ReturnType: TypeAny, ReceiverType: TypeDictAny, RowSafe: true},
	{Name: "set", Runtime: "q_dset", MinArgs: 2, MaxArgs: 2, ParamTypes: []TypeKey{TypeString, TypeAny}, ReturnType: TypeDictAny, ReceiverType: TypeDictAny, Mutates: true},
	{Name: "keys", Runtime: "q_dkeys", MinArgs: 0, MaxArgs: 0, ParamTypes: []TypeKey{}, ReturnType: TypeListAny, ReceiverType: TypeDictAny, RowSafe: true},
	{Name: "values", Runtime: "q_dvalues", MinArgs: 0, MaxArgs: 0, ParamTypes: []TypeKey{}, ReturnType: TypeListAny, ReceiverType: TypeDictAny, RowSafe: true},
	{Name: "items", Runtime: "q_ditems", MinArgs: 0, MaxArgs: 0, ParamTypes: []TypeKey{}, ReturnType: TypeListAny, ReceiverType: TypeDictAny, RowSafe: true},

	// Vector methods (receiver = vector)
	{Name: "get", Runtime: "q_vec_get", MinArgs: 1, MaxArgs: 1, ParamTypes: []TypeKey{TypeInt}, ReturnType: TypeAny, ReceiverType: TypeVectorAny, RowSafe: true},
	{Name: "fillna", Runtime: "q_fillna", MinArgs: 1, MaxArgs: 1, ParamTypes: []TypeKey{TypeAny}, ReturnType: TypeVectorAny, ReceiverType: TypeVectorAny, RowSafe: true},
	{Name: "drop_na", Runtime: "q_drop_na", MinArgs: 0, MaxArgs: 0, ParamTypes: []TypeKey{}, ReturnType: TypeVectorAny, ReceiverType: TypeVectorAny, RowSafe: true},

	// Table methods (specs/tables_v0_1_spec.md §6, D25): the first or last n rows
	// (6 by default) as a table of the same row type, and one line per column.
	// describe prints, so it is not row-safe.
	{Name: "head", Runtime: "q_tbl_head_method", MinArgs: 0, MaxArgs: 1, ParamTypes: []TypeKey{TypeInt}, ReturnType: TypeTableAny, ReceiverType: TypeTableAny, Header: TableHeader, RowSafe: true},
	{Name: "tail", Runtime: "q_tbl_tail_method", MinArgs: 0, MaxArgs: 1, ParamTypes: []TypeKey{TypeInt}, ReturnType: TypeTableAny, ReceiverType: TypeTableAny, Header: TableHeader, RowSafe: true},
	{Name: "describe", Runtime: "q_tbl_describe", MinArgs: 0, MaxArgs: 1, ParamTypes: []TypeKey{TypeInt}, ReturnType: TypeVoid, ReceiverType: TypeTableAny, Header: TableHeader},
	{Name: "astype", Runtime: "q_astype", MinArgs: 1, MaxArgs: 1, ParamTypes: []TypeKey{TypeString}, ReturnType: TypeVectorAny, ReceiverType: TypeVectorAny, RowSafe: true},
	{Name: "to_list", Runtime: "q_to_list", MinArgs: 0, MaxArgs: 0, ParamTypes: []TypeKey{}, ReturnType: TypeListAny, ReceiverType: TypeVectorAny, RowSafe: true},
	{Name: "enumerate", Runtime: "q_enumerate", MinArgs: 0, MaxArgs: 0, ParamTypes: []TypeKey{}, ReturnType: TypeListAny, ReceiverType: TypeVectorAny, RowSafe: true},
}

var byName map[string]Spec
var byMethod map[TypeKey]map[string]Spec

func init() {
	byName = make(map[string]Spec, len(catalog))
	byMethod = make(map[TypeKey]map[string]Spec)
	for _, s := range catalog {
		if s.ReceiverType == "" {
			// Free function: index by name. First definition wins.
			if _, exists := byName[s.Name]; !exists {
				byName[s.Name] = s
			}
		} else {
			// Method: index by (ReceiverType, Name).
			if byMethod[s.ReceiverType] == nil {
				byMethod[s.ReceiverType] = make(map[string]Spec)
			}
			byMethod[s.ReceiverType][s.Name] = s
		}
	}
}

func Catalog() []Spec {
	out := make([]Spec, len(catalog))
	copy(out, catalog)
	return out
}

func Lookup(name string) (Spec, bool) {
	s, ok := byName[name]
	return s, ok
}

// IsQueryAggregate reports whether name is a query aggregate (tables v0.3
// §3.1): inside filter, mutate and summarize, a call of one of these names is
// the aggregate when its argument reads the row. count and n_distinct exist
// only as aggregates; the others are also prelude functions.
func IsQueryAggregate(name string) bool {
	switch name {
	case "count", "sum", "mean", "sd", "min", "max", "n_distinct":
		return true
	}
	return false
}

// LookupMethod looks up a method by receiver type and method name.
// receiverType must be one of the TypeKey constants.
func LookupMethod(receiverType TypeKey, methodName string) (Spec, bool) {
	if m, ok := byMethod[receiverType]; ok {
		if s, ok := m[methodName]; ok {
			return s, true
		}
	}
	return Spec{}, false
}
