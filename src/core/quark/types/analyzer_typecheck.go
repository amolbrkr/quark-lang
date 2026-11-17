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
			if _, isTbl := t.(*TableType); isTbl {
				// tables v0.3 D49: the row count is a method.
				shown := "t"
				if callNode != nil && len(callNode.Children) > 1 && len(callNode.Children[1].Children) > 0 {
					if txt := exprText(callNode.Children[1].Children[0]); txt != "x" {
						shown = txt
					}
				}
				a.errorAt(callNode, "len() of a table isn't defined; its row count is %s.nrow()", shown)
				return TypeInt
			}
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
	case "mean", "sd":
		// Prelude reductions (tables v0.3 D40): a float, with the table
		// aggregate's rules. A vec[date] is rejected here; an untyped vec is
		// checked when it runs.
		if len(argTypes) >= 1 && !isUnknownType(argTypes[0]) {
			t := argTypes[0]
			vec, isVec := t.(*VectorType)
			if !isVec {
				a.errorAt(callNode, "argument 1 of '%s' expects numeric or bool vec, got %s", name, t.String())
				return TypeError
			}
			switch el := vec.ElementType; {
			case isUnknownType(el):
				return TypeFloat
			case el.Equals(TypeDate):
				a.errorAt(callNode, "%s of a vec[date]: dates can't be averaged", name)
				return TypeError
			case el.Equals(TypeInt), el.Equals(TypeFloat), el.Equals(TypeBool):
				return TypeFloat
			default:
				a.errorAt(callNode, "argument 1 of '%s' expects numeric or bool vec, got %s", name, t.String())
				return TypeError
			}
		}
		// An untyped argument is checked when the call runs, and the
		// result is a float or a runtime error (q_mean, q_sd).
		return TypeFloat
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
				if IsNumeric(el) || el.Equals(TypeBool) || el.Equals(TypeString) || el.Equals(TypeDate) {
					return el
				}
				a.errorAt(callNode, "argument 1 of '%s' with single argument expects a vec, got %s", name, t.String())
				return TypeError
			}
			a.errorAt(callNode, "argument 1 of '%s' with single argument expects a vec, got %s", name, t.String())
			return TypeAny
		}
		if len(argTypes) == 2 {
			return a.minMax2Type(name, argTypes, callNode)
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

// minMax2Type types two-argument min and max (tables v0.3 D51): two values
// of one type (int, float, str or date) give that type. An int and a float
// stay untyped, because a wide int that wins comes back as an int (q_minmax);
// in a verb, where a column needs one type, that is an error.
func (a *Analyzer) minMax2Type(name string, argTypes []Type, callNode *ast.TreeNode) Type {
	ordered := func(t Type) bool { return IsNumeric(t) || t.Equals(TypeString) || t.Equals(TypeDate) }
	known := 0
	for i, t := range argTypes {
		if isUnknownType(t) {
			continue
		}
		if !ordered(t) {
			a.errorAt(callNode, "argument %d of '%s' expects int, float, str or date, got %s", i+1, name, t.String())
			return TypeError
		}
		known++
	}
	if known < 2 {
		return TypeAny
	}
	t0, t1 := argTypes[0], argTypes[1]
	switch {
	case t0.Equals(t1):
		return t0
	case IsNumeric(t0) && IsNumeric(t1):
		if a.inVerbExpr() {
			intArg := callArg(callNode, 0)
			if t1.Equals(TypeInt) {
				intArg = callArg(callNode, 1)
			}
			a.errorAt(callNode, "%s of an int and a float has no single type, and a column needs one; convert the int with to_float(%s)", name, exprText(intArg))
			return TypeError
		}
		return TypeAny
	}
	a.errorAt(callNode, "'%s' compares two values of one type, got %s and %s", name, t0.String(), t1.String())
	return TypeError
}

// callArg is argument i of a call node, or nil.
func callArg(call *ast.TreeNode, i int) *ast.TreeNode {
	if call == nil || len(call.Children) < 2 || i >= len(call.Children[1].Children) {
		return nil
	}
	return call.Children[1].Children[i]
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
