package toolchain

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

// REPL host and input libraries (specs/implemented/repl_v0_1_spec.md §5.2). The runtime
// is compiled once into the host at -O3. Each input is a shared library
// compiled at -O0 against that runtime.

// replPCHFlags are the flags of the input precompiled header. The header is
// -O0 so an input compiles quickly; -fpch-codegen emits the runtime's inline
// functions into one object the host links, instead of into every input.
func (tc *Toolchain) replPCHFlags() []string {
	args := []string{"-std=c++17", "-O0", "-fPIC", "-fpch-instantiate-templates", "-fpch-codegen"}
	if runtime.GOARCH == "amd64" {
		args = append(args, "-march=x86-64-v3")
	}
	args = append(args, "-DQUARK_USE_GC", "-I"+tc.RuntimeInclude, "-I"+tc.GCInclude)
	return args
}

// HostPath is the cached REPL host binary, built on first use. On Windows it
// also builds quarkrepl.dll, which owns the one GC and the session slots;
// input DLLs link against it.
func (tc *Toolchain) HostPath(verbose bool) (string, string, error) {
	if runtime.GOOS == "windows" {
		return tc.hostPathWindows(verbose)
	}
	flags := tc.replPCHFlags()
	key, err := tc.pchKey(flags)
	if err != nil {
		return "", "", err
	}
	hostSrc := filepath.Join(filepath.Dir(tc.RuntimeInclude), "repl", "host.cpp")
	slotsSrc := filepath.Join(filepath.Dir(tc.RuntimeInclude), "repl", "slots.cpp")
	stateHdr := filepath.Join(tc.RuntimeInclude, "quark", "repl", "state.hpp")
	extra, err := hashFiles(hostSrc, slotsSrc, stateHdr)
	if err != nil {
		return "", "", err
	}
	key = key + "-" + extra

	cache, err := os.UserCacheDir()
	if err != nil {
		return "", "", err
	}
	dir := filepath.Join(cache, "quark", "repl", key)
	host := filepath.Join(dir, "host")
	pch := filepath.Join(dir, "runtime.pch")
	if _, err := os.Stat(host); err == nil {
		if _, err := os.Stat(pch); err == nil {
			return host, pch, nil
		}
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", "", err
	}
	fmt.Fprintln(os.Stderr, "Building the REPL runtime (about 10 seconds)...")
	opts := Options{Verbose: verbose}
	if err := tc.buildFile(opts, append(flags, "-x", "c++-header", filepath.Join(tc.RuntimeInclude, "quark", "quark.hpp"), "-o", pch)); err != nil {
		return "", "", err
	}
	obj := filepath.Join(dir, "runtime.o")
	objFlags := append([]string{}, flags...)
	for i, f := range objFlags {
		if f == "-O0" {
			objFlags[i] = "-O3"
		}
	}
	err = tc.buildFile(opts, append(objFlags, "-c", pch, "-o", obj))
	if err != nil {
		// The PCH was built at -O0. Some clang versions refuse to codegen it
		// at -O3; the object is then the -O0 copy, and inputs still call it.
		if err2 := tc.buildFile(opts, append(flags, "-c", pch, "-o", obj)); err2 != nil {
			return "", "", err
		}
	}
	link := []string{"-fPIC", "-rdynamic", hostSrc, slotsSrc, obj, "-o", host}
	link = append(link, "-I"+tc.RuntimeInclude, "-I"+tc.GCInclude, "-DQUARK_USE_GC", "-std=c++17", "-O3")
	link = append(link, GCLinkArgs(tc.GCLib)...)
	link = append(link, "-lm")
	if err := tc.buildFile(opts, link); err != nil {
		return "", "", err
	}
	return host, pch, nil
}

