package toolchain

import (
	"strings"
	"testing"
)

func TestParseMakeDeps(t *testing.T) {
	text := "answer.o: answer.cpp answer.hpp \\\n  /usr/include/stdc-predef.h \\\r\n  \"C:/Program Files/clang/stdint.h\"\n"
	got := strings.Join(parseMakeDeps(text), "|")
	want := "answer.cpp|answer.hpp|/usr/include/stdc-predef.h|C:/Program Files/clang/stdint.h"
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}

	win := parseMakeDeps("C:/tmp/answer.o: C:/tmp/answer.cpp C:/tmp/answer.hpp\n")
	if strings.Join(win, "|") != "C:/tmp/answer.cpp|C:/tmp/answer.hpp" {
		t.Fatalf("windows target: got %q", win)
	}

	// clang on Windows escapes spaces and keeps backslash separators.
	escaped := parseMakeDeps("depprobe.o: C:\\Users\\a\\depprobe.cpp \\\n  C:\\Program\\ Files\\LLVM\\include\\stdint.h\n")
	if strings.Join(escaped, "|") != "C:\\Users\\a\\depprobe.cpp|C:\\Program Files\\LLVM\\include\\stdint.h" {
		t.Fatalf("escaped spaces: got %q", escaped)
	}
}
