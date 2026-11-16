package main

import (
	"bytes"
	"os/exec"
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
