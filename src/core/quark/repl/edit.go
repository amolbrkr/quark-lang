package repl

import (
	"fmt"
	"io"
	"os"
	"strings"
	"unicode/utf8"

	"golang.org/x/term"
)

// readTerminal reads one input from a terminal in raw mode: left and right,
// Home and End, Up and Down for this session's history, Ctrl-C clears the
// line, Ctrl-D at an empty prompt ends the session, and a bracketed paste is
// one input.
func readTerminal(in *os.File, out io.Writer, history *[]string) (string, error) {
	fd := int(in.Fd())
	st, err := term.MakeRaw(fd)
	if err != nil {
		return "", err
	}
	defer term.Restore(fd, st)
	var lines []string
	block := false
	for {
		cont := len(lines) > 0
		if cont {
			fmt.Fprint(out, "\r\n. ")
		}
		line, paste, err := editLine(in, out, history, cont)
		if err != nil {
			if err == io.EOF && len(lines) == 0 {
				fmt.Fprint(out, "\r\n")
				return "", io.EOF
			}
			return "", err
		}
		if paste {
			fmt.Fprint(out, "\r\n")
			remember(history, line)
			return line, nil
		}
		if block && strings.TrimSpace(line) == "" && !bracketsOpen(strings.Join(lines, "\n")) {
			fmt.Fprint(out, "\r\n")
			src := strings.Join(lines, "\n")
			remember(history, src)
			return finishInput(lines), nil
		}
		lines = append(lines, line)
		src := strings.Join(lines, "\n")
		if len(lines) == 1 && opensBlock(src) {
			block = true
			continue
		}
		if block || bracketsOpen(src) {
			continue
		}
		fmt.Fprint(out, "\r\n")
		remember(history, src)
		return finishInput(lines), nil
	}
}

func remember(history *[]string, src string) {
	if strings.TrimSpace(src) != "" {
		*history = append(*history, src)
	}
}

func editLine(in *os.File, out io.Writer, history *[]string, cont bool) (string, bool, error) {
	var buf []rune
	pos := 0
	hist := len(*history)
	var buf1 [1]byte
	for {
		if _, err := in.Read(buf1[:]); err != nil {
			return "", false, err
		}
		b := buf1[0]
		switch b {
		case 3: // Ctrl-C
			buf = nil
			pos = 0
			redraw(out, buf, pos, cont)
		case 4: // Ctrl-D
			if len(buf) == 0 && !cont {
				return "", false, io.EOF
			}
		case '\r', '\n':
			return string(buf), false, nil
		case 127, 8: // backspace
			if pos > 0 {
				buf = append(buf[:pos-1], buf[pos:]...)
				pos--
				redraw(out, buf, pos, cont)
			}
		case 27:
			seq := readEscape(in)
			if seq == "[200~" {
				pasted, err := readPaste(in)
				return pasted, true, err
			}
			switch seq {
			case "[D": // left
				if pos > 0 {
					pos--
					fmt.Fprint(out, "\x1b[D")
				}
			case "[C": // right
				if pos < len(buf) {
					pos++
					fmt.Fprint(out, "\x1b[C")
				}
			case "[H", "[1~":
				pos = 0
				redraw(out, buf, pos, cont)
			case "[F", "[4~":
				pos = len(buf)
				redraw(out, buf, pos, cont)
			case "[3~": // delete
				if pos < len(buf) {
					buf = append(buf[:pos], buf[pos+1:]...)
					redraw(out, buf, pos, cont)
				}
			case "[A": // up
				if hist > 0 {
					hist--
					buf = []rune((*history)[hist])
					pos = len(buf)
					redraw(out, buf, pos, cont)
				}
			case "[B": // down
				if hist < len(*history) {
					hist++
					if hist == len(*history) {
						buf = nil
					} else {
						buf = []rune((*history)[hist])
					}
					pos = len(buf)
					redraw(out, buf, pos, cont)
				}
			}
		default:
			if b < 32 {
				continue
			}
			r, err := readRune(in, b)
			if err != nil {
				return "", false, err
			}
			if r == utf8.RuneError {
				continue
			}
			buf = append(buf[:pos], append([]rune{r}, buf[pos:]...)...)
			pos++
			redraw(out, buf, pos, cont)
		}
	}
}

func readRune(in *os.File, first byte) (rune, error) {
	if first < 128 {
		return rune(first), nil
	}
	buf := []byte{first}
	for !utf8.FullRune(buf) && len(buf) < utf8.UTFMax {
		var b [1]byte
		if _, err := in.Read(b[:]); err != nil {
			return 0, err
		}
		buf = append(buf, b[0])
	}
	r, _ := utf8.DecodeRune(buf)
	return r, nil
}

func readEscape(in *os.File) string {
	var seq []byte
	var b [1]byte
	for len(seq) < 8 {
		if _, err := in.Read(b[:]); err != nil {
			break
		}
		seq = append(seq, b[0])
		if (b[0] >= 'A' && b[0] <= 'Z') || b[0] == '~' {
			break
		}
	}
	return string(seq)
}

func readPaste(in *os.File) (string, error) {
	var raw []byte
	var b [1]byte
	for {
		if _, err := in.Read(b[:]); err != nil {
			return "", err
		}
		raw = append(raw, b[0])
		if strings.HasSuffix(string(raw), "\x1b[201~") {
			raw = raw[:len(raw)-len("\x1b[201~")]
			return strings.ReplaceAll(string(raw), "\r\n", "\n"), nil
		}
	}
}

func redraw(out io.Writer, buf []rune, pos int, cont bool) {
	prompt := "> "
	if cont {
		prompt = ". "
	}
	fmt.Fprintf(out, "\r\x1b[K%s%s", prompt, string(buf))
	if back := len(buf) - pos; back > 0 {
		fmt.Fprintf(out, "\x1b[%dD", back)
	}
}
