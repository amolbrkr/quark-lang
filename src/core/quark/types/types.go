package types

import (
	"fmt"
	"sort"
	"strings"
)

// Type represents a Quark type
type Type interface {
	String() string
	Equals(other Type) bool
}

// Basic types
type BasicType struct {
	Name string
}

func (t *BasicType) String() string {
	return t.Name
}

func (t *BasicType) Equals(other Type) bool {
	if o, ok := other.(*BasicType); ok {
		return t.Name == o.Name
	}
	return false
}

// Predefined basic types
var (
	TypeInt    = &BasicType{Name: "int"}
	TypeFloat  = &BasicType{Name: "float"}
	TypeString = &BasicType{Name: "str"}
	TypeBool   = &BasicType{Name: "bool"}
	TypeNull   = &BasicType{Name: "null"}
	TypeAny    = &BasicType{Name: "any"}  // For unresolved types
	TypeVoid   = &BasicType{Name: "void"} // For statements with no value
)

// ListType represents a list of elements
type ListType struct {
	ElementType Type
}

func (t *ListType) String() string {
	return fmt.Sprintf("list[%s]", t.ElementType.String())
}

// VectorType represents a homogeneous numeric vector (MVP: float element type)
type VectorType struct {
	ElementType Type
}

func (t *VectorType) String() string {
	return fmt.Sprintf("vector[%s]", t.ElementType.String())
}

func (t *VectorType) Equals(other Type) bool {
	if o, ok := other.(*VectorType); ok {
		return t.ElementType.Equals(o.ElementType)
	}
	return false
}

func (t *ListType) Equals(other Type) bool {
	if o, ok := other.(*ListType); ok {
		return t.ElementType.Equals(o.ElementType)
	}
	return false
}

// DictType represents a dictionary
type DictType struct {
	KeyType   Type
	ValueType Type
}

func (t *DictType) String() string {
	return fmt.Sprintf("dict[%s, %s]", t.KeyType.String(), t.ValueType.String())
}

func (t *DictType) Equals(other Type) bool {
	if o, ok := other.(*DictType); ok {
		return t.KeyType.Equals(o.KeyType) && t.ValueType.Equals(o.ValueType)
	}
	return false
}

// FunctionType represents a function signature
type FunctionType struct {
	ParamTypes          []Type
	ReturnType          Type
	AnnotatedReturnType Type   // Declared return type (nil = not annotated)
	DefaultCount        int    // Number of parameters with default values (always trailing)
	DefaultValues       []*DefaultValueInfo // Default value info per parameter (nil = no default)
}

// DefaultValueInfo stores info about a default parameter value for codegen
type DefaultValueInfo struct {
	Node interface{} // The AST node for the default expression (used by codegen)
}

// ResultType represents an explicit ok/err result value
type ResultType struct {
	OkType  Type
	ErrType Type
}

func (t *FunctionType) String() string {
	params := ""
	for i, p := range t.ParamTypes {
		if i > 0 {
			params += ", "
		}
		params += p.String()
	}
	retType := t.ReturnType
	if t.AnnotatedReturnType != nil {
		retType = t.AnnotatedReturnType
	}
	return fmt.Sprintf("fn(%s) -> %s", params, retType.String())
}

// MinArity returns the minimum number of arguments required (total params - defaults)
func (t *FunctionType) MinArity() int {
	return len(t.ParamTypes) - t.DefaultCount
}

func (t *FunctionType) Equals(other Type) bool {
	if o, ok := other.(*FunctionType); ok {
		if len(t.ParamTypes) != len(o.ParamTypes) {
			return false
		}
		for i, p := range t.ParamTypes {
			if !p.Equals(o.ParamTypes[i]) {
				return false
			}
		}
		return t.ReturnType.Equals(o.ReturnType)
	}
	return false
}

func (t *ResultType) String() string {
	return fmt.Sprintf("result[%s, %s]", t.OkType.String(), t.ErrType.String())
}

func (t *ResultType) Equals(other Type) bool {
	if o, ok := other.(*ResultType); ok {
		return t.OkType.Equals(o.OkType) && t.ErrType.Equals(o.ErrType)
	}
	return false
}

