package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"quark/codegen"
	"quark/lexer"
	"quark/parser"
	"quark/types"
)

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
		if len(os.Args) >= 5 && os.Args[3] == "-o" {
			output = os.Args[4]
		}
		runBuild(os.Args[2], output)

	case "run":
		if len(os.Args) < 3 {
			fmt.Println("Usage: quark run <file.qrk> [--debug]")
			os.Exit(1)
		}
		debug := false
		for _, arg := range os.Args[3:] {
			if arg == "--debug" || arg == "-d" {
				debug = true
			}
		}
		runRun(os.Args[2], debug)

	case "help", "-h", "--help":
		printUsage()

	default:
		// Check if it's a .qrk file - if so, run it
		if strings.HasSuffix(os.Args[1], ".qrk") {
			debug := false
			for _, arg := range os.Args[2:] {
				if arg == "--debug" || arg == "-d" {
					debug = true
				}
			}
			runRun(os.Args[1], debug)
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
	fmt.Println("  lex <file>                 Tokenize a file and print tokens")
	fmt.Println("  parse <file>               Parse a file and print the AST")
	fmt.Println("  check <file>               Type check a file")
	fmt.Println("  emit <file>                Emit C code to stdout")
	fmt.Println("  build <file> [-o out]      Compile to executable")
	fmt.Println("  run <file> [--debug|-d]    Compile and run (--debug saves .c file)")
	fmt.Println("  help                       Show this help message")
	fmt.Println()
	fmt.Println("Examples:")
	fmt.Println("  quark run test.qrk           # Compile and run")
	fmt.Println("  quark build test.qrk -o app  # Compile to executable")
	fmt.Println("  quark test.qrk               # Shorthand for run")
}

func compile(filename string) (*codegen.Generator, error) {
	content, err := os.ReadFile(filename)
	if err != nil {
		return nil, fmt.Errorf("error reading file: %w", err)
	}

	// Lexer phase
	l := lexer.New(string(content))
	tokens := l.Tokenize()

	// Parser phase
	p := parser.New(tokens)
	ast := p.Parse()

	if len(p.Errors()) > 0 {
		return nil, fmt.Errorf("parser errors:\n  %s", strings.Join(p.Errors(), "\n  "))
	}

	// Type checking phase
	analyzer := types.NewAnalyzer()
	analyzer.Analyze(ast)

	if len(analyzer.Errors()) > 0 {
		return nil, fmt.Errorf("type errors:\n  %s", strings.Join(analyzer.Errors(), "\n  "))
	}

	// Code generation phase
	gen := codegen.New()
	gen.Generate(ast)

	return gen, nil
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
	ast := p.Parse()

	if len(p.Errors()) > 0 {
		fmt.Println("Parser errors:")
		for _, err := range p.Errors() {
			fmt.Printf("  %s\n", err)
		}
		os.Exit(1)
	}

	fmt.Printf("AST for %s:\n", filename)
	fmt.Println("========================================")
	ast.PrintTree()
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
	ast := p.Parse()

	if len(p.Errors()) > 0 {
		fmt.Println("Parser errors:")
		for _, err := range p.Errors() {
			fmt.Printf("  %s\n", err)
		}
		os.Exit(1)
	}

	analyzer := types.NewAnalyzer()
	analyzer.Analyze(ast)

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
	ast := p.Parse()

	if len(p.Errors()) > 0 {
		fmt.Println("Parser errors:")
		for _, err := range p.Errors() {
			fmt.Printf("  %s\n", err)
		}
		os.Exit(1)
	}

	gen := codegen.New()
	cCode := gen.Generate(ast)
	fmt.Println(cCode)
}

func runBuild(filename string, output string) {
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
	ast := p.Parse()

	if len(p.Errors()) > 0 {
		fmt.Fprintln(os.Stderr, "Parser errors:")
		for _, err := range p.Errors() {
			fmt.Fprintf(os.Stderr, "  %s\n", err)
		}
		os.Exit(1)
	}

	gen := codegen.New()
	cCode := gen.Generate(ast)

	// Write C code to temp file
	tmpDir := os.TempDir()
	cFile := filepath.Join(tmpDir, "quark_temp.c")
	err = os.WriteFile(cFile, []byte(cCode), 0644)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error writing C file: %s\n", err)
		os.Exit(1)
	}

	// Compile with clang (or gcc as fallback)
	compiler := "clang"
	if _, err := exec.LookPath("clang"); err != nil {
		compiler = "gcc"
		if _, err := exec.LookPath("gcc"); err != nil {
			fmt.Fprintln(os.Stderr, "Error: neither clang nor gcc found in PATH")
			os.Exit(1)
		}
	}

	cmd := exec.Command(compiler, "-O3", "-march=native", "-o", output, cFile)
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

func runRun(filename string, debug bool) {
	content, err := os.ReadFile(filename)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error reading file: %s\n", err)
		os.Exit(1)
	}

	// Compile
	l := lexer.New(string(content))
	tokens := l.Tokenize()

	p := parser.New(tokens)
	ast := p.Parse()

	if len(p.Errors()) > 0 {
		fmt.Fprintln(os.Stderr, "Parser errors:")
		for _, err := range p.Errors() {
			fmt.Fprintf(os.Stderr, "  %s\n", err)
		}
		os.Exit(1)
	}

	gen := codegen.New()
	cCode := gen.Generate(ast)

	// Determine file paths
	var cFile, exeFile string
	if debug {
		// Save C file next to the source file
		base := strings.TrimSuffix(filename, filepath.Ext(filename))
		cFile = base + ".c"
		exeFile = base
	} else {
		tmpDir := os.TempDir()
		cFile = filepath.Join(tmpDir, "quark_temp.c")
		exeFile = filepath.Join(tmpDir, "quark_temp")
	}

	err = os.WriteFile(cFile, []byte(cCode), 0644)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error writing C file: %s\n", err)
		os.Exit(1)
	}

	if debug {
		fmt.Fprintf(os.Stderr, "Debug: Generated C file: %s\n", cFile)
	}

	// Compile with clang (or gcc as fallback)
	compiler := "clang"
	if _, err := exec.LookPath("clang"); err != nil {
		compiler = "gcc"
		if _, err := exec.LookPath("gcc"); err != nil {
			fmt.Fprintln(os.Stderr, "Error: neither clang nor gcc found in PATH")
			os.Exit(1)
		}
	}

	compileCmd := exec.Command(compiler, "-O3", "-march=native", "-o", exeFile, cFile)
	compileCmd.Stderr = os.Stderr

	err = compileCmd.Run()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Compilation failed: %s\n", err)
		// Print the C code for debugging
		fmt.Fprintln(os.Stderr, "\nGenerated C code:")
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
