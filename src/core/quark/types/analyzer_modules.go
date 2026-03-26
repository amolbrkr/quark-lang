package types

import (
	"quark/ast"
	"quark/builtins"
	"quark/ir"
)

func (a *Analyzer) analyzeModule(node *ast.TreeNode) Type {
	if len(node.Children) < 2 {
		a.errorAt(node, "invalid module definition")
		return TypeVoid
	}
	nameNode := node.Children[0]
	bodyNode := node.Children[1]
	moduleName := nameNode.TokenLiteral()
	if _, exists := a.modules[moduleName]; exists {
		a.errorAt(nameNode, "module '%s' already defined", moduleName)
		return TypeVoid
	}
	moduleScope := NewScope(a.currentScope)
	oldScope := a.currentScope
	a.currentScope = moduleScope
	a.currentModule = moduleName
	if bodyNode != nil && bodyNode.NodeType == ast.BlockNode {
		a.predeclareFunctions(bodyNode.Children)
		for _, child := range bodyNode.Children {
			a.Analyze(child)
		}
	} else {
		a.Analyze(bodyNode)
	}
	module := &Module{Name: moduleName, Scope: moduleScope, Symbols: moduleScope.Symbols}
	a.modules[moduleName] = module
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
	alias := moduleName
	hasExplicitAlias := false
	if len(node.Children) >= 2 && node.Children[1] != nil && node.Children[1].NodeType == ast.IdentifierNode {
		alias = node.Children[1].TokenLiteral()
		hasExplicitAlias = true
	}
	module, exists := a.modules[moduleName]
	if !exists {
		a.errorAt(nameNode, "undefined module '%s'", moduleName)
		return TypeVoid
	}
	if alias != "" {
		if existing, exists := a.moduleAliases[alias]; exists && existing != moduleName {
			a.errorAt(node, "module alias '%s' already refers to module '%s'", alias, existing)
		} else {
			a.moduleAliases[alias] = moduleName
		}
	}
	if hasExplicitAlias {
		return TypeVoid
	}
	for name, sym := range module.Symbols {
		if existing := a.currentScope.LookupLocal(name); existing != nil {
			a.errorAt(node, "symbol '%s' from module '%s' conflicts with existing definition", name, moduleName)
			continue
		}
		a.currentScope.Define(name, sym.Type, sym.Mutable)
	}
	return TypeVoid
}

func (a *Analyzer) GetModules() map[string]*Module {
	return a.modules
}

func (a *Analyzer) GetNodeTypes() map[*ast.TreeNode]Type {
	return a.nodeTypes
}

func (a *Analyzer) GetCaptures() map[*ast.TreeNode][]string {
	return a.captures
}

func (a *Analyzer) GetCallPlans() map[*ast.TreeNode]*ir.CallPlan {
	return a.callPlans
}

func (a *Analyzer) GetReturnValidation() map[*ast.TreeNode]bool {
	return a.returnValidated
}

// GetExternFns returns the map of extern fn registration entries (for codegen).
func (a *Analyzer) GetExternFns() map[string]*ir.CallPlan {
	return a.externFns
}

// GetNativeFns returns the map of fully-annotated user-defined function entries (for codegen).
func (a *Analyzer) GetNativeFns() map[string]*ir.CallPlan {
	return a.nativeFns
}

// isFullyAnnotated reports whether a FunctionType has explicit type annotations on
// all parameters and the return type, and all those types map to scalar C++ types.
// Functions with default-value parameters are excluded (defaults are passed as QValue).
func isFullyAnnotated(ft *FunctionType) bool {
	if ft == nil || ft.AnnotatedReturnType == nil {
		return false
	}
	if quarkTypeToNativeCType(ft.AnnotatedReturnType) == "QValue" {
		return false
	}
	for _, pt := range ft.ParamTypes {
		if pt == nil || pt.Equals(TypeAny) {
			return false
		}
		if quarkTypeToNativeCType(pt) == "QValue" {
			return false
		}
	}
	// Reject if any parameter has a default value — defaults are filled in as QValue
	// by the caller, so the native signature would need to accept QValue for those.
	if ft.DefaultCount > 0 {
		return false
	}
	return true
}