// CompileInput compiles one REPL input into a shared library against pch.
func (tc *Toolchain) CompileInput(cppPath, soPath, pch string, opts Options) error {
	obj := strings.TrimSuffix(soPath, filepath.Ext(soPath)) + ".o"
	var args []string
	if runtime.GOOS == "windows" {
		// Same flags as the input precompiled header, or clang rejects it.
		args = append([]string{}, tc.replWindowsPCHFlags()...)
	} else {
		args = []string{"-std=c++17", "-O0", "-DQUARK_USE_GC", "-I" + tc.RuntimeInclude, "-I" + tc.GCInclude}
		if runtime.GOARCH == "amd64" {
			args = append(args, "-march=x86-64-v3")
		}
		args = append(args, "-fPIC")
	}
	for _, dir := range opts.IncludeDirs {
		args = append(args, "-I"+dir)
	}
	if pch != "" {
		args = append(args, "-include-pch", pch)
	}
	args = append(args, "-c", cppPath, "-o", obj)
	if err := tc.buildFile(opts, args); err != nil {
		return err
	}
	libs, err := tc.resolveLibs(opts)
	if err != nil {
		return err
	}
	link := []string{"-shared"}
	if runtime.GOOS != "windows" {
		link = append(link, "-fPIC")
	}
	link = append(link, obj)
	if runtime.GOOS == "windows" {
		// Earlier inputs export the functions this one calls. The runtime
		// DLL comes last: those imports are resolved there.
		link = append(link, opts.PrevLibs...)
		link = append(link, "-Wl,/IMPLIB:"+strings.TrimSuffix(soPath, filepath.Ext(soPath))+".lib")
		link = append(link, tc.replLib)
		link = append(link, replWindowsCRTLink...)
	}
	link = append(link, "-o", soPath)
	link = append(link, nativeLinkArgs(libs, opts)...)
	return tc.buildFile(opts, link)
}

func (tc *Toolchain) buildFile(opts Options, args []string) error {
	out, err := tc.runCaptured(opts, args)
	if err != nil {
		if strings.TrimSpace(out) != "" {
			return fmt.Errorf("%s\n%s", out, err)
		}
		return err
	}
	return nil
}

func hashFiles(paths ...string) (string, error) {
	h := sha256.New()
	for _, p := range paths {
		info, err := os.Stat(p)
		if err != nil {
			return "", err
		}
		fmt.Fprintf(h, "%s:%d:%d\n", p, info.Size(), info.ModTime().UnixNano())
	}
	return hex.EncodeToString(h.Sum(nil))[:12], nil
}

