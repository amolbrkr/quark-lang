# Quark Grammar Specification

This file defines the grammar for the Quark language as it stands now, contrary to the `lex_grammr.py` which defines the grammar used by the language's lexer `QuarkLexer`. The grammar is written in standard `EBNF` notation and not everything is concrete at the moment. The `::=` symbol is used to represent rule-definition relations and terminal symbols are wrapped in `<>`.

## CompilationUnit
    CompilationUnit ::= Block

## Block
    Block ::= { Statment 'NEWLINE' }
          |   'NEWLINE' { 'INDENT' } Statment 'NEWLINE' { 'DEDENT' }
          |   'EOF'

## Statement
    Statement ::= IfStatement
              |   Function
              |   FunctionCall
              |   Expression

## Expression
    Expression ::= <Identifier> '=' Expression
               |   Term ( '+' | '-' | '*' | '/' ) Expression
               |   Term ( '<' | '>' | '<=' | '>=' ) Expression 
               |   ('!' | '-' ) Expression
               |   '(' Expression ')'
               |   Term

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