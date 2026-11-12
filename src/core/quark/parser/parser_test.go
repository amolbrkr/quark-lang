package parser_test

import (
	"strings"
	"testing"

	"quark/ast"
	"quark/internal/testutil"
	"quark/token"
)

// --- Operator precedence ---

func TestPrecedence_MultiplicationBindsTighterThanAddition(t *testing.T) {
	node, errs := testutil.Parse("1 + 2 * 3\n")
	if len(errs) > 0 {
		t.Fatalf("unexpected parse errors: %v", errs)
	}
	if len(node.Children) != 1 {
		t.Fatalf("expected 1 top-level statement, got %d", len(node.Children))
	}
	expr := node.Children[0]
	if expr.NodeType != ast.BinaryNode || expr.Token == nil || expr.Token.Type != token.PLUS {
		t.Fatalf("expected top operator PLUS, got %v", expr)
	}
	if len(expr.Children) != 2 {
		t.Fatalf("expected binary op children, got %d", len(expr.Children))
	}
	right := expr.Children[1]
	if right.NodeType != ast.BinaryNode || right.Token == nil || right.Token.Type != token.MULTIPLY {
		t.Fatalf("expected right operator MULTIPLY, got %v", right)
	}
}

func TestPrecedence_ExponentIsRightAssociative(t *testing.T) {
	node, errs := testutil.Parse("2 ** 3 ** 2\n")
	if len(errs) > 0 {
		t.Fatalf("unexpected parse errors: %v", errs)
	}
	expr := node.Children[0]
	if expr.NodeType != ast.BinaryNode || expr.Token == nil || expr.Token.Type != token.DOUBLESTAR {
		t.Fatalf("expected top operator DOUBLESTAR, got %v", expr)
	}
	right := expr.Children[1]
	if right.NodeType != ast.BinaryNode || right.Token == nil || right.Token.Type != token.DOUBLESTAR {
		t.Fatalf("expected right operator DOUBLESTAR, got %v", right)
	}
}

func TestPrecedence_ComparisonLowerThanArithmetic(t *testing.T) {
	// 1 + 2 < 3 * 4 should parse as (1+2) < (3*4)
	node, errs := testutil.Parse("1 + 2 < 3 * 4\n")
	if len(errs) > 0 {
		t.Fatalf("unexpected parse errors: %v", errs)
	}
	expr := node.Children[0]
	if expr.Token == nil || expr.Token.Type != token.LT {
		t.Fatalf("expected top operator LT, got %v", expr)
	}
	left := expr.Children[0]
	right := expr.Children[1]
	if left.Token == nil || left.Token.Type != token.PLUS {
		t.Fatalf("expected left child PLUS, got %v", left)
	}
	if right.Token == nil || right.Token.Type != token.MULTIPLY {
		t.Fatalf("expected right child MULTIPLY, got %v", right)
	}
}

func TestPrecedence_LogicalAndLowerThanComparison(t *testing.T) {
	// x > 0 and y > 0 should parse as (x>0) and (y>0)
	node, errs := testutil.Parse("x > 0 and y > 0\n")
	if len(errs) > 0 {
		t.Fatalf("unexpected parse errors: %v", errs)
	}
	expr := node.Children[0]
	if expr.Token == nil || expr.Token.Type != token.AND {
		t.Fatalf("expected top operator AND, got %v", expr)
	}
}

func TestPrecedence_LogicalOrLowerThanAnd(t *testing.T) {
	// a or b and c should parse as a or (b and c)
	node, errs := testutil.Parse("a or b and c\n")
	if len(errs) > 0 {
		t.Fatalf("unexpected parse errors: %v", errs)
	}
	expr := node.Children[0]
	if expr.Token == nil || expr.Token.Type != token.OR {
		t.Fatalf("expected top operator OR, got %v", expr)
	}
	right := expr.Children[1]
	if right.Token == nil || right.Token.Type != token.AND {
		t.Fatalf("expected right child AND, got %v", right)
	}
}

func TestPrecedence_DotBindsTighterThanArithmetic(t *testing.T) {
	// x.y + 1 should parse as (x.y) + 1
	node, errs := testutil.Parse("x.y + 1\n")
	if len(errs) > 0 {
		t.Fatalf("unexpected parse errors: %v", errs)
	}
	expr := node.Children[0]
	if expr.Token == nil || expr.Token.Type != token.PLUS {
		t.Fatalf("expected top operator PLUS, got %v", expr)
	}
	left := expr.Children[0]
	if left.Token == nil || left.Token.Type != token.DOT {
		t.Fatalf("expected left child DOT, got %v", left)
	}
}

