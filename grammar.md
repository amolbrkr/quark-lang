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
| `?` | "might be null" | Optional types, safe navigation *(future)* |
| `??` | "or else" | Null coalescing *(future)* |

### Minimal Punctuation

Quark aims to be readable by minimizing punctuation:

- Function calls use spaces: `print x` not `print(x)`
- Parentheses only for grouping and nested calls
- Indentation defines blocks (like Python)

## Lexical Elements (Terminals)

### Keywords (Reserved)

```
use, module, struct, impl, in, and, or, if, elseif, else, for, while, when, fn, null, ok, err, try
```

### Operators

```
+ - * / % **        (arithmetic)
< > <= >= == !=     (comparison, equality)
=                   (assignment)
->                  (arrow: function body, pattern result)
!                   (logical not)
|                   (pipe)
. , : .. @          (member access, comma, colon, range, at)
?                   (optional type, safe navigation) [FUTURE]
??                  (null coalescing) [FUTURE]
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

Quark supports Python-style slicing for lists and strings.

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
string[0]       // First character
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

### Slicing on Strings

Same semantics as lists, operates on characters:

```quark
text = 'Hello, World!'
text[0:5]       // 'Hello'
text[7:]        // 'World!'
text[-1]        // '!'
text[::-1]      // '!dlroW ,olleH'
```

## Null Safety [FUTURE - NOT YET IMPLEMENTED]

> **Note**: The following null safety features are planned but not yet implemented.

### Optional Types

Types that might be null are marked with `?`:

```
Type            ::= BaseType [ "?" ]
```

```quark
struct User:
    name: string
    email: string?          // Might be null
    age: int?               // Might be null

fn find_user id: int -> User?    // Might return null
```

### Null Coalescing Operator `??`

Returns left side if not null, otherwise right side:

```quark
name = user.name ?? 'Anonymous'
port = config.port ?? 8080
value = first ?? second ?? third ?? 'fallback'
```

### Safe Navigation Operator `?.`

Accesses member only if receiver is not null, otherwise returns null:

```quark
name = user?.profile?.name
name = user?.profile?.name ?? 'Anonymous'
```

### Safe Index Operator `?[]`

Returns null if list is null or index is out of bounds:

```quark
first = items?[0]
value = matrix?[row]?[col]
```

### Grammar Rules for Null Safety

```
Accessor        ::= ...
                |   "?." <ID>                   // Safe member access [FUTURE]
                |   "?[" Expression "]"         // Safe index [FUTURE]

NullCoalesce    ::= Ternary { "??" Ternary }    // [FUTURE]
```

## Error Handling with ok/err

Quark uses explicit Result types for error handling, similar to Rust but with simpler syntax.

### Result Values

Functions that can fail return either `ok value` or `err message`:

```
ResultExpr      ::= "ok" Expression
                |   "err" Expression
```

### Examples

```quark
// Function that returns a result
fn divide a, b -> Result
    if b == 0:
        err 'Division by zero'
    else:
        ok a / b

fn parse_int text -> Result
    // ... parsing logic
    if valid:
        ok number
    else:
        err 'Invalid number: {text}'

// Returning results
fn load_config path -> Result
    if !file_exists path:
        err 'File not found: {path}'
    else:
        ok read_file path
```

### Pattern Matching on Results

Use `when` to handle both cases:

```quark
result = divide 10, 2

when result:
    ok value -> println 'Result: {value}'
    err msg -> println 'Error: {msg}'

// With destructuring in functions
fn process_data path ->
    when load_config path:
        ok config -> 
            data = transform config
            ok data
        err msg -> 
            err 'Failed to load: {msg}'
```

### Try Statement

The `try` statement provides a block-based way to handle errors:

```
TryStatement    ::= "try" ":" Block
                    "err" <ID> ":" Block
```

### Examples

```quark
try:
    config = load_config 'app.json'
    data = fetch_data config.url
    process data
err e:
    println 'Operation failed: {e}'
    use_defaults

// Nested error handling
try:
    try:
        risky_operation
    err inner:
        fallback_operation
err outer:
    println 'Everything failed: {outer}'
```

### Propagating Errors

Functions can propagate errors upward by returning the error:

```quark
fn full_pipeline path -> Result
    when load_file path:
        err e -> err e              // Propagate error
        ok content ->
            when parse content:
                err e -> err e      // Propagate error
                ok data -> ok (transform data)
```

### Result Type

```
ResultType      ::= "Result" [ "[" Type "]" ]
```

```quark
fn find_user id: int -> Result[User]
    // Returns ok User or err string

