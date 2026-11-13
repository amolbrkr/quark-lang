package lower

import (
	"fmt"

	"quark/ast"
	"quark/ir"
	"quark/token"
	"quark/types"
)

var (
	nullptrExpr = &Const{R: Rep("std::nullptr_t"), Text: "nullptr"}
)

func nullValue() Expr { return &Const{R: RepValue, Text: "qv_null()"} }

// cstr is s as a C++ string literal.
func cstr(s string) string { return "\"" + escapeCppString(s) + "\"" }

// boxed is v as a QValue.
func boxed(v Expr) Expr {
	if v.Rep() == RepValue {
		return v
	}
	if c, ok := v.(*Const); ok && c.R.IsScalar() {
		return &Box{X: c}
	}
	return &Box{X: v}
}

// adaptTo converts a QValue to native rep r with the checked q_as_* helpers;
// QValue stays as it is.
func adaptTo(v Expr, r Rep) Expr {
	if r == RepValue {
		return v
	}
	return &Adapt{X: v, To: r}
}

// isStable reports whether evaluating v later (or twice) gives the same value
// with no effects: constants, temporaries, parameters and variables that
// aren't shared cells, and function values.
func isStable(v Expr) bool {
	switch e := v.(type) {
	case *Const, *TempRef, *ParamRef, *FuncValue:
		return true
	case *VarRef:
		return e.Sym.Storage != ir.StorageCell
	case *Box:
		return isStable(e.X)
	case *Cast:
		return isStable(e.X)
	}
	return false
}

// isQuiet reports whether evaluating v has no effect another operand could
// observe: no calls (except value constructors), no checks. A quiet operand
// doesn't force the operands before it into temporaries.
func isQuiet(v Expr) bool {
	switch e := v.(type) {
	case *Const, *TempRef, *ParamRef, *FuncValue:
		return true
	case *VarRef:
		return !e.Checked
	case *Box:
		return isQuiet(e.X)
	case *Cast:
		return isQuiet(e.X)
	case *Unbox:
		return isQuiet(e.X)
	case *Arith:
		return isQuiet(e.L) && isQuiet(e.X)
	case *Neg:
		return isQuiet(e.X)
	case *Compare:
		return isQuiet(e.L) && isQuiet(e.X)
	case *Not:
		return isQuiet(e.X)
	case *AndAlso:
		return isQuiet(e.L) && isQuiet(e.X)
	case *OrElse:
		return isQuiet(e.L) && isQuiet(e.X)
	case *Call:
		if !e.Pure {
			return false
		}
		for _, a := range e.Args {
			if !isQuiet(a) {
				return false
			}
		}
		return true
	}
	return false
}

// operands lowers nodes left to right. An operand is moved into a temporary
// when a later operand needs statements or has effects, so it is evaluated
// before them.
func (l *lowerer) operands(nodes []*ast.TreeNode) []Expr {
	blocks := make([]*Block, len(nodes))
	vals := make([]Expr, len(nodes))
	for i, n := range nodes {
		n := n
		blocks[i] = l.sub(func() { vals[i] = l.expr(n) })
	}
	return l.sequence(blocks, vals)
}

// sequence appends the operands' statements in order, moving each operand
// that a later one could disturb into a temporary first.
func (l *lowerer) sequence(blocks []*Block, vals []Expr) []Expr {
	last := -1
	for i := range vals {
		if len(blocks[i].Stmts) > 0 || !isQuiet(vals[i]) {
			last = i
		}
	}
	for i := range vals {
		l.cur.Stmts = append(l.cur.Stmts, blocks[i].Stmts...)
		if i < last && !isStable(vals[i]) {
			vals[i] = l.let(vals[i])
		}
	}
	return vals
}

