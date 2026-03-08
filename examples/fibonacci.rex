-- Fibonacci

fib n =
    match n
        when 0 ->
            0
        when 1 ->
            1
        when _ ->
            fib (n - 1) + fib (n - 2)


test "fibonacci" =
    assert (fib 0 == 0)
    assert (fib 1 == 1)
    assert (fib 10 == 55)
    assert (fib 20 == 6765)
