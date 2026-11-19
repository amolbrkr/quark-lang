package lower

// Checked mode's comparison (specs/tables_v0_3_spec.md §8.7, D57): with
// Options.ParChecked, each query step, and each vec operation outside a row
// loop (types.Analysis.VecOps), is a ParCheck. The runtime runs its body
// again on one thread when the first run split a loop across threads, and
// compares the two results (runtime/.../core/parcheck.hpp). Inside a row
// loop nothing is checked on its own: the loop's step is.

// checkedStep lowers a query step with body, as a ParCheck named after its
// verb when the program is checked.
func (l *lowerer) checkedStep(verb string, body func() Expr) Expr {
	if !l.parChecked || l.row != nil {
		return body()
	}
	var res Expr
	blk := l.sub(func() { res = body() })
	if !parCheckRep(res.Rep()) {
		l.ice("INV-QUERY", nil, "a query step's result is a %s", res.Rep())
	}
	t := l.newTemp(res.Rep())
	l.add(&ParCheck{T: t, What: "'" + verb + "'", Body: blk, Result: res})
	return &TempRef{T: t}
}

// checkedVecOp makes e, the lowered vec operation named what, a ParCheck.
// The arguments of its call are computed first, into temporaries, so that
// only the operation itself runs again. An expression of another shape is
// left as it is (it isn't a runtime call, so it doesn't use the pool).
func (l *lowerer) checkedVecOp(what string, e Expr) Expr {
	if !l.parChecked || l.row != nil || !parCheckRep(e.Rep()) {
		return e
	}
	call := innerCall(e)
	if call == nil {
		return e
	}
	for i, a := range call.Args {
		if !isStable(a) {
			call.Args[i] = l.let(a)
		}
	}
	t := l.newTemp(e.Rep())
	l.add(&ParCheck{T: t, What: what, Body: &Block{}, Result: e})
	return &TempRef{T: t}
}

// parCheckRep reports whether the runtime can compare two results of rep.
func parCheckRep(r Rep) bool {
	switch r {
	case RepValue, RepTable, RepInt, RepFloat, RepBool:
		return true
	}
	return false
}

// innerCall is the call e computes, under conversions of its result.
func innerCall(e Expr) *Call {
	switch x := e.(type) {
	case *Call:
		return x
	case *Box:
		return innerCall(x.X)
	case *Unbox:
		return innerCall(x.X)
	case *Cast:
		return innerCall(x.X)
	case *Adapt:
		return innerCall(x.X)
	}
	return nil
}