// analyzeExternFn processes an extern fn declaration and registers it into the
// same function/method tables used by builtins. Free functions go into builtins
// and the global scope; type.method declarations go into the methods table.
func (a *Analyzer) analyzeExternFn(node *ast.TreeNode) Type {
	funcName := node.TokenLiteral()
	if funcName == "" {
		a.errorAt(node, "extern fn declaration has no name")
		return TypeVoid
	}
	symbol := node.ExternSymbol
	if symbol == "" {
		a.errorAt(node, "extern fn '%s' is missing 'as' symbol", funcName)
		return TypeVoid
	}

	// Collect parameter types from ParameterNode children
	paramTypes := make([]Type, 0, len(node.Children))
	nativeParams := make([]string, 0, len(node.Children))
	for _, child := range node.Children {
		if child == nil || child.NodeType != ast.ParameterNode {
			continue
		}
		var typeNode *ast.TreeNode
		if len(child.Children) > 1 {
			typeNode = child.Children[1]
		}
		t := resolveTypeNodeStatic(typeNode) // nil typeNode → TypeAny
		if t == nil {
			t = TypeAny
		}
		paramTypes = append(paramTypes, t)
		nativeParams = append(nativeParams, quarkTypeToNativeCType(t))
	}

	// Return type
	var returnType Type = TypeAny
	if node.ReturnType != nil {
		if rt := resolveTypeNodeStatic(node.ReturnType); rt != nil {
			returnType = rt
		}
	}
	nativeReturn := quarkTypeToNativeCType(returnType)

	funcType := &FunctionType{
		ParamTypes: paramTypes,
		ReturnType: returnType,
	}
	sig := &builtinSignature{
		Type:    funcType,
		MinArgs: len(paramTypes),
		MaxArgs: len(paramTypes),
	}

	receiverTypeStr := node.ExternReceiver // e.g. "list", "vector", "str", or ""

	if receiverTypeStr == "" {
		// Free function
		if _, exists := a.builtins[funcName]; exists {
			a.errorAt(node, "cannot redefine prelude function '%s'", funcName)
			return TypeVoid
		}
		a.builtins[funcName] = sig
		a.currentScope.Define(funcName, funcType, false)
		a.functions[funcName] = funcType

		// Record the extern registration so call sites can use DispatchExtern
		a.externFns[funcName] = &ir.CallPlan{
			Kind:             ir.CallBuiltin,
			CalleeName:       funcName,
			MinArity:         len(paramTypes),
			MaxArity:         len(paramTypes),
			Dispatch:         ir.DispatchExtern,
			RuntimeSymbol:    symbol,
			NativeParamTypes: nativeParams,
			NativeReturnType: nativeReturn,
		}
	} else {
		// Method on a type
		receiverKey := receiverStringToTypeKey(receiverTypeStr)
		if receiverKey == "" {
			a.errorAt(node, "unknown receiver type '%s' in extern fn declaration", receiverTypeStr)
			return TypeVoid
		}
		nativeReceiver := receiverStringToNativeCType(receiverTypeStr)

		if a.methods[receiverKey] == nil {
			a.methods[receiverKey] = make(map[string]*builtinSignature)
		}
		if _, isBuiltin := builtins.LookupMethod(receiverKey, funcName); isBuiltin {
			a.errorAt(node, "cannot redefine prelude method '%s.%s'", receiverTypeStr, funcName)
			return TypeVoid
		}
		a.methods[receiverKey][funcName] = sig

		methodKey := receiverTypeStr + "." + funcName
		a.externFns[methodKey] = &ir.CallPlan{
			Kind:               ir.CallBuiltin,
			CalleeName:         funcName,
			MinArity:           len(paramTypes) + 1,
			MaxArity:           len(paramTypes) + 1,
			Dispatch:           ir.DispatchExtern,
			RuntimeSymbol:      symbol,
			IsMethod:           true,
			NativeParamTypes:   nativeParams,
			NativeReturnType:   nativeReturn,
			NativeReceiverType: nativeReceiver,
		}
	}

	return TypeVoid
}

// quarkTypeToNativeCType maps a Quark Type to the C++ type string used in extern fn ABI.
func quarkTypeToNativeCType(t Type) string {
	if t == nil {
		return "QValue"
	}
	switch v := t.(type) {
	case *BasicType:
		switch v.Name {
		case "int":
			return "int64_t"
		case "float":
			return "double"
		case "bool":
			return "bool"
		case "str":
			return "const char*"
		}
	case *ListType:
		return "QList*"
	case *DictType:
		return "QDict*"
	case *VectorType:
		return "QVector*"
	case *FunctionType:
		return "QClosure*"
	}
	if t.Equals(TypeAny) {
		return "QValue"
	}
	return "QValue"
}

// receiverStringToTypeKey maps the receiver string from an extern fn declaration
// to the catalog TypeKey used for method lookup.
func receiverStringToTypeKey(s string) builtins.TypeKey {
	switch s {
	case "int":
		return builtins.TypeInt
	case "float":
		return builtins.TypeFloat
	case "str":
		return builtins.TypeString
	case "bool":
		return builtins.TypeBool
	case "list":
		return builtins.TypeListAny
	case "dict":
		return builtins.TypeDictAny
	case "vector":
		return builtins.TypeVectorAny
	}
	return ""
}

// receiverStringToNativeCType maps the receiver type name to its C++ type.
func receiverStringToNativeCType(s string) string {
	switch s {
	case "int":
		return "int64_t"
	case "float":
		return "double"
	case "bool":
		return "bool"
	case "str":
		return "const char*"
	case "list":
		return "QList*"
	case "dict":
		return "QDict*"
	case "vector":
		return "QVector*"
	}
	return "QValue"
}

// GetCapturedByFunction returns, for each function or lambda node, the set of
// variable names that any directly-nested lambda captures from that scope.
// Codegen uses this to decide: QCell* (captured) vs QValue (stack-local).
func (a *Analyzer) GetCapturedByFunction(root *ast.TreeNode) map[*ast.TreeNode]map[string]bool {
	result := make(map[*ast.TreeNode]map[string]bool)
	if root == nil || len(a.captures) == 0 {
		return result
	}

	// Walk the AST tracking the immediately enclosing function/lambda for each node.
	// When we hit a LambdaNode that has captures, attribute those names to its
	// enclosing function/lambda — that is the scope that owns the variables.
	var walk func(n *ast.TreeNode, enclosing *ast.TreeNode)
	walk = func(n *ast.TreeNode, enclosing *ast.TreeNode) {
		if n == nil {
			return
		}
		if n.NodeType == ast.LambdaNode {
			if enclosing != nil {
				if names, ok := a.captures[n]; ok {
					if result[enclosing] == nil {
						result[enclosing] = make(map[string]bool)
					}
					for _, name := range names {
						result[enclosing][name] = true
					}
				}
			}
			// Children of this lambda are now enclosed by it.
			for _, child := range n.Children {
				walk(child, n)
			}
			return
		}
		if n.NodeType == ast.FunctionNode {
			for _, child := range n.Children {
				walk(child, n)
			}
			return
		}
		for _, child := range n.Children {
			walk(child, enclosing)
		}
	}
	walk(root, nil)
	return result
}
