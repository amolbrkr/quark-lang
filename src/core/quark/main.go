package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"quark/driver"
	"quark/lexer"
	"quark/toolchain"
)

func main() {
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}

	command, args := os.Args[1], os.Args[2:]
	switch command {
	case "lex":
		os.Exit(cmdLex(args))
	case "parse":
		os.Exit(cmdParse(args))
	case "check":
		os.Exit(cmdCheck(args))
	case "emit":
		os.Exit(cmdEmit(args))
	case "build":
		os.Exit(cmdBuild(args))
	case "run":
		os.Exit(cmdRun(args))
	case "help", "-h", "--help":
		printUsage()
	default:
		// `quark file.qrk [flags]` is shorthand for `quark run`.
		if strings.HasSuffix(command, ".qrk") {
			os.Exit(cmdRun(os.Args[1:]))
		}
		fmt.Printf("Unknown command: %s\n", command)
		printUsage()
		os.Exit(1)
	}
}

func printUsage() {
	fmt.Println("Quark Compiler v0.1")
	fmt.Println()
	fmt.Println("Usage: quark <command> [arguments]")
	fmt.Println()
	fmt.Println("Commands:")
	fmt.Println("  lex <file>                    Tokenize a file and print tokens")
	fmt.Println("  parse <file>                  Parse a file and print the AST")
	fmt.Println("  check <file>                  Type check a file")
	fmt.Println("  emit <file>                   Emit C++ code to stdout")
	fmt.Println("  build <file> [-o out]         Compile to executable")
	fmt.Println("  run <file> [--debug]          Compile and run")
	fmt.Println("  help                          Show this help message")
	fmt.Println()
	fmt.Println("Flags (run/build):")
	fmt.Println("  --debug, -d    Keep the generated C++ file and print compiler commands")
	fmt.Println("  --lto          Enable link-time optimization")
	fmt.Println("  --no-pch       Do not use the cached precompiled runtime header")
	fmt.Println("  --lib-path <dir>       Search <dir> for extern link libraries (repeatable)")
	fmt.Println("  --include-path <dir>   Add <dir> to the C++ include path (repeatable)")
	fmt.Println("  --link-arg <arg>       Pass <arg> to the linker as-is (repeatable)")
	fmt.Println()
	fmt.Println("Environment:")
	fmt.Println("  QUARK_LIB_PATH, QUARK_INCLUDE_PATH   Like --lib-path / --include-path; path lists")
	fmt.Println()
	fmt.Println("Examples:")
	fmt.Println("  quark run test.qrk                # Compile and run")
	fmt.Println("  quark build test.qrk -o app       # Build an executable")
	fmt.Println("  quark test.qrk                    # Shorthand for run")
}

// parseCommand parses flags for a subcommand and returns its single file
// argument. Flags may appear before or after the file.
func parseCommand(fs *flag.FlagSet, usage string, args []string) (string, bool) {
	fs.SetOutput(io.Discard)
	var positional []string
	for {
		if err := fs.Parse(args); err != nil {
			fmt.Fprintf(os.Stderr, "%s\nUsage: %s\n", err, usage)
			return "", false
		}
		args = fs.Args()
		if len(args) == 0 {
			break
		}
		positional = append(positional, args[0])
		args = args[1:]
	}
	if len(positional) != 1 {
		fmt.Printf("Usage: %s\n", usage)
		return "", false
	}
	return positional[0], true
}

// buildFlags are the flags shared by run and build.
type buildFlags struct {
	debug        bool
	lto          bool
	noPCH        bool
	libPaths     repeatedFlag
	includePaths repeatedFlag
	linkArgs     repeatedFlag
}

// repeatedFlag collects every occurrence of a repeatable string flag.
type repeatedFlag []string

func (r *repeatedFlag) String() string     { return strings.Join(*r, ",") }
func (r *repeatedFlag) Set(v string) error { *r = append(*r, v); return nil }

func (f *buildFlags) register(fs *flag.FlagSet) {
	fs.BoolVar(&f.debug, "debug", false, "")
	fs.BoolVar(&f.debug, "d", false, "")
	fs.BoolVar(&f.lto, "lto", false, "")
	fs.BoolVar(&f.noPCH, "no-pch", false, "")
	fs.Var(&f.libPaths, "lib-path", "")
	fs.Var(&f.includePaths, "include-path", "")
	fs.Var(&f.linkArgs, "link-arg", "")
}

