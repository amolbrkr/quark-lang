package lower

import (
	"strconv"
	"strings"

	"quark/ast"
	"quark/ir"
	"quark/types"
)

// sessionProgram lowers the last input of a REPL session. Earlier inputs
// stay in the libraries already loaded; this one declares them extern and
// defines only its own functions.
func (l *lowerer) sessionProgram() {
	sess := l.an.Session
	block := sess.Emit
	if block == nil {
		l.ice("INV-REPL", nil, "session has no input to emit")
		return
	}
	p := l.prog
	p.Session = &SessionEmit{Input: inputNumber(block.SourceFile)}
	p.Session.NInt, p.Session.NFloat, p.Session.NBool, p.Session.NCell = slotCounts(sess.Slots)
	p.Includes = collectExternSources(block)
	l.externHome = externSymbols(block)

	l.collectFunctions(block)
	for _, fd := range l.fns {
		l.funcShell(fd)
		p.Decls = append(p.Decls, fd.fn)
		if fd.thunk != nil {
			p.Decls = append(p.Decls, fd.thunk)
		}
	}
	l.externShells()
	externThunks := l.externThunks()
	p.Decls = append(p.Decls, externThunks...)

	l.fnInfos()
	l.structDefs()
	l.defaultArgFns()

	for _, fd := range l.fns {
		l.funcBody(fd)
		p.Funcs = append(p.Funcs, fd.fn)
		if fd.thunk != nil {
			p.Funcs = append(p.Funcs, fd.thunk)
		}
	}
	p.Funcs = append(p.Funcs, externThunks...)

	p.Main = &Block{}
	l.cur = p.Main
	l.checkedCells = nil
	p.Session.Binds = l.sessionBinds(block)
	for _, s := range sess.Settles {
		if s.At != nil {
			l.setLoc(s.At)
		}
		desc := "nullptr"
		if s.Sym != nil {
			desc = l.elemDesc(s.Sym.Type)
		}
		l.add(&Do{X: &Call{Fn: "q_settle_elems", Args: []Expr{
			boxed(l.readVar(s.Sym)),
			&Const{R: Rep("const QElemType*"), Text: desc},
			&Const{R: RepStr, Text: cstr(l.names.Display(s.Sym.Name))},
		}, R: RepValue}})
	}
	var hoisted []*ast.TreeNode
	hoisted = append(hoisted, block)
	for _, child := range block.Children {
		if child != nil && child.NodeType == ast.ModuleNode && len(child.Children) >= 2 {
			hoisted = append(hoisted, child.Children[1])
		}
	}
	l.hoistScopes(hoisted...)
	for _, child := range block.Children {
		if child != nil && child.NodeType == ast.ModuleNode {
			l.moduleBindings(child)
		}
	}
	last := lastRunnable(block.Children)
	for _, child := range block.Children {
		if child == nil || skippedStmt(child) {
			continue
		}
		l.setLoc(child)
		if child == last && l.showable(child) {
			v := l.expr(child)
			l.add(&Do{X: &Call{Fn: "q_print", Args: []Expr{boxed(v)}, R: RepValue}})
			continue
		}
		l.stmt(child)
	}
}

// externSymbols is the C++ names of extern fns declared in this input,
// including a module the input included. The header is part of this input,
// so the wrappers are compiled here.
func externSymbols(block *ast.TreeNode) map[string]bool {
	syms := map[string]bool{}
	var walk func(*ast.TreeNode)
	walk = func(n *ast.TreeNode) {
		if n == nil {
			return
		}
		if n.NodeType == ast.ExternFnNode && n.ExternSymbol != "" {
			syms[n.ExternSymbol] = true
		}
		for _, c := range n.Children {
			walk(c)
		}
	}
	walk(block)
	return syms
}

