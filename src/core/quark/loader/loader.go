package loader

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"quark/ast"
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
	errors        []string
}

// NewModuleLoader creates a new module loader.
func NewModuleLoader() *ModuleLoader {
	return &ModuleLoader{
		loaded:        make(map[string]bool),
		loadedModules: make(map[string]string),
		resolving:     make(map[string]int),
		stack:         make([]string, 0),
		errors:        make([]string, 0),
	}
}

// Errors returns any errors encountered during import resolution.
func (ml *ModuleLoader) Errors() []string {
	return ml.errors
}

func (ml *ModuleLoader) addError(format string, args ...interface{}) {
	ml.errors = append(ml.errors, fmt.Sprintf(format, args...))
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

func extractUseAlias(useNode *ast.TreeNode) string {
	if useNode == nil || len(useNode.Children) < 2 {
		return ""
	}
	aliasNode := useNode.Children[1]
	if aliasNode == nil || aliasNode.NodeType != ast.IdentifierNode {
		return ""
	}
	return aliasNode.TokenLiteral()
}

func buildSyntheticUseNode(moduleName string, alias string, useLine int) *ast.TreeNode {
	syntheticUseTok := token.Token{
		Type:    token.USE,
		Literal: "use",
		Line:    useLine,
		Column:  0,
	}
	syntheticUse := ast.NewNode(ast.UseNode, &syntheticUseTok)
	syntheticNameTok := token.Token{
		Type:    token.ID,
		Literal: moduleName,
		Line:    useLine,
		Column:  0,
	}
	syntheticName := ast.NewNode(ast.IdentifierNode, &syntheticNameTok)
	syntheticUse.AddChild(syntheticName)

	if alias != "" {
		aliasTok := token.Token{Type: token.ID, Literal: alias, Line: useLine, Column: 0}
		syntheticUse.AddChild(ast.NewNode(ast.IdentifierNode, &aliasTok))
	}

	return syntheticUse
}

// ResolveImports walks the AST rooted at `root`, finds UseNode children that
// reference file paths (string literals), loads and parses those files, and
// splices their ModuleNode + a synthetic UseNode back into the tree.
//
// currentFilePath is the absolute path of the file that produced `root`.
func (ml *ModuleLoader) ResolveImports(root *ast.TreeNode, currentFilePath string) {
	absPath, err := filepath.Abs(currentFilePath)
	if err != nil {
		ml.addError("cannot resolve path for '%s': %s", currentFilePath, err)
		return
	}
	ml.beginResolve(absPath)
	ml.resolveImportsInNode(root, absPath)
	ml.endResolve(absPath)
}

// resolveImportsInNode processes all UseNode children of `node`.
// It modifies node.Children in place, replacing file-based UseNodes with
// [ModuleNode, UseNode(identifier)] pairs.
func (ml *ModuleLoader) resolveImportsInNode(node *ast.TreeNode, currentFilePath string) {
	currentDir := filepath.Dir(currentFilePath)

	// We need to iterate carefully since we're modifying the children slice.
	// Process from the end to preserve indices, or rebuild the slice.
	newChildren := make([]*ast.TreeNode, 0, len(node.Children))

	for _, child := range node.Children {
		if child.NodeType != ast.UseNode || len(child.Children) == 0 {
			newChildren = append(newChildren, child)
			continue
		}

		useChild := child.Children[0]

		// Only process string-literal use nodes (file imports).
		// Identifier use nodes (same-file modules) pass through unchanged.
		if useChild.NodeType != ast.LiteralNode || useChild.Token == nil || useChild.Token.Type != token.STRING {
			newChildren = append(newChildren, child)
			continue
		}

		importPath := useChild.Token.Literal
		alias := extractUseAlias(child)
		useLine := 0
		if child.Token != nil {
			useLine = child.Token.Line
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
				ml.addError("line %d: %s", useLine, err)
				continue
			}
			resolvedPath = stdResolved
		} else {
			ml.addError("line %d: unsupported import path '%s'; use relative, absolute, or std/... path", useLine, importPath)
			continue
		}

		absResolved, err := filepath.Abs(resolvedPath)
		if err != nil {
			ml.addError("line %d: cannot resolve import path '%s': %s", useLine, importPath, err)
			continue
		}

		// Check for circular import (current DFS path)
		if idx, inProgress := ml.resolving[absResolved]; inProgress {
			chain := append(append([]string{}, ml.stack[idx:]...), absResolved)
			ml.addError("line %d: circular import detected: %s", useLine, formatImportChain(chain))
			continue
		}

		// Check for duplicate import (dedup after successful load)
		if ml.loaded[absResolved] {
			if moduleName, ok := ml.loadedModules[absResolved]; ok && moduleName != "" {
				newChildren = append(newChildren, buildSyntheticUseNode(moduleName, alias, useLine))
			}
			continue
		}

		// Check file exists
		if _, err := os.Stat(absResolved); os.IsNotExist(err) {
			ml.addError("line %d: cannot find module '%s': file '%s' does not exist", useLine, importPath, absResolved)
			continue
		}

		// Read and parse the imported file
		content, err := os.ReadFile(absResolved)
		if err != nil {
			ml.addError("line %d: cannot read '%s': %s", useLine, absResolved, err)
			continue
		}

		l := lexer.New(string(content))
		tokens := l.Tokenize()

		p := parser.New(tokens)
		importedAST := p.Parse()

		if len(p.Errors()) > 0 {
			for _, pErr := range p.Errors() {
				ml.addError("in '%s': %s", importPath, pErr)
			}
			continue
		}

		// Capture module names defined directly in this imported file before
		// recursive splicing introduces transitive modules.
		directModules := findModuleNodes(importedAST)

		// Mark as resolving before descending (for cycle detection)
		ml.beginResolve(absResolved)

		// Recursively resolve imports in the imported file
		ml.resolveImportsInNode(importedAST, absResolved)
		ml.endResolve(absResolved)

		if len(directModules) == 0 {
			ml.addError("line %d: imported file '%s' does not define a module", useLine, importPath)
			continue
		}

		// Splice all children from the imported AST into our tree.
		// This includes transitively-resolved ModuleNodes/UseNodes from sub-imports
		// as well as the file's own ModuleNode.
		for _, importedChild := range importedAST.Children {
			newChildren = append(newChildren, importedChild)
		}

		// Use only the primary direct module from the imported file.
		// Additional modules can still be imported explicitly via `use moduleName`.
		moduleName := ""
		if len(directModules[0].Children) > 0 {
			moduleName = directModules[0].Children[0].TokenLiteral()
		}
		if moduleName == "" {
			ml.addError("line %d: module in '%s' has no name", useLine, importPath)
			continue
		}
		ml.loadedModules[absResolved] = moduleName

		newChildren = append(newChildren, buildSyntheticUseNode(moduleName, alias, useLine))
	}

	node.Children = newChildren
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
