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
			a.errorAt(errorNode, "argument %d of '%s' expects %s, got %s%s", i+1, calleeName, paramType.String(), argType.String(), mismatchDetail(paramType, argType))
		}
	}
}

// checkArity reports a call with the wrong number of arguments. what names
// the callee ("builtin 'len'", "method 'split'", "function 'f'"); got counts
// a piped input, which the message then mentions.
func (a *Analyzer) checkArity(call *ast.TreeNode, what string, minArgs, maxArgs, got int) {
	if got >= minArgs && got <= maxArgs {
		return
	}
	ctx := ""
	if call != nil && call.Piped {
		ctx = " (including piped input)"
	}
	a.errorAt(call, "%s expects %s but got %d%s", what, arityText(minArgs, maxArgs), got, ctx)
}

// methodCallResult holds the outcome of resolveMethodCall.
type methodCallResult struct {
	returnType   Type
	receiverType Type
	resolved     bool // true = method found/error propagated/union dispatched; false = not found
}

// resolveMethodCall plans the method call `receiver.method(args)` (a pipe
// `x | receiver.method(args)` is parsed into the same call). It analyzes the
// receiver, looks the method up, checks arity and argument types, records
// the CallPlan on call, applies flow typing and refines the return type.
//
// funcNode is the DOT node [receiver, method name]; argTypes and argNodes
// are the call's arguments, without the receiver. It returns resolved=false
// when no method was found, for the caller to report.
func (a *Analyzer) resolveMethodCall(call, funcNode *ast.TreeNode, argTypes []Type, argNodes []*ast.TreeNode) methodCallResult {
	argCount := len(argNodes)
	if len(funcNode.Children) < 2 {
		return methodCallResult{resolved: false, returnType: TypeError, receiverType: TypeError}
	}
	receiverNode := funcNode.Children[0]
	methodName := funcNode.Children[1].TokenLiteral()
	receiverType := a.Analyze(receiverNode)

	// Propagate ErrorType silently — don't report "can't call method on <error>"
	if IsErrorType(receiverType) {
		a.callPlans[call] = &ir.CallPlan{CalleeName: calleeNameFromNode(funcNode), MinArity: argCount, MaxArity: argCount, Dispatch: ir.DispatchClosure}
		return methodCallResult{returnType: TypeError, receiverType: receiverType, resolved: true}
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
					a.checkArity(call, "extern method '"+methodName+"'", sig.MinArgs, sig.MaxArgs, argCount)
					a.settleArgs(sig.Type.ParamTypes, argTypes, argNodes)
					a.checkArgTypes(methodName, sig.Type.ParamTypes, argTypes, argNodes)
					a.callPlans[call] = &plan
					return methodCallResult{returnType: sig.Type.ReturnType, receiverType: receiverType, resolved: true}
				}

				runtimeSym, header := "", ""
				if spec, ok := builtins.LookupMethod(receiverKey, methodName); ok {
					runtimeSym, header = spec.Runtime, spec.Header
				}
				a.callPlans[call] = &ir.CallPlan{CalleeName: methodName, MinArity: totalMin, MaxArity: totalMax, Dispatch: ir.DispatchBuiltin, RuntimeSymbol: runtimeSym, RuntimeHeader: header, IsMethod: true, ReceiverNode: receiverNode, ReceiverTypeKey: string(receiverKey)}
				a.checkArity(call, "method '"+methodName+"'", sig.MinArgs, sig.MaxArgs, argCount)
				a.checkArgTypes(methodName, sig.Type.ParamTypes, argTypes, argNodes)
				a.checkMutation(receiverType, methodName, argTypes, argNodes)
				ret := refineMethodReturnType(sig.Type.ReturnType, receiverType, methodName)
				switch methodName {
				case "to_vec":
					ret = a.inferToVectorType(receiverType, call)
				case "astype":
					ret = inferAstypeReturnType(receiverType, argNodes)
				case "to_list":
					ret = inferToListReturnType(receiverType)
				}
				return methodCallResult{returnType: ret, receiverType: receiverType, resolved: true}
			}
		}
	}

	// Receiver type unknown at compile time (e.g. the result of unwrap): dispatch on
	// the runtime type over every builtin receiver type that has this method.
	if isUnknownType(receiverType) {
		if result, ok := a.resolveDynamicMethodCall(call, funcNode, methodName, argTypes, argNodes); ok {
			result.receiverType = receiverType
			return result
		}
	}

	// Try union type method dispatch — check if all union members support the method
	if unionType, ok := receiverType.(*UnionType); ok {
		retType := a.analyzeUnionMethodCall(call, unionType, methodName, argTypes, argNodes, funcNode)
		return methodCallResult{returnType: retType, receiverType: receiverType, resolved: true}
	}

	// Method not found: the caller reports it.
	return methodCallResult{resolved: false, returnType: TypeError, receiverType: receiverType}
}