// expr lowers node for its value.
func (l *lowerer) expr(node *ast.TreeNode) Expr {
	if node == nil {
		return nullValue()
	}
	switch node.NodeType {
	case ast.LiteralNode:
		return literal(node)
	case ast.IdentifierNode:
		return l.identifier(node)
	case ast.FnDeclNode:
		return l.fixedFnValue(node)
	case ast.AssignNode:
		return l.assign(node)
	case ast.MemberNode:
		return l.member(node)
	case ast.UnaryNode:
		return l.unary(node)
	case ast.BinaryNode:
		return l.binary(node)
	case ast.FunctionCallNode:
		return l.call(node)
	case ast.TernaryNode:
		return l.ternary(node)
	case ast.IfStatementNode:
		return l.lowerIf(node, true)
	case ast.WhenStatementNode:
		return l.lowerWhen(node, true)
	case ast.ForLoopNode:
		l.lowerFor(node)
		return nullValue()
	case ast.WhileLoopNode:
		l.lowerWhile(node)
		return nullValue()
	case ast.ListNode:
		return l.list(node)
	case ast.VectorNode:
		return l.vector(node)
	case ast.DictNode:
		return l.dict(node)
	case ast.IndexNode:
		if len(node.Children) < 2 {
			return nullValue()
		}
		ops := l.operands(node.Children[:2])
		return &Call{Fn: "q_index", Args: []Expr{boxed(ops[0]), boxed(ops[1])}, R: RepValue}
	case ast.ResultNode:
		if len(node.Children) == 0 {
			return nullValue()
		}
		v := boxed(l.expr(node.Children[0]))
		if node.Token != nil && node.Token.Type == token.ERR {
			return &Call{Fn: "qv_err", Args: []Expr{v}, R: RepValue}
		}
		return &Call{Fn: "qv_ok", Args: []Expr{v}, R: RepValue}
	case ast.VarDeclNode:
		if len(node.Children) < 3 {
			return nullValue()
		}
		return l.assignVar(node.Children[0], node.Children[2])
	case ast.LambdaNode:
		return l.lambdaValue(node)
	case ast.BlockNode:
		return l.blockValue(node)
	case ast.StructLiteralNode:
		return l.structLiteral(node)
	case ast.ModuleNode, ast.StructDefNode, ast.ExternHeaderNode, ast.ExternFnNode,
		ast.ExternLinkNode, ast.ExternIncludePathNode:
		return nullValue()
	case ast.BreakNode:
		l.add(&Break{})
		return nullValue()
	case ast.ContinueNode:
		l.add(&Continue{})
		return nullValue()
	}
	l.ice("INV-NODE-TYPE", node, "unhandled AST node type '%s'", node.NodeType.String())
	return nil
}

// isVec reports whether the analyzer typed node as a vec.
func (l *lowerer) isVec(node *ast.TreeNode) bool {
	_, ok := l.nodeType(node).(*types.VectorType)
	return ok
}

func (l *lowerer) isList(node *ast.TreeNode) bool {
	_, ok := l.nodeType(node).(*types.ListType)
	return ok
}

func literal(node *ast.TreeNode) Expr {
	if node.Token == nil {
		return nullValue()
	}
	switch node.Token.Type {
	case token.INT:
		return &Const{R: RepInt, Text: node.Token.Literal}
	case token.FLOAT:
		return &Const{R: RepFloat, Text: node.Token.Literal}
	case token.STRING:
		return &Const{R: RepValue, Text: fmt.Sprintf("qv_string(%s)", cstr(node.Token.Literal))}
	case token.TRUE:
		return &Const{R: RepBool, Text: "true"}
	case token.FALSE:
		return &Const{R: RepBool, Text: "false"}
	}
	return nullValue()
}

func (l *lowerer) nodeType(node *ast.TreeNode) types.Type {
	if node == nil {
		return nil
	}
	return l.an.NodeTypes[node]
}

// varSym is the variable symbol of an identifier node the analyzer resolved.
func (l *lowerer) varSym(node *ast.TreeNode) *types.Symbol {
	sym := l.an.Refs[node]
	if !sym.IsVariable() || sym.CName == "" {
		l.ice("INV-SYMBOL", node, "no variable symbol for '%s'", node.TokenLiteral())
	}
	return sym
}

func (l *lowerer) readVar(sym *types.Symbol) Expr {
	r := &VarRef{Sym: sym}
	if sym.Storage == ir.StorageCell && l.checkedCells[sym] {
		r.Checked = true
		r.Display = l.names.Display(sym.Name)
	}
	return r
}

func (l *lowerer) identifier(node *ast.TreeNode) Expr {
	name := node.TokenLiteral()
	if name == "_" {
		return nullValue()
	}
	if f := l.an.FnRefs[node]; f != nil {
		return directFnValue(f)
	}
	if sym := l.an.Refs[node]; sym.IsVariable() {
		return l.readVar(sym)
	}
	if proto, ok := l.an.ExternFns[name]; ok && !proto.IsMethod {
		return &FuncValue{Fn: "_thunk_" + proto.RuntimeSymbol, Info: externFnInfoName(proto.RuntimeSymbol)}
	}
	// A prelude builtin used as a value (changelog.md, Known issues): there
	// is no C++ function of that name, so clang reports it.
	return &Const{R: RepValue, Text: "quark_" + name}
}

// directFnValue is the function value of a declaration that needs no closure.
func directFnValue(f *types.FixedFn) Expr {
	if f.Native != nil {
		return &FuncValue{Fn: "quark_" + f.CName + "__thunk", Info: fnInfoName(f.CName)}
	}
	return &FuncValue{Fn: "quark_" + f.CName, Info: fnInfoName(f.CName)}
}

// fixedFnValue is the value of a `fn` declaration used as an expression.
func (l *lowerer) fixedFnValue(node *ast.TreeNode) Expr {
	f := l.an.FixedFns[node.Children[1]]
	if f == nil {
		l.ice("INV-FIXED-FN", node, "declaration '%s' was not analyzed", node.Children[0].TokenLiteral())
	}
	if f.Captures {
		return l.identifier(node.Children[0])
	}
	return directFnValue(f)
}

