from .token import Token


class Error(Exception):
    pass


def tokenize(source: str) -> list:
    pos = 0
    n = len(source)
    tokens = []
    line_start = 0
    line = 1

    def skip_block_comment():
        nonlocal pos, line_start, line
        pos += 2  # skip '(' and '*'
        depth = 1
        while depth > 0:
            if pos >= n:
                raise Error(
                    f"unterminated comment at line {line}, col {pos - line_start + 1}"
                )
            c = source[pos]
            pos += 1
            if c == "\n":
                line += 1
                line_start = pos
            elif c == "(" and pos < n and source[pos] == "*":
                pos += 1
                depth += 1
            elif c == "*" and pos < n and source[pos] == ")":
                pos += 1
                depth -= 1

    def skip_whitespace_and_comments():
        nonlocal pos, line_start, line
        while True:
            while pos < n and source[pos] in " \n\t\r":
                if source[pos] == "\n":
                    pos += 1
                    line += 1
                    line_start = pos
                else:
                    pos += 1
            if pos + 1 < n and source[pos] == "-" and source[pos + 1] == "-":
                pos += 2
                while pos < n and source[pos] != "\n":
                    pos += 1
            elif pos + 1 < n and source[pos] == "(" and source[pos + 1] == "*":
                skip_block_comment()
            else:
                break

    def read_number():
        nonlocal pos
        start = pos
        while pos < n and source[pos].isdigit():
            pos += 1
        if pos < n and source[pos] == ".":
            pos += 1
            while pos < n and source[pos].isdigit():
                pos += 1
            return Token("float", float(source[start:pos]))
        return Token("int", int(source[start:pos]))

    def read_string():
        nonlocal pos
        pos += 1  # skip opening '"'
        buf = []
        while True:
            if pos >= n:
                raise Error(
                    f"unterminated string at line {line}, col {pos - line_start + 1}"
                )
            c = source[pos]
            pos += 1
            if c == '"':
                return Token("string", "".join(buf))
            elif c == "\\":
                if pos >= n:
                    raise Error(
                        f"unterminated string escape at line {line}, col {pos - line_start + 1}"
                    )
                esc = source[pos]
                pos += 1
                if esc == "n":
                    buf.append("\n")
                elif esc == "t":
                    buf.append("\t")
                elif esc == "\\":
                    buf.append("\\")
                elif esc == '"':
                    buf.append('"')
                else:
                    raise Error(
                        f"unknown escape: \\{esc} at line {line}, col {pos - line_start + 1}"
                    )
            else:
                buf.append(c)

    KEYWORDS = {
        "let",
        "rec",
        "and",
        "in",
        "if",
        "then",
        "else",
        "fun",
        "case",
        "type",
        "of",
        "import",
        "export",
        "as",
        "trait",
        "impl",
        "where",
        "test",
        "assert",
    }

    def read_ident():
        nonlocal pos
        start = pos
        while pos < n and (source[pos].isalnum() or source[pos] == "_"):
            pos += 1
        s = source[start:pos]
        if s in KEYWORDS:
            return Token(s)
        elif s == "true":
            return Token("bool", True)
        elif s == "false":
            return Token("bool", False)
        else:
            return Token("ident", s)

    while True:
        skip_whitespace_and_comments()
        if pos >= n:
            tokens.append(Token("eof"))
            break
        token_col = pos - line_start
        token_line = line
        c = source[pos]
        if c.isdigit():
            tok = read_number()
            tok.line = token_line
            tok.col = token_col
            tokens.append(tok)
        elif c == '"':
            tok = read_string()
            tok.line = token_line
            tok.col = token_col
            tokens.append(tok)
        elif c.isalpha() or c == "_":
            tok = read_ident()
            tok.line = token_line
            tok.col = token_col
            tokens.append(tok)
        elif c == "+":
            pos += 1
            if pos < n and source[pos] == "+":
                pos += 1
                tokens.append(Token("++", line=token_line, col=token_col))
            else:
                tokens.append(Token("+", line=token_line, col=token_col))
        elif c == "*":
            pos += 1
            tokens.append(Token("*", line=token_line, col=token_col))
        elif c == "%":
            pos += 1
            tokens.append(Token("%", line=token_line, col=token_col))
        elif c == "/":
            pos += 1
            if pos < n and source[pos] == "=":
                pos += 1
                tokens.append(Token("/=", line=token_line, col=token_col))
            else:
                tokens.append(Token("/", line=token_line, col=token_col))
        elif c == "=":
            pos += 1
            if pos < n and source[pos] == "=":
                pos += 1
                tokens.append(Token("==", line=token_line, col=token_col))
            else:
                tokens.append(Token("=", line=token_line, col=token_col))
        elif c == "(":
            pos += 1
            tokens.append(Token("(", line=token_line, col=token_col))
        elif c == ")":
            pos += 1
            tokens.append(Token(")", line=token_line, col=token_col))
        elif c == "|":
            pos += 1
            if pos < n and source[pos] == ">":
                pos += 1
                tokens.append(Token("|>", line=token_line, col=token_col))
            elif pos < n and source[pos] == "|":
                pos += 1
                tokens.append(Token("||", line=token_line, col=token_col))
            else:
                tokens.append(Token("|", line=token_line, col=token_col))
        elif c == "&":
            pos += 1
            if pos < n and source[pos] == "&":
                pos += 1
                tokens.append(Token("&&", line=token_line, col=token_col))
            else:
                raise Error(
                    f"unexpected character: & at line {token_line}, col {token_col + 1}"
                )
        elif c == "-":
            pos += 1
            if pos < n and source[pos] == ">":
                pos += 1
                tokens.append(Token("->", line=token_line, col=token_col))
            else:
                tokens.append(Token("-", line=token_line, col=token_col))
        elif c == "<":
            pos += 1
            if pos < n and source[pos] == "=":
                pos += 1
                tokens.append(Token("<=", line=token_line, col=token_col))
            else:
                tokens.append(Token("<", line=token_line, col=token_col))
        elif c == ">":
            pos += 1
            if pos < n and source[pos] == "=":
                pos += 1
                tokens.append(Token(">=", line=token_line, col=token_col))
            else:
                tokens.append(Token(">", line=token_line, col=token_col))
        elif c == "[":
            pos += 1
            tokens.append(Token("[", line=token_line, col=token_col))
        elif c == "]":
            pos += 1
            tokens.append(Token("]", line=token_line, col=token_col))
        elif c == ",":
            pos += 1
            tokens.append(Token(",", line=token_line, col=token_col))
        elif c == ":":
            pos += 1
            if pos < n and source[pos] == ":":
                pos += 1
                tokens.append(Token("::", line=token_line, col=token_col))
            else:
                tokens.append(Token(":", line=token_line, col=token_col))
        elif c == ".":
            pos += 1
            tokens.append(Token(".", line=token_line, col=token_col))
        elif c == ";":
            raise Error(
                f"unexpected character: ; at line {token_line}, col {token_col + 1}"
            )
        else:
            raise Error(
                f"unexpected character: {c} at line {token_line}, col {token_col + 1}"
            )

    return tokens
