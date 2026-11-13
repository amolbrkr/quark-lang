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
	a.refs[node] = sym
	if f := a.fixedFnOf(sym); f != nil {
		a.fixedRefs[node] = f
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

// analyzeMember analyzes `object.name`: a dict key or a struct field. Method
// calls (`object.name(...)`) are planned by resolveMethodCall instead.
func (a *Analyzer) analyzeMember(node *ast.TreeNode) Type {
	if len(node.Children) < 2 || node.Children[0] == nil || node.Children[1] == nil {
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
	case *StructType:
		field, _ := t.FieldByName(member)
		if field == nil {
			a.errorAt(node, "struct %s has no field '%s'", t.Name, member)
			return TypeError
		}
		return field.Type
	default:
		if isUnknownType(targetType) {
			return TypeAny
		}
		a.errorAt(node, "dot access is only supported on dict; use len(entity), upper(entity), etc. instead of entity.%s", member)
		return TypeError
	}
}

// analyzeUnary analyzes `-x` and `!x`.
func (a *Analyzer) analyzeUnary(node *ast.TreeNode) Type {
	if node.Token == nil || len(node.Children) != 1 || node.Children[0] == nil {
		a.errorAt(node, "malformed operator expression")
		return TypeError
	}
	operandType := a.Analyze(node.Children[0])
	if IsErrorType(operandType) {
		return TypeError
	}
	switch node.Token.Type {
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
		// Uses truthiness (§4.3). Always returns bool.
		a.checkCondition(node.Children[0], operandType)
		return TypeBool
	}
	a.errorAt(node, "unsupported operator '%s'", node.Token.Type.String())
	return TypeError
}

// analyzeAssign analyzes `target = value`, dispatching on the target: a
// name, a member (`d.key = v`; struct fields are immutable) or an index
// (`l[i] = v`).
func (a *Analyzer) analyzeAssign(node *ast.TreeNode) Type {
	if len(node.Children) != 2 || node.Children[0] == nil || node.Children[1] == nil {
		a.errorAt(node, "malformed assignment")
		return TypeError
	}
	target, value := node.Children[0], node.Children[1]
	switch target.NodeType {
	case ast.IdentifierNode:
		return a.assignName(target, value)
	case ast.MemberNode:
		return a.assignMember(target, value)
	case ast.IndexNode:
		return a.assignIndex(target, value)
	case ast.AssignNode:
		// `a = b = 1` parses as `(a = b) = 1`; say what is actually wrong.
		rightType := a.Analyze(value)
		a.errorAt(target, "chained assignment is not supported; assign each variable separately (a = 1, then b = 1)")
		return rightType
	default:
		rightType := a.Analyze(value)
		a.errorAt(target, "left side of assignment must be an identifier")
		return rightType
	}
}

// assignMember analyzes `object.name = value`.
func (a *Analyzer) assignMember(target, value *ast.TreeNode) Type {
	if len(target.Children) < 2 {
		a.errorAt(target, "malformed dot access expression")
		return TypeError
	}
	objectType := a.Analyze(target.Children[0])
	if st, ok := objectType.(*StructType); ok {
		a.errorAt(target, "cannot assign to field '%s' of immutable struct %s", target.Children[1].TokenLiteral(), st.Name)
		a.Analyze(value) // still analyze the value for its own errors
		return TypeError
	}
	rightType := a.Analyze(value)
	if objectType.Equals(TypeNull) {
		a.errorAt(target.Children[0], "cannot assign member on null")
		return rightType
	}
	if dictType, ok := objectType.(*DictType); ok {
		if !isUnknownType(rightType) && !IsErrorType(rightType) && !isUnknownType(dictType.ValueType) && !CanAssign(dictType.ValueType, rightType) {
			a.warnAt(target, "assigning '%s' to dict with value type '%s'", rightType.String(), dictType.ValueType.String())
		}
		return rightType
	}
	if !isUnknownType(objectType) {
		a.errorAt(target, "only dict members are assignable")
	}
	return rightType
}

// assignIndex analyzes `object[index] = value`.
func (a *Analyzer) assignIndex(target, value *ast.TreeNode) Type {
	rightType := a.Analyze(value)
	if len(target.Children) < 2 {
		return rightType
	}
	targetType := a.Analyze(target.Children[0])
	indexType := a.Analyze(target.Children[1])
	switch {
	case targetType.Equals(TypeNull):
		a.errorAt(target.Children[0], "cannot index null")
	case targetType.Equals(TypeString):
		a.errorAt(target, "strings are immutable")
	default:
		switch targetType.(type) {
		case *ListType:
			if !isIntLike(indexType) && !isUnknownType(indexType) {
				a.errorAt(target.Children[1], "list index must be int, got %s", indexType.String())
			}
		case *DictType:
			a.errorAt(target, "use dot access for dict assignment: d.key = value")
		default:
			if !isUnknownType(targetType) {
				a.errorAt(target, "type '%s' is not index-assignable", targetType.String())
			}
		}
	}
	return rightType
}

// assignName analyzes `name = value`: the first assignment defines the
// variable, later ones must fit its type.
func (a *Analyzer) assignName(target, value *ast.TreeNode) Type {
	varName := target.TokenLiteral()
	// `fn len(x) -> ...` or `len = ...` would rebind a builtin or extern fn,
	// but calls by that name always reach the builtin, so the new definition
	// would be silently ignored. (Parameters, loop variables and nested
	// functions can shadow one.)
	if a.refersToBuiltin(a.currentScope, varName) {
		if _, isExtern := a.externFns[varName]; isExtern {
			a.errorAt(target, "'%s' is an extern fn and can't be redefined or assigned; choose another name", varName)
		} else {
			a.errorAt(target, "'%s' is a builtin function and can't be redefined or assigned; choose another name", varName)
		}
		a.Analyze(value)
		return TypeError
	}
	// A declared function is fixed (rule 19).
	if f := a.fixedFnOf(a.currentScope.Lookup(varName)); f != nil {
		a.reassignedDeclError(target, f)
		a.Analyze(value)
		return TypeError
	}
	if value.NodeType == ast.LambdaNode {
		// A variable holding a function: defined before the lambda is
		// analyzed so the lambda can call itself through it.
		if a.currentScope.LookupLocal(varName) == nil {
			funcType := functionTypeFromLambdaNodeWithStructs(value, a.structTypes)
			a.recordDef(a.declareSym(varName, funcType, true, SymLocal, nil, target), target)
		}
		a.pendingFuncName = varName
	}
	rightType := a.Analyze(value)
	sym := a.currentScope.Lookup(varName)
	if sym == nil {
		a.recordDef(a.declareSym(varName, rightType, true, SymLocal, rightType, target), target)
		return rightType
	}
	a.refs[target] = sym
	a.markParamReassigned(sym)
	if _, dstIsFunc := sym.Type.(*FunctionType); dstIsFunc {
		if srcFunc, srcIsFunc := rightType.(*FunctionType); srcIsFunc {
			sym.Type = srcFunc
			return rightType
		}
	}
	if isUnknownType(rightType) && !sym.Type.Equals(TypeAny) {
		a.errorAt(target, "cannot assign value of unknown type to '%s'", sym.Type.String())
		return rightType
	}
	if _, srcIsResult := rightType.(*ResultType); srcIsResult {
		if _, dstIsResult := sym.Type.(*ResultType); !dstIsResult && !isUnknownType(sym.Type) {
			a.errorAt(target, "[C-TYPE] cannot assign result to '%s'; use 'unwrap()' or 'when' to extract the value", sym.Type.String())
			return rightType
		}
	}
	if !CanAssign(sym.Type, rightType) && !isUnknownType(rightType) {
		a.errorAt(target, "cannot assign value of type '%s' to '%s'", rightType.String(), sym.Type.String())
		return rightType
	}
	if !isUnknownType(rightType) && sym.Type.Equals(TypeAny) {
		sym.Type = rightType
	}
	return rightType
}

// analyzeBinary analyzes arithmetic, comparison and `and`/`or`.
func (a *Analyzer) analyzeBinary(node *ast.TreeNode) Type {
	if node.Token == nil || len(node.Children) != 2 || node.Children[0] == nil || node.Children[1] == nil {
		a.errorAt(node, "malformed operator expression")
		return TypeError
	}
	op := node.Token.Type
	leftType := a.Analyze(node.Children[0])
	rightType := a.Analyze(node.Children[1])
	// Propagate ErrorType silently
	if IsErrorType(leftType) || IsErrorType(rightType) {
		return TypeError
	}
	if t, ok := a.analyzeVectorArith(node, leftType, rightType); ok {
		return t
	}
	switch op {
	case token.PLUS, token.MINUS, token.MULTIPLY, token.DIVIDE, token.MODULO, token.DOUBLESTAR:
		return a.analyzeArith(node, leftType, rightType)
	case token.LT, token.LTE, token.GT, token.GTE:
		return a.analyzeOrdering(node, leftType, rightType)
	case token.DEQ, token.NE:
		return a.analyzeEquality(node, leftType, rightType)
	case token.AND, token.OR:
		// Short-circuit via truthiness (Python semantics).
		a.checkCondition(node.Children[0], leftType)
		a.checkCondition(node.Children[1], rightType)
		// Return type is the union of both operand types since the result
		// is one of the two operands (not necessarily bool).
		if isBoolLike(leftType) && isBoolLike(rightType) {
			return TypeBool
		}
		return MergeTypes(leftType, rightType)
	}
	a.errorAt(node, "unsupported operator '%s'", node.Token.Type.String())
	return TypeError
}

func isNumericScalar(t Type) bool { return t.Equals(TypeInt) || t.Equals(TypeFloat) }

// analyzeVectorArith types `+ - * /` with a vec operand (elementwise, or
// broadcast against a number). ok is false when the operation isn't one.
func (a *Analyzer) analyzeVectorArith(node *ast.TreeNode, leftType, rightType Type) (Type, bool) {
	op := node.Token.Type
	if op != token.PLUS && op != token.MINUS && op != token.MULTIPLY && op != token.DIVIDE {
		return nil, false
	}
	leftVec, leftIsVec := leftType.(*VectorType)
	rightVec, rightIsVec := rightType.(*VectorType)
	numericElems := func(v *VectorType, t Type) bool {
		if !IsNumeric(v.ElementType) && !isUnknownType(v.ElementType) {
			a.errorAt(node, "operator '%s' requires numeric vec operands, got %s", op.String(), t.String())
			return false
		}
		return true
	}
	switch {
	case leftIsVec && rightIsVec:
		if !numericElems(leftVec, leftType) || !numericElems(rightVec, rightType) {
			return TypeError, true
		}
		if leftVec.ElementType.Equals(TypeFloat) || rightVec.ElementType.Equals(TypeFloat) {
			return &VectorType{ElementType: TypeFloat}, true
		}
		if leftVec.ElementType.Equals(TypeInt) && rightVec.ElementType.Equals(TypeInt) {
			if op == token.DIVIDE {
				return &VectorType{ElementType: TypeFloat}, true
			}
			return &VectorType{ElementType: TypeInt}, true
		}
		return &VectorType{ElementType: TypeAny}, true
	case leftIsVec && (isNumericScalar(rightType) || isUnknownType(rightType)):
		if !numericElems(leftVec, leftType) {
			return TypeError, true
		}
		if op == token.DIVIDE && leftVec.ElementType.Equals(TypeInt) && rightType.Equals(TypeInt) {
			return &VectorType{ElementType: TypeFloat}, true
		}
		return leftType, true
	case rightIsVec && (isNumericScalar(leftType) || isUnknownType(leftType)):
		if !numericElems(rightVec, rightType) {
			return TypeError, true
		}
		if op == token.DIVIDE && rightVec.ElementType.Equals(TypeInt) && leftType.Equals(TypeInt) {
			return &VectorType{ElementType: TypeFloat}, true
		}
		return rightType, true
	}
	return nil, false
}

// analyzeArith types `+ - * / % **` on scalars (and `+` on strings).
func (a *Analyzer) analyzeArith(node *ast.TreeNode, leftType, rightType Type) Type {
	op := node.Token.Type
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
		if op == token.DIVIDE || leftType.Equals(TypeFloat) || rightType.Equals(TypeFloat) {
			return TypeFloat
		}
		return TypeInt
	}
	if isUnknownType(leftType) || isUnknownType(rightType) {
		return TypeAny
	}
	a.errorAt(node, "operator '%s' requires numeric operands, got %s and %s", op.String(), leftType.String(), rightType.String())
	return TypeError
}

