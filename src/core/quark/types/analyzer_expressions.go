package types

import (
	"quark/ast"
	"quark/token"
)

func (a *Analyzer) analyzeIdentifier(node *ast.TreeNode) Type {
	name := node.TokenLiteral()
	if name == "_" {
		return TypeAny
	}
	sym := a.currentScope.Lookup(name)
	if sym == nil {
		a.errorAt(node, "undefined identifier '%s'", name)
		return TypeError
	}
	return sym.Type
}

func (a *Analyzer) analyzeLiteral(node *ast.TreeNode) Type {
	if node.Token == nil {
		return TypeAny
	}
	switch node.Token.Type {
	case token.INT:
		return TypeInt
	case token.FLOAT:
		return TypeFloat
	case token.STRING:
		return TypeString
	case token.TRUE, token.FALSE:
		return TypeBool
	case token.NULL:
		return TypeNull
	default:
		a.errorAt(node, "unsupported literal type: %s", node.Token.Type)
		return TypeError
	}
}

func (a *Analyzer) analyzeOperator(node *ast.TreeNode) Type {
	if node.Token == nil || len(node.Children) == 0 {
		a.errorAt(node, "malformed operator expression")
		return TypeError
	}
	op := node.Token.Type
	if len(node.Children) >= 2 {
		if node.Children[0] == nil || node.Children[1] == nil {
			a.errorAt(node, "malformed operator expression")
			return TypeError
		}
	}
	if op == token.DOT {
		if len(node.Children) < 2 {
			return TypeError
		}
		if node.Children[0] == nil || node.Children[1] == nil {
			a.errorAt(node, "malformed dot access expression")
			return TypeError
		}
		targetType := a.Analyze(node.Children[0])
		if IsErrorType(targetType) {
			return TypeError
		}
		member := node.Children[1].TokenLiteral()
		if targetType.Equals(TypeNull) {
			a.errorAt(node.Children[0], "cannot access member '%s' on null", member)
			return TypeError
		}
		switch t := targetType.(type) {
		case *DictType:
			return t.ValueType
		default:
			if isUnknownType(targetType) {
				return TypeAny
			}
			a.errorAt(node, "dot access is only supported on dict; use len(entity), upper(entity), etc. instead of entity.%s", member)
			return TypeError
		}
	}
	if len(node.Children) == 1 {
		operandType := a.Analyze(node.Children[0])
		if IsErrorType(operandType) {
			return TypeError
		}
		switch op {
		case token.MINUS:
			if IsNumeric(operandType) {
				return operandType
			}
			if isUnknownType(operandType) {
				return TypeAny
			}
			a.errorAt(node, "unary '-' expects numeric operand, got %s", operandType.String())
			return TypeError
		case token.BANG:
			if !isBoolLike(operandType) && !isUnknownType(operandType) {
				a.errorAt(node, "unary '!' expects bool operand, got %s", operandType.String())
			}
			return TypeBool
		}
		return operandType
	}
	if op == token.EQUALS && len(node.Children) == 2 {
		target := node.Children[0]
		if target == nil {
			a.errorAt(node, "left side of assignment must be an identifier")
			return TypeError
		}
		if target.NodeType == ast.IdentifierNode && node.Children[1].NodeType == ast.LambdaNode {
			varName := target.TokenLiteral()
			if a.currentScope.LookupLocal(varName) == nil {
				funcType := functionTypeFromLambdaNode(node.Children[1])
				a.currentScope.Define(varName, funcType, true)
				a.functions[varName] = funcType
			}
			a.pendingFuncName = varName
		}
		rightType := a.Analyze(node.Children[1])
		if target.NodeType == ast.OperatorNode && target.Token != nil && target.Token.Type == token.DOT {
			targetType := a.Analyze(target.Children[0])
			if targetType.Equals(TypeNull) {
				a.errorAt(target.Children[0], "cannot assign member on null")
				return rightType
			}
			if dictType, ok := targetType.(*DictType); ok {
				// Check value type compatibility
				if !isUnknownType(rightType) && !IsErrorType(rightType) && !isUnknownType(dictType.ValueType) && !CanAssign(dictType.ValueType, rightType) {
					a.warnAt(target, "assigning '%s' to dict with value type '%s'", rightType.String(), dictType.ValueType.String())
				}
				return rightType
			}
			if isUnknownType(targetType) {
				return rightType
			}
			a.errorAt(target, "only dict members are assignable")
			return rightType
		}
		if target.NodeType == ast.IndexNode {
			if len(target.Children) >= 2 {
				targetType := a.Analyze(target.Children[0])
				indexType := a.Analyze(target.Children[1])
				if targetType.Equals(TypeNull) {
					a.errorAt(target.Children[0], "cannot index null")
					return rightType
				}
				if _, ok := targetType.(*ListType); ok {
					if !isIntLike(indexType) && !isUnknownType(indexType) {
						a.errorAt(target.Children[1], "list index must be int, got %s", indexType.String())
					}
					return rightType
				}
				if targetType.Equals(TypeString) {
					a.errorAt(target, "strings are immutable")
					return rightType
				}
				if _, ok := targetType.(*DictType); ok {
					a.errorAt(target, "use dot access for dict assignment: d.key = value")
					return rightType
				}
				if !isUnknownType(targetType) {
					a.errorAt(target, "type '%s' is not index-assignable", targetType.String())
				}
			}
			return rightType
		}
		if target.NodeType != ast.IdentifierNode {
			a.errorAt(target, "left side of assignment must be an identifier")
			return rightType
		}
		varName := target.TokenLiteral()
		sym := a.currentScope.Lookup(varName)
		if sym == nil {
			a.currentScope.Define(varName, rightType, true)
		} else {
			if _, dstIsFunc := sym.Type.(*FunctionType); dstIsFunc {
				if srcFunc, srcIsFunc := rightType.(*FunctionType); srcIsFunc {
					sym.Type = srcFunc
					a.functions[varName] = srcFunc
					return rightType
				}
			}
			if _, srcIsResult := rightType.(*ResultType); srcIsResult {
				if _, dstIsResult := sym.Type.(*ResultType); !dstIsResult && !isUnknownType(sym.Type) {
					a.errorAt(target, "[C-TYPE] cannot assign result to '%s'; use 'unwrap()' or 'when' to extract the value", sym.Type.String())
					return rightType
				}
			}
			if !CanAssign(sym.Type, rightType) && !isUnknownType(rightType) {
				a.errorAt(target, "cannot assign value of type '%s' to '%s'", rightType.String(), sym.Type.String())
			}
			sym.Type = rightType
		}
		return rightType
	}

	leftType := a.Analyze(node.Children[0])
	rightType := a.Analyze(node.Children[1])
	// Propagate ErrorType silently
	if IsErrorType(leftType) || IsErrorType(rightType) {
		return TypeError
	}
	leftVec, leftIsVec := leftType.(*VectorType)
	rightVec, rightIsVec := rightType.(*VectorType)
	isNumericScalar := func(t Type) bool { return t.Equals(TypeInt) || t.Equals(TypeFloat) }

	if op == token.PLUS || op == token.MINUS || op == token.MULTIPLY || op == token.DIVIDE {
		if leftIsVec && rightIsVec {
			if !IsNumeric(leftVec.ElementType) && !isUnknownType(leftVec.ElementType) {
				a.errorAt(node, "operator '%s' requires numeric vector operands, got %s", node.Token.Type.String(), leftType.String())
				return TypeError
			}
			if !IsNumeric(rightVec.ElementType) && !isUnknownType(rightVec.ElementType) {
				a.errorAt(node, "operator '%s' requires numeric vector operands, got %s", node.Token.Type.String(), rightType.String())
				return TypeError
			}
			if leftVec.ElementType.Equals(TypeFloat) || rightVec.ElementType.Equals(TypeFloat) {
				return &VectorType{ElementType: TypeFloat}
			}
			if leftVec.ElementType.Equals(TypeInt) && rightVec.ElementType.Equals(TypeInt) {
				if op == token.DIVIDE {
					return &VectorType{ElementType: TypeFloat}
				}
				return &VectorType{ElementType: TypeInt}
			}
			return &VectorType{ElementType: TypeAny}
		}
		if leftIsVec && (isNumericScalar(rightType) || isUnknownType(rightType)) {
			if !IsNumeric(leftVec.ElementType) && !isUnknownType(leftVec.ElementType) {
				a.errorAt(node, "operator '%s' requires numeric vector operands, got %s", node.Token.Type.String(), leftType.String())
				return TypeError
			}
			if op == token.DIVIDE && leftVec.ElementType.Equals(TypeInt) && rightType.Equals(TypeInt) {
				return &VectorType{ElementType: TypeFloat}
			}
			return leftType
		}
		if rightIsVec && (isNumericScalar(leftType) || isUnknownType(leftType)) {
			if !IsNumeric(rightVec.ElementType) && !isUnknownType(rightVec.ElementType) {
				a.errorAt(node, "operator '%s' requires numeric vector operands, got %s", node.Token.Type.String(), rightType.String())
				return TypeError
			}
			if op == token.DIVIDE && rightVec.ElementType.Equals(TypeInt) && leftType.Equals(TypeInt) {
				return &VectorType{ElementType: TypeFloat}
			}
			return rightType
		}
	}

	switch op {
	case token.PLUS, token.MINUS, token.MULTIPLY, token.DIVIDE, token.MODULO, token.DOUBLESTAR:
		if op == token.MODULO {
			if isIntLike(leftType) && isIntLike(rightType) {
				return TypeInt
			}
			if isUnknownType(leftType) || isUnknownType(rightType) {
				return TypeAny
			}
			a.errorAt(node, "operator '%%' requires integer operands, got %s and %s", leftType.String(), rightType.String())
			return TypeError
		}
		if op == token.PLUS && isStringLike(leftType) && isStringLike(rightType) {
			return TypeString
		}
		if IsNumeric(leftType) && IsNumeric(rightType) {
			if op == token.DIVIDE {
				return TypeFloat
			}
			if leftType.Equals(TypeFloat) || rightType.Equals(TypeFloat) {
				return TypeFloat
			}
			if op == token.MODULO {
				return TypeInt
			}
			return TypeInt
		}
		if isUnknownType(leftType) || isUnknownType(rightType) {
			return TypeAny
		}
		a.errorAt(node, "operator '%s' requires numeric operands, got %s and %s", node.Token.Type.String(), leftType.String(), rightType.String())
		return TypeError
	case token.LT, token.LTE, token.GT, token.GTE:
		if leftIsVec || rightIsVec {
			if leftIsVec && !IsNumeric(leftVec.ElementType) && !isUnknownType(leftVec.ElementType) {
				a.errorAt(node, "ordering comparison requires numeric vector, got %s", leftType.String())
			}
			if rightIsVec && !IsNumeric(rightVec.ElementType) && !isUnknownType(rightVec.ElementType) {
				a.errorAt(node, "ordering comparison requires numeric vector, got %s", rightType.String())
			}
			if leftIsVec && !rightIsVec && !isNumericScalar(rightType) && !isUnknownType(rightType) {
				a.errorAt(node, "cannot compare vector with %s", rightType.String())
			}
			if rightIsVec && !leftIsVec && !isNumericScalar(leftType) && !isUnknownType(leftType) {
				a.errorAt(node, "cannot compare vector with %s", leftType.String())
			}
			return &VectorType{ElementType: TypeBool}
		}
		if IsComparable(leftType) && IsComparable(rightType) {
			return TypeBool
		}
		if isUnknownType(leftType) || isUnknownType(rightType) {
			return TypeBool
		}
		a.errorAt(node, "comparison requires comparable operands, got %s and %s", leftType.String(), rightType.String())
		return TypeBool
	case token.DEQ, token.NE:
		if leftIsVec || rightIsVec {
			return &VectorType{ElementType: TypeBool}
		}
		return TypeBool
	case token.AND, token.OR:
		if isBoolLike(leftType) && isBoolLike(rightType) {
			return TypeBool
		}
		if isUnknownType(leftType) || isUnknownType(rightType) {
			return TypeBool
		}
		a.errorAt(node, "logical operator '%s' expects boolean operands, got %s and %s", node.Token.Type.String(), leftType.String(), rightType.String())
		return TypeBool
	}
	a.errorAt(node, "unsupported operator '%s'", node.Token.Type.String())
	return TypeError
}

