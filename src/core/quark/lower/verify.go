package lower

import (
	"fmt"
	"strings"

	"quark/ir"
	"quark/types"
)

// Verify checks the invariants of a lowered program (architecture.md §13):
//
//   - every variable and temporary is declared before it is used, in an
//     enclosing scope of the same function, and declared once per scope;
//   - every value has the representation its use needs (a raw bool for a
//     condition, a QValue for q_truthy, a variable's storage for its value);
//   - every Unbox says why the value's tag is known, and the reason fits it;
//   - values a loop re-evaluates on each iteration are stable;
//   - break and continue are inside a loop, return inside a function;
//   - closures are bound by a Let and capture cells that are in scope;
//   - calls of generated functions pass one argument per parameter.
func Verify(p *Program) error {
	v := &verifier{funcs: map[string]*Func{}, locs: len(p.Locs)}
	for _, f := range p.Decls {
		v.funcs[f.CName] = f
	}
	for _, f := range p.Funcs {
		v.funcs[f.CName] = f
	}
	for _, df := range p.DefaultArgFns {
		for _, c := range df.Cases {
			v.where = df.CName
			v.fn = &Func{CName: df.CName, Ret: "QValue"}
			v.push()
			v.block(c.Body)
			v.pop()
		}
	}
	for _, f := range p.Funcs {
		v.function(f)
	}
	v.where, v.fn = "main", nil
	v.push()
	v.block(p.Main)
	v.pop()
	return v.err
}

type verifier struct {
	funcs  map[string]*Func
	locs   int
	scopes []map[interface{}]bool
	loops  int
	fn     *Func
	where  string
	err    error
}

func (v *verifier) fail(code, format string, args ...interface{}) {
	if v.err == nil {
		v.err = fmt.Errorf("internal compiler error [%s]: in %s: %s", code, v.where, fmt.Sprintf(format, args...))
	}
}

func (v *verifier) push() { v.scopes = append(v.scopes, map[interface{}]bool{}) }
func (v *verifier) pop()  { v.scopes = v.scopes[:len(v.scopes)-1] }

func (v *verifier) declare(key interface{}, what string) {
	top := v.scopes[len(v.scopes)-1]
	if top[key] {
		v.fail("INV-IR-SCOPE", "%s is declared twice in one scope", what)
	}
	top[key] = true
}

func (v *verifier) inScope(key interface{}) bool {
	for i := len(v.scopes) - 1; i >= 0; i-- {
		if v.scopes[i][key] {
			return true
		}
	}
	return false
}

func symName(s *types.Symbol) string { return fmt.Sprintf("variable '%s' (%s)", s.Name, s.CName) }

func (v *verifier) function(f *Func) {
	v.where, v.fn = f.CName, f
	v.loops = 0
	v.push()
	for _, p := range f.Params {
		v.declare(p, "parameter "+p.CName)
	}
	for _, s := range f.Captures {
		if s.Storage != ir.StorageCell {
			v.fail("INV-IR-REP", "captured %s is not a cell", symName(s))
		}
		v.declare(s, symName(s))
	}
	v.block(f.Body)
	v.pop()
}

func (v *verifier) block(b *Block) {
	if b == nil {
		return
	}
	for _, s := range b.Stmts {
		v.stmt(s)
	}
}

func (v *verifier) nested(b *Block) {
	v.push()
	v.block(b)
	v.pop()
}

// storageRep is the rep a value stored in sym must have.
func storageRep(sym *types.Symbol) Rep {
	if sym.Storage.IsScalar() {
		return Rep(sym.Storage.CType())
	}
	return RepValue
}

func (v *verifier) wantRep(e Expr, want Rep, what string) {
	if e == nil {
		v.fail("INV-IR-REP", "%s has no value", what)
		return
	}
	if got := e.Rep(); got != want {
		v.fail("INV-IR-REP", "%s is %s, want %s", what, got, want)
	}
}

