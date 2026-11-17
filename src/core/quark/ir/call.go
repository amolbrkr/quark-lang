package ir

import "quark/ast"

// DispatchMode tells lowering how to lower an analyzed call without re-deriving semantics.
type DispatchMode int

const (
	DispatchUnknown DispatchMode = iota
	DispatchBuiltin
	DispatchDirect
	DispatchClosure
	DispatchExtern // Native-typed extern fn call
	DispatchNative // Native-typed user-defined function (fully annotated)
	// DispatchDynamicMethod is a builtin method call on a receiver whose type is
	// unknown at compile time (e.g. the result of unwrap). Lowering tests the
	// receiver's runtime type tag over MethodCandidates.
	DispatchDynamicMethod
	// DispatchAggregate is a query aggregate (sum(e.x) inside a verb). It is
	// syntax, not a call: lowering reads the value computed for the row's group.
	DispatchAggregate
)

// MethodCandidate is one receiver type a DispatchDynamicMethod call may resolve to.
type MethodCandidate struct {
	ValueTag        string // QValue::ValueType enumerator, e.g. "VAL_STRING"
	ReceiverTypeKey string // builtins.TypeKey of the receiver
	RuntimeSymbol   string // runtime function for this receiver type
	RuntimeHeader   string // header defining it, when quark.hpp doesn't
}

// CallPlan is a call-focused IR node produced by the analyzer and consumed by lowering.
// It freezes call semantics so lowering doesn't have to re-derive them.
type CallPlan struct {
	CalleeName    string
	MinArity      int
	MaxArity      int
	Dispatch      DispatchMode
	RuntimeSymbol string
	// RuntimeHeader is the runtime header defining RuntimeSymbol when
	// quark.hpp doesn't (builtins.Spec.Header).
	RuntimeHeader   string
	DefaultNodes    []*ast.TreeNode // Trailing default args to append in call order.
	IsMethod        bool            // True when call was x.method(args) — receiver is first runtime arg.
	ReceiverNode    *ast.TreeNode   // The receiver expression node (x in x.method(args)).
	ReceiverTypeKey string          // builtins.TypeKey of the receiver (set when IsMethod == true).
	// MethodCandidates lists the receiver types a DispatchDynamicMethod call
	// supports, in switch order.
	MethodCandidates []MethodCandidate

	// RowSafe is set on an extern fn declared `extern rowsafe fn` (D43).
	// Draws is set on a random draw: row-safe, but each call draws its own
	// number, so a verb does not compute it once (D44).
	// Once is set on a verb call whose result is the same for every row of
	// the step. Lowering evaluates it the first time a row needs it.
	RowSafe bool
	Draws   bool
	Once    bool

	// Populated for DispatchExtern and DispatchNative calls:
	// C++ type per explicit parameter: "int64_t", "double", "bool", "const char*",
	// "QVector*", "QList*", "QDict*", "QClosure*", "QValue"
	NativeParamTypes   []string
	NativeReturnType   string // C++ return type (same set as above)
	NativeReceiverType string // C++ type of the method receiver; empty for free functions
}
