import pytest
import sys, os, tempfile

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
