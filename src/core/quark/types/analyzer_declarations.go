package types

import (
	"quark/ast"
	"quark/token"
)

func (a *Analyzer) predeclareFunctions(nodes []*ast.TreeNode) {
	for _, child := range nodes {
		if child == nil {
			continue
		}
		if child.NodeType == ast.FunctionNode {
			a.declareFunctionSignature(child)
			continue
		}
		if isFunctionBindingAssignment(child) {
			a.declareFunctionAssignmentSignature(child)
		}
	}
}

func isFunctionBindingAssignment(node *ast.TreeNode) bool {
	if node == nil || node.NodeType != ast.OperatorNode || node.Token == nil || node.Token.Type != token.EQUALS {
		return false
	}
	if len(node.Children) != 2 {
		return false
	}
	left := node.Children[0]
	right := node.Children[1]
	if left == nil || right == nil {
		return false
	}
	return left.NodeType == ast.IdentifierNode && right.NodeType == ast.LambdaNode
}

func functionTypeFromLambdaNode(lambdaNode *ast.TreeNode) *FunctionType {
	return functionTypeFromLambdaNodeWithStructs(lambdaNode, nil)
}

func functionTypeFromLambdaNodeWithStructs(lambdaNode *ast.TreeNode, structTypes map[string]*StructType) *FunctionType {
	if lambdaNode == nil || len(lambdaNode.Children) < 1 {
		return &FunctionType{ParamTypes: []Type{}, ReturnType: TypeAny}
	}
	argsNode := lambdaNode.Children[0]
	paramSpecs := collectParamSpecs(argsNode)
	paramTypes := make([]Type, len(paramSpecs))
	defaultCount := 0
	defaultValues := make([]*DefaultValueInfo, len(paramSpecs))
	for i, spec := range paramSpecs {
		if spec.typeNode == nil {
			if spec.defaultValue != nil {
				paramTypes[i] = inferLiteralType(spec.defaultValue)
			} else {
				paramTypes[i] = TypeAny
			}
		} else {
			typeName := spec.typeNode.TokenLiteral()
			switch typeName {
			case "int":
				paramTypes[i] = TypeInt
			case "float":
				paramTypes[i] = TypeFloat
			case "str":
				paramTypes[i] = TypeString
			case "bool":
				paramTypes[i] = TypeBool
			case "null":
				paramTypes[i] = TypeNull
			case "list":
				paramTypes[i] = &ListType{ElementType: TypeAny}
			case "dict":
				paramTypes[i] = &DictType{KeyType: TypeAny, ValueType: TypeAny}
			case "vector":
				paramTypes[i] = &VectorType{ElementType: TypeAny}
			case "resource":
				paramTypes[i] = TypeResource
			case "file_handle":
				paramTypes[i] = TypeFileHandle
			default:
				if structTypes != nil {
					if st, ok := structTypes[typeName]; ok {
						paramTypes[i] = st
						break
					}
				}
				paramTypes[i] = TypeAny
			}
		}
		if spec.defaultValue != nil {
			defaultCount++
			defaultValues[i] = &DefaultValueInfo{Node: spec.defaultValue}
		}
	}

	var annotatedReturnType Type
	if lambdaNode.ReturnType != nil {
		annotatedReturnType = resolveTypeNodeStaticWithStructs(lambdaNode.ReturnType, structTypes)
	}

	var returnType Type = TypeAny
	if annotatedReturnType != nil {
		returnType = annotatedReturnType
	}

	return &FunctionType{
		ParamTypes:          paramTypes,
		ReturnType:          returnType,
		AnnotatedReturnType: annotatedReturnType,
		DefaultCount:        defaultCount,
		DefaultValues:       defaultValues,
	}
}

func resolveTypeNodeStatic(node *ast.TreeNode) Type {
	return resolveTypeNodeStaticWithStructs(node, nil)
}

func resolveTypeNodeStaticWithStructs(node *ast.TreeNode, structTypes map[string]*StructType) Type {
	if node == nil || node.NodeType != ast.TypeNode {
		return nil
	}
	name := node.TokenLiteral()
	switch name {
	case "int":
		return TypeInt
	case "float":
		return TypeFloat
	case "str":
		return TypeString
	case "bool":
		return TypeBool
	case "null":
		return TypeNull
	case "any":
		return TypeAny
	case "list":
		return &ListType{ElementType: TypeAny}
	case "dict":
		return &DictType{KeyType: TypeAny, ValueType: TypeAny}
	case "vector":
		return &VectorType{ElementType: TypeAny}
	case "result":
		return &ResultType{OkType: TypeAny, ErrType: TypeAny}
	case "resource":
		return TypeResource
	case "file_handle":
		return TypeFileHandle
	default:
		if structTypes != nil {
			if st, ok := structTypes[name]; ok {
				return st
			}
		}
		return TypeAny
	}
}

