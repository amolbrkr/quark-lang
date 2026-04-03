package parser

import (
	"fmt"
	"quark/ast"
	"quark/diagnostics"
	"quark/token"
)

const maxParserErrors = 10

type Parser struct {
	tokens      []token.Token
	pos         int
	curToken    token.Token
	errors      []diagnostics.Diagnostic
	structNames map[string]bool // struct names collected in pre-pass for literal parsing
}

func New(tokens []token.Token) *Parser {
	p := &Parser{
		tokens:      tokens,
		errors:      make([]diagnostics.Diagnostic, 0),
		structNames: make(map[string]bool),
	}
	if len(tokens) > 0 {
		p.curToken = tokens[0]
	}
	p.collectStructNames()
	return p
}

// collectStructNames does a quick scan of the token stream to find all
// `struct <ID> :` patterns so the expression parser can recognize struct literals.
func (p *Parser) collectStructNames() {
	for i := 0; i+2 < len(p.tokens); i++ {
		if p.tokens[i].Type == token.STRUCT &&
			p.tokens[i+1].Type == token.ID &&
			p.tokens[i+2].Type == token.COLON {
			p.structNames[p.tokens[i+1].Literal] = true
		}
	}
}

func (p *Parser) Errors() []string {
	out := make([]string, 0, len(p.errors))
	for _, d := range p.errors {
		out = append(out, d.String())
	}
	return out
}

func (p *Parser) Diagnostics() []diagnostics.Diagnostic {
	out := make([]diagnostics.Diagnostic, len(p.errors))
	copy(out, p.errors)
	return out
}

func (p *Parser) addError(format string, args ...interface{}) {
	msg := fmt.Sprintf(format, args...)
	loc := &diagnostics.Location{Line: p.curToken.Line, Column: p.curToken.Column}
	p.errors = append(p.errors, diagnostics.Diagnostic{
		Code:     "QK-PARSE-001",
		Stage:    diagnostics.StageParse,
		Severity: diagnostics.SeverityError,
		Message:  msg,
		Location: loc,
	})
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

// synchronize skips tokens until a statement boundary (NEWLINE, DEDENT, or EOF).
// Called after a parse error to recover and continue parsing subsequent statements.
func (p *Parser) synchronize() {
	for !p.isAtEnd() {
		if p.curToken.Type == token.NEWLINE {
			p.nextToken()
			return
		}
		if p.curToken.Type == token.DEDENT {
			return // don't consume — the block parser needs it
		}
		p.nextToken()
	}
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
			p.synchronize()
			if len(p.errors) >= maxParserErrors {
				p.addError("too many errors, stopping")
				break
			}
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
	case token.EXTERN:
		return p.parseExtern()
	case token.STRUCT:
		return p.parseStruct()
	case token.IF:
		return p.parseIfStatement()
	case token.WHEN:
		return p.parseWhenStatement()
	case token.FOR:
		return p.parseForLoop()
	case token.WHILE:
		return p.parseWhileLoop()
	case token.BREAK:
		return p.parseBreak()
	case token.CONTINUE:
		return p.parseContinue()
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
					p.synchronize()
					if len(p.errors) >= maxParserErrors {
						break
					}
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
				p.synchronize()
				if len(p.errors) >= maxParserErrors {
					break
				}
			}
		}
		if p.curToken.Type == token.NEWLINE {
			p.nextToken()
		}
	}

	return node
}

func (p *Parser) parseBreak() *ast.TreeNode {
	tok := p.curToken
	p.nextToken()
	return ast.NewNode(ast.BreakNode, &tok)
}

func (p *Parser) parseContinue() *ast.TreeNode {
	tok := p.curToken
	p.nextToken()
	return ast.NewNode(ast.ContinueNode, &tok)
}

