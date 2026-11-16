package main

// FFI Phase 1 integration tests (specs/ffi_v0_1_spec.md §5.2): extern link,
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

// TestStdBlas runs std/blas against the system OpenBLAS (specs/ffi_v0_1_spec.md
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
		"3", "9.0", "15.0",
		"4", "4.0", "5.0", "10.0", "11.0",
		"35.0", "44.0", "44.0", "56.0",
		"[0.0, 0.0, 0.0, 0.0]", "9", "[0.0, 0.0]",
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
		{"gemv trans x length", "blas::gemv(vec [1.0, 2.0], 1, 2, vec [1.0, 2.0], true)", "blas.gemv: x has 2 elements, but the matrix has 1 rows"},
		{"gemv negative dims", "blas::gemv(vec [], -1, 0, vec [])", "blas.gemv: rows and cols must not be negative, got rows=-1 cols=0"},
		{"gemm a size", "blas::gemm(vec [1.0, 2.0], 2, 2, vec [1.0, 2.0], 1)", "blas.gemm: a has 2 elements, but m * k = 2 * 2"},
		{"gemm b size", "blas::gemm(vec [1.0, 2.0], 1, 2, vec [1.0, 2.0, 3.0], 1)", "blas.gemm: b has 3 elements, but k * n = 2 * 1"},
		{"gemm i64 input", "blas::gemm(vec [1, 2], 1, 2, vec [1.0, 2.0], 1)", "blas.gemm: a must be an f64 vec, got i64"},
		{"gemm nulls", "blas::gemm(vec [1.0, 2.0], 1, 2, list [1.0, null].to_vec(), 1)", "blas.gemm: b has 1 null values"},
		{"gemm negative dims", "blas::gemm(vec [], 0, 2, vec [1.0, 2.0], -1)", "blas.gemm: k and n must not be negative, got k=2 n=-1"},
		{"gemm dims over INT_MAX", "blas::gemm(vec [], 4611686018427387904, 0, vec [], 2)", "blas.gemm: m=4611686018427387904 k=0: BLAS supports dimensions up to 2147483647"},
		{"gemm result over INT_MAX", "blas::gemm(vec [], 2147483647, 0, vec [], 1073741825)", "blas.gemm: the result would have 2147483647 x 1073741825 elements; BLAS vectors are limited to 2147483647"},
		{"syrk result over INT_MAX", "blas::syrk(vec [], 0, 50000)", "blas.syrk: the result would have 50000 x 50000 elements"},
		{"gemm null from any", "blas::gemm(none, 1, 1, vec [1.0], 1)", "expected vec, got null"},
		{"syrk size", "blas::syrk(vec [1.0, 2.0, 3.0], 2, 2)", "blas.syrk: a has 3 elements, but rows * cols = 2 * 2"},
		{"syrk i64 input", "blas::syrk(vec [1, 2], 1, 2)", "blas.syrk: a must be an f64 vec, got i64"},
		{"syrk wrong arity via fn value", "syrk_any(vec [1.0], 1)", "function 'blas::syrk' expects 3 arguments but got 2"},
		{"gemv trans from any", "blas::gemv(vec [1.0], 1, 1, vec [1.0], one)", "argument 'trans' of 'blas::gemv' expects bool, got int"},
	}
	tmp := t.TempDir()
	for i, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			prog := filepath.Join(tmp, "blas_err_"+string(rune('a'+i))+".qrk")
			// Values typed any, so the cases using them reach the runtime checks.
			pre := "none: any = null\none: any = 1\nsyrk_any: any = blas::syrk\n"
			src := "incl 'std/blas' as blas\n" + pre + "print(" + tc.call + ")\n"
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

