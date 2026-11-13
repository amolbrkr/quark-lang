package loader

import (
	"fmt"
	"sort"
	"strings"

	"quark/ast"
	"quark/builtins"
	"quark/names"
)

// MemberSep joins a module's internal name and a member name into the
// member's internal name: member `square` of main-script module `mathx` is
// `mathx__square`; member `show_vec` of std/fmt's module fmt (file 2) is
// `q2_fmt__fmt__show_vec`.
const MemberSep = "__"

// rctx is the naming context of a node being renamed.
type rctx struct {
	f      *srcFile
	mod    *modScope       // enclosing module body, if any
	shadow map[string]bool // parameters, loop variables and `when` bindings
	locals map[string]bool // names assigned inside the enclosing functions
	inFn   bool
}

func (c rctx) with(shadow ...string) rctx {
	out := make(map[string]bool, len(c.shadow)+len(shadow))
	for k := range c.shadow {
		out[k] = true
	}
	for _, s := range shadow {
		if s != "" {
			out[s] = true
		}
	}
	c.shadow = out
	return c
}

// rename resolves every name in f (modules v0.2 rule 12): a local, then a
// member of the enclosing module, then a top-level name of the file, then the
// prelude. Definitions and references get internal names; `a::b` paths are
// resolved against modules and imported files; constants of imported files
// are replaced by their value.
func (ml *ModuleLoader) rename(f *srcFile) {
	c := rctx{f: f}
	for _, stmt := range f.ast.Children {
		ml.walk(stmt, c)
	}
}

// mainScriptNames is every name the main script defines or writes anywhere:
// its top-level definitions, and every identifier, parameter and typed
// declaration in it. Overcounting only renames more locals of imported files.
func mainScriptNames(main *srcFile) map[string]bool {
	out := map[string]bool{}
	for name := range main.top {
		out[name] = true
	}
	var walk func(*ast.TreeNode)
	walk = func(n *ast.TreeNode) {
		if n == nil {
			return
		}
		switch n.NodeType {
		case ast.IdentifierNode, ast.ParameterNode, ast.VarDeclNode:
			out[n.TokenLiteral()] = true
		}
		for _, c := range n.Children {
			walk(c)
		}
	}
	walk(main.ast)
	return out
}

// renameLocal gives a local of an imported file (a parameter, loop
// variable, `when` binding, or a name assigned in a function) its own
// internal name when the main script uses the same name. Imported files are
// spliced into the main script's tree, where the main script's top-level
// names stay bare, so without this an importer's `fn total` would be visible
// to a library function's local `total` (rule 12: nothing from another file
// is visible unqualified). The caller has established that n is a local.
//
// The name, q<N>__<name>__l, can't be produced by anything else: an imported
// file's names are q<N>_<stem>__... (a stem starts with a letter), main-script
// members have one "__", and the back end's suffixes (__thunk, __default_arg,
// __<n>) don't end in __l.
func (ml *ModuleLoader) renameLocal(n *ast.TreeNode, c rctx) {
	if n == nil || c.f.info.Main {
		return
	}
	name := n.TokenLiteral()
	if name == "" || name == "_" || !ml.mainNames[name] {
		return
	}
	internal := fmt.Sprintf("q%d__%s__l", c.f.info.ID, name)
	if _, ok := ml.names[internal]; !ok {
		ml.names[internal] = &names.Origin{File: c.f.info, Name: name, Kind: names.Local}
	}
	setLiteral(n, internal)
}

// renameParam renames a parameter's name node and the parameter node, which
// carries the same token.
func (ml *ModuleLoader) renameParam(p *ast.TreeNode, c rctx) {
	if p == nil || p.NodeType != ast.ParameterNode || len(p.Children) == 0 {
		return
	}
	ml.renameLocal(p.Children[0], c)
	if lit := p.Children[0].TokenLiteral(); lit != p.TokenLiteral() {
		setLiteral(p, lit)
	}
}

// lookupBare resolves an unqualified name that isn't shadowed.
func (c rctx) lookupBare(name string) *def {
	if c.shadow[name] {
		return nil
	}
	if c.mod != nil {
		if d := c.mod.members[name]; d != nil {
			return d
		}
	}
	return c.f.top[name]
}

