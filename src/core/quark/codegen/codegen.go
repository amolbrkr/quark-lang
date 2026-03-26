package codegen

import (
	"fmt"
	"os"
	"quark/ast"
	"quark/ir"
	"quark/token"
	"quark/types"
	"strings"
)

// escapeCppString escapes a Go string for safe embedding in a C++ string literal.
func escapeCppString(s string) string {
	escaped := strings.ReplaceAll(s, "\\", "\\\\")
	escaped = strings.ReplaceAll(escaped, "\"", "\\\"")
	escaped = strings.ReplaceAll(escaped, "\n", "\\n")
	escaped = strings.ReplaceAll(escaped, "\t", "\\t")
	escaped = strings.ReplaceAll(escaped, "\r", "\\r")
	return escaped
}

// panicICEf emits a fatal internal compiler error with source location context.
func panicICEf(code string, node *ast.TreeNode, format string, args ...interface{}) {
	msg := fmt.Sprintf(format, args...)
	line := 0
	if node != nil && node.Token != nil {
		line = node.Token.Line
	}
	fmt.Fprintf(os.Stderr, "internal compiler error [%s]: %s at line %d\n", code, msg, line)
	os.Exit(2)
}

// funcDecl stores a function name and signature info for forward declarations.
// If nativeParamTypes is non-nil, the function uses a native C++ signature.
type funcDecl struct {
	name             string
	paramCount       int
	nativeParamTypes []string // nil = QValue params; non-nil = native-typed
	nativeReturnType string   // "" = QValue return
}

// Generator generates C code from an AST
type Generator struct {
	output        strings.Builder
	indentLevel   int
	sourceName    string
	funcDecls     []funcDecl               // Function declarations with param counts
	lambdas       []*ast.TreeNode          // Lambda expressions to generate
	lambdaNames   map[*ast.TreeNode]string // Maps lambda nodes to their generated names
	tempCounter   int
	lambdaCounter int
	inFunction    bool
	currentFunc   string
	declaredVars  map[string]bool            // Tracks declared variables to avoid redeclaration
	cellVars      map[string]bool            // Subset of declaredVars that are QCell* (vs QValue)
	scopeStack    []map[string]bool          // Stack of variable scopes for nested blocks
	cellStack     []map[string]bool          // Mirrors scopeStack for cellVars
	captures      map[*ast.TreeNode][]string // Lambda node → captured variable names (from analyzer)
	funcNames     map[string]bool            // Set of declared function names (for first-class funcs)
	callPlans     map[*ast.TreeNode]*ir.CallPlan
	// capturedByFunction maps each function/lambda node to the set of variable
	// names that any directly-nested lambda captures from it. A variable NOT in
	// this set for the current function can be emitted as a stack QValue instead
	// of a heap-allocated QCell*.
	capturedByFunction map[*ast.TreeNode]map[string]bool
	// currentFuncNode is the AST node for the function/lambda currently being
	// generated. Used to look up capturedByFunction.
	currentFuncNode *ast.TreeNode
	// nodeTypes maps every AST expression node to the type the analyzer inferred
	// for it. Codegen uses this to decide scalar storage and native operator lowering.
	nodeTypes map[*ast.TreeNode]types.Type
	// varTiers tracks the C++ storage tier chosen for each declared variable in
	// the current scope: "long long", "double", "bool", or "" (meaning QValue).
	// Mirrors declaredVars / scopeStack.
	varTiers  map[string]string
	tierStack []map[string]string
	// nativeFns maps function name → CallPlan for fully-annotated user functions.
	// Used by collectFunctions and generateFunction to emit native signatures + thunks.
	nativeFns map[string]*ir.CallPlan
}

func New() *Generator {
	return &Generator{
		funcDecls:          make([]funcDecl, 0),
		lambdas:            make([]*ast.TreeNode, 0),
		lambdaNames:        make(map[*ast.TreeNode]string),
		tempCounter:        0,
		sourceName:         "<unknown>",
		declaredVars:       make(map[string]bool),
		cellVars:           make(map[string]bool),
		scopeStack:         make([]map[string]bool, 0),
		cellStack:          make([]map[string]bool, 0),
		captures:           make(map[*ast.TreeNode][]string),
		funcNames:          make(map[string]bool),
		callPlans:          make(map[*ast.TreeNode]*ir.CallPlan),
		capturedByFunction: make(map[*ast.TreeNode]map[string]bool),
		nodeTypes:          make(map[*ast.TreeNode]types.Type),
		varTiers:           make(map[string]string),
		tierStack:          make([]map[string]string, 0),
		nativeFns:          make(map[string]*ir.CallPlan),
	}
}

// SetNativeFns passes the fully-annotated function map from the analyzer to the generator.
func (g *Generator) SetNativeFns(m map[string]*ir.CallPlan) {
	if m != nil {
		g.nativeFns = m
	}
}

// SetCapturedByFunction passes the per-function captured-variable sets from the
// analyzer so codegen can elide QCell* for variables that are never captured.
func (g *Generator) SetCapturedByFunction(m map[*ast.TreeNode]map[string]bool) {
	if m != nil {
		g.capturedByFunction = m
	}
}

// SetNodeTypes passes the per-node inferred types from the analyzer to the generator.
func (g *Generator) SetNodeTypes(m map[*ast.TreeNode]types.Type) {
	if m != nil {
		g.nodeTypes = m
	}
}

// nodeType returns the analyzer-inferred type for a given AST node, or nil if unknown.
func (g *Generator) nodeType(node *ast.TreeNode) types.Type {
	if node == nil {
		return nil
	}
	return g.nodeTypes[node]
}

// nativeCType returns the C++ scalar type string for a Quark type, or "" if the
// type must stay boxed as QValue (any, unknown, composite, or nil).
func nativeCType(t types.Type) string {
	if t == nil {
		return ""
	}
	if t.Equals(types.TypeInt) {
		return "long long"
	}
	if t.Equals(types.TypeFloat) {
		return "double"
	}
	if t.Equals(types.TypeBool) {
		return "bool"
	}
	return ""
}

// markTier records the C++ storage tier for varName in the current scope.
func (g *Generator) markTier(name string, ctype string) {
	g.varTiers[name] = ctype
}

// tierOf returns the C++ storage tier for varName, or "" for QValue/unknown.
func (g *Generator) tierOf(name string) string {
	return g.varTiers[name]
}

// boxExpr wraps a C++ expression of the given tier into a QValue.
// If tier is "" (already QValue), expr is returned unchanged.
func boxExpr(expr string, tier string) string {
	switch tier {
	case "long long":
		return fmt.Sprintf("qv_int(%s)", expr)
	case "double":
		return fmt.Sprintf("qv_float(%s)", expr)
	case "bool":
		return fmt.Sprintf("qv_bool(%s)", expr)
	}
	return expr
}

// nativeCppTypeToTier maps a C++ native type string (as used in NativeParamTypes)
// to the varTier string used by scalarExpr and the scalar lowering pass.
func nativeCppTypeToTier(cppType string) string {
	switch cppType {
	case "int64_t":
		return "long long"
	case "double":
		return "double"
	case "bool":
		return "bool"
	}
	return ""
}

// unboxToNative ensures a generateExpr result (always QValue) is unboxed to the
// given C++ native return type. If retType is QValue or empty, returns expr unchanged.
func unboxToNative(expr string, retType string) string {
	tier := nativeCppTypeToTier(retType)
	if tier == "" {
		return expr // already QValue or non-scalar
	}
	field := unboxFieldAccessor(tier)
	if field != "" {
		return fmt.Sprintf("(%s).data.%s", expr, field)
	}
	return expr
}

// unboxForTier extracts the raw scalar value from a QValue expression.
// generateExpr always returns QValue, so we always use .data field access.
func unboxForTier(expr string, tier string, _ types.Type) string {
	field := unboxFieldAccessor(tier)
	if field != "" {
		return fmt.Sprintf("(%s).data.%s", expr, field)
	}
	return expr
}

// isCaptured returns true if the named variable is captured by a nested lambda
// within the function currently being generated, meaning it must stay a QCell*.
func (g *Generator) isCaptured(name string) bool {
	if g.currentFuncNode == nil {
		return false
	}
	if captured := g.capturedByFunction[g.currentFuncNode]; captured != nil {
		return captured[name]
	}
	return false
}

// markCell records that varName is stored as QCell* in the current scope.
func (g *Generator) markCell(name string) {
	g.cellVars[name] = true
}

// isCell reports whether varName is stored as QCell* in the current scope.
func (g *Generator) isCell(name string) bool {
	return g.cellVars[name]
}

// SetSourceName configures the source file label used for runtime diagnostics.
func (g *Generator) SetSourceName(name string) {
	if strings.TrimSpace(name) == "" {
		return
	}
	g.sourceName = name
}

// emitSourceLoc emits a q_set_source_loc() call before a statement for runtime error context.
func (g *Generator) emitSourceLoc(node *ast.TreeNode) {
	if node == nil || node.Token == nil {
		return
	}
	g.emitLine("q_set_source_loc(\"%s\", %d, %d);", escapeCppString(g.sourceName), node.Token.Line, node.Token.Column)
}

// SetCaptures passes the captured variable info from the analyzer to the generator
func (g *Generator) SetCaptures(captures map[*ast.TreeNode][]string) {
	if captures != nil {
		g.captures = captures
	}
}

// SetCallPlans passes call-focused IR metadata from the analyzer to the generator.
func (g *Generator) SetCallPlans(plans map[*ast.TreeNode]*ir.CallPlan) {
	if plans != nil {
		g.callPlans = plans
	}
}

// sanitizeVarName prefixes all user variable names with quark_ to avoid
// collisions with C++ reserved keywords. This is the same prefix used for
// user-defined functions, giving a uniform naming scheme in generated code.
func sanitizeVarName(name string) string {
	return "quark_" + name
}

