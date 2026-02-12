# Quark Language Grammar

This grammar specification includes proper precedence and associativity rules. It uses extended BNF notation with precedence climbing for expressions.

## Notation

- `::=` means "is defined as"
- `|` separates alternatives
- `{ }` means zero or more repetitions
- `[ ]` means optional (zero or one)
- `( )` groups elements
- `<TOKEN>` represents terminal symbols from the lexer
- Everything else is a non-terminal

## Language Philosophy

### Symbols and Their Meanings

| Symbol | Meaning | Used For |
|--------|---------|----------|
| `->` | "produces" / "maps to" | Function bodies, pattern results |
| `:` | "has type" / "contains" | Type annotations, block containers, dict entries |
| `\|` | "then" / "pipe to" | Data flow pipelines |

### Minimal Punctuation

Quark aims to be readable by minimizing punctuation:

- Function calls always use parentheses: `print(x)`
- Parentheses are used for grouping and for function calls; avoid extra parens elsewhere
- Indentation defines blocks

### Error Handling Philosophy

Quark uses explicit Result types with `ok`/`err` for fallible operations. Results are handled via `when` pattern matching.

## Lexical Elements (Terminals)

### Keywords (Reserved)

```
use, module, tensor, list, dict
in, and, or, not
if, elseif, else, for, while, when
fn, return
ok, err
true, false, null
```

### Operators

```
+ - * / % **        (arithmetic)
< > <= >= == !=     (comparison, equality)
=                   (assignment)
->                  (arrow: function body, pattern result)
!                   (logical not)
|                   (pipe)
. ,                 (member access, comma)
:                   (type annotation, dict entry, block start)
```

### Delimiters

```
( ) [ ] { }
'
```

### Literals

```
<INT>       ::= [0-9]+
<FLOAT>     ::= [0-9]*\.[0-9]+ | [0-9]+\.[0-9]*
<STRING>    ::= '([^'\n]|\\'|\\n|\\t|\\\\)*'
<BOOL>      ::= true | false
<NULL>      ::= null
```

### Identifiers

```
<ID>        ::= [a-zA-Z_][a-zA-Z0-9_]*
```

### Special

```
<NEWLINE>   ::= \n
<INDENT>    ::= (increase in indentation)
<DEDENT>    ::= (decrease in indentation)
<EOF>       ::= (end of file)
```

## Grammar Rules

### Program Structure

```
Program         ::= { Statement <NEWLINE> } <EOF>

Statement       ::= FunctionDef
                |   ModuleDef
                |   UseStatement
                |   IfStatement
                |   WhenStatement
                |   ForLoop
                |   WhileLoop
                |   Expression
                |   <NEWLINE>
```

## Indexing

Quark supports single-element indexing for lists and strings.

```
Accessor        ::= "." <ID>                   // Member access / dict key
                |   "[" Expression "]"          // Single index (lists/strings only)
```

### Examples

```quark
list[0]         // First element
list[-1]        // Last element
text[0]         // First character
text[-1]        // Last character
```

String indexing returns a 1-character string.

For slicing, use the `slice` builtin: `slice(list, 1, 3)`

## Dict Literals

Dicts are key-value maps backed by `std::unordered_map<std::string, QValue>`.

Dict literals always use the `dict` keyword and **identifier keys** (stored as strings internally):

```
DictLiteral     ::= "dict" "{" [ DictEntries ] "}"

DictEntries     ::= DictEntry { "," DictEntry }

DictEntry       ::= <ID> ":" Expression
```

### Dict Access

- **Static keys** use dot access: `d.key`
- **Dynamic keys** (from variables/expressions) use builtins `dget(d, k)` and `dset(d, k, v)`

```
DictAccess      ::= Expression "." <ID>                  // Read key
DictAssignment  ::= Expression "." <ID> "=" Expression   // Set key
```

### Examples

```quark
// Creation (always requires `dict` keyword)
info = dict { name: 'Alex', age: 30, active: true }
empty = dict {}

// Dot access (reads dict key)
println(info.name)         // Alex
println(info.age)          // 30

// Dot assignment (sets dict key)
info.name = 'James'
info.city = 'NYC'          // adds new key

// Properties
println(info.size)         // number of entries
println(len(info))         // same as .size

// Dict in function
fn get_name(d) -> d.name
println(get_name(info))    // James

// Dict truthiness (non-empty = true, empty = false)
if info:
    println('has entries')
```

### Dict Properties

| Property | Returns | Description |
|----------|---------|-------------|
| `.size` | int | Number of entries |
| `.length` | int | Same as `.size` |

### Dict with `len`

```quark
info = dict { a: 1, b: 2, c: 3 }
println(len(info))         // 3
```

