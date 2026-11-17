package types

import (
	"quark/ast"
)

// Table queries, `t | e -> verb(...) | verb(...)` (specs/tables_v0_1_spec.md §3-4).
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

	Pred      *ast.TreeNode   // filter: the condition
	Cols      []*MutateColumn // mutate: the columns, in the order written
	Keys      []SortKey       // sort; distinct (Desc unused)
	Keep      []int           // select, deselect: columns of In kept, in output order
	SumCols   []*SumCol       // summarize: the columns, after the group keys
	GroupKeys []GroupKey      // the group_by this step consumes; a group_by step's own keys
	Aggs      []*AggUse       // filter and summarize: aggregates computed before the step
	Count     *ast.TreeNode   // head, tail: the row count; nil for the default (6)
}

// SumCol is one `name = expression` of a summarize. The expression's
// references to the row are aggregates or group keys (D38).
type SumCol struct {
	Name string
	Expr *ast.TreeNode
	Type Type
}

// GroupKey is a group_by key. Col is a column of the input when the key is
// that column (bare, or name = e.col); Expr is set when the key is computed
// from something other than one column. Name is the summarize output's name.
type GroupKey struct {
	Name string
	Col  int
	Expr *ast.TreeNode
	Type Type
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
	// Aggs are the aggregates this column introduces. One computed for an
	// earlier column of the same mutate is not repeated here.
	Aggs []*AggUse
}

// SortKey is a sort or group key: a column of the step's input row.
type SortKey struct {
	Col  int
	Desc bool
}

// queryVerbs are the steps a query may have, for messages.
const queryVerbs = "filter, mutate, select, deselect, rename, distinct, sort, group_by, summarize, head, tail"

// rowContext is the row a verb expression reads: the row name's symbol, the
// row type, and the function nesting where it is bound (a function written
// inside a verb expression can't read the row). noReads marks an argument
// computed once for the whole step (head's count). grouped means an aggregate
// in this expression differs from row to row (a grouped filter or mutate, or
// any summarize).
type rowContext struct {
	sym     *Symbol
	row     *StructType
	depth   int
	noReads bool
	verb    string
	grouped bool
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
		if groupBy != nil && verb != "filter" && verb != "mutate" && verb != "summarize" {
			a.errorAt(stepNode, "group_by applies to the next step, which must be filter, mutate or summarize (got %s)", verb)
			failed = true
			break
		}
		step := &QueryStep{Node: stepNode, Verb: verb, In: row, Out: row}
		// An aggregate is one value for the whole step unless the step is
		// grouped, in which case it is the row's group. summarize always
		// has one result per group.
		a.verbGrouped = (groupBy != nil && verb != "group_by") || verb == "summarize"
		ok := true
		switch verb {
		case "filter":
			ok = a.queryFilter(step, rowName)
		case "mutate":
			ok = a.queryMutate(step, rowName)
		case "select":
			ok = a.querySelect(step, rowName)
		case "deselect":
			ok = a.queryDeselect(step, rowName)
		case "rename":
			ok = a.queryRename(step, rowName)
		case "distinct":
			ok = a.queryDistinct(step, rowName)
		case "sort":
			ok = a.querySort(step, rowName)
		case "group_by":
			ok = a.queryGroupBy(step, rowName)
		case "summarize":
			var keys []GroupKey
			if groupBy != nil {
				keys = groupBy.GroupKeys
			}
			ok = a.querySummarize(step, rowName, keys)
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
			if groupBy != nil && verb != "summarize" {
				step.GroupKeys = groupBy.GroupKeys
			}
			groupBy = nil
		}
		q.Steps = append(q.Steps, step)
		row = step.Out
	}
	if failed {
		return TypeError
	}
	if groupBy != nil {
		a.errorAt(groupBy.Node, "group_by must be followed by filter, mutate or summarize in the same query")
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
	a.rowCtx = &rowContext{sym: sym, row: row, depth: a.lambdaDepth, noReads: noReads, verb: verb, grouped: a.verbGrouped}
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
	done := a.openAggs()
	t := a.rowExpr(args[0], rowName, "filter", step.In)
	step.Aggs = done()
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
	saved := a.aggs
	scope := &aggScope{byKey: map[string]*AggUse{}, gen: map[int]int{}}
	a.aggs = scope
	defer func() { a.aggs = saved }()
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
		before := len(scope.list)
		t := a.rowExpr(arg.Children[0], rowName, "mutate", row)
		fresh := append([]*AggUse(nil), scope.list[before:]...)
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
		step.Cols = append(step.Cols, &MutateColumn{Name: name, Value: arg.Children[0], Type: t, Reads: row, After: after, Index: idx, Aggs: fresh})
		// The next column reads what this one wrote.
		scope.gen[idx]++
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
		a.errorAt(at, "the type of column '%s' isn't known; table columns are int, float, bool, str or date (annotate the return type of a function it calls)", name)
		return false
	}
	a.errorAt(at, "column '%s' would be %s; table columns are int, float, bool, str or date", name, t.String())
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