func sanitizeArgName(name string) string {
	return "_arg_" + sanitizeVarName(name)
}

func isFunctionBindingAssignmentNode(node *ast.TreeNode) bool {
	if node == nil || node.NodeType != ast.OperatorNode || node.Token == nil || node.Token.Type != token.EQUALS {
		return false
	}
	if len(node.Children) != 2 {
		return false
	}
	return node.Children[0].NodeType == ast.IdentifierNode && node.Children[1].NodeType == ast.LambdaNode
}

func (g *Generator) indent() string {
	return strings.Repeat("    ", g.indentLevel)
}

func (g *Generator) emit(format string, args ...interface{}) {
	g.output.WriteString(fmt.Sprintf(format, args...))
}

func (g *Generator) emitLine(format string, args ...interface{}) {
	g.output.WriteString(g.indent())
	g.output.WriteString(fmt.Sprintf(format, args...))
	g.output.WriteString("\n")
}

func (g *Generator) newTemp() string {
	g.tempCounter++
	return fmt.Sprintf("_t%d", g.tempCounter)
}

func (g *Generator) newLambda() string {
	g.lambdaCounter++
	return fmt.Sprintf("_lambda%d", g.lambdaCounter)
}

func (g *Generator) paramName(node *ast.TreeNode) string {
	if node == nil {
		return ""
	}
	if node.NodeType == ast.ParameterNode && len(node.Children) > 0 {
		return node.Children[0].TokenLiteral()
	}
	return node.TokenLiteral()
}

// pushScope saves current variable scope and creates an isolated one (used for functions)
func (g *Generator) pushScope() {
	g.scopeStack = append(g.scopeStack, g.declaredVars)
	g.cellStack = append(g.cellStack, g.cellVars)
	g.tierStack = append(g.tierStack, g.varTiers)
	g.declaredVars = make(map[string]bool)
	g.cellVars = make(map[string]bool)
	g.varTiers = make(map[string]string)
}

func (g *Generator) pushBlockScope() {
	parent := g.declaredVars
	parentCells := g.cellVars
	parentTiers := g.varTiers
	g.scopeStack = append(g.scopeStack, parent)
	g.cellStack = append(g.cellStack, parentCells)
	g.tierStack = append(g.tierStack, parentTiers)
	child := make(map[string]bool)
	for k, v := range parent {
		child[k] = v
	}
	childCells := make(map[string]bool)
	for k, v := range parentCells {
		childCells[k] = v
	}
	childTiers := make(map[string]string)
	for k, v := range parentTiers {
		childTiers[k] = v
	}
	g.declaredVars = child
	g.cellVars = childCells
	g.varTiers = childTiers
}

// popScope restores the previous variable scope
func (g *Generator) popScope() {
	if len(g.scopeStack) > 0 {
		g.declaredVars = g.scopeStack[len(g.scopeStack)-1]
		g.scopeStack = g.scopeStack[:len(g.scopeStack)-1]
	}
	if len(g.cellStack) > 0 {
		g.cellVars = g.cellStack[len(g.cellStack)-1]
		g.cellStack = g.cellStack[:len(g.cellStack)-1]
	}
	if len(g.tierStack) > 0 {
		g.varTiers = g.tierStack[len(g.tierStack)-1]
		g.tierStack = g.tierStack[:len(g.tierStack)-1]
	}
}

// collectExternSources gathers all resolved extern source paths from the AST
// (ExternSourceNode children with their absolute path in the token literal).
func collectExternSources(node *ast.TreeNode) []string {
	var paths []string
	seen := make(map[string]bool)
	var walk func(*ast.TreeNode)
	walk = func(n *ast.TreeNode) {
		if n == nil {
			return
		}
		if n.NodeType == ast.ExternSourceNode && len(n.Children) > 0 {
			pathNode := n.Children[0]
			if pathNode != nil && pathNode.Token != nil && pathNode.Token.Literal != "" {
				p := pathNode.Token.Literal
				if !seen[p] {
					seen[p] = true
					paths = append(paths, p)
				}
			}
			return
		}
		for _, child := range n.Children {
			walk(child)
		}
	}
	walk(node)
	return paths
}

// adaptArgForExtern adapts a QValue expression to the native C++ type required by an extern fn.
// If nativeType is "QValue" or the same as the source, the expression is returned unchanged.
// Otherwise an unboxing helper is emitted.
func adaptArgForExtern(expr string, nativeType string) string {
	switch nativeType {
	case "int64_t":
		return fmt.Sprintf("q_as_int(%s)", expr)
	case "double":
		return fmt.Sprintf("q_as_float(%s)", expr)
	case "bool":
		return fmt.Sprintf("q_as_bool(%s)", expr)
	case "const char*":
		return fmt.Sprintf("q_as_str(%s)", expr)
	case "QVector*":
		return fmt.Sprintf("q_as_vector(%s)", expr)
	case "QList*":
		return fmt.Sprintf("q_as_list(%s)", expr)
	case "QDict*":
		return fmt.Sprintf("q_as_dict(%s)", expr)
	case "QClosure*":
		return fmt.Sprintf("q_as_closure(%s)", expr)
	case "QValue", "":
		return expr
	default:
		return expr
	}
}

// wrapExternReturn wraps the result of a native extern call back into QValue.
func wrapExternReturn(callExpr string, nativeReturn string) string {
	switch nativeReturn {
	case "int64_t":
		return fmt.Sprintf("qv_int(%s)", callExpr)
	case "double":
		return fmt.Sprintf("qv_float(%s)", callExpr)
	case "bool":
		return fmt.Sprintf("qv_bool(%s)", callExpr)
	case "const char*":
		return fmt.Sprintf("qv_string(%s)", callExpr)
	case "QVector*":
		return fmt.Sprintf("qv_vector_ptr(%s)", callExpr)
	case "QList*":
		return fmt.Sprintf("qv_list_ptr(%s)", callExpr)
	case "QDict*":
		return fmt.Sprintf("qv_dict_ptr(%s)", callExpr)
	case "QValue", "":
		return callExpr
	default:
		return callExpr
	}
}

// Generate produces C++ code from the AST
func (g *Generator) Generate(node *ast.TreeNode) string {
	// Use external modular runtime header.
	g.output.WriteString("#include \"quark/quark.hpp\"\n")

	// Emit extern source includes before any generated code
	externPaths := collectExternSources(node)
	if len(externPaths) > 0 {
		g.output.WriteString("\n// Extension includes\n")
		for _, p := range externPaths {
			g.output.WriteString(fmt.Sprintf("#include \"%s\"\n", escapeCppString(p)))
		}
	}
	g.output.WriteString("\n")

	g.output.WriteString("// Forward declarations\n")

	// First pass: collect function declarations
	g.collectFunctions(node)

	// Emit forward declarations with correct parameter counts.
	// All functions take QClosure* as hidden first parameter for closure support.
	// Native functions also get a QValue-based thunk for first-class use.
	for _, fd := range g.funcDecls {
		if fd.nativeParamTypes != nil {
			// Native-typed signature
			retType := fd.nativeReturnType
			if retType == "" {
				retType = "QValue"
			}
			params := []string{"QClosure*"}
			for _, pt := range fd.nativeParamTypes {
				params = append(params, pt)
			}
			g.emitLine("%s quark_%s(%s);", retType, fd.name, strings.Join(params, ", "))
			// Thunk forward decl: same QValue-based signature as a regular function
			thunkParams := []string{"QClosure*"}
			for i := 0; i < fd.paramCount; i++ {
				thunkParams = append(thunkParams, "QValue")
			}
			g.emitLine("QValue quark_%s__thunk(%s);", fd.name, strings.Join(thunkParams, ", "))
		} else {
			params := []string{"QClosure*"}
			for i := 0; i < fd.paramCount; i++ {
				params = append(params, "QValue")
			}
			g.emitLine("QValue quark_%s(%s);", fd.name, strings.Join(params, ", "))
		}
	}
	g.emit("\n")

	// Generate function definitions
	g.generateNode(node)

	// Generate main function
	g.emit("\nint main() {\n")
	g.indentLevel++
	g.emitLine("q_gc_init();")
	g.predeclareMainFunctionBindings(node)

	// Initialize module body bindings so imported symbols are available at runtime.
	for _, child := range node.Children {
		if child.NodeType == ast.ModuleNode {
			g.generateModuleBindings(child)
		}
	}

	// Generate top-level statements that aren't function/module/extern definitions
	for _, child := range node.Children {
		if child.NodeType == ast.FunctionNode || child.NodeType == ast.ModuleNode ||
			child.NodeType == ast.UseNode || child.NodeType == ast.ExternSourceNode ||
			child.NodeType == ast.ExternFnNode {
			continue
		}
		g.emitSourceLoc(child)
		g.emitLine("%s;", g.generateExpr(child))
	}

	g.emitLine("return 0;")
	g.indentLevel--
	g.emit("}\n")

	return g.output.String()
}

func (g *Generator) predeclareMainFunctionBindings(root *ast.TreeNode) {
	if root == nil {
		return
	}
	declareBinding := func(name string) {
		if name == "" || g.declaredVars[name] {
			return
		}
		cName := sanitizeVarName(name)
		g.emitLine("QValue %s = qv_null();", cName)
		g.declaredVars[name] = true
	}

	for _, child := range root.Children {
		if isFunctionBindingAssignmentNode(child) {
			declareBinding(child.Children[0].TokenLiteral())
			continue
		}
		if child.NodeType != ast.ModuleNode || len(child.Children) < 2 {
			continue
		}
		body := child.Children[1]
		if body == nil || body.NodeType != ast.BlockNode {
			continue
		}
		for _, stmt := range body.Children {
			if isFunctionBindingAssignmentNode(stmt) {
				declareBinding(stmt.Children[0].TokenLiteral())
			}
		}
	}
}

