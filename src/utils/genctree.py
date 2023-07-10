from core.helper_types import *


def gen_c_node(node):
    cnode = cnode_factory(len(node.children))
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
        # cnode_arr = (cnode_factory len(tree.children))(*tmp)
        cnode.children = tmp

    return cnode
