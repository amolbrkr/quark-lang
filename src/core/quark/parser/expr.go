package parser

import (
	"strings"

	"quark/ast"
	"quark/token"
)

// Precedence mapping for tokens
var precedences = map[token.TokenType]ast.Precedence{
	token.EQUALS:     ast.PrecAssignment,
	token.PIPE:       ast.PrecPipe,
	token.OR:         ast.PrecOr,
	token.AND:        ast.PrecAnd,
	token.DEQ:        ast.PrecEquality,
	token.NE:         ast.PrecEquality,
	token.LT:         ast.PrecComparison,
	token.LTE:        ast.PrecComparison,
	token.GT:         ast.PrecComparison,
	token.GTE:        ast.PrecComparison,
	token.PLUS:       ast.PrecTerm,
	token.MINUS:      ast.PrecTerm,
	token.MULTIPLY:   ast.PrecFactor,
	token.DIVIDE:     ast.PrecFactor,
	token.MODULO:     ast.PrecFactor,
	token.DOUBLESTAR: ast.PrecExponent,
	token.DOT:        ast.PrecAccess,
	token.LBRACKET:   ast.PrecAccess,
	token.LPAR:       ast.PrecAccess,
}

func (p *Parser) curPrecedence() ast.Precedence {
	if prec, ok := precedences[p.curToken.Type]; ok {
		return prec
	}
	return ast.PrecLowest
}

// parseExpression is the main entry point for Pratt parsing
func (p *Parser) parseExpression(precedence ast.Precedence) *ast.TreeNode {
	// Handle ternary if-else specially when it starts with IF
	if p.curToken.Type == token.IF {
		return p.parseTernary()
	}

	// Get prefix handler
	prefix := p.prefixParseFn(p.curToken.Type)
	if prefix == nil {
		if p.curToken.Type == token.ILLEGAL {
			// The lexer puts its message (e.g. "unexpected indent") in the
			// literal; a single character is one it didn't recognize.
			if len(p.curToken.Literal) > 1 {
				p.addError("%s", p.curToken.Literal)
			} else {
				p.addError("unexpected character '%s'", p.curToken.Literal)
			}
			return nil
		}
		p.addError("no prefix parse function for %s", p.curToken.Type)
		return nil
	}

	left := prefix()

	// Infix parsing loop
	for !p.isAtEnd() && !p.isEndOfExpression() {
		// Skip newline if followed by pipe (line continuation)
		if p.curToken.Type == token.NEWLINE && p.peek(1).Type == token.PIPE {
			p.nextToken() // skip NEWLINE, continue with PIPE
			continue
		}

		// Check for ternary (infix if)
		if p.curToken.Type == token.IF && precedence <= ast.PrecTernary {
			left = p.parseTernaryInfix(left)
			continue
		}

		// Check precedence for normal infix
		if p.curPrecedence() < precedence {
			break
		}

		infix := p.infixParseFn(p.curToken.Type)
		if infix == nil {
			break
		}

		left = infix(left)
	}

	return left
}

func (p *Parser) isEndOfExpression() bool {
	if p.blockExprEnded {
		return true
	}
	// If at NEWLINE, check if next token continues the expression (PIPE)
	if p.curToken.Type == token.NEWLINE {
		next := p.peek(1)
		if next.Type == token.PIPE {
			return false // PIPE continues the expression on next line
		}
		return true
	}
	return p.curToken.Type == token.RPAR ||
		p.curToken.Type == token.RBRACKET ||
		p.curToken.Type == token.RBRACE ||
		p.curToken.Type == token.COLON ||
		p.curToken.Type == token.EOF
}

// Prefix parse functions

func (p *Parser) prefixParseFn(t token.TokenType) func() *ast.TreeNode {
	switch t {
	case token.ID:
		return p.parseIdentifier
	case token.INT, token.FLOAT:
		return p.parseNumber
	case token.STRING:
		return p.parseString
	case token.TRUE, token.FALSE:
		return p.parseBoolean
	case token.NULL:
		return p.parseNull
	case token.UNDERSCORE:
		return p.parseWildcard
	case token.LPAR:
		return p.parseGroupedExpression
	case token.BANG, token.MINUS:
		return p.parseUnary
	case token.FN:
		return p.parseLambda
	case token.OK, token.ERR:
		return p.parseResultLiteral
	case token.LIST:
		return p.parseListLiteral
	case token.VEC:
		return p.parseVectorLiteral
	case token.DICT:
		return p.parseDictLiteral
	case token.WHEN:
		return p.parseWhenExpression
	}
	return nil
}

