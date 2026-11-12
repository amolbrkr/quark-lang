package types

import (
	"quark/ast"
	"quark/token"
	"quark/ir"
)

func (a *Analyzer) analyzeCompilationUnit(node *ast.TreeNode) Type {
	a.predeclareStructTypes(node.Children)
	a.predeclareFunctions(node.Children)
	a.checkCallsBeforeDefinition(node.Children)
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
	a.checkCallsBeforeDefinition(node.Children)
	var lastType Type = TypeVoid
	for _, child := range node.Children {
		lastType = a.Analyze(child)
	}
	return lastType
}

func (a *Analyzer) analyzeFunction(node *ast.TreeNode) Type {
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

	a.validateDefaultValues(paramSpecs, funcType, nameNode)

	a.pushScope()
	paramSyms := make([]*Symbol, len(paramSpecs))
	owner := a.newParamOwner()
	for i, spec := range paramSpecs {
		if spec.name == "" {
			continue
		}
		var paramType Type = TypeAny
		if i < len(funcType.ParamTypes) {
			paramType = funcType.ParamTypes[i]
		}
		paramSyms[i] = a.currentScope.Define(spec.name, paramType, true)
		a.trackParam(paramSyms[i], owner, i)
	}
	returnType := a.Analyze(bodyNode)
	a.popScope()
	funcType.ParamCalls = a.paramCallsOf(paramSyms)

	a.validateReturnType(funcType, returnType, funcName, nameNode)
	if funcType.AnnotatedReturnType == nil {
		funcType.ReturnType = returnType
	}
	return funcType
}

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
		if lambdaScope.LookupLocal(name) != nil {
			return
		}
		// Native functions are C++ functions, called (and used as values,
		// through their thunk) by name, so there is no binding to capture.
		if _, isNative := a.nativeFns[name]; isNative {
			return
		}
		if lambdaScope.Parent != nil && lambdaScope.Parent.Lookup(name) != nil {
			seen[name] = true
			*result = append(*result, name)
		}
		return
	}
	// Dot access: the right side is a member, key or method name, not a
	// variable, so only the left side is walked. For alias.member (a module
	// call), the member's binding lives in main, so it is captured unless it is
	// a native function, which is called directly.
	if node.NodeType == ast.OperatorNode && node.Token != nil && node.Token.Type == token.DOT && len(node.Children) == 2 {
		left, right := node.Children[0], node.Children[1]
		if left != nil && left.NodeType == ast.IdentifierNode && right != nil && right.NodeType == ast.IdentifierNode {
			if _, isAlias := a.moduleAliases[left.TokenLiteral()]; isAlias && lambdaScope.Lookup(left.TokenLiteral()) == nil {
				member := right.TokenLiteral()
				if _, isNative := a.nativeFns[member]; !isNative && member != "" && !params[member] && !seen[member] {
					seen[member] = true
					*result = append(*result, member)
				}
				return
			}
		}
		a.collectFreeVars(left, lambdaScope, params, seen, result)
		return
	}
	if node.NodeType == ast.LambdaNode {
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
	// Names bound by a nested construct are local to that construct, even
	// when an outer variable has the same name. Their scopes are already
	// popped when this walk runs, so they must be tracked here explicitly.
	if node.NodeType == ast.ForLoopNode && len(node.Children) >= 3 {
		a.collectFreeVars(node.Children[1], lambdaScope, params, seen, result)
		loopParams := withParam(params, node.Children[0].TokenLiteral())
		a.collectFreeVars(node.Children[2], lambdaScope, loopParams, seen, result)
		return
	}
	if node.NodeType == ast.PatternNode && len(node.Children) > 0 {
		armParams := params
		if bindName, hasBinding, _, _ := extractResultPatternBinding(node); hasBinding && bindName != "" {
			armParams = withParam(params, bindName)
		}
		for _, child := range node.Children {
			if child.NodeType == ast.ResultPatternNode {
				continue // the binding identifier itself is not a use
			}
			a.collectFreeVars(child, lambdaScope, armParams, seen, result)
		}
		return
	}
	for _, child := range node.Children {
		a.collectFreeVars(child, lambdaScope, params, seen, result)
	}
}

// withParam returns a copy of params with name added.
func withParam(params map[string]bool, name string) map[string]bool {
	if name == "" || params[name] {
		return params
	}
	merged := make(map[string]bool, len(params)+1)
	for k, v := range params {
		merged[k] = v
	}
	merged[name] = true
	return merged
}

