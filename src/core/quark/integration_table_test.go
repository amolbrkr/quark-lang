package main

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"quark/toolchain"
)

// TestTableEngine compiles the C++ tests of the table engine and CSV reader
// (src/testfiles/table/*_test.cpp) against the runtime headers, with -O2 so
// the timings they print are meaningful, and runs them.
func TestTableEngine(t *testing.T) {
	if _, err := exec.LookPath("clang++"); err != nil {
		t.Skip("skipping: clang++ not found in PATH")
	}
	gcInclude, gcLib, err := toolchain.EnsureGC()
	if err != nil {
		t.Fatalf("EnsureGC: %v", err)
	}
	dir := filepath.Join(repoRootFromThisFile(t), "src", "testfiles", "table")
	sources, err := filepath.Glob(filepath.Join(dir, "*_test.cpp"))
	if err != nil || len(sources) == 0 {
		t.Fatalf("no table tests in %s (%v)", dir, err)
	}
	for _, src := range sources {
		src := src
		name := strings.TrimSuffix(filepath.Base(src), ".cpp")
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			tmp := t.TempDir()
			bin := filepath.Join(tmp, name)
			if runtime.GOOS == "windows" {
				bin += ".exe"
			}
			args := []string{
				"-std=c++17", "-O2", "-Wall", "-Wextra", "-Werror",
				"-DQUARK_USE_GC",
				"-I" + toolchain.RuntimeIncludePath(),
				"-I" + gcInclude,
				"-I" + dir,
				"-o", bin, src,
			}
			args = append(args, toolchain.GCLinkArgs(gcLib)...)
			if runtime.GOOS != "windows" {
				args = append(args, "-lm")
			}
			var compileOut bytes.Buffer
			cmd := exec.Command("clang++", args...)
			cmd.Stdout, cmd.Stderr = &compileOut, &compileOut
			if err := cmd.Run(); err != nil {
				t.Fatalf("compile %s: %v\n%s", filepath.Base(src), err, compileOut.String())
			}
			var out bytes.Buffer
			run := exec.Command(bin, tmp)
			run.Stdout, run.Stderr = &out, &out
			err := run.Run()
			t.Logf("%s", strings.TrimSpace(out.String()))
			if err != nil || !strings.Contains(out.String(), " 0 failed") {
				t.Fatalf("%s reported failures (%v):\n%s", name, err, out.String())
			}
		})
	}
}

// TestTableEngine_FatalErrors checks the engine's runtime errors: each case
// is a small program that must exit with status 1 and print its message.
func TestTableEngine_FatalErrors(t *testing.T) {
	cases := []struct{ name, body, want string }{
		{"sum_overflow",
			`QVector* v = q_vec_alloc(QVector::Type::I64, 2);
			q_vec_i64_data_mut(*v)[0] = INT64_MAX; q_vec_i64_data_mut(*v)[1] = 1;
			quark::tbl::agg_sum(*v, quark::tbl::whole_table(2));`,
			"integer overflow in 'sum'"},
		{"sum_of_str",
			`QValue s = qv_vector_str(); q_vec_str_append(*s.data.vector_val, "a", 1);
			quark::tbl::agg_sum(*s.data.vector_val, quark::tbl::whole_table(1));`,
			"sum needs a number or bool column, got str"},
		{"mean_of_str",
			`QValue s = qv_vector_str(); q_vec_str_append(*s.data.vector_val, "a", 1);
			quark::tbl::agg_mean(*s.data.vector_val, quark::tbl::whole_table(1));`,
			"mean needs a number or bool column, got str"},
		{"ragged_columns",
			`const char* names[] = {"a", "b"}; QStructDef def = {"T", names, 2};
			QVector* cols[] = {q_vec_alloc(QVector::Type::I64, 2), q_vec_alloc(QVector::Type::I64, 3)};
			quark::tbl::make_table(&def, cols);`,
			"internal: table columns have different lengths"},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			src := "#include \"quark/quark.hpp\"\n#include \"quark/table/engine.hpp\"\n#include <cstdint>\n" +
				"int main() {\n    q_gc_init();\n    " + tc.body + "\n    return 0;\n}\n"
			cppFile := filepath.Join(t.TempDir(), tc.name+".cpp")
			out, code := compileAndRunExpectingExit(t, cppFile, src)
			if code != 1 || !strings.Contains(out, tc.want) {
				t.Fatalf("expected exit 1 with %q, got exit %d:\n%s", tc.want, code, out)
			}
		})
	}
}

// compileAndRunExpectingExit compiles a C++ program against the runtime and
// returns its combined output and exit code.
func compileAndRunExpectingExit(t *testing.T, cppFile, source string) (string, int) {
	t.Helper()
	if _, err := exec.LookPath("clang++"); err != nil {
		t.Skip("skipping: clang++ not found in PATH")
	}
	gcInclude, gcLib, err := toolchain.EnsureGC()
	if err != nil {
		t.Fatalf("EnsureGC: %v", err)
	}
	if err := os.WriteFile(cppFile, []byte(source), 0o644); err != nil {
		t.Fatal(err)
	}
	bin := strings.TrimSuffix(cppFile, ".cpp")
	if runtime.GOOS == "windows" {
		bin += ".exe"
	}
	args := []string{"-std=c++17", "-O0", "-DQUARK_USE_GC", "-I" + toolchain.RuntimeIncludePath(), "-I" + gcInclude, "-o", bin, cppFile}
	args = append(args, toolchain.GCLinkArgs(gcLib)...)
	if runtime.GOOS != "windows" {
		args = append(args, "-lm")
	}
	if out, err := exec.Command("clang++", args...).CombinedOutput(); err != nil {
		t.Fatalf("compile: %v\n%s", err, out)
	}
	out, err := exec.Command(bin).CombinedOutput()
	code := 0
	if exitErr, ok := err.(*exec.ExitError); ok {
		code = exitErr.ExitCode()
	} else if err != nil {
		t.Fatalf("run: %v", err)
	}
	return string(out), code
}