// dynamicReceivers lists the builtin receiver types a dynamic method call can
// dispatch to, with the QValue type tag that identifies each at runtime.
var dynamicReceivers = []struct {
	key builtins.TypeKey
	tag string
}{
	{builtins.TypeString, "VAL_STRING"},
	{builtins.TypeListAny, "VAL_LIST"},
	{builtins.TypeDictAny, "VAL_DICT"},
	{builtins.TypeVectorAny, "VAL_VECTOR"},
	{builtins.TypeTableAny, "VAL_TABLE"},
}

// resolveDynamicMethodCall plans a method call on a receiver of unknown type as a
// runtime switch over the builtin receiver types that define methodName and accept
// that many arguments. It returns ok=false when no builtin type has the
// method, for the caller to report.
func (a *Analyzer) resolveDynamicMethodCall(call, funcNode *ast.TreeNode, methodName string, argTypes []Type, argNodes []*ast.TreeNode) (methodCallResult, bool) {
	argCount := len(argNodes)
	var candidates []ir.MethodCandidate
	var sigs []*builtinSignature
	var arityMismatch *builtinSignature
	hasExtern := false
	for _, recv := range dynamicReceivers {
		sig, ok := a.methods[recv.key][methodName]
		if !ok {
			continue
		}
		if _, isExtern := a.externFns[typeKeyToReceiverString(recv.key)+"."+methodName]; isExtern {
			hasExtern = true
			continue
		}
		spec, ok := builtins.LookupMethod(recv.key, methodName)
		if !ok {
			continue
		}
		if argCount < sig.MinArgs || argCount > sig.MaxArgs {
			if arityMismatch == nil {
				arityMismatch = sig
			}
			continue
		}
		candidates = append(candidates, ir.MethodCandidate{ValueTag: recv.tag, ReceiverTypeKey: string(recv.key), RuntimeSymbol: spec.Runtime, RuntimeHeader: spec.Header})
		sigs = append(sigs, sig)
	}

	if len(candidates) == 0 {
		switch {
		case arityMismatch != nil:
			a.checkArity(call, "method '"+methodName+"'", arityMismatch.MinArgs, arityMismatch.MaxArgs, argCount)
		case hasExtern:
			a.errorAt(funcNode, "extern method '%s' needs a receiver of known type; add a type annotation", methodName)
		default:
			return methodCallResult{resolved: false, returnType: TypeError}, false
		}
		a.callPlans[call] = &ir.CallPlan{CalleeName: methodName, MinArity: argCount, MaxArity: argCount, Dispatch: ir.DispatchClosure}
		return methodCallResult{returnType: TypeError, resolved: true}, true
	}

	// With a single candidate the parameter types are known; with several they
	// differ per receiver (e.g. get takes a str key on dicts, an int on lists),
	// so the runtime functions check them.
	if len(sigs) == 1 {
		a.checkArgTypes(methodName, sigs[0].Type.ParamTypes, argTypes, argNodes)
	}
	// The result is as unknown as the receiver unless every candidate agrees on
	// the return type: a union would make a following method call fail for the
	// members that lack it, although the runtime value may well have it.
	returnType := sigs[0].Type.ReturnType
	for _, sig := range sigs[1:] {
		if !sig.Type.ReturnType.Equals(returnType) {
			returnType = TypeAny
			break
		}
	}
	a.callPlans[call] = &ir.CallPlan{
		CalleeName: methodName,
		MinArity:   argCount + 1, MaxArity: argCount + 1,
		Dispatch: ir.DispatchDynamicMethod, IsMethod: true,
		ReceiverNode: funcNode.Children[0], MethodCandidates: candidates,
	}
	return methodCallResult{returnType: returnType, resolved: true}, true
}

