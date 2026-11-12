package loader

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"quark/ast"
	"quark/diagnostics"
	"quark/lexer"
	"quark/parser"
	"quark/token"
)

// ModuleLoader resolves multi-file imports by loading external .qrk files,
// parsing them, and splicing their AST into the main tree.
type ModuleLoader struct {
	loaded        map[string]bool   // absolute paths fully processed (for dedup)
	loadedModules map[string]string // absolute path -> primary module name
	resolving     map[string]int    // absolute paths currently in DFS stack (for cycle detection)
	stack         []string          // current import chain
	errors        []diagnostics.Diagnostic

	// Native dependencies from extern link / extern include_path, recorded
	// per file so NativeDeps can order libraries by the import graph.
	root         string // absolute path of the root file
	fileLibs     map[string][]NativeLib
	fileImports  map[string][]string // file -> files it imports, in source order
	includeDirs  []string            // absolute, first-seen order
	seenIncludes map[string]bool
	headers      []string // absolute extern header paths, first-seen order
	seenHeaders  map[string]bool
}

// NativeLib is one `extern link 'name'` request.
type NativeLib struct {
	Name   string // library name as passed to -l
	File   string // absolute path of the .qrk file that requested it
	Line   int
	Column int
}

// Origin returns "file:line" for diagnostics.
func (l NativeLib) Origin() string {
	return fmt.Sprintf("%s:%d", l.File, l.Line)
}

// NativeDeps are the native build inputs requested by extern directives in a
// program and everything it imports.
type NativeDeps struct {
	IncludeDirs []string    // absolute directories, deduplicated, first-seen order
	Libs        []NativeLib // deduplicated, in link order
	Headers     []string    // absolute paths of extern glue headers
}

// NewModuleLoader creates a new module loader.
func NewModuleLoader() *ModuleLoader {
	return &ModuleLoader{
		loaded:        make(map[string]bool),
		loadedModules: make(map[string]string),
		resolving:     make(map[string]int),
		stack:         make([]string, 0),
		errors:        make([]diagnostics.Diagnostic, 0),
		fileLibs:      make(map[string][]NativeLib),
		fileImports:   make(map[string][]string),
		seenIncludes:  make(map[string]bool),
		seenHeaders:   make(map[string]bool),
	}
}

// NativeDeps returns the libraries and include directories requested by
// extern directives across all loaded files.
//
// Link order: a file's libraries come before those of the files it imports,
// so each library appears after everything that needs it (static libraries
// depend on this). Files are visited in a topological order of the import
// graph; when a library is requested more than once, its last position wins.
func (ml *ModuleLoader) NativeDeps() NativeDeps {
	var postorder []string
	visited := make(map[string]bool)
	var visit func(string)
	visit = func(file string) {
		if visited[file] {
			return
		}
		visited[file] = true
		for _, dep := range ml.fileImports[file] {
			visit(dep)
		}
		postorder = append(postorder, file)
	}
	if ml.root != "" {
		visit(ml.root)
	}

	var all []NativeLib
	for i := len(postorder) - 1; i >= 0; i-- {
		all = append(all, ml.fileLibs[postorder[i]]...)
	}
	lastIndex := make(map[string]int)
	for i, lib := range all {
		lastIndex[lib.Name] = i
	}
	var libs []NativeLib
	for i, lib := range all {
		if lastIndex[lib.Name] == i {
			libs = append(libs, lib)
		}
	}
	return NativeDeps{
		IncludeDirs: append([]string(nil), ml.includeDirs...),
		Libs:        libs,
		Headers:     append([]string(nil), ml.headers...),
	}
}

// addFFIError records an extern-directive error located in file.
func (ml *ModuleLoader) addFFIError(code, file string, tok *token.Token, format string, args ...interface{}) {
	loc := &diagnostics.Location{File: file}
	if tok != nil {
		loc.Line, loc.Column = tok.Line, tok.Column
	}
	ml.errors = append(ml.errors, diagnostics.Diagnostic{
		Code:     code,
		Stage:    diagnostics.StageLoad,
		Severity: diagnostics.SeverityError,
		Message:  fmt.Sprintf(format, args...),
		Location: loc,
	})
}