// parseWhenExpression parses `when` in expression position, for example
// `y = when x:` followed by an indented block of arms.
func (p *Parser) parseWhenExpression() *ast.TreeNode {
	node := p.parseWhenStatement()
	if node != nil {
		p.blockExprEnded = true
	}
	return node
}

// Infix parse functions

func (p *Parser) infixParseFn(t token.TokenType) func(*ast.TreeNode) *ast.TreeNode {
	switch t {
	case token.PLUS, token.MINUS, token.MULTIPLY, token.DIVIDE, token.MODULO,
		token.LT, token.LTE, token.GT, token.GTE, token.DEQ, token.NE,
		token.AND, token.OR, token.EQUALS:
		return p.parseBinaryOp
	case token.DOUBLESTAR:
		return p.parseExponent
	case token.PIPE:
		return p.parsePipe
	case token.DOT:
		return p.parseMemberAccess
	case token.LBRACKET:
		return p.parseIndexExpression
	case token.LPAR:
		return p.parseCallExpression
	}
	return nil
}

func (p *Parser) parseIdentifier() *ast.TreeNode {
	tok := p.curToken

	// A qualified name: io::exists, geo::Point { ... }, formats::csv::read.
	if p.peek(1).Type == token.COLONCOLON {
		qtok, ok := p.parseQualifiedPath()
		if !ok {
			return nil
		}
		if p.curToken.Type == token.LBRACE {
			return p.parseStructLiteralBody(qtok)
		}
		return ast.NewNode(ast.QualifiedNameNode, &qtok)
	}

	// Struct literal: StructName { field: value, ... }. '{' after a name has no
	// other meaning, so this doesn't depend on where the struct is defined; the
	// checker reports a name that isn't a known struct.
	if p.peek(1).Type == token.LBRACE {
		p.nextToken() // skip struct name
		return p.parseStructLiteralBody(tok)
	}

	node := ast.NewNode(ast.IdentifierNode, &tok)
	p.nextToken()
	return node
}

// parseQualifiedPath reads `ID (:: ID)+` starting at the current ID and
// returns one token for the whole path (Literal "a::b::c", position of a).
func (p *Parser) parseQualifiedPath() (token.Token, bool) {
	tok := p.curToken
	parts := []string{tok.Literal}
	p.nextToken()
	for p.curToken.Type == token.COLONCOLON {
		p.nextToken()
		if p.curToken.Type != token.ID {
			p.addError("expected a name after '::' in '%s::'", strings.Join(parts, "::"))
			return tok, false
		}
		parts = append(parts, p.curToken.Literal)
		p.nextToken()
	}
	tok.Literal = strings.Join(parts, "::")
	return tok, true
}

// parseStructLiteralBody parses `{ field: expr, ... }` after the struct name
// tok (bare or qualified). Supports both inline (comma-separated) and
// multiline (newline-separated) forms.
func (p *Parser) parseStructLiteralBody(tok token.Token) *ast.TreeNode {
	node := ast.NewNode(ast.StructLiteralNode, &tok)

	if !p.expect(token.LBRACE) {
		return nil
	}

	p.skipNewlines()
	if p.curToken.Type != token.RBRACE {
		for {
			p.skipNewlines()
			if p.curToken.Type == token.RBRACE {
				break // trailing comma or empty after newlines
			}
			if p.curToken.Type != token.ID {
				p.addError("expected field name in struct literal")
				return nil
			}

			fieldTok := p.curToken
			fieldName := ast.NewNode(ast.IdentifierNode, &fieldTok)
			p.nextToken()

			if !p.expect(token.COLON) {
				return nil
			}

			value := p.parseExpression(ast.PrecPipe)
			if value == nil {
				p.addError("expected value after ':' in struct literal")
				return nil
			}

			// Store as a pair node: StructFieldNode with field name in Token, value as child
			pair := ast.NewNode(ast.StructFieldNode, &fieldTok)
			pair.AddChildren(fieldName, value)
			node.AddChild(pair)

			p.skipNewlines()
			if p.curToken.Type == token.COMMA {
				p.nextToken()
			} else if p.curToken.Type == token.RBRACE {
				break
			} else if p.curToken.Type == token.ID {
				// Newline-separated fields (no comma needed)
				continue
			} else {
				break
			}
		}
	}
	p.skipNewlines()

	if !p.expect(token.RBRACE) {
		return nil
	}
	return node
}

