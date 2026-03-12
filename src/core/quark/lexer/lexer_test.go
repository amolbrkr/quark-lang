package lexer_test

import (
	"testing"

	"quark/internal/testutil"
	"quark/token"
)

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
