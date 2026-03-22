package lexer_test

import (
	"testing"

	"quark/internal/testutil"
	"quark/token"
)

// --- Indentation ---

func TestIndentation_ProperIndentEmitsIndentDedent(t *testing.T) {
	src := "if true:\n    println(1)\nprintln(2)\n"
	toks := testutil.Lex(src)

	var hasIndent, hasDedent bool
	for _, tok := range toks {
		if tok.Type == token.INDENT {
			hasIndent = true
		}
		if tok.Type == token.DEDENT {
			hasDedent = true
		}
	}
	if !hasIndent || !hasDedent {
		t.Fatalf("expected INDENT and DEDENT tokens, got=%v", toks)
	}
}

func TestIndentation_MissingIndentAfterColonYieldsIllegal(t *testing.T) {
	src := "if true:\nprintln(1)\n"
	toks := testutil.Lex(src)

	for _, tok := range toks {
		if tok.Type == token.ILLEGAL && tok.Literal == "expected indented block" {
			return
		}
	}
	t.Fatalf("expected ILLEGAL token with 'expected indented block', got=%v", toks)
}

func TestIndentation_NestedIndent(t *testing.T) {
	src := "if true:\n    if false:\n        x = 1\n"
	toks := testutil.Lex(src)

	indentCount := 0
	dedentCount := 0
	for _, tok := range toks {
		if tok.Type == token.INDENT {
			indentCount++
		}
		if tok.Type == token.DEDENT {
			dedentCount++
		}
	}
	if indentCount != 2 {
		t.Fatalf("expected 2 INDENT tokens for nested blocks, got %d", indentCount)
	}
	if dedentCount != 2 {
		t.Fatalf("expected 2 DEDENT tokens for nested blocks, got %d", dedentCount)
	}
}

func TestIndentation_InconsistentDedentYieldsIllegal(t *testing.T) {
	// Indent to 4, then dedent to 3 (not a valid level)
	src := "if true:\n    x = 1\n   y = 2\n"
	toks := testutil.Lex(src)

	for _, tok := range toks {
		if tok.Type == token.ILLEGAL && tok.Literal == "inconsistent indentation" {
			return
		}
	}
	t.Fatalf("expected ILLEGAL 'inconsistent indentation', got=%v", toks)
}

func TestIndentation_SuppressedInsideParens(t *testing.T) {
	src := "f(\n    1,\n    2\n)\n"
	toks := testutil.Lex(src)

	for _, tok := range toks {
		if tok.Type == token.INDENT || tok.Type == token.DEDENT {
			t.Fatalf("expected no INDENT/DEDENT inside parens, got %s", tok.Type)
		}
	}
}

func TestIndentation_SuppressedInsideBrackets(t *testing.T) {
	src := "list [\n    1,\n    2\n]\n"
	toks := testutil.Lex(src)

	for _, tok := range toks {
		if tok.Type == token.INDENT || tok.Type == token.DEDENT {
			t.Fatalf("expected no INDENT/DEDENT inside brackets, got %s", tok.Type)
		}
	}
}

func TestIndentation_SuppressedInsideBraces(t *testing.T) {
	src := "dict {\n    a: 1,\n    b: 2\n}\n"
	toks := testutil.Lex(src)

	for _, tok := range toks {
		if tok.Type == token.INDENT || tok.Type == token.DEDENT {
			t.Fatalf("expected no INDENT/DEDENT inside braces, got %s", tok.Type)
		}
	}
}

func TestIndentation_BlankLinesIgnored(t *testing.T) {
	src := "x = 1\n\n\ny = 2\n"
	toks := testutil.Lex(src)

	// Should produce tokens for x=1 and y=2 with no INDENT/DEDENT
	for _, tok := range toks {
		if tok.Type == token.INDENT || tok.Type == token.DEDENT {
			t.Fatalf("expected no INDENT/DEDENT from blank lines, got %s", tok.Type)
		}
	}
}

// --- Keywords ---

func TestVectorKeyword_TokenizedAsKeyword(t *testing.T) {
	src := "x = vector [1, 2, 3]\n"
	toks := testutil.Lex(src)

	for _, tok := range toks {
		if tok.Type == token.VECTOR {
			return
		}
	}
	t.Fatalf("expected VECTOR token, got=%v", toks)
}

func TestResultKeyword_TokenizedAsKeyword(t *testing.T) {
	src := "x: result = ok 1\n"
	toks := testutil.Lex(src)

	for _, tok := range toks {
		if tok.Type == token.RESULT {
			return
		}
	}
	t.Fatalf("expected RESULT token, got=%v", toks)
}

