package token

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

	BANG  // !

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
	UNDERSCORE // _

	// Keywords
	keyword_beg
	USE
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
	VECTOR
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

	BANG:  "BANG",

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
	UNDERSCORE: "UNDERSCORE",

	USE:    "USE",
	MODULE: "MODULE",
	IN:     "IN",
	AND:    "AND",
	OR:     "OR",
	IF:     "IF",
	ELSEIF: "ELSEIF",
	ELSE:   "ELSE",
	FOR:    "FOR",
	WHILE:  "WHILE",
	WHEN:   "WHEN",
	FN:     "FN",
	TRUE:   "TRUE",
	FALSE:  "FALSE",
	NULL:   "NULL",
	OK:     "OK",
	ERR:    "ERR",
	LIST:   "LIST",
	DICT:   "DICT",
	VECTOR: "VECTOR",
}

func (t TokenType) String() string {
	if name, ok := tokenNames[t]; ok {
		return name
	}
	return "UNKNOWN"
}

var keywords = map[string]TokenType{
	"use":    USE,
	"module": MODULE,
	"in":     IN,
	"and":    AND,
	"or":     OR,
	"if":     IF,
	"elseif": ELSEIF,
	"else":   ELSE,
	"for":    FOR,
	"while":  WHILE,
	"when":   WHEN,
	"fn":     FN,
	"true":   TRUE,
	"false":  FALSE,
	"null":   NULL,
	"ok":     OK,
	"err":    ERR,
	"list":   LIST,
	"dict":   DICT,
	"vector": VECTOR,
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
