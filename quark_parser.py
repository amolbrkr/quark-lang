from ply.lex import Token
from dataclasses import dataclass


@dataclass
class TreeNode:
    t: Token 
    left: any = None
    right: any = None