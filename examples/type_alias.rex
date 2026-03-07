-- Type aliases: transparent alternative names for existing types

-- Simple alias
type alias Name = String

greet : Name -> String
greet name = "Hello, " ++ name

-- Function type alias
type alias Predicate a = a -> Bool

isPositive : Predicate Int
isPositive n = n > 0

-- Parametric alias
type alias Pair a b = (a, b)

swap : Pair a b -> Pair b a
swap p =
    case p of
        (a, b) ->
            (b, a)

-- Alias for list type
type alias IntList = [Int]

sum : IntList -> Int
sum lst =
    case lst of
        [] ->
            0
        [h|t] ->
            h + sum t

-- Aliases are transparent — Name and String are interchangeable
test "simple alias" =
    let n = "Alice"
    assert (greet n == "Hello, Alice")
    assert (greet "Bob" == "Hello, Bob")

test "function type alias" =
    assert (isPositive 5)
    assert (isPositive (-3) == false)

test "parametric alias" =
    assert (swap (1, "hello") == ("hello", 1))

test "list alias" =
    assert (sum [1, 2, 3, 4, 5] == 15)
