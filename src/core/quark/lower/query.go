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
	// key and saved are set when the loop's expressions draw random
	// numbers (specs/tables_v0_3_spec.md §5): the loop's key, and the draw
	// context to restore after the loop (endRow).
	key   *Temp
	saved *Temp
}

const (
	repRowSet   Rep = "quark::tbl::RowSet"
	repColOut   Rep = "quark::tbl::ColOut"
	repGrouping Rep = "quark::tbl::Grouping"
	repLookup   Rep = "quark::tbl::LookupSet"
	repRandKey  Rep = "uint64_t"
	repRowCtx   Rep = "QRowCtx"
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
		case "select", "deselect":
			cur = l.queryProject(cur, st, st.Keep)
		case "rename":
			cols := make([]int, len(st.In.Fields))
			for i := range cols {
				cols[i] = i
			}
			cur = l.queryProject(cur, st, cols)
		case "distinct":
			cur = l.queryDistinct(cur, st)
		case "drop_na":
			cols := make([]Expr, len(st.Keep))
			for i, c := range st.Keep {
				cols[i] = intConst(c)
			}
			cur = l.let(&Call{Fn: "q_tbl_drop_na", Args: []Expr{cur, &InitList{Type: "std::initializer_list<int>", Elems: cols}}, R: RepTable})
		case "count":
			cur = l.queryCount(cur, st)
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

// openRow binds the columns the expressions read of table cur. dense is true
// when every one of those columns has no missing values (nil when the
// expressions read no column, which is dense too). The caller runs one loop
// or both.
func (l *lowerer) openRow(cur Expr, row *types.StructType, exprs []*ast.TreeNode) (*rowLoop, Expr, Expr) {
	n := l.let(&Call{Fn: "q_tbl_rows", Args: []Expr{cur}, R: RepInt})
	rl := &rowLoop{row: row, cols: map[int]*Temp{}}
	var checks []Expr
	for _, e := range exprs {
		for _, c := range l.columnReads(e, nil) {
			if rl.cols[c] != nil {
				continue
			}
			t := l.newTemp(RepVecRef)
			l.add(&Let{T: t, Value: &Call{Fn: "q_tbl_col", Args: []Expr{cur, intConst(c)}, R: RepVecRef}})
			rl.cols[c] = t
			checks = append(checks, &Call{Fn: "q_vec_dense", Args: []Expr{&TempRef{T: t}}, R: RepBool})
		}
	}
	rl.idx = l.newTemp(RepInt)
	l.drawContext(rl, exprs)
	if len(checks) == 0 {
		return rl, n, nil
	}
	all := checks[0]
	for _, c := range checks[1:] {
		all = &AndAlso{L: all, X: c}
	}
	return rl, n, l.let(all)
}

// onceSlot is one call a row loop evaluates the first time a row needs it.
type onceSlot struct {
	ready *Temp
	value *Temp
}

// onceSlots are the once-calls of the row loop being lowered.
type onceSlots struct {
	list   []*onceSlot
	byNode map[*ast.TreeNode]*onceSlot
}

func (o *onceSlots) slot(l *lowerer, node *ast.TreeNode, rep Rep) *onceSlot {
	if s := o.byNode[node]; s != nil {
		return s
	}
	s := &onceSlot{ready: l.newTemp(RepBool), value: l.newTemp(rep)}
	o.byNode[node] = s
	o.list = append(o.list, s)
	return s
}

// onceZero is the placeholder a once-slot holds until a row needs the call.
// A pointer result keeps the pointer the call returned; a row-safe function's
// pointer refers to memory that outlives the call (a broken rowsafe promise
// can dangle, like any other broken native contract).
func onceZero(r Rep) Expr {
	switch r {
	case RepInt, RepFloat, RepBool, RepDate:
		return zeroOf(r)
	case RepValue:
		return nullValue()
	case repLookup:
		return &Const{R: r, Text: "quark::tbl::LookupSet{}"}
	default:
		return &Const{R: r, Text: "nullptr"}
	}
}

// drawContext gives rl a draw context when exprs draw random numbers
// (specs/tables_v0_3_spec.md §5): the loop's key, taken once before the
// loop (one number for the dense and the general loop, of which one runs),
// and the context the loop replaces.
func (l *lowerer) drawContext(rl *rowLoop, exprs []*ast.TreeNode) {
	if !l.drawsIn(exprs) {
		return
	}
	rl.key = l.newTemp(repRandKey)
	l.add(&Let{T: rl.key, Value: &Call{Fn: "q_rand_loop_key", R: repRandKey}})
	rl.saved = l.newTemp(repRowCtx)
	l.add(&Let{T: rl.saved, Value: &Call{Fn: "q_row_ctx_save", R: repRowCtx}})
}

// endRow restores the draw context after rl's loops.
func (l *lowerer) endRow(rl *rowLoop) {
	if rl.saved != nil {
		l.add(&Do{X: &Call{Fn: "q_row_ctx_restore", Args: []Expr{&TempRef{T: rl.saved}}, R: Rep("void")}})
	}
}

// drawsIn reports whether evaluating exprs for a row can draw a random
// number: a call the analyzer marked Draws. An aggregate's argument runs
// in its own loop, and a function written in an expression isn't called.
func (l *lowerer) drawsIn(exprs []*ast.TreeNode) bool {
	var walk func(n *ast.TreeNode) bool
	walk = func(n *ast.TreeNode) bool {
		if n == nil || n.NodeType == ast.LambdaNode || l.an.AggUses[n] != nil {
			return false
		}
		if n.NodeType == ast.FunctionCallNode {
			if p := l.an.CallPlans[n]; p != nil && p.Draws {
				return true
			}
		}
		for _, ch := range n.Children {
			if walk(ch) {
				return true
			}
		}
		return false
	}
	for _, e := range exprs {
		if walk(e) {
			return true
		}
	}
	return false
}

// rowFor lowers body as a loop over the rows openRow bound. With a draw
// context, each row starts by setting it.
func (l *lowerer) rowFor(rl *rowLoop, n Expr, body func()) {
	saved, savedOnce := l.row, l.once
	l.row = rl
	l.once = &onceSlots{byNode: map[*ast.TreeNode]*onceSlot{}}
	loop := l.sub(func() {
		if rl.key != nil {
			l.add(&Do{X: &Call{Fn: "q_row_ctx_set", Args: []Expr{&TempRef{T: rl.key}, &TempRef{T: rl.idx}}, R: Rep("void")}})
		}
		body()
	})
	slots := l.once
	l.row, l.once = saved, savedOnce
	for _, s := range slots.list {
		l.add(&Let{T: s.ready, Value: falseExpr})
		l.add(&Let{T: s.value, Value: onceZero(s.value.R)})
	}
	l.add(&ForIndex{Idx: rl.idx, Len: n, Body: loop})
}

// denseOrSlow runs fast when dense is true and slow otherwise. dense nil
// means the expressions read no column, so only fast runs.
func (l *lowerer) denseOrSlow(dense Expr, fast, slow func()) {
	if dense == nil {
		fast()
		return
	}
	l.add(&If{Cond: dense, Then: l.sub(fast), Else: l.sub(slow)})
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
		if l.an.AggUses[n] != nil {
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
// doesn't follow the strict rule: and, or, !, a ternary (three-valued
// logic), and `x == null`, `x != null` and `x.fillna(v)` (they look at the
// missing value itself, tables v0.3 D46, D47).
func (l *lowerer) isRowLogic(node *ast.TreeNode) bool {
	if l.an.RowMissing[node] != nil {
		return true
	}
	switch node.Op() {
	case token.AND, token.OR:
		return node.NodeType == ast.BinaryNode
	case token.BANG:
		return node.NodeType == ast.UnaryNode
	}
	return node.NodeType == ast.TernaryNode
}

// logicParts are the outermost non-strict parts below node. An aggregate's
// argument is not a part: the aggregate is computed before the row loop,
// over its own loop (evalAgg), and the row reads its value.
func (l *lowerer) logicParts(node *ast.TreeNode) []*ast.TreeNode {
	var out []*ast.TreeNode
	var walk func(n *ast.TreeNode)
	walk = func(n *ast.TreeNode) {
		if l.an.AggUses[n] != nil {
			return
		}
		for _, ch := range n.Children {
			if ch == nil || ch.NodeType == ast.LambdaNode {
				continue
			}
			if l.isRowLogic(ch) {
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
	case RepDate:
		return &Const{R: RepDate, Text: "0"}
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
	if rm := l.an.RowMissing[node]; rm != nil {
		if rm.Fill != nil {
			return l.rowFillna(node, rm)
		}
		return l.rowNullTest(rm), nil
	}
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
	parts := l.logicParts(node)
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
	for _, u := range l.aggUsesIn(node, skip) {
		if u.CanMiss {
			pres = append(pres, l.aggPresent(u))
		}
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

// rowNullTest lowers `x == null` / `x != null` (D46): whether x is missing,
// never missing itself. x is computed as anywhere else (when the values it
// reads are present), so an error computing it is reported.
func (l *lowerer) rowNullTest(rm *types.RowMissing) Expr {
	v, p := l.rowValue(rm.X)
	if p == nil {
		l.stable(v)
		if rm.Not {
			return trueExpr
		}
		return falseExpr
	}
	if rm.Not {
		return p
	}
	return &Not{X: p}
}

// rowFillna lowers `x.fillna(v)` (D47): x where it is present, else v, which
// is computed only for those rows; missing only where both are.
func (l *lowerer) rowFillna(node *ast.TreeNode, rm *types.RowMissing) (Expr, Expr) {
	r := rowRep(l.nodeType(node))
	vx, px := l.rowValue(rm.X)
	if px == nil {
		return l.asRowRep(rm.X, vx, r), nil
	}
	out := l.newTemp(r)
	l.add(&Let{T: out, Value: zeroOf(r)})
	present := l.newTemp(RepBool)
	l.add(&Let{T: present, Value: trueExpr})
	then := &Block{Stmts: []Stmt{&SetTemp{T: out, Value: l.asRowRep(rm.X, vx, r)}}}
	els := l.sub(func() {
		vf, pf := l.rowValue(rm.Fill)
		l.add(&SetTemp{T: out, Value: l.asRowRep(rm.Fill, vf, r)})
		if pf != nil {
			l.add(&SetTemp{T: present, Value: pf})
		}
	})
	l.add(&If{Cond: px, Then: then, Else: els})
	return &TempRef{T: out}, &TempRef{T: present}
}

// rowMissingDense lowers `x == null` or `x.fillna(v)` in a row loop whose
// values are all present (the dense loop): x itself, or whether it is
// missing (never), after computing it.
func (l *lowerer) rowMissingDense(node *ast.TreeNode, rm *types.RowMissing) Expr {
	v := l.expr(rm.X)
	if rm.Fill != nil {
		return l.asRowRep(rm.X, v, rowRep(l.nodeType(node)))
	}
	l.stable(v)
	if rm.Not {
		return trueExpr
	}
	return falseExpr
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
	// A str cell is a view of the column buffer. It becomes a Quark string
	// only when something boxes it (a new column, or a call).
	return &Call{Fn: "q_tbl_str_view", Args: []Expr{ref, idx}, R: RepStrView}
}

func (l *lowerer) queryFilter(cur Expr, st *types.QueryStep) Expr {
	if len(st.Aggs) > 0 {
		l.bindGrouping(cur, st.GroupKeys, st.In)
		for _, a := range st.Aggs {
			l.bindAgg(a, l.evalAgg(cur, a, st.In))
		}
	}
	rows := l.newTemp(repRowSet)
	l.add(&Let{T: rows, Value: &Call{Fn: "q_rows_new", Args: []Expr{&Call{Fn: "q_tbl_rows", Args: []Expr{cur}, R: RepInt}}, R: repRowSet}})
	rl, n, dense := l.openRow(cur, st.In, []*ast.TreeNode{st.Pred})
	dense = l.andDense(dense, l.aggDense(st.Pred))
	l.denseOrSlow(dense, func() {
		// No missing values: the predicate is an ordinary bool, and the
		// selected indexes are gathered without a branch on it.
		l.rowFor(rl, n, func() {
			keep := l.asRowRep(st.Pred, l.expr(st.Pred), RepBool)
			l.add(&Do{X: &Call{Fn: "q_rows_keep", Args: []Expr{&TempRef{T: rows}, &TempRef{T: rl.idx}, keep}, R: Rep("void")}})
		})
	}, func() {
		l.rowFor(rl, n, func() {
			v, p := l.rowValue(st.Pred)
			var keep Expr = l.asRowRep(st.Pred, v, RepBool)
			if p != nil {
				keep = &AndAlso{L: p, X: keep}
			}
			push := &Do{X: &Call{Fn: "q_rows_push", Args: []Expr{&TempRef{T: rows}, &TempRef{T: l.row.idx}}, R: Rep("void")}}
			l.add(&If{Cond: keep, Then: &Block{Stmts: []Stmt{push}}})
		})
	})
	l.endRow(rl)
	l.agg = nil
	return l.let(&Call{Fn: "q_tbl_take", Args: []Expr{cur, &TempRef{T: rows}}, R: RepTable})
}

// pushFns are the ColOut pushes by column type.
var pushFns = map[Rep]string{RepInt: "q_col_push_int", RepFloat: "q_col_push_float", RepBool: "q_col_push_bool", RepDate: "q_col_push_date", RepValue: "q_col_push_str"}

func (l *lowerer) queryMutate(cur Expr, st *types.QueryStep) Expr {
	needsAgg := false
	for _, c := range st.Cols {
		if len(c.Aggs) > 0 {
			needsAgg = true
			break
		}
	}
	if needsAgg {
		l.bindGrouping(cur, st.GroupKeys, st.In)
	}
	for _, c := range st.Cols {
		for _, a := range c.Aggs {
			if l.agg.vecs[a] != nil {
				continue
			}
			l.bindAgg(a, l.evalAgg(cur, a, c.Reads))
		}
		vec := l.buildColumn(cur, c.Reads, c.Value, c.Type)
		def := &Const{R: Rep("const QStructDef*"), Text: "&_sdef_" + structCName(c.After.Name)}
		cur = l.let(&Call{Fn: "q_tbl_put", Args: []Expr{cur, def, intConst(c.Index), vec}, R: RepTable})
	}
	l.agg = nil
	return cur
}

// queryProject is select, deselect and rename: a new row type over columns
// of the input, shared, not copied. cols are indexes of st.In, in the
// output's order.
func (l *lowerer) queryProject(cur Expr, st *types.QueryStep, cols []int) Expr {
	elems := make([]Expr, len(cols))
	for i, c := range cols {
		elems[i] = intConst(c)
	}
	def := &Const{R: Rep("const QStructDef*"), Text: "&_sdef_" + structCName(st.Out.Name)}
	return l.let(&Call{Fn: "q_tbl_project", Args: []Expr{cur, def, &InitList{Type: "std::initializer_list<int>", Elems: elems}}, R: RepTable})
}

func (l *lowerer) queryDistinct(cur Expr, st *types.QueryStep) Expr {
	elems := make([]Expr, len(st.Keys))
	for i, k := range st.Keys {
		elems[i] = intConst(k.Col)
	}
	return l.let(&Call{Fn: "q_tbl_distinct", Args: []Expr{cur, &InitList{Type: "std::initializer_list<int>", Elems: elems}}, R: RepTable})
}

func (l *lowerer) querySummarize(cur Expr, st *types.QueryStep) Expr {
	l.bindGrouping(cur, st.GroupKeys, st.In)
	for _, a := range st.Aggs {
		// Read per group (perGroup), so nothing is hoisted.
		l.agg.vecs[a] = l.evalAgg(cur, a, st.In)
	}
	cols := append([]Expr{}, l.agg.keyVecs...)
	if len(st.SumCols) > 0 {
		n := l.let(&Call{Fn: "q_tbl_ngroups", Args: []Expr{l.agg.group}, R: RepInt})
		keyRefs := map[int]*Temp{}
		for i, k := range st.GroupKeys {
			if k.Col < 0 || keyRefs[k.Col] != nil {
				continue
			}
			ref := l.newTemp(RepVecRef)
			l.add(&Let{T: ref, Value: &Call{Fn: "q_vec_deref", Args: []Expr{l.agg.keyVecs[i]}, R: RepVecRef}})
			keyRefs[k.Col] = ref
		}
		l.agg.perGroup = true
		for _, sc := range st.SumCols {
			cols = append(cols, l.buildGroupColumn(n, keyRefs, st.In, sc.Expr, sc.Type))
		}
		l.agg.perGroup = false
	}
	def := &Const{R: Rep("const QStructDef*"), Text: "&_sdef_" + structCName(st.Out.Name)}
	out := l.let(&Call{Fn: "q_tbl_make", Args: []Expr{def, &InitList{Type: "std::initializer_list<QVector*>", Elems: cols}}, R: RepTable})
	l.agg = nil
	return out
}

// queryCount is the count step (tables v0.3 D50): the group keys and the
// row count of each group.
func (l *lowerer) queryCount(cur Expr, st *types.QueryStep) Expr {
	l.bindGrouping(cur, st.GroupKeys, st.In)
	cols := append([]Expr{}, l.agg.keyVecs...)
	cols = append(cols, l.let(&Call{Fn: "q_tbl_agg_count", Args: []Expr{l.agg.group}, R: RepVector}))
	def := &Const{R: Rep("const QStructDef*"), Text: "&_sdef_" + structCName(st.Out.Name)}
	out := l.let(&Call{Fn: "q_tbl_make", Args: []Expr{def, &InitList{Type: "std::initializer_list<QVector*>", Elems: cols}}, R: RepTable})
	l.agg = nil
	return out
}

// aggBind is the grouping and the aggregate values of the step being lowered.
type aggBind struct {
	group    Expr
	whole    bool
	perGroup bool
	vecs     map[*types.AggUse]Expr
	keyVecs  []Expr
	// wholeVals are the value and presence of each aggregate of a
	// whole-table step, read once before the row loop (bindAgg).
	wholeVals map[*types.AggUse]wholeAgg
}

// wholeAgg is a whole-table aggregate held in locals: present is nil when
// the aggregate can't be missing.
type wholeAgg struct {
	value   Expr
	present Expr
}

// bindGrouping computes the step's groups: the whole table, or the group_by
// keys (a computed key is evaluated into a column first).
func (l *lowerer) bindGrouping(cur Expr, keys []types.GroupKey, row *types.StructType) {
	l.agg = &aggBind{vecs: map[*types.AggUse]Expr{}, keyVecs: make([]Expr, len(keys)), wholeVals: map[*types.AggUse]wholeAgg{}}
	if len(keys) == 0 {
		l.agg.group = l.let(&Call{Fn: "q_tbl_whole", Args: []Expr{cur}, R: repGrouping})
		l.agg.whole = true
		return
	}
	ptrs := make([]Expr, len(keys))
	full := make([]Expr, len(keys))
	for i, k := range keys {
		if k.Expr != nil {
			full[i] = l.buildColumn(cur, row, k.Expr, k.Type)
			ptrs[i] = full[i]
		} else {
			ptrs[i] = &Call{Fn: "q_tbl_col_ptr", Args: []Expr{cur, intConst(k.Col)}, R: Rep("const QVector*")}
		}
	}
	n := &Call{Fn: "q_tbl_rows", Args: []Expr{cur}, R: RepInt}
	g := l.let(&Call{Fn: "q_tbl_group_vecs", Args: []Expr{n, &InitList{Type: "std::initializer_list<const QVector*>", Elems: ptrs}}, R: repGrouping})
	l.agg.group = g
	for i, k := range keys {
		if k.Expr != nil {
			l.agg.keyVecs[i] = l.let(&Call{Fn: "q_tbl_key_col", Args: []Expr{full[i], g}, R: RepVector})
		} else {
			l.agg.keyVecs[i] = l.let(&Call{Fn: "q_tbl_group_key", Args: []Expr{cur, g, intConst(k.Col)}, R: RepVector})
		}
	}
}

// evalAgg computes one aggregate into a vector of one value per group.
// A column argument uses the column in place; an expression is evaluated
// into a temporary column first, then the same kernel.
func (l *lowerer) evalAgg(cur Expr, a *types.AggUse, row *types.StructType) Expr {
	g := l.agg.group
	switch {
	case a.Fn == "count" && a.Arg == nil:
		return l.let(&Call{Fn: "q_tbl_agg_count", Args: []Expr{g}, R: RepVector})
	case a.Col >= 0 && a.Fn == "count":
		return l.let(&Call{Fn: "q_tbl_agg_count_values", Args: []Expr{cur, intConst(a.Col), g}, R: RepVector})
	case a.Col >= 0:
		return l.let(&Call{Fn: "q_tbl_agg_" + a.Fn, Args: []Expr{cur, intConst(a.Col), g}, R: RepVector})
	}
	vec := l.buildColumn(cur, row, a.Arg, a.ArgType)
	fn := "q_tbl_agg_" + a.Fn + "_vec"
	if a.Fn == "count" {
		fn = "q_tbl_agg_count_values_vec"
	}
	return l.let(&Call{Fn: fn, Args: []Expr{vec, g}, R: RepVector})
}

// buildColumn evaluates expr once per row into a new column.
func (l *lowerer) buildColumn(cur Expr, row *types.StructType, expr *ast.TreeNode, typ types.Type) Expr {
	r := rowRep(typ)
	b := l.newTemp(repColOut)
	rows := &Call{Fn: "q_tbl_rows", Args: []Expr{cur}, R: RepInt}
	l.add(&Let{T: b, Value: &Call{Fn: "q_col_new", Args: []Expr{intConst(ColumnDtype(typ)), rows}, R: repColOut}})
	rl, n, dense := l.openRow(cur, row, []*ast.TreeNode{expr})
	dense = l.andDense(dense, l.aggDense(expr))
	pushOf := func(v Expr) *Do {
		return &Do{X: &Call{Fn: pushFns[r], Args: []Expr{&TempRef{T: b}, l.asRowRep(expr, v, r)}, R: Rep("void")}}
	}
	l.denseOrSlow(dense, func() {
		l.rowFor(rl, n, func() {
			l.add(pushOf(l.expr(expr)))
		})
	}, func() {
		l.rowFor(rl, n, func() {
			v, p := l.rowValue(expr)
			l.pushOrNull(b, p, pushOf(v))
		})
	})
	l.endRow(rl)
	return l.let(&Call{Fn: "q_col_finish", Args: []Expr{&TempRef{T: b}}, R: RepVector})
}

// buildGroupColumn evaluates expr once per group. keys are the group-key
// columns, indexed by the input column, already one value per group.
func (l *lowerer) buildGroupColumn(n Expr, keys map[int]*Temp, row *types.StructType, expr *ast.TreeNode, typ types.Type) Expr {
	r := rowRep(typ)
	b := l.newTemp(repColOut)
	l.add(&Let{T: b, Value: &Call{Fn: "q_col_new", Args: []Expr{intConst(ColumnDtype(typ)), n}, R: repColOut}})
	idx := l.newTemp(RepInt)
	rl := &rowLoop{idx: idx, row: row, cols: keys}
	l.drawContext(rl, []*ast.TreeNode{expr})
	l.rowFor(rl, n, func() {
		v, p := l.rowValue(expr)
		l.pushOrNull(b, p, &Do{X: &Call{Fn: pushFns[r], Args: []Expr{&TempRef{T: b}, l.asRowRep(expr, v, r)}, R: Rep("void")}})
	})
	l.endRow(rl)
	return l.let(&Call{Fn: "q_col_finish", Args: []Expr{&TempRef{T: b}}, R: RepVector})
}

func (l *lowerer) pushOrNull(b *Temp, present Expr, push *Do) {
	if present == nil {
		l.add(push)
		return
	}
	null := &Do{X: &Call{Fn: "q_col_push_null", Args: []Expr{&TempRef{T: b}}, R: Rep("void")}}
	l.add(&If{Cond: present, Then: &Block{Stmts: []Stmt{push}}, Else: &Block{Stmts: []Stmt{null}}})
}

func (l *lowerer) andDense(a, b Expr) Expr {
	switch {
	case a == nil:
		return b
	case b == nil:
		return a
	default:
		return l.let(&AndAlso{L: a, X: b})
	}
}

// aggDense is true when every aggregate expr reads that can be missing is
// present for every group. Nil when none of them can be missing.
func (l *lowerer) aggDense(exprs ...*ast.TreeNode) Expr {
	if l.agg == nil {
		return nil
	}
	var checks []Expr
	seen := map[*types.AggUse]bool{}
	for _, e := range exprs {
		for _, u := range l.aggUsesIn(e, nil) {
			if seen[u] || !u.CanMiss {
				continue
			}
			seen[u] = true
			checks = append(checks, &Call{Fn: "q_grp_dense", Args: []Expr{l.agg.vecs[u]}, R: RepBool})
		}
	}
	if len(checks) == 0 {
		return nil
	}
	all := checks[0]
	for _, c := range checks[1:] {
		all = &AndAlso{L: all, X: c}
	}
	return l.let(all)
}

func (l *lowerer) aggUsesIn(node *ast.TreeNode, skip map[*ast.TreeNode]bool) []*types.AggUse {
	var out []*types.AggUse
	seen := map[*types.AggUse]bool{}
	var walk func(*ast.TreeNode)
	walk = func(n *ast.TreeNode) {
		if n == nil || skip[n] || n.NodeType == ast.LambdaNode {
			return
		}
		if u := l.an.AggUses[n]; u != nil {
			if !seen[u] {
				seen[u] = true
				out = append(out, u)
			}
			return
		}
		if _, ok := l.an.ColumnRefs[n]; ok {
			return
		}
		for _, ch := range n.Children {
			walk(ch)
		}
	}
	walk(node)
	return out
}

func (l *lowerer) groupIndex() Expr {
	if l.agg.perGroup {
		return &TempRef{T: l.row.idx}
	}
	if l.agg.whole {
		return intConst(0)
	}
	return &Call{Fn: "q_tbl_group_of", Args: []Expr{l.agg.group, &TempRef{T: l.row.idx}}, R: RepInt}
}

// bindAgg records an aggregate's per-group vector. For a whole-table
// filter or mutate it also reads the one value, and whether it is present,
// into locals before the row loop: a row then reads a register, where
// reading the vector on every row is a load clang can't hoist past the
// loop's writes (specs/tables_v0_3_spec.md §3.7). An entry of a missing
// aggregate is zeroed, so the read is defined; the row checks present first.
func (l *lowerer) bindAgg(u *types.AggUse, vec Expr) {
	l.agg.vecs[u] = vec
	if !l.agg.whole {
		return
	}
	w := wholeAgg{value: l.let(l.aggRead(u, vec, intConst(0)))}
	if u.CanMiss {
		w.present = l.let(&Call{Fn: "q_grp_present", Args: []Expr{vec, intConst(0)}, R: RepBool})
	}
	l.agg.wholeVals[u] = w
}

func (l *lowerer) aggPresent(u *types.AggUse) Expr {
	if w, ok := l.agg.wholeVals[u]; ok && !l.agg.perGroup && w.present != nil {
		return w.present
	}
	return &Call{Fn: "q_grp_present", Args: []Expr{l.agg.vecs[u], l.groupIndex()}, R: RepBool}
}

func (l *lowerer) aggValue(node *ast.TreeNode, u *types.AggUse) Expr {
	if l.agg == nil || l.agg.vecs[u] == nil {
		l.ice("INV-QUERY-AGG", node, "aggregate %s was not computed", u.Fn)
	}
	if w, ok := l.agg.wholeVals[u]; ok && !l.agg.perGroup {
		return w.value
	}
	return l.aggRead(u, l.agg.vecs[u], l.groupIndex())
}

// aggRead reads group idx of an aggregate's per-group vector.
func (l *lowerer) aggRead(u *types.AggUse, vec, idx Expr) Expr {
	switch rowRep(u.Type) {
	case RepFloat:
		return &Call{Fn: "q_grp_f64", Args: []Expr{vec, idx}, R: RepFloat}
	case RepInt:
		return &Call{Fn: "q_grp_i64", Args: []Expr{vec, idx}, R: RepInt}
	case RepBool:
		return &Call{Fn: "q_grp_bool", Args: []Expr{vec, idx}, R: RepBool}
	case RepDate:
		return &Call{Fn: "q_grp_date", Args: []Expr{vec, idx}, R: RepDate}
	default:
		return &Call{Fn: "q_grp_str", Args: []Expr{vec, idx}, R: RepStrView}
	}
}
