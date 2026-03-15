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
