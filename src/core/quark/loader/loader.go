package loader

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"quark/ast"
	"quark/diagnostics"
	"quark/names"
	"quark/token"
)

// ModuleLoader resolves multi-file imports (modules v0.2): it loads every
// imported .qrk file once, gives each definition a unique internal name,
// resolves every reference (bare and `::`-qualified) per file, and splices
// the files into one tree.
type ModuleLoader struct {
	resolving map[string]int // absolute paths currently in DFS stack (for cycle detection)

	// Every loaded file, main script first, and the internal-name table.
	files  []*srcFile
	byPath map[string]*srcFile
	names  names.Table
	stack  []string // current import chain
	// Every name the main script defines or writes. A local of an imported
	// file with one of these names is renamed (renameLocal).
	mainNames map[string]bool
	errors    []diagnostics.Diagnostic

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
		byPath:       make(map[string]*srcFile),
		names:        make(names.Table),
		resolving:    make(map[string]int),
		stack:        make([]string, 0),
		errors:       make([]diagnostics.Diagnostic, 0),
		fileLibs:     make(map[string][]NativeLib),
		fileImports:  make(map[string][]string),
		seenIncludes: make(map[string]bool),
		seenHeaders:  make(map[string]bool),
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
			root, err := findStdlibRoot()
			if err != nil {
				ml.addFFIError("QK-FFI-001", currentFilePath, node.Token, "cannot resolve extern include_path '%s': %v", arg, err)
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
	ml.resolving[realPath(absPath)] = len(ml.stack)
	ml.stack = append(ml.stack, absPath)
}

func (ml *ModuleLoader) endResolve(absPath string) {
	if len(ml.stack) > 0 {
		ml.stack = ml.stack[:len(ml.stack)-1]
	}
	delete(ml.resolving, realPath(absPath))
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

// findStdlibRoot is the standard library directory: $QUARK_STDLIB_ROOT when
// set, which must then be a directory, else the stdlib directory installed
// with the compiler (src/core/stdlib next to src/core/quark). It never looks
// near the script: a directory named stdlib there would silently replace the
// standard library.
func findStdlibRoot() (string, error) {
	if envRoot := os.Getenv("QUARK_STDLIB_ROOT"); envRoot != "" {
		if st, err := os.Stat(envRoot); err != nil || !st.IsDir() {
			return "", fmt.Errorf("QUARK_STDLIB_ROOT is '%s', which is not a directory", envRoot)
		}
		return envRoot, nil
	}
	exePath, err := os.Executable()
	if err == nil {
		exePath, err = filepath.EvalSymlinks(exePath)
	}
	if err != nil {
		return "", fmt.Errorf("can't locate the standard library: %v (set QUARK_STDLIB_ROOT)", err)
	}
	exeDir := filepath.Dir(exePath)
	candidates := []string{
		filepath.Join(exeDir, "stdlib"),
		filepath.Join(exeDir, "..", "stdlib"),
		filepath.Join(exeDir, "..", "..", "stdlib"),
	}
	for _, candidate := range candidates {
		if st, err := os.Stat(candidate); err == nil && st.IsDir() {
			return candidate, nil
		}
	}
	return "", fmt.Errorf("can't find the standard library next to the compiler (looked in %s); set QUARK_STDLIB_ROOT", strings.Join(candidates, ", "))
}

func normalizeStdlibImportPath(importPath, currentFilePath string) (string, error) {
	root, err := findStdlibRoot()
	if err != nil {
		return "", fmt.Errorf("cannot resolve stdlib import '%s': %v", importPath, err)
	}
	rel := strings.TrimPrefix(importPath, "std/")
	resolved := filepath.Join(root, rel)
	if filepath.Ext(resolved) == "" {
		resolved += ".qrk"
	}
	return resolved, nil
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
