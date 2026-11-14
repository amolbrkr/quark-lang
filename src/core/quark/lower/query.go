package lower

import (
	"fmt"

	"quark/ast"
	"quark/builtins"
	"quark/token"
	"quark/types"
)

// Table queries (specs/tables_v0_1_spec.md §4, types/analyzer_query.go). Each step
// becomes engine calls on QTable* temporaries, and each verb expression a
// typed loop over the input's column buffers: per row the expression reads
// its columns' raw values, guarded by their validity (runtime/.../table/
// query.hpp).
//
// Missing values (§5, D21, D22): an expression is computed for a row only
// when the columns it reads are present, and is otherwise missing, without
// running it (so a function it calls doesn't run for that row). `and`, `or`,
// `!` and `x if c else y` are the exception: they follow SQL's three-valued
// logic, so `true or <missing>` is true and `false and <missing>` is false.
// Their parts are lowered first and stand for their values (and presence)
// inside a larger expression.

// rowLoop is the row a verb expression is lowered for: the loop's row index,
// the table it reads, its row type, and the columns bound for the loop.
type rowLoop struct {
	idx  *Temp
	row  *types.StructType
	cols map[int]*Temp // column index -> const QVector& temp
}

const (
	repRowSet   Rep = "quark::tbl::RowSet"
	repColOut   Rep = "quark::tbl::ColOut"
	repGrouping Rep = "quark::tbl::Grouping"
)

func intConst(n int) Expr { return &Const{R: RepInt, Text: fmt.Sprint(n)} }

// query lowers a QueryNode to the result table, as a QValue.
func (l *lowerer) query(node *ast.TreeNode) Expr {
	q := l.an.Queries[node]
	if q == nil {
		l.ice("INV-QUERY", node, "query was not analyzed")
	}
	l.include(builtins.TableHeader)
	src := l.expr(node.Children[0])
	cur := l.let(&Call{Fn: "q_tbl_of", Args: []Expr{boxed(src)}, R: RepTable})
	for _, st := range q.Steps {
		l.setLoc(st.Node)
		switch st.Verb {
		case "filter":
			cur = l.queryFilter(cur, st)
		case "mutate":
			cur = l.queryMutate(cur, st)
		case "sort":
			keys := make([]Expr, 0, 2*len(st.Keys))
			for _, k := range st.Keys {
				desc := 0
				if k.Desc {
					desc = 1
				}
				keys = append(keys, intConst(k.Col), intConst(desc))
			}
			cur = l.let(&Call{Fn: "q_tbl_sort", Args: []Expr{cur, &InitList{Type: "std::initializer_list<int>", Elems: keys}}, R: RepTable})
		case "group_by":
			// Grouping happens in the summarize that follows.
		case "summarize":
			cur = l.querySummarize(cur, st)
		case "head", "tail":
			var n Expr = intConst(6)
			if st.Count != nil {
				n = toNative(l.expr(st.Count), RepInt)
			}
			cur = l.let(&Call{Fn: "q_tbl_" + st.Verb, Args: []Expr{cur, n}, R: RepTable})
		default:
			l.ice("INV-QUERY", st.Node, "unknown query step '%s'", st.Verb)
		}
	}
	return &Box{X: cur}
}

// include adds a runtime header quark.hpp doesn't include ("" for none) to
// the program, once.
func (l *lowerer) include(header string) {
	if header == "" || l.included[header] {
		return
	}
	l.included[header] = true
	l.prog.Includes = append(l.prog.Includes, header)
}

// forRows binds the columns the expressions read of table cur (rows of type
// row) and lowers body in a loop over its rows.
func (l *lowerer) forRows(cur Expr, row *types.StructType, exprs []*ast.TreeNode, body func()) {
	n := l.let(&Call{Fn: "q_tbl_rows", Args: []Expr{cur}, R: RepInt})
	rl := &rowLoop{row: row, cols: map[int]*Temp{}}
	for _, e := range exprs {
		for _, c := range l.columnReads(e, nil) {
			if rl.cols[c] != nil {
				continue
			}
			t := l.newTemp(RepVecRef)
			l.add(&Let{T: t, Value: &Call{Fn: "q_tbl_col", Args: []Expr{cur, intConst(c)}, R: RepVecRef}})
			rl.cols[c] = t
		}
	}
	rl.idx = l.newTemp(RepInt)
	saved := l.row
	l.row = rl
	loop := l.sub(body)
	l.row = saved
	l.add(&ForIndex{Idx: rl.idx, Len: n, Body: loop})
}

