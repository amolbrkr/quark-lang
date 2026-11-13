package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestEvaluationOrder runs programs whose parts used to run at the wrong time
// (semantics.md §4.5): each part of an expression runs only when the
// expression needs it, once, left to right.
func TestEvaluationOrder(t *testing.T) {
	cases := []struct {
		name, src, want string
	}{
		{
			name: "and_or_ternary_short_circuit",
			src: "fn noisy() ->\n    print('noisy called')\n    'x'\n" +
				"print(false and noisy().upper())\n" +
				"print('x' if true else noisy().upper())\n" +
				"print(true or noisy().upper())\n" +
				"print(true and noisy().upper())\n",
			want: "false\nx\ntrue\nnoisy called\nX\n",
		},
		{
			name: "while_condition_each_iteration",
			src: "x = 'aaa'\nn = 0\nwhile x.contains('a'):\n    x = x.replace('a', 'b')\n    n = n + 1\n" +
				"print(x)\nprint(n)\n",
			want: "bbb\n1\n",
		},
		{
			name: "receiver_before_arguments",
			src: "fn f() ->\n    print('f')\n    'abc'\nfn g() ->\n    print('g')\n    'b'\n" +
				"print(f().contains(g().upper()))\n",
			want: "f\ng\nfalse\n",
		},
		{
			name: "elseif_condition_with_method_call",
			src: "fn name() -> 'b'\n" +
				"if name().upper() == 'A':\n    print('a')\n" +
				"elseif name().upper() == 'B':\n    print('b')\n" +
				"else:\n    print('other')\n",
			want: "b\n",
		},
		{
			name: "elseif_condition_runs_only_when_reached",
			src: "fn noisy() ->\n    print('noisy called')\n    'x'\n" +
				"if true:\n    print('first')\nelseif noisy().upper() == 'X':\n    print('second')\n",
			want: "first\n",
		},
		{
			name: "member_assignment_value_runs_once",
			src: "fn bump() ->\n    print('bump')\n    1\n" +
				"fn setit(d) ->\n    d.x = bump()\n" +
				"d = dict { x: 0 }\nprint(setit(d))\nprint(d.x)\n",
			want: "bump\n1\n1\n",
		},
		{
			name: "arguments_left_to_right",
			src: "fn t(s) ->\n    print(s)\n    s\n" +
				"fn three(a, b, c) -> a + b + c\n" +
				"print(three(t('a'), t('b'), t('c')))\n" +
				"print(t('l') + t('r'))\n",
			want: "a\nb\nc\nabc\nl\nr\nlr\n",
		},
		{
			name: "while_condition_variable_after_loop",
			src: "xs = list ['a', 'b']\ni = 0\nwhile s = xs.get(i):\n    i = i + 1\nprint(s)\nprint(i)\n",
			want: "null\n2\n",
		},
		{
			name: "dynamic_method_arguments_before_dispatch",
			src: "fn id(v) -> v\nfn t(v) ->\n    print(v)\n    v\n" +
				"print(id('abcdef').slice(t(1), t(3)))\n",
			want: "1\n3\nbc\n",
		},
		{
			name: "and_or_of_untyped_values_return_an_operand",
			src: "fn id(x) -> x\ns = id(0) or id(5)\nprint(s)\nu = id(2) and id(0)\nprint(u)\n",
			want: "5\n0\n",
		},
		{
			name: "bool_never_equals_a_number",
			src: "b = true\nprint(b == 1)\nprint(b != 1)\n" +
				"fn is_pos(x: int) bool -> x > 0\nprint(is_pos(5) == 1)\nprint((!b) == 0)\n",
			want: "false\ntrue\nfalse\nfalse\n",
		},
		{
			name: "when_value_computed_once",
			src: "fn t(v) ->\n    print(v)\n    v\n" +
				"fn pick(x) ->\n    when t(x):\n        1 -> 'one'\n        2 -> 'two'\n        _ -> 'other'\n" +
				"print(pick(2))\n",
			want: "2\ntwo\n",
		},
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
			if got := strings.TrimLeft(r.stdout, "\n"); got != tc.want {
				t.Fatalf("output:\n%s\nwant:\n%s", got, tc.want)
			}
		})
	}
}
