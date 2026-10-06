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
			name: "vector_layout",
			file: filepath.Join(testfilesDir, "smoke_vector_layout.qrk"),
			expected: join(
				"== smoke: vector layout ==",
				"10", "false", "true", "true", "10", "155",
				"2", "null", "null", "78", "null", "true",
				"2", "40",
				"0", "null", "39",
				"ab", "null", "cde", "0", "[vector len=4]", "true", "zz", "cde",
				"null", "3", "true", "true",
				"null", "5",
			),
		},
		{
			name: "semantics_safety",
			file: filepath.Join(testfilesDir, "smoke_semantics_safety.qrk"),
			expected: join(
				"== smoke: semantics safety ==",
				// structural equality
				"true", "true", "false", "false", "true", "true", "false",
				"true", "false", "true", "false",
				// all / any
				"false", "true", "true", "false",
				// explicit result checks
				"true",
				// int boundaries
				"9223372036854775807", "4611686018427387904", "4052555153018976267", "-1",
				// when as an expression
				"three", "typed", "ok 5", "err boom",
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
		{
			name: "structs",
			file: filepath.Join(testfilesDir, "smoke_structs.qrk"),
			expected: join(
				"10",
				"20",
				"1",
				"Alice",
				"0",
				"NYC",
				"30",
				"Charlie",
				"SF",
				"Point",
				"Customer",
				"Boston",
				"1",
				"Bob",
				"3",
				"3600",
				"3",
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
		{
			name:      "struct_unknown_field",
			file:      filepath.Join(testfilesDir, "smoke_structs_err_unknown_field.qrk"),
			errSubstr: "unknown field 'z' in Foo literal",
		},
		{
			name:      "struct_missing_field",
			file:      filepath.Join(testfilesDir, "smoke_structs_err_missing_field.qrk"),
			errSubstr: "missing required field 'y' in Foo literal",
		},
		{
			name:      "struct_field_assignment",
			file:      filepath.Join(testfilesDir, "smoke_structs_err_assign.qrk"),
			errSubstr: "cannot assign to field 'x' of immutable struct Foo",
		},
		{
			name:      "struct_equality",
			file:      filepath.Join(testfilesDir, "smoke_structs_err_equality.qrk"),
			errSubstr: "operator '==' is not defined for struct values",
		},
		{
			name:      "struct_type_mismatch",
			file:      filepath.Join(testfilesDir, "smoke_structs_err_type_mismatch.qrk"),
			errSubstr: "cannot assign value of type 'str' to field 'x' of type 'int'",
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

// TestSemanticsSafety_Rejected covers programs that must fail, either at
// compile time (statically known types) or at runtime (dynamic values), with
// a clear message instead of silently producing a wrong result.
func TestSemanticsSafety_Rejected(t *testing.T) {
	cases := []struct {
		name      string
		source    string
		errSubstr string
	}{
		// Results and vectors have no truthiness.
		{"result_if", "r = err 'x'\nif r:\n    println(1)\n", "result cannot be used as a condition"},
		{"result_while", "r = ok 1\nwhile r:\n    println(1)\n", "result cannot be used as a condition"},
		{"result_not", "r = ok 1\nprintln(!r)\n", "result cannot be used as a condition"},
		{"result_and", "r = ok 1\nprintln(r and true)\n", "result cannot be used as a condition"},
		{"vector_if", "v = vector [1, 2]\nif v == v:\n    println(1)\n", "vector cannot be used as a condition"},
		{"vector_ternary", "v = vector [1, 2]\nprintln(1 if v else 2)\n", "vector cannot be used as a condition"},
		{"result_if_dynamic", "fn f(x) ->\n    if x:\n        1\n    else:\n        2\nprintln(f(ok 1))\n", "result used as a condition"},
		{"vector_if_dynamic", "fn f(x) ->\n    if x:\n        1\n    else:\n        2\nprintln(f(vector [1]))\n", "vector used as a condition"},
		// Integer overflow is fatal on every path.
		{"overflow_add_scalar", "x = 9223372036854775807\nprintln(x + 1)\n", "integer overflow in '+'"},
		{"overflow_sub_scalar", "x = -9223372036854775807\nprintln(x - 2)\n", "integer overflow in '-'"},
		{"overflow_neg_scalar", "x = -9223372036854775807 - 1\nprintln(-x)\n", "integer overflow in '-'"},
		{"overflow_mul_boxed", "fn f(a, b) -> a * b\nprintln(f(4611686018427387904, 2))\n", "integer overflow in '*'"},
		{"overflow_pow", "println(2 ** 63)\n", "integer overflow in '**'"},
		{"overflow_vector", "v = vector [9223372036854775807]\nprintln(sum(v + 1))\n", "integer overflow in '+'"},
		// Scalar-lowered division and modulo match the boxed operators.
		{"mod_zero_scalar", "x = 7\ny = 0\nprintln(x % y)\n", "modulo by zero"},
		{"fdiv_zero_scalar", "x = 1.0\ny = 0.0\nprintln(x / y)\n", "division by zero"},
	}

	tmp := t.TempDir()
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			program := filepath.Join(tmp, tc.name+".qrk")
			if err := os.WriteFile(program, []byte(tc.source), 0o644); err != nil {
				t.Fatalf("write %s: %v", program, err)
			}
			cmd := exec.Command(quarkExePath, "run", program)
			var out, errBuf bytes.Buffer
			cmd.Stdout = &out
			cmd.Stderr = &errBuf
			if err := cmd.Run(); err == nil {
				t.Fatalf("expected failure, but it succeeded with output:\n%s", out.String())
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

	/* --- vectors: object + Arrow-layout buffers (64-byte aligned) --- */
    #define CHECK_ALIGNED(label, ptr) do {                                \
        if ((reinterpret_cast<uintptr_t>(ptr) % 64) == 0) { pass++; }   \
        else { fail++; std::fprintf(stderr, "FAIL: %s not 64-byte aligned\n", label); } \
    } while(0)

	QValue vf = qv_vector(4);
	CHECK("vector_f64_object", vf.data.vector_val);
	vf = q_vec_push(vf, qv_float(1.25));
	CHECK("vector_f64_values", vf.data.vector_val->values);
	CHECK_ALIGNED("vector_f64_values", vf.data.vector_val->values);

	QValue vi = qv_vector_i64(4);
	CHECK("vector_i64_object", vi.data.vector_val);
	vi = q_vec_push_i64(vi, qv_int(7));
	CHECK("vector_i64_values", vi.data.vector_val->values);

	QValue vb = qv_vector_bool(4);
	CHECK("vector_bool_object", vb.data.vector_val);
	vb = q_vec_push_bool(vb, qv_bool(true));
	CHECK("vector_bool_values", vb.data.vector_val->values);

	QValue vs = q_to_vector(qv_list_from(2, qv_string("hello"), qv_null()));
	CHECK("vector_str_object", vs.data.vector_val);
	CHECK("vector_str_offsets", vs.data.vector_val->offsets);
	CHECK("vector_str_values", vs.data.vector_val->values);
	CHECK("vector_str_validity", vs.data.vector_val->validity);
	CHECK_ALIGNED("vector_str_offsets", vs.data.vector_val->offsets);
	CHECK_ALIGNED("vector_str_validity", vs.data.vector_val->validity);

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
	got := compileAndRunRuntimeCheck(t, cppFile, source)
	if !strings.Contains(got, "0 failed") {
		t.Fatalf("gc_selfcheck reported failures:\n%s", got)
	}
	t.Logf("gc_selfcheck: %s", got)
}

// compileAndRunRuntimeCheck compiles a standalone C++ program against the
// runtime headers and the static GC, runs it, and returns its stdout.
func compileAndRunRuntimeCheck(t *testing.T, cppFile, source string) string {
	t.Helper()
	compiler := "clang++"
	if _, err := exec.LookPath(compiler); err != nil {
		t.Skip("skipping: clang++ not found in PATH")
	}
	gcInclude, gcLib, err := ensureGC()
	if err != nil {
		t.Fatalf("ensureGC: %v", err)
	}
	if err := os.WriteFile(cppFile, []byte(source), 0o644); err != nil {
		t.Fatalf("write %s: %v", cppFile, err)
	}
	outBin := strings.TrimSuffix(cppFile, ".cpp")
	if runtime.GOOS == "windows" {
		outBin += ".exe"
	}
	args := []string{
		"-std=c++17", "-O0", "-Wall", "-Wextra",
		"-DQUARK_USE_GC",
		"-I" + getRuntimeIncludePath(),
		"-I" + gcInclude,
		"-o", outBin,
		cppFile,
	}
	args = append(args, gcLinkArgs(gcLib)...)
	if runtime.GOOS != "windows" {
		args = append(args, "-lm")
	}
	compileCmd := exec.Command(compiler, args...)
	var compileBuf bytes.Buffer
	compileCmd.Stdout = &compileBuf
	compileCmd.Stderr = &compileBuf
	if err := compileCmd.Run(); err != nil {
		t.Fatalf("compile %s failed:\n%s\n%v", filepath.Base(cppFile), compileBuf.String(), err)
	}
	runCmd := exec.Command(outBin)
	var runOut, runErr bytes.Buffer
	runCmd.Stdout = &runOut
	runCmd.Stderr = &runErr
	if err := runCmd.Run(); err != nil {
		t.Fatalf("%s failed:\nstdout: %s\nstderr: %s\n%v", filepath.Base(cppFile), runOut.String(), runErr.String(), err)
	}
	return strings.TrimSpace(runOut.String() + runErr.String())
}

// TestVectorArrowLayout checks that vector buffers match the Apache Arrow
// columnar format bit for bit: bit-packed bools, LSB-first validity bitmaps,
// int32 string offsets, null counts, and that operations never mutate their
// inputs.
func TestVectorArrowLayout(t *testing.T) {
	cppFile := filepath.Join(t.TempDir(), "vector_layout.cpp")
	source := `
#include "quark/quark.hpp"
#include <cstdio>

static int pass = 0, fail = 0;
#define EXPECT(label, cond) do { if (cond) { pass++; } else { fail++; std::fprintf(stderr, "FAIL: %s\n", label); } } while (0)

int main() {
    q_gc_init();

    // Bools are bit-packed, LSB first: [T,F,T,T,F,F,F,F,T] -> 0x0D, 0x01.
    QValue b = q_to_vector(qv_list_from(9, qv_bool(true), qv_bool(false), qv_bool(true), qv_bool(true),
        qv_bool(false), qv_bool(false), qv_bool(false), qv_bool(false), qv_bool(true)));
    const QVector& bv = *b.data.vector_val;
    EXPECT("bool byte 0", bv.values[0] == 0x0D);
    EXPECT("bool byte 1", (bv.values[1] & 0x01) == 0x01);
    EXPECT("bool no validity", bv.validity == nullptr && bv.null_count == 0);

    // Validity bitmap: [1, null, 3] -> bits 1,0,1 -> 0x05, null_count 1.
    QValue i = q_to_vector(qv_list_from(3, qv_int(1), qv_null(), qv_int(3)));
    const QVector& iv = *i.data.vector_val;
    EXPECT("i64 validity byte", iv.validity && (iv.validity[0] & 0x07) == 0x05);
    EXPECT("i64 null_count", iv.null_count == 1);
    EXPECT("i64 values", q_vec_i64_data(iv)[0] == 1 && q_vec_i64_data(iv)[2] == 3);

    // Strings: ['ab', null, 'cde', ''] -> offsets 0,2,2,5,5 and bytes "abcde".
    QValue s = q_to_vector(qv_list_from(4, qv_string("ab"), qv_null(), qv_string("cde"), qv_string("")));
    const QVector& sv = *s.data.vector_val;
    EXPECT("str offsets", sv.offsets[0] == 0 && sv.offsets[1] == 2 && sv.offsets[2] == 2 &&
                          sv.offsets[3] == 5 && sv.offsets[4] == 5);
    EXPECT("str bytes", std::memcmp(sv.values, "abcde", 5) == 0);
    EXPECT("str null_count", sv.null_count == 1 && q_vec_is_null_at(sv, 1));

    // Builders grow across byte and capacity boundaries.
    QValue grown = qv_vector_bool(0);
    for (int k = 0; k < 100; k++) grown = q_vec_push_bool(grown, qv_bool(k % 3 == 0));
    size_t trues = 0;
    for (size_t k = 0; k < 100; k++) trues += q_vec_bool_at(*grown.data.vector_val, k) ? 1 : 0;
    EXPECT("bool builder count", grown.data.vector_val->count == 100 && trues == 34);

    // Operations return new vectors and leave inputs untouched.
    QValue filled = q_fillna(i, qv_int(0));
    EXPECT("fillna new vector", filled.data.vector_val != i.data.vector_val);
    EXPECT("fillna input unchanged", iv.null_count == 1 && q_vec_is_null_at(iv, 1));
    EXPECT("fillna output dense", filled.data.vector_val->null_count == 0 &&
                                  filled.data.vector_val->validity == nullptr);
    QValue sum = q_add(i, qv_int(1));
    EXPECT("add propagates nulls", sum.data.vector_val->null_count == 1 && q_vec_is_null_at(*sum.data.vector_val, 1));
    EXPECT("add input unchanged", q_vec_i64_data(iv)[0] == 1);

    // Comparison results are bit-packed with AND-ed validity.
    QValue cmp = q_vec_gt(i, qv_int(1));
    const QVector& cv = *cmp.data.vector_val;
    EXPECT("cmp type", cv.type == QVector::Type::BOOL);
    EXPECT("cmp bits", !q_vec_bool_at(cv, 0) && q_vec_bool_at(cv, 2));
    EXPECT("cmp null", q_vec_is_null_at(cv, 1) && cv.null_count == 1);

    std::printf("vector_layout: %d passed, %d failed\n", pass, fail);
    return fail > 0 ? 1 : 0;
}
`
	got := compileAndRunRuntimeCheck(t, cppFile, source)
	if !strings.Contains(got, " 0 failed") {
		t.Fatalf("vector layout check reported failures:\n%s", got)
	}
	t.Logf("%s", got)
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
