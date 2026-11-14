package toolchain

// Native dependencies requested by `extern link` / `extern include_path`
// (specs/ffi_v0_1_spec.md P1) and the build diagnostics that go with them (P8).

import (
	"bufio"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"

	"quark/diagnostics"
)

// Lib is a native library to link.
type Lib struct {
	Name   string // as in -l<name>
	Origin string // "file.qrk:line" that requested it, for diagnostics
}

// DiagnosticError is a build failure that has user-facing diagnostics.
// Callers print the diagnostics instead of a generic failure message.
type DiagnosticError struct {
	Diags []diagnostics.Diagnostic
}

func (e *DiagnosticError) Error() string {
	parts := make([]string, len(e.Diags))
	for i, d := range e.Diags {
		parts[i] = d.String()
	}
	return strings.Join(parts, "\n")
}

// resolvedLib is a library and the file the linker will use for it.
type resolvedLib struct {
	Lib
	Path   string // file found in the search directories
	Dir    string // directory Path is in
	Shared bool
}

// libFileNames returns the file names the linker accepts for -l<name>, in
// the order it prefers them within one directory.
func libFileNames(name string) []string {
	switch runtime.GOOS {
	case "darwin":
		return []string{"lib" + name + ".dylib", "lib" + name + ".tbd", "lib" + name + ".a"}
	case "windows":
		return []string{name + ".lib", "lib" + name + ".lib", "lib" + name + ".a"}
	default:
		return []string{"lib" + name + ".so", "lib" + name + ".a"}
	}
}

func isSharedLib(path string) bool {
	switch filepath.Ext(path) {
	case ".so", ".dylib", ".tbd":
		return true
	}
	return false
}

// defaultLibDirs returns the directories the linker searches without -L:
// LIBRARY_PATH entries and clang's own library search path.
func (tc *Toolchain) defaultLibDirs() []string {
	if tc.defaultDirs != nil {
		return tc.defaultDirs
	}
	dirs := filepath.SplitList(os.Getenv("LIBRARY_PATH"))
	if out, err := exec.Command(tc.Compiler, "-print-search-dirs").Output(); err == nil {
		sc := bufio.NewScanner(strings.NewReader(string(out)))
		for sc.Scan() {
			if rest, ok := strings.CutPrefix(sc.Text(), "libraries: ="); ok {
				dirs = append(dirs, filepath.SplitList(rest)...)
			}
		}
	}
	if runtime.GOOS == "darwin" {
		// System libraries live in the SDK, which ld searches via -syslibroot.
		if out, err := exec.Command("xcrun", "--show-sdk-path").Output(); err == nil {
			dirs = append(dirs, filepath.Join(strings.TrimSpace(string(out)), "usr", "lib"))
		}
	}
	tc.defaultDirs = cleanDirs(dirs)
	return tc.defaultDirs
}

// cleanDirs drops empty entries and duplicates, keeping the first.
func cleanDirs(dirs []string) []string {
	seen := map[string]bool{}
	out := []string{}
	for _, d := range dirs {
		if d == "" {
			continue
		}
		d = filepath.Clean(d)
		if !seen[d] {
			seen[d] = true
			out = append(out, d)
		}
	}
	return out
}

// resolveLibs finds the file for every library in opts.Libs, searching
// opts.LibDirs and then the linker's default directories, the way the linker
// will. Every missing library becomes a QK-FFI-002 diagnostic.
func (tc *Toolchain) resolveLibs(opts Options) ([]resolvedLib, error) {
	if len(opts.Libs) == 0 {
		return nil, nil
	}
	searched := cleanDirs(append(append([]string{}, opts.LibDirs...), tc.defaultLibDirs()...))
	var found []resolvedLib
	var diags []diagnostics.Diagnostic
	for _, lib := range opts.Libs {
		r, ok := findLib(lib, searched)
		if !ok {
			diags = append(diags, missingLibDiagnostic(lib, searched))
			continue
		}
		if opts.Verbose {
			fmt.Fprintf(os.Stderr, "Debug: library '%s' -> %s\n", lib.Name, r.Path)
		}
		found = append(found, r)
	}
	if len(diags) > 0 {
		return nil, &DiagnosticError{Diags: diags}
	}
	return found, nil
}

func findLib(lib Lib, dirs []string) (resolvedLib, bool) {
	for _, dir := range dirs {
		for _, name := range libFileNames(lib.Name) {
			path := filepath.Join(dir, name)
			if st, err := os.Stat(path); err == nil && !st.IsDir() {
				return resolvedLib{Lib: lib, Path: path, Dir: dir, Shared: isSharedLib(path)}, true
			}
		}
	}
	return resolvedLib{}, false
}