func (tc *Toolchain) hostPathWindows(verbose bool) (string, string, error) {
	hostSrc := filepath.Join(filepath.Dir(tc.RuntimeInclude), "repl", "host_win.cpp")
	slotsSrc := filepath.Join(filepath.Dir(tc.RuntimeInclude), "repl", "slots.cpp")
	stateHdr := filepath.Join(tc.RuntimeInclude, "quark", "repl", "state.hpp")
	diag := filepath.Join(tc.RuntimeInclude, "quark", "core", "diagnostics.hpp")
	extra, err := hashFiles(hostSrc, slotsSrc, stateHdr, diag, tc.GCLib)
	if err != nil {
		return "", "", err
	}
	flags := tc.replWindowsPCHFlags()
	key, err := tc.pchKey(flags)
	if err != nil {
		return "", "", err
	}
	key = key + "-" + extra
	cache, err := os.UserCacheDir()
	if err != nil {
		return "", "", err
	}
	dir := filepath.Join(cache, "quark", "repl", key)
	host := filepath.Join(dir, "host.exe")
	dll := filepath.Join(dir, "quarkrepl.dll")
	lib := filepath.Join(dir, "quarkrepl.lib")
	pch := filepath.Join(dir, "runtime.pch")
	if _, err := os.Stat(host); err == nil {
		if _, err := os.Stat(dll); err == nil {
			if _, err := os.Stat(pch); err == nil {
				tc.replLib = lib
				return host, pch, nil
			}
		}
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", "", err
	}
	fmt.Fprintln(os.Stderr, "Building the REPL runtime (once, about a minute)...")
	opts := Options{Verbose: verbose}
	header := filepath.Join(tc.RuntimeInclude, "quark", "quark.hpp")
	// The input PCH references runtime functions instead of compiling them.
	// A second PCH, built with QUARK_REPL_BUILD, is compiled into the DLL so
	// those functions call q_exit_hook directly rather than through an import.
	if err := tc.buildFile(opts, append(flags, "-x", "c++-header", header, "-o", pch)); err != nil {
		return "", "", err
	}
	buildFlags := append([]string{}, flags...)
	buildFlags = append(buildFlags, "-DQUARK_REPL_BUILD")
	buildPCH := filepath.Join(dir, "runtime-build.pch")
	if err := tc.buildFile(opts, append(buildFlags, "-x", "c++-header", header, "-o", buildPCH)); err != nil {
		return "", "", err
	}
	objFlags := append([]string{}, buildFlags...)
	for i, f := range objFlags {
		if f == "-O0" {
			objFlags[i] = "-O3"
		}
	}
	obj := filepath.Join(dir, "runtime.obj")
	objArgs := append(objFlags, "-Wno-unused-command-line-argument", "-c", buildPCH, "-o", obj)
	if err := tc.buildFile(opts, objArgs); err != nil {
		// The PCH was built at -O0. Some clang versions refuse to codegen it
		// at -O3; the object is then the -O0 copy, and inputs still call it.
		for i, f := range objArgs {
			if f == "-O3" {
				objArgs[i] = "-O0"
			}
		}
		if err2 := tc.buildFile(opts, objArgs); err2 != nil {
			return "", "", err
		}
	}
	def, err := tc.replDefFile(dir, obj)
	if err != nil {
		return "", "", err
	}
	dllArgs := []string{"-shared", "-std=c++17", "-O3", replWindowsCRT, "-DQUARK_USE_GC", "-DQUARK_REPL", "-DQUARK_REPL_BUILD", "-fms-extensions",
		"-Wno-deprecated-declarations", "-I" + tc.RuntimeInclude, "-I" + tc.GCInclude,
		slotsSrc, obj, tc.GCLib, "-Wl,/DEF:" + def, "-o", dll}
	dllArgs = append(dllArgs, replWindowsCRTLink...)
	if err := tc.buildFile(opts, dllArgs); err != nil {
		return "", "", err
	}
	hostArgs := []string{"-std=c++17", "-O3", replWindowsCRT, "-DQUARK_USE_GC", "-DQUARK_REPL", "-fms-extensions",
		"-Wno-deprecated-declarations", "-I" + tc.RuntimeInclude, "-I" + tc.GCInclude, hostSrc, lib, "-o", host}
	hostArgs = append(hostArgs, replWindowsCRTLink...)
	if err := tc.buildFile(opts, hostArgs); err != nil {
		return "", "", err
	}
	tc.replLib = lib
	return host, pch, nil
}

// The Windows REPL's host, quarkrepl.dll and every input use the DLL C
// runtime (ucrtbase, vcruntime140, msvcp140), so the process has one: one
// stdout buffer, heap, environment and file descriptor table. With clang's
// default static runtime each DLL had its own copy, so the host couldn't
// flush what print wrote, and an input whose glue pulled in a static runtime
// object (std::thread::hardware_concurrency, _putenv_s) got a second
// definition of inline code quarkrepl.dll exports (std::string's
// constructor, snprintf): LNK2005. quark run and quark build keep the
// static runtime.
const replWindowsCRT = "-fms-runtime-lib=dll"

// replWindowsCRTLink goes on every Windows REPL link. clang's driver names
// libcmt (the static runtime) on every link even with -fms-runtime-lib=dll,
// and the prebuilt GC library names it too; with msvcrt also named, both
// runtimes would be linked.
var replWindowsCRTLink = []string{"-Wl,/NODEFAULTLIB:libcmt.lib"}

// replWindowsPCHFlags are the flags of the precompiled header each input
// compiles against. They match replPCHFlags on the codegen options, and add
// the Windows REPL macros and runtime. -DQUARK_REPL_BUILD is not one of them:
// an input imports the session, it does not define it.
func (tc *Toolchain) replWindowsPCHFlags() []string {
	args := []string{"-std=c++17", "-O0", replWindowsCRT, "-DQUARK_USE_GC", "-DQUARK_REPL", "-fms-extensions",
		"-Wno-deprecated-declarations", "-fpch-instantiate-templates", "-fpch-codegen",
		"-I" + tc.RuntimeInclude, "-I" + tc.GCInclude}
	if runtime.GOARCH == "amd64" {
		args = append(args, "-march=x86-64-v3")
	}
	return args
}

