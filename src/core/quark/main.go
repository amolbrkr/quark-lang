package main

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"quark/ast"
	"quark/codegen"
	"quark/lexer"
	"quark/loader"
	"quark/parser"
	"quark/types"
)

// getRuntimeIncludePath returns the path to the runtime include directory
// relative to the quark executable
func getRuntimeIncludePath() string {
	// Get the executable path
	exePath, err := os.Executable()
	if err != nil {
		// Fallback to current directory
		return filepath.Join("runtime", "include")
	}

	// Get the directory containing the executable
	exeDir := filepath.Dir(exePath)

	// Runtime headers are in ../runtime/include relative to the executable
	// (exe is in src/core/quark, runtime is in src/core/quark/runtime)
	runtimePath := filepath.Join(exeDir, "runtime", "include")

	// Check if the path exists
	if _, err := os.Stat(runtimePath); err == nil {
		return runtimePath
	}

	// Fallback: try relative to current directory
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

func findGCLibrary(buildDir string) (string, error) {
	candidates := []string{
		filepath.Join(buildDir, "libgc.a"),
		filepath.Join(buildDir, "libgc.so"),
		filepath.Join(buildDir, "libgc.dylib"),
		filepath.Join(buildDir, "gc.lib"),
		filepath.Join(buildDir, "libgc.lib"),
		filepath.Join(buildDir, "Release", "gc.lib"),
		filepath.Join(buildDir, "Release", "libgc.lib"),
		filepath.Join(buildDir, "Debug", "gc.lib"),
		filepath.Join(buildDir, "Debug", "libgc.lib"),
	}

	for _, candidate := range candidates {
		if _, err := os.Stat(candidate); err == nil {
			return candidate, nil
		}
	}

	return "", fmt.Errorf("could not find built Boehm GC library under %s", buildDir)
}

func ensureGC() (includePath string, libPath string, err error) {
	gcSourceDir, err := findGCSourceDir()
	if err != nil {
		return "", "", err
	}

	includePath = filepath.Join(gcSourceDir, "include")
	if _, statErr := os.Stat(filepath.Join(includePath, "gc", "gc.h")); statErr != nil {
		return "", "", fmt.Errorf("Boehm GC headers not found at %s", includePath)
	}

	buildDir := filepath.Join(gcSourceDir, "build")
	if libPath, err = findGCLibrary(buildDir); err == nil {
		return includePath, libPath, nil
	}

	if _, lookErr := exec.LookPath("cmake"); lookErr != nil {
		return "", "", fmt.Errorf("Boehm GC is not built and cmake is not available in PATH; install cmake or build deps/bdwgc manually")
	}

	fmt.Fprintln(os.Stderr, "Boehm GC library not found; bootstrapping deps/bdwgc/build with CMake...")

	configureCmd := exec.Command("cmake", "-S", gcSourceDir, "-B", buildDir)
	configureCmd.Stdout = os.Stdout
	configureCmd.Stderr = os.Stderr
	if runErr := configureCmd.Run(); runErr != nil {
		return "", "", fmt.Errorf("failed to configure Boehm GC with CMake: %w", runErr)
	}

	buildArgs := []string{"--build", buildDir}
	if runtime.GOOS == "windows" {
		buildArgs = append(buildArgs, "--config", "Release")
	}
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

func main() {
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}

	command := os.Args[1]

	switch command {
	case "lex":
		if len(os.Args) < 3 {
			fmt.Println("Usage: quark lex <file.qrk>")
			os.Exit(1)
		}
		runLexer(os.Args[2])

	case "parse":
		if len(os.Args) < 3 {
			fmt.Println("Usage: quark parse <file.qrk>")
			os.Exit(1)
		}
		runParser(os.Args[2])

	case "check":
		if len(os.Args) < 3 {
			fmt.Println("Usage: quark check <file.qrk>")
			os.Exit(1)
		}
		runCheck(os.Args[2])

	case "emit":
		if len(os.Args) < 3 {
			fmt.Println("Usage: quark emit <file.qrk>")
			os.Exit(1)
		}
		runEmit(os.Args[2])

	case "build":
		if len(os.Args) < 3 {
			fmt.Println("Usage: quark build <file.qrk> [-o output]")
			os.Exit(1)
		}
		output := ""
		useGC := true
		for i := 3; i < len(os.Args); i++ {
			if os.Args[i] == "-o" && i+1 < len(os.Args) {
				output = os.Args[i+1]
				i++ // Skip next arg
			}
		}
		runBuild(os.Args[2], output, useGC)

	case "run":
		if len(os.Args) < 3 {
			fmt.Println("Usage: quark run <file.qrk> [--debug]")
			os.Exit(1)
		}
		debug := false
		useGC := true
		for _, arg := range os.Args[3:] {
			if arg == "--debug" || arg == "-d" {
				debug = true
			}
		}
		runRun(os.Args[2], debug, useGC)

	case "help", "-h", "--help":
		printUsage()

	default:
		// Check if it's a .qrk file - if so, run it
		if strings.HasSuffix(os.Args[1], ".qrk") {
			debug := false
			useGC := true
			for _, arg := range os.Args[2:] {
				if arg == "--debug" || arg == "-d" {
					debug = true
				}
			}
			runRun(os.Args[1], debug, useGC)
		} else {
			fmt.Printf("Unknown command: %s\n", command)
			printUsage()
			os.Exit(1)
		}
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
	fmt.Println("  emit <file>                   Emit C code to stdout")
	fmt.Println("  build <file> [-o out]         Compile to executable")
	fmt.Println("  run <file> [--debug]          Compile and run")
	fmt.Println("  help                          Show this help message")
	fmt.Println()
	fmt.Println("Flags:")
	fmt.Println("  --debug, -d    Save generated C++ file (for run/build)")
	fmt.Println()
	fmt.Println("Examples:")
	fmt.Println("  quark run test.qrk                # Compile and run with GC")
	fmt.Println("  quark build test.qrk -o app      # Build with GC")
	fmt.Println("  quark test.qrk                    # Shorthand for run")
}

// resolveImports runs the module loader on the parsed AST to splice in external file imports.
// Returns true if successful, false if there were errors (printed to stderr).
func resolveImports(tree *ast.TreeNode, filename string) bool {
	absPath, err := filepath.Abs(filename)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error resolving path: %s\n", err)
		return false
	}

	ml := loader.NewModuleLoader()
	ml.ResolveImports(tree, absPath)

	if len(ml.Errors()) > 0 {
		fmt.Fprintln(os.Stderr, "Import errors:")
		for _, e := range ml.Errors() {
			fmt.Fprintf(os.Stderr, "  %s\n", e)
		}
		return false
	}
	return true
}

func runLexer(filename string) {
	content, err := os.ReadFile(filename)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error reading file: %s\n", err)
		os.Exit(1)
	}

	l := lexer.New(string(content))
	tokens := l.Tokenize()

	fmt.Printf("Tokens from %s:\n", filename)
	fmt.Println("----------------------------------------")
	for i, tok := range tokens {
		fmt.Printf("%3d: %-12s %q (line %d, col %d)\n",
			i, tok.Type.String(), tok.Literal, tok.Line, tok.Column)
	}
	fmt.Println("----------------------------------------")
	fmt.Printf("Total: %d tokens\n", len(tokens))
}

