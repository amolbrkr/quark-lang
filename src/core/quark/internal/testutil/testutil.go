package testutil

import (
	"quark/ast"
	"quark/codegen"
	"quark/invariants"
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
	if len(parseErrs) == 0 && len(typeErrs) == 0 {
		if err := invariants.ValidateCallPlans(node, analyzer.GetCallPlans()); err != nil {
			typeErrs = append(typeErrs, err.Error())
		}
		if err := invariants.ValidateReturnAnnotations(node, analyzer.GetReturnValidation()); err != nil {
			typeErrs = append(typeErrs, err.Error())
		}
	}
	cpp := ""
	if len(parseErrs) == 0 && len(typeErrs) == 0 {
		gen := codegen.New()
		gen.SetCaptures(analyzer.GetCaptures())
		gen.SetCallPlans(analyzer.GetCallPlans())
		cpp = gen.Generate(node)
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
