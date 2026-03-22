package types_test

import (
	"strings"
	"testing"

	"quark/internal/testutil"
)

// This file tests error paths — programs that MUST be rejected by the analyzer.
// Each test verifies that invalid code produces the correct error.

// --- Scope errors ---

func TestNeg_UndefinedVariable(t *testing.T) {
	_, _, parseErrs, typeErrs := testutil.Analyze("println(x)\n")
	if len(parseErrs) > 0 {
		t.Fatalf("unexpected parse errors: %v", parseErrs)
	}
	if len(typeErrs) == 0 {
		t.Fatalf("expected error for undefined variable 'x'")
	}
	joined := strings.Join(typeErrs, "\n")
	if !strings.Contains(joined, "undefined") {
		t.Fatalf("expected 'undefined' error, got: %v", typeErrs)
	}
}

func TestNeg_BreakOutsideLoop(t *testing.T) {
	_, _, parseErrs, typeErrs := testutil.Analyze("break\n")
	if len(parseErrs) > 0 {
		t.Fatalf("unexpected parse errors: %v", parseErrs)
	}
	if len(typeErrs) == 0 {
		t.Fatalf("expected error for break outside loop")
	}
	joined := strings.Join(typeErrs, "\n")
	if !strings.Contains(joined, "break") && !strings.Contains(joined, "loop") {
		t.Fatalf("expected break-outside-loop error, got: %v", typeErrs)
	}
}

func TestNeg_ContinueOutsideLoop(t *testing.T) {
	_, _, parseErrs, typeErrs := testutil.Analyze("continue\n")
	if len(parseErrs) > 0 {
		t.Fatalf("unexpected parse errors: %v", parseErrs)
	}
	if len(typeErrs) == 0 {
		t.Fatalf("expected error for continue outside loop")
	}
	joined := strings.Join(typeErrs, "\n")
	if !strings.Contains(joined, "continue") && !strings.Contains(joined, "loop") {
		t.Fatalf("expected continue-outside-loop error, got: %v", typeErrs)
	}
}

// --- Type mismatch errors ---

func TestNeg_ArithmeticOnStrings(t *testing.T) {
	_, _, parseErrs, typeErrs := testutil.Analyze("println('a' - 'b')\n")
	if len(parseErrs) > 0 {
		t.Fatalf("unexpected parse errors: %v", parseErrs)
	}
	if len(typeErrs) == 0 {
		t.Fatalf("expected type error for string subtraction")
	}
	joined := strings.Join(typeErrs, "\n")
	if !strings.Contains(joined, "numeric") {
		t.Fatalf("expected numeric operands error, got: %v", typeErrs)
	}
}

func TestNeg_ArithmeticOnBooleans(t *testing.T) {
	_, _, parseErrs, typeErrs := testutil.Analyze("println(true * false)\n")
	if len(parseErrs) > 0 {
		t.Fatalf("unexpected parse errors: %v", parseErrs)
	}
	if len(typeErrs) == 0 {
		t.Fatalf("expected type error for boolean multiplication")
	}
}

func TestNeg_CompareStringToInt(t *testing.T) {
	// Quark is dynamically typed — cross-type comparisons are allowed at compile time
	// and checked at runtime. Verify this doesn't produce a type error.
	_, _, parseErrs, typeErrs := testutil.Analyze("println('a' < 1)\n")
	if len(parseErrs) > 0 {
		t.Fatalf("unexpected parse errors: %v", parseErrs)
	}
	if len(typeErrs) > 0 {
		t.Fatalf("unexpected type errors for cross-type comparison: %v", typeErrs)
	}
}

// --- Arity errors ---

func TestNeg_TooFewArgsToBuiltin(t *testing.T) {
	_, _, parseErrs, typeErrs := testutil.Analyze("len()\n")
	if len(parseErrs) > 0 {
		t.Fatalf("unexpected parse errors: %v", parseErrs)
	}
	if len(typeErrs) == 0 {
		t.Fatalf("expected arity error for len()")
	}
	joined := strings.Join(typeErrs, "\n")
	if !strings.Contains(joined, "expects") {
		t.Fatalf("expected arity error message, got: %v", typeErrs)
	}
}

