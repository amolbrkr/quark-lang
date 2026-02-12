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
