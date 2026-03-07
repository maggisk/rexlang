-- Lib.Helpers module: nested module example

import Std:List (map, foldl)

export
sumDoubles lst =
    foldl (\acc x -> acc + x * 2) 0 lst

export
squares : [a] -> [a]
squares lst =
    map (\x -> x * x) lst