func (a *Analyzer) analyzeList(node *ast.TreeNode) Type {
	if len(node.Children) == 0 {
		return &ListType{ElementType: TypeAny}
	}
	elemType := a.Analyze(node.Children[0])
	for _, child := range node.Children[1:] {
		childType := a.Analyze(child)
		elemType = MergeTypes(elemType, childType)
	}
	return &ListType{ElementType: elemType}
}

func (a *Analyzer) analyzeVector(node *ast.TreeNode) Type {
	if len(node.Children) == 0 {
		return &VectorType{ElementType: TypeFloat}
	}
	var elemType Type
	for _, child := range node.Children {
		childType := a.Analyze(child)
		if isUnknownType(childType) {
			elemType = MergeTypes(elemType, childType)
			continue
		}
		if !childType.Equals(TypeInt) && !childType.Equals(TypeFloat) && !childType.Equals(TypeString) {
			a.errorAt(child, "vector elements must be homogeneous int, float, or str, got %s", childType.String())
			elemType = MergeTypes(elemType, TypeAny)
			continue
		}
		if elemType == nil || elemType.Equals(TypeAny) {
			elemType = childType
			continue
		}
		if !elemType.Equals(childType) {
			a.errorAt(child, "vector literal requires homogeneous element types; found %s and %s", elemType.String(), childType.String())
			elemType = TypeAny
		}
	}
	if elemType == nil {
		return &VectorType{ElementType: TypeFloat}
	}
	return &VectorType{ElementType: elemType}
}

