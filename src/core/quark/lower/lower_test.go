package lower_test

import (
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"quark/driver"
	"quark/emit"
	"quark/ir"
	"quark/lower"
	"quark/types"
)

func testfilesDir(t *testing.T) string {
	t.Helper()
	_, here, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("can't locate test file")
	}
	return filepath.Join(filepath.Dir(here), "..", "..", "..", "testfiles")
}

// TestLower_SmokeCorpusVerifies lowers every smoke program that passes the
// checker, verifies the IR and prints it.
func TestLower_SmokeCorpusVerifies(t *testing.T) {
	t.Setenv("QUARK_STDLIB_ROOT", filepath.Join(testfilesDir(t), "..", "core", "stdlib"))
	files, err := filepath.Glob(filepath.Join(testfilesDir(t), "smoke_*.qrk"))
	if err != nil || len(files) == 0 {
		t.Fatalf("no smoke programs found: %v", err)
	}
	for _, path := range files {
		path := path
		t.Run(filepath.Base(path), func(t *testing.T) {
			u := driver.CompileFile(path, driver.StageCheck)
			if u.Failed() || u.Analysis == nil {
				t.Skipf("program is rejected before lowering: %v %v", u.Diagnostics, u.Err)
			}
			if err := driver.Validate(u.AST, u.Analysis); err != nil {
				t.Skip("program fails the pre-lowering invariants")
			}
			prog, err := lower.Lower(u.AST, u.Analysis, path)
			if err != nil {
				t.Fatalf("lower: %v", err)
			}
			if err := lower.Verify(prog); err != nil {
				t.Fatalf("verify: %v", err)
			}
			if cpp := emit.Program(prog); !strings.Contains(cpp, "int main(") {
				t.Fatalf("emitted C++ has no main")
			}
		})
	}
}

func sym(name string, st ir.Storage) *types.Symbol {
	return &types.Symbol{Name: name, CName: "quark_" + name, Storage: st, Kind: types.SymLocal}
}

func mainProgram(stmts ...lower.Stmt) *lower.Program {
	return &lower.Program{Main: &lower.Block{Stmts: stmts}}
}