// TestFFI_HandleLiveDuringNativeCall requires a handle passed to a native
// function to stay open until the function returns, however the call is
// made. The glue reads the payload after a collection, having dropped its
// own copy of the handle (as an optimizer may), so only the caller keeps it
// reachable; the close function zeroes the payload, so a handle closed
// during the call shows as a short count.
func TestFFI_HandleLiveDuringNativeCall(t *testing.T) {
	dir := t.TempDir()
	write := func(name, src string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(dir, name), []byte(src), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("live.hpp", `#pragma once
#include "quark/quark.hpp"
#include <cstdint>
QValue ql_make(int64_t n);
int64_t ql_count(QValue h);
int64_t ql_count_list(QList* hs);
`)
	write("live.cpp", `#include "live.hpp"
#include "quark/ext/api.hpp"
#include <cstdlib>
struct Data { int64_t n; int64_t* vals; };
static void close_data(void* p) {
    auto* d = static_cast<Data*>(p);
    for (int64_t i = 0; i < d->n; i++) d->vals[i] = 0;
}
static const uint16_t K = qext::define_handle_kind("qlive.data", close_data);
QValue ql_make(int64_t n) {
    auto* d = static_cast<Data*>(std::malloc(sizeof(Data)));
    d->n = n;
    d->vals = static_cast<int64_t*>(std::malloc(sizeof(int64_t) * n));
    for (int64_t i = 0; i < n; i++) d->vals[i] = 1;
    return qext::new_handle(K, d, qext::null_val());
}
static int64_t count(Data* d) {
    volatile char sink[65536];
    for (int i = 0; i < 65536; i++) sink[i] = 0;
    GC_gcollect();
    GC_invoke_finalizers();
    int64_t c = 0;
    for (int64_t i = 0; i < d->n; i++) c += d->vals[i];
    return c;
}
int64_t ql_count(QValue h) {
    Data* d = qext::handle_get<Data>(h, K);
    h.data.resource_val = nullptr;
    return count(d);
}
int64_t ql_count_list(QList* hs) {
    Data* d = qext::handle_get<Data>((*hs)[0], K);
    hs = nullptr;
    return count(d);
}
`)
	write("main.qrk", `extern 'live.hpp'
extern source 'live.cpp'
extern fn make(n: int) resource as 'ql_make'
extern fn count(h: resource) int as 'ql_count'
extern fn count_list(hs: list) int as 'ql_count_list'
print(count(make(1000)))
h = make(1000)
print(count(h))
f = count
print(f(make(1000)))
fn id(v) -> v
print(count(id(make(1000))))
print(count_list(list [make(1000)]))
`)
	r := runIn(dir, cleanEnv(), quarkExePath, "run", filepath.Join(dir, "main.qrk"))
	if r.err != nil {
		t.Fatalf("run failed: %v\n%s", r.err, r.stderr)
	}
	want := "1000\n1000\n1000\n1000\n1000"
	if got := strings.TrimSpace(normalizeNewlines(r.stdout)); got != want {
		t.Fatalf("a handle was closed while a native call used it:\n%s\nwant:\n%s", got, want)
	}
}

// TestFFI_Phase2 covers separately compiled glue (P2), extension handles and
// finalizers (P3) and qext result helpers (P4). It does not need a system
// library, so it runs on every platform the compiler does.
func TestFFI_Phase2(t *testing.T) {
	dir := t.TempDir()
	write := func(name, src string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(dir, name), []byte(src), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("handles.hpp", handlesHeader)
	write("handles.cpp", handlesSource)
	write("handles.qrk", handlesQuark)
	run := func(src string, extra ...string) quarkResult {
		t.Helper()
		write("main.qrk", src)
		args := append([]string{"run", filepath.Join(dir, "main.qrk")}, extra...)
		return runIn(dir, cleanEnv(), quarkExePath, args...)
	}
	mustOK := func(t *testing.T, r quarkResult, want string) {
		t.Helper()
		if r.err != nil {
			t.Fatalf("run failed: %v\n%s", r.err, r.stderr)
		}
		if got := strings.TrimSpace(r.stdout); got != want {
			t.Fatalf("got:\n%s\nwant:\n%s\nstderr:\n%s", got, want, r.stderr)
		}
	}
	mustFail := func(t *testing.T, r quarkResult, wants ...string) {
		t.Helper()
		if r.err == nil {
			t.Fatalf("expected failure, got:\n%s", r.stdout)
		}
		for _, want := range wants {
			if !strings.Contains(r.stderr, want) {
				t.Fatalf("expected %q in stderr:\n%s", want, r.stderr)
			}
		}
	}

	t.Run("handles, results and finalizers", func(t *testing.T) {
		mustOK(t, run(`incl './handles'
h = handles::make_item(4)
print(h)
print(handles::read_item(h))
fn id(v) -> v
print(handles::read_item(id(h)))
when handles::ok_n():
    ok n -> print(n)
    err m -> print(m)
when handles::err_s():
    ok n -> print(n)
    err m -> print(m)
when handles::errf_s():
    ok n -> print(n)
    err m -> print(m)
handles::close_item(h)
handles::close_item(h)
print(handles::item_closes())
h = null
print(handles::sweep())
print(handles::hold_item())
print(handles::release_item())
print(handles::release_item())
print(handles::hold_pair())
print(handles::release_child())
print(handles::release_parent())
`), strings.Join([]string{
			"<resource qffi.item>",
			"4",
			"4",
			"7",
			"nope",
			"bad 3",
			"1",
			"1",
			"0",
			"2",
			"2",
			"0",
			"1 0",
			"1 1",
		}, "\n"))
	})

	t.Run("parent is finalized after the child", func(t *testing.T) {
		mustOK(t, run(`incl './handles'
print(handles::hold_pair())
print(handles::release_both_once())
print(handles::release_both_again())
`), "0\n1 0\n1 1")
	})

	t.Run("file handle is closed when it becomes unreachable", func(t *testing.T) {
		root := repoRootFromThisFile(t)
		stdlib := filepath.Join(root, "src", "core", "stdlib")
		write("file.hpp", fileFinalizerHeader)
		write("file.cpp", fileFinalizerSource)
		mustOK(t, run("extern include_path '"+filepath.ToSlash(stdlib)+"'\nextern 'file.hpp'\nextern source 'file.cpp'\nextern fn hold() int as 'qffi_hold_file'\nextern fn closed() int as 'qffi_file_closed'\nprint(hold())\nprint(closed())\n"), "0\n1")
	})

	t.Run("wrong kind, closed handle, null, any, default", func(t *testing.T) {
		mustFail(t, run("incl './handles'\nprint(handles::read_item(handles::make_box(1)))\n"), "QK-FFI-004", "expected qffi.item, got qffi.box")
		mustFail(t, run("incl './handles'\nh = handles::make_item(1)\nhandles::close_item(h)\nprint(handles::read_item(h))\n"), "QK-FFI-004", "qffi.item is closed")
		mustFail(t, run("incl './handles'\nprint(handles::read_item(null))\n"), "QK-FFI-004", "expected qffi.item, got null")
		mustFail(t, run("incl './handles'\nfn id(v) -> v\nprint(handles::read_item(id('x')))\n"), "QK-FFI-004", "expected qffi.item, got str")
		mustFail(t, run("incl './handles'\nfn read_default(h: resource = null) -> handles::read_item(h)\nprint(read_default())\n"), "QK-FFI-004", "expected qffi.item, got null")
		mustFail(t, run("incl './handles'\nprint(handles::close_item(null))\n"), "QK-FFI-004", "handle_close: expected a resource, got null")
		mustFail(t, run("incl './handles'\nprint(handles::null_payload())\n"), "new_handle: null payload")
		mustFail(t, run("incl './handles'\nprint(handles::bad_kind())\n"), "unknown handle kind")
	})

	t.Run("wrong type and arity are compile errors", func(t *testing.T) {
		mustFail(t, run("incl './handles'\nprint(handles::read_item(1))\n"), "resource")
		mustFail(t, run("incl './handles'\nprint(handles::read_item())\n"), "argument")
	})

	t.Run("missing source is QK-FFI-001", func(t *testing.T) {
		mustFail(t, run("extern source 'no_such.cpp'\nprint(1)\n"), "QK-FFI-001", "no_such.cpp")
	})

	t.Run("source inside a module is QK-LOAD-001", func(t *testing.T) {
		mustFail(t, run("module m:\n    extern source 'glue.cpp'\n    fn f() -> 1\nprint(m::f())\n"), "QK-LOAD-001", "extern source must be at the top level")
	})

	t.Run("C++ error in a glue source is QK-FFI-003", func(t *testing.T) {
		write("bad.cpp", "#include <cstdint>\nint64_t qffi_bad( { return 1; }\n")
		write("bad.hpp", "#include <cstdint>\nint64_t qffi_bad();\n")
		mustFail(t, run("extern 'bad.hpp'\nextern source 'bad.cpp'\nextern fn bad() int as 'qffi_bad'\nprint(bad())\n"), "QK-FFI-003", "bad.cpp")
	})

	t.Run("duplicate kind name fails", func(t *testing.T) {
		write("dup.hpp", "#include <cstdint>\nint64_t qffi_dup();\n")
		write("dup.cpp", dupSource)
		mustFail(t, run("extern 'dup.hpp'\nextern source 'dup.cpp'\nextern fn go() int as 'qffi_dup'\nprint(go())\n"), "already defined", "qffi.dup")
	})

	t.Run("one source linked from two modules is compiled once", func(t *testing.T) {
		write("once.hpp", "#include <cstdint>\nint64_t qffi_once();\n")
		write("once.cpp", "#include <cstdint>\nint64_t qffi_once() { return 11; }\n")
		write("a.qrk", "extern 'once.hpp'\nextern source 'once.cpp'\nextern fn once() int as 'qffi_once'\n")
		write("b.qrk", "extern 'once.hpp'\nextern source 'once.cpp'\nextern fn once() int as 'qffi_once'\n")
		mustOK(t, run("incl './a'\nincl './b'\nprint(a::once())\nprint(b::once())\n"), "11\n11")
	})

	t.Run("object cache", func(t *testing.T) {
		write("answer.hpp", "#include <cstdint>\nint64_t qffi_answer();\n")
		write("answer.cpp", "#include <cstdint>\nint64_t qffi_answer() { return 7; }\n")
		src := "extern 'answer.hpp'\nextern source 'answer.cpp'\nextern fn answer() int as 'qffi_answer'\nprint(answer())\n"
		first := run(src, "--debug")
		mustOK(t, first, "7")
		if !strings.Contains(first.stderr, "compiling extern source:") {
			t.Fatalf("first build should compile the source:\n%s", first.stderr)
		}
		if strings.Contains(first.stderr, "extern source cache hit:") {
			t.Fatalf("first build should not be a cache hit:\n%s", first.stderr)
		}
		second := run(src, "--debug")
		mustOK(t, second, "7")
		if !strings.Contains(second.stderr, "extern source cache hit:") {
			t.Fatalf("second build should reuse the object:\n%s", second.stderr)
		}
		write("answer.cpp", "#include <cstdint>\nint64_t qffi_answer() { return 9; }\n")
		third := run(src, "--debug")
		mustOK(t, third, "9")
		if !strings.Contains(third.stderr, "compiling extern source:") {
			t.Fatalf("a changed source should be compiled again:\n%s", third.stderr)
		}
	})
}

const handlesHeader = `#pragma once
#include "quark/quark.hpp"
#include <cstdint>
QValue qffi_make(int64_t n);
QValue qffi_make_box(int64_t n);
int64_t qffi_read(QValue h);
int64_t qffi_close(QValue h);
int64_t qffi_closes();
int64_t qffi_sweep();
int64_t qffi_hold_item();
int64_t qffi_release_item();
int64_t qffi_hold_pair();
const char* qffi_release_child();
const char* qffi_release_parent();
const char* qffi_release_both_once();
const char* qffi_release_both_again();
QValue qffi_ok();
QValue qffi_err();
QValue qffi_errf();
QValue qffi_null_payload();
QValue qffi_bad_kind();
`

const handlesSource = `#include "handles.hpp"
#include <cstdlib>
#include <cstdio>

static int item_closes = 0;
static int box_closes = 0;
static int child_closes = 0;
static int parent_closes = 0;

static void close_item(void* p) { item_closes++; std::free(p); }
static void close_box(void* p) { box_closes++; std::free(p); }
static void close_child(void* p) { child_closes++; std::free(p); }
static void close_parent(void* p) { parent_closes++; std::free(p); }

static const uint16_t KIND_ITEM = qext::define_handle_kind("qffi.item", close_item);
static const uint16_t KIND_BOX = qext::define_handle_kind("qffi.box", close_box);
static const uint16_t KIND_CHILD = qext::define_handle_kind("qffi.child", close_child);
static const uint16_t KIND_PARENT = qext::define_handle_kind("qffi.parent", close_parent);

static QValue held_item;
static QValue held_child;
static QValue held_parent;
static char status_buf[32];

static void drop_value(QValue& v) {
    v.type = QValue::VAL_NULL;
    v.data.resource_val = nullptr;
}

static void scrub_and_collect() {
    volatile char sink[65536];
    for (int i = 0; i < 65536; i++) sink[i] = 0;
    GC_gcollect();
    GC_invoke_finalizers();
}

static const char* pair_status() {
    std::snprintf(status_buf, sizeof(status_buf), "%d %d", child_closes, parent_closes);
    return status_buf;
}

static int64_t* owned_int(int64_t n) {
    auto* p = static_cast<int64_t*>(std::malloc(sizeof(int64_t)));
    *p = n;
    return p;
}

QValue qffi_make(int64_t n) {
    return qext::new_handle(KIND_ITEM, owned_int(n), qext::null_val());
}

QValue qffi_make_box(int64_t n) {
    return qext::new_handle(KIND_BOX, owned_int(n), qext::null_val());
}

int64_t qffi_read(QValue h) {
    return *qext::handle_get<int64_t>(h, KIND_ITEM);
}

int64_t qffi_close(QValue h) {
    qext::handle_close(h);
    return 0;
}

int64_t qffi_closes() { return item_closes; }

int64_t qffi_sweep() {
    scrub_and_collect();
    return item_closes;
}

int64_t qffi_hold_item() {
    held_item = qext::new_handle(KIND_ITEM, owned_int(5), qext::null_val());
    return 0;
}

int64_t qffi_release_item() {
    drop_value(held_item);
    scrub_and_collect();
    return item_closes;
}

int64_t qffi_hold_pair() {
    held_parent = qext::new_handle(KIND_PARENT, owned_int(1), qext::null_val());
    held_child = qext::new_handle(KIND_CHILD, owned_int(2), held_parent);
    return 0;
}

const char* qffi_release_child() {
    drop_value(held_child);
    scrub_and_collect();
    return pair_status();
}

const char* qffi_release_parent() {
    drop_value(held_parent);
    scrub_and_collect();
    return pair_status();
}

const char* qffi_release_both_once() {
    drop_value(held_child);
    drop_value(held_parent);
    scrub_and_collect();
    return pair_status();
}

const char* qffi_release_both_again() {
    scrub_and_collect();
    return pair_status();
}

QValue qffi_ok() { return qext::ok(qext::box(static_cast<int64_t>(7))); }
QValue qffi_err() { return qext::err("nope"); }
QValue qffi_errf() { return qext::errf("bad %d", 3); }

QValue qffi_null_payload() {
    return qext::new_handle(KIND_ITEM, nullptr, qext::null_val());
}

QValue qffi_bad_kind() {
    return qext::new_handle(99, owned_int(1), qext::null_val());
}
`

const handlesQuark = `extern 'handles.hpp'
extern source 'handles.cpp'
extern fn make_item(n: int) resource as 'qffi_make'
extern fn make_box(n: int) resource as 'qffi_make_box'
extern fn read_item(h: resource) int as 'qffi_read'
extern fn close_item(h: resource) int as 'qffi_close'
extern fn item_closes() int as 'qffi_closes'
extern fn sweep() int as 'qffi_sweep'
extern fn hold_item() int as 'qffi_hold_item'
extern fn release_item() int as 'qffi_release_item'
extern fn hold_pair() int as 'qffi_hold_pair'
extern fn release_child() str as 'qffi_release_child'
extern fn release_parent() str as 'qffi_release_parent'
extern fn release_both_once() str as 'qffi_release_both_once'
extern fn release_both_again() str as 'qffi_release_both_again'
extern fn ok_n() result as 'qffi_ok'
extern fn err_s() result as 'qffi_err'
extern fn errf_s() result as 'qffi_errf'
extern fn null_payload() resource as 'qffi_null_payload'
extern fn bad_kind() resource as 'qffi_bad_kind'
`

const fileFinalizerHeader = `#pragma once
#include <cstdint>
int64_t qffi_hold_file();
int64_t qffi_file_closed();
`

const fileFinalizerSource = `#include "quark/quark.hpp"
#include "io.hpp"
#include <cstdint>

static QValue held_file;
static int file_closes = 0;

static void counting_close(void* payload) {
    file_closes++;
    q_file_payload_close(payload);
}

static void drop_value(QValue& v) {
    v.type = QValue::VAL_NULL;
    v.data.resource_val = nullptr;
}

static void scrub_and_collect() {
    volatile char sink[65536];
    for (int i = 0; i < 65536; i++) sink[i] = 0;
    GC_gcollect();
    GC_invoke_finalizers();
}

int64_t qffi_hold_file() {
    QValue opened = q_file_open("notes.txt", "w", false);
    if (opened.type != QValue::VAL_RESULT || opened.data.result_val == nullptr || !opened.data.result_val->is_ok) {
        return 1;
    }
    held_file = opened.data.result_val->payload;
    QResourceSlot* slot = nullptr;
    QResourceHandle* handle = nullptr;
    const char* err = nullptr;
    if (!q_resource_resolve(held_file, QRES_KIND_FILE, slot, handle, err) || slot->payload == nullptr) {
        return 2;
    }
    // The finalizer looks the close function up when it runs, so this
    // replaces the one q_file_open registered and counts that run.
    q_resource_set_close(QRES_KIND_FILE, counting_close);
    return 0;
}

int64_t qffi_file_closed() {
    drop_value(held_file);
    scrub_and_collect();
    return file_closes;
}
`

const dupSource = `#include "quark/quark.hpp"
#include <cstdint>
static void close_dup(void*) {}
static const uint16_t KIND_DUP_A = qext::define_handle_kind("qffi.dup", close_dup);
static const uint16_t KIND_DUP_B = qext::define_handle_kind("qffi.dup", close_dup);
int64_t qffi_dup() { return static_cast<int64_t>(KIND_DUP_A + KIND_DUP_B); }
`
