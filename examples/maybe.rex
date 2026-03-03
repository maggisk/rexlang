-- Maybe module examples

import std:Maybe (Nothing, Just, isNothing, fromMaybe, map)


let double x = x * 2

test "fromMaybe" =
    assert (Just 7 |> fromMaybe 0 == 7)
    assert (fromMaybe 0 Nothing == 0)

test "map" =
    assert (Just 5 |> map double == Just 10)
    assert (map double Nothing == Nothing)

test "isNothing" =
    assert (isNothing Nothing)
    assert (Just 1 |> isNothing |> not)
