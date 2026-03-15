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
	res := testutil.GenerateCPP("d = dict { a: 1 }\nprintln(dget(d, 'a'))\n")
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
	res := testutil.GenerateCPP("println(split('a,b', ','))\n")
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
	res := testutil.GenerateCPP("xs = list [1, 2, 3]\nv = to_vector(xs)\n")
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
	res := testutil.GenerateCPP("for x in to_vector(range(3)):\n    println(x)\n")
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
