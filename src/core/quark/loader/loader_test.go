package loader

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"quark/ast"
	"quark/lexer"
	"quark/parser"
)

func parseRoot(t *testing.T, filePath string) *ast.TreeNode {
	t.Helper()

	content, err := os.ReadFile(filePath)
	if err != nil {
		t.Fatalf("read %s: %v", filePath, err)
	}

	l := lexer.New(string(content))
	tokens := l.Tokenize()
	p := parser.New(tokens)
	tree := p.Parse()
	if len(p.Errors()) > 0 {
		t.Fatalf("parse errors in %s: %v", filePath, p.Errors())
	}

	return tree
}

func writeFile(t *testing.T, filePath, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(filePath), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(filePath), err)
	}
	if err := os.WriteFile(filePath, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", filePath, err)
	}
}

func TestResolveImports_DetectsCircularImport(t *testing.T) {
	tmp := t.TempDir()
	entry := filepath.Join(tmp, "main.qrk")
	a := filepath.Join(tmp, "a.qrk")
	b := filepath.Join(tmp, "b.qrk")

	writeFile(t, entry, "use './a'\n")
	writeFile(t, a, "use './b'\nmodule a:\n    fn fa() -> 1\n")
	writeFile(t, b, "use './a'\nmodule b:\n    fn fb() -> 2\n")

	root := parseRoot(t, entry)
	ml := NewModuleLoader()
	ml.ResolveImports(root, entry)

	errs := strings.Join(ml.Errors(), "\n")
	if !strings.Contains(errs, "circular import detected") {
		t.Fatalf("expected circular import error, got: %v", ml.Errors())
	}
	if !strings.Contains(errs, "a.qrk -> b.qrk -> a.qrk") {
		t.Fatalf("expected cycle chain in error, got: %v", ml.Errors())
	}
}

func TestResolveImports_DedupsAlreadyLoadedModule(t *testing.T) {
	tmp := t.TempDir()
	entry := filepath.Join(tmp, "main.qrk")
	a := filepath.Join(tmp, "a.qrk")

	writeFile(t, entry, "use './a'\nuse './a'\n")
	writeFile(t, a, "module a:\n    fn foo() -> 1\n")

	root := parseRoot(t, entry)

	ml := NewModuleLoader()
	ml.ResolveImports(root, entry)
	if len(ml.Errors()) > 0 {
		t.Fatalf("unexpected loader errors: %v", ml.Errors())
	}
}

func TestResolveImports_SupportsStdlibImport(t *testing.T) {
	tmp := t.TempDir()
	entry := filepath.Join(tmp, "main.qrk")
	stdlibRoot := filepath.Join(tmp, "stdlib")
	writeFile(t, filepath.Join(stdlibRoot, "math.qrk"), "module math:\n    fn floor(x) -> x\n")
	writeFile(t, entry, "use 'std/math'\n")

	if err := os.Setenv("QUARK_STDLIB_ROOT", stdlibRoot); err != nil {
		t.Fatalf("set env: %v", err)
	}
	t.Cleanup(func() {
		_ = os.Unsetenv("QUARK_STDLIB_ROOT")
	})

	root := parseRoot(t, entry)

	ml := NewModuleLoader()
	ml.ResolveImports(root, entry)

	if len(ml.Errors()) > 0 {
		t.Fatalf("unexpected loader errors: %v", ml.Errors())
	}
	if len(root.Children) != 2 {
		t.Fatalf("expected imported module and synthetic use node, got %d children", len(root.Children))
	}
}

func TestResolveImports_AllowsAbsoluteImportPath(t *testing.T) {
	tmp := t.TempDir()
	entry := filepath.Join(tmp, "main.qrk")
	lib := filepath.Join(tmp, "lib", "math.qrk")
	writeFile(t, lib, "module math:\n    fn square(n) -> n * n\n")
	writeFile(t, entry, "use '"+filepath.ToSlash(strings.TrimSuffix(lib, ".qrk"))+"'\n")

	root := parseRoot(t, entry)
	if len(root.Children) != 1 {
		t.Fatalf("expected one top-level child before import resolution, got %d", len(root.Children))
	}

	ml := NewModuleLoader()
	ml.ResolveImports(root, entry)
	if len(ml.Errors()) > 0 {
		t.Fatalf("unexpected loader errors: %v", ml.Errors())
	}
	if len(root.Children) != 2 {
		t.Fatalf("expected imported module and synthetic use node, got %d children", len(root.Children))
	}
}

