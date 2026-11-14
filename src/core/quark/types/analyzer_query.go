package types

import (
	"quark/ast"
)

// Table queries, `t | e -> verb(...) | verb(...)` (tables_v0_1_spec.md §3-4).
// The checker follows the row type through every step (§3.1): each step reads
// the row its input has and produces the row of its output, so every column
// a verb expression reads (e.col) is known with its type at compile time.
// Lowering compiles each step from the Query recorded here.

// Query is an analyzed query: its steps in order, and the row of its result.
type Query struct {
	Steps []*QueryStep
	Row   *StructType
}

// QueryStep is one verb. In is the row it reads, Out the row it produces.
type QueryStep struct {
	Node *ast.TreeNode
	Verb string
	In   *StructType
	Out  *StructType

	Pred  *ast.TreeNode   // filter: the condition
	Cols  []*MutateColumn // mutate: the columns, in the order written
	Keys  []SortKey       // sort; group_by (Desc unused)
	Aggs  []*AggregateCol // summarize
	Group []int           // summarize: the group_by key columns of In; nil for the whole table
	Count *ast.TreeNode   // head, tail: the row count; nil for the default (6)
}

// MutateColumn is one `name = value` of a mutate (D23): its value reads the
// row Reads (the input with the columns written before it) and makes the
// row After, where it is column Index.
type MutateColumn struct {
	Name  string
	Value *ast.TreeNode
	Type  Type
	Reads *StructType
	After *StructType
	Index int
}

// SortKey is a sort or group key: a column of the step's input row.
type SortKey struct {
	Col  int
	Desc bool
}

// AggregateCol is one `name = agg(e.col)` of a summarize (D24). Col is the
// aggregated column of the input row, -1 for count().
type AggregateCol struct {
	Name string
	Fn   string
	Col  int
	Type Type
}

// queryVerbs are the steps a query may have, for messages.
const queryVerbs = "filter, mutate, sort, group_by, summarize, head, tail"

// rowContext is the row a verb expression reads: the row name's symbol, the
// row type, and the function nesting where it is bound (a function written
// inside a verb expression can't read the row). noReads marks an argument
// computed once for the whole step (head's count).
type rowContext struct {
	sym     *Symbol
	row     *StructType
	depth   int
	noReads bool
	verb    string
}

// analyzeQuery checks a query and records it for lowering.
func (a *Analyzer) analyzeQuery(node *ast.TreeNode) Type {
	if len(node.Children) < 3 {
		a.errorAt(node, "malformed query")
		return TypeError
	}
	if a.rowCtx != nil {
		a.errorAt(node, "a query can't run inside a verb of another query; compute it before the query and use the result")
		return TypeError
	}
	source, rowName := node.Children[0], node.Children[1].TokenLiteral()
	srcType := a.Analyze(source)
	row := a.queryRow(source, srcType)
	if row == nil {
		return TypeError
	}
	q := &Query{}
	var groupBy *QueryStep
	failed := false
	for _, stepNode := range node.Children[2:] {
		verb := stepNode.TokenLiteral()
		if groupBy != nil && verb != "summarize" {
			a.errorAt(stepNode, "after group_by comes summarize: a grouped table can only be summarized (got %s)", verb)
			failed = true
			break
		}
		step := &QueryStep{Node: stepNode, Verb: verb, In: row, Out: row}
		ok := true
		switch verb {
		case "filter":
			ok = a.queryFilter(step, rowName)
		case "mutate":
			ok = a.queryMutate(step, rowName)
		case "sort":
			ok = a.querySort(step, rowName)
		case "group_by":
			ok = a.queryGroupBy(step, rowName)
		case "summarize":
			ok = a.querySummarize(step, rowName, groupBy)
		case "head", "tail":
			ok = a.querySlice(step, rowName)
		default:
			a.errorAt(stepNode, "'%s' is not a query step (verbs: %s)", verb, queryVerbs)
			ok = false
		}
		if !ok {
			failed = true
			break
		}
		if verb == "group_by" {
			groupBy = step
		} else {
			groupBy = nil
		}
		q.Steps = append(q.Steps, step)
		row = step.Out
	}
	if failed {
		return TypeError
	}
	if groupBy != nil {
		a.errorAt(groupBy.Node, "group_by must be followed by summarize in the same query: a grouped table can't be stored or passed on")
		return TypeError
	}
	q.Row = row
	a.queries[node] = q
	return &TableType{Row: row}
}

