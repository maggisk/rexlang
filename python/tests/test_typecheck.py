import sys
import os

sys.path.insert(0, os.path.dirname(os.path.dirname(os.path.abspath(__file__))))

from rexlang import typecheck as TypeCheck
from rexlang.parser import parse
from rexlang.types import type_to_string, TypeError as RexTypeError


EXAMPLES = os.path.join(os.path.dirname(__file__), "..", "..", "examples")


def tc(source: str) -> dict:
    """Type-check a program and return the final type environment."""
    return TypeCheck.check_program(parse(source))


def ty(source: str) -> str:
    """Infer the type of the last top-level expression and return as string."""
    exprs = parse(source)
    checker = TypeCheck.TypeChecker()
    env = TypeCheck.initial_type_env()
    type_defs: dict = {}
    last_ty = None
    for expr in exprs:
        _, t, env, type_defs = checker.infer_toplevel(env, type_defs, {}, expr)
        last_ty = t
    return type_to_string(last_ty)


def raises_type_error(source: str) -> bool:
    """Return True if the source raises a type error."""
    try:
        tc(source)
        return False
    except RexTypeError:
        return True


# ---------------------------------------------------------------------------
# Primitives
# ---------------------------------------------------------------------------


class TestHMPrimitives:
    def test_int(self):
        assert ty("42") == "Int"

    def test_float(self):
        assert ty("3.14") == "Float"

    def test_string(self):
        assert ty('"hello"') == "String"

    def test_bool_true(self):
        assert ty("true") == "Bool"

    def test_bool_false(self):
        assert ty("false") == "Bool"

    def test_unit_does_not_raise(self):
        # TypeDecl returns Unit — just check no exception
        tc("type Color = Red | Green | Blue")


# ---------------------------------------------------------------------------
# Arithmetic
# ---------------------------------------------------------------------------


class TestHMArithmetic:
    def test_int_plus_int(self):
        assert ty("1 + 2") == "Int"

    def test_float_plus_float(self):
        assert ty("1.5 + 2.5") == "Float"

    def test_int_plus_float_is_error(self):
        assert raises_type_error("1 + 1.5")

    def test_float_plus_int_is_error(self):
        assert raises_type_error("1.5 + 1")

    def test_int_sub(self):
        assert ty("10 - 3") == "Int"

    def test_int_mul(self):
        assert ty("3 * 4") == "Int"

    def test_int_div(self):
        assert ty("10 / 2") == "Int"

    def test_int_mod(self):
        assert ty("10 % 3") == "Int"

    def test_concat(self):
        assert ty('"a" ++ "b"') == "String"

    def test_concat_non_string_error(self):
        assert raises_type_error("1 ++ 2")

    def test_comparison_int(self):
        assert ty("1 < 2") == "Bool"

    def test_comparison_float(self):
        assert ty("1.0 < 2.0") == "Bool"

    def test_comparison_type_mismatch(self):
        assert raises_type_error("1 < 2.0")

    def test_eq_int(self):
        assert ty("1 == 1") == "Bool"

    def test_and_bool(self):
        assert ty("true && false") == "Bool"

    def test_or_bool(self):
        assert ty("true || false") == "Bool"

    def test_and_non_bool_error(self):
        assert raises_type_error("1 && true")

    def test_unary_minus_int(self):
        assert ty("-5") == "Int"

    def test_unary_minus_float(self):
        assert ty("-3.14") == "Float"


# ---------------------------------------------------------------------------
# Functions
# ---------------------------------------------------------------------------


class TestHMFunctions:
    def test_identity_function(self):
        env = tc("let id x = x")
        assert "id" in env
        # id should be polymorphic: ∀a. a -> a
        scheme = env["id"]
        assert len(scheme.vars) == 1  # one quantified variable

    def test_identity_applied_int(self):
        assert ty("let id x = x in id 1") == "Int"

    def test_identity_applied_string(self):
        assert ty('let id x = x in id "hi"') == "String"

    def test_curried_add(self):
        env = tc("let add x y = x + y")
        assert "add" in env
        scheme = env["add"]
        ts = type_to_string(scheme.ty)
        # Arithmetic is polymorphic: ∀a. a -> a -> a
        assert ts == "a -> a -> a"
        assert len(scheme.vars) == 1

    def test_closure_captures_env(self):
        assert ty("let x = 5\nlet f y = x + y") == "Int -> Int"

    def test_recursive_function(self):
        env = tc("let rec fact n = if n == 0 then 1 else n * fact (n - 1)")
        ts = type_to_string(env["fact"].ty)
        assert ts == "Int -> Int"

    def test_higher_order(self):
        env = tc("let apply f x = f x")
        scheme = env["apply"]
        # apply: ∀a b. (a -> b) -> a -> b
        assert len(scheme.vars) == 2