// columnReads lists, in order and once each, the columns node reads,
// skipping the parts in skip (lowered separately) and functions written in
// it (which can't read the row).
func (l *lowerer) columnReads(node *ast.TreeNode, skip map[*ast.TreeNode]bool) []int {
	var out []int
	seen := map[int]bool{}
	var walk func(n *ast.TreeNode)
	walk = func(n *ast.TreeNode) {
		if n == nil || skip[n] || n.NodeType == ast.LambdaNode {
			return
		}
		if c, ok := l.an.ColumnRefs[n]; ok {
			if !seen[c] {
				seen[c] = true
				out = append(out, c)
			}
			return
		}
		for _, ch := range n.Children {
			walk(ch)
		}
	}
	walk(node)
	return out
}

// isRowLogic reports whether node is a part of a verb expression that
// follows three-valued logic: and, or, !, or a ternary.
func isRowLogic(node *ast.TreeNode) bool {
	switch node.Op() {
	case token.AND, token.OR:
		return node.NodeType == ast.BinaryNode
	case token.BANG:
		return node.NodeType == ast.UnaryNode
	}
	return node.NodeType == ast.TernaryNode
}

// logicParts are the outermost three-valued parts below node.
func logicParts(node *ast.TreeNode) []*ast.TreeNode {
	var out []*ast.TreeNode
	var walk func(n *ast.TreeNode)
	walk = func(n *ast.TreeNode) {
		for _, ch := range n.Children {
			if ch == nil || ch.NodeType == ast.LambdaNode {
				continue
			}
			if isRowLogic(ch) {
				out = append(out, ch)
				continue
			}
			walk(ch)
		}
	}
	walk(node)
	return out
}

// rowRep is how a verb expression's value of type t is held: a raw scalar,
// or a QValue (str).
func rowRep(t types.Type) Rep {
	if r := ScalarRepOf(t); r != "" {
		return r
	}
	return RepValue
}

func zeroOf(r Rep) Expr {
	switch r {
	case RepInt:
		return intConst(0)
	case RepFloat:
		return &Const{R: RepFloat, Text: "0.0"}
	case RepBool:
		return &Const{R: RepBool, Text: "false"}
	}
	return nullValue()
}

var trueExpr = &Const{R: RepBool, Text: "true"}
var falseExpr = &Const{R: RepBool, Text: "false"}

// asRowRep converts v, node's value, to rep r: raw scalars as they are, a
// QValue the analyzer typed exactly unboxed.
func (l *lowerer) asRowRep(node *ast.TreeNode, v Expr, r Rep) Expr {
	switch {
	case v.Rep() == r:
		return v
	case r == RepValue:
		return boxed(v)
	case r == RepFloat && v.Rep() == RepInt:
		return &Cast{X: v, To: RepFloat}
	case v.Rep() == RepValue:
		return &Unbox{X: v, To: r, Proof: ProofType, Type: l.nodeType(node)}
	}
	l.ice("INV-QUERY-REP", node, "verb expression of rep %s, want %s", v.Rep(), r)
	return nil
}

// rowValue lowers verb expression node for the current row: its value (in
// rowRep of its type) and whether it is present (nil: always).
func (l *lowerer) rowValue(node *ast.TreeNode) (Expr, Expr) {
	switch {
	case node.Op() == token.AND && node.NodeType == ast.BinaryNode:
		return l.rowLogic(node, true)
	case node.Op() == token.OR && node.NodeType == ast.BinaryNode:
		return l.rowLogic(node, false)
	case node.Op() == token.BANG && node.NodeType == ast.UnaryNode:
		v, p := l.rowValue(node.Children[0])
		return &Not{X: l.asRowRep(node.Children[0], v, RepBool)}, p
	case node.NodeType == ast.TernaryNode:
		return l.rowTernary(node)
	}
	return l.rowStrict(node)
}

// rowStrict lowers a part of a verb expression that is missing when any
// column it reads (or any three-valued part in it) is: it is computed only
// when all of them are present.
func (l *lowerer) rowStrict(node *ast.TreeNode) (Expr, Expr) {
	parts := logicParts(node)
	skip := map[*ast.TreeNode]bool{}
	var pres []Expr
	for _, part := range parts {
		v, p := l.rowValue(part)
		l.rowSubst[part] = l.stable(v)
		skip[part] = true
		if p != nil {
			pres = append(pres, l.stable(p))
		}
	}
	defer func() {
		for _, part := range parts {
			delete(l.rowSubst, part)
		}
	}()
	for _, c := range l.columnReads(node, skip) {
		pres = append(pres, &Call{Fn: "q_tbl_present", Args: []Expr{&TempRef{T: l.row.cols[c]}, &TempRef{T: l.row.idx}}, R: RepBool})
	}
	r := rowRep(l.nodeType(node))
	if len(pres) == 0 {
		return l.asRowRep(node, l.expr(node), r), nil
	}
	var all Expr = pres[0]
	for _, p := range pres[1:] {
		all = &AndAlso{L: all, X: p}
	}
	present := l.let(all)
	out := l.newTemp(r)
	l.add(&Let{T: out, Value: zeroOf(r)})
	then := l.sub(func() {
		l.add(&SetTemp{T: out, Value: l.asRowRep(node, l.expr(node), r)})
	})
	l.add(&If{Cond: present, Then: then})
	return &TempRef{T: out}, present
}

