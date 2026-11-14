package types

import (
	"quark/ast"
	"quark/token"
)

func (a *Analyzer) inferBuiltinReturnType(name string, argTypes []Type, callNode *ast.TreeNode) Type {
	switch name {
	case "input":
		// ok(line) or err('end of input'): a line is always a str.
		return &ResultType{OkType: TypeString, ErrType: TypeString}
	case "unwrap":
		if len(argTypes) >= 1 {
			if res, ok := argTypes[0].(*ResultType); ok {
				return res.OkType
			}
			if !isUnknownType(argTypes[0]) {
				a.errorAt(callNode, "argument 1 of 'unwrap' expects result, got %s", argTypes[0].String())
			}
		}
		return TypeAny
	case "is_ok", "is_err":
		if len(argTypes) >= 1 {
			if _, ok := argTypes[0].(*ResultType); !ok && !isUnknownType(argTypes[0]) {
				a.errorAt(callNode, "argument 1 of '%s' expects result, got %s", name, argTypes[0].String())
			}
		}
		return TypeBool
	case "len":
		if len(argTypes) >= 1 && !isUnknownType(argTypes[0]) {
			t := argTypes[0]
			_, isList := t.(*ListType)
			_, isDict := t.(*DictType)
			_, isVec := t.(*VectorType)
			isStr := t.Equals(TypeString)
			if !isList && !isDict && !isVec && !isStr {
				a.errorAt(callNode, "argument 1 of 'len' expects str, list, dict, or vec, got %s", t.String())
			}
		}
		return TypeInt
	case "abs":
		if len(argTypes) >= 1 && !isUnknownType(argTypes[0]) {
			t := argTypes[0]
			if IsNumeric(t) {
				return t
			}
			a.errorAt(callNode, "argument 1 of 'abs' expects int or float, got %s", t.String())
			return TypeError
		}
		return TypeAny
	case "sum":
		// Like the table aggregate (specs/tables_v0_1_spec.md D13): an int for int
		// and bool vectors, a float for float vectors.
		if len(argTypes) >= 1 && !isUnknownType(argTypes[0]) {
			t := argTypes[0]
			switch st := t.(type) {
			case *VectorType:
				switch el := st.ElementType; {
				case el.Equals(TypeInt), el.Equals(TypeBool):
					return TypeInt
				case el.Equals(TypeFloat):
					return TypeFloat
				case !isUnknownType(el):
					a.errorAt(callNode, "argument 1 of 'sum' expects numeric or bool vec, got %s", t.String())
					return TypeError
				}
			default:
				a.errorAt(callNode, "argument 1 of 'sum' expects numeric or bool vec, got %s", t.String())
				return TypeError
			}
		}
		return TypeAny
	case "min", "max":
		// min(v) / max(v) of a vec keep its element type; a vec with no present
		// values is a runtime error, never a null typed as the element (D13).
		if len(argTypes) == 1 && !isUnknownType(argTypes[0]) {
			t := argTypes[0]
			if vec, isVec := t.(*VectorType); isVec {
				el := vec.ElementType
				if isUnknownType(el) {
					return TypeAny
				}
				if IsNumeric(el) || el.Equals(TypeBool) || el.Equals(TypeString) {
					return el
				}
				a.errorAt(callNode, "argument 1 of '%s' with single argument expects a vec, got %s", name, t.String())
				return TypeError
			}
			a.errorAt(callNode, "argument 1 of '%s' with single argument expects a vec, got %s", name, t.String())
			return TypeAny
		}
		if len(argTypes) == 2 {
			for i, t := range argTypes {
				if !IsNumeric(t) && !isUnknownType(t) {
					a.errorAt(callNode, "argument %d of '%s' expects numeric, got %s", i+1, name, t.String())
				}
			}
		}
		if sig, ok := a.builtins[name]; ok {
			return sig.Type.ReturnType
		}
		return TypeAny
	}

	if sig, ok := a.builtins[name]; ok {
		return sig.Type.ReturnType
	}
	return TypeAny
}

