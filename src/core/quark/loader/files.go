package loader

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"quark/ast"
	"quark/diagnostics"
	"quark/lexer"
	"quark/names"
	"quark/parser"
	"quark/token"
)

// srcFile is one loaded .qrk file and its top-level scope.
type srcFile struct {
	info    *names.File
	ast     *ast.TreeNode
	imports []*fileImport // in source order

	top      map[string]*def // top-level names: definitions, modules, import bindings
	topOrder []*def
}

type fileImport struct {
	name string // binding: the `as` name or the file's stem
	path string // as written
	file *srcFile
	node *ast.TreeNode
}

type defKind int

const (
	kFunc defKind = iota
	kStruct
	kExtern
	kValue // a constant of an imported file, or a main-script variable
	kModule
	kImport
)

func (k defKind) describe() string {
	switch k {
	case kFunc:
		return "function"
	case kStruct:
		return "struct"
	case kExtern:
		return "extern fn"
	case kValue:
		return "constant"
	case kModule:
		return "module"
	case kImport:
		return "import"
	}
	return "name"
}

func (k defKind) nameKind() names.Kind {
	switch k {
	case kStruct:
		return names.Struct
	case kExtern:
		return names.Extern
	case kValue:
		return names.Value
	case kModule:
		return names.Module
	}
	return names.Func
}

// describe names d's kind for messages.
func (d *def) describe() string {
	if d.kind == kValue && d.file.info.Main {
		return "variable"
	}
	return d.kind.describe()
}

// def is one name in a file's top-level scope or a module body.
type def struct {
	name     string
	internal string
	kind     defKind
	file     *srcFile
	node     *ast.TreeNode // the defining statement
	line     int
	module   *modScope     // enclosing module, for members
	value    *ast.TreeNode // constant expression, for constants of imported files
	imp      *fileImport   // for kImport
	mod      *modScope     // for kModule
}

// constant reports whether references to d are replaced by its value.
func (d *def) constant() bool {
	return d.kind == kValue && !d.file.info.Main
}

// qualified is how d is written from outside its file (geo::Point).
func (d *def) qualified() string {
	o := &names.Origin{File: d.file.info, Name: d.name}
	if d.module != nil {
		o.Module = d.module.name
	}
	return o.Qualified()
}

type modScope struct {
	name     string
	internal string
	members  map[string]*def
	node     *ast.TreeNode
	line     int
}

// errIn reports a load error located at node in file f.
func (ml *ModuleLoader) errIn(f *srcFile, node *ast.TreeNode, format string, args ...interface{}) {
	loc := &diagnostics.Location{}
	if f != nil && !f.info.Main {
		loc.File = f.info.Display
	}
	if node != nil && node.Token != nil {
		loc.Line, loc.Column = node.Token.Line, node.Token.Column
	}
	ml.errors = append(ml.errors, diagnostics.Diagnostic{
		Code:     "QK-LOAD-001",
		Stage:    diagnostics.StageLoad,
		Severity: diagnostics.SeverityError,
		Message:  fmt.Sprintf(format, args...),
		Location: loc,
	})
}

// Names returns the table of internal names built by ResolveImports.
func (ml *ModuleLoader) Names() names.Table {
	return ml.names
}

// ResolveImports loads every file root imports (directly and transitively),
// resolves all names, and replaces root's statements with the statements of
// all files, imported files first. currentFilePath is the file root came from.
func (ml *ModuleLoader) ResolveImports(root *ast.TreeNode, currentFilePath string) {
	absPath, err := filepath.Abs(currentFilePath)
	if err != nil {
		ml.addErrorAt(0, 0, "cannot resolve path for '%s': %s", currentFilePath, err)
		return
	}
	if ml.root == "" {
		ml.root = absPath
	}
	main := &srcFile{
		info: &names.File{ID: 0, Path: absPath, Stem: fileStem(absPath), Display: filepath.Base(absPath), Main: true},
		ast:  root,
	}
	ml.files = append(ml.files, main)
	ml.byPath[realPath(absPath)] = main

	ml.beginResolve(absPath)
	ml.loadImports(main)
	ml.endResolve(absPath)
	if len(ml.errors) > 0 {
		return
	}
	for _, f := range ml.files {
		ml.checkReservedNames(f)
	}
	if len(ml.errors) > 0 {
		return
	}
	for _, f := range ml.files {
		ml.collect(f)
	}
	if len(ml.errors) > 0 {
		return
	}
	ml.mainNames = mainScriptNames(main)
	for _, f := range ml.files {
		ml.rename(f)
	}
	if len(ml.errors) > 0 {
		return
	}
	ml.splice(main)
}

