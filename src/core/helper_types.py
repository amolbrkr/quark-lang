from enum import Enum
from typing import Any
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

    def __str__(self):
        return self._name_


@dataclass(frozen=True)
class Precedence:
    Zero = 0
    Assignment = 1
    Term = 2
    Factor = 3
    Unary = 4


@dataclass
class TreeNode:
    type: NodeType
    tok: Token = None
    children: list = field(default_factory=list)

    def __str__(self):
        return f"{self.type}" + (f"[{self.tok.value}]" if self.tok else "")

    def print(self, level=0):
        print("\t" * level + str(self))
        for child in self.children:
            child.print(level + 1)


@dataclass(frozen=True)
class Rule:
    type: str
    precedence: Precedence
    prefix: any = None
    infix: any = None
