# Quark Compiler - Rust Implementation

A Rust implementation of the Quark language compiler with custom lexer, Pratt parser, and GraphViz visualization.

## Features

- **Custom Lexer**: Hand-written lexer with Python-style indentation handling (INDENT/DEDENT tokens)
- **Pratt Parser**: Precedence climbing parser with 15 precedence levels
- **AST Visualization**: GraphViz integration for visualizing abstract syntax trees
- **CLI Utility**: Unified command-line interface for all compiler stages

## Building

```bash
cd src/core_new
cargo build --release
```

## Usage

The Quark compiler provides several subcommands for different stages of compilation:

### 1. Lexing (Tokenization)

Display tokens from a source file:

```bash
cargo run -- lex ../../test.qrk
```

With verbose output:

```bash
cargo run -- lex ../../test.qrk --verbose
```

### 2. Parsing

Parse a source file and generate AST:

```bash
cargo run -- parse ../../test.qrk
```

Display the tree structure:

```bash
cargo run -- parse ../../test.qrk --tree
```

### 3. Visualization

Generate GraphViz visualization (DOT and PNG):

```bash
cargo run -- visualize ../../test.qrk
```

Custom output files:

```bash
cargo run -- visualize ../../test.qrk -d output.dot -p output.png
```

Generate only DOT file (skip PNG):

```bash
cargo run -- visualize ../../test.qrk --no-png
```

### 4. Complete Pipeline

Run the complete compilation pipeline (lex → parse → visualize):

```bash
cargo run -- run ../../test.qrk
```

This is equivalent to:
```bash
cd /path/to/quark-lang/src
python -m drivers.run_parser ../test.qrk && dot -Tpng treeviz.dot -o treeviz.png
```

## Architecture

### Lexer (`lexer.rs`)

- Custom hand-written lexer (no lex/yacc dependencies)
- Handles 44+ token types
- Python-style indentation with INDENT/DEDENT tokens
- Two-stage filtering: indentation tracking → token emission
- Comment support (`//`)
- Single-quoted string literals with escape sequences

### Parser (`parser.rs`)

- Pratt parser implementation (precedence climbing algorithm)
- 15 precedence levels matching Python implementation
- Supports:
  - Functions with pattern matching (`fn`, `when`)
  - Control flow (`if`, `elseif`, `else`, `for`, `while`)
  - All operators (arithmetic, comparison, logical, bitwise)
  - Function application (space operator)
  - Pipe chains (`|`)
  - Ternary expressions
  - Lists and dictionaries
  - Member access

### AST (`ast.rs`)

- Node types: CompilationUnit, Block, Statement, Expression, Function, FunctionCall, etc.
- Precedence constants matching Python implementation
- Tree display implementation

### Visualizer (`visualizer.rs`)

- Generates GraphViz DOT format
- Creates hierarchical tree diagrams
- Integrates with `dot` command for PNG generation

## Precedence Levels

```
 0: = (assignment)
 1: | (pipe)
 2: , (comma)
 3: if-else (ternary)
 4: or
 5: and
 6: & (bitwise AND)
 7: == != (equality)
 8: < <= > >= (comparison)
 9: .. (range)
10: + - (term)
11: * / % (factor)
12: ** (exponent, right-associative)
13: ! ~ - (unary)
14: function application (space)
15: . [] () (member access)
```

## Example

Given `test.qrk`:

```quark
fn fact n:
    when n:
        0 or 1: 1
        _: n * fact n - 1

fact 5
```

Run:

```bash
cargo run -- run ../../test.qrk
```

Output:
```
=== Quark Compiler Pipeline ===

1. Lexing...
   ✓ Generated 27 tokens
2. Parsing...
   ✓ Generated AST
3. Visualizing...
   ✓ Generated DOT file: treeviz.dot
   ✓ Generated PNG file: treeviz.png

=== Compilation Complete ===
```

## Dependencies

- `clap` - Command-line argument parsing
- `anyhow` - Error handling
- `thiserror` - Error derive macros

External (optional):
- `graphviz` (`dot` command) - For PNG generation from DOT files

## Testing

Run tests:

```bash
cargo test
```

Run tests with output:

```bash
cargo test -- --nocapture
```

## Comparison with Python Implementation

### Similarities
- Same precedence levels and associativity rules
- Same AST node types
- Identical indentation handling logic
- Same language feature support

### Differences
- Custom lexer instead of PLY (lex/yacc)
- Direct Pratt parser implementation instead of rule-based system
- Unified CLI instead of separate driver scripts
- Strongly typed AST nodes
- Zero runtime dependencies (except for visualization)

## Future Work

- Array indexing and slicing support
- Lambda shorthand in pipes
- Type annotations
- Class definitions
- Code generation (LLVM backend)
- Improved error messages with spans
- REPL mode
