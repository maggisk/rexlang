import pytest
import sys, os

sys.path.insert(0, os.path.dirname(os.path.dirname(os.path.abspath(__file__))))

from rexlang.eval import (
    run,
    run_program,
    eval_toplevel,
    VInt,
    VFloat,
    VString,
    VBool,
    VClosure,
    VCtor,
    VCtorFn,
    VBuiltin,
    VList,
    VTuple,
    Error,
    value_to_string,
)


def ev(source):
    """Evaluate a single expression and return the value."""
    return run(source)


def prog(source):
    """Run a program (multiple top-level expressions) and return the last value."""
    return run_program(source)


class TestLiterals:
    def test_int(self):
        assert ev("42") == VInt(42)

    def test_negative_int(self):
        assert ev("-7") == VInt(-7)

    def test_float(self):
        assert ev("3.14") == VFloat(3.14)

    def test_string(self):
        assert ev('"hello"') == VString("hello")

    def test_bool_true(self):
        assert ev("true") == VBool(True)

    def test_bool_false(self):
        assert ev("false") == VBool(False)


class TestArithmetic:
    def test_add(self):
        assert ev("1 + 2") == VInt(3)

    def test_sub(self):
        assert ev("5 - 3") == VInt(2)

    def test_mul(self):
        assert ev("3 * 4") == VInt(12)

    def test_div(self):
        assert ev("10 / 2") == VInt(5)

    def test_integer_division(self):
        assert ev("7 / 2") == VInt(3)

    def test_div_by_zero(self):
        with pytest.raises(Error, match="division by zero"):
            ev("1 / 0")

    def test_mod(self):
        assert ev("10 % 3") == VInt(1)

    def test_mod_even(self):
        assert ev("8 % 2") == VInt(0)

    def test_mod_by_zero(self):
        with pytest.raises(Error, match="modulo by zero"):
            ev("5 % 0")

    def test_precedence(self):
        assert ev("2 + 3 * 4") == VInt(14)

    def test_parens(self):
        assert ev("(2 + 3) * 4") == VInt(20)

    def test_unary_minus(self):
        assert ev("-(3 + 4)") == VInt(-7)

    def test_string_concat(self):
        assert ev('"foo" ++ "bar"') == VString("foobar")

    def test_add_int_float_type_error(self):
        with pytest.raises(Error, match="type error"):
            ev("1 + 1.5")

    def test_type_error(self):
        with pytest.raises(Error, match="type error"):
            ev("1 + true")


class TestFloat:
    def test_float_literal(self):
        assert ev("3.14") == VFloat(3.14)

    def test_float_add(self):
        assert ev("1.5 + 2.5") == VFloat(4.0)

    def test_float_sub(self):
        assert ev("3.5 - 1.5") == VFloat(2.0)

    def test_float_mul(self):
        assert ev("2.0 * 3.0") == VFloat(6.0)

    def test_float_div(self):
        assert ev("7.0 / 2.0") == VFloat(3.5)

    def test_float_lt(self):
        assert ev("1.5 < 2.5") == VBool(True)

    def test_float_eq(self):
        assert ev("1.5 == 1.5") == VBool(True)

    def test_unary_minus_float(self):
        assert ev("-3.14") == VFloat(-3.14)

    def test_sqrt(self):
        assert ev("sqrt 4.0") == VFloat(2.0)

    def test_to_float(self):
        assert ev("toFloat 3") == VFloat(3.0)

    def test_round(self):
        assert ev("round 3.7") == VInt(4)

    def test_floor(self):
        assert ev("floor 3.9") == VInt(3)

    def test_ceiling(self):
        assert ev("ceiling 3.1") == VInt(4)

    def test_truncate(self):
        assert ev("truncate 3.9") == VInt(3)


class TestLogic:
    def test_and_true(self):
        assert ev("true && true") == VBool(True)

    def test_and_false(self):
        assert ev("true && false") == VBool(False)

    def test_or_true(self):
        assert ev("false || true") == VBool(True)

    def test_or_false(self):
        assert ev("false || false") == VBool(False)

    def test_not_true(self):
        assert ev("not true") == VBool(False)

    def test_not_false(self):
        assert ev("not false") == VBool(True)


