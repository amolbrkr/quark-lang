use crate::token::{keyword_type, Token, TokenType};
use anyhow::{anyhow, Result};

pub struct Lexer {
    input: Vec<char>,
    position: usize,
    line: usize,
    column: usize,
    indent_stack: Vec<usize>,
    at_line_start: bool,
    last_token_needs_indent_tracking: bool,
}

impl Lexer {
    pub fn new(input: &str) -> Self {
        Self {
            input: input.chars().collect(),
            position: 0,
            line: 1,
            column: 1,
            indent_stack: vec![0],
            at_line_start: true,
            last_token_needs_indent_tracking: false,
        }
    }

    pub fn tokenize(&mut self) -> Result<Vec<Token>> {
        let mut tokens = Vec::new();

        loop {
            // Handle indentation at line start
            if self.at_line_start && !self.is_at_end() {
                let current_char = self.current_char();

                // Skip empty lines and comment lines
                if current_char == '\n' {
                    self.advance();
                    continue;
                }

                if current_char == '/' && self.peek() == Some('/') {
                    self.skip_comment();
                    continue;
                }

                // Count current indentation level
                let indent_level = self.count_indentation();
                let current_indent = *self.indent_stack.last().unwrap();

                // Emit INDENT only after a colon (block start)
                if self.last_token_needs_indent_tracking && indent_level > current_indent {
                    self.indent_stack.push(indent_level);
                    tokens.push(Token::new(
                        TokenType::Indent,
                        String::new(),
                        self.line,
                        self.column,
                    ));
                    self.last_token_needs_indent_tracking = false;
                } else if indent_level < current_indent {
                    // Always emit DEDENT when indentation decreases
                    while let Some(&stack_indent) = self.indent_stack.last() {
                        if stack_indent <= indent_level {
                            break;
                        }
                        self.indent_stack.pop();
                        // Emit all DEDENTs immediately
                        tokens.push(Token::new(
                            TokenType::Dedent,
                            String::new(),
                            self.line,
                            self.column,
                        ));
                    }
                    self.last_token_needs_indent_tracking = false;
                } else {
                    // Same level - reset flag without action
                    self.last_token_needs_indent_tracking = false;
                }

                self.at_line_start = false;
            }

            if self.is_at_end() {
                break;
            }

            self.skip_whitespace_inline();

            if self.is_at_end() {
                break;
            }

            let token = self.next_token()?;

            // Track if this token requires indentation tracking for the next line
            // Only set to true on colon; it gets reset after indentation processing
            if matches!(token.token_type, TokenType::Colon) {
                self.last_token_needs_indent_tracking = true;
            }

            tokens.push(token);
        }

        // Add remaining dedents
        while self.indent_stack.len() > 1 {
            self.indent_stack.pop();
            tokens.push(Token::new(
                TokenType::Dedent,
                String::new(),
                self.line,
                self.column,
            ));
        }

        tokens.push(Token::new(
            TokenType::Eof,
            String::new(),
            self.line,
            self.column,
        ));

        Ok(tokens)
    }

