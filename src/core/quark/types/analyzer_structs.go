package types

import (
	"quark/ast"
	"quark/token"
)

// predeclareStructTypes scans top-level children for struct definitions and
// registers them in the structTypes map so they can be referenced by type
// annotations and struct literals before the definition is fully analyzed.
func (a *Analyzer) predeclareStructTypes(children []*ast.TreeNode) {
	for _, child := range children {
		if child.NodeType == ast.StructDefNode {
			name := child.TokenLiteral()
			if _, exists := a.structTypes[name]; exists {
				a.errorAt(child, "struct '%s' already defined", name)
				continue
			}
			// Build field list from children (StructFieldNode nodes)
			fields := make([]StructField, 0, len(child.Children))
			seenFields := make(map[string]bool)
			for _, fieldNode := range child.Children {
				if fieldNode.NodeType != ast.StructFieldNode {
					continue
				}
				fieldName := fieldNode.TokenLiteral()
				if seenFields[fieldName] {
					a.errorAt(fieldNode, "duplicate field '%s' in struct '%s'", fieldName, name)
					continue
				}
				seenFields[fieldName] = true

				var fieldType Type = TypeAny
				if len(fieldNode.Children) > 0 && fieldNode.Children[0].NodeType == ast.TypeNode {
					fieldType = a.resolveTypeNode(fieldNode.Children[0])
				}

				sf := StructField{
					Name:       fieldName,
					Type:       fieldType,
					HasDefault: fieldNode.DefaultValue != nil,
				}
				if fieldNode.DefaultValue != nil {
					sf.DefaultNode = fieldNode.DefaultValue
				}
				fields = append(fields, sf)
			}

			st := &StructType{Name: name, Fields: fields}
			a.structTypes[name] = st
		}
	}
}

// analyzeStructDef validates a struct definition. The type is already registered
// by predeclareStructTypes; this pass validates default value types.
func (a *Analyzer) analyzeStructDef(node *ast.TreeNode) Type {
	name := node.TokenLiteral()
	st := a.structTypes[name]
	if st == nil {
		// Should not happen — predeclare runs first
		a.errorAt(node, "internal: struct '%s' not pre-declared", name)
		return TypeVoid
	}

	// Validate default expressions
	for i, fieldNode := range node.Children {
		if fieldNode.NodeType != ast.StructFieldNode {
			continue
		}
		if fieldNode.DefaultValue != nil && i < len(st.Fields) {
			defaultType := a.Analyze(fieldNode.DefaultValue)
			if !isUnknownType(defaultType) && !IsErrorType(defaultType) && !CanAssign(st.Fields[i].Type, defaultType) {
				a.errorAt(fieldNode, "default value type '%s' is not compatible with field type '%s'", defaultType.String(), st.Fields[i].Type.String())
			}
		}
	}

	return TypeVoid
}

// analyzeStructLiteral type-checks a struct literal: Name { field: expr, ... }
func (a *Analyzer) analyzeStructLiteral(node *ast.TreeNode) Type {
	name := node.TokenLiteral()
	st := a.structTypes[name]
	if st == nil {
		a.errorAt(node, "unknown struct type '%s'", name)
		return TypeError
	}

	providedFields := make(map[string]bool)

	for _, pair := range node.Children {
		if pair.NodeType != ast.StructFieldNode || len(pair.Children) < 2 {
			a.errorAt(pair, "malformed field initializer in struct literal")
			continue
		}
		fieldName := pair.TokenLiteral()

		// Check for duplicate
		if providedFields[fieldName] {
			a.errorAt(pair, "duplicate field '%s' in %s literal", fieldName, name)
			continue
		}
		providedFields[fieldName] = true

		// Check field exists in struct
		field, _ := st.FieldByName(fieldName)
		if field == nil {
			a.errorAt(pair, "unknown field '%s' in %s literal", fieldName, name)
			continue
		}

		// Type-check value expression
		valueNode := pair.Children[1]
		valueType := a.Analyze(valueNode)

		if !isUnknownType(valueType) && !IsErrorType(valueType) && !CanAssign(field.Type, valueType) {
			a.errorAt(pair, "cannot assign value of type '%s' to field '%s' of type '%s'", valueType.String(), fieldName, field.Type.String())
		}
	}

	// Check all required fields are provided
	for _, field := range st.Fields {
		if !providedFields[field.Name] && !field.HasDefault {
			a.errorAt(node, "missing required field '%s' in %s literal", field.Name, name)
		}
	}

	return st
}

// GetStructTypes returns the registered struct types for codegen.
func (a *Analyzer) GetStructTypes() map[string]*StructType {
	return a.structTypes
}

// isStructType returns true if t is a *StructType.
func isStructType(t Type) bool {
	_, ok := t.(*StructType)
	return ok
}

// checkStructEquality rejects == / != on struct operands.
func (a *Analyzer) checkStructEquality(node *ast.TreeNode, leftType, rightType Type, op token.TokenType) bool {
	if isStructType(leftType) || isStructType(rightType) {
		opStr := "=="
		if op == token.NE {
			opStr = "!="
		}
		a.errorAt(node, "operator '%s' is not defined for struct values; compare fields explicitly", opStr)
		return true
	}
	return false
}