func setLiteral(n *ast.TreeNode, lit string) {
	if n == nil || n.Token == nil {
		return
	}
	tok := *n.Token
	tok.Literal = lit
	n.Token = &tok
}

func (ml *ModuleLoader) walk(n *ast.TreeNode, c rctx) {
	if n == nil {
		return
	}
	switch n.NodeType {
	case ast.InclNode, ast.ExternHeaderNode, ast.ExternLinkNode, ast.ExternIncludePathNode:
		return
	case ast.ModuleNode:
		ml.walkModule(n, c)
	case ast.ExternFnNode:
		ml.walkExternFn(n, c)
	case ast.StructDefNode:
		ml.walkStructDef(n, c)
	case ast.TypeNode:
		ml.walkType(n, c)
	case ast.ParameterNode:
		for _, ch := range n.Children[min(1, len(n.Children)):] {
			ml.walkType(ch, c)
		}
		ml.walk(n.DefaultValue, c)
	case ast.IdentifierNode:
		ml.resolveIdent(n, c)
	case ast.QualifiedNameNode:
		ml.resolveQualifiedValue(n, c)
	case ast.StructLiteralNode:
		ml.walkStructLiteral(n, c)
	case ast.LambdaNode:
		ml.walkLambda(n, c)
	case ast.ForLoopNode:
		ml.walkFor(n, c)
	case ast.PatternNode:
		ml.walkPattern(n, c)
	case ast.VarDeclNode:
		ml.walkVarDecl(n, c)
	case ast.MemberNode:
		ml.walkMember(n, c)
	case ast.AssignNode, ast.FnDeclNode:
		if len(n.Children) != 2 {
			ml.walkChildren(n, c)
			return
		}
		ml.renameTarget(n.Children[0], n, c)
		ml.walk(n.Children[1], c)
	default:
		ml.walkChildren(n, c)
	}
}

func (ml *ModuleLoader) walkChildren(n *ast.TreeNode, c rctx) {
	for _, ch := range n.Children {
		ml.walk(ch, c)
	}
	ml.walk(n.DefaultValue, c)
}

func (ml *ModuleLoader) walkModule(n *ast.TreeNode, c rctx) {
	if len(n.Children) < 2 || n.Children[1] == nil {
		return
	}
	d := c.f.top[n.Children[0].TokenLiteral()]
	if d == nil || d.mod == nil || d.node != n {
		return
	}
	m := d.mod
	setLiteral(n.Children[0], m.internal)
	mc := c
	mc.mod = m
	for _, stmt := range n.Children[1].Children {
		ml.walk(stmt, mc)
	}
}

func (ml *ModuleLoader) walkExternFn(n *ast.TreeNode, c rctx) {
	if n.ExternReceiver == "" {
		if d := c.lookupBare(n.TokenLiteral()); d != nil && d.kind == kExtern && d.node == n {
			setLiteral(n, d.internal)
		}
	}
	for _, p := range n.Children {
		ml.walk(p, c)
	}
	ml.walkType(n.ReturnType, c)
}

func (ml *ModuleLoader) walkStructDef(n *ast.TreeNode, c rctx) {
	if d := c.lookupBare(n.TokenLiteral()); d != nil && d.kind == kStruct && d.node == n {
		setLiteral(n, d.internal)
	}
	for _, field := range n.Children {
		if field == nil || field.NodeType != ast.StructFieldNode {
			continue
		}
		for _, t := range field.Children {
			ml.walkType(t, c)
		}
		ml.walk(field.DefaultValue, c)
	}
}

func (ml *ModuleLoader) walkStructLiteral(n *ast.TreeNode, c rctx) {
	ml.resolveStructName(n, c, "unknown struct type '%s'")
	for _, field := range n.Children {
		if field != nil && field.NodeType == ast.StructFieldNode {
			for _, v := range field.Children[min(1, len(field.Children)):] {
				ml.walk(v, c)
			}
			continue
		}
		ml.walk(field, c)
	}
}

