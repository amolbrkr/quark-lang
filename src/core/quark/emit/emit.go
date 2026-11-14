// Package emit prints a lowered program (package lower) as C++17. It makes no
// decisions: every temporary, conversion, check and evaluation order is
// already explicit in the IR.
package emit

import (
	"fmt"
	"strings"

	"quark/ir"
	"quark/lower"
)

type printer struct {
	b   strings.Builder
	ind int
}

// Program prints p as a C++ translation unit.
func Program(p *lower.Program) string {
	w := &printer{}
	w.raw("#include \"quark/quark.hpp\"\n")
	if len(p.Includes) > 0 {
		w.raw("\n// Extension includes\n")
		for _, inc := range p.Includes {
			w.raw("#include \"%s\"\n", esc(inc))
		}
	}
	w.raw("\n// Forward declarations\n")
	for _, f := range p.Decls {
		types := []string{"QClosure*"}
		for _, prm := range f.Params {
			types = append(types, prm.CType)
		}
		w.line("%s %s(%s);", f.Ret, f.CName, strings.Join(types, ", "))
	}
	w.raw("\n")
	for _, fi := range p.FnInfos {
		defaults := "nullptr"
		if fi.DefaultsFn != "" {
			w.line("static QValue %s(int index);", fi.DefaultsFn)
			defaults = fi.DefaultsFn
		}
		w.line("static const QFnInfo %s = {\"%s\", %d, %d, %s};", fi.CName, esc(fi.Display), fi.Min, fi.Max, defaults)
	}
	w.raw("\n")
	if len(p.Structs) > 0 {
		w.raw("// Struct type definitions\n")
		for _, s := range p.Structs {
			w.line("static const char* _sfields_%s[] = {", s.CName)
			w.ind++
			for _, f := range s.Fields {
				w.line("\"%s\",", esc(f))
			}
			w.ind--
			w.line("};")
			if s.Dtypes == nil {
				w.line("static const QStructDef _sdef_%s = {\"%s\", _sfields_%s, %d};", s.CName, esc(s.Display), s.CName, len(s.Fields))
				continue
			}
			dts := make([]string, len(s.Dtypes))
			for i, d := range s.Dtypes {
				dts[i] = fmt.Sprint(d)
			}
			w.line("static const signed char _sdtypes_%s[] = {%s};", s.CName, strings.Join(dts, ", "))
			w.line("static const QStructDef _sdef_%s = {\"%s\", _sfields_%s, %d, _sdtypes_%s, %t};", s.CName, esc(s.Display), s.CName, len(s.Fields), s.CName, s.Anonymous)
		}
		w.raw("\n")
	}
	if len(p.Locs) > 0 {
		w.raw("// Source locations for runtime errors\n")
		w.raw("static const QSourceLoc _qloc[] = {\n")
		for _, l := range p.Locs {
			w.raw("    {\"%s\", %d, %d},\n", esc(l.File), l.Line, l.Col)
		}
		w.raw("};\n\n")
	}
	if len(p.ElemDescs) > 0 {
		w.raw("// Element types of lists and dicts\n")
		for _, d := range p.ElemDescs {
			w.raw("static const QElemType %s = %s;\n", d.CName, d.Init)
		}
		w.raw("\n")
	}
	for _, df := range p.DefaultArgFns {
		w.raw("static QValue %s(int index) {\n", df.CName)
		w.ind++
		w.line("switch (index) {")
		for _, c := range df.Cases {
			w.line("case %d: {", c.Index)
			w.ind++
			w.block(c.Body)
			w.ind--
			w.line("}")
		}
		w.line("}")
		w.line("return qv_null();")
		w.ind--
		w.raw("}\n\n")
	}
	for _, f := range p.Funcs {
		w.fn(f)
	}
	w.raw("\nint main(int argc, char** argv) {\n")
	w.ind++
	w.line("q_gc_init();")
	w.line("q_set_args(argc, argv);")
	w.block(p.Main)
	w.line("return 0;")
	w.ind--
	w.raw("}\n")
	return w.b.String()
}

func esc(s string) string { return escapeCppString(s) }

var cppStringEscaper = strings.NewReplacer(
	"\\", "\\\\", "\"", "\\\"", "\n", "\\n", "\t", "\\t", "\r", "\\r",
	"\x00", "\\000", "\a", "\\a", "\b", "\\b", "\f", "\\f", "\v", "\\v",
)