func TestNeg_TooManyArgsToUserFunc(t *testing.T) {
	_, _, parseErrs, typeErrs := testutil.Analyze("fn f(x) -> x\nf(1, 2, 3)\n")
	if len(parseErrs) > 0 {
		t.Fatalf("unexpected parse errors: %v", parseErrs)
	}
	if len(typeErrs) == 0 {
		t.Fatalf("expected arity error for f(1, 2, 3)")
	}
	joined := strings.Join(typeErrs, "\n")
	if !strings.Contains(joined, "expects") && !strings.Contains(joined, "arguments") {
		t.Fatalf("expected arity error, got: %v", typeErrs)
	}
}

func TestNeg_TooFewArgsToUserFunc(t *testing.T) {
	_, _, parseErrs, typeErrs := testutil.Analyze("fn f(x, y) -> x + y\nf(1)\n")
	if len(parseErrs) > 0 {
		t.Fatalf("unexpected parse errors: %v", parseErrs)
	}
	if len(typeErrs) == 0 {
		t.Fatalf("expected arity error for f(1) when f expects 2 args")
	}
}

func TestNeg_MethodArityError(t *testing.T) {
	_, _, parseErrs, typeErrs := testutil.Analyze("'hello'.split()\n")
	if len(parseErrs) > 0 {
		t.Fatalf("unexpected parse errors: %v", parseErrs)
	}
	if len(typeErrs) == 0 {
		t.Fatalf("expected arity error for split() with 0 args (needs 1)")
	}
	joined := strings.Join(typeErrs, "\n")
	if !strings.Contains(joined, "method 'split'") {
		t.Fatalf("expected method arity error, got: %v", typeErrs)
	}
}

// --- Method dispatch errors ---

func TestNeg_MethodOnInt(t *testing.T) {
	_, _, parseErrs, typeErrs := testutil.Analyze("x = 42\nx.upper()\n")
	if len(parseErrs) > 0 {
		t.Fatalf("unexpected parse errors: %v", parseErrs)
	}
	if len(typeErrs) == 0 {
		t.Fatalf("expected error for calling .upper() on int")
	}
	joined := strings.Join(typeErrs, "\n")
	if !strings.Contains(joined, "has no method 'upper'") {
		t.Fatalf("expected 'has no method' error, got: %v", typeErrs)
	}
}

func TestNeg_MethodOnBool(t *testing.T) {
	_, _, parseErrs, typeErrs := testutil.Analyze("x = true\nx.push(1)\n")
	if len(parseErrs) > 0 {
		t.Fatalf("unexpected parse errors: %v", parseErrs)
	}
	if len(typeErrs) == 0 {
		t.Fatalf("expected error for calling .push() on bool")
	}
	joined := strings.Join(typeErrs, "\n")
	if !strings.Contains(joined, "has no method") {
		t.Fatalf("expected 'has no method' error, got: %v", typeErrs)
	}
}

func TestNeg_UnknownMethodOnStr(t *testing.T) {
	_, _, parseErrs, typeErrs := testutil.Analyze("'hello'.foobar()\n")
	if len(parseErrs) > 0 {
		t.Fatalf("unexpected parse errors: %v", parseErrs)
	}
	if len(typeErrs) == 0 {
		t.Fatalf("expected error for unknown method on str")
	}
	joined := strings.Join(typeErrs, "\n")
	if !strings.Contains(joined, "has no method 'foobar'") {
		t.Fatalf("expected 'has no method' error, got: %v", typeErrs)
	}
}

func TestNeg_UnknownMethodOnList(t *testing.T) {
	_, _, parseErrs, typeErrs := testutil.Analyze("xs = list [1, 2]\nxs.sort()\n")
	if len(parseErrs) > 0 {
		t.Fatalf("unexpected parse errors: %v", parseErrs)
	}
	if len(typeErrs) == 0 {
		t.Fatalf("expected error for unknown method 'sort' on list")
	}
	joined := strings.Join(typeErrs, "\n")
	if !strings.Contains(joined, "has no method 'sort'") {
		t.Fatalf("expected 'has no method' error, got: %v", typeErrs)
	}
}

// --- Method argument type errors ---