func (ml *ModuleLoader) walkLambda(n *ast.TreeNode, c rctx) {
	if len(n.Children) < 2 {
		return
	}
	params := paramNames(n.Children[0])
	lc := c.with(params...)
	lc.inFn = true
	lc.locals = map[string]bool{}
	for k := range c.locals {
		lc.locals[k] = true
	}
	collectAssigned(n.Children[1], lc.locals)
	for _, p := range n.Children[0].Children {
		ml.walk(p, c) // annotation and default
		ml.renameParam(p, lc)
	}
	ml.walkType(n.ReturnType, c)
	ml.walk(n.Children[1], lc)
}

func (ml *ModuleLoader) walkFor(n *ast.TreeNode, c rctx) {
	if len(n.Children) < 3 {
		ml.walkChildren(n, c)
		return
	}
	ml.walk(n.Children[1], c)
	bc := c.with(n.Children[0].TokenLiteral())
	ml.renameLocal(n.Children[0], bc)
	ml.walk(n.Children[2], bc)
}

func (ml *ModuleLoader) walkPattern(n *ast.TreeNode, c rctx) {
	armCtx := c
	for _, ch := range n.Children {
		if ch != nil && ch.NodeType == ast.ResultPatternNode && len(ch.Children) > 0 {
			armCtx = c.with(ch.Children[0].TokenLiteral())
			ml.renameLocal(ch.Children[0], armCtx)
		}
	}
	for _, ch := range n.Children {
		if ch != nil && ch.NodeType == ast.ResultPatternNode {
			continue
		}
		ml.walk(ch, armCtx)
	}
}

func (ml *ModuleLoader) walkVarDecl(n *ast.TreeNode, c rctx) {
	if len(n.Children) < 3 {
		ml.walkChildren(n, c)
		return
	}
	ml.renameTarget(n.Children[0], n, c)
	if lit := n.Children[0].TokenLiteral(); c.inFn && lit != n.TokenLiteral() {
		setLiteral(n, lit) // a renamed local; the node carries the name too
	}
	ml.walkType(n.Children[1], c)
	ml.walk(n.Children[2], c)
}

// walkMember resolves `left.name`. The right side is a field, key or method
// name, never a variable; `io.exists` on an imported file or a module is an
// error that suggests `::`.
func (ml *ModuleLoader) walkMember(n *ast.TreeNode, c rctx) {
	if len(n.Children) != 2 {
		ml.walkChildren(n, c)
		return
	}
	left := n.Children[0]
	if left != nil && left.NodeType == ast.IdentifierNode && !c.shadow[left.TokenLiteral()] {
		if d := c.lookupBare(left.TokenLiteral()); d != nil {
			member := n.Children[1].TokenLiteral()
			switch d.kind {
			case kImport:
				ml.errIn(c.f, left, "'%s' is an imported file; use %s::%s", d.name, d.name, member)
				return
			case kModule:
				ml.errIn(c.f, left, "'%s' is a module; use %s::%s", d.name, d.name, member)
				return
			}
		}
	}
	ml.walk(left, c)
}

// renameTarget handles the name an assignment or declaration writes to.
func (ml *ModuleLoader) renameTarget(target, stmt *ast.TreeNode, c rctx) {
	if target == nil {
		return
	}
	switch target.NodeType {
	case ast.IdentifierNode:
		name := target.TokenLiteral()
		d := c.lookupBare(name)
		if d == nil {
			// A local (or an undefined name the checker reports).
			if c.inFn {
				ml.renameLocal(target, c)
			}
			return
		}
		switch d.kind {
		case kImport:
			ml.errIn(c.f, target, "'%s' is an imported file (line %d) and can't be assigned; choose another name", name, d.line)
			return
		case kModule:
			ml.errIn(c.f, target, "'%s' is a module (line %d) and can't be assigned; choose another name", name, d.line)
			return
		}
		if d.constant() && d.node != stmt {
			ml.errIn(c.f, target, "'%s' is a constant (line %d) and can't be assigned", name, d.line)
			return
		}
		setLiteral(target, d.internal)
	case ast.QualifiedNameNode:
		d := ml.resolvePath(target, c)
		if d == nil {
			return
		}
		if d.file != c.f {
			ml.errIn(c.f, target, "'%s' belongs to %s and can't be assigned here", target.TokenLiteral(), d.file.info.Display)
			return
		}
		if d.constant() {
			ml.errIn(c.f, target, "'%s' is a constant (line %d) and can't be assigned", target.TokenLiteral(), d.line)
			return
		}
		target.NodeType = ast.IdentifierNode
		setLiteral(target, d.internal)
	default:
		ml.walk(target, c)
	}
}

