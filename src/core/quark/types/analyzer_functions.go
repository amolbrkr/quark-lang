package types

import "quark/ast"

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
		if lambdaScope.Parent != nil && lambdaScope.Parent.Lookup(name) != nil {
			seen[name] = true
			*result = append(*result, name)
		}
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
	for _, child := range node.Children {
		a.collectFreeVars(child, lambdaScope, params, seen, result)
	}
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
	}

	a.validateDefaultValues(paramSpecs, funcType, node)

	funcName := "lambda"
	if a.pendingFuncName != "" {
		funcName = a.pendingFuncName
		a.pendingFuncName = ""
	}
	a.validateReturnType(funcType, returnType, funcName, node)

	return funcType
}
