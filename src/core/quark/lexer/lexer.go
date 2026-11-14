package lexer

import (
	"fmt"
	"strings"
	"unicode"

	"quark/token"
)

type Lexer struct {
	input        string
	position     int  // current position in input (points to current char)
	readPosition int  // current reading position in input (after current char)
	ch           byte // current char under examination
	line         int
	column       int

	// stringHasNul is set by readString when a literal contains the \0 escape,
	// which strings can't hold (they are NUL-terminated C strings).
	stringHasNul bool

	// For indentation handling
	parenCount   int
	bracketCount int // tracks [] nesting
	braceCount   int // tracks {} nesting
	atLineStart  bool
	indentLevels []int

	// Token buffer for INDENT/DEDENT injection
	tokens      []token.Token
	tokenIndex  int
	initialized bool
}

func New(input string) *Lexer {
	l := &Lexer{
		input:        input,
		line:         1,
		column:       0,
		atLineStart:  true,
		indentLevels: []int{0},
		tokens:       make([]token.Token, 0),
	}
	l.readChar()
	return l
}

func (l *Lexer) readChar() {
	if l.readPosition >= len(l.input) {
		l.ch = 0
	} else {
		l.ch = l.input[l.readPosition]
	}
	l.position = l.readPosition
	l.readPosition++
	l.column++
	if l.ch == '\n' {
		l.line++
		l.column = 0
	}
}

func (l *Lexer) peekChar() byte {
	if l.readPosition >= len(l.input) {
		return 0
	}
	return l.input[l.readPosition]
}

// insideBrackets returns true when inside (), [] or {} — suppresses indentation
func (l *Lexer) insideBrackets() bool {
	return l.parenCount > 0 || l.bracketCount > 0 || l.braceCount > 0
}

// Tokenize processes all tokens and handles indentation
func (l *Lexer) Tokenize() []token.Token {
	// First pass: collect raw tokens
	rawTokens := l.collectRawTokens()

	// Second pass: track which tokens need indentation
	trackedTokens := l.trackTokens(rawTokens)

	// Third pass: convert indentation to INDENT/DEDENT
	return l.indentationFilter(trackedTokens)
}

type trackedToken struct {
	token       token.Token
	atLineStart bool
	mustIndent  bool
}

func (l *Lexer) collectRawTokens() []token.Token {
	var tokens []token.Token
	for {
		tok := l.nextRawToken()
		tokens = append(tokens, tok)
		if tok.Type == token.EOF {
			break
		}
	}
	return tokens
}

func (l *Lexer) trackTokens(tokens []token.Token) []trackedToken {
	const (
		NO_INDENT = iota
		MAY_INDENT
		MUST_INDENT
	)

	tracked := make([]trackedToken, 0, len(tokens))
	atLineStart := true
	indent := NO_INDENT

	for _, tok := range tokens {
		tt := trackedToken{
			token:       tok,
			atLineStart: atLineStart,
			mustIndent:  false,
		}

		switch tok.Type {
		case token.COLON, token.ARROW:
			atLineStart = false
			indent = MAY_INDENT

		case token.NEWLINE:
			atLineStart = true
			if indent == MAY_INDENT {
				indent = MUST_INDENT
			}

		case token.WS:
			// WS at line start stays at line start

		default:
			// A real token
			if indent == MUST_INDENT {
				tt.mustIndent = true
			}
			atLineStart = false
			indent = NO_INDENT
		}

		tracked = append(tracked, tt)
	}

	return tracked
}