func TestNeg_MethodWrongArgType(t *testing.T) {
	_, _, parseErrs, typeErrs := testutil.Analyze("'hello'.contains(42)\n")
	if len(parseErrs) > 0 {
		t.Fatalf("unexpected parse errors: %v", parseErrs)
	}
	if len(typeErrs) == 0 {
		t.Fatalf("expected type error for contains(int) — needs str")
	}
	joined := strings.Join(typeErrs, "\n")
	if !strings.Contains(joined, "expects str, got int") {
		t.Fatalf("expected argument type error, got: %v", typeErrs)
	}
}

func TestNeg_ListGetWrongArgType(t *testing.T) {
	_, _, parseErrs, typeErrs := testutil.Analyze("xs = list [1, 2]\nxs.get('a')\n")
	if len(parseErrs) > 0 {
		t.Fatalf("unexpected parse errors: %v", parseErrs)
	}
	if len(typeErrs) == 0 {
		t.Fatalf("expected type error for list.get(str) — needs int")
	}
	joined := strings.Join(typeErrs, "\n")
	if !strings.Contains(joined, "expects int, got str") {
		t.Fatalf("expected argument type error, got: %v", typeErrs)
	}
}

// --- Callable checks ---

func TestNeg_IntNotCallable(t *testing.T) {
	_, _, parseErrs, typeErrs := testutil.Analyze("x = 42\nx()\n")
	if len(parseErrs) > 0 {
		t.Fatalf("unexpected parse errors: %v", parseErrs)
	}
	if len(typeErrs) == 0 {
		t.Fatalf("expected error for calling int as function")
	}
	joined := strings.Join(typeErrs, "\n")
	if !strings.Contains(joined, "not callable") {
		t.Fatalf("expected 'not callable' error, got: %v", typeErrs)
	}
}

func TestNeg_StringNotCallable(t *testing.T) {
	_, _, parseErrs, typeErrs := testutil.Analyze("x = 'hello'\nx()\n")
	if len(parseErrs) > 0 {
		t.Fatalf("unexpected parse errors: %v", parseErrs)
	}
	if len(typeErrs) == 0 {
		t.Fatalf("expected error for calling string as function")
	}
	joined := strings.Join(typeErrs, "\n")
	if !strings.Contains(joined, "not callable") {
		t.Fatalf("expected 'not callable' error, got: %v", typeErrs)
	}
}

// --- Type annotation mismatches ---

func TestNeg_TypeAnnotationMismatch_IntToStr(t *testing.T) {
	_, _, parseErrs, typeErrs := testutil.Analyze("x: str = 42\n")
	if len(parseErrs) > 0 {
		t.Fatalf("unexpected parse errors: %v", parseErrs)
	}
	if len(typeErrs) == 0 {
		t.Fatalf("expected type error for assigning int to str-annotated var")
	}
	joined := strings.Join(typeErrs, "\n")
	if !strings.Contains(joined, "cannot assign") {
		t.Fatalf("expected assignment type mismatch error, got: %v", typeErrs)
	}
}

func TestNeg_ResultAnnotation_RejectsNonResult(t *testing.T) {
	_, _, parseErrs, typeErrs := testutil.Analyze("r: result = 1\n")
	if len(parseErrs) > 0 {
		t.Fatalf("unexpected parse errors: %v", parseErrs)
	}
	if len(typeErrs) == 0 {
		t.Fatalf("expected type error for assigning int to result-annotated var")
	}
}

// --- ErrorType propagation (cascade suppression) ---

func TestNeg_ErrorTypeSuppressesCascade(t *testing.T) {
	// x is undefined → TypeError. y = x.upper() should NOT add a second error.
	_, _, parseErrs, typeErrs := testutil.Analyze("y = x.upper()\nz = y.trim()\n")
	if len(parseErrs) > 0 {
		t.Fatalf("unexpected parse errors: %v", parseErrs)
	}
	if len(typeErrs) != 1 {
		t.Fatalf("expected exactly 1 error (undefined 'x'), got %d: %v", len(typeErrs), typeErrs)
	}
	joined := strings.Join(typeErrs, "\n")
	if !strings.Contains(joined, "undefined") {
		t.Fatalf("expected 'undefined' error, got: %v", typeErrs)
	}
}

