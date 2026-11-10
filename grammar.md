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
| `.` | member of | dict keys, struct fields, method calls and module-qualified calls |

## 3) Lexical Elements

### 3.1 Keywords (reserved)

`use, as, module, extern, fn, struct, if, elseif, else, for, while, break, continue, when, in, and, or, true, false, null, ok, err, list, dict, vector, result`

`link` and `include_path` are contextual words after `extern`, not reserved keywords.

### 3.2 Operators and delimiters

- Arithmetic: `+ - * / % **`
- Comparison/equality: `< <= > >= == !=`
- Assignment: `=`
- Unary: `! -`
- Flow: `-> |`
- Access/call: `. [] ()`
- Punctuation: `, : { } [ ]`

### 3.3 Literals

- `INT`: decimal digits (`42`). There are no hex, octal or binary forms and no digit separators.
- `FLOAT`: digits, a dot, and digits (`3.14`), or digits followed by a dot (`2.` is `2.0`), optionally followed by an exponent: `e` or `E`, an optional sign, and digits (`1e3`, `2.5E-4`, `6e+2`). An exponent makes the literal a float even without a dot. A leading dot (`.5`) is not accepted.
- `STRING` (single-quoted or double-quoted)
- `BOOL` (`true`, `false`)
- `NULL` (`null`)

Because `2.` is a float, a method call directly on an int literal does not work: `42.upper()` lexes as the float `42.` followed by `upper()`. Call methods on a variable instead.

Supported string escape sequences:
- `\\`
- `\'` in single-quoted strings
- `\"` in double-quoted strings
- `\n`, `\t`, `\r`

`\0` is rejected (`QK-PARSE-001`): strings are NUL-terminated, so they can't contain a NUL character.

Planned lexical additions (future):
- Interpolation opener: `!{`
- Interpolation closer: `}`
- Interpolation recognized only inside string literals

### 3.4 Comments and layout

- Line comments: `// ...`
- Python-style indentation blocks with `INDENT`/`DEDENT` token injection
- Indentation blocks are triggered after `:` and `->`

## 4) Program Structure

```ebnf
Program         ::= { Statement NEWLINE } EOF

Statement       ::= FunctionDef
                |   StructDef
                |   ModuleDef
                |   UseStatement
                |   ExternStatement
                |   IfStatement
                |   WhenExpr
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
UseStatement    ::= "use" ( ID | STRING ) [ "as" ID ]

ExternStatement ::= ExternHeader
                |   ExternLink
                |   ExternIncludePath
                |   ExternFnDecl

ExternHeader      ::= "extern" STRING
ExternLink        ::= "extern" "link" STRING
ExternIncludePath ::= "extern" "include_path" STRING

ExternFnDecl    ::= "extern" "fn" ExternName Parameters [ ReturnType ] "as" STRING
ExternName      ::= ID | BuiltinTypeName "." ID
BuiltinTypeName ::= "int" | "float" | "str" | "bool" | "list" | "dict" | "vector"
```

Semantics:
- `use ID`: same-file module import
- `use './path'` or `use '../path'`: file import resolved by loader
- `use 'C:/path/to/file'` or `use '/path/to/file'`: absolute file import resolved by loader
- `use 'std/name'`: stdlib import resolved from stdlib root (`QUARK_STDLIB_ROOT` or discovered `stdlib/` directory)
- `use ... as alias`: binds `alias` as a module qualifier for `alias.symbol(...)` calls

Extern/QEI semantics:
- `extern 'path.hpp'` `#include`s a C++ header. The path is resolved by the loader relative to the declaring file (`std/...` paths resolve in the stdlib).
- `extern link 'name'` links a native library (`-lname`). `extern include_path 'dir'` adds a C++ include search directory, relative to the declaring file. See semantics.md §10.6 and `ffi_v0_1_spec.md` P1.
- Directives must be at the top level of a file (inside a `module` block or any other block they are a `QK-LOAD-001` error), and nothing may follow the string.
- `extern source` is reserved for separately compiled glue files and is not supported yet (`QK-PARSE-001`).
- `extern fn name(...) T as 'symbol'` declares a free extern function.
- `extern fn type.name(...) T as 'symbol'` declares a receiver method extension.
- Free extern functions can be used as first-class values via generated thunks.