func (l *Lexer) indentationFilter(tracked []trackedToken) []token.Token {
	levels := []int{0}
	var result []token.Token
	depth := 0
	prevWasWS := false

	for i, tt := range tracked {
		tok := tt.token

		switch tok.Type {
		case token.WS:
			depth = len(tok.Literal)
			prevWasWS = true
			continue // WS tokens are never passed to parser

		case token.NEWLINE:
			depth = 0
			if prevWasWS || tt.atLineStart {
				// Ignore blank lines
				continue
			}
			result = append(result, tok)
			continue
		}

		// Real token (not WS, not NEWLINE)
		prevWasWS = false

		if tt.mustIndent {
			// Current depth must be larger than previous level
			if depth <= levels[len(levels)-1] {
				// Insert error token or handle error
				result = append(result, token.Token{
					Type:    token.ILLEGAL,
					Literal: "expected indented block",
					Line:    tok.Line,
					Column:  tok.Column,
				})
			} else {
				levels = append(levels, depth)
				result = append(result, token.Token{
					Type:   token.INDENT,
					Line:   tok.Line,
					Column: tok.Column,
				})
			}
		} else if tt.atLineStart {
			if depth == levels[len(levels)-1] {
				// Same level, nothing to do
			} else if depth > levels[len(levels)-1] {
				// A deeper line starting with '|' continues the expression
				// above it (grammar.md §3.4): it opens no block, and the
				// parser joins it across the NEWLINE.
				if tok.Type == token.PIPE {
					result = append(result, tok)
					continue
				}
				// Unexpected indent increase
				result = append(result, token.Token{
					Type:    token.ILLEGAL,
					Literal: "unexpected indent",
					Line:    tok.Line,
					Column:  tok.Column,
				})
			} else {
				// Dedent - find matching level
				found := -1
				for j := len(levels) - 1; j >= 0; j-- {
					if levels[j] == depth {
						found = j
						break
					}
				}
				if found == -1 {
					result = append(result, token.Token{
						Type:    token.ILLEGAL,
						Literal: "inconsistent indentation",
						Line:    tok.Line,
						Column:  tok.Column,
					})
				} else {
					for j := found + 1; j < len(levels); j++ {
						result = append(result, token.Token{
							Type:   token.DEDENT,
							Line:   tok.Line,
							Column: tok.Column,
						})
					}
					levels = levels[:found+1]
				}
			}
		}

		result = append(result, tok)
		_ = i
	}

	// Emit remaining DEDENTs at EOF
	if len(levels) > 1 {
		lastTok := tracked[len(tracked)-1].token
		for i := 1; i < len(levels); i++ {
			result = append(result, token.Token{
				Type:   token.DEDENT,
				Line:   lastTok.Line,
				Column: lastTok.Column,
			})
		}
	}

	return result
}

// twoCharTokens are the operators spelled with two characters; they win over
// the one-character token their first character would be.
var twoCharTokens = map[[2]byte]token.TokenType{
	{'=', '='}: token.DEQ,
	{'-', '>'}: token.ARROW,
	{'*', '*'}: token.DOUBLESTAR,
	{'!', '='}: token.NE,
	{'<', '='}: token.LTE,
	{'>', '='}: token.GTE,
	{':', ':'}: token.COLONCOLON,
}

var oneCharTokens = map[byte]token.TokenType{
	'=': token.EQUALS,
	'+': token.PLUS,
	'-': token.MINUS,
	'*': token.MULTIPLY,
	'/': token.DIVIDE,
	'%': token.MODULO,
	'!': token.BANG,
	'<': token.LT,
	'>': token.GT,
	'(': token.LPAR,
	')': token.RPAR,
	'[': token.LBRACKET,
	']': token.RBRACKET,
	'{': token.LBRACE,
	'}': token.RBRACE,
	'.': token.DOT,
	',': token.COMMA,
	'|': token.PIPE,
	':': token.COLON,
}

// trackBracket counts open brackets: newlines inside them don't end a line.
func (l *Lexer) trackBracket(ch byte) {
	switch ch {
	case '(':
		l.parenCount++
	case ')':
		if l.parenCount > 0 {
			l.parenCount--
		}
	case '[':
		l.bracketCount++
	case ']':
		if l.bracketCount > 0 {
			l.bracketCount--
		}
	case '{':
		l.braceCount++
	case '}':
		if l.braceCount > 0 {
			l.braceCount--
		}
	}
}

