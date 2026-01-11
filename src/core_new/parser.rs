use crate::ast::{AstNode, NodeType, Precedence};
use crate::token::{Token, TokenType};
use anyhow::{anyhow, Result};

pub struct Parser {
    tokens: Vec<Token>,
    position: usize,
}

impl Parser {
    pub fn new(tokens: Vec<Token>) -> Self {
        Self { tokens, position: 0 }
    }

    pub fn parse(&mut self) -> Result<AstNode> {
        let mut root = AstNode::new(NodeType::CompilationUnit, None);

        while !self.is_at_end() && self.current().token_type != TokenType::Eof {
            self.skip_newlines();
            if self.is_at_end() || self.current().token_type == TokenType::Eof {
                break;
            }

            let stmt = self.statement()?;
            root.add_child(stmt);
            self.skip_newlines();
        }

        Ok(root)
    }

    // Statement parsing
    fn statement(&mut self) -> Result<AstNode> {
        match self.current().token_type {
            TokenType::Fn => self.function_def(),
            TokenType::If => self.if_statement(),
            TokenType::When => self.when_statement(),
            TokenType::For => self.for_loop(),
            TokenType::While => self.while_loop(),
            _ => {
                let expr = self.expression(Precedence::LOWEST)?;
                Ok(AstNode::with_children(
                    NodeType::Statement,
                    None,
                    vec![expr],
                ))
            }
        }
    }

    fn function_def(&mut self) -> Result<AstNode> {
        self.expect(TokenType::Fn)?;

        let name = self.expect(TokenType::Identifier)?;
        let mut func_node = AstNode::new(NodeType::Function, Some(name));

        // Parse parameters
        let mut params = AstNode::new(NodeType::Arguments, None);
        while !self.check(&TokenType::Colon) && !self.is_at_end() {
            let param = self.expect(TokenType::Identifier)?;
            params.add_child(AstNode::new(NodeType::Identifier, Some(param)));

            if self.check(&TokenType::Comma) {
                self.advance();
            }
        }
        func_node.add_child(params);

        self.expect(TokenType::Colon)?;

        // Parse body
        let body = self.block()?;
        func_node.add_child(body);

        Ok(func_node)
    }

    fn if_statement(&mut self) -> Result<AstNode> {
        let if_token = self.expect(TokenType::If)?;
        let mut if_node = AstNode::new(NodeType::IfStatement, Some(if_token));

        // Condition
        let condition = self.expression(Precedence::LOWEST)?;
        if_node.add_child(condition);

        self.expect(TokenType::Colon)?;

        // Then block
        let then_block = self.block()?;
        if_node.add_child(then_block);

        // Elseif clauses
        while self.check(&TokenType::Elseif) {
            self.advance();
            let elseif_condition = self.expression(Precedence::LOWEST)?;
            self.expect(TokenType::Colon)?;
            let elseif_block = self.block()?;

            let mut elseif_node = AstNode::new(NodeType::IfStatement, None);
            elseif_node.add_child(elseif_condition);
            elseif_node.add_child(elseif_block);
            if_node.add_child(elseif_node);
        }

        // Else clause
        if self.check(&TokenType::Else) {
            self.advance();
            self.expect(TokenType::Colon)?;
            let else_block = self.block()?;
            if_node.add_child(else_block);
        }

        Ok(if_node)
    }

    fn when_statement(&mut self) -> Result<AstNode> {
        let when_token = self.expect(TokenType::When)?;
        let mut when_node = AstNode::new(NodeType::WhenStatement, Some(when_token));

        // Match expression
        let match_expr = self.expression(Precedence::LOWEST)?;
        when_node.add_child(match_expr);

        self.expect(TokenType::Colon)?;
        self.skip_newlines();

        self.expect(TokenType::Indent)?;

        // Parse patterns
        while !self.check(&TokenType::Dedent) && !self.is_at_end() {
            let pattern = self.parse_pattern()?;
            when_node.add_child(pattern);
            self.skip_newlines();
        }

        self.expect(TokenType::Dedent)?;

        Ok(when_node)
    }