func inferLiteralType(node *ast.TreeNode) Type {
	if node == nil {
		return TypeAny
	}
	if node.NodeType == ast.LiteralNode && node.Token != nil {
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
			return TypeAny
		}
	}
	if node.NodeType == ast.OperatorNode && node.Token != nil && node.Token.Type == token.MINUS && len(node.Children) == 1 {
		return inferLiteralType(node.Children[0])
	}
	if node.NodeType == ast.ListNode {
		return &ListType{ElementType: TypeAny}
	}
	return TypeAny
}

func (a *Analyzer) declareFunctionAssignmentSignature(node *ast.TreeNode) *FunctionType {
	if !isFunctionBindingAssignment(node) {
		return nil
	}

	nameNode := node.Children[0]
	lambdaNode := node.Children[1]
	funcName := nameNode.TokenLiteral()
	if funcName == "" {
		return nil
	}

	if existing := a.currentScope.LookupLocal(funcName); existing != nil {
		if ft, ok := existing.Type.(*FunctionType); ok {
			return ft
		}
		a.errorAt(nameNode, "symbol '%s' already defined and is not a function", funcName)
		return nil
	}

	funcType := functionTypeFromLambdaNodeWithStructs(lambdaNode, a.structTypes)
	a.currentScope.Define(funcName, funcType, true)
	a.functions[funcName] = funcType
	return funcType
}

func (a *Analyzer) declareFunctionSignature(node *ast.TreeNode) *FunctionType {
	if len(node.Children) < 2 {
		return nil
	}
	nameNode := node.Children[0]
	argsNode := node.Children[1]
	funcName := nameNode.TokenLiteral()
	if funcName == "" {
		return nil
	}
	if existing := a.currentScope.LookupLocal(funcName); existing != nil {
		a.errorAt(nameNode, "symbol '%s' already defined in this scope", funcName)
		if ft, ok := existing.Type.(*FunctionType); ok {
			return ft
		}
		return nil
	}
	paramSpecs := collectParamSpecs(argsNode)
	paramTypes := make([]Type, len(paramSpecs))
	defaultCount := 0
	defaultValues := make([]*DefaultValueInfo, len(paramSpecs))
	for i, spec := range paramSpecs {
		if spec.typeNode != nil {
			paramTypes[i] = a.resolveTypeNode(spec.typeNode)
		} else if spec.defaultValue != nil {
			paramTypes[i] = inferLiteralType(spec.defaultValue)
		} else {
			paramTypes[i] = TypeAny
		}
		if spec.defaultValue != nil {
			defaultCount++
			defaultValues[i] = &DefaultValueInfo{Node: spec.defaultValue}
		}
	}

	funcType := &FunctionType{
		ParamTypes:    paramTypes,
		ReturnType:    TypeAny,
		DefaultCount:  defaultCount,
		DefaultValues: defaultValues,
	}
	a.currentScope.Define(funcName, funcType, false)
	a.functions[funcName] = funcType
	return funcType
}

func collectParamSpecs(argsNode *ast.TreeNode) []paramSpec {
	if argsNode == nil {
		return nil
	}
	specs := make([]paramSpec, 0, len(argsNode.Children))
	for _, child := range argsNode.Children {
		if child == nil {
			continue
		}
		switch child.NodeType {
		case ast.ParameterNode:
			nameNode := (*ast.TreeNode)(nil)
			typeNode := (*ast.TreeNode)(nil)
			if len(child.Children) > 0 {
				nameNode = child.Children[0]
			}
			if len(child.Children) > 1 {
				typeNode = child.Children[1]
			}
			name := ""
			if nameNode != nil {
				name = nameNode.TokenLiteral()
			}
			specs = append(specs, paramSpec{name: name, typeNode: typeNode, defaultValue: child.DefaultValue})
		case ast.IdentifierNode:
			name := child.TokenLiteral()
			specs = append(specs, paramSpec{name: name})
		}
	}
	return specs
}