func (p *Parser) parseNumber() *ast.TreeNode {
	tok := p.curToken
	node := ast.NewNode(ast.LiteralNode, &tok)
	p.nextToken()
	return node
}

func (p *Parser) parseString() *ast.TreeNode {
	tok := p.curToken
	node := ast.NewNode(ast.LiteralNode, &tok)
	p.nextToken()
	return node
}

func (p *Parser) parseBoolean() *ast.TreeNode {
	tok := p.curToken
	node := ast.NewNode(ast.LiteralNode, &tok)
	p.nextToken()
	return node
}

func (p *Parser) parseNull() *ast.TreeNode {
	tok := p.curToken
	node := ast.NewNode(ast.LiteralNode, &tok)
	p.nextToken()
	return node
}

func (p *Parser) parseResultLiteral() *ast.TreeNode {
	tok := p.curToken
	node := ast.NewNode(ast.ResultNode, &tok)
	p.nextToken()
	value := p.parseExpression(ast.PrecAssignment)
	if value != nil {
		node.AddChild(value)
	}
	return node
}

func (p *Parser) parseWildcard() *ast.TreeNode {
	tok := p.curToken
	node := ast.NewNode(ast.IdentifierNode, &tok)
	p.nextToken()
	return node
}

func (p *Parser) parseGroupedExpression() *ast.TreeNode {
	open := p.curToken
	p.nextToken() // skip '('
	expr := p.parseExpression(ast.PrecLowest)
	if !p.expect(token.RPAR) {
		return nil
	}
	// An assignment is a statement (grammar.md §10.1). Inside parentheses it
	// would be part of a larger expression, which may run it conditionally
	// (`a and (n = f())`) and leave the variable unassigned.
	if expr != nil && expr.NodeType == ast.AssignNode {
		p.addErrorAt(&open, "an assignment can't be part of an expression; assign it on its own line first")
	}
	return expr
}

// skipNewlines skips any NEWLINE tokens (defense-in-depth for multi-line literals)
func (p *Parser) skipNewlines() {
	for p.curToken.Type == token.NEWLINE {
		p.nextToken()
	}
}

func (p *Parser) parseListLiteral() *ast.TreeNode {
	tok := p.curToken
	node := ast.NewNode(ast.ListNode, &tok)
	p.nextToken() // skip 'list'

	if !p.expect(token.LBRACKET) {
		return nil
	}

	p.skipNewlines()
	if p.curToken.Type != token.RBRACKET {
		for {
			p.skipNewlines()
			if p.curToken.Type == token.RBRACKET {
				break // trailing comma
			}
			elem := p.parseExpression(ast.PrecPipe)
			if elem != nil {
				node.AddChild(elem)
			}
			p.skipNewlines()
			if p.curToken.Type == token.COMMA {
				p.nextToken()
			} else {
				break
			}
		}
	}
	p.skipNewlines()

	if !p.expect(token.RBRACKET) {
		return nil
	}
	return node
}

func (p *Parser) parseDictLiteral() *ast.TreeNode {
	tok := p.curToken
	node := ast.NewNode(ast.DictNode, &tok)
	p.nextToken() // skip 'dict'
	if !p.expect(token.LBRACE) {
		return nil
	}

	p.skipNewlines()
	if p.curToken.Type != token.RBRACE {
		for {
			p.skipNewlines()
			if p.curToken.Type == token.RBRACE {
				break // trailing comma
			}
			if p.curToken.Type != token.ID {
				p.addError("expected identifier as dict key")
				return nil
			}
			keyTok := token.Token{
				Type:    token.STRING,
				Literal: p.curToken.Literal,
				Line:    p.curToken.Line,
				Column:  p.curToken.Column,
			}
			key := ast.NewNode(ast.LiteralNode, &keyTok)
			p.nextToken()

			if !p.expect(token.COLON) {
				return nil
			}

			value := p.parseExpression(ast.PrecPipe)
			if value == nil {
				p.addError("expected value after ':' in dict literal")
				return nil
			}

			pair := ast.NewNode(ast.PairNode, nil)
			pair.AddChildren(key, value)
			node.AddChild(pair)

			p.skipNewlines()
			if p.curToken.Type == token.COMMA {
				p.nextToken()
			} else {
				break
			}
		}
	}
	p.skipNewlines()

	if !p.expect(token.RBRACE) {
		return nil
	}
	return node
}