// lambdaValue is the function value of a lambda: a closure over its captured
// cells, or a plain function value.
func (l *lowerer) lambdaValue(node *ast.TreeNode) Expr {
	fd := l.fnByLambda[node]
	if fd == nil {
		l.ice("INV-LAMBDA", node, "lambda was not collected")
	}
	if caps := l.an.Captures[node]; len(caps) > 0 {
		return l.let(&MakeClosure{Fn: "quark_" + fd.name, Info: fnInfoName(fd.name), Cells: caps})
	}
	if fd.native != nil {
		return &FuncValue{Fn: "quark_" + fd.name + "__thunk", Info: fnInfoName(fd.name)}
	}
	return &FuncValue{Fn: "quark_" + fd.name, Info: fnInfoName(fd.name)}
}

// ---------------------------------------------------------------------------
// Operators

func (l *lowerer) unary(node *ast.TreeNode) Expr {
	if node.Token == nil || len(node.Children) != 1 {
		return nullValue()
	}
	switch node.Token.Type {
	case token.MINUS:
		x := l.expr(node.Children[0])
		switch x.Rep() {
		case RepInt:
			return &Neg{X: x, Checked: node.Children[0].NodeType != ast.LiteralNode}
		case RepFloat:
			return &Neg{X: x}
		}
		return &Call{Fn: "q_neg", Args: []Expr{boxed(x)}, R: RepValue}
	case token.BANG:
		return &Not{X: l.cond(node.Children[0])}
	}
	return l.expr(node.Children[0])
}

var binaryRuntimeOps = map[token.TokenType]string{
	token.PLUS:       "q_add",
	token.MINUS:      "q_sub",
	token.MULTIPLY:   "q_mul",
	token.DIVIDE:     "q_div",
	token.MODULO:     "q_mod",
	token.DOUBLESTAR: "q_pow",
	token.LT:         "q_lt",
	token.LTE:        "q_lte",
	token.GT:         "q_gt",
	token.GTE:        "q_gte",
	token.DEQ:        "q_eq",
	token.NE:         "q_neq",
}

var arithOps = map[token.TokenType]string{
	token.PLUS: "+", token.MINUS: "-", token.MULTIPLY: "*", token.DIVIDE: "/", token.MODULO: "%",
}

var compareOps = map[token.TokenType]string{
	token.LT: "<", token.LTE: "<=", token.GT: ">", token.GTE: ">=", token.DEQ: "==", token.NE: "!=",
}

// promotedRep is the result of raw arithmetic op on reps a and b: "" when it
// can't be done on raw scalars. Division always gives a double.
func promotedRep(a, b Rep, op token.TokenType) Rep {
	switch {
	case !a.IsScalar() || !b.IsScalar():
		return ""
	case a == RepFloat || b == RepFloat:
		return RepFloat
	case op == token.DIVIDE && a == RepInt && b == RepInt:
		return RepFloat
	case a == RepInt && b == RepInt:
		return RepInt
	case a == RepBool && b == RepBool:
		return RepBool
	}
	return ""
}

func (l *lowerer) binary(node *ast.TreeNode) Expr {
	if node.Token == nil || len(node.Children) != 2 {
		return nullValue()
	}
	op := node.Token.Type
	switch op {
	case token.AND, token.OR:
		return l.logical(node, op == token.AND)
	}
	ops := l.operands(node.Children)
	a, b := ops[0], ops[1]
	if cop, ok := arithOps[op]; ok {
		if r := promotedRep(a.Rep(), b.Rep(), op); r != "" {
			return &Arith{Op: cop, L: a, X: b, R: r}
		}
	}
	// Raw comparisons are for two numbers or two bools. A bool and a number
	// are different types, so `==` is false (semantics.md §4.2): the boxed
	// q_eq says so, where C++ would compare true with 1.
	if cop, ok := compareOps[op]; ok && a.Rep().IsScalar() && b.Rep().IsScalar() {
		ordered := op == token.LT || op == token.LTE || op == token.GT || op == token.GTE
		numeric := a.Rep() != RepBool && b.Rep() != RepBool
		if numeric || (!ordered && a.Rep() == b.Rep()) {
			return &Compare{Op: cop, L: a, X: b}
		}
	}
	fn, ok := binaryRuntimeOps[op]
	if !ok {
		l.ice("INV-OPERATOR", node, "unhandled binary operator '%s'", op.String())
	}
	return &Call{Fn: fn, Args: []Expr{boxed(a), boxed(b)}, R: RepValue}
}

