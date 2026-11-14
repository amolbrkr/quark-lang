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
		{"parts_str", "incl 'std/time' as time\nprint(time::parts('x'))\n", "argument 1 of 'time::parts' expects float, got str"},
		{"parts_null", "incl 'std/time' as time\nprint(time::parts(null))\n", "argument 1 of 'time::parts' expects float, got null"},
		{"format_pattern_int", "incl 'std/time' as time\nprint(time::format(0.0, 1))\n", "argument 2 of 'time::format' expects str, got int"},
		{"make_arity", "incl 'std/time' as time\nprint(time::make(1970, 1))\n", "function 'time::make' expects 3-7 arguments but got 2"},
		{"seconds_str", "incl 'std/time' as time\nprint(time::seconds('a'))\n", "argument 1 of 'time::seconds' expects float, got str"},
		{"sample_arity", "incl 'std/random' as random\nprint(random::sample(list [1]))\n", "function 'random::sample' expects 2 arguments but got 1"},
		{"sample_null", "incl 'std/random' as random\nprint(random::sample(null, 1))\n", "argument 1 of 'random::sample' expects list[any], got null"},
		{"uniform_arity", "incl 'std/random' as random\nprint(random::uniform(1.0, 2.0, 3.0))\n", "function 'random::uniform' expects 0-2 arguments but got 3"},
		{"weighted_int", "incl 'std/random' as random\nprint(random::weighted(list [1], 2))\n", "argument 2 of 'random::weighted' expects list[any], got int"},
		{"from_module", "incl './lib' as lib\nincl 'std/time' as time\nprint(time::format(lib::label()))\n", "argument 1 of 'time::format' expects float, got str"},
		{"chdir_any_int", "incl 'std/sys' as sys\nfn id(v) -> v\nsys::chdir(id(1))\n", "argument 'path' of 'sys::chdir' expects str, got int"},
		{"chdir_any_null", "incl 'std/sys' as sys\nfn id(v) -> v\nsys::chdir(id(null))\n", "argument 'path' of 'sys::chdir' expects str, got null"},
		{"parts_any", "incl 'std/time' as time\nfn id(v) -> v\nprint(time::parts(id('nope')))\n", "argument 'ts' of 'time::parts' expects float, got str"},
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
		{"duration_nan", "incl 'std/time' as time\nprint(time::duration(to_float('nan')))\n", "time::duration() seconds must be a finite number"},
		{"make_month", "incl 'std/time' as time\nprint(time::make(1970, 13, 1))\n", "date is not a real calendar time"},
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