func (a *Analyzer) analyzeDict(node *ast.TreeNode) Type {
	if len(node.Children) == 0 {
		return &DictType{KeyType: TypeString, ValueType: TypeAny}
	}
	seenKeys := make(map[string]struct{})
	var valueType Type
	for _, pair := range node.Children {
		if pair == nil || len(pair.Children) < 2 {
			a.errorAt(node, "invalid dict entry")
			continue
		}
		keyNode := pair.Children[0]
		valueNode := pair.Children[1]
		keyType := a.Analyze(keyNode)
		if !keyType.Equals(TypeString) && !isUnknownType(keyType) {
			a.errorAt(keyNode, "dict keys must be str, got %s", keyType.String())
		}
		if keyNode != nil && keyNode.Token != nil && keyNode.Token.Type == token.STRING {
			key := keyNode.Token.Literal
			if _, exists := seenKeys[key]; exists {
				a.errorAt(keyNode, "duplicate dict key '%s'", key)
			} else {
				seenKeys[key] = struct{}{}
			}
		}
		childType := a.Analyze(valueNode)
		if valueType == nil {
			valueType = childType
		} else {
			valueType = MergeTypes(valueType, childType)
		}
	}
	if valueType == nil {
		valueType = TypeAny
	}
	return &DictType{KeyType: TypeString, ValueType: valueType}
}

