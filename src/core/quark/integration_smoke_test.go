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
				"10.0",
				"1.0",
				"4.0",
				"[1, 2, 3, 4]",
				"10.0",
				"1.0",
				"4.0",
				"[3, 4, 5, 6]",
				"[3, 8, 15, 24]",
				"[1, 2, 3, 4]",
				"vector[i64]",
				"vector[i64]",
				"[10, 20, 30]",
				"60.0",
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
				"vector[i64]",
				"vector[str]",
				"vector[str]",
				"[false, false, true, true, true]",
				"3.0",
				"2.0",
				"3.0",
				"3.0",
				"1.0",
				"4.0",
				"1.0",
				"[30, 40, 50]",
				"3",
				"10",
				"50",
				"50",
				"120.0",
				"0",
				"[4, 5]",
				"1.0",
				"4.0",
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
				"12.0",
				"vector[bool]",
				"2.0",
				"vector[bool]",
				"2.0",
				"4.0",
				"1.0",
				"3.0",
				"15",
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
				"== smoke: vector layout ==",
				"10.0", "false", "true", "true", "10", "155.0",
				"2", "null", "null", "78.0", "null", "true",
				"2", "40.0",
				"0", "null", "39.0",
				"ab", "null", "cde", "0", "[false, null, true, false]", "true", "zz", "cde",
				"null", "3.0", "true", "true",
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

			// Run from a temp dir: smoke_stdlib_io.qrk writes a relative file.
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
		// Methods on values of unknown type: checked at compile time when no type
		// has the method or the arity is wrong, at runtime when the value's type lacks it.
		{"dynamic_method_wrong_type", "fn id(x) -> x\nprintln(id(5).upper())\n", "type 'int' has no method 'upper'"},
		{"dynamic_method_unknown", "fn id(x) -> x\nid(5).frobnicate()\n", "no type has a method 'frobnicate'"},
		{"dynamic_method_arity", "fn id(x) -> x\nid('a').split()\n", "method 'split' expects 1 argument(s) but got 0"},
		{"dynamic_method_arg_type", "fn id(x) -> x\nid('a').split(5)\n", "argument 1 of 'split' expects str, got int"},
		// Ordering compares two numbers or two strings; mixes are runtime errors.
		{"order_str_int", "println('a' < 1)\n", "operator '<' expects two numbers or two strings"},
		{"csv_unterminated_quote", "use 'std/csv' as csv\nprintln(csv.parse('a,\"b\\n'))\n", "csv.parse: unterminated quoted field"},
		{"csv_bad_separator", "use 'std/csv' as csv\nprintln(csv.parse('a', ';;'))\n", "separator must be a single character"},
		// A statement can't call a function defined later in the same block.
		{"call_before_definition", "println(later(2))\nfn later(x) -> x + 1\n", "'later' is called before its definition on line 2"},
		{"call_before_definition_in_fn", "fn outer() ->\n    r = inner()\n    fn inner() -> 1\n    r\nprintln(outer())\n", "'inner' is called before its definition on line 3"},
		{"chained_assignment", "a = 0\nb = 0\na = b = 1\n", "chained assignment is not supported"},
		// sort keys must be mutually comparable.
		{"sort_mixed_types", "println(list [1, 'a'].sort())\n", "sort() can't compare str and int"},
		{"sort_null", "println(list [1, null].sort())\n", "sort() can't compare null and int"},
		{"sort_by_not_fn", "println(list [1].sort_by(5))\n", "sort_by() expects a function"},
		// when patterns are literals, '_' or ok/err bindings.
		{"when_call_pattern", "fn f(x) -> x\nn = 3\nwhen n:\n    f(3) -> println(1)\n    _ -> println(2)\n", "when patterns must be literal values"},
		{"when_identifier_pattern", "y = 3\nn = 3\nwhen n:\n    y -> println(1)\n    _ -> println(2)\n", "when patterns must be literal values"},
		{"when_result_or", "r = ok 1\nwhen r:\n    ok x or err x -> println(1)\n", "ok patterns cannot be combined with other patterns using 'or'"},
		// The lowered for-range loop checks its arguments like range().
		{"range_loop_step_zero", "for i in range(0, 5, 0):\n    println(i)\n", "range() step must not be zero"},
		{"range_loop_step_zero_dynamic", "fn id(x) -> x\nfor i in range(0, 5, id(0)):\n    println(i)\n", "range() step must not be zero"},
		{"range_loop_non_numeric", "fn id(x) -> x\nfor i in range(id('x')):\n    println(i)\n", "range(end) expects numeric end value"},
		{"when_literal_or_result", "n = 1\nwhen n:\n    1 or ok x -> println(1)\n    _ -> println(2)\n", "ok patterns cannot be combined with other patterns using 'or'"},
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

