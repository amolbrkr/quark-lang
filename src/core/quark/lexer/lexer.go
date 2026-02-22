package lexer

import (
	"quark/token"
	"unicode"
)

type Lexer struct {
	input        string
	position     int  // current position in input (points to current char)
	readPosition int  // current reading position in input (after current char)
	ch           byte // current char under examination
	line         int
	column       int

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

func (l *Lexer) nextRawToken() token.Token {
	var tok token.Token

	// Handle whitespace at line start (for indentation tracking)
	if l.atLineStart && !l.insideBrackets() && (l.ch == ' ' || l.ch == '\t') {
		return l.readWhitespace()
	}

	l.skipComment()

	tok.Line = l.line
	tok.Column = l.column

	switch l.ch {
	case '=':
		if l.peekChar() == '=' {
			l.readChar()
			tok = token.Token{Type: token.DEQ, Literal: "==", Line: tok.Line, Column: tok.Column}
		} else {
			tok = newToken(token.EQUALS, l.ch, tok.Line, tok.Column)
		}
	case '+':
		tok = newToken(token.PLUS, l.ch, tok.Line, tok.Column)
	case '-':
		if l.peekChar() == '>' {
			l.readChar()
			tok = token.Token{Type: token.ARROW, Literal: "->", Line: tok.Line, Column: tok.Column}
		} else {
			tok = newToken(token.MINUS, l.ch, tok.Line, tok.Column)
		}
	case '*':
		if l.peekChar() == '*' {
			l.readChar()
			tok = token.Token{Type: token.DOUBLESTAR, Literal: "**", Line: tok.Line, Column: tok.Column}
		} else {
			tok = newToken(token.MULTIPLY, l.ch, tok.Line, tok.Column)
		}
	case '/':
		tok = newToken(token.DIVIDE, l.ch, tok.Line, tok.Column)
	case '%':
		tok = newToken(token.MODULO, l.ch, tok.Line, tok.Column)
	case '!':
		if l.peekChar() == '=' {
			l.readChar()
			tok = token.Token{Type: token.NE, Literal: "!=", Line: tok.Line, Column: tok.Column}
		} else {
			tok = newToken(token.BANG, l.ch, tok.Line, tok.Column)
		}
	case '<':
		if l.peekChar() == '=' {
			l.readChar()
			tok = token.Token{Type: token.LTE, Literal: "<=", Line: tok.Line, Column: tok.Column}
		} else {
			tok = newToken(token.LT, l.ch, tok.Line, tok.Column)
		}
	case '>':
		if l.peekChar() == '=' {
			l.readChar()
			tok = token.Token{Type: token.GTE, Literal: ">=", Line: tok.Line, Column: tok.Column}
		} else {
			tok = newToken(token.GT, l.ch, tok.Line, tok.Column)
		}
	case '(':
		l.parenCount++
		tok = newToken(token.LPAR, l.ch, tok.Line, tok.Column)
	case ')':
		if l.parenCount > 0 {
			l.parenCount--
		}
		tok = newToken(token.RPAR, l.ch, tok.Line, tok.Column)
	case '[':
		l.bracketCount++
		tok = newToken(token.LBRACKET, l.ch, tok.Line, tok.Column)
	case ']':
		if l.bracketCount > 0 {
			l.bracketCount--
		}
		tok = newToken(token.RBRACKET, l.ch, tok.Line, tok.Column)
	case '{':
		l.braceCount++
		tok = newToken(token.LBRACE, l.ch, tok.Line, tok.Column)
	case '}':
		if l.braceCount > 0 {
			l.braceCount--
		}
		tok = newToken(token.RBRACE, l.ch, tok.Line, tok.Column)
	case '.':
		tok = newToken(token.DOT, l.ch, tok.Line, tok.Column)
	case ',':
		tok = newToken(token.COMMA, l.ch, tok.Line, tok.Column)
	case '|':
		tok = newToken(token.PIPE, l.ch, tok.Line, tok.Column)
	case ':':
		tok = newToken(token.COLON, l.ch, tok.Line, tok.Column)
	case '_':
		if isLetter(l.peekChar()) || isDigit(l.peekChar()) {
			// Part of an identifier
			tok.Literal = l.readIdentifier()
			tok.Type = token.LookupIdent(tok.Literal)
			return tok
		}
		tok = newToken(token.UNDERSCORE, l.ch, tok.Line, tok.Column)
	case '\'':
		tok.Type = token.STRING
		tok.Literal = l.readString()
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

func (l *Lexer) readNumber() (token.TokenType, string) {
	position := l.position
	isFloat := false

	// Read integer part
	for isDigit(l.ch) {
		l.readChar()
	}

	// Check for decimal point
	if l.ch == '.' && isDigit(l.peekChar()) {
		isFloat = true
		l.readChar() // consume '.'
		for isDigit(l.ch) {
			l.readChar()
		}
	} else if l.ch == '.' && l.position > position {
		// Handle cases like "2." (float with no decimal digits)
		// Check if next char could be part of .. operator
		if l.peekChar() != '.' {
			isFloat = true
			l.readChar() // consume '.'
		}
	}

	// Handle ".5" style floats (leading dot)
	if position == l.position && l.ch == '.' {
		isFloat = true
		l.readChar()
		for isDigit(l.ch) {
			l.readChar()
		}
	}

	if isFloat {
		return token.FLOAT, l.input[position:l.position]
	}
	return token.INT, l.input[position:l.position]
}

func (l *Lexer) readString() string {
	l.readChar() // skip opening quote
	var buf []byte

	for {
		if l.ch == '\'' || l.ch == 0 || l.ch == '\n' {
			break
		}
		if l.ch == '\\' {
			next := l.peekChar()
			switch next {
			case '\'':
				buf = append(buf, '\'')
				l.readChar() // skip backslash
				l.readChar() // skip quote
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
				buf = append(buf, 0)
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

	if l.ch == '\'' {
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
