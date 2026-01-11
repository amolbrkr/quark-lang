# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

**Quark** is a human-friendly, functional, type-inferred language inspired by Python. The project is in early stages of development with a working lexer, parser, and initial code generation framework. The language emphasizes a **minimal punctuation philosophy** - using as few parentheses, brackets, and braces as possible to create English-like readable code.

## Core Language Philosophy

**Minimal Punctuation:** Quark reduces punctuation wherever possible:
- Function calls: `print msg` not `print(msg)`
- Multiple arguments: `add 2, 5` not `add(2, 5)`
- Chaining: `x | f | g` (pipe operator)
- Operator precedence allows: `fact n - 1` which parses as `fact(n-1)` because arithmetic binds tighter than function application

Parentheses are ONLY required for:
1. Overriding precedence: `2 * (3 + 4)`
2. Nested function calls: `outer (inner x, y)`
3. Complex expressions as arguments: `func (x + y), z`

## Development Commands

### Project Structure Note

**IMPORTANT**: The `src/` folder is NOT a Python module. Only subfolders within `src/` (like `core/`, `drivers/`, `utils/`) are modules.

**Always run commands from the `src/` directory** using module syntax like:
```bash
cd src
python -m drivers.run_parser ../test.qrk
```

Never use `python -m src.drivers.run_parser` - this will fail with import errors.

### Testing Parser Stages

All commands should be run from the `src/` directory:

```bash
# Test lexer only (tokenization + indentation handling)
python -m drivers.run_lexer ../<input.qrk>

# Test parser - generates AST tree and visualization
python -m drivers.run_parser ../<input.qrk>

# Test parser AND generate PNG visualization (RECOMMENDED)
# This command runs the parser and generates treeviz.png from the AST
python -m drivers.run_parser ../test.qrk && dot -Tpng treeviz.dot -o treeviz.png

# Full pipeline: lex → parse → codegen (produces x86-64 assembly)
python -m drivers.run_codegen ../<input.qrk>

# Assemble and link (requires NASM and GCC installed)
python -m drivers.run_assembler ../<input.asm>
```

### Running the Parser (Default Usage)

**When the user asks to "run the parser", use this command:**
```bash
cd /d/Source/quark-lang/src
python -m drivers.run_parser ../test.qrk && dot -Tpng treeviz.dot -o treeviz.png
```

This will:
1. Parse the test file and generate the AST
2. Create `treeviz.dot` (GraphViz format)
3. Convert to `treeviz.png` for visualization
4. Output will be in the `src/` directory

### Example Test File

The `test.qrk` file contains a simple factorial function with pattern matching:
```quark
fn fact n:
    when n:
        0 or 1: 1
        _: n - 1
```

### Dependencies

**Python packages:**
```bash
pip install -r requirements.txt
# Key dependencies: gvgen, llvmlite==0.41.1, peachpy
```

**External tools (for assembly/linking):**
- NASM (assembler)
- GCC (linker)

## Architecture Overview

### Compiler Pipeline Flow

```
Source Code (.qrk)
    ↓
QuarkLexer (tokenization + indentation tracking)
    ↓
Token Stream (with INDENT/DEDENT tokens)
    ↓
QuarkParser + ExprParser (AST generation)
    ↓
Abstract Syntax Tree (TreeNode hierarchy)
    ↓
QuarkCG (code generation to x86-64 assembly)
    ↓
NASM (assembly to object file)
    ↓
GCC (linking to executable)
```

### Core Components

**1. Lexer** (`src/core/quark_lexer.py` + `src/core/lex_grammar.py`)
- Tokenizes source into 44+ token types
- Handles Python-style indentation (INDENT/DEDENT tokens)
- Two-stage filtering: track_tokens → indentation_filter
- Reserved keywords: `use`, `module`, `in`, `and`, `or`, `if`, `elseif`, `else`, `for`, `while`, `when`, `fn`, `class`

**2. Expression Parser** (`src/core/expr_parser.py`)
- Precedence climbing algorithm with 15 precedence levels
- Rule-based system with prefix/infix handlers
- Handles all operators with correct associativity
- Special handling for right-associative `**` (exponentiation)

