-- Modulo operator examples

let isEven = \n -> n % 2 == 0

test "modulo" =
    assert (10 % 3 == 1)
    assert (7 % 2 == 1)
    assert (6 % 3 == 0)

test "isEven" =
    assert (isEven 4)
    assert (5 |> isEven |> not)

true