fn get_count -> Result[int]
    // Returns ok int or err string
```

### Complete Error Handling Example

```quark
struct Config:
    host: string
    port: int
    timeout: int = 30

fn load_config path -> Result[Config]
    if !file_exists path:
        err 'Config file not found: {path}'
    
    content = read_file path
    
    when parse_json content:
        err e -> err 'Invalid JSON: {e}'
        ok data ->
            if !has_key data, 'host':
                err 'Missing required field: host'
            elseif !has_key data, 'port':
                err 'Missing required field: port'
            else:
                ok Config {
                    host: data['host'],
                    port: data['port'],
                    timeout: data['timeout'] ?? 30
                }

fn connect_to_server config_path -> Result
    when load_config config_path:
        err e -> err e
        ok config ->
            println 'Connecting to {config.host}:{config.port}'
            when establish_connection config.host, config.port:
                err e -> err 'Connection failed: {e}'
                ok conn -> ok conn

// Usage
try:
    conn = connect_to_server 'config.json'
    conn.send 'Hello'
err e:
    println 'Failed: {e}'
    exit 1
```

## Function Definitions

Functions use `->` to separate parameters from body.

```
FunctionDef     ::= "fn" <ID> SimpleFnDef
                |   "fn" <ID> <NEWLINE> <INDENT> MultiLineFnDef

SimpleFnDef     ::= SimpleParams "->" Expression

MultiLineFnDef  ::= { TypedParam "," <NEWLINE> }
                    [ TypedParam <NEWLINE> ]
                    "->" [ Type ] <NEWLINE>
                    { Statement <NEWLINE> }
                    <DEDENT>

SimpleParams    ::= { <ID> [ "," ] }

TypedParam      ::= <ID> [ ":" Type ] [ "=" Expression ]
```

### Examples

```quark
// Single-line function
fn double x -> x * 2
fn add x, y -> x + y
fn greet name -> 'Hello, {name}!'

// Multi-line function with types
fn calculate
    x: float,
    y: float,
    z: float = 0.0
-> float
    result = x + y
    result * z

// Function returning result
fn safe_divide a, b -> Result[float]
    if b == 0:
        err 'Division by zero'
    else:
        ok a / b
```

## Anonymous Functions

Anonymous functions can be assigned to variables:

```
AnonFunction    ::= "fn" SimpleParams "->" Expression
```

### Examples

```quark
double = fn x -> x * 2
add = fn x, y -> x + y
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
    name: string
    port: int = 8080
    debug: bool = false

struct Person:
    name: string
    age: int
    email: string?          // Optional field [FUTURE]
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
    fn distance self, other -> float
        dx = self.x - other.x
        dy = self.y - other.y
        sqrt (dx*dx + dy*dy)
    
    fn scale self, factor -> Point
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
    fn square x -> x * x
    fn cube x -> x * x * x
    
    fn clamp x, min_val, max_val ->
        max min_val, (min max_val, x)

use math

result = square 5
```

## Expressions (with Precedence)

Expressions use precedence climbing. Listed from lowest to highest precedence:

```
Expression      ::= Assignment

Assignment      ::= ( <ID> | MemberAccess ) "=" Assignment
                |   PipeExpr

PipeExpr        ::= NullCoalesce { "|" PipeTarget }
PipeTarget      ::= <ID> [ Arguments ]
                |   Access [ Arguments ]

NullCoalesce    ::= Ternary { "??" Ternary }       // [FUTURE]

Ternary         ::= LogicalOr [ "if" LogicalOr "else" Ternary ]

LogicalOr       ::= LogicalAnd { "or" LogicalAnd }

LogicalAnd      ::= Equality { "and" Equality }

Equality        ::= Comparison { ( "==" | "!=" ) Comparison }

Comparison      ::= Range { ( "<" | "<=" | ">" | ">=" ) Range }

Range           ::= Additive [ ".." Additive ]

Additive        ::= Multiplicative { ( "+" | "-" ) Multiplicative }

Multiplicative  ::= Exponent { ( "*" | "/" | "%" ) Exponent }

Exponent        ::= Unary [ "**" Exponent ]

Unary           ::= ( "!" | "-" ) Unary
                |   Application

Application     ::= Access [ Arguments ]

Access          ::= Primary { Accessor }

Accessor        ::= "." <ID>                                    // Member access  
                |   "?." <ID>                                   // Safe member access [FUTURE]
                |   "[" Expression "]"                          // Index
                |   "?[" Expression "]"                         // Safe index [FUTURE]
                |   "[" [ Expression ] ":" [ Expression ] "]"   // Slice
                |   "[" [ Expression ] ":" [ Expression ] ":" [ Expression ] "]"  // Slice with step

