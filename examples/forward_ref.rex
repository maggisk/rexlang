-- Forward references: top-level bindings are automatically
-- reordered by dependency, so order in the source doesn't matter.

-- Use a function defined below
let result = 2 |> triple |> double

let double x = x * 2
let triple x = x * 3

-- Chain of value dependencies
let c = b + a
let b = a * 2
let a = 5

-- Forward ref with type annotation
greet : String -> String
let greet name = salutation ++ name

let salutation = "Hello, "

test "forward function references" =
    assert result == 12

test "forward value chain" =
    assert a == 5
    assert b == 10
    assert c == 15

test "forward ref with annotation" =
    assert greet "Rex" == "Hello, Rex"

true