func runParser(filename string) {
	content, err := os.ReadFile(filename)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error reading file: %s\n", err)
		os.Exit(1)
	}

	l := lexer.New(string(content))
	tokens := l.Tokenize()

	p := parser.New(tokens)
	tree := p.Parse()

	if len(p.Errors()) > 0 {
		fmt.Println("Parser errors:")
		for _, err := range p.Errors() {
			fmt.Printf("  %s\n", err)
		}
		os.Exit(1)
	}

	if !resolveImports(tree, filename) {
		os.Exit(1)
	}

	fmt.Printf("AST for %s:\n", filename)
	fmt.Println("========================================")
	tree.PrintTree()
	fmt.Println("========================================")
}

func runCheck(filename string) {
	content, err := os.ReadFile(filename)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error reading file: %s\n", err)
		os.Exit(1)
	}

	l := lexer.New(string(content))
	tokens := l.Tokenize()

	p := parser.New(tokens)
	tree := p.Parse()

	if len(p.Errors()) > 0 {
		fmt.Println("Parser errors:")
		for _, err := range p.Errors() {
			fmt.Printf("  %s\n", err)
		}
		os.Exit(1)
	}

	if !resolveImports(tree, filename) {
		os.Exit(1)
	}

	analyzer := types.NewAnalyzer()
	analyzer.Analyze(tree)

	if len(analyzer.Errors()) > 0 {
		fmt.Println("Type errors:")
		for _, err := range analyzer.Errors() {
			fmt.Printf("  %s\n", err)
		}
		os.Exit(1)
	}

	fmt.Println("No errors found.")
}

