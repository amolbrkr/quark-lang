// Package lower turns the analyzed AST into a small typed IR (architecture.md
// §13): every decision about evaluation order, boxing, storage, fast paths
// and runtime checks is made here, so the emitter (package emit) only prints.
//
// The IR is a tree of statements and expressions. Lowering an expression
// appends the statements it needs (temporaries, checks) to the current block
// and returns an expression that is safe to evaluate where it is used:
// operands are evaluated left to right, and the temporaries of a conditional
// or repeated context (`and`, `or`, `x if c else y`, `elseif`, a `while`
// condition) live in the branch or loop that evaluates them.
package lower

import "quark/types"

// Rep is how a value is held in generated C++: QValue, a raw scalar, or
// another native C++ type (named by its spelling).
type Rep string

const (
	RepValue  Rep = "QValue"
	RepInt    Rep = "long long"
	RepFloat  Rep = "double"
	RepBool   Rep = "bool"
	RepStr    Rep = "const char*"
	RepList   Rep = "QList*"
	RepDict   Rep = "QDict*"
	RepVector Rep = "QVector*"
	RepFunc   Rep = "QClosure*"
	RepStruct Rep = "QStruct*"
	RepTable  Rep = "QTable*"
	RepVecRef Rep = "const QVector&"
	RepLoc    Rep = "const QSourceLoc*"
)

// IsScalar reports whether r is a raw C++ scalar.
func (r Rep) IsScalar() bool { return r == RepInt || r == RepFloat || r == RepBool }

// RepOfCType maps a native C++ type of a signature (ir.CallPlan's
// NativeParamTypes) to its Rep; int64_t is held as long long.
func RepOfCType(c string) Rep {
	switch c {
	case "int64_t", "long long":
		return RepInt
	case "", "QValue":
		return RepValue
	}
	return Rep(c)
}

// ---------------------------------------------------------------------------
// Expressions

// Expr is an IR expression.
type Expr interface {
	Rep() Rep
}

// Const is a C++ constant expression with no effects: a literal (5, 2.5,
// true), a QValue constructor of a literal (qv_null(), qv_string("a")), or
// the address of a static (&q_elem_int).
type Const struct {
	R    Rep
	Text string
}

// Temp is a temporary declared by a Let.
type Temp struct {
	ID int
	R  Rep
}

// TempRef reads a temporary.
type TempRef struct{ T *Temp }

// VarRef reads a variable. A captured variable (a cell) is read as a QValue;
// Checked marks a cell that a hoisted function may read before it is
// assigned (q_cell_get, naming Display in the error).
type VarRef struct {
	Sym     *types.Symbol
	Checked bool
	Display string
}

// Param is a C++ parameter of a function. Sym is the Quark parameter it
// binds, or nil for a thunk's arguments.
type Param struct {
	Sym   *types.Symbol
	CName string
	CType string
}

// ParamRef reads a C++ parameter.
type ParamRef struct{ P *Param }

// Call is a call of a C++ function: a runtime helper (q_add), a generated
// function (quark_f), or a function value call (q_call2). Pure marks calls
// that have no effects (value constructors), so they need no temporary to
// keep evaluation order.
type Call struct {
	Fn   string
	Args []Expr
	R    Rep
	Pure bool
}

// Box turns a raw scalar or native value into a QValue.
type Box struct{ X Expr }

// Unbox reads a raw scalar out of a QValue (the union field, no tag check).
// Proof says why the tag is known (architecture.md §12); the verifier rejects
// an Unbox without one.
type Unbox struct {
	X     Expr
	To    Rep
	Proof Proof
	// Type is the analyzer's type of the value (ProofType), or the element
	// type of the list or vec a loop iterates (ProofElement).
	Type types.Type
}

// Proof is the reason an Unbox may skip the tag check.
type Proof int

const (
	ProofNone Proof = iota
	// ProofType: the analyzer typed the value exactly as Type, which is a
	// guarantee when every way into the value is guarded (§12).
	ProofType
	// ProofElement: X reads an element of a list or vec that a for loop keeps
	// in range. The runtime checks every write into a list against its
	// element type (rule C) and every vec element taken out for a missing
	// entry (D12).
	ProofElement
)

// ScalarRepOf is the raw representation of a scalar type, "" for any other.
func ScalarRepOf(t types.Type) Rep {
	switch {
	case t == nil:
		return ""
	case t.Equals(types.TypeInt):
		return RepInt
	case t.Equals(types.TypeFloat):
		return RepFloat
	case t.Equals(types.TypeBool):
		return RepBool
	}
	return ""
}

// Cast converts a raw scalar to another scalar rep: an int widened to a
// double, or a double truncated to an int (range() bounds).
type Cast struct {
	X  Expr
	To Rep
}

// InitList is a braced C++ initializer of type Type (the arguments of
// q_calln, for calls with more than 12 arguments).
type InitList struct {
	Type  string
	Elems []Expr
}

// Adapt converts a QValue to a native C++ type with the checked q_as_*
// helpers (an int widens to a double).
type Adapt struct {
	X  Expr
	To Rep
}

