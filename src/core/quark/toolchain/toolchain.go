// Package toolchain locates the native build tools and runtime (clang++, the
// runtime headers, the vendored Boehm GC) and turns generated C++ into an
// executable in two steps: Compile (C++ to object) and Link (objects to
// executable).
package toolchain

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

// Toolchain holds the discovered tools and runtime paths.
type Toolchain struct {
	Compiler       string // clang++
	RuntimeInclude string // runtime/include
	GCInclude      string // deps/bdwgc/include
	GCLib          string // static libgc archive
}

// Options control a single build.
type Options struct {
	LTO     bool // link-time optimization
	PCH     bool // use the cached precompiled runtime header
	Verbose bool // print every command to stderr
}

// ErrNoClang is returned by Find when clang++ is not on PATH.
var ErrNoClang = errors.New(`clang++ not found in PATH
Quark requires clang++ to compile. Install it with:
  Ubuntu/Debian: sudo apt install clang
  macOS:         brew install llvm`)

// Find locates clang++, the runtime headers, and the GC, building the GC
// with CMake on first use.
func Find() (*Toolchain, error) {
	// Quark requires clang++ — g++ is not supported due to gc_allocator
	// incompatibilities with custom-allocator string keys in unordered_map.
	compiler, err := exec.LookPath("clang++")
	if err != nil {
		return nil, ErrNoClang
	}
	gcInclude, gcLib, err := EnsureGC()
	if err != nil {
		return nil, fmt.Errorf("preparing Boehm GC: %w", err)
	}
	return &Toolchain{
		Compiler:       compiler,
		RuntimeInclude: RuntimeIncludePath(),
		GCInclude:      gcInclude,
		GCLib:          gcLib,
	}, nil
}

// ExeName appends the platform executable suffix to path if it is missing.
func ExeName(path string) string {
	if runtime.GOOS == "windows" && !strings.HasSuffix(path, ".exe") {
		return path + ".exe"
	}
	return path
}

// CompileFlags returns the clang++ flags for compiling generated C++.
//
// Optimization policy:
// - Always use -O3 for release-like codegen performance.
// - Keep x86-64-v3 tuning on amd64 as currently intended by the project.
//
// Diagnostics policy:
// - Force colored diagnostics for readability.
// - Raise error limit so generated C++ failures don't truncate too early.
// - Suppress deprecated-declaration noise only on Windows (MSVC CRT shims).
func (tc *Toolchain) CompileFlags(opts Options) []string {
	args := []string{"-std=c++17", "-O3"}
	if runtime.GOARCH == "amd64" {
		args = append(args, "-march=x86-64-v3")
	}
	args = append(args,
		"-DQUARK_USE_GC",
		"-I"+tc.RuntimeInclude,
		"-I"+tc.GCInclude,
	)
	if runtime.GOOS == "windows" {
		args = append(args, "-Wno-deprecated-declarations")
	}
	if opts.LTO {
		args = append(args, "-flto")
	}
	return args
}

// Compile compiles cppFile into objFile.
func (tc *Toolchain) Compile(cppFile, objFile string, opts Options) error {
	flags := tc.CompileFlags(opts)
	args := append([]string{}, flags...)
	args = append(args, "-fcolor-diagnostics", "-ferror-limit=20")
	files := []string{"-c", cppFile, "-o", objFile}
	if !opts.PCH {
		return tc.run(opts, append(args, files...))
	}

	pch, err := tc.precompiledHeader(flags, opts)
	if err != nil {
		// A missing PCH only costs time, so build without it.
		if opts.Verbose {
			fmt.Fprintf(os.Stderr, "Debug: precompiled header unavailable: %s\n", err)
		}
		return tc.run(opts, append(args, files...))
	}
	withPCH := append(append(append([]string{}, args...), "-include-pch", pch), files...)
	out, err := tc.runCaptured(opts, withPCH)
	if err != nil && isPCHError(out) {
		// clang rejected the cached PCH; drop it and build without one.
		if opts.Verbose {
			fmt.Fprintf(os.Stderr, "Debug: discarding stale precompiled header %s\n", pch)
		}
		os.Remove(pch)
		return tc.run(opts, append(args, files...))
	}
	os.Stderr.WriteString(out)
	return err
}

