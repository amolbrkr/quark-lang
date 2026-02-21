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

type paramSpec struct {
	name     string
	typeNode *ast.TreeNode
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
	captures      map[*ast.TreeNode][]string // Lambda node → captured variable names
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
		{"type", 1, 1, []Type{TypeAny}, TypeString},
		{"range", 1, 3, []Type{TypeAny, TypeAny, TypeAny}, &ListType{ElementType: TypeInt}},
		{"abs", 1, 1, []Type{TypeAny}, TypeAny},
		{"min", 1, 2, []Type{TypeAny, TypeAny}, TypeAny},
		{"max", 1, 2, []Type{TypeAny, TypeAny}, TypeAny},
		{"sum", 1, 1, []Type{TypeAny}, TypeAny},
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
		{"split", 2, 2, []Type{TypeString, TypeString}, &ListType{ElementType: TypeString}},
		{"push", 2, 2, []Type{TypeAny, TypeAny}, TypeAny},
		{"pop", 1, 1, []Type{TypeAny}, TypeAny},
		{"get", 2, 2, []Type{TypeAny, TypeInt}, TypeAny},
		{"set", 3, 3, []Type{TypeAny, TypeInt, TypeAny}, TypeAny},
		{"insert", 3, 3, []Type{TypeAny, TypeInt, TypeAny}, TypeAny},
		{"remove", 2, 2, []Type{TypeAny, TypeInt}, TypeAny},
		{"slice", 3, 3, []Type{TypeAny, TypeInt, TypeInt}, TypeAny},
		{"reverse", 1, 1, []Type{TypeAny}, TypeAny},
		{"dget", 2, 2, []Type{TypeAny, TypeAny}, TypeAny},
		{"dset", 3, 3, []Type{TypeAny, TypeAny, TypeAny}, TypeAny},
		{"fillna", 2, 2, []Type{TypeAny, TypeAny}, TypeAny},
		{"astype", 2, 2, []Type{TypeAny, TypeString}, TypeAny},
		{"to_vector", 1, 1, []Type{TypeAny}, TypeAny},
		{"cat_from_str", 1, 1, []Type{TypeAny}, TypeAny},
		{"cat_to_str", 1, 1, []Type{TypeAny}, TypeAny},
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
		captures:      make(map[*ast.TreeNode][]string),
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
	paramSpecs := collectParamSpecs(argsNode)
	paramTypes := make([]Type, len(paramSpecs))
	for i, spec := range paramSpecs {
		paramTypes[i] = a.resolveTypeNode(spec.typeNode)
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

	paramSpecs := collectParamSpecs(argsNode)
	if len(funcType.ParamTypes) != len(paramSpecs) {
		funcType.ParamTypes = make([]Type, len(paramSpecs))
	}
	for i, spec := range paramSpecs {
		funcType.ParamTypes[i] = a.resolveTypeNode(spec.typeNode)
	}

	// Create function scope for parameters and body
	a.pushScope()
	for i, spec := range paramSpecs {
		if spec.name == "" {
			continue
		}
		var paramType Type = TypeAny
		if i < len(funcType.ParamTypes) {
			paramType = funcType.ParamTypes[i]
		}
		a.currentScope.Define(spec.name, paramType, true)
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
		if len(funcNode.Children) >= 2 {
			targetType := a.Analyze(funcNode.Children[0])
			method := funcNode.Children[1].TokenLiteral()
			if targetType.Equals(TypeNull) {
				a.errorAt(funcNode.Children[0], "cannot call method '%s' on null", method)
			} else {
				switch t := targetType.(type) {
				case *ListType:
					switch method {
					case "push", "get", "remove", "concat", "set", "insert", "slice":
						// Valid list methods
					default:
						a.errorAt(funcNode.Children[1], "list has no method '%s'", method)
					}
				case *BasicType:
					if t.Name == "str" {
						switch method {
						case "contains", "startswith", "endswith", "concat", "replace":
							// Valid string methods
						default:
							a.errorAt(funcNode.Children[1], "string has no method '%s'", method)
						}
					} else if t.Name != "any" {
						a.errorAt(funcNode.Children[1], "type '%s' has no methods", t.Name)
					}
				case *DictType:
					a.errorAt(funcNode.Children[1], "dict has no methods")
				default:
					if !isUnknownType(targetType) {
						a.errorAt(funcNode.Children[1], "type '%s' has no methods", targetType.String())
					}
				}
			}
		}
		for _, arg := range argsNode.Children {
			a.Analyze(arg)
		}
		return TypeAny
	}

	funcExprType := a.Analyze(funcNode)
	argCount := len(argsNode.Children)
	argTypes := make([]Type, 0, argCount)
	for _, arg := range argsNode.Children {
		argTypes = append(argTypes, a.Analyze(arg))
	}

	if funcNode.NodeType == ast.IdentifierNode {
		name := funcNode.TokenLiteral()
		if sig, ok := a.builtins[name]; ok {
			if argCount < sig.MinArgs || argCount > sig.MaxArgs {
				a.errorAt(node, "builtin '%s' expects %d-%d arguments but got %d", name, sig.MinArgs, sig.MaxArgs, argCount)
			}
			return a.inferBuiltinReturnType(name, argTypes, node)
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
		if pattern.NodeType != ast.PatternNode || len(pattern.Children) == 0 {
			continue
		}

		resultExpr := pattern.Children[len(pattern.Children)-1]
		bindName, hasBinding := extractResultPatternBinding(pattern)

		if hasBinding && bindName != "" {
			a.pushScope()
			a.currentScope.Define(bindName, TypeAny, true)
			branchType := a.Analyze(resultExpr)
			a.popScope()
			resultType = MergeTypes(resultType, branchType)
			continue
		}

		branchType := a.Analyze(resultExpr)
		resultType = MergeTypes(resultType, branchType)
	}

	return resultType
}

func extractResultPatternBinding(pattern *ast.TreeNode) (string, bool) {
	if pattern == nil || len(pattern.Children) == 0 {
		return "", false
	}
	for i := 0; i < len(pattern.Children)-1; i++ {
		child := pattern.Children[i]
		if child.NodeType != ast.ResultPatternNode || len(child.Children) == 0 {
			continue
		}
		bindNode := child.Children[0]
		if bindNode == nil {
			return "", true
		}
		name := bindNode.TokenLiteral()
		if name == "_" {
			return "", true
		}
		return name, true
	}
	return "", false
}

func (a *Analyzer) analyzeResult(node *ast.TreeNode) Type {
	if len(node.Children) == 0 {
		return TypeAny
	}
	a.Analyze(node.Children[0])
	return TypeAny
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

	// Enforce supported iterable types for runtime/codegen compatibility
	if _, ok := iterType.(*ListType); !ok {
		if _, isVector := iterType.(*VectorType); !isVector {
			if !isUnknownType(iterType) {
				a.errorAt(iterNode, "for loop expects list or vector iterable, got %s", iterType.String())
				return TypeVoid
			}
		}
	}

	// Create loop scope and define loop variable
	a.pushScope()

	varName := varNode.TokenLiteral()
	var varType Type = TypeInt // Default for numeric ranges
	switch t := iterType.(type) {
	case *ListType:
		varType = t.ElementType
	case *VectorType:
		varType = t.ElementType
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
		if len(node.Children) < 2 {
			return TypeAny
		}
		targetType := a.Analyze(node.Children[0])
		member := node.Children[1].TokenLiteral()
		if targetType.Equals(TypeNull) {
			a.errorAt(node.Children[0], "cannot access member '%s' on null", member)
			return TypeAny
		}

		switch t := targetType.(type) {
		case *ListType:
			switch member {
			case "length", "size":
				return TypeInt
			case "empty":
				return TypeBool
			case "reverse", "clear":
				return targetType
			case "pop":
				return t.ElementType
			default:
				a.errorAt(node.Children[1], "list has no member '%s'", member)
				return TypeAny
			}
		case *DictType:
			if member == "length" || member == "size" {
				return TypeInt
			}
			return t.ValueType
		case *BasicType:
			if t.Name == "str" {
				switch member {
				case "length", "size":
					return TypeInt
				case "upper", "lower", "trim":
					return TypeString
				default:
					a.errorAt(node.Children[1], "string has no member '%s'", member)
					return TypeAny
				}
			}
			if t.Name == "any" {
				return TypeAny
			}
			a.errorAt(node.Children[1], "type '%s' has no members", t.Name)
			return TypeAny
		case *UnionType:
			return TypeAny
		default:
			if isUnknownType(targetType) {
				return TypeAny
			}
			a.errorAt(node.Children[1], "type '%s' has no members", targetType.String())
			return TypeAny
		}
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
			// All types support truthiness, so ! works on any value
			return TypeBool
		}
		return operandType
	}

	// Assignment must be handled before analyzing the left side,
	// because the left side may be a new variable being defined.
	if op == token.EQUALS && len(node.Children) == 2 {
		target := node.Children[0]
		rightType := a.Analyze(node.Children[1])
		// Allow member assignment: obj.member = value
		if target.NodeType == ast.OperatorNode && target.Token != nil && target.Token.Type == token.DOT {
			targetType := a.Analyze(target.Children[0])
			if targetType.Equals(TypeNull) {
				a.errorAt(target.Children[0], "cannot assign member on null")
				return rightType
			}
			if _, ok := targetType.(*DictType); ok {
				return rightType
			}
			if isUnknownType(targetType) {
				return rightType
			}
			a.errorAt(target, "only dict members are assignable")
			return rightType
		}
		// Allow index assignment: obj[key] = value
		if target.NodeType == ast.IndexNode {
			if len(target.Children) >= 2 {
				targetType := a.Analyze(target.Children[0])
				indexType := a.Analyze(target.Children[1])
				if targetType.Equals(TypeNull) {
					a.errorAt(target.Children[0], "cannot index null")
					return rightType
				}
				if _, ok := targetType.(*ListType); ok {
					if !isIntLike(indexType) && !isUnknownType(indexType) {
						a.errorAt(target.Children[1], "list index must be int, got %s", indexType.String())
					}
					return rightType
				}
				if targetType.Equals(TypeString) {
					a.errorAt(target, "strings are immutable")
					return rightType
				}
				if _, ok := targetType.(*DictType); ok {
					a.errorAt(target, "use dot access for dict assignment: d.key = value")
					return rightType
				}
				if !isUnknownType(targetType) {
					a.errorAt(target, "type '%s' is not index-assignable", targetType.String())
				}
			}
			return rightType
		}
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

	leftVec, leftIsVec := leftType.(*VectorType)
	rightVec, rightIsVec := rightType.(*VectorType)

	isNumericScalar := func(t Type) bool {
		return t.Equals(TypeInt) || t.Equals(TypeFloat)
	}

	// Vector arithmetic (MVP): +, -, *, /
	if op == token.PLUS || op == token.MINUS || op == token.MULTIPLY || op == token.DIVIDE {
		if leftIsVec && rightIsVec {
			if !IsNumeric(leftVec.ElementType) && !isUnknownType(leftVec.ElementType) {
				a.errorAt(node, "operator '%s' requires numeric vector operands, got %s", node.Token.Type.String(), leftType.String())
				return TypeAny
			}
			if !IsNumeric(rightVec.ElementType) && !isUnknownType(rightVec.ElementType) {
				a.errorAt(node, "operator '%s' requires numeric vector operands, got %s", node.Token.Type.String(), rightType.String())
				return TypeAny
			}
			if leftVec.ElementType.Equals(TypeFloat) || rightVec.ElementType.Equals(TypeFloat) {
				return &VectorType{ElementType: TypeFloat}
			}
			if leftVec.ElementType.Equals(TypeInt) && rightVec.ElementType.Equals(TypeInt) {
				if op == token.DIVIDE {
					return &VectorType{ElementType: TypeFloat}
				}
				return &VectorType{ElementType: TypeInt}
			}
			return &VectorType{ElementType: TypeAny}
		}
		if leftIsVec && (isNumericScalar(rightType) || isUnknownType(rightType)) {
			if !IsNumeric(leftVec.ElementType) && !isUnknownType(leftVec.ElementType) {
				a.errorAt(node, "operator '%s' requires numeric vector operands, got %s", node.Token.Type.String(), leftType.String())
				return TypeAny
			}
			if op == token.DIVIDE && leftVec.ElementType.Equals(TypeInt) && rightType.Equals(TypeInt) {
				return &VectorType{ElementType: TypeFloat}
			}
			return leftType
		}
		if rightIsVec && (isNumericScalar(leftType) || isUnknownType(leftType)) {
			if !IsNumeric(rightVec.ElementType) && !isUnknownType(rightVec.ElementType) {
				a.errorAt(node, "operator '%s' requires numeric vector operands, got %s", node.Token.Type.String(), rightType.String())
				return TypeAny
			}
			if op == token.DIVIDE && rightVec.ElementType.Equals(TypeInt) && leftType.Equals(TypeInt) {
				return &VectorType{ElementType: TypeFloat}
			}
			return rightType
		}
	}

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
		// Vector ordering comparisons (numeric vectors only)
		if leftIsVec || rightIsVec {
			if leftIsVec && !IsNumeric(leftVec.ElementType) && !isUnknownType(leftVec.ElementType) {
				a.errorAt(node, "ordering comparison requires numeric vector, got %s", leftType.String())
			}
			if rightIsVec && !IsNumeric(rightVec.ElementType) && !isUnknownType(rightVec.ElementType) {
				a.errorAt(node, "ordering comparison requires numeric vector, got %s", rightType.String())
			}
			if leftIsVec && !rightIsVec && !isNumericScalar(rightType) && !isUnknownType(rightType) {
				a.errorAt(node, "cannot compare vector with %s", rightType.String())
			}
			if rightIsVec && !leftIsVec && !isNumericScalar(leftType) && !isUnknownType(leftType) {
				a.errorAt(node, "cannot compare vector with %s", leftType.String())
			}
			return &VectorType{ElementType: TypeBool}
		}
		if IsComparable(leftType) && IsComparable(rightType) {
			return TypeBool
		}
		if isUnknownType(leftType) || isUnknownType(rightType) {
			return TypeBool
		}
		a.errorAt(node, "comparison requires comparable operands, got %s and %s", leftType.String(), rightType.String())
		return TypeBool

	case token.DEQ, token.NE:
		// Vector equality (all vector element types allowed)
		if leftIsVec || rightIsVec {
			return &VectorType{ElementType: TypeBool}
		}
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

	// Left side is the input value
	inputNode := node.Children[0]
	inputType := a.Analyze(inputNode)

	// Right side must be an explicit function call
	rightNode := node.Children[1]
	if rightNode.NodeType != ast.FunctionCallNode || len(rightNode.Children) < 2 {
		a.errorAt(rightNode, "pipe target must be a function call; use f(...) or obj.method(...)")
		return TypeAny
	}

	funcNode := rightNode.Children[0]
	argsNode := rightNode.Children[1]
	funcExprType := a.Analyze(funcNode)
	argTypes := make([]Type, 0, len(argsNode.Children))
	for _, arg := range argsNode.Children {
		argTypes = append(argTypes, a.Analyze(arg))
	}

	pipeArgCount := len(argsNode.Children) + 1 // +1 for the piped input

	if funcNode.NodeType == ast.IdentifierNode {
		name := funcNode.TokenLiteral()
		if sig, ok := a.builtins[name]; ok {
			if pipeArgCount < sig.MinArgs || pipeArgCount > sig.MaxArgs {
				a.errorAt(node, "builtin '%s' expects %d-%d arguments but got %d (including piped input)", name, sig.MinArgs, sig.MaxArgs, pipeArgCount)
			}
			pipeArgTypes := make([]Type, 0, pipeArgCount)
			pipeArgTypes = append(pipeArgTypes, inputType)
			pipeArgTypes = append(pipeArgTypes, argTypes...)
			return a.inferBuiltinReturnType(name, pipeArgTypes, node)
		}
	}

	if funcType, ok := funcExprType.(*FunctionType); ok {
		if pipeArgCount != len(funcType.ParamTypes) {
			a.errorAt(node, "function expects %d arguments but got %d (including piped input)", len(funcType.ParamTypes), pipeArgCount)
		}
		return funcType.ReturnType
	}

	return TypeAny
}

func (a *Analyzer) inferBuiltinReturnType(name string, argTypes []Type, callNode *ast.TreeNode) Type {
	if name != "to_vector" {
		if sig, ok := a.builtins[name]; ok {
			return sig.Type.ReturnType
		}
		return TypeAny
	}

	if len(argTypes) != 1 {
		return TypeAny
	}

	srcType := argTypes[0]
	if vec, ok := srcType.(*VectorType); ok {
		return &VectorType{ElementType: vec.ElementType}
	}

	listType, ok := srcType.(*ListType)
	if !ok {
		if !isUnknownType(srcType) {
			a.errorAt(callNode, "to_vector expects list or vector input, got %s", srcType.String())
		}
		return TypeAny
	}

	elem := listType.ElementType
	if elem.Equals(TypeInt) {
		return &VectorType{ElementType: TypeInt}
	}
	if elem.Equals(TypeFloat) {
		return &VectorType{ElementType: TypeFloat}
	}
	if elem.Equals(TypeString) {
		return &VectorType{ElementType: TypeString}
	}
	if union, ok := elem.(*UnionType); ok {
		onlyAllowed := true
		hasInt := false
		hasFloat := false
		hasString := false
		for _, opt := range union.Options {
			if opt.Equals(TypeInt) {
				hasInt = true
				continue
			}
			if opt.Equals(TypeFloat) {
				hasFloat = true
				continue
			}
			if opt.Equals(TypeString) {
				hasString = true
				continue
			}
			if opt.Equals(TypeNull) {
				continue
			}
			onlyAllowed = false
			break
		}
		if onlyAllowed {
			kinds := 0
			if hasInt {
				kinds++
			}
			if hasFloat {
				kinds++
			}
			if hasString {
				kinds++
			}
			if kinds > 1 {
				a.errorAt(callNode, "to_vector requires homogeneous list elements (all int, all float, or all str)")
				return TypeAny
			}
			if hasInt && !hasFloat {
				return &VectorType{ElementType: TypeInt}
			}
			if hasFloat && !hasInt {
				return &VectorType{ElementType: TypeFloat}
			}
			if hasString {
				return &VectorType{ElementType: TypeString}
			}
			return &VectorType{ElementType: TypeInt}
		}
	}

	if isUnknownType(elem) {
		return TypeAny
	}

	a.errorAt(callNode, "to_vector requires list elements of type int, float, or str, got %s", elem.String())
	return TypeAny
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

func (a *Analyzer) analyzeVector(node *ast.TreeNode) Type {
	if len(node.Children) == 0 {
		return &VectorType{ElementType: TypeFloat}
	}

	var elemType Type
	for _, child := range node.Children {
		childType := a.Analyze(child)
		if isUnknownType(childType) {
			elemType = MergeTypes(elemType, childType)
			continue
		}
		if !childType.Equals(TypeInt) && !childType.Equals(TypeFloat) && !childType.Equals(TypeString) {
			a.errorAt(child, "vector elements must be homogeneous int, float, or str, got %s", childType.String())
			elemType = MergeTypes(elemType, TypeAny)
			continue
		}
		if elemType == nil || elemType.Equals(TypeAny) {
			elemType = childType
			continue
		}
		if !elemType.Equals(childType) {
			a.errorAt(child, "vector literal requires homogeneous element types; found %s and %s", elemType.String(), childType.String())
			elemType = TypeAny
		}
	}

	if elemType == nil {
		return &VectorType{ElementType: TypeFloat}
	}
	return &VectorType{ElementType: elemType}
}

func (a *Analyzer) analyzeDict(node *ast.TreeNode) Type {
	if len(node.Children) == 0 {
		return &DictType{KeyType: TypeString, ValueType: TypeAny}
	}

	seenKeys := make(map[string]struct{})
	var valueType Type

	for _, pair := range node.Children {
		if pair == nil || len(pair.Children) < 2 {
			a.errorAt(node, "invalid dict entry")
			continue
		}
		keyNode := pair.Children[0]
		valueNode := pair.Children[1]

		keyType := a.Analyze(keyNode)
		if !keyType.Equals(TypeString) && !isUnknownType(keyType) {
			a.errorAt(keyNode, "dict keys must be str, got %s", keyType.String())
		}

		if keyNode != nil && keyNode.Token != nil && keyNode.Token.Type == token.STRING {
			key := keyNode.Token.Literal
			if _, exists := seenKeys[key]; exists {
				a.errorAt(keyNode, "duplicate dict key '%s'", key)
			} else {
				seenKeys[key] = struct{}{}
			}
		}

		childType := a.Analyze(valueNode)
		if valueType == nil {
			valueType = childType
		} else {
			valueType = MergeTypes(valueType, childType)
		}
	}

	if valueType == nil {
		valueType = TypeAny
	}

	return &DictType{KeyType: TypeString, ValueType: valueType}
}

func (a *Analyzer) analyzeIndex(node *ast.TreeNode) Type {
	if len(node.Children) < 2 {
		return TypeAny
	}

	targetType := a.Analyze(node.Children[0])
	indexType := a.Analyze(node.Children[1])

	// Vector indexing: scalar int or boolean mask
	if vecType, ok := targetType.(*VectorType); ok {
		if isIntLike(indexType) || isUnknownType(indexType) {
			// Scalar index returns element type as scalar
			return vecType.ElementType
		}
		if idxVec, ok := indexType.(*VectorType); ok {
			if idxVec.ElementType.Equals(TypeBool) || isUnknownType(idxVec.ElementType) {
				// Boolean mask: returns same vector type (filtered)
				return vecType
			}
			a.errorAt(node.Children[1], "vector mask index requires bool vector, got %s", indexType.String())
			return vecType
		}
		a.errorAt(node.Children[1], "vector index must be int or bool vector, got %s", indexType.String())
		return TypeAny
	}

	if listType, ok := targetType.(*ListType); ok {
		if !isIntLike(indexType) && !isUnknownType(indexType) {
			a.errorAt(node.Children[1], "list index must be int, got %s", indexType.String())
		}
		return listType.ElementType
	}
	if targetType.Equals(TypeString) {
		if !isIntLike(indexType) && !isUnknownType(indexType) {
			a.errorAt(node.Children[1], "string index must be int, got %s", indexType.String())
		}
		return TypeString
	}
	if _, ok := targetType.(*DictType); ok {
		a.errorAt(node, "use dot access for dicts: d.key instead of d['key']")
		return TypeAny
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

// GetCaptures returns the captured variable names for each lambda node
func (a *Analyzer) GetCaptures() map[*ast.TreeNode][]string {
	return a.captures
}

// collectFreeVars walks the AST body to find identifiers that are free variables
// (not parameters, not builtins, not locally defined, but defined in an enclosing scope)
func (a *Analyzer) collectFreeVars(node *ast.TreeNode, lambdaScope *Scope, params map[string]bool, seen map[string]bool, result *[]string) {
	if node == nil {
		return
	}
	if node.NodeType == ast.IdentifierNode {
		name := node.TokenLiteral()
		if name == "_" || params[name] || seen[name] {
			return
		}
		if _, isBuiltin := a.builtins[name]; isBuiltin {
			return
		}
		// Check: is it defined in the lambda's own scope? If so, not a capture
		if lambdaScope.LookupLocal(name) != nil {
			return
		}
		// It must come from a parent scope
		if lambdaScope.Parent != nil && lambdaScope.Parent.Lookup(name) != nil {
			seen[name] = true
			*result = append(*result, name)
		}
		return
	}
	// For nested lambdas: walk their body to find variables from OUR enclosing
	// scope that they reference. We must capture those vars so nested lambdas
	// can access them through our closure.
	if node.NodeType == ast.LambdaNode {
		// Merge our params with nested lambda's params — skip both
		mergedParams := make(map[string]bool)
		for k, v := range params {
			mergedParams[k] = v
		}
		if len(node.Children) >= 1 {
			for _, p := range node.Children[0].Children {
				name := p.TokenLiteral()
				if name != "" {
					mergedParams[name] = true
				}
			}
		}
		if len(node.Children) >= 2 {
			a.collectFreeVars(node.Children[1], lambdaScope, mergedParams, seen, result)
		}
		return
	}
	for _, child := range node.Children {
		a.collectFreeVars(child, lambdaScope, params, seen, result)
	}
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

	// Define parameters and collect param names
	paramSpecs := collectParamSpecs(argsNode)
	paramTypes := make([]Type, 0, len(paramSpecs))
	paramNames := make(map[string]bool)
	for _, spec := range paramSpecs {
		if spec.name == "" {
			continue
		}
		paramType := a.resolveTypeNode(spec.typeNode)
		a.currentScope.Define(spec.name, paramType, true)
		paramTypes = append(paramTypes, paramType)
		paramNames[spec.name] = true
	}

	lambdaScope := a.currentScope

	// Analyze body to infer return type
	returnType := a.Analyze(bodyNode)

	// Compute free variables (captured from enclosing scopes)
	freeVars := []string{}
	seen := map[string]bool{}
	a.collectFreeVars(bodyNode, lambdaScope, paramNames, seen, &freeVars)
	if len(freeVars) > 0 {
		a.captures[node] = freeVars
	}

	a.popScope()

	// Create and return function type
	return &FunctionType{
		ParamTypes: paramTypes,
		ReturnType: returnType,
	}
}

func (a *Analyzer) analyzeVarDecl(node *ast.TreeNode) Type {
	if len(node.Children) < 3 {
		a.errorAt(node, "invalid typed declaration")
		return TypeAny
	}

	nameNode := node.Children[0]
	typeNode := node.Children[1]
	valueNode := node.Children[2]
	varName := nameNode.TokenLiteral()

	declType := a.resolveTypeNode(typeNode)
	valueType := a.Analyze(valueNode)
	if !CanAssign(declType, valueType) && !isUnknownType(valueType) {
		a.errorAt(nameNode, "cannot assign value of type '%s' to '%s'", valueType.String(), declType.String())
	}

	if existing := a.currentScope.LookupLocal(varName); existing != nil {
		a.errorAt(nameNode, "symbol '%s' already defined in this scope", varName)
		return declType
	}

	a.currentScope.Define(varName, declType, true)
	return declType
}

func collectParamSpecs(argsNode *ast.TreeNode) []paramSpec {
	if argsNode == nil {
		return nil
	}
	specs := make([]paramSpec, 0, len(argsNode.Children))
	for _, child := range argsNode.Children {
		if child == nil {
			continue
		}
		switch child.NodeType {
		case ast.ParameterNode:
			nameNode := (*ast.TreeNode)(nil)
			typeNode := (*ast.TreeNode)(nil)
			if len(child.Children) > 0 {
				nameNode = child.Children[0]
			}
			if len(child.Children) > 1 {
				typeNode = child.Children[1]
			}
			name := ""
			if nameNode != nil {
				name = nameNode.TokenLiteral()
			}
			specs = append(specs, paramSpec{name: name, typeNode: typeNode})
		case ast.IdentifierNode:
			name := child.TokenLiteral()
			specs = append(specs, paramSpec{name: name})
		}
	}
	return specs
}

func (a *Analyzer) resolveTypeNode(node *ast.TreeNode) Type {
	if node == nil {
		return TypeAny
	}
	if node.NodeType != ast.TypeNode {
		return TypeAny
	}

	name := node.TokenLiteral()
	switch name {
	case "int":
		return TypeInt
	case "float":
		return TypeFloat
	case "str":
		return TypeString
	case "bool":
		return TypeBool
	case "null":
		return TypeNull
	case "any":
		return TypeAny
	case "list":
		return &ListType{ElementType: TypeAny}
	case "dict":
		return &DictType{KeyType: TypeAny, ValueType: TypeAny}
	case "vector":
		return &VectorType{ElementType: TypeFloat}
	default:
		a.errorAt(node, "unknown type '%s'", name)
		return TypeAny
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
