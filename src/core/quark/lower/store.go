package lower

import (
	"fmt"
	"strings"

	"quark/ast"
	"quark/ir"
	"quark/types"
)

// Every value stored in a variable, parameter, struct field or returned from
// an annotated function is made to have the slot's type here: proven values
// are stored as they are (unboxed for scalar storage), unproven ones are
// checked at runtime (architecture.md §12).

var cppStringEscaper = strings.NewReplacer(
	"\\", "\\\\",
	"\"", "\\\"",
	"\n", "\\n",
	"\t", "\\t",
	"\r", "\\r",
	"\x00", "\\000",
	"\a", "\\a",
	"\b", "\\b",
	"\f", "\\f",
	"\v", "\\v",
)

func escapeCppString(s string) string { return cppStringEscaper.Replace(s) }

// storeValue converts v, the value of valueNode, to what sym stores: a raw
// scalar for scalar storage, a QValue otherwise, checked against sym's
// StoreType unless the analyzer proved it.
func (l *lowerer) storeValue(sym *types.Symbol, valueNode *ast.TreeNode, v Expr) Expr {
	what := l.names.Display(sym.Name)
	if sym.Storage.IsScalar() {
		return l.toScalar(sym.Storage, valueNode, v, what)
	}
	want := sym.StoreType
	if want == nil {
		return boxed(v)
	}
	if st := scalarStorage(want); st.IsScalar() {
		if vt := l.nodeType(valueNode); vt != nil && vt.Equals(want) {
			return boxed(v)
		}
		return boxed(l.toScalar(st, valueNode, v, what))
	}
	if l.proven(want, valueNode) {
		return boxed(v)
	}
	if c, ok := l.checkFor(want); ok {
		return c.check("var", v, []string{cstr(what)})
	}
	return boxed(v)
}

// toStorage converts a value of unknown type (a `when` binding) to sym's
// storage.
func (l *lowerer) toStorage(sym *types.Symbol, node *ast.TreeNode, v Expr) Expr {
	if sym.Storage.IsScalar() {
		return l.toScalar(sym.Storage, node, v, l.names.Display(sym.Name))
	}
	return boxed(v)
}

// toScalar converts v, the value of node, to the raw scalar of storage st: a
// raw scalar as is (an int widened for a float), a QValue the analyzer typed
// exactly read from the union, anything else checked at runtime.
func (l *lowerer) toScalar(st ir.Storage, node *ast.TreeNode, v Expr, what string) Expr {
	vt := l.nodeType(node)
	is := func(t types.Type) bool { return vt != nil && vt.Equals(t) && v.Rep() == RepValue }
	r := v.Rep()
	switch st {
	case ir.StorageInt:
		if r == RepInt {
			return v
		}
		if is(types.TypeInt) {
			return &Unbox{X: v, To: RepInt, Proof: ProofType, Type: vt}
		}
		return scalarCheck("var", "int", v, []string{cstr(what)})
	case ir.StorageFloat:
		switch {
		case r == RepFloat:
			return v
		case r == RepInt:
			return &Cast{X: v, To: RepFloat}
		case is(types.TypeFloat):
			return &Unbox{X: v, To: RepFloat, Proof: ProofType, Type: vt}
		case is(types.TypeInt):
			return &Cast{X: &Unbox{X: v, To: RepInt, Proof: ProofType, Type: vt}, To: RepFloat}
		}
		return scalarCheck("var", "float", v, []string{cstr(what)})
	case ir.StorageBool:
		if r == RepBool {
			return v
		}
		if is(types.TypeBool) {
			return &Unbox{X: v, To: RepBool, Proof: ProofType, Type: vt}
		}
		return scalarCheck("var", "bool", v, []string{cstr(what)})
	}
	l.ice("INV-STORAGE", node, "toScalar called for non-scalar storage")
	return nil
}

func scalarCheck(family, scalar string, v Expr, args []string) Expr {
	return typeCheck{scalar: scalar}.check(family, v, args)
}

// scalarStorage is the raw scalar storage of values of type t, or StorageValue.
func scalarStorage(t types.Type) ir.Storage {
	switch {
	case t == nil:
		return ir.StorageValue
	case t.Equals(types.TypeInt):
		return ir.StorageInt
	case t.Equals(types.TypeFloat):
		return ir.StorageFloat
	case t.Equals(types.TypeBool):
		return ir.StorageBool
	}
	return ir.StorageValue
}

