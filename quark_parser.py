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
    tok: Token = None
    children: list = field(default_factory=list)

    def __str__(self):
        return f"{self.type}[{self.tok.value if self.tok else 'None'}]"


class QuarkParser:
    def __init__(self, token_stream):
        self.tree = None
        self.errors = []
        self.tokens = list(token_stream)

    ## Util functions
    def cur(self):
        t = self.tokens[0]
        print("cur", t)
        return t

    def peek(self, index=1):
        t = self.tokens[index] if index < len(self.tokens) else None
        print("peek", t)
        return t

    def consume(self):
        t = self.tokens.pop(0)
        print("cons", t)
        return t

    def is_term(self, token):
        return token.type in ["ID", "INT", "FLOAT", "STR"]

    def error(self, msg):
        self.errors.append(f"ParseError: {msg}")

    ## Parsing functions
    def block(self):
        print(f"Block: {self.peek()}")
        node = None

        while self.cur().type in ["NEWLINE", "INDENT"]:
            self.consume()
        else:
            node = self.statement()

            while self.cur().type in ["NEWLINE", "DEDENT"]:
                self.consume()

        return node

    def statement(self):
        print(f"Statement: {self.peek()}")
        node = None

        match self.cur().type:
            case "if":
                node = self.ifelse()
            case "fn":
                node = self.function()
            case _:
                node = self.expression()

        return node

    def expression(self):
        print(f"Expression: {self.peek()}")
        node = None

        if self.cur().type == "ID" and self.peek().type == "EQUALS":
            lterm = TreeNode(NodeType.Identifier, self.consume())
            node = TreeNode(NodeType.Operator, self.consume())
            node.children.extend([lterm, self.expression()])
        elif self.is_term(self.cur()):
            node = TreeNode(
                NodeType.Identifier if self.cur().type == "ID" else NodeType.Literal,
                self.consume(),
            )

        return node

    def function(self):
        pass

    def ifelse(sefl):
        pass

    def parse(self):
        self.tree = TreeNode(NodeType.CompilationUnit)
        self.tree.children.append(self.block())

        if self.cur().type != "EOF":
            self.error(f"Expected EOF but got {self.cur().value}")
