package loader

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func libNames(deps NativeDeps) string {
	names := make([]string, len(deps.Libs))
	for i, l := range deps.Libs {
		names[i] = l.Name
	}
	return strings.Join(names, " ")
}

func TestNativeDeps_LinkOrderFollowsImportGraph(t *testing.T) {
	tmp := t.TempDir()
	entry := filepath.Join(tmp, "main.qrk")
	// main -> a, c; a -> b (imported before a's own link); c -> b.
	// Every file's libraries must come before those of the files it imports,
	// and b's library must come after both a's and c's.
	writeFile(t, entry, "extern link 'zmain'\nincl './a'\nincl './c'\n")
	writeFile(t, filepath.Join(tmp, "a.qrk"), "incl './b'\nextern link 'la'\nmodule a:\n    fn fa() -> 1\n")
	writeFile(t, filepath.Join(tmp, "b.qrk"), "extern link 'lb'\nmodule b:\n    fn fb() -> 2\n")
	writeFile(t, filepath.Join(tmp, "c.qrk"), "incl './b'\nextern link 'lc'\nextern link 'zmain'\nmodule c:\n    fn fc() -> 3\n")

	ml := NewModuleLoader()
	ml.ResolveImports(parseRoot(t, entry), entry)
	if len(ml.Errors()) > 0 {
		t.Fatalf("unexpected loader errors: %v", ml.Errors())
	}
	deps := ml.NativeDeps()
	// Topological order main, c, a, b. zmain appears twice; its last
	// position (in c) wins.
	if got, want := libNames(deps), "lc zmain la lb"; got != want {
		t.Fatalf("link order: got %q, want %q", got, want)
	}
	if lib := deps.Libs[1]; lib.File != filepath.Join(tmp, "c.qrk") || lib.Line != 3 {
		t.Fatalf("expected zmain to be attributed to c.qrk:3, got %s", lib.Origin())
	}
}

func TestNativeDeps_IncludePathsAndHeaders(t *testing.T) {
	tmp := t.TempDir()
	entry := filepath.Join(tmp, "main.qrk")
	sub := filepath.Join(tmp, "sub")
	if err := os.MkdirAll(filepath.Join(sub, "include"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(tmp, "include"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, entry, "extern include_path 'include'\nincl './sub/m'\n")
	// Relative to the declaring file (sub/), not the entry file.
	writeFile(t, filepath.Join(sub, "m.qrk"), "extern include_path 'include'\nextern include_path 'include'\nextern 'glue.hpp'\nmodule m:\n    fn f() -> 1\n")

	ml := NewModuleLoader()
	ml.ResolveImports(parseRoot(t, entry), entry)
	if len(ml.Errors()) > 0 {
		t.Fatalf("unexpected loader errors: %v", ml.Errors())
	}
	deps := ml.NativeDeps()
	want := []string{filepath.Join(tmp, "include"), filepath.Join(sub, "include")}
	if strings.Join(deps.IncludeDirs, ",") != strings.Join(want, ",") {
		t.Fatalf("include dirs: got %v, want %v", deps.IncludeDirs, want)
	}
	if len(deps.Headers) != 1 || deps.Headers[0] != filepath.Join(sub, "glue.hpp") {
		t.Fatalf("headers: got %v", deps.Headers)
	}
}

func TestNativeDeps_MissingIncludePath(t *testing.T) {
	tmp := t.TempDir()
	entry := filepath.Join(tmp, "main.qrk")
	writeFile(t, entry, "print(1)\nextern include_path 'missing'\n")

	ml := NewModuleLoader()
	ml.ResolveImports(parseRoot(t, entry), entry)
	diags := ml.Diagnostics()
	if len(diags) != 1 {
		t.Fatalf("expected one diagnostic, got %v", ml.Errors())
	}
	d := diags[0]
	if d.Code != "QK-FFI-001" || d.Location == nil || d.Location.File != entry || d.Location.Line != 2 {
		t.Fatalf("expected QK-FFI-001 at %s:2, got %s", entry, d.String())
	}
}

func TestNativeDeps_DirectiveInsideModuleRejected(t *testing.T) {
	tmp := t.TempDir()
	entry := filepath.Join(tmp, "main.qrk")
	writeFile(t, entry, "module m:\n    extern link 'foo'\n    fn f() -> 1\n")

	ml := NewModuleLoader()
	ml.ResolveImports(parseRoot(t, entry), entry)
	errs := strings.Join(ml.Errors(), "\n")
	if !strings.Contains(errs, "extern link must be at the top level") {
		t.Fatalf("expected top-level error, got: %v", ml.Errors())
	}
}

func TestNativeDeps_Sources(t *testing.T) {
	tmp := t.TempDir()
	entry := filepath.Join(tmp, "main.qrk")
	sub := filepath.Join(tmp, "sub")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	src := filepath.Join(sub, "glue.cpp")
	writeFile(t, src, "int x;\n")
	writeFile(t, entry, "extern source 'missing.cpp'\nincl './sub/m'\n")
	// Same source twice, resolved relative to the declaring file.
	writeFile(t, filepath.Join(sub, "m.qrk"), "extern source 'glue.cpp'\nextern source 'glue.cpp'\nmodule m:\n    fn f() -> 1\n")

	ml := NewModuleLoader()
	ml.ResolveImports(parseRoot(t, entry), entry)
	diags := ml.Diagnostics()
	if len(diags) != 1 || diags[0].Code != "QK-FFI-001" || diags[0].Location == nil || diags[0].Location.Line != 1 {
		t.Fatalf("expected QK-FFI-001 at line 1, got %v", ml.Errors())
	}
	// A source is identified by its real path: links resolved, and on
	// Windows the long form of a short (8.3) name such as RUNNER~1, which
	// the temporary directory can have.
	want, err := filepath.EvalSymlinks(src)
	if err != nil {
		t.Fatal(err)
	}
	deps := ml.NativeDeps()
	if len(deps.Sources) != 1 || deps.Sources[0] != want {
		t.Fatalf("sources: got %v, want [%s]", deps.Sources, want)
	}
}

func TestNativeDeps_SourceInsideModuleRejected(t *testing.T) {
	tmp := t.TempDir()
	entry := filepath.Join(tmp, "main.qrk")
	writeFile(t, entry, "module m:\n    extern source 'glue.cpp'\n    fn f() -> 1\n")

	ml := NewModuleLoader()
	ml.ResolveImports(parseRoot(t, entry), entry)
	errs := strings.Join(ml.Errors(), "\n")
	if !strings.Contains(errs, "extern source must be at the top level") {
		t.Fatalf("expected top-level error, got: %v", ml.Errors())
	}
}
