-- Map module examples

import std:Map as M

let m = M.insert 3 30 (M.insert 2 20 (M.insert 1 10 M.empty))

test "map operations" =
    assert (M.size m == 3)
    assert (M.lookup 1 m == Just 10)
    assert (M.lookup 2 m == Just 20)
    assert (M.lookup 3 m == Just 30)
    assert (M.foldl (fn k v acc -> acc + v) 0 m == 60)

true
