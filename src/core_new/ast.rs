use crate::token::Token;
use std::fmt;

#[derive(Debug, Clone, PartialEq)]
pub enum NodeType {
    CompilationUnit,
    Block,
    Statement,
    Expression,
    Function,
    FunctionCall,
    Arguments,
    IfStatement,
    WhenStatement,
    Pattern,
    ForLoop,
    WhileLoop,
    Lambda,
    Ternary,
    Pipe,
    Identifier,
    Literal,
    Operator,
    BinaryOp,
    UnaryOp,
    List,
    Dict,
    MemberAccess,
}

#[derive(Debug, Clone)]
pub struct AstNode {
    pub node_type: NodeType,
    pub token: Option<Token>,
    pub children: Vec<AstNode>,
}

impl AstNode {
    pub fn new(node_type: NodeType, token: Option<Token>) -> Self {
        Self {
            node_type,
            token,
            children: Vec::new(),
        }
    }

    pub fn with_children(node_type: NodeType, token: Option<Token>, children: Vec<AstNode>) -> Self {
        Self {
            node_type,
            token,
            children,
        }
    }

    pub fn add_child(&mut self, child: AstNode) {
        self.children.push(child);
    }

    pub fn token_lexeme(&self) -> String {
        self.token.as_ref().map(|t| t.lexeme.clone()).unwrap_or_default()
    }
}

impl fmt::Display for AstNode {
    fn fmt(&self, f: &mut fmt::Formatter) -> fmt::Result {
        self.display_recursive(f, 0)
    }
}

impl AstNode {
    fn display_recursive(&self, f: &mut fmt::Formatter, depth: usize) -> fmt::Result {
        let indent = "  ".repeat(depth);

        write!(f, "{}{:?}", indent, self.node_type)?;

        if let Some(ref token) = self.token {
            write!(f, " '{}'", token.lexeme)?;
        }

        writeln!(f)?;

        for child in &self.children {
            child.display_recursive(f, depth + 1)?;
        }

        Ok(())
    }
}

// Precedence levels (higher number = tighter binding)
#[derive(Debug, Clone, Copy, PartialEq, Eq, PartialOrd, Ord)]
pub struct Precedence(pub i32);

impl Precedence {
    pub const LOWEST: Precedence = Precedence(0);
    pub const ASSIGNMENT: Precedence = Precedence(1);
    pub const PIPE: Precedence = Precedence(2);
    pub const COMMA: Precedence = Precedence(3);
    pub const TERNARY: Precedence = Precedence(4);
    pub const OR: Precedence = Precedence(5);
    pub const AND: Precedence = Precedence(6);
    pub const BITWISE_AND: Precedence = Precedence(7);
    pub const EQUALITY: Precedence = Precedence(8);
    pub const COMPARISON: Precedence = Precedence(9);
    pub const RANGE: Precedence = Precedence(10);
    pub const TERM: Precedence = Precedence(11);
    pub const FACTOR: Precedence = Precedence(12);
    pub const EXPONENT: Precedence = Precedence(13);
    pub const UNARY: Precedence = Precedence(14);
    pub const APPLICATION: Precedence = Precedence(15);
    pub const CALL: Precedence = Precedence(16);
}