## 6) Functions and Lambdas

### 6.1 Named function

```ebnf
FunctionDef     ::= "fn" ID Parameters [ ReturnType ] "->" BlockOrExpr
Parameters      ::= "(" [ ParamList ] ")"
ParamList       ::= Param { "," Param } [ "," ]
Param           ::= ID [ ":" Type ] [ "=" DefaultLiteral ]
DefaultLiteral  ::= INT | FLOAT | STRING | BOOL | NULL | "-" INT | "-" FLOAT | "list" "[" "]"
```

### 6.2 Lambda expression

```ebnf
LambdaExpr      ::= "fn" Parameters [ ReturnType ] "->" Expression
```

### 6.3 Return type annotations

Return type annotations appear between the closing `)` and the `->` arrow:

```quark
fn add(x: int, y: int) int -> x + y
fn greet(name: str) str -> 'Hello, ' + name
double = fn(x: int) int -> x * 2
```

Semantics:
- The annotation is compile-time only — all runtime values are `QValue`
- Parameter annotations on Quark functions are compile-time only too: nothing is checked or converted at runtime. Only `extern fn` parameters are converted (see semantics.md §2)
- The analyzer infers the actual return type from the body and checks it against the annotation
- A mismatch is a compile-time error (e.g., annotated `int` but body returns `str`)
- Union types with `void` from incomplete branches (e.g., `when` without wildcard) are accepted if the non-void component matches the annotation

### 6.4 Default parameters

Parameters may have default values specified with `= literal`:

```quark
fn greet(name, greeting = 'Hello') -> greeting + ', ' + name
fn add_n(x, n = 1) -> x + n
fn point(x = 0, y = 0) -> list [x, y]
```

Semantics:
- Only literal values are allowed as defaults (int, float, string, bool, null, negated numerics, empty list)
- Required parameters must come before parameters with defaults — mixing is a compile-time error
- When a type annotation is present, the default value type must be compatible with the annotation
- When no type annotation is present, the type is inferred from the default value (null infers `any`)
- At the call site, omitted trailing arguments are filled with their defaults
- Arity checking uses a range: `MinArity` (required params) to `MaxArity` (total params)
- Pipes interact with defaults: `5 | add_n()` fills the default for `n`

Notes:
- Parentheses are required for both named and lambda parameters
- Named functions may have inline or indented bodies
- Lambda body is expression form

## 7) Result Values and Matching

### 7.1 Result constructors

```ebnf
ResultExpr      ::= "ok" PipeExpr
                |   "err" PipeExpr
```

The operand takes everything that binds tighter than `=`, so `ok 5 == 1` is `ok (5 == 1)` and `ok(5) == ok 5` is `ok ((5) == ok 5)`. Parenthesize a result to compare it: `(ok 5) == (ok 5)`.

### 7.2 When

```ebnf
WhenExpr        ::= "when" Expression ":" NEWLINE INDENT
                    { Pattern "->" Expression NEWLINE }
                    DEDENT

Pattern         ::= ResultPattern
                |   WildcardPattern
                |   LiteralPattern { "or" LiteralPattern }

LiteralPattern  ::= Literal | "-" INT | "-" FLOAT

ResultPattern   ::= "ok" (ID | "_")
                |   "err" (ID | "_")

WildcardPattern ::= "_"
```

Semantics:
- `when` is both a statement and an expression: it evaluates to the matched arm's value, so it can end a line such as `size = when n: ...` (see semantics.md §8)
- Pattern arms are checked in source order
- Value patterns are literals; they match when the scrutinee equals one of them
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

Supported iterables: `list`, `vector`, `str`

### 8.3 While loop

```ebnf
WhileLoop       ::= "while" Expression ":" Block
```

### 8.4 Loop control

```ebnf
BreakStatement      ::= "break"
ContinueStatement   ::= "continue"
```

Current semantics:
- `break` exits the nearest enclosing `for`/`while`
- `continue` skips to the next iteration of the nearest enclosing `for`/`while`
- Using either outside a loop is a compile-time error

