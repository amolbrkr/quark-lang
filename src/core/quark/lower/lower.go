package lower

import (
	"fmt"
	"sort"
	"strings"

	"quark/ast"
	"quark/ir"
	"quark/names"
	"quark/types"
)

// ICE is an internal compiler error found while lowering: the analyzer's
// outputs don't describe the tree (a missing call plan or symbol).
type ICE struct {
	Code string
	Msg  string
	Line int
}

func (e *ICE) Error() string {
	return fmt.Sprintf("internal compiler error [%s]: %s at line %d", e.Code, e.Msg, e.Line)
}

// fnDecl is one function the program defines: a `fn` declaration or a
// lambda, in the order they appear in the tree.
type fnDecl struct {
	name       string // C++ name without the quark_ prefix
	lambda     *ast.TreeNode
	paramCount int
	display    string
	native     *ir.CallPlan
	fn, thunk  *Func
}

type lowerer struct {
	an         *types.Analysis
	names      names.Table
	sourceName string
	prog       *Program

	cur     *Block
	tempN   int
	lambdaN int

	fns             []*fnDecl
	fnByLambda      map[*ast.TreeNode]*fnDecl
	lambdaUserNames map[*ast.TreeNode]string

	// checkedCells are the captured variables of the function being lowered
	// that a hoisted function may read before they are assigned.
	checkedCells map[*types.Symbol]bool

	elemDescNames map[string]string
	locIndex      map[SourceLoc]int
}

// Lower lowers an analyzed, validated tree to a Program. sourceName labels
// runtime source locations of the main file.
func Lower(root *ast.TreeNode, an *types.Analysis, sourceName string) (prog *Program, err error) {
	if strings.TrimSpace(sourceName) == "" {
		sourceName = "<unknown>"
	}
	l := &lowerer{
		an:              an,
		names:           an.Names,
		sourceName:      sourceName,
		prog:            &Program{},
		fnByLambda:      map[*ast.TreeNode]*fnDecl{},
		lambdaUserNames: map[*ast.TreeNode]string{},
		elemDescNames:   map[string]string{},
		locIndex:        map[SourceLoc]int{},
	}
	defer func() {
		if r := recover(); r != nil {
			if ice, ok := r.(*ICE); ok {
				prog, err = nil, ice
				return
			}
			panic(r)
		}
	}()
	l.program(root)
	return l.prog, nil
}

func (l *lowerer) ice(code string, node *ast.TreeNode, format string, args ...interface{}) {
	line := 0
	if node != nil && node.Token != nil {
		line = node.Token.Line
	}
	panic(&ICE{Code: code, Msg: fmt.Sprintf(format, args...), Line: line})
}

// ---------------------------------------------------------------------------
// Program

func (l *lowerer) program(root *ast.TreeNode) {
	p := l.prog
	p.Includes = collectExternSources(root)
	l.collectFunctions(root)

	// Signatures first, so every body can refer to every function.
	for _, fd := range l.fns {
		l.funcShell(fd)
		p.Decls = append(p.Decls, fd.fn)
		if fd.thunk != nil {
			p.Decls = append(p.Decls, fd.thunk)
		}
	}
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

	// main: the declarations of the top level and of every module body exist
	// before its first statement runs; module bodies run before the top level.
	p.Main = &Block{}
	l.cur = p.Main
	l.checkedCells = nil
	hoisted := []*ast.TreeNode{root}
	for _, child := range root.Children {
		if child.NodeType == ast.ModuleNode && len(child.Children) >= 2 {
			hoisted = append(hoisted, child.Children[1])
		}
	}
	l.hoistScopes(hoisted...)
	for _, child := range root.Children {
		if child.NodeType == ast.ModuleNode {
			l.moduleBindings(child)
		}
	}
	for _, child := range root.Children {
		switch child.NodeType {
		case ast.ModuleNode, ast.ExternHeaderNode, ast.ExternLinkNode, ast.ExternIncludePathNode,
			ast.ExternFnNode, ast.StructDefNode, ast.FnDeclNode:
			continue
		}
		l.setLoc(child)
		l.stmt(child)
	}
}

