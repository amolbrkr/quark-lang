package lower

import (
	"fmt"

	"quark/ast"
	"quark/ir"
	"quark/types"
)

// call lowers a call by its plan. Operands are evaluated left to right: the
// callee (for a function value), the receiver (for a method), the arguments,
// then the filled-in defaults.
func (l *lowerer) call(node *ast.TreeNode) Expr {
	if len(node.Children) < 2 {
		return nullValue()
	}
	funcNode, argsNode := node.Children[0], node.Children[1]
	if tr := l.an.TableReads[node]; tr != nil {
		return l.tableRead(argsNode, tr)
	}
	plan := l.an.CallPlans[node]
	if plan == nil {
		l.ice("INV-CALLPLAN-MISSING", node, "missing call plan")
	}
	if funcNode.NodeType == ast.MemberNode && plan.Dispatch == ir.DispatchClosure {
		l.ice("INV-DOT-CALL", node, "dot-call syntax is not supported")
	}

	var nodes []*ast.TreeNode
	calleeIdx, recvIdx := -1, -1
	if plan.Dispatch == ir.DispatchClosure {
		calleeIdx = len(nodes)
		nodes = append(nodes, funcNode)
	}
	if plan.IsMethod && plan.ReceiverNode != nil {
		recvIdx = len(nodes)
		nodes = append(nodes, plan.ReceiverNode)
	}
	argStart := len(nodes)
	if argsNode != nil {
		nodes = append(nodes, argsNode.Children...)
	}
	for _, d := range plan.DefaultNodes {
		if d != nil {
			nodes = append(nodes, d)
		}
	}
	vals := l.operands(nodes)
	args := vals[argStart:]
	l.include(plan.RuntimeHeader)
	for _, c := range plan.MethodCandidates {
		l.include(c.RuntimeHeader)
	}

	switch plan.Dispatch {
	case ir.DispatchBuiltin:
		if plan.RuntimeSymbol == "" {
			l.ice("INV-CALLPLAN-RUNTIME", node, "builtin call '%s' missing runtime symbol", plan.CalleeName)
		}
		all := args
		if recvIdx >= 0 {
			all = append([]Expr{vals[recvIdx]}, args...)
		}
		return &Call{Fn: plan.RuntimeSymbol, Args: boxedAll(all), R: RepValue}

	case ir.DispatchDynamicMethod:
		if recvIdx < 0 {
			l.ice("INV-CALLPLAN-RUNTIME", node, "dynamic method call '%s' has no receiver", plan.CalleeName)
		}
		// Every argument is evaluated before the receiver's type is tested,
		// as for any call.
		dargs := boxedAll(args)
		for i := range dargs {
			dargs[i] = l.stable(dargs[i])
		}
		return l.dynamicMethod(funcNode, plan, l.stable(boxed(vals[recvIdx])), dargs)

	case ir.DispatchDirect:
		if plan.RuntimeSymbol == "" {
			l.ice("INV-CALLPLAN-RUNTIME", node, "direct call '%s' missing runtime symbol", plan.CalleeName)
		}
		all := args
		if recvIdx >= 0 {
			all = append([]Expr{vals[recvIdx]}, args...)
		}
		return &Call{Fn: plan.RuntimeSymbol, Args: append([]Expr{nullptrExpr}, boxedAll(all)...), R: RepValue}

	case ir.DispatchNative:
		if plan.RuntimeSymbol == "" {
			l.ice("INV-CALLPLAN-RUNTIME", node, "native call '%s' missing runtime symbol", plan.CalleeName)
		}
		out := []Expr{nullptrExpr}
		for i, a := range args {
			ct := ""
			if i < len(plan.NativeParamTypes) {
				ct = plan.NativeParamTypes[i]
			}
			out = append(out, toNative(a, RepOfCType(ct)))
		}
		return &Call{Fn: plan.RuntimeSymbol, Args: out, R: RepOfCType(plan.NativeReturnType)}

	case ir.DispatchExtern:
		if plan.RuntimeSymbol == "" {
			l.ice("INV-CALLPLAN-RUNTIME", node, "extern call '%s' missing runtime symbol", plan.CalleeName)
		}
		var out []Expr
		if recvIdx >= 0 {
			r := boxed(vals[recvIdx])
			if plan.NativeReceiverType != "" {
				r = toNative(vals[recvIdx], RepOfCType(plan.NativeReceiverType))
			}
			out = append(out, r)
		}
		for i, a := range args {
			if i < len(plan.NativeParamTypes) {
				out = append(out, toNative(a, RepOfCType(plan.NativeParamTypes[i])))
			} else {
				out = append(out, boxed(a))
			}
		}
		return &Call{Fn: plan.RuntimeSymbol, Args: out, R: RepOfCType(plan.NativeReturnType)}

	case ir.DispatchClosure:
		callee := boxed(vals[calleeIdx])
		bargs := boxedAll(args)
		if n := len(bargs); n <= 12 {
			return &Call{Fn: fmt.Sprintf("q_call%d", n), Args: append([]Expr{callee}, bargs...), R: RepValue}
		}
		return &Call{Fn: "q_calln", Args: []Expr{callee, &InitList{Type: "std::vector<QValue>", Elems: bargs}}, R: RepValue}
	}
	l.ice("INV-CALLPLAN-DISPATCH", node, "unknown dispatch for '%s'", funcNode.TokenLiteral())
	return nil
}

