package testutil

import (
	"quark/ast"
	"quark/codegen"
	"quark/lexer"
	"quark/parser"
	"quark/token"
	"quark/types"
)

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

func Analyze(source string) (*types.Analyzer, *ast.TreeNode, []string, []string) {
	node, parseErrs := Parse(source)
	analyzer := types.NewAnalyzer()
	analyzer.Analyze(node)
	return analyzer, node, parseErrs, analyzer.Errors()
}

func GenerateCPP(source string) PipelineResult {
	analyzer, node, parseErrs, typeErrs := Analyze(source)
	gen := codegen.New()
	gen.SetCaptures(analyzer.GetCaptures())
	cpp := gen.Generate(node)
	return PipelineResult{
		Tokens:       nil,
		AST:          node,
		ParserErrors: parseErrs,
		Analyzer:     analyzer,
		TypeErrors:   typeErrs,
		CPP:          cpp,
	}
}
