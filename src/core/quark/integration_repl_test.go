package main

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestRepl_ValuesSurviveAndFailureRollsBack(t *testing.T) {
	cmd := exec.Command(quarkExePath, "repl")
	cmd.Stdin = strings.NewReader(strings.Join([]string{
		"n = 1",
		"n",
		"if true:",
		"    n = 2",
		"    print(1 / 0)",
		"",
		"n",
		"",
	}, "\n"))
	var out, errb bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errb
	done := make(chan error, 1)
	go func() { done <- cmd.Run() }()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("repl: %v\nstderr:\n%s", err, errb.String())
		}
	case <-time.After(3 * time.Minute):
		_ = cmd.Process.Kill()
		t.Fatal("repl timed out\n" + errb.String())
	}
	got := strings.ReplaceAll(out.String(), "\r\n", "\n")
	if strings.Count(got, "1\n") < 2 {
		t.Fatalf("stdout:\n%s\nstderr:\n%s", got, errb.String())
	}
	if strings.Contains(got, "2\n") {
		t.Fatalf("failed input kept n = 2\nstdout:\n%s\nstderr:\n%s", got, errb.String())
	}
	if !strings.Contains(errb.String(), "division by zero") {
		t.Fatalf("missing runtime error\nstderr:\n%s", errb.String())
	}
}

func TestRepl_EarlierInputStaysCallable(t *testing.T) {
	cmd := exec.Command(quarkExePath, "repl")
	cmd.Stdin = strings.NewReader(strings.Join([]string{
		"fn sq(x: int) int -> x * x",
		"sq(4)",
		"sq('no')",
		"sq(3)",
		"incl 'std/sys' as sys",
		"sys::platform()",
		"sys::cwd()",
		"1 + 1",
		"",
	}, "\n"))
	var out, errb bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errb
	done := make(chan error, 1)
	go func() { done <- cmd.Run() }()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("repl: %v\nstderr:\n%s", err, errb.String())
		}
	case <-time.After(4 * time.Minute):
		_ = cmd.Process.Kill()
		t.Fatal("repl timed out\n" + errb.String())
	}
	got := strings.ReplaceAll(out.String(), "\r\n", "\n")
	errText := errb.String()
	if strings.Contains(errText, "Compilation failed") {
		t.Fatalf("compile error\nstdout:\n%s\nstderr:\n%s", got, errText)
	}
	for _, want := range []string{"16\n", "9\n", "2\n"} {
		if !strings.Contains(got, want) {
			t.Fatalf("missing %q\nstdout:\n%s\nstderr:\n%s", want, got, errText)
		}
	}
	if !strings.Contains(errText, "expects int") {
		t.Fatalf("wrong-type call did not fail\nstderr:\n%s", errText)
	}
	if !strings.Contains(got, "windows\n") && !strings.Contains(got, "linux\n") && !strings.Contains(got, "darwin\n") {
		t.Fatalf("platform missing\nstdout:\n%s", got)
	}
	if !strings.Contains(got, ":\\") && !strings.Contains(got, "/") {
		t.Fatalf("cwd missing\nstdout:\n%s\nstderr:\n%s", got, errText)
	}
}

