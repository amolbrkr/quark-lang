from core.expr_parser import ExprParser
from .helper_types import NodeType, TreeNode


class QuarkParser:
    def __init__(self, token_stream):
        self.tree = None
        self.tokens = list(token_stream)
        self.expr_parser = ExprParser(self)
        self.prev, self.cur = None, self.tokens[0] if self.tokens else None

    # Util functions
    def peek(self, index=1):
        return self.tokens[index] if index < len(self.tokens) else None

    def consume(self):
        if not self.tokens:
            raise Exception("Unexpected end of input")
        self.prev = self.tokens.pop(0)
        self.cur = self.tokens[0] if self.tokens else None
        return self.prev

    def is_term(self, token):
        return token and token.type in ["ID", "INT", "FLOAT", "STR"]

    def expect(self, type):
        if self.cur and self.cur.type == type:
            return self.consume()
        else:
            raise Exception(f"Expected {type} but got {self.cur.type if self.cur else 'EOF'}.")

    # Parsing functions
    def block(self):
        print(f"Block: {self.cur}")
        node = TreeNode(NodeType.Block)

        if self.cur and self.cur.type == "NEWLINE":
            next_tok = self.peek()
            if next_tok and next_tok.type == "INDENT":
                self.expect("NEWLINE")
                self.expect("INDENT")
                while self.cur and self.cur.type != "DEDENT":
                    if self.cur.type == "NEWLINE":
                        self.consume()
                        continue
                    node.children.append(self.statement())
                    if self.cur and self.cur.type == "NEWLINE":
                        self.consume()
                self.expect("DEDENT")
            else:
                # Single line block
                self.expect("NEWLINE")
        else:
            # Inline block (no newline)
            while self.cur and self.cur.type not in ["NEWLINE", "EOF"]:
                node.children.append(self.statement())
            if self.cur and self.cur.type == "NEWLINE":
                self.expect("NEWLINE")

        return node

    def statement(self):
        print(f"Statement: {self.cur}")
        if not self.cur:
            raise Exception("Unexpected end of input in statement")

        node = None

        if self.cur.type == "IF":
            node = self.ifelse()
        elif self.cur.type == "WHEN":
            node = self.when_statement()
        elif self.cur.type == "FOR":
            node = self.for_loop()
        elif self.cur.type == "WHILE":
            node = self.while_loop()
        elif self.cur.type == "FN":
            node = self.function()
        elif self.peek(2) and self.peek(2).type == "FN":
            node = self.function()
        elif self.cur.type == "AT":
            self.consume()
            node = self.function_call()
        else:
            node = self.expression()

        return node

    def expression(self):
        print(f"Expression: {self.cur}")
        result = self.expr_parser.parse()
        print(f"  Expression result: {result}, next token: {self.cur}")
        return result

    def function(self):
        print(f"Function: {self.cur}")
        node = None

        if self.cur.type == "FN":
            node = TreeNode(NodeType.Function, self.consume())
            node.children.extend(
                [TreeNode(NodeType.Identifier, self.expect("ID")), self.arguments()]
            )
            self.expect("COLON")
            node.children.append(self.block())
        elif self.peek(2) and self.peek(2).type == "FN":
            id = TreeNode(NodeType.Identifier, self.expect("ID"))
            self.expect("EQUALS")
            node = TreeNode(NodeType.Function, self.consume())
            node.children.extend([id, self.arguments()])
            self.expect("COLON")
            node.children.append(self.block())

        return node

    def function_call(self):
        print(f"Function Call: {self.cur}")
        node = TreeNode(NodeType.FunctionCall)
        node.children.extend(
            [TreeNode(NodeType.Identifier, self.expect("ID")), self.arguments()]
        )
        return node

    def arguments(self):
        print(f"Arguments: {self.cur}")
        node = TreeNode(NodeType.Arguments)

        while self.cur and self.cur.type not in ["COLON", "NEWLINE", "EOF"]:
            node.children.append(self.expression())

            if self.cur and self.cur.type == "COMMA":
                self.consume()
            else:
                break

        print(node)
        return node

    def ifelse(self):
        print(f"IfElse: {self.cur}")
        node = TreeNode(NodeType.IfStatement, self.consume())  # consume IF

        # Parse condition
        condition = self.expression()
        self.expect("COLON")
        if_block = self.block()

        node.children.extend([condition, if_block])

        # Parse elseif/else
        while self.cur and self.cur.type == "ELSEIF":
            self.consume()  # consume ELSEIF
            elseif_condition = self.expression()
            self.expect("COLON")
            elseif_block = self.block()

            # Create elseif node
            elseif_node = TreeNode(NodeType.IfStatement)
            elseif_node.children.extend([elseif_condition, elseif_block])
            node.children.append(elseif_node)

        if self.cur and self.cur.type == "ELSE":
            self.consume()  # consume ELSE
            self.expect("COLON")
            else_block = self.block()
            node.children.append(else_block)

        return node

    def when_statement(self):
        print(f"WhenStatement: {self.cur}")
        node = TreeNode(NodeType.WhenStatement, self.consume())  # consume WHEN

        # Parse expression to match against
        expr = self.expression()
        node.children.append(expr)

        self.expect("COLON")
        self.expect("NEWLINE")
        self.expect("INDENT")

        # Parse patterns
        while self.cur and self.cur.type != "DEDENT":
            if self.cur.type == "NEWLINE":
                self.consume()
                continue

            pattern_node = self.pattern()
            node.children.append(pattern_node)

            if self.cur and self.cur.type == "NEWLINE":
                self.consume()

        self.expect("DEDENT")
        return node

    def pattern(self):
        print(f"Pattern: {self.cur}")
        pattern_node = TreeNode(NodeType.Pattern)

        # Parse pattern expression(s) - can be multiple with 'or'
        # We need to parse patterns at a precedence ABOVE LogicalOr (Precedence.LogicalOr = 4)
        # so that 'or' is not consumed as an operator within pattern expressions
        patterns = []
        while True:
            if self.cur.type == "UNDERSCORE":
                # Wildcard pattern
                patterns.append(TreeNode(NodeType.Identifier, self.consume()))
            else:
                # Regular expression pattern - stop before 'or' operator
                # Use precedence level 5 (LogicalAnd) to prevent 'or' (level 4) from being consumed
                from .helper_types import Precedence
                print(f"  Parsing pattern with precedence={Precedence.LogicalAnd} (should be 5)")
                pattern_expr = self.expr_parser.parse(precedence=Precedence.LogicalAnd)
                print(f"  Pattern expression parsed: {pattern_expr}, next token: {self.cur}")
                patterns.append(pattern_expr)

            if self.cur and self.cur.type == "OR":
                self.consume()
            else:
                break

        self.expect("COLON")

        # Parse result expression (can include 'or' operator here)
        result = self.expression()

        pattern_node.children = patterns + [result]
        return pattern_node

    def for_loop(self):
        print(f"ForLoop: {self.cur}")
        node = TreeNode(NodeType.ForLoop, self.consume())  # consume FOR

        # Parse loop variable
        loop_var = TreeNode(NodeType.Identifier, self.expect("ID"))

        # Check for 'in' keyword
        if self.cur and self.cur.type == "IN":
            self.consume()  # consume IN
            # Parse iterable expression (could have ..)
            iterable = self.expression()
            node.children.extend([loop_var, iterable])
        else:
            raise Exception("Expected 'in' after loop variable")

        self.expect("COLON")
        body = self.block()
        node.children.append(body)

        return node

    def while_loop(self):
        print(f"WhileLoop: {self.cur}")
        node = TreeNode(NodeType.WhileLoop, self.consume())  # consume WHILE

        # Parse condition
        condition = self.expression()
        self.expect("COLON")
        body = self.block()

        node.children.extend([condition, body])
        return node

    def term(self):
        return TreeNode(
            NodeType.Identifier if self.cur.type == "ID" else NodeType.Literal,
            self.consume(),
        )

    def parse(self):
        self.tree = TreeNode(NodeType.CompilationUnit)

        while self.cur and self.cur.type != "EOF":
            if self.cur.type == "NEWLINE":
                self.consume()
                continue
            self.tree.children.append(self.statement())

        return self.tree
