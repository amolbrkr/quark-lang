from peachpy import *
from peachpy.x86_64 import *
from .helper_types import *


data_sec = """
section .data

"""

text_sec = """
global _start

section .text
_start: 

"""


class QuarkCG:
    def __init__(self, root):
        self.tree = root

    def compile(self):
        self.tree.print()
