package main

import (
	"bytes"
	"context"
	"fmt"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	goruntime "runtime"
	"strings"
	"testing"
	"time"
)

// Tables v0.3 §8 (specs/tables_v0_3_spec.md): parallel execution. A program
// prints the same output, and fails with the same error, at every thread
// count (D53, D56). The tables are large enough to be split into blocks of
// 65,536 rows (D55): 131,071 rows run on one thread, 131,072 and more are
// split, and 300,000 ends mid-block.

// parThreads are the thread counts every program runs at (§11).
var parThreads = []int{1, 2, 3, 7, 16}

// parRows are the table sizes: one under the split, the split, one over,
// and one that ends mid-block.
var parRows = []int{131071, 131072, 131073, 300000}

// parStruct is the row of parCSV.
const parStruct = "struct P:\n    i: int\n    x: float\n    s: str\n    b: bool\n    g: str\n    d: date\n"

// parCSV is n rows with every column type and missing values in x and s.
func parCSV(n int) string {
	var b strings.Builder
	b.WriteString("i,x,s,b,g,d\n")
	base := time.Date(2020, 1, 1, 0, 0, 0, 0, time.UTC)
	for i := 1; i <= n; i++ {
		x := ""
		if i%97 != 0 {
			x = fmt.Sprintf("%g", float64((i*7919)%100003)/37.0)
		}
		s := ""
		if i%101 != 0 {
			s = fmt.Sprintf("s%d", (i*31)%1009)
		}
		fmt.Fprintf(&b, "%d,%s,%s,%t,g%d,%s\n", i, x, s, i%3 == 0, i%5, base.AddDate(0, 0, i%1000).Format("2006-01-02"))
	}
	return b.String()
}

// parRun is one run of a built program.
type parRun struct {
	stdout, stderr string
	code           int
}

// buildPar builds source in dir (with files) and returns the executable.
func buildPar(t *testing.T, dir, source string, files map[string]string) string {
	t.Helper()
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	program := filepath.Join(dir, "main.qrk")
	if err := os.WriteFile(program, []byte(source), 0o644); err != nil {
		t.Fatal(err)
	}
	exe := filepath.Join(dir, "main")
	cmd := exec.Command(quarkExePath, "build", program, "-o", exe)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("build failed: %v\n%s", err, out)
	}
	if _, err := os.Stat(exe); err != nil {
		exe += ".exe"
	}
	return exe
}

// runPar runs exe in dir with the environment additions env.
func runPar(t *testing.T, exe, dir string, env ...string) parRun {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, exe)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), env...)
	var out, errBuf bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errBuf
	err := cmd.Run()
	if ctx.Err() != nil {
		t.Fatalf("%s did not finish: %v\nstderr: %s", exe, ctx.Err(), errBuf.String())
	}
	r := parRun{stdout: normalizeNewlines(out.String()), stderr: normalizeNewlines(errBuf.String())}
	if exitErr, ok := err.(*exec.ExitError); ok {
		r.code = exitErr.ExitCode()
	} else if err != nil {
		t.Fatalf("running %s: %v", exe, err)
	}
	return r
}

// sameAtEveryCount runs exe in dir at every thread count and requires the
// runs to match the one-thread run; it returns that run.
func sameAtEveryCount(t *testing.T, exe, dir string) parRun {
	t.Helper()
	var first parRun
	for i, n := range parThreads {
		r := runPar(t, exe, dir, fmt.Sprintf("QUARK_THREADS=%d", n))
		if i == 0 {
			first = r
			continue
		}
		if r != first {
			t.Fatalf("%d threads differ from 1 thread\n--- 1 thread (exit %d) ---\n%s%s\n--- %d threads (exit %d) ---\n%s%s",
				n, first.code, first.stdout, first.stderr, n, r.code, r.stdout, r.stderr)
		}
	}
	return first
}

