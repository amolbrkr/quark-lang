package types

import (
	"fmt"
	"sort"

	"quark/ast"
	"quark/ir"
)

// SessionSettle is a session variable whose element type this input fixed.
// Lowering asks the runtime to apply that type to the value already stored,
// before any statement of the input runs.
type SessionSettle struct {
	Sym *Symbol
	At  *ast.TreeNode
}

// SessionInfo is the analyzer's REPL output. Nil for an ordinary program.
type SessionInfo struct {
	Emit       *ast.TreeNode // the input block lowering emits
	Uses       map[*ast.TreeNode][]*Symbol
	Settles    []SessionSettle
	Slots      []*Symbol         // session variables, declaration order
	Fns        []*FixedFn        // every declaration, source order
	StructHome map[string]string // struct name -> input it was defined in
}

// analyzeSession analyzes a compilation unit whose children are input blocks.
// Each block's functions are hoisted within the block, so a block does not
// see definitions from later blocks. Top-level variables are session variables.
func (a *Analyzer) analyzeSession(node *ast.TreeNode) Type {
	var last Type = TypeVoid
	for _, block := range node.Children {
		if block == nil || block.NodeType != ast.InputBlockNode {
			continue
		}
		a.curBlock = block.SourceFile
		stmts := block.Children
		a.predeclareStructTypes(stmts)
		a.noteStructHome(stmts, block.SourceFile)
		for _, child := range stmts {
			if child != nil && child.NodeType == ast.ModuleNode && len(child.Children) >= 2 && child.Children[1] != nil {
				body := child.Children[1].Children
				a.predeclareStructTypes(body)
				a.noteStructHome(body, block.SourceFile)
			}
		}
		a.declareFixedFns(block, stmts)
		for _, child := range stmts {
			if child != nil && child.NodeType == ast.ModuleNode && len(child.Children) >= 2 && child.Children[1] != nil {
				a.declareFixedFns(child.Children[1], child.Children[1].Children)
			}
		}
		watched := a.watchPending()
		last = a.analyzeStatements(block, stmts)
		a.noteSettles(watched)
	}
	a.finalizeFixedFns()
	a.finalizeSymbols()
	return last
}

// noteStructHome records which input defined each struct named in stmts.
// A struct already recorded keeps its input, so a later block does not steal it.
func (a *Analyzer) noteStructHome(stmts []*ast.TreeNode, home string) {
	for _, stmt := range stmts {
		if stmt == nil || stmt.NodeType != ast.StructDefNode {
			continue
		}
		name := stmt.TokenLiteral()
		if _, ok := a.structHome[name]; !ok {
			a.structHome[name] = home
		}
	}
}

// definedWhere is "on line N" or "on line N of <input K>" when the definition
// came from a REPL input.
func definedWhere(n *ast.TreeNode, line int) string {
	if n != nil && n.SourceFile != "" {
		return fmt.Sprintf("on line %d of %s", line, n.SourceFile)
	}
	return fmt.Sprintf("on line %d", line)
}

type pendWatch struct {
	sym *Symbol
	ps  []*pending
}

func (a *Analyzer) watchPending() []pendWatch {
	var out []pendWatch
	for _, s := range a.symbols {
		if !s.Session {
			continue
		}
		var ps []*pending
		collectPending(s.Type, &ps)
		if len(ps) > 0 {
			out = append(out, pendWatch{sym: s, ps: ps})
		}
	}
	return out
}

func collectPending(t Type, acc *[]*pending) {
	switch c := t.(type) {
	case *ListType:
		if c.pending.Pending {
			*acc = append(*acc, &c.pending)
		}
		collectPending(c.ElementType, acc)
	case *DictType:
		if c.pending.Pending {
			*acc = append(*acc, &c.pending)
		}
		collectPending(c.ValueType, acc)
	case *ResultType:
		collectPending(c.OkType, acc)
		collectPending(c.ErrType, acc)
	}
}

func (a *Analyzer) noteSettles(watched []pendWatch) {
	for _, w := range watched {
		var at *ast.TreeNode
		fixed := false
		for _, p := range w.ps {
			if p.Pending {
				continue
			}
			fixed = true
			if p.FixedAt != nil {
				at = p.FixedAt
			}
		}
		if fixed {
			a.settles = append(a.settles, SessionSettle{Sym: w.sym, At: at})
		}
	}
}

// sessionInfo builds the REPL handoff after finalizeSymbols. emit is the last
// input block.
func (a *Analyzer) sessionInfo(root *ast.TreeNode) *SessionInfo {
	if !a.session {
		return nil
	}
	var emit *ast.TreeNode
	for _, c := range root.Children {
		if c != nil && c.NodeType == ast.InputBlockNode {
			emit = c
		}
	}
	var slots []*Symbol
	for _, s := range a.symbols {
		if s.Session {
			slots = append(slots, s)
		}
	}
	return &SessionInfo{
		Emit:       emit,
		Uses:       a.sessionUses,
		Settles:    a.settles,
		Slots:      slots,
		Fns:        a.fixedList,
		StructHome: a.structHome,
	}
}

