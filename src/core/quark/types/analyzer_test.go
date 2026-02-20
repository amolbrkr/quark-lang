package types_test

import (
	"strings"
	"testing"

	"quark/ast"
	"quark/internal/testutil"
	qtypes "quark/types"
)

func TestBuiltins_ArityError(t *testing.T) {
	_, _, parseErrs, typeErrs := testutil.Analyze("len()\n")
	if len(parseErrs) > 0 {
		t.Fatalf("unexpected parse errors: %v", parseErrs)
	}
	if len(typeErrs) == 0 {
		t.Fatalf("expected type error for len() with 0 args")
	}
	joined := strings.Join(typeErrs, "\n")
	if !strings.Contains(joined, "builtin 'len' expects") {
		t.Fatalf("expected builtin len arity error, got: %v", typeErrs)
	}
}

func TestDictLiteral_DuplicateKeyError(t *testing.T) {
	_, _, parseErrs, typeErrs := testutil.Analyze("d = dict { a: 1, a: 2 }\n")
	if len(parseErrs) > 0 {
		t.Fatalf("unexpected parse errors: %v", parseErrs)
	}
	joined := strings.Join(typeErrs, "\n")
	if !strings.Contains(joined, "duplicate dict key") {
		t.Fatalf("expected duplicate dict key error, got: %v", typeErrs)
	}
}

func TestDictHelpers_BuiltinsRegistered(t *testing.T) {
	_, _, parseErrs, typeErrs := testutil.Analyze("d = dict { a: 1 }\nprintln(dget(d, 'a'))\n")
	if len(parseErrs) > 0 {
		t.Fatalf("unexpected parse errors: %v", parseErrs)
	}
	if len(typeErrs) > 0 {
		t.Fatalf("unexpected type errors: %v", typeErrs)
	}
}

func TestSplit_BuiltinRegistered(t *testing.T) {
	_, _, parseErrs, typeErrs := testutil.Analyze("println(split('a,b', ','))\n")
	if len(parseErrs) > 0 {
		t.Fatalf("unexpected parse errors: %v", parseErrs)
	}
	if len(typeErrs) > 0 {
		t.Fatalf("unexpected type errors: %v", typeErrs)
	}
}

func TestVectorLiteral_InfersVectorFloat(t *testing.T) {
	analyzer, node, parseErrs, typeErrs := testutil.Analyze("vector [1, 2, 3]\n")
	if len(parseErrs) > 0 {
		t.Fatalf("unexpected parse errors: %v", parseErrs)
	}
	if len(typeErrs) > 0 {
		t.Fatalf("unexpected type errors: %v", typeErrs)
	}
	if len(node.Children) != 1 {
		t.Fatalf("expected one top-level expression, got %d", len(node.Children))
	}
	typ := analyzer.Analyze(node.Children[0])
	vec, ok := typ.(*qtypes.VectorType)
	if !ok {
		t.Fatalf("expected VectorType, got %T (%v)", typ, typ)
	}
	if !vec.ElementType.Equals(qtypes.TypeFloat) {
		t.Fatalf("expected vector element type float, got %s", vec.ElementType.String())
	}
}

func TestVectorArithmetic_VectorAndScalar(t *testing.T) {
	analyzer, node, parseErrs, typeErrs := testutil.Analyze("v = vector [1,2,3]\nv + 2\n")
	if len(parseErrs) > 0 {
		t.Fatalf("unexpected parse errors: %v", parseErrs)
	}
	if len(typeErrs) > 0 {
		t.Fatalf("unexpected type errors: %v", typeErrs)
	}
	if len(node.Children) < 2 {
		t.Fatalf("expected at least 2 top-level nodes, got %d", len(node.Children))
	}
	if node.Children[1].NodeType != ast.OperatorNode {
		t.Fatalf("expected second expression to be operator, got %v", node.Children[1])
	}
	typ := analyzer.Analyze(node.Children[1])
	if _, ok := typ.(*qtypes.VectorType); !ok {
		t.Fatalf("expected vector result type, got %T (%v)", typ, typ)
	}
}

func TestVectorReductions_Builtins(t *testing.T) {
	_, _, parseErrs, typeErrs := testutil.Analyze("v = vector [1,2,3]\nprintln(sum(v))\nprintln(min(v))\nprintln(max(v))\n")
	if len(parseErrs) > 0 {
		t.Fatalf("unexpected parse errors: %v", parseErrs)
	}
	if len(typeErrs) > 0 {
		t.Fatalf("unexpected type errors: %v", typeErrs)
	}
}

