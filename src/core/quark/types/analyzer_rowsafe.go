package types

import (
	"fmt"
	"strings"

	"quark/ast"
	"quark/builtins"
	"quark/ir"
)

// Row-safety of calls in a verb (specs/tables_v0_3_spec.md D41-D44).
//
// A verb may call only functions that can run for any row, in any order, on
// several threads, and that give the same result for the same arguments. The
// compiler works that out for every fn declaration from its body, the way
// finalizeFixedFns works out which declarations need a closure: a declaration
// is unsafe when its body has an unsafe call or construct, or it calls a
// declaration that is unsafe. Recursive declarations with no unsafe call are
// row-safe.
//
// A row-safe call whose arguments don't depend on the row is the same for
// every row, so lowering evaluates it the first time a row needs it (D44).
// A random draw is row-safe but not the same twice, so it is never evaluated
// once; a verb call that draws, directly or through the functions it calls,
// has CallPlan.Draws set, and lowering gives its row loop a draw context
// (specs §5, core/rowrand.hpp).

// drawSymbols are the std/random draws. They are declared rowsafe, and each
// call draws its own number.
var drawSymbols = map[string]bool{
	"q_random_uniform_range": true,
	"q_random_normal":        true,
	"q_random_randint":       true,
	"q_random_choice":        true,
	"q_random_sample":        true,
	"q_random_weighted":      true,
	"q_random_uniform_vec":   true,
	"q_random_normal_vec":    true,
	"q_random_randint_vec":   true,
}

// verbCall is a call written in a verb expression.
type verbCall struct {
	node    *ast.TreeNode
	grouped bool
}

// rowEvent is one thing in a function body that can make it unsafe, in
// source order. Exactly one of fault and callee is set.
type rowEvent struct {
	fault  *rowFault
	callee *FixedFn
	name   string // the callee's internal name, when callee is set
	line   int
}

// rowFault is a reason that doesn't depend on another declaration.
type rowFault struct {
	kind string // "call", "native", "value", "assign", "elem"
	name string
	line int
}

func (f *rowFault) clause() string {
	switch f.kind {
	case "native":
		return fmt.Sprintf("calls the native function '%s', which isn't declared rowsafe", f.name)
	case "value":
		return fmt.Sprintf("calls a function value (line %d)", f.line)
	case "assign":
		return fmt.Sprintf("assigns '%s' (line %d)", f.name, f.line)
	case "elem":
		return fmt.Sprintf("changes a list or dict (line %d)", f.line)
	default:
		return fmt.Sprintf("calls '%s' (line %d)", f.name, f.line)
	}
}

// finalizeRowSafety classifies every fn declaration and rejects a verb call
// that isn't row-safe. It marks the calls lowering evaluates once per step.
func (a *Analyzer) finalizeRowSafety() {
	callFn := map[*ast.TreeNode]*FixedFn{}
	for _, c := range a.fixedCalls {
		callFn[c.node] = c.fn
	}
	inside := map[*FixedFn]map[*ast.TreeNode]bool{}
	for _, f := range a.fixedList {
		inside[f] = nodeSet(f.Lambda)
		a.walkRowSafe(f, f.Lambda, inside[f], callFn)
	}
	for _, f := range a.fixedList {
		f.rowSafe = true
		for _, e := range f.events {
			if e.fault != nil {
				f.rowSafe = false
				break
			}
		}
	}
	for changed := true; changed; {
		changed = false
		for _, f := range a.fixedList {
			if !f.rowSafe && f.draws {
				continue
			}
			for _, e := range f.events {
				if e.callee == nil {
					continue
				}
				if f.rowSafe && !e.callee.rowSafe {
					f.rowSafe = false
					changed = true
				}
				if !f.draws && e.callee.draws {
					f.draws = true
					changed = true
				}
			}
		}
	}
	for _, c := range a.verbCalls {
		a.checkVerbCall(c, callFn)
	}
}

// walkRowSafe records the calls and assignments of f's body. Nested
// functions are their own declarations; a lambda is a function value, and
// calling one is unsafe on its own, so neither body's contents count here.
func (a *Analyzer) walkRowSafe(f *FixedFn, lam *ast.TreeNode, inside map[*ast.TreeNode]bool, callFn map[*ast.TreeNode]*FixedFn) {
	if lam == nil {
		return
	}
	var walk func(*ast.TreeNode)
	walk = func(n *ast.TreeNode) {
		if n == nil || n.NodeType == ast.LambdaNode || n.NodeType == ast.FnDeclNode {
			return
		}
		switch n.NodeType {
		case ast.AssignNode:
			a.noteRowAssign(f, n, inside)
		case ast.FunctionCallNode:
			a.noteRowCall(f, n, callFn)
		}
		for _, c := range n.Children {
			walk(c)
		}
		walk(n.DefaultValue)
	}
	for _, c := range lam.Children {
		walk(c)
	}
}