// logical lowers `and` / `or`, which return one of their operands; the right
// operand (and everything it needs) runs only when the left doesn't decide.
func (l *lowerer) logical(node *ast.TreeNode, isAnd bool) Expr {
	a := l.expr(node.Children[0])
	var b Expr
	rb := l.sub(func() { b = l.expr(node.Children[1]) })
	if a.Rep() == RepBool && b.Rep() == RepBool && len(rb.Stmts) == 0 {
		if isAnd {
			return &AndAlso{L: a, X: b}
		}
		return &OrElse{L: a, X: b}
	}
	t := l.newTemp(RepValue)
	l.add(&Let{T: t, Value: boxed(a)})
	var test Expr = &Truthy{X: &TempRef{T: t}}
	if !isAnd {
		test = &Not{X: test}
	}
	rb.Stmts = append(rb.Stmts, &SetTemp{T: t, Value: boxed(b)})
	l.add(&If{Cond: test, Then: rb})
	return &TempRef{T: t}
}

// cond lowers a condition to a raw bool: a raw bool as is, anything else
// through q_truthy.
func (l *lowerer) cond(node *ast.TreeNode) Expr {
	v := l.expr(node)
	if v.Rep() == RepBool {
		return v
	}
	return &Truthy{X: boxed(v)}
}

func (l *lowerer) ternary(node *ast.TreeNode) Expr {
	if len(node.Children) < 3 {
		return nullValue()
	}
	c := l.cond(node.Children[0])
	var tv, fv Expr
	tb := l.sub(func() { tv = l.expr(node.Children[1]) })
	fb := l.sub(func() { fv = l.expr(node.Children[2]) })
	if len(tb.Stmts) == 0 && len(fb.Stmts) == 0 {
		if tv.Rep() == fv.Rep() && tv.Rep().IsScalar() {
			return &Select{C: c, T: tv, F: fv, R: tv.Rep()}
		}
		return &Select{C: c, T: boxed(tv), F: boxed(fv), R: RepValue}
	}
	t := l.newTemp(RepValue)
	l.add(&Let{T: t, Value: nullValue()})
	tb.Stmts = append(tb.Stmts, &SetTemp{T: t, Value: boxed(tv)})
	fb.Stmts = append(fb.Stmts, &SetTemp{T: t, Value: boxed(fv)})
	l.add(&If{Cond: c, Then: tb, Else: fb})
	return &TempRef{T: t}
}

// ---------------------------------------------------------------------------
// if / when

// lowerIf lowers an if/elseif/else chain. Each elseif condition is lowered
// inside the else branch of the test before it, so it runs only when needed.
func (l *lowerer) lowerIf(node *ast.TreeNode, wantValue bool) Expr {
	if len(node.Children) < 2 {
		return nullValue()
	}
	var t *Temp
	if wantValue {
		t = l.newTemp(RepValue)
		l.add(&Let{T: t, Value: nullValue()})
	}
	arm := func(body *ast.TreeNode) *Block {
		return l.sub(func() {
			if wantValue {
				v := l.branchValue(body)
				l.add(&SetTemp{T: t, Value: boxed(v)})
			} else {
				l.branch(body)
			}
		})
	}
	top := &If{Cond: l.cond(node.Children[0]), Then: arm(node.Children[1])}
	l.add(top)
	at := top
	for i := 2; i < len(node.Children); i++ {
		child := node.Children[i]
		if child.NodeType == ast.IfStatementNode && len(child.Children) >= 2 {
			var next *If
			at.Else = l.sub(func() {
				next = &If{Cond: l.cond(child.Children[0])}
				next.Then = arm(child.Children[1])
				l.add(next)
			})
			at = next
		} else {
			at.Else = arm(child)
		}
	}
	if wantValue {
		return &TempRef{T: t}
	}
	return nullValue()
}

