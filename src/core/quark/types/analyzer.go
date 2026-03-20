package types

import (
	"fmt"
	"quark/ast"
	"quark/builtins"
	"quark/ir"
)

type builtinSignature struct {
	Type    *FunctionType
	MinArgs int
	MaxArgs int
}

type paramSpec struct {
	name         string
	typeNode     *ast.TreeNode
	defaultValue *ast.TreeNode
}

// Module represents a defined module with its symbols
type Module struct {
	Name    string
	Scope   *Scope
	Symbols map[string]*Symbol
}

// Analyzer performs semantic analysis on the AST
type Analyzer struct {
	currentScope    *Scope
	errors          []string
	functions       map[string]*FunctionType // Track function signatures
	modules         map[string]*Module       // Track defined modules
	moduleAliases   map[string]string        // Alias -> module name mapping from use statements
	currentModule   string                   // Current module being defined (empty if global)
	builtins        map[string]*builtinSignature
	methods         map[builtins.TypeKey]map[string]*builtinSignature // receiver type -> method name -> sig
	captures        map[*ast.TreeNode][]string                        // Lambda node -> captured variable names
	callPlans       map[*ast.TreeNode]*ir.CallPlan
	returnValidated map[*ast.TreeNode]bool
	loopDepth       int
	pendingFuncName string
}

func NewAnalyzer() *Analyzer {
	globalScope := NewScope(nil)

	builtinSigs := make(map[string]*builtinSignature)
	methodSigs := make(map[builtins.TypeKey]map[string]*builtinSignature)
	funcs := make(map[string]*FunctionType)
	for _, spec := range builtins.Catalog() {
		paramTypes := make([]Type, 0, len(spec.ParamTypes))
		for _, key := range spec.ParamTypes {
			paramTypes = append(paramTypes, mapBuiltinTypeKey(key))
		}
		funcType := &FunctionType{ParamTypes: paramTypes, ReturnType: mapBuiltinTypeKey(spec.ReturnType)}
		sig := &builtinSignature{Type: funcType, MinArgs: spec.MinArgs, MaxArgs: spec.MaxArgs}

		if spec.ReceiverType == "" {
			if _, exists := builtinSigs[spec.Name]; !exists {
				globalScope.Define(spec.Name, funcType, false)
				funcs[spec.Name] = funcType
				builtinSigs[spec.Name] = sig
			}
		} else {
			if methodSigs[spec.ReceiverType] == nil {
				methodSigs[spec.ReceiverType] = make(map[string]*builtinSignature)
			}
			if _, exists := methodSigs[spec.ReceiverType][spec.Name]; !exists {
				methodSigs[spec.ReceiverType][spec.Name] = sig
			}
		}
	}

	return &Analyzer{
		currentScope:    globalScope,
		errors:          make([]string, 0),
		functions:       funcs,
		modules:         make(map[string]*Module),
		moduleAliases:   make(map[string]string),
		currentModule:   "",
		builtins:        builtinSigs,
		methods:         methodSigs,
		captures:        make(map[*ast.TreeNode][]string),
		callPlans:       make(map[*ast.TreeNode]*ir.CallPlan),
		returnValidated: make(map[*ast.TreeNode]bool),
	}
}

func (a *Analyzer) Errors() []string {
	return a.errors
}

func (a *Analyzer) addError(format string, args ...interface{}) {
	a.errors = append(a.errors, fmt.Sprintf(format, args...))
}

func (a *Analyzer) errorAt(node *ast.TreeNode, format string, args ...interface{}) {
	msg := fmt.Sprintf(format, args...)
	if node != nil && node.Token != nil {
		msg = fmt.Sprintf("line %d, col %d: %s", node.Token.Line, node.Token.Column, msg)
	}
	a.errors = append(a.errors, msg)
}

func (a *Analyzer) pushScope() {
	a.currentScope = NewScope(a.currentScope)
}

func (a *Analyzer) popScope() {
	a.currentScope = a.currentScope.Parent
}

// Analyze performs semantic analysis on the AST
func (a *Analyzer) Analyze(node *ast.TreeNode) Type {
	if node == nil {
		return TypeVoid
	}

	switch node.NodeType {
	case ast.CompilationUnitNode:
		return a.analyzeCompilationUnit(node)
	case ast.BlockNode:
		return a.analyzeBlock(node)
	case ast.FunctionNode:
		return a.analyzeFunction(node)
	case ast.FunctionCallNode:
		return a.analyzeFunctionCall(node)
	case ast.IfStatementNode:
		return a.analyzeIfStatement(node)
	case ast.WhenStatementNode:
		return a.analyzeWhenStatement(node)
	case ast.ForLoopNode:
		return a.analyzeForLoop(node)
	case ast.WhileLoopNode:
		return a.analyzeWhileLoop(node)
	case ast.IdentifierNode:
		return a.analyzeIdentifier(node)
	case ast.LiteralNode:
		return a.analyzeLiteral(node)
	case ast.OperatorNode:
		return a.analyzeOperator(node)
	case ast.PipeNode:
		return a.analyzePipe(node)
	case ast.TernaryNode:
		return a.analyzeTernary(node)
	case ast.ListNode:
		return a.analyzeList(node)
	case ast.VectorNode:
		return a.analyzeVector(node)
	case ast.DictNode:
		return a.analyzeDict(node)
	case ast.IndexNode:
		return a.analyzeIndex(node)
	case ast.ResultNode:
		return a.analyzeResult(node)
	case ast.VarDeclNode:
		return a.analyzeVarDecl(node)
	case ast.ModuleNode:
		return a.analyzeModule(node)
	case ast.UseNode:
		return a.analyzeUse(node)
	case ast.BreakNode:
		if a.loopDepth == 0 {
			a.errorAt(node, "[C-SCOPE] 'break' must be inside a for or while loop")
		}
		return TypeVoid
	case ast.ContinueNode:
		if a.loopDepth == 0 {
			a.errorAt(node, "[C-SCOPE] 'continue' must be inside a for or while loop")
		}
		return TypeVoid
	case ast.LambdaNode:
		return a.analyzeLambda(node)
	default:
		return TypeAny
	}
}