func (g *Generator) collectFunctions(node *ast.TreeNode) {
	switch node.NodeType {
	case ast.FunctionNode:
		if len(node.Children) >= 2 {
			name := node.Children[0].TokenLiteral()
			argsNode := node.Children[1]
			paramCount := len(argsNode.Children)
			fd := funcDecl{name: name, paramCount: paramCount}
			if proto, isNative := g.nativeFns[name]; isNative {
				fd.nativeParamTypes = proto.NativeParamTypes
				fd.nativeReturnType = proto.NativeReturnType
			}
			g.funcDecls = append(g.funcDecls, fd)
			g.funcNames[name] = true
		}
	case ast.LambdaNode:
		// If this lambda was already named (via the function-binding assignment path
		// below), skip it — it is already registered.
		if _, already := g.lambdaNames[node]; already {
			return
		}
		// Anonymous lambda: assign an auto-generated name.
		lambdaName := g.newLambda()
		g.lambdaNames[node] = lambdaName
		g.lambdas = append(g.lambdas, node)
		paramCount := 0
		if len(node.Children) >= 1 {
			paramCount = len(node.Children[0].Children)
		}
		fd := funcDecl{
			name:       lambdaName,
			paramCount: paramCount,
		}
		g.funcDecls = append(g.funcDecls, fd)
	case ast.OperatorNode:
		// Detect `name = fn(...)` function-binding assignments.
		// For native functions, use the user-visible name as the C++ function name
		// so call sites can emit `quark_name(nullptr, ...)` directly.
		if node.Token != nil && node.Token.Type == token.EQUALS &&
			len(node.Children) == 2 &&
			node.Children[0] != nil && node.Children[0].NodeType == ast.IdentifierNode &&
			node.Children[1] != nil && node.Children[1].NodeType == ast.LambdaNode {
			userName := node.Children[0].TokenLiteral()
			lambdaNode := node.Children[1]
			if proto, isNative := g.nativeFns[userName]; isNative {
				// Give the lambda the canonical user name so codegen emits quark_<name>.
				g.lambdaNames[lambdaNode] = userName
				g.lambdas = append(g.lambdas, lambdaNode)
				g.funcNames[userName] = true
				paramCount := 0
				if len(lambdaNode.Children) >= 1 {
					paramCount = len(lambdaNode.Children[0].Children)
				}
				g.funcDecls = append(g.funcDecls, funcDecl{
					name:             userName,
					paramCount:       paramCount,
					nativeParamTypes: proto.NativeParamTypes,
					nativeReturnType: proto.NativeReturnType,
				})
				// Recurse into children but the lambda itself will be skipped above.
				for _, child := range node.Children {
					g.collectFunctions(child)
				}
				return
			}
		}
	case ast.ModuleNode:
		if len(node.Children) >= 2 {
			bodyNode := node.Children[1]
			// Collect functions from module body (without prefix)
			g.collectFunctions(bodyNode)
		}
		return // Don't recurse further, we handled the module body
	}
	for _, child := range node.Children {
		g.collectFunctions(child)
	}
}

func (g *Generator) generateNode(node *ast.TreeNode) {
	switch node.NodeType {
	case ast.CompilationUnitNode:
		for _, child := range node.Children {
			g.generateNode(child)
		}
		// Generate all collected lambdas
		for _, lambda := range g.lambdas {
			g.generateLambdaFunc(lambda)
		}
	case ast.FunctionNode:
		g.generateFunction(node)
	case ast.ModuleNode:
		g.generateModule(node)
	case ast.ExternSourceNode, ast.ExternFnNode:
		// No code emitted — extern sources are #included in preamble,
		// extern fn declarations are registered in the analyzer.
	}
}

func (g *Generator) generateFunction(node *ast.TreeNode) {
	if len(node.Children) < 3 {
		return
	}

	nameNode := node.Children[0]
	argsNode := node.Children[1]
	bodyNode := node.Children[2]

	funcName := nameNode.TokenLiteral()
	proto := g.nativeFns[funcName] // nil if not a native fn

	if proto != nil {
		g.generateNativeFunction(node, funcName, argsNode, bodyNode, proto)
	} else {
		g.generateQValueFunction(node, funcName, argsNode, bodyNode)
	}
}

// generateQValueFunction emits the standard QValue-based function body.
func (g *Generator) generateQValueFunction(node *ast.TreeNode, funcName string, argsNode *ast.TreeNode, bodyNode *ast.TreeNode) {
	g.currentFunc = funcName
	g.currentFuncNode = node
	g.inFunction = true
	g.pushScope()

	params := []string{"QClosure* _cl"}
	for _, param := range argsNode.Children {
		paramName := g.paramName(param)
		if paramName == "" {
			continue
		}
		params = append(params, fmt.Sprintf("QValue %s", sanitizeArgName(paramName)))
	}

	g.emit("QValue quark_%s(%s) {\n", funcName, strings.Join(params, ", "))
	g.indentLevel++

	for _, param := range argsNode.Children {
		paramName := g.paramName(param)
		if paramName == "" {
			continue
		}
		cName := sanitizeVarName(paramName)
		argName := sanitizeArgName(paramName)
		if g.isCaptured(paramName) {
			g.emitLine("QCell* %s = q_new_cell(%s);", cName, argName)
			g.markCell(paramName)
		} else {
			g.emitLine("QValue %s = %s;", cName, argName)
		}
		g.declaredVars[paramName] = true
	}

	result := g.generateBlock(bodyNode)
	g.emitLine("return %s;", g.boxResultToQValue(result, bodyNode))

	g.indentLevel--
	g.emit("}\n\n")

	g.popScope()
	g.inFunction = false
	g.currentFuncNode = nil
}

// generateNativeFunction emits a native-typed function body and a QValue thunk.
func (g *Generator) generateNativeFunction(node *ast.TreeNode, funcName string, argsNode *ast.TreeNode, bodyNode *ast.TreeNode, proto *ir.CallPlan) {
	g.currentFunc = funcName
	g.currentFuncNode = node
	g.inFunction = true
	g.pushScope()

	// Collect param names in order
	paramNames := make([]string, 0, len(argsNode.Children))
	for _, param := range argsNode.Children {
		paramNames = append(paramNames, g.paramName(param))
	}

	// Native-typed signature
	retType := proto.NativeReturnType
	if retType == "" {
		retType = "QValue"
	}
	params := []string{"QClosure* _cl"}
	for i, paramName := range paramNames {
		if paramName == "" {
			continue
		}
		nativeType := "QValue"
		if i < len(proto.NativeParamTypes) {
			nativeType = proto.NativeParamTypes[i]
		}
		params = append(params, fmt.Sprintf("%s %s", nativeType, sanitizeArgName(paramName)))
	}

	g.emit("%s quark_%s(%s) {\n", retType, funcName, strings.Join(params, ", "))
	g.indentLevel++

	for i, paramName := range paramNames {
		if paramName == "" {
			continue
		}
		cName := sanitizeVarName(paramName)
		argName := sanitizeArgName(paramName)
		nativeType := "QValue"
		if i < len(proto.NativeParamTypes) {
			nativeType = proto.NativeParamTypes[i]
		}
		// For native params, store directly at the native tier (no unboxing needed).
		// Captured params still need a QCell, but must be boxed first.
		if g.isCaptured(paramName) {
			boxed := boxExpr(argName, nativeCppTypeToTier(nativeType))
			g.emitLine("QCell* %s = q_new_cell(%s);", cName, boxed)
			g.markCell(paramName)
		} else {
			g.emitLine("%s %s = %s;", nativeType, cName, argName)
			g.markTier(paramName, nativeCppTypeToTier(nativeType))
		}
		g.declaredVars[paramName] = true
	}

	// For the return value, try scalarExpr on the last body expression to avoid
	// the box-then-unbox roundtrip (e.g. emit `x + y` directly instead of
	// `(qv_int(x + y)).data.int_val`).
	retTier := nativeCppTypeToTier(retType)
	var nativeResult string
	if retTier != "" {
		// Find the last statement node to try scalarExpr on.
		var lastStmt *ast.TreeNode
		if bodyNode != nil && bodyNode.NodeType == ast.BlockNode && len(bodyNode.Children) > 0 {
			lastStmt = bodyNode.Children[len(bodyNode.Children)-1]
		} else {
			lastStmt = bodyNode
		}
		if rawVal, tier := g.scalarExpr(lastStmt); tier == retTier {
			// Emit all but the last body statement, then return raw.
			if bodyNode != nil && bodyNode.NodeType == ast.BlockNode {
				g.pushBlockScope()
				for i, child := range bodyNode.Children {
					if i < len(bodyNode.Children)-1 {
						g.emitSourceLoc(child)
						g.emitLine("%s;", g.generateExpr(child))
					}
				}
				g.popScope()
			}
			nativeResult = rawVal
		}
	}
	if nativeResult == "" {
		result := g.generateBlock(bodyNode)
		nativeResult = unboxToNative(result, retType)
	}
	g.emitLine("return %s;", nativeResult)

	g.indentLevel--
	g.emit("}\n\n")

	g.popScope()
	g.inFunction = false
	g.currentFuncNode = nil

	// Emit QValue thunk so the function can be used as a first-class value.
	g.generateNativeThunk(funcName, paramNames, proto)
}