// parQueries exercises every row loop of a query: filter (dense and with
// missing values), mutate of each column type, whole-table and grouped
// aggregates in filter and mutate, aggregates of expressions, summarize
// columns, computed keys, the count and drop_na steps, == null and fillna,
// a contains() set, calls made once per step, draws, and a query inside a
// function a verb calls. Floats print in shortest round-trip form, so equal
// text is equal bits.
const parQueries = "incl 'std/csv'\nincl 'std/random'\n" + parStruct + `
t: tbl[P] = unwrap(csv::read_file('p.csv'))
small = t | e -> head(20)
fn band(x: float) str -> 'hi' if x > 1000.0 else 'lo'
fn inner(k: int) float -> (small | e -> summarize(m = mean(random::uniform() + to_float(e.i + k) * 0.0))).m[0]
keep = list ['s3', 's7', 's11', 's500']

f = t | e -> filter(e.x > mean(e.x) and e.b)
print(list [f.nrow(), sum(f.i), sum(f.x)])
fd = t | e -> filter(e.i % 7 == 3)
print(list [fd.nrow(), sum(fd.i)])
print(fd | e -> head(3))
print(fd | e -> tail(3))

m = t | e -> mutate(y = e.x * 2.5 - to_float(e.i), z = e.s + '|' + e.g, w = e.i % 7 == 0, dd = e.d, k = e.i * 3, bd = band(e.x))
print(list [m.nrow(), sum(m.y), sum(m.w), sum(m.k)])
print(m | e -> head(3))
print(m | e -> tail(3))
zz = m | e -> mutate(q = to_float(e.i) * to_float(len(e.z)))
print(sum(zz.q))

gm = t | e -> group_by(e.g) | mutate(dev = e.x - mean(e.x), share = to_float(e.i) / to_float(sum(e.i)))
print(list [sum(gm.dev), sum(gm.share)])
gf = t | e -> group_by(e.g) | filter(e.x == max(e.x))
print(gf)
ex = t | e -> summarize(a = sum(e.x * 2.0), b = mean(e.x > 500.0), c = n_distinct(e.s), sp = max(e.x) - min(e.x))
print(ex)
s = t | e -> group_by(e.g, big = e.i > 150000) | summarize(n = count(), sx = sum(e.x * 2.0), mx = max(e.s), lab = e.g + '!')
print(s)
print(t | e -> count(e.g, odd = e.i % 2 == 1))

dn = t | e -> drop_na(e.x, e.s)
print(list [dn.nrow(), sum(dn.i)])
nm = t | e -> filter(e.x == null)
print(list [nm.nrow(), sum(nm.i)])
fl = t | e -> mutate(x2 = e.x.fillna(mean(e.x)), s2 = e.s.fillna('none'))
print(list [sum(fl.x2), sum(fl.s2 == 'none')])
lk = t | e -> filter(keep.contains(e.s))
print(list [lk.nrow(), sum(lk.i)])
oc = t | e -> filter(e.x > mean(t.x) / 2.0)
print(oc.nrow())

random::seed(11)
r = t | e -> mutate(u = random::uniform(), n = random::randint(1, 100), v = random::normal())
print(list [sum(r.u), sum(r.n)])
print(r | e -> head(2))
rs = t | e -> group_by(e.g) | summarize(c = random::randint(1, 1000000))
print(rs.c)
nq = t | e -> filter(e.i % 1000 == 0) | mutate(z = inner(e.i))
print(sum(nq.z))
`

// TestParallel_SameOutput: every row loop of a query prints the same output
// at 1, 2, 3, 7 and 16 threads, on tables under, at and over the split.
func TestParallel_SameOutput(t *testing.T) {
	buildDir := t.TempDir()
	exe := buildPar(t, buildDir, parQueries, nil)
	outputs := map[int]string{}
	for _, n := range parRows {
		n := n
		t.Run(fmt.Sprint(n), func(t *testing.T) {
			dir := t.TempDir()
			if err := os.WriteFile(filepath.Join(dir, "p.csv"), []byte(parCSV(n)), 0o644); err != nil {
				t.Fatal(err)
			}
			r := sameAtEveryCount(t, exe, dir)
			if r.code != 0 {
				t.Fatalf("exit %d\nstdout: %s\nstderr: %s", r.code, r.stdout, r.stderr)
			}
			outputs[n] = r.stdout
		})
	}
	// The sizes differ, so their outputs must too: a guard against a run
	// that printed nothing useful.
	if outputs[131072] == outputs[300000] {
		t.Fatalf("outputs for 131072 and 300000 rows are the same:\n%s", outputs[300000])
	}
}

