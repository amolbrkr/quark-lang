package parser

import (
	"quark/ast"
	"quark/token"
)

// Precedence mapping for tokens
var precedences = map[token.TokenType]ast.Precedence{
	token.EQUALS:     ast.PrecAssignment,
	token.PIPE:       ast.PrecPipe,
	token.COMMA:      ast.PrecComma,
	token.OR:         ast.PrecOr,
	token.AND:        ast.PrecAnd,
	token.AMPER:      ast.PrecBitwiseAnd,
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

func (p *Parser) peekPrecedence() ast.Precedence {
	if prec, ok := precedences[p.curToken.Type]; ok {
		return prec
	}
	return ast.PrecLowest
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

		// Check for function application (space operator)
		if p.canStartExpression(p.curToken.Type) {
			prec := p.curPrecedence()
			// Only apply function application if:
			// 1. We're at precedence that allows it
			// 2. Next token is NOT an infix operator at current or higher precedence
			infix := p.infixParseFn(p.curToken.Type)
			if precedence <= ast.PrecApplication && (infix == nil || prec < precedence) {
				left = p.parseFunctionApplication(left)
				continue
			}
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

func (p *Parser) canStartExpression(tokType token.TokenType) bool {
	switch tokType {
	case token.ID, token.INT, token.FLOAT, token.STRING,
		token.LPAR, token.LBRACKET, token.LBRACE,
		token.UNDERSCORE, token.BANG, token.NOT, token.MINUS,
		token.TRUE, token.FALSE, token.NULL, token.FN,
		token.OK, token.ERR:
		return true
	}
	return false
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
	case token.LBRACKET:
		return p.parseListLiteral
	case token.LBRACE:
		return p.parseDictLiteral
	case token.BANG, token.NOT, token.MINUS:
		return p.parseUnary
	case token.FN:
		return p.parseLambda
	case token.OK, token.ERR:
		return p.parseResultLiteral
	}
	return nil
}

// Infix parse functions

func (p *Parser) infixParseFn(t token.TokenType) func(*ast.TreeNode) *ast.TreeNode {
	switch t {
	case token.PLUS, token.MINUS, token.MULTIPLY, token.DIVIDE, token.MODULO,
		token.LT, token.LTE, token.GT, token.GTE, token.DEQ, token.NE,
		token.AND, token.OR, token.AMPER, token.EQUALS, token.COMMA:
		return p.parseBinaryOp
	case token.DOUBLESTAR:
		return p.parseExponent
	case token.PIPE:
		return p.parsePipe
	case token.DOT:
		return p.parseMemberAccess
	case token.LBRACKET:
		return p.parseIndexExpression
	}
	return nil
}

func (p *Parser) parseIdentifier() *ast.TreeNode {
	tok := p.curToken
	node := ast.NewNode(ast.IdentifierNode, &tok)
	p.nextToken()
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
	p.nextToken() // skip '('
	expr := p.parseExpression(ast.PrecLowest)
	if !p.expect(token.RPAR) {
		return nil
	}
	return expr
}

func (p *Parser) parseListLiteral() *ast.TreeNode {
	tok := p.curToken
	node := ast.NewNode(ast.ListNode, &tok)
	p.nextToken() // skip '['

	if p.curToken.Type != token.RBRACKET {
		for {
			// Parse at PrecTernary to stop before comma (which has lower precedence)
			elem := p.parseExpression(ast.PrecTernary)
			if elem != nil {
				node.AddChild(elem)
			}
			if p.curToken.Type == token.COMMA {
				p.nextToken()
			} else {
				break
			}
		}
	}

	if !p.expect(token.RBRACKET) {
		return nil
	}
	return node
}

func (p *Parser) parseDictLiteral() *ast.TreeNode {
	tok := p.curToken
	node := ast.NewNode(ast.DictNode, &tok)
	p.nextToken() // skip '{'

	if p.curToken.Type != token.RBRACE {
		for {
			// Parse key (identifier)
			if p.curToken.Type != token.ID {
				p.addError("expected identifier as dict key")
				return nil
			}
			keyTok := p.curToken
			key := ast.NewNode(ast.IdentifierNode, &keyTok)
			p.nextToken()

			// Expect colon
			if !p.expect(token.COLON) {
				return nil
			}

			// Parse value
			value := p.parseExpression(ast.PrecLowest)

			// Create key-value pair
			pair := ast.NewNode(ast.OperatorNode, nil)
			pair.AddChildren(key, value)
			node.AddChild(pair)

			if p.curToken.Type == token.COMMA {
				p.nextToken()
			} else {
				break
			}
		}
	}

	if !p.expect(token.RBRACE) {
		return nil
	}
	return node
}

func (p *Parser) parseUnary() *ast.TreeNode {
	tok := p.curToken
	node := ast.NewNode(ast.OperatorNode, &tok)
	p.nextToken()
	operand := p.parseExpression(ast.PrecUnary)
	node.AddChild(operand)
	return node
}

func (p *Parser) parseBinaryOp(left *ast.TreeNode) *ast.TreeNode {
	tok := p.curToken
	node := ast.NewNode(ast.OperatorNode, &tok)
	precedence := p.curPrecedence()
	p.nextToken()
	right := p.parseExpression(precedence + 1)
	node.AddChildren(left, right)
	return node
}

func (p *Parser) parseExponent(left *ast.TreeNode) *ast.TreeNode {
	// Right-associative: use same precedence instead of +1
	tok := p.curToken
	node := ast.NewNode(ast.OperatorNode, &tok)
	p.nextToken()
	right := p.parseExpression(ast.PrecExponent)
	node.AddChildren(left, right)
	return node
}

func (p *Parser) parsePipe(left *ast.TreeNode) *ast.TreeNode {
	tok := p.curToken
	node := ast.NewNode(ast.PipeNode, &tok)
	p.nextToken()
	right := p.parseExpression(ast.PrecPipe + 1)
	node.AddChildren(left, right)
	return node
}

func (p *Parser) parseMemberAccess(left *ast.TreeNode) *ast.TreeNode {
	tok := p.curToken
	node := ast.NewNode(ast.OperatorNode, &tok)
	p.nextToken()

	if p.curToken.Type != token.ID {
		p.addError("expected identifier after '.'")
		return nil
	}

	memberTok := p.curToken
	member := ast.NewNode(ast.IdentifierNode, &memberTok)
	p.nextToken()

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

func (p *Parser) parseFunctionApplication(funcExpr *ast.TreeNode) *ast.TreeNode {
	node := ast.NewNode(ast.FunctionCallNode, nil)
	node.AddChild(funcExpr)

	args := ast.NewNode(ast.ArgumentsNode, nil)

	// Parse first argument at Term precedence to allow arithmetic
	arg := p.parseExpression(ast.PrecTerm)
	args.AddChild(arg)

	// Handle additional comma-separated arguments
	for p.curToken.Type == token.COMMA {
		p.nextToken()
		if p.canStartExpression(p.curToken.Type) {
			arg := p.parseExpression(ast.PrecTerm)
			args.AddChild(arg)
		}
	}

	node.AddChild(args)
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

// parseLambda parses inline lambda expressions: fn x: expr or fn x, y: expr
func (p *Parser) parseLambda() *ast.TreeNode {
	tok := p.curToken
	node := ast.NewNode(ast.LambdaNode, &tok)
	p.nextToken() // skip 'fn'

	// Parse parameters (identifiers until we hit colon)
	args := ast.NewNode(ast.ArgumentsNode, nil)

	for p.curToken.Type == token.ID {
		paramTok := p.curToken
		param := ast.NewNode(ast.IdentifierNode, &paramTok)
		args.AddChild(param)
		p.nextToken()

		if p.curToken.Type == token.COMMA {
			p.nextToken() // skip comma
		} else {
			break
		}
	}

	node.AddChild(args)

	// Expect arrow
	if !p.expect(token.ARROW) {
		return nil
	}

	// Parse body expression (at lowest precedence to capture everything)
	body := p.parseExpression(ast.PrecLowest)
	node.AddChild(body)

	return node
}
