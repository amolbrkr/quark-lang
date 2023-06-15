from ply import lex


class IndentLexer:
    def __init__(self, ply_lexer):
        self.lexer = ply_lexer
        self.token_stream = None

    def _new_token(self, type, lineno, column):
        tok = lex.Token()
        tok.type, tok.value, tok.lineno, tok.column = type, None, lineno, column
        return tok

    def _track_tokens_filter(self, tokens):
        NO_INDENT, MAY_INDENT, MUST_INDENT = 0, 1, 2
        self.lexer.at_line_start = at_line_start = True
        indent = NO_INDENT
        saw_colon = False
        for token in tokens:
            token.at_line_start = at_line_start

            if token.type == "COLON":
                at_line_start = False
                indent = MAY_INDENT
                token.must_indent = False

            elif token.type == "NEWLINE":
                at_line_start = True
                if indent == MAY_INDENT:
                    indent = MUST_INDENT
                token.must_indent = False

            elif token.type == "WS":
                assert token.at_line_start == True
                at_line_start = True
                token.must_indent = False

            else:
                # A real token; only indent after COLON NEWLINE
                if indent == MUST_INDENT:
                    token.must_indent = True
                else:
                    token.must_indent = False
                at_line_start = False
                indent = NO_INDENT

            yield token
            self.lexer.at_line_start = at_line_start

    def _indentation_filter(self, tokens):
        # A stack of indentation levels; will never pop item 0
        levels = [0]
        token = None
        depth = 0
        prev_was_ws = False
        for token in tokens:
            # WS only occurs at the start of the line
            # There may be WS followed by NEWLINE so
            # only track the depth here.  Don't indent/dedent
            # until there's something real.
            if token.type == "WS":
                assert depth == 0
                depth = len(token.value)
                prev_was_ws = True
                # WS tokens are never passed to the parser
                continue

            if token.type == "NEWLINE":
                depth = 0
                if prev_was_ws or token.at_line_start:
                    # ignore blank lines
                    continue
                # pass the other cases on through
                yield token
                continue

            # then it must be a real token (not WS, not NEWLINE)
            # which can affect the indentation level

            prev_was_ws = False
            if token.must_indent:
                # The current depth must be larger than the previous level
                if not (depth > levels[-1]):
                    raise IndentationError("expected an indented block")

                levels.append(depth)
                yield self._new_token("INDENT", token.lineno, token.column)

            elif token.at_line_start:
                # Must be on the same level or one of the previous levels
                if depth == levels[-1]:
                    # At the same level
                    pass
                elif depth > levels[-1]:
                    raise IndentationError("indentation increase but not in new block")
                else:
                    # Back up; but only if it matches a previous level
                    try:
                        i = levels.index(depth)
                    except ValueError:
                        raise IndentationError("inconsistent indentation")
                    for _ in range(i + 1, len(levels)):
                        yield self._new_token("DEDENT", token.lineno, token.column)
                        levels.pop()

            yield token

        # Must dedent any remaining levels
        if len(levels) > 1:
            assert token is not None
            for _ in range(1, len(levels)):
                yield self._new_token("DEDENT", token.lineno, token.column)

    def _indent_filter(self, add_endmarker=True):
        token = None
        tokens = iter(self.lexer.token, None)
        tokens = self._track_tokens_filter(tokens)
        for token in self._indentation_filter(tokens):
            yield token

        if add_endmarker:
            yield self._new_token(
                "EOF", *(token.lineno, token.column) if token else (1, 0)
            )

    def input(self, source, add_endmarker=True):
        self.lexer.paren_count = 0
        self.lexer.input(source)
        self.token_stream = self._indent_filter(add_endmarker)

    def token(self):
        try:
            return next(self.token_stream)
        except StopIteration:
            return None
