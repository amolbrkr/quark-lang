package codegen_test

import (
	"strings"
	"testing"

	"quark/ast"
	"quark/codegen"
	"quark/internal/testutil"
	"quark/ir"
)

func TestCodegen_EmitsListConstruction(t *testing.T) {
	res := testutil.GenerateCPP("x = list [1, 2, 3]\n")
	if len(res.ParserErrors) > 0 {
		t.Fatalf("unexpected parse errors: %v", res.ParserErrors)
	}
	if len(res.TypeErrors) > 0 {
		t.Fatalf("unexpected type errors: %v", res.TypeErrors)
	}
	if !strings.Contains(res.CPP, "qv_list") || !strings.Contains(res.CPP, "q_push") {
		t.Fatalf("expected list codegen to contain qv_list and q_push")
	}
}

func TestCodegen_EmitsDictHelpers(t *testing.T) {
	res := testutil.GenerateCPP("d = dict { a: 1 }\nprintln(d.get('a'))\n")
	if len(res.ParserErrors) > 0 {
		t.Fatalf("unexpected parse errors: %v", res.ParserErrors)
	}
	if len(res.TypeErrors) > 0 {
		t.Fatalf("unexpected type errors: %v", res.TypeErrors)
	}
	if !strings.Contains(res.CPP, "q_dget") {
		t.Fatalf("expected codegen to call q_dget, cpp=\n%s", res.CPP)
	}
}

func TestCodegen_EmitsSplit(t *testing.T) {
	res := testutil.GenerateCPP("println('a,b'.split(','))\n")
	if len(res.ParserErrors) > 0 {
		t.Fatalf("unexpected parse errors: %v", res.ParserErrors)
	}
	if len(res.TypeErrors) > 0 {
		t.Fatalf("unexpected type errors: %v", res.TypeErrors)
	}
	if !strings.Contains(res.CPP, "q_split") {
		t.Fatalf("expected codegen to call q_split, cpp=\n%s", res.CPP)
	}
}

func TestCodegen_EmitsSslice(t *testing.T) {
	res := testutil.GenerateCPP("println('hello'.slice(1, 4))\n")
	if len(res.ParserErrors) > 0 {
		t.Fatalf("unexpected parse errors: %v", res.ParserErrors)
	}
	if len(res.TypeErrors) > 0 {
		t.Fatalf("unexpected type errors: %v", res.TypeErrors)
	}
	if !strings.Contains(res.CPP, "q_str_slice") {
		t.Fatalf("expected codegen to call q_str_slice, cpp=\n%s", res.CPP)
	}
}

func TestCodegen_EmitsSjoin(t *testing.T) {
	res := testutil.GenerateCPP("parts = list ['a', 'b']\nprintln(parts.join(','))\n")
	if len(res.ParserErrors) > 0 {
		t.Fatalf("unexpected parse errors: %v", res.ParserErrors)
	}
	if len(res.TypeErrors) > 0 {
		t.Fatalf("unexpected type errors: %v", res.TypeErrors)
	}
	if !strings.Contains(res.CPP, "q_str_join") {
		t.Fatalf("expected codegen to call q_str_join, cpp=\n%s", res.CPP)
	}
}

func TestCodegen_SslicePipeChain(t *testing.T) {
	res := testutil.GenerateCPP("s = 'hello world'\nprintln(s.slice(6, 11).upper())\n")
	if len(res.ParserErrors) > 0 {
		t.Fatalf("unexpected parse errors: %v", res.ParserErrors)
	}
	if len(res.TypeErrors) > 0 {
		t.Fatalf("unexpected type errors: %v", res.TypeErrors)
	}
	if !strings.Contains(res.CPP, "q_str_slice") {
		t.Fatalf("expected pipe chain to emit q_str_slice, cpp=\n%s", res.CPP)
	}
	if !strings.Contains(res.CPP, "q_upper") {
		t.Fatalf("expected pipe chain to emit q_upper, cpp=\n%s", res.CPP)
	}
}

func TestCodegen_EmitsVectorLiteral(t *testing.T) {
	res := testutil.GenerateCPP("v = vector [1, 2, 3]\n")
	if len(res.ParserErrors) > 0 {
		t.Fatalf("unexpected parse errors: %v", res.ParserErrors)
	}
	if len(res.TypeErrors) > 0 {
		t.Fatalf("unexpected type errors: %v", res.TypeErrors)
	}
	if !strings.Contains(res.CPP, "qv_list") || !strings.Contains(res.CPP, "q_push") || !strings.Contains(res.CPP, "q_to_vector") {
		t.Fatalf("expected codegen to lower vector literal through list + q_to_vector, cpp=\n%s", res.CPP)
	}
}

