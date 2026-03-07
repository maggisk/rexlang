-- Higher-order functions

apply f x = f x
compose f g x = f (g x)

double n = n * 2
inc n = n + 1

test "apply" =
    assert (apply double 5 == 10)
    assert (apply inc 41 == 42)

test "compose" =
    assert (compose double inc 20 == 42)
    assert (compose inc double 20 == 41)