// resolveIdent renames a bare identifier read as a value.
func (ml *ModuleLoader) resolveIdent(n *ast.TreeNode, c rctx) {
	name := n.TokenLiteral()
	if name == "_" || name == "" {
		return
	}
	if c.shadow[name] {
		ml.renameLocal(n, c)
		return
	}
	d := c.lookupBare(name)
	if d == nil {
		// Nothing from another file is visible unqualified: in an imported
		// file, a name that isn't local, defined in the file or a builtin
		// would otherwise resolve to the importer's definitions.
		if !c.f.info.Main && !c.locals[name] && !isPreludeName(name) {
			ml.errIn(c.f, n, "undefined identifier '%s'", name)
			return
		}
		if c.locals[name] {
			ml.renameLocal(n, c)
		}
		return
	}
	ml.useDef(n, d, c, name)
}

// useDef turns a value reference n into a reference to d.
func (ml *ModuleLoader) useDef(n *ast.TreeNode, d *def, c rctx, written string) {
	switch d.kind {
	case kImport:
		ml.errIn(c.f, n, "'%s' is an imported file, not a value; use %s::name", written, written)
		return
	case kModule:
		ml.errIn(c.f, n, "'%s' is a module, not a value; use %s::name", written, written)
		return
	}
	if d.constant() {
		inlineConstant(n, d.value)
		return
	}
	n.NodeType = ast.IdentifierNode
	setLiteral(n, d.internal)
}

func (ml *ModuleLoader) resolveQualifiedValue(n *ast.TreeNode, c rctx) {
	if d := ml.resolvePath(n, c); d != nil {
		ml.useDef(n, d, c, n.TokenLiteral())
	}
}

// resolvePath resolves a `::` path (n's literal) to a definition, reporting
// an error and returning nil when it doesn't name one.
func (ml *ModuleLoader) resolvePath(n *ast.TreeNode, c rctx) *def {
	path := n.TokenLiteral()
	parts := strings.Split(path, "::")
	head := parts[0]
	if c.shadow[head] || (c.lookupBare(head) == nil && c.locals[head]) {
		ml.errIn(c.f, n, "'%s' is a local variable, not a module or imported file", head)
		return nil
	}
	d := c.lookupBare(head)
	if c.mod != nil && c.mod.name == head {
		d = &def{kind: kModule, mod: c.mod, name: head}
	}
	if d == nil {
		ml.errIn(c.f, n, "unknown name '%s' in '%s': it isn't a module of this file or an imported file%s", head, path, ml.hint(head, c.f.topNames(kModule, kImport)))
		return nil
	}
	switch d.kind {
	case kModule:
		if len(parts) != 2 {
			ml.errIn(c.f, n, "'%s' has too many parts: a module member is written %s::name", path, head)
			return nil
		}
		if md := d.mod.members[parts[1]]; md != nil {
			return md
		}
		ml.errIn(c.f, n, "module '%s' has no '%s'%s", head, parts[1], ml.hint(parts[1], memberNames(d.mod)))
		return nil
	case kImport:
		g := d.imp.file
		where := d.imp.path
		switch len(parts) {
		case 2:
			name := parts[1]
			if td := g.top[name]; td != nil {
				switch td.kind {
				case kImport:
					ml.errIn(c.f, n, "imports are not re-exported: '%s' imports '%s' but '%s' doesn't define it; incl '%s' in this file", head, name, path, td.imp.path)
					return nil
				case kModule:
					ml.errIn(c.f, n, "'%s' is a module; write %s::name", path, path)
					return nil
				}
				return td
			}
			if pm := g.top[g.info.Stem]; pm != nil && pm.kind == kModule {
				if md := pm.mod.members[name]; md != nil {
					return md
				}
			}
			ml.errIn(c.f, n, "'%s' (%s) has no '%s'%s", head, where, name, ml.hint(name, g.exportedNames()))
			return nil
		case 3:
			if td := g.top[parts[1]]; td != nil {
				switch td.kind {
				case kModule:
					if md := td.mod.members[parts[2]]; md != nil {
						return md
					}
					ml.errIn(c.f, n, "module '%s::%s' has no '%s'%s", head, parts[1], parts[2], ml.hint(parts[2], memberNames(td.mod)))
					return nil
				case kImport:
					ml.errIn(c.f, n, "imports are not re-exported: '%s' imports '%s'; incl '%s' in this file", head, parts[1], td.imp.path)
					return nil
				}
			}
			ml.errIn(c.f, n, "'%s' (%s) has no module '%s'%s", head, where, parts[1], ml.hint(parts[1], g.topNames(kModule)))
			return nil
		default:
			ml.errIn(c.f, n, "'%s' has too many parts: write file::name or file::module::name", path)
			return nil
		}
	}
	ml.errIn(c.f, n, "'%s' is a %s, not a module or imported file", head, d.describe())
	return nil
}