func missingLibDiagnostic(lib Lib, searched []string) diagnostics.Diagnostic {
	d := diagnostics.Diagnostic{
		Code:     "QK-FFI-002",
		Stage:    diagnostics.StageBuild,
		Severity: diagnostics.SeverityError,
		Message:  fmt.Sprintf("native library '%s' not found", lib.Name),
	}
	if file, line, ok := splitOrigin(lib.Origin); ok {
		d.Location = &diagnostics.Location{File: file, Line: line}
	}
	d.Notes = []string{
		"looked for: " + strings.Join(libFileNames(lib.Name), ", "),
		"searched: " + strings.Join(searched, ", "),
		"install the library, or pass its directory with --lib-path <dir> or QUARK_LIB_PATH",
	}
	return d
}

func splitOrigin(origin string) (string, int, bool) {
	i := strings.LastIndex(origin, ":")
	if i <= 0 {
		return "", 0, false
	}
	line, err := strconv.Atoi(origin[i+1:])
	if err != nil {
		return "", 0, false
	}
	return origin[:i], line, true
}

// nativeLinkArgs returns the linker arguments for the resolved libraries:
// -L for each user directory, an rpath for user directories that provide a
// shared library (so the binary runs without LD_LIBRARY_PATH), then -l for
// each library in link order, then opts.LinkArgs.
func nativeLinkArgs(libs []resolvedLib, opts Options) []string {
	var args []string
	userDirs := cleanDirs(opts.LibDirs)
	for _, d := range userDirs {
		args = append(args, "-L"+d)
	}
	if runtime.GOOS != "windows" {
		isUser := map[string]bool{}
		for _, d := range userDirs {
			isUser[d] = true
		}
		rpaths := map[string]bool{}
		for _, l := range libs {
			if l.Shared && isUser[l.Dir] && !rpaths[l.Dir] {
				rpaths[l.Dir] = true
				args = append(args, "-Wl,-rpath,"+l.Dir)
			}
		}
	}
	for _, l := range libs {
		args = append(args, "-l"+l.Name)
	}
	return append(args, opts.LinkArgs...)
}

var (
	ansiEscape   = regexp.MustCompile(`\x1b\[[0-9;]*m`)
	includedFrom = regexp.MustCompile(`^In file included from (.+):(\d+):$`)
	clangError   = regexp.MustCompile(`^(.+?):(\d+):(\d+): (?:fatal )?error: (.*)$`)
)

// glueErrorDiagnostic looks through clang output for an error inside an
// extern glue header, either directly or in a file the glue header includes.
// It returns a QK-FFI-003 diagnostic located in the glue header, or nil.
func glueErrorDiagnostic(output string, glueFiles []string) *diagnostics.Diagnostic {
	if len(glueFiles) == 0 {
		return nil
	}
	isGlue := map[string]bool{}
	for _, f := range glueFiles {
		isGlue[filepath.Clean(f)] = true
	}
	type site struct {
		file      string
		line, col int
	}
	var chain []site
	sc := bufio.NewScanner(strings.NewReader(ansiEscape.ReplaceAllString(output, "")))
	for sc.Scan() {
		line := sc.Text()
		if m := includedFrom.FindStringSubmatch(line); m != nil {
			n, _ := strconv.Atoi(m[2])
			chain = append(chain, site{file: filepath.Clean(m[1]), line: n})
			continue
		}
		m := clangError.FindStringSubmatch(line)
		if m == nil {
			chain = nil
			continue
		}
		errFile := filepath.Clean(m[1])
		ln, _ := strconv.Atoi(m[2])
		col, _ := strconv.Atoi(m[3])
		var at *site
		var notes []string
		if isGlue[errFile] {
			at = &site{file: errFile, line: ln, col: col}
		} else {
			// Innermost glue header on the include chain, if any.
			for i := len(chain) - 1; i >= 0; i-- {
				if isGlue[chain[i].file] {
					at = &chain[i]
					notes = append(notes, fmt.Sprintf("the error is in %s:%d, included from here", errFile, ln))
					break
				}
			}
		}
		chain = nil
		if at == nil {
			continue
		}
		return &diagnostics.Diagnostic{
			Code:     "QK-FFI-003",
			Stage:    diagnostics.StageBuild,
			Severity: diagnostics.SeverityError,
			Message:  "C++ error in extern glue code: " + m[4],
			Location: &diagnostics.Location{File: at.file, Line: at.line, Column: at.col},
			Notes:    append(notes, "clang's full output is above"),
		}
	}
	return nil
}