func (l *lowerer) moduleBindings(node *ast.TreeNode) {
	if len(node.Children) < 2 {
		return
	}
	body := node.Children[1]
	if body == nil || body.NodeType != ast.BlockNode {
		return
	}
	for _, child := range body.Children {
		if child == nil || child.NodeType == ast.ModuleNode || child.NodeType == ast.FnDeclNode {
			continue
		}
		l.setLoc(child)
		l.stmt(child)
	}
}

// collectExternSources gathers the resolved paths of `extern 'x.hpp'` headers.
func collectExternSources(node *ast.TreeNode) []string {
	var paths []string
	seen := map[string]bool{}
	var walk func(*ast.TreeNode)
	walk = func(n *ast.TreeNode) {
		if n == nil {
			return
		}
		if n.NodeType == ast.ExternHeaderNode && len(n.Children) > 0 {
			if pn := n.Children[0]; pn != nil && pn.Token != nil && pn.Token.Literal != "" && !seen[pn.Token.Literal] {
				seen[pn.Token.Literal] = true
				paths = append(paths, pn.Token.Literal)
			}
			return
		}
		for _, c := range n.Children {
			walk(c)
		}
	}
	walk(node)
	return paths
}

// collectFunctions registers every function the tree defines, in source
// order: `fn` declarations under their C++ name, lambdas as __lambdaN.
func (l *lowerer) collectFunctions(node *ast.TreeNode) {
	if node == nil {
		return
	}
	switch node.NodeType {
	case ast.LambdaNode:
		if _, done := l.fnByLambda[node]; done {
			return
		}
		l.lambdaN++
		l.addFn(&fnDecl{name: ir.LambdaFnName(l.lambdaN), lambda: node, display: l.lambdaDisplayName(node)})
	case ast.AssignNode:
		if len(node.Children) == 2 && node.Children[0].NodeType == ast.IdentifierNode && node.Children[1].NodeType == ast.LambdaNode {
			l.lambdaUserNames[node.Children[1]] = node.Children[0].TokenLiteral()
		}
	case ast.FnDeclNode:
		if !ast.IsFnDecl(node) {
			break
		}
		lambda := node.Children[1]
		l.lambdaUserNames[lambda] = node.Children[0].TokenLiteral()
		if f := l.an.FixedFns[lambda]; f != nil {
			l.addFn(&fnDecl{name: f.CName, lambda: lambda, display: l.names.Display(f.Name), native: f.Native})
			for _, child := range lambda.Children {
				l.collectFunctions(child)
			}
			return
		}
	case ast.ModuleNode:
		if len(node.Children) >= 2 {
			l.collectFunctions(node.Children[1])
		}
		return
	}
	for _, child := range node.Children {
		l.collectFunctions(child)
	}
}

func (l *lowerer) addFn(fd *fnDecl) {
	if len(fd.lambda.Children) >= 1 && fd.lambda.Children[0] != nil {
		fd.paramCount = len(fd.lambda.Children[0].Children)
	}
	if fd.native == nil {
		fd.native = l.an.NativeFns[fd.name]
	}
	l.fns = append(l.fns, fd)
	l.fnByLambda[fd.lambda] = fd
}

// lambdaDisplayName names a lambda in runtime messages: the name it is bound
// to, or "lambda".
func (l *lowerer) lambdaDisplayName(node *ast.TreeNode) string {
	if name, ok := l.lambdaUserNames[node]; ok {
		return l.names.Display(name)
	}
	return "lambda"
}

func lambdaParams(node *ast.TreeNode) *ast.TreeNode {
	if node == nil || len(node.Children) == 0 {
		return nil
	}
	return node.Children[0]
}

func paramNameNode(param *ast.TreeNode) *ast.TreeNode {
	if param == nil {
		return nil
	}
	if param.NodeType == ast.ParameterNode {
		if len(param.Children) == 0 {
			return nil
		}
		return param.Children[0]
	}
	return param
}

func (l *lowerer) paramSyms(params *ast.TreeNode) []*types.Symbol {
	if params == nil {
		return nil
	}
	var syms []*types.Symbol
	for _, p := range params.Children {
		n := paramNameNode(p)
		if n == nil || n.TokenLiteral() == "" {
			continue
		}
		syms = append(syms, l.varSym(n))
	}
	return syms
}