// generateNativeThunk emits a QValue-typed wrapper that unboxes args, calls the
// native function, and boxes the result back. Used when the function is passed as
// a first-class value.
func (g *Generator) generateNativeThunk(funcName string, paramNames []string, proto *ir.CallPlan) {
	retType := proto.NativeReturnType
	if retType == "" {
		retType = "QValue"
	}

	thunkParams := []string{"QClosure* _cl"}
	for _, paramName := range paramNames {
		if paramName == "" {
			continue
		}
		thunkParams = append(thunkParams, fmt.Sprintf("QValue %s", sanitizeArgName(paramName)))
	}
	g.emit("QValue quark_%s__thunk(%s) {\n", funcName, strings.Join(thunkParams, ", "))
	g.indentLevel++

	// Adapt each QValue arg to its native type
	adaptedArgs := []string{"nullptr"}
	for i, paramName := range paramNames {
		if paramName == "" {
			continue
		}
		argName := sanitizeArgName(paramName)
		nativeType := "QValue"
		if i < len(proto.NativeParamTypes) {
			nativeType = proto.NativeParamTypes[i]
		}
		adaptedArgs = append(adaptedArgs, adaptArgForExtern(argName, nativeType))
	}

	callExpr := fmt.Sprintf("quark_%s(%s)", funcName, strings.Join(adaptedArgs, ", "))
	g.emitLine("return %s;", wrapExternReturn(callExpr, retType))

	g.indentLevel--
	g.emit("}\n\n")
}

func (g *Generator) generateModule(node *ast.TreeNode) {
	if len(node.Children) < 2 {
		return
	}

	bodyNode := node.Children[1]

	// Generate module functions (all functions are global in the C output)
	for _, child := range bodyNode.Children {
		if child.NodeType == ast.FunctionNode {
			g.generateFunction(child)
		}
	}
}

func (g *Generator) generateModuleBindings(node *ast.TreeNode) {
	if len(node.Children) < 2 {
		return
	}
	bodyNode := node.Children[1]
	if bodyNode == nil || bodyNode.NodeType != ast.BlockNode {
		return
	}
	for _, child := range bodyNode.Children {
		if child == nil || child.NodeType == ast.UseNode || child.NodeType == ast.ModuleNode {
			continue
		}
		g.emitSourceLoc(child)
		expr := g.generateExpr(child)
		g.emitLine("%s;", expr)
	}
}

func (g *Generator) generateBlock(node *ast.TreeNode) string {
	if node == nil {
		return "qv_null()"
	}
	g.pushBlockScope()
	defer g.popScope()
	var lastExpr string = "qv_null()"
	for idx, child := range node.Children {
		g.emitSourceLoc(child)
		lastExpr = g.generateExpr(child)
		if idx < len(node.Children)-1 {
			g.emitLine("%s;", lastExpr)
		}
	}
	return lastExpr
}

func (g *Generator) generateExpr(node *ast.TreeNode) string {
	if node == nil {
		return "qv_null()"
	}

	switch node.NodeType {
	case ast.LiteralNode:
		return g.generateLiteral(node)
	case ast.IdentifierNode:
		return g.generateIdentifier(node)
	case ast.OperatorNode:
		return g.generateOperator(node)
	case ast.FunctionCallNode:
		return g.generateFunctionCall(node)
	case ast.PipeNode:
		return g.generatePipe(node)
	case ast.TernaryNode:
		return g.generateTernary(node)
	case ast.IfStatementNode:
		return g.generateIf(node)
	case ast.WhenStatementNode:
		return g.generateWhen(node)
	case ast.ForLoopNode:
		return g.generateFor(node)
	case ast.WhileLoopNode:
		return g.generateWhile(node)
	case ast.ListNode:
		return g.generateList(node)
	case ast.VectorNode:
		return g.generateVector(node)
	case ast.DictNode:
		return g.generateDict(node)
	case ast.IndexNode:
		return g.generateIndex(node)
	case ast.ResultNode:
		return g.generateResult(node)
	case ast.VarDeclNode:
		return g.generateVarDecl(node)
	case ast.LambdaNode:
		return g.generateLambdaExpr(node)
	case ast.BlockNode:
		return g.generateBlock(node)
	case ast.ModuleNode:
		// Module definitions are handled at top level
		return "qv_null()"
	case ast.UseNode:
		// Use statements are handled at compile time (imports are resolved by analyzer)
		return "qv_null()"
	case ast.ExternSourceNode, ast.ExternFnNode:
		// Extern declarations produce no runtime code
		return "qv_null()"
	case ast.BreakNode:
		g.emitLine("break;")
		return "qv_null()"
	case ast.ContinueNode:
		g.emitLine("continue;")
		return "qv_null()"
	default:
		panicICEf("INV-NODE-TYPE", node, "unhandled AST node type '%s'", node.NodeType.String())
		return ""
	}
}

func (g *Generator) generateLiteral(node *ast.TreeNode) string {
	if node.Token == nil {
		return "qv_null()"
	}

	switch node.Token.Type {
	case token.INT:
		return fmt.Sprintf("qv_int(%s)", node.Token.Literal)
	case token.FLOAT:
		return fmt.Sprintf("qv_float(%s)", node.Token.Literal)
	case token.STRING:
		// Escape the string properly for C++ output
		escaped := strings.ReplaceAll(node.Token.Literal, "\\", "\\\\")
		escaped = strings.ReplaceAll(escaped, "\"", "\\\"")
		escaped = strings.ReplaceAll(escaped, "\n", "\\n")
		escaped = strings.ReplaceAll(escaped, "\t", "\\t")
		escaped = strings.ReplaceAll(escaped, "\r", "\\r")
		escaped = strings.ReplaceAll(escaped, "\x00", "\\0")
		escaped = strings.ReplaceAll(escaped, "\a", "\\a")
		escaped = strings.ReplaceAll(escaped, "\b", "\\b")
		escaped = strings.ReplaceAll(escaped, "\f", "\\f")
		escaped = strings.ReplaceAll(escaped, "\v", "\\v")
		return fmt.Sprintf("qv_string(\"%s\")", escaped)
	case token.TRUE:
		return "qv_bool(true)"
	case token.FALSE:
		return "qv_bool(false)"
	case token.NULL:
		return "qv_null()"
	default:
		return "qv_null()"
	}
}

func (g *Generator) generateIdentifier(node *ast.TreeNode) string {
	name := node.TokenLiteral()
	if name == "_" {
		return "qv_null()"
	}
	if g.declaredVars[name] {
		if g.isCell(name) {
			return fmt.Sprintf("%s->value", sanitizeVarName(name))
		}
		cName := sanitizeVarName(name)
		// Box scalar variables back to QValue at read sites so the rest of
		// codegen always works with QValue expressions.
		return boxExpr(cName, g.tierOf(name))
	}
	if g.funcNames[name] {
		if _, isNative := g.nativeFns[name]; isNative {
			return fmt.Sprintf("qv_func((void*)quark_%s__thunk)", name)
		}
		return fmt.Sprintf("qv_func((void*)quark_%s)", name)
	}
	return sanitizeVarName(name)
}

// boxToQValue ensures expr (which may be a raw scalar or already a QValue) is
// returned as a QValue. tier is the C++ storage tier of expr ("long long",
// "double", "bool", or "" for QValue).
func boxToQValue(expr string, tier string) string {
	return boxExpr(expr, tier)
}

// boxResultToQValue is a no-op since generateExpr already returns QValue-compatible
// expressions (identifiers are boxed at read time via generateIdentifier).
// Kept as a named function for documentation clarity at return sites.
func (g *Generator) boxResultToQValue(expr string, _ *ast.TreeNode) string {
	return expr
}

// scalarExpr attempts to generate a raw C++ scalar expression for node without
// any QValue boxing. Returns (expr, tier) where tier is non-empty iff the
// entire expression evaluates to a raw C++ scalar.
//
// Handles recursively:
//   - Scalar literals (int, float, bool)
//   - Scalar-tiered local variables
//   - Unary minus on a scalar operand
//   - Binary arithmetic on two scalar operands (recurses into sub-expressions)
//
// Falls back to ("", "") — caller must use the boxed generateExpr path.
func (g *Generator) scalarExpr(node *ast.TreeNode) (string, string) {
	if node == nil {
		return "", ""
	}
	switch node.NodeType {
	case ast.LiteralNode:
		if node.Token == nil {
			return "", ""
		}
		switch node.Token.Type {
		case token.INT:
			return node.Token.Literal, "long long"
		case token.FLOAT:
			return node.Token.Literal, "double"
		case token.TRUE:
			return "true", "bool"
		case token.FALSE:
			return "false", "bool"
		}

	case ast.IdentifierNode:
		name := node.TokenLiteral()
		if g.declaredVars[name] && !g.isCell(name) {
			tier := g.tierOf(name)
			if tier != "" {
				return sanitizeVarName(name), tier
			}
		}

	case ast.OperatorNode:
		if node.Token == nil {
			return "", ""
		}
		op := node.Token.Type

		// Unary minus: -x where x is scalar
		if len(node.Children) == 1 && op == token.MINUS {
			operand, tier := g.scalarExpr(node.Children[0])
			if tier != "" && tier != "bool" {
				return fmt.Sprintf("(-%s)", operand), tier
			}
			return "", ""
		}

		// Binary arithmetic: both operands must be scalar
		if len(node.Children) == 2 {
			if cppOp := nativeArithOp(op); cppOp != "" && op != token.DOUBLESTAR {
				lRaw, lTier := g.scalarExpr(node.Children[0])
				rRaw, rTier := g.scalarExpr(node.Children[1])
				if lTier != "" && rTier != "" {
					resTier := promotedTier(lTier, rTier, op)
					if resTier != "" {
						expr := fmt.Sprintf("((%s)%s %s (%s)%s)", resTier, lRaw, cppOp, resTier, rRaw)
						return expr, resTier
					}
				}
			}
			// Binary comparison: both operands must be scalar (non-bool for ordered ops)
			if cppOp := nativeCompareOp(op); cppOp != "" {
				lRaw, lTier := g.scalarExpr(node.Children[0])
				rRaw, rTier := g.scalarExpr(node.Children[1])
				if lTier != "" && rTier != "" {
					// For ordered comparisons (<, <=, >, >=) skip bool operands.
					isOrdered := op == token.LT || op == token.LTE || op == token.GT || op == token.GTE
					if !isOrdered || (lTier != "bool" && rTier != "bool") {
						expr := fmt.Sprintf("(%s %s %s)", lRaw, cppOp, rRaw)
						return expr, "bool"
					}
				}
			}
		}
	}
	return "", ""
}