func TestPrecedence_UnaryMinusBindsTighterThanBinary(t *testing.T) {
	// -x + 1 should parse as (-x) + 1
	node, errs := testutil.Parse("-x + 1\n")
	if len(errs) > 0 {
		t.Fatalf("unexpected parse errors: %v", errs)
	}
	expr := node.Children[0]
	if expr.Token == nil || expr.Token.Type != token.PLUS {
		t.Fatalf("expected top operator PLUS, got %v", expr)
	}
	left := expr.Children[0]
	if left.Token == nil || left.Token.Type != token.MINUS {
		t.Fatalf("expected left child unary MINUS, got %v", left)
	}
	if len(left.Children) != 1 {
		t.Fatalf("expected unary MINUS to have 1 child, got %d", len(left.Children))
	}
}

func TestPrecedence_ModuloSameAsDivide(t *testing.T) {
	// 10 % 3 + 1 should parse as (10%3) + 1
	node, errs := testutil.Parse("10 % 3 + 1\n")
	if len(errs) > 0 {
		t.Fatalf("unexpected parse errors: %v", errs)
	}
	expr := node.Children[0]
	if expr.Token == nil || expr.Token.Type != token.PLUS {
		t.Fatalf("expected top operator PLUS, got %v", expr)
	}
	left := expr.Children[0]
	if left.Token == nil || left.Token.Type != token.MODULO {
		t.Fatalf("expected left child MODULO, got %v", left)
	}
}

// --- Ternary ---

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

// --- Literals ---

func TestLiteral_IntegerNode(t *testing.T) {
	node, errs := testutil.Parse("42\n")
	if len(errs) > 0 {
		t.Fatalf("unexpected parse errors: %v", errs)
	}
	expr := node.Children[0]
	if expr.NodeType != ast.LiteralNode || expr.Token == nil || expr.Token.Type != token.INT {
		t.Fatalf("expected INT literal, got %v", expr)
	}
	if expr.Token.Literal != "42" {
		t.Fatalf("expected literal '42', got %q", expr.Token.Literal)
	}
}

func TestLiteral_FloatNode(t *testing.T) {
	node, errs := testutil.Parse("3.14\n")
	if len(errs) > 0 {
		t.Fatalf("unexpected parse errors: %v", errs)
	}
	expr := node.Children[0]
	if expr.NodeType != ast.LiteralNode || expr.Token == nil || expr.Token.Type != token.FLOAT {
		t.Fatalf("expected FLOAT literal, got %v", expr)
	}
}

func TestLiteral_BooleanTrue(t *testing.T) {
	node, errs := testutil.Parse("true\n")
	if len(errs) > 0 {
		t.Fatalf("unexpected parse errors: %v", errs)
	}
	expr := node.Children[0]
	if expr.NodeType != ast.LiteralNode || expr.Token == nil || expr.Token.Type != token.TRUE {
		t.Fatalf("expected TRUE literal, got %v", expr)
	}
}

func TestLiteral_BooleanFalse(t *testing.T) {
	node, errs := testutil.Parse("false\n")
	if len(errs) > 0 {
		t.Fatalf("unexpected parse errors: %v", errs)
	}
	expr := node.Children[0]
	if expr.NodeType != ast.LiteralNode || expr.Token == nil || expr.Token.Type != token.FALSE {
		t.Fatalf("expected FALSE literal, got %v", expr)
	}
}

func TestLiteral_Null(t *testing.T) {
	node, errs := testutil.Parse("null\n")
	if len(errs) > 0 {
		t.Fatalf("unexpected parse errors: %v", errs)
	}
	expr := node.Children[0]
	if expr.NodeType != ast.LiteralNode || expr.Token == nil || expr.Token.Type != token.NULL {
		t.Fatalf("expected NULL literal, got %v", expr)
	}
}

func TestLiteral_StringNode(t *testing.T) {
	node, errs := testutil.Parse("'hello'\n")
	if len(errs) > 0 {
		t.Fatalf("unexpected parse errors: %v", errs)
	}
	expr := node.Children[0]
	if expr.NodeType != ast.LiteralNode || expr.Token == nil || expr.Token.Type != token.STRING {
		t.Fatalf("expected STRING literal, got %v", expr)
	}
}

// --- Collection literals ---

func TestListLiteral_Basic(t *testing.T) {
	node, errs := testutil.Parse("list [1, 2, 3]\n")
	if len(errs) > 0 {
		t.Fatalf("unexpected parse errors: %v", errs)
	}
	expr := node.Children[0]
	if expr.NodeType != ast.ListNode {
		t.Fatalf("expected ListNode, got %v", expr)
	}
	if len(expr.Children) != 3 {
		t.Fatalf("expected 3 elements, got %d", len(expr.Children))
	}
}