class TestPipe:
    def test_single_pipe(self):
        assert ev("5 |> (fun x -> x * 2)") == VInt(10)

    def test_chained_pipe(self):
        assert ev("5 |> (fun x -> x + 1) |> (fun x -> x * 2)") == VInt(12)


class TestComparison:
    def test_less_true(self):
        assert ev("1 < 2") == VBool(True)

    def test_less_false(self):
        assert ev("2 < 1") == VBool(False)

    def test_greater(self):
        assert ev("3 > 2") == VBool(True)

    def test_leq(self):
        assert ev("2 <= 2") == VBool(True)

    def test_geq(self):
        assert ev("3 >= 4") == VBool(False)

    def test_eq_int(self):
        assert ev("1 == 1") == VBool(True)

    def test_eq_int_false(self):
        assert ev("1 == 2") == VBool(False)

    def test_eq_string(self):
        assert ev('"a" == "a"') == VBool(True)

    def test_eq_bool(self):
        assert ev("true == true") == VBool(True)

    def test_neq(self):
        assert ev("1 /= 2") == VBool(True)


class TestIf:
    def test_then_branch(self):
        assert ev("if true then 1 else 2") == VInt(1)

    def test_else_branch(self):
        assert ev("if false then 1 else 2") == VInt(2)

    def test_condition_expression(self):
        assert ev("if 1 < 2 then 10 else 20") == VInt(10)

    def test_non_bool_condition(self):
        with pytest.raises(Error, match="expected bool"):
            ev("if 1 then 2 else 3")


class TestLet:
    def test_let_in(self):
        assert ev("let x = 5 in x") == VInt(5)

    def test_let_shadow(self):
        assert ev("let x = 1 in let x = 2 in x") == VInt(2)

    def test_let_body_uses_outer(self):
        assert ev("let x = 3 in let y = x + 1 in y") == VInt(4)

    def test_unbound_variable(self):
        with pytest.raises(Error, match="unbound variable"):
            ev("z")

    def test_top_level_let(self):
        result = prog("let x = 10\nx")
        assert result == VInt(10)

    def test_top_level_let_sequence(self):
        result = prog("let x = 3\nlet y = 4\nx + y")
        assert result == VInt(7)


class TestFunctions:
    def test_fun_identity(self):
        assert ev("(fun x -> x) 42") == VInt(42)

    def test_fun_add(self):
        assert ev("(fun x -> x + 1) 5") == VInt(6)

    def test_closure_captures_env(self):
        assert ev("let n = 10 in (fun x -> x + n) 5") == VInt(15)

    def test_currying(self):
        assert ev("(fun x -> fun y -> x + y) 3 4") == VInt(7)

    def test_higher_order(self):
        assert ev("let apply f x = f x in apply (fun n -> n * 2) 21") == VInt(42)

    def test_function_value(self):
        assert isinstance(ev("fun x -> x"), VClosure)

    def test_apply_non_function(self):
        with pytest.raises(Error, match="cannot apply"):
            ev("42 1")


class TestRecursion:
    def test_factorial(self):
        src = "let rec fact n = if n <= 1 then 1 else n * fact (n - 1) in fact 10"
        assert ev(src) == VInt(3628800)

    def test_fibonacci(self):
        src = "let rec fib n = if n <= 1 then n else fib (n-1) + fib (n-2) in fib 10"
        assert ev(src) == VInt(55)

    def test_mutual_style_via_top_level(self):
        src = "let rec fact n = if n <= 1 then 1 else n * fact (n - 1) in fact 5"
        assert ev(src) == VInt(120)

    def test_top_level_rec(self):
        src = "let rec fact n = if n <= 1 then 1 else n * fact (n - 1)\nfact 6"
        assert prog(src) == VInt(720)


class TestPatternMatching:
    def test_wildcard(self):
        assert ev("case 42 of _ -> 0") == VInt(0)

    def test_variable_pattern(self):
        assert ev("case 5 of n -> n + 1") == VInt(6)

    def test_int_pattern(self):
        assert ev("case 1 of\n  1 -> true\n  _ -> false") == VBool(True)

    def test_int_pattern_fallthrough(self):
        assert ev("case 2 of\n  1 -> true\n  _ -> false") == VBool(False)

    def test_bool_pattern(self):
        assert ev("case true of\n  true -> 1\n  false -> 0") == VInt(1)

    def test_string_pattern(self):
        assert ev('case "hi" of\n  "hi" -> 1\n  _ -> 0') == VInt(1)

    def test_match_failure(self):
        with pytest.raises(Error, match="match failure"):
            ev("case 5 of 1 -> 0")

    def test_first_arm_wins(self):
        assert ev("case 1 of\n  1 -> 10\n  1 -> 20") == VInt(10)


