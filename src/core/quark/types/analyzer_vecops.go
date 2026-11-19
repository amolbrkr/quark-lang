package types

import (
	"quark/ast"
	"quark/ir"
	"quark/token"
)

// vecOperators are the operators that compute a vec element by element when
// an operand is a vec.
var vecOperators = map[token.TokenType]bool{
	token.PLUS: true, token.MINUS: true, token.MULTIPLY: true, token.DIVIDE: true, token.MODULO: true, token.DOUBLESTAR: true,
	token.LT: true, token.LTE: true, token.GT: true, token.GTE: true, token.DEQ: true, token.NE: true,
}

// vecOps finds the program's vec operations (Analysis.VecOps): an arithmetic
// or comparison operator, or a unary minus, with a vec operand; an index of
// a vec; and a call of a row-safe builtin or method with a vec receiver,
// argument or result. With --checked, lowering runs each of them again on
// one thread when it ran on several, and compares the two results
// (specs/tables_v0_3_spec.md §8.7). Operators and row-safe builtins have no
// effects, so running one again changes nothing. The value is the
// operation's name for the message.
func (a *Analyzer) vecOps(root *ast.TreeNode) map[*ast.TreeNode]string {
	ops := map[*ast.TreeNode]string{}
	isVec := func(n *ast.TreeNode) bool {
		if n == nil {
			return false
		}
		_, ok := a.nodeTypes[n].(*VectorType)
		return ok
	}
	var walk func(n *ast.TreeNode)
	walk = func(n *ast.TreeNode) {
		if n == nil {
			return
		}
		switch n.NodeType {
		case ast.BinaryNode:
			if n.Token != nil && vecOperators[n.Token.Type] && len(n.Children) == 2 && (isVec(n.Children[0]) || isVec(n.Children[1])) {
				ops[n] = "'" + n.Token.Literal + "'"
			}
		case ast.UnaryNode:
			if n.Token != nil && n.Token.Type == token.MINUS && len(n.Children) == 1 && isVec(n.Children[0]) {
				ops[n] = "'-'"
			}
		case ast.IndexNode:
			if len(n.Children) >= 2 && isVec(n.Children[0]) {
				ops[n] = "an index"
			}
		case ast.FunctionCallNode:
			plan := a.callPlans[n]
			if plan != nil && plan.Dispatch == ir.DispatchBuiltin && a.builtinRowSafe(plan) {
				touches := isVec(n) || isVec(plan.ReceiverNode)
				if len(n.Children) > 1 && n.Children[1] != nil {
					for _, arg := range n.Children[1].Children {
						touches = touches || isVec(arg)
					}
				}
				if touches {
					ops[n] = "'" + plan.CalleeName + "'"
				}
			}
		}
		for _, c := range n.Children {
			walk(c)
		}
	}
	walk(root)
	return ops
}
