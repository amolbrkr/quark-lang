package types

import (
	"fmt"
	"sort"
	"strings"

	"quark/ast"
	"quark/ir"
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

// ErrorType represents a type error that has already been reported.
// It propagates silently through expressions, suppressing cascading errors.
// Distinct from TypeAny which means "genuinely unknown but no error."
type errorType struct{}

func (t *errorType) String() string { return "<error>" }
func (t *errorType) Equals(other Type) bool {
	_, ok := other.(*errorType)
	return ok
}

// Predefined basic types
var (
	TypeInt      = &BasicType{Name: "int"}
	TypeFloat    = &BasicType{Name: "float"}
	TypeString   = &BasicType{Name: "str"}
	TypeBool     = &BasicType{Name: "bool"}
	TypeNull     = &BasicType{Name: "null"}
	TypeAny      = &BasicType{Name: "any"}  // For unresolved types
	TypeVoid     = &BasicType{Name: "void"} // For statements with no value
	TypeResource = &BasicType{Name: "resource"}
	TypeFn       = &BasicType{Name: "fn"} // The `fn` annotation: a function value of any signature
	TypeError    = &errorType{}           // Poison type — error already reported upstream
)

// IsErrorType returns true if the type is the poison error type.
func IsErrorType(t Type) bool {
	_, ok := t.(*errorType)
	return ok
}

// ListType represents a list of elements.
//
// A list's element type is fixed by where it is created (rule B,
// architecture.md §12.9): the literal's elements, the builtin that made it,
// or for an empty `list []` the slot it is first stored into or its first
// write (Pending, see fix). Every value written into it must have that type,
// checked at compile time when the value's type is known and by the runtime
// element descriptor otherwise, so reads can trust it.
type ListType struct {
	ElementType Type
	pending
}

// pending is the state of the type of an empty list or dict literal whose
// element type isn't known yet. Until it is fixed it reads as any. peers are
// pending types of other literals stored in the same places, which must get
// the same element type.
type pending struct {
	Pending bool
	// FixedLine is the line of the write that fixed the element type (0 when
	// the type came from the literal itself), for messages.
	FixedLine int
	peers     []Type
}

// NewPendingList is the type of an empty `list []` literal.
func NewPendingList() *ListType {
	return &ListType{ElementType: TypeAny, pending: pending{Pending: true}}
}

// NewPendingDict is the type of an empty `dict {}` literal.
func NewPendingDict() *DictType {
	return &DictType{KeyType: TypeString, ValueType: TypeAny, pending: pending{Pending: true}}
}

func (t *ListType) String() string {
	return fmt.Sprintf("list[%s]", t.ElementType.String())
}

// VectorType represents a homogeneous numeric vector (MVP: float element type)
type VectorType struct {
	ElementType Type
}

func (t *VectorType) String() string {
	return fmt.Sprintf("vec[%s]", t.ElementType.String())
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

// DictType represents a dictionary. Keys are always strings at runtime;
// the value type is fixed like a list's element type (see ListType).
type DictType struct {
	KeyType   Type
	ValueType Type
	pending
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
	AnnotatedReturnType Type                // Declared return type (nil = not annotated)
	DefaultCount        int                 // Number of parameters with default values (always trailing)
	DefaultValues       []*DefaultValueInfo // Default value info per parameter (nil = no default)
	// ParamCalls holds, per parameter, the calls the body makes through it
	// (see analyzer_callshapes.go); nil when none are known.
	ParamCalls [][]*CallShape
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

// StructField describes a single field in a struct type.
type StructField struct {
	Name        string
	Type        Type
	HasDefault  bool
	DefaultNode interface{} // *ast.TreeNode — codegen uses this to emit default values
}

// StructType represents a named struct type with ordered fields.
// Identity is nominal: two structs with identical fields but different names are distinct.
type StructType struct {
	Name   string
	Fields []StructField
}

func (t *StructType) String() string {
	return t.Name
}

func (t *StructType) Equals(other Type) bool {
	if o, ok := other.(*StructType); ok {
		return t.Name == o.Name
	}
	return false
}

// FieldByName returns the field and its index, or nil/-1 if not found.
func (t *StructType) FieldByName(name string) (*StructField, int) {
	for i := range t.Fields {
		if t.Fields[i].Name == name {
			return &t.Fields[i], i
		}
	}
	return nil, -1
}

// SymKind says what declared a symbol.
type SymKind int

const (
	SymBuiltin SymKind = iota // a prelude builtin or extern fn (no C++ variable)
	SymLocal                  // a variable, defined by its first assignment or a typed declaration
	SymParam                  // a function or lambda parameter
	SymLoop                   // a for loop variable
	SymBinding                // an ok/err binding of a when arm
	SymFn                     // a fn declaration (its binding, when it needs a closure)
)

// Symbol is one declaration: a variable, parameter, loop variable, when
// binding, fn declaration or builtin. Every identifier the analyzer resolves
// maps to one (Analysis.Refs), so two variables with one name are never
// confused, and how a symbol is stored is decided once, by the analyzer.
type Symbol struct {
	Name    string
	Type    Type // current type (flow typing may narrow it after the declaration)
	Mutable bool
	Defined bool // Whether it has been assigned a value

	// Set for every kind but SymBuiltin:
	ID   int
	Kind SymKind
	// StoreType is the type every value the symbol holds is known to have:
	// its annotation (checked where the value comes in), or the type of its
	// first value. nil means any value.
	StoreType Type
	// Decl is the identifier node where codegen declares the C++ variable
	// (the first assignment, the declaration, the parameter, ...).
	Decl *ast.TreeNode

	// Set once analysis is done (finalizeSymbols):
	Storage ir.Storage
	CName   string // C++ variable name
	// Hoisted marks a captured variable whose cell is created unassigned at
	// the start of its scope (a hoisted fn captures it); closures read it
	// with q_cell_get.
	Hoisted bool
}

// IsVariable reports whether the symbol has a C++ variable of its own.
func (s *Symbol) IsVariable() bool { return s != nil && s.Kind != SymBuiltin }

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

// CanAssign reports whether a value of type src can be stored where a value
// of type dst is expected (a variable, parameter, element, field or return
// value). An int is widened where a float is stored. Below the top level
// (the elements of a list, the payload of a result) values are shared, not
// converted, so they need a subtype (IsSubtype).
func CanAssign(dstType, srcType Type) bool {
	if IsErrorType(dstType) || IsErrorType(srcType) {
		return true
	}
	if dstType.Equals(TypeFloat) && srcType.Equals(TypeInt) {
		return true
	}
	if u, ok := dstType.(*UnionType); ok {
		if _, srcIsUnion := srcType.(*UnionType); !srcIsUnion {
			for _, opt := range u.Options {
				if CanAssign(opt, srcType) {
					return true
				}
			}
		}
	}
	return IsSubtype(dstType, srcType)
}

// IsSubtype reports whether every value of type src is also a value of type
// dst, with no conversion. A list whose static type says list[T] holds only
// T values (rule B), so a list[int] is a list[any] but not a list[float]
// (its ints would be read as floats), and a list[any] is not a list[int].
func IsSubtype(dstType, srcType Type) bool {
	if IsErrorType(dstType) || IsErrorType(srcType) {
		return true
	}
	if dstType.Equals(TypeAny) {
		return true
	}
	if srcType.Equals(TypeAny) {
		return false
	}
	if u, ok := srcType.(*UnionType); ok {
		if len(u.Options) == 0 {
			return false
		}
		for _, opt := range u.Options {
			if !IsSubtype(dstType, opt) {
				return false
			}
		}
		return true
	}
	if u, ok := dstType.(*UnionType); ok {
		for _, opt := range u.Options {
			if IsSubtype(opt, srcType) {
				return true
			}
		}
		return false
	}
	// null is a value of the struct and resource types only: `list`, `dict`,
	// `vec` and `fn` mean a list, dict, vec or function (decided 2026-11-13),
	// and a result is always ok(...) or err(...), so `r == null` can't stand
	// in for is_err(r).
	if srcType.Equals(TypeNull) {
		_, isStruct := dstType.(*StructType)
		return isStruct || dstType.Equals(TypeResource) || dstType.Equals(TypeNull)
	}
	// `fn` accepts every function value. A value known only as `fn` has no
	// known signature, so it isn't a value of a specific one: calls through
	// a signature trust its return type.
	if dstType.Equals(TypeFn) {
		return IsCallableType(srcType)
	}
	switch dst := dstType.(type) {
	case *ListType:
		src, ok := srcType.(*ListType)
		return ok && IsSubtype(dst.ElementType, src.ElementType)
	case *DictType:
		src, ok := srcType.(*DictType)
		return ok && IsSubtype(dst.ValueType, src.ValueType)
	case *VectorType:
		src, ok := srcType.(*VectorType)
		if !ok {
			return false
		}
		return dst.ElementType.Equals(TypeAny) || dst.ElementType.Equals(src.ElementType)
	case *ResultType:
		src, ok := srcType.(*ResultType)
		return ok && IsSubtype(dst.OkType, src.OkType) && IsSubtype(dst.ErrType, src.ErrType)
	case *FunctionType:
		src, ok := srcType.(*FunctionType)
		if !ok || len(dst.ParamTypes) != len(src.ParamTypes) || dst.MinArity() != src.MinArity() {
			return false
		}
		for i, p := range dst.ParamTypes {
			sp := src.ParamTypes[i]
			if !p.Equals(sp) && !p.Equals(TypeAny) && !sp.Equals(TypeAny) {
				return false
			}
		}
		return IsSubtype(dst.ReturnType, src.ReturnType)
	case *StructType:
		return dst.Equals(srcType)
	}
	return dstType.Equals(srcType)
}

// SettleContainers fixes a pending (empty literal) list or dict type from
// the container type it is stored into or compared with, in either
// direction, including the containers nested in both. Pending types that
// meet each other become peers and are fixed together.
func SettleContainers(dst, src Type) {
	switch d := dst.(type) {
	case *ListType:
		if s, ok := src.(*ListType); ok && d != s {
			settleElems(d, s, &d.pending, &s.pending, d.ElementType, s.ElementType)
		}
	case *DictType:
		if s, ok := src.(*DictType); ok && d != s {
			settleElems(d, s, &d.pending, &s.pending, d.ValueType, s.ValueType)
		}
	case *ResultType:
		if s, ok := src.(*ResultType); ok {
			SettleContainers(d.OkType, s.OkType)
			SettleContainers(d.ErrType, s.ErrType)
		}
	}
}

func settleElems(d, s Type, dp, sp *pending, de, se Type) {
	switch {
	case dp.Pending && sp.Pending:
		dp.peers = append(dp.peers, s)
		sp.peers = append(sp.peers, d)
	case sp.Pending:
		FixElementType(s, de, 0)
	case dp.Pending:
		FixElementType(d, se, 0)
	default:
		SettleContainers(de, se)
	}
}

// FixElementType gives the pending list or dict type t (and its peers) the
// element type elem, at the write on line. A null or void write doesn't
// decide anything.
func FixElementType(t Type, elem Type, line int) {
	if elem == nil || IsErrorType(elem) || elem.Equals(TypeNull) || elem.Equals(TypeVoid) {
		return
	}
	elem = ElementOf(elem)
	var p *pending
	switch c := t.(type) {
	case *ListType:
		if !c.Pending {
			return
		}
		c.ElementType = elem
		p = &c.pending
	case *DictType:
		if !c.Pending {
			return
		}
		c.ValueType = elem
		p = &c.pending
	default:
		return
	}
	p.Pending = false
	p.FixedLine = line
	peers := p.peers
	p.peers = nil
	for _, peer := range peers {
		FixElementType(peer, elem, line)
	}
}

// ElementOf is the type a container records for elements of type t. A
// function's return type is dropped: a function stored in a list can be
// replaced by untyped code with one returning something else, and the
// runtime check on that write sees only that it is a function.
func ElementOf(t Type) Type {
	switch v := t.(type) {
	case *FunctionType:
		if v.ReturnType.Equals(TypeAny) && v.AnnotatedReturnType == nil {
			return v
		}
		erased := *v
		erased.ReturnType = TypeAny
		erased.AnnotatedReturnType = nil
		return &erased
	case *UnionType:
		opts := make([]Type, len(v.Options))
		for i, opt := range v.Options {
			opts[i] = ElementOf(opt)
		}
		return MergeTypes(opts...)
	}
	return t
}

// IsCallableType reports whether t is a function value type: a signature, the
// `fn` annotation, or a union of those (the element type of a list of
// functions with different signatures).
func IsCallableType(t Type) bool {
	switch v := t.(type) {
	case *FunctionType:
		return true
	case *UnionType:
		if len(v.Options) == 0 {
			return false
		}
		for _, opt := range v.Options {
			if !IsCallableType(opt) {
				return false
			}
		}
		return true
	}
	return t != nil && t.Equals(TypeFn)
}

// MergeTypes combines multiple type possibilities into the most precise representation.
// ErrorType is filtered out — it should not appear in merged results.
func MergeTypes(types ...Type) Type {
	settleMerged(types)
	resultTypes := make([]*ResultType, 0)
	hasNonResult := false
	for _, t := range types {
		if t == nil || IsErrorType(t) {
			continue
		}
		if union, ok := t.(*UnionType); ok {
			for _, opt := range union.Options {
				if IsErrorType(opt) {
					continue
				}
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
		if t == nil || IsErrorType(t) {
			continue
		}
		if union, ok := t.(*UnionType); ok {
			for _, opt := range union.Options {
				if !IsErrorType(opt) {
					unique[typeKey(opt)] = opt
				}
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
		return "vec[" + typeKey(v.ElementType) + "]"
	case *FunctionType:
		params := make([]string, len(v.ParamTypes))
		for i, p := range v.ParamTypes {
			params[i] = typeKey(p)
		}
		return "fn[" + strings.Join(params, "|") + ":" + typeKey(v.ReturnType) + "]"
	case *ResultType:
		return "result[" + typeKey(v.OkType) + "," + typeKey(v.ErrType) + "]"
	case *StructType:
		return "struct:" + v.Name
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

// settleMerged fixes pending (empty literal) container types among types
// from the one known container type of their kind, if there is exactly
// one: `list [list [], list [1]]` is a list[list[int]].
func settleMerged(types []Type) {
	var known, pend []Type
	for _, t := range types {
		switch c := t.(type) {
		case *ListType:
			if c.Pending {
				pend = append(pend, c)
			} else {
				known = append(known, c)
			}
		case *DictType:
			if c.Pending {
				pend = append(pend, c)
			} else {
				known = append(known, c)
			}
		}
	}
	if len(pend) == 0 || len(known) == 0 {
		return
	}
	for _, p := range pend {
		var match Type
		for _, k := range known {
			if fmt.Sprintf("%T", k) != fmt.Sprintf("%T", p) {
				continue
			}
			if match != nil && typeKey(match) != typeKey(k) {
				match = nil
				break
			}
			match = k
		}
		if match != nil {
			SettleContainers(match, p)
		}
	}
}