func TestResolveImports_PreservesUseAliasInSyntheticUse(t *testing.T) {
	tmp := t.TempDir()
	entry := filepath.Join(tmp, "main.qrk")
	lib := filepath.Join(tmp, "lib", "math.qrk")
	writeFile(t, lib, "module math:\n    fn square(n) -> n * n\n")
	writeFile(t, entry, "use './lib/math' as m\n")

	root := parseRoot(t, entry)
	ml := NewModuleLoader()
	ml.ResolveImports(root, entry)
	if len(ml.Errors()) > 0 {
		t.Fatalf("unexpected loader errors: %v", ml.Errors())
	}
	if len(root.Children) != 2 {
		t.Fatalf("expected imported module and synthetic use node, got %d children", len(root.Children))
	}
	useNode := root.Children[1]
	if useNode.NodeType != ast.UseNode {
		t.Fatalf("expected synthetic UseNode, got %v", useNode)
	}
	if len(useNode.Children) < 2 || useNode.Children[1].TokenLiteral() != "m" {
		t.Fatalf("expected synthetic alias 'm', got %#v", useNode.Children)
	}
}

// --- Error paths ---

func TestResolveImports_MissingFileError(t *testing.T) {
	tmp := t.TempDir()
	entry := filepath.Join(tmp, "main.qrk")
	writeFile(t, entry, "use './nonexistent'\n")

	root := parseRoot(t, entry)
	ml := NewModuleLoader()
	ml.ResolveImports(root, entry)

	errs := strings.Join(ml.Errors(), "\n")
	if !strings.Contains(errs, "cannot find module") || !strings.Contains(errs, "does not exist") {
		t.Fatalf("expected 'cannot find module' error, got: %v", ml.Errors())
	}
}

func TestResolveImports_ImportedFileWithNoModule(t *testing.T) {
	tmp := t.TempDir()
	entry := filepath.Join(tmp, "main.qrk")
	lib := filepath.Join(tmp, "lib.qrk")
	// lib.qrk has no module declaration — just bare code
	writeFile(t, lib, "x = 1\n")
	writeFile(t, entry, "use './lib'\n")

	root := parseRoot(t, entry)
	ml := NewModuleLoader()
	ml.ResolveImports(root, entry)

	errs := strings.Join(ml.Errors(), "\n")
	if !strings.Contains(errs, "does not define a module") {
		t.Fatalf("expected 'does not define a module' error, got: %v", ml.Errors())
	}
}

func TestResolveImports_CircularImport3Modules(t *testing.T) {
	tmp := t.TempDir()
	entry := filepath.Join(tmp, "main.qrk")
	a := filepath.Join(tmp, "a.qrk")
	b := filepath.Join(tmp, "b.qrk")
	c := filepath.Join(tmp, "c.qrk")

	writeFile(t, entry, "use './a'\n")
	writeFile(t, a, "use './b'\nmodule a:\n    fn fa() -> 1\n")
	writeFile(t, b, "use './c'\nmodule b:\n    fn fb() -> 2\n")
	writeFile(t, c, "use './a'\nmodule c:\n    fn fc() -> 3\n")

	root := parseRoot(t, entry)
	ml := NewModuleLoader()
	ml.ResolveImports(root, entry)

	errs := strings.Join(ml.Errors(), "\n")
	if !strings.Contains(errs, "circular import detected") {
		t.Fatalf("expected circular import error for 3-module cycle, got: %v", ml.Errors())
	}
	// Cycle chain should mention all 3 files
	if !strings.Contains(errs, "a.qrk") || !strings.Contains(errs, "b.qrk") || !strings.Contains(errs, "c.qrk") {
		t.Fatalf("expected cycle chain to mention a.qrk, b.qrk, c.qrk, got: %v", ml.Errors())
	}
}

func TestResolveImports_TransitiveImports(t *testing.T) {
	// A imports B which imports C — all should resolve without error
	tmp := t.TempDir()
	entry := filepath.Join(tmp, "main.qrk")
	a := filepath.Join(tmp, "a.qrk")
	b := filepath.Join(tmp, "b.qrk")

	writeFile(t, b, "module b:\n    fn fb() -> 2\n")
	writeFile(t, a, "use './b'\nmodule a:\n    fn fa() -> 1\n")
	writeFile(t, entry, "use './a'\n")

	root := parseRoot(t, entry)
	ml := NewModuleLoader()
	ml.ResolveImports(root, entry)

	if len(ml.Errors()) > 0 {
		t.Fatalf("unexpected errors for transitive imports: %v", ml.Errors())
	}
	// Should have: module b, module a, synthetic use for a
	if len(root.Children) < 3 {
		t.Fatalf("expected at least 3 children (modules + synthetic use), got %d", len(root.Children))
	}
}

