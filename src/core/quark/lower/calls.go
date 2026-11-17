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
	// A call that doesn't depend on the row is the same for every row. It
	// stays where the expression evaluates it, so a row that doesn't reach
	// it doesn't run it, and the first row that does is where an error is
	// reported (D44).
	if plan.Once && l.row != nil && l.onceNode != node {
		return l.onceCall(node)
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
		if l.useForeignThunk(plan) {
			// The C++ function lives in the input that included the header.
			// Its wrapper takes Quark values and returns a Quark value.
			out := []Expr{nullptrExpr}
			if recvIdx >= 0 {
				out = append(out, boxed(vals[recvIdx]))
			}
			for _, a := range args {
				out = append(out, boxed(a))
			}
			return &Call{Fn: "_thunk_" + plan.RuntimeSymbol, Args: out, R: RepValue}
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
		return l.keepArgsAlive(&Call{Fn: plan.RuntimeSymbol, Args: out, R: RepOfCType(plan.NativeReturnType)})

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

// onceCall evaluates node the first time a row reaches it and reuses the
// result for later rows. The slot is declared before the loop (rowFor);
// the call itself stays in this branch, so a row that doesn't reach here
// doesn't make it, and a failure is reported at this row.
func (l *lowerer) onceCall(node *ast.TreeNode) Expr {
	if l.once == nil {
		l.ice("INV-QUERY-ONCE", node, "a call computed once per step is outside a row loop")
	}
	var value Expr
	then := l.sub(func() {
		l.setLoc(node)
		prev := l.onceNode
		l.onceNode = node
		value = l.call(node)
		l.onceNode = prev
	})
	if value == nil {
		return nullValue()
	}
	slot := l.once.slot(l, node, value.Rep())
	then.Stmts = append(then.Stmts,
		&SetTemp{T: slot.value, Value: value},
		&SetTemp{T: slot.ready, Value: trueExpr},
	)
	l.add(&If{Cond: &Not{X: &TempRef{T: slot.ready}}, Then: then})
	return &TempRef{T: slot.value}
}

// keepArgsAlive keeps every argument of a call into native code that refers
// to GC memory reachable until the call returns (q_keep_alive after it).
// Native code may hold only a handle's payload, which doesn't refer to the
// handle; if nothing else did, a collection inside the call could finalize
// the handle and close the payload in use. The arguments become temporaries,
// in order, so they can be named after the call.
func (l *lowerer) keepArgsAlive(call *Call) Expr {
	keep := false
	for _, a := range call.Args {
		keep = keep || refersToGC(a.Rep())
	}
	if !keep {
		return call
	}
	var kept []Expr
	for i, a := range call.Args {
		call.Args[i] = l.stable(a)
		if refersToGC(a.Rep()) {
			kept = append(kept, call.Args[i])
		}
	}
	result := l.let(call)
	for _, k := range kept {
		l.add(&Do{X: &Call{Fn: "q_keep_alive", Args: []Expr{k}, R: Rep("void")}})
	}
	return result
}

// refersToGC reports whether a value of rep r can refer to GC memory: a
// QValue or a native pointer, not a raw scalar.
func refersToGC(r Rep) bool {
	return !r.IsScalar() && r != Rep("std::nullptr_t")
}

// tableRead lowers a csv::read_file or csv::parse_text call to its native
// reader with the row's definition (D18), or nullptr when the column types
// come from the file. The wrapper's default mapping is no mapping (nullptr).
//
// The reader is inline in the header the module included. A later REPL input
// cannot call the earlier input's wrapper: the wrapper does not take the row
// struct. This input includes the header and compiles the call itself.
func (l *lowerer) tableRead(argsNode *ast.TreeNode, tr *types.TableRead) Expr {
	if !l.externDefinedHere(tr.Native) {
		headers := l.nativeHeaders[tr.Native]
		if len(headers) == 0 {
			l.ice("INV-REPL", argsNode, "csv read calls '%s', whose declaring header is not in this input", tr.Native)
		}
		for _, h := range headers {
			l.include(h)
		}
	}
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
