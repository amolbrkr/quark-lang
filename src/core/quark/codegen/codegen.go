package codegen

import (
	_ "embed"
	"fmt"
	"quark/ast"
	"quark/token"
	"strings"
)

//go:embed runtime.hpp
var runtimeHeader string

// funcDecl stores a function name and its parameter count for forward declarations
type funcDecl struct {
	name       string
	paramCount int
}

// Generator generates C code from an AST
type Generator struct {
	output        strings.Builder
	indentLevel   int
	funcDecls     []funcDecl               // Function declarations with param counts
	lambdas       []*ast.TreeNode          // Lambda expressions to generate
	lambdaNames   map[*ast.TreeNode]string // Maps lambda nodes to their generated names
	tempCounter   int
	lambdaCounter int
	inFunction    bool
	currentFunc   string
	declaredVars  map[string]bool            // Tracks declared variables to avoid redeclaration
	scopeStack    []map[string]bool          // Stack of variable scopes for nested blocks
	embedRuntime  bool                       // If true, embed full runtime; if false, use #include
	captures      map[*ast.TreeNode][]string // Lambda node → captured variable names (from analyzer)
	funcNames     map[string]bool            // Set of declared function names (for first-class funcs)
}

func New() *Generator {
	return &Generator{
		funcDecls:    make([]funcDecl, 0),
		lambdas:      make([]*ast.TreeNode, 0),
		lambdaNames:  make(map[*ast.TreeNode]string),
		tempCounter:  0,
		declaredVars: make(map[string]bool),
		scopeStack:   make([]map[string]bool, 0),
		embedRuntime: false, // Default: use #include instead of embedding
		captures:     make(map[*ast.TreeNode][]string),
		funcNames:    make(map[string]bool),
	}
}

// SetCaptures passes the captured variable info from the analyzer to the generator
func (g *Generator) SetCaptures(captures map[*ast.TreeNode][]string) {
	if captures != nil {
		g.captures = captures
	}
}

// SetEmbedRuntime configures whether to embed the full runtime or use #include
func (g *Generator) SetEmbedRuntime(embed bool) {
	g.embedRuntime = embed
}

// sanitizeVarName prefixes all user variable names with quark_ to avoid
// collisions with C++ reserved keywords. This is the same prefix used for
// user-defined functions, giving a uniform naming scheme in generated code.
func sanitizeVarName(name string) string {
	return "quark_" + name
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
	g.declaredVars = make(map[string]bool)
}

func (g *Generator) pushBlockScope() {
	parent := g.declaredVars
	g.scopeStack = append(g.scopeStack, parent)
	child := make(map[string]bool)
	for k, v := range parent {
		child[k] = v
	}
	g.declaredVars = child
}

// popScope restores the previous variable scope
func (g *Generator) popScope() {
	if len(g.scopeStack) > 0 {
		g.declaredVars = g.scopeStack[len(g.scopeStack)-1]
		g.scopeStack = g.scopeStack[:len(g.scopeStack)-1]
	}
}

// Generate produces C++ code from the AST
func (g *Generator) Generate(node *ast.TreeNode) string {
	// Emit C++ runtime header
	if g.embedRuntime {
		// Embed full runtime (use WriteString directly to avoid % interpretation)
		g.output.WriteString(runtimeHeader)
		g.output.WriteString("\n")
	} else {
		// Use external header (clean, readable output)
		g.output.WriteString("#include \"quark/quark.hpp\"\n\n")
	}

	g.output.WriteString("// Forward declarations\n")

	// First pass: collect function declarations
	g.collectFunctions(node)

	// Emit forward declarations with correct parameter counts
	// All functions take QClosure* as hidden first parameter for closure support
	for _, fd := range g.funcDecls {
		params := []string{"QClosure*"}
		for i := 0; i < fd.paramCount; i++ {
			params = append(params, "QValue")
		}
		g.emitLine("QValue quark_%s(%s);", fd.name, strings.Join(params, ", "))
	}
	g.emit("\n")

	// Generate function definitions
	g.generateNode(node)

	// Generate main function
	g.emit("\nint main() {\n")
	g.indentLevel++
	g.emitLine("q_gc_init();")

	// Generate top-level statements that aren't function/module definitions
	for _, child := range node.Children {
		if child.NodeType != ast.FunctionNode && child.NodeType != ast.ModuleNode && child.NodeType != ast.UseNode {
			g.emitLine("%s;", g.generateExpr(child))
		}
	}

	g.emitLine("return 0;")
	g.indentLevel--
	g.emit("}\n")

	return g.output.String()
}

