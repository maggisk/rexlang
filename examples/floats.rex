-- Float arithmetic

import std:Math (sqrt, toFloat)


let pi = 3.14159
let circleArea r = pi * r * r
let hypotenuse a b = sqrt (toFloat a * toFloat a + toFloat b * toFloat b)

test "circle area" =
    assert (circleArea 1.0 == 3.14159)

test "hypotenuse" =
    assert (hypotenuse 3 4 == 5.0)

true
