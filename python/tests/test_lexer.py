import pytest
import sys, os

sys.path.insert(0, os.path.dirname(os.path.dirname(os.path.abspath(__file__))))

from rexlang.lexer import tokenize, Error
from rexlang.token import Token


def kinds(source):
    return [t.kind for t in tokenize(source)]


def tok(source):
    return tokenize(source)


class TestLiterals:
    def test_integer(self):
        assert tok("42") == [Token("int", 42), Token("eof")]

    def test_zero(self):
        assert tok("0") == [Token("int", 0), Token("eof")]

    def test_float(self):
        assert tok("3.14") == [Token("float", 3.14), Token("eof")]

    def test_float_no_frac(self):
        assert tok("5.0") == [Token("float", 5.0), Token("eof")]

    def test_string(self):
        assert tok('"hello"') == [Token("string", "hello"), Token("eof")]

    def test_string_escapes(self):
        assert tok(r'"a\nb"') == [Token("string", "a\nb"), Token("eof")]
        assert tok(r'"a\tb"') == [Token("string", "a\tb"), Token("eof")]
        assert tok(r'"a\\b"') == [Token("string", "a\\b"), Token("eof")]
        assert tok(r'"a\"b"') == [Token("string", 'a"b'), Token("eof")]

    def test_bool_true(self):
        assert tok("true") == [Token("bool", True), Token("eof")]

    def test_bool_false(self):
        assert tok("false") == [Token("bool", False), Token("eof")]


class TestKeywords:
    def test_all_keywords(self):
        src = "let rec in if then else fun case type of"
        expected_kinds = [
            "let",
            "rec",
            "in",
            "if",
            "then",
            "else",
            "fun",
            "case",
            "type",
            "of",
            "eof",
        ]
        assert kinds(src) == expected_kinds

    def test_import_keyword(self):
        assert kinds("import") == ["import", "eof"]

    def test_export_keyword(self):
        assert kinds("export") == ["export", "eof"]

    def test_import_statement(self):
        assert kinds("import std:List (map, filter)") == [
            "import",
            "ident",
            ":",
            "ident",
            "(",
            "ident",
            ",",
            "ident",
            ")",
            "eof",
        ]

    def test_export_statement(self):
        assert kinds("export map, filter") == ["export", "ident", ",", "ident", "eof"]


class TestOperators:
    def test_arithmetic(self):
        assert kinds("+ - * / %") == ["+", "-", "*", "/", "%", "eof"]

    def test_concat(self):
        assert kinds("++") == ["++", "eof"]

    def test_comparison(self):
        assert kinds("== /= < > <= >=") == ["==", "/=", "<", ">", "<=", ">=", "eof"]

    def test_single_eq(self):
        assert kinds("=") == ["=", "eof"]

    def test_logical(self):
        assert kinds("&& ||") == ["&&", "||", "eof"]

    def test_pipe(self):
        assert kinds("|>") == ["|>", "eof"]

    def test_arrow(self):
        assert kinds("->") == ["->", "eof"]

    def test_minus_vs_arrow(self):
        assert kinds("- >") == ["-", ">", "eof"]

    def test_single_amp_raises(self):
        with pytest.raises(Error, match="unexpected character"):
            tokenize("&")


class TestDelimiters:
    def test_parens(self):
        assert kinds("( )") == ["(", ")", "eof"]

    def test_pipe_single(self):
        assert kinds("|") == ["|", "eof"]

    def test_semi_raises(self):
        with pytest.raises(Error, match="unexpected character"):
            tokenize(";")

    def test_double_semi_raises(self):
        with pytest.raises(Error, match="unexpected character"):
            tokenize(";;")


class TestIdentifiers:
    def test_lowercase(self):
        assert tok("foo") == [Token("ident", "foo"), Token("eof")]

    def test_uppercase(self):
        assert tok("Foo") == [Token("ident", "Foo"), Token("eof")]

    def test_underscore(self):
        assert tok("_") == [Token("ident", "_"), Token("eof")]

    def test_mixed(self):
        assert tok("foo_Bar42") == [Token("ident", "foo_Bar42"), Token("eof")]


class TestWhitespaceAndComments:
    def test_whitespace_ignored(self):
        assert kinds("1  +\t2\n") == ["int", "+", "int", "eof"]

    def test_block_comment(self):
        assert kinds("1 (* comment *) + 2") == ["int", "+", "int", "eof"]

    def test_nested_block_comment(self):
        assert kinds("1 (* outer (* inner *) outer *) + 2") == [
            "int",
            "+",
            "int",
            "eof",
        ]

    def test_line_comment(self):
        assert kinds("1 -- this is a comment\n+ 2") == ["int", "+", "int", "eof"]

    def test_line_comment_end_of_input(self):
        assert kinds("42 -- no newline at end") == ["int", "eof"]

    def test_unterminated_block_comment(self):
        with pytest.raises(Error, match="unterminated comment"):
            tokenize("(* never closed")

    def test_unterminated_string(self):
        with pytest.raises(Error, match="unterminated string"):
            tokenize('"never closed')


class TestListTokens:
    def test_lbracket(self):
        assert kinds("[") == ["[", "eof"]

    def test_rbracket(self):
        assert kinds("]") == ["]", "eof"]

    def test_comma(self):
        assert kinds(",") == [",", "eof"]

    def test_cons(self):
        assert kinds("::") == ["::", "eof"]

    def test_list_literal(self):
        assert kinds("[1, 2, 3]") == ["[", "int", ",", "int", ",", "int", "]", "eof"]

    def test_cons_expr(self):
        assert kinds("1 :: []") == ["int", "::", "[", "]", "eof"]

    def test_single_colon(self):
        assert kinds(":") == [":", "eof"]

    def test_double_colon_still_works(self):
        assert kinds("::") == ["::", "eof"]


class TestUnexpectedChar:
    def test_unexpected_char(self):
        with pytest.raises(Error, match="unexpected character"):
            tokenize("@")


class TestPosition:
    def test_single_line(self):
        toks = tokenize("let x = 1")
        assert toks[0].line == 1 and toks[0].col == 0  # 'let'
        assert toks[1].line == 1 and toks[1].col == 4  # 'x'
        assert toks[2].line == 1 and toks[2].col == 6  # '='
        assert toks[3].line == 1 and toks[3].col == 8  # '1'

    def test_multiline(self):
        toks = tokenize("let x\n= 1")
        assert toks[0].line == 1  # 'let'
        assert toks[2].line == 2 and toks[2].col == 0  # '='
        assert toks[3].line == 2 and toks[3].col == 2  # '1'

    def test_lex_error_position(self):
        with pytest.raises(Error, match="line 1"):
            tokenize("@")

    def test_lex_error_multiline_position(self):
        with pytest.raises(Error, match="line 2"):
            tokenize("let x =\n@")
