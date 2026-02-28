-- Built-in test framework demo

let double x = x * 2

let add x y = x + y

test "double works" =
    assert (double 5 == 10)
    assert (double 0 == 0)
    assert (double (-3) == -6)

test "addition" =
    assert (add 1 2 == 3)
    assert (add 0 0 == 0)

test "list operations" =
    import std:List (length, map, foldl)
    let xs = [1, 2, 3]
    assert (length xs == 3)
    assert (length (map double xs) == 3)
    assert (foldl (fun a b -> a + b) 0 (map double xs) == 12)

test "boolean logic" =
    assert (true && true)
    assert (true || false)
    assert (not false)

true
