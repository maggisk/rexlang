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
    let label = case compare 3 5 of
        LT ->
            "less"
        EQ ->
            "equal"
        GT ->
            "greater"
    assert (label == "less")

true
