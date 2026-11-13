package codegen

import (
	"fmt"

	"quark/ast"
	"quark/ir"
	"quark/types"
)

// elemDesc is a C++ expression for the runtime element type (a
// `const QElemType*`, runtime types/elemtype.hpp) of values of type t: what
// a list or dict created with element type t checks every write against.
// Types the runtime can't check (any, unions, null) are nullptr, any value.
// Composite types get a static descriptor, emitted once per distinct type.
func (g *Generator) elemDesc(t types.Type) string {
	switch {
	case t == nil || types.IsErrorType(t):
		return "nullptr"
	case t.Equals(types.TypeInt):
		return "&q_elem_int"
	case t.Equals(types.TypeFloat):
		return "&q_elem_float"
	case t.Equals(types.TypeBool):
		return "&q_elem_bool"
	case t.Equals(types.TypeString):
		return "&q_elem_str"
	}
	elem, errDesc, sdef, vdtype := "nullptr", "nullptr", "nullptr", -1
	var tag string
	switch v := t.(type) {
	case *types.ListType:
		tag, elem = "VAL_LIST", g.elemDesc(v.ElementType)
	case *types.DictType:
		tag, elem = "VAL_DICT", g.elemDesc(v.ValueType)
	case *types.ResultType:
		tag, elem, errDesc = "VAL_RESULT", g.elemDesc(v.OkType), g.elemDesc(v.ErrType)
	case *types.StructType:
		tag, sdef = "VAL_STRUCT", "&_sdef_"+sanitizeVarName(v.Name)
	case *types.VectorType:
		tag = "VAL_VECTOR"
		switch {
		case v.ElementType.Equals(types.TypeFloat):
			vdtype = 0 // QVector::Type::F64
		case v.ElementType.Equals(types.TypeInt):
			vdtype = 1 // I64
		case v.ElementType.Equals(types.TypeBool):
			vdtype = 2 // BOOL
		case v.ElementType.Equals(types.TypeString):
			vdtype = 3 // STR
		}
	case *types.FunctionType:
		// Only that it is a function: containers don't trust a stored
		// function's return type (types.ElementOf).
		tag = "VAL_FUNC"
	default:
		switch {
		case t.Equals(types.TypeFn):
			tag = "VAL_FUNC"
		case t.Equals(types.TypeResource):
			tag = "VAL_RESOURCE"
		default:
			return "nullptr"
		}
	}
	name := escapeCppString(g.names.DisplayText(t.String()))
	if _, isFn := t.(*types.FunctionType); isFn {
		name = "fn"
	}
	init := fmt.Sprintf("{QValue::%s, %s, %s, %s, %d, \"%s\"}", tag, elem, errDesc, sdef, vdtype, name)
	if g.elemDescNames == nil {
		g.elemDescNames = map[string]string{}
	}
	if n, ok := g.elemDescNames[init]; ok {
		return "&" + n
	}
	n := fmt.Sprintf("_qet_%d", len(g.elemDescNames)+1)
	g.elemDescNames[init] = n
	fmt.Fprintf(&g.elemDescs, "static const QElemType %s = %s;\n", n, init)
	return "&" + n
}

// checkedReturn makes result, the value the body of lambda returns, match the
// function's return annotation (rule D, architecture.md §12.9), as parameters
// are checked on entry: unchanged when the analyzer proved its type, an int
// widened for a float return, otherwise checked once (q_ret_*), so callers
// can trust the annotation. Native functions convert their result themselves.
func (g *Generator) checkedReturn(lambda, body *ast.TreeNode, result string) string {
	ft, ok := g.nodeType(lambda).(*types.FunctionType)
	if !ok || ft.AnnotatedReturnType == nil {
		return result
	}
	want := ft.AnnotatedReturnType
	if want.Equals(types.TypeAny) || types.IsErrorType(want) {
		return result
	}
	vt := g.nodeType(body)
	if st := scalarStorage(want); st.IsScalar() {
		if vt != nil && vt.Equals(want) {
			return result
		}
		if st == ir.StorageFloat && vt != nil && vt.Equals(types.TypeInt) {
			return fmt.Sprintf("qv_float((double)(%s).data.int_val)", result)
		}
	} else if g.proven(want, body) {
		return result
	}
	c, ok := checkFor(want)
	if !ok {
		return result
	}
	return c.boxedCall("ret", result, fmt.Sprintf("\"%s\"", escapeCppString(g.lambdaDisplayName(lambda))))
}
