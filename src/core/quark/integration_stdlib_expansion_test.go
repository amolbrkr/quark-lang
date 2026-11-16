package main

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
)

// TestStdlibExpansion_Sys checks the std/sys additions whose results depend
// on the operating system: platform, arch, the process, the environment,
// the working directory, stderr, and a call from another module.
func TestStdlibExpansion_Sys(t *testing.T) {
	tmp := t.TempDir()
	work := filepath.Join(tmp, "work")
	if err := os.MkdirAll(work, 0o755); err != nil {
		t.Fatal(err)
	}
	other := strings.Join([]string{
		"incl 'std/time' as time",
		"incl 'std/random' as random",
		"",
		"module other:",
		"    fn mins(n: float) float -> time::minutes(n)",
		"    fn take(xs: list, k: int) list -> random::sample(xs, k)",
		"",
	}, "\n")
	if err := os.WriteFile(filepath.Join(tmp, "other.qrk"), []byte(other), 0o644); err != nil {
		t.Fatal(err)
	}
	source := strings.Join([]string{
		"incl 'std/sys' as sys",
		"incl './other' as other",
		"sys::eprint('err-line')",
		"print('platform ' + sys::platform())",
		"print('arch ' + sys::arch())",
		"print('pid ' + to_str(sys::pid()))",
		"print('cpus ' + to_str(sys::cpu_count()))",
		"print('program ' + sys::program())",
		"print(sys::env_or('QK_EXP_MISSING', 'fallback'))",
		"sys::set_env('QK_EXP_SET', 'yes')",
		"print(sys::env('QK_EXP_SET'))",
		"print(sys::environ().get('QK_EXP_SET'))",
		"sys::unset_env('QK_EXP_SET')",
		"print(sys::env('QK_EXP_SET') == null)",
		"sys::unset_env('QK_EXP_NEVER_SET')",
		"print('unset-missing')",
		"dir = sys::args()[0]",
		"when sys::chdir(''):",
		"    ok _ -> print('empty-ok')",
		"    err e -> print(e)",
		"when sys::chdir(dir):",
		"    ok _ -> print('chdir-ok')",
		"    err e -> print(e)",
		"print('cwd ' + sys::cwd())",
		"when sys::chdir(dir + '/no-such-dir-qk'):",
		"    ok _ -> print('missing-ok')",
		"    err e -> print('missing-err')",
		"print(other::mins(2))",
		"xs = list [1, 2, 3, 4]",
		"print(other::take(xs, 4).sort().join(','))",
		"print(xs.join(','))",
		"",
	}, "\n")
	program := filepath.Join(tmp, "exp_sys.qrk")
	if err := os.WriteFile(program, []byte(source), 0o644); err != nil {
		t.Fatal(err)
	}
	exe := filepath.Join(tmp, "exp_sys")
	runQuark(t, "build", program, "-o", exe)

	cmd := exec.Command(exe, work)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("run: %v\nstdout:\n%s\nstderr:\n%s", err, stdout.String(), stderr.String())
	}
	out := normalizeNewlines(stdout.String())
	errOut := normalizeNewlines(stderr.String())
	if !strings.Contains(errOut, "err-line\n") {
		t.Fatalf("stderr:\n%s", errOut)
	}
	if strings.Contains(out, "err-line") {
		t.Fatalf("eprint wrote to stdout:\n%s", out)
	}
	if !strings.Contains(out, "platform "+wantPlatform()+"\n") {
		t.Fatalf("platform: output:\n%s", out)
	}
	if !strings.Contains(out, "arch "+wantArch()+"\n") {
		t.Fatalf("arch: output:\n%s", out)
	}
	pid := labeledInt(t, out, "pid ")
	if pid <= 0 {
		t.Fatalf("pid %d in:\n%s", pid, out)
	}
	if os.Getpid() == pid {
		t.Fatalf("pid %d is the test process, not the program", pid)
	}
	cpus := labeledInt(t, out, "cpus ")
	if cpus <= 0 {
		t.Fatalf("cpu_count %d in:\n%s", cpus, out)
	}
	if !strings.Contains(out, "program ") || !strings.Contains(out, "exp_sys") {
		t.Fatalf("program(): output:\n%s", out)
	}
	for _, line := range []string{
		"fallback",
		"yes",
		"true",
		"unset-missing",
		"chdir: path is empty",
		"chdir-ok",
		"missing-err",
		"120.0",
		"1,2,3,4",
	} {
		if !strings.Contains(out, line) {
			t.Fatalf("missing %q in:\n%s", line, out)
		}
	}
	cwd := labeled(t, out, "cwd ")
	if !samePath(cwd, work) {
		t.Fatalf("cwd %q, want %q\n%s", cwd, work, out)
	}
	// take() returns every element sorted, and leaves the caller's list alone.
	if strings.Count(out, "1,2,3,4") < 2 {
		t.Fatalf("sample from another module:\n%s", out)
	}
}

