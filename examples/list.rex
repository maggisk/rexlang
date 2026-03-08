-- Built-in list examples

xs = [1, 2, 3, 4, 5]

sum lst =
    match lst
        when [] ->
            0
        when [h|t] ->
            h + sum t

test "sum" =
    assert (sum xs == 15)
    assert (sum [] == 0)

test "cons and pattern match" =
    let ys = 0 :: xs
    assert (sum ys == 15)
    match ys
        when [h|_] ->
            assert (h == 0)
        when _ ->
            assert false
