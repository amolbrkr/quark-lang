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

func TestVectorInplaceAdd_BuiltinRegistered(t *testing.T) {
	_, _, parseErrs, typeErrs := testutil.Analyze("v = vector [1,2,3]\nvadd_inplace(v, 1)\n")
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
