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
	loaded    map[string]bool // absolute paths fully processed (for dedup)
	resolving map[string]int  // absolute paths currently in DFS stack (for cycle detection)
	stack     []string        // current import chain
	errors    []string
}

// NewModuleLoader creates a new module loader.
func NewModuleLoader() *ModuleLoader {
	return &ModuleLoader{
		loaded:    make(map[string]bool),
		resolving: make(map[string]int),
		stack:     make([]string, 0),
		errors:    make([]string, 0),
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
		useLine := 0
		if child.Token != nil {
			useLine = child.Token.Line
		}

		// Determine resolution strategy
		if !strings.HasPrefix(importPath, "./") && !strings.HasPrefix(importPath, "../") {
			// Tier 2: stdlib import (future)
			ml.addError("line %d: stdlib imports are not yet supported; use relative paths (e.g. use './mymodule')", useLine)
			continue
		}

		// Tier 1: local import — resolve relative to current file
		resolvedPath := filepath.Join(currentDir, importPath+".qrk")
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
			// Already imported — skip silently, don't add the UseNode
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

		// Mark as resolving before descending (for cycle detection)
		ml.beginResolve(absResolved)

		// Recursively resolve imports in the imported file
		ml.resolveImportsInNode(importedAST, absResolved)
		ml.endResolve(absResolved)

		// Find the ModuleNode in the imported AST
		moduleNode := findModuleNode(importedAST)
		if moduleNode == nil {
			ml.addError("line %d: imported file '%s' does not define a module", useLine, importPath)
			continue
		}

		// Extract the module name
		moduleName := ""
		if len(moduleNode.Children) > 0 {
			moduleName = moduleNode.Children[0].TokenLiteral()
		}
		if moduleName == "" {
			ml.addError("line %d: module in '%s' has no name", useLine, importPath)
			continue
		}

		// Splice all children from the imported AST into our tree.
		// This includes transitively-resolved ModuleNodes/UseNodes from sub-imports
		// as well as the file's own ModuleNode.
		for _, importedChild := range importedAST.Children {
			newChildren = append(newChildren, importedChild)
		}

		// Create synthetic use node: use <moduleName> (identifier-style)
		// This triggers the analyzer to import the module's symbols into scope.
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
		newChildren = append(newChildren, syntheticUse)
	}

	node.Children = newChildren
}

// findModuleNode searches the top-level children of an AST for the last ModuleNode.
// We use the last one because transitive imports splice their ModuleNodes before
// the file's own module definition.
func findModuleNode(root *ast.TreeNode) *ast.TreeNode {
	var last *ast.TreeNode
	for _, child := range root.Children {
		if child.NodeType == ast.ModuleNode {
			last = child
		}
	}
	return last
}
