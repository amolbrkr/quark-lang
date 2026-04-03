package main

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

var quarkExePath string

func TestMain(m *testing.M) {
	// Build quark once for all integration tests.
	// IMPORTANT: build into src/core/quark (same as normal usage) so
	// getRuntimeIncludePath/getGCPaths can resolve runtime/ and deps/.
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		os.Exit(1)
	}
	pkgDir := filepath.Dir(thisFile)
	exeName := "quark_test"
	if runtime.GOOS == "windows" {
		exeName += ".exe"
	}
	quarkExePath = filepath.Join(pkgDir, exeName)
	_ = os.Remove(quarkExePath)
	defer os.Remove(quarkExePath)

	buildCmd := exec.Command("go", "build", "-o", quarkExePath, ".")
	buildCmd.Stdout = os.Stdout
	buildCmd.Stderr = os.Stderr
	buildCmd.Dir = pkgDir
	if err := buildCmd.Run(); err != nil {
		os.Exit(1)
	}

	os.Exit(m.Run())
}

func repoRootFromThisFile(t *testing.T) string {
	t.Helper()
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("unable to locate test file path")
	}
	// this file is: <repo>/src/core/quark/integration_smoke_test.go
	dir := filepath.Dir(thisFile)
	return filepath.Clean(filepath.Join(dir, "..", "..", ".."))
}

func normalizeNewlines(s string) string {
	return strings.ReplaceAll(s, "\r\n", "\n")
}

func runQuark(t *testing.T, args ...string) string {
	t.Helper()
	cmd := exec.Command(quarkExePath, args...)
	var out bytes.Buffer
	var errBuf bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errBuf
	if err := cmd.Run(); err != nil {
		t.Fatalf("quark %v failed: %v\n--- stderr ---\n%s\n--- stdout ---\n%s", args, err, errBuf.String(), out.String())
	}
	return out.String()
}

