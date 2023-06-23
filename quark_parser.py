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
    FunctionCall = 6
    Arguments = 7
    Identifier = 8
    Literal = 9
    Operator = 10


@dataclass
class TreeNode:
    type: NodeType
    tok: Token = None
    children: list = field(default_factory=list)

    def __str__(self):
        return f"{self.type}" + f"[{self.tok.value}]" if self.tok else ""
    
    def print(self, level=0):
        print('\t' * level + str(self))
        for child in self.children:
            child.print(level + 1)


class QuarkParser:
    def __init__(self, token_stream):
        self.tree = None
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

    def expect(self, type):
        if self.cur().type == type:
            return self.consume()
        else:
            raise Exception(f"Expected {type} but got {self.cur().type}.")

    ## Parsing functions
    def block(self): # Needs re-work
        print(f"Block: {self.cur()}")
        node = TreeNode(NodeType.Block)

        # Both newline and indent are present, which means this is an indendted block,
        # which means we only need to process statements until the dedent
        if self.cur().type == "NEWLINE" and self.peek().type == "INDENT":
            pass
        # No indent present so process statements EOF
        else:
            while self.cur().type != "NEWLINE":
                node.children.append(self.statement())
            self.expect("NEWLINE")

        return node

    def statement(self):
        print(f"Statement: {self.cur()}")
        node = None

        if self.cur().type == "IF":
            node = self.ifelse()
        elif "FN" in [self.cur().type, self.peek(2).type]:
            node = self.function()
        elif self.cur().type == "AT":
            self.consume()
            node = self.function_call()
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
        print(f"Function: {self.cur()}")
        node = None

        if self.cur().type == "FN":
            node = TreeNode(NodeType.Function, self.consume())
            node.children.extend(
                [TreeNode(NodeType.Identifier, self.expect("ID")), self.arguments()]
            )
            self.expect("COLON")
            node.children.append(self.block())
        elif self.peek(2).type == "FN":
            id = TreeNode(NodeType.Identifier, self.expect("ID"))
            self.expect("EQUALS")
            node = TreeNode(NodeType.Function, self.consume())
            node.children.extend([id, self.arguments()])
            self.expect("COLON")
            node.children.append(self.block())

        return node

    def function_call(self):
        print(f"Function Call: {self.cur()}")
        node = TreeNode(NodeType.FunctionCall)
        node.children.extend(
            [TreeNode(NodeType.Identifier, self.expect("ID")), self.arguments()]
        )
        return node

    def arguments(self):
        print(f"Arguments: {self.cur()}")
        node = TreeNode(NodeType.Arguments)

        while self.cur().type not in ["COLON", "NEWLINE"]:
            node.children.append(self.expression())

            if self.cur().type == "COMMA":
                self.consume()
            
        print(node)
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

        self.tree.print()
