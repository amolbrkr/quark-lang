from helper_types import *


class ExprParser:
    def __init__(self, parser):
        self.parser = parser
        self.rules = [
            {"tok": "PLUS", "rule": Rule(Precedence.Term, infix=self.binary)},
            {"tok": "NE", "rule": Rule(Precedence.Zero, prefix=self.unary)},
            {"tok": "INT", "rule": Rule(Precedence.Zero, prefix=self.number)},
            {"tok": "MULTIPLY", "rule": Rule(Precedence.Factor, infix=self.binary)},
            {"tok": "DIVIDE", "rule": Rule(Precedence.Factor, infix=self.binary)},
            {"tok": "ID", "rule": Rule(Precedence.Zero, prefix=self.identifier)},
            {
                "tok": "MINUS",
                "rule": Rule(Precedence.Term, prefix=self.unary, infix=self.binary),
            },
        ]

    def rule(self, tok_type):
        print(f"Token type: {tok_type}")
        return next(filter(lambda x: x["tok"] == tok_type, self.rules))["rule"]

    def identifier(self):
        return TreeNode(NodeType.Identifier, self.parser.prev)

    def number(self):
        return TreeNode(NodeType.Literal, self.parser.prev)

    def unary(self):
        node = TreeNode(NodeType.Operator, self.parser.prev)
        node.children.append(self.parse(Precedence.Unary))
        return node

    def binary(self, left):
        node = TreeNode(NodeType.Operator, self.parser.prev)
        rule = self.rule(self.parser.cur.type)
        node.children.extend([left, self.parse(rule.precedence + 1)])
        return node

    def parse(self, precedence=Precedence.Term):
        tok = self.parser.consume()
        prefix = self.rule(tok.type).prefix

        if not prefix:
            raise Exception("Expected expression.")

        expr = prefix()

        while (
            self.parser.cur.type not in ["NEWLINE", "COMMA", "COLON"]
            and self.rule(self.parser.cur.type).precedence >= precedence
        ):
            tok = self.parser.consume()
            infix = self.rule(tok.type).infix
            expr = infix(expr)

        return expr