// UnionType represents a value that can be one of several concrete types
type UnionType struct {
	Options []Type
}

func (t *UnionType) String() string {
	parts := make([]string, len(t.Options))
	for i, opt := range t.Options {
		parts[i] = opt.String()
	}
	return fmt.Sprintf("union[%s]", strings.Join(parts, " | "))
}

func (t *UnionType) Equals(other Type) bool {
	if o, ok := other.(*UnionType); ok {
		if len(t.Options) != len(o.Options) {
			return false
		}
		for i, opt := range t.Options {
			if !opt.Equals(o.Options[i]) {
				return false
			}
		}
		return true
	}
	if len(t.Options) == 1 {
		return t.Options[0].Equals(other)
	}
	return false
}

// Symbol represents a variable or function in the symbol table
type Symbol struct {
	Name    string
	Type    Type
	Mutable bool
	Defined bool // Whether it has been assigned a value
}

// Scope represents a lexical scope
type Scope struct {
	Parent  *Scope
	Symbols map[string]*Symbol
}

func NewScope(parent *Scope) *Scope {
	return &Scope{
		Parent:  parent,
		Symbols: make(map[string]*Symbol),
	}
}

func (s *Scope) Define(name string, typ Type, mutable bool) *Symbol {
	sym := &Symbol{
		Name:    name,
		Type:    typ,
		Mutable: mutable,
		Defined: true,
	}
	s.Symbols[name] = sym
	return sym
}

func (s *Scope) Lookup(name string) *Symbol {
	if sym, ok := s.Symbols[name]; ok {
		return sym
	}
	if s.Parent != nil {
		return s.Parent.Lookup(name)
	}
	return nil
}

func (s *Scope) LookupLocal(name string) *Symbol {
	if sym, ok := s.Symbols[name]; ok {
		return sym
	}
	return nil
}

// IsNumeric checks if a type is numeric (int or float)
func IsNumeric(t Type) bool {
	if union, ok := t.(*UnionType); ok {
		if len(union.Options) == 0 {
			return false
		}
		for _, opt := range union.Options {
			if !IsNumeric(opt) {
				return false
			}
		}
		return true
	}
	if basic, ok := t.(*BasicType); ok {
		return basic.Name == "int" || basic.Name == "float"
	}
	return false
}

// IsComparable checks if a type can be compared
func IsComparable(t Type) bool {
	if union, ok := t.(*UnionType); ok {
		if len(union.Options) == 0 {
			return false
		}
		for _, opt := range union.Options {
			if !IsComparable(opt) {
				return false
			}
		}
		return true
	}
	if basic, ok := t.(*BasicType); ok {
		return basic.Name == "int" || basic.Name == "float" || basic.Name == "str" || basic.Name == "bool"
	}
	return false
}

// CanAssign checks if srcType can be assigned to dstType
func CanAssign(dstType, srcType Type) bool {
	// Any type can be assigned to any
	if dstType.Equals(TypeAny) || srcType.Equals(TypeAny) {
		return true
	}
	// Null can be assigned to any reference type
	if srcType.Equals(TypeNull) {
		_, isList := dstType.(*ListType)
		_, isDict := dstType.(*DictType)
		_, isFunc := dstType.(*FunctionType)
		_, isResult := dstType.(*ResultType)
		return isList || isDict || isFunc || isResult
	}
	// Int can be promoted to float
	if dstType.Equals(TypeFloat) && srcType.Equals(TypeInt) {
		return true
	}
	// List covariance: list[any] accepts list[T] for any T
	if dstList, ok := dstType.(*ListType); ok {
		if srcList, ok := srcType.(*ListType); ok {
			if dstList.ElementType.Equals(TypeAny) || srcList.ElementType.Equals(TypeAny) {
				return true
			}
			return CanAssign(dstList.ElementType, srcList.ElementType)
		}
	}
	// Dict covariance: dict[any,any] accepts dict[K,V]
	if dstDict, ok := dstType.(*DictType); ok {
		if srcDict, ok := srcType.(*DictType); ok {
			if (dstDict.KeyType.Equals(TypeAny) || CanAssign(dstDict.KeyType, srcDict.KeyType)) &&
				(dstDict.ValueType.Equals(TypeAny) || CanAssign(dstDict.ValueType, srcDict.ValueType)) {
				return true
			}
		}
	}
	// Vector covariance: vector[any] accepts vector[T]
	if dstVec, ok := dstType.(*VectorType); ok {
		if srcVec, ok := srcType.(*VectorType); ok {
			if dstVec.ElementType.Equals(TypeAny) || srcVec.ElementType.Equals(TypeAny) {
				return true
			}
			return CanAssign(dstVec.ElementType, srcVec.ElementType)
		}
	}
	if dstResult, ok := dstType.(*ResultType); ok {
		if srcResult, ok := srcType.(*ResultType); ok {
			okAssignable := dstResult.OkType.Equals(TypeAny) || CanAssign(dstResult.OkType, srcResult.OkType)
			errAssignable := dstResult.ErrType.Equals(TypeAny) || CanAssign(dstResult.ErrType, srcResult.ErrType)
			return okAssignable && errAssignable
		}
	}
	return dstType.Equals(srcType)
}

