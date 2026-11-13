package codegen

import (
	"fmt"

	"quark/ast"
	"quark/ir"
	"quark/types"
)

// Variables are declared, read and written only through the analyzer's
// symbols (types.Symbol, Analysis.Refs): the symbol says how the variable is
// stored and what its C++ variable is called, and its Decl node is where the
// C++ declaration goes. Codegen keeps no scope state of its own.

// varSym is the variable symbol of an identifier node the analyzer resolved
// (a declaration, an assignment target or a read).
func (g *Generator) varSym(node *ast.TreeNode) *types.Symbol {
	sym := g.refs[node]
	if !sym.IsVariable() || sym.CName == "" {
		panicICEf("INV-SYMBOL", node, "no variable symbol for '%s'", node.TokenLiteral())
	}
	return sym
}

// readVar is a QValue expression for the value of sym.
func (g *Generator) readVar(sym *types.Symbol) string {
	if sym.Storage == ir.StorageCell {
		if g.checkedCells[sym] {
			return fmt.Sprintf("q_cell_get(%s, \"%s\")", sym.CName, escapeCppString(g.names.Display(sym.Name)))
		}
		return sym.CName + "->value"
	}
	return boxExpr(sym.CName, sym.Storage.CType())
}

// assignSym emits `name = value` for the identifier node target, declaring
// the C++ variable when target is where the symbol is declared. It returns
// the variable's value as a QValue.
func (g *Generator) assignSym(target, valueNode *ast.TreeNode) string {
	sym := g.varSym(target)
	declare := sym.Decl == target && !sym.Hoisted
	if sym.Storage == ir.StorageCell {
		// The cell exists before the value is built: a lambda assigned to
		// the variable (`fib = fn(n) -> fib(n - 1)`) captures that cell.
		if declare {
			g.emitLine("QCell* %s = q_new_cell(qv_null());", sym.CName)
		}
		g.emitLine("%s->value = %s;", sym.CName, g.storeValue(sym, valueNode, g.generateExpr(valueNode)))
		return g.readVar(sym)
	}
	value := g.storeValue(sym, valueNode, g.generateExpr(valueNode))
	if declare {
		g.emitLine("%s %s = %s;", sym.Storage.CType(), sym.CName, value)
	} else {
		g.emitLine("%s = %s;", sym.CName, value)
	}
	return g.readVar(sym)
}

// storeValue converts expr, the QValue of valueNode, to what sym stores: a
// raw scalar for scalar storage, a QValue otherwise. The value is made to have
// sym's StoreType: an int is widened for a float, and a value whose type the
// analyzer didn't prove is checked (q_var_*), so later reads can trust it.
func (g *Generator) storeValue(sym *types.Symbol, valueNode *ast.TreeNode, expr string) string {
	what := escapeCppString(g.names.Display(sym.Name))
	if sym.Storage.IsScalar() {
		return g.toScalar(sym.Storage, valueNode, expr, what)
	}
	want := sym.StoreType
	if want == nil {
		return expr
	}
	if st := scalarStorage(want); st.IsScalar() {
		if vt := g.nodeType(valueNode); vt != nil && vt.Equals(want) {
			return expr
		}
		return boxExpr(g.toScalar(st, valueNode, expr, what), st.CType())
	}
	if g.proven(want, valueNode) {
		return expr
	}
	if c, ok := checkFor(want); ok {
		return c.call("var", expr, fmt.Sprintf("\"%s\"", what))
	}
	return expr
}

// toScalar converts expr, the QValue of node, to the raw C++ scalar of st. A
// node already lowered to a scalar is used as is (an int widened to float);
// a value the analyzer typed exactly is read from the union; anything else is
// checked at runtime, naming the variable what.
func (g *Generator) toScalar(st ir.Storage, node *ast.TreeNode, expr, what string) string {
	raw, tier := g.scalarExpr(node)
	vt := g.nodeType(node)
	is := func(t types.Type) bool { return vt != nil && vt.Equals(t) }
	switch st {
	case ir.StorageInt:
		if tier == "long long" {
			return raw
		}
		if is(types.TypeInt) {
			return fmt.Sprintf("(%s).data.int_val", expr)
		}
		return fmt.Sprintf("q_var_int(%s, \"%s\")", expr, what)
	case ir.StorageFloat:
		switch {
		case tier == "double":
			return raw
		case tier == "long long":
			return fmt.Sprintf("(double)(%s)", raw)
		case is(types.TypeFloat):
			return fmt.Sprintf("(%s).data.float_val", expr)
		case is(types.TypeInt):
			return fmt.Sprintf("(double)((%s).data.int_val)", expr)
		}
		return fmt.Sprintf("q_var_float(%s, \"%s\")", expr, what)
	case ir.StorageBool:
		if tier == "bool" {
			return raw
		}
		if is(types.TypeBool) {
			return fmt.Sprintf("(%s).data.bool_val", expr)
		}
		return fmt.Sprintf("q_var_bool(%s, \"%s\")", expr, what)
	}
	panicICEf("INV-STORAGE", node, "toScalar called for non-scalar storage")
	return expr
}

// scalarStorage is the raw scalar storage for values of type t, or
// StorageValue.
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

// proven reports whether the analyzer proved that valueNode's value has
// type want (so no runtime check is needed).
func (g *Generator) proven(want types.Type, valueNode *ast.TreeNode) bool {
	vt := g.nodeType(valueNode)
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
	if st := scalarStorage(want); st.IsScalar() {
		return vt.Equals(want)
	}
	return types.CanAssign(want, vt)
}

// typeCheck is the runtime check that a value has an annotated type, used
// for parameters (q_param_*), struct fields (q_field_*) and typed
// declarations (q_var_*). Each family takes the value, the check, and its
// own names for the message.
type typeCheck struct {
	scalar     string // "int", "float" or "bool"
	structName string // internal name of a struct type
	tag        string // QValue tag of any other type
	allowNull  bool
	key        string // type name in messages
}

// checkFor is the check for values of type t; ok is false when any value
// is allowed.
func checkFor(t types.Type) (typeCheck, bool) {
	if t == nil {
		return typeCheck{}, false
	}
	switch tt := t.(type) {
	case *types.StructType:
		return typeCheck{structName: tt.Name}, true
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

// call emits the check of family ("param", "field" or "var") on value; args
// are the family's message arguments. Scalar checks of the param and var
// families return a raw scalar, every other check a QValue.
func (c typeCheck) call(family, value, args string) string {
	switch {
	case c.scalar != "":
		return fmt.Sprintf("q_%s_%s(%s, %s)", family, c.scalar, value, args)
	case c.structName != "":
		return fmt.Sprintf("q_%s_struct(%s, &_sdef_%s, %s)", family, value, sanitizeVarName(c.structName), args)
	}
	return fmt.Sprintf("q_%s_check(%s, QValue::%s, %t, \"%s\", %s)", family, value, c.tag, c.allowNull, c.key, args)
}

// boxedCall is call for a QValue result: raw scalars of the param and var
// families are boxed again.
func (c typeCheck) boxedCall(family, value, args string) string {
	expr := c.call(family, value, args)
	if family == "field" {
		return expr
	}
	switch c.scalar {
	case "int":
		return "qv_int(" + expr + ")"
	case "float":
		return "qv_float(" + expr + ")"
	case "bool":
		return "qv_bool(" + expr + ")"
	}
	return expr
}

// declareCaptured emits the declaration of a captured variable holding the
// QValue value.
func (g *Generator) declareCaptured(sym *types.Symbol, value string) {
	g.emitLine("QCell* %s = q_new_cell(%s);", sym.CName, value)
}
