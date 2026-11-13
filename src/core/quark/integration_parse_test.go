package main

import (
	"os"
	"path/filepath"
	"testing"
)

// TestStatementEnd_Programs runs programs whose statements end with a block
// of their own (grammar.md §4): the next line runs as a statement of its own,
// not as part of the block.
func TestStatementEnd_Programs(t *testing.T) {
	cases := []struct{ name, src, want string }{
		{"fn_with_inline_when", "fn pick(x) -> when x:\n    1 -> 'one'\n    _ -> 'other'\nprint(pick(1))\nprint(pick(2))\n", "one\nother\n"},
		{"fn_assigning_inline_when", "fn h(x) -> y = when x:\n    1 -> 'one'\n    _ -> 'other'\nprint(h(1))\n", "one\n"},
		{"inline_when_in_if", "if false: when 1:\n    1 -> print('one')\n    _ -> print('other')\nprint('after')\n", "after\n"},
		{"inline_if_elseif_else", "x = 2\nif x == 1: print('one')\nelseif x == 2: print('two')\nelse: print('other')\nprint('done')\n", "two\ndone\n"},
		{"pipe_same_indentation", "fn twice(x) -> x * 2\ny = 3\n| twice()\n| twice()\nprint(y)\n", "12\n"},
		{"pipe_indented", "fn twice(x) -> x * 2\ny = 3\n    | twice()\n    | twice()\nz = y + 1\nprint(z)\n", "13\n"},
		{"pipe_indented_in_fn_body", "fn twice(x) -> x * 2\nfn quad(x) ->\n    r = x\n        | twice()\n        | twice()\n    r\nprint(quad(1))\n", "4\n"},
		{"pipe_indented_with_comment", "fn twice(x) -> x * 2\nprint(3\n    // doubled\n    | twice())\n", "6\n"},
	}
	dir := t.TempDir()
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(dir, tc.name+".qrk")
			if err := os.WriteFile(path, []byte(tc.src), 0o644); err != nil {
				t.Fatal(err)
			}
			r := runIn(dir, os.Environ(), quarkExePath, "run", path)
			if r.err != nil {
				t.Fatalf("quark run failed: %v\n--- stderr ---\n%s", r.err, r.stderr)
			}
			if r.stdout != tc.want {
				t.Fatalf("output:\n%s\nwant:\n%s", r.stdout, tc.want)
			}
		})
	}
}