func TestSmokePrograms_Run(t *testing.T) {
	root := repoRootFromThisFile(t)
	testfilesDir := filepath.Join(root, "src", "testfiles")

	join := func(lines ...string) string {
		return strings.Join(lines, "\n")
	}

	cases := []struct {
		name     string
		file     string
		expected string
	}{
		{
			name: "syntax",
			file: filepath.Join(testfilesDir, "smoke_syntax.qrk"),
			expected: join(
				"== smoke: syntax ==",
				"7",
				"512",
				"-5",
				"true",
				"seven",
				"big",
				"seven or eight",
				"720",
				"6",
				"15",
				"3",
				"2",
				"1",
				"5",
				"division by zero",
			),
		},
		{
			name: "modules_success",
			file: filepath.Join(testfilesDir, "smoke_modules_success.qrk"),
			expected: join(
				"== smoke: modules success ==",
				"10",
				"9",
				"16",
				"18",
				"42",
			),
		},
		{
			name: "types",
			file: filepath.Join(testfilesDir, "smoke_types.qrk"),
			expected: join(
				"== smoke: types ==",
				"42",
				"3.14",
				"hello",
				"true",
				"null",
				"3",
				"quark",
				"3",
				"1",
				"99",
				"1",
				"2",
				"99",
				"null",
				"123",
				"[vector len=4]",
				"4",
				"[vector len=4]",
				"[vector len=4]",
				"10",
				"1",
				"4",
				"[vector len=4]",
				"10",
				"1",
				"4",
				"[vector len=4]",
				"[vector len=4]",
				"[vector len=4]",
				"vector[i64]",
				"vector[i64]",
				"[vector len=3]",
				"60",
				"[list len=3]",
				"list",
				"10",
			),
		},
		{
			name: "string_list_builtins",
			file: filepath.Join(testfilesDir, "smoke_string_list_builtins.qrk"),
			expected: join(
				"== smoke: string/list builtins ==",
				"HELLO WORLD",
				"hello world",
				"hello",
				"true",
				"false",
				"true",
				"true",
				"hello quark",
				"hello world",
				"hello quark",
				"3",
				"10",
				"30",
				"null",
				"40",
				"99",
				"10",
				"[list len=2]",
				"[list len=3]",
				"[list len=5]",
				"[list len=5]",
				"[list len=5]",
				"[list len=4]",
			),
		},
		{
			name: "break_continue",
			file: filepath.Join(testfilesDir, "smoke_break_continue.qrk"),
			expected: join(
				"== smoke: break/continue ==",
				"0",
				"1",
				"2",
				"3",
				"4",
				"1",
				"3",
				"5",
				"0",
				"1",
				"2",
				"1",
				"3",
				"5",
				"0",
				"1",
				"0",
				"1",
				"0",
				"1",
			),
		},
		{
			name: "stdlib_io",
			file: filepath.Join(testfilesDir, "smoke_stdlib_io.qrk"),
			expected: join(
				"== smoke: stdlib io ==",
				"11",
				"true",
				"6",
				"world",
			),
		},
		{
			name: "string_slice_join",
			file: filepath.Join(testfilesDir, "smoke_string_slice_join.qrk"),
			expected: join(
				"== smoke: sslice and sjoin ==",
				"hello",
				"world",
				"cd",
				"llo",
				"hel",
				"ll",
				"hi",
				"",
				"",
				"WORLD",
				"hello world",
				"a,b,c",
				"foobar",
				"only",
				"",
				"1+2+3",
				"a-b-c",
				"one|two|three",
			),
		},
		{
			name: "prefixed_builtins",
			file: filepath.Join(testfilesDir, "smoke_prefixed_builtins.qrk"),
			expected: join(
				"== smoke: prefixed builtins ==",
				"3",
				"20",
				"hello",
				"true",
				"2",
				"2",
				"list",
				"list",
				"2",
				"row.....|00000042",
			),
		},
		{
			name: "default_params",
			file: filepath.Join(testfilesDir, "smoke_default_params.qrk"),
			expected: join(
				"== smoke: default parameters ==",
				"Hello World",
				"Hi World",
				"localhost:8080",
				"localhost:3000",
				"localhost:3000",
				"3",
				"5",
				"test",
				"[VERBOSE] test",
				"10",
				"15",
				"15",
				"25",
				"5",
				"10",
				"5",
				"null",
				"9",
				"15",
				"Result: 42!",
				"Result: 42.",
			),
		},
		{
			name: "functions_closures",
			file: filepath.Join(testfilesDir, "smoke_functions_closures.qrk"),
			expected: join(
				"55",
				"1",
				"2",
			),
		},
		{
			name: "return_types",
			file: filepath.Join(testfilesDir, "smoke_return_types.qrk"),
			expected: join(
				"== smoke: return type annotations ==",
				"7",
				"10",
				"3.14",
				"Hello, World",
				"true",
				"false",
				"5",
				"division by zero",
				"36",
				"zero",
				"one",
				"other",
				"5",
				"3",
				"3",
				"10",
				"120",
			),
		},
		{
			name: "vectors",
			file: filepath.Join(testfilesDir, "smoke_vectors.qrk"),
			expected: join(
				"== smoke: vectors ==",
				"vector[i64]",
				"vector[str]",
				"vector[str]",
				"[vector len=5]",
				"3",
				"2",
				"3",
				"3",
				"1",
				"4",
				"1",
				"[vector len=3]",
				"3",
				"10",
				"50",
				"50",
				"120",
				"0",
				"[vector len=2]",
				"1",
				"4",
			),
		},
		{
			name: "vector_soundness",
			file: filepath.Join(testfilesDir, "smoke_vector_soundness.qrk"),
			expected: join(
				"== smoke: vector soundness ==",
				"vector[f64]",
				"13.5",
				"vector[f64]",
				"10.5",
				"vector[f64]",
				"vector[f64]",
				"12",
				"vector[bool]",
				"2",
				"vector[bool]",
				"2",
				"4",
				"1",
				"3",
				"15",
			),
		},
		{
			name: "modules_error_graph",
			file: filepath.Join(testfilesDir, "smoke_modules_error_graph.qrk"),
			expected: join(
				"2",
			),
		},
		{
			name: "stdlib_fmt",
			file: filepath.Join(testfilesDir, "smoke_stdlib_fmt.qrk"),
			expected: join(
				"== smoke: stdlib fmt ==",
				"   value",
				"--------",
				"0     10",
				"1     20",
				"2     30",
				"... (4 rows omitted) ...",
				"7     80",
				"8     90",
				"9    100",
				"[list: 10 elements]",
				"value",
				"-----",
				"    1",
				"    2",
				"    3",
				"[list: 3 elements]",
				"   value",
				"--------",
				"0     10",
				"1     20",
				"2     30",
				"[vector[i64]: 3 elements]",
				"   key   value",
				"-------  -----",
				"0  city  NYC  ",
				"1  pop   8M   ",
				"[dict: 2 keys]",
				"+---+-------+-------+",
				"| # | name  | score |",
				"+---+-------+-------+",
				"| 0 | Alice |    95 |",
				"| 1 | Bob   |    87 |",
				"+---+-------+-------+",
				"[2 rows x 2 cols]",
				"   value",
				"--------",
				"0      1",
				"1      2",
				"2      3",
				"[list: 3 elements]",
				"   value",
				"--------",
				"0      8",
				"1      9",
				"2     10",
				"[list: 3 elements]",
			),
		},
		{
			name: "numeric_lowering",
			file: filepath.Join(testfilesDir, "smoke_numeric_lowering.qrk"),
			expected: join(
				"== smoke: numeric lowering ==",
				"10",
				"4",
				"4.5",
				"true",
				"35",
				"true",
				"8",
				"15",
				"3.5",
				"true",
				"true",
				"false",
				"6",
				"good",
			),
		},
		{
			name: "truthiness",
			file: filepath.Join(testfilesDir, "smoke_truthiness.qrk"),
			expected: join(
				"== smoke: truthiness ==",
				"int truthy",
				"empty str falsy",
				"list truthy",
				"null falsy",
				"zero falsy",
				"3",
				"2",
				"1",
				"yes",
				"0",
				"42",
				"5",
				"7",
				"true",
				"false",
				"false",
				"true",
				"true",
				"guard ok",
				"guard matched",
				"3",
				"fallback",
				"mixed ok",
			),
		},
		{
			name: "native_fns",
			file: filepath.Join(testfilesDir, "smoke_native_fns.qrk"),
			expected: join(
				"== smoke: native functions ==",
				"7",
				"6",
				"true",
				"false",
				"30",
				"14",
				"5",
				"0",
				"10",
				"12",
			),
		},
		{
			name: "range_lowering",
			file: filepath.Join(testfilesDir, "smoke_range_lowering.qrk"),
			expected: join(
				"== smoke: range lowering ==",
				"10",
				"25",
				"15",
				"18",
				"30",
				"0",
				"9",
				"6",
			),
		},
		{
			name: "extern_fns",
			file: filepath.Join(testfilesDir, "smoke_extern.qrk"),
			expected: join(
				"== smoke: extern fns ==",
				"10",
				"3.5",
				"true",
				"false",
				"hello quark",
				"5",
				"21",
			),
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.name == "modules_success" {
				modulesFixtureDir := filepath.Join(testfilesDir, "lib")
				if _, err := os.Stat(modulesFixtureDir); err != nil {
					t.Skipf("skipping %s: missing module fixtures at %s", tc.name, modulesFixtureDir)
				}

				coreFixture := filepath.Join(modulesFixtureDir, "core.qrk")
				coreSource, err := os.ReadFile(coreFixture)
				if err != nil {
					t.Skipf("skipping %s: missing module fixture file %s", tc.name, coreFixture)
				}
				if !strings.Contains(string(coreSource), "module core") {
					t.Skipf("skipping %s: %s no longer exposes module core", tc.name, coreFixture)
				}
			}

			got := runQuark(t, "run", tc.file)
			gotNorm := strings.TrimSpace(normalizeNewlines(got))
			expNorm := strings.TrimSpace(normalizeNewlines(tc.expected))
			if gotNorm != expNorm {
				t.Fatalf("unexpected output\n--- got ---\n%s\n--- expected ---\n%s", gotNorm, expNorm)
			}
		})
	}
}