// queryRow is the row type of the table a query runs on, or nil after an
// error (D11: the compiler must know the row).
func (a *Analyzer) queryRow(source *ast.TreeNode, t Type) *StructType {
	if IsErrorType(t) {
		return nil
	}
	if tt, ok := t.(*TableType); ok {
		if tt.Row == nil {
			a.errorAt(source, "the row type of this table isn't known, so the query's columns can't be checked; give it a row struct where it is created or passed (t: tbl[Employee] = ...)")
			return nil
		}
		return tt.Row
	}
	if isUnknownType(t) {
		a.errorAt(source, "a query needs a table whose row type is known, and this value's type isn't; annotate it (t: tbl[Employee]), or the return type of the function it comes from (fn f(...) tbl[Employee] -> ...)")
		return nil
	}
	a.errorAt(source, "a query runs on a table, not on %s", t.String())
	return nil
}

// inRow analyzes fn with the row name bound to row, in a scope of its own
// (the row name shadows an outer variable of that name, like a parameter).
func (a *Analyzer) inRow(name, verb string, row *StructType, noReads bool, fn func()) {
	a.pushScope()
	sym := a.currentScope.Define(name, TypeVoid, false)
	sym.Kind = SymRow
	saved := a.rowCtx
	a.rowCtx = &rowContext{sym: sym, row: row, depth: a.lambdaDepth, noReads: noReads, verb: verb}
	fn()
	a.rowCtx = saved
	a.popScope()
}

// rowExpr analyzes a verb expression reading row.
func (a *Analyzer) rowExpr(node *ast.TreeNode, name, verb string, row *StructType) Type {
	var t Type
	a.inRow(name, verb, row, false, func() { t = a.Analyze(node) })
	return t
}

// rowRead types `e.col` when node's object is the query's row name; ok is
// false when it isn't.
func (a *Analyzer) rowRead(node *ast.TreeNode) (Type, bool) {
	obj := node.Children[0]
	if obj.NodeType != ast.IdentifierNode {
		return nil, false
	}
	sym := a.currentScope.Lookup(obj.TokenLiteral())
	if sym == nil || sym.Kind != SymRow {
		return nil, false
	}
	rc := a.rowCtx
	name, col := obj.TokenLiteral(), node.Children[1].TokenLiteral()
	switch {
	case rc == nil || rc.sym != sym:
		a.errorAt(node, "'%s' is the row of an outer query; a query inside a verb can't read it", name)
		return TypeError, true
	case rc.noReads:
		a.errorAt(node, "%s's row count is one number for the whole table; it can't read the row ('%s.%s')", rc.verb, name, col)
		return TypeError, true
	case a.lambdaDepth > rc.depth:
		a.errorAt(node, "a function written inside a verb can't read the row ('%s.%s'); it may run after the row's step", name, col)
		return TypeError, true
	}
	field, idx := rc.row.FieldByName(col)
	if field == nil {
		a.errorAt(node.Children[1], "%s has no column '%s' (columns: %s)", rowShown(rc.row), col, columnList(rc.row))
		return TypeError, true
	}
	a.columnRefs[node] = idx
	return field.Type, true
}

// rowShown is a row as messages show it: a struct's name, or the shape of
// an anonymous row.
func rowShown(row *StructType) string {
	if row.Anonymous {
		return "the row " + row.String()
	}
	return row.Name
}

// isColumnRef reports whether node is `e.col` for the current row (the
// checker recorded it), the only form sort and group keys and aggregate
// arguments take.
func (a *Analyzer) isColumnRef(node *ast.TreeNode) (int, bool) {
	idx, ok := a.columnRefs[node]
	return idx, ok && node.NodeType == ast.MemberNode
}

// noNamedArgs reports a `name = value` argument of a verb that takes none.
func (a *Analyzer) noNamedArgs(step *QueryStep) bool {
	for _, arg := range step.Node.Children {
		if arg.NodeType == ast.ColumnDefNode {
			a.errorAt(arg, "%s takes no named arguments ('%s = ...'); named columns are for mutate and summarize", step.Verb, arg.TokenLiteral())
			return false
		}
	}
	return true
}

func (a *Analyzer) queryFilter(step *QueryStep, rowName string) bool {
	args := step.Node.Children
	if !a.noNamedArgs(step) {
		return false
	}
	if len(args) != 1 {
		a.errorAt(step.Node, "filter takes one condition, got %d; join conditions with 'and'", len(args))
		return false
	}
	t := a.rowExpr(args[0], rowName, "filter", step.In)
	if IsErrorType(t) {
		return false
	}
	if !t.Equals(TypeBool) {
		a.errorAt(args[0], "filter's condition must be a bool, got %s", t.String())
		return false
	}
	step.Pred = args[0]
	return true
}