// parseStruct parses:
//
//	struct Name:
//	    field1: Type
//	    field2: Type = default
func (p *Parser) parseStruct() *ast.TreeNode {
	tok := p.curToken
	p.nextToken() // skip 'struct'

	if p.curToken.Type != token.ID {
		p.addError("expected struct name after 'struct'")
		return nil
	}

	nameTok := p.curToken
	node := ast.NewNode(ast.StructDefNode, &nameTok)
	p.nextToken()

	if !p.expect(token.COLON) {
		return nil
	}

	// Expect indented block of field declarations
	if !p.expect(token.NEWLINE) {
		return nil
	}
	if !p.expect(token.INDENT) {
		return nil
	}

	for p.curToken.Type != token.DEDENT && !p.isAtEnd() {
		if p.curToken.Type == token.NEWLINE {
			p.nextToken()
			continue
		}
		field := p.parseStructField()
		if field != nil {
			node.AddChild(field)
		} else {
			p.synchronize()
			if len(p.errors) >= maxParserErrors {
				break
			}
			continue
		}
		if p.curToken.Type == token.NEWLINE {
			p.nextToken()
		}
	}

	p.expect(token.DEDENT)

	_ = tok
	return node
}

// parseStructField parses a single field declaration: name: Type [= default]
func (p *Parser) parseStructField() *ast.TreeNode {
	if p.curToken.Type != token.ID {
		p.addError("expected field name in struct definition")
		return nil
	}

	fieldTok := p.curToken
	node := ast.NewNode(ast.StructFieldNode, &fieldTok)
	p.nextToken()

	if !p.expect(token.COLON) {
		return nil
	}

	typeNode := p.parseTypeExpr()
	if typeNode == nil {
		return nil
	}
	node.AddChild(typeNode)

	// Optional default value
	if p.curToken.Type == token.EQUALS {
		p.nextToken()
		defaultExpr := p.parseExpression(ast.PrecTernary)
		if defaultExpr != nil {
			if !p.isConstantDefault(defaultExpr) {
				p.addError("struct field default values must be constant expressions")
			}
			node.DefaultValue = defaultExpr
		}
	}

	return node
}

// isConstantDefault checks if an expression is a valid constant default for struct fields.
// Allows literals, unary minus on numeric literals, and simple binary ops on literals.
func (p *Parser) isConstantDefault(node *ast.TreeNode) bool {
	if node == nil {
		return false
	}
	if node.NodeType == ast.LiteralNode {
		return true
	}
	// Unary minus on numeric literal
	if node.NodeType == ast.OperatorNode && node.Token != nil && node.Token.Type == token.MINUS && len(node.Children) == 1 {
		return p.isConstantDefault(node.Children[0])
	}
	// Binary constant expression (e.g. 60 * 60)
	if node.NodeType == ast.OperatorNode && len(node.Children) == 2 {
		switch node.Token.Type {
		case token.PLUS, token.MINUS, token.MULTIPLY, token.DIVIDE, token.MODULO:
			return p.isConstantDefault(node.Children[0]) && p.isConstantDefault(node.Children[1])
		}
	}
	// Empty list or empty dict
	if node.NodeType == ast.ListNode && len(node.Children) == 0 {
		return true
	}
	if node.NodeType == ast.DictNode && len(node.Children) == 0 {
		return true
	}
	return false
}

// isTypeToken checks if the current token can be a type name for annotations
func (p *Parser) isTypeToken() bool {
	switch p.curToken.Type {
	case token.LIST, token.DICT, token.VECTOR, token.RESULT:
		return true
	case token.ID:
		switch p.curToken.Literal {
		case "int", "float", "str", "bool", "any":
			return true
		}
		// User-defined struct names are valid types
		if p.structNames[p.curToken.Literal] {
			return true
		}
	}
	return false
}

