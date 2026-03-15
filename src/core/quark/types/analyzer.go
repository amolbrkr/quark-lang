package types

import (
	"fmt"
	"quark/ast"
	"quark/builtins"
	"quark/ir"
	"quark/token"
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
	currentModule   string                   // Current module being defined (empty if global)
	builtins        map[string]*builtinSignature
	captures        map[*ast.TreeNode][]string // Lambda node → captured variable names
	callPlans       map[*ast.TreeNode]*ir.CallPlan
	returnValidated map[*ast.TreeNode]bool
	loopDepth       int    // >0 when inside for/while loop (for break/continue validation)
	pendingFuncName string // Set when analyzing a lambda assigned to a named variable
}

func NewAnalyzer() *Analyzer {
	globalScope := NewScope(nil)

	builtinSigs := make(map[string]*builtinSignature)
	funcs := make(map[string]*FunctionType)
	for _, spec := range builtins.Catalog() {
		paramTypes := make([]Type, 0, len(spec.ParamTypes))
		for _, key := range spec.ParamTypes {
			paramTypes = append(paramTypes, mapBuiltinTypeKey(key))
		}
		funcType := &FunctionType{ParamTypes: paramTypes, ReturnType: mapBuiltinTypeKey(spec.ReturnType)}
		globalScope.Define(spec.Name, funcType, false)
		funcs[spec.Name] = funcType
		builtinSigs[spec.Name] = &builtinSignature{Type: funcType, MinArgs: spec.MinArgs, MaxArgs: spec.MaxArgs}
	}

	return &Analyzer{
		currentScope:    globalScope,
		errors:          make([]string, 0),
		functions:       funcs,
		modules:         make(map[string]*Module),
		currentModule:   "",
		builtins:        builtinSigs,
		captures:        make(map[*ast.TreeNode][]string),
		callPlans:       make(map[*ast.TreeNode]*ir.CallPlan),
		returnValidated: make(map[*ast.TreeNode]bool),
	}
}

func mapBuiltinTypeKey(key builtins.TypeKey) Type {
	switch key {
	case builtins.TypeInt:
		return TypeInt
	case builtins.TypeFloat:
		return TypeFloat
	case builtins.TypeString:
		return TypeString
	case builtins.TypeBool:
		return TypeBool
	case builtins.TypeVoid:
		return TypeVoid
	case builtins.TypeListAny:
		return &ListType{ElementType: TypeAny}
	case builtins.TypeListInt:
		return &ListType{ElementType: TypeInt}
	case builtins.TypeListString:
		return &ListType{ElementType: TypeString}
	case builtins.TypeDictAny:
		return &DictType{KeyType: TypeAny, ValueType: TypeAny}
	case builtins.TypeVectorAny:
		return &VectorType{ElementType: TypeAny}
	default:
		return TypeAny
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
			continue
		}
		if isFunctionBindingAssignment(child) {
			a.declareFunctionAssignmentSignature(child)
		}
	}
}

func isFunctionBindingAssignment(node *ast.TreeNode) bool {
	if node == nil || node.NodeType != ast.OperatorNode || node.Token == nil || node.Token.Type != token.EQUALS {
		return false
	}
	if len(node.Children) != 2 {
		return false
	}
	left := node.Children[0]
	right := node.Children[1]
	if left == nil || right == nil {
		return false
	}
	return left.NodeType == ast.IdentifierNode && right.NodeType == ast.LambdaNode
}