func (l *Lexer) nextRawToken() token.Token {
	var tok token.Token

	// Handle whitespace at line start (for indentation tracking)
	if l.atLineStart && !l.insideBrackets() && (l.ch == ' ' || l.ch == '\t') {
		return l.readWhitespace()
	}

	l.skipComment()

	tok.Line = l.line
	tok.Column = l.column

	if tt, ok := twoCharTokens[[2]byte{l.ch, l.peekChar()}]; ok {
		lit := string([]byte{l.ch, l.peekChar()})
		l.readChar()
		l.atLineStart = false
		l.readChar()
		return token.Token{Type: tt, Literal: lit, Line: tok.Line, Column: tok.Column}
	}
	if tt, ok := oneCharTokens[l.ch]; ok {
		l.trackBracket(l.ch)
		tok = newToken(tt, l.ch, tok.Line, tok.Column)
		l.atLineStart = false
		l.readChar()
		return tok
	}

	switch l.ch {
	case '_':
		if isLetter(l.peekChar()) || isDigit(l.peekChar()) {
			// Part of an identifier
			tok.Literal = l.readIdentifier()
			tok.Type = token.LookupIdent(tok.Literal)
			return tok
		}
		tok = newToken(token.UNDERSCORE, l.ch, tok.Line, tok.Column)
	case '\'', '"':
		tok.Type = token.STRING
		tok.Literal = l.readString(l.ch)
		if l.stringHasNul {
			l.stringHasNul = false
			tok.Type = token.ILLEGAL
			tok.Literal = "'\\0' is not supported in string literals: strings can't contain NUL characters"
		}
		return tok
	case '\n':
		l.readChar()
		if l.insideBrackets() {
			// Inside [], {}, or () — skip newlines, don't emit
			return l.nextRawToken()
		}
		tok = newToken(token.NEWLINE, '\n', tok.Line, tok.Column)
		l.atLineStart = true
		return tok
	case '\r':
		if l.peekChar() == '\n' {
			l.readChar()
		}
		l.readChar()
		if l.insideBrackets() {
			return l.nextRawToken()
		}
		tok = newToken(token.NEWLINE, '\n', tok.Line, tok.Column)
		l.atLineStart = true
		return tok
	case ' ', '\t':
		// Non-line-start whitespace - skip it
		l.skipWhitespace()
		return l.nextRawToken()
	case 0:
		tok.Literal = ""
		tok.Type = token.EOF
		return tok
	default:
		if isLetter(l.ch) {
			tok.Literal = l.readIdentifier()
			tok.Type = token.LookupIdent(tok.Literal)
			return tok
		} else if isDigit(l.ch) {
			tok.Type, tok.Literal = l.readNumber()
			return tok
		} else {
			tok = newToken(token.ILLEGAL, l.ch, tok.Line, tok.Column)
		}
	}

	l.atLineStart = false
	l.readChar()
	return tok
}

func (l *Lexer) readWhitespace() token.Token {
	position := l.position
	line := l.line
	col := l.column

	for l.ch == ' ' || l.ch == '\t' {
		l.readChar()
	}

	return token.Token{
		Type:    token.WS,
		Literal: l.input[position:l.position],
		Line:    line,
		Column:  col,
	}
}

func (l *Lexer) skipWhitespace() {
	for l.ch == ' ' || l.ch == '\t' {
		l.readChar()
	}
}

func (l *Lexer) skipComment() {
	if l.ch == '/' && l.peekChar() == '/' {
		for l.ch != '\n' && l.ch != 0 {
			l.readChar()
		}
	}
}

func (l *Lexer) readIdentifier() string {
	position := l.position
	for isLetter(l.ch) || isDigit(l.ch) {
		l.readChar()
	}
	return l.input[position:l.position]
}

