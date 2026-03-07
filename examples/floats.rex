-- Float arithmetic

import Std:Math (sqrt, toFloat)


pi = 3.14159
circleArea r = pi * r * r
hypotenuse a b = (toFloat a * toFloat a + toFloat b * toFloat b) |> sqrt

test "circle area" =
    assert (circleArea 1.0 == 3.14159)

test "hypotenuse" =
    assert (hypotenuse 3 4 == 5.0)