// recordExternDirective handles a top-level extern link / extern
// include_path node from currentFilePath.
func (ml *ModuleLoader) recordExternDirective(node *ast.TreeNode, currentFilePath string) {
	if len(node.Children) == 0 || node.Children[0].Token == nil {
		return
	}
	arg := node.Children[0].Token.Literal
	switch node.NodeType {
	case ast.ExternLinkNode:
		lib := NativeLib{Name: arg, File: currentFilePath}
		if node.Token != nil {
			lib.Line, lib.Column = node.Token.Line, node.Token.Column
		}
		ml.fileLibs[currentFilePath] = append(ml.fileLibs[currentFilePath], lib)
	case ast.ExternIncludePathNode:
		var dir string
		if isStdlibImportPath(arg) {
			root, ok := findStdlibRoot(currentFilePath)
			if !ok {
				ml.addFFIError("QK-FFI-001", currentFilePath, node.Token, "cannot resolve extern include_path '%s': no stdlib root found (set QUARK_STDLIB_ROOT)", arg)
				return
			}
			dir = filepath.Join(root, strings.TrimPrefix(arg, "std/"))
		} else {
			dir = normalizeExternPath(filepath.Dir(currentFilePath), arg)
		}
		abs, err := filepath.Abs(dir)
		if err == nil {
			var st os.FileInfo
			if st, err = os.Stat(abs); err == nil && !st.IsDir() {
				err = fmt.Errorf("not a directory")
			}
		}
		if err != nil {
			ml.addFFIError("QK-FFI-001", currentFilePath, node.Token, "extern include_path directory '%s' not found (resolved to %s)", arg, abs)
			return
		}
		if !ml.seenIncludes[abs] {
			ml.seenIncludes[abs] = true
			ml.includeDirs = append(ml.includeDirs, abs)
		}
	}
}

// checkNestedDirectives reports extern link / include_path directives that
// are not at the top level of a file; they would otherwise be ignored.
func (ml *ModuleLoader) checkNestedDirectives(node *ast.TreeNode, currentFilePath string) {
	var walk func(*ast.TreeNode)
	walk = func(n *ast.TreeNode) {
		if n == nil {
			return
		}
		if n.NodeType == ast.ExternLinkNode || n.NodeType == ast.ExternIncludePathNode {
			word := "link"
			if n.NodeType == ast.ExternIncludePathNode {
				word = "include_path"
			}
			ml.addFFIError("QK-LOAD-001", currentFilePath, n.Token, "extern %s must be at the top level of a file, not inside a block or module", word)
			return
		}
		for _, c := range n.Children {
			walk(c)
		}
	}
	for _, c := range node.Children {
		if c.NodeType == ast.ExternLinkNode || c.NodeType == ast.ExternIncludePathNode {
			continue // top level: allowed
		}
		walk(c)
	}
}

// Errors returns any errors encountered during import resolution.
func (ml *ModuleLoader) Errors() []string {
	out := make([]string, 0, len(ml.errors))
	for _, d := range ml.errors {
		out = append(out, d.String())
	}
	return out
}

func (ml *ModuleLoader) Diagnostics() []diagnostics.Diagnostic {
	out := make([]diagnostics.Diagnostic, len(ml.errors))
	copy(out, ml.errors)
	return out
}

func (ml *ModuleLoader) addErrorAt(line int, col int, format string, args ...interface{}) {
	msg := fmt.Sprintf(format, args...)
	var loc *diagnostics.Location
	if line > 0 {
		loc = &diagnostics.Location{Line: line, Column: col}
	}
	ml.errors = append(ml.errors, diagnostics.Diagnostic{
		Code:     "QK-LOAD-001",
		Stage:    diagnostics.StageLoad,
		Severity: diagnostics.SeverityError,
		Message:  msg,
		Location: loc,
	})
}

func (ml *ModuleLoader) beginResolve(absPath string) {
	ml.resolving[absPath] = len(ml.stack)
	ml.stack = append(ml.stack, absPath)
}

func (ml *ModuleLoader) endResolve(absPath string) {
	if len(ml.stack) > 0 {
		ml.stack = ml.stack[:len(ml.stack)-1]
	}
	delete(ml.resolving, absPath)
	ml.loaded[absPath] = true
}

func formatImportChain(paths []string) string {
	parts := make([]string, 0, len(paths))
	for _, p := range paths {
		parts = append(parts, filepath.Base(p))
	}
	return strings.Join(parts, " -> ")
}

