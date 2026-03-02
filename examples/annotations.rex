-- Type annotations

double : Int -> Int
let double x = x * 2

identity : a -> a
let identity x = x

greet : String -> String
let greet name = "Hello, ${name}!"

test "annotated functions" =
    assert (double 21 == 42)
    assert (identity "hello" == "hello")
    assert (identity 42 == 42)
    assert (greet "Rex" == "Hello, Rex!")

true
