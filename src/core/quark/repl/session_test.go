package repl

import (
	"bufio"
	"strings"
	"testing"

	"quark/driver"
)

func newReader(s string) *bufio.Reader {
	return bufio.NewReader(strings.NewReader(s))
}

func TestSession_ScalarSlotAndState(t *testing.T) {
	u := driver.CompileSession(".", []string{
		"n = 1\n",
		"n = n + 1\n",
	})
	if u.Failed() {
		t.Fatalf("compile: %v %v", u.Err, u.Diagnostics)
	}
	cpp := u.CPP
	for _, want := range []string{
		"extern \"C\" void quark_input_2()",
		"q_repl_ensure(",
		"long long& quark_n = q_repl_i64[0];",
		"q_checked_add(quark_n, 1)",
	} {
		if !strings.Contains(cpp, want) {
			t.Errorf("missing %s\n%s", want, cpp)
		}
	}
	if strings.Contains(cpp, "int main(") {
		t.Fatalf("session input emitted main:\n%s", cpp)
	}
	if strings.Contains(cpp, "q_add(") {
		t.Fatalf("top-level int arithmetic went through q_add:\n%s", cpp)
	}

	fn := driver.CompileSession(".", []string{"n = 2\nfn bump() -> n + 1\nbump()\n"})
	if fn.Failed() {
		t.Fatalf("fn: %v %v", fn.Err, fn.Diagnostics)
	}
	if !strings.Contains(fn.CPP, "long long& quark_n = q_repl_i64[0];") || !strings.Contains(fn.CPP, "q_checked_add(") {
		t.Fatalf("function did not read the session int as a raw slot:\n%s", fn.CPP)
	}
	if strings.Contains(fn.CPP, "q_alloc_closure((void*)quark_bump") {
		t.Fatalf("bump was a closure:\n%s", fn.CPP)
	}

	call := driver.CompileSession(".", []string{
		"n = 1\n",
		"fn inc(x: int) int -> x + 1\n",
		"inc(n)\n",
	})
	if call.Failed() {
		t.Fatalf("call: %v %v", call.Err, call.Diagnostics)
	}
	if !strings.Contains(call.CPP, "quark_inc(") || !strings.Contains(call.CPP, "q_print(") {
		t.Fatalf("call or display missing:\n%s", call.CPP)
	}
}

func TestSession_TypeChange(t *testing.T) {
	ok := driver.CompileSession(".", []string{
		"x = 1\n",
		"x = 'text'\n",
	})
	if ok.Failed() {
		t.Fatalf("type change should warn, not fail: %v", ok.Diagnostics)
	}
	var warned bool
	for _, d := range ok.Diagnostics {
		if d.Code == "QK-CHECK-002" && strings.Contains(d.Message, "changes type from int to str") {
			warned = true
		}
	}
	if !warned {
		t.Fatalf("missing type-change warning: %v", ok.Diagnostics)
	}
	if !strings.Contains(ok.CPP, "q_repl_i64_clear(0)") {
		t.Fatalf("old int slot was not cleared:\n%s", ok.CPP)
	}
	if !strings.Contains(ok.CPP, "q_repl_cell[") {
		t.Fatalf("new value was not stored in a cell:\n%s", ok.CPP)
	}

	bad := driver.CompileSession(".", []string{
		"n = 1\n",
		"fn bump() -> n + 1\n",
		"n = 'text'\n",
	})
	if !bad.Failed() {
		t.Fatal("type change of a variable a function uses should fail")
	}
	joined := ""
	for _, d := range bad.Diagnostics {
		joined += d.Message + "\n"
	}
	if !strings.Contains(joined, "bump") || !strings.Contains(joined, "can't change type") {
		t.Fatalf("got %s", joined)
	}
}

func TestSession_NoForwardRefAcrossInputs(t *testing.T) {
	u := driver.CompileSession(".", []string{
		"fn call() -> sq(1)\n",
		"fn sq(n) -> n\n",
	})
	if !u.Failed() {
		t.Fatal("a function must not see a function defined in a later input")
	}
	joined := ""
	for _, d := range u.Diagnostics {
		joined += d.Message
	}
	if !strings.Contains(joined, "undefined") && !strings.Contains(joined, "sq") {
		t.Fatalf("got %s", joined)
	}
}

func TestSession_DuplicateFn(t *testing.T) {
	u := driver.CompileSession(".", []string{
		"fn f(x) -> x\n",
		"fn f(x) -> x + 1\n",
	})
	if !u.Failed() {
		t.Fatal("expected a duplicate definition")
	}
	joined := ""
	for _, d := range u.Diagnostics {
		joined += d.String() + "\n"
	}
	if !strings.Contains(joined, "already defined") || !strings.Contains(joined, "<input 1>") {
		t.Fatalf("got %s", joined)
	}
}

func TestSession_FingerprintStable(t *testing.T) {
	one := driver.CompileSession(".", []string{"fn sq(n: int) int -> n * n\nn = 1\n"})
	if one.Failed() {
		t.Fatal(one.Diagnostics, one.Err)
	}
	two := driver.CompileSession(".", []string{"fn sq(n: int) int -> n * n\nn = 1\n", "sq(n)\n"})
	if two.Failed() {
		t.Fatal(two.Diagnostics, two.Err)
	}
	if fingerprint(one.Analysis, 1) != fingerprint(two.Analysis, 1) {
		t.Fatalf("re-analysis changed earlier names\n%q\n%q", fingerprint(one.Analysis, 1), fingerprint(two.Analysis, 1))
	}
}

func TestSession_EmptyListFixedLater(t *testing.T) {
	u := driver.CompileSession(".", []string{
		"xs = list []\n",
		"xs.push(1)\n",
	})
	if u.Failed() {
		t.Fatalf("%v %v", u.Err, u.Diagnostics)
	}
	if !strings.Contains(u.CPP, "q_settle_elems(") {
		t.Fatalf("missing settle:\n%s", u.CPP)
	}
	bad := driver.CompileSession(".", []string{
		"xs = list []\n",
		"xs.push(1)\n",
		"xs.push('a')\n",
	})
	if !bad.Failed() {
		t.Fatal("pushing a str into a list[int] should fail")
	}
}

func TestReadInput_BlockAndBrackets(t *testing.T) {
	src, err := readInput(newReader("for i in range(3):\n    i\n\nn = 1\n"), func() {})
	if err != nil {
		t.Fatal(err)
	}
	if src != "for i in range(3):\n    i\n" {
		t.Fatalf("got %q", src)
	}
	src, err = readInput(newReader("xs = list [1,\n2]\n"), func() {})
	if err != nil {
		t.Fatal(err)
	}
	if src != "xs = list [1,\n2]\n" {
		t.Fatalf("got %q", src)
	}
}
