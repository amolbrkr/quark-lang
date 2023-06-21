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
    Arguments = 6
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
        return self.tokens[0]

    def peek(self, index=1):
        return self.tokens[index] if index < len(self.tokens) else None

    def consume(self):
        return self.tokens.pop(0)

    def is_term(self, token):
        return token.type in ["ID", "INT", "FLOAT", "STR"]

    def error(self, msg):
        self.errors.append(f"ParseError: {msg}")

    def expect(self, type):
        if self.cur().type == type:
            return self.consume()
        else:
            raise Exception(f"Expected {type} but got {self.cur().type}.")

    ## Parsing functions
    def block(self):
        print(f"Block: {self.cur()}")
        node = None

        while self.cur().type in ["NEWLINE", "INDENT"]:
            self.consume()
        else:
            node = self.statement()

            while self.cur().type in ["NEWLINE", "DEDENT"]:
                self.consume()

        return node

    def statement(self):
        print(f"Statement: {self.cur()}")
        node = None

        if self.cur().type == "IF":
            node = self.ifelse()
        elif "FN" in [self.cur().type, self.peek(2).type]:
            node = self.function()
        else:
            node = self.expression()

        return node

    def expression(self):
        print(f"Expression: {self.cur()}")
        node = None

        if self.cur().type == "ID" and self.peek().type == "EQUALS":
            lterm = TreeNode(NodeType.Identifier, self.consume())
            node = TreeNode(NodeType.Operator, self.consume())
            node.children.extend([lterm, self.expression()])
        elif self.is_term(self.cur()):
            node = self.term()
        return node

    def function(self):
        print(f"Funciton: {self.cur()}")
        node = None

        # fn - root
        # id, args, block - children
        if self.cur().type == "FN":
            node = TreeNode(NodeType.Function, self.consume())
            node.children.extend([self.expect("ID"), self.arguments()])
            self.expect("COLON")
            node.children.append(self.block)
        elif self.peek(2).type == "FN":
            id = TreeNode(NodeType.Identifier, self.expect("ID"))
            self.expect("EQUALS")
            node = TreeNode(NodeType.Function, self.consume())
            node.children.extend([id, self.arguments()])
            self.expect("COLON")
            node.children.append(self.block())
        elif self.cur().type == "ID":
            pass
        elif self.cur().type == "LPAR":
            pass

        return node

    def arguments(self):
        print(f"Arguments: {self.cur()}")
        node = TreeNode(NodeType.ArgumentList)

        while self.cur().type != "COLON":
            node.children.append(self.expression())

            if self.cur().type == "COMMA":
                self.consume()

        return node

    def ifelse(self):
        pass

    def term(self):
        return TreeNode(
            NodeType.Identifier if self.cur().type == "ID" else NodeType.Literal,
            self.consume(),
        )

    def parse(self):
        self.tree = TreeNode(NodeType.CompilationUnit)
        self.tree.children.append(self.block())

        if self.cur().type != "EOF":
            self.error(f"Expected EOF but got {self.cur().value}")
