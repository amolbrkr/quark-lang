package main

// FFI Phase 1 integration tests (ffi_v0_1_spec.md §5.2): extern link,
// extern include_path, --lib-path / QUARK_LIB_PATH, rpath, link order, and
// the QK-FFI-001/002/003 diagnostics, against a small C library built here.

import (
	"bytes"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// quarkResult is the outcome of one quark invocation.
type quarkResult struct {
	stdout, stderr string
	err            error
}

// cleanEnv is the test process environment without anything that would let
// a binary or the toolchain find libraries on its own.
func cleanEnv(extra ...string) []string {
	var env []string
	for _, kv := range os.Environ() {
		switch strings.SplitN(kv, "=", 2)[0] {
		case "LD_LIBRARY_PATH", "DYLD_LIBRARY_PATH", "QUARK_LIB_PATH", "QUARK_INCLUDE_PATH":
			continue
		}
		env = append(env, kv)
	}
	return append(env, extra...)
}

func runIn(dir string, env []string, exe string, args ...string) quarkResult {
	cmd := exec.Command(exe, args...)
	cmd.Dir = dir
	cmd.Env = env
	var out, errBuf bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errBuf
	err := cmd.Run()
	return quarkResult{normalizeNewlines(out.String()), normalizeNewlines(errBuf.String()), err}
}

func copyDir(t *testing.T, src, dst string) {
	t.Helper()
	err := filepath.WalkDir(src, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(src, path)
		target := filepath.Join(dst, rel)
		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(target, data, 0o644)
	})
	if err != nil {
		t.Fatalf("copy %s: %v", src, err)
	}
}

func mustRun(t *testing.T, dir string, name string, args ...string) {
	t.Helper()
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("%s %v failed: %v\n%s", name, args, err, out)
	}
}

// buildFFITestLibs compiles the test libraries from srcDir into libDir:
// libqffi_test shared, libqffi_a and libqffi_b static-only.
func buildFFITestLibs(t *testing.T, srcDir, libDir string) {
	t.Helper()
	ar, err := exec.LookPath("ar")
	if err != nil {
		if ar, err = exec.LookPath("llvm-ar"); err != nil {
			t.Skip("no ar or llvm-ar in PATH; needed to build static test libraries")
		}
	}
	if err := os.MkdirAll(libDir, 0o755); err != nil {
		t.Fatal(err)
	}
	shared := []string{"-shared", "-fPIC"}
	sharedName := "libqffi_test.so"
	if runtime.GOOS == "darwin" {
		shared = []string{"-dynamiclib"}
		sharedName = "libqffi_test.dylib"
	}
	args := append([]string{"-x", "c"}, shared...)
	args = append(args, "-I", filepath.Join(srcDir, "include"), filepath.Join(srcDir, "qffi_test.c"), "-o", filepath.Join(libDir, sharedName))
	mustRun(t, srcDir, "clang++", args...)
	for _, name := range []string{"qffi_a", "qffi_b"} {
		obj := filepath.Join(libDir, name+".o")
		mustRun(t, srcDir, "clang++", "-x", "c", "-c", filepath.Join(srcDir, name+".c"), "-o", obj)
		mustRun(t, srcDir, ar, "rcs", filepath.Join(libDir, "lib"+name+".a"), obj)
		os.Remove(obj)
	}
}

