-- Mutual recursion: isEven and isOdd

isEven n =
    if n == 0 then
        true
    else
        isOdd (n - 1)

isOdd n =
    if n == 0 then
        false
    else
        isEven (n - 1)

test "isEven" =
    assert (isEven 0)
    assert (isEven 10)
    assert (7 |> isEven |> not)

test "isOdd" =
    assert (isOdd 1)
    assert (isOdd 11)
    assert (8 |> isOdd |> not)
