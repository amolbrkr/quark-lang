from ply.lex import Token
from quark_lexer import QuarkLexer
from dataclasses import dataclass


@dataclass
class TreeNode:
    t: Token
    left: any = None
    right: any = None


@dataclass
class QuarkParser:
    lexer: QuarkLexer

    def parse(self):
        pass