func TestListLiteral_Empty(t *testing.T) {
	node, errs := testutil.Parse("list []\n")
	if len(errs) > 0 {
		t.Fatalf("unexpected parse errors: %v", errs)
	}
	expr := node.Children[0]
	if expr.NodeType != ast.ListNode {
		t.Fatalf("expected ListNode, got %v", expr)
	}
	if len(expr.Children) != 0 {
		t.Fatalf("expected 0 elements, got %d", len(expr.Children))
	}
}

func TestListLiteral_TrailingComma(t *testing.T) {
	node, errs := testutil.Parse("list [1, 2,]\n")
	if len(errs) > 0 {
		t.Fatalf("unexpected parse errors: %v", errs)
	}
	expr := node.Children[0]
	if expr.NodeType != ast.ListNode {
		t.Fatalf("expected ListNode, got %v", expr)
	}
	if len(expr.Children) != 2 {
		t.Fatalf("expected 2 elements with trailing comma, got %d", len(expr.Children))
	}
}

func TestVectorLiteral_ParseNode(t *testing.T) {
	node, errs := testutil.Parse("v = vec [1, 2, 3]\n")
	if len(errs) > 0 {
		t.Fatalf("unexpected parse errors: %v", errs)
	}
	if len(node.Children) != 1 {
		t.Fatalf("expected 1 top-level node, got %d", len(node.Children))
	}
	assign := node.Children[0]
	if assign.NodeType != ast.AssignNode || assign.Token == nil || assign.Token.Type != token.EQUALS {
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

func TestVectorLiteral_RejectsSemicolonRows(t *testing.T) {
	_, errs := testutil.Parse("vec [1, 2; 3, 4]\n")
	if len(errs) == 0 {
		t.Fatalf("expected parse error for semicolon row separators in vector literal")
	}
}

func TestDictLiteral_Basic(t *testing.T) {
	node, errs := testutil.Parse("dict { a: 1, b: 2 }\n")
	if len(errs) > 0 {
		t.Fatalf("unexpected parse errors: %v", errs)
	}
	expr := node.Children[0]
	if expr.NodeType != ast.DictNode {
		t.Fatalf("expected DictNode, got %v", expr)
	}
	if len(expr.Children) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(expr.Children))
	}
}

func TestDictLiteral_Empty(t *testing.T) {
	node, errs := testutil.Parse("dict {}\n")
	if len(errs) > 0 {
		t.Fatalf("unexpected parse errors: %v", errs)
	}
	expr := node.Children[0]
	if expr.NodeType != ast.DictNode {
		t.Fatalf("expected DictNode, got %v", expr)
	}
	if len(expr.Children) != 0 {
		t.Fatalf("expected 0 entries, got %d", len(expr.Children))
	}
}

// --- Functions ---

func TestFunction_BasicNamedFunction(t *testing.T) {
	// A named function is a FnDeclNode [name, lambda].
	node, errs := testutil.Parse("fn add(x, y) -> x + y\n")
	if len(errs) > 0 {
		t.Fatalf("unexpected parse errors: %v", errs)
	}
	assign := node.Children[0]
	if !ast.IsFnDecl(assign) {
		t.Fatalf("expected a fn declaration, got %v", assign)
	}
	// LHS is the name, RHS is the lambda
	if assign.Children[0].TokenLiteral() != "add" {
		t.Fatalf("expected name 'add', got %s", assign.Children[0].TokenLiteral())
	}
	if assign.Children[1].NodeType != ast.LambdaNode {
		t.Fatalf("expected LambdaNode on RHS, got %v", assign.Children[1])
	}
}

func TestFunction_WithReturnTypeAnnotation(t *testing.T) {
	node, errs := testutil.Parse("fn id(x: any) any -> x\n")
	if len(errs) > 0 {
		t.Fatalf("unexpected parse errors: %v", errs)
	}
	if len(node.Children) != 1 {
		t.Fatalf("expected 1 top-level node, got %d", len(node.Children))
	}
}

func TestFunction_WithBlockBody(t *testing.T) {
	// Block bodies use -> not : — fn greet(name) -> print(name) is single-expr
	// Multi-statement block: fn greet(name) ->\n    print(name)\n
	node, errs := testutil.Parse("fn greet(name) ->\n    print(name)\n")
	if len(errs) > 0 {
		t.Fatalf("unexpected parse errors: %v", errs)
	}
	assign := node.Children[0]
	if !ast.IsFnDecl(assign) {
		t.Fatalf("expected a fn declaration, got %v", assign)
	}
	if assign.Children[1].NodeType != ast.LambdaNode {
		t.Fatalf("expected LambdaNode on RHS, got %v", assign.Children[1])
	}
}

