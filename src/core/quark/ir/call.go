package ir

import "quark/ast"

// CallKind captures how a callable is resolved after analysis.
type CallKind int

const (
	CallUnknown CallKind = iota
	CallBuiltin
	CallFunctionValue
)

// DispatchMode tells codegen how to lower an analyzed call without re-deriving semantics.
type DispatchMode int

const (
	DispatchUnknown DispatchMode = iota
	DispatchBuiltin
	DispatchDirect
	DispatchClosure
)

// CallPlan is a call-focused IR node produced by the analyzer and consumed by codegen.
// It freezes call semantics so codegen doesn't have to re-derive them.
type CallPlan struct {
	Kind            CallKind
	CalleeName      string
	MinArity        int
	MaxArity        int
	Dispatch        DispatchMode
	RuntimeSymbol   string
	ArgTypesChecked bool
	DefaultNodes    []*ast.TreeNode // Trailing default args to append in call order.
}
