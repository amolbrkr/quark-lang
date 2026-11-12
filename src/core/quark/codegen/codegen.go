package codegen

import (
	"fmt"
	"os"
	"quark/ast"
	"quark/ir"
	"quark/names"
	"quark/token"
	"quark/types"
	"sort"
	"strings"
)

// cppStringEscaper escapes a Go string for a C++ string literal. It is the
// only escaper: string literals, file paths and names all go through it.
var cppStringEscaper = strings.NewReplacer(
	"\\", "\\\\",
	"\"", "\\\"",
	"\n", "\\n",
	"\t", "\\t",
	"\r", "\\r",
	"\x00", "\\000", // three octal digits, so a following digit isn't read as part of it
	"\a", "\\a",
	"\b", "\\b",
	"\f", "\\f",
	"\v", "\\v",
)

// escapeCppString escapes a Go string for safe embedding in a C++ string literal.
func escapeCppString(s string) string {
	return cppStringEscaper.Replace(s)
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
	params           *ast.TreeNode // parameter list (for arity and defaults in its QFnInfo)
	display          string        // name in runtime messages
	nativeParamTypes []string      // nil = QValue params; non-nil = native-typed
	nativeReturnType string        // "" = QValue return
}

// Generator generates C code from an AST
type Generator struct {
	output      strings.Builder
	indentLevel int
	sourceName  string
	funcDecls   []funcDecl               // Function declarations with param counts
	lambdas     []*ast.TreeNode          // Lambda expressions to generate
	lambdaNames map[*ast.TreeNode]string // Maps lambda nodes to their generated names
	// lambdaUserNames maps lambdas bound by `name = fn(...)` (how named functions
	// are written) to that name, for runtime messages.
	lambdaUserNames map[*ast.TreeNode]string
	tempCounter     int
	lambdaCounter   int
	declaredVars    map[string]bool            // Tracks declared variables to avoid redeclaration
	cellVars        map[string]bool            // Subset of declaredVars that are QCell* (vs QValue)
	scopeStack      []map[string]bool          // Stack of variable scopes for nested blocks
	cellStack       []map[string]bool          // Mirrors scopeStack for cellVars
	captures        map[*ast.TreeNode][]string // Lambda node → captured variable names (from analyzer)
	callPlans       map[*ast.TreeNode]*ir.CallPlan
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
	// Used by collectFunctions and generateLambdaFunc to emit native signatures + thunks.
	nativeFns map[string]*ir.CallPlan
	// externFns maps extern fn registration key → CallPlan (free fn name, or "type.method").
	// Used to detect when an extern fn identifier is used as a first-class value
	// and emit a thunk wrapping the native symbol.
	externFns map[string]*ir.CallPlan
	// structTypes maps struct name → StructType from the analyzer.
	structTypes map[string]*types.StructType
	// names maps internal names to the names users write (runtime messages).
	names names.Table
	// Fixed `fn` declarations, from the analyzer (types/analyzer_fixed.go).
	fixedFns        map[*ast.TreeNode]*types.FixedFn
	fnRefs          map[*ast.TreeNode]*types.FixedFn
	hoists          map[*ast.TreeNode]*types.HoistScope
	checkedCaptures map[*ast.TreeNode]map[string]bool
	// checkedCells holds the captured variables of the lambda being generated
	// that may still be unassigned; they are read with q_cell_get.
	checkedCells map[string]bool
}

func New() *Generator {
	return &Generator{
		funcDecls:          make([]funcDecl, 0),
		lambdas:            make([]*ast.TreeNode, 0),
		lambdaNames:        make(map[*ast.TreeNode]string),
		lambdaUserNames:    make(map[*ast.TreeNode]string),
		tempCounter:        0,
		sourceName:         "<unknown>",
		declaredVars:       make(map[string]bool),
		cellVars:           make(map[string]bool),
		scopeStack:         make([]map[string]bool, 0),
		cellStack:          make([]map[string]bool, 0),
		captures:           make(map[*ast.TreeNode][]string),
		callPlans:          make(map[*ast.TreeNode]*ir.CallPlan),
		capturedByFunction: make(map[*ast.TreeNode]map[string]bool),
		nodeTypes:          make(map[*ast.TreeNode]types.Type),
		varTiers:           make(map[string]string),
		tierStack:          make([]map[string]string, 0),
		nativeFns:          make(map[string]*ir.CallPlan),
		externFns:          make(map[string]*ir.CallPlan),
		structTypes:        make(map[string]*types.StructType),
	}
}

// SetAnalysis passes the analyzer outputs to the generator. Nil maps keep the
// generator's empty defaults.
func (g *Generator) SetAnalysis(an *types.Analysis) {
	if an == nil {
		return
	}
	if an.Captures != nil {
		g.captures = an.Captures
	}
	if an.CallPlans != nil {
		g.callPlans = an.CallPlans
	}
	if an.CapturedByFunction != nil {
		g.capturedByFunction = an.CapturedByFunction
	}
	if an.NodeTypes != nil {
		g.nodeTypes = an.NodeTypes
	}
	if an.NativeFns != nil {
		g.nativeFns = an.NativeFns
	}
	if an.ExternFns != nil {
		g.externFns = an.ExternFns
	}
	if an.StructTypes != nil {
		g.structTypes = an.StructTypes
	}
	g.names = an.Names
	g.fixedFns = an.FixedFns
	g.fnRefs = an.FnRefs
	g.hoists = an.HoistScopes
	g.checkedCaptures = an.CheckedCaptures
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
	// Stdlib code keeps the caller's location, so an error raised inside
	// std/... (bad arguments, mostly) points at the user's call.
	if isStdlibSource(node) {
		return
	}
	g.emitLine("q_set_source_loc(\"%s\", %d, %d);", escapeCppString(g.sourceFileOf(node)), node.Token.Line, node.Token.Column)
}

// isStdlibSource reports whether a node was spliced in from a std/ module.
func isStdlibSource(node *ast.TreeNode) bool {
	return node != nil && strings.HasPrefix(node.SourceFile, "std/")
}