func TestFunction_DefaultParameters(t *testing.T) {
	// A named function with defaults is a declaration too
	node, errs := testutil.Parse("fn add(x, y = 0) -> x + y\n")
	if len(errs) > 0 {
		t.Fatalf("unexpected parse errors: %v", errs)
	}
	assign := node.Children[0]
	if !ast.IsFnDecl(assign) {
		t.Fatalf("expected a fn declaration, got %v", assign)
	}
	lambda := assign.Children[1]
	if lambda.NodeType != ast.LambdaNode {
		t.Fatalf("expected LambdaNode, got %v", lambda)
	}
}

func TestLambda_InlineExpression(t *testing.T) {
	node, errs := testutil.Parse("f = fn(x) -> x * 2\n")
	if len(errs) > 0 {
		t.Fatalf("unexpected parse errors: %v", errs)
	}
	assign := node.Children[0]
	lambda := assign.Children[1]
	if lambda.NodeType != ast.LambdaNode {
		t.Fatalf("expected LambdaNode, got %v", lambda)
	}
}

func TestLambda_NoParams(t *testing.T) {
	node, errs := testutil.Parse("f = fn() -> 42\n")
	if len(errs) > 0 {
		t.Fatalf("unexpected parse errors: %v", errs)
	}
	assign := node.Children[0]
	lambda := assign.Children[1]
	if lambda.NodeType != ast.LambdaNode {
		t.Fatalf("expected LambdaNode, got %v", lambda)
	}
}

// --- Control flow ---

func TestIf_BasicParse(t *testing.T) {
	node, errs := testutil.Parse("if x > 0:\n    print(x)\n")
	if len(errs) > 0 {
		t.Fatalf("unexpected parse errors: %v", errs)
	}
	stmt := node.Children[0]
	if stmt.NodeType != ast.IfStatementNode {
		t.Fatalf("expected IfStatementNode, got %v", stmt)
	}
}

func TestIfElse_Parse(t *testing.T) {
	node, errs := testutil.Parse("if x > 0:\n    print(x)\nelse:\n    print(0)\n")
	if len(errs) > 0 {
		t.Fatalf("unexpected parse errors: %v", errs)
	}
	stmt := node.Children[0]
	if stmt.NodeType != ast.IfStatementNode {
		t.Fatalf("expected IfStatementNode, got %v", stmt)
	}
}

func TestForLoop_Parse(t *testing.T) {
	node, errs := testutil.Parse("for x in range(10):\n    print(x)\n")
	if len(errs) > 0 {
		t.Fatalf("unexpected parse errors: %v", errs)
	}
	stmt := node.Children[0]
	if stmt.NodeType != ast.ForLoopNode {
		t.Fatalf("expected ForLoopNode, got %v", stmt)
	}
}

func TestWhileLoop_Parse(t *testing.T) {
	node, errs := testutil.Parse("while true:\n    print(1)\n")
	if len(errs) > 0 {
		t.Fatalf("unexpected parse errors: %v", errs)
	}
	stmt := node.Children[0]
	if stmt.NodeType != ast.WhileLoopNode {
		t.Fatalf("expected WhileLoopNode, got %v", stmt)
	}
}

func TestBreak_Parse(t *testing.T) {
	node, errs := testutil.Parse("while true:\n    break\n")
	if len(errs) > 0 {
		t.Fatalf("unexpected parse errors: %v", errs)
	}
	body := node.Children[0] // WhileLoopNode
	// Break should be inside the loop body
	found := findNodeType(body, ast.BreakNode)
	if found == nil {
		t.Fatalf("expected BreakNode inside while loop")
	}
}

func TestContinue_Parse(t *testing.T) {
	node, errs := testutil.Parse("while true:\n    continue\n")
	if len(errs) > 0 {
		t.Fatalf("unexpected parse errors: %v", errs)
	}
	found := findNodeType(node, ast.ContinueNode)
	if found == nil {
		t.Fatalf("expected ContinueNode inside while loop")
	}
}

// --- Pipe ---

func TestPipe_BasicParse(t *testing.T) {
	node, errs := testutil.Parse("1 | print()\n")
	if len(errs) > 0 {
		t.Fatalf("unexpected parse errors: %v", errs)
	}
	// A pipe is parsed into the call it stands for: print(1).
	call := node.Children[0]
	if call.NodeType != ast.FunctionCallNode || !call.Piped {
		t.Fatalf("expected a piped FunctionCallNode, got %v", call)
	}
	args := call.Children[1].Children
	if call.Children[0].TokenLiteral() != "print" || len(args) != 1 || args[0].TokenLiteral() != "1" {
		t.Fatalf("expected print(1), got callee %v and args %v", call.Children[0], args)
	}
}

