package toolchain

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestGlueErrorDiagnostic_DirectError(t *testing.T) {
	// clang output as printed with -fcolor-diagnostics.
	out := "\x1b[1m/src/glue.hpp:2:25: \x1b[0m\x1b[0;1;31merror: \x1b[0m\x1b[1mexpected expression\x1b[0m\n" +
		"    2 | inline long long bad( { return 1; }\n" +
		"/tmp/main.cpp:9:3: error: use of undeclared identifier 'x'\n"
	d := glueErrorDiagnostic(out, []string{"/src/glue.hpp"})
	if d == nil {
		t.Fatal("expected a QK-FFI-003 diagnostic")
	}
	if d.Code != "QK-FFI-003" || d.Location.File != "/src/glue.hpp" || d.Location.Line != 2 || d.Location.Column != 25 {
		t.Fatalf("unexpected diagnostic: %s", d.String())
	}
	if !strings.Contains(d.Message, "expected expression") {
		t.Fatalf("expected clang's message, got %q", d.Message)
	}
}

func TestGlueErrorDiagnostic_ErrorInHeaderIncludedByGlue(t *testing.T) {
	out := "In file included from /tmp/main.cpp:4:\n" +
		"In file included from /src/glue.hpp:3:\n" +
		"/usr/include/lib.h:10:1: fatal error: 'dep.h' file not found\n"
	d := glueErrorDiagnostic(out, []string{"/src/glue.hpp"})
	if d == nil {
		t.Fatal("expected a QK-FFI-003 diagnostic")
	}
	if d.Location.File != "/src/glue.hpp" || d.Location.Line != 3 {
		t.Fatalf("expected location at the glue include line, got %s", d.String())
	}
	if !strings.Contains(strings.Join(d.Notes, "\n"), "/usr/include/lib.h:10") {
		t.Fatalf("expected a note naming the failing header, got %v", d.Notes)
	}
}

func TestGlueErrorDiagnostic_IgnoresOtherErrors(t *testing.T) {
	out := "/tmp/main.cpp:9:3: error: use of undeclared identifier 'x'\n" +
		"In file included from /tmp/main.cpp:1:\n" +
		"/rt/quark.hpp:5:1: error: something in the runtime\n"
	if d := glueErrorDiagnostic(out, []string{"/src/glue.hpp"}); d != nil {
		t.Fatalf("expected no diagnostic, got %s", d.String())
	}
	if d := glueErrorDiagnostic(out, nil); d != nil {
		t.Fatalf("expected no diagnostic without glue files, got %s", d.String())
	}
}

func TestNativeLinkArgs_OrderAndRpath(t *testing.T) {
	libs := []resolvedLib{
		{Lib: Lib{Name: "shared_user"}, Dir: "/user/lib", Shared: true},
		{Lib: Lib{Name: "static_user"}, Dir: "/user/lib", Shared: false},
		{Lib: Lib{Name: "system"}, Dir: "/usr/lib", Shared: true},
	}
	opts := Options{LibDirs: []string{"/user/lib", "/user/lib"}, LinkArgs: []string{"-Wl,--as-needed"}}
	got := strings.Join(nativeLinkArgs(libs, opts), " ")
	want := "-L/user/lib -Wl,-rpath,/user/lib -lshared_user -lstatic_user -lsystem -Wl,--as-needed"
	if runtime.GOOS == "windows" {
		want = "-L/user/lib -lshared_user -lstatic_user -lsystem -Wl,--as-needed"
	}
	if got != want {
		t.Fatalf("link args:\n got  %s\n want %s", got, want)
	}
}

func TestFindLib_SearchOrder(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("library file names are platform-specific; checked on Linux")
	}
	first, second := t.TempDir(), t.TempDir()
	touch := func(path string) {
		if err := os.WriteFile(path, nil, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	touch(filepath.Join(first, "libfoo.a"))
	touch(filepath.Join(second, "libfoo.so"))
	touch(filepath.Join(second, "libbar.so"))
	touch(filepath.Join(second, "libbar.a"))

	// An earlier directory wins even with only a static library.
	r, ok := findLib(Lib{Name: "foo"}, []string{first, second})
	if !ok || r.Path != filepath.Join(first, "libfoo.a") || r.Shared {
		t.Fatalf("foo: got %+v, %v", r, ok)
	}
	// Within one directory, shared is preferred, like the linker.
	r, ok = findLib(Lib{Name: "bar"}, []string{first, second})
	if !ok || r.Path != filepath.Join(second, "libbar.so") || !r.Shared {
		t.Fatalf("bar: got %+v, %v", r, ok)
	}
	if _, ok := findLib(Lib{Name: "missing"}, []string{first, second}); ok {
		t.Fatal("expected missing library not to be found")
	}
}

func TestMissingLibDiagnostic(t *testing.T) {
	d := missingLibDiagnostic(Lib{Name: "duckdb", Origin: "/std/duckdb.qrk:3"}, []string{"/a", "/b"})
	s := d.String()
	for _, want := range []string{"QK-FFI-002", "'duckdb'", "/std/duckdb.qrk:3", "searched: /a, /b", "--lib-path"} {
		if !strings.Contains(s, want) {
			t.Fatalf("expected %q in:\n%s", want, s)
		}
	}
}