func TestSmokePrograms_CompileError(t *testing.T) {
	root := repoRootFromThisFile(t)
	testfilesDir := filepath.Join(root, "src", "testfiles")

	cases := []struct {
		name      string
		file      string
		errSubstr string
	}{
		{
			name:      "modules_error_resolve",
			file:      filepath.Join(testfilesDir, "smoke_modules_error_resolve.qrk"),
			errSubstr: "cannot find module",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cmd := exec.Command(quarkExePath, "run", tc.file)
			var out bytes.Buffer
			var errBuf bytes.Buffer
			cmd.Stdout = &out
			cmd.Stderr = &errBuf
			err := cmd.Run()
			if err == nil {
				t.Fatalf("expected compile error for %s, but it succeeded with output:\n%s", tc.name, out.String())
			}
			combined := out.String() + errBuf.String()
			if !strings.Contains(combined, tc.errSubstr) {
				t.Fatalf("expected error containing %q, got:\nstdout: %s\nstderr: %s", tc.errSubstr, out.String(), errBuf.String())
			}
		})
	}
}

func TestRuntimeContracts_FunctionTypeNameAndDoubleQuotedStrings(t *testing.T) {
	tmp := t.TempDir()
	program := filepath.Join(tmp, "runtime_contracts.qrk")
	source := strings.Join([]string{
		"fn add(x, y) -> x + y",
		"println(type(add))",
		"println(\"hello\\nworld\")",
		"r: result = ok 1",
		"println(type(r))",
		"",
	}, "\n")
	if err := os.WriteFile(program, []byte(source), 0o644); err != nil {
		t.Fatalf("write %s: %v", program, err)
	}

	got := runQuark(t, "run", program)
	gotNorm := strings.TrimSpace(normalizeNewlines(got))
	expected := strings.Join([]string{"fn", "hello", "world", "result"}, "\n")
	if gotNorm != expected {
		t.Fatalf("unexpected output\n--- got ---\n%s\n--- expected ---\n%s", gotNorm, expected)
	}
}