    fn parse_pattern(&mut self) -> Result<AstNode> {
        let mut pattern_node = AstNode::new(NodeType::Pattern, None);

        // Parse pattern expressions (can be multiple with 'or')
        loop {
            let pattern_expr = self.expression(Precedence::COMMA)?;
            pattern_node.add_child(pattern_expr);

            if self.check(&TokenType::Or) {
                self.advance();
            } else {
                break;
            }
        }

        self.expect(TokenType::Colon)?;

        // Parse result expression
        let result = self.expression(Precedence::LOWEST)?;
        pattern_node.add_child(result);

        Ok(pattern_node)
    }

    fn for_loop(&mut self) -> Result<AstNode> {
        let for_token = self.expect(TokenType::For)?;
        let mut for_node = AstNode::new(NodeType::ForLoop, Some(for_token));

        // Loop variable
        let var = self.expect(TokenType::Identifier)?;
        for_node.add_child(AstNode::new(NodeType::Identifier, Some(var)));

        self.expect(TokenType::In)?;

        // Iterable expression
        let iterable = self.expression(Precedence::LOWEST)?;
        for_node.add_child(iterable);

        self.expect(TokenType::Colon)?;

        // Body
        let body = self.block()?;
        for_node.add_child(body);

        Ok(for_node)
    }

    fn while_loop(&mut self) -> Result<AstNode> {
        let while_token = self.expect(TokenType::While)?;
        let mut while_node = AstNode::new(NodeType::WhileLoop, Some(while_token));

        // Condition
        let condition = self.expression(Precedence::LOWEST)?;
        while_node.add_child(condition);

        self.expect(TokenType::Colon)?;

        // Body
        let body = self.block()?;
        while_node.add_child(body);

        Ok(while_node)
    }

    fn block(&mut self) -> Result<AstNode> {
        let mut block = AstNode::new(NodeType::Block, None);

        self.skip_newlines();

        // Check for indented block
        if self.check(&TokenType::Indent) {
            self.advance();

            while !self.check(&TokenType::Dedent) && !self.is_at_end() {
                self.skip_newlines();
                if self.check(&TokenType::Dedent) {
                    break;
                }

                let stmt = self.statement()?;
                block.add_child(stmt);
                self.skip_newlines();
            }

            self.expect(TokenType::Dedent)?;
        } else {
            // Single-line block or inline block
            let stmt = self.statement()?;
            block.add_child(stmt);
        }

        Ok(block)
    }

    // Expression parsing (Pratt parser)
    fn expression(&mut self, precedence: Precedence) -> Result<AstNode> {
        let mut left = self.prefix()?;

        while !self.is_at_end() && precedence < self.current_precedence() {
            // Don't consume newlines in expression context
            if self.check(&TokenType::Newline) {
                break;
            }

            left = self.infix(left)?;
        }

        Ok(left)
    }

    fn prefix(&mut self) -> Result<AstNode> {
        match self.current().token_type {
            TokenType::Integer | TokenType::Float | TokenType::String => {
                let token = self.advance().clone();
                Ok(AstNode::new(NodeType::Literal, Some(token)))
            }
            TokenType::Identifier => {
                let token = self.advance().clone();
                Ok(AstNode::new(NodeType::Identifier, Some(token)))
            }
            TokenType::Lparen => {
                self.advance();
                let expr = self.expression(Precedence::LOWEST)?;
                self.expect(TokenType::Rparen)?;
                Ok(expr)
            }
            TokenType::Lbrace => self.parse_list(),
            TokenType::Lsquare => self.parse_dict(),
            TokenType::Minus | TokenType::Not | TokenType::Tilde => {
                let op_token = self.advance().clone();
                let operand = self.expression(Precedence::UNARY)?;
                Ok(AstNode::with_children(
                    NodeType::UnaryOp,
                    Some(op_token),
                    vec![operand],
                ))
            }
            TokenType::At => {
                self.advance();
                let func = self.expression(Precedence::CALL)?;
                self.parse_function_call_with_func(func)
            }
            _ => Err(anyhow!(
                "Unexpected token in prefix position: {:?}",
                self.current().token_type
            )),
        }
    }