func TestNeg_ErrorTypeInBinaryOp(t *testing.T) {
	// undefined + 1 should produce 1 error, not cascading
	_, _, parseErrs, typeErrs := testutil.Analyze("y = x + 1\nz = y * 2\n")
	if len(parseErrs) > 0 {
		t.Fatalf("unexpected parse errors: %v", parseErrs)
	}
	if len(typeErrs) != 1 {
		t.Fatalf("expected exactly 1 error (undefined 'x'), got %d: %v", len(typeErrs), typeErrs)
	}
}

// --- unwrap on non-result ---

func TestNeg_UnwrapNonResult(t *testing.T) {
	_, _, parseErrs, typeErrs := testutil.Analyze("println(unwrap(42))\n")
	if len(parseErrs) > 0 {
		t.Fatalf("unexpected parse errors: %v", parseErrs)
	}
	if len(typeErrs) == 0 {
		t.Fatalf("expected type error for unwrap(int)")
	}
	joined := strings.Join(typeErrs, "\n")
	if !strings.Contains(joined, "unwrap") && !strings.Contains(joined, "result") {
		t.Fatalf("expected unwrap-non-result error, got: %v", typeErrs)
	}
}

// --- when pattern on non-result ---

func TestNeg_WhenResultPatternOnNonResult(t *testing.T) {
	_, _, parseErrs, typeErrs := testutil.Analyze("when 42:\n    ok v -> v\n    _ -> 0\n")
	if len(parseErrs) > 0 {
		t.Fatalf("unexpected parse errors: %v", parseErrs)
	}
	if len(typeErrs) == 0 {
		t.Fatalf("expected type error for result pattern on int")
	}
	joined := strings.Join(typeErrs, "\n")
	if !strings.Contains(joined, "result") {
		t.Fatalf("expected result pattern error, got: %v", typeErrs)
	}
}

// --- Module errors ---

func TestNeg_ModuleUnknownSymbol(t *testing.T) {
	_, _, parseErrs, typeErrs := testutil.Analyze("module math:\n    fn add(x, y) -> x + y\nuse math as m\nm.nonexistent()\n")
	if len(parseErrs) > 0 {
		t.Fatalf("unexpected parse errors: %v", parseErrs)
	}
	if len(typeErrs) == 0 {
		t.Fatalf("expected error for unknown module symbol")
	}
	joined := strings.Join(typeErrs, "\n")
	if !strings.Contains(joined, "no symbol") {
		t.Fatalf("expected 'no symbol' error, got: %v", typeErrs)
	}
}

// --- Dict duplicate keys ---

func TestNeg_DictDuplicateKeys(t *testing.T) {
	_, _, parseErrs, typeErrs := testutil.Analyze("d = dict { a: 1, a: 2 }\n")
	if len(parseErrs) > 0 {
		t.Fatalf("unexpected parse errors: %v", parseErrs)
	}
	joined := strings.Join(typeErrs, "\n")
	if !strings.Contains(joined, "duplicate") {
		t.Fatalf("expected duplicate key error, got: %v", typeErrs)
	}
}

// --- Vector type errors ---

func TestNeg_VectorMixedTypes(t *testing.T) {
	_, _, parseErrs, typeErrs := testutil.Analyze("vector [1, 'a', 3]\n")
	if len(parseErrs) > 0 {
		t.Fatalf("unexpected parse errors: %v", parseErrs)
	}
	if len(typeErrs) == 0 {
		t.Fatalf("expected error for mixed-type vector")
	}
	joined := strings.Join(typeErrs, "\n")
	if !strings.Contains(joined, "homogeneous") {
		t.Fatalf("expected homogeneous error, got: %v", typeErrs)
	}
}

func TestNeg_VectorStringArithmetic(t *testing.T) {
	_, _, parseErrs, typeErrs := testutil.Analyze("v = vector ['a', 'b']\nw = vector ['c', 'd']\nprintln(v + w)\n")
	if len(parseErrs) > 0 {
		t.Fatalf("unexpected parse errors: %v", parseErrs)
	}
	if len(typeErrs) == 0 {
		t.Fatalf("expected error for string vector arithmetic")
	}
}

// --- For loop on non-iterable ---