func functionTypeFromLambdaNode(lambdaNode *ast.TreeNode) *FunctionType {
	if lambdaNode == nil || len(lambdaNode.Children) < 1 {
		return &FunctionType{ParamTypes: []Type{}, ReturnType: TypeAny}
	}
	argsNode := lambdaNode.Children[0]
	paramSpecs := collectParamSpecs(argsNode)
	paramTypes := make([]Type, len(paramSpecs))
	defaultCount := 0
	defaultValues := make([]*DefaultValueInfo, len(paramSpecs))
	for i, spec := range paramSpecs {
		if spec.typeNode == nil {
			// If no type annotation but there's a default, infer from default literal
			if spec.defaultValue != nil {
				paramTypes[i] = inferLiteralType(spec.defaultValue)
			} else {
				paramTypes[i] = TypeAny
			}
		} else {
			typeName := spec.typeNode.TokenLiteral()
			switch typeName {
			case "int":
				paramTypes[i] = TypeInt
			case "float":
				paramTypes[i] = TypeFloat
			case "str":
				paramTypes[i] = TypeString
			case "bool":
				paramTypes[i] = TypeBool
			case "null":
				paramTypes[i] = TypeNull
			case "list":
				paramTypes[i] = &ListType{ElementType: TypeAny}
			case "dict":
				paramTypes[i] = &DictType{KeyType: TypeAny, ValueType: TypeAny}
			case "vector":
				paramTypes[i] = &VectorType{ElementType: TypeAny}
			default:
				paramTypes[i] = TypeAny
			}
		}
		if spec.defaultValue != nil {
			defaultCount++
			defaultValues[i] = &DefaultValueInfo{Node: spec.defaultValue}
		}
	}

	// Resolve annotated return type
	var annotatedReturnType Type
	if lambdaNode.ReturnType != nil {
		annotatedReturnType = resolveTypeNodeStatic(lambdaNode.ReturnType)
	}

	var returnType Type = TypeAny
	if annotatedReturnType != nil {
		returnType = annotatedReturnType
	}

	return &FunctionType{
		ParamTypes:          paramTypes,
		ReturnType:          returnType,
		AnnotatedReturnType: annotatedReturnType,
		DefaultCount:        defaultCount,
		DefaultValues:       defaultValues,
	}
}

// resolveTypeNodeStatic resolves a type node without an analyzer instance (for pre-declaration)
func resolveTypeNodeStatic(node *ast.TreeNode) Type {
	if node == nil || node.NodeType != ast.TypeNode {
		return nil
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
	case "result":
		return &ResultType{OkType: TypeAny, ErrType: TypeAny}
	default:
		return TypeAny
	}
}

// inferLiteralType infers the type from a literal AST node (for default param type inference)
func inferLiteralType(node *ast.TreeNode) Type {
	if node == nil {
		return TypeAny
	}
	if node.NodeType == ast.LiteralNode && node.Token != nil {
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
			return TypeAny // null default doesn't constrain the parameter type
		}
	}
	// Unary minus on numeric literal
	if node.NodeType == ast.OperatorNode && node.Token != nil && node.Token.Type == token.MINUS && len(node.Children) == 1 {
		return inferLiteralType(node.Children[0])
	}
	// Empty list literal
	if node.NodeType == ast.ListNode {
		return &ListType{ElementType: TypeAny}
	}
	return TypeAny
}

func (a *Analyzer) declareFunctionAssignmentSignature(node *ast.TreeNode) *FunctionType {
	if !isFunctionBindingAssignment(node) {
		return nil
	}

	nameNode := node.Children[0]
	lambdaNode := node.Children[1]
	funcName := nameNode.TokenLiteral()
	if funcName == "" {
		return nil
	}

	if existing := a.currentScope.LookupLocal(funcName); existing != nil {
		if ft, ok := existing.Type.(*FunctionType); ok {
			return ft
		}
		a.errorAt(nameNode, "symbol '%s' already defined and is not a function", funcName)
		return nil
	}

	funcType := functionTypeFromLambdaNode(lambdaNode)
	a.currentScope.Define(funcName, funcType, true)
	a.functions[funcName] = funcType
	return funcType
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
	defaultCount := 0
	defaultValues := make([]*DefaultValueInfo, len(paramSpecs))
	for i, spec := range paramSpecs {
		if spec.typeNode != nil {
			paramTypes[i] = a.resolveTypeNode(spec.typeNode)
		} else if spec.defaultValue != nil {
			paramTypes[i] = inferLiteralType(spec.defaultValue)
		} else {
			paramTypes[i] = TypeAny
		}
		if spec.defaultValue != nil {
			defaultCount++
			defaultValues[i] = &DefaultValueInfo{Node: spec.defaultValue}
		}
	}

	var returnType Type = TypeAny
	// This is called for FunctionNode (not desugared). For desugared (assignment),
	// declareFunctionAssignmentSignature handles it via functionTypeFromLambdaNode.
	funcType := &FunctionType{
		ParamTypes:    paramTypes,
		ReturnType:    returnType,
		DefaultCount:  defaultCount,
		DefaultValues: defaultValues,
	}
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
	// Functions create a new scope where break/continue are invalid
	savedLoopDepth := a.loopDepth
	a.loopDepth = 0
	defer func() { a.loopDepth = savedLoopDepth }()

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
		if spec.typeNode != nil {
			funcType.ParamTypes[i] = a.resolveTypeNode(spec.typeNode)
		} else if spec.defaultValue != nil {
			funcType.ParamTypes[i] = inferLiteralType(spec.defaultValue)
		} else {
			funcType.ParamTypes[i] = TypeAny
		}
	}

	// Validate default value types against parameter types
	a.validateDefaultValues(paramSpecs, funcType, nameNode)

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

	// Validate return type annotation if present
	a.validateReturnType(funcType, returnType, funcName, nameNode)

	funcType.ReturnType = returnType
	return funcType
}

