package types

import (
	"quark/ast"
	"quark/builtins"
	"quark/ir"
	"quark/names"
)

func (a *Analyzer) analyzeModule(node *ast.TreeNode) Type {
	if len(node.Children) < 2 {
		a.errorAt(node, "invalid module definition")
		return TypeVoid
	}
	nameNode := node.Children[0]
	bodyNode := node.Children[1]
	if a.currentScope != a.rootScope {
		a.errorAt(nameNode, "a module must be defined at the top level of a file")
		return TypeVoid
	}
	// The loader gave every member a unique internal name (module__member),
	// so the body is analyzed in the file scope, where code outside the
	// module reaches members as module::member. Its declarations were
	// declared with the top level's (analyzeCompilationUnit).
	if bodyNode != nil && bodyNode.NodeType == ast.BlockNode {
		a.analyzeStatements(bodyNode, bodyNode.Children)
	} else {
		a.Analyze(bodyNode)
	}
	return TypeVoid
}

// Analysis is everything the analyzer hands to the invariant checks and to
// lowering. New analyzer outputs belong here so every consumer picks them up.
type Analysis struct {
	CallPlans        map[*ast.TreeNode]*ir.CallPlan
	ReturnValidation map[*ast.TreeNode]bool
	NodeTypes        map[*ast.TreeNode]Type
	// Refs maps every identifier node the analyzer resolved (reads,
	// assignment targets, declarations, parameters, loop variables, when
	// bindings, fn names) to its symbol. A symbol's Storage and CName say how
	// lowering declares and accesses it; its Decl node is where it is declared.
	Refs map[*ast.TreeNode]*Symbol
	// Captures lists, per lambda, the symbols its closure captures, in
	// capture-slot order.
	Captures    map[*ast.TreeNode][]*Symbol
	NativeFns   map[string]*ir.CallPlan
	ExternFns   map[string]*ir.CallPlan
	StructTypes map[string]*StructType
	// Names maps the loader's internal names to the names users write.
	Names names.Table

	// Fixed `fn` declarations (analyzer_fixed.go). FixedFns is keyed by the
	// declaration's lambda node. FnRefs maps identifier nodes that name a
	// declaration needing no closure to it: lowering emits its C++ function
	// (or thunk) instead of reading a binding. HoistScopes is keyed by the
	// CompilationUnit, module body or block that declares functions.
	FixedFns    map[*ast.TreeNode]*FixedFn
	FnRefs      map[*ast.TreeNode]*FixedFn
	HoistScopes map[*ast.TreeNode]*HoistScope

	// TableReads are the calls of csv::read_file and csv::parse_text, which
	// lowering turns into calls of the native reader with the row type.
	TableReads map[*ast.TreeNode]*TableRead
	// Queries are the table queries by QueryNode; ColumnRefs maps every
	// e.col of a verb expression to the column's index in the row the
	// expression reads.
	Queries    map[*ast.TreeNode]*Query
	ColumnRefs map[*ast.TreeNode]int
}

// Analysis returns the analyzer outputs for root, which must be the tree
// passed to Analyze.
func (a *Analyzer) Analysis(root *ast.TreeNode) *Analysis {
	return &Analysis{
		CallPlans:        a.callPlans,
		ReturnValidation: a.returnValidated,
		Refs:             a.refs,
		Captures:         a.captureSyms,
		NodeTypes:        a.nodeTypes,
		NativeFns:        a.nativeFns,
		ExternFns:        a.externFns,
		StructTypes:      a.structTypes,
		Names:            a.names,
		FixedFns:         a.fixedByLambda,
		FnRefs:           a.fixedRefs,
		HoistScopes:      a.hoistScopes,
		TableReads:       a.tableReads,
		Queries:          a.queries,
		ColumnRefs:       a.columnRefs,
	}
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
		a.checkExternTableType(typeNode)
		t := resolveTypeNodeStatic(typeNode) // nil typeNode → TypeAny
		if t == nil {
			t = TypeAny
		}
		paramTypes = append(paramTypes, t)
		nativeParams = append(nativeParams, quarkTypeToNativeCType(t))
	}

	// Return type
	var returnType Type = TypeAny
	a.checkExternTableType(node.ReturnType)
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

	receiverTypeStr := node.ExternReceiver // e.g. "list", "vec", "str", or ""

	if receiverTypeStr == "" {
		// Free function
		if _, exists := a.builtins[funcName]; exists {
			a.errorAt(node, "cannot redefine prelude function '%s'", funcName)
			return TypeVoid
		}
		a.builtins[funcName] = sig
		a.builtinSyms[funcName] = a.currentScope.Define(funcName, funcType, false)

		// Record the extern registration so call sites can use DispatchExtern
		a.externFns[funcName] = &ir.CallPlan{
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

// checkExternTableType rejects tbl[R] in an extern fn signature: native code
// sees a table as a QTable* and can't know R's definition, so it takes and
// returns a bare `tbl` (FFI P6), which a Quark wrapper can annotate.
func (a *Analyzer) checkExternTableType(typeNode *ast.TreeNode) {
	if typeNode != nil && typeNode.TokenLiteral() == "tbl" && len(typeNode.Children) > 0 {
		a.errorAt(typeNode, "an extern fn takes and returns a bare 'tbl', not tbl[...]: native code can't know the row struct; give a Quark function that calls it the tbl[...] annotation")
	}
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
		case "fn":
			return "QClosure*"
		}
	case *ListType:
		return "QList*"
	case *DictType:
		return "QDict*"
	case *VectorType:
		return "QVector*"
	case *FunctionType:
		return "QClosure*"
	case *TableType:
		// Only a bare tbl: a QTable* can't carry tbl[R]'s row check, so a
		// function taking or returning a tbl[R] keeps the QValue convention,
		// where q_param_table / q_ret_table check and relabel it.
		if v.Row == nil {
			return "QTable*"
		}
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
	case "vec":
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
	case "vec":
		return "QVector*"
	}
	return "QValue"
}
