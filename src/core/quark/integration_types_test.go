package main

// Element types, variable types and return annotations across files and
// native code (architecture.md §12.9, rules A-E): a list or dict keeps the
// element type it was created with however it is written to.

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestElementTypes_ModulesAndNative checks writes into typed lists and dicts
// from another module's functions and from native code (qext::list_push,
// qext::dict_set), returns checked across a module boundary, and native
// functions returning a null pointer for a type that has no null.
func TestElementTypes_ModulesAndNative(t *testing.T) {
	tmp := t.TempDir()
	if err := os.MkdirAll(filepath.Join(tmp, "lib"), 0o755); err != nil {
		t.Fatal(err)
	}
	glue := strings.Join([]string{
		"#pragma once",
		"#include <cstdint>",
		"inline QList* qei_push_str(QList* l) { qext::list_push(l, qext::box(\"x\")); return l; }",
		"inline QList* qei_push_int(QList* l) { qext::list_push(l, qext::box(static_cast<int64_t>(5))); return l; }",
		"inline QDict* qei_set_str(QDict* d) { qext::dict_set(d, \"k\", qext::box(\"s\")); return d; }",
		"inline QList* qei_null_list() { return nullptr; }",
		"inline const char* qei_null_str() { return nullptr; }",
		"",
	}, "\n")
	native := "extern './glue.hpp'\n" +
		"extern fn push_str(l: list) list as 'qei_push_str'\n" +
		"extern fn push_int(l: list) list as 'qei_push_int'\n" +
		"extern fn set_str(d: dict) dict as 'qei_set_str'\n" +
		"extern fn null_list() list as 'qei_null_list'\n" +
		"extern fn null_str() str as 'qei_null_str'\n"
	files := map[string]string{
		"glue.hpp": glue,
		"lib/util.qrk": "module util:\n" +
			"    fn add(xs, v) -> xs.push(v)\n" +
			"    fn put(d, v) ->\n" +
			"        d.k = v\n" +
			"    fn count(xs) int -> xs.get(0)\n",
		"ok.qrk": "incl './lib/util' as u\n" + native +
			"fs = list [0.5]\nu::add(fs, 2)\npush_int(fs)\nprint(fs)\n" +
			"any_list: list = list [1]\npush_str(any_list)\nprint(any_list)\n" +
			"print(u::count(list [7]))\n",
		"module_push.qrk": "incl './lib/util' as u\nxs = list [1, 2]\nu::add(xs, 'three')\n",
		"module_key.qrk":  "incl './lib/util' as u\nd = dict { a: 1 }\nu::put(d, 'x')\n",
		"module_ret.qrk":  "incl './lib/util' as u\nprint(u::count(list ['a']))\n",
		"native_push.qrk": native + "xs = list [1, 2]\npush_str(xs)\n",
		"native_key.qrk":  native + "d = dict { a: 1 }\nset_str(d)\n",
		"native_null.qrk": native + "print(len(null_list()))\n",
		"native_nstr.qrk": native + "print(null_str())\n",
	}
	for name, src := range files {
		if err := os.WriteFile(filepath.Join(tmp, name), []byte(src), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	got := strings.TrimSpace(normalizeNewlines(runQuarkIn(t, tmp, "run", "ok.qrk")))
	want := "[0.5, 2.0, 5.0]\n[1, 'x']\n7"
	if got != want {
		t.Fatalf("ok.qrk: got\n%s\nwant\n%s", got, want)
	}

	cases := []struct{ file, want string }{
		{"module_push.qrk", "push() can't add a str to a list of int"},
		{"module_key.qrk", "can't store a str under key 'k' of a dict of int"},
		{"module_ret.qrk", "function 'util::count' declares return type int but returned str"},
		{"native_push.qrk", "qext::list_push() can't add a str to a list of int"},
		{"native_key.qrk", "can't store a str under key 'k' of a dict of int"},
		{"native_null.qrk", "a native function declared to return list returned a null pointer"},
		{"native_nstr.qrk", "a native function declared to return str returned a null pointer"},
	}
	for _, tc := range cases {
		cmd := exec.Command(quarkExePath, "run", tc.file)
		cmd.Dir = tmp
		out, err := cmd.CombinedOutput()
		if err == nil || !strings.Contains(string(out), tc.want) {
			t.Errorf("%s: expected a runtime error containing %q, got (err=%v):\n%s", tc.file, tc.want, err, out)
		}
	}
}
