-- Type annotations

double : Int -> Int
double x = x * 2

identity : a -> a
identity x = x

greet : String -> String
greet name = "Hello, ${name}!"

test "annotated functions" =
    assert (double 21 == 42)
    assert (identity "hello" == "hello")
    assert (identity 42 == 42)
    assert (greet "Rex" == "Hello, Rex!")
