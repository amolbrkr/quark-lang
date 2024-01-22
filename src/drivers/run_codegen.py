import sys
import ply.lex as lex
from core.lex_grammar import *
from core.helper_types import *
from core.quark_lexer import QuarkLexer
from core.quark_parser import QuarkParser
from core.quark_codegen import QuarkCG


if __name__ == "__main__":
    with open(sys.argv[1], "r") as inputf:
        lexer = QuarkLexer(lex.lex())
        lexer.input(inputf.read())

        parser = QuarkParser(lexer.token_stream)
        parser.parse()

        if not parser.tree:
            raise Exception("Parser failed.")
            
        cg = QuarkCG(parser.tree)
        asm = cg.generate()
        print(asm)

            