func TestPipe_ChainedParse(t *testing.T) {
	node, errs := testutil.Parse("x | f(1) | g()\n")
	if len(errs) > 0 {
		t.Fatalf("unexpected parse errors: %v", errs)
	}
	// Left-associative: (x | f(1)) | g() is g(f(x, 1)).
	outer := node.Children[0]
	if outer.NodeType != ast.FunctionCallNode || outer.Children[0].TokenLiteral() != "g" {
		t.Fatalf("expected outer call g, got %v", outer)
	}
	inner := outer.Children[1].Children
	if len(inner) != 1 || inner[0].NodeType != ast.FunctionCallNode || inner[0].Children[0].TokenLiteral() != "f" {
		t.Fatalf("expected g's only argument to be the call f, got %v", inner)
	}
	fArgs := inner[0].Children[1].Children
	if len(fArgs) != 2 || fArgs[0].TokenLiteral() != "x" || fArgs[1].TokenLiteral() != "1" {
		t.Fatalf("expected f(x, 1), got args %v", fArgs)
	}
}

func TestPipe_MethodTarget(t *testing.T) {
	node, errs := testutil.Parse("x | s.replace('b')\n")
	if len(errs) > 0 {
		t.Fatalf("unexpected parse errors: %v", errs)
	}
	// The piped input is the method's first argument; s stays the receiver.
	call := node.Children[0]
	if call.NodeType != ast.FunctionCallNode || call.Children[0].NodeType != ast.MemberNode {
		t.Fatalf("expected a method call, got %v", call)
	}
	args := call.Children[1].Children
	if len(args) != 2 || args[0].TokenLiteral() != "x" {
		t.Fatalf("expected s.replace(x, 'b'), got args %v", args)
	}
}

func TestPipe_TargetMustBeCall(t *testing.T) {
	for _, src := range []string{"x | f\n", "x | 1 + 2\n", "x | f() + 1\n"} {
		_, errs := testutil.Parse(src)
		if len(errs) == 0 || !strings.Contains(strings.Join(errs, "\n"), "pipe target must be a function call") {
			t.Fatalf("%q: expected pipe target error, got %v", src, errs)
		}
	}
}

func TestPipe_InsideFunctionArg(t *testing.T) {
	node, errs := testutil.Parse("print(x | f())\n")
	if len(errs) > 0 {
		t.Fatalf("unexpected parse errors: %v", errs)
	}
	call := node.Children[0]
	if call.NodeType != ast.FunctionCallNode {
		t.Fatalf("expected FunctionCallNode, got %v", call)
	}
	args := call.Children[1]
	if len(args.Children) != 1 {
		t.Fatalf("expected 1 argument, got %d", len(args.Children))
	}
	arg := args.Children[0]
	if arg.NodeType != ast.FunctionCallNode || !arg.Piped {
		t.Fatalf("expected a piped call as argument, got %v", arg)
	}
}

// --- Assignment ---

func TestAssignment_Simple(t *testing.T) {
	node, errs := testutil.Parse("x = 42\n")
	if len(errs) > 0 {
		t.Fatalf("unexpected parse errors: %v", errs)
	}
	expr := node.Children[0]
	if expr.NodeType != ast.AssignNode || expr.Token == nil || expr.Token.Type != token.EQUALS {
		t.Fatalf("expected EQUALS assignment, got %v", expr)
	}
}

func TestTypedVarDecl_Parse(t *testing.T) {
	node, errs := testutil.Parse("v: vec = vec [1, 2]\n")
	if len(errs) > 0 {
		t.Fatalf("unexpected parse errors: %v", errs)
	}
	if len(node.Children) != 1 || node.Children[0].NodeType != ast.VarDeclNode {
		t.Fatalf("expected VarDeclNode, got %v", node.Children)
	}
}

// --- Member access / method call ---

func TestMemberAccess_DotParse(t *testing.T) {
	node, errs := testutil.Parse("x.y\n")
	if len(errs) > 0 {
		t.Fatalf("unexpected parse errors: %v", errs)
	}
	expr := node.Children[0]
	if expr.NodeType != ast.MemberNode || expr.Token == nil || expr.Token.Type != token.DOT {
		t.Fatalf("expected DOT operator, got %v", expr)
	}
	if len(expr.Children) != 2 {
		t.Fatalf("expected 2 children for dot access, got %d", len(expr.Children))
	}
}