func (l *lowerer) externShells() {
	p := l.prog.Session
	for _, f := range l.an.Session.Fns {
		if f == nil || f.Lambda == nil {
			continue
		}
		if _, defined := l.fnByLambda[f.Lambda]; defined {
			continue
		}
		if !f.TopLevel || f.Captures {
			continue
		}
		fd := &fnDecl{name: f.CName, lambda: f.Lambda, display: l.names.Display(f.Name), native: f.Native, exported: true}
		l.funcShell(fd)
		fd.fn.Session = nil
		fd.fn.Imported = true
		if fd.thunk != nil {
			fd.thunk.Session = nil
			fd.thunk.Imported = true
		}
		p.Extern = append(p.Extern, fd.fn)
		if fd.thunk != nil {
			p.Extern = append(p.Extern, fd.thunk)
		}
		n := 0
		if f.Type != nil {
			n = len(f.Type.ParamTypes)
		}
		min, max := paramArity(lambdaParams(f.Lambda), n)
		info := FnInfo{CName: fnInfoName(f.CName), Display: l.names.Display(f.Name), Min: min, Max: max, Exported: true}
		if min < max {
			info.DefaultsFn = defaultArgFnName(f.CName)
			p.ExternDefaults = append(p.ExternDefaults, info.DefaultsFn)
		}
		p.ExternInfos = append(p.ExternInfos, info)
	}
}

func (l *lowerer) clearSlot(sym *types.Symbol) {
	fn := "q_repl_cell_clear"
	switch sym.Storage {
	case ir.StorageInt:
		fn = "q_repl_i64_clear"
	case ir.StorageFloat:
		fn = "q_repl_f64_clear"
	case ir.StorageBool:
		fn = "q_repl_bool_clear"
	}
	l.add(&Do{X: &Call{Fn: fn, Args: []Expr{&Const{R: RepInt, Text: strconv.Itoa(sym.Slot)}}, R: RepValue}})
}

func (l *lowerer) sessionBinds(n *ast.TreeNode) []*types.Symbol {
	seen := map[*types.Symbol]bool{}
	var out []*types.Symbol
	var walk func(*ast.TreeNode)
	walk = func(n *ast.TreeNode) {
		if n == nil {
			return
		}
		if n.NodeType == ast.IdentifierNode {
			if s := l.an.Refs[n]; s != nil && s.Session && !seen[s] {
				seen[s] = true
				out = append(out, s)
			}
		}
		for _, c := range n.Children {
			walk(c)
		}
		walk(n.ReturnType)
		walk(n.DefaultValue)
	}
	walk(n)
	return out
}

func (l *lowerer) showable(n *ast.TreeNode) bool {
	if n == nil || skippedStmt(n) {
		return false
	}
	switch n.NodeType {
	case ast.IfStatementNode, ast.WhenStatementNode, ast.ForLoopNode, ast.WhileLoopNode,
		ast.AssignNode, ast.VarDeclNode, ast.BlockNode, ast.BreakNode, ast.ContinueNode:
		return false
	}
	t := l.an.NodeTypes[n]
	return t != nil && !t.Equals(types.TypeVoid) && !types.IsErrorType(t)
}

func skippedStmt(n *ast.TreeNode) bool {
	if n == nil {
		return true
	}
	switch n.NodeType {
	case ast.ModuleNode, ast.ExternHeaderNode, ast.ExternLinkNode, ast.ExternIncludePathNode,
		ast.ExternSourceNode, ast.ExternFnNode, ast.StructDefNode, ast.FnDeclNode, ast.InclNode,
		ast.InputBlockNode:
		return true
	}
	return false
}

func lastRunnable(stmts []*ast.TreeNode) *ast.TreeNode {
	for i := len(stmts) - 1; i >= 0; i-- {
		if stmts[i] != nil && !skippedStmt(stmts[i]) {
			return stmts[i]
		}
	}
	return nil
}

func inputNumber(file string) int {
	file = strings.TrimPrefix(file, "<input ")
	file = strings.TrimSuffix(file, ">")
	n, _ := strconv.Atoi(file)
	if n <= 0 {
		return 1
	}
	return n
}

func slotCounts(slots []*types.Symbol) (ni, nf, nb, nc int) {
	for _, s := range slots {
		if s == nil || !s.Session {
			continue
		}
		n := s.Slot + 1
		switch s.Storage {
		case ir.StorageInt:
			if n > ni {
				ni = n
			}
		case ir.StorageFloat:
			if n > nf {
				nf = n
			}
		case ir.StorageBool:
			if n > nb {
				nb = n
			}
		default:
			if n > nc {
				nc = n
			}
		}
	}
	return
}
