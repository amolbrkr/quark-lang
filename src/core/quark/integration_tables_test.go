package main

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// Tables v0.1 (tables_v0_1_spec.md): the tbl type, std/csv on the table
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
			want: "[2.5]\nid,name,dept,salary\n1,a,b,2.5"},
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
