package emit_test

import (
	"strings"
	"testing"

	"quark/ast"
	"quark/internal/testutil"
	"quark/ir"
	"quark/lower"
)

func TestEmit_EmitsListConstruction(t *testing.T) {
	res := testutil.GenerateCPP("x = list [1, 2, 3]\n")
	if len(res.ParserErrors) > 0 {
		t.Fatalf("unexpected parse errors: %v", res.ParserErrors)
	}
	if len(res.TypeErrors) > 0 {
		t.Fatalf("unexpected type errors: %v", res.TypeErrors)
	}
	if !strings.Contains(res.CPP, "qv_list") || !strings.Contains(res.CPP, "q_push") {
		t.Fatalf("expected list construction to contain qv_list and q_push")
	}
}

func TestEmit_EmitsDictHelpers(t *testing.T) {
	res := testutil.GenerateCPP("d = dict { a: 1 }\nprint(d.get('a'))\n")
	if len(res.ParserErrors) > 0 {
		t.Fatalf("unexpected parse errors: %v", res.ParserErrors)
	}
	if len(res.TypeErrors) > 0 {
		t.Fatalf("unexpected type errors: %v", res.TypeErrors)
	}
	if !strings.Contains(res.CPP, "q_dget") {
		t.Fatalf("expected the C++ to call q_dget, cpp=\n%s", res.CPP)
	}
}

func TestEmit_EmitsSplit(t *testing.T) {
	res := testutil.GenerateCPP("print('a,b'.split(','))\n")
	if len(res.ParserErrors) > 0 {
		t.Fatalf("unexpected parse errors: %v", res.ParserErrors)
	}
	if len(res.TypeErrors) > 0 {
		t.Fatalf("unexpected type errors: %v", res.TypeErrors)
	}
	if !strings.Contains(res.CPP, "q_split") {
		t.Fatalf("expected the C++ to call q_split, cpp=\n%s", res.CPP)
	}
}

func TestEmit_EmitsSslice(t *testing.T) {
	res := testutil.GenerateCPP("print('hello'.slice(1, 4))\n")
	if len(res.ParserErrors) > 0 {
		t.Fatalf("unexpected parse errors: %v", res.ParserErrors)
	}
	if len(res.TypeErrors) > 0 {
		t.Fatalf("unexpected type errors: %v", res.TypeErrors)
	}
	if !strings.Contains(res.CPP, "q_str_slice") {
		t.Fatalf("expected the C++ to call q_str_slice, cpp=\n%s", res.CPP)
	}
}

func TestEmit_EmitsSjoin(t *testing.T) {
	res := testutil.GenerateCPP("parts = list ['a', 'b']\nprint(parts.join(','))\n")
	if len(res.ParserErrors) > 0 {
		t.Fatalf("unexpected parse errors: %v", res.ParserErrors)
	}
	if len(res.TypeErrors) > 0 {
		t.Fatalf("unexpected type errors: %v", res.TypeErrors)
	}
	if !strings.Contains(res.CPP, "q_str_join") {
		t.Fatalf("expected the C++ to call q_str_join, cpp=\n%s", res.CPP)
	}
}