func TestMethodCall_Parse(t *testing.T) {
	node, errs := testutil.Parse("'hello'.upper()\n")
	if len(errs) > 0 {
		t.Fatalf("unexpected parse errors: %v", errs)
	}
	expr := node.Children[0]
	if expr.NodeType != ast.FunctionCallNode {
		t.Fatalf("expected FunctionCallNode, got %v", expr)
	}
	funcNode := expr.Children[0]
	if funcNode.Token == nil || funcNode.Token.Type != token.DOT {
		t.Fatalf("expected DOT callee, got %v", funcNode)
	}
}

func TestMethodChain_Parse(t *testing.T) {
	node, errs := testutil.Parse("'hello'.upper().trim()\n")
	if len(errs) > 0 {
		t.Fatalf("unexpected parse errors: %v", errs)
	}
	// Should parse as (('hello'.upper()).trim())
	outer := node.Children[0]
	if outer.NodeType != ast.FunctionCallNode {
		t.Fatalf("expected outer FunctionCallNode, got %v", outer)
	}
}

// --- Index ---

func TestIndex_Parse(t *testing.T) {
	node, errs := testutil.Parse("x[0]\n")
	if len(errs) > 0 {
		t.Fatalf("unexpected parse errors: %v", errs)
	}
	expr := node.Children[0]
	if expr.NodeType != ast.IndexNode {
		t.Fatalf("expected IndexNode, got %v", expr)
	}
	if len(expr.Children) != 2 {
		t.Fatalf("expected 2 children (target, index), got %d", len(expr.Children))
	}
}

func TestIndex_ChainedWithMethodCall(t *testing.T) {
	// x[0].upper() should parse without errors
	node, errs := testutil.Parse("x[0].upper()\n")
	if len(errs) > 0 {
		t.Fatalf("unexpected parse errors: %v", errs)
	}
	expr := node.Children[0]
	if expr.NodeType != ast.FunctionCallNode {
		t.Fatalf("expected FunctionCallNode, got %v", expr)
	}
}

// --- Result ---

func TestResult_OkParse(t *testing.T) {
	node, errs := testutil.Parse("ok 1\n")
	if len(errs) > 0 {
		t.Fatalf("unexpected parse errors: %v", errs)
	}
	expr := node.Children[0]
	if expr.NodeType != ast.ResultNode {
		t.Fatalf("expected ResultNode, got %v", expr)
	}
}

func TestResult_ErrParse(t *testing.T) {
	node, errs := testutil.Parse("err 'boom'\n")
	if len(errs) > 0 {
		t.Fatalf("unexpected parse errors: %v", errs)
	}
	expr := node.Children[0]
	if expr.NodeType != ast.ResultNode {
		t.Fatalf("expected ResultNode, got %v", expr)
	}
}

// --- When ---

func TestWhen_BasicParse(t *testing.T) {
	node, errs := testutil.Parse("when x:\n    1 -> print('one')\n    _ -> print('other')\n")
	if len(errs) > 0 {
		t.Fatalf("unexpected parse errors: %v", errs)
	}
	stmt := node.Children[0]
	if stmt.NodeType != ast.WhenStatementNode {
		t.Fatalf("expected WhenStatementNode, got %v", stmt)
	}
}

// --- Use/Module ---

func TestInclAlias_Parse(t *testing.T) {
	node, errs := testutil.Parse("incl './lib/math' as math\n")
	if len(errs) > 0 {
		t.Fatalf("unexpected parse errors: %v", errs)
	}
	if len(node.Children) != 1 {
		t.Fatalf("expected one top-level node, got %d", len(node.Children))
	}
	inclNode := node.Children[0]
	if inclNode.NodeType != ast.InclNode {
		t.Fatalf("expected InclNode, got %v", inclNode)
	}
	if len(inclNode.Children) != 2 {
		t.Fatalf("expected incl node with import target and alias, got %d children", len(inclNode.Children))
	}
	if inclNode.Children[0].NodeType != ast.LiteralNode || inclNode.Children[0].Token == nil || inclNode.Children[0].Token.Type != token.STRING {
		t.Fatalf("expected string import path, got %v", inclNode.Children[0])
	}
	if inclNode.Children[1].NodeType != ast.IdentifierNode || inclNode.Children[1].TokenLiteral() != "math" {
		t.Fatalf("expected alias 'math', got %v", inclNode.Children[1])
	}
}

func TestModule_BasicParse(t *testing.T) {
	node, errs := testutil.Parse("module math:\n    fn add(x, y) -> x + y\n")
	if len(errs) > 0 {
		t.Fatalf("unexpected parse errors: %v", errs)
	}
	mod := node.Children[0]
	if mod.NodeType != ast.ModuleNode {
		t.Fatalf("expected ModuleNode, got %v", mod)
	}
}