    fn next_token(&mut self) -> Result<Token> {
        let start_line = self.line;
        let start_column = self.column;
        let ch = self.current_char();

        let token = match ch {
            '\n' => {
                self.advance();
                self.at_line_start = true;
                Token::new(TokenType::Newline, String::from("\n"), start_line, start_column)
            }
            '+' => {
                self.advance();
                Token::new(TokenType::Plus, String::from("+"), start_line, start_column)
            }
            '-' => {
                self.advance();
                Token::new(TokenType::Minus, String::from("-"), start_line, start_column)
            }
            '*' => {
                self.advance();
                if self.current_char_if_not_end() == Some('*') {
                    self.advance();
                    Token::new(TokenType::Power, String::from("**"), start_line, start_column)
                } else {
                    Token::new(TokenType::Star, String::from("*"), start_line, start_column)
                }
            }
            '/' => {
                self.advance();
                if self.current_char_if_not_end() == Some('/') {
                    self.skip_comment();
                    return self.next_token();
                } else {
                    Token::new(TokenType::Slash, String::from("/"), start_line, start_column)
                }
            }
            '%' => {
                self.advance();
                Token::new(TokenType::Percent, String::from("%"), start_line, start_column)
            }
            '=' => {
                self.advance();
                if self.current_char_if_not_end() == Some('=') {
                    self.advance();
                    Token::new(TokenType::EqualsEquals, String::from("=="), start_line, start_column)
                } else {
                    Token::new(TokenType::Equals, String::from("="), start_line, start_column)
                }
            }
            '!' => {
                self.advance();
                if self.current_char_if_not_end() == Some('=') {
                    self.advance();
                    Token::new(TokenType::NotEquals, String::from("!="), start_line, start_column)
                } else {
                    Token::new(TokenType::Not, String::from("!"), start_line, start_column)
                }
            }
            '<' => {
                self.advance();
                if self.current_char_if_not_end() == Some('=') {
                    self.advance();
                    Token::new(TokenType::LessEquals, String::from("<="), start_line, start_column)
                } else {
                    Token::new(TokenType::Less, String::from("<"), start_line, start_column)
                }
            }
            '>' => {
                self.advance();
                if self.current_char_if_not_end() == Some('=') {
                    self.advance();
                    Token::new(TokenType::GreaterEquals, String::from(">="), start_line, start_column)
                } else {
                    Token::new(TokenType::Greater, String::from(">"), start_line, start_column)
                }
            }
            '~' => {
                self.advance();
                Token::new(TokenType::Tilde, String::from("~"), start_line, start_column)
            }
            '&' => {
                self.advance();
                Token::new(TokenType::Ampersand, String::from("&"), start_line, start_column)
            }
            '|' => {
                self.advance();
                Token::new(TokenType::Pipe, String::from("|"), start_line, start_column)
            }
            '.' => {
                self.advance();
                if self.current_char_if_not_end() == Some('.') {
                    self.advance();
                    Token::new(TokenType::DotDot, String::from(".."), start_line, start_column)
                } else if self.current_char_if_not_end().map_or(false, |c| c.is_ascii_digit()) {
                    // Float starting with .
                    self.position -= 1;
                    self.column -= 1;
                    self.scan_number()
                } else {
                    Token::new(TokenType::Dot, String::from("."), start_line, start_column)
                }
            }
            '@' => {
                self.advance();
                Token::new(TokenType::At, String::from("@"), start_line, start_column)
            }
            '(' => {
                self.advance();
                Token::new(TokenType::Lparen, String::from("("), start_line, start_column)
            }
            ')' => {
                self.advance();
                Token::new(TokenType::Rparen, String::from(")"), start_line, start_column)
            }
            '{' => {
                self.advance();
                Token::new(TokenType::Lsquare, String::from("{"), start_line, start_column)
            }
            '}' => {
                self.advance();
                Token::new(TokenType::Rsquare, String::from("}"), start_line, start_column)
            }
            '[' => {
                self.advance();
                Token::new(TokenType::Lbrace, String::from("["), start_line, start_column)
            }
            ']' => {
                self.advance();
                Token::new(TokenType::Rbrace, String::from("]"), start_line, start_column)
            }
            ',' => {
                self.advance();
                Token::new(TokenType::Comma, String::from(","), start_line, start_column)
            }
            ':' => {
                self.advance();
                Token::new(TokenType::Colon, String::from(":"), start_line, start_column)
            }
            '\'' => self.scan_string()?,
            _ if ch.is_ascii_digit() => self.scan_number(),
            _ if ch.is_alphabetic() || ch == '_' => self.scan_identifier(),
            _ => {
                return Err(anyhow!(
                    "Unexpected character '{}' at {}:{}",
                    ch,
                    start_line,
                    start_column
                ))
            }
        };

        Ok(token)
    }

    fn scan_number(&mut self) -> Token {
        let start_line = self.line;
        let start_column = self.column;
        let mut lexeme = String::new();
        let mut is_float = false;

        // Handle leading dot for floats like .5
        if self.current_char() == '.' {
            is_float = true;
            lexeme.push('.');
            self.advance();
        }

        while !self.is_at_end() && self.current_char().is_ascii_digit() {
            lexeme.push(self.current_char());
            self.advance();
        }

        if !self.is_at_end() && self.current_char() == '.' && self.peek() != Some('.') {
            is_float = true;
            lexeme.push('.');
            self.advance();

            while !self.is_at_end() && self.current_char().is_ascii_digit() {
                lexeme.push(self.current_char());
                self.advance();
            }
        }

        let token_type = if is_float {
            TokenType::Float
        } else {
            TokenType::Integer
        };

        Token::new(token_type, lexeme, start_line, start_column)
    }

