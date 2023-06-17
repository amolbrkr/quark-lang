import sys
import ply.lex as lex
from lex_grammar import *
from quark_lexer import QuarkLexer

# Lexer
lexer = QuarkLexer(lex.lex())


if __name__ == "__main__":
    with open(sys.argv[1], "r") as inputf:
        lexer.input(inputf.read())
        tokens = lexer.token_stream
        for i, tok in enumerate(tokens):
            print(i, tok)