// paramArity is the number of parameters without a default, and the total.
func paramArity(params *ast.TreeNode, count int) (int, int) {
	if params == nil {
		return count, count
	}
	required := 0
	for _, p := range params.Children {
		if p != nil && p.DefaultValue == nil {
			required++
		}
	}
	return required, len(params.Children)
}

func fnInfoName(name string) string         { return "_qfi_" + name }
func externFnInfoName(symbol string) string { return "_qfi__thunk_" + symbol }
func defaultArgFnName(name string) string   { return "quark_" + name + "__default_arg" }

// funcShell creates fd's Func (and its thunk) with its signature.
func (l *lowerer) funcShell(fd *fnDecl) {
	syms := l.paramSyms(lambdaParams(fd.lambda))
	if fd.native != nil {
		ret := fd.native.NativeReturnType
		if ret == "" {
			ret = "QValue"
		}
		f := &Func{CName: "quark_" + fd.name, Ret: ret, LocGuard: true}
		for i, sym := range syms {
			ct := "QValue"
			if i < len(fd.native.NativeParamTypes) {
				ct = fd.native.NativeParamTypes[i]
			}
			f.Params = append(f.Params, &Param{Sym: sym, CName: "_arg_" + sym.CName, CType: ct})
		}
		fd.fn = f
		thunk := &Func{CName: "quark_" + fd.name + "__thunk", Ret: "QValue"}
		for i := range syms {
			thunk.Params = append(thunk.Params, &Param{CName: fmt.Sprintf("_a%d", i), CType: "QValue"})
		}
		fd.thunk = thunk
		return
	}
	f := &Func{CName: "quark_" + fd.name, Ret: "QValue", LocGuard: true, Captures: l.an.Captures[fd.lambda]}
	for _, sym := range syms {
		f.Params = append(f.Params, &Param{Sym: sym, CName: "_arg_" + sym.CName, CType: "QValue"})
	}
	fd.fn = f
}

// externThunks are the QValue wrappers of free extern fns, so they can be
// used as function values.
func (l *lowerer) externThunks() []*Func {
	keys := make([]string, 0, len(l.an.ExternFns))
	for k, proto := range l.an.ExternFns {
		if !proto.IsMethod {
			keys = append(keys, k)
		}
	}
	sort.Strings(keys)
	var out []*Func
	seen := map[string]bool{}
	for _, k := range keys {
		proto := l.an.ExternFns[k]
		if seen[proto.RuntimeSymbol] {
			continue
		}
		seen[proto.RuntimeSymbol] = true
		f := &Func{CName: "_thunk_" + proto.RuntimeSymbol, Ret: "QValue"}
		args := make([]Expr, len(proto.NativeParamTypes))
		for i, ct := range proto.NativeParamTypes {
			p := &Param{CName: fmt.Sprintf("_a%d", i), CType: "QValue"}
			f.Params = append(f.Params, p)
			args[i] = adaptTo(&ParamRef{P: p}, RepOfCType(ct))
		}
		call := &Call{Fn: proto.RuntimeSymbol, Args: args, R: RepOfCType(proto.NativeReturnType)}
		f.Body = &Block{Stmts: []Stmt{&Return{Value: boxed(call)}}}
		out = append(out, f)
	}
	return out
}

func (l *lowerer) fnInfos() {
	seen := map[string]bool{}
	for _, fd := range l.fns {
		if seen[fd.name] {
			continue
		}
		seen[fd.name] = true
		min, max := paramArity(lambdaParams(fd.lambda), fd.paramCount)
		info := FnInfo{CName: fnInfoName(fd.name), Display: fd.display, Min: min, Max: max}
		if info.Display == "" {
			info.Display = l.names.Display(fd.name)
		}
		if min < max {
			info.DefaultsFn = defaultArgFnName(fd.name)
		}
		l.prog.FnInfos = append(l.prog.FnInfos, info)
	}
	keys := make([]string, 0, len(l.an.ExternFns))
	for k := range l.an.ExternFns {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		proto := l.an.ExternFns[k]
		if proto.IsMethod || seen["extern:"+proto.RuntimeSymbol] {
			continue
		}
		seen["extern:"+proto.RuntimeSymbol] = true
		n := len(proto.NativeParamTypes)
		l.prog.FnInfos = append(l.prog.FnInfos, FnInfo{CName: externFnInfoName(proto.RuntimeSymbol), Display: l.names.Display(k), Min: n, Max: n})
	}
}

