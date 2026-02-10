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

### Native tensor types
Quark has native N-dimensional `tensor` types which leverages SIMD intstructions for fast data processing.

### Minimal Punctuation

Quark aims to be readable by minimizing punctuation:

- Function calls use spaces: `print x` not `print(x)`
- Parentheses only for grouping and nested calls
- Indentation defines blocks 

### Error Handling Philosophy

Quark uses explicit Result types with `ok`/`err` for all fallible operations and all Results must be explicitly handled via pattern matching or `unwrap`.

### Struct + Impl blocks

Quark provides simple syntax for `struct` data types and custom behaviour implementation using `impl` blocks.

## Lexical Elements (Terminals)

### Keywords (Reserved)

```
use, module, struct, impl, tensor, list
in, and, or, not
if, elseif, else, for, while, when
fn, return
ok, err, try, unwrap
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
<STRING>    ::= '([^'\n]|\\'|\\\{|\\n|\\t|\\\\|{Expression})*'
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
                |   StructDef
                |   ImplDef
                |   ModuleDef
                |   UseStatement
                |   IfStatement
                |   WhenStatement
                |   ForLoop
                |   WhileLoop
                |   TryStatement
                |   Expression
                |   <NEWLINE>
```

## String Literals and Interpolation

All strings use single quotes. Interpolation is triggered by `{}`:

```
StringLiteral   ::= "'" { StringChar | Interpolation } "'"
StringChar      ::= <any character except ' \ {>
                |   "\\'"                    // Escaped quote
                |   "\\{"                    // Literal brace (no interpolation)
                |   "\\n"                    // Newline
                |   "\\t"                    // Tab
                |   "\\\\"                   // Backslash
Interpolation   ::= "{" Expression "}"
```

### Examples

```quark
// Plain strings
name = 'Alice'
path = 'C:\Users\files'

// Interpolated strings
greeting = 'Hello, {name}!'
info = 'Name: {name}, Age: {age}'
calc = 'Sum: {1 + 2 + 3}'
nested = 'Result: {process (load data)}'

// Escaped braces
literal = 'Use \{x} for set notation'
```

## Slicing and Indexing

Quark supports Python-style slicing for lists, strings, and tensors.

```
Accessor        ::= "." <ID>                                    // Member access
                |   "[" Expression "]"                          // Single index
                |   "[" [ Expression ] ":" [ Expression ] "]"   // Slice
                |   "[" [ Expression ] ":" [ Expression ] ":" [ Expression ] "]"  // Slice with step
```

### Single Index

```quark
list[0]         // First element
list[-1]        // Last element
list[-2]        // Second to last
text[0]         // First character
```

### Basic Slicing `[start:end]`

Returns elements from `start` up to (but not including) `end`:

```quark
list[1:4]       // Elements at index 1, 2, 3
list[0:3]       // First three elements
list[:3]        // First three (start defaults to 0)
list[3:]        // From index 3 to end
list[:]         // Copy of entire list
list[-3:]       // Last three elements
list[:-1]       // All except last
```

### Slicing with Step `[start:end:step]`

```quark
list[::2]       // Every second element
list[1::2]      // Every second element, starting at index 1
list[::-1]      // Reversed
list[10:0:-1]   // Reverse slice from 10 down to 1
```


## Error Handling with ok/err

Quark uses explicit Result types for error handling. All fallible operations must be explicitly handled.

### Result Values

Functions that can fail return either `ok value` or `err message`:

```
ResultExpr      ::= "ok" Expression
                |   "err" Expression
```

### Examples

```quark
fn divide a, b -> Result
    if b == 0:
        err 'Division by zero'
    else:
        ok a / b

fn parse_int text -> Result
    if valid:
        ok number
    else:
        err 'Invalid number: {text}'
```

### Pattern Matching on Results

Use `when` to handle both cases:

```quark
result = divide 10, 2

when result:
    ok value -> println 'Result: {value}'
    err msg -> println 'Error: {msg}'
```

### Unwrap Keyword

The `unwrap` keyword extracts the value from a Result, with a required default for the error case:

```
UnwrapExpr      ::= "unwrap" Expression "," Expression
```

```quark
// unwrap result, default_value
value = unwrap divide 10, 2, 0          // Returns 5
value = unwrap divide 10, 0, -1         // Returns -1 (division failed)

user = unwrap find_user id, default_user
port = unwrap parse_int port_str, 8080
```

The second argument is always required—this forces explicit handling of the error case. If you want to crash on error, use pattern matching and call a panic function explicitly.