// tagAnonHome marks an anonymous row created while analyzing an input as
// defined in that input. Called from the table analyzer.
func (a *Analyzer) tagAnonHome(st *StructType) {
	if a.session && st != nil && a.curBlock != "" {
		if _, ok := a.structHome[st.Name]; !ok {
			a.structHome[st.Name] = a.curBlock
		}
	}
}

// assignSessionSlots gives each session variable its host slot. int, float
// and bool get raw slots; everything else gets a cell. A captured session
// variable stays a session slot rather than a closure cell: functions read
// the host table instead of capturing it.
func (a *Analyzer) assignSessionSlots() (nInt, nFloat, nBool, nCell int) {
	for _, s := range a.symbols {
		if !s.Session {
			continue
		}
		switch {
		case s.StoreType != nil && s.StoreType.Equals(TypeInt):
			s.Storage = ir.StorageInt
			s.Slot = nInt
			nInt++
		case s.StoreType != nil && s.StoreType.Equals(TypeFloat):
			s.Storage = ir.StorageFloat
			s.Slot = nFloat
			nFloat++
		case s.StoreType != nil && s.StoreType.Equals(TypeBool):
			s.Storage = ir.StorageBool
			s.Slot = nBool
			nBool++
		default:
			s.Storage = ir.StorageCell
			s.Slot = nCell
			nCell++
		}
	}
	return
}

// sessionUser is a function that reads sym, and where it is defined, for the
// type-change error. The earliest definition wins.
func (a *Analyzer) sessionUser(sym *Symbol) (name, where string, ok bool) {
	type hit struct {
		name, where string
		file        string
		line        int
	}
	var hits []hit
	add := func(lambda *ast.TreeNode, uses []*Symbol) {
		if lambda == nil {
			return
		}
		for _, u := range uses {
			if u != sym {
				continue
			}
			h := hit{name: "a function", line: nodeLine(lambda), file: lambda.SourceFile}
			if f := a.fixedByLambda[lambda]; f != nil {
				h.name = f.Name
				h.line = f.Line
				if f.Decl != nil && f.Decl.SourceFile != "" {
					h.file = f.Decl.SourceFile
				}
			}
			if h.file != "" {
				h.where = fmt.Sprintf("%s:%d:1", h.file, h.line)
			} else {
				h.where = fmt.Sprintf("line %d", h.line)
			}
			hits = append(hits, h)
			return
		}
	}
	for _, f := range a.fixedList {
		add(f.Lambda, a.sessionUses[f.Lambda])
	}
	var lambdas []*ast.TreeNode
	for lambda := range a.sessionUses {
		if a.fixedByLambda[lambda] == nil {
			lambdas = append(lambdas, lambda)
		}
	}
	sort.Slice(lambdas, func(i, j int) bool {
		return nodeLine(lambdas[i]) < nodeLine(lambdas[j])
	})
	for _, lambda := range lambdas {
		add(lambda, a.sessionUses[lambda])
	}
	if len(hits) == 0 {
		return "", "", false
	}
	sort.Slice(hits, func(i, j int) bool {
		if hits[i].file != hits[j].file {
			return hits[i].file < hits[j].file
		}
		return hits[i].line < hits[j].line
	})
	return hits[0].name, hits[0].where, true
}

// allowTypeChange applies §3.4. It returns true when the assignment is a
// session type change (warned and rebound, or rejected because a function
// uses the variable). It returns false when the file's error should be
// reported instead: the assignment is not at the top level of an input, or
// the variable was defined in this same input.
func (a *Analyzer) allowTypeChange(target *ast.TreeNode, sym *Symbol, rightType Type) bool {
	if target == nil || a.curList == nil || a.curList.NodeType != ast.InputBlockNode {
		return false
	}
	if sym.Decl != nil && sym.Decl.SourceFile != "" && sym.Decl.SourceFile == target.SourceFile {
		return false
	}
	name := target.TokenLiteral()
	if fn, where, ok := a.sessionUser(sym); ok {
		a.errorAt(target, "'%s' can't change type from %s to %s: '%s' (%s) uses it", name, sym.Type.String(), rightType.String(), fn, where)
		return true
	}
	a.warnAt(target, "'%s' changes type from %s to %s; a compiled program rejects this", name, sym.Type.String(), rightType.String())
	newSym := a.declareSym(sym.Name, rightType, true, SymLocal, rightType, target)
	newSym.Replaces = sym
	a.currentScope.Symbols[sym.Name] = newSym
	a.refs[target] = newSym
	a.recordDef(newSym, target)
	return true
}
