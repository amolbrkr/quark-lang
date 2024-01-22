import os
import argparse
import ply.lex as lex
from core.lex_grammar import *
from core.helper_types import *
from core.quark_lexer import QuarkLexer
from core.quark_parser import QuarkParser
from core.quark_codegen import QuarkCG
from core.quark_assembler import QuarkAssembler


if __name__ == "__main__":
    argp = argparse.ArgumentParser()

    argp.add_argument(
        "-i",
        "--input",
        type=str,
        required=True,
        help="Input object file (e.g., myfile.obj)",
    )
    argp.add_argument(
        "-o",
        "--output",
        type=str,
        required=False,
        help="Output executable file (e.g., myfile.exe)",
    )

    args = argp.parse_args()

    with open(args.input, "r") as inputf:
        filenm = args.input[args.input.rfind("/") + 1 : args.input.rfind(".")]

        lexer = QuarkLexer(lex.lex())
        lexer.input(inputf.read())

        parser = QuarkParser(lexer.token_stream)
        parser.parse()

        if not parser.tree:
            raise Exception("Parser failed.")

        cg = QuarkCG(parser.tree)
        asm = cg.generate()
        print(asm)

        # Write the asm output to a file, then call assembler
        if not os.path.exists(os.path.join(os.getcwd(), r'build')):
           os.makedirs("build")

        #with open(f"build/{filenm}.asm", "w") as outf:
        #    outf.writelines(asm)

        #        qasm = QuarkAssembler()

        #qasm.assemble(f"build/{filenm}.asm")
        #qasm.link(out_path=)