func (a *Analyzer) queryMutate(step *QueryStep, rowName string) bool {
	if len(step.Node.Children) == 0 {
		a.errorAt(step.Node, "mutate needs at least one column: mutate(name = expression)")
		return false
	}
	row := step.In
	written := map[string]bool{}
	for _, arg := range step.Node.Children {
		if arg.NodeType != ast.ColumnDefNode || len(arg.Children) != 1 {
			a.errorAt(arg, "mutate's columns are written name = expression")
			return false
		}
		name := arg.TokenLiteral()
		if written[name] {
			a.errorAt(arg, "mutate defines column '%s' twice", name)
			return false
		}
		written[name] = true
		t := a.rowExpr(arg.Children[0], rowName, "mutate", row)
		if IsErrorType(t) {
			return false
		}
		if !a.columnTypeOK(arg.Children[0], name, t) {
			return false
		}
		fields := append([]StructField(nil), row.Fields...)
		_, idx := row.FieldByName(name)
		if idx >= 0 {
			fields[idx] = StructField{Name: name, Type: t}
		} else {
			idx = len(fields)
			fields = append(fields, StructField{Name: name, Type: t})
		}
		after := a.anonRow(fields)
		step.Cols = append(step.Cols, &MutateColumn{Name: name, Value: arg.Children[0], Type: t, Reads: row, After: after, Index: idx})
		row = after
	}
	step.Out = row
	return true
}

// columnTypeOK checks that a computed column's values have one column type.
func (a *Analyzer) columnTypeOK(at *ast.TreeNode, name string, t Type) bool {
	if IsColumnType(t) {
		return true
	}
	if isUnknownType(t) {
		a.errorAt(at, "the type of column '%s' isn't known; table columns are int, float, bool or str (annotate the return type of a function it calls)", name)
		return false
	}
	a.errorAt(at, "column '%s' would be %s; table columns are int, float, bool or str", name, t.String())
	return false
}

// keyColumns checks sort and group_by keys: each one a column, e.col.
func (a *Analyzer) keyColumns(step *QueryStep, rowName string, sorting bool) bool {
	if !a.noNamedArgs(step) {
		return false
	}
	if len(step.Node.Children) == 0 {
		a.errorAt(step.Node, "%s needs at least one column: %s(e.col)", step.Verb, step.Verb)
		return false
	}
	seen := map[int]bool{}
	for _, arg := range step.Node.Children {
		key, desc := arg, false
		if arg.NodeType == ast.SortKeyNode && len(arg.Children) == 1 {
			key, desc = arg.Children[0], arg.Descending
		}
		t := a.rowExpr(key, rowName, step.Verb, step.In)
		if IsErrorType(t) {
			return false
		}
		col, ok := a.isColumnRef(key)
		if !ok {
			what := "group keys"
			if sorting {
				what = "sort keys"
			}
			a.errorAt(key, "%s are columns (%s.col); to use a computed value, add it with mutate first", what, rowName)
			return false
		}
		if seen[col] {
			a.errorAt(key, "%s names column '%s' twice", step.Verb, step.In.Fields[col].Name)
			return false
		}
		seen[col] = true
		step.Keys = append(step.Keys, SortKey{Col: col, Desc: desc})
	}
	return true
}

func (a *Analyzer) querySort(step *QueryStep, rowName string) bool {
	return a.keyColumns(step, rowName, true)
}

func (a *Analyzer) queryGroupBy(step *QueryStep, rowName string) bool {
	return a.keyColumns(step, rowName, false)
}

func (a *Analyzer) querySummarize(step *QueryStep, rowName string, groupBy *QueryStep) bool {
	if len(step.Node.Children) == 0 {
		a.errorAt(step.Node, "summarize needs at least one column: summarize(n = count())")
		return false
	}
	var fields []StructField
	names := map[string]string{} // column name -> what defined it
	if groupBy != nil {
		for _, k := range groupBy.Keys {
			f := step.In.Fields[k.Col]
			fields = append(fields, StructField{Name: f.Name, Type: f.Type})
			names[f.Name] = "a group key"
			step.Group = append(step.Group, k.Col)
		}
	}
	for _, arg := range step.Node.Children {
		if arg.NodeType == ast.AggregateNode {
			a.errorAt(arg, "give the summarize column a name: name = %s(...)", arg.TokenLiteral())
			return false
		}
		if arg.NodeType != ast.ColumnDefNode || len(arg.Children) != 1 || arg.Children[0].NodeType != ast.AggregateNode {
			a.errorAt(arg, "summarize's columns are written name = agg(%s.col)", rowName)
			return false
		}
		name := arg.TokenLiteral()
		if what, dup := names[name]; dup {
			if what == "a group key" {
				a.errorAt(arg, "summarize column '%s' has the name of a group key; choose another name", name)
			} else {
				a.errorAt(arg, "summarize defines column '%s' twice", name)
			}
			return false
		}
		names[name] = "a column"
		agg := a.aggregate(arg.Children[0], name, rowName, step.In)
		if agg == nil {
			return false
		}
		step.Aggs = append(step.Aggs, agg)
		fields = append(fields, StructField{Name: name, Type: agg.Type})
	}
	step.Out = a.anonRow(fields)
	return true
}