func escapeCppString(s string) string { return cppStringEscaper.Replace(s) }

func (w *printer) raw(format string, args ...interface{}) {
	fmt.Fprintf(&w.b, format, args...)
}

func (w *printer) line(format string, args ...interface{}) {
	w.b.WriteString(strings.Repeat("    ", w.ind))
	fmt.Fprintf(&w.b, format, args...)
	w.b.WriteByte('\n')
}

func (w *printer) fn(f *lower.Func) {
	params := []string{"QClosure* _cl"}
	for _, p := range f.Params {
		params = append(params, p.CType+" "+p.CName)
	}
	w.raw("%s %s(%s) {\n", f.Ret, f.CName, strings.Join(params, ", "))
	w.ind++
	if f.LocGuard {
		w.line("QSourceLocGuard _loc_guard;")
	}
	for i, sym := range f.Captures {
		w.line("QCell* %s = _cl->captures[%d];", sym.CName, i)
	}
	w.block(f.Body)
	w.ind--
	w.raw("}\n\n")
}

func (w *printer) block(b *lower.Block) {
	if b == nil {
		return
	}
	for _, s := range b.Stmts {
		w.stmt(s)
	}
}

func tempName(t *lower.Temp) string { return fmt.Sprintf("_t%d", t.ID) }

func (w *printer) stmt(s lower.Stmt) {
	switch s := s.(type) {
	case *lower.Let:
		if mc, ok := s.Value.(*lower.MakeClosure); ok {
			cl := tempName(s.T) + "_cl"
			w.line("QClosure* %s = q_alloc_closure((void*)%s, &%s, %d);", cl, mc.Fn, mc.Info, len(mc.Cells))
			for i, c := range mc.Cells {
				w.line("%s->captures[%d] = %s;", cl, i, c.CName)
			}
			n := tempName(s.T)
			w.line("QValue %s; %s.type = QValue::VAL_FUNC; %s.data.func_val = %s;", n, n, n, cl)
			return
		}
		w.line("%s %s = %s;", s.T.R, tempName(s.T), w.expr(s.Value))
	case *lower.SetTemp:
		w.line("%s = %s;", tempName(s.T), w.expr(s.Value))
	case *lower.DeclVar:
		sym := s.Sym
		switch {
		case sym.Storage == ir.StorageCell && s.Unassigned:
			w.line("QCell* %s = q_new_unassigned_cell();", sym.CName)
		case sym.Storage == ir.StorageCell:
			w.line("QCell* %s = q_new_cell(%s);", sym.CName, w.expr(s.Value))
		default:
			w.line("%s %s = %s;", sym.Storage.CType(), sym.CName, w.expr(s.Value))
		}
	case *lower.SetVar:
		if s.Sym.Storage == ir.StorageCell {
			w.line("%s->value = %s;", s.Sym.CName, w.expr(s.Value))
		} else {
			w.line("%s = %s;", s.Sym.CName, w.expr(s.Value))
		}
	case *lower.SetField:
		w.line("%s->fields[%d] = %s;", w.expr(s.Obj), s.Index, w.expr(s.Value))
	case *lower.Do:
		w.line("%s;", w.expr(s.X))
	case *lower.If:
		w.ifChain(s, false)
	case *lower.While:
		if len(s.Pre.Stmts) == 0 {
			w.line("while (%s) {", w.cond(s.Cond))
			w.ind++
		} else {
			w.line("while (true) {")
			w.ind++
			w.block(s.Pre)
			w.line("if (!(%s)) break;", w.expr(s.Cond))
		}
		w.block(s.Body)
		w.ind--
		w.line("}")
	case *lower.ForRange:
		v := s.Var.CName
		st, en, sp := w.expr(s.Start), w.expr(s.End), w.expr(s.Step)
		w.line("for (long long %s = %s; %s > 0 ? (%s < %s) : (%s > %s); %s += %s) {", v, st, sp, v, en, v, en, v, sp)
		w.ind++
		w.block(s.Body)
		w.ind--
		w.line("}")
	case *lower.ForIndex:
		i := tempName(s.Idx)
		test := fmt.Sprintf("%s < %s", i, w.expr(s.Len))
		if s.Sized != nil {
			fn := "q_for_more"
			if s.Sized.Rep() == lower.RepList {
				fn = "q_for_list_more"
			}
			test = fmt.Sprintf("%s(%s, %s, %s, %s)", fn, w.expr(s.Sized), w.expr(s.Len), i, w.expr(s.Loc))
		}
		w.line("for (long long %s = 0; %s; %s++) {", i, test, i)
		w.ind++
		w.block(s.Body)
		w.ind--
		w.line("}")
	case *lower.Break:
		w.line("break;")
	case *lower.Continue:
		w.line("continue;")
	case *lower.Return:
		w.line("return %s;", w.expr(s.Value))
	case *lower.SetLoc:
		w.line("q_set_source_loc(&_qloc[%d]);", s.Index)
	default:
		panic(fmt.Sprintf("emit: unhandled statement %T", s))
	}
}