func isFileImportPath(importPath string) bool {
	if strings.HasPrefix(importPath, "./") || strings.HasPrefix(importPath, "../") {
		return true
	}
	if strings.HasPrefix(importPath, "/") {
		return true
	}
	if filepath.IsAbs(importPath) {
		return true
	}
	if len(importPath) >= 3 && ((importPath[0] >= 'A' && importPath[0] <= 'Z') || (importPath[0] >= 'a' && importPath[0] <= 'z')) && importPath[1] == ':' && (importPath[2] == '/' || importPath[2] == '\\') {
		return true
	}
	return false
}

func normalizeImportPath(currentDir, importPath string) string {
	resolved := importPath
	if !filepath.IsAbs(resolved) && !strings.HasPrefix(resolved, "/") {
		resolved = filepath.Join(currentDir, resolved)
	}
	if filepath.Ext(resolved) == "" {
		resolved += ".qrk"
	}
	return resolved
}

// normalizeExternPath resolves an extern 'path' string to a file system path.
// Unlike normalizeImportPath, it does not append .qrk — extern paths are .hpp files.
func normalizeExternPath(currentDir, importPath string) string {
	if filepath.IsAbs(importPath) {
		return importPath
	}
	return filepath.Join(currentDir, importPath)
}

func isStdlibImportPath(importPath string) bool {
	return strings.HasPrefix(importPath, "std/")
}

