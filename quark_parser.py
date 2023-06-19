from enum import Enum
from ply.lex import Token
from dataclasses import dataclass, field


class NodeType(Enum):
    CompilationUnit = 0
    Block = 1
    Statement = 2
    Expression = 3
    Condition = 4
    Function = 5
    Argument = 6
    Identifier = 7
    Literal = 8
    Assignment = 9
    Operator = 10


@dataclass
class TreeNode:
    type: NodeType
    tok: Token
    children: list = field(default_factory=list)

    def __str__(self):
        return f"{self.type}[{self.tok.type}]"


class QuarkParser:
    def __init__(self, lexer):
        self.tok = None
        self.tree = None
        self.errors = []
        self.lexer = lexer

    def _is_term(self):
        return self.tok.type in ["ID", "INT", "FLOAT", "STR"]

    def _next(self):
        self.tok = self.lexer.token()

    def _error(self, msg):
        self.errors.append(f"ParseError: {msg} at {self.tok.lineno}")

    def _parse_bloc(self):
        print(f"parse_bloc: {self.tok}")
        n = None

        while self.tok.type in ["NEWLINE", "INDENT"]:
            self._next()
        else:
            n = self._parse_stat()
            self._next()

            while self.tok.type in ["NEWLINE", "DEDENT"]:
                self._next()
            else:
                self._error(
                    f"Expected newline but got {self.tok.value}, did you forgot to add a new line?"
                )

        return n

    def _parse_stat(self):
        print(f"parse_stat: {self.tok}")
        n = None

        if self.tok.type == "if":
            pass
        elif self.tok.type == "fn":
            pass
        elif self._is_term():
            n = self._parse_expr()
        else:
            self._error(f"Unexpected token: {self.tok.value}")

        return n

    def _parse_term(self):
        if not self._is_term():
            self.error(f"Expected Identifier or Literal got '{self.tok.value}'")

        return TreeNode(
            NodeType.Identifier if self.tok.type == "ID" else NodeType.Literal, self.tok
        )

    def _parse_expr(self):
        print(f"parse_expr: {self.tok}")
        n = None

        if self._is_term():
            lterm = self._parse_term()
            self._next()
            if self.tok.type in [
                "PLUS",
                "MINUS",
                "MULTIPLY",
                "DIVIDE",
                "GT",
                "LT",
                "GTE",
                "LTE",
                "DEQ",
                "NE",
                "EQUALS",
            ]:
                n = TreeNode(NodeType.Operator, self.tok)
                n.children.append(lterm)
                self._next()
                n.children.append(self._parse_expr())
            else:
                n = lterm
        elif self.tok.type == "LPAR":
            self._next()
            n = self._parse_expr()
        else:
            self.error(f"Expected Term or '(' but got '{self.tok.value}'")

        return n

    def _parse_asgn(self):
        print(f"parse_asgn: {self.tok}")

    def _parse_func(self):
        print(f"parse_func: {self.tok}")

    def _print_all(self, nodes):
        for node in nodes:
            print(node)
            if node and len(node.children) > 0:
                self._print_all(node.children)

    def parse(self):
        self._next()
        self.tree = TreeNode(NodeType.CompilationUnit, self.tok)
        self.tree.children.append(self._parse_bloc())
        self._print_all([self.tree])

        if self.tok.type != "EOF":
            self._error(f"Expected EOF but got {self.tok.value}")
