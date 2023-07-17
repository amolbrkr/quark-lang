import sys
import ply.lex as lex
from core.lex_grammar import *
from core.helper_types import *
from core.quark_lexer import QuarkLexer
from core.quark_parser import QuarkParser
import pytreetonative as cg

lexer = QuarkLexer(lex.lex())

if __name__ == "__main__":
    with open(sys.argv[1], "r") as inputf:
        lexer.input(inputf.read())
        parser = QuarkParser(lexer.token_stream)
        parser.parse()

        if parser.tree:
            cg.initCodegen(parser.tree)