**Precedence Levels (0 = lowest, 15 = highest):**
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

**3. Statement Parser** (`src/core/quark_parser.py`)
- Parses top-level statements and control flow
- Delegates expression parsing to ExprParser
- Handles blocks (indented, single-line, inline)
- Constructs: functions, if-elseif-else, when (pattern matching), for/while loops

**4. AST Types** (`src/core/helper_types.py`)
- `NodeType` enum: CompilationUnit, Block, Statement, Expression, Function, FunctionCall, Arguments, IfStatement, WhenStatement, Pattern, ForLoop, WhileLoop, Lambda, Ternary, Pipe, Identifier, Literal, Operator
- `Precedence` dataclass: Frozen immutable precedence constants
- `TreeNode` dataclass: AST node with type, token, and children
- `Rule` dataclass: Parser rules with prefix/infix handlers

**5. Code Generator** (`src/core/quark_codegen.py`)
- **Status:** Early implementation (framework exists, needs completion)
- Uses PEachPy for x86-64 code generation
- Symbol table for variable tracking
- Sections: `.text`, `.data`, generated code
- Pattern matching for operators: EQUALS, PLUS, MINUS, etc.

**6. Assembler** (`src/core/quark_assembler.py`)
- Two-stage: NASM assembly → GCC linking
- Outputs Windows PE executables (win64 format)

### Driver Scripts

All in `src/drivers/`:
- `run_lexer.py` - Lexer-only testing
- `run_parser.py` - Parser testing with AST visualization (uses treeviz)
- `run_codegen.py` - Full pipeline to assembly
- `run_assembler.py` - Assembly and linking stage

### Utilities

- `src/utils/treeviz.py` - AST visualization using GraphViz (generates .dot files and .png images)
- `src/utils/genctree.py` - Tree generation utilities

## Language Features

### Implemented in Parser

✅ **Operators:** All arithmetic (`+`, `-`, `*`, `/`, `%`, `**`), comparison (`<`, `<=`, `>`, `>=`, `==`, `!=`), logical (`and`, `or`, `!`, `~`), bitwise (`&`), range (`..`), pipe (`|`)

✅ **Control Flow:**
- If-elseif-else statements
- When pattern matching with `or` patterns and `_` wildcard
- For loops with range: `for i in 0..10:`
- While loops: `while condition:`

✅ **Functions:**
- Named: `fn name params: body`
- Anonymous: `name = fn params: body`
- Calls with space operator: `func arg1 arg2` or with commas: `func arg1, arg2`
- Calls with @ prefix: `@func args`
- **Function application (space operator) NOW IMPLEMENTED!**

✅ **Expressions:**
- Ternary: `value if condition else other`
- Pipe chains: `x | f | g`
- Member access: `obj.method`
- Lists: `[1, 2, 3]`
- Dicts: `{key: value}`

✅ **Literals:**
- Integers: `42`, `0`, `1000`
- Floats: `3.14`, `.5`, `2.`
- Strings: `'hello world'` (single-quoted only)
- **Note:** Only single-quoted strings are currently supported. Double quotes are not yet implemented.

### Not Yet Implemented

#### Documented in Grammar/Examples but Missing from Parser:

**1. Array/List Indexing and Slicing** (grammar.md:118-119, examples.md:89,122-123)
   - ❌ `arr[0]` - single index access
   - ❌ `arr[0:5]` - slice notation
   - ❌ `arr[start..end]` - range-based slicing
   - **Status:** Token `LBRACE`/`RBRACE` exists, but no indexing handler in expr_parser.py
   - **Impact:** Examples like `myList[0].length` and `myList[0..5]` will fail

**2. Lambda Shorthand in Pipes** (grammar.md:156,302-306, examples.md:70-72)
   - ❌ `map x: x * 2` should auto-convert to `map (fn x: x * 2)`
   - ❌ `filter c: bool c` should auto-convert to `filter (fn c: bool c)`
   - **Status:** Grammar specifies shorthand after `map`, `filter` keywords, but no context-aware lambda parsing
   - **Impact:** All examples.md pipe chains with lambda shorthand won't parse