// replDefFile writes the exports of quarkrepl.dll: the GC, so every input
// allocates from the host's collector, and the runtime functions compiled
// into runtimeObj, so an input calls those instead of compiling them.
func (tc *Toolchain) replDefFile(dir, runtimeObj string) (string, error) {
	var b strings.Builder
	b.WriteString("LIBRARY quarkrepl\nEXPORTS\n")
	if err := writeGCExports(&b, tc.GCLib); err != nil {
		return "", err
	}
	if err := writeRuntimeExports(&b, runtimeObj); err != nil {
		return "", err
	}
	path := filepath.Join(dir, "quarkrepl.def")
	if err := os.WriteFile(path, []byte(b.String()), 0o644); err != nil {
		return "", err
	}
	return path, nil
}

// writeGCExports appends the GC's public symbols. A static library does not
// export them from the DLL on its own.
func writeGCExports(b *strings.Builder, gcLib string) error {
	nm, err := exec.LookPath("llvm-nm")
	if err != nil {
		return fmt.Errorf("llvm-nm is required to build the REPL runtime: %s", err)
	}
	out, err := exec.Command(nm, "--defined-only", gcLib).Output()
	if err != nil {
		return fmt.Errorf("reading %s: %s", gcLib, err)
	}
	seen := map[string]bool{}
	for _, line := range strings.Split(string(out), "\n") {
		kind, name, ok := nmSymbol(line)
		if !ok || seen[name] || !strings.HasPrefix(name, "GC_") {
			continue
		}
		if kind != "T" && kind != "D" && kind != "B" && kind != "C" && kind != "R" {
			continue
		}
		seen[name] = true
		if kind == "T" {
			fmt.Fprintf(b, "  %s\n", name)
		} else {
			fmt.Fprintf(b, "  %s DATA\n", name)
		}
	}
	return nil
}

// writeRuntimeExports appends the globals compiled into the runtime object.
// The input precompiled header is built with -fpch-codegen, so an input
// calls those functions (q_print, the QSourceLocGuard constructor, the
// std::vector methods the runtime instantiates) instead of compiling them.
// Destructor thunks and thread-local guards that llvm-nm lists are not
// symbols the linker can export; naming them in the .def file fails the link.
func writeRuntimeExports(b *strings.Builder, obj string) error {
	nm, err := exec.LookPath("llvm-nm")
	if err != nil {
		return fmt.Errorf("llvm-nm is required to build the REPL runtime: %s", err)
	}
	out, err := exec.Command(nm, "--defined-only", obj).Output()
	if err != nil {
		return fmt.Errorf("reading %s: %s", obj, err)
	}
	seen := map[string]bool{}
	for _, line := range strings.Split(string(out), "\n") {
		kind, name, ok := nmSymbol(line)
		if !ok || seen[name] || !runtimeExport(name, kind) {
			continue
		}
		seen[name] = true
		if kind == "T" {
			fmt.Fprintf(b, "  %s\n", name)
		} else {
			fmt.Fprintf(b, "  %s DATA\n", name)
		}
	}
	return nil
}

func nmSymbol(line string) (kind, name string, ok bool) {
	f := strings.Fields(line)
	if len(f) < 3 {
		return "", "", false
	}
	return f[len(f)-2], f[len(f)-1], true
}

func runtimeExport(name, kind string) bool {
	if kind != "T" && kind != "D" && kind != "B" && kind != "R" {
		return false
	}
	// llvm-nm prints these as defined, but they are local labels. A .def
	// entry for one of them is an undefined symbol at link time.
	if strings.Contains(name, "dtor") || strings.Contains(name, "TSS") || strings.Contains(name, "catch$") {
		return false
	}
	return true
}
