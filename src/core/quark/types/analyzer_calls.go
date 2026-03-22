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

	if funcNode.NodeType == ast.OperatorNode && funcNode.Token != nil && funcNode.Token.Type == token.DOT {
		if len(funcNode.Children) >= 2 {
			receiverNode := funcNode.Children[0]
			methodName := funcNode.Children[1].TokenLiteral()
			receiverType := a.Analyze(receiverNode)
			// Propagate ErrorType silently — don't report "can't call method on <error>"
			if IsErrorType(receiverType) {
				a.callPlans[node] = &ir.CallPlan{Kind: ir.CallFunctionValue, CalleeName: calleeNameFromNode(funcNode), MinArity: argCount, MaxArity: argCount, Dispatch: ir.DispatchClosure, ArgTypesChecked: true}
				return TypeError
			}
			receiverKey := typeToBuiltinTypeKey(receiverType)
			if receiverKey != "" {
				if methodSigs, ok := a.methods[receiverKey]; ok {
					if sig, ok := methodSigs[methodName]; ok {
						totalMin := sig.MinArgs + 1
						totalMax := sig.MaxArgs + 1
						runtimeSym := ""
						if spec, ok := builtins.LookupMethod(receiverKey, methodName); ok {
							runtimeSym = spec.Runtime
						}
						a.callPlans[node] = &ir.CallPlan{Kind: ir.CallBuiltin, CalleeName: methodName, MinArity: totalMin, MaxArity: totalMax, Dispatch: ir.DispatchBuiltin, RuntimeSymbol: runtimeSym, IsMethod: true, ReceiverNode: receiverNode, ReceiverTypeKey: string(receiverKey)}
						if argCount < sig.MinArgs || argCount > sig.MaxArgs {
							if sig.MinArgs == sig.MaxArgs {
								a.errorAt(node, "method '%s' expects %d argument(s) but got %d", methodName, sig.MaxArgs, argCount)
							} else {
								a.errorAt(node, "method '%s' expects %d-%d arguments but got %d", methodName, sig.MinArgs, sig.MaxArgs, argCount)
							}
						}
						a.checkArgTypes(methodName, sig.Type.ParamTypes, argTypes, argsNode.Children)
						a.callPlans[node].ArgTypesChecked = true
						// Flow typing: widen element type on mutation methods
						a.widenReceiverOnMutation(receiverNode, receiverType, methodName, argTypes)
						if methodName == "to_vector" {
							return a.inferBuiltinReturnType("vfrom_list", []Type{receiverType}, node)
						}
						if methodName == "to_list" {
							return inferToListReturnType(receiverType)
						}
						return refineMethodReturnType(sig.Type.ReturnType, receiverType, methodName)
					}
				}
			}
			// Try union type method dispatch — check if all union members support the method
			if unionType, ok := receiverType.(*UnionType); ok {
				return a.analyzeUnionMethodCall(node, unionType, methodName, argCount, argTypes, argsNode, funcNode)
			}
			if !isUnknownType(receiverType) && receiverKey != "" {
				a.errorAt(funcNode, "type '%s' has no method '%s'", receiverType.String(), methodName)
			} else if !isUnknownType(receiverType) {
				a.errorAt(funcNode, "dot-call syntax is not supported on values of type '%s'", receiverType.String())
			}
		}
		a.callPlans[node] = &ir.CallPlan{Kind: ir.CallFunctionValue, CalleeName: calleeNameFromNode(funcNode), MinArity: argCount, MaxArity: argCount, Dispatch: ir.DispatchClosure, ArgTypesChecked: true}
		return TypeError
	}

	funcExprType := a.Analyze(funcNode)
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
	if funcNode.NodeType == ast.IdentifierNode {
		name := funcNode.TokenLiteral()
		if sym := a.currentScope.Lookup(name); sym != nil && !sym.Mutable {
			if _, exists := a.functions[name]; exists {
				dispatch = ir.DispatchDirect
				runtimeSymbol = "quark_" + name
			}
		}
	}
	a.callPlans[node] = &ir.CallPlan{Kind: ir.CallFunctionValue, CalleeName: calleeNameFromNode(funcNode), MinArity: minArity, MaxArity: maxArity, Dispatch: dispatch, RuntimeSymbol: runtimeSymbol, DefaultNodes: defaultNodes}
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

	if funcNode.NodeType == ast.OperatorNode && funcNode.Token != nil && funcNode.Token.Type == token.DOT {
		if len(funcNode.Children) >= 2 {
			receiverNode := funcNode.Children[0]
			methodName := funcNode.Children[1].TokenLiteral()
			receiverType := a.Analyze(receiverNode)
			if IsErrorType(receiverType) {
				a.callPlans[rightNode] = &ir.CallPlan{Kind: ir.CallFunctionValue, CalleeName: calleeNameFromNode(funcNode), MinArity: pipeArgCount, MaxArity: pipeArgCount, Dispatch: ir.DispatchClosure, ArgTypesChecked: true}
				return TypeError
			}
			receiverKey := typeToBuiltinTypeKey(receiverType)
			if receiverKey != "" {
				if methodSigs, ok := a.methods[receiverKey]; ok {
					if sig, ok := methodSigs[methodName]; ok {
						pipeIntoMethodArgCount := len(argsNode.Children) + 1
						runtimeSym := ""
						if spec, ok := builtins.LookupMethod(receiverKey, methodName); ok {
							runtimeSym = spec.Runtime
						}
						a.callPlans[rightNode] = &ir.CallPlan{Kind: ir.CallBuiltin, CalleeName: methodName, MinArity: sig.MinArgs + 1, MaxArity: sig.MaxArgs + 1, Dispatch: ir.DispatchBuiltin, RuntimeSymbol: runtimeSym, IsMethod: true, ReceiverNode: receiverNode, ReceiverTypeKey: string(receiverKey)}
						if pipeIntoMethodArgCount < sig.MinArgs || pipeIntoMethodArgCount > sig.MaxArgs {
							if sig.MinArgs == sig.MaxArgs {
								a.errorAt(node, "method '%s' expects %d argument(s) but got %d (including piped input)", methodName, sig.MaxArgs, pipeIntoMethodArgCount)
							} else {
								a.errorAt(node, "method '%s' expects %d-%d arguments but got %d (including piped input)", methodName, sig.MinArgs, sig.MaxArgs, pipeIntoMethodArgCount)
							}
						}
						pipeMethodArgTypes := []Type{inputType}
						pipeMethodArgTypes = append(pipeMethodArgTypes, argTypes...)
						pipeMethodArgNodes := []*ast.TreeNode{inputNode}
						pipeMethodArgNodes = append(pipeMethodArgNodes, argsNode.Children...)
						a.checkArgTypes(methodName, sig.Type.ParamTypes, pipeMethodArgTypes, pipeMethodArgNodes)
						a.callPlans[rightNode].ArgTypesChecked = true
						if methodName == "to_vector" {
							return a.inferBuiltinReturnType("vfrom_list", []Type{receiverType}, node)
						}
						if methodName == "to_list" {
							return inferToListReturnType(receiverType)
						}
						return refineMethodReturnType(sig.Type.ReturnType, receiverType, methodName)
					}
				}
			}
		}
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
		if funcNode.NodeType == ast.IdentifierNode {
			name := funcNode.TokenLiteral()
			if sym := a.currentScope.Lookup(name); sym != nil && !sym.Mutable {
				if _, exists := a.functions[name]; exists {
					dispatch = ir.DispatchDirect
					runtimeSymbol = "quark_" + name
				}
			}
		}
		a.callPlans[rightNode] = &ir.CallPlan{Kind: ir.CallFunctionValue, CalleeName: calleeNameFromNode(funcNode), MinArity: minArity, MaxArity: maxArity, Dispatch: dispatch, RuntimeSymbol: runtimeSymbol, DefaultNodes: defaultNodes}
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
func (a *Analyzer) analyzeUnionMethodCall(node *ast.TreeNode, unionType *UnionType, methodName string, argCount int, argTypes []Type, argsNode *ast.TreeNode, funcNode *ast.TreeNode) Type {
	returnTypes := make([]Type, 0, len(unionType.Options))
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
		if argCount < sig.MinArgs || argCount > sig.MaxArgs {
			if sig.MinArgs == sig.MaxArgs {
				a.errorAt(node, "method '%s' expects %d argument(s) but got %d", methodName, sig.MaxArgs, argCount)
			} else {
				a.errorAt(node, "method '%s' expects %d-%d arguments but got %d", methodName, sig.MinArgs, sig.MaxArgs, argCount)
			}
		}
		a.checkArgTypes(methodName, sig.Type.ParamTypes, argTypes, argsNode.Children)
		returnTypes = append(returnTypes, refineMethodReturnType(sig.Type.ReturnType, opt, methodName))
	}
	// Use the first member's method for the call plan (runtime dispatch is dynamic anyway)
	firstKey := typeToBuiltinTypeKey(unionType.Options[0])
	runtimeSym := ""
	if spec, foundSpec := builtins.LookupMethod(firstKey, methodName); foundSpec {
		runtimeSym = spec.Runtime
	}
	firstSig := a.methods[firstKey][methodName]
	a.callPlans[node] = &ir.CallPlan{
		Kind: ir.CallBuiltin, CalleeName: methodName,
		MinArity: firstSig.MinArgs + 1, MaxArity: firstSig.MaxArgs + 1,
		Dispatch: ir.DispatchBuiltin, RuntimeSymbol: runtimeSym,
		IsMethod: true, ReceiverNode: funcNode.Children[0], ReceiverTypeKey: string(firstKey),
	}
	a.callPlans[node].ArgTypesChecked = true
	return MergeTypes(returnTypes...)
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