func (a *Analyzer) analyzeIndex(node *ast.TreeNode) Type {
	if len(node.Children) < 2 {
		return TypeAny
	}
	targetType := a.Analyze(node.Children[0])
	indexType := a.Analyze(node.Children[1])
	if vecType, ok := targetType.(*VectorType); ok {
		if isIntLike(indexType) || isUnknownType(indexType) {
			return vecType.ElementType
		}
		if idxVec, ok := indexType.(*VectorType); ok {
			if idxVec.ElementType.Equals(TypeBool) || isUnknownType(idxVec.ElementType) {
				return vecType
			}
			a.errorAt(node.Children[1], "vector mask index requires bool vector, got %s", indexType.String())
			return vecType
		}
		a.errorAt(node.Children[1], "vector index must be int or bool vector, got %s", indexType.String())
		return TypeError
	}
	if listType, ok := targetType.(*ListType); ok {
		if !isIntLike(indexType) && !isUnknownType(indexType) {
			a.errorAt(node.Children[1], "list index must be int, got %s", indexType.String())
		}
		return listType.ElementType
	}
	if targetType.Equals(TypeString) {
		if !isIntLike(indexType) && !isUnknownType(indexType) {
			a.errorAt(node.Children[1], "string index must be int, got %s", indexType.String())
		}
		return TypeString
	}
	if _, ok := targetType.(*DictType); ok {
		a.errorAt(node, "use dot access for dicts: d.key instead of d['key']")
		return TypeError
	}
	if IsErrorType(targetType) {
		return TypeError
	}
	if !isUnknownType(targetType) {
		a.errorAt(node, "type '%s' is not indexable", targetType.String())
		return TypeError
	}
	return TypeAny
}

func (a *Analyzer) analyzeVarDecl(node *ast.TreeNode) Type {
	if len(node.Children) < 3 {
		a.errorAt(node, "invalid typed declaration")
		return TypeError
	}
	nameNode := node.Children[0]
	typeNode := node.Children[1]
	valueNode := node.Children[2]
	varName := nameNode.TokenLiteral()
	declType := a.resolveTypeNode(typeNode)
	valueType := a.Analyze(valueNode)
	if _, srcIsResult := valueType.(*ResultType); srcIsResult {
		if _, dstIsResult := declType.(*ResultType); !dstIsResult && !isUnknownType(declType) {
			a.errorAt(nameNode, "[C-TYPE] cannot assign result to '%s'; use 'unwrap()' or 'when' to extract the value", declType.String())
			a.currentScope.Define(varName, declType, true)
			return declType
		}
	}
	if !CanAssign(declType, valueType) && !isUnknownType(valueType) {
		a.errorAt(nameNode, "cannot assign value of type '%s' to '%s'", valueType.String(), declType.String())
	}
	if existing := a.currentScope.LookupLocal(varName); existing != nil {
		a.errorAt(nameNode, "symbol '%s' already defined in this scope", varName)
		return declType
	}
	a.currentScope.Define(varName, declType, true)
	return declType
}
