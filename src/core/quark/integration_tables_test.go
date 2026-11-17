package main

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// Tables v0.1 (specs/tables_v0_1_spec.md): the tbl type, std/csv on the table
// reader, and queries. Programs run in a temporary directory that also holds
// the CSV files they read.

// tableCase is one program: its source, the files it reads, and either its
// expected output or a substring of its expected error.
type tableCase struct {
	name   string
	source string
	files  map[string]string
	want   string // expected stdout (trimmed), when err is empty
	err    string // expected substring of the failure
}

func runTableCases(t *testing.T, cases []tableCase) {
	t.Helper()
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			// Each case is its own process and directory; the toolchain
			// writes its precompiled header cache atomically.
			t.Parallel()
			dir := t.TempDir()
			for name, content := range tc.files {
				if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			program := filepath.Join(dir, "main.qrk")
			if err := os.WriteFile(program, []byte(tc.source), 0o644); err != nil {
				t.Fatal(err)
			}
			cmd := exec.Command(quarkExePath, "run", program)
			cmd.Dir = dir
			var out, errBuf bytes.Buffer
			cmd.Stdout, cmd.Stderr = &out, &errBuf
			runErr := cmd.Run()
			if tc.err != "" {
				if runErr == nil {
					t.Fatalf("expected failure containing %q, but it succeeded with output:\n%s", tc.err, out.String())
				}
				if combined := out.String() + errBuf.String(); !strings.Contains(combined, tc.err) {
					t.Fatalf("expected error containing %q, got:\nstdout: %s\nstderr: %s", tc.err, out.String(), errBuf.String())
				}
				return
			}
			if runErr != nil {
				t.Fatalf("run failed: %v\nstdout: %s\nstderr: %s", runErr, out.String(), errBuf.String())
			}
			got := strings.TrimSpace(normalizeNewlines(out.String()))
			if want := strings.TrimSpace(tc.want); got != want {
				t.Fatalf("unexpected output\n--- got ---\n%s\n--- want ---\n%s", got, want)
			}
		})
	}
}

const empStruct = "struct Emp:\n    id: int\n    name: str\n    dept: str\n    salary: float\n"

// TestTables_TypeRejected: tbl annotations that name no table type, and
// misuse the compiler can see.
func TestTables_TypeRejected(t *testing.T) {
	runTableCases(t, []tableCase{
		{name: "row_not_struct", source: "fn f(t: tbl[int]) -> 1\n", err: "a table's row type is a struct, not int"},
		{name: "row_unknown", source: "fn f(t: tbl[Nope]) -> 1\n", err: "unknown type 'Nope'"},
		{name: "row_non_column_field", source: "struct S:\n    xs: list\nfn f(t: tbl[S]) -> 1\n", err: "struct S can't be a table row: field 'xs' is list[any]"},
		{name: "row_struct_field", source: "struct P:\n    x: int\nstruct S:\n    p: P\nfn f(t: tbl[S]) -> 1\n", err: "struct S can't be a table row: field 'p' is P"},
		{name: "type_arg_on_list", source: "x: list[int] = list [1]\n", err: "'list' takes no type argument; element types aren't written in annotations yet"},
		{name: "type_arg_on_struct", source: empStruct + "x: Emp[int] = 1\n", err: "'Emp' takes no type argument"},
		{name: "empty_type_arg", source: "fn f(t: tbl[]) -> 1\n", err: "expected a type inside 'tbl[...]'"},
		{name: "two_type_args", source: empStruct + "fn f(t: tbl[Emp, Emp]) -> 1\n", err: "'tbl[...]' takes one type"},
		{name: "null_param", source: empStruct + "fn f(t: tbl[Emp]) -> 1\nf(null)\n", err: "argument 1 of 'f' expects tbl[Emp], got null"},
		{name: "null_bare_param", source: "fn f(t: tbl) -> 1\nf(null)\n", err: "argument 1 of 'f' expects tbl, got null"},
		{name: "null_var", source: "t: tbl = null\n", err: "cannot assign value of type 'null' to 'tbl'"},
		{name: "int_param", source: empStruct + "fn f(t: tbl[Emp]) -> 1\nf(5)\n", err: "argument 1 of 'f' expects tbl[Emp], got int"},
		{name: "struct_param", source: empStruct + "fn f(t: tbl[Emp]) -> 1\nf(Emp { id: 1, name: 'a', dept: 'b', salary: 1.0 })\n", err: "argument 1 of 'f' expects tbl[Emp], got Emp"},
		{name: "unknown_column", source: empStruct + "fn f(t: tbl[Emp]) -> t.wage\n", err: "tbl[Emp] has no column 'wage' (columns: id, name, dept, salary)"},
		{name: "column_of_bare_tbl", source: "fn f(t: tbl) -> t.salary\n", err: "the row type of this table isn't known"},
		{name: "column_is_vec", source: empStruct + "fn f(t: tbl[Emp]) int -> t.salary\n", err: "declares return type 'int' but body returns 'vec[float]'"},
		{name: "untyped_null_param", source: empStruct + "fn f(t: tbl[Emp]) -> 1\nfn id(x) -> x\nf(id(null))\n", err: "argument 't' of 'f' expects tbl[Emp], got null"},
		{name: "untyped_int_var", source: "fn id(x) -> x\nt: tbl = id(3)\n", err: "variable 't' is declared tbl but got int"},
		{name: "extern_row_param", source: "extern 'x.hpp'\n" + empStruct + "extern fn f(t: tbl[Emp]) int as 'f'\n", err: "an extern fn takes and returns a bare 'tbl', not tbl[...]"},
		{name: "extern_row_return", source: "extern 'x.hpp'\n" + empStruct + "extern fn f() tbl[Emp] as 'f'\n", err: "an extern fn takes and returns a bare 'tbl', not tbl[...]"},
	})
}

const staffCSV = "id,name,dept,salary\n1,Ann,eng,120000\n2,Bo,ops,90000.5\n3,Cy,eng,\n"

// TestTables_CSV: std/csv reads typed by the expected type (D18), the
// column mapping (D17), untyped reads checked where they reach a tbl[R]
// slot (§3.4), and writing.
func TestTables_CSV(t *testing.T) {
	files := map[string]string{"staff.csv": staffCSV}
	pre := "incl 'std/csv'\n" + empStruct
	runTableCases(t, []tableCase{
		{name: "typed_decl", files: files, source: pre + "staff: tbl[Emp] = unwrap(csv::read_file('staff.csv'))\nprint(type(staff))\nprint(staff.salary)\nprint(sum(staff.id))\nprint(staff.name)\n",
			want: "tbl[Emp]\n[120000.0, 90000.5, null]\n6\n['Ann', 'Bo', 'Cy']"},
		{name: "typed_return", files: files, source: pre + "fn load(p: str) tbl[Emp] -> unwrap(csv::read_file(p))\nprint(type(load('staff.csv')))\n", want: "tbl[Emp]"},
		{name: "typed_return_block", files: files, source: pre + "fn load(p: str) tbl[Emp] ->\n    print('loading')\n    unwrap(csv::read_file(p))\nprint(load('staff.csv').dept)\n", want: "loading\n['eng', 'ops', 'eng']"},
		{name: "typed_param", files: files, source: pre + "fn top(t: tbl[Emp]) -> t.name\nprint(top(unwrap(csv::read_file('staff.csv'))))\n", want: "['Ann', 'Bo', 'Cy']"},
		{name: "typed_field", files: files, source: pre + "struct Box:\n    t: tbl[Emp]\nb = Box { t: unwrap(csv::read_file('staff.csv')) }\nprint(type(b.t))\n", want: "tbl[Emp]"},
		{name: "typed_reassign", files: files, source: pre + "s: tbl[Emp] = unwrap(csv::read_file('staff.csv'))\ns = unwrap(csv::read_file('staff.csv'))\nprint(type(s))\n", want: "tbl[Emp]"},
		{name: "typed_pipe", files: files, source: pre + "s: tbl[Emp] = unwrap('staff.csv' | csv::read_file())\nprint(type(s))\n", want: "tbl[Emp]"},
		{name: "typed_result", files: files, source: pre + "fn load(p: str) result[tbl[Emp]] -> 1\n", err: "'result' takes no type argument"},
		{name: "typed_text", source: pre + "s: tbl[Emp] = unwrap(csv::parse_text('salary,id,name,dept\\n1,2,a,b\\n'))\nprint(s.salary)\nprint(type(s))\n", want: "[1.0]\ntbl[Emp]"},
		{name: "typed_int_file_float_field", source: pre + "s: tbl[Emp] = unwrap(csv::parse_text('id,name,dept,salary\\n1,a,b,7\\n'))\nprint(s.salary)\n", want: "[7.0]"},
		{name: "mapping", files: map[string]string{"emp.csv": "EmpID,Name,Dept,Salary ($)\n1,a,b,2.5\n"},
			source: pre + "s: tbl[Emp] = unwrap(csv::read_file('emp.csv', dict { id: 'EmpID', name: 'Name', dept: 'Dept', salary: 'Salary ($)' }))\nprint(s.salary)\nprint(csv::format_text(s))\n",
			want:   "[2.5]\nid,name,dept,salary\n1,a,b,2.5"},
		{name: "mapping_partial", files: map[string]string{"emp.csv": "EmpID,name,dept,salary\n1,a,b,2.5\n"},
			source: pre + "s: tbl[Emp] = unwrap(csv::read_file('emp.csv', dict { id: 'EmpID' }))\nprint(s.id)\n", want: "[1]"},
		{name: "mapping_untyped", files: map[string]string{"emp.csv": "EmpID,name\n1,a\n"},
			source: "incl 'std/csv'\nt = unwrap(csv::read_file('emp.csv', dict { id: 'EmpID' }))\nprint(type(t))\n", want: "tbl[{id: int, name: str}]"},
		{name: "untyped_then_typed", files: files, source: pre + "u = unwrap(csv::read_file('staff.csv'))\nprint(type(u))\nv: tbl[Emp] = u\nprint(type(v))\nprint(v.salary)\n",
			want: "tbl[{id: int, name: str, dept: str, salary: float}]\ntbl[Emp]\n[120000.0, 90000.5, null]"},
		{name: "untyped_reordered_widened", source: pre + "u = unwrap(csv::parse_text('salary,name,id,dept\\n5,a,1,b\\n'))\nprint(type(u))\nv: tbl[Emp] = u\nprint(type(v))\nprint(v.salary)\nprint(csv::format_text(v))\n",
			want: "tbl[{salary: int, name: str, id: int, dept: str}]\ntbl[Emp]\n[5.0]\nid,name,dept,salary\n1,a,b,5.0"},
		{name: "untyped_when", files: files, source: pre + "fn top(t: tbl[Emp]) -> t.name\nwhen csv::read_file('staff.csv'):\n    ok t -> print(top(t))\n    err e -> print(e)\n", want: "['Ann', 'Bo', 'Cy']"},
		{name: "untyped_through_any", files: files, source: pre + "fn id(x) -> x\ns: tbl[Emp] = unwrap(csv::read_file('staff.csv'))\nv: tbl[Emp] = id(s)\nprint(type(v))\n", want: "tbl[Emp]"},
		{name: "column_through_any", files: files, source: pre + "fn col(x) -> x.salary\nprint(col(unwrap(csv::read_file('staff.csv'))))\n", want: "[120000.0, 90000.5, null]"},
		{name: "list_of_tables", files: files, source: pre + "s: tbl[Emp] = unwrap(csv::read_file('staff.csv'))\nts = list [s, s]\nprint(len(ts))\nprint(type(ts[1]))\n", want: "2\ntbl[Emp]"},
		{name: "write_read_back", files: files, source: pre + "s: tbl[Emp] = unwrap(csv::read_file('staff.csv'))\nprint(unwrap(csv::write_file(s, 'out.tsv')))\nb: tbl[Emp] = unwrap(csv::read_file('out.tsv'))\nprint(b.salary)\nprint(csv::format_text(b) == csv::format_text(s))\n",
			want: "null\n[120000.0, 90000.5, null]\ntrue"},
		{name: "tsv_file", files: map[string]string{"t.tsv": "a\tb\n1\tx y\n"}, source: "incl 'std/csv'\nprint(type(unwrap(csv::read_file('t.tsv'))))\n", want: "tbl[{a: int, b: str}]"},
		{name: "function_value", files: files, source: "incl 'std/csv'\nf = csv::read_file\nprint(type(unwrap(f('staff.csv'))))\n", want: "tbl[{id: int, name: str, dept: str, salary: float}]"},
		{name: "mapping_value_through_fn_value", files: files, source: "incl 'std/csv'\nf = csv::read_file\nprint(f('staff.csv', dict { id: 5 }))\n",
			want: "err 'csv: the column mapping maps \\'id\\' to a value of type int; map each field to a header name (a str)'"},
		{name: "native_bare_tbl_param", files: files, source: pre + "fn kind(t: tbl) str -> type(t)\nprint(kind(unwrap(csv::read_file('staff.csv'))))\ns: tbl[Emp] = unwrap(csv::read_file('staff.csv'))\nprint(kind(s))\n",
			want: "tbl[{id: int, name: str, dept: str, salary: float}]\ntbl[Emp]"},
		{name: "write_err", files: files, source: pre + "s: tbl[Emp] = unwrap(csv::read_file('staff.csv'))\nprint(is_err(csv::write_file(s, 'no/such/dir/x.csv')))\n", want: "true"},
	})
}

