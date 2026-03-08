-- Factorial

fact n =
    match n
        when 0 ->
            1
        when _ ->
            n * fact (n - 1)


test "factorial" =
    assert (fact 0 == 1)
    assert (fact 5 == 120)
    assert (fact 10 == 3628800)