// --- Grouped expressions ---

func TestGroupedExpression_Parens(t *testing.T) {
	// (1 + 2) * 3 should have MULTIPLY at top
	node, errs := testutil.Parse("(1 + 2) * 3\n")
	if len(errs) > 0 {
		t.Fatalf("unexpected parse errors: %v", errs)
	}
	expr := node.Children[0]
	if expr.Token == nil || expr.Token.Type != token.MULTIPLY {
		t.Fatalf("expected top MULTIPLY, got %v", expr)
	}
	left := expr.Children[0]
	if left.Token == nil || left.Token.Type != token.PLUS {
		t.Fatalf("expected grouped PLUS on left, got %v", left)
	}
}

// --- Error cases ---

func TestError_UnclosedParen(t *testing.T) {
	_, errs := testutil.Parse("f(1, 2\n")
	if len(errs) == 0 {
		t.Fatalf("expected parse error for unclosed paren")
	}
}

func TestError_UnclosedBracket(t *testing.T) {
	_, errs := testutil.Parse("list [1, 2\n")
	if len(errs) == 0 {
		t.Fatalf("expected parse error for unclosed bracket")
	}
}

func TestError_UnclosedBrace(t *testing.T) {
	_, errs := testutil.Parse("dict { a: 1\n")
	if len(errs) == 0 {
		t.Fatalf("expected parse error for unclosed brace")
	}
}

func TestDefaultParam_RejectsNegatedStringLiteral(t *testing.T) {
	_, errs := testutil.Parse("fn bad(x = -'oops') -> x\n")
	if len(errs) == 0 {
		t.Fatalf("expected parse error for invalid negated string default")
	}
}

func TestError_MissingArrowInLambda(t *testing.T) {
	_, errs := testutil.Parse("f = fn(x) x * 2\n")
	if len(errs) == 0 {
		t.Fatalf("expected parse error for missing -> in lambda")
	}
}

func TestError_MissingColonAfterIf(t *testing.T) {
	_, errs := testutil.Parse("if true\n    x = 1\n")
	if len(errs) == 0 {
		t.Fatalf("expected parse error for missing colon after if condition")
	}
}

func TestError_MissingDictValue(t *testing.T) {
	_, errs := testutil.Parse("dict { a: }\n")
	if len(errs) == 0 {
		t.Fatalf("expected parse error for missing dict value after colon")
	}
}

// --- Multiline constructs ---

func TestMultilineList_Parse(t *testing.T) {
	src := "list [\n    1,\n    2,\n    3\n]\n"
	node, errs := testutil.Parse(src)
	if len(errs) > 0 {
		t.Fatalf("unexpected parse errors: %v", errs)
	}
	expr := node.Children[0]
	if expr.NodeType != ast.ListNode {
		t.Fatalf("expected ListNode, got %v", expr)
	}
	if len(expr.Children) != 3 {
		t.Fatalf("expected 3 elements in multiline list, got %d", len(expr.Children))
	}
}

func TestMultilineDict_Parse(t *testing.T) {
	src := "dict {\n    a: 1,\n    b: 2\n}\n"
	node, errs := testutil.Parse(src)
	if len(errs) > 0 {
		t.Fatalf("unexpected parse errors: %v", errs)
	}
	expr := node.Children[0]
	if expr.NodeType != ast.DictNode {
		t.Fatalf("expected DictNode, got %v", expr)
	}
	if len(expr.Children) != 2 {
		t.Fatalf("expected 2 entries in multiline dict, got %d", len(expr.Children))
	}
}

// --- Unary ---

func TestUnary_Bang(t *testing.T) {
	node, errs := testutil.Parse("!true\n")
	if len(errs) > 0 {
		t.Fatalf("unexpected parse errors: %v", errs)
	}
	expr := node.Children[0]
	if expr.NodeType != ast.UnaryNode || expr.Token == nil || expr.Token.Type != token.BANG {
		t.Fatalf("expected BANG operator, got %v", expr)
	}
	if len(expr.Children) != 1 {
		t.Fatalf("expected 1 child for unary BANG, got %d", len(expr.Children))
	}
}

func TestUnary_NegateNumber(t *testing.T) {
	node, errs := testutil.Parse("-42\n")
	if len(errs) > 0 {
		t.Fatalf("unexpected parse errors: %v", errs)
	}
	expr := node.Children[0]
	if expr.NodeType != ast.UnaryNode || expr.Token == nil || expr.Token.Type != token.MINUS {
		t.Fatalf("expected unary MINUS, got %v", expr)
	}
	if len(expr.Children) != 1 {
		t.Fatalf("expected 1 child for unary MINUS, got %d", len(expr.Children))
	}
}

