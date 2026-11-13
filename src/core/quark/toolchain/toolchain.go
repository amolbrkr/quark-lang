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

	"quark/diagnostics"
)

// Toolchain holds the discovered tools and runtime paths.
type Toolchain struct {
	Compiler       string // clang++
	RuntimeInclude string // runtime/include
	GCInclude      string // deps/bdwgc/include
	GCLib          string // static libgc archive

	defaultDirs []string // linker's default library directories, computed on first use
}

// Options control a single build.
type Options struct {
	LTO     bool // link-time optimization
	PCH     bool // use the cached precompiled runtime header
	Verbose bool // print every command to stderr

	// Native dependencies (extern directives, CLI flags, environment).
	IncludeDirs []string // extra -I directories, searched after the runtime's
	LibDirs     []string // library directories, searched before the linker's defaults
	Libs        []Lib    // libraries to link, in link order
	LinkArgs    []string // passed to the linker as-is
	GlueFiles   []string // extern glue headers; errors in them are reported as QK-FFI-003
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
// - Always use -O3 for release-like performance of the generated code.
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

// Compile compiles cppFile into objFile. A compile error inside an extern
// glue header is returned as a *DiagnosticError with code QK-FFI-003.
func (tc *Toolchain) Compile(cppFile, objFile string, opts Options) error {
	// The PCH is built from the runtime flags only, so it can be shared by
	// programs with different extern include paths.
	flags := tc.CompileFlags(opts)
	args := append([]string{}, flags...)
	for _, dir := range opts.IncludeDirs {
		args = append(args, "-I"+dir)
	}
	args = append(args, "-fcolor-diagnostics", "-ferror-limit=20")
	files := []string{"-c", cppFile, "-o", objFile}
	if !opts.PCH {
		return tc.compileOnce(opts, append(args, files...))
	}

	pch, err := tc.precompiledHeader(flags, opts)
	if err != nil {
		// A missing PCH only costs time, so build without it.
		if opts.Verbose {
			fmt.Fprintf(os.Stderr, "Debug: precompiled header unavailable: %s\n", err)
		}
		return tc.compileOnce(opts, append(args, files...))
	}
	withPCH := append(append(append([]string{}, args...), "-include-pch", pch), files...)
	out, err := tc.runCaptured(opts, withPCH)
	if err != nil && isPCHError(out) {
		// clang rejected the cached PCH; drop it and build without one.
		if opts.Verbose {
			fmt.Fprintf(os.Stderr, "Debug: discarding stale precompiled header %s\n", pch)
		}
		os.Remove(pch)
		return tc.compileOnce(opts, append(args, files...))
	}
	return compileResult(out, err, opts)
}

// compileOnce runs one compile and reports its result like Compile.
func (tc *Toolchain) compileOnce(opts Options, args []string) error {
	out, err := tc.runCaptured(opts, args)
	return compileResult(out, err, opts)
}

// compileResult prints clang's output and turns an error located in glue
// code into a QK-FFI-003 diagnostic.
func compileResult(out string, err error, opts Options) error {
	os.Stderr.WriteString(out)
	if err == nil {
		return nil
	}
	if d := glueErrorDiagnostic(out, opts.GlueFiles); d != nil {
		return &DiagnosticError{Diags: []diagnostics.Diagnostic{*d}}
	}
	return err
}

// Link links objFiles with the native libraries in opts and the runtime
// libraries into exe. Libraries that can't be found are reported as
// QK-FFI-002 diagnostics before the linker runs.
func (tc *Toolchain) Link(objFiles []string, exe string, opts Options) error {
	libs, err := tc.resolveLibs(opts)
	if err != nil {
		return err
	}
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
	args = append(args, nativeLinkArgs(libs, opts)...)
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
		// The repository commits only the GC headers and prebuilt library
		// (see deps/bdwgc/VENDORED.md), so look for the headers, not the
		// CMake project.
		if _, err := os.Stat(filepath.Join(abs, "include", "gc", "gc.h")); err == nil {
			return abs, nil
		}
	}

	return "", errors.New("could not locate deps/bdwgc (expected the vendored Boehm GC headers in the repository)")
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

// prebuiltGCDir is where the repository keeps the static GC library built for
// the host platform, e.g. deps/bdwgc/lib/linux-amd64 (see deps/bdwgc/VENDORED.md).
func prebuiltGCDir(gcDir string) string {
	return filepath.Join(gcDir, "lib", runtime.GOOS+"-"+runtime.GOARCH)
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

// EnsureGC returns the GC include directory and static library: the prebuilt
// library for the host platform if the repository has one, else a local build
// in build-static, which it bootstraps with CMake from the GC source if needed.
func EnsureGC() (includePath string, libPath string, err error) {
	gcSourceDir, err := findGCSourceDir()
	if err != nil {
		return "", "", err
	}

	includePath = filepath.Join(gcSourceDir, "include")
	if _, statErr := os.Stat(filepath.Join(includePath, "gc", "gc.h")); statErr != nil {
		return "", "", fmt.Errorf("Boehm GC headers not found at %s", includePath)
	}

	prebuilt := prebuiltGCDir(gcSourceDir)
	if libPath, err = findGCLibrary(prebuilt); err == nil {
		return includePath, libPath, nil
	}
	buildDir := filepath.Join(gcSourceDir, gcBuildDirName)
	if libPath, err = findGCLibrary(buildDir); err == nil {
		return includePath, libPath, nil
	}

	if _, statErr := os.Stat(filepath.Join(gcSourceDir, "CMakeLists.txt")); statErr != nil {
		return "", "", fmt.Errorf("no prebuilt Boehm GC library for %s/%s (expected in %s) and no GC source to build one; see deps/bdwgc/VENDORED.md", runtime.GOOS, runtime.GOARCH, prebuilt)
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