func TestFFI_Phase1(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("FFI test libraries are built with Unix conventions")
	}
	root := repoRootFromThisFile(t)
	tmp := t.TempDir()
	src := filepath.Join(tmp, "src") // test programs, copied so --debug output stays out of the repo
	lib := filepath.Join(tmp, "lib")
	work := filepath.Join(tmp, "work") // a working directory unrelated to the programs
	copyDir(t, filepath.Join(root, "src", "testfiles", "ffi"), src)
	buildFFITestLibs(t, src, lib)
	if err := os.MkdirAll(work, 0o755); err != nil {
		t.Fatal(err)
	}

	basicOut := strings.Join([]string{"42", "6.0", "5", "qffi 1.0", "true", "false", "6.0"}, "\n")

	t.Run("shared library, include_path relative to the program", func(t *testing.T) {
		r := runIn(work, cleanEnv(), quarkExePath, "run", filepath.Join(src, "ffi_basic.qrk"), "--lib-path", lib)
		if r.err != nil {
			t.Fatalf("run failed: %v\n%s", r.err, r.stderr)
		}
		if got := strings.TrimSpace(r.stdout); got != basicOut {
			t.Fatalf("got:\n%s\nwant:\n%s", got, basicOut)
		}
	})

	t.Run("built binary finds its library through rpath", func(t *testing.T) {
		exe := filepath.Join(tmp, "bin", "ffi_basic")
		if err := os.MkdirAll(filepath.Dir(exe), 0o755); err != nil {
			t.Fatal(err)
		}
		r := runIn(work, cleanEnv(), quarkExePath, "build", filepath.Join(src, "ffi_basic.qrk"), "-o", exe, "--lib-path", lib)
		if r.err != nil {
			t.Fatalf("build failed: %v\n%s", r.err, r.stderr)
		}
		r = runIn(work, cleanEnv(), exe)
		if r.err != nil {
			t.Fatalf("binary failed without LD_LIBRARY_PATH: %v\n%s", r.err, r.stderr)
		}
		if got := strings.TrimSpace(r.stdout); got != basicOut {
			t.Fatalf("got:\n%s\nwant:\n%s", got, basicOut)
		}
	})

	t.Run("link order across modules and duplicate links", func(t *testing.T) {
		// QUARK_LIB_PATH instead of --lib-path.
		r := runIn(work, cleanEnv("QUARK_LIB_PATH="+lib), quarkExePath, "run", filepath.Join(src, "ffi_modules.qrk"), "--debug")
		if r.err != nil {
			t.Fatalf("run failed: %v\n%s", r.err, r.stderr)
		}
		if got := strings.TrimSpace(r.stdout); got != "13\n7" {
			t.Fatalf("got:\n%s\nwant:\n13\n7", got)
		}
		var linkLine string
		for _, line := range strings.Split(r.stderr, "\n") {
			if strings.Contains(line, "-lqffi_a") {
				linkLine = line
			}
		}
		if linkLine == "" {
			t.Fatalf("no link command in --debug output:\n%s", r.stderr)
		}
		if n := strings.Count(linkLine, "-lqffi_test "); n != 1 {
			t.Fatalf("expected -lqffi_test once, got %d times: %s", n, linkLine)
		}
		if strings.Index(linkLine, "-lqffi_a ") > strings.Index(linkLine, "-lqffi_b ") {
			t.Fatalf("expected -lqffi_a before -lqffi_b: %s", linkLine)
		}
	})

	writeProgram := func(t *testing.T, name, source string) string {
		t.Helper()
		path := filepath.Join(src, name)
		if err := os.WriteFile(path, []byte(source), 0o644); err != nil {
			t.Fatal(err)
		}
		return path
	}
	expectFailure := func(t *testing.T, r quarkResult, wants ...string) {
		t.Helper()
		if r.err == nil {
			t.Fatalf("expected failure, got success\nstdout: %s", r.stdout)
		}
		for _, want := range wants {
			if !strings.Contains(r.stderr, want) {
				t.Fatalf("expected %q in stderr:\n%s", want, r.stderr)
			}
		}
		if strings.Contains(r.stderr, "Generated C++ code") {
			t.Fatalf("FFI diagnostics should not dump the generated C++:\n%s", r.stderr)
		}
	}

	t.Run("missing library is QK-FFI-002", func(t *testing.T) {
		prog := writeProgram(t, "missing_lib.qrk", "print(1)\nextern link 'qffi_missing'\n")
		r := runIn(work, cleanEnv(), quarkExePath, "run", prog, "--lib-path", lib)
		expectFailure(t, r, "QK-FFI-002", "'qffi_missing'", prog+":2", "searched: "+lib)
	})

	t.Run("missing include_path is QK-FFI-001", func(t *testing.T) {
		prog := writeProgram(t, "missing_include.qrk", "extern include_path 'no_such_dir'\nprint(1)\n")
		r := runIn(work, cleanEnv(), quarkExePath, "run", prog)
		expectFailure(t, r, "QK-FFI-001", "no_such_dir")
	})

	t.Run("words after a directive string are reserved", func(t *testing.T) {
		prog := writeProgram(t, "reserved_option.qrk", "extern link 'qffi_test' static\nprint(1)\n")
		r := runIn(work, cleanEnv(), quarkExePath, "run", prog, "--lib-path", lib)
		expectFailure(t, r, "QK-PARSE-001", "reserved")
	})

	t.Run("C++ error in glue header is QK-FFI-003", func(t *testing.T) {
		glue := filepath.Join(src, "broken_glue.hpp")
		if err := os.WriteFile(glue, []byte("#pragma once\ninline long long broken( { return 1; }\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		prog := writeProgram(t, "broken_glue.qrk", "extern 'broken_glue.hpp'\nprint(1)\n")
		r := runIn(work, cleanEnv(), quarkExePath, "run", prog)
		expectFailure(t, r, "QK-FFI-003", glue+":2")
	})
}

// TestStdBlas runs std/blas against the system OpenBLAS (ffi_v0_1_spec.md
// §5.3). It is skipped when OpenBLAS or its headers aren't installed.
func TestStdBlas(t *testing.T) {
	root := repoRootFromThisFile(t)
	smoke := filepath.Join(root, "src", "testfiles", "smoke_blas.qrk")
	r := runIn(filepath.Dir(smoke), cleanEnv(), quarkExePath, "run", smoke)
	if r.err != nil && (strings.Contains(r.stderr, "QK-FFI-002") ||
		(strings.Contains(r.stderr, "QK-FFI-003") && strings.Contains(r.stderr, "cblas.h"))) {
		t.Skipf("OpenBLAS not available:\n%s", r.stderr)
	}
	if r.err != nil {
		t.Fatalf("smoke_blas failed: %v\n%s", r.err, r.stderr)
	}
	want := strings.Join([]string{
		"== smoke: blas ==",
		"32.0", "5.0", "6.5",
		"6.0", "9.0", "12.0",
		"1.5",
		"4.0", "3.0",
		"2", "6.0", "15.0",
		"0.0",
	}, "\n")
	if got := strings.TrimSpace(r.stdout); got != want {
		t.Fatalf("got:\n%s\nwant:\n%s", got, want)
	}

	cases := []struct{ name, call, want string }{
		{"length mismatch", "blas::dot(vec [1.0, 2.0], vec [1.0])", "blas.dot: vec size mismatch: x has 2 elements, y has 1"},
		{"i64 input", "blas::nrm2(vec [1, 2])", "blas.nrm2: x must be an f64 vec, got i64; convert it with x.astype('f64')"},
		{"nulls", "blas::asum(list [1.0, null].to_vec())", "blas.asum: x has 1 null values"},
		{"gemv matrix size", "blas::gemv(vec [1.0, 2.0, 3.0], 2, 2, vec [1.0, 1.0])", "blas.gemv: a has 3 elements, but rows * cols = 2 * 2"},
		{"gemv x length", "blas::gemv(vec [1.0, 2.0], 1, 2, vec [1.0])", "blas.gemv: x has 1 elements, but the matrix has 2 columns"},
	}
	tmp := t.TempDir()
	for i, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			prog := filepath.Join(tmp, "blas_err_"+string(rune('a'+i))+".qrk")
			src := "incl 'std/blas' as blas\nprint(" + tc.call + ")\n"
			if err := os.WriteFile(prog, []byte(src), 0o644); err != nil {
				t.Fatal(err)
			}
			r := runIn(tmp, cleanEnv(), quarkExePath, "run", prog)
			if r.err == nil {
				t.Fatalf("expected a runtime error, got output %q", r.stdout)
			}
			if !strings.Contains(r.stderr, tc.want) {
				t.Fatalf("expected %q in stderr:\n%s", tc.want, r.stderr)
			}
		})
	}
}