## Error Handling with ok/err

Quark uses explicit Result types for error handling. Fallible operations return `ok` or `err` and are handled via `when` pattern matching.

### Result Values

Functions that can fail return either `ok value` or `err message`:

```
ResultExpr      ::= "ok" Expression
                |   "err" Expression
```

### Examples

```quark
fn divide(a, b) ->
    if b == 0:
        err 'Division by zero'
    else:
        ok a / b
```

### Pattern Matching on Results

Use `when` to handle both cases:

```quark
result = divide 10, 2

when result:
    ok value -> println(value)
    err msg -> println(msg)
```

### Propagating Errors

Functions propagate errors by returning them:

```quark
fn full_pipeline(path) ->
    when load_file path:
        err e -> err e
        ok content ->
            when parse content:
                err e -> err e
                ok data -> ok (transform(data))
```

## Function Definitions

Functions use `->` to separate parameters from body, and **parameter lists are always parenthesized**. Named functions are hoisted (available before their definition).

```
FunctionDef     ::= "fn" <ID> "(" [ ParamList ] ")" "->" Expression
                |   "fn" <ID> "(" [ ParamList ] ")" "->" <NEWLINE> <INDENT> Block <DEDENT>

ParamList       ::= Param { "," Param } [ "," ]

Param           ::= <ID> [ ":" Type ]
```

### Examples

```quark
// Single-line functions
fn double(x) -> x * 2
fn add(x, y) -> x + y

// With type annotations
fn add(x: int, y: int) -> x + y
fn square(n: float) -> n * n

// Multi-line function
fn factorial(n) ->
    when n:
        0 or 1 -> 1
        _ -> n * factorial(n - 1)

// Function returning result
fn safe_divide(a, b) ->
    if b == 0:
        err 'Division by zero'
    else:
        ok a / b
```

## Anonymous Functions (Lambdas)

Lambdas are expressions — they can be passed as arguments, assigned to variables, or used inline. Parameter lists are parenthesized just like named functions.

```
AnonFunction    ::= "fn" "(" [ ParamList ] ")" "->" Expression
```

### Examples

```quark
// Assign lambda to variable
double = fn(x) -> x * 2

// Pass lambda inline
data | filter(fn(x) -> x > 0)
data | map(fn(row) -> row * 2)

// With type annotations
add = fn(x: int, y: int) -> x + y
```

## Struct Definitions [FUTURE]

> **Note**: Structs and impl blocks are planned but not yet implemented.

```
StructDef       ::= "struct" <ID> ":" <NEWLINE> <INDENT>
                    { FieldDef <NEWLINE> }
                    <DEDENT>

FieldDef        ::= <ID> ":" Type [ "=" Expression ]
```

## Impl Blocks [FUTURE]

> **Note**: Impl blocks are planned but not yet implemented.

```
ImplDef         ::= "impl" <ID> ":" <NEWLINE> <INDENT>
                    { FunctionDef <NEWLINE> }
                    <DEDENT>
```

## Module Definitions

Modules group related definitions.

```
ModuleDef       ::= "module" <ID> ":" <NEWLINE> <INDENT>
                    { Statement <NEWLINE> }
                    <DEDENT>

UseStatement    ::= "use" <ID>
```

### Examples

```quark
module math:
    fn square(x) -> x * x
    fn cube(x) -> x * x * x

use math

result = square 5.0
```

## Tensor Type [FUTURE]

> **Note**: Tensor types are planned for SIMD/GPU acceleration but not yet implemented.

```
TensorDef       ::= "tensor" "[" Expression { "," Expression } "]"
                |   "tensor" "[" TensorLiteral "]"

TensorLiteral   ::= Expression { "," Expression } { ";" Expression { "," Expression } }
```

### Tensor vs List

| Feature | List | Tensor |
|---------|------|--------|
| Element types | Mixed (any QValue) | Homogeneous (float64) |
| Memory layout | Pointer array | Contiguous buffer |
| Operations | General purpose | SIMD-accelerated |
| Indexing | `list[i]` | `tensor[i]` or `tensor[i, j]` |

## Expressions (with Precedence)

Expressions use precedence climbing. Listed from lowest to highest precedence:

