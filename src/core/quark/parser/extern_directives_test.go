package parser_test

import (
	"strings"
	"testing"

	"quark/ast"
	"quark/internal/testutil"
)

func TestExternDirectives_Parse(t *testing.T) {
	src := "extern include_path 'include'\nextern link 'openblas'\nextern 'glue.hpp'\nlink = 1\n"
	node, errs := testutil.Parse(src)
	if len(errs) > 0 {
		t.Fatalf("unexpected parse errors: %v", errs)
	}
	want := []struct {
		typ ast.NodeType
		arg string
	}{
		{ast.ExternIncludePathNode, "include"},
		{ast.ExternLinkNode, "openblas"},
		{ast.ExternHeaderNode, "glue.hpp"},
	}
	if len(node.Children) != len(want)+1 {
		t.Fatalf("expected %d statements, got %d", len(want)+1, len(node.Children))
	}
	for i, w := range want {
		n := node.Children[i]
		if n.NodeType != w.typ {
			t.Fatalf("statement %d: expected %s, got %s", i, w.typ, n.NodeType)
		}
		if len(n.Children) != 1 || n.Children[0].TokenLiteral() != w.arg {
			t.Fatalf("statement %d: expected argument %q, got %v", i, w.arg, n.Children)
		}
	}
	// `link` is a contextual word: still usable as an identifier.
	if node.Children[3].NodeType != ast.OperatorNode {
		t.Fatalf("expected 'link = 1' to parse as an assignment, got %s", node.Children[3].NodeType)
	}
}

func TestExternDirectives_Errors(t *testing.T) {
	cases := []struct {
		name, src, want string
	}{
		{"trailing words reserved", "extern link 'duckdb' static\n", "options after the string are reserved"},
		{"trailing words on header", "extern 'glue.hpp' extra\n", "options after the string are reserved"},
		{"missing string", "extern link duckdb\n", "expected a string after 'extern link'"},
		{"empty name", "extern link ''\n", "needs a non-empty string"},
		{"file name instead of library name", "extern link 'libduckdb.so'\n", "without 'lib' and the file extension"},
		{"path in library name", "extern link '/opt/lib/duckdb'\n", "--lib-path"},
		{"flag in library name", "extern link '-lduckdb'\n", "leading '-'"},
		{"include is not a directive", "extern include 'dir'\n", "use extern include_path"},
		{"source not supported yet", "extern source 'glue.cpp'\n", "extern source is not supported yet"},
		{"unknown word", "extern frobnicate 'x'\n", "'link' or 'include_path'"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, errs := testutil.Parse(tc.src)
			joined := strings.Join(errs, "\n")
			if !strings.Contains(joined, tc.want) {
				t.Fatalf("expected error containing %q, got: %v", tc.want, errs)
			}
			if !strings.Contains(joined, "QK-PARSE-001") {
				t.Fatalf("expected QK-PARSE-001, got: %v", errs)
			}
		})
	}
}