// TestTables_CSVRejected: reads that can't give a table of the expected
// type, at compile time where the compiler can tell, at runtime otherwise.
func TestTables_CSVRejected(t *testing.T) {
	files := map[string]string{"staff.csv": staffCSV}
	pre := "incl 'std/csv'\n" + empStruct
	runTableCases(t, []tableCase{
		{name: "bad_cell", files: map[string]string{"s.csv": "id,name,dept,salary\n1,a,b,1\n2,c,d,12k\n"},
			source: pre + "s: tbl[Emp] = unwrap(csv::read_file('s.csv'))\n", err: "unwrap on err: s.csv:3: column 'salary': '12k' is not a float"},
		{name: "missing_column", files: map[string]string{"s.csv": "EmpID,name,dept,salary\n1,a,b,1\n"},
			source: pre + "s: tbl[Emp] = unwrap(csv::read_file('s.csv'))\n", err: "s.csv: no column named 'id' in the header (columns: EmpID, name, dept, salary); map field 'id' to one of them with dict { id: '...' }"},
		{name: "mapped_column_missing", files: map[string]string{"s.csv": "ID,name,dept,salary\n1,a,b,1\n"},
			source: pre + "s: tbl[Emp] = unwrap(csv::read_file('s.csv', dict { id: 'EmpID' }))\n", err: "no column named 'EmpID' in the header (columns: ID, name, dept, salary); field 'id' is mapped to it"},
		{name: "mapping_not_literal", files: files, source: pre + "m = dict { id: 'EmpID' }\ns: tbl[Emp] = unwrap(csv::read_file('staff.csv', m))\n", err: "the column mapping must be a dict literal"},
		{name: "mapping_unknown_field", files: files, source: pre + "s: tbl[Emp] = unwrap(csv::read_file('staff.csv', dict { idd: 'EmpID' }))\n", err: "the column mapping names 'idd', but Emp has no field 'idd' (fields: id, name, dept, salary)"},
		{name: "mapping_value_not_literal", files: files, source: pre + "h = 'EmpID'\ns: tbl[Emp] = unwrap(csv::read_file('staff.csv', dict { id: h }))\n", err: "the column mapping maps field 'id' to a header name, which must be a string literal"},
		{name: "mapping_same_header", files: files, source: pre + "s: tbl[Emp] = unwrap(csv::read_file('staff.csv', dict { id: 'X', name: 'X' }))\n", err: "fields 'id' and 'name' both read the header column 'X'"},
		{name: "mapping_untyped_not_literal", files: files, source: "incl 'std/csv'\nm = dict { id: 'EmpID' }\nt = csv::read_file('staff.csv', m)\n", err: "the column mapping must be a dict literal"},
		{name: "untyped_wrong_shape", source: pre + "u = unwrap(csv::parse_text('id,name\\n1,a\\n'))\nv: tbl[Emp] = u\n", err: "variable 'v' is declared tbl[Emp] but got tbl[{id: int, name: str}]"},
		{name: "untyped_extra_column", source: pre + "u = unwrap(csv::parse_text('id,name,dept,salary,x\\n1,a,b,1,2\\n'))\nv: tbl[Emp] = u\n", err: "variable 'v' is declared tbl[Emp] but got tbl[{id: int, name: str, dept: str, salary: int, x: int}]"},
		{name: "untyped_wrong_column_type", source: pre + "u = unwrap(csv::parse_text('id,name,dept,salary\\n1,2,b,1\\n'))\nv: tbl[Emp] = u\n", err: "variable 'v' is declared tbl[Emp] but got tbl[{id: int, name: int, dept: str, salary: int}]"},
		{name: "untyped_param", source: pre + "fn top(t: tbl[Emp]) -> t.name\nu = unwrap(csv::parse_text('id\\n1\\n'))\nprint(top(u))\n", err: "argument 't' of 'top' expects tbl[Emp], got tbl[{id: int}]"},
		{name: "param_types_the_read", source: pre + "fn top(t: tbl[Emp]) -> t.name\nprint(top(unwrap(csv::parse_text('id\\n1\\n'))))\n", err: "text: no column named 'name' in the header (columns: id)"},
		{name: "untyped_return", source: pre + "fn load(s: str) tbl[Emp] ->\n    fn id(x) -> x\n    id(unwrap(csv::parse_text(s)))\nload('id\\n1\\n')\n", err: "function 'load' declares return type tbl[Emp] but returned tbl[{id: int}]"},
		{name: "named_rows_differ", source: pre + "struct Emp2:\n    id: int\n    name: str\n    dept: str\n    salary: float\ns: tbl[Emp] = unwrap(csv::parse_text('id,name,dept,salary\\n1,a,b,1\\n'))\nv: tbl[Emp2] = s\n", err: "cannot assign value of type 'tbl[Emp]' to 'tbl[Emp2]': Emp and Emp2 are different structs"},
		{name: "named_rows_differ_runtime", source: pre + "struct Emp2:\n    id: int\n    name: str\n    dept: str\n    salary: float\nfn id(x) -> x\ns: tbl[Emp] = unwrap(csv::parse_text('id,name,dept,salary\\n1,a,b,1\\n'))\nv: tbl[Emp2] = id(s)\n", err: "variable 'v' is declared tbl[Emp2] but got tbl[Emp]"},
		{name: "latin1_cell", files: map[string]string{"l.csv": "id,name,dept,salary\n1,Jos\xe9,b,1\n"},
			source: pre + "s: tbl[Emp] = unwrap(csv::read_file('l.csv'))\n", err: "l.csv:2: column 'name' is not valid UTF-8 (the file may be Latin-1"},
		{name: "nul_cell", files: map[string]string{"n.csv": "id,name,dept,salary\n1,a\x00b,c,1\n"},
			source: pre + "s: tbl[Emp] = unwrap(csv::read_file('n.csv'))\n", err: "n.csv:2: column 'name' contains a NUL byte"},
		{name: "list_push_other_row", files: files, source: pre + "s: tbl[Emp] = unwrap(csv::read_file('staff.csv'))\nts = list [s]\nts.push(unwrap(csv::read_file('staff.csv')))\n", err: "push() can't add a tbl[{id: int, name: str, dept: str, salary: float}] to a list of tbl[Emp]"},
		{name: "list_push_other_row_runtime", files: files, source: pre + "fn id(x) -> x\ns: tbl[Emp] = unwrap(csv::read_file('staff.csv'))\nts = list [s]\nts.push(id(unwrap(csv::read_file('staff.csv'))))\n", err: "can't add a tbl[{id: int, name: str, dept: str, salary: float}] to a list of tbl[Emp]"},
		{name: "read_arg_type", source: "incl 'std/csv'\nt = csv::read_file(5)\n", err: "argument 1 of 'csv::read_file' expects str, got int"},
		{name: "format_arg_type", source: "incl 'std/csv'\nprint(csv::format_text(5))\n", err: "argument 1 of 'csv::format_text' expects tbl, got int"},
		{name: "format_through_any", source: "incl 'std/csv'\nfn id(x) -> x\nprint(csv::format_text(id(5)))\n", err: "expected tbl, got int"},
		{name: "unwrap_err_no_column_access", files: files, source: "incl 'std/csv'\nt = unwrap(csv::read_file('staff.csv'))\nprint(t.salary)\n", err: "the row type of this table isn't known"},
	})
}

// queryCSV has a missing salary (Cy) and a missing dept (Ed).
const queryCSV = "id,name,dept,salary\n1,Ann,eng,120000\n2,Bo,ops,90000.5\n3,Cy,eng,\n4,Di,ops,150000\n5,Ed,,80000\n"

// queryPre reads queryCSV as staff: tbl[Emp]; show renders a table.
const queryPre = "incl 'std/csv'\n" + empStruct + "staff: tbl[Emp] = unwrap(csv::read_file('staff.csv'))\nshow = csv::format_text\n"

// TestTables_Query: each verb's results, missing values (§5: strict
// expressions, three-valued and/or/!, D22), the schema through steps (§3.1),
// mutate's column order (D23), and queries in functions and modules.
func TestTables_Query(t *testing.T) {
	files := map[string]string{"staff.csv": queryCSV}
	q := func(name, body, want string) tableCase {
		return tableCase{name: name, files: files, source: queryPre + body, want: want}
	}
	runTableCases(t, []tableCase{
		q("filter", "print(show(staff | e -> filter(e.salary >= 100_000)))\n",
			"id,name,dept,salary\n1,Ann,eng,120000.0\n4,Di,ops,150000.0"),
		q("filter_or_missing", "print(show(staff | e -> filter(e.salary >= 100_000 or e.dept == 'eng')))\n",
			"id,name,dept,salary\n1,Ann,eng,120000.0\n3,Cy,eng,\n4,Di,ops,150000.0"),
		q("filter_and_missing", "print(show(staff | e -> filter(e.salary < 100_000 and e.dept == 'ops')))\n",
			"id,name,dept,salary\n2,Bo,ops,90000.5"),
		q("filter_and_false_missing", "print(show(staff | e -> filter(!(e.salary > 0.0 and e.id > 2))))\n",
			"id,name,dept,salary\n1,Ann,eng,120000.0\n2,Bo,ops,90000.5"),
		q("filter_not_missing", "print(show(staff | e -> filter(!(e.dept == 'eng'))))\n",
			"id,name,dept,salary\n2,Bo,ops,90000.5\n4,Di,ops,150000.0"),
		q("filter_method", "print(show(staff | e -> filter(e.name.startswith('D') or e.name.endswith('n'))))\n",
			"id,name,dept,salary\n1,Ann,eng,120000.0\n4,Di,ops,150000.0"),
		q("filter_outer_var", "limit = 100000\nprint(show(staff | e -> filter(e.salary > limit)))\n",
			"id,name,dept,salary\n1,Ann,eng,120000.0\n4,Di,ops,150000.0"),
		q("filter_none", "none = staff | e -> filter(e.id > 99)\nprint(show(none))\nprint(show(none | e -> summarize(n = count(), s = sum(e.salary), m = mean(e.salary))))\nprint(show(none | e -> group_by(e.dept) | summarize(n = count())))\n",
			"id,name,dept,salary\n\nn,s,m\n0,0.0,\n\ndept,n"),
		q("mutate_in_order", "print(show(staff | e -> mutate(bonus = e.salary * 0.1, total = e.salary + e.bonus, up = e.name.upper())))\n",
			"id,name,dept,salary,bonus,total,up\n1,Ann,eng,120000.0,12000.0,132000.0,ANN\n2,Bo,ops,90000.5,9000.050000000001,99000.55,BO\n3,Cy,eng,,,,CY\n4,Di,ops,150000.0,15000.0,165000.0,DI\n5,Ed,,80000.0,8000.0,88000.0,ED"),
		q("mutate_replace_type", "r = staff | e -> mutate(salary = to_str(e.id) + '!')\nprint(type(r))\nprint(r.salary)\n",
			"tbl[{id: int, name: str, dept: str, salary: str}]\n['1!', '2!', '3!', '4!', '5!']"),
		q("mutate_replace_then_read", "print((staff | e -> mutate(id = e.id * 10, x = e.id + 1)).x)\n", "[11, 21, 31, 41, 51]"),
		q("mutate_ternary", "print((staff | e -> mutate(band = 'high' if e.salary > 100000.0 else 'low')).band)\n", "['high', 'low', null, 'high', 'low']"),
		q("mutate_bool_column", "r = staff | e -> mutate(rich = e.salary > 100000.0)\nprint(r.rich)\nprint(show(r | e -> summarize(n = sum(e.rich), share = mean(e.rich))))\n",
			"[true, false, null, true, false]\nn,share\n2,0.5"),
		{name: "mutate_calls_skip_missing", files: map[string]string{"staff.csv": queryCSV, "tick.hpp": rowSafeTickH},
			source: queryPre + "extern 'tick.hpp'\nextern rowsafe fn tick() int as 'q_rowsafe_tick'\nextern rowsafe fn ticks() int as 'q_rowsafe_ticks'\nfn note(s: float) float ->\n    tick()\n    s / 2.0\nr = staff | e -> mutate(half = note(e.salary))\nprint(r.half)\nprint(ticks())\n",
			want:   "[60000.0, 45000.25, null, 75000.0, 40000.0]\n4"},
		q("mutate_user_fn", "fn band(s: float) str -> 'high' if s > 100000.0 else 'low'\nprint((staff | e -> mutate(b = band(e.salary))).b)\n", "['high', 'low', null, 'high', 'low']"),
		q("group_summarize", "print(show(staff | e -> group_by(e.dept) | summarize(avg = mean(e.salary), top = max(e.salary), n = count(), k = count(e.salary), s = sum(e.id))))\n",
			"dept,avg,top,n,k,s\neng,120000.0,120000.0,2,1,4\nops,120000.25,150000.0,2,2,6\n,80000.0,80000.0,1,1,5"),
		q("group_two_keys", "print(show(staff | e -> mutate(hi = e.id > 2) | group_by(e.dept, e.hi) | summarize(n = count())))\n",
			"dept,hi,n\neng,false,1\neng,true,1\nops,false,1\nops,true,1\n,true,1"),
		q("summarize_whole", "print(show(staff | e -> summarize(n = count(), lo = min(e.name), hi = max(e.salary))))\n", "n,lo,hi\n5,Ann,150000.0"),
		q("sort_desc", "print((staff | e -> sort(desc(e.salary))).name)\n", "['Di', 'Ann', 'Bo', 'Ed', 'Cy']"),
		q("sort_keys_head", "print((staff | e -> sort(e.dept, desc(e.id)) | head(3)).name)\n", "['Cy', 'Ann', 'Di']"),
		q("head_tail", "n = 1\nprint((staff | e -> head(n + 1)).id)\nprint((staff | e -> tail(2)).id)\nprint((staff | e -> head()).id)\nprint((staff | e -> head(0)).id)\n",
			"[1, 2]\n[4, 5]\n[1, 2, 3, 4, 5]\n[]"),
		q("named_result", "struct DeptStats:\n    dept: str\n    avg: float\n    n: int\nds: tbl[DeptStats] = staff | e -> group_by(e.dept) | summarize(n = count(), avg = mean(e.salary))\nprint(type(ds))\nprint(show(ds))\n",
			"tbl[DeptStats]\ndept,avg,n\neng,120000.0,2\nops,120000.25,2\n,80000.0,1"),
		q("named_result_widened", "struct C:\n    dept: str\n    n: float\nc: tbl[C] = staff | e -> group_by(e.dept) | summarize(n = count())\nprint(c.n)\n", "[2.0, 2.0, 1.0]"),
		q("same_shape_back_to_struct", "back: tbl[Emp] = staff | e -> mutate(salary = e.salary * 2.0)\nprint(type(back))\nprint(back.salary)\n", "tbl[Emp]\n[240000.0, 180001.0, null, 300000.0, 160000.0]"),
		q("function", "fn top(t: tbl[Emp], k: int) tbl[Emp] -> t | e -> sort(desc(e.salary)) | head(k)\nprint(top(staff, 2).name)\nfn over(t: tbl[Emp], m: float) -> t | e -> filter(e.salary > m)\nprint(over(staff, 100000.0).name)\n",
			"['Di', 'Ann']\n['Ann', 'Di']"),
		q("closure", "m = 100000.0\nf = fn(t: tbl[Emp]) -> t | e -> filter(e.salary > m)\nprint(f(staff).id)\n", "[1, 4]"),
		q("row_name_shadows", "e = 5\nr = staff | e -> filter(e.id > 3)\nprint(e)\nprint(r.id)\n", "5\n[4, 5]"),
		q("parenthesized_then_pipe", "(staff | e -> head(1)) | show() | print()\n", "id,name,dept,salary\n1,Ann,eng,120000.0"),
		q("block_form", "r = staff | e ->\n    filter(e.salary > 0.0)\n    | mutate(k = e.salary / 1000.0)\n    | sort(desc(e.k))\nprint(r.k)\n", "[150.0, 120.0, 90.0005, 80.0]"),
		q("continuation_lines", "r = staff | e -> filter(e.salary > 0.0)\n    | sort(e.salary)\n    | head(2)\nprint(r.name)\n", "['Ed', 'Bo']"),
		q("query_of_query", "a = staff | e -> mutate(k = e.id * 2)\nb = a | x -> filter(x.k > 4) | mutate(j = x.k + 1)\nprint(b.j)\nprint(type(b))\n",
			"[7, 9, 11]\ntbl[{id: int, name: str, dept: str, salary: float, k: int, j: int}]"),
		q("anonymous_reassign", "x = staff | e -> mutate(k = e.id * 2)\nx = staff | e -> mutate(k = 7)\nprint(x.k.to_list()[0])\nz = staff | e -> mutate(d = 1) | mutate(k = 3)\nw = staff | e -> mutate(k = 3) | mutate(d = 1)\nw = z\nprint(type(w))\n",
			"7\ntbl[{id: int, name: str, dept: str, salary: float, k: int, d: int}]"),
		q("nan_in_filter", "r = staff | e -> mutate(z = to_float('nan') + e.salary) | filter(e.z == e.z or e.z > 0.0)\nprint(len(r.id))\n", "0"),
		q("select", "print(show(staff | e -> select(e.salary, e.name)))\nprint(type(staff | e -> select(e.name, e.salary)))\n",
			"salary,name\n120000.0,Ann\n90000.5,Bo\n,Cy\n150000.0,Di\n80000.0,Ed\n\ntbl[{name: str, salary: float}]"),
		q("select_into_struct", "struct Pay:\n    name: str\n    salary: float\np: tbl[Pay] = staff | e -> select(e.salary, e.name)\nprint(type(p))\nprint(p.name)\n",
			"tbl[Pay]\n['Ann', 'Bo', 'Cy', 'Di', 'Ed']"),
		q("deselect", "print(show(staff | e -> deselect(e.id, e.dept)))\nprint(type(staff | e -> deselect(e.id, e.dept)))\n",
			"name,salary\nAnn,120000.0\nBo,90000.5\nCy,\nDi,150000.0\nEd,80000.0\n\ntbl[{name: str, salary: float}]"),
		q("rename_swap", "r = staff | e -> rename(id = e.name, name = e.id)\nprint(type(r))\nprint(show(r | e -> head(1)))\n",
			"tbl[{name: int, id: str, dept: str, salary: float}]\nname,id,dept,salary\n1,Ann,eng,120000.0"),
		q("distinct_first", "print(show(staff | e -> distinct(e.dept)))\n",
			"id,name,dept,salary\n1,Ann,eng,120000.0\n2,Bo,ops,90000.5\n5,Ed,,80000.0"),
		q("distinct_all", "print((staff | e -> distinct()).id)\nprint((staff | e -> filter(e.id > 99) | distinct(e.dept)).id)\n",
			"[1, 2, 3, 4, 5]\n[]"),
		q("n_distinct", "print(show(staff | e -> summarize(depts = n_distinct(e.dept), n = count())))\nprint(show(staff | e -> group_by(e.dept) | summarize(people = n_distinct(e.name))))\nprint(show(staff | e -> filter(e.id > 99) | summarize(k = n_distinct(e.dept))))\n",
			"depts,n\n3,5\n\ndept,people\neng,2\nops,2\n,1\n\nk\n0"),
		q("n_distinct_nan", "r = staff | e -> mutate(z = to_float('nan') if e.id == 1 else e.salary)\nprint(show(r | e -> summarize(k = n_distinct(e.z))))\nprint((r | e -> summarize(lo = min(e.z))).lo)\n",
			"k\n5\n\n[80000.0]"),
		q("distinct_nan_and_missing", "r = staff | e -> mutate(z = to_float('nan') if e.id <= 2 else e.salary)\nprint((r | e -> distinct(e.z)).id)\nprint((r | e -> summarize(k = n_distinct(e.z))).k)\n",
			"[1, 3, 4, 5]\n[4]"),
		q("deselect_into_struct", "struct Who:\n    name: str\n    salary: float\nw: tbl[Who] = staff | e -> deselect(e.id, e.dept)\nprint(type(w))\nprint(w.salary)\n",
			"tbl[Who]\n[120000.0, 90000.5, null, 150000.0, 80000.0]"),
		{name: "select_in_module", files: map[string]string{"staff.csv": queryCSV, "lib.qrk": "struct Emp:\n    id: int\n    name: str\n    dept: str\n    salary: float\nmodule lib:\n    fn names(t: tbl[Emp]) -> t | e -> select(e.name, e.dept)\n"},
			source: "incl 'std/csv'\nincl './lib'\nfn load(p: str) tbl[lib::Emp] -> unwrap(csv::read_file(p))\nprint(lib::names(load('staff.csv')).name)\n",
			want:   "['Ann', 'Bo', 'Cy', 'Di', 'Ed']"},
		{name: "in_module", files: map[string]string{"staff.csv": queryCSV, "lib.qrk": "struct Emp:\n    id: int\n    name: str\n    dept: str\n    salary: float\nmodule lib:\n    fn rich(t: tbl[Emp]) -> t | e -> filter(e.salary > 100000.0)\n"},
			source: "incl 'std/csv'\nincl './lib'\ne = 'main e'\nfn load(p: str) tbl[lib::Emp] -> unwrap(csv::read_file(p))\nprint(lib::rich(load('staff.csv')).name)\nprint(e)\n",
			want:   "['Ann', 'Di']\nmain e"},
	})
}

