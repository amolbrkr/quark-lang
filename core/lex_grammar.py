reserved = {
    "use": "USE",
    "module": "MODULE",
    "in": "IN",
    "and": "AND",
    "or": "OR",
    "if": "IF",
    "elif": "ELIF",
    "else": "ELSE",
    "for": "FOR",
    "while": "WHILE",
    "fn": "FN",
    "class": "CLASS",
}

tokens = (
    "ID",  # Identifiers
    "PLUS",  # +
    "MINUS",  # -
    "MULTIPLY",  # *
    "DIVIDE",  # /
    "MODULO",  # %
    "NOT",  # ~
    "EQUALS",  # =
    "LT",  # <
    "GT",  # >
    "LTE",  # <=
    "GTE",  # >=
    "DOUBLEEQUAL",  # ==
    "NE",  # #
    "LPAR",  # (
    "RPAR",  # )
    "LBRACE",  # [
    "RBRACE",  # ]
    "BLOCKSTART",  # {
    "BLOCKEND",  # }
    "INT",  # int
    "FLOAT",  # float
    "STR",  # str
    "DOT",  # .
    "COMMA",  # ,
    "QUOTES",  # '
    "DOUBLEQUOTES",  # "
    "PIPE",
    "COLON",  # :
    "COMMENT",  # //
    "WS",  # Whitespaces
    "NEWLINE",  # \n
    "INDENT",   # + Indent
    "DEDENT",   # - Indent
    "ENDMARKER" # To be used by parser to know when a block has ended
)

t_PLUS = r"\+"
t_MINUS = r"-"
t_MULTIPLY = r"\*"
t_DIVIDE = r"/"
t_MODULO = r"%"

t_NOT = r"\~"
t_EQUALS = r"\="

t_LT = r"\<"
t_GT = r"\>"
t_LTE = r"\<\="
t_GTE = r"\>\="
t_DOUBLEEQUAL = r"\=\="
t_NE = r"\!\="

t_LBRACE = r"\["
t_RBRACE = r"\]"

t_DOT = r"\."
t_COMMA = r"\,"
t_QUOTES = r"\'"
t_DOUBLEQUOTES = r'"'
t_PIPE = r"\|"
t_COLON = r":"


# Identifier
def t_ID(t):
    r"[a-zA-Z_][a-zA-Z_0-9]*"
    t.type = reserved.get(t.value, "ID")
    return t


# Data Types
t_STR = r'"([^"\n]|(\\"))*"'


def t_INT(t):
    r"\d+"
    t.value = int(t.value)
    return t


def t_FLOAT(t):
    r"(\d*\.\d+)|(\d+\.\d*)"
    t.value = float(t.value)
    return t


# Parentheses
def t_LPAR(t):
    r"\("
    t.lexer.paren_count += 1
    return t


def t_RPAR(t):
    r"\)"
    # check for underflow?  should be the job of the parser
    t.lexer.paren_count -= 1
    return t


# Misc
def t_WS(t):
    r"[ ]+"
    if t.lexer.at_line_start and t.lexer.paren_count == 0:
        return t


def t_newline(t):
    r"\n+"
    t.lineno += len(t.value)
    t.type = "NEWLINE"
    if t.lexer.paren_count == 0:
        return t


def t_error(t):
    print(f"Illegal Character: '{t.value[0]}'")
    t.lexer.skip(1)


t_ignore_COMMENT = r"\//.*"
# t_ignore = r" \t"