func runEmit(filename string) {
	content, err := os.ReadFile(filename)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error reading file: %s\n", err)
		os.Exit(1)
	}

	l := lexer.New(string(content))
	tokens := l.Tokenize()

	p := parser.New(tokens)
	tree := p.Parse()

	if len(p.Errors()) > 0 {
		fmt.Println("Parser errors:")
		for _, err := range p.Errors() {
			fmt.Printf("  %s\n", err)
		}
		os.Exit(1)
	}

	if !resolveImports(tree, filename) {
		os.Exit(1)
	}

	// Run analyzer to compute closure captures
	analyzer := types.NewAnalyzer()
	analyzer.Analyze(tree)

	if len(analyzer.Errors()) > 0 {
		fmt.Fprintln(os.Stderr, "Type errors:")
		for _, err := range analyzer.Errors() {
			fmt.Fprintf(os.Stderr, "  %s\n", err)
		}
		os.Exit(1)
	}

	gen := codegen.New()
	gen.SetCaptures(analyzer.GetCaptures())
	cCode := gen.Generate(tree)
	fmt.Println(cCode)
}

func runBuild(filename string, output string, useGC bool) {
	content, err := os.ReadFile(filename)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error reading file: %s\n", err)
		os.Exit(1)
	}

	// Determine output filename
	if output == "" {
		base := filepath.Base(filename)
		output = strings.TrimSuffix(base, filepath.Ext(base))
	}

	// Compile
	l := lexer.New(string(content))
	tokens := l.Tokenize()

	p := parser.New(tokens)
	tree := p.Parse()

	if len(p.Errors()) > 0 {
		fmt.Fprintln(os.Stderr, "Parser errors:")
		for _, err := range p.Errors() {
			fmt.Fprintf(os.Stderr, "  %s\n", err)
		}
		os.Exit(1)
	}

	if !resolveImports(tree, filename) {
		os.Exit(1)
	}

	// Type checking phase
	analyzer := types.NewAnalyzer()
	analyzer.Analyze(tree)

	if len(analyzer.Errors()) > 0 {
		fmt.Fprintln(os.Stderr, "Type errors:")
		for _, err := range analyzer.Errors() {
			fmt.Fprintf(os.Stderr, "  %s\n", err)
		}
		os.Exit(1)
	}

	gen := codegen.New()
	gen.SetCaptures(analyzer.GetCaptures())
	cCode := gen.Generate(tree)

	// Write C++ code to temp file
	tmpDir := os.TempDir()
	cFile := filepath.Join(tmpDir, "quark_temp.cpp")
	err = os.WriteFile(cFile, []byte(cCode), 0644)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error writing C++ file: %s\n", err)
		os.Exit(1)
	}

	// Compile with clang++ (or g++ as fallback)
	compiler := "clang++"
	if _, err := exec.LookPath("clang++"); err != nil {
		compiler = "g++"
		if _, err := exec.LookPath("g++"); err != nil {
			fmt.Fprintln(os.Stderr, "Error: neither clang++ nor g++ found in PATH")
			os.Exit(1)
		}
	}

	// Get runtime include path
	runtimeInclude := getRuntimeIncludePath()
	includePath := fmt.Sprintf("-I%s", runtimeInclude)

	// Build compilation arguments
	args := []string{"-std=c++17", "-O3", includePath}
	if runtime.GOARCH == "amd64" {
		args = append(args, "-march=x86-64-v3")
	}
	if compiler == "clang++" {
		args = append(args,
			"-Rpass=loop-vectorize",
			"-Rpass-missed=loop-vectorize",
			"-Rpass-analysis=loop-vectorize",
		)
	}
	var gcLibPath string
	// Add GC flags if enabled
	if useGC {
		gcInclude, resolvedLibPath, gcErr := ensureGC()
		if gcErr != nil {
			fmt.Fprintf(os.Stderr, "Error preparing Boehm GC: %s\n", gcErr)
			os.Exit(1)
		}
		gcLibPath = resolvedLibPath
		args = append(args, "-DQUARK_USE_GC", fmt.Sprintf("-I%s", gcInclude))
	}

	args = append(args, "-o", output, cFile)

	// Add linker flags
	if useGC {
		args = append(args, gcLibPath)
	}
	args = append(args, "-lm")

	cmd := exec.Command(compiler, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	err = cmd.Run()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Compilation failed: %s\n", err)
		os.Exit(1)
	}

	// Clean up
	os.Remove(cFile)

	fmt.Printf("Built: %s\n", output)
}

