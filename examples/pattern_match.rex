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

print (fib 10)


-- Describe a number as a string
let describe x =
    case x of
        0 ->
            "zero"
        1 ->
            "one"
        _ ->
            "other"

print (describe 0)
print (describe 1)
print (describe 42)


-- Boolean negation via pattern matching
let myNot b =
    case b of
        true ->
            false
        false ->
            true

print (myNot true)
print (myNot false)