func fileStem(path string) string {
	return strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
}

// realPath is a file's identity (rule 6): its absolute path with symbolic
// links resolved, so one file reached through a link and through its own
// path is loaded once. A path that doesn't exist is its own identity; loading
// it reports the missing file.
func realPath(absPath string) string {
	if r, err := filepath.EvalSymlinks(absPath); err == nil {
		return r
	}
	return absPath
}

// loadImports processes the top-level statements of f: extern directives and
// headers, and `incl`, loading each imported file once.
func (ml *ModuleLoader) loadImports(f *srcFile) {
	path := f.info.Path
	currentDir := filepath.Dir(path)
	ml.checkNestedDirectives(f.ast, path)
	ml.checkNestedIncl(f)

	for _, child := range f.ast.Children {
		switch {
		case child.NodeType == ast.ExternLinkNode || child.NodeType == ast.ExternIncludePathNode:
			ml.recordExternDirective(child, path)
			continue
		case child.NodeType == ast.ExternHeaderNode:
			ml.resolveExternHeader(f, child)
			continue
		case child.NodeType != ast.InclNode || len(child.Children) == 0:
			continue
		}

		arg := child.Children[0]
		if arg.NodeType != ast.LiteralNode || arg.Token == nil || arg.Token.Type != token.STRING {
			name := arg.TokenLiteral()
			ml.errIn(f, child, "incl %s: a module in the same file needs no incl; write %s::name directly", name, name)
			continue
		}
		importPath := arg.Token.Literal
		alias := ""
		if len(child.Children) >= 2 && child.Children[1] != nil && child.Children[1].NodeType == ast.IdentifierNode {
			alias = child.Children[1].TokenLiteral()
		}

		var resolvedPath string
		switch {
		case isFileImportPath(importPath):
			resolvedPath = normalizeImportPath(currentDir, importPath)
		case isStdlibImportPath(importPath):
			stdResolved, err := normalizeStdlibImportPath(importPath, path)
			if err != nil {
				ml.errIn(f, child, "%s", err)
				continue
			}
			resolvedPath = stdResolved
		default:
			ml.errIn(f, child, "unsupported import path '%s'; use relative, absolute, or std/... path", importPath)
			continue
		}
		absResolved, err := filepath.Abs(resolvedPath)
		if err != nil {
			ml.errIn(f, child, "cannot resolve import path '%s': %s", importPath, err)
			continue
		}
		if msg := checkImportPathNames(importPath, absResolved); msg != "" {
			ml.errIn(f, child, "%s", msg)
			continue
		}
		if idx, inProgress := ml.resolving[realPath(absResolved)]; inProgress {
			chain := append(append([]string{}, ml.stack[idx:]...), absResolved)
			ml.errIn(f, child, "circular import detected: %s", formatImportChain(chain))
			continue
		}

		g := ml.byPath[realPath(absResolved)]
		if g == nil {
			g = ml.loadFile(f, child, importPath, absResolved)
			if g == nil {
				continue
			}
		}
		ml.fileImports[path] = append(ml.fileImports[path], g.info.Path)
		// The binding is the stem of the path as written (rule 3), which
		// differs from g's stem when g was first loaded through a link.
		binding := alias
		if binding == "" {
			binding = fileStem(absResolved)
		}
		f.imports = append(f.imports, &fileImport{name: binding, path: importPath, file: g, node: child})
	}
}

