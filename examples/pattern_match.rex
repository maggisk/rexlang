-- Pattern matching examples


-- Fibonacci using match
fib n =
    match n
        when 0 ->
            0
        when 1 ->
            1
        when _ ->
            fib (n - 1) + fib (n - 2)

test "fibonacci via pattern match" =
    assert (fib 10 == 55)


-- Describe a number as a string
describe x =
    match x
        when 0 ->
            "zero"
        when 1 ->
            "one"
        when _ ->
            "other"

test "describe" =
    assert (describe 0 == "zero")
    assert (describe 1 == "one")
    assert (describe 42 == "other")


-- Boolean negation via pattern matching
myNot b =
    match b
        when true ->
            false
        when false ->
            true

test "boolean negation" =
    assert (myNot true == false)
    assert (myNot false == true)