// TestTables_QueryRejected: misuse of queries, at compile time where the
// compiler can tell (most of it: the row type is known), at runtime
// otherwise.
func TestTables_QueryRejected(t *testing.T) {
	files := map[string]string{"staff.csv": queryCSV}
	q := func(name, body, err string) tableCase {
		return tableCase{name: name, files: files, source: queryPre + body, err: err}
	}
	runTableCases(t, []tableCase{
		// The source must be a table whose row is known (D11).
		{name: "bare_tbl", files: files, source: "incl 'std/csv'\nt = unwrap(csv::read_file('staff.csv'))\nr = t | e -> head(1)\n", err: "the row type of this table isn't known, so the query's columns can't be checked"},
		{name: "untyped_param", source: "fn f(t) -> t | e -> head(1)\n", err: "a query needs a table whose row type is known, and this value's type isn't; annotate it (t: tbl[Employee])"},
		{name: "not_a_table", source: "r = 5 | e -> head(1)\n", err: "a query runs on a table, not on int"},
		{name: "hoisted_unannotated", source: "incl 'std/csv'\n" + empStruct + "r = load() | e -> head(1)\nfn load() -> unwrap(csv::read_file('staff.csv'))\n", err: "a query needs a table whose row type is known"},
		// Verbs.
		q("unknown_verb", "r = staff | e -> pick(e.id)\n", "'pick' is not a query step (verbs: filter, mutate, select, deselect, rename, distinct, drop_na, sort, group_by, summarize, count, head, tail)"),
		q("not_a_verb_after_query", "staff | e -> head(1) | show()\n", "'show' is not a query step"),
		q("qualified_after_query", "staff | e -> head(1) | csv::format_text()\n", "expected a query step, verb(...)"),
		q("second_row_name", "r = staff | e -> head(1) | x -> head(1)\n", "a query names its row once ('e ->'); write the next step as '| verb(...)'"),
		q("trailing_pipe", "r = staff | e -> head(1) |\n", "a line can't end with '|'"),
		// Columns and the row name.
		q("unknown_column", "r = staff | e -> filter(e.wage > 1)\n", "Emp has no column 'wage' (columns: id, name, dept, salary)"),
		q("unknown_column_after_mutate", "r = staff | e -> mutate(k = 1) | filter(e.kk > 1)\n", "the row {id: int, name: str, dept: str, salary: float, k: int} has no column 'kk'"),
		q("column_gone_after_summarize", "r = staff | e -> summarize(n = count()) | filter(e.id > 1)\n", "the row {n: int} has no column 'id' (columns: n)"),
		q("row_as_value", "fn f(x) -> 1\nr = staff | e -> filter(f(e) > 0)\n", "'e' is the query's row: read its columns as e.col"),
		q("row_in_lambda", "r = staff | e -> filter(any(list [1].map(fn(x) -> x > e.id)))\n", "a function written inside a verb can't read the row ('e.id')"),
		q("nested_query", "r = staff | e -> filter(len((staff | x -> head(1)).id) > 0)\n", "a query can't run inside a verb of another query"),
		// filter.
		q("filter_not_bool", "r = staff | e -> filter(e.salary)\n", "filter's condition must be a bool, got float"),
		q("filter_two_conditions", "r = staff | e -> filter(e.id > 1, e.id < 3)\n", "filter takes one condition, got 2; join conditions with 'and'"),
		q("filter_named", "r = staff | e -> filter(keep = e.id > 1)\n", "filter takes no named arguments ('keep = ...')"),
		q("and_not_bool", "r = staff | e -> filter(e.name and e.id > 1)\n", "in a query, 'and' takes bools, got str"),
		q("not_not_bool", "r = staff | e -> filter(!e.id)\n", "in a query, '!' takes bools, got int"),
		q("ternary_cond_not_bool", "r = staff | e -> mutate(x = 1 if e.id else 2)\n", "in a query, a condition (x if c else y) takes bools, got int"),
		// mutate.
		q("mutate_positional", "r = staff | e -> mutate(e.id + 1)\n", "mutate's columns are written name = expression"),
		q("mutate_twice", "r = staff | e -> mutate(k = 1, k = 2)\n", "mutate defines column 'k' twice"),
		q("mutate_list_column", "r = staff | e -> mutate(k = list [e.id])\n", "column 'k' would be list[int]; table columns are int, float, bool, str or date"),
		q("mutate_unknown_type", "fn f(x) -> x\nr = staff | e -> mutate(k = f(e.id))\n", "the type of column 'k' isn't known"),
		q("mutate_null", "r = staff | e -> mutate(k = null)\n", "column 'k' would be null"),
		q("mutate_overflow", "r = staff | e -> mutate(k = e.id * 9223372036854775807)\n", "integer overflow in '*'"),
		// sort and group_by.
		q("sort_expression", "r = staff | e -> sort(e.id * 2)\n", "sort keys are columns (e.col); to use a computed value, add it with mutate first"),
		q("sort_empty", "r = staff | e -> sort()\n", "sort needs at least one column"),
		q("group_expression", "r = staff | e -> group_by(e.id > 1) | summarize(n = count())\n", "a computed group key needs a name"),
		q("group_at_end", "g = staff | e -> group_by(e.dept)\n", "group_by must be followed by filter, mutate or summarize in the same query"),
		q("group_key_twice", "r = staff | e -> group_by(e.dept, e.dept) | summarize(n = count())\n", "group_by names column 'dept' twice"),
		// summarize (D24).
		q("summarize_no_name", "r = staff | e -> summarize(count())\n", "give the summarize column a name: name = count(...)"),
		q("summarize_not_aggregate", "r = staff | e -> summarize(n = e.id)\n", "'e.id' has a value per row; use it in an aggregate (mean(e.id)) or group by it"),
		q("summarize_unknown_aggregate", "r = staff | e -> summarize(m = median(e.salary))\n", "undefined identifier 'median'"),
		q("select_expr", "r = staff | e -> select(e.salary * 1.1)\n", "select keeps columns (e.col); drop columns with deselect, compute a column with mutate, or rename it with rename"),
		q("select_minus", "r = staff | e -> select(-e.id)\n", "drop columns with deselect"),
		q("select_rename_form", "r = staff | e -> select(pay = e.salary)\n", "select takes no named arguments"),
		q("select_empty", "r = staff | e -> select()\n", "select needs at least one column"),
		q("select_twice", "r = staff | e -> select(e.id, e.id)\n", "select names column 'id' twice"),
		q("select_unknown", "r = staff | e -> select(e.wage)\n", "Emp has no column 'wage' (columns: id, name, dept, salary)"),
		q("select_after_group", "r = staff | e -> group_by(e.dept) | select(e.dept)\n", "group_by applies to the next step, which must be filter, mutate or summarize (got select)"),
		q("select_bare", "t = unwrap(csv::read_file('staff.csv'))\nr = t | e -> select(e.id)\n", "the row type of this table isn't known"),
		q("select_original_struct", "r: tbl[Emp] = staff | e -> select(e.name, e.salary)\n", "Emp has a field 'id', but the table has no column 'id'"),
		q("deselect_expr", "r = staff | e -> deselect(e.id + 1)\n", "deselect drops columns (e.col)"),
		q("deselect_minus", "r = staff | e -> deselect(-e.id)\n", "deselect drops columns (e.col)"),
		q("deselect_all", "r = staff | e -> deselect(e.id, e.name, e.dept, e.salary)\n", "deselect would remove every column"),
		q("deselect_empty", "r = staff | e -> deselect()\n", "deselect needs at least one column"),
		q("deselect_twice", "r = staff | e -> deselect(e.id, e.id)\n", "deselect names column 'id' twice"),
		q("deselect_after_group", "r = staff | e -> group_by(e.dept) | deselect(e.dept)\n", "got deselect"),
		q("rename_expr", "r = staff | e -> rename(pay = e.salary * 1.1)\n", "rename takes a column (pay = e.salary)"),
		q("rename_collide", "r = staff | e -> rename(name = e.id)\n", "rename would give two columns named 'name'"),
		q("rename_same_new", "r = staff | e -> rename(a = e.id, a = e.name)\n", "rename would give two columns named 'a'"),
		q("rename_column_twice", "r = staff | e -> rename(a = e.id, b = e.id)\n", "rename names column 'id' twice"),
		q("rename_empty", "r = staff | e -> rename()\n", "rename needs at least one column"),
		q("rename_positional", "r = staff | e -> rename(e.id)\n", "rename's columns are written name = e.col"),
		q("distinct_expr", "r = staff | e -> distinct(e.salary * 1.1)\n", "distinct takes columns (e.col); compute the value with mutate first"),
		q("distinct_twice", "r = staff | e -> distinct(e.dept, e.dept)\n", "distinct names column 'dept' twice"),
		q("distinct_after_group", "r = staff | e -> group_by(e.dept) | distinct(e.dept)\n", "got distinct"),
		q("n_distinct_outside", "print(n_distinct(staff.dept))\n", "undefined identifier 'n_distinct'"),
		q("n_distinct_none", "r = staff | e -> summarize(k = n_distinct())\n", "n_distinct takes one expression of the row"),
		q("n_distinct_two", "r = staff | e -> summarize(k = n_distinct(e.dept, e.name))\n", "n_distinct takes one expression of the row"),
		q("agg_in_agg", "r = staff | e -> summarize(s = sum(e.salary - mean(e.salary)))\n", "an aggregate can't contain another aggregate"),
		q("agg_in_sort", "r = staff | e -> sort(e.salary - mean(e.salary))\n", "mean is an aggregate here, and aggregates are for filter, mutate and summarize"),
		q("count_not_of_row", "r = staff | e -> filter(count(1) > 0)\n", "count's argument must be an expression of the row"),
		q("summarize_sum_str", "r = staff | e -> summarize(s = sum(e.name))\n", "sum needs a number or bool column; column 'name' is str"),
		q("summarize_mean_str", "r = staff | e -> summarize(s = mean(e.name))\n", "mean needs a number or bool column; column 'name' is str"),
		q("summarize_key_name", "r = staff | e -> group_by(e.dept) | summarize(dept = count())\n", "summarize column 'dept' has the name of a group key"),
		q("summarize_twice", "r = staff | e -> summarize(n = count(), n = sum(e.id))\n", "summarize defines column 'n' twice"),
		q("count_two_args", "r = staff | e -> summarize(n = count(e.id, e.name))\n", "count() takes no arguments, or one expression of the row"),
		q("sum_overflow", "r = staff | e -> mutate(k = 9223372036854775807) | summarize(s = sum(e.k))\n", "integer overflow in 'sum'"),
		// head and tail.
		q("head_str", "r = staff | e -> head('3')\n", "head's row count must be an int, got str"),
		q("head_reads_row", "r = staff | e -> head(e.id)\n", "head's row count is one number for the whole table; it can't read the row ('e.id')"),
		q("head_negative", "n = -1\nr = staff | e -> head(n)\n", "head(n) needs n >= 0, got -1"),
		q("tail_two_args", "r = staff | e -> tail(1, 2)\n", "tail takes at most one argument"),
		// The result's type (§3.2).
		q("result_missing_column", "struct S:\n    dept: str\n    n: int\nr: tbl[S] = staff | e -> group_by(e.dept) | summarize(n = count(), avg = mean(e.salary))\n",
			"cannot assign value of type 'tbl[{dept: str, n: int, avg: float}]' to 'tbl[S]': the table has a column 'avg', but S has no field 'avg'"),
		q("result_column_type", "struct S:\n    dept: str\n    n: str\nr: tbl[S] = staff | e -> group_by(e.dept) | summarize(n = count())\n",
			"column 'n' is int, but S declares it str"),
		q("result_field_missing", "struct S:\n    dept: str\n    n: int\n    x: int\nr: tbl[S] = staff | e -> group_by(e.dept) | summarize(n = count())\n",
			"S has a field 'x', but the table has no column 'x'"),
		q("anonymous_reassign_type", "x = staff | e -> mutate(k = 1)\nx = staff | e -> mutate(k = 1.5)\n", "column 'k' is float, but the row {id: int, name: str, dept: str, salary: float, k: int} declares it int"),
		q("result_param", "struct S:\n    n: int\nfn f(t: tbl[S]) -> 1\nf(staff | e -> summarize(m = count()))\n", "argument 1 of 'f' expects tbl[S], got tbl[{m: int}]: the table has a column 'm', but S has no field 'm'"),
	})
}

