# Quark Language Grammar (Implementation-Synced)

This document is the grammar and semantic reference for the Quark compiler in `src/core/quark`.

## 1) Notation

- `::=` means “is defined as”
- `|` separates alternatives
- `{ ... }` means zero or more repetitions
- `[ ... ]` means optional
- `TOKEN` names refer to lexer token categories

## 2) Language Philosophy

### Symbol meanings

| Symbol | Meaning | Primary use |
|---|---|---|
| `->` | produces/maps to | function bodies, `when` pattern results |
| `:` | contains/has type | block headers, type annotations, dict entries |
| `\|` | pipe to | dataflow chaining |
| `.` | member of | dict key access only |

## 3) Lexical Elements

### 3.1 Keywords (reserved)

`use, module, fn, if, elseif, else, for, while, break, continue, when, in, and, or, true, false, null, ok, err, list, dict, vector`

### 3.2 Operators and delimiters

- Arithmetic: `+ - * / % **`
- Comparison/equality: `< <= > >= == !=`
- Assignment: `=`
- Unary: `! -`
- Flow: `-> |`
- Access/call: `. [] ()`
- Punctuation: `, : { } [ ]`

### 3.3 Literals

- `INT`
- `FLOAT`
- `STRING` (single-quoted or double-quoted in source)
- `BOOL` (`true`, `false`)
- `NULL` (`null`)

Planned lexical additions for interpolation:
- Interpolation opener: `!{`
- Interpolation closer: `}`
- Interpolation is only recognized inside string literals

### 3.4 Comments and layout

- Line comments: `// ...`
- Python-style indentation blocks with `INDENT`/`DEDENT` token injection
- Indentation blocks are triggered after `:` and `->`

## 4) Program Structure

```ebnf
Program         ::= { Statement NEWLINE } EOF

Statement       ::= FunctionDef
                |   ModuleDef
                |   UseStatement
                |   IfStatement
                |   WhenStatement
                |   ForLoop
                |   WhileLoop
                |   BreakStatement
                |   ContinueStatement
                |   TypedDecl
                |   Expression
                |   NEWLINE
```

## 5) Modules and Imports

### 5.1 Module definition

```ebnf
ModuleDef       ::= "module" ID ":" Block
```

### 5.2 Use forms

```ebnf
UseStatement    ::= "use" ID
                |   "use" STRING
```

Semantics:
- `use ID`: same-file module import
- `use './path'` or `use '../path'`: file import resolved by loader
- `use 'name'` (non-relative string): currently rejected by loader as stdlib-import-not-yet-supported

## 6) Functions and Lambdas

### 6.1 Named function

```ebnf
FunctionDef     ::= "fn" ID Parameters "->" BlockOrExpr
Parameters      ::= "(" [ ParamList ] ")"
ParamList       ::= Param { "," Param } [ "," ]
Param           ::= ID [ ":" Type ]
```

### 6.2 Lambda expression

```ebnf
LambdaExpr      ::= "fn" Parameters "->" Expression
```

Notes:
- Parentheses are required for both named and lambda parameters
- Named functions may have inline or indented bodies
- Lambda body is expression form

## 7) Result Values and Matching

### 7.1 Result constructors

```ebnf
ResultExpr      ::= "ok" Expression
                |   "err" Expression
```

### 7.2 When statement

```ebnf
WhenStatement   ::= "when" Expression ":" NEWLINE INDENT
                    { Pattern "->" Expression NEWLINE }
                    DEDENT

Pattern         ::= ResultPattern
                |   WildcardPattern
                |   Expression { "or" Expression }

ResultPattern   ::= "ok" (ID | "_")
                |   "err" (ID | "_")

WildcardPattern ::= "_"
```

Semantics:
- Pattern arms are checked in source order
- Result propagation is explicit (no implicit bubbling)
- Nested `when` is supported

## 8) Control Flow

### 8.1 If / elseif / else

```ebnf
IfStatement     ::= "if" Expression ":" Block
                    { "elseif" Expression ":" Block }
                    [ "else" ":" Block ]
```

### 8.2 For loop

```ebnf
ForLoop         ::= "for" ID "in" Expression ":" Block
```

