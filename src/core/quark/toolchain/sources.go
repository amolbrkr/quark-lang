package toolchain

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"quark/diagnostics"
)

// externObjectCacheDir is where compiled glue sources are cached
// (specs/ffi_v0_1_spec.md P2): ~/.cache/quark/obj/.
func externObjectCacheDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".cache", "quark", "obj"), nil
}

// CompileExternSources compiles each glue source as its own translation unit,
// with the same flags and precompiled header as the program, and returns the
// object files. An object already cached for this source, the headers it
// includes, the compile flags and the runtime header state is reused.
func (tc *Toolchain) CompileExternSources(sources []string, opts Options) ([]string, error) {
	if len(sources) == 0 {
		return nil, nil
	}
	var objs []string
	for _, src := range sources {
		obj, err := tc.compileExternSource(src, opts)
		if err != nil {
			return nil, err
		}
		objs = append(objs, obj)
	}
	return objs, nil
}

func (tc *Toolchain) compileExternSource(src string, opts Options) (string, error) {
	src = filepath.Clean(src)
	key, err := tc.externObjectKey(src, opts)
	if err != nil {
		return "", err
	}
	dir, err := externObjectCacheDir()
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	obj := filepath.Join(dir, key+".o")
	if st, err := os.Stat(obj); err == nil && !st.IsDir() && st.Size() > 0 {
		if opts.Verbose {
			fmt.Fprintf(os.Stderr, "Debug: extern source cache hit: %s -> %s\n", src, obj)
		}
		return obj, nil
	}
	if opts.Verbose {
		fmt.Fprintf(os.Stderr, "Debug: compiling extern source: %s\n", src)
	}
	tmp, err := os.CreateTemp(dir, key+".*.o")
	if err != nil {
		return "", err
	}
	tmpPath := tmp.Name()
	tmp.Close()
	if err := tc.Compile(src, tmpPath, opts); err != nil {
		os.Remove(tmpPath)
		return "", err
	}
	if err := os.Rename(tmpPath, obj); err != nil {
		if st, statErr := os.Stat(obj); statErr == nil && !st.IsDir() && st.Size() > 0 {
			os.Remove(tmpPath)
			return obj, nil
		}
		os.Remove(tmpPath)
		return "", err
	}
	return obj, nil
}

// externObjectKey hashes the compiler, the compile flags, the include
// directories, the precompiled-header key (the runtime and GC header state),
// the source bytes, and the path, size and mtime of every header the source
// includes.
func (tc *Toolchain) externObjectKey(src string, opts Options) (string, error) {
	flags := tc.CompileFlags(opts)
	deps, err := tc.externDeps(src, flags, opts)
	if err != nil {
		return "", err
	}
	h := sha256.New()
	compiler := tc.Compiler
	if resolved, err := filepath.EvalSymlinks(tc.Compiler); err == nil {
		compiler = resolved
	}
	if err := hashFileStat(h, compiler); err != nil {
		return "", err
	}
	fmt.Fprintf(h, "flags:%s\n", strings.Join(flags, "\x00"))
	fmt.Fprintf(h, "includes:%s\n", strings.Join(opts.IncludeDirs, "\x00"))
	pchID := "nopch"
	if opts.PCH {
		if key, err := tc.pchKey(flags); err == nil {
			pchID = key
		}
	}
	fmt.Fprintf(h, "pch:%s\n", pchID)
	if err := hashFileBytes(h, src); err != nil {
		return "", err
	}
	for _, dep := range deps {
		if err := hashFileStat(h, dep); err != nil {
			return "", err
		}
	}
	return hex.EncodeToString(h.Sum(nil))[:24], nil
}

func hashFileBytes(h io.Writer, path string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = io.Copy(h, f)
	return err
}

// externDeps asks clang which files the source includes (-M). A compile
// error in the source or a header it includes is a QK-FFI-003 diagnostic.
func (tc *Toolchain) externDeps(src string, flags []string, opts Options) ([]string, error) {
	depFile, err := os.CreateTemp("", "quark-deps-*.d")
	if err != nil {
		return nil, err
	}
	depPath := depFile.Name()
	depFile.Close()
	defer os.Remove(depPath)

	args := append([]string{}, flags...)
	for _, dir := range opts.IncludeDirs {
		args = append(args, "-I"+dir)
	}
	args = append(args, "-M", "-MF", depPath, src)
	out, err := tc.runCaptured(opts, args)
	if err != nil {
		os.Stderr.WriteString(out)
		glue := append(append([]string{}, opts.GlueFiles...), src)
		if d := glueErrorDiagnostic(out, glue); d != nil {
			return nil, &DiagnosticError{Diags: []diagnostics.Diagnostic{*d}}
		}
		return nil, fmt.Errorf("reading dependencies of %s: %v\n%s", src, err, out)
	}
	text, err := os.ReadFile(depPath)
	if err != nil {
		return nil, err
	}
	var deps []string
	seen := map[string]bool{}
	for _, dep := range parseMakeDeps(string(text)) {
		if !filepath.IsAbs(dep) {
			dep = filepath.Join(filepath.Dir(src), dep)
		}
		dep = filepath.Clean(dep)
		if seen[dep] {
			continue
		}
		seen[dep] = true
		deps = append(deps, dep)
	}
	if len(deps) == 0 {
		return nil, fmt.Errorf("no dependencies reported for %s:\n%s", src, text)
	}
	return deps, nil
}

// parseMakeDeps reads the dependency list clang -M writes. The first field
// is the make target (`file.o:`); the rest are the source and the headers it
// includes. A path that contains a space is quoted, or written with the
// space escaped (`Program\ Files`), which is what clang prints on Windows.
// A trailing backslash continues the line.
func parseMakeDeps(text string) []string {
	var fields []string
	var b strings.Builder
	inQuote := false
	for i := 0; i < len(text); i++ {
		c := text[i]
		if !inQuote && c == '\\' && i+1 < len(text) {
			next := text[i+1]
			switch next {
			case ' ', '\t':
				b.WriteByte(next)
				i++
				continue
			case '\n':
				i++
				continue
			case '\r':
				i++
				if i+1 < len(text) && text[i+1] == '\n' {
					i++
				}
				continue
			}
		}
		switch {
		case inQuote && c == '"':
			inQuote = false
		case inQuote:
			b.WriteByte(c)
		case c == '"':
			inQuote = true
		case c == ' ' || c == '\t' || c == '\n' || c == '\r':
			if b.Len() > 0 {
				fields = append(fields, b.String())
				b.Reset()
			}
		default:
			b.WriteByte(c)
		}
	}
	if b.Len() > 0 {
		fields = append(fields, b.String())
	}
	var deps []string
	for i, f := range fields {
		if f == ":" {
			continue
		}
		if i == 0 && strings.HasSuffix(f, ":") {
			continue
		}
		deps = append(deps, f)
	}
	return deps
}