class TestADTs:
    def test_nullary_ctor(self):
        src = "type Color = Red | Green | Blue\nRed"
        assert prog(src) == VCtor("Red", [])

    def test_ctor_with_arg(self):
        src = "type Option = None | Some int\nSome 42"
        assert prog(src) == VCtor("Some", [VInt(42)])

    def test_match_ctor(self):
        src = """type Option = None | Some int
let x = Some 7 in
case x of
  None -> 0
  Some n -> n"""
        assert prog(src) == VInt(7)

    def test_match_nullary_ctor(self):
        src = "type Bool2 = Yes | No\ncase Yes of\n  Yes -> 1\n  No -> 0"
        assert prog(src) == VInt(1)

    def test_ctor_multi_arg(self):
        src = "type Pair = Pair int int\nPair 3 4"
        assert prog(src) == VCtor("Pair", [VInt(3), VInt(4)])

    def test_nested_ctor(self):
        src = """type List = Nil | Cons int List
let rec length xs =
  case xs of
  Nil -> 0
  Cons _ t -> 1 + length t
in
length (Cons 1 (Cons 2 (Cons 3 Nil)))"""
        assert prog(src) == VInt(3)


class TestLists:
    def test_empty_list(self):
        assert ev("[]") == VList([])

    def test_list_literal(self):
        assert ev("[1, 2, 3]") == VList([VInt(1), VInt(2), VInt(3)])

    def test_cons_onto_list(self):
        assert ev("1 :: [2, 3]") == VList([VInt(1), VInt(2), VInt(3)])

    def test_cons_chain(self):
        assert ev("1 :: 2 :: []") == VList([VInt(1), VInt(2)])

    def test_match_nil(self):
        assert ev("case [] of\n  [] -> 0\n  _ -> 1") == VInt(0)

    def test_match_cons(self):
        assert ev("case [1, 2, 3] of\n  [h|t] -> h\n  [] -> 0") == VInt(1)

    def test_match_tail(self):
        src = "case [1, 2, 3] of\n  [h|t] -> t\n  [] -> []"
        assert ev(src) == VList([VInt(2), VInt(3)])

    def test_cons_type_error(self):
        with pytest.raises(Error, match="type error"):
            ev("1 :: 2")

    def test_value_to_string_empty(self):
        assert value_to_string(VList([])) == "[]"

    def test_value_to_string_list(self):
        assert value_to_string(VList([VInt(1), VInt(2)])) == "[1, 2]"

    def test_recursive_sum(self):
        src = """let rec sum lst =
    case lst of
        [] -> 0
        [h|t] -> h + sum t
sum [1, 2, 3, 4, 5]"""
        assert prog(src) == VInt(15)


class TestValueToString:
    def test_int(self):
        assert value_to_string(VInt(42)) == "42"

    def test_float(self):
        assert value_to_string(VFloat(3.14)) == "3.14"

    def test_string_escaping(self):
        assert value_to_string(VString("hello")) == '"hello"'
        assert value_to_string(VString("a\nb")) == '"a\\nb"'

    def test_bool(self):
        assert value_to_string(VBool(True)) == "true"
        assert value_to_string(VBool(False)) == "false"

    def test_closure(self):
        assert value_to_string(VClosure("x", None, {})) == "<fun>"

    def test_builtin(self):
        assert value_to_string(VBuiltin("not", None)) == "<builtin not>"

    def test_ctor_no_args(self):
        assert value_to_string(VCtor("None", [])) == "None"

    def test_ctor_with_args(self):
        assert value_to_string(VCtor("Some", [VInt(1)])) == "(Some 1)"


