# Quark Syntax Examples

## Variable Declarations
	name = 'Amol'
	pi = 3.1415926535
	fruits = ['orange', 'apple', 'banana']

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
	for i to limit: i

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
	params = val[val.find '(' + 1 : val.find ')']
		| split ','
		| filter c: bool c 
		| map p: p.strip
		| map p: interpretParams p

## Pushing new value to a HashMap
	utils.meta.push {key, start: match.start, end: match.end}