// TestTables_Inspect: printing a table (§6), and the head, tail and
// describe methods (D25).
func TestTables_Inspect(t *testing.T) {
	files := map[string]string{"staff.csv": queryCSV, "long.csv": "n,s\n1,a\n2,b\n3,c\n4,d\n5,e\n6,f\n7,g\n8,h\n9,i\n10,j\n11,k\n12,l\n"}
	q := func(name, body, want string) tableCase {
		return tableCase{name: name, files: files, source: queryPre + body, want: want}
	}
	staffBlock := "tbl[Emp]: 5 rows\n id  name  dept    salary\nint  str   str      float\n  1  Ann   eng   120000.0\n  2  Bo    ops    90000.5\n  3  Cy    eng       null\n  4  Di    ops   150000.0\n  5  Ed    null   80000.0"
	runTableCases(t, []tableCase{
		q("print", "print(staff)\n", staffBlock),
		q("to_str", "s = to_str(staff)\nprint(s == to_str(staff))\nprint(len(s.split('\\n')))\n", "true\n8"),
		q("print_anonymous", "print(staff | e -> group_by(e.dept) | summarize(n = count()))\n", "tbl: 3 rows\ndept    n\nstr   int\neng     2\nops     2\nnull    1"),
		q("print_in_list", "print(list [staff, staff.head(0)])\nprint(ok(staff.head(1)))\n", "[<tbl[Emp], 5 rows>, <tbl[Emp], 0 rows>]\nok <tbl[Emp], 1 row>"),
		q("print_empty", "print(staff.head(0))\n", "tbl[Emp]: 0 rows\n id  name  dept  salary\nint  str   str    float"),
		{name: "print_long", files: files, source: "incl 'std/csv'\nt = unwrap(csv::read_file('long.csv'))\nprint(t)\n",
			want: "tbl: 12 rows\n  n  s\nint  str\n  1  a\n  2  b\n  3  c\n  4  d\n  5  e\n  6  f\n  7  g\n  8  h\n  9  i\n 10  j\n... 2 more rows"},
		{name: "print_full", files: files, source: "incl 'std/csv'\nincl 'std/fmt'\nt = unwrap(csv::read_file('long.csv'))\nprint(len(fmt::full(t).split('\\n')))\n", want: "15"},
		{name: "print_escapes_and_width", source: "incl 'std/csv'\nt = unwrap(csv::parse_text('s\\n\"a\\nb\"\\n" + strings.Repeat("x", 40) + "\\n'))\nprint(t)\n",
			want: "tbl: 2 rows\ns\nstr\na\\nb\nxxxxxxxxxxxxxxxxxxxxxxxxxxxxx..."},
		q("head_tail", "print(staff.head(2).id)\nprint(staff.tail(2).id)\nprint(len(staff.head().id))\nprint(staff.head(99).id)\nprint(type(staff.tail()))\n",
			"[1, 2]\n[4, 5]\n5\n[1, 2, 3, 4, 5]\ntbl[Emp]"),
		q("head_query_result_type", "r = staff | e -> mutate(k = e.id * 2)\nprint(r.head(1).k)\n", "[2]"),
		q("head_through_any", "fn id(x) -> x\nprint(type(id(staff).head(1)))\n", "tbl[Emp]"),
		q("describe", "staff.describe()\nstaff.describe(2)\n",
			"tbl[Emp]: 5 rows, 4 columns\nid      int    1, 2, 3, 4, 5\nname    str    'Ann', 'Bo', 'Cy', 'Di', 'Ed'\ndept    str    'eng', 'ops', 'eng', 'ops', null\nsalary  float  120000.0, 90000.5, null, 150000.0, 80000.0\n"+
				"tbl[Emp]: 5 rows, 4 columns\nid      int    1, 2, ...\nname    str    'Ann', 'Bo', ...\ndept    str    'eng', 'ops', ...\nsalary  float  120000.0, 90000.5, ..."),
		// Misuse.
		{name: "head_arg_type", files: files, source: queryPre + "print(staff.head('2'))\n", err: "argument 1 of 'head' expects int, got str"},
		{name: "head_arity", files: files, source: queryPre + "print(staff.head(1, 2))\n", err: "method 'head' expects 0-1 arguments but got 2"},
		{name: "head_negative", files: files, source: queryPre + "n = -2\nprint(staff.head(n))\n", err: "head(n) needs n >= 0, got -2"},
		{name: "head_any_arg", files: files, source: queryPre + "fn id(x) -> x\nprint(staff.tail(id('2')))\n", err: "tail(n) expects an int, got str"},
		{name: "describe_negative", files: files, source: queryPre + "staff.describe(-1)\n", err: "describe(n) needs n >= 0, got -1"},
		{name: "head_on_list", source: "print(list [1].head())\n", err: "type 'list[int]' has no method 'head'"},
		{name: "head_on_any_list", source: "fn id(x) -> x\nprint(id(list [1]).describe())\n", err: "has no method 'describe'"},
	})
}

// TestTables_ValueRejected: what a table value isn't (immutable, not
// indexable, no ==, not a condition, not iterable), at compile time and
// through `any`.
func TestTables_ValueRejected(t *testing.T) {
	files := map[string]string{"staff.csv": queryCSV}
	pre := queryPre + "fn id(x) -> x\n"
	q := func(name, body, err string) tableCase {
		return tableCase{name: name, files: files, source: pre + body, err: err}
	}
	runTableCases(t, []tableCase{
		q("assign_column", "staff.salary = vec [1.0]\n", "only dict members are assignable"),
		q("assign_column_any", "u = id(staff)\nu.salary = 1\n", "cannot set member 'salary' on non-dict type"),
		q("index", "print(staff[0])\n", "type 'tbl[Emp]' is not indexable"),
		q("index_any", "print(id(staff)[0])\n", "cannot index a tbl"),
		q("equality", "print(staff == staff)\n", "operator '==' is not defined for tables; compare their columns"),
		q("inequality", "print(staff != staff.head(1))\n", "operator '!=' is not defined for tables"),
		q("equality_any", "print(id(staff) == id(staff))\n", "'==' is not defined for tables"),
		q("equality_in_list", "print(list [id(staff)] == list [id(staff)])\n", "'==' is not defined for tables"),
		q("condition", "if staff:\n    print(1)\n", "a table cannot be used as a condition"),
		q("condition_any", "if id(staff):\n    print(1)\n", "a table used as a condition"),
		q("for_loop", "for r in staff:\n    print(r)\n", "for loop expects list, vec, or str iterable, got tbl[Emp]"),
		q("for_loop_any", "for r in id(staff):\n    print(r)\n", "for-loop iterable must be list, string, or vec, got tbl"),
		q("len", "print(len(staff))\n", "len() of a table isn't defined; its row count is staff.nrow()"),
	})
}

