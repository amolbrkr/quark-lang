package invariants

import (
	"fmt"
	"quark/ast"
	"quark/builtins"
	"quark/ir"
)

func tokenPos(node *ast.TreeNode) (int, int) {
	if node != nil && node.Token != nil {
		return node.Token.Line, node.Token.Column
	}
	return 0, 0
}

func invError(code string, node *ast.TreeNode, format string, args ...interface{}) error {
	line, col := tokenPos(node)
	return fmt.Errorf("error[%s] (invariants): %s at line %d, col %d", code, fmt.Sprintf(format, args...), line, col)
}

func walk(node *ast.TreeNode, fn func(*ast.TreeNode)) {
	if node == nil {
		return
	}
	fn(node)
	for _, child := range node.Children {
		walk(child, fn)
	}
}

// ValidateCallPlans enforces analyzer->codegen call invariants before emission.
func ValidateCallPlans(root *ast.TreeNode, plans map[*ast.TreeNode]*ir.CallPlan) error {
	if root == nil {
		return nil
	}
	if plans == nil {
		return invError("INV-CALLPLAN-MAP", root, "call plan map is nil")
	}
	var firstErr error
	walk(root, func(n *ast.TreeNode) {
		if firstErr == nil && n.NodeType == ast.FunctionCallNode {
			firstErr = validateCallPlan(n, plans[n])
		}
	})
	return firstErr
}

// validateCallPlan checks the plan of one call.
func validateCallPlan(n *ast.TreeNode, plan *ir.CallPlan) error {
	if plan == nil {
		return invError("INV-CALLPLAN-MISSING", n, "missing call plan")
	}
	if plan.MinArity < 0 || plan.MaxArity < 0 || plan.MinArity > plan.MaxArity {
		return invError("INV-ARITY-ENVELOPE", n, "invalid arity envelope min=%d max=%d", plan.MinArity, plan.MaxArity)
	}
	switch plan.Dispatch {
	case ir.DispatchBuiltin:
		if err := validateBuiltinPlan(n, plan); err != nil {
			return err
		}
	case ir.DispatchDirect, ir.DispatchExtern, ir.DispatchNative:
		if plan.RuntimeSymbol == "" {
			return invError("INV-RUNTIME-SYMBOL", n, "call plan for '%s' requires runtime symbol but has none", plan.CalleeName)
		}
	case ir.DispatchDynamicMethod:
		if err := validateDynamicMethodPlan(n, plan); err != nil {
			return err
		}
	case ir.DispatchClosure:
	default:
		return invError("INV-DISPATCH-MODE", n, "call plan for '%s' has unresolved dispatch mode", plan.CalleeName)
	}

	// Planned defaults fill missing trailing arguments only.
	provided := 0
	if len(n.Children) >= 2 && n.Children[1] != nil {
		provided = len(n.Children[1].Children)
	}
	missing := max(plan.MaxArity-min(provided, plan.MaxArity), 0)
	if len(plan.DefaultNodes) > missing {
		return invError("INV-DEFAULT-COUNT", n, "planned defaults (%d) exceed missing arg slots (%d)", len(plan.DefaultNodes), missing)
	}
	return nil
}

// validateBuiltinPlan checks that a builtin call or method names a catalog
// entry with the matching runtime symbol.
func validateBuiltinPlan(n *ast.TreeNode, plan *ir.CallPlan) error {
	if plan.RuntimeSymbol == "" {
		return invError("INV-RUNTIME-SYMBOL", n, "call plan for '%s' requires runtime symbol but has none", plan.CalleeName)
	}
	if plan.CalleeName == "" {
		return invError("INV-BUILTIN-NAME", n, "builtin call plan missing callee name")
	}
	if plan.IsMethod {
		if plan.ReceiverTypeKey == "" {
			return invError("INV-METHOD-RECEIVER", n, "method call plan for '%s' missing receiver type key", plan.CalleeName)
		}
		spec, ok := builtins.LookupMethod(builtins.TypeKey(plan.ReceiverTypeKey), plan.CalleeName)
		if !ok {
			return invError("INV-BUILTIN-CATALOG", n, "method '%s' is not in catalog for receiver type '%s'", plan.CalleeName, plan.ReceiverTypeKey)
		}
		if plan.RuntimeSymbol != spec.Runtime {
			return invError("INV-BUILTIN-RUNTIME", n, "method '%s' runtime symbol mismatch: plan='%s' catalog='%s'", plan.CalleeName, plan.RuntimeSymbol, spec.Runtime)
		}
		return nil
	}
	spec, ok := builtins.Lookup(plan.CalleeName)
	if !ok {
		return invError("INV-BUILTIN-CATALOG", n, "builtin '%s' is not in catalog", plan.CalleeName)
	}
	if plan.RuntimeSymbol != spec.Runtime {
		return invError("INV-BUILTIN-RUNTIME", n, "builtin '%s' runtime symbol mismatch: plan='%s' catalog='%s'", plan.CalleeName, plan.RuntimeSymbol, spec.Runtime)
	}
	return nil
}

// validateDynamicMethodPlan checks that every candidate of a dynamic method
// call is a catalog method with the matching runtime symbol.
func validateDynamicMethodPlan(n *ast.TreeNode, plan *ir.CallPlan) error {
	if !plan.IsMethod || plan.ReceiverNode == nil || len(plan.MethodCandidates) == 0 {
		return invError("INV-DYNAMIC-METHOD", n, "dynamic method call plan for '%s' needs a receiver and candidates", plan.CalleeName)
	}
	for _, c := range plan.MethodCandidates {
		spec, ok := builtins.LookupMethod(builtins.TypeKey(c.ReceiverTypeKey), plan.CalleeName)
		if !ok || c.ValueTag == "" || c.RuntimeSymbol != spec.Runtime {
			return invError("INV-DYNAMIC-METHOD", n, "dynamic method '%s' has an invalid candidate for receiver type '%s'", plan.CalleeName, c.ReceiverTypeKey)
		}
	}
	return nil
}

// ValidateReturnAnnotations ensures annotated function/lambda nodes were validated in analyzer.
func ValidateReturnAnnotations(root *ast.TreeNode, validated map[*ast.TreeNode]bool) error {
	if root == nil {
		return nil
	}
	if validated == nil {
		return invError("INV-RETURN-MAP", root, "return-validation map is nil")
	}

	var firstErr error
	walk(root, func(n *ast.TreeNode) {
		if firstErr != nil {
			return
		}
		if n.ReturnType == nil {
			return
		}
		if n.NodeType != ast.LambdaNode {
			return
		}
		if !validated[n] {
			firstErr = invError("INV-RETURN-VALIDATE", n, "annotated return type was not validated before codegen")
		}
	})
	return firstErr
}
