from core.helper_types import *


def gen_c_node(node):
    print("\nGenerateing node for: ")
    node.print()

    cnode = CTreeNode()
    tok = CToken()

    if node.tok:
        tok.type = node.tok.type.encode("utf-8")
        tok.value = node.tok.value.encode("utf-8")
        tok.lineno = node.tok.lineno
        tok.pos = node.tok.pos

    cnode.type = node.type.value
    cnode.tok = tok

    return cnode


def gen_c_tree(tree):
    cnode = gen_c_node(tree)

    if len(tree.children) > 0:
        tmp = [gen_c_node(child) for child in tree.children]
        print(tmp)
        cnode.children = POINTER(CTreeNode * len(tree.children))(*tmp)
    # else:
    #     cnode.children = POINTER(CTreeNode())

    return cnode
