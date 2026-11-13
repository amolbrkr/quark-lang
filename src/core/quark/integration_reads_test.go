package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// writeFiles writes name -> source into dir (names may contain a directory).
func writeFiles(t *testing.T, dir string, files map[string]string) {
	t.Helper()
	for name, src := range files {
		path := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(src), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

// TestForLoop_ListResized checks that a `for` loop over a list fails at the
// `for` when its body changes the list's length (semantics.md §14), on every
// path into the loop: a list the analyzer typed, a list of unknown type, a
// `continue`, a change in the last iteration, a loop in a function and in
// another file.
func TestForLoop_ListResized(t *testing.T) {
	dir := t.TempDir()
	writeFiles(t, dir, map[string]string{
		"pop.qrk":       "xs = list [1, 2, 3]\nfor y in xs:\n    print(y)\n    xs.pop()\n",
		"push.qrk":      "xs = list [1, 2, 3]\nfor y in xs:\n    xs.push(y)\n",
		"insert.qrk":    "xs = list ['a', 'b']\nfor s in xs:\n    xs.insert(0, s)\n",
		"remove.qrk":    "xs = list [1.5, 2.5]\nfor v in xs:\n    xs.remove(0)\n",
		"untyped.qrk":   "fn id(v) -> v\nxs = list [1, 2, 3]\nfor y in id(xs):\n    xs.pop()\n",
		"continue.qrk":  "xs = list [1, 2, 3]\nfor y in xs:\n    if y == 1:\n        xs.pop()\n        continue\n    print(y)\n",
		"last.qrk":      "xs = list [1, 2, 3]\nfor y in xs:\n    if y == 3:\n        xs.push(4)\nprint('after')\n",
		"in_fn.qrk":     "fn drain(xs) ->\n    for x in xs:\n        xs.pop()\nys = list [4, 5]\ndrain(ys)\n",
		"in_lib.qrk":    "incl './lib/drain'\nzs = list [1]\ndrain::drain(zs)\n",
		"lib/drain.qrk": "fn drain(xs) ->\n    for x in xs:\n        xs.pop()\n",
	})
	cases := []struct{ file, loc, out string }{
		{"pop.qrk", "pop.qrk:2:1", "1\n"},
		{"push.qrk", "push.qrk:2:1", ""},
		{"insert.qrk", "insert.qrk:2:1", ""},
		{"remove.qrk", "remove.qrk:2:1", ""},
		{"untyped.qrk", "untyped.qrk:3:1", ""},
		{"continue.qrk", "continue.qrk:2:1", ""},
		{"last.qrk", "last.qrk:2:1", ""},
		{"in_fn.qrk", "in_fn.qrk:2:5", ""},
		{"in_lib.qrk", "lib/drain.qrk:2:5", ""},
	}
	for _, tc := range cases {
		t.Run(tc.file, func(t *testing.T) {
			r := runIn(dir, os.Environ(), quarkExePath, "run", tc.file)
			if r.err == nil {
				t.Fatalf("expected a runtime error, got output:\n%s", r.stdout)
			}
			if !strings.Contains(r.stderr, "list changed size during the for loop over it") || !strings.Contains(r.stderr, tc.loc) {
				t.Fatalf("expected the resize error at %s, got:\n%s", tc.loc, r.stderr)
			}
			if r.stdout != tc.out {
				t.Fatalf("output before the error:\n%s\nwant:\n%s", r.stdout, tc.out)
			}
		})
	}
}

// TestForLoop_ListAllowed checks the loops the length rule allows: writes
// that keep the length, a change followed by `break`, nested loops over one
// list, element kinds that aren't raw scalars, captured loop variables, and
// strings and vecs.
func TestForLoop_ListAllowed(t *testing.T) {
	cases := []struct{ name, src, want string }{
		{"set_keeps_length", "xs = list [1, 2, 3]\nfor y in xs:\n    xs[0] = y * 10\nprint(xs)\n", "[30, 2, 3]\n"},
		// Each read sees the list as it is: the second iteration reads the 11.
		{"pop_then_push", "xs = list [1, 2]\nfor y in xs:\n    xs.pop()\n    xs.push(y + 10)\nprint(xs)\n", "[1, 21]\n"},
		{"remove_then_break", "xs = list [1, 2, 3]\nfor y in xs:\n    if y == 2:\n        xs.remove(1)\n        break\nprint(xs)\n", "[1, 3]\n"},
		{"nested_same_list", "xs = list [1, 2]\nfor a in xs:\n    for b in xs:\n        print(a * 10 + b)\n", "11\n12\n21\n22\n"},
		{"floats_bools_strs", "for f in list [1.5, 2.5]:\n    print(f)\nfor b in list [true, false]:\n    print(b)\nfor s in list ['a', 'b']:\n    print(s.upper())\n", "1.5\n2.5\ntrue\nfalse\nA\nB\n"},
		{"mixed_any", "for v in list [1, 'a', null]:\n    print(v)\n", "1\na\nnull\n"},
		{"captured", "fs = list []\nfor y in list [1, 2, 3]:\n    fs.push(fn() -> y * 2)\nfor f in fs:\n    print(f())\n", "2\n4\n6\n"},
		{"empty", "xs = list []\nfor y in xs:\n    print(y)\nprint('none')\n", "none\n"},
		{"string_and_vec", "for c in 'hi':\n    print(c)\nfor v in vec [1.5, 2.5]:\n    print(v)\n", "h\ni\n1.5\n2.5\n"},
		{"list_of_lists", "for row in list [list [1, 2], list [3]]:\n    print(len(row))\n", "2\n1\n"},
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

// TestReads_ProbingAndTyped checks the reads that stay allowed now that
// `xs[i]` and `s[i]` out of range are errors and `.get` has type any
// (semantics.md §1.3, §9): probing with `.get`, converting its result once,
// negative indexes, member access on an untyped result, and `when` on values
// that aren't vecs.
func TestReads_ProbingAndTyped(t *testing.T) {
	cases := []struct{ name, src, want string }{
		{"get_past_end_is_null", "xs = list [1, 2]\nprint(xs.get(5))\nprint(xs.get(-1))\n", "null\n2\n"},
		{"get_converted_once", "xs = list [1, 2]\nn: int = xs.get(0)\nt = 0\nt = t + n\nprint(t)\n", "1\n"},
		{"dict_get_missing", "d = dict { a: 1 }\nprint(d.get('b'))\nv: int = d.get('a')\nprint(v + 1)\n", "null\n2\n"},
		{"get_then_member", "rows = list [dict { name: 'b' }]\nprint(rows.get(0).name)\n", "b\n"},
		{"index_negative", "xs = list [1, 2, 3]\nprint(xs[-1])\nprint('abc'[-1])\n", "3\nc\n"},
		{"index_typed_arith", "xs = list [1, 2, 3]\nt = 0\nfor i in range(3):\n    t = t + xs[i]\nprint(t)\n", "6\n"},
		{"when_values", "fn g(a) ->\n    when a:\n        1 -> 'one'\n        's' -> 'str'\n        null -> 'null'\n        _ -> 'other'\n" +
			"print(g(1))\nprint(g('s'))\nprint(g(null))\nprint(g(list [1]))\nprint(g(1.0))\n", "one\nstr\nnull\nother\none\n"},
		{"when_vec_wildcard", "v = vec [1, 1]\nwhen v:\n    _ -> print('any')\n", "any\n"},
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
