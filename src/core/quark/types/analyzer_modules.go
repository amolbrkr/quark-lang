package types

import (
	"quark/ast"
	"quark/ir"
)

func (a *Analyzer) analyzeModule(node *ast.TreeNode) Type {
	if len(node.Children) < 2 {
		a.errorAt(node, "invalid module definition")
		return TypeVoid
	}
	nameNode := node.Children[0]
	bodyNode := node.Children[1]
	moduleName := nameNode.TokenLiteral()
	if _, exists := a.modules[moduleName]; exists {
		a.errorAt(nameNode, "module '%s' already defined", moduleName)
		return TypeVoid
	}
	moduleScope := NewScope(a.currentScope)
	oldScope := a.currentScope
	a.currentScope = moduleScope
	a.currentModule = moduleName
	if bodyNode != nil && bodyNode.NodeType == ast.BlockNode {
		a.predeclareFunctions(bodyNode.Children)
		for _, child := range bodyNode.Children {
			a.Analyze(child)
		}
	} else {
		a.Analyze(bodyNode)
	}
	module := &Module{Name: moduleName, Scope: moduleScope, Symbols: moduleScope.Symbols}
	a.modules[moduleName] = module
	a.currentScope = oldScope
	a.currentModule = ""
	return TypeVoid
}

func (a *Analyzer) analyzeUse(node *ast.TreeNode) Type {
	if len(node.Children) < 1 {
		a.errorAt(node, "invalid use statement")
		return TypeVoid
	}
	nameNode := node.Children[0]
	moduleName := nameNode.TokenLiteral()
	alias := moduleName
	hasExplicitAlias := false
	if len(node.Children) >= 2 && node.Children[1] != nil && node.Children[1].NodeType == ast.IdentifierNode {
		alias = node.Children[1].TokenLiteral()
		hasExplicitAlias = true
	}
	module, exists := a.modules[moduleName]
	if !exists {
		a.errorAt(nameNode, "undefined module '%s'", moduleName)
		return TypeVoid
	}
	if alias != "" {
		if existing, exists := a.moduleAliases[alias]; exists && existing != moduleName {
			a.errorAt(node, "module alias '%s' already refers to module '%s'", alias, existing)
		} else {
			a.moduleAliases[alias] = moduleName
		}
	}
	if hasExplicitAlias {
		return TypeVoid
	}
	for name, sym := range module.Symbols {
		if existing := a.currentScope.LookupLocal(name); existing != nil {
			a.errorAt(node, "symbol '%s' from module '%s' conflicts with existing definition", name, moduleName)
			continue
		}
		a.currentScope.Define(name, sym.Type, sym.Mutable)
	}
	return TypeVoid
}

func (a *Analyzer) GetModules() map[string]*Module {
	return a.modules
}

func (a *Analyzer) GetCaptures() map[*ast.TreeNode][]string {
	return a.captures
}

func (a *Analyzer) GetCallPlans() map[*ast.TreeNode]*ir.CallPlan {
	return a.callPlans
}

func (a *Analyzer) GetReturnValidation() map[*ast.TreeNode]bool {
	return a.returnValidated
}

// GetCapturedByFunction returns, for each function or lambda node, the set of
// variable names that any directly-nested lambda captures from that scope.
// Codegen uses this to decide: QCell* (captured) vs QValue (stack-local).
func (a *Analyzer) GetCapturedByFunction(root *ast.TreeNode) map[*ast.TreeNode]map[string]bool {
	result := make(map[*ast.TreeNode]map[string]bool)
	if root == nil || len(a.captures) == 0 {
		return result
	}

	// Walk the AST tracking the immediately enclosing function/lambda for each node.
	// When we hit a LambdaNode that has captures, attribute those names to its
	// enclosing function/lambda — that is the scope that owns the variables.
	var walk func(n *ast.TreeNode, enclosing *ast.TreeNode)
	walk = func(n *ast.TreeNode, enclosing *ast.TreeNode) {
		if n == nil {
			return
		}
		if n.NodeType == ast.LambdaNode {
			if enclosing != nil {
				if names, ok := a.captures[n]; ok {
					if result[enclosing] == nil {
						result[enclosing] = make(map[string]bool)
					}
					for _, name := range names {
						result[enclosing][name] = true
					}
				}
			}
			// Children of this lambda are now enclosed by it.
			for _, child := range n.Children {
				walk(child, n)
			}
			return
		}
		if n.NodeType == ast.FunctionNode {
			for _, child := range n.Children {
				walk(child, n)
			}
			return
		}
		for _, child := range n.Children {
			walk(child, enclosing)
		}
	}
	walk(root, nil)
	return result
}
