package parser_test

import (
	"testing"

	"quark/ast"
	"quark/internal/testutil"
	"quark/token"
)

func TestPrecedence_MultiplicationBindsTighterThanAddition(t *testing.T) {
	node, errs := testutil.Parse("1 + 2 * 3\n")
	if len(errs) > 0 {
		t.Fatalf("unexpected parse errors: %v", errs)
	}
	if len(node.Children) != 1 {
		t.Fatalf("expected 1 top-level statement, got %d", len(node.Children))
	}
	expr := node.Children[0]
	if expr.NodeType != ast.OperatorNode || expr.Token == nil || expr.Token.Type != token.PLUS {
		t.Fatalf("expected top operator PLUS, got %v", expr)
	}
	if len(expr.Children) != 2 {
		t.Fatalf("expected binary op children, got %d", len(expr.Children))
	}
	right := expr.Children[1]
	if right.NodeType != ast.OperatorNode || right.Token == nil || right.Token.Type != token.MULTIPLY {
		t.Fatalf("expected right operator MULTIPLY, got %v", right)
	}
}

func TestPrecedence_ExponentIsRightAssociative(t *testing.T) {
	node, errs := testutil.Parse("2 ** 3 ** 2\n")
	if len(errs) > 0 {
		t.Fatalf("unexpected parse errors: %v", errs)
	}
	expr := node.Children[0]
	if expr.NodeType != ast.OperatorNode || expr.Token == nil || expr.Token.Type != token.DOUBLESTAR {
		t.Fatalf("expected top operator DOUBLESTAR, got %v", expr)
	}
	right := expr.Children[1]
	if right.NodeType != ast.OperatorNode || right.Token == nil || right.Token.Type != token.DOUBLESTAR {
		t.Fatalf("expected right operator DOUBLESTAR, got %v", right)
	}
}

func TestTernary_ParseOrder(t *testing.T) {
	node, errs := testutil.Parse("'a' if true else 'b'\n")
	if len(errs) > 0 {
		t.Fatalf("unexpected parse errors: %v", errs)
	}
	expr := node.Children[0]
	if expr.NodeType != ast.TernaryNode {
		t.Fatalf("expected TernaryNode, got %v", expr)
	}
	if len(expr.Children) != 3 {
		t.Fatalf("expected ternary children=3, got %d", len(expr.Children))
	}
	// Parser stores: condition, trueValue, falseValue
	if expr.Children[0].NodeType != ast.LiteralNode || expr.Children[0].Token == nil || expr.Children[0].Token.Type != token.TRUE {
		t.Fatalf("expected condition TRUE literal, got %v", expr.Children[0])
	}
}

func TestVectorLiteral_ParseNode(t *testing.T) {
	node, errs := testutil.Parse("v = vector [1, 2, 3]\n")
	if len(errs) > 0 {
		t.Fatalf("unexpected parse errors: %v", errs)
	}
	if len(node.Children) != 1 {
		t.Fatalf("expected 1 top-level node, got %d", len(node.Children))
	}
	assign := node.Children[0]
	if assign.NodeType != ast.OperatorNode || assign.Token == nil || assign.Token.Type != token.EQUALS {
		t.Fatalf("expected assignment node, got %v", assign)
	}
	if len(assign.Children) != 2 {
		t.Fatalf("expected assignment children=2, got %d", len(assign.Children))
	}
	vec := assign.Children[1]
	if vec.NodeType != ast.VectorNode {
		t.Fatalf("expected VectorNode RHS, got %v", vec)
	}
	if len(vec.Children) != 3 {
		t.Fatalf("expected vector literal length 3, got %d", len(vec.Children))
	}
}

func TestTypedVectorDecl_Parse(t *testing.T) {
	node, errs := testutil.Parse("v: vector = vector [1, 2]\n")
	if len(errs) > 0 {
		t.Fatalf("unexpected parse errors: %v", errs)
	}
	if len(node.Children) != 1 || node.Children[0].NodeType != ast.VarDeclNode {
		t.Fatalf("expected VarDeclNode, got %v", node.Children)
	}
}

func TestVectorLiteral_RejectsSemicolonRows(t *testing.T) {
	_, errs := testutil.Parse("vector [1, 2; 3, 4]\n")
	if len(errs) == 0 {
		t.Fatalf("expected parse error for semicolon row separators in vector literal")
	}
}
