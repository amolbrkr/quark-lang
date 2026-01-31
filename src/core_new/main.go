package main

import (
	"fmt"
	"os"
	"quark/lexer"
	"quark/parser"
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

	case "help", "-h", "--help":
		printUsage()

	default:
		// Assume it's a file to compile
		if len(os.Args) >= 2 {
			runParser(os.Args[1])
		} else {
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
	fmt.Println("  lex <file>     Tokenize a file and print tokens")
	fmt.Println("  parse <file>   Parse a file and print the AST")
	fmt.Println("  help           Show this help message")
	fmt.Println()
	fmt.Println("Example:")
	fmt.Println("  quark lex test.qrk")
	fmt.Println("  quark parse test.qrk")
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

	// Lexer phase
	l := lexer.New(string(content))
	tokens := l.Tokenize()

	// Parser phase
	p := parser.New(tokens)
	ast := p.Parse()

	// Check for errors
	if len(p.Errors()) > 0 {
		fmt.Println("Parser errors:")
		for _, err := range p.Errors() {
			fmt.Printf("  %s\n", err)
		}
		os.Exit(1)
	}

	// Print AST
	fmt.Printf("AST for %s:\n", filename)
	fmt.Println("========================================")
	ast.PrintTree()
	fmt.Println("========================================")
}