// TestTables_Aggregates is tables v0.3 on one thread: aggregates in filter
// and mutate, grouped steps, expression aggregates, summarize expressions,
// computed keys, and sd (specs/tables_v0_3_spec.md D35–D40).
func TestTables_Aggregates(t *testing.T) {
	files := map[string]string{
		"staff.csv": queryCSV,
		"lib.qrk":   "fn twice(x: float, k: float = 2.0) float -> x * k\n",
	}
	q := func(name, body, want string) tableCase {
		return tableCase{name: name, files: files, source: "incl './lib'\n" + queryPre + body, want: want}
	}
	runTableCases(t, []tableCase{
		q("filter_above_mean", "print((staff | e -> filter(e.salary > mean(e.salary))).name)\n", "['Ann', 'Di']"),
		q("filter_is_max", "print((staff | e -> filter(e.salary == max(e.salary))).name)\n", "['Di']"),
		q("filter_count_values", "print(len((staff | e -> filter(count(e.salary) == 4)).id))\n", "5"),
		q("grouped_filter_max", "print((staff | e -> group_by(e.dept) | filter(e.salary == max(e.salary))).name)\n", "['Ann', 'Di', 'Ed']"),
		q("grouped_filter_count", "print((staff | e -> group_by(e.dept) | filter(count() >= 2)).name)\n", "['Ann', 'Bo', 'Cy', 'Di']"),
		q("group_then_whole", "print((staff | e -> group_by(e.dept) | filter(count() >= 2) | summarize(n = count())).n)\n", "[4]"),
		q("mutate_dev", "print((staff | e -> mutate(dev = e.salary - mean(e.salary))).dev)\n", "[9999.875, -19999.625, null, 39999.875, -30000.125]"),
		q("mutate_same_agg_twice", "r = staff | e -> mutate(a = e.salary - mean(e.salary), b = e.salary - mean(e.salary))\nprint(r.a)\nprint(r.b)\n",
			"[9999.875, -19999.625, null, 39999.875, -30000.125]\n[9999.875, -19999.625, null, 39999.875, -30000.125]"),
		q("mutate_earlier_column", "print((staff | e -> mutate(a = e.id * 2, s = sum(e.a))).s)\n", "[30, 30, 30, 30, 30]"),
		q("grouped_mutate", "print((staff | e -> group_by(e.dept) | mutate(dev = e.salary - mean(e.salary))).dev)\n",
			"[0.0, -29999.75, null, 29999.75, 0.0]"),
		q("expr_sum_matches_vec", "print((staff | e -> summarize(s = sum(e.salary * 2.0))).s[0] == sum(staff.salary * 2.0))\n", "true"),
		q("expr_mean_bool", "print((staff | e -> summarize(p = mean(e.salary > 100000.0))).p[0])\n", "0.5"),
		q("expr_n_distinct", "print((staff | e -> summarize(k = n_distinct(e.id > 2))).k[0])\n", "2"),
		q("summarize_spread", "print(show(staff | e -> summarize(spread = max(e.salary) - min(e.salary), n = count())))\n", "spread,n\n70000.0,5"),
		q("summarize_label", "print(show(staff | e -> group_by(e.dept) | summarize(label = e.dept + ' team', n = count())))\n",
			"dept,label,n\neng,eng team,2\nops,ops team,2\n,,1"),
		q("computed_key", "print(show(staff | e -> group_by(hi = e.salary > 100000.0) | summarize(n = count())))\n",
			"hi,n\nfalse,2\ntrue,2\n,1"),
		q("renamed_key", "print(show(staff | e -> group_by(department = e.dept) | summarize(n = count())))\n",
			"department,n\neng,2\nops,2\n,1"),
		q("sd_matches_vec", "print(sd(staff.salary) == (staff | e -> summarize(s = sd(e.salary))).s[0])\n", "true"),
		q("sd_grouped_matches_vec", "print(sd(staff.salary[staff.dept == 'ops']) == (staff | e -> group_by(e.dept) | summarize(s = sd(e.salary))).s[1])\n", "true"),
		q("sd_too_few", "none = staff | e -> filter(e.id > 99)\none = staff | e -> filter(e.id == 1)\nprint((none | e -> summarize(s = sd(e.salary))).s)\nprint((one | e -> summarize(s = sd(e.salary))).s)\n",
			"[null]\n[null]"),
		q("mean_outside_vec", "print(mean(staff.salary))\nprint(sd(vec [1.0, 2.0, 3.0]))\n", "110000.125\n1.0"),
		q("call_in_aggregate", "print((staff | e -> summarize(s = sum(lib::twice(e.salary)))).s[0] == sum(staff.salary * 2.0))\n", "true"),
		q("prelude_mean_of_column", "print((staff | e -> filter(e.salary > mean(staff.salary))).name)\n", "['Ann', 'Di']"),
		q("n_distinct_in_mutate", "print((staff | e -> mutate(k = n_distinct(e.dept))).k)\n", "[3, 3, 3, 3, 3]"),
		{name: "mean_null", files: files, source: "print(mean(null))\n", err: "argument 1 of 'mean' expects numeric or bool vec, got null"},
		{name: "sum_untyped_in_query", files: files, source: queryPre + "fn id(x) -> x\nr = staff | e -> summarize(s = sum(id(e.salary)))\n", err: "sum needs a number or bool, got any"},
		{name: "vec_where_cell", files: files, source: queryPre + "fn f(v: vec) -> 1\nr = staff | e -> filter(f(e.salary) == 1)\n", err: "'e.salary' is one row's float here, not the column"},
		{name: "key_name_clash", files: files, source: queryPre + "r = staff | e -> group_by(e.dept, dept = e.id) | summarize(n = count())\n", err: "group_by names key 'dept' twice"},
		{name: "sd_of_str", files: files, source: queryPre + "r = staff | e -> summarize(s = sd(e.name))\n", err: "sd needs a number or bool column; column 'name' is str"},
	})
}

// gapsCSV has a group whose values are all missing (a), a NaN (in the
// group of missing keys) and a group with two present values (b).
const gapsCSV = "id,g,x\n1,a,\n2,a,\n3,b,1.5\n4,b,2.5\n5,,NaN\n"

const gapsPre = "incl 'std/csv'\nstruct M:\n    id: int\n    g: str\n    x: float\nm: tbl[M] = unwrap(csv::read_file('m.csv'))\nshow = csv::format_text\n"

// TestTables_AggregateEdges is tables v0.3 §11's aggregate edge cases in
// filter and mutate: a group whose values are all missing, NaN, the
// whole-table min/max loop with and without missing values, an int sum
// that overflows before any row fails, and aggregates in an imported file.
func TestTables_AggregateEdges(t *testing.T) {
	files := map[string]string{"m.csv": gapsCSV}
	q := func(name, body, want string) tableCase {
		return tableCase{name: name, files: files, source: gapsPre + body, want: want}
	}
	dense := "struct D:\n    id: int\n    x: float\n    d: date\nd: tbl[D] = unwrap(csv::parse_text('id,x,d\\n1,NaN,2020-01-01\\n2,3.0,2021-05-05\\n3,-1.0,2019-02-02\\n'))\n"
	runTableCases(t, []tableCase{
		q("grouped_all_missing", "print(show(m | e -> group_by(e.g) | mutate(mu = mean(e.x), s = sd(e.x), mx = max(e.x), c = count(e.x), n = count())))\n",
			"id,g,x,mu,s,mx,c,n\n1,a,,,,,0,2\n2,a,,,,,0,2\n3,b,1.5,2.0,0.7071067811865476,2.5,2,2\n4,b,2.5,2.0,0.7071067811865476,2.5,2,2\n5,,nan,,,,0,1"),
		q("grouped_filter_missing_aggregate", "print((m | e -> group_by(e.g) | filter(e.x == max(e.x) or count(e.x) == 0)).id)\n", "[1, 2, 4, 5]"),
		q("whole_mean_skips_nan", "print((m | e -> filter(e.x >= mean(e.x) - 0.5)).id)\nprint((m | e -> mutate(top = max(e.x), lo = min(e.x))).top)\n", "[3, 4]\n[2.5, 2.5, 2.5, 2.5, 2.5]"),
		q("whole_all_missing", "none = m | e -> filter(e.g == 'a')\nprint((none | e -> mutate(mu = mean(e.x), mx = max(e.x), c = count(e.x))).mx)\nprint(len((none | e -> filter(e.x <= max(e.x))).id))\n",
			"[null, null]\n0"),
		q("whole_dense_extremes", dense+"print(show(d | e -> summarize(a = max(e.x), b = min(e.x), c = max(e.id), lo = min(e.d), hi = max(e.d))))\nprint((d | e -> filter(e.x == max(e.x))).id)\nprint(max(d.x) == (d | e -> summarize(a = max(e.x))).a[0])\n",
			"a,b,c,lo,hi\n3.0,-1.0,3,2019-02-02,2021-05-05\n\n[2]\ntrue"),
		q("whole_dense_all_nan", "struct N:\n    x: float\nn: tbl[N] = unwrap(csv::parse_text('x\\nNaN\\nNaN\\n'))\nprint((n | e -> summarize(a = max(e.x), b = min(e.x))).a)\nprint(len((n | e -> filter(e.x == max(e.x))).x))\n",
			"[null]\n0"),
		{name: "sum_overflow_in_filter", files: files, err: "integer overflow in 'sum'",
			source: gapsPre + "r = m | e -> mutate(k = 9223372036854775807 if e.id < 3 else 1) | filter(e.k > sum(e.k))\n"},
		{name: "sum_overflow_in_mutate", files: files, err: "integer overflow in 'sum'",
			source: gapsPre + "r = m | e -> mutate(k = 9223372036854775807 if e.id < 3 else 1) | mutate(s = sum(e.k))\n"},
		{name: "sum_overflow_before_row_error", files: files, err: "integer overflow in 'sum'",
			source: gapsPre + "r = m | e -> mutate(k = 9223372036854775807 if e.id < 3 else 1) | filter(to_int(e.g) > sum(e.k))\n"},
		// Aggregate names in an imported file are the aggregates, whatever
		// the main script defines.
		{name: "imported_file", want: "dept,n,k,m\neng,2,2,120000.0\nops,2,2,120000.25\n,1,1,80000.0\n\n['Ann', 'Di']\n7",
			files: map[string]string{"staff.csv": queryCSV, "lib.qrk": "incl 'std/csv'\n" + empStruct +
				"fn per_dept(t: tbl[Emp]) tbl -> t | e -> group_by(e.dept) | summarize(n = count(), k = n_distinct(e.name), m = mean(e.salary))\n" +
				"fn above(t: tbl[Emp]) tbl[Emp] -> t | e -> filter(e.salary > mean(e.salary) and count() > 1)\n"},
			source: "incl 'std/csv'\nincl './lib'\ncount = 7\nstaff: tbl[lib::Emp] = unwrap(csv::read_file('staff.csv'))\nprint(csv::format_text(lib::per_dept(staff)))\nprint(lib::above(staff).name)\nprint(count)\n"},
	})
}

// TestTables_AggregateNames: in a verb, a call of an aggregate name is the
// aggregate, so a definition of that name it would hide is an error, in the
// main script and in an imported file (decided 2026-11-17).
func TestTables_AggregateNames(t *testing.T) {
	staff := map[string]string{"staff.csv": queryCSV}
	q := func(name, head, body, err string) tableCase {
		return tableCase{name: name, files: staff, source: head + queryPre + body, err: err}
	}
	lib := func(name, libSrc, err string) tableCase {
		return tableCase{name: name, err: err, files: map[string]string{"lib.qrk": "struct Emp:\n    id: int\n" + libSrc},
			source: "incl './lib'\nprint(1)\n"}
	}
	runTableCases(t, []tableCase{
		q("user_fn", "fn count(x: int) int -> x + 100\n", "r = staff | e -> mutate(y = count(e.id))\n",
			"'count(...)' in a query is the aggregate, but this program defines 'count' (line 1); rename it to use the aggregate"),
		q("variable", "count = 3\n", "r = staff | e -> filter(count() > 1)\n", "but this program defines 'count' (line 1)"),
		q("n_distinct_fn", "fn n_distinct(x: str) int -> 1\n", "r = staff | e -> filter(n_distinct(e.dept) > 1)\n", "'n_distinct(...)' in a query is the aggregate"),
		q("parameter", "", "fn f(t: tbl[Emp], n_distinct: int) -> t | e -> filter(n_distinct(e.dept) > 1)\n", "but this program defines 'n_distinct' (line 9)"),
		q("parameter_mean", "", "fn f(t: tbl[Emp], mean: float) -> t | e -> filter(e.salary > mean(e.salary))\n", "'mean(...)' in a query is the aggregate, but this program defines 'mean' (line 9)"),
		{name: "extern_fn", files: map[string]string{"staff.csv": queryCSV, "tick.hpp": rowSafeTickH}, err: "'count(...)' in a query is the aggregate, but this program defines 'count'",
			source: "extern 'tick.hpp'\nextern rowsafe fn count() int as 'q_rowsafe_tick'\n" + queryPre + "r = staff | e -> filter(count() > 1)\n"},
		q("row_name", "", "r = staff | count -> filter(count() > 1)\n", "the query's row is named 'count'; rename the row"),
		lib("imported_def", "fn count(x: int) int -> x\nfn f(t: tbl[Emp]) tbl -> t | e -> mutate(y = count(e.id))\n",
			"'count(...)' in a query is the aggregate, but this program defines 'count' (line 3)"),
		lib("imported_param", "fn g(t: tbl[Emp], n_distinct: int) tbl -> t | e -> filter(n_distinct(e.id) > 0)\n", "'n_distinct(...)' in a query is the aggregate"),
		// head's count isn't a verb expression, so count() there names
		// nothing in the imported file, not the main script's count.
		{name: "imported_head_count", err: "undefined identifier 'count'",
			files:  map[string]string{"lib.qrk": "struct Emp:\n    id: int\nfn h(t: tbl[Emp]) tbl[Emp] -> t | e -> head(count())\n"},
			source: "incl './lib'\nfn count() int -> 2\nprint(count())\n"},
		// The name is still free outside verbs and as a value in one.
		{name: "value_and_outside", want: "[1, 2]\n101\n[101, 102, 103, 104, 105]",
			files:  map[string]string{"staff.csv": queryCSV, "lib.qrk": "fn count(x: int) int -> x + 100\n"},
			source: "incl './lib'\ncount = 3\n" + queryPre + "print((staff | e -> filter(e.id < count)).id)\nprint(lib::count(1))\nprint((staff | e -> mutate(y = lib::count(e.id))).y)\n"},
	})
}

// rowSafeTickH is a glue header whose rowsafe functions count calls or fail.
// The generated program includes quark.hpp first, so q_runtime_reportf is in scope.
const rowSafeTickH = `#pragma once
static int q_rowsafe_tick_n = 0;
inline int64_t q_rowsafe_tick() { return ++q_rowsafe_tick_n; }
inline int64_t q_rowsafe_ticks() { return q_rowsafe_tick_n; }
inline int64_t q_rowsafe_boom() {
    q_runtime_reportf("runtime error: boom\n");
    q_exit(1);
    return 0;
}
`

const rowSafeTagH = `#pragma once
inline const char* q_tag(const char* s) { return s != nullptr ? s : ""; }
`

