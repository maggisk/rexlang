import pytest
import sys, os

sys.path.insert(0, os.path.dirname(os.path.dirname(os.path.abspath(__file__))))

from rexlang import ast
from rexlang.parser import parse, parse_single, Error
from rexlang.lexer import Error as LexError


def expr(source):
    return parse_single(source)


class TestLiterals:
    def test_int(self):
        assert expr("1") == ast.Int(1)

    def test_float(self):
        assert expr("3.14") == ast.Float(3.14)

    def test_string(self):
        assert expr('"hi"') == ast.String("hi")

    def test_bool_true(self):
        assert expr("true") == ast.Bool(True)

    def test_bool_false(self):
        assert expr("false") == ast.Bool(False)

    def test_var(self):
        assert expr("x") == ast.Var("x")


class TestArithmetic:
    def test_addition(self):
        assert expr("1 + 2") == ast.Binop("Add", ast.Int(1), ast.Int(2))

    def test_subtraction(self):
        assert expr("3 - 1") == ast.Binop("Sub", ast.Int(3), ast.Int(1))

    def test_multiplication(self):
        assert expr("2 * 3") == ast.Binop("Mul", ast.Int(2), ast.Int(3))

    def test_division(self):
        assert expr("6 / 2") == ast.Binop("Div", ast.Int(6), ast.Int(2))

    def test_modulo(self):
        assert expr("10 % 3") == ast.Binop("Mod", ast.Int(10), ast.Int(3))

    def test_modulo_precedence(self):
        # 1 + 10 % 3  =>  1 + (10 % 3)
        assert expr("1 + 10 % 3") == ast.Binop(
            "Add", ast.Int(1), ast.Binop("Mod", ast.Int(10), ast.Int(3))
        )

    def test_concat(self):
        assert expr('"a" ++ "b"') == ast.Binop(
            "Concat", ast.String("a"), ast.String("b")
        )

    def test_precedence_mul_over_add(self):
        # 1 + 2 * 3  =>  1 + (2 * 3)
        assert expr("1 + 2 * 3") == ast.Binop(
            "Add", ast.Int(1), ast.Binop("Mul", ast.Int(2), ast.Int(3))
        )

    def test_left_associativity(self):
        # 1 + 2 + 3  =>  (1 + 2) + 3
        assert expr("1 + 2 + 3") == ast.Binop(
            "Add", ast.Binop("Add", ast.Int(1), ast.Int(2)), ast.Int(3)
        )

    def test_unary_minus(self):
        assert expr("-1") == ast.Unary_minus(ast.Int(1))

    def test_parens(self):
        assert expr("(1 + 2) * 3") == ast.Binop(
            "Mul", ast.Binop("Add", ast.Int(1), ast.Int(2)), ast.Int(3)
        )


class TestFloat:
    def test_float_literal(self):
        assert expr("3.14") == ast.Float(3.14)

    def test_float_add(self):
        assert expr("1.5 + 2.5") == ast.Binop("Add", ast.Float(1.5), ast.Float(2.5))

    def test_float_in_app(self):
        assert expr("f 3.14") == ast.App(ast.Var("f"), ast.Float(3.14))


class TestComparison:
    def test_less(self):
        assert expr("1 < 2") == ast.Binop("Lt", ast.Int(1), ast.Int(2))

    def test_greater(self):
        assert expr("2 > 1") == ast.Binop("Gt", ast.Int(2), ast.Int(1))

    def test_leq(self):
        assert expr("1 <= 1") == ast.Binop("Leq", ast.Int(1), ast.Int(1))

    def test_geq(self):
        assert expr("2 >= 1") == ast.Binop("Geq", ast.Int(2), ast.Int(1))

    def test_equal(self):
        assert expr("x == y") == ast.Binop("Eq", ast.Var("x"), ast.Var("y"))

    def test_not_equal(self):
        assert expr("x /= y") == ast.Binop("Neq", ast.Var("x"), ast.Var("y"))


class TestLogic:
    def test_and(self):
        assert expr("true && false") == ast.Binop(
            "And", ast.Bool(True), ast.Bool(False)
        )

    def test_or(self):
        assert expr("true || false") == ast.Binop("Or", ast.Bool(True), ast.Bool(False))

    def test_and_over_or(self):
        # true || false && true  =>  true || (false && true)
        assert expr("true || false && true") == ast.Binop(
            "Or", ast.Bool(True), ast.Binop("And", ast.Bool(False), ast.Bool(True))
        )

    def test_and_left_assoc(self):
        # a && b && c  =>  (a && b) && c
        assert expr("x && y && z") == ast.Binop(
            "And", ast.Binop("And", ast.Var("x"), ast.Var("y")), ast.Var("z")
        )