// inferToVectorType is the result type of xs.to_vec(): a typed vector when
// the list's elements are all int, all float, all bool or all str (nulls allowed).
func (a *Analyzer) inferToVectorType(srcType Type, callNode *ast.TreeNode) Type {
	if vec, ok := srcType.(*VectorType); ok {
		return &VectorType{ElementType: vec.ElementType}
	}

	listType, ok := srcType.(*ListType)
	if !ok {
		if !isUnknownType(srcType) {
			a.errorAt(callNode, "to_vec expects list or vec input, got %s", srcType.String())
		}
		return TypeAny
	}

	elem := listType.ElementType
	if elem.Equals(TypeInt) {
		return &VectorType{ElementType: TypeInt}
	}
	if elem.Equals(TypeFloat) {
		return &VectorType{ElementType: TypeFloat}
	}
	if elem.Equals(TypeBool) {
		return &VectorType{ElementType: TypeBool}
	}
	if elem.Equals(TypeString) {
		return &VectorType{ElementType: TypeString}
	}
	if union, ok := elem.(*UnionType); ok {
		onlyAllowed := true
		hasInt := false
		hasFloat := false
		hasBool := false
		hasString := false
		for _, opt := range union.Options {
			if opt.Equals(TypeInt) {
				hasInt = true
				continue
			}
			if opt.Equals(TypeFloat) {
				hasFloat = true
				continue
			}
			if opt.Equals(TypeBool) {
				hasBool = true
				continue
			}
			if opt.Equals(TypeString) {
				hasString = true
				continue
			}
			if opt.Equals(TypeNull) {
				continue
			}
			onlyAllowed = false
			break
		}
		if onlyAllowed {
			kinds := 0
			if hasInt {
				kinds++
			}
			if hasFloat {
				kinds++
			}
			if hasBool {
				kinds++
			}
			if hasString {
				kinds++
			}
			if kinds > 1 {
				a.errorAt(callNode, "to_vec requires homogeneous list elements (all int, all float, all bool, or all str)")
				return TypeAny
			}
			if hasInt && !hasFloat {
				return &VectorType{ElementType: TypeInt}
			}
			if hasFloat && !hasInt {
				return &VectorType{ElementType: TypeFloat}
			}
			if hasBool {
				return &VectorType{ElementType: TypeBool}
			}
			if hasString {
				return &VectorType{ElementType: TypeString}
			}
			return &VectorType{ElementType: TypeInt}
		}
	}

	if isUnknownType(elem) {
		return TypeAny
	}

	a.errorAt(callNode, "to_vec requires list elements of type int, float, bool, or str, got %s", elem.String())
	return TypeAny
}

func (a *Analyzer) validateReturnType(funcType *FunctionType, inferredType Type, funcName string, errorNode *ast.TreeNode) {
	if funcType.AnnotatedReturnType == nil {
		return
	}
	if errorNode != nil {
		a.returnValidated[errorNode] = true
	}
	annotated := funcType.AnnotatedReturnType
	funcType.ReturnType = annotated
	if annotated.Equals(TypeAny) {
		escapePending(inferredType)
		return
	}
	// A body that can end without a value (an if without else, a when
	// without a matching arm) returns null, which no annotation but any
	// accepts.
	if u, ok := inferredType.(*UnionType); ok {
		rest := make([]Type, 0, len(u.Options))
		for _, opt := range u.Options {
			if !opt.Equals(TypeVoid) {
				rest = append(rest, opt)
			}
		}
		if len(rest) < len(u.Options) {
			a.errorAt(errorNode, "function '%s' declares return type '%s' but can end without a value (an if without else, or a when with no matching arm, returns null); give every branch a value", funcName, annotated.String())
			return
		}
	}
	// Otherwise a body of unknown type is checked when it returns (lowering emits q_ret_*).
	if isUnknownType(inferredType) {
		return
	}
	SettleContainers(annotated, inferredType)
	if !CanAssign(annotated, inferredType) {
		a.errorAt(errorNode, "function '%s' declares return type '%s' but body returns '%s'%s", funcName, annotated.String(), inferredType.String(), mismatchDetail(annotated, inferredType))
	}
}

func (a *Analyzer) validateDefaultValues(specs []paramSpec, funcType *FunctionType, errorNode *ast.TreeNode) {
	for i, spec := range specs {
		if spec.defaultValue == nil {
			continue
		}
		defaultType := inferLiteralType(spec.defaultValue)
		// inferLiteralType reads `= null` as any (an unannotated `x = null`
		// accepts anything); against an annotation it is checked as null, so
		// `x: int = null` or `r: result = null` fails here, not at every call.
		if lit := spec.defaultValue; lit.NodeType == ast.LiteralNode && lit.Token != nil && lit.Token.Type == token.NULL {
			defaultType = TypeNull
		}
		if i >= len(funcType.ParamTypes) {
			continue
		}
		paramType := funcType.ParamTypes[i]
		if paramType.Equals(TypeAny) || isUnknownType(paramType) {
			continue
		}
		if isUnknownType(defaultType) {
			continue
		}
		if !CanAssign(paramType, defaultType) {
			a.errorAt(errorNode, "default value type '%s' doesn't match parameter type '%s' for parameter '%s'", defaultType.String(), paramType.String(), spec.name)
		}
	}
}
