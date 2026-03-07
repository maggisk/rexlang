-- Map module examples

import Std:Map as M
import Std:Maybe (Just, Nothing)

m = M.empty |> M.insert 1 10 |> M.insert 2 20 |> M.insert 3 30

test "map operations" =
    assert (M.size m == 3)
    assert (M.lookup 1 m == Just 10)
    assert (M.lookup 2 m == Just 20)
    assert (M.lookup 3 m == Just 30)
    assert (M.foldl (\k v acc -> acc + v) 0 m == 60)
