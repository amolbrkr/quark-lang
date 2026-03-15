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
