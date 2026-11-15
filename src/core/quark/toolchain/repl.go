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

// REPL host and input libraries (specs/repl_v0_1_spec.md §5.2). The runtime
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
	args := []string{"-std=c++17", "-O0", "-DQUARK_USE_GC", "-I" + tc.RuntimeInclude, "-I" + tc.GCInclude}
	if runtime.GOARCH == "amd64" {
		args = append(args, "-march=x86-64-v3")
	}
	if runtime.GOOS == "windows" {
		args = append(args, "-DQUARK_REPL", "-fms-extensions")
	} else {
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
	link = append(link, obj, "-o", soPath)
	if runtime.GOOS == "windows" {
		link = append(link, tc.replLib)
	}
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
	flags := []string{"-std=c++17", "-O0", "-DQUARK_USE_GC", "-DQUARK_REPL", "-fms-extensions", "-I" + tc.RuntimeInclude, "-I" + tc.GCInclude}
	if runtime.GOARCH == "amd64" {
		flags = append(flags, "-march=x86-64-v3")
	}
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
	fmt.Fprintln(os.Stderr, "Building the REPL runtime (about 10 seconds)...")
	opts := Options{Verbose: verbose}
	def, err := gcDefFile(dir, tc.GCLib)
	if err != nil {
		return "", "", err
	}
	dllArgs := []string{"-shared", "-std=c++17", "-O3", "-DQUARK_USE_GC", "-DQUARK_REPL", "-DQUARK_REPL_BUILD", "-fms-extensions",
		"-I" + tc.RuntimeInclude, "-I" + tc.GCInclude, slotsSrc, tc.GCLib, "-Wl,/DEF:" + def, "-o", dll}
	if err := tc.buildFile(opts, dllArgs); err != nil {
		return "", "", err
	}
	hostArgs := []string{"-std=c++17", "-O3", "-DQUARK_USE_GC", "-DQUARK_REPL", "-fms-extensions",
		"-I" + tc.RuntimeInclude, "-I" + tc.GCInclude, hostSrc, lib, "-o", host}
	if err := tc.buildFile(opts, hostArgs); err != nil {
		return "", "", err
	}
	if err := tc.buildFile(opts, append(flags, "-x", "c++-header", filepath.Join(tc.RuntimeInclude, "quark", "quark.hpp"), "-o", pch)); err != nil {
		return "", "", err
	}
	tc.replLib = lib
	return host, pch, nil
}

// gcDefFile writes a module definition that re-exports the GC, so an input
// DLL allocates from the same collector as the host.
func gcDefFile(dir, gcLib string) (string, error) {
	nm, err := exec.LookPath("llvm-nm")
	if err != nil {
		return "", fmt.Errorf("llvm-nm is required to build the REPL runtime: %s", err)
	}
	out, err := exec.Command(nm, "--defined-only", gcLib).Output()
	if err != nil {
		return "", fmt.Errorf("reading %s: %s", gcLib, err)
	}
	var b strings.Builder
	b.WriteString("LIBRARY quarkrepl\nEXPORTS\n")
	seen := map[string]bool{}
	for _, line := range strings.Split(string(out), "\n") {
		f := strings.Fields(line)
		if len(f) < 3 {
			continue
		}
		kind, name := f[len(f)-2], f[len(f)-1]
		if kind != "T" && kind != "D" && kind != "B" && kind != "C" && kind != "R" {
			continue
		}
		if !strings.HasPrefix(name, "GC_") || seen[name] {
			continue
		}
		seen[name] = true
		if kind == "T" {
			fmt.Fprintf(&b, "  %s\n", name)
		} else {
			fmt.Fprintf(&b, "  %s DATA\n", name)
		}
	}
	path := filepath.Join(dir, "quarkrepl.def")
	if err := os.WriteFile(path, []byte(b.String()), 0o644); err != nil {
		return "", err
	}
	return path, nil
}
