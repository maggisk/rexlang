-- Forward references: top-level bindings are automatically
-- reordered by dependency, so order in the source doesn't matter.

-- Use a function defined below
result = 2 |> triple |> double

double x = x * 2
triple x = x * 3

-- Chain of value dependencies
c = b + a
b = a * 2
a = 5

-- Forward ref with type annotation
greet : String -> String
greet name = salutation ++ name

salutation = "Hello, "

test "forward function references" =
    assert result == 12

test "forward value chain" =
    assert a == 5
    assert b == 10
    assert c == 15

test "forward ref with annotation" =
    assert greet "Rex" == "Hello, Rex"