// --- Function call argument parsing ---

func TestFunctionCall_ZeroArgs(t *testing.T) {
	node, errs := testutil.Parse("foo()\n")
	if len(errs) > 0 {
		t.Fatalf("unexpected parse errors: %v", errs)
	}
	call := node.Children[0]
	if call.NodeType != ast.FunctionCallNode {
		t.Fatalf("expected FunctionCallNode, got %v", call)
	}
	args := call.Children[1]
	if len(args.Children) != 0 {
		t.Fatalf("expected 0 args, got %d", len(args.Children))
	}
}

func TestFunctionCall_MultipleArgs(t *testing.T) {
	node, errs := testutil.Parse("foo(1, 2, 3)\n")
	if len(errs) > 0 {
		t.Fatalf("unexpected parse errors: %v", errs)
	}
	call := node.Children[0]
	args := call.Children[1]
	if len(args.Children) != 3 {
		t.Fatalf("expected 3 args, got %d", len(args.Children))
	}
}

func TestFunctionCall_TrailingComma(t *testing.T) {
	node, errs := testutil.Parse("foo(1, 2,)\n")
	if len(errs) > 0 {
		t.Fatalf("unexpected parse errors: %v", errs)
	}
	call := node.Children[0]
	args := call.Children[1]
	if len(args.Children) != 2 {
		t.Fatalf("expected 2 args with trailing comma, got %d", len(args.Children))
	}
}

// --- helpers ---

func findNodeType(node *ast.TreeNode, nt ast.NodeType) *ast.TreeNode {
	if node == nil {
		return nil
	}
	if node.NodeType == nt {
		return node
	}
	for _, child := range node.Children {
		if found := findNodeType(child, nt); found != nil {
			return found
		}
	}
	return nil
}

func TestParser_WhenAsExpression(t *testing.T) {
	src := "x = 3\ny = when x:\n    3 -> 'a'\n    _ -> 'b'\n-y\nprint(y)\n"
	node, errs := testutil.Parse(src)
	if len(errs) > 0 {
		t.Fatalf("unexpected parse errors: %v", errs)
	}
	// The `-y` line must be its own statement, not `when ... - y`.
	if got := len(node.Children); got != 4 {
		t.Fatalf("expected 4 top-level statements, got %d", got)
	}
}

// TestPrecedence_UnaryAndExponent: `**` binds tighter than prefix `-` and `!`
// (Python's rule), and its right operand may be negated.
func TestPrecedence_UnaryAndExponent(t *testing.T) {
	cases := []struct{ src, want string }{
		{"-2 ** 2", "(-(2 ** 2))"},
		{"-x ** 2 ** 3", "(-(x ** (2 ** 3)))"},
		{"2 ** -1", "(2 ** (-1))"},
		{"2 ** -1 ** 2", "(2 ** (-(1 ** 2)))"},
		{"-a * b", "((-a) * b)"},
		{"-a + b", "((-a) + b)"},
		{"x - -2 ** 2", "(x - (-(2 ** 2)))"},
		{"!x ** 2", "(!(x ** 2))"},
		{"(-2) ** 2", "((-2) ** 2)"},
		{"-xs[0] ** 2", "(-(xs[0] ** 2))"},
	}
	for _, tc := range cases {
		node, errs := testutil.Parse(tc.src + "\n")
		if len(errs) > 0 {
			t.Fatalf("%s: parse errors: %v", tc.src, errs)
		}
		if got := exprString(node.Children[0]); got != tc.want {
			t.Errorf("%s: got %s, want %s", tc.src, got, tc.want)
		}
	}
}

// exprString renders an operator tree with full parentheses.
func exprString(n *ast.TreeNode) string {
	if n == nil {
		return "<nil>"
	}
	switch {
	case n.NodeType == ast.UnaryNode && len(n.Children) == 1:
		return "(" + n.Token.Literal + exprString(n.Children[0]) + ")"
	case (n.NodeType == ast.BinaryNode || n.NodeType == ast.AssignNode) && len(n.Children) == 2:
		return "(" + exprString(n.Children[0]) + " " + n.Token.Literal + " " + exprString(n.Children[1]) + ")"
	case n.NodeType == ast.IndexNode && len(n.Children) == 2:
		return exprString(n.Children[0]) + "[" + exprString(n.Children[1]) + "]"
	}
	if n.Token != nil {
		return n.Token.Literal
	}
	return n.NodeType.String()
}
