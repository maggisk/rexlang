export abs, min, max, sin, cos, tan, asin, acos, atan, atan2, log, exp, pow, sqrt, pi, e, clamp, degrees, radians, logBase, toFloat, round, floor, ceiling, truncate


-- # Derived


-- | Restrict a value to a given range.
--
--     clamp 0 10 15 == 10
--     clamp 0 10 -3 == 0
--
clamp : a -> a -> a -> a
let clamp lo hi x =
    max lo (min hi x)


-- | Convert radians to degrees.
--
--     degrees pi == 180.0
--
degrees : Float -> Float
let degrees r =
    r * (180.0 / pi)


-- | Convert degrees to radians.
--
--     radians 180.0 == 3.141592653589793
--
radians : Float -> Float
let radians d =
    d * (pi / 180.0)


-- | Logarithm with a given base.
--
--     logBase 10.0 100.0 == 2.0
--
logBase : Float -> Float -> Float
let logBase base x =
    log x / log base