func (a *Analyzer) noteRowAssign(f *FixedFn, n *ast.TreeNode, inside map[*ast.TreeNode]bool) {
	if len(n.Children) < 1 || n.Children[0] == nil {
		return
	}
	target := n.Children[0]
	switch target.NodeType {
	case ast.IndexNode, ast.MemberNode:
		if containerWrite(a, target) {
			f.events = append(f.events, rowEvent{fault: &rowFault{kind: "elem", line: nodeLine(n)}})
		}
	case ast.IdentifierNode:
		sym := a.refs[target]
		if sym == nil || sym.Kind == SymBuiltin || sym.Kind == SymFn || sym.Kind == SymRow {
			return
		}
		if sym.Decl != nil && inside[sym.Decl] {
			return
		}
		f.events = append(f.events, rowEvent{fault: &rowFault{kind: "assign", name: sym.Name, line: nodeLine(n)}})
	}
}

// containerWrite reports whether target is an element assignment of a list
// (`xs[i] = v`) or a dict (`d.key = v`). A struct field assignment is already
// an error, and a value of unknown type might be a list or a dict.
func containerWrite(a *Analyzer, target *ast.TreeNode) bool {
	if len(target.Children) == 0 || target.Children[0] == nil {
		return true
	}
	t := a.nodeTypes[target.Children[0]]
	if t == nil || isUnknownType(t) {
		return true
	}
	switch target.NodeType {
	case ast.IndexNode:
		_, ok := t.(*ListType)
		return ok
	case ast.MemberNode:
		_, ok := t.(*DictType)
		return ok
	}
	return false
}

func (a *Analyzer) noteRowCall(f *FixedFn, n *ast.TreeNode, callFn map[*ast.TreeNode]*FixedFn) {
	plan := a.callPlans[n]
	if plan == nil || plan.Dispatch == ir.DispatchAggregate || plan.Dispatch == ir.DispatchRowFillna {
		return
	}
	if g := callFn[n]; g != nil {
		f.events = append(f.events, rowEvent{callee: g, name: g.Name, line: nodeLine(n)})
		return
	}
	line := nodeLine(n)
	switch plan.Dispatch {
	case ir.DispatchExtern:
		if plan.Draws {
			f.draws = true
		}
		if !plan.RowSafe {
			f.events = append(f.events, rowEvent{fault: &rowFault{kind: "native", name: a.bareName(plan.CalleeName), line: line}})
		}
	case ir.DispatchBuiltin:
		if !a.builtinRowSafe(plan) {
			f.events = append(f.events, rowEvent{fault: &rowFault{kind: "call", name: plan.CalleeName, line: line}})
		}
	case ir.DispatchDynamicMethod:
		if !a.dynamicRowSafe(plan) {
			f.events = append(f.events, rowEvent{fault: &rowFault{kind: "call", name: plan.CalleeName, line: line}})
		}
	default:
		f.events = append(f.events, rowEvent{fault: &rowFault{kind: "value", line: line}})
	}
}

func (a *Analyzer) builtinRowSafe(plan *ir.CallPlan) bool {
	if plan.IsMethod {
		spec, ok := builtins.LookupMethod(builtins.TypeKey(plan.ReceiverTypeKey), plan.CalleeName)
		return ok && spec.RowSafe
	}
	spec, ok := builtins.Lookup(plan.CalleeName)
	return ok && spec.RowSafe
}

func (a *Analyzer) dynamicRowSafe(plan *ir.CallPlan) bool {
	if len(plan.MethodCandidates) == 0 {
		return false
	}
	for _, c := range plan.MethodCandidates {
		spec, ok := builtins.LookupMethod(builtins.TypeKey(c.ReceiverTypeKey), plan.CalleeName)
		if !ok || !spec.RowSafe {
			return false
		}
	}
	return true
}

// noteVerbCall records a call written in a verb expression. Aggregates are
// syntax, not calls. The call's type isn't stored yet (this runs before
// Analyze records it); a call that failed to type is dropped in checkVerbCall.
func (a *Analyzer) noteVerbCall(node *ast.TreeNode, grouped bool) {
	plan := a.callPlans[node]
	if plan == nil || plan.Dispatch == ir.DispatchAggregate || plan.Dispatch == ir.DispatchRowFillna {
		return
	}
	a.verbCalls = append(a.verbCalls, verbCall{node: node, grouped: grouped})
}