func (a *Analyzer) analyzeLambda(node *ast.TreeNode) Type {
	savedLoopDepth := a.loopDepth
	a.loopDepth = 0
	defer func() { a.loopDepth = savedLoopDepth }()

	if len(node.Children) < 2 {
		a.addError("invalid lambda expression")
		return TypeAny
	}

	argsNode := node.Children[0]
	bodyNode := node.Children[1]
	a.pushScope()

	paramSpecs := collectParamSpecs(argsNode)
	paramTypes := make([]Type, 0, len(paramSpecs))
	paramNames := make(map[string]bool)
	paramSyms := make([]*Symbol, 0, len(paramSpecs))
	owner := a.newParamOwner()
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
		sym := a.currentScope.Define(spec.name, paramType, true)
		a.trackParam(sym, owner, len(paramSyms))
		paramSyms = append(paramSyms, sym)
		paramTypes = append(paramTypes, paramType)
		paramNames[spec.name] = true
		if spec.defaultValue != nil {
			defaultCount++
			defaultValues[i] = &DefaultValueInfo{Node: spec.defaultValue}
		}
	}

	var annotatedReturnType Type
	if node.ReturnType != nil {
		annotatedReturnType = a.resolveTypeNode(node.ReturnType)
	}

	lambdaScope := a.currentScope
	returnType := a.Analyze(bodyNode)

	freeVars := []string{}
	seen := map[string]bool{}
	a.collectFreeVars(bodyNode, lambdaScope, paramNames, seen, &freeVars)
	if len(freeVars) > 0 {
		a.captures[node] = freeVars
	}

	a.popScope()

	funcType := &FunctionType{
		ParamTypes:          paramTypes,
		ReturnType:          returnType,
		AnnotatedReturnType: annotatedReturnType,
		DefaultCount:        defaultCount,
		DefaultValues:       defaultValues,
		ParamCalls:          a.paramCallsOf(paramSyms),
	}

	a.validateDefaultValues(paramSpecs, funcType, node)

	funcName := "lambda"
	if a.pendingFuncName != "" {
		funcName = a.pendingFuncName
		a.pendingFuncName = ""
	}
	a.validateReturnType(funcType, returnType, funcName, node)

	// If this lambda is a named top-level function binding with full scalar annotations,
	// register it as a native fn so call sites can use DispatchNative. A native
	// function is called without its closure, so one that captures outer
	// variables stays a closure (its annotations are enforced on entry instead).
	if funcName != "lambda" && isFullyAnnotated(funcType) && len(a.captures[node]) == 0 {
		nativeParams := make([]string, len(funcType.ParamTypes))
		for i, pt := range funcType.ParamTypes {
			nativeParams[i] = quarkTypeToNativeCType(pt)
		}
		a.nativeFns[funcName] = &ir.CallPlan{
			Kind:             ir.CallFunctionValue,
			CalleeName:       funcName,
			MinArity:         len(funcType.ParamTypes),
			MaxArity:         len(funcType.ParamTypes),
			Dispatch:         ir.DispatchNative,
			RuntimeSymbol:    "quark_" + funcName,
			NativeParamTypes: nativeParams,
			NativeReturnType: quarkTypeToNativeCType(funcType.AnnotatedReturnType),
		}
	}

	return funcType
}

// checkCallsBeforeDefinition reports a statement that calls a function defined
// later in the same statement list. Predeclaration makes the name visible, but
// the function value is only bound when its definition runs, so such a call
// fails at runtime. Calls inside function and lambda bodies are fine (they run
// later) and are not checked; calls made indirectly through another function
// are still caught at runtime.
func (a *Analyzer) checkCallsBeforeDefinition(stmts []*ast.TreeNode) {
	defIndex := map[string]int{}
	defLine := map[string]int{}
	for i, stmt := range stmts {
		name := ""
		var nameNode *ast.TreeNode
		switch {
		case stmt == nil:
			continue
		case stmt.NodeType == ast.FunctionNode && len(stmt.Children) > 0:
			nameNode = stmt.Children[0]
		case isFunctionBindingAssignment(stmt):
			nameNode = stmt.Children[0]
		default:
			continue
		}
		name = nameNode.TokenLiteral()
		if _, seen := defIndex[name]; name == "" || seen {
			continue
		}
		defIndex[name] = i
		if nameNode.Token != nil {
			defLine[name] = nameNode.Token.Line
		}
	}
	if len(defIndex) == 0 {
		return
	}
	var walk func(n *ast.TreeNode, stmtIdx int)
	walk = func(n *ast.TreeNode, stmtIdx int) {
		if n == nil {
			return
		}
		switch n.NodeType {
		case ast.FunctionNode, ast.LambdaNode, ast.ModuleNode, ast.StructDefNode:
			return
		case ast.FunctionCallNode:
			if len(n.Children) > 0 && n.Children[0] != nil && n.Children[0].NodeType == ast.IdentifierNode {
				callee := n.Children[0].TokenLiteral()
				if idx, ok := defIndex[callee]; ok && idx > stmtIdx {
					a.errorAt(n.Children[0], "'%s' is called before its definition on line %d; move the definition above this call", callee, defLine[callee])
				}
			}
		}
		for _, c := range n.Children {
			walk(c, stmtIdx)
		}
	}
	for i, stmt := range stmts {
		if stmt == nil || stmt.NodeType == ast.FunctionNode || isFunctionBindingAssignment(stmt) {
			continue
		}
		walk(stmt, i)
	}
}
