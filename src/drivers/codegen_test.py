import os
import sys
import ctypes
import ply.lex as lex
from utils import genctree
from core.lex_grammar import *
from core.helper_types import TreeNode
from core.quark_lexer import QuarkLexer
from core.quark_parser import QuarkParser


# Lexer
lexer = QuarkLexer(lex.lex())

# C++ Library for codegen
lib_codegen = ctypes.cdll.LoadLibrary(
    os.path.abspath("../src/core/codegen/build/libquark-codegen.so")
)

# lib_codegen.consumeTree.argtypes = [ctypes.POINTER(TreeNode)]
# lib_codegen.consumeTree.restype = None

if __name__ == "__main__":
    with open(sys.argv[1], "r") as inputf:
        lexer.input(inputf.read())
        parser = QuarkParser(lexer.token_stream)
        parser.parse()

        if parser.tree:
            print("Codegen begins\n")

            genctree.gen_c_tree(parser.tree)
            # lib_codegen.consumeTree(ctypes.byref(parser.tree))

            # proto = ctypes.CFUNCTYPE(ctypes.c_void_p, ctypes.py_object)
            # consume_tree = proto(("consumeTree", lib_codegen))
            # consume_tree(parser.tree)
