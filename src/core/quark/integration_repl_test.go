package main

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
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
