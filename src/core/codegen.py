from llvmlite import ir
from core.helper_types import *
import llvmlite.binding as llvm


class QuarkCodeGen:
    def __init__(self) -> None:
        self.fl = ir.FloatType()
        self.num = ir.IntType(32)

        self.sym_table = list()
        self.builder = ir.builder.IRBuilder()
        self.context = llvm.context.get_global_context()
        self.cur_module = None

    def generate(self, node, module_name="default"):
        print(node)
        if node.type == NodeType.CompilationUnit:
            self.cur_module = ir.Module(name=module_name)

        if node.type == NodeType.Block:
            self.block = self.builder.block

        if node.type == NodeType.Operator:
            if len(node.children) == 2:
                lhs = self.generate(node.children[0])
                rhs = self.generate(node.children[1])

                match node.tok.type:
                    case "EQUALS":
                        return ir.GlobalVariable(self.cur_module, rhs.type, lhs)
                    case "PLUS":
                        return self.builder.add(lhs, rhs)
                    case "MINUS":
                        return self.builder.sub(lhs, rhs)
                    case "MULTIPLY":
                        return self.builder.mul(lhs, rhs)
                    case "DIVIDE":
                        return self.builder.sdiv(lhs, rhs)
                    case _: return None

        if node.type == NodeType.Identifier:
            val = self.sym_table[node.tok.value]
            if not val:
                self.sym_table[node.tok.value] = ir.Undefined

            return val

        if node.type == NodeType.Literal:
            return ir.Constant(
                self.fl if node.tok.type == "FLOAT" else self.num, node.tok.value
            )

        for child in node.children:
            self.generate(child)