func TestToVector_InfersVectorIntType(t *testing.T) {
	analyzer, node, parseErrs, typeErrs := testutil.Analyze("xs = list [1,2,3]\nv = to_vector(xs)\nv\n")
	if len(parseErrs) > 0 {
		t.Fatalf("unexpected parse errors: %v", parseErrs)
	}
	if len(typeErrs) > 0 {
		t.Fatalf("unexpected type errors: %v", typeErrs)
	}
	if len(node.Children) < 3 {
		t.Fatalf("expected at least 3 top-level nodes, got %d", len(node.Children))
	}
	typ := analyzer.Analyze(node.Children[2])
	vec, ok := typ.(*qtypes.VectorType)
	if !ok {
		t.Fatalf("expected VectorType, got %T (%v)", typ, typ)
	}
	if !vec.ElementType.Equals(qtypes.TypeInt) {
		t.Fatalf("expected vector element type int, got %s", vec.ElementType.String())
	}
}

func TestToVector_InfersVectorFloatType(t *testing.T) {
	analyzer, node, parseErrs, typeErrs := testutil.Analyze("xs = list [1.0,2.0,3.0]\nv = to_vector(xs)\nv\n")
	if len(parseErrs) > 0 {
		t.Fatalf("unexpected parse errors: %v", parseErrs)
	}
	if len(typeErrs) > 0 {
		t.Fatalf("unexpected type errors: %v", typeErrs)
	}
	if len(node.Children) < 3 {
		t.Fatalf("expected at least 3 top-level nodes, got %d", len(node.Children))
	}
	typ := analyzer.Analyze(node.Children[2])
	vec, ok := typ.(*qtypes.VectorType)
	if !ok {
		t.Fatalf("expected VectorType, got %T (%v)", typ, typ)
	}
	if !vec.ElementType.Equals(qtypes.TypeFloat) {
		t.Fatalf("expected vector element type float, got %s", vec.ElementType.String())
	}
}

func TestToVector_RejectsMixedNumericList(t *testing.T) {
	_, _, parseErrs, typeErrs := testutil.Analyze("xs = list [1, 2.0, 3]\nv = to_vector(xs)\n")
	if len(parseErrs) > 0 {
		t.Fatalf("unexpected parse errors: %v", parseErrs)
	}
	if len(typeErrs) == 0 {
		t.Fatalf("expected type error for mixed int/float list in to_vector")
	}
	joined := strings.Join(typeErrs, "\n")
	if !strings.Contains(joined, "homogeneous numeric") {
		t.Fatalf("expected homogeneous numeric error, got: %v", typeErrs)
	}
}

func TestArithmetic_RejectsListPlusInt(t *testing.T) {
	_, _, parseErrs, typeErrs := testutil.Analyze("nums = list [1, 2, 3]\nprintln(nums + 1)\n")
	if len(parseErrs) > 0 {
		t.Fatalf("unexpected parse errors: %v", parseErrs)
	}
	if len(typeErrs) == 0 {
		t.Fatalf("expected type error for list + int")
	}
	joined := strings.Join(typeErrs, "\n")
	if !strings.Contains(joined, "requires numeric operands") {
		t.Fatalf("expected numeric operand error, got: %v", typeErrs)
	}
}

func TestForLoop_AllowsVectorIterable(t *testing.T) {
	_, _, parseErrs, typeErrs := testutil.Analyze("for x in to_vector(range(3)):\n    println(x)\n")
	if len(parseErrs) > 0 {
		t.Fatalf("unexpected parse errors: %v", parseErrs)
	}
	if len(typeErrs) > 0 {
		t.Fatalf("unexpected type errors: %v", typeErrs)
	}
}

func TestType_BuiltinRegistered(t *testing.T) {
	_, _, parseErrs, typeErrs := testutil.Analyze("v = vector [1,2,3]\nprintln(type(v))\n")
	if len(parseErrs) > 0 {
		t.Fatalf("unexpected parse errors: %v", parseErrs)
	}
	if len(typeErrs) > 0 {
		t.Fatalf("unexpected type errors: %v", typeErrs)
	}
}

func TestVectorFillnaAndAstype_BuiltinsRegistered(t *testing.T) {
	_, _, parseErrs, typeErrs := testutil.Analyze("v = vector [1,2,3]\nprintln(fillna(v, 0))\nprintln(astype(v, 'i64'))\n")
	if len(parseErrs) > 0 {
		t.Fatalf("unexpected parse errors: %v", parseErrs)
	}
	if len(typeErrs) > 0 {
		t.Fatalf("unexpected type errors: %v", typeErrs)
	}
}

func TestVectorCategoricalBuiltinsRegistered(t *testing.T) {
	_, _, parseErrs, typeErrs := testutil.Analyze("xs = list ['red','blue','red']\nc = cat_from_str(xs)\nprintln(cat_to_str(c))\n")
	if len(parseErrs) > 0 {
		t.Fatalf("unexpected parse errors: %v", parseErrs)
	}
	if len(typeErrs) > 0 {
		t.Fatalf("unexpected type errors: %v", typeErrs)
	}
}