Primary         ::= <ID>
                |   Literal
                |   AnonFunction
                |   ResultExpr
                |   "(" Expression ")"
                |   ListLiteral
                |   DictLiteral
                |   StructLiteral

Literal         ::= <INT>
                |   <FLOAT>
                |   <STRING>
                |   <BOOL>
                |   <NULL>

ResultExpr      ::= "ok" Expression
                |   "err" Expression

ListLiteral     ::= "[" [ Expression { "," Expression } ] "]"

DictLiteral     ::= "{" [ DictPair { "," DictPair } ] "}"
DictPair        ::= Expression ":" Expression

Arguments       ::= Arg { "," Arg }
Arg             ::= Expression
                |   <ID> ":" Expression
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
                |   "null"
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
for i in 0..10:
    println i

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
Type            ::= BaseType [ "?" ]              // ? is [FUTURE]
BaseType        ::= "int" | "float" | "string" | "bool" 
                |   "list" "[" Type "]"
                |   "dict" "[" Type "," Type "]"
                |   "Result" [ "[" Type "]" ]
                |   <ID>

TypeAnnotation  ::= <ID> ":" Type
```

## Precedence Table Summary

| Precedence | Operators | Associativity | Rule |
|------------|-----------|---------------|------|
| 16 | `.` `?.` `[]` `?[]` | Left | Access |
| 15 | (space) | Left | Application |
| 14 | `**` | Right | Exponent |
| 13 | `!` `-` (unary) | Right | Unary |
| 12 | `*` `/` `%` | Left | Multiplicative |
| 11 | `+` `-` | Left | Additive |
| 10 | `..` | None | Range |
| 9 | `<` `<=` `>` `>=` | Left | Comparison |
| 8 | `==` `!=` | Left | Equality |
| 7 | `and` | Left | LogicalAnd |
| 6 | `or` | Left | LogicalOr |
| 5 | `if-else` | Right | Ternary |
| 4 | `??` | Left | NullCoalesce [FUTURE] |
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
fn load_csv path -> Result[list]
    if !file_exists path:
        err 'File not found: {path}'
    else:
        content = read_file path
        ok parse_csv content

fn process_data path ->
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

### Data Slicing

```quark
fn analyze_recent data, count ->
    recent = data[-count:]
    
    avg = (sum recent) / (len recent)
    peak = max recent
    
    { average: avg, peak: peak }

fn get_page items, page, page_size ->
    start = page * page_size
    items[start:start + page_size]
```

### String Processing

```quark
fn format_user user ->
    name = user.name
    age = user.age
    status = 'active' if user.active else 'inactive'
    
    'Name: {name}, Age: {age}, Status: {status}'

fn truncate text, max_len, suffix = '...' ->
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
    fn area self -> self.width * self.height
    
    fn perimeter self -> 2 * (self.width + self.height)
    
    fn scale self, factor -> Result[Rectangle]
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
```

### Module with Error Handling

```quark
module io:
    fn read_json path -> Result[dict]
        if !file_exists path:
            err 'File not found: {path}'
        
        content = read_file path
        
        when parse_json content:
            err e -> err 'Invalid JSON: {e}'
            ok data -> ok data
    
    fn write_json path, data -> Result
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
| Functions (`fn`, `->`) | ✓ Implemented |
| Structs and Impl | ✓ Implemented |
| Modules | ✓ Implemented |
| Pattern Matching (`when`) | ✓ Implemented |
| Pipes (`\|`) | ✓ Implemented |
| String Interpolation | Planned |
| Slicing (`[:]`) | Planned |
| Error Handling (`ok`/`err`/`try`) | Planned |
| Null Safety (`?`, `??`, `?.`, `?[]`) | Future |

## Notes on Implementation

1. **Indentation**: Lexer emits `INDENT`/`DEDENT` tokens (like Python)
2. **Arrow Lookahead**: When parsing parameters, look for `->` to determine if in function context
3. **Struct vs Dict**: `Name {` starts struct literal, bare `{` starts dict
4. **String Interpolation**: Lexer scans for `{` within strings to switch to expression mode
5. **Result Type**: `ok` and `err` are keywords that wrap values into a tagged union
6. **Error Recovery**: Helpful messages for:
   - Missing `->` in function definitions
   - Mismatched indentation
   - Ambiguous nested calls (suggest parens)
   - Unclosed string interpolation braces
7. **Precedence Climbing**: Use table above for expression parsing