func (l *lowerer) lowerWhen(node *ast.TreeNode, wantValue bool) Expr {
	if len(node.Children) < 2 {
		return nullValue()
	}
	m := l.newTemp(RepValue)
	l.add(&Let{T: m, Value: boxed(l.expr(node.Children[0]))})
	var t *Temp
	if wantValue {
		t = l.newTemp(RepValue)
		l.add(&Let{T: t, Value: nullValue()})
	}
	match := &TempRef{T: m}
	at := l.cur
	for i := 1; i < len(node.Children); i++ {
		pattern := node.Children[i]
		if pattern.NodeType != ast.PatternNode || len(pattern.Children) < 2 {
			continue
		}
		resultIdx := len(pattern.Children) - 1
		type binding struct {
			node *ast.TreeNode
			isOk bool
		}
		var bindings []binding
		var test Expr
		var elseBlock *Block
		saved := l.cur
		l.cur = at
		// The arm's tests: each pattern after the previous one failed.
		var ok *Temp
		for j := 0; j < resultIdx; j++ {
			pe := pattern.Children[j]
			var c Expr
			pre := l.sub(func() {
				switch pe.NodeType {
				case ast.IdentifierNode:
					if pe.TokenLiteral() == "_" {
						c = &Const{R: RepBool, Text: "true"}
						return
					}
					c = &Call{Fn: "q_match_eq", Args: []Expr{match, boxed(l.expr(pe))}, R: RepBool}
				case ast.ResultPatternNode:
					isOk := pe.Token == nil || pe.Token.Type != token.ERR
					c = &Call{Fn: "q_is_ok", Args: []Expr{match}, R: RepBool}
					if !isOk {
						c = &Not{X: c}
					}
					if len(pe.Children) > 0 && pe.Children[0] != nil {
						if name := pe.Children[0].TokenLiteral(); name != "" && name != "_" {
							bindings = append(bindings, binding{node: pe.Children[0], isOk: isOk})
						}
					}
				default:
					c = &Call{Fn: "q_match_eq", Args: []Expr{match, boxed(l.expr(pe))}, R: RepBool}
				}
			})
			switch {
			case test == nil && ok == nil:
				l.cur.Stmts = append(l.cur.Stmts, pre.Stmts...)
				test = c
			case ok == nil && len(pre.Stmts) == 0:
				test = &OrElse{L: test, X: c}
			default:
				if ok == nil {
					ok = l.newTemp(RepBool)
					l.add(&Let{T: ok, Value: test})
				}
				pre.Stmts = append(pre.Stmts, &SetTemp{T: ok, Value: c})
				l.add(&If{Cond: &Not{X: &TempRef{T: ok}}, Then: pre})
				test = &TempRef{T: ok}
			}
		}
		if test == nil {
			test = &Const{R: RepBool, Text: "false"}
		}
		then := l.sub(func() {
			for _, b := range bindings {
				fn := "q_result_value"
				if !b.isOk {
					fn = "q_result_error"
				}
				sym := l.varSym(b.node)
				v := &Call{Fn: fn, Args: []Expr{match}, R: RepValue}
				l.add(&DeclVar{Sym: sym, Value: l.toStorage(sym, nil, v)})
			}
			if wantValue {
				v := l.branchValue(pattern.Children[resultIdx])
				l.add(&SetTemp{T: t, Value: boxed(v)})
			} else {
				l.branch(pattern.Children[resultIdx])
			}
		})
		elseBlock = &Block{}
		l.add(&If{Cond: test, Then: then, Else: elseBlock})
		l.cur = saved
		at = elseBlock
	}
	if wantValue {
		return &TempRef{T: t}
	}
	return nullValue()
}

// ---------------------------------------------------------------------------
// Loops

// rangeArgs returns the argument nodes of a range() call.
func (l *lowerer) rangeArgs(node *ast.TreeNode) ([]*ast.TreeNode, bool) {
	if node == nil || node.NodeType != ast.FunctionCallNode || len(node.Children) < 2 {
		return nil, false
	}
	plan := l.an.CallPlans[node]
	if plan == nil || plan.RuntimeSymbol != "q_range" {
		return nil, false
	}
	args := node.Children[1]
	if args == nil || len(args.Children) == 0 {
		return nil, false
	}
	return args.Children, true
}