// TestTables_RowSafety is tables v0.3 D41-D43: a verb may call only row-safe
// functions, and the error names the call that makes a user function unsafe.
func TestTables_RowSafety(t *testing.T) {
	staff := map[string]string{"staff.csv": queryCSV}
	use := func(name, head, call, err string) tableCase {
		return tableCase{name: name, files: staff, err: err,
			source: head + queryPre + "r = staff | e -> filter(" + call + ")\n"}
	}
	runTableCases(t, []tableCase{
		use("direct_print", "", "print(e.name) == null", "'print' can't be called in a query"),
		use("direct_input", "", "is_ok(input())", "'input' can't be called in a query"),
		use("direct_describe", "", "e.id > 0 or type(staff.describe()) == 'int'", "'describe' can't be called in a query"),
		use("direct_push", "", "len(list [e.id].push(1)) > 0", "'push' can't be called in a query"),
		use("direct_pop", "", "list [1].pop() != null", "'pop' can't be called in a query"),
		use("direct_set", "", "len(list [1].set(0, 2)) > 0", "'set' can't be called in a query"),
		use("direct_insert", "", "len(list [1].insert(0, 2)) > 0", "'insert' can't be called in a query"),
		use("direct_remove", "", "list [1].remove(0) != null", "'remove' can't be called in a query"),
		use("direct_reverse", "", "len(list [1].reverse()) > 0", "'reverse' can't be called in a query"),
		use("direct_sort", "", "len(list [1].sort()) > 0", "'sort' can't be called in a query"),
		use("direct_sort_by", "", "len(list [1].sort_by(fn(a, b) -> a)) > 0", "'sort_by' can't be called in a query"),
		use("direct_dict_set", "", "dict { a: 1 }.set('a', 2) != null", "'set' can't be called in a query"),
		use("helper_print", "fn bad(x: float) float ->\n    print(x)\n    x\n", "bad(e.salary) > 0.0",
			"'bad' can't be called in a query: it calls 'print' (line 2)"),
		use("two_helpers", "fn inner(x: float) float ->\n    print(x)\n    x\nfn mid(x: float) float -> inner(x)\nfn outer(x: float) float -> mid(x)\n",
			"outer(e.salary) > 0.0",
			"'outer' can't be called in a query: it calls 'mid' (line 5), which calls 'inner' (line 4), which calls 'print' (line 2)"),
		use("mutual", "fn a(x: float) float -> b(x)\nfn b(x: float) float ->\n    print(x)\n    a(x)\n",
			"a(e.salary) > 0.0",
			"'a' can't be called in a query: it calls 'b' (line 1), which calls 'print' (line 3)"),
		use("assign_outer", "n = 0\nfn bump(x: float) float ->\n    n = n + 1\n    x\n", "bump(e.salary) > 0.0",
			"'bump' can't be called in a query: it assigns 'n' (line 3)"),
		use("push_local", "fn traced(x: float) float ->\n    log = list []\n    log.push(x)\n    x\n", "traced(e.salary) > 0.0",
			"'traced' can't be called in a query: it calls 'push' (line 3)"),
		use("elem_assign", "fn bad(x: float) float ->\n    xs = list [x]\n    xs[0] = x\n    x\n", "bad(e.salary) > 0.0",
			"'bad' can't be called in a query: it changes a list or dict (line 3)"),
		use("dict_assign", "fn bad(x: float) float ->\n    d = dict { a: x }\n    d.a = x\n    x\n", "bad(e.salary) > 0.0",
			"'bad' can't be called in a query: it changes a list or dict (line 3)"),
		use("calls_lambda", "fn apply(x: float) float ->\n    f = fn(y: float) float -> y * 2.0\n    f(x)\n", "apply(e.salary) > 0.0",
			"'apply' can't be called in a query: it calls a function value (line 3)"),
		use("function_value", "f = fn(x: float) float -> x\n", "f(e.salary) > 0.0",
			"'f' can't be called in a query: it's a function value"),
		use("lambda_direct", "", "(fn(x: float) float -> x)(e.salary) > 0.0",
			"a function value can't be called in a query"),
		use("default_param", "fn pad(x: float, k: float = 1.0) float ->\n    print(k)\n    x\n", "pad(e.salary) > 0.0",
			"'pad' can't be called in a query: it calls 'print' (line 2)"),
		use("io_exists", "incl 'std/io'\n", "io::exists('nope')",
			"'io::exists' can't be called in a query: it calls the native function 'io_exists', which isn't declared rowsafe"),
		use("sys_cpu", "incl 'std/sys'\n", "sys::cpu_count() > 0",
			"'sys::cpu_count' can't be called in a query: it calls the native function 'sys_cpu_count', which isn't declared rowsafe"),
		use("csv_format", "", "csv::format_text(staff) == ''",
			"'csv::format_text' can't be called in a query: it calls the native function 'csv_format_text', which isn't declared rowsafe"),
		use("time_now", "incl 'std/time'\n", "time::now() > 0.0",
			"'time::now' can't be called in a query: it calls the native function 'time_now', which isn't declared rowsafe"),
		use("time_since", "incl 'std/time'\n", "time::since(0.0) > 0.0",
			"'time::since' can't be called in a query: it calls 'time::clock'"),
		use("time_today", "incl 'std/time'\n", "time::today() == time::date(2020, 1, 1)",
			"'time::today' can't be called in a query"),
		use("time_sleep", "incl 'std/time'\n", "time::sleep(0.0) == null",
			"'time::sleep' can't be called in a query"),
		use("random_seed", "incl 'std/random'\n", "random::seed(1) == null",
			"'random::seed' can't be called in a query"),
		use("random_shuffle", "incl 'std/random'\n", "len(random::shuffle(list [1])) > 0",
			"'random::shuffle' can't be called in a query"),
		use("blas_dot", "incl 'std/blas'\n", "blas::dot(vec [1.0], vec [1.0]) > 0.0",
			"'blas::dot' can't be called in a query: it calls the native function 'blas_dot', which isn't declared rowsafe"),
		// A rowsafe native function can't take a function value: its promise
		// can't cover the Quark code it would call (decided 2026-11-17).
		{name: "rowsafe_fn_param", files: map[string]string{"h.hpp": rowSafeTagH},
			source: "extern 'h.hpp'\nextern rowsafe fn apply(f: fn, x: float) float as 'q_tag'\nprint(1)\n",
			err:    "a rowsafe native function can't take a function value ('f: fn')"},
		{name: "rowsafe_method_fn_param", files: map[string]string{"h.hpp": rowSafeTagH},
			source: "extern 'h.hpp'\nextern rowsafe fn str.apply(f: fn) str as 'q_tag'\nprint(1)\n",
			err:    "a rowsafe native function can't take a function value ('f: fn')"},
		{name: "plain_fn_param_in_verb", files: map[string]string{"staff.csv": queryCSV, "h.hpp": rowSafeTagH},
			source: "extern 'h.hpp'\nextern fn apply(f: fn, x: float) float as 'q_tag'\n" + queryPre + "r = staff | e -> mutate(y = apply(fn(v: float) float -> v, e.salary))\n",
			err:    "'apply' can't be called in a query: it isn't declared rowsafe"},
		{name: "module_helper", files: map[string]string{"staff.csv": queryCSV, "lib.qrk": "fn loud(x: float) float ->\n    print(x)\n    x\n"},
			source: "incl './lib'\n" + queryPre + "r = staff | e -> filter(lib::loud(e.salary) > 0.0)\n",
			err:    "'lib::loud' can't be called in a query: it calls 'print' (line 2)"},
		{name: "extern_not_rowsafe", files: map[string]string{"staff.csv": queryCSV, "h.hpp": rowSafeTagH, "lib.qrk": "extern 'h.hpp'\nextern fn c_norm(x: float) float as 'q_tag'\nmodule lib:\n    fn norm(x: float) float -> c_norm(x)\n"},
			source: "incl './lib'\n" + queryPre + "r = staff | e -> filter(lib::norm(e.salary) > 0.0)\n",
			err:    "'lib::norm' can't be called in a query: it calls the native function 'c_norm', which isn't declared rowsafe"},
		{name: "extern_method_not_rowsafe", files: map[string]string{"staff.csv": queryCSV, "h.hpp": rowSafeTagH},
			source: "extern 'h.hpp'\nextern fn str.tag() str as 'q_tag'\nfn labeled(s: str) str -> s.tag()\n" + queryPre + "r = staff | e -> filter(labeled(e.name) == 'Ann')\n",
			err:    "'labeled' can't be called in a query: it calls the native function 'tag', which isn't declared rowsafe"},
		{name: "reads_outer", files: staff,
			source: "threshold = 100000.0\nfn band(x: float) str -> 'high' if x > threshold else 'low'\n" + queryPre + "print((staff | e -> mutate(b = band(e.salary))).b)\n",
			want:   "['high', 'low', null, 'high', 'low']"},
		{name: "mutual_safe", files: staff,
			source: "fn a(x: float) float -> b(x)\nfn b(x: float) float -> x\n" + queryPre + "print((staff | e -> mutate(z = a(e.salary))).z)\n",
			want:   "[120000.0, 90000.5, null, 150000.0, 80000.0]"},
		{name: "default_ok", files: staff,
			source: "fn pad(x: float, k: float = 1.0) float -> x + k\n" + queryPre + "print((staff | e -> mutate(y = pad(e.id))).y)\n",
			want:   "[2.0, 3.0, 4.0, 5.0, 6.0]"},
		{name: "time_year", files: staff,
			source: "incl 'std/time'\n" + queryPre + "print((staff | e -> mutate(y = time::year(time::date(2020, 1, e.id)))).y)\n",
			want:   "[2020, 2020, 2020, 2020, 2020]"},
		{name: "extern_method_rowsafe", files: map[string]string{"staff.csv": queryCSV, "h.hpp": rowSafeTagH},
			source: "extern 'h.hpp'\nextern rowsafe fn str.tag() str as 'q_tag'\nfn labeled(s: str) str -> s.tag()\n" + queryPre + "print((staff | e -> mutate(n = labeled(e.name))).n)\n",
			want:   "['Ann', 'Bo', 'Cy', 'Di', 'Ed']"},
	})
}

// TestTables_Once is D44: a row-safe call whose arguments don't read the row
// runs the first time a row needs it, and not at all when no row does.
// A random draw is not reused.
func TestTables_Once(t *testing.T) {
	files := map[string]string{"staff.csv": queryCSV, "tick.hpp": rowSafeTickH}
	tick := "extern 'tick.hpp'\nextern rowsafe fn tick() int as 'q_rowsafe_tick'\nextern rowsafe fn ticks() int as 'q_rowsafe_ticks'\nextern rowsafe fn boom() int as 'q_rowsafe_boom'\n"
	q := func(name, body, want, err string) tableCase {
		return tableCase{name: name, files: files, source: tick + queryPre + body, want: want, err: err}
	}
	runTableCases(t, []tableCase{
		q("counted_once", "r = staff | e -> mutate(n = tick())\nprint(r.n)\nprint(ticks())\n", "[1, 1, 1, 1, 1]\n1", ""),
		q("mean_of_outside_column", "fn mark(x: float) float ->\n    tick()\n    x\nr = staff | e -> filter(e.salary > mark(mean(staff.salary)))\nprint(r.name)\nprint(ticks())\n", "['Ann', 'Di']\n1", ""),
		q("ungrouped_aggregate_arg", "fn mark(x: float) float ->\n    tick()\n    x\nr = staff | e -> filter(e.salary > mark(mean(e.salary)))\nprint(ticks())\n", "1", ""),
		q("grouped_not_once", "fn mark(x: float) float ->\n    tick()\n    x\nr = staff | e -> group_by(e.dept) | filter(e.salary > mark(mean(e.salary)))\nprint(ticks())\n", "4", ""),
		q("never_and", "r = staff | e -> filter(false and tick() > 0)\nprint(len(r.id))\nprint(ticks())\n", "0\n0", ""),
		q("never_ternary", "r = staff | e -> mutate(n = tick() if false else e.id)\nprint(r.n)\nprint(ticks())\n", "[1, 2, 3, 4, 5]\n0", ""),
		q("empty_table", "none = staff | e -> filter(e.id > 99)\nr = none | e -> mutate(n = tick())\nprint(len(r.id))\nprint(ticks())\n", "0\n0", ""),
		q("empty_does_not_fail", "none = staff | e -> filter(e.id > 99)\nr = none | e -> filter(boom() > 0)\nprint(len(r.id))\n", "0", ""),
		q("never_branch_does_not_fail", "r = staff | e -> filter(false and boom() > 0)\nprint(len(r.id))\n", "0", ""),
		q("earlier_row_error_wins", "struct S:\n    x: int\n    s: str\nt: tbl[S] = unwrap(csv::parse_text('x,s\\n0,no\\n1,2\\n'))\nr = t | e -> filter(to_int(e.s) > 0 and boom() > 0)\n", "", "cannot parse 'no'"),
		q("fails_at_first_row_that_needs_it", "struct S:\n    x: int\nt: tbl[S] = unwrap(csv::parse_text('x\\n0\\n1\\n'))\nr = t | e -> filter(e.x == 1 and boom() > 0)\n", "", "boom"),
		{name: "error_location_after_once", files: map[string]string{"staff.csv": queryCSV},
			source: queryPre + "fn k(x: int) int -> x\nr = staff | e -> mutate(a = k(1) + 9223372036854775807 * (e.id - 1))\n",
			err:    "main.qrk:10:18"},
		{name: "draw_not_once", files: map[string]string{"staff.csv": queryCSV},
			source: "incl 'std/random'\n" + queryPre + "random::seed(1)\nr = staff | e -> mutate(u = random::uniform())\nprint((r | e -> summarize(k = n_distinct(e.u))).k[0])\n",
			want:   "5"},
		{name: "draw_in_helper_not_once", files: map[string]string{"staff.csv": queryCSV},
			source: "incl 'std/random'\nfn jitter() float -> random::uniform()\n" + queryPre + "random::seed(1)\nr = staff | e -> mutate(u = jitter())\nprint((r | e -> summarize(k = n_distinct(e.u))).k[0])\n",
			want:   "5"},
	})
}

// rowSafeListH is a glue header whose rowsafe functions return a list (and
// count their calls) or fail, for contains() sets built once per step.
const rowSafeListH = `#pragma once
static int q_rowsafe_list_n = 0;
inline QList* q_rowsafe_tick_list() {
    ++q_rowsafe_list_n;
    QValue l = qv_list(2, &q_elem_int);
    l.data.list_val->push_back(qv_int(2));
    l.data.list_val->push_back(qv_int(4));
    return l.data.list_val;
}
inline int64_t q_rowsafe_list_ticks() { return q_rowsafe_list_n; }
inline QList* q_rowsafe_boom_list() {
    q_runtime_reportf("runtime error: boom\n");
    q_exit(1);
    return nullptr;
}
`