func TestNeg_ForLoopOnInt(t *testing.T) {
	_, _, parseErrs, typeErrs := testutil.Analyze("for x in 42:\n    println(x)\n")
	if len(parseErrs) > 0 {
		t.Fatalf("unexpected parse errors: %v", parseErrs)
	}
	if len(typeErrs) == 0 {
		t.Fatalf("expected error for iterating over int")
	}
	joined := strings.Join(typeErrs, "\n")
	if !strings.Contains(joined, "iterable") {
		t.Fatalf("expected iterable error, got: %v", typeErrs)
	}
}

// --- Pipe errors ---

func TestNeg_PipeMethodWrongArgType(t *testing.T) {
	_, _, parseErrs, typeErrs := testutil.Analyze("x = 1 | 'abc'.concat()\n")
	if len(parseErrs) > 0 {
		t.Fatalf("unexpected parse errors: %v", parseErrs)
	}
	if len(typeErrs) == 0 {
		t.Fatalf("expected type error for piping int into str.concat")
	}
	joined := strings.Join(typeErrs, "\n")
	if !strings.Contains(joined, "expects str, got int") {
		t.Fatalf("expected concat arg type error, got: %v", typeErrs)
	}
}

// --- Return type annotation errors ---

func TestNeg_ReturnTypeMismatch(t *testing.T) {
	_, _, parseErrs, typeErrs := testutil.Analyze("fn bad() int -> 'hello'\n")
	if len(parseErrs) > 0 {
		t.Fatalf("unexpected parse errors: %v", parseErrs)
	}
	if len(typeErrs) == 0 {
		t.Fatalf("expected error for returning str from int-annotated function")
	}
	joined := strings.Join(typeErrs, "\n")
	if !strings.Contains(joined, "return type") || !strings.Contains(joined, "int") {
		t.Fatalf("expected return type mismatch error, got: %v", typeErrs)
	}
}

// --- Builtin argument type errors ---

func TestNeg_SqrtExpectsFloat(t *testing.T) {
	_, _, parseErrs, typeErrs := testutil.Analyze("sqrt('hello')\n")
	if len(parseErrs) > 0 {
		t.Fatalf("unexpected parse errors: %v", parseErrs)
	}
	if len(typeErrs) == 0 {
		t.Fatalf("expected type error for sqrt(str)")
	}
	joined := strings.Join(typeErrs, "\n")
	if !strings.Contains(joined, "expects") {
		t.Fatalf("expected argument type error, got: %v", typeErrs)
	}
}

// --- Dot-call on unsupported types ---

func TestNeg_DotCallOnFunction(t *testing.T) {
	_, _, parseErrs, typeErrs := testutil.Analyze("fn f() -> 1\nf.method()\n")
	if len(parseErrs) > 0 {
		t.Fatalf("unexpected parse errors: %v", parseErrs)
	}
	if len(typeErrs) == 0 {
		t.Fatalf("expected error for dot-call on function type")
	}
	joined := strings.Join(typeErrs, "\n")
	if !strings.Contains(joined, "not supported") || !strings.Contains(joined, "dot-call") {
		t.Fatalf("expected 'dot-call not supported' error, got: %v", typeErrs)
	}
}

// --- Multiple errors don't cascade ---

func TestNeg_MultipleIndependentErrors(t *testing.T) {
	// Two independent errors should both be reported
	_, _, parseErrs, typeErrs := testutil.Analyze("len()\nsqrt('x')\n")
	if len(parseErrs) > 0 {
		t.Fatalf("unexpected parse errors: %v", parseErrs)
	}
	if len(typeErrs) < 2 {
		t.Fatalf("expected at least 2 independent errors, got %d: %v", len(typeErrs), typeErrs)
	}
}

// --- Valid programs should NOT produce errors ---

func TestValid_MethodChainOnString(t *testing.T) {
	_, _, parseErrs, typeErrs := testutil.Analyze("x = 'hello'.upper().trim()\nprintln(x)\n")
	if len(parseErrs) > 0 {
		t.Fatalf("unexpected parse errors: %v", parseErrs)
	}
	if len(typeErrs) > 0 {
		t.Fatalf("unexpected type errors for valid method chain: %v", typeErrs)
	}
}