func (l *lowerer) lowerFor(node *ast.TreeNode) {
	if len(node.Children) < 3 {
		return
	}
	varNode, iterNode, bodyNode := node.Children[0], node.Children[1], node.Children[2]
	sym := l.varSym(varNode)

	// for i in range(...): a counted C++ loop, no list.
	if sym.Storage == ir.StorageInt {
		if args, ok := l.rangeArgs(iterNode); ok {
			l.forRange(sym, args, bodyNode)
			return
		}
	}

	it := l.newTemp(RepValue)
	l.add(&Let{T: it, Value: boxed(l.expr(iterNode))})
	iter := &TempRef{T: it}

	// The loop variable takes each element in turn. Its scalar storage is
	// the element type the analyzer inferred, which every write into the
	// list was checked against (architecture.md §12, rule C).
	bind := func(elem Expr) {
		var v Expr = elem
		if sym.Storage.IsScalar() {
			v = &Unbox{X: elem, To: Rep(sym.Storage.CType()), Proof: ProofElement, Type: sym.StoreType}
		}
		l.add(&DeclVar{Sym: sym, Value: v})
	}

	// Over a list: read its elements directly. The length is counted once,
	// and the loop test fails if the body changed it, so every read is in
	// range.
	if l.isList(iterNode) {
		lst := l.newTemp(RepList)
		l.add(&Let{T: lst, Value: &Call{Fn: "q_for_list", Args: []Expr{iter}, R: RepList}})
		n := l.newTemp(RepInt)
		l.add(&Let{T: n, Value: &Call{Fn: "q_list_len", Args: []Expr{&TempRef{T: lst}}, R: RepInt}})
		idx := l.newTemp(RepInt)
		body := l.sub(func() {
			bind(&Call{Fn: "q_list_at", Args: []Expr{&TempRef{T: lst}, &TempRef{T: idx}}, R: RepValue})
			l.branch(bodyNode)
		})
		l.add(&ForIndex{Idx: idx, Len: &TempRef{T: n}, Sized: &TempRef{T: lst}, Loc: l.locPtr(node), Body: body})
		return
	}

	// q_iter_get reports a vec's missing entry (D12). Over a vec, the loop
	// records its own location first so the error points at the `for`; with a
	// scalar loop variable only vectors with missing entries come here, the
	// others take the buffer loop below.
	vecIter := l.isVec(iterNode)
	generic := func() {
		n := l.newTemp(RepInt)
		l.add(&Let{T: n, Value: &Call{Fn: "q_for_len", Args: []Expr{iter}, R: RepInt}})
		idx := l.newTemp(RepInt)
		body := l.sub(func() {
			if vecIter {
				l.setLoc(node)
			}
			bind(&Call{Fn: "q_iter_get", Args: []Expr{iter, &Box{X: &TempRef{T: idx}}}, R: RepValue})
			l.branch(bodyNode)
		})
		// A list of unknown type at compile time gets the same length check.
		l.add(&ForIndex{Idx: idx, Len: &TempRef{T: n}, Sized: iter, Loc: l.locPtr(node), Body: body})
	}

	// A scalar loop variable over a dense vector of the matching dtype reads
	// the vector's buffer directly.
	dtype := map[ir.Storage]string{
		ir.StorageInt:   "QVector::Type::I64",
		ir.StorageFloat: "QVector::Type::F64",
		ir.StorageBool:  "QVector::Type::BOOL",
	}[sym.Storage]
	if dtype == "" {
		generic()
		return
	}
	fast := l.sub(func() {
		ref := l.newTemp(RepVecRef)
		l.add(&Let{T: ref, Value: &VecRef{X: iter}})
		idx := l.newTemp(RepInt)
		body := l.sub(func() {
			l.add(&DeclVar{Sym: sym, Value: &VecElem{V: &TempRef{T: ref}, Idx: &TempRef{T: idx}, R: Rep(sym.Storage.CType())}})
			l.branch(bodyNode)
		})
		l.add(&ForIndex{Idx: idx, Len: &VecCount{V: &TempRef{T: ref}}, Body: body})
	})
	slow := l.sub(generic)
	dense := &Call{Fn: "q_vec_is_dense_of", Args: []Expr{iter, &Const{R: Rep("QVector::Type"), Text: dtype}}, R: RepBool}
	l.add(&If{Cond: dense, Then: fast, Else: slow})
}

// forRange lowers `for i in range(...)`. Scalar int arguments are used as
// they are and scalar floats truncated, like the q_range builtins; any other
// value goes through q_range_arg, which checks its type at runtime.
func (l *lowerer) forRange(sym *types.Symbol, args []*ast.TreeNode, bodyNode *ast.TreeNode) {
	sig := map[int]string{1: "range(end)", 2: "range(start, end)"}[len(args)]
	if sig == "" {
		sig = "range(start, end, step)"
	}
	whats := map[int][]string{1: {"end"}, 2: {"start", "end"}}[len(args)]
	if whats == nil {
		whats = []string{"start", "end", "step"}
	}
	vals := l.operands(args[:len(whats)])
	ints := make([]Expr, len(vals))
	for i, v := range vals {
		switch v.Rep() {
		case RepInt:
			ints[i] = v
		case RepFloat:
			ints[i] = &Cast{X: v, To: RepInt}
		default:
			ints[i] = &Call{Fn: "q_range_arg", Args: []Expr{boxed(v), &Const{R: RepStr, Text: cstr(sig)}, &Const{R: RepStr, Text: cstr(whats[i])}}, R: RepInt}
		}
	}
	var start, end, step Expr
	switch len(ints) {
	case 1:
		start, end, step = &Const{R: RepInt, Text: "0"}, l.let(ints[0]), &Const{R: RepInt, Text: "1"}
	case 2:
		start, end = l.let(ints[0]), l.let(ints[1])
		step = l.let(&Select{C: &Compare{Op: "<=", L: start, X: end}, T: &Const{R: RepInt, Text: "1LL"}, F: &Const{R: RepInt, Text: "-1LL"}, R: RepInt})
	default:
		start, end = l.let(ints[0]), l.let(ints[1])
		step = l.let(&Call{Fn: "q_range_step", Args: []Expr{ints[2]}, R: RepInt})
	}
	body := l.sub(func() { l.branch(bodyNode) })
	l.add(&ForRange{Var: sym, Start: start, End: end, Step: step, Body: body})
}

// lowerWhile tests the condition before every iteration, with everything the
// condition needs.
func (l *lowerer) lowerWhile(node *ast.TreeNode) {
	if len(node.Children) < 2 {
		return
	}
	var c Expr
	pre := l.sub(func() { c = l.cond(node.Children[0]) })
	// A variable the condition declares (`while line = next():`) is read after
	// the loop too. The condition runs before the loop can end, so declaring
	// it in front of the loop never exposes an unassigned value.
	for i, s := range pre.Stmts {
		if d, ok := s.(*DeclVar); ok && !d.Unassigned {
			l.add(&DeclVar{Sym: d.Sym, Value: placeholder(d.Sym)})
			pre.Stmts[i] = &SetVar{Sym: d.Sym, Value: d.Value}
		}
	}
	body := l.sub(func() { l.branch(node.Children[1]) })
	l.add(&While{Pre: pre, Cond: c, Body: body})
}

