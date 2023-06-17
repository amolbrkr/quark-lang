# Quark Grammar Specification

This file defines the grammar for the Quark language as it stands now, contrary to the `lex_grammr.py` which defines the grammar used by the language's lexer `QuarkLexer`. The grammar is written in standard `EBNF` notation and not everything is concrete at the moment. The `::=` symbol is used to represent rule-definition relations and terminal symbols are wrapped in `<>`.

## CompilationUnit
    CompilationUnit ::= { Statement }

## Statement
    Statement ::= <Identifier> '=' Expression
              |   Function

## Expression
    Expression ::= <Literal> ('+' | '-') Expression
               |   <Literal> ('*' | '/') Expression
               |   <Literal> 

## Function
    Function ::= 'fn' <Identifier> ' ' Arguments ':' { Statement }
             |   'fn' ' ' Arguments : { Statement }
             | <Identifier> ' ' Arguments
             | { <Identifier> '.' } Arguments
             | '(' <Identifier> ' ' Arguments ')'

## Arugments
    Arguments ::= Argument { ',' Arguemnt }

## Argument
    Argument ::= <Identifier>
             |   <Literal>