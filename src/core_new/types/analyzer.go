package types

import (
	"fmt"
	"quark/ast"
	"quark/token"
)

// Analyzer performs semantic analysis on the AST
type Analyzer struct {
	currentScope *Scope
	errors       []string
	functions    map[string]*FunctionType // Track function signatures
}

func NewAnalyzer() *Analyzer {
	globalScope := NewScope(nil)

	// Define built-in functions
	builtins := map[string]*FunctionType{
		"print":  {ParamTypes: []Type{TypeAny}, ReturnType: TypeVoid},
		"println": {ParamTypes: []Type{TypeAny}, ReturnType: TypeVoid},
		"len":    {ParamTypes: []Type{TypeAny}, ReturnType: TypeInt},
		"str":    {ParamTypes: []Type{TypeAny}, ReturnType: TypeString},
		"int":    {ParamTypes: []Type{TypeAny}, ReturnType: TypeInt},
		"float":  {ParamTypes: []Type{TypeAny}, ReturnType: TypeFloat},
		"bool":   {ParamTypes: []Type{TypeAny}, ReturnType: TypeBool},
		"input":  {ParamTypes: []Type{}, ReturnType: TypeString},
	}

	funcs := make(map[string]*FunctionType)
	for name, typ := range builtins {
		globalScope.Define(name, typ, false)
		funcs[name] = typ
	}

	return &Analyzer{
		currentScope: globalScope,
		errors:       make([]string, 0),
		functions:    funcs,
	}
}

func (a *Analyzer) Errors() []string {
	return a.errors
}

func (a *Analyzer) addError(format string, args ...interface{}) {
	a.errors = append(a.errors, fmt.Sprintf(format, args...))
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
	case ast.DictNode:
		return a.analyzeDict(node)
	case ast.IndexNode:
		return a.analyzeIndex(node)
	default:
		return TypeAny
	}
}

func (a *Analyzer) analyzeCompilationUnit(node *ast.TreeNode) Type {
	var lastType Type = TypeVoid
	for _, child := range node.Children {
		lastType = a.Analyze(child)
	}
	return lastType
}

func (a *Analyzer) analyzeBlock(node *ast.TreeNode) Type {
	var lastType Type = TypeVoid
	for _, child := range node.Children {
		lastType = a.Analyze(child)
	}
	return lastType
}

func (a *Analyzer) analyzeFunction(node *ast.TreeNode) Type {
	if len(node.Children) < 3 {
		a.addError("invalid function definition")
		return TypeVoid
	}

	nameNode := node.Children[0]
	argsNode := node.Children[1]
	bodyNode := node.Children[2]

	funcName := nameNode.TokenLiteral()

	// Create function scope
	a.pushScope()

	// Define parameters
	paramTypes := make([]Type, 0)
	for _, param := range argsNode.Children {
		paramName := param.TokenLiteral()
		paramType := TypeAny // Type inference will refine this
		a.currentScope.Define(paramName, paramType, true)
		paramTypes = append(paramTypes, paramType)
	}

	// Analyze body to infer return type
	returnType := a.Analyze(bodyNode)

	a.popScope()

	// Create function type
	funcType := &FunctionType{
		ParamTypes: paramTypes,
		ReturnType: returnType,
	}

	// Define function in parent scope
	a.currentScope.Define(funcName, funcType, false)
	a.functions[funcName] = funcType

	return funcType
}

func (a *Analyzer) analyzeFunctionCall(node *ast.TreeNode) Type {
	if len(node.Children) < 2 {
		return TypeAny
	}

	funcNode := node.Children[0]
	argsNode := node.Children[1]

	// Get function name and type
	funcName := funcNode.TokenLiteral()
	sym := a.currentScope.Lookup(funcName)

	if sym == nil {
		a.addError("undefined function: %s", funcName)
		return TypeAny
	}

	funcType, ok := sym.Type.(*FunctionType)
	if !ok {
		a.addError("%s is not a function", funcName)
		return TypeAny
	}

	// Analyze arguments
	for _, arg := range argsNode.Children {
		a.Analyze(arg)
	}

	return funcType.ReturnType
}

func (a *Analyzer) analyzeIfStatement(node *ast.TreeNode) Type {
	if len(node.Children) < 2 {
		return TypeVoid
	}

	// Analyze condition
	condType := a.Analyze(node.Children[0])
	if !condType.Equals(TypeBool) && !condType.Equals(TypeAny) {
		// Allow any type as condition (truthy/falsy)
	}

	// Analyze if block
	a.pushScope()
	ifType := a.Analyze(node.Children[1])
	a.popScope()

	// Analyze elseif/else blocks
	var elseType Type = TypeVoid
	for i := 2; i < len(node.Children); i++ {
		child := node.Children[i]
		a.pushScope()
		elseType = a.Analyze(child)
		a.popScope()
	}

	// Return type is union of branches (simplified to any for now)
	if !ifType.Equals(elseType) {
		return TypeAny
	}
	return ifType
}

func (a *Analyzer) analyzeWhenStatement(node *ast.TreeNode) Type {
	if len(node.Children) < 2 {
		return TypeVoid
	}

	// Analyze expression being matched
	a.Analyze(node.Children[0])

	// Analyze patterns
	var resultType Type = TypeAny
	for i := 1; i < len(node.Children); i++ {
		pattern := node.Children[i]
		if pattern.NodeType == ast.PatternNode && len(pattern.Children) > 0 {
			// Last child of pattern is the result expression
			result := pattern.Children[len(pattern.Children)-1]
			resultType = a.Analyze(result)
		}
	}

	return resultType
}

