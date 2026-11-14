package types

import (
	"quark/ast"
	"quark/token"
)

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
		} else if t := resolveTypeNodeStaticWithStructs(spec.typeNode, structTypes); t != nil {
			paramTypes[i] = t
		} else {
			paramTypes[i] = TypeAny
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

// resolveTypeNodeStaticWithStructs resolves an annotation without reporting
// errors (the checker reports them where it analyzes the declaration); an
// annotation that names no valid type is any.
func resolveTypeNodeStaticWithStructs(node *ast.TreeNode, structTypes map[string]*StructType) Type {
	if node == nil || node.NodeType != ast.TypeNode {
		return nil
	}
	t, _, msg := annotationType(node, structTypes)
	if msg != "" {
		return TypeAny
	}
	return t
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
	if node.Op() == token.MINUS && len(node.Children) == 1 {
		return inferLiteralType(node.Children[0])
	}
	if node.NodeType == ast.ListNode {
		return &ListType{ElementType: TypeAny}
	}
	if node.NodeType == ast.DictNode {
		return &DictType{KeyType: TypeString, ValueType: TypeAny}
	}
	return TypeAny
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
			specs = append(specs, paramSpec{name: name, nameNode: nameNode, typeNode: typeNode, defaultValue: child.DefaultValue})
		case ast.IdentifierNode:
			name := child.TokenLiteral()
			specs = append(specs, paramSpec{name: name, nameNode: child})
		}
	}
	return specs
}
