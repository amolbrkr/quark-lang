package types

import (
	"quark/ast"
	"quark/builtins"
	"quark/ir"
	"quark/token"
)

// checkArgTypes validates argument types against parameter types using the knowability rule.
func (a *Analyzer) checkArgTypes(calleeName string, paramTypes []Type, argTypes []Type, argNodes []*ast.TreeNode) {
	for i, argType := range argTypes {
		if i >= len(paramTypes) {
			break
		}
		paramType := paramTypes[i]
		if paramType.Equals(TypeAny) || isUnknownType(paramType) {
			continue
		}
		if isUnknownType(argType) || IsErrorType(argType) {
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

// methodCallResult holds the outcome of resolveMethodCall.
type methodCallResult struct {
	returnType Type
	resolved   bool // true = method found/error propagated/union dispatched; false = not found
}

// resolveMethodCall is the shared method dispatch logic for both direct calls (x.method(args))
// and pipe calls (value | x.method(args)). It handles receiver analysis, method lookup,
// arity/type validation, CallPlan creation, flow typing, and return type refinement.
//
// Parameters:
//   - planNode: the AST node that receives the CallPlan
//   - errorNode: the AST node for arity error locations (planNode for direct calls, pipe node for pipes)
//   - funcNode: the DOT operator node containing [receiverNode, methodIdentifier]
//   - argCount: number of args the method receives (excludes receiver; includes piped input for pipes)
//   - argTypes: types of those args
//   - argNodes: AST nodes of those args
//   - arityCtx: suffix for arity errors (e.g. " (including piped input)" or "")
//
// Returns resolved=false when no method was found, letting the caller decide whether to
// report an error (direct calls) or fall through to other dispatch (pipe calls).
func (a *Analyzer) resolveMethodCall(
	planNode *ast.TreeNode,
	errorNode *ast.TreeNode,
	funcNode *ast.TreeNode,
	argCount int,
	argTypes []Type,
	argNodes []*ast.TreeNode,
	arityCtx string,
) methodCallResult {
	if len(funcNode.Children) < 2 {
		return methodCallResult{resolved: false}
	}
	receiverNode := funcNode.Children[0]
	methodName := funcNode.Children[1].TokenLiteral()
	receiverType := a.Analyze(receiverNode)

	// Propagate ErrorType silently — don't report "can't call method on <error>"
	if IsErrorType(receiverType) {
		a.callPlans[planNode] = &ir.CallPlan{Kind: ir.CallFunctionValue, CalleeName: calleeNameFromNode(funcNode), MinArity: argCount, MaxArity: argCount, Dispatch: ir.DispatchClosure, ArgTypesChecked: true}
		return methodCallResult{returnType: TypeError, resolved: true}
	}

	receiverKey := typeToBuiltinTypeKey(receiverType)
	if receiverKey != "" {
		if methodSigs, ok := a.methods[receiverKey]; ok {
			if sig, ok := methodSigs[methodName]; ok {
				totalMin := sig.MinArgs + 1
				totalMax := sig.MaxArgs + 1

				// Check if this method is an extern fn
				methodKey := string(receiverKey)
				// Convert TypeKey back to receiver string for externFns lookup
				receiverStr := typeKeyToReceiverString(receiverKey)
				externKey := receiverStr + "." + methodName
				if proto, isExtern := a.externFns[externKey]; isExtern {
					plan := *proto
					plan.CalleeName = methodName
					plan.ReceiverNode = receiverNode
					plan.ReceiverTypeKey = methodKey
					plan.MinArity = totalMin
					plan.MaxArity = totalMax
					if argCount < sig.MinArgs || argCount > sig.MaxArgs {
						if sig.MinArgs == sig.MaxArgs {
							a.errorAt(errorNode, "extern method '%s' expects %d argument(s) but got %d%s", methodName, sig.MaxArgs, argCount, arityCtx)
						} else {
							a.errorAt(errorNode, "extern method '%s' expects %d-%d arguments but got %d%s", methodName, sig.MinArgs, sig.MaxArgs, argCount, arityCtx)
						}
					}
					a.checkArgTypes(methodName, sig.Type.ParamTypes, argTypes, argNodes)
					plan.ArgTypesChecked = true
					a.callPlans[planNode] = &plan
					return methodCallResult{returnType: sig.Type.ReturnType, resolved: true}
				}

				runtimeSym := ""
				if spec, ok := builtins.LookupMethod(receiverKey, methodName); ok {
					runtimeSym = spec.Runtime
				}
				a.callPlans[planNode] = &ir.CallPlan{Kind: ir.CallBuiltin, CalleeName: methodName, MinArity: totalMin, MaxArity: totalMax, Dispatch: ir.DispatchBuiltin, RuntimeSymbol: runtimeSym, IsMethod: true, ReceiverNode: receiverNode, ReceiverTypeKey: string(receiverKey)}
				if argCount < sig.MinArgs || argCount > sig.MaxArgs {
					if sig.MinArgs == sig.MaxArgs {
						a.errorAt(errorNode, "method '%s' expects %d argument(s) but got %d%s", methodName, sig.MaxArgs, argCount, arityCtx)
					} else {
						a.errorAt(errorNode, "method '%s' expects %d-%d arguments but got %d%s", methodName, sig.MinArgs, sig.MaxArgs, argCount, arityCtx)
					}
				}
				a.checkArgTypes(methodName, sig.Type.ParamTypes, argTypes, argNodes)
				a.callPlans[planNode].ArgTypesChecked = true
				// Flow typing: widen element type on mutation methods
				a.widenReceiverOnMutation(receiverNode, receiverType, methodName, argTypes)
				if methodName == "to_vector" {
					return methodCallResult{returnType: a.inferBuiltinReturnType("vfrom_list", []Type{receiverType}, errorNode), resolved: true}
				}
				if methodName == "to_list" {
					return methodCallResult{returnType: inferToListReturnType(receiverType), resolved: true}
				}
				return methodCallResult{returnType: refineMethodReturnType(sig.Type.ReturnType, receiverType, methodName), resolved: true}
			}
		}
	}

	// Try union type method dispatch — check if all union members support the method
	if unionType, ok := receiverType.(*UnionType); ok {
		retType := a.analyzeUnionMethodCall(planNode, unionType, methodName, argCount, argTypes, argNodes, funcNode)
		return methodCallResult{returnType: retType, resolved: true}
	}

	// Method not found — caller decides whether to error or fall through
	return methodCallResult{resolved: false, returnType: TypeError}
}

func (a *Analyzer) analyzeFunctionCall(node *ast.TreeNode) Type {
	if len(node.Children) < 2 {
		a.errorAt(node, "invalid function call expression")
		return TypeError
	}

	funcNode := node.Children[0]
	argsNode := node.Children[1]
	argCount := len(argsNode.Children)
	argTypes := make([]Type, 0, argCount)
	for _, arg := range argsNode.Children {
		argTypes = append(argTypes, a.Analyze(arg))
	}

	if moduleName, member, sym, ok := a.lookupModuleCall(funcNode); ok {
		if sym == nil {
			a.callPlans[node] = &ir.CallPlan{Kind: ir.CallFunctionValue, CalleeName: moduleName + "." + member, MinArity: argCount, MaxArity: argCount, Dispatch: ir.DispatchClosure, ArgTypesChecked: true}
			return TypeAny
		}
		funcType, isFunc := sym.Type.(*FunctionType)
		if !isFunc {
			a.errorAt(funcNode, "module symbol '%s.%s' is not callable", moduleName, member)
			a.callPlans[node] = &ir.CallPlan{Kind: ir.CallFunctionValue, CalleeName: moduleName + "." + member, MinArity: argCount, MaxArity: argCount, Dispatch: ir.DispatchClosure, ArgTypesChecked: true}
			return TypeAny
		}
		minArity := funcType.MinArity()
		maxArity := len(funcType.ParamTypes)
		defaultNodes := defaultNodesFromFunctionType(funcType, argCount)
		dispatch := ir.DispatchClosure
		runtimeSymbol := ""
		if _, exists := a.functions[member]; exists && !sym.Mutable {
			dispatch = ir.DispatchDirect
			runtimeSymbol = "quark_" + member
		}
		a.callPlans[node] = &ir.CallPlan{Kind: ir.CallFunctionValue, CalleeName: moduleName + "." + member, MinArity: minArity, MaxArity: maxArity, Dispatch: dispatch, RuntimeSymbol: runtimeSymbol, DefaultNodes: defaultNodes}
		if argCount < minArity || argCount > maxArity {
			if minArity == maxArity {
				a.errorAt(node, "function expects %d arguments but got %d", maxArity, argCount)
			} else {
				a.errorAt(node, "function expects %d-%d arguments but got %d", minArity, maxArity, argCount)
			}
		}
		a.checkArgTypes(moduleName+"."+member, funcType.ParamTypes, argTypes, argsNode.Children)
		a.callPlans[node].ArgTypesChecked = true
		return funcType.ReturnType
	}

	// Method dispatch: x.method(args)
	if funcNode.NodeType == ast.OperatorNode && funcNode.Token != nil && funcNode.Token.Type == token.DOT {
		result := a.resolveMethodCall(node, node, funcNode, argCount, argTypes, argsNode.Children, "")
		if result.resolved {
			return result.returnType
		}
		// Method not found — report error
		receiverType := a.Analyze(funcNode.Children[0])
		receiverKey := typeToBuiltinTypeKey(receiverType)
		if !isUnknownType(receiverType) && receiverKey != "" {
			a.errorAt(funcNode, "type '%s' has no method '%s'", receiverType.String(), funcNode.Children[1].TokenLiteral())
		} else if !isUnknownType(receiverType) {
			a.errorAt(funcNode, "dot-call syntax is not supported on values of type '%s'", receiverType.String())
		}
		a.callPlans[node] = &ir.CallPlan{Kind: ir.CallFunctionValue, CalleeName: calleeNameFromNode(funcNode), MinArity: argCount, MaxArity: argCount, Dispatch: ir.DispatchClosure, ArgTypesChecked: true}
		return TypeError
	}

	funcExprType := a.Analyze(funcNode)
	if funcNode.NodeType == ast.IdentifierNode {
		name := funcNode.TokenLiteral()
		if sig, ok := a.builtins[name]; ok {
			// Check if this builtin is actually an extern fn (DispatchExtern)
			if proto, isExtern := a.externFns[name]; isExtern {
				plan := *proto // copy prototype
				plan.CalleeName = name
				plan.MinArity = sig.MinArgs
				plan.MaxArity = sig.MaxArgs
				if argCount < sig.MinArgs || argCount > sig.MaxArgs {
					a.errorAt(node, "extern fn '%s' expects %d-%d arguments but got %d", name, sig.MinArgs, sig.MaxArgs, argCount)
				}
				a.checkArgTypes(name, sig.Type.ParamTypes, argTypes, argsNode.Children)
				plan.ArgTypesChecked = true
				a.callPlans[node] = &plan
				return sig.Type.ReturnType
			}
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
		if IsErrorType(funcExprType) {
			a.callPlans[node] = &ir.CallPlan{Kind: ir.CallFunctionValue, CalleeName: calleeNameFromNode(funcNode), MinArity: argCount, MaxArity: argCount, Dispatch: ir.DispatchClosure, ArgTypesChecked: true}
			return TypeError
		}
		if !isUnknownType(funcExprType) {
			a.errorAt(funcNode, "expression is not callable")
		}
		a.callPlans[node] = &ir.CallPlan{Kind: ir.CallFunctionValue, CalleeName: calleeNameFromNode(funcNode), MinArity: argCount, MaxArity: argCount, Dispatch: ir.DispatchClosure, ArgTypesChecked: true}
		return TypeError
	}

	minArity := funcType.MinArity()
	maxArity := len(funcType.ParamTypes)
	defaultNodes := defaultNodesFromFunctionType(funcType, argCount)
	dispatch := ir.DispatchClosure
	runtimeSymbol := ""
	var nativeParamTypes []string
	var nativeReturnType string
	if funcNode.NodeType == ast.IdentifierNode {
		name := funcNode.TokenLiteral()
		// Check nativeFns first — applies regardless of mutability.
		if proto, isNative := a.nativeFns[name]; isNative {
			dispatch = ir.DispatchNative
			runtimeSymbol = proto.RuntimeSymbol
			nativeParamTypes = proto.NativeParamTypes
			nativeReturnType = proto.NativeReturnType
		} else if sym := a.currentScope.Lookup(name); sym != nil && !sym.Mutable {
			if _, exists := a.functions[name]; exists {
				dispatch = ir.DispatchDirect
				runtimeSymbol = "quark_" + name
			}
		}
	}
	a.callPlans[node] = &ir.CallPlan{Kind: ir.CallFunctionValue, CalleeName: calleeNameFromNode(funcNode), MinArity: minArity, MaxArity: maxArity, Dispatch: dispatch, RuntimeSymbol: runtimeSymbol, DefaultNodes: defaultNodes, NativeParamTypes: nativeParamTypes, NativeReturnType: nativeReturnType}
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

func (a *Analyzer) lookupModuleCall(funcNode *ast.TreeNode) (string, string, *Symbol, bool) {
	if funcNode == nil || funcNode.NodeType != ast.OperatorNode || funcNode.Token == nil || funcNode.Token.Type != token.DOT {
		return "", "", nil, false
	}
	if len(funcNode.Children) < 2 || funcNode.Children[0] == nil || funcNode.Children[1] == nil {
		return "", "", nil, false
	}
	left := funcNode.Children[0]
	right := funcNode.Children[1]
	if left.NodeType != ast.IdentifierNode || right.NodeType != ast.IdentifierNode {
		return "", "", nil, false
	}
	alias := left.TokenLiteral()
	moduleName, ok := a.moduleAliases[alias]
	if !ok {
		return "", "", nil, false
	}
	module, exists := a.modules[moduleName]
	if !exists {
		a.errorAt(left, "undefined module '%s' for alias '%s'", moduleName, alias)
		return moduleName, right.TokenLiteral(), nil, true
	}
	member := right.TokenLiteral()
	sym, exists := module.Symbols[member]
	if !exists {
		a.errorAt(right, "module '%s' has no symbol '%s'", moduleName, member)
		return moduleName, member, nil, true
	}
	return moduleName, member, sym, true
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

func inferToListReturnType(receiverType Type) Type {
	vec, ok := receiverType.(*VectorType)
	if !ok {
		return &ListType{ElementType: TypeAny}
	}
	if isUnknownType(vec.ElementType) {
		return &ListType{ElementType: TypeAny}
	}
	return &ListType{ElementType: vec.ElementType}
}

func (a *Analyzer) analyzePipe(node *ast.TreeNode) Type {
	if len(node.Children) < 2 {
		return TypeAny
	}
	inputNode := node.Children[0]
	inputType := a.Analyze(inputNode)
	rightNode := node.Children[1]
	if rightNode.NodeType != ast.FunctionCallNode || len(rightNode.Children) < 2 {
		a.errorAt(rightNode, "pipe target must be a function call; use f(...) or obj.method(...)")
		return TypeAny
	}

	funcNode := rightNode.Children[0]
	argsNode := rightNode.Children[1]
	argTypes := make([]Type, 0, len(argsNode.Children))
	for _, arg := range argsNode.Children {
		argTypes = append(argTypes, a.Analyze(arg))
	}
	pipeArgCount := len(argsNode.Children) + 1

	if moduleName, member, sym, ok := a.lookupModuleCall(funcNode); ok {
		if sym == nil {
			a.callPlans[rightNode] = &ir.CallPlan{Kind: ir.CallFunctionValue, CalleeName: moduleName + "." + member, MinArity: pipeArgCount, MaxArity: pipeArgCount, Dispatch: ir.DispatchClosure, ArgTypesChecked: true}
			return TypeAny
		}
		funcType, isFunc := sym.Type.(*FunctionType)
		if !isFunc {
			a.errorAt(funcNode, "module symbol '%s.%s' is not callable", moduleName, member)
			a.callPlans[rightNode] = &ir.CallPlan{Kind: ir.CallFunctionValue, CalleeName: moduleName + "." + member, MinArity: pipeArgCount, MaxArity: pipeArgCount, Dispatch: ir.DispatchClosure, ArgTypesChecked: true}
			return TypeAny
		}
		minArity := funcType.MinArity()
		maxArity := len(funcType.ParamTypes)
		defaultNodes := defaultNodesFromFunctionType(funcType, pipeArgCount)
		dispatch := ir.DispatchClosure
		runtimeSymbol := ""
		if _, exists := a.functions[member]; exists && !sym.Mutable {
			dispatch = ir.DispatchDirect
			runtimeSymbol = "quark_" + member
		}
		a.callPlans[rightNode] = &ir.CallPlan{Kind: ir.CallFunctionValue, CalleeName: moduleName + "." + member, MinArity: minArity, MaxArity: maxArity, Dispatch: dispatch, RuntimeSymbol: runtimeSymbol, DefaultNodes: defaultNodes}
		if pipeArgCount < minArity || pipeArgCount > maxArity {
			if minArity == maxArity {
				a.errorAt(node, "function expects %d arguments but got %d (including piped input)", maxArity, pipeArgCount)
			} else {
				a.errorAt(node, "function expects %d-%d arguments but got %d (including piped input)", minArity, maxArity, pipeArgCount)
			}
		}
		pipeArgTypes := []Type{inputType}
		pipeArgTypes = append(pipeArgTypes, argTypes...)
		pipeArgNodes := []*ast.TreeNode{inputNode}
		pipeArgNodes = append(pipeArgNodes, argsNode.Children...)
		a.checkArgTypes(moduleName+"."+member, funcType.ParamTypes, pipeArgTypes, pipeArgNodes)
		a.callPlans[rightNode].ArgTypesChecked = true
		return funcType.ReturnType
	}

	// Method dispatch in pipe: value | x.method(args)
	if funcNode.NodeType == ast.OperatorNode && funcNode.Token != nil && funcNode.Token.Type == token.DOT {
		pipeMethodArgCount := len(argsNode.Children) + 1
		pipeMethodArgTypes := []Type{inputType}
		pipeMethodArgTypes = append(pipeMethodArgTypes, argTypes...)
		pipeMethodArgNodes := []*ast.TreeNode{inputNode}
		pipeMethodArgNodes = append(pipeMethodArgNodes, argsNode.Children...)

		result := a.resolveMethodCall(rightNode, node, funcNode, pipeMethodArgCount, pipeMethodArgTypes, pipeMethodArgNodes, " (including piped input)")
		if result.resolved {
			return result.returnType
		}
		// Not a known method — fall through to regular function dispatch
		// (DOT could be dict property access returning a callable)
	}

	funcExprType := a.Analyze(funcNode)
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
		var nativeParamTypes []string
		var nativeReturnType string
		if funcNode.NodeType == ast.IdentifierNode {
			name := funcNode.TokenLiteral()
			// Check nativeFns first — applies regardless of mutability.
			if proto, isNative := a.nativeFns[name]; isNative {
				dispatch = ir.DispatchNative
				runtimeSymbol = proto.RuntimeSymbol
				nativeParamTypes = proto.NativeParamTypes
				nativeReturnType = proto.NativeReturnType
			} else if sym := a.currentScope.Lookup(name); sym != nil && !sym.Mutable {
				if _, exists := a.functions[name]; exists {
					dispatch = ir.DispatchDirect
					runtimeSymbol = "quark_" + name
				}
			}
		}
		a.callPlans[rightNode] = &ir.CallPlan{Kind: ir.CallFunctionValue, CalleeName: calleeNameFromNode(funcNode), MinArity: minArity, MaxArity: maxArity, Dispatch: dispatch, RuntimeSymbol: runtimeSymbol, DefaultNodes: defaultNodes, NativeParamTypes: nativeParamTypes, NativeReturnType: nativeReturnType}
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
	a.callPlans[rightNode] = &ir.CallPlan{Kind: ir.CallFunctionValue, CalleeName: calleeNameFromNode(funcNode), MinArity: pipeArgCount, MaxArity: pipeArgCount, Dispatch: ir.DispatchClosure, ArgTypesChecked: true}
	return TypeAny
}

// resolveReceiverSymbol resolves a receiver expression (e.g., an identifier node)
// back to its symbol in the current scope, for flow-typing updates.
func (a *Analyzer) resolveReceiverSymbol(node *ast.TreeNode) *Symbol {
	if node == nil {
		return nil
	}
	if node.NodeType == ast.IdentifierNode {
		name := node.TokenLiteral()
		return a.currentScope.Lookup(name)
	}
	return nil
}

// widenReceiverOnMutation checks if a method call is a mutation (push/set/insert)
// and widens the receiver symbol's element type accordingly.
func (a *Analyzer) widenReceiverOnMutation(receiverNode *ast.TreeNode, receiverType Type, methodName string, argTypes []Type) {
	listType, isList := receiverType.(*ListType)
	if !isList {
		return
	}
	// Determine the new element type from the mutation
	var newElemType Type
	switch methodName {
	case "push":
		if len(argTypes) >= 1 {
			newElemType = argTypes[0]
		}
	case "set":
		if len(argTypes) >= 2 {
			newElemType = argTypes[1] // set(index, value)
		}
	case "insert":
		if len(argTypes) >= 2 {
			newElemType = argTypes[1] // insert(index, value)
		}
	default:
		return
	}
	if newElemType == nil || IsErrorType(newElemType) || isUnknownType(newElemType) {
		return
	}
	// If the element already covers this type, nothing to do
	if CanAssign(listType.ElementType, newElemType) {
		return
	}
	// Widen the element type
	widened := MergeTypes(listType.ElementType, newElemType)
	sym := a.resolveReceiverSymbol(receiverNode)
	if sym != nil {
		sym.Type = &ListType{ElementType: widened}
	}
}

// refineMethodReturnType narrows a generic method return type using the actual
// receiver type. For example, list.get() returns TypeAny in the catalog, but if
// the receiver is list[str], the return should be str.
func refineMethodReturnType(catalogReturn Type, receiverType Type, methodName string) Type {
	if listType, ok := receiverType.(*ListType); ok {
		switch methodName {
		case "get", "pop", "remove":
			// These return element type
			return listType.ElementType
		case "concat", "push", "insert", "slice", "reverse", "enumerate":
			// These return a list (preserve element type for concat/push/slice/reverse)
			if methodName == "enumerate" {
				return &ListType{ElementType: TypeAny} // list of [idx, val] pairs
			}
			return receiverType
		}
	}
	if vecType, ok := receiverType.(*VectorType); ok {
		switch methodName {
		case "get":
			return vecType.ElementType
		case "fillna", "astype":
			return receiverType
		case "to_list":
			return &ListType{ElementType: vecType.ElementType}
		}
	}
	if _, ok := receiverType.(*DictType); ok {
		switch methodName {
		case "get":
			return receiverType.(*DictType).ValueType
		case "set":
			return receiverType
		case "keys":
			return &ListType{ElementType: receiverType.(*DictType).KeyType}
		case "values":
			return &ListType{ElementType: receiverType.(*DictType).ValueType}
		}
	}
	return catalogReturn
}

// analyzeUnionMethodCall checks if all members of a union type support the given method,
// and returns the merged return type. Falls back to TypeError if any member doesn't support it.
func (a *Analyzer) analyzeUnionMethodCall(node *ast.TreeNode, unionType *UnionType, methodName string, argCount int, argTypes []Type, argNodes []*ast.TreeNode, funcNode *ast.TreeNode) Type {
	returnTypes := make([]Type, 0, len(unionType.Options))
	commonRuntimeSym := ""
	runtimeSymSet := false
	for _, opt := range unionType.Options {
		optKey := typeToBuiltinTypeKey(opt)
		if optKey == "" {
			a.errorAt(funcNode, "dot-call syntax is not supported on values of type '%s' (in union)", opt.String())
			return TypeError
		}
		methodSigs, ok := a.methods[optKey]
		if !ok {
			a.errorAt(funcNode, "type '%s' has no methods (in union)", opt.String())
			return TypeError
		}
		sig, ok := methodSigs[methodName]
		if !ok {
			a.errorAt(funcNode, "type '%s' has no method '%s' (in union)", opt.String(), methodName)
			return TypeError
		}
		spec, foundSpec := builtins.LookupMethod(optKey, methodName)
		if !foundSpec {
			a.errorAt(funcNode, "method '%s' is not registered for type '%s' (in union)", methodName, opt.String())
			return TypeError
		}
		if !runtimeSymSet {
			commonRuntimeSym = spec.Runtime
			runtimeSymSet = true
		} else if spec.Runtime != commonRuntimeSym {
			a.errorAt(funcNode, "method '%s' has incompatible runtime dispatch across union members (%s vs %s)", methodName, commonRuntimeSym, spec.Runtime)
			return TypeError
		}
		if argCount < sig.MinArgs || argCount > sig.MaxArgs {
			if sig.MinArgs == sig.MaxArgs {
				a.errorAt(node, "method '%s' expects %d argument(s) but got %d", methodName, sig.MaxArgs, argCount)
			} else {
				a.errorAt(node, "method '%s' expects %d-%d arguments but got %d", methodName, sig.MinArgs, sig.MaxArgs, argCount)
			}
		}
		a.checkArgTypes(methodName, sig.Type.ParamTypes, argTypes, argNodes)
		returnTypes = append(returnTypes, refineMethodReturnType(sig.Type.ReturnType, opt, methodName))
	}
	// Use the first member's method for the call plan (runtime dispatch is dynamic anyway)
	firstKey := typeToBuiltinTypeKey(unionType.Options[0])
	firstSig := a.methods[firstKey][methodName]
	a.callPlans[node] = &ir.CallPlan{
		Kind: ir.CallBuiltin, CalleeName: methodName,
		MinArity: firstSig.MinArgs + 1, MaxArity: firstSig.MaxArgs + 1,
		Dispatch: ir.DispatchBuiltin, RuntimeSymbol: commonRuntimeSym,
		IsMethod: true, ReceiverNode: funcNode.Children[0], ReceiverTypeKey: string(firstKey),
	}
	a.callPlans[node].ArgTypesChecked = true
	return MergeTypes(returnTypes...)
}

// typeKeyToReceiverString maps a catalog TypeKey back to the receiver string
// used as a prefix in extern fn declarations (e.g. TypeListAny → "list").
func typeKeyToReceiverString(key builtins.TypeKey) string {
	switch key {
	case builtins.TypeInt:
		return "int"
	case builtins.TypeFloat:
		return "float"
	case builtins.TypeString:
		return "str"
	case builtins.TypeBool:
		return "bool"
	case builtins.TypeListAny:
		return "list"
	case builtins.TypeDictAny:
		return "dict"
	case builtins.TypeVectorAny:
		return "vector"
	}
	return string(key)
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