// rowLogic lowers `a and b` / `a or b` with three-valued logic (D22): the
// decisive value (false for and, true for or) of either operand decides
// alone, both present otherwise give the other value, and anything else is
// missing. b is evaluated only when a doesn't decide.
func (l *lowerer) rowLogic(node *ast.TreeNode, isAnd bool) (Expr, Expr) {
	decisive := func(v Expr) Expr {
		if isAnd {
			return &Not{X: v}
		}
		return v
	}
	given := func(p, x Expr) Expr {
		if p == nil {
			return x
		}
		return &AndAlso{L: p, X: x}
	}
	dv, other := trueExpr, falseExpr
	if isAnd {
		dv, other = falseExpr, trueExpr
	}
	va, pa := l.rowValue(node.Children[0])
	va = l.stable(l.asRowRep(node.Children[0], va, RepBool))
	out := l.newTemp(RepBool)
	l.add(&Let{T: out, Value: dv})
	present := l.newTemp(RepBool)
	l.add(&Let{T: present, Value: trueExpr})
	rest := l.sub(func() {
		vb, pb := l.rowValue(node.Children[1])
		vb = l.stable(l.asRowRep(node.Children[1], vb, RepBool))
		var both Expr
		switch {
		case pa == nil && pb == nil:
			both = trueExpr
		case pa == nil:
			both = pb
		case pb == nil:
			both = pa
		default:
			both = &AndAlso{L: pa, X: pb}
		}
		l.add(&If{
			Cond: given(pb, decisive(vb)),
			Then: &Block{},
			Else: &Block{Stmts: []Stmt{&If{
				Cond: both,
				Then: &Block{Stmts: []Stmt{&SetTemp{T: out, Value: other}}},
				Else: &Block{Stmts: []Stmt{&SetTemp{T: present, Value: falseExpr}}},
			}}},
		})
	})
	l.add(&If{Cond: given(pa, decisive(va)), Then: &Block{}, Else: rest})
	return &TempRef{T: out}, &TempRef{T: present}
}

// rowTernary lowers `x if c else y`: missing when c is, else the chosen
// branch.
func (l *lowerer) rowTernary(node *ast.TreeNode) (Expr, Expr) {
	cond, thenNode, elseNode := node.Children[0], node.Children[1], node.Children[2]
	r := rowRep(l.nodeType(node))
	vc, pc := l.rowValue(cond)
	vc = l.stable(l.asRowRep(cond, vc, RepBool))
	out := l.newTemp(r)
	l.add(&Let{T: out, Value: zeroOf(r)})
	present := l.newTemp(RepBool)
	l.add(&Let{T: present, Value: falseExpr})
	branch := func(n *ast.TreeNode) *Block {
		return l.sub(func() {
			v, p := l.rowValue(n)
			l.add(&SetTemp{T: out, Value: l.asRowRep(n, v, r)})
			if p == nil {
				p = trueExpr
			}
			l.add(&SetTemp{T: present, Value: p})
		})
	}
	choose := &If{Cond: vc, Then: branch(thenNode), Else: branch(elseNode)}
	if pc == nil {
		l.add(choose)
	} else {
		l.add(&If{Cond: pc, Then: &Block{Stmts: []Stmt{choose}}})
	}
	return &TempRef{T: out}, &TempRef{T: present}
}

// columnRead is `e.col` in the current row: the raw value (a QValue for a
// str column), which rowStrict reads only when it is present.
func (l *lowerer) columnRead(node *ast.TreeNode, col int) Expr {
	rl := l.row
	if rl == nil || rl.cols[col] == nil {
		l.ice("INV-QUERY-COLUMN", node, "column read outside its row loop")
	}
	ref, idx := &TempRef{T: rl.cols[col]}, &TempRef{T: rl.idx}
	t := rl.row.Fields[col].Type
	if r := ScalarRepOf(t); r != "" {
		return &VecElem{V: ref, Idx: idx, R: r}
	}
	return &Call{Fn: "q_tbl_str", Args: []Expr{ref, idx}, R: RepValue}
}