// MergeTypes combines multiple type possibilities into the most precise representation.
func MergeTypes(types ...Type) Type {
	resultTypes := make([]*ResultType, 0)
	hasNonResult := false
	for _, t := range types {
		if t == nil {
			continue
		}
		if union, ok := t.(*UnionType); ok {
			for _, opt := range union.Options {
				if rt, ok := opt.(*ResultType); ok {
					resultTypes = append(resultTypes, rt)
				} else {
					hasNonResult = true
				}
			}
			continue
		}
		if rt, ok := t.(*ResultType); ok {
			resultTypes = append(resultTypes, rt)
		} else {
			hasNonResult = true
		}
	}

	if len(resultTypes) > 0 && !hasNonResult {
		okTypes := make([]Type, 0, len(resultTypes))
		errTypes := make([]Type, 0, len(resultTypes))
		for _, rt := range resultTypes {
			okTypes = append(okTypes, rt.OkType)
			errTypes = append(errTypes, rt.ErrType)
		}
		return &ResultType{OkType: MergeTypes(okTypes...), ErrType: MergeTypes(errTypes...)}
	}

	unique := make(map[string]Type)
	for _, t := range types {
		if t == nil {
			continue
		}
		if union, ok := t.(*UnionType); ok {
			for _, opt := range union.Options {
				unique[typeKey(opt)] = opt
			}
			continue
		}
		unique[typeKey(t)] = t
	}

	switch len(unique) {
	case 0:
		return TypeVoid
	case 1:
		for _, v := range unique {
			return v
		}
	}

	keys := make([]string, 0, len(unique))
	for k := range unique {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	opts := make([]Type, 0, len(keys))
	for _, k := range keys {
		opts = append(opts, unique[k])
	}
	return &UnionType{Options: opts}
}

func typeKey(t Type) string {
	switch v := t.(type) {
	case *BasicType:
		return "basic:" + v.Name
	case *ListType:
		return "list[" + typeKey(v.ElementType) + "]"
	case *DictType:
		return "dict[" + typeKey(v.KeyType) + "," + typeKey(v.ValueType) + "]"
	case *VectorType:
		return "vector[" + typeKey(v.ElementType) + "]"
	case *FunctionType:
		params := make([]string, len(v.ParamTypes))
		for i, p := range v.ParamTypes {
			params[i] = typeKey(p)
		}
		return "fn[" + strings.Join(params, "|") + ":" + typeKey(v.ReturnType) + "]"
	case *ResultType:
		return "result[" + typeKey(v.OkType) + "," + typeKey(v.ErrType) + "]"
	case *UnionType:
		parts := make([]string, len(v.Options))
		for i, opt := range v.Options {
			parts[i] = typeKey(opt)
		}
		return "union[" + strings.Join(parts, "|") + "]"
	default:
		return fmt.Sprintf("%T:%s", t, t.String())
	}
}
