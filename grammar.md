# Quark Language Grammar (Revised with Precedence)

This grammar specification includes proper precedence and associativity rules. It uses extended BNF notation with precedence climbing for expressions.

## Notation
- `::=` means "is defined as"
- `|` separates alternatives
- `{ }` means zero or more repetitions
- `[ ]` means optional (zero or one)
- `( )` groups elements
- `<TOKEN>` represents terminal symbols from the lexer
- Everything else is a non-terminal

## Lexical Elements (Terminals)

### Keywords (Reserved)
```
use, module, in, and, or, if, elseif, else, for, while, when, fn, class
```

### Operators
```
+ - * / % **        (arithmetic)
< > <= >= == !=     (comparison, equality)
=                   (assignment)
! ~ &               (logical not, bitwise not, bitwise and)
|                   (pipe)
. , : .. @          (member access, comma, colon, range, at)
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
<STRING>    ::= '([^'\n]|\\')*'
<BOOL>      ::= true | false (NOT YET IMPLEMENTED)
<NULL>      ::= null (NOT YET IMPLEMENTED)
```

**Note:** Currently only single-quoted strings are supported. Double-quoted strings are planned for future implementation.

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
                |   IfStatement
                |   WhenStatement
                |   ForLoop
                |   WhileLoop
                |   ClassDef
                |   Expression
                |   <NEWLINE>
```

### Expressions (with Precedence)

Expressions use precedence climbing. Listed from lowest to highest precedence:

```
Expression      ::= Assignment

Assignment      ::= ( <ID> | MemberAccess ) "=" Assignment
                |   PipeExpr

PipeExpr        ::= Ternary { "|" PipeTarget }
PipeTarget      ::= <ID> [ Arguments ]           # Function with piped value as first arg

Ternary         ::= Comma [ "if" Comma "else" Ternary ]

Comma           ::= LogicalOr { "," LogicalOr }

LogicalOr       ::= LogicalAnd { "or" LogicalAnd }

LogicalAnd      ::= BitwiseAnd { "and" BitwiseAnd }

BitwiseAnd      ::= Equality { "&" Equality }

Equality        ::= Comparison { ( "==" | "!=" ) Comparison }

Comparison      ::= Range { ( "<" | "<=" | ">" | ">=" ) Range }

Range           ::= Additive [ ".." Additive ]

Additive        ::= Multiplicative { ( "+" | "-" ) Multiplicative }

Multiplicative  ::= Exponent { ( "*" | "/" | "%" ) Exponent }

Exponent        ::= Unary [ "**" Exponent ]       # Right-associative

Unary           ::= ( "!" | "~" | "-" ) Unary
                |   Application

Application     ::= Access { Arguments }

Access          ::= Primary { Accessor }
Accessor        ::= "." <ID>                      # Member access
                |   "[" Expression "]"            # Indexing
                |   "[" [ Expression ] ":" [ Expression ] "]"  # Slicing

Primary         ::= <ID>
                |   Literal
                |   Lambda
                |   "(" Expression ")"
                |   ListLiteral
                |   DictLiteral

Literal         ::= <INT>
                |   <FLOAT>
                |   <STRING>
                |   <BOOL>
                |   <NULL>

ListLiteral     ::= "[" [ Expression { "," Expression } ] "]"

DictLiteral     ::= "{" [ DictPair { "," DictPair } ] "}"
DictPair        ::= Expression ":" Expression
```

### Function Definitions

```
FunctionDef     ::= "fn" <ID> Parameters ":" Block
                |   <ID> "=" "fn" Parameters ":" Block

Parameters      ::= { <ID> [ "," ] }              # Space or comma separated

Arguments       ::= { Expression [ "," ] }        # For function calls

Lambda          ::= "fn" Parameters ":" Expression
```

**Note**: Lambda vs Dict disambiguation:
- `{ key: value }` → Dict (curly braces)
- `fn x: expr` → Lambda (fn keyword)
- In pipe context after `map`, `filter`, etc., allow shorthand: `x: expr` → `fn x: expr`

### Control Flow

```
IfStatement     ::= "if" Expression ":" Block
                    { "elseif" Expression ":" Block }
                    [ "else" ":" Block ]

WhenStatement   ::= "when" Expression ":" <NEWLINE> <INDENT>
                    { Pattern ":" Expression <NEWLINE> }
                    <DEDENT>

Pattern         ::= Expression { "or" Expression }
                |   "_"                           # Wildcard

ForLoop         ::= "for" <ID> "in" Expression ":" Block
                |   "for" <ID> ".." Expression ":" Block  # Range syntax

WhileLoop       ::= "while" Expression ":" Block
```

### Class Definition

```
ClassDef        ::= "class" <ID> [ "(" <ID> ")" ] ":" Block
```

### Blocks

```
Block           ::= SimpleBlock | IndentedBlock

SimpleBlock     ::= Statement                     # Single-line block

IndentedBlock   ::= <NEWLINE> <INDENT>
                    { Statement <NEWLINE> }
                    <DEDENT>
```

## Type Annotations (Optional)

```
TypedAssignment ::= Type "." <ID> "=" Expression

