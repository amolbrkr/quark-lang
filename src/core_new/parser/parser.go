package parser

import (
	"fmt"
	"quark/ast"
	"quark/token"
)

type Parser struct {
	tokens   []token.Token
	pos      int
	curToken token.Token
	errors   []string
}

func New(tokens []token.Token) *Parser {
	p := &Parser{
		tokens: tokens,
		errors: make([]string, 0),
	}
	if len(tokens) > 0 {
		p.curToken = tokens[0]
	}
	return p
}

func (p *Parser) Errors() []string {
	return p.errors
}

func (p *Parser) addError(format string, args ...interface{}) {
	msg := fmt.Sprintf(format, args...)
	p.errors = append(p.errors, fmt.Sprintf("line %d: %s", p.curToken.Line, msg))
}

func (p *Parser) nextToken() {
	p.pos++
	if p.pos < len(p.tokens) {
		p.curToken = p.tokens[p.pos]
	} else {
		p.curToken = token.Token{Type: token.EOF}
	}
}

func (p *Parser) peek(offset int) token.Token {
	idx := p.pos + offset
	if idx < len(p.tokens) {
		return p.tokens[idx]
	}
	return token.Token{Type: token.EOF}
}

func (p *Parser) expect(t token.TokenType) bool {
	if p.curToken.Type == t {
		p.nextToken()
		return true
	}
	p.addError("expected %s but got %s", t, p.curToken.Type)
	return false
}

func (p *Parser) isAtEnd() bool {
	return p.curToken.Type == token.EOF
}

// Parse is the main entry point
func (p *Parser) Parse() *ast.TreeNode {
	root := ast.NewNode(ast.CompilationUnitNode, nil)

	for !p.isAtEnd() {
		if p.curToken.Type == token.NEWLINE {
			p.nextToken()
			continue
		}
		stmt := p.parseStatement()
		if stmt != nil {
			root.AddChild(stmt)
		}
	}

	return root
}

func (p *Parser) parseStatement() *ast.TreeNode {
	switch p.curToken.Type {
	case token.IF:
		return p.parseIfStatement()
	case token.WHEN:
		return p.parseWhenStatement()
	case token.FOR:
		return p.parseForLoop()
	case token.WHILE:
		return p.parseWhileLoop()
	case token.FN:
		return p.parseFunction()
	case token.AT:
		p.nextToken() // skip @
		return p.parseFunctionCall()
	default:
		// Check for anonymous function: id = fn ...
		if p.curToken.Type == token.ID && p.peek(1).Type == token.EQUALS && p.peek(2).Type == token.FN {
			return p.parseFunction()
		}
		return p.parseExpression(ast.PrecLowest)
	}
}

func (p *Parser) parseBlock() *ast.TreeNode {
	node := ast.NewNode(ast.BlockNode, nil)

	if p.curToken.Type == token.NEWLINE {
		nextTok := p.peek(1)
		if nextTok.Type == token.INDENT {
			// Indented block
			p.expect(token.NEWLINE)
			p.expect(token.INDENT)

			for p.curToken.Type != token.DEDENT && !p.isAtEnd() {
				if p.curToken.Type == token.NEWLINE {
					p.nextToken()
					continue
				}
				stmt := p.parseStatement()
				if stmt != nil {
					node.AddChild(stmt)
				}
				if p.curToken.Type == token.NEWLINE {
					p.nextToken()
				}
			}
			p.expect(token.DEDENT)
		} else {
			// Single-line block (newline but no indent)
			p.expect(token.NEWLINE)
		}
	} else {
		// Inline block (no newline)
		for p.curToken.Type != token.NEWLINE && !p.isAtEnd() {
			stmt := p.parseStatement()
			if stmt != nil {
				node.AddChild(stmt)
			}
		}
		if p.curToken.Type == token.NEWLINE {
			p.nextToken()
		}
	}

	return node
}

func (p *Parser) parseFunction() *ast.TreeNode {
	var node *ast.TreeNode

	if p.curToken.Type == token.FN {
		// Named function: fn name params: body
		tok := p.curToken
		node = ast.NewNode(ast.FunctionNode, &tok)
		p.nextToken() // skip 'fn'

		// Parse function name
		if p.curToken.Type != token.ID {
			p.addError("expected function name")
			return nil
		}
		nameTok := p.curToken
		nameNode := ast.NewNode(ast.IdentifierNode, &nameTok)
		p.nextToken()

		// Parse arguments
		args := p.parseArguments()

		node.AddChildren(nameNode, args)

		// Expect colon
		if !p.expect(token.COLON) {
			return nil
		}

		// Parse body
		body := p.parseBlock()
		node.AddChild(body)

	} else if p.curToken.Type == token.ID && p.peek(1).Type == token.EQUALS && p.peek(2).Type == token.FN {
		// Anonymous function: id = fn params: body
		nameTok := p.curToken
		nameNode := ast.NewNode(ast.IdentifierNode, &nameTok)
		p.nextToken() // skip id
		p.nextToken() // skip =

		tok := p.curToken
		node = ast.NewNode(ast.FunctionNode, &tok)
		p.nextToken() // skip 'fn'

		// Parse arguments
		args := p.parseArguments()

		node.AddChildren(nameNode, args)

		// Expect colon
		if !p.expect(token.COLON) {
			return nil
		}

		// Parse body
		body := p.parseBlock()
		node.AddChild(body)
	}

	return node
}