func (l *lowerer) structDefs() {
	keys := make([]string, 0, len(l.an.StructTypes))
	for k := range l.an.StructTypes {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		st := l.an.StructTypes[k]
		d := StructDef{CName: structCName(st.Name), Display: l.names.Display(st.Name)}
		for _, f := range st.Fields {
			d.Fields = append(d.Fields, f.Name)
		}
		l.prog.Structs = append(l.prog.Structs, d)
	}
}

// structCName is the C++ name a struct's statics are built from:
// _sdef_<structCName>, _sfields_<structCName>.
func structCName(internal string) string { return "quark_" + internal }

// defaultArgFns lowers, per function with defaults, the function that builds
// an omitted default for a call through a function value (a fresh value per
// call, so an empty-list default isn't shared).
func (l *lowerer) defaultArgFns() {
	seen := map[string]bool{}
	for _, fd := range l.fns {
		params := lambdaParams(fd.lambda)
		if seen[fd.name] || params == nil {
			continue
		}
		seen[fd.name] = true
		if min, max := paramArity(params, fd.paramCount); min == max {
			continue
		}
		df := DefaultArgFn{CName: defaultArgFnName(fd.name)}
		for i, p := range params.Children {
			if p == nil || p.DefaultValue == nil {
				continue
			}
			body := &Block{}
			l.withBlock(body, func() {
				l.checkedCells = nil
				v := l.expr(p.DefaultValue)
				l.add(&Return{Value: boxed(v)})
			})
			df.Cases = append(df.Cases, DefaultCase{Index: i, Body: body})
		}
		l.prog.DefaultArgFns = append(l.prog.DefaultArgFns, df)
	}
}

// funcBody lowers fd's body into its Func (and its thunk's body).
func (l *lowerer) funcBody(fd *fnDecl) {
	f := fd.fn
	f.Body = &Block{}
	params := lambdaParams(fd.lambda)
	var body *ast.TreeNode
	if len(fd.lambda.Children) >= 2 {
		body = fd.lambda.Children[1]
	}
	l.withBlock(f.Body, func() {
		l.checkedCells = map[*types.Symbol]bool{}
		for _, sym := range f.Captures {
			if sym.Hoisted {
				l.checkedCells[sym] = true
			}
		}
		if fd.native != nil {
			for _, p := range f.Params {
				var v Expr = &ParamRef{P: p}
				sym := p.Sym
				switch {
				case sym.Storage == ir.StorageCell:
					l.add(&DeclVar{Sym: sym, Value: boxed(v)})
				case sym.Storage.IsScalar():
					l.add(&DeclVar{Sym: sym, Value: v})
				default:
					l.add(&DeclVar{Sym: sym, Value: boxed(v)})
				}
			}
			result := l.bodyValue(body)
			l.add(&Return{Value: nativeResult(result, RepOfCType(f.Ret))})
		} else {
			l.paramBindings(f, fd.display)
			_ = params
			result := l.bodyValue(body)
			l.add(&Return{Value: l.checkedReturn(fd.lambda, body, result, fd.display)})
		}
	})
	if fd.thunk != nil {
		args := []Expr{nullptrExpr}
		for i, p := range fd.thunk.Params {
			ct := "QValue"
			if i < len(fd.native.NativeParamTypes) {
				ct = fd.native.NativeParamTypes[i]
			}
			args = append(args, adaptTo(&ParamRef{P: p}, RepOfCType(ct)))
		}
		call := &Call{Fn: f.CName, Args: args, R: RepOfCType(f.Ret)}
		fd.thunk.Body = &Block{Stmts: []Stmt{&Return{Value: boxed(call)}}}
	}
}

