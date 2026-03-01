import std:IO (print)
import std:List (map, filter, sum)

let xs = [1, 2, 3, 4, 5]
let doubled = map (fn x -> x * 2) xs
let evens = filter (fn x -> x % 2 == 0) xs

print (sum doubled)
