from gvgen import *
from quark_parser import TreeNode


class TreeViz:
    def __init__(self):
        self.graph = GvGen()
        self.graph.styleDefaultAppend("shape", "rectangle")

    def _new(self, tree):
        val = (
            tree.tok.value.replace('"', "").replace(",", "")
            if type(tree.tok.value) == str
            else tree.tok.value
        )
        return self.graph.newItem(f"{tree} ({val})")

    def _link(self, node1, node2):
        self.graph.newLink(node1, node2)

    def generate(self, tree, parent=None):
        if tree:
            node = self._new(tree) if not parent else parent
            for child in tree.children:
                if child:
                    node1 = self._new(child)
                    self._link(node, node1)
                    self.generate(child, node1)

    def save(self):
        outf = open("treeviz.dot", "w+")
        self.graph.dot(outf)