// columnArgs checks a verb whose arguments are columns, e.col, and records
// them on step.Keep. bad is the error for an argument that isn't a column.
func (a *Analyzer) columnArgs(step *QueryStep, rowName, bad string) bool {
	if !a.noNamedArgs(step) {
		return false
	}
	if len(step.Node.Children) == 0 {
		a.errorAt(step.Node, "%s needs at least one column: %s(%s.col)", step.Verb, step.Verb, rowName)
		return false
	}
	seen := map[int]bool{}
	for _, arg := range step.Node.Children {
		t := a.rowExpr(arg, rowName, step.Verb, step.In)
		if IsErrorType(t) {
			return false
		}
		col, ok := a.isColumnRef(arg)
		if !ok {
			a.errorAt(arg, "%s", bad)
			return false
		}
		if seen[col] {
			a.errorAt(arg, "%s names column '%s' twice", step.Verb, step.In.Fields[col].Name)
			return false
		}
		seen[col] = true
		step.Keep = append(step.Keep, col)
	}
	return true
}

func (a *Analyzer) querySelect(step *QueryStep, rowName string) bool {
	if !a.columnArgs(step, rowName, "select keeps columns ("+rowName+".col); drop columns with deselect, compute a column with mutate, or rename it with rename") {
		return false
	}
	fields := make([]StructField, len(step.Keep))
	for i, c := range step.Keep {
		f := step.In.Fields[c]
		fields[i] = StructField{Name: f.Name, Type: f.Type}
	}
	step.Out = a.anonRow(fields)
	return true
}

func (a *Analyzer) queryDeselect(step *QueryStep, rowName string) bool {
	if !a.columnArgs(step, rowName, "deselect drops columns ("+rowName+".col)") {
		return false
	}
	if len(step.Keep) == len(step.In.Fields) {
		a.errorAt(step.Node, "deselect would remove every column")
		return false
	}
	drop := map[int]bool{}
	for _, c := range step.Keep {
		drop[c] = true
	}
	step.Keep = nil
	var fields []StructField
	for i, f := range step.In.Fields {
		if drop[i] {
			continue
		}
		step.Keep = append(step.Keep, i)
		fields = append(fields, StructField{Name: f.Name, Type: f.Type})
	}
	step.Out = a.anonRow(fields)
	return true
}

func (a *Analyzer) queryRename(step *QueryStep, rowName string) bool {
	if len(step.Node.Children) == 0 {
		a.errorAt(step.Node, "rename needs at least one column: rename(name = %s.col)", rowName)
		return false
	}
	names := make([]string, len(step.In.Fields))
	for i, f := range step.In.Fields {
		names[i] = f.Name
	}
	renamed := map[int]bool{}
	taken := map[string]bool{}
	for _, arg := range step.Node.Children {
		if arg.NodeType != ast.ColumnDefNode || len(arg.Children) != 1 {
			a.errorAt(arg, "rename's columns are written name = %s.col", rowName)
			return false
		}
		newName := arg.TokenLiteral()
		if taken[newName] {
			a.errorAt(arg, "rename would give two columns named '%s'", newName)
			return false
		}
		val := arg.Children[0]
		t := a.rowExpr(val, rowName, "rename", step.In)
		if IsErrorType(t) {
			return false
		}
		col, ok := a.isColumnRef(val)
		if !ok {
			a.errorAt(val, "rename takes a column (pay = %s.salary)", rowName)
			return false
		}
		if renamed[col] {
			a.errorAt(val, "rename names column '%s' twice", step.In.Fields[col].Name)
			return false
		}
		renamed[col] = true
		taken[newName] = true
		names[col] = newName
	}
	for i, n := range names {
		if renamed[i] {
			continue
		}
		if taken[n] {
			a.errorAt(step.Node, "rename would give two columns named '%s'", n)
			return false
		}
	}
	fields := make([]StructField, len(step.In.Fields))
	for i, f := range step.In.Fields {
		fields[i] = StructField{Name: names[i], Type: f.Type}
	}
	step.Out = a.anonRow(fields)
	return true
}

func (a *Analyzer) queryDistinct(step *QueryStep, rowName string) bool {
	if !a.noNamedArgs(step) {
		return false
	}
	if len(step.Node.Children) == 0 {
		for i := range step.In.Fields {
			step.Keys = append(step.Keys, SortKey{Col: i})
		}
		return true
	}
	seen := map[int]bool{}
	for _, arg := range step.Node.Children {
		t := a.rowExpr(arg, rowName, "distinct", step.In)
		if IsErrorType(t) {
			return false
		}
		col, ok := a.isColumnRef(arg)
		if !ok {
			a.errorAt(arg, "distinct takes columns (%s.col); compute the value with mutate first", rowName)
			return false
		}
		if seen[col] {
			a.errorAt(arg, "distinct names column '%s' twice", step.In.Fields[col].Name)
			return false
		}
		seen[col] = true
		step.Keys = append(step.Keys, SortKey{Col: col})
	}
	return true
}