// aggregates are the summarize aggregates (D24), for messages.
const aggregates = "count, sum, mean, min, max"

// aggregate checks one aggregate: count() or agg(e.col) (D6, §4).
func (a *Analyzer) aggregate(node *ast.TreeNode, name, rowName string, row *StructType) *AggregateCol {
	fn := node.TokenLiteral()
	args := node.Children
	switch fn {
	case "count", "sum", "mean", "min", "max":
	default:
		a.errorAt(node, "'%s' is not an aggregate (aggregates: %s); to summarize a computed value, add it with mutate first", fn, aggregates)
		return nil
	}
	if fn == "count" && len(args) == 0 {
		return &AggregateCol{Name: name, Fn: fn, Col: -1, Type: TypeInt}
	}
	if len(args) != 1 {
		a.errorAt(node, "%s takes one column, %s(%s.col)", fn, fn, rowName)
		return nil
	}
	t := a.rowExpr(args[0], rowName, "summarize", row)
	if IsErrorType(t) {
		return nil
	}
	col, ok := a.isColumnRef(args[0])
	if !ok {
		a.errorAt(args[0], "%s aggregates a column (%s.col), not an expression; compute the value with mutate first", fn, rowName)
		return nil
	}
	ct := row.Fields[col].Type
	agg := &AggregateCol{Name: name, Fn: fn, Col: col}
	switch fn {
	case "count":
		agg.Type = TypeInt
	case "sum":
		switch {
		case ct.Equals(TypeFloat):
			agg.Type = TypeFloat
		case ct.Equals(TypeInt), ct.Equals(TypeBool):
			agg.Type = TypeInt
		}
	case "mean":
		if !ct.Equals(TypeString) {
			agg.Type = TypeFloat
		}
	default:
		agg.Type = ct
	}
	if agg.Type == nil {
		a.errorAt(args[0], "%s needs a number or bool column; column '%s' is %s", fn, row.Fields[col].Name, ct.String())
		return nil
	}
	return agg
}

func (a *Analyzer) querySlice(step *QueryStep, rowName string) bool {
	args := step.Node.Children
	if !a.noNamedArgs(step) {
		return false
	}
	if len(args) > 1 {
		a.errorAt(step.Node, "%s takes at most one argument, the number of rows", step.Verb)
		return false
	}
	if len(args) == 1 {
		var t Type
		a.inRow(rowName, step.Verb, step.In, true, func() { t = a.Analyze(args[0]) })
		if IsErrorType(t) {
			return false
		}
		if !t.Equals(TypeInt) {
			a.errorAt(args[0], "%s's row count must be an int, got %s", step.Verb, t.String())
			return false
		}
		step.Count = args[0]
	}
	return true
}

// rowCondition checks an operand of and, or, ! or a ternary's condition
// inside a verb expression: a bool, so a missing value can follow SQL's
// three-valued logic (D22) instead of truthiness.
func (a *Analyzer) rowCondition(node *ast.TreeNode, t Type, what string) bool {
	if !a.inVerbExpr() || IsErrorType(t) || t.Equals(TypeBool) {
		return true
	}
	hint := ""
	switch {
	case t.Equals(TypeString):
		hint = " (write a comparison, such as " + exprText(node) + " != '')"
	case IsNumeric(t):
		hint = " (write a comparison, such as " + exprText(node) + " != 0)"
	}
	a.errorAt(node, "in a query, %s takes bools, got %s%s", what, t.String(), hint)
	return false
}

// inVerbExpr reports whether the analyzer is in a verb expression itself
// (not in a function written inside one).
func (a *Analyzer) inVerbExpr() bool {
	return a.rowCtx != nil && !a.rowCtx.noReads && a.lambdaDepth == a.rowCtx.depth
}

// exprText is a short rendering of an expression for messages.
func exprText(node *ast.TreeNode) string {
	if node == nil {
		return "x"
	}
	switch node.NodeType {
	case ast.MemberNode:
		if len(node.Children) == 2 {
			return exprText(node.Children[0]) + "." + node.Children[1].TokenLiteral()
		}
	case ast.IdentifierNode, ast.LiteralNode:
		return node.TokenLiteral()
	}
	return "x"
}
