package main

// Soundness differentials (architecture.md §12): a proven raw read checked
// at runtime, a query against the vec operation on the same column, and an
// expression against the same value stored in a variable. A difference is a
// soundness bug: the two sides are the same values, so they must agree.

import (
	"bytes"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// buildDirRE matches the temporary directory of one quark run. Clang names it
// when a generated file fails to compile, and each run gets its own.
var buildDirRE = regexp.MustCompile(`quark-(?:run|build)-\d+`)

func scrubBuildDir(s string) string {
	return buildDirRE.ReplaceAllString(s, "quark-DIR")
}

type captured struct {
	stdout string
	stderr string
	exit   int
}

func captureQuark(dir string, args ...string) captured {
	cmd := exec.Command(quarkExePath, args...)
	cmd.Dir = dir
	var out, errBuf bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &errBuf
	err := cmd.Run()
	got := captured{
		stdout: normalizeNewlines(out.String()),
		stderr: normalizeNewlines(errBuf.String()),
	}
	if err == nil {
		return got
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		got.exit = exitErr.ExitCode()
		return got
	}
	got.exit = -1
	got.stderr += err.Error()
	return got
}

// agreeModes runs the program with and without --checked. want is the
// expected stdout when errSub is empty; otherwise both runs must fail with
// that substring. The two runs must match each other either way.
func agreeModes(t *testing.T, source, want, errSub string) {
	t.Helper()
	dir := t.TempDir()
	program := filepath.Join(dir, "main.qrk")
	if err := os.WriteFile(program, []byte(source), 0o644); err != nil {
		t.Fatal(err)
	}
	normal := captureQuark(dir, "run", program)
	checked := captureQuark(dir, "run", "--checked", program)
	if scrubBuildDir(normal.stdout) != scrubBuildDir(checked.stdout) || scrubBuildDir(normal.stderr) != scrubBuildDir(checked.stderr) || normal.exit != checked.exit {
		t.Fatalf("--checked disagreed with the default build\n--- default (exit %d) ---\nstdout:\n%s\nstderr:\n%s\n--- checked (exit %d) ---\nstdout:\n%s\nstderr:\n%s",
			normal.exit, normal.stdout, normal.stderr, checked.exit, checked.stdout, checked.stderr)
	}
	if errSub != "" {
		if normal.exit == 0 {
			t.Fatalf("expected failure containing %q, got stdout:\n%s", errSub, normal.stdout)
		}
		if !strings.Contains(normal.stdout+normal.stderr, errSub) {
			t.Fatalf("expected error containing %q, got stdout:\n%s\nstderr:\n%s", errSub, normal.stdout, normal.stderr)
		}
		return
	}
	if normal.exit != 0 {
		t.Fatalf("run failed (exit %d)\nstdout:\n%s\nstderr:\n%s", normal.exit, normal.stdout, normal.stderr)
	}
	got := strings.TrimSpace(normal.stdout)
	if got != strings.TrimSpace(want) {
		t.Fatalf("unexpected output\n--- got ---\n%s\n--- want ---\n%s", got, strings.TrimSpace(want))
	}
}

// TestCheckedMode_SmokeAgrees runs every smoke program both ways. Compile
// errors and runtime errors must come out the same; a proven read of the
// wrong tag fails only the checked build.
func TestCheckedMode_SmokeAgrees(t *testing.T) {
	root := repoRootFromThisFile(t)
	files, err := filepath.Glob(filepath.Join(root, "src", "testfiles", "smoke_*.qrk"))
	if err != nil {
		t.Fatal(err)
	}
	if len(files) == 0 {
		t.Fatal("no smoke programs")
	}
	for _, file := range files {
		file := file
		t.Run(filepath.Base(file), func(t *testing.T) {
			t.Parallel()
			dir := t.TempDir()
			normal := captureQuark(dir, "run", file)
			checked := captureQuark(dir, "run", "--checked", file)
			if scrubBuildDir(normal.stdout) != scrubBuildDir(checked.stdout) || scrubBuildDir(normal.stderr) != scrubBuildDir(checked.stderr) || normal.exit != checked.exit {
				t.Fatalf("--checked disagreed\n--- default (exit %d) ---\nstdout:\n%s\nstderr:\n%s\n--- checked (exit %d) ---\nstdout:\n%s\nstderr:\n%s",
					normal.exit, normal.stdout, normal.stderr, checked.exit, checked.stdout, checked.stderr)
			}
		})
	}
}

// The staff table shared by the query agreement cases: one missing salary
// (Cy) and one missing dept (Ed).
const agreeStaff = "incl 'std/csv'\nincl 'std/stats'\n" +
	"struct Emp:\n    id: int\n    name: str\n    dept: str\n    salary: float\n" +
	"staff: tbl[Emp] = unwrap(csv::parse_text('id,name,dept,salary\\n" +
	"1,Ann,eng,120000\\n2,Bo,ops,90000.5\\n3,Cy,eng,\\n4,Di,ops,150000\\n5,Ed,,80000\\n'))\n" +
	"module m:\n    fn total(t: tbl[Emp]) -> sum(t.salary)\n"

// TestQueryAgreesWithVec requires a query to compute what the vec operation
// computes on the same column. summarize calls the engine with a column
// index; filter and mutate are loops the compiler emits. The vec side is
// the reference, so a wrong index or a dropped validity test fails here
// even when both kernels are right.
func TestQueryAgreesWithVec(t *testing.T) {
	cases := []struct{ name, source, want, err string }{
		{
			name: "reductions",
			source: agreeStaff + `
print(sum(staff.salary))
print((staff | e -> summarize(s = sum(e.salary))).s[0])
print(m::total(staff))
print(sum(staff.id))
print((staff | e -> summarize(s = sum(e.id))).s[0])
print(stats::mean(staff.salary))
print((staff | e -> summarize(m = mean(e.salary))).m[0])
print(min(staff.salary))
print((staff | e -> summarize(lo = min(e.salary))).lo[0])
print(max(staff.salary))
print((staff | e -> summarize(hi = max(e.salary))).hi[0])
print(min(staff.name))
print((staff | e -> summarize(lo = min(e.name))).lo[0])
print(max(staff.name))
print((staff | e -> summarize(hi = max(e.name))).hi[0])
`,
			want: `
440000.5
440000.5
440000.5
15
15
110000.125
110000.125
80000.0
80000.0
150000.0
150000.0
Ann
Ann
Ed
Ed
`,
		},
		{
			name: "groups",
			source: agreeStaff + `
eng = sum(staff.salary[staff.dept == 'eng'])
ops = sum(staff.salary[staff.dept == 'ops'])
gap = sum(staff.salary[staff.dept.fillna('?') == '?'])
want = vec [eng, ops, gap]
got = (staff | e -> group_by(e.dept) | summarize(s = sum(e.salary))).s
print(want)
print(got)
print(sum(want == got) == len(got))
engm = stats::mean(staff.salary[staff.dept == 'eng'])
opsm = stats::mean(staff.salary[staff.dept == 'ops'])
gapm = stats::mean(staff.salary[staff.dept.fillna('?') == '?'])
wantm = vec [engm, opsm, gapm]
gotm = (staff | e -> group_by(e.dept) | summarize(m = mean(e.salary))).m
print(sum(wantm == gotm) == len(gotm))
print(vec [sum(staff.id[staff.dept == 'eng']), sum(staff.id[staff.dept == 'ops']), sum(staff.id[staff.dept.fillna('?') == '?'])])
print((staff | e -> group_by(e.dept) | summarize(s = sum(e.id))).s)
`,
			want: `
[120000.0, 240000.5, 80000.0]
[120000.0, 240000.5, 80000.0]
true
true
[4, 6, 5]
[4, 6, 5]
`,
		},
		{
			name: "bool_and_empty",
			source: agreeStaff + `
rich = (staff | e -> mutate(rich = e.salary > 100000.0)).rich
print(sum(rich))
print((staff | e -> mutate(rich = e.salary > 100000.0) | summarize(s = sum(e.rich))).s[0])
print(stats::mean(rich))
print((staff | e -> mutate(rich = e.salary > 100000.0) | summarize(m = mean(e.rich))).m[0])
none = staff | e -> filter(e.id < 0)
print(sum(none.id))
print((none | e -> summarize(s = sum(e.id))).s[0])
print((none | e -> summarize(m = mean(e.salary))).m)
`,
			want: `
2
2
0.5
0.5
0
0
[null]
`,
		},
		{
			name: "filter_and_sort",
			source: agreeStaff + `
print((staff | e -> filter(e.salary > 100000.0)).id.to_list() == staff.id[staff.salary > 100000.0].to_list())
print(staff.id[staff.salary > 100000.0])
print((staff | e -> filter(e.dept == 'eng')).name.to_list() == staff.name[staff.dept == 'eng'].to_list())
print(staff.name[staff.dept == 'eng'])
print((staff | e -> sort(desc(e.name))).name.to_list() == staff.name.to_list().sort().reverse())
print((staff | e -> sort(desc(e.name))).name)
ordered = (staff | e -> sort(e.salary)).salary
print(ordered.drop_na().to_list() == staff.salary.drop_na().to_list().sort())
print(staff.salary.drop_na().to_list().sort())
print(ordered.get(4))
print((staff | e -> sort(e.salary)).id.to_list().sort() == staff.id.to_list().sort())
print((staff | e -> sort(e.dept, desc(e.id))).id.to_list().sort() == staff.id.to_list().sort())
print((staff | e -> sort(e.dept, desc(e.id))).name)
`,
			want: `
true
[1, 4]
true
['Ann', 'Cy']
true
['Ed', 'Di', 'Cy', 'Bo', 'Ann']
true
[80000.0, 90000.5, 120000.0, 150000.0]
null
true
true
['Cy', 'Ann', 'Di', 'Bo', 'Ed']
`,
		},
		{
			name: "mutate_nan_relabel",
			source: agreeStaff + `
bonus = (staff | e -> mutate(bonus = e.salary * 0.1)).bonus
print(bonus)
print(staff.salary * 0.1)
print(sum(bonus) == (staff | e -> mutate(bonus = e.salary * 0.1) | summarize(s = sum(e.bonus))).s[0])
nanrow = staff | e -> mutate(z = to_float('nan') if e.id == 1 else e.salary)
print(sum(nanrow.z))
print((nanrow | e -> summarize(s = sum(e.z))).s[0])
struct Raw:
    salary: int
    name: str
    id: int
    dept: str
raw: tbl[Raw] = unwrap(csv::parse_text('salary,name,id,dept\n5,a,1,b\n7,c,2,d\n'))
anon = raw | e -> mutate(salary = e.salary + 0)
print(sum(anon.salary))
v: tbl[Emp] = anon
print(sum(v.salary))
print((v | e -> summarize(s = sum(e.salary))).s[0])
`,
			want: `
[12000.0, 9000.050000000001, null, 15000.0, 8000.0]
[12000.0, 9000.050000000001, null, 15000.0, 8000.0]
true
320000.5
320000.5
12
12.0
12.0
`,
		},
		{
			name: "all_missing_group",
			source: "incl 'std/csv'\nstruct G:\n    k: str\n    v: int\n" +
				"t: tbl[G] = unwrap(csv::parse_text('k,v\\na,\\na,\\nb,3\\n'))\n" +
				"print(sum(t.v[t.k == 'a']))\nprint(sum(t.v[t.k == 'b']))\n" +
				"print((t | e -> group_by(e.k) | summarize(s = sum(e.v))).s)\n" +
				"print((t | e -> group_by(e.k) | summarize(lo = min(e.v))).lo)\n",
			want: "0\n3\n[0, 3]\n[null, 3]",
		},
		{
			name: "overflow_vec",
			source: "incl 'std/csv'\nstruct R:\n    k: int\n" +
				"t: tbl[R] = unwrap(csv::parse_text('k\\n9223372036854775807\\n1\\n'))\nprint(sum(t.k))\n",
			err: "integer overflow in 'sum'",
		},
		{
			name: "overflow_query",
			source: "incl 'std/csv'\nstruct R:\n    k: int\n" +
				"t: tbl[R] = unwrap(csv::parse_text('k\\n9223372036854775807\\n1\\n'))\n" +
				"print((t | e -> summarize(s = sum(e.k))).s)\n",
			err: "integer overflow in 'sum'",
		},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			agreeModes(t, tc.source, tc.want, tc.err)
		})
	}
}

