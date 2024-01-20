# Quark Syntax Examples

## Example Factorial Program
	// Function definition
	fn fact n:
		when n:
			0 or 1 : 1
			_ : n * fact n - 1
	
	// Function call
	exp 10, 1 + 1 -> fact -> print

## Variable Declarations
	name = 'Amol'
	pi = 3.1415926535
	fruits = ['orange', 'apple', 'banana']

### With type annotations
	str.name = 'Amol'
	digit.pi = 3.1415926535

## Conditionals

### If-Else

	if age <= 16:
		'Can't drive'
	elseif age == 16:
		'Get a learner's permit'
	else:
		"Drive safe!" 

### When (Pattern Matching)
	when position:
		1 : 'Gold Medal'
		2 : 'Silver Medal'
		3 : 'Bronze Medal'
		_ : 'Runner Up'

## Loops	

### For Loops
	for i..limit: print i

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
		-> split ','
		-> filter c: bool c 
		-> map p: p.strip
		-> map p: interpretParams p

## Pushing new value to a HashMap
	utils.meta.push {key, start: match.start, end: match.end}