// unboxProof checks that an Unbox says why the value's tag is known, and that
// the reason fits the value (architecture.md §12).
func (v *verifier) unboxProof(e *Unbox) {
	call, _ := e.X.(*Call)
	switch e.Proof {
	case ProofType, ProofElement:
		if got := ScalarRepOf(e.Type); got != e.To {
			v.fail("INV-IR-UNBOX", "unboxing a value of type %s as %s", typeText(e.Type), e.To)
		}
		if e.Proof == ProofElement && (call == nil || (call.Fn != "q_list_at" && call.Fn != "q_iter_get")) {
			v.fail("INV-IR-UNBOX", "element proof for a value that isn't a loop's element read")
		}
	default:
		v.fail("INV-IR-UNBOX", "unboxing to %s without a proof", e.To)
	}
}

func typeText(t types.Type) string {
	if t == nil {
		return "unknown"
	}
	return t.String()
}

// loopStable reports whether e may be evaluated on every iteration.
func loopStable(e Expr) bool {
	switch x := e.(type) {
	case *Const, *TempRef:
		return true
	case *VecCount:
		return loopStable(x.V)
	}
	return false
}

func (v *verifier) stmt(s Stmt) {
	switch s := s.(type) {
	case *Let:
		if mc, ok := s.Value.(*MakeClosure); ok {
			for _, c := range mc.Cells {
				if c.Storage != ir.StorageCell {
					v.fail("INV-IR-REP", "closure %s captures %s, which is not a cell", mc.Fn, symName(c))
				}
				if !v.inScope(c) {
					v.fail("INV-IR-SCOPE", "closure %s captures %s outside its scope", mc.Fn, symName(c))
				}
			}
			v.wantRep(s.Value, s.T.R, "temporary _t"+fmt.Sprint(s.T.ID))
		} else {
			v.expr(s.Value)
			v.wantRep(s.Value, s.T.R, "temporary _t"+fmt.Sprint(s.T.ID))
		}
		v.declare(s.T, fmt.Sprintf("temporary _t%d", s.T.ID))
	case *SetTemp:
		if !v.inScope(s.T) {
			v.fail("INV-IR-SCOPE", "temporary _t%d assigned outside its scope", s.T.ID)
		}
		v.expr(s.Value)
		v.wantRep(s.Value, s.T.R, fmt.Sprintf("value assigned to _t%d", s.T.ID))
	case *DeclVar:
		if !s.Unassigned {
			v.expr(s.Value)
			v.wantRep(s.Value, storageRep(s.Sym), "initial value of "+symName(s.Sym))
		} else if s.Sym.Storage != ir.StorageCell {
			v.fail("INV-IR-REP", "unassigned %s is not a cell", symName(s.Sym))
		}
		v.declare(s.Sym, symName(s.Sym))
	case *SetVar:
		if !v.inScope(s.Sym) {
			v.fail("INV-IR-SCOPE", "%s assigned outside its scope", symName(s.Sym))
		}
		v.expr(s.Value)
		v.wantRep(s.Value, storageRep(s.Sym), "value assigned to "+symName(s.Sym))
	case *SetField:
		v.expr(s.Obj)
		v.wantRep(s.Obj, RepStruct, "struct under construction")
		v.expr(s.Value)
		v.wantRep(s.Value, RepValue, "field value")
	case *Do:
		v.expr(s.X)
	case *If:
		v.expr(s.Cond)
		v.wantRep(s.Cond, RepBool, "if condition")
		v.nested(s.Then)
		if s.Else != nil {
			v.nested(s.Else)
		}
	case *While:
		v.push()
		v.block(s.Pre)
		v.expr(s.Cond)
		v.wantRep(s.Cond, RepBool, "while condition")
		v.loops++
		v.block(s.Body)
		v.loops--
		v.pop()
	case *ForRange:
		for _, e := range []Expr{s.Start, s.End, s.Step} {
			v.expr(e)
			v.wantRep(e, RepInt, "range bound")
		}
		if !loopStable(s.End) || !loopStable(s.Step) {
			v.fail("INV-IR-LOOP", "range bounds of %s must be temporaries or constants", symName(s.Var))
		}
		if s.Var.Storage != ir.StorageInt {
			v.fail("INV-IR-REP", "range loop variable %s is not an int", symName(s.Var))
		}
		v.push()
		v.declare(s.Var, symName(s.Var))
		v.loops++
		v.block(s.Body)
		v.loops--
		v.pop()
	case *ForIndex:
		v.expr(s.Len)
		v.wantRep(s.Len, RepInt, "loop length")
		if !loopStable(s.Len) {
			v.fail("INV-IR-LOOP", "loop length must be a temporary or constant")
		}
		if s.Sized != nil {
			v.expr(s.Sized)
			if r := s.Sized.Rep(); r != RepList && r != RepValue {
				v.fail("INV-IR-REP", "sized loop over %s, want %s or %s", r, RepList, RepValue)
			}
			if !loopStable(s.Sized) {
				v.fail("INV-IR-LOOP", "the list a loop checks must be a temporary")
			}
			if s.Loc == nil {
				v.fail("INV-IR-NODE", "sized loop without a source location")
			} else {
				v.expr(s.Loc)
				v.wantRep(s.Loc, RepLoc, "loop location")
			}
		}
		v.push()
		v.declare(s.Idx, fmt.Sprintf("temporary _t%d", s.Idx.ID))
		v.loops++
		v.block(s.Body)
		v.loops--
		v.pop()
	case *Break, *Continue:
		if v.loops == 0 {
			v.fail("INV-IR-LOOP", "break or continue outside a loop")
		}
	case *Return:
		if v.fn == nil {
			v.fail("INV-IR-RETURN", "return outside a function")
			return
		}
		v.expr(s.Value)
		v.wantRep(s.Value, RepOfCType(v.fn.Ret), "returned value")
	case *SetLoc:
		if s.Index < 0 || s.Index >= v.locs {
			v.fail("INV-IR-NODE", "source location %d is not in the location table (%d entries)", s.Index, v.locs)
		}
	default:
		v.fail("INV-IR-NODE", "unknown statement %T", s)
	}
}

