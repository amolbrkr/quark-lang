package types

import "quark/ast"

func (a *Analyzer) inferBuiltinReturnType(name string, argTypes []Type, callNode *ast.TreeNode) Type {
	switch name {
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
				a.errorAt(callNode, "argument 1 of 'len' expects str, list, dict, or vector, got %s", t.String())
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
		if len(argTypes) >= 1 && !isUnknownType(argTypes[0]) {
			t := argTypes[0]
			switch st := t.(type) {
			case *VectorType:
				if !IsNumeric(st.ElementType) && st.ElementType != TypeBool && !isUnknownType(st.ElementType) {
					a.errorAt(callNode, "argument 1 of 'sum' expects numeric or bool vector, got %s", t.String())
				}
			default:
				a.errorAt(callNode, "argument 1 of 'sum' expects numeric or bool vector, got %s", t.String())
			}
		}
		return TypeAny
	case "min", "max":
		if len(argTypes) == 1 && !isUnknownType(argTypes[0]) {
			t := argTypes[0]
			if vec, isVec := t.(*VectorType); isVec {
				if !IsNumeric(vec.ElementType) && !isUnknownType(vec.ElementType) {
					a.errorAt(callNode, "argument 1 of '%s' with single argument expects numeric vector, got %s", name, t.String())
				}
				return TypeFloat
			}
			a.errorAt(callNode, "argument 1 of '%s' with single argument expects numeric vector, got %s", name, t.String())
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
	case "concat":
		if len(argTypes) == 2 {
			t0, t1 := argTypes[0], argTypes[1]
			if !isUnknownType(t0) && !isUnknownType(t1) {
				_, t0List := t0.(*ListType)
				_, t1List := t1.(*ListType)
				bothStr := isStringLike(t0) && isStringLike(t1)
				bothList := t0List && t1List
				if !bothStr && !bothList {
					a.errorAt(callNode, "concat requires both arguments to be str+str or list+list, got %s and %s", t0.String(), t1.String())
				}
			}
		}
		if sig, ok := a.builtins[name]; ok {
			return sig.Type.ReturnType
		}
		return TypeAny
	}

	if name != "vfrom_list" {
		if sig, ok := a.builtins[name]; ok {
			return sig.Type.ReturnType
		}
		return TypeAny
	}

	if len(argTypes) != 1 {
		return TypeAny
	}

	srcType := argTypes[0]
	if vec, ok := srcType.(*VectorType); ok {
		return &VectorType{ElementType: vec.ElementType}
	}

	listType, ok := srcType.(*ListType)
	if !ok {
		if !isUnknownType(srcType) {
			a.errorAt(callNode, "to_vector expects list or vector input, got %s", srcType.String())
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
	if elem.Equals(TypeString) {
		return &VectorType{ElementType: TypeString}
	}
	if union, ok := elem.(*UnionType); ok {
		onlyAllowed := true
		hasInt := false
		hasFloat := false
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
			if hasString {
				kinds++
			}
			if kinds > 1 {
				a.errorAt(callNode, "to_vector requires homogeneous list elements (all int, all float, or all str)")
				return TypeAny
			}
			if hasInt && !hasFloat {
				return &VectorType{ElementType: TypeInt}
			}
			if hasFloat && !hasInt {
				return &VectorType{ElementType: TypeFloat}
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

	a.errorAt(callNode, "to_vector requires list elements of type int, float, or str, got %s", elem.String())
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
	if isUnknownType(inferredType) {
		funcType.ReturnType = annotated
		return
	}
	if !canReturnAs(annotated, inferredType) {
		a.errorAt(errorNode, "function '%s' declares return type '%s' but body returns '%s'", funcName, annotated.String(), inferredType.String())
	}
	funcType.ReturnType = annotated
}

func canReturnAs(annotated Type, inferred Type) bool {
	if CanAssign(annotated, inferred) {
		return true
	}
	if union, ok := inferred.(*UnionType); ok {
		for _, opt := range union.Options {
			if opt.Equals(TypeVoid) {
				continue
			}
			if !CanAssign(annotated, opt) {
				return false
			}
		}
		return true
	}
	return false
}

func (a *Analyzer) validateDefaultValues(specs []paramSpec, funcType *FunctionType, errorNode *ast.TreeNode) {
	for i, spec := range specs {
		if spec.defaultValue == nil {
			continue
		}
		defaultType := inferLiteralType(spec.defaultValue)
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
