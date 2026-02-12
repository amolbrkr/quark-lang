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
		} else {
			// Parsing failed - advance token to avoid infinite loop
			p.nextToken()
		}
	}

	return root
}

func (p *Parser) parseStatement() *ast.TreeNode {
	switch p.curToken.Type {
	case token.MODULE:
		return p.parseModule()
	case token.USE:
		return p.parseUse()
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
	default:
		if p.curToken.Type == token.ID && p.peek(1).Type == token.COLON {
			return p.parseVarDecl()
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
				} else {
					// Parsing failed - advance token to avoid infinite loop
					p.nextToken()
					continue
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
			} else {
				// Parsing failed - advance token to avoid infinite loop
				p.nextToken()
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
		// Named function: fn name params -> body
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

		// Parse parameters
		args := p.parseParameters()

		node.AddChildren(nameNode, args)

		// Expect arrow
		if !p.expect(token.ARROW) {
			return nil
		}

		// Parse body
		body := p.parseBlock()
		node.AddChild(body)
	}

	return node
}

func (p *Parser) parseCallArguments() *ast.TreeNode {
	node := ast.NewNode(ast.ArgumentsNode, nil)

	for p.curToken.Type != token.ARROW &&
		p.curToken.Type != token.NEWLINE &&
		!p.isAtEnd() {

		// Parse at PrecTernary to stop before comma (which has lower precedence)
		// This ensures we get individual parameters, not comma expressions
		expr := p.parseExpression(ast.PrecTernary)
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

func (p *Parser) parseParameters() *ast.TreeNode {
	node := ast.NewNode(ast.ArgumentsNode, nil)

	if !p.expect(token.LPAR) {
		return node
	}

	// Allow empty parameter list: fn () ->
	if p.curToken.Type == token.RPAR {
		p.nextToken()
		return node
	}

	for {
		if p.curToken.Type != token.ID {
			p.addError("expected parameter name")
			return node
		}

		paramTok := p.curToken
		paramNode := ast.NewNode(ast.ParameterNode, &paramTok)
		nameNode := ast.NewNode(ast.IdentifierNode, &paramTok)
		paramNode.AddChild(nameNode)
		p.nextToken()

		if p.curToken.Type == token.COLON {
			p.nextToken()
			typeNode := p.parseTypeExpr()
			if typeNode != nil {
				paramNode.AddChild(typeNode)
			}
		}

		node.AddChild(paramNode)

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
		return node
	}

	return node
}

func (p *Parser) parseTypeExpr() *ast.TreeNode {
	if p.curToken.Type != token.ID && p.curToken.Type != token.LIST && p.curToken.Type != token.DICT {
		p.addError("expected type name")
		return nil
	}

	tok := p.curToken
	node := ast.NewNode(ast.TypeNode, &tok)
	p.nextToken()

	return node
}

func (p *Parser) parseVarDecl() *ast.TreeNode {
	nameTok := p.curToken
	nameNode := ast.NewNode(ast.IdentifierNode, &nameTok)
	p.nextToken()

	if !p.expect(token.COLON) {
		return nil
	}

	typeNode := p.parseTypeExpr()
	if typeNode == nil {
		return nil
	}

	if !p.expect(token.EQUALS) {
		return nil
	}

	valueNode := p.parseExpression(ast.PrecLowest)
	if valueNode == nil {
		return nil
	}

	node := ast.NewNode(ast.VarDeclNode, &nameTok)
	node.AddChildren(nameNode, typeNode, valueNode)
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
		} else {
			// Parsing failed - advance token to avoid infinite loop
			p.nextToken()
			continue
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
		switch p.curToken.Type {
		case token.OK, token.ERR:
			tok := p.curToken
			patternExpr = ast.NewNode(ast.ResultPatternNode, &tok)
			p.nextToken()
			if p.curToken.Type != token.ID && p.curToken.Type != token.UNDERSCORE {
				p.addError("expected identifier after %s in pattern", tok.Type.String())
				return nil
			}
			bindTok := p.curToken
			binding := ast.NewNode(ast.IdentifierNode, &bindTok)
			patternExpr.AddChild(binding)
			p.nextToken()
			node.AddChild(patternExpr)
			// Result patterns cannot be combined with additional OR patterns
			break
		case token.UNDERSCORE:
			// Wildcard pattern
			tok := p.curToken
			patternExpr = ast.NewNode(ast.IdentifierNode, &tok)
			p.nextToken()
			node.AddChild(patternExpr)
		default:
			// Regular expression pattern - stop before 'or'
			patternExpr = p.parseExpression(ast.PrecAnd) // Above OR precedence
			node.AddChild(patternExpr)
			if p.curToken.Type == token.OR {
				p.nextToken()
				continue
			}
			return p.finishPatternNode(node)
		}

		if p.curToken.Type == token.OR {
			p.nextToken()
			continue
		}
		break
	}

	return p.finishPatternNode(node)
}

func (p *Parser) finishPatternNode(node *ast.TreeNode) *ast.TreeNode {

	// Expect arrow
	if !p.expect(token.ARROW) {
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

// parseModule parses: module name:
//
//	<body>
func (p *Parser) parseModule() *ast.TreeNode {
	tok := p.curToken
	node := ast.NewNode(ast.ModuleNode, &tok)
	p.nextToken() // skip 'module'

	// Parse module name
	if p.curToken.Type != token.ID {
		p.addError("expected module name")
		return nil
	}
	nameTok := p.curToken
	nameNode := ast.NewNode(ast.IdentifierNode, &nameTok)
	node.AddChild(nameNode)
	p.nextToken()

	// Expect colon
	if !p.expect(token.COLON) {
		return nil
	}

	// Parse module body (indented block with functions, variables, etc.)
	body := p.parseBlock()
	node.AddChild(body)

	return node
}

// parseUse parses: use module_name
func (p *Parser) parseUse() *ast.TreeNode {
	tok := p.curToken
	node := ast.NewNode(ast.UseNode, &tok)
	p.nextToken() // skip 'use'

	// Parse module name
	if p.curToken.Type != token.ID {
		p.addError("expected module name after 'use'")
		return nil
	}
	nameTok := p.curToken
	nameNode := ast.NewNode(ast.IdentifierNode, &nameTok)
	node.AddChild(nameNode)
	p.nextToken()

	return node
}