func (p *Parser) parseVectorLiteral() *ast.TreeNode {
	tok := p.curToken
	node := ast.NewNode(ast.VectorNode, &tok)
	p.nextToken() // skip 'vec'

	if !p.expect(token.LBRACKET) {
		return nil
	}

	p.skipNewlines()
	if p.curToken.Type != token.RBRACKET {
		for {
			p.skipNewlines()
			if p.curToken.Type == token.RBRACKET {
				break // trailing comma
			}
			// 1D vectors only in MVP: reject ';' row separators for now.
			if p.curToken.Type == token.ILLEGAL && p.curToken.Literal == ";" {
				p.addError("vec literal currently supports 1D only; ';' rows are not supported yet")
				return nil
			}

			elem := p.parseExpression(ast.PrecPipe)
			if elem != nil {
				node.AddChild(elem)
			}
			p.skipNewlines()
			if p.curToken.Type == token.COMMA {
				p.nextToken()
			} else {
				break
			}
		}
	}
	p.skipNewlines()

	if !p.expect(token.RBRACKET) {
		return nil
	}
	return node
}

// parseUnary parses `-x` and `!x`. The operand is parsed at exponent
// precedence, so `**` binds tighter than a prefix operator (`-2 ** 2` is
// `-(2 ** 2)`, as in math and Python) while `*`, `+`, comparisons etc. bind
// looser (`-a * b` is `(-a) * b`). `**`'s right operand may itself start with a
// prefix operator (`2 ** -1`).
func (p *Parser) parseUnary() *ast.TreeNode {
	tok := p.curToken
	node := ast.NewNode(ast.UnaryNode, &tok)
	p.nextToken()
	operand := p.parseExpression(ast.PrecExponent)
	node.AddChild(operand)
	return node
}

func (p *Parser) parseBinaryOp(left *ast.TreeNode) *ast.TreeNode {
	tok := p.curToken
	kind := ast.BinaryNode
	if tok.Type == token.EQUALS {
		kind = ast.AssignNode
	}
	node := ast.NewNode(kind, &tok)
	precedence := p.curPrecedence()
	p.nextToken()
	right := p.parseExpression(precedence + 1)
	node.AddChildren(left, right)
	return node
}

func (p *Parser) parseExponent(left *ast.TreeNode) *ast.TreeNode {
	// Right-associative: use same precedence instead of +1
	tok := p.curToken
	node := ast.NewNode(ast.BinaryNode, &tok)
	p.nextToken()
	right := p.parseExpression(ast.PrecExponent)
	node.AddChildren(left, right)
	return node
}

// parsePipe parses `left | f(args)` straight into the call `f(left, args)`
// (and `left | obj.m(args)` into `obj.m(left, args)`), marked Piped. No later
// stage has a pipe path.
func (p *Parser) parsePipe(left *ast.TreeNode) *ast.TreeNode {
	tok := p.curToken
	p.nextToken()
	if p.curToken.Type == token.NEWLINE || p.curToken.Type == token.EOF {
		p.addErrorAt(&tok, "a line can't end with '|'; start the next line with it instead (x on one line, then '    | f()')")
		return nil
	}
	if p.curToken.Type == token.ID && p.peek(1).Type == token.ARROW {
		if left == nil {
			return nil
		}
		return p.parseQuery(left, tok)
	}
	right := p.parseExpression(ast.PrecPipe + 1)
	if right == nil || left == nil {
		return nil
	}
	if right.NodeType != ast.FunctionCallNode || len(right.Children) < 2 || right.Children[1] == nil {
		p.addErrorAt(&tok, "pipe target must be a function call; use f(...) or obj.method(...)")
		return nil
	}
	args := right.Children[1]
	args.Children = append([]*ast.TreeNode{left}, args.Children...)
	right.Piped = true
	return right
}

