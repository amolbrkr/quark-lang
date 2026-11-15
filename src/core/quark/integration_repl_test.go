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
