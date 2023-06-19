import sys
import ply.lex as lex
from utils import treeviz
from lex_grammar import *
from quark_lexer import QuarkLexer
from quark_parser import QuarkParser

# Lexer
lexer = QuarkLexer(lex.lex())

if __name__ == "__main__":
    with open(sys.argv[1], "r") as inputf:
        lexer.input(inputf.read())
        parser = QuarkParser(lexer)

        parser.parse()
        viz = treeviz.TreeViz()
        viz.generate(parser.tree)
        viz.save()