// analyzeOrdering types `< <= > >=`.
func (a *Analyzer) analyzeOrdering(node *ast.TreeNode, leftType, rightType Type) Type {
	leftVec, leftIsVec := leftType.(*VectorType)
	rightVec, rightIsVec := rightType.(*VectorType)
	if leftIsVec || rightIsVec {
		if leftIsVec && !IsNumeric(leftVec.ElementType) && !isUnknownType(leftVec.ElementType) {
			a.errorAt(node, "ordering comparison requires numeric vec, got %s", leftType.String())
		}
		if rightIsVec && !IsNumeric(rightVec.ElementType) && !isUnknownType(rightVec.ElementType) {
			a.errorAt(node, "ordering comparison requires numeric vec, got %s", rightType.String())
		}
		if leftIsVec && !rightIsVec && !isNumericScalar(rightType) && !isUnknownType(rightType) {
			a.errorAt(node, "cannot compare vec with %s", rightType.String())
		}
		if rightIsVec && !leftIsVec && !isNumericScalar(leftType) && !isUnknownType(leftType) {
			a.errorAt(node, "cannot compare vec with %s", leftType.String())
		}
		return &VectorType{ElementType: TypeBool}
	}
	if (IsComparable(leftType) && IsComparable(rightType)) || isUnknownType(leftType) || isUnknownType(rightType) {
		return TypeBool
	}
	a.errorAt(node, "comparison requires comparable operands, got %s and %s", leftType.String(), rightType.String())
	return TypeBool
}

