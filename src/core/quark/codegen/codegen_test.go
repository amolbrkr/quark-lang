package codegen_test

import (
	"strings"
	"testing"

	"quark/internal/testutil"
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