func (g *Generator) collectFunctions(node *ast.TreeNode) {
	switch node.NodeType {
	case ast.FunctionNode:
		if len(node.Children) >= 2 {
			name := node.Children[0].TokenLiteral()
			argsNode := node.Children[1]
			paramCount := len(argsNode.Children)
			g.funcDecls = append(g.funcDecls, funcDecl{name: name, paramCount: paramCount})
			g.funcNames[name] = true
		}
	case ast.LambdaNode:
		// Assign a unique name to this lambda
		lambdaName := g.newLambda()
		g.lambdaNames[node] = lambdaName
		g.lambdas = append(g.lambdas, node)
		paramCount := 0
		if len(node.Children) >= 1 {
			paramCount = len(node.Children[0].Children)
		}
		g.funcDecls = append(g.funcDecls, funcDecl{name: lambdaName, paramCount: paramCount})
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
	g.currentFunc = funcName
	g.inFunction = true
	g.pushScope() // Create new scope for function

	// Build parameter list: QClosure* _cl as hidden first param, then user params
	params := []string{"QClosure* _cl"}
	for _, param := range argsNode.Children {
		paramName := g.paramName(param)
		if paramName == "" {
			continue
		}
		cParamName := sanitizeVarName(paramName)
		params = append(params, fmt.Sprintf("QValue %s", cParamName))
		g.declaredVars[paramName] = true // Parameters are already declared
	}

	g.emit("QValue quark_%s(%s) {\n", funcName, strings.Join(params, ", "))
	g.indentLevel++

	// Generate body
	result := g.generateBlock(bodyNode)
	g.emitLine("return %s;", result)

	g.indentLevel--
	g.emit("}\n\n")

	g.popScope() // Restore previous scope
	g.inFunction = false
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

func (g *Generator) generateBlock(node *ast.TreeNode) string {
	if node == nil {
		return "qv_null()"
	}
	g.pushBlockScope()
	defer g.popScope()
	var lastExpr string = "qv_null()"
	for idx, child := range node.Children {
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
	default:
		return "qv_null()"
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
	if g.funcNames[name] {
		return fmt.Sprintf("qv_func((void*)quark_%s)", name)
	}
	return sanitizeVarName(name)
}

func (g *Generator) generateOperator(node *ast.TreeNode) string {
	if node.Token == nil {
		return "qv_null()"
	}

	op := node.Token.Type

	// Unary operators
	if len(node.Children) == 1 {
		operand := g.generateExpr(node.Children[0])
		switch op {
		case token.MINUS:
			return fmt.Sprintf("q_neg(%s)", operand)
		case token.BANG:
			return fmt.Sprintf("q_not(%s)", operand)
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
	case token.AND:
		return fmt.Sprintf("q_and(%s, %s)", left, right)
	case token.OR:
		return fmt.Sprintf("q_or(%s, %s)", left, right)
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
			// Variable already declared, just assign
			g.emitLine("%s = %s;", cName, right)
		} else {
			// First declaration
			g.emitLine("QValue %s = %s;", cName, right)
			g.declaredVars[varName] = true
		}
		return cName
	}

	return "qv_null()"
}

func (g *Generator) generateFunctionCall(node *ast.TreeNode) string {
	if len(node.Children) < 2 {
		return "qv_null()"
	}

	funcNode := node.Children[0]
	argsNode := node.Children[1]

	// Dot-call syntax is rejected by the analyzer; if it somehow reaches codegen, fail
	if funcNode.NodeType == ast.OperatorNode && funcNode.Token != nil && funcNode.Token.Type == token.DOT {
		return "(fprintf(stderr, \"compile error: dot-call syntax is not supported\\n\"), qv_null())"
	}

	funcName := funcNode.TokenLiteral()

	// Generate arguments
	args := make([]string, 0)
	for _, arg := range argsNode.Children {
		args = append(args, g.generateExpr(arg))
	}

	// Built-in functions — lookup from centralized registry
	if result, ok := GenerateBuiltinCall(funcName, args); ok {
		return result
	}

	// Check if this is a known user-defined function
	isKnownFunc := false
	for _, fd := range g.funcDecls {
		if fd.name == funcName {
			isKnownFunc = true
			break
		}
	}

	if isKnownFunc {
		// User-defined function - call directly with nullptr closure
		if len(args) == 0 {
			return fmt.Sprintf("quark_%s(nullptr)", funcName)
		}
		return fmt.Sprintf("quark_%s(nullptr, %s)", funcName, strings.Join(args, ", "))
	}

	// Otherwise, it might be a function value - use dynamic call
	funcExpr := g.generateExpr(funcNode)
	switch len(args) {
	case 0:
		return fmt.Sprintf("q_call0(%s)", funcExpr)
	case 1:
		return fmt.Sprintf("q_call1(%s, %s)", funcExpr, args[0])
	case 2:
		return fmt.Sprintf("q_call2(%s, %s, %s)", funcExpr, args[0], args[1])
	case 3:
		return fmt.Sprintf("q_call3(%s, %s, %s, %s)", funcExpr, args[0], args[1], args[2])
	case 4:
		return fmt.Sprintf("q_call4(%s, %s, %s, %s, %s)", funcExpr, args[0], args[1], args[2], args[3])
	default:
		// For more than 4 args, fall back to direct call including hidden closure param
		return fmt.Sprintf("quark_%s(nullptr, %s)", funcName, strings.Join(args, ", "))
	}
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

	funcName := funcNode.TokenLiteral()
	args := []string{input}
	for _, arg := range argsNode.Children {
		args = append(args, g.generateExpr(arg))
	}

	// Built-in functions — lookup from centralized registry
	if result, ok := GenerateBuiltinCall(funcName, args); ok {
		return result
	}

	// Check if known user function
	isKnownFunc := false
	for _, fd := range g.funcDecls {
		if fd.name == funcName {
			isKnownFunc = true
			break
		}
	}
	if isKnownFunc {
		return fmt.Sprintf("quark_%s(nullptr, %s)", funcName, strings.Join(args, ", "))
	}

	// Dynamic call for function values
	funcExpr := g.generateExpr(funcNode)
	switch len(args) {
	case 1:
		return fmt.Sprintf("q_call1(%s, %s)", funcExpr, args[0])
	case 2:
		return fmt.Sprintf("q_call2(%s, %s, %s)", funcExpr, args[0], args[1])
	case 3:
		return fmt.Sprintf("q_call3(%s, %s, %s, %s)", funcExpr, args[0], args[1], args[2])
	default:
		return fmt.Sprintf("q_call1(%s, %s)", funcExpr, args[0])
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
	g.emitLine("QValue %s;", temp)

	cond := g.generateExpr(node.Children[0])
	g.emitLine("if (q_truthy(%s)) {", cond)
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
			g.emit(" else if (q_truthy(%s)) {\n", g.generateExpr(child.Children[0]))
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

	g.emitLine("QValue %s;", temp)
	g.emitLine("QValue %s = %s;", matchTemp, matchExpr)

	first := true
	for i := 1; i < len(node.Children); i++ {
		pattern := node.Children[i]
		if pattern.NodeType != ast.PatternNode || len(pattern.Children) < 2 {
			continue
		}

		// Last child is the result, others are patterns
		resultIdx := len(pattern.Children) - 1
		result := g.generateExpr(pattern.Children[resultIdx])

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
		for _, bind := range bindings {
			accessor := "q_result_value"
			if !bind.isOk {
				accessor = "q_result_error"
			}
			g.emitLine("QValue %s = %s(%s);", sanitizeVarName(bind.name), accessor, matchTemp)
		}
		g.emitLine("%s = %s;", temp, result)
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
	g.emitLine("QValue %s = q_iter_get(%s, qv_int(%s));", cVarName, listTemp, idxTemp)

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

	g.emitLine("while (q_truthy(%s)) {", g.generateExpr(condNode))
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
		g.emitLine("%s = %s;", cName, value)
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

	// No captures — use qv_func (allocates QClosure with 0 captures)
	return fmt.Sprintf("qv_func((void*)quark_%s)", lambdaName)
}

func (g *Generator) generateLambdaFunc(node *ast.TreeNode) {
	if len(node.Children) < 2 {
		return
	}

	lambdaName := g.lambdaNames[node]
	argsNode := node.Children[0]
	bodyNode := node.Children[1]

	g.inFunction = true
	g.currentFunc = lambdaName
	g.pushScope() // Create new scope for lambda

	// Build parameter list: QClosure* _cl as hidden first param, then user params
	params := []string{"QClosure* _cl"}
	for _, param := range argsNode.Children {
		paramName := g.paramName(param)
		if paramName == "" {
			continue
		}
		cParamName := sanitizeVarName(paramName)
		params = append(params, fmt.Sprintf("QValue %s", cParamName))
		g.declaredVars[paramName] = true // Parameters are already declared
	}

	g.emit("QValue quark_%s(%s) {\n", lambdaName, strings.Join(params, ", "))
	g.indentLevel++

	// Extract captured variables from closure
	if caps, ok := g.captures[node]; ok {
		for i, capName := range caps {
			cName := sanitizeVarName(capName)
			g.emitLine("QValue %s = _cl->captures[%d];", cName, i)
			g.declaredVars[capName] = true
		}
	}

	// Generate body - for lambdas, the body is a single expression
	result := g.generateExpr(bodyNode)
	g.emitLine("return %s;", result)

	g.indentLevel--
	g.emit("}\n\n")

	g.popScope() // Restore previous scope
	g.inFunction = false
}