// TestTables_MissingValues is tables v0.3 §6 (D46, D47): `== null` and
// `!= null` test whether a verb expression is missing, `x.fillna(v)` replaces
// a missing value, and drop_na is a step.
func TestTables_MissingValues(t *testing.T) {
	// pairCSV has a row where both a and b are missing, and NaN in b.
	pairCSV := "id,a,b\n1,1.5,\n2,,2.5\n3,,\n4,4.5,NaN\n"
	files := map[string]string{"staff.csv": queryCSV, "p.csv": pairCSV,
		"lib.qrk": "struct Emp:\n    id: int\n    name: str\n    dept: str\n    salary: float\nmodule lib:\n    fn unpaid(t: tbl[Emp]) -> t | e -> filter(e.salary == null)\n"}
	q := func(name, body, want string) tableCase {
		return tableCase{name: name, files: files, source: queryPre + body, want: want}
	}
	pairs := "struct P:\n    id: int\n    a: float\n    b: float\np: tbl[P] = unwrap(csv::read_file('p.csv'))\n"
	runTableCases(t, []tableCase{
		q("eq_null_column", "print((staff | e -> filter(e.salary == null)).name)\n", "['Cy']"),
		q("null_eq_column", "print((staff | e -> filter(null == e.dept)).name)\n", "['Ed']"),
		q("ne_null", "print((staff | e -> filter(e.salary != null and e.dept != null)).name)\nprint((staff | e -> filter(null != e.salary)).name)\n",
			"['Ann', 'Bo', 'Di']\n['Ann', 'Bo', 'Di', 'Ed']"),
		q("never_missing", "print((staff | e -> mutate(m = e.salary == null, k = to_int(e.salary == null) + 1)).m)\n", "[false, false, true, false, false]"),
		q("expression", "print((staff | e -> filter(e.salary * 2.0 + to_float(e.id) == null)).name)\n", "['Cy']"),
		q("in_logic", "print((staff | e -> filter(e.salary == null or e.dept == null)).name)\nprint((staff | e -> filter(!(e.salary == null))).name)\n",
			"['Cy', 'Ed']\n['Ann', 'Bo', 'Di', 'Ed']"),
		q("aggregate", "none = staff | e -> filter(e.id > 99)\nprint((none | e -> summarize(m = mean(e.salary) == null)).m)\nprint((staff | e -> summarize(m = mean(e.salary) == null)).m)\n",
			"[true]\n[false]"),
		q("count_missing", "print(show(staff | e -> summarize(gaps = sum(e.salary == null), both = sum(e.salary == null or e.dept == null))))\nprint((staff | e -> group_by(e.dept) | summarize(g = sum(e.salary == null))).g)\nprint((staff | e -> mutate(g = sum(e.salary == null))).g)\n",
			"gaps,both\n1,2\n\n[1, 0, 0]\n[1, 1, 1, 1, 1]"),
		q("null_variable", "n = null\nprint((staff | e -> filter(e.dept == n)).name)\n", "['Ed']"),
		q("dense_table", "full = staff | e -> drop_na()\nprint((full | e -> mutate(m = e.salary == null, k = e.salary != null)).m)\nprint((full | e -> mutate(f = e.salary.fillna(0.0))).f)\n",
			"[false, false, false]\n[120000.0, 90000.5, 150000.0]"),
		q("fillna", "print((staff | e -> mutate(s = e.salary.fillna(0.0))).s)\nprint((staff | e -> mutate(s = e.salary.fillna(1))).s)\nprint((staff | e -> mutate(d = e.dept.fillna('none'))).d)\n",
			"[120000.0, 90000.5, 0.0, 150000.0, 80000.0]\n[120000.0, 90000.5, 1.0, 150000.0, 80000.0]\n['eng', 'ops', 'eng', 'ops', 'none']"),
		q("fillna_aggregate", "print((staff | e -> mutate(s = e.salary.fillna(mean(e.salary)))).s)\nprint((staff | e -> group_by(e.dept) | mutate(s = e.salary.fillna(max(e.salary)))).s)\n",
			"[120000.0, 90000.5, 110000.125, 150000.0, 80000.0]\n[120000.0, 90000.5, 120000.0, 150000.0, 80000.0]"),
		q("fillna_expression", "print((staff | e -> mutate(x = (e.salary * 2.0).fillna(-1.0) + 1.0)).x)\nprint((staff | e -> filter(e.salary.fillna(0.0) < 100000.0)).name)\n",
			"[240001.0, 180002.0, 0.0, 300001.0, 160001.0]\n['Bo', 'Cy', 'Ed']"),
		// The fill value is computed only where the value is missing: here it
		// divides by zero on the first row, whose salary is present.
		q("fillna_value_lazy", "print((staff | e -> mutate(s = e.salary.fillna(to_float(100 / (e.id - 1))))).s)\n",
			"[120000.0, 90000.5, 50.0, 150000.0, 80000.0]"),
		q("fillna_other_column", pairs+"print((p | e -> mutate(c = e.a.fillna(e.b))).c)\nprint((p | e -> summarize(k = count(e.a.fillna(e.b)))).k)\n",
			"[1.5, 2.5, null, 4.5]\n[3]"),
		q("fillna_in_aggregate", "print((staff | e -> summarize(s = sum(e.salary.fillna(1.0)), m = mean(e.salary.fillna(0.0)))).s)\n", "[440001.5]"),
		q("drop_na", "print((staff | e -> drop_na()).name)\nprint((staff | e -> drop_na(e.salary)).name)\nprint((staff | e -> drop_na(e.id)).name)\nprint(type(staff | e -> drop_na(e.dept)))\n",
			"['Ann', 'Bo', 'Di']\n['Ann', 'Bo', 'Di', 'Ed']\n['Ann', 'Bo', 'Cy', 'Di', 'Ed']\ntbl[Emp]"),
		q("drop_na_keeps_nan", pairs+"print((p | e -> drop_na(e.b)).id)\nprint((p | e -> drop_na()).id)\n", "[2, 4]\n[4]"),
		q("drop_na_empty", "none = staff | e -> filter(e.id > 99)\nprint((none | e -> drop_na()).id)\n", "[]"),
		{name: "in_module", files: files, source: "incl 'std/csv'\nincl './lib'\nfn load(p: str) tbl[lib::Emp] -> unwrap(csv::read_file(p))\nprint(lib::unpaid(load('staff.csv')).name)\n", want: "['Cy']"},
		// Outside a verb, == null keeps its meaning, in a function a verb
		// calls too (which isn't called for a row whose value is missing).
		q("outside_verb", "x = null\nprint(x == null)\nprint(staff.salary.get(2) == null)\nfn isnull(v) bool -> v == null\nprint((staff | e -> filter(isnull(e.salary))).name)\n",
			"true\ntrue\n[]"),
	})
}

// TestTables_MissingValuesRejected: misuse of == null, fillna and drop_na in
// verbs.
func TestTables_MissingValuesRejected(t *testing.T) {
	files := map[string]string{"staff.csv": queryCSV}
	q := func(name, body, err string) tableCase {
		return tableCase{name: name, files: files, source: queryPre + body, err: err}
	}
	runTableCases(t, []tableCase{
		q("ordered_null", "r = staff | e -> filter(e.salary < null)\n", "in a query, '<' with null compares with a missing value"),
		q("ordered_null_left", "r = staff | e -> filter(null >= e.id)\n", "in a query, '>=' with null compares with a missing value"),
		q("null_null", "r = staff | e -> filter(null == null)\n", "in a query, '==' compares null with null"),
		q("fillna_narrowing", "r = staff | e -> mutate(x = e.id.fillna(1.5))\n", "fillna() can't fill int with float"),
		q("fillna_str_int", "r = staff | e -> mutate(x = e.dept.fillna(1))\n", "fillna() can't fill str with int"),
		q("fillna_null", "r = staff | e -> mutate(x = e.salary.fillna(null))\n", "fillna() can't fill float with null"),
		q("fillna_no_value", "r = staff | e -> mutate(x = e.salary.fillna())\n", "fillna takes one value, the value for rows where 'e.salary' is missing, got 0"),
		q("fillna_two_values", "r = staff | e -> mutate(x = e.salary.fillna(1.0, 2.0))\n", "fillna takes one value"),
		q("fillna_untyped_value", "fn id(v) -> v\nr = staff | e -> mutate(x = e.salary.fillna(id(1.0)))\n", "the type of fillna()'s value isn't known; it must be float"),
		q("fillna_unsafe_value", "r = staff | e -> mutate(x = e.salary.fillna(to_float(print(1))))\n", "'print' can't be called in a query"),
		q("drop_na_expression", "r = staff | e -> drop_na(e.salary * 2.0)\n", "drop_na takes columns (e.col); to drop the rows where a computed value is missing, write filter(... != null)"),
		q("drop_na_named", "r = staff | e -> drop_na(s = e.salary)\n", "drop_na takes no named arguments"),
		q("drop_na_twice", "r = staff | e -> drop_na(e.id, e.id)\n", "drop_na names column 'id' twice"),
		q("drop_na_unknown_column", "r = staff | e -> drop_na(e.wage)\n", "Emp has no column 'wage'"),
		q("drop_na_after_group_by", "r = staff | e -> group_by(e.dept) | drop_na()\n", "group_by applies to the next step, which must be filter, mutate or summarize (got drop_na)"),
		q("fillna_vec_still_checked", "v = staff.salary.fillna('x')\n", "fillna() can't fill vec[float] with str"),
	})
}

// TestTables_CountStep is D50: count(keys) is group_by(keys) |
// summarize(n = count()).
func TestTables_CountStep(t *testing.T) {
	files := map[string]string{"staff.csv": queryCSV}
	q := func(name, body, want string) tableCase {
		return tableCase{name: name, files: files, source: queryPre + body, want: want}
	}
	r := func(name, body, err string) tableCase {
		return tableCase{name: name, files: files, source: queryPre + body, err: err}
	}
	runTableCases(t, []tableCase{
		q("by_column", "print(show(staff | e -> count(e.dept)))\nprint(type(staff | e -> count(e.dept)))\n",
			"dept,n\neng,2\nops,2\n,1\n\ntbl[{dept: str, n: int}]"),
		q("whole", "print(show(staff | e -> count()))\n", "n\n5"),
		q("computed_key", "print(show(staff | e -> count(e.dept, hi = e.salary > 100000.0)))\n",
			"dept,hi,n\neng,true,1\neng,,1\nops,false,1\nops,true,1\n,false,1"),
		q("same_as_summarize", "a = staff | e -> count(e.dept)\nb = staff | e -> group_by(e.dept) | summarize(n = count())\nprint(show(a) == show(b))\n", "true"),
		q("empty", "none = staff | e -> filter(e.id > 99)\nprint(show(none | e -> count()))\nprint(show(none | e -> count(e.dept)))\n", "n\n0\n\ndept,n"),
		q("then_steps", "print((staff | e -> count(e.dept) | filter(e.n > 1) | sort(desc(e.dept))).dept)\n", "['ops', 'eng']"),
		q("into_struct", "struct C:\n    dept: str\n    n: int\nc: tbl[C] = staff | e -> count(e.dept)\nprint(type(c))\nprint(c.n)\n", "tbl[C]\n[2, 2, 1]"),
		q("count_variable_free", "count = 3\nprint(show(staff | e -> count(e.dept) | filter(e.n < count)))\n", "dept,n\neng,2\nops,2\n,1"),
		r("key_named_n", "r = staff | e -> mutate(n = e.id) | count(e.n)\n", "count's result has its own column 'n' for the counts, so a key can't be named 'n'"),
		r("computed_key_named_n", "r = staff | e -> count(n = e.id > 2)\n", "a key can't be named 'n'"),
		r("unnamed_computed_key", "r = staff | e -> count(e.id > 2)\n", "a computed group key needs a name: count(year = time::year(e.day))"),
		r("after_group_by", "r = staff | e -> group_by(e.dept) | count()\n", "group_by applies to the next step, which must be filter, mutate or summarize (got count)"),
		r("twice", "r = staff | e -> count(e.dept, e.dept)\n", "count names column 'dept' twice"),
		r("aggregate_key", "r = staff | e -> count(s = sum(e.id))\n", "aggregates are for filter, mutate and summarize"),
		r("non_column_key", "r = staff | e -> count(k = list [e.id])\n", "column 'k' would be list[int]"),
	})
}