func TestStdIO_FileModes(t *testing.T) {
	tmp := t.TempDir()
	targetPath := filepath.Join(tmp, "io_v0.txt")
	targetForQuark := strings.ReplaceAll(targetPath, "\\", "/")

	program := filepath.Join(tmp, "std_io_modes.qrk")
	source := strings.Join([]string{
		"use 'std/io' as io",
		"p = '" + targetForQuark + "'",
		"f = unwrap(io.open(p, 'w'))",
		"println(type(f))",
		"println(type(io.exists(p)))",
		"io.write(f, 'hello file') | unwrap() | println()",
		"io.close(f) | unwrap()",
		"r = unwrap(io.open(p, 'r'))",
		"println(unwrap(io.read(r, 64)))",
		"io.close(r) | unwrap()",
		"println(io.exists(p))",
		"a = unwrap(io.open(p, 'a'))",
		"io.write(a, '!') | unwrap()",
		"io.close(a) | unwrap()",
		"r2 = unwrap(io.open(p, 'r'))",
		"println(unwrap(io.read(r2, 64)))",
		"io.close(r2) | unwrap()",
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

// TestInput_EOFAndLongLines checks that input() returns null at end of input
// (distinct from an empty line), strips "\n" and "\r\n", and reads lines longer
// than its internal buffer in one piece.
func TestInput_EOFAndLongLines(t *testing.T) {
	tmp := t.TempDir()
	program := filepath.Join(tmp, "input_eof.qrk")
	source := strings.Join([]string{
		"n = 0",
		"while true:",
		"    line = input()",
		"    if line == null:",
		"        break",
		"    n = n + 1",
		"    println(to_str(len(line)))",
		"println('lines: ' + to_str(n))",
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
		"use 'std/sys' as sys",
		"args = sys.args()",
		"println(to_str(len(args)) + ': ' + args.join('|'))",
		"println(sys.env('QK_SYS_TEST'))",
		"println(sys.env('QK_SYS_TEST_UNSET') == null)",
		"if len(args) > 0 and args[0] == 'fail':",
		"    sys.exit(3)",
		"println('done')",
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
		"after_call.qrk":  "fn id(x) -> x\n\n\nprintln(id(5) + 'a')\n",
		"lib/helpers.qrk": "module helpers:\n    fn bad(x) ->\n        x + 'oops'\n",
		"in_module.qrk":   "use './lib/helpers' as h\nprintln(h.bad(1))\n",
		"in_stdlib.qrk":   "use 'std/random' as random\n\nprintln(random.randint(5, 1))\n",
	}
	for name, src := range files {
		if err := os.WriteFile(filepath.Join(tmp, name), []byte(src), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	cases := []struct{ file, want string }{
		{"after_call.qrk", "after_call.qrk:4:8"},
		{"in_module.qrk", "lib/helpers.qrk:3:11"},
		// Errors raised inside std/ modules point at the user's call.
		{"in_stdlib.qrk", "in_stdlib.qrk:3:8"},
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

// TestModuleNamespacing checks that module members don't collide with members
// of other modules or with user definitions, and that ambiguous bare imports
// are reported.
func TestModuleNamespacing(t *testing.T) {
	tmp := t.TempDir()
	if err := os.MkdirAll(filepath.Join(tmp, "lib"), 0o755); err != nil {
		t.Fatal(err)
	}
	files := map[string]string{
		"lib/a.qrk": "module a:\n    fn name() -> 'from a'\n    fn twice() -> name() + name()\n",
		"lib/b.qrk": "module b:\n    fn name() -> 'from b'\n",
		"ok.qrk": strings.Join([]string{
			"use './lib/a' as a",
			"use './lib/b' as b",
			"use 'std/io' as io",
			"use 'std/csv' as csv",
			"use 'std/sys' as sys",
			"fn name() -> 'mine'",
			"open = 5",
			"args = sys.args()",
			"println(a.name() + ' ' + b.name() + ' ' + name() + ' ' + a.twice())",
			"f = unwrap(io.open('t.csv', 'w'))",
			"io.write(f, 'x,y\\n')",
			"io.close(f)",
			"g = unwrap(io.open('t.csv'))",
			"println(unwrap(io.read(g, 100)).trim() + ' ' + to_str(len(unwrap(csv.read('t.csv')))))",
			"println(to_str(open) + ' ' + to_str(len(args)))",
			"",
		}, "\n"),
		"bare_ok.qrk":       "use './lib/a'\nprintln(name())\nfn f(name) -> name\nprintln(f('param'))\n",
		"bare_conflict.qrk": "use './lib/a'\nuse './lib/b'\nprintln(name())\n",
		"bare_vs_def.qrk":   "use './lib/a'\nfn name() -> 1\n",
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
	got = strings.TrimSpace(normalizeNewlines(runQuarkIn(t, tmp, "run", "bare_ok.qrk")))
	if got != "from a\nparam" {
		t.Fatalf("bare_ok.qrk output:\n%s", got)
	}
	for file, wantErr := range map[string]string{
		"bare_conflict.qrk": "symbol 'name' from module 'b' conflicts with symbol 'name' from module 'a'",
		"bare_vs_def.qrk":   "symbol 'name' from module 'a' (imported on line 1) conflicts with this definition",
	} {
		cmd := exec.Command(quarkExePath, "run", file)
		cmd.Dir = tmp
		out, err := cmd.CombinedOutput()
		if err == nil || !strings.Contains(string(out), wantErr) {
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
		"println(scale(1, 2))",
		"println(g(id(3), 1))",
		"println(kinds(1, 'b'))",
		"println(first(null))",
		"println(add_n(1))",
		"println(id(2) | scale(3))",
		"",
	}, "\n")
	tmp := t.TempDir()
	if err := os.WriteFile(filepath.Join(tmp, "ok.qrk"), []byte(ok), 0o644); err != nil {
		t.Fatal(err)
	}
	got := strings.TrimSpace(normalizeNewlines(runQuarkIn(t, tmp, "run", "ok.qrk")))
	if want := "2.0\n3.0\nint str\nnull\n11\n6.0"; got != want {
		t.Fatalf("output:\n%s\nwant:\n%s", got, want)
	}

	bad := map[string]string{
		"partial":   "fn id(v) -> v\nfn kinds(x: int, y) -> type(x)\nprintln(kinds(id('a'), 1))\n",
		"via_value": "fn id(v) -> v\nfn f(x: float) -> x\ng = f\nprintln(g(id('s')))\n",
		"default":   "fn id(v) -> v\nfn add(x: int, y: int = 5) -> x + y\nprintln(add(1, id(2.5)))\n",
		"struct":    "struct P:\n    x: int\nstruct Q:\n    x: int\nfn id(v) -> v\nfn f(p: P) -> p.x\nprintln(f(id(Q { x: 2 })))\n",
		"captured":  "fn id(v) -> v\nn = 10\nfn f(x: int) int -> x + n\nprintln(f(id('z')))\n",
	}
	wants := map[string]string{
		"partial":   "argument 'x' of 'kinds' expects int, got str at bad_partial.qrk:3:8",
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
