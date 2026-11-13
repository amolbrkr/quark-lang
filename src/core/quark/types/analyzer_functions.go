package types

import (
	"quark/ast"
)

func (a *Analyzer) analyzeCompilationUnit(node *ast.TreeNode) Type {
	// Structs of every file and module body (the loader gave each a unique
	// internal name), so any of them can be used before its definition.
	structs := append([]*ast.TreeNode(nil), node.Children...)
	for _, child := range node.Children {
		if child != nil && child.NodeType == ast.ModuleNode && len(child.Children) >= 2 && child.Children[1] != nil {
			structs = append(structs, child.Children[1].Children...)
		}
	}
	a.predeclareStructTypes(structs)
	// Module bodies share the file scope: all of the program's top-level
	// and module declarations are hoisted together.
	a.declareFixedFns(node, node.Children)
	for _, child := range node.Children {
		if child != nil && child.NodeType == ast.ModuleNode && len(child.Children) >= 2 && child.Children[1] != nil {
			a.declareFixedFns(child.Children[1], child.Children[1].Children)
		}
	}
	lastType := a.analyzeStatements(node, node.Children)
	a.finalizeFixedFns()
	a.finalizeSymbols()
	return lastType
}

func (a *Analyzer) analyzeBlock(node *ast.TreeNode) Type {
	a.pushScope()
	defer a.popScope()
	a.declareFixedFns(node, node.Children)
	return a.analyzeStatements(node, node.Children)
}

// analyzeStatements analyzes a statement list in order, tracking the current
// statement so first assignments can be ordered against calls (rule 23).
func (a *Analyzer) analyzeStatements(list *ast.TreeNode, stmts []*ast.TreeNode) Type {
	savedList, savedIdx := a.curList, a.curIdx
	defer func() { a.curList, a.curIdx = savedList, savedIdx }()
	var lastType Type = TypeVoid
	for i, child := range stmts {
		a.curList, a.curIdx = list, i
		lastType = a.Analyze(child)
	}
	return lastType
}

func (a *Analyzer) collectFreeVars(node *ast.TreeNode, lambdaScope *Scope, params map[string]bool, seen map[string]bool, result *[]string, syms *[]*Symbol) {
	if node == nil {
		return
	}
	if node.NodeType == ast.IdentifierNode {
		name := node.TokenLiteral()
		if name == "_" || params[name] || seen[name] {
			return
		}
		// A parameter or local named like a builtin or a native function
		// shadows it and is captured like any other variable.
		if a.refersToBuiltin(lambdaScope, name) {
			return
		}
		if lambdaScope.LookupLocal(name) != nil {
			return
		}
		// Declared functions that need no closure are dropped from the
		// captures once that is known (finalizeFixedFns).
		if lambdaScope.Parent != nil {
			if sym := lambdaScope.Parent.Lookup(name); sym != nil {
				seen[name] = true
				*result = append(*result, name)
				*syms = append(*syms, sym)
			}
		}
		return
	}
	// Dot access: the right side is a field, key or method name, not a
	// variable, so only the left side is walked.
	if node.NodeType == ast.MemberNode && len(node.Children) == 2 {
		left := node.Children[0]
		a.collectFreeVars(left, lambdaScope, params, seen, result, syms)
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
			a.collectFreeVars(node.Children[1], lambdaScope, mergedParams, seen, result, syms)
		}
		return
	}
	// Names bound by a nested construct are local to that construct, even
	// when an outer variable has the same name. Their scopes are already
	// popped when this walk runs, so they must be tracked here explicitly.
	if node.NodeType == ast.ForLoopNode && len(node.Children) >= 3 {
		a.collectFreeVars(node.Children[1], lambdaScope, params, seen, result, syms)
		loopParams := withParam(params, node.Children[0].TokenLiteral())
		a.collectFreeVars(node.Children[2], lambdaScope, loopParams, seen, result, syms)
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
			a.collectFreeVars(child, lambdaScope, armParams, seen, result, syms)
		}
		return
	}
	for _, child := range node.Children {
		a.collectFreeVars(child, lambdaScope, params, seen, result, syms)
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
		// An annotation, or else a literal default, gives the parameter its
		// type; either is checked on entry, however the function is called.
		var paramType Type
		if spec.typeNode != nil {
			paramType = a.resolveTypeNode(spec.typeNode)
		} else if spec.defaultValue != nil {
			paramType = inferLiteralType(spec.defaultValue)
		} else {
			paramType = TypeAny
		}
		sym := a.declareSym(spec.name, paramType, true, SymParam, paramType, spec.nameNode)
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
	if annotatedReturnType != nil {
		a.expect(lastValue(bodyNode), annotatedReturnType)
	}
	returnType := a.Analyze(bodyNode)

	freeVars := []string{}
	freeSyms := []*Symbol{}
	seen := map[string]bool{}
	a.collectFreeVars(bodyNode, lambdaScope, paramNames, seen, &freeVars, &freeSyms)
	if len(freeSyms) > 0 {
		a.captureSyms[node] = freeSyms
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

	return funcType
}
