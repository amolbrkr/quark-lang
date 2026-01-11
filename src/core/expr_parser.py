from .helper_types import *


class ExprParser:
    def __init__(self, parser):
        self.parser = parser
        self.rules = [
            # Assignment (lowest precedence)
            Rule("EQUALS", Precedence.Assignment, infix=self.binary),

            # Pipe
            Rule("PIPE", Precedence.Pipe, infix=self.pipe),

            # Comma
            Rule("COMMA", Precedence.Comma, infix=self.binary),

            # Ternary (if-else) - handled specially
            # LogicalOr
            Rule("OR", Precedence.LogicalOr, infix=self.binary),

            # LogicalAnd
            Rule("AND", Precedence.LogicalAnd, infix=self.binary),

            # BitwiseAnd
            Rule("AMPER", Precedence.BitwiseAnd, infix=self.binary),

            # Equality
            Rule("DEQ", Precedence.Equality, infix=self.binary),
            Rule("NE", Precedence.Equality, infix=self.binary),

            # Comparison
            Rule("LT", Precedence.Comparison, infix=self.binary),
            Rule("LTE", Precedence.Comparison, infix=self.binary),
            Rule("GT", Precedence.Comparison, infix=self.binary),
            Rule("GTE", Precedence.Comparison, infix=self.binary),

            # Range
            Rule("DOTDOT", Precedence.Range, infix=self.binary),

            # Term (+ -)
            Rule("PLUS", Precedence.Term, infix=self.binary),
            Rule("MINUS", Precedence.Term, prefix=self.unary, infix=self.binary),

            # Factor (* / %)
            Rule("MULTIPLY", Precedence.Factor, infix=self.binary),
            Rule("DIVIDE", Precedence.Factor, infix=self.binary),
            Rule("MODULO", Precedence.Factor, infix=self.binary),

            # Exponent (**)
            Rule("DOUBLESTAR", Precedence.Exponent, infix=self.exponent),

            # Unary (! ~ -)
            Rule("BANG", Precedence.Unary, prefix=self.unary),
            Rule("NOT", Precedence.Unary, prefix=self.unary),

            # Primary
            Rule("INT", Precedence.Access, prefix=self.number),
            Rule("FLOAT", Precedence.Access, prefix=self.number),
            Rule("STR", Precedence.Access, prefix=self.string),
            Rule("ID", Precedence.Access, prefix=self.identifier),
            Rule("LPAR", Precedence.Access, prefix=self.paren),
            Rule("LBRACE", Precedence.Access, prefix=self.list_literal),
            Rule("BLOCKSTART", Precedence.Access, prefix=self.dict_literal),
            Rule("UNDERSCORE", Precedence.Access, prefix=self.wildcard),

            # Member access
            Rule("DOT", Precedence.Access, infix=self.member_access),
        ]

        # Default rule for unknown tokens
        self._default_rule = Rule("UNKNOWN", Precedence.Assignment, None, None)

    def rule(self, tok_type):
        try:
            return next(filter(lambda x: x.type == tok_type, self.rules))
        except StopIteration:
            return self._default_rule

    def paren(self):
        expr = self.parse()
        self.parser.expect("RPAR")
        return expr

    def list_literal(self):
        node = TreeNode(NodeType.Literal, self.parser.prev)
        node.children = []
        if self.parser.cur.type != "RBRACE":
            while True:
                node.children.append(self.parse())
                if self.parser.cur.type == "COMMA":
                    self.parser.consume()
                else:
                    break
        self.parser.expect("RBRACE")
        return node

    def dict_literal(self):
        node = TreeNode(NodeType.Literal, self.parser.prev)
        node.children = []
        if self.parser.cur.type != "BLOCKEND":
            while True:
                key = self.identifier()
                self.parser.expect("COLON")
                value = self.parse()
                pair = TreeNode(NodeType.Operator)
                pair.children = [key, value]
                node.children.append(pair)
                if self.parser.cur.type == "COMMA":
                    self.parser.consume()
                else:
                    break
        self.parser.expect("BLOCKEND")
        return node

    def wildcard(self):
        return TreeNode(NodeType.Identifier, self.parser.prev)

    def identifier(self):
        return TreeNode(NodeType.Identifier, self.parser.prev)

    def number(self):
        return TreeNode(NodeType.Literal, self.parser.prev)

    def string(self):
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

    def exponent(self, left):
        # Right-associative: use same precedence instead of +1
        node = TreeNode(NodeType.Operator, self.parser.prev)
        node.children.extend([left, self.parse(precedence=Precedence.Exponent)])
        return node

    def pipe(self, left):
        node = TreeNode(NodeType.Pipe, self.parser.prev)
        # Right side can be a function name or function call
        right = self.parse(precedence=Precedence.Pipe + 1)
        node.children.extend([left, right])
        return node

    def member_access(self, left):
        node = TreeNode(NodeType.Operator, self.parser.prev)
        # Consume the identifier after the dot
        node.children.extend([left, TreeNode(NodeType.Identifier, self.parser.expect("ID"))])
        return node

    def function_application(self, func):
        """Parse function application: func arg1 arg2 ...

        The key insight: arguments should be parsed at a precedence level that allows
        arithmetic operators to bind the arguments together.

        For `fact n - 1` to parse as `fact(n-1)`, we need to parse at a precedence
        BELOW Term (10) so that `-` binds the argument.

        But we're at Application level (14), which is TIGHTER than Term (10).
        So when parsing arguments, we should use a lower precedence to allow operators
        to grab the arguments.
        """
        node = TreeNode(NodeType.FunctionCall)
        node.children.append(func)

        args = TreeNode(NodeType.Arguments)

        # Parse first argument - needs to consume operators too
        # Parse at Term level (10) to allow arithmetic within arguments
        # This way `fact n - 1` becomes `fact(n - 1)` because Term precedence
        # allows the subtraction to bind
        arg = self.parse(precedence=Precedence.Term)
        args.children.append(arg)

        # Handle additional comma-separated arguments
        while self.parser.cur.type == "COMMA":
            self.parser.consume()
            if self.can_start_expression(self.parser.cur.type):
                arg = self.parse(precedence=Precedence.Term)
                args.children.append(arg)

        node.children.append(args)
        return node

    def can_start_expression(self, tok_type):
        """Check if a token can start an expression (for function application)"""
        return tok_type in ["ID", "INT", "FLOAT", "STR", "LPAR", "LBRACE", "BLOCKSTART", "UNDERSCORE", "BANG", "NOT", "MINUS"]

    def parse(self, precedence=Precedence.Assignment):
        # Handle ternary (if-else) specially
        if self.parser.cur.type == "IF":
            return self.ternary()

        prefix = self.rule(self.parser.consume().type).prefix

        if not prefix:
            raise Exception(f"Expected expression, got {self.parser.prev.type}")

        expr = prefix()

        while (
            self.parser.cur.type not in ["RPAR", "NEWLINE", "RBRACE", "BLOCKEND", "EOF"]
            and self.parser.cur.type not in ["COMMA", "COLON"] or self.rule(self.parser.cur.type).precedence > Precedence.Comma
        ):
            # Check for ternary
            if self.parser.cur.type == "IF" and precedence <= Precedence.Ternary:
                expr = self.ternary_infix(expr)
                continue

            # Check for function application (space operator)
            # Function application happens when:
            # 1. Current precedence allows it (we're not already at too high precedence)
            # 2. Next token can start an expression
            # 3. Next token is not an infix operator at our current precedence level or higher
            if self.can_start_expression(self.parser.cur.type):
                rule = self.rule(self.parser.cur.type)
                # Application precedence is 14
                # Only apply function if:
                # - We're parsing at precedence that allows Application (precedence <= 14)
                # - The next token is NOT an infix operator at current or higher precedence
                # This ensures `n - 1` doesn't become `n(-1)` when precedence >= Term
                if (precedence <= Precedence.Application and
                    (not rule.infix or rule.precedence < precedence)):
                    expr = self.function_application(expr)
                    continue

            rule = self.rule(self.parser.cur.type)
            if rule.precedence < precedence:
                break

            if not rule.infix:
                break

            self.parser.consume()
            expr = rule.infix(expr)

        return expr

    def ternary(self):
        # Parse: value if condition else other
        # This shouldn't be called in current flow, but keep for completeness
        raise Exception("Ternary should be parsed as infix")

    def ternary_infix(self, value_if_true):
        # We have: <expr> IF
        self.parser.expect("IF")
        condition = self.parse(precedence=Precedence.Ternary + 1)
        self.parser.expect("ELSE")
        value_if_false = self.parse(precedence=Precedence.Ternary)

        node = TreeNode(NodeType.Ternary)
        node.children = [condition, value_if_true, value_if_false]
        return node