// walkType renames a struct name used as a type annotation.
func (ml *ModuleLoader) walkType(n *ast.TreeNode, c rctx) {
	if n == nil || n.NodeType != ast.TypeNode {
		return
	}
	ml.resolveStructName(n, c, "unknown type '%s'")
}

// builtinTypeNames are the annotation types that aren't structs.
var builtinTypeNames = map[string]bool{
	"int": true, "float": true, "str": true, "bool": true, "null": true, "any": true,
	"list": true, "dict": true, "vec": true, "result": true, "resource": true, "fn": true,
}

// resolveStructName renames the struct a type annotation or struct literal
// (n's literal) names.
func (ml *ModuleLoader) resolveStructName(n *ast.TreeNode, c rctx, unknownFmt string) {
	lit := n.TokenLiteral()
	if lit == "" || builtinTypeNames[lit] {
		return
	}
	var d *def
	if strings.Contains(lit, "::") {
		// Types aren't shadowed by values, so resolve without the locals.
		tc := c
		tc.shadow, tc.locals = nil, nil
		if d = ml.resolvePath(n, tc); d == nil {
			return
		}
	} else {
		if c.mod != nil {
			d = c.mod.members[lit]
		}
		if d == nil {
			d = c.f.top[lit]
		}
		if d == nil {
			if !c.f.info.Main {
				ml.errIn(c.f, n, unknownFmt, lit)
			}
			return // the checker reports unknown names in the main script
		}
	}
	if d.kind != kStruct {
		ml.errIn(c.f, n, "'%s' is a %s, not a struct type", lit, d.describe())
		return
	}
	setLiteral(n, d.internal)
}

// inlineConstant replaces the reference n by a copy of a constant's value.
func inlineConstant(n *ast.TreeNode, value *ast.TreeNode) {
	cp := copyTree(value, n)
	file := n.SourceFile
	*n = *cp
	ast.SetSourceFile(n, file)
}

func copyTree(n, at *ast.TreeNode) *ast.TreeNode {
	if n == nil {
		return nil
	}
	cp := *n
	if n.Token != nil {
		tok := *n.Token
		if at != nil && at.Token != nil {
			tok.Line, tok.Column = at.Token.Line, at.Token.Column
		}
		cp.Token = &tok
	}
	cp.Children = make([]*ast.TreeNode, len(n.Children))
	for i, ch := range n.Children {
		cp.Children[i] = copyTree(ch, at)
	}
	return &cp
}

// collectAssigned adds the names a function body assigns or declares (not
// those of nested functions) to out.
func collectAssigned(n *ast.TreeNode, out map[string]bool) {
	if n == nil || n.NodeType == ast.LambdaNode {
		return
	}
	if (n.NodeType == ast.AssignNode || n.NodeType == ast.FnDeclNode) && len(n.Children) == 2 &&
		n.Children[0] != nil && n.Children[0].NodeType == ast.IdentifierNode {
		out[n.Children[0].TokenLiteral()] = true
	}
	if n.NodeType == ast.VarDeclNode {
		out[n.TokenLiteral()] = true
	}
	for _, ch := range n.Children {
		collectAssigned(ch, out)
	}
}

