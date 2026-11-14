package toolchain

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func repoPaths(t *testing.T) (runtimeInclude, gcDir, quarkDir string) {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller")
	}
	quarkDir = filepath.Dir(filepath.Dir(file))
	runtimeInclude = filepath.Join(quarkDir, "runtime", "include")
	gcDir = filepath.Join(quarkDir, "..", "..", "..", "deps", "bdwgc")
	return runtimeInclude, gcDir, quarkDir
}

func TestRuntimeIncludePath_Env(t *testing.T) {
	inc, _, _ := repoPaths(t)
	t.Setenv("QUARK_RUNTIME", inc)
	got, err := RuntimeIncludePath()
	if err != nil {
		t.Fatal(err)
	}
	want, err := filepath.Abs(inc)
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("got %s, want %s", got, want)
	}
}

func TestRuntimeIncludePath_EnvNotADirectory(t *testing.T) {
	t.Setenv("QUARK_RUNTIME", filepath.Join(t.TempDir(), "missing"))
	_, err := RuntimeIncludePath()
	if err == nil || !strings.Contains(err.Error(), "which is not a directory") {
		t.Fatalf("got %v", err)
	}
}

func TestRuntimeIncludePath_EnvMissingHeader(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("QUARK_RUNTIME", dir)
	_, err := RuntimeIncludePath()
	if err == nil || !strings.Contains(err.Error(), "does not contain quark/quark.hpp") {
		t.Fatalf("got %v", err)
	}
}

func TestRuntimeIncludePath_IgnoresWorkingDirectory(t *testing.T) {
	_, _, quarkDir := repoPaths(t)
	t.Setenv("QUARK_RUNTIME", "")
	old, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(quarkDir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(old) })
	got, err := RuntimeIncludePath()
	if err == nil {
		exe, _ := os.Executable()
		beside := filepath.Join(filepath.Dir(exe), "runtime", "include", "quark", "quark.hpp")
		if _, statErr := os.Stat(beside); statErr != nil {
			t.Fatalf("found %s from the working directory", got)
		}
		t.Skip("this test binary sits next to runtime/include")
	}
	if !strings.Contains(err.Error(), "QUARK_RUNTIME") {
		t.Fatal(err)
	}
}

func TestFindGCSourceDir_Env(t *testing.T) {
	_, gcDir, _ := repoPaths(t)
	t.Setenv("QUARK_GC", gcDir)
	got, err := findGCSourceDir()
	if err != nil {
		t.Fatal(err)
	}
	header := filepath.Join(got, "include", "gc", "gc.h")
	if _, err := os.Stat(header); err != nil {
		t.Fatal(err)
	}
}

func TestFindGCSourceDir_EnvNotADirectory(t *testing.T) {
	t.Setenv("QUARK_GC", filepath.Join(t.TempDir(), "missing"))
	_, err := findGCSourceDir()
	if err == nil || !strings.Contains(err.Error(), "which is not a directory") {
		t.Fatalf("got %v", err)
	}
}

func TestFindGCSourceDir_IgnoresWorkingDirectory(t *testing.T) {
	_, _, quarkDir := repoPaths(t)
	repo := filepath.Join(quarkDir, "..", "..", "..")
	t.Setenv("QUARK_GC", "")
	old, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(repo); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(old) })
	_, err = findGCSourceDir()
	if err == nil {
		exe, _ := os.Executable()
		exeDir := filepath.Dir(exe)
		beside := []string{
			filepath.Join(exeDir, "..", "..", "..", "deps", "bdwgc", "include", "gc", "gc.h"),
			filepath.Join(exeDir, "deps", "bdwgc", "include", "gc", "gc.h"),
		}
		for _, p := range beside {
			if _, statErr := os.Stat(p); statErr == nil {
				t.Skip("this test binary sits next to deps/bdwgc")
			}
		}
		t.Fatal("found the GC from the working directory")
	}
	if !strings.Contains(err.Error(), "QUARK_GC") {
		t.Fatal(err)
	}
}