// placeholder is the initial value of a variable declared before its first
// assignment runs.
func placeholder(sym *types.Symbol) Expr {
	switch sym.Storage {
	case ir.StorageInt:
		return &Const{R: RepInt, Text: "0"}
	case ir.StorageFloat:
		return &Const{R: RepFloat, Text: "0.0"}
	case ir.StorageBool:
		return &Const{R: RepBool, Text: "false"}
	}
	return nullValue()
}

// ---------------------------------------------------------------------------
// Literals of containers and structs

// list builds a list literal that records the element type the analyzer
// gave it; every later write is checked against it.
func (l *lowerer) list(node *ast.TreeNode) Expr {
	desc := "nullptr"
	if lt, ok := l.nodeType(node).(*types.ListType); ok {
		desc = l.elemDesc(lt.ElementType)
	}
	if len(node.Children) == 0 {
		return &Call{Fn: "qv_list", Args: []Expr{&Const{R: RepInt, Text: "8"}, &Const{R: Rep("const QElemType*"), Text: desc}}, R: RepValue}
	}
	t := l.newTemp(RepValue)
	l.add(&Let{T: t, Value: &Call{Fn: "qv_list", Args: []Expr{&Const{R: RepInt, Text: fmt.Sprint(len(node.Children))}, &Const{R: Rep("const QElemType*"), Text: desc}}, R: RepValue}})
	for _, child := range node.Children {
		v := boxed(l.expr(child))
		l.add(&SetTemp{T: t, Value: &Call{Fn: "q_push", Args: []Expr{&TempRef{T: t}, v}, R: RepValue}})
	}
	return &TempRef{T: t}
}

func vectorTypedOps(elem types.Type) (string, string) {
	switch {
	case elem == nil:
		return "", ""
	case elem.Equals(types.TypeFloat):
		return "qv_vector", "q_vec_push"
	case elem.Equals(types.TypeInt):
		return "qv_vector_i64", "q_vec_push_i64"
	case elem.Equals(types.TypeBool):
		return "qv_vector_bool", "q_vec_push_bool"
	}
	return "", ""
}

func (l *lowerer) vector(node *ast.TreeNode) Expr {
	n := &Const{R: RepInt, Text: fmt.Sprint(len(node.Children))}
	if vt, ok := l.nodeType(node).(*types.VectorType); ok && vt != nil {
		if ctor, push := vectorTypedOps(vt.ElementType); ctor != "" {
			t := l.newTemp(RepValue)
			l.add(&Let{T: t, Value: &Call{Fn: ctor, Args: []Expr{n}, R: RepValue}})
			for _, child := range node.Children {
				v := boxed(l.expr(child))
				l.add(&SetTemp{T: t, Value: &Call{Fn: push, Args: []Expr{&TempRef{T: t}, v}, R: RepValue}})
			}
			return &TempRef{T: t}
		}
	}
	t := l.newTemp(RepValue)
	l.add(&Let{T: t, Value: &Call{Fn: "qv_list", Args: []Expr{n}, R: RepValue}})
	for _, child := range node.Children {
		v := boxed(l.expr(child))
		l.add(&SetTemp{T: t, Value: &Call{Fn: "q_push", Args: []Expr{&TempRef{T: t}, v}, R: RepValue}})
	}
	return &Call{Fn: "q_to_vec", Args: []Expr{&TempRef{T: t}}, R: RepValue}
}

func (l *lowerer) dict(node *ast.TreeNode) Expr {
	desc := "nullptr"
	if dt, ok := l.nodeType(node).(*types.DictType); ok {
		desc = l.elemDesc(dt.ValueType)
	}
	t := l.newTemp(RepValue)
	l.add(&Let{T: t, Value: &Call{Fn: "qv_dict", Args: []Expr{&Const{R: Rep("const QElemType*"), Text: desc}}, R: RepValue}})
	for _, pair := range node.Children {
		if pair == nil || len(pair.Children) < 2 {
			continue
		}
		kv := l.operands(pair.Children[:2])
		l.add(&SetTemp{T: t, Value: &Call{Fn: "q_dict_set", Args: []Expr{&TempRef{T: t}, boxed(kv[0]), boxed(kv[1])}, R: RepValue}})
	}
	return &TempRef{T: t}
}