    fn infix(&mut self, left: AstNode) -> Result<AstNode> {
        match self.current().token_type {
            TokenType::Plus
            | TokenType::Minus
            | TokenType::Star
            | TokenType::Slash
            | TokenType::Percent
            | TokenType::EqualsEquals
            | TokenType::NotEquals
            | TokenType::Less
            | TokenType::LessEquals
            | TokenType::Greater
            | TokenType::GreaterEquals
            | TokenType::And
            | TokenType::Or
            | TokenType::Ampersand
            | TokenType::DotDot
            | TokenType::Comma => {
                let op_token = self.advance().clone();
                let precedence = self.token_precedence(&op_token.token_type);
                let right = self.expression(precedence)?;
                Ok(AstNode::with_children(
                    NodeType::BinaryOp,
                    Some(op_token),
                    vec![left, right],
                ))
            }
            TokenType::Power => {
                let op_token = self.advance().clone();
                // Right-associative: use same precedence (not precedence + 1)
                let right = self.expression(Precedence::EXPONENT)?;
                Ok(AstNode::with_children(
                    NodeType::BinaryOp,
                    Some(op_token),
                    vec![left, right],
                ))
            }
            TokenType::Pipe => {
                let pipe_token = self.advance().clone();
                let right = self.expression(Precedence::PIPE)?;
                Ok(AstNode::with_children(
                    NodeType::Pipe,
                    Some(pipe_token),
                    vec![left, right],
                ))
            }
            TokenType::Equals => {
                let eq_token = self.advance().clone();
                let right = self.expression(Precedence::ASSIGNMENT)?;
                Ok(AstNode::with_children(
                    NodeType::Operator,
                    Some(eq_token),
                    vec![left, right],
                ))
            }
            TokenType::If => {
                self.advance();
                let condition = self.expression(Precedence::OR)?;
                self.expect(TokenType::Else)?;
                let else_expr = self.expression(Precedence::TERNARY)?;
                Ok(AstNode::with_children(
                    NodeType::Ternary,
                    None,
                    vec![condition, left, else_expr],
                ))
            }
            TokenType::Dot => {
                self.advance();
                let member = self.expect(TokenType::Identifier)?;
                Ok(AstNode::with_children(
                    NodeType::MemberAccess,
                    Some(member),
                    vec![left],
                ))
            }
            TokenType::Lparen => {
                self.parse_function_call_with_func(left)
            }
            // Function application (space operator)
            _ if self.can_start_expression() && !self.check(&TokenType::Newline) => {
                // Parse argument at TERM level to allow arithmetic within args
                let arg = self.expression(Precedence::TERM)?;
                Ok(AstNode::with_children(
                    NodeType::FunctionCall,
                    None,
                    vec![left, arg],
                ))
            }
            _ => Ok(left),
        }
    }

    fn parse_function_call_with_func(&mut self, func: AstNode) -> Result<AstNode> {
        self.expect(TokenType::Lparen)?;

        let mut call_node = AstNode::new(NodeType::FunctionCall, None);
        call_node.add_child(func);

        let mut args = AstNode::new(NodeType::Arguments, None);

        if !self.check(&TokenType::Rparen) {
            loop {
                let arg = self.expression(Precedence(Precedence::COMMA.0 + 1))?;
                args.add_child(arg);

                if self.check(&TokenType::Comma) {
                    self.advance();
                } else {
                    break;
                }
            }
        }

        self.expect(TokenType::Rparen)?;

        call_node.add_child(args);
        Ok(call_node)
    }

    fn parse_list(&mut self) -> Result<AstNode> {
        self.expect(TokenType::Lbrace)?;

        let mut list_node = AstNode::new(NodeType::List, None);

        if !self.check(&TokenType::Rbrace) {
            loop {
                let elem = self.expression(Precedence(Precedence::COMMA.0 + 1))?;
                list_node.add_child(elem);

                if self.check(&TokenType::Comma) {
                    self.advance();
                } else {
                    break;
                }
            }
        }

        self.expect(TokenType::Rbrace)?;
        Ok(list_node)
    }