// Link links objFiles with the runtime libraries into exe.
func (tc *Toolchain) Link(objFiles []string, exe string, opts Options) error {
	args := []string{}
	if opts.LTO {
		args = append(args, "-flto")
		// The system linker needs LLVM's gold plugin for LTO, which many
		// LLVM installs lack; lld handles LLVM bitcode natively.
		if _, err := exec.LookPath("ld.lld"); err == nil {
			args = append(args, "-fuse-ld=lld")
		}
	}
	args = append(args, objFiles...)
	args = append(args, "-o", exe)
	args = append(args, GCLinkArgs(tc.GCLib)...)
	if runtime.GOOS != "windows" {
		args = append(args, "-lm")
	}
	return tc.run(opts, args)
}

func (tc *Toolchain) run(opts Options, args []string) error {
	if opts.Verbose {
		fmt.Fprintf(os.Stderr, "Debug: %s %s\n", tc.Compiler, strings.Join(args, " "))
	}
	cmd := exec.Command(tc.Compiler, args...)
	cmd.Stdout = os.Stderr
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// runCaptured is run with clang's output returned instead of streamed.
func (tc *Toolchain) runCaptured(opts Options, args []string) (string, error) {
	if opts.Verbose {
		fmt.Fprintf(os.Stderr, "Debug: %s %s\n", tc.Compiler, strings.Join(args, " "))
	}
	out, err := exec.Command(tc.Compiler, args...).CombinedOutput()
	return string(out), err
}

// RuntimeIncludePath returns the runtime include directory next to the quark
// executable (src/core/quark/runtime/include), falling back to a path
// relative to the working directory.
func RuntimeIncludePath() string {
	if exePath, err := os.Executable(); err == nil {
		runtimePath := filepath.Join(filepath.Dir(exePath), "runtime", "include")
		if _, err := os.Stat(runtimePath); err == nil {
			return runtimePath
		}
	}
	if abs, err := filepath.Abs(filepath.Join("runtime", "include")); err == nil {
		return abs
	}
	return filepath.Join("runtime", "include")
}

func findGCSourceDir() (string, error) {
	candidates := []string{}

	if exePath, err := os.Executable(); err == nil {
		exeDir := filepath.Dir(exePath)
		candidates = append(candidates,
			filepath.Join(exeDir, "..", "..", "..", "deps", "bdwgc"),
			filepath.Join(exeDir, "deps", "bdwgc"),
		)
	}

	if wd, err := os.Getwd(); err == nil {
		candidates = append(candidates, filepath.Join(wd, "deps", "bdwgc"))
		for cur := wd; ; {
			parent := filepath.Dir(cur)
			if parent == cur {
				break
			}
			candidates = append(candidates, filepath.Join(parent, "deps", "bdwgc"))
			cur = parent
		}
	}

	seen := map[string]struct{}{}
	for _, candidate := range candidates {
		if candidate == "" {
			continue
		}
		abs, err := filepath.Abs(candidate)
		if err != nil {
			continue
		}
		if _, ok := seen[abs]; ok {
			continue
		}
		seen[abs] = struct{}{}
		if _, err := os.Stat(filepath.Join(abs, "CMakeLists.txt")); err == nil {
			return abs, nil
		}
	}

	return "", errors.New("could not locate deps/bdwgc (expected vendored Boehm GC source in repository)")
}

// gcBuildDirName is the CMake build directory used for the vendored Boehm GC.
// It is distinct from the conventional "build" directory so that an older
// shared-library build there is never picked up: generated binaries must link
// the GC statically so they run standalone, without needing libgc on the
// dynamic loader path.
const gcBuildDirName = "build-static"

// findGCLibrary returns the static Boehm GC archive under buildDir.
// Only static archives are accepted (see gcBuildDirName).
func findGCLibrary(buildDir string) (string, error) {
	candidates := []string{
		filepath.Join(buildDir, "libgc.a"),
		filepath.Join(buildDir, "gc.lib"),
		filepath.Join(buildDir, "libgc.lib"),
		filepath.Join(buildDir, "Release", "gc.lib"),
		filepath.Join(buildDir, "Release", "libgc.lib"),
		filepath.Join(buildDir, "Release", "libgc.a"),
		filepath.Join(buildDir, "Debug", "gc.lib"),
		filepath.Join(buildDir, "Debug", "libgc.lib"),
	}

	for _, candidate := range candidates {
		if _, err := os.Stat(candidate); err == nil {
			return candidate, nil
		}
	}

	return "", fmt.Errorf("could not find static Boehm GC library under %s", buildDir)
}

// GCLinkArgs returns the linker arguments needed to link the static GC archive.
// A static libgc depends on the platform threads library, which a shared
// libgc would otherwise have pulled in itself.
func GCLinkArgs(libPath string) []string {
	args := []string{libPath}
	if runtime.GOOS != "windows" {
		args = append(args, "-pthread")
	}
	if runtime.GOOS == "linux" {
		args = append(args, "-ldl")
	}
	return args
}

// EnsureGC returns the GC include directory and static library, building the
// vendored GC with CMake if it has not been built yet.
func EnsureGC() (includePath string, libPath string, err error) {
	gcSourceDir, err := findGCSourceDir()
	if err != nil {
		return "", "", err
	}

	includePath = filepath.Join(gcSourceDir, "include")
	if _, statErr := os.Stat(filepath.Join(includePath, "gc", "gc.h")); statErr != nil {
		return "", "", fmt.Errorf("Boehm GC headers not found at %s", includePath)
	}

	buildDir := filepath.Join(gcSourceDir, gcBuildDirName)
	if libPath, err = findGCLibrary(buildDir); err == nil {
		return includePath, libPath, nil
	}

	if _, lookErr := exec.LookPath("cmake"); lookErr != nil {
		return "", "", fmt.Errorf("Boehm GC is not built and cmake is not available in PATH; install cmake or build deps/bdwgc manually")
	}

	fmt.Fprintf(os.Stderr, "Boehm GC library not found; bootstrapping %s with CMake...\n", buildDir)

	// Always build a static GC library so generated executables are
	// standalone on every platform.
	configureArgs := []string{
		"-S", gcSourceDir,
		"-B", buildDir,
		"-DCMAKE_BUILD_TYPE=Release",
		// bdwgc reads GC_BUILD_SHARED_LIBS; BUILD_SHARED_LIBS covers
		// older bdwgc releases that used the generic CMake option.
		"-DGC_BUILD_SHARED_LIBS=OFF",
		"-DBUILD_SHARED_LIBS=OFF",
		"-DCMAKE_POSITION_INDEPENDENT_CODE=ON",
		"-Denable_docs=OFF",
		"-Dbuild_cord=OFF",
		"-DBUILD_TESTING=OFF",
	}
	if runtime.GOOS == "windows" {
		// Force clang so the GC library matches the MSVC ABI that
		// clang++ targets. Without this, cmake may pick MinGW gcc
		// which produces incompatible object files (longjmp ABI mismatch).
		configureArgs = append(configureArgs, "-DCMAKE_C_COMPILER=clang")
	}
	configureCmd := exec.Command("cmake", configureArgs...)
	configureCmd.Stdout = os.Stdout
	configureCmd.Stderr = os.Stderr
	if runErr := configureCmd.Run(); runErr != nil {
		return "", "", fmt.Errorf("failed to configure Boehm GC with CMake: %w", runErr)
	}

	// --config only matters for multi-config generators (MSVC, Xcode);
	// single-config generators ignore it.
	buildArgs := []string{"--build", buildDir, "--config", "Release"}
	buildCmd := exec.Command("cmake", buildArgs...)
	buildCmd.Stdout = os.Stdout
	buildCmd.Stderr = os.Stderr
	if runErr := buildCmd.Run(); runErr != nil {
		return "", "", fmt.Errorf("failed to build Boehm GC with CMake: %w", runErr)
	}

	libPath, err = findGCLibrary(buildDir)
	if err != nil {
		return "", "", err
	}

	return includePath, libPath, nil
}