// parErrors are programs that fail in more than one block: the error is the
// one of the first failing row, as one thread reports it (D56).
var parErrors = []struct {
	name, body, want string
}{
	{"mutate_two_blocks", "fn check(x: int) int -> unwrap(err('bad ' + to_str(x))) if x == 150000 or x == 270000 else x\nr = t | e -> mutate(y = check(e.i))\nprint(r.nrow())\n", "bad 150000"},
	{"filter_last_block_first", "fn check(x: int) int -> unwrap(err('bad ' + to_str(x))) if x == 299999 or x == 70000 else x\nr = t | e -> filter(check(e.i) > 0)\nprint(r.nrow())\n", "bad 70000"},
	{"overflow_location", "r = t | e -> mutate(y = e.i * 1000000000000000 if e.i > 200000 else 0)\nprint(r.nrow())\n", "integer overflow in '*' at "},
	{"aggregate_pass", "fn check(x: int) int -> unwrap(err('bad ' + to_str(x))) if x == 140000 or x == 260000 else x\nr = t | e -> summarize(s = sum(check(e.i)))\nprint(r)\n", "bad 140000"},
	{"once_fails_after_row_error", "fn check(x: int) int -> unwrap(err('bad ' + to_str(x))) if x == 150000 else x\nr = t | e -> filter(check(e.i) > 0 and (e.i < 140000 or boom() > 0))\nprint(r.nrow())\n", "boom"},
	{"row_error_before_once", "fn check(x: int) int -> unwrap(err('bad ' + to_str(x))) if x == 130000 else x\nr = t | e -> filter(check(e.i) > 0 and (e.i < 140000 or boom() > 0))\nprint(r.nrow())\n", "bad 130000"},
	{"stack_overflow_on_worker", "fn deep(n: int) int -> deep(n + 1) + 1 if n > 0 else 0\nr = t | e -> mutate(y = deep(1) if e.i == 250000 else 0)\nprint(r.nrow())\n", "stack overflow"},
}

// TestParallel_Errors: a failing step reports the first failing row's error
// at every thread count, and the program exits with status 1.
func TestParallel_Errors(t *testing.T) {
	tick := "extern 'tick.hpp'\nextern rowsafe fn boom() int as 'q_rowsafe_boom'\n"
	files := map[string]string{"tick.hpp": rowSafeTickH, "p.csv": parCSV(300000)}
	for _, tc := range parErrors {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			dir := t.TempDir()
			src := "incl 'std/csv'\n" + tick + parStruct + "t: tbl[P] = unwrap(csv::read_file('p.csv'))\nprint('before')\n" + tc.body
			exe := buildPar(t, dir, src, files)
			r := sameAtEveryCount(t, exe, dir)
			if r.code != 1 || !strings.Contains(r.stderr, tc.want) || !strings.Contains(r.stderr, "main.qrk:") {
				t.Fatalf("want exit 1 and an error containing %q at a location, got exit %d\nstdout: %s\nstderr: %s", tc.want, r.code, r.stdout, r.stderr)
			}
			if strings.Count(r.stderr, "error[") != 1 {
				t.Fatalf("want one error, got:\n%s", r.stderr)
			}
			if !strings.HasPrefix(r.stdout, "before") {
				t.Fatalf("stdout lost the line printed before the error:\n%s", r.stdout)
			}
		})
	}
}

// TestParallel_OnceAcrossThreads: a call the rows share is made once per
// step at every thread count (D44), counted by a rowsafe glue function.
func TestParallel_OnceAcrossThreads(t *testing.T) {
	dir := t.TempDir()
	tick := "extern 'tick.hpp'\nextern rowsafe fn tick() int as 'q_rowsafe_tick'\nextern rowsafe fn ticks() int as 'q_rowsafe_ticks'\n"
	src := "incl 'std/csv'\n" + tick + parStruct + "t: tbl[P] = unwrap(csv::read_file('p.csv'))\n" +
		"r = t | e -> mutate(n = tick() + e.i)\nprint(ticks())\n" +
		"f = t | e -> filter(e.i > 200000 and tick() > 0)\nprint(list [ticks(), f.nrow()])\n" +
		"g = t | e -> mutate(n = tick() + e.i) | summarize(s = sum(e.n))\nprint(list [ticks(), g.s[0]])\n"
	exe := buildPar(t, dir, src, map[string]string{"tick.hpp": rowSafeTickH, "p.csv": parCSV(300000)})
	r := sameAtEveryCount(t, exe, dir)
	if want := "1\n[2, 100000]\n[3, 45001050000]\n"; r.code != 0 || r.stdout != want {
		t.Fatalf("want %q, got exit %d\nstdout: %s\nstderr: %s", want, r.code, r.stdout, r.stderr)
	}
}

