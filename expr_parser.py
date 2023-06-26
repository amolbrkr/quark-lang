from helper_types import *


class ExprParser:
    def __init__(self, parser):
        self.parser = parser
        self.rules = [
            Rule("PLUS", Precedence.Term, infix=self.binary),
            Rule("MINUS", Precedence.Term, prefix=self.unary, infix=self.binary),
            Rule("MULTIPLY", Precedence.Factor, infix=self.binary),
            Rule("DIVIDE", Precedence.Factor, infix=self.binary),
            Rule("EQUALS", Precedence.Assignment, infix=self.binary),
            Rule("NE", Precedence.Zero, prefix=self.unary),
            Rule("INT", Precedence.Zero, prefix=self.number),
            Rule("FLOAT", Precedence.Zero, prefix=self.number),
            Rule("ID", Precedence.Zero, prefix=self.identifier),
            Rule("LPAR", Precedence.Zero, prefix=self.paren),
        ]

    def rule(self, tok_type):
        print(f"Token type: {tok_type}")
        return next(filter(lambda x: x.type == tok_type, self.rules))

    def paren(self):
        expr = self.parse()
        self.parser.expect("RPAR")
        return expr

    def identifier(self):
        return TreeNode(NodeType.Identifier, self.parser.prev)

    def number(self):
        return TreeNode(NodeType.Literal, self.parser.prev)

    def unary(self):
        node = TreeNode(NodeType.Operator, self.parser.prev)
        node.children.append(self.parse(precedence=Precedence.Unary))
        return node

    def binary(self, left):
        node = TreeNode(NodeType.Operator, self.parser.prev)
        rule = self.rule(node.tok.type)
        node.children.extend([left, self.parse(precedence=rule.precedence + 1)])
        return node

    def parse(self, precedence=Precedence.Assignment):
        prefix = self.rule(self.parser.consume().type).prefix

        if not prefix:
            raise Exception("Expected expression.")

        expr = prefix()

        while (
            self.parser.cur.type not in ["RPAR", "NEWLINE", "COMMA", "COLON"]
            and self.rule(self.parser.cur.type).precedence >= precedence
        ):
            infix = self.rule(self.parser.consume().type).infix
            expr = infix(expr)

        return expr
