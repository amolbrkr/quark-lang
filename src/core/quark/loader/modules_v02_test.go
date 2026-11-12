package loader

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"quark/ast"
)

// loadProgram writes files (relative path -> source) into a temp dir and runs
// the loader on main.qrk.
func loadProgram(t *testing.T, files map[string]string) (*ModuleLoader, *ast.TreeNode) {
	t.Helper()
	tmp := t.TempDir()
	for rel, src := range files {
		writeFile(t, filepath.Join(tmp, rel), src)
	}
	entry := filepath.Join(tmp, "main.qrk")
	root := parseRoot(t, entry)
	ml := NewModuleLoader()
	ml.ResolveImports(root, entry)
	return ml, root
}

// identifiers lists every identifier literal in the tree.
func identifiers(n *ast.TreeNode, out map[string]bool) map[string]bool {
	if out == nil {
		out = map[string]bool{}
	}
	if n == nil {
		return out
	}
	if n.NodeType == ast.IdentifierNode || n.NodeType == ast.QualifiedNameNode {
		out[n.TokenLiteral()] = true
	}
	for _, c := range n.Children {
		identifiers(c, out)
	}
	return out
}

func TestModulesV02_Resolution(t *testing.T) {
	ml, root := loadProgram(t, map[string]string{
		"main.qrk": "incl './lib/geo'\nincl './lib/other' as o\nfn helper() -> 0\n" +
			"print(geo::area(2), geo::shapes::sq(1), o::helper(), geo::PI, helper())\n" +
			"p = geo::Point { x: 1.0 }\nfn f(q: geo::Point) -> q\n",
		"lib/geo.qrk": "PI = 3.5\nstruct Point:\n    x: float\nfn helper() -> 1\nfn area(r) -> PI * r * helper()\n" +
			"module shapes:\n    fn sq(n) -> n * n\n",
		"lib/other.qrk": "fn helper() -> 2\n",
	})
	if errs := ml.Errors(); len(errs) > 0 {
		t.Fatalf("unexpected errors: %v", errs)
	}
	ids := identifiers(root, nil)
	for _, want := range []string{"q1_geo__area", "q1_geo__shapes__sq", "q2_other__helper", "q1_geo__helper", "helper"} {
		if !ids[want] {
			t.Errorf("expected identifier %q after renaming; have %v", want, ids)
		}
	}
	for name := range ids {
		if strings.Contains(name, "::") {
			t.Errorf("unresolved qualified name %q", name)
		}
	}
	if ids["q1_geo__PI"] || ids["PI"] {
		t.Errorf("constants should be inlined, found a reference: %v", ids)
	}
	// Display names come back as written.
	tbl := ml.Names()
	for internal, want := range map[string]string{
		"q1_geo__area":       "geo::area",
		"q1_geo__shapes__sq": "geo::shapes::sq",
		"q1_geo__Point":      "geo::Point",
		"q2_other__helper":   "other::helper",
		"helper":             "helper",
	} {
		if got := tbl.Display(internal); got != want {
			t.Errorf("Display(%q) = %q, want %q", internal, got, want)
		}
	}
	if got := tbl.DisplayText("expects q1_geo__Point, got q2_other__helper"); got != "expects geo::Point, got other::helper" {
		t.Errorf("DisplayText = %q", got)
	}
}

func TestModulesV02_PrimaryModule(t *testing.T) {
	ml, root := loadProgram(t, map[string]string{
		"main.qrk": "incl './fmtx' as f\nprint(f::show(1), f::raw(2))\n",
		"fmtx.qrk": "fn raw(x) -> x\nmodule fmtx:\n    fn show(x) -> raw(x)\n",
	})
	if errs := ml.Errors(); len(errs) > 0 {
		t.Fatalf("unexpected errors: %v", errs)
	}
	ids := identifiers(root, nil)
	if !ids["q1_fmtx__fmtx__show"] || !ids["q1_fmtx__raw"] {
		t.Fatalf("expected primary-module and top-level names, got %v", ids)
	}
	if got := ml.Names().Display("q1_fmtx__fmtx__show"); got != "fmtx::show" {
		t.Errorf("primary module member displays as %q, want fmtx::show", got)
	}
}