// ifChain prints an if statement; an else block holding only another if is
// printed as `else if`.
func (w *printer) ifChain(s *lower.If, chained bool) {
	if chained {
		w.b.WriteString(fmt.Sprintf("if (%s) {\n", w.cond(s.Cond)))
	} else {
		w.line("if (%s) {", w.cond(s.Cond))
	}
	w.ind++
	w.block(s.Then)
	w.ind--
	if s.Else != nil && len(s.Else.Stmts) > 0 {
		if len(s.Else.Stmts) == 1 {
			if next, ok := s.Else.Stmts[0].(*lower.If); ok {
				w.b.WriteString(strings.Repeat("    ", w.ind) + "} else ")
				w.ifChain(next, true)
				return
			}
		}
		w.line("} else {")
		w.ind++
		w.block(s.Else)
		w.ind--
	}
	w.line("}")
}

// cond prints a condition; a comparison loses its own parentheses, which
// clang warns about inside if (...).
func (w *printer) cond(e lower.Expr) string {
	switch c := e.(type) {
	case *lower.Compare:
		return fmt.Sprintf("%s %s %s", w.expr(c.L), c.Op, w.expr(c.X))
	case *lower.TagIs:
		return fmt.Sprintf("%s.type == QValue::%s", w.expr(c.X), c.Tag)
	}
	return w.expr(e)
}

var boxFns = map[lower.Rep]string{
	lower.RepInt:    "qv_int",
	lower.RepFloat:  "qv_float",
	lower.RepBool:   "qv_bool",
	lower.RepStr:    "q_box_native_str",
	lower.RepVector: "qv_vector_ptr",
	lower.RepList:   "qv_list_ptr",
	lower.RepDict:   "qv_dict_ptr",
	lower.RepFunc:   "qv_closure_ptr",
	lower.RepStruct: "qv_struct",
	lower.RepTable:  "qv_table_ptr",
}

var adaptFns = map[lower.Rep]string{
	lower.RepInt:    "q_as_int",
	lower.RepFloat:  "q_as_float",
	lower.RepBool:   "q_as_bool",
	lower.RepStr:    "q_as_str",
	lower.RepVector: "q_as_vector",
	lower.RepList:   "q_as_list",
	lower.RepDict:   "q_as_dict",
	lower.RepFunc:   "q_as_closure",
	lower.RepTable:  "q_as_table",
}

var unboxFields = map[lower.Rep]string{
	lower.RepInt:   "int_val",
	lower.RepFloat: "float_val",
	lower.RepBool:  "bool_val",
}

var checkedArith = map[string]string{"+": "q_checked_add", "-": "q_checked_sub", "*": "q_checked_mul", "%": "q_checked_mod"}

func (w *printer) exprs(es []lower.Expr) string {
	parts := make([]string, len(es))
	for i, e := range es {
		parts[i] = w.expr(e)
	}
	return strings.Join(parts, ", ")
}