func findStdlibRoot(currentFilePath string) (string, bool) {
	if envRoot := os.Getenv("QUARK_STDLIB_ROOT"); envRoot != "" {
		if st, err := os.Stat(envRoot); err == nil && st.IsDir() {
			return envRoot, true
		}
	}

	currentDir := filepath.Dir(currentFilePath)
	for dir := currentDir; ; {
		candidate := filepath.Join(dir, "stdlib")
		if st, err := os.Stat(candidate); err == nil && st.IsDir() {
			return candidate, true
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}

	if exePath, err := os.Executable(); err == nil {
		exeDir := filepath.Dir(exePath)
		candidates := []string{
			filepath.Join(exeDir, "stdlib"),
			filepath.Join(exeDir, "..", "stdlib"),
			filepath.Join(exeDir, "..", "..", "stdlib"),
		}
		for _, candidate := range candidates {
			if st, err := os.Stat(candidate); err == nil && st.IsDir() {
				return candidate, true
			}
		}
	}

	return "", false
}

func normalizeStdlibImportPath(importPath, currentFilePath string) (string, error) {
	root, ok := findStdlibRoot(currentFilePath)
	if !ok {
		return "", fmt.Errorf("cannot resolve stdlib import '%s': no stdlib root found (set QUARK_STDLIB_ROOT or add a stdlib directory)", importPath)
	}
	rel := strings.TrimPrefix(importPath, "std/")
	resolved := filepath.Join(root, rel)
	if filepath.Ext(resolved) == "" {
		resolved += ".qrk"
	}
	return resolved, nil
}

func extractInclAlias(inclNode *ast.TreeNode) string {
	if inclNode == nil || len(inclNode.Children) < 2 {
		return ""
	}
	aliasNode := inclNode.Children[1]
	if aliasNode == nil || aliasNode.NodeType != ast.IdentifierNode {
		return ""
	}
	return aliasNode.TokenLiteral()
}

func buildSyntheticInclNode(moduleName string, alias string, inclLine int) *ast.TreeNode {
	syntheticInclTok := token.Token{
		Type:    token.INCL,
		Literal: "incl",
		Line:    inclLine,
		Column:  0,
	}
	syntheticIncl := ast.NewNode(ast.InclNode, &syntheticInclTok)
	syntheticNameTok := token.Token{
		Type:    token.ID,
		Literal: moduleName,
		Line:    inclLine,
		Column:  0,
	}
	syntheticName := ast.NewNode(ast.IdentifierNode, &syntheticNameTok)
	syntheticIncl.AddChild(syntheticName)

	if alias != "" {
		aliasTok := token.Token{Type: token.ID, Literal: alias, Line: inclLine, Column: 0}
		syntheticIncl.AddChild(ast.NewNode(ast.IdentifierNode, &aliasTok))
	}

	return syntheticIncl
}

// ResolveImports walks the AST rooted at `root`, finds InclNode children that
// reference file paths (string literals), loads and parses those files, and
// splices their ModuleNode + a synthetic InclNode back into the tree.
//
// currentFilePath is the absolute path of the file that produced `root`.
func (ml *ModuleLoader) ResolveImports(root *ast.TreeNode, currentFilePath string) {
	absPath, err := filepath.Abs(currentFilePath)
	if err != nil {
		ml.addErrorAt(0, 0, "cannot resolve path for '%s': %s", currentFilePath, err)
		return
	}
	if ml.root == "" {
		ml.root = absPath
	}
	ml.beginResolve(absPath)
	ml.resolveImportsInNode(root, absPath)
	ml.endResolve(absPath)
	ml.checkTopLevelClashes(root)
	ml.namespaceModules(root)
}

// topLevelDef is a name defined at the top level of one of the spliced files.
type topLevelDef struct {
	name string
	kind string // "fn", "variable", "extern fn" or "struct"
	file string // ast.TreeNode.SourceFile: "" for the main file
	node *ast.TreeNode
}

// topLevelDefinition returns what a top-level statement defines, if anything.
// `fn f(...)` is parsed as `f = fn(...)`, so it is an assignment here.
func topLevelDefinition(stmt *ast.TreeNode) (topLevelDef, bool) {
	if stmt == nil {
		return topLevelDef{}, false
	}
	switch stmt.NodeType {
	case ast.ExternFnNode:
		if stmt.ExternReceiver != "" || stmt.TokenLiteral() == "" {
			return topLevelDef{}, false // methods live per receiver type
		}
		return topLevelDef{name: stmt.TokenLiteral(), kind: "extern fn", file: stmt.SourceFile, node: stmt}, true
	case ast.StructDefNode:
		if stmt.TokenLiteral() == "" {
			return topLevelDef{}, false
		}
		return topLevelDef{name: stmt.TokenLiteral(), kind: "struct", file: stmt.SourceFile, node: stmt}, true
	}
	name := definedName(stmt)
	if name == "" {
		return topLevelDef{}, false
	}
	kind := "variable"
	if stmt.NodeType == ast.FunctionNode || (len(stmt.Children) == 2 && stmt.Children[1] != nil && stmt.Children[1].NodeType == ast.LambdaNode) {
		kind = "fn"
	}
	return topLevelDef{name: name, kind: kind, file: stmt.SourceFile, node: stmt.Children[0]}, true
}

// checkTopLevelClashes rejects a top-level name defined by two different files.
// Every top-level name of an included file lands in the importer's single
// scope (only `module` members are namespaced), so without this check one
// definition silently replaces the other: an importer's `fn helper` would
// hijack a library's internal calls, or a user's `fn fmt_full` would be
// ignored in favour of std/fmt's extern. Within one file, a `fn` or variable
// may be assigned again as usual, but an `extern fn` or `struct` name can't be
// reused.
func (ml *ModuleLoader) checkTopLevelClashes(root *ast.TreeNode) {
	if root == nil {
		return
	}
	first := map[string]topLevelDef{}
	reported := map[string]bool{}
	for _, stmt := range root.Children {
		def, ok := topLevelDefinition(stmt)
		if !ok {
			continue
		}
		prev, seen := first[def.name]
		if !seen {
			first[def.name] = def
			continue
		}
		if reported[def.name] {
			continue
		}
		reassignable := func(k string) bool { return k == "fn" || k == "variable" }
		if def.file == prev.file && reassignable(def.kind) && reassignable(prev.kind) {
			continue
		}
		if def.file == prev.file && def.kind == "struct" && prev.kind == "struct" {
			continue // the checker reports `struct 'X' already defined`
		}
		reported[def.name] = true
		// Point at the main file's definition when there is one: that is the
		// one the user can rename.
		at, other := def, prev
		if prev.file == "" && def.file != "" {
			at, other = prev, def
		}
		where := fmt.Sprintf("line %d", lineOf(other.node))
		if other.file != "" && other.file != at.file {
			where = fmt.Sprintf("%s:%d", other.file, lineOf(other.node))
		}
		var hint string
		switch {
		case at.file == other.file:
			hint = "rename one of them"
		case at.file == "":
			hint = "every top-level name of an included file is visible to the importer, so rename this one"
		default:
			hint = "both files are included and their top-level names share one scope; rename one of them or move it into its file's module block"
		}
		ml.addClashError(at, fmt.Sprintf("'%s' is already defined as %s (%s); %s",
			at.name, article(other.kind), where, hint))
	}
}

func article(kind string) string {
	if kind == "extern fn" {
		return "an extern fn"
	}
	return "a " + kind
}

// addClashError reports msg at def, naming def's own file when it isn't the
// main file (loader diagnostics default to the main file).
func (ml *ModuleLoader) addClashError(def topLevelDef, msg string) {
	loc := &diagnostics.Location{File: def.file, Line: lineOf(def.node)}
	if def.node != nil && def.node.Token != nil {
		loc.Column = def.node.Token.Column
	}
	ml.errors = append(ml.errors, diagnostics.Diagnostic{
		Code:     "QK-LOAD-001",
		Stage:    diagnostics.StageLoad,
		Severity: diagnostics.SeverityError,
		Message:  msg,
		Location: loc,
	})
}

// resolveImportsInNode processes all InclNode children of `node`.
// It modifies node.Children in place, replacing file-based InclNodes with
// [ModuleNode, InclNode(identifier)] pairs.
func (ml *ModuleLoader) resolveImportsInNode(node *ast.TreeNode, currentFilePath string) {
	currentDir := filepath.Dir(currentFilePath)

	// We need to iterate carefully since we're modifying the children slice.
	// Process from the end to preserve indices, or rebuild the slice.
	newChildren := make([]*ast.TreeNode, 0, len(node.Children))
	ml.checkNestedDirectives(node, currentFilePath)

	for _, child := range node.Children {
		if child.NodeType == ast.ExternLinkNode || child.NodeType == ast.ExternIncludePathNode {
			ml.recordExternDirective(child, currentFilePath)
			newChildren = append(newChildren, child)
			continue
		}

		// Resolve extern 'path' source includes — rewrite token literal to absolute path.
		if child.NodeType == ast.ExternHeaderNode && len(child.Children) > 0 {
			pathNode := child.Children[0]
			if pathNode != nil && pathNode.Token != nil && pathNode.Token.Type == token.STRING {
				importPath := pathNode.Token.Literal
				var resolvedPath string
				if isStdlibImportPath(importPath) {
					stdResolved, err := normalizeStdlibImportPath(importPath, currentFilePath)
					if err != nil {
						inclLine := 0
						if child.Token != nil {
							inclLine = child.Token.Line
						}
						ml.addErrorAt(inclLine, 0, "%s", err)
						newChildren = append(newChildren, child)
						continue
					}
					resolvedPath = stdResolved
				} else {
					resolvedPath = normalizeExternPath(currentDir, importPath)
				}
				absResolved, err := filepath.Abs(resolvedPath)
				if err != nil {
					inclLine := 0
					if child.Token != nil {
						inclLine = child.Token.Line
					}
					ml.addErrorAt(inclLine, 0, "cannot resolve extern path '%s': %s", importPath, err)
					newChildren = append(newChildren, child)
					continue
				}
				if !ml.seenHeaders[absResolved] {
					ml.seenHeaders[absResolved] = true
					ml.headers = append(ml.headers, absResolved)
				}
				// Rewrite the literal to the resolved absolute path
				newTok := *pathNode.Token
				newTok.Literal = absResolved
				newPathNode := ast.NewNode(ast.LiteralNode, &newTok)
				child.Children[0] = newPathNode
			}
			newChildren = append(newChildren, child)
			continue
		}

		if child.NodeType != ast.InclNode || len(child.Children) == 0 {
			newChildren = append(newChildren, child)
			continue
		}

		inclChild := child.Children[0]

		// Only process string-literal incl nodes (file imports).
		// Identifier incl nodes (same-file modules) pass through unchanged.
		if inclChild.NodeType != ast.LiteralNode || inclChild.Token == nil || inclChild.Token.Type != token.STRING {
			newChildren = append(newChildren, child)
			continue
		}

		importPath := inclChild.Token.Literal
		alias := extractInclAlias(child)
		inclLine := 0
		if child.Token != nil {
			inclLine = child.Token.Line
		}

		resolvedPath := ""
		// Determine resolution strategy
		if isFileImportPath(importPath) {
			// Tier 1: file import — resolve relative to current file or use absolute path directly
			resolvedPath = normalizeImportPath(currentDir, importPath)
		} else if isStdlibImportPath(importPath) {
			// Tier 2: stdlib import
			stdResolved, err := normalizeStdlibImportPath(importPath, currentFilePath)
			if err != nil {
				ml.addErrorAt(inclLine, 0, "%s", err)
				continue
			}
			resolvedPath = stdResolved
		} else {
			ml.addErrorAt(inclLine, 0, "unsupported import path '%s'; use relative, absolute, or std/... path", importPath)
			continue
		}

		absResolved, err := filepath.Abs(resolvedPath)
		if err != nil {
			ml.addErrorAt(inclLine, 0, "cannot resolve import path '%s': %s", importPath, err)
			continue
		}

		// Check for circular import (current DFS path)
		if idx, inProgress := ml.resolving[absResolved]; inProgress {
			chain := append(append([]string{}, ml.stack[idx:]...), absResolved)
			ml.addErrorAt(inclLine, 0, "circular import detected: %s", formatImportChain(chain))
			continue
		}

		// Record the import edge so NativeDeps can order libraries.
		ml.fileImports[currentFilePath] = append(ml.fileImports[currentFilePath], absResolved)

		// Check for duplicate import (dedup after successful load)
		if ml.loaded[absResolved] {
			if moduleName, ok := ml.loadedModules[absResolved]; ok && moduleName != "" {
				newChildren = append(newChildren, buildSyntheticInclNode(moduleName, alias, inclLine))
			}
			continue
		}

		// Check file exists
		if _, err := os.Stat(absResolved); os.IsNotExist(err) {
			ml.addErrorAt(inclLine, 0, "cannot find module '%s': file '%s' does not exist", importPath, absResolved)
			continue
		}

		// Read and parse the imported file
		content, err := os.ReadFile(absResolved)
		if err != nil {
			ml.addErrorAt(inclLine, 0, "cannot read '%s': %s", absResolved, err)
			continue
		}

		l := lexer.New(string(content))
		tokens := l.Tokenize()

		p := parser.New(tokens)
		importedAST := p.Parse()

		if len(p.Errors()) > 0 {
			for _, pErr := range p.Errors() {
				ml.addErrorAt(inclLine, 0, "in '%s': %s", importPath, pErr)
			}
			continue
		}
		// Tag the imported file's nodes before transitive imports are spliced in,
		// so runtime errors inside the module name its file.
		ast.SetSourceFile(importedAST, moduleDisplayName(importPath, absResolved))

		// Capture module names defined directly in this imported file before
		// recursive splicing introduces transitive modules.
		directModules := findModuleNodes(importedAST)

		// Mark as resolving before descending (for cycle detection)
		ml.beginResolve(absResolved)

		// Recursively resolve imports in the imported file
		ml.resolveImportsInNode(importedAST, absResolved)
		ml.endResolve(absResolved)

		if len(directModules) == 0 {
			ml.addErrorAt(inclLine, 0, "imported file '%s' does not define a module", importPath)
			continue
		}

		// Splice all children from the imported AST into our tree.
		// This includes transitively-resolved ModuleNodes/InclNodes from sub-imports
		// as well as the file's own ModuleNode.
		for _, importedChild := range importedAST.Children {
			newChildren = append(newChildren, importedChild)
		}

		// Use only the primary direct module from the imported file.
		// Additional modules can still be imported explicitly via `incl moduleName`.
		moduleName := ""
		if len(directModules[0].Children) > 0 {
			moduleName = directModules[0].Children[0].TokenLiteral()
		}
		if moduleName == "" {
			ml.addErrorAt(inclLine, 0, "module in '%s' has no name", importPath)
			continue
		}
		ml.loadedModules[absResolved] = moduleName

		newChildren = append(newChildren, buildSyntheticInclNode(moduleName, alias, inclLine))
	}

	node.Children = newChildren
}

// moduleDisplayName is how runtime errors name an imported file: std/ imports
// as "std/<name>.qrk", others relative to the working directory when possible.
func moduleDisplayName(importPath, absPath string) string {
	if isStdlibImportPath(importPath) {
		return "std/" + filepath.Base(absPath)
	}
	if wd, err := os.Getwd(); err == nil {
		if rel, err := filepath.Rel(wd, absPath); err == nil && !strings.HasPrefix(rel, "..") {
			return filepath.ToSlash(rel)
		}
	}
	return filepath.ToSlash(absPath)
}

// findModuleNodes returns all top-level ModuleNodes in source order.
func findModuleNodes(root *ast.TreeNode) []*ast.TreeNode {
	modules := make([]*ast.TreeNode, 0)
	for _, child := range root.Children {
		if child.NodeType == ast.ModuleNode {
			modules = append(modules, child)
		}
	}
	return modules
}
