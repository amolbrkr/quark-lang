from ply.lex import Token
from quark_lexer import QuarkLexer
from dataclasses import dataclass


(
    CompilationUnit,
    Statement,
    Expression,
    Function,
    Argument,
    Identifier,
    Literal,
    Operator,
) = range(7)


@dataclass
class TreeNode:
    type: int
    t: Token
    left: any = None
    right: any = None


class QuarkParser:
    def __init__(self, lexer):
        self.tree = None
        self.lexer = lexer
        self.tokens = self.lexer.token_stream
    
    def token(self):
        return next(self.tokens)

    def parse_statement(self):
        t = self.token()
        if t.type == 'ID':
            n1 = TreeNode(Identifier, t)
            n = self.token()
            if n.type == 'EQUALS':
                n2 = TreeNode(Operator, n, left=n1)
    
    def parse_function(self):
        pass