**3. Type Annotations** (grammar.md:196-202, examples.md:25-27)
   - ❌ `str.name = 'value'` - type prefix syntax
   - ❌ `num.pi = 3.14` - numeric type annotation
   - ❌ Type keywords: `int`, `float`, `str`, `bool`, `list`, `dict`
   - **Status:** No tokens or parser rules for type prefix syntax
   - **Impact:** Cannot specify variable types explicitly

**4. Function Application (Space Operator)** - ✅ **NOW IMPLEMENTED!** (grammar.md:114,209,276-283)
   - ✅ Space-based function calls: `func x y` without parentheses
   - ✅ Examples like `fact n - 1` now work correctly (parses as `fact(n-1)`)
   - **Status:** IMPLEMENTED in expr_parser.py with proper precedence handling
   - **How it works:**
     - Precedence 14 (Application) binds tighter than arithmetic operators
     - `fact n - 1` parses as `fact(n-1)` because `-` binds the argument
     - `n * fact n - 1` parses as `n * fact(n-1)` correctly
     - Works in pipe chains: `10 | double | add 5`
   - **Implementation details:**
     - Function application checks if next token can start an expression
     - Prioritizes infix operators at current precedence level
     - Arguments parsed at Term level (10) to allow arithmetic within args

**5. Class Definitions** (grammar.md:180-182)
   - ❌ `class Name:` or `class Child (Parent):`
   - **Status:** Keyword `class` reserved, but no `class_def()` method in quark_parser.py
   - **Impact:** No OOP support

**6. Boolean and Null Literals** (grammar.md:42-43)
   - ❌ `true` / `false` boolean literals
   - ❌ `null` literal
   - **Status:** No tokens defined in lex_grammar.py for these keywords
   - **Impact:** Must use 1/0 for booleans, no null representation

**7. Double-Quoted Strings**
   - ❌ `"double quoted strings"` - only single quotes supported
   - **Status:** Lexer changed to only support single-quoted strings (simpler implementation)
   - **Impact:** All strings must use single quotes `'like this'`

**8. Use/Module System** (grammar.md:16-17)
   - ❌ `use` keyword for imports
   - ❌ `module` keyword for module definitions
   - **Status:** Keywords reserved but no parser implementation
   - **Impact:** No module/import system

**9. Enhanced For Loop Syntax** (grammar.md:172-174)
   - ❌ `for i .. expression:` - direct range without `in`
   - **Status:** Grammar allows it, parser only implements `for i in expression:`
   - **Impact:** Alternative for loop syntax won't work

**10. Comments** (examples.md:4,10 - has `//` comments)
   - ✅ **IMPLEMENTED:** Token exists (`t_ignore_COMMENT = r"\//.*"`)
   - **Status:** Lexer ignores comments correctly
   - **Note:** Single-line `//` comments work

#### Code Generation Limitations:

**11. Complete Code Generation**
   - ❌ Full x86-64 code generation for all AST node types
   - **Status:** Framework exists in quark_codegen.py but incomplete
   - **Impact:** Cannot compile to executable yet

#### Summary of Missing Token Definitions:

Missing from lex_grammar.py:
- `true`, `false` - boolean literals
- `null` - null literal
- `"double quotes"` - only single quotes implemented
- No special handling for type annotations (type.identifier syntax)
- Indexing works with `LBRACE`/`RBRACE` (`[` `]`) but no parser support

## Key Implementation Details

### Indentation Handling

The lexer uses a two-stage filtering system:
1. **track_tokens()**: Marks tokens that require indentation tracking (after `:` in control flow)
2. **indentation_filter()**: Converts whitespace into INDENT/DEDENT tokens

This allows Python-style indentation-based blocks while supporting inline blocks too.

### Block Parsing

`quark_parser.py::block()` handles three block types:
- **Indented blocks:** `NEWLINE INDENT statements DEDENT`
- **Single-line blocks:** `NEWLINE statement` (no indent)
- **Inline blocks:** No newline, statement on same line

### Pattern Matching

When statements support multiple patterns with `or`:
```quark
when n:
    0 or 1: 1
    _: n * fact n - 1
```

The parser creates Pattern nodes with multiple pattern expressions followed by a result expression.

