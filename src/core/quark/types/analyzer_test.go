package types_test

import (
	"strings"
	"testing"

	"quark/internal/testutil"
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
