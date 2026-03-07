-- Fibonacci

fib n =
    case n of
        0 ->
            0
        1 ->
            1
        _ ->
            fib (n - 1) + fib (n - 2)


test "fibonacci" =
    assert (fib 0 == 0)
    assert (fib 1 == 1)
    assert (fib 10 == 55)
    assert (fib 20 == 6765)