// proven reports whether the analyzer proved that valueNode's value has type
// want, so no runtime check is needed.
func (l *lowerer) proven(want types.Type, valueNode *ast.TreeNode) bool {
	vt := l.nodeType(valueNode)
	if vt == nil || types.IsErrorType(vt) || vt.Equals(types.TypeAny) {
		return false
	}
	if _, isUnion := vt.(*types.UnionType); isUnion {
		return false
	}
	if wst, ok := want.(*types.StructType); ok {
		if vst, ok := vt.(*types.StructType); ok {
			return vst.Name == wst.Name
		}
		return vt.Equals(types.TypeNull)
	}
	// A table with another (anonymous) row the analyzer accepted by shape
	// still has to be relabeled, so only a subtype is stored as it is.
	if _, ok := want.(*types.TableType); ok {
		return types.IsSubtype(want, vt)
	}
	if st := scalarStorage(want); st.IsScalar() {
		return vt.Equals(want)
	}
	return types.CanAssign(want, vt)
}

// paramTagChecks maps non-scalar annotations to their QValue tag and whether
// null is accepted (only resource; structs have their own check).
var paramTagChecks = map[string]struct {
	tag       string
	allowNull bool
}{
	"str":      {"VAL_STRING", false},
	"list":     {"VAL_LIST", false},
	"dict":     {"VAL_DICT", false},
	"vec":      {"VAL_VECTOR", false},
	"result":   {"VAL_RESULT", false},
	"resource": {"VAL_RESOURCE", true},
	"fn":       {"VAL_FUNC", false},
}

// typeCheck is the runtime check that a value has an annotated type.
type typeCheck struct {
	scalar     string
	structName string
	tag        string
	allowNull  bool
	key        string
	// table is set for tbl annotations; structName is then the row ("" for
	// a bare tbl).
	table bool
}

// checkFor is the check of values of type t; ok is false when any value is
// allowed.
func (l *lowerer) checkFor(t types.Type) (typeCheck, bool) {
	if t == nil {
		return typeCheck{}, false
	}
	switch tt := t.(type) {
	case *types.StructType:
		return typeCheck{structName: tt.Name}, true
	case *types.TableType:
		c := typeCheck{table: true, key: l.names.DisplayText(t.String())}
		if tt.Row != nil {
			c.structName = tt.Row.Name
		}
		return c, true
	case *types.ListType:
		return tagCheck("list")
	case *types.DictType:
		return tagCheck("dict")
	case *types.VectorType:
		return tagCheck("vec")
	case *types.ResultType:
		return tagCheck("result")
	case *types.FunctionType:
		return tagCheck("fn")
	}
	switch {
	case t.Equals(types.TypeInt):
		return typeCheck{scalar: "int"}, true
	case t.Equals(types.TypeFloat):
		return typeCheck{scalar: "float"}, true
	case t.Equals(types.TypeBool):
		return typeCheck{scalar: "bool"}, true
	case t.Equals(types.TypeString):
		return tagCheck("str")
	case t.Equals(types.TypeResource):
		return tagCheck("resource")
	case t.Equals(types.TypeFn):
		return tagCheck("fn")
	}
	return typeCheck{}, false
}

func tagCheck(key string) (typeCheck, bool) {
	c, ok := paramTagChecks[key]
	if !ok {
		return typeCheck{}, false
	}
	return typeCheck{tag: c.tag, allowNull: c.allowNull, key: key}, true
}

// check is the check of family (param, var, field, ret) on v. Scalar checks
// of every family but field give a raw scalar, every other check a QValue.
func (c typeCheck) check(family string, v Expr, args []string) Expr {
	ch := &Check{Family: family, X: boxed(v), Args: args, AllowNull: c.allowNull, Key: c.key, R: RepValue}
	switch {
	case c.scalar != "":
		ch.Kind, ch.Scalar = CheckScalar, c.scalar
		if family != "field" {
			ch.R = map[string]Rep{"int": RepInt, "float": RepFloat, "bool": RepBool}[c.scalar]
		}
	case c.table:
		ch.Kind = CheckTable
		if c.structName != "" {
			ch.StructDef = "_sdef_" + structCName(c.structName)
		}
	case c.structName != "":
		ch.Kind, ch.StructDef = CheckStruct, "_sdef_"+structCName(c.structName)
	default:
		ch.Kind, ch.Tag = CheckTag, c.tag
	}
	return ch
}