## 9) Types and Typed Declarations

```ebnf
TypedDecl       ::= ID ":" Type "=" Expression
Type            ::= "int" | "float" | "str" | "bool" | "any" | "result"
                |   "list" | "dict" | "vector"
                |   "resource" | "file_handle"
                |   StructName
ReturnType      ::= Type
```

Notes:
- `Type` is accepted in variable, parameter, struct field and return type annotations (`resource` and `file_handle` included)
- Other names are a compile-time error (`unknown type 'void'`)
- Generic type syntax like `list[int]` is not implemented
- `any` is the current top-level escape-hatch annotation for gradual typing
- `null` is a runtime literal/value and is not accepted as an annotation; `void` appears only in inferred types (for example a non-exhaustive `when`), and `fn` is not an annotation
- `result` is a first-class Quark type annotation keyword
- `result` annotations are non-generic in v0.1 and represent a general result value

### 9.1 Structs

```ebnf
StructDef       ::= "struct" ID ":" NEWLINE INDENT
                    { StructField NEWLINE }
                    DEDENT
StructField     ::= ID ":" Type [ "=" ConstExpr ]
ConstExpr       ::= Literal | "-" ConstExpr
                |   ConstExpr ("+" | "-" | "*" | "/" | "%") ConstExpr
                |   "list" "[" "]" | "dict" "{" "}"

StructLiteral   ::= StructName "{" [ FieldInit { ( "," | NEWLINE ) FieldInit } [ "," ] ] "}"
FieldInit       ::= ID ":" PipeExpr
```

```quark
struct Customer:
    id: int
    name: str
    age: int = 0

c = Customer { id: 1, name: 'Alice' }
println(c.name)     // Alice
println(type(c))    // Customer
```

Semantics:
- `StructName` is a struct declared in the same file. Names of structs from imported files are not recognized as types or literals
- Field defaults must be constant expressions (`timeout: int = 60 * 60`); anything else is a `QK-PARSE-001` error
- A literal must set every field that has no default. Missing, unknown or mistyped fields are compile-time errors
- Fields are read with `c.name`. Struct values are immutable: `c.age = 31` is a compile-time error; build a new value instead
- `==` and `!=` on structs are compile-time errors; compare fields explicitly

## 10) Expressions and Precedence

### 10.1 Expression grammar

```ebnf
Expression      ::= Assignment

Assignment      ::= PipeExpr
                |   AssignTarget "=" PipeExpr

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
Arguments       ::= PipeExpr { "," PipeExpr } [ "," ]

Primary         ::= ID
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

### 10.2 Literal forms

```ebnf
Literal         ::= INT
                |   FLOAT
                |   StringLiteral
                |   BOOL
                |   NULL

StringLiteral   ::= SQString | DQString
SQString        ::= "'" { SQChar } "'"
DQString        ::= '"' { DQChar } '"'

