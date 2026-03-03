-- Math module examples

import std:Math (clamp, pi, degrees)

let area r = pi * r * r

test "clamp" =
    assert (clamp 0 10 15 == 10)
    assert (clamp 0 10 5 == 5)
    assert (clamp 0 10 (0 - 5) == 0)

test "area" =
    assert (area 1.0 == pi)