```
Expression      ::= Assignment

Assignment      ::= ( <ID> | MemberAccess ) "=" Assignment
                |   TypedDecl
                |   PipeExpr

TypedDecl       ::= <ID> ":" Type "=" Expression

PipeExpr        ::= Ternary { "|" Call }

Ternary         ::= LogicalOr [ "if" LogicalOr "else" Ternary ]

LogicalOr       ::= LogicalAnd { "or" LogicalAnd }

LogicalAnd      ::= Equality { "and" Equality }

Equality        ::= Comparison { ( "==" | "!=" ) Comparison }

Comparison      ::= Additive { ( "<" | "<=" | ">" | ">=" ) Additive }

Additive        ::= Multiplicative { ( "+" | "-" ) Multiplicative }

Multiplicative  ::= Exponent { ( "*" | "/" | "%" ) Exponent }

Exponent        ::= Unary [ "**" Exponent ]

Unary           ::= ( "!" | "-" ) Unary          // NO whitespace between operator and operand
                |   Call

Call            ::= Postfix

Postfix         ::= Primary { Accessor | CallArgs }

Access          ::= Primary { Accessor }            // Retained for clarity; Accessor is also used by Call

Accessor        ::= "." <ID>                      // Member access
                |   "[" Expression "]"             // Index

CallArgs        ::= "(" [ Arguments ] ")"

Primary         ::= <ID>
                |   Literal
                |   AnonFunction
                |   ResultExpr
                |   "(" Expression ")"
                |   ListLiteral
                |   DictLiteral
                |   TensorLiteral                  // [FUTURE]

Literal         ::= <INT>
                |   <FLOAT>
                |   <STRING>
                |   <BOOL>
                |   <NULL>

ResultExpr      ::= "ok" Expression
                |   "err" Expression

ListLiteral     ::= "list" "[" [ Expression { "," Expression } ] "]"

DictLiteral     ::= "dict" "{" [ DictEntries ] "}"

DictEntries     ::= DictEntry { "," DictEntry }

DictEntry       ::= <ID> ":" Expression

Arguments       ::= Expression { "," Expression }
```

## Control Flow

### If Statement

```
IfStatement     ::= "if" Expression ":" Block
                    { "elseif" Expression ":" Block }
                    [ "else" ":" Block ]
```

### Examples

```quark
if x > 10:
    println('big')
elseif x > 5:
    println('medium')
else:
    println('small')
```

### When Statement (Pattern Matching)

Pattern matching uses `->` for results.

```
WhenStatement   ::= "when" Expression ":" <NEWLINE> <INDENT>
                    { Pattern "->" Expression <NEWLINE> }
                    <DEDENT>

Pattern         ::= "ok" <ID>                     // Match ok result
                |   "err" <ID>                    // Match err result
                |   Expression { "or" Expression }
                |   "_"
```

### Examples

```quark
when status:
    200 -> 'ok'
    404 -> 'not found'
    500 or 502 or 503 -> 'server error'
    _ -> 'unknown'

when result:
    ok value -> process(value)
    err msg -> println(msg)

when n:
    0 or 1 -> 1
    _ -> n * fact (n - 1)
```

### For Loop

```
ForLoop         ::= "for" <ID> "in" Expression ":" Block
```

### Examples

```quark
for i in range(10):
    println(i)

for i in range(0, 100, 5):
    println(i)

for item in items:
    process(item)
```

### While Loop

```
WhileLoop       ::= "while" Expression ":" Block
```

### Examples

```quark
while x > 0:
    println(x)
    x = x - 1
```

## Blocks

```
Block           ::= SimpleBlock | IndentedBlock

SimpleBlock     ::= Expression

IndentedBlock   ::= <NEWLINE> <INDENT>
                    { Statement <NEWLINE> }
                    <DEDENT>
```

## Type Annotations

Type annotations are used on function parameters and typed variable declarations.

```
Type            ::= "int" | "float" | "str" | "bool"
                |   "list"
                |   "dict"
                |   <ID>

TypeAnnotation  ::= <ID> ":" Type
```

### Examples

```quark
// Function parameter types
fn add(x: int, y: int) -> x + y
fn greet(name: str) -> println(name)

// Typed variable declarations
count: int = 0
name: str = 'alice'
items: list = list [1, 2, 3]
```

## Precedence Table Summary

| Precedence | Operators | Associativity | Rule |
|------------|-----------|---------------|------|
| 13 | `.` `[]` `()` | Left | Access/Call |
| 12 | `**` | Right | Exponent |
| 11 | `!` `-` (unary) | Right | Unary |
| 10 | `*` `/` `%` | Left | Multiplicative |
| 9 | `+` `-` | Left | Additive |
| 8 | `<` `<=` `>` `>=` | Left | Comparison |
| 7 | `==` `!=` | Left | Equality |
| 6 | `and` | Left | LogicalAnd |
| 5 | `or` | Left | LogicalOr |
| 4 | `if-else` | Right | Ternary |
| 3 | `\|` | Left | Pipe |
| 2 | `->` | Right | Arrow |
| 1 | `=` | Right | Assignment |

## Semantic Rules