Current semantic restriction:
- Iterable must be `list` or `vector`

### 8.3 While loop

```ebnf
WhileLoop       ::= "while" Expression ":" Block
```

### 8.4 Loop control (planned)

```ebnf
BreakStatement      ::= "break"
ContinueStatement   ::= "continue"
```

Planned semantics:
- `break` exits the nearest enclosing `for`/`while`
- `continue` skips to the next iteration of the nearest enclosing `for`/`while`
- Using either outside a loop is a compile-time error

## 9) Types and Typed Declarations

```ebnf
TypedDecl       ::= ID ":" Type "=" Expression
Type            ::= "int" | "float" | "str" | "bool" | "null" | "any"
                |   "list" | "dict" | "vector"
                |   ID
```

Notes:
- Generic type syntax like `list[int]` is not implemented
- `result` is modeled in analyzer/runtime semantics but not yet exposed as a user type annotation keyword

## 10) Expressions and Precedence

### 10.1 Expression grammar

```ebnf
Expression      ::= Assignment

Assignment      ::= PipeExpr
                |   AssignTarget "=" Assignment

AssignTarget    ::= ID
                |   MemberAccess
                |   IndexExpr

PipeExpr        ::= Ternary { "|" CallExpr }

Ternary         ::= LogicalOr [ "if" LogicalOr "else" Ternary ]

LogicalOr       ::= LogicalAnd { "or" LogicalAnd }
LogicalAnd      ::= Equality { "and" Equality }
Equality        ::= Comparison { ("==" | "!=") Comparison }
Comparison      ::= Additive { ("<" | "<=" | ">" | ">=") Additive }
Additive        ::= Multiplicative { ("+" | "-") Multiplicative }
Multiplicative  ::= Exponent { ("*" | "/" | "%") Exponent }
Exponent        ::= Unary [ "**" Exponent ]
Unary           ::= ("!" | "-") Unary | Postfix

Postfix         ::= Primary { Accessor | CallArgs }
Accessor        ::= "." ID | "[" Expression "]"
CallArgs        ::= "(" [ Arguments ] ")"
Arguments       ::= Expression { "," Expression } [ "," ]

Primary         ::= ID
                |   Literal
                |   ResultExpr
                |   LambdaExpr
                |   ListLiteral
                |   VectorLiteral
                |   DictLiteral
                |   "(" Expression ")"
```

### 10.2 Literal forms

```ebnf
Literal         ::= INT
                |   FLOAT
                |   StringLiteral
                |   BOOL
                |   NULL

StringLiteral   ::= PlainString
                |   InterpolatedString

PlainString     ::= SQString | DQString
SQString        ::= "'" { SQChar } "'"
DQString        ::= '"' { DQChar } '"'

InterpolatedString ::= "'" { SQSegment } "'"
                    | '"' { DQSegment } '"'

SQSegment       ::= SQText | Interpolation
DQSegment       ::= DQText | Interpolation
Interpolation   ::= "!{" Expression "}"

ListLiteral     ::= "list" "[" [ Expression { "," Expression } [ "," ] ] "]"
VectorLiteral   ::= "vector" "[" [ Expression { "," Expression } [ "," ] ] "]"
DictLiteral     ::= "dict" "{" [ DictEntries ] "}"
DictEntries     ::= DictEntry { "," DictEntry } [ "," ]
DictEntry       ::= ID ":" Expression
```

Interpolation parsing rules (planned):
- Anything between `!{` and the matching `}` is parsed as a normal `Expression`
- Interpolations may appear multiple times in one string
- `!{` without a matching `}` is a compile-time error
- Unescaped delimiter quotes must match the opening quote style
- Nested interpolation is not allowed inside interpolation expressions

### 10.3 Precedence (low to high)

| Level | Operators |
|---|---|
| 1 | `=` |
| 2 | `|` |
| 3 | ternary `a if cond else b` |
| 4 | `or` |
| 5 | `and` |
| 6 | `== !=` |
| 7 | `< <= > >=` |
| 8 | `+ -` |
| 9 | `* / %` |
| 10 | `**` (right-associative) |
| 11 | unary `! -` |
| 12 | postfix `. [] ()` |

## 11) Runtime-Oriented Semantic Rules