// readNumber reads an int or float literal. `_` may separate two digits
// (100_000, 1_000.5, 1e1_0) and is dropped from the literal; anywhere else
// in a number (1_, 1__0, 1_.5) it is an error.
func (l *Lexer) readNumber() (token.TokenType, string) {
	position := l.position
	isFloat := false
	badSep := false

	// digits reads a run of digits, with `_` allowed between two of them.
	digits := func() {
		for isDigit(l.ch) || l.ch == '_' {
			if l.ch == '_' {
				prevDigit := l.position > position && isDigit(l.input[l.position-1])
				if !prevDigit || !isDigit(l.peekChar()) {
					badSep = true
				}
			}
			l.readChar()
		}
	}

	// Read integer part
	digits()

	// Check for decimal point
	if l.ch == '.' && isDigit(l.peekChar()) {
		isFloat = true
		l.readChar() // consume '.'
		digits()
	} else if l.ch == '.' && l.position > position {
		// Handle cases like "2." (float with no decimal digits)
		// Check if next char could be part of .. operator
		if l.peekChar() != '.' {
			isFloat = true
			l.readChar() // consume '.'
			if l.ch == '_' {
				digits() // 1._5: the separator follows the point
			}
		}
	}

	// Handle ".5" style floats (leading dot)
	if position == l.position && l.ch == '.' {
		isFloat = true
		l.readChar()
		digits()
	}

	// Exponent: 1e3, 2.5E-4, 6e+2 (a digit must follow e/E and its sign).
	if l.position > position && (l.ch == 'e' || l.ch == 'E') {
		next := l.peekChar()
		afterSign := byte(0)
		if l.readPosition+1 < len(l.input) {
			afterSign = l.input[l.readPosition+1]
		}
		if isDigit(next) || ((next == '+' || next == '-') && isDigit(afterSign)) {
			isFloat = true
			l.readChar() // consume e/E
			if l.ch == '+' || l.ch == '-' {
				l.readChar()
			}
			digits()
		}
	}

	text := l.input[position:l.position]
	if badSep {
		return token.ILLEGAL, fmt.Sprintf("'_' in a number must be between two digits: '%s' (write 100_000)", text)
	}
	lit := strings.ReplaceAll(text, "_", "")
	if isFloat {
		return token.FLOAT, lit
	}
	return token.INT, lit
}

func (l *Lexer) readString(quote byte) string {
	l.readChar() // skip opening quote
	var buf []byte

	for {
		if l.ch == quote || l.ch == 0 || l.ch == '\n' {
			break
		}
		if l.ch == '\\' {
			next := l.peekChar()
			switch next {
			case '\'', '"':
				if next == quote {
					buf = append(buf, quote)
					l.readChar() // skip backslash
					l.readChar() // skip escaped quote
					continue
				}
				buf = append(buf, l.ch)
				l.readChar() // skip backslash
				continue
			case 'n':
				buf = append(buf, '\n')
				l.readChar()
				l.readChar()
				continue
			case 't':
				buf = append(buf, '\t')
				l.readChar()
				l.readChar()
				continue
			case '\\':
				buf = append(buf, '\\')
				l.readChar()
				l.readChar()
				continue
			case 'r':
				buf = append(buf, '\r')
				l.readChar()
				l.readChar()
				continue
			case '0':
				l.stringHasNul = true
				l.readChar()
				l.readChar()
				continue
			default:
				// Unknown escape: keep backslash and next char as-is
				buf = append(buf, l.ch)
			}
		} else {
			buf = append(buf, l.ch)
		}
		l.readChar()
	}

	if l.ch == quote {
		l.readChar() // skip closing quote
	}
	return string(buf)
}

func newToken(tokenType token.TokenType, ch byte, line, col int) token.Token {
	return token.Token{Type: tokenType, Literal: string(ch), Line: line, Column: col}
}

func isLetter(ch byte) bool {
	return unicode.IsLetter(rune(ch)) || ch == '_'
}

func isDigit(ch byte) bool {
	return '0' <= ch && ch <= '9'
}