### Try Statement

The `try` statement provides block-based error handling:

```
TryStatement    ::= "try" ":" Block
                    "err" <ID> ":" Block
```

```quark
try:
    config = load_config 'app.json'
    data = fetch_data config.url
    process data
err e:
    println 'Operation failed: {e}'
    use_defaults
```

### Propagating Errors

Functions propagate errors by returning them:

```quark
fn full_pipeline path -> Result
    when load_file path:
        err e -> err e
        ok content ->
            when parse content:
                err e -> err e
                ok data -> ok (transform data)
```

### Result Type

```
ResultType      ::= "Result" [ "[" Type "]" ]
```

```quark
fn find_user id: int -> Result[User]
fn get_count -> Result[int]
fn save_file path: str, data: str -> Result
```

## Function Definitions

Functions use `->` to separate parameters from body.

```
FunctionDef     ::= "fn" <ID> SimpleFnDef
                |   "fn" <ID> <NEWLINE> <INDENT> MultiLineFnDef

SimpleFnDef     ::= TypedParams "->" Expression

MultiLineFnDef  ::= { TypedParam "," <NEWLINE> }
                    [ TypedParam <NEWLINE> ]
                    "->" [ Type ] <NEWLINE>
                    { Statement <NEWLINE> }
                    <DEDENT>

TypedParams     ::= { TypedParam [ "," ] }

TypedParam      ::= <ID> [ ":" Type ] [ "=" Expression ]
```

### Examples

```quark
// Single-line functions
fn double x -> x * 2
fn add x, y -> x + y
fn greet name -> 'Hello, {name}!'

// Single-line with type annotations
fn add x: int, y: int -> x + y
fn square n: float -> n * n
fn format_user name: str, age: int -> 'Name: {name}, Age: {age}'

// Multi-line function with types
fn calculate
    x: float,
    y: float,
    z: float = 0.0
-> float
    result = x + y
    result * z

// Function returning result
fn safe_divide a: float, b: float -> Result[float]
    if b == 0:
        err 'Division by zero'
    else:
        ok a / b
```

## Anonymous Functions

Anonymous functions can be assigned to variables:

```
AnonFunction    ::= "fn" TypedParams "->" Expression
```

### Examples

```quark
double = fn x -> x * 2
add = fn x: int, y: int -> x + y
safe_get = fn list, idx, default -> unwrap list[idx], default
```

## Struct Definitions

Structs define data structures with typed fields.

```
StructDef       ::= "struct" <ID> ":" <NEWLINE> <INDENT>
                    { FieldDef <NEWLINE> }
                    <DEDENT>

FieldDef        ::= <ID> ":" Type [ "=" Expression ]
```

### Examples

```quark
struct Point:
    x: float
    y: float

struct Config:
    name: str
    port: int = 8080
    debug: bool = false

struct Person:
    name: str
    age: int
    email: str
```

## Struct Literals

```
StructLiteral   ::= <ID> "{" [ FieldInit { "," FieldInit } ] "}"
FieldInit       ::= <ID> ":" Expression
```

### Examples

```quark
// Single line
p = Point { x: 1.0, y: 2.0 }

// Multi-line
config = Config {
    name: 'myapp',
    port: 3000,
    debug: true
}
```

## Impl Blocks

Impl blocks attach methods to structs.

```
ImplDef         ::= "impl" <ID> ":" <NEWLINE> <INDENT>
                    { FunctionDef <NEWLINE> }
                    <DEDENT>
```

### Examples

```quark
impl Point:
    fn distance self, other: Point -> float
        dx = self.x - other.x
        dy = self.y - other.y
        sqrt (dx*dx + dy*dy)
    
    fn scale self, factor: float -> Point
        Point { x: self.x * factor, y: self.y * factor }
    
    fn origin -> Point
        Point { x: 0.0, y: 0.0 }
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
    fn square x: float -> x * x
    fn cube x: float -> x * x * x
    
    fn clamp x: float, min_val: float, max_val: float -> float
        if x < min_val:
            min_val
        elseif x > max_val:
            max_val
        else:
            x

use math

result = square 5.0
```

## Tensor Type [FUTURE - NOT YET IMPLEMENTED]

> **Note**: Tensor types are planned for SIMD/GPU acceleration but not yet implemented.

### Tensor Declaration

```
TensorDef       ::= "tensor" "[" Expression { "," Expression } "]"
                |   "tensor" "[" TensorLiteral "]"

TensorLiteral   ::= Expression { "," Expression } { ";" Expression { "," Expression } }
```

