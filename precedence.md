# Quark Precedence and Associativity Rules

This document defines the complete precedence and associativity rules for the Quark language.

## Precedence Table

Listed from **highest** (tightest binding) to **lowest** (loosest binding):

| Level | Operators/Constructs | Associativity | Description |
|-------|---------------------|---------------|-------------|
| 14 | `()` `[]` `.` | Left | Grouping, indexing, member access |
| 13 | Function application (space) | Left | `f x y` = `(f x) y` |
| 12 | `**` | Right | Exponentiation |
| 11 | `!` `~` `-` (unary) | Right | Logical NOT, bitwise NOT, negation |
| 10 | `*` `/` `%` | Left | Multiplication, division, modulo |
| 9 | `+` `-` | Left | Addition, subtraction |
| 8 | `..` | None | Range (non-associative) |
| 7 | `<` `<=` `>` `>=` | Left | Comparison |
| 6 | `==` `!=` | Left | Equality |
| 5 | `&` | Left | Bitwise AND |
| 4 | `and` | Left | Logical AND |
| 3 | `or` | Left | Logical OR |
| 2 | `if-else` (ternary) | Right | Conditional expression |
| 1 | `,` | Left | Comma (argument separator) |
| 0 | `\|` | Left | Pipe operator |
| -1 | `=` | Right | Assignment |
| -2 | `:` (lambda/block) | Right | Function body, block start |

## Language Philosophy: Minimal Punctuation

**Quark's core principle**: Be as English-like as possible. Minimize parentheses `()`, brackets `[]`, and braces `{}` unless absolutely necessary.

### Parentheses Usage Rules

**Use parentheses ONLY when**:
1. Grouping expressions to override precedence: `2 * (3 + 4)`
2. Nested function calls: `outer (inner x, y)`
3. Complex expressions as arguments: `func (x + y), z`

**NO parentheses needed for**:
1. Simple function calls: `print msg`, `add 2, 3`
2. Method calls: `obj.method x, y`
3. Sequential operations: `fact n - 1` (parsed as `fact(n-1)`)

## Detailed Rules

### 1. Function Application (Level 13)

**Rule**: Space between expressions means function application, left-associative.

```
f x y z    ≡  ((f x) y) z     // Left-associative
```

**Key insight**: Function application binds **looser** than arithmetic operators.

```
fact n - 1     ≡  fact(n - 1)      // Arithmetic binds tighter, n-1 passed to fact
double x * 2   ≡  (double x) * 2   // Only x is argument to double
f x.method     ≡  f(x.method)      // Member access binds tightest
```

**Multi-argument functions**: Use commas to separate arguments:
```
add 2, 3           ≡  add(2, 3)       // Comma separates arguments
max 1 + 2, 5       ≡  max((1+2), 5)  // Each arg evaluated first
func x, y, z       ≡  func(x, y, z)  // Multiple args
obj.method x, y    ≡  obj.method(x, y)
```

**When you need parentheses**:
```
fact n - 1         // OK: fact(n - 1)
fact (add n, 1)    // NEED: nested function call
outer (inner x, y) // NEED: inner's result passed to outer
```

### 2. Member Access and Indexing (Level 14)

**Rule**: Dot and brackets bind tightest (except grouping parens).

```
obj.method x    ≡  (obj.method) x     // method then applied to x
arr[0] + 1      ≡  (arr[0]) + 1
x.y.z           ≡  (x.y).z            // Left-associative
```

### 3. Arithmetic Operators (Levels 9-12)

Standard math precedence:
```
2 + 3 * 4       ≡  2 + (3 * 4)        // * binds tighter than +
-x * y          ≡  (-x) * y           // Unary - binds tighter than *
2 ** 3 ** 4     ≡  2 ** (3 ** 4)      // ** is right-associative
```

### 4. Comparison and Equality (Levels 6-7)

**Rule**: Comparisons are left-associative but typically non-chaining.

```
x < y < z       ≡  (x < y) < z        // Legal but returns bool < z
x == y != z     ≡  (x == y) != z      // Same issue
```

**Recommendation**: Disallow chaining or use Python-style `x < y < z` ≡ `x < y and y < z`.

### 5. Logical Operators (Levels 3-4)

```
a or b and c    ≡  a or (b and c)     // 'and' binds tighter than 'or'
a and b or c    ≡  (a and b) or c
```

### 6. Ternary If-Else (Level 2)

**Rule**: Right-associative, very low precedence.

```
x if a else y if b else z   ≡  x if a else (y if b else z)

// But higher than pipe:
x if a else y | f           ≡  (x if a else y) | f
```

### 7. Comma (Level 1)

**Rule**: Groups function arguments, lower precedence than most operators.

