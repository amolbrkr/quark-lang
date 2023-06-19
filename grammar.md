# Quark Grammar Specification

This file defines the grammar for the Quark language as it stands now, contrary to the `lex_grammr.py` which defines the grammar used by the language's lexer `QuarkLexer`. The grammar is written in standard `EBNF` notation and not everything is concrete at the moment. The `::=` symbol is used to represent rule-definition relations and terminal symbols are wrapped in `<>`.

## CompilationUnit
    CompilationUnit ::= Block 'EOF'

## Block
    Block ::= { Statment 'NEWLINE' }
          |   { 'INDENT' } Statment 'NEWLINE' 
          |   { 'INDENT' } Statment 'NEWLINE' { 'DEDENT' }

## Statement
    Statement ::= <Identifier> '=' Expression
              |   'if' Expression ':' { 'NEWLINE' } Block
              |   'if' Expression ':' { 'NEWLINE' } Block 'else:' { 'NEWLINE' } Block
              |   Function
              |   Expression

## Expression
    Expression ::= Term ( '+' | '-' | '*' | '/' ) Expression
               |   Term ( '<' | '>' | '<=' | '>=' ) Expression 
               |   ('!' | '-' ) Expression
               |   '(' Expression ')'
               |   Term

## Function
    Function ::= 'fn' <Identifier> ' ' Arguments ':' { 'NEWLINE' } Block
             |   <Identifier> '=' fn' ' ' Arguments ':' { 'NEWLINE' } Block
             | <Identifier> ' ' Arguments
             | { <Identifier> '.' } Arguments
             | '(' <Identifier> ' ' Arguments ')'

## Arugments
    Arguments ::= Expression { ',' Expression }

## Term
    Term ::= <Identifier>
             |   <Literal>