func TestCodegen_EmitsToVectorBuiltin(t *testing.T) {
	res := testutil.GenerateCPP("xs = list [1, 2, 3]\nv = vfrom_list(xs)\n")
	if len(res.ParserErrors) > 0 {
		t.Fatalf("unexpected parse errors: %v", res.ParserErrors)
	}
	if len(res.TypeErrors) > 0 {
		t.Fatalf("unexpected type errors: %v", res.TypeErrors)
	}
	if !strings.Contains(res.CPP, "q_to_vector") {
		t.Fatalf("expected codegen to call q_to_vector, cpp=\n%s", res.CPP)
	}
}

func TestCodegen_ForLoopUsesGenericLenForVector(t *testing.T) {
	res := testutil.GenerateCPP("for x in vfrom_list(range(3)):\n    println(x)\n")
	if len(res.ParserErrors) > 0 {
		t.Fatalf("unexpected parse errors: %v", res.ParserErrors)
	}
	if len(res.TypeErrors) > 0 {
		t.Fatalf("unexpected type errors: %v", res.TypeErrors)
	}
	if !strings.Contains(res.CPP, "q_len(") {
		t.Fatalf("expected for-loop codegen to use q_len for iterable size, cpp=\n%s", res.CPP)
	}
	if !strings.Contains(res.CPP, "q_iter_get(") {
		t.Fatalf("expected for-loop codegen to use q_iter_get for iterable access, cpp=\n%s", res.CPP)
	}
	if strings.Contains(res.CPP, ".data.list_val->size()") {
		t.Fatalf("for-loop should not assume list storage directly, cpp=\n%s", res.CPP)
	}
}

func TestCodegen_EmitsResultHelperBuiltins(t *testing.T) {
	res := testutil.GenerateCPP("r = ok 1\nprintln(is_ok(r))\nprintln(is_err(r))\nx = unwrap(r)\n")
	if len(res.ParserErrors) > 0 {
		t.Fatalf("unexpected parse errors: %v", res.ParserErrors)
	}
	if len(res.TypeErrors) > 0 {
		t.Fatalf("unexpected type errors: %v", res.TypeErrors)
	}
	if !strings.Contains(res.CPP, "q_is_ok_builtin") {
		t.Fatalf("expected codegen to call q_is_ok_builtin, cpp=\n%s", res.CPP)
	}
	if !strings.Contains(res.CPP, "q_is_err_builtin") {
		t.Fatalf("expected codegen to call q_is_err_builtin, cpp=\n%s", res.CPP)
	}
	if !strings.Contains(res.CPP, "q_unwrap") {
		t.Fatalf("expected codegen to call q_unwrap, cpp=\n%s", res.CPP)
	}
}

func TestCodegen_WhenResultPatternBindingScopeRegression(t *testing.T) {
	program := "fn safe_div(a, b) ->\n" +
		"    if b == 0:\n" +
		"        err 'divide by zero'\n" +
		"    else:\n" +
		"        ok a / b\n" +
		"\n" +
		"fn compute(x, y) ->\n" +
		"    when safe_div(x, y):\n" +
		"        ok v -> println(v)\n" +
		"        err e -> dict { error: e }\n"

	res := testutil.GenerateCPP(program)
	if len(res.ParserErrors) > 0 {
		t.Fatalf("unexpected parse errors: %v", res.ParserErrors)
	}
	if len(res.TypeErrors) > 0 {
		t.Fatalf("unexpected type errors: %v", res.TypeErrors)
	}

	bindDecl := strings.Index(res.CPP, "QCell* quark_e = q_new_cell(q_result_error")
	bindUse := strings.Index(res.CPP, "q_dict_set")
	if bindDecl == -1 || bindUse == -1 {
		t.Fatalf("expected generated code to include err-binding and dict set usage, cpp=\n%s", res.CPP)
	}
	if bindDecl > bindUse {
		t.Fatalf("expected err-binding declaration before usage in when arm, cpp=\n%s", res.CPP)
	}
}

func TestCodegen_PanicsOnMissingCallPlan(t *testing.T) {
	analyzer, node, parseErrs, typeErrs := testutil.Analyze("println(len('x'))\n")
	if len(parseErrs) > 0 {
		t.Fatalf("unexpected parse errors: %v", parseErrs)
	}
	if len(typeErrs) > 0 {
		t.Fatalf("unexpected type errors: %v", typeErrs)
	}

	gen := codegen.New()
	gen.SetCaptures(analyzer.GetCaptures())
	gen.SetCallPlans(map[*ast.TreeNode]*ir.CallPlan{})

	defer func() {
		r := recover()
		if r == nil {
			t.Fatalf("expected panic for missing call plan")
		}
		msg := ""
		switch v := r.(type) {
		case string:
			msg = v
		case error:
			msg = v.Error()
		default:
			msg = "unknown panic"
		}
		if !strings.Contains(msg, "INV-CALLPLAN-MISSING") {
			t.Fatalf("expected INV-CALLPLAN-MISSING panic, got: %s", msg)
		}
	}()

	_ = gen.Generate(node)
}