// nativeArithOp returns the C++ infix operator string for a token type, or "".
func nativeArithOp(op token.TokenType) string {
	switch op {
	case token.PLUS:
		return "+"
	case token.MINUS:
		return "-"
	case token.MULTIPLY:
		return "*"
	case token.DIVIDE:
		return "/"
	case token.MODULO:
		return "%"
	}
	return ""
}

// nativeCompareOp returns the C++ infix comparison operator string for a token type, or "".
func nativeCompareOp(op token.TokenType) string {
	switch op {
	case token.LT:
		return "<"
	case token.LTE:
		return "<="
	case token.GT:
		return ">"
	case token.GTE:
		return ">="
	case token.DEQ:
		return "=="
	case token.NE:
		return "!="
	}
	return ""
}

// promotedTier returns the result tier for a binary op on two scalar tiers.
// Division always promotes to double. Everything else follows C++ promotion rules.
func promotedTier(leftTier, rightTier string, op token.TokenType) string {
	if leftTier == "double" || rightTier == "double" {
		return "double"
	}
	if op == token.DIVIDE && leftTier == "long long" && rightTier == "long long" {
		return "double"
	}
	if leftTier == "long long" && rightTier == "long long" {
		return "long long"
	}
	if leftTier == "bool" && rightTier == "bool" {
		return "bool"
	}
	return ""
}

// unboxFieldAccessor returns the QValue data field name for a scalar tier.
// Used to extract raw values from QValue expressions inline.
func unboxFieldAccessor(tier string) string {
	switch tier {
	case "long long":
		return "int_val"
	case "double":
		return "float_val"
	case "bool":
		return "bool_val"
	}
	return ""
}

// loopVarTier returns the C++ scalar tier for a for-loop variable based on the
// analyzer-inferred type of the range expression. Returns "" if not scalar.
func (g *Generator) loopVarTier(rangeNode *ast.TreeNode) string {
	t := g.nodeType(rangeNode)
	if t == nil {
		return ""
	}
	switch v := t.(type) {
	case *types.ListType:
		return nativeCType(v.ElementType)
	case *types.VectorType:
		return nativeCType(v.ElementType)
	}
	return ""
}

func (g *Generator) generateOperator(node *ast.TreeNode) string {
	if node.Token == nil {
		return "qv_null()"
	}

	op := node.Token.Type

	// Unary operators
	if len(node.Children) == 1 {
		// Try scalar path for unary minus before falling back to q_neg.
		if op == token.MINUS {
			if raw, tier := g.scalarExpr(node.Children[0]); tier != "" && tier != "bool" {
				return boxExpr(fmt.Sprintf("(-%s)", raw), tier)
			}
		}
		operand := g.generateExpr(node.Children[0])
		switch op {
		case token.MINUS:
			return fmt.Sprintf("q_neg(%s)", operand)
		case token.BANG:
			return fmt.Sprintf("qv_bool(!q_truthy(%s))", operand)
		}
		return operand
	}

	// Dict member access: d.key → q_member_get(d, "key")
	if op == token.DOT && len(node.Children) >= 2 {
		obj := g.generateExpr(node.Children[0])
		memberName := node.Children[1].TokenLiteral()
		return fmt.Sprintf("q_member_get(%s, \"%s\")", obj, memberName)
	}

	// Binary operators
	if len(node.Children) < 2 {
		return "qv_null()"
	}

	// Try native scalar lowering for arithmetic and comparison ops before
	// generating boxed QValue expressions for the operands.
	if cppArith := nativeArithOp(op); cppArith != "" && op != token.DOUBLESTAR {
		lRaw, lTier := g.scalarExpr(node.Children[0])
		rRaw, rTier := g.scalarExpr(node.Children[1])
		if lTier != "" && rTier != "" {
			resTier := promotedTier(lTier, rTier, op)
			if resTier != "" {
				// Cast operands to result tier to avoid C++ integer promotion surprises.
				lCast := fmt.Sprintf("((%s)%s)", resTier, lRaw)
				rCast := fmt.Sprintf("((%s)%s)", resTier, rRaw)
				rawResult := fmt.Sprintf("(%s %s %s)", lCast, cppArith, rCast)
				return boxExpr(rawResult, resTier)
			}
		}
	}
	if cppCmp := nativeCompareOp(op); cppCmp != "" {
		lRaw, lTier := g.scalarExpr(node.Children[0])
		rRaw, rTier := g.scalarExpr(node.Children[1])
		if lTier != "" && rTier != "" {
			isOrdered := op == token.LT || op == token.LTE || op == token.GT || op == token.GTE
			if !isOrdered || (lTier != "bool" && rTier != "bool") {
				rawResult := fmt.Sprintf("(%s %s %s)", lRaw, cppCmp, rRaw)
				return boxExpr(rawResult, "bool")
			}
		}
	}

	// Short-circuit logical operators: right operand must be evaluated lazily.
	// Python semantics: `and` returns the first falsy operand or the last;
	// `or` returns the first truthy operand or the last.
	if op == token.AND {
		left := g.generateExpr(node.Children[0])
		right := g.generateExpr(node.Children[1])
		return fmt.Sprintf("([&]() -> QValue { auto _l = %s; return q_truthy(_l) ? (%s) : _l; }())", left, right)
	}
	if op == token.OR {
		left := g.generateExpr(node.Children[0])
		right := g.generateExpr(node.Children[1])
		return fmt.Sprintf("([&]() -> QValue { auto _l = %s; return q_truthy(_l) ? _l : (%s); }())", left, right)
	}

	left := g.generateExpr(node.Children[0])
	right := g.generateExpr(node.Children[1])

	switch op {
	case token.PLUS:
		return fmt.Sprintf("q_add(%s, %s)", left, right)
	case token.MINUS:
		return fmt.Sprintf("q_sub(%s, %s)", left, right)
	case token.MULTIPLY:
		return fmt.Sprintf("q_mul(%s, %s)", left, right)
	case token.DIVIDE:
		return fmt.Sprintf("q_div(%s, %s)", left, right)
	case token.MODULO:
		return fmt.Sprintf("q_mod(%s, %s)", left, right)
	case token.DOUBLESTAR:
		return fmt.Sprintf("q_pow(%s, %s)", left, right)
	case token.LT:
		return fmt.Sprintf("q_lt(%s, %s)", left, right)
	case token.LTE:
		return fmt.Sprintf("q_lte(%s, %s)", left, right)
	case token.GT:
		return fmt.Sprintf("q_gt(%s, %s)", left, right)
	case token.GTE:
		return fmt.Sprintf("q_gte(%s, %s)", left, right)
	case token.DEQ:
		return fmt.Sprintf("q_eq(%s, %s)", left, right)
	case token.NE:
		return fmt.Sprintf("q_neq(%s, %s)", left, right)
	case token.EQUALS:
		lhs := node.Children[0]
		// Member assignment: obj.member = value
		if lhs.NodeType == ast.OperatorNode && lhs.Token != nil && lhs.Token.Type == token.DOT && len(lhs.Children) >= 2 {
			obj := g.generateExpr(lhs.Children[0])
			memberName := lhs.Children[1].TokenLiteral()
			g.emitLine("q_member_set(%s, \"%s\", %s);", obj, memberName, right)
			return right
		}
		// Index assignment: obj[key] = value
		if lhs.NodeType == ast.IndexNode && len(lhs.Children) >= 2 {
			target := g.generateExpr(lhs.Children[0])
			index := g.generateExpr(lhs.Children[1])
			g.emitLine("q_set(%s, %s, %s);", target, index, right)
			return right
		}
		// Variable assignment
		varName := lhs.TokenLiteral()
		cName := sanitizeVarName(varName)
		if g.declaredVars[varName] {
			// Already declared — write using whichever storage type was used at declaration.
			if g.isCell(varName) {
				g.emitLine("%s->value = %s;", cName, right)
				return fmt.Sprintf("%s->value", cName)
			}
			tier := g.tierOf(varName)
			if tier != "" {
				// Scalar variable: try to get a raw RHS expression directly.
				// If scalarExpr succeeds the assignment is pure scalar — no box/unbox.
				rawRight, rawTier := g.scalarExpr(node.Children[1])
				if rawTier == "" {
					rawRight = unboxForTier(right, tier, nil)
				}
				g.emitLine("%s = %s;", cName, rawRight)
				return cName
			}
			g.emitLine("%s = %s;", cName, right)
			return cName
		}
		// First declaration — choose storage.
		// Captured variables must be QCell* regardless of type.
		if g.isCaptured(varName) {
			g.emitLine("QCell* %s = q_new_cell(qv_null());", cName)
			g.emitLine("%s->value = %s;", cName, right)
			g.declaredVars[varName] = true
			g.markCell(varName)
			return fmt.Sprintf("%s->value", cName)
		}
		// Use scalar storage when the RHS has a statically-known primitive type.
		rhsType := g.nodeType(node.Children[1])
		if tier := nativeCType(rhsType); tier != "" {
			rawRight, rawTier := g.scalarExpr(node.Children[1])
			if rawTier == "" {
				// RHS is a compound QValue expression — unbox it.
				rawRight = unboxForTier(right, tier, nil)
			}
			g.emitLine("%s %s = %s;", tier, cName, rawRight)
			g.declaredVars[varName] = true
			g.markTier(varName, tier)
			return cName
		}
		g.emitLine("QValue %s = %s;", cName, right)
		g.declaredVars[varName] = true
		return cName
	}

	return "qv_null()"
}

