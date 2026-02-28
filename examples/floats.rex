import std:IO (print)
import std:Math (sqrt, toFloat)

-- Float arithmetic


let pi = 3.14159
let circleArea r = pi * r * r
let hypotenuse a b = sqrt (toFloat a * toFloat a + toFloat b * toFloat b)

print (circleArea 5.0)