func TestEmit_SslicePipeChain(t *testing.T) {
	res := testutil.GenerateCPP("s = 'hello world'\nprint(s.slice(6, 11).upper())\n")
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

func TestEmit_EmitsVectorLiteral(t *testing.T) {
	// Int vector literals build a typed vector directly, without an
	// intermediate list.
	res := testutil.GenerateCPP("v = vec [1, 2, 3]\n")
	if len(res.ParserErrors) > 0 {
		t.Fatalf("unexpected parse errors: %v", res.ParserErrors)
	}
	if len(res.TypeErrors) > 0 {
		t.Fatalf("unexpected type errors: %v", res.TypeErrors)
	}
	if !strings.Contains(res.CPP, "qv_vector_i64(3)") || strings.Count(res.CPP, "q_vec_push_i64(") != 3 {
		t.Fatalf("expected vector literal to build an i64 vector with 3 pushes, cpp=\n%s", res.CPP)
	}
	if strings.Contains(res.CPP, "q_to_vec") {
		t.Fatalf("expected vector literal not to round-trip through a list, cpp=\n%s", res.CPP)
	}
}

func TestEmit_EmitsToVectorBuiltin(t *testing.T) {
	res := testutil.GenerateCPP("xs = list [1, 2, 3]\nv = xs.to_vec()\n")
	if len(res.ParserErrors) > 0 {
		t.Fatalf("unexpected parse errors: %v", res.ParserErrors)
	}
	if len(res.TypeErrors) > 0 {
		t.Fatalf("unexpected type errors: %v", res.TypeErrors)
	}
	if !strings.Contains(res.CPP, "q_to_vec") {
		t.Fatalf("expected the C++ to call q_to_vec, cpp=\n%s", res.CPP)
	}
}

func TestEmit_ForLoopUsesGenericLenForVector(t *testing.T) {
	res := testutil.GenerateCPP("for x in range(3).to_vec():\n    print(x)\n")
	if len(res.ParserErrors) > 0 {
		t.Fatalf("unexpected parse errors: %v", res.ParserErrors)
	}
	if len(res.TypeErrors) > 0 {
		t.Fatalf("unexpected type errors: %v", res.TypeErrors)
	}
	if !strings.Contains(res.CPP, "q_for_len(") {
		t.Fatalf("expected the for loop to use q_for_len for iterable size, cpp=\n%s", res.CPP)
	}
	if !strings.Contains(res.CPP, "q_iter_get(") {
		t.Fatalf("expected the for loop to use q_iter_get for iterable access, cpp=\n%s", res.CPP)
	}
	if strings.Contains(res.CPP, ".data.list_val->size()") {
		t.Fatalf("for-loop should not assume list storage directly, cpp=\n%s", res.CPP)
	}
}

func TestEmit_EmitsResultHelperBuiltins(t *testing.T) {
	res := testutil.GenerateCPP("r = ok 1\nprint(is_ok(r))\nprint(is_err(r))\nx = unwrap(r)\n")
	if len(res.ParserErrors) > 0 {
		t.Fatalf("unexpected parse errors: %v", res.ParserErrors)
	}
	if len(res.TypeErrors) > 0 {
		t.Fatalf("unexpected type errors: %v", res.TypeErrors)
	}
	if !strings.Contains(res.CPP, "q_is_ok_builtin") {
		t.Fatalf("expected the C++ to call q_is_ok_builtin, cpp=\n%s", res.CPP)
	}
	if !strings.Contains(res.CPP, "q_is_err_builtin") {
		t.Fatalf("expected the C++ to call q_is_err_builtin, cpp=\n%s", res.CPP)
	}
	if !strings.Contains(res.CPP, "q_unwrap") {
		t.Fatalf("expected the C++ to call q_unwrap, cpp=\n%s", res.CPP)
	}
}

func TestEmit_WhenResultPatternBindingScopeRegression(t *testing.T) {
	program := "fn safe_div(a, b) ->\n" +
		"    if b == 0:\n" +
		"        err 'divide by zero'\n" +
		"    else:\n" +
		"        ok a / b\n" +
		"\n" +
		"fn compute(x, y) ->\n" +
		"    when safe_div(x, y):\n" +
		"        ok v -> print(v)\n" +
		"        err e -> dict { error: e }\n"

	res := testutil.GenerateCPP(program)
	if len(res.ParserErrors) > 0 {
		t.Fatalf("unexpected parse errors: %v", res.ParserErrors)
	}
	if len(res.TypeErrors) > 0 {
		t.Fatalf("unexpected type errors: %v", res.TypeErrors)
	}

	bindDecl := strings.Index(res.CPP, "QValue quark_e = q_result_error(")
	bindUse := strings.Index(res.CPP, "q_dict_set")
	if bindDecl == -1 || bindUse == -1 {
		t.Fatalf("expected generated code to include err-binding and dict set usage, cpp=\n%s", res.CPP)
	}
	if bindDecl > bindUse {
		t.Fatalf("expected err-binding declaration before usage in when arm, cpp=\n%s", res.CPP)
	}
}

func TestEmit_NoBuiltinArityFallbackBranch(t *testing.T) {
	res := testutil.GenerateCPP("print(len('abc'))\n")
	if len(res.ParserErrors) > 0 {
		t.Fatalf("unexpected parse errors: %v", res.ParserErrors)
	}
	if len(res.TypeErrors) > 0 {
		t.Fatalf("unexpected type errors: %v", res.TypeErrors)
	}
	if strings.Contains(res.CPP, "expects at least") || strings.Contains(res.CPP, "expects at most") {
		t.Fatalf("generated code should not contain back-end builtin arity fallback branches, cpp=\n%s", res.CPP)
	}
}

func TestEmit_ClosureCallEmitsDirectCallN(t *testing.T) {
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

func TestEmit_ClosureCallZeroArgs(t *testing.T) {
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

func TestEmit_ClosurePipeEmitsDirectCallN(t *testing.T) {
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

func TestEmit_ModuleQualifiedCallLowersDirectly(t *testing.T) {
	res := testutil.GenerateCPP("module math:\n    fn myfloor(x) -> x\nprint(math::myfloor(3))\n")
	if len(res.ParserErrors) > 0 {
		t.Fatalf("unexpected parse errors: %v", res.ParserErrors)
	}
	if len(res.TypeErrors) > 0 {
		t.Fatalf("unexpected type errors: %v", res.TypeErrors)
	}
	// The alias m.myfloor resolves to the module's function declaration,
	// named module__member; it captures nothing, so the call is direct.
	if !strings.Contains(res.CPP, "quark_math__myfloor(nullptr, qv_int(3))") {
		t.Fatalf("expected module-qualified call to lower to resolved module symbol value, cpp=\n%s", res.CPP)
	}
	if strings.Contains(res.CPP, "dot-call syntax is not supported") {
		t.Fatalf("module-qualified call should not hit dot-call fallback branch, cpp=\n%s", res.CPP)
	}
}

// --- Control flow ---

func TestEmit_IfElseEmitsConditional(t *testing.T) {
	// Dynamic condition: goes through q_truthy.
	res := testutil.GenerateCPP("fn f(x) ->\n    if x:\n        print('yes')\n    else:\n        print('no')\n")
	if len(res.ParserErrors) > 0 {
		t.Fatalf("unexpected parse errors: %v", res.ParserErrors)
	}
	if len(res.TypeErrors) > 0 {
		t.Fatalf("unexpected type errors: %v", res.TypeErrors)
	}
	if !strings.Contains(res.CPP, "if (q_truthy(quark_x))") {
		t.Fatalf("expected dynamic if to emit q_truthy, cpp=\n%s", res.CPP)
	}
	if !strings.Contains(res.CPP, "} else {") {
		t.Fatalf("expected else branch in output, cpp=\n%s", res.CPP)
	}

	// Scalar condition: lowered to a native C++ comparison.
	res = testutil.GenerateCPP("x = 1\nif x == 1:\n    print('yes')\nelse:\n    print('no')\n")
	if len(res.ParserErrors) > 0 {
		t.Fatalf("unexpected parse errors: %v", res.ParserErrors)
	}
	if len(res.TypeErrors) > 0 {
		t.Fatalf("unexpected type errors: %v", res.TypeErrors)
	}
	if !strings.Contains(res.CPP, "if (quark_x == 1) {") || strings.Contains(res.CPP, "q_truthy(") {
		t.Fatalf("expected scalar if to emit a native comparison, cpp=\n%s", res.CPP)
	}
}

func TestEmit_IfElseIfChain(t *testing.T) {
	src := "x = 1\nif x == 1:\n    print('a')\nelseif x == 2:\n    print('b')\nelse:\n    print('c')\n"
	res := testutil.GenerateCPP(src)
	if len(res.ParserErrors) > 0 {
		t.Fatalf("unexpected parse errors: %v", res.ParserErrors)
	}
	if len(res.TypeErrors) > 0 {
		t.Fatalf("unexpected type errors: %v", res.TypeErrors)
	}
	if !strings.Contains(res.CPP, "else if") {
		t.Fatalf("expected 'else if' in output, cpp=\n%s", res.CPP)
	}
}

func TestEmit_ForLoopEmitsIteration(t *testing.T) {
	cases := []struct {
		name, src string
		want      []string
	}{
		// A list the analyzer typed: direct element reads, raw loop variable,
		// and a test that fails if the body changes the list's length.
		{"typed list", "for x in list [1, 2, 3]:\n    print(x)\n", []string{
			"QList* _t3 = q_for_list(_t1);",
			"long long _t4 = q_list_len(_t3);",
			"q_for_list_more(_t3, _t4, _t5, &_qloc[0]); _t5++) {",
			"long long quark_x = (q_list_at(_t3, _t5)).data.int_val;",
		}},
		// An iterable of unknown type: q_iter_get, with the same length check
		// when the value turns out to be a list.
		{"untyped iterable", "fn f(xs) ->\n    for x in xs:\n        print(x)\n", []string{
			"q_for_len(",
			"q_for_more(",
			"q_iter_get(",
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			res := testutil.GenerateCPP(tc.src)
			if len(res.ParserErrors) > 0 {
				t.Fatalf("unexpected parse errors: %v", res.ParserErrors)
			}
			if len(res.TypeErrors) > 0 {
				t.Fatalf("unexpected type errors: %v", res.TypeErrors)
			}
			for _, w := range tc.want {
				if !strings.Contains(res.CPP, w) {
					t.Fatalf("expected %q, cpp=\n%s", w, res.CPP)
				}
			}
		})
	}
}

func TestEmit_WhileLoopEmitsWhile(t *testing.T) {
	// Dynamic condition: goes through q_truthy.
	res := testutil.GenerateCPP("fn f(x) ->\n    while x:\n        x = x - 1\n")
	if len(res.ParserErrors) > 0 {
		t.Fatalf("unexpected parse errors: %v", res.ParserErrors)
	}
	if len(res.TypeErrors) > 0 {
		t.Fatalf("unexpected type errors: %v", res.TypeErrors)
	}
	if !strings.Contains(res.CPP, "while (q_truthy(") {
		t.Fatalf("expected dynamic while loop with q_truthy, cpp=\n%s", res.CPP)
	}

	// Scalar condition and body: lowered to native C++.
	res = testutil.GenerateCPP("x = 10\nwhile x > 0:\n    x = x - 1\n")
	if len(res.ParserErrors) > 0 {
		t.Fatalf("unexpected parse errors: %v", res.ParserErrors)
	}
	if len(res.TypeErrors) > 0 {
		t.Fatalf("unexpected type errors: %v", res.TypeErrors)
	}
	if !strings.Contains(res.CPP, "while (quark_x > 0) {") {
		t.Fatalf("expected scalar while loop with native comparison, cpp=\n%s", res.CPP)
	}
}

func TestEmit_BreakEmitsBreak(t *testing.T) {
	res := testutil.GenerateCPP("for x in list [1, 2, 3]:\n    break\n")
	if len(res.ParserErrors) > 0 {
		t.Fatalf("unexpected parse errors: %v", res.ParserErrors)
	}
	if len(res.TypeErrors) > 0 {
		t.Fatalf("unexpected type errors: %v", res.TypeErrors)
	}
	if !strings.Contains(res.CPP, "break;") {
		t.Fatalf("expected break statement, cpp=\n%s", res.CPP)
	}
}

func TestEmit_ContinueEmitsContinue(t *testing.T) {
	res := testutil.GenerateCPP("for x in list [1, 2, 3]:\n    continue\n")
	if len(res.ParserErrors) > 0 {
		t.Fatalf("unexpected parse errors: %v", res.ParserErrors)
	}
	if len(res.TypeErrors) > 0 {
		t.Fatalf("unexpected type errors: %v", res.TypeErrors)
	}
	if !strings.Contains(res.CPP, "continue;") {
		t.Fatalf("expected continue statement, cpp=\n%s", res.CPP)
	}
}

func TestEmit_WhenEmitsMatchChain(t *testing.T) {
	res := testutil.GenerateCPP("x = 1\nwhen x:\n    1 -> print('one')\n    2 -> print('two')\n")
	if len(res.ParserErrors) > 0 {
		t.Fatalf("unexpected parse errors: %v", res.ParserErrors)
	}
	if len(res.TypeErrors) > 0 {
		t.Fatalf("unexpected type errors: %v", res.TypeErrors)
	}
	// q_match_eq is a C++ bool, and a runtime error for a vec.
	if !strings.Contains(res.CPP, "if (q_match_eq(") {
		t.Fatalf("expected when to emit q_match_eq for pattern matching, cpp=\n%s", res.CPP)
	}
	if !strings.Contains(res.CPP, "} else if") {
		t.Fatalf("expected when to chain with else if, cpp=\n%s", res.CPP)
	}
}

// --- Closures ---

func TestEmit_ClosureWithCaptures(t *testing.T) {
	src := "x = 10\nf = fn() -> x + 1\nprint(f())\n"
	res := testutil.GenerateCPP(src)
	if len(res.ParserErrors) > 0 {
		t.Fatalf("unexpected parse errors: %v", res.ParserErrors)
	}
	if len(res.TypeErrors) > 0 {
		t.Fatalf("unexpected type errors: %v", res.TypeErrors)
	}
	if !strings.Contains(res.CPP, "q_alloc_closure") {
		t.Fatalf("expected closure allocation for capturing lambda, cpp=\n%s", res.CPP)
	}
	if !strings.Contains(res.CPP, "captures[") {
		t.Fatalf("expected capture array access, cpp=\n%s", res.CPP)
	}
}

func TestEmit_NonCapturingLambdaUsesQvFunc(t *testing.T) {
	src := "f = fn(x) -> x * 2\nprint(f(5))\n"
	res := testutil.GenerateCPP(src)
	if len(res.ParserErrors) > 0 {
		t.Fatalf("unexpected parse errors: %v", res.ParserErrors)
	}
	if len(res.TypeErrors) > 0 {
		t.Fatalf("unexpected type errors: %v", res.TypeErrors)
	}
	if !strings.Contains(res.CPP, "qv_func(") {
		t.Fatalf("expected qv_func for non-capturing lambda, cpp=\n%s", res.CPP)
	}
	// Should NOT allocate a closure since there are no captures
	if strings.Contains(res.CPP, "q_alloc_closure") {
		t.Fatalf("non-capturing lambda should not use q_alloc_closure, cpp=\n%s", res.CPP)
	}
}

func TestEmit_ClosureHiddenFirstParam(t *testing.T) {
	src := "fn add(x, y) -> x + y\n"
	res := testutil.GenerateCPP(src)
	if len(res.ParserErrors) > 0 {
		t.Fatalf("unexpected parse errors: %v", res.ParserErrors)
	}
	if len(res.TypeErrors) > 0 {
		t.Fatalf("unexpected type errors: %v", res.TypeErrors)
	}
	// All functions should take QClosure* _cl as first param
	if !strings.Contains(res.CPP, "QClosure* _cl") {
		t.Fatalf("expected QClosure* _cl as hidden first param, cpp=\n%s", res.CPP)
	}
}

func TestEmit_DirectCallOfDeclaredFunction(t *testing.T) {
	// A `fn` declaration that captures nothing is a plain C++ function:
	// calls are direct, and there is no binding or closure for it.
	src := "fn add(x, y) -> x + y\nprint(add(1, 2))\n"
	res := testutil.GenerateCPP(src)
	if len(res.ParserErrors) > 0 {
		t.Fatalf("unexpected parse errors: %v", res.ParserErrors)
	}
	if len(res.TypeErrors) > 0 {
		t.Fatalf("unexpected type errors: %v", res.TypeErrors)
	}
	if !strings.Contains(res.CPP, "quark_add(nullptr, qv_int(1), qv_int(2))") {
		t.Fatalf("expected a direct call for a declared function, cpp=\n%s", res.CPP)
	}
	if strings.Contains(res.CPP, "QValue quark_add =") || strings.Contains(res.CPP, "QCell* quark_add") {
		t.Fatalf("a declared function that captures nothing needs no binding, cpp=\n%s", res.CPP)
	}
}

func TestEmit_NestedClosureCapturesOuter(t *testing.T) {
	// Nested closures: outer captures x, inner captures x through outer
	// Both lambdas are single-expression (inline with ->)
	src := "x = 1\ng = fn() -> x\nf = fn() -> g()\nprint(f())\n"
	res := testutil.GenerateCPP(src)
	if len(res.ParserErrors) > 0 {
		t.Fatalf("unexpected parse errors: %v", res.ParserErrors)
	}
	if len(res.TypeErrors) > 0 {
		t.Fatalf("unexpected type errors: %v", res.TypeErrors)
	}
	// g captures x, so should use q_alloc_closure
	if !strings.Contains(res.CPP, "q_alloc_closure") {
		t.Fatalf("expected closure allocation for capturing lambda, cpp=\n%s", res.CPP)
	}
}

// --- Expressions ---

func TestEmit_UnaryNegation(t *testing.T) {
	// Dynamic operand: q_neg.
	res := testutil.GenerateCPP("fn neg(a) -> -a\n")
	if len(res.ParserErrors) > 0 {
		t.Fatalf("unexpected parse errors: %v", res.ParserErrors)
	}
	if len(res.TypeErrors) > 0 {
		t.Fatalf("unexpected type errors: %v", res.TypeErrors)
	}
	if !strings.Contains(res.CPP, "q_neg(quark_a)") {
		t.Fatalf("expected q_neg for unary minus on a dynamic value, cpp=\n%s", res.CPP)
	}

	// Int literal: native negation into a long long local.
	res = testutil.GenerateCPP("x = -5\nprint(x)\n")
	if len(res.ParserErrors) > 0 {
		t.Fatalf("unexpected parse errors: %v", res.ParserErrors)
	}
	if len(res.TypeErrors) > 0 {
		t.Fatalf("unexpected type errors: %v", res.TypeErrors)
	}
	if !strings.Contains(res.CPP, "long long quark_x = (-5);") {
		t.Fatalf("expected native negation for an int literal, cpp=\n%s", res.CPP)
	}
}

func TestEmit_AllArithmeticOperators(t *testing.T) {
	// Dynamic operands use runtime helpers; int literal operands are lowered
	// to native C++ arithmetic through overflow-checked helpers.
	tests := []struct {
		src      string
		expected string
	}{
		{"fn f(a, b) -> a + b\n", "q_add(quark_a, quark_b)"},
		{"fn f(a, b) -> a - b\n", "q_sub(quark_a, quark_b)"},
		{"fn f(a, b) -> a * b\n", "q_mul(quark_a, quark_b)"},
		{"fn f(a, b) -> a / b\n", "q_div(quark_a, quark_b)"},
		{"fn f(a, b) -> a % b\n", "q_mod(quark_a, quark_b)"},
		{"fn f(a, b) -> a ** b\n", "q_pow(quark_a, quark_b)"},
		{"x = 1 + 2\n", "q_checked_add(1, 2)"},
		{"x = 1 - 2\n", "q_checked_sub(1, 2)"},
		{"x = 1 * 2\n", "q_checked_mul(1, 2)"},
		{"x = 2 ** 3\n", "q_pow(qv_int(2), qv_int(3))"},
	}
	for _, tt := range tests {
		res := testutil.GenerateCPP(tt.src)
		if len(res.ParserErrors) > 0 || len(res.TypeErrors) > 0 {
			t.Errorf("input %q: errors: parse=%v type=%v", tt.src, res.ParserErrors, res.TypeErrors)
			continue
		}
		if !strings.Contains(res.CPP, tt.expected) {
			t.Errorf("input %q: expected %q in output, cpp=\n%s", tt.src, tt.expected, res.CPP)
		}
	}
}

func TestEmit_AllComparisonOperators(t *testing.T) {
	// Dynamic operands use runtime helpers; int literal operands are lowered
	// to a native comparison stored in a bool local.
	tests := []struct {
		src      string
		expected string
	}{
		{"fn f(a, b) -> a < b\n", "q_lt(quark_a, quark_b)"},
		{"fn f(a, b) -> a <= b\n", "q_lte(quark_a, quark_b)"},
		{"fn f(a, b) -> a > b\n", "q_gt(quark_a, quark_b)"},
		{"fn f(a, b) -> a >= b\n", "q_gte(quark_a, quark_b)"},
		{"fn f(a, b) -> a == b\n", "q_eq(quark_a, quark_b)"},
		{"fn f(a, b) -> a != b\n", "q_neq(quark_a, quark_b)"},
		{"x = 1 < 2\n", "bool quark_x = (1 < 2);"},
		{"x = 1 <= 2\n", "bool quark_x = (1 <= 2);"},
		{"x = 1 > 2\n", "bool quark_x = (1 > 2);"},
		{"x = 1 >= 2\n", "bool quark_x = (1 >= 2);"},
		{"x = 1 == 2\n", "bool quark_x = (1 == 2);"},
		{"x = 1 != 2\n", "bool quark_x = (1 != 2);"},
	}
	for _, tt := range tests {
		res := testutil.GenerateCPP(tt.src)
		if len(res.ParserErrors) > 0 || len(res.TypeErrors) > 0 {
			t.Errorf("input %q: errors: parse=%v type=%v", tt.src, res.ParserErrors, res.TypeErrors)
			continue
		}
		if !strings.Contains(res.CPP, tt.expected) {
			t.Errorf("input %q: expected %q in output, cpp=\n%s", tt.src, tt.expected, res.CPP)
		}
	}
}

func TestEmit_DictMemberAccess(t *testing.T) {
	res := testutil.GenerateCPP("d = dict { name: 'alice' }\nprint(d.name)\n")
	if len(res.ParserErrors) > 0 {
		t.Fatalf("unexpected parse errors: %v", res.ParserErrors)
	}
	if len(res.TypeErrors) > 0 {
		t.Fatalf("unexpected type errors: %v", res.TypeErrors)
	}
	if !strings.Contains(res.CPP, "q_member_get(") {
		t.Fatalf("expected q_member_get for dict member access, cpp=\n%s", res.CPP)
	}
}

func TestEmit_IndexAccess(t *testing.T) {
	res := testutil.GenerateCPP("x = list [1, 2, 3]\nprint(x[0])\n")
	if len(res.ParserErrors) > 0 {
		t.Fatalf("unexpected parse errors: %v", res.ParserErrors)
	}
	if len(res.TypeErrors) > 0 {
		t.Fatalf("unexpected type errors: %v", res.TypeErrors)
	}
	// xs[i] is q_index (out of range is an error); xs.get(i) is q_get.
	if !strings.Contains(res.CPP, "q_index(") {
		t.Fatalf("expected q_index for index access, cpp=\n%s", res.CPP)
	}
}

func TestEmit_IndexAssignment(t *testing.T) {
	res := testutil.GenerateCPP("x = list [1, 2, 3]\nx[0] = 99\n")
	if len(res.ParserErrors) > 0 {
		t.Fatalf("unexpected parse errors: %v", res.ParserErrors)
	}
	if len(res.TypeErrors) > 0 {
		t.Fatalf("unexpected type errors: %v", res.TypeErrors)
	}
	if !strings.Contains(res.CPP, "q_set(") {
		t.Fatalf("expected q_set for index assignment, cpp=\n%s", res.CPP)
	}
}

func TestEmit_DictEmitsQvDict(t *testing.T) {
	res := testutil.GenerateCPP("d = dict { a: 1, b: 2 }\n")
	if len(res.ParserErrors) > 0 {
		t.Fatalf("unexpected parse errors: %v", res.ParserErrors)
	}
	if len(res.TypeErrors) > 0 {
		t.Fatalf("unexpected type errors: %v", res.TypeErrors)
	}
	if !strings.Contains(res.CPP, "qv_dict(") {
		t.Fatalf("expected qv_dict(...) for dict literal, cpp=\n%s", res.CPP)
	}
	if !strings.Contains(res.CPP, "q_dict_set(") {
		t.Fatalf("expected q_dict_set for dict entries, cpp=\n%s", res.CPP)
	}
}

func TestEmit_LiteralsEmitCorrectConstructors(t *testing.T) {
	// Uncaptured scalar literals use native locals; strings, null and
	// captured scalars use QValue constructors.
	tests := []struct {
		src      string
		expected string
	}{
		{"x = 42\n", "long long quark_x = 42;"},
		{"x = 3.14\n", "double quark_x = 3.14;"},
		{"x = true\n", "bool quark_x = true;"},
		{"x = false\n", "bool quark_x = false;"},
		{"x = 'hello'\n", "qv_string(\"hello\")"},
		{"x = null\n", "qv_null()"},
		{"x = 42\nf = fn() -> x\n", "qv_int(42)"},
		{"x = 3.14\nf = fn() -> x\n", "qv_float(3.14)"},
		{"x = true\nf = fn() -> x\n", "qv_bool(true)"},
	}
	for _, tt := range tests {
		res := testutil.GenerateCPP(tt.src)
		if len(res.ParserErrors) > 0 || len(res.TypeErrors) > 0 {
			t.Errorf("input %q: errors: parse=%v type=%v", tt.src, res.ParserErrors, res.TypeErrors)
			continue
		}
		if !strings.Contains(res.CPP, tt.expected) {
			t.Errorf("input %q: expected %q in output, cpp=\n%s", tt.src, tt.expected, res.CPP)
		}
	}
}

// --- Variable handling ---

func TestEmit_VariableDeclUsesQCell(t *testing.T) {
	// Only captured variables are stored in a QCell.
	res := testutil.GenerateCPP("x = 42\nf = fn() -> x\nprint(f())\n")
	if len(res.ParserErrors) > 0 {
		t.Fatalf("unexpected parse errors: %v", res.ParserErrors)
	}
	if len(res.TypeErrors) > 0 {
		t.Fatalf("unexpected type errors: %v", res.TypeErrors)
	}
	if !strings.Contains(res.CPP, "QCell* quark_x") {
		t.Fatalf("expected QCell* quark_x declaration, cpp=\n%s", res.CPP)
	}
	if !strings.Contains(res.CPP, "quark_x->value") {
		t.Fatalf("expected quark_x->value access, cpp=\n%s", res.CPP)
	}

	res = testutil.GenerateCPP("x = 42\nprint(x)\n")
	if len(res.ParserErrors) > 0 {
		t.Fatalf("unexpected parse errors: %v", res.ParserErrors)
	}
	if len(res.TypeErrors) > 0 {
		t.Fatalf("unexpected type errors: %v", res.TypeErrors)
	}
	if strings.Contains(res.CPP, "QCell* quark_x") {
		t.Fatalf("expected uncaptured variable not to use a QCell, cpp=\n%s", res.CPP)
	}
}

func TestEmit_VariableReassignment(t *testing.T) {
	// Captured: declare the QCell once, then reassign via ->value.
	res := testutil.GenerateCPP("x = 1\nx = 2\nf = fn() -> x\nprint(f())\n")
	if len(res.ParserErrors) > 0 {
		t.Fatalf("unexpected parse errors: %v", res.ParserErrors)
	}
	if len(res.TypeErrors) > 0 {
		t.Fatalf("unexpected type errors: %v", res.TypeErrors)
	}
	cpp := res.CPP
	declCount := strings.Count(cpp, "QCell* quark_x = q_new_cell(")
	if declCount != 1 {
		t.Fatalf("expected exactly 1 QCell declaration for x, got %d, cpp=\n%s", declCount, cpp)
	}
	assignCount := strings.Count(cpp, "quark_x->value =")
	if assignCount < 2 {
		t.Fatalf("expected at least 2 assignments to quark_x->value, got %d, cpp=\n%s", assignCount, cpp)
	}

	// Uncaptured int: declare the native local once, then assign directly.
	res = testutil.GenerateCPP("x = 1\nx = 2\nprint(x)\n")
	if len(res.ParserErrors) > 0 {
		t.Fatalf("unexpected parse errors: %v", res.ParserErrors)
	}
	if len(res.TypeErrors) > 0 {
		t.Fatalf("unexpected type errors: %v", res.TypeErrors)
	}
	cpp = res.CPP
	if strings.Count(cpp, "long long quark_x") != 1 || !strings.Contains(cpp, "quark_x = 2;") {
		t.Fatalf("expected one long long declaration and a direct reassignment, cpp=\n%s", cpp)
	}
}

func TestEmit_TypedVarDecl(t *testing.T) {
	res := testutil.GenerateCPP("x: int = 42\nprint(x)\n")
	if len(res.ParserErrors) > 0 {
		t.Fatalf("unexpected parse errors: %v", res.ParserErrors)
	}
	if len(res.TypeErrors) > 0 {
		t.Fatalf("unexpected type errors: %v", res.TypeErrors)
	}
	if !strings.Contains(res.CPP, "long long quark_x = 42;") {
		t.Fatalf("expected typed int var decl to emit a long long local, cpp=\n%s", res.CPP)
	}
}

// --- Pipes ---

func TestEmit_PipeIntoFreeFunction(t *testing.T) {
	// Pipe into a free function (len is a free function, not a method)
	res := testutil.GenerateCPP("x = list [1, 2, 3] | len()\nprint(x)\n")
	if len(res.ParserErrors) > 0 {
		t.Fatalf("unexpected parse errors: %v", res.ParserErrors)
	}
	if len(res.TypeErrors) > 0 {
		t.Fatalf("unexpected type errors: %v", res.TypeErrors)
	}
	if !strings.Contains(res.CPP, "q_len(") {
		t.Fatalf("expected pipe to emit q_len, cpp=\n%s", res.CPP)
	}
}

func TestEmit_PipeIntoUserFunction(t *testing.T) {
	// Pipe into a user-defined function
	src := "fn double(x) -> x + x\nx = 5 | double()\nprint(x)\n"
	res := testutil.GenerateCPP(src)
	if len(res.ParserErrors) > 0 {
		t.Fatalf("unexpected parse errors: %v", res.ParserErrors)
	}
	if len(res.TypeErrors) > 0 {
		t.Fatalf("unexpected type errors: %v", res.TypeErrors)
	}
	// A declared function that captures nothing is called directly.
	if !strings.Contains(res.CPP, "quark_double(nullptr, qv_int(5))") {
		t.Fatalf("expected pipe into a declared fn to call it directly, cpp=\n%s", res.CPP)
	}
}

func TestEmit_DotCallMethodChain(t *testing.T) {
	// Method chaining via dot-call (the correct syntax for methods)
	res := testutil.GenerateCPP("s = 'hello world'\nx = s.split(' ')\nprint(x)\n")
	if len(res.ParserErrors) > 0 {
		t.Fatalf("unexpected parse errors: %v", res.ParserErrors)
	}
	if len(res.TypeErrors) > 0 {
		t.Fatalf("unexpected type errors: %v", res.TypeErrors)
	}
	if !strings.Contains(res.CPP, "q_split(") {
		t.Fatalf("expected dot-call method to emit q_split, cpp=\n%s", res.CPP)
	}
}

// --- Source location tracking ---

func TestEmit_EmitsSourceLocCalls(t *testing.T) {
	res := testutil.GenerateCPP("print('hello')\n")
	if len(res.ParserErrors) > 0 {
		t.Fatalf("unexpected parse errors: %v", res.ParserErrors)
	}
	if !strings.Contains(res.CPP, "q_set_source_loc(") {
		t.Fatalf("expected source location tracking calls, cpp=\n%s", res.CPP)
	}
}

// --- GC initialization ---

func TestEmit_EmitsGCInit(t *testing.T) {
	res := testutil.GenerateCPP("print('hello')\n")
	if len(res.ParserErrors) > 0 {
		t.Fatalf("unexpected parse errors: %v", res.ParserErrors)
	}
	if !strings.Contains(res.CPP, "q_gc_init()") {
		t.Fatalf("expected q_gc_init() in main, cpp=\n%s", res.CPP)
	}
}

// --- Runtime include ---

func TestEmit_IncludesQuarkHeader(t *testing.T) {
	res := testutil.GenerateCPP("print('hello')\n")
	if !strings.Contains(res.CPP, "#include \"quark/quark.hpp\"") {
		t.Fatalf("expected quark header include, cpp=\n%s", res.CPP)
	}
}

// --- Forward declarations ---

func TestEmit_ForwardDeclaresLambdas(t *testing.T) {
	// Anonymous lambdas get generated quark___lambdaN names and forward declarations.
	src := "add = fn(x, y) -> x + y\nprint(add(1, 2))\n"
	res := testutil.GenerateCPP(src)
	if len(res.ParserErrors) > 0 {
		t.Fatalf("unexpected parse errors: %v", res.ParserErrors)
	}
	if len(res.TypeErrors) > 0 {
		t.Fatalf("unexpected type errors: %v", res.TypeErrors)
	}
	// Forward declarations should appear before main
	declIdx := strings.Index(res.CPP, "QValue quark___lambda1(QClosure*")
	mainIdx := strings.Index(res.CPP, "int main(int argc, char** argv)")
	if declIdx == -1 {
		t.Fatalf("expected forward declaration for lambda, cpp=\n%s", res.CPP)
	}
	if mainIdx == -1 {
		t.Fatalf("expected main function, cpp=\n%s", res.CPP)
	}
	if declIdx > mainIdx {
		t.Fatalf("forward declaration should appear before main, cpp=\n%s", res.CPP)
	}
}

// --- Method receiver injection ---

func TestEmit_MethodReceiverInjectedAsFirstArg(t *testing.T) {
	res := testutil.GenerateCPP("s = 'hello world'\nparts = s.split(' ')\nprint(parts)\n")
	if len(res.ParserErrors) > 0 {
		t.Fatalf("unexpected parse errors: %v", res.ParserErrors)
	}
	if len(res.TypeErrors) > 0 {
		t.Fatalf("unexpected type errors: %v", res.TypeErrors)
	}
	// Should cache receiver in a temp, then call q_split(temp, arg)
	if !strings.Contains(res.CPP, "q_split(") {
		t.Fatalf("expected q_split call, cpp=\n%s", res.CPP)
	}
}

// --- String escaping ---

func TestEmit_StringEscaping(t *testing.T) {
	res := testutil.GenerateCPP("x = 'line1\\nline2'\nprint(x)\n")
	if len(res.ParserErrors) > 0 {
		t.Fatalf("unexpected parse errors: %v", res.ParserErrors)
	}
	// The \n should be preserved as \\n in the C++ string literal
	if !strings.Contains(res.CPP, "qv_string(") {
		t.Fatalf("expected qv_string call, cpp=\n%s", res.CPP)
	}
}

// TestEmit_MissingCallPlanIsAnInternalError: lowering reports a call the
// analyzer gave no plan as an internal error instead of emitting C++.
func TestEmit_MissingCallPlanIsAnInternalError(t *testing.T) {
	analyzer, node, parseErrs, typeErrs := testutil.Analyze("print(len('x'))\n")
	if len(parseErrs) > 0 || len(typeErrs) > 0 {
		t.Fatalf("unexpected errors: %v %v", parseErrs, typeErrs)
	}
	an := *analyzer.Analysis
	an.CallPlans = map[*ast.TreeNode]*ir.CallPlan{}
	_, err := lower.Lower(node, &an, "test_input.qrk")
	if err == nil || !strings.Contains(err.Error(), "INV-CALLPLAN-MISSING") {
		t.Fatalf("expected INV-CALLPLAN-MISSING, got %v", err)
	}
}

// TestEmit_FunctionValueCallsByArity: calls through a function value use
// q_callN for up to 12 arguments and q_calln with a vector beyond.
func TestEmit_FunctionValueCallsByArity(t *testing.T) {
	res := testutil.GenerateCPP("f0 = fn() -> 0\nf1 = fn(a) -> a\nf3 = fn(a, b, c) -> a\n" +
		"f13 = fn(a, b, c, d, e, f, g, h, i, j, k, l, m) -> a\n" +
		"print(f0())\nprint(f1(1))\nprint(f3(1, 2, 3))\nprint(f13(1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13))\n")
	if len(res.ParserErrors) > 0 || len(res.TypeErrors) > 0 {
		t.Fatalf("unexpected errors: %v %v", res.ParserErrors, res.TypeErrors)
	}
	for _, want := range []string{"q_call0(quark_f0)", "q_call1(quark_f1, qv_int(1))", "q_call3(quark_f3, qv_int(1), qv_int(2), qv_int(3))", "q_calln(quark_f13, std::vector<QValue>{qv_int(1), "} {
		if !strings.Contains(res.CPP, want) {
			t.Fatalf("expected %q in:\n%s", want, res.CPP)
		}
	}
}

func TestEmit_IntFloatCompareAndStackGuard(t *testing.T) {
	res := testutil.GenerateCPP("n: int = 9007199254740993\nprint(n == 9007199254740992.0)\nprint(9007199254740992.0 < n)\nprint(n == 1)\n")
	if len(res.ParserErrors) > 0 || len(res.TypeErrors) > 0 {
		t.Fatalf("unexpected errors: %v %v", res.ParserErrors, res.TypeErrors)
	}
	for _, want := range []string{"q_int_float_eq(", "q_int_float_gt(", "q_protect_and_run(quark__entry)"} {
		if !strings.Contains(res.CPP, want) {
			t.Fatalf("expected %q in:\n%s", want, res.CPP)
		}
	}
	// Two ints stay a raw compare.
	if strings.Contains(res.CPP, "q_int_float_eq(quark_n, 1)") {
		t.Fatalf("an int compared with an int should stay raw:\n%s", res.CPP)
	}
}