```
f 1 + 2, 3 * 4    ≡  f((1+2), (3*4))
f a if b else c, d ≡  f((a if b else c), d)
```

### 8. Pipe Operator (Level 0)

**Rule**: **Lowest precedence** (except assignment), left-associative.

```
f x | g y | h   ≡  ((f x) | (g y)) | h   ≡  h((g y)((f x)))

// Wait, that's wrong! Pipe should work like:
x | f           ≡  f(x)
x | f | g       ≡  g(f(x))

// So actually:
f x | g y | h   ≡  h(g(y, f(x)))  // This is still confusing!
```

**CLARIFIED PIPE RULE**:
- Left side of `|` is evaluated completely
- Result becomes **first argument** to the right side
- Right side can be a function name or a function call with additional args

```
x | f           ≡  f x
x | f y         ≡  f x, y           // x becomes first arg
x | f y, z      ≡  f x, y, z
5 | add 3       ≡  add 5, 3
[1,2,3] | map f | filter g ≡ filter g, (map f, [1,2,3])

// From your examples:
exp 10, 2 | fact | print
≡  (exp 10, 2) | fact | print
≡  fact (exp 10, 2) | print
≡  print (fact (exp 10, 2))
```

### 9. Assignment (Level -1)

**Rule**: Lowest precedence, right-associative.

```
x = y = 5       ≡  x = (y = 5)
x = y | f       ≡  x = (y | f)
```

### 10. Colon for Lambda/Blocks (Level -2)

**Rule**: Colon introduces function bodies or blocks. Context-dependent.

**In function definition**:
```
fn add x, y: x + y
// Colon starts the block/body
```

**In inline lambda (after certain keywords or in pipe)**:
```
map f: f * 2            // f: starts lambda
filter x: x > 0
```

**Context-sensitive parsing**:
- After `fn`, `filter`, `map`, etc. → lambda syntax
- After `if`, `elif`, `else`, `when`, `for`, `while` → block start
- In dictionary literal `{key: value}` → dict syntax (not lambda)

## Special Constructs

### When Pattern Matching

```
when expr:
    pattern1: result1
    pattern2: result2
```

- Colon after `when` starts the match block (indent expected)
- Colon after each pattern introduces the result expression
- Each result is evaluated at precedence level 0 (allows pipes, etc.)

### For Loops with Ranges

```
for i..limit: body
```

- `..` is range operator (precedence 8, between arithmetic and comparison)
- Colon starts the loop body

### Lambda Syntax Resolution

**In pipe context**:
```
nums | filter x: x > 0 | map y: y * 2
```

Parse as:
```
nums | filter (fn x: x > 0) | map (fn y: y * 2)
```

**Rule**: After keywords that expect functions (`map`, `filter`, `reduce`, etc.):
- If you see `identifier :`, treat as inline lambda (shorthand for `fn`)
- Parse until lower precedence operator (like `|` or `,`)

## Ambiguity Resolution Examples

### Example 1: Function Call vs Pipe
```
f x y | g z
```

Parse as:
```
((f x) y) | (g z)    // Function application (level 13) before pipe (level 0)
= g(z, f(x, y))      // Wait, this still isn't clear!
```

Actually, with commas:
```
f x, y | g z
= (f(x, y)) | (g z)
= g(z, f(x, y))      // Pipe passes left result as FIRST arg to right
```

### Example 2: Arithmetic in Function Args
```
max 3 + 5, 2 * 4
```

Parse as:
```
max((3 + 5), (2 * 4))   // Comma is low precedence, math is higher
= max(8, 8)
```

### Example 3: Ternary with Pipe
```
x if a else y | f
```

Parse as:
```
(x if a else y) | f     // Ternary binds tighter than pipe
= f(x) if a else f(y)   // NO! That's wrong
= f(x if a else y)      // Correct
```

### Example 4: Method Chaining
```
obj.method1 x | obj2.method2 y
```

Parse as:
```
(obj.method1 x) | (obj2.method2 y)      // Member access level 14, function app 13
= (obj.method1)(x) | (obj2.method2)(y)
= (obj2.method2)(y, (obj.method1)(x))
```

### Example 5: Nested Lambdas
```
map outer: filter inner: inner > 0
```

This is ambiguous! Does `inner > 0` belong to which lambda?

**Resolution**: Lambdas are greedy - they consume until:
- End of line
- Lower precedence operator (`,` or `|`)
- Closing delimiter (`)`, `]`, `}`)

So:
```
map outer: filter inner: inner > 0
= map(lambda outer: filter(lambda inner: inner > 0))
```

But:
```
map outer: filter inner: inner > 0, default
= map(lambda outer: filter(lambda inner: inner > 0, default))
```
