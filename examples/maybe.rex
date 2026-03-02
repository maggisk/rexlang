-- Maybe module examples

import std:Maybe (Nothing, Just, isNothing, fromMaybe, map)


let double x = x * 2

test "fromMaybe" =
    assert (fromMaybe 0 (Just 7) == 7)
    assert (fromMaybe 0 Nothing == 0)

test "map" =
    assert (map double (Just 5) == Just 10)
    assert (map double Nothing == Nothing)

test "isNothing" =
    assert (isNothing Nothing)
    assert (not (isNothing (Just 1)))

true
