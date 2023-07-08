import sys
import ply.lex as lex
from core.lex_grammar import *
from core.quark_lexer import QuarkLexer

# Lexer
lexer = QuarkLexer(lex.lex())


if __name__ == "__main__":
    with open(sys.argv[1], "r") as inputf:
        lexer.input(inputf.read())

        for i, tok in enumerate(lexer.token_stream):
            print(i, tok)