# ---------------------------------------------------------------------------
# Polymorphism
# ---------------------------------------------------------------------------


class TestHMPolymorphism:
    def test_identity_used_at_two_types(self):
        # let id x = x in (id 1, id "hi")  — should not error
        result = ty('let id x = x in (id 1, id "hi")')
        assert result == "(Int, String)"

    def test_generalization(self):
        env = tc("let id x = x")
        scheme = env["id"]
        assert scheme.vars  # must be generalized

    def test_polymorphic_list_functions(self):
        # Using map with different element types should type-check
        env = tc(
            "import std:List (map)\n"
            "let f = map (fun x -> x + 1)\n"
            "let g = map (fun s -> s ++ s)"
        )
        assert "f" in env
        assert "g" in env


# ---------------------------------------------------------------------------
# ADTs
# ---------------------------------------------------------------------------


class TestHMADTs:
    def test_nullary_ctor_registered(self):
        env = tc("type Color = Red | Green | Blue")
        assert "Red" in env
        assert "Green" in env
        assert "Blue" in env

    def test_nullary_ctor_type(self):
        ts = ty("type Color = Red | Green | Blue\nRed")
        assert ts == "Color"

    def test_ctor_with_arg(self):
        env = tc("type Option = None | Some int")
        ts = type_to_string(env["Some"].ty)
        assert ts == "Int -> Option"

    def test_match_ctor_infer(self):
        result = ty(
            "type Option = None | Some int\n"
            "let x = Some 7\n"
            "case x of\n"
            "  None -> 0\n"
            "  Some n -> n"
        )
        assert result == "Int"

    def test_recursive_adt(self):
        env = tc("type List = Nil | Cons int List")
        assert "Nil" in env
        assert "Cons" in env
        ts = type_to_string(env["Cons"].ty)
        assert ts == "Int -> List -> List"

    def test_unknown_ctor_is_error(self):
        assert raises_type_error("type Option = None | Some int\nMissing 5")


# ---------------------------------------------------------------------------
# Lists
# ---------------------------------------------------------------------------


class TestHMLists:
    def test_empty_list(self):
        result = ty("[]")
        assert result == "[a]"

    def test_int_list(self):
        result = ty("[1, 2, 3]")
        assert result == "[Int]"

    def test_string_list(self):
        result = ty('["a", "b"]')
        assert result == "[String]"

    def test_heterogeneous_list_error(self):
        assert raises_type_error('[1, "hi"]')

    def test_cons_int(self):
        result = ty("1 :: [2, 3]")
        assert result == "[Int]"

    def test_cons_tail_mismatch_error(self):
        assert raises_type_error("1 :: 2")

    def test_list_length(self):
        env = tc(
            "let rec length lst =\n"
            "    case lst of\n"
            "        [] -> 0\n"
            "        [_|t] -> 1 + length t"
        )
        ts = type_to_string(env["length"].ty)
        assert ts == "[a] -> Int"

    def test_map_type(self):
        env = tc("import std:List (map)")
        scheme = env["map"]
        ts = type_to_string(scheme.ty)
        assert ts == "(a -> b) -> [a] -> [b]"


# ---------------------------------------------------------------------------
# Tuples
# ---------------------------------------------------------------------------


class TestHMTuples:
    def test_int_tuple(self):
        result = ty("(1, 2)")
        assert result == "(Int, Int)"

    def test_mixed_tuple(self):
        result = ty('(1, "hi")')
        assert result == "(Int, String)"

    def test_triple(self):
        result = ty("(1, 2, 3)")
        assert result == "(Int, Int, Int)"

    def test_tuple_destructure(self):
        result = ty("let (a, b) = (1, 2) in a + b")
        assert result == "Int"

    def test_swap_type(self):
        env = tc("let swap pair = let (a, b) = pair in (b, a)")
        scheme = env["swap"]
        # swap: ∀a b. (a, b) -> (b, a)
        assert len(scheme.vars) == 2


# ---------------------------------------------------------------------------
# Type errors
# ---------------------------------------------------------------------------


class TestHMTypeErrors:
    def test_if_cond_not_bool(self):
        assert raises_type_error("if 1 then 2 else 3")

    def test_if_branches_mismatch(self):
        assert raises_type_error("if true then 1 else true")

    def test_wrong_arg_type(self):
        assert raises_type_error('let f x = x + 1 in f "hi"')

    def test_unbound_variable(self):
        assert raises_type_error("unknownVar")

    def test_not_a_function(self):
        assert raises_type_error("1 2")

    def test_arithmetic_mismatch(self):
        assert raises_type_error("1 + true")