func (p *Parser) parseFunction() *ast.TreeNode {
	if p.curToken.Type != token.FN {
		return nil
	}

	// Normalize named function form into assignment-to-lambda:
	// fn name(params) [ReturnType] -> body  ==>  name = fn(params) [ReturnType] -> body
	fnTok := p.curToken
	p.nextToken() // skip 'fn'

	if p.curToken.Type != token.ID {
		p.addError("expected function name")
		return nil
	}

	nameTok := p.curToken
	nameNode := ast.NewNode(ast.IdentifierNode, &nameTok)
	p.nextToken()

	args := p.parseParameters()

	// Optional return type annotation before ->
	var returnTypeNode *ast.TreeNode
	if p.isTypeToken() && p.peek(1).Type == token.ARROW {
		returnTypeNode = p.parseTypeExpr()
	}

	if !p.expect(token.ARROW) {
		return nil
	}

	body := p.parseBlock()

	lambdaNode := ast.NewNode(ast.LambdaNode, &fnTok)
	lambdaNode.AddChildren(args, body)
	lambdaNode.ReturnType = returnTypeNode

	eqTok := token.Token{Type: token.EQUALS, Literal: "=", Line: fnTok.Line, Column: fnTok.Column}
	assignNode := ast.NewNode(ast.OperatorNode, &eqTok)
	assignNode.AddChildren(nameNode, lambdaNode)

	return assignNode
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

	seenDefault := false

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

		// Optional default value: = <literal>
		if p.curToken.Type == token.EQUALS {
			p.nextToken()
			defaultExpr := p.parseExpression(ast.PrecTernary)
			if defaultExpr != nil {
				// Enforce literals only for default values
				if !p.isLiteralDefault(defaultExpr) {
					p.addError("default parameter values must be literals (int, float, string, bool, null, or empty list)")
				}
				paramNode.DefaultValue = defaultExpr
			}
			seenDefault = true
		} else if seenDefault {
			p.addError("required parameter '%s' cannot follow a parameter with a default value", paramTok.Literal)
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

// isLiteralDefault checks if an expression is a valid default parameter value (literal only)
func (p *Parser) isLiteralDefault(node *ast.TreeNode) bool {
	if node == nil {
		return false
	}
	if node.NodeType == ast.LiteralNode {
		return true
	}
	// Allow unary minus on numeric literals: -5, -3.14
	if node.NodeType == ast.OperatorNode && node.Token != nil && node.Token.Type == token.MINUS && len(node.Children) == 1 {
		return p.isNegatedNumericLiteral(node)
	}
	// Allow empty list: list []
	if node.NodeType == ast.ListNode && len(node.Children) == 0 {
		return true
	}
	return false
}

func (p *Parser) isNumericLiteral(node *ast.TreeNode) bool {
	if node == nil || node.NodeType != ast.LiteralNode || node.Token == nil {
		return false
	}
	return node.Token.Type == token.INT || node.Token.Type == token.FLOAT
}

func (p *Parser) isNegatedNumericLiteral(node *ast.TreeNode) bool {
	if node == nil || node.NodeType != ast.OperatorNode || node.Token == nil || node.Token.Type != token.MINUS || len(node.Children) != 1 {
		return false
	}
	child := node.Children[0]
	if p.isNumericLiteral(child) {
		return true
	}
	// Allow nested unary minus forms like --5.
	if child != nil && child.NodeType == ast.OperatorNode && child.Token != nil && child.Token.Type == token.MINUS {
		return p.isNegatedNumericLiteral(child)
	}
	return false
}

func (p *Parser) parseTypeExpr() *ast.TreeNode {
	if p.curToken.Type != token.ID && p.curToken.Type != token.LIST && p.curToken.Type != token.DICT && p.curToken.Type != token.VECTOR && p.curToken.Type != token.RESULT {
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
	if condition == nil {
		p.addError("expected condition after 'if'")
		return nil
	}
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
		if elseifCondition == nil {
			p.addError("expected condition after 'elseif'")
			return nil
		}

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
	if expr == nil {
		p.addError("expected expression after 'when'")
		return nil
	}
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
	if result == nil {
		p.addError("expected expression after '->' in pattern")
		return nil
	}
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
	if iterable == nil {
		p.addError("expected iterable expression after 'in'")
		return nil
	}
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
	if condition == nil {
		p.addError("expected condition after 'while'")
		return nil
	}
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

// parseExtern parses:
//
//	extern 'path/to/impl.hpp'
//	extern fn name(params) ReturnType as 'symbol'
//	extern fn type.name(params) ReturnType as 'symbol'
func (p *Parser) parseExtern() *ast.TreeNode {
	tok := p.curToken
	p.nextToken() // skip 'extern'

	switch p.curToken.Type {
	case token.STRING:
		// extern 'path/to/impl.hpp'
		pathTok := p.curToken
		node := ast.NewNode(ast.ExternSourceNode, &tok)
		pathNode := ast.NewNode(ast.LiteralNode, &pathTok)
		node.AddChild(pathNode)
		p.nextToken()
		return node
	case token.FN:
		return p.parseExternFn(&tok)
	default:
		p.addError("expected string path or 'fn' after 'extern', got %s", p.curToken.Type)
		return nil
	}
}

// parseExternFn parses:
//
//	extern fn name(params) ReturnType as 'symbol'
//	extern fn type.name(params) ReturnType as 'symbol'
func (p *Parser) parseExternFn(externTok *token.Token) *ast.TreeNode {
	p.nextToken() // skip 'fn'

	// Parse function name — may be 'name' or 'type.name'
	if p.curToken.Type != token.ID && !p.isBuiltinTypeKeyword() {
		p.addError("expected function name after 'extern fn'")
		return nil
	}
	firstTok := p.curToken
	firstName := p.curToken.Literal
	p.nextToken()

	receiverType := ""
	funcName := firstName

	// Check for type.name pattern
	if p.curToken.Type == token.DOT {
		p.nextToken() // skip '.'
		if p.curToken.Type != token.ID {
			p.addError("expected method name after '.' in extern fn declaration")
			return nil
		}
		receiverType = firstName
		funcName = p.curToken.Literal
		p.nextToken()
	}

	// Parse parameters
	params := p.parseParameters()

	// Parse return type
	var returnTypeNode *ast.TreeNode
	if p.isTypeToken() {
		returnTypeNode = p.parseTypeExpr()
	}

	// Expect 'as'
	if p.curToken.Type != token.AS {
		p.addError("expected 'as' after extern fn signature")
		return nil
	}
	p.nextToken() // skip 'as'

	// Expect symbol string
	if p.curToken.Type != token.STRING {
		p.addError("expected string symbol after 'as' in extern fn")
		return nil
	}
	symbolLiteral := p.curToken.Literal
	p.nextToken()

	nameTok := token.Token{Type: token.ID, Literal: funcName, Line: firstTok.Line, Column: firstTok.Column}
	node := ast.NewNode(ast.ExternFnNode, &nameTok)
	node.AddChildren(params.Children...)
	node.ReturnType = returnTypeNode
	node.ExternSymbol = symbolLiteral
	node.ExternReceiver = receiverType

	_ = externTok
	return node
}

// isBuiltinTypeKeyword returns true if the current token is a type keyword that
// can appear as the receiver prefix in an extern fn declaration (e.g. 'list', 'vector').
func (p *Parser) isBuiltinTypeKeyword() bool {
	switch p.curToken.Type {
	case token.LIST, token.DICT, token.VECTOR:
		return true
	case token.ID:
		switch p.curToken.Literal {
		case "int", "float", "str", "bool":
			return true
		}
	}
	return false
}

// parseUse parses:
//
//	use module_name
//	use './path/to/module'
//	use module_name as alias
//	use './path/to/module' as alias
func (p *Parser) parseUse() *ast.TreeNode {
	tok := p.curToken
	node := ast.NewNode(ast.UseNode, &tok)
	p.nextToken() // skip 'use'

	switch p.curToken.Type {
	case token.STRING:
		// File import: use './helpers' or use 'csv'
		pathTok := p.curToken
		pathNode := ast.NewNode(ast.LiteralNode, &pathTok)
		node.AddChild(pathNode)
		p.nextToken()
	case token.ID:
		// Same-file module reference: use math
		nameTok := p.curToken
		nameNode := ast.NewNode(ast.IdentifierNode, &nameTok)
		node.AddChild(nameNode)
		p.nextToken()
	default:
		p.addError("expected module path string or module name after 'use'")
		return nil
	}

	if p.curToken.Type == token.AS {
		p.nextToken() // skip 'as'
		if p.curToken.Type != token.ID {
			p.addError("expected alias identifier after 'as'")
			return nil
		}
		aliasTok := p.curToken
		aliasNode := ast.NewNode(ast.IdentifierNode, &aliasTok)
		node.AddChild(aliasNode)
		p.nextToken()
	}

	return node
}