// CheckKind is what a Check tests.
type CheckKind int

const (
	CheckScalar CheckKind = iota // int, float or bool: result is the raw scalar for param/var/ret
	CheckStruct                  // a struct type, by its QStructDef
	CheckTag                     // a QValue tag (str, list, dict, vec, result, resource, fn)
	CheckTable                   // a table of a row type (StructDef; "" for a bare tbl), relabeling an anonymous row
)

// Check is a runtime type check of a value against an annotated type:
// q_<Family>_<scalar>, q_<Family>_struct or q_<Family>_check. Family is
// param, var, field or ret; Args are the message arguments (already C++
// string literals).
type Check struct {
	Family    string
	Kind      CheckKind
	Scalar    string // int, float, bool
	StructDef string // C++ name of the QStructDef
	Tag       string // QValue tag enumerator
	AllowNull bool
	Key       string // type name in messages
	X         Expr
	Args      []string
	R         Rep
}

// Arith is raw scalar arithmetic with the boxed operators' failure semantics
// (int overflow and division or modulo by zero are fatal).
type Arith struct {
	Op   string // + - * / %
	L, X Expr
	R    Rep
}

// Neg is raw scalar negation (checked for ints unless the operand is a literal).
type Neg struct {
	X       Expr
	Checked bool
}

// Compare is a raw scalar comparison.
type Compare struct {
	Op   string // < <= > >= == !=
	L, X Expr
}

// Not is raw boolean negation.
type Not struct{ X Expr }

// Truthy is q_truthy(x) on a QValue.
type Truthy struct{ X Expr }

// AndAlso and OrElse are raw short-circuit boolean operators; lowering uses
// them only when the right operand needs no statements.
type AndAlso struct{ L, X Expr }
type OrElse struct{ L, X Expr }

// Select is c ? t : f; lowering uses it only when neither branch needs
// statements.
type Select struct {
	C, T, F Expr
	R       Rep
}

// FuncValue is a function value for a C++ function that needs no closure:
// qv_func((void*)Fn, &Info).
type FuncValue struct {
	Fn   string
	Info string
}

// MakeClosure allocates a closure of Fn capturing Cells; it may only be the
// value of a Let.
type MakeClosure struct {
	Fn    string
	Info  string
	Cells []*types.Symbol
}

// TagIs tests a QValue's runtime tag.
type TagIs struct {
	X   Expr
	Tag string
}

// LocRef is the address of entry Index of the program's location table.
type LocRef struct{ Index int }

// VecRef binds a vector QValue's buffers (const QVector&).
type VecRef struct{ X Expr }

// VecCount is a bound vector's element count.
type VecCount struct{ V Expr }

// VecElem reads element Idx of a bound dense vector as a raw scalar of rep R.
type VecElem struct {
	V, Idx Expr
	R      Rep
}

func (e *Const) Rep() Rep       { return e.R }
func (e *TempRef) Rep() Rep     { return e.T.R }
func (e *ParamRef) Rep() Rep    { return RepOfCType(e.P.CType) }
func (e *Call) Rep() Rep        { return e.R }
func (e *Box) Rep() Rep         { return RepValue }
func (e *Unbox) Rep() Rep       { return e.To }
func (e *Cast) Rep() Rep        { return e.To }
func (e *InitList) Rep() Rep    { return Rep(e.Type) }
func (e *Adapt) Rep() Rep       { return e.To }
func (e *Check) Rep() Rep       { return e.R }
func (e *Arith) Rep() Rep       { return e.R }
func (e *Neg) Rep() Rep         { return e.X.Rep() }
func (e *Compare) Rep() Rep     { return RepBool }
func (e *Not) Rep() Rep         { return RepBool }
func (e *Truthy) Rep() Rep      { return RepBool }
func (e *AndAlso) Rep() Rep     { return RepBool }
func (e *OrElse) Rep() Rep      { return RepBool }
func (e *Select) Rep() Rep      { return e.R }
func (e *FuncValue) Rep() Rep   { return RepValue }
func (e *MakeClosure) Rep() Rep { return RepValue }
func (e *TagIs) Rep() Rep       { return RepBool }
func (e *LocRef) Rep() Rep      { return RepLoc }
func (e *VecRef) Rep() Rep      { return RepVecRef }
func (e *VecCount) Rep() Rep    { return RepInt }
func (e *VecElem) Rep() Rep     { return e.R }

// Rep of a variable read: its storage, a QValue for a cell.
func (e *VarRef) Rep() Rep {
	if e.Sym.Storage.IsScalar() {
		return Rep(e.Sym.Storage.CType())
	}
	return RepValue
}

// ---------------------------------------------------------------------------
// Statements

// Stmt is an IR statement.
type Stmt interface{ stmt() }

// Block is a statement list (a C++ block when nested).
type Block struct{ Stmts []Stmt }

// Let declares temporary T with Value.
type Let struct {
	T     *Temp
	Value Expr
}

// SetTemp assigns a declared temporary.
type SetTemp struct {
	T     *Temp
	Value Expr
}