class TestPipe:
    def test_pipe(self):
        # x |> f  =>  App(f, x)
        assert expr("x |> f") == ast.App(ast.Var("f"), ast.Var("x"))

    def test_pipe_left_assoc(self):
        # x |> f |> g  =>  App(g, App(f, x))
        assert expr("x |> f |> g") == ast.App(
            ast.Var("g"), ast.App(ast.Var("f"), ast.Var("x"))
        )


class TestIf:
    def test_if(self):
        assert expr("if true then 1 else 2") == ast.If(
            ast.Bool(True), ast.Int(1), ast.Int(2)
        )


class TestLet:
    def test_let_in(self):
        assert expr("let x = 1 in x") == ast.Let("x", False, ast.Int(1), ast.Var("x"))

    def test_let_no_in(self):
        result = parse("let x = 42")
        assert result == [ast.Let("x", False, ast.Int(42), None)]

    def test_let_rec(self):
        node = expr("let rec f x = f x in f")
        assert isinstance(node, ast.Let)
        assert node.recursive is True
        assert node.name == "f"

    def test_let_with_params(self):
        # let f x = x  =>  let f = fun x -> x
        node = expr("let f x = x in f")
        assert node == ast.Let("f", False, ast.Fun("x", ast.Var("x")), ast.Var("f"))

    def test_let_multi_params(self):
        # let f x y = x  =>  let f = fun x -> fun y -> x
        node = expr("let f x y = x in f")
        assert node == ast.Let(
            "f", False, ast.Fun("x", ast.Fun("y", ast.Var("x"))), ast.Var("f")
        )


class TestFun:
    def test_fun_single_param(self):
        assert expr("fun x -> x") == ast.Fun("x", ast.Var("x"))

    def test_fun_multi_param(self):
        # fun x y -> x  =>  Fun(x, Fun(y, x))
        assert expr("fun x y -> x") == ast.Fun("x", ast.Fun("y", ast.Var("x")))

    def test_fun_no_param_raises(self):
        with pytest.raises(Error):
            parse_single("fun -> x")


class TestApp:
    def test_application(self):
        assert expr("f x") == ast.App(ast.Var("f"), ast.Var("x"))

    def test_application_left_assoc(self):
        # f x y  =>  App(App(f, x), y)
        assert expr("f x y") == ast.App(
            ast.App(ast.Var("f"), ast.Var("x")), ast.Var("y")
        )


class TestCase:
    def test_simple_case(self):
        node = expr("case x of\n  0 -> 1\n  _ -> 2")
        assert isinstance(node, ast.Match)
        assert node.scrutinee == ast.Var("x")
        assert len(node.arms) == 2
        assert node.arms[0] == (ast.PInt(0), ast.Int(1))
        assert node.arms[1] == (ast.PWild(), ast.Int(2))

    def test_case_two_arms(self):
        node = expr("case x of\n  0 -> 1\n  _ -> 2")
        assert isinstance(node, ast.Match)
        assert len(node.arms) == 2

    def test_case_ctor_pattern(self):
        node = expr("case x of\n  Some n -> n\n  None -> 0")
        assert node.arms[0][0] == ast.PCtor("Some", [ast.PVar("n")])
        assert node.arms[1][0] == ast.PCtor("None", [])


class TestTypeDecl:
    def test_nullary_ctors(self):
        result = parse("type Color = Red | Green | Blue")
        assert result == [
            ast.TypeDecl("Color", [], [("Red", []), ("Green", []), ("Blue", [])])
        ]

    def test_ctor_with_args(self):
        result = parse("type Option = None | Some int")
        assert result == [ast.TypeDecl("Option", [], [("None", []), ("Some", ["int"])])]

    def test_ctor_multi_arg(self):
        result = parse("type Pair = Pair int int")
        assert result == [ast.TypeDecl("Pair", [], [("Pair", ["int", "int"])])]

    def test_parametric_type(self):
        result = parse("type Maybe a = Nothing | Just a")
        assert result == [
            ast.TypeDecl("Maybe", ["a"], [("Nothing", []), ("Just", ["a"])])
        ]

    def test_lowercase_name_raises(self):
        with pytest.raises(Error, match="uppercase"):
            parse("type option = None | Some int")


class TestListLit:
    def test_empty_list(self):
        assert expr("[]") == ast.ListLit([])

    def test_singleton_list(self):
        assert expr("[1]") == ast.ListLit([ast.Int(1)])

    def test_list_three(self):
        assert expr("[1, 2, 3]") == ast.ListLit([ast.Int(1), ast.Int(2), ast.Int(3)])

    def test_cons_nil(self):
        assert expr("1 :: []") == ast.Binop("Cons", ast.Int(1), ast.ListLit([]))

    def test_cons_right_assoc(self):
        # 1 :: 2 :: []  =>  Cons(1, Cons(2, []))
        assert expr("1 :: 2 :: []") == ast.Binop(
            "Cons", ast.Int(1), ast.Binop("Cons", ast.Int(2), ast.ListLit([]))
        )

    def test_list_as_app_arg(self):
        assert expr("f [1, 2]") == ast.App(
            ast.Var("f"), ast.ListLit([ast.Int(1), ast.Int(2)])
        )


