package main

import (
	"bytes"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"quark/toolchain"
)

var quarkExePath string

func TestMain(m *testing.M) {
	// Build quark once for all integration tests.
	// IMPORTANT: build into src/core/quark (same as normal usage) so the
	// toolchain package can resolve runtime/ and deps/ next to the binary.
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

	buildCmd := exec.Command("go", "build", "-o", quarkExePath, ".")
	buildCmd.Stdout = os.Stdout
	buildCmd.Stderr = os.Stderr
	buildCmd.Dir = pkgDir
	if err := buildCmd.Run(); err != nil {
		os.Exit(1)
	}

	code := m.Run()
	_ = os.Remove(quarkExePath)
	os.Exit(code)
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
	return runQuarkIn(t, "", args...)
}

// runQuarkIn runs quark with the working directory set to dir ("" for the
// current one), so programs that write relative paths don't litter the repo.
func runQuarkIn(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command(quarkExePath, args...)
	cmd.Dir = dir
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
				"5.0",
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
				"1",
				"2",
				"2",
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
				"[1, 2, 3, 4]",
				"4",
				"[2, 3, 4, 5]",
				"[2, 6, 12, 20]",
				"10",
				"1",
				"4",
				"[1, 2, 3, 4]",
				"10",
				"1",
				"4",
				"[3, 4, 5, 6]",
				"[3, 8, 15, 24]",
				"[1, 2, 3, 4]",
				"vec[i64]",
				"vec[i64]",
				"[10, 20, 30]",
				"60",
				"[10, 20, 30]",
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
				"[55, 99]",
				"[30, 99, 55]",
				"[30, 99, 55, 1, 2]",
				"[0, 1, 2, 3, 4]",
				"[2, 3, 4, 5, 6]",
				"[10, 7, 4, 1]",
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
			name: "fn_declarations",
			file: filepath.Join(testfilesDir, "smoke_fn_declarations.qrk"),
			expected: join(
				"== smoke: fn declarations ==",
				"true", "true", "11", "4", "211", "15", "4",
				"11", "3", "42", "8", "4", "40", "0", "27",
				"7", "8",
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
				"5.0",
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
				"vec[i64]",
				"vec[str]",
				"vec[str]",
				"[false, false, true, true, true]",
				"3",
				"2",
				"3",
				"3",
				"1",
				"4",
				"1",
				"[30, 40, 50]",
				"3",
				"10",
				"50",
				"50",
				"120",
				"0",
				"[4, 5]",
				"1",
				"4",
			),
		},
		{
			name: "vector_soundness",
			file: filepath.Join(testfilesDir, "smoke_vector_soundness.qrk"),
			expected: join(
				"== smoke: vec soundness ==",
				"vec[f64]",
				"13.5",
				"vec[f64]",
				"10.5",
				"vec[f64]",
				"vec[f64]",
				"12.0",
				"vec[bool]",
				"2",
				"vec[bool]",
				"2",
				"4",
				"1",
				"3",
				"15",
				"[1, 3]",
				"2",
				"null",
				"4",
				"1",
				"10",
				"3",
				"[1, 0, 3]",
				"3",
				"9007199254740994",
				"pear",
				"2.0",
				"0.5",
			),
		},
		{
			name: "modules_error_graph",
			file: filepath.Join(testfilesDir, "smoke_modules_error_graph.qrk"),
			expected: join(
				"1",
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
				"[vec[i64]: 3 elements]",
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
				"4.0",
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
				"true",
			),
		},
		{
			name: "vector_layout",
			file: filepath.Join(testfilesDir, "smoke_vector_layout.qrk"),
			expected: join(
				"== smoke: vec layout ==",
				"10", "false", "true", "true", "10", "155",
				"2", "null", "null", "78", "null", "true",
				"2", "40",
				"0", "null", "39",
				"ab", "null", "cde", "0", "[false, null, true, false]", "true", "zz", "cde",
				"null", "3.0", "true", "true",
				"0", "6",
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
				"6.0",
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
				"2",
				"321",
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
				"3.5",
				"8.0",
				"8",
				"2",
				"20",
				"2",
				"8",
			),
		},
		{
			name: "struct_fields",
			file: filepath.Join(testfilesDir, "smoke_struct_fields.qrk"),
			expected: join(
				"3.0",
				"3.0",
				"Row {id: 1, score: 2.0, weight: 2.0}",
				"1.5",
				"8",
				"4",
				"2",
				"10",
				"empty",
				"[]",
				"Ada",
				"null",
				"London",
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
		{
			name: "storage",
			file: filepath.Join(testfilesDir, "smoke_storage.qrk"),
			expected: join(
				"== smoke: variable storage ==",
				"3.0", "2.5", "float",
				"3.0", "1.0", "1.0", "1.0", "2.0",
				"42", "2.0",
				"outer", "inner", "1",
				"6",
				"0", "10", "0", "1",
				"6.0",
			),
		},
		{
			name: "element_types",
			file: filepath.Join(testfilesDir, "smoke_element_types.qrk"),
			expected: join(
				"3.0", "4.0", "5.0",
				"5.0",
				"30",
				"[[1, 2], [7]]",
				"[1, 2, 'three', 4.5]",
				"{a: 1, b: 'two'}",
				"2", "3.5", "4",
				"[0, 1]",
				"[0.5, 1.0, 1.25]",
				"['a', 'b', 'c']",
				"[0, 1, 2, 3, 4]",
				"[1, 2]",
				"[1, 'x']",
				"back to str",
				"3.0", "4.0", "quark",
			),
		},
		{
			name: "shadowing",
			file: filepath.Join(testfilesDir, "smoke_shadowing.qrk"),
			expected: join(
				"== smoke: shadowing builtins and native functions ==",
				"6", "42", "7", "11",
				"4", "5", "5", "8",
				"1000", "20",
				"42",
				"20", "2",
				"3", "2",
			),
		},
		{
			name: "function_values",
			file: filepath.Join(testfilesDir, "smoke_function_values.qrk"),
			expected: join(
				"== smoke: function values ==",
				"2", "5", "7", "100", "false", "10", "fn",
				"2", "2",
				"20", "3", "4", "9", "5",
				"2", "6",
				"3", "7",
				"1.0",
				"11", "3", "[1]", "[2]",
			),
		},
		{
			name: "module_calls_in_fns",
			file: filepath.Join(testfilesDir, "smoke_module_calls_in_fns.qrk"),
			expected: join(
				"== smoke: module calls inside functions ==",
				"a,b",
				"1,2",
				"true",
				"false",
				"true",
				"true",
			),
		},
		{
			name: "csv",
			file: filepath.Join(testfilesDir, "smoke_csv.qrk"),
			expected: join(
				"== smoke: csv ==",
				"3",
				"name | note",
				"Smith, J | say \"hi\"",
				"multi",
				"line | x",
				"1+2",
				"true",
			),
		},
		{
			name: "time_random",
			file: filepath.Join(testfilesDir, "smoke_time_random.qrk"),
			expected: join(
				"== smoke: time and random ==",
				"true",
				"true",
				"true",
				"true",
				"1,2,3,4,5",
				"true",
			),
		},
		{
			name: "printing",
			file: filepath.Join(testfilesDir, "smoke_printing.qrk"),
			expected: join(
				"== smoke: printing ==",
				"[1, 'a', null, 2.5, true]",
				"[1.5, null, 2.0]",
				"['x', 'y']",
				"{age: 36, 'first name': 'A', name: 'Ada'}",
				"Point {x: 1, y: 2}",
				"[ok 5, err 'boom']",
				"[0, 1, 2, 3, 4, ..., 995, 996, 997, 998, 999] (1000 items)",
				"390",
				"4890",
				"[1, [...]]",
				"[4.0, 0.30000000000000004, 1e+16, 1.5e-05]",
				"it's: ['it\\'s', 'a\\nb']",
			),
		},
		{
			name: "sort",
			file: filepath.Join(testfilesDir, "smoke_sort.qrk"),
			expected: join(
				"== smoke: sort ==",
				"-4 1.5 2 3",
				"-4 1.5 2 3",
				"apple fig pear",
				"false true true",
				"3 2 1",
				"c b a",
				"a bb ccc",
				"true",
				"false",
				"true",
			),
		},
		{
			name: "dynamic_methods",
			file: filepath.Join(testfilesDir, "smoke_dynamic_methods.qrk"),
			expected: join(
				"== smoke: dynamic methods ==",
				"3",
				"a-b-c",
				"20",
				"x",
				"1.5",
				"el",
				"1,2,3",
				"PAD",
				"1",
				"short-circuit ok",
			),
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := runQuarkIn(t, t.TempDir(), "run", tc.file)
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
		{"result_if", "r = err 'x'\nif r:\n    print(1)\n", "result cannot be used as a condition"},
		{"result_while", "r = ok 1\nwhile r:\n    print(1)\n", "result cannot be used as a condition"},
		{"result_not", "r = ok 1\nprint(!r)\n", "result cannot be used as a condition"},
		{"result_and", "r = ok 1\nprint(r and true)\n", "result cannot be used as a condition"},
		{"vector_if", "v = vec [1, 2]\nif v == v:\n    print(1)\n", "vec cannot be used as a condition"},
		{"vector_ternary", "v = vec [1, 2]\nprint(1 if v else 2)\n", "vec cannot be used as a condition"},
		{"result_if_dynamic", "fn f(x) ->\n    if x:\n        1\n    else:\n        2\nprint(f(ok 1))\n", "result used as a condition"},
		{"vector_if_dynamic", "fn f(x) ->\n    if x:\n        1\n    else:\n        2\nprint(f(vec [1]))\n", "vec used as a condition"},
		// Integer overflow is fatal on every path.
		{"overflow_add_scalar", "x = 9223372036854775807\nprint(x + 1)\n", "integer overflow in '+'"},
		{"overflow_sub_scalar", "x = -9223372036854775807\nprint(x - 2)\n", "integer overflow in '-'"},
		{"overflow_neg_scalar", "x = -9223372036854775807 - 1\nprint(-x)\n", "integer overflow in '-'"},
		{"overflow_mul_boxed", "fn f(a, b) -> a * b\nprint(f(4611686018427387904, 2))\n", "integer overflow in '*'"},
		{"overflow_pow", "print(2 ** 63)\n", "integer overflow in '**'"},
		{"overflow_vector", "v = vec [9223372036854775807]\nprint(sum(v + 1))\n", "integer overflow in '+'"},
		// Scalar-lowered division and modulo match the boxed operators.
		{"mod_zero_scalar", "x = 7\ny = 0\nprint(x % y)\n", "modulo by zero"},
		{"fdiv_zero_scalar", "x = 1.0\ny = 0.0\nprint(x / y)\n", "division by zero"},
		// Methods on values of unknown type: checked at compile time when no type
		// has the method or the arity is wrong, at runtime when the value's type lacks it.
		{"dynamic_method_wrong_type", "fn id(x) -> x\nprint(id(5).upper())\n", "type 'int' has no method 'upper'"},
		{"dynamic_method_unknown", "fn id(x) -> x\nid(5).frobnicate()\n", "no type has a method 'frobnicate'"},
		{"dynamic_method_arity", "fn id(x) -> x\nid('a').split()\n", "method 'split' expects 1 argument but got 0"},
		{"dynamic_method_arg_type", "fn id(x) -> x\nid('a').split(5)\n", "argument 1 of 'split' expects str, got int"},
		// A typed declaration holds a value of its type: a value of unknown
		// type is checked when it is stored (an int widens to float).
		{"decl_int_from_any", "fn id(v) -> v\nx: int = id('hello')\ny = x\nprint(y + 1)\n", "variable 'x' is declared int but got str"},
		{"decl_float_from_any", "fn id(v) -> v\nx: float = id('s')\n", "variable 'x' is declared float but got str"},
		{"decl_bool_from_any", "fn id(v) -> v\nb: bool = id(1)\n", "variable 'b' is declared bool but got int"},
		{"decl_str_from_any", "fn id(v) -> v\nx: str = id(5)\nprint(x)\n", "variable 'x' is declared str but got int"},
		{"decl_str_from_null", "fn id(v) -> v\ns: str = id(null)\n", "variable 's' is declared str but got null"},
		{"decl_list_from_any", "fn id(v) -> v\nl: list = id('s')\n", "variable 'l' is declared list but got str"},
		{"decl_result_from_any", "fn id(v) -> v\nr: result = id(5)\n", "variable 'r' is declared result but got int"},
		{"decl_struct_other", "struct P:\n    n: int\nstruct Q:\n    n: int\nfn id(v) -> v\np: P = id(Q { n: 1 })\n", "variable 'p' is declared P but got Q"},
		{"decl_captured_from_any", "fn id(v) -> v\nx: int = id('s')\nf = fn() -> x\nprint(f())\n", "variable 'x' is declared int but got str"},
		{"decl_in_fn_from_any", "fn id(v) -> v\nfn f(a) ->\n    n: int = a\n    n + 1\nprint(f(id('s')))\n", "variable 'n' is declared int but got str"},
		// Element types are fixed when a list or dict is created (rules B, C,
		// E, architecture.md §12.9): checked at compile time when the value's
		// type is known, by the runtime otherwise.
		{"elem_push_alias", "a = list [1, 2]\nb = a\nb.push('s')\n", "cannot add str to list[int]"},
		{"elem_index_assign", "l = list [1]\nl[0] = 'str'\n", "cannot add str to list[int]"},
		{"elem_insert", "l = list [1]\nl.insert(0, true)\n", "cannot add bool to list[int]"},
		{"elem_concat", "l = list [1]\nprint(l.concat(list ['a']))\n", "cannot add str to list[int]"},
		{"elem_concat_not_list", "fn id(v) -> v\nprint(list [1].concat(id(5)))\n", "concat() expects two lists, got list and int"},
		{"elem_concat_any_list", "fn id(v) -> v\nprint(list [1].concat(id(list ['a'])))\n", "concat() can't add a str to a list of int"},
		{"elem_push_untyped_fn", "fn add(xs) -> xs.push('x')\nl = list [1, 2]\nadd(l)\n", "push() can't add a str to a list of int"},
		{"elem_set_untyped_fn", "fn s(xs) -> xs.set(0, 'a')\nl = list [1]\ns(l)\n", "set() can't add a str to a list of int"},
		{"elem_insert_untyped_fn", "fn s(xs) -> xs.insert(0, 1.5)\nl = list [1]\ns(l)\n", "insert() can't add a float to a list of int"},
		{"elem_push_any_value", "fn id(v) -> v\nzs = list [1, 2]\nzs.push(id('hello'))\n", "push() can't add a str to a list of int"},
		{"elem_push_null", "fn id(v) -> v\nzs = list [1, 2]\nzs.push(id(null))\n", "push() can't add a null to a list of int"},
		{"elem_float_list_from_untyped", "fn add(xs) -> xs.push('a')\nl = list [1.5]\nadd(l)\n", "push() can't add a str to a list of float"},
		{"elem_nested_untyped", "fn add_row(grid, row) -> grid.push(row)\nrows = list [list [1, 2]]\nadd_row(rows, list ['a'])\n", "push() can't add a list[str] to a list of list[int]"},
		{"elem_nested_int_into_float", "a = list [1, 2]\nb = list [list [1.5]]\nb.push(a)\n", "cannot add list[int] to list[list[float]]"},
		{"elem_nested_any_into_int", "fn f(xs: list) -> xs\ng = list [list [1]]\ng.push(f(list []))\n", "cannot add list[any] to list[list[int]]"},
		{"elem_nested_vec_dtype", "fn put(xs) -> xs.push(vec [1.5])\nvs = list [vec [1, 2]]\nput(vs)\n", "push() can't add a vec to a list of vec[int]"},
		{"elem_nested_result", "fn put(xs) -> xs.push(ok 'a')\nrs = list [ok 1]\nput(rs)\n", "push() can't add a result to a list of result[int, any]"},
		{"elem_nested_struct", "struct P:\n    n: int\nstruct Q:\n    n: int\nfn put(xs) -> xs.push(Q { n: 1 })\nps = list [P { n: 1 }]\nput(ps)\n", "push() can't add a Q to a list of P"},
		{"elem_first_write", "out = list []\nfor i in range(3):\n    out.push(i)\nout.push('x')\n", "set by its first write, on line 3"},
		{"elem_first_write_runtime", "fn add(xs) -> xs.push('s')\nout = list []\nout.push(1)\nadd(out)\n", "push() can't add a str to a list of int"},
		{"elem_empty_into_typed_var", "xs = list [1]\nxs = list []\nxs.push('a')\n", "cannot add str to list[int]"},
		{"elem_list_var_from_any_list", "fn f(xs: list) -> xs\nys = list [1]\nys = f(ys)\n", "cannot assign value of type 'list[any]' to 'list[int]'"},
		{"elem_dict_key", "d = dict { a: 1 }\nd.b = 'hello'\n", "cannot store str under key 'b' of dict[str, int]"},
		{"elem_dict_set", "d = dict { a: 1 }\nd.set('b', 'x')\n", "cannot store str under key 'b' of dict[str, int]"},
		{"elem_dict_untyped_fn", "fn setk(d) ->\n    d.k = 's'\nm = dict { a: 1 }\nsetk(m)\n", "can't store a str under key 'k' of a dict of int"},
		{"elem_dict_set_untyped_fn", "fn setk(d) -> d.set('k', list [1])\nm = dict { a: 1 }\nsetk(m)\n", "can't store a list[int] under key 'k' of a dict of int"},
		// A function stored in a list doesn't keep its return type: untyped code
		// can replace it with one returning something else.
		{"elem_fn_return_not_trusted", "fs = list [fn() -> 1]\nfn put(xs) -> xs.push(fn() -> 'a')\nput(fs)\ng = fs[1]\nz = g()\nprint(z + 1)\n", "got str and int"},
		// Variables don't narrow (rule A).
		{"var_no_narrow_branch", "fn id(v) -> v\nx = id('hello')\nif false:\n    x = 5\ny = x\nprint(y + 1)\n", "got str and int"},
		{"var_no_narrow_loop", "fn id(v) -> v\nx = id('s')\nfor i in range(0):\n    x = 5\nz = x\nprint(z + 1)\n", "got str and int"},
		{"var_no_narrow_field", "struct P:\n    n: int\nfn id(v) -> v\nx = id(2.5)\nif false:\n    x = 5\nprint(P { n: x })\n", "field 'n' of P expects int, got float"},
		{"fn_var_signature", "g = fn() -> 5\nrun = fn() -> g()\ng = fn() -> 'abc'\n", "cannot assign fn() -> str to 'g', which holds fn() -> int"},
		{"fn_var_signature_in_loop", "g = fn() -> 5\nfor i in range(2):\n    g = fn() -> 'abc'\n", "cannot assign fn() -> str to 'g'"},
		{"fn_var_from_fn_value", "g = fn() -> 5\nh: fn = fn() -> 'a'\ng = h\n", "cannot assign fn to 'g'"},
		{"union_arith_not_int", "xs = list [1, 2.5]\nfor v in xs:\n    z = v + 1\n    n: int = z\n", "cannot assign value of type 'union[float | int]' to 'int'"},
		// Return annotations are checked on every function (rule D).
		{"return_if_without_else", "fn f(c) int ->\n    if c:\n        1\nprint(f(false) + 10)\n", "can end without a value"},
		{"return_when_no_match", "fn f(c) str ->\n    when c:\n        1 -> 'one'\nprint(f(2))\n", "can end without a value"},
		{"return_unknown_checked", "fn f(k) int -> k()\nprint(f(fn() -> 'str') + 1)\n", "function 'f' declares return type int but returned str"},
		{"return_unknown_float", "fn id(v) -> v\nfn f(x) float -> id(x)\nprint(f('a'))\n", "function 'f' declares return type float but returned str"},
		{"return_list_null", "fn id(v) -> v\nfn f(x) list -> id(x)\nprint(f(null))\n", "function 'f' declares return type list but returned null"},
		{"return_list_null_native", "fn id(v) -> v\nfn f() list -> id(null)\nprint(f())\n", "expected list, got null"},
		{"return_struct_other", "struct P:\n    n: int\nstruct Q:\n    n: int\nfn id(v) -> v\nfn f() P -> id(Q { n: 1 })\nprint(f())\n", "function 'f' declares return type P but returned Q"},
		{"return_lambda_checked", "fn id(v) -> v\nh = fn(x) bool -> id(x)\nprint(h(1))\n", "function 'h' declares return type bool but returned int"},
		// `list`, `dict`, `vec` and `fn` annotations don't accept null (decided
		// 2026-11-13); struct and resource annotations do.
		{"null_list_param", "fn g(l: list) -> 1\ng(null)\n", "argument 1 of 'g' expects list[any], got null"},
		{"null_list_param_runtime", "fn id(v) -> v\nfn g(l: list) -> 1\ng(id(null))\n", "argument 'l' of 'g' expects list, got null"},
		{"null_dict_param_runtime", "fn id(v) -> v\nfn g(d: dict) -> 1\ng(id(null))\n", "argument 'd' of 'g' expects dict, got null"},
		{"null_vec_param_runtime", "fn id(v) -> v\nfn g(v: vec) -> 1\ng(id(null))\n", "argument 'v' of 'g' expects vec, got null"},
		{"null_fn_param", "fn g(k: fn) -> 1\ng(null)\n", "argument 1 of 'g' expects fn, got null"},
		{"null_fn_param_runtime", "fn id(v) -> v\nfn g(k: fn) -> 1\ng(id(null))\n", "argument 'k' of 'g' expects fn, got null"},
		{"null_list_native_param", "fn id(v) -> v\nfn g(l: list) int -> 1\ng(id(null))\n", "expected list, got null"},
		{"null_list_decl", "l: list = null\n", "cannot assign value of type 'null' to 'list[any]'"},
		{"null_list_decl_runtime", "fn id(v) -> v\nl: list = id(null)\n", "variable 'l' is declared list but got null"},
		{"null_list_field", "struct S:\n    xs: list\nprint(S { xs: null })\n", "cannot assign value of type 'null' to field 'xs'"},
		{"null_list_field_runtime", "struct S:\n    xs: list\nfn id(v) -> v\nprint(S { xs: id(null) })\n", "field 'xs' of S expects list, got null"},
		{"null_list_default", "fn g(l: list = null) -> 1\nprint(g())\n", "default value type 'null' doesn't match parameter type"},
		{"null_list_reassign", "xs = list [1]\nxs = null\n", "cannot assign value of type 'null' to 'list[int]'"},
		// A parameter typed by a literal default is checked on entry like an
		// annotated one.
		{"default_typed_param_int", "fn foo(y = 0) ->\n    z = y\n    z + 1\nfn id(v) -> v\nprint(foo(id('abc')))\n", "argument 'y' of 'foo' expects int, got str"},
		{"default_typed_param_str", "fn id(v) -> v\nfn g(s = 'a') -> s.upper()\nprint(g(id(5)))\n", "argument 's' of 'g' expects str, got int"},
		{"default_typed_lambda_param", "fn id(v) -> v\nh = fn(n = 1) -> n * 2\nprint(h(id('x')))\n", "argument 'n' of 'h' expects int, got str"},
		// A pipe is the call with the piped input as its first argument, and is
		// checked like one.
		{"pipe_target_not_call", "x = 1\nprint(x | len)\n", "pipe target must be a function call"},
		{"pipe_builtin_arity", "print('a' | len(1))\n", "builtin 'len' expects 1 argument but got 2 (including piped input)"},
		{"pipe_fn_arity", "fn f(a) -> a\nprint(1 | f(2))\n", "function 'f' expects 1 argument but got 2 (including piped input)"},
		{"pipe_fn_arg_type", "fn f(a: str) -> a\nprint(1 | f())\n", "argument 1 of 'f' expects str, got int"},
		{"pipe_fn_null_arg", "fn f(a: str) -> a\nprint(null | f())\n", "argument 1 of 'f' expects str, got null"},
		{"pipe_fn_any_arg", "fn id(x) -> x\nfn f(a: str) -> a\nprint(id(5) | f())\n", "argument 'a' of 'f' expects str, got int"},
		{"pipe_method_arity", "print(1 | 'x'.replace())\n", "method 'replace' expects 2 arguments but got 1 (including piped input)"},
		{"pipe_dynamic_method_arity", "fn id(x) -> x\nprint('a' | id('b').split('c'))\n", "method 'split' expects 1 argument but got 2 (including piped input)"},
		{"pipe_module_extern_arg_type", "incl 'std/io' as io\nprint(1 | io::exists())\n", "argument 1 of 'io::exists' expects str, got int"},
		{"pipe_dict_member", "d = dict { f: fn(x) -> x }\nprint(3 | d.f())\n", "has no method 'f'"},
		{"pipe_callback_arity", "fn h(a, b) -> b\nfn apply(k) -> 1 | k()\nprint(apply(h))\n", "but 'h' takes 2 arguments"},
		// Ordering compares two numbers or two strings; mixes are runtime errors.
		{"order_str_int", "print('a' < 1)\n", "operator '<' expects two numbers or two strings"},
		{"csv_unterminated_quote", "incl 'std/csv' as csv\nprint(csv::parse('a,\"b\\n'))\n", "csv.parse: unterminated quoted field"},
		{"csv_bad_separator", "incl 'std/csv' as csv\nprint(csv::parse('a', ';;'))\n", "separator must be a single character"},
		// `fn` declarations are fixed (modules v0.2, rules 18-20).
		{"fn_declared_twice", "fn f(x) -> x\nfn f(x) -> x + 1\nprint(f(1))\n", "'f' is already defined on line 1"},
		{"fn_declared_twice_in_fn", "fn outer() ->\n    fn g() -> 1\n    fn g() -> 2\n    g()\nprint(outer())\n", "'g' is already defined on line 2"},
		{"fn_declared_twice_in_module", "module m:\n    fn g() -> 1\n    fn g() -> 2\nprint(m::g())\n", "'g' is already defined on line 2"},
		{"fn_reassigned", "fn f(x) -> x\nf = fn(x) -> 0\nprint(f(1))\n", "'f' is a function declaration (line 1) and can't be reassigned; use a variable: g = fn ..."},
		{"fn_reassigned_value", "fn f(x) -> x\nf = 3\nprint(f)\n", "'f' is a function declaration (line 1) and can't be reassigned"},
		{"fn_reassigned_native", "fn f(x: int) int -> x\nf = fn(x: int) int -> 0\nprint(f(1))\n", "'f' is a function declaration (line 1) and can't be reassigned"},
		{"fn_reassigned_in_fn", "fn f(x) -> x\nfn g() ->\n    f = 1\n    f\nprint(g())\n", "'f' is a function declaration (line 1) and can't be reassigned"},
		{"fn_reassigned_before_decl", "f = 1\nfn f() -> 2\nprint(f)\n", "'f' is a function declaration (line 2) and can't be reassigned"},
		{"fn_var_decl_same_name", "fn f() -> 1\nf: int = 2\n", "'f' is a function declaration (line 1) and can't be reassigned"},
		{"fn_shadows_outer_fn", "fn f(x) -> x\nfn g() ->\n    fn f() -> 2\n    f()\nprint(g())\n", "'f' is already a function in an enclosing scope (line 1); choose another name"},
		{"fn_shadows_param", "fn g(f) ->\n    fn f() -> 2\n    f()\nprint(g(1))\n", "'f' is already defined in an enclosing scope; choose another name for this function"},
		{"fn_shadows_outer_var", "x = 1\nfn g() ->\n    fn x() -> 2\n    x()\nprint(g())\n", "'x' is already defined in an enclosing scope"},
		{"fn_shadows_builtin_nested", "fn g() ->\n    fn len(x) -> 2\n    len(1)\nprint(g())\n", "'len' is a builtin function and can't be redefined or assigned"},
		// Hoisted functions can't run before a variable they use is assigned
		// (rule 23): rejected when the call order is visible, a runtime error
		// (never null) through a function value.
		{"hoisted_call_before_var", "report()\ntotal = 5\nfn report() -> print(total)\n", "'report' uses 'total', which is assigned on line 2, after this call"},
		{"hoisted_call_in_own_assignment", "total = report()\nfn report() -> total + 1\nprint(total)\n", "'report' uses 'total', which is assigned on line 1, after this call"},
		{"hoisted_call_chain", "a()\ntotal = 5\nfn report() -> print(total)\nfn a() -> report()\n", "'a' uses 'total' (through 'report'), which is assigned on line 2, after this call"},
		{"hoisted_call_pipe", "1 | show()\nmsg = 'm'\nfn show(x) -> print(msg)\n", "'show' uses 'msg', which is assigned on line 2, after this call"},
		{"hoisted_call_in_if", "if true:\n    report()\ntotal = 5\nfn report() -> total\n", "'report' uses 'total', which is assigned on line 3, after this call"},
		{"hoisted_call_in_fn_body", "fn outer() ->\n    show()\n    msg = 'hi'\n    fn show() -> print(msg)\nouter()\n", "'show' uses 'msg', which is assigned on line 3, after this call"},
		{"hoisted_value_call_unassigned", "g = report\ng()\ntotal = 5\nfn report() -> print(total)\n", "'total' is used before it is assigned"},
		{"hoisted_value_call_unassigned_in_fn", "fn outer() ->\n    g = show\n    g()\n    msg = 'hi'\n    fn show() -> print(msg)\nouter()\n", "'msg' is used before it is assigned"},
		{"hoisted_value_call_unassigned_nested_lambda", "g = report\ng()\ntotal = 5\nfn report() -> (fn() -> total)()\n", "'total' is used before it is assigned"},
		// Variables are not hoisted: a body only sees what is assigned above it.
		{"fn_uses_later_var", "fn report() -> print(total)\ntotal = 5\nreport()\n", "undefined identifier 'total'"},
		// Wrong uses of declared functions are still checked at the call.
		{"hoisted_forward_call_arity", "print(f(1, 2))\nfn f(x) -> x\n", "function 'f' expects 1 argument but got 2"},
		{"hoisted_forward_call_type", "print(f('s'))\nfn f(x: int) int -> x\n", "argument 1 of 'f' expects int, got str"},
		{"direct_fn_value_arity", "fn id(v) -> v\nfn h(a, b) -> b\nf = id(h)\nprint(f(1))\n", "function 'h' expects 2 arguments but got 1"},
		{"direct_fn_param_dynamic", "fn id(v) -> v\nfn f(x: int) -> x\nprint(f(id('s')))\n", "argument 'x' of 'f' expects int, got str"},
		{"direct_native_param_dynamic", "fn id(v) -> v\nfn f(x: int) int -> x\nprint(f(id('s')))\n", "expected int, got str"},
		{"chained_assignment", "a = 0\nb = 0\na = b = 1\n", "chained assignment is not supported"},
		// sort keys must be mutually comparable.
		{"sort_mixed_types", "print(list [1, 'a'].sort())\n", "sort() can't compare str and int"},
		{"sort_null", "print(list [1, null].sort())\n", "sort() can't compare null and int"},
		{"sort_by_not_fn", "print(list [1].sort_by(5))\n", "sort_by() expects a function"},
		// when patterns are literals, '_' or ok/err bindings.
		{"when_call_pattern", "fn f(x) -> x\nn = 3\nwhen n:\n    f(3) -> print(1)\n    _ -> print(2)\n", "when patterns must be literal values"},
		{"when_identifier_pattern", "y = 3\nn = 3\nwhen n:\n    y -> print(1)\n    _ -> print(2)\n", "when patterns must be literal values"},
		{"when_result_or", "r = ok 1\nwhen r:\n    ok x or err x -> print(1)\n", "ok patterns cannot be combined with other patterns using 'or'"},
		// The lowered for-range loop checks its arguments like range().
		{"range_loop_step_zero", "for i in range(0, 5, 0):\n    print(i)\n", "range() step must not be zero"},
		{"range_loop_step_zero_dynamic", "fn id(x) -> x\nfor i in range(0, 5, id(0)):\n    print(i)\n", "range() step must not be zero"},
		{"range_loop_non_numeric", "fn id(x) -> x\nfor i in range(id('x')):\n    print(i)\n", "range(end) expects numeric end value"},
		// `fn` annotations: checked at compile time when the argument's type is
		// known, on entry otherwise; a native function's result is checked too.
		{"fn_param_static", "fn apply(f: fn) -> f(1)\napply(5)\n", "argument 1 of 'apply' expects fn, got int"},
		{"fn_param_dynamic", "fn id(v) -> v\nfn apply(f: fn) -> f(1)\napply(id(5))\n", "argument 'f' of 'apply' expects fn, got int"},
		{"fn_param_dynamic_native", "fn id(v) -> v\nfn nat(f: fn, y: int) int -> f(y)\nprint(nat(id(5), 1))\n", "expected fn, got int"},
		{"fn_var_annotation", "f: fn = 3\n", "cannot assign value of type 'int' to 'fn'"},
		{"fn_struct_field", "struct H:\n    run: fn\nh = H { run: 1 }\n", "to field 'run' of type 'fn'"},
		{"native_return_checked", "fn nat(f: fn) int -> f(1)\nprint(nat(fn(a) -> 'x'))\n", "expected int, got str"},
		{"module_member_value_unknown", "incl 'std/io' as io\nf = io::nope\n", "'io' (std/io) has no 'nope'"},
		// Calls through function parameters are checked at compile time against
		// the function passed: arity, argument types (including arguments the
		// body passes on from its own parameters), across pipes, modules and
		// functions that hand the parameter on.
		{"callback_arity", "fn h(a, b) -> b\nfn apply(k) -> k(1)\nprint(apply(h))\n", "argument 1 of 'apply' is called with 1 argument (line 2), but 'h' takes 2 arguments"},
		{"callback_arity_passed_on", "fn h(a, b) -> b\nfn apply(k: fn) -> k(1)\nfn wrap(k) -> apply(k)\nprint(wrap(h))\n", "argument 1 of 'wrap' is called with 1 argument (line 2), but 'h' takes 2 arguments"},
		{"callback_arity_pipe", "fn h(a, b) -> b\nfn apply(k) -> 1 | k()\nprint(apply(h))\n", "argument 1 of 'apply' is called with 1 argument (line 2), but 'h' takes 2 arguments"},
		{"callback_arity_lambda", "fn apply(k) -> k(1)\nprint(apply(fn(a, b) -> b))\n", "but the lambda passed takes 2 arguments"},
		{"callback_arity_module_member", "incl 'std/io' as io\nfn apply(k) -> k()\nprint(apply(io::exists))\n", "but 'io::exists' takes 1 argument"},
		{"callback_arity_in_closure", "fn h(a, b) -> b\nfn apply(k) ->\n    g = fn() -> k(1)\n    g()\nprint(apply(h))\n", "is called with 1 argument (line 3), but 'h' takes 2 arguments"},
		{"callback_arg_type", "fn inc(x: int) -> x + 1\nfn apply(k) -> k('s')\nprint(apply(inc))\n", "argument 1 of 'apply' is called with str as argument 1 (line 2), but 'inc' expects int"},
		{"callback_arg_type_from_param", "fn apply(k, v) -> k(v)\nprint(apply(fn(x: int) -> x, 'str'))\n", "is called with str as argument 1 (line 1), but the lambda passed expects int"},
		{"callback_arg_type_two_levels", "fn inc(x: int) -> x + 1\nfn apply(k, v) -> k(v)\nfn wrap(g, w) -> apply(g, w)\nprint(wrap(inc, 'oops'))\n", "argument 1 of 'wrap' is called with str as argument 1 (line 2), but 'inc' expects int"},
		{"callback_arg_type_default", "fn inc(x: int) -> x + 1\nfn apply(k, v = 'd') -> k(v)\nprint(apply(inc))\n", "is called with str as argument 1 (line 2), but 'inc' expects int"},
		{"callback_not_a_function", "fn apply(k) -> k(1)\nprint(apply(5))\n", "argument 1 of 'apply' is called as a function (line 1), but gets int"},
		{"callback_union_arity", "fn a1(x) -> x\nfn a2(x, y) -> y\nfs = list [a1, a2]\nprint(fs[0](1, 2, 3))\n", "none of the functions this value can be accepts 3 arguments"},
		// Function values the compiler can't see through check the argument
		// count when called.
		{"dynamic_arity_too_few", "fn id(v) -> v\nfn h(a, b) -> b\nf = id(h)\nprint(f(1))\n", "function 'h' expects 2 arguments but got 1"},
		{"dynamic_arity_too_many", "fn id(v) -> v\nf = id(fn(a) -> a)\nprint(f(1, 2))\n", "function expects 1 argument but got 2"},
		{"dynamic_arity_default_range", "fn id(v) -> v\nfn g(a, b = 1) -> a\nf = id(g)\nprint(f())\n", "function 'g' expects 1-2 arguments but got 0"},
		{"dynamic_arity_struct_field", "struct H:\n    run: fn\nfn h(a, b) -> b\nx = H { run: h }\nr = x.run\nprint(r(1))\n", "function 'h' expects 2 arguments but got 1"},
		{"dynamic_arity_module_member", "incl 'std/io' as io\nfn id(v) -> v\nf = id(io::exists)\nprint(f())\n", "function 'io::exists' expects 1 argument but got 0"},
		{"when_literal_or_result", "n = 1\nwhen n:\n    1 or ok x -> print(1)\n    _ -> print(2)\n", "ok patterns cannot be combined with other patterns using 'or'"},
		// Struct field values of unknown type are checked when the literal is
		// built, so a field read never reinterprets a value of the wrong type.
		{"struct_field_any_int", "struct P:\n    x: int\nfn mk(a) -> P { x: a }\np = mk('hello')\nprint(p.x + 1)\n", "field 'x' of P expects int, got str"},
		{"struct_field_any_float", "struct P:\n    x: float\nfn mk(a) -> P { x: a }\np = mk('hello')\nprint(p.x * 2.0)\n", "field 'x' of P expects float, got str"},
		{"struct_field_any_bool", "struct P:\n    flag: bool\nfn mk(a) -> P { flag: a }\np = mk(1)\nprint(p.flag)\n", "field 'flag' of P expects bool, got int"},
		{"struct_field_any_str", "struct P:\n    name: str\nfn mk(a) -> P { name: a }\nprint(mk(null).name)\n", "field 'name' of P expects str, got null"},
		{"struct_field_any_list", "struct P:\n    xs: list\nfn mk(a) -> P { xs: a }\nprint(mk(3).xs)\n", "field 'xs' of P expects list, got int"},
		{"struct_field_any_fn", "struct H:\n    run: fn\nfn mk(a) -> H { run: a }\nprint(mk(3).run)\n", "field 'run' of H expects fn, got int"},
		{"struct_field_wrong_struct", "struct A:\n    x: int\nstruct B:\n    a: A\nfn mk(v) -> B { a: v }\nq: any = B { a: A { x: 1 } }\nprint(mk(q))\n", "field 'a' of B expects A, got B"},
		{"struct_field_null_read", "struct O:\n    id: int\no: O = null\nprint(o.id + 1)\n", "cannot read a field of null"},
		{"struct_literal_unknown_name", "q = Pnt { x: 1 }\n", "unknown struct type 'Pnt'"},
		{"struct_return_type_unknown_name", "fn f() Pnt -> 1\nprint(f())\n", "unknown type 'Pnt'"},
		// d.key names a key in the source: a missing key is an error, not null.
		{"dict_dot_missing_key", "rows = list [dict {name: 'b', amount: 2}]\nfor r in rows:\n    print(r.amonut)\n", "dict has no key 'amonut'"},
		{"dict_dot_missing_key_untyped", "fn id(v) -> v\nd = id(dict {a: 1})\nprint(d.b)\n", "dict has no key 'b'"},
		// A definition named like a builtin or extern fn would be ignored by
		// every call; it is rejected (parameters and locals may shadow one).
		{"builtin_redefined_fn", "fn len(x) -> 42\nprint(len('ab'))\n", "'len' is a builtin function and can't be redefined or assigned"},
		{"builtin_redefined_lambda", "len = fn(x) -> 42\nprint(len('ab'))\n", "'len' is a builtin function and can't be redefined or assigned"},
		{"builtin_redefined_print", "fn print(x) -> 1\nprint(5)\n", "'print' is a builtin function and can't be redefined or assigned"},
		{"builtin_assigned_value", "sum = 0\nprint(sum)\n", "'sum' is a builtin function and can't be redefined or assigned"},
		{"builtin_assigned_in_fn", "fn f() ->\n    max = 1\n    max\nprint(f())\n", "'max' is a builtin function and can't be redefined or assigned"},
		{"extern_redefined_same_file", "extern fn myargs() list as 'q_sys_args'\nfn myargs() -> 1\nprint(1)\n", "'myargs' is already defined on line 1"},
		// input() returns ok(line) / err('end of input'), and a result is never
		// null, so the old `line == null` EOF check can't silently loop forever.
		{"input_used_as_str", "print(len(input()))\n", "argument 1 of 'len' expects str, list, dict, or vec, got result[str, str]"},
		{"input_str_param", "fn f(s: str) -> s\nprint(f(input()))\n", "argument 1 of 'f' expects str, got result[str, str]"},
		{"input_method", "x = input()\nprint(x.upper())\n", "type 'result[str, str]' has no method 'upper'"},
		{"input_eq_null", "while true:\n    line = input()\n    if line == null:\n        break\n    print(line)\n", "a result is never null, so this comparison is always false"},
		{"input_ne_null", "print(input() != null)\n", "a result is never null, so this comparison is always true"},
		{"input_unwrap_eof", "print(unwrap(input()))\n", "unwrap on err: end of input"},
		{"result_param_null", "fn f(r: result) -> is_ok(r)\nprint(f(null))\n", "argument 1 of 'f' expects result[any, any], got null"},
		{"result_param_null_untyped", "fn id(v) -> v\nfn f(r: result) -> is_ok(r)\nprint(f(id(null)))\n", "argument 'r' of 'f' expects result, got null"},
		{"result_var_null", "r: result = null\nprint(r)\n", "cannot assign value of type 'null' to 'result[any, any]'"},
		{"result_reassigned_null", "r = ok(1)\nr = null\nprint(r)\n", "cannot assign value of type 'null' to 'result[int, any]'"},
		{"result_field_null", "struct S:\n    r: result\nprint(S { r: null })\n", "cannot assign value of type 'null' to field 'r'"},
		{"result_field_null_untyped", "fn id(v) -> v\nstruct S:\n    r: result\nprint(S { r: id(null) })\n", "field 'r' of S expects result, got null"},
		{"result_return_null", "fn f() result -> null\nprint(f())\n", "declares return type 'result[any, any]' but body returns 'null'"},
		{"result_default_null", "fn f(r: result = null) -> r\nprint(f())\n", "default value type 'null' doesn't match parameter type 'result[any, any]'"},
		{"int_default_null", "fn f(x: int = null) -> x\nprint(f())\n", "default value type 'null' doesn't match parameter type 'int'"},
		// An assignment is a statement: inside a larger expression or as an
		// elseif condition it could run conditionally and leave the variable
		// unassigned (semantics.md §4.5).
		{"assign_in_and", "fn f() -> 3\na = true\nif a and (n = f()) > 0:\n    print(n)\n", "an assignment can't be part of an expression"},
		{"assign_in_or", "a = true\nb = a or (z = 3) > 1\nprint(z)\n", "an assignment can't be part of an expression"},
		{"assign_in_ternary", "c = true\nw = (q = 4) if c else 0\nprint(q)\n", "an assignment can't be part of an expression"},
		{"assign_in_arith", "x = 1\ny = x + (x = 5)\nprint(y)\n", "an assignment can't be part of an expression"},
		{"assign_as_elseif", "fn f() -> 3\nif false:\n    print(1)\nelseif x = f():\n    print(x)\n", "an assignment can't be an elseif condition"},
		// A missing vec entry taken out as one value is an error (tables spec D12).
		{"vec_missing_loop_int", "v = list [1, null, 3].to_vec()\nfor x in v:\n    print(x)\n", "vec element 1 is missing; use fillna or drop_na first"},
		{"vec_missing_loop_float", "v = list [1.5, null].to_vec()\ns = 0.0\nfor x in v:\n    s = s + x\n", "vec element 1 is missing"},
		{"vec_missing_loop_bool", "v = list [true, null].to_vec()\nfor x in v:\n    print(x)\n", "vec element 1 is missing"},
		{"vec_missing_loop_str", "v = list ['a', null].to_vec()\nfor s in v:\n    print(s.upper())\n", "vec element 1 is missing"},
		{"vec_missing_loop_captured", "v = list [1, null].to_vec()\nfs = list []\nfor x in v:\n    fs.push(fn() -> x)\n", "vec element 1 is missing"},
		{"vec_missing_loop_untyped", "fn id(a) -> a\nfor x in id(list [1, null].to_vec()):\n    print(x)\n", "vec element 1 is missing"},
		{"vec_missing_loop_param", "fn f(w: vec) ->\n    for x in w:\n        print(x)\nf(list [1, null].to_vec())\n", "vec element 1 is missing"},
		{"vec_missing_index_var", "v = list [1, null, 3].to_vec()\nn = v[1]\nprint(n * 2)\n", "vec element 1 is missing"},
		{"vec_missing_index_negative", "v = list [1, null].to_vec()\nprint(v[-1])\n", "vec element -1 is missing"},
		{"vec_missing_index_untyped", "fn id(a) -> a\nprint(id(list [1, null].to_vec())[1])\n", "vec element 1 is missing"},
		{"vec_missing_to_list", "v = list [1, null, null].to_vec()\nprint(v.to_list())\n", "to_list() of a vec with 2 missing entries; use fillna or drop_na first"},
		{"vec_missing_enumerate", "v = list [1, null].to_vec()\nprint(v.enumerate())\n", "vec element 1 is missing"},
		{"vec_get_null_typed", "v = list [1, null].to_vec()\nn: int = v.get(1)\n", "variable 'n' is declared int but got null"},
		{"vec_get_null_arith", "v = list [1, null].to_vec()\nx = v.get(1)\nprint(x + 1)\n", "expects numeric operands"},
		// Vec reductions follow the table aggregates (tables spec D13); a vec
		// with no present values has no min, max or mean.
		{"vec_min_no_values", "v = list [1, null].to_vec()[vec [false, true]]\nprint(min(v))\n", "min() of a vec with no present values"},
		{"vec_max_empty", "v = list [1].to_vec()[vec [false]]\nprint(max(v))\n", "max() of a vec with no present values"},
		{"vec_min_all_nan", "v = vec [to_float('nan')]\nprint(min(v))\n", "min() of a vec with no present values"},
		{"vec_max_untyped", "fn id(a) -> a\nprint(max(id(list [1, null].to_vec()[vec [false, true]])))\n", "max() of a vec with no present values"},
		{"vec_sum_overflow", "print(sum(vec [9223372036854775807, 1]))\n", "integer overflow in 'sum'"},
		{"vec_sum_str", "print(sum(vec ['a', 'b']))\n", "argument 1 of 'sum' expects numeric or bool vec, got vec[str]"},
		{"vec_min_typed_result", "m: str = min(vec [1, 2])\n", "cannot assign"},
		{"stats_mean_no_values", "incl 'std/stats'\nprint(stats::mean(list [1, null].to_vec()[vec [false, true]]))\n", "stats::mean() of a vec with no present values"},
		{"stats_mean_str", "incl 'std/stats'\nprint(stats::mean(vec ['a']))\n", "stats::mean() expects a numeric or bool vec, got vec[str]"},
		{"stats_mean_not_vec", "incl 'std/stats'\nprint(stats::mean(list [1, 2]))\n", "argument 1 of 'stats::mean' expects vec[any], got list[int]"},
		// A null pointer from one native function never reaches the next.
		{"native_null_str_chained", "extern 'nn.hpp'\nextern fn nullstr() str as 'nn_nullstr'\nextern fn slen(s: str) int as 'nn_len'\nprint(slen(nullstr()))\n", "a native function declared to return str returned a null pointer"},
		{"native_null_list_chained", "extern 'nn.hpp'\nextern fn nulllist() list as 'nn_nulllist'\nextern fn lcount(l: list) int as 'nn_count'\nprint(lcount(nulllist()))\n", "a native function declared to return list returned a null pointer"},
		{"native_null_via_native_fn", "extern 'nn.hpp'\nextern fn nullstr() str as 'nn_nullstr'\nextern fn slen(s: str) int as 'nn_len'\nfn g() str -> nullstr()\nprint(slen(g()))\n", "a native function declared to return str returned a null pointer"},
		// xs[i] and s[i] are typed reads: out of range is an error on every
		// path, never a null read as the element type.
		{"list_index_past_end", "w = list [1, 2][5]\nprint(w)\n", "list index 5 out of range (list length 2)"},
		{"list_index_negative", "xs = list [1, 2]\nprint(xs[-3])\n", "list index -3 out of range (list length 2)"},
		{"list_index_above_32_bits", "print(list [10, 20, 30][4294967296])\n", "list index 4294967296 out of range"},
		{"list_index_untyped", "fn id(v) -> v\nprint(id(list [1])[3])\n", "list index 3 out of range (list length 1)"},
		{"list_index_in_fn_param", "fn at(xs: list, i: int) -> xs[i]\nprint(at(list [1], 1))\n", "list index 1 out of range"},
		{"list_index_in_module", "module m:\n    fn first(xs) -> xs[0]\nprint(m::first(list []))\n", "list index 0 out of range (list length 0)"},
		{"str_index_past_end", "print('abc'[3])\n", "str index 3 out of range (str length 3)"},
		{"str_index_untyped_non_int", "fn id(v) -> v\nprint('abc'[id(1.5)])\n", "str index must be int, got float"},
		{"str_index_untyped_null", "fn id(v) -> v\nprint('abc'[id(null)])\n", "str index must be int, got null"},
		{"index_untyped_dict", "fn id(v) -> v\nprint(id(dict { a: 1 })[0])\n", "cannot index a dict"},
		// A for loop over an untyped value that isn't iterable fails before
		// the first iteration, also when it is empty.
		{"for_untyped_empty_dict", "fn id(v) -> v\nfor k in id(dict {}):\n    print(k)\n", "for-loop iterable must be list, string, or vec, got dict"},
		{"for_untyped_null", "fn id(v) -> v\nfor k in id(null):\n    print(k)\n", "for-loop iterable must be list, string, or vec, got null"},
		// .get returns null for a missing item, so its result is any.
		{"list_get_used_as_int", "xs = list [1, 2]\nt = 0\nt = t + xs.get(0)\n", "cannot assign value of unknown type to 'int'"},
		{"list_get_null_into_int", "xs = list [1, 2]\nn: int = xs.get(9)\n", "variable 'n' is declared int but got null"},
		{"dict_get_null_into_int", "d = dict { a: 1 }\nk: int = d.get('b')\n", "variable 'k' is declared int but got null"},
		{"dict_get_null_into_param", "fn twice(n: int) -> n * 2\nd = dict { a: 1 }\nprint(twice(d.get('b')))\n", "argument 'n' of 'twice' expects int, got null"},
		// when compares with ==, which is elementwise on a vec.
		{"when_vec_known", "v = vec [1, 1]\nwhen v:\n    1 -> print('one')\n    _ -> print('other')\n", "when can't match a vec"},
		{"when_vec_param", "fn g(a: vec) ->\n    when a:\n        1 -> 'one'\n        _ -> 'other'\nprint(g(vec [1]))\n", "when can't match a vec"},
		{"when_vec_untyped", "fn g(a) ->\n    when a:\n        1 -> 'one'\n        _ -> 'other'\nprint(g(vec [1, 1]))\n", "when can't match a vec"},
	}

	tmp := t.TempDir()
	nn := "#pragma once\n#include <cstdint>\n#include <cstring>\n" +
		"inline const char* nn_nullstr() { return nullptr; }\n" +
		"inline int64_t nn_len(const char* s) { return s ? (int64_t)std::strlen(s) : -1; }\n" +
		"inline QList* nn_nulllist() { return nullptr; }\n" +
		"inline int64_t nn_count(QList* l) { return l ? 1 : -1; }\n"
	if err := os.WriteFile(filepath.Join(tmp, "nn.hpp"), []byte(nn), 0o644); err != nil {
		t.Fatal(err)
	}
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
		"print(type(add))",
		"print(\"hello\\nworld\")",
		"r: result = ok 1",
		"print(type(r))",
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
		"print(alias(5))",
		"print(alias(5, 20))",
		"alias2 = alias",
		"print(alias2(7))",
		"5 | alias() | print()",
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

func TestStdIO_FileModes(t *testing.T) {
	tmp := t.TempDir()
	targetPath := filepath.Join(tmp, "io_v0.txt")
	targetForQuark := strings.ReplaceAll(targetPath, "\\", "/")

	program := filepath.Join(tmp, "std_io_modes.qrk")
	source := strings.Join([]string{
		"incl 'std/io' as io",
		"p = '" + targetForQuark + "'",
		"f = unwrap(io::open(p, 'w'))",
		"print(type(f))",
		"print(type(io::exists(p)))",
		"io::write(f, 'hello file') | unwrap() | print()",
		"io::close(f) | unwrap()",
		"r = unwrap(io::open(p, 'r'))",
		"print(unwrap(io::read(r, 64)))",
		"io::close(r) | unwrap()",
		"print(io::exists(p))",
		"a = unwrap(io::open(p, 'a'))",
		"io::write(a, '!') | unwrap()",
		"io::close(a) | unwrap()",
		"r2 = unwrap(io::open(p, 'r'))",
		"print(unwrap(io::read(r2, 64)))",
		"io::close(r2) | unwrap()",
		"",
	}, "\n")
	if err := os.WriteFile(program, []byte(source), 0o644); err != nil {
		t.Fatalf("write %s: %v", program, err)
	}

	got := runQuark(t, "run", program)
	gotNorm := strings.TrimSpace(normalizeNewlines(got))
	expected := strings.Join([]string{"resource", "bool", "10", "hello file", "true", "hello file!"}, "\n")
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

	/* --- dict: object, and key buffers beyond the small-string size --- */
    QValue d = qv_dict();
    CHECK("dict_object", d.data.dict_val);
    d = q_dict_set(d, qv_string("a key well beyond the small string buffer"), qv_int(1));
    CHECK("dict_long_key", d.data.dict_val->entries.begin()->first.data());

	/* --- resource registry: slot buffer must be scanned (payloads live only there) --- */
    QResourceHandle* rh = q_resource_create(QRES_KIND_NONE, q_malloc(16), 0);
    CHECK("resource_handle", rh);
    CHECK("resource_slots", q_resource_registry().slots.data());

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

	QValue vs = q_to_vec(qv_list_from(2, qv_string("hello"), qv_null()));
	CHECK("vector_str_object", vs.data.vector_val);
	CHECK("vector_str_offsets", vs.data.vector_val->offsets);
	CHECK("vector_str_values", vs.data.vector_val->values);
	CHECK("vector_str_validity", vs.data.vector_val->validity);
	CHECK_ALIGNED("vector_str_offsets", vs.data.vector_val->offsets);
	CHECK_ALIGNED("vector_str_validity", vs.data.vector_val->validity);

    /* --- closure --- */
    static const QFnInfo info = {"selfcheck", 0, 0, nullptr};
    QClosure* cl = q_alloc_closure(nullptr, &info, 0);
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
	gcInclude, gcLib, err := toolchain.EnsureGC()
	if err != nil {
		t.Fatalf("EnsureGC: %v", err)
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
		"-I" + toolchain.RuntimeIncludePath(),
		"-I" + gcInclude,
		"-o", outBin,
		cppFile,
	}
	args = append(args, toolchain.GCLinkArgs(gcLib)...)
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
    QValue b = q_to_vec(qv_list_from(9, qv_bool(true), qv_bool(false), qv_bool(true), qv_bool(true),
        qv_bool(false), qv_bool(false), qv_bool(false), qv_bool(false), qv_bool(true)));
    const QVector& bv = *b.data.vector_val;
    EXPECT("bool byte 0", bv.values[0] == 0x0D);
    EXPECT("bool byte 1", (bv.values[1] & 0x01) == 0x01);
    EXPECT("bool no validity", bv.validity == nullptr && bv.null_count == 0);

    // Validity bitmap: [1, null, 3] -> bits 1,0,1 -> 0x05, null_count 1.
    QValue i = q_to_vec(qv_list_from(3, qv_int(1), qv_null(), qv_int(3)));
    const QVector& iv = *i.data.vector_val;
    EXPECT("i64 validity byte", iv.validity && (iv.validity[0] & 0x07) == 0x05);
    EXPECT("i64 null_count", iv.null_count == 1);
    EXPECT("i64 values", q_vec_i64_data(iv)[0] == 1 && q_vec_i64_data(iv)[2] == 3);

    // Strings: ['ab', null, 'cde', ''] -> offsets 0,2,2,5,5 and bytes "abcde".
    QValue s = q_to_vec(qv_list_from(4, qv_string("ab"), qv_null(), qv_string("cde"), qv_string("")));
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
		"print(id(42))",
		"print(id('ok'))",
		"v: any = 1",
		"v = 'changed'",
		"print(v)",
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
		"print(add1(v))",
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

// TestNativeCallback_Arity checks that native code calling a Quark function
// value through qext::call gets the same argument-count check and default
// filling as a call in Quark.
func TestNativeCallback_Arity(t *testing.T) {
	tmp := t.TempDir()
	hpp := strings.Join([]string{
		"#pragma once",
		"#include <cstdint>",
		"inline QValue qei_call1(QClosure* f, QValue x) { return qext::call(f, x); }",
		"",
	}, "\n")
	if err := os.WriteFile(filepath.Join(tmp, "cb.hpp"), []byte(hpp), 0o644); err != nil {
		t.Fatal(err)
	}
	header := "extern './cb.hpp'\nextern fn call1(f: fn, x: any) any as 'qei_call1'\n"
	write := func(name, body string) {
		if err := os.WriteFile(filepath.Join(tmp, name), []byte(header+body), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	write("ok.qrk", "print(call1(fn(v, step = 2) -> v * step, 5))\n")
	if got := strings.TrimSpace(normalizeNewlines(runQuarkIn(t, tmp, "run", "ok.qrk"))); got != "10" {
		t.Fatalf("default filled by a native call: got %q, want 10", got)
	}

	write("bad.qrk", "print(call1(fn(a, b) -> b, 1))\n")
	cmd := exec.Command(quarkExePath, "run", "bad.qrk")
	cmd.Dir = tmp
	out, err := cmd.CombinedOutput()
	if err == nil || !strings.Contains(string(out), "function expects 2 arguments but got 1") {
		t.Fatalf("expected an arity error from the native call, got (err=%v):\n%s", err, out)
	}
}

// TestInput_EOFAndLongLines checks that input() returns null at end of input
// (distinct from an empty line), strips "\n" and "\r\n", and reads lines longer
// than its internal buffer in one piece.
func TestInput_EOFAndLongLines(t *testing.T) {
	tmp := t.TempDir()
	program := filepath.Join(tmp, "input_eof.qrk")
	source := strings.Join([]string{
		"n = 0",
		"while true:",
		"    r = input()",
		"    if is_err(r):",
		"        break",
		"    line = unwrap(r)",
		"    n = n + 1",
		"    print(to_str(len(line)))",
		"print('lines: ' + to_str(n))",
		"",
	}, "\n")
	if err := os.WriteFile(program, []byte(source), 0o644); err != nil {
		t.Fatalf("write %s: %v", program, err)
	}
	exe := filepath.Join(tmp, "input_eof")
	runQuark(t, "build", program, "-o", exe)

	stdin := "a\n\n" + strings.Repeat("x", 10000) + "\ncrlf\r\nno-newline"
	cmd := exec.Command(exe)
	cmd.Stdin = strings.NewReader(stdin)
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("run %s: %v", exe, err)
	}
	got := strings.TrimSpace(normalizeNewlines(string(out)))
	want := strings.Join([]string{"1", "0", "10000", "4", "10", "lines: 5"}, "\n")
	if got != want {
		t.Fatalf("unexpected output\n--- got ---\n%s\n--- want ---\n%s", got, want)
	}
}

// TestStdSys checks std/sys: program arguments (in a built binary and through
// `quark run -- ...`), environment variables, and exit status.
func TestStdSys(t *testing.T) {
	tmp := t.TempDir()
	program := filepath.Join(tmp, "sys_args.qrk")
	source := strings.Join([]string{
		"incl 'std/sys' as sys",
		"args = sys::args()",
		"print(to_str(len(args)) + ': ' + args.join('|'))",
		"print(sys::env('QK_SYS_TEST'))",
		"print(sys::env('QK_SYS_TEST_UNSET') == null)",
		"if len(args) > 0 and args[0] == 'fail':",
		"    sys::exit(3)",
		"print('done')",
		"",
	}, "\n")
	if err := os.WriteFile(program, []byte(source), 0o644); err != nil {
		t.Fatalf("write %s: %v", program, err)
	}
	exe := filepath.Join(tmp, "sys_args")
	runQuark(t, "build", program, "-o", exe)

	run := func(args ...string) (string, int) {
		cmd := exec.Command(exe, args...)
		cmd.Env = append(os.Environ(), "QK_SYS_TEST=hello")
		out, err := cmd.Output()
		code := 0
		if err != nil {
			var exitErr *exec.ExitError
			if !errors.As(err, &exitErr) {
				t.Fatalf("run %s: %v", exe, err)
			}
			code = exitErr.ExitCode()
		}
		return strings.TrimSpace(normalizeNewlines(string(out))), code
	}

	out, code := run("a", "b c", "--flag")
	want := "3: a|b c|--flag\nhello\ntrue\ndone"
	if out != want || code != 0 {
		t.Fatalf("args run: exit %d, output:\n%s\nwant exit 0, output:\n%s", code, out, want)
	}
	out, code = run("fail")
	if code != 3 || !strings.HasPrefix(out, "1: fail") || strings.Contains(out, "done") {
		t.Fatalf("exit run: exit %d, output:\n%s\nwant exit 3 before 'done'", code, out)
	}

	// quark run forwards the arguments after "--".
	cmd := exec.Command(quarkExePath, "run", program, "--", "x", "y")
	cmd.Env = append(os.Environ(), "QK_SYS_TEST=hello")
	got, err := cmd.Output()
	if err != nil {
		t.Fatalf("quark run -- x y: %v", err)
	}
	if !strings.HasPrefix(strings.TrimSpace(normalizeNewlines(string(got))), "2: x|y") {
		t.Fatalf("quark run -- x y: unexpected output:\n%s", got)
	}
}

// TestRuntimeErrorLocations checks that runtime errors name the statement that
// failed: in the caller after a function call returns, and in the imported
// module's own file for code spliced in from a module.
func TestRuntimeErrorLocations(t *testing.T) {
	tmp := t.TempDir()
	if err := os.MkdirAll(filepath.Join(tmp, "lib"), 0o755); err != nil {
		t.Fatal(err)
	}
	files := map[string]string{
		"after_call.qrk":  "fn id(x) -> x\n\n\nprint(id(5) + 'a')\n",
		"lib/helpers.qrk": "module helpers:\n    fn bad(x) ->\n        x + 'oops'\n",
		"in_module.qrk":   "incl './lib/helpers' as h\nprint(h::bad(1))\n",
		"in_stdlib.qrk":   "incl 'std/random' as random\n\nprint(random::randint(5, 1))\n",
		"in_for_body.qrk": "for r in list [1, 2]:\n    x = r\n    print(r % 0)\n",
		"in_while.qrk":    "i = 0\nwhile i < 1:\n    i = i + 1\n    print(i + 'a')\n",
	}
	for name, src := range files {
		if err := os.WriteFile(filepath.Join(tmp, name), []byte(src), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	cases := []struct{ file, want string }{
		{"after_call.qrk", "after_call.qrk:4:6"},
		{"in_module.qrk", "lib/helpers.qrk:3:11"},
		// Errors raised inside std/ modules point at the user's call.
		{"in_stdlib.qrk", "in_stdlib.qrk:3:6"},
		// Loop bodies set the location of each statement, not just the loop's.
		{"in_for_body.qrk", "in_for_body.qrk:3:"},
		{"in_while.qrk", "in_while.qrk:4:"},
	}
	for _, tc := range cases {
		cmd := exec.Command(quarkExePath, "run", tc.file)
		cmd.Dir = tmp
		var errBuf bytes.Buffer
		cmd.Stderr = &errBuf
		if err := cmd.Run(); err == nil {
			t.Fatalf("%s: expected a runtime error", tc.file)
		}
		if !strings.Contains(errBuf.String(), tc.want) {
			t.Fatalf("%s: expected location %q in:\n%s", tc.file, tc.want, errBuf.String())
		}
	}
}

// TestModuleNamespacing checks that members of different modules and files,
// and user definitions, never collide: everything from another file or module
// is reached with `::`, and nothing from another file is visible unqualified.
func TestModuleNamespacing(t *testing.T) {
	tmp := t.TempDir()
	if err := os.MkdirAll(filepath.Join(tmp, "lib"), 0o755); err != nil {
		t.Fatal(err)
	}
	files := map[string]string{
		"lib/a.qrk": "module a:\n    fn name() -> 'from a'\n    fn twice() -> name() + name()\n",
		"lib/b.qrk": "module b:\n    fn name() -> 'from b'\n",
		"ok.qrk": strings.Join([]string{
			"incl './lib/a' as a",
			"incl './lib/b' as b",
			"incl 'std/io' as io",
			"incl 'std/csv' as csv",
			"incl 'std/sys' as sys",
			"fn name() -> 'mine'",
			"open = 5",
			"args = sys::args()",
			"print(a::name() + ' ' + b::name() + ' ' + name() + ' ' + a::twice())",
			"f = unwrap(io::open('t.csv', 'w'))",
			"io::write(f, 'x,y\\n')",
			"io::close(f)",
			"g = unwrap(io::open('t.csv'))",
			"print(unwrap(io::read(g, 100)).trim() + ' ' + to_str(len(unwrap(csv::read('t.csv')))))",
			"print(to_str(open) + ' ' + to_str(len(args)))",
			"",
		}, "\n"),
		"not_visible.qrk": "incl './lib/a'\nprint(name())\n",
		"dot_access.qrk":  "incl './lib/a'\nprint(a.name())\n",
		"flat_stdlib.qrk": "incl 'std/fmt'\nprint(show_vec(vec [1]))\n",
	}
	for name, src := range files {
		if err := os.WriteFile(filepath.Join(tmp, name), []byte(src), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	got := strings.TrimSpace(normalizeNewlines(runQuarkIn(t, tmp, "run", "ok.qrk")))
	want := "from a from b mine from afrom a\nx,y 1\n5 0"
	if got != want {
		t.Fatalf("ok.qrk output:\n%s\nwant:\n%s", got, want)
	}
	for file, wantErr := range map[string]string{
		"not_visible.qrk": "undefined identifier 'name'",
		"dot_access.qrk":  "'a' is an imported file; use a::name",
		"flat_stdlib.qrk": "undefined identifier 'show_vec'",
	} {
		cmd := exec.Command(quarkExePath, "run", file)
		cmd.Dir = tmp
		out, err := cmd.CombinedOutput()
		if err == nil || !strings.Contains(string(out), wantErr) {
			t.Fatalf("%s: expected error %q, got (err=%v):\n%s", file, wantErr, err, out)
		}
	}
}

// TestNamesArePerFile: each file has its own scope (modules v0.2 rule 12).
// Two files can define the same name, a library's own calls always reach its
// own definitions whatever the importer defines, and a file imported along
// two paths is one file.
func TestNamesArePerFile(t *testing.T) {
	tmp := t.TempDir()
	if err := os.MkdirAll(filepath.Join(tmp, "lib"), 0o755); err != nil {
		t.Fatal(err)
	}
	files := map[string]string{
		"lib/helpy.qrk":       "fn helper(x) -> x + 100\nstruct Box:\n    v: int\nmodule helpy:\n    fn api(x) -> helper(x)\n",
		"lib/helpy2.qrk":      "fn helper(x) -> x + 200\nmodule helpy2:\n    fn api(x) -> helper(x)\n",
		"lib/base.qrk":        "fn shared(x) -> x * 2\nmodule base:\n    fn twice(x) -> shared(x)\n",
		"lib/left.qrk":        "incl './base.qrk' as b\nmodule left:\n    fn go(x) -> b::shared(x) + 1\n",
		"lib/right.qrk":       "incl './base.qrk' as b\nmodule right:\n    fn go(x) -> b::shared(x) + 2\n",
		"ok.qrk":              "incl './lib/helpy' as h\nincl './lib/left' as l\nincl './lib/right' as r\nfn mine(x) -> x\nprint(h::api(1))\nprint(l::go(1) + r::go(1))\nprint(h::Box { v: mine(3) })\n",
		"fn_after.qrk":        "incl './lib/helpy' as h\nfn helper(x) -> x + 1\nprint(h::api(1))\nprint(helper(1))\n",
		"fn_before.qrk":       "fn helper(x) -> x + 1\nincl './lib/helpy' as h\nprint(h::api(1))\n",
		"variable.qrk":        "incl './lib/helpy' as h\nhelper = 5\nprint(h::api(1))\nprint(helper)\n",
		"struct.qrk":          "incl './lib/helpy' as h\nstruct Box:\n    w: int\nprint(Box { w: 1 })\nprint(h::Box { v: 2 })\n",
		"two_libs.qrk":        "incl './lib/helpy' as h\nincl './lib/helpy2' as h2\nprint(h::api(1))\nprint(h2::api(1))\n",
		"stdlib_names.qrk":    "incl 'std/fmt' as fmt\nfn fmt_full(x) -> 'mine'\nfmt_vec = 3\nprint(fmt_full(1))\nprint(fmt_vec)\nprint(fmt::fmt_full(list [1]))\n",
		"struct_mismatch.qrk": "incl './lib/helpy' as h\nstruct Box:\n    v: int\nfn f(b: Box) -> b.v\nprint(f(h::Box { v: 1 }))\n",
		"lib/u1.qrk":          "module utils:\n    fn tag() -> 'u1'\n",
		"lib/u2.qrk":          "module utils:\n    fn tag() -> 'u2'\n",
		"two_utils.qrk":       "incl './lib/u1'\nincl './lib/u2'\nprint(u1::utils::tag() + u2::utils::tag())\n",
		// A library's locals never see the importer's names, which are
		// spliced into the same tree with their bare names.
		"lib/acc.qrk": "fn g() ->\n    total = 5\n    inner = fn() -> total\n    inner() + 1\n" +
			"fn h(r, n: int) ->\n    s = 0\n    for total in range(n):\n        s = s + total\n    when r:\n        ok total -> total + s\n        err e -> 0\n" +
			"fn k(x) ->\n    fn total(y) -> y * 2\n    total(x)\n" +
			"fn rt(total: int) -> total\n",
		"locals.qrk":    "incl './lib/acc'\nfn total() -> 100\nfn inner() -> 'main inner'\nprint(acc::g())\nprint(acc::h(ok(10), 3))\nprint(acc::k(4))\nprint(total())\nprint(acc::rt(7))\n",
		"locals_rt.qrk": "incl './lib/acc'\nfn total() -> 1\nfn id(v) -> v\nprint(acc::rt(id('s')))\n",
	}
	for name, src := range files {
		if err := os.WriteFile(filepath.Join(tmp, name), []byte(src), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	for file, want := range map[string]string{
		"ok.qrk":           "101\n7\nhelpy::Box {v: 3}",
		"fn_after.qrk":     "101\n2",
		"fn_before.qrk":    "101",
		"variable.qrk":     "101\n5",
		"struct.qrk":       "Box {w: 1}\nhelpy::Box {v: 2}",
		"two_libs.qrk":     "101\n201",
		"stdlib_names.qrk": "mine\n3\n[1]",
		"two_utils.qrk":    "u1u2",
		"locals.qrk":       "6\n13\n8\n100\n7",
	} {
		got := strings.TrimSpace(normalizeNewlines(runQuarkIn(t, tmp, "run", file)))
		if got != want {
			t.Fatalf("%s output:\n%s\nwant:\n%s", file, got, want)
		}
	}
	cmd := exec.Command(quarkExePath, "run", "struct_mismatch.qrk")
	cmd.Dir = tmp
	out, err := cmd.CombinedOutput()
	if wantErr := "argument 1 of 'f' expects Box, got helpy::Box"; err == nil || !strings.Contains(string(out), wantErr) {
		t.Fatalf("struct_mismatch.qrk: expected error %q, got (err=%v):\n%s", wantErr, err, out)
	}
	// A renamed local is named as written in runtime messages.
	cmd = exec.Command(quarkExePath, "run", "locals_rt.qrk")
	cmd.Dir = tmp
	out, err = cmd.CombinedOutput()
	if wantErr := "argument 'total' of 'acc::rt' expects int, got str"; err == nil || !strings.Contains(string(out), wantErr) {
		t.Fatalf("locals_rt.qrk: expected error %q, got (err=%v):\n%s", wantErr, err, out)
	}
}

// TestStructsAcrossFiles: a struct of an imported file is named file::Struct
// (or file::module::Struct) in literals, annotations and return types; its
// field values are checked like a local struct's; a struct can be defined in
// a module body; and struct identity is the definition, at compile time and
// at runtime, so two structs named Point never pass for each other.
func TestStructsAcrossFiles(t *testing.T) {
	tmp := t.TempDir()
	if err := os.MkdirAll(filepath.Join(tmp, "lib"), 0o755); err != nil {
		t.Fatal(err)
	}
	files := map[string]string{
		"lib/geo.qrk":    "struct Point:\n    x: int\n    y: float = 0\n\nmodule geo:\n    fn sum(p: Point) float -> p.x + p.y\n",
		"lib/geo2.qrk":   "struct Point:\n    x: int\n",
		"lib/shapes.qrk": "module shapes:\n    struct Sq:\n        side: int\n    fn mk(n) -> Sq { side: n }\n    fn area(s: Sq) -> s.side * s.side\nfn outside() -> shapes::Sq { side: 2 }\n",
		"ok.qrk":         "incl './lib/geo.qrk'\np = geo::Point { x: 3, y: 4 }\nprint(geo::sum(p))\nfn mk(a: int) geo::Point -> geo::Point { x: a }\nprint(mk(2))\nprint(type(p))\n",
		"bad.qrk":        "incl './lib/geo.qrk'\nfn mk(a) -> geo::Point { x: a }\nprint(mk('s'))\n",
		"module_struct.qrk": "incl './lib/shapes'\nprint(shapes::area(shapes::mk(3)))\nprint(shapes::outside())\nprint(shapes::shapes::Sq { side: 1 })\n" +
			"module local:\n    struct L:\n        n: int\n    fn mk() -> L { n: 1 }\nprint(local::mk())\nprint(local::L { n: 2 })\n",
		"identity_static.qrk":  "incl './lib/geo'\nincl './lib/geo2'\nfn f(p: geo::Point) -> p.x\nprint(f(geo2::Point { x: 1 }))\n",
		"identity_runtime.qrk": "incl './lib/geo'\nincl './lib/geo2'\nfn f(p: geo::Point) -> p.x\nq: any = geo2::Point { x: 1 }\nprint(f(q))\n",
		"identity_field.qrk":   "incl './lib/geo'\nincl './lib/geo2'\nstruct W:\n    p: geo::Point\nq: any = geo2::Point { x: 1 }\nprint(W { p: q })\n",
		"dot_literal.qrk":      "incl './lib/shapes'\nx = shapes.Sq { side: 1 }\n",
	}
	for name, src := range files {
		if err := os.WriteFile(filepath.Join(tmp, name), []byte(src), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	for file, want := range map[string]string{
		"ok.qrk":            "7.0\ngeo::Point {x: 2, y: 0.0}\ngeo::Point",
		"module_struct.qrk": "9\nshapes::Sq {side: 2}\nshapes::Sq {side: 1}\nlocal::L {n: 1}\nlocal::L {n: 2}",
	} {
		got := strings.TrimSpace(normalizeNewlines(runQuarkIn(t, tmp, "run", file)))
		if got != want {
			t.Fatalf("%s output:\n%s\nwant:\n%s", file, got, want)
		}
	}
	for file, wantErr := range map[string]string{
		"bad.qrk":              "field 'x' of geo::Point expects int, got str",
		"identity_static.qrk":  "argument 1 of 'f' expects geo::Point, got geo2::Point",
		"identity_runtime.qrk": "argument 'p' of 'f' expects geo::Point, got geo2::Point",
		"identity_field.qrk":   "field 'p' of W expects geo::Point, got geo2::Point",
		"dot_literal.qrk":      "a struct from a module or imported file is written shapes::Sq { ... }, with '::'",
	} {
		cmd := exec.Command(quarkExePath, "run", file)
		cmd.Dir = tmp
		out, err := cmd.CombinedOutput()
		if err == nil || !strings.Contains(string(out), wantErr) || strings.Contains(string(out), "internal:") {
			t.Fatalf("%s: expected error %q, got (err=%v):\n%s", file, wantErr, err, out)
		}
	}
}

// TestParamAnnotationsEnforced: each annotated parameter is enforced on its
// own, also for partially annotated functions and for values of unknown type,
// and that a fully annotated function that captures a variable compiles (it
// stays a closure instead of becoming a native C++ function).
func TestParamAnnotationsEnforced(t *testing.T) {
	ok := strings.Join([]string{
		"fn id(v) -> v",
		"fn scale(x: float, k) -> x * k",
		"fn kinds(x: int, y) -> type(x) + ' ' + type(y)",
		"fn first(xs: list) -> type(xs)",
		"n = 10",
		"fn add_n(x: int) int -> x + n",
		"g = scale",
		"print(scale(1, 2))",
		"print(g(id(3), 1))",
		"print(kinds(1, 'b'))",
		"print(first(list []))",
		"print(add_n(1))",
		"print(id(2) | scale(3))",
		"",
	}, "\n")
	tmp := t.TempDir()
	if err := os.WriteFile(filepath.Join(tmp, "ok.qrk"), []byte(ok), 0o644); err != nil {
		t.Fatal(err)
	}
	got := strings.TrimSpace(normalizeNewlines(runQuarkIn(t, tmp, "run", "ok.qrk")))
	if want := "2.0\n3.0\nint str\nlist\n11\n6.0"; got != want {
		t.Fatalf("output:\n%s\nwant:\n%s", got, want)
	}

	bad := map[string]string{
		"partial":   "fn id(v) -> v\nfn kinds(x: int, y) -> type(x)\nprint(kinds(id('a'), 1))\n",
		"via_value": "fn id(v) -> v\nfn f(x: float) -> x\ng = f\nprint(g(id('s')))\n",
		"default":   "fn id(v) -> v\nfn add(x: int, y: int = 5) -> x + y\nprint(add(1, id(2.5)))\n",
		"struct":    "struct P:\n    x: int\nstruct Q:\n    x: int\nfn id(v) -> v\nfn f(p: P) -> p.x\nprint(f(id(Q { x: 2 })))\n",
		"captured":  "fn id(v) -> v\nn = 10\nfn f(x: int) int -> x + n\nprint(f(id('z')))\n",
	}
	wants := map[string]string{
		"partial":   "argument 'x' of 'kinds' expects int, got str at bad_partial.qrk:3:6",
		"via_value": "argument 'x' of 'f' expects float, got str",
		"default":   "argument 'y' of 'add' expects int, got float",
		"struct":    "argument 'p' of 'f' expects P, got Q",
		"captured":  "argument 'x' of 'f' expects int, got str",
	}
	for name, src := range bad {
		file := "bad_" + name + ".qrk"
		if err := os.WriteFile(filepath.Join(tmp, file), []byte(src), 0o644); err != nil {
			t.Fatal(err)
		}
		cmd := exec.Command(quarkExePath, "run", file)
		cmd.Dir = tmp
		out, err := cmd.CombinedOutput()
		if err == nil || !strings.Contains(string(out), wants[name]) {
			t.Fatalf("%s: expected %q, got (err=%v):\n%s", name, wants[name], err, out)
		}
	}
}