// parseQuery parses a table query after `source |` (tables_v0_1_spec.md
// §4): `e ->`, then one or more steps `verb(...)` separated by `|`, on one
// line, on following lines that start with `|`, or as an indented block
// (`t | e ->` on its own line, then a step per line):
//
//	stats = staff | e ->
//	    filter(e.salary > 100_000)
//	    | summarize(n = count())
//
// Every step after `e ->` belongs to the query; to pipe its result into a
// function, put the query in parentheses.
func (p *Parser) parseQuery(source *ast.TreeNode, pipeTok token.Token) *ast.TreeNode {
	rowTok := p.curToken
	p.nextToken() // the row name
	p.nextToken() // '->'
	node := ast.NewNode(ast.QueryNode, &pipeTok)
	node.AddChildren(source, ast.NewNode(ast.IdentifierNode, &rowTok))
	block := p.curToken.Type == token.NEWLINE && p.peek(1).Type == token.INDENT
	if block {
		p.nextToken()
		p.nextToken()
	}
	for {
		step := p.parseQueryStep()
		if step == nil {
			return nil
		}
		node.AddChild(step)
		switch {
		case p.curToken.Type == token.PIPE:
			p.nextToken()
		case p.curToken.Type == token.NEWLINE && p.peek(1).Type == token.PIPE:
			p.nextToken()
			p.nextToken()
		default:
			if block {
				if p.curToken.Type == token.NEWLINE {
					p.nextToken()
				}
				if !p.expect(token.DEDENT) {
					return nil
				}
				p.blockExprEnded = true
			}
			return node
		}
		if p.curToken.Type == token.NEWLINE || p.curToken.Type == token.EOF {
			p.addError("a line can't end with '|'; start the next line with it instead")
			return nil
		}
		if p.curToken.Type == token.ID && p.peek(1).Type == token.ARROW {
			p.addError("a query names its row once ('%s ->'); write the next step as '| verb(...)'", rowTok.Literal)
			return nil
		}
	}
}

// parseQueryStep parses `verb(args)`. In mutate and summarize an argument
// may be `name = value` (a ColumnDefNode); summarize values are aggregates
// and sort arguments sort keys, so neither is resolved as a function call.
func (p *Parser) parseQueryStep() *ast.TreeNode {
	if p.curToken.Type != token.ID || p.peek(1).Type != token.LPAR {
		p.addError("expected a query step, verb(...) (filter, mutate, sort, group_by, summarize, head or tail), but got %s; to pipe a query's result into a function, put the query in parentheses", sourceText(p.curToken))
		return nil
	}
	verbTok := p.curToken
	step := ast.NewNode(ast.QueryStepNode, &verbTok)
	p.nextToken() // verb
	p.nextToken() // '('
	for p.curToken.Type != token.RPAR {
		arg := p.parseStepArg(verbTok.Literal)
		if arg == nil {
			return nil
		}
		step.AddChild(arg)
		if p.curToken.Type != token.COMMA {
			break
		}
		p.nextToken()
	}
	if !p.expect(token.RPAR) {
		return nil
	}
	return step
}

func (p *Parser) parseStepArg(verb string) *ast.TreeNode {
	if p.curToken.Type == token.ID && p.peek(1).Type == token.EQUALS {
		nameTok := p.curToken
		p.nextToken()
		p.nextToken()
		def := ast.NewNode(ast.ColumnDefNode, &nameTok)
		var value *ast.TreeNode
		if verb == "summarize" {
			value = p.parseAggregate()
		} else {
			value = p.parseExpression(ast.PrecPipe)
		}
		if value == nil {
			return nil
		}
		def.AddChild(value)
		return def
	}
	switch verb {
	case "summarize":
		// A column without a name; the checker asks for one.
		return p.parseAggregate()
	case "sort":
		key := ast.NewNode(ast.SortKeyNode, nil)
		if p.curToken.Type == token.ID && p.curToken.Literal == "desc" && p.peek(1).Type == token.LPAR {
			tok := p.curToken
			key.Token = &tok
			key.Descending = true
			p.nextToken()
			p.nextToken()
			e := p.parseExpression(ast.PrecPipe)
			if e == nil || !p.expect(token.RPAR) {
				return nil
			}
			key.AddChild(e)
			return key
		}
		e := p.parseExpression(ast.PrecPipe)
		if e == nil {
			return nil
		}
		key.Token = e.Token
		key.AddChild(e)
		return key
	}
	return p.parseExpression(ast.PrecPipe)
}

// parseAggregate parses a summarize aggregate: `agg(args)`, nothing more
// (tables D24: one aggregate of a column per summarize column).
func (p *Parser) parseAggregate() *ast.TreeNode {
	if p.curToken.Type != token.ID || p.peek(1).Type != token.LPAR {
		p.addError("a summarize column is one aggregate, name = agg(e.col): count(), count(e.col), sum, mean, min or max; got %s", sourceText(p.curToken))
		return nil
	}
	tok := p.curToken
	node := ast.NewNode(ast.AggregateNode, &tok)
	p.nextToken()
	p.nextToken()
	for p.curToken.Type != token.RPAR {
		arg := p.parseExpression(ast.PrecPipe)
		if arg == nil {
			return nil
		}
		node.AddChild(arg)
		if p.curToken.Type != token.COMMA {
			break
		}
		p.nextToken()
	}
	if !p.expect(token.RPAR) {
		return nil
	}
	if p.curToken.Type != token.COMMA && p.curToken.Type != token.RPAR {
		p.addError("a summarize column is exactly one aggregate of a column (%s(...) followed by %s); compute other values with mutate first, or in a second step", tok.Literal, sourceText(p.curToken))
		return nil
	}
	return node
}

