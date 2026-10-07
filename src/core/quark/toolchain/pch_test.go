package toolchain

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestPCHKey_ChangesWithFlagsAndHeaders(t *testing.T) {
	compiler, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	runtimeDir, gcDir := t.TempDir(), t.TempDir()
	header := filepath.Join(runtimeDir, "quark", "quark.hpp")
	if err := os.MkdirAll(filepath.Dir(header), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(header, []byte("// v1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	tc := &Toolchain{Compiler: compiler, RuntimeInclude: runtimeDir, GCInclude: gcDir}

	key := func(flags ...string) string {
		t.Helper()
		k, err := tc.pchKey(flags)
		if err != nil {
			t.Fatal(err)
		}
		return k
	}

	base := key("-O3")
	if again := key("-O3"); again != base {
		t.Fatalf("key not stable: %s vs %s", base, again)
	}
	if key("-O0") == base {
		t.Fatalf("key ignores flags")
	}

	// Touching a header must change the key, since clang rejects a PCH
	// whose headers have a newer mtime.
	later := time.Now().Add(time.Hour)
	if err := os.Chtimes(header, later, later); err != nil {
		t.Fatal(err)
	}
	if key("-O3") == base {
		t.Fatalf("key ignores header mtime")
	}
}

func TestIsPCHError(t *testing.T) {
	stale := "fatal error: file '/usr/include/c++/13/vector' has been modified since the precompiled header 'x.pch' was built"
	if !isPCHError(stale) {
		t.Fatalf("stale PCH not detected")
	}
	if isPCHError("main.cpp:3:5: error: use of undeclared identifier 'quark_x'") {
		t.Fatalf("ordinary compile error treated as PCH error")
	}
}