// options returns the toolchain options for building u. Search directories
// come from the CLI first, then the environment; include directories from
// extern include_path directives follow them.
func (f *buildFlags) options(u *driver.Unit) toolchain.Options {
	opts := toolchain.Options{LTO: f.lto, PCH: !f.noPCH, Verbose: f.debug}
	opts.LibDirs = absDirs(append(append([]string{}, f.libPaths...), filepath.SplitList(os.Getenv("QUARK_LIB_PATH"))...))
	opts.IncludeDirs = absDirs(append(append([]string{}, f.includePaths...), filepath.SplitList(os.Getenv("QUARK_INCLUDE_PATH"))...))
	opts.IncludeDirs = append(opts.IncludeDirs, u.Native.IncludeDirs...)
	for _, lib := range u.Native.Libs {
		opts.Libs = append(opts.Libs, toolchain.Lib{Name: lib.Name, Origin: lib.Origin()})
	}
	opts.LinkArgs = append([]string{}, f.linkArgs...)
	opts.GlueFiles = append([]string{}, u.Native.Headers...)
	return opts
}

// absDirs makes each non-empty directory absolute, relative to the working
// directory.
func absDirs(dirs []string) []string {
	var out []string
	for _, d := range dirs {
		if d == "" {
			continue
		}
		if abs, err := filepath.Abs(d); err == nil {
			d = abs
		}
		out = append(out, d)
	}
	return out
}

// reportBuildError prints a compile or link failure. Diagnostics from the
// toolchain (missing library, error in glue code) are printed as they are;
// it returns true for those so callers can skip the generated-C++ dump.
func reportBuildError(err error) bool {
	var de *toolchain.DiagnosticError
	if errors.As(err, &de) {
		fmt.Fprintln(os.Stderr, de.Error())
		return true
	}
	fmt.Fprintf(os.Stderr, "Compilation failed: %s\n", err)
	return false
}

// frontEnd runs the compiler front end through upTo and prints diagnostics.
// It returns nil when compilation failed.
func frontEnd(file string, upTo driver.Stage) *driver.Unit {
	u := driver.CompileFile(file, upTo)
	u.PrintDiagnostics()
	if u.Failed() {
		return nil
	}
	return u
}

func cmdLex(args []string) int {
	file, ok := parseCommand(flag.NewFlagSet("lex", flag.ContinueOnError), "quark lex <file.qrk>", args)
	if !ok {
		return 1
	}
	content, err := os.ReadFile(file)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error reading file: %s\n", err)
		return 1
	}

	tokens := lexer.New(string(content)).Tokenize()
	fmt.Printf("Tokens from %s:\n", file)
	fmt.Println("----------------------------------------")
	for i, tok := range tokens {
		fmt.Printf("%3d: %-12s %q (line %d, col %d)\n",
			i, tok.Type.String(), tok.Literal, tok.Line, tok.Column)
	}
	fmt.Println("----------------------------------------")
	fmt.Printf("Total: %d tokens\n", len(tokens))
	return 0
}

func cmdParse(args []string) int {
	file, ok := parseCommand(flag.NewFlagSet("parse", flag.ContinueOnError), "quark parse <file.qrk>", args)
	if !ok {
		return 1
	}
	u := frontEnd(file, driver.StageParse)
	if u == nil {
		return 1
	}
	fmt.Printf("AST for %s:\n", file)
	fmt.Println("========================================")
	u.AST.PrintTree()
	fmt.Println("========================================")
	return 0
}

func cmdCheck(args []string) int {
	file, ok := parseCommand(flag.NewFlagSet("check", flag.ContinueOnError), "quark check <file.qrk>", args)
	if !ok {
		return 1
	}
	if frontEnd(file, driver.StageCheck) == nil {
		return 1
	}
	fmt.Println("No errors found.")
	return 0
}

func cmdEmit(args []string) int {
	file, ok := parseCommand(flag.NewFlagSet("emit", flag.ContinueOnError), "quark emit <file.qrk>", args)
	if !ok {
		return 1
	}
	u := frontEnd(file, driver.StageEmit)
	if u == nil {
		return 1
	}
	fmt.Println(u.CPP)
	return 0
}

