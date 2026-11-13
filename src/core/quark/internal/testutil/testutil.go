// Package testutil runs the compiler on inline test sources. Everything past
// parsing goes through the driver, so a test sees exactly what the CLI does:
// the same stage order, the same early stops and the same diagnostics.
package testutil

import (
	"quark/ast"
	"quark/diagnostics"
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

// Result is the outcome of running the driver on an inline source.
type Result struct {
	Unit *driver.Unit
	AST  *ast.TreeNode
	// Analysis is nil unless analysis succeeded.
	Analysis *types.Analysis
	// ParserErrors holds parse and load errors, TypeErrors the errors of
	// semantic analysis and of the invariant checks. The driver stops at the
	// first stage that fails, so at most one of them is non-empty.
	ParserErrors []string
	TypeErrors   []string
	Warnings     []diagnostics.Diagnostic
	CPP          string
}

// PipelineResult is the name the emit tests use for Result.
type PipelineResult = Result

func Lex(source string) []token.Token {
	l := lexer.New(source)
	return l.Tokenize()
}

// Parse lexes and parses source only (no imports are loaded).
func Parse(source string) (*ast.TreeNode, []string) {
	toks := Lex(source)
	p := parser.New(toks)
	node := p.Parse()
	return node, p.Errors()
}

// Compile runs the driver on source through stage upTo.
func Compile(source string, upTo driver.Stage) *Result {
	u := driver.CompileSource(source, sourceFile, upTo)
	r := &Result{Unit: u, AST: u.AST, Analysis: u.Analysis, CPP: u.CPP}
	for _, d := range u.Diagnostics {
		if d.Severity == diagnostics.SeverityWarning {
			r.Warnings = append(r.Warnings, d)
			continue
		}
		switch d.Stage {
		case diagnostics.StageLex, diagnostics.StageParse, diagnostics.StageLoad:
			r.ParserErrors = append(r.ParserErrors, d.String())
		default:
			r.TypeErrors = append(r.TypeErrors, d.String())
		}
	}
	if u.Err != nil {
		r.TypeErrors = append(r.TypeErrors, u.Err.Error())
	}
	return r
}

// Analyze runs the driver through semantic analysis. It returns the result,
// the tree, the parse and load errors, and the analysis errors.
func Analyze(source string) (*Result, *ast.TreeNode, []string, []string) {
	r := Compile(source, driver.StageCheck)
	return r, r.AST, r.ParserErrors, r.TypeErrors
}

// GenerateCPP runs the driver through C++ generation, with the same invariant
// checks and back end as the compiler.
func GenerateCPP(source string) Result {
	return *Compile(source, driver.StageEmit)
}
