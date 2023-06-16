import sys
import ply.lex as lex
from lex_grammar import *
from quark_lexer import QuarkLexer
from quark_parser import TreeNode

# Lexer
lexer = QuarkLexer(lex.lex())


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