// sourceFileOf is the file a node came from: its SourceFile when it was spliced
// in from an imported module, otherwise the root file being compiled.
func (g *Generator) sourceFileOf(node *ast.TreeNode) string {
	if node != nil && node.SourceFile != "" {
		return node.SourceFile
	}
	return g.sourceName
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
	if node == nil || node.NodeType != ast.AssignNode {
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

// emitStmtExpr emits a C++ expression as a statement, but only if the
// originating AST node does not already emit its own side effects. Nodes
// like VarDecl, assignment, if/when, for/while, and blocks emit their
// own C++ statements internally — their returned expression is only a
// residual value reference. Emitting it as a bare statement produces
// clang's "expression result unused" warning (-Wunused-value).
func (g *Generator) emitStmtExpr(node *ast.TreeNode, expr string) {
	switch node.NodeType {
	case ast.VarDeclNode, ast.IfStatementNode, ast.WhenStatementNode,
		ast.ForLoopNode, ast.WhileLoopNode, ast.BlockNode, ast.AssignNode, ast.FnDeclNode:
		return
	}
	g.emitLine("%s;", expr)
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
// (ExternHeaderNode children with their absolute path in the token literal).
func collectExternSources(node *ast.TreeNode) []string {
	var paths []string
	seen := make(map[string]bool)
	var walk func(*ast.TreeNode)
	walk = func(n *ast.TreeNode) {
		if n == nil {
			return
		}
		if n.NodeType == ast.ExternHeaderNode && len(n.Children) > 0 {
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

// boxNative wraps an expression of a native C++ type (an extern or native
// function's parameter or result) into a QValue; adaptArgForExtern is the
// other direction.
func boxNative(callExpr string, nativeReturn string) string {
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
	case "QClosure*":
		return fmt.Sprintf("qv_closure_ptr(%s)", callExpr)
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
	// Emit forward declarations for extern fn thunks (free functions only —
	// methods are not callable as first-class values in v0.1).
	for _, proto := range g.externFns {
		if proto.IsMethod {
			continue
		}
		thunkParams := []string{"QClosure*"}
		for range proto.NativeParamTypes {
			thunkParams = append(thunkParams, "QValue")
		}
		g.emitLine("QValue _thunk_%s(%s);", proto.RuntimeSymbol, strings.Join(thunkParams, ", "))
	}
	g.emit("\n")
	g.emitFnInfos()

	// Emit struct definitions (QStructDef statics)
	g.emitStructDefs()
	g.emitDefaultArgFns()

	// Generate function definitions
	g.generateNode(node)

	// Emit extern fn thunk bodies (free functions only).
	for _, proto := range g.externFns {
		if proto.IsMethod {
			continue
		}
		g.generateExternThunk(proto)
	}

	// Generate main function
	g.emit("\nint main(int argc, char** argv) {\n")
	g.indentLevel++
	// Set root as currentFuncNode so isCaptured() works for module-scope variables.
	g.currentFuncNode = node
	g.emitLine("q_gc_init();")
	g.emitLine("q_set_args(argc, argv);")
	// Hoist the declarations of the top level and of every module body: all
	// of main's functions exist before its first statement runs.
	hoisted := []*ast.TreeNode{node}
	for _, child := range node.Children {
		if child.NodeType == ast.ModuleNode && len(child.Children) >= 2 {
			hoisted = append(hoisted, child.Children[1])
		}
	}
	g.hoistScopes(hoisted...)

	// Initialize module body bindings so imported symbols are available at runtime.
	for _, child := range node.Children {
		if child.NodeType == ast.ModuleNode {
			g.generateModuleBindings(child)
		}
	}

	// Generate top-level statements that aren't function/module/extern definitions
	for _, child := range node.Children {
		if child.NodeType == ast.ModuleNode ||
			child.NodeType == ast.ExternHeaderNode ||
			child.NodeType == ast.ExternLinkNode || child.NodeType == ast.ExternIncludePathNode ||
			child.NodeType == ast.ExternFnNode || child.NodeType == ast.StructDefNode {
			continue
		}
		if child.NodeType == ast.FnDeclNode {
			continue // hoisted
		}
		g.emitSourceLoc(child)
		expr := g.generateExpr(child)
		g.emitStmtExpr(child, expr)
	}

	g.emitLine("return 0;")
	g.indentLevel--
	g.emit("}\n")

	return g.output.String()
}

// hoistScopes emits, at the start of one or more scopes, everything their
// `fn` declarations need before the first statement runs: the cells of the
// variables their closures capture (unassigned until the variable's first
// assignment), a binding for each declaration that needs a closure, and then
// the closures. Declarations that need no closure are C++ functions and get
// nothing here.
func (g *Generator) hoistScopes(nodes ...*ast.TreeNode) {
	var scopes []*types.HoistScope
	for _, n := range nodes {
		if hs := g.hoists[n]; hs != nil {
			scopes = append(scopes, hs)
		}
	}
	for _, hs := range scopes {
		for _, name := range hs.Cells {
			if g.declaredVars[name] {
				continue
			}
			g.emitLine("QCell* %s = q_new_unassigned_cell();", sanitizeVarName(name))
			g.declaredVars[name] = true
			g.markCell(name)
		}
		for _, f := range hs.Fns {
			if !f.Captures || g.declaredVars[f.Name] {
				continue
			}
			cName := sanitizeVarName(f.Name)
			if g.isCaptured(f.Name) {
				g.emitLine("QCell* %s = q_new_cell(qv_null());", cName)
				g.markCell(f.Name)
			} else {
				g.emitLine("QValue %s = qv_null();", cName)
			}
			g.declaredVars[f.Name] = true
		}
	}
	for _, hs := range scopes {
		for _, f := range hs.Fns {
			if !f.Captures {
				continue
			}
			value := g.generateLambdaExpr(f.Lambda)
			if g.isCell(f.Name) {
				g.emitLine("%s->value = %s;", sanitizeVarName(f.Name), value)
			} else {
				g.emitLine("%s = %s;", sanitizeVarName(f.Name), value)
			}
		}
	}
}

// fixedFnValue is the value of a `fn` declaration used as an expression (the
// declaration statement itself, e.g. as a block's last statement).
func (g *Generator) fixedFnValue(node *ast.TreeNode) string {
	f := g.fixedFns[node.Children[1]]
	if f == nil {
		panicICEf("INV-FIXED-FN", node, "declaration '%s' was not analyzed", node.Children[0].TokenLiteral())
		return "qv_null()"
	}
	if f.Captures {
		return g.generateIdentifier(node.Children[0])
	}
	return directFnValue(f)
}

// directFnValue is a function value for a declaration that needs no closure.
func directFnValue(f *types.FixedFn) string {
	if f.Native != nil {
		return fmt.Sprintf("qv_func((void*)quark_%s__thunk, &%s)", f.CName, fnInfoSymbol(f.CName))
	}
	return fmt.Sprintf("qv_func((void*)quark_%s, &%s)", f.CName, fnInfoSymbol(f.CName))
}

func (g *Generator) collectFunctions(node *ast.TreeNode) {
	switch node.NodeType {
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
			params:     lambdaParams(node),
			display:    g.lambdaDisplayName(node),
		}
		g.funcDecls = append(g.funcDecls, fd)
	case ast.AssignNode:
		if isFunctionBindingAssignmentNode(node) {
			g.lambdaUserNames[node.Children[1]] = node.Children[0].TokenLiteral()
		}
	case ast.FnDeclNode:
		if !ast.IsFnDecl(node) {
			break
		}
		lambdaNode := node.Children[1]
		g.lambdaUserNames[lambdaNode] = node.Children[0].TokenLiteral()
		// A `fn` declaration's function is quark_<CName>: the name calls use
		// directly when it needs no closure.
		if f := g.fixedFns[lambdaNode]; f != nil {
			g.lambdaNames[lambdaNode] = f.CName
			g.lambdas = append(g.lambdas, lambdaNode)
			paramCount := 0
			if len(lambdaNode.Children) >= 1 {
				paramCount = len(lambdaNode.Children[0].Children)
			}
			fd := funcDecl{
				name:       f.CName,
				paramCount: paramCount,
				params:     lambdaParams(lambdaNode),
				display:    g.displayFuncName(f.Name),
			}
			if f.Native != nil {
				fd.nativeParamTypes = f.Native.NativeParamTypes
				fd.nativeReturnType = f.Native.NativeReturnType
			}
			g.funcDecls = append(g.funcDecls, fd)
			// The lambda itself is registered; collect what is inside it.
			for _, child := range lambdaNode.Children {
				g.collectFunctions(child)
			}
			return
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
	case ast.ExternHeaderNode, ast.ExternFnNode, ast.ExternLinkNode, ast.ExternIncludePathNode:
		// No code emitted — extern headers are #included in the preamble,
		// extern fn declarations are registered in the analyzer, and link /
		// include_path directives are handled by the toolchain.
	case ast.StructDefNode:
		// No code emitted — struct defs are emitted as static data by emitStructDefs().
	}
}

// displayFuncName is how runtime messages name a function: as written
// (fmt::show_vec, mathx::square), not by its internal name.
func (g *Generator) displayFuncName(name string) string {
	return g.names.Display(name)
}

// lambdaParams is a lambda node's parameter list.
func lambdaParams(node *ast.TreeNode) *ast.TreeNode {
	if node == nil || len(node.Children) == 0 {
		return nil
	}
	return node.Children[0]
}

// fnInfoSymbol is the QFnInfo describing generated function quark_<name>
// (and its native thunk); externFnInfoSymbol describes extern thunk
// _thunk_<symbol>.
func fnInfoSymbol(name string) string         { return "_qfi_" + name }
func externFnInfoSymbol(symbol string) string { return "_qfi__thunk_" + symbol }

// defaultArgFnName is the function that builds quark_<name>'s default
// arguments for calls through a function value.
func defaultArgFnName(name string) string { return "quark_" + name + "__default_arg" }

// paramArity is the number of parameters without a default and the total.
func paramArity(params *ast.TreeNode, paramCount int) (int, int) {
	if params == nil {
		return paramCount, paramCount
	}
	required := 0
	for _, p := range params.Children {
		if p != nil && p.DefaultValue == nil {
			required++
		}
	}
	return required, len(params.Children)
}

// emitFnInfos emits the QFnInfo of every function that can become a value, so
// calls through a function value can check their argument count and fill in
// defaults (see types/closure.hpp).
func (g *Generator) emitFnInfos() {
	seen := map[string]bool{}
	for _, fd := range g.funcDecls {
		if seen[fd.name] {
			continue
		}
		seen[fd.name] = true
		minArity, maxArity := paramArity(fd.params, fd.paramCount)
		defaults := "nullptr"
		if minArity < maxArity {
			g.emitLine("static QValue %s(int index);", defaultArgFnName(fd.name))
			defaults = defaultArgFnName(fd.name)
		}
		display := fd.display
		if display == "" {
			display = g.displayFuncName(fd.name)
		}
		g.emitLine("static const QFnInfo %s = {\"%s\", %d, %d, %s};", fnInfoSymbol(fd.name), escapeCppString(display), minArity, maxArity, defaults)
	}
	names := make([]string, 0, len(g.externFns))
	for name := range g.externFns {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		proto := g.externFns[name]
		if proto.IsMethod || seen["extern:"+proto.RuntimeSymbol] {
			continue
		}
		seen["extern:"+proto.RuntimeSymbol] = true
		n := len(proto.NativeParamTypes)
		g.emitLine("static const QFnInfo %s = {\"%s\", %d, %d, nullptr};", externFnInfoSymbol(proto.RuntimeSymbol), escapeCppString(g.displayFuncName(name)), n, n)
	}
	g.emit("\n")
}

// emitDefaultArgFns emits, for each function with default parameters, the
// function its QFnInfo uses to build an omitted default (a fresh value per
// call, so an empty-list default isn't shared between calls).
func (g *Generator) emitDefaultArgFns() {
	seen := map[string]bool{}
	for _, fd := range g.funcDecls {
		if seen[fd.name] || fd.params == nil {
			continue
		}
		seen[fd.name] = true
		if minArity, maxArity := paramArity(fd.params, fd.paramCount); minArity == maxArity {
			continue
		}
		g.emit("static QValue %s(int index) {\n", defaultArgFnName(fd.name))
		g.indentLevel++
		g.emitLine("switch (index) {")
		for i, p := range fd.params.Children {
			if p == nil || p.DefaultValue == nil {
				continue
			}
			g.emitLine("case %d: {", i)
			g.indentLevel++
			g.emitLine("return %s;", g.generateExpr(p.DefaultValue))
			g.indentLevel--
			g.emitLine("}")
		}
		g.emitLine("}")
		g.emitLine("return qv_null();")
		g.indentLevel--
		g.emit("}\n\n")
	}
}

// lambdaDisplayName names a lambda for runtime messages: the name it is bound
// to (`fn f(...)` is `f = fn(...)`), or "lambda".
func (g *Generator) lambdaDisplayName(node *ast.TreeNode) string {
	if name, ok := g.lambdaUserNames[node]; ok {
		return g.displayFuncName(name)
	}
	return "lambda"
}

// paramAnnotation returns the type name a parameter is annotated with, or "".
func paramAnnotation(param *ast.TreeNode) string {
	if param == nil || param.NodeType != ast.ParameterNode {
		return ""
	}
	for _, c := range param.Children[min(1, len(param.Children)):] {
		if c != nil && c.NodeType == ast.TypeNode {
			return c.TokenLiteral()
		}
	}
	return ""
}

// paramTagChecks maps non-scalar annotations to their QValue tag and whether
// null is accepted (reference types other than result accept null, as in the
// checker).
var paramTagChecks = map[string]struct {
	tag       string
	allowNull bool
}{
	"str":      {"VAL_STRING", false},
	"list":     {"VAL_LIST", true},
	"dict":     {"VAL_DICT", true},
	"vec":      {"VAL_VECTOR", true},
	"result":   {"VAL_RESULT", false},
	"resource": {"VAL_RESOURCE", true},
	"fn":       {"VAL_FUNC", true},
}

// emitParamBindings binds a QValue-convention function's parameters. Every
// annotated parameter is enforced on entry (q_param_* reports the caller's
// line, since the callee hasn't set a location yet); int, float and bool
// parameters are unboxed to native C++ locals (float widens an int) so the
// body works on raw values. Unannotated and `any` parameters stay QValue.
func (g *Generator) emitParamBindings(argsNode *ast.TreeNode, fnDisplay string) {
	if argsNode == nil {
		return
	}
	for _, param := range argsNode.Children {
		paramName := g.paramName(param)
		if paramName == "" {
			continue
		}
		cName := sanitizeVarName(paramName)
		argName := sanitizeArgName(paramName)
		fn := escapeCppString(fnDisplay)
		pn := escapeCppString(g.names.Display(paramName))

		value := argName // checked QValue expression
		scalarExpr, tier := "", ""
		switch ann := paramAnnotation(param); ann {
		case "", "any":
		case "int":
			scalarExpr, tier = fmt.Sprintf("q_param_int(%s, \"%s\", \"%s\")", argName, fn, pn), "long long"
			value = fmt.Sprintf("qv_int(%s)", scalarExpr)
		case "float":
			scalarExpr, tier = fmt.Sprintf("q_param_float(%s, \"%s\", \"%s\")", argName, fn, pn), "double"
			value = fmt.Sprintf("qv_float(%s)", scalarExpr)
		case "bool":
			scalarExpr, tier = fmt.Sprintf("q_param_bool(%s, \"%s\", \"%s\")", argName, fn, pn), "bool"
			value = fmt.Sprintf("qv_bool(%s)", scalarExpr)
		default:
			if c, ok := paramTagChecks[ann]; ok {
				value = fmt.Sprintf("q_param_check(%s, QValue::%s, %t, \"%s\", \"%s\", \"%s\")", argName, c.tag, c.allowNull, ann, fn, pn)
			} else {
				// A struct (the checker rejects unknown type names): checked
				// by definition, so two structs with the same name differ.
				value = fmt.Sprintf("q_param_struct(%s, &_sdef_%s, \"%s\", \"%s\")", argName, sanitizeVarName(ann), fn, pn)
			}
		}

		switch {
		case g.isCaptured(paramName):
			g.emitLine("QCell* %s = q_new_cell(%s);", cName, value)
			g.markCell(paramName)
		case tier != "":
			g.emitLine("%s %s = %s;", tier, cName, scalarExpr)
			g.markTier(paramName, tier)
		default:
			g.emitLine("QValue %s = %s;", cName, value)
		}
		g.declaredVars[paramName] = true
	}
}

// generateNativeFunction emits a native-typed function body and a QValue thunk.
func (g *Generator) generateNativeFunction(node *ast.TreeNode, funcName string, argsNode *ast.TreeNode, bodyNode *ast.TreeNode, proto *ir.CallPlan) {
	g.currentFuncNode = node
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
	g.emitLine("QSourceLocGuard _loc_guard;")

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
		// For native params with a scalar tier (int64_t/double/bool), store at
		// that tier so scalarExpr can optimise. For non-scalar native types
		// (const char*, QList*, etc.) box back to QValue so the body works
		// uniformly with generateExpr.
		tier := nativeCppTypeToTier(nativeType)
		if g.isCaptured(paramName) {
			boxed := boxNative(argName, nativeType)
			g.emitLine("QCell* %s = q_new_cell(%s);", cName, boxed)
			g.markCell(paramName)
		} else if tier != "" {
			// Scalar: store at native tier
			g.emitLine("%s %s = %s;", nativeType, cName, argName)
			g.markTier(paramName, tier)
		} else {
			// Non-scalar native (str, list, etc.): box to QValue
			g.emitLine("QValue %s = %s;", cName, boxNative(argName, nativeType))
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
				g.hoistScopes(bodyNode)
				for i, child := range bodyNode.Children {
					if i < len(bodyNode.Children)-1 && child.NodeType != ast.FnDeclNode {
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
		// The body's value may have a type the compiler doesn't know (a
		// callback's result, a dict field), so it is converted with the
		// checked q_as_* helpers; an int result widens to a float return.
		nativeResult = adaptArgForExtern(result, retType)
	}
	g.emitLine("return %s;", nativeResult)

	g.indentLevel--
	g.emit("}\n\n")

	g.popScope()
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
	g.emitLine("return %s;", boxNative(callExpr, retType))

	g.indentLevel--
	g.emit("}\n\n")
}

// generateExternThunk emits a QValue-convention thunk for a free extern fn so it can
// be stored in a QClosure and used as a first-class value. The thunk unboxes each
// QValue argument to the native type, calls the native symbol directly, and boxes
// the result back to QValue.
func (g *Generator) generateExternThunk(proto *ir.CallPlan) {
	symbol := proto.RuntimeSymbol // e.g. "q_sqrt"
	paramCount := len(proto.NativeParamTypes)

	thunkParams := []string{"QClosure* _cl"}
	argNames := make([]string, paramCount)
	for i := range argNames {
		argNames[i] = fmt.Sprintf("_a%d", i)
		thunkParams = append(thunkParams, fmt.Sprintf("QValue _a%d", i))
	}
	g.emit("QValue _thunk_%s(%s) {\n", symbol, strings.Join(thunkParams, ", "))
	g.indentLevel++

	adaptedArgs := make([]string, paramCount)
	for i, argName := range argNames {
		nativeType := "QValue"
		if i < len(proto.NativeParamTypes) {
			nativeType = proto.NativeParamTypes[i]
		}
		adaptedArgs[i] = adaptArgForExtern(argName, nativeType)
	}

	callExpr := fmt.Sprintf("%s(%s)", symbol, strings.Join(adaptedArgs, ", "))
	g.emitLine("return %s;", boxNative(callExpr, proto.NativeReturnType))

	g.indentLevel--
	g.emit("}\n\n")
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
		if child == nil || child.NodeType == ast.ModuleNode {
			continue
		}
		if child.NodeType == ast.FnDeclNode {
			continue // hoisted
		}
		g.emitSourceLoc(child)
		expr := g.generateExpr(child)
		g.emitStmtExpr(child, expr)
	}
}

func (g *Generator) generateBlock(node *ast.TreeNode) string {
	if node == nil {
		return "qv_null()"
	}
	g.pushBlockScope()
	defer g.popScope()
	g.hoistScopes(node)
	var lastExpr string = "qv_null()"
	for idx, child := range node.Children {
		g.emitSourceLoc(child)
		lastExpr = g.generateExpr(child)
		if idx < len(node.Children)-1 {
			g.emitStmtExpr(child, lastExpr)
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
	case ast.FnDeclNode:
		return g.fixedFnValue(node)
	case ast.AssignNode:
		return g.generateAssign(node)
	case ast.MemberNode:
		return g.generateMember(node)
	case ast.UnaryNode:
		return g.generateUnary(node)
	case ast.BinaryNode:
		return g.generateBinary(node)
	case ast.FunctionCallNode:
		return g.generateFunctionCall(node)
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
	case ast.StructDefNode:
		// Struct definitions are emitted as static data by emitStructDefs(); no runtime code here
		return "qv_null()"
	case ast.StructLiteralNode:
		return g.generateStructLiteral(node)
	case ast.ExternHeaderNode, ast.ExternFnNode, ast.ExternLinkNode, ast.ExternIncludePathNode:
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
		return fmt.Sprintf("qv_string(\"%s\")", escapeCppString(node.Token.Literal))
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
	if f := g.fnRefs[node]; f != nil {
		return directFnValue(f)
	}
	if g.declaredVars[name] {
		if g.isCell(name) {
			if g.checkedCells[name] {
				return fmt.Sprintf("q_cell_get(%s, \"%s\")", sanitizeVarName(name), escapeCppString(g.names.Display(name)))
			}
			return fmt.Sprintf("%s->value", sanitizeVarName(name))
		}
		cName := sanitizeVarName(name)
		// Box scalar variables back to QValue at read sites so the rest of
		// codegen always works with QValue expressions.
		return boxExpr(cName, g.tierOf(name))
	}
	// Extern fn free functions used as first-class values: wrap the thunk.
	if proto, isExtern := g.externFns[name]; isExtern && !proto.IsMethod {
		return fmt.Sprintf("qv_func((void*)_thunk_%s, &%s)", proto.RuntimeSymbol, externFnInfoSymbol(proto.RuntimeSymbol))
	}
	return sanitizeVarName(name)
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

	case ast.UnaryNode, ast.BinaryNode:
		if node.Token == nil {
			return "", ""
		}
		op := node.Token.Type

		// Unary minus: -x where x is scalar
		if len(node.Children) == 1 && op == token.MINUS {
			operand, tier := g.scalarExpr(node.Children[0])
			if tier == "long long" {
				// Literal negation cannot overflow; anything else is checked.
				if child := node.Children[0]; child.NodeType == ast.LiteralNode {
					return fmt.Sprintf("(-%s)", operand), tier
				}
				return fmt.Sprintf("q_checked_neg(%s)", operand), tier
			}
			if tier == "double" {
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
						return checkedScalarArith(op, cppOp, resTier, lRaw, rRaw), resTier
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

// checkedScalarArith emits a scalar-lowered binary arithmetic expression with
// the same failure semantics as the boxed runtime operators: int overflow,
// modulo by zero and division by zero are fatal, never undefined behavior.
func checkedScalarArith(op token.TokenType, cppOp, resTier, lRaw, rRaw string) string {
	if resTier == "long long" {
		switch op {
		case token.PLUS:
			return fmt.Sprintf("q_checked_add(%s, %s)", lRaw, rRaw)
		case token.MINUS:
			return fmt.Sprintf("q_checked_sub(%s, %s)", lRaw, rRaw)
		case token.MULTIPLY:
			return fmt.Sprintf("q_checked_mul(%s, %s)", lRaw, rRaw)
		case token.MODULO:
			return fmt.Sprintf("q_checked_mod(%s, %s)", lRaw, rRaw)
		}
	}
	if resTier == "double" && op == token.DIVIDE {
		return fmt.Sprintf("q_checked_fdiv((double)%s, (double)%s)", lRaw, rRaw)
	}
	return fmt.Sprintf("((%s)%s %s (%s)%s)", resTier, lRaw, cppOp, resTier, rRaw)
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

// generateUnary emits `-x` and `!x`.
func (g *Generator) generateUnary(node *ast.TreeNode) string {
	if node.Token == nil || len(node.Children) != 1 {
		return "qv_null()"
	}
	switch node.Token.Type {
	case token.MINUS:
		// scalarExpr on the whole node applies checked int negation.
		if raw, tier := g.scalarExpr(node); tier != "" && tier != "bool" {
			return boxExpr(raw, tier)
		}
		return fmt.Sprintf("q_neg(%s)", g.generateExpr(node.Children[0]))
	case token.BANG:
		return fmt.Sprintf("qv_bool(!q_truthy(%s))", g.generateExpr(node.Children[0]))
	}
	return g.generateExpr(node.Children[0])
}

// generateMember emits `object.name`: a struct field by index, otherwise a
// dict key.
func (g *Generator) generateMember(node *ast.TreeNode) string {
	if len(node.Children) < 2 {
		return "qv_null()"
	}
	obj := g.generateExpr(node.Children[0])
	memberName := node.Children[1].TokenLiteral()
	if st, ok := g.nodeType(node.Children[0]).(*types.StructType); ok {
		if _, idx := st.FieldByName(memberName); idx >= 0 {
			return fmt.Sprintf("q_struct_get_field(%s, %d)", obj, idx)
		}
	}
	return fmt.Sprintf("q_member_get(%s, \"%s\")", obj, memberName)
}

// binaryRuntimeOps maps boxed binary operators to their runtime functions.
var binaryRuntimeOps = map[token.TokenType]string{
	token.PLUS:       "q_add",
	token.MINUS:      "q_sub",
	token.MULTIPLY:   "q_mul",
	token.DIVIDE:     "q_div",
	token.MODULO:     "q_mod",
	token.DOUBLESTAR: "q_pow",
	token.LT:         "q_lt",
	token.LTE:        "q_lte",
	token.GT:         "q_gt",
	token.GTE:        "q_gte",
	token.DEQ:        "q_eq",
	token.NE:         "q_neq",
}

// generateBinary emits arithmetic, comparison and `and`/`or`, lowered to raw
// C++ scalars when both operands are scalar.
func (g *Generator) generateBinary(node *ast.TreeNode) string {
	if node.Token == nil || len(node.Children) != 2 {
		return "qv_null()"
	}
	op := node.Token.Type
	if raw, tier := g.scalarBinary(node); tier != "" {
		return boxExpr(raw, tier)
	}
	// Short-circuit logical operators: right operand must be evaluated lazily.
	// Python semantics: `and` returns the first falsy operand or the last;
	// `or` returns the first truthy operand or the last.
	switch op {
	case token.AND:
		left := g.generateExpr(node.Children[0])
		right := g.generateExpr(node.Children[1])
		return fmt.Sprintf("([&]() -> QValue { auto _l = %s; return q_truthy(_l) ? (%s) : _l; }())", left, right)
	case token.OR:
		left := g.generateExpr(node.Children[0])
		right := g.generateExpr(node.Children[1])
		return fmt.Sprintf("([&]() -> QValue { auto _l = %s; return q_truthy(_l) ? _l : (%s); }())", left, right)
	}
	fn, ok := binaryRuntimeOps[op]
	if !ok {
		panicICEf("INV-OPERATOR", node, "unhandled binary operator '%s'", op.String())
		return "qv_null()"
	}
	left := g.generateExpr(node.Children[0])
	right := g.generateExpr(node.Children[1])
	return fmt.Sprintf("%s(%s, %s)", fn, left, right)
}

// scalarBinary lowers a binary arithmetic or comparison node whose operands
// are both scalar to a raw C++ expression; tier is "" when it can't.
func (g *Generator) scalarBinary(node *ast.TreeNode) (string, string) {
	op := node.Token.Type
	if cppArith := nativeArithOp(op); cppArith != "" && op != token.DOUBLESTAR {
		lRaw, lTier := g.scalarExpr(node.Children[0])
		rRaw, rTier := g.scalarExpr(node.Children[1])
		if lTier != "" && rTier != "" {
			if resTier := promotedTier(lTier, rTier, op); resTier != "" {
				// Cast operands to result tier to avoid C++ integer promotion surprises.
				lCast := fmt.Sprintf("((%s)%s)", resTier, lRaw)
				rCast := fmt.Sprintf("((%s)%s)", resTier, rRaw)
				return checkedScalarArith(op, cppArith, resTier, lCast, rCast), resTier
			}
		}
	}
	if cppCmp := nativeCompareOp(op); cppCmp != "" {
		lRaw, lTier := g.scalarExpr(node.Children[0])
		rRaw, rTier := g.scalarExpr(node.Children[1])
		if lTier != "" && rTier != "" {
			isOrdered := op == token.LT || op == token.LTE || op == token.GT || op == token.GTE
			if !isOrdered || (lTier != "bool" && rTier != "bool") {
				return fmt.Sprintf("(%s %s %s)", lRaw, cppCmp, rRaw), "bool"
			}
		}
	}
	return "", ""
}

// generateAssign emits `target = value` for a name, a member or an index.
func (g *Generator) generateAssign(node *ast.TreeNode) string {
	if len(node.Children) != 2 {
		return "qv_null()"
	}
	lhs, rhs := node.Children[0], node.Children[1]
	switch lhs.NodeType {
	case ast.MemberNode:
		if len(lhs.Children) >= 2 {
			right := g.generateExpr(rhs)
			obj := g.generateExpr(lhs.Children[0])
			g.emitLine("q_member_set(%s, \"%s\", %s);", obj, lhs.Children[1].TokenLiteral(), right)
			return right
		}
	case ast.IndexNode:
		if len(lhs.Children) >= 2 {
			right := g.generateExpr(rhs)
			target := g.generateExpr(lhs.Children[0])
			index := g.generateExpr(lhs.Children[1])
			g.emitLine("q_set(%s, %s, %s);", target, index, right)
			return right
		}
	case ast.IdentifierNode:
		return g.assignVar(lhs.TokenLiteral(), rhs)
	}
	panicICEf("INV-ASSIGN-TARGET", node, "unhandled assignment target '%s'", lhs.NodeType.String())
	return "qv_null()"
}

// assignVar emits `name = value`, declaring the variable on its first
// assignment with the storage its type and capture allow.
func (g *Generator) assignVar(varName string, rhs *ast.TreeNode) string {
	cName := sanitizeVarName(varName)
	// The first assignment of a captured variable declares its cell before
	// the value is built: a lambda assigned to it (`fib = fn(n) -> fib(n - 1)`)
	// captures that cell.
	if !g.declaredVars[varName] && g.isCaptured(varName) {
		g.emitLine("QCell* %s = q_new_cell(qv_null());", cName)
		g.declaredVars[varName] = true
		g.markCell(varName)
		g.emitLine("%s->value = %s;", cName, g.generateExpr(rhs))
		return fmt.Sprintf("%s->value", cName)
	}
	right := g.generateExpr(rhs)
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
			rawRight, rawTier := g.scalarExpr(rhs)
			if rawTier == "" {
				rawRight = unboxForTier(right, tier, nil)
			}
			g.emitLine("%s = %s;", cName, rawRight)
			return boxExpr(cName, tier)
		}
		g.emitLine("%s = %s;", cName, right)
		return cName
	}
	// First declaration: use scalar storage when the RHS has a statically-known
	// primitive type.
	if tier := nativeCType(g.nodeType(rhs)); tier != "" {
		rawRight, rawTier := g.scalarExpr(rhs)
		if rawTier == "" {
			// RHS is a compound QValue expression — unbox it.
			rawRight = unboxForTier(right, tier, nil)
		}
		g.emitLine("%s %s = %s;", tier, cName, rawRight)
		g.declaredVars[varName] = true
		g.markTier(varName, tier)
		return boxExpr(cName, tier)
	}
	g.emitLine("QValue %s = %s;", cName, right)
	g.declaredVars[varName] = true
	return cName
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

// generateDynamicMethodCall lowers a DispatchDynamicMethod call to a switch on the
// receiver's runtime type tag. args[0] is the receiver temp. The switch lives in an
// immediately-invoked lambda so the call stays an expression and is evaluated only
// where the expression is (e.g. not when short-circuited away).
func (g *Generator) generateDynamicMethodCall(node *ast.TreeNode, plan *ir.CallPlan, args []string) string {
	receiver := args[0]
	var b strings.Builder
	fmt.Fprintf(&b, "([&]() -> QValue { switch (%s.type) { ", receiver)
	for _, c := range plan.MethodCandidates {
		fmt.Fprintf(&b, "case QValue::%s: return %s; ", c.ValueTag, generateBuiltinCall(c.RuntimeSymbol, args))
	}
	b.WriteString("default: ")
	// Point the error at this call, not at the last statement that ran (which
	// may be inside a function called by an argument).
	if node != nil && node.Token != nil && !isStdlibSource(node) {
		fmt.Fprintf(&b, "q_set_source_loc(\"%s\", %d, %d); ", escapeCppString(g.sourceFileOf(node)), node.Token.Line, node.Token.Column)
	}
	fmt.Fprintf(&b, "q_no_method(%s, \"%s\"); } }())", receiver, escapeCppString(plan.CalleeName))
	return b.String()
}

func (g *Generator) generateFunctionCall(node *ast.TreeNode) string {
	if len(node.Children) < 2 {
		return "qv_null()"
	}

	funcNode := node.Children[0]
	argsNode := node.Children[1]
	plan := g.getCallPlanOrPanic(node)

	// Dot-call syntax on values is rejected by analyzer unless it's a known method dispatch.
	if funcNode.NodeType == ast.MemberNode && plan.Dispatch == ir.DispatchClosure {
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
	case ir.DispatchDynamicMethod:
		return g.generateDynamicMethodCall(funcNode, plan, args)
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
		return boxNative(callExpr, plan.NativeReturnType)
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
		return boxNative(callExpr, plan.NativeReturnType)
	case ir.DispatchClosure:
		// It is a closure/function value call by analyzer contract.
		funcExpr := g.generateExpr(funcNode)
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

// isRangeCall returns the argument nodes if the given node is a range() builtin call.
func (g *Generator) isRangeCall(node *ast.TreeNode) ([]*ast.TreeNode, bool) {
	if node == nil || node.NodeType != ast.FunctionCallNode || len(node.Children) < 2 {
		return nil, false
	}
	plan := g.callPlans[node]
	if plan == nil || plan.RuntimeSymbol != "q_range" {
		return nil, false
	}
	argsNode := node.Children[1]
	if argsNode == nil || len(argsNode.Children) == 0 {
		return nil, false
	}
	return argsNode.Children, true
}

// emitRangeScalar generates a long long expression for a range() argument.
// Scalar ints are used as is and scalar floats truncated, like the q_range
// builtins; any other value goes through q_range_arg, which checks its type at
// runtime. sig and what name the call form and argument for that error message.
func (g *Generator) emitRangeScalar(argNode *ast.TreeNode, sig, what string) string {
	if raw, tier := g.scalarExpr(argNode); tier == "long long" {
		return raw
	}
	if raw, tier := g.scalarExpr(argNode); tier == "double" {
		return fmt.Sprintf("(long long)(%s)", raw)
	}
	expr := g.generateExpr(argNode)
	return fmt.Sprintf("q_range_arg(%s, \"%s\", \"%s\")", expr, sig, what)
}

// generateForRange emits a raw C++ for-loop for `for i in range(...)`.
func (g *Generator) generateForRange(cVarName string, varName string, args []*ast.TreeNode, bodyNode *ast.TreeNode) string {
	startTemp := g.newTemp()
	endTemp := g.newTemp()
	stepTemp := g.newTemp()

	switch len(args) {
	case 1:
		// range(end): 0 to end, step 1
		g.emitLine("long long %s = 0;", startTemp)
		g.emitLine("long long %s = %s;", endTemp, g.emitRangeScalar(args[0], "range(end)", "end"))
		g.emitLine("long long %s = 1;", stepTemp)
	case 2:
		// range(start, end): auto-direction
		g.emitLine("long long %s = %s;", startTemp, g.emitRangeScalar(args[0], "range(start, end)", "start"))
		g.emitLine("long long %s = %s;", endTemp, g.emitRangeScalar(args[1], "range(start, end)", "end"))
		g.emitLine("long long %s = (%s <= %s) ? 1LL : -1LL;", stepTemp, startTemp, endTemp)
	default:
		// range(start, end, step)
		g.emitLine("long long %s = %s;", startTemp, g.emitRangeScalar(args[0], "range(start, end, step)", "start"))
		g.emitLine("long long %s = %s;", endTemp, g.emitRangeScalar(args[1], "range(start, end, step)", "end"))
		g.emitLine("long long %s = q_range_step(%s);", stepTemp, g.emitRangeScalar(args[2], "range(start, end, step)", "step"))
	}

	g.emitLine("for (long long %s = %s; %s > 0 ? (%s < %s) : (%s > %s); %s += %s) {",
		cVarName, startTemp,
		stepTemp, cVarName, endTemp, cVarName, endTemp,
		cVarName, stepTemp)
	g.indentLevel++

	g.pushBlockScope()
	g.declaredVars[varName] = true
	g.markTier(varName, "long long")

	if bodyNode.NodeType == ast.BlockNode {
		for _, stmt := range bodyNode.Children {
			expr := g.generateExpr(stmt)
			g.emitStmtExpr(stmt, expr)
		}
	} else {
		expr := g.generateExpr(bodyNode)
		g.emitStmtExpr(bodyNode, expr)
	}

	g.popScope()
	g.indentLevel--
	g.emitLine("}")

	return "qv_null()"
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
	emitLoopBody := func() {
		if bodyNode.NodeType == ast.BlockNode {
			for _, stmt := range bodyNode.Children {
				expr := g.generateExpr(stmt)
				g.emitStmtExpr(stmt, expr)
			}
		} else {
			expr := g.generateExpr(bodyNode)
			g.emitStmtExpr(bodyNode, expr)
		}
	}

	// Range lowering: emit raw C++ loop instead of list allocation.
	if !g.isCaptured(varName) {
		if rangeArgs, isRange := g.isRangeCall(rangeNode); isRange {
			return g.generateForRange(cVarName, varName, rangeArgs, bodyNode)
		}
	}

	// Handle iterable iteration (list/string/vec).
	listExpr := g.generateExpr(rangeNode)
	listTemp := g.newTemp()
	g.emitLine("QValue %s = %s;", listTemp, listExpr)

	emitGenericLoop := func() {
		lenTemp := g.newTemp()
		idxTemp := g.newTemp()

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
				delete(g.varTiers, varName) // clear any inherited scalar tier from outer scope
			}
		}

		emitLoopBody()

		g.popScope()
		g.indentLevel--
		g.emitLine("}")
	}

	// Fast path: avoid per-element q_iter_get boxing for scalar-tier loops over
	// valid null-free vectors of the exact matching dtype. Reads the vector's
	// Arrow-layout buffers directly.
	if !g.isCaptured(varName) {
		loopVarTier := g.loopVarTier(rangeNode)
		dtype := ""
		elemExpr := ""
		switch loopVarTier {
		case "long long":
			dtype = "QVector::Type::I64"
		case "double":
			dtype = "QVector::Type::F64"
		case "bool":
			dtype = "QVector::Type::BOOL"
		}
		if dtype != "" {
			vecRef := g.newTemp()
			idxTemp := g.newTemp()
			switch loopVarTier {
			case "long long":
				elemExpr = fmt.Sprintf("static_cast<long long>(q_vec_i64_data(%s)[%s])", vecRef, idxTemp)
			case "double":
				elemExpr = fmt.Sprintf("q_vec_f64_data(%s)[%s]", vecRef, idxTemp)
			case "bool":
				elemExpr = fmt.Sprintf("q_vec_bool_at(%s, %s)", vecRef, idxTemp)
			}

			g.emitLine("if (q_vec_is_dense_of(%s, %s)) {", listTemp, dtype)
			g.indentLevel++
			g.emitLine("const QVector& %s = *%s.data.vector_val;", vecRef, listTemp)
			g.emitLine("for (long long %s = 0; %s < (long long)%s.count; %s++) {", idxTemp, idxTemp, vecRef, idxTemp)
			g.indentLevel++

			g.pushBlockScope()
			g.declaredVars[varName] = true
			g.emitLine("%s %s = %s;", loopVarTier, cVarName, elemExpr)
			g.markTier(varName, loopVarTier)
			emitLoopBody()
			g.popScope()

			g.indentLevel--
			g.emitLine("}")
			g.indentLevel--
			g.emitLine("} else {")
			g.indentLevel++
			emitGenericLoop()
			g.indentLevel--
			g.emitLine("}")
			return "qv_null()"
		}
	}

	emitGenericLoop()
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
			g.emitStmtExpr(stmt, expr)
		}
	} else {
		expr := g.generateExpr(bodyNode)
		g.emitStmtExpr(bodyNode, expr)
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

// vectorTypedOps returns the C++ constructor and typed push function for a known
// vector element type. Returns ("", "") if the type requires the list fallback.
func vectorTypedOps(elemType types.Type) (constructor string, pushFn string) {
	if elemType == nil {
		return "", ""
	}
	if elemType.Equals(types.TypeFloat) {
		return "qv_vector", "q_vec_push"
	}
	if elemType.Equals(types.TypeInt) {
		return "qv_vector_i64", "q_vec_push_i64"
	}
	if elemType.Equals(types.TypeBool) {
		return "qv_vector_bool", "q_vec_push_bool"
	}
	return "", ""
}

func (g *Generator) generateVector(node *ast.TreeNode) string {
	n := len(node.Children)

	// Direct typed construction when the analyzer knows the element type.
	if vt, ok := g.nodeType(node).(*types.VectorType); ok && vt != nil {
		ctor, pushFn := vectorTypedOps(vt.ElementType)
		if ctor != "" && pushFn != "" {
			temp := g.newTemp()
			g.emitLine("QValue %s = %s(%d);", temp, ctor, n)
			for _, child := range node.Children {
				elem := g.generateExpr(child)
				g.emitLine("%s = %s(%s, %s);", temp, pushFn, temp, elem)
			}
			return temp
		}
	}

	// Fallback: build list then convert.
	tempList := g.newTemp()
	g.emitLine("QValue %s = qv_list(%d);", tempList, len(node.Children))

	for _, child := range node.Children {
		elem := g.generateExpr(child)
		g.emitLine("%s = q_push(%s, %s);", tempList, tempList, elem)
	}

	return fmt.Sprintf("q_to_vec(%s)", tempList)
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
	declType := g.nodeType(node)
	rhsType := g.nodeType(valueNode)
	if declType == nil || !declType.Equals(types.TypeAny) {
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
		g.funcDecls = append(g.funcDecls, funcDecl{name: lambdaName, paramCount: paramCount, params: lambdaParams(node), display: g.lambdaDisplayName(node)})
	}

	// Check if this lambda captures any variables
	caps, hasCaps := g.captures[node]
	if hasCaps && len(caps) > 0 {
		// Emit closure allocation with captured values
		clTemp := g.newTemp()
		g.emitLine("QClosure* %s = q_alloc_closure((void*)quark_%s, &%s, %d);", clTemp, lambdaName, fnInfoSymbol(lambdaName), len(caps))
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
		return fmt.Sprintf("qv_func((void*)quark_%s__thunk, &%s)", lambdaName, fnInfoSymbol(lambdaName))
	}
	return fmt.Sprintf("qv_func((void*)quark_%s, &%s)", lambdaName, fnInfoSymbol(lambdaName))
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
	g.emitLine("QSourceLocGuard _loc_guard;")

	g.emitParamBindings(argsNode, g.lambdaDisplayName(node))

	// Extract captured variables from closure — always QCell* (shared with outer scope)
	savedChecked := g.checkedCells
	g.checkedCells = g.checkedCaptures[node]
	defer func() { g.checkedCells = savedChecked }()
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
	g.emitLine("return %s;", result)

	g.indentLevel--
	g.emit("}\n\n")

	g.popScope() // Restore previous scope
	g.currentFuncNode = nil
}

// emitStructDefs emits static QStructDef constants for each registered struct type.
func (g *Generator) emitStructDefs() {
	if len(g.structTypes) == 0 {
		return
	}
	// Sort names for deterministic output across compilations.
	names := make([]string, 0, len(g.structTypes))
	for name := range g.structTypes {
		names = append(names, name)
	}
	sort.Strings(names)
	g.emit("// Struct type definitions\n")
	for _, name := range names {
		st := g.structTypes[name]
		safeName := sanitizeVarName(st.Name)

		// Emit field name array
		g.emitLine("static const char* _sfields_%s[] = {", safeName)
		g.indentLevel++
		for _, f := range st.Fields {
			g.emitLine("\"%s\",", escapeCppString(f.Name))
		}
		g.indentLevel--
		g.emitLine("};")

		// Emit QStructDef
		// The runtime name is the display name (geo::Point); identity is the
		// QStructDef's address, one per definition.
		g.emitLine("static const QStructDef _sdef_%s = {\"%s\", _sfields_%s, %d};",
			safeName, escapeCppString(g.names.Display(st.Name)), safeName, len(st.Fields))
	}
	g.emit("\n")
}

// checkedFieldValue makes a struct field value match the field's declared
// type before it is stored. Field reads trust the stored tag (a float field is
// read as .data.float_val), so a value is stored unchecked only when the
// analyzer proved its type. An int going into a float field is widened, as for
// parameters; any other unproven value goes through a q_field_* check, so a
// mismatch is a runtime error instead of a garbage read.
func (g *Generator) checkedFieldValue(st *types.StructType, field types.StructField, valueNode *ast.TreeNode, value string) string {
	ft := field.Type
	if ft == nil || ft.Equals(types.TypeAny) {
		return value
	}
	vt := g.nodeType(valueNode)
	proven := vt != nil && !types.IsErrorType(vt) && !vt.Equals(types.TypeAny)
	if _, isUnion := vt.(*types.UnionType); isUnion {
		proven = false
	}
	sname := escapeCppString(g.names.Display(st.Name))
	fname := escapeCppString(field.Name)
	switch {
	case ft.Equals(types.TypeInt):
		if proven && vt.Equals(types.TypeInt) {
			return value
		}
		return fmt.Sprintf("q_field_int(%s, \"%s\", \"%s\")", value, sname, fname)
	case ft.Equals(types.TypeFloat):
		if proven && vt.Equals(types.TypeFloat) {
			return value
		}
		return fmt.Sprintf("q_field_float(%s, \"%s\", \"%s\")", value, sname, fname)
	case ft.Equals(types.TypeBool):
		if proven && vt.Equals(types.TypeBool) {
			return value
		}
		return fmt.Sprintf("q_field_bool(%s, \"%s\", \"%s\")", value, sname, fname)
	}
	if fst, ok := ft.(*types.StructType); ok {
		if vst, ok := vt.(*types.StructType); proven && ok && vst.Name == fst.Name {
			return value
		}
		if proven && vt.Equals(types.TypeNull) {
			return value
		}
		return fmt.Sprintf("q_field_struct(%s, &_sdef_%s, \"%s\", \"%s\")", value, sanitizeVarName(fst.Name), sname, fname)
	}
	var key string
	switch t := ft.(type) {
	case *types.ListType:
		key = "list"
	case *types.DictType:
		key = "dict"
	case *types.VectorType:
		key = "vec"
	case *types.ResultType:
		key = "result"
	case *types.FunctionType:
		key = "fn"
	default:
		switch {
		case t.Equals(types.TypeString):
			key = "str"
		case t.Equals(types.TypeResource):
			key = "resource"
		case t.Equals(types.TypeFn):
			key = "fn"
		}
	}
	check, ok := paramTagChecks[key]
	if !ok {
		return value
	}
	if proven && types.CanAssign(ft, vt) {
		return value
	}
	return fmt.Sprintf("q_field_check(%s, QValue::%s, %t, \"%s\", \"%s\", \"%s\")", value, check.tag, check.allowNull, key, sname, fname)
}

// generateStructLiteral emits code to construct a struct value.
func (g *Generator) generateStructLiteral(node *ast.TreeNode) string {
	structName := node.TokenLiteral()
	st := g.structTypes[structName]
	if st == nil {
		panicICEf("INV-STRUCT-LITERAL", node, "unknown struct type '%s'", structName)
		return "qv_null()"
	}

	safeName := sanitizeVarName(structName)
	temp := g.newTemp()

	// Create the struct instance
	g.emitLine("QStruct* %s_raw = q_struct_create(&_sdef_%s);", temp, safeName)

	// Build a map of provided field values from the literal
	provided := make(map[string]*ast.TreeNode)
	for _, pair := range node.Children {
		if pair.NodeType == ast.StructFieldNode && len(pair.Children) >= 2 {
			fieldName := pair.TokenLiteral()
			provided[fieldName] = pair.Children[1] // value expression
		}
	}

	// Emit field assignments in declaration order
	for i, field := range st.Fields {
		if valueNode, ok := provided[field.Name]; ok {
			value := g.checkedFieldValue(st, field, valueNode, g.generateExpr(valueNode))
			g.emitLine("%s_raw->fields[%d] = %s;", temp, i, value)
		} else if field.HasDefault {
			// Emit default value expression
			if defaultNode, ok := field.DefaultNode.(*ast.TreeNode); ok {
				value := g.checkedFieldValue(st, field, defaultNode, g.generateExpr(defaultNode))
				g.emitLine("%s_raw->fields[%d] = %s;", temp, i, value)
			} else {
				panicICEf("INV-STRUCT-DEFAULT", nil, "struct '%s' field '%s' has non-nil HasDefault but DefaultNode is not *ast.TreeNode", structName, field.Name)
			}
		}
	}

	// Wrap in QValue
	g.emitLine("QValue %s = qv_struct(%s_raw);", temp, temp)
	return temp
}
