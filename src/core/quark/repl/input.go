package repl

import (
	"bufio"
	"io"
	"strings"

	"quark/lexer"
	"quark/token"
)

// readInput reads one input. prompt is called before each continuation line.
// A block (a first line ending in ':' or '->') runs at the next empty line.
// An open bracket keeps the input open. At EOF with nothing read, the error
// is io.EOF.
func readInput(in *bufio.Reader, prompt func()) (string, error) {
	var lines []string
	block := false
	for {
		if len(lines) > 0 {
			prompt()
		}
		line, err := in.ReadString('\n')
		if err != nil && len(line) == 0 {
			if err == io.EOF && len(lines) == 0 {
				return "", io.EOF
			}
			if err == io.EOF {
				return finishInput(lines), nil
			}
			return "", err
		}
		line = strings.TrimRight(line, "\r\n")
		if block && strings.TrimSpace(line) == "" && !bracketsOpen(strings.Join(lines, "\n")) {
			break
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
		break
	}
	return finishInput(lines), nil
}

// finishInput joins the lines and terminates the last one, so a block ends
// with the dedent the parser expects.
func finishInput(lines []string) string {
	if len(lines) == 0 {
		return "\n"
	}
	return strings.Join(lines, "\n") + "\n"
}

func opensBlock(src string) bool {
	toks := significant(src)
	if len(toks) == 0 || bracketsOpen(src) {
		return false
	}
	last := toks[len(toks)-1].Type
	return last == token.COLON || last == token.ARROW
}

func bracketsOpen(src string) bool {
	depth := 0
	for _, t := range significant(src) {
		switch t.Type {
		case token.LPAR, token.LBRACKET, token.LBRACE:
			depth++
		case token.RPAR, token.RBRACKET, token.RBRACE:
			if depth > 0 {
				depth--
			}
		}
	}
	return depth > 0
}

func significant(src string) []token.Token {
	var out []token.Token
	for _, t := range lexer.New(src).Tokenize() {
		if t.Type == token.EOF || t.Type == token.NEWLINE || t.Type == token.INDENT || t.Type == token.DEDENT {
			continue
		}
		out = append(out, t)
	}
	return out
}