func (a *Analyzer) querySort(step *QueryStep, rowName string) bool {
	return a.keyColumns(step, rowName, true)
}

func (a *Analyzer) queryGroupBy(step *QueryStep, rowName string) bool {
	if len(step.Node.Children) == 0 {
		a.errorAt(step.Node, "group_by needs at least one key: group_by(%s.col)", rowName)
		return false
	}
	seenCol := map[int]bool{}
	seenName := map[string]bool{}
	for _, arg := range step.Node.Children {
		name, expr := "", arg
		named := false
		if arg.NodeType == ast.ColumnDefNode && len(arg.Children) == 1 {
			named, name, expr = true, arg.TokenLiteral(), arg.Children[0]
		}
		t := a.rowExpr(expr, rowName, "group_by", step.In)
		if IsErrorType(t) {
			return false
		}
		col, isCol := a.isColumnRef(expr)
		if !named && !isCol {
			a.errorAt(expr, "a computed group key needs a name: group_by(year = time::year(%s.day))", rowName)
			return false
		}
		if !named {
			name = step.In.Fields[col].Name
		}
		if !a.columnTypeOK(expr, name, t) {
			return false
		}
		if seenName[name] {
			if !named && isCol {
				a.errorAt(expr, "group_by names column '%s' twice", name)
			} else {
				a.errorAt(arg, "group_by names key '%s' twice", name)
			}
			return false
		}
		if isCol && !named && seenCol[col] {
			a.errorAt(expr, "group_by names column '%s' twice", name)
			return false
		}
		seenName[name] = true
		key := GroupKey{Name: name, Col: -1, Type: t}
		if isCol {
			key.Col = col
			if !named {
				seenCol[col] = true
			}
		} else {
			key.Expr = expr
		}
		step.GroupKeys = append(step.GroupKeys, key)
	}
	return true
}

func (a *Analyzer) querySummarize(step *QueryStep, rowName string, keys []GroupKey) bool {
	if len(step.Node.Children) == 0 {
		a.errorAt(step.Node, "summarize needs at least one column: summarize(n = count())")
		return false
	}
	step.GroupKeys = keys
	var fields []StructField
	keyNames := map[string]bool{}
	keyCols := map[int]bool{}
	for _, k := range keys {
		fields = append(fields, StructField{Name: k.Name, Type: k.Type})
		keyNames[k.Name] = true
		if k.Col >= 0 && k.Expr == nil {
			keyCols[k.Col] = true
		}
	}
	done := a.openAggs()
	names := map[string]bool{}
	ok := true
	for _, arg := range step.Node.Children {
		if arg.NodeType != ast.ColumnDefNode || len(arg.Children) != 1 {
			if arg.NodeType == ast.FunctionCallNode && len(arg.Children) > 0 && arg.Children[0].NodeType == ast.IdentifierNode {
				a.errorAt(arg, "give the summarize column a name: name = %s(...)", arg.Children[0].TokenLiteral())
			} else {
				a.errorAt(arg, "give the summarize column a name: name = expression")
			}
			ok = false
			break
		}
		name := arg.TokenLiteral()
		if keyNames[name] {
			a.errorAt(arg, "summarize column '%s' has the name of a group key; choose another name", name)
			ok = false
			break
		}
		if names[name] {
			a.errorAt(arg, "summarize defines column '%s' twice", name)
			ok = false
			break
		}
		names[name] = true
		expr := arg.Children[0]
		t := a.rowExpr(expr, rowName, "summarize", step.In)
		if IsErrorType(t) {
			ok = false
			break
		}
		if !a.summarizeRefsOK(expr, keyCols) {
			ok = false
			break
		}
		if !a.columnTypeOK(expr, name, t) {
			ok = false
			break
		}
		step.SumCols = append(step.SumCols, &SumCol{Name: name, Expr: expr, Type: t})
		fields = append(fields, StructField{Name: name, Type: t})
	}
	step.Aggs = done()
	if !ok {
		return false
	}
	step.Out = a.anonRow(fields)
	return true
}

// summarizeRefsOK reports whether every reference to the row outside an
// aggregate is a group key (D38).
func (a *Analyzer) summarizeRefsOK(node *ast.TreeNode, keyCols map[int]bool) bool {
	var walk func(*ast.TreeNode) bool
	walk = func(n *ast.TreeNode) bool {
		if n == nil || n.NodeType == ast.LambdaNode {
			return true
		}
		if _, isAgg := a.aggUses[n]; isAgg {
			return true
		}
		if col, ok := a.columnRefs[n]; ok && n.NodeType == ast.MemberNode {
			if keyCols[col] {
				return true
			}
			sug := "min"
			if t := a.nodeTypes[n]; t != nil && (IsNumeric(t) || t.Equals(TypeBool)) {
				sug = "mean"
			}
			shown := exprText(n)
			a.errorAt(n, "'%s' has a value per row; use it in an aggregate (%s(%s)) or group by it", shown, sug, shown)
			return false
		}
		for _, ch := range n.Children {
			if !walk(ch) {
				return false
			}
		}
		return true
	}
	return walk(node)
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
