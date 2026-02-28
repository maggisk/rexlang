import pytest
import sys, os, tempfile

sys.path.insert(0, os.path.dirname(os.path.dirname(os.path.abspath(__file__))))

from rexlang.eval import (
    run,
    run_program,
    run_tests,
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
    VUnit,
    VModule,
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
        assert prog("import std:Math (sqrt)\nsqrt 4.0") == VFloat(2.0)

    def test_to_float(self):
        assert prog("import std:Math (toFloat)\ntoFloat 3") == VFloat(3.0)

    def test_round(self):
        assert prog("import std:Math (round)\nround 3.7") == VInt(4)

    def test_floor(self):
        assert prog("import std:Math (floor)\nfloor 3.9") == VInt(3)

    def test_ceiling(self):
        assert prog("import std:Math (ceiling)\nceiling 3.1") == VInt(4)

    def test_truncate(self):
        assert prog("import std:Math (truncate)\ntruncate 3.9") == VInt(3)


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

    def test_int_literals_no_check(self):
        # int-only patterns: no exhaustiveness check (infinite domain)
        with pytest.raises(Error, match="match failure"):
            ev("case 5 of\n  1 -> 0")


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

    def test_traits(self):
        assert self._run("traits.rex") == VString("less positive zero no")

    def test_map(self):
        assert self._run("map.rex") == VInt(60)


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


class TestIO:
    def test_print_string(self, capsys):
        result = prog('import std:IO (print)\nprint "hello"')
        assert result == VString("hello")
        assert capsys.readouterr().out == "hello"

    def test_print_int(self, capsys):
        result = prog("import std:IO (print)\nprint 42")
        assert result == VInt(42)
        assert capsys.readouterr().out == "42"

    def test_print_returns_value(self, capsys):
        assert prog('import std:IO (print)\nprint "x"') == VString("x")
        capsys.readouterr()  # discard output

    def test_println_string(self, capsys):
        result = prog('import std:IO (println)\nprintln "hello"')
        assert result == VString("hello")
        assert capsys.readouterr().out == "hello\n"

    def test_println_int(self, capsys):
        result = prog("import std:IO (println)\nprintln 42")
        assert result == VInt(42)
        assert capsys.readouterr().out == "42\n"

    def test_println_bool(self, capsys):
        result = prog("import std:IO (println)\nprintln true")
        assert result == VBool(True)
        assert capsys.readouterr().out == "true\n"

    def test_println_returns_value(self, capsys):
        assert prog('import std:IO (println)\nprintln "x"') == VString("x")
        capsys.readouterr()  # discard output

    def test_readline(self, monkeypatch):
        monkeypatch.setattr("builtins.input", lambda prompt: "test input")
        result = prog('import std:IO (readLine)\nreadLine "Enter: "')
        assert result == VString("test input")

    def test_readline_eof(self, monkeypatch):
        def raise_eof(prompt):
            raise EOFError

        monkeypatch.setattr("builtins.input", raise_eof)
        result = prog('import std:IO (readLine)\nreadLine ""')
        assert result == VString("")


class TestMaybeStdlib:
    def test_import_nothing(self):
        src = "import std:Maybe (Nothing, Just)\nNothing"
        assert prog(src) == VCtor("Nothing", [])

    def test_import_just(self):
        src = "import std:Maybe (Nothing, Just)\nJust 42"
        assert prog(src) == VCtor("Just", [VInt(42)])

    def test_import_fromMaybe(self):
        src = "import std:Maybe (Nothing, Just, fromMaybe)\nfromMaybe 0 (Just 7)"
        assert prog(src) == VInt(7)

    def test_import_isNothing(self):
        src = "import std:Maybe (Nothing, Just, isNothing)\nisNothing Nothing"
        assert prog(src) == VBool(True)

    def test_import_map(self):
        src = "import std:Maybe (Nothing, Just, map)\nmap (fun x -> x * 2) (Just 5)"
        assert prog(src) == VCtor("Just", [VInt(10)])

    def test_andThen_just(self):
        src = "import std:Maybe (Nothing, Just, andThen)\nandThen (fun x -> Just (x * 2)) (Just 5)"
        assert prog(src) == VCtor("Just", [VInt(10)])

    def test_andThen_returns_nothing(self):
        src = "import std:Maybe (Nothing, Just, andThen)\nandThen (fun x -> Nothing) (Just 5)"
        assert prog(src) == VCtor("Nothing", [])

    def test_andThen_nothing_input(self):
        src = "import std:Maybe (Nothing, Just, andThen)\nandThen (fun x -> Just (x * 2)) Nothing"
        assert prog(src) == VCtor("Nothing", [])


class TestQualifiedImports:
    def test_length_via_alias(self):
        src = "import std:List as L\nL.length [1,2,3]"
        assert prog(src) == VInt(3)

    def test_map_via_alias(self):
        src = "import std:List as L\nL.map (fun x -> x * 2) [1,2,3]"
        assert prog(src) == VList([VInt(2), VInt(4), VInt(6)])

    def test_collision_resolved(self):
        # L.length = list length, S.length = string length — no collision
        src = "import std:List as L\nimport std:String as S\nL.length [1,2]"
        assert prog(src) == VInt(2)
        src2 = 'import std:List as L\nimport std:String as S\nS.length "hi"'
        assert prog(src2) == VInt(2)

    def test_nonexistent_field_raises(self):
        src = "import std:List as L\nL.nonexistent"
        with pytest.raises(Error, match="does not export"):
            prog(src)

    def test_non_module_value_raises(self):
        src = "let x = 42\nx.foo"
        with pytest.raises(Error, match="is not a module"):
            prog(src)

    def test_unimported_alias_raises(self):
        src = "Z.length [1,2,3]"
        with pytest.raises(Error, match="is not a module"):
            prog(src)


class TestUnit:
    def test_unit_value(self):
        assert ev("()") == VUnit()

    def test_unit_in_let(self):
        assert prog("let x = ()\nx") == VUnit()

    def test_unit_pattern(self):
        src = "case () of\n    () ->\n        42"
        assert ev(src) == VInt(42)

    def test_unit_as_ctor_arg(self):
        src = (
            "import std:Result (Ok, Err)\n"
            "case Ok () of\n"
            "    Ok () ->\n"
            "        1\n"
            "    Err _ ->\n"
            "        2"
        )
        assert prog(src) == VInt(1)


class TestResultStdlib:
    def test_ok(self):
        assert prog("import std:Result (Ok, Err)\nOk 42") == VCtor("Ok", [VInt(42)])

    def test_err(self):
        assert prog('import std:Result (Ok, Err)\nErr "oops"') == VCtor(
            "Err", [VString("oops")]
        )

    def test_map_ok(self):
        src = "import std:Result (Ok, Err, map)\nmap (fun x -> x * 2) (Ok 5)"
        assert prog(src) == VCtor("Ok", [VInt(10)])

    def test_map_err(self):
        src = 'import std:Result (Ok, Err, map)\nmap (fun x -> x * 2) (Err "oops")'
        assert prog(src) == VCtor("Err", [VString("oops")])

    def test_mapErr(self):
        src = 'import std:Result (Ok, Err, mapErr)\nmapErr (fun e -> "error: " ++ e) (Err "oops")'
        assert prog(src) == VCtor("Err", [VString("error: oops")])

    def test_withDefault_ok(self):
        src = "import std:Result (Ok, Err, withDefault)\nwithDefault 0 (Ok 42)"
        assert prog(src) == VInt(42)

    def test_withDefault_err(self):
        src = 'import std:Result (Ok, Err, withDefault)\nwithDefault 0 (Err "oops")'
        assert prog(src) == VInt(0)

    def test_isOk_true(self):
        assert prog("import std:Result (Ok, Err, isOk)\nisOk (Ok 1)") == VBool(True)

    def test_isOk_false(self):
        assert prog('import std:Result (Ok, Err, isOk)\nisOk (Err "x")') == VBool(False)

    def test_isErr_true(self):
        assert prog('import std:Result (Ok, Err, isErr)\nisErr (Err "x")') == VBool(
            True
        )

    def test_andThen_ok(self):
        src = (
            "import std:Result (Ok, Err, andThen)\nandThen (fun x -> Ok (x * 2)) (Ok 5)"
        )
        assert prog(src) == VCtor("Ok", [VInt(10)])

    def test_andThen_err_passthrough(self):
        src = 'import std:Result (Ok, Err, andThen)\nandThen (fun x -> Ok (x * 2)) (Err "oops")'
        assert prog(src) == VCtor("Err", [VString("oops")])

    def test_andThen_returns_err(self):
        src = (
            'import std:Result (Ok, Err, andThen)\nandThen (fun x -> Err "nope") (Ok 5)'
        )
        assert prog(src) == VCtor("Err", [VString("nope")])


class TestIOStdlib:
    def test_read_file(self):
        with tempfile.NamedTemporaryFile(mode="w", suffix=".txt", delete=False) as f:
            f.write("hello")
            path = f.name
        try:
            src = f'import std:IO (readFile)\nreadFile "{path}"'
            assert prog(src) == VCtor("Ok", [VString("hello")])
        finally:
            os.unlink(path)

    def test_read_file_not_found(self):
        src = 'import std:IO (readFile)\nreadFile "/nonexistent/rexlang_xyz.txt"'
        result = prog(src)
        assert isinstance(result, VCtor) and result.name == "Err"

    def test_write_file(self):
        with tempfile.NamedTemporaryFile(suffix=".txt", delete=False) as f:
            path = f.name
        try:
            src = f'import std:IO (writeFile)\nwriteFile "{path}" "world"'
            assert prog(src) == VCtor("Ok", [VUnit()])
            with open(path) as f:
                assert f.read() == "world"
        finally:
            os.unlink(path)

    def test_append_file(self):
        with tempfile.NamedTemporaryFile(mode="w", suffix=".txt", delete=False) as f:
            f.write("hello")
            path = f.name
        try:
            src = f'import std:IO (appendFile)\nappendFile "{path}" " world"'
            assert prog(src) == VCtor("Ok", [VUnit()])
            with open(path) as f:
                assert f.read() == "hello world"
        finally:
            os.unlink(path)

    def test_file_exists_true(self):
        with tempfile.NamedTemporaryFile(delete=False) as f:
            path = f.name
        try:
            src = f'import std:IO (fileExists)\nfileExists "{path}"'
            assert prog(src) == VBool(True)
        finally:
            os.unlink(path)

    def test_file_exists_false(self):
        src = 'import std:IO (fileExists)\nfileExists "/nonexistent/rexlang_xyz123"'
        assert prog(src) == VBool(False)

    def test_list_dir(self):
        with tempfile.TemporaryDirectory() as d:
            open(os.path.join(d, "a.txt"), "w").close()
            open(os.path.join(d, "b.txt"), "w").close()
            src = f'import std:IO (listDir)\nlistDir "{d}"'
            assert prog(src) == VCtor(
                "Ok", [VList([VString("a.txt"), VString("b.txt")])]
            )

    def test_qualified_import(self):
        with tempfile.NamedTemporaryFile(mode="w", suffix=".txt", delete=False) as f:
            f.write("data")
            path = f.name
        try:
            src = f'import std:IO as IO\nIO.readFile "{path}"'
            assert prog(src) == VCtor("Ok", [VString("data")])
        finally:
            os.unlink(path)


class TestEnvStdlib:
    def test_get_env_existing(self):
        os.environ["REX_TEST_VAR"] = "hello"
        try:
            src = 'import std:Env (getEnv)\ngetEnv "REX_TEST_VAR"'
            assert prog(src) == VCtor("Just", [VString("hello")])
        finally:
            del os.environ["REX_TEST_VAR"]

    def test_get_env_missing(self):
        src = 'import std:Env (getEnv)\ngetEnv "REX_NONEXISTENT_XYZ_123"'
        assert prog(src) == VCtor("Nothing", [])

    def test_get_env_or_existing(self):
        os.environ["REX_TEST_VAR2"] = "found"
        try:
            src = 'import std:Env (getEnvOr)\ngetEnvOr "REX_TEST_VAR2" "default"'
            assert prog(src) == VString("found")
        finally:
            del os.environ["REX_TEST_VAR2"]

    def test_get_env_or_missing(self):
        src = 'import std:Env (getEnvOr)\ngetEnvOr "REX_NONEXISTENT_XYZ_123" "fallback"'
        assert prog(src) == VString("fallback")

    def test_args_is_list_of_strings(self):
        src = "import std:Env (args)\nargs"
        result = prog(src)
        assert isinstance(result, VList)
        assert all(isinstance(v, VString) for v in result.items)

    def test_qualified_import(self):
        os.environ["REX_TEST_VAR3"] = "qualified"
        try:
            src = 'import std:Env as Env\nEnv.getEnv "REX_TEST_VAR3"'
            assert prog(src) == VCtor("Just", [VString("qualified")])
        finally:
            del os.environ["REX_TEST_VAR3"]


# ---------------------------------------------------------------------------
# Traits
# ---------------------------------------------------------------------------


class TestTraits:
    def test_compare_int(self):
        assert prog("compare 3 5") == VCtor("LT", [])
        assert prog("compare 5 5") == VCtor("EQ", [])
        assert prog("compare 7 3") == VCtor("GT", [])

    def test_compare_float(self):
        assert prog("compare 1.0 2.0") == VCtor("LT", [])

    def test_compare_string(self):
        assert prog('compare "abc" "xyz"') == VCtor("LT", [])
        assert prog('compare "hello" "hello"') == VCtor("EQ", [])

    def test_compare_bool(self):
        assert prog("compare false true") == VCtor("LT", [])
        assert prog("compare true true") == VCtor("EQ", [])

    def test_eq_int(self):
        assert prog("eq 3 3") == VBool(True)
        assert prog("eq 3 4") == VBool(False)

    def test_eq_string(self):
        assert prog('eq "hello" "hello"') == VBool(True)
        assert prog('eq "hello" "world"') == VBool(False)

    def test_eq_bool(self):
        assert prog("eq true true") == VBool(True)
        assert prog("eq true false") == VBool(False)

    def test_eq_float(self):
        assert prog("eq 1.0 1.0") == VBool(True)

    def test_ordering_constructors(self):
        assert prog("LT") == VCtor("LT", [])
        assert prog("EQ") == VCtor("EQ", [])
        assert prog("GT") == VCtor("GT", [])

    def test_custom_trait(self):
        src = """
trait Greet a where
    greet : a -> String

impl Greet Int where
    greet x = "hi int"

impl Greet String where
    greet x = "hi string"

greet 42
"""
        assert prog(src) == VString("hi int")

    def test_custom_trait_string_dispatch(self):
        src = """
trait Greet a where
    greet : a -> String

impl Greet Int where
    greet x = "hi int"

impl Greet String where
    greet x = "hi string"

greet "world"
"""
        assert prog(src) == VString("hi string")

    def test_missing_instance_error(self):
        src = """
trait Foo a where
    foo : a -> a

impl Foo Int where
    foo x = x

foo "oops"
"""
        with pytest.raises(Error, match="no Foo instance for String"):
            prog(src)

    def test_trait_in_higher_order(self):
        src = """
let apply f x = f x
apply compare 3
"""
        # compare 3 returns a closure waiting for second arg
        result = prog(src)
        assert isinstance(result, VClosure)

    def test_trait_method_in_let(self):
        src = """
let result = compare 1 2
result
"""
        assert prog(src) == VCtor("LT", [])

    def test_string_ordering_operators(self):
        assert prog('"a" < "b"') == VBool(True)
        assert prog('"z" > "a"') == VBool(True)
        assert prog('"a" <= "a"') == VBool(True)
        assert prog('"b" >= "a"') == VBool(True)

    def test_bool_ordering_operators(self):
        assert prog("false < true") == VBool(True)
        assert prog("true > false") == VBool(True)


# ---------------------------------------------------------------------------
# Map stdlib
# ---------------------------------------------------------------------------


class TestMapStdlib:
    def test_empty_is_empty(self):
        assert prog("import std:Map (empty, isEmpty)\nisEmpty empty") == VBool(True)

    def test_singleton(self):
        assert prog("import std:Map (singleton, size)\nsize (singleton 1 10)") == VInt(
            1
        )

    def test_insert_lookup(self):
        src = "import std:Map (empty, insert, lookup)\nlet m = insert 1 10 empty\nlookup 1 m"
        assert prog(src) == VCtor("Just", [VInt(10)])

    def test_lookup_missing(self):
        src = "import std:Map (empty, insert, lookup)\nlet m = insert 1 10 empty\nlookup 2 m"
        assert prog(src) == VCtor("Nothing", [])

    def test_insert_overwrite(self):
        src = "import std:Map (empty, insert, lookup)\nlet m = insert 1 99 (insert 1 10 empty)\nlookup 1 m"
        assert prog(src) == VCtor("Just", [VInt(99)])

    def test_member_true(self):
        src = "import std:Map (empty, insert, member)\nmember 1 (insert 1 10 empty)"
        assert prog(src) == VBool(True)

    def test_member_false(self):
        src = "import std:Map (empty, insert, member)\nmember 2 (insert 1 10 empty)"
        assert prog(src) == VBool(False)

    def test_size(self):
        src = "import std:Map (empty, insert, size)\nsize (insert 3 30 (insert 2 20 (insert 1 10 empty)))"
        assert prog(src) == VInt(3)

    def test_isEmpty_nonempty(self):
        src = "import std:Map (singleton, isEmpty)\nisEmpty (singleton 1 10)"
        assert prog(src) == VBool(False)

    def test_remove(self):
        src = "import std:Map (empty, insert, remove, member)\nlet m = insert 2 20 (insert 1 10 empty)\nmember 1 (remove 1 m)"
        assert prog(src) == VBool(False)

    def test_remove_missing(self):
        src = "import std:Map (empty, insert, remove, size)\nlet m = insert 1 10 empty\nsize (remove 99 m)"
        assert prog(src) == VInt(1)

    def test_update(self):
        src = "import std:Map (empty, insert, update, lookup)\nlet m = insert 1 10 empty\nlookup 1 (update 1 (fun x -> x + 5) m)"
        assert prog(src) == VCtor("Just", [VInt(15)])

    def test_update_missing(self):
        src = "import std:Map (empty, update, size)\nsize (update 1 (fun x -> x + 1) empty)"
        assert prog(src) == VInt(0)

    def test_fromList(self):
        src = "import std:Map (fromList, size)\nsize (fromList [(1, 10), (2, 20), (3, 30)])"
        assert prog(src) == VInt(3)

    def test_toList(self):
        src = "import std:Map (fromList, toList)\ntoList (fromList [(3, 30), (1, 10), (2, 20)])"
        assert prog(src) == VList(
            [
                VTuple([VInt(1), VInt(10)]),
                VTuple([VInt(2), VInt(20)]),
                VTuple([VInt(3), VInt(30)]),
            ]
        )

    def test_keys(self):
        src = "import std:Map (fromList, keys)\nkeys (fromList [(3, 30), (1, 10), (2, 20)])"
        assert prog(src) == VList([VInt(1), VInt(2), VInt(3)])

    def test_values(self):
        src = "import std:Map (fromList, values)\nvalues (fromList [(3, 30), (1, 10), (2, 20)])"
        assert prog(src) == VList([VInt(10), VInt(20), VInt(30)])

    def test_map(self):
        src = "import std:Map (fromList, map, toList)\ntoList (map (fun x -> x * 2) (fromList [(1, 10), (2, 20)]))"
        assert prog(src) == VList(
            [
                VTuple([VInt(1), VInt(20)]),
                VTuple([VInt(2), VInt(40)]),
            ]
        )

    def test_filter(self):
        src = "import std:Map (fromList, filter, keys)\nkeys (filter (fun k v -> v > 15) (fromList [(1, 10), (2, 20), (3, 30)]))"
        assert prog(src) == VList([VInt(2), VInt(3)])

    def test_foldl(self):
        src = "import std:Map (fromList, foldl)\nfoldl (fun k v acc -> acc + v) 0 (fromList [(1, 10), (2, 20), (3, 30)])"
        assert prog(src) == VInt(60)

    def test_foldr(self):
        src = "import std:Map (fromList, foldr)\nfoldr (fun k v acc -> acc + v) 0 (fromList [(1, 10), (2, 20), (3, 30)])"
        assert prog(src) == VInt(60)

    def test_string_keys(self):
        src = 'import std:Map (fromList, lookup)\nlookup "b" (fromList [("a", 1), ("b", 2), ("c", 3)])'
        assert prog(src) == VCtor("Just", [VInt(2)])

    def test_qualified_import(self):
        src = "import std:Map as M\nM.size (M.fromList [(1, 10), (2, 20)])"
        assert prog(src) == VInt(2)


# ---------------------------------------------------------------------------
# Built-in test framework
# ---------------------------------------------------------------------------


class TestBuiltinTests:
    def test_assert_true(self):
        assert prog("assert true") == VUnit()

    def test_assert_false_raises(self):
        with pytest.raises(Error, match="assert failed"):
            prog("assert false")

    def test_assert_expression(self):
        assert prog("assert (1 + 1 == 2)") == VUnit()

    def test_test_skipped_normal(self):
        # Test blocks are skipped in normal mode; only the final expression runs
        src = 'test "should not run" =\n    error "boom"\n42'
        assert prog(src) == VInt(42)

    def test_run_tests_pass(self, capsys):
        src = 'let x = 10\ntest "x is 10" =\n    assert (x == 10)\n'
        failures = run_tests(src)
        assert failures == 0
        out = capsys.readouterr().out
        assert "PASS" in out
        assert "1 passed, 0 failed" in out

    def test_run_tests_fail(self, capsys):
        src = 'test "bad" =\n    assert false\n'
        failures = run_tests(src)
        assert failures == 1
        out = capsys.readouterr().out
        assert "FAIL" in out
        assert "0 passed, 1 failed" in out

    def test_test_with_let(self, capsys):
        src = 'test "let in test" =\n    let y = 5\n    assert (y == 5)\n'
        failures = run_tests(src)
        assert failures == 0

    def test_test_env_isolated(self):
        # Bindings from test body don't leak to outer scope
        src = 'test "isolated" =\n    let secret = 42\n    assert (secret == 42)\n1 + 1'
        assert prog(src) == VInt(2)

    def test_multiple_tests(self, capsys):
        src = 'test "a" =\n    assert true\ntest "b" =\n    assert true\n'
        failures = run_tests(src)
        assert failures == 0
        out = capsys.readouterr().out
        assert "2 passed, 0 failed" in out


# ---------------------------------------------------------------------------
# Structural equality
# ---------------------------------------------------------------------------


class TestStructuralEquality:
    def test_maybe_just_eq(self):
        assert prog("Just 42 == Just 42") == VBool(True)

    def test_maybe_just_neq(self):
        assert prog("Just 42 == Just 99") == VBool(False)

    def test_maybe_nothing_eq(self):
        assert prog("Nothing == Nothing") == VBool(True)

    def test_just_nothing_neq(self):
        assert prog("Just 1 == Nothing") == VBool(False)

    def test_list_eq(self):
        assert prog("[1, 2, 3] == [1, 2, 3]") == VBool(True)

    def test_list_neq(self):
        assert prog("[1, 2] == [1, 2, 3]") == VBool(False)

    def test_empty_list_eq(self):
        assert prog("[] == []") == VBool(True)

    def test_tuple_eq(self):
        assert prog("(1, true) == (1, true)") == VBool(True)

    def test_tuple_neq(self):
        assert prog("(1, true) == (1, false)") == VBool(False)

    def test_nested_ctor_eq(self):
        src = "type Wrap = W int\nW 42 == W 42"
        assert prog(src) == VBool(True)


# ---------------------------------------------------------------------------
# String builtins
# ---------------------------------------------------------------------------


class TestStringBuiltins:
    def test_char_at_valid(self):
        assert prog('import std:String (charAt)\ncharAt 0 "hello"') == VCtor(
            "Just", [VString("h")]
        )

    def test_char_at_out_of_bounds(self):
        assert prog('import std:String (charAt)\ncharAt 5 "hi"') == VCtor("Nothing", [])

    def test_substring(self):
        assert prog('import std:String (substring)\nsubstring 1 3 "hello"') == VString(
            "el"
        )

    def test_index_of_found(self):
        assert prog('import std:String (indexOf)\nindexOf "ll" "hello"') == VCtor(
            "Just", [VInt(2)]
        )

    def test_index_of_not_found(self):
        assert prog('import std:String (indexOf)\nindexOf "x" "hello"') == VCtor(
            "Nothing", []
        )

    def test_replace(self):
        assert prog('import std:String (replace)\nreplace "l" "r" "hello"') == VString(
            "herro"
        )

    def test_str_repeat(self):
        assert prog('import std:String (repeat)\nrepeat 3 "ab"') == VString("ababab")

    def test_pad_left(self):
        assert prog('import std:String (padLeft)\npadLeft 5 "0" "42"') == VString(
            "00042"
        )

    def test_pad_right(self):
        assert prog('import std:String (padRight)\npadRight 5 "." "hi"') == VString(
            "hi..."
        )

    def test_words(self):
        src = 'import std:String (words)\nimport std:List (length)\nlength (words "a b c")'
        assert prog(src) == VInt(3)

    def test_lines(self):
        src = 'import std:String (lines)\nimport std:List (length)\nlength (lines "a\\nb\\nc")'
        assert prog(src) == VInt(3)

    def test_char_code(self):
        assert prog('import std:String (charCode)\ncharCode "A"') == VInt(65)

    def test_from_char_code(self):
        assert prog("import std:String (fromCharCode)\nfromCharCode 65") == VString("A")

    def test_parse_int_valid(self):
        assert prog('import std:String (parseInt)\nparseInt "42"') == VCtor(
            "Just", [VInt(42)]
        )

    def test_parse_int_invalid(self):
        assert prog('import std:String (parseInt)\nparseInt "bad"') == VCtor(
            "Nothing", []
        )

    def test_parse_float_valid(self):
        assert prog('import std:String (parseFloat)\nparseFloat "3.14"') == VCtor(
            "Just", [VFloat(3.14)]
        )


# ---------------------------------------------------------------------------
# List stdlib extensions
# ---------------------------------------------------------------------------


class TestListExtensions:
    def test_zip(self):
        src = "import std:List (zip, head)\nhead (zip [1, 2, 3] [4, 5, 6])"
        assert prog(src) == VTuple([VInt(1), VInt(4)])

    def test_range(self):
        src = "import std:List (range, sum)\nsum (range 1 6)"
        assert prog(src) == VInt(15)

    def test_repeat(self):
        src = "import std:List (repeat, sum)\nsum (repeat 3 5)"
        assert prog(src) == VInt(15)

    def test_concat(self):
        src = "import std:List (concat, sum)\nsum (concat [[1, 2], [3], [4, 5]])"
        assert prog(src) == VInt(15)

    def test_concat_map(self):
        src = "import std:List (concatMap, length)\nlength (concatMap (fun x -> [x, x]) [1, 2, 3])"
        assert prog(src) == VInt(6)

    def test_is_empty(self):
        src = "import std:List (isEmpty)\nisEmpty []"
        assert prog(src) == VBool(True)

    def test_last(self):
        src = "import std:List (last)\nlast [1, 2, 3]"
        assert prog(src) == VInt(3)

    def test_init(self):
        src = "import std:List (init, length)\nlength (init [1, 2, 3])"
        assert prog(src) == VInt(2)

    def test_nth(self):
        src = "import std:List (nth)\nnth 1 [10, 20, 30]"
        assert prog(src) == VCtor("Just", [VInt(20)])

    def test_nth_out_of_bounds(self):
        src = "import std:List (nth)\nnth 5 [1, 2, 3]"
        assert prog(src) == VCtor("Nothing", [])

    def test_find(self):
        src = "import std:List (find)\nfind (fun x -> x > 2) [1, 2, 3, 4]"
        assert prog(src) == VCtor("Just", [VInt(3)])

    def test_partition(self):
        src = "import std:List (partition, sum)\nlet (yes, no) = partition (fun x -> x > 2) [1, 2, 3, 4]\nsum yes"
        assert prog(src) == VInt(7)

    def test_intersperse(self):
        src = "import std:List (intersperse, sum)\nsum (intersperse 0 [1, 2, 3])"
        assert prog(src) == VInt(6)

    def test_maximum(self):
        src = "import std:List (maximum)\nmaximum [3, 1, 4, 1, 5]"
        assert prog(src) == VCtor("Just", [VInt(5)])

    def test_minimum(self):
        src = "import std:List (minimum)\nminimum [3, 1, 4, 1, 5]"
        assert prog(src) == VCtor("Just", [VInt(1)])

    def test_maximum_empty(self):
        src = "import std:List (maximum)\nmaximum []"
        assert prog(src) == VCtor("Nothing", [])


# ---------------------------------------------------------------------------
# JSON module
# ---------------------------------------------------------------------------


class TestJsonModule:
    def test_stringify_null(self):
        assert prog("import std:Json (stringify, JNull)\nstringify JNull") == VString(
            "null"
        )

    def test_stringify_bool(self):
        assert prog(
            "import std:Json (stringify, JBool)\nstringify (JBool true)"
        ) == VString("true")

    def test_stringify_num(self):
        assert prog(
            "import std:Json (stringify, JNum)\nstringify (JNum 3.14)"
        ) == VString("3.14")

    def test_stringify_str(self):
        assert prog(
            'import std:Json (stringify, JStr)\nstringify (JStr "hi")'
        ) == VString('"hi"')

    def test_stringify_empty_arr(self):
        assert prog(
            "import std:Json (stringify, JArr, ArrNil)\nstringify (JArr ArrNil)"
        ) == VString("[]")

    def test_stringify_empty_obj(self):
        assert prog(
            "import std:Json (stringify, JObj, ObjNil)\nstringify (JObj ObjNil)"
        ) == VString("{}")

    def test_parse_ok(self):
        assert prog(
            'import std:Json (parse)\nimport std:Result (isOk)\nisOk (parse "null")'
        ) == VBool(True)

    def test_parse_err(self):
        assert prog(
            'import std:Json (parse)\nimport std:Result (isErr)\nisErr (parse "!bad")'
        ) == VBool(True)

    def test_parse_round_trip(self):
        src = 'import std:Json (parse, stringify, JNull)\nimport std:Result (withDefault)\nstringify (withDefault JNull (parse "null"))'
        assert prog(src) == VString("null")

    def test_encode_arr(self):
        assert prog(
            "import std:Json (encodeArr, stringify, JNull, JBool)\nstringify (encodeArr [JNull, JBool true])"
        ) == VString("[null, true]")

    def test_get_field_found(self):
        src = 'import std:Json (getField, ObjNil, ObjCons, JNum)\ngetField "x" (ObjCons "x" (JNum 1.0) ObjNil)'
        assert prog(src) == VCtor("Just", [VCtor("JNum", [VFloat(1.0)])])

    def test_get_field_missing(self):
        src = 'import std:Json (getField, ObjNil, ObjCons, JNum)\ngetField "z" (ObjCons "x" (JNum 1.0) ObjNil)'
        assert prog(src) == VCtor("Nothing", [])
