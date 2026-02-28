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

-- Use built-in Ord
let cmp = compare 3 5

-- Use built-in Eq
let same = eq "hello" "hello"

-- Use custom trait
let d1 = describe 42
let d2 = describe 0
let d3 = describe false

-- Pattern match on Ordering
let label = case cmp of
    LT ->
        "less"
    EQ ->
        "equal"
    GT ->
        "greater"

-- Combine results
label ++ " " ++ d1 ++ " " ++ d2 ++ " " ++ d3