// TestStdlibExpansion_Limits checks std/random and std/time at the edges of
// the float range and under a local time zone: uniform stays below hi when
// hi - lo overflows or the range is one float wide, weighted works when the
// weights' sum overflows and never picks a zero weight, and a UTC time is
// formatted as UTC (%s, %Z, %z) whatever TZ says, while a local one keeps its
// zone. TZ=EST5EDT is a POSIX zone string, so the C library reads it the same
// way on every platform.
func TestStdlibExpansion_Limits(t *testing.T) {
	tmp := t.TempDir()
	source := strings.Join([]string{
		"incl 'std/random' as random",
		"incl 'std/time' as time",
		"random::seed(7)",
		"wide = 0",
		"for i in range(2000):",
		"    x = random::uniform(-1e308, 1e308)",
		"    if x < -1e308 or x >= 1e308:",
		"        wide = wide + 1",
		"print('wide ' + to_str(wide))",
		"hi = 1.0000000000000002",
		"narrow = 0",
		"for i in range(2000):",
		"    if random::uniform(1.0, hi) != 1.0:",
		"        narrow = narrow + 1",
		"print('narrow ' + to_str(narrow))",
		"v = random::uniform_vec(100000)",
		"print('unit ' + to_str(max(v) < 1.0 and min(v) >= 0.0))",
		"counts = dict { a: 0, b: 0, c: 0 }",
		"for i in range(2000):",
		"    k = random::weighted(list ['a', 'b', 'c'], list [1e308, 0, 1e308])",
		"    counts.set(k, counts.get(k) + 1)",
		"print('big b ' + to_str(counts.get('b')))",
		"print('big a ' + to_str(counts.get('a') > 800))",
		"print('big c ' + to_str(counts.get('c') > 800))",
		"zero = 0",
		"for i in range(2000):",
		"    if random::weighted(list ['a', 'b'], list [1, 0]) == 'b':",
		"        zero = zero + 1",
		"print('zero ' + to_str(zero))",
		"print('utc ' + time::format_time(-1.5, '%s %S %Z %z'))",
		"print('date ' + time::format(time::from_days(1), '%s %Z %z %d'))",
		"print('local ' + time::format_time(0.0, '%s %Z %z', true))",
		"print('literal ' + time::format_time(90061.0, '%%s %%Z 100%%'))",
		"time::sleep(-1.0)",
		"time::sleep(0.0)",
		"time::sleep(0.001)",
		"print('slept')",
		"",
	}, "\n")
	program := filepath.Join(tmp, "limits.qrk")
	if err := os.WriteFile(program, []byte(source), 0o644); err != nil {
		t.Fatal(err)
	}
	exe := filepath.Join(tmp, "limits")
	runQuark(t, "build", program, "-o", exe)

	cmd := exec.Command(exe)
	cmd.Env = append(os.Environ(), "TZ=EST5EDT")
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("run: %v\nstdout:\n%s\nstderr:\n%s", err, stdout.String(), stderr.String())
	}
	want := strings.Join([]string{
		"wide 0",
		"narrow 0",
		"unit true",
		"big b 0",
		"big a true",
		"big c true",
		"zero 0",
		"utc -2 58 UTC +0000",
		"date 86400 UTC +0000 02",
		"local 0 EST -0500",
		"literal %s %Z 100%",
		"slept",
		"",
	}, "\n")
	if got := normalizeNewlines(stdout.String()); got != want {
		t.Fatalf("output:\n%s\nwant:\n%s", got, want)
	}
}