// loadFile reads, parses and checks an imported file, then its own imports.
func (ml *ModuleLoader) loadFile(from *srcFile, inclNode *ast.TreeNode, importPath, absPath string) *srcFile {
	if _, err := os.Stat(absPath); os.IsNotExist(err) {
		ml.errIn(from, inclNode, "cannot find module '%s': file '%s' does not exist", importPath, absPath)
		return nil
	}
	content, err := os.ReadFile(absPath)
	if err != nil {
		ml.errIn(from, inclNode, "cannot read '%s': %s", absPath, err)
		return nil
	}
	p := parser.New(lexer.New(string(content)).Tokenize())
	tree := p.Parse()
	if len(p.Errors()) > 0 {
		for _, pErr := range p.Errors() {
			ml.errIn(from, inclNode, "in '%s': %s", importPath, pErr)
		}
		return nil
	}
	display := moduleDisplayName(importPath, absPath)
	ast.SetSourceFile(tree, display)
	g := &srcFile{
		info: &names.File{ID: len(ml.files), Path: absPath, Stem: fileStem(absPath), Display: display},
		ast:  tree,
	}
	ml.files = append(ml.files, g)
	ml.byPath[realPath(absPath)] = g
	ml.checkDefinitionsOnly(g)

	ml.beginResolve(absPath)
	ml.loadImports(g)
	ml.endResolve(absPath)
	return g
}

// resolveExternHeader rewrites an `extern 'path'` header to its absolute path.
func (ml *ModuleLoader) resolveExternHeader(f *srcFile, child *ast.TreeNode) {
	if len(child.Children) == 0 {
		return
	}
	pathNode := child.Children[0]
	if pathNode == nil || pathNode.Token == nil || pathNode.Token.Type != token.STRING {
		return
	}
	importPath := pathNode.Token.Literal
	var resolvedPath string
	if isStdlibImportPath(importPath) {
		stdResolved, err := normalizeStdlibImportPath(importPath, f.info.Path)
		if err != nil {
			ml.errIn(f, child, "%s", err)
			return
		}
		resolvedPath = stdResolved
	} else {
		resolvedPath = normalizeExternPath(filepath.Dir(f.info.Path), importPath)
	}
	absResolved, err := filepath.Abs(resolvedPath)
	if err != nil {
		ml.errIn(f, child, "cannot resolve extern path '%s': %s", importPath, err)
		return
	}
	if !ml.seenHeaders[absResolved] {
		ml.seenHeaders[absResolved] = true
		ml.headers = append(ml.headers, absResolved)
	}
	newTok := *pathNode.Token
	newTok.Literal = absResolved
	child.Children[0] = ast.NewNode(ast.LiteralNode, &newTok)
}

// checkNestedIncl reports `incl` anywhere but the top level of a file.
func (ml *ModuleLoader) checkNestedIncl(f *srcFile) {
	var walk func(*ast.TreeNode)
	walk = func(n *ast.TreeNode) {
		if n == nil {
			return
		}
		if n.NodeType == ast.InclNode {
			ml.errIn(f, n, "incl must be at the top level of a file, not inside a block or module")
			return
		}
		for _, c := range n.Children {
			walk(c)
		}
	}
	for _, c := range f.ast.Children {
		if c.NodeType != ast.InclNode {
			walk(c)
		}
	}
}

var validFileName = regexp.MustCompile(`^[a-z][a-z0-9_]*$`)

