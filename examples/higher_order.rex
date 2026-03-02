-- Higher-order functions

let apply f x = f x
let compose f g x = f (g x)

let double n = n * 2
let inc n = n + 1

test "apply" =
    assert (apply double 5 == 10)
    assert (apply inc 41 == 42)

test "compose" =
    assert (compose double inc 20 == 42)
    assert (compose inc double 20 == 41)

true