func (v *verifier) expr(e Expr) {
	if e == nil {
		v.fail("INV-IR-NODE", "missing expression")
		return
	}
	switch e := e.(type) {
	case *Const:
	case *TempRef:
		if !v.inScope(e.T) {
			v.fail("INV-IR-SCOPE", "temporary _t%d read outside its scope", e.T.ID)
		}
	case *VarRef:
		if !v.inScope(e.Sym) {
			v.fail("INV-IR-SCOPE", "%s read outside its scope", symName(e.Sym))
		}
		if e.Checked && e.Sym.Storage != ir.StorageCell {
			v.fail("INV-IR-REP", "checked read of %s, which is not a cell", symName(e.Sym))
		}
	case *ParamRef:
		if !v.inScope(e.P) {
			v.fail("INV-IR-SCOPE", "parameter %s used outside its function", e.P.CName)
		}
	case *Call:
		for _, a := range e.Args {
			v.expr(a)
		}
		v.call(e)
	case *Box:
		v.expr(e.X)
		if e.X.Rep() == RepValue {
			v.fail("INV-IR-REP", "boxing a QValue")
		}
	case *Unbox:
		v.expr(e.X)
		v.wantRep(e.X, RepValue, "unboxed value")
		if !e.To.IsScalar() {
			v.fail("INV-IR-REP", "unboxing to %s", e.To)
		}
		v.unboxProof(e)
	case *Cast:
		v.expr(e.X)
		if !e.X.Rep().IsScalar() || !e.To.IsScalar() {
			v.fail("INV-IR-REP", "cast from %s to %s", e.X.Rep(), e.To)
		}
	case *Adapt:
		v.expr(e.X)
		v.wantRep(e.X, RepValue, "converted value")
	case *Check:
		v.expr(e.X)
		v.wantRep(e.X, RepValue, "checked value")
	case *Arith:
		v.expr(e.L)
		v.expr(e.X)
		if !e.L.Rep().IsScalar() || !e.X.Rep().IsScalar() {
			v.fail("INV-IR-REP", "raw arithmetic on %s and %s", e.L.Rep(), e.X.Rep())
		}
	case *Neg:
		v.expr(e.X)
		if r := e.X.Rep(); r != RepInt && r != RepFloat {
			v.fail("INV-IR-REP", "raw negation of %s", r)
		}
	case *Compare:
		v.expr(e.L)
		v.expr(e.X)
		if !e.L.Rep().IsScalar() || !e.X.Rep().IsScalar() {
			v.fail("INV-IR-REP", "raw comparison of %s and %s", e.L.Rep(), e.X.Rep())
		}
	case *Not:
		v.expr(e.X)
		v.wantRep(e.X, RepBool, "negated value")
	case *AndAlso:
		v.expr(e.L)
		v.expr(e.X)
		v.wantRep(e.L, RepBool, "operand of &&")
		v.wantRep(e.X, RepBool, "operand of &&")
	case *OrElse:
		v.expr(e.L)
		v.expr(e.X)
		v.wantRep(e.L, RepBool, "operand of ||")
		v.wantRep(e.X, RepBool, "operand of ||")
	case *Truthy:
		v.expr(e.X)
		v.wantRep(e.X, RepValue, "tested value")
	case *Select:
		v.expr(e.C)
		v.expr(e.T)
		v.expr(e.F)
		v.wantRep(e.C, RepBool, "selection condition")
		v.wantRep(e.T, e.R, "selected value")
		v.wantRep(e.F, e.R, "selected value")
	case *FuncValue:
		if _, ok := v.funcs[e.Fn]; !ok {
			v.fail("INV-IR-CALL", "function value of unknown function %s", e.Fn)
		}
	case *MakeClosure:
		v.fail("INV-IR-NODE", "closure of %s not bound by a Let", e.Fn)
	case *TagIs:
		v.expr(e.X)
		v.wantRep(e.X, RepValue, "tag-tested value")
		if !isStable(e.X) {
			v.fail("INV-IR-NODE", "tag test of an unstable value")
		}
	case *LocRef:
		if e.Index < 0 || e.Index >= v.locs {
			v.fail("INV-IR-NODE", "source location %d is not in the location table (%d entries)", e.Index, v.locs)
		}
	case *VecRef:
		v.expr(e.X)
		v.wantRep(e.X, RepValue, "vector")
	case *VecCount:
		v.expr(e.V)
		v.wantRep(e.V, RepVecRef, "vector")
	case *VecElem:
		v.expr(e.V)
		v.expr(e.Idx)
		v.wantRep(e.V, RepVecRef, "vector")
		v.wantRep(e.Idx, RepInt, "vector index")
	case *InitList:
		for _, x := range e.Elems {
			v.expr(x)
		}
	default:
		v.fail("INV-IR-NODE", "unknown expression %T", e)
	}
}

// call checks the arguments of calls of generated functions and of q_callN.
func (v *verifier) call(c *Call) {
	if f, ok := v.funcs[c.Fn]; ok {
		if len(c.Args) != len(f.Params)+1 {
			v.fail("INV-IR-CALL", "%s takes %d arguments, called with %d", c.Fn, len(f.Params), len(c.Args)-1)
			return
		}
		for i, p := range f.Params {
			v.wantRep(c.Args[i+1], RepOfCType(p.CType), fmt.Sprintf("argument %d of %s", i+1, c.Fn))
		}
		return
	}
	if strings.HasPrefix(c.Fn, "q_call") && c.Fn != "q_calln" {
		var n int
		if _, err := fmt.Sscanf(c.Fn, "q_call%d", &n); err == nil && n != len(c.Args)-1 {
			v.fail("INV-IR-CALL", "%s called with %d arguments", c.Fn, len(c.Args)-1)
		}
	}
}
