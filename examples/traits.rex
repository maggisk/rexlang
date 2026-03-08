-- Traits (typeclasses) example

-- Ordering type and Eq/Ord traits are loaded from the Prelude automatically.
-- compare and eq work on Int, Float, String, Bool out of the box.

-- Custom trait
trait Describable a where
    describe : a -> String

impl Describable Int where
    describe x =
        if x < 0 then
            "negative"
        else if x == 0 then
            "zero"
        else
            "positive"

impl Describable Bool where
    describe x =
        if x == true then
            "yes"
        else
            "no"

test "built-in Ord" =
    assert (compare 3 5 == LT)
    assert (compare 5 3 == GT)
    assert (compare 3 3 == EQ)

test "built-in Eq" =
    assert (eq "hello" "hello")
    assert (eq "a" "b" |> not)

test "custom trait" =
    assert (describe 42 == "positive")
    assert (describe 0 == "zero")
    assert (describe (0 - 1) == "negative")
    assert (describe true == "yes")
    assert (describe false == "no")

test "ordering pattern match" =
    let label = match compare 3 5
        when LT ->
            "less"
        when EQ ->
            "equal"
        when GT ->
            "greater"
    assert (label == "less")


-- ## Parameterized instances

import Std:Maybe (Just, Nothing)
import Std:Result (Ok, Err)

test "show list" =
    assert (show [1, 2, 3] == "[1, 2, 3]")
    assert (show [] == "[]")
    assert (show ["hello", "world"] == "[hello, world]")

test "show tuple" =
    assert (show (1, "hello") == "(1, hello)")
    assert (show (true, 42) == "(true, 42)")

test "show unit" =
    assert (show () == "()")

test "show Maybe" =
    assert (show (Just 42) == "Just 42")
    assert (show Nothing == "Nothing")
    assert (show (Just "hello") == "Just hello")

test "show Result" =
    assert (show (Ok 42) == "Ok 42")
    assert (show (Err "oops") == "Err oops")

test "show nested" =
    assert (show [Just 1, Nothing, Just 3] == "[Just 1, Nothing, Just 3]")
    assert (show (Just [1, 2]) == "Just [1, 2]")

test "eq list" =
    assert (eq [1, 2, 3] [1, 2, 3])
    assert (eq [1, 2] [1, 3] |> not)
    assert (eq [] [1] |> not)
    assert (eq [1] [] |> not)

test "eq tuple" =
    assert (eq (1, 2) (1, 2))
    assert (eq (1, 2) (1, 3) |> not)

test "eq Maybe" =
    assert (eq (Just 1) (Just 1))
    assert (eq (Just 1) (Just 2) |> not)
    assert (eq Nothing Nothing)
    assert (eq (Just 1) Nothing |> not)

test "eq Result" =
    assert (eq (Ok 1) (Ok 1))
    assert (eq (Ok 1) (Ok 2) |> not)
    assert (eq (Err "a") (Err "a"))
    assert (eq (Ok 1) (Err 1) |> not)

test "ord list" =
    assert (compare [1, 2] [1, 3] == LT)
    assert (compare [1, 2, 3] [1, 2] == GT)
    assert (compare [1, 2] [1, 2] == EQ)
    assert (compare [] [1] == LT)

test "ord tuple" =
    assert (compare (1, 2) (1, 3) == LT)
    assert (compare (2, 1) (1, 9) == GT)
    assert (compare (1, 2) (1, 2) == EQ)

test "ord Maybe" =
    assert (compare Nothing (Just 1) == LT)
    assert (compare (Just 1) Nothing == GT)
    assert (compare (Just 1) (Just 2) == LT)
    assert (compare Nothing Nothing == EQ)

test "string interpolation with compound types" =
    assert ("list is ${[1, 2, 3]}" == "list is [1, 2, 3]")
    assert ("maybe is ${Just 42}" == "maybe is Just 42")
    assert ("pair is ${(1, true)}" == "pair is (1, true)")