# ---------------------------------------------------------------------------
# Imports
# ---------------------------------------------------------------------------


class TestHMImport:
    def test_import_map(self):
        env = tc("import std:List (map)")
        assert "map" in env
        ts = type_to_string(env["map"].ty)
        assert ts == "(a -> b) -> [a] -> [b]"

    def test_import_and_use(self):
        result = ty("import std:List (map)\nmap (fun x -> x + 1) [1, 2, 3]")
        assert result == "[Int]"

    def test_import_filter(self):
        env = tc("import std:List (filter)")
        scheme = env["filter"]
        ts = type_to_string(scheme.ty)
        assert ts == "(a -> Bool) -> [a] -> [a]"

    def test_import_foldl(self):
        env = tc("import std:List (foldl)")
        scheme = env["foldl"]
        ts = type_to_string(scheme.ty)
        assert ts == "(a -> b -> a) -> a -> [b] -> a"

    def test_import_nonexistent_module(self):
        assert raises_type_error("import std:Nonexistent (foo)")

    def test_import_nonexported_name(self):
        assert raises_type_error("import std:List (nonexistentFn)")


# ---------------------------------------------------------------------------
# Parametric ADTs
# ---------------------------------------------------------------------------


class TestHMParametricADTs:
    def test_nothing_is_polymorphic(self):
        env = tc("type Maybe a = Nothing | Just a")
        assert len(env["Nothing"].vars) == 1

    def test_just_type(self):
        env = tc("type Maybe a = Nothing | Just a")
        assert type_to_string(env["Just"].ty) == "a -> (Maybe a)"

    def test_just_int(self):
        assert ty("type Maybe a = Nothing | Just a\nJust 5") == "(Maybe Int)"

    def test_just_string(self):
        assert ty('type Maybe a = Nothing | Just a\nJust "hi"') == "(Maybe String)"

    def test_match_maybe(self):
        assert (
            ty(
                "type Maybe a = Nothing | Just a\n"
                "case Just 5 of\n"
                "  Nothing -> 0\n"
                "  Just n -> n"
            )
            == "Int"
        )

    def test_polymorphic_function(self):
        # isNothing should work for maybe Int and maybe String
        tc(
            "type Maybe a = Nothing | Just a\n"
            "let isNothing x = case x of\n"
            "  Nothing -> true\n"
            "  Just _ -> false\n"
            "isNothing (Just 5)\n"
            "isNothing (Just true)"
        )

    def test_arm_type_mismatch_still_errors(self):
        assert raises_type_error(
            "type Maybe a = Nothing | Just a\n"
            "case Just 5 of\n"
            "  Nothing -> 0\n"
            "  Just n -> true"
        )


# ---------------------------------------------------------------------------
# Example files
# ---------------------------------------------------------------------------


class TestHMExamples:
    """Type-check all .rex example files without error."""

    def _typecheck(self, filename):
        path = os.path.join(EXAMPLES, filename)
        with open(path) as f:
            source = f.read()
        TypeCheck.check_program(parse(source))

    def test_factorial(self):
        self._typecheck("factorial.rex")

    def test_fibonacci(self):
        self._typecheck("fibonacci.rex")

    def test_adt(self):
        self._typecheck("adt.rex")

    def test_pattern_match(self):
        self._typecheck("pattern_match.rex")

    def test_higher_order(self):
        self._typecheck("higher_order.rex")

    def test_floats(self):
        self._typecheck("floats.rex")

    def test_pipe(self):
        self._typecheck("pipe.rex")

    def test_modulo(self):
        self._typecheck("modulo.rex")

    def test_list(self):
        self._typecheck("list.rex")

    def test_import(self):
        self._typecheck("import.rex")

    def test_tuple(self):
        self._typecheck("tuple.rex")

    def test_mutual_recursion(self):
        self._typecheck("mutual_recursion.rex")

    def test_io(self):
        self._typecheck("io.rex")

    def test_maybe(self):
        self._typecheck("maybe.rex")


class TestMaybeStdlib:
    def test_import_just_type(self):
        result = ty('import std:Maybe (Nothing, Just)\nJust 5')
        assert result == "(Maybe Int)"

    def test_import_fromMaybe_type(self):
        result = ty('import std:Maybe (Nothing, Just, fromMaybe)\nfromMaybe 0 (Just 7)')
        assert result == "Int"

    def test_import_map_type(self):
        result = ty('import std:Maybe (Nothing, Just, map)\nmap (fun x -> x * 2) (Just 5)')
        assert result == "(Maybe Int)"
