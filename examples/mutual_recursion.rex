-- Mutual recursion: isEven and isOdd

let rec isEven n =
    if n == 0 then
        true
    else
        isOdd (n - 1)
and isOdd n =
    if n == 0 then
        false
    else
        isEven (n - 1)

test "isEven" =
    assert (isEven 0)
    assert (isEven 10)
    assert (not (isEven 7))

test "isOdd" =
    assert (isOdd 1)
    assert (isOdd 11)
    assert (not (isOdd 8))

true