func (w *printer) expr(e lower.Expr) string {
	switch e := e.(type) {
	case *lower.Const:
		return e.Text
	case *lower.TempRef:
		return tempName(e.T)
	case *lower.VarRef:
		if e.Sym.Storage == ir.StorageCell {
			if e.Checked {
				return fmt.Sprintf("q_cell_get(%s, \"%s\")", e.Sym.CName, esc(e.Display))
			}
			return e.Sym.CName + "->value"
		}
		return e.Sym.CName
	case *lower.ParamRef:
		return e.P.CName
	case *lower.Call:
		return fmt.Sprintf("%s(%s)", e.Fn, w.exprs(e.Args))
	case *lower.Box:
		r := e.X.Rep()
		if r == lower.RepValue {
			return w.expr(e.X)
		}
		fn, ok := boxFns[r]
		if !ok {
			panic(fmt.Sprintf("emit: can't box %s", r))
		}
		return fmt.Sprintf("%s(%s)", fn, w.expr(e.X))
	case *lower.Unbox:
		return fmt.Sprintf("(%s).data.%s", w.expr(e.X), unboxFields[e.To])
	case *lower.Cast:
		return fmt.Sprintf("(%s)(%s)", e.To, w.expr(e.X))
	case *lower.Adapt:
		fn, ok := adaptFns[e.To]
		if !ok {
			return w.expr(e.X)
		}
		return fmt.Sprintf("%s(%s)", fn, w.expr(e.X))
	case *lower.Check:
		x := w.expr(e.X)
		args := strings.Join(e.Args, ", ")
		switch e.Kind {
		case lower.CheckScalar:
			return fmt.Sprintf("q_%s_%s(%s, %s)", e.Family, e.Scalar, x, args)
		case lower.CheckStruct:
			return fmt.Sprintf("q_%s_struct(%s, &%s, %s)", e.Family, x, e.StructDef, args)
		case lower.CheckTable:
			def := "nullptr"
			if e.StructDef != "" {
				def = "&" + e.StructDef
			}
			return fmt.Sprintf("q_%s_table(%s, %s, \"%s\", %s)", e.Family, x, def, esc(e.Key), args)
		}
		return fmt.Sprintf("q_%s_check(%s, QValue::%s, %t, \"%s\", %s)", e.Family, x, e.Tag, e.AllowNull, e.Key, args)
	case *lower.Arith:
		l, r := w.expr(e.L), w.expr(e.X)
		if e.R == lower.RepInt {
			if fn, ok := checkedArith[e.Op]; ok {
				return fmt.Sprintf("%s(%s, %s)", fn, l, r)
			}
		}
		if e.R == lower.RepFloat && e.Op == "/" {
			return fmt.Sprintf("q_checked_fdiv((double)(%s), (double)(%s))", l, r)
		}
		return fmt.Sprintf("((%s)(%s) %s (%s)(%s))", e.R, l, e.Op, e.R, r)
	case *lower.Neg:
		if e.Checked && e.X.Rep() == lower.RepInt {
			return fmt.Sprintf("q_checked_neg(%s)", w.expr(e.X))
		}
		return fmt.Sprintf("(-%s)", w.expr(e.X))
	case *lower.Compare:
		return fmt.Sprintf("(%s %s %s)", w.expr(e.L), e.Op, w.expr(e.X))
	case *lower.Not:
		return fmt.Sprintf("(!%s)", w.expr(e.X))
	case *lower.Truthy:
		return fmt.Sprintf("q_truthy(%s)", w.expr(e.X))
	case *lower.AndAlso:
		return fmt.Sprintf("(%s && %s)", w.expr(e.L), w.expr(e.X))
	case *lower.OrElse:
		return fmt.Sprintf("(%s || %s)", w.expr(e.L), w.expr(e.X))
	case *lower.Select:
		return fmt.Sprintf("(%s ? %s : %s)", w.expr(e.C), w.expr(e.T), w.expr(e.F))
	case *lower.FuncValue:
		return fmt.Sprintf("qv_func((void*)%s, &%s)", e.Fn, e.Info)
	case *lower.TagIs:
		return fmt.Sprintf("(%s.type == QValue::%s)", w.expr(e.X), e.Tag)
	case *lower.LocRef:
		return fmt.Sprintf("&_qloc[%d]", e.Index)
	case *lower.VecRef:
		return fmt.Sprintf("*%s.data.vector_val", w.expr(e.X))
	case *lower.VecCount:
		return fmt.Sprintf("(long long)%s.count", w.expr(e.V))
	case *lower.VecElem:
		v, i := w.expr(e.V), w.expr(e.Idx)
		switch e.R {
		case lower.RepInt:
			return fmt.Sprintf("static_cast<long long>(q_vec_i64_data(%s)[%s])", v, i)
		case lower.RepFloat:
			return fmt.Sprintf("q_vec_f64_data(%s)[%s]", v, i)
		}
		return fmt.Sprintf("q_vec_bool_at(%s, %s)", v, i)
	case *lower.InitList:
		return fmt.Sprintf("%s{%s}", e.Type, w.exprs(e.Elems))
	case *lower.MakeClosure:
		panic("emit: a closure must be bound by a Let")
	}
	panic(fmt.Sprintf("emit: unhandled expression %T", e))
}
