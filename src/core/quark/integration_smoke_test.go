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
			name: "module_use",
			file: filepath.Join(testfilesDir, "smoke_module_use.qrk"),
			expected: join(
				"== smoke: module/use ==",
				"25",
				"27",
				"5",
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
				"vector[f64]",
				"vector[i64]",
				"[vector len=3]",
				"60",
				"[vector len=4]",
				"[list len=4]",
				"red",
				"blue",
				"list",
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
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := runQuark(t, "run", tc.file)
			gotNorm := strings.TrimSpace(normalizeNewlines(got))
			expNorm := strings.TrimSpace(normalizeNewlines(tc.expected))
			if gotNorm != expNorm {
				t.Fatalf("unexpected output\n--- got ---\n%s\n--- expected ---\n%s", gotNorm, expNorm)
			}
		})
	}
}
