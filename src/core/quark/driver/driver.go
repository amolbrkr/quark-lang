// Package driver runs the compiler front end: lex, parse, load imports,
// analyze, validate invariants, and emit C++. Every CLI command and the test
// helpers go through it so the stage sequence lives in exactly one place.
package driver

import (
	"fmt"
	"os"
	"path/filepath"

	"quark/ast"
	"quark/codegen"
	"quark/diagnostics"
	"quark/invariants"
	"quark/lexer"
	"quark/loader"
	"quark/parser"
	"quark/token"
	"quark/types"
)

// Stage is the last front-end stage to run.
type Stage int

const (
	StageParse Stage = iota // lex, parse, resolve imports
	StageCheck              // + semantic analysis
	StageEmit               // + invariant validation and C++ generation
)

// Unit is the result of running the front end on one source file.
type Unit struct {
	File        string // path as given, used to label diagnostics
	Tokens      []token.Token
	AST         *ast.TreeNode
	Analysis    *types.Analysis
	CPP         string
	Diagnostics []diagnostics.Diagnostic // errors and warnings, in stage order
	Err         error                    // I/O or invariant failure
	failed      bool
}

// Failed reports whether any stage produced an error.
func (u *Unit) Failed() bool {
	return u.failed || u.Err != nil
}

// CompileFile reads path and runs the front end through stage upTo.
func CompileFile(path string, upTo Stage) *Unit {
	content, err := os.ReadFile(path)
	if err != nil {
		return &Unit{File: path, Err: fmt.Errorf("Error reading file: %s", err)}
	}
	return CompileSource(string(content), path, upTo)
}

// CompileSource runs the front end on src through stage upTo. file locates
// relative imports and labels diagnostics.
func CompileSource(src, file string, upTo Stage) *Unit {
	u := &Unit{File: file}

	u.Tokens = lexer.New(src).Tokenize()
	p := parser.New(u.Tokens)
	u.AST = p.Parse()
	if len(p.Errors()) > 0 {
		u.addDiags(diagnostics.WithDefaultFile(p.Diagnostics(), file), true)
		return u
	}

	absPath, err := filepath.Abs(file)
	if err != nil {
		u.Err = fmt.Errorf("Error resolving path: %s", err)
		return u
	}
	if loadDiags := ResolveImports(u.AST, absPath); len(loadDiags) > 0 {
		u.addDiags(diagnostics.WithDefaultFile(loadDiags, absPath), true)
		return u
	}
	if upTo < StageCheck {
		return u
	}

	analyzer := types.NewAnalyzer()
	analyzer.Analyze(u.AST)
	u.addDiags(diagnostics.WithDefaultFile(analyzer.Diagnostics(), file), analyzer.HasErrors())
	if analyzer.HasErrors() {
		return u
	}
	u.Analysis = analyzer.Analysis(u.AST)
	if upTo < StageEmit {
		return u
	}

	if err := Validate(u.AST, u.Analysis); err != nil {
		u.Err = err
		return u
	}
	u.CPP = Generate(u.AST, u.Analysis, file)
	return u
}

// ResolveImports splices `use` imports into tree and resolves `extern`
// header paths. absPath is the absolute path of the file tree came from.
func ResolveImports(tree *ast.TreeNode, absPath string) []diagnostics.Diagnostic {
	ml := loader.NewModuleLoader()
	ml.ResolveImports(tree, absPath)
	if len(ml.Errors()) > 0 {
		return ml.Diagnostics()
	}
	return nil
}

// Validate checks the pre-codegen invariants on an analyzed tree.
func Validate(tree *ast.TreeNode, an *types.Analysis) error {
	if err := invariants.ValidateCallPlans(tree, an.CallPlans); err != nil {
		return err
	}
	return invariants.ValidateReturnAnnotations(tree, an.ReturnValidation)
}

// Generate emits C++ for an analyzed, validated tree.
func Generate(tree *ast.TreeNode, an *types.Analysis, sourceName string) string {
	gen := codegen.New()
	gen.SetSourceName(sourceName)
	gen.SetAnalysis(an)
	return gen.Generate(tree)
}

// PrintDiagnostics writes u's diagnostics and error to stderr.
func (u *Unit) PrintDiagnostics() {
	for _, d := range u.Diagnostics {
		fmt.Fprintln(os.Stderr, d.String())
	}
	if u.Err != nil {
		fmt.Fprintln(os.Stderr, u.Err.Error())
	}
}

func (u *Unit) addDiags(diags []diagnostics.Diagnostic, isError bool) {
	u.Diagnostics = append(u.Diagnostics, diags...)
	if isError {
		u.failed = true
	}
}