// TestParallel_GCStrings: a mutate that builds strings on every thread while
// the collector runs (workers are registered with it), repeated so the
// collections fall in different places; the strings must survive intact.
func TestParallel_GCStrings(t *testing.T) {
	dir := t.TempDir()
	src := "incl 'std/csv'\n" + parStruct + "t: tbl[P] = unwrap(csv::read_file('p.csv'))\n" +
		"total = 0\nfor k in range(8):\n" +
		"    m = t | e -> mutate(z = (e.g + '-' + to_str(e.i + k) + '-' + e.g).upper(), n = len(e.g + to_str(e.i)))\n" +
		"    c = m | e -> mutate(same = e.z == ('G' + to_str(e.i % 5) + '-' + to_str(e.i + k) + '-G' + to_str(e.i % 5)))\n" +
		"    total = total + sum(c.same) + sum(m.n)\n" +
		"print(total)\n"
	exe := buildPar(t, dir, src, map[string]string{"p.csv": parCSV(300000)})
	r := sameAtEveryCount(t, exe, dir)
	if r.code != 0 || strings.TrimSpace(r.stdout) != "20711160" {
		t.Fatalf("exit %d\nstdout: %s\nstderr: %s", r.code, r.stdout, r.stderr)
	}
}

// TestParallel_ThreadCount: QUARK_THREADS and --threads (§8.2).
func TestParallel_ThreadCount(t *testing.T) {
	dir := t.TempDir()
	exe := buildPar(t, dir, "incl 'std/sys'\nprint(sys::threads())\n", nil)
	for _, bad := range []string{"0", "-1", "x", "", "2.5", " 3", "4097", "99999999999999999999"} {
		r := runPar(t, exe, dir, "QUARK_THREADS="+bad)
		if r.code != 1 || !strings.Contains(r.stderr, "QUARK_THREADS must be a whole number of threads from 1 to 4096, got '"+bad+"'") || r.stdout != "" {
			t.Fatalf("QUARK_THREADS=%q: want a startup error, got exit %d\nstdout: %s\nstderr: %s", bad, r.code, r.stdout, r.stderr)
		}
	}
	for _, good := range []string{"1", "3", "16", "4096"} {
		if r := runPar(t, exe, dir, "QUARK_THREADS="+good); r.code != 0 || strings.TrimSpace(r.stdout) != good {
			t.Fatalf("QUARK_THREADS=%s: exit %d\nstdout: %s\nstderr: %s", good, r.code, r.stdout, r.stderr)
		}
	}
	// The default is at least one thread.
	if r := runPar(t, exe, dir); r.code != 0 || strings.TrimSpace(r.stdout) == "0" {
		t.Fatalf("default: exit %d\nstdout: %s\nstderr: %s", r.code, r.stdout, r.stderr)
	}

	program := filepath.Join(dir, "main.qrk")
	run := func(env []string, args ...string) (string, string, error) {
		cmd := exec.Command(quarkExePath, append([]string{"run"}, args...)...)
		cmd.Dir = dir
		cmd.Env = append(os.Environ(), env...)
		var out, errBuf bytes.Buffer
		cmd.Stdout, cmd.Stderr = &out, &errBuf
		err := cmd.Run()
		return out.String(), errBuf.String(), err
	}
	// --threads wins over QUARK_THREADS.
	if out, stderr, err := run([]string{"QUARK_THREADS=5"}, "--threads", "2", program); err != nil || strings.TrimSpace(out) != "2" {
		t.Fatalf("--threads 2 with QUARK_THREADS=5: %v\nstdout: %s\nstderr: %s", err, out, stderr)
	}
	if out, stderr, err := run([]string{"QUARK_THREADS=5"}, program); err != nil || strings.TrimSpace(out) != "5" {
		t.Fatalf("QUARK_THREADS=5: %v\nstdout: %s\nstderr: %s", err, out, stderr)
	}
	for _, bad := range []string{"0", "-2", "two", "4097"} {
		_, stderr, err := run(nil, "--threads", bad, program)
		if err == nil || !strings.Contains(stderr, "--threads needs a whole number of threads from 1 to 4096, got '"+bad+"'") {
			t.Fatalf("--threads %s: want an error, got %v\nstderr: %s", bad, err, stderr)
		}
	}
}