    fn parse_dict(&mut self) -> Result<AstNode> {
        self.expect(TokenType::Lsquare)?;

        let mut dict_node = AstNode::new(NodeType::Dict, None);

        if !self.check(&TokenType::Rsquare) {
            loop {
                let key = self.expression(Precedence(Precedence::COMMA.0 + 1))?;
                self.expect(TokenType::Colon)?;
                let value = self.expression(Precedence(Precedence::COMMA.0 + 1))?;

                let mut pair = AstNode::new(NodeType::Expression, None);
                pair.add_child(key);
                pair.add_child(value);
                dict_node.add_child(pair);

                if self.check(&TokenType::Comma) {
                    self.advance();
                } else {
                    break;
                }
            }
        }

        self.expect(TokenType::Rsquare)?;
        Ok(dict_node)
    }

    fn can_start_expression(&self) -> bool {
        matches!(
            self.current().token_type,
            TokenType::Integer
                | TokenType::Float
                | TokenType::String
                | TokenType::Identifier
                | TokenType::Lparen
                | TokenType::Lbrace
                | TokenType::Lsquare
                | TokenType::Minus
                | TokenType::Not
                | TokenType::Tilde
                | TokenType::At
        )
    }

    fn current_precedence(&self) -> Precedence {
        if self.is_at_end() {
            return Precedence::LOWEST;
        }
        self.token_precedence(&self.current().token_type)
    }

    fn token_precedence(&self, token_type: &TokenType) -> Precedence {
        match token_type {
            TokenType::Equals => Precedence::ASSIGNMENT,
            TokenType::Pipe => Precedence::PIPE,
            TokenType::Comma => Precedence::COMMA,
            TokenType::If => Precedence::TERNARY,
            TokenType::Or => Precedence::OR,
            TokenType::And => Precedence::AND,
            TokenType::Ampersand => Precedence::BITWISE_AND,
            TokenType::EqualsEquals | TokenType::NotEquals => Precedence::EQUALITY,
            TokenType::Less | TokenType::LessEquals | TokenType::Greater | TokenType::GreaterEquals => {
                Precedence::COMPARISON
            }
            TokenType::DotDot => Precedence::RANGE,
            TokenType::Plus | TokenType::Minus => Precedence::TERM,
            TokenType::Star | TokenType::Slash | TokenType::Percent => Precedence::FACTOR,
            TokenType::Power => Precedence::EXPONENT,
            TokenType::Dot | TokenType::Lparen => Precedence::CALL,
            _ if self.can_start_expression() => Precedence::APPLICATION,
            _ => Precedence::LOWEST,
        }
    }

    // Utility methods
    fn current(&self) -> &Token {
        &self.tokens[self.position]
    }

    fn advance(&mut self) -> &Token {
        let token = &self.tokens[self.position];
        if !self.is_at_end() {
            self.position += 1;
        }
        token
    }

    fn check(&self, token_type: &TokenType) -> bool {
        !self.is_at_end() && &self.current().token_type == token_type
    }

    fn expect(&mut self, token_type: TokenType) -> Result<Token> {
        if self.check(&token_type) {
            Ok(self.advance().clone())
        } else {
            Err(anyhow!(
                "Expected {:?}, found {:?} at {}:{}",
                token_type,
                self.current().token_type,
                self.current().line,
                self.current().column
            ))
        }
    }

    fn skip_newlines(&mut self) {
        while self.check(&TokenType::Newline) {
            self.advance();
        }
    }

    fn is_at_end(&self) -> bool {
        self.position >= self.tokens.len() || self.current().token_type == TokenType::Eof
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::lexer::Lexer;

    #[test]
    fn test_simple_expression() {
        let mut lexer = Lexer::new("2 + 3 * 4");
        let tokens = lexer.tokenize().unwrap();
        let mut parser = Parser::new(tokens);
        let ast = parser.parse().unwrap();

        assert_eq!(ast.node_type, NodeType::CompilationUnit);
        assert_eq!(ast.children.len(), 1);
    }

    #[test]
    fn test_function_definition() {
        let input = "fn add x, y:\n    x + y";
        let mut lexer = Lexer::new(input);
        let tokens = lexer.tokenize().unwrap();
        let mut parser = Parser::new(tokens);
        let ast = parser.parse().unwrap();

        assert_eq!(ast.node_type, NodeType::CompilationUnit);
        assert_eq!(ast.children.len(), 1);
        assert_eq!(ast.children[0].node_type, NodeType::Function);
    }
}