func TestDefaults_DynamicCallViaAlias(t *testing.T) {
	tmp := t.TempDir()
	program := filepath.Join(tmp, "defaults_dynamic_alias.qrk")
	source := strings.Join([]string{
		"fn add_n(x: int, n: int = 10) int -> x + n",
		"alias = add_n",
		"println(alias(5))",
		"println(alias(5, 20))",
		"alias2 = alias",
		"println(alias2(7))",
		"5 | alias() | println()",
		"",
	}, "\n")
	if err := os.WriteFile(program, []byte(source), 0o644); err != nil {
		t.Fatalf("write %s: %v", program, err)
	}

	got := runQuark(t, "run", program)
	gotNorm := strings.TrimSpace(normalizeNewlines(got))
	expected := strings.Join([]string{"15", "25", "17", "15"}, "\n")
	if gotNorm != expected {
		t.Fatalf("unexpected output\n--- got ---\n%s\n--- expected ---\n%s", gotNorm, expected)
	}
}

func TestFileBuiltins_V0(t *testing.T) {
	tmp := t.TempDir()
	targetPath := filepath.Join(tmp, "io_v0.txt")
	targetForQuark := strings.ReplaceAll(targetPath, "\\", "/")

	program := filepath.Join(tmp, "file_builtins_v0.qrk")
	source := strings.Join([]string{
		"p = '" + targetForQuark + "'",
		"f = unwrap(_file_open(p, 'w'))",
		"println(type(f))",
		"println(type(_file_exists(p)))",
		"_file_write(f, 'hello file') | unwrap() | println()",
		"_file_close(f) | unwrap()",
		"r = unwrap(_file_open(p, 'r'))",
		"println(unwrap(_file_read(r, 64)))",
		"_file_close(r) | unwrap()",
		"println(_file_exists(p))",
		"",
	}, "\n")
	if err := os.WriteFile(program, []byte(source), 0o644); err != nil {
		t.Fatalf("write %s: %v", program, err)
	}

	got := runQuark(t, "run", program)
	gotNorm := strings.TrimSpace(normalizeNewlines(got))
	expected := strings.Join([]string{"file_handle", "bool", "10", "hello file", "true"}, "\n")
	if gotNorm != expected {
		t.Fatalf("unexpected output\n--- got ---\n%s\n--- expected ---\n%s", gotNorm, expected)
	}
}

