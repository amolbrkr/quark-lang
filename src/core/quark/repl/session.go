// Package repl is the interactive session: it compiles each input with the
// ordinary compiler and runs it in a long-lived host process.
package repl

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"syscall"

	"quark/driver"
	"quark/toolchain"
	"quark/types"
)

// Session is one REPL. Inputs that compile and run are kept; a failed input
// is not.
type Session struct {
	Dir     string
	inputs  []string
	fp      string
	work    string
	libs    []string // import libraries of inputs that ran, Windows only
	host    *exec.Cmd
	stdin   io.WriteCloser
	result  *bufio.Reader
	winStat chan string
	closeFn func()
	n       int
	tc      *toolchain.Toolchain
	pch     string
	opts    toolchain.Options
}

// Run reads inputs from in and writes the prompt to prompt. Program output
// goes to the host's own stdout, which is the terminal. When in is not a
// terminal, prompt is ignored and no prompts are written.
func Run(dir string, in io.Reader, prompt io.Writer, interactive bool) error {
	s := &Session{Dir: dir}
	defer s.Close()
	if err := s.start(); err != nil {
		return err
	}
	return s.loop(in, prompt, interactive)
}

func (s *Session) start() error {
	tc, err := toolchain.Find()
	if err != nil {
		return err
	}
	s.tc = tc
	host, pch, err := tc.HostPath(false)
	if err != nil {
		return fmt.Errorf("error[QK-REPL-002] (repl): the REPL host could not be built: %s", err)
	}
	s.pch = pch
	work, err := os.MkdirTemp("", "quark-repl-*")
	if err != nil {
		return err
	}
	s.work = work
	if runtime.GOOS == "windows" {
		return s.startWindows(host)
	}
	r, w, err := os.Pipe()
	if err != nil {
		return err
	}
	cmd := exec.Command(host, "--result-fd", "3")
	cmd.Stdin = nil
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.ExtraFiles = []*os.File{w}
	pr, pw, err := os.Pipe()
	if err != nil {
		return err
	}
	cmd.Stdin = pr
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("error[QK-REPL-002] (repl): the REPL host could not be started: %s", err)
	}
	signal.Ignore(syscall.SIGINT)
	w.Close()
	pr.Close()
	s.host = cmd
	s.stdin = pw
	s.result = bufio.NewReader(r)
	s.closeFn = func() {
		pw.Close()
		r.Close()
	}
	go func() { _ = cmd.Wait() }()
	return nil
}

func (s *Session) startWindows(host string) error {
	pr, pw, err := os.Pipe()
	if err != nil {
		return err
	}
	errR, errW, err := os.Pipe()
	if err != nil {
		return err
	}
	cmd := exec.Command(host)
	cmd.Stdin = pr
	cmd.Stdout = os.Stdout
	cmd.Stderr = errW
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("error[QK-REPL-002] (repl): the REPL host could not be started: %s", err)
	}
	signal.Ignore(syscall.SIGINT)
	pr.Close()
	errW.Close()
	s.host = cmd
	s.stdin = pw
	ch := make(chan string, 4)
	s.winStat = ch
	go func() {
		rd := bufio.NewReader(errR)
		for {
			line, err := rd.ReadString('\n')
			if len(line) > 0 {
				line = strings.TrimRight(line, "\r\n")
				const mark = "@@QUARK@@ "
				if strings.HasPrefix(line, mark) {
					ch <- strings.TrimPrefix(line, mark)
				} else {
					fmt.Fprintln(os.Stderr, line)
				}
			}
			if err != nil {
				close(ch)
				return
			}
		}
	}()
	s.closeFn = func() { pw.Close(); errR.Close() }
	go func() { _ = cmd.Wait() }()
	return nil
}

func (s *Session) readStatus() (string, error) {
	if s.winStat != nil {
		st, ok := <-s.winStat
		if !ok {
			return "", io.EOF
		}
		return st, nil
	}
	line, err := s.result.ReadString('\n')
	return strings.TrimSpace(line), err
}

// Close ends the host and removes the input libraries.
func (s *Session) Close() {
	if s.stdin != nil {
		fmt.Fprintln(s.stdin, "quit")
		s.stdin.Close()
		s.stdin = nil
	}
	if s.closeFn != nil {
		s.closeFn()
	}
	if s.work != "" {
		os.RemoveAll(s.work)
	}
}

func (s *Session) loop(in io.Reader, prompt io.Writer, interactive bool) error {
	if interactive {
		f, ok := in.(*os.File)
		if ok {
			return s.loopTerminal(f, prompt)
		}
	}
	rd := bufio.NewReader(in)
	for {
		src, err := readInput(rd, func() {})
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
		if strings.TrimSpace(src) == "" {
			continue
		}
		if err := s.eval(src); err != nil {
			return err
		}
	}
}