class TestMutualRecursion:
    def test_is_even_is_odd(self):
        src = (
            "let rec isEven n = if n == 0 then true else isOdd (n - 1)\n"
            "and isOdd n = if n == 0 then false else isEven (n - 1)\n"
            "isEven 4"
        )
        assert prog(src) == VBool(True)

    def test_is_odd(self):
        src = (
            "let rec isEven n = if n == 0 then true else isOdd (n - 1)\n"
            "and isOdd n = if n == 0 then false else isEven (n - 1)\n"
            "isOdd 3"
        )
        assert prog(src) == VBool(True)

    def test_let_in_form(self):
        src = (
            "let rec isEven n = if n == 0 then true else isOdd (n - 1)\n"
            "and isOdd n = if n == 0 then false else isEven (n - 1)\n"
            "in isEven 6"
        )
        assert ev(src) == VBool(True)

    def test_three_way(self):
        src = (
            "let rec f n = if n == 0 then 0 else g (n - 1)\n"
            "and g n = if n == 0 then 0 else h (n - 1)\n"
            "and h n = if n == 0 then 42 else f (n - 1)\n"
            "f 2"
        )
        assert prog(src) == VInt(42)


class TestExampleFiles:
    """Run the .rex example files end-to-end."""

    EXAMPLES = os.path.join(os.path.dirname(__file__), "..", "..", "examples")

    def _run(self, filename):
        path = os.path.join(self.EXAMPLES, filename)
        with open(path) as f:
            source = f.read()
        return run_program(source)

    def test_factorial(self):
        assert self._run("factorial.rex") == VInt(3628800)

    def test_fibonacci(self):
        assert self._run("fibonacci.rex") == VInt(6765)

    def test_adt(self):
        assert self._run("adt.rex") == VInt(3)

    def test_pattern_match(self):
        assert self._run("pattern_match.rex") == VBool(True)

    def test_higher_order(self):
        assert self._run("higher_order.rex") == VInt(42)

    def test_floats(self):
        result = self._run("floats.rex")
        assert isinstance(result, VFloat)
        assert abs(result.value - 78.53975) < 1e-6

    def test_pipe(self):
        assert self._run("pipe.rex") == VInt(64)

    def test_modulo(self):
        assert self._run("modulo.rex") == VBool(True)

    def test_list(self):
        assert self._run("list.rex") == VInt(15)

    def test_import(self):
        assert self._run("import.rex") == VInt(30)

    def test_math(self):
        assert self._run("math.rex") == VInt(10)

    def test_string(self):
        assert self._run("string.rex") == VString("hello-world-rex")

    def test_tuple(self):
        assert self._run("tuple.rex") == VInt(30)

    def test_mutual_recursion(self):
        assert self._run("mutual_recursion.rex") == VBool(True)

    def test_io(self, capsys):
        result = self._run("io.rex")
        assert result == VString("Hello, RexLang!")
        capsys.readouterr()  # discard printed output

    def test_maybe(self):
        assert self._run("maybe.rex") == VInt(7)


class TestMathStdlib:
    def test_abs_int(self):
        assert prog("import std:Math (abs)\nabs (-5)") == VInt(5)

    def test_abs_float(self):
        assert prog("import std:Math (abs)\nabs (-3.14)") == VFloat(3.14)

    def test_min(self):
        assert prog("import std:Math (min)\nmin 3 5") == VInt(3)

    def test_max(self):
        assert prog("import std:Math (max)\nmax 3 5") == VInt(5)

    def test_pow(self):
        assert prog("import std:Math (pow)\npow 2.0 10.0") == VFloat(1024.0)

    def test_sin_pi(self):
        result = prog("import std:Math (sin, pi)\nsin pi")
        assert isinstance(result, VFloat) and abs(result.value) < 1e-10

    def test_clamp(self):
        assert prog("import std:Math (clamp)\nclamp 0 10 15") == VInt(10)

    def test_degrees(self):
        result = prog("import std:Math (degrees, pi)\ndegrees pi")
        assert isinstance(result, VFloat) and abs(result.value - 180.0) < 1e-10

    def test_logBase(self):
        result = prog("import std:Math (logBase)\nlogBase 10.0 100.0")
        assert isinstance(result, VFloat) and abs(result.value - 2.0) < 1e-10