func (a *Analyzer) analyzeFunctionCall(node *ast.TreeNode) Type {
	if len(node.Children) < 2 {
		a.errorAt(node, "invalid function call expression")
		return TypeError
	}

	funcNode := node.Children[0]
	argNodes := node.Children[1].Children
	argCount := len(argNodes)
	argTypes := make([]Type, 0, argCount)
	a.expectArgs(node, funcNode, argNodes)
	for _, arg := range argNodes {
		argTypes = append(argTypes, a.Analyze(arg))
	}
	// A call that fails to resolve still gets a plan, so the invariant checks
	// see every call; lowering never runs after an error.
	unresolved := func() Type {
		a.callPlans[node] = &ir.CallPlan{CalleeName: calleeNameFromNode(funcNode), MinArity: argCount, MaxArity: argCount, Dispatch: ir.DispatchClosure}
		return TypeError
	}

	// Method dispatch: x.method(args)
	if funcNode.NodeType == ast.MemberNode {
		result := a.resolveMethodCall(node, funcNode, argTypes, argNodes)
		if result.resolved {
			return result.returnType
		}
		receiverType := result.receiverType
		methodName := funcNode.Children[1].TokenLiteral()
		switch {
		case !isUnknownType(receiverType) && typeToBuiltinTypeKey(receiverType) != "":
			a.errorAt(funcNode, "type '%s' has no method '%s'", receiverType.String(), methodName)
		case !isUnknownType(receiverType):
			a.errorAt(funcNode, "dot-call syntax is not supported on values of type '%s'", receiverType.String())
		case !IsErrorType(receiverType):
			a.errorAt(funcNode, "no type has a method '%s'", methodName)
		}
		return unresolved()
	}

	funcExprType := a.Analyze(funcNode)
	a.recordParamCall(funcNode, argTypes, argNodes)
	if funcNode.NodeType == ast.IdentifierNode {
		name := funcNode.TokenLiteral()
		if sig, ok := a.builtins[name]; ok && a.refersToBuiltin(a.currentScope, name) {
			// An extern fn is registered as a builtin with a DispatchExtern prototype.
			if proto, isExtern := a.externFns[name]; isExtern {
				plan := *proto // copy prototype
				plan.CalleeName = name
				plan.MinArity = sig.MinArgs
				plan.MaxArity = sig.MaxArgs
				a.checkArity(node, "extern fn '"+name+"'", sig.MinArgs, sig.MaxArgs, argCount)
				a.settleArgs(sig.Type.ParamTypes, argTypes, argNodes)
				a.checkArgTypes(name, sig.Type.ParamTypes, argTypes, argNodes)
				a.callPlans[node] = &plan
				return sig.Type.ReturnType
			}
			a.callPlans[node] = &ir.CallPlan{CalleeName: name, MinArity: sig.MinArgs, MaxArity: sig.MaxArgs, Dispatch: ir.DispatchBuiltin, RuntimeSymbol: builtinsRuntimeName(name)}
			a.checkArity(node, "builtin '"+name+"'", sig.MinArgs, sig.MaxArgs, argCount)
			a.checkArgTypes(name, sig.Type.ParamTypes, argTypes, argNodes)
			a.checkPrintAlign(name, argNodes)
			return a.inferBuiltinReturnType(name, argTypes, node)
		}
	}

	funcType, ok := funcExprType.(*FunctionType)
	if !ok {
		if IsErrorType(funcExprType) {
			return unresolved()
		}
		unresolved()
		a.settleArgs(nil, argTypes, argNodes)
		// A value typed `fn` (or one of several signatures) is called as a
		// closure; its arity and argument types are checked at runtime, but
		// a call none of the possible signatures accepts is rejected now.
		if IsCallableType(funcExprType) {
			if u, isUnion := funcExprType.(*UnionType); isUnion && !anyOptionAccepts(u, argCount) {
				a.errorAt(node, "none of the functions this value can be accepts %s", pluralArgs(argCount))
			}
			return TypeAny
		}
		if !isUnknownType(funcExprType) {
			a.errorAt(funcNode, "expression is not callable")
		}
		return TypeError
	}

	minArity := funcType.MinArity()
	maxArity := len(funcType.ParamTypes)
	a.callPlans[node] = &ir.CallPlan{CalleeName: calleeNameFromNode(funcNode), MinArity: minArity, MaxArity: maxArity, Dispatch: ir.DispatchClosure, DefaultNodes: defaultNodesFromFunctionType(funcType, argCount)}
	calleeName := "function"
	what := "function"
	if funcNode.NodeType == ast.IdentifierNode {
		calleeName = funcNode.TokenLiteral()
		what = "function '" + calleeName + "'"
		a.deferFixedDispatch(a.callPlans[node], a.currentScope.Lookup(calleeName))
	}
	a.checkArity(node, what, minArity, maxArity, argCount)
	a.settleArgs(funcType.ParamTypes, argTypes, argNodes)
	a.checkArgTypes(calleeName, funcType.ParamTypes, argTypes, argNodes)
	a.checkCallbackArgs(calleeName, funcType, argTypes, argNodes)
	if native := a.tableReaderOf(funcNode); native != "" {
		return a.tableRead(node, native, argNodes)
	}
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

func inferAstypeReturnType(receiverType Type, argNodes []*ast.TreeNode) Type {
	if _, ok := receiverType.(*VectorType); !ok {
		return TypeAny
	}
	if len(argNodes) == 0 || argNodes[0] == nil || argNodes[0].Token == nil || argNodes[0].Token.Type != token.STRING {
		return &VectorType{ElementType: TypeAny}
	}
	switch argNodes[0].Token.Literal {
	case "f64":
		return &VectorType{ElementType: TypeFloat}
	case "i64":
		return &VectorType{ElementType: TypeInt}
	case "bool":
		return &VectorType{ElementType: TypeBool}
	default:
		return &VectorType{ElementType: TypeAny}
	}
}

// checkMutation checks the value a builtin method writes into its list or
// dict receiver against the receiver's element type (rule B): push, insert
// and set on lists, set on dicts, and the elements concat copies into its
// result (which has the receiver's element type).
func (a *Analyzer) checkMutation(receiverType Type, methodName string, argTypes []Type, argNodes []*ast.TreeNode) {
	valueArg := -1
	switch rt := receiverType.(type) {
	case *ListType:
		switch methodName {
		case "push":
			valueArg = 0
		case "set", "insert":
			valueArg = 1
		case "concat":
			if len(argTypes) >= 1 {
				if other, ok := argTypes[0].(*ListType); ok && !other.Pending {
					a.checkElemWrite(receiverType, other.ElementType, argNodes[0], "")
				}
			}
			return
		}
	case *DictType:
		if methodName == "set" {
			valueArg = 1
		}
	case *VectorType:
		if methodName == "fillna" && len(argTypes) == 1 {
			a.checkFillValue(rt, argTypes[0], argNodes[0])
		}
		return
	}
	if valueArg < 0 || valueArg >= len(argTypes) {
		return
	}
	key := ""
	if _, isDict := receiverType.(*DictType); isDict && len(argNodes) > 0 && argNodes[0].Token != nil && argNodes[0].Token.Type == token.STRING {
		key = argNodes[0].Token.Literal
	}
	a.checkElemWrite(receiverType, argTypes[valueArg], argNodes[valueArg], key)
}

// checkPrintAlign rejects a literal print() alignment other than 'left',
// 'right' and 'center'; a computed one is checked at runtime (q_print_impl).
func (a *Analyzer) checkPrintAlign(name string, argNodes []*ast.TreeNode) {
	if name != "print" || len(argNodes) < 4 {
		return
	}
	at := argNodes[3]
	if at == nil || at.Token == nil || at.Token.Type != token.STRING {
		return
	}
	switch at.Token.Literal {
	case "left", "right", "center":
		return
	}
	a.errorAt(at, "print() align must be 'left', 'right' or 'center', got '%s'", at.Token.Literal)
}

// checkFillValue checks the value of v.fillna(value) against the vec's
// element type, like a write into a typed list: an int fills a float vec,
// anything else must match (fillna(2.7) on a vec[int] would store 2). An
// unknown type on either side is checked at runtime (q_fillna).
func (a *Analyzer) checkFillValue(vt *VectorType, valueType Type, at *ast.TreeNode) {
	elem := vt.ElementType
	if IsErrorType(valueType) || isUnknownType(elem) || isUnknownType(valueType) {
		return
	}
	if !CanAssign(elem, valueType) {
		a.errorAt(at, "fillna() can't fill %s with %s: the fill value must have the element type (an int may fill a vec of float)", vt.String(), valueType.String())
	}
}

// settleArgs relates the arguments of a call to a user function or extern fn
// to its parameter types: a pending (empty literal) list passed to a
// `list` parameter, or to an untyped one, may get any value there; a literal
// passed to a `list` parameter holds any value (contextLiteral).
func (a *Analyzer) settleArgs(params []Type, argTypes []Type, argNodes []*ast.TreeNode) {
	for i, at := range argTypes {
		var pt Type = TypeAny
		if i < len(params) && params[i] != nil {
			pt = params[i]
		}
		if isUnknownType(pt) {
			escapePending(at)
			continue
		}
		SettleContainers(pt, at)
		if i < len(argNodes) {
			a.contextLiteral(argNodes[i], pt)
		}
	}
}

// refineMethodReturnType narrows a generic method return type using the actual
// receiver type. For example, list.get() returns TypeAny in the catalog, but if
// the receiver is list[str], the return should be str.
func refineMethodReturnType(catalogReturn Type, receiverType Type, methodName string) Type {
	if listType, ok := receiverType.(*ListType); ok {
		switch methodName {
		case "get":
			// null past the end, so not the element type; xs[i] is the
			// typed read, and errors out of range.
			return TypeAny
		case "pop", "remove":
			// Errors on an empty list or out of range, so always an element.
			return listType.ElementType
		case "concat", "push", "insert", "slice", "reverse", "sort", "sort_by", "enumerate":
			// These return a list (preserve element type for concat/push/slice/reverse)
			if methodName == "enumerate" {
				return &ListType{ElementType: TypeAny} // list of [idx, val] pairs
			}
			return receiverType
		}
	}
	if _, ok := receiverType.(*TableType); ok && (methodName == "head" || methodName == "tail") {
		return receiverType // the same rows' type
	}
	if vecType, ok := receiverType.(*VectorType); ok {
		switch methodName {
		case "get":
			// null for a missing entry, so not the element type (D12);
			// v[i] is the checked, typed read.
			return TypeAny
		case "fillna", "drop_na":
			return receiverType
		case "astype":
			return &VectorType{ElementType: TypeAny}
		case "to_list":
			return &ListType{ElementType: vecType.ElementType}
		}
	}
	if _, ok := receiverType.(*DictType); ok {
		switch methodName {
		case "get":
			// null for a missing key, so not the value type; d.key is the
			// typed read, and errors on a missing key.
			return TypeAny
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
func (a *Analyzer) analyzeUnionMethodCall(node *ast.TreeNode, unionType *UnionType, methodName string, argTypes []Type, argNodes []*ast.TreeNode, funcNode *ast.TreeNode) Type {
	argCount := len(argNodes)
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
		a.checkArity(node, "method '"+methodName+"'", sig.MinArgs, sig.MaxArgs, argCount)
		a.checkArgTypes(methodName, sig.Type.ParamTypes, argTypes, argNodes)
		returnTypes = append(returnTypes, refineMethodReturnType(sig.Type.ReturnType, opt, methodName))
	}
	// Use the first member's method for the call plan (runtime dispatch is dynamic anyway)
	firstKey := typeToBuiltinTypeKey(unionType.Options[0])
	firstSig := a.methods[firstKey][methodName]
	a.callPlans[node] = &ir.CallPlan{
		CalleeName: methodName,
		MinArity:   firstSig.MinArgs + 1, MaxArity: firstSig.MaxArgs + 1,
		Dispatch: ir.DispatchBuiltin, RuntimeSymbol: commonRuntimeSym,
		IsMethod: true, ReceiverNode: funcNode.Children[0], ReceiverTypeKey: string(firstKey),
	}
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
		return "vec"
	case builtins.TypeTableAny:
		return "tbl"
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