func TestCodegen_NoBuiltinArityFallbackBranch(t *testing.T) {
	res := testutil.GenerateCPP("println(len('abc'))\n")
	if len(res.ParserErrors) > 0 {
		t.Fatalf("unexpected parse errors: %v", res.ParserErrors)
	}
	if len(res.TypeErrors) > 0 {
		t.Fatalf("unexpected type errors: %v", res.TypeErrors)
	}
	if strings.Contains(res.CPP, "expects at least") || strings.Contains(res.CPP, "expects at most") {
		t.Fatalf("generated code should not contain codegen-side builtin arity fallback branches, cpp=\n%s", res.CPP)
	}
}

func TestCodegen_ClosureCallEmitsDirectCallN(t *testing.T) {
	// A closure call with 1 arg should emit q_call1, not q_calln
	res := testutil.GenerateCPP("f = fn(x) -> x * 2\nf(5)\n")
	if len(res.ParserErrors) > 0 {
		t.Fatalf("unexpected parse errors: %v", res.ParserErrors)
	}
	if len(res.TypeErrors) > 0 {
		t.Fatalf("unexpected type errors: %v", res.TypeErrors)
	}
	if !strings.Contains(res.CPP, "q_call1(") {
		t.Fatalf("expected closure call to emit q_call1, cpp=\n%s", res.CPP)
	}
	if strings.Contains(res.CPP, "q_calln(") {
		t.Fatalf("expected no q_calln for known-arity closure call, cpp=\n%s", res.CPP)
	}
}

func TestCodegen_ClosureCallZeroArgs(t *testing.T) {
	// A closure call with 0 args should emit q_call0
	res := testutil.GenerateCPP("f = fn() -> 42\nf()\n")
	if len(res.ParserErrors) > 0 {
		t.Fatalf("unexpected parse errors: %v", res.ParserErrors)
	}
	if len(res.TypeErrors) > 0 {
		t.Fatalf("unexpected type errors: %v", res.TypeErrors)
	}
	if !strings.Contains(res.CPP, "q_call0(") {
		t.Fatalf("expected closure call to emit q_call0, cpp=\n%s", res.CPP)
	}
	if strings.Contains(res.CPP, "q_calln(") {
		t.Fatalf("expected no q_calln for known-arity closure call, cpp=\n%s", res.CPP)
	}
}

func TestCodegen_ClosurePipeEmitsDirectCallN(t *testing.T) {
	// A piped closure call with 1 explicit arg + 1 pipe input should emit q_call1
	res := testutil.GenerateCPP("f = fn(x) -> x * 2\n5 | f()\n")
	if len(res.ParserErrors) > 0 {
		t.Fatalf("unexpected parse errors: %v", res.ParserErrors)
	}
	if len(res.TypeErrors) > 0 {
		t.Fatalf("unexpected type errors: %v", res.TypeErrors)
	}
	if !strings.Contains(res.CPP, "q_call1(") {
		t.Fatalf("expected piped closure call to emit q_call1, cpp=\n%s", res.CPP)
	}
	if strings.Contains(res.CPP, "q_calln(") {
		t.Fatalf("expected no q_calln for known-arity piped closure call, cpp=\n%s", res.CPP)
	}
}

func TestCodegen_EmitClosureCallHelper(t *testing.T) {
	// Unit test the emitClosureCall helper directly
	tests := []struct {
		funcExpr string
		args     []string
		expected string
	}{
		{"f", nil, "q_call0(f)"},
		{"f", []string{}, "q_call0(f)"},
		{"f", []string{"a"}, "q_call1(f, a)"},
		{"f", []string{"a", "b"}, "q_call2(f, a, b)"},
		{"f", []string{"a", "b", "c"}, "q_call3(f, a, b, c)"},
	}
	for _, tt := range tests {
		result := codegen.EmitClosureCall(tt.funcExpr, tt.args)
		if result != tt.expected {
			t.Errorf("emitClosureCall(%q, %v) = %q, want %q", tt.funcExpr, tt.args, result, tt.expected)
		}
	}
}

func TestCodegen_ModuleQualifiedCallLowersDirectly(t *testing.T) {
	res := testutil.GenerateCPP("module math:\n    fn myfloor(x) -> x\nuse math as m\nprintln(m.myfloor(3))\n")
	if len(res.ParserErrors) > 0 {
		t.Fatalf("unexpected parse errors: %v", res.ParserErrors)
	}
	if len(res.TypeErrors) > 0 {
		t.Fatalf("unexpected type errors: %v", res.TypeErrors)
	}
	if !strings.Contains(res.CPP, "quark_myfloor->value") {
		t.Fatalf("expected module-qualified call to lower to resolved module symbol value, cpp=\n%s", res.CPP)
	}
	if strings.Contains(res.CPP, "dot-call syntax is not supported") {
		t.Fatalf("module-qualified call should not hit dot-call fallback branch, cpp=\n%s", res.CPP)
	}
}
