-- Type aliases: transparent alternative names for existing types

-- Simple alias
type Name = String

greet : Name -> String
let greet name = "Hello, " ++ name

-- Function type alias
type Predicate a = a -> Bool

isPositive : Predicate Int
let isPositive n = n > 0

-- Parametric alias
type Pair a b = (a, b)

swap : Pair a b -> Pair b a
let swap p =
    case p of
        (a, b) ->
            (b, a)

-- Alias for list type
type IntList = [Int]

sum : IntList -> Int
let rec sum lst =
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

greet "World"