func (p *Parser) parseMemberAccess(left *ast.TreeNode) *ast.TreeNode {
	tok := p.curToken
	node := ast.NewNode(ast.MemberNode, &tok)
	p.nextToken()

	if p.curToken.Type != token.ID {
		p.addError("expected identifier after '.'")
		return nil
	}

	memberTok := p.curToken
	member := ast.NewNode(ast.IdentifierNode, &memberTok)
	p.nextToken()

	// `shapes.Sq { ... }`: a struct from a module or imported file is named
	// with `::`.
	if p.curToken.Type == token.LBRACE && left != nil && (left.NodeType == ast.IdentifierNode || left.NodeType == ast.QualifiedNameNode) {
		p.addError("a struct from a module or imported file is written %s::%s { ... }, with '::'", left.TokenLiteral(), memberTok.Literal)
		// Skip the literal so it isn't reported again.
		for depth := 0; !p.isAtEnd(); p.nextToken() {
			if p.curToken.Type == token.LBRACE {
				depth++
			} else if p.curToken.Type == token.RBRACE {
				depth--
				if depth == 0 {
					p.nextToken()
					break
				}
			}
		}
		return nil
	}

	node.AddChildren(left, member)
	return node
}

func (p *Parser) parseIndexExpression(left *ast.TreeNode) *ast.TreeNode {
	tok := p.curToken
	node := ast.NewNode(ast.IndexNode, &tok)
	p.nextToken() // skip '['

	index := p.parseExpression(ast.PrecLowest)
	node.AddChildren(left, index)

	if !p.expect(token.RBRACKET) {
		return nil
	}
	return node
}

func (p *Parser) parseCallExpression(callee *ast.TreeNode) *ast.TreeNode {
	callTok := p.curToken
	p.nextToken() // skip '('

	args := ast.NewNode(ast.ArgumentsNode, nil)

	// Allow zero-argument calls like foo()
	if p.curToken.Type == token.RPAR {
		p.nextToken()
		node := ast.NewNode(ast.FunctionCallNode, &callTok)
		node.AddChildren(callee, args)
		return node
	}

	// Parse comma-separated arguments until ')'
	for {
		arg := p.parseExpression(ast.PrecPipe)
		if arg != nil {
			args.AddChild(arg)
		}

		if p.curToken.Type == token.COMMA {
			p.nextToken()
			// Allow trailing comma before ')'
			if p.curToken.Type == token.RPAR {
				break
			}
			continue
		}
		break
	}

	if !p.expect(token.RPAR) {
		return nil
	}

	node := ast.NewNode(ast.FunctionCallNode, &callTok)
	node.AddChildren(callee, args)
	return node
}

func (p *Parser) parseTernary() *ast.TreeNode {
	// This shouldn't normally be called - ternary is parsed as infix
	p.addError("unexpected IF at start of expression")
	return nil
}

func (p *Parser) parseTernaryInfix(valueIfTrue *ast.TreeNode) *ast.TreeNode {
	// We have: <expr> IF
	if !p.expect(token.IF) {
		return nil
	}

	condition := p.parseExpression(ast.PrecTernary + 1)

	if !p.expect(token.ELSE) {
		return nil
	}

	valueIfFalse := p.parseExpression(ast.PrecTernary)

	node := ast.NewNode(ast.TernaryNode, nil)
	node.AddChildren(condition, valueIfTrue, valueIfFalse)
	return node
}

// parseLambda parses inline lambda expressions: fn(x) -> expr or fn(x: int) int -> expr
func (p *Parser) parseLambda() *ast.TreeNode {
	tok := p.curToken
	node := ast.NewNode(ast.LambdaNode, &tok)
	p.nextToken() // skip 'fn'

	// Parse parameters
	args := p.parseParameters()

	node.AddChild(args)

	// Optional return type annotation before ->
	if p.atReturnType() {
		node.ReturnType = p.parseTypeExpr()
	}

	// Expect arrow
	if !p.expect(token.ARROW) {
		return nil
	}

	// Parse body expression (at lowest precedence to capture everything)
	body := p.parseExpression(ast.PrecLowest)
	node.AddChild(body)

	return node
}
