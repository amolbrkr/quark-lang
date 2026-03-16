package invariants_test

import (
	"strings"
	"testing"

	"quark/ast"
	"quark/internal/testutil"
	"quark/invariants"
	"quark/ir"
)

func findFirstCall(node *ast.TreeNode) *ast.TreeNode {
	if node == nil {
		return nil
	}
	if node.NodeType == ast.FunctionCallNode {
		return node
	}
	for _, child := range node.Children {
		if found := findFirstCall(child); found != nil {
			return found
		}
	}
	return nil
}

func TestValidateCallPlans_MissingPlan(t *testing.T) {
	_, tree, parseErrs, typeErrs := testutil.Analyze("println(len('x'))\n")
	if len(parseErrs) > 0 {
		t.Fatalf("unexpected parse errors: %v", parseErrs)
	}
	if len(typeErrs) > 0 {
		t.Fatalf("unexpected type errors: %v", typeErrs)
	}

	err := invariants.ValidateCallPlans(tree, map[*ast.TreeNode]*ir.CallPlan{})
	if err == nil {
		t.Fatalf("expected missing call plan invariant error")
	}
	if !strings.Contains(err.Error(), "INV-CALLPLAN-MISSING") {
		t.Fatalf("expected INV-CALLPLAN-MISSING, got: %v", err)
	}
}

func TestValidateCallPlans_DefaultOverflow(t *testing.T) {
	analyzer, tree, parseErrs, typeErrs := testutil.Analyze("fn add_n(x: int, n: int = 10) int -> x + n\nadd_n(1, 2)\n")
	if len(parseErrs) > 0 {
		t.Fatalf("unexpected parse errors: %v", parseErrs)
	}
	if len(typeErrs) > 0 {
		t.Fatalf("unexpected type errors: %v", typeErrs)
	}

	call := findFirstCall(tree)
	if call == nil {
		t.Fatalf("expected call node")
	}
	plans := analyzer.GetCallPlans()
	plan := plans[call]
	if plan == nil {
		t.Fatalf("expected call plan")
	}
	plan.DefaultNodes = []*ast.TreeNode{call, call}

	err := invariants.ValidateCallPlans(tree, plans)
	if err == nil {
		t.Fatalf("expected default overflow invariant error")
	}
	if !strings.Contains(err.Error(), "INV-DEFAULT-COUNT") {
		t.Fatalf("expected INV-DEFAULT-COUNT, got: %v", err)
	}
}

func TestValidateCallPlans_DispatchUnknown(t *testing.T) {
	analyzer, tree, parseErrs, typeErrs := testutil.Analyze("println('hello')\n")
	if len(parseErrs) > 0 {
		t.Fatalf("unexpected parse errors: %v", parseErrs)
	}
	if len(typeErrs) > 0 {
		t.Fatalf("unexpected type errors: %v", typeErrs)
	}

	call := findFirstCall(tree)
	if call == nil {
		t.Fatalf("expected call node")
	}
	plans := analyzer.GetCallPlans()
	plan := plans[call]
	if plan == nil {
		t.Fatalf("expected call plan")
	}
	// Corrupt the dispatch mode to zero (DispatchUnknown)
	plan.Dispatch = ir.DispatchUnknown

	err := invariants.ValidateCallPlans(tree, plans)
	if err == nil {
		t.Fatalf("expected dispatch mode invariant error")
	}
	if !strings.Contains(err.Error(), "INV-DISPATCH-MODE") {
		t.Fatalf("expected INV-DISPATCH-MODE, got: %v", err)
	}
}

func TestValidateCallPlans_RuntimeSymbolMissing(t *testing.T) {
	// Construct a synthetic call plan with DispatchDirect but empty RuntimeSymbol
	callNode := &ast.TreeNode{
		NodeType: ast.FunctionCallNode,
		Children: []*ast.TreeNode{
			{NodeType: ast.IdentifierNode},
			{NodeType: ast.ArgumentsNode, Children: []*ast.TreeNode{}},
		},
	}
	root := &ast.TreeNode{
		NodeType: ast.CompilationUnitNode,
		Children: []*ast.TreeNode{callNode},
	}
	plans := map[*ast.TreeNode]*ir.CallPlan{
		callNode: {
			Kind:            ir.CallFunctionValue,
			CalleeName:      "foo",
			MinArity:        0,
			MaxArity:        0,
			Dispatch:        ir.DispatchDirect,
			RuntimeSymbol:   "", // intentionally empty
			ArgTypesChecked: true,
		},
	}

	err := invariants.ValidateCallPlans(root, plans)
	if err == nil {
		t.Fatalf("expected runtime symbol invariant error")
	}
	if !strings.Contains(err.Error(), "INV-RUNTIME-SYMBOL") {
		t.Fatalf("expected INV-RUNTIME-SYMBOL, got: %v", err)
	}
}

func TestValidateCallPlans_ValidPlansPass(t *testing.T) {
	// Valid program with all 3 dispatch modes should pass invariants
	analyzer, tree, parseErrs, typeErrs := testutil.Analyze("fn double(x) -> x * 2\ndouble(5)\nprintln(10)\nf = fn(x) -> x\nf(1)\n")
	if len(parseErrs) > 0 {
		t.Fatalf("unexpected parse errors: %v", parseErrs)
	}
	if len(typeErrs) > 0 {
		t.Fatalf("unexpected type errors: %v", typeErrs)
	}
	err := invariants.ValidateCallPlans(tree, analyzer.GetCallPlans())
	if err != nil {
		t.Fatalf("expected no invariant errors for valid program, got: %v", err)
	}
}

func TestValidateReturnAnnotations_MissingValidation(t *testing.T) {
	_, tree, parseErrs, typeErrs := testutil.Analyze("f = fn(x: int) int -> x\n")
	if len(parseErrs) > 0 {
		t.Fatalf("unexpected parse errors: %v", parseErrs)
	}
	if len(typeErrs) > 0 {
		t.Fatalf("unexpected type errors: %v", typeErrs)
	}

	err := invariants.ValidateReturnAnnotations(tree, map[*ast.TreeNode]bool{})
	if err == nil {
		t.Fatalf("expected return validation invariant error")
	}
	if !strings.Contains(err.Error(), "INV-RETURN-VALIDATE") {
		t.Fatalf("expected INV-RETURN-VALIDATE, got: %v", err)
	}
}