ListLiteral     ::= "list" "[" [ PipeExpr { "," PipeExpr } [ "," ] ] "]"
VectorLiteral   ::= "vector" "[" [ PipeExpr { "," PipeExpr } [ "," ] ] "]"
DictLiteral     ::= "dict" "{" [ DictEntries ] "}"
DictEntries     ::= DictEntry { "," DictEntry } [ "," ]
DictEntry       ::= ID ":" PipeExpr
```

Interpolation parsing rules (future):
- String interpolation is not implemented in v0.1

Call arguments and collection elements are `PipeExpr`s, so an assignment cannot appear there. Assignment is not chainable: `a = b = 1` is a compile-time error.

### 10.3 Precedence (low to high)

| Level | Operators |
|---|---|
| 1 | `=` (not chainable) |
| 2 | `|` |
| 3 | ternary `a if cond else b` (right-associative) |
| 4 | `or` |
| 5 | `and` |
| 6 | `== !=` |
| 7 | `< <= > >=` |
| 8 | `+ -` |
| 9 | `* / %` |
| 10 | `**` (right-associative) |
| 11 | unary `! -` |
| 12 | postfix `. [] ()` |

Unary minus binds tighter than `**`: `-2 ** 2` is `(-2) ** 2`, which is `4`. Write `-(2 ** 2)` for `-4`.

## 11) Runtime-Oriented Semantic Rules

### 11.1 Dot syntax rules

Dot syntax (`.`) serves five purposes:

- `d.key` — dict key read
- `d.key = value` — dict key write
- `s.field` — struct field read (struct fields can't be assigned; see §9.1)
- `value.method(args)` — method dispatch (receiver type determines which method is resolved; see stdlib.md for available methods per type)
- `alias.fn(args)` — module-qualified call after `use ... as alias`

Dot-key access on a value that isn't a dict or struct is a compile-time error when the type is known (`dot access is only supported on dict ...`) and a runtime error otherwise. Method calls on types that don't define the method are a compile-time error.

### 11.2 Indexing

- `list[idx]` supported (negative indices supported at runtime)
- `str[idx]` supported (returns single-character string)
- `vector[idx]` supported
- `vector[mask]` where `mask` is bool vector supported
- Dict bracket indexing is not supported; use dot access or `.get()` / `.set()` methods

### 11.3 Assignment targets

Allowed:
- `x = expr`
- `d.key = expr`
- `list[i] = expr`

Disallowed/diagnosed:
- assigning through non-assignable expressions
- chained assignment (`a = b = 1`)
- struct field assignment (structs are immutable)
- string index assignment (strings are immutable)

### 11.4 Result helpers

Builtins currently available:
- `is_ok(result) -> bool`
- `is_err(result) -> bool`
- `unwrap(result) -> any` (panics on `err` or non-result)

Result construction and use:
- `ok expr` constructs a result value
- `err expr` constructs a result value
- `x: result = ok 1` is valid
- `fn handle(r: result) -> ...` is valid

### 11.5 Condition and logical operator type rules

- `if`, `elseif`, `while`, and ternary conditions accept **any type** — non-bool values are converted via truthiness (see semantics.md §3)
- `and`/`or` are **short-circuit** with Python semantics: they accept any type and return one of the two operand values (not necessarily bool)
- The unary `!` operator accepts any type and always returns `bool`
- `to_bool(expr)` is available for explicit conversion but is no longer required in conditions

### 11.6 Lowering notes (implementation-synced)

- Scalar arithmetic/comparison on scalar-tiered operands lowers to native C++ expressions. Int `+ - * %` and `/` go through overflow- and zero-checked helpers (`q_checked_add`, `q_checked_fdiv`, ...); comparisons and float `+ - *` use raw operators.
- Equality/inequality (`==`, `!=`) are included in scalar lowering when both operands are scalar-tiered.
- `if`/`elseif`/`while` conditions lower directly to native bool expressions when statically scalar-bool.
- `for i in range(...)` lowers to raw C++ range loops in codegen fast-paths.

## 12) Builtin Surface (Current)

### 12.1 Free functions

| Category | Functions |
|----------|-----------|
| I/O | `print`, `println`, `input` |
| Conversion | `len`, `to_str`, `to_int`, `to_float`, `to_bool`, `type` |
| Result | `is_ok`, `is_err`, `unwrap` |
| Math/range | `range`, `abs`, `min`, `max`, `sum`, `sqrt`, `floor`, `ceil`, `round` |
| Aggregates | `all`, `any` |
| Iteration | `enumerate` |
| Vector | `vfrom_list` (same as `list.to_vector()`) |
| File I/O | `_file_open`, `_file_read`, `_file_write`, `_file_close`, `_file_seek`, `_file_exists` |

### 12.2 Methods (by receiver type)

| Receiver | Methods |
|----------|---------|
| `str` | `.upper()`, `.lower()`, `.trim()`, `.contains(sub)`, `.startswith(s)`, `.endswith(s)`, `.replace(old, new)`, `.concat(s)`, `.split(sep)`, `.slice(start, end)` |
| `list` | `.push(item)`, `.pop()`, `.get(idx)`, `.set(idx, val)`, `.insert(idx, val)`, `.remove(idx)`, `.slice(start, end)`, `.reverse()`, `.sort()`, `.sort_by(key)`, `.concat(other)`, `.join(sep)`, `.enumerate()`, `.to_vector()` |
| `dict` | `.get(key)`, `.set(key, val)`, `.keys()`, `.values()`, `.items()` |
| `vector` | `.get(idx)`, `.fillna(val)`, `.astype(dtype)`, `.to_list()` |

## 13) Feature Status Matrix

| Feature | Status | Notes |
|---|---|---|
| Indentation blocks | Implemented | Triggered after `:` and `->` |
| `module` / `use` same-file | Implemented | `use ID` |
| `use STRING` file imports | Implemented | Relative and absolute file paths |
| Stdlib string imports (`use 'std/name'`) | Implemented | Resolved via stdlib root |
| Named functions and lambdas | Implemented | Parenthesized params required |
| Return type annotations | Implemented | `fn f(x) int -> x + 1`; compile-time check only |
| Default parameters | Implemented | `fn f(x, y = 0) -> x + y`; literals only, required-before-defaults |
| Typed params / typed var declarations | Implemented | `int`, `float`, `str`, `bool`, `any`, `list`, `dict`, `vector`, `result`, `resource`, `file_handle`, struct names (see §9) |
| Generic type expressions | Not implemented | e.g. `list[int]` unsupported |
| List literals (`list [...]`) | Implemented | Keyword required |
| Vector literals (`vector [...]`) | Implemented | 1D only |
| Dict literals (`dict {k: v}`) | Implemented | Keys are identifiers in source |
| Loop control (`break`, `continue`) | Implemented | Exits/skips nearest enclosing loop; compile error outside loops |
| Method dispatch (`value.method()`) | Implemented | String, list, dict, vector methods |
| Module-qualified call syntax (`alias.fn(...)`) | Implemented | Requires `use ... as alias` |
| Dot data access on dict | Implemented | read/write |
| Result values `ok` / `err` | Implemented | Analyzer has `ResultType` |
| `when` result patterns | Implemented | `ok x`, `err e` |
| `when` as an expression | Implemented | `size = when n: ...` |
| `extern` header and fn declarations | Implemented | QEI parser/analyzer/codegen path |
| `extern link` / `extern include_path` | Implemented | Native library linking (`ffi_v0_1_spec.md` P1) |
| `extern source` | Not implemented | Reserved (`ffi_v0_1_spec.md` P2); `QK-PARSE-001` |
| Structs (`struct`) | Implemented | Definitions, literals, constant defaults, immutable field reads (§9.1) |
| Extern fn first-class value thunks | Implemented | Free extern fns emit QValue thunks |
| Double-quoted strings | Implemented | Single and double-quoted strings are both valid |
| String interpolation (`!{expr}`) | Not implemented | Deferred from v0.1 |
| `unwrap_or`, `map_ok`, etc. | Future | Not implemented yet |
| Struct methods / impl blocks | Future | Not implemented |
| `table` type | Not implemented | Runtime representation only; see `tables_v0_1_spec.md` |
| Tensor type | Future | Not implemented |

## 14) Known Limits / Current Diagnostics

- Quoted non-path imports must use `std/...`; bare quoted names are rejected
- Dict bracket indexing is rejected by analyzer; use dot access or `.get()`/`.set()`
- String interpolation (`!{...}`) is deferred from v0.1
- Assignment is not chainable (`a = b = 1`)
- No leading-dot float literals (`.5`)
- Struct names are only recognized in the file that declares them

## 15) Source of Truth Policy

To reduce drift:
- `grammar.md` is canonical for syntax and semantic surface definitions.
- `semantics.md` is canonical for runtime behaviour, error model, and design decisions.
- `stdlib.md` is canonical for builtin surface and documented behavior contracts.
- `architecture.md` documents implementation internals (compiler pipeline, runtime structs, codegen).
- `src/core/quark/builtins/catalog.go` is the code-level source of truth for builtin names, arity, and runtime symbol mapping.

Changes to language behavior must update the relevant canonical docs in the same change. If any canonical doc conflicts with implementation, treat it as a release blocker and resolve before adding features.
