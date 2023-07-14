import os
import sys
import ctypes
import ply.lex as lex
from utils import genctree
from core.lex_grammar import *
from core.helper_types import *
from core.quark_lexer import QuarkLexer
from core.quark_parser import QuarkParser
import quark_codegen as cg

# Lexer
lexer = QuarkLexer(lex.lex())

# C++ Library for codegen
# lib_codegen = ctypes.cdll.LoadLibrary(
#     os.path.abspath("../src/core/codegen/build/libquark-codegen.so")
# )


if __name__ == "__main__":
    with open(sys.argv[1], "r") as inputf:
        lexer.input(inputf.read())
        parser = QuarkParser(lexer.token_stream)
        parser.parse()

        if parser.tree:
            cg.initCodegen(parser.tree)
