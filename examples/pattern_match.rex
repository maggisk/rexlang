-- Pattern matching examples


-- Fibonacci using case
let rec fib n =
    case n of
        0 ->
            0
        1 ->
            1
        _ ->
            fib (n - 1) + fib (n - 2)

test "fibonacci via pattern match" =
    assert (fib 10 == 55)


-- Describe a number as a string
let describe x =
    case x of
        0 ->
            "zero"
        1 ->
            "one"
        _ ->
            "other"

test "describe" =
    assert (describe 0 == "zero")
    assert (describe 1 == "one")
    assert (describe 42 == "other")


-- Boolean negation via pattern matching
let myNot b =
    case b of
        true ->
            false
        false ->
            true

test "boolean negation" =
    assert (myNot true == false)
    assert (myNot false == true)

true