func panicMissingCallPlan(callNode *ast.TreeNode) {
	line := 0
	col := 0
	if callNode != nil && callNode.Token != nil {
		line = callNode.Token.Line
		col = callNode.Token.Column
	}
	panic(fmt.Sprintf("internal compiler error [INV-CALLPLAN-MISSING]: missing CallPlan for call at line %d, col %d", line, col))
}

func (g *Generator) getCallPlanOrPanic(callNode *ast.TreeNode) *ir.CallPlan {
	if callNode == nil {
		panicMissingCallPlan(callNode)
	}
	plan, ok := g.callPlans[callNode]
	if !ok || plan == nil {
		panicMissingCallPlan(callNode)
	}
	return plan
}

func (g *Generator) appendPlannedDefaults(args []string, plan *ir.CallPlan) []string {
	if plan == nil || len(plan.DefaultNodes) == 0 {
		return args
	}
	for _, defaultNode := range plan.DefaultNodes {
		if defaultNode == nil {
			continue
		}
		args = append(args, g.generateExpr(defaultNode))
	}
	return args
}

func generateBuiltinCall(runtimeSymbol string, args []string) string {
	return fmt.Sprintf("%s(%s)", runtimeSymbol, strings.Join(args, ", "))
}

func (g *Generator) generateFunctionCall(node *ast.TreeNode) string {
	if len(node.Children) < 2 {
		return "qv_null()"
	}

	funcNode := node.Children[0]
	argsNode := node.Children[1]
	plan := g.getCallPlanOrPanic(node)

	// Dot-call syntax on values is rejected by analyzer unless it's a known method dispatch.
	if funcNode.NodeType == ast.OperatorNode && funcNode.Token != nil && funcNode.Token.Type == token.DOT && plan.Dispatch == ir.DispatchClosure && !strings.Contains(plan.CalleeName, ".") {
		panicICEf("INV-DOT-CALL", node, "dot-call syntax is not supported")
		return "qv_null()"
	}

	funcName := funcNode.TokenLiteral()

	// Generate arguments
	args := make([]string, 0)
	for _, arg := range argsNode.Children {
		args = append(args, g.generateExpr(arg))
	}
	args = g.appendPlannedDefaults(args, plan)

	// For method calls, cache receiver in a temp to avoid re-evaluation,
	// then inject as the first argument.
	// For DispatchExtern methods the receiver is adapted to its native type.
	if plan.IsMethod && plan.ReceiverNode != nil {
		receiverTemp := g.newTemp()
		receiverExpr := g.generateExpr(plan.ReceiverNode)
		if plan.Dispatch == ir.DispatchExtern && plan.NativeReceiverType != "" {
			receiverExpr = adaptArgForExtern(receiverExpr, plan.NativeReceiverType)
			g.emitLine("%s %s = %s;", plan.NativeReceiverType, receiverTemp, receiverExpr)
		} else {
			g.emitLine("QValue %s = %s;", receiverTemp, receiverExpr)
		}
		args = append([]string{receiverTemp}, args...)
	}

	switch plan.Dispatch {
	case ir.DispatchBuiltin:
		if plan.RuntimeSymbol == "" {
			panicICEf("INV-CALLPLAN-RUNTIME", node, "builtin call '%s' missing runtime symbol", plan.CalleeName)
			return "qv_null()"
		}
		return generateBuiltinCall(plan.RuntimeSymbol, args)
	case ir.DispatchDirect:
		if plan.RuntimeSymbol == "" {
			panicICEf("INV-CALLPLAN-RUNTIME", node, "direct call '%s' missing runtime symbol", plan.CalleeName)
			return "qv_null()"
		}
		if len(args) == 0 {
			return fmt.Sprintf("%s(nullptr)", plan.RuntimeSymbol)
		}
		return fmt.Sprintf("%s(nullptr, %s)", plan.RuntimeSymbol, strings.Join(args, ", "))
	case ir.DispatchNative:
		if plan.RuntimeSymbol == "" {
			panicICEf("INV-CALLPLAN-RUNTIME", node, "native call '%s' missing runtime symbol", plan.CalleeName)
			return "qv_null()"
		}
		nativeArgs := []string{"nullptr"}
		argNodes := argsNode.Children
		for i, arg := range args {
			nativeType := ""
			if i < len(plan.NativeParamTypes) {
				nativeType = plan.NativeParamTypes[i]
			}
			// Prefer raw scalar from scalarExpr to avoid box-then-unbox.
			if i < len(argNodes) {
				if rawVal, tier := g.scalarExpr(argNodes[i]); tier != "" {
					expectedTier := nativeCppTypeToTier(nativeType)
					if expectedTier != "" && tier == expectedTier {
						nativeArgs = append(nativeArgs, rawVal)
						continue
					}
				}
			}
			nativeArgs = append(nativeArgs, adaptArgForExtern(arg, nativeType))
		}
		callExpr := fmt.Sprintf("%s(%s)", plan.RuntimeSymbol, strings.Join(nativeArgs, ", "))
		return wrapExternReturn(callExpr, plan.NativeReturnType)
	case ir.DispatchExtern:
		if plan.RuntimeSymbol == "" {
			panicICEf("INV-CALLPLAN-RUNTIME", node, "extern call '%s' missing runtime symbol", plan.CalleeName)
			return "qv_null()"
		}
		// args[0] is the receiver (already adapted above if IsMethod).
		// Remaining args need adaptation based on NativeParamTypes.
		externArgs := make([]string, len(args))
		copy(externArgs, args)
		offset := 0
		if plan.IsMethod {
			offset = 1 // args[0] is receiver, already adapted
		}
		for i := offset; i < len(externArgs); i++ {
			paramIdx := i - offset
			if paramIdx < len(plan.NativeParamTypes) {
				externArgs[i] = adaptArgForExtern(externArgs[i], plan.NativeParamTypes[paramIdx])
			}
		}
		callExpr := fmt.Sprintf("%s(%s)", plan.RuntimeSymbol, strings.Join(externArgs, ", "))
		return wrapExternReturn(callExpr, plan.NativeReturnType)
	case ir.DispatchClosure:
		// It is a closure/function value call by analyzer contract.
		funcExpr := g.generateExpr(funcNode)
		if funcNode.NodeType == ast.OperatorNode && funcNode.Token != nil && funcNode.Token.Type == token.DOT && strings.Contains(plan.CalleeName, ".") && len(funcNode.Children) >= 2 {
			member := funcNode.Children[1].TokenLiteral()
			funcExpr = fmt.Sprintf("%s->value", sanitizeVarName(member))
		}
		return EmitClosureCall(funcExpr, args)
	default:
		panicICEf("INV-CALLPLAN-DISPATCH", node, "unknown dispatch for '%s'", funcName)
		return "qv_null()"
	}
}

// EmitClosureCall emits a direct q_callN when arity is known (0..12),
// falling back to q_calln with a std::vector only for >12 args.
func EmitClosureCall(funcExpr string, args []string) string {
	n := len(args)
	if n <= 12 {
		if n == 0 {
			return fmt.Sprintf("q_call0(%s)", funcExpr)
		}
		return fmt.Sprintf("q_call%d(%s, %s)", n, funcExpr, strings.Join(args, ", "))
	}
	return fmt.Sprintf("q_calln(%s, std::vector<QValue>{%s})", funcExpr, strings.Join(args, ", "))
}