func (a *Analyzer) analyzeForLoop(node *ast.TreeNode) Type {
	if len(node.Children) < 3 {
		return TypeVoid
	}

	varNode := node.Children[0]
	iterNode := node.Children[1]
	bodyNode := node.Children[2]

	// Analyze iterable
	iterType := a.Analyze(iterNode)

	// Create loop scope and define loop variable
	a.pushScope()

	varName := varNode.TokenLiteral()
	var varType Type = TypeInt // For range, loop variable is int
	if listType, ok := iterType.(*ListType); ok {
		varType = listType.ElementType
	}
	a.currentScope.Define(varName, varType, false)

	// Analyze body
	a.Analyze(bodyNode)

	a.popScope()

	return TypeVoid
}

func (a *Analyzer) analyzeWhileLoop(node *ast.TreeNode) Type {
	if len(node.Children) < 2 {
		return TypeVoid
	}

	// Analyze condition
	a.Analyze(node.Children[0])

	// Analyze body
	a.pushScope()
	a.Analyze(node.Children[1])
	a.popScope()

	return TypeVoid
}

func (a *Analyzer) analyzeIdentifier(node *ast.TreeNode) Type {
	name := node.TokenLiteral()

	// Wildcard is always valid
	if name == "_" {
		return TypeAny
	}

	sym := a.currentScope.Lookup(name)
	if sym == nil {
		// Could be a forward reference or undefined - allow for now
		return TypeAny
	}

	return sym.Type
}

func (a *Analyzer) analyzeLiteral(node *ast.TreeNode) Type {
	if node.Token == nil {
		return TypeAny
	}

	switch node.Token.Type {
	case token.INT:
		return TypeInt
	case token.FLOAT:
		return TypeFloat
	case token.STRING:
		return TypeString
	case token.TRUE, token.FALSE:
		return TypeBool
	case token.NULL:
		return TypeNull
	default:
		return TypeAny
	}
}

func (a *Analyzer) analyzeOperator(node *ast.TreeNode) Type {
	if node.Token == nil || len(node.Children) == 0 {
		return TypeAny
	}

	op := node.Token.Type

	// Unary operators
	if len(node.Children) == 1 {
		operandType := a.Analyze(node.Children[0])
		switch op {
		case token.MINUS:
			if IsNumeric(operandType) {
				return operandType
			}
			return TypeInt
		case token.BANG, token.NOT:
			return TypeBool
		}
		return operandType
	}

	// Binary operators
	leftType := a.Analyze(node.Children[0])
	rightType := a.Analyze(node.Children[1])

	switch op {
	case token.PLUS, token.MINUS, token.MULTIPLY, token.DIVIDE, token.MODULO, token.DOUBLESTAR:
		// Arithmetic operators
		if leftType.Equals(TypeFloat) || rightType.Equals(TypeFloat) {
			return TypeFloat
		}
		return TypeInt

	case token.LT, token.LTE, token.GT, token.GTE:
		return TypeBool

	case token.DEQ, token.NE:
		return TypeBool

	case token.AND, token.OR:
		return TypeBool

	case token.AMPER:
		return TypeInt

	case token.DOTDOT:
		// Range returns a special range type, but for simplicity use list
		return &ListType{ElementType: TypeInt}

	case token.EQUALS:
		// Assignment - define or update variable
		if node.Children[0].NodeType == ast.IdentifierNode {
			varName := node.Children[0].TokenLiteral()
			a.currentScope.Define(varName, rightType, true)
		}
		return rightType

	case token.DOT:
		// Member access - return any for now
		return TypeAny

	case token.COMMA:
		// Comma returns the right operand's type
		return rightType
	}

	return TypeAny
}

func (a *Analyzer) analyzePipe(node *ast.TreeNode) Type {
	if len(node.Children) < 2 {
		return TypeAny
	}

	// Left side is the input
	a.Analyze(node.Children[0])

	// Right side is the function (or function call)
	rightNode := node.Children[1]
	return a.Analyze(rightNode)
}

func (a *Analyzer) analyzeTernary(node *ast.TreeNode) Type {
	if len(node.Children) < 3 {
		return TypeAny
	}

	// condition, trueVal, falseVal
	a.Analyze(node.Children[0]) // condition
	trueType := a.Analyze(node.Children[1])
	falseType := a.Analyze(node.Children[2])

	if trueType.Equals(falseType) {
		return trueType
	}
	return TypeAny
}

func (a *Analyzer) analyzeList(node *ast.TreeNode) Type {
	if len(node.Children) == 0 {
		return &ListType{ElementType: TypeAny}
	}

	// Use first element's type as list element type
	elemType := a.Analyze(node.Children[0])
	for _, child := range node.Children[1:] {
		a.Analyze(child)
	}

	return &ListType{ElementType: elemType}
}

func (a *Analyzer) analyzeDict(node *ast.TreeNode) Type {
	return &DictType{KeyType: TypeString, ValueType: TypeAny}
}

func (a *Analyzer) analyzeIndex(node *ast.TreeNode) Type {
	if len(node.Children) < 2 {
		return TypeAny
	}

	targetType := a.Analyze(node.Children[0])
	a.Analyze(node.Children[1]) // index

	if listType, ok := targetType.(*ListType); ok {
		return listType.ElementType
	}
	if dictType, ok := targetType.(*DictType); ok {
		return dictType.ValueType
	}

	return TypeAny
}
