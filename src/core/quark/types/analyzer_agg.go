package types

import (
	"fmt"
	"sort"
	"strings"

	"quark/ast"
	"quark/builtins"
	"quark/ir"
)

// Aggregates in verb expressions (specs/tables_v0_3_spec.md D35, D37, D38).
// count, sum, mean, sd, min, max and n_distinct are syntax when their
// argument reads the row, and prelude functions when it doesn't (min and
// max with two arguments are always the prelude functions). An aggregate is
// computed once for the table or for the row's group, before the row loop.

// AggUse is one aggregate in a step. Col is a column of the row the
// argument reads when the argument is exactly that column, and -1 when the
// argument is an expression (or count() of the rows). The same aggregate
// written twice in a step is one AggUse.
type AggUse struct {
	Fn      string
	Arg     *ast.TreeNode // nil for count()
	ArgType Type
	Type    Type
	Col     int
	CanMiss bool
}

// aggScope collects the aggregates of the verb expression being analyzed.
// gen is how many times each column has been written by the current mutate,
// so an aggregate of a column written earlier in the same mutate is not the
// aggregate of the column it replaced.
type aggScope struct {
	list  []*AggUse
	byKey map[string]*AggUse
	gen   map[int]int
}

// tryAggregate reports whether call is an aggregate and, when it is, its
// type. A false handled means the caller analyzes it as an ordinary call;
// the arguments have not been analyzed.
//
// In a verb, a call of an aggregate name is never a user's definition: when
// the name resolves to one (a function, variable or parameter, or the row),
// the call is an error, since the aggregate would silently take its place
// (decided 2026-11-17). The loader marks a callee written as a bare
// aggregate name (fn.AggregateName: resolving it may have renamed the node);
// one it marked free has no definition in its imported file, so it isn't
// looked up: the scopes would find the main script's names. A qualified
// call (lib::count) is an ordinary call.
func (a *Analyzer) tryAggregate(call, fn *ast.TreeNode, args []*ast.TreeNode) (Type, bool) {
	if fn == nil || fn.NodeType != ast.IdentifierNode {
		return nil, false
	}
	name := fn.TokenLiteral()
	written := fn.AggregateName
	if written == "" {
		written = name // a tree the loader didn't resolve (the main script's names are as written)
	}
	if !builtins.IsQueryAggregate(written) {
		return nil, false
	}
	if !a.inVerbExpr() {
		if fn.AggregateFree && !isPreludeFunction(name) {
			// count(...) in a step argument that isn't a verb expression
			// (head's count) of an imported file: it names nothing there.
			a.errorAt(fn, "undefined identifier '%s'", name)
			return TypeError, true
		}
		return nil, false
	}
	if !fn.AggregateFree {
		if sym := a.currentScope.Lookup(name); sym != nil && !(sym.Kind == SymBuiltin && isPreludeFunction(name)) {
			if sym.Kind == SymRow {
				a.errorAt(fn, "'%s(...)' in a query is the aggregate, but the query's row is named '%s'; rename the row", written, written)
				return TypeError, true
			}
			where := ""
			if line := nodeLine(sym.Decl); line > 0 {
				where = fmt.Sprintf(" (line %d)", line)
			}
			a.errorAt(fn, "'%s(...)' in a query is the aggregate, but this program defines '%s'%s; rename it to use the aggregate", written, written, where)
			return TypeError, true
		}
	}
	rowName := a.rowCtx.sym.Name
	switch name {
	case "min", "max", "sum", "mean", "sd":
		if len(args) != 1 {
			return nil, false
		}
	case "count":
		if len(args) == 0 {
			return a.countRowsAgg(call), true
		}
		if len(args) != 1 {
			a.errorAt(call, "count() takes no arguments, or one expression of the row (count(%s.col)), got %d", rowName, len(args))
			return TypeError, true
		}
	case "n_distinct":
		if len(args) != 1 {
			a.errorAt(call, "n_distinct takes one expression of the row (%s.col), got %d", rowName, len(args))
			return TypeError, true
		}
	default:
		return nil, false
	}
	arg := args[0]
	if !a.exprReadsRow(arg) {
		if name == "count" || name == "n_distinct" {
			a.errorAt(arg, "%s's argument must be an expression of the row (%s.col)", name, rowName)
			return TypeError, true
		}
		return nil, false
	}
	if a.inAgg {
		a.errorAt(call, "an aggregate can't contain another aggregate")
		return TypeError, true
	}
	if a.aggs == nil {
		a.errorAt(call, "%s is an aggregate here, and aggregates are for filter, mutate and summarize", name)
		return TypeError, true
	}
	a.inAgg = true
	t := a.Analyze(arg)
	a.inAgg = false
	if IsErrorType(t) {
		return TypeError, true
	}
	return a.finishAgg(call, name, arg, t), true
}

// isPreludeFunction reports whether name is a prelude builtin (an extern fn
// is a SymBuiltin too, but it is the program's own definition).
func isPreludeFunction(name string) bool {
	_, ok := builtins.Lookup(name)
	return ok
}

func (a *Analyzer) countRowsAgg(call *ast.TreeNode) Type {
	if a.inAgg {
		a.errorAt(call, "an aggregate can't contain another aggregate")
		return TypeError
	}
	if a.aggs == nil {
		a.errorAt(call, "count() is an aggregate here, and aggregates are for filter, mutate and summarize")
		return TypeError
	}
	a.noteAgg(call, "count", nil, nil, TypeInt, -1, false)
	return TypeInt
}

