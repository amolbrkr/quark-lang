import sys
import ply.lex as lex
from lex_grammar import *
from indent_lexer import IndentLexer

# Lexer
lexer = IndentLexer(lex.lex())


if __name__ == "__main__":
    with open(sys.argv[1], "r") as inputf:
        lexer.input(inputf.read())
        count = 0
        while True:
            tok = lexer.token()
            if not tok:
                break
            print(f"#{count}   {tok}")
            count += 1