func paramNames(args *ast.TreeNode) []string {
	if args == nil {
		return nil
	}
	var out []string
	for _, p := range args.Children {
		if p == nil {
			continue
		}
		if p.NodeType == ast.ParameterNode && len(p.Children) > 0 {
			out = append(out, p.Children[0].TokenLiteral())
		} else {
			out = append(out, p.TokenLiteral())
		}
	}
	return out
}

// isPreludeName reports whether name is a builtin function.
func isPreludeName(name string) bool {
	_, ok := builtins.Lookup(name)
	return ok
}

func (f *srcFile) topNames(kinds ...defKind) []string {
	var out []string
	for _, d := range f.topOrder {
		for _, k := range kinds {
			if d.kind == k {
				out = append(out, d.name)
			}
		}
	}
	return out
}

// exportedNames are the names `file::name` can reach: top-level definitions
// and the members of the primary module.
func (f *srcFile) exportedNames() []string {
	var out []string
	for _, d := range f.topOrder {
		if d.kind != kImport {
			out = append(out, d.name)
		}
	}
	if pm := f.top[f.info.Stem]; pm != nil && pm.kind == kModule {
		out = append(out, memberNames(pm.mod)...)
	}
	return out
}

func memberNames(m *modScope) []string {
	out := make([]string, 0, len(m.members))
	for name := range m.members {
		out = append(out, name)
	}
	sort.Strings(out)
	return out
}

// hint suggests the closest candidate to name, if one is close.
func (ml *ModuleLoader) hint(name string, candidates []string) string {
	limit := max(2, len(name)/3)
	best, bestDist := "", limit+1
	for _, cand := range candidates {
		if d := editDistance(name, cand); d < bestDist {
			best, bestDist = cand, d
		}
	}
	if best == "" || best == name {
		return ""
	}
	return "; did you mean '" + best + "'?"
}

func editDistance(a, b string) int {
	prev := make([]int, len(b)+1)
	cur := make([]int, len(b)+1)
	for j := range prev {
		prev[j] = j
	}
	for i := 1; i <= len(a); i++ {
		cur[0] = i
		for j := 1; j <= len(b); j++ {
			cost := 1
			if a[i-1] == b[j-1] {
				cost = 0
			}
			cur[j] = min(prev[j]+1, cur[j-1]+1, prev[j-1]+cost)
		}
		prev, cur = cur, prev
	}
	return prev[len(b)]
}

// splice replaces the main tree's statements with every file's statements:
// imported files first (each after the files it imports), then the main
// script. `incl` statements are dropped, as are the definitions of imported
// constants, whose references were replaced by their values.
func (ml *ModuleLoader) splice(main *srcFile) {
	var order []*srcFile
	seen := map[*srcFile]bool{}
	var visit func(*srcFile)
	visit = func(f *srcFile) {
		if seen[f] {
			return
		}
		seen[f] = true
		for _, imp := range f.imports {
			visit(imp.file)
		}
		order = append(order, f)
	}
	visit(main)

	var out []*ast.TreeNode
	for _, f := range order {
		for _, stmt := range f.ast.Children {
			if keep := ml.keepStatement(f, stmt); keep {
				out = append(out, stmt)
			}
		}
	}
	main.ast.Children = out
}

func (ml *ModuleLoader) keepStatement(f *srcFile, stmt *ast.TreeNode) bool {
	if stmt == nil || stmt.NodeType == ast.InclNode {
		return false
	}
	if f.info.Main {
		return true
	}
	if _, ok := assignedName(stmt); ok {
		return false // a constant
	}
	if stmt.NodeType == ast.ModuleNode && len(stmt.Children) >= 2 && stmt.Children[1] != nil {
		body := stmt.Children[1]
		kept := body.Children[:0]
		for _, s := range body.Children {
			if _, ok := assignedName(s); !ok {
				kept = append(kept, s)
			}
		}
		body.Children = kept
	}
	return true
}
