"""
Full Compilation Driver for Quark

Runs the complete compilation pipeline:
1. Lexer
2. Parser
3. Semantic Analyzer
4. Python Code Generator

Usage: python -m drivers.run_full_compile <input.qrk>
"""

import sys
import os
from pathlib import Path
import ply.lex as lex

# Add src to path
sys.path.insert(0, os.path.join(os.path.dirname(__file__), '..'))

from core.lex_grammar import *
from core.quark_lexer import QuarkLexer
from core.quark_parser import QuarkParser
from core.semantic_analyzer import SemanticAnalyzer
from core.quark_codegen import QuarkCodeGen


def compile_quark_file(input_path: str, output_path: str = None, verbose: bool = True):
    """
    Compile a Quark source file to Python.

    Args:
        input_path: Path to .qrk source file
        output_path: Path to output .py file (optional)
        verbose: Print compilation stages

    Returns:
        True if compilation succeeded, False otherwise
    """
    # Determine output path
    if output_path is None:
        input_file = Path(input_path)
        output_path = input_file.with_suffix('.py')

    # Read source
    try:
        with open(input_path, 'r') as f:
            source = f.read()
    except FileNotFoundError:
        print(f"Error: File not found: {input_path}")
        return False

    if verbose:
        print(f"Compiling: {input_path}")
        print(f"Output: {output_path}")
        print("=" * 60)

    # Stage 1: Lexing
    if verbose:
        print("\n[1/4] Lexing...")

    ply_lexer = lex.lex()
    lexer = QuarkLexer(ply_lexer)
    lexer.input(source)
    tokens = lexer.token_stream

    if verbose:
        print(f"  OK Tokenization complete")

    # Stage 2: Parsing
    if verbose:
        print("\n[2/4] Parsing...")

    parser = QuarkParser(tokens)
    try:
        ast = parser.parse()
        if verbose:
            print(f"  OK AST generated")
            print(f"\nAST Structure:")
            ast.print(1)
    except Exception as e:
        print(f"  ERROR Parse error: {e}")
        return False

    # Stage 3: Semantic Analysis
    if verbose:
        print("\n[3/4] Semantic Analysis...")

    analyzer = SemanticAnalyzer()
    success = analyzer.analyze(ast)

    if not success:
        if verbose:
            print(f"  ERROR Semantic analysis failed")
        analyzer.print_errors()
        return False

    if verbose:
        print(f"  OK Semantic analysis passed")
        print(f"\nSymbol Table:")
        analyzer.print_symbol_table()

    # Stage 4: Code Generation
    if verbose:
        print("\n[4/4] Generating Python code...")

    codegen = QuarkCodeGen()
    try:
        python_code = codegen.generate(ast)

        if verbose:
            print(f"  OK Code generation complete")
            print(f"\nGenerated Code:")
            print("-" * 60)
            # Number lines for readability
            for i, line in enumerate(python_code.split('\n'), 1):
                print(f"{i:3d} | {line}")
            print("-" * 60)

    except Exception as e:
        print(f"  ERROR Code generation error: {e}")
        import traceback
        traceback.print_exc()
        return False

    # Write output
    try:
        with open(output_path, 'w') as f:
            f.write(python_code)

        if verbose:
            print(f"\nOK Compilation successful!")
            print(f"  Output written to: {output_path}")
            print(f"\nTo run: python {output_path}")

        return True

    except Exception as e:
        print(f"  ERROR Error writing output: {e}")
        return False


def main():
    """Main entry point"""
    if len(sys.argv) < 2:
        print("Usage: python -m drivers.run_full_compile <input.qrk> [output.py]")
        print("\nExample:")
        print("  cd src")
        print("  python -m drivers.run_full_compile ../test.qrk ../test.py")
        sys.exit(1)

    input_file = sys.argv[1]
    output_file = sys.argv[2] if len(sys.argv) > 2 else None

    success = compile_quark_file(input_file, output_file, verbose=True)

    sys.exit(0 if success else 1)


if __name__ == '__main__':
    main()
