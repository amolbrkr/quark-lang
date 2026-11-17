package types

import (
	"quark/ast"
	"quark/ir"
	"quark/token"
)

// Missing values in verb expressions (specs/tables_v0_3_spec.md §6, D46,
// D47). A part of a verb expression that reads a missing value is missing
// without being computed; these operations look at the missing value
// itself, so their result is present whether or not x is.

// RowMissing is `x == null` or `x != null` (Fill nil), or `x.fillna(v)`, in
// a verb expression. Lowering computes whether x is present and doesn't make
// the expression around it missing.
type RowMissing struct {
	X    *ast.TreeNode
	Fill *ast.TreeNode // fillna's value; nil for a null test
	Not  bool          // x != null
}

// rowNullTest types `x == null` / `x != null` in a verb expression (D46): a
// bool, true or false whether x is missing. ok is false when node isn't one.
func (a *Analyzer) rowNullTest(node *ast.TreeNode, leftType, rightType Type) (Type, bool) {
	if !a.inVerbExpr() {
		return nil, false
	}
	lNull, rNull := leftType.Equals(TypeNull), rightType.Equals(TypeNull)
	if !lNull && !rNull {
		return nil, false
	}
	if lNull && rNull {
		a.errorAt(node, "in a query, '%s' compares null with null; test a value of the row (%s.col == null)", node.Token.Literal, a.rowCtx.sym.Name)
		return TypeError, true
	}
	x := node.Children[0]
	if lNull {
		x = node.Children[1]
	}
	a.rowMissing[node] = &RowMissing{X: x, Not: node.Token.Type == token.NE}
	return TypeBool, true
}

// rowOrderedNull rejects `x < null` and the other ordered comparisons with
// null in a verb expression (D46).
func (a *Analyzer) rowOrderedNull(node *ast.TreeNode, leftType, rightType Type) bool {
	if !a.inVerbExpr() || (!leftType.Equals(TypeNull) && !rightType.Equals(TypeNull)) {
		return false
	}
	a.errorAt(node, "in a query, '%s' with null compares with a missing value, which is never true or false; test whether a value is missing with == null or != null", node.Token.Literal)
	return true
}

// isRowFillna reports whether a method call is `x.fillna(v)` on one row's
// value in a verb expression (D47), not the vec method: x has a column type,
// or an unknown type and reads the row.
func (a *Analyzer) isRowFillna(methodName string, receiverNode *ast.TreeNode, receiverType Type) bool {
	if methodName != "fillna" || !a.inVerbExpr() {
		return false
	}
	if IsColumnType(receiverType) {
		return true
	}
	if _, isVec := receiverType.(*VectorType); isVec {
		return false
	}
	return isUnknownType(receiverType) && a.exprReadsRow(receiverNode)
}

// rowFillna types `x.fillna(v)` in a verb expression (D47): x where it is
// present, else v, which has x's type (an int fills a float).
func (a *Analyzer) rowFillna(call, receiverNode *ast.TreeNode, receiverType Type, argTypes []Type, argNodes []*ast.TreeNode) methodCallResult {
	a.callPlans[call] = &ir.CallPlan{CalleeName: "fillna", MinArity: 2, MaxArity: 2, Dispatch: ir.DispatchRowFillna}
	res := methodCallResult{returnType: TypeError, receiverType: receiverType, resolved: true}
	if len(argNodes) != 1 {
		a.errorAt(call, "fillna takes one value, the value for rows where '%s' is missing, got %d", exprText(receiverNode), len(argNodes))
		return res
	}
	vt := argTypes[0]
	if IsErrorType(vt) {
		return res
	}
	t := receiverType
	switch {
	case isUnknownType(t):
		t = TypeAny
	case isUnknownType(vt):
		a.errorAt(argNodes[0], "the type of fillna()'s value isn't known; it must be %s (annotate the return type of a function it calls)", t.String())
		return res
	case !CanAssign(t, vt):
		a.errorAt(argNodes[0], "fillna() can't fill %s with %s: the fill value must have the value's type (an int may fill a float)", t.String(), vt.String())
		return res
	}
	a.rowMissing[call] = &RowMissing{X: receiverNode, Fill: argNodes[0]}
	res.returnType = t
	return res
}
