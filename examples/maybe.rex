import std:IO (print)
import std:Maybe (Nothing, Just, isNothing, fromMaybe, map)


let double x = x * 2

print (fromMaybe 0 (Just 7))