// checkImportPathNames enforces rule 1: an imported file's name, and each
// directory written in a relative or std/ import path, is lowercase letters,
// digits and underscores, starting with a letter.
func checkImportPathNames(importPath, absPath string) string {
	stem := fileStem(absPath)
	if !validFileName.MatchString(stem) {
		return fmt.Sprintf("module file name '%s' must be lowercase letters, digits and underscores (e.g. %s)", stem, suggestFileName(stem))
	}
	if filepath.IsAbs(importPath) || strings.HasPrefix(importPath, "/") {
		return ""
	}
	segs := strings.Split(filepath.ToSlash(strings.TrimPrefix(importPath, "std/")), "/")
	for _, seg := range segs[:len(segs)-1] {
		if seg == "." || seg == ".." || seg == "" {
			continue
		}
		if !validFileName.MatchString(seg) {
			return fmt.Sprintf("directory name '%s' in import path '%s' must be lowercase letters, digits and underscores (e.g. %s)", seg, importPath, suggestFileName(seg))
		}
	}
	return ""
}

func suggestFileName(s string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(s) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '_' {
			b.WriteRune(r)
		} else {
			b.WriteByte('_')
		}
	}
	out := b.String()
	if out == "" || out[0] < 'a' || out[0] > 'z' {
		out = "m" + out
	}
	return out
}

// checkDefinitionsOnly enforces rule 15: an imported file only contains
// definitions, at the top level and in module bodies, so importing it never
// runs code.
func (ml *ModuleLoader) checkDefinitionsOnly(f *srcFile) {
	file := filepath.Base(f.info.Path)
	var check func(stmts []*ast.TreeNode, inModule bool)
	check = func(stmts []*ast.TreeNode, inModule bool) {
		for _, stmt := range stmts {
			if stmt == nil {
				continue
			}
			switch stmt.NodeType {
			case ast.StructDefNode, ast.ExternHeaderNode, ast.ExternFnNode, ast.ExternLinkNode, ast.ExternIncludePathNode:
				continue
			case ast.InclNode:
				continue // placement is checked by checkNestedIncl
			case ast.ModuleNode:
				if !inModule && len(stmt.Children) >= 2 && stmt.Children[1] != nil {
					check(stmt.Children[1].Children, true)
					continue
				}
			case ast.OperatorNode:
				if stmt.FnDecl {
					continue
				}
				if stmt.Token != nil && stmt.Token.Type == token.EQUALS && len(stmt.Children) == 2 &&
					stmt.Children[0] != nil && stmt.Children[0].NodeType == ast.IdentifierNode {
					if isConstantValue(stmt.Children[1]) {
						continue
					}
					ml.errIn(f, stmt, "'%s' is imported, so it can only contain definitions; line %d assigns '%s' a value that isn't a constant (an int, float, str or bool literal, or arithmetic on them)",
						file, lineOf(stmt), stmt.Children[0].TokenLiteral())
					continue
				}
			}
			ml.errIn(f, stmt, "'%s' is imported, so it can only contain definitions; line %d is a statement (%s)", file, lineOf(stmt), describeStatement(stmt))
		}
	}
	check(f.ast.Children, false)
}

// isConstantValue reports whether n is a constant an imported file may
// define: an int, float, str or bool literal, unary minus on a number, or
// + - * / % over those.
func isConstantValue(n *ast.TreeNode) bool {
	if n == nil {
		return false
	}
	switch n.NodeType {
	case ast.LiteralNode:
		if n.Token == nil {
			return false
		}
		switch n.Token.Type {
		case token.INT, token.FLOAT, token.STRING, token.TRUE, token.FALSE:
			return true
		}
		return false
	case ast.OperatorNode:
		if n.Token == nil {
			return false
		}
		if len(n.Children) == 1 && n.Token.Type == token.MINUS {
			c := n.Children[0]
			return c != nil && c.NodeType == ast.LiteralNode && c.Token != nil && (c.Token.Type == token.INT || c.Token.Type == token.FLOAT)
		}
		if len(n.Children) == 2 {
			switch n.Token.Type {
			case token.PLUS, token.MINUS, token.MULTIPLY, token.DIVIDE, token.MODULO:
				return isConstantValue(n.Children[0]) && isConstantValue(n.Children[1])
			}
		}
	}
	return false
}

