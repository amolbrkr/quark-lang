package repl

import (
	"bufio"
	"os"
	"path/filepath"
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

func TestSession_CSVReadFromLaterInput(t *testing.T) {
	t.Setenv("QUARK_STDLIB_ROOT", stdlibRoot(t))

	structIn := "struct car:\n    model: str\n    mpg: int\n    cyl: int\n"
	read := "cars: tbl[car] = unwrap(csv::read_file('cars.csv'))\n"
	later := driver.CompileSession(".", []string{
		"incl 'std/csv'\n",
		structIn,
		read,
	})
	if later.Failed() {
		t.Fatalf("later read: %v %v", later.Err, later.Diagnostics)
	}
	if !strings.Contains(later.CPP, "csv.hpp") || !strings.Contains(later.CPP, "q_csv_read_file(") {
		t.Fatalf("later input did not include the csv reader:\n%s", later.CPP)
	}
	if !strings.Contains(later.CPP, "&_sdef_") {
		t.Fatalf("typed read did not pass the row struct:\n%s", later.CPP)
	}

	same := driver.CompileSession(".", []string{"incl 'std/csv'\n" + structIn + read})
	if same.Failed() {
		t.Fatalf("same input: %v %v", same.Err, same.Diagnostics)
	}
	if !strings.Contains(same.CPP, "csv.hpp") || !strings.Contains(same.CPP, "q_csv_read_file(") {
		t.Fatalf("same-input read lost the header:\n%s", same.CPP)
	}

	text := driver.CompileSession(".", []string{
		"incl 'std/csv'\n",
		"unwrap(csv::parse_text('a,b\\n1,2\\n'))\n",
	})
	if text.Failed() {
		t.Fatalf("parse_text: %v %v", text.Err, text.Diagnostics)
	}
	if !strings.Contains(text.CPP, "csv.hpp") || !strings.Contains(text.CPP, "q_csv_parse_text(") {
		t.Fatalf("parse_text did not include the reader:\n%s", text.CPP)
	}

	anyCall := driver.CompileSession(".", []string{
		"incl 'std/csv'\n",
		"fn id(v) -> v\ncsv::read_file(id(1))\n",
	})
	if anyCall.Failed() {
		t.Fatalf("any argument should reach the reader: %v %v", anyCall.Err, anyCall.Diagnostics)
	}
	if !strings.Contains(anyCall.CPP, "q_as_str(") || !strings.Contains(anyCall.CPP, "csv.hpp") {
		t.Fatalf("untyped path was not checked at the reader:\n%s", anyCall.CPP)
	}

	for _, tc := range []struct{ src, want string }{
		{"csv::read_file(1)\n", "expects str, got int"},
		{"csv::read_file(null)\n", "expects str, got null"},
		{"csv::read_file()\n", "expects 1"},
	} {
		bad := driver.CompileSession(".", []string{"incl 'std/csv'\n", tc.src})
		if !bad.Failed() {
			t.Fatalf("%s compiled", tc.src)
		}
		joined := ""
		for _, d := range bad.Diagnostics {
			joined += d.Message + "\n"
		}
		if !strings.Contains(joined, tc.want) {
			t.Fatalf("%s: got %s", tc.src, joined)
		}
	}
}

// stdlibRoot is src/core/stdlib, found by walking up from the test's
// working directory. go test runs the repl package from its own directory.
func stdlibRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 6; i++ {
		cand := filepath.Join(dir, "stdlib", "csv.qrk")
		if st, err := os.Stat(cand); err == nil && !st.IsDir() {
			return filepath.Join(dir, "stdlib")
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	t.Fatal("could not find stdlib/csv.qrk above the working directory")
	return ""
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