// tableRead lowers a csv::read_file or csv::parse_text call to its native
// reader with the row's definition (D18), or nullptr when the column types
// come from the file. The wrapper's default mapping is no mapping (nullptr).
func (l *lowerer) tableRead(argsNode *ast.TreeNode, tr *types.TableRead) Expr {
	vals := l.operands(argsNode.Children)
	args := []Expr{toNative(vals[0], RepStr), nullptrExpr}
	if len(vals) > 1 {
		args[1] = toNative(vals[1], RepDict)
	}
	row := &Const{R: Rep("const QStructDef*"), Text: "nullptr"}
	if tr.Row != nil {
		row.Text = "&_sdef_" + structCName(tr.Row.Name)
	}
	return &Call{Fn: tr.Native, Args: append(args, row), R: RepValue}
}

func boxedAll(vs []Expr) []Expr {
	out := make([]Expr, len(vs))
	for i, v := range vs {
		out[i] = boxed(v)
	}
	return out
}

// toNative converts v to native rep want: a raw scalar of that kind as is (an
// int widened for a double), a QValue unchanged for QValue, anything else
// through the checked q_as_* conversions.
//
// A pointer from another native call is boxed and converted again rather than
// passed on: boxing is where a null pointer returned for str, list, dict, vec
// or fn is reported, so it never reaches the next native function.
func toNative(v Expr, want Rep) Expr {
	switch {
	case v.Rep() == want && want.IsScalar():
		return v
	case want == RepValue:
		return boxed(v)
	case want == RepFloat && v.Rep() == RepInt:
		return &Cast{X: v, To: RepFloat}
	}
	return adaptTo(boxed(v), want)
}

// dynamicMethod lowers a builtin method call on a receiver whose type is known
// only at runtime: a test of the receiver's tag per candidate, and a runtime
// error naming the call when none matches.
func (l *lowerer) dynamicMethod(funcNode *ast.TreeNode, plan *ir.CallPlan, recv Expr, args []Expr) Expr {
	t := l.newTemp(RepValue)
	l.add(&Let{T: t, Value: nullValue()})
	all := append([]Expr{recv}, args...)
	at := l.cur
	for _, c := range plan.MethodCandidates {
		then := &Block{Stmts: []Stmt{&SetTemp{T: t, Value: &Call{Fn: c.RuntimeSymbol, Args: all, R: RepValue}}}}
		els := &Block{}
		at.Stmts = append(at.Stmts, &If{Cond: &TagIs{X: recv, Tag: c.ValueTag}, Then: then, Else: els})
		at = els
	}
	// Point the error at this call, not at the last statement that ran.
	if loc := l.locOf(funcNode); loc != nil {
		at.Stmts = append(at.Stmts, loc)
	}
	at.Stmts = append(at.Stmts, &Do{X: &Call{Fn: "q_no_method", Args: []Expr{recv, &Const{R: RepStr, Text: cstr(plan.CalleeName)}}, R: Rep("void")}})
	return &TempRef{T: t}
}