// TestGCManagedSelfCheck compiles a standalone C++ program that allocates core
// runtime values and verifies (via GC_base) that object and internal storage
// buffers land on the Boehm GC heap.
func TestGCManagedSelfCheck(t *testing.T) {
	// Quark requires clang++.
	compiler := "clang++"
	if _, err := exec.LookPath("clang++"); err != nil {
		t.Skip("skipping: clang++ not found in PATH")
	}

	// Resolve runtime include and GC paths (same helpers the compiler uses).
	runtimeInclude := getRuntimeIncludePath()
	gcInclude, gcLib, err := ensureGC()
	if err != nil {
		t.Fatalf("ensureGC: %v", err)
	}

	tmp := t.TempDir()
	cppFile := filepath.Join(tmp, "gc_selfcheck.cpp")
	source := `
#include "quark/quark.hpp"
#include <cstdio>

int main() {
    q_gc_init();

    int pass = 0;
    int fail = 0;

    #define CHECK(label, ptr) do {                                       \
        if (GC_base((void*)(ptr)) != nullptr) { pass++; }               \
        else { fail++; std::fprintf(stderr, "FAIL: %s not GC-managed\n", label); } \
    } while(0)

    /* --- strings --- */
    QValue s = qv_string("hello");
    CHECK("string", s.data.string_val);

    /* --- list (object + internal buffer via gc_allocator) --- */
    QValue l = qv_list(4);
    CHECK("list_object", l.data.list_val);
    l.data.list_val->push_back(qv_int(1));
    CHECK("list_buffer", l.data.list_val->data());

	/* --- dict (object via gc_allocator; keys are plain std::string, not GC-managed) --- */
    QValue d = qv_dict();
    CHECK("dict_object", d.data.dict_val);

	/* --- vectors: object + per-type storage buffers --- */
	QValue vf = qv_vector(4);
	CHECK("vector_f64_object", vf.data.vector_val);
	vf = q_vec_push(vf, qv_float(1.25));
	CHECK("vector_f64_buffer", std::get<QVecF64>(vf.data.vector_val->storage).data());

	QValue vi = qv_vector_i64(4);
	CHECK("vector_i64_object", vi.data.vector_val);
	vi = q_vec_push_i64(vi, qv_int(7));
	CHECK("vector_i64_buffer", std::get<QVecI64>(vi.data.vector_val->storage).data());

	QValue vb = qv_vector_bool(4);
	CHECK("vector_bool_object", vb.data.vector_val);
	vb = q_vec_push_bool(vb, qv_bool(true));
	CHECK("vector_bool_buffer", std::get<QVecU8>(vb.data.vector_val->storage).data());

	QValue vs = q_to_vector(qv_list_from(1, qv_string("hello")));
	CHECK("vector_str_object", vs.data.vector_val);
	const auto& sstorage = std::get<QStringStorage>(vs.data.vector_val->storage);
	CHECK("vector_str_offsets", sstorage.offsets.data());
	CHECK("vector_str_bytes", sstorage.bytes.data());

    /* --- closure --- */
    QClosure* cl = q_alloc_closure(nullptr, 0);
    CHECK("closure", cl);

    /* --- cell --- */
    QCell* cell = q_new_cell(qv_int(1));
    CHECK("cell", cell);

    /* --- ok result --- */
    QValue rok = qv_ok(qv_int(42));
    CHECK("ok_result", rok.data.result_val);

    /* --- err result --- */
    QValue rerr = qv_err(qv_string("oops"));
    CHECK("err_result", rerr.data.result_val);

	std::printf("gc_selfcheck: %d passed, %d failed\n", pass, fail);
    return fail > 0 ? 1 : 0;
}
`
	if err := os.WriteFile(cppFile, []byte(source), 0o644); err != nil {
		t.Fatalf("write %s: %v", cppFile, err)
	}

	outName := "gc_selfcheck"
	if runtime.GOOS == "windows" {
		outName += ".exe"
	}
	outBin := filepath.Join(tmp, outName)

	args := []string{
		"-std=c++17", "-O0",
		"-DQUARK_USE_GC",
		"-I" + runtimeInclude,
		"-I" + gcInclude,
		"-o", outBin,
		cppFile,
		gcLib,
	}
	if runtime.GOOS != "windows" {
		args = append(args, "-lm")
	}

	compileCmd := exec.Command(compiler, args...)
	var compileBuf bytes.Buffer
	compileCmd.Stdout = &compileBuf
	compileCmd.Stderr = &compileBuf
	if err := compileCmd.Run(); err != nil {
		t.Fatalf("compile gc_selfcheck failed:\n%s\n%v", compileBuf.String(), err)
	}

	runCmd := exec.Command(outBin)
	var runOut, runErr bytes.Buffer
	runCmd.Stdout = &runOut
	runCmd.Stderr = &runErr
	if err := runCmd.Run(); err != nil {
		t.Fatalf("gc_selfcheck failed:\nstdout: %s\nstderr: %s\n%v",
			runOut.String(), runErr.String(), err)
	}

	got := strings.TrimSpace(runOut.String())
	if !strings.Contains(got, "0 failed") {
		t.Fatalf("gc_selfcheck reported failures:\nstdout: %s\nstderr: %s",
			got, runErr.String())
	}
	t.Logf("gc_selfcheck: %s", got)
}

