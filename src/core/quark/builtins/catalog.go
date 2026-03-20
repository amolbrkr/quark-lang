package builtins

// TypeKey is a frontend-agnostic type hint used by the analyzer when
// building builtin signatures from the shared catalog.
type TypeKey string

const (
	TypeAny              TypeKey = "any"
	TypeInt              TypeKey = "int"
	TypeFloat            TypeKey = "float"
	TypeString           TypeKey = "str"
	TypeBool             TypeKey = "bool"
	TypeVoid             TypeKey = "void"
	TypeListAny          TypeKey = "list_any"
	TypeListInt          TypeKey = "list_int"
	TypeListString       TypeKey = "list_str"
	TypeDictAny          TypeKey = "dict_any"
	TypeVectorAny        TypeKey = "vector_any"
	TypeFileHandle       TypeKey = "file_handle"
	TypeResultAny        TypeKey = "result_any"
	TypeResultString     TypeKey = "result_str"
	TypeResultInt        TypeKey = "result_int"
	TypeResultNull       TypeKey = "result_null"
	TypeResultFileHandle TypeKey = "result_file_handle"
)

// Spec is the single source of truth for builtin definitions.
// ReceiverType is empty for free functions.
// When non-empty, the builtin is a method: x.Method(args) where x has type ReceiverType.
// The receiver is injected as the first argument at codegen time.
type Spec struct {
	Name         string
	Runtime      string
	MinArgs      int
	MaxArgs      int
	ParamTypes   []TypeKey
	ReturnType   TypeKey
	ReceiverType TypeKey // empty = free function; non-empty = method on this type
}

