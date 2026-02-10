package types

import (
	"fmt"
	"quark/ast"
	"quark/token"
)

type builtinSignature struct {
	Type    *FunctionType
	MinArgs int
	MaxArgs int
}

// Module represents a defined module with its symbols
type Module struct {
	Name    string
	Scope   *Scope
	Symbols map[string]*Symbol
}

// Analyzer performs semantic analysis on the AST
type Analyzer struct {
	currentScope  *Scope
	errors        []string
	functions     map[string]*FunctionType // Track function signatures
	modules       map[string]*Module       // Track defined modules
	currentModule string                   // Current module being defined (empty if global)
	builtins      map[string]*builtinSignature
}

func NewAnalyzer() *Analyzer {
	globalScope := NewScope(nil)

	// NOTE: keep this list in sync with codegen/builtins.go
	builtinDefs := []struct {
		name       string
		minArgs    int
		maxArgs    int
		paramTypes []Type
		returnType Type
	}{
		{"print", 0, 1, []Type{TypeAny}, TypeVoid},
		{"println", 0, 1, []Type{TypeAny}, TypeVoid},
		{"input", 0, 1, []Type{TypeAny}, TypeString},
		{"len", 1, 1, []Type{TypeAny}, TypeInt},
		{"str", 1, 1, []Type{TypeAny}, TypeString},
		{"int", 1, 1, []Type{TypeAny}, TypeInt},
		{"float", 1, 1, []Type{TypeAny}, TypeFloat},
		{"bool", 1, 1, []Type{TypeAny}, TypeBool},
		{"range", 1, 3, []Type{TypeAny, TypeAny, TypeAny}, TypeAny},
		{"abs", 1, 1, []Type{TypeAny}, TypeAny},
		{"min", 2, 2, []Type{TypeAny, TypeAny}, TypeAny},
		{"max", 2, 2, []Type{TypeAny, TypeAny}, TypeAny},
		{"sqrt", 1, 1, []Type{TypeAny}, TypeFloat},
		{"floor", 1, 1, []Type{TypeAny}, TypeInt},
		{"ceil", 1, 1, []Type{TypeAny}, TypeInt},
		{"round", 1, 1, []Type{TypeAny}, TypeInt},
		{"upper", 1, 1, []Type{TypeString}, TypeString},
		{"lower", 1, 1, []Type{TypeString}, TypeString},
		{"trim", 1, 1, []Type{TypeString}, TypeString},
		{"contains", 2, 2, []Type{TypeString, TypeString}, TypeBool},
		{"startswith", 2, 2, []Type{TypeString, TypeString}, TypeBool},
		{"endswith", 2, 2, []Type{TypeString, TypeString}, TypeBool},
		{"replace", 3, 3, []Type{TypeString, TypeString, TypeString}, TypeString},
		{"concat", 2, 2, []Type{TypeAny, TypeAny}, TypeAny},
		{"push", 2, 2, []Type{TypeAny, TypeAny}, TypeAny},
		{"pop", 1, 1, []Type{TypeAny}, TypeAny},
		{"get", 2, 2, []Type{TypeAny, TypeInt}, TypeAny},
		{"set", 3, 3, []Type{TypeAny, TypeInt, TypeAny}, TypeAny},
		{"insert", 3, 3, []Type{TypeAny, TypeInt, TypeAny}, TypeAny},
		{"remove", 2, 2, []Type{TypeAny, TypeInt}, TypeAny},
		{"slice", 3, 3, []Type{TypeAny, TypeInt, TypeInt}, TypeAny},
		{"reverse", 1, 1, []Type{TypeAny}, TypeAny},
	}

	builtins := make(map[string]*builtinSignature)
	funcs := make(map[string]*FunctionType)
	for _, def := range builtinDefs {
		params := make([]Type, len(def.paramTypes))
		copy(params, def.paramTypes)
		funcType := &FunctionType{ParamTypes: params, ReturnType: def.returnType}
		globalScope.Define(def.name, funcType, false)
		funcs[def.name] = funcType
		builtins[def.name] = &builtinSignature{Type: funcType, MinArgs: def.minArgs, MaxArgs: def.maxArgs}
	}

	return &Analyzer{
		currentScope:  globalScope,
		errors:        make([]string, 0),
		functions:     funcs,
		modules:       make(map[string]*Module),
		currentModule: "",
		builtins:      builtins,
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

func (a *Analyzer) predeclareFunctions(nodes []*ast.TreeNode) {
	for _, child := range nodes {
		if child == nil {
			continue
		}
		if child.NodeType == ast.FunctionNode {
			a.declareFunctionSignature(child)
		}
	}
}

func (a *Analyzer) declareFunctionSignature(node *ast.TreeNode) *FunctionType {
	if len(node.Children) < 2 {
		return nil
	}
	nameNode := node.Children[0]
	argsNode := node.Children[1]
	funcName := nameNode.TokenLiteral()
	if funcName == "" {
		return nil
	}
	if existing := a.currentScope.LookupLocal(funcName); existing != nil {
		a.errorAt(nameNode, "symbol '%s' already defined in this scope", funcName)
		if ft, ok := existing.Type.(*FunctionType); ok {
			return ft
		}
		return nil
	}
	paramTypes := make([]Type, len(argsNode.Children))
	for i := range paramTypes {
		paramTypes[i] = TypeAny
	}
	funcType := &FunctionType{ParamTypes: paramTypes, ReturnType: TypeAny}
	a.currentScope.Define(funcName, funcType, false)
	a.functions[funcName] = funcType
	return funcType
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
	case ast.ModuleNode:
		return a.analyzeModule(node)
	case ast.UseNode:
		return a.analyzeUse(node)
	case ast.LambdaNode:
		return a.analyzeLambda(node)
	default:
		return TypeAny
	}
}

func (a *Analyzer) analyzeCompilationUnit(node *ast.TreeNode) Type {
	a.predeclareFunctions(node.Children)
	var lastType Type = TypeVoid
	for _, child := range node.Children {
		lastType = a.Analyze(child)
	}
	return lastType
}

func (a *Analyzer) analyzeBlock(node *ast.TreeNode) Type {
	a.pushScope()
	defer a.popScope()
	a.predeclareFunctions(node.Children)
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

	sym := a.currentScope.LookupLocal(funcName)
	var funcType *FunctionType
	if sym != nil {
		var ok bool
		funcType, ok = sym.Type.(*FunctionType)
		if !ok {
			a.errorAt(nameNode, "symbol '%s' already defined and is not a function", funcName)
			return TypeVoid
		}
	} else {
		funcType = a.declareFunctionSignature(node)
		if funcType == nil {
			return TypeVoid
		}
	}

	if len(funcType.ParamTypes) != len(argsNode.Children) {
		funcType.ParamTypes = make([]Type, len(argsNode.Children))
		for i := range funcType.ParamTypes {
			funcType.ParamTypes[i] = TypeAny
		}
	}

	// Create function scope for parameters and body
	a.pushScope()
	for _, param := range argsNode.Children {
		paramName := param.TokenLiteral()
		if paramName == "" {
			continue
		}
		a.currentScope.Define(paramName, TypeAny, true)
	}
	returnType := a.Analyze(bodyNode)
	a.popScope()

	funcType.ReturnType = returnType
	return funcType
}

func (a *Analyzer) analyzeFunctionCall(node *ast.TreeNode) Type {
	if len(node.Children) < 2 {
		a.errorAt(node, "invalid function call expression")
		return TypeAny
	}

	funcNode := node.Children[0]
	argsNode := node.Children[1]

	// Method calls (obj.method()) are handled dynamically at runtime
	if funcNode.NodeType == ast.OperatorNode && funcNode.Token != nil && funcNode.Token.Type == token.DOT {
		a.Analyze(funcNode)
		for _, arg := range argsNode.Children {
			a.Analyze(arg)
		}
		return TypeAny
	}

	funcExprType := a.Analyze(funcNode)
	argCount := len(argsNode.Children)
	for _, arg := range argsNode.Children {
		a.Analyze(arg)
	}

	if funcNode.NodeType == ast.IdentifierNode {
		name := funcNode.TokenLiteral()
		if sig, ok := a.builtins[name]; ok {
			if argCount < sig.MinArgs || argCount > sig.MaxArgs {
				a.errorAt(node, "builtin '%s' expects %d-%d arguments but got %d", name, sig.MinArgs, sig.MaxArgs, argCount)
			}
			return sig.Type.ReturnType
		}
	}

	funcType, ok := funcExprType.(*FunctionType)
	if !ok {
		if !isUnknownType(funcExprType) {
			a.errorAt(funcNode, "expression is not callable")
		}
		return TypeAny
	}

	if argCount != len(funcType.ParamTypes) {
		a.errorAt(node, "function expects %d arguments but got %d", len(funcType.ParamTypes), argCount)
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

	_ = condType
	resultType := a.Analyze(node.Children[1])

	for i := 2; i < len(node.Children); i++ {
		branchType := a.Analyze(node.Children[i])
		resultType = MergeTypes(resultType, branchType)
	}

	return resultType
}

func (a *Analyzer) analyzeWhenStatement(node *ast.TreeNode) Type {
	if len(node.Children) < 2 {
		return TypeVoid
	}

	// Analyze expression being matched
	a.Analyze(node.Children[0])

	// Analyze patterns
	var resultType Type = TypeVoid
	for i := 1; i < len(node.Children); i++ {
		pattern := node.Children[i]
		if pattern.NodeType == ast.PatternNode && len(pattern.Children) > 0 {
			// Last child of pattern is the result expression
			result := pattern.Children[len(pattern.Children)-1]
			branchType := a.Analyze(result)
			resultType = MergeTypes(resultType, branchType)
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
	var varType Type = TypeInt // Default for numeric ranges
	switch t := iterType.(type) {
	case *ListType:
		varType = t.ElementType
	case *DictType:
		varType = t.ValueType
	default:
		if !isUnknownType(iterType) {
			a.errorAt(iterNode, "value of type '%s' is not iterable", iterType.String())
		}
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
		a.errorAt(node, "undefined identifier '%s'", name)
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
		a.errorAt(node, "unsupported literal type: %s", node.Token.Type)
		return TypeAny
	}
}

func (a *Analyzer) analyzeOperator(node *ast.TreeNode) Type {
	if node.Token == nil || len(node.Children) == 0 {
		a.errorAt(node, "malformed operator expression")
		return TypeAny
	}

	op := node.Token.Type

	if op == token.DOT {
		// Member access: only analyze the target to avoid treating the member name as an identifier lookup
		if len(node.Children) >= 1 {
			a.Analyze(node.Children[0])
		}
		return TypeAny
	}

	// Unary operators
	if len(node.Children) == 1 {
		operandType := a.Analyze(node.Children[0])
		switch op {
		case token.MINUS:
			if IsNumeric(operandType) {
				return operandType
			}
			if isUnknownType(operandType) {
				return TypeAny
			}
			a.errorAt(node, "unary '-' expects numeric operand, got %s", operandType.String())
			return TypeAny
		case token.BANG, token.NOT:
			if isBoolLike(operandType) {
				return TypeBool
			}
			if isUnknownType(operandType) {
				return TypeAny
			}
			a.errorAt(node, "logical not expects boolean operand, got %s", operandType.String())
			return TypeAny
		}
		return operandType
	}

	// Assignment must be handled before analyzing the left side,
	// because the left side may be a new variable being defined.
	if op == token.EQUALS && len(node.Children) == 2 {
		target := node.Children[0]
		rightType := a.Analyze(node.Children[1])
		if target.NodeType != ast.IdentifierNode {
			a.errorAt(target, "left side of assignment must be an identifier")
			return rightType
		}
		varName := target.TokenLiteral()
		sym := a.currentScope.Lookup(varName)
		if sym == nil {
			a.currentScope.Define(varName, rightType, true)
		} else {
			if !CanAssign(sym.Type, rightType) && !isUnknownType(rightType) {
				a.errorAt(target, "cannot assign value of type '%s' to '%s'", rightType.String(), sym.Type.String())
			}
			sym.Type = rightType
		}
		return rightType
	}

	// Binary operators
	leftType := a.Analyze(node.Children[0])
	rightType := a.Analyze(node.Children[1])

	switch op {
	case token.PLUS, token.MINUS, token.MULTIPLY, token.DIVIDE, token.MODULO, token.DOUBLESTAR:
		if op == token.MODULO {
			if isIntLike(leftType) && isIntLike(rightType) {
				return TypeInt
			}
			if isUnknownType(leftType) || isUnknownType(rightType) {
				return TypeAny
			}
			a.errorAt(node, "operator '%%' requires integer operands, got %s and %s", leftType.String(), rightType.String())
			return TypeAny
		}
		if op == token.PLUS && isStringLike(leftType) && isStringLike(rightType) {
			return TypeString
		}
		if IsNumeric(leftType) && IsNumeric(rightType) {
			if op == token.DIVIDE {
				return TypeFloat
			}
			if leftType.Equals(TypeFloat) || rightType.Equals(TypeFloat) {
				return TypeFloat
			}
			if op == token.MODULO {
				return TypeInt
			}
			return TypeInt
		}
		if isUnknownType(leftType) || isUnknownType(rightType) {
			return TypeAny
		}
		a.errorAt(node, "operator '%s' requires numeric operands, got %s and %s", node.Token.Type.String(), leftType.String(), rightType.String())
		return TypeAny

	case token.LT, token.LTE, token.GT, token.GTE:
		if IsComparable(leftType) && IsComparable(rightType) {
			return TypeBool
		}
		if isUnknownType(leftType) || isUnknownType(rightType) {
			return TypeBool
		}
		a.errorAt(node, "comparison requires comparable operands, got %s and %s", leftType.String(), rightType.String())
		return TypeBool

	case token.DEQ, token.NE:
		return TypeBool

	case token.AND, token.OR:
		if isBoolLike(leftType) && isBoolLike(rightType) {
			return TypeBool
		}
		if isUnknownType(leftType) || isUnknownType(rightType) {
			return TypeBool
		}
		a.errorAt(node, "logical operator '%s' expects boolean operands, got %s and %s", node.Token.Type.String(), leftType.String(), rightType.String())
		return TypeBool

	case token.AMPER:
		if isIntLike(leftType) && isIntLike(rightType) {
			return TypeInt
		}
		if isUnknownType(leftType) || isUnknownType(rightType) {
			return TypeAny
		}
		a.errorAt(node, "'&' operator expects numeric operands, got %s and %s", leftType.String(), rightType.String())
		return TypeAny

	case token.COMMA:
		// Comma returns the right operand's type
		return rightType
	}

	a.errorAt(node, "unsupported operator '%s'", node.Token.Type.String())
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

	return MergeTypes(trueType, falseType)
}

func (a *Analyzer) analyzeList(node *ast.TreeNode) Type {
	if len(node.Children) == 0 {
		return &ListType{ElementType: TypeAny}
	}

	// Use first element's type as list element type
	elemType := a.Analyze(node.Children[0])
	for _, child := range node.Children[1:] {
		childType := a.Analyze(child)
		elemType = MergeTypes(elemType, childType)
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

	if !isUnknownType(targetType) {
		a.errorAt(node, "type '%s' is not indexable", targetType.String())
	}

	return TypeAny
}

func (a *Analyzer) analyzeModule(node *ast.TreeNode) Type {
	if len(node.Children) < 2 {
		a.errorAt(node, "invalid module definition")
		return TypeVoid
	}

	nameNode := node.Children[0]
	bodyNode := node.Children[1]

	moduleName := nameNode.TokenLiteral()

	// Check for duplicate module definition
	if _, exists := a.modules[moduleName]; exists {
		a.errorAt(nameNode, "module '%s' already defined", moduleName)
		return TypeVoid
	}

	// Create new scope for module
	moduleScope := NewScope(a.currentScope)
	oldScope := a.currentScope
	a.currentScope = moduleScope
	a.currentModule = moduleName

	// Analyze module body without introducing an extra block scope
	if bodyNode != nil && bodyNode.NodeType == ast.BlockNode {
		a.predeclareFunctions(bodyNode.Children)
		for _, child := range bodyNode.Children {
			a.Analyze(child)
		}
	} else {
		a.Analyze(bodyNode)
	}

	// Store module with its symbols
	module := &Module{
		Name:    moduleName,
		Scope:   moduleScope,
		Symbols: moduleScope.Symbols,
	}
	a.modules[moduleName] = module

	// Restore scope
	a.currentScope = oldScope
	a.currentModule = ""

	return TypeVoid
}

func (a *Analyzer) analyzeUse(node *ast.TreeNode) Type {
	if len(node.Children) < 1 {
		a.errorAt(node, "invalid use statement")
		return TypeVoid
	}

	nameNode := node.Children[0]
	moduleName := nameNode.TokenLiteral()

	// Look up module
	module, exists := a.modules[moduleName]
	if !exists {
		a.errorAt(nameNode, "undefined module '%s'", moduleName)
		return TypeVoid
	}

	// Import all symbols from module into current scope
	for name, sym := range module.Symbols {
		a.currentScope.Define(name, sym.Type, sym.Mutable)
	}

	return TypeVoid
}

// GetModules returns the list of defined modules (for codegen)
func (a *Analyzer) GetModules() map[string]*Module {
	return a.modules
}

func (a *Analyzer) analyzeLambda(node *ast.TreeNode) Type {
	if len(node.Children) < 2 {
		a.addError("invalid lambda expression")
		return TypeAny
	}

	argsNode := node.Children[0]
	bodyNode := node.Children[1]

	// Create lambda scope
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

	// Create and return function type
	return &FunctionType{
		ParamTypes: paramTypes,
		ReturnType: returnType,
	}
}

func isUnknownType(t Type) bool {
	if t == nil {
		return true
	}
	if t.Equals(TypeAny) {
		return true
	}
	if union, ok := t.(*UnionType); ok {
		for _, opt := range union.Options {
			if isUnknownType(opt) {
				return true
			}
		}
	}
	return false
}

func isBoolLike(t Type) bool {
	if t == nil {
		return false
	}
	if t.Equals(TypeBool) {
		return true
	}
	if union, ok := t.(*UnionType); ok {
		if len(union.Options) == 0 {
			return false
		}
		for _, opt := range union.Options {
			if !isBoolLike(opt) {
				return false
			}
		}
		return true
	}
	return isUnknownType(t)
}

func isStringLike(t Type) bool {
	if t == nil {
		return false
	}
	if t.Equals(TypeString) {
		return true
	}
	if union, ok := t.(*UnionType); ok {
		if len(union.Options) == 0 {
			return false
		}
		for _, opt := range union.Options {
			if !isStringLike(opt) {
				return false
			}
		}
		return true
	}
	return isUnknownType(t)
}

func isIntLike(t Type) bool {
	if t == nil {
		return false
	}
	if t.Equals(TypeInt) {
		return true
	}
	if union, ok := t.(*UnionType); ok {
		if len(union.Options) == 0 {
			return false
		}
		for _, opt := range union.Options {
			if !isIntLike(opt) {
				return false
			}
		}
		return true
	}
	return isUnknownType(t)
}
