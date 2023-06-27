# Quark Grammar Specification

This file defines the grammar for the Quark language as it stands now, contrary to the `lex_grammr.py` which defines the grammar used by the language's lexer `QuarkLexer`. The grammar is written in extended `EBNF` notation and not everything is concrete at the moment. Terminal symbols are wrapped in `<>`.

## CompilationUnit
    CompilationUnit ::= Block 'EOF'

## Block
    Block ::= Statements
          |   'NEWLINE' 'INDENT' Statements 'DEDENT'

## Statement
    Statements ::= { Statement 'NEWLINE' }
    Statement ::= IfStatement
              |   Function
              |   FunctionCall
              |   Expression

## Expression
    Expression ::= <Identifier> '=' Expression
               |   Equality
               |   Comparison
               |   Term
               |   Factor
               |   Unary
               |   Primary
               |   '(' Expression ')'
    
    Equality ::= Comparison { ( "!=" | "==" ) Comparison }
    Comparison ::= Term { ( ">" | ">=" | "<=" | "<" ) Term }
    Term ::= Factor { ( "-" | "+" ) Factor }
    Factor ::= Unary { ( "/" | "*" ) Unary }

    Unary ::= ( "!" | "-" ) Unary
          |   Primary
    
    Primary ::= <Identifier>
            |   <Literal>
            |   "true"
            |   "false"
            |   "null"
            |   "it"

## If-Else Statement
    IfStatement ::= 'if' Expression ':' Block { ElseStatement }

    ElseStatement ::= 'else' ':' Block

## Function
    Function ::= 'fn' <Identifier> ' ' Arguments ':' Block
             |   <Identifier> '=' fn' ' ' Arguments ':' Block
    
    FunctionCall ::= '@' <Identifier> ' ' Arguments
                 |   '@' { <Identifier> '.' } Arguments
                 |   '(' '@' <Identifier> ' ' Arguments ')'

## Arugments
    Arguments ::= { Expression ',' }

## Term
    Term ::= <Identifier>
             |   <Literal>