// TestParallel_ReplOneThread: the REPL runs on one thread whatever
// QUARK_THREADS says (D52, D54).
func TestParallel_ReplOneThread(t *testing.T) {
	cmd := exec.Command(quarkExePath, "repl")
	cmd.Dir = t.TempDir()
	cmd.Env = append(os.Environ(), "QUARK_THREADS=4")
	cmd.Stdin = strings.NewReader("incl 'std/sys'\nprint(sys::threads())\n")
	var out, errBuf bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errBuf
	if err := cmd.Run(); err != nil {
		t.Fatalf("repl: %v\nstderr: %s", err, errBuf.String())
	}
	if got := strings.TrimSpace(normalizeNewlines(out.String())); got != "1" {
		t.Fatalf("want 1, got %q\nstderr: %s", got, errBuf.String())
	}
}

// parEngine exercises the table engine and vec operations on the pool:
// sorts (ties, descending, NaN, missing values, str and multi-key), a
// grouping with many groups and every aggregate, distinct, whole-table
// aggregates, and vec arithmetic, comparisons, reductions, masks, fillna,
// drop_na and astype.
const parEngine = "incl 'std/csv'\n" + parStruct + "t: tbl[P] = unwrap(csv::read_file('p.csv'))\n" + `
fn digest(v: vec) float -> sum(v * 1.0)

srt = t | e -> mutate(k = e.i % 1000, y = e.x * 0.0 + to_float(e.i % 7)) | sort(desc(e.k), e.y)
print(srt | e -> head(5))
print(srt | e -> tail(5))
print(list [digest(srt.i), sum(srt.i * srt.k)])
s2 = t | e -> sort(e.s, desc(e.x))
print(s2 | e -> head(4))
print(s2 | e -> tail(4))
s3 = t | e -> sort(e.x)
print(s3 | e -> tail(4))
print(list [digest(s3.i), s3.x.get(0)])
s4 = t | e -> sort(desc(e.b), e.d)
print(s4 | e -> head(3))
grp = t | e -> group_by(k = e.i % 50000) | summarize(n = count(), sx = sum(e.x), mx = max(e.s), mn = min(e.x), sd = sd(e.x), nd = n_distinct(e.g), m = mean(e.i))
print(list [grp.nrow(), sum(grp.n), sum(grp.sx), sum(grp.sd.fillna(0.0)), sum(grp.m)])
print(grp | e -> head(3))
ds = t | e -> distinct(e.g, e.b)
print(ds)
whole = t | e -> summarize(a = min(e.x), b = max(e.x), c = min(e.s), d = max(e.d), e2 = max(e.b), f = min(e.i), sd = sd(e.x), m = mean(e.x), s = sum(e.i))
print(whole)
v = t.x
w = t.i
print(list [sum(v), mean(v), sd(v), min(v), max(v), sum(w), mean(w), sd(w), min(w), max(w)])
a = v * 2.0 + w
print(list [sum(a), sum(v - w), sum(w * 3), sum(w / 7)])
c = v > 1000.0
print(list [sum(c), all(c), any(c), sum(w > 2.5), sum(w == 7), sum(v != w)])
m = v[v > 2000.0]
print(list [len(m), sum(m)])
f = v.fillna(-1.0)
print(list [sum(f), len(v.drop_na())])
ai = v.fillna(0.0).astype('i64')
print(list [sum(ai), sum(w.astype('f64')), sum(c.astype('i64'))])
print(list [sum(t.b), mean(t.b), sd(t.b)])
`