// structLiteral builds a struct value. Field values are evaluated in the
// order written, then stored by field index (declaration order), each checked
// against its field's type when the analyzer couldn't prove it.
func (l *lowerer) structLiteral(node *ast.TreeNode) Expr {
	name := node.TokenLiteral()
	st := l.an.StructTypes[name]
	if st == nil {
		l.ice("INV-STRUCT-LITERAL", node, "unknown struct type '%s'", name)
	}
	var fieldNames []string
	var valueNodes []*ast.TreeNode
	for _, pair := range node.Children {
		if pair.NodeType == ast.StructFieldNode && len(pair.Children) >= 2 {
			fieldNames = append(fieldNames, pair.TokenLiteral())
			valueNodes = append(valueNodes, pair.Children[1])
		}
	}
	vals := l.operands(valueNodes)
	provided := map[string]int{}
	for i, f := range fieldNames {
		provided[f] = i
	}
	raw := l.newTemp(RepStruct)
	l.add(&Let{T: raw, Value: &Call{Fn: "q_struct_create", Args: []Expr{&Const{R: Rep("const QStructDef*"), Text: "&_sdef_" + structCName(name)}}, R: RepStruct}})
	for i, field := range st.Fields {
		if j, ok := provided[field.Name]; ok {
			l.add(&SetField{Obj: &TempRef{T: raw}, Index: i, Value: l.checkedField(st, field, valueNodes[j], vals[j])})
		} else if field.HasDefault {
			dn, ok := field.DefaultNode.(*ast.TreeNode)
			if !ok {
				l.ice("INV-STRUCT-DEFAULT", nil, "struct '%s' field '%s' has a default but no default node", name, field.Name)
			}
			l.add(&SetField{Obj: &TempRef{T: raw}, Index: i, Value: l.checkedField(st, field, dn, l.expr(dn))})
		}
	}
	return l.let(&Call{Fn: "qv_struct", Args: []Expr{&TempRef{T: raw}}, R: RepValue, Pure: true})
}

// ---------------------------------------------------------------------------
// Members and assignment

func (l *lowerer) member(node *ast.TreeNode) Expr {
	if len(node.Children) < 2 {
		return nullValue()
	}
	obj := boxed(l.expr(node.Children[0]))
	name := node.Children[1].TokenLiteral()
	if st, ok := l.nodeType(node.Children[0]).(*types.StructType); ok {
		if _, idx := st.FieldByName(name); idx >= 0 {
			return &Call{Fn: "q_struct_get_field", Args: []Expr{obj, &Const{R: RepInt, Text: fmt.Sprint(idx)}}, R: RepValue}
		}
	}
	return &Call{Fn: "q_member_get", Args: []Expr{obj, &Const{R: RepStr, Text: cstr(name)}}, R: RepValue}
}

// assign lowers `target = value`. Operands are evaluated left to right: the
// target's object (and index), then the value.
func (l *lowerer) assign(node *ast.TreeNode) Expr {
	if len(node.Children) != 2 {
		return nullValue()
	}
	lhs, rhs := node.Children[0], node.Children[1]
	switch lhs.NodeType {
	case ast.MemberNode:
		if len(lhs.Children) >= 2 {
			ops := l.operands([]*ast.TreeNode{lhs.Children[0], rhs})
			v := l.stable(boxed(ops[1]))
			l.add(&Do{X: &Call{Fn: "q_member_set", Args: []Expr{boxed(ops[0]), &Const{R: RepStr, Text: cstr(lhs.Children[1].TokenLiteral())}, v}, R: RepValue}})
			return v
		}
	case ast.IndexNode:
		if len(lhs.Children) >= 2 {
			ops := l.operands([]*ast.TreeNode{lhs.Children[0], lhs.Children[1], rhs})
			v := l.stable(boxed(ops[2]))
			l.add(&Do{X: &Call{Fn: "q_set", Args: []Expr{boxed(ops[0]), boxed(ops[1]), v}, R: RepValue}})
			return v
		}
	case ast.IdentifierNode:
		return l.assignVar(lhs, rhs)
	}
	l.ice("INV-ASSIGN-TARGET", node, "unhandled assignment target '%s'", lhs.NodeType.String())
	return nil
}

// assignVar stores valueNode's value in the variable target names,
// declaring the C++ variable where the symbol is declared. A cell exists
// before the value is built, since a lambda assigned to the variable
// (`fib = fn(n) -> fib(n - 1)`) captures it.
func (l *lowerer) assignVar(target, valueNode *ast.TreeNode) Expr {
	sym := l.varSym(target)
	declare := sym.Decl == target && !sym.Hoisted
	if sym.Storage == ir.StorageCell {
		if declare {
			l.add(&DeclVar{Sym: sym, Value: nullValue()})
		}
		l.add(&SetVar{Sym: sym, Value: l.storeValue(sym, valueNode, l.expr(valueNode))})
		return l.readVar(sym)
	}
	v := l.storeValue(sym, valueNode, l.expr(valueNode))
	if declare {
		l.add(&DeclVar{Sym: sym, Value: v})
	} else {
		l.add(&SetVar{Sym: sym, Value: v})
	}
	return l.readVar(sym)
}