func TestKeywords_AllRecognized(t *testing.T) {
	keywords := map[string]token.TokenType{
		"use": token.USE, "as": token.AS, "module": token.MODULE,
		"in": token.IN, "and": token.AND, "or": token.OR,
		"if": token.IF, "elseif": token.ELSEIF, "else": token.ELSE,
		"for": token.FOR, "while": token.WHILE, "when": token.WHEN,
		"fn": token.FN, "true": token.TRUE, "false": token.FALSE,
		"null": token.NULL, "ok": token.OK, "err": token.ERR,
		"list": token.LIST, "dict": token.DICT, "vector": token.VECTOR,
		"result": token.RESULT, "break": token.BREAK, "continue": token.CONTINUE,
	}
	for kw, expected := range keywords {
		toks := testutil.Lex(kw + "\n")
		found := false
		for _, tok := range toks {
			if tok.Type == expected {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("keyword %q not tokenized as %s", kw, expected)
		}
	}
}

func TestKeywords_NotRecognizedInIdentifiers(t *testing.T) {
	// "ifx" should be ID, not IF
	toks := testutil.Lex("ifx\n")
	for _, tok := range toks {
		if tok.Type == token.ID && tok.Literal == "ifx" {
			return
		}
	}
	t.Fatalf("expected 'ifx' as ID token")
}

// --- String literals ---

func TestStringLiteral_DoubleQuotedAndEscaped(t *testing.T) {
	src := "x = \"line1\\nline2\"\n"
	toks := testutil.Lex(src)

	for _, tok := range toks {
		if tok.Type == token.STRING {
			if tok.Literal != "line1\nline2" {
				t.Fatalf("expected escaped double-quoted string literal, got %q", tok.Literal)
			}
			return
		}
	}
	t.Fatalf("expected STRING token, got=%v", toks)
}

func TestStringLiteral_SingleQuoted(t *testing.T) {
	toks := testutil.Lex("'hello'\n")
	for _, tok := range toks {
		if tok.Type == token.STRING {
			if tok.Literal != "hello" {
				t.Fatalf("expected 'hello', got %q", tok.Literal)
			}
			return
		}
	}
	t.Fatalf("expected STRING token")
}

func TestStringLiteral_EscapeSequences(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{`'\n'`, "\n"},
		{`'\t'`, "\t"},
		{`'\r'`, "\r"},
		{`'\\'`, "\\"},
		{`'\0'`, "\x00"},
		{`'\''`, "'"},
		{`"hello \"world\""`, `hello "world"`},
	}
	for _, tt := range tests {
		toks := testutil.Lex(tt.input + "\n")
		found := false
		for _, tok := range toks {
			if tok.Type == token.STRING {
				if tok.Literal != tt.expected {
					t.Errorf("input %s: expected %q, got %q", tt.input, tt.expected, tok.Literal)
				}
				found = true
				break
			}
		}
		if !found {
			t.Errorf("input %s: no STRING token found", tt.input)
		}
	}
}

func TestStringLiteral_EmptyString(t *testing.T) {
	toks := testutil.Lex("''\n")
	for _, tok := range toks {
		if tok.Type == token.STRING {
			if tok.Literal != "" {
				t.Fatalf("expected empty string, got %q", tok.Literal)
			}
			return
		}
	}
	t.Fatalf("expected STRING token for empty string")
}

// --- Numbers ---

func TestNumber_Integer(t *testing.T) {
	toks := testutil.Lex("42\n")
	for _, tok := range toks {
		if tok.Type == token.INT && tok.Literal == "42" {
			return
		}
	}
	t.Fatalf("expected INT token '42'")
}

func TestNumber_Float(t *testing.T) {
	toks := testutil.Lex("3.14\n")
	for _, tok := range toks {
		if tok.Type == token.FLOAT && tok.Literal == "3.14" {
			return
		}
	}
	t.Fatalf("expected FLOAT token '3.14'")
}

func TestNumber_FloatNoDec(t *testing.T) {
	// "2." should be treated as float
	toks := testutil.Lex("2.\n")
	for _, tok := range toks {
		if tok.Type == token.FLOAT {
			return
		}
	}
	t.Fatalf("expected FLOAT token for '2.'")
}

// --- Operators ---

