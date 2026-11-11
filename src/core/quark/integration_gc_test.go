package main

import (
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
)

// GC regression tests. The programs live in src/testfiles/gc and read memory
// statistics through gcstats.hpp.
//
// Two kinds of leak are possible with a conservative collector:
//   - GC-visible: garbage stays reachable (for example from a global
//     registry), so the GC heap keeps growing.
//   - Outside the GC: a GC object owns memory from malloc (a std::string or
//     std::vector using the default allocator). The GC frees the object but
//     never runs its destructor, so the malloc'd memory leaks. Only RSS shows
//     this one.
// The opposite bug, memory that only a malloc'd buffer points to, makes the
// GC free live objects; TestGC_ResourceSurvivesCollection covers the known
// case.

// Allowed growth between the warm-up report and the final one. Without leaks
// both numbers stay flat. The dict key leak fixed on 2026-11-11 grew peak RSS
// by about 6 MiB over the eight rounds.
const (
	gcLiveGrowthLimit  = 512 * 1024
	peakRSSGrowthLimit = 2 * 1024 * 1024
)

type gcReport struct {
	live, peakRSS int64
}

func parseGCReport(t *testing.T, out, label string) gcReport {
	t.Helper()
	for _, line := range strings.Split(normalizeNewlines(out), "\n") {
		fields := strings.Fields(line)
		if len(fields) != 3 || fields[0] != label {
			continue
		}
		live, err1 := strconv.ParseInt(fields[1], 10, 64)
		rss, err2 := strconv.ParseInt(fields[2], 10, 64)
		if err1 != nil || err2 != nil {
			t.Fatalf("malformed %q report: %q", label, line)
		}
		return gcReport{live: live, peakRSS: rss}
	}
	t.Fatalf("no %q report in output:\n%s", label, out)
	return gcReport{}
}

func TestGC_SteadyStateMemory(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("peak RSS is not available on Windows")
	}
	root := repoRootFromThisFile(t)
	program := filepath.Join(root, "src", "testfiles", "gc", "gc_steady_state.qrk")

	out := runQuarkIn(t, t.TempDir(), "run", program)
	warm := parseGCReport(t, out, "warm")
	final := parseGCReport(t, out, "final")
	t.Logf("GC live: %d -> %d bytes; peak RSS: %d -> %d bytes", warm.live, final.live, warm.peakRSS, final.peakRSS)

	if growth := final.live - warm.live; growth > gcLiveGrowthLimit {
		t.Errorf("GC heap grew by %d bytes across rounds that leave no live data: garbage is staying reachable", growth)
	}
	if warm.peakRSS < 0 {
		t.Skip("peak RSS not available on this platform")
	}
	if growth := final.peakRSS - warm.peakRSS; growth > peakRSSGrowthLimit {
		t.Errorf("peak RSS grew by %d bytes while the GC heap stayed flat: memory allocated outside the GC is leaking", growth)
	}
}

func TestGC_ResourceSurvivesCollection(t *testing.T) {
	root := repoRootFromThisFile(t)
	program := filepath.Join(root, "src", "testfiles", "gc", "gc_resource_stress.qrk")

	dir := t.TempDir()
	out := runQuarkIn(t, dir, "run", program)
	want := "line 0\nline 1\nline 2\nline 3\nline 4"
	if got := strings.TrimSpace(normalizeNewlines(out)); got != want {
		t.Fatalf("file written across collections reads back wrong\n--- got ---\n%s\n--- want ---\n%s", got, want)
	}
	if _, err := os.Stat(filepath.Join(dir, "gc_resource_stress.tmp")); err != nil {
		t.Fatalf("expected the program to write in its working directory: %v", err)
	}
}