func wantPlatform() string {
	switch runtime.GOOS {
	case "windows", "linux", "darwin":
		return runtime.GOOS
	default:
		return "unknown"
	}
}

func wantArch() string {
	switch runtime.GOARCH {
	case "amd64":
		return "x86_64"
	case "arm64":
		return "arm64"
	case "386":
		return "x86"
	default:
		return "unknown"
	}
}

func labeled(t *testing.T, out, prefix string) string {
	t.Helper()
	for _, line := range strings.Split(out, "\n") {
		if strings.HasPrefix(line, prefix) {
			return strings.TrimPrefix(line, prefix)
		}
	}
	t.Fatalf("no line %q in:\n%s", prefix, out)
	return ""
}

func labeledInt(t *testing.T, out, prefix string) int {
	t.Helper()
	n, err := strconv.Atoi(labeled(t, out, prefix))
	if err != nil {
		t.Fatalf("%s: %v", prefix, err)
	}
	return n
}

func samePath(a, b string) bool {
	a = filepath.Clean(a)
	b = filepath.Clean(b)
	if a == b {
		return true
	}
	if runtime.GOOS == "windows" && strings.EqualFold(a, b) {
		return true
	}
	return false
}

// TestStdlibExpansion_Rejected probes the misuse paths of the new std/sys,
// std/time and std/random functions: a wrong type, a wrong argument count,
// null, an untyped value, a default that makes the call invalid, a value from
// another module, a call into the native binding, and a write into a list or
// dict whose element type the new function fixed.
func TestStdlibExpansion_Rejected(t *testing.T) {
	cases := []struct {
		name, source, errSubstr string
	}{
		{"chdir_int", "incl 'std/sys' as sys\nsys::chdir(1)\n", "argument 1 of 'sys::chdir' expects str, got int"},
		{"chdir_null", "incl 'std/sys' as sys\nsys::chdir(null)\n", "argument 1 of 'sys::chdir' expects str, got null"},
		{"chdir_arity", "incl 'std/sys' as sys\nsys::chdir()\n", "function 'sys::chdir' expects 1 argument but got 0"},
		{"eprint_null_end", "incl 'std/sys' as sys\nsys::eprint(1, null)\n", "argument 2 of 'sys::eprint' expects str, got null"},
		{"eprint_arity", "incl 'std/sys' as sys\nsys::eprint()\n", "function 'sys::eprint' expects 1-2 arguments but got 0"},
		{"parts_str", "incl 'std/time' as time\nprint(time::parts('x'))\n", "argument 1 of 'time::parts' expects date, got str"},
		{"parts_null", "incl 'std/time' as time\nprint(time::parts(null))\n", "argument 1 of 'time::parts' expects date, got null"},
		{"format_pattern_int", "incl 'std/time' as time\nprint(time::format_time(0.0, 1))\n", "argument 2 of 'time::format_time' expects str, got int"},
		{"make_arity", "incl 'std/time' as time\nprint(time::timestamp(1970, 1))\n", "function 'time::timestamp' expects 3-7 arguments but got 2"},
		{"seconds_str", "incl 'std/time' as time\nprint(time::seconds('a'))\n", "argument 1 of 'time::seconds' expects float, got str"},
		{"sample_arity", "incl 'std/random' as random\nprint(random::sample(list [1]))\n", "function 'random::sample' expects 2 arguments but got 1"},
		{"sample_null", "incl 'std/random' as random\nprint(random::sample(null, 1))\n", "argument 1 of 'random::sample' expects list[any], got null"},
		{"uniform_arity", "incl 'std/random' as random\nprint(random::uniform(1.0, 2.0, 3.0))\n", "function 'random::uniform' expects 0-2 arguments but got 3"},
		{"weighted_int", "incl 'std/random' as random\nprint(random::weighted(list [1], 2))\n", "argument 2 of 'random::weighted' expects list[any], got int"},
		{"from_module", "incl './lib' as lib\nincl 'std/time' as time\nprint(time::format_time(lib::label()))\n", "argument 1 of 'time::format_time' expects float, got str"},
		{"chdir_any_int", "incl 'std/sys' as sys\nfn id(v) -> v\nsys::chdir(id(1))\n", "argument 'path' of 'sys::chdir' expects str, got int"},
		{"chdir_any_null", "incl 'std/sys' as sys\nfn id(v) -> v\nsys::chdir(id(null))\n", "argument 'path' of 'sys::chdir' expects str, got null"},
		{"parts_any", "incl 'std/time' as time\nfn id(v) -> v\nprint(time::parts(id('nope')))\n", "argument 'd' of 'time::parts' expects date, got str"},
		{"sample_any_null", "incl 'std/random' as random\nfn id(v) -> v\nprint(random::sample(id(null), 1))\n", "expected list, got null"},
		{"sample_k", "incl 'std/random' as random\nprint(random::sample(list [1, 2], 3))\n", "random::sample() needs 0 <= k <= len(xs), got k=3 and len=2"},
		{"uniform_order", "incl 'std/random' as random\nprint(random::uniform(5.0, 1.0))\n", "random::uniform() needs a finite lo <= hi, got 5.0 and 1.0"},
		{"uniform_one_arg", "incl 'std/random' as random\nprint(random::uniform(2.0))\n", "random::uniform() needs a finite lo <= hi, got 2.0 and 1.0"},
		{"normal_sd", "incl 'std/random' as random\nprint(random::normal(0.0, -1.0))\n", "random::normal() needs a finite mean and sd >= 0"},
		{"weighted_len", "incl 'std/random' as random\nprint(random::weighted(list [1, 2], list [1]))\n", "needs one weight per element, got 2 elements and 1 weights"},
		{"weighted_neg", "incl 'std/random' as random\nprint(random::weighted(list ['a'], list [-1]))\n", "must be a finite number >= 0"},
		{"weighted_empty", "incl 'std/random' as random\nprint(random::weighted(list [], list []))\n", "random::weighted() on an empty list"},
		{"weighted_zero", "incl 'std/random' as random\nprint(random::weighted(list ['a'], list [0]))\n", "weights must sum to a positive number"},
		{"set_env_empty", "incl 'std/sys' as sys\nsys::set_env('', 'a')\n", "sys::set_env() name must not be empty"},
		{"set_env_eq", "incl 'std/sys' as sys\nsys::set_env('A=B', 'a')\n", "sys::set_env() name must not contain '='"},
		{"env_eq", "incl 'std/sys' as sys\nsys::set_env('QA', 'B=c')\nprint(sys::env('QA=B'))\n", "sys::env() name must not contain '='"},
		{"env_or_eq", "incl 'std/sys' as sys\nprint(sys::env_or('QA=B', 'd'))\n", "sys::env_or() name must not contain '='"},
		{"env_empty", "incl 'std/sys' as sys\nprint(sys::env(''))\n", "sys::env() name must not be empty"},
		{"env_any_eq", "incl 'std/sys' as sys\nfn id(v) -> v\nprint(sys::env(id('A=B')))\n", "sys::env() name must not contain '='"},
		{"env_int", "incl 'std/sys' as sys\nprint(sys::env(1))\n", "argument 1 of 'sys::env' expects str, got int"},
		{"sleep_inf", "incl 'std/time' as time\ntime::sleep(to_float('inf'))\nprint('woke')\n", "time::sleep() seconds must be a finite number"},
		{"sleep_nan", "incl 'std/time' as time\ntime::sleep(to_float('nan'))\nprint('woke')\n", "time::sleep() seconds must be a finite number"},
		{"sleep_neg_inf", "incl 'std/time' as time\ntime::sleep(-to_float('inf'))\nprint('woke')\n", "time::sleep() seconds must be a finite number"},
		{"format_s_flag", "incl 'std/time' as time\nprint(time::format_time(0.0, '%-s'))\n", "time::format() %s, %Z and %z can't take a flag or a width"},
		{"format_z_width", "incl 'std/time' as time\nprint(time::format(time::from_days(0), '%10Z'))\n", "time::format() %s, %Z and %z can't take a flag or a width"},
		{"normal_overflow", "incl 'std/random' as random\nrandom::seed(1)\nfor i in range(1000):\n    x = random::normal(1e308, 1e308)\nprint('no error')\n", "random::normal() drew a value too large for a float, with mean 1e+308 and sd 1e+308"},
		{"normal_vec_overflow", "incl 'std/random' as random\nrandom::seed(1)\nprint(random::normal_vec(1000, 1e308, 1e308))\n", "random::normal_vec() drew a value too large for a float"},
		{"duration_nan", "incl 'std/time' as time\nprint(time::duration(to_float('nan')))\n", "time::duration() seconds must be a finite number"},
		{"make_month", "incl 'std/time' as time\nprint(time::timestamp(1970, 13, 1))\n", "date is not a real calendar time"},
		{"iso_inf", "incl 'std/time' as time\nprint(time::iso(to_float('inf')))\n", "time::iso() timestamp is not a finite time it can show"},
		{"parse_empty", "incl 'std/time' as time\nprint(time::parse('a', ''))\n", "time::parse() pattern must not be empty"},
		{"uniform_vec_neg", "incl 'std/random' as random\nprint(random::uniform_vec(-1))\n", "random::uniform_vec() count must be >= 0, got -1"},
		{"randint_vec_order", "incl 'std/random' as random\nprint(random::randint_vec(1, 5, 1))\n", "random::randint_vec() needs lo <= hi, got 5 and 1"},
		{"args_push", "incl 'std/sys' as sys\nsys::args().push(1)\n", "can't add a int to a list of str"},
		{"sample_push", "incl 'std/random' as random\nxs = list [1, 2, 3]\nrandom::sample(xs, 2).push('a')\n", "can't add a str to a list of int"},
		{"environ_set", "incl 'std/sys' as sys\nsys::environ().set('X', 1)\n", "can't store a int under key 'X' of a dict of str"},
		{"native_format", "incl 'std/time' as time\nfn id(v) -> v\nprint(time::time_format(id('nope'), '%Y', false))\n", "expected float, got str"},
	}

	tmp := t.TempDir()
	lib := "module lib:\n    fn label() str -> 'nope'\n"
	if err := os.WriteFile(filepath.Join(tmp, "lib.qrk"), []byte(lib), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			program := filepath.Join(tmp, tc.name+".qrk")
			if err := os.WriteFile(program, []byte(tc.source), 0o644); err != nil {
				t.Fatalf("write: %v", err)
			}
			cmd := exec.Command(quarkExePath, "run", program)
			var out, errBuf bytes.Buffer
			cmd.Stdout = &out
			cmd.Stderr = &errBuf
			if err := cmd.Run(); err == nil {
				t.Fatalf("expected failure, but it succeeded with output:\n%s", out.String())
			}
			combined := out.String() + errBuf.String()
			if !strings.Contains(combined, tc.errSubstr) {
				t.Fatalf("expected error containing %q, got:\nstdout: %s\nstderr: %s", tc.errSubstr, out.String(), errBuf.String())
			}
		})
	}
}
