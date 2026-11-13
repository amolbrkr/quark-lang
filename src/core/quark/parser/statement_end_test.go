package parser_test

import (
	"strings"
	"testing"

	"quark/internal/testutil"
)

// TestStatementEnd_Rejected checks that a statement, a `when` arm or a struct
// field followed by more on its line is a parse error, never two statements
// (or arms, or fields) run one after the other.
func TestStatementEnd_Rejected(t *testing.T) {
	cases := []struct{ name, src, want string }{
		{"two names", "price = 4\nqty = 3\ntotal = price qty\n", "unexpected 'qty' after 'price'; put each statement on its own line, or join the two with an operator"},
		{"two strings", "y = 'a' 'b'\n", "unexpected string 'b' after string 'a'"},
		{"number then call", "x = 5 print(x)\n", "unexpected 'print' after '5'"},
		{"two calls", "print(1) print(2)\n", "unexpected 'print' after ')'"},
		{"stray paren", "x = 1)\n", "unexpected ')' after '1'"},
		{"typed declaration", "n: int = 1 2\n", "unexpected '2' after '1'"},
		{"in an indented block", "if true:\n    a = 1 2\n    print(a)\n", "unexpected '2' after '1'"},
		{"inline fn body", "fn f(x) -> x + 1 print(9)\n", "unexpected 'print' after '1'"},
		{"inline if body", "if true: x = 1 y = 2\n", "unexpected 'y' after '1'"},
		{"inline for body", "for i in range(2): print(i) print(i)\n", "unexpected 'print' after ')'"},
		{"lambda body", "g = fn(x) -> x 1\n", "unexpected '1' after 'x'"},
		{"in a module", "module m:\n    k = 1 2\n", "unexpected '2' after '1'"},
		{"when arm", "x = 1\nwhen x:\n    1 -> 'one' 'two'\n    _ -> 'other'\n", "unexpected string 'two' after string 'one'; put each when arm on its own line"},
		{"two fields on one line", "struct P:\n    x: int y: int\n", "unexpected 'y' after 'int'; put each field on its own line"},
		{"field default", "struct P:\n    x: int = 1 2\n", "unexpected '2' after '1'; put each field on its own line"},
		// A '|' line continues the expression on the line above; it can't be
		// dedented below it, follow a block, or end a line.
		{"pipe line dedented", "if true:\n    y = 3\n| f()\n", "'|' can't start a statement"},
		{"pipe line after a when block", "y = when 1:\n    1 -> 'a'\n    _ -> 'b'\n| upper()\n", "'|' can't start a statement"},
		{"pipe line opening a block", "if true:\n    | f()\n", "'|' can't start a statement"},
		{"trailing pipe", "y = 3 |\n    f()\n", "a line can't end with '|'; start the next line with it instead"},
		{"trailing pipe at the end of the file", "y = 3 |", "a line can't end with '|'"},
		{"deeper line without a pipe", "y = 3\n    + 1\n", "unexpected indent"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, errs := testutil.Parse(tc.src)
			if len(errs) == 0 {
				t.Fatalf("parsed with no error:\n%s", tc.src)
			}
			if !strings.Contains(strings.Join(errs, "\n"), tc.want) {
				t.Fatalf("errors:\n%s\nwant one containing %q", strings.Join(errs, "\n"), tc.want)
			}
		})
	}
}

// TestStatementEnd_Accepted checks the statements whose next statement
// follows with no line break in the token stream, because they end with a
// block of their own: each top-level statement stays separate.
func TestStatementEnd_Accepted(t *testing.T) {
	cases := []struct {
		name string
		src  string
		top  int // top-level statements
	}{
		{"after an if block", "if true:\n    print('in')\nprint('after')\n", 2},
		{"inline if, elseif, else", "x = 2\nif x == 1: print('one')\nelseif x == 2: print('two')\nelse: print('other')\nprint(x)\n", 3},
		{"inline for", "for i in range(2): print(i)\nprint('done')\n", 2},
		// The inline `when` body ends with the arms' block; the next line is a
		// statement of its own, not part of the function.
		{"fn with an inline when body", "fn pick(x) -> when x:\n    1 -> 'one'\n    _ -> 'other'\nprint(pick(1))\n", 2},
		{"fn assigning an inline when", "fn h(x) -> y = when x:\n    1 -> 'one'\n    _ -> 'other'\nprint(h(1))\n", 2},
		{"inline when in an if body", "if false: when 1:\n    1 -> print('one')\n    _ -> print('other')\nprint('after')\n", 2},
		{"inline when in a for body", "for i in range(0): when i:\n    1 -> print('one')\n    _ -> print('other')\nprint('after')\n", 2},
		{"lambda with an inline when", "g = fn(x) -> when x:\n    1 -> 'one'\n    _ -> 'other'\nprint(g(1))\n", 2},
		{"when statement", "when 1:\n    1 -> print('one')\n    _ -> print('other')\nprint('after')\n", 2},
		{"nested when arm", "r = when 1:\n    1 -> when 2:\n        2 -> 'two'\n        _ -> 'no'\n    _ -> 'other'\nprint(r)\n", 2},
		{"struct", "struct P:\n    x: int\n    y: int = 2\np = P { x: 1 }\n", 2},
		{"module", "module m:\n    fn f() -> 1\nprint(m::f())\n", 2},
		{"pipe continued at the same indentation", "y = 3\n| f()\n| g()\nprint(y)\n", 2},
		{"pipe continued on deeper lines", "y = 3\n    | f()\n    | g()\nprint(y)\n", 2},
		{"pipe lines at mixed depths", "y = 3\n    | f()\n        | g()\n  | h()\nprint(y)\n", 2},
		{"pipe lines in a block", "if true:\n    y = 3\n        | f()\n    print(y)\nprint(1)\n", 2},
		{"comment and blank lines between pipe lines", "y = 3\n    // double it\n    | f()\n\n    | g()\nprint(y)\n", 2},
		{"pipe line inside an argument", "print(3\n    | f())\n", 1},
		{"no newline at the end of the file", "x = 1", 1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			node, errs := testutil.Parse(tc.src)
			if len(errs) > 0 {
				t.Fatalf("parse errors: %v", errs)
			}
			if got := len(node.Children); got != tc.top {
				t.Fatalf("%d top-level statements, want %d", got, tc.top)
			}
		})
	}
}