func TestModulesV02_Rejected(t *testing.T) {
	cases := []struct {
		name  string
		files map[string]string
		want  string
	}{
		{"dot_on_import", map[string]string{"main.qrk": "incl './iox' as io\nprint(io.exists('x'))\n", "iox.qrk": "fn exists(p) -> p\n"},
			"'io' is an imported file; use io::exists"},
		{"dot_on_module", map[string]string{"main.qrk": "module m:\n    fn f() -> 1\nprint(m.f())\n"},
			"'m' is a module; use m::f"},
		{"unknown_member", map[string]string{"main.qrk": "incl './lib'\nprint(lib::shw(1))\n", "lib.qrk": "fn show(x) -> x\n"},
			"'lib' (./lib) has no 'shw'; did you mean 'show'?"},
		{"unknown_module_member", map[string]string{"main.qrk": "module m:\n    fn f() -> 1\nprint(m::g())\n"},
			"module 'm' has no 'g'"},
		{"unknown_head", map[string]string{"main.qrk": "print(nope::f())\n"},
			"unknown name 'nope' in 'nope::f'"},
		{"too_many_parts", map[string]string{"main.qrk": "module m:\n    fn f() -> 1\nprint(m::f::g())\n"},
			"'m::f::g' has too many parts"},
		{"not_reexported", map[string]string{"main.qrk": "incl './a'\nprint(a::io::exists('x'))\n", "a.qrk": "incl './iox' as io\nfn f() -> 1\n", "iox.qrk": "fn exists(p) -> p\n"},
			"imports are not re-exported"},
		{"import_vs_definition", map[string]string{"main.qrk": "incl './geo'\nfn geo() -> 1\n", "geo.qrk": "fn f() -> 1\n"},
			"conflicts with import 'geo' on line 1; incl './geo' as other_name"},
		{"import_vs_import", map[string]string{"main.qrk": "incl './a/geo'\nincl './b/geo'\n", "a/geo.qrk": "fn f() -> 1\n", "b/geo.qrk": "fn g() -> 1\n"},
			"import 'geo' (line 2) conflicts with the import 'geo'"},
		{"member_vs_top", map[string]string{"main.qrk": "fn read() -> 1\nmodule csv:\n    fn read() -> 2\n"},
			"module member 'csv::read' has the same name as the top-level function 'read' on line 1"},
		{"module_twice", map[string]string{"main.qrk": "module m:\n    fn f() -> 1\nmodule m:\n    fn g() -> 1\n"},
			"module 'm' is already defined on line 1"},
		{"struct_twice", map[string]string{"main.qrk": "struct P:\n    x: int\nstruct P:\n    y: int\n"},
			"'P' is already defined on line 1"},
		{"fn_and_struct", map[string]string{"main.qrk": "struct P:\n    x: int\nfn P() -> 1\n"},
			"'P' is already defined on line 1"},
		{"lib_statement", map[string]string{"main.qrk": "incl './lib'\n", "lib.qrk": "fn f() -> 1\nprint(f())\n"},
			"'lib.qrk' is imported, so it can only contain definitions; line 2 is a statement (print(...))"},
		{"lib_statement_in_module", map[string]string{"main.qrk": "incl './lib'\n", "lib.qrk": "module lib:\n    for i in range(3):\n        print(i)\n"},
			"line 2 is a statement (for)"},
		{"lib_mutable_constant", map[string]string{"main.qrk": "incl './lib'\n", "lib.qrk": "CACHE = list []\n"},
			"line 1 assigns 'CACHE' a value that isn't a constant"},
		{"lib_computed_constant", map[string]string{"main.qrk": "incl './lib'\n", "lib.qrk": "fn f() -> 1\nX = f()\n"},
			"line 2 assigns 'X' a value that isn't a constant"},
		{"lib_constant_assigned", map[string]string{"main.qrk": "incl './lib'\n", "lib.qrk": "X = 1\nfn f() ->\n    X = 2\n    X\n"},
			"'X' is a constant (line 1) and can't be assigned"},
		{"lib_constant_twice", map[string]string{"main.qrk": "incl './lib'\n", "lib.qrk": "X = 1\nX = 2\n"},
			"'X' is already defined on line 1"},
		{"imported_name_assigned", map[string]string{"main.qrk": "incl './lib'\nlib::f = 3\n", "lib.qrk": "fn f() -> 1\n"},
			"lib.qrk and can't be assigned here"},
		{"import_binding_assigned_top", map[string]string{"main.qrk": "incl './lib'\nlib = 3\n", "lib.qrk": "fn f() -> 1\n"},
			"the variable 'lib' (line 2) conflicts with import 'lib' on line 1"},
		{"import_binding_assigned_in_fn", map[string]string{"main.qrk": "incl './lib'\nfn g() ->\n    lib = 3\n    lib\n", "lib.qrk": "fn f() -> 1\n"},
			"'lib' is an imported file (line 1) and can't be assigned"},
		{"import_binding_as_value", map[string]string{"main.qrk": "incl './lib'\nx = lib\n", "lib.qrk": "fn f() -> 1\n"},
			"'lib' is an imported file, not a value; use lib::name"},
		{"no_leak_from_importer", map[string]string{"main.qrk": "counter = 1\nincl './lib'\n", "lib.qrk": "fn f() -> counter\n"},
			"undefined identifier 'counter'"},
		{"no_leak_struct_from_importer", map[string]string{"main.qrk": "struct P:\n    x: int\nincl './lib'\n", "lib.qrk": "fn f(p: P) -> p\n"},
			"unknown type 'P'"},
		{"no_leak_struct_literal", map[string]string{"main.qrk": "struct P:\n    x: int\nincl './lib'\n", "lib.qrk": "fn f() -> P { x: 1 }\n"},
			"unknown struct type 'P'"},
		{"not_a_struct", map[string]string{"main.qrk": "incl './lib'\nfn g(x: lib::f) -> x\n", "lib.qrk": "fn f() -> 1\n"},
			"'lib::f' is a function, not a struct type"},
		{"bad_file_name", map[string]string{"main.qrk": "incl './GeoUtils'\n", "GeoUtils.qrk": "fn f() -> 1\n"},
			"module file name 'GeoUtils' must be lowercase letters, digits and underscores (e.g. geoutils)"},
		{"bad_dir_name", map[string]string{"main.qrk": "incl './My-Lib/geo'\n", "My-Lib/geo.qrk": "fn f() -> 1\n"},
			"directory name 'My-Lib' in import path './My-Lib/geo' must be lowercase letters"},
		{"nested_incl", map[string]string{"main.qrk": "fn f() ->\n    incl './lib'\n    1\n", "lib.qrk": "fn g() -> 1\n"},
			"incl must be at the top level of a file"},
		{"internal_name_reference", map[string]string{"main.qrk": "module mathx:\n    fn square(x) -> x * x\nprint(mathx__square(2))\n"},
			"'mathx__square': names can't contain '__'"},
		{"internal_name_in_lib", map[string]string{"main.qrk": "incl './lib'\n", "lib.qrk": "fn f__thunk() -> 1\n"},
			"'f__thunk': names can't contain '__'"},
		{"internal_name_qualified", map[string]string{"main.qrk": "incl './lib'\nprint(lib::q1_lib__f())\n", "lib.qrk": "fn f() -> 1\n"},
			"names can't contain '__'"},
		{"local_used_as_module", map[string]string{"main.qrk": "fn f(io) -> io::exists('x')\n"},
			"'io' is a local variable, not a module or imported file"},
		// m + _x and m_ + x would both be m___x.
		{"internal_name_clash_members", map[string]string{"main.qrk": "module m:\n    _x = 1\nmodule m_:\n    x = 2\nprint(m::_x)\n"},
			"'m_::x' and 'm::_x' (line 2) can't both be defined: the compiler would give them the same internal name"},
		{"internal_name_clash_lib_members", map[string]string{"main.qrk": "incl './lib'\n", "lib.qrk": "module m:\n    fn _f() -> 1\nmodule m_:\n    fn f() -> 2\n"},
			"'lib::m_::f' and 'lib::m::_f' (line 2) can't both be defined"},
		// Members of main-script module q1_a would take the names of a.qrk (file 1).
		{"internal_name_clash_across_files", map[string]string{"main.qrk": "incl './a'\nmodule q1_a:\n    fn f() -> 'main'\n", "a.qrk": "fn f() -> 'a'\n"},
			"'q1_a::f' and 'a::f' ("},
		{"nested_module_in_main", map[string]string{"main.qrk": "module outer:\n    module inner:\n        fn f() -> 1\n    print('outer')\n"},
			"module 'inner' is inside module 'outer'; modules can't be nested"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ml, _ := loadProgram(t, tc.files)
			errs := strings.Join(ml.Errors(), "\n")
			if !strings.Contains(errs, tc.want) {
				t.Fatalf("expected error containing %q, got:\n%s", tc.want, errs)
			}
		})
	}
}

