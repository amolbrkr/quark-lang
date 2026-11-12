package types

import (
	"fmt"

	"quark/ast"
)

// Call shapes: how a function uses its parameters as functions.
//
// While a function body is analyzed, every call made through one of its
// parameters (`k(1)`, `x | k()`) is recorded as a CallShape. So is passing the
// parameter on to another function's parameter that has shapes of its own
// (`fn wrap(k) -> apply(k)` inherits apply's calls of its first parameter).
// The shapes end up in FunctionType.ParamCalls. A call that passes a function
// whose signature is known is then checked against them at compile time:
// `apply(h)` where apply calls its parameter with one argument and h takes
// two is rejected at the call to apply, before the program runs. Function
// values the compiler can't see through are checked when called (QFnInfo).

// CallShape is one call a function body makes through a parameter.
type CallShape struct {
	ArgTypes []Type // argument types at that call; unknown ones aren't checked
	// ArgParams[j] is the index of the function's own parameter passed
	// unchanged as argument j (`fn apply(k, v) -> k(v)`), or -1. Its type is
	// only known at each call of the function, where it replaces ArgTypes[j].
	ArgParams []int
	Node      *ast.TreeNode // the callee expression at that call, for diagnostics
}

// paramUsage collects the call shapes of one parameter of a function whose
// body is being analyzed.
type paramUsage struct {
	owner      int // identifies the function the parameter belongs to
	index      int // the parameter's position
	calls      []*CallShape
	reassigned bool // the body assigns the parameter, so its calls may be of another value
}

// newParamOwner returns an id for a function whose parameters are tracked.
func (a *Analyzer) newParamOwner() int {
	a.paramOwnerSeq++
	return a.paramOwnerSeq
}

// trackParam starts collecting call shapes for parameter index of owner.
func (a *Analyzer) trackParam(sym *Symbol, owner, index int) {
	if sym != nil {
		a.paramUsages[sym] = &paramUsage{owner: owner, index: index}
	}
}

// paramCallsOf returns the call shapes collected for a function's parameter
// symbols, in parameter order, and stops tracking them.
func (a *Analyzer) paramCallsOf(syms []*Symbol) [][]*CallShape {
	var out [][]*CallShape
	for i, sym := range syms {
		u := a.paramUsages[sym]
		delete(a.paramUsages, sym)
		if u == nil || u.reassigned || len(u.calls) == 0 {
			continue
		}
		if out == nil {
			out = make([][]*CallShape, len(syms))
		}
		out[i] = u.calls
	}
	return out
}

// usageOf is the call-shape tracker of the parameter an identifier refers to,
// or nil.
func (a *Analyzer) usageOf(node *ast.TreeNode) *paramUsage {
	if node == nil || node.NodeType != ast.IdentifierNode {
		return nil
	}
	sym := a.currentScope.Lookup(node.TokenLiteral())
	if sym == nil {
		return nil
	}
	return a.paramUsages[sym]
}

// recordParamCall records a call through calleeNode when it names a tracked
// parameter. argNodes are the call's arguments (with a piped input first).
func (a *Analyzer) recordParamCall(calleeNode *ast.TreeNode, argTypes []Type, argNodes []*ast.TreeNode) {
	u := a.usageOf(calleeNode)
	if u == nil {
		return
	}
	shape := &CallShape{ArgTypes: append([]Type(nil), argTypes...), ArgParams: make([]int, len(argTypes)), Node: calleeNode}
	for j := range shape.ArgParams {
		shape.ArgParams[j] = -1
		if j < len(argNodes) {
			if v := a.usageOf(argNodes[j]); v != nil && v.owner == u.owner {
				shape.ArgParams[j] = v.index
			}
		}
	}
	a.addShapes(u, []*CallShape{shape})
}

// markParamReassigned stops trusting the shapes of an assigned parameter.
func (a *Analyzer) markParamReassigned(sym *Symbol) {
	if u := a.paramUsages[sym]; u != nil {
		u.reassigned = true
	}
}

// maxShapesPerParam bounds how many shapes a parameter keeps when they are
// passed along chains of functions.
const maxShapesPerParam = 32

func (a *Analyzer) addShapes(u *paramUsage, shapes []*CallShape) {
	for _, s := range shapes {
		if len(u.calls) >= maxShapesPerParam {
			return
		}
		u.calls = append(u.calls, s)
	}
}

// checkCallbackArgs checks the arguments of a call to a function with known
// signature ft against the calls ft's body makes through those parameters.
// An argument that is itself a parameter of the function being analyzed
// inherits the shapes instead.
func (a *Analyzer) checkCallbackArgs(calleeName string, ft *FunctionType, argTypes []Type, argNodes []*ast.TreeNode) {
	if ft == nil {
		return
	}
	for i, calls := range ft.ParamCalls {
		if len(calls) == 0 || i >= len(argTypes) || i >= len(argNodes) {
			continue
		}
		if u := a.usageOf(argNodes[i]); u != nil {
			a.addShapes(u, a.rebaseShapes(calls, u.owner, argTypes, argNodes))
			continue
		}
		a.checkShapesAgainst(calleeName, i, calls, argTypes, argNodes[i], argTypes[i])
	}
}

