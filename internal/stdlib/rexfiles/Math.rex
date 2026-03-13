-- # Builtins

export external toFloat : Int -> Float

export external round : Float -> Int

export external floor : Float -> Int

export external ceiling : Float -> Int

export external truncate : Float -> Int

export external sqrt : Float -> Float

export external abs : a -> a

export external min : a -> a -> a

export external max : a -> a -> a

export external pow : Float -> Float -> Float

export external sin : Float -> Float

export external cos : Float -> Float

export external tan : Float -> Float

export external asin : Float -> Float

export external acos : Float -> Float

export external atan : Float -> Float

export external atan2 : Float -> Float -> Float

export external log : Float -> Float

export external exp : Float -> Float

export external pi : Float

export external e : Float


-- # Derived


-- | Restrict a value to a given range.
--
--     clamp 0 10 15 == 10
--     clamp 0 10 -3 == 0
--
export
clamp : a -> a -> a -> a
clamp lo hi x =
    max lo (min hi x)


-- | Convert radians to degrees.
--
--     degrees pi == 180.0
--
export
degrees : Float -> Float
degrees r =
    r * (180.0 / pi)


-- | Convert degrees to radians.
--
--     radians 180.0 == 3.141592653589793
--
export
radians : Float -> Float
radians d =
    d * (pi / 180.0)


-- | Logarithm with a given base.
--
--     logBase 10.0 100.0 == 2.0
--
export
logBase : Float -> Float -> Float
logBase base x =
    log x / log base
