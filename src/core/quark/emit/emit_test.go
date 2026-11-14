package emit_test

import (
	"regexp"
	"strings"
	"testing"

	"quark/internal/testutil"
)

func cpp(t *testing.T, src string) string {
	t.Helper()
	res := testutil.GenerateCPP(src)
	if len(res.ParserErrors) > 0 || len(res.TypeErrors) > 0 {
		t.Fatalf("unexpected errors: %v %v", res.ParserErrors, res.TypeErrors)
	}
	return res.CPP
}

func mustContain(t *testing.T, out string, want ...string) {
	t.Helper()
	for _, w := range want {
		if !strings.Contains(out, w) {
			t.Fatalf("expected %q in:\n%s", w, out)
		}
	}
}

// mustMatch checks out against a regular expression in which `_tN` matches
// any temporary.
func mustMatch(t *testing.T, out, pattern string) {
	t.Helper()
	re := regexp.QuoteMeta(pattern)
	re = strings.ReplaceAll(re, "_tN", `_t\d+`)
	if !regexp.MustCompile(re).MatchString(out) {
		t.Fatalf("expected %q in:\n%s", pattern, out)
	}
}

func mustNotContain(t *testing.T, out string, bad ...string) {
	t.Helper()
	for _, b := range bad {
		if strings.Contains(out, b) {
			t.Fatalf("unexpected %q in:\n%s", b, out)
		}
	}
}

func TestEmit_BoolLogicIsRawShortCircuit(t *testing.T) {
	out := cpp(t, "a = 1\nb = 2\nif a < b and b < 3:\n    print('y')\n")
	mustContain(t, out, "((quark_a < quark_b) && (quark_b < 3))")
	mustNotContain(t, out, "[&]")
}

func TestEmit_ValueLogicRunsRightOperandInBranch(t *testing.T) {
	out := cpp(t, "fn f() -> 'x'\nfn g() -> 'y'\ns = 'a' or f().contains(g())\nprint(s)\n")
	// The receiver f() is computed inside the branch that needs it, before g().
	mustMatch(t, out, "if ((!q_truthy(_tN))) {\n        QValue _tN = quark_f(nullptr);\n        _tN = q_contains(_tN, quark_g(nullptr));")
}

func TestEmit_WhileConditionWithStatementsIsTestedEachIteration(t *testing.T) {
	out := cpp(t, "fn a() -> 'a'\nwhile a().contains(a()):\n    break\n")
	mustMatch(t, out, "while (true) {\n        QValue _tN = quark_a(nullptr);\n        if (!(q_truthy(q_contains(_tN, quark_a(nullptr))))) break;")
}

func TestEmit_PlainWhileCondition(t *testing.T) {
	out := cpp(t, "n = 0\nwhile n < 3:\n    n = n + 1\n")
	mustContain(t, out, "while (quark_n < 3) {", "quark_n = q_checked_add(quark_n, 1);")
}

func TestEmit_ElseifConditionInsideElse(t *testing.T) {
	out := cpp(t, "fn name() -> 'b'\nif name() == 'a':\n    print(1)\nelseif name().contains(name()):\n    print(2)\n")
	// The elseif's receiver temporary lives in the else branch.
	mustMatch(t, out, "} else {\n        QValue _tN = quark_name(nullptr);\n        if (q_truthy(q_contains(_tN, quark_name(nullptr)))) {")
}

func TestEmit_TernaryOfScalarsIsSelect(t *testing.T) {
	out := cpp(t, "c = true\nx = 1 if c else 2\nprint(x)\n")
	mustContain(t, out, "long long quark_x = (quark_c ? 1 : 2);")
}

func TestEmit_BangIsRawNot(t *testing.T) {
	out := cpp(t, "x = !true\nprint(x)\n")
	mustContain(t, out, "bool quark_x = (!true);")
}

func TestEmit_RangeLoop(t *testing.T) {
	out := cpp(t, "for i in range(1, 10):\n    print(i)\n")
	mustContain(t, out, "q_range_count(", "long long quark_i = q_range_at(", " ? 1LL : -1LL")
	mustNotContain(t, out, "q_range(")
}

func TestEmit_VectorLoopFastPath(t *testing.T) {
	out := cpp(t, "v = vec [1.5, 2.5]\ns = 0.0\nfor x in v:\n    s = s + x\nprint(s)\n")
	mustContain(t, out, "q_vec_is_dense_of(", "QVector::Type::F64", "q_vec_f64_data(")
}

func TestEmit_ClosureCapturesCell(t *testing.T) {
	out := cpp(t, "n = 0\ng = fn() -> n + 1\nprint(g())\n")
	mustContain(t, out, "q_alloc_closure((void*)quark___lambda1, &_qfi___lambda1, 1);", "->captures[0] = quark_n;", "QCell* quark_n = _cl->captures[0];")
}

func TestEmit_NativeFunction(t *testing.T) {
	out := cpp(t, "fn add(x: int, y: int) int -> x + y\nprint(add(1, 2))\n")
	mustContain(t, out, "int64_t quark_add(QClosure* _cl, int64_t _arg_quark_x, int64_t _arg_quark_y) {",
		"return q_checked_add(quark_x, quark_y);", "qv_int(quark_add(nullptr, 1, 2))", "QValue quark_add__thunk(")
}

func TestEmit_DynamicMethodTagChain(t *testing.T) {
	out := cpp(t, "fn id(v) -> v\nprint(id('abc').upper())\n")
	mustMatch(t, out, "if (_tN.type == QValue::VAL_STRING) {")
	mustContain(t, out, "q_no_method(")
	mustNotContain(t, out, "switch (")
}

func TestEmit_ConditionComparisonHasNoExtraParentheses(t *testing.T) {
	out := cpp(t, "x = 1\nif x == 1:\n    print(x)\n")
	mustContain(t, out, "if (quark_x == 1) {")
}
