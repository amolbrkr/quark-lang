from ply.lex import Token
from quark_lexer import QuarkLexer
from dataclasses import dataclass


(
    CompilationUnit,
    Statement,
    Expression,
    Condition,
    Function,
    Assignement,
    MathExpression,
    Argument,
    Identifier,
    Literal,
    Operator,
) = range(11)


@dataclass
class TreeNode:
    type: int
    t: Token
    left: any = None
    mid: any = None
    right: any = None


class QuarkParser:
    def __init__(self, lexer):
        self.tok = None
        self.tree = None
        self.lexer = lexer

    def next_token(self):
        self.tok = self.lexer.token()

    def error(self, msg):
        raise Exception(f"ParseError: {msg}")

    def parse_stat(self):
        print(f"parse_statement: {self.tok}")
        n = None

        if self.tok.type == "IF":
            n = TreeNode(Statement, self.tok)
            self.next_token()
            n.left = self.parse_expr()
            n.mid = self.parse_stat()
            if self.tok.type == "ELSE":
                self.next_token()
                n.right = self.parse_stat()
        elif self.tok.type == "FN":
            n = TreeNode(Function, self.tok, left=self.parse_func())
        else:
            n = TreeNode(Expression, self.tok, left=self.parse_expr())

        print(f"Node: {n}")
        return n

    def parse_expr(self):
        print(f"parse_expr: {self.tok}")
        n = None

        if self.tok.type == "ID":
            self.next_token()
            if self.tok.type == 'EQUALS':
                    
            else:
                self.parse_cond()

    def parse_asgn(self):
        print(f"parse_asgn: {self.tok}")

    def parse_func(self):
        print(f"parse_func: {self.tok}")

    def parse(self):
        self.next_token()
        self.tree = TreeNode(CompilationUnit, self.tok, left=self.parse_stat())

        if self.tok.type != "EOF":
            self.error(f"Expected EOF but got {self.tok.value}")