// TestStoreAgreesWithBoxed: print(E) boxes the value, and x = E stores it as
// the type the compiler claimed. The two prints must match, including a
// typed declaration (n: int = E) and a loop variable read with an element proof.
func TestStoreAgreesWithBoxed(t *testing.T) {
	source := `
print(10 + 20)
x = 10 + 20
print(x)
n: int = 10 + 20
print(n)

print(7 % 4)
m = 7 % 4
print(m)
m2: int = 7 % 4
print(m2)

print(8 / 2)
d = 8 / 2
print(d)
d2: float = 8 / 2
print(d2)

print(1.25 * 4.0)
h = 1.25 * 4.0
print(h)
h2: float = 1.25 * 4.0
print(h2)

print(-3)
neg = -3
print(neg)
neg2: int = -3
print(neg2)

print(2 < 2.5)
c = 2 < 2.5
print(c)
c2: bool = 2 < 2.5
print(c2)

xs = list [10, 20, 30]
print(xs[0])
a: int = xs[0]
print(a)
print(xs[0] + xs[1])
s = xs[0] + xs[1]
print(s)
s2: int = xs[0] + xs[1]
print(s2)

ys = list [1.5, 2.5]
print(ys[1])
f: float = ys[1]
print(f)
g: float = xs[0]
print(g)
print(10.0)

bs = list [true, false]
print(bs[0])
b: bool = bs[0]
print(b)

fn twice(k) -> k + k
print(twice(21))
t = twice(21)
print(t)
t2: int = twice(21)
print(t2)

total = 0
for v in xs:
    total = total + v
print(total)
print(10 + 20 + 30)

fs = 0.0
for v in ys:
    fs = fs + v
print(fs)
print(1.5 + 2.5)
`
	want := `
30
30
30
3
3
3
4.0
4.0
4.0
5.0
5.0
5.0
-3
-3
-3
true
true
true
10
10
30
30
30
2.5
2.5
10.0
10.0
true
true
42
42
42
60
60
4.0
4.0
`
	agreeModes(t, source, want, "")
}
