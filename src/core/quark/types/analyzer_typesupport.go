package types

import (
	"quark/ast"
	"quark/builtins"
)

func typeToBuiltinTypeKey(t Type) builtins.TypeKey {
	switch v := t.(type) {
	case *BasicType:
		switch v.Name {
		case "str":
			return builtins.TypeString
		case "int":
			return builtins.TypeInt
		case "float":
			return builtins.TypeFloat
		case "bool":
			return builtins.TypeBool
		}
	case *ListType:
		return builtins.TypeListAny
	case *DictType:
		return builtins.TypeDictAny
	case *VectorType:
		return builtins.TypeVectorAny
	case *ResultType:
		return builtins.TypeResultAny
	}
	return ""
}

func mapBuiltinTypeKey(key builtins.TypeKey) Type {
	switch key {
	case builtins.TypeInt:
		return TypeInt
	case builtins.TypeFloat:
		return TypeFloat
	case builtins.TypeString:
		return TypeString
	case builtins.TypeBool:
		return TypeBool
	case builtins.TypeVoid:
		return TypeVoid
	case builtins.TypeListAny:
		return &ListType{ElementType: TypeAny}
	case builtins.TypeListInt:
		return &ListType{ElementType: TypeInt}
	case builtins.TypeListString:
		return &ListType{ElementType: TypeString}
	case builtins.TypeDictAny:
		return &DictType{KeyType: TypeAny, ValueType: TypeAny}
	case builtins.TypeVectorAny:
		return &VectorType{ElementType: TypeAny}
	case builtins.TypeResultAny:
		return &ResultType{OkType: TypeAny, ErrType: TypeString}
	default:
		return TypeAny
	}
}

func (a *Analyzer) resolveTypeNode(node *ast.TreeNode) Type {
	if node == nil {
		return TypeAny
	}
	if node.NodeType != ast.TypeNode {
		return TypeAny
	}
	t, at, msg := annotationType(node, a.structTypes)
	if msg != "" {
		a.errorAt(at, "%s", msg)
		return TypeError
	}
	return t
}

// annotationType resolves the type annotation node (a TypeNode) against the
// struct types. When it names no valid type it returns a message and the
// node to report it at.
func annotationType(node *ast.TreeNode, structTypes map[string]*StructType) (Type, *ast.TreeNode, string) {
	name := node.TokenLiteral()
	if len(node.Children) > 0 && name != "tbl" {
		hint := ""
		switch name {
		case "list", "dict", "vec", "result":
			hint = "; element types aren't written in annotations yet (write '" + name + "')"
		}
		return nil, node, "'" + name + "' takes no type argument" + hint
	}
	switch name {
	case "int":
		return TypeInt, nil, ""
	case "float":
		return TypeFloat, nil, ""
	case "str":
		return TypeString, nil, ""
	case "bool":
		return TypeBool, nil, ""
	case "null":
		return TypeNull, nil, ""
	case "any":
		return TypeAny, nil, ""
	case "list":
		return &ListType{ElementType: TypeAny}, nil, ""
	case "dict":
		return &DictType{KeyType: TypeAny, ValueType: TypeAny}, nil, ""
	case "vec":
		return &VectorType{ElementType: TypeAny}, nil, ""
	case "result":
		return &ResultType{OkType: TypeAny, ErrType: TypeAny}, nil, ""
	case "resource":
		return TypeResource, nil, ""
	case "fn":
		return TypeFn, nil, ""
	case "tbl":
		if len(node.Children) == 0 {
			return &TableType{}, nil, ""
		}
		return tableAnnotation(node.Children[0], structTypes)
	}
	if st, ok := structTypes[name]; ok {
		return st, nil, ""
	}
	return nil, node, "unknown type '" + name + "'"
}

// tableAnnotation resolves R in tbl[R]: a struct whose fields all have
// column types (tables_v0_1_spec.md §3).
func tableAnnotation(arg *ast.TreeNode, structTypes map[string]*StructType) (Type, *ast.TreeNode, string) {
	t, at, msg := annotationType(arg, structTypes)
	if msg != "" {
		return nil, at, msg
	}
	st, ok := t.(*StructType)
	if !ok {
		return nil, arg, "a table's row type is a struct, not " + t.String() + ": write tbl[Employee] for 'struct Employee:' with one field per column"
	}
	for _, f := range st.Fields {
		if !IsColumnType(f.Type) && !IsErrorType(f.Type) {
			return nil, arg, "struct " + st.Name + " can't be a table row: field '" + f.Name + "' is " + f.Type.String() + ", and table columns are int, float, bool or str"
		}
	}
	return &TableType{Row: st}, nil, ""
}

func isUnknownType(t Type) bool {
	if t == nil {
		return true
	}
	if IsErrorType(t) {
		return true
	}
	if t.Equals(TypeAny) {
		return true
	}
	if union, ok := t.(*UnionType); ok {
		for _, opt := range union.Options {
			if isUnknownType(opt) {
				return true
			}
		}
	}
	return false
}

// isBoolLike reports whether every value of type t is a bool. A value of
// unknown type is not: `id(0) or id(5)` is 5, so an `and`/`or` of it isn't
// typed bool.
func isBoolLike(t Type) bool {
	if t == nil {
		return false
	}
	if t.Equals(TypeBool) {
		return true
	}
	if union, ok := t.(*UnionType); ok {
		if len(union.Options) == 0 {
			return false
		}
		for _, opt := range union.Options {
			if !isBoolLike(opt) {
				return false
			}
		}
		return true
	}
	return false
}

func isStringLike(t Type) bool {
	if t == nil {
		return false
	}
	if t.Equals(TypeString) {
		return true
	}
	if union, ok := t.(*UnionType); ok {
		if len(union.Options) == 0 {
			return false
		}
		for _, opt := range union.Options {
			if !isStringLike(opt) {
				return false
			}
		}
		return true
	}
	return false
}

func isIntLike(t Type) bool {
	if t == nil {
		return false
	}
	if t.Equals(TypeInt) {
		return true
	}
	if union, ok := t.(*UnionType); ok {
		if len(union.Options) == 0 {
			return false
		}
		for _, opt := range union.Options {
			if !isIntLike(opt) {
				return false
			}
		}
		return true
	}
	return false
}
