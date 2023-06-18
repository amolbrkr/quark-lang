from ply.lex import Token
from dataclasses import dataclass


(
    CompilationUnit,  # 0
    Statement,  # 1
    Expression,  # 2
    Condition,  # 3
    Function,  # 4
    Argument,  # 5
    Identifier,  # 6
    Literal,  # 7
    Assignment,  # 8
    Operator,  # 9
) = range(10)


@dataclass
class TreeNode:
    type: int
    t: Token
    left: any = None
    mid: any = None
    right: any = None

    def __str__(self):
        tmp = (
            "CompilationUnit",  # 0
            "Statement",  # 1
            "Expression",  # 2
            "Condition",  # 3
            "Function",  # 4
            "Argument",  # 5
            "Identifier",  # 6
            "Literal",  # 7
            "Assignment",  # 8
            "Operator",  # 9
        )[self.type]
        return f"{tmp}[{self.t.type}]"


class QuarkParser:
    def __init__(self, lexer):
        self.tok = None
        self.tree = None
        self.lexer = lexer

    def _is_term(self):
        return self.tok.type in ["ID", "INT", "FLOAT", "STR"]

    def next_token(self):
        self.tok = self.lexer.token()

    def error(self, msg):
        raise Exception(f"ParseError: {msg}")

    def parse_stat(self):
        print(f"parse_statement: {self.tok}")
        n = None

        if self._is_term():
            id = self.parse_term()
            self.next_token()
            if self.tok.type == "EQUALS":
                n = TreeNode(Assignment, self.tok, left=id)
                self.next_token()
                n.mid = self.parse_expr()
        elif self.tok.type == "IF":
            n = TreeNode(Statement, self.tok)
            self.next_token()
            n.left = self.parse_expr()
            self.next_token()
            if self.tok.type == "COLON":
                self.next_token()
                while self.tok.type in ["NEWLINE", "INDENT"]:
                    self.next_token()
                n.mid = self.parse_stat()
                self.next_token()
                while self.tok.type in ["NEWLINE", "DEDENT"]:
                    self.next_token()
                print("before else", self.tok)
                if self.tok.type == "ELSE":
                    print("else start")
                    self.next_token()
                    self.next_token()
                    while self.tok.type in ["NEWLINE", "INDENT"]:
                        self.next_token()
                    n.right = self.parse_stat()
        elif self.tok.type == "FN":
            n = TreeNode(Function, self.tok, left=self.parse_func())
        else:
            n = TreeNode(Expression, self.tok, left=self.parse_expr())

        print(f"Node: {n}")
        return n

    def parse_term(self):
        if not self._is_term():
            self.error(f"Expected Identifier or Literal got '{self.tok.value}'")

        return TreeNode(Identifier if self.tok.type == "ID" else Literal, self.tok)

    def parse_expr(self):
        print(f"parse_expr: {self.tok}")
        n = None

        if self._is_term():
            lterm = self.parse_term()
            self.next_token()
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
            ]:
                n = TreeNode(Operator, self.tok, left=lterm)
                self.next_token()
                n.mid = self.parse_term()
            else:
                n = lterm
        elif self.tok.type == "LPAR":
            self.next_token()
            self.parse_expr()
        else:
            self.error(f"Expected Term or '(' but got '{self.tok.value}'")

        return n

    def parse_asgn(self):
        print(f"parse_asgn: {self.tok}")

    def parse_func(self):
        print(f"parse_func: {self.tok}")

    def parse(self):
        self.next_token()
        self.tree = TreeNode(CompilationUnit, self.tok, left=self.parse_stat())

        self.next_token()
        # if self.tok.type != "EOF":
        #     self.error(f"Expected EOF but got {self.tok.value}")