// DeclVar declares a variable's C++ variable, with the symbol's storage,
// initialized to Value (already converted to the storage). A cell is created
// holding Value, or unassigned when Unassigned is set (a variable a hoisted
// function captures, read with q_cell_get until assigned).
type DeclVar struct {
	Sym        *types.Symbol
	Value      Expr
	Unassigned bool
}

// SetVar assigns a declared variable (a cell's value for a cell).
type SetVar struct {
	Sym   *types.Symbol
	Value Expr
}

// SetField stores a field of a struct under construction (a QStruct* temp).
type SetField struct {
	Obj   Expr
	Index int
	Value Expr
}

// Do evaluates an expression for its effects.
type Do struct{ X Expr }

// If runs Then when Cond (a bool) holds, else Else (nil for none).
type If struct {
	Cond       Expr
	Then, Else *Block
}

// While runs Pre, tests Cond, and runs Body while it holds. Pre holds the
// statements Cond needs, so they run before every test.
type While struct {
	Pre  *Block
	Cond Expr
	Body *Block
}

// ForRange is `for (long long v = Start; Step > 0 ? v < End : v > End; v += Step)`
// for a range() loop; Start, End and Step are temporaries or constants.
type ForRange struct {
	Var              *types.Symbol
	Start, End, Step Expr
	Body             *Block
}

// ForIndex is `for (long long i = 0; i < Len; i++)`. With Sized set (a list
// as a QList*, or a QValue that may hold one) the test also checks that the
// list still has Len elements, a runtime error at Loc (a LocRef, or nullptr
// for none) if the body changed its length.
type ForIndex struct {
	Idx   *Temp
	Len   Expr
	Sized Expr
	Loc   Expr
	Body  *Block
}

// Break and Continue leave or restart the innermost loop.
type Break struct{}
type Continue struct{}

// Return returns Value from the function.
type Return struct{ Value Expr }

// SetLoc records the source location for runtime errors: entry Index of the
// program's location table (Program.Locs).
type SetLoc struct{ Index int }

// SourceLoc is one entry of the program's location table.
type SourceLoc struct {
	File      string
	Line, Col int
}

func (*Let) stmt()      {}
func (*SetTemp) stmt()  {}
func (*DeclVar) stmt()  {}
func (*SetVar) stmt()   {}
func (*SetField) stmt() {}
func (*Do) stmt()       {}
func (*If) stmt()       {}
func (*While) stmt()    {}
func (*ForRange) stmt() {}
func (*ForIndex) stmt() {}
func (*Break) stmt()    {}
func (*Continue) stmt() {}
func (*Return) stmt()   {}
func (*SetLoc) stmt()   {}

// ---------------------------------------------------------------------------
// Program

// Func is a generated C++ function. Params are its C++ parameters after the
// hidden `QClosure* _cl`; Captures are the cells it reads from _cl.
type Func struct {
	CName    string // full C++ name (quark_f, quark_f__thunk, _thunk_q_sqrt)
	Ret      string // C++ return type
	Params   []*Param
	Captures []*types.Symbol
	// LocGuard restores the caller's source location on return (every
	// function compiled from Quark code; not thunks).
	LocGuard bool
	Body     *Block
}

// FnInfo is the static QFnInfo of a function that can become a value.
type FnInfo struct {
	CName      string // _qfi_<name>
	Display    string
	Min, Max   int
	DefaultsFn string // "" for none
}

// DefaultArgFn builds a function's omitted default arguments for calls
// through a function value: one case per parameter with a default.
type DefaultArgFn struct {
	CName string
	Cases []DefaultCase
}

// DefaultCase is the default of parameter Index; Body ends with a Return.
type DefaultCase struct {
	Index int
	Body  *Block
}

// StructDef is the static QStructDef of a struct type, which is also a table
// row type: Dtypes is each field's column type (QVector::Type as int, -1 for
// a field that can't be a column), nil when no field can; Anonymous marks a
// row a query derived.
type StructDef struct {
	CName     string // quark_<internal>, as in _sdef_quark_<internal>
	Display   string
	Fields    []string
	Dtypes    []int
	Anonymous bool
}

// ColumnDtype is the QVector::Type (as int) of a column of type t, or -1.
func ColumnDtype(t types.Type) int {
	switch {
	case t == nil:
		return -1
	case t.Equals(types.TypeFloat):
		return 0
	case t.Equals(types.TypeInt):
		return 1
	case t.Equals(types.TypeBool):
		return 2
	case t.Equals(types.TypeString):
		return 3
	}
	return -1
}

// ElemDesc is a static QElemType descriptor.
type ElemDesc struct {
	CName string
	Init  string
}

// Program is a whole lowered program.
type Program struct {
	Includes []string
	// Decls are forward declarations of every generated function, in order.
	Decls     []*Func
	FnInfos   []FnInfo
	Structs   []StructDef
	ElemDescs []ElemDesc
	// Locs is the static table of source locations SetLoc points into.
	Locs          []SourceLoc
	DefaultArgFns []DefaultArgFn
	Funcs         []*Func
	Main          *Block
}