// checkedField makes a struct field value match the field's type before it is
// stored: unchanged when proven, otherwise q_field_* (an int widens for a
// float field).
func (l *lowerer) checkedField(st *types.StructType, field types.StructField, valueNode *ast.TreeNode, v Expr) Expr {
	ft := field.Type
	if ft == nil || ft.Equals(types.TypeAny) || l.proven(ft, valueNode) {
		return boxed(v)
	}
	c, ok := l.checkFor(ft)
	if !ok {
		return boxed(v)
	}
	return c.check("field", v, []string{cstr(l.names.Display(st.Name)), cstr(field.Name)})
}

// checkedReturn makes a function's result match its return annotation
// (rule D): unchanged when proven, an int widened for a float, otherwise
// checked once (q_ret_*).
func (l *lowerer) checkedReturn(lambda, body *ast.TreeNode, result Expr, display string) Expr {
	ft, ok := l.nodeType(lambda).(*types.FunctionType)
	if !ok || ft.AnnotatedReturnType == nil {
		return boxed(result)
	}
	want := ft.AnnotatedReturnType
	if want.Equals(types.TypeAny) || types.IsErrorType(want) {
		return boxed(result)
	}
	vt := l.nodeType(body)
	if st := scalarStorage(want); st.IsScalar() {
		if vt != nil && vt.Equals(want) {
			return boxed(result)
		}
		if st == ir.StorageFloat && vt != nil && vt.Equals(types.TypeInt) {
			if result.Rep() == RepInt {
				return boxed(&Cast{X: result, To: RepFloat})
			}
			return boxed(&Cast{X: &Unbox{X: boxed(result), To: RepInt, Proof: ProofType, Type: vt}, To: RepFloat})
		}
	} else if l.proven(want, body) {
		return boxed(result)
	}
	c, ok := l.checkFor(want)
	if !ok {
		return boxed(result)
	}
	return boxed(c.check("ret", result, []string{cstr(display)}))
}

// elemDesc is a C++ expression for the runtime element type
// (`const QElemType*`) of values of type t: what a list or dict created with
// element type t checks every write against. nullptr means any value.
// Composite types get one static descriptor per distinct type.
func (l *lowerer) elemDesc(t types.Type) string {
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
		tag, elem = "VAL_LIST", l.elemDesc(v.ElementType)
	case *types.DictType:
		tag, elem = "VAL_DICT", l.elemDesc(v.ValueType)
	case *types.ResultType:
		tag, elem, errDesc = "VAL_RESULT", l.elemDesc(v.OkType), l.elemDesc(v.ErrType)
	case *types.StructType:
		tag, sdef = "VAL_STRUCT", "&_sdef_"+structCName(v.Name)
	case *types.TableType:
		tag = "VAL_TABLE"
		if v.Row != nil {
			sdef = "&_sdef_" + structCName(v.Row.Name)
		}
	case *types.VectorType:
		tag = "VAL_VECTOR"
		switch {
		case v.ElementType.Equals(types.TypeFloat):
			vdtype = 0
		case v.ElementType.Equals(types.TypeInt):
			vdtype = 1
		case v.ElementType.Equals(types.TypeBool):
			vdtype = 2
		case v.ElementType.Equals(types.TypeString):
			vdtype = 3
		}
	case *types.FunctionType:
		// Only that it is a function: containers don't trust a stored
		// function's return type.
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
	name := escapeCppString(l.names.DisplayText(t.String()))
	if _, isFn := t.(*types.FunctionType); isFn {
		name = "fn"
	}
	init := fmt.Sprintf("{QValue::%s, %s, %s, %s, %d, \"%s\"}", tag, elem, errDesc, sdef, vdtype, name)
	if n, ok := l.elemDescNames[init]; ok {
		return "&" + n
	}
	n := fmt.Sprintf("_qet_%d", len(l.elemDescNames)+1)
	l.elemDescNames[init] = n
	l.prog.ElemDescs = append(l.prog.ElemDescs, ElemDesc{CName: n, Init: init})
	return "&" + n
}