### 11.1 Dict/member rules

- `d.key` reads dict key
- `d.key = value` writes dict key
- Dot access on non-dict is an analyzer/runtime error
- Dot-call (`x.f()`) is unsupported

### 11.2 Indexing

- `list[idx]` supported (negative indices supported at runtime)
- `str[idx]` supported (returns single-character string)
- `vector[idx]` supported
- `vector[mask]` where `mask` is bool vector supported
- Dict bracket indexing is not supported; use dot access or `dget/dset`

### 11.3 Assignment targets

Allowed:
- `x = expr`
- `d.key = expr`
- `list[i] = expr`

Disallowed/diagnosed:
- assigning through non-assignable expressions
- string index assignment (strings are immutable)

### 11.4 Result helpers

Builtins currently available:
- `is_ok(result) -> bool`
- `is_err(result) -> bool`
- `unwrap(result) -> any` (panics on `err` or non-result)

## 12) Builtin Surface (Current)

### 12.1 I/O
- `print`, `println`, `input`

### 12.2 Conversion/introspection
- `len`, `to_str`, `to_int`, `to_float`, `to_bool`, `type`
- `is_ok`, `is_err`, `unwrap`

### 12.3 Math and range
- `range`, `abs`, `min`, `max`, `sum`, `sqrt`, `floor`, `ceil`, `round`

### 12.4 String
- `upper`, `lower`, `trim`, `contains`, `startswith`, `endswith`, `replace`, `concat`, `split`

### 12.5 List
- `push`, `pop`, `get`, `set`, `insert`, `remove`, `slice`, `reverse`, `concat`

### 12.6 Dict
- `dget`, `dset`

### 12.7 Vector
- `to_vector`, `to_list`, `fillna`, `astype`

## 13) Feature Status Matrix

| Feature | Status | Notes |
|---|---|---|
| Indentation blocks | Implemented | Triggered after `:` and `->` |
| `module` / `use` same-file | Implemented | `use ID` |
| `use STRING` file imports | Implemented | Relative paths only (`./`, `../`) |
| Stdlib string imports (`use 'csv'`) | Not implemented | Loader emits error |
| Named functions and lambdas | Implemented | Parenthesized params required |
| Typed params / typed var declarations | Implemented | Basic types + list/dict/vector |
| Generic type expressions | Not implemented | e.g. `list[int]` unsupported |
| List literals (`list [...]`) | Implemented | Keyword required |
| Vector literals (`vector [...]`) | Implemented | 1D only |
| Dict literals (`dict {k: v}`) | Implemented | Keys are identifiers in source |
| Loop control (`break`, `continue`) | Not implemented | Keywords reserved; parser/analyzer/codegen pending |
| Dot-call syntax | Not implemented | Use function-call/pipe model |
| Dot data access on dict | Implemented | read/write |
| Result values `ok` / `err` | Implemented | Analyzer has `ResultType` |
| `when` result patterns | Implemented | `ok x`, `err e` |
| Double-quoted strings | Not implemented | Grammar reserved; current source convention is single quotes |
| String interpolation (`!{expr}`) | Not implemented | Grammar reserved; lexer/parser/analyzer/codegen/runtime pending |
| `unwrap_or`, `map_ok`, etc. | Future | Not implemented yet |
| Structs / impl blocks | Future | Not implemented |
| Tensor type | Future | Not implemented |

## 14) Known Limits / Current Diagnostics

- `for` iterables are currently restricted to list/vector
- `break` and `continue` are reserved keywords but not implemented yet
- `use 'name'` (non-relative string) is rejected pending stdlib import support
- Dict bracket indexing is rejected by analyzer
- Dot access is dict-only; non-dict dot access is diagnosed
- Double-quoted strings and interpolation (`!{...}`) are grammar-defined but not implemented yet

## 15) Source of Truth Policy

To reduce drift:
- Parser/lexer behavior in `src/core/quark/{lexer,parser}` is canonical for syntax
- Analyzer behavior in `src/core/quark/types/analyzer.go` is canonical for semantic checks
- Builtin registry in `src/core/quark/codegen/builtins.go` is canonical for available builtins

If this document conflicts with code, update this document immediately after code changes.