class TestStringStdlib:
    def test_length(self):
        assert prog('import std:String (length)\nlength "hello"') == VInt(5)

    def test_to_upper(self):
        assert prog('import std:String (toUpper)\ntoUpper "hello"') == VString("HELLO")

    def test_to_lower(self):
        assert prog('import std:String (toLower)\ntoLower "HELLO"') == VString("hello")

    def test_trim(self):
        assert prog('import std:String (trim)\ntrim "  hi  "') == VString("hi")

    def test_split(self):
        assert prog('import std:String (split)\nsplit "," "a,b,c"') == VList(
            [VString("a"), VString("b"), VString("c")]
        )

    def test_join(self):
        assert prog('import std:String (join)\njoin "-" ["a", "b", "c"]') == VString(
            "a-b-c"
        )

    def test_to_string_int(self):
        assert prog("import std:String (toString)\ntoString 42") == VString("42")

    def test_contains(self):
        assert prog('import std:String (contains)\ncontains "ell" "hello"') == VBool(
            True
        )

    def test_starts_with(self):
        assert prog('import std:String (startsWith)\nstartsWith "he" "hello"') == VBool(
            True
        )

    def test_ends_with(self):
        assert prog('import std:String (endsWith)\nendsWith "lo" "hello"') == VBool(
            True
        )

    def test_is_empty(self):
        assert prog('import std:String (isEmpty)\nisEmpty ""') == VBool(True)


class TestImport:
    def test_length(self):
        assert prog("import std:List (length)\nlength [1,2,3]") == VInt(3)

    def test_map(self):
        assert prog("import std:List (map)\nmap (fun x -> x * 2) [1,2,3]") == VList(
            [VInt(2), VInt(4), VInt(6)]
        )

    def test_filter(self):
        assert prog(
            "import std:List (filter)\nfilter (fun x -> x > 2) [1,2,3,4]"
        ) == VList([VInt(3), VInt(4)])

    def test_foldl(self):
        assert prog(
            "import std:List (foldl)\nfoldl (fun acc x -> acc + x) 0 [1,2,3]"
        ) == VInt(6)

    def test_sum(self):
        assert prog("import std:List (sum)\nsum [1,2,3,4,5]") == VInt(15)

    def test_head(self):
        assert prog("import std:List (head)\nhead [1,2,3]") == VInt(1)

    def test_take(self):
        assert prog("import std:List (take)\ntake 2 [1,2,3,4]") == VList(
            [VInt(1), VInt(2)]
        )

    def test_unknown_namespace_raises(self):
        with pytest.raises(Error):
            prog("import foo:List (x)\nx")

    def test_bare_name_raises(self):
        with pytest.raises(Error):
            prog("import List (x)\nx")

    def test_non_exported_name_raises(self):
        with pytest.raises(Error):
            prog("import std:List (nonexistent)\nnonexistent")


class TestTuple:
    def test_tuple_value(self):
        assert ev("(1, 2)") == VTuple([VInt(1), VInt(2)])

    def test_tuple_three(self):
        assert ev("(1, 2, 3)") == VTuple([VInt(1), VInt(2), VInt(3)])

    def test_let_tuple_in(self):
        assert ev("let (a, b) = (10, 20) in a + b") == VInt(30)

    def test_let_tuple_toplevel(self):
        assert prog("let (a, b) = (1, 2)") == VTuple([VInt(1), VInt(2)])

    def test_let_tuple_binds(self):
        assert prog("let (a, b) = (3, 4)\na + b") == VInt(7)

    def test_case_tuple_pattern(self):
        assert ev("case (1, 2) of\n  (a, b) -> a + b") == VInt(3)

    def test_nested_tuple(self):
        assert ev("let (a, b) = ((1, 2), 3) in b") == VInt(3)

    def test_wildcard_in_tuple(self):
        assert ev("let (a, _) = (10, 99) in a") == VInt(10)