func TestRepl_CSVReadAfterIncl(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "cars.csv"), []byte("model,mpg,cyl\nMazda,21,6\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(quarkExePath, "repl")
	cmd.Dir = dir
	cmd.Stdin = strings.NewReader(strings.Join([]string{
		"incl 'std/csv'",
		"struct car:",
		"    model: str",
		"    mpg: int",
		"    cyl: int",
		"",
		"csv::read_file(1)",
		"csv::read_file(null)",
		"csv::read_file()",
		"text: tbl[car] = unwrap(csv::parse_text('model,mpg,cyl\\nMazda,21,6\\n'))",
		"print(sum(text.mpg))",
		"fn id(v) -> v",
		"csv::read_file(id(1))",
		"cars: tbl[car] = unwrap(csv::read_file('cars.csv'))",
		"print(cars.model)",
		"print(csv::format_text(cars))",
		"unwrap(csv::read_file('missing.csv'))",
		"print(sum(cars.mpg))",
		"",
	}, "\n"))
	var out, errb bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errb
	done := make(chan error, 1)
	go func() { done <- cmd.Run() }()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("repl: %v\nstderr:\n%s", err, errb.String())
		}
	case <-time.After(4 * time.Minute):
		_ = cmd.Process.Kill()
		t.Fatal("repl timed out\n" + errb.String())
	}
	got := strings.ReplaceAll(out.String(), "\r\n", "\n")
	errText := errb.String()
	if strings.Contains(errText, "Compilation failed") || strings.Contains(errText, "undeclared identifier") {
		t.Fatalf("C++ compile failed\nstdout:\n%s\nstderr:\n%s", got, errText)
	}
	for _, want := range []string{"21\n", "['Mazda']\n", "Mazda,21,6\n"} {
		if !strings.Contains(got, want) {
			t.Fatalf("missing %q\nstdout:\n%s\nstderr:\n%s", want, got, errText)
		}
	}
	for _, want := range []string{
		"expects str, got int",
		"expects str, got null",
		"expects 1",
		"expected str, got int",
		"unwrap on err: cannot open 'missing.csv'",
	} {
		if !strings.Contains(errText, want) {
			t.Fatalf("missing %q\nstderr:\n%s", want, errText)
		}
	}
	if strings.Count(got, "21\n") < 2 {
		t.Fatalf("the failed read dropped the table\nstdout:\n%s\nstderr:\n%s", got, errText)
	}
}

func TestRepl_ListMutationRollsBack(t *testing.T) {
	cmd := exec.Command(quarkExePath, "repl")
	cmd.Stdin = strings.NewReader(strings.Join([]string{
		"xs = list [1]",
		"xs",
		"if true:",
		"    xs.push(9)",
		"    print(1 / 0)",
		"",
		"xs",
		"",
	}, "\n"))
	var out, errb bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errb
	if err := cmd.Run(); err != nil {
		t.Fatalf("repl: %v\nstderr:\n%s", err, errb.String())
	}
	got := strings.ReplaceAll(out.String(), "\r\n", "\n")
	if strings.Contains(got, "9") {
		t.Fatalf("failed input kept the pushed element\nstdout:\n%s", got)
	}
	if strings.Count(got, "[1]\n") < 2 {
		t.Fatalf("stdout:\n%s\nstderr:\n%s", got, errb.String())
	}
}

// runRepl runs `quark repl` on the inputs and returns stdout and stderr.
func runRepl(t *testing.T, dir string, inputs []string) (string, string) {
	t.Helper()
	cmd := exec.Command(quarkExePath, "repl")
	cmd.Dir = dir
	cmd.Stdin = strings.NewReader(strings.Join(inputs, "\n") + "\n")
	var out, errb bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errb
	done := make(chan error, 1)
	go func() { done <- cmd.Run() }()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("repl: %v\nstderr:\n%s", err, errb.String())
		}
	case <-time.After(4 * time.Minute):
		_ = cmd.Process.Kill()
		t.Fatal("repl timed out\n" + errb.String())
	}
	return strings.ReplaceAll(out.String(), "\r\n", "\n"), errb.String()
}