// TestTables_Contains is D48: xs.contains(x) on lists and vecs, and in a
// verb against a list or vec that doesn't read the row (a hash set).
func TestTables_Contains(t *testing.T) {
	files := map[string]string{"staff.csv": queryCSV, "l.hpp": rowSafeListH}
	q := func(name, body, want string) tableCase {
		return tableCase{name: name, files: files, source: queryPre + body, want: want}
	}
	r := func(name, body, err string) tableCase {
		return tableCase{name: name, files: files, source: queryPre + body, err: err}
	}
	glue := "extern 'l.hpp'\nextern rowsafe fn tick_list() list as 'q_rowsafe_tick_list'\nextern rowsafe fn list_ticks() int as 'q_rowsafe_list_ticks'\nextern rowsafe fn boom_list() list as 'q_rowsafe_boom_list'\n"
	runTableCases(t, []tableCase{
		q("list", "print(list ['eng', 'x'].contains('eng'))\nprint(list ['eng'].contains('ops'))\nprint(list [1, 2].contains(2.0))\nprint(list [1, 2].contains(2.5))\nprint(list [list [1]].contains(list [1]))\n",
			"true\nfalse\ntrue\nfalse\ntrue"),
		q("vec", "print(vec [1.0, 2.0].contains(2))\nprint(vec [1, 2].contains(2.0))\nprint(vec ['a'].contains('a'))\nprint(vec [true].contains(false))\nprint(staff.salary.contains(120000))\n",
			"true\ntrue\ntrue\nfalse\ntrue"),
		q("exact_int_float", "print(list [9007199254740993].contains(9007199254740992.0))\nprint(vec [9007199254740992.0].contains(9007199254740993))\nprint(vec [9007199254740992.0].contains(9007199254740992))\nprint(list [-0.0].contains(0))\n",
			"false\nfalse\ntrue\ntrue"),
		q("missing_and_nan", "print(list [1, null].to_vec().contains(0))\nprint(staff.salary.contains(0.0))\nprint(list [to_float('nan')].contains(to_float('nan')))\nprint(vec [to_float('nan')].contains(to_float('nan')))\n",
			"false\nfalse\nfalse\nfalse"),
		q("dates", "incl 'std/time'\nstruct D:\n    d: date\ndt: tbl[D] = unwrap(csv::parse_text('d\\n2020-01-01\\n'))\nprint(dt.d.contains(time::date(2020, 1, 1)))\nprint(list [time::date(2020, 1, 2)].contains(time::date(2020, 1, 1)))\nprint((staff | e -> filter(dt.d.contains(time::date(2020, 1, e.id)))).id)\n", "true\nfalse\n[1]"),
		q("str_still_substring", "print('abc'.contains('b'))\nfn id(v) -> v\nprint(id('abc').contains('b'))\nprint(id(list ['b']).contains('b'))\nprint(id(vec [1]).contains(1))\n", "true\ntrue\ntrue\ntrue"),
		q("verb_outside_list", "keep = list ['eng', 'x']\nprint((staff | e -> filter(keep.contains(e.dept))).name)\n", "['Ann', 'Cy']"),
		q("verb_negated_missing", "keep = list ['eng']\nprint((staff | e -> filter(!keep.contains(e.dept))).name)\n", "['Bo', 'Di']"),
		q("verb_other_table", "ids = vec [2, 4]\nprint((staff | e -> mutate(k = ids.contains(e.id), j = staff.salary.contains(e.salary))).k)\nprint((staff | e -> mutate(j = staff.salary.contains(e.salary))).j)\n",
			"[false, true, false, true, false]\n[true, true, null, true, true]"),
		q("verb_int_float", "fs = list [2.0, 4.5]\nprint((staff | e -> filter(fs.contains(e.id))).id)\nis = vec [120000, 7]\nprint((staff | e -> filter(is.contains(e.salary))).id)\n", "[2]\n[1]"),
		q("verb_untyped_elements", "fn id(v) -> v\nxs = list []\nxs.push(id(list [1]))\nxs.push(id('Bo'))\nxs.push(id(null))\nxs.push(id(4.0))\nprint((staff | e -> filter(xs.contains(e.name) or xs.contains(e.id))).name)\nprint((staff | e -> mutate(b = xs.contains(list [e.id]))).b)\n",
			"['Bo', 'Di']\n[true, false, false, false, false]"),
		q("verb_grouped", "keep = list ['Ann', 'Bo']\nprint((staff | e -> group_by(e.dept) | filter(keep.contains(e.name) or count() == 1)).name)\n", "['Ann', 'Bo', 'Ed']"),
		q("verb_set_built_once", glue+"print((staff | e -> filter(tick_list().contains(e.id))).id)\nprint(list_ticks())\n", "[2, 4]\n1"),
		q("verb_set_not_built_without_rows", glue+"none = staff | e -> filter(e.id > 99)\nprint((none | e -> filter(boom_list().contains(e.id))).id)\nprint((staff | e -> filter(false and boom_list().contains(e.id))).id)\n", "[]\n[]"),
		{name: "verb_set_error_at_first_row", files: files, err: "boom", source: glue + queryPre + "r = staff | e -> filter(boom_list().contains(e.id))\n"},
		r("never_true", "print(list ['a'].contains(1))\n", "contains() can't be true here: the elements of list[str] are str, and this value is int"),
		r("never_true_vec", "print(vec [1, 2].contains('a'))\n", "the elements of vec[int] are int, and this value is str"),
		r("never_true_null", "print(vec [1, 2].contains(null))\n", "the elements of vec[int] are int, and this value is null"),
		r("never_true_in_verb", "keep = list ['a']\nr = staff | e -> filter(keep.contains(e.id))\n", "contains() can't be true here"),
		r("structs", "struct P:\n    x: int\nprint(list [P { x: 1 }].contains(1))\n", "contains() compares with ==, which isn't defined for struct values"),
		r("verb_set_of_lists", "ll = list [list [1]]\nr = staff | e -> filter(ll.contains(list [e.id]))\n", "in a query, contains() looks the value up in a set of the list's elements, made once for the step, which holds int, float, bool, str or date values; these elements are list[int]"),
		r("arity", "print(list [1].contains())\n", "method 'contains' expects 1 argument but got 0"),
		r("arity_two", "print(vec [1].contains(1, 2))\n", "method 'contains' expects 1 argument but got 2"),
		r("dynamic_str_int", "fn id(v) -> v\nprint(id('abc').contains(1))\n", "contains() expects (str, str)"),
	})
}

// TestTables_TableInfo is D49: t.nrow(), t.ncol() and t.names(); len(t)
// names nrow.
func TestTables_TableInfo(t *testing.T) {
	files := map[string]string{"staff.csv": queryCSV}
	q := func(name, body, want string) tableCase {
		return tableCase{name: name, files: files, source: queryPre + body, want: want}
	}
	r := func(name, body, err string) tableCase {
		return tableCase{name: name, files: files, source: queryPre + body, err: err}
	}
	runTableCases(t, []tableCase{
		q("counts", "print(staff.nrow())\nprint(staff.ncol())\nprint(staff.names())\n", "5\n4\n['id', 'name', 'dept', 'salary']"),
		q("query_result", "r = staff | e -> filter(e.id > 3) | select(e.name)\nprint(r.nrow())\nprint(r.ncol())\nprint(r.names())\n", "2\n1\n['name']"),
		q("untyped", "fn id(v) -> v\nprint(id(staff).nrow())\nprint(id(staff).names())\n", "5\n['id', 'name', 'dept', 'salary']"),
		q("names_is_str_list", "n = staff.names()\nn.push('x')\nprint(n)\nprint(staff.names())\n", "['id', 'name', 'dept', 'salary', 'x']\n['id', 'name', 'dept', 'salary']"),
		q("in_verb", "print((staff | e -> mutate(k = staff.nrow() - e.id)).k)\n", "[4, 3, 2, 1, 0]"),
		q("empty", "none = staff | e -> filter(e.id > 99)\nprint(none.nrow())\nprint(none.ncol())\n", "0\n4"),
		r("names_push_int", "n = staff.names()\nn.push(1)\n", "list[str]"),
		r("nrow_arg", "print(staff.nrow(1))\n", "method 'nrow' expects 0 arguments but got 1"),
		r("len", "print(len(staff))\n", "len() of a table isn't defined; its row count is staff.nrow()"),
		r("len_untyped", "fn id(v) -> v\nprint(len(id(staff)))\n", "len() of a table isn't defined; its row count is t.nrow()"),
		r("nrow_on_list", "print(list [1].nrow())\n", "type 'list[int]' has no method 'nrow'"),
	})
}

// TestTables_MinMax2 is D51: two-argument min and max are typed by their
// arguments, in and outside verbs.
func TestTables_MinMax2(t *testing.T) {
	files := map[string]string{"staff.csv": queryCSV}
	q := func(name, body, want string) tableCase {
		return tableCase{name: name, files: files, source: queryPre + body, want: want}
	}
	r := func(name, body, err string) tableCase {
		return tableCase{name: name, files: files, source: queryPre + body, err: err}
	}
	runTableCases(t, []tableCase{
		q("typed", "incl 'std/time'\nprint(type(max(1, 2)))\nprint(min(2.5, 1.5))\nprint(max('apple', 'banana'))\nprint(min('b', 'a'))\nprint(min(time::date(2020, 1, 1), time::date(2019, 5, 5)))\nprint(type(max(time::date(2020, 1, 1), time::date(2019, 5, 5))))\n",
			"int\n1.5\nbanana\na\n2019-05-05\ndate"),
		q("mixed_untyped", "x = max(1, 2.5)\nprint(x)\nprint(max(9007199254740993, 1.0))\ny: float = max(1, 2.5)\nprint(y)\n", "2.5\n9007199254740993\n2.5"),
		q("nan_wins", "print(max(1.0, to_float('nan')))\nprint(min(to_float('nan'), 1.0))\n", "nan\nnan"),
		q("in_verb", "r = staff | e -> mutate(m = max(e.salary, 100000.0), n = min(e.name, 'B'), k = max(e.id, 3))\nprint(r.m)\nprint(r.n)\nprint(r.k)\nprint(type(r))\n",
			"[120000.0, 100000.0, null, 150000.0, 100000.0]\n['Ann', 'B', 'B', 'B', 'B']\n[3, 3, 3, 4, 5]\ntbl[{id: int, name: str, dept: str, salary: float, m: float, n: str, k: int}]"),
		q("dynamic", "fn id(v) -> v\nprint(max(id('a'), id('b')))\nprint(min(id(3), id(2.5)))\n", "b\n2.5"),
		r("in_verb_int_float", "r = staff | e -> mutate(m = max(e.salary, 20))\n", "max of an int and a float has no single type, and a column needs one; convert the int with to_float(20)"),
		r("in_verb_int_float_col", "r = staff | e -> mutate(m = min(e.id, e.salary))\n", "convert the int with to_float(e.id)"),
		r("str_int", "print(max(1, 'a'))\n", "'max' compares two values of one type, got int and str"),
		r("bool", "print(min(true, false))\n", "argument 1 of 'min' expects int, float, str or date, got bool"),
		r("dynamic_bool", "fn id(v) -> v\nprint(max(id(true), 1))\n", "max() expects two numbers, two strs or two dates, got bool and int"),
		r("variable_typed", "x = max(1, 2)\nx = 2.5\n", "cannot assign value of type 'float' to 'int'"),
		r("null", "print(min(1, null))\n", "argument 2 of 'min' expects int, float, str or date, got null"),
	})
}

// TestTables_Random is tables v0.3 §5 (D45): a draw in a verb is computed
// from the seed, the loop, the row and the draw's position in the row
// (Philox4x32-10), not taken from the sequence that draws outside verbs use.
func TestTables_Random(t *testing.T) {
	files := map[string]string{"staff.csv": queryCSV}
	pre := "incl 'std/random'\n" + queryPre
	q := func(name, body, want string) tableCase {
		return tableCase{name: name, files: files, source: pre + body, want: want}
	}
	runTableCases(t, []tableCase{
		// The values are the runtime's own algorithms over Philox bits, so
		// they are the same on every platform (CI runs this on Windows too).
		q("fixed_values", "random::seed(42)\nr = staff | e -> mutate(u = random::uniform(), k = random::randint(1, 6), w = random::uniform(10.0, 20.0))\nprint(r.u)\nprint(r.k)\nprint(r.w)\nprint((staff | e -> filter(random::choice(list [1, 2, 3]) == 2)).id)\nprint((staff | e -> mutate(b = random::randint(-9223372036854775807 - 1, 9223372036854775807))).b)\n",
			"[0.4685865183391049, 0.32706338120338474, 0.6583155272123093, 0.6706398529302326, 0.839045447606225]\n[6, 5, 4, 5, 2]\n[18.803964482882158, 12.62144533649401, 12.301875305504868, 17.82020150978942, 12.195637057195112]\n[2]\n[-9154696446549032815, -3463510196736257792, 4154311677657610901, 7660504997838088410, 6923990931457500105]"),
		q("seed_repeats", "random::seed(5)\na = staff | e -> mutate(u = random::uniform(), n = random::normal())\nrandom::seed(5)\nb = staff | e -> mutate(u = random::uniform(), n = random::normal())\nprint(all(a.u == b.u) and all(a.n == b.n))\n", "true"),
		q("two_draws_in_a_row", "random::seed(1)\nr = staff | e -> mutate(d = random::uniform() - random::uniform(), same = random::uniform() == random::uniform())\nprint(sum(r.same))\nprint(all(r.d != 0.0))\n", "0\ntrue"),
		q("each_row_differs", "random::seed(1)\nr = staff | e -> mutate(u = random::uniform())\nprint((r | e -> summarize(k = n_distinct(e.u))).k[0])\n", "5"),
		q("loops_differ", "random::seed(1)\nr = staff | e -> mutate(u = random::uniform(), v = random::uniform())\nprint(sum(r.u == r.v))\ns = staff | e -> summarize(m = max(random::uniform() + to_float(e.id) * 0.0))\nprint(s.m[0] == max(r.u))\n", "0\nfalse"),
		q("aggregate_pass_and_row_loop", "random::seed(3)\nr = staff | e -> filter(random::uniform() + to_float(e.id) * 0.0 < max(random::uniform() + to_float(e.id) * 0.0))\nprint(r.nrow() > 0)\n", "true"),
		q("sequence_unchanged", "random::seed(9)\nx1 = random::uniform()\ny1 = random::uniform()\nrandom::seed(9)\nx2 = random::uniform()\nr = staff | e -> mutate(u = random::uniform(), k = random::randint(1, 9))\ny2 = random::uniform()\nprint(x1 == x2 and y1 == y2)\n", "true"),
		q("helper_draws", "fn jitter(x: float) float -> x + random::uniform(0.0, 0.5)\nrandom::seed(2)\na = staff | e -> mutate(j = jitter(e.salary))\nrandom::seed(2)\nb = staff | e -> mutate(j = jitter(e.salary))\nprint(a.j == b.j)\nprint((a | e -> summarize(k = n_distinct(e.j))).k[0])\n",
			"[true, true, null, true, true]\n5"),
		q("grouped_summarize", "random::seed(4)\na = staff | e -> group_by(e.dept) | summarize(c = random::randint(1, 1000000))\nrandom::seed(4)\nb = staff | e -> group_by(e.dept) | summarize(c = random::randint(1, 1000000))\nprint(all(a.c == b.c))\nprint((a | e -> summarize(k = n_distinct(e.c))).k[0])\n", "true\n3"),
		q("vec_draws_in_a_row", "random::seed(6)\nr = staff | e -> mutate(s = mean(random::uniform_vec(4)), m = to_int(sum(random::randint_vec(3, 1, 1))), z = len(random::sample(list [1, 2, 3], 2)))\nprint(r.m)\nprint(r.z)\nprint((r | e -> summarize(k = n_distinct(e.s))).k[0])\n",
			"[3, 3, 3, 3, 3]\n[2, 2, 2, 2, 2]\n5"),
		q("weighted", "random::seed(8)\nr = staff | e -> filter(random::weighted(list ['a', 'b'], list [0.0, 1.0]) == 'b')\nprint(r.nrow())\n", "5"),
		q("nested_query", "fn inner(k: int) float -> (staff | e -> summarize(m = mean(random::uniform() + to_float(e.id + k) * 0.0))).m[0]\nrandom::seed(7)\na = staff | e -> mutate(x = inner(e.id), y = random::uniform())\nrandom::seed(7)\nb = staff | e -> mutate(x = inner(e.id), y = random::uniform())\nprint(all(a.x == b.x) and all(a.y == b.y))\nprint((a | e -> summarize(k = n_distinct(e.x))).k[0])\n",
			"true\n5"),
		q("missing_row_skips_draw", "random::seed(1)\nr = staff | e -> mutate(u = e.salary * 0.0 + random::uniform())\nprint(r.u.get(2))\n", "null"),
	})
}