### Function Application

1. **Calls always use parentheses**: `f()` or `f(a, b)`
2. **Commas separate arguments inside the parens**
3. **Nested calls are straightforward**: `f(g(x), y)`
4. **Parenthesize complex arguments**: `calculate(a + b, c * d)`

```quark
// Simple calls
print(x)
add(x, y)
sqrt(16)

// Nested calls
print(double(x))
foo(bar(x, y), z)
process(transform(load(path)))

// Complex expressions as arguments
calculate(a + b, c * d)
```

### Unary Operators and Whitespace

**CRITICAL RULE**: Unary operators (`-` and `!`) MUST NOT have whitespace between the operator and operand.

```quark
// Unary negation (no space)
x = -5              // Negative five
y = !flag           // Logical not

// Function call with unary argument
f(-5)               // function called with negative five
process(!ready)     // explicit call with unary argument

// Binary subtraction (space on both sides)
a - b               // a minus b (binary subtraction)
x - 2               // x minus 2 (binary subtraction)

// Function call vs subtraction disambiguation
add(a, -b)          // second arg is negative b
fact(n - 1)         // argument is n minus 1
```

### Pipe Operator

The pipe passes the left-hand result as the **first argument** to the right call. The pipe target must be an explicit call:

```quark
x | f()               // f(x)
x | f(y)              // f(x, y)
x | f(y, z)           // f(x, y, z)
a | f() | g() | h()   // h(g(f(a)))
```

## Complete Examples

### Data Processing with Error Handling

```quark
fn load_csv(path) ->
    if !file_exists(path):
        err 'File not found'
    else:
        content = read_file(path)
        ok parse_csv(content)

fn process_data(path) ->
    when load_csv(path):
        err e ->
            println(e)
            list []
        ok data ->
            data
                | filter(fn(row) -> row > 0)
                | map(fn(row) -> row * 2)
```

### Factorial

```quark
fn fact(n) ->
    when n:
        0 or 1 -> 1
    _ -> n * fact(n - 1)

fact(10) | println()
```

### List Operations

```quark
nums = list [10, 20, 30, 40, 50]
reverse(nums)

for i in range(len(nums)):
    print(get(nums, i))
    print(' ')
println('')

s = slice(nums, 1, 3)
```

### Module with Functions

```quark
module math:
    fn square(x) -> x * x
    fn cube(x) -> x * x * x

use math

square(5) | println()
cube(3) | println()
```

### Tensor Operations [FUTURE]

```quark
// Vector operations
v1 = tensor [1.0, 2.0, 3.0]
v2 = tensor [4.0, 5.0, 6.0]
v3 = v1 + v2                    // [5.0, 7.0, 9.0]

// Matrix operations
m1 = tensor [1, 2; 3, 4]
m2 = tensor [5, 6; 7, 8]
m3 = m1 @ m2                    // Matrix multiply

// Data science workflow
data = tensor (load_floats 'data.bin')
normalized = (data - (mean data)) / (std data)
result = normalized @ weights + bias
```

## Implementation Status

| Feature | Status |
|---------|--------|
| Basic expressions (arithmetic, comparison) | Implemented |
| Variables and assignment | Implemented |
| Typed variable declarations (`name: str = val`) | Implemented |
| If/elseif/else | Implemented |
| Ternary (`a if cond else b`) | Implemented |
| While loops | Implemented |
| For loops | Implemented |
| Named functions (`fn name(params) -> body`) | Implemented |
| Anonymous functions (`fn(params) -> expr`) | Implemented |
| Pattern matching (`when`) | Implemented |
| Pipes (`\|`) | Implemented |
| Modules (`module` / `use`) | Implemented |
| Lists (`list [...]`) | Implemented |
| Indexing (`list[i]`) | Implemented |
| Member access (`.property`, `.method`) | Implemented |
| Error handling (`ok` / `err` + `when`) | Implemented |
| Type annotations on params (`x: int`) | Implemented |
| Dict literals (`dict {k: v}`) | Implemented |
| Dict member access (`d.key`) | Implemented |
| Dict member assignment (`d.key = val`) | Implemented |
| Structs / impl blocks | Future |
| Tensor type | Future |

## Notes on Implementation

1. **Indentation**: Lexer emits `INDENT`/`DEDENT` tokens (like Python)
2. **Arrow Lookahead**: When parsing parameters, look for `->` to determine if in function context
3. **Result Type**: `ok` and `err` are keywords that wrap values into a tagged union
4. **Range Function**: `range` is a builtin function, not an operator
5. **Tensor Literals**: Use `;` as row separator inside `tensor [...]`
6. **Precedence Climbing**: Use table above for expression parsing
