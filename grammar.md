# Quark Language Grammar

The syntax of Quark as the compiler in `src/core/quark` accepts it: tokens (`lexer/`, `token/`), statements and expressions (`parser/`), and operator precedence. What programs mean at runtime is in **semantics.md**; builtins are in **stdlib.md**.

## Contents

1. [Notation](#1-notation)
2. [Symbols](#2-symbols)
3. [Lexical Elements](#3-lexical-elements)
4. [Program Structure](#4-program-structure)
5. [Modules, Imports and Extern](#5-modules-imports-and-extern)
6. [Functions and Lambdas](#6-functions-and-lambdas)
7. [Results and when](#7-results-and-when)
8. [Control Flow](#8-control-flow)
9. [Types and Typed Declarations](#9-types-and-typed-declarations) ([structs](#91-structs))
10. [Expressions](#10-expressions) ([precedence](#103-precedence-low-to-high), [pipes](#104-pipes))
11. [Not in the Language](#11-not-in-the-language)
12. [Source of Truth](#12-source-of-truth)

## 1) Notation

- `::=` means "is defined as"
- `|` separates alternatives
- `{ ... }` means zero or more repetitions
- `[ ... ]` means optional
- Quoted text is a literal token; `ID`, `INT`, `FLOAT`, `STRING`, `NEWLINE`, `INDENT`, `DEDENT` are lexer token categories

## 2) Symbols

| Symbol | Meaning | Use |
|---|---|---|
| `->` | produces | function bodies, `when` arms |
| `:` | contains / has type | block headers, type annotations, dict and struct entries |
| `\|` | pipe to | `x \| f()` is `f(x)` |
| `.` | member of a value | dict keys, struct fields, method calls |
| `::` | qualified name | a member of a module or an imported file (`io::exists`, `geo::Point`, `core::helpers::double`) |

## 3) Lexical Elements

### 3.1 Keywords

Reserved (`token/token.go`):

`incl as module extern fn struct if elseif else for while break continue when in and or true false null ok err list dict vec result`

`_` is its own token (the wildcard pattern). The type names `int`, `float`, `str`, `bool`, `any` and `resource` are ordinary identifiers that the checker resolves in type positions. `link` and `include_path` are contextual words after `extern`.

Identifiers are letters, digits and `_`, starting with a letter or `_`. A name containing `__` is a load error: that form is reserved for compiler-generated names.

### 3.2 Operators and delimiters

- Arithmetic: `+ - * / % **`
- Comparison: `< <= > >= == !=`
- Logical: `and or !` (there are no `&&`, `||` or `not`)
- Assignment: `=`
- Flow: `-> |`
- Access and call: `. [ ] ( )`, qualified names `::`
- Punctuation: `, : { }`

`;` and `#` are not tokens.

### 3.3 Literals

- `INT`: decimal digits (`42`). There are no hex, octal or binary forms.
- Digit separators: `_` may stand between two digits of an `INT` or `FLOAT`, in any part of it (`100_000`, `1_000.5`, `0.000_1`, `1e1_0`), and is ignored. Anywhere else in a number it is a lex error (`1_`, `1__0`, `1_.5`, `1._5`, `1_e5`: `'_' in a number must be between two digits`); `_1` is a name.
- `FLOAT`: digits, a dot and digits (`3.14`), or digits and a dot (`2.` is `2.0`), optionally followed by an exponent: `e` or `E`, an optional sign, and digits (`1e3`, `2.5E-4`, `6e+2`). An exponent makes the literal a float even without a dot. A leading dot (`.5`) is a parse error.
- `STRING`: single-quoted or double-quoted, on one line.
- `true`, `false`, `null`.

Because `2.` is a float, a method call directly on an int literal does not work: `42.upper()` lexes as the float `42.` followed by `upper()`.

String escapes: `\\`, `\n`, `\t`, `\r`, and the string's own quote (`\'` in single-quoted strings, `\"` in double-quoted ones). Any other backslash is kept as written (`'a\qb'` is the four characters `a\qb`). `\0` is a `QK-PARSE-001` error: strings are NUL-terminated, so they can't contain a NUL character.

### 3.4 Comments and layout

- Line comments start with `//`.
- Blocks are indentation-based. A line ending in `:` or `->` must be followed by a more indented line (`expected indented block` otherwise), which opens a block (`INDENT`); returning to an enclosing level closes it (`DEDENT`). An indented line anywhere else is `unexpected indent`; dedenting to a level that was never opened is `inconsistent indentation`.
- Newlines inside `( )`, `[ ]` and `{ }` are ignored, so calls and literals may span lines.
- A line that starts with `|` continues the expression on the line above (§10.4), at the same indentation or deeper; it opens no block, so a deeper `|` line is not `unexpected indent`. A `|` line indented less than the expression's first line, or after an expression that ends with an indented block, is `'|' can't start a statement`. A line can't end with `|`.
- Blank lines and comment-only lines are ignored.

## 4) Program Structure

```ebnf
Program         ::= { Statement NEWLINE } EOF

Statement       ::= FunctionDef
                |   StructDef
                |   ModuleDef
                |   InclStatement
                |   ExternStatement
                |   IfStatement
                |   WhenExpr
                |   ForLoop
                |   WhileLoop
                |   "break"
                |   "continue"
                |   TypedDecl
                |   Expression

Block           ::= NEWLINE INDENT { Statement NEWLINE } DEDENT
                |   Statement                         (inline, on the header's line)
```

Each statement ends at a newline. An inline block is one statement on the header's line: `if x > 5: print('big')`, `fn sq(x) -> x * x`. Anything after a complete statement on its line is a parse error, never a second statement: `total = price qty` is `unexpected 'qty' after 'price'; put each statement on its own line, or join the two with an operator`. The same holds for a `when` arm and a struct field, one per line. A statement that ends with an indented block of its own ends with that block, so the next line starts a new statement: after `fn pick(x) -> when x:` and its arms, the next line is not part of `pick`.

Where statements may appear:
- `incl`, `extern link`, `extern include_path` and `module`: the top level of a file only.
- `struct`: the top level or a module body.
- An imported file may only contain definitions (semantics.md §10.4).

## 5) Modules, Imports and Extern

### 5.1 Module definition

```ebnf
ModuleDef       ::= "module" ID ":" Block
```

Members of a module are written `module::name`. Modules can't be nested.

### 5.2 Incl and qualified names

```ebnf
InclStatement   ::= "incl" STRING [ "as" ID ]

QualifiedName   ::= ID "::" ID { "::" ID }
TypeName        ::= ID { "::" ID }
```

- `incl './path'`, `incl '../path'`, `incl '/abs/path'`, `incl 'C:/path'`: a file, bound under its stem.
- `incl 'std/name'`: a stdlib module, bound as `name`.
- `as alias` binds the import under `alias` instead.
- `incl name` (an identifier) is a load error: a module of the same file needs no `incl`.

A `QualifiedName` is a value, a call target or a pipe target; a `TypeName` is an annotation or a struct literal's name (`geo::Point { x: 1 }`). Resolution rules are in semantics.md §10.

### 5.3 Extern declarations

```ebnf
ExternStatement   ::= "extern" STRING                          (header)
                  |   "extern" "link" STRING
                  |   "extern" "include_path" STRING
                  |   ExternFnDecl

ExternFnDecl      ::= "extern" "fn" ExternName Parameters [ Type ] "as" STRING
ExternName        ::= ID | ReceiverType "." ID
ReceiverType      ::= "int" | "float" | "str" | "bool" | "list" | "dict" | "vec"
```

- `extern 'path.hpp'` `#include`s a C++ header, resolved relative to the declaring file (`std/...` in the stdlib).
- `extern fn name(...) T as 'symbol'` binds a free function; `extern fn list.name(...) T as 'symbol'` adds a method to a builtin type. The return type comes before `as`, with no `->`.
- Nothing may follow a directive's string on its line. `extern link` takes a bare library name (`'openblas'`, not `'-lopenblas'` or `'libopenblas.so'`).
- `extern source` and `extern include` are parse errors (`extern source` is planned: `ffi_v0_1_spec.md` P2).

Semantics: semantics.md §10.8 and `ffi_v0_1_spec.md`.

## 6) Functions and Lambdas

### 6.1 Function declaration

```ebnf
FunctionDef     ::= "fn" ID Parameters [ Type ] "->" Block
Parameters      ::= "(" [ Param { "," Param } [ "," ] ] ")"
Param           ::= ID [ ":" Type ] [ "=" DefaultLiteral ]
DefaultLiteral  ::= INT | FLOAT | STRING | "true" | "false" | "null"
                |   "-" { "-" } ( INT | FLOAT )
                |   "list" "[" "]"
```

- Parentheses are required, also for no parameters: `fn main() -> ...`.
- A parameter with a default can't be followed by one without (`required parameter 'b' cannot follow a parameter with a default value`).
- A non-literal default (`x = 1 + 1`) is a parse error.

### 6.2 Lambda expression

```ebnf
LambdaExpr      ::= "fn" Parameters [ Type ] "->" Expression
```

A lambda's body is one expression on the same line; an indented lambda body is a parse error. Use a `fn` declaration for a body of several statements.

### 6.3 Return type annotations

The return type goes between `)` and `->`:

```quark
fn add(x: int, y: int) int -> x + y
fn greet(name: str) str -> 'Hello, ' + name
double = fn(x: int) int -> x * 2
fn origin() geo::Point -> geo::Point { x: 0, y: 0 }
```

Parameter and return annotation rules are in semantics.md §13; default parameters in semantics.md §6.4.

## 7) Results and when

### 7.1 Result constructors

```ebnf
ResultExpr      ::= ( "ok" | "err" ) Expression
```

The operand extends as far as possible: `ok 5 == 1` is `ok (5 == 1)`, and `ok(5) == ok 5` is `ok ((5) == ok 5)`. Parenthesize a result to compare it: `(ok 5) == (ok 5)`.

### 7.2 When

```ebnf
WhenExpr        ::= "when" Expression ":" NEWLINE INDENT
                    { Pattern "->" Expression NEWLINE }
                    DEDENT

Pattern         ::= ResultPattern
                |   "_"
                |   LiteralPattern { "or" LiteralPattern }

ResultPattern   ::= ( "ok" | "err" ) ( ID | "_" )
LiteralPattern  ::= INT | FLOAT | STRING | "true" | "false" | "null"
                |   "-" ( INT | FLOAT )
```

- `when` is a statement and an expression (§10.1). As an expression it must end its line: `size = when n:` followed by the arms.
- The arms are an indented block; each arm's body is one expression.
- An `ok`/`err` pattern stands alone in its arm: `ok x or err x` is a parse error.
- A pattern that isn't a literal (a variable, a call) is a compile-time error (`when patterns must be literal values ...`).

Matching rules: semantics.md §8.

## 8) Control Flow

```ebnf
IfStatement     ::= "if" Expression ":" Block
                    { "elseif" Expression ":" Block }
                    [ "else" ":" Block ]

ForLoop         ::= "for" ID "in" Expression ":" Block
WhileLoop       ::= "while" Expression ":" Block
```

`if` is also an expression in the ternary form `a if cond else b` (§10.1). `break` and `continue` apply to the innermost `for` or `while`; outside a loop they are compile-time errors. `in` appears only in `for`; there is no `in` operator.

## 9) Types and Typed Declarations

```ebnf
TypedDecl       ::= ID ":" Type "=" Expression
Type            ::= "int" | "float" | "str" | "bool" | "any"
                |   "list" | "dict" | "vec" | "result" | "resource" | "fn"
                |   TypeName                                  (a struct: Point, geo::Point)
```

- `Type` is used in variable, parameter, struct field and return annotations.
- `fn` is any function value; there is no signature syntax such as `fn(int) int`.
- Element types can't be written: `x: list[int] = ...` is a parse error (planned: `tables_v0_1_spec.md` D16). `tbl` is not a type yet (`unknown type 'tbl'`; planned as `tbl[Row]`, D10).
- `null` is a parse error in a type position; `void` and other names are a compile-time error (`unknown type 'void'`).
- A typed declaration needs a value: `x: int` alone is a parse error.

### 9.1 Structs

```ebnf
StructDef       ::= "struct" ID ":" NEWLINE INDENT
                    { StructField NEWLINE }
                    DEDENT
StructField     ::= ID ":" Type [ "=" ConstExpr ]
ConstExpr       ::= Literal
                |   "-" ConstExpr
                |   ConstExpr ( "+" | "-" | "*" | "/" | "%" ) ConstExpr
                |   "list" "[" "]" | "dict" "{" "}"

StructLiteral   ::= TypeName "{" [ FieldInit { ( "," | NEWLINE ) FieldInit } [ "," ] ] "}"
FieldInit       ::= ID ":" PipeExpr
```

```quark
struct Customer:
    id: int
    name: str
    age: int = 0

c = Customer { id: 1, name: 'Alice' }
print(c.name)     // Alice
print(type(c))    // Customer
```

- A name followed by `{` is always a struct literal; the checker rejects a name that isn't a struct. A struct of a module or another file is written with `::` (`geo::Point { x: 1 }`; `geo.Point { ... }` is a parse error).
- Field defaults must be constant expressions (`timeout: int = 60 * 60`); anything else is a `QK-PARSE-001` error.
- A literal must set every field without a default. Missing, unknown and mistyped fields are compile-time errors.
- Fields are read with `c.name`. Struct values are immutable: `c.age = 31` is a compile-time error.
- `==` and `!=` on structs are compile-time errors; compare fields.

Full rules: `structs_v0_1_spec.md`.

## 10) Expressions

### 10.1 Expression grammar

```ebnf
Expression      ::= Assignment

Assignment      ::= PipeExpr
                |   AssignTarget "=" PipeExpr

AssignTarget    ::= ID
                |   Postfix "." ID                (dict key)
                |   Postfix "[" Expression "]"    (list element)

PipeExpr        ::= Ternary { "|" PipeTarget }
PipeTarget      ::= Ternary                       (must be a call, §10.4)

Ternary         ::= LogicalOr [ "if" LogicalOr "else" Ternary ]

LogicalOr       ::= LogicalAnd { "or" LogicalAnd }
LogicalAnd      ::= Equality { "and" Equality }
Equality        ::= Comparison { ( "==" | "!=" ) Comparison }
Comparison      ::= Additive { ( "<" | "<=" | ">" | ">=" ) Additive }
Additive        ::= Multiplicative { ( "+" | "-" ) Multiplicative }
Multiplicative  ::= Unary { ( "*" | "/" | "%" ) Unary }
Unary           ::= ( "!" | "-" ) Unary | Power
Power           ::= Postfix [ "**" Unary ]

Postfix         ::= Primary { "." ID | "[" Expression "]" | CallArgs }
CallArgs        ::= "(" [ PipeExpr { "," PipeExpr } [ "," ] ] ")"

Primary         ::= ID
                |   "_"
                |   QualifiedName
                |   Literal
                |   ResultExpr
                |   WhenExpr
                |   LambdaExpr
                |   StructLiteral
                |   ListLiteral
                |   VectorLiteral
                |   DictLiteral
                |   "(" Expression ")"
```

Call arguments, collection elements and field values are `PipeExpr`s, so an assignment can't appear there. An assignment is a statement: inside parentheses (`a and (n = f())`, `x + (x = 5)`) or as an `elseif` condition it is a parse error (`an assignment can't be part of an expression; assign it on its own line first`), since it could run conditionally and leave the variable unassigned. It may be the condition of an `if` or a `while` (`while line = next():`), which always runs. Assignment isn't chainable: `a = b = 1` is a compile-time error. Other assignment targets are compile-time errors too: a struct field (structs are immutable), a string index (strings are immutable), an imported name. `x: T = ...` (§9) is a separate statement form, recognized when a line starts with `ID ":"`.

`d['key']` parses but is a compile-time error: dict keys are read with `d.key` or `d.get(k)` (semantics.md §9.2).

### 10.2 Literal forms

```ebnf
Literal         ::= INT | FLOAT | STRING | "true" | "false" | "null"

ListLiteral     ::= "list" "[" [ PipeExpr { "," PipeExpr } [ "," ] ] "]"
VectorLiteral   ::= "vec" "[" [ PipeExpr { "," PipeExpr } [ "," ] ] "]"
DictLiteral     ::= "dict" "{" [ DictEntry { "," DictEntry } [ "," ] ] "}"
DictEntry       ::= ID ":" PipeExpr
```

- The `list`, `vec` and `dict` keywords are required; `[1, 2]` alone is a parse error.
- Dict literal keys are identifiers (`dict { name: 'a' }`); a string key (`dict { 'a': 1 }`) is a parse error.
- Vector literals are one-dimensional; a `;` row separator is a parse error.

### 10.3 Precedence (low to high)

This is the canonical precedence table (`parser/expr.go`, `ast.Precedence`).

| Level | Operators | Associativity |
|---|---|---|
| 1 | `=` | not chainable |
| 2 | `\|` | left |
| 3 | `a if cond else b` | right |
| 4 | `or` | left |
| 5 | `and` | left |
| 6 | `==` `!=` | left |
| 7 | `<` `<=` `>` `>=` | left |
| 8 | `+` `-` | left |
| 9 | `*` `/` `%` | left |
| 10 | prefix `!` `-` | prefix |
| 11 | `**` | right |
| 12 | `.` `[]` `()` | left |

- `**` binds tighter than a prefix operator, as in math and Python: `-2 ** 2` is `-(2 ** 2)`, which is `-4`; write `(-2) ** 2` for `4`. The right operand of `**` may be negated (`2 ** -1`), and `-x ** 2 ** 3` is `-(x ** (2 ** 3))`. Every other binary operator binds looser than a prefix operator: `-a * b` is `(-a) * b` and `!a == b` is `(!a) == b`.
- `-2 ** 63` is `-(2 ** 63)`, which overflows; the smallest int can be written `-(2 ** 62) * 2`.
- The ternary's condition is an `or` expression; its `else` branch may be another ternary: `a if c else b if d else e`.
- `ok` and `err` take the rest of the expression as their operand (§7.1).

### 10.4 Pipes

`x | f(a)` is parsed directly as the call `f(x, a)`, and `x | obj.m(a)` as `obj.m(x, a)`; no later stage sees a pipe. The target is parsed at ternary level and must be a call: `x | f`, `x | f() + 1` and `x | f() if c else g()` are parse errors (`pipe target must be a function call`). Parenthesize to use a pipe's result: `(x | f()) + 1`.

Since `|` binds looser than the ternary and `or`, the whole left side is piped: `a if c else b | f()` is `f(a if c else b)`, and `0 or 3 | f()` is `f(0 or 3)`.

A line starting with `|` continues the expression on the line above, indented deeper or at the same indentation; comment and blank lines may come between:

```quark
y = 5
    | double()
    // then one more
    | inc()

z = 5
| double()
```

The `|` goes at the start of the continuing line: a line ending with `|` is a parse error (`a line can't end with '|'; start the next line with it instead`). A `|` line can't be indented less than the first line of the expression it continues, and can't follow an expression that ends with an indented block (a `when` used as a value); assign that to a variable and pipe the variable.

What a pipe means for arity and checking: semantics.md §7.

## 11) Not in the Language

These are not part of Quark, by decision or because they are planned in a spec:

| Feature | Status |
|---|---|
| `tbl[Row]` type and queries (`t \| e -> verb(...)`) | Planned: `tables_v0_1_spec.md` (D3, D10); the runtime engine exists |
| Written element types (`list[int]`) | Planned: `tables_v0_1_spec.md` D16 |
| `extern source` | Planned: `ffi_v0_1_spec.md` P2 |
| `struct` inside a function body | Not supported; currently an internal error (known_issues.md) |
| `return` | None: a function's value is its last statement (semantics.md §6.1) |
| `in` as an operator | None: use `any(v == x)`, `d.get(k) != null`, `s.contains(sub)` |
| String interpolation, varargs, keyword arguments | None (`stdlib_expansion_v0_1_spec.md` §1) |
| Struct methods | None (`structs_v0_1_spec.md`) |
| Exceptions | None: runtime errors are fatal, expected failures are results (semantics.md §11) |

## 12) Source of Truth

- `grammar.md` (this file): syntax, tokens and precedence.
- `semantics.md`: runtime behaviour, types, scoping, the error model and design decisions.
- `stdlib.md`: builtin functions, methods and stdlib modules. The code-level source of truth for builtin names, arity and types is `src/core/quark/builtins/catalog.go`.
- `architecture.md`: compiler pipeline, runtime structures and the back end.
- `modules_v0_2_spec.md`, `structs_v0_1_spec.md`, `ffi_v0_1_spec.md`, `tables_v0_1_spec.md`: the full rules of each area.

A change to the language updates these docs in the same change. A conflict between a doc and the implementation is a bug to fix before adding features.
