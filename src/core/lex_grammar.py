reserved = {
    "use": "USE",
    "module": "MODULE",
    "in": "IN",
    "and": "AND",
    "or": "OR",
    "if": "IF",
    "elseif": "ELSEIF",
    "else": "ELSE",
    "for": "FOR",
    "while": "WHILE",
    "when": "WHEN",
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
    "DOUBLESTAR",  # **
    "AMPER", # &
    "NOT",  # ~
    "BANG",  # !
    "EQUALS",  # =
    "LT",  # <
    "GT",  # >
    "LTE",  # <=
    "GTE",  # >=
    "DEQ",  # ==
    "NE",  # !=
    "LPAR",  # (
    "RPAR",  # )
    "LBRACE",  # [
    "RBRACE",  # ]
    "BLOCKSTART",  # {
    "BLOCKEND",  # }
    "INT",  # int
    "FLOAT",  # float
    "STR",  # str
    "AT",   # @
    "DOT",  # .
    "DOTDOT",  # ..
    "COMMA",  # ,
    "PIPE",  # |
    "COLON",  # :
    "UNDERSCORE",  # _
    "COMMENT",  # //
    "WS",  # Whitespaces
    "NEWLINE",  # \n
    "INDENT",   # + Indent
    "DEDENT",   # - Indent
    "EOF"
)

# Order matters - longer patterns first!
t_DOUBLESTAR = r"\*\*"
t_DOTDOT = r"\.\."
t_LTE = r"\<\="
t_GTE = r"\>\="
t_DEQ = r"\=\="
t_NE = r"\!\="

t_PLUS = r"\+"
t_MINUS = r"-"
t_MULTIPLY = r"\*"
t_DIVIDE = r"/"
t_MODULO = r"%"
t_AMPER = r"&"

t_NOT = r"\~"
t_BANG = r"\!"
t_EQUALS = r"\="

t_LT = r"\<"
t_GT = r"\>"

t_LBRACE = r"\["
t_RBRACE = r"\]"

t_DOT = r"\."
t_AT = r"@"
t_COMMA = r"\,"
t_PIPE = r"\|"
t_COLON = r":"


# Underscore wildcard (must be a function to be checked before t_ID)
def t_UNDERSCORE(t):
    r"_"
    return t


# Identifier
def t_ID(t):
    r"[a-zA-Z_][a-zA-Z_0-9]*"
    t.type = reserved.get(t.value, "ID")
    return t


# Data Types - Single-quoted strings only (for now)
def t_STR(t):
    r"'([^'\n]|(\\'))*'"
    # Remove quotes from the value
    t.value = t.value[1:-1]
    return t


def t_FLOAT(t):
    r"(\d*\.\d+)|(\d+\.\d*)"
    t.value = float(t.value)
    return t


def t_INT(t):
    r"\d+"
    t.value = int(t.value)
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
    r"[ \t]+"
    if t.lexer.at_line_start and t.lexer.paren_count == 0:
        return t


def t_newline(t):
    r"(\r?\n)+"
    t.lexer.lineno += t.value.count('\n')
    t.type = "NEWLINE"
    if t.lexer.paren_count == 0:
        return t


def t_error(t):
    print(f"Illegal Character: '{t.value[0]}'")
    t.lexer.skip(1)


t_ignore_COMMENT = r"\//.*"
