package token

import "sort"

type TokenType int

const (
	// Special tokens
	ILLEGAL TokenType = iota
	EOF
	NEWLINE
	INDENT
	DEDENT
	WS

	// Identifiers and literals
	ID     // identifiers
	INT    // integer literals
	FLOAT  // float literals
	STRING // string literals

	// Operators
	PLUS       // +
	MINUS      // -
	MULTIPLY   // *
	DIVIDE     // /
	MODULO     // %
	DOUBLESTAR // **

	BANG // !

	EQUALS // =
	LT     // <
	GT     // >
	LTE    // <=
	GTE    // >=
	DEQ    // ==
	NE     // !=
	ARROW  // ->

	// Delimiters
	LPAR       // (
	RPAR       // )
	LBRACKET   // [
	RBRACKET   // ]
	LBRACE     // {
	RBRACE     // }
	DOT        // .
	COMMA      // ,
	PIPE       // |
	COLON      // :
	COLONCOLON // :: (qualified names: io::exists)
	UNDERSCORE // _

	// Keywords
	keyword_beg
	INCL
	AS
	MODULE
	IN
	AND
	OR
	IF
	ELSEIF
	ELSE
	FOR
	WHILE
	WHEN
	FN
	TRUE
	FALSE
	NULL
	OK
	ERR
	LIST
	DICT
	VEC
	RESULT
	BREAK
	CONTINUE
	EXTERN
	STRUCT
	keyword_end
)

var tokenNames = map[TokenType]string{
	ILLEGAL: "ILLEGAL",
	EOF:     "EOF",
	NEWLINE: "NEWLINE",
	INDENT:  "INDENT",
	DEDENT:  "DEDENT",
	WS:      "WS",

	ID:     "ID",
	INT:    "INT",
	FLOAT:  "FLOAT",
	STRING: "STRING",

	PLUS:       "PLUS",
	MINUS:      "MINUS",
	MULTIPLY:   "MULTIPLY",
	DIVIDE:     "DIVIDE",
	MODULO:     "MODULO",
	DOUBLESTAR: "DOUBLESTAR",

	BANG: "BANG",

	EQUALS: "EQUALS",
	LT:     "LT",
	GT:     "GT",
	LTE:    "LTE",
	GTE:    "GTE",
	DEQ:    "DEQ",
	NE:     "NE",
	ARROW:  "ARROW",

	LPAR:       "LPAR",
	RPAR:       "RPAR",
	LBRACKET:   "LBRACKET",
	RBRACKET:   "RBRACKET",
	LBRACE:     "LBRACE",
	RBRACE:     "RBRACE",
	DOT:        "DOT",
	COMMA:      "COMMA",
	PIPE:       "PIPE",
	COLON:      "COLON",
	COLONCOLON: "COLONCOLON",
	UNDERSCORE: "UNDERSCORE",

	INCL:     "INCL",
	AS:       "AS",
	MODULE:   "MODULE",
	IN:       "IN",
	AND:      "AND",
	OR:       "OR",
	IF:       "IF",
	ELSEIF:   "ELSEIF",
	ELSE:     "ELSE",
	FOR:      "FOR",
	WHILE:    "WHILE",
	WHEN:     "WHEN",
	FN:       "FN",
	TRUE:     "TRUE",
	FALSE:    "FALSE",
	NULL:     "NULL",
	OK:       "OK",
	ERR:      "ERR",
	LIST:     "LIST",
	DICT:     "DICT",
	VEC:      "VEC",
	RESULT:   "RESULT",
	BREAK:    "BREAK",
	CONTINUE: "CONTINUE",
	EXTERN:   "EXTERN",
	STRUCT:   "STRUCT",
}

func (t TokenType) String() string {
	if name, ok := tokenNames[t]; ok {
		return name
	}
	return "UNKNOWN"
}

var keywords = map[string]TokenType{
	"incl":     INCL,
	"as":       AS,
	"module":   MODULE,
	"in":       IN,
	"and":      AND,
	"or":       OR,
	"if":       IF,
	"elseif":   ELSEIF,
	"else":     ELSE,
	"for":      FOR,
	"while":    WHILE,
	"when":     WHEN,
	"fn":       FN,
	"true":     TRUE,
	"false":    FALSE,
	"null":     NULL,
	"ok":       OK,
	"err":      ERR,
	"list":     LIST,
	"dict":     DICT,
	"vec":      VEC,
	"result":   RESULT,
	"break":    BREAK,
	"continue": CONTINUE,
	"extern":   EXTERN,
	"struct":   STRUCT,
}

// Keywords returns every reserved word, sorted.
func Keywords() []string {
	out := make([]string, 0, len(keywords))
	for k := range keywords {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func LookupIdent(ident string) TokenType {
	if tok, ok := keywords[ident]; ok {
		return tok
	}
	return ID
}

type Token struct {
	Type    TokenType
	Literal string
	Line    int
	Column  int
}

func (t Token) String() string {
	return t.Type.String() + "(" + t.Literal + ")"
}