func (g *Generator) generatePipe(node *ast.TreeNode) string {
	if len(node.Children) < 2 {
		return "qv_null()"
	}

	// Left side is input value
	input := g.generateExpr(node.Children[0])

	// Right side must be a function call
	rightNode := node.Children[1]
	if rightNode.NodeType != ast.FunctionCallNode || len(rightNode.Children) < 2 {
		// Analyzer should already report an error; fall back to evaluating the expression
		return g.generateExpr(rightNode)
	}

	// Function call - prepend input to arguments
	funcNode := rightNode.Children[0]
	argsNode := rightNode.Children[1]
	plan := g.getCallPlanOrPanic(rightNode)

	funcName := funcNode.TokenLiteral()
	args := []string{input}
	for _, arg := range argsNode.Children {
		args = append(args, g.generateExpr(arg))
	}
	args = g.appendPlannedDefaults(args, plan)

	// For method calls, cache receiver in a temp to avoid re-evaluation,
	// then inject as the first argument (before piped input).
	if plan.IsMethod && plan.ReceiverNode != nil {
		receiverTemp := g.newTemp()
		receiverExpr := g.generateExpr(plan.ReceiverNode)
		if plan.Dispatch == ir.DispatchExtern && plan.NativeReceiverType != "" {
			receiverExpr = adaptArgForExtern(receiverExpr, plan.NativeReceiverType)
			g.emitLine("%s %s = %s;", plan.NativeReceiverType, receiverTemp, receiverExpr)
		} else {
			g.emitLine("QValue %s = %s;", receiverTemp, receiverExpr)
		}
		args = append([]string{receiverTemp}, args...)
	}

	switch plan.Dispatch {
	case ir.DispatchBuiltin:
		if plan.RuntimeSymbol == "" {
			panicICEf("INV-CALLPLAN-RUNTIME", rightNode, "builtin call '%s' missing runtime symbol", plan.CalleeName)
			return "qv_null()"
		}
		return generateBuiltinCall(plan.RuntimeSymbol, args)
	case ir.DispatchDirect:
		if plan.RuntimeSymbol == "" {
			panicICEf("INV-CALLPLAN-RUNTIME", rightNode, "direct call '%s' missing runtime symbol", plan.CalleeName)
			return "qv_null()"
		}
		return fmt.Sprintf("%s(nullptr, %s)", plan.RuntimeSymbol, strings.Join(args, ", "))
	case ir.DispatchNative:
		if plan.RuntimeSymbol == "" {
			panicICEf("INV-CALLPLAN-RUNTIME", rightNode, "native call '%s' missing runtime symbol", plan.CalleeName)
			return "qv_null()"
		}
		nativeArgs := []string{"nullptr"}
		// args[0] is the piped input; args[1..] correspond to argsNode.Children.
		pipeExplicitNodes := argsNode.Children
		for i, arg := range args {
			nativeType := ""
			if i < len(plan.NativeParamTypes) {
				nativeType = plan.NativeParamTypes[i]
			}
			// For explicit args (index > 0), try scalarExpr to avoid box-unbox.
			explicitIdx := i - 1
			if i > 0 && explicitIdx < len(pipeExplicitNodes) {
				if rawVal, tier := g.scalarExpr(pipeExplicitNodes[explicitIdx]); tier != "" {
					if expectedTier := nativeCppTypeToTier(nativeType); expectedTier != "" && tier == expectedTier {
						nativeArgs = append(nativeArgs, rawVal)
						continue
					}
				}
			}
			nativeArgs = append(nativeArgs, adaptArgForExtern(arg, nativeType))
		}
		callExpr := fmt.Sprintf("%s(%s)", plan.RuntimeSymbol, strings.Join(nativeArgs, ", "))
		return wrapExternReturn(callExpr, plan.NativeReturnType)
	case ir.DispatchExtern:
		if plan.RuntimeSymbol == "" {
			panicICEf("INV-CALLPLAN-RUNTIME", rightNode, "extern call '%s' missing runtime symbol", plan.CalleeName)
			return "qv_null()"
		}
		externArgs := make([]string, len(args))
		copy(externArgs, args)
		offset := 0
		if plan.IsMethod {
			offset = 1
		}
		for i := offset; i < len(externArgs); i++ {
			paramIdx := i - offset
			if paramIdx < len(plan.NativeParamTypes) {
				externArgs[i] = adaptArgForExtern(externArgs[i], plan.NativeParamTypes[paramIdx])
			}
		}
		callExpr := fmt.Sprintf("%s(%s)", plan.RuntimeSymbol, strings.Join(externArgs, ", "))
		return wrapExternReturn(callExpr, plan.NativeReturnType)
	case ir.DispatchClosure:
		funcExpr := g.generateExpr(funcNode)
		if funcNode.NodeType == ast.OperatorNode && funcNode.Token != nil && funcNode.Token.Type == token.DOT && strings.Contains(plan.CalleeName, ".") && len(funcNode.Children) >= 2 {
			member := funcNode.Children[1].TokenLiteral()
			funcExpr = fmt.Sprintf("%s->value", sanitizeVarName(member))
		}
		return EmitClosureCall(funcExpr, args)
	default:
		panicICEf("INV-CALLPLAN-DISPATCH", rightNode, "unknown dispatch for '%s'", funcName)
		return "qv_null()"
	}
}

func (g *Generator) generateTernary(node *ast.TreeNode) string {
	if len(node.Children) < 3 {
		return "qv_null()"
	}

	cond := g.generateExpr(node.Children[0])
	trueVal := g.generateExpr(node.Children[1])
	falseVal := g.generateExpr(node.Children[2])

	return fmt.Sprintf("(q_truthy(%s) ? %s : %s)", cond, trueVal, falseVal)
}

func (g *Generator) generateIf(node *ast.TreeNode) string {
	if len(node.Children) < 2 {
		return "qv_null()"
	}

	temp := g.newTemp()
	g.emitLine("QValue %s = qv_null();", temp)

	var cond string
	if rawCond, tier := g.scalarExpr(node.Children[0]); tier == "bool" {
		cond = rawCond
	} else {
		cond = fmt.Sprintf("q_truthy(%s)", g.generateExpr(node.Children[0]))
	}
	g.emitLine("if (%s) {", cond)
	g.indentLevel++

	ifResult := g.generateExpr(node.Children[1])
	g.emitLine("%s = %s;", temp, ifResult)

	g.indentLevel--
	g.emit(g.indent() + "}")

	// Handle elseif/else
	for i := 2; i < len(node.Children); i++ {
		child := node.Children[i]
		if child.NodeType == ast.IfStatementNode && len(child.Children) >= 2 {
			// elseif
			var elseifCond string
			if rawCond, tier := g.scalarExpr(child.Children[0]); tier == "bool" {
				elseifCond = rawCond
			} else {
				elseifCond = fmt.Sprintf("q_truthy(%s)", g.generateExpr(child.Children[0]))
			}
			g.emit(" else if (%s) {\n", elseifCond)
			g.indentLevel++
			elseifResult := g.generateExpr(child.Children[1])
			g.emitLine("%s = %s;", temp, elseifResult)
			g.indentLevel--
			g.emit(g.indent() + "}")
		} else {
			// else
			g.emit(" else {\n")
			g.indentLevel++
			elseResult := g.generateExpr(child)
			g.emitLine("%s = %s;", temp, elseResult)
			g.indentLevel--
			g.emit(g.indent() + "}")
		}
	}
	g.emit("\n")

	return temp
}

func (g *Generator) generateWhen(node *ast.TreeNode) string {
	if len(node.Children) < 2 {
		return "qv_null()"
	}

	temp := g.newTemp()
	matchExpr := g.generateExpr(node.Children[0])
	matchTemp := g.newTemp()

	g.emitLine("QValue %s = qv_null();", temp)
	g.emitLine("QValue %s = %s;", matchTemp, matchExpr)

	first := true
	for i := 1; i < len(node.Children); i++ {
		pattern := node.Children[i]
		if pattern.NodeType != ast.PatternNode || len(pattern.Children) < 2 {
			continue
		}

		// Last child is the result, others are patterns
		resultIdx := len(pattern.Children) - 1
		resultExprNode := pattern.Children[resultIdx]

		// Build condition from patterns
		conditions := make([]string, 0)
		bindings := make([]struct {
			name string
			isOk bool
		}, 0)
		for j := 0; j < resultIdx; j++ {
			patternExpr := pattern.Children[j]
			switch patternExpr.NodeType {
			case ast.IdentifierNode:
				if patternExpr.TokenLiteral() == "_" {
					conditions = append(conditions, "true")
					continue
				}
				patternVal := g.generateExpr(patternExpr)
				conditions = append(conditions, fmt.Sprintf("q_eq(%s, %s).data.bool_val", matchTemp, patternVal))
			case ast.ResultPatternNode:
				if patternExpr.Token != nil && patternExpr.Token.Type == token.ERR {
					conditions = append(conditions, fmt.Sprintf("!q_is_ok(%s)", matchTemp))
				} else {
					conditions = append(conditions, fmt.Sprintf("q_is_ok(%s)", matchTemp))
				}
				if len(patternExpr.Children) > 0 {
					bindNode := patternExpr.Children[0]
					if bindNode != nil {
						name := bindNode.TokenLiteral()
						if name != "" && name != "_" {
							bindings = append(bindings, struct {
								name string
								isOk bool
							}{name: name, isOk: patternExpr.Token == nil || patternExpr.Token.Type != token.ERR})
						}
					}
				}
			default:
				patternVal := g.generateExpr(patternExpr)
				conditions = append(conditions, fmt.Sprintf("q_eq(%s, %s).data.bool_val", matchTemp, patternVal))
			}
		}

		condStr := strings.Join(conditions, " || ")
		if condStr == "" {
			condStr = "false"
		}
		if first {
			g.emitLine("if (%s) {", condStr)
			first = false
		} else {
			g.emit(g.indent()+"} else if (%s) {\n", condStr)
		}
		g.indentLevel++
		g.pushBlockScope()
		for _, bind := range bindings {
			accessor := "q_result_value"
			if !bind.isOk {
				accessor = "q_result_error"
			}
			cName := sanitizeVarName(bind.name)
			if g.isCaptured(bind.name) {
				g.emitLine("QCell* %s = q_new_cell(%s(%s));", cName, accessor, matchTemp)
				g.markCell(bind.name)
			} else {
				g.emitLine("QValue %s = %s(%s);", cName, accessor, matchTemp)
			}
			g.declaredVars[bind.name] = true
		}
		result := g.generateExpr(resultExprNode)
		g.emitLine("%s = %s;", temp, result)
		g.popScope()
		g.indentLevel--
	}

	if !first {
		g.emitLine("}")
	}

	return temp
}

func (g *Generator) generateFor(node *ast.TreeNode) string {
	if len(node.Children) < 3 {
		return "qv_null()"
	}

	varNode := node.Children[0]
	rangeNode := node.Children[1]
	bodyNode := node.Children[2]

	varName := varNode.TokenLiteral()
	cVarName := sanitizeVarName(varName)

	// Handle list iteration (for item in mylist or for i in range(10))
	listExpr := g.generateExpr(rangeNode)
	listTemp := g.newTemp()
	lenTemp := g.newTemp()
	idxTemp := g.newTemp()

	g.emitLine("QValue %s = %s;", listTemp, listExpr)
	g.emitLine("long long %s = q_len(%s).data.int_val;", lenTemp, listTemp)
	g.emitLine("for (long long %s = 0; %s < %s; %s++) {", idxTemp, idxTemp, lenTemp, idxTemp)
	g.indentLevel++

	g.pushBlockScope()
	g.declaredVars[varName] = true // Loop variable is declared
	if g.isCaptured(varName) {
		g.emitLine("QCell* %s = q_new_cell(q_iter_get(%s, qv_int(%s)));", cVarName, listTemp, idxTemp)
		g.markCell(varName)
	} else {
		// Use scalar storage for loop variable if the iterable has a known element type.
		loopVarTier := g.loopVarTier(rangeNode)
		if loopVarTier != "" {
			field := unboxFieldAccessor(loopVarTier)
			g.emitLine("%s %s = q_iter_get(%s, qv_int(%s)).data.%s;",
				loopVarTier, cVarName, listTemp, idxTemp, field)
			g.markTier(varName, loopVarTier)
		} else {
			g.emitLine("QValue %s = q_iter_get(%s, qv_int(%s));", cVarName, listTemp, idxTemp)
		}
	}

	if bodyNode.NodeType == ast.BlockNode {
		for _, stmt := range bodyNode.Children {
			expr := g.generateExpr(stmt)
			g.emitLine("%s;", expr)
		}
	} else {
		expr := g.generateExpr(bodyNode)
		g.emitLine("%s;", expr)
	}

	g.popScope()

	g.indentLevel--
	g.emitLine("}")

	return "qv_null()"
}

