use std::fmt;

#[derive(Debug, Clone, PartialEq)]
pub enum TokenType {
    // Literals
    Integer,
    Float,
    String,

    // Identifiers and Keywords
    Identifier,
    Use,
    Module,
    In,
    And,
    Or,
    If,
    Elseif,
    Else,
    For,
    While,
    When,
    Fn,
    Class,

    // Operators
    Plus,
    Minus,
    Star,
    Slash,
    Percent,
    Power,      // **
    Equals,
    EqualsEquals,
    NotEquals,
    Less,
    LessEquals,
    Greater,
    GreaterEquals,
    Not,
    Tilde,
    Ampersand,
    Pipe,
    DotDot,     // ..
    Dot,
    At,

    // Delimiters
    Lparen,
    Rparen,
    Lbrace,
    Rbrace,
    Lsquare,
    Rsquare,
    Comma,
    Colon,

    // Special
    Newline,
    Indent,
    Dedent,
    Eof,
}

#[derive(Debug, Clone)]
pub struct Token {
    pub token_type: TokenType,
    pub lexeme: String,
    pub line: usize,
    pub column: usize,
}

impl Token {
    pub fn new(token_type: TokenType, lexeme: String, line: usize, column: usize) -> Self {
        Self {
            token_type,
            lexeme,
            line,
            column,
        }
    }
}

impl fmt::Display for Token {
    fn fmt(&self, f: &mut fmt::Formatter) -> fmt::Result {
        write!(f, "{:?}('{}') at {}:{}", self.token_type, self.lexeme, self.line, self.column)
    }
}

pub fn keyword_type(word: &str) -> Option<TokenType> {
    match word {
        "use" => Some(TokenType::Use),
        "module" => Some(TokenType::Module),
        "in" => Some(TokenType::In),
        "and" => Some(TokenType::And),
        "or" => Some(TokenType::Or),
        "if" => Some(TokenType::If),
        "elseif" => Some(TokenType::Elseif),
        "else" => Some(TokenType::Else),
        "for" => Some(TokenType::For),
        "while" => Some(TokenType::While),
        "when" => Some(TokenType::When),
        "fn" => Some(TokenType::Fn),
        "class" => Some(TokenType::Class),
        _ => None,
    }
}