func TestResolveImports_DiamondImportDeduplication(t *testing.T) {
	// Both A and B import C. Entry imports A and B.
	// C should only be loaded once.
	tmp := t.TempDir()
	entry := filepath.Join(tmp, "main.qrk")
	a := filepath.Join(tmp, "a.qrk")
	b := filepath.Join(tmp, "b.qrk")
	c := filepath.Join(tmp, "c.qrk")

	writeFile(t, c, "module c:\n    fn fc() -> 3\n")
	writeFile(t, a, "use './c'\nmodule a:\n    fn fa() -> 1\n")
	writeFile(t, b, "use './c'\nmodule b:\n    fn fb() -> 2\n")
	writeFile(t, entry, "use './a'\nuse './b'\n")

	root := parseRoot(t, entry)
	ml := NewModuleLoader()
	ml.ResolveImports(root, entry)

	if len(ml.Errors()) > 0 {
		t.Fatalf("unexpected errors for diamond import: %v", ml.Errors())
	}

	// Count how many times module c appears (should be exactly once as ModuleNode)
	moduleCount := 0
	for _, child := range root.Children {
		if child.NodeType == ast.ModuleNode && len(child.Children) > 0 && child.Children[0].TokenLiteral() == "c" {
			moduleCount++
		}
	}
	if moduleCount != 1 {
		t.Fatalf("expected module c to appear exactly once, got %d", moduleCount)
	}
}

func TestResolveImports_UnsupportedImportPath(t *testing.T) {
	tmp := t.TempDir()
	entry := filepath.Join(tmp, "main.qrk")
	// A bare name (not relative, not absolute, not std/) should error
	writeFile(t, entry, "use 'justname'\n")

	root := parseRoot(t, entry)
	ml := NewModuleLoader()
	ml.ResolveImports(root, entry)

	errs := strings.Join(ml.Errors(), "\n")
	if !strings.Contains(errs, "unsupported import path") {
		t.Fatalf("expected 'unsupported import path' error, got: %v", ml.Errors())
	}
}

func TestResolveImports_StdlibNotFoundError(t *testing.T) {
	tmp := t.TempDir()
	entry := filepath.Join(tmp, "main.qrk")
	writeFile(t, entry, "use 'std/nonexistent'\n")

	// Unset QUARK_STDLIB_ROOT to force filesystem search (which should fail in tmpdir)
	old := os.Getenv("QUARK_STDLIB_ROOT")
	_ = os.Setenv("QUARK_STDLIB_ROOT", filepath.Join(tmp, "no_such_stdlib"))
	t.Cleanup(func() {
		if old == "" {
			_ = os.Unsetenv("QUARK_STDLIB_ROOT")
		} else {
			_ = os.Setenv("QUARK_STDLIB_ROOT", old)
		}
	})

	root := parseRoot(t, entry)
	ml := NewModuleLoader()
	ml.ResolveImports(root, entry)

	if len(ml.Errors()) == 0 {
		t.Fatal("expected error for non-existent stdlib module")
	}
}

func TestResolveImports_ParseErrorInImportedFile(t *testing.T) {
	tmp := t.TempDir()
	entry := filepath.Join(tmp, "main.qrk")
	bad := filepath.Join(tmp, "bad.qrk")
	// Invalid syntax: unclosed paren
	writeFile(t, bad, "module bad:\n    fn f( -> 1\n")
	writeFile(t, entry, "use './bad'\n")

	root := parseRoot(t, entry)
	ml := NewModuleLoader()
	ml.ResolveImports(root, entry)

	if len(ml.Errors()) == 0 {
		t.Fatal("expected error for imported file with parse errors")
	}
	errs := strings.Join(ml.Errors(), "\n")
	if !strings.Contains(errs, "bad") {
		t.Fatalf("expected error to reference the bad file, got: %v", ml.Errors())
	}
}

func TestResolveImports_IdentifierUsePassesThrough(t *testing.T) {
	// `use moduleName` (identifier, not string literal) should not be processed as file import
	tmp := t.TempDir()
	entry := filepath.Join(tmp, "main.qrk")
	writeFile(t, entry, "module math:\n    fn add(x, y) -> x + y\nuse math\n")

	root := parseRoot(t, entry)
	ml := NewModuleLoader()
	ml.ResolveImports(root, entry)

	if len(ml.Errors()) > 0 {
		t.Fatalf("unexpected errors for identifier use: %v", ml.Errors())
	}
}

func TestDiagnostics_ReturnsStructuredErrors(t *testing.T) {
	tmp := t.TempDir()
	entry := filepath.Join(tmp, "main.qrk")
	writeFile(t, entry, "use './missing'\n")

	root := parseRoot(t, entry)
	ml := NewModuleLoader()
	ml.ResolveImports(root, entry)

	diags := ml.Diagnostics()
	if len(diags) == 0 {
		t.Fatal("expected diagnostics for missing file")
	}
	d := diags[0]
	if d.Code != "QK-LOAD-001" {
		t.Errorf("expected code QK-LOAD-001, got %s", d.Code)
	}
	if d.Stage != "load" {
		t.Errorf("expected stage 'load', got %s", d.Stage)
	}
	if d.Severity != "error" {
		t.Errorf("expected severity 'error', got %s", d.Severity)
	}
}