### 1D Tensor (Vector)

Use `tensor` keyword to create 1D tensors:

```quark
// 1D tensor (vector)
vec = tensor [1.0, 2.0, 3.0, 4.0]

// From range
indices = tensor (range 100)

// Operations are SIMD-accelerated
result = vec * 2.0          // Element-wise multiply
dot_prod = dot vec1, vec2   // Dot product
```

### 2D Tensor (Matrix)

Use `;` to separate rows (MATLAB-style):

```quark
// 2D tensor (matrix) - semicolon separates rows
matrix = tensor [1, 2, 3; 4, 5, 6; 7, 8, 9]

// Identity matrix
eye = tensor [1, 0, 0; 0, 1, 0; 0, 0, 1]

// Matrix operations
result = matrix @ other     // Matrix multiplication
transposed = transpose matrix
```

### ND Tensor

Higher-dimensional tensors via constructor functions:

```quark
// 3D tensor
cube = zeros 10, 10, 10
cube = ones 5, 5, 5

// From nested structure
t3d = tensor_3d [
    [[1, 2], [3, 4]],
    [[5, 6], [7, 8]]
]
```

### Tensor vs List

| Feature | List | Tensor |
|---------|------|--------|
| Element types | Mixed (any QValue) | Homogeneous (float64) |
| Memory layout | Pointer array | Contiguous buffer |
| Operations | General purpose | SIMD-accelerated |
| Indexing | `list[i]` | `tensor[i]` or `tensor[i, j]` |

```quark
// List - general purpose, mixed types
items = [1, 'hello', true, [1, 2, 3]]

// Tensor - numeric, SIMD operations
data = tensor [1.0, 2.0, 3.0, 4.0]
result = data * 2.0 + 1.0   // Vectorized
```

## Expressions (with Precedence)

Expressions use precedence climbing. Listed from lowest to highest precedence:

```
Expression      ::= Assignment

Assignment      ::= ( <ID> | MemberAccess ) "=" Assignment
                |   PipeExpr

PipeExpr        ::= Ternary { "|" PipeTarget }
PipeTarget      ::= <ID> [ Arguments ]
                |   Access [ Arguments ]

Ternary         ::= LogicalOr [ "if" LogicalOr "else" Ternary ]

LogicalOr       ::= LogicalAnd { "or" LogicalAnd }

LogicalAnd      ::= Equality { "and" Equality }

Equality        ::= Comparison { ( "==" | "!=" ) Comparison }

Comparison      ::= Additive { ( "<" | "<=" | ">" | ">=" ) Additive }

Additive        ::= Multiplicative { ( "+" | "-" ) Multiplicative }

Multiplicative  ::= Exponent { ( "*" | "/" | "%" ) Exponent }

Exponent        ::= Unary [ "**" Exponent ]

Unary           ::= ( "!" | "-" ) Unary          // NO whitespace between operator and operand
                |   Application

Application     ::= Access [ Arguments ]

Access          ::= Primary { Accessor }

Accessor        ::= "." <ID>                                    // Member access
                |   "[" Expression "]"                          // Index
                |   "[" [ Expression ] ":" [ Expression ] "]"   // Slice
                |   "[" [ Expression ] ":" [ Expression ] ":" [ Expression ] "]"  // Slice with step

Primary         ::= <ID>
                |   Literal
                |   AnonFunction
                |   ResultExpr
                |   UnwrapExpr
                |   "(" Expression ")"
                |   ListLiteral
                |   DictLiteral
                |   StructLiteral
                |   TensorLiteral                               // [FUTURE]

Literal         ::= <INT>
                |   <FLOAT>
                |   <STRING>
                |   <BOOL>
                |   <NULL>

ResultExpr      ::= "ok" Expression
                |   "err" Expression

UnwrapExpr      ::= "unwrap" Expression "," Expression

ListLiteral     ::= "list" "[" [ Expression { "," Expression } ] "]"

DictLiteral     ::= "{" [ DictPair { "," DictPair } ] "}"
DictPair        ::= Expression ":" Expression

Arguments       ::= Arg { "," Arg }
Arg             ::= Expression
                |   <ID> ":" Expression                         // Named argument
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
    println 'big'
elseif x > 5:
    println 'medium'
else:
    println 'small'
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
    ok value -> process value
    err msg -> println 'Error: {msg}'

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
for i in range 10:
    println i

for i in range 0, 100, 5:
    println 'Step: {i}'

for item in items:
    process item

for char in text[0:10]:
    println char
```

### While Loop

