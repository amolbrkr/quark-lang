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

	writeFile(t, entry, "incl './a'\n")
	writeFile(t, a, "incl './b'\nmodule a:\n    fn fa() -> 1\n")
	writeFile(t, b, "incl './a'\nmodule b:\n    fn fb() -> 2\n")

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

	// One file under two bindings is loaded once.
	writeFile(t, entry, "incl './a'\nincl './a' as b\nprint(a::foo() + b::foo())\n")
	writeFile(t, a, "module a:\n    fn foo() -> 1\n")

	root := parseRoot(t, entry)

	ml := NewModuleLoader()
	ml.ResolveImports(root, entry)
	if len(ml.Errors()) > 0 {
		t.Fatalf("unexpected loader errors: %v", ml.Errors())
	}
	if n := countModules(root, "q1_a__a"); n != 1 {
		t.Fatalf("expected module a once, got %d", n)
	}
	if len(ml.files) != 2 {
		t.Fatalf("expected 2 loaded files, got %d", len(ml.files))
	}
}

func countModules(root *ast.TreeNode, internal string) int {
	n := 0
	for _, child := range root.Children {
		if child.NodeType == ast.ModuleNode && len(child.Children) > 0 && child.Children[0].TokenLiteral() == internal {
			n++
		}
	}
	return n
}

func TestResolveImports_SupportsStdlibImport(t *testing.T) {
	tmp := t.TempDir()
	entry := filepath.Join(tmp, "main.qrk")
	stdlibRoot := filepath.Join(tmp, "stdlib")
	writeFile(t, filepath.Join(stdlibRoot, "math.qrk"), "module math:\n    fn floor(x) -> x\n")
	writeFile(t, entry, "incl 'std/math'\n")

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
	if n := countModules(root, "q1_math__math"); n != 1 {
		t.Fatalf("expected the stdlib module spliced once, got %d", n)
	}
}

func TestResolveImports_AllowsAbsoluteImportPath(t *testing.T) {
	tmp := t.TempDir()
	entry := filepath.Join(tmp, "main.qrk")
	lib := filepath.Join(tmp, "lib", "math.qrk")
	writeFile(t, lib, "module math:\n    fn square(n) -> n * n\n")
	writeFile(t, entry, "incl '"+filepath.ToSlash(strings.TrimSuffix(lib, ".qrk"))+"'\n")

	root := parseRoot(t, entry)
	if len(root.Children) != 1 {
		t.Fatalf("expected one top-level child before import resolution, got %d", len(root.Children))
	}

	ml := NewModuleLoader()
	ml.ResolveImports(root, entry)
	if len(ml.Errors()) > 0 {
		t.Fatalf("unexpected loader errors: %v", ml.Errors())
	}
	if n := countModules(root, "q1_math__math"); n != 1 {
		t.Fatalf("expected the imported module spliced once, got %d", n)
	}
}

// --- Error paths ---

func TestResolveImports_MissingFileError(t *testing.T) {
	tmp := t.TempDir()
	entry := filepath.Join(tmp, "main.qrk")
	writeFile(t, entry, "incl './nonexistent'\n")

	root := parseRoot(t, entry)
	ml := NewModuleLoader()
	ml.ResolveImports(root, entry)

	errs := strings.Join(ml.Errors(), "\n")
	if !strings.Contains(errs, "cannot find module") || !strings.Contains(errs, "does not exist") {
		t.Fatalf("expected 'cannot find module' error, got: %v", ml.Errors())
	}
}

func TestResolveImports_ImportedFileWithNoModuleIsALibrary(t *testing.T) {
	tmp := t.TempDir()
	entry := filepath.Join(tmp, "main.qrk")
	lib := filepath.Join(tmp, "lib.qrk")
	// A file of top-level definitions is a valid library (rule 5).
	writeFile(t, lib, "x = 1\nfn f() -> x\n")
	writeFile(t, entry, "incl './lib'\nprint(lib::f())\n")

	root := parseRoot(t, entry)
	ml := NewModuleLoader()
	ml.ResolveImports(root, entry)
	if len(ml.Errors()) > 0 {
		t.Fatalf("unexpected loader errors: %v", ml.Errors())
	}
}

func TestResolveImports_CircularImport3Modules(t *testing.T) {
	tmp := t.TempDir()
	entry := filepath.Join(tmp, "main.qrk")
	a := filepath.Join(tmp, "a.qrk")
	b := filepath.Join(tmp, "b.qrk")
	c := filepath.Join(tmp, "c.qrk")

	writeFile(t, entry, "incl './a'\n")
	writeFile(t, a, "incl './b'\nmodule a:\n    fn fa() -> 1\n")
	writeFile(t, b, "incl './c'\nmodule b:\n    fn fb() -> 2\n")
	writeFile(t, c, "incl './a'\nmodule c:\n    fn fc() -> 3\n")

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
	writeFile(t, a, "incl './b'\nmodule a:\n    fn fa() -> 1\n")
	writeFile(t, entry, "incl './a'\n")

	root := parseRoot(t, entry)
	ml := NewModuleLoader()
	ml.ResolveImports(root, entry)

	if len(ml.Errors()) > 0 {
		t.Fatalf("unexpected errors for transitive imports: %v", ml.Errors())
	}
	if countModules(root, "q1_a__a") != 1 || countModules(root, "q2_b__b") != 1 {
		t.Fatalf("expected modules a and b spliced once each, got %v", root.Children)
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
	writeFile(t, a, "incl './c'\nmodule a:\n    fn fa() -> 1\n")
	writeFile(t, b, "incl './c'\nmodule b:\n    fn fb() -> 2\n")
	writeFile(t, entry, "incl './a'\nincl './b'\n")

	root := parseRoot(t, entry)
	ml := NewModuleLoader()
	ml.ResolveImports(root, entry)

	if len(ml.Errors()) > 0 {
		t.Fatalf("unexpected errors for diamond import: %v", ml.Errors())
	}

	// Module c is spliced exactly once (file 2: a is 1, c 2, b 3).
	if moduleCount := countModules(root, "q2_c__c"); moduleCount != 1 {
		t.Fatalf("expected module c to appear exactly once, got %d", moduleCount)
	}
}

func TestResolveImports_UnsupportedImportPath(t *testing.T) {
	tmp := t.TempDir()
	entry := filepath.Join(tmp, "main.qrk")
	// A bare name (not relative, not absolute, not std/) should error
	writeFile(t, entry, "incl 'justname'\n")

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
	writeFile(t, entry, "incl 'std/nonexistent'\n")

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
	writeFile(t, entry, "incl './bad'\n")

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

func TestResolveImports_SameFileModuleInclRejected(t *testing.T) {
	// `incl moduleName` of a same-file module was removed: write math::add.
	tmp := t.TempDir()
	entry := filepath.Join(tmp, "main.qrk")
	writeFile(t, entry, "module math:\n    fn add(x, y) -> x + y\nincl math\n")

	root := parseRoot(t, entry)
	ml := NewModuleLoader()
	ml.ResolveImports(root, entry)

	errs := strings.Join(ml.Errors(), "\n")
	if !strings.Contains(errs, "a module in the same file needs no incl; write math::name directly") {
		t.Fatalf("expected same-file incl error, got: %v", ml.Errors())
	}
}

func TestDiagnostics_ReturnsStructuredErrors(t *testing.T) {
	tmp := t.TempDir()
	entry := filepath.Join(tmp, "main.qrk")
	writeFile(t, entry, "incl './missing'\n")

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
