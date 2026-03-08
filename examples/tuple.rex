-- Tuple type and let destructuring

swap pair =
    let (a, b) = pair
    in (b, a)

fst pair =
    let (a, _) = pair
    in a

snd pair =
    let (_, b) = pair
    in b

test "swap" =
    assert (swap (1, 2) == (2, 1))

test "fst and snd" =
    assert (fst (10, 20) == 10)
    assert (snd (10, 20) == 20)

test "destructuring" =
    let (x, y) = (10, 20)
    assert (x + y == 30)