// describeStatement is a short rendering of a statement for messages.
func describeStatement(n *ast.TreeNode) string {
	switch n.NodeType {
	case ast.FunctionCallNode:
		if len(n.Children) > 0 && n.Children[0] != nil {
			if name := n.Children[0].TokenLiteral(); name != "" && n.Children[0].NodeType != ast.OperatorNode {
				return name + "(...)"
			}
		}
		return "a call"
	case ast.PipeNode:
		return "a pipe"
	case ast.IfStatementNode:
		return "if"
	case ast.WhenStatementNode:
		return "when"
	case ast.ForLoopNode:
		return "for"
	case ast.WhileLoopNode:
		return "while"
	case ast.VarDeclNode:
		return n.TokenLiteral() + ": ..."
	case ast.ModuleNode:
		return "a module inside a module"
	}
	if lit := n.TokenLiteral(); lit != "" {
		return lit
	}
	return "an expression"
}

func lineOf(n *ast.TreeNode) int {
	if n != nil && n.Token != nil {
		return n.Token.Line
	}
	return 0
}

// isFnDecl reports whether stmt is a `fn name(...)` declaration.
func isFnDecl(stmt *ast.TreeNode) bool {
	return stmt != nil && stmt.FnDecl && len(stmt.Children) == 2 && stmt.Children[0] != nil && stmt.Children[0].NodeType == ast.IdentifierNode
}

// assignedName is the identifier a plain `name = value` statement assigns.
func assignedName(stmt *ast.TreeNode) (*ast.TreeNode, bool) {
	if stmt == nil || stmt.FnDecl || stmt.NodeType != ast.OperatorNode || stmt.Token == nil || stmt.Token.Type != token.EQUALS ||
		len(stmt.Children) != 2 || stmt.Children[0] == nil || stmt.Children[0].NodeType != ast.IdentifierNode {
		return nil, false
	}
	return stmt.Children[0], true
}

// collect builds f's top-level scope and module scopes, assigns internal
// names, and reports the clash rules (18 and 14).
func (ml *ModuleLoader) collect(f *srcFile) {
	f.top = map[string]*def{}
	prefix := ""
	if !f.info.Main {
		prefix = fmt.Sprintf("q%d_%s__", f.info.ID, f.info.Stem)
	}

	addTop := func(d *def) {
		prev, seen := f.top[d.name]
		if !seen {
			f.top[d.name] = d
			f.topOrder = append(f.topOrder, d)
			return
		}
		switch {
		case prev.kind == kValue && d.kind == kValue && f.info.Main:
			return // a main-script variable assigned again
		case f.info.Main && ((prev.kind == kValue && d.kind == kFunc) || (prev.kind == kFunc && d.kind == kValue)):
			return // the checker reports the reassigned declaration
		case d.kind == kImport:
			ml.errIn(f, d.node, "import '%s' (line %d) conflicts with the %s '%s' defined on line %d; incl '%s' as other_name",
				d.name, d.line, prev.describe(), prev.name, prev.line, d.imp.path)
		case prev.kind == kImport:
			ml.errIn(f, d.node, "the %s '%s' (line %d) conflicts with import '%s' on line %d; incl '%s' as other_name",
				d.describe(), d.name, d.line, prev.name, prev.line, prev.imp.path)
		case prev.kind == kModule && d.kind == kModule:
			ml.errIn(f, d.node, "module '%s' is already defined on line %d", d.name, prev.line)
		default:
			ml.errIn(f, d.node, "'%s' is already defined on line %d", d.name, prev.line)
		}
	}

	// Import bindings, then definitions in source order (an import binding
	// clashing with a definition is reported at the definition or import,
	// whichever comes second).
	var defs []*def
	for _, imp := range f.imports {
		defs = append(defs, &def{name: imp.name, kind: kImport, file: f, node: imp.node, line: lineOf(imp.node), imp: imp})
	}
	for _, stmt := range f.ast.Children {
		if d := ml.topDef(f, stmt, prefix); d != nil {
			defs = append(defs, d)
		}
	}
	// Keep source order across both kinds.
	for i := 1; i < len(defs); i++ {
		for j := i; j > 0 && defs[j].line < defs[j-1].line; j-- {
			defs[j], defs[j-1] = defs[j-1], defs[j]
		}
	}
	for _, d := range defs {
		addTop(d)
	}

	// Module members.
	for _, d := range f.topOrder {
		if d.kind != kModule {
			continue
		}
		m := d.mod
		body := m.node.Children[1]
		for _, stmt := range body.Children {
			if stmt != nil && stmt.NodeType == ast.ModuleNode {
				// Imported files report this in checkDefinitionsOnly.
				if f.info.Main {
					ml.errIn(f, stmt, "module '%s' is inside module '%s'; modules can't be nested, so define it at the top level of the file",
						stmt.Children[0].TokenLiteral(), m.name)
				}
				continue
			}
			md := ml.memberDef(f, m, stmt)
			if md == nil {
				continue
			}
			if prev, seen := m.members[md.name]; seen {
				if prev.kind == kValue && md.kind == kValue && f.info.Main {
					continue
				}
				ml.errIn(f, stmt, "'%s' is already defined on line %d", md.name, prev.line)
				continue
			}
			if top, clash := f.top[md.name]; clash {
				ml.errIn(f, stmt, "module member '%s::%s' has the same name as the top-level %s '%s' on line %d",
					m.name, md.name, top.describe(), top.name, top.line)
				continue
			}
			m.members[md.name] = md
		}
	}

	for _, d := range f.topOrder {
		ml.record(d)
		if d.kind == kModule {
			for _, md := range d.mod.members {
				ml.record(md)
			}
		}
	}
}

