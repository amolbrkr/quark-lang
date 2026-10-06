package types

import (
	"quark/ast"
	"quark/token"
)

// checkCondition reports an error when a value with no truthiness is used
// where a condition is expected (if, elseif, while, ternary, !, and, or).
// Results would silently discard errors, and vector comparisons yield a
// vector[bool] whose emptiness says nothing about its elements.
func (a *Analyzer) checkCondition(node *ast.TreeNode, t Type) {
	switch t.(type) {
	case *ResultType:
		a.errorAt(node, "result cannot be used as a condition; use is_ok(r), is_err(r) or when")
	case *VectorType:
		a.errorAt(node, "vector cannot be used as a condition; use all(v), any(v) or len(v) > 0")
	}
}

func (a *Analyzer) analyzeIfStatement(node *ast.TreeNode) Type {
	if len(node.Children) < 2 {
		return TypeVoid
	}
	a.checkCondition(node.Children[0], a.Analyze(node.Children[0]))
	resultType := a.Analyze(node.Children[1])
	for i := 2; i < len(node.Children); i++ {
		branchType := a.Analyze(node.Children[i])
		resultType = MergeTypes(resultType, branchType)
	}
	return resultType
}

func (a *Analyzer) analyzeWhenStatement(node *ast.TreeNode) Type {
	if len(node.Children) < 2 {
		return TypeVoid
	}
	matchType := a.Analyze(node.Children[0])
	resultMatchType, isResultMatch := matchType.(*ResultType)
	// The when evaluates to the matched arm's value. It is exhaustive when it
	// has a wildcard arm or both ok and err arms; otherwise no arm may match
	// and the value is null, so void joins the result type.
	var resultType Type
	merge := func(t Type) {
		if resultType == nil {
			resultType = t
			return
		}
		resultType = MergeTypes(resultType, t)
	}
	hasWildcard, hasOk, hasErr := false, false, false
	for i := 1; i < len(node.Children); i++ {
		pattern := node.Children[i]
		if pattern.NodeType != ast.PatternNode || len(pattern.Children) == 0 {
			continue
		}
		resultExpr := pattern.Children[len(pattern.Children)-1]
		for _, pc := range pattern.Children[:len(pattern.Children)-1] {
			if pc.NodeType == ast.IdentifierNode && pc.TokenLiteral() == "_" {
				hasWildcard = true
			}
		}
		bindName, hasBinding, bindingIsErr, resultPatternNode := extractResultPatternBinding(pattern)
		if hasBinding {
			if bindingIsErr {
				hasErr = true
			} else {
				hasOk = true
			}
		}
		if hasBinding && !isResultMatch && !isUnknownType(matchType) {
			a.errorAt(resultPatternNode, "result pattern requires result value, got %s", matchType.String())
		}
		var bindingType Type = TypeAny
		if isResultMatch {
			if bindingIsErr {
				bindingType = resultMatchType.ErrType
			} else {
				bindingType = resultMatchType.OkType
			}
		}
		if hasBinding && bindName != "" {
			a.pushScope()
			a.currentScope.Define(bindName, bindingType, true)
			branchType := a.Analyze(resultExpr)
			a.popScope()
			merge(branchType)
			continue
		}
		branchType := a.Analyze(resultExpr)
		merge(branchType)
	}
	if resultType == nil {
		return TypeVoid
	}
	if !hasWildcard && !(hasOk && hasErr) {
		resultType = MergeTypes(resultType, TypeVoid)
	}
	return resultType
}

func extractResultPatternBinding(pattern *ast.TreeNode) (string, bool, bool, *ast.TreeNode) {
	if pattern == nil || len(pattern.Children) == 0 {
		return "", false, false, nil
	}
	for i := 0; i < len(pattern.Children)-1; i++ {
		child := pattern.Children[i]
		if child.NodeType != ast.ResultPatternNode || len(child.Children) == 0 {
			continue
		}
		bindNode := child.Children[0]
		isErr := child.Token != nil && child.Token.Type == token.ERR
		if bindNode == nil {
			return "", true, isErr, child
		}
		name := bindNode.TokenLiteral()
		if name == "_" {
			return "", true, isErr, child
		}
		return name, true, isErr, child
	}
	return "", false, false, nil
}

func (a *Analyzer) analyzeResult(node *ast.TreeNode) Type {
	if len(node.Children) == 0 {
		return &ResultType{OkType: TypeAny, ErrType: TypeAny}
	}
	payloadType := a.Analyze(node.Children[0])
	if node.Token != nil && node.Token.Type == token.ERR {
		return &ResultType{OkType: TypeAny, ErrType: payloadType}
	}
	return &ResultType{OkType: payloadType, ErrType: TypeAny}
}

func (a *Analyzer) analyzeForLoop(node *ast.TreeNode) Type {
	if len(node.Children) < 3 {
		return TypeVoid
	}
	varNode := node.Children[0]
	iterNode := node.Children[1]
	bodyNode := node.Children[2]
	iterType := a.Analyze(iterNode)
	if _, ok := iterType.(*ListType); !ok {
		if _, isVector := iterType.(*VectorType); !isVector {
			if !iterType.Equals(TypeString) {
				if !isUnknownType(iterType) {
					a.errorAt(iterNode, "for loop expects list, vector, or str iterable, got %s", iterType.String())
					return TypeVoid
				}
			}
		}
	}
	a.loopDepth++
	a.pushScope()
	varName := varNode.TokenLiteral()
	var varType Type = TypeAny
	switch t := iterType.(type) {
	case *ListType:
		varType = t.ElementType
	case *VectorType:
		varType = t.ElementType
	default:
		if iterType.Equals(TypeString) {
			varType = TypeString
		} else if !isUnknownType(iterType) {
			a.errorAt(iterNode, "value of type '%s' is not iterable", iterType.String())
		}
	}
	a.currentScope.Define(varName, varType, false)
	a.Analyze(bodyNode)
	a.popScope()
	a.loopDepth--
	return TypeVoid
}

func (a *Analyzer) analyzeWhileLoop(node *ast.TreeNode) Type {
	if len(node.Children) < 2 {
		return TypeVoid
	}
	a.checkCondition(node.Children[0], a.Analyze(node.Children[0]))
	a.loopDepth++
	a.pushScope()
	a.Analyze(node.Children[1])
	a.popScope()
	a.loopDepth--
	return TypeVoid
}

func (a *Analyzer) analyzeTernary(node *ast.TreeNode) Type {
	if len(node.Children) < 3 {
		return TypeAny
	}
	a.checkCondition(node.Children[0], a.Analyze(node.Children[0]))
	trueType := a.Analyze(node.Children[1])
	falseType := a.Analyze(node.Children[2])
	if !isUnknownType(trueType) && !isUnknownType(falseType) && !CanAssign(trueType, falseType) && !CanAssign(falseType, trueType) {
		a.warnAt(node, "ternary branches have incompatible types: '%s' and '%s'", trueType.String(), falseType.String())
	}
	return MergeTypes(trueType, falseType)
}