func (p *Parser) parseFunctionCall() *ast.TreeNode {
	node := ast.NewNode(ast.FunctionCallNode, nil)

	// Parse function name
	if p.curToken.Type != token.ID {
		p.addError("expected function name after @")
		return nil
	}
	nameTok := p.curToken
	nameNode := ast.NewNode(ast.IdentifierNode, &nameTok)
	p.nextToken()

	// Parse arguments
	args := p.parseArguments()

	node.AddChildren(nameNode, args)
	return node
}

func (p *Parser) parseArguments() *ast.TreeNode {
	node := ast.NewNode(ast.ArgumentsNode, nil)

	for p.curToken.Type != token.COLON &&
		p.curToken.Type != token.NEWLINE &&
		!p.isAtEnd() {

		expr := p.parseExpression(ast.PrecLowest)
		if expr != nil {
			node.AddChild(expr)
		}

		if p.curToken.Type == token.COMMA {
			p.nextToken()
		} else {
			break
		}
	}

	return node
}

func (p *Parser) parseIfStatement() *ast.TreeNode {
	tok := p.curToken
	node := ast.NewNode(ast.IfStatementNode, &tok)
	p.nextToken() // skip 'if'

	// Parse condition
	condition := p.parseExpression(ast.PrecLowest)
	node.AddChild(condition)

	// Expect colon
	if !p.expect(token.COLON) {
		return nil
	}

	// Parse if block
	ifBlock := p.parseBlock()
	node.AddChild(ifBlock)

	// Parse elseif/else
	for p.curToken.Type == token.ELSEIF {
		p.nextToken() // skip 'elseif'
		elseifCondition := p.parseExpression(ast.PrecLowest)

		if !p.expect(token.COLON) {
			return nil
		}

		elseifBlock := p.parseBlock()

		elseifNode := ast.NewNode(ast.IfStatementNode, nil)
		elseifNode.AddChildren(elseifCondition, elseifBlock)
		node.AddChild(elseifNode)
	}

	if p.curToken.Type == token.ELSE {
		p.nextToken() // skip 'else'
		if !p.expect(token.COLON) {
			return nil
		}
		elseBlock := p.parseBlock()
		node.AddChild(elseBlock)
	}

	return node
}

func (p *Parser) parseWhenStatement() *ast.TreeNode {
	tok := p.curToken
	node := ast.NewNode(ast.WhenStatementNode, &tok)
	p.nextToken() // skip 'when'

	// Parse expression to match against
	expr := p.parseExpression(ast.PrecLowest)
	node.AddChild(expr)

	// Expect colon
	if !p.expect(token.COLON) {
		return nil
	}
	if !p.expect(token.NEWLINE) {
		return nil
	}
	if !p.expect(token.INDENT) {
		return nil
	}

	// Parse patterns
	for p.curToken.Type != token.DEDENT && !p.isAtEnd() {
		if p.curToken.Type == token.NEWLINE {
			p.nextToken()
			continue
		}
		pattern := p.parsePattern()
		if pattern != nil {
			node.AddChild(pattern)
		}
		if p.curToken.Type == token.NEWLINE {
			p.nextToken()
		}
	}

	p.expect(token.DEDENT)
	return node
}

func (p *Parser) parsePattern() *ast.TreeNode {
	node := ast.NewNode(ast.PatternNode, nil)

	// Parse pattern expression(s) - can be multiple with 'or'
	// Parse at precedence above OR so 'or' separates patterns
	for {
		var patternExpr *ast.TreeNode
		if p.curToken.Type == token.UNDERSCORE {
			// Wildcard pattern
			tok := p.curToken
			patternExpr = ast.NewNode(ast.IdentifierNode, &tok)
			p.nextToken()
		} else {
			// Regular expression pattern - stop before 'or'
			patternExpr = p.parseExpression(ast.PrecAnd) // Above OR precedence
		}
		node.AddChild(patternExpr)

		if p.curToken.Type == token.OR {
			p.nextToken()
		} else {
			break
		}
	}

	// Expect colon
	if !p.expect(token.COLON) {
		return nil
	}

	// Parse result expression
	result := p.parseExpression(ast.PrecLowest)
	node.AddChild(result)

	return node
}

func (p *Parser) parseForLoop() *ast.TreeNode {
	tok := p.curToken
	node := ast.NewNode(ast.ForLoopNode, &tok)
	p.nextToken() // skip 'for'

	// Parse loop variable
	if p.curToken.Type != token.ID {
		p.addError("expected loop variable")
		return nil
	}
	varTok := p.curToken
	varNode := ast.NewNode(ast.IdentifierNode, &varTok)
	p.nextToken()

	// Expect 'in'
	if !p.expect(token.IN) {
		return nil
	}

	// Parse iterable expression
	iterable := p.parseExpression(ast.PrecLowest)
	node.AddChildren(varNode, iterable)

	// Expect colon
	if !p.expect(token.COLON) {
		return nil
	}

	// Parse body
	body := p.parseBlock()
	node.AddChild(body)

	return node
}

func (p *Parser) parseWhileLoop() *ast.TreeNode {
	tok := p.curToken
	node := ast.NewNode(ast.WhileLoopNode, &tok)
	p.nextToken() // skip 'while'

	// Parse condition
	condition := p.parseExpression(ast.PrecLowest)
	node.AddChild(condition)

	// Expect colon
	if !p.expect(token.COLON) {
		return nil
	}

	// Parse body
	body := p.parseBlock()
	node.AddChild(body)

	return node
}