Type            ::= "int" | "float" | "str" | "bool" | "list" | "dict" | <ID>
```

## Precedence Table Summary

| Precedence | Operators | Associativity | Rule |
|-----------|-----------|---------------|------|
| 15 | `.` `[]` `()` | Left | Access |
| 14 | (space) | Left | Application |
| 13 | `**` | Right | Exponent |
| 12 | `!` `~` `-` (unary) | Right | Unary |
| 11 | `*` `/` `%` | Left | Multiplicative |
| 10 | `+` `-` | Left | Additive |
| 9 | `..` | None | Range |
| 8 | `<` `<=` `>` `>=` | Left | Comparison |
| 7 | `==` `!=` | Left | Equality |
| 6 | `&` | Left | BitwiseAnd |
| 5 | `and` | Left | LogicalAnd |
| 4 | `or` | Left | LogicalOr |
| 3 | `,` | Left | Comma |
| 2 | `if-else` | Right | Ternary |
| 1 | `|` | Left | Pipe |
| 0 | `=` | Right | Assignment |

## Language Philosophy: Minimal Punctuation

Quark aims to be as English-like as possible by minimizing the use of parentheses `()`, brackets `[]`, and braces `{}` unless absolutely necessary. This creates cleaner, more readable code.

### When Parentheses Are Required

1. **Grouping expressions** - When you need to override default precedence:
   ```
   result = 2 * (3 + 4)      // Force addition before multiplication
   ```

2. **Nested function calls** - When a function argument is itself a function call:
   ```
   result = outer inner x, y
   ```
   This is ambiguous! Use parens to clarify:
   ```
   result = outer (inner x, y)    // Pass result of inner(x,y) to outer
   ```

3. **Complex expressions as arguments** - When argument contains operators:
   ```
   value = func (x + y), z        // First arg is x+y, second is z
   ```

### When Parentheses Are NOT Required

1. **Simple function calls**:
   ```
   print msg           // NOT print(msg)
   add 2, 3            // NOT add(2, 3)
   obj.method x, y     // NOT obj.method(x, y)
   ```

2. **Sequential operations** - Operators naturally separate arguments:
   ```
   fact n - 1          // Parses as fact(n - 1) due to precedence
   double x * 2        // Parses as double(x) * 2
   ```

3. **In pattern matching and control flow**:
   ```
   when n:
       0 or 1: 1
       _: n * fact n - 1      // No parens needed!
   ```

## Semantic Rules

### Function Application

1. **Space Application**: `f x y` means function `f` applied to arguments `x` and `y`
2. **Comma Grouping**: `f x, y` explicitly groups `x` and `y` as separate arguments
3. **Precedence**: **Comma binds tighter than function application**
   - `f x, y` → `f(x, y)` - x and y are separate arguments
   - `f x + y` → `f(x) + y` - only x is the argument
   - `f (x + y)` → use parens to pass the sum as argument
4. **Method calls**: `obj.method x, y` → method(obj, x, y) in traditional notation

### Pipe Operator

The pipe operator passes the left-hand result as the **first argument** to the right-hand side:

```
x | f           →  f(x)
x | f y         →  f(x, y)
x | f y, z      →  f(x, y, z)
a | f | g       →  g(f(a))
```

### Lambda Syntax

**Full form**:
```
fn x, y: x + y
```

**Shorthand in context** (after `map`, `filter`, etc.):
```
map x: x * 2     →  map(fn x: x * 2)
filter p: p > 0  →  filter(fn p: p > 0)
```

**Disambiguation**:
- If colon appears after `fn` keyword → function/lambda
- If colon appears after identifier in certain contexts (map, filter, when patterns) → lambda or pattern
- If colon appears in braces `{x: y}` → dictionary

### Pattern Matching

In `when` statements:
```
when value:
    pattern1 or pattern2: result1
    _: defaultResult
```

- Patterns can use `or` for multiple matches
- `_` is wildcard matching anything
- First matching pattern wins

## Examples with Parse Trees

### Example 1: Factorial
```
fn fact n:
    when n:
        0 or 1: 1
        _: n * fact n - 1
```

Parse:
```
FunctionDef
├─ fn
├─ ID(fact)
├─ Parameters
│  └─ ID(n)
└─ Block
   └─ WhenStatement
      ├─ Expression: ID(n)
      └─ Patterns
         ├─ Pattern: (0 or 1) → Expression: 1
         └─ Pattern: _ → Expression: (n * fact(n - 1))
```

### Example 2: Pipe Chain
```
exp 10, 2 | fact | print
```

Parse:
```
PipeExpr
├─ Application: exp(10, 2)
├─ | → Application: fact(...)
└─ | → Application: print(...)

Evaluates as: print(fact(exp(10, 2)))
```

### Example 3: Inline Lambda
```
nums | filter x: x > 0 | map y: y * 2
```

Parse:
```
PipeExpr
├─ Primary: nums
├─ | → Application: filter(Lambda(x: x > 0))
└─ | → Application: map(Lambda(y: y * 2))
```

### Example 4: Method Chaining
```
val.find '(' + 1
```

Parse:
```
Additive
├─ Access
│  ├─ Primary: val
│  ├─ . → ID(find)
│  └─ Arguments: '('
└─ + → 1

Evaluates as: (val.find('(')) + 1
```

## Notes on Implementation

1. **Indentation**: Use a lexer that emits `INDENT`/`DEDENT` tokens (like Python's)
2. **Context-Sensitive**: Track when parsing inside pipes, after certain keywords
3. **Error Recovery**: Provide helpful messages for common mistakes:
   - Missing colons after `if`, `fn`, etc.
   - Mismatched indentation
   - Ambiguous function calls
4. **Precedence Climbing**: Implement expression parsing with precedence table above