// TestParallel_EngineAndVec: the engine kernels and vec operations print the
// same output at every thread count, on every table size.
func TestParallel_EngineAndVec(t *testing.T) {
	exe := buildPar(t, t.TempDir(), parEngine, nil)
	for _, n := range parRows {
		n := n
		t.Run(fmt.Sprint(n), func(t *testing.T) {
			dir := t.TempDir()
			if err := os.WriteFile(filepath.Join(dir, "p.csv"), []byte(parCSV(n)), 0o644); err != nil {
				t.Fatal(err)
			}
			if r := sameAtEveryCount(t, exe, dir); r.code != 0 {
				t.Fatalf("exit %d\nstdout: %s\nstderr: %s", r.code, r.stdout, r.stderr)
			}
		})
	}
}

// TestParallel_ExactIntSum: an int sum is exact (128-bit), so it fails only
// when the total doesn't fit, whatever the order of the values (§8.4): a
// running total that would overflow on the way is not an error.
func TestParallel_ExactIntSum(t *testing.T) {
	dir := t.TempDir()
	src := "incl 'std/csv'\n" + parStruct + "t: tbl[P] = unwrap(csv::read_file('p.csv'))\n" +
		"print(sum(vec [9223372036854775807, 1, -1]))\n" +
		"big = t | e -> mutate(k = 9223372036854775807 if e.i == 3 else (-9223372036854775807 if e.i == 300000 else 1))\n" +
		"print(sum(big.k))\n" +
		"print((big | e -> summarize(s = sum(e.k))).s[0])\n" +
		"print((big | e -> group_by(e.b) | summarize(s = sum(e.k))).s)\n" +
		"print(mean(big.k))\n"
	exe := buildPar(t, dir, src, map[string]string{"p.csv": parCSV(300000)})
	r := sameAtEveryCount(t, exe, dir)
	want := "9223372036854775807\n299998\n299998\n[200000, 99998]\n0.9999933333333333\n"
	if r.code != 0 || r.stdout != want {
		t.Fatalf("want %q, got exit %d\nstdout: %s\nstderr: %s", want, r.code, r.stdout, r.stderr)
	}
	over := runPar(t, buildPar(t, t.TempDir(), "print(sum(vec [9223372036854775807, 1, 1, -1]))\n", nil), dir)
	if over.code != 1 || !strings.Contains(over.stderr, "integer overflow in 'sum'") {
		t.Fatalf("a total that doesn't fit must fail: exit %d\nstderr: %s", over.code, over.stderr)
	}
}

// TestParallel_DefaultFollowsCgroupQuota: on Linux, the default thread
// count is the CPUs in the affinity mask capped by the cgroup v2 quota,
// rounded up (§8.2). It runs where the process is at the root of its cgroup
// namespace and that cgroup sets a quota (a container), as on the machine
// where the quota rule was measured.
func TestParallel_DefaultFollowsCgroupQuota(t *testing.T) {
	if goruntime.GOOS != "linux" {
		t.Skip("cgroup quotas are Linux only")
	}
	self, err := os.ReadFile("/proc/self/cgroup")
	if err != nil || strings.TrimSpace(string(self)) != "0::/" {
		t.Skip("not at the root of a cgroup v2 namespace")
	}
	raw, err := os.ReadFile("/sys/fs/cgroup/cpu.max")
	if err != nil {
		t.Skip("no cpu.max")
	}
	var quota, period float64
	if n, _ := fmt.Sscanf(string(raw), "%g %g", &quota, &period); n != 2 || quota <= 0 || period <= 0 {
		t.Skip("no CPU quota")
	}
	want := goruntime.NumCPU() // the affinity mask
	if q := int(math.Ceil(quota/period - 1e-9)); q < want {
		want = q
	}
	dir := t.TempDir()
	exe := buildPar(t, dir, "incl 'std/sys'\nprint(sys::threads())\n", nil)
	env := []string{}
	for _, kv := range os.Environ() {
		if !strings.HasPrefix(kv, "QUARK_THREADS=") {
			env = append(env, kv)
		}
	}
	cmd := exec.Command(exe)
	cmd.Dir = dir
	cmd.Env = env
	out, err := cmd.Output()
	if err != nil || strings.TrimSpace(string(out)) != fmt.Sprint(want) {
		t.Fatalf("want %d threads (quota %g/%g, %d CPUs), got %q (%v)", want, quota, period, goruntime.NumCPU(), out, err)
	}
}
