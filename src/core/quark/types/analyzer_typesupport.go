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
		case "file_handle":
			return builtins.TypeFileHandle
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
	case builtins.TypeFileHandle:
		return TypeFileHandle
	case builtins.TypeResultAny:
		return &ResultType{OkType: TypeAny, ErrType: TypeString}
	case builtins.TypeResultString:
		return &ResultType{OkType: TypeString, ErrType: TypeString}
	case builtins.TypeResultInt:
		return &ResultType{OkType: TypeInt, ErrType: TypeString}
	case builtins.TypeResultNull:
		return &ResultType{OkType: TypeNull, ErrType: TypeString}
	case builtins.TypeResultFileHandle:
		return &ResultType{OkType: TypeFileHandle, ErrType: TypeString}
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

	name := node.TokenLiteral()
	switch name {
	case "int":
		return TypeInt
	case "float":
		return TypeFloat
	case "str":
		return TypeString
	case "bool":
		return TypeBool
	case "null":
		return TypeNull
	case "any":
		return TypeAny
	case "list":
		return &ListType{ElementType: TypeAny}
	case "dict":
		return &DictType{KeyType: TypeAny, ValueType: TypeAny}
	case "vector":
		return &VectorType{ElementType: TypeAny}
	case "result":
		return &ResultType{OkType: TypeAny, ErrType: TypeAny}
	case "resource":
		return TypeResource
	case "file_handle":
		return TypeFileHandle
	default:
		a.errorAt(node, "unknown type '%s'", name)
		return TypeError
	}
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
	return isUnknownType(t)
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
	return isUnknownType(t)
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
	return isUnknownType(t)
}
