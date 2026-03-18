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
type Spec struct {
	Name       string
	Runtime    string
	MinArgs    int
	MaxArgs    int
	ParamTypes []TypeKey
	ReturnType TypeKey
}

var catalog = []Spec{
	// I/O
	{Name: "print", Runtime: "q_print", MinArgs: 1, MaxArgs: 5, ParamTypes: []TypeKey{TypeAny, TypeString, TypeInt, TypeString, TypeString}, ReturnType: TypeVoid},
	{Name: "println", Runtime: "q_println", MinArgs: 1, MaxArgs: 1, ParamTypes: []TypeKey{TypeAny}, ReturnType: TypeVoid},
	{Name: "input", Runtime: "q_input", MinArgs: 0, MaxArgs: 1, ParamTypes: []TypeKey{TypeString}, ReturnType: TypeString},
	{Name: "_file_open", Runtime: "q_file_open", MinArgs: 2, MaxArgs: 3, ParamTypes: []TypeKey{TypeString, TypeString, TypeBool}, ReturnType: TypeResultFileHandle},
	{Name: "_file_read", Runtime: "q_file_read", MinArgs: 2, MaxArgs: 2, ParamTypes: []TypeKey{TypeFileHandle, TypeInt}, ReturnType: TypeResultString},
	{Name: "_file_write", Runtime: "q_file_write", MinArgs: 2, MaxArgs: 2, ParamTypes: []TypeKey{TypeFileHandle, TypeString}, ReturnType: TypeResultInt},
	{Name: "_file_close", Runtime: "q_file_close", MinArgs: 1, MaxArgs: 1, ParamTypes: []TypeKey{TypeFileHandle}, ReturnType: TypeResultNull},
	{Name: "_file_seek", Runtime: "q_file_seek", MinArgs: 3, MaxArgs: 3, ParamTypes: []TypeKey{TypeFileHandle, TypeInt, TypeInt}, ReturnType: TypeResultInt},
	{Name: "_file_exists", Runtime: "q_file_exists", MinArgs: 1, MaxArgs: 1, ParamTypes: []TypeKey{TypeString}, ReturnType: TypeBool},

	// Conversions
	{Name: "len", Runtime: "q_len", MinArgs: 1, MaxArgs: 1, ParamTypes: []TypeKey{TypeAny}, ReturnType: TypeInt},
	{Name: "to_str", Runtime: "q_str", MinArgs: 1, MaxArgs: 1, ParamTypes: []TypeKey{TypeAny}, ReturnType: TypeString},
	{Name: "to_int", Runtime: "q_int", MinArgs: 1, MaxArgs: 1, ParamTypes: []TypeKey{TypeAny}, ReturnType: TypeInt},
	{Name: "to_float", Runtime: "q_float", MinArgs: 1, MaxArgs: 1, ParamTypes: []TypeKey{TypeAny}, ReturnType: TypeFloat},
	{Name: "to_bool", Runtime: "q_bool", MinArgs: 1, MaxArgs: 1, ParamTypes: []TypeKey{TypeAny}, ReturnType: TypeBool},
	{Name: "type", Runtime: "q_type", MinArgs: 1, MaxArgs: 1, ParamTypes: []TypeKey{TypeAny}, ReturnType: TypeString},
	{Name: "is_ok", Runtime: "q_is_ok_builtin", MinArgs: 1, MaxArgs: 1, ParamTypes: []TypeKey{TypeAny}, ReturnType: TypeBool},
	{Name: "is_err", Runtime: "q_is_err_builtin", MinArgs: 1, MaxArgs: 1, ParamTypes: []TypeKey{TypeAny}, ReturnType: TypeBool},
	{Name: "unwrap", Runtime: "q_unwrap", MinArgs: 1, MaxArgs: 1, ParamTypes: []TypeKey{TypeAny}, ReturnType: TypeAny},

	// Range
	{Name: "range", Runtime: "q_range", MinArgs: 1, MaxArgs: 3, ParamTypes: []TypeKey{TypeFloat, TypeFloat, TypeFloat}, ReturnType: TypeListInt},

	// Math
	{Name: "abs", Runtime: "q_abs", MinArgs: 1, MaxArgs: 1, ParamTypes: []TypeKey{TypeAny}, ReturnType: TypeAny},
	{Name: "min", Runtime: "q_min", MinArgs: 1, MaxArgs: 2, ParamTypes: []TypeKey{TypeAny, TypeAny}, ReturnType: TypeAny},
	{Name: "max", Runtime: "q_max", MinArgs: 1, MaxArgs: 2, ParamTypes: []TypeKey{TypeAny, TypeAny}, ReturnType: TypeAny},
	{Name: "sum", Runtime: "q_sum", MinArgs: 1, MaxArgs: 1, ParamTypes: []TypeKey{TypeAny}, ReturnType: TypeAny},
	{Name: "sqrt", Runtime: "q_sqrt", MinArgs: 1, MaxArgs: 1, ParamTypes: []TypeKey{TypeFloat}, ReturnType: TypeFloat},
	{Name: "floor", Runtime: "q_floor", MinArgs: 1, MaxArgs: 1, ParamTypes: []TypeKey{TypeFloat}, ReturnType: TypeInt},
	{Name: "ceil", Runtime: "q_ceil", MinArgs: 1, MaxArgs: 1, ParamTypes: []TypeKey{TypeFloat}, ReturnType: TypeInt},
	{Name: "round", Runtime: "q_round", MinArgs: 1, MaxArgs: 1, ParamTypes: []TypeKey{TypeFloat}, ReturnType: TypeInt},

	// String
	{Name: "supper", Runtime: "q_upper", MinArgs: 1, MaxArgs: 1, ParamTypes: []TypeKey{TypeString}, ReturnType: TypeString},
	{Name: "slower", Runtime: "q_lower", MinArgs: 1, MaxArgs: 1, ParamTypes: []TypeKey{TypeString}, ReturnType: TypeString},
	{Name: "strim", Runtime: "q_trim", MinArgs: 1, MaxArgs: 1, ParamTypes: []TypeKey{TypeString}, ReturnType: TypeString},
	{Name: "scontains", Runtime: "q_contains", MinArgs: 2, MaxArgs: 2, ParamTypes: []TypeKey{TypeString, TypeString}, ReturnType: TypeBool},
	{Name: "sstartswith", Runtime: "q_startswith", MinArgs: 2, MaxArgs: 2, ParamTypes: []TypeKey{TypeString, TypeString}, ReturnType: TypeBool},
	{Name: "sendswith", Runtime: "q_endswith", MinArgs: 2, MaxArgs: 2, ParamTypes: []TypeKey{TypeString, TypeString}, ReturnType: TypeBool},
	{Name: "sreplace", Runtime: "q_replace", MinArgs: 3, MaxArgs: 3, ParamTypes: []TypeKey{TypeString, TypeString, TypeString}, ReturnType: TypeString},
	{Name: "sconcat", Runtime: "q_str_concat", MinArgs: 2, MaxArgs: 2, ParamTypes: []TypeKey{TypeString, TypeString}, ReturnType: TypeString},
	{Name: "ssplit", Runtime: "q_split", MinArgs: 2, MaxArgs: 2, ParamTypes: []TypeKey{TypeString, TypeString}, ReturnType: TypeListString},

	// List
	{Name: "lconcat", Runtime: "q_list_concat", MinArgs: 2, MaxArgs: 2, ParamTypes: []TypeKey{TypeListAny, TypeListAny}, ReturnType: TypeListAny},
	{Name: "lpush", Runtime: "q_push", MinArgs: 2, MaxArgs: 2, ParamTypes: []TypeKey{TypeListAny, TypeAny}, ReturnType: TypeListAny},
	{Name: "lpop", Runtime: "q_pop", MinArgs: 1, MaxArgs: 1, ParamTypes: []TypeKey{TypeListAny}, ReturnType: TypeAny},
	{Name: "lget", Runtime: "q_get", MinArgs: 2, MaxArgs: 2, ParamTypes: []TypeKey{TypeListAny, TypeInt}, ReturnType: TypeAny},
	{Name: "lset", Runtime: "q_set", MinArgs: 3, MaxArgs: 3, ParamTypes: []TypeKey{TypeListAny, TypeInt, TypeAny}, ReturnType: TypeAny},
	{Name: "linsert", Runtime: "q_insert", MinArgs: 3, MaxArgs: 3, ParamTypes: []TypeKey{TypeListAny, TypeInt, TypeAny}, ReturnType: TypeListAny},
	{Name: "lremove", Runtime: "q_remove", MinArgs: 2, MaxArgs: 2, ParamTypes: []TypeKey{TypeListAny, TypeInt}, ReturnType: TypeAny},
	{Name: "lslice", Runtime: "q_slice", MinArgs: 3, MaxArgs: 3, ParamTypes: []TypeKey{TypeListAny, TypeInt, TypeInt}, ReturnType: TypeListAny},
	{Name: "lreverse", Runtime: "q_reverse", MinArgs: 1, MaxArgs: 1, ParamTypes: []TypeKey{TypeListAny}, ReturnType: TypeListAny},
	{Name: "enumerate", Runtime: "q_enumerate", MinArgs: 1, MaxArgs: 1, ParamTypes: []TypeKey{TypeAny}, ReturnType: TypeListAny},

	// Dict
	{Name: "dget", Runtime: "q_dget", MinArgs: 2, MaxArgs: 2, ParamTypes: []TypeKey{TypeDictAny, TypeAny}, ReturnType: TypeAny},
	{Name: "dset", Runtime: "q_dset", MinArgs: 3, MaxArgs: 3, ParamTypes: []TypeKey{TypeDictAny, TypeAny, TypeAny}, ReturnType: TypeDictAny},
	{Name: "dkeys", Runtime: "q_dkeys", MinArgs: 1, MaxArgs: 1, ParamTypes: []TypeKey{TypeDictAny}, ReturnType: TypeListAny},
	{Name: "dvalues", Runtime: "q_dvalues", MinArgs: 1, MaxArgs: 1, ParamTypes: []TypeKey{TypeDictAny}, ReturnType: TypeListAny},
	{Name: "ditems", Runtime: "q_ditems", MinArgs: 1, MaxArgs: 1, ParamTypes: []TypeKey{TypeDictAny}, ReturnType: TypeListAny},

	// Vector
	{Name: "vget", Runtime: "q_get", MinArgs: 2, MaxArgs: 2, ParamTypes: []TypeKey{TypeVectorAny, TypeInt}, ReturnType: TypeAny},
	{Name: "vfillna", Runtime: "q_fillna", MinArgs: 2, MaxArgs: 2, ParamTypes: []TypeKey{TypeVectorAny, TypeAny}, ReturnType: TypeVectorAny},
	{Name: "vastype", Runtime: "q_astype", MinArgs: 2, MaxArgs: 2, ParamTypes: []TypeKey{TypeVectorAny, TypeString}, ReturnType: TypeVectorAny},
	{Name: "vfrom_list", Runtime: "q_to_vector", MinArgs: 1, MaxArgs: 1, ParamTypes: []TypeKey{TypeAny}, ReturnType: TypeAny},
	{Name: "vto_list", Runtime: "q_to_list", MinArgs: 1, MaxArgs: 1, ParamTypes: []TypeKey{TypeAny}, ReturnType: TypeAny},
}

var byName map[string]Spec

func init() {
	byName = make(map[string]Spec, len(catalog))
	for _, s := range catalog {
		byName[s.Name] = s
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