// checkArgTypes validates argument types against parameter types using the knowability rule
// from policy §3.3: skip if paramType is any/unknown; skip if argType is any/unknown
// (deferred to runtime); otherwise CanAssign must hold, else C-TYPE.
func (a *Analyzer) checkArgTypes(calleeName string, paramTypes []Type, argTypes []Type, argNodes []*ast.TreeNode) {
	for i, argType := range argTypes {
		if i >= len(paramTypes) {
			break
		}
		paramType := paramTypes[i]
		if paramType.Equals(TypeAny) || isUnknownType(paramType) {
			continue
		}
		if isUnknownType(argType) {
			continue
		}
		if !CanAssign(paramType, argType) {
			var errorNode *ast.TreeNode
			if i < len(argNodes) {
				errorNode = argNodes[i]
			}
			a.errorAt(errorNode, "argument %d of '%s' expects %s, got %s", i+1, calleeName, paramType.String(), argType.String())
		}
	}
}

func (a *Analyzer) analyzeFunctionCall(node *ast.TreeNode) Type {
	if len(node.Children) < 2 {
		a.errorAt(node, "invalid function call expression")
		return TypeAny
	}

	funcNode := node.Children[0]
	argsNode := node.Children[1]

	// Dot-call syntax is not supported — reject with diagnostic
	if funcNode.NodeType == ast.OperatorNode && funcNode.Token != nil && funcNode.Token.Type == token.DOT {
		if len(funcNode.Children) >= 2 {
			method := funcNode.Children[1].TokenLiteral()
			a.Analyze(funcNode.Children[0])
			a.errorAt(funcNode, "dot-call syntax is not supported; use %s(entity, ...) instead", method)
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
			a.callPlans[node] = &ir.CallPlan{Kind: ir.CallBuiltin, CalleeName: name, MinArity: sig.MinArgs, MaxArity: sig.MaxArgs, Dispatch: ir.DispatchBuiltin, RuntimeSymbol: builtinsRuntimeName(name)}
			if argCount < sig.MinArgs || argCount > sig.MaxArgs {
				a.errorAt(node, "builtin '%s' expects %d-%d arguments but got %d", name, sig.MinArgs, sig.MaxArgs, argCount)
			}
			a.checkArgTypes(name, sig.Type.ParamTypes, argTypes, argsNode.Children)
			a.callPlans[node].ArgTypesChecked = true
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

	minArity := funcType.MinArity()
	maxArity := len(funcType.ParamTypes)
	defaultNodes := defaultNodesFromFunctionType(funcType, argCount)
	dispatch := ir.DispatchClosure
	runtimeSymbol := ""
	if funcNode.NodeType == ast.IdentifierNode {
		name := funcNode.TokenLiteral()
		if sym := a.currentScope.Lookup(name); sym != nil && !sym.Mutable {
			if _, exists := a.functions[name]; exists {
				dispatch = ir.DispatchDirect
				runtimeSymbol = "quark_" + name
			}
		}
	}
	a.callPlans[node] = &ir.CallPlan{
		Kind:          ir.CallFunctionValue,
		CalleeName:    calleeNameFromNode(funcNode),
		MinArity:      minArity,
		MaxArity:      maxArity,
		Dispatch:      dispatch,
		RuntimeSymbol: runtimeSymbol,
		DefaultNodes:  defaultNodes,
	}
	if argCount < minArity || argCount > maxArity {
		if minArity == maxArity {
			a.errorAt(node, "function expects %d arguments but got %d", maxArity, argCount)
		} else {
			a.errorAt(node, "function expects %d-%d arguments but got %d", minArity, maxArity, argCount)
		}
	}

	calleeName := "function"
	if funcNode.NodeType == ast.IdentifierNode {
		calleeName = funcNode.TokenLiteral()
	}
	a.checkArgTypes(calleeName, funcType.ParamTypes, argTypes, argsNode.Children)
	a.callPlans[node].ArgTypesChecked = true

	return funcType.ReturnType
}

func builtinsRuntimeName(name string) string {
	if spec, ok := builtins.Lookup(name); ok {
		return spec.Runtime
	}
	return ""
}

func calleeNameFromNode(node *ast.TreeNode) string {
	if node == nil {
		return "function"
	}
	if node.NodeType == ast.IdentifierNode {
		name := node.TokenLiteral()
		if name != "" {
			return name
		}
	}
	return "function"
}

func (a *Analyzer) analyzeIfStatement(node *ast.TreeNode) Type {
	if len(node.Children) < 2 {
		return TypeVoid
	}

	// Analyze condition — must be bool (policy §3.1)
	condType := a.Analyze(node.Children[0])
	if !isBoolLike(condType) && !isUnknownType(condType) {
		a.errorAt(node.Children[0], "condition must be bool, got %s; use a comparison or explicit to_bool()", condType.String())
	}

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
	matchType := a.Analyze(node.Children[0])
	resultMatchType, isResultMatch := matchType.(*ResultType)

	// Analyze patterns
	var resultType Type = TypeVoid
	for i := 1; i < len(node.Children); i++ {
		pattern := node.Children[i]
		if pattern.NodeType != ast.PatternNode || len(pattern.Children) == 0 {
			continue
		}

		resultExpr := pattern.Children[len(pattern.Children)-1]
		bindName, hasBinding, bindingIsErr, resultPatternNode := extractResultPatternBinding(pattern)

		if hasBinding && !isResultMatch && !isUnknownType(matchType) {
			a.errorAt(resultPatternNode, "result pattern requires result value, got %s", matchType.String())
		}

		var bindingType Type = TypeAny
		if isResultMatch {
			if bindingIsErr {
				bindingType = resultMatchType.ErrType
			} else {
				bindingType = resultMatchType.OkType
			}
		}

		if hasBinding && bindName != "" {
			a.pushScope()
			a.currentScope.Define(bindName, bindingType, true)
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

func extractResultPatternBinding(pattern *ast.TreeNode) (string, bool, bool, *ast.TreeNode) {
	if pattern == nil || len(pattern.Children) == 0 {
		return "", false, false, nil
	}
	for i := 0; i < len(pattern.Children)-1; i++ {
		child := pattern.Children[i]
		if child.NodeType != ast.ResultPatternNode || len(child.Children) == 0 {
			continue
		}
		bindNode := child.Children[0]
		isErr := child.Token != nil && child.Token.Type == token.ERR
		if bindNode == nil {
			return "", true, isErr, child
		}
		name := bindNode.TokenLiteral()
		if name == "_" {
			return "", true, isErr, child
		}
		return name, true, isErr, child
	}
	return "", false, false, nil
}

func (a *Analyzer) analyzeResult(node *ast.TreeNode) Type {
	if len(node.Children) == 0 {
		return &ResultType{OkType: TypeAny, ErrType: TypeAny}
	}
	payloadType := a.Analyze(node.Children[0])
	if node.Token != nil && node.Token.Type == token.ERR {
		return &ResultType{OkType: TypeAny, ErrType: payloadType}
	}
	return &ResultType{OkType: payloadType, ErrType: TypeAny}
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
	a.loopDepth++
	a.pushScope()

	varName := varNode.TokenLiteral()
	var varType Type = TypeAny // Unknown iterable → unknown element type (policy §5.3)
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
	a.loopDepth--

	return TypeVoid
}

func (a *Analyzer) analyzeWhileLoop(node *ast.TreeNode) Type {
	if len(node.Children) < 2 {
		return TypeVoid
	}

	// Analyze condition — must be bool (policy §3.1)
	condType := a.Analyze(node.Children[0])
	if !isBoolLike(condType) && !isUnknownType(condType) {
		a.errorAt(node.Children[0], "condition must be bool, got %s; use a comparison or explicit to_bool()", condType.String())
	}

	// Analyze body
	a.loopDepth++
	a.pushScope()
	a.Analyze(node.Children[1])
	a.popScope()
	a.loopDepth--

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

	if len(node.Children) >= 2 {
		if node.Children[0] == nil || node.Children[1] == nil {
			a.errorAt(node, "malformed operator expression")
			return TypeAny
		}
	}

	if op == token.DOT {
		// Dot access: only allowed on dict types for member read
		if len(node.Children) < 2 {
			return TypeAny
		}
		if node.Children[0] == nil || node.Children[1] == nil {
			a.errorAt(node, "malformed dot access expression")
			return TypeAny
		}
		targetType := a.Analyze(node.Children[0])
		member := node.Children[1].TokenLiteral()
		if targetType.Equals(TypeNull) {
			a.errorAt(node.Children[0], "cannot access member '%s' on null", member)
			return TypeAny
		}

		switch t := targetType.(type) {
		case *DictType:
			return t.ValueType
		default:
			if isUnknownType(targetType) {
				return TypeAny
			}
			a.errorAt(node, "dot access is only supported on dict; use len(entity), upper(entity), etc. instead of entity.%s", member)
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
		case token.BANG:
			// Policy §4.3: ! requires bool operand
			if !isBoolLike(operandType) && !isUnknownType(operandType) {
				a.errorAt(node, "unary '!' expects bool operand, got %s", operandType.String())
			}
			return TypeBool
		}
		return operandType
	}

	// Assignment must be handled before analyzing the left side,
	// because the left side may be a new variable being defined.
	if op == token.EQUALS && len(node.Children) == 2 {
		target := node.Children[0]
		if target == nil {
			a.errorAt(node, "left side of assignment must be an identifier")
			return TypeAny
		}
		if target.NodeType == ast.IdentifierNode && node.Children[1].NodeType == ast.LambdaNode {
			varName := target.TokenLiteral()
			if a.currentScope.LookupLocal(varName) == nil {
				funcType := functionTypeFromLambdaNode(node.Children[1])
				a.currentScope.Define(varName, funcType, true)
				a.functions[varName] = funcType
			}
			a.pendingFuncName = varName
		}
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
			if _, dstIsFunc := sym.Type.(*FunctionType); dstIsFunc {
				if srcFunc, srcIsFunc := rightType.(*FunctionType); srcIsFunc {
					sym.Type = srcFunc
					a.functions[varName] = srcFunc
					return rightType
				}
			}
			if _, srcIsResult := rightType.(*ResultType); srcIsResult {
				if _, dstIsResult := sym.Type.(*ResultType); !dstIsResult && !isUnknownType(sym.Type) {
					a.errorAt(target, "[C-TYPE] cannot assign result to '%s'; use 'unwrap()' or 'when' to extract the value", sym.Type.String())
					return rightType
				}
			}
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
			a.callPlans[rightNode] = &ir.CallPlan{Kind: ir.CallBuiltin, CalleeName: name, MinArity: sig.MinArgs, MaxArity: sig.MaxArgs, Dispatch: ir.DispatchBuiltin, RuntimeSymbol: builtinsRuntimeName(name)}
			if pipeArgCount < sig.MinArgs || pipeArgCount > sig.MaxArgs {
				a.errorAt(node, "builtin '%s' expects %d-%d arguments but got %d (including piped input)", name, sig.MinArgs, sig.MaxArgs, pipeArgCount)
			}
			pipeArgTypes := make([]Type, 0, pipeArgCount)
			pipeArgTypes = append(pipeArgTypes, inputType)
			pipeArgTypes = append(pipeArgTypes, argTypes...)
			pipeArgNodes := make([]*ast.TreeNode, 0, pipeArgCount)
			pipeArgNodes = append(pipeArgNodes, inputNode)
			pipeArgNodes = append(pipeArgNodes, argsNode.Children...)
			a.checkArgTypes(name, sig.Type.ParamTypes, pipeArgTypes, pipeArgNodes)
			a.callPlans[rightNode].ArgTypesChecked = true
			return a.inferBuiltinReturnType(name, pipeArgTypes, node)
		}
	}

	if funcType, ok := funcExprType.(*FunctionType); ok {
		minArity := funcType.MinArity()
		maxArity := len(funcType.ParamTypes)
		defaultNodes := defaultNodesFromFunctionType(funcType, pipeArgCount)
		dispatch := ir.DispatchClosure
		runtimeSymbol := ""
		if funcNode.NodeType == ast.IdentifierNode {
			name := funcNode.TokenLiteral()
			if sym := a.currentScope.Lookup(name); sym != nil && !sym.Mutable {
				if _, exists := a.functions[name]; exists {
					dispatch = ir.DispatchDirect
					runtimeSymbol = "quark_" + name
				}
			}
		}
		a.callPlans[rightNode] = &ir.CallPlan{
			Kind:          ir.CallFunctionValue,
			CalleeName:    calleeNameFromNode(funcNode),
			MinArity:      minArity,
			MaxArity:      maxArity,
			Dispatch:      dispatch,
			RuntimeSymbol: runtimeSymbol,
			DefaultNodes:  defaultNodes,
		}
		if pipeArgCount < minArity || pipeArgCount > maxArity {
			if minArity == maxArity {
				a.errorAt(node, "function expects %d arguments but got %d (including piped input)", maxArity, pipeArgCount)
			} else {
				a.errorAt(node, "function expects %d-%d arguments but got %d (including piped input)", minArity, maxArity, pipeArgCount)
			}
		}
		pipeCallee := "function"
		if funcNode.NodeType == ast.IdentifierNode {
			pipeCallee = funcNode.TokenLiteral()
		}
		pipeArgTypes := []Type{inputType}
		pipeArgTypes = append(pipeArgTypes, argTypes...)
		pipeArgNodes := []*ast.TreeNode{inputNode}
		pipeArgNodes = append(pipeArgNodes, argsNode.Children...)
		a.checkArgTypes(pipeCallee, funcType.ParamTypes, pipeArgTypes, pipeArgNodes)
		a.callPlans[rightNode].ArgTypesChecked = true
		return funcType.ReturnType
	}

	return TypeAny
}

func (a *Analyzer) inferBuiltinReturnType(name string, argTypes []Type, callNode *ast.TreeNode) Type {
	// Special-case builtins that need custom type checks beyond what checkArgTypes handles.
	switch name {
	case "unwrap":
		if len(argTypes) >= 1 {
			if res, ok := argTypes[0].(*ResultType); ok {
				return res.OkType
			}
			if !isUnknownType(argTypes[0]) {
				a.errorAt(callNode, "argument 1 of 'unwrap' expects result, got %s", argTypes[0].String())
			}
		}
		return TypeAny

	case "is_ok", "is_err":
		if len(argTypes) >= 1 {
			if _, ok := argTypes[0].(*ResultType); !ok && !isUnknownType(argTypes[0]) {
				a.errorAt(callNode, "argument 1 of '%s' expects result, got %s", name, argTypes[0].String())
			}
		}
		return TypeBool

	case "len":
		if len(argTypes) >= 1 && !isUnknownType(argTypes[0]) {
			t := argTypes[0]
			_, isList := t.(*ListType)
			_, isDict := t.(*DictType)
			_, isVec := t.(*VectorType)
			isStr := t.Equals(TypeString)
			if !isList && !isDict && !isVec && !isStr {
				a.errorAt(callNode, "argument 1 of 'len' expects str, list, dict, or vector, got %s", t.String())
			}
		}
		return TypeInt

	case "abs":
		if len(argTypes) >= 1 && !isUnknownType(argTypes[0]) {
			t := argTypes[0]
			if IsNumeric(t) {
				return t
			}
			a.errorAt(callNode, "argument 1 of 'abs' expects int or float, got %s", t.String())
		}
		return TypeAny

	case "sum":
		if len(argTypes) >= 1 && !isUnknownType(argTypes[0]) {
			t := argTypes[0]
			switch st := t.(type) {
			case *VectorType:
				if !IsNumeric(st.ElementType) && st.ElementType != TypeBool && !isUnknownType(st.ElementType) {
					a.errorAt(callNode, "argument 1 of 'sum' expects numeric or bool vector, got %s", t.String())
				}
			default:
				a.errorAt(callNode, "argument 1 of 'sum' expects numeric or bool vector, got %s", t.String())
			}
		}
		return TypeAny

	case "min", "max":
		if len(argTypes) == 1 && !isUnknownType(argTypes[0]) {
			t := argTypes[0]
			if vec, isVec := t.(*VectorType); isVec {
				if !IsNumeric(vec.ElementType) && !isUnknownType(vec.ElementType) {
					a.errorAt(callNode, "argument 1 of '%s' with single argument expects numeric vector, got %s", name, t.String())
				}
				return TypeFloat
			}
			a.errorAt(callNode, "argument 1 of '%s' with single argument expects numeric vector, got %s", name, t.String())
			return TypeAny
		}
		if len(argTypes) == 2 {
			for i, t := range argTypes {
				if !IsNumeric(t) && !isUnknownType(t) {
					a.errorAt(callNode, "argument %d of '%s' expects numeric, got %s", i+1, name, t.String())
				}
			}
		}
		if sig, ok := a.builtins[name]; ok {
			return sig.Type.ReturnType
		}
		return TypeAny

	case "concat":
		if len(argTypes) == 2 {
			t0, t1 := argTypes[0], argTypes[1]
			if !isUnknownType(t0) && !isUnknownType(t1) {
				_, t0List := t0.(*ListType)
				_, t1List := t1.(*ListType)
				bothStr := isStringLike(t0) && isStringLike(t1)
				bothList := t0List && t1List
				if !bothStr && !bothList {
					a.errorAt(callNode, "concat requires both arguments to be str+str or list+list, got %s and %s", t0.String(), t1.String())
				}
			}
		}
		if sig, ok := a.builtins[name]; ok {
			return sig.Type.ReturnType
		}
		return TypeAny
	}

	// All other non-to_vector builtins: use the signature's return type.
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
	condType := a.Analyze(node.Children[0]) // condition
	if !isBoolLike(condType) && !isUnknownType(condType) {
		a.errorAt(node.Children[0], "ternary condition must be bool, got %s; use a comparison or explicit to_bool()", condType.String())
	}
	trueType := a.Analyze(node.Children[1])
	falseType := a.Analyze(node.Children[2])

	// §5.2: warn when both branches are concrete but incompatible
	if !isUnknownType(trueType) && !isUnknownType(falseType) &&
		!CanAssign(trueType, falseType) && !CanAssign(falseType, trueType) {
		a.errorAt(node, "[warning] ternary branches have incompatible types: '%s' and '%s'", trueType.String(), falseType.String())
	}

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

	// Import all symbols from module into current scope, checking for collisions
	for name, sym := range module.Symbols {
		if existing := a.currentScope.LookupLocal(name); existing != nil {
			// Allow re-importing the same symbol from the same module (dedup)
			// but reject conflicts with other definitions
			a.errorAt(node, "symbol '%s' from module '%s' conflicts with existing definition", name, moduleName)
			continue
		}
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

// GetCallPlans returns call-focused IR metadata keyed by FunctionCall AST node.
func (a *Analyzer) GetCallPlans() map[*ast.TreeNode]*ir.CallPlan {
	return a.callPlans
}

// GetReturnValidation returns function/lambda nodes that were checked against return annotations.
func (a *Analyzer) GetReturnValidation() map[*ast.TreeNode]bool {
	return a.returnValidated
}

func defaultNodesFromFunctionType(ft *FunctionType, provided int) []*ast.TreeNode {
	if ft == nil || ft.DefaultValues == nil || provided >= len(ft.ParamTypes) {
		return nil
	}
	nodes := make([]*ast.TreeNode, 0)
	for i := provided; i < len(ft.ParamTypes); i++ {
		if i >= len(ft.DefaultValues) || ft.DefaultValues[i] == nil || ft.DefaultValues[i].Node == nil {
			continue
		}
		if node, ok := ft.DefaultValues[i].Node.(*ast.TreeNode); ok {
			nodes = append(nodes, node)
		}
	}
	if len(nodes) == 0 {
		return nil
	}
	return nodes
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
	// Lambdas create a new scope where break/continue are invalid
	savedLoopDepth := a.loopDepth
	a.loopDepth = 0
	defer func() { a.loopDepth = savedLoopDepth }()

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
	defaultCount := 0
	defaultValues := make([]*DefaultValueInfo, len(paramSpecs))
	for i, spec := range paramSpecs {
		if spec.name == "" {
			continue
		}
		var paramType Type
		if spec.typeNode != nil {
			paramType = a.resolveTypeNode(spec.typeNode)
		} else if spec.defaultValue != nil {
			paramType = inferLiteralType(spec.defaultValue)
		} else {
			paramType = TypeAny
		}
		a.currentScope.Define(spec.name, paramType, true)
		paramTypes = append(paramTypes, paramType)
		paramNames[spec.name] = true
		if spec.defaultValue != nil {
			defaultCount++
			defaultValues[i] = &DefaultValueInfo{Node: spec.defaultValue}
		}
	}

	// Resolve annotated return type
	var annotatedReturnType Type
	if node.ReturnType != nil {
		annotatedReturnType = a.resolveTypeNode(node.ReturnType)
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

	// Build function type
	funcType := &FunctionType{
		ParamTypes:          paramTypes,
		ReturnType:          returnType,
		AnnotatedReturnType: annotatedReturnType,
		DefaultCount:        defaultCount,
		DefaultValues:       defaultValues,
	}

	// Validate default value types
	a.validateDefaultValues(paramSpecs, funcType, node)

	// Validate return type annotation
	funcName := "lambda"
	if a.pendingFuncName != "" {
		funcName = a.pendingFuncName
		a.pendingFuncName = ""
	}
	a.validateReturnType(funcType, returnType, funcName, node)

	return funcType
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
	if _, srcIsResult := valueType.(*ResultType); srcIsResult {
		if _, dstIsResult := declType.(*ResultType); !dstIsResult && !isUnknownType(declType) {
			a.errorAt(nameNode, "[C-TYPE] cannot assign result to '%s'; use 'unwrap()' or 'when' to extract the value", declType.String())
			a.currentScope.Define(varName, declType, true)
			return declType
		}
	}
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

// validateReturnType checks that the inferred return type matches the annotated return type
func (a *Analyzer) validateReturnType(funcType *FunctionType, inferredType Type, funcName string, errorNode *ast.TreeNode) {
	if funcType.AnnotatedReturnType == nil {
		return
	}
	if errorNode != nil {
		a.returnValidated[errorNode] = true
	}
	annotated := funcType.AnnotatedReturnType
	// If inferred type is unknown/any, trust the annotation
	if isUnknownType(inferredType) {
		funcType.ReturnType = annotated
		return
	}
	// Check compatibility — also accept union types where all non-void options match
	if !canReturnAs(annotated, inferredType) {
		a.errorAt(errorNode, "function '%s' declares return type '%s' but body returns '%s'", funcName, annotated.String(), inferredType.String())
	}
	// Keep the annotated type as the authoritative return type
	funcType.ReturnType = annotated
}

// canReturnAs checks if inferredType is compatible with the annotated return type.
// It handles union types by checking if all non-void options are assignable.
func canReturnAs(annotated Type, inferred Type) bool {
	if CanAssign(annotated, inferred) {
		return true
	}
	// If inferred is a union, check if all non-void options are compatible
	if union, ok := inferred.(*UnionType); ok {
		for _, opt := range union.Options {
			if opt.Equals(TypeVoid) {
				continue // Ignore void from incomplete branches
			}
			if !CanAssign(annotated, opt) {
				return false
			}
		}
		return true
	}
	return false
}

// validateDefaultValues checks that default value types match parameter type annotations
func (a *Analyzer) validateDefaultValues(specs []paramSpec, funcType *FunctionType, errorNode *ast.TreeNode) {
	for i, spec := range specs {
		if spec.defaultValue == nil {
			continue
		}
		defaultType := inferLiteralType(spec.defaultValue)
		if i >= len(funcType.ParamTypes) {
			continue
		}
		paramType := funcType.ParamTypes[i]
		if paramType.Equals(TypeAny) || isUnknownType(paramType) {
			continue
		}
		if isUnknownType(defaultType) {
			continue
		}
		if !CanAssign(paramType, defaultType) {
			a.errorAt(errorNode, "default value type '%s' doesn't match parameter type '%s' for parameter '%s'", defaultType.String(), paramType.String(), spec.name)
		}
	}
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
			specs = append(specs, paramSpec{name: name, typeNode: typeNode, defaultValue: child.DefaultValue})
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
	case "result":
		return &ResultType{OkType: TypeAny, ErrType: TypeAny}
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
