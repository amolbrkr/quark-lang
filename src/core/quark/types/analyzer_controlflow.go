package types

import (
	"quark/ast"
	"quark/token"
)

func (a *Analyzer) analyzeIfStatement(node *ast.TreeNode) Type {
	if len(node.Children) < 2 {
		return TypeVoid
	}
	condType := a.Analyze(node.Children[0])
	if !isBoolLike(condType) && !isUnknownType(condType) {
		a.errorAt(node.Children[0], "condition must be bool, got %s; use a comparison or explicit to_bool()", condType.String())
	}
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
	var resultType Type = TypeVoid
	for i := 1; i < len(node.Children); i++ {
		pattern := node.Children[i]
		if pattern.NodeType != ast.PatternNode || len(pattern.Children) == 0 {
			continue
		}
		resultExpr := pattern.Children[len(pattern.Children)-1]
		bindName, hasBinding, bindingIsErr, resultPatternNode := extractResultPatternBinding(pattern)
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
			resultType = MergeTypes(resultType, branchType)
			continue
		}
		branchType := a.Analyze(resultExpr)
		resultType = MergeTypes(resultType, branchType)
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
	condType := a.Analyze(node.Children[0])
	if !isBoolLike(condType) && !isUnknownType(condType) {
		a.errorAt(node.Children[0], "condition must be bool, got %s; use a comparison or explicit to_bool()", condType.String())
	}
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
	condType := a.Analyze(node.Children[0])
	if !isBoolLike(condType) && !isUnknownType(condType) {
		a.errorAt(node.Children[0], "ternary condition must be bool, got %s; use a comparison or explicit to_bool()", condType.String())
	}
	trueType := a.Analyze(node.Children[1])
	falseType := a.Analyze(node.Children[2])
	if !isUnknownType(trueType) && !isUnknownType(falseType) && !CanAssign(trueType, falseType) && !CanAssign(falseType, trueType) {
		a.errorAt(node, "[warning] ternary branches have incompatible types: '%s' and '%s'", trueType.String(), falseType.String())
	}
	return MergeTypes(trueType, falseType)
}
