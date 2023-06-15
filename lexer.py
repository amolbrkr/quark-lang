from lex_grammar import *
import ply.lex as lex
from indent_lexer import IndentLexer

# Lexer
lexer = IndentLexer(lex.lex())

code = """
// This is a comment
fn addEven x, y:
    if x % 2 == 0:
        x + y

add(2, 5)
"""

lexer.input(code)

while True:
    tok = lexer.token()
    if not tok:
        break
    print(tok)
