package types

import (
	"quark/ast"
	"quark/builtins"
	"quark/ir"
)

// xs.contains(x) on lists and vecs (specs/tables_v0_3_spec.md D48, §7.1):
// whether some element equals x under ==. str.contains(sub) is the
// substring test, a different method.

// checkContains rejects a contains() that can't be true: the elements' type
// and x's type are known and no value of one equals a value of the other (a
// list[str] and an int). Struct elements are rejected as == on structs is.
func (a *Analyzer) checkContains(call *ast.TreeNode, receiverType Type, argTypes []Type, argNodes []*ast.TreeNode) {
	var elem Type
	switch rt := receiverType.(type) {
	case *ListType:
		if rt.Pending {
			return
		}
		elem = rt.ElementType
	case *VectorType:
		elem = rt.ElementType
	default:
		return
	}
	if len(argTypes) != 1 || IsErrorType(argTypes[0]) {
		return
	}
	x := argTypes[0]
	if isStructType(elem) || holdsStruct(elem) || isStructType(x) || holdsStruct(x) {
		a.errorAt(call, "contains() compares with ==, which isn't defined for struct values; compare their fields")
		return
	}
	if isUnknownType(elem) || isUnknownType(x) {
		return
	}
	if !mayEqual(elem, x) {
		a.errorAt(argNodes[0], "contains() can't be true here: the elements of %s are %s, and this value is %s", receiverType.String(), elem.String(), x.String())
	}
}

// mayEqual reports whether some value of type s can equal (==) some value
// of type t. Numbers compare across int and float; other scalars only with
// their own type; containers only with containers of their kind (their
// elements are compared when the program runs).
func mayEqual(s, t Type) bool {
	if u, ok := s.(*UnionType); ok {
		for _, o := range u.Options {
			if mayEqual(o, t) {
				return true
			}
		}
		return false
	}
	if u, ok := t.(*UnionType); ok {
		for _, o := range u.Options {
			if mayEqual(s, o) {
				return true
			}
		}
		return false
	}
	if IsNumeric(s) && IsNumeric(t) {
		return true
	}
	if s.Equals(t) {
		return true
	}
	switch s.(type) {
	case *ListType:
		_, ok := t.(*ListType)
		return ok
	case *DictType:
		_, ok := t.(*DictType)
		return ok
	case *VectorType:
		_, ok := t.(*VectorType)
		return ok
	case *ResultType:
		_, ok := t.(*ResultType)
		return ok
	}
	return false
}

// isContainsCall reports whether plan is xs.contains(x) on a list or vec.
func isContainsCall(plan *ir.CallPlan) bool {
	if plan == nil || !plan.IsMethod || plan.CalleeName != "contains" || plan.Dispatch != ir.DispatchBuiltin {
		return false
	}
	k := builtins.TypeKey(plan.ReceiverTypeKey)
	return k == builtins.TypeListAny || k == builtins.TypeVectorAny
}

// lookupSetOK checks, for xs.contains(x) in a verb with xs the same for
// every row, that xs's elements can go in the hash set lowering builds once
// per step: int, float, bool, str or date (D48). Elements of unknown type
// are checked when the set is built.
func (a *Analyzer) lookupSetOK(node *ast.TreeNode, plan *ir.CallPlan) bool {
	var elem Type
	switch rt := a.nodeTypes[plan.ReceiverNode].(type) {
	case *ListType:
		elem = rt.ElementType
	case *VectorType:
		elem = rt.ElementType
	default:
		return true
	}
	if isUnknownType(elem) {
		return true
	}
	if IsColumnType(elem) {
		return true
	}
	a.errorAt(node, "in a query, contains() looks the value up in a set of the list's elements, made once for the step, which holds int, float, bool, str or date values; these elements are %s", elem.String())
	return false
}