func TestOperators_AllSingleChar(t *testing.T) {
	tests := []struct {
		input    string
		expected token.TokenType
	}{
		{"+", token.PLUS},
		{"-", token.MINUS},
		{"*", token.MULTIPLY},
		{"/", token.DIVIDE},
		{"%", token.MODULO},
		{"=", token.EQUALS},
		{"<", token.LT},
		{">", token.GT},
		{"!", token.BANG},
		{".", token.DOT},
		{",", token.COMMA},
		{"|", token.PIPE},
		{":", token.COLON},
		{"_", token.UNDERSCORE},
	}
	for _, tt := range tests {
		toks := testutil.Lex(tt.input + "\n")
		found := false
		for _, tok := range toks {
			if tok.Type == tt.expected {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("input %q: expected %s token", tt.input, tt.expected)
		}
	}
}

func TestOperators_AllTwoChar(t *testing.T) {
	tests := []struct {
		input    string
		expected token.TokenType
	}{
		{"==", token.DEQ},
		{"!=", token.NE},
		{"<=", token.LTE},
		{">=", token.GTE},
		{"->", token.ARROW},
		{"**", token.DOUBLESTAR},
	}
	for _, tt := range tests {
		toks := testutil.Lex(tt.input + "\n")
		found := false
		for _, tok := range toks {
			if tok.Type == tt.expected {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("input %q: expected %s token", tt.input, tt.expected)
		}
	}
}

// --- Delimiters ---

func TestDelimiters_MatchedBrackets(t *testing.T) {
	tests := []struct {
		open  string
		close string
		openT token.TokenType
		closeT token.TokenType
	}{
		{"(", ")", token.LPAR, token.RPAR},
		{"[", "]", token.LBRACKET, token.RBRACKET},
		{"{", "}", token.LBRACE, token.RBRACE},
	}
	for _, tt := range tests {
		toks := testutil.Lex(tt.open + "1" + tt.close + "\n")
		var foundOpen, foundClose bool
		for _, tok := range toks {
			if tok.Type == tt.openT {
				foundOpen = true
			}
			if tok.Type == tt.closeT {
				foundClose = true
			}
		}
		if !foundOpen || !foundClose {
			t.Errorf("expected %s and %s tokens", tt.openT, tt.closeT)
		}
	}
}

// --- Comments ---

func TestComments_LineCommentIgnored(t *testing.T) {
	toks := testutil.Lex("x = 1 // this is a comment\ny = 2\n")
	for _, tok := range toks {
		if tok.Type == token.STRING || (tok.Type == token.ID && tok.Literal == "this") {
			t.Fatalf("comment content should not appear as token: %v", tok)
		}
	}
}

func TestComments_FullLineComment(t *testing.T) {
	toks := testutil.Lex("// full line comment\nx = 1\n")
	for _, tok := range toks {
		if tok.Type == token.ID && tok.Literal == "x" {
			return
		}
	}
	t.Fatalf("expected 'x' after full-line comment")
}

// --- Line/column tracking ---

func TestLineColumn_BasicTracking(t *testing.T) {
	toks := testutil.Lex("x = 1\ny = 2\n")
	var xTok, yTok token.Token
	for _, tok := range toks {
		if tok.Type == token.ID && tok.Literal == "x" {
			xTok = tok
		}
		if tok.Type == token.ID && tok.Literal == "y" {
			yTok = tok
		}
	}
	if xTok.Line != 1 {
		t.Errorf("expected x on line 1, got %d", xTok.Line)
	}
	if yTok.Line != 2 {
		t.Errorf("expected y on line 2, got %d", yTok.Line)
	}
}

// --- Edge cases ---

func TestEOF_EmptyInput(t *testing.T) {
	toks := testutil.Lex("")
	if len(toks) == 0 {
		t.Fatalf("expected at least EOF token")
	}
	if toks[len(toks)-1].Type != token.EOF {
		t.Fatalf("expected last token to be EOF")
	}
}

func TestEOF_NoTrailingNewline(t *testing.T) {
	toks := testutil.Lex("x = 1")
	// Should still produce tokens without crashing
	found := false
	for _, tok := range toks {
		if tok.Type == token.ID && tok.Literal == "x" {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected 'x' token even without trailing newline")
	}
}

func TestIllegal_UnknownCharacter(t *testing.T) {
	toks := testutil.Lex("@\n")
	for _, tok := range toks {
		if tok.Type == token.ILLEGAL {
			return
		}
	}
	t.Fatalf("expected ILLEGAL token for '@'")
}

func TestUnderscore_AsIdentifierPrefix(t *testing.T) {
	toks := testutil.Lex("_foo = 1\n")
	for _, tok := range toks {
		if tok.Type == token.ID && tok.Literal == "_foo" {
			return
		}
	}
	t.Fatalf("expected '_foo' as ID token")
}

func TestUnderscore_Standalone(t *testing.T) {
	toks := testutil.Lex("_\n")
	for _, tok := range toks {
		if tok.Type == token.UNDERSCORE {
			return
		}
	}
	t.Fatalf("expected UNDERSCORE token for standalone '_'")
}

// --- CRLF handling ---

func TestCRLF_NormalizedToNewline(t *testing.T) {
	toks := testutil.Lex("x = 1\r\ny = 2\r\n")
	newlineCount := 0
	for _, tok := range toks {
		if tok.Type == token.NEWLINE {
			newlineCount++
		}
	}
	if newlineCount < 1 {
		t.Fatalf("expected NEWLINE tokens from CRLF input, got %d", newlineCount)
	}
}