var catalog = []Spec{
	// I/O (free functions)
	{Name: "print", Runtime: "q_print", MinArgs: 1, MaxArgs: 5, ParamTypes: []TypeKey{TypeAny, TypeString, TypeInt, TypeString, TypeString}, ReturnType: TypeVoid},
	{Name: "println", Runtime: "q_println", MinArgs: 1, MaxArgs: 1, ParamTypes: []TypeKey{TypeAny}, ReturnType: TypeVoid},
	{Name: "input", Runtime: "q_input", MinArgs: 0, MaxArgs: 1, ParamTypes: []TypeKey{TypeString}, ReturnType: TypeString},
	{Name: "_file_open", Runtime: "q_file_open", MinArgs: 2, MaxArgs: 3, ParamTypes: []TypeKey{TypeString, TypeString, TypeBool}, ReturnType: TypeResultFileHandle},
	{Name: "_file_read", Runtime: "q_file_read", MinArgs: 2, MaxArgs: 2, ParamTypes: []TypeKey{TypeFileHandle, TypeInt}, ReturnType: TypeResultString},
	{Name: "_file_write", Runtime: "q_file_write", MinArgs: 2, MaxArgs: 2, ParamTypes: []TypeKey{TypeFileHandle, TypeString}, ReturnType: TypeResultInt},
	{Name: "_file_close", Runtime: "q_file_close", MinArgs: 1, MaxArgs: 1, ParamTypes: []TypeKey{TypeFileHandle}, ReturnType: TypeResultNull},
	{Name: "_file_seek", Runtime: "q_file_seek", MinArgs: 3, MaxArgs: 3, ParamTypes: []TypeKey{TypeFileHandle, TypeInt, TypeInt}, ReturnType: TypeResultInt},
	{Name: "_file_exists", Runtime: "q_file_exists", MinArgs: 1, MaxArgs: 1, ParamTypes: []TypeKey{TypeString}, ReturnType: TypeBool},

	// Conversions (free functions)
	{Name: "len", Runtime: "q_len", MinArgs: 1, MaxArgs: 1, ParamTypes: []TypeKey{TypeAny}, ReturnType: TypeInt},
	{Name: "to_str", Runtime: "q_str", MinArgs: 1, MaxArgs: 1, ParamTypes: []TypeKey{TypeAny}, ReturnType: TypeString},
	{Name: "to_int", Runtime: "q_int", MinArgs: 1, MaxArgs: 1, ParamTypes: []TypeKey{TypeAny}, ReturnType: TypeInt},
	{Name: "to_float", Runtime: "q_float", MinArgs: 1, MaxArgs: 1, ParamTypes: []TypeKey{TypeAny}, ReturnType: TypeFloat},
	{Name: "to_bool", Runtime: "q_bool", MinArgs: 1, MaxArgs: 1, ParamTypes: []TypeKey{TypeAny}, ReturnType: TypeBool},
	{Name: "type", Runtime: "q_type", MinArgs: 1, MaxArgs: 1, ParamTypes: []TypeKey{TypeAny}, ReturnType: TypeString},
	{Name: "is_ok", Runtime: "q_is_ok_builtin", MinArgs: 1, MaxArgs: 1, ParamTypes: []TypeKey{TypeAny}, ReturnType: TypeBool},
	{Name: "is_err", Runtime: "q_is_err_builtin", MinArgs: 1, MaxArgs: 1, ParamTypes: []TypeKey{TypeAny}, ReturnType: TypeBool},
	{Name: "unwrap", Runtime: "q_unwrap", MinArgs: 1, MaxArgs: 1, ParamTypes: []TypeKey{TypeAny}, ReturnType: TypeAny},

	// Range (free function)
	{Name: "range", Runtime: "q_range", MinArgs: 1, MaxArgs: 3, ParamTypes: []TypeKey{TypeFloat, TypeFloat, TypeFloat}, ReturnType: TypeListInt},

	// Math (free functions)
	{Name: "abs", Runtime: "q_abs", MinArgs: 1, MaxArgs: 1, ParamTypes: []TypeKey{TypeAny}, ReturnType: TypeAny},
	{Name: "min", Runtime: "q_min", MinArgs: 1, MaxArgs: 2, ParamTypes: []TypeKey{TypeAny, TypeAny}, ReturnType: TypeAny},
	{Name: "max", Runtime: "q_max", MinArgs: 1, MaxArgs: 2, ParamTypes: []TypeKey{TypeAny, TypeAny}, ReturnType: TypeAny},
	{Name: "sum", Runtime: "q_sum", MinArgs: 1, MaxArgs: 1, ParamTypes: []TypeKey{TypeAny}, ReturnType: TypeAny},
	{Name: "sqrt", Runtime: "q_sqrt", MinArgs: 1, MaxArgs: 1, ParamTypes: []TypeKey{TypeFloat}, ReturnType: TypeFloat},
	{Name: "floor", Runtime: "q_floor", MinArgs: 1, MaxArgs: 1, ParamTypes: []TypeKey{TypeFloat}, ReturnType: TypeInt},
	{Name: "ceil", Runtime: "q_ceil", MinArgs: 1, MaxArgs: 1, ParamTypes: []TypeKey{TypeFloat}, ReturnType: TypeInt},
	{Name: "round", Runtime: "q_round", MinArgs: 1, MaxArgs: 1, ParamTypes: []TypeKey{TypeFloat}, ReturnType: TypeInt},

	// String methods (receiver = str)
	{Name: "upper", Runtime: "q_upper", MinArgs: 0, MaxArgs: 0, ParamTypes: []TypeKey{}, ReturnType: TypeString, ReceiverType: TypeString},
	{Name: "lower", Runtime: "q_lower", MinArgs: 0, MaxArgs: 0, ParamTypes: []TypeKey{}, ReturnType: TypeString, ReceiverType: TypeString},
	{Name: "trim", Runtime: "q_trim", MinArgs: 0, MaxArgs: 0, ParamTypes: []TypeKey{}, ReturnType: TypeString, ReceiverType: TypeString},
	{Name: "contains", Runtime: "q_contains", MinArgs: 1, MaxArgs: 1, ParamTypes: []TypeKey{TypeString}, ReturnType: TypeBool, ReceiverType: TypeString},
	{Name: "startswith", Runtime: "q_startswith", MinArgs: 1, MaxArgs: 1, ParamTypes: []TypeKey{TypeString}, ReturnType: TypeBool, ReceiverType: TypeString},
	{Name: "endswith", Runtime: "q_endswith", MinArgs: 1, MaxArgs: 1, ParamTypes: []TypeKey{TypeString}, ReturnType: TypeBool, ReceiverType: TypeString},
	{Name: "replace", Runtime: "q_replace", MinArgs: 2, MaxArgs: 2, ParamTypes: []TypeKey{TypeString, TypeString}, ReturnType: TypeString, ReceiverType: TypeString},
	{Name: "concat", Runtime: "q_str_concat", MinArgs: 1, MaxArgs: 1, ParamTypes: []TypeKey{TypeString}, ReturnType: TypeString, ReceiverType: TypeString},
	{Name: "split", Runtime: "q_split", MinArgs: 1, MaxArgs: 1, ParamTypes: []TypeKey{TypeString}, ReturnType: TypeListString, ReceiverType: TypeString},
	{Name: "slice", Runtime: "q_str_slice", MinArgs: 2, MaxArgs: 2, ParamTypes: []TypeKey{TypeInt, TypeInt}, ReturnType: TypeString, ReceiverType: TypeString},
	// Note: join is on list, not str — runtime signature is q_str_join(list, sep).

	// List methods (receiver = list)
	{Name: "concat", Runtime: "q_list_concat", MinArgs: 1, MaxArgs: 1, ParamTypes: []TypeKey{TypeListAny}, ReturnType: TypeListAny, ReceiverType: TypeListAny},
	{Name: "push", Runtime: "q_push", MinArgs: 1, MaxArgs: 1, ParamTypes: []TypeKey{TypeAny}, ReturnType: TypeListAny, ReceiverType: TypeListAny},
	{Name: "pop", Runtime: "q_pop", MinArgs: 0, MaxArgs: 0, ParamTypes: []TypeKey{}, ReturnType: TypeAny, ReceiverType: TypeListAny},
	{Name: "get", Runtime: "q_get", MinArgs: 1, MaxArgs: 1, ParamTypes: []TypeKey{TypeInt}, ReturnType: TypeAny, ReceiverType: TypeListAny},
	{Name: "set", Runtime: "q_set", MinArgs: 2, MaxArgs: 2, ParamTypes: []TypeKey{TypeInt, TypeAny}, ReturnType: TypeAny, ReceiverType: TypeListAny},
	{Name: "insert", Runtime: "q_insert", MinArgs: 2, MaxArgs: 2, ParamTypes: []TypeKey{TypeInt, TypeAny}, ReturnType: TypeListAny, ReceiverType: TypeListAny},
	{Name: "remove", Runtime: "q_remove", MinArgs: 1, MaxArgs: 1, ParamTypes: []TypeKey{TypeInt}, ReturnType: TypeAny, ReceiverType: TypeListAny},
	{Name: "slice", Runtime: "q_slice", MinArgs: 2, MaxArgs: 2, ParamTypes: []TypeKey{TypeInt, TypeInt}, ReturnType: TypeListAny, ReceiverType: TypeListAny},
	{Name: "reverse", Runtime: "q_reverse", MinArgs: 0, MaxArgs: 0, ParamTypes: []TypeKey{}, ReturnType: TypeListAny, ReceiverType: TypeListAny},
	{Name: "enumerate", Runtime: "q_enumerate", MinArgs: 0, MaxArgs: 0, ParamTypes: []TypeKey{}, ReturnType: TypeListAny, ReceiverType: TypeListAny},
	{Name: "join", Runtime: "q_str_join", MinArgs: 1, MaxArgs: 1, ParamTypes: []TypeKey{TypeString}, ReturnType: TypeString, ReceiverType: TypeListAny},
	{Name: "to_vector", Runtime: "q_to_vector", MinArgs: 0, MaxArgs: 0, ParamTypes: []TypeKey{}, ReturnType: TypeAny, ReceiverType: TypeListAny},

	// Dict methods (receiver = dict)
	{Name: "get", Runtime: "q_dget", MinArgs: 1, MaxArgs: 1, ParamTypes: []TypeKey{TypeAny}, ReturnType: TypeAny, ReceiverType: TypeDictAny},
	{Name: "set", Runtime: "q_dset", MinArgs: 2, MaxArgs: 2, ParamTypes: []TypeKey{TypeAny, TypeAny}, ReturnType: TypeDictAny, ReceiverType: TypeDictAny},
	{Name: "keys", Runtime: "q_dkeys", MinArgs: 0, MaxArgs: 0, ParamTypes: []TypeKey{}, ReturnType: TypeListAny, ReceiverType: TypeDictAny},
	{Name: "values", Runtime: "q_dvalues", MinArgs: 0, MaxArgs: 0, ParamTypes: []TypeKey{}, ReturnType: TypeListAny, ReceiverType: TypeDictAny},
	{Name: "items", Runtime: "q_ditems", MinArgs: 0, MaxArgs: 0, ParamTypes: []TypeKey{}, ReturnType: TypeListAny, ReceiverType: TypeDictAny},

	// Vector methods (receiver = vector)
	{Name: "get", Runtime: "q_get", MinArgs: 1, MaxArgs: 1, ParamTypes: []TypeKey{TypeInt}, ReturnType: TypeAny, ReceiverType: TypeVectorAny},
	{Name: "fillna", Runtime: "q_fillna", MinArgs: 1, MaxArgs: 1, ParamTypes: []TypeKey{TypeAny}, ReturnType: TypeVectorAny, ReceiverType: TypeVectorAny},
	{Name: "astype", Runtime: "q_astype", MinArgs: 1, MaxArgs: 1, ParamTypes: []TypeKey{TypeString}, ReturnType: TypeVectorAny, ReceiverType: TypeVectorAny},
	{Name: "to_list", Runtime: "q_to_list", MinArgs: 0, MaxArgs: 0, ParamTypes: []TypeKey{}, ReturnType: TypeListAny, ReceiverType: TypeVectorAny},

	// Free-function aliases kept for backward compat during transition
	{Name: "enumerate", Runtime: "q_enumerate", MinArgs: 1, MaxArgs: 1, ParamTypes: []TypeKey{TypeAny}, ReturnType: TypeListAny},
	{Name: "vfrom_list", Runtime: "q_to_vector", MinArgs: 1, MaxArgs: 1, ParamTypes: []TypeKey{TypeAny}, ReturnType: TypeAny},
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
