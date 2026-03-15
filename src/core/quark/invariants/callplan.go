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
	return fmt.Errorf("internal compiler error [%s] at line %d, col %d: %s", code, line, col, fmt.Sprintf(format, args...))
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

func walkWithParent(node *ast.TreeNode, parent *ast.TreeNode, fn func(*ast.TreeNode, *ast.TreeNode)) {
	if node == nil {
		return
	}
	fn(node, parent)
	for _, child := range node.Children {
		walkWithParent(child, node, fn)
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
	walkWithParent(root, nil, func(n *ast.TreeNode, parent *ast.TreeNode) {
		if firstErr != nil {
			return
		}
		if n.NodeType != ast.FunctionCallNode {
			return
		}

		plan := plans[n]
		if plan == nil {
			firstErr = invError("INV-CALLPLAN-MISSING", n, "missing call plan")
			return
		}
		if plan.MinArity < 0 || plan.MaxArity < 0 || plan.MinArity > plan.MaxArity {
			firstErr = invError("INV-ARITY-ENVELOPE", n, "invalid arity envelope min=%d max=%d", plan.MinArity, plan.MaxArity)
			return
		}
		if !plan.ArgTypesChecked {
			firstErr = invError("INV-ARGCHECK", n, "call plan for '%s' did not record argument type-checking", plan.CalleeName)
			return
		}
		if plan.Dispatch == ir.DispatchBuiltin {
			if plan.CalleeName == "" {
				firstErr = invError("INV-BUILTIN-NAME", n, "builtin call plan missing callee name")
				return
			}
			spec, ok := builtins.Lookup(plan.CalleeName)
			if !ok {
				firstErr = invError("INV-BUILTIN-CATALOG", n, "builtin '%s' is not in catalog", plan.CalleeName)
				return
			}
			if plan.RuntimeSymbol == "" || plan.RuntimeSymbol != spec.Runtime {
				firstErr = invError("INV-BUILTIN-RUNTIME", n, "builtin '%s' runtime symbol mismatch: plan='%s' catalog='%s'", plan.CalleeName, plan.RuntimeSymbol, spec.Runtime)
				return
			}
		}

		provided := 0
		if len(n.Children) >= 2 && n.Children[1] != nil {
			provided = len(n.Children[1].Children)
		}
		if parent != nil && parent.NodeType == ast.PipeNode && len(parent.Children) >= 2 && parent.Children[1] == n {
			provided++ // piped input occupies argument slot 0
		}
		if provided > plan.MaxArity {
			provided = plan.MaxArity
		}
		missing := plan.MaxArity - provided
		if missing < 0 {
			missing = 0
		}
		if len(plan.DefaultNodes) > missing {
			firstErr = invError("INV-DEFAULT-COUNT", n, "planned defaults (%d) exceed missing arg slots (%d)", len(plan.DefaultNodes), missing)
			return
		}
	})
	return firstErr
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
		if n.NodeType != ast.FunctionNode && n.NodeType != ast.LambdaNode {
			return
		}
		if !validated[n] {
			firstErr = invError("INV-RETURN-VALIDATE", n, "annotated return type was not validated before codegen")
		}
	})
	return firstErr
}