func runRun(filename string, debug bool, useGC bool) {
	content, err := os.ReadFile(filename)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error reading file: %s\n", err)
		os.Exit(1)
	}

	// Compile
	l := lexer.New(string(content))
	tokens := l.Tokenize()

	p := parser.New(tokens)
	tree := p.Parse()

	if len(p.Errors()) > 0 {
		fmt.Fprintln(os.Stderr, "Parser errors:")
		for _, err := range p.Errors() {
			fmt.Fprintf(os.Stderr, "  %s\n", err)
		}
		os.Exit(1)
	}

	if !resolveImports(tree, filename) {
		os.Exit(1)
	}

	// Type checking phase
	analyzer := types.NewAnalyzer()
	analyzer.Analyze(tree)

	if len(analyzer.Errors()) > 0 {
		fmt.Fprintln(os.Stderr, "Type errors:")
		for _, err := range analyzer.Errors() {
			fmt.Fprintf(os.Stderr, "  %s\n", err)
		}
		os.Exit(1)
	}

	gen := codegen.New()
	gen.SetCaptures(analyzer.GetCaptures())
	cCode := gen.Generate(tree)

	// Determine file paths
	var cFile, exeFile string
	if debug {
		// Save C++ file next to the source file
		base := strings.TrimSuffix(filename, filepath.Ext(filename))
		cFile = base + ".cpp"
		exeFile = base
	} else {
		tmpDir := os.TempDir()
		cFile = filepath.Join(tmpDir, "quark_temp.cpp")
		exeFile = filepath.Join(tmpDir, "quark_temp")
	}

	err = os.WriteFile(cFile, []byte(cCode), 0644)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error writing C++ file: %s\n", err)
		os.Exit(1)
	}

	if debug {
		fmt.Fprintf(os.Stderr, "Debug: Generated C++ file: %s\n", cFile)
	}

	// Compile with clang++ (or g++ as fallback)
	compiler := "clang++"
	if _, err := exec.LookPath("clang++"); err != nil {
		compiler = "g++"
		if _, err := exec.LookPath("g++"); err != nil {
			fmt.Fprintln(os.Stderr, "Error: neither clang++ nor g++ found in PATH")
			os.Exit(1)
		}
	}

	// Get runtime include path
	runtimeInclude := getRuntimeIncludePath()
	includePath := fmt.Sprintf("-I%s", runtimeInclude)

	// Build compilation arguments
	args := []string{"-std=c++17", "-O3", includePath}
	if runtime.GOARCH == "amd64" {
		args = append(args, "-march=x86-64-v3")
	}
	if compiler == "clang++" {
		args = append(args,
			"-Rpass=loop-vectorize",
			"-Rpass-missed=loop-vectorize",
			"-Rpass-analysis=loop-vectorize",
		)
	}
	var gcLibPath string
	// Add GC flags if enabled
	if useGC {
		gcInclude, resolvedLibPath, gcErr := ensureGC()
		if gcErr != nil {
			fmt.Fprintf(os.Stderr, "Error preparing Boehm GC: %s\n", gcErr)
			os.Exit(1)
		}
		gcLibPath = resolvedLibPath
		args = append(args, "-DQUARK_USE_GC", fmt.Sprintf("-I%s", gcInclude))
	}

	args = append(args, "-o", exeFile, cFile)

	// Add linker flags
	if useGC {
		args = append(args, gcLibPath)
	}
	args = append(args, "-lm")

	if debug {
		fmt.Fprintf(os.Stderr, "Debug: Runtime include path: %s\n", runtimeInclude)
		fmt.Fprintf(os.Stderr, "Debug: Compile command: %s %s\n", compiler, strings.Join(args, " "))
	}

	compileCmd := exec.Command(compiler, args...)
	compileCmd.Stderr = os.Stderr

	err = compileCmd.Run()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Compilation failed: %s\n", err)
		// Print the C++ code for debugging
		fmt.Fprintln(os.Stderr, "\nGenerated C++ code:")
		fmt.Fprintln(os.Stderr, cCode)
		os.Exit(1)
	}

	// Run the executable
	runCmd := exec.Command(exeFile)
	runCmd.Stdout = os.Stdout
	runCmd.Stderr = os.Stderr
	runCmd.Stdin = os.Stdin

	err = runCmd.Run()

	// Clean up (only if not debug mode)
	if !debug {
		os.Remove(cFile)
		os.Remove(exeFile)
	}

	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			os.Exit(exitErr.ExitCode())
		}
		os.Exit(1)
	}
}