func (a *Analyzer) finishAgg(call *ast.TreeNode, fn string, arg *ast.TreeNode, t Type) Type {
	col := -1
	if c, ok := a.isColumnRef(arg); ok {
		col = c
	}
	res, canMiss, ok := a.aggResultType(fn, t)
	if !ok {
		a.aggTypeError(arg, fn, col, t)
		return TypeError
	}
	a.noteAgg(call, fn, arg, t, res, col, canMiss)
	return res
}

// aggResultType is the type of an aggregate of a value of type t, and
// whether a group with no present values makes it missing.
func (a *Analyzer) aggResultType(fn string, t Type) (Type, bool, bool) {
	switch fn {
	case "count", "n_distinct":
		if !IsColumnType(t) {
			return nil, false, false
		}
		return TypeInt, false, true
	case "sum":
		switch {
		case t.Equals(TypeFloat):
			return TypeFloat, false, true
		case t.Equals(TypeInt), t.Equals(TypeBool):
			return TypeInt, false, true
		}
	case "mean", "sd":
		if t.Equals(TypeInt) || t.Equals(TypeFloat) || t.Equals(TypeBool) {
			return TypeFloat, true, true
		}
	case "min", "max":
		if IsColumnType(t) {
			return t, true, true
		}
	}
	return nil, false, false
}

func (a *Analyzer) aggTypeError(arg *ast.TreeNode, fn string, col int, t Type) {
	if fn == "count" || fn == "n_distinct" || fn == "min" || fn == "max" {
		a.errorAt(arg, "%s's argument must be a column value (int, float, bool, str or date), got %s", fn, t.String())
		return
	}
	if col >= 0 {
		name := a.rowCtx.row.Fields[col].Name
		a.errorAt(arg, "%s needs a number or bool column; column '%s' is %s", fn, name, t.String())
		return
	}
	if t.Equals(TypeDate) {
		a.errorAt(arg, "%s of a date isn't defined; dates have min, max, count and n_distinct", fn)
		return
	}
	a.errorAt(arg, "%s needs a number or bool, got %s", fn, t.String())
}

func (a *Analyzer) noteAgg(call *ast.TreeNode, fn string, arg *ast.TreeNode, argType, res Type, col int, canMiss bool) {
	n := 0
	if arg != nil {
		n = 1
	}
	a.callPlans[call] = &ir.CallPlan{CalleeName: fn, MinArity: n, MaxArity: n, Dispatch: ir.DispatchAggregate}
	key := a.aggKey(fn, arg)
	if prev := a.aggs.byKey[key]; prev != nil {
		a.aggUses[call] = prev
		return
	}
	use := &AggUse{Fn: fn, Arg: arg, ArgType: argType, Type: res, Col: col, CanMiss: canMiss}
	a.aggs.byKey[key] = use
	a.aggs.list = append(a.aggs.list, use)
	a.aggUses[call] = use
}

// aggKey identifies an aggregate inside one step. A mutate that writes a
// column bumps that column's generation, so sum(e.x) before the write and
// sum(e.x) after it are different.
func (a *Analyzer) aggKey(fn string, arg *ast.TreeNode) string {
	if arg == nil {
		return fn
	}
	var b strings.Builder
	b.WriteString(fn)
	b.WriteByte(0)
	writeFP(&b, arg)
	cols := a.colRefsIn(arg)
	sort.Ints(cols)
	for _, c := range cols {
		fmt.Fprintf(&b, "|%d@%d", c, a.aggs.gen[c])
	}
	return b.String()
}

func writeFP(b *strings.Builder, n *ast.TreeNode) {
	if n == nil {
		b.WriteByte('#')
		return
	}
	fmt.Fprintf(b, "%d:", n.NodeType)
	if n.Token != nil {
		b.WriteString(n.Token.Literal)
		if n.Op() != 0 {
			fmt.Fprintf(b, "/%d", n.Op())
		}
	}
	b.WriteByte('(')
	for _, c := range n.Children {
		writeFP(b, c)
		b.WriteByte(',')
	}
	b.WriteByte(')')
}

// colRefsIn lists the columns of the current row that n reads.
func (a *Analyzer) colRefsIn(n *ast.TreeNode) []int {
	var out []int
	seen := map[int]bool{}
	var walk func(*ast.TreeNode)
	walk = func(n *ast.TreeNode) {
		if n == nil || n.NodeType == ast.LambdaNode {
			return
		}
		if c, ok := a.columnRefs[n]; ok && n.NodeType == ast.MemberNode {
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
	walk(n)
	return out
}

// exprReadsRow reports whether n reads the current query row. It looks at
// the text, so it can decide before the expression is type-checked. A
// function written inside the expression is skipped: it can't read the row.
func (a *Analyzer) exprReadsRow(n *ast.TreeNode) bool {
	name := a.rowCtx.sym.Name
	var walk func(*ast.TreeNode) bool
	walk = func(n *ast.TreeNode) bool {
		if n == nil || n.NodeType == ast.LambdaNode {
			return false
		}
		if n.NodeType == ast.MemberNode && len(n.Children) == 2 &&
			n.Children[0].NodeType == ast.IdentifierNode && n.Children[0].TokenLiteral() == name {
			return true
		}
		for _, ch := range n.Children {
			if walk(ch) {
				return true
			}
		}
		return false
	}
	return walk(n)
}

// openAggs starts a scope for the aggregates of one verb expression. The
// returned function restores the previous scope and yields the aggregates
// recorded, in first-seen order.
func (a *Analyzer) openAggs() (done func() []*AggUse) {
	saved := a.aggs
	scope := &aggScope{byKey: map[string]*AggUse{}, gen: map[int]int{}}
	a.aggs = scope
	return func() []*AggUse {
		list := scope.list
		a.aggs = saved
		return list
	}
}
