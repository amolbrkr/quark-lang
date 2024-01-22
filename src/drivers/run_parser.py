import sys
import ply.lex as lex
from utils import treeviz
from core.lex_grammar import *
from core.quark_lexer import QuarkLexer
from core.quark_parser import QuarkParser

# Lexer
lexer = QuarkLexer(lex.lex())

if __name__ == "__main__":
    with open(sys.argv[1], "r") as inputf:
        lexer.input(inputf.read())
        parser = QuarkParser(lexer.token_stream)

        parser.parse()
        viz = treeviz.TreeViz()

        if not parser.tree:
            raise Exception("Parser failed, tree is null.")

        viz.generate(parser.tree)
        viz.save()
