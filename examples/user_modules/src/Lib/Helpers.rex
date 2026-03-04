-- Lib.Helpers module: nested module example

import Std:List (map, foldl)

export let sumDoubles lst =
    foldl (\acc x -> acc + x * 2) 0 lst

export let squares lst =
    map (\x -> x * x) lst