class TestListPatterns:
    def test_nil_pattern(self):
        node = expr("case x of\n  [] -> 0\n  _ -> 1")
        assert node.arms[0][0] == ast.PNil()

    def test_cons_pattern(self):
        node = expr("case x of\n  [h|t] -> h\n  _ -> 0")
        assert node.arms[0][0] == ast.PCons(ast.PVar("h"), ast.PVar("t"))

    def test_list_pattern_desugar(self):
        # [1, 2, 3] pattern => PCons(1, PCons(2, PCons(3, PNil)))
        node = expr("case x of\n  [1,2,3] -> 0\n  _ -> 1")
        assert node.arms[0][0] == ast.PCons(
            ast.PInt(1),
            ast.PCons(ast.PInt(2), ast.PCons(ast.PInt(3), ast.PNil())),
        )

    def test_cons_with_nil_tail(self):
        # [h|[]] => PCons(h, PNil)
        node = expr("case x of\n  [h|[]] -> h\n  _ -> 0")
        assert node.arms[0][0] == ast.PCons(ast.PVar("h"), ast.PNil())


class TestProgram:
    def test_multiple_exprs(self):
        result = parse("let x = 1\nlet y = 2")
        assert len(result) == 2

    def test_semi_raises(self):
        with pytest.raises(LexError):
            parse("1;; 2")


class TestErrorPosition:
    def test_parse_error_includes_position(self):
        with pytest.raises(Error, match=r"line \d+"):
            parse("let x =")  # hits eof unexpectedly


class TestImportExport:
    def test_import_with_namespace(self):
        node = parse_single("import std:List (map, filter)")
        assert node == ast.Import("std:List", ["map", "filter"])

    def test_import_bare_name(self):
        node = parse_single("import List (f, g)")
        assert node == ast.Import("List", ["f", "g"])

    def test_import_single_name(self):
        node = parse_single("import std:List (length)")
        assert node == ast.Import("std:List", ["length"])

    def test_export(self):
        node = parse_single("export map, filter, foldl")
        assert node == ast.Export(["map", "filter", "foldl"])

    def test_export_single(self):
        node = parse_single("export length")
        assert node == ast.Export(["length"])


class TestTuple:
    def test_tuple_literal(self):
        assert expr("(1, 2)") == ast.Tuple([ast.Int(1), ast.Int(2)])

    def test_tuple_three(self):
        assert expr("(1, 2, 3)") == ast.Tuple([ast.Int(1), ast.Int(2), ast.Int(3)])

    def test_grouping_unchanged(self):
        assert expr("(1 + 2)") == ast.Binop("Add", ast.Int(1), ast.Int(2))

    def test_nested_tuple(self):
        assert expr("((1, 2), 3)") == ast.Tuple(
            [ast.Tuple([ast.Int(1), ast.Int(2)]), ast.Int(3)]
        )

    def test_tuple_pattern_in_case(self):
        node = expr("case t of\n  (a, b) -> a")
        assert node.arms[0][0] == ast.PTuple([ast.PVar("a"), ast.PVar("b")])

    def test_let_tuple_in(self):
        node = expr("let (a, b) = t in a")
        assert isinstance(node, ast.LetPat)
        assert node.pat == ast.PTuple([ast.PVar("a"), ast.PVar("b")])
        assert node.in_expr == ast.Var("a")

    def test_let_tuple_toplevel(self):
        result = parse("let (a, b) = t")
        assert isinstance(result[0], ast.LetPat)
        assert result[0].in_expr is None


class TestMutualRecursion:
    def test_let_rec_and_parses(self):
        node = expr("let rec f x = g x and g x = f x in f")
        assert isinstance(node, ast.LetRec)
        assert len(node.bindings) == 2
        assert node.bindings[0][0] == "f"
        assert node.bindings[1][0] == "g"

    def test_let_rec_and_in_expr(self):
        node = expr("let rec f x = g x and g x = f x in f 0")
        assert isinstance(node, ast.LetRec)
        assert node.in_expr is not None

    def test_let_rec_and_toplevel(self):
        result = parse("let rec f x = g x\nand g x = f x")
        assert isinstance(result[0], ast.LetRec)
        assert result[0].in_expr is None

    def test_let_rec_three_way(self):
        node = expr("let rec a x = b x and b x = c x and c x = a x in a")
        assert isinstance(node, ast.LetRec)
        assert len(node.bindings) == 3

    def test_single_let_rec_unchanged(self):
        node = expr("let rec f x = f x in f")
        assert isinstance(node, ast.Let)
        assert node.recursive is True