// record adds a renamed definition to the name table.
func (ml *ModuleLoader) record(d *def) {
	if d.kind == kImport || d.internal == d.name {
		return
	}
	o := &names.Origin{File: d.file.info, Name: d.name, Kind: d.kind.nameKind(), Def: d.node}
	if d.module != nil {
		o.Module = d.module.name
	}
	if prev, taken := ml.names[d.internal]; taken && prev.Def != d.node {
		ml.reportInternalClash(d, o, prev)
		return
	}
	ml.names[d.internal] = o
}

// reportInternalClash reports two definitions that would get the same
// internal name. Internal names join parts with "__", which user names can't
// contain, but a part may start or end with "_": member `_x` of module `m`
// and member `x` of module `m_` both become `m___x`, and a main-script
// `module q1_a` gives its members the names of the first imported file
// `a.qrk`. These are rare, so they are an error rather than a more complex
// naming scheme; they must never silently share one definition (rule 6).
func (ml *ModuleLoader) reportInternalClash(d *def, o, prev *names.Origin) {
	// Report it in the main script when one of the two is there, since that
	// is the file the user is writing.
	at, atOrigin, other := d.file, o, prev
	if prev.File.Main && !o.File.Main {
		at, atOrigin, other = ml.byPath[realPath(prev.File.Path)], prev, o
	}
	where := fmt.Sprintf("line %d", lineOf(other.Def))
	if other.File != atOrigin.File {
		where = fmt.Sprintf("%s line %d", other.File.Display, lineOf(other.Def))
	}
	ml.errIn(at, atOrigin.Def, "'%s' and '%s' (%s) can't both be defined: the compiler would give them the same internal name; rename one of them",
		atOrigin.Qualified(), other.Qualified(), where)
}

