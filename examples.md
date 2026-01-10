# Quark Syntax Examples

## Factorial Example
	// Function definition
	fn fact n:
		when n:
			0 or 1: 1
			_: n * fact n - 1

	// Function call
	exp 10, 2 | fact | print

## Fibonacci Example
	// Function definition
	fn fib n:
		n if n <= 1 else fib (n - 1) + fib (n - 2)

	fib 5 | print

## Variable Declarations
	name = 'Amol'
	pi = 3.1415926535
	fruits = ['orange', 'apple', 'banana']

### With type annotations
	str.name = 'Amol'
	num.pi = 3.1415926535

## Conditionals

### If-Else
	if age < 16:
		'Can't drive'
	elseif age == 16:
		'Get a learner's permit'
	else:
		"Drive safe!" 

### When (Pattern Matching)
	when position:
		1: 'Gold Medal'
		2: 'Silver Medal'
		3: 'Bronze Medal'
		_: 'Runner Up'

## Loops	

### For Loops
	// Range-based for loop
	for i in 0..limit:
		print i

### While Loops
	while stream.hasNext:
		temp = consumer.consume stream.next
		// Do something

## Functions
### Anonymous Functions
	add = fn x, y: x + y 	// Definition
	add 2, 5 				// Call

### Normal Functions
	fn add x, y: x + y 		// Definition
	add 2, 5 				// Call

## Method Chaining using Pipes
	params = val[(val.find '(') + 1 : val.find ')']
		| split ','
		| filter c: bool c
		| map p: p.strip
		| map p: interpretParams p

## Pushing new value to a HashMap
	utils.meta.push {key: key, start: match.start, end: match.end}

## Precedence Examples

### Operator Precedence
	// Arithmetic
	result = 2 + 3 * 4          // 14, not 20 (* binds tighter)
	power = 2 ** 3 ** 2         // 512, not 64 (** is right-associative)

	// Function application vs operators
	double = fn x: x * 2
	value = double 3 + 5        // 11, not 16 (only 3 is arg to double)

	// Member access binds tightest
	length = myList[0].length   // Access index, then member

### Pipe Precedence
	// Pipe has very low precedence
	result = 5 + 3 | double     // (5 + 3) | double → double 8 → 16

	// Pipe chains left-to-right
	nums = [1, 2, 3, 4, 5]
		| filter x: x > 2
		| map y: y * 2
		| sum
	// Evaluates as: sum (map y: y * 2, (filter x: x > 2, [1,2,3,4,5]))

### Ternary Expressions
	// Ternary binds looser than most operators but tighter than pipe
	status = 'pass' if score >= 60 else 'fail'
	message = ('pass' if score >= 60 else 'fail') | capitalize

	// Nested ternary (right-associative)
	grade = 'A' if score >= 90 else 'B' if score >= 80 else 'C'

### Comma in Function Arguments
	// Comma groups arguments at low precedence
	result = max 1 + 2, 3 * 4   // max (3, 12) → 12

	// Ternary before comma
	value = someFunc (a if b else c), d  // First arg is ternary, second is d

### Range Operator
	// Range syntax for loops and slicing
	for i in 1..10:
		print i

	subset = myList[0..5]        // Slice from 0 to 5
	subset2 = myList[start..end]

### When Parentheses ARE Needed
	// Nested function calls
	result = outer (inner x, y)  // Pass result of inner to outer

	// Override precedence
	product = 2 * (3 + 4)        // Force addition first

	// Complex expressions as arguments
	value = func (x + y), (a * b)