```
WhileLoop       ::= "while" Expression ":" Block
```

### Examples

```quark
while x > 0:
    println x
    x = x - 1
```

### Try Statement

```
TryStatement    ::= "try" ":" Block
                    "err" <ID> ":" Block
```

### Examples

```quark
try:
    data = load_file path
    process data
err e:
    println 'Failed: {e}'
    use_fallback
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

```
Type            ::= BaseType
BaseType        ::= "int" | "float" | "str" | "bool"
                |   "list" [ "[" Type "]" ]
                |   "dict" [ "[" Type "," Type "]" ]
                |   "tensor" [ "[" Type "]" ]           // [FUTURE]
                |   "Result" [ "[" Type "]" ]
                |   <ID>

TypeAnnotation  ::= <ID> ":" Type
```

## Precedence Table Summary

| Precedence | Operators | Associativity | Rule |
|------------|-----------|---------------|------|
| 14 | `.` `[]` | Left | Access |
| 13 | (space) | Left | Application |
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

1. **Space separates function from arguments**: `f x y` means `f(x, y)`
2. **Comma groups arguments**: `f x, y, z` means `f(x, y, z)`
3. **Parentheses required for nested calls**: `f (g x), y` means `f(g(x), y)`
4. **Parentheses required for complex expressions**: `f (x + y)` passes sum to f

```quark
// Simple calls - no parens
print x
add x, y
sqrt 16

// Nested calls - parens required
print (double x)
foo (bar x, y), z
process (transform (load path))

// Complex expressions - parens required
calculate (a + b), (c * d)
```

### Unary Operators and Whitespace

**CRITICAL RULE**: Unary operators (`-` and `!`) MUST NOT have whitespace between the operator and operand.

```quark
// Unary negation (no space)
x = -5              // Negative five
y = !flag           // Logical not

// Function call with unary argument
f -5                // f(-5) - function called with negative five
process !ready      // process(!ready)

// Binary subtraction (space on both sides)
a - b               // a minus b (binary subtraction)
x - 2               // x minus 2 (binary subtraction)

// Function call vs subtraction disambiguation
add a, -b           // add(a, -b) - second arg is negative b
add a, b - c        // add(a, b-c) - second arg is b minus c
fact n - 1          // fact(n-1) - argument is n minus 1
fact n -1           // INVALID - would be fact(n, -1) but mixing space/no-space
```

**Lexer Rule**: The lexer disambiguates based on whitespace:
- `-value` (no space) → Unary minus token
- `a - b` (spaces) → Binary minus operator
- `a -b` (space before, none after) → Binary minus operator followed by unary minus (INVALID - parse error expected)

**Parser Behavior**:
- In expression context, `-` with no following whitespace is always unary
- In expression context, `-` with preceding whitespace is always binary
- The parser enforces this during expression parsing to avoid ambiguity

### Pipe Operator

The pipe passes the left-hand result as the **first argument** to the right:

```quark
x | f             // f(x)
x | f y           // f(x, y)
x | f y, z        // f(x, y, z)
a | f | g | h     // h(g(f(a)))
```

### Method Calls

Methods are called with dot notation. `self` is the receiver:

```quark
point.distance other       // Point.distance(point, other)
point.scale 2.0            // Point.scale(point, 2.0)
```

### Named Arguments

Use colon for named arguments in calls:

```quark
connect host, port: 8080, timeout: 30
create_user 'alice', admin: true, active: true
```

## Complete Examples

### Data Processing with Error Handling

```quark
fn load_csv path: str -> Result[list]
    if !file_exists path:
        err 'File not found: {path}'
    else:
        content = read_file path
        ok parse_csv content

fn process_data path: str -> list
    when load_csv path:
        err e -> 
            println 'Error: {e}'
            []
        ok data ->
            data
                | filter (fn row -> row['value'] > 0)
                | map (fn row -> row['value'] * 2)

// With try statement
try:
    data = load_csv 'data.csv'
    results = data | map transform | filter valid
    save_csv results, 'output.csv'
err e:
    println 'Pipeline failed: {e}'
```

### Using Unwrap

```quark
fn get_config_value key: str, default: str -> str
    config = unwrap load_config 'app.json', {}
    unwrap config[key], default

// Chained unwraps
fn process_user_data user_id: int -> str
    user = unwrap find_user user_id, default_user
    profile = unwrap user.profile, default_profile
    unwrap profile.display_name, 'Anonymous'