// topDef is the definition a top-level statement makes, if any.
func (ml *ModuleLoader) topDef(f *srcFile, stmt *ast.TreeNode, prefix string) *def {
	if stmt == nil {
		return nil
	}
	d := &def{file: f, node: stmt, line: lineOf(stmt)}
	switch {
	case isFnDecl(stmt):
		d.name, d.kind = stmt.Children[0].TokenLiteral(), kFunc
		d.line = lineOf(stmt.Children[0])
	case stmt.NodeType == ast.StructDefNode:
		d.name, d.kind = stmt.TokenLiteral(), kStruct
	case stmt.NodeType == ast.ExternFnNode:
		if stmt.ExternReceiver != "" || stmt.TokenLiteral() == "" {
			return nil // methods are per receiver type, not names
		}
		d.name, d.kind = stmt.TokenLiteral(), kExtern
	case stmt.NodeType == ast.ModuleNode:
		if len(stmt.Children) < 2 || stmt.Children[1] == nil {
			return nil
		}
		d.name, d.kind = stmt.Children[0].TokenLiteral(), kModule
		d.mod = &modScope{name: d.name, internal: prefix + d.name, members: map[string]*def{}, node: stmt, line: d.line}
	case stmt.NodeType == ast.VarDeclNode && f.info.Main:
		d.name, d.kind = stmt.TokenLiteral(), kValue
	default:
		target, ok := assignedName(stmt)
		if !ok {
			return nil
		}
		d.name, d.kind = target.TokenLiteral(), kValue
		if !f.info.Main {
			d.value = stmt.Children[1]
		}
	}
	if d.name == "" {
		return nil
	}
	d.internal = prefix + d.name
	if f.info.Main {
		d.internal = d.name
	}
	if d.kind == kModule {
		d.internal = d.mod.internal
	}
	return d
}

// memberDef is the definition a module-body statement makes, if any.
func (ml *ModuleLoader) memberDef(f *srcFile, m *modScope, stmt *ast.TreeNode) *def {
	if stmt == nil {
		return nil
	}
	d := &def{file: f, node: stmt, line: lineOf(stmt), module: m}
	switch {
	case isFnDecl(stmt):
		d.name, d.kind = stmt.Children[0].TokenLiteral(), kFunc
		d.line = lineOf(stmt.Children[0])
	case stmt.NodeType == ast.StructDefNode:
		d.name, d.kind = stmt.TokenLiteral(), kStruct
	case stmt.NodeType == ast.ExternFnNode:
		if stmt.ExternReceiver != "" || stmt.TokenLiteral() == "" {
			return nil
		}
		d.name, d.kind = stmt.TokenLiteral(), kExtern
	case stmt.NodeType == ast.VarDeclNode && f.info.Main:
		d.name, d.kind = stmt.TokenLiteral(), kValue
	default:
		target, ok := assignedName(stmt)
		if !ok {
			return nil
		}
		d.name, d.kind = target.TokenLiteral(), kValue
		if !f.info.Main {
			d.value = stmt.Children[1]
		}
	}
	if d.name == "" {
		return nil
	}
	d.internal = m.internal + MemberSep + d.name
	return d
}

// checkReservedNames rejects user names containing "__". Every name the
// compiler generates contains it (q1_geo__area, mathx__square, f__thunk,
// helper__2), so a user name can never reach or clash with one.
func (ml *ModuleLoader) checkReservedNames(f *srcFile) {
	check := func(n *ast.TreeNode, name string) {
		if strings.Contains(name, "__") {
			ml.errIn(f, n, "'%s': names can't contain '__', which is reserved for names the compiler generates", name)
		}
	}
	var walk func(*ast.TreeNode)
	walk = func(n *ast.TreeNode) {
		if n == nil {
			return
		}
		switch n.NodeType {
		case ast.IdentifierNode, ast.StructDefNode, ast.ExternFnNode:
			check(n, n.TokenLiteral())
		case ast.QualifiedNameNode, ast.StructLiteralNode, ast.TypeNode:
			for _, part := range strings.Split(n.TokenLiteral(), "::") {
				if strings.Contains(part, "__") {
					check(n, n.TokenLiteral())
					break
				}
			}
		}
		for _, c := range n.Children {
			walk(c)
		}
		walk(n.ReturnType)
		walk(n.DefaultValue)
	}
	walk(f.ast)
}
