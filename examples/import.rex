-- Import examples

import std:List (map, filter, sum)

let xs = [1, 2, 3, 4, 5]
let doubled = xs |> map (fn x -> x * 2)
let evens = xs |> filter (fn x -> x % 2 == 0)

test "import std:List" =
    assert (sum doubled == 30)
    assert (sum evens == 6)

true