func (a *Analyzer) checkVerbCall(c verbCall, callFn map[*ast.TreeNode]*FixedFn) {
	node := c.node
	if IsErrorType(a.nodeTypes[node]) {
		return
	}
	plan := a.callPlans[node]
	if plan == nil {
		return
	}
	if g := callFn[node]; g != nil {
		if !g.rowSafe {
			a.errorAt(node, "'%s' can't be called in a query: it %s", g.Name, a.explainUnsafe(g, map[*FixedFn]bool{}))
		} else if g.draws {
			// Lowering sets the row's draw context for a loop with a draw
			// (specs/tables_v0_3_spec.md §5).
			plan.Draws = true
		} else if !a.callArgsVary(node, c.grouped, callFn) {
			plan.Once = true
		}
		return
	}
	switch plan.Dispatch {
	case ir.DispatchExtern:
		if !plan.RowSafe {
			a.errorAt(node, "'%s' can't be called in a query: it isn't declared rowsafe", a.bareName(plan.CalleeName))
			return
		}
		if !plan.Draws && !a.callArgsVary(node, c.grouped, callFn) {
			plan.Once = true
		}
	case ir.DispatchBuiltin:
		if !a.builtinRowSafe(plan) {
			a.errorAt(node, "'%s' can't be called in a query", plan.CalleeName)
			return
		}
		if !a.callArgsVary(node, c.grouped, callFn) {
			plan.Once = true
		} else if isContainsCall(plan) && !a.exprVaries(plan.ReceiverNode, c.grouped, callFn) {
			if a.lookupSetOK(node, plan) {
				plan.Lookup = true
			}
		}
	case ir.DispatchDynamicMethod:
		if !a.dynamicRowSafe(plan) {
			a.errorAt(node, "'%s' can't be called in a query", plan.CalleeName)
			return
		}
		if !a.callArgsVary(node, c.grouped, callFn) {
			plan.Once = true
		}
	default:
		if len(node.Children) > 0 && node.Children[0] != nil && node.Children[0].NodeType == ast.IdentifierNode {
			a.errorAt(node, "'%s' can't be called in a query: it's a function value", plan.CalleeName)
		} else {
			a.errorAt(node, "a function value can't be called in a query")
		}
	}
}

// explainUnsafe is the path from f to the call or assignment that makes it
// unsafe: "calls 'band' (line 8), which calls 'print' (line 3)".
func (a *Analyzer) explainUnsafe(f *FixedFn, seen map[*FixedFn]bool) string {
	if f == nil || seen[f] {
		return "isn't row-safe"
	}
	seen[f] = true
	// A direct fault in this body is the reason, even when an earlier call
	// happens to be another unsafe function (a recursive call before print).
	for _, e := range f.events {
		if e.fault != nil {
			return e.fault.clause()
		}
	}
	for _, e := range f.events {
		if e.callee != nil && !e.callee.rowSafe {
			return fmt.Sprintf("calls '%s' (line %d), which %s", e.name, e.line, a.explainUnsafe(e.callee, seen))
		}
	}
	return "isn't row-safe"
}

// callArgsVary reports whether a call's receiver or arguments depend on the
// row, or on a draw, so the call is not the same for every row.
func (a *Analyzer) callArgsVary(call *ast.TreeNode, grouped bool, callFn map[*ast.TreeNode]*FixedFn) bool {
	plan := a.callPlans[call]
	if plan != nil && plan.ReceiverNode != nil && a.exprVaries(plan.ReceiverNode, grouped, callFn) {
		return true
	}
	if len(call.Children) > 1 && a.exprVaries(call.Children[1], grouped, callFn) {
		return true
	}
	for _, d := range plan.DefaultNodes {
		if a.exprVaries(d, grouped, callFn) {
			return true
		}
	}
	return false
}

// exprVaries reports whether n's value depends on the row or on a draw.
// An aggregate is one value for an ungrouped step and the row's group
// otherwise; the columns it reads are not row reads of this expression.
func (a *Analyzer) exprVaries(n *ast.TreeNode, grouped bool, callFn map[*ast.TreeNode]*FixedFn) bool {
	if n == nil || n.NodeType == ast.LambdaNode {
		return false
	}
	if _, ok := a.columnRefs[n]; ok {
		return true
	}
	if _, ok := a.aggUses[n]; ok {
		return grouped
	}
	if n.NodeType == ast.FunctionCallNode && a.callDraws(n, callFn) {
		return true
	}
	for _, c := range n.Children {
		if a.exprVaries(c, grouped, callFn) {
			return true
		}
	}
	return a.exprVaries(n.DefaultValue, grouped, callFn)
}

func (a *Analyzer) callDraws(n *ast.TreeNode, callFn map[*ast.TreeNode]*FixedFn) bool {
	if g := callFn[n]; g != nil {
		return g.draws
	}
	plan := a.callPlans[n]
	return plan != nil && plan.Draws
}

// bareName is the declared name of a native function, without the file
// qualifier Display would add (the message says 'c_norm', not 'lib::c_norm').
func (a *Analyzer) bareName(internal string) string {
	shown := a.names.Display(internal)
	if i := strings.LastIndex(shown, "::"); i >= 0 {
		return shown[i+2:]
	}
	return shown
}

func nodeSet(n *ast.TreeNode) map[*ast.TreeNode]bool {
	set := map[*ast.TreeNode]bool{}
	var walk func(*ast.TreeNode)
	walk = func(n *ast.TreeNode) {
		if n == nil || set[n] {
			return
		}
		set[n] = true
		for _, c := range n.Children {
			walk(c)
		}
		walk(n.ReturnType)
		walk(n.DefaultValue)
	}
	walk(n)
	return set
}
