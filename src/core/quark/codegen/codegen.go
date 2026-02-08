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

// Generator generates C code from an AST
type Generator struct {
	output        strings.Builder
	indentLevel   int
	functions     []string           // Function definitions (generated separately)
	lambdas       []*ast.TreeNode    // Lambda expressions to generate
	lambdaNames   map[*ast.TreeNode]string // Maps lambda nodes to their generated names
	tempCounter   int
	lambdaCounter int
	inFunction    bool
	currentFunc   string
	declaredVars  map[string]bool    // Tracks declared variables to avoid redeclaration
	scopeStack    []map[string]bool  // Stack of variable scopes for nested blocks
	embedRuntime  bool               // If true, embed full runtime; if false, use #include
}

func New() *Generator {
	return &Generator{
		functions:    make([]string, 0),
		lambdas:      make([]*ast.TreeNode, 0),
		lambdaNames:  make(map[*ast.TreeNode]string),
		tempCounter:  0,
		declaredVars: make(map[string]bool),
		scopeStack:   make([]map[string]bool, 0),
		embedRuntime: false, // Default: use #include instead of embedding
	}
}

// SetEmbedRuntime configures whether to embed the full runtime or use #include
func (g *Generator) SetEmbedRuntime(embed bool) {
	g.embedRuntime = embed
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

// pushScope saves current variable scope and creates a new one
func (g *Generator) pushScope() {
	g.scopeStack = append(g.scopeStack, g.declaredVars)
	g.declaredVars = make(map[string]bool)
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

	// Emit forward declarations
	for _, fname := range g.functions {
		g.emitLine("QValue quark_%s();", fname)
	}
	g.emit("\n")

	// Generate function definitions
	g.generateNode(node)

	// Generate main function
	g.emit("\nint main() {\n")
	g.indentLevel++

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
		if len(node.Children) >= 1 {
			name := node.Children[0].TokenLiteral()
			// No module prefix - all functions are global in C
			// Modules are just a grouping mechanism in Quark
			g.functions = append(g.functions, name)
		}
	case ast.LambdaNode:
		// Assign a unique name to this lambda
		lambdaName := g.newLambda()
		g.lambdaNames[node] = lambdaName
		g.lambdas = append(g.lambdas, node)
		g.functions = append(g.functions, lambdaName)
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

	// Build parameter list and mark parameters as declared
	params := make([]string, 0)
	for _, param := range argsNode.Children {
		paramName := param.TokenLiteral()
		params = append(params, fmt.Sprintf("QValue %s", paramName))
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
	var lastExpr string = "qv_null()"
	for _, child := range node.Children {
		lastExpr = g.generateExpr(child)
		// Only emit as statement if it's not the last expression
		if child != node.Children[len(node.Children)-1] {
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
	case ast.IndexNode:
		return g.generateIndex(node)
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
		// Escape the string properly
		escaped := strings.ReplaceAll(node.Token.Literal, "\\", "\\\\")
		escaped = strings.ReplaceAll(escaped, "\"", "\\\"")
		escaped = strings.ReplaceAll(escaped, "\n", "\\n")
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
	return name
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
		case token.BANG, token.NOT:
			return fmt.Sprintf("q_not(%s)", operand)
		}
		return operand
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
		// Assignment - emit as statement and return the value
		varName := node.Children[0].TokenLiteral()
		if g.declaredVars[varName] {
			// Variable already declared, just assign
			g.emitLine("%s = %s;", varName, right)
		} else {
			// First declaration
			g.emitLine("QValue %s = %s;", varName, right)
			g.declaredVars[varName] = true
		}
		return varName
	}

	return "qv_null()"
}

func (g *Generator) generateFunctionCall(node *ast.TreeNode) string {
	if len(node.Children) < 2 {
		return "qv_null()"
	}

	funcNode := node.Children[0]
	argsNode := node.Children[1]

	funcName := funcNode.TokenLiteral()

	// Generate arguments
	args := make([]string, 0)
	for _, arg := range argsNode.Children {
		args = append(args, g.generateExpr(arg))
	}

	// Built-in functions
	switch funcName {
	case "print":
		if len(args) > 0 {
			return fmt.Sprintf("q_print(%s)", args[0])
		}
		return "q_print(qv_string(\"\"))"
	case "println":
		if len(args) > 0 {
			return fmt.Sprintf("q_println(%s)", args[0])
		}
		return "q_println(qv_string(\"\"))"
	case "len":
		if len(args) > 0 {
			return fmt.Sprintf("q_len(%s)", args[0])
		}
		return "qv_int(0)"
	case "input":
		return "q_input()"
	case "str":
		if len(args) > 0 {
			return fmt.Sprintf("q_str(%s)", args[0])
		}
		return "qv_string(\"\")"
	case "int":
		if len(args) > 0 {
			return fmt.Sprintf("q_int(%s)", args[0])
		}
		return "qv_int(0)"
	case "float":
		if len(args) > 0 {
			return fmt.Sprintf("q_float(%s)", args[0])
		}
		return "qv_float(0.0)"
	case "bool":
		if len(args) > 0 {
			return fmt.Sprintf("q_bool(%s)", args[0])
		}
		return "qv_bool(false)"
	case "range":
		if len(args) == 1 {
			return fmt.Sprintf("q_range(%s)", args[0])
		} else if len(args) == 2 {
			return fmt.Sprintf("q_range(%s, %s)", args[0], args[1])
		} else if len(args) >= 3 {
			return fmt.Sprintf("q_range(%s, %s, %s)", args[0], args[1], args[2])
		}
		return "qv_list()"
	// Math module functions
	case "abs":
		if len(args) > 0 {
			return fmt.Sprintf("q_abs(%s)", args[0])
		}
		return "qv_int(0)"
	case "min":
		if len(args) >= 2 {
			return fmt.Sprintf("q_min(%s, %s)", args[0], args[1])
		}
		return "qv_int(0)"
	case "max":
		if len(args) >= 2 {
			return fmt.Sprintf("q_max(%s, %s)", args[0], args[1])
		}
		return "qv_int(0)"
	case "sqrt":
		if len(args) > 0 {
			return fmt.Sprintf("q_sqrt(%s)", args[0])
		}
		return "qv_float(0.0)"
	case "floor":
		if len(args) > 0 {
			return fmt.Sprintf("q_floor(%s)", args[0])
		}
		return "qv_int(0)"
	case "ceil":
		if len(args) > 0 {
			return fmt.Sprintf("q_ceil(%s)", args[0])
		}
		return "qv_int(0)"
	case "round":
		if len(args) > 0 {
			return fmt.Sprintf("q_round(%s)", args[0])
		}
		return "qv_int(0)"
	// String module functions
	case "upper":
		if len(args) > 0 {
			return fmt.Sprintf("q_upper(%s)", args[0])
		}
		return "qv_string(\"\")"
	case "lower":
		if len(args) > 0 {
			return fmt.Sprintf("q_lower(%s)", args[0])
		}
		return "qv_string(\"\")"
	case "trim":
		if len(args) > 0 {
			return fmt.Sprintf("q_trim(%s)", args[0])
		}
		return "qv_string(\"\")"
	case "contains":
		if len(args) >= 2 {
			return fmt.Sprintf("q_contains(%s, %s)", args[0], args[1])
		}
		return "qv_bool(false)"
	case "startswith":
		if len(args) >= 2 {
			return fmt.Sprintf("q_startswith(%s, %s)", args[0], args[1])
		}
		return "qv_bool(false)"
	case "endswith":
		if len(args) >= 2 {
			return fmt.Sprintf("q_endswith(%s, %s)", args[0], args[1])
		}
		return "qv_bool(false)"
	case "replace":
		if len(args) >= 3 {
			return fmt.Sprintf("q_replace(%s, %s, %s)", args[0], args[1], args[2])
		}
		return "qv_string(\"\")"
	case "concat":
		if len(args) >= 2 {
			return fmt.Sprintf("q_concat(%s, %s)", args[0], args[1])
		}
		return "qv_string(\"\")"
	// List functions
	case "push":
		if len(args) >= 2 {
			return fmt.Sprintf("q_push(%s, %s)", args[0], args[1])
		}
		return "qv_null()"
	case "pop":
		if len(args) >= 1 {
			return fmt.Sprintf("q_pop(%s)", args[0])
		}
		return "qv_null()"
	case "get":
		if len(args) >= 2 {
			return fmt.Sprintf("q_get(%s, %s)", args[0], args[1])
		}
		return "qv_null()"
	case "set":
		if len(args) >= 3 {
			return fmt.Sprintf("q_set(%s, %s, %s)", args[0], args[1], args[2])
		}
		return "qv_null()"
	}

	// Check if this is a known user-defined function
	isKnownFunc := false
	for _, fname := range g.functions {
		if fname == funcName {
			isKnownFunc = true
			break
		}
	}

	if isKnownFunc {
		// User-defined function - call directly
		return fmt.Sprintf("quark_%s(%s)", funcName, strings.Join(args, ", "))
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
		// For more than 4 args, fall back to direct call (won't work for function values)
		return fmt.Sprintf("quark_%s(%s)", funcName, strings.Join(args, ", "))
	}
}

func (g *Generator) generatePipe(node *ast.TreeNode) string {
	if len(node.Children) < 2 {
		return "qv_null()"
	}

	// Left side is input value
	input := g.generateExpr(node.Children[0])

	// Right side is function or function call
	rightNode := node.Children[1]

	if rightNode.NodeType == ast.IdentifierNode {
		// Simple function name - call with input as argument
		funcName := rightNode.TokenLiteral()
		switch funcName {
		case "print":
			return fmt.Sprintf("q_print(%s)", input)
		case "println":
			return fmt.Sprintf("q_println(%s)", input)
		case "len":
			return fmt.Sprintf("q_len(%s)", input)
		case "str":
			return fmt.Sprintf("q_str(%s)", input)
		case "int":
			return fmt.Sprintf("q_int(%s)", input)
		case "float":
			return fmt.Sprintf("q_float(%s)", input)
		case "bool":
			return fmt.Sprintf("q_bool(%s)", input)
		// Math functions
		case "abs":
			return fmt.Sprintf("q_abs(%s)", input)
		case "sqrt":
			return fmt.Sprintf("q_sqrt(%s)", input)
		case "floor":
			return fmt.Sprintf("q_floor(%s)", input)
		case "ceil":
			return fmt.Sprintf("q_ceil(%s)", input)
		case "round":
			return fmt.Sprintf("q_round(%s)", input)
		// String functions
		case "upper":
			return fmt.Sprintf("q_upper(%s)", input)
		case "lower":
			return fmt.Sprintf("q_lower(%s)", input)
		case "trim":
			return fmt.Sprintf("q_trim(%s)", input)
		default:
			return fmt.Sprintf("quark_%s(%s)", funcName, input)
		}
	} else if rightNode.NodeType == ast.FunctionCallNode {
		// Function call - prepend input to arguments
		if len(rightNode.Children) >= 2 {
			funcNode := rightNode.Children[0]
			argsNode := rightNode.Children[1]

			funcName := funcNode.TokenLiteral()
			args := []string{input}
			for _, arg := range argsNode.Children {
				args = append(args, g.generateExpr(arg))
			}

			switch funcName {
			case "print":
				return fmt.Sprintf("q_print(%s)", args[0])
			case "println":
				return fmt.Sprintf("q_println(%s)", args[0])
			case "len":
				return fmt.Sprintf("q_len(%s)", args[0])
			case "str":
				return fmt.Sprintf("q_str(%s)", args[0])
			case "int":
				return fmt.Sprintf("q_int(%s)", args[0])
			case "float":
				return fmt.Sprintf("q_float(%s)", args[0])
			case "bool":
				return fmt.Sprintf("q_bool(%s)", args[0])
			case "range":
				if len(args) == 1 {
					return fmt.Sprintf("q_range(%s)", args[0])
				} else if len(args) == 2 {
					return fmt.Sprintf("q_range(%s, %s)", args[0], args[1])
				} else if len(args) >= 3 {
					return fmt.Sprintf("q_range(%s, %s, %s)", args[0], args[1], args[2])
				}
				return "qv_list()"
			// Math functions
			case "abs":
				return fmt.Sprintf("q_abs(%s)", args[0])
			case "min":
				if len(args) >= 2 {
					return fmt.Sprintf("q_min(%s, %s)", args[0], args[1])
				}
				return "qv_int(0)"
			case "max":
				if len(args) >= 2 {
					return fmt.Sprintf("q_max(%s, %s)", args[0], args[1])
				}
				return "qv_int(0)"
			case "sqrt":
				return fmt.Sprintf("q_sqrt(%s)", args[0])
			case "floor":
				return fmt.Sprintf("q_floor(%s)", args[0])
			case "ceil":
				return fmt.Sprintf("q_ceil(%s)", args[0])
			case "round":
				return fmt.Sprintf("q_round(%s)", args[0])
			// String functions
			case "upper":
				return fmt.Sprintf("q_upper(%s)", args[0])
			case "lower":
				return fmt.Sprintf("q_lower(%s)", args[0])
			case "trim":
				return fmt.Sprintf("q_trim(%s)", args[0])
			case "contains":
				if len(args) >= 2 {
					return fmt.Sprintf("q_contains(%s, %s)", args[0], args[1])
				}
				return "qv_bool(false)"
			case "startswith":
				if len(args) >= 2 {
					return fmt.Sprintf("q_startswith(%s, %s)", args[0], args[1])
				}
				return "qv_bool(false)"
			case "endswith":
				if len(args) >= 2 {
					return fmt.Sprintf("q_endswith(%s, %s)", args[0], args[1])
				}
				return "qv_bool(false)"
			case "replace":
				if len(args) >= 3 {
					return fmt.Sprintf("q_replace(%s, %s, %s)", args[0], args[1], args[2])
				}
				return "qv_string(\"\")"
			case "concat":
				if len(args) >= 2 {
					return fmt.Sprintf("q_concat(%s, %s)", args[0], args[1])
				}
				return "qv_string(\"\")"
			default:
				return fmt.Sprintf("quark_%s(%s)", funcName, strings.Join(args, ", "))
			}
		}
	}

	return g.generateExpr(rightNode)
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
		for j := 0; j < resultIdx; j++ {
			patternExpr := pattern.Children[j]
			if patternExpr.NodeType == ast.IdentifierNode && patternExpr.TokenLiteral() == "_" {
				// Wildcard matches everything
				conditions = append(conditions, "true")
			} else {
				patternVal := g.generateExpr(patternExpr)
				conditions = append(conditions, fmt.Sprintf("q_eq(%s, %s).data.bool_val", matchTemp, patternVal))
			}
		}

		condStr := strings.Join(conditions, " || ")
		if first {
			g.emitLine("if (%s) {", condStr)
			first = false
		} else {
			g.emit(g.indent() + "} else if (%s) {\n", condStr)
		}
		g.indentLevel++
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

	// Handle list iteration (for item in mylist or for i in range(10))
	listExpr := g.generateExpr(rangeNode)
	listTemp := g.newTemp()
	lenTemp := g.newTemp()
	idxTemp := g.newTemp()

	g.emitLine("QValue %s = %s;", listTemp, listExpr)
	g.emitLine("long long %s = (long long)%s.data.list_val->size();", lenTemp, listTemp)
	g.emitLine("for (long long %s = 0; %s < %s; %s++) {", idxTemp, idxTemp, lenTemp, idxTemp)
	g.indentLevel++

	// Copy parent scope and add loop variable
	oldDeclaredVars := g.declaredVars
	g.declaredVars = make(map[string]bool)
	for k, v := range oldDeclaredVars {
		g.declaredVars[k] = v
	}
	g.declaredVars[varName] = true // Loop variable is declared
	g.emitLine("QValue %s = q_get(%s, qv_int(%s));", varName, listTemp, idxTemp)

	// Generate body - emit each statement
	if bodyNode.NodeType == ast.BlockNode {
		for _, stmt := range bodyNode.Children {
			expr := g.generateExpr(stmt)
			g.emitLine("%s;", expr)
		}
	} else {
		expr := g.generateExpr(bodyNode)
		g.emitLine("%s;", expr)
	}

	// Restore parent scope
	g.declaredVars = oldDeclaredVars

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

func (g *Generator) generateIndex(node *ast.TreeNode) string {
	if len(node.Children) < 2 {
		return "qv_null()"
	}

	target := g.generateExpr(node.Children[0])
	index := g.generateExpr(node.Children[1])

	return fmt.Sprintf("q_get(%s, %s)", target, index)
}

func (g *Generator) generateLambdaExpr(node *ast.TreeNode) string {
	// Look up the lambda name we assigned during collection
	lambdaName, ok := g.lambdaNames[node]
	if !ok {
		// Lambda wasn't collected - this shouldn't happen but handle it
		lambdaName = g.newLambda()
		g.lambdaNames[node] = lambdaName
		g.lambdas = append(g.lambdas, node)
		g.functions = append(g.functions, lambdaName)
	}

	// Return a function value wrapping the lambda
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

	// Build parameter list and mark parameters as declared
	params := make([]string, 0)
	for _, param := range argsNode.Children {
		paramName := param.TokenLiteral()
		params = append(params, fmt.Sprintf("QValue %s", paramName))
		g.declaredVars[paramName] = true // Parameters are already declared
	}

	g.emit("QValue quark_%s(%s) {\n", lambdaName, strings.Join(params, ", "))
	g.indentLevel++

	// Generate body - for lambdas, the body is a single expression
	result := g.generateExpr(bodyNode)
	g.emitLine("return %s;", result)

	g.indentLevel--
	g.emit("}\n\n")

	g.popScope() // Restore previous scope
	g.inFunction = false
}