func cmdBuild(args []string) int {
	var flags buildFlags
	var output string
	fs := flag.NewFlagSet("build", flag.ContinueOnError)
	flags.register(fs)
	fs.StringVar(&output, "o", "", "")
	file, ok := parseCommand(fs, "quark build <file.qrk> [-o output] [--debug] [--lto] [--no-pch] [--lib-path dir] [--include-path dir] [--link-arg arg]", args)
	if !ok {
		return 1
	}
	if output == "" {
		base := filepath.Base(file)
		output = strings.TrimSuffix(base, filepath.Ext(base))
	}
	output = toolchain.ExeName(output)

	u := frontEnd(file, driver.StageEmit)
	if u == nil {
		return 1
	}

	workDir, err := os.MkdirTemp("", "quark-build-")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error creating build directory: %s\n", err)
		return 1
	}
	defer os.RemoveAll(workDir)

	cppFile := filepath.Join(workDir, "main.cpp")
	if flags.debug {
		cppFile = output + ".cpp"
	}
	tc, err := toolchain.Find()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %s\n", err)
		return 1
	}
	if err := compileProgram(tc, u, cppFile, workDir, output, flags); err != nil {
		reportBuildError(err)
		return 1
	}
	fmt.Printf("Built: %s\n", output)
	return 0
}

func cmdRun(args []string) int {
	var flags buildFlags
	fs := flag.NewFlagSet("run", flag.ContinueOnError)
	flags.register(fs)
	file, ok := parseCommand(fs, "quark run <file.qrk> [--debug] [--lto] [--no-pch] [--lib-path dir] [--include-path dir] [--link-arg arg]", args)
	if !ok {
		return 1
	}

	u := frontEnd(file, driver.StageEmit)
	if u == nil {
		return 1
	}

	workDir, err := os.MkdirTemp("", "quark-run-")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error creating build directory: %s\n", err)
		return 1
	}
	defer os.RemoveAll(workDir)

	cppFile := filepath.Join(workDir, "main.cpp")
	exeFile := toolchain.ExeName(filepath.Join(workDir, "main"))
	if flags.debug {
		// Keep the C++ and the executable next to the source.
		base := strings.TrimSuffix(file, filepath.Ext(file))
		cppFile = base + ".cpp"
		exeFile = toolchain.ExeName(base)
	}

	tc, err := toolchain.Find()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error: %s\n", err)
		return 1
	}
	if err := compileProgram(tc, u, cppFile, workDir, exeFile, flags); err != nil {
		if !reportBuildError(err) {
			fmt.Fprintln(os.Stderr, "\nGenerated C++ code:")
			fmt.Fprintln(os.Stderr, u.CPP)
		}
		return 1
	}

	// A bare name like "hello" would be looked up on PATH.
	if abs, err := filepath.Abs(exeFile); err == nil {
		exeFile = abs
	}
	runCmd := exec.Command(exeFile)
	runCmd.Stdout = os.Stdout
	runCmd.Stderr = os.Stderr
	runCmd.Stdin = os.Stdin
	if err := runCmd.Run(); err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			return exitErr.ExitCode()
		}
		fmt.Fprintf(os.Stderr, "Error running %s: %s\n", exeFile, err)
		return 1
	}
	return 0
}

// compileProgram writes u's C++ to cppFile, then compiles it (object file in
// workDir) and links it into exe.
func compileProgram(tc *toolchain.Toolchain, u *driver.Unit, cppFile, workDir, exe string, flags buildFlags) error {
	if err := os.WriteFile(cppFile, []byte(u.CPP), 0o644); err != nil {
		return fmt.Errorf("writing C++ file: %w", err)
	}
	if flags.debug {
		fmt.Fprintf(os.Stderr, "Debug: Generated C++ file: %s\n", cppFile)
		fmt.Fprintf(os.Stderr, "Debug: Runtime include path: %s\n", tc.RuntimeInclude)
	}

	opts := flags.options(u)
	objFile := filepath.Join(workDir, "main.o")
	if err := tc.Compile(cppFile, objFile, opts); err != nil {
		return err
	}
	return tc.Link([]string{objFile}, exe, opts)
}