// rebaseShapes adapts a callee's call shapes to the function being analyzed,
// which passes one of its parameters on to the callee: an argument that was
// the callee's own parameter p becomes the type of this call's argument p, or,
// when that argument is in turn a parameter of owner, a reference to it.
func (a *Analyzer) rebaseShapes(calls []*CallShape, owner int, argTypes []Type, argNodes []*ast.TreeNode) []*CallShape {
	out := make([]*CallShape, 0, len(calls))
	for _, call := range calls {
		shape := &CallShape{ArgTypes: append([]Type(nil), call.ArgTypes...), ArgParams: make([]int, len(call.ArgTypes)), Node: call.Node}
		for j := range shape.ArgParams {
			shape.ArgParams[j] = -1
			p := paramRef(call, j)
			if p < 0 {
				continue
			}
			if p < len(argNodes) {
				if v := a.usageOf(argNodes[p]); v != nil && v.owner == owner {
					shape.ArgParams[j] = v.index
					continue
				}
			}
			if p < len(argTypes) {
				shape.ArgTypes[j] = argTypes[p]
			}
		}
		out = append(out, shape)
	}
	return out
}

// paramRef is the parameter index argument j of call refers to, or -1.
func paramRef(call *CallShape, j int) int {
	if j < len(call.ArgParams) {
		return call.ArgParams[j]
	}
	return -1
}

// checkShapesAgainst reports the first call shape the function passed as
// argument index of calleeName can't satisfy.
// siteArgTypes are the argument types of the call being checked, which
// resolve the shapes' references to calleeName's own parameters.
func (a *Analyzer) checkShapesAgainst(calleeName string, index int, calls []*CallShape, siteArgTypes []Type, argNode *ast.TreeNode, argType Type) {
	what := a.describeFunctionArg(argNode)
	switch t := argType.(type) {
	case *FunctionType:
		for _, call := range calls {
			n := len(call.ArgTypes)
			if n < t.MinArity() || n > len(t.ParamTypes) {
				a.errorAt(argNode, "argument %d of '%s' is called with %s%s, but %s takes %s",
					index+1, calleeName, pluralArgs(n), shapeLocation(call), what, arityText(t.MinArity(), len(t.ParamTypes)))
				return
			}
			for j, at := range call.ArgTypes {
				if p := paramRef(call, j); p >= 0 && p < len(siteArgTypes) {
					at = siteArgTypes[p]
				}
				pt := t.ParamTypes[j]
				if pt == nil || pt.Equals(TypeAny) || isUnknownType(pt) || isUnknownType(at) || IsErrorType(at) {
					continue
				}
				if !CanAssign(pt, at) {
					a.errorAt(argNode, "argument %d of '%s' is called with %s as argument %d%s, but %s expects %s",
						index+1, calleeName, at.String(), j+1, shapeLocation(call), what, pt.String())
					return
				}
			}
		}
	case *UnionType:
		if !maybeCallable(t) {
			a.errorAt(argNode, "argument %d of '%s' is called as a function%s, but gets %s", index+1, calleeName, shapeLocation(calls[0]), t.String())
			return
		}
		if !IsCallableType(t) {
			return // may be null or unknown: checked when called
		}
		// One of several functions: reject a call none of them accepts.
		for _, call := range calls {
			if !anyOptionAccepts(t, len(call.ArgTypes)) {
				a.errorAt(argNode, "argument %d of '%s' is called with %s%s, which none of the functions it can be accepts",
					index+1, calleeName, pluralArgs(len(call.ArgTypes)), shapeLocation(call))
				return
			}
		}
	default:
		if !maybeCallable(argType) {
			a.errorAt(argNode, "argument %d of '%s' is called as a function%s, but gets %s", index+1, calleeName, shapeLocation(calls[0]), argType.String())
		}
	}
}

// maybeCallable reports whether a value of type t may be called: a function,
// a type the compiler doesn't know, or null (an optional callback the body
// may check before calling).
func maybeCallable(t Type) bool {
	if t == nil || isUnknownType(t) || IsErrorType(t) || IsCallableType(t) || t.Equals(TypeNull) {
		return true
	}
	if u, ok := t.(*UnionType); ok {
		for _, opt := range u.Options {
			if maybeCallable(opt) {
				return true
			}
		}
	}
	return false
}

// describeFunctionArg names a function passed as an argument in diagnostics.
func (a *Analyzer) describeFunctionArg(node *ast.TreeNode) string {
	if node == nil {
		return "the function passed"
	}
	switch node.NodeType {
	case ast.IdentifierNode:
		return fmt.Sprintf("'%s'", node.TokenLiteral())
	case ast.LambdaNode:
		return "the lambda passed"
	}
	return "the function passed"
}

// anyOptionAccepts reports whether some function type in a union of function
// types can be called with n arguments (true when an option isn't a known
// signature).
func anyOptionAccepts(u *UnionType, n int) bool {
	for _, opt := range u.Options {
		ft, ok := opt.(*FunctionType)
		if !ok {
			return true
		}
		if n >= ft.MinArity() && n <= len(ft.ParamTypes) {
			return true
		}
	}
	return false
}

func shapeLocation(call *CallShape) string {
	if call.Node == nil || call.Node.Token == nil {
		return ""
	}
	return fmt.Sprintf(" (line %d)", call.Node.Token.Line)
}

func pluralArgs(n int) string {
	if n == 1 {
		return "1 argument"
	}
	return fmt.Sprintf("%d arguments", n)
}

func arityText(minArity, maxArity int) string {
	if minArity == maxArity {
		return pluralArgs(maxArity)
	}
	return fmt.Sprintf("%d-%d arguments", minArity, maxArity)
}