func TestValid_ListPushAndGet(t *testing.T) {
	_, _, parseErrs, typeErrs := testutil.Analyze("xs = list []\nxs.push(1)\nxs.push(2)\nprintln(xs.get(0))\n")
	if len(parseErrs) > 0 {
		t.Fatalf("unexpected parse errors: %v", parseErrs)
	}
	if len(typeErrs) > 0 {
		t.Fatalf("unexpected type errors for valid list operations: %v", typeErrs)
	}
}

func TestValid_DictSetAndGet(t *testing.T) {
	_, _, parseErrs, typeErrs := testutil.Analyze("d = dict {}\nd.set('key', 1)\nprintln(d.get('key'))\n")
	if len(parseErrs) > 0 {
		t.Fatalf("unexpected parse errors: %v", parseErrs)
	}
	if len(typeErrs) > 0 {
		t.Fatalf("unexpected type errors for valid dict operations: %v", typeErrs)
	}
}

func TestValid_PipeIntoBuiltin(t *testing.T) {
	// Note: ok captures the entire RHS, so "ok 1 | unwrap()" parses as ok(1 | unwrap()).
	// Use parens to pipe a result into unwrap: (ok 1) can't be written that way either.
	// Instead, use a variable to hold the result.
	_, _, parseErrs, typeErrs := testutil.Analyze("r = ok 1\nx = r | unwrap()\nprintln(x)\n")
	if len(parseErrs) > 0 {
		t.Fatalf("unexpected parse errors: %v", parseErrs)
	}
	if len(typeErrs) > 0 {
		t.Fatalf("unexpected type errors for valid pipe: %v", typeErrs)
	}
}

func TestValid_ClosureCapture(t *testing.T) {
	_, _, parseErrs, typeErrs := testutil.Analyze("x = 10\nf = fn() -> x + 1\nprintln(f())\n")
	if len(parseErrs) > 0 {
		t.Fatalf("unexpected parse errors: %v", parseErrs)
	}
	if len(typeErrs) > 0 {
		t.Fatalf("unexpected type errors for valid closure: %v", typeErrs)
	}
}

func TestValid_DefaultParameters(t *testing.T) {
	_, _, parseErrs, typeErrs := testutil.Analyze("fn add(x: int, y: int = 0) int -> x + y\nprintln(add(5))\nprintln(add(5, 3))\n")
	if len(parseErrs) > 0 {
		t.Fatalf("unexpected parse errors: %v", parseErrs)
	}
	if len(typeErrs) > 0 {
		t.Fatalf("unexpected type errors for valid default params: %v", typeErrs)
	}
}

func TestValid_BreakInsideLoop(t *testing.T) {
	_, _, parseErrs, typeErrs := testutil.Analyze("while true:\n    break\n")
	if len(parseErrs) > 0 {
		t.Fatalf("unexpected parse errors: %v", parseErrs)
	}
	if len(typeErrs) > 0 {
		t.Fatalf("unexpected type errors for break inside loop: %v", typeErrs)
	}
}

func TestValid_ContinueInsideLoop(t *testing.T) {
	_, _, parseErrs, typeErrs := testutil.Analyze("for x in range(10):\n    continue\n")
	if len(parseErrs) > 0 {
		t.Fatalf("unexpected parse errors: %v", parseErrs)
	}
	if len(typeErrs) > 0 {
		t.Fatalf("unexpected type errors for continue inside loop: %v", typeErrs)
	}
}

func TestValid_IndexThenMethodCall(t *testing.T) {
	_, _, parseErrs, typeErrs := testutil.Analyze("xs = list ['hello', 'world']\nprintln(xs[0].upper())\n")
	if len(parseErrs) > 0 {
		t.Fatalf("unexpected parse errors: %v", parseErrs)
	}
	if len(typeErrs) > 0 {
		t.Fatalf("unexpected type errors for index-then-method: %v", typeErrs)
	}
}

func TestValid_SplitThenIndex(t *testing.T) {
	_, _, parseErrs, typeErrs := testutil.Analyze("parts = 'a,b,c'.split(',')\nprintln(parts[0])\n")
	if len(parseErrs) > 0 {
		t.Fatalf("unexpected parse errors: %v", parseErrs)
	}
	if len(typeErrs) > 0 {
		t.Fatalf("unexpected type errors for split-then-index: %v", typeErrs)
	}
}
