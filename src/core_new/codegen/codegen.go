package codegen

import (
	"fmt"
	"quark/ast"
	"quark/token"
	"strings"
)

// Generator generates C code from an AST
type Generator struct {
	output       strings.Builder
	indentLevel  int
	functions    []string // Function definitions (generated separately)
	tempCounter  int
	inFunction   bool
	currentFunc  string
}

func New() *Generator {
	return &Generator{
		functions:   make([]string, 0),
		tempCounter: 0,
	}
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

// Generate produces C code from the AST
func (g *Generator) Generate(node *ast.TreeNode) string {
	// Generate header
	g.emit(`#include <stdio.h>
#include <stdlib.h>
#include <string.h>
#include <stdbool.h>

// Quark runtime types
typedef struct {
    enum { VAL_INT, VAL_FLOAT, VAL_STRING, VAL_BOOL, VAL_NULL, VAL_LIST, VAL_FUNC } type;
    union {
        long long int_val;
        double float_val;
        char* string_val;
        bool bool_val;
        struct { void** items; int len; int cap; } list_val;
        void* func_val;
    } data;
} QValue;

// Runtime functions
QValue qv_int(long long v) { QValue q; q.type = VAL_INT; q.data.int_val = v; return q; }
QValue qv_float(double v) { QValue q; q.type = VAL_FLOAT; q.data.float_val = v; return q; }
QValue qv_string(const char* v) { QValue q; q.type = VAL_STRING; q.data.string_val = strdup(v); return q; }
QValue qv_bool(bool v) { QValue q; q.type = VAL_BOOL; q.data.bool_val = v; return q; }
QValue qv_null() { QValue q; q.type = VAL_NULL; return q; }

void print_qvalue(QValue v) {
    switch (v.type) {
        case VAL_INT: printf("%%lld", v.data.int_val); break;
        case VAL_FLOAT: printf("%%g", v.data.float_val); break;
        case VAL_STRING: printf("%%s", v.data.string_val); break;
        case VAL_BOOL: printf(v.data.bool_val ? "true" : "false"); break;
        case VAL_NULL: printf("null"); break;
        default: printf("<value>"); break;
    }
}

QValue q_print(QValue v) { print_qvalue(v); return qv_null(); }
QValue q_println(QValue v) { print_qvalue(v); printf("\n"); return qv_null(); }

// Arithmetic operations
QValue q_add(QValue a, QValue b) {
    if (a.type == VAL_FLOAT || b.type == VAL_FLOAT) {
        double av = a.type == VAL_FLOAT ? a.data.float_val : (double)a.data.int_val;
        double bv = b.type == VAL_FLOAT ? b.data.float_val : (double)b.data.int_val;
        return qv_float(av + bv);
    }
    return qv_int(a.data.int_val + b.data.int_val);
}

QValue q_sub(QValue a, QValue b) {
    if (a.type == VAL_FLOAT || b.type == VAL_FLOAT) {
        double av = a.type == VAL_FLOAT ? a.data.float_val : (double)a.data.int_val;
        double bv = b.type == VAL_FLOAT ? b.data.float_val : (double)b.data.int_val;
        return qv_float(av - bv);
    }
    return qv_int(a.data.int_val - b.data.int_val);
}

QValue q_mul(QValue a, QValue b) {
    if (a.type == VAL_FLOAT || b.type == VAL_FLOAT) {
        double av = a.type == VAL_FLOAT ? a.data.float_val : (double)a.data.int_val;
        double bv = b.type == VAL_FLOAT ? b.data.float_val : (double)b.data.int_val;
        return qv_float(av * bv);
    }
    return qv_int(a.data.int_val * b.data.int_val);
}

QValue q_div(QValue a, QValue b) {
    double av = a.type == VAL_FLOAT ? a.data.float_val : (double)a.data.int_val;
    double bv = b.type == VAL_FLOAT ? b.data.float_val : (double)b.data.int_val;
    return qv_float(av / bv);
}

QValue q_mod(QValue a, QValue b) {
    return qv_int(a.data.int_val %% b.data.int_val);
}

QValue q_pow(QValue a, QValue b) {
    double av = a.type == VAL_FLOAT ? a.data.float_val : (double)a.data.int_val;
    double bv = b.type == VAL_FLOAT ? b.data.float_val : (double)b.data.int_val;
    double result = 1;
    for (int i = 0; i < (int)bv; i++) result *= av;
    return a.type == VAL_FLOAT || b.type == VAL_FLOAT ? qv_float(result) : qv_int((long long)result);
}

QValue q_neg(QValue a) {
    if (a.type == VAL_FLOAT) return qv_float(-a.data.float_val);
    return qv_int(-a.data.int_val);
}

// Comparison operations
QValue q_lt(QValue a, QValue b) {
    if (a.type == VAL_FLOAT || b.type == VAL_FLOAT) {
        double av = a.type == VAL_FLOAT ? a.data.float_val : (double)a.data.int_val;
        double bv = b.type == VAL_FLOAT ? b.data.float_val : (double)b.data.int_val;
        return qv_bool(av < bv);
    }
    return qv_bool(a.data.int_val < b.data.int_val);
}

QValue q_lte(QValue a, QValue b) {
    if (a.type == VAL_FLOAT || b.type == VAL_FLOAT) {
        double av = a.type == VAL_FLOAT ? a.data.float_val : (double)a.data.int_val;
        double bv = b.type == VAL_FLOAT ? b.data.float_val : (double)b.data.int_val;
        return qv_bool(av <= bv);
    }
    return qv_bool(a.data.int_val <= b.data.int_val);
}

QValue q_gt(QValue a, QValue b) {
    if (a.type == VAL_FLOAT || b.type == VAL_FLOAT) {
        double av = a.type == VAL_FLOAT ? a.data.float_val : (double)a.data.int_val;
        double bv = b.type == VAL_FLOAT ? b.data.float_val : (double)b.data.int_val;
        return qv_bool(av > bv);
    }
    return qv_bool(a.data.int_val > b.data.int_val);
}

QValue q_gte(QValue a, QValue b) {
    if (a.type == VAL_FLOAT || b.type == VAL_FLOAT) {
        double av = a.type == VAL_FLOAT ? a.data.float_val : (double)a.data.int_val;
        double bv = b.type == VAL_FLOAT ? b.data.float_val : (double)b.data.int_val;
        return qv_bool(av >= bv);
    }
    return qv_bool(a.data.int_val >= b.data.int_val);
}

QValue q_eq(QValue a, QValue b) {
    if (a.type != b.type) return qv_bool(false);
    switch (a.type) {
        case VAL_INT: return qv_bool(a.data.int_val == b.data.int_val);
        case VAL_FLOAT: return qv_bool(a.data.float_val == b.data.float_val);
        case VAL_BOOL: return qv_bool(a.data.bool_val == b.data.bool_val);
        case VAL_STRING: return qv_bool(strcmp(a.data.string_val, b.data.string_val) == 0);
        case VAL_NULL: return qv_bool(true);
        default: return qv_bool(false);
    }
}

QValue q_neq(QValue a, QValue b) {
    return qv_bool(!q_eq(a, b).data.bool_val);
}

// Logical operations
QValue q_and(QValue a, QValue b) {
    bool av = a.type == VAL_BOOL ? a.data.bool_val : (a.type == VAL_INT ? a.data.int_val != 0 : true);
    bool bv = b.type == VAL_BOOL ? b.data.bool_val : (b.type == VAL_INT ? b.data.int_val != 0 : true);
    return qv_bool(av && bv);
}

QValue q_or(QValue a, QValue b) {
    bool av = a.type == VAL_BOOL ? a.data.bool_val : (a.type == VAL_INT ? a.data.int_val != 0 : true);
    bool bv = b.type == VAL_BOOL ? b.data.bool_val : (b.type == VAL_INT ? b.data.int_val != 0 : true);
    return qv_bool(av || bv);
}

QValue q_not(QValue a) {
    bool av = a.type == VAL_BOOL ? a.data.bool_val : (a.type == VAL_INT ? a.data.int_val != 0 : true);
    return qv_bool(!av);
}

// Truthiness check
bool q_truthy(QValue v) {
    switch (v.type) {
        case VAL_BOOL: return v.data.bool_val;
        case VAL_INT: return v.data.int_val != 0;
        case VAL_FLOAT: return v.data.float_val != 0.0;
        case VAL_STRING: return v.data.string_val != NULL && strlen(v.data.string_val) > 0;
        case VAL_NULL: return false;
        default: return true;
    }
}

// Forward declarations
`)

	// First pass: collect function declarations
	g.collectFunctions(node)

	// Emit forward declarations
	for _, fname := range g.functions {
		g.emitLine("QValue q_%s();", fname)
	}
	g.emit("\n")

	// Generate function definitions
	g.generateNode(node)

	// Generate main function
	g.emit("\nint main() {\n")
	g.indentLevel++

	// Generate top-level statements that aren't function definitions
	for _, child := range node.Children {
		if child.NodeType != ast.FunctionNode {
			g.emitLine("%s;", g.generateExpr(child))
		}
	}

	g.emitLine("return 0;")
	g.indentLevel--
	g.emit("}\n")

	return g.output.String()
}

func (g *Generator) collectFunctions(node *ast.TreeNode) {
	if node.NodeType == ast.FunctionNode && len(node.Children) >= 1 {
		name := node.Children[0].TokenLiteral()
		g.functions = append(g.functions, name)
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
	case ast.FunctionNode:
		g.generateFunction(node)
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

	// Build parameter list
	params := make([]string, 0)
	for _, param := range argsNode.Children {
		params = append(params, fmt.Sprintf("QValue %s", param.TokenLiteral()))
	}

	g.emit("QValue q_%s(%s) {\n", funcName, strings.Join(params, ", "))
	g.indentLevel++

	// Generate body
	result := g.generateBlock(bodyNode)
	g.emitLine("return %s;", result)

	g.indentLevel--
	g.emit("}\n\n")

	g.inFunction = false
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
	case ast.BlockNode:
		return g.generateBlock(node)
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
		g.emitLine("QValue %s = %s;", varName, right)
		return varName
	case token.DOTDOT:
		// Range - used in for loops, not directly as a value
		return fmt.Sprintf("/* range %s..%s */", left, right)
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
	}

	// User-defined functions
	return fmt.Sprintf("q_%s(%s)", funcName, strings.Join(args, ", "))
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
		default:
			return fmt.Sprintf("q_%s(%s)", funcName, input)
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
			default:
				return fmt.Sprintf("q_%s(%s)", funcName, strings.Join(args, ", "))
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

	// Handle range expression
	if rangeNode.NodeType == ast.OperatorNode && rangeNode.Token != nil && rangeNode.Token.Type == token.DOTDOT {
		startExpr := g.generateExpr(rangeNode.Children[0])
		endExpr := g.generateExpr(rangeNode.Children[1])

		startTemp := g.newTemp()
		endTemp := g.newTemp()

		g.emitLine("long long %s = %s.data.int_val;", startTemp, startExpr)
		g.emitLine("long long %s = %s.data.int_val;", endTemp, endExpr)
		g.emitLine("for (long long _i = %s; _i < %s; _i++) {", startTemp, endTemp)
		g.indentLevel++
		g.emitLine("QValue %s = qv_int(_i);", varName)

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

		g.indentLevel--
		g.emitLine("}")
	}

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
	// For now, lists are not fully implemented
	// This would require dynamic arrays
	return "qv_null()"
}