func TestAnyTypeAnnotations_Runtime(t *testing.T) {
	tmp := t.TempDir()
	program := filepath.Join(tmp, "any_type_annotations.qrk")
	source := strings.Join([]string{
		"fn id(x: any) any -> x",
		"println(id(42))",
		"println(id('ok'))",
		"v: any = 1",
		"v = 'changed'",
		"println(v)",
		"",
	}, "\n")
	if err := os.WriteFile(program, []byte(source), 0o644); err != nil {
		t.Fatalf("write %s: %v", program, err)
	}

	got := runQuark(t, "run", program)
	gotNorm := strings.TrimSpace(normalizeNewlines(got))
	expected := strings.Join([]string{"42", "ok", "changed"}, "\n")
	if gotNorm != expected {
		t.Fatalf("unexpected output\n--- got ---\n%s\n--- expected ---\n%s", gotNorm, expected)
	}
}

func TestExternTypedArg_RuntimeTypeGuard(t *testing.T) {
	tmp := t.TempDir()
	hpp := filepath.Join(tmp, "ext_guard.hpp")
	program := filepath.Join(tmp, "extern_type_guard.qrk")

	extSource := strings.Join([]string{
		"#ifndef EXT_GUARD_HPP",
		"#define EXT_GUARD_HPP",
		"#include <cstdint>",
		"inline int64_t qei_add1(int64_t x) { return x + 1; }",
		"#endif",
		"",
	}, "\n")
	if err := os.WriteFile(hpp, []byte(extSource), 0o644); err != nil {
		t.Fatalf("write %s: %v", hpp, err)
	}

	programSource := strings.Join([]string{
		"extern './ext_guard.hpp'",
		"extern fn add1(x: int) int as 'qei_add1'",
		"fn id(x) -> x",
		"v = id('oops')",
		"println(add1(v))",
		"",
	}, "\n")
	if err := os.WriteFile(program, []byte(programSource), 0o644); err != nil {
		t.Fatalf("write %s: %v", program, err)
	}

	cmd := exec.Command(quarkExePath, "run", program)
	var out bytes.Buffer
	var errBuf bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errBuf
	err := cmd.Run()
	if err == nil {
		t.Fatalf("expected runtime type error, got success\nstdout: %s\nstderr: %s", out.String(), errBuf.String())
	}
	combined := normalizeNewlines(out.String() + errBuf.String())
	if !strings.Contains(combined, "QK-RUNTIME-001") {
		t.Fatalf("expected runtime diagnostic code QK-RUNTIME-001, got:\n%s", combined)
	}
	if !strings.Contains(combined, "expected int, got str") {
		t.Fatalf("expected type mismatch detail in runtime error, got:\n%s", combined)
	}
}