// TestModulesV02_LocalsOfImportedFiles: a local of a library function named
// like one of the main script's names gets its own internal name, so the
// importer's definitions are never visible to it (rule 12); other locals keep
// their names. The display table shows the renamed local as written.
func TestModulesV02_LocalsOfImportedFiles(t *testing.T) {
	ml, root := loadProgram(t, map[string]string{
		"main.qrk": "incl './lib'\nfn total() -> 100\nprint(lib::g(1))\n",
		"lib.qrk":  "fn g(n) ->\n    total = n\n    other = 2\n    total + other\n",
	})
	if errs := ml.Errors(); len(errs) > 0 {
		t.Fatalf("unexpected errors: %v", errs)
	}
	ids := identifiers(root, nil)
	if !ids["q1__total__l"] || !ids["other"] || !ids["total"] {
		t.Fatalf("expected the library's local 'total' renamed to q1__total__l, 'other' kept and main's 'total' kept; have %v", ids)
	}
	if got := ml.Names().Display("q1__total__l"); got != "total" {
		t.Errorf("Display(q1__total__l) = %q, want %q", got, "total")
	}
}

// TestModulesV02_SymlinkIsOneFile: a file reached through a symbolic link and
// through its own path is one file (rule 6), whatever name the link has; each
// import binds the stem written in its path.
func TestModulesV02_SymlinkIsOneFile(t *testing.T) {
	tmp := t.TempDir()
	writeFile(t, filepath.Join(tmp, "geo.qrk"), "struct P:\n    x: int\nfn f() -> P { x: 1 }\n")
	if err := os.Symlink("geo.qrk", filepath.Join(tmp, "geo2.qrk")); err != nil {
		t.Skipf("symbolic links not available: %v", err)
	}
	writeFile(t, filepath.Join(tmp, "main.qrk"), "incl './geo'\nincl './geo2'\nprint(geo::f(), geo2::f())\n")
	entry := filepath.Join(tmp, "main.qrk")
	root := parseRoot(t, entry)
	ml := NewModuleLoader()
	ml.ResolveImports(root, entry)
	if errs := ml.Errors(); len(errs) > 0 {
		t.Fatalf("unexpected errors: %v", errs)
	}
	if len(ml.files) != 2 {
		t.Fatalf("expected main and one library file, got %d files", len(ml.files))
	}
	ids := identifiers(root, nil)
	if !ids["q1_geo__f"] || len(ml.files[0].imports) != 2 || ml.files[0].imports[1].name != "geo2" {
		t.Fatalf("expected both imports to reach q1_geo__f, the second bound as geo2; have %v", ids)
	}
}

func TestModulesV02_FileIDsAndIdentity(t *testing.T) {
	// base.qrk is imported directly and through left.qrk: one file, one id.
	ml, root := loadProgram(t, map[string]string{
		"main.qrk": "incl './left'\nincl './base'\nprint(base::f(), left::g())\n",
		"left.qrk": "incl './base'\nfn g() -> base::f()\n",
		"base.qrk": "struct P:\n    x: int\nfn f() -> P { x: 1 }\n",
	})
	if errs := ml.Errors(); len(errs) > 0 {
		t.Fatalf("unexpected errors: %v", errs)
	}
	if len(ml.files) != 3 {
		t.Fatalf("expected 3 files, got %d", len(ml.files))
	}
	structs := 0
	for _, c := range root.Children {
		if c.NodeType == ast.StructDefNode {
			structs++
			if c.TokenLiteral() != "q2_base__P" {
				t.Errorf("struct internal name %q, want q2_base__P", c.TokenLiteral())
			}
		}
	}
	if structs != 1 {
		t.Fatalf("expected one struct definition, got %d", structs)
	}
}
