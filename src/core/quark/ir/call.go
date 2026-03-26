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
	DispatchExtern // Native-typed extern fn call
	DispatchNative // Native-typed user-defined function (fully annotated)
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
	IsMethod        bool            // True when call was x.method(args) — receiver is first runtime arg.
	ReceiverNode    *ast.TreeNode   // The receiver expression node (x in x.method(args)).
	ReceiverTypeKey string          // builtins.TypeKey of the receiver (set when IsMethod == true).

	// Populated for DispatchExtern and DispatchNative calls:
	// C++ type per explicit parameter: "int64_t", "double", "bool", "const char*",
	// "QVector*", "QList*", "QDict*", "QClosure*", "QValue"
	NativeParamTypes   []string
	NativeReturnType   string // C++ return type (same set as above)
	NativeReceiverType string // C++ type of the method receiver; empty for free functions
}