// TestRepl_SessionValuesAndCompiledCode covers inputs whose compiled code
// used to read the wrong slot or fail to compile: a type change that reads
// the old value, a scalar slot read before its first assignment, an input
// after a list's element type was fixed, and an earlier input's function
// used as a value.
func TestRepl_SessionValuesAndCompiledCode(t *testing.T) {
	out, errText := runRepl(t, t.TempDir(), []string{
		"t = 'abc'",
		"t = t == 'abc'",
		"t",
		"fn g() ->",
		"    f()",
		"fn id(v) -> v",
		"h = id(g)",
		"print(h())",
		"n = 5",
		"fn f() -> n + 1",
		"",
		"xs = list []",
		"xs.push(1)",
		"fn sq(x: int) int -> x * x",
		"sq(3)",
		"fn cube(x) -> x * x * x",
		"c = cube",
		"c(2)",
	})
	if strings.Contains(errText, "internal compiler error") {
		t.Fatalf("stderr:\n%s", errText)
	}
	if want := "true\n[1]\n9\n8\n"; out != want {
		t.Fatalf("stdout:\n%s\nwant:\n%s\nstderr:\n%s", out, want, errText)
	}
	if !strings.Contains(errText, "'n' is used before it is assigned") {
		t.Fatalf("a slot read before its first assignment should be an error\nstderr:\n%s", errText)
	}
}

// TestRepl_IncludedFileReadOnce requires an included file to be read once
// per session: editing it afterwards doesn't change what the session runs.
func TestRepl_IncludedFileReadOnce(t *testing.T) {
	dir := t.TempDir()
	lib := filepath.Join(dir, "lib3.qrk")
	if err := os.WriteFile(lib, []byte("fn f(x: int) int -> x + 1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(quarkExePath, "repl")
	cmd.Dir = dir
	stdin, err := cmd.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	var out, errb lockedBuffer
	cmd.Stdout = &out
	cmd.Stderr = &errb
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = cmd.Process.Kill() }()
	if _, err := stdin.Write([]byte("incl './lib3'\nprint(lib3::f(1))\n")); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(3 * time.Minute)
	for !strings.Contains(out.String(), "2") {
		if time.Now().After(deadline) {
			t.Fatalf("no output from the first inputs\nstderr:\n%s", errb.String())
		}
		time.Sleep(100 * time.Millisecond)
	}
	if err := os.WriteFile(lib, []byte("fn f(x: int) float -> x * 1.5\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := stdin.Write([]byte("print(lib3::f(4))\n")); err != nil {
		t.Fatal(err)
	}
	_ = stdin.Close()
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	select {
	case <-done:
	case <-time.After(3 * time.Minute):
		t.Fatal("repl timed out\n" + errb.String())
	}
	if got := strings.ReplaceAll(out.String(), "\r\n", "\n"); got != "2\n5\n" {
		t.Fatalf("stdout:\n%s\nwant the file as first read (2, then 5)\nstderr:\n%s", got, errb.String())
	}
}

// lockedBuffer is a bytes.Buffer the test can read while the command writes.
type lockedBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *lockedBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *lockedBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

// TestRepl_FunctionMadeListStaysUntyped requires an empty list made inside a
// function of an earlier input to keep holding any value: the function is
// compiled, so a later write can't type what it makes, and a typed read of
// such a list is checked (specs/repl_v0_1_spec.md §3.5). A top-level empty
// list still takes its type from a later input's first write.
func TestRepl_FunctionMadeListStaysUntyped(t *testing.T) {
	out, errText := runRepl(t, t.TempDir(), []string{
		"fn mk() -> list []",
		"fn addany(l, v) -> l.push(v)",
		"zs = mk()",
		"zs.push(1)",
		"ws = mk()",
		"addany(ws, 'a')",
		"y: int = ws[0]",
		"zs.push('b')",
		"xs = list []",
		"xs.push(1)",
		"xs.push('c')",
	})
	if want := "[1]\n['a']\n[1, 'b']\n[1]\n"; out != want {
		t.Fatalf("stdout:\n%s\nwant:\n%s\nstderr:\n%s", out, want, errText)
	}
	if !strings.Contains(errText, "variable 'y' is declared int but got str") {
		t.Fatalf("a typed read of a str from an untyped list should be an error\nstderr:\n%s", errText)
	}
	if !strings.Contains(errText, "cannot add str to list[int]") {
		t.Fatalf("a top-level list typed by a later input should reject a str\nstderr:\n%s", errText)
	}
	if strings.Contains(errText, "internal compiler error") {
		t.Fatalf("stderr:\n%s", errText)
	}
}
