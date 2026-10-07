package driver_test

import (
	"strings"
	"testing"

	"quark/driver"
)

func TestCompileSource_StopsAtRequestedStage(t *testing.T) {
	src := "println(1 + 2)\n"

	parsed := driver.CompileSource(src, "t.qrk", driver.StageParse)
	if parsed.Failed() || parsed.AST == nil || parsed.Analysis != nil || parsed.CPP != "" {
		t.Fatalf("StageParse: failed=%v ast=%v analysis=%v cpp=%q", parsed.Failed(), parsed.AST != nil, parsed.Analysis != nil, parsed.CPP)
	}

	checked := driver.CompileSource(src, "t.qrk", driver.StageCheck)
	if checked.Failed() || checked.Analysis == nil || checked.CPP != "" {
		t.Fatalf("StageCheck: failed=%v analysis=%v cpp=%q", checked.Failed(), checked.Analysis != nil, checked.CPP)
	}

	emitted := driver.CompileSource(src, "t.qrk", driver.StageEmit)
	if emitted.Failed() || !strings.Contains(emitted.CPP, "int main()") {
		t.Fatalf("StageEmit: failed=%v cpp=%q", emitted.Failed(), emitted.CPP)
	}
}

func TestCompileSource_ReportsErrorsPerStage(t *testing.T) {
	cases := []struct {
		name, src, want string
	}{
		{"parse", "x = (1 +\n", "(parse)"},
		{"load", "use './does_not_exist'\n", "does_not_exist"},
		{"check", "println(undefined_name)\n", "undefined identifier"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			u := driver.CompileSource(tc.src, "t.qrk", driver.StageEmit)
			if !u.Failed() {
				t.Fatalf("expected failure")
			}
			if u.CPP != "" {
				t.Fatalf("emitted C++ despite errors")
			}
			var msgs []string
			for _, d := range u.Diagnostics {
				msgs = append(msgs, d.String())
			}
			if joined := strings.Join(msgs, "\n"); !strings.Contains(joined, tc.want) {
				t.Fatalf("diagnostics %q do not mention %q", joined, tc.want)
			}
		})
	}
}

func TestCompileFile_MissingFile(t *testing.T) {
	u := driver.CompileFile("no/such/file.qrk", driver.StageEmit)
	if !u.Failed() || u.Err == nil || !strings.Contains(u.Err.Error(), "Error reading file") {
		t.Fatalf("expected read error, got failed=%v err=%v", u.Failed(), u.Err)
	}
}
