-- Import examples

import Std:List (map, filter, sum)

xs = [1, 2, 3, 4, 5]
doubled = xs |> map (\x -> x * 2)
evens = xs |> filter (\x -> x % 2 == 0)

test "import Std:List" =
    assert (sum doubled == 30)
    assert (sum evens == 6)