func (g *Generator) generateWhile(node *ast.TreeNode) string {
	if len(node.Children) < 2 {
		return "qv_null()"
	}

	condNode := node.Children[0]
	bodyNode := node.Children[1]

	var whileCond string
	if rawCond, tier := g.scalarExpr(condNode); tier == "bool" {
		whileCond = rawCond
	} else {
		whileCond = fmt.Sprintf("q_truthy(%s)", g.generateExpr(condNode))
	}
	g.emitLine("while (%s) {", whileCond)
	g.indentLevel++

	// Generate body
	if bodyNode.NodeType == ast.BlockNode {
		for _, stmt := range bodyNode.Children {
			expr := g.generateExpr(stmt)
			g.emitLine("%s;", expr)
		}
	} else {
		expr := g.generateExpr(bodyNode)
		g.emitLine("%s;", expr)
	}

	g.indentLevel--
	g.emitLine("}")

	return "qv_null()"
}

func (g *Generator) generateList(node *ast.TreeNode) string {
	if len(node.Children) == 0 {
		return "qv_list(8)"
	}

	// Generate list with initial elements
	temp := g.newTemp()
	g.emitLine("QValue %s = qv_list(%d);", temp, len(node.Children))

	for _, child := range node.Children {
		elem := g.generateExpr(child)
		g.emitLine("%s = q_push(%s, %s);", temp, temp, elem)
	}

	return temp
}

func (g *Generator) generateVector(node *ast.TreeNode) string {
	tempList := g.newTemp()
	g.emitLine("QValue %s = qv_list(%d);", tempList, len(node.Children))

	for _, child := range node.Children {
		elem := g.generateExpr(child)
		g.emitLine("%s = q_push(%s, %s);", tempList, tempList, elem)
	}

	return fmt.Sprintf("q_to_vector(%s)", tempList)
}

func (g *Generator) generateDict(node *ast.TreeNode) string {
	temp := g.newTemp()
	g.emitLine("QValue %s = qv_dict();", temp)

	for _, pair := range node.Children {
		if pair == nil || len(pair.Children) < 2 {
			continue
		}
		key := g.generateExpr(pair.Children[0])
		value := g.generateExpr(pair.Children[1])
		g.emitLine("%s = q_dict_set(%s, %s, %s);", temp, temp, key, value)
	}

	return temp
}

func (g *Generator) generateIndex(node *ast.TreeNode) string {
	if len(node.Children) < 2 {
		return "qv_null()"
	}

	target := g.generateExpr(node.Children[0])
	index := g.generateExpr(node.Children[1])

	return fmt.Sprintf("q_get(%s, %s)", target, index)
}

func (g *Generator) generateResult(node *ast.TreeNode) string {
	if len(node.Children) == 0 {
		return "qv_null()"
	}
	value := g.generateExpr(node.Children[0])
	if node.Token != nil && node.Token.Type == token.ERR {
		return fmt.Sprintf("qv_err(%s)", value)
	}
	return fmt.Sprintf("qv_ok(%s)", value)
}

func (g *Generator) generateVarDecl(node *ast.TreeNode) string {
	if len(node.Children) < 3 {
		return "qv_null()"
	}
	nameNode := node.Children[0]
	valueNode := node.Children[2]
	name := nameNode.TokenLiteral()
	cName := sanitizeVarName(name)
	value := g.generateExpr(valueNode)
	if g.declaredVars[name] {
		if g.isCell(name) {
			g.emitLine("%s->value = %s;", cName, value)
			return fmt.Sprintf("%s->value", cName)
		}
		tier := g.tierOf(name)
		if tier != "" {
			g.emitLine("%s = %s;", cName, unboxForTier(value, tier, g.nodeType(valueNode)))
			return cName
		}
		g.emitLine("%s = %s;", cName, value)
		return cName
	}
	if g.isCaptured(name) {
		g.emitLine("QCell* %s = q_new_cell(qv_null());", cName)
		g.emitLine("%s->value = %s;", cName, value)
		g.declaredVars[name] = true
		g.markCell(name)
		return fmt.Sprintf("%s->value", cName)
	}
	rhsType := g.nodeType(valueNode)
	if tier := nativeCType(rhsType); tier != "" {
		rawValue, rawTier := g.scalarExpr(valueNode)
		if rawTier == "" {
			rawValue = unboxForTier(value, tier, nil)
		}
		g.emitLine("%s %s = %s;", tier, cName, rawValue)
		g.declaredVars[name] = true
		g.markTier(name, tier)
		return cName
	}
	g.emitLine("QValue %s = %s;", cName, value)
	g.declaredVars[name] = true
	return cName
}

func (g *Generator) generateLambdaExpr(node *ast.TreeNode) string {
	// Look up the lambda name we assigned during collection
	lambdaName, ok := g.lambdaNames[node]
	if !ok {
		// Lambda wasn't collected - this shouldn't happen but handle it
		lambdaName = g.newLambda()
		g.lambdaNames[node] = lambdaName
		g.lambdas = append(g.lambdas, node)
		paramCount := 0
		if len(node.Children) >= 1 {
			paramCount = len(node.Children[0].Children)
		}
		g.funcDecls = append(g.funcDecls, funcDecl{name: lambdaName, paramCount: paramCount})
	}

	// Check if this lambda captures any variables
	caps, hasCaps := g.captures[node]
	if hasCaps && len(caps) > 0 {
		// Emit closure allocation with captured values
		clTemp := g.newTemp()
		g.emitLine("QClosure* %s = q_alloc_closure((void*)quark_%s, %d);", clTemp, lambdaName, len(caps))
		for i, capName := range caps {
			g.emitLine("%s->captures[%d] = %s;", clTemp, i, sanitizeVarName(capName))
		}
		valTemp := g.newTemp()
		g.emitLine("QValue %s; %s.type = QValue::VAL_FUNC; %s.data.func_val = %s;", valTemp, valTemp, valTemp, clTemp)
		return valTemp
	}

	// No captures — use qv_func (allocates QClosure with 0 captures).
	// For native functions, point to the thunk so the QValue call convention works.
	if _, isNative := g.nativeFns[lambdaName]; isNative {
		return fmt.Sprintf("qv_func((void*)quark_%s__thunk)", lambdaName)
	}
	return fmt.Sprintf("qv_func((void*)quark_%s)", lambdaName)
}

func (g *Generator) generateLambdaFunc(node *ast.TreeNode) {
	if len(node.Children) < 2 {
		return
	}

	lambdaName := g.lambdaNames[node]
	argsNode := node.Children[0]
	bodyNode := node.Children[1]

	// If this lambda is a named native function, delegate to the native path.
	// generateNativeFunction already emits the thunk at the end.
	if proto, isNative := g.nativeFns[lambdaName]; isNative {
		g.generateNativeFunction(node, lambdaName, argsNode, bodyNode, proto)
		return
	}

	g.inFunction = true
	g.currentFunc = lambdaName
	g.currentFuncNode = node
	g.pushScope() // Create new scope for lambda

	// Build parameter list: QClosure* _cl as hidden first param, then user params
	params := []string{"QClosure* _cl"}
	for _, param := range argsNode.Children {
		paramName := g.paramName(param)
		if paramName == "" {
			continue
		}
		cParamName := sanitizeArgName(paramName)
		params = append(params, fmt.Sprintf("QValue %s", cParamName))
	}

	g.emit("QValue quark_%s(%s) {\n", lambdaName, strings.Join(params, ", "))
	g.indentLevel++

	for _, param := range argsNode.Children {
		paramName := g.paramName(param)
		if paramName == "" {
			continue
		}
		cName := sanitizeVarName(paramName)
		argName := sanitizeArgName(paramName)
		if g.isCaptured(paramName) {
			g.emitLine("QCell* %s = q_new_cell(%s);", cName, argName)
			g.markCell(paramName)
		} else {
			g.emitLine("QValue %s = %s;", cName, argName)
		}
		g.declaredVars[paramName] = true
	}

	// Extract captured variables from closure — always QCell* (shared with outer scope)
	if caps, ok := g.captures[node]; ok {
		for i, capName := range caps {
			cName := sanitizeVarName(capName)
			g.emitLine("QCell* %s = _cl->captures[%d];", cName, i)
			g.declaredVars[capName] = true
			g.markCell(capName)
		}
	}

	// Generate body - for lambdas, the body is a single expression
	result := g.generateExpr(bodyNode)
	// Lambdas return QValue — box any scalar result.
	g.emitLine("return %s;", g.boxResultToQValue(result, bodyNode))

	g.indentLevel--
	g.emit("}\n\n")

	g.popScope() // Restore previous scope
	g.inFunction = false
	g.currentFuncNode = nil
}