class TestExhaustiveness:
    def test_wildcard_is_exhaustive(self):
        ev("case 42 of _ -> 0")

    def test_var_is_exhaustive(self):
        ev("case 42 of n -> n")

    def test_bool_missing_false(self):
        with pytest.raises(Error, match="non-exhaustive"):
            ev("case true of\n  true -> 1")

    def test_bool_missing_true(self):
        with pytest.raises(Error, match="non-exhaustive"):
            ev("case false of\n  false -> 0")

    def test_bool_exhaustive(self):
        ev("case true of\n  true -> 1\n  false -> 0")

    def test_list_missing_cons(self):
        with pytest.raises(Error, match="non-exhaustive"):
            ev("case [] of\n  [] -> 0")

    def test_list_missing_nil(self):
        with pytest.raises(Error, match="non-exhaustive"):
            ev("case [1, 2] of\n  [h|t] -> h")

    def test_list_exhaustive(self):
        ev("case [] of\n  [] -> 0\n  [h|t] -> h")

    def test_adt_missing_ctor(self):
        src = "type Color = Red | Green | Blue\ncase Red of\n  Red -> 1\n  Green -> 2"
        with pytest.raises(Error, match="non-exhaustive"):
            prog(src)

    def test_adt_exhaustive(self):
        src = "type Color = Red | Green | Blue\ncase Red of\n  Red -> 1\n  Green -> 2\n  Blue -> 3"
        assert prog(src) == VInt(1)

    def test_adt_wildcard_exhaustive(self):
        src = "type Color = Red | Green | Blue\ncase Red of\n  Red -> 1\n  _ -> 0"
        assert prog(src) == VInt(1)

    def test_adt_missing_in_function(self):
        src = (
            "type Option = None | Some int\n"
            "let f x = case x of\n"
            "    Some n -> n\n"
            "f (Some 42)"
        )
        with pytest.raises(Error, match="non-exhaustive"):
            prog(src)

    def test_int_literals_no_check(self):
        # int-only patterns: no exhaustiveness check (infinite domain)
        with pytest.raises(Error, match="match failure"):
            ev("case 5 of\n  1 -> 0")


class TestIO:
    def test_print_string(self, capsys):
        result = ev('print "hello"')
        assert result == VString("hello")
        assert capsys.readouterr().out == "hello"

    def test_print_int(self, capsys):
        result = ev("print 42")
        assert result == VInt(42)
        assert capsys.readouterr().out == "42"

    def test_print_returns_value(self, capsys):
        assert ev('print "x"') == VString("x")
        capsys.readouterr()  # discard output

    def test_println_string(self, capsys):
        result = ev('println "hello"')
        assert result == VString("hello")
        assert capsys.readouterr().out == "hello\n"

    def test_println_int(self, capsys):
        result = ev("println 42")
        assert result == VInt(42)
        assert capsys.readouterr().out == "42\n"

    def test_println_bool(self, capsys):
        result = ev("println true")
        assert result == VBool(True)
        assert capsys.readouterr().out == "true\n"

    def test_println_returns_value(self, capsys):
        assert ev('println "x"') == VString("x")
        capsys.readouterr()  # discard output

    def test_readline(self, monkeypatch):
        monkeypatch.setattr("builtins.input", lambda prompt: "test input")
        result = ev('readLine "Enter: "')
        assert result == VString("test input")

    def test_readline_eof(self, monkeypatch):
        def raise_eof(prompt):
            raise EOFError

        monkeypatch.setattr("builtins.input", raise_eof)
        result = ev('readLine ""')
        assert result == VString("")


class TestMaybeStdlib:
    def test_import_nothing(self):
        src = 'import std:Maybe (Nothing, Just)\nNothing'
        assert prog(src) == VCtor("Nothing", [])

    def test_import_just(self):
        src = 'import std:Maybe (Nothing, Just)\nJust 42'
        assert prog(src) == VCtor("Just", [VInt(42)])

    def test_import_fromMaybe(self):
        src = 'import std:Maybe (Nothing, Just, fromMaybe)\nfromMaybe 0 (Just 7)'
        assert prog(src) == VInt(7)

    def test_import_isNothing(self):
        src = 'import std:Maybe (Nothing, Just, isNothing)\nisNothing Nothing'
        assert prog(src) == VBool(True)

    def test_import_map(self):
        src = 'import std:Maybe (Nothing, Just, map)\nmap (fun x -> x * 2) (Just 5)'
        assert prog(src) == VCtor("Just", [VInt(10)])

    def test_exhaustive_check_works(self):
        # exhaustiveness check should work for imported constructors
        src = (
            'import std:Maybe (Nothing, Just)\n'
            'case Just 5 of\n'
            '  Just n -> n'
        )
        with pytest.raises(Error, match="non-exhaustive"):
            prog(src)
