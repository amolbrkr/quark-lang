package testutil

import (
	"path/filepath"

	"quark/ast"
	"quark/driver"
	"quark/lexer"
	"quark/parser"
	"quark/token"
	"quark/types"
)

// sourceFile is the virtual path inline test sources are compiled as. It
// sits in the test's working directory so relative and std/ imports resolve
// the same way they would for a real file there.
const sourceFile = "test_input.qrk"

type PipelineResult struct {
	Tokens       []token.Token
	AST          *ast.TreeNode
	ParserErrors []string
	Analyzer     *types.Analyzer
	TypeErrors   []string
	CPP          string
}

func Lex(source string) []token.Token {
	l := lexer.New(source)
	return l.Tokenize()
}

func Parse(source string) (*ast.TreeNode, []string) {
	toks := Lex(source)
	p := parser.New(toks)
	node := p.Parse()
	return node, p.Errors()
}

// resolveImports runs the loader on a parsed test source, as the driver
// does, and returns its errors.
func resolveImports(node *ast.TreeNode) []string {
	absPath, err := filepath.Abs(sourceFile)
	if err != nil {
		return []string{err.Error()}
	}
	var errs []string
	for _, d := range driver.ResolveImports(node, absPath) {
		errs = append(errs, d.Message)
	}
	return errs
}

// Analyze parses, resolves imports and analyzes source. Unlike the driver it
// analyzes even when parsing failed, so tests can inspect both sets of
// errors. Loader errors are reported with the parse errors.
func Analyze(source string) (*types.Analyzer, *ast.TreeNode, []string, []string) {
	node, parseErrs := Parse(source)
	if len(parseErrs) == 0 {
		parseErrs = resolveImports(node)
	}
	analyzer := types.NewAnalyzer()
	analyzer.Analyze(node)
	return analyzer, node, parseErrs, analyzer.Errors()
}

// GenerateCPP runs the full front end on source with the same invariant
// checks and codegen setup as the compiler.
func GenerateCPP(source string) PipelineResult {
	analyzer, node, parseErrs, typeErrs := Analyze(source)
	cpp := ""
	if len(parseErrs) == 0 && len(typeErrs) == 0 {
		an := analyzer.Analysis(node)
		if err := driver.Validate(node, an); err != nil {
			typeErrs = append(typeErrs, err.Error())
		} else {
			cpp = driver.Generate(node, an, sourceFile)
		}
	}
	return PipelineResult{
		Tokens:       nil,
		AST:          node,
		ParserErrors: parseErrs,
		Analyzer:     analyzer,
		TypeErrors:   typeErrs,
		CPP:          cpp,
	}
}