### Right-Associative Exponentiation

The expression parser handles `**` specially in `exponent()` method - uses same precedence instead of `precedence + 1` to achieve right-associativity: `2**3**2` = `2**(3**2)` = 512

## Working with the Codebase

### Modifying Grammar

When changing grammar:
1. Update `grammar.md` with BNF changes
2. Update `precedence.md` if precedence/associativity changes
3. Add examples to `examples.md`
4. Update token definitions in `src/core/lex_grammar.py`
5. Update parser methods in `src/core/expr_parser.py` or `src/core/quark_parser.py`
6. Add corresponding NodeType to `src/core/helper_types.py` if needed

### Adding New Operators

1. Add token to `lex_grammar.py` (order matters - longer tokens first!)
2. Add precedence level to `helper_types.py::Precedence`
3. Add Rule to `expr_parser.py::__init__()` with handler method
4. Implement handler method (prefix for unary, infix for binary)

### Debugging Parser Issues

Use `run_parser.py` to generate AST visualization:
```bash
python -m src.drivers.run_parser test.qrk
```

This creates:
- Console output with debug prints
- `src/treeviz.png` - Visual AST diagram
- Shows precedence and associativity are correct

### Important Parsing Principles

- **Arithmetic binds tighter than function application:** `fact n - 1` parses as `fact(n-1)` not `fact(n) - 1`
- **Comma is an operator:** Has precedence level 2, separates function arguments
- **Pipe has low precedence:** Level 1, allows `5 + 3 | double` to work as `(5+3) | double`
- **Ternary before comma:** `func (a if b else c), d` works correctly
- **Member access binds tightest:** `obj.method x` is `(obj.method)(x)`

## Current Development Status

**Recent Fixes (2026-01-10):**
- ✅ **Fixed string literal parsing** - Changed lexer to support single-quoted strings only
- ✅ **Implemented function application (space operator)** - Core feature for minimal punctuation philosophy
  - Added precedence 14 handling in expr_parser.py
  - `fact n - 1` now correctly parses as `fact(n-1)`
  - Works with pipes, comma-separated args, and complex expressions

**Git Status:**
- Modified: `src/core/lex_grammar.py` (removed QUOTES/DQUOTES tokens, single-quote strings only)
- Modified: `src/core/expr_parser.py` (added function_application() method)
- Modified: `grammar.md` and `examples.md` (documented single-quote restriction)
- Modified: `CLAUDE.md` (comprehensive feature gap analysis)

**Parser Status:** ~85% complete for documented grammar (up from 65%)
- Major remaining gaps: array indexing, lambda shorthand, type annotations, classes

**Next Steps:**
- Add array indexing/slicing support (high priority - examples depend on it)
- Implement lambda shorthand in pipes
- Add boolean literals (`true`, `false`) and `null`
- Complete code generation implementation
- Add formal test suite
- Implement type annotations
- Complete class definitions

## Documentation Files

- `grammar.md` - Complete BNF grammar specification
- `precedence.md` - Detailed precedence rules with examples
- `examples.md` - Language syntax examples
- `MINIMAL_PUNCTUATION_PHILOSOPHY.md` - Design philosophy explanation
- `PARSER_UPDATE_SUMMARY.md` - Recent parser changes documentation

## Example Code

**Factorial with pattern matching:**
```quark
fn fact n:
    when n:
        0 or 1: 1
        _: n * fact n - 1

exp 10, 2 | fact | print
```

**Fibonacci with ternary:**
```quark
fn fib n:
    n if n <= 1 else fib (n - 1) + fib (n - 2)

fib 5 | print
```

**If-elseif-else:**
```quark
if age < 16:
    'Can't drive'
elseif age == 16:
    'Get a learner's permit'
else:
    "Drive safe!"
```

## Important Conventions

- **Use `elseif` not `elif`:** Changed to be more English-like
- **Minimal parentheses:** Only use when necessary (see philosophy)
- **Space for function calls:** `func x, y` not `func(x, y)`
- **Pipe for chaining:** `x | f | g` not nested calls
- **Underscore for wildcard:** `_` in pattern matching
- **Python-style indentation:** Blocks defined by indentation level