```

### Data Slicing

```quark
fn analyze_recent data: list, count: int -> dict
    recent = data[-count:]
    
    avg = (sum recent) / (len recent)
    peak = max recent
    
    { average: avg, peak: peak }

fn get_page items: list, page: int, page_size: int -> list
    start = page * page_size
    items[start:start + page_size]
```

### String Processing

```quark
fn format_user user: User -> str
    name = user.name
    age = user.age
    status = 'active' if user.active else 'inactive'
    
    'Name: {name}, Age: {age}, Status: {status}'

fn truncate text: str, max_len: int, suffix: str = '...' -> str
    if (len text) <= max_len:
        text
    else:
        '{text[0:max_len - (len suffix)]}{suffix}'
```

### Struct with Methods and Error Handling

```quark
struct Rectangle:
    width: float
    height: float

impl Rectangle:
    fn area self -> float
        self.width * self.height
    
    fn perimeter self -> float
        2 * (self.width + self.height)
    
    fn scale self, factor: float -> Result[Rectangle]
        if factor <= 0:
            err 'Scale factor must be positive'
        else:
            ok Rectangle {
                width: self.width * factor,
                height: self.height * factor
            }

rect = Rectangle { width: 10.0, height: 5.0 }
println (rect.area)

when rect.scale 2.0:
    ok scaled -> println 'New area: {scaled.area}'
    err msg -> println 'Error: {msg}'

// Using unwrap with default
scaled = unwrap rect.scale 2.0, rect   // Returns scaled or original on error
```

### Tensor Operations [FUTURE]

```quark
// Vector operations
v1 = tensor [1.0, 2.0, 3.0]
v2 = tensor [4.0, 5.0, 6.0]
v3 = v1 + v2                    // [5.0, 7.0, 9.0]
v4 = v1 * 2.0                   // [2.0, 4.0, 6.0]
d = dot v1, v2                  // 32.0

// Matrix operations
m1 = tensor [1, 2; 3, 4]
m2 = tensor [5, 6; 7, 8]
m3 = m1 @ m2                    // Matrix multiply
m4 = transpose m1

// Data science workflow
data = tensor (load_floats 'data.bin')
normalized = (data - (mean data)) / (std data)
result = normalized @ weights + bias
```

### Module with Error Handling

```quark
module io:
    fn read_json path: str -> Result[dict]
        if !file_exists path:
            err 'File not found: {path}'
        
        content = read_file path
        
        when parse_json content:
            err e -> err 'Invalid JSON: {e}'
            ok data -> ok data
    
    fn write_json path: str, data: dict -> Result
        content = to_json data
        when write_file path, content:
            err e -> err 'Write failed: {e}'
            ok _ -> ok null

use io

try:
    config = read_json 'config.json'
    config['updated'] = true
    write_json 'config.json', config
err e:
    println 'Config error: {e}'
```

## Implementation Status

| Feature | Status |
|---------|--------|
| Basic expressions (arithmetic, comparison) | ✓ Implemented |
| Variables and assignment | ✓ Implemented |
| If/elseif/else | ✓ Implemented |
| While loops | ✓ Implemented |
| For loops | ✓ Implemented |
| Functions (`fn`, `->`) | Partially implemented (updating syntax) |
| Pattern matching (`when`) | Partially implemented (updating syntax) |
| Pipes (`\|`) | ✓ Implemented |
| Modules | ✓ Implemented |
| Structs | Not implemented |
| Impl blocks | Not implemented |
| String interpolation | Not implemented |
| Slicing (`[:]`) | Not implemented |
| Error handling (`ok`/`err`/`try`/`unwrap`) | Not implemented |
| Tensor type | Future |

## Notes on Implementation

1. **Indentation**: Lexer emits `INDENT`/`DEDENT` tokens (like Python)
2. **Arrow Lookahead**: When parsing parameters, look for `->` to determine if in function context
3. **Struct vs Dict**: `Name {` starts struct literal, bare `{` starts dict
4. **String Interpolation**: Lexer scans for `{` within strings to switch to expression mode
5. **Result Type**: `ok` and `err` are keywords that wrap values into a tagged union
6. **Unwrap**: Always requires a default value—no implicit panicking
7. **Range Function**: `range` is a builtin function, not an operator
8. **Tensor Literals**: Use `;` as row separator inside `tensor [...]`
9. **Error Recovery**: Helpful messages for:
   - Missing `->` in function definitions
   - Mismatched indentation
   - Ambiguous nested calls (suggest parens)
   - Unclosed string interpolation braces
   - Missing default in `unwrap`
10. **Precedence Climbing**: Use table above for expression parsing