// analyzeEquality types `==` and `!=`.
func (a *Analyzer) analyzeEquality(node *ast.TreeNode, leftType, rightType Type) Type {
	op := node.Token.Type
	_, leftIsVec := leftType.(*VectorType)
	_, rightIsVec := rightType.(*VectorType)
	if leftIsVec || rightIsVec {
		return &VectorType{ElementType: TypeBool}
	}
	if a.checkStructEquality(node, leftType, rightType, op) {
		return TypeError
	}
	// A result is never null (null can't be stored where a result is
	// expected), so this comparison would be a constant: `r == null` never
	// detects an err, e.g. the end of input() in a read loop.
	_, leftIsResult := leftType.(*ResultType)
	_, rightIsResult := rightType.(*ResultType)
	if (leftIsResult && rightType.Equals(TypeNull)) || (rightIsResult && leftType.Equals(TypeNull)) {
		a.errorAt(node, "a result is never null, so this comparison is always %t; use is_ok(r), is_err(r) or when to check it", op == token.NE)
		return TypeError
	}
	return TypeBool
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
		if !childType.Equals(TypeInt) && !childType.Equals(TypeFloat) && !childType.Equals(TypeBool) && !childType.Equals(TypeString) {
			a.errorAt(child, "vec elements must be homogeneous int, float, bool, or str, got %s", childType.String())
			elemType = MergeTypes(elemType, TypeAny)
			continue
		}
		if elemType == nil || elemType.Equals(TypeAny) {
			elemType = childType
			continue
		}
		if !elemType.Equals(childType) {
			a.errorAt(child, "vec literal requires homogeneous element types; found %s and %s", elemType.String(), childType.String())
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
			a.errorAt(node.Children[1], "vec mask index requires bool vec, got %s", indexType.String())
			return vecType
		}
		a.errorAt(node.Children[1], "vec index must be int or bool vec, got %s", indexType.String())
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
			a.declareSym(varName, declType, true, SymLocal, declType, nameNode)
			return declType
		}
	}
	if !CanAssign(declType, valueType) && !isUnknownType(valueType) {
		a.errorAt(nameNode, "cannot assign value of type '%s' to '%s'", valueType.String(), declType.String())
	}
	if existing := a.currentScope.LookupLocal(varName); existing != nil {
		if f := a.fixedFnOf(existing); f != nil {
			a.reassignedDeclError(nameNode, f)
			return declType
		}
		a.errorAt(nameNode, "symbol '%s' already defined in this scope", varName)
		return declType
	}
	a.recordDef(a.declareSym(varName, declType, true, SymLocal, declType, nameNode), nameNode)
	return declType
}