func (s *Session) loopTerminal(in *os.File, prompt io.Writer) error {
	var history []string
	for {
		fmt.Fprint(prompt, "> ")
		src, err := readTerminal(in, prompt, &history)
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
		if strings.TrimSpace(src) == "" {
			continue
		}
		if err := s.eval(src); err != nil {
			return err
		}
	}
}

func (s *Session) eval(src string) error {
	all := append(append([]string{}, s.inputs...), src)
	u := driver.CompileSession(s.Dir, all)
	if u.Failed() {
		u.PrintDiagnostics()
		return nil
	}
	prev := fingerprint(u.Analysis, len(s.inputs))
	if s.fp != "" && prev != s.fp {
		fmt.Fprintf(os.Stderr, "internal compiler error [INV-REPL-STABLE]: re-analysis changed the names of earlier inputs\n")
		return fmt.Errorf("error[QK-REPL-002] (repl): the REPL host stopped unexpectedly; the session has ended")
	}
	s.n++
	ext := ".so"
	if runtime.GOOS == "windows" {
		ext = ".dll"
	}
	cpp := filepath.Join(s.work, fmt.Sprintf("input-%d.cpp", s.n))
	so := filepath.Join(s.work, fmt.Sprintf("input-%d%s", s.n, ext))
	if err := os.WriteFile(cpp, []byte(u.CPP), 0o644); err != nil {
		return err
	}
	opts := toolchain.Options{}
	opts.PrevLibs = append([]string{}, s.libs...)
	opts.IncludeDirs = append(opts.IncludeDirs, u.Native.IncludeDirs...)
	for _, lib := range u.Native.Libs {
		opts.Libs = append(opts.Libs, toolchain.Lib{Name: lib.Name, Origin: lib.Origin()})
	}
	if err := s.tc.CompileInput(cpp, so, s.pch, opts); err != nil {
		fmt.Fprintf(os.Stderr, "Compilation failed: %s\n", err)
		return nil
	}
	sym := fmt.Sprintf("quark_input_%d", len(all))
	fmt.Fprintf(s.stdin, "run\t%s\t%s\n", so, sym)
	status, err := s.readStatus()
	if err != nil {
		return fmt.Errorf("error[QK-REPL-002] (repl): the REPL host stopped unexpectedly; the session has ended")
	}
	switch status {
	case "ok":
		s.inputs = all
		s.fp = fingerprint(u.Analysis, len(all))
		if runtime.GOOS == "windows" {
			s.libs = append(s.libs, strings.TrimSuffix(so, ext)+".lib")
		}
	case "interrupted":
		fmt.Fprintln(os.Stderr, "interrupted")
	case "fail", "signal":
		// The runtime or the host already printed the error.
	default:
		return fmt.Errorf("error[QK-REPL-002] (repl): the REPL host stopped unexpectedly; the session has ended")
	}
	return nil
}

// fingerprint is the stable names of the first n inputs: slot numbers and the
// C names of their top-level functions and structs. A later re-analysis must
// produce the same string (INV-REPL-STABLE).
func fingerprint(an *types.Analysis, n int) string {
	if an == nil || an.Session == nil || n <= 0 {
		return ""
	}
	var b strings.Builder
	for _, s := range an.Session.Slots {
		if s == nil || s.Decl == nil || inputNum(s.Decl.SourceFile) > n {
			continue
		}
		fmt.Fprintf(&b, "slot %s %d %d %s\n", s.Name, s.Storage, s.Slot, s.Decl.SourceFile)
	}
	fns := append([]*types.FixedFn(nil), an.Session.Fns...)
	sort.Slice(fns, func(i, j int) bool {
		if fns[i] == nil || fns[j] == nil || fns[i].Sym == nil || fns[j].Sym == nil {
			return false
		}
		return fns[i].Sym.ID < fns[j].Sym.ID
	})
	for _, f := range fns {
		if f == nil || !f.TopLevel || f.Decl == nil || inputNum(f.Decl.SourceFile) > n {
			continue
		}
		fmt.Fprintf(&b, "fn %s %s\n", f.CName, f.Decl.SourceFile)
	}
	var structs []string
	for name, home := range an.Session.StructHome {
		if inputNum(home) <= n {
			structs = append(structs, name+" "+home)
		}
	}
	sort.Strings(structs)
	for _, st := range structs {
		fmt.Fprintf(&b, "struct %s\n", st)
	}
	return b.String()
}

func inputNum(file string) int {
	file = strings.TrimPrefix(file, "<input ")
	file = strings.TrimSuffix(file, ">")
	n, err := strconv.Atoi(file)
	if err != nil {
		return 1 << 30
	}
	return n
}
