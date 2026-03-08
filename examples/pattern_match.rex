-- Pattern matching examples
-- Tests exhaustiveness checking (Maranget's usefulness algorithm)


import Std:List (map, foldl)
import Std:Maybe (Just, Nothing)
import Std:Result (Ok, Err)


-- # Basic patterns


-- Fibonacci using match
fib n =
    match n
        when 0 ->
            0
        when 1 ->
            1
        when _ ->
            fib (n - 1) + fib (n - 2)

test "fibonacci via pattern match" =
    assert (fib 10 == 55)


-- Describe a number as a string
describe x =
    match x
        when 0 ->
            "zero"
        when 1 ->
            "one"
        when _ ->
            "other"

test "describe" =
    assert (describe 0 == "zero")
    assert (describe 1 == "one")
    assert (describe 42 == "other")


-- Boolean negation via pattern matching
myNot b =
    match b
        when true ->
            false
        when false ->
            true

test "boolean negation" =
    assert (myNot true == false)
    assert (myNot false == true)


-- # List patterns


-- | Sum a list of ints.
sum xs =
    match xs
        when [] ->
            0
        when [h|t] ->
            h + sum t

test "list sum" =
    assert (sum [1, 2, 3, 4, 5] == 15)
    assert (sum [] == 0)


-- | Check whether a list is empty.
isEmpty xs =
    match xs
        when [] ->
            true
        when [_|_] ->
            false

test "list isEmpty" =
    assert (isEmpty [])
    assert (isEmpty [1] |> not)


-- # ADT patterns


type Color = Red | Green | Blue

colorName c =
    match c
        when Red ->
            "red"
        when Green ->
            "green"
        when Blue ->
            "blue"

test "ADT exhaustive" =
    assert (colorName Red == "red")
    assert (colorName Green == "green")
    assert (colorName Blue == "blue")


type Shape = Circle Float | Rect Float Float

-- | Wildcard catch-all after partial ADT match.
isCircle s =
    match s
        when Circle _ ->
            true
        when _ ->
            false

test "ADT with catch-all" =
    assert (isCircle (Circle 5.0))
    assert (isCircle (Rect 3.0 4.0) |> not)


-- # Tuple patterns


-- | Boolean AND via tuple matching (all four combos).
myAnd a b =
    match (a, b)
        when (true, true) ->
            true
        when (true, false) ->
            false
        when (false, true) ->
            false
        when (false, false) ->
            false

test "tuple bool all combos" =
    assert (myAnd true true)
    assert (myAnd true false |> not)
    assert (myAnd false true |> not)
    assert (myAnd false false |> not)


-- | Boolean XOR — uses wildcard row for remaining cases.
myXor a b =
    match (a, b)
        when (true, false) ->
            true
        when (false, true) ->
            true
        when _ ->
            false

test "tuple with catch-all" =
    assert (myXor true false)
    assert (myXor false true)
    assert (myXor true true |> not)
    assert (myXor false false |> not)


-- | Match on two lists — all four constructor combos.
zipStatus xs ys =
    match (xs, ys)
        when ([], []) ->
            "both empty"
        when ([], [_|_]) ->
            "left empty"
        when ([_|_], []) ->
            "right empty"
        when ([_|_], [_|_]) ->
            "both non-empty"

test "tuple list cross-column" =
    assert (zipStatus [] [] == "both empty")
    assert (zipStatus [] [1] == "left empty")
    assert (zipStatus [1] [] == "right empty")
    assert (zipStatus [1] [2] == "both non-empty")


-- # Maybe/Result in tuples (cross-column)


-- | Match on two Maybes — all four combos.
bothPresent a b =
    match (a, b)
        when (Just x, Just y) ->
            Just (x + y)
        when (Just _, Nothing) ->
            Nothing
        when (Nothing, Just _) ->
            Nothing
        when (Nothing, Nothing) ->
            Nothing

test "tuple Maybe all combos" =
    assert (bothPresent (Just 1) (Just 2) == Just 3)
    assert (bothPresent (Just 1) Nothing == Nothing)
    assert (bothPresent Nothing (Just 2) == Nothing)
    assert (bothPresent Nothing Nothing == Nothing)


-- | Partial coverage with wildcard catch-all.
firstJust a b =
    match (a, b)
        when (Just x, _) ->
            Just x
        when (_, Just y) ->
            Just y
        when _ ->
            Nothing

test "tuple Maybe with catch-all" =
    assert (firstJust (Just 1) (Just 2) == Just 1)
    assert (firstJust Nothing (Just 2) == Just 2)
    assert (firstJust Nothing Nothing == Nothing)


-- # Nested patterns


-- | Nested Maybe patterns.
deepMaybe x =
    match x
        when Just (Just v) ->
            v
        when Just Nothing ->
            0
        when Nothing ->
            0

test "nested Maybe" =
    assert (deepMaybe (Just (Just 42)) == 42)
    assert (deepMaybe (Just Nothing) == 0)
    assert (deepMaybe Nothing == 0)


-- | Nested constructors in a tuple — cross-column with nesting.
nestedInTuple pair =
    match pair
        when (Just (Just _), _) ->
            "deep"
        when (Just Nothing, _) ->
            "shallow"
        when (Nothing, _) ->
            "none"

test "nested ADT in tuple" =
    assert (nestedInTuple (Just (Just 1), 0) == "deep")
    assert (nestedInTuple (Just Nothing, 0) == "shallow")
    assert (nestedInTuple (Nothing, 0) == "none")


-- | List nested inside Maybe.
maybeList x =
    match x
        when Just [] ->
            "empty list"
        when Just [_|_] ->
            "non-empty list"
        when Nothing ->
            "nothing"

test "list nested in Maybe" =
    assert (maybeList (Just []) == "empty list")
    assert (maybeList (Just [1, 2]) == "non-empty list")
    assert (maybeList Nothing == "nothing")


-- # Three-element tuples


-- | Three bools — partial coverage with catch-all.
tripleDesc triple =
    match triple
        when (true, true, true) ->
            "all true"
        when (false, false, false) ->
            "all false"
        when _ ->
            "mixed"

test "triple tuple with catch-all" =
    assert (tripleDesc (true, true, true) == "all true")
    assert (tripleDesc (false, false, false) == "all false")
    assert (tripleDesc (true, false, true) == "mixed")


-- # Unit and record patterns


test "unit pattern" =
    let f u =
        match u
            when () ->
                "unit"
    in assert (f () == "unit")


type Point = { x : Int, y : Int }

test "record pattern" =
    let origin = Point { x = 0, y = 0 }
    in let f p =
        match p
            when Point { x = x } ->
                x
    in assert (f origin == 0)


-- # Practical: interpreter-style dispatch


type Token = TNum Int | TPlus | TMinus | TEnd

tokenName tok =
    match tok
        when TNum _ ->
            "number"
        when TPlus ->
            "plus"
        when TMinus ->
            "minus"
        when TEnd ->
            "end"

test "ADT with mix of nullary and unary constructors" =
    assert (tokenName (TNum 42) == "number")
    assert (tokenName TPlus == "plus")
    assert (tokenName TMinus == "minus")
    assert (tokenName TEnd == "end")


-- # Mixed: ADT constructor with nested tuple


type Expr = Lit Int | Add Expr Expr

eval expr =
    match expr
        when Lit n ->
            n
        when Add a b ->
            eval a + eval b

test "recursive ADT" =
    assert (eval (Add (Lit 1) (Add (Lit 2) (Lit 3))) == 6)