func (l *lowerer) queryFilter(cur Expr, st *types.QueryStep) Expr {
	rows := l.newTemp(repRowSet)
	l.add(&Let{T: rows, Value: &Call{Fn: "q_rows_new", Args: []Expr{&Call{Fn: "q_tbl_rows", Args: []Expr{cur}, R: RepInt}}, R: repRowSet}})
	l.forRows(cur, st.In, []*ast.TreeNode{st.Pred}, func() {
		v, p := l.rowValue(st.Pred)
		var keep Expr = l.asRowRep(st.Pred, v, RepBool)
		if p != nil {
			keep = &AndAlso{L: p, X: keep}
		}
		push := &Do{X: &Call{Fn: "q_rows_push", Args: []Expr{&TempRef{T: rows}, &TempRef{T: l.row.idx}}, R: Rep("void")}}
		l.add(&If{Cond: keep, Then: &Block{Stmts: []Stmt{push}}})
	})
	return l.let(&Call{Fn: "q_tbl_take", Args: []Expr{cur, &TempRef{T: rows}}, R: RepTable})
}

// pushFns are the ColOut pushes by column type.
var pushFns = map[Rep]string{RepInt: "q_col_push_int", RepFloat: "q_col_push_float", RepBool: "q_col_push_bool", RepValue: "q_col_push_str"}

func (l *lowerer) queryMutate(cur Expr, st *types.QueryStep) Expr {
	for _, c := range st.Cols {
		r := rowRep(c.Type)
		b := l.newTemp(repColOut)
		n := &Call{Fn: "q_tbl_rows", Args: []Expr{cur}, R: RepInt}
		l.add(&Let{T: b, Value: &Call{Fn: "q_col_new", Args: []Expr{intConst(ColumnDtype(c.Type)), n}, R: repColOut}})
		l.forRows(cur, c.Reads, []*ast.TreeNode{c.Value}, func() {
			v, p := l.rowValue(c.Value)
			push := &Do{X: &Call{Fn: pushFns[r], Args: []Expr{&TempRef{T: b}, l.asRowRep(c.Value, v, r)}, R: Rep("void")}}
			if p == nil {
				l.add(push)
				return
			}
			null := &Do{X: &Call{Fn: "q_col_push_null", Args: []Expr{&TempRef{T: b}}, R: Rep("void")}}
			l.add(&If{Cond: p, Then: &Block{Stmts: []Stmt{push}}, Else: &Block{Stmts: []Stmt{null}}})
		})
		vec := l.let(&Call{Fn: "q_col_finish", Args: []Expr{&TempRef{T: b}}, R: RepVector})
		def := &Const{R: Rep("const QStructDef*"), Text: "&_sdef_" + structCName(c.After.Name)}
		cur = l.let(&Call{Fn: "q_tbl_put", Args: []Expr{cur, def, intConst(c.Index), vec}, R: RepTable})
	}
	return cur
}

func (l *lowerer) querySummarize(cur Expr, st *types.QueryStep) Expr {
	var g Expr
	if st.Group == nil {
		g = l.let(&Call{Fn: "q_tbl_whole", Args: []Expr{cur}, R: repGrouping})
	} else {
		keys := make([]Expr, len(st.Group))
		for i, k := range st.Group {
			keys[i] = intConst(k)
		}
		g = l.let(&Call{Fn: "q_tbl_group", Args: []Expr{cur, &InitList{Type: "std::initializer_list<int>", Elems: keys}}, R: repGrouping})
	}
	var cols []Expr
	for _, k := range st.Group {
		cols = append(cols, l.let(&Call{Fn: "q_tbl_group_key", Args: []Expr{cur, g, intConst(k)}, R: RepVector}))
	}
	for _, a := range st.Aggs {
		var call *Call
		switch {
		case a.Fn == "count" && a.Col < 0:
			call = &Call{Fn: "q_tbl_agg_count", Args: []Expr{g}, R: RepVector}
		case a.Fn == "count":
			call = &Call{Fn: "q_tbl_agg_count_values", Args: []Expr{cur, intConst(a.Col), g}, R: RepVector}
		default:
			call = &Call{Fn: "q_tbl_agg_" + a.Fn, Args: []Expr{cur, intConst(a.Col), g}, R: RepVector}
		}
		cols = append(cols, l.let(call))
	}
	def := &Const{R: Rep("const QStructDef*"), Text: "&_sdef_" + structCName(st.Out.Name)}
	return l.let(&Call{Fn: "q_tbl_make", Args: []Expr{def, &InitList{Type: "std::initializer_list<QVector*>", Elems: cols}}, R: RepTable})
}