    fn scan_string(&mut self) -> Result<Token> {
        let start_line = self.line;
        let start_column = self.column;
        self.advance(); // Skip opening quote

        let mut lexeme = String::new();

        while !self.is_at_end() && self.current_char() != '\'' {
            if self.current_char() == '\\' {
                self.advance();
                if !self.is_at_end() {
                    let escaped = match self.current_char() {
                        'n' => '\n',
                        't' => '\t',
                        'r' => '\r',
                        '\\' => '\\',
                        '\'' => '\'',
                        c => c,
                    };
                    lexeme.push(escaped);
                    self.advance();
                }
            } else {
                lexeme.push(self.current_char());
                self.advance();
            }
        }

        if self.is_at_end() {
            return Err(anyhow!("Unterminated string at {}:{}", start_line, start_column));
        }

        self.advance(); // Skip closing quote

        Ok(Token::new(TokenType::String, lexeme, start_line, start_column))
    }

    fn scan_identifier(&mut self) -> Token {
        let start_line = self.line;
        let start_column = self.column;
        let mut lexeme = String::new();

        while !self.is_at_end() {
            let ch = self.current_char();
            if ch.is_alphanumeric() || ch == '_' {
                lexeme.push(ch);
                self.advance();
            } else {
                break;
            }
        }

        let token_type = keyword_type(&lexeme).unwrap_or(TokenType::Identifier);
        Token::new(token_type, lexeme, start_line, start_column)
    }

    fn skip_comment(&mut self) {
        while !self.is_at_end() && self.current_char() != '\n' {
            self.advance();
        }
    }

    fn skip_whitespace_inline(&mut self) {
        while !self.is_at_end() {
            let ch = self.current_char();
            if ch == ' ' || ch == '\t' || ch == '\r' {
                self.advance();
            } else {
                break;
            }
        }
    }

    fn count_indentation(&mut self) -> usize {
        let mut count = 0;
        while !self.is_at_end() {
            let ch = self.current_char();
            if ch == ' ' {
                count += 1;
                self.advance();
            } else if ch == '\t' {
                count += 4;
                self.advance();
            } else {
                break;
            }
        }
        count
    }

    fn current_char(&self) -> char {
        self.input[self.position]
    }

    fn current_char_if_not_end(&self) -> Option<char> {
        if self.is_at_end() {
            None
        } else {
            Some(self.input[self.position])
        }
    }

    fn peek(&self) -> Option<char> {
        if self.position + 1 < self.input.len() {
            Some(self.input[self.position + 1])
        } else {
            None
        }
    }

    fn advance(&mut self) {
        if !self.is_at_end() {
            if self.input[self.position] == '\n' {
                self.line += 1;
                self.column = 1;
            } else {
                self.column += 1;
            }
            self.position += 1;
        }
    }

    fn is_at_end(&self) -> bool {
        self.position >= self.input.len()
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn test_simple_tokens() {
        let mut lexer = Lexer::new("+ - * / ** == !=");
        let tokens = lexer.tokenize().unwrap();

        assert_eq!(tokens[0].token_type, TokenType::Plus);
        assert_eq!(tokens[1].token_type, TokenType::Minus);
        assert_eq!(tokens[2].token_type, TokenType::Star);
        assert_eq!(tokens[3].token_type, TokenType::Slash);
        assert_eq!(tokens[4].token_type, TokenType::Power);
        assert_eq!(tokens[5].token_type, TokenType::EqualsEquals);
        assert_eq!(tokens[6].token_type, TokenType::NotEquals);
    }

    #[test]
    fn test_numbers() {
        let mut lexer = Lexer::new("42 3.14 .5 2.");
        let tokens = lexer.tokenize().unwrap();

        assert_eq!(tokens[0].token_type, TokenType::Integer);
        assert_eq!(tokens[0].lexeme, "42");
        assert_eq!(tokens[1].token_type, TokenType::Float);
        assert_eq!(tokens[1].lexeme, "3.14");
        assert_eq!(tokens[2].token_type, TokenType::Float);
        assert_eq!(tokens[2].lexeme, ".5");
    }

    #[test]
    fn test_keywords() {
        let mut lexer = Lexer::new("fn if else while for");
        let tokens = lexer.tokenize().unwrap();

        assert_eq!(tokens[0].token_type, TokenType::Fn);
        assert_eq!(tokens[1].token_type, TokenType::If);
        assert_eq!(tokens[2].token_type, TokenType::Else);
        assert_eq!(tokens[3].token_type, TokenType::While);
        assert_eq!(tokens[4].token_type, TokenType::For);
    }
}