// nativeResult converts a native function's body value to its return type:
// a raw scalar of the right kind as is (an int widened for a float), anything
// else through the checked q_as_* conversions.
func nativeResult(v Expr, want Rep) Expr {
	if v.Rep() == want && (want.IsScalar() || want == RepValue) {
		return v
	}
	if want == RepFloat && v.Rep() == RepInt {
		return &Cast{X: v, To: RepFloat}
	}
	if want == RepValue {
		return boxed(v)
	}
	return adaptTo(boxed(v), want)
}

// paramBindings binds a QValue-convention function's parameters: every typed
// parameter is checked on entry (q_param_*), scalars are unboxed into raw
// locals, untyped ones stay QValue.
func (l *lowerer) paramBindings(f *Func, fnDisplay string) {
	for _, p := range f.Params {
		sym := p.Sym
		var arg Expr = &ParamRef{P: p}
		args := []string{cstr(fnDisplay), cstr(l.names.Display(sym.Name))}
		c, checked := checkFor(sym.StoreType)
		switch {
		case sym.Storage.IsScalar():
			l.add(&DeclVar{Sym: sym, Value: c.check("param", arg, args)})
		default:
			v := arg
			if checked {
				v = boxed(c.check("param", arg, args))
			}
			l.add(&DeclVar{Sym: sym, Value: v})
		}
	}
}

// ---------------------------------------------------------------------------
// Blocks and statements

func (l *lowerer) add(s Stmt) { l.cur.Stmts = append(l.cur.Stmts, s) }

// withBlock lowers into b, restoring the current block and checked cells.
func (l *lowerer) withBlock(b *Block, fn func()) {
	saved, savedCells := l.cur, l.checkedCells
	l.cur = b
	fn()
	l.cur, l.checkedCells = saved, savedCells
}

// sub lowers fn into a new block and returns it.
func (l *lowerer) sub(fn func()) *Block {
	b := &Block{}
	saved := l.cur
	l.cur = b
	fn()
	l.cur = saved
	return b
}

func (l *lowerer) newTemp(r Rep) *Temp {
	l.tempN++
	return &Temp{ID: l.tempN, R: r}
}

// let binds v to a new temporary and returns a read of it.
func (l *lowerer) let(v Expr) Expr {
	t := l.newTemp(v.Rep())
	l.add(&Let{T: t, Value: v})
	return &TempRef{T: t}
}

// stable makes v safe to evaluate later or more than once.
func (l *lowerer) stable(v Expr) Expr {
	if isStable(v) {
		return v
	}
	return l.let(v)
}

// setLoc records node's source location before a statement. Standard library
// code keeps the caller's location, so an error inside std/... points at the
// user's call.
func (l *lowerer) setLoc(node *ast.TreeNode) {
	s := l.locOf(node)
	if s == nil {
		return
	}
	// A location nothing ran under (a hoisted `fn` declaration) is replaced.
	if n := len(l.cur.Stmts); n > 0 {
		if _, ok := l.cur.Stmts[n-1].(*SetLoc); ok {
			l.cur.Stmts[n-1] = s
			return
		}
	}
	l.add(s)
}

func (l *lowerer) locOf(node *ast.TreeNode) *SetLoc {
	if i, ok := l.locEntry(node); ok {
		return &SetLoc{Index: i}
	}
	return nil
}

// locPtr is node's location as a runtime argument (for errors reported by a
// loop test, where no statement sets it): nullptr in std/ code, which keeps
// the caller's location.
func (l *lowerer) locPtr(node *ast.TreeNode) Expr {
	if i, ok := l.locEntry(node); ok {
		return &LocRef{Index: i}
	}
	return &Const{R: RepLoc, Text: "nullptr"}
}

// locEntry is node's entry in the location table, added on first use.
func (l *lowerer) locEntry(node *ast.TreeNode) (int, bool) {
	if node == nil || node.Token == nil || isStdlibSource(node) {
		return 0, false
	}
	loc := SourceLoc{File: l.sourceFileOf(node), Line: node.Token.Line, Col: node.Token.Column}
	i, ok := l.locIndex[loc]
	if !ok {
		i = len(l.prog.Locs)
		l.locIndex[loc] = i
		l.prog.Locs = append(l.prog.Locs, loc)
	}
	return i, true
}

