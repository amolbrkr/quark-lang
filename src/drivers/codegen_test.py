import sys
import ply.lex as lex
from utils import treeviz
from core.lex_grammar import *
from core.quark_lexer import QuarkLexer
from core.quark_parser import QuarkParser
from core.codegen import QuarkCodeGen


qc = QuarkCodeGen()

# Lexer
lexer = QuarkLexer(lex.lex())

if __name__ == "__main__":
    with open(sys.argv[1], "r") as inputf:
        lexer.input(inputf.read())
        parser = QuarkParser(lexer.token_stream)

        parser.parse()
        viz = treeviz.TreeViz()
        if parser.tree:
            print("Codegen begins\n")
            qc.generate(parser.tree)
            print(qc.cur_module)