// TestVerify_RejectsMalformedIR checks that the verifier catches each kind of
// lowering mistake it is meant to catch.
func TestVerify_RejectsMalformedIR(t *testing.T) {
	x := sym("x", ir.StorageInt)
	c := sym("c", ir.StorageCell)
	one := &lower.Const{R: lower.RepInt, Text: "1"}
	val := &lower.Const{R: lower.RepValue, Text: "qv_int(1)"}
	tmp := &lower.Temp{ID: 1, R: lower.RepInt}
	lst := &lower.Temp{ID: 3, R: lower.RepList}
	bindList := &lower.Let{T: lst, Value: &lower.Const{R: lower.RepList, Text: "nullptr"}}
	f := &lower.Func{CName: "quark_f", Ret: "QValue", Params: []*lower.Param{{CName: "_a", CType: "QValue"}}, Body: &lower.Block{}}
	cases := []struct {
		name string
		prog *lower.Program
		code string
	}{
		{"read before declaration", mainProgram(&lower.Do{X: &lower.VarRef{Sym: x}}), "INV-IR-SCOPE"},
		{"read outside the declaring block", mainProgram(
			&lower.If{Cond: &lower.Const{R: lower.RepBool, Text: "true"}, Then: &lower.Block{Stmts: []lower.Stmt{&lower.DeclVar{Sym: x, Value: one}}}},
			&lower.Do{X: &lower.VarRef{Sym: x}},
		), "INV-IR-SCOPE"},
		{"declared twice in one scope", mainProgram(&lower.DeclVar{Sym: x, Value: one}, &lower.DeclVar{Sym: x, Value: one}), "INV-IR-SCOPE"},
		{"temporary read before its let", mainProgram(&lower.Do{X: &lower.TempRef{T: tmp}}), "INV-IR-SCOPE"},
		{"int stored in a QValue variable", mainProgram(&lower.DeclVar{Sym: sym("v", ir.StorageValue), Value: one}), "INV-IR-REP"},
		{"QValue condition", mainProgram(&lower.If{Cond: &lower.Const{R: lower.RepValue, Text: "qv_null()"}, Then: &lower.Block{}}), "INV-IR-REP"},
		{"q_truthy of a raw bool", mainProgram(&lower.Do{X: &lower.Truthy{X: &lower.Const{R: lower.RepBool, Text: "true"}}}), "INV-IR-REP"},
		{"unbox of a raw int", mainProgram(&lower.Do{X: &lower.Unbox{X: one, To: lower.RepInt}}), "INV-IR-REP"},
		{"break outside a loop", mainProgram(&lower.Break{}), "INV-IR-LOOP"},
		{"return in main", mainProgram(&lower.Return{Value: one}), "INV-IR-RETURN"},
		{"closure not bound by a let", mainProgram(&lower.Do{X: &lower.MakeClosure{Fn: "quark_f", Info: "_qfi_f"}}), "INV-IR-NODE"},
		{"closure over a cell out of scope", mainProgram(&lower.Let{T: &lower.Temp{ID: 2, R: lower.RepValue}, Value: &lower.MakeClosure{Fn: "quark_f", Info: "_qfi_f", Cells: []*types.Symbol{c}}}), "INV-IR-SCOPE"},
		{"loop length re-evaluated", mainProgram(&lower.ForIndex{Idx: tmp, Len: &lower.Call{Fn: "q_n", R: lower.RepInt}, Body: &lower.Block{}}), "INV-IR-LOOP"},
		{"wrong argument count", &lower.Program{Decls: []*lower.Func{f}, Funcs: []*lower.Func{f},
			Main: &lower.Block{Stmts: []lower.Stmt{&lower.Do{X: &lower.Call{Fn: "quark_f", Args: []lower.Expr{&lower.Const{R: "std::nullptr_t", Text: "nullptr"}}, R: lower.RepValue}}}}}, "INV-IR-CALL"},
		{"unbox without a proof", mainProgram(&lower.Do{X: &lower.Unbox{X: val, To: lower.RepInt}}), "INV-IR-UNBOX"},
		{"unbox of a float typed value as an int", mainProgram(&lower.Do{X: &lower.Unbox{X: val, To: lower.RepInt, Proof: lower.ProofType, Type: types.TypeFloat}}), "INV-IR-UNBOX"},
		{"unbox of a union typed value", mainProgram(&lower.Do{X: &lower.Unbox{X: val, To: lower.RepInt, Proof: lower.ProofType, Type: &types.UnionType{Options: []types.Type{types.TypeInt, types.TypeString}}}}), "INV-IR-UNBOX"},
		// q_eq of a vec is a vec: a call's result is never unboxed on trust.
		{"unbox of a runtime call's result", mainProgram(&lower.Do{X: &lower.Unbox{X: &lower.Call{Fn: "q_eq", Args: []lower.Expr{val, val}, R: lower.RepValue}, To: lower.RepBool}}), "INV-IR-UNBOX"},
		{"element proof of a value that isn't an element", mainProgram(&lower.Do{X: &lower.Unbox{X: val, To: lower.RepInt, Proof: lower.ProofElement, Type: types.TypeInt}}), "INV-IR-UNBOX"},
		{"source location outside the table", mainProgram(&lower.SetLoc{Index: 0}), "INV-IR-NODE"},
		{"sized loop over an int", mainProgram(&lower.ForIndex{Idx: tmp, Len: one, Sized: one, Loc: &lower.Const{R: lower.RepLoc, Text: "nullptr"}, Body: &lower.Block{}}), "INV-IR-REP"},
		{"sized loop list re-evaluated", mainProgram(&lower.ForIndex{Idx: tmp, Len: one, Sized: &lower.Call{Fn: "q_for_list", R: lower.RepList}, Loc: &lower.Const{R: lower.RepLoc, Text: "nullptr"}, Body: &lower.Block{}}), "INV-IR-LOOP"},
		{"sized loop without a location", mainProgram(bindList,
			&lower.ForIndex{Idx: tmp, Len: one, Sized: &lower.TempRef{T: lst}, Body: &lower.Block{}}), "INV-IR-NODE"},
		{"loop location outside the table", mainProgram(bindList,
			&lower.ForIndex{Idx: tmp, Len: one, Sized: &lower.TempRef{T: lst}, Loc: &lower.LocRef{Index: 2}, Body: &lower.Block{}}), "INV-IR-NODE"},
		{"q_call2 with one argument", mainProgram(&lower.Do{X: &lower.Call{Fn: "q_call2", Args: []lower.Expr{&lower.Const{R: lower.RepValue, Text: "qv_null()"}, &lower.Const{R: lower.RepValue, Text: "qv_null()"}}, R: lower.RepValue}}), "INV-IR-CALL"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := lower.Verify(tc.prog)
			if err == nil {
				t.Fatalf("Verify accepted malformed IR")
			}
			if !strings.Contains(err.Error(), tc.code) {
				t.Fatalf("Verify: %v, want %s", err, tc.code)
			}
		})
	}
}