func isStdlibSource(node *ast.TreeNode) bool {
	return node != nil && strings.HasPrefix(node.SourceFile, "std/")
}

func (l *lowerer) sourceFileOf(node *ast.TreeNode) string {
	if node != nil && node.SourceFile != "" {
		return node.SourceFile
	}
	return l.sourceName
}

// hoistScopes creates, at the start of one or more scopes, what their `fn`
// declarations need before the first statement runs: the cells of variables
// their closures capture (unassigned until first assigned), a binding per
// capturing declaration, then the closures.
func (l *lowerer) hoistScopes(nodes ...*ast.TreeNode) {
	var scopes []*types.HoistScope
	for _, n := range nodes {
		if hs := l.an.HoistScopes[n]; hs != nil {
			scopes = append(scopes, hs)
		}
	}
	for _, hs := range scopes {
		for _, sym := range hs.Cells {
			l.add(&DeclVar{Sym: sym, Unassigned: true})
		}
		for _, f := range hs.Fns {
			if f.Captures {
				l.add(&DeclVar{Sym: f.Sym, Value: nullValue()})
			}
		}
	}
	for _, hs := range scopes {
		for _, f := range hs.Fns {
			if f.Captures {
				l.add(&SetVar{Sym: f.Sym, Value: l.lambdaValue(f.Lambda)})
			}
		}
	}
}

// bodyValue lowers a function body: a block's statements with their
// locations, or a bare expression; it returns the body's value.
func (l *lowerer) bodyValue(body *ast.TreeNode) Expr {
	if body == nil {
		return nullValue()
	}
	if body.NodeType == ast.BlockNode {
		return l.blockValue(body)
	}
	return l.expr(body)
}

// blockValue lowers a block's statements and returns the last one's value.
func (l *lowerer) blockValue(node *ast.TreeNode) Expr {
	if node == nil {
		return nullValue()
	}
	l.hoistScopes(node)
	var last Expr = nullValue()
	for i, child := range node.Children {
		l.setLoc(child)
		if i < len(node.Children)-1 {
			l.stmt(child)
		} else {
			last = l.expr(child)
		}
	}
	return last
}

// blockStmts lowers a block's statements for their effects.
func (l *lowerer) blockStmts(node *ast.TreeNode) {
	if node == nil {
		return
	}
	l.hoistScopes(node)
	for _, child := range node.Children {
		l.setLoc(child)
		l.stmt(child)
	}
}

// branch lowers an if/when branch or loop body (a block or one statement)
// for its effects.
func (l *lowerer) branch(node *ast.TreeNode) {
	if node == nil {
		return
	}
	if node.NodeType == ast.BlockNode {
		l.blockStmts(node)
		return
	}
	l.setLoc(node)
	l.stmt(node)
}

// branchValue lowers a branch and returns its value; a single-statement
// branch records its location, as in branch.
func (l *lowerer) branchValue(node *ast.TreeNode) Expr {
	if node == nil {
		return nullValue()
	}
	if node.NodeType == ast.BlockNode {
		return l.blockValue(node)
	}
	l.setLoc(node)
	return l.expr(node)
}

// stmt lowers a statement whose value is not used.
func (l *lowerer) stmt(node *ast.TreeNode) {
	if node == nil {
		return
	}
	switch node.NodeType {
	case ast.IfStatementNode:
		l.lowerIf(node, false)
	case ast.WhenStatementNode:
		l.lowerWhen(node, false)
	case ast.ForLoopNode:
		l.lowerFor(node)
	case ast.WhileLoopNode:
		l.lowerWhile(node)
	case ast.BlockNode:
		l.blockStmts(node)
	case ast.BreakNode:
		l.add(&Break{})
	case ast.ContinueNode:
		l.add(&Continue{})
	case ast.ModuleNode, ast.StructDefNode, ast.ExternHeaderNode, ast.ExternFnNode,
		ast.ExternLinkNode, ast.ExternIncludePathNode:
	case ast.FnDeclNode:
		// Hoisted: the scope's start created it.
	case ast.AssignNode, ast.VarDeclNode:
		l.expr(node)
	default:
		v := l.expr(node)
		if !isStable(v) {
			l.add(&Do{X: v})
		}
	